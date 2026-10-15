package app

import (
	"errors"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"sync"

	"golang.org/x/sys/windows"

	"github.com/MatthewPocock/tuck/internal/win32"
)

// handler receives the messages of one window.
type handler interface {
	wndProc(hwnd win32.HWND, msg uint32, wp, lp uintptr) uintptr
}

var (
	// handlers maps each of our windows to the object that handles its
	// messages. It is only touched from the UI thread.
	handlers = map[win32.HWND]handler{}
	// creating is the handler for the window currently being created. Its
	// first messages arrive before CreateWindowEx returns the handle, so
	// they cannot be routed through handlers yet.
	creating handler

	wndProcPtr = sync.OnceValue(func() uintptr { return win32.NewWndProc(dispatch) })
)

// dispatch is the single window procedure shared by all of our window
// classes. It routes each message to the window's handler.
func dispatch(hwnd win32.HWND, msg uint32, wp, lp uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			// A panic must not unwind into Windows. Record it and stop.
			log.Printf("panic in window procedure (msg %#x): %v\n%s", msg, r, debug.Stack())
			win32.MessageBox(0, fmt.Sprintf("%s hit an unexpected error and has to close.\n\n%v", appName, r),
				appName, win32.MB_OK|win32.MB_ICONERROR)
			os.Exit(2)
		}
	}()

	h, ok := handlers[hwnd]
	if !ok {
		if creating == nil {
			return win32.DefWindowProc(hwnd, msg, wp, lp)
		}
		h = creating
		handlers[hwnd] = h
	}
	if msg == win32.WM_DESTROY {
		defer delete(handlers, hwnd)
	}
	return h.wndProc(hwnd, msg, wp, lp)
}

// registerClass registers a window class that dispatches to handlers.
// Registering the same class again is a no-op.
func registerClass(inst win32.HINSTANCE, name string, cursor uintptr) error {
	err := win32.RegisterClassEx(&win32.WNDCLASSEX{
		WndProc:   wndProcPtr(),
		Instance:  inst,
		Cursor:    win32.LoadCursor(cursor),
		ClassName: win32.UTF16(name),
	})
	if errors.Is(err, windows.ERROR_CLASS_ALREADY_EXISTS) {
		return nil
	}
	return err
}

// createWindow creates a top-level window whose messages go to h.
func createWindow(h handler, inst win32.HINSTANCE, exStyle uint32, class, title string, style uint32) (win32.HWND, error) {
	creating = h
	defer func() { creating = nil }()
	return win32.CreateWindowEx(exStyle, class, title, style, 0, 0, 0, 0, 0, 0, inst)
}
