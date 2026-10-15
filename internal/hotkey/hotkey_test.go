package hotkey

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in      string
		want    Hotkey
		wantErr bool
	}{
		{in: "", want: Hotkey{}},
		{in: "Ctrl+Alt+N", want: Hotkey{ModCtrl | ModAlt, 'N'}},
		{in: " ctrl + shift + 7 ", want: Hotkey{ModCtrl | ModShift, '7'}},
		{in: "Win+F2", want: Hotkey{ModWin, 0x71}},
		{in: "Control+Alt+Space", want: Hotkey{ModCtrl | ModAlt, 0x20}},
		{in: "Ctrl+Alt+.", want: Hotkey{ModCtrl | ModAlt, 0xBE}},
		{in: "Alt+F24", want: Hotkey{ModAlt, 0x87}},
		{in: "N", wantErr: true},
		{in: "Ctrl+Alt", wantErr: true},
		{in: "Ctrl+A+B", wantErr: true},
		{in: "Ctrl+F25", wantErr: true},
		{in: "Ctrl+Bogus", wantErr: true},
		{in: "Ctrl++", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsZero(t *testing.T) {
	if !(Hotkey{}).IsZero() {
		t.Error("zero Hotkey reported as set")
	}
	if (Hotkey{ModCtrl, 'N'}).IsZero() {
		t.Error("non-zero Hotkey reported as unset")
	}
}

func TestFormatRoundTrips(t *testing.T) {
	for _, s := range []string{"Ctrl+Alt+N", "Ctrl+Shift+F12", "Alt+Win+Space", "Ctrl+Alt+.", "Shift+Win+PageDown", `Ctrl+\`, "Ctrl+7"} {
		h, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		if got := Format(h); got != s {
			t.Errorf("Format(Parse(%q)) = %q", s, got)
		}
	}
}

func TestFormatOrdersModifiersAndHandlesUnknownKeys(t *testing.T) {
	h, _ := Parse("win+shift+alt+ctrl+a")
	if got, want := Format(h), "Ctrl+Alt+Shift+Win+A"; got != want {
		t.Errorf("Format = %q, want %q", got, want)
	}
	if got := Format(Hotkey{}); got != "" {
		t.Errorf("Format(zero) = %q, want empty", got)
	}
	if got := Format(Hotkey{Mods: ModCtrl, Key: 0xFF}); got != "" {
		t.Errorf("Format of an unknown key = %q, want empty", got)
	}
}
