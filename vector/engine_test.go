package vector

import (
	"image"
	"testing"
	"time"

	"github.com/mrcyjanek/cargadgetd_app_maps/render"
)

type fakePlate struct {
	covers bool
	stamp  string
	ready  bool
	sample int
}

func (p *fakePlate) Covers(render.Cam, int, int) bool { return p.covers }
func (p *fakePlate) Stamp() string                    { return p.stamp }
func (p *fakePlate) SetStamp(s string)                { p.stamp = s }
func (p *fakePlate) Ready() bool                      { return p.ready }
func (p *fakePlate) Sample(*image.RGBA, render.Cam, time.Time) {
	p.sample++
}

type fakeHost struct {
	stamp string
	plate render.Plate
	retry bool
	down  bool
	pause int
	marks int
}

func (h *fakeHost) Stamp(render.Cam, int, int) string { return h.stamp }
func (h *fakeHost) Build(render.Job) render.Plate     { return h.plate }
func (h *fakeHost) Retry() bool                       { return h.retry }
func (h *fakeHost) Down() bool                        { return h.down }
func (h *fakeHost) Pause()                            { h.pause++ }
func (h *fakeHost) PaintMarks(*image.RGBA, render.Cam, render.Marks) {
	h.marks++
}

func TestDrawFramePaintsMarks(t *testing.T) {
	plate := &fakePlate{ready: true}
	h := &fakeHost{plate: plate}
	e := New(h)
	e.Publish(render.Picture{Plate: plate, Ready: true}, "a")
	dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if !e.DrawFrame(dst, render.Cam{Has: true}, render.Marks{Follow: true}, time.Time{}) {
		t.Fatal("ready plate did not draw")
	}
	if plate.sample != 1 || h.marks != 1 {
		t.Fatalf("sample %d marks %d", plate.sample, h.marks)
	}
}

func TestPlateCoversSkipsRebuild(t *testing.T) {
	h := &fakeHost{stamp: "a", plate: &fakePlate{covers: true, ready: true}}
	e := New(h)
	pic := e.Prepare(render.Job{})
	e.Publish(pic, "a")
	if e.JobStamp(render.Cam{}, "", 10, 10) != "" {
		t.Fatal("covered plate asked for another build")
	}
}

func TestUncoveredPlateBuildsOnceMore(t *testing.T) {
	plate := &fakePlate{ready: true}
	h := &fakeHost{stamp: "a", plate: plate, retry: true}
	e := New(h)
	e.Publish(e.Prepare(render.Job{}), "a")
	if got := e.JobStamp(render.Cam{}, "", 10, 10); got != "a|+" {
		t.Fatalf("stamp %q", got)
	}
	e.Publish(e.Prepare(render.Job{}), "a|+")
	if e.JobStamp(render.Cam{}, "", 10, 10) != "" {
		t.Fatal("second miss kept rebuilding")
	}
}

func TestMissingPlateWaits(t *testing.T) {
	h := &fakeHost{retry: true}
	e := New(h)
	if e.Prepare(render.Job{}).Ready {
		t.Fatal("missing plate was ready")
	}
	h.retry = false
	if !e.Prepare(render.Job{}).Ready {
		t.Fatal("offline miss kept waiting")
	}
}
