// Package render is the drive-map picture. The vector plate and the
// future MapLibre still each implement Engine, and the maps app does
// not know which one filled the frame.
package render

import (
	"image"
	"time"
)

// Cam is the view the engine draws. Pitch is 0 when flat and 1 at full tilt.
type Cam struct {
	Lat, Lon float64
	Heading  float64
	Zoom     float64
	Pitch    float64
	Has      bool
}

// LatLng is one ground point, in degrees.
type LatLng struct {
	Lat, Lon float64
}

// Marks is the route and the position a backend draws with its own camera.
// Follow means the camera is on the car: the vector picture puts the arrow
// on its anchor, and a MapLibre still puts it on the center of that still.
// A browse projects the car instead.
type Marks struct {
	Route  []LatLng
	Car    LatLng
	HasCar bool
	Follow bool
}

// Job is one build requested off the video clock. Style is an OSM Scout
// Mapbox style id, such as osmbright. An empty style is that default.
type Job struct {
	Cam   Cam
	W, H  int
	Now   time.Time
	Stamp string
	Style string
}

// Plate is a north-up picture the vector engine samples into the frame.
type Plate interface {
	Covers(cam Cam, w, h int) bool
	Stamp() string
	SetStamp(string)
	Ready() bool
	Sample(dst *image.RGBA, cam Cam, now time.Time)
}

// Picture is one finished build. The drive publishes it only when the
// camera has not moved on to another job.
type Picture struct {
	Plate Plate
	Img   *image.RGBA
	Cam   Cam
	W, H  int
	Ready bool
	Style string
	// ZoomMiss is set when the picture is the closest zoom that had tiles.
	// DrawnZoom is that zoom.
	ZoomMiss  bool
	DrawnZoom float64
}

// Engine builds the drive picture off the video clock. The frame copies
// whatever the engine last published. Each backend draws the route and the
// position in its own projection. The maneuver plate stays outside.
type Engine interface {
	ID() string
	// JobStamp is the picture to build, or "" when the current one still covers.
	// style is the OSM Scout style id. An empty string is osmbright.
	JobStamp(cam Cam, style string, w, h int) string
	Prepare(job Job) Picture
	Publish(p Picture, stamp string)
	// DrawFrame paints the published picture and the marks. False means there
	// is nothing to show.
	DrawFrame(dst *image.RGBA, cam Cam, marks Marks, now time.Time) bool
	Down() bool
	Pause()
}
