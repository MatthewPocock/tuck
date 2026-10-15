package app

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/note"
	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/win32"
)

// snapshotDir, when set through the TUCK_SNAPSHOT_DIR environment variable,
// makes TestSnapshots write PNG captures of the real windows there. It is a
// debugging aid for checking the native parts of the UI by eye.
var snapshotDir = os.Getenv("TUCK_SNAPSHOT_DIR")

// capture copies a window's client area off the screen into an image.
func capture(t *testing.T, hwnd win32.HWND) *image.RGBA {
	t.Helper()
	client := win32.GetClientRect(hwnd)
	w, h := int(client.Width()), int(client.Height())
	dib, err := win32.NewDIB(w, h)
	if err != nil {
		t.Fatal(err)
	}
	defer dib.Close()

	dc := win32.GetDC(hwnd)
	win32.BitBlt(dib.DC, 0, 0, int32(w), int32(h), dc, 0, 0)
	win32.ReleaseDC(hwnd, dc)
	win32.Flush()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i+3 < len(dib.Pix); i += 4 {
		img.Pix[i+0] = dib.Pix[i+2]
		img.Pix[i+1] = dib.Pix[i+1]
		img.Pix[i+2] = dib.Pix[i+0]
		img.Pix[i+3] = 255
	}
	return img
}

func TestSnapshots(t *testing.T) {
	if snapshotDir == "" {
		t.Skip("set TUCK_SNAPSHOT_DIR to capture the windows")
	}
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t)
	saveImage := func(name string, img image.Image) {
		t.Helper()
		path := filepath.Join(snapshotDir, name+".png")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", path)
	}
	save := func(name string, hwnd win32.HWND) {
		t.Helper()
		saveImage(name, capture(t, hwnd))
	}
	addNote(t, a, "Standup\nAsk Sam about the milk budget")
	n := addNote(t, a, "Groceries for the weekend\n\n- [ ] oat milk\n- [x] coffee beans\n- [ ] something green\n\nRemember the **reusable bags**.")

	for _, theme := range []config.Theme{config.ThemeLight, config.ThemeDark} {
		a.cfg.Theme = theme
		for _, c := range []note.Color{note.Yellow, note.Lilac} {
			a.openNote(n.ID)
			a.editor.setPinned(true)
			a.setColor(n.ID, c)
			win32.UpdateWindow(a.editor.hwnd)
			win32.UpdateWindow(a.editor.edit)
			save("editor-"+string(theme)+"-"+string(c), a.editor.hwnd)
			if a.editor.shadow != nil {
				saveImage("framed-"+string(theme)+"-"+string(c), framed(t, a.editor, theme == config.ThemeDark))
			}
		}
		a.editor.setFolded(true)
		win32.UpdateWindow(a.editor.hwnd)
		save("folded-"+string(theme), a.editor.hwnd)
		a.editor.setFolded(false)

		a.showSearch()
		win32.SetWindowText(a.search.input, "milk")
		win32.UpdateWindow(a.search.hwnd)
		win32.UpdateWindow(a.search.input)
		save("search-"+string(theme), a.search.hwnd)
		a.search.hide()
	}
	win32.DestroyWindow(a.search.hwnd)
}

// framed composes a note window over its Windows 10 shadow on a plain
// desktop colour, the way they look together on screen.
func framed(t *testing.T, e *editorWindow, dark bool) *image.RGBA {
	t.Helper()
	shadow := e.shadow.canvas.Img
	b := shadow.Bounds()
	out := image.NewRGBA(b)
	desktop := color.NRGBA{0xDD, 0xE2, 0xEA, 0xFF}
	if dark {
		desktop = color.NRGBA{0x1B, 0x1F, 0x26, 0xFF}
	}
	draw.Draw(out, b, image.NewUniform(desktop), image.Point{}, draw.Src)
	draw.Draw(out, b, shadow, image.Point{}, draw.Over)

	body := capture(t, e.hwnd)
	pad := int(shadowPad(e.scale))
	w, h := body.Bounds().Dx(), body.Bounds().Dy()
	// The window region, approximately: its jagged corners sit inside the
	// shadow window's smooth rim.
	mask := render.NewCanvas(w, h)
	mask.FillRoundRect(deck.Rect{X: 0.5, Y: 0.5, W: float64(w) - 1, H: float64(h) - 1}, editorRadius*e.scale, color.NRGBA{A: 0xFF})
	draw.DrawMask(out, body.Bounds().Add(image.Pt(pad, pad)), body, image.Point{}, mask.Img, image.Point{}, draw.Over)
	return out
}

// TestScreenSnapshot captures a note exactly as it appears on screen, with
// its shadow and rounded corners, over a plain backdrop window so that
// nothing else on the desktop is captured.
func TestScreenSnapshot(t *testing.T) {
	if snapshotDir == "" {
		t.Skip("set TUCK_SNAPSHOT_DIR to capture the windows")
	}
	// Screen coordinates must be physical pixels for the capture to land on
	// the backdrop, as they are in the app itself.
	win32.SetProcessDpiAwarenessContext(win32.DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2)
	a := newTestApp(t)
	n := addNote(t, a, "Groceries for the weekend\n\n- [ ] oat milk\n- [ ] sourdough\n- [ ] something green\n- [x] coffee beans\n- [x] eggs\n\nRemember the **reusable bags**.")
	work := primaryDisplay().work
	scale := primaryDisplay().scale

	for _, theme := range []config.Theme{config.ThemeLight, config.ThemeDark} {
		a.cfg.Theme = theme
		desktop := color.NRGBA{0xDD, 0xE2, 0xEA, 0xFF}
		if theme == config.ThemeDark {
			desktop = color.NRGBA{0x1B, 0x1F, 0x26, 0xFF}
		}
		at := win32.RECT{Left: work.Left + 120, Top: work.Top + 120}
		at.Right, at.Bottom = at.Left+int32(340*scale), at.Top+int32(420*scale)
		pad := int32(48 * scale)
		area := win32.RECT{Left: at.Left - pad, Top: at.Top - pad, Right: at.Right + pad, Bottom: at.Bottom + pad}

		backdrop := solidWindow(t, a, area, desktop)
		a.openNote(n.ID)
		a.editor.setPinned(true)
		win32.SetWindowPos(a.editor.hwnd, win32.HWND_TOPMOST, at.Left, at.Top, at.Width(), at.Height(), win32.SWP_NOACTIVATE)
		// Put the caret on the last line, as when typing there.
		end := win32.SendMessage(a.editor.edit, win32.WM_GETTEXTLENGTH, 0, 0)
		win32.SendMessage(a.editor.edit, win32.EM_SETSEL, end, end)
		win32.UpdateWindow(a.editor.hwnd)
		win32.UpdateWindow(a.editor.edit)
		pump(300 * time.Millisecond)

		dib, err := win32.NewDIB(int(area.Width()), int(area.Height()))
		if err != nil {
			t.Fatal(err)
		}
		win32.CaptureScreen(dib.DC, area)
		win32.Flush()
		img := image.NewRGBA(image.Rect(0, 0, dib.Width, dib.Height))
		for i := 0; i+3 < len(dib.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = dib.Pix[i+2], dib.Pix[i+1], dib.Pix[i], 255
		}
		dib.Close()
		win32.DestroyWindow(backdrop)
		a.editor.hide()
		// Never keep a capture that reached past the backdrop: it would
		// hold whatever else is on the desktop.
		if !framedBy(img, desktop) {
			t.Fatal("the capture is not surrounded by the backdrop; discarded it")
		}

		path := filepath.Join(snapshotDir, "screen-"+string(theme)+".png")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
		t.Logf("wrote %s", path)
	}
}

// framedBy reports whether every pixel along the edges of img is col.
func framedBy(img *image.RGBA, col color.NRGBA) bool {
	b := img.Bounds()
	want := color.RGBA{col.R, col.G, col.B, 0xFF}
	for x := b.Min.X; x < b.Max.X; x++ {
		if img.RGBAAt(x, b.Min.Y) != want || img.RGBAAt(x, b.Max.Y-1) != want {
			return false
		}
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if img.RGBAAt(b.Min.X, y) != want || img.RGBAAt(b.Max.X-1, y) != want {
			return false
		}
	}
	return true
}

// pump handles window messages for a while, so windows paint and Desktop
// Window Manager composes them.
func pump(d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		var msg win32.MSG
		for win32.PeekMessage(&msg) {
			win32.TranslateMessage(&msg)
			win32.DispatchMessage(&msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// solidWindow shows a click-through window of one colour, below every
// other topmost window of the test.
func solidWindow(t *testing.T, a *App, r win32.RECT, col color.NRGBA) win32.HWND {
	t.Helper()
	s, err := newShadowWindow(a)
	if err != nil {
		t.Fatal(err)
	}
	c := render.NewCanvas(int(r.Width()), int(r.Height()))
	c.Fill(col)
	dib, err := win32.NewDIB(int(r.Width()), int(r.Height()))
	if err != nil {
		t.Fatal(err)
	}
	defer dib.Close()
	dib.CopyFrom(c.Img)
	win32.SetWindowPos(s.hwnd, win32.HWND_TOPMOST, r.Left, r.Top, r.Width(), r.Height(), win32.SWP_NOACTIVATE|win32.SWP_SHOWWINDOW)
	if err := win32.UpdateLayeredWindow(s.hwnd, win32.SIZE{CX: r.Width(), CY: r.Height()}, dib.DC); err != nil {
		t.Fatal(err)
	}
	return s.hwnd
}
