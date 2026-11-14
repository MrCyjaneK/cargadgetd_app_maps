// Package vector paints the OSM Scout drive map into a north-up plate.
// The frame samples that plate, so pan, heading, zoom, and tilt stay off
// the paint.
package vector

import (
	"image"
	"sync"
	"time"

	"github.com/mrcyjanek/cargadgetd_app_maps/render"
)

// Host is the Scout painter. The engine decides when to build; the host
// fetches tiles and fills the plate.
type Host interface {
	Stamp(cam render.Cam, w, h int) string
	Build(job render.Job) render.Plate
	// Retry is true when a missing plate should be requested again.
	Retry() bool
	Down() bool
	Pause()
	// PaintMarks draws the route and the position in this plate's projection.
	PaintMarks(dst *image.RGBA, cam render.Cam, marks render.Marks)
}

// Engine is the vector plate.
type Engine struct {
	host  Host
	mu    sync.Mutex
	plate render.Plate
}

func New(host Host) *Engine {
	return &Engine{host: host}
}

func (e *Engine) ID() string { return "vector" }

func (e *Engine) JobStamp(cam render.Cam, _ string, w, h int) string {
	e.mu.Lock()
	p := e.plate
	e.mu.Unlock()
	if p != nil && p.Covers(cam, w, h) {
		return ""
	}
	stamp := e.host.Stamp(cam, w, h)
	// Same quantized view, but the plate does not cover the screen.
	// One more build, then wait until the camera cell changes.
	if p != nil && p.Stamp() == stamp {
		return stamp + "|+"
	}
	if p != nil && p.Stamp() == stamp+"|+" {
		return ""
	}
	return stamp
}

func (e *Engine) Prepare(job render.Job) render.Picture {
	plate := e.host.Build(job)
	if plate == nil && e.host.Retry() {
		return render.Picture{}
	}
	return render.Picture{Plate: plate, Ready: true}
}

func (e *Engine) Publish(p render.Picture, stamp string) {
	if p.Plate == nil {
		return
	}
	p.Plate.SetStamp(stamp)
	e.mu.Lock()
	e.plate = p.Plate
	e.mu.Unlock()
}

func (e *Engine) DrawFrame(dst *image.RGBA, cam render.Cam, marks render.Marks, now time.Time) bool {
	e.mu.Lock()
	p := e.plate
	e.mu.Unlock()
	if p == nil || !p.Ready() {
		return false
	}
	p.Sample(dst, cam, now)
	e.host.PaintMarks(dst, cam, marks)
	return true
}

func (e *Engine) Down() bool { return e.host.Down() }

func (e *Engine) Pause() {
	e.host.Pause()
	e.mu.Lock()
	e.plate = nil
	e.mu.Unlock()
}
