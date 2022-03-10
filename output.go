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

type queueInfo struct {
	b            []byte
	timestamp    uint64
	done         bool
	image_header image_header
}

type teleportOutput struct {
	sync.Mutex
	sync.WaitGroup
	conn          net.Conn
	done          chan interface{}
	output        *C.obs_output_t
	queueLock     sync.Mutex
	data          []*queueInfo
	quality       int
	droppedFrames int
}

//export output_get_name
func output_get_name(type_data C.uintptr_t) *C.char {
	return frontend_str
}

//export output_create
func output_create(settings *C.obs_data_t, output *C.obs_output_t) C.uintptr_t {
	h := &teleportOutput{
		output: output,
	}

	return C.uintptr_t(cgo.NewHandle(h))
}

//export output_destroy
func output_destroy(data C.uintptr_t) {
	cgo.Handle(data).Delete()
}

//export output_start
func output_start(data C.uintptr_t) C.bool {
	h := cgo.Handle(data).Value().(*teleportOutput)

	if !C.obs_output_can_begin_data_capture(h.output, 0) {
		return false
	}

	h.done = make(chan interface{})
	h.droppedFrames = 0

	h.Add(1)
	go output_loop(h)

	C.obs_output_begin_data_capture(h.output, 0)

	return true
}

//export output_stop
func output_stop(data C.uintptr_t, ts C.uint64_t) {
	h := cgo.Handle(data).Value().(*teleportOutput)

	// hack: prevent deadlock, why called multiple times?
	if h.done == nil {
		return
	}

	C.obs_output_end_data_capture(h.output)

	h.done <- nil
	h.Wait()

	close(h.done)
	h.done = nil
}

//export output_raw_video
func output_raw_video(data C.uintptr_t, frame *C.struct_video_data) {
	h := cgo.Handle(data).Value().(*teleportOutput)

	h.Lock()
	if h.conn == nil {
		h.Unlock()

		return
	}
	h.Unlock()

	video := C.obs_output_video(h.output)
	info := C.video_output_get_info(video)

	img := createImage(C.obs_output_get_width(h.output), C.obs_output_get_height(h.output), info.format, frame.data)
	if img == nil {
		return
	}

	j := &queueInfo{
		timestamp: uint64(frame.timestamp),
	}

	C.video_format_get_parameters(info.colorspace, info._range, (*C.float)(unsafe.Pointer(&j.image_header.ColorMatrix[0])), (*C.float)(unsafe.Pointer(&j.image_header.ColorRangeMin[0])), (*C.float)(unsafe.Pointer(&j.image_header.ColorRangeMax[0])))

	h.queueLock.Lock()
	if len(h.data) > 20 {
		h.droppedFrames++
		h.queueLock.Unlock()
		return
	}

	h.data = append(h.data, j)
	h.queueLock.Unlock()

	h.Add(1)
	go func(j *queueInfo, img image.Image) {
		defer h.Done()

		j.b = createJpegBuffer(img, j.timestamp, j.image_header, h.quality)

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
}

//export output_raw_audio
func output_raw_audio(data C.uintptr_t, frames *C.struct_audio_data) {
	h := cgo.Handle(data).Value().(*teleportOutput)

	h.Lock()
	if h.conn == nil {
		h.Unlock()
		return
	}
	h.Unlock()

	audio := C.obs_output_audio(h.output)
	info := C.audio_output_get_info(audio)

	f := &C.struct_obs_audio_data{
		frames:    frames.frames,
		timestamp: frames.timestamp,
		data:      frames.data,
	}

	buffer := createAudioBuffer(info, f)

	h.Lock()
	defer h.Unlock()

	if h.conn != nil {
		_, err := h.conn.Write(buffer)
		if err != nil {
			h.conn.Close()
			h.conn = nil
		}
	}
}

//export output_get_dropped_frames
func output_get_dropped_frames(data C.uintptr_t) C.int {
	h := cgo.Handle(data).Value().(*teleportOutput)

	h.queueLock.Lock()
	defer h.queueLock.Unlock()

	return C.int(h.droppedFrames)
}

func output_loop(h *teleportOutput) {
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

		settings := C.obs_source_get_settings(dummy)
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
