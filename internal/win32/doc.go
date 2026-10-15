// Package win32 is a thin, typed layer over the handful of Win32 APIs the app
// needs: windows and messages, GDI, the shell notification area and DWM.
//
// It deliberately contains no application logic. Functions mirror their Win32
// namesakes closely so that the official documentation applies directly; the
// only liberties taken are Go-friendly types and error returns.
//
// Everything is implemented with direct system calls, so the app needs no C
// toolchain and cross-compiles from any platform. Only 64-bit Windows is
// supported.
package win32
