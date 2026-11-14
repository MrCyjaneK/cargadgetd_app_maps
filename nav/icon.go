package nav

import (
	"os"
	"path/filepath"
	"strings"
)

// GlyphDirs are the Pure Maps maneuver glyphs. Older packages use qml/.
// 3.5.1 uses pm/. Tests replace the list.
var GlyphDirs = []string{
	"/usr/share/harbour-pure-maps/qml/icons/navigation",
	"/usr/share/harbour-pure-maps/pm/icons/navigation",
}

// GlyphPath is the white maneuver SVG for a navigator icon name.
// Both Pure Maps directories are searched, qml first. An empty name,
// a slash, or ".." yields no path.
func GlyphPath(icon string) string {
	icon = strings.TrimSpace(icon)
	if icon == "" || strings.Contains(icon, "/") || strings.Contains(icon, "..") {
		return ""
	}
	names := []string{icon + "-white.svg", icon + ".svg"}
	for _, dir := range GlyphDirs {
		for _, name := range names {
			p := filepath.Join(dir, name)
			if isFile(p) {
				return p
			}
		}
	}
	return ""
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
