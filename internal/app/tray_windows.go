package app

import (
	"unicode/utf16"

	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/win32"
)

const trayID = 1

// tray is the app's notification area icon.
type tray struct {
	owner win32.HWND
	icon  win32.HICON
}

// add shows the icon. It is also called again when Explorer restarts and
// forgets every icon.
func (t *tray) add(owner win32.HWND) {
	t.owner = owner
	if t.icon == 0 {
		t.icon = makeIcon(int(win32.GetSystemMetrics(win32.SM_CXSMICON)))
	}
	data := win32.NOTIFYICONDATA{
		HWnd:            owner,
		ID:              trayID,
		Flags:           win32.NIF_MESSAGE | win32.NIF_ICON | win32.NIF_TIP,
		CallbackMessage: msgTray,
		Icon:            t.icon,
	}
	copy(data.Tip[:len(data.Tip)-1], utf16.Encode([]rune(appName)))
	win32.ShellNotifyIcon(win32.NIM_ADD, &data)
}

// remove takes the icon out of the notification area.
func (t *tray) remove() {
	if t.owner != 0 {
		win32.ShellNotifyIcon(win32.NIM_DELETE, &win32.NOTIFYICONDATA{HWnd: t.owner, ID: trayID})
	}
	if t.icon != 0 {
		win32.DestroyIcon(t.icon)
		t.icon = 0
	}
}

// makeIcon renders the app icon at the given size and wraps it in an HICON.
func makeIcon(size int) win32.HICON {
	if size <= 0 {
		size = 16
	}
	dib, err := win32.NewDIB(size, size)
	if err != nil {
		return 0
	}
	dib.CopyFrom(render.AppIcon(size))
	color := dib.Detach()
	// The colour bitmap carries its own alpha channel; the mask is required
	// by the API but left empty.
	mask := win32.CreateMonoBitmap(int32(size), int32(size))
	defer win32.DeleteObject(win32.HGDIOBJ(color))
	defer win32.DeleteObject(win32.HGDIOBJ(mask))

	return win32.CreateIconIndirect(&win32.ICONINFO{IsIcon: 1, Mask: mask, Color: color})
}
