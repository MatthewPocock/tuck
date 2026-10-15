// Package hotkey parses human-readable keyboard shortcuts such as
// "Ctrl+Alt+N" into the modifier flags and virtual-key codes used by the
// Win32 RegisterHotKey API.
//
// The package has no platform dependencies so it can be tested anywhere.
package hotkey

import (
	"fmt"
	"strconv"
	"strings"
)

// Modifier flags, matching the Win32 MOD_* constants.
const (
	ModAlt   uint32 = 0x0001
	ModCtrl  uint32 = 0x0002
	ModShift uint32 = 0x0004
	ModWin   uint32 = 0x0008
)

// Hotkey is a key combination: a set of modifiers plus one virtual key.
type Hotkey struct {
	Mods uint32
	Key  uint32
}

// IsZero reports whether the hotkey is unset.
func (h Hotkey) IsZero() bool { return h == Hotkey{} }

var modifiers = map[string]uint32{
	"alt":     ModAlt,
	"ctrl":    ModCtrl,
	"control": ModCtrl,
	"shift":   ModShift,
	"win":     ModWin,
	"super":   ModWin,
}

// namedKeys maps key names to Win32 virtual-key codes.
var namedKeys = map[string]uint32{
	"backspace": 0x08,
	"tab":       0x09,
	"enter":     0x0D,
	"return":    0x0D,
	"esc":       0x1B,
	"escape":    0x1B,
	"space":     0x20,
	"pageup":    0x21,
	"pagedown":  0x22,
	"end":       0x23,
	"home":      0x24,
	"left":      0x25,
	"up":        0x26,
	"right":     0x27,
	"down":      0x28,
	"insert":    0x2D,
	"delete":    0x2E,
	";":         0xBA,
	"=":         0xBB,
	",":         0xBC,
	"-":         0xBD,
	".":         0xBE,
	"/":         0xBF,
	"`":         0xC0,
	"[":         0xDB,
	`\`:         0xDC,
	"]":         0xDD,
	"'":         0xDE,
}

// Parse converts a shortcut such as "Ctrl+Alt+N" or "win+shift+f2" into a
// Hotkey. An empty string yields the zero Hotkey, which callers treat as
// "disabled". A global hotkey must include at least one modifier.
func Parse(s string) (Hotkey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Hotkey{}, nil
	}

	var h Hotkey
	for _, part := range strings.Split(s, "+") {
		name := strings.ToLower(strings.TrimSpace(part))
		if mod, ok := modifiers[name]; ok {
			h.Mods |= mod
			continue
		}
		key, ok := parseKey(name)
		if !ok {
			return Hotkey{}, fmt.Errorf("hotkey: unknown key %q in %q", part, s)
		}
		if h.Key != 0 {
			return Hotkey{}, fmt.Errorf("hotkey: more than one key in %q", s)
		}
		h.Key = key
	}

	if h.Key == 0 {
		return Hotkey{}, fmt.Errorf("hotkey: no key in %q", s)
	}
	if h.Mods == 0 {
		return Hotkey{}, fmt.Errorf("hotkey: %q needs at least one modifier", s)
	}
	return h, nil
}

func parseKey(name string) (uint32, bool) {
	if key, ok := namedKeys[name]; ok {
		return key, true
	}
	if len(name) == 1 {
		c := name[0]
		switch {
		case c >= 'a' && c <= 'z':
			return uint32(c - 'a' + 'A'), true
		case c >= '0' && c <= '9':
			return uint32(c), true
		}
	}
	if rest, ok := strings.CutPrefix(name, "f"); ok {
		if n, err := strconv.Atoi(rest); err == nil && n >= 1 && n <= 24 {
			return 0x70 + uint32(n-1), true
		}
	}
	return 0, false
}
