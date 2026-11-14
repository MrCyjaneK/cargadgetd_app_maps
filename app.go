package main

import (
	"image"
	"image/draw"
	"time"
)

// app is the maps plugin: the OSM drive map filling whatever rect the shell gives it.
type mapsPlugin struct {
	d         *drive
	chromeTap bool
}

func newMaps() *mapsPlugin {
	return &mapsPlugin{d: newDrive()}
}

func (m *mapsPlugin) draw(dst *image.RGBA) {
	if dst == nil {
		return
	}
	b := dst.Bounds()
	m.d.setSize(b.Dx(), b.Dy())
	now := time.Now()
	m.d.stepMap(now)
	draw.Draw(dst, b, image.NewUniform(colLand), image.Point{}, draw.Src)
	m.d.blitDriveMap(dst, now)
	m.d.syncChrome()
	m.d.overlayChrome(dst)
}

func (m *mapsPlugin) touch(x, y, action int) {
	// 0 up, 1 down, 2 move. A drag on the map pans. A finger on the plate,
	// the ETA, or the zoom keys stays with those widgets.
	if action == 1 {
		m.chromeTap = m.d.chromeHits(x, y)
	}
	if m.chromeTap {
		m.d.touchChrome(x, y, action)
		if action == 0 {
			m.chromeTap = false
		}
		return
	}
	switch action {
	case 1:
		m.d.mu.Lock()
		m.d.mapGrabOn = true
		m.d.mapGrabMoved = false
		m.d.mapGrabX, m.d.mapGrabY = float32(x), float32(y)
		m.d.mapGrabLastX, m.d.mapGrabLastY = float32(x), float32(y)
		m.d.mu.Unlock()
	case 2:
		m.d.panPixels(float32(x), float32(y))
	default:
		m.d.endMapDrag()
	}
}

func (m *mapsPlugin) pause() {
	m.d.stopMap()
	liveAtlas.pause()
	m.d.closeNav()
}

func (m *mapsPlugin) resume() {}

func (m *mapsPlugin) shutdown() {
	m.pause()
}

func (m *mapsPlugin) gpsFix(hasPos bool, lat, lon, acc float64, hasSpeed bool, kmh int, hasCourse bool, course float64) {
	m.d.setFix(loc{
		HasPos: hasPos, Lat: lat, Lon: lon, AccM: acc,
		HasSpeed: hasSpeed, Kmh: kmh, HasCourse: hasCourse, Course: course,
	})
}

// panPixels continues a drag in buffer pixels. The shell capture owns the gesture.
func (s *drive) panPixels(x, y float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.mapGrabOn {
		s.mapGrabOn = true
		s.mapGrabX, s.mapGrabY = x, y
	}
	fromX, fromY := s.mapGrabLastX, s.mapGrabLastY
	s.mapGrabLastX, s.mapGrabLastY = x, y
	if !s.mapGrabMoved {
		dx, dy := x-s.mapGrabX, y-s.mapGrabY
		if dx*dx+dy*dy < 64 {
			return
		}
		s.mapGrabMoved = true
		fromX, fromY = s.mapGrabX, s.mapGrabY
	}
	w, h := s.w, s.h
	if !s.cam.has || w < 1 || h < 1 {
		s.mapGrabMoved = false
		return
	}
	s.cam.pan(float64(x), float64(y), float64(x-fromX), float64(y-fromY), w, h)
	s.cam.browsing = true
	s.cam.returning = false
	s.cam.dragging = true
	s.cam.browseUntil = s.holdUntil()
}
