package app

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/note"
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
	n := addNote(t, a, "Groceries for the weekend\n\n- [ ] oat milk\n- [x] coffee beans\n- [ ] something green\n\nRemember the **reusable bags**.")

	for _, theme := range []config.Theme{config.ThemeLight, config.ThemeDark} {
		a.cfg.Theme = theme
		for _, c := range []note.Color{note.Yellow, note.Lilac} {
			a.openNote(n.ID)
			a.editor.setPinned(true)
			a.setColor(n.ID, c)
			win32.UpdateWindow(a.editor.hwnd)
			win32.UpdateWindow(a.editor.edit)

			path := filepath.Join(snapshotDir, "editor-"+string(theme)+"-"+string(c)+".png")
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, capture(t, a.editor.hwnd)); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote %s", path)
		}
	}
}
