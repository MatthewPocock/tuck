# Contributing to Tuck

Thanks for taking an interest! Tuck is small on purpose, and contributions
that keep it that way are very welcome.

## Getting set up

You need [Go](https://go.dev/dl/) (the version in `go.mod`) and nothing else —
no C compiler, no SDK. Tuck cross-compiles from macOS and Linux:

```sh
make build      # bin/tuck.exe
make test       # portable tests
make vet lint   # static checks for the Windows build
make preview    # render the deck, header, settings and icon to ./preview/*.png
```

On Windows, plain `go build ./cmd/tuck` and `go test ./...` work too.

## How the code is organised

| Package | What it does | Runs on |
| --- | --- | --- |
| `internal/note` | The note model and its Markdown + front matter file format | anywhere |
| `internal/store` | Reads and writes notes in a folder | anywhere |
| `internal/config` | The settings file | anywhere |
| `internal/hotkey` | Parses shortcuts like `Ctrl+Alt+N` | anywhere |
| `internal/deck` | Layout, hit testing and animation of the deck — pure geometry | anywhere |
| `internal/settings` | Layout and hit testing of the settings window | anywhere |
| `internal/markdown` | Finds the Markdown the editor styles and task boxes to click; ticks tasks off and strips Markdown for copying | anywhere |
| `internal/search` | Finds notes by their words; lays out the search window | anywhere |
| `internal/render` | Paints the deck, editor header, settings and search windows and the icon into images | anywhere |
| `internal/win32` | Thin typed bindings for the Win32 APIs used | Windows |
| `internal/app` | Windows, tray icon, hotkeys: wires input to the packages above | Windows |

The rule of thumb: **if it can be decided without calling Windows, it belongs
in a portable package** with a unit test. `internal/app` should stay a thin
shell. That is what lets most of the app be developed and tested on any OS.

## Testing

- Portable packages have ordinary table-driven tests.
- `internal/app` has integration tests that create real windows. They run on
  Windows (and in CI on `windows-latest`). They also pass under
  [Wine](https://www.winehq.org/), which is handy on macOS and Linux:

  ```sh
  GOOS=windows GOARCH=amd64 go test -c -o app.test.exe ./internal/app
  wine app.test.exe -test.v
  ```

- Set `TUCK_SNAPSHOT_DIR` when running those tests to save PNG captures of
  the editor window.
- Changed how something looks? Run `make preview` and attach the before/after
  images to your pull request.

## Guidelines

- Run `gofmt`, `make vet` and `make test` before sending a pull request.
- Keep dependencies to the Go standard library and `golang.org/x`.
- No cgo: being able to build with nothing but Go is a feature.
- All UI code runs on one OS thread. Do not touch windows from goroutines.
- Comments explain *why*; the code should already say *what*.
- If you changed the icon or `cmd/tuck/winres/winres.json`, run
  `make resources` and commit the regenerated `.syso` files.
