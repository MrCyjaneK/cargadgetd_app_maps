package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStyleNames(t *testing.T) {
	prev := styleDirs
	styleDirs = []string{filepath.Join(t.TempDir(), "missing")}
	t.Cleanup(func() { styleDirs = prev })

	if got := normalizeStyle("Midnight Commander"); got != "mc" {
		t.Fatalf("label %s", got)
	}
	if got := normalizeStyle("OSM Bright Car EN"); got != "osmbright-car-en" {
		t.Fatalf("car en %s", got)
	}
	if got := normalizeStyle("mc-en"); got != "mc-en" {
		t.Fatalf("id %s", got)
	}
	if got := normalizeStyle("nope"); got != styleOSMBright {
		t.Fatalf("default %s", got)
	}
	if styleLabel("osmbright-car") != "OSM Bright Car" {
		t.Fatal(styleLabel("osmbright-car"))
	}
	if n := len(strings.Split(styleChoices(), ",")); n != len(builtinStyles) {
		t.Fatalf("choices %d %s", n, styleChoices())
	}
	setScoutStyle("mc")
	t.Cleanup(func() {
		setScoutStyle(styleOSMBright)
		liveAtlas.drop()
	})
	if got := scoutTileURL(1, 2, 3); !strings.Contains(got, "style=mc&x=1&y=2&z=3") {
		t.Fatal(got)
	}
}

func TestStyleDir(t *testing.T) {
	dir := t.TempDir()
	writeStyle(t, dir, "osmbright.json", `{"name":"OSM Bright","layers":[]}`)
	writeStyle(t, dir, "osmbright-en.json", `{"name":"OSM Bright"}`)
	writeStyle(t, dir, "hike.json", `{"name":"Day, Night"}`)
	writeStyle(t, dir, "not a style.json", `{"name":"Nope"}`)
	writeStyle(t, dir, "notes.txt", `{"name":"Notes"}`)

	prev := styleDirs
	styleDirs = []string{dir}
	t.Cleanup(func() { styleDirs = prev })

	if got := normalizeStyle("Day Night"); got != "hike" {
		t.Fatalf("hike %s", got)
	}
	if got := normalizeStyle("hike"); got != "hike" {
		t.Fatalf("id %s", got)
	}
	if got := normalizeStyle("OSM Bright (osmbright-en)"); got != "osmbright-en" {
		t.Fatalf("en %s", got)
	}
	if normalizeStyle("Nope") != styleOSMBright {
		t.Fatal("rejected file was offered")
	}
	choices := strings.Split(styleChoices(), ",")
	if len(choices) != 3 {
		t.Fatalf("choices %q", choices)
	}
	if choices[0] != "OSM Bright (osmbright)" {
		t.Fatalf("first %q", choices[0])
	}
}

func writeStyle(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
