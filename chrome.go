package main

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/mrcyjanek/cargadgetd_app_maps/nav"
	"github.com/mrcyjanek/cargadgetd_sdk/fynekit"
)

// navMetrics matches the music card: half the drive wide, a heading-sized
// distance, and body text at the same size as the track title's neighbour.
// The column is the wrap width for the instruction under the icon row.
func navMetrics(w int) (glyph int, dist, body, col float32) {
	if w < 320 {
		w = 320
	}
	body = 16
	dist = 26
	glyph = 52
	col = float32(w)/2 - 24
	if col < 200 {
		col = 200
	}
	return glyph, dist, body, col
}

type zoomLayout struct{}

func (zoomLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(64, float32(len(objs))*72-8)
}

func (zoomLayout) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	y := float32(0)
	for _, o := range objs {
		o.Resize(fyne.NewSize(64, 64))
		o.Move(fyne.NewPos(0, y))
		y += 72
	}
}

type chromeLayout struct{}

func (chromeLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(1, 1)
}

// Layout pins the maneuver plate to the top-left, the remaining trip to
// the top-right, and the zoom keys to the right, above the music card.
func (chromeLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if len(objs) < 3 {
		return
	}
	plate, zoom, eta := objs[0], objs[1], objs[2]
	if plate.Visible() {
		plate.Resize(plate.MinSize())
		plate.Move(fyne.NewPos(8, 8))
	}
	if eta.Visible() {
		e := eta.MinSize()
		eta.Resize(e)
		eta.Move(fyne.NewPos(size.Width-e.Width-8, 8))
	}
	if zoom.Visible() {
		z := zoom.MinSize()
		bottom := size.Height - 110
		if bottom < z.Height+8 {
			bottom = size.Height
		}
		zoom.Resize(z)
		zoom.Move(fyne.NewPos(size.Width-z.Width-12, bottom-z.Height-12))
	}
}

func (s *drive) ensureChrome() {
	if s.kit != nil {
		return
	}
	ink := color.NRGBA{R: 232, G: 236, B: 240, A: 255}
	s.navDist = canvas.NewText("", ink)
	s.navDist.TextStyle = fyne.TextStyle{Bold: true}
	s.navMeasure = canvas.NewText("", ink)
	s.navMeasure.TextStyle = fyne.TextStyle{Bold: true}
	s.navIcon = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	s.navIcon.FillMode = canvas.ImageFillContain
	s.navExit = canvas.NewText("", color.NRGBA{R: 16, G: 20, B: 24, A: 255})
	s.navExit.Alignment = fyne.TextAlignCenter
	s.navExit.TextStyle = fyne.TextStyle{Bold: true}
	s.navExit.Hide()
	s.navRows = container.NewVBox()
	head := container.NewHBox(container.NewStack(s.navIcon, s.navExit), s.navDist)
	body := container.NewVBox(head, s.navRows)
	bg := canvas.NewRectangle(color.NRGBA{R: 16, G: 20, B: 24, A: 224})
	s.navPlate = container.NewStack(bg, container.NewPadded(body))
	s.navEta = canvas.NewText("", ink)
	s.navEta.TextStyle = fyne.TextStyle{Bold: true}
	s.navEta.Alignment = fyne.TextAlignTrailing
	s.navEtaSpace = canvas.NewRectangle(color.NRGBA{})
	etaBG := canvas.NewRectangle(color.NRGBA{R: 16, G: 20, B: 24, A: 224})
	s.navEtaPlate = container.NewStack(etaBG, container.NewPadded(container.NewStack(s.navEtaSpace, s.navEta)))
	s.navEtaPlate.Hide()
	plus := widget.NewButton("+", func() { s.nudgeMapZoom(1) })
	minus := widget.NewButton("−", func() { s.nudgeMapZoom(-1) })
	plus.Importance = widget.HighImportance
	minus.Importance = widget.HighImportance
	s.zoomBox = container.New(&zoomLayout{}, plus, minus)
	s.zoomBox.Hide()
	s.navPlate.Hide()

	s.kit = fynekit.NewTransparent()
	s.kit.Ensure(func() fyne.CanvasObject {
		return container.New(&chromeLayout{}, s.navPlate, s.zoomBox, s.navEtaPlate)
	})
	if c := s.kit.Canvas(); c != nil {
		c.SetPadded(false)
	}
}

func (s *drive) syncChrome() {
	s.ensureChrome()
	trip := s.navSnapshot()
	s.noteNavErr(trip)
	left, dist := navCopy(trip)
	px, distSize, bodySize, colW := navMetrics(s.w)
	glyph := s.glyphImage(trip.Icon, px)
	show := trip.HasRoute && (dist != "" || len(left) > 0 || glyph != nil || trip.RoundaboutExit > 0)
	if s.navDist.TextSize != distSize {
		s.navDist.TextSize = distSize
		s.markChrome()
	}
	if s.navExit.TextSize != 18 {
		s.navExit.TextSize = 18
		s.markChrome()
	}
	if setText(s.navDist, dist) {
		s.markChrome()
	}
	if s.layoutNav(left, bodySize, colW) {
		s.markChrome()
	}
	exit := ""
	if trip.RoundaboutExit > 0 {
		exit = strconv.Itoa(trip.RoundaboutExit)
	}
	if setText(s.navExit, exit) {
		s.markChrome()
	}
	if s.navPx != px {
		s.navPx = px
		s.navIcon.SetMinSize(fyne.NewSize(float32(px), float32(px)))
		s.markChrome()
	}
	if glyph != nil && s.navIcon.Image != glyph {
		s.navIcon.Image = glyph
		s.navIcon.Refresh()
		s.markChrome()
	}
	if showIcon := glyph != nil; showIcon != s.navIcon.Visible() {
		if showIcon {
			s.navIcon.Show()
		} else {
			s.navIcon.Hide()
		}
		s.markChrome()
	}
	if show != s.navPlate.Visible() {
		if show {
			s.navPlate.Show()
		} else {
			s.navPlate.Hide()
		}
		s.markChrome()
	}
	s.syncEta(nav.Plain(trip.DestDist), nav.Plain(trip.DestTime), distSize, trip.HasRoute)
	s.mu.Lock()
	on := s.cam.showZoom()
	s.mu.Unlock()
	s.syncMapZoom(on)
}

// etaClock is the clock the corner readout uses. Tests pin it.
var etaClock = time.Now

// etaText alternates remaining distance and remaining time every two seconds.
// One empty side leaves the other on screen.
func etaText(dist, dur string, now time.Time) string {
	if dist == "" {
		return dur
	}
	if dur == "" {
		return dist
	}
	if (now.Unix()/2)&1 == 0 {
		return dist
	}
	return dur
}

func (s *drive) syncEta(dist, dur string, size float32, show bool) {
	face := ""
	if show {
		face = etaText(dist, dur, etaClock())
	}
	if s.navEta.TextSize != size {
		s.navEta.TextSize = size
		s.markChrome()
	}
	// canvas.Text ignores SetMinSize, so a clear spacer holds the wider of
	// the two strings and the plate does not jump every two seconds.
	s.measureNav(dist, size)
	wide := s.navMeasure.MinSize()
	s.measureNav(dur, size)
	if other := s.navMeasure.MinSize(); other.Width > wide.Width || other.Height > wide.Height {
		if other.Width > wide.Width {
			wide.Width = other.Width
		}
		if other.Height > wide.Height {
			wide.Height = other.Height
		}
	}
	if s.navEtaWide != wide.Width {
		s.navEtaWide = wide.Width
		s.navEtaSpace.SetMinSize(wide)
		s.markChrome()
	}
	if setText(s.navEta, face) {
		s.markChrome()
	}
	on := face != ""
	if on != s.navEtaPlate.Visible() {
		if on {
			s.navEtaPlate.Show()
		} else {
			s.navEtaPlate.Hide()
		}
		s.markChrome()
	}
}

func setText(t *canvas.Text, v string) bool {
	if t.Text == v && t.Visible() == (v != "") {
		return false
	}
	t.Text = v
	if v == "" {
		t.Hide()
	} else {
		t.Show()
	}
	t.Refresh()
	return true
}

// layoutNav wraps every instruction onto as many lines as it needs.
// Nothing is dropped or ellipsized.
func (s *drive) layoutNav(lines []string, size, maxW float32) bool {
	var wrapped []string
	for _, ln := range lines {
		wrapped = append(wrapped, wrapNav(ln, size, maxW, s.measureNav)...)
	}
	sig := strings.Join(wrapped, "\n")
	if sig == s.navSig && (len(wrapped) == 0 || (len(s.navBody) > 0 && s.navBody[0].TextSize == size)) {
		return false
	}
	s.navSig = sig
	ink := color.NRGBA{R: 232, G: 236, B: 240, A: 255}
	s.navBody = s.navBody[:0]
	objs := make([]fyne.CanvasObject, 0, len(wrapped))
	for _, ln := range wrapped {
		t := canvas.NewText(ln, ink)
		t.TextSize = size
		t.TextStyle = fyne.TextStyle{Bold: true}
		s.navBody = append(s.navBody, t)
		objs = append(objs, t)
	}
	s.navRows.Objects = objs
	s.navRows.Refresh()
	return true
}

func (s *drive) measureNav(text string, size float32) float32 {
	s.navMeasure.Text = text
	s.navMeasure.TextSize = size
	return s.navMeasure.MinSize().Width
}

// wrapNav breaks text on spaces so each line fits maxW. Words stay whole.
func wrapNav(text string, size, maxW float32, measure func(string, float32) float32) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := ""
	for _, word := range words {
		if cur == "" {
			cur = word
			continue
		}
		cand := cur + " " + word
		if maxW > 0 && measure(cand, size) > maxW {
			lines = append(lines, cur)
			cur = word
			continue
		}
		cur = cand
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func (s *drive) markChrome() {
	if s.kit != nil {
		s.kit.MarkDirty()
	}
}

func (s *drive) overlayChrome(dst *image.RGBA) {
	if s.kit == nil || dst == nil {
		return
	}
	s.kit.Overlay(dst)
}

func (s *drive) chromeHits(x, y int) bool {
	s.ensureChrome()
	if s.kit == nil {
		return false
	}
	return s.kit.Hits(x, y)
}

func (s *drive) touchChrome(x, y, action int) {
	if s.kit != nil {
		s.kit.Touch(x, y, action)
	}
}

// glyphImage is the Pure Maps maneuver sign, rasterized once per icon name.
func (s *drive) glyphImage(icon string, px int) *image.RGBA {
	if px < 1 {
		px = 1
	}
	path := nav.GlyphPath(icon)
	key := path + "\x00" + strconv.Itoa(px)
	if key == s.glyphFile {
		return s.glyphImg
	}
	s.glyphFile = key
	s.glyphImg = nil
	if path == "" {
		return nil
	}
	img, err := rasterSVG(path, px)
	if err != nil {
		s.logf("nav icon: %v", err)
		return nil
	}
	s.glyphImg = img
	return img
}
