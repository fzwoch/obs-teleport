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
// #include <util/dstr.h>
//
// bool filter_apply_clicked(obs_properties_t *props, obs_property_t *property, uintptr_t data);
//
import "C"
import (
	"encoding/json"
	"image"
	"math"
	"net"
	"os"
	"runtime/cgo"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/schollz/peerdiscovery"
)

type teleportFilter struct {
	sync.Mutex
	sync.WaitGroup
	conns       map[net.Conn]interface{}
	connsLock   sync.Mutex
	done        chan interface{}
	filter      *C.obs_source_t
	queueLock   sync.Mutex
	data        []*queueInfo
	audioOnly   bool
	videoOnly   bool
	offsetVideo C.uint64_t
	offsetAudio C.uint64_t
}

//export filter_get_name
func filter_get_name(type_data C.uintptr_t) *C.char {
	return frontend_str
}

//export filter_video_get_name
func filter_video_get_name(type_data C.uintptr_t) *C.char {
	return frontend_video_str
}

//export filter_audio_get_name
func filter_audio_get_name(type_data C.uintptr_t) *C.char {
	return frontend_audio_str
}

//export filter_create
func filter_create(settings *C.obs_data_t, source *C.obs_source_t) C.uintptr_t {
	h := &teleportFilter{
		conns:       make(map[net.Conn]interface{}),
		done:        make(chan interface{}),
		filter:      source,
		offsetVideo: math.MaxUint64,
		offsetAudio: math.MaxUint64,
	}

	if C.astrcmpi(C.obs_source_get_id(source), filter_audio_str) == 0 {
		h.audioOnly = true
	} else if C.astrcmpi(C.obs_source_get_id(source), filter_video_str) == 0 {
		h.videoOnly = true
	}

	h.Add(1)
	go filter_loop(h)

	return C.uintptr_t(cgo.NewHandle(h))
}

//export filter_destroy
func filter_destroy(data C.uintptr_t) {
	h := cgo.Handle(data).Value().(*teleportFilter)

	h.done <- nil
	h.Wait()

	close(h.done)

	cgo.Handle(data).Delete()
}

//export filter_apply_clicked
func filter_apply_clicked(properties *C.obs_properties_t, prop *C.obs_property_t, data C.uintptr_t) C.bool {
	filter_update(data, nil)

	return false
}

//export filter_get_properties
func filter_get_properties(data C.uintptr_t) *C.obs_properties_t {
	properties := C.obs_properties_create()

	C.obs_properties_set_flags(properties, C.OBS_PROPERTIES_DEFER_UPDATE)

	prop := C.obs_properties_add_text(properties, identifier_str, identifier_readable_str, C.OBS_TEXT_DEFAULT)
	C.obs_property_set_long_description(prop, identifier_description_str)

	prop = C.obs_properties_add_int(properties, port_str, port_readable_str, 0, math.MaxUint16, 1)
	C.obs_property_set_long_description(prop, port_description_str)

	C.obs_properties_add_int_slider(properties, quality_str, quality_readable_str, 0, 100, 1)

	C.obs_properties_add_button(properties, apply_str, apply_str, C.obs_property_clicked_t(unsafe.Pointer(C.filter_apply_clicked)))

	return properties
}

//export filter_get_defaults
func filter_get_defaults(settings *C.obs_data_t) {
	C.obs_data_set_default_string(settings, identifier_str, empty_str)
	C.obs_data_set_default_int(settings, port_str, 0)
	C.obs_data_set_default_int(settings, quality_str, 90)
}

//export filter_update
func filter_update(data C.uintptr_t, settings *C.obs_data_t) {
	h := cgo.Handle(data).Value().(*teleportFilter)

	h.done <- nil
	h.Wait()

	h.offsetVideo = math.MaxUint64
	h.offsetAudio = math.MaxUint64

	h.Add(1)
	go filter_loop(h)
}

//export filter_video
func filter_video(data C.uintptr_t, frame *C.struct_obs_source_frame) *C.struct_obs_source_frame {
	h := cgo.Handle(data).Value().(*teleportFilter)

	if h.offsetVideo == math.MaxUint64 {
		h.offsetVideo = frame.timestamp
	}

	h.Lock()
	if len(h.conns) == 0 {
		h.Unlock()
		return frame
	}
	h.Unlock()

	settings := C.obs_source_get_settings(h.filter)
	quality := int(C.obs_data_get_int(settings, quality_str))
	C.obs_data_release(settings)

	img := createImage(frame.width, frame.height, frame.format, frame.data)
	if img == nil {
		return frame
	}

	j := &queueInfo{
		timestamp: uint64(frame.timestamp - h.offsetVideo),
	}

	switch img.(type) {
	case *image.YCbCr:
		copy(j.image_header.ColorMatrix[:], (unsafe.Slice((*float32)(&frame.color_matrix[0]), 16)))
		copy(j.image_header.ColorRangeMin[:], (unsafe.Slice((*float32)(&frame.color_range_min[0]), 3)))
		copy(j.image_header.ColorRangeMax[:], (unsafe.Slice((*float32)(&frame.color_range_max[0]), 3)))
		if frame.full_range {
			j.image_header.ColorRangeMin = [3]float32{0, 0, 0}
			j.image_header.ColorRangeMax = [3]float32{1, 1, 1}
		}
	default:
		C.video_format_get_parameters(C.VIDEO_CS_SRGB, C.VIDEO_RANGE_FULL, (*C.float)(unsafe.Pointer(&j.image_header.ColorMatrix[0])), (*C.float)(unsafe.Pointer(&j.image_header.ColorRangeMin[0])), (*C.float)(unsafe.Pointer(&j.image_header.ColorRangeMax[0])))
	}

	h.queueLock.Lock()
	if len(h.data) > 0 && time.Duration(h.data[len(h.data)-1].timestamp-h.data[0].timestamp) > time.Second {
		//	j.b = createDummyJpegBuffer(j.timestamp)
	}

	h.data = append(h.data, j)
	h.queueLock.Unlock()

	h.Add(1)
	go func(j *queueInfo, img image.Image) {
		defer h.Done()

		if j.b == nil {
			j.b = createJpegBuffer(img, j.timestamp, j.image_header, quality)
		}

		if h.videoOnly {
			//			j.b = append(j.b, createDummyAudioBuffer(j.timestamp)...)
		}

		h.queueLock.Lock()
		defer h.queueLock.Unlock()

		j.done = true

		for len(h.data) > 0 && h.data[0].done {
			h.Lock()
			h.connsLock.Lock()

			for c := range h.conns {
				go func(c net.Conn, j *queueInfo) {
					_, err := c.Write(j.b)
					if err != nil {
						c.Close()
						h.connsLock.Lock()
						delete(h.conns, c)
						h.connsLock.Unlock()
					}
				}(c, h.data[0])
			}

			h.connsLock.Unlock()
			h.Unlock()

			h.data = h.data[1:]
		}
	}(j, img)

	return frame
}

//export filter_audio
func filter_audio(data C.uintptr_t, frames *C.struct_obs_audio_data) *C.struct_obs_audio_data {
	h := cgo.Handle(data).Value().(*teleportFilter)

	if h.offsetVideo == math.MaxUint64 {
		h.offsetVideo = frames.timestamp
	}

	h.Lock()
	if len(h.conns) == 0 {
		h.Unlock()

		return frames
	}
	h.Unlock()

	audio := C.obs_get_audio()
	info := C.audio_output_get_info(audio)

	buffer := createAudioBuffer(info, uint64(frames.timestamp-h.offsetAudio), frames)

	if h.audioOnly {
		//		buffer = append(buffer, createDummyJpegBuffer(uint64(frames.timestamp-h.offsetAudio))...)
	}

	h.Lock()
	defer h.Unlock()

	h.connsLock.Lock()
	defer h.connsLock.Unlock()

	for c := range h.conns {
		go func(c net.Conn, buffer []byte) {
			_, err := c.Write(buffer)
			if err != nil {
				c.Close()
				h.connsLock.Lock()
				delete(h.conns, c)
				h.connsLock.Unlock()
			}
		}(c, buffer)
	}

	return frames
}

func filter_loop(h *teleportFilter) {
	defer h.Done()

	defer func() {
		h.Lock()
		defer h.Unlock()

		for c := range h.conns {
			c.Close()
			delete(h.conns, c)
		}

	}()

	settings := C.obs_source_get_settings(h.filter)
	listenPort := int(C.obs_data_get_int(settings, port_str))
	C.obs_data_release(settings)

	l, err := net.Listen("tcp", ":"+strconv.Itoa(listenPort))
	if err != nil {
		panic(err)
	}
	defer l.Close()

	h.Add(1)
	go func() {
		defer h.Done()

		for {
			c, err := l.Accept()
			if err != nil {
				break
			}

			h.Lock()
			h.conns[c] = nil
			h.Unlock()
		}
	}()

	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		panic(err)
	}

	discover := make(chan struct{})
	defer close(discover)

	h.Add(1)
	go func() {
		defer h.Done()

		p, _ := strconv.Atoi(port)

		settings := C.obs_source_get_settings(h.filter)
		name := C.GoString(C.obs_data_get_string(settings, identifier_str))
		C.obs_data_release(settings)

		if name == "" {
			name, err = os.Hostname()
			if err != nil {
				name = "(None)"
			}
		}

		j := struct {
			Name string
			Port int
		}{
			Name: name,
			Port: p,
		}

		b, _ := json.Marshal(j)

		peerdiscovery.Discover(peerdiscovery.Settings{
			TimeLimit: -1,
			StopChan:  discover,
			Payload:   b,
		})
	}()

	<-h.done
}
