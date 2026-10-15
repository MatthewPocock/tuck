package deck

import (
	"math"
	"testing"
)

func TestEasing(t *testing.T) {
	for _, ease := range []func(float64) float64{EmphasizedDecelerate, EmphasizedAccelerate} {
		if ease(0) != 0 || ease(1) != 1 || ease(-1) != 0 || ease(2) != 1 {
			t.Error("easing does not run from 0 to 1")
		}
		prev := 0.0
		for i := 1; i <= 100; i++ {
			v := ease(float64(i) / 100)
			if v < prev-1e-9 {
				t.Fatalf("easing goes backwards at %d%%", i)
			}
			prev = v
		}
	}
	// Entering is quick at first, leaving is slow at first.
	if EmphasizedDecelerate(0.2) < 0.6 || EmphasizedAccelerate(0.2) > 0.1 {
		t.Errorf("at 20%%: decelerate %v, accelerate %v", EmphasizedDecelerate(0.2), EmphasizedAccelerate(0.2))
	}
}

func TestSpringSettles(t *testing.T) {
	x, v := 0.0, 0.0
	peak := 0.0
	steps := 0
	for defaultSpatial.step(&x, &v, 1, 1.0/60) {
		peak = math.Max(peak, x)
		if steps++; steps > 120 {
			t.Fatal("the spring did not settle within two seconds")
		}
	}
	if x != 1 || v != 0 {
		t.Errorf("settled at %v moving %v", x, v)
	}
	if peak > 1.01 {
		t.Errorf("overshot to %v", peak)
	}
	if steps > 30 {
		t.Errorf("took %d frames to settle; Material's default spring is quicker", steps)
	}
}
