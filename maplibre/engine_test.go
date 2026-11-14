package maplibre

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/mrcyjanek/cargadgetd_app_maps/render"
)

func TestRenderReturnsNothing(t *testing.T) {
	e := New()
	cam := render.Cam{Lat: 50.2, Lon: 19.1, Zoom: 15, Heading: 10, Has: true}
	job := render.Job{Cam: cam, W: 80, H: 60}
	pic := e.Prepare(job)
	if pic.Img != nil || !pic.Ready {
		t.Fatalf("picture %+v", pic)
	}
	e.Publish(pic, "ml")
	dst := image.NewRGBA(image.Rect(0, 0, 80, 60))
	if e.DrawFrame(dst, cam, render.Marks{}, job.Now) {
		t.Fatal("empty still drew")
	}
	if e.JobStamp(cam, "", 80, 60) != "" {
		t.Fatal("empty still asked for another build")
	}
}

func TestRenderNotReadyAsksAgain(t *testing.T) {
	prev := Render
	Render = func(render.Job) Still { return Still{} }
	t.Cleanup(func() { Render = prev })

	e := New()
	cam := render.Cam{Lat: 50.2, Lon: 19.1, Zoom: 15, Has: true}
	pic := e.Prepare(render.Job{Cam: cam, W: 80, H: 60})
	if pic.Ready || pic.Img != nil {
		t.Fatalf("picture %+v", pic)
	}
	if e.JobStamp(cam, "", 80, 60) == "" {
		t.Fatal("unfinished still stopped the painter")
	}
}

func TestJobStampChangesWithStyle(t *testing.T) {
	e := New()
	cam := render.Cam{Lat: 50.2, Lon: 19.1, Zoom: 15, Has: true}
	e.Publish(render.Picture{Cam: cam, W: 80, H: 60, Ready: true, Style: "osmbright"}, "ml")
	if e.JobStamp(cam, "osmbright", 80, 60) != "" {
		t.Fatal("same style asked for another still")
	}
	if e.JobStamp(cam, "mc", 80, 60) == "" {
		t.Fatal("style change kept the old still")
	}
}

func TestReturnedImageIsCopied(t *testing.T) {
	prev := Render
	ink := color.RGBA{R: 10, G: 20, B: 200, A: 255}
	Render = func(job render.Job) Still {
		img := image.NewRGBA(image.Rect(0, 0, job.W, job.H))
		img.SetRGBA(4, 4, ink)
		return Still{Img: img, Ready: true}
	}
	t.Cleanup(func() { Render = prev })

	e := New()
	cam := render.Cam{Lat: 50.2, Lon: 19.1, Zoom: 15, Has: true}
	pic := e.Prepare(render.Job{Cam: cam, W: 80, H: 60})
	e.Publish(pic, "ml")
	dst := image.NewRGBA(image.Rect(0, 0, 80, 60))
	if !e.DrawFrame(dst, cam, render.Marks{}, time.Time{}) {
		t.Fatal("still was not copied")
	}
	c := dst.RGBAAt(4, 4)
	if c.R != ink.R || c.G != ink.G || c.B != ink.B {
		t.Fatalf("pixel %+v", c)
	}
}
