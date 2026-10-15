package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"

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
	c.path(r, col, func(ox, oy float32) {
		roundRect(&c.ras, r, radius, ox, oy, false)
		if !inner.Empty() {
			// Winding the inner path the other way punches it out.
			roundRect(&c.ras, inner, max(radius-width, 0), ox, oy, true)
		}
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
func (c *Canvas) Shadow(r deck.Rect, radius, blur, dy, opacity float64) {
	if opacity <= 0 || blur <= 0 {
		return
	}
	// A true Gaussian blur is overkill for shapes this small: stacking a few
	// progressively larger, faint copies gives a convincing falloff.
	const layers = 5
	col := WithAlpha(shadowColor, opacity/layers)
	r.Y += dy
	for i := 1; i <= layers; i++ {
		grow := blur * float64(i) / layers
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
