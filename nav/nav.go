// Package nav is the Pure Maps navigator behind the maps app.
// A new backend implements Source and registers itself with Register.
package nav

import "strings"

// ErrNotRunning is the snapshot error when Pure Maps is not on the bus.
const ErrNotRunning = "Pure Maps not running"

// Trip is one navigator snapshot. Strings are Pure Maps' own text.
// Snapshot must not block. Close stops any background poll and may be
// called more than once.
type Trip struct {
	OK             bool
	Err            string
	HasRoute       bool
	Running        bool
	AlongRoute     bool
	Direction      float64
	DirectionValid bool
	Icon           string
	ManDist        string
	ManTime        string
	Street         string
	Narrative      string
	DestDist       string
	DestTime       string
	DestEta        string
	Progress       int
	RoundaboutExit int
}

// Point is one vertex of the route, in degrees.
type Point struct {
	Lat, Lon float64
}

// Source is one navigation backend.
// Route returns the last path. The slice is not modified. Snapshot and
// Route must not block.
type Source interface {
	Snapshot() Trip
	Route() []Point
	Close()
}

// Backend is one value of maps.nav.
type Backend struct {
	ID string
}

// Backends is the known list. There is no settings control for this.
func Backends() []Backend {
	return []Backend{
		{ID: "mock"},
		{ID: "puremaps"},
	}
}

// Normalize maps a config value onto a known backend id.
// Empty and unknown values are puremaps.
func Normalize(kind string) string {
	kind = strings.TrimSpace(kind)
	for _, b := range Backends() {
		if strings.EqualFold(kind, b.ID) {
			return b.ID
		}
	}
	return "puremaps"
}

// Plain drops a blank Pure Maps field. "-" is the app's empty marker.
func Plain(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return ""
	}
	return s
}

// Remain is remaining time, then remaining distance.
func Remain(t Trip) string {
	var parts []string
	if s := Plain(t.DestTime); s != "" {
		parts = append(parts, s)
	}
	if s := Plain(t.DestDist); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, " · ")
}

var backends = map[string]func() Source{}

// Register adds a backend under maps.nav. A later registration replaces
// the same id.
func Register(id string, open func() Source) {
	if id == "" || open == nil {
		return
	}
	backends[id] = open
}

func init() {
	Register("mock", func() Source { return mockSource{} })
	Register("puremaps", openPureMaps)
}

// Open returns the named backend. Empty and unknown names are puremaps.
func Open(kind string) Source {
	kind = Normalize(kind)
	if open, ok := backends[kind]; ok && open != nil {
		return open()
	}
	return openPureMaps()
}

type mockSource struct{}

// Snapshot is the demo trip. It does not touch the bus.
func (mockSource) Snapshot() Trip {
	return Trip{
		OK:        true,
		HasRoute:  true,
		Running:   true,
		Icon:      "turn-right",
		ManDist:   "200 m",
		Street:    "High Street",
		Narrative: "Turn right onto High Street",
		DestTime:  "18 min",
		DestDist:  "12 km",
	}
}

func (mockSource) Route() []Point { return nil }

func (mockSource) Close() {}
