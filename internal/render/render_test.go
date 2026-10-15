package render

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/note"
)

// stubText records the labels it is asked to draw.
type stubText struct {
	drawn []string
}

func (s *stubText) DrawText(_ *image.RGBA, _ image.Rectangle, text string, _ TextStyle) {
	s.drawn = append(s.drawn, text)
}

func alphaAt(img *image.RGBA, x, y float64) uint8 {
	return img.RGBAAt(int(x), int(y)).A
}

func TestFillRoundRect(t *testing.T) {
	c := NewCanvas(40, 40)
	c.FillRoundRect(deck.Rect{X: 4, Y: 4, W: 32, H: 32}, 10, color.NRGBA{255, 0, 0, 255})

	if got := c.Img.RGBAAt(20, 20); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("centre pixel = %v, want opaque red", got)
	}
	if a := alphaAt(c.Img, 4, 4); a != 0 {
		t.Errorf("rounded corner pixel has alpha %d, want 0", a)
	}
	if a := alphaAt(c.Img, 1, 20); a != 0 {
		t.Errorf("pixel outside the shape has alpha %d, want 0", a)
	}
}

func TestStrokeRoundRectIsHollow(t *testing.T) {
	c := NewCanvas(40, 40)
	c.StrokeRoundRect(deck.Rect{X: 4, Y: 4, W: 32, H: 32}, 6, 2, color.NRGBA{0, 0, 0, 255})

	if a := alphaAt(c.Img, 20, 20); a != 0 {
		t.Errorf("interior alpha = %d, want 0", a)
	}
	if a := alphaAt(c.Img, 20, 5); a != 255 {
		t.Errorf("edge alpha = %d, want 255", a)
	}
}

func TestLineCoversEndpoints(t *testing.T) {
	c := NewCanvas(40, 40)
	c.Line(8, 8, 32, 32, 4, color.NRGBA{0, 0, 0, 255})
	for _, p := range [][2]float64{{8, 8}, {20, 20}, {31, 31}} {
		if a := alphaAt(c.Img, p[0], p[1]); a < 200 {
			t.Errorf("alpha at %v = %d, want the line to cover it", p, a)
		}
	}
	if a := alphaAt(c.Img, 32, 8); a != 0 {
		t.Errorf("alpha away from the line = %d, want 0", a)
	}
}

func TestShapesOutsideCanvasAreIgnored(t *testing.T) {
	c := NewCanvas(10, 10)
	c.FillRoundRect(deck.Rect{X: -50, Y: -50, W: 20, H: 20}, 4, color.NRGBA{255, 255, 255, 255})
	c.Line(100, 100, 200, 200, 3, color.NRGBA{255, 255, 255, 255})
	c.FillRoundRect(deck.Rect{X: -5, Y: -5, W: 100, H: 100}, 4, color.NRGBA{255, 255, 255, 255})
	if a := alphaAt(c.Img, 5, 5); a != 255 {
		t.Errorf("partially visible shape was not clipped and drawn: alpha %d", a)
	}
}

func testDeck(edge deck.Edge) *deck.Deck {
	d := deck.New(edge)
	d.SetItems([]deck.Item{
		{ID: "a", Title: "Alpha", Color: note.Yellow},
		{ID: "b", Title: "", Color: note.Sky},
	})
	return d
}

func TestDrawDeckAtRest(t *testing.T) {
	for _, edge := range []deck.Edge{deck.Right, deck.Left} {
		d := testDeck(edge)
		f := d.Frame()
		c := NewCanvas(1, 1)
		text := &stubText{}
		DrawDeck(c, f, text)

		if b := c.Img.Bounds(); b.Dx() != f.Width || b.Dy() != f.Height {
			t.Fatalf("canvas is %v, want %dx%d", b, f.Width, f.Height)
		}
		if len(text.drawn) != 0 {
			t.Errorf("labels drawn at rest: %q", text.drawn)
		}

		// The whole hover target must be solid to the pointer, all the way
		// to the screen edge.
		edgeX := float64(f.Width - 1)
		if edge == deck.Left {
			edgeX = 0
		}
		midY := f.HotZone.Y + f.HotZone.H/2
		if a := alphaAt(c.Img, edgeX, midY); a == 0 {
			t.Errorf("edge %v: pixel at the screen edge is fully transparent", edge)
		}
		if a := alphaAt(c.Img, f.Pill.X+f.Pill.W/2, f.Pill.Y+2); a < 128 {
			t.Errorf("edge %v: pill alpha = %d, want it clearly visible", edge, a)
		}
		// Far from the pill the window must let clicks through.
		if a := alphaAt(c.Img, float64(f.Width)/2, 1); a != 0 {
			t.Errorf("edge %v: empty area has alpha %d, want 0", edge, a)
		}
	}
}

func TestDrawDeckExpanded(t *testing.T) {
	d := testDeck(deck.Right)
	d.SetHeld(true)
	for range 100 {
		d.Advance(16 * time.Millisecond)
	}
	f := d.Frame()
	c := NewCanvas(1, 1)
	text := &stubText{}
	DrawDeck(c, f, text)

	want := []string{"Alpha", "New note"}
	if len(text.drawn) != len(want) || text.drawn[0] != want[0] || text.drawn[1] != want[1] {
		t.Errorf("labels = %q, want %q", text.drawn, want)
	}

	tab := f.Tabs[0].Rect
	fill := Palette(note.Yellow, false).Fill
	got := c.Img.RGBAAt(int(tab.X+tab.W/2), int(tab.Y+tab.H/2))
	if got != (color.RGBA{fill.R, fill.G, fill.B, 255}) {
		t.Errorf("tab centre = %v, want the note's fill colour %v", got, fill)
	}
	if a := alphaAt(c.Img, f.Add.X+f.Add.W/2, f.Add.Y+3); a < 128 {
		t.Errorf("add button alpha = %d, want it visible", a)
	}
}

func TestDrawDeckWithoutTextDrawer(t *testing.T) {
	d := testDeck(deck.Right)
	d.SetHeld(true)
	d.Advance(time.Second)
	DrawDeck(NewCanvas(1, 1), d.Frame(), nil) // must not panic
}

func TestHeaderLayout(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		l := HeaderLayout{Width: int(300 * scale), Scale: scale}
		var rects []deck.Rect
		for _, b := range headerButtons {
			r := l.ButtonRect(b)
			if r.Empty() {
				t.Fatalf("scale %v: button %d has no area", scale, b)
			}
			if r.X < 0 || r.X+r.W > float64(l.Width) || r.Y < 0 || r.Y+r.H > float64(l.Height()) {
				t.Errorf("scale %v: button %d %+v lies outside the header", scale, b, r)
			}
			if got := l.Hit(int(r.X+r.W/2), int(r.Y+r.H/2)); got != b {
				t.Errorf("scale %v: Hit at the centre of button %d = %d", scale, b, got)
			}
			for _, other := range rects {
				if r.X < other.X+other.W && other.X < r.X+r.W {
					t.Errorf("scale %v: button %d overlaps another button", scale, b)
				}
			}
			rects = append(rects, r)
		}
		if got := l.Hit(l.Width/2, l.Height()/2); got != ButtonNone {
			t.Errorf("scale %v: Hit in the empty middle = %d, want none", scale, got)
		}
	}
}

func TestDrawHeaderFillsWithPaper(t *testing.T) {
	sw := Palette(note.Mint, true)
	l := HeaderLayout{Width: 300, Scale: 1}
	c := NewCanvas(1, 1)
	DrawHeader(c, l, sw, HeaderState{Hover: ButtonClose, Pinned: true})

	if b := c.Img.Bounds(); b.Dx() != 300 || b.Dy() != l.Height() {
		t.Fatalf("canvas is %v", b)
	}
	got := c.Img.RGBAAt(150, l.Height()/2)
	if got != (color.RGBA{sw.Paper.R, sw.Paper.G, sw.Paper.B, 255}) {
		t.Errorf("background = %v, want paper %v", got, sw.Paper)
	}
}

func TestPaletteCoversEveryColour(t *testing.T) {
	for _, c := range note.Colors {
		for _, dark := range []bool{false, true} {
			sw := Palette(c, dark)
			if sw.Fill.A != 255 || sw.Paper.A != 255 || sw.Text.A != 255 {
				t.Errorf("%s (dark=%v): fill, paper and text must be opaque: %+v", c, dark, sw)
			}
			if contrast(sw.Paper, sw.Text) < 7 {
				t.Errorf("%s (dark=%v): text contrast %.1f is below 7:1", c, dark, contrast(sw.Paper, sw.Text))
			}
			if contrast(sw.Fill, sw.Ink) < 7 {
				t.Errorf("%s: tab label contrast %.1f is below 7:1", c, contrast(sw.Fill, sw.Ink))
			}
		}
	}
	if Palette("unknown", false) != Palette(note.DefaultColor, false) {
		t.Error("unknown colour does not fall back to the default")
	}
}

func TestAppIconSizes(t *testing.T) {
	for _, size := range []int{16, 32, 256} {
		img := AppIcon(size)
		if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
			t.Errorf("AppIcon(%d) is %v", size, b)
		}
		if a := alphaAt(img, float64(size)/2, float64(size)/2); a != 255 {
			t.Errorf("AppIcon(%d) centre alpha = %d, want opaque", size, a)
		}
	}
}

// contrast returns the WCAG contrast ratio between two opaque colours.
func contrast(a, b color.NRGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c color.NRGBA) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		// A cheap but close approximation of the sRGB transfer curve.
		x := (f + 0.055) / 1.055
		return x * x * (0.8*x + 0.2)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}
