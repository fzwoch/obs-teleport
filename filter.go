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

// #include <obs-module.h>
// #include <util/dstr.h>
//
// extern bool filter_apply_clicked(obs_properties_t *props, obs_property_t *property, uintptr_t data);
// extern bool quality_warning_callback(obs_properties_t *properties, obs_property_t *prop, obs_data_t *settings);
//
import "C"
import (
	"image"
	"math"
	"net"
	"runtime/cgo"
	"strconv"
	"sync"
	"unsafe"
)

type teleportFilter struct {
	sync.Mutex
	sync.WaitGroup
	Announcer
	Sender
	done        chan any
	filter      *C.obs_source_t
	queue       []*Packet
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
		done:        make(chan any),
		filter:      source,
		offsetVideo: math.MaxUint64,
		offsetAudio: math.MaxUint64,
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

	prop = C.obs_properties_add_int_slider(properties, quality_str, quality_readable_str, 0, 100, 1)
	C.obs_property_set_modified_callback(prop, C.obs_property_modified_t(unsafe.Pointer(C.quality_warning_callback)))

	C.obs_properties_add_button(properties, apply_str, apply_str, C.obs_property_clicked_t(unsafe.Pointer(C.filter_apply_clicked)))

	prop = C.obs_properties_add_text(properties, quality_warning, quality_warning_str, C.OBS_TEXT_INFO)
	C.obs_property_text_set_info_type(prop, C.OBS_TEXT_INFO_WARNING)

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

	if h.SenderGetNumConns() == 0 {
		return frame
	}

	p := &Packet{
		Header: Header{
			Timestamp: uint64(frame.timestamp - h.offsetVideo),
		},
	}

	settings := C.obs_source_get_settings(h.filter)
	p.Quality = int(C.obs_data_get_int(settings, quality_str))
	C.obs_data_release(settings)

	p.ToImage(frame.width, frame.height, frame.format, frame.data)
	if p.Image == nil {
		return frame
	}

	switch p.Image.(type) {
	case *image.YCbCr:
		copy(p.ImageHeader.ColorMatrix[:], (unsafe.Slice((*float32)(&frame.color_matrix[0]), 16)))
		if frame.full_range {
			p.ImageHeader.ColorRangeMin = [3]float32{0, 0, 0}
			p.ImageHeader.ColorRangeMax = [3]float32{1, 1, 1}
		} else {
			copy(p.ImageHeader.ColorRangeMin[:], (unsafe.Slice((*float32)(&frame.color_range_min[0]), 3)))
			copy(p.ImageHeader.ColorRangeMax[:], (unsafe.Slice((*float32)(&frame.color_range_max[0]), 3)))
		}
	default:
		C.video_format_get_parameters(C.VIDEO_CS_SRGB, C.VIDEO_RANGE_FULL, (*C.float)(unsafe.Pointer(&p.ImageHeader.ColorMatrix[0])), (*C.float)(unsafe.Pointer(&p.ImageHeader.ColorRangeMin[0])), (*C.float)(unsafe.Pointer(&p.ImageHeader.ColorRangeMax[0])))
	}

	h.Lock()
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
			h.SenderSend(h.queue[0].Buffer)
			h.queue = h.queue[1:]
		}
	}(p)

	return frame
}

//export filter_audio
func filter_audio(data C.uintptr_t, frames *C.struct_obs_audio_data) *C.struct_obs_audio_data {
	h := cgo.Handle(data).Value().(*teleportFilter)

	if h.offsetAudio == math.MaxUint64 {
		h.offsetAudio = frames.timestamp
	}

	if h.SenderGetNumConns() == 0 {
		return frames
	}

	audio := C.obs_get_audio()
	info := C.audio_output_get_info(audio) //FIXME: output??

	p := Packet{
		Header: Header{
			Timestamp: uint64(frames.timestamp - h.offsetAudio),
		},
	}

	p.ToWAVE(info, frames.frames, frames.data)

	h.SenderSend(p.Buffer)

	return frames
}

func filter_loop(h *teleportFilter) {
	defer h.Done()
	defer h.SenderClose()

	settings := C.obs_source_get_settings(h.filter)
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

	audioAndVideo := false
	if C.astrcmpi(C.obs_source_get_id(h.filter), filter_str) == 0 {
		audioAndVideo = true
	}

	h.StartAnnouncer(name, port, audioAndVideo)
	defer h.StopAnnouncer()

	<-h.done
}
