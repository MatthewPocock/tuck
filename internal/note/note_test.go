package note

import (
	"strings"
	"testing"
	"time"
)

func TestTitle(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"blank", "  \n\n", ""},
		{"plain", "Buy milk\nand eggs", "Buy milk"},
		{"skips blank lines", "\n\n  hello  \n", "hello"},
		{"heading", "## Sprint goals", "Sprint goals"},
		{"task", "- [ ] call the bank", "call the bank"},
		{"done task", "- [x] call the bank", "call the bank"},
		{"bullet", "* idea", "idea"},
		{"quote", "> quoted", "quoted"},
		{"nested markers", "> - # deep", "deep"},
		{"marker only line is skipped", "#\nreal title", "real title"},
		{"truncates", strings.Repeat("é", 200), strings.Repeat("é", maxTitleRunes)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Note{Body: tt.body}
			if got := n.Title(); got != tt.want {
				t.Errorf("Title() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	created := time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)
	in := &Note{
		ID:       "20261014-093000-abcdef",
		Color:    Mint,
		Created:  created,
		Updated:  created.Add(5 * time.Minute),
		Archived: true,
		Body:     "# Title\n\n- [ ] one\n- [x] two\n\n---\n\nafter a rule",
	}

	out := Decode(in.ID, in.Encode())

	if out.ID != in.ID || out.Color != in.Color || out.Archived != in.Archived || out.Body != in.Body {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", out, in)
	}
	if !out.Created.Equal(in.Created) || !out.Updated.Equal(in.Updated) {
		t.Errorf("times = %v / %v, want %v / %v", out.Created, out.Updated, in.Created, in.Updated)
	}
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		wantColor Color
		wantBody  string
	}{
		{"no front matter", "just text\nmore", DefaultColor, "just text\nmore"},
		{"empty file", "", DefaultColor, ""},
		{"crlf and bom", "\xef\xbb\xbf---\r\ncolor: pink\r\n---\r\n\r\nhi\r\nthere\r\n", Pink, "hi\nthere"},
		{"unknown colour falls back", "---\ncolor: chartreuse\n---\nx", DefaultColor, "x"},
		{"empty front matter", "---\n---\nbody", DefaultColor, "body"},
		{"front matter without body", "---\ncolor: sky\n---", Sky, ""},
		{"unterminated fence is body", "---\ncolor: sky\nno end", DefaultColor, "---\ncolor: sky\nno end"},
		{"keeps leading indentation", "---\ncolor: sky\n---\n\n    code", Sky, "    code"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := Decode("id", []byte(tt.data))
			if n.Color != tt.wantColor {
				t.Errorf("Color = %q, want %q", n.Color, tt.wantColor)
			}
			if n.Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", n.Body, tt.wantBody)
			}
		})
	}
}

func TestDecodePreservesUnknownFields(t *testing.T) {
	data := "---\ncolor: sky\nfuture: value\n---\n\nbody\n"
	got := string(Decode("id", []byte(data)).Encode())
	if !strings.Contains(got, "future: value\n") {
		t.Errorf("unknown front matter field was dropped:\n%s", got)
	}
}

func TestColorNext(t *testing.T) {
	seen := map[Color]bool{}
	c := Colors[0]
	for range Colors {
		if seen[c] {
			t.Fatalf("colour %q repeated before the cycle completed", c)
		}
		seen[c] = true
		c = c.Next()
	}
	if c != Colors[0] {
		t.Errorf("cycle ended at %q, want %q", c, Colors[0])
	}
	if got := Color("bogus").Next(); got != DefaultColor {
		t.Errorf("Next() of an invalid colour = %q, want %q", got, DefaultColor)
	}
}

func TestNewID(t *testing.T) {
	now := time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)
	a, b := NewID(now), NewID(now)
	if !strings.HasPrefix(a, "20261014-093000-") {
		t.Errorf("NewID = %q, want a timestamp prefix", a)
	}
	if a == b {
		t.Errorf("two IDs generated in the same second collided: %q", a)
	}
}
