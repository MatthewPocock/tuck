package app

import (
	"image"

	"github.com/MatthewPocock/tuck/internal/win32"
)

// menu builds a native popup menu whose items run Go callbacks.
//
// Items are identified by sequential IDs shared between a menu and all of
// its submenus, so a single TrackPopupMenu call can report a choice made at
// any depth.
type menu struct {
	handle  win32.HMENU
	root    *menu
	actions []func() // owned by the root; an item's ID is its index + 1
	// bitmaps are owned by the root and freed once the menu is dismissed.
	bitmaps []win32.HBITMAP
}

func newMenu() *menu {
	m := &menu{handle: win32.CreatePopupMenu()}
	m.root = m
	return m
}

// item adds a command.
func (m *menu) item(label string, action func()) {
	m.add(label, 0, action)
}

// image shows a picture beside the most recently added item.
func (m *menu) image(img *image.RGBA) {
	b := img.Bounds()
	dib, err := win32.NewDIB(b.Dx(), b.Dy())
	if err != nil {
		return
	}
	dib.CopyFrom(img)
	bmp := dib.Detach()
	m.root.bitmaps = append(m.root.bitmaps, bmp)
	win32.SetMenuItemBitmap(m.handle, uintptr(len(m.root.actions)), bmp)
}

// disabled adds a greyed-out, non-interactive entry.
func (m *menu) disabled(label string) {
	m.add(label, win32.MF_GRAYED, nil)
}

func (m *menu) add(label string, flags uint32, action func()) {
	m.root.actions = append(m.root.actions, action)
	win32.AppendMenu(m.handle, win32.MF_STRING|flags, uintptr(len(m.root.actions)), label)
}

func (m *menu) separator() {
	win32.AppendMenu(m.handle, win32.MF_SEPARATOR, 0, "")
}

// submenu adds a nested menu and returns it for populating.
func (m *menu) submenu(label string) *menu {
	sub := &menu{handle: win32.CreatePopupMenu(), root: m.root}
	win32.AppendMenu(m.handle, win32.MF_POPUP, uintptr(sub.handle), label)
	return sub
}

// show displays the menu at a screen position, blocks until it is dismissed
// and runs the chosen item's callback, if any. Submenus are destroyed along
// with the root menu.
func (m *menu) show(owner win32.HWND, at win32.POINT) {
	id := win32.TrackPopupMenu(m.handle, win32.TPM_RETURNCMD|win32.TPM_NONOTIFY|win32.TPM_RIGHTBUTTON, at.X, at.Y, owner)
	win32.DestroyMenu(m.handle)
	for _, bmp := range m.bitmaps {
		win32.DeleteObject(win32.HGDIOBJ(bmp))
	}
	// Windows only dismisses a popup menu on an outside click if a message
	// follows it; this is the documented workaround.
	win32.PostMessage(owner, win32.WM_NULL, 0, 0)

	if id == 0 || int(id) > len(m.actions) {
		return
	}
	if action := m.actions[id-1]; action != nil {
		action()
	}
}
