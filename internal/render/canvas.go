package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"

	"golang.org/x/image/vector"

	"github.com/MatthewPocock/tuck/internal/deck"
)

// kappa is the control point offset that makes a cubic Bézier approximate a
// quarter circle.
const kappa = 0.5522847498

// Canvas draws anti-aliased shapes onto an RGBA image. Its pixels use
// premultiplied alpha, which is also what layered windows expect.
type Canvas struct {
	Img *image.RGBA
	ras vector.Rasterizer
}

// NewCanvas returns a fully transparent canvas of the given size.
func NewCanvas(width, height int) *Canvas {
	return &Canvas{Img: image.NewRGBA(image.Rect(0, 0, max(width, 1), max(height, 1)))}
}

// Resize makes the canvas the given size, reusing its buffer when possible,
// and clears it.
func (c *Canvas) Resize(width, height int) {
	width, height = max(width, 1), max(height, 1)
	if b := c.Img.Bounds(); b.Dx() != width || b.Dy() != height {
		c.Img = image.NewRGBA(image.Rect(0, 0, width, height))
		return
	}
	c.Clear()
}

// Clear makes every pixel transparent.
func (c *Canvas) Clear() {
	clear(c.Img.Pix)
}

// Fill paints the whole canvas with an opaque colour.
func (c *Canvas) Fill(col color.NRGBA) {
	draw.Draw(c.Img, c.Img.Bounds(), image.NewUniform(col), image.Point{}, draw.Src)
}

// FillRect blends a solid rectangle onto the canvas.
func (c *Canvas) FillRect(r deck.Rect, col color.NRGBA) {
	c.FillRoundRect(r, 0, col)
}

// FillRoundRect blends a rounded rectangle onto the canvas.
func (c *Canvas) FillRoundRect(r deck.Rect, radius float64, col color.NRGBA) {
	if r.Empty() || col.A == 0 {
		return
	}
	c.path(r, col, func(ox, oy float32) {
		roundRect(&c.ras, r, radius, ox, oy, false)
	})
}

// StrokeRoundRect blends the outline of a rounded rectangle, drawn inside r,
// onto the canvas.
func (c *Canvas) StrokeRoundRect(r deck.Rect, radius, width float64, col color.NRGBA) {
	if r.Empty() || col.A == 0 || width <= 0 {
		return
	}
	inner := r.Inflate(-width, -width)
	if inner.Empty() {
		c.FillRoundRect(r, radius, col)
		return
	}
	radius = math.Max(0, math.Min(radius, math.Min(r.W, r.H)/2))
	if radius < width || r.W*r.H < 64*64 {
		c.path(r, col, func(ox, oy float32) {
			roundRect(&c.ras, r, radius, ox, oy, false)
			// Winding the inner path the other way punches it out.
			roundRect(&c.ras, inner, max(radius-width, 0), ox, oy, true)
		})
		return
	}
	// Rasterising costs as much as the area around the shape, so a large
	// outline is drawn as four thin edges and four small corners rather
	// than as one ring.
	x0, y0, x1, y1 := r.X, r.Y, r.X+r.W, r.Y+r.H
	c.FillRect(deck.Rect{X: x0 + radius, Y: y0, W: r.W - 2*radius, H: width}, col)
	c.FillRect(deck.Rect{X: x0 + radius, Y: y1 - width, W: r.W - 2*radius, H: width}, col)
	c.FillRect(deck.Rect{X: x0, Y: y0 + radius, W: width, H: r.H - 2*radius}, col)
	c.FillRect(deck.Rect{X: x1 - width, Y: y0 + radius, W: width, H: r.H - 2*radius}, col)
	for _, k := range [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		cx, cy := x0+radius, y0+radius
		if k[0] > 0 {
			cx = x1 - radius
		}
		if k[1] > 0 {
			cy = y1 - radius
		}
		c.strokeCorner(cx, cy, k[0], k[1], radius, radius-width, col)
	}
}

// strokeCorner fills a quarter of a ring centred on (cx, cy) between radii
// inner and outer, in the quadrant that (dx, dy) points to.
func (c *Canvas) strokeCorner(cx, cy, dx, dy, outer, inner float64, col color.NRGBA) {
	box := deck.Rect{X: math.Min(cx, cx+dx*outer), Y: math.Min(cy, cy+dy*outer), W: outer, H: outer}
	c.path(box, col, func(ox, oy float32) {
		pt := func(x, y float64) (float32, float32) { return float32(x) - ox, float32(y) - oy }
		z := &c.ras
		z.MoveTo(pt(cx+dx*outer, cy))
		a1, a2 := pt(cx+dx*outer, cy+dy*outer*kappa)
		b1, b2 := pt(cx+dx*outer*kappa, cy+dy*outer)
		e1, e2 := pt(cx, cy+dy*outer)
		z.CubeTo(a1, a2, b1, b2, e1, e2)
		z.LineTo(pt(cx, cy+dy*inner))
		a1, a2 = pt(cx+dx*inner*kappa, cy+dy*inner)
		b1, b2 = pt(cx+dx*inner, cy+dy*inner*kappa)
		e1, e2 = pt(cx+dx*inner, cy)
		z.CubeTo(a1, a2, b1, b2, e1, e2)
		z.ClosePath()
	})
}

// FillCircle blends a filled circle onto the canvas.
func (c *Canvas) FillCircle(cx, cy, radius float64, col color.NRGBA) {
	c.FillRoundRect(deck.Rect{X: cx - radius, Y: cy - radius, W: 2 * radius, H: 2 * radius}, radius, col)
}

// StrokeCircle blends a circle outline of the given line width, centred on
// the circle's radius, onto the canvas.
func (c *Canvas) StrokeCircle(cx, cy, radius, width float64, col color.NRGBA) {
	outer := radius + width/2
	c.StrokeRoundRect(deck.Rect{X: cx - outer, Y: cy - outer, W: 2 * outer, H: 2 * outer}, outer, width, col)
}

// Line blends a line with round caps onto the canvas.
func (c *Canvas) Line(x1, y1, x2, y2, width float64, col color.NRGBA) {
	if col.A == 0 || width <= 0 {
		return
	}
	hw := width / 2
	dx, dy := x2-x1, y2-y1
	length := math.Hypot(dx, dy)
	if length == 0 {
		c.FillCircle(x1, y1, hw, col)
		return
	}
	// (ux, uy) is the half-width vector along the line and (nx, ny) the one
	// perpendicular to it.
	ux, uy := dx/length*hw, dy/length*hw
	nx, ny := -uy, ux

	bounds := deck.Rect{
		X: math.Min(x1, x2) - hw, Y: math.Min(y1, y2) - hw,
		W: math.Abs(dx) + width, H: math.Abs(dy) + width,
	}
	c.path(bounds, col, func(ox, oy float32) {
		pt := func(x, y float64) (float32, float32) { return float32(x) - ox, float32(y) - oy }
		move := func(x, y float64) { c.ras.MoveTo(pt(x, y)) }
		line := func(x, y float64) { c.ras.LineTo(pt(x, y)) }
		cube := func(ax, ay, bx, by, cx, cy float64) {
			a1, a2 := pt(ax, ay)
			b1, b2 := pt(bx, by)
			c1, c2 := pt(cx, cy)
			c.ras.CubeTo(a1, a2, b1, b2, c1, c2)
		}
		// cap draws a semicircle around (px, py) bulging toward (sx, sy),
		// starting on the side given by (mx, my).
		cap := func(px, py, sx, sy, mx, my float64) {
			cube(px+mx+kappa*sx, py+my+kappa*sy, px+sx+kappa*mx, py+sy+kappa*my, px+sx, py+sy)
			cube(px+sx-kappa*mx, py+sy-kappa*my, px-mx+kappa*sx, py-my+kappa*sy, px-mx, py-my)
		}
		move(x1+nx, y1+ny)
		line(x2+nx, y2+ny)
		cap(x2, y2, ux, uy, nx, ny)
		line(x1-nx, y1-ny)
		cap(x1, y1, -ux, -uy, -nx, -ny)
		c.ras.ClosePath()
	})
}

// Shadow blends a soft drop shadow for a rounded rectangle onto the canvas.
// blur is how far the shadow spreads, dy how far it is offset downward and
// opacity its darkness directly beneath the shape.
//
// Shadows are drawn on every frame of an animation, so rather than
// rasterising the whole shape each time, the shadow of a small rectangle with
// the same corners is drawn once and stretched to size: its corners are
// copied, its edges repeated and its middle filled.
func (c *Canvas) Shadow(r deck.Rect, radius, blur, dy, opacity float64) {
	if opacity <= 0 || blur <= 0 {
		return
	}
	sl := shadowSlice(radius, blur, opacity)
	n := sl.corner
	x0 := int(math.Round(r.X - blur))
	y0 := int(math.Round(r.Y + dy - blur))
	w := int(math.Round(r.W + 2*blur))
	h := int(math.Round(r.H + 2*blur))
	if w < 2*n+1 || h < 2*n+1 {
		c.layeredShadow(r, radius, blur, dy, opacity)
		return
	}
	// Map each target pixel to a pixel of the slice: corners one to one,
	// edges and middle from the slice's centre row or column.
	sx := func(x int) int {
		switch {
		case x < n:
			return x
		case x >= w-n:
			return sl.size - (w - x)
		}
		return n
	}
	bounds := c.Img.Bounds()
	for y := max(0, -y0); y < h; y++ {
		ty := y0 + y
		if ty >= bounds.Max.Y {
			break
		}
		syy := sx2(y, h, n, sl.size)
		row := sl.alpha[syy*sl.size : (syy+1)*sl.size]
		for x := max(0, -x0); x < w; x++ {
			tx := x0 + x
			if tx >= bounds.Max.X {
				break
			}
			if a := row[sx(x)]; a != 0 {
				darken(c.Img, tx, ty, a)
			}
		}
	}
}

// sx2 maps a row of a stretched shadow to a row of its slice.
func sx2(y, h, n, size int) int {
	switch {
	case y < n:
		return y
	case y >= h-n:
		return size - (h - y)
	}
	return n
}

// darken blends black at coverage a onto a premultiplied pixel.
func darken(img *image.RGBA, x, y int, a uint8) {
	i := img.PixOffset(x, y)
	p := img.Pix[i : i+4 : i+4]
	k := 255 - uint32(a)
	p[0] = uint8((uint32(p[0])*k + 127) / 255)
	p[1] = uint8((uint32(p[1])*k + 127) / 255)
	p[2] = uint8((uint32(p[2])*k + 127) / 255)
	p[3] = uint8(uint32(a) + (uint32(p[3])*k+127)/255)
}

// slice is the shadow of a square just big enough to have straight edges,
// as alpha values.
type slice struct {
	size, corner int
	alpha        []uint8
}

type sliceKey struct{ radius, blur, opacity float32 }

var shadowSlices struct {
	sync.Mutex
	m map[sliceKey]*slice
}

func shadowSlice(radius, blur, opacity float64) *slice {
	key := sliceKey{float32(radius), float32(blur), float32(opacity)}
	shadowSlices.Lock()
	defer shadowSlices.Unlock()
	if sl, ok := shadowSlices.m[key]; ok {
		return sl
	}
	// The corner covers the blur outside the shape and its rounding inside
	// it; the pixel after it lies on a straight edge.
	corner := int(math.Ceil(blur + radius + 1))
	size := 2*corner + 1
	tmp := NewCanvas(size, size)
	inner := float64(size) - 2*blur
	tmp.layeredShadow(deck.Rect{X: blur, Y: blur, W: inner, H: inner}, radius, blur, 0, opacity)
	sl := &slice{size: size, corner: corner, alpha: make([]uint8, size*size)}
	for i := range sl.alpha {
		sl.alpha[i] = tmp.Img.Pix[4*i+3]
	}
	if shadowSlices.m == nil || len(shadowSlices.m) > 256 {
		shadowSlices.m = map[sliceKey]*slice{}
	}
	shadowSlices.m[key] = sl
	return sl
}

// layeredShadow draws a shadow the direct way: a stack of progressively
// larger, fainter copies of the shape, which gives a convincing falloff.
func (c *Canvas) layeredShadow(r deck.Rect, radius, blur, dy, opacity float64) {
	// Each layer adds a step of about two pixels, so wide shadows need
	// more of them to stay smooth.
	layers := max(5, int(blur/2))
	col := WithAlpha(shadowColor, opacity/float64(layers))
	r.Y += dy
	for i := 1; i <= layers; i++ {
		grow := blur * float64(i) / float64(layers)
		c.FillRoundRect(r.Inflate(grow, grow), radius+grow, col)
	}
}

// path rasterises a shape whose outline is emitted by build and blends it
// onto the canvas. To keep the cost proportional to the shape rather than
// the canvas, the rasteriser only covers the shape's bounding box; build
// receives that box's origin and must emit coordinates relative to it.
func (c *Canvas) path(bounds deck.Rect, col color.NRGBA, build func(ox, oy float32)) {
	box := image.Rect(
		int(math.Floor(bounds.X)), int(math.Floor(bounds.Y)),
		int(math.Ceil(bounds.X+bounds.W)), int(math.Ceil(bounds.Y+bounds.H)),
	).Intersect(c.Img.Bounds())
	if box.Empty() {
		return
	}
	c.ras.Reset(box.Dx(), box.Dy())
	build(float32(box.Min.X), float32(box.Min.Y))
	c.ras.Draw(c.Img, box, image.NewUniform(col), image.Point{})
}

// roundRect emits a closed rounded-rectangle path. reverse flips the winding
// direction so the shape subtracts from a path wound the other way.
func roundRect(z *vector.Rasterizer, r deck.Rect, radius float64, ox, oy float32, reverse bool) {
	radius = math.Max(0, math.Min(radius, math.Min(r.W, r.H)/2))
	x0, x1 := r.X, r.X+r.W
	if reverse {
		x0, x1 = x1, x0
	}
	y0, y1 := r.Y, r.Y+r.H
	// rx points from x0 toward x1 so the same code walks either direction.
	rx := radius
	if x1 < x0 {
		rx = -radius
	}
	ry := radius
	k := 1 - kappa

	pt := func(x, y float64) (float32, float32) { return float32(x) - ox, float32(y) - oy }
	line := func(x, y float64) { z.LineTo(pt(x, y)) }
	cube := func(ax, ay, bx, by, cx, cy float64) {
		a1, a2 := pt(ax, ay)
		b1, b2 := pt(bx, by)
		c1, c2 := pt(cx, cy)
		z.CubeTo(a1, a2, b1, b2, c1, c2)
	}

	z.MoveTo(pt(x0+rx, y0))
	line(x1-rx, y0)
	if radius > 0 {
		cube(x1-rx*k, y0, x1, y0+ry*k, x1, y0+ry)
	}
	line(x1, y1-ry)
	if radius > 0 {
		cube(x1, y1-ry*k, x1-rx*k, y1, x1-rx, y1)
	}
	line(x0+rx, y1)
	if radius > 0 {
		cube(x0+rx*k, y1, x0, y1-ry*k, x0, y1-ry)
	}
	line(x0, y0+ry)
	if radius > 0 {
		cube(x0, y0+ry*k, x0+rx*k, y0, x0+rx, y0)
	}
	z.ClosePath()
}
