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
// bool filter_apply_clicked(obs_properties_t *props, obs_property_t *property, uintptr_t data);
//
import "C"
import (
	"encoding/binary"
	"encoding/json"
	"image"
	"io"
	"net"
	"os"
	"runtime/cgo"
	"strconv"
	"sync"
	"unsafe"

	"github.com/schollz/peerdiscovery"
)

type teleportFilter struct {
	sync.Mutex
	sync.WaitGroup
	conn      net.Conn
	done      chan interface{}
	filter    *C.obs_source_t
	queueLock sync.Mutex
	data      []*queueInfo
	quality   int
}

//export filter_get_name
func filter_get_name(type_data C.uintptr_t) *C.char {
	return frontend_str
}

//export filter_audio_get_name
func filter_audio_get_name(type_data C.uintptr_t) *C.char {
	return frontend_audio_str
}

//export filter_create
func filter_create(settings *C.obs_data_t, source *C.obs_source_t) C.uintptr_t {
	h := &teleportFilter{
		done:   make(chan interface{}),
		filter: source,
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

	C.obs_properties_add_button(properties, apply_str, apply_str, C.obs_property_clicked_t(unsafe.Pointer(C.filter_apply_clicked)))

	return properties
}

//export filter_get_defaults
func filter_get_defaults(settings *C.obs_data_t) {
	C.obs_data_set_default_string(settings, identifier_str, empty_str)
}

//export filter_update
func filter_update(data C.uintptr_t, settings *C.obs_data_t) {
	h := cgo.Handle(data).Value().(*teleportFilter)

	h.done <- nil
	h.Wait()

	h.Add(1)
	go filter_loop(h)
}

//export filter_video
func filter_video(data C.uintptr_t, frame *C.struct_obs_source_frame) *C.struct_obs_source_frame {
	h := cgo.Handle(data).Value().(*teleportFilter)

	h.Lock()
	if h.conn == nil {
		h.Unlock()
		return frame
	}
	h.Unlock()

	img := createImage(frame.width, frame.height, frame.format, frame.data)
	if img == nil {
		return frame
	}

	j := &queueInfo{
		timestamp: uint64(frame.timestamp),
	}

	h.queueLock.Lock()
	if len(h.data) > 20 {
		h.queueLock.Unlock()
		return frame
	}

	h.data = append(h.data, j)
	h.queueLock.Unlock()

	h.Add(1)
	go func(j *queueInfo, img image.Image) {
		defer h.Done()

		j.b = createJpegBuffer(img, j.timestamp, h.quality)

		h.queueLock.Lock()
		defer h.queueLock.Unlock()

		j.done = true

		for len(h.data) > 0 && h.data[0].done {
			h.Lock()
			if h.conn != nil {
				_, err := h.conn.Write(h.data[0].b)
				if err != nil {
					h.conn.Close()
					h.conn = nil
				}
			}
			h.Unlock()

			h.data = h.data[1:]
		}
	}(j, img)

	return frame
}

//export filter_audio
func filter_audio(data C.uintptr_t, frames *C.struct_obs_audio_data) *C.struct_obs_audio_data {
	h := cgo.Handle(data).Value().(*teleportFilter)

	h.Lock()
	if h.conn == nil {
		h.Unlock()

		return frames
	}
	h.Unlock()

	audio := C.obs_get_audio()
	info := C.audio_output_get_info(audio)

	buffer := createAudioBuffer(info, frames)

	h.Lock()
	defer h.Unlock()

	if h.conn != nil {
		_, err := h.conn.Write(buffer)
		if err != nil {
			h.conn.Close()
			h.conn = nil
		}
	}

	return frames
}

func filter_loop(h *teleportFilter) {
	defer h.Done()

	defer func() {
		h.Lock()
		defer h.Unlock()

		if h.conn != nil {
			h.conn.Close()
			h.conn = nil
		}
	}()

	l, err := net.Listen("tcp", "")
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
			if h.conn != nil {
				h.conn.Close()
				h.conn = nil
			}
			h.conn = c

			var header options_header

			err = binary.Read(h.conn, binary.LittleEndian, &header)
			if err != nil {
				h.Unlock()
				continue
			}
			if header.Magic != [4]byte{'O', 'P', 'T', 'S'} {
				h.conn.Close()
				h.conn = nil
				h.Unlock()
				continue
			}

			b := make([]byte, header.Size)

			_, err = io.ReadFull(h.conn, b)
			if err != nil {
				h.Unlock()
				continue
			}

			var options options

			err = json.Unmarshal(b, &options)
			if err != nil {
				h.conn.Close()
				h.conn = nil
				h.Unlock()
				continue
			}

			h.quality = options.Quality
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
