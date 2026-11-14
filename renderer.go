package main

import (
	"image"
	"time"

	"github.com/mrcyjanek/cargadgetd_app_maps/maplibre"
	"github.com/mrcyjanek/cargadgetd_app_maps/nav"
	"github.com/mrcyjanek/cargadgetd_app_maps/render"
	"github.com/mrcyjanek/cargadgetd_app_maps/vector"
)

// mapEngine is the drive picture. vector publishes a plate; maplibre
// publishes a still, which is empty until Render returns one.
type mapEngine = render.Engine

const (
	rendererVector   = "vector"
	rendererMapLibre = "maplibre"
)

func newMapEngine(kind string) mapEngine {
	switch normalizeRenderer(kind) {
	case rendererMapLibre:
		return maplibre.New()
	default:
		return vector.New(plateHost{})
	}
}

func renderCam(c mapCam) render.Cam {
	return render.Cam{
		Lat: c.lat, Lon: c.lon, Heading: c.heading,
		Zoom: c.zoom, Pitch: c.pitch, Has: c.has,
	}
}

func mapCamOf(c render.Cam) mapCam {
	return mapCam{
		lat: c.Lat, lon: c.Lon, heading: c.Heading,
		zoom: c.Zoom, pitch: c.Pitch, has: c.Has,
	}
}

func renderJob(job mapJob) render.Job {
	return render.Job{
		Cam: renderCam(job.cam), W: job.w, H: job.h,
		Now: job.now, Stamp: job.stamp, Style: job.style,
	}
}

type plateHost struct{}

func (plateHost) Stamp(cam render.Cam, w, h int) string {
	return plateStamp(mapCamOf(cam), w, h)
}

func (plateHost) Build(job render.Job) render.Plate {
	p := buildDrivePlate(mapJob{
		cam: mapCamOf(job.Cam), w: job.W, h: job.H,
		now: job.Now, stamp: job.Stamp,
	})
	if p == nil {
		return nil
	}
	return p
}

func (plateHost) Retry() bool { return enableMapNet }

func (plateHost) Down() bool { return liveAtlas.isDown() }

func (plateHost) Pause() { liveAtlas.pause() }

func (plateHost) PaintMarks(dst *image.RGBA, cam render.Cam, marks render.Marks) {
	mc := mapCamOf(cam)
	mc.browsing = !marks.Follow
	mc.hasCar = marks.HasCar
	mc.carLat = marks.Car.Lat
	mc.carLon = marks.Car.Lon
	pts := make([]nav.Point, len(marks.Route))
	for i, p := range marks.Route {
		pts[i] = nav.Point{Lat: p.Lat, Lon: p.Lon}
	}
	paintRouteLine(dst, mc, pts)
	paintCarArrow(dst, mc, dst.Bounds())
}

func (p *mapPlate) Covers(cam render.Cam, w, h int) bool {
	if p == nil {
		return false
	}
	return p.covers(mapCamOf(cam), w, h)
}

func (p *mapPlate) Stamp() string {
	if p == nil {
		return ""
	}
	return p.stamp
}

func (p *mapPlate) SetStamp(s string) {
	if p != nil {
		p.stamp = s
	}
}

func (p *mapPlate) Ready() bool { return p != nil && p.img != nil }

func (p *mapPlate) Sample(dst *image.RGBA, cam render.Cam, now time.Time) {
	samplePlate(dst, p, mapCamOf(cam), now)
}
