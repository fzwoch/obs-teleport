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
// #include <obs-frontend-api.h>
//
// typedef char* (*get_name_t)(uintptr_t type_data);
// extern char* source_get_name(uintptr_t type_data);
// extern char* filter_get_name(uintptr_t type_data);
// extern char* filter_audio_get_name(uintptr_t type_data);
// extern char* output_get_name(uintptr_t type_data);
// extern char* dummy_get_name(uintptr_t type_data);
//
// typedef uintptr_t (*source_create_t)(obs_data_t *settings, obs_source_t *source);
// extern uintptr_t source_create(obs_data_t *settings, obs_source_t *source);
// extern uintptr_t filter_create(obs_data_t *settings, obs_source_t *source);
// extern uintptr_t dummy_create(obs_data_t *settings, obs_source_t *source);
//
// typedef uintptr_t (*output_create_t)(obs_data_t *settings, obs_output_t *output);
// extern uintptr_t output_create(obs_data_t *settings, obs_output_t *output);
//
// typedef void (*destroy_t)(uintptr_t data);
// extern void source_destroy(uintptr_t data);
// extern void filter_destroy(uintptr_t data);
// extern void output_destroy(uintptr_t data);
// extern void dummy_destroy(uintptr_t data);
//
// typedef obs_properties_t* (*get_properties_t)(uintptr_t data);
// extern obs_properties_t* source_get_properties(uintptr_t data);
// extern obs_properties_t* filter_get_properties(uintptr_t data);
// extern obs_properties_t* dummy_get_properties(uintptr_t data);
//
// typedef void (*get_defaults_t)(obs_data_t *settings);
// extern void source_get_defaults(obs_data_t *settings);
// extern void filter_get_defaults(obs_data_t *settings);
// extern void dummy_get_defaults(obs_data_t *settings);
//
// typedef void (*update_t)(uintptr_t data, obs_data_t *settings);
// extern void (source_update)(uintptr_t data, obs_data_t *settings);
// extern void (filter_update)(uintptr_t data, obs_data_t *settings);
// extern void (dummy_update)(uintptr_t data, obs_data_t *settings);
//
// typedef void (*activate_t)(uintptr_t data);
// extern void source_activate(uintptr_t data);
//
// typedef struct obs_source_frame* (*filter_video_t)(uintptr_t data, struct obs_source_frame *frames);
// extern struct obs_source_frame* filter_video(uintptr_t data, struct obs_source_frame *frames);
//
// typedef struct obs_audio_data* (*filter_audio_t)(uintptr_t data, struct obs_audio_data *frames);
// extern struct obs_audio_data* filter_audio(uintptr_t data, struct obs_audio_data *frames);
//
// typedef void (*raw_video_t)(uintptr_t data, struct video_data *frame);
// extern void output_raw_video(uintptr_t data, struct video_data *frame);
//
// typedef void (*raw_audio_t)(uintptr_t data, struct audio_data *frames);
// extern void output_raw_audio(uintptr_t data, struct audio_data *frames);
//
// typedef bool (*start_t)(uintptr_t data);
// extern bool output_start(uintptr_t data);
//
// typedef void (*stop_t)(uintptr_t data, uint64_t ts);
// extern void output_stop(uintptr_t data, uint64_t ts);
//
// typedef int (*get_dropped_frames_t)(uintptr_t data);
// extern int output_get_dropped_frames(uintptr_t data);
//
// extern void frontend_cb(uintptr_t data);
// extern void frontend_event_cb(enum obs_frontend_event event, uintptr_t data);
//
// static void blog_version(const char* string) {
//   blog(LOG_INFO, "[obs-teleport] Version %s", string);
// }
//
import "C"
import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"unsafe"

	"github.com/pixiv/go-libjpeg/jpeg"
	"github.com/pixiv/go-libjpeg/rgb"
)

var obsModulePointer *C.obs_module_t

//export obs_module_set_pointer
func obs_module_set_pointer(module *C.obs_module_t) {
	obsModulePointer = module
}

//export obs_current_module
func obs_current_module() *C.obs_module_t {
	return obsModulePointer
}

//export obs_module_ver
func obs_module_ver() C.uint32_t {
	return C.LIBOBS_API_VER
}

var (
	source_str         = C.CString("teleport-source")
	output_str         = C.CString("teleport-output")
	filter_video_str   = C.CString("teleport-video-filter")
	filter_audio_str   = C.CString("teleport-audio-filter")
	frontend_str       = C.CString("Teleport")
	frontend_audio_str = C.CString("Teleport (Audio)")
	dummy_str          = C.CString("teleport-dummy")

	version = "0.0.0"
)

//export obs_module_load
func obs_module_load() C.bool {
	v := C.CString(version)
	C.blog_version(v)
	C.free(unsafe.Pointer(v))

	C.obs_register_source_s(&C.struct_obs_source_info{
		id:             source_str,
		_type:          C.OBS_SOURCE_TYPE_INPUT,
		output_flags:   C.OBS_SOURCE_ASYNC_VIDEO | C.OBS_SOURCE_AUDIO | C.OBS_SOURCE_DO_NOT_DUPLICATE,
		get_name:       C.get_name_t(unsafe.Pointer(C.source_get_name)),
		create:         C.source_create_t(unsafe.Pointer(C.source_create)),
		destroy:        C.destroy_t(unsafe.Pointer(C.source_destroy)),
		get_properties: C.get_properties_t(unsafe.Pointer(C.source_get_properties)),
		get_defaults:   C.get_defaults_t(unsafe.Pointer(C.source_get_defaults)),
		update:         C.update_t(unsafe.Pointer(C.source_update)),
		activate:       C.activate_t(unsafe.Pointer(C.source_activate)),
	}, C.sizeof_struct_obs_source_info)

	C.obs_register_source_s(&C.struct_obs_source_info{
		id:             filter_video_str,
		_type:          C.OBS_SOURCE_TYPE_FILTER,
		output_flags:   C.OBS_SOURCE_ASYNC_VIDEO | C.OBS_SOURCE_DO_NOT_DUPLICATE,
		get_name:       C.get_name_t(unsafe.Pointer(C.filter_get_name)),
		create:         C.source_create_t(unsafe.Pointer(C.filter_create)),
		destroy:        C.destroy_t(unsafe.Pointer(C.filter_destroy)),
		get_properties: C.get_properties_t(unsafe.Pointer(C.filter_get_properties)),
		get_defaults:   C.get_defaults_t(unsafe.Pointer(C.filter_get_defaults)),
		update:         C.update_t(unsafe.Pointer(C.filter_update)),
		filter_video:   C.filter_video_t(unsafe.Pointer(C.filter_video)),
		filter_audio:   C.filter_audio_t(unsafe.Pointer(C.filter_audio)),
	}, C.sizeof_struct_obs_source_info)

	C.obs_register_source_s(&C.struct_obs_source_info{
		id:             filter_audio_str,
		_type:          C.OBS_SOURCE_TYPE_FILTER,
		output_flags:   C.OBS_SOURCE_AUDIO | C.OBS_SOURCE_DO_NOT_DUPLICATE,
		get_name:       C.get_name_t(unsafe.Pointer(C.filter_audio_get_name)),
		create:         C.source_create_t(unsafe.Pointer(C.filter_create)),
		destroy:        C.destroy_t(unsafe.Pointer(C.filter_destroy)),
		get_properties: C.get_properties_t(unsafe.Pointer(C.filter_get_properties)),
		get_defaults:   C.get_defaults_t(unsafe.Pointer(C.filter_get_defaults)),
		update:         C.update_t(unsafe.Pointer(C.filter_update)),
		filter_audio:   C.filter_audio_t(unsafe.Pointer(C.filter_audio)),
	}, C.sizeof_struct_obs_source_info)

	C.obs_register_output_s(&C.struct_obs_output_info{
		id:                 output_str,
		flags:              C.OBS_OUTPUT_AV,
		get_name:           C.get_name_t(unsafe.Pointer(C.output_get_name)),
		create:             C.output_create_t(unsafe.Pointer(C.output_create)),
		destroy:            C.destroy_t(unsafe.Pointer(C.output_destroy)),
		start:              C.start_t(unsafe.Pointer(C.output_start)),
		stop:               C.stop_t(unsafe.Pointer(C.output_stop)),
		raw_video:          C.raw_video_t(unsafe.Pointer(C.output_raw_video)),
		raw_audio:          C.raw_audio_t(unsafe.Pointer(C.output_raw_audio)),
		get_dropped_frames: C.get_dropped_frames_t(unsafe.Pointer(C.output_get_dropped_frames)),
	}, C.sizeof_struct_obs_output_info)

	C.obs_frontend_add_tools_menu_item(frontend_str, C.obs_frontend_cb(unsafe.Pointer(C.frontend_cb)), nil)
	C.obs_frontend_add_event_callback(C.obs_frontend_event_cb(unsafe.Pointer(C.frontend_event_cb)), nil)

	// this is just here to have a way to show some UI properties for the output module.
	// the frontend API has no way to display output properties, only sources.
	// so we have a dummy source just for the purpose of the interactive property page.
	C.obs_register_source_s(&C.struct_obs_source_info{
		id:             dummy_str,
		_type:          C.OBS_SOURCE_TYPE_FILTER,
		output_flags:   C.OBS_SOURCE_CAP_DISABLED,
		get_name:       C.get_name_t(unsafe.Pointer(C.dummy_get_name)),
		create:         C.source_create_t(unsafe.Pointer(C.dummy_create)),
		destroy:        C.destroy_t(unsafe.Pointer(C.dummy_destroy)),
		get_properties: C.get_properties_t(unsafe.Pointer(C.dummy_get_properties)),
		get_defaults:   C.get_defaults_t(unsafe.Pointer(C.dummy_get_defaults)),
		update:         C.update_t(unsafe.Pointer(C.dummy_update)),
	}, C.sizeof_struct_obs_source_info)

	return true
}

type header struct {
	Type      [4]byte
	Timestamp uint64
	Size      int32
}

type image_header struct {
	ColorMatrix   [16]float32
	ColorRangeMin [3]float32
	ColorRangeMax [3]float32
}

type wave_header struct {
	Format     int32
	SampleRate int32
	Speakers   int32
	Frames     int32
}

func createImage(w C.uint32_t, h C.uint32_t, format C.enum_video_format, data [C.MAX_AV_PLANES]*C.uint8_t) (img image.Image) {
	width := int(w)
	height := int(h)

	paddedHeight := height + 16

	rectangle := image.Rectangle{
		Max: image.Point{
			X: width,
			Y: height,
		},
	}

	switch format {
	case C.VIDEO_FORMAT_NV12:
		img = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/4),
			Cr:             make([]byte, width*paddedHeight/4),
			SubsampleRatio: image.YCbCrSubsampleRatio420,
		}

		copy(img.(*image.YCbCr).Y, unsafe.Slice((*byte)(data[0]), width*height))

		tmp := unsafe.Slice((*byte)(data[1]), width*height/2)

		for i := 0; i < len(tmp)/2; i++ {
			img.(*image.YCbCr).Cb[i] = tmp[2*i+0]
			img.(*image.YCbCr).Cr[i] = tmp[2*i+1]
		}
	case C.VIDEO_FORMAT_I420:
		img = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/4),
			Cr:             make([]byte, width*paddedHeight/4),
			SubsampleRatio: image.YCbCrSubsampleRatio420,
		}

		copy(img.(*image.YCbCr).Y, unsafe.Slice((*byte)(data[0]), width*height))
		copy(img.(*image.YCbCr).Cb, unsafe.Slice((*byte)(data[1]), width*height/4))
		copy(img.(*image.YCbCr).Cr, unsafe.Slice((*byte)(data[2]), width*height/4))
	case C.VIDEO_FORMAT_I422:
		img = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/2),
			Cr:             make([]byte, width*paddedHeight/2),
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}

		copy(img.(*image.YCbCr).Y, unsafe.Slice((*byte)(data[0]), width*height))
		copy(img.(*image.YCbCr).Cb, unsafe.Slice((*byte)(data[1]), width*height/2))
		copy(img.(*image.YCbCr).Cr, unsafe.Slice((*byte)(data[2]), width*height/2))
	case C.VIDEO_FORMAT_YVYU:
		img = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/2),
			Cr:             make([]byte, width*paddedHeight/2),
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*2)

		for i := 0; i < width*height; i++ {
			img.(*image.YCbCr).Y[i] = tmp[i*2]
		}
		for i := 0; i < width*height/2; i++ {
			img.(*image.YCbCr).Cb[i] = tmp[4*i+3]
			img.(*image.YCbCr).Cr[i] = tmp[4*i+1]
		}
	case C.VIDEO_FORMAT_YUY2:
		img = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/2),
			Cr:             make([]byte, width*paddedHeight/2),
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*2)

		for i := 0; i < width*height; i++ {
			img.(*image.YCbCr).Y[i] = tmp[i*2]
		}
		for i := 0; i < width*height/2; i++ {
			img.(*image.YCbCr).Cb[i] = tmp[4*i+1]
			img.(*image.YCbCr).Cr[i] = tmp[4*i+3]
		}
	case C.VIDEO_FORMAT_UYVY:
		img = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width / 2,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight/2),
			Cr:             make([]byte, width*paddedHeight/2),
			SubsampleRatio: image.YCbCrSubsampleRatio422,
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*2)

		for i := 0; i < width*height; i++ {
			img.(*image.YCbCr).Y[i] = tmp[i*2+1]
		}
		for i := 0; i < width*height/2; i++ {
			img.(*image.YCbCr).Cb[i] = tmp[4*i+0]
			img.(*image.YCbCr).Cr[i] = tmp[4*i+2]
		}
	case C.VIDEO_FORMAT_I444:
		img = &image.YCbCr{
			Rect:           rectangle,
			YStride:        width,
			CStride:        width,
			Y:              make([]byte, width*paddedHeight),
			Cb:             make([]byte, width*paddedHeight),
			Cr:             make([]byte, width*paddedHeight),
			SubsampleRatio: image.YCbCrSubsampleRatio444,
		}

		copy(img.(*image.YCbCr).Y, unsafe.Slice((*byte)(data[0]), width*height))
		copy(img.(*image.YCbCr).Cb, unsafe.Slice((*byte)(data[1]), width*height))
		copy(img.(*image.YCbCr).Cr, unsafe.Slice((*byte)(data[2]), width*height))
	case C.VIDEO_FORMAT_BGRX:
		img = &image.RGBA{
			Rect:   rectangle,
			Stride: width * 4,
			Pix:    make([]byte, width*paddedHeight*4),
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*4)

		for i := 0; i < len(tmp); i += 4 {
			img.(*image.RGBA).Pix[i+0] = tmp[i+2]
			img.(*image.RGBA).Pix[i+1] = tmp[i+1]
			img.(*image.RGBA).Pix[i+2] = tmp[i+0]
			img.(*image.RGBA).Pix[i+3] = 0xff
		}
	case C.VIDEO_FORMAT_BGRA:
		img = &image.RGBA{
			Rect:   rectangle,
			Stride: width * 4,
			Pix:    make([]byte, width*paddedHeight*4),
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*4)

		for i := 0; i < len(tmp); i += 4 {
			img.(*image.RGBA).Pix[i+0] = tmp[i+2]
			img.(*image.RGBA).Pix[i+1] = tmp[i+1]
			img.(*image.RGBA).Pix[i+2] = tmp[i+0]
			img.(*image.RGBA).Pix[i+3] = tmp[i+3]
		}
	case C.VIDEO_FORMAT_BGR3:
		img = &rgb.Image{
			Rect:   rectangle,
			Stride: width * 3,
			Pix:    make([]byte, width*paddedHeight*3),
		}

		tmp := unsafe.Slice((*byte)(data[0]), width*height*3)

		for i := 0; i < len(tmp); i += 3 {
			img.(*rgb.Image).Pix[i+0] = tmp[i+2]
			img.(*rgb.Image).Pix[i+1] = tmp[i+1]
			img.(*rgb.Image).Pix[i+2] = tmp[i+0]
		}
	case C.VIDEO_FORMAT_RGBA:
		img = &image.RGBA{
			Rect:   rectangle,
			Stride: width * 4,
			Pix:    make([]byte, width*paddedHeight*4),
		}

		copy(img.(*image.RGBA).Pix, unsafe.Slice((*byte)(data[0]), width*height*4))
	default:
		img = image.NewRGBA(
			rectangle,
		)
		for x := 0; x < width; x++ {
			for y := 0; y < height; y++ {
				color := color.RGBA{
					uint8(255 * x / width),
					uint8(255 * y / height),
					55,
					255}
				img.(*image.RGBA).Set(x, y, color)
			}
		}
	}

	return
}

func createAudioBuffer(info *C.struct_audio_output_info, frames *C.struct_obs_audio_data) (buf []byte) {
	var (
		bytesPerSample int
		format         C.enum_video_format
	)

	switch info.format {
	case C.AUDIO_FORMAT_U8BIT:
		fallthrough
	case C.AUDIO_FORMAT_U8BIT_PLANAR:
		bytesPerSample = 1
		format = C.AUDIO_FORMAT_U8BIT
	case C.AUDIO_FORMAT_16BIT:
		fallthrough
	case C.AUDIO_FORMAT_16BIT_PLANAR:
		bytesPerSample = 2
		format = C.AUDIO_FORMAT_16BIT
	case C.AUDIO_FORMAT_32BIT:
		fallthrough
	case C.AUDIO_FORMAT_32BIT_PLANAR:
		bytesPerSample = 4
		format = C.AUDIO_FORMAT_32BIT
	case C.AUDIO_FORMAT_FLOAT:
		fallthrough
	case C.AUDIO_FORMAT_FLOAT_PLANAR:
		bytesPerSample = 4
		format = C.AUDIO_FORMAT_FLOAT
	}

	size := bytesPerSample * int(info.speakers) * int(frames.frames)

	h := bytes.Buffer{}

	binary.Write(&h, binary.LittleEndian, &header{
		Type:      [4]byte{'W', 'A', 'V', 'E'},
		Timestamp: uint64(frames.timestamp),
		Size:      int32(size),
	})

	wave_h := bytes.Buffer{}

	binary.Write(&wave_h, binary.LittleEndian, &wave_header{
		Format:     int32(format),
		SampleRate: int32(info.samples_per_sec),
		Speakers:   int32(info.speakers),
		Frames:     int32(frames.frames),
	})

	buf = make([]byte, h.Len()+wave_h.Len()+size)

	copy(buf, append(h.Bytes(), wave_h.Bytes()...))

	wave := buf[len(buf)-size:]

	switch info.format {
	case C.AUDIO_FORMAT_32BIT_PLANAR:
		fallthrough
	case C.AUDIO_FORMAT_FLOAT_PLANAR:
		var tmp [C.MAX_AUDIO_CHANNELS][]byte

		for i := 0; i < int(info.speakers); i++ {
			tmp[i] = unsafe.Slice((*byte)(frames.data[i]), int(frames.frames)*bytesPerSample)
		}

		for i := 0; i < int(frames.frames); i++ {
			for j := 0; j < int(info.speakers); j++ {
				wave[i*int(info.speakers)*4+j*4+0] = tmp[j][i*4+0]
				wave[i*int(info.speakers)*4+j*4+1] = tmp[j][i*4+1]
				wave[i*int(info.speakers)*4+j*4+2] = tmp[j][i*4+2]
				wave[i*int(info.speakers)*4+j*4+3] = tmp[j][i*4+3]
			}
		}
	case C.AUDIO_FORMAT_16BIT_PLANAR:
		var tmp [C.MAX_AUDIO_CHANNELS][]byte

		for i := 0; i < int(info.speakers); i++ {
			tmp[i] = unsafe.Slice((*byte)(frames.data[i]), int(frames.frames)*bytesPerSample)
		}

		for i := 0; i < int(frames.frames); i++ {
			for j := 0; j < int(info.speakers); j++ {
				wave[i*int(info.speakers)*2+j*2+0] = tmp[j][i*2+0]
				wave[i*int(info.speakers)*2+j*2+1] = tmp[j][i*2+1]
			}
		}
	case C.AUDIO_FORMAT_U8BIT_PLANAR:
		var tmp [C.MAX_AUDIO_CHANNELS][]byte

		for i := 0; i < int(info.speakers); i++ {
			tmp[i] = unsafe.Slice((*byte)(frames.data[i]), int(frames.frames)*bytesPerSample)
		}

		for i := 0; i < int(frames.frames); i++ {
			for j := 0; j < int(info.speakers); j++ {
				wave[i*int(info.speakers)+j] = tmp[j][i]
			}
		}
	default:
		copy(wave, unsafe.Slice((*byte)(frames.data[0]), len(wave)))
	}

	return
}

func createJpegBuffer(img image.Image, timestamp uint64, image_header image_header, quality int) []byte {
	p := bytes.Buffer{}

	jpeg.Encode(&p, img, &jpeg.EncoderOptions{
		Quality: quality,
	})

	head := bytes.Buffer{}

	binary.Write(&head, binary.LittleEndian, &header{
		Type:      [4]byte{'J', 'P', 'E', 'G'},
		Timestamp: timestamp,
		Size:      int32(p.Len()),
	})

	binary.Write(&head, binary.LittleEndian, &image_header)

	return append(head.Bytes(), p.Bytes()...)
}

func main() {}
