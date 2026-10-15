//go:build !windows

// Command tuck is a sticky notes app for Windows. On other platforms it only
// explains itself; see cmd/tuck-preview for a way to render the UI anywhere.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "tuck is a Windows application. Build it with GOOS=windows, "+
		"or run ./cmd/tuck-preview to render its UI to PNG files on this platform.")
	os.Exit(1)
}
