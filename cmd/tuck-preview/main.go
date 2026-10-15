// Command tuck-preview renders the app's custom-drawn UI to PNG files.
//
// The deck, the editor header and the icon are all painted by portable Go
// code, so this tool lets contributors on any operating system see what a
// change looks like without a Windows machine:
//
//	go run ./cmd/tuck-preview -out preview
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/fonttext"
	"github.com/MatthewPocock/tuck/internal/note"
	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/search"
	"github.com/MatthewPocock/tuck/internal/settings"
)

var sampleNotes = []deck.Item{
	{ID: "1", Title: "Groceries for the weekend", Color: note.Yellow},
	{ID: "2", Title: "Standup: blocked on review", Color: note.Sky},
	{ID: "3", Title: "Ideas for Ana's birthday", Color: note.Pink},
	{ID: "4", Title: "Wi-Fi: guest network password is on the fridge", Color: note.Mint},
	{ID: "5", Title: "Read later", Color: note.Lilac},
	{ID: "6", Title: "", Color: note.Peach},
	{ID: "7", Title: "Scratch", Color: note.Stone},
}

func main() {
	out := flag.String("out", "preview", "directory to write the PNG files to")
	scale := flag.Float64("scale", 2, "display scale factor to render at")
	icons := flag.String("icons", "", "instead of previews, write the app icon at every size to this directory")
	flag.Parse()

	var err error
	if *icons != "" {
		err = writeIcons(*icons)
	} else {
		err = run(*out, *scale)
	}
	if err != nil {
		log.Fatal(err)
	}
}

// iconSizes are the icon sizes embedded in the Windows executable.
var iconSizes = []int{16, 24, 32, 48, 64, 256}

// writeIcons renders the app icon at each size natively, which stays crisper
// at small sizes than scaling down one large image. The files feed the
// resource compiler (see cmd/tuck/winres).
func writeIcons(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, size := range iconSizes {
		path := filepath.Join(dir, fmt.Sprintf("icon%d.png", size))
		if err := writePNG(path, render.AppIcon(size)); err != nil {
			return err
		}
		fmt.Println("wrote", path)
	}
	return nil
}

func run(dir string, scale float64) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	text := fonttext.New()

	images := map[string]image.Image{
		"deck-right.png": deckStrip(deck.Right, scale, text),
		"deck-left.png":  deckStrip(deck.Left, scale, text),
		"header.png":     headers(scale, text),
		"settings.png":   settingsWindow(scale),
		"search.png":     searchWindow(scale, text),
		"icon.png":       icons(),
	}
	for name, img := range images {
		path := filepath.Join(dir, name)
		if err := writePNG(path, img); err != nil {
			return err
		}
		fmt.Println("wrote", path)
	}
	return nil
}

// deckStrip renders the deck at several moments of its life, side by side:
// at rest, part-way through expanding, fully expanded and with a tab hovered.
func deckStrip(edge deck.Edge, scale float64, text render.TextDrawer) image.Image {
	type moment struct {
		elapsed time.Duration
		hover   int // tab index to hover once expanded, or -1
		active  string
	}
	moments := []moment{
		{0, -1, ""},
		{90 * time.Millisecond, -1, ""},
		{160 * time.Millisecond, -1, ""},
		{2 * time.Second, -1, ""},
		{2 * time.Second, 2, "5"},
	}

	var frames []*image.RGBA
	for _, m := range moments {
		d := deck.New(edge)
		d.SetScale(scale)
		d.SetItems(sampleNotes)
		d.SetActive(m.active)
		if m.elapsed > 0 {
			px, py := midpoint(d.Frame().Pill)
			d.Pointer(px, py, true)
			advance(d, m.elapsed)
		}
		if m.hover >= 0 {
			px, py := midpoint(d.Frame().Tabs[m.hover].Rect)
			d.Pointer(px, py, true)
			advance(d, time.Second)
		}
		c := render.NewCanvas(1, 1)
		render.DrawDeck(c, d.Frame(), text)
		frames = append(frames, c.Img)
	}

	// Compose the frames over a wallpaper-like backdrop, half light and half
	// dark, to check contrast against both.
	w, h := frames[0].Bounds().Dx(), frames[0].Bounds().Dy()
	sheet := image.NewRGBA(image.Rect(0, 0, w*len(frames), h*2))
	half := image.Rect(0, 0, sheet.Bounds().Dx(), h)
	draw.Draw(sheet, half, image.NewUniform(color.NRGBA{0xE9, 0xEC, 0xF1, 0xFF}), image.Point{}, draw.Src)
	draw.Draw(sheet, half.Add(image.Pt(0, h)), image.NewUniform(color.NRGBA{0x20, 0x24, 0x2C, 0xFF}), image.Point{}, draw.Src)
	for i, f := range frames {
		for row := range 2 {
			at := image.Pt(i*w, row*h)
			draw.Draw(sheet, f.Bounds().Add(at), f, image.Point{}, draw.Over)
		}
	}
	return sheet
}

// settingsWindow renders the settings window in the light and dark themes,
// side by side, with a shortcut being recorded in the dark one.
func settingsWindow(scale float64) image.Image {
	state := settings.State{
		Config:    config.Default(),
		Autostart: true,
		NotesDir:  `C:\Users\Ana\OneDrive\Notes`,
		Version:   "v1.0.0",
		Hover:     settings.ChangeFolder,
	}
	text := fonttext.New()
	l := settings.NewLayout(scale, state)
	gap := int(16 * scale)
	out := image.NewRGBA(image.Rect(0, 0, 2*l.Width+gap, l.Height))
	for i, dark := range []bool{false, true} {
		s := state
		if dark {
			s.Capturing, s.Hover = settings.ToggleDeckKey, settings.Theme
			s.HoverSegment = 2
		}
		c := render.NewCanvas(1, 1)
		render.DrawSettings(c, settings.NewLayout(scale, s), s, dark, text)
		draw.Draw(out, c.Img.Bounds().Add(image.Pt(i*(l.Width+gap), 0)), c.Img, image.Point{}, draw.Src)
	}
	return out
}

// searchWindow renders the search window in the light and dark themes,
// side by side, with a query typed in.
func searchWindow(scale float64, text render.TextDrawer) image.Image {
	at := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	var notes []*note.Note
	for i, body := range []string{
		"Groceries for the weekend\n- [ ] oat milk\n- [x] coffee beans",
		"Standup: blocked on review\nAsk Sam about the milk budget for the office",
		"Ideas for Ana's birthday\nA milk frother?",
		"Old recipes\nMilk bread, brioche",
	} {
		n := note.New(at.Add(time.Duration(i)*time.Hour), note.Colors[i*2%len(note.Colors)])
		n.Body = body
		n.Archived = i == 3
		notes = append(notes, n)
	}
	results := search.Find(notes, "milk")
	l := search.NewLayout(scale, len(results), 1, 0)
	gap := int(16 * scale)
	out := image.NewRGBA(image.Rect(0, 0, 2*l.Width+gap, l.Height))
	for i, dark := range []bool{false, true} {
		c := render.NewCanvas(1, 1)
		render.DrawSearch(c, l, render.SearchState{Query: "milk", Results: results, Selected: 1, Hover: -1}, dark, text)
		// The query itself is a native text box; stand in for it.
		_, ink := render.SearchColors(dark)
		r := l.Input
		text.DrawText(c.Img, image.Rect(int(r.X), int(r.Y), int(r.X+r.W), int(r.Y+r.H)), "milk",
			render.TextStyle{Size: 17 * scale, Color: ink, Opacity: 1})
		draw.Draw(out, c.Img.Bounds().Add(image.Pt(i*(l.Width+gap), 0)), c.Img, image.Point{}, draw.Src)
	}
	return out
}

// headers renders the editor header for every colour in both themes.
func headers(scale float64, text render.TextDrawer) image.Image {
	layout := render.HeaderLayout{Width: int(360 * scale), Scale: scale}
	h := layout.Height()
	sheet := image.NewRGBA(image.Rect(0, 0, layout.Width*2, h*len(note.Colors)))
	for i, col := range note.Colors {
		for j, dark := range []bool{false, true} {
			state := render.HeaderState{Pinned: i%2 == 1}
			if i == 2 {
				state.Hover = render.ButtonMenu
			}
			if i == 3 {
				// Folded to its title bar.
				state.Title = "Groceries for the weekend"
			}
			c := render.NewCanvas(1, 1)
			render.DrawHeader(c, layout, render.Palette(col, dark), state, text)
			at := image.Pt(j*layout.Width, i*h)
			draw.Draw(sheet, c.Img.Bounds().Add(at), c.Img, image.Point{}, draw.Src)
		}
	}
	return sheet
}

// icons renders the app icon at the sizes Windows uses.
func icons() image.Image {
	width := 0
	for _, s := range iconSizes {
		width += s + 16
	}
	sheet := image.NewRGBA(image.Rect(0, 0, width+16, 256+32))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.NRGBA{0xE9, 0xEC, 0xF1, 0xFF}), image.Point{}, draw.Src)
	x := 16
	for _, s := range iconSizes {
		icon := render.AppIcon(s)
		draw.Draw(sheet, icon.Bounds().Add(image.Pt(x, 16)), icon, image.Point{}, draw.Over)
		x += s + 16
	}
	return sheet
}

func advance(d *deck.Deck, total time.Duration) {
	const step = 8 * time.Millisecond
	for elapsed := time.Duration(0); elapsed < total; elapsed += step {
		d.Advance(step)
	}
}

func midpoint(r deck.Rect) (int, int) {
	return int(r.X + r.W/2), int(r.Y + r.H/2)
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
