package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/mvt"
	"github.com/paulmach/orb/geojson"
)

func TestFixtureRoadPaintsLime(t *testing.T) {
	fc := geojson.NewFeatureCollection()
	f := geojson.NewFeature(orb.LineString{{0, 2048}, {4096, 2048}})
	f.Properties["class"] = "motorway"
	fc.Append(f)
	raw, err := mvt.Marshal(mvt.Layers{mvt.NewLayer("transportation", fc)})
	if err != nil {
		t.Fatal(err)
	}
	g, err := decodeMVT(raw)
	if err != nil {
		t.Fatal(err)
	}
	img := paintTile(g)
	c := img.RGBAAt(128, 128)
	if c.R != colRoadMajor.R || c.G != colRoadMajor.G || c.B != colRoadMajor.B {
		t.Fatalf("pixel %+v", c)
	}
}

func TestGlitchIsMostlyDarkScanLines(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 240, 160))
	paintGlitch(dst, time.Unix(0, 0))
	var dark, red, green, blue, orange, partial int
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		var ink, ground int
		for x := b.Min.X; x < b.Max.X; x++ {
			c := dst.RGBAAt(x, y)
			switch {
			case c.R < 40 && c.G < 50 && c.B < 70:
				dark++
				ground++
			case c.R > 200 && c.G < 80 && c.B < 100:
				red++
				ink++
			case c.G > 180 && c.R < 120:
				green++
				ink++
			case c.B > 180 && c.R < 80:
				blue++
				ink++
			case c.R > 200 && c.G > 80 && c.G < 160 && c.B < 80:
				orange++
				ink++
			default:
				if glitchBright(c) {
					ink++
				} else {
					ground++
				}
			}
		}
		if ink > 0 && ground > 0 {
			partial++
		}
	}
	n := b.Dx() * b.Dy()
	if dark*100/n < 70 || red == 0 || green == 0 || blue == 0 || orange == 0 || partial < 8 {
		t.Fatalf("dark %d red %d green %d blue %d orange %d partial %d of %d", dark, red, green, blue, orange, partial, n)
	}
}

func glitchBright(c color.RGBA) bool {
	for _, p := range glitchPalette {
		if c.R == p.R && c.G == p.G && c.B == p.B {
			return true
		}
	}
	return false
}

func TestLoadGlitchIsQuieterThanTheDownField(t *testing.T) {
	full := image.NewRGBA(image.Rect(0, 0, 240, 160))
	load := image.NewRGBA(image.Rect(0, 0, 240, 160))
	now := time.Unix(0, 0)
	paintGlitchField(full, now)
	paintLoadGlitch(load, now)
	bright := func(img *image.RGBA) int {
		n := 0
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if glitchBright(img.RGBAAt(x, y)) {
					n++
				}
			}
		}
		return n
	}
	fb, lb := bright(full), bright(load)
	if lb == 0 || lb*4 > fb {
		t.Fatalf("load streaks %d full %d", lb, fb)
	}
	if glitchTick(now.Add(80*time.Millisecond)) == glitchTick(now) {
		t.Fatal("full field did not step")
	}
	if glitchTickAt(now.Add(80*time.Millisecond), 0.4) != glitchTickAt(now, 0.4) {
		t.Fatal("load field stepped at the full rate")
	}
	if glitchTickAt(now.Add(200*time.Millisecond), 0.4) == glitchTickAt(now, 0.4) {
		t.Fatal("load field never steps")
	}
	waiting := image.NewRGBA(image.Rect(0, 0, 80, 60))
	paintDriveMap(waiting, mapOSM, mapCam{}, nil, false, now)
	if bright(waiting) == 0 {
		t.Fatal("loading map has no glitch")
	}
}

func TestArrowSitsBelowCenter(t *testing.T) {
	cam := mapCam{lat: 50.25, lon: 19.04, zoom: 16, has: true, heading: 10}
	h := 60
	img := renderMap(mapOSM, cam, 80, h, time.Unix(0, 0))
	cy := int(float64(h) * camAnchor)
	if cy <= h/2 {
		t.Fatalf("anchor y %d is not below the center of %d", cy, h)
	}
	c := img.RGBAAt(40, cy-8)
	if c.R != colLime.R || c.G != colLime.G || c.B != colLime.B {
		t.Fatalf("arrow %+v at y %d", c, cy-8)
	}
}

func TestVectorPaintKeepsFineBuilding(t *testing.T) {
	// Eight units of a 4096 tile is half a pixel in the old 256 bitmap.
	const z, x, y = 14, 9058, 5538
	half := float32(8) / 4096
	g := mapGeom{}
	g.addPoly(kindBuilding, [][]pt2{{{
		0.5 - half, 0.5 - half,
	}, {
		0.5 + half, 0.5 - half,
	}, {
		0.5 + half, 0.5 + half,
	}, {
		0.5 - half, 0.5 + half,
	}, {
		0.5 - half, 0.5 - half,
	}}})
	coarse := paintTile(g)
	c := coarse.RGBAAt(128, 128)
	if c.R == colBldg.R && c.G == colBldg.G && c.B == colBldg.B {
		t.Fatal("256 bitmap already kept the building")
	}
	lat, lon := latLonAtTile(z, float64(x)+0.5, float64(y)+0.5)
	cam := mapCam{lat: lat, lon: lon, zoom: float64(z) + math.Log2(5), has: true}
	const viewH = 160
	dst := image.NewRGBA(image.Rect(0, 0, 240, viewH))
	paintView(dst, cam, map[tileKey]*mapGeom{{z: z, x: x, y: y}: &g})
	px := dst.RGBAAt(120, anchorRow(viewH))
	if px.R != colBldg.R || px.G != colBldg.G || px.B != colBldg.B {
		t.Fatalf("screen %+v", px)
	}
	plate := renderPlate(cam, 240, viewH, map[tileKey]*mapGeom{{z: z, x: x, y: y}: &g})
	sampled := sampleOf(plate, cam, 240, viewH)
	sp := sampled.RGBAAt(120, anchorRow(viewH))
	if sp.R != colBldg.R || sp.G != colBldg.G || sp.B != colBldg.B {
		t.Fatalf("plate %+v", sp)
	}
}

func TestVectorPaintUsesParentTile(t *testing.T) {
	g := mapGeom{}
	g.addPoly(kindWater, [][]pt2{{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}}})
	const pz, px, py = 13, 100, 80
	lat, lon := latLonAtTile(pz+1, float64(px<<1)+0.5, float64(py<<1)+0.5)
	cam := mapCam{lat: lat, lon: lon, zoom: float64(pz + 1), has: true}
	const viewH = 60
	dst := image.NewRGBA(image.Rect(0, 0, 80, viewH))
	paintView(dst, cam, map[tileKey]*mapGeom{{z: pz, x: px, y: py}: &g})
	c := dst.RGBAAt(40, anchorRow(viewH))
	if c.R != colWater.R || c.G != colWater.G || c.B != colWater.B {
		t.Fatalf("parent %+v", c)
	}
}

func TestMapPaintsOffTheDraw(t *testing.T) {
	prev := liveAtlas
	liveAtlas = newAtlas()
	t.Cleanup(func() { liveAtlas = prev })
	sh := newDrive()
	sh.engine = newMapEngine(rendererVector)
	defer sh.stopMap()
	cam := mapCam{lat: 50.25, lon: 19.04, zoom: 14, has: true}
	_, keys, _, _, _, _ := plateTiles(cam, 80, 60)
	if len(keys) == 0 {
		t.Fatal("no tiles")
	}
	g := mapGeom{}
	g.addPoly(kindWater, [][]pt2{{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}}})
	liveAtlas.store(keys[0], g, styleOSMBright)
	sh.mu.Lock()
	sh.mapKind = mapOSM
	sh.w, sh.h = 80, 60
	sh.cam = cam
	sh.mu.Unlock()
	sh.stepMap(time.Unix(0, 0))
	sh.mu.Lock()
	started := sh.mapJob.stamp != ""
	sh.mu.Unlock()
	if !started {
		t.Fatal("camera did not ask for a plate")
	}
	dst := image.NewRGBA(image.Rect(0, 0, 80, 60))
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		sh.blitDriveMap(dst, time.Unix(0, 0))
		if countInk(dst, colWater) > 10 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("painter did not publish a plate")
}

func TestTiltKeepsPlateScale(t *testing.T) {
	cam := mapCam{lat: 50.25, lon: 19.04, zoom: 16, has: true, heading: 15}
	flat := renderPlate(cam, 800, 480, map[tileKey]*mapGeom{})
	cam.pitch = 1
	tilted := renderPlate(cam, 800, 480, map[tileKey]*mapGeom{})
	if flat == nil || tilted == nil {
		t.Fatal("no plate")
	}
	if tilted.scale+0.05 < flat.scale {
		t.Fatalf("tilt scale %v flat %v capped %v/%v", tilted.scale, flat.scale, tilted.capped, flat.capped)
	}
}

func TestPlateZoomScalesWithoutRebuild(t *testing.T) {
	const z, x, y = 14, 100, 80
	g := mapGeom{}
	g.addPoly(kindWater, [][]pt2{{{0.4, 0.4}, {0.6, 0.4}, {0.6, 0.6}, {0.4, 0.6}, {0.4, 0.4}}})
	lat, lon := latLonAtTile(z, float64(x)+0.5, float64(y)+0.5)
	cam := mapCam{lat: lat, lon: lon, zoom: float64(z), has: true}
	p := renderPlate(cam, 200, 150, map[tileKey]*mapGeom{{z: z, x: x, y: y}: &g})
	if p == nil || p.capped {
		t.Fatalf("plate %+v", p)
	}
	closer := cam
	closer.zoom = float64(z) + 0.4
	turned := cam
	turned.heading = 6
	if !p.covers(closer, 200, 150) || !p.covers(turned, 200, 150) {
		t.Fatal("a small zoom and turn left the plate")
	}
	n0 := countInk(sampleOf(p, cam, 200, 150), colWater)
	n1 := countInk(sampleOf(p, closer, 200, 150), colWater)
	if n0 < 20 || n1 <= n0 {
		t.Fatalf("water pixels %d -> %d", n0, n1)
	}
}

func TestBlankTileGlitchesOnlyItsSquare(t *testing.T) {
	const z, x, y = 14, 100, 80
	water := mapGeom{}
	water.addPoly(kindWater, [][]pt2{{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}}})
	var blank mapGeom
	lat, lon := latLonAtTile(z, float64(x)+1, float64(y)+0.5)
	cam := mapCam{lat: lat, lon: lon, zoom: float64(z), has: true}
	p := renderPlate(cam, 320, 200, map[tileKey]*mapGeom{
		{z: z, x: x, y: y}:     &water,
		{z: z, x: x + 1, y: y}: &blank,
	})
	if p == nil {
		t.Fatal("no plate")
	}
	dst := sampleOf(p, cam, 320, 200)
	ay := int(float64(dst.Bounds().Dy()) * camAnchor)
	left := dst.RGBAAt(dst.Bounds().Dx()/2-24, ay)
	right := dst.RGBAAt(dst.Bounds().Dx()/2+24, ay)
	if left.R != colWater.R || left.G != colWater.G || left.B != colWater.B {
		t.Fatalf("water side %+v", left)
	}
	if (right.R == colWater.R && right.G == colWater.G && right.B == colWater.B) ||
		(right.R == colLand.R && right.G == colLand.G && right.B == colLand.B) {
		t.Fatalf("blank side %+v", right)
	}
	if countInk(dst, colGlitchRed)+countInk(dst, colGlitchBlue) == 0 {
		t.Fatal("blank square has no glitch streak")
	}
}

func TestBlankTileGlitchCoversParent(t *testing.T) {
	const z, x, y = 14, 100, 80
	parent := mapGeom{}
	parent.addPoly(kindWater, [][]pt2{{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}}})
	var blank mapGeom
	lat, lon := latLonAtTile(z, float64(x)+0.5, float64(y)+0.5)
	cam := mapCam{lat: lat, lon: lon, zoom: float64(z), has: true}
	p := renderPlate(cam, 180, 140, map[tileKey]*mapGeom{
		{z: z - 1, x: x >> 1, y: y >> 1}: &parent,
		{z: z, x: x, y: y}:               &blank,
	})
	if p == nil {
		t.Fatal("no plate")
	}
	dst := sampleOf(p, cam, 180, 140)
	c := dst.RGBAAt(dst.Bounds().Dx()/2, int(float64(dst.Bounds().Dy())*camAnchor))
	if c.R == colWater.R && c.G == colWater.G && c.B == colWater.B {
		t.Fatal("parent showed through a blank tile")
	}
	if c.R == colLand.R && c.G == colLand.G && c.B == colLand.B {
		t.Fatal("blank tile stayed land")
	}
}

func anchorRow(h int) int {
	return int(float64(h) * camAnchor)
}

func sampleOf(p *mapPlate, cam mapCam, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	samplePlate(dst, p, cam, time.Unix(0, 0))
	return dst
}

func countInk(img *image.RGBA, want color.NRGBA) int {
	if img == nil {
		return 0
	}
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.R == want.R && c.G == want.G && c.B == want.B {
				n++
			}
		}
	}
	return n
}

func TestMapLogReachesShell(t *testing.T) {
	var got string
	prev := mapLog
	mapLog = func(format string, args ...any) { got = fmt.Sprintf(format, args...) }
	t.Cleanup(func() { mapLog = prev })
	mapLog("fetch z%d %d,%d", 14, 3, 4)
	if !strings.Contains(got, "fetch z14 3,4") {
		t.Fatalf("%q", got)
	}
}

func latLonAtTile(z int, tx, ty float64) (lat, lon float64) {
	n := float64(uint(1)<<uint(z)) * float64(tilePx)
	lon = tx*float64(tilePx)/n*360 - 180
	merc := math.Pi * (1 - 2*ty*float64(tilePx)/n)
	lat = math.Atan(math.Sinh(merc)) * 180 / math.Pi
	return lat, lon
}

func TestZoomMissDoesNotBlankCachedTiles(t *testing.T) {
	a := newAtlas()
	a.store(tileKey{z: 14, x: 1, y: 1}, mapGeom{}, styleOSMBright)
	a.markMiss(tileKey{z: 15, x: 2, y: 2}, 418)
	if a.isDown() {
		t.Fatal("http 418 marked the atlas down")
	}
	z, ok := a.zoomCap()
	if !ok || z != 14 {
		t.Fatalf("cap %d %v", z, ok)
	}
}

func TestBrowsingArrowStaysOnTheCar(t *testing.T) {
	const w, h = 200, 120
	cam := mapCam{lat: 50, lon: 19, zoom: 16, has: true, hasCar: true, carLat: 50, carLon: 19, browsing: true}
	fy := float64(h) * camAnchor
	cam.pan(float64(w)/2, fy, 50, 0, w, h)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	paintDriveMap(dst, mapOSM, cam, nil, false, time.Unix(0, 0))
	sx, sy, ok := cam.project(cam.carLat, cam.carLon, w, h)
	if !ok || sx <= float64(w)/2+10 {
		t.Fatalf("car x %v ok %v", sx, ok)
	}
	c := dst.RGBAAt(int(math.Round(sx)), int(math.Round(sy))-8)
	if c.R != colLime.R || c.G != colLime.G || c.B != colLime.B {
		t.Fatalf("arrow %+v at %v,%v", c, sx, sy)
	}
	anchor := dst.RGBAAt(w/2, int(fy)-8)
	if anchor.R == colLime.R && anchor.G == colLime.G && anchor.B == colLime.B {
		t.Fatal("arrow stayed on the anchor")
	}
}

func TestEmptyTileFailureIsGlitchSignal(t *testing.T) {
	a := newAtlas()
	a.markBad(tileKey{z: 14, x: 1, y: 2})
	_, fail := a.prepare([]tileKey{{z: 14, x: 1, y: 2}})
	if !fail {
		t.Fatal("expected empty failure")
	}
}
