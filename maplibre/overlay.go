package maplibre

import (
	"image"
	"image/color"
	"math"

	"github.com/mrcyjanek/cargadgetd_app_maps/render"
)

// The route stays off the road greens. The arrow matches the vector one.
var (
	colRoute = color.NRGBA{R: 70, G: 170, B: 255, A: 255}
	colLime  = color.NRGBA{R: 212, G: 255, B: 0, A: 255}
	colEdge  = color.NRGBA{R: 8, G: 12, B: 8, A: 255}
)

// paintStillMarks strokes the route and the position through the still's
// camera. Following puts the arrow on the center of that picture, which is
// where MapLibre drew the camera. A browse uses the projected car.
func paintStillMarks(dst *image.RGBA, marks render.Marks) {
	if dst == nil {
		return
	}
	b := dst.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return
	}
	pts := marks.Route
	car := -1
	if !marks.Follow && marks.HasCar {
		car = len(pts)
		pts = append(append([]render.LatLng{}, pts...), marks.Car)
	}
	var xs, ys []float64
	if len(pts) > 0 {
		var ok bool
		xs, ys, ok = projectStill(pts)
		if !ok || len(xs) != len(pts) || len(ys) != len(pts) {
			xs, ys = nil, nil
		}
	}
	if xs != nil {
		strokeRoute(dst, xs[:len(marks.Route)], ys[:len(marks.Route)])
	}
	if marks.Follow && marks.HasCar {
		paintArrow(dst, b.Min.X+w/2, b.Min.Y+h/2)
		return
	}
	if car >= 0 && xs != nil && pointOnScreen(xs[car], ys[car], w, h) {
		paintArrow(dst, b.Min.X+int(math.Round(xs[car])), b.Min.Y+int(math.Round(ys[car])))
	}
}

func pointOnScreen(x, y float64, w, h int) bool {
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return false
	}
	return x >= -48 && y >= -48 && x < float64(w+48) && y < float64(h+48)
}

func strokeRoute(dst *image.RGBA, xs, ys []float64) {
	if len(xs) < 2 || len(ys) < 2 {
		return
	}
	var px, py float64
	var have bool
	for i := range xs {
		if i >= len(ys) || !pointOnScreen(xs[i], ys[i], dst.Bounds().Dx(), dst.Bounds().Dy()) {
			have = false
			continue
		}
		if !have {
			px, py = xs[i], ys[i]
			have = true
			continue
		}
		dx, dy := xs[i]-px, ys[i]-py
		if dx*dx+dy*dy < 4 && i != len(xs)-1 {
			continue
		}
		stroke(dst, px, py, xs[i], ys[i], 5, colRoute)
		px, py = xs[i], ys[i]
	}
}

func stroke(dst *image.RGBA, x0, y0, x1, y1 float64, thick int, c color.NRGBA) {
	n := math.Hypot(x1-x0, y1-y0)
	if n < 0.5 {
		set(dst, int(math.Round(x0)), int(math.Round(y0)), c)
		return
	}
	steps := int(n)
	if steps < 1 {
		steps = 1
	}
	px, py := -(y1-y0)/n, (x1-x0)/n
	half := float64(thick-1) / 2
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x0 + (x1-x0)*t
		y := y0 + (y1-y0)*t
		for s := -half; s <= half+0.01; s++ {
			set(dst, int(math.Round(x+px*s)), int(math.Round(y+py*s)), c)
		}
	}
}

func paintArrow(dst *image.RGBA, cx, cy int) {
	fillTri(dst, float64(cx), float64(cy-20), float64(cx-16), float64(cy+14), float64(cx+16), float64(cy+14), colEdge)
	fillTri(dst, float64(cx), float64(cy-16), float64(cx-12), float64(cy+10), float64(cx+12), float64(cy+10), colLime)
}

func fillTri(dst *image.RGBA, x0, y0, x1, y1, x2, y2 float64, c color.NRGBA) {
	minX := int(math.Floor(math.Min(x0, math.Min(x1, x2))))
	maxX := int(math.Ceil(math.Max(x0, math.Max(x1, x2))))
	minY := int(math.Floor(math.Min(y0, math.Min(y1, y2))))
	maxY := int(math.Ceil(math.Max(y0, math.Max(y1, y2))))
	b := dst.Bounds()
	if minX < b.Min.X {
		minX = b.Min.X
	}
	if minY < b.Min.Y {
		minY = b.Min.Y
	}
	if maxX >= b.Max.X {
		maxX = b.Max.X - 1
	}
	if maxY >= b.Max.Y {
		maxY = b.Max.Y - 1
	}
	den := (y1-y2)*(x0-x2) + (x2-x1)*(y0-y2)
	if math.Abs(den) < 1e-6 {
		return
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			a := ((y1-y2)*(fx-x2) + (x2-x1)*(fy-y2)) / den
			b := ((y2-y0)*(fx-x2) + (x0-x2)*(fy-y2)) / den
			g := 1 - a - b
			if a >= 0 && b >= 0 && g >= 0 {
				set(dst, x, y, c)
			}
		}
	}
}

func set(dst *image.RGBA, x, y int, c color.NRGBA) {
	if dst == nil || !image.Pt(x, y).In(dst.Bounds()) {
		return
	}
	i := dst.PixOffset(x, y)
	dst.Pix[i] = c.R
	dst.Pix[i+1] = c.G
	dst.Pix[i+2] = c.B
	dst.Pix[i+3] = c.A
}
