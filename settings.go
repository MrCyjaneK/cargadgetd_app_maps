package main

import "sync"

const (
	mapSetSource = iota
	mapSetStyle
	mapSetRenderer
	mapSetTilt
	mapSetAhead
	mapSetCount
)

var (
	mapSettingsMu  sync.Mutex
	mapSettingsErr string
)

func mapSettingsCount() int { return mapSetCount }

func mapSettingsLabel(i int) string {
	switch i {
	case mapSetSource:
		return "Map"
	case mapSetStyle:
		return "Style"
	case mapSetRenderer:
		return "Renderer"
	case mapSetTilt:
		return "Map tilt"
	case mapSetAhead:
		return "Further while driving"
	default:
		return ""
	}
}

func mapSettingsType(i int) int {
	switch i {
	case mapSetSource, mapSetStyle, mapSetRenderer, mapSetTilt:
		return 1
	case mapSetAhead:
		return 2
	default:
		return -1
	}
}

func mapSettingsEnums(i int) string {
	switch i {
	case mapSetSource:
		return "OSM,Glitch"
	case mapSetStyle:
		return styleChoices()
	case mapSetRenderer:
		return "Vector,MapLibre"
	case mapSetTilt:
		return "Always,While driving,Never"
	default:
		return ""
	}
}

func mapSettingsGetEnum(i int) string {
	app.d.mu.Lock()
	defer app.d.mu.Unlock()
	switch i {
	case mapSetSource:
		return mapLabel(app.d.mapKind)
	case mapSetStyle:
		return styleLabel(app.d.mapStyle)
	case mapSetRenderer:
		return rendererLabel(rendererOf(app.d.engine))
	case mapSetTilt:
		return tiltLabel(app.d.mapTilt)
	default:
		return ""
	}
}

func mapSettingsGetInt(i int) int {
	if i != mapSetAhead {
		return 0
	}
	app.d.mu.Lock()
	v := app.d.mapAhead
	app.d.mu.Unlock()
	return int(clampAhead(v) + 0.5)
}

func mapSettingsSetEnum(i int, v string) int {
	mapClearErr()
	if !mapToken(i, v) {
		return mapFail("unknown value")
	}
	var err error
	switch i {
	case mapSetSource:
		err = app.d.setMapMode(v)
	case mapSetStyle:
		err = app.d.setMapStyle(v)
	case mapSetRenderer:
		err = app.d.setRenderer(v)
	case mapSetTilt:
		err = app.d.setMapTilt(v)
	default:
		return mapFail("not an enum")
	}
	if err != nil {
		return mapFail(err.Error())
	}
	return 0
}

func mapSettingsSetInt(i, v int) int {
	mapClearErr()
	if i != mapSetAhead {
		return mapFail("not an int")
	}
	if v < int(aheadMin) || v > int(aheadMax) {
		return mapFail("out of range")
	}
	if err := app.d.setMapAhead(float64(v)); err != nil {
		return mapFail(err.Error())
	}
	return 0
}

func mapToken(i int, v string) bool {
	raw := mapSettingsEnums(i)
	if raw == "" {
		return false
	}
	for _, t := range splitComma(raw) {
		if t == v {
			return true
		}
	}
	return false
}

func splitComma(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}

func mapClearErr() {
	mapSettingsMu.Lock()
	mapSettingsErr = ""
	mapSettingsMu.Unlock()
}

func mapFail(msg string) int {
	mapSettingsMu.Lock()
	mapSettingsErr = msg
	mapSettingsMu.Unlock()
	return 1
}
