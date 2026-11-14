package main

import (
	"image"
	"os"

	"github.com/mrcyjanek/cargadgetd_app_maps/nav"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// openNav is the navigator factory. Tests replace it.
var openNav = nav.Open

// navCopy is the left-hand lines and the maneuver distance.
// A narrative that repeats the street is dropped. "-" is blank.
func navCopy(t nav.Trip) (left []string, dist string) {
	street := nav.Plain(t.Street)
	line := nav.Plain(t.Narrative)
	if street != "" && street == line {
		line = ""
	}
	if street != "" {
		left = append(left, street)
	}
	if line != "" {
		left = append(left, line)
	}
	return left, nav.Plain(t.ManDist)
}

func (s *drive) navSource() nav.Source {
	s.mu.Lock()
	src := s.navSrc
	kind := s.navKind
	s.mu.Unlock()
	if src != nil {
		return src
	}
	src = openNav(kind)
	s.mu.Lock()
	if s.navSrc == nil {
		s.navSrc = src
		s.mu.Unlock()
		return src
	}
	extra := src
	src = s.navSrc
	s.mu.Unlock()
	extra.Close()
	return src
}

func (s *drive) navSnapshot() nav.Trip {
	src := s.navSource()
	if src == nil {
		return nav.Trip{}
	}
	return src.Snapshot()
}

func (s *drive) closeNav() {
	s.mu.Lock()
	src := s.navSrc
	s.navSrc = nil
	s.mu.Unlock()
	if src != nil {
		src.Close()
	}
}

func (s *drive) noteNavErr(trip nav.Trip) {
	s.mu.Lock()
	prev := s.navErr
	if trip.Err == "" {
		s.navErr = ""
		s.mu.Unlock()
		return
	}
	if trip.Err == prev {
		s.mu.Unlock()
		return
	}
	s.navErr = trip.Err
	s.mu.Unlock()
	s.logf("nav: %s", trip.Err)
}

func rasterSVG(path string, size int) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	icon, err := oksvg.ReadIconStream(f)
	if err != nil {
		return nil, err
	}
	icon.SetTarget(0, 0, float64(size), float64(size))
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	scanner := rasterx.NewScannerGV(size, size, dst, dst.Bounds())
	icon.Draw(rasterx.NewDasher(size, size, scanner), 1)
	return dst, nil
}
