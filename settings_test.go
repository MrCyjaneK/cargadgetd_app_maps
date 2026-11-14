package main

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrcyjanek/cargadgetd_sdk/cfg"
)

func TestZoomButtonsNudgeWhileBrowsing(t *testing.T) {
	m := newMaps()
	m.d.mu.Lock()
	m.d.w, m.d.h = 800, 480
	m.d.cam = mapCam{lat: 50, lon: 19, zoom: 14, has: true, browsing: true}
	m.d.mu.Unlock()
	dst := image.NewRGBA(image.Rect(0, 0, 800, 480))
	m.d.syncChrome()
	m.d.overlayChrome(dst)
	if m.d.zoomBox == nil || !m.d.zoomBox.Visible() {
		t.Fatal("zoom hidden")
	}
	plus := m.d.zoomBox.Position()
	z := m.d.cam.zoom
	m.touch(int(plus.X)+8, int(plus.Y)+8, 1)
	m.touch(int(plus.X)+8, int(plus.Y)+8, 0)
	if m.d.cam.zoom < z+0.9 {
		t.Fatalf("zoom %v from %v", m.d.cam.zoom, z)
	}
}

func TestMapsSettingsWritesPluginFile(t *testing.T) {
	dir := t.TempDir()
	cfg.SetOwnRootForTest(t, dir)
	t.Cleanup(func() {
		app.d.mu.Lock()
		app.d.engine = newMapEngine(rendererVector)
		app.d.mapStyle = styleOSMBright
		app.d.mapStamp = ""
		app.d.mapJob = mapJob{}
		app.d.mu.Unlock()
		setScoutStyle(styleOSMBright)
		liveAtlas.drop()
	})

	if loadMap() != mapOSM || loadTilt() != tiltDriving || loadAhead() != aheadDefault {
		t.Fatalf("map %s tilt %s ahead %v", loadMap(), loadTilt(), loadAhead())
	}
	app.d.mu.Lock()
	app.d.mapKind = mapOSM
	app.d.mapTilt = tiltDriving
	app.d.mapAhead = aheadDefault
	app.d.mu.Unlock()
	if mapSettingsSetEnum(mapSetSource, "Glitch") != 0 {
		t.Fatal(mapSettingsErr)
	}
	if mapSettingsSetEnum(mapSetTilt, "Always") != 0 {
		t.Fatal(mapSettingsErr)
	}
	if mapSettingsSetInt(mapSetAhead, 4) != 0 {
		t.Fatal(mapSettingsErr)
	}
	if mapSettingsSetEnum(mapSetRenderer, "MapLibre") != 0 {
		t.Fatal(mapSettingsErr)
	}
	if mapSettingsGetEnum(mapSetRenderer) != "MapLibre" {
		t.Fatalf("renderer %s", mapSettingsGetEnum(mapSetRenderer))
	}
	if mapSettingsSetEnum(mapSetStyle, "Midnight Commander") != 0 {
		t.Fatal(mapSettingsErr)
	}
	if mapSettingsGetEnum(mapSetStyle) != "Midnight Commander" {
		t.Fatalf("style %s", mapSettingsGetEnum(mapSetStyle))
	}
	plug, err := os.ReadFile(filepath.Join(dir, "maps.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plug), "source: glitch") || !strings.Contains(string(plug), "tilt: always") || !strings.Contains(string(plug), "ahead:") || !strings.Contains(string(plug), "renderer: maplibre") || !strings.Contains(string(plug), "style: mc") {
		t.Fatalf("plugin:\n%s", plug)
	}
	if loadMap() != mapGlitch || loadTilt() != tiltAlways || loadAhead() != 4 {
		t.Fatalf("new map %s tilt %s ahead %v", loadMap(), loadTilt(), loadAhead())
	}
}
