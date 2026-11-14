package main

import (
	"image"
	"image/color"
	"math"

	"github.com/mrcyjanek/cargadgetd_app_maps/nav"
	"github.com/mrcyjanek/cargadgetd_app_maps/render"
)

// colRoute is the path stroked over the map. It stays off the road greens.
var colRoute = color.NRGBA{R: 70, G: 170, B: 255, A: 255}

// frameMarks is the route and the car the backend draws in its own camera.
func (s *drive) frameMarks() render.Marks {
	var pts []nav.Point
	if src := s.navSource(); src != nil {
		pts = src.Route()
	}
	s.mu.Lock()
	cam := s.cam
	s.mu.Unlock()
	marks := render.Marks{
		Route:  make([]render.LatLng, len(pts)),
		Car:    render.LatLng{Lat: cam.carLat, Lon: cam.carLon},
		HasCar: cam.hasCar,
		Follow: cam.has && !cam.browsing,
	}
	for i, p := range pts {
		marks.Route[i] = render.LatLng{Lat: p.Lat, Lon: p.Lon}
	}
	return marks
}

// paintRouteLine projects the path and draws the part that meets the view.
// Points closer than two screen pixels are skipped. A point past the
// horizon breaks the stroke.
func paintRouteLine(dst *image.RGBA, cam mapCam, pts []nav.Point) {
	if dst == nil || !cam.has || len(pts) < 2 {
		return
	}
	b := dst.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 2 || h < 2 {
		return
	}
	minLat, maxLat, minLon, maxLon, win := routeWindow(cam, w, h)
	inside := func(p nav.Point) bool {
		if !win {
			return true
		}
		return p.Lat >= minLat && p.Lat <= maxLat && p.Lon >= minLon && p.Lon <= maxLon
	}
	var curX, curY float64
	var have bool
	drawPt := func(p nav.Point, force bool) {
		sx, sy, ok := cam.project(p.Lat, p.Lon, w, h)
		if !ok {
			have = false
			return
		}
		sx += float64(b.Min.X)
		sy += float64(b.Min.Y)
		if !have {
			curX, curY = sx, sy
			have = true
			return
		}
		dx, dy := sx-curX, sy-curY
		if !force && dx*dx+dy*dy < 4 {
			return
		}
		strokeSegPx(dst, curX, curY, sx, sy, 5, colRoute)
		curX, curY = sx, sy
	}
	var prev nav.Point
	var prevIn, started bool
	for i, p := range pts {
		in := inside(p)
		if !started {
			prev, prevIn, started = p, in, true
			if in {
				drawPt(p, false)
			}
			continue
		}
		if in || prevIn {
			if !prevIn {
				drawPt(prev, true)
			}
			drawPt(p, i == len(pts)-1)
		} else {
			have = false
		}
		prev, prevIn = p, in
	}
}

// routeWindow is the ground rectangle the screen can see, padded so a
// segment that enters the view is not dropped.
func routeWindow(cam mapCam, w, h int) (minLat, maxLat, minLon, maxLon float64, ok bool) {
	z := cam.sampleZoom()
	first := true
	for _, sy := range []int{0, h / 2, h - 1} {
		for _, sx := range []int{0, w / 2, w - 1} {
			wx, wy := cam.screenWorld(sx, sy, w, h, z)
			lat, lon := worldToLatLon(wx, wy, z)
			if math.IsNaN(lat) || math.IsNaN(lon) {
				continue
			}
			if first {
				minLat, maxLat, minLon, maxLon = lat, lat, lon, lon
				first = false
				continue
			}
			if lat < minLat {
				minLat = lat
			}
			if lat > maxLat {
				maxLat = lat
			}
			if lon < minLon {
				minLon = lon
			}
			if lon > maxLon {
				maxLon = lon
			}
		}
	}
	if first {
		return 0, 0, 0, 0, false
	}
	const pad = 0.05
	return minLat - pad, maxLat + pad, minLon - pad, maxLon + pad, true
}
