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
// extern char* filter_video_get_name(uintptr_t type_data);
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
// extern void frontend_event_cb(enum obs_frontend_event event, uintptr_t data);
//
// static void blog_string(const int log_level, const char* string) {
//   blog(log_level, "[obs-teleport] %s", string);
// }
//
import "C"
import (
	"unsafe"
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
	filter_str         = C.CString("teleport-filter")
	filter_video_str   = C.CString("teleport-video-filter")
	filter_audio_str   = C.CString("teleport-audio-filter")
	frontend_str       = C.CString("Teleport")
	frontend_video_str = C.CString("Teleport (Video)")
	frontend_audio_str = C.CString("Teleport (Audio)")
	dummy_str          = C.CString("teleport-dummy")

	version = "0.0.0"
)

//export obs_module_load
func obs_module_load() C.bool {
	v := C.CString("Version " + version)
	C.blog_string(C.LOG_INFO, v)
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
	/*
		C.obs_register_source_s(&C.struct_obs_source_info{
			id:             filter_str,
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
			id:             filter_video_str,
			_type:          C.OBS_SOURCE_TYPE_FILTER,
			output_flags:   C.OBS_SOURCE_ASYNC_VIDEO | C.OBS_SOURCE_DO_NOT_DUPLICATE,
			get_name:       C.get_name_t(unsafe.Pointer(C.filter_video_get_name)),
			create:         C.source_create_t(unsafe.Pointer(C.filter_create)),
			destroy:        C.destroy_t(unsafe.Pointer(C.filter_destroy)),
			get_properties: C.get_properties_t(unsafe.Pointer(C.filter_get_properties)),
			get_defaults:   C.get_defaults_t(unsafe.Pointer(C.filter_get_defaults)),
			update:         C.update_t(unsafe.Pointer(C.filter_update)),
			filter_video:   C.filter_video_t(unsafe.Pointer(C.filter_video)),
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
	*/
	C.obs_register_output_s(&C.struct_obs_output_info{
		id:        output_str,
		flags:     C.OBS_OUTPUT_AV,
		get_name:  C.get_name_t(unsafe.Pointer(C.output_get_name)),
		create:    C.output_create_t(unsafe.Pointer(C.output_create)),
		destroy:   C.destroy_t(unsafe.Pointer(C.output_destroy)),
		start:     C.start_t(unsafe.Pointer(C.output_start)),
		stop:      C.stop_t(unsafe.Pointer(C.output_stop)),
		raw_video: C.raw_video_t(unsafe.Pointer(C.output_raw_video)),
		raw_audio: C.raw_audio_t(unsafe.Pointer(C.output_raw_audio)),
	}, C.sizeof_struct_obs_output_info)

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

func main() {}
