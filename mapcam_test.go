package main

import (
	"math"
	"testing"
	"time"
)

func TestTileZoomStaysOnTheLowerBitmap(t *testing.T) {
	if tileZoom(14.6) != 14 || tileZoom(16.8) != 16 || tileZoom(15) != 15 {
		t.Fatalf("%d %d %d", tileZoom(14.6), tileZoom(16.8), tileZoom(15))
	}
	if tileZoom(0) != 2 {
		t.Fatalf("world zoom samples %d", tileZoom(0))
	}
}

func TestZoomOutReachesTheWorld(t *testing.T) {
	c := mapCam{lat: 50, lon: 19, zoom: 1, has: true}
	c.nudgeZoom(-1)
	if c.zoom != 0 {
		t.Fatalf("zoom %v", c.zoom)
	}
	c.nudgeZoom(-1)
	if c.zoom != 0 {
		t.Fatalf("below the world %v", c.zoom)
	}
	z, keys := c.viewTiles(800, 480)
	if z != 2 || len(keys) == 0 || len(keys) > 16 {
		t.Fatalf("world tiles z %d n %d", z, len(keys))
	}
	_, plateKeys, _, _, _, _ := plateTiles(c, 800, 480)
	if len(plateKeys) == 0 || len(plateKeys) > 16 {
		t.Fatalf("plate %d", len(plateKeys))
	}
}

func TestCameraStaysOnLastServedZoom(t *testing.T) {
	prev := liveAtlas
	a := newAtlas()
	a.maxOK = 14
	a.blockAbove = true
	a.blockZ = 15
	liveAtlas = a
	t.Cleanup(func() { liveAtlas = prev })
	var c mapCam
	t0 := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	fix := loc{HasPos: true, Lat: 50, Lon: 19, HasSpeed: true, AccM: 5}
	c.step(t0, fix)
	if math.Abs(c.zoom-14) > 0.05 {
		t.Fatalf("start %v", c.zoom)
	}
	closeZ := 14 + math.Log2(5)
	if math.Abs(c.zoomTarget()-closeZ) > 0.02 {
		t.Fatalf("target %v", c.zoomTarget())
	}
	for i := 1; i <= 4; i++ {
		c.step(t0.Add(time.Duration(i)*250*time.Millisecond), fix)
	}
	if c.zoom < 14.01 || c.zoom > 14.03 {
		t.Fatalf("creep %v", c.zoom)
	}
}

func TestZoomMissStopsOnTheAvailableZoom(t *testing.T) {
	c := mapCam{lat: 27.5, lon: -82.5, zoom: 10, has: true, browsing: true}
	c.noteZoomMiss(6, 27.5, -82.5)
	if c.zoom != 6 || !c.missOn {
		t.Fatalf("zoom %v on %v", c.zoom, c.missOn)
	}
	c.nudgeZoom(1)
	if c.zoom != 6 {
		t.Fatalf("nudge %v", c.zoom)
	}
	c.lat, c.lon = 27.7, -82.4
	c.nudgeZoom(1)
	if !c.missOn || c.zoom != 6 {
		t.Fatalf("nearby zoom %v on %v", c.zoom, c.missOn)
	}
	c.lat, c.lon = 50.2, 19.0
	c.zoom = 12
	c.holdZoomMiss()
	if c.missOn || c.zoom != 12 {
		t.Fatalf("home zoom %v on %v", c.zoom, c.missOn)
	}
}

func TestKatowiceTile(t *testing.T) {
	x, y := tileXY(50.2476, 19.0403, 14)
	if x != 9058 || y != 5538 {
		t.Fatalf("tile %d %d", x, y)
	}
}

func TestHeadingDoesNotSnapNinety(t *testing.T) {
	var c mapCam
	t0 := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	fix := loc{HasPos: true, Lat: 50.25, Lon: 19.04, HasSpeed: true, Kmh: 40, HasCourse: true}
	c.step(t0, fix)
	if c.heading != 0 {
		t.Fatalf("start %v", c.heading)
	}
	fix.Course = 90
	for i := 1; i <= 4; i++ {
		c.step(t0.Add(time.Duration(i)*250*time.Millisecond), fix)
	}
	if c.heading < 3 || c.heading > 4 {
		t.Fatalf("heading %v", c.heading)
	}
}

func TestZoomDoesNotJumpALevel(t *testing.T) {
	var c mapCam
	t0 := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	slow := loc{HasPos: true, Lat: 50, Lon: 19, HasSpeed: true, AccM: 5}
	c.step(t0, slow)
	if math.Abs(c.zoom-16.8) > 0.01 {
		t.Fatalf("zoom %v", c.zoom)
	}
	fast := slow
	fast.Kmh = 130
	fast.HasCourse = true
	fast.Course = 10
	c.step(t0.Add(time.Second), fast)
	if c.zoom >= 16.8 || 16.8-c.zoom >= 1 {
		t.Fatalf("zoom %v", c.zoom)
	}
}

func TestCoarseFixStaysWide(t *testing.T) {
	if z := zoomForKmh(0, 100); z != 14 {
		t.Fatal(z)
	}
	if z := zoomForKmh(0, 10); math.Abs(z-16.8) > 1e-9 {
		t.Fatal(z)
	}
	if math.Abs(zoomForKmh(30, 5)-16.2) > 1e-9 {
		t.Fatal(zoomForKmh(30, 5))
	}
}

func TestHeadingHoldsThroughACrawl(t *testing.T) {
	var c mapCam
	t0 := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	fix := loc{HasPos: true, Lat: 50, Lon: 19, HasSpeed: true, Kmh: 40, HasCourse: true, Course: 90}
	c.step(t0, fix)
	fix.Kmh = 8
	fix.Course = 0
	c.step(t0.Add(time.Second), fix)
	if math.Abs(c.heading-90) > 1 {
		t.Fatalf("crawl %v", c.heading)
	}
	fix.Kmh = 3
	for i := 1; i <= 12; i++ {
		c.step(t0.Add(time.Duration(i+1)*time.Second), fix)
	}
	if c.heading < 75 || c.heading > 85 {
		t.Fatalf("slow north %v", c.heading)
	}
	for i := 13; i <= 120; i++ {
		c.step(t0.Add(time.Duration(i+1)*time.Second), fix)
	}
	if c.heading > 1 {
		t.Fatalf("north %v", c.heading)
	}
}

func TestPitchZeroMatchesFlat(t *testing.T) {
	cam := mapCam{lat: 50.25, lon: 19.04, zoom: 16, has: true, heading: 20}
	w, h, z := 80, 100, 16
	cx, cy := worldPx(cam.lat, cam.lon, z)
	scale := math.Pow(2, cam.zoom-float64(z))
	sin, cos := math.Sincos(cam.heading * math.Pi / 180)
	for _, p := range [][2]int{{0, 0}, {40, 10}, {79, 62}, {10, 90}} {
		wx, wy := cam.screenWorld(p[0], p[1], w, h, z)
		rx := float64(p[0]) - float64(w)/2
		ry := float64(p[1]) - float64(h)*camAnchor
		wantX := cx + (rx*cos-ry*sin)/scale
		wantY := cy + (rx*sin+ry*cos)/scale
		if math.Abs(wx-wantX) > 1e-6 || math.Abs(wy-wantY) > 1e-6 {
			t.Fatalf("flat %v got %v %v want %v %v", p, wx, wy, wantX, wantY)
		}
	}
	_, xMul := rowMap(float64(h)*camAnchor, float64(h), 1)
	if math.Abs(xMul-1) > 1e-9 {
		t.Fatalf("car row scale %v", xMul)
	}
}

func TestTiltIsPureMapsPitch(t *testing.T) {
	if camTiltDeg != 50 {
		t.Fatalf("pitch %v", camTiltDeg)
	}
	h := 480.0
	_, carMul := rowMap(h*camAnchor, h, 1)
	if math.Abs(carMul-1) > 1e-6 {
		t.Fatalf("car scale %v", carMul)
	}
	ry, topMul := rowMap(0, h, 1)
	flat := -h * camAnchor
	if topMul < 1.5 || topMul > 2.5 {
		t.Fatalf("top scale %v", topMul)
	}
	if ry >= flat {
		t.Fatalf("top ground %v flat %v", ry, flat)
	}
	// A north-south road stays straight: screen x is the world offset
	// divided by the row scale, so the sides meet at one vanishing point.
	ry2, mul2 := rowMap(h*0.3, h, 1)
	if mul2 <= 1 || ry2 >= -h*camAnchor+h*0.3 {
		t.Fatalf("mid row scale %v ground %v", mul2, ry2)
	}
}

func TestPitchReachesFurtherAhead(t *testing.T) {
	base := mapCam{lat: 50.25, lon: 19.04, zoom: 16, has: true}
	w, h, z := 80, 100, 16
	_, carY := worldPx(base.lat, base.lon, z)
	sy := 8
	_, flatY := base.screenWorld(w/2, sy, w, h, z)
	tilted := base
	tilted.pitch = 1
	_, tiltY := tilted.screenWorld(w/2, sy, w, h, z)
	if math.Abs(tiltY-carY) <= math.Abs(flatY-carY)+1 {
		t.Fatalf("tilt %v flat %v car %v", tiltY, flatY, carY)
	}
	flatX, _ := base.screenWorld(8, sy, w, h, z)
	tiltX, _ := tilted.screenWorld(8, sy, w, h, z)
	cx, _ := worldPx(base.lat, base.lon, z)
	if math.Abs(tiltX-cx) <= math.Abs(flatX-cx) {
		t.Fatalf("row did not widen tilt %v flat %v car %v", tiltX, flatX, cx)
	}
}

func TestTiltEasesWhileDriving(t *testing.T) {
	var c mapCam
	c.tilt = tiltDriving
	t0 := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	stop := loc{HasPos: true, Lat: 50, Lon: 19, HasSpeed: true, AccM: 5}
	c.step(t0, stop)
	if c.pitch != 0 {
		t.Fatalf("stop %v", c.pitch)
	}
	drive := stop
	drive.Kmh = 40
	drive.HasCourse = true
	drive.Course = 90
	c.step(t0.Add(250*time.Millisecond), drive)
	if c.pitch < 0.10 || c.pitch > 0.14 {
		t.Fatalf("ease %v", c.pitch)
	}
	c.tilt = tiltNever
	for i := 1; i <= 10; i++ {
		c.step(t0.Add(time.Duration(i+1)*250*time.Millisecond), drive)
	}
	if c.pitch > 0.05 {
		t.Fatalf("ease off %v", c.pitch)
	}
	var always mapCam
	always.tilt = tiltAlways
	always.step(t0, stop)
	if always.pitch != 1 {
		t.Fatalf("always %v", always.pitch)
	}
}

func TestAheadScaleOpensWithSpeed(t *testing.T) {
	var c mapCam
	c.ahead = aheadDefault
	t0 := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	stop := loc{HasPos: true, Lat: 50, Lon: 19, HasSpeed: true, AccM: 5}
	c.step(t0, stop)
	if math.Abs(c.zoomTarget()-16.8) > 0.01 {
		t.Fatalf("stop %v", c.zoomTarget())
	}
	c.zoomKmh = 100
	want := interpZoom(100) - math.Log2(aheadDefault)
	if math.Abs(c.zoomTarget()-want) > 0.02 {
		t.Fatalf("drive %v want %v", c.zoomTarget(), want)
	}
	if want >= interpZoom(100)-2 {
		t.Fatalf("scale did not open past two levels: %v", want)
	}
	c.ahead = 1
	if math.Abs(c.zoomTarget()-interpZoom(100)) > 0.02 {
		t.Fatalf("scale 1 %v", c.zoomTarget())
	}
}

func TestParkedZoomStaysStill(t *testing.T) {
	var c mapCam
	c.ahead = aheadDefault
	t0 := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	c.step(t0, loc{HasPos: true, Lat: 50, Lon: 19, HasSpeed: true, AccM: 8})
	for i := 1; i <= 40; i++ {
		kmh := 0
		acc := 8.0
		if i%10 == 0 {
			kmh = 40
			acc = 150
		}
		c.step(t0.Add(time.Duration(i)*250*time.Millisecond), loc{
			HasPos: true, Lat: 50, Lon: 19, HasSpeed: true, Kmh: kmh, AccM: acc,
		})
	}
	if math.Abs(c.zoom-16.8) > 0.08 {
		t.Fatalf("parked zoom walked to %v", c.zoom)
	}
}

func TestPositionSnapsAndEases(t *testing.T) {
	var c mapCam
	t0 := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	c.step(t0, loc{HasPos: true, Lat: 50, Lon: 19})
	c.step(t0.Add(time.Second), loc{HasPos: true, Lat: 50.001, Lon: 19})
	if c.lat <= 50 || c.lat >= 50.001 {
		t.Fatalf("ease %v", c.lat)
	}
	c.step(t0.Add(2*time.Second), loc{HasPos: true, Lat: 51, Lon: 19})
	if math.Abs(c.lat-51) > 1e-6 {
		t.Fatalf("snap %v", c.lat)
	}
	c.step(t0.Add(3*time.Second), loc{})
	if !c.has || math.Abs(c.lat-51) > 1e-6 {
		t.Fatalf("held %+v", c)
	}
}

func TestPanFollowsTheFinger(t *testing.T) {
	const w, h = 800, 480
	fy := float64(h) * camAnchor
	right := mapCam{lat: 50, lon: 19, zoom: 16, has: true}
	right.pan(400, fy, 40, 0, w, h)
	if right.lon >= 19 {
		t.Fatalf("drag right lon %v", right.lon)
	}
	up := mapCam{lat: 50, lon: 19, zoom: 16, has: true}
	up.pan(400, fy, 0, -40, w, h)
	if up.lat >= 50 {
		t.Fatalf("drag up lat %v", up.lat)
	}
}

func TestBrowseHoldsTheView(t *testing.T) {
	prev := liveAtlas
	liveAtlas = newAtlas()
	t.Cleanup(func() { liveAtlas = prev })
	t0 := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	c := mapCam{
		lat: 50, lon: 19, zoom: 16, has: true,
		browsing: true, browseUntil: t0.Add(camBrowseHold),
	}
	fix := loc{HasPos: true, Lat: 50.01, Lon: 19.01, HasSpeed: true, Kmh: 40, HasCourse: true, Course: 90}
	c.step(t0, fix)
	if !c.browsing || c.lat != 50 || c.lon != 19 {
		t.Fatalf("fix moved the held view: %+v", c)
	}
	if c.carLat != 50.01 || c.carLon != 19.01 {
		t.Fatalf("car %+v %+v", c.carLat, c.carLon)
	}
	c.nudgeZoom(1)
	if c.zoom != 17 {
		t.Fatalf("zoom %v", c.zoom)
	}
	c.step(t0.Add(time.Second), fix)
	if math.Abs(c.zoom-17) > 0.001 {
		t.Fatalf("zoom eased to %v", c.zoom)
	}
	c.step(t0.Add(camBrowseHold+time.Second), fix)
	if c.browsing {
		t.Fatal("browse did not end")
	}
	if math.Abs(c.lat-50.01) > 1e-6 || math.Abs(c.lon-19.01) > 1e-6 {
		t.Fatalf("did not snap back %+v %+v", c.lat, c.lon)
	}
	if !c.showZoom() {
		t.Fatal("zoom controls dropped while the view was still zooming back")
	}
}

func TestProjectMatchesTheGround(t *testing.T) {
	cam := mapCam{lat: 50.2, lon: 19.1, zoom: 15.4, heading: 30, pitch: 0, has: true}
	w, h := 800, 480
	points := [][2]int{{40, 20}, {400, int(float64(h) * camAnchor)}, {760, 400}}
	for _, pitch := range []float64{0, 0.6} {
		cam.pitch = pitch
		for _, p := range points {
			wx, wy := cam.screenWorld(p[0], p[1], w, h, cam.sampleZoom())
			lat, lon := worldToLatLon(wx, wy, cam.sampleZoom())
			sx, sy, ok := cam.project(lat, lon, w, h)
			if !ok || math.Abs(sx-float64(p[0])) > 0.75 || math.Abs(sy-float64(p[1])) > 0.75 {
				t.Fatalf("pitch %v point %v -> %v,%v ok %v", pitch, p, sx, sy, ok)
			}
		}
	}
}
