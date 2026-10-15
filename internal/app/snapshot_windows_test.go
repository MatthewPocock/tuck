package app

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

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
