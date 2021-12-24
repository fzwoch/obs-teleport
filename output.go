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
	"bytes"
	"encoding/binary"
	"encoding/json"
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

type jpegInfo struct {
	b         bytes.Buffer
	timestamp int64
	done      bool
}

type teleportOutput struct {
	sync.Mutex
	conn    net.Conn
	done    chan interface{}
	output  *C.obs_output_t
	data    []*jpegInfo
	quality int
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

	video_info := C.struct_video_scale_info{
		format: C.VIDEO_FORMAT_I420,
	}
	C.obs_output_set_video_conversion(output, &video_info)

	audio_info := C.struct_audio_convert_info{
		samples_per_sec: 48000,
		format:          C.AUDIO_FORMAT_16BIT,
		speakers:        C.SPEAKERS_STEREO,
	}
	C.obs_output_set_audio_conversion(output, &audio_info)

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

	C.obs_output_begin_data_capture(h.output, 0)

	go output_loop(h)

	return true
}

//export output_stop
func output_stop(data C.uintptr_t, ts C.uint64_t) {
	h := cgo.Handle(data).Value().(*teleportOutput)

	if h.done == nil { // obs calls this twice?
		return
	}

	C.obs_output_end_data_capture(h.output)

	h.done <- nil
	<-h.done

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

	img := image.NewYCbCr(
		image.Rectangle{
			Min: image.Point{0, 0},
			Max: image.Point{
				X: int(C.obs_output_get_width(h.output)),
				Y: int(C.obs_output_get_height(h.output)),
			},
		},
		image.YCbCrSubsampleRatio420)

	copy(img.Y, unsafe.Slice((*byte)(frame.data[0]), len(img.Y)))
	copy(img.Cb, unsafe.Slice((*byte)(frame.data[1]), len(img.Cb)))
	copy(img.Cr, unsafe.Slice((*byte)(frame.data[2]), len(img.Cr)))

	j := &jpegInfo{
		b:         bytes.Buffer{},
		timestamp: int64(frame.timestamp),
	}

	h.Lock()
	h.data = append(h.data, j)
	h.Unlock()

	go func(j *jpegInfo, img *image.YCbCr) {
		jpeg.Encode(&j.b, img, &jpeg.EncoderOptions{
			Quality: h.quality,
		})

		h.Lock()
		defer h.Unlock()

		j.done = true

		for len(h.data) > 0 && h.data[0].done {
			b := bytes.Buffer{}

			binary.Write(&b, binary.LittleEndian, &header{
				Type:      [4]byte{'J', 'P', 'E', 'G'},
				Timestamp: h.data[0].timestamp,
				Size:      int32(h.data[0].b.Len()),
			})

			buffers := net.Buffers{
				b.Bytes(),
				h.data[0].b.Bytes(),
			}

			if h.conn != nil {
				_, err := buffers.WriteTo(h.conn)
				if err != nil {
					h.conn.Close()
					h.conn = nil
				}
			}

			h.data = h.data[1:]
		}
	}(j, img)
}

//export output_raw_audio
func output_raw_audio(data C.uintptr_t, frames *C.struct_audio_data) {
	h := cgo.Handle(data).Value().(*teleportOutput)

	b := bytes.Buffer{}

	binary.Write(&b, binary.LittleEndian, &header{
		Type:      [4]byte{'W', 'A', 'V', 'E'},
		Timestamp: int64(frames.timestamp),
		Size:      int32(frames.frames) * 4,
	})

	buffers := net.Buffers{
		b.Bytes(),
		C.GoBytes(unsafe.Pointer(frames.data[0]), C.int(frames.frames)*4),
	}

	h.Lock()
	defer h.Unlock()

	if h.conn != nil {
		_, err := buffers.WriteTo(h.conn)
		if err != nil {
			h.conn.Close()
			h.conn = nil
		}
	}
}

func output_loop(h *teleportOutput) {
	defer close(h.done)

	defer func() {
		for {
			h.Lock()
			len := len(h.data)
			h.Unlock()

			if len == 0 {
				break
			}

			time.Sleep(time.Millisecond)
		}
	}()

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

	go func() {
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
				log.Println(err)
				return
			}
			if header.Magic != [4]byte{'O', 'P', 'T', 'S'} {
				panic("")
			}

			b := make([]byte, header.Size)

			_, err = io.ReadFull(h.conn, b)
			if err != nil {
				log.Println(err)
				return
			}

			var options options

			err = json.Unmarshal(b, &options)
			if err != nil {
				panic(err)
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

	go func() {
		p, _ := strconv.Atoi(port)

		settings := C.obs_source_get_settings(dummy)
		name := C.GoString(C.obs_data_get_string(settings, C.CString("identifier")))
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
