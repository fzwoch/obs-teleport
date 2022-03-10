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
//
// extern bool refresh_list(obs_properties_t *props, obs_property_t *property, uintptr_t data);
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
	timestamp    uint64
	b            []byte
	image        image.Image
	done         bool
	image_header image_header
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

var (
	teleport_list_str    = C.CString("teleport_list")
	refresh_readable_str = C.CString("Refresh List")
	quality_str          = C.CString("quality")
	quality_readable_str = C.CString("Quality")
	no_services_str      = C.CString("Press 'Refresh List' to search for streams")
	disabled_str         = C.CString("- Disabled -")
)

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

//export refresh_list
func refresh_list(props *C.obs_properties_t, property *C.obs_property_t, data C.uintptr_t) C.bool {
	h := cgo.Handle(data).Value().(*teleportSource)

	prop := C.obs_properties_get(props, teleport_list_str)
	C.obs_property_list_clear(prop)

	if len(h.services) == 0 {
		C.obs_property_set_enabled(prop, false)
		C.obs_property_list_add_string(prop, no_services_str, empty_str)
	} else {
		C.obs_property_set_enabled(prop, true)
		C.obs_property_list_add_string(prop, disabled_str, empty_str)
	}

	h.Lock()
	for key, service := range h.services {
		key := C.CString(key)
		val := C.CString(fmt.Sprintf("%s / %s:%d", service.name, service.address, service.port))

		C.obs_property_list_add_string(prop, val, key)

		C.free(unsafe.Pointer(key))
		C.free(unsafe.Pointer(val))
	}
	h.Unlock()

	return true
}

//export source_get_properties
func source_get_properties(data C.uintptr_t) *C.obs_properties_t {
	properties := C.obs_properties_create()

	C.obs_properties_add_list(properties, teleport_list_str, frontend_str, C.OBS_COMBO_TYPE_LIST, C.OBS_COMBO_FORMAT_STRING)
	refresh_list(properties, nil, data)

	C.obs_properties_add_button(properties, refresh_readable_str, refresh_readable_str, C.obs_property_clicked_t(unsafe.Pointer(C.refresh_list)))
	C.obs_properties_add_int_slider(properties, quality_str, quality_readable_str, 0, 100, 1)

	return properties
}

//export source_get_defaults
func source_get_defaults(settings *C.obs_data_t) {
	C.obs_data_set_default_string(settings, teleport_list_str, empty_str)
	C.obs_data_set_default_int(settings, quality_str, 90)
}

//export source_update
func source_update(data C.uintptr_t, settings *C.obs_data_t) {
	h := cgo.Handle(data).Value().(*teleportSource)

	h.done <- nil
	h.Wait()

	h.Add(1)
	go source_loop(h)
}

//export source_activate
func source_activate(data C.uintptr_t) {
	h := cgo.Handle(data).Value().(*teleportSource)

	C.obs_source_output_video(h.source, nil)
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
					return
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

		teleport := C.GoString(C.obs_data_get_string(settings, teleport_list_str))
		quality := C.obs_data_get_int(settings, quality_str)

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
				time.Sleep(100 * time.Millisecond)
				continue
			}

			var err error

			connMutex.Lock()
			if shutdown {
				connMutex.Unlock()
				return
			}

			if c != nil {
				c.Close()
			}
			c, err = net.DialTimeout("tcp", service.address+":"+strconv.Itoa(service.port), 100*time.Millisecond)
			connMutex.Unlock()

			if err != nil {
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					time.Sleep(100 * time.Millisecond)
				}
				continue
			}

			j, err := json.Marshal(&options{
				Quality: int(quality),
			})
			if err != nil {
				continue
			}

			b := bytes.Buffer{}

			binary.Write(&b, binary.LittleEndian, &options_header{
				Magic: [4]byte{'O', 'P', 'T', 'S'},
				Size:  int32(len(j)),
			})

			_, err = c.Write(append(b.Bytes(), j...))
			if err != nil {
				continue
			}

			for {
				var (
					header       header
					image_header image_header
					wave_header  wave_header
				)

				err = binary.Read(c, binary.LittleEndian, &header)
				if err != nil {
					break
				}
				switch header.Type {
				case [4]byte{'J', 'P', 'E', 'G'}:
					err = binary.Read(c, binary.LittleEndian, &image_header)
					if err != nil {
						break
					}
				case [4]byte{'W', 'A', 'V', 'E'}:
					err = binary.Read(c, binary.LittleEndian, &wave_header)
					if err != nil {
						break
					}
				case [4]byte{'A', 'N', 'J', 'A'}:
					fallthrough
				default:
					break
				}

				b := make([]byte, header.Size)

				_, err := io.ReadFull(c, b)
				if err != nil {
					break
				}

				switch header.Type {
				case [4]byte{'J', 'P', 'E', 'G'}:
					if !C.obs_source_showing(h.source) {
						continue
					}

					info := &imageInfo{
						timestamp:    header.Timestamp,
						b:            b,
						image_header: image_header,
					}

					h.imageLock.Lock()
					if len(h.images) > 20 {
						h.imageLock.Unlock()
						continue
					}

					h.images = append(h.images, info)
					h.imageLock.Unlock()

					h.Add(1)
					go func(info *imageInfo) {
						defer h.Done()

						reader := bytes.NewReader(info.b)

						img, _ := jpeg.Decode(reader, &jpeg.DecoderOptions{})

						h.imageLock.Lock()
						defer h.imageLock.Unlock()

						info.image = img
						info.done = true

						for len(h.images) > 0 && h.images[0].done {
							i := h.images[0]

							if i == nil {
								h.images = h.images[1:]
								continue
							}

							switch i.image.(type) {
							case *image.YCbCr:
								img := i.image.(*image.YCbCr)

								h.frame.linesize[0] = C.uint(img.YStride)
								h.frame.linesize[1] = C.uint(img.CStride)
								h.frame.linesize[2] = C.uint(img.CStride)
								h.frame.data[0] = (*C.uint8_t)(unsafe.Pointer(&img.Y[0]))
								h.frame.data[1] = (*C.uint8_t)(unsafe.Pointer(&img.Cb[0]))
								h.frame.data[2] = (*C.uint8_t)(unsafe.Pointer(&img.Cr[0]))

								switch img.SubsampleRatio {
								case image.YCbCrSubsampleRatio444:
									h.frame.format = C.VIDEO_FORMAT_I444
								case image.YCbCrSubsampleRatio422:
									h.frame.format = C.VIDEO_FORMAT_I422
								default:
									h.frame.format = C.VIDEO_FORMAT_I420
								}
							default:
								h.images = h.images[1:]
								continue
							}

							h.frame.width = C.uint(i.image.Bounds().Dx())
							h.frame.height = C.uint(i.image.Bounds().Dy())
							h.frame.timestamp = C.uint64_t(i.timestamp)

							copy(unsafe.Slice((*float32)(&h.frame.color_matrix[0]), 16), i.image_header.ColorMatrix[:])
							copy(unsafe.Slice((*float32)(&h.frame.color_range_min[0]), 3), i.image_header.ColorRangeMin[:])
							copy(unsafe.Slice((*float32)(&h.frame.color_range_max[0]), 3), i.image_header.ColorRangeMax[:])

							C.obs_source_output_video(h.source, h.frame)

							h.frame.data[0] = nil
							h.frame.data[1] = nil
							h.frame.data[2] = nil

							h.images = h.images[1:]
						}
					}(info)
				case [4]byte{'W', 'A', 'V', 'E'}:
					h.audio.timestamp = C.uint64_t(header.Timestamp)
					h.audio.samples_per_sec = C.uint(wave_header.SampleRate)
					h.audio.speakers = uint32(wave_header.Speakers)
					h.audio.format = uint32(wave_header.Format)
					h.audio.frames = C.uint(wave_header.Frames)
					h.audio.data[0] = (*C.uint8_t)(unsafe.Pointer(&b[0]))

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
