package maplibre

import (
	"image"
	"testing"
	"time"

	"github.com/mrcyjanek/cargadgetd_app_maps/render"
)

func TestStillMarksFollowCenterAndRoute(t *testing.T) {
	prev := projectStill
	projectStill = func(pts []render.LatLng) ([]float64, []float64, bool) {
		xs := make([]float64, len(pts))
		ys := make([]float64, len(pts))
		for i := range pts {
			xs[i] = 12
			ys[i] = 8 + float64(i)*30
		}
		return xs, ys, true
	}
	t.Cleanup(func() { projectStill = prev })

	e := New()
	img := image.NewRGBA(image.Rect(0, 0, 80, 60))
	e.Publish(render.Picture{Img: img, W: 80, H: 60, Ready: true}, "ml")
	dst := image.NewRGBA(image.Rect(0, 0, 80, 60))
	marks := render.Marks{
		Route:  []render.LatLng{{Lat: 1, Lon: 2}, {Lat: 3, Lon: 4}},
		HasCar: true,
		Follow: true,
	}
	if !e.DrawFrame(dst, render.Cam{Has: true}, marks, time.Time{}) {
		t.Fatal("still did not draw")
	}
	mid := dst.RGBAAt(12, 23)
	if mid.R != colRoute.R || mid.G != colRoute.G || mid.B != colRoute.B {
		t.Fatalf("route %+v", mid)
	}
	arrow := dst.RGBAAt(40, 22)
	if arrow.R != colLime.R || arrow.G != colLime.G {
		t.Fatalf("follow arrow %+v", arrow)
	}
}

func TestStillMarksBrowseProjectsCar(t *testing.T) {
	prev := projectStill
	projectStill = func(pts []render.LatLng) ([]float64, []float64, bool) {
		xs := make([]float64, len(pts))
		ys := make([]float64, len(pts))
		xs[len(pts)-1] = 18
		ys[len(pts)-1] = 40
		return xs, ys, true
	}
	t.Cleanup(func() { projectStill = prev })

	e := New()
	e.Publish(render.Picture{Img: image.NewRGBA(image.Rect(0, 0, 80, 60)), W: 80, H: 60, Ready: true}, "ml")
	dst := image.NewRGBA(image.Rect(0, 0, 80, 60))
	marks := render.Marks{HasCar: true, Car: render.LatLng{Lat: 50, Lon: 19}}
	if !e.DrawFrame(dst, render.Cam{Has: true}, marks, time.Time{}) {
		t.Fatal("still did not draw")
	}
	if dst.RGBAAt(40, 22).R == colLime.R && dst.RGBAAt(40, 22).G == colLime.G {
		t.Fatal("browse arrow stayed on the center")
	}
	got := dst.RGBAAt(18, 30)
	if got.R != colLime.R || got.G != colLime.G {
		t.Fatalf("projected arrow %+v", got)
	}
}
