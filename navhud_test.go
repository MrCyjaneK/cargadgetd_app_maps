package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mrcyjanek/cargadgetd_app_maps/nav"
	"github.com/mrcyjanek/cargadgetd_sdk/cfg"
)

type stubNav struct {
	trip nav.Trip
	path []nav.Point
}

func (s stubNav) Snapshot() nav.Trip { return s.trip }
func (s stubNav) Route() []nav.Point { return s.path }
func (stubNav) Close()               {}

func TestNavCopy(t *testing.T) {
	left, dist := navCopy(nav.Trip{
		HasRoute:  true,
		Street:    "High Street",
		Narrative: "Turn right onto High Street",
		ManDist:   "200 m",
		DestTime:  "18 min",
		DestDist:  "12 km",
	})
	if dist != "200 m" || len(left) != 2 || left[0] != "High Street" || left[1] != "Turn right onto High Street" {
		t.Fatalf("left %q dist %q", left, dist)
	}
	left, dist = navCopy(nav.Trip{Street: "High Street", Narrative: "High Street", ManDist: "-"})
	if dist != "" || len(left) != 1 || left[0] != "High Street" {
		t.Fatalf("dup %q dist %q", left, dist)
	}
	left, dist = navCopy(nav.Trip{Narrative: "Skręć w Łódź", ManDist: "50 m"})
	if dist != "50 m" || len(left) != 1 || left[0] != "Skręć w Łódź" {
		t.Fatalf("pl %q dist %q", left, dist)
	}
}

func TestChromeKeepsPolish(t *testing.T) {
	prev := openNav
	openNav = func(string) nav.Source {
		return stubNav{trip: nav.Trip{HasRoute: true, Narrative: "Skręć w Łódź", ManDist: "50 m"}}
	}
	t.Cleanup(func() { openNav = prev })
	d := &drive{w: 800, h: 480}
	d.syncChrome()
	if d.navDist.Text != "50 m" || navPlain(d) != "Skręć w Łódź" {
		t.Fatalf("dist %q line %q", d.navDist.Text, navPlain(d))
	}
	if len(d.navBody) == 0 || d.navDist.TextSize <= d.navBody[0].TextSize || d.navBody[0].TextSize > 20 || d.navDist.TextSize > 32 {
		t.Fatalf("size dist %v body %v", d.navDist.TextSize, d.navBody)
	}
	sz := d.navPlate.MinSize()
	if sz.Width > 280 || sz.Height > 140 {
		t.Fatalf("plate %v", sz)
	}
	d.closeNav()
}

func TestPaintNavPlates(t *testing.T) {
	prev := openNav
	openNav = func(string) nav.Source {
		return stubNav{trip: nav.Trip{
			HasRoute:  true,
			Icon:      "turn-right",
			ManDist:   "200 m",
			Street:    "High Street",
			Narrative: "Turn right onto High Street",
			DestTime:  "18 min",
			DestDist:  "12 km",
		}}
	}
	t.Cleanup(func() { openNav = prev })

	dir := t.TempDir()
	prevDirs := nav.GlyphDirs
	nav.GlyphDirs = []string{dir, filepath.Join(dir, "missing")}
	t.Cleanup(func() { nav.GlyphDirs = prevDirs })
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48"><rect width="48" height="48" fill="#ffffff"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "turn-right-white.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &drive{navKind: "mock", w: 800, h: 480}
	dst := image.NewRGBA(image.Rect(0, 0, 800, 480))
	fill := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	fillRect(dst, dst.Bounds(), fill)
	d.syncChrome()
	d.overlayChrome(dst)
	got := navPlain(d)
	if d.navDist.Text != "200 m" || strings.Contains(got, "...") || !strings.Contains(got, "High Street") || !strings.Contains(got, "Turn right onto High Street") || strings.Contains(got, "18 min") || strings.Contains(got, "12 km") {
		t.Fatalf("plate dist %q text %q", d.navDist.Text, got)
	}
	if chromeInk(dst, fill) < 50 {
		t.Fatal("plate missing")
	}
	sz := d.navPlate.MinSize()
	if sz.Width > float32(d.w)/2+40 || sz.Height > 200 || d.navDist.TextSize <= d.navBody[0].TextSize {
		t.Fatalf("plate %v dist %v body %v", sz, d.navDist.TextSize, d.navBody[0].TextSize)
	}
	if !whiteNear(dst, 220, 120) {
		t.Fatal("glyph missing")
	}
	d.closeNav()

	openNav = func(string) nav.Source { return stubNav{} }
	d.navSrc = nil
	fillRect(dst, dst.Bounds(), fill)
	d.syncChrome()
	d.overlayChrome(dst)
	if chromeInk(dst, fill) != 0 || cornerInk(dst, fill) != 0 {
		t.Fatal("cleared route still painted")
	}
}

func TestEtaAlternates(t *testing.T) {
	prev := openNav
	openNav = func(string) nav.Source {
		return stubNav{trip: nav.Trip{
			HasRoute: true,
			DestDist: "12 km",
			DestTime: "1 h 20 min",
		}}
	}
	t.Cleanup(func() { openNav = prev })
	prevClock := etaClock
	t.Cleanup(func() { etaClock = prevClock })

	if etaText("12 km", "1 h 20 min", time.Unix(0, 0)) != "12 km" || etaText("12 km", "1 h 20 min", time.Unix(2, 0)) != "1 h 20 min" {
		t.Fatal("faces")
	}
	if etaText("12 km", "", time.Unix(2, 0)) != "12 km" || etaText("", "18 min", time.Unix(0, 0)) != "18 min" || etaText("", "", time.Unix(0, 0)) != "" {
		t.Fatal("one side")
	}

	d := &drive{w: 800, h: 480}
	dst := image.NewRGBA(image.Rect(0, 0, 800, 480))
	fill := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	fillRect(dst, dst.Bounds(), fill)
	etaClock = func() time.Time { return time.Unix(0, 0) }
	d.syncChrome()
	d.overlayChrome(dst)
	if d.navEta.Text != "12 km" || !d.navEtaPlate.Visible() {
		t.Fatalf("km %q vis %v", d.navEta.Text, d.navEtaPlate.Visible())
	}
	pos := d.navEtaPlate.Position()
	sz := d.navEtaPlate.Size()
	if pos.X < 500 || pos.Y > 16 || pos.X+sz.Width > 800 || cornerInk(dst, fill) < 20 {
		t.Fatalf("corner pos %v size %v", pos, sz)
	}
	wide := sz.Width
	fillRect(dst, dst.Bounds(), fill)
	etaClock = func() time.Time { return time.Unix(2, 0) }
	d.syncChrome()
	d.overlayChrome(dst)
	if d.navEta.Text != "1 h 20 min" || d.navEtaPlate.Size().Width != wide {
		t.Fatalf("time %q width %v want %v", d.navEta.Text, d.navEtaPlate.Size().Width, wide)
	}
	d.closeNav()
}

func TestWrapNavKeepsEveryWord(t *testing.T) {
	d := &drive{w: 800, h: 480}
	d.ensureChrome()
	const full = "Skręć w prawo w bardzo długą ulicę Jana Pawła Drugiego"
	lines := wrapNav(full, 48, 220, d.measureNav)
	if strings.Join(lines, " ") != full {
		t.Fatalf("wrapped %q", lines)
	}
	if len(lines) < 2 {
		t.Fatalf("one line %q", lines)
	}
	d.closeNav()
}

func navPlain(d *drive) string {
	var parts []string
	for _, t := range d.navBody {
		if t != nil && t.Text != "" {
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, " ")
}

func chromeInk(dst *image.RGBA, fill color.RGBA) int {
	n := 0
	for y := 0; y < 140 && y < dst.Bounds().Dy(); y++ {
		for x := 0; x < 360 && x < dst.Bounds().Dx(); x++ {
			if dst.RGBAAt(x, y) != fill {
				n++
			}
		}
	}
	return n
}

func cornerInk(dst *image.RGBA, fill color.RGBA) int {
	b := dst.Bounds()
	n := 0
	x0 := b.Dx() - 220
	if x0 < 0 {
		x0 = 0
	}
	for y := 0; y < 80 && y < b.Dy(); y++ {
		for x := x0; x < b.Dx(); x++ {
			if dst.RGBAAt(x, y) != fill {
				n++
			}
		}
	}
	return n
}

func whiteNear(dst *image.RGBA, w, h int) bool {
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Min.Y+h && y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Min.X+w && x < b.Max.X; x++ {
			p := dst.RGBAAt(x, y)
			if p.R > 200 && p.G > 200 && p.B > 200 {
				return true
			}
		}
	}
	return false
}

func TestPaintNavRoundabout(t *testing.T) {
	prev := openNav
	openNav = func(string) nav.Source {
		return stubNav{trip: nav.Trip{
			HasRoute:       true,
			Icon:           "roundabout",
			ManDist:        "40 m",
			RoundaboutExit: 3,
		}}
	}
	t.Cleanup(func() { openNav = prev })
	dir := t.TempDir()
	prevDirs := nav.GlyphDirs
	nav.GlyphDirs = []string{dir}
	t.Cleanup(func() { nav.GlyphDirs = prevDirs })
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48"><rect width="48" height="48" fill="#ffffff"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "roundabout-white.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	d := &drive{w: 800, h: 480}
	dst := image.NewRGBA(image.Rect(0, 0, 800, 480))
	fill := color.RGBA{A: 255}
	fillRect(dst, dst.Bounds(), fill)
	d.syncChrome()
	d.overlayChrome(dst)
	if d.navExit.Text != "3" || !d.navExit.Visible() {
		t.Fatalf("exit %q visible %v", d.navExit.Text, d.navExit.Visible())
	}
	if d.navDist.Text != "40 m" {
		t.Fatalf("dist %q", d.navDist.Text)
	}
	if chromeInk(dst, fill) < 50 {
		t.Fatal("roundabout plate missing")
	}
	d.closeNav()
}

func TestLoadNavReadsPluginFile(t *testing.T) {
	cfg.SetOwnRootForTest(t, t.TempDir())
	if got := loadNav(); got != "puremaps" {
		t.Fatalf("default %q", got)
	}
	if err := cfg.OwnSet("maps", "nav", "mock"); err != nil {
		t.Fatal(err)
	}
	if got := loadNav(); got != "mock" {
		t.Fatalf("nav %q", got)
	}
}
