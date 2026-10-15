package settings

import (
	"testing"

	"github.com/MatthewPocock/tuck/internal/config"
)

func sample() State {
	return State{Config: config.Default(), NotesDir: `C:\Users\me\AppData\Roaming\Tuck\notes`, Version: "test"}
}

func TestEveryControlIsHitAtItsCentre(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 2} {
		l := NewLayout(scale, sample())
		for _, it := range l.Items {
			if len(it.Segments) > 0 {
				for i, seg := range it.Segments {
					c, got := l.Hit(int(seg.X+seg.W/2), int(seg.Y+seg.H/2))
					if c != it.Control || got != i {
						t.Errorf("scale %v: centre of option %d of %v hits %v option %d", scale, i, it.Control, c, got)
					}
				}
				continue
			}
			if c, _ := l.Hit(int(it.Rect.X+it.Rect.W/2), int(it.Rect.Y+it.Rect.H/2)); c != it.Control {
				t.Errorf("scale %v: centre of %v hits %v", scale, it.Control, c)
			}
		}
	}
}

func TestItemsFitAndDoNotOverlap(t *testing.T) {
	l := NewLayout(1.5, sample())
	for i, a := range l.Items {
		if a.Rect.X < 0 || a.Rect.Y < 0 || a.Rect.X+a.Rect.W > float64(l.Width) || a.Rect.Y+a.Rect.H > float64(l.Height) {
			t.Errorf("%v at %+v lies outside the %dx%d window", a.Control, a.Rect, l.Width, l.Height)
		}
		for _, b := range l.Items[i+1:] {
			if a.Rect.X < b.Rect.X+b.Rect.W && b.Rect.X < a.Rect.X+a.Rect.W &&
				a.Rect.Y < b.Rect.Y+b.Rect.H && b.Rect.Y < a.Rect.Y+a.Rect.H {
				t.Errorf("%v and %v overlap", a.Control, b.Control)
			}
		}
	}
	for _, c := range []Control{Close, Edge, Position, Theme, FontSmaller, FontLarger, NewNoteKey, ToggleDeckKey, Autostart, ChangeFolder, OpenFolder, EditFile} {
		if _, ok := l.Find(c); !ok {
			t.Errorf("control %v is missing", c)
		}
	}
}

func TestLayoutScales(t *testing.T) {
	a, b := NewLayout(1, sample()), NewLayout(2, sample())
	if b.Width != 2*a.Width || b.Height < 2*a.Height-1 || b.Height > 2*a.Height+1 {
		t.Errorf("2x layout is %dx%d, 1x is %dx%d", b.Width, b.Height, a.Width, a.Height)
	}
	if c, _ := a.Hit(1, 1); c != None {
		t.Errorf("window corner hits %v", c)
	}
}

func TestChooseAndSelectedAgree(t *testing.T) {
	for c, opts := range Options {
		for i := range opts {
			s := sample()
			Choose(&s.Config, c, i)
			if got := s.Selected(c); got != i {
				t.Errorf("after choosing option %d of %v, Selected = %d", i, c, got)
			}
		}
	}
	s := sample()
	s.Config.Position = 0.3
	if got := s.Selected(Position); got != -1 {
		t.Errorf("a custom position selects option %d", got)
	}
}

func TestStepFontClamps(t *testing.T) {
	if got := StepFont(MaxFontSize, FontLarger); got != MaxFontSize {
		t.Errorf("larger than max = %d", got)
	}
	if got := StepFont(MinFontSize, FontSmaller); got != MinFontSize {
		t.Errorf("smaller than min = %d", got)
	}
	if got := StepFont(11, FontLarger); got != 12 {
		t.Errorf("11 + 1 = %d", got)
	}
}
