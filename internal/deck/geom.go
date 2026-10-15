package deck

// Rect is an axis-aligned rectangle in window pixels. Coordinates are
// floating point so animated shapes can move by sub-pixel amounts.
type Rect struct {
	X, Y, W, H float64
}

// Contains reports whether the point (x, y) lies inside r.
func (r Rect) Contains(x, y float64) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Inflate returns r grown by dx on the left and right and dy on the top and
// bottom. Negative values shrink the rectangle.
func (r Rect) Inflate(dx, dy float64) Rect {
	return Rect{r.X - dx, r.Y - dy, r.W + 2*dx, r.H + 2*dy}
}

// Empty reports whether r has no area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// lerp linearly interpolates from a to b by t in [0, 1].
func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// lerpRect interpolates every component of a rectangle.
func lerpRect(a, b Rect, t float64) Rect {
	return Rect{lerp(a.X, b.X, t), lerp(a.Y, b.Y, t), lerp(a.W, b.W, t), lerp(a.H, b.H, t)}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// easeOutCubic starts fast and settles gently, which reads as "springing
// out" from the screen edge.
func easeOutCubic(t float64) float64 {
	t = 1 - clamp(t, 0, 1)
	return 1 - t*t*t
}

// approach moves v toward target by at most step.
func approach(v, target, step float64) float64 {
	if v < target {
		return min(v+step, target)
	}
	return max(v-step, target)
}
