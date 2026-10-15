package search

import "github.com/MatthewPocock/tuck/internal/deck"

// Layout metrics in device-independent pixels.
const (
	Width     = 560.0
	FieldH    = 56.0
	rowH      = 54.0
	listPad   = 6.0
	messageH  = 64.0
	footerH   = 34.0
	MaxRows   = 7
	fieldPadL = 50.0
	fieldPadR = 18.0
)

// Layout is the arrangement of the search window for a number of results.
type Layout struct {
	Scale float64
	// Width and Height are the window size in pixels.
	Width, Height int
	// Field is the band holding the query; Input is where the text box
	// goes within it.
	Field, Input deck.Rect
	// Rows are the rectangles of the visible results, the first of which
	// is result First.
	Rows  []deck.Rect
	First int
	// Message holds the text shown when nothing matches; it is empty when
	// there are results.
	Message deck.Rect
	// Footer holds the keyboard hints.
	Footer deck.Rect
}

// NewLayout arranges the window to show count results, scrolled so that the
// selected one is visible. first is the previously first visible result.
func NewLayout(scale float64, count, selected, first int) Layout {
	px := func(v float64) float64 { return v * scale }
	w := px(Width)
	l := Layout{Scale: scale}
	l.Field = deck.Rect{W: w, H: px(FieldH)}
	inputH := px(28)
	l.Input = deck.Rect{X: px(fieldPadL), Y: (px(FieldH) - inputH) / 2, W: w - px(fieldPadL) - px(fieldPadR), H: inputH}
	y := px(FieldH)

	if count == 0 {
		l.Message = deck.Rect{Y: y, W: w, H: px(messageH)}
		y += px(messageH)
	} else {
		visible := min(count, MaxRows)
		first = max(0, min(first, count-visible))
		if selected >= 0 {
			if selected < first {
				first = selected
			} else if selected >= first+visible {
				first = selected - visible + 1
			}
		}
		l.First = first
		y += px(listPad)
		for range visible {
			l.Rows = append(l.Rows, deck.Rect{X: px(listPad), Y: y, W: w - 2*px(listPad), H: px(rowH)})
			y += px(rowH)
		}
		y += px(listPad)
	}
	l.Footer = deck.Rect{Y: y, W: w, H: px(footerH)}
	y += px(footerH)
	l.Width, l.Height = int(w+0.5), int(y+0.5)
	return l
}

// Hit returns the result under the point (x, y), or -1.
func (l Layout) Hit(x, y int) int {
	for i, r := range l.Rows {
		if r.Contains(float64(x), float64(y)) {
			return l.First + i
		}
	}
	return -1
}

// Scroll returns the first visible result after scrolling by lines rows,
// for count results.
func Scroll(first, lines, count int) int {
	return max(0, min(first+lines, count-MaxRows))
}
