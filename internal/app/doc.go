// Package app is the Windows shell of Tuck: it owns the windows, the
// notification area icon and the global hotkeys, and wires user input to the
// portable model packages (deck, note, store, config) and the renderer.
//
// All of it runs on a single, locked OS thread, as Win32 requires. Nothing in
// this package may be called from another goroutine.
package app
