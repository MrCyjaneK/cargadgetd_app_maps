package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"time"
)

// The vector picture is painted into a north-up plate, at most plateMaxPx
// on a side, and each video frame only samples that image. Pan, heading,
// zoom, and tilt are the sample transform, so they move on the frame clock.
// The plate is rebuilt when the view walks off it, the tile set changes,
// or a closer zoom would stretch it soft.

const (
	plateMaxPx   = 4096
	plateTileCap = 80
)

type mapPlate struct {
	img       *image.RGBA
	z         int
	originX   float64
	originY   float64
	scale     float64 // plate pixels per world pixel at z
	builtZoom float64
	capped    bool
	gen       int
	stamp     string
}

// covers reports whether sampling cam into this plate still shows the
// screen without walking off the image or stretching it soft.
func (p *mapPlate) covers(cam mapCam, w, h int) bool {
	if p == nil || p.img == nil || !cam.has || w < 1 || h < 1 {
		return false
	}
	if p.z != cam.sampleZoom() || p.gen != liveAtlas.generation() {
		return false
	}
	screen := math.Pow(2, cam.zoom-float64(p.z))
	if !p.capped && screen > p.scale*0.75 {
		return false
	}
	pw := p.img.Bounds().Dx()
	ph := p.img.Bounds().Dy()
	inset := 4
	corners := [4][2]int{{inset, inset}, {w - 1 - inset, inset}, {inset, h - 1 - inset}, {w - 1 - inset, h - 1 - inset}}
	for _, c := range corners {
		sx, sy := c[0], c[1]
		if sx < 0 {
			sx = 0
		}
		if sy < 0 {
			sy = 0
		}
		wx, wy := cam.screenWorld(sx, sy, w, h, p.z)
		px := (wx - p.originX) * p.scale
		py := (wy - p.originY) * p.scale
		if px < 2 || py < 2 || px >= float64(pw)-2 || py >= float64(ph)-2 {
			return false
		}
	}
	return true
}

func plateStamp(cam mapCam, w, h int) string {
	z := cam.sampleZoom()
	cx, cy := worldPx(cam.lat, cam.lon, z)
	cell := float64(tilePx) / 4
	return fmt.Sprintf("osm|%s|%d|%d|%d|%d|%d|%d|%dx%d|%d",
		currentScoutStyle(),
		z,
		int(math.Floor(cx/cell)),
		int(math.Floor(cy/cell)),
		int(math.Floor(cam.zoom*2)),
		int(normDeg(cam.heading)/10),
		int(cam.pitch*10),
		w, h,
		liveAtlas.generation(),
	)
}

// plateTiles is the neighbourhood around the view, nearest first, capped
// so one plate cannot ask for the whole city.
func plateTiles(cam mapCam, w, h int) (z int, keys []tileKey, minX, minY, maxX, maxY float64) {
	z = cam.sampleZoom()
	if w < 1 || h < 1 || !cam.has {
		return z, nil, 0, 0, 0, 0
	}
	screen := math.Pow(2, cam.zoom-float64(z))
	if screen < 1e-6 {
		screen = 1
	}
	short := w
	if h < short {
		short = h
	}
	pad := int(float64(short) * 0.45)
	minPad := int(math.Ceil(96 * screen))
	if pad < minPad {
		pad = minPad
	}
	minX, minY = math.MaxFloat64, math.MaxFloat64
	maxX, maxY = -math.MaxFloat64, -math.MaxFloat64
	// A flat pad is a few screen pixels of slack. Pushing that same pad
	// through a pitched camera walks off the top toward the horizon and
	// the plate then shrinks to fit, which softens the car.
	corners := [4][2]int{{-pad, -pad}, {w - 1 + pad, -pad}, {-pad, h - 1 + pad}, {w - 1 + pad, h - 1 + pad}}
	if cam.pitch >= 1e-4 {
		corners = [4][2]int{{0, 0}, {w - 1, 0}, {0, h - 1}, {w - 1, h - 1}}
	}
	for _, c := range corners {
		wx, wy := cam.screenWorld(c[0], c[1], w, h, z)
		if wx < minX {
			minX = wx
		}
		if wy < minY {
			minY = wy
		}
		if wx > maxX {
			maxX = wx
		}
		if wy > maxY {
			maxY = wy
		}
	}
	if cam.pitch >= 1e-4 {
		margin := float64(pad) / screen
		minX -= margin
		minY -= margin
		maxX += margin
		maxY += margin
	}
	n := 1 << z
	minTX := int(math.Floor(minX / tilePx))
	minTY := int(math.Floor(minY / tilePx))
	maxTX := int(math.Floor(maxX / tilePx))
	maxTY := int(math.Floor(maxY / tilePx))
	tx0, tx1, okX := tileSpan(minTX, maxTX, n)
	ty0, ty1, okY := tileSpan(minTY, maxTY, n)
	if !okX || !okY {
		return z, nil, 0, 0, 0, 0
	}
	cx, cy := worldPx(cam.lat, cam.lon, z)
	type cand struct {
		k    tileKey
		dist float64
	}
	var cs []cand
	for ty := ty0; ty <= ty1; ty++ {
		for tx := tx0; tx <= tx1; tx++ {
			mx := (float64(tx) + 0.5) * tilePx
			my := (float64(ty) + 0.5) * tilePx
			dx, dy := mx-cx, my-cy
			cs = append(cs, cand{tileKey{z: z, x: tx, y: ty}, dx*dx + dy*dy})
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].dist < cs[j].dist })
	if len(cs) > plateTileCap {
		cs = cs[:plateTileCap]
	}
	if len(cs) == 0 {
		return z, nil, 0, 0, 0, 0
	}
	minX, minY = math.MaxFloat64, math.MaxFloat64
	maxX, maxY = -math.MaxFloat64, -math.MaxFloat64
	keys = make([]tileKey, len(cs))
	for i, c := range cs {
		keys[i] = c.k
		x0 := float64(c.k.x) * tilePx
		y0 := float64(c.k.y) * tilePx
		x1 := x0 + tilePx
		y1 := y0 + tilePx
		if x0 < minX {
			minX = x0
		}
		if y0 < minY {
			minY = y0
		}
		if x1 > maxX {
			maxX = x1
		}
		if y1 > maxY {
			maxY = y1
		}
	}
	return z, keys, minX, minY, maxX, maxY
}

// renderPlate paints keys into a world-locked image. A tile with no geometry,
// including one that decoded empty, is left transparent so the sampler can
// glitch that square alone.
func renderPlate(cam mapCam, w, h int, geoms map[tileKey]*mapGeom) *mapPlate {
	z, keys, minX, minY, maxX, maxY := plateTiles(cam, w, h)
	if len(keys) == 0 || maxX <= minX || maxY <= minY {
		return nil
	}
	screen := math.Pow(2, cam.zoom-float64(z))
	if screen < 1e-6 {
		screen = 1
	}
	worldW := maxX - minX
	worldH := maxY - minY
	scale := screen * 2
	capped := false
	if worldW*scale > plateMaxPx {
		scale = plateMaxPx / worldW
		capped = true
	}
	if worldH*scale > plateMaxPx {
		if s := plateMaxPx / worldH; s < scale {
			scale = s
			capped = true
		}
	}
	pw := int(math.Ceil(worldW * scale))
	ph := int(math.Ceil(worldH * scale))
	if pw < 1 {
		pw = 1
	}
	if ph < 1 {
		ph = 1
	}
	if pw > plateMaxPx {
		pw = plateMaxPx
	}
	if ph > plateMaxPx {
		ph = plateMaxPx
	}
	dst := image.NewRGBA(image.Rect(0, 0, pw, ph))
	fillRect(dst, dst.Bounds(), colLand)
	p := &mapPlate{
		img: dst, z: z, originX: minX, originY: minY, scale: scale,
		builtZoom: cam.zoom, capped: capped, gen: liveAtlas.generation(),
	}
	xf := screenXF{z: z, cx: minX, cy: minY, scale: scale, cos: 1}
	for _, k := range tilesToPaint(keys, geoms) {
		paintGeom(dst, xf, k, geoms[k])
	}
	for _, k := range keys {
		if plateCovered(k, geoms) {
			continue
		}
		fillRect(dst, plateTileRect(p, k), color.NRGBA{})
	}
	return p
}

// plateCovered is false when this square should glitch: the tile itself
// decoded empty, or nothing in its parent chain has geometry.
func plateCovered(k tileKey, geoms map[tileKey]*mapGeom) bool {
	if g, ok := geoms[k]; ok {
		return !g.blank()
	}
	for step := 1; step <= 6 && k.z-step >= 2; step++ {
		pk := tileKey{z: k.z - step, x: k.x >> step, y: k.y >> step}
		g, ok := geoms[pk]
		if !ok || g == nil {
			continue
		}
		return !g.blank()
	}
	return false
}

func plateTileRect(p *mapPlate, k tileKey) image.Rectangle {
	if p == nil {
		return image.Rectangle{}
	}
	mul := math.Pow(2, float64(p.z-k.z))
	x0 := (float64(k.x)*tilePx*mul - p.originX) * p.scale
	y0 := (float64(k.y)*tilePx*mul - p.originY) * p.scale
	x1 := (float64(k.x+1)*tilePx*mul - p.originX) * p.scale
	y1 := (float64(k.y+1)*tilePx*mul - p.originY) * p.scale
	return image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1)))
}

// samplePlate copies the plate through the live camera. Opaque plate pixels
// are the map. Anything else — a blank tile, or a view that has left the
// plate — is a glitch pixel, so a missing square does not stay black.
func samplePlate(dst *image.RGBA, p *mapPlate, cam mapCam, now time.Time) {
	if dst == nil {
		return
	}
	b := dst.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return
	}
	if p == nil || p.img == nil || !cam.has {
		paintGlitchField(dst, now)
		return
	}
	tick := glitchTick(now)
	z := p.z
	cx, cy := worldPx(cam.lat, cam.lon, z)
	screen := math.Pow(2, cam.zoom-float64(z))
	if screen < 1e-6 {
		screen = 1
	}
	inv := 1 / screen
	sin, cos := math.Sincos(cam.heading * math.Pi / 180)
	dwx := cos * inv
	dwy := sin * inv
	ps := p.scale
	dpx := dwx * ps
	dpy := dwy * ps
	flat := cam.pitch < 1e-4
	rx0 := -float64(w) / 2
	ry0 := -float64(h) * camAnchor
	wx := cx + (rx0*cos-ry0*sin)*inv
	wy := cy + (rx0*sin+ry0*cos)*inv
	px0 := (wx - p.originX) * ps
	py0 := (wy - p.originY) * ps
	rowDx := -sin * inv * ps
	rowDy := cos * inv * ps

	pp := p.img.Pix
	pw := p.img.Bounds().Dx()
	ph := p.img.Bounds().Dy()
	pstride := p.img.Stride
	for sy := 0; sy < h; sy++ {
		px, py := px0, py0
		if !flat {
			ry, xMul := rowMap(float64(sy), float64(h), cam.pitch)
			rx := -float64(w) / 2 * xMul
			step := xMul * inv * ps
			dpx = cos * step
			dpy = sin * step
			wx = cx + (rx*cos-ry*sin)*inv
			wy = cy + (rx*sin+ry*cos)*inv
			px = (wx - p.originX) * ps
			py = (wy - p.originY) * ps
		}
		row := dst.Pix[dst.PixOffset(b.Min.X, b.Min.Y+sy):]
		for sx := 0; sx < w; sx++ {
			o := sx * 4
			if !flat {
				if r, g, b, ok := plateBilinear(pp, pstride, pw, ph, px, py); ok {
					row[o] = r
					row[o+1] = g
					row[o+2] = b
					row[o+3] = 255
					px += dpx
					py += dpy
					continue
				}
			}
			ix := floorPx(px)
			iy := floorPx(py)
			if ix >= 0 && iy >= 0 && ix < pw && iy < ph {
				i := iy*pstride + ix*4
				if pp[i+3] != 0 {
					row[o] = pp[i]
					row[o+1] = pp[i+1]
					row[o+2] = pp[i+2]
					row[o+3] = 255
					px += dpx
					py += dpy
					continue
				}
			}
			c := glitchAt(sx, sy, w, tick)
			row[o] = c.R
			row[o+1] = c.G
			row[o+2] = c.B
			row[o+3] = 255
			px += dpx
			py += dpy
		}
		if flat {
			px0 += rowDx
			py0 += rowDy
		}
	}
}

// plateBilinear blends the four plate pixels around x, y. A transparent
// neighbour means this sample is a tile edge, and the caller keeps the
// nearest opaque pixel instead of mixing the map with a hole.
func plateBilinear(pp []byte, stride, pw, ph int, x, y float64) (byte, byte, byte, bool) {
	x0 := floorPx(x)
	y0 := floorPx(y)
	if x0 < 0 || y0 < 0 || x0 >= pw || y0 >= ph {
		return 0, 0, 0, false
	}
	x1, y1 := x0+1, y0+1
	fx := x - float64(x0)
	fy := y - float64(y0)
	if x1 >= pw {
		x1 = x0
		fx = 0
	}
	if y1 >= ph {
		y1 = y0
		fy = 0
	}
	i00 := y0*stride + x0*4
	i10 := y0*stride + x1*4
	i01 := y1*stride + x0*4
	i11 := y1*stride + x1*4
	if pp[i00+3] == 0 || pp[i10+3] == 0 || pp[i01+3] == 0 || pp[i11+3] == 0 {
		return 0, 0, 0, false
	}
	w00 := (1 - fx) * (1 - fy)
	w10 := fx * (1 - fy)
	w01 := (1 - fx) * fy
	w11 := fx * fy
	r := w00*float64(pp[i00]) + w10*float64(pp[i10]) + w01*float64(pp[i01]) + w11*float64(pp[i11])
	g := w00*float64(pp[i00+1]) + w10*float64(pp[i10+1]) + w01*float64(pp[i01+1]) + w11*float64(pp[i11+1])
	b := w00*float64(pp[i00+2]) + w10*float64(pp[i10+2]) + w01*float64(pp[i01+2]) + w11*float64(pp[i11+2])
	return byte(r + 0.5), byte(g + 0.5), byte(b + 0.5), true
}

func floorPx(v float64) int {
	i := int(v)
	if v < float64(i) {
		i--
	}
	return i
}
