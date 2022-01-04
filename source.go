//
// obs-teleport. OBS Studio plugin.
// Copyright (C) 2021-2022 Florian Zwoch <fzwoch@gmail.com>
//
// This file is part of obs-teleport.
//
// obs-teleport is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 2 of the License, or
// (at your option) any later version.
//
// obs-teleport is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with obs-teleport. If not, see <http://www.gnu.org/licenses/>.
//

package main

//
// #include <obs-module.h>
// #include <util/platform.h>
//
import "C"
import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"net"
	"os"
	"runtime/cgo"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/pixiv/go-libjpeg/jpeg"
	"github.com/schollz/peerdiscovery"
)

type peer struct {
	name    string
	address string
	port    int
	time    time.Time
}

type imageInfo struct {
	timestamp int64
	b         []byte
	image     *image.YCbCr
	done      bool
}

type teleportSource struct {
	sync.Mutex
	sync.WaitGroup
	done      chan interface{}
	services  map[string]peer
	source    *C.obs_source_t
	imageLock sync.Mutex
	images    []*imageInfo
	frame     *C.struct_obs_source_frame
	audio     *C.struct_obs_source_audio
}

//export source_get_name
func source_get_name(type_data C.uintptr_t) *C.char {
	return frontend_str
}

//export source_create
func source_create(settings *C.obs_data_t, source *C.obs_source_t) C.uintptr_t {
	h := &teleportSource{
		done:     make(chan interface{}),
		services: map[string]peer{},
		source:   source,
		frame:    (*C.struct_obs_source_frame)(C.bzalloc(C.sizeof_struct_obs_source_frame)),
		audio:    (*C.struct_obs_source_audio)(C.bzalloc(C.sizeof_struct_obs_source_audio)),
	}

	C.video_format_get_parameters(C.VIDEO_CS_709, C.VIDEO_RANGE_PARTIAL, &h.frame.color_matrix[0], &h.frame.color_range_min[0], &h.frame.color_range_max[0])

	h.Add(1)

	go source_loop(h)

	return C.uintptr_t(cgo.NewHandle(h))
}

//export source_destroy
func source_destroy(data C.uintptr_t) {
	h := cgo.Handle(data).Value().(*teleportSource)

	h.done <- nil
	h.Wait()

	close(h.done)

	C.bfree(unsafe.Pointer(h.frame))
	C.bfree(unsafe.Pointer(h.audio))

	cgo.Handle(data).Delete()
}

//export source_get_properties
func source_get_properties(data C.uintptr_t) *C.obs_properties_t {
	h := cgo.Handle(data).Value().(*teleportSource)

	properties := C.obs_properties_create()

	prop := C.obs_properties_add_list(properties, C.CString("teleport_list"), frontend_str, C.OBS_COMBO_TYPE_LIST, C.OBS_COMBO_FORMAT_STRING)
	C.obs_property_list_add_string(prop, C.CString("- Disabled -"), C.CString(""))

	h.Lock()
	for key, service := range h.services {
		key := C.CString(key)
		val := C.CString(fmt.Sprintf("%s / %s:%d", service.name, service.address, service.port))

		C.obs_property_list_add_string(prop, val, key)

		C.free(unsafe.Pointer(key))
		C.free(unsafe.Pointer(val))
	}
	h.Unlock()

	C.obs_properties_add_int_slider(properties, C.CString("quality"), C.CString("Quality"), 0, 100, 1)

	prop = C.obs_properties_add_bool(properties, C.CString("ignore_timestamps"), C.CString("Ignore timestamps"))
	C.obs_property_set_long_description(prop, C.CString("May help against long time synchronization clock drifts, but may also increase jitter."))

	return properties
}

//export source_get_defaults
func source_get_defaults(settings *C.obs_data_t) {
	tel := C.CString("teleport_list")
	str := C.CString("")
	qua := C.CString("quality")

	C.obs_data_set_default_string(settings, tel, str)
	C.obs_data_set_default_int(settings, qua, 90)
	C.obs_data_set_default_bool(settings, C.CString("ignore_timestamps"), false)

	C.free(unsafe.Pointer(tel))
	C.free(unsafe.Pointer(str))
	C.free(unsafe.Pointer(qua))
}

//export source_update
func source_update(data C.uintptr_t, settings *C.obs_data_t) {
	h := cgo.Handle(data).Value().(*teleportSource)

	h.done <- nil
	h.Wait()

	h.Add(1)

	go source_loop(h)
}

func source_loop(h *teleportSource) {
	defer h.Done()

	discover := make(chan struct{})
	defer close(discover)

	h.Add(1)

	go func() {
		defer h.Done()

		peerdiscovery.Discover(peerdiscovery.Settings{
			TimeLimit:        -1,
			StopChan:         discover,
			AllowSelf:        true,
			DisableBroadcast: true,
			Notify: func(d peerdiscovery.Discovered) {
				j := struct {
					Name string
					Port int
				}{}

				err := json.Unmarshal(d.Payload, &j)
				if err != nil {
					panic(err)
				}

				h.Lock()
				h.services[j.Name+":"+d.Address] = peer{
					name:    j.Name,
					address: d.Address,
					port:    j.Port,
					time:    time.Now().Add(5 * time.Second),
				}
				h.Unlock()
			},
		})
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var (
		c         net.Conn
		connMutex sync.Mutex
		shutdown  bool
	)

	dial := make(chan struct{})
	defer func() {
		close(dial)

		connMutex.Lock()

		shutdown = true

		if c != nil {
			c.Close()
		}

		connMutex.Unlock()
	}()

	h.Add(1)

	go func() {
		defer h.Done()

		settings := C.obs_source_get_settings(h.source)

		tel := C.CString("teleport_list")
		teleport := C.GoString(C.obs_data_get_string(settings, tel))
		C.free(unsafe.Pointer(tel))

		qua := C.CString("quality")
		quality := C.obs_data_get_int(settings, qua)
		C.free(unsafe.Pointer(qua))

		C.obs_data_release(settings)

		if teleport == "" {
			C.obs_source_output_video(h.source, nil)

			return
		}

		for {
			select {
			case <-dial:
				return
			default:
			}

			C.obs_source_output_video(h.source, nil)

			h.Lock()
			service, ok := h.services[teleport]
			h.Unlock()

			if !ok {
				time.Sleep(time.Second)
				continue
			}

			var (
				err        error
				videoCount uint64
				audioCount uint64
			)

			connMutex.Lock()
			if shutdown {
				connMutex.Unlock()
				return
			}

			if c != nil {
				c.Close()
			}
			c, err = net.DialTimeout("tcp", service.address+":"+strconv.Itoa(service.port), time.Second)
			connMutex.Unlock()

			if err != nil {
				log.Println(err)
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					time.Sleep(time.Second)
				}
				continue
			}

			j, err := json.Marshal(&options{
				Quality: int(quality),
			})
			if err != nil {
				log.Println(err)
				continue
			}

			b := bytes.Buffer{}

			binary.Write(&b, binary.LittleEndian, &options_header{
				Magic: [4]byte{'O', 'P', 'T', 'S'},
				Size:  int32(len(j)),
			})

			buffers := net.Buffers{
				b.Bytes(),
				j,
			}

			_, err = buffers.WriteTo(c)
			if err != nil {
				log.Println(err)
				continue
			}

			for {
				var header header

				err = binary.Read(c, binary.LittleEndian, &header)
				if err != nil {
					log.Println(err)
					break
				}
				switch header.Type {
				case [4]byte{'J', 'P', 'E', 'G'}:
				case [4]byte{'W', 'A', 'V', 'E'}:
				default:
					panic("UNKNOWN HEADER TYPE")
				}

				b := make([]byte, header.Size)

				_, err := io.ReadFull(c, b)
				if err != nil {
					log.Println(err)
					break
				}

				switch header.Type {
				case [4]byte{'J', 'P', 'E', 'G'}:
					info := &imageInfo{
						timestamp: header.Timestamp,
						b:         b,
					}

					h.imageLock.Lock()
					h.images = append(h.images, info)
					h.imageLock.Unlock()

					h.Add(1)

					go func(info *imageInfo) {
						defer h.Done()

						reader := bytes.NewReader(info.b)

						img, err := jpeg.Decode(reader, &jpeg.DecoderOptions{})
						if err != nil {
							panic(err)
						}
						info.image = img.(*image.YCbCr)

						h.imageLock.Lock()
						defer h.imageLock.Unlock()

						info.done = true

						for len(h.images) > 0 && h.images[0].done {
							h.frame.width = C.uint(h.images[0].image.Bounds().Dx())
							h.frame.height = C.uint(h.images[0].image.Bounds().Dy())
							h.frame.format = C.VIDEO_FORMAT_I420
							h.frame.timestamp = C.uint64_t(h.images[0].timestamp)
							h.frame.linesize[0] = C.uint(h.images[0].image.YStride)
							h.frame.linesize[1] = C.uint(h.images[0].image.CStride)
							h.frame.linesize[2] = C.uint(h.images[0].image.CStride)
							h.frame.data[0] = (*C.uint8_t)(unsafe.Pointer(&h.images[0].image.Y[0]))
							h.frame.data[1] = (*C.uint8_t)(unsafe.Pointer(&h.images[0].image.Cb[0]))
							h.frame.data[2] = (*C.uint8_t)(unsafe.Pointer(&h.images[0].image.Cr[0]))

							settings := C.obs_source_get_settings(h.source)
							if C.obs_data_get_bool(settings, C.CString("ignore_timestamps")) {
								h.frame.timestamp = C.uint64_t(videoCount)
								videoCount++

							}
							C.obs_data_release(settings)

							C.obs_source_output_video(h.source, h.frame)

							h.frame.data[0] = nil
							h.frame.data[1] = nil
							h.frame.data[2] = nil

							h.images = h.images[1:]
						}
					}(info)
				case [4]byte{'W', 'A', 'V', 'E'}:
					h.audio.timestamp = C.uint64_t(header.Timestamp)
					h.audio.samples_per_sec = 48000
					h.audio.speakers = C.SPEAKERS_STEREO
					h.audio.format = C.AUDIO_FORMAT_16BIT
					h.audio.frames = C.uint(header.Size) / 4
					h.audio.data[0] = (*C.uint8_t)(unsafe.Pointer(&b[0]))

					settings := C.obs_source_get_settings(h.source)
					if C.obs_data_get_bool(settings, C.CString("ignore_timestamps")) {
						h.audio.timestamp = C.uint64_t(audioCount)
						audioCount++
					}
					C.obs_data_release(settings)

					C.obs_source_output_audio(h.source, h.audio)

					h.audio.data[0] = nil
				}
			}
		}
	}()

	for {
		select {
		case t := <-ticker.C:
			h.Lock()
			for key, peer := range h.services {
				if peer.time.Before(t) {
					delete(h.services, key)
				}
			}
			h.Unlock()
		case <-h.done:
			return
		}
	}
}
