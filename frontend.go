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
// #include <util/platform.h>
//
// extern void frontend_cb(uintptr_t data);
//
import "C"
import (
	"math"
	"runtime/cgo"
	"unsafe"
)

var (
	teleport_enabled_str          = C.CString("teleport-enabled")
	teleport_enabled_readable_str = C.CString("Teleport Enabled")
	identifier_str                = C.CString("identifier")
	identifier_readable_str       = C.CString("Identifier")
	identifier_description_str    = C.CString("Name of the stream. Uses hostname if blank.")
	port_str                      = C.CString("port")
	port_readable_str             = C.CString("Port")
	port_description_str          = C.CString("0 means 'Auto'")
	quality_str                   = C.CString("quality")
	quality_readable_str          = C.CString("Quality")
	apply_str                     = C.CString("Apply")
	empty_str                     = C.CString("")
	config_str                    = C.CString("obs-teleport.json")

	output *C.obs_output_t
	dummy  *C.obs_source_t
)

//export frontend_cb
func frontend_cb(data C.uintptr_t) {
	C.obs_frontend_open_source_properties(dummy)
}

//export frontend_event_cb
func frontend_event_cb(event C.enum_obs_frontend_event, data C.uintptr_t) {
	switch event {
	case C.OBS_FRONTEND_EVENT_FINISHED_LOADING:
		C.obs_frontend_add_tools_menu_item(frontend_str, C.obs_frontend_cb(unsafe.Pointer(C.frontend_cb)), nil)

		output = C.obs_output_create(output_str, frontend_str, nil, nil)
		dummy = C.obs_source_create(dummy_str, frontend_str, nil, nil)

		config := C.obs_module_get_config_path(C.obs_current_module(), nil)

		C.os_mkdirs(config)
		C.bfree(unsafe.Pointer(config))

		config = C.obs_module_get_config_path(C.obs_current_module(), config_str)

		settings := C.obs_data_create_from_json_file(config)
		C.obs_source_update(dummy, settings)
		C.obs_data_release(settings)

		C.bfree(unsafe.Pointer(config))
	case C.OBS_FRONTEND_EVENT_EXIT:
		if C.obs_output_active(output) {
			C.obs_output_stop(output)
		}

		config := C.obs_module_get_config_path(C.obs_current_module(), config_str)

		settings := C.obs_source_get_settings(dummy)
		C.obs_data_save_json(settings, config)
		C.obs_data_release(settings)

		C.bfree(unsafe.Pointer(config))

		C.obs_output_release(output)
		C.obs_source_release(dummy)
	}
}

//export dummy_get_name
func dummy_get_name(type_data C.uintptr_t) *C.char {
	return nil
}

//export dummy_create
func dummy_create(settings *C.obs_data_t, source *C.obs_source_t) C.uintptr_t {
	h := &struct{}{}

	return C.uintptr_t(cgo.NewHandle(h))
}

//export dummy_destroy
func dummy_destroy(data C.uintptr_t) {
	cgo.Handle(data).Delete()
}

//export dummy_get_properties
func dummy_get_properties(data C.uintptr_t) *C.obs_properties_t {
	properties := C.obs_properties_create()

	C.obs_properties_set_flags(properties, C.OBS_PROPERTIES_DEFER_UPDATE)

	C.obs_properties_add_bool(properties, teleport_enabled_str, teleport_enabled_readable_str)

	prop := C.obs_properties_add_text(properties, identifier_str, identifier_readable_str, C.OBS_TEXT_DEFAULT)
	C.obs_property_set_long_description(prop, identifier_description_str)

	prop = C.obs_properties_add_int(properties, port_str, port_readable_str, 0, math.MaxUint16, 1)
	C.obs_property_set_long_description(prop, port_description_str)

	C.obs_properties_add_int_slider(properties, quality_str, quality_readable_str, 0, 100, 1)

	return properties
}

//export dummy_get_defaults
func dummy_get_defaults(settings *C.obs_data_t) {
	C.obs_data_set_default_bool(settings, teleport_enabled_str, false)
	C.obs_data_set_default_string(settings, identifier_str, empty_str)
	C.obs_data_set_default_int(settings, port_str, 0)
	C.obs_data_set_default_int(settings, quality_str, 90)
}

//export dummy_update
func dummy_update(data C.uintptr_t, settings *C.obs_data_t) {
	if !C.obs_output_can_begin_data_capture(output, 0) {
		C.obs_output_stop(output)
	}

	C.obs_output_release(output)
	output = C.obs_output_create(output_str, frontend_str, nil, nil)

	if C.obs_data_get_bool(settings, teleport_enabled_str) {
		C.obs_output_start(output)
	}
}
