package app

import (
	"image/color"
	"log"
	"math"

	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/win32"
)

// The note window's shape and shadow, in device-independent pixels.
const (
	editorRadius = 12.0
	// A key shadow cast from above and a tighter ambient one, as Material
	// Design draws elevation.
	shadowBlur     = 22.0
	shadowDrop     = 8.0
	shadowOpacity  = 0.22
	ambientBlur    = 3.0
	ambientOpacity = 0.12
)

// shadowWindow sits directly behind a note window on Windows 10, which
// neither rounds nor shadows borderless windows. The note is clipped to a
// rounded rectangle, and this click-through layered window draws its soft
// shadow, a hairline edge and the anti-aliased rim of the corners that the
// clipping leaves jagged. Windows 11 does all of this itself.
type shadowWindow struct {
	hwnd   win32.HWND
	canvas *render.Canvas
	dib    *win32.DIB
	look   shadowLook
	// alpha fades the whole shadow, for the note's entrance.
	alpha uint8
}

// shadowLook is everything the picture depends on, so that it is only
// redrawn when one of them changes; moving the note just moves the window.
type shadowLook struct {
	width, height int32
	scale         float64
	header        int32
	fill, paper   color.NRGBA
	border        color.NRGBA
}

func newShadowWindow(a *App) (*shadowWindow, error) {
	s := &shadowWindow{canvas: render.NewCanvas(1, 1), alpha: 255}
	hwnd, err := createWindow(s, a.inst,
		win32.WS_EX_LAYERED|win32.WS_EX_TRANSPARENT|win32.WS_EX_TOOLWINDOW|win32.WS_EX_NOACTIVATE|win32.WS_EX_TOPMOST,
		shadowClass, "", win32.WS_POPUP)
	if err != nil {
		return nil, err
	}
	s.hwnd = hwnd
	return s, nil
}

// shadowPad is how far the shadow reaches beyond the note, in pixels.
func shadowPad(scale float64) int32 {
	return int32(math.Ceil((shadowBlur + shadowDrop) * scale))
}

// follow places the shadow behind the note window, or hides it with the
// note.
func (s *shadowWindow) follow(e *editorWindow, visible bool) {
	if s == nil {
		return
	}
	if !visible {
		win32.ShowWindow(s.hwnd, win32.SW_HIDE)
		return
	}
	r := win32.GetWindowRect(e.hwnd)
	look := shadowLook{
		width: r.Width(), height: r.Height(), scale: e.scale,
		header: int32(e.headerLayout().Height()),
		fill:   e.sw.Fill, paper: e.sw.Paper, border: e.sw.Border,
	}
	pad := shadowPad(e.scale)
	w, h := r.Width()+2*pad, r.Height()+2*pad
	// Directly below the note in the z-order, wherever the note is.
	win32.SetWindowPos(s.hwnd, e.hwnd, r.Left-pad, r.Top-pad, w, h, win32.SWP_NOACTIVATE|win32.SWP_SHOWWINDOW)
	if look == s.look && s.dib != nil {
		return
	}
	s.look = look
	drawShadow(s.canvas, look, pad)
	if s.dib == nil || s.dib.Width != int(w) || s.dib.Height != int(h) {
		s.dib.Close()
		dib, err := win32.NewDIB(int(w), int(h))
		if err != nil {
			s.dib = nil
			log.Printf("shadow: %v", err)
			return
		}
		s.dib = dib
	}
	s.dib.CopyFrom(s.canvas.Img)
	if err := win32.UpdateLayeredWindowAlpha(s.hwnd, win32.SIZE{CX: w, CY: h}, s.dib.DC, s.alpha); err != nil {
		log.Printf("shadow: %v", err)
	}
}

// setAlpha fades the shadow along with its note.
func (s *shadowWindow) setAlpha(alpha uint8) {
	if s == nil || s.alpha == alpha {
		return
	}
	s.alpha = alpha
	if s.dib != nil {
		size := win32.SIZE{CX: int32(s.dib.Width), CY: int32(s.dib.Height)}
		if err := win32.UpdateLayeredWindowAlpha(s.hwnd, size, s.dib.DC, alpha); err != nil {
			log.Printf("shadow: %v", err)
		}
	}
}

// drawShadow paints the shadow and rim for a note of the given look, which
// sits pad pixels in from the canvas's edges.
func drawShadow(c *render.Canvas, look shadowLook, pad int32) {
	s := look.scale
	c.Resize(int(look.width+2*pad), int(look.height+2*pad))
	note := deck.Rect{X: float64(pad), Y: float64(pad), W: float64(look.width), H: float64(look.height)}
	radius := editorRadius * s

	c.Shadow(note, radius, shadowBlur*s, shadowDrop*s/2, shadowOpacity)
	c.Shadow(note, radius, ambientBlur*s, s, ambientOpacity)

	// The note's own colours under its clipped corners, anti-aliased.
	c.FillRoundRect(note, radius, look.paper)
	head := note
	head.H = math.Min(float64(look.header)+radius, note.H)
	c.FillRoundRect(head, radius, look.fill)
	if note.H > float64(look.header) {
		c.FillRect(deck.Rect{X: note.X, Y: note.Y + float64(look.header), W: note.W, H: math.Min(radius, note.H-float64(look.header))}, look.paper)
	}
	// A hairline just outside the note keeps its edge crisp on any
	// background, dark ones included.
	t := math.Max(1, math.Round(s))
	c.StrokeRoundRect(note.Inflate(t/2, t/2), radius+t/2, t, look.border)
}

// clip rounds the corners of a note window to match its shadow.
func (s *shadowWindow) clip(e *editorWindow) {
	if s == nil {
		return
	}
	r := win32.GetWindowRect(e.hwnd)
	d := int32(math.Round(2 * editorRadius * e.scale))
	win32.SetWindowRgn(e.hwnd, win32.CreateRoundRectRgn(0, 0, r.Width()+1, r.Height()+1, d), true)
}

func (s *shadowWindow) wndProc(hwnd win32.HWND, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case win32.WM_NCHITTEST:
		return win32.HTTRANSPARENT
	case win32.WM_DESTROY:
		s.dib.Close()
		s.dib = nil
		return 0
	}
	return win32.DefWindowProc(hwnd, msg, wp, lp)
}
