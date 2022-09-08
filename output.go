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
	"math"
	"net"
	"runtime/cgo"
	"strconv"
	"sync"
	"time"
	"unsafe"
)

type teleportOutput struct {
	sync.Mutex
	sync.WaitGroup
	Announcer
	Sender
	done         chan any
	output       *C.obs_output_t
	queue        []*Packet
	laggedFrames int
	offsetVideo  C.uint64_t
	offsetAudio  C.uint64_t
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

	h.done = make(chan any)
	h.laggedFrames = 0
	h.offsetVideo = math.MaxUint64
	h.offsetAudio = math.MaxUint64

	h.Add(1)
	go h.outputLoop()

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

	p := &Packet{
		Header: Header{
			Timestamp: uint64(frame.timestamp - h.offsetVideo),
		},
	}

	settings := C.obs_source_get_settings(dummy)
	p.Quality = int(C.obs_data_get_int(settings, quality_str))
	C.obs_data_release(settings)

	video := C.obs_output_video(h.output)
	info := C.video_output_get_info(video)

	p.ToImage(C.obs_output_get_width(h.output), C.obs_output_get_height(h.output), info.format, frame.data)
	if p.Image == nil {
		return
	}

	C.video_format_get_parameters(info.colorspace, info._range, (*C.float)(unsafe.Pointer(&p.ImageHeader.ColorMatrix[0])), (*C.float)(unsafe.Pointer(&p.ImageHeader.ColorRangeMin[0])), (*C.float)(unsafe.Pointer(&p.ImageHeader.ColorRangeMax[0])))

	h.Lock()
	if len(h.queue) > 0 && time.Duration(h.queue[len(h.queue)-1].Header.Timestamp-h.queue[0].Header.Timestamp) > time.Second {
		h.laggedFrames++
	}

	h.queue = append(h.queue, p)
	h.Unlock()

	h.Add(1)
	go func(p *Packet) {
		defer h.Done()

		p.ToJPEG()

		h.Lock()
		defer h.Unlock()

		p.DoneProcessing = true

		for len(h.queue) > 0 && h.queue[0].DoneProcessing {
			h.SenderSend(p.Buffer)
			h.queue = h.queue[1:]
		}
	}(p)
}

//export output_raw_audio
func output_raw_audio(data C.uintptr_t, frames *C.struct_audio_data) {
	h := cgo.Handle(data).Value().(*teleportOutput)

	if h.offsetAudio == math.MaxUint64 {
		h.offsetAudio = frames.timestamp
	}

	audio := C.obs_output_audio(h.output)
	info := C.audio_output_get_info(audio)

	p := Packet{
		Header: Header{
			Timestamp: uint64(frames.timestamp - h.offsetAudio),
		},
	}

	p.ToWAVE(info, frames.frames, frames.data)

	h.SenderSend(p.Buffer)
}

//export output_get_dropped_frames
func output_get_dropped_frames(data C.uintptr_t) C.int {
	h := cgo.Handle(data).Value().(*teleportOutput)

	h.Lock()
	defer h.Unlock()

	return C.int(h.laggedFrames)
}

func (h *teleportOutput) outputLoop() {
	defer h.Done()
	defer h.SenderClose()

	settings := C.obs_source_get_settings(dummy)
	name := C.GoString(C.obs_data_get_string(settings, identifier_str))
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

			h.SenderAdd(c)
		}
	}()

	_, p, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		panic(err)
	}

	port, _ := strconv.Atoi(p)

	h.StartAnnouncer(name, port, true)
	defer h.StopAnnouncer()

	<-h.done
}
