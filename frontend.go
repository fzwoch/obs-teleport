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
import "C"
import (
	"runtime/cgo"
)

var (
	obs_teleport_enable = C.CString("teleport-enable")
	obs_teleport_str    = C.CString("Teleport Enabled")
)

//export frontend_cb
func frontend_cb(data C.uintptr_t) {
	C.obs_frontend_open_source_properties(dummy)
}

//export frontend_event_cb
func frontend_event_cb(event C.enum_obs_frontend_event, data C.uintptr_t) {
	switch event {
	case C.OBS_FRONTEND_EVENT_SCRIPTING_SHUTDOWN:
		if C.obs_output_active(output) {
			C.obs_output_stop(output)
		}
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

	C.obs_properties_add_bool(properties, obs_teleport_enable, obs_teleport_str)

	prop := C.obs_properties_add_text(properties, C.CString("identifier"), C.CString("Identifier"), C.OBS_TEXT_DEFAULT)
	C.obs_property_set_long_description(prop, C.CString("Name of the stream. Uses hostname if blank."))

	return properties
}

//export dummy_get_defaults
func dummy_get_defaults(settings *C.obs_data_t) {
	C.obs_data_set_default_bool(settings, obs_teleport_enable, false)
	C.obs_data_set_default_string(settings, C.CString("identifier"), C.CString(""))
}

//export dummy_update
func dummy_update(data C.uintptr_t, settings *C.obs_data_t) {
	enable := C.obs_data_get_bool(settings, obs_teleport_enable)

	if enable {
		C.obs_output_start(output)
	} else {
		C.obs_output_stop(output)
	}
}
