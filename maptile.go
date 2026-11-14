package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/mvt"
)

// OSM Scout on the phone serves gzip Mapbox vector tiles. Raster tiles are
// not enabled there. Draw never calls this itself; the atlas goroutine does.

func scoutTileURL(x, y, z int) string {
	return fmt.Sprintf("http://127.0.0.1:8553/v1/mbgl/tile?style=%s&x=%d&y=%d&z=%d",
		currentScoutStyle(), x, y, z)
}

const (
	kindSkip = iota
	kindWater
	kindPark
	kindBuilding
	kindRoad
)

const (
	rankMinor = 1
	rankMid   = 2
	rankMajor = 3
)

var (
	colLand       = color.NRGBA{R: 10, G: 14, B: 16, A: 255}
	colWater      = color.NRGBA{R: 6, G: 36, B: 44, A: 255}
	colPark       = color.NRGBA{R: 12, G: 32, B: 26, A: 255}
	colBldg       = color.NRGBA{R: 24, G: 28, B: 32, A: 255}
	colRoadMinor  = color.NRGBA{R: 46, G: 58, B: 52, A: 255}
	colRoadMid    = color.NRGBA{R: 110, G: 140, B: 48, A: 255}
	colRoadMajor  = color.NRGBA{R: 150, G: 186, B: 32, A: 255}
	colGlitchBG   = color.NRGBA{R: 4, G: 8, B: 18, A: 255}
	colGlitchRed  = color.NRGBA{R: 255, G: 32, B: 64, A: 255}
	colGlitchBlue = color.NRGBA{R: 32, G: 72, B: 255, A: 255}
	colArrowEdge  = color.NRGBA{R: 8, G: 12, B: 8, A: 255}
)

// glitchPalette is the scan-line ink. The field stays the dark ground;
// streaks are short runs of these colours, the way a broken frame looks.
var glitchPalette = [...]color.NRGBA{
	colGlitchRed,
	{R: 255, G: 110, B: 24, A: 255},
	{R: 220, G: 196, B: 36, A: 255},
	{R: 64, G: 220, B: 48, A: 255},
	{R: 32, G: 210, B: 186, A: 255},
	colGlitchBlue,
}

type pt2 struct{ x, y float32 }

type mapLine struct {
	pts  []pt2
	rank int
	ink  color.NRGBA
}

type mapPoly struct {
	rings [][]pt2
	ink   color.NRGBA
}

type mapGeom struct {
	polys []mapPoly
	lines []mapLine
}

func decodeMVT(data []byte) (mapGeom, error) {
	if len(data) == 0 {
		return mapGeom{}, fmt.Errorf("empty tile")
	}
	layers, err := mvt.UnmarshalGzipped(data)
	if err != nil {
		layers, err = mvt.Unmarshal(data)
	}
	if err != nil {
		return mapGeom{}, err
	}
	var g mapGeom
	for _, layer := range layers {
		if layer == nil {
			continue
		}
		ext := float32(layer.Extent)
		if ext < 1 {
			ext = 4096
		}
		for _, f := range layer.Features {
			if f == nil || f.Geometry == nil {
				continue
			}
			kind, rank := classOf(layer.Name, map[string]interface{}(f.Properties))
			if kind == kindSkip {
				continue
			}
			addGeom(&g, kind, rank, f.Geometry, ext)
		}
	}
	return g, nil
}

func classOf(layer string, props map[string]interface{}) (kind, rank int) {
	name := strings.ToLower(layer)
	if strings.Contains(name, "name") || strings.Contains(name, "label") || strings.Contains(name, "poi") || strings.Contains(name, "housenum") {
		return kindSkip, 0
	}
	class := strings.ToLower(propStr(props, "class", "type", "kind", "highway", "subclass"))
	switch {
	case strings.Contains(name, "water"):
		return kindWater, 0
	case strings.Contains(name, "park") || strings.Contains(name, "landcover") || strings.Contains(name, "landuse") || strings.Contains(name, "leisure"):
		if strings.Contains(name, "park") || parkClass(class) {
			return kindPark, 0
		}
		return kindSkip, 0
	case strings.Contains(name, "building"):
		return kindBuilding, 0
	case strings.Contains(name, "transport") || name == "road" || name == "roads" || strings.Contains(name, "highway") || name == "street" || name == "streets":
		return kindRoad, roadRank(class)
	default:
		return kindSkip, 0
	}
}

func parkClass(class string) bool {
	switch class {
	case "park", "grass", "wood", "forest", "meadow", "garden", "recreation_ground", "scrub", "wetland", "pitch", "cemetery":
		return true
	default:
		return false
	}
}

func roadRank(class string) int {
	switch class {
	case "motorway", "trunk", "primary", "motorway_link", "trunk_link", "primary_link":
		return rankMajor
	case "secondary", "tertiary", "secondary_link", "tertiary_link":
		return rankMid
	default:
		return rankMinor
	}
}

func propStr(p map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		v, ok := p[k]
		if !ok || v == nil {
			continue
		}
		s, ok := v.(string)
		if ok && s != "" {
			return s
		}
	}
	return ""
}

func addGeom(g *mapGeom, kind, rank int, geom orb.Geometry, extent float32) {
	switch t := geom.(type) {
	case orb.LineString:
		g.addLine(kind, rank, ptsOf(t, extent))
	case orb.MultiLineString:
		for _, ls := range t {
			g.addLine(kind, rank, ptsOf(ls, extent))
		}
	case orb.Polygon:
		if kind == kindRoad {
			for _, r := range t {
				g.addLine(kind, rank, ptsOf(orb.LineString(r), extent))
			}
			return
		}
		var rings [][]pt2
		for _, r := range t {
			rings = append(rings, closeRing(ptsOf(orb.LineString(r), extent)))
		}
		g.addPoly(kind, rings)
	case orb.MultiPolygon:
		for _, p := range t {
			addGeom(g, kind, rank, p, extent)
		}
	}
}

// blank is a tile we decoded and that has nothing to draw. The square is
// the land colour, which reads as a missing tile.
func (g *mapGeom) blank() bool {
	return g == nil || (len(g.polys) == 0 && len(g.lines) == 0)
}

func (g *mapGeom) addLine(kind, rank int, pts []pt2) {
	if len(pts) < 2 {
		return
	}
	ink := colRoadMinor
	thickRank := rank
	switch kind {
	case kindWater:
		ink = colWater
		thickRank = rankMinor
	case kindRoad:
		switch rank {
		case rankMajor:
			ink = colRoadMajor
		case rankMid:
			ink = colRoadMid
		default:
			ink = colRoadMinor
			thickRank = rankMinor
		}
	default:
		return
	}
	g.lines = append(g.lines, mapLine{pts: pts, rank: thickRank, ink: ink})
}

func (g *mapGeom) addPoly(kind int, rings [][]pt2) {
	ink := colLand
	switch kind {
	case kindWater:
		ink = colWater
	case kindPark:
		ink = colPark
	case kindBuilding:
		ink = colBldg
	default:
		return
	}
	g.polys = append(g.polys, mapPoly{rings: rings, ink: ink})
}

func ptsOf(ls orb.LineString, extent float32) []pt2 {
	if extent < 1 {
		extent = 4096
	}
	out := make([]pt2, 0, len(ls))
	for _, p := range ls {
		out = append(out, pt2{x: float32(p[0]) / extent, y: float32(p[1]) / extent})
	}
	return out
}

func closeRing(pts []pt2) []pt2 {
	if len(pts) < 2 {
		return pts
	}
	a, b := pts[0], pts[len(pts)-1]
	if a.x == b.x && a.y == b.y {
		return pts
	}
	out := make([]pt2, len(pts)+1)
	copy(out, pts)
	out[len(pts)] = a
	return out
}

func paintTile(g mapGeom) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, tilePx, tilePx))
	fillRect(dst, dst.Bounds(), colLand)
	for _, p := range g.polys {
		fillRings(dst, p.rings, p.ink)
	}
	for _, ln := range g.lines {
		strokeChain(dst, ln.pts, lineThick(ln.rank), ln.ink)
	}
	return dst
}

func lineThick(rank int) int {
	switch rank {
	case rankMajor:
		return 5
	case rankMid:
		return 3
	default:
		return 2
	}
}

func strokeChain(dst *image.RGBA, pts []pt2, thick int, col color.NRGBA) {
	for i := 1; i < len(pts); i++ {
		strokeSeg(dst, pts[i-1], pts[i], thick, col)
	}
}

func strokeSeg(dst *image.RGBA, a, b pt2, thick int, col color.NRGBA) {
	bds := dst.Bounds()
	strokeSegPx(dst,
		float64(a.x)*float64(bds.Dx()), float64(a.y)*float64(bds.Dy()),
		float64(b.x)*float64(bds.Dx()), float64(b.y)*float64(bds.Dy()),
		thick, col)
}

func strokeSegPx(dst *image.RGBA, x0, y0, x1, y1 float64, thick int, col color.NRGBA) {
	if thick < 1 {
		thick = 1
	}
	b := dst.Bounds()
	margin := float64(thick) + 1
	x0, y0, x1, y1, ok := clipLine(x0, y0, x1, y1,
		float64(b.Min.X)-margin, float64(b.Min.Y)-margin,
		float64(b.Max.X-1)+margin, float64(b.Max.Y-1)+margin)
	if !ok {
		return
	}
	dx, dy := x1-x0, y1-y0
	n := math.Hypot(dx, dy)
	if n < 0.5 {
		setNRGBA(dst, int(math.Round(x0)), int(math.Round(y0)), col)
		return
	}
	px, py := -dy/n, dx/n
	half := float64(thick-1) / 2
	steps := int(n)
	if steps < 1 {
		steps = 1
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x0 + dx*t
		y := y0 + dy*t
		for s := -half; s <= half+0.01; s++ {
			setNRGBA(dst, int(math.Round(x+px*s)), int(math.Round(y+py*s)), col)
		}
	}
}

// clipLine clips a segment to an inclusive rectangle. It drops segments that miss it.
func clipLine(x0, y0, x1, y1, minX, minY, maxX, maxY float64) (float64, float64, float64, float64, bool) {
	dx, dy := x1-x0, y1-y0
	u0, u1 := 0.0, 1.0
	cut := func(p, q float64) bool {
		if p == 0 {
			return q >= 0
		}
		r := q / p
		if p < 0 {
			if r > u1 {
				return false
			}
			if r > u0 {
				u0 = r
			}
			return true
		}
		if r < u0 {
			return false
		}
		if r < u1 {
			u1 = r
		}
		return true
	}
	if !cut(-dx, x0-minX) || !cut(dx, maxX-x0) || !cut(-dy, y0-minY) || !cut(dy, maxY-y0) {
		return 0, 0, 0, 0, false
	}
	return x0 + u0*dx, y0 + u0*dy, x0 + u1*dx, y0 + u1*dy, true
}

func setNRGBA(dst *image.RGBA, x, y int, c color.NRGBA) {
	if dst == nil || !image.Pt(x, y).In(dst.Bounds()) {
		return
	}
	i := dst.PixOffset(x, y)
	dst.Pix[i] = c.R
	dst.Pix[i+1] = c.G
	dst.Pix[i+2] = c.B
	dst.Pix[i+3] = c.A
}

func fillRings(dst *image.RGBA, rings [][]pt2, c color.NRGBA) {
	b := dst.Bounds()
	var px [][][2]float64
	for _, ring := range rings {
		if len(ring) < 3 {
			continue
		}
		out := make([][2]float64, len(ring))
		for i, p := range ring {
			out[i] = [2]float64{float64(p.x) * float64(b.Dx()), float64(p.y) * float64(b.Dy())}
		}
		px = append(px, out)
	}
	fillPolyPx(dst, px, c)
}

func fillPolyPx(dst *image.RGBA, rings [][][2]float64, c color.NRGBA) {
	if dst == nil || len(rings) == 0 {
		return
	}
	b := dst.Bounds()
	minY, maxY := b.Max.Y, b.Min.Y
	var edges [][4]float64
	for _, ring := range rings {
		for i := 1; i < len(ring); i++ {
			x0, y0 := ring[i-1][0], ring[i-1][1]
			x1, y1 := ring[i][0], ring[i][1]
			if y0 == y1 {
				continue
			}
			edges = append(edges, [4]float64{x0, y0, x1, y1})
			lo, hi := y0, y1
			if lo > hi {
				lo, hi = hi, lo
			}
			if int(math.Floor(lo)) < minY {
				minY = int(math.Floor(lo))
			}
			if int(math.Ceil(hi)) > maxY {
				maxY = int(math.Ceil(hi))
			}
		}
	}
	if minY < b.Min.Y {
		minY = b.Min.Y
	}
	if maxY > b.Max.Y-1 {
		maxY = b.Max.Y - 1
	}
	xs := make([]float64, 0, 8)
	for y := minY; y <= maxY; y++ {
		scan := float64(y) + 0.5
		xs = xs[:0]
		for _, e := range edges {
			y0, y1 := e[1], e[3]
			if (y0 <= scan && y1 > scan) || (y1 <= scan && y0 > scan) {
				t := (scan - y0) / (y1 - y0)
				xs = append(xs, e[0]+t*(e[2]-e[0]))
			}
		}
		sort.Float64s(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			x0 := int(math.Ceil(xs[i]))
			x1 := int(math.Floor(xs[i+1]))
			if x0 < b.Min.X {
				x0 = b.Min.X
			}
			if x1 > b.Max.X-1 {
				x1 = b.Max.X - 1
			}
			for x := x0; x <= x1; x++ {
				setNRGBA(dst, x, y, c)
			}
		}
	}
}

var tileHTTP = &http.Client{Timeout: 1500 * time.Millisecond}

func fetchTileHTTP(z, x, y int) ([]byte, error) {
	resp, err := tileHTTP.Get(scoutTileURL(x, y, z))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, tileStatusError{code: resp.StatusCode}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

// tileStatusError is an HTTP response that is not a tile. 418 and 404 mean
// this zoom is not in the Scout dataset. A transport failure is a plain error.
type tileStatusError struct{ code int }

func (e tileStatusError) Error() string { return fmt.Sprintf("tile http %d", e.code) }

// fetchTileFn is the tile body source. Tests replace it.
var fetchTileFn = fetchTileHTTP

// mapLog is the shell logger. The default writes the daemon log; init points
// it at the shell so the same line is kept for the on-screen log.
var mapLog = func(format string, args ...any) {
	log.Printf("maps: "+format, args...)
}

// enableMapNet starts the background fetcher. Tests turn it off.
var enableMapNet = true

type mapAtlas struct {
	mu         sync.Mutex
	geoms      map[tileKey]*mapGeom
	order      []tileKey
	bad        map[tileKey]time.Time
	want       []tileKey
	down       bool
	gen        int
	style      string
	running    bool
	paused     bool
	wake       chan struct{}
	maxOK      int
	blockAbove bool
	blockZ     int
	missLogged map[int]bool
	waitLogged bool
}

var liveAtlas = newAtlas()

func newAtlas() *mapAtlas {
	return &mapAtlas{
		geoms: map[tileKey]*mapGeom{},
		bad:   map[tileKey]time.Time{},
		style: styleOSMBright,
	}
}

// prepare returns cached geometry for keys and asks the fetcher for the rest.
// emptyFail is set when nothing is cached and Scout has already failed.
func (a *mapAtlas) prepare(keys []tileKey) (map[tileKey]*mapGeom, bool) {
	if a == nil {
		return nil, false
	}
	a.ensure()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.want = append([]tileKey(nil), keys...)
	a.kick()
	out := make(map[tileKey]*mapGeom, len(keys))
	for _, k := range keys {
		for step := 0; step <= 6 && k.z-step >= 2; step++ {
			pk := tileKey{z: k.z - step, x: k.x >> step, y: k.y >> step}
			if g := a.geoms[pk]; g != nil {
				out[pk] = g
			}
		}
	}
	if len(out) == 0 && a.down && len(keys) > 0 {
		return nil, true
	}
	return out, false
}

func (a *mapAtlas) isDown() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.down && len(a.geoms) == 0
}

func (a *mapAtlas) generation() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.gen
}

func (a *mapAtlas) pause() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
}

func (a *mapAtlas) ensure() {
	if a == nil || !enableMapNet {
		return
	}
	a.mu.Lock()
	start := !a.running
	a.paused = false
	if start {
		a.wake = make(chan struct{}, 1)
		a.running = true
	}
	a.kick()
	a.mu.Unlock()
	if start {
		go a.loop()
	}
}

func (a *mapAtlas) kick() {
	if a.wake == nil || a.paused {
		return
	}
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *mapAtlas) loop() {
	woke := false
	for {
		a.mu.Lock()
		w := a.wake
		a.mu.Unlock()
		if w == nil {
			return
		}
		<-w
		for {
			k, ok := a.nextMissing()
			if !ok {
				break
			}
			if !woke {
				woke = true
				mapLog("waking OSM Scout")
			}
			mapLog("fetch z%d %d,%d", k.z, k.x, k.y)
			started := time.Now()
			style := currentScoutStyle()
			body, err := fetchTileFn(k.z, k.x, k.y)
			ms := time.Since(started).Milliseconds()
			if err != nil {
				mapLog("z%d %d,%d: %v (%dms)", k.z, k.x, k.y, err, ms)
				var status tileStatusError
				if errors.As(err, &status) && status.code >= 400 && status.code < 500 {
					a.markMiss(k, status.code)
				} else {
					a.markBad(k)
				}
				continue
			}
			g, err := decodeMVT(body)
			if err != nil {
				mapLog("z%d %d,%d: %v", k.z, k.x, k.y, err)
				a.markBad(k)
				continue
			}
			a.store(k, g, style)
			mapLog("z%d %d,%d ready in %dms", k.z, k.x, k.y, ms)
		}
	}
}

func (a *mapAtlas) nextMissing() (tileKey, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.paused {
		return tileKey{}, false
	}
	now := time.Now()
	for _, k := range a.want {
		if a.geoms[k] != nil {
			continue
		}
		if t, ok := a.bad[k]; ok && now.Sub(t) < 5*time.Second {
			continue
		}
		// Claim it so the next iteration does not fetch it twice.
		a.bad[k] = now
		return k, true
	}
	return tileKey{}, false
}

// zoomCap is the deepest integer zoom the camera may use. The second
// result is false until a higher zoom has been refused.
func (a *mapAtlas) zoomCap() (int, bool) {
	if a == nil {
		return 0, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.blockAbove {
		return 0, false
	}
	if a.maxOK > 0 {
		return a.maxOK, true
	}
	if a.blockZ > 2 {
		return a.blockZ - 1, true
	}
	return 0, false
}

// markMiss records a tile the server refused. It does not blank the map:
// a 418 at z15 means "stay on the tiles we already have".
func (a *mapAtlas) markMiss(k tileKey, status int) {
	a.mu.Lock()
	line := ""
	if a.bad == nil {
		a.bad = map[tileKey]time.Time{}
	}
	a.bad[k] = time.Now()
	fresh := !(a.maxOK > 0 && k.z <= a.maxOK) && !(a.blockAbove && (a.blockZ == 0 || k.z >= a.blockZ))
	if fresh {
		a.blockAbove = true
		a.blockZ = k.z
		if a.missLogged == nil {
			a.missLogged = map[int]bool{}
		}
		if !a.missLogged[k.z] {
			a.missLogged[k.z] = true
			have := a.maxOK
			if have == 0 && k.z > 2 {
				have = k.z - 1
			}
			line = fmt.Sprintf("zoom %d is not served (HTTP %d); staying at z%d", k.z, status, have)
		}
	}
	a.mu.Unlock()
	if line != "" {
		mapLog("%s", line)
	}
}

// noteWaiting records that a frame had nothing to draw yet. Once per session.
func (a *mapAtlas) noteWaiting() {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.waitLogged || len(a.geoms) > 0 {
		a.mu.Unlock()
		return
	}
	a.waitLogged = true
	a.mu.Unlock()
	mapLog("waiting for tiles")
}

func (a *mapAtlas) markBad(k tileKey) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bad == nil {
		a.bad = map[tileKey]time.Time{}
	}
	a.bad[k] = time.Now()
	if len(a.geoms) == 0 {
		a.down = true
		a.gen++
	}
}

func (a *mapAtlas) drop() {
	style := currentScoutStyle()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.geoms = map[tileKey]*mapGeom{}
	a.order = nil
	a.bad = map[tileKey]time.Time{}
	a.style = style
	a.gen++
}

func (a *mapAtlas) store(k tileKey, g mapGeom, style string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.style != "" && style != a.style {
		delete(a.bad, k)
		return
	}
	if a.geoms == nil {
		a.geoms = map[tileKey]*mapGeom{}
	}
	if _, ok := a.geoms[k]; !ok {
		a.order = append(a.order, k)
	}
	a.geoms[k] = &g
	delete(a.bad, k)
	a.down = false
	if k.z > a.maxOK {
		a.maxOK = k.z
	}
	a.gen++
	for len(a.geoms) > 160 && len(a.order) > 0 {
		old := a.order[0]
		a.order = a.order[1:]
		if old == k {
			a.order = append(a.order, old)
			break
		}
		delete(a.geoms, old)
	}
}
