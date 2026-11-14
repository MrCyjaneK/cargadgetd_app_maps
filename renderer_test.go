package main

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/mrcyjanek/cargadgetd_app_maps/maplibre"
	"github.com/mrcyjanek/cargadgetd_app_maps/render"
)

func TestMapLibreSettingShowsNothing(t *testing.T) {
	prevAtlas := liveAtlas
	liveAtlas = newAtlas()
	t.Cleanup(func() { liveAtlas = prevAtlas })

	sh := newDrive()
	sh.engine = newMapEngine(rendererMapLibre)
	defer sh.stopMap()
	sh.mu.Lock()
	sh.mapKind = mapOSM
	sh.w, sh.h = 80, 60
	sh.cam = mapCam{lat: 50.25, lon: 19.04, zoom: 15, heading: 20, pitch: 0.4, has: true}
	sh.mu.Unlock()

	now := time.Unix(0, 0)
	sh.stepMap(now)
	dst := image.NewRGBA(image.Rect(0, 0, 80, 60))
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		sh.blitDriveMap(dst, now)
		sh.mu.Lock()
		done := sh.mapStamp != ""
		sh.mu.Unlock()
		if done {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	sh.mu.Lock()
	done := sh.mapStamp != ""
	sh.mu.Unlock()
	if !done {
		t.Fatal("empty still kept the painter spinning")
	}
	dark := 0
	for y := 0; y < 60; y++ {
		for x := 0; x < 80; x++ {
			c := dst.RGBAAt(x, y)
			if c.R == colGlitchBG.R && c.G == colGlitchBG.G && c.B == colGlitchBG.B {
				dark++
			}
		}
	}
	if dark < 80*60/2 {
		t.Fatalf("loading field %d", dark)
	}
}

func TestMapLibreNotReadyKeepsAsking(t *testing.T) {
	prev := maplibre.Render
	var calls int
	maplibre.Render = func(render.Job) maplibre.Still {
		calls++
		return maplibre.Still{}
	}
	t.Cleanup(func() { maplibre.Render = prev })

	sh := newDrive()
	sh.engine = newMapEngine(rendererMapLibre)
	defer sh.stopMap()
	sh.mu.Lock()
	sh.mapKind = mapOSM
	sh.w, sh.h = 80, 60
	sh.cam = mapCam{lat: 50.25, lon: 19.04, zoom: 15, has: true}
	sh.mu.Unlock()
	sh.stepMap(time.Unix(0, 0))
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) && calls < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	sh.mu.Lock()
	stamp := sh.mapStamp
	sh.mu.Unlock()
	if stamp != "" {
		t.Fatal("unfinished still was published")
	}
	if calls < 2 {
		t.Fatalf("retries %d", calls)
	}
}

func TestMapLibreImageIsCopied(t *testing.T) {
	prev := maplibre.Render
	ink := color.RGBA{R: 10, G: 20, B: 200, A: 255}
	var calls int
	maplibre.Render = func(job render.Job) maplibre.Still {
		calls++
		img := image.NewRGBA(image.Rect(0, 0, job.W, job.H))
		img.SetRGBA(4, 4, ink)
		return maplibre.Still{Img: img, Ready: true}
	}
	t.Cleanup(func() { maplibre.Render = prev })

	sh := newDrive()
	sh.engine = newMapEngine(rendererMapLibre)
	defer sh.stopMap()
	sh.mu.Lock()
	sh.mapKind = mapOSM
	sh.w, sh.h = 80, 60
	sh.cam = mapCam{lat: 50.25, lon: 19.04, zoom: 15, heading: 20, pitch: 0.4, has: true}
	sh.mu.Unlock()

	now := time.Unix(0, 0)
	sh.stepMap(now)
	dst := image.NewRGBA(image.Rect(0, 0, 80, 60))
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		sh.blitDriveMap(dst, now)
		c := dst.RGBAAt(4, 4)
		if c.R == ink.R && c.G == ink.G && c.B == ink.B {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	c := dst.RGBAAt(4, 4)
	if c.R != ink.R || c.G != ink.G || c.B != ink.B {
		t.Fatalf("still %+v calls %d", c, calls)
	}
	if calls != 1 {
		t.Fatalf("renders %d", calls)
	}
	sh.mu.Lock()
	sh.cam.heading += 0.2
	sh.mu.Unlock()
	sh.stepMap(now.Add(time.Second))
	time.Sleep(20 * time.Millisecond)
	if calls != 1 {
		t.Fatalf("a small heading change rebuilt the still (%d)", calls)
	}
}

func TestRendererDefaultsToVector(t *testing.T) {
	if normalizeRenderer("") != rendererVector || normalizeRenderer("nope") != rendererVector {
		t.Fatal("unknown renderer")
	}
	if normalizeRenderer("maplibre") != rendererMapLibre {
		t.Fatal("maplibre")
	}
	if newMapEngine("vector").ID() != rendererVector {
		t.Fatal("vector engine")
	}
	if newMapEngine("libre").ID() != rendererMapLibre {
		t.Fatal("maplibre engine")
	}
}
