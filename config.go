package main

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mrcyjanek/cargadgetd_sdk/cfg"
)

// configUser, when set, is the path callers pass into the patch helpers.
// Those helpers write the maps plugin file and ignore the path.
var configUser string

func userConfigPath() string {
	if configUser != "" {
		return configUser
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "cargadgetd", "config.yaml")
}

func normalizeRenderer(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "maplibre", "libre":
		return rendererMapLibre
	default:
		return rendererVector
	}
}

func loadRenderer() string {
	if v, ok := cfg.OwnGet("maps", "renderer"); ok && strings.TrimSpace(v) != "" {
		return normalizeRenderer(v)
	}
	return rendererVector
}

func patchRenderer(kind string) error {
	return cfg.OwnSet("maps", "renderer", normalizeRenderer(kind))
}

func loadStyle() string {
	if v, ok := cfg.OwnGet("maps", "style"); ok && strings.TrimSpace(v) != "" {
		return normalizeStyle(v)
	}
	return styleOSMBright
}

func patchStyle(kind string) error {
	return cfg.OwnSet("maps", "style", normalizeStyle(kind))
}

func loadMap() string {
	if v, ok := cfg.OwnGet("maps", "source"); ok && strings.TrimSpace(v) != "" {
		return normalizeMap(v)
	}
	return mapOSM
}

func patchMap(path, kind string) error {
	return cfg.OwnSet("maps", "source", normalizeMap(kind))
}

func normalizeTilt(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "always":
		return tiltAlways
	case "never", "off":
		return tiltNever
	case "driving", "drive", "while driving":
		return tiltDriving
	default:
		return tiltDriving
	}
}

func tiltLabel(mode string) string {
	switch normalizeTilt(mode) {
	case tiltAlways:
		return "Always"
	case tiltNever:
		return "Never"
	default:
		return "While driving"
	}
}

func loadTilt() string {
	if v, ok := cfg.OwnGet("maps", "tilt"); ok && strings.TrimSpace(v) != "" {
		return normalizeTilt(v)
	}
	return tiltDriving
}

func patchTilt(path, mode string) error {
	return cfg.OwnSet("maps", "tilt", normalizeTilt(mode))
}

func loadAhead() float64 {
	if v, ok := cfg.OwnGet("maps", "ahead"); ok && strings.TrimSpace(v) != "" {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return clampAhead(f)
		}
	}
	return aheadDefault
}

func patchAhead(path string, scale float64) error {
	return cfg.OwnSet("maps", "ahead", strconv.Itoa(int(math.Round(clampAhead(scale)))))
}

func loadNav() string {
	if v, ok := cfg.OwnGet("maps", "nav"); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return "puremaps"
}
