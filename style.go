package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// styleOSMBright is the OSM Scout Mapbox style the drive uses until Settings
// picks another. The id is the style filename without .json.
const styleOSMBright = "osmbright"

// scoutStyleDir is the folder OSM Scout serves. A style is any *.json there;
// the id in /v1/mbgl/style?style= is the filename without .json.
const scoutStyleDir = "/usr/share/harbour-osmscout-server/styles/mapboxgl/styles"

type scoutStyle struct {
	id, label string
}

// builtinStyles is the set Scout ships. It is used when that folder cannot
// be read, so the row still has a choice on a machine without OSM Scout.
var builtinStyles = []scoutStyle{
	{"osmbright", "OSM Bright"},
	{"osmbright-en", "OSM Bright EN"},
	{"osmbright-car", "OSM Bright Car"},
	{"osmbright-car-en", "OSM Bright Car EN"},
	{"mc", "Midnight Commander"},
	{"mc-en", "Midnight Commander EN"},
	{"mc-car", "Midnight Commander Car"},
	{"mc-car-en", "Midnight Commander Car EN"},
}

// styleDirs, when non-nil, replaces the OSM Scout style folder (tests).
var styleDirs []string

var (
	scoutStyleMu sync.Mutex
	scoutStyleID = styleOSMBright
)

func currentScoutStyle() string {
	scoutStyleMu.Lock()
	defer scoutStyleMu.Unlock()
	if scoutStyleID == "" {
		return styleOSMBright
	}
	return scoutStyleID
}

func setScoutStyle(id string) {
	id = normalizeStyle(id)
	scoutStyleMu.Lock()
	scoutStyleID = id
	scoutStyleMu.Unlock()
}

func styleFolders() []string {
	if styleDirs != nil {
		return styleDirs
	}
	return []string{scoutStyleDir}
}

func styleList() []scoutStyle {
	var found []scoutStyle
	seen := map[string]bool{}
	for _, dir := range styleFolders() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".json") {
				continue
			}
			id := strings.TrimSuffix(name, ".json")
			if !styleIDOK(id) || seen[id] {
				continue
			}
			seen[id] = true
			found = append(found, scoutStyle{
				id:    id,
				label: styleFileLabel(filepath.Join(dir, name), id),
			})
		}
	}
	if len(found) == 0 {
		out := make([]scoutStyle, len(builtinStyles))
		copy(out, builtinStyles)
		return out
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].id == styleOSMBright {
			return true
		}
		if found[j].id == styleOSMBright {
			return false
		}
		if found[i].label == found[j].label {
			return found[i].id < found[j].id
		}
		return found[i].label < found[j].label
	})
	return uniqueLabels(found)
}

// uniqueLabels keeps the settings enum unambiguous. Scout's English files
// reuse the base style's JSON name, and a comma would split the row.
func uniqueLabels(in []scoutStyle) []scoutStyle {
	count := map[string]int{}
	for _, s := range in {
		count[s.label]++
	}
	for i, s := range in {
		if count[s.label] > 1 {
			in[i].label = s.label + " (" + s.id + ")"
		}
	}
	return in
}

func styleFileLabel(path, id string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return id
	}
	var doc struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(b, &doc) != nil {
		return id
	}
	name := strings.Join(strings.Fields(strings.ReplaceAll(doc.Name, ",", " ")), " ")
	if name == "" {
		return id
	}
	return name
}

func styleIDOK(id string) bool {
	if id == "" || len(id) > 47 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

func normalizeStyle(kind string) string {
	k := strings.TrimSpace(kind)
	low := strings.ToLower(k)
	for _, s := range styleList() {
		if low == strings.ToLower(s.id) || strings.EqualFold(k, s.label) {
			return s.id
		}
	}
	return styleOSMBright
}

func styleLabel(id string) string {
	id = normalizeStyle(id)
	for _, s := range styleList() {
		if s.id == id {
			return s.label
		}
	}
	return builtinStyles[0].label
}

func styleChoices() string {
	list := styleList()
	parts := make([]string, len(list))
	for i, s := range list {
		parts[i] = s.label
	}
	return strings.Join(parts, ",")
}
