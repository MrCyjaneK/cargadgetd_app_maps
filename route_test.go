package main

import (
	"image"
	"testing"

	"github.com/mrcyjanek/cargadgetd_app_maps/nav"
)

func latLonAt(cam mapCam, sx, sy, w, h int) nav.Point {
	z := cam.sampleZoom()
	wx, wy := cam.screenWorld(sx, sy, w, h, z)
	lat, lon := worldToLatLon(wx, wy, z)
	return nav.Point{Lat: lat, Lon: lon}
}

func TestPaintRouteLine(t *testing.T) {
	cam := mapCam{lat: 50.2, lon: 19.1, zoom: 15, heading: 0, pitch: 0, has: true}
	w, h := 200, 120
	a := latLonAt(cam, 30, 40, w, h)
	b := latLonAt(cam, 170, 90, w, h)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(dst, dst.Bounds(), colLand)
	paintRouteLine(dst, cam, []nav.Point{a, b})
	mid := dst.RGBAAt(100, 65)
	if mid.R == colLand.R && mid.G == colLand.G && mid.B == colLand.B {
		t.Fatalf("route missing at mid %+v", mid)
	}
	if dst.RGBAAt(30, 40).R != colRoute.R || dst.RGBAAt(170, 90).B != colRoute.B {
		t.Fatalf("ends %+v %+v", dst.RGBAAt(30, 40), dst.RGBAAt(170, 90))
	}

	fillRect(dst, dst.Bounds(), colLand)
	paintRouteLine(dst, cam, []nav.Point{{Lat: 10, Lon: 10}, {Lat: 11, Lon: 11}})
	mid = dst.RGBAAt(100, 65)
	if mid.R != colLand.R || mid.G != colLand.G || mid.B != colLand.B {
		t.Fatalf("far route painted %+v", mid)
	}
}

func TestPaintRouteOnDrive(t *testing.T) {
	cam := mapCam{lat: 50.2, lon: 19.1, zoom: 15, heading: 0, pitch: 0, has: true}
	w, h := 200, 120
	prev := openNav
	openNav = func(string) nav.Source {
		return stubNav{path: []nav.Point{
			latLonAt(cam, 40, 30, w, h),
			latLonAt(cam, 150, 80, w, h),
		}}
	}
	t.Cleanup(func() { openNav = prev })
	d := &drive{cam: cam, w: w, h: h}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(dst, dst.Bounds(), colLand)
	plateHost{}.PaintMarks(dst, renderCam(d.cam), d.frameMarks())
	p := dst.RGBAAt(95, 55)
	if p.R == colLand.R && p.G == colLand.G && p.B == colLand.B {
		t.Fatalf("drive route missing %+v", p)
	}
	d.closeNav()
}
