package app

import (
	"log"
	"path/filepath"

	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/hotkey"
	"github.com/MatthewPocock/tuck/internal/note"
	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/settings"
	"github.com/MatthewPocock/tuck/internal/store"
	"github.com/MatthewPocock/tuck/internal/win32"
)

// settingsWindow is the custom-drawn settings window. Every change applies
// immediately; there is nothing to confirm or cancel.
type settingsWindow struct {
	app    *App
	hwnd   win32.HWND
	scale  float64
	state  settings.State
	layout settings.Layout
	canvas *render.Canvas
	dib    *win32.DIB
	// tracking is true while a WM_MOUSELEAVE notification is pending.
	tracking bool
	// drawBorder is true before Windows 11, where DWM draws no border.
	drawBorder bool
}

func newSettingsWindow(a *App) (*settingsWindow, error) {
	w := &settingsWindow{app: a, canvas: render.NewCanvas(1, 1), scale: 1}
	hwnd, err := createWindow(w, a.inst, win32.WS_EX_APPWINDOW, settingsClass, appName+" settings",
		win32.WS_POPUP|win32.WS_THICKFRAME|win32.WS_SYSMENU|win32.WS_MINIMIZEBOX)
	if err != nil {
		return nil, err
	}
	w.hwnd = hwnd
	win32.DwmSetWindowAttribute(hwnd, win32.DWMWA_WINDOW_CORNER_PREFERENCE, win32.DWMWCP_ROUND)
	win32.DwmExtendFrameIntoClientArea(hwnd, win32.MARGINS{Top: 1})
	w.drawBorder = !win32.IsWindows11()
	for _, size := range []struct {
		which uintptr
		px    int32
	}{{win32.ICON_SMALL, win32.GetSystemMetrics(win32.SM_CXSMICON)}, {win32.ICON_BIG, win32.GetSystemMetrics(win32.SM_CXICON)}} {
		win32.SendMessage(hwnd, win32.WM_SETICON, size.which, uintptr(makeIcon(int(size.px))))
	}
	return w, nil
}

// show opens the window in the middle of the primary work area, or brings it
// to the front if it is already open.
func (w *settingsWindow) show() {
	if !win32.IsWindowVisible(w.hwnd) {
		disp := primaryDisplay()
		w.scale = disp.scale
		w.refresh()
		x := disp.work.Left + (disp.work.Width()-int32(w.layout.Width))/2
		y := disp.work.Top + (disp.work.Height()-int32(w.layout.Height))/2
		win32.SetWindowPos(w.hwnd, 0, x, y, int32(w.layout.Width), int32(w.layout.Height), win32.SWP_NOZORDER)
		win32.ShowWindow(w.hwnd, win32.SW_SHOW)
	}
	w.refresh()
	win32.SetForegroundWindow(w.hwnd)
}

func (w *settingsWindow) hide() {
	w.endCapture("")
	win32.ShowWindow(w.hwnd, win32.SW_HIDE)
}

// refresh reads the current settings and repaints.
func (w *settingsWindow) refresh() {
	a := w.app
	w.state.Config = a.cfg
	w.state.Autostart = autostartEnabled()
	w.state.NotesDir = a.store.Dir()
	w.state.Version = a.version
	w.layout = settings.NewLayout(w.scale, w.state)
	r := win32.GetWindowRect(w.hwnd)
	if win32.IsWindowVisible(w.hwnd) && (r.Width() != int32(w.layout.Width) || r.Height() != int32(w.layout.Height)) {
		win32.SetWindowPos(w.hwnd, 0, 0, 0, int32(w.layout.Width), int32(w.layout.Height), win32.SWP_NOZORDER|win32.SWP_NOMOVE)
	}
	win32.InvalidateRect(w.hwnd, nil)
}

// activate performs the action of a control; option is the chosen segment of
// a segmented control.
func (w *settingsWindow) activate(c settings.Control, option int) {
	a := w.app
	switch c {
	case settings.Close:
		w.hide()
		return
	case settings.Edge, settings.Position, settings.Theme:
		if option < 0 {
			return
		}
		cfg := a.cfg
		settings.Choose(&cfg, c, option)
		switch {
		case cfg.Edge != a.cfg.Edge:
			a.setEdge(cfg.Edge)
		case cfg.Position != a.cfg.Position:
			a.setPosition(cfg.Position)
		case cfg.Theme != a.cfg.Theme:
			a.setTheme(cfg.Theme)
		}
	case settings.FontSmaller, settings.FontLarger:
		a.setFontSize(settings.StepFont(a.cfg.FontSize, c))
	case settings.NewNoteKey, settings.ToggleDeckKey:
		w.startCapture(c)
		return
	case settings.Autostart:
		if err := setAutostart(!autostartEnabled()); err != nil {
			log.Printf("autostart: %v", err)
		}
	case settings.ChangeFolder:
		if dir, ok := win32.PickFolder(w.hwnd, "Choose where Tuck keeps your notes", a.store.Dir()); ok {
			a.setNotesDir(dir)
		}
	case settings.OpenFolder:
		win32.ShellOpen(a.store.Dir())
	case settings.EditFile:
		win32.ShellOpen(a.paths.config)
	}
	w.refresh()
}

// startCapture makes a hotkey field record the next shortcut pressed. The
// global hotkeys are released meanwhile, so that pressing the current
// shortcut again reaches the field instead of triggering it.
func (w *settingsWindow) startCapture(c settings.Control) {
	w.state.Capturing = c
	w.state.Message, w.state.Problem = "", false
	w.app.unregisterHotkeys()
	w.refresh()
}

// endCapture stops recording, shows message (if any) as a problem and puts
// the global hotkeys back.
func (w *settingsWindow) endCapture(problem string) {
	if w.state.Capturing == settings.None {
		return
	}
	w.state.Capturing = settings.None
	w.state.Message, w.state.Problem = problem, problem != ""
	failures := w.app.registerHotkeys()
	if problem == "" && len(failures) > 0 {
		w.state.Message, w.state.Problem = failures[0], true
	}
	w.refresh()
}

// captureKey handles a key press while a hotkey field is recording. mods are
// the hotkey modifier flags held down.
func (w *settingsWindow) captureKey(vk, mods uint32) {
	field := w.state.Capturing
	switch {
	case vk == win32.VK_ESCAPE && mods == 0:
		w.endCapture("")
		return
	case (vk == win32.VK_BACK || vk == win32.VK_DELETE) && mods == 0:
		w.setHotkey(field, "")
		w.endCapture("")
		return
	case hotkey.IsModifierKey(vk):
		return // keep waiting for the actual key
	}
	h := hotkey.Hotkey{Mods: mods, Key: vk}
	name := hotkey.Format(h)
	switch {
	case mods == 0:
		w.endCapture("A shortcut needs Ctrl, Alt, Shift or Win as well as a key.")
		return
	case name == "":
		w.endCapture("That key cannot be used in a shortcut.")
		return
	}
	other := w.app.cfg.Hotkeys.ToggleDeck
	if field == settings.ToggleDeckKey {
		other = w.app.cfg.Hotkeys.NewNote
	}
	if o, err := hotkey.Parse(other); err == nil && o == h {
		w.endCapture(name + " is already used by the other shortcut.")
		return
	}

	previous := w.hotkey(field)
	w.setHotkey(field, name)
	w.state.Capturing = settings.None
	if failures := w.app.registerHotkeys(); len(failures) > 0 {
		// Most likely another app owns the combination.
		w.setHotkey(field, previous)
		w.app.registerHotkeys()
		w.state.Message, w.state.Problem = name+" is already taken by another app.", true
	} else {
		w.state.Message, w.state.Problem = "", false
	}
	w.refresh()
}

func (w *settingsWindow) hotkey(c settings.Control) string {
	if c == settings.ToggleDeckKey {
		return w.app.cfg.Hotkeys.ToggleDeck
	}
	return w.app.cfg.Hotkeys.NewNote
}

func (w *settingsWindow) setHotkey(c settings.Control, value string) {
	if c == settings.ToggleDeckKey {
		w.app.cfg.Hotkeys.ToggleDeck = value
	} else {
		w.app.cfg.Hotkeys.NewNote = value
	}
	w.app.saveConfig()
}

// heldModifiers returns the hotkey modifiers currently held down.
func heldModifiers() uint32 {
	var mods uint32
	for _, m := range []struct {
		vk   int32
		flag uint32
	}{{win32.VK_CONTROL, hotkey.ModCtrl}, {win32.VK_MENU, hotkey.ModAlt}, {win32.VK_SHIFT, hotkey.ModShift},
		{win32.VK_LWIN, hotkey.ModWin}, {win32.VK_RWIN, hotkey.ModWin}} {
		if win32.KeyDown(m.vk) {
			mods |= m.flag
		}
	}
	return mods
}

func (w *settingsWindow) paint() {
	var ps win32.PAINTSTRUCT
	dc := win32.BeginPaint(w.hwnd, &ps)
	defer win32.EndPaint(w.hwnd, &ps)

	dark := w.app.dark()
	render.DrawSettings(w.canvas, w.layout, w.state, dark, w.app.text)
	if w.drawBorder {
		border := render.Palette(note.Stone, dark).Border
		width, height := float64(w.layout.Width), float64(w.layout.Height)
		for _, r := range []deck.Rect{{W: width, H: 1}, {Y: height - 1, W: width, H: 1}, {W: 1, H: height}, {X: width - 1, W: 1, H: height}} {
			w.canvas.FillRect(r, border)
		}
	}
	width, height := w.layout.Width, w.layout.Height
	if w.dib == nil || w.dib.Width != width || w.dib.Height != height {
		w.dib.Close()
		var err error
		if w.dib, err = win32.NewDIB(width, height); err != nil {
			w.dib = nil
			log.Printf("settings: %v", err)
			return
		}
	}
	w.dib.CopyFrom(w.canvas.Img)
	win32.BitBlt(dc, 0, 0, int32(width), int32(height), w.dib.DC, 0, 0)
}

func (w *settingsWindow) setHover(c settings.Control, option int) {
	if c != w.state.Hover || option != w.state.HoverSegment {
		w.state.Hover, w.state.HoverSegment = c, option
		win32.InvalidateRect(w.hwnd, nil)
	}
}

func (w *settingsWindow) wndProc(hwnd win32.HWND, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case win32.WM_NCCALCSIZE:
		if wp != 0 {
			return 0 // the whole window is client area; see the editor
		}

	case win32.WM_NCHITTEST:
		pt := win32.PointFromLParam(lp)
		r := win32.GetWindowRect(hwnd)
		x, y := float64(pt.X-r.Left), float64(pt.Y-r.Top)
		if c, _ := w.layout.Hit(int(x), int(y)); c == settings.None && w.layout.Title.Contains(x, y) {
			return win32.HTCAPTION
		}
		// Not resizable: no edge ever reports a border.
		return win32.HTCLIENT

	case win32.WM_NCACTIVATE:
		return win32.DefWindowProc(hwnd, msg, wp, ^uintptr(0))

	case win32.WM_ERASEBKGND:
		return 1

	case win32.WM_PAINT:
		w.paint()
		return 0

	case win32.WM_MOUSEMOVE:
		pt := win32.PointFromLParam(lp)
		w.setHover(w.layout.Hit(int(pt.X), int(pt.Y)))
		if !w.tracking {
			w.tracking = true
			win32.TrackMouseEvent(&win32.TRACKMOUSEEVENT{Flags: win32.TME_LEAVE, Track: hwnd})
		}
		return 0

	case win32.WM_MOUSELEAVE:
		w.tracking = false
		w.setHover(settings.None, -1)
		return 0

	case win32.WM_SETCURSOR:
		if win32.LoWord(lp) == win32.HTCLIENT {
			cursor := uintptr(win32.IDC_ARROW)
			if w.state.Hover != settings.None {
				cursor = win32.IDC_HAND
			}
			win32.SetCursor(win32.LoadCursor(cursor))
			return 1
		}

	case win32.WM_LBUTTONDOWN:
		pt := win32.PointFromLParam(lp)
		w.state.Pressed, _ = w.layout.Hit(int(pt.X), int(pt.Y))
		win32.InvalidateRect(hwnd, nil)
		return 0

	case win32.WM_LBUTTONUP:
		pt := win32.PointFromLParam(lp)
		pressed := w.state.Pressed
		w.state.Pressed = settings.None
		win32.InvalidateRect(hwnd, nil)
		c, option := w.layout.Hit(int(pt.X), int(pt.Y))
		if c == settings.None || c != pressed {
			// A click elsewhere abandons a recording.
			w.endCapture("")
			return 0
		}
		if w.state.Capturing != settings.None && c != w.state.Capturing {
			w.endCapture("")
		}
		w.activate(c, option)
		return 0

	case win32.WM_KEYDOWN, win32.WM_SYSKEYDOWN:
		if w.state.Capturing != settings.None {
			w.captureKey(uint32(wp), heldModifiers())
			return 0
		}
		if wp == win32.VK_ESCAPE {
			w.hide()
			return 0
		}

	case win32.WM_SYSCHAR, win32.WM_CHAR:
		if w.state.Capturing != settings.None {
			return 0 // no beep or system menu while recording Alt shortcuts
		}

	case win32.WM_ACTIVATE:
		if win32.LoWord(wp) == win32.WA_INACTIVE {
			w.endCapture("")
		}

	case win32.WM_DPICHANGED:
		w.scale = float64(win32.HiWord(wp)) / win32.USER_DEFAULT_DPI
		suggested := win32.PointerFromLParam[win32.RECT](lp)
		w.layout = settings.NewLayout(w.scale, w.state)
		win32.SetWindowPos(hwnd, 0, suggested.Left, suggested.Top, int32(w.layout.Width), int32(w.layout.Height),
			win32.SWP_NOZORDER|win32.SWP_NOACTIVATE)
		return 0

	case win32.WM_CLOSE:
		w.hide()
		return 0

	case win32.WM_DESTROY:
		w.dib.Close()
		w.dib = nil
		return 0
	}
	return win32.DefWindowProc(hwnd, msg, wp, lp)
}

// setFontSize changes the editor font size.
func (a *App) setFontSize(points int) {
	if points == a.cfg.FontSize {
		return
	}
	a.cfg.FontSize = points
	a.saveConfig()
	a.editor.applyFont()
}

// setNotesDir switches to another notes folder. The notes in the old folder
// stay where they are.
func (a *App) setNotesDir(dir string) {
	if filepath.Clean(dir) == filepath.Clean(a.store.Dir()) {
		return
	}
	s, err := store.Open(dir)
	if err != nil {
		win32.MessageBox(a.settingsHWND(), "Tuck cannot use that folder.\n\n"+err.Error(), appName, win32.MB_OK|win32.MB_ICONERROR)
		return
	}
	a.editor.hide()
	notes, err := s.List()
	if err != nil {
		log.Printf("notes: %v", err)
	}
	a.store, a.notes, a.lastTrashed = s, notes, ""
	a.watch.close()
	a.watch = watchFolder(dir)
	if !a.watch.active() {
		win32.SetTimer(a.controller, changesTimer, changesInterval)
	}
	a.cfg.NotesDir = dir
	if filepath.Clean(dir) == filepath.Clean(a.paths.notes) {
		a.cfg.NotesDir = "" // the default needs no setting
	}
	a.saveConfig()
	a.syncDeck()
	a.deck.peek()
}

func (a *App) settingsHWND() win32.HWND {
	if a.settings == nil {
		return 0
	}
	return a.settings.hwnd
}

// showSettings opens the settings window, creating it on first use.
func (a *App) showSettings() {
	if a.settings == nil {
		w, err := newSettingsWindow(a)
		if err != nil {
			log.Printf("settings: %v", err)
			win32.ShellOpen(a.paths.config)
			return
		}
		a.settings = w
	}
	a.settings.show()
}

// refreshSettings updates the settings window, if open, after a setting was
// changed elsewhere, e.g. from the tray menu.
func (a *App) refreshSettings() {
	if a.settings != nil && win32.IsWindowVisible(a.settings.hwnd) {
		a.settings.refresh()
	}
}
