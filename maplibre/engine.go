// Package maplibre draws one pitched still. Without the maplibrenative
// build tag, Render is finished and empty, so the loading field stays and
// the painter does not spin. A real still already has pitch and heading.
// The route and the position are projected with that still's camera, not
// the vector one.
package maplibre

import (
	"fmt"
	"image"
	"image/draw"
	"log"
	"math"
	"sync"
	"time"

	"github.com/mrcyjanek/cargadgetd_app_maps/render"
)

// Still is one Render result. Ready is false while the style or tiles are
// still loading and the drive should ask again. Ready with a nil image means
// this renderer cannot draw: the loading field stays and the painter stops.
// Ready with an image is the view, pitch and heading already applied.
// ZoomMiss means Scout had no tiles at the asked zoom. DrawnZoom is the
// closest zoom that did load, and the image is that view.
type Still struct {
	Img       *image.RGBA
	Ready     bool
	ZoomMiss  bool
	DrawnZoom float64
}

// Render draws one still of job. The default has no native renderer, so it
// is finished with nothing to show. A build with the maplibrenative tag
// replaces this.
var Render = func(render.Job) Still {
	return Still{Ready: true}
}

// Engine publishes that still. Heading and pitch are part of the image,
// so a change in either asks for another one.
type Engine struct {
	mu     sync.Mutex
	img    *image.RGBA
	cam    render.Cam
	style  string
	w, h   int
	have   bool
	logged bool
}

// releaseSession drops the native map and its EGL context. The tagged
// build sets it; the stub leaves it empty.
var releaseSession = func() {}

// projectStill maps ground points through the camera of the last still.
// The stub has no camera, so it declines. The tagged build uses MapLibre's
// projection of the frame it just drew.
var projectStill = func([]render.LatLng) (xs, ys []float64, ok bool) {
	return nil, nil, false
}

func New() *Engine { return &Engine{} }

func (e *Engine) ID() string { return "maplibre" }

func (e *Engine) JobStamp(cam render.Cam, style string, w, h int) string {
	style = styleName(style)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.have && e.style == style && e.w == w && e.h == h && fresh(e.cam, cam) {
		return ""
	}
	return fmt.Sprintf("ml|%s|%.5f|%.5f|%.2f|%.1f|%.2f|%dx%d",
		style, cam.Lat, cam.Lon, cam.Zoom, cam.Heading, cam.Pitch, w, h)
}

func (e *Engine) Prepare(job render.Job) render.Picture {
	still := Render(job)
	if !still.Ready {
		return render.Picture{}
	}
	if still.Img == nil {
		e.logGaveUp()
	}
	return render.Picture{
		Img: still.Img, Cam: job.Cam, W: job.W, H: job.H, Ready: true,
		Style: styleName(job.Style), ZoomMiss: still.ZoomMiss, DrawnZoom: still.DrawnZoom,
	}
}

func (e *Engine) logGaveUp() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.logged {
		return
	}
	e.logged = true
	log.Printf("maps: maplibre still renderer has no image")
}

func (e *Engine) Publish(p render.Picture, _ string) {
	e.mu.Lock()
	e.have = true
	e.cam = p.Cam
	e.style = styleName(p.Style)
	e.w, e.h = p.W, p.H
	if p.Img != nil {
		e.img = p.Img
	}
	e.mu.Unlock()
}

func (e *Engine) DrawFrame(dst *image.RGBA, _ render.Cam, marks render.Marks, _ time.Time) bool {
	e.mu.Lock()
	img := e.img
	e.mu.Unlock()
	if img == nil || dst == nil {
		return false
	}
	if img.Bounds().Dx() != dst.Bounds().Dx() || img.Bounds().Dy() != dst.Bounds().Dy() {
		return false
	}
	draw.Draw(dst, dst.Bounds(), img, img.Bounds().Min, draw.Src)
	paintStillMarks(dst, marks)
	return true
}

func (e *Engine) Down() bool { return false }

func (e *Engine) Pause() {
	releaseSession()
	e.mu.Lock()
	e.img = nil
	e.have = false
	e.style = ""
	e.mu.Unlock()
}

func styleName(s string) string {
	if s == "" {
		return "osmbright"
	}
	return s
}

func fresh(have, cam render.Cam) bool {
	if math.Abs(angDelta(have.Heading, cam.Heading)) > 1 {
		return false
	}
	if math.Abs(have.Pitch-cam.Pitch) > 0.05 || math.Abs(have.Zoom-cam.Zoom) > 0.08 {
		return false
	}
	return meters(have.Lat, have.Lon, cam.Lat, cam.Lon) < 8
}

func angDelta(a, b float64) float64 {
	d := math.Mod(b-a, 360)
	if d > 180 {
		d -= 360
	} else if d < -180 {
		d += 360
	}
	return d
}

func meters(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	p1 := lat1 * math.Pi / 180
	p2 := lat2 * math.Pi / 180
	dp := (lat2 - lat1) * math.Pi / 180
	dl := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
