package main

import (
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

const (
	mapOSM    = "osm"
	mapGlitch = "glitch"
)

func normalizeMap(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "glitch", "glitchy":
		return mapGlitch
	default:
		return mapOSM
	}
}

func mapLabel(kind string) string {
	if normalizeMap(kind) == mapGlitch {
		return "Glitch"
	}
	return "OSM"
}

func rendererLabel(kind string) string {
	if normalizeRenderer(kind) == rendererMapLibre {
		return "MapLibre"
	}
	return "Vector"
}

func (s *drive) newMapRaster() fyne.CanvasObject {
	// The drive map is sampled under the chrome on each frame. This raster
	// stays hidden so a Fyne capture does not bake a stale bitmap.
	r := canvas.NewRaster(func(w, h int) image.Image {
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		return image.NewRGBA(image.Rect(0, 0, w, h))
	})
	r.Hide()
	s.mapRaster = r
	return r
}

// stepMap eases the camera. A new plate is posted only when the one we
// have no longer covers the view. The frame itself samples the plate.
func (s *drive) stepMap(now time.Time) {
	s.mu.Lock()
	if s.mapKind == "" {
		s.mapKind = normalizeMap(loadMap())
	}
	kind := s.mapKind
	w, h := s.w, s.h
	s.mu.Unlock()

	fix := s.gpsSnapshot()
	s.mu.Lock()
	s.cam.tilt = s.mapTilt
	s.cam.ahead = s.mapAhead
	s.cam.step(now, fix)
	showZoom := s.cam.showZoom()
	s.mapNow = now
	cam := s.cam
	s.mu.Unlock()
	s.syncMapZoom(showZoom)
	if kind == mapGlitch || !cam.has || s.eng().Down() {
		return
	}
	s.postMap(kind, cam, w, h, now)
}

// panMap follows a finger on the drive map. A short move stays a tap.
// Past that, the view leaves the car and − / + appear.
func (s *drive) panMap(e *fyne.DragEvent) {
	if e == nil {
		return
	}
	pre := e.AbsolutePosition.Subtract(fyne.NewPos(e.Dragged.DX, e.Dragged.DY))
	s.mu.Lock()
	fresh := !s.mapGrabOn || posDist2(pre, fyne.NewPos(s.mapGrabLastX, s.mapGrabLastY)) > 1
	if fresh {
		s.mapGrabOn = true
		s.mapGrabMoved = false
		s.mapGrabX, s.mapGrabY = pre.X, pre.Y
	}
	s.mapGrabLastX, s.mapGrabLastY = e.AbsolutePosition.X, e.AbsolutePosition.Y
	if !s.mapGrabMoved {
		if posDist2(e.AbsolutePosition, fyne.NewPos(s.mapGrabX, s.mapGrabY)) < 64 {
			s.mu.Unlock()
			return
		}
		s.mapGrabMoved = true
		ok := s.applyPan(fyne.NewPos(s.mapGrabX, s.mapGrabY), e.AbsolutePosition)
		s.mu.Unlock()
		if ok {
			s.syncMapZoom(true)
		}
		return
	}
	from := e.AbsolutePosition.Subtract(fyne.NewPos(e.Dragged.DX, e.Dragged.DY))
	ok := s.applyPan(from, e.AbsolutePosition)
	s.mu.Unlock()
	if ok {
		s.syncMapZoom(true)
	}
}

func (s *drive) applyPan(from, to fyne.Position) bool {
	sc := s.uiScale
	w, h := s.w, s.h
	if sc < 0.05 {
		sc = 1
	}
	if !s.cam.has || w < 1 || h < 1 {
		s.mapGrabMoved = false
		return false
	}
	s.cam.pan(float64(to.X*sc), float64(to.Y*sc), float64((to.X-from.X)*sc), float64((to.Y-from.Y)*sc), w, h)
	s.cam.browsing = true
	s.cam.returning = false
	s.cam.dragging = true
	s.cam.browseUntil = s.holdUntil()
	return true
}

// holdUntil is the browse deadline on the same clock the frame uses.
func (s *drive) holdUntil() time.Time {
	base := s.mapNow
	if base.IsZero() {
		base = time.Now()
	}
	return base.Add(camBrowseHold)
}

func (s *drive) endMapDrag() {
	s.mu.Lock()
	moved := s.mapGrabMoved
	s.mapGrabOn = false
	s.mapGrabMoved = false
	if moved {
		s.cam.dragging = false
		s.cam.browseUntil = s.holdUntil()
	}
	s.mu.Unlock()
}

func (s *drive) nudgeMapZoom(delta float64) {
	s.mu.Lock()
	if !s.cam.has || !s.cam.showZoom() {
		s.mu.Unlock()
		return
	}
	s.cam.browsing = true
	s.cam.returning = false
	s.cam.dragging = false
	s.cam.nudgeZoom(delta)
	s.cam.browseUntil = s.holdUntil()
	s.mu.Unlock()
	s.bump()
}

func (s *drive) syncMapZoom(on bool) {
	s.mu.Lock()
	box := s.zoomBox
	s.mu.Unlock()
	if box == nil {
		return
	}
	if on && !box.Visible() {
		box.Show()
		s.markChrome()
		return
	}
	if !on && box.Visible() {
		box.Hide()
		s.markChrome()
	}
}

func posDist2(a, b fyne.Position) float32 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx*dx + dy*dy
}

// blitDriveMap paints the map for this frame. Sampling a plate is the
// whole per-frame cost; vectors are not drawn here.
func (s *drive) blitDriveMap(dst *image.RGBA, now time.Time) {
	s.mu.Lock()
	kind := s.mapKind
	if kind == "" {
		kind = mapOSM
	}
	cam := s.cam
	s.mu.Unlock()
	if kind == mapGlitch || (cam.has && s.eng().Down()) {
		paintGlitch(dst, now)
		return
	}
	b := dst.Bounds()
	if !cam.has || !s.eng().DrawFrame(dst, renderCam(cam), s.frameMarks(), now) {
		paintLoadGlitch(dst, now)
		paintCarArrow(dst, cam, b)
		return
	}
}

type mapJob struct {
	kind  string
	cam   mapCam
	w, h  int
	now   time.Time
	stamp string
	style string
}

func (s *drive) kickMap() {
	s.mu.Lock()
	if !s.mapRunning {
		stop := make(chan struct{})
		wake := make(chan struct{}, 1)
		done := make(chan struct{})
		s.mapStop = stop
		s.mapWake = wake
		s.mapDone = done
		s.mapRunning = true
		go s.mapLoop(stop, wake, done)
	}
	wake := s.mapWake
	s.mu.Unlock()
	if wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

func (s *drive) stopMap() {
	s.mu.Lock()
	if !s.mapRunning {
		s.mu.Unlock()
		return
	}
	close(s.mapStop)
	done := s.mapDone
	s.mapRunning = false
	s.mu.Unlock()
	<-done
}

func (s *drive) mapLoop(stop <-chan struct{}, wake <-chan struct{}, done chan struct{}) {
	defer close(done)
	for {
		select {
		case <-stop:
			return
		case <-wake:
			if !s.paintLatest(stop) {
				return
			}
		}
	}
}

func (s *drive) paintLatest(stop <-chan struct{}) bool {
	for {
		select {
		case <-stop:
			return false
		default:
		}
		s.mu.Lock()
		job := s.mapJob
		ready := job.stamp != "" && s.mapStamp == job.stamp
		s.mu.Unlock()
		if ready || job.w < 1 || job.h < 1 || job.stamp == "" {
			return true
		}
		eng := s.eng()
		built := eng.Prepare(renderJob(job))
		s.mu.Lock()
		still := s.mapJob.stamp == job.stamp && s.mapJob.w == job.w && s.mapJob.h == job.h && s.engine == eng
		if still && built.Ready {
			eng.Publish(built, job.stamp)
			s.mapStamp = job.stamp
			if built.ZoomMiss {
				s.cam.noteZoomMiss(built.DrawnZoom, job.cam.lat, job.cam.lon)
			}
		}
		s.mu.Unlock()
		if !still {
			continue
		}
		if built.Ready {
			return true
		}
		// Tiles are not in yet. Keep the loading glitch up and try again.
		select {
		case <-stop:
			return false
		case <-time.After(40 * time.Millisecond):
		}
	}
}

func (s *drive) setMapMode(kind string) error {
	kind = normalizeMap(kind)
	s.mu.Lock()
	same := s.mapKind == kind
	s.mu.Unlock()
	if same {
		return nil
	}
	if err := patchMap(userConfigPath(), kind); err != nil {
		s.logf("map: %v", err)
		return err
	}
	s.mu.Lock()
	s.mapKind = kind
	s.mapStamp = ""
	s.mapJob = mapJob{}
	w, h := s.w, s.h
	cam := s.cam
	now := s.mapNow
	s.mu.Unlock()
	if kind == mapGlitch {
		s.eng().Pause()
	} else {
		s.postMap(kind, cam, w, h, now)
	}
	s.bump()
	return nil
}

func (s *drive) setMapStyle(kind string) error {
	id := normalizeStyle(kind)
	s.mu.Lock()
	same := s.mapStyle == id
	s.mu.Unlock()
	if same {
		return nil
	}
	if err := patchStyle(id); err != nil {
		s.logf("style: %v", err)
		return err
	}
	setScoutStyle(id)
	liveAtlas.drop()
	s.mu.Lock()
	s.mapStyle = id
	s.mapStamp = ""
	s.mapJob = mapJob{}
	w, h := s.w, s.h
	cam := s.cam
	now := s.mapNow
	mapKind := s.mapKind
	s.mu.Unlock()
	if mapKind != mapGlitch {
		s.postMap(mapKind, cam, w, h, now)
	}
	s.bump()
	return nil
}

func (s *drive) setRenderer(kind string) error {
	kind = normalizeRenderer(kind)
	s.mu.Lock()
	same := rendererOf(s.engine) == kind
	s.mu.Unlock()
	if same {
		return nil
	}
	if err := patchRenderer(kind); err != nil {
		s.logf("renderer: %v", err)
		return err
	}
	next := newMapEngine(kind)
	s.mu.Lock()
	old := s.engine
	s.engine = next
	s.mapStamp = ""
	s.mapJob = mapJob{}
	w, h := s.w, s.h
	cam := s.cam
	now := s.mapNow
	mapKind := s.mapKind
	s.mu.Unlock()
	if old != nil {
		old.Pause()
	}
	if mapKind != mapGlitch {
		s.postMap(mapKind, cam, w, h, now)
	}
	s.bump()
	return nil
}

func (s *drive) rendererKind() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return rendererOf(s.engine)
}

func rendererOf(e mapEngine) string {
	if e == nil {
		return rendererVector
	}
	if e.ID() == rendererMapLibre {
		return rendererMapLibre
	}
	return rendererVector
}

func (s *drive) eng() mapEngine {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		s.engine = newMapEngine(rendererVector)
	}
	return s.engine
}

func (s *drive) setMapTilt(mode string) error {
	mode = normalizeTilt(mode)
	s.mu.Lock()
	same := s.mapTilt == mode
	s.mu.Unlock()
	if same {
		return nil
	}
	if err := patchTilt(userConfigPath(), mode); err != nil {
		s.logf("map tilt: %v", err)
		return err
	}
	s.mu.Lock()
	s.mapTilt = mode
	s.mu.Unlock()
	s.bump()
	return nil
}

func (s *drive) setMapAhead(scale float64) error {
	scale = clampAhead(scale)
	s.mu.Lock()
	same := s.mapAhead == scale
	s.mu.Unlock()
	if same {
		return nil
	}
	if err := patchAhead(userConfigPath(), scale); err != nil {
		s.logf("map ahead: %v", err)
		return err
	}
	s.mu.Lock()
	s.mapAhead = scale
	s.mu.Unlock()
	s.bump()
	return nil
}

func (s *drive) postMap(kind string, cam mapCam, w, h int, now time.Time) {
	if w < 1 || h < 1 || kind != mapOSM || !cam.has {
		return
	}
	s.mu.Lock()
	style := s.mapStyle
	s.mu.Unlock()
	if style == "" {
		style = styleOSMBright
	}
	stamp := s.eng().JobStamp(renderCam(cam), style, w, h)
	if stamp == "" {
		return
	}
	s.mu.Lock()
	if s.mapJob.stamp == stamp && s.mapJob.w == w && s.mapJob.h == h {
		s.mu.Unlock()
		return
	}
	s.mapJob = mapJob{kind: kind, cam: cam, w: w, h: h, now: now, stamp: stamp, style: style}
	s.mu.Unlock()
	s.kickMap()
}

func buildDrivePlate(job mapJob) *mapPlate {
	_, keys, _, _, _, _ := plateTiles(job.cam, job.w, job.h)
	geoms, failed := liveAtlas.prepare(keys)
	if len(keys) > 0 && len(geoms) == 0 {
		if !failed {
			liveAtlas.noteWaiting()
		}
		return nil
	}
	return renderPlate(job.cam, job.w, job.h, geoms)
}

func renderMap(kind string, cam mapCam, w, h int, now time.Time) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if w < 1 || h < 1 {
		return dst
	}
	if kind == mapGlitch {
		paintGlitch(dst, now)
		return dst
	}
	if !cam.has {
		paintLoadGlitch(dst, now)
		return dst
	}
	_, keys, _, _, _, _ := plateTiles(cam, w, h)
	geoms, failed := liveAtlas.prepare(keys)
	if len(keys) > 0 && len(geoms) == 0 && !failed {
		liveAtlas.noteWaiting()
		paintDriveMap(dst, mapOSM, cam, nil, false, now)
		return dst
	}
	if failed && len(geoms) == 0 {
		paintGlitch(dst, now)
		return dst
	}
	paintDriveMap(dst, mapOSM, cam, renderPlate(cam, w, h, geoms), false, now)
	return dst
}

// paintDriveMap is one video frame of the map. The plate is already painted;
// this only samples it, then puts the arrow back in screen space.
func paintDriveMap(dst *image.RGBA, kind string, cam mapCam, plate *mapPlate, down bool, now time.Time) {
	if dst == nil {
		return
	}
	if kind == mapGlitch || down {
		paintGlitch(dst, now)
		return
	}
	b := dst.Bounds()
	if !cam.has || plate == nil {
		paintLoadGlitch(dst, now)
		paintCarArrow(dst, cam, b)
		return
	}
	samplePlate(dst, plate, cam, now)
	paintCarArrow(dst, cam, b)
}

// paintCarArrow draws the car. While the view follows, that is the anchor.
// A drag leaves the arrow on the GPS point, which can leave the screen.
func paintCarArrow(dst *image.RGBA, cam mapCam, b image.Rectangle) {
	if !cam.has {
		return
	}
	w, h := b.Dx(), b.Dy()
	ax := w / 2
	ay := int(float64(h) * camAnchor)
	if cam.browsing && cam.hasCar {
		sx, sy, ok := cam.project(cam.carLat, cam.carLon, w, h)
		if !ok {
			return
		}
		ax = int(math.Round(sx))
		ay = int(math.Round(sy))
		if ax < -48 || ay < -48 || ax >= w+48 || ay >= h+48 {
			return
		}
	}
	paintArrow(dst, b.Min.X+ax, b.Min.Y+ay)
}

// paintView draws vector tiles in screen space. A 256px bitmap threw away
// the tile's 4096-unit geometry, and the close zoom then stretched those pixels.
func paintView(dst *image.RGBA, cam mapCam, geoms map[tileKey]*mapGeom) {
	b := dst.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return
	}
	fillRect(dst, b, colLand)
	if len(geoms) == 0 {
		return
	}
	z := cam.sampleZoom()
	_, keys := cam.viewTiles(w, h)
	xf := newScreenXF(cam, w, h, z)
	for _, k := range tilesToPaint(keys, geoms) {
		g := geoms[k]
		if g == nil {
			continue
		}
		paintGeom(dst, xf, k, g)
	}
}

type screenXF struct {
	z        int
	cx, cy   float64
	scale    float64
	sin, cos float64
	ox, oy   float64
}

func newScreenXF(cam mapCam, w, h, z int) screenXF {
	cx, cy := worldPx(cam.lat, cam.lon, z)
	scale := math.Pow(2, cam.zoom-float64(z))
	if scale < 1e-6 {
		scale = 1
	}
	s, c := math.Sincos(cam.heading * math.Pi / 180)
	return screenXF{
		z: z, cx: cx, cy: cy, scale: scale,
		sin: s, cos: c,
		ox: float64(w) / 2, oy: float64(h) * camAnchor,
	}
}

func (xf screenXF) point(k tileKey, p pt2) (x, y float64) {
	mul := math.Pow(2, float64(xf.z-k.z))
	wx := (float64(k.x) + float64(p.x)) * tilePx * mul
	wy := (float64(k.y) + float64(p.y)) * tilePx * mul
	dx, dy := wx-xf.cx, wy-xf.cy
	rx := xf.scale * (dx*xf.cos + dy*xf.sin)
	ry := xf.scale * (-dx*xf.sin + dy*xf.cos)
	return xf.ox + rx, xf.oy + ry
}

func (xf screenXF) span(k tileKey) float64 {
	return xf.scale * math.Pow(2, float64(xf.z-k.z))
}

// tilesToPaint picks the sharpest cached tile under each view cell.
// Coarser tiles go first so a finer one can cover them.
func tilesToPaint(keys []tileKey, geoms map[tileKey]*mapGeom) []tileKey {
	seen := map[tileKey]bool{}
	var list []tileKey
	for _, k := range keys {
		for step := 0; step <= 6 && k.z-step >= 2; step++ {
			pk := tileKey{z: k.z - step, x: k.x >> step, y: k.y >> step}
			if geoms[pk] == nil || geoms[pk].blank() {
				continue
			}
			if !seen[pk] {
				list = append(list, pk)
				seen[pk] = true
			}
			break
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].z < list[j].z })
	return list
}

func paintGeom(dst *image.RGBA, xf screenXF, k tileKey, g *mapGeom) {
	if g == nil {
		return
	}
	b := dst.Bounds()
	span := xf.span(k)
	for i := range g.polys {
		p := &g.polys[i]
		rings := make([][][2]float64, 0, len(p.rings))
		minX, minY := math.MaxFloat64, math.MaxFloat64
		maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
		for _, ring := range p.rings {
			if len(ring) < 3 {
				continue
			}
			out := make([][2]float64, len(ring))
			for i, pt := range ring {
				x, y := xf.point(k, pt)
				out[i] = [2]float64{x, y}
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
			rings = append(rings, out)
		}
		if len(rings) == 0 || maxX < float64(b.Min.X) || maxY < float64(b.Min.Y) || minX > float64(b.Max.X) || minY > float64(b.Max.Y) {
			continue
		}
		fillPolyPx(dst, rings, p.ink)
	}
	for i := range g.lines {
		ln := &g.lines[i]
		thick := int(math.Round(float64(lineThick(ln.rank)) * span))
		if thick < 1 {
			thick = 1
		}
		for i := 1; i < len(ln.pts); i++ {
			x0, y0 := xf.point(k, ln.pts[i-1])
			x1, y1 := xf.point(k, ln.pts[i])
			strokeSegPx(dst, x0, y0, x1, y1, thick, ln.ink)
		}
	}
}

func paintArrow(dst *image.RGBA, cx, cy int) {
	fillTriPx(dst, float64(cx), float64(cy-20), float64(cx-16), float64(cy+14), float64(cx+16), float64(cy+14), colArrowEdge)
	fillTriPx(dst, float64(cx), float64(cy-16), float64(cx-12), float64(cy+10), float64(cx+12), float64(cy+10), colLime)
}

func fillTriPx(dst *image.RGBA, x0, y0, x1, y1, x2, y2 float64, c color.NRGBA) {
	fillPolyPx(dst, [][][2]float64{{{x0, y0}, {x1, y1}, {x2, y2}, {x0, y0}}}, c)
}

func glitchTick(now time.Time) int64 {
	return glitchTickAt(now, 1)
}

// glitchTickAt steps the field. speed 1 changes every 80ms; a slower speed
// stretches that step so the same pattern lingers.
func glitchTickAt(now time.Time, speed float64) int64 {
	if speed <= 0 {
		speed = 1
	}
	step := int64(math.Round(80 / speed))
	if step < 1 {
		step = 1
	}
	t := now.UnixMilli() / step
	if t < 0 {
		t = -t
	}
	return t
}

func mix64(n uint64) uint64 {
	n ^= n >> 30
	n *= 0xbf58476d1ce4e5b9
	n ^= n >> 27
	n *= 0x94d049bb133111eb
	n ^= n >> 31
	return n
}

// glitchAt is one pixel of the scan-line field. Streaks are short horizontal
// runs, not full-width bars, and a few of them are two pixels tall.
func glitchAt(x, y, w int, tick int64) color.NRGBA {
	if c, ok := glitchRow(x, y, w, tick); ok {
		return c
	}
	if y > 0 {
		if c, ok := glitchRow(x, y-1, w, tick); ok && glitchThick(y-1, tick) {
			return c
		}
	}
	row := mix64(uint64(uint32(y)) ^ uint64(tick)*0x9E3779B185EBCA87)
	n := mix64(uint64(uint32(x)) ^ row)
	if n%70 == 0 {
		return color.NRGBA{R: 16, G: 26, B: 42, A: 255}
	}
	return colGlitchBG
}

func glitchThick(y int, tick int64) bool {
	row := mix64(uint64(uint32(y)) ^ uint64(tick)*0x9E3779B185EBCA87)
	return row%5 <= 1 && row%9 == 0
}

func glitchRow(x, y, w int, tick int64) (color.NRGBA, bool) {
	var ink color.NRGBA
	hit := false
	glitchWalk(y, w, tick, func(x0, x1 int, c color.NRGBA) {
		if !hit && x >= x0 && x < x1 {
			ink = c
			hit = true
		}
	})
	return ink, hit
}

// glitchWalk visits the streak pieces on one row. Dashed runs are already split.
func glitchWalk(y, w int, tick int64, fn func(x0, x1 int, c color.NRGBA)) {
	glitchWalkAt(y, w, tick, 1, fn)
}

func glitchWalkAt(y, w int, tick int64, density float64, fn func(x0, x1 int, c color.NRGBA)) {
	if fn == nil {
		return
	}
	if w < 1 {
		w = 1
	}
	row := mix64(uint64(uint32(y)) ^ uint64(tick)*0x9E3779B185EBCA87)
	// About two rows in five, picked by the hash so the gaps are uneven.
	if row%5 > 1 {
		return
	}
	if density < 1 {
		keep := int(density*1000 + 0.5)
		if keep < 1 {
			keep = 1
		}
		if int((row>>17)%1000) >= keep {
			return
		}
	}
	n := 1 + int(row>>10)%3
	for s := 0; s < n; s++ {
		hs := mix64(row ^ uint64(s+1)*0xBF58476D1CE4E5B9)
		var length int
		switch hs % 6 {
		case 0:
			length = w/3 + int(hs>>23)%(w/2+1)
		case 1, 2:
			length = 6 + int(hs>>17)%(w/14+8)
		default:
			length = w/20 + int(hs>>17)%(w/6+1)
		}
		if length > w {
			length = w
		}
		x0 := int(hs>>8) % w
		x1 := x0 + length
		if x1 > w {
			x1 = w
		}
		c := glitchPalette[int(hs%uint64(len(glitchPalette)))]
		if hs%7 != 0 {
			fn(x0, x1, c)
			continue
		}
		for x := x0; x < x1; {
			run := 3
			if x+run > x1 {
				run = x1 - x
			}
			fn(x, x+run, c)
			x += run + 3
		}
	}
}

func paintGlitchField(dst *image.RGBA, now time.Time) {
	paintGlitchStyled(dst, now, 1, 1)
}

// paintLoadGlitch is the field while the first tiles are still coming in.
// A fifth of the streaks, stepping at two fifths of the usual rate.
func paintLoadGlitch(dst *image.RGBA, now time.Time) {
	paintGlitchStyled(dst, now, 0.2, 0.4)
}

func paintGlitchStyled(dst *image.RGBA, now time.Time, density, speed float64) {
	if dst == nil {
		return
	}
	b := dst.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return
	}
	fillRect(dst, b, colGlitchBG)
	tick := glitchTickAt(now, speed)
	speck := color.NRGBA{R: 16, G: 26, B: 42, A: 255}
	speckKeep := 1000
	if density < 1 {
		speckKeep = int(density*1000 + 0.5)
		if speckKeep < 1 {
			speckKeep = 1
		}
	}
	for y := 0; y < h; y += 2 {
		row := mix64(uint64(uint32(y)) ^ uint64(tick))
		if density < 1 && int((row>>13)%1000) >= speckKeep {
			continue
		}
		for x := int(row % 9); x < w; x += 9 {
			if mix64(uint64(uint32(x))^row)%5 != 0 {
				continue
			}
			setNRGBA(dst, b.Min.X+x, b.Min.Y+y, speck)
		}
	}
	paintRow := func(srcY, dstY int) {
		glitchWalkAt(srcY, w, tick, density, func(x0, x1 int, c color.NRGBA) {
			fillRect(dst, image.Rect(b.Min.X+x0, b.Min.Y+dstY, b.Min.X+x1, b.Min.Y+dstY+1), c)
		})
	}
	for y := 0; y < h; y++ {
		if y > 0 && glitchThick(y-1, tick) {
			paintRow(y-1, y)
		}
		paintRow(y, y)
	}
}

func paintGlitch(dst *image.RGBA, now time.Time) {
	paintGlitchField(dst, now)
	if dst == nil {
		return
	}
	b := dst.Bounds()
	drawString(dst, "OSM unavailable", b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2, 2, colFG, true)
}
