package render

import (
	"image/color"
	"math"

	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/hotkey"
	"github.com/MatthewPocock/tuck/internal/note"
	"github.com/MatthewPocock/tuck/internal/settings"
)

// chrome is the neutral palette of the settings window.
type chrome struct {
	bg, text, muted, problem, line color.NRGBA
	// control is the fill of an idle control; raised is a selected option
	// or a button under the pointer.
	control, controlHover, raised color.NRGBA
	accent, accentInk             color.NRGBA
}

func chromeFor(dark bool) chrome {
	yellow := Palette(note.Yellow, false)
	if dark {
		return chrome{
			bg: rgb(0x202124), text: rgb(0xECECEC), muted: rgb(0x9C9DA3), problem: rgb(0xFF8A80),
			line:    color.NRGBA{0xFF, 0xFF, 0xFF, 0x1A},
			control: color.NRGBA{0xFF, 0xFF, 0xFF, 0x12}, controlHover: color.NRGBA{0xFF, 0xFF, 0xFF, 0x1E},
			raised: rgb(0x45464D), accent: yellow.Fill, accentInk: yellow.Ink,
		}
	}
	return chrome{
		bg: rgb(0xF6F6F4), text: rgb(0x1F1F1F), muted: rgb(0x6B6B70), problem: rgb(0xC62828),
		line:    color.NRGBA{0x00, 0x00, 0x00, 0x14},
		control: color.NRGBA{0x00, 0x00, 0x00, 0x0D}, controlHover: color.NRGBA{0x00, 0x00, 0x00, 0x17},
		raised: rgb(0xFFFFFF), accent: yellow.Fill, accentInk: yellow.Ink,
	}
}

// Settings typography in device-independent pixels.
const (
	settingsTitleSize   = 17.0
	settingsSectionSize = 12.0
	settingsTextSize    = 13.0
	settingsSmallSize   = 12.0
	controlRadius       = 7.0
)

// DrawSettings paints the settings window onto the canvas, which is resized
// to the layout.
func DrawSettings(c *Canvas, l settings.Layout, s settings.State, dark bool, text TextDrawer) {
	c.Resize(l.Width, l.Height)
	p := chromeFor(dark)
	c.Fill(p.bg)
	sc := l.Scale
	hairline := math.Max(1, math.Round(sc))

	draw := func(r deck.Rect, str string, size float64, bold bool, col color.NRGBA, align Align) {
		if text != nil {
			text.DrawText(c.Img, pixelRect(r), str, TextStyle{Size: size * sc, Bold: bold, Color: col, Opacity: 1, Align: align})
		}
	}

	for _, t := range l.Texts {
		switch t.Role {
		case settings.RoleTitle:
			draw(t.Rect, t.Text, settingsTitleSize, true, p.text, AlignLeft)
		case settings.RoleSection:
			draw(t.Rect, t.Text, settingsSectionSize, true, p.muted, AlignLeft)
		case settings.RoleLabel:
			draw(t.Rect, t.Text, settingsTextSize, false, p.text, AlignLeft)
		case settings.RoleValue:
			draw(t.Rect, t.Text, settingsTextSize, false, p.text, AlignCenter)
		case settings.RoleMessage:
			col := p.muted
			if s.Problem {
				col = p.problem
			}
			draw(t.Rect, t.Text, settingsSmallSize, false, col, AlignLeft)
		case settings.RolePath:
			if text != nil {
				text.DrawText(c.Img, pixelRect(t.Rect), t.Text, TextStyle{Size: settingsSmallSize * sc, Color: p.muted, Opacity: 1, Path: true})
			}
		case settings.RoleFooter:
			draw(t.Rect, t.Text, settingsSmallSize, false, p.muted, AlignLeft)
		}
	}
	c.FillRect(deck.Rect{Y: l.Separator, W: float64(l.Width), H: hairline}, p.line)

	for _, it := range l.Items {
		hover := s.Hover == it.Control
		pressed := s.Pressed == it.Control
		r := it.Rect
		radius := controlRadius * sc
		switch it.Kind {
		case settings.KindClose:
			if hover {
				c.FillRoundRect(r, 6*sc, p.controlHover)
			}
			cx, cy, arm := r.X+r.W/2, r.Y+r.H/2, 4.5*sc
			c.Line(cx-arm, cy-arm, cx+arm, cy+arm, 1.5*sc, p.muted)
			c.Line(cx-arm, cy+arm, cx+arm, cy-arm, 1.5*sc, p.muted)

		case settings.KindSegmented:
			c.FillRoundRect(r, radius, p.control)
			selected := s.Selected(it.Control)
			inset := 2 * sc
			for i, seg := range it.Segments {
				label := settings.Options[it.Control][i]
				if i == selected {
					knob := seg.Inflate(-inset, -inset)
					c.Shadow(knob, radius-inset, 3*sc, sc, 0.12)
					c.FillRoundRect(knob, radius-inset, p.raised)
					draw(seg, label, settingsTextSize, true, p.text, AlignCenter)
					continue
				}
				if hover && s.HoverSegment == i {
					c.FillRoundRect(seg.Inflate(-inset, -inset), radius-inset, p.control)
				}
				draw(seg, label, settingsTextSize, false, p.muted, AlignCenter)
			}

		case settings.KindStep:
			fill := p.control
			if pressed || hover {
				fill = p.controlHover
			}
			c.FillRoundRect(r, radius, fill)
			cx, cy, arm := r.X+r.W/2, r.Y+r.H/2, 4.5*sc
			c.Line(cx-arm, cy, cx+arm, cy, 1.5*sc, p.text)
			if it.Control == settings.FontLarger {
				c.Line(cx, cy-arm, cx, cy+arm, 1.5*sc, p.text)
			}

		case settings.KindHotkey:
			capturing := s.Capturing == it.Control
			fill := p.control
			if hover && !capturing {
				fill = p.controlHover
			}
			c.FillRoundRect(r, radius, fill)
			label, col, bold := hotkeyLabel(s, it.Control), p.text, true
			if capturing {
				c.StrokeRoundRect(r, radius, 2*sc, p.accent)
				label, col, bold = "Press a shortcut…", p.muted, false
			} else if label == "" {
				label, col, bold = "Off", p.muted, false
			}
			draw(r, label, settingsTextSize, bold, col, AlignCenter)

		case settings.KindToggle:
			on := s.On(it.Control)
			track := p.control
			if on {
				track = p.accent
			} else if hover {
				track = p.controlHover
			}
			c.FillRoundRect(r, r.H/2, track)
			if !on {
				c.StrokeRoundRect(r, r.H/2, hairline, p.line)
			}
			knob := r.H/2 - 3*sc
			x := r.X + r.H/2
			if on {
				x = r.X + r.W - r.H/2
			}
			knobCol := p.muted
			if on {
				knobCol = p.accentInk
			}
			c.FillCircle(x, r.Y+r.H/2, knob, knobCol)

		case settings.KindButton:
			fill := p.control
			if pressed || hover {
				fill = p.controlHover
			}
			c.FillRoundRect(r, radius, fill)
			draw(r, it.Label, settingsTextSize, false, p.text, AlignCenter)

		case settings.KindLink:
			col := p.muted
			if hover {
				col = p.text
			}
			draw(r, it.Label, settingsSmallSize, false, col, AlignRight)
		}
	}
}

// hotkeyLabel is the shortcut a hotkey field shows, normalised to the
// canonical spelling.
func hotkeyLabel(s settings.State, c settings.Control) string {
	raw := *settings.Hotkey(&s.Config.Hotkeys, c)
	h, err := hotkey.Parse(raw)
	if err != nil {
		return raw
	}
	return hotkey.Format(h)
}
