//
// obs-teleport. OBS Studio plugin.
// Copyright (C) 2021-2024 Florian Zwoch <fzwoch@gmail.com>
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
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"net"
	"os"
	"runtime/cgo"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"
	"unsafe"
)

type Peer struct {
	Payload AnnouncePayload
	Time    time.Time
}

type teleportSource struct {
	sync.Mutex
	sync.WaitGroup
	Discoverer
	done            chan any
	services        map[string]Peer
	source          *C.obs_source_t
	queueLock       sync.Mutex
	queue           []*Packet
	frame           *C.struct_obs_source_frame
	audio           *C.struct_obs_source_audio
	isStart         bool
	isAudioAndVideo bool
	offset          uint64
	pool            *Pool
}

var (
	teleport_list_str    = C.CString("teleport_list")
	refresh_readable_str = C.CString("Refresh List")
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
		done:     make(chan any),
		services: map[string]Peer{},
		source:   source,
		frame:    (*C.struct_obs_source_frame)(C.bzalloc(C.sizeof_struct_obs_source_frame)),
		audio:    (*C.struct_obs_source_audio)(C.bzalloc(C.sizeof_struct_obs_source_audio)),
		pool:     NewPool(10),
	}

	h.Add(1)
	go h.sourceLoop()

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
	keys := make([]string, 0, len(h.services))

	for k := range h.services {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		service := h.services[k]

		key := C.CString(k)
		val := C.CString(fmt.Sprintf("%s / %s:%d", service.Payload.Name, service.Payload.Address, service.Payload.Port))

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

	return properties
}

//export source_get_defaults
func source_get_defaults(settings *C.obs_data_t) {
	C.obs_data_set_default_string(settings, teleport_list_str, empty_str)
}

//export source_update
func source_update(data C.uintptr_t, settings *C.obs_data_t) {
	h := cgo.Handle(data).Value().(*teleportSource)

	h.done <- nil
	h.Wait()

	h.Add(1)
	go h.sourceLoop()
}

//export source_activate
func source_activate(data C.uintptr_t) {
	h := cgo.Handle(data).Value().(*teleportSource)

	C.obs_source_output_video(h.source, nil)
}

func (t *teleportSource) newPacket(p *Packet) {
	t.queueLock.Lock()

	t.queue = append(t.queue, p)

	sort.Slice(t.queue, func(i, j int) bool {
		return t.queue[i].Header.Timestamp < t.queue[j].Header.Timestamp
	})

	queueSize := time.Duration(t.queue[len(t.queue)-1].Header.Timestamp - t.queue[0].Header.Timestamp)

	if queueSize > 5*time.Second {
		blog(C.LOG_WARNING, "decode queue exceeded: "+queueSize.String())
	}

	t.queueLock.Unlock()

	t.Add(1)
	go func(p *Packet) {
		defer t.Done()

		defer func() {
			if r, ok := recover().(error); ok {
				blog(C.LOG_ERROR, "jpeg corrupt, discarding.. "+r.Error())

				t.queueLock.Lock()
				defer t.queueLock.Unlock()

				for i, packet := range t.queue {
					if p == packet {
						t.queue = slices.Delete(t.queue, i, i+1)
						break
					}
				}
			}
		}()

		if !p.IsAudio {
			p.FromJPEG(t.pool)
		}

		t.queueLock.Lock()
		defer t.queueLock.Unlock()

		p.DoneProcessing = true

		for len(t.queue) > 0 && t.queue[0].DoneProcessing {
			p = t.queue[0]

			if t.isAudioAndVideo {
				hasAudioAndVideo := false
				for _, n := range t.queue[1:] {
					if n.IsAudio != p.IsAudio {
						hasAudioAndVideo = true
						break
					}
				}
				if !hasAudioAndVideo {
					return
				}
			}

			if t.isStart {
				for i := len(t.queue) - 1; i >= 0; i-- {
					if t.queue[i].IsAudio != t.queue[len(t.queue)-1].IsAudio {
						t.queue = t.queue[i:]
						break
					}
				}
				t.isStart = false
				t.offset = t.queue[0].Header.Timestamp
				continue
			}

			if p.IsAudio {
				t.audio.timestamp = C.uint64_t(p.Header.Timestamp - t.offset)
				t.audio.samples_per_sec = C.uint(p.WaveHeader.SampleRate)
				t.audio.speakers = uint32(p.WaveHeader.Speakers)
				t.audio.format = uint32(p.WaveHeader.Format)
				t.audio.frames = C.uint(p.WaveHeader.Frames)
				t.audio.data[0] = (*C.uint8_t)(unsafe.Pointer(&p.Buffer[0]))

				C.obs_source_output_audio(t.source, t.audio)

				t.audio.data[0] = nil
			} else {
				switch p.Image.(type) {
				case *image.YCbCr:
					img := p.Image.(*image.YCbCr)

					t.frame.linesize[0] = C.uint(img.YStride)
					t.frame.linesize[1] = C.uint(img.CStride)
					t.frame.linesize[2] = C.uint(img.CStride)
					t.frame.data[0] = (*C.uint8_t)(unsafe.Pointer(&img.Y[0]))
					t.frame.data[1] = (*C.uint8_t)(unsafe.Pointer(&img.Cb[0]))
					t.frame.data[2] = (*C.uint8_t)(unsafe.Pointer(&img.Cr[0]))

					switch img.SubsampleRatio {
					case image.YCbCrSubsampleRatio444:
						t.frame.format = C.VIDEO_FORMAT_I444
					case image.YCbCrSubsampleRatio422:
						t.frame.format = C.VIDEO_FORMAT_I422
					default:
						t.frame.format = C.VIDEO_FORMAT_I420
					}

					if p.ImageHeader.ColorRangeMin == [3]float32{0, 0, 0} && p.ImageHeader.ColorRangeMax == [3]float32{1, 1, 1} {
						t.frame.full_range = true
					} else {
						t.frame.full_range = false
					}

					t.frame.width = C.uint(p.Image.Bounds().Dx())
					t.frame.height = C.uint(p.Image.Bounds().Dy())
					t.frame.timestamp = C.uint64_t(p.Header.Timestamp - t.offset)

					copy(unsafe.Slice((*float32)(&t.frame.color_matrix[0]), 16), p.ImageHeader.ColorMatrix[:])
					copy(unsafe.Slice((*float32)(&t.frame.color_range_min[0]), 3), p.ImageHeader.ColorRangeMin[:])
					copy(unsafe.Slice((*float32)(&t.frame.color_range_max[0]), 3), p.ImageHeader.ColorRangeMax[:])

					C.obs_source_output_video(t.source, t.frame)

					t.pool.Put(bytes.NewBuffer(img.Y))

					t.frame.data[0] = nil
					t.frame.data[1] = nil
					t.frame.data[2] = nil
				case *image.RGBA:
					img := p.Image.(*image.RGBA)

					t.frame.linesize[0] = C.uint(img.Stride)
					t.frame.data[0] = (*C.uint8_t)(unsafe.Pointer(&img.Pix[0]))
					t.frame.format = C.VIDEO_FORMAT_BGR3

					if p.ImageHeader.ColorRangeMin == [3]float32{0, 0, 0} && p.ImageHeader.ColorRangeMax == [3]float32{1, 1, 1} {
						t.frame.full_range = true
					} else {
						t.frame.full_range = false
					}

					t.frame.width = C.uint(p.Image.Bounds().Dx())
					t.frame.height = C.uint(p.Image.Bounds().Dy())
					t.frame.timestamp = C.uint64_t(p.Header.Timestamp - t.offset)

					copy(unsafe.Slice((*float32)(&t.frame.color_matrix[0]), 16), p.ImageHeader.ColorMatrix[:])
					copy(unsafe.Slice((*float32)(&t.frame.color_range_min[0]), 3), p.ImageHeader.ColorRangeMin[:])
					copy(unsafe.Slice((*float32)(&t.frame.color_range_max[0]), 3), p.ImageHeader.ColorRangeMax[:])

					C.obs_source_output_video(t.source, t.frame)

					t.pool.Put(bytes.NewBuffer(img.Pix))

					t.frame.data[0] = nil
				default:
					panic("invalid video format")
				}
			}

			t.queue[0] = nil
			t.queue = t.queue[1:]
		}
	}(p)
}

func (h *teleportSource) sourceLoop() {
	defer h.Done()

	h.StartDiscoverer(h.services, h)
	defer h.StopDiscoverer()

	discover := make(chan any)
	defer close(discover)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var (
		c         net.Conn
		connMutex sync.Mutex
		shutdown  bool
	)

	dial := make(chan any)
	defer func() {
		close(dial)

		connMutex.Lock()
		shutdown = true
		if c != nil {
			c.Close()
		}
		connMutex.Unlock()
	}()

	var run = func() {}

	run = func() {
		defer h.Done()

		defer func() {
			if r, ok := recover().(error); ok {
				blog(C.LOG_ERROR, "stream corrupt, re-trying.. "+r.Error())

				time.Sleep(time.Second)

				h.Add(1)
				go run()
			}
		}()

		settings := C.obs_source_get_settings(h.source)

		teleport := C.GoString(C.obs_data_get_string(settings, teleport_list_str))

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
				blog(C.LOG_INFO, "disconnected from: "+c.RemoteAddr().String())
				c.Close()
			}
			c, err = net.DialTimeout("tcp", service.Payload.Address+":"+strconv.Itoa(service.Payload.Port), 100*time.Millisecond)
			connMutex.Unlock()

			if err != nil {
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					time.Sleep(100 * time.Millisecond)
				}
				blog(C.LOG_DEBUG, err.Error())
				continue
			}

			blog(C.LOG_INFO, "connected to: "+c.RemoteAddr().String())
			if service.Payload.Version != "" && service.Payload.Version != version {
				blog(C.LOG_WARNING, "version mismatch: "+service.Payload.Version+" != "+version)
			}

			h.audio.timestamp = math.MaxUint64
			h.audio.samples_per_sec = 48000
			h.audio.speakers = 2
			h.audio.format = C.AUDIO_FORMAT_FLOAT
			h.audio.frames = 0

			C.obs_source_output_audio(h.source, h.audio)

			h.isStart = true
			h.queue = nil
			h.isAudioAndVideo = service.Payload.AudioAndVideo

			for {
				p := &Packet{}

				err = binary.Read(c, binary.LittleEndian, &p.Header)
				if err != nil {
					break
				}
				switch p.Header.Type {
				case [4]byte{'J', 'P', 'E', 'G'}:
					err = binary.Read(c, binary.LittleEndian, &p.ImageHeader)
					if err != nil {
						break
					}
				case [4]byte{'W', 'A', 'V', 'E'}:
					err = binary.Read(c, binary.LittleEndian, &p.WaveHeader)
					if err != nil {
						break
					}
					p.IsAudio = true
				case [4]byte{'A', 'N', 'J', 'A'}:
					fallthrough
				default:
					break
				}

				p.Buffer = make([]byte, p.Header.Size)

				_, err := io.ReadFull(c, p.Buffer)
				if err != nil {
					break
				}

				h.newPacket(p)
			}
		}
	}

	h.Add(1)
	go run()

	for {
		select {
		case t := <-ticker.C:
			h.Lock()
			for key, peer := range h.services {
				if peer.Time.Before(t) {
					delete(h.services, key)
				}
			}
			h.Unlock()
		case <-h.done:
			return
		}
	}
}
