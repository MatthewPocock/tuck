// Command tuck is a sticky notes app for Windows that keeps every note in a
// deck tucked against the edge of the screen.
package main

import (
	"log"
	"os"
	"runtime"

	"github.com/MatthewPocock/tuck/internal/app"
	"github.com/MatthewPocock/tuck/internal/win32"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func init() {
	// Win32 windows belong to the thread that created them, so the main
	// goroutine must stay on the main thread for the life of the process.
	runtime.LockOSThread()
}

func main() {
	if err := app.Run(version); err != nil {
		log.Print(err)
		win32.MessageBox(0, "Tuck could not start.\n\n"+err.Error(), "Tuck", win32.MB_OK|win32.MB_ICONERROR)
		os.Exit(1)
	}
}
