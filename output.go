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

type queueInfo struct {
	b            []byte
	timestamp    uint64
	done         bool
	image_header image_header
}

type teleportOutput struct {
	sync.Mutex
	sync.WaitGroup
	conns         map[net.Conn]interface{}
	connsLock     sync.Mutex
	done          chan interface{}
	output        *C.obs_output_t
	queueLock     sync.Mutex
	data          []*queueInfo
	droppedFrames int
	offsetVideo   C.ulong
	offsetAudio   C.ulong
}

//export output_get_name
func output_get_name(type_data C.uintptr_t) *C.char {
	return frontend_str
}

//export output_create
func output_create(settings *C.obs_data_t, output *C.obs_output_t) C.uintptr_t {
	h := &teleportOutput{
		conns:       make(map[net.Conn]interface{}),
		output:      output,
		offsetVideo: math.MaxUint64,
		offsetAudio: math.MaxUint64,
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

	if h.offsetVideo == math.MaxUint64 {
		h.offsetVideo = frame.timestamp
	}

	h.Lock()
	if len(h.conns) == 0 {
		h.Unlock()
		return
	}
	h.Unlock()

	settings := C.obs_source_get_settings(dummy)
	quality := int(C.obs_data_get_int(settings, quality_str))
	C.obs_data_release(settings)

	video := C.obs_output_video(h.output)
	info := C.video_output_get_info(video)

	img := createImage(C.obs_output_get_width(h.output), C.obs_output_get_height(h.output), info.format, frame.data)
	if img == nil {
		return
	}

	j := &queueInfo{
		timestamp: uint64(frame.timestamp - h.offsetVideo),
	}

	C.video_format_get_parameters(info.colorspace, info._range, (*C.float)(unsafe.Pointer(&j.image_header.ColorMatrix[0])), (*C.float)(unsafe.Pointer(&j.image_header.ColorRangeMin[0])), (*C.float)(unsafe.Pointer(&j.image_header.ColorRangeMax[0])))

	h.queueLock.Lock()
	if len(h.data) > 0 && time.Duration(h.data[len(h.data)-1].timestamp-h.data[0].timestamp) > time.Nanosecond {
		h.droppedFrames++
		h.queueLock.Unlock()
		return
	}

	h.data = append(h.data, j)
	h.queueLock.Unlock()

	h.Add(1)
	go func(j *queueInfo, img image.Image) {
		defer h.Done()

		j.b = createJpegBuffer(img, j.timestamp, j.image_header, quality)

		h.queueLock.Lock()
		defer h.queueLock.Unlock()

		j.done = true

		for len(h.data) > 0 && h.data[0].done {
			h.Lock()
			var wg sync.WaitGroup
			for c := range h.conns {
				wg.Add(1)
				go func(c net.Conn) {
					defer wg.Done()
					_, err := c.Write(h.data[0].b)
					if err != nil {
						c.Close()
						h.connsLock.Lock()
						delete(h.conns, c)
						h.connsLock.Unlock()
					}
				}(c)
			}
			wg.Wait()
			h.Unlock()

			h.data = h.data[1:]
		}
	}(j, img)
}

//export output_raw_audio
func output_raw_audio(data C.uintptr_t, frames *C.struct_audio_data) {
	h := cgo.Handle(data).Value().(*teleportOutput)

	if h.offsetAudio == math.MaxUint64 {
		h.offsetAudio = frames.timestamp
	}

	h.Lock()
	if len(h.conns) == 0 {
		h.Unlock()
		return
	}
	h.Unlock()

	audio := C.obs_output_audio(h.output)
	info := C.audio_output_get_info(audio)

	f := &C.struct_obs_audio_data{
		frames: frames.frames,
		data:   frames.data,
	}

	buffer := createAudioBuffer(info, uint64(frames.timestamp-h.offsetAudio), f)

	h.Lock()
	defer h.Unlock()

	var wg sync.WaitGroup
	defer wg.Wait()

	for c := range h.conns {
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			_, err := c.Write(buffer)
			if err != nil {
				c.Close()
				h.connsLock.Lock()
				delete(h.conns, c)
				h.connsLock.Unlock()
			}
		}(c)
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

		for c := range h.conns {
			c.Close()
			delete(h.conns, c)
		}
	}()

	settings := C.obs_source_get_settings(dummy)
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
