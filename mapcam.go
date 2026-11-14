package main

import (
	"math"
	"time"
)

// The drive camera stays on the car. Heading slews, zoom eases with speed,
// pitch eases with the tilt setting, and a jump of a few hundred meters
// snaps instead of sliding across town. A finger drag holds the view where
// it was left; after a quiet stretch it eases back onto the car.

const (
	camHeadFast     = 12.0 // km/h: follow the GNSS course
	camHeadSlow     = 6.0  // km/h: ease back to north
	camHeadRate     = 3.5  // deg/s, so a right angle takes about 25s
	camZoomOutTau   = 15.0
	camZoomInTau    = 60.0
	camZoomOut      = 0.1   // levels per second, opening back out
	camZoomIn       = 0.018 // levels per second, creeping in on the detail
	camZoomSpeedTau = 8.0   // seconds, speed and accuracy that drive zoom
	camPosTau       = 0.5
	camSnapM        = 300.0
	camDTMax        = 0.25
	camTiltTau      = 2.0       // seconds, flat to full tilt
	camTiltDeg      = 50.0      // Pure Maps navigate / follow-me pitch
	camFovTan       = 1.0 / 3.0 // tan(half of Mapbox's vertical field of view)
	camBrowseHold   = 15 * time.Second
	camZoomMin      = 0 // the whole world; the bitmap stays on z2 and scales down
	camZoomMax      = 18.0
	// zoomMissReach is how far a missing download keeps the zoom cap.
	// Past this the view is somewhere else, and a closer zoom is tried again.
	zoomMissReach = 250000.0
	aheadMin      = 1.0 // no extra ground
	aheadMax      = 8.0 // eight times the speed-curve distance
	aheadDefault  = 8.0
	aheadFullKmh  = 80.0
	// The car and the arrow sit a bit below the center of the screen.
	camAnchor = 0.62
	// camClose is log2(5). The tight view keeps a fifth of the served frame.
	camClose = 2.321928094887362
	tilePx   = 256

	tiltAlways  = "always"
	tiltDriving = "driving"
	tiltNever   = "never"
)

type tileKey struct {
	z, x, y int
}

type mapCam struct {
	lat, lon   float64
	heading    float64
	zoom       float64
	pitch      float64 // 0 flat, 1 full tilt
	has        bool
	targetHead float64
	following  bool
	tilt       string  // always, driving, never; empty stays flat
	ahead      float64 // ground scale while moving, 1 = none
	zoomKmh    float64 // eased speed for the zoom only
	zoomAcc    float64 // eased accuracy, metres
	last       time.Time
	// browsing is a finger pan. The view center stays where the drag left
	// it, and the arrow is drawn on the car instead of the anchor.
	browsing    bool
	returning   bool // easing back onto the car; the zoom buttons stay up
	dragging    bool
	browseUntil time.Time
	hasCar      bool
	// missOn holds the closest zoom Scout could draw around missLat/missLon.
	missOn           bool
	missZoom         float64
	missLat, missLon float64
	carLat           float64
	carLon           float64
}

// step moves the camera toward fix. It reports whether the picture changed
// enough to redraw. A fix with no position leaves the last view where it is.
func (c *mapCam) step(now time.Time, fix loc) bool {
	prevLat, prevLon := c.lat, c.lon
	prevH, prevZ, prevP, prevHas := c.heading, c.zoom, c.pitch, c.has
	c.advance(now, fix)
	if c.has != prevHas {
		return true
	}
	if !c.has {
		return false
	}
	if math.Abs(angDelta(prevH, c.heading)) > 0.2 {
		return true
	}
	if math.Abs(c.zoom-prevZ) > 0.01 || math.Abs(c.pitch-prevP) > 0.01 {
		return true
	}
	return metersBetween(prevLat, prevLon, c.lat, c.lon) > 0.4
}

func (c *mapCam) advance(now time.Time, fix loc) {
	dt := c.takeDT(now)
	if c.browsing && !c.dragging && !c.browseUntil.IsZero() && !now.Before(c.browseUntil) {
		c.browsing = false
		c.returning = true
	}
	if !fix.HasPos {
		return
	}
	c.carLat, c.carLon = fix.Lat, fix.Lon
	c.hasCar = true
	if !c.has {
		c.lat, c.lon = fix.Lat, fix.Lon
		c.following = fix.HasSpeed && float64(fix.Kmh) >= camHeadFast && fix.HasCourse
		if c.following {
			c.targetHead = normDeg(fix.Course)
		}
		c.heading = c.targetHead
		c.zoom = c.zoomStart()
		c.pitch = c.wantPitch()
		c.has = true
		return
	}
	c.retarget(fix)
	if c.browsing {
		return
	}
	c.easePos(fix, dt)
	c.slewHead(dt)
	c.easeZoomInput(fix, dt)
	c.easeZoom(dt)
	c.easePitch(dt)
	if c.returning && c.backOnCar(fix) {
		c.returning = false
	}
}

// backOnCar is true once a released browse has eased onto the fix again.
func (c *mapCam) backOnCar(fix loc) bool {
	if !fix.HasPos || metersBetween(c.lat, c.lon, fix.Lat, fix.Lon) > 20 {
		return false
	}
	return math.Abs(c.zoom-c.zoomTarget()) < 0.05
}

// showZoom is why − and + are on screen: the finger has the view, or it is
// still easing back and can be grabbed again.
func (c *mapCam) showZoom() bool {
	return c.browsing || c.returning
}

func (c *mapCam) takeDT(now time.Time) float64 {
	dt := 0.0
	if !c.last.IsZero() {
		dt = now.Sub(c.last).Seconds()
	}
	c.last = now
	if dt < 0 {
		dt = 0
	}
	if dt > camDTMax {
		dt = camDTMax
	}
	return dt
}

func (c *mapCam) retarget(fix loc) {
	spd := 0.0
	if fix.HasSpeed {
		spd = float64(fix.Kmh)
	}
	switch {
	case spd >= camHeadFast && fix.HasCourse:
		c.targetHead = normDeg(fix.Course)
		c.following = true
	case spd < camHeadSlow:
		c.targetHead = 0
		c.following = false
	}
}

func (c *mapCam) slewHead(dt float64) {
	d := angDelta(c.heading, c.targetHead)
	maxD := camHeadRate * dt
	if math.Abs(d) <= maxD {
		c.heading = c.targetHead
		return
	}
	if d > 0 {
		c.heading += maxD
	} else {
		c.heading -= maxD
	}
	c.heading = normDeg(c.heading)
}

// zoomStart is the first frame: the close view. Speed opens it afterwards.
func (c *mapCam) zoomStart() float64 {
	z := interpZoom(0)
	if cap, ok := liveAtlas.zoomCap(); ok && z > float64(cap) {
		return float64(cap)
	}
	return z
}

func (c *mapCam) zoomTarget() float64 {
	target := interpZoom(c.zoomKmh) - c.aheadDrop(c.zoomKmh)
	if c.zoomAcc > 80 && target > 14 {
		target = 14
	}
	if z, ok := liveAtlas.zoomCap(); ok {
		ceil := float64(z) + camClose
		if target > ceil {
			target = ceil
		}
	}
	return target
}

// easeZoomInput lets a noisy fix settle before it can move the zoom.
func (c *mapCam) easeZoomInput(fix loc, dt float64) {
	if dt <= 0 {
		return
	}
	kmh := 0.0
	if fix.HasSpeed && fix.Kmh > 0 {
		kmh = float64(fix.Kmh)
	}
	acc := 0.0
	if fix.HasPos && fix.AccM > 0 {
		acc = fix.AccM
	}
	a := 1 - math.Exp(-dt/camZoomSpeedTau)
	c.zoomKmh += (kmh - c.zoomKmh) * a
	c.zoomAcc += (acc - c.zoomAcc) * a
}

func (c *mapCam) easeZoom(dt float64) {
	target := c.zoomTarget()
	tau := camZoomOutTau
	rate := camZoomOut
	if target > c.zoom {
		tau = camZoomInTau
		rate = camZoomIn
	}
	step := (target - c.zoom) * (1 - math.Exp(-dt/tau))
	maxStep := rate * dt
	if step > maxStep {
		step = maxStep
	} else if step < -maxStep {
		step = -maxStep
	}
	c.zoom += step
	c.holdZoomMiss()
}

// noteZoomMiss remembers that zoom is the closest Scout could draw at lat/lon.
// The camera stops there until the view leaves that place.
func (c *mapCam) noteZoomMiss(zoom, lat, lon float64) {
	if zoom < camZoomMin {
		zoom = camZoomMin
	}
	moved := !c.missOn || metersBetween(lat, lon, c.missLat, c.missLon) > zoomMissReach
	if moved || zoom < c.missZoom {
		c.missOn = true
		c.missZoom = zoom
		c.missLat, c.missLon = lat, lon
	}
	c.holdZoomMiss()
}

func (c *mapCam) holdZoomMiss() {
	if !c.missOn {
		return
	}
	if metersBetween(c.lat, c.lon, c.missLat, c.missLon) > zoomMissReach {
		c.missOn = false
		return
	}
	if c.zoom > c.missZoom {
		c.zoom = c.missZoom
	}
}

// aheadDrop is how many zoom levels the scale opens at this speed.
// A scale of 1 adds nothing. 8 is three levels, and only once the eased
// speed is up near aheadFullKmh. A parked fix stays on the close view.
func (c *mapCam) aheadDrop(kmh float64) float64 {
	if c.ahead <= 1 || kmh <= 0 {
		return 0
	}
	return math.Log2(c.ahead) * aheadBlend(kmh)
}

func aheadBlend(kmh float64) float64 {
	if kmh <= 0 {
		return 0
	}
	if kmh >= aheadFullKmh {
		return 1
	}
	t := kmh / aheadFullKmh
	return t * t * (3 - 2*t)
}

func clampAhead(v float64) float64 {
	if v < aheadMin || math.IsNaN(v) {
		return aheadMin
	}
	if v > aheadMax {
		return aheadMax
	}
	return v
}

// wantPitch is 1 when the tilt setting says the picture should be pitched.
// An empty tilt stays flat so a camera that never loaded settings does too.
func (c *mapCam) wantPitch() float64 {
	switch c.tilt {
	case tiltAlways:
		return 1
	case tiltDriving:
		if c.following {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func (c *mapCam) easePitch(dt float64) {
	target := c.wantPitch()
	if dt <= 0 {
		return
	}
	c.pitch += (target - c.pitch) * (1 - math.Exp(-dt/camTiltTau))
	if math.Abs(c.pitch-target) < 1e-3 {
		c.pitch = target
	}
	if c.pitch < 0 {
		c.pitch = 0
	} else if c.pitch > 1 {
		c.pitch = 1
	}
}

// nudgeZoom steps the view while it is held off the car. One tap is one level.
func (c *mapCam) nudgeZoom(delta float64) {
	c.zoom += delta
	if c.zoom < camZoomMin {
		c.zoom = camZoomMin
	}
	if c.zoom > camZoomMax {
		c.zoom = camZoomMax
	}
	if z, ok := liveAtlas.zoomCap(); ok {
		ceil := float64(z) + camClose
		if c.zoom > ceil {
			c.zoom = ceil
		}
	}
	c.holdZoomMiss()
}

// pan moves the view so the ground under the finger stays under the finger.
// dx and dy are the finger's latest step, in framebuffer pixels.
func (c *mapCam) pan(fx, fy, dx, dy float64, w, h int) {
	if !c.has || w < 1 || h < 1 {
		return
	}
	z := c.sampleZoom()
	wx0, wy0 := c.screenWorldF(fx-dx, fy-dy, w, h, z)
	wx1, wy1 := c.screenWorldF(fx, fy, w, h, z)
	cx, cy := worldPx(c.lat, c.lon, z)
	c.lat, c.lon = worldToLatLon(cx+(wx0-wx1), cy+(wy0-wy1), z)
	c.holdZoomMiss()
}

func (c *mapCam) easePos(fix loc, dt float64) {
	if metersBetween(c.lat, c.lon, fix.Lat, fix.Lon) > camSnapM {
		c.lat, c.lon = fix.Lat, fix.Lon
		return
	}
	a := 1 - math.Exp(-dt/camPosTau)
	c.lat += (fix.Lat - c.lat) * a
	c.lon += (fix.Lon - c.lon) * a
}

// zoomForKmh is the continuous zoom a speed should settle on.
// Accuracy coarser than 80 m is not allowed tighter than z14.
func zoomForKmh(kmh int, accM float64) float64 {
	z := interpZoom(float64(kmh))
	if accM > 80 && z > 14 {
		return 14
	}
	return z
}

func interpZoom(kmh float64) float64 {
	if kmh <= 0 {
		return 16.8
	}
	if kmh >= 130 {
		return 14
	}
	type knot struct{ k, z float64 }
	knots := [...]knot{{0, 16.8}, {30, 16.2}, {60, 15.4}, {100, 14.6}, {130, 14}}
	for i := 1; i < len(knots); i++ {
		if kmh <= knots[i].k {
			span := knots[i].k - knots[i-1].k
			t := (kmh - knots[i-1].k) / span
			return knots[i-1].z + t*(knots[i].z-knots[i-1].z)
		}
	}
	return 14
}

// tileZoom is the integer tiles under a continuous zoom. The fraction is a
// scale of that bitmap, so 14.8 stays on z14 instead of jumping to empty z15.
func tileZoom(z float64) int {
	zi := int(math.Floor(z))
	if zi < 2 {
		return 2
	}
	if zi > 18 {
		return 18
	}
	return zi
}

// sampleZoom is the tile level to fetch and draw. It will not go past the
// deepest zoom Scout has actually served.
func (c mapCam) sampleZoom() int {
	z := tileZoom(c.zoom)
	if cap, ok := liveAtlas.zoomCap(); ok && z > cap {
		return cap
	}
	return z
}

// tileXY is the slippy-map index of lat/lon at integer zoom z.
func tileXY(lat, lon float64, z int) (x, y int) {
	if z < 0 {
		z = 0
	}
	wx, wy := worldPx(lat, lon, z)
	x = int(math.Floor(wx / tilePx))
	y = int(math.Floor(wy / tilePx))
	n := 1 << z
	if n < 1 {
		n = 1
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= n {
		x = n - 1
	}
	if y >= n {
		y = n - 1
	}
	return x, y
}

func worldToLatLon(x, y float64, z int) (lat, lon float64) {
	if z < 0 {
		z = 0
	}
	n := float64(uint(1)<<uint(z)) * tilePx
	if n < 1 {
		n = tilePx
	}
	lon = x/n*360 - 180
	lat = math.Atan(math.Sinh(math.Pi*(1-2*y/n))) * 180 / math.Pi
	if lat > 85.05112878 {
		lat = 85.05112878
	}
	if lat < -85.05112878 {
		lat = -85.05112878
	}
	return lat, lon
}

func worldPx(lat, lon float64, z int) (x, y float64) {
	if z < 0 {
		z = 0
	}
	n := float64(uint(1)<<uint(z)) * tilePx
	x = (lon + 180) / 360 * n
	if lat > 85.05112878 {
		lat = 85.05112878
	}
	if lat < -85.05112878 {
		lat = -85.05112878
	}
	r := lat * math.Pi / 180
	y = (1 - math.Log(math.Tan(r)+1/math.Cos(r))/math.Pi) / 2 * n
	return x, y
}

// viewTiles lists the integer-zoom tiles under the rotated screen, plus a
// one-tile margin. The list is capped so a bad camera cannot request the world.
func (c mapCam) viewTiles(w, h int) (z int, keys []tileKey) {
	z = c.sampleZoom()
	if w < 1 || h < 1 || !c.has {
		return z, nil
	}
	n := 1 << z
	minTX, minTY := n, n
	maxTX, maxTY := -1, -1
	corners := [4][2]int{{0, 0}, {w - 1, 0}, {0, h - 1}, {w - 1, h - 1}}
	for _, p := range corners {
		wx, wy := c.screenWorld(p[0], p[1], w, h, z)
		tx := int(math.Floor(wx / tilePx))
		ty := int(math.Floor(wy / tilePx))
		if tx < minTX {
			minTX = tx
		}
		if ty < minTY {
			minTY = ty
		}
		if tx > maxTX {
			maxTX = tx
		}
		if ty > maxTY {
			maxTY = ty
		}
	}
	tx0, tx1, okX := tileSpan(minTX-1, maxTX+1, n)
	ty0, ty1, okY := tileSpan(minTY-1, maxTY+1, n)
	if !okX || !okY {
		return z, nil
	}
	for ty := ty0; ty <= ty1; ty++ {
		for tx := tx0; tx <= tx1; tx++ {
			keys = append(keys, tileKey{z: z, x: tx, y: ty})
			if len(keys) >= 24 {
				return z, keys
			}
		}
	}
	return z, keys
}

// tileSpan clamps a tile index range onto 0..n-1. A zoomed-out view is wider
// than the planet, and walking every index outside the map would hang.
func tileSpan(minV, maxV, n int) (lo, hi int, ok bool) {
	if n < 1 {
		return 0, -1, false
	}
	if minV < 0 {
		minV = 0
	}
	if maxV > n-1 {
		maxV = n - 1
	}
	if minV > maxV {
		return 0, -1, false
	}
	return minV, maxV, true
}

// screenWorld is the mercator pixel under a screen pixel. Heading 0 keeps
// north toward the top of the screen. The car sits a bit below center.
// Pitch 0 is that flat mapping. A tilt is Mapbox's ground-plane camera,
// the same 50° pitch Pure Maps uses while navigating.
func (c mapCam) screenWorld(sx, sy, w, h, z int) (wx, wy float64) {
	return c.screenWorldF(float64(sx), float64(sy), w, h, z)
}

func (c mapCam) screenWorldF(sx, sy float64, w, h, z int) (wx, wy float64) {
	cx, cy := worldPx(c.lat, c.lon, z)
	scale := math.Pow(2, c.zoom-float64(z))
	if scale < 1e-6 {
		scale = 1
	}
	ry, xMul := rowMap(sy, float64(h), c.pitch)
	rx := (sx - float64(w)/2) * xMul
	sin, cos := math.Sincos(c.heading * math.Pi / 180)
	wx = cx + (rx*cos-ry*sin)/scale
	wy = cy + (rx*sin+ry*cos)/scale
	return wx, wy
}

// project is the screen pixel of a ground point. ok is false past the horizon.
func (c mapCam) project(lat, lon float64, w, h int) (sx, sy float64, ok bool) {
	if w < 1 || h < 1 {
		return 0, 0, false
	}
	z := c.sampleZoom()
	wx, wy := worldPx(lat, lon, z)
	cx, cy := worldPx(c.lat, c.lon, z)
	scale := math.Pow(2, c.zoom-float64(z))
	if scale < 1e-6 {
		scale = 1
	}
	dx, dy := wx-cx, wy-cy
	sin, cos := math.Sincos(c.heading * math.Pi / 180)
	rx := scale * (dx*cos + dy*sin)
	ry := scale * (-dx*sin + dy*cos)
	var xMul float64
	sy, xMul, ok = unrow(ry, float64(h), c.pitch)
	if !ok || math.Abs(xMul) < 1e-3 {
		return 0, 0, false
	}
	sx = float64(w)/2 + rx/xMul
	return sx, sy, true
}

// rowMap is the ground offset of a screen row, in flat screen pixels, and
// the lateral scale of that row. Pitch 0 is linear: the car row is the
// anchor and one screen pixel is one flat pixel.
//
// A tilt is a pinhole camera orbiting the car, pitched up to camTiltDeg
// from straight down, with Mapbox's field of view. Parallel roads stay
// straight and meet at one vanishing point. The horizon stays above the
// screen, which is how Pure Maps looks at 50°.
func rowMap(sy, h, pitch float64) (ry, xMul float64) {
	anchor := h * camAnchor
	dy := sy - anchor
	if h < 1 || pitch < 1e-4 {
		return dy, 1
	}
	sin, cos := math.Sincos(pitch * camTiltDeg * math.Pi / 180)
	focal := 0.5 / camFovTan * h
	denom := dy*sin + focal*cos
	if denom < focal*1e-3 {
		denom = focal * 1e-3
	}
	t := focal * cos / denom
	ry = focal*sin + t*(dy*cos-focal*sin)
	return ry, t
}

// unrow is the screen row whose ground offset is ry. It is the inverse of
// rowMap, so the car can be drawn back onto a view the finger has panned.
func unrow(ry, h, pitch float64) (sy, xMul float64, ok bool) {
	anchor := h * camAnchor
	if h < 1 || pitch < 1e-4 {
		return ry + anchor, 1, true
	}
	sin, cos := math.Sincos(pitch * camTiltDeg * math.Pi / 180)
	focal := 0.5 / camFovTan * h
	if focal < 1 {
		return ry + anchor, 1, true
	}
	t := 1 - ry*sin/focal
	if t < 1e-3 {
		return 0, 0, false
	}
	dy := ry
	if math.Abs(sin) >= 1e-6 {
		dy = focal * cos * (1 - t) / (t * sin)
	}
	return dy + anchor, t, true
}

func normDeg(d float64) float64 {
	d = math.Mod(d, 360)
	if d < 0 {
		d += 360
	}
	return d
}

// angDelta is the shortest turn from `from` to `to`, in -180..180.
func angDelta(from, to float64) float64 {
	d := math.Mod(to-from, 360)
	if d > 180 {
		d -= 360
	}
	if d < -180 {
		d += 360
	}
	return d
}

func metersBetween(lat1, lon1, lat2, lon2 float64) float64 {
	const earth = 6371000.0
	r := func(d float64) float64 { return d * math.Pi / 180 }
	φ1, φ2 := r(lat1), r(lat2)
	dφ := r(lat2 - lat1)
	dλ := r(lon2 - lon1)
	h := math.Sin(dφ/2)*math.Sin(dφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(dλ/2)*math.Sin(dλ/2)
	return 2 * earth * math.Asin(math.Min(1, math.Sqrt(h)))
}
