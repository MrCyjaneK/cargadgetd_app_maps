package main

/*
#cgo CFLAGS: -I${SRCDIR}/include
#define CARGADGETD_APP_IMPLEMENT_PLAY
#include "cargadgetd/cg_app.h"
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"image"
	"unsafe"
)

var (
	app      = newMaps()
	mapsName = C.CString("Maps")
	iconC    *C.uint8_t
)

func ensureIconC() {
	if iconC != nil {
		return
	}
	loadIcon()
	if len(iconPix) == 0 {
		return
	}
	n := C.size_t(len(iconPix))
	iconC = (*C.uint8_t)(C.malloc(n))
	if iconC == nil {
		return
	}
	C.memcpy(unsafe.Pointer(iconC), unsafe.Pointer(&iconPix[0]), n)
}

//export cargadgetd_app_maps_icon
func cargadgetd_app_maps_icon(w, h *C.int) *C.uint8_t {
	ensureIconC()
	if w != nil {
		*w = C.int(iconW)
	}
	if h != nil {
		*h = C.int(iconH)
	}
	return iconC
}

func iconABI() (unsafe.Pointer, int, int) {
	var w, h C.int
	return unsafe.Pointer(cargadgetd_app_maps_icon(&w, &h)), int(w), int(h)
}

//export cargadgetd_app_maps_init
func cargadgetd_app_maps_init(host *C.struct_cargadgetd_host) {
	_ = host
}

//export cargadgetd_app_maps_draw
func cargadgetd_app_maps_draw(rgba *C.uint32_t, w, h, stride C.int) {
	if rgba == nil || w < 1 || h < 1 || stride < w*4 {
		return
	}
	pix := unsafe.Slice((*uint8)(unsafe.Pointer(rgba)), int(h)*int(stride))
	dst := &image.RGBA{
		Pix:    pix,
		Stride: int(stride),
		Rect:   image.Rect(0, 0, int(w), int(h)),
	}
	app.draw(dst)
}

//export cargadgetd_app_maps_touch
func cargadgetd_app_maps_touch(x, y, action C.int) {
	app.touch(int(x), int(y), int(action))
}

//export cargadgetd_app_maps_pause
func cargadgetd_app_maps_pause() { app.pause() }

//export cargadgetd_app_maps_resume
func cargadgetd_app_maps_resume() { app.resume() }

//export cargadgetd_app_maps_stop
func cargadgetd_app_maps_stop() { app.shutdown() }

//export cargadgetd_app_maps_gps_fix
func cargadgetd_app_maps_gps_fix(hasPos C.int, lat, lon, acc C.double, hasSpeed, kmh, hasCourse C.int, course C.double, err *C.char) {
	_ = err
	app.gpsFix(hasPos != 0, float64(lat), float64(lon), float64(acc), hasSpeed != 0, int(kmh), hasCourse != 0, float64(course))
}

//export cargadgetd_app_maps_name
func cargadgetd_app_maps_name() *C.char { return mapsName }

//export cargadgetd_app_maps_settings_count
func cargadgetd_app_maps_settings_count() C.int { return C.int(mapSettingsCount()) }

//export cargadgetd_app_maps_settings_label
func cargadgetd_app_maps_settings_label(i C.int) *C.char {
	return C.CString(mapSettingsLabel(int(i)))
}

//export cargadgetd_app_maps_settings_type
func cargadgetd_app_maps_settings_type(i C.int) C.int { return C.int(mapSettingsType(int(i))) }

//export cargadgetd_app_maps_settings_enum_values
func cargadgetd_app_maps_settings_enum_values(i C.int) *C.char {
	return C.CString(mapSettingsEnums(int(i)))
}

//export cargadgetd_app_maps_settings_get_bool
func cargadgetd_app_maps_settings_get_bool(i C.int) C.int { return 0 }

//export cargadgetd_app_maps_settings_get_enum
func cargadgetd_app_maps_settings_get_enum(i C.int) *C.char {
	return C.CString(mapSettingsGetEnum(int(i)))
}

//export cargadgetd_app_maps_settings_get_int
func cargadgetd_app_maps_settings_get_int(i C.int) C.int {
	return C.int(mapSettingsGetInt(int(i)))
}

//export cargadgetd_app_maps_settings_set_bool
func cargadgetd_app_maps_settings_set_bool(i, v C.int) C.int { return 1 }

//export cargadgetd_app_maps_settings_set_enum
func cargadgetd_app_maps_settings_set_enum(i C.int, v *C.char) C.int {
	return C.int(mapSettingsSetEnum(int(i), C.GoString(v)))
}

//export cargadgetd_app_maps_settings_set_int
func cargadgetd_app_maps_settings_set_int(i, v C.int) C.int {
	return C.int(mapSettingsSetInt(int(i), int(v)))
}

//export cargadgetd_app_maps_settings_error
func cargadgetd_app_maps_settings_error() *C.char {
	mapSettingsMu.Lock()
	defer mapSettingsMu.Unlock()
	if mapSettingsErr == "" {
		return nil
	}
	return C.CString(mapSettingsErr)
}

func main() {}
