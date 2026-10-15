package deck

import "math"

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

// EmphasizedDecelerate is Material Design's easing for elements entering the
// screen, cubic-bezier(0.05, 0.7, 0.1, 1): it starts quickly and spends most
// of its time settling, so motion feels responsive without being abrupt.
// Played backwards, as the deck does when it folds away, it accelerates out.
func EmphasizedDecelerate(t float64) float64 {
	return cubicBezier(0.05, 0.7, 0.1, 1, clamp(t, 0, 1))
}

// EmphasizedAccelerate is Material Design's easing for elements leaving the
// screen, cubic-bezier(0.3, 0, 0.8, 0.15): it eases away and then picks up
// speed, so things leave decisively.
func EmphasizedAccelerate(t float64) float64 {
	return cubicBezier(0.3, 0, 0.8, 0.15, clamp(t, 0, 1))
}

// cubicBezier evaluates a CSS-style timing function with control points
// (x1, y1) and (x2, y2) at time x.
func cubicBezier(x1, y1, x2, y2, x float64) float64 {
	if x <= 0 || x >= 1 {
		return x
	}
	bez := func(a, b, t float64) float64 {
		u := 1 - t
		return 3*u*u*t*a + 3*u*t*t*b + t*t*t
	}
	// Find the curve parameter whose x is the given time by bisection,
	// which is robust for every valid timing function.
	lo, hi := 0.0, 1.0
	t := x
	for range 30 {
		if bez(x1, x2, t) < x {
			lo = t
		} else {
			hi = t
		}
		t = (lo + hi) / 2
	}
	return bez(y1, y2, t)
}

// spring is a damped spring, the motion model Material Design uses for
// things that move in response to the pointer. Unlike a fixed-length curve,
// a spring keeps its momentum when its target changes half-way, so
// interrupted motion stays smooth.
type spring struct {
	stiffness float64
	damping   float64 // ratio: 1 is critically damped, below 1 overshoots
}

// Material 3's spatial springs, for movement and changes of size.
var (
	fastSpatial    = spring{stiffness: 1400, damping: 0.9}
	defaultSpatial = spring{stiffness: 700, damping: 0.9}
)

// step advances the spring at *x with velocity *v toward target by dt
// seconds and reports whether anything moved.
func (sp spring) step(x, v *float64, target, dt float64) bool {
	if *x == target && *v == 0 {
		return false
	}
	c := 2 * sp.damping * math.Sqrt(sp.stiffness)
	// Small fixed steps keep the integration stable for stiff springs.
	const h = 1.0 / 480
	for dt > 0 {
		step := min(dt, h)
		a := -sp.stiffness*(*x-target) - c**v
		*v += a * step
		*x += *v * step
		dt -= step
	}
	if math.Abs(*x-target) < 1e-3 && math.Abs(*v) < 1e-2 {
		*x, *v = target, 0
	}
	return true
}

// approach moves v toward target by at most step.
func approach(v, target, step float64) float64 {
	if v < target {
		return min(v+step, target)
	}
	return max(v-step, target)
}
