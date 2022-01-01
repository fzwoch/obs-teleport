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
// #include <stdlib.h>
//
// typedef char* (*get_name_t)(uintptr_t type_data);
// extern char* source_get_name(uintptr_t type_data);
// extern char* output_get_name(uintptr_t type_data);
// extern char* dummy_get_name(uintptr_t type_data);
//
// typedef uintptr_t (*source_create_t)(obs_data_t *settings, obs_source_t *source);
// extern uintptr_t source_create(obs_data_t *settings, obs_source_t *source);
// extern uintptr_t dummy_create(obs_data_t *settings, obs_source_t *source);
//
// typedef uintptr_t (*output_create_t)(obs_data_t *settings, obs_output_t *output);
// extern uintptr_t output_create(obs_data_t *settings, obs_output_t *output);
//
// typedef void (*destroy_t)(uintptr_t data);
// extern void source_destroy(uintptr_t data);
// extern void output_destroy(uintptr_t data);
// extern void dummy_destroy(uintptr_t data);
//
// typedef obs_properties_t* (*get_properties_t)(uintptr_t data);
// extern obs_properties_t* source_get_properties(uintptr_t data);
// extern obs_properties_t* dummy_get_properties(uintptr_t data);
//
// typedef void (*get_defaults_t)(obs_data_t *settings);
// extern void source_get_defaults(obs_data_t *settings);
// extern void dummy_get_defaults(obs_data_t *settings);
//
// typedef void (*update_t)(uintptr_t data, obs_data_t *settings);
// extern void (source_update)(uintptr_t data, obs_data_t *settings);
// extern void (dummy_update)(uintptr_t data, obs_data_t *settings);
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
// extern void frontend_cb(uintptr_t data);
//
import "C"
import (
	"os"
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
	source_str   = C.CString("teleport-source")
	output_str   = C.CString("teleport-output")
	frontend_str = C.CString("Teleport")
	dummy_str    = C.CString("teleport-dummy")

	output *C.obs_output_t
	dummy  *C.obs_source_t
)

//export obs_module_load
func obs_module_load() C.bool {
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
	}, C.sizeof_struct_obs_source_info)

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

	C.obs_frontend_add_tools_menu_item(frontend_str, C.obs_frontend_cb(unsafe.Pointer(C.frontend_cb)), nil)

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

	output = C.obs_output_create(output_str, frontend_str, nil, nil)
	dummy = C.obs_source_create(dummy_str, frontend_str, nil, nil)

	dir, _ := os.UserConfigDir()

	settings := C.obs_data_create_from_json_file(C.CString(dir + string(os.PathSeparator) + "obs-teleport.json"))
	C.obs_source_update(dummy, settings)
	C.obs_data_release(settings)

	return true
}

//export obs_module_unload
func obs_module_unload() {
	dir, _ := os.UserConfigDir()

	settings := C.obs_source_get_settings(dummy)
	C.obs_data_save_json(settings, C.CString(dir+string(os.PathSeparator)+"obs-teleport.json"))
	C.obs_data_release(settings)

	C.obs_output_release(output)
	C.obs_source_release(dummy)
}

type options_header struct {
	Magic [4]byte
	Size  int32
}

type options struct {
	Quality int `json:"quality"`
}

type header struct {
	Type      [4]byte
	Timestamp int64
	Size      int32
}

func main() {}
