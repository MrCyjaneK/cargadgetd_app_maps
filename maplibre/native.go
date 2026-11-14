//go:build maplibrenative

package maplibre

/*
#cgo CFLAGS: -std=c2x
#cgo pkg-config: maplibre-native-c
#include <stdlib.h>
#include "still.h"
*/
import "C"
import (
	"image"
	"unsafe"

	"github.com/mrcyjanek/cargadgetd_app_maps/render"
)

func init() {
	Render = renderNative
	releaseSession = func() { C.cg_ml_pause() }
	projectStill = projectNative
}

func projectNative(pts []render.LatLng) ([]float64, []float64, bool) {
	n := len(pts)
	if n == 0 {
		return nil, nil, false
	}
	lat := make([]C.double, n)
	lon := make([]C.double, n)
	x := make([]C.double, n)
	y := make([]C.double, n)
	for i, p := range pts {
		lat[i] = C.double(p.Lat)
		lon[i] = C.double(p.Lon)
	}
	if C.cg_ml_project(&lat[0], &lon[0], C.int(n), &x[0], &y[0]) != 0 {
		return nil, nil, false
	}
	xs := make([]float64, n)
	ys := make([]float64, n)
	for i := 0; i < n; i++ {
		xs[i] = float64(x[i])
		ys[i] = float64(y[i])
	}
	return xs, ys, true
}

// renderNative asks the process-wide map for one still. Pitch uses the same
// 50 degree full tilt as the vector camera. A slow style load returns not
// ready so the drive asks again.
func renderNative(job render.Job) Still {
	if job.W <= 0 || job.H <= 0 {
		return Still{Ready: true}
	}
	cjob := C.cg_ml_job{
		lat:     C.double(job.Cam.Lat),
		lon:     C.double(job.Cam.Lon),
		zoom:    C.double(job.Cam.Zoom),
		bearing: C.double(job.Cam.Heading),
		pitch:   C.double(job.Cam.Pitch * 50),
		w:       C.int(job.W),
		h:       C.int(job.H),
	}
	putCStyle(&cjob, job.Style)
	var rgba *C.uint8_t
	var w, h C.int
	switch C.cg_ml_render(&cjob, &rgba, &w, &h) {
	case 0:
		if rgba == nil {
			return Still{}
		}
		defer C.free(unsafe.Pointer(rgba))
		img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
		n := int(w) * int(h) * 4
		copy(img.Pix, C.GoBytes(unsafe.Pointer(rgba), C.int(n)))
		drawn := float64(C.cg_ml_drawn_zoom())
		still := Still{Img: img, Ready: true}
		if job.Cam.Zoom-drawn > 0.05 {
			still.ZoomMiss = true
			still.DrawnZoom = drawn
		}
		return still
	case 2:
		return Still{Ready: true}
	default:
		return Still{}
	}
}

func putCStyle(cjob *C.cg_ml_job, style string) {
	if style == "" {
		style = "osmbright"
	}
	b := []byte(style)
	if len(b) >= len(cjob.style) {
		b = b[:len(cjob.style)-1]
	}
	for i := range b {
		cjob.style[i] = C.char(b[i])
	}
}
