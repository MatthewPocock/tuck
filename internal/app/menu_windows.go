package app

import "github.com/MatthewPocock/tuck/internal/win32"

// menu builds a native popup menu whose items run Go callbacks.
//
// Items are identified by sequential IDs shared between a menu and all of
// its submenus, so a single TrackPopupMenu call can report a choice made at
// any depth.
type menu struct {
	handle  win32.HMENU
	root    *menu
	actions []func() // owned by the root; an item's ID is its index + 1
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

// check adds a command with a check mark shown when checked is true.
func (m *menu) check(label string, checked bool, action func()) {
	var flags uint32
	if checked {
		flags = win32.MF_CHECKED
	}
	m.add(label, flags, action)
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
