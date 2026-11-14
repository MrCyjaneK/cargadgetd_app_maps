package main

import (
	"image"
	"image/color"
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/mrcyjanek/cargadgetd_app_maps/nav"
	"github.com/mrcyjanek/cargadgetd_sdk/fynekit"
)

// loc is one position sample from the daemon gps_fix broadcast.
type loc struct {
	HasPos         bool
	Lat, Lon, AccM float64
	HasSpeed       bool
	Kmh            int
	HasCourse      bool
	Course         float64
}

var (
	colLime = color.NRGBA{R: 212, G: 255, B: 0, A: 255}
	colFG   = color.NRGBA{R: 232, G: 236, B: 240, A: 255}
)

// drive is the OSM map. The shell gives it the rect under the top bar.
type drive struct {
	mu      sync.Mutex
	w, h    int
	uiScale float32

	mapKind    string
	mapStyle   string
	mapTilt    string
	mapAhead   float64
	mapRaster  *canvas.Raster
	engine     mapEngine
	mapNow     time.Time
	mapStamp   string
	mapJob     mapJob
	mapWake    chan struct{}
	mapStop    chan struct{}
	mapDone    chan struct{}
	mapRunning bool

	cam          mapCam
	mapGrabOn    bool
	mapGrabMoved bool
	mapGrabX     float32
	mapGrabY     float32
	mapGrabLastX float32
	mapGrabLastY float32
	zoomBox      fyne.CanvasObject

	fix loc

	kit         *fynekit.Kit
	navPlate    fyne.CanvasObject
	navRows     *fyne.Container
	navDist     *canvas.Text
	navBody     []*canvas.Text
	navMeasure  *canvas.Text
	navIcon     *canvas.Image
	navExit     *canvas.Text
	navEta      *canvas.Text
	navEtaSpace *canvas.Rectangle
	navEtaPlate fyne.CanvasObject
	navEtaWide  float32
	navSig      string
	navPx       int

	navKind   string
	navSrc    nav.Source
	navErr    string
	glyphFile string
	glyphImg  *image.RGBA
}

func newDrive() *drive {
	style := loadStyle()
	setScoutStyle(style)
	return &drive{
		mapKind:  normalizeMap(loadMap()),
		mapStyle: style,
		mapTilt:  loadTilt(),
		mapAhead: loadAhead(),
		engine:   newMapEngine(loadRenderer()),
		navKind:  nav.Normalize(loadNav()),
		uiScale:  1,
	}
}

func (s *drive) logf(format string, args ...any) {
	log.Printf("maps: "+format, args...)
}

func (s *drive) bump() {}

func (s *drive) gpsSnapshot() loc {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fix
}

func (s *drive) setFix(f loc) {
	s.mu.Lock()
	s.fix = f
	s.mu.Unlock()
}

func (s *drive) setSize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	s.mu.Lock()
	s.w, s.h = w, h
	s.mu.Unlock()
}
