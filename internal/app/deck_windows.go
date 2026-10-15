package app

import (
	"log"
	"time"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/win32"
)

const (
	deckTimer = 1
	// fastTick drives animation at roughly the display refresh rate.
	fastTick = 15 // milliseconds
	// slowTick is used while the deck is open but nothing is moving, to
	// keep watching the pointer without wasting CPU.
	slowTick = 120 // milliseconds
	// quietBeforeSlow is how long the deck must be still before the timer
	// slows down.
	quietBeforeSlow = 400 * time.Millisecond
	// wheelStep is how far one notch of the mouse wheel scrolls, in
	// device-independent pixels.
	wheelStep = 48
)

// deckWindow is the always-on-top layered window that shows the deck at the
// edge of the screen. It never takes keyboard focus.
type deckWindow struct {
	app   *App
	hwnd  win32.HWND
	model *deck.Deck

	canvas *render.Canvas
	dib    *win32.DIB

	ticking  bool
	fast     bool
	lastTick time.Time
	quiet    time.Duration
	// pressed is the element the left button went down on.
	pressed deck.Hit
	// hidden is set when the user hides the deck; suppressed while a
	// full-screen app is in front. Either keeps the window off screen.
	hidden     bool
	suppressed bool
}

func newDeckWindow(a *App) (*deckWindow, error) {
	d := &deckWindow{
		app:    a,
		model:  deck.New(deckEdge(a.cfg.Edge)),
		canvas: render.NewCanvas(1, 1),
	}
	hwnd, err := createWindow(d, a.inst,
		win32.WS_EX_LAYERED|win32.WS_EX_TOPMOST|win32.WS_EX_TOOLWINDOW|win32.WS_EX_NOACTIVATE,
		deckClass, appName, win32.WS_POPUP)
	if err != nil {
		return nil, err
	}
	d.hwnd = hwnd
	return d, nil
}

func deckEdge(e config.Edge) deck.Edge {
	if e == config.EdgeLeft {
		return deck.Left
	}
	return deck.Right
}

// place sizes the window for the current notes and docks it to the
// configured edge of the primary monitor's work area.
func (d *deckWindow) place() {
	disp := primaryDisplay()
	work := disp.work
	d.model.SetEdge(deckEdge(d.app.cfg.Edge))
	d.model.SetScale(disp.scale)
	d.model.SetMaxHeight(int(work.Height()))

	w, h := d.model.Size()
	x := work.Right - int32(w)
	if d.app.cfg.Edge == config.EdgeLeft {
		x = work.Left
	}
	y := work.Top + int32(float64(int(work.Height())-h)*d.app.cfg.Position)

	want := win32.RECT{Left: x, Top: y, Right: x + int32(w), Bottom: y + int32(h)}
	if win32.GetWindowRect(d.hwnd) != want {
		win32.SetWindowPos(d.hwnd, win32.HWND_TOPMOST, x, y, int32(w), int32(h), win32.SWP_NOACTIVATE)
	}
}

// refresh re-places and repaints the deck after its contents or settings
// changed.
func (d *deckWindow) refresh() {
	d.place()
	d.render()
	d.wake()
}

// render paints the current frame and pushes it to the screen.
func (d *deckWindow) render() {
	frame := d.model.Frame()
	render.DrawDeck(d.canvas, frame, d.app.text)

	if d.dib == nil || d.dib.Width != frame.Width || d.dib.Height != frame.Height {
		d.dib.Close()
		dib, err := win32.NewDIB(frame.Width, frame.Height)
		if err != nil {
			d.dib = nil
			log.Printf("deck: %v", err)
			return
		}
		d.dib = dib
	}
	d.dib.CopyFrom(d.canvas.Img)
	size := win32.SIZE{CX: int32(frame.Width), CY: int32(frame.Height)}
	if err := win32.UpdateLayeredWindow(d.hwnd, size, d.dib.DC); err != nil {
		log.Printf("deck: %v", err)
	}
}

// visible reports whether the deck window should be on screen.
func (d *deckWindow) visible() bool {
	return !d.hidden && !d.suppressed
}

// show brings back a deck the user had hidden, without activating it.
func (d *deckWindow) show() {
	d.hidden = false
	d.applyVisibility()
}

// hide removes the deck from the screen until show is called.
func (d *deckWindow) hide() {
	d.hidden = true
	d.applyVisibility()
}

// setSuppressed hides the deck while a full-screen app is in front, without
// changing whether the user wants it shown.
func (d *deckWindow) setSuppressed(suppressed bool) {
	if d.suppressed == suppressed {
		return
	}
	d.suppressed = suppressed
	d.applyVisibility()
}

func (d *deckWindow) applyVisibility() {
	if !d.visible() {
		win32.ShowWindow(d.hwnd, win32.SW_HIDE)
		d.stopTicking()
		return
	}
	d.refresh()
	win32.ShowWindow(d.hwnd, win32.SW_SHOWNOACTIVATE)
}

// peek expands the deck for a moment to show the user where it lives.
func (d *deckWindow) peek() {
	d.suppressed = false
	d.show()
	d.model.Peek(1500 * time.Millisecond)
	d.wake()
}

// wake makes sure the animation timer is running at full speed. It stops
// itself again once the deck is idle.
func (d *deckWindow) wake() {
	if !d.visible() {
		return
	}
	d.quiet = 0
	if d.ticking && d.fast {
		return
	}
	if !d.ticking {
		d.lastTick = time.Now()
	}
	d.ticking, d.fast = true, true
	win32.SetTimer(d.hwnd, deckTimer, fastTick)
}

func (d *deckWindow) stopTicking() {
	if d.ticking {
		win32.KillTimer(d.hwnd, deckTimer)
		d.ticking = false
	}
}

// tick advances the animation by the time elapsed since the previous tick.
func (d *deckWindow) tick() {
	now := time.Now()
	dt := now.Sub(d.lastTick)
	d.lastTick = now

	// Poll the pointer rather than relying on mouse messages: the window
	// receives none while the pointer is over its transparent parts or has
	// left it, which is exactly when the deck needs to notice.
	if pt, ok := win32.GetCursorPos(); ok {
		origin := win32.GetWindowRect(d.hwnd)
		d.model.Pointer(int(pt.X-origin.Left), int(pt.Y-origin.Top), true)
	} else {
		d.model.Pointer(0, 0, false)
	}

	if d.model.Advance(dt) {
		d.render()
		d.quiet = 0
		if !d.fast {
			d.fast = true
			win32.SetTimer(d.hwnd, deckTimer, fastTick)
		}
		return
	}

	if d.model.Idle() {
		d.stopTicking()
		return
	}
	d.quiet += dt
	if d.fast && d.quiet >= quietBeforeSlow {
		d.fast = false
		win32.SetTimer(d.hwnd, deckTimer, slowTick)
	}
}

// anchor returns the screen rectangle of the expanded tab for a note, or of
// the add button if the note has no tab.
func (d *deckWindow) anchor(id string) win32.RECT {
	r, ok := d.model.TabAnchor(id)
	if !ok {
		r = d.model.AddAnchor()
	}
	origin := win32.GetWindowRect(d.hwnd)
	return win32.RECT{
		Left:   origin.Left + int32(r.X),
		Top:    origin.Top + int32(r.Y),
		Right:  origin.Left + int32(r.X+r.W),
		Bottom: origin.Top + int32(r.Y+r.H),
	}
}

func (d *deckWindow) wndProc(hwnd win32.HWND, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case win32.WM_TIMER:
		if wp == deckTimer {
			d.tick()
		}
		return 0

	case win32.WM_MOUSEMOVE:
		d.wake()
		return 0

	case win32.WM_MOUSEACTIVATE:
		// Clicking the deck must not take focus away from the app the
		// user is working in.
		return win32.MA_NOACTIVATE

	case win32.WM_LBUTTONDOWN:
		pt := win32.PointFromLParam(lp)
		d.pressed = d.model.HitTest(int(pt.X), int(pt.Y))
		return 0

	case win32.WM_LBUTTONUP:
		pt := win32.PointFromLParam(lp)
		hit := d.model.HitTest(int(pt.X), int(pt.Y))
		pressed := d.pressed
		d.pressed = deck.Hit{}
		if hit != pressed {
			return 0
		}
		switch hit.Kind {
		case deck.HitTab:
			d.app.toggleNote(d.model.Items()[hit.Index].ID)
		case deck.HitAdd:
			d.app.newNote()
		}
		return 0

	case win32.WM_RBUTTONUP:
		pt := win32.PointFromLParam(lp)
		hit := d.model.HitTest(int(pt.X), int(pt.Y))
		at, _ := win32.GetCursorPos()
		if hit.Kind == deck.HitTab {
			d.app.showNoteMenu(d.model.Items()[hit.Index].ID, at)
		} else {
			d.app.showMainMenu(at)
		}
		return 0

	case win32.WM_MOUSEWHEEL:
		notches := float64(int16(win32.HiWord(wp))) / win32.WHEEL_DELTA
		d.model.Scroll(-notches * wheelStep * d.model.Scale())
		d.render()
		d.wake()
		return 0

	case win32.WM_DISPLAYCHANGE, win32.WM_DPICHANGED:
		d.refresh()
		return 0

	case win32.WM_SETTINGCHANGE:
		if wp == win32.SPI_SETWORKAREA {
			d.refresh()
		}
		return 0

	case win32.WM_CLOSE:
		// The deck is the app's only visible window, so this is where a
		// polite request to close the app arrives (taskkill, installers,
		// Restart Manager). Letting the default handler destroy just the
		// deck would leave an invisible process that blocks relaunching.
		d.app.quit()
		return 0

	case win32.WM_DESTROY:
		d.stopTicking()
		d.dib.Close()
		d.dib = nil
		return 0
	}
	return win32.DefWindowProc(hwnd, msg, wp, lp)
}
