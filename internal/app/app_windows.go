package app

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"slices"
	"time"

	"golang.org/x/sys/windows"

	"github.com/MatthewPocock/tuck/internal/config"
	"github.com/MatthewPocock/tuck/internal/deck"
	"github.com/MatthewPocock/tuck/internal/hotkey"
	"github.com/MatthewPocock/tuck/internal/note"
	"github.com/MatthewPocock/tuck/internal/render"
	"github.com/MatthewPocock/tuck/internal/store"
	"github.com/MatthewPocock/tuck/internal/win32"
)

const (
	appName = "Tuck"

	controllerClass = "TuckController"
	deckClass       = "TuckDeck"
	editorClass     = "TuckEditor"
	settingsClass   = "TuckSettings"

	// msgTray carries notification area icon events.
	msgTray = win32.WM_APP + 1
	// msgWake is posted by a second instance to reveal the running one.
	msgWake = win32.WM_APP + 2

	hotkeyNewNote    = 1
	hotkeyToggleDeck = 2

	// busyTimer periodically checks whether a full-screen app is in front,
	// in which case the deck steps out of the way.
	busyTimer    = 1
	busyInterval = 2000 // milliseconds

	// changesTimer picks up changes other programs made to note files. It
	// runs shortly after the notes folder reports a change, so that a burst
	// of writes is handled once, or periodically if the folder cannot be
	// watched.
	changesTimer    = 2
	changesDelay    = 300  // milliseconds
	changesInterval = 5000 // milliseconds

	// screenMargin keeps the editor away from the edges of the work area,
	// in device-independent pixels.
	screenMargin = 8
)

const welcomeNote = `Welcome to Tuck

Your notes live in the deck at the edge of the screen.

- Hover the deck to fan it out
- Click a note to open it, or + to start a new one
- Right-click a note to recolour, archive or delete it
- Press Esc to tuck a note away again

Notes are plain Markdown files, saved as you type.
Right-click the tray icon for settings.`

// App is the running application.
type App struct {
	version string
	paths   paths
	cfg     config.Config
	store   *store.Store
	// notes holds every note, archived or not, oldest first.
	notes []*note.Note
	// lastTrashed is the ID of the most recently deleted note, which can
	// still be restored.
	lastTrashed string

	inst       win32.HINSTANCE
	controller win32.HWND
	deck       *deckWindow
	editor     *editorWindow
	// settings is created when first opened.
	settings *settingsWindow
	tray     tray
	text     *gdiText
	watch    *folderWatch

	// taskbarCreated is the message Explorer broadcasts when it restarts.
	taskbarCreated uint32
	// menuOpen is true while a popup menu is showing, so that the focus
	// change it causes does not close the editor.
	menuOpen bool
	// menuHoldsDeck is true while a menu opened from an expanded deck is
	// showing, so the deck does not collapse underneath it.
	menuHoldsDeck bool
	// saveFailed records that a save error has already been reported, to
	// avoid a dialog on every keystroke.
	saveFailed bool
}

// Run starts the app and blocks until the user quits. It must be called from
// the main goroutine, which it expects to be locked to its OS thread.
func Run(version string) error {
	// The manifest normally sets this; doing it here too keeps unmanifested
	// development builds sharp on high-DPI displays.
	win32.SetProcessDpiAwarenessContext(win32.DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2)

	release, already, err := acquireSingleInstance()
	if err != nil {
		return err
	}
	defer release()
	if already {
		if running := win32.FindWindow(controllerClass); running != 0 {
			win32.PostMessage(running, msgWake, 0, 0)
		}
		return nil
	}

	p, err := resolvePaths()
	if err != nil {
		return err
	}
	defer setupLog(p.log)()
	log.Printf("%s %s starting", appName, version)

	a := &App{version: version, paths: p, inst: win32.ModuleHandle()}
	if err := a.load(); err != nil {
		return err
	}
	if err := a.createWindows(); err != nil {
		return err
	}
	defer a.shutdown()

	a.tray.add(a.controller)
	a.registerHotkeys()
	win32.SetTimer(a.controller, busyTimer, busyInterval)
	a.watch = watchFolder(a.store.Dir())
	if !a.watch.active() {
		win32.SetTimer(a.controller, changesTimer, changesInterval)
	}
	a.syncDeck()
	a.deck.show()
	// A short peek on launch shows the user where the deck lives.
	a.deck.peek()

	a.messageLoop()
	return nil
}

// load reads the config and the notes from disk.
func (a *App) load() error {
	_, statErr := os.Stat(a.paths.config)
	firstRun := errors.Is(statErr, fs.ErrNotExist)

	cfg, err := config.Load(a.paths.config)
	if err != nil {
		// Keep going with defaults; the user's file is left untouched
		// until a setting is changed.
		log.Printf("config: %v", err)
	}
	a.cfg = cfg

	dir := a.paths.notes
	if cfg.NotesDir != "" {
		dir = cfg.NotesDir
	}
	a.store, err = store.Open(dir)
	if err != nil {
		return err
	}
	a.notes, err = a.store.List()
	if err != nil {
		log.Printf("notes: %v", err)
	}

	if firstRun {
		a.saveConfig()
		if len(a.notes) == 0 {
			n := note.New(time.Now(), note.DefaultColor)
			n.Body = welcomeNote
			a.notes = append(a.notes, n)
			a.saveNote(n)
		}
	}
	return nil
}

func (a *App) createWindows() error {
	for class, cursor := range map[string]uintptr{
		controllerClass: win32.IDC_ARROW,
		deckClass:       win32.IDC_HAND,
		editorClass:     win32.IDC_ARROW,
		settingsClass:   win32.IDC_ARROW,
	} {
		if err := registerClass(a.inst, class, cursor); err != nil {
			return err
		}
	}

	// The controller is an invisible window that receives tray, hotkey and
	// system notifications.
	var err error
	if a.controller, err = createWindow(a, a.inst, win32.WS_EX_TOOLWINDOW, controllerClass, appName, win32.WS_POPUP); err != nil {
		return err
	}
	a.taskbarCreated = win32.RegisterWindowMessage("TaskbarCreated")
	a.text = newGDIText()

	if a.deck, err = newDeckWindow(a); err != nil {
		return err
	}
	if a.editor, err = newEditorWindow(a); err != nil {
		return err
	}
	return nil
}

// messageLoop pumps window messages until the app quits. Between messages it
// also waits for the notes folder to change.
func (a *App) messageLoop() {
	var msg win32.MSG
	for {
		var watched windows.Handle
		if a.watch.active() {
			watched = a.watch.handle
		}
		if win32.WaitForMessageOr(watched) {
			a.watch.rearm()
			// Wait for the burst of writes to settle before looking.
			win32.SetTimer(a.controller, changesTimer, changesDelay)
			continue
		}
		for win32.PeekMessage(&msg) {
			if msg.Message == win32.WM_QUIT {
				return
			}
			if a.preTranslate(&msg) {
				continue
			}
			win32.TranslateMessage(&msg)
			win32.DispatchMessage(&msg)
		}
	}
}

// preTranslate gives the editor first refusal on key presses aimed at it or
// its text control. It reports whether the message was consumed.
func (a *App) preTranslate(msg *win32.MSG) bool {
	if msg.Message != win32.WM_KEYDOWN {
		return false
	}
	if msg.HWnd != a.editor.edit && msg.HWnd != a.editor.hwnd {
		return false
	}
	return a.editor.handleKey(msg.WParam)
}

// quit ends the message loop; shutdown then saves and cleans up.
func (a *App) quit() {
	win32.DestroyWindow(a.controller)
}

func (a *App) shutdown() {
	a.editor.flush()
	a.tray.remove()
	a.watch.close()
	a.unregisterHotkeys()
	a.text.close()
	log.Printf("%s stopped", appName)
}

// registerHotkeys (re)registers the global shortcuts from the config. It
// returns a message for each shortcut that could not be registered.
func (a *App) registerHotkeys() (failures []string) {
	bindings := []struct {
		id       int32
		name     string
		shortcut string
	}{
		{hotkeyNewNote, "new_note", a.cfg.Hotkeys.NewNote},
		{hotkeyToggleDeck, "toggle_deck", a.cfg.Hotkeys.ToggleDeck},
	}
	for _, b := range bindings {
		win32.UnregisterHotKey(a.controller, b.id)
		hk, err := hotkey.Parse(b.shortcut)
		if err != nil {
			log.Printf("hotkey %s: %v", b.name, err)
			continue
		}
		if hk.IsZero() {
			continue
		}
		// Registration fails when another app owns the shortcut. That is
		// not fatal: everything stays reachable with the mouse.
		if err := win32.RegisterHotKey(a.controller, b.id, hk.Mods, hk.Key); err != nil {
			log.Printf("hotkey %s (%s): %v", b.name, b.shortcut, err)
			failures = append(failures, hotkey.Format(hk)+" is already taken by another app.")
		}
	}
	return failures
}

// unregisterHotkeys releases the global shortcuts.
func (a *App) unregisterHotkeys() {
	win32.UnregisterHotKey(a.controller, hotkeyNewNote)
	win32.UnregisterHotKey(a.controller, hotkeyToggleDeck)
}

// wndProc handles the controller window's messages.
func (a *App) wndProc(hwnd win32.HWND, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case msgTray:
		switch uint32(win32.LoWord(lp)) {
		case win32.WM_LBUTTONUP:
			a.deck.peek()
		case win32.WM_LBUTTONDBLCLK:
			a.newNote()
		case win32.WM_RBUTTONUP, win32.WM_CONTEXTMENU:
			at, _ := win32.GetCursorPos()
			a.showMainMenu(at)
		}
		return 0

	case msgWake:
		a.deck.peek()
		return 0

	case win32.WM_HOTKEY:
		switch int32(wp) {
		case hotkeyNewNote:
			a.newNote()
		case hotkeyToggleDeck:
			a.toggleDeck()
		}
		return 0

	case win32.WM_TIMER:
		switch wp {
		case busyTimer:
			// Never cover a full-screen game, video or presentation.
			a.deck.setSuppressed(win32.UserIsBusy() && !a.editor.shown)
		case changesTimer:
			if a.watch.active() {
				win32.KillTimer(hwnd, changesTimer)
			}
			a.pickUpChanges()
		}
		return 0

	case win32.WM_ENDSESSION:
		// The user is signing out or shutting down: the process may be
		// terminated as soon as this returns.
		if wp != 0 {
			a.editor.flush()
		}
		return 0

	case win32.WM_SETTINGCHANGE:
		// The light/dark app mode may have changed.
		if a.editor != nil {
			a.editor.applyTheme()
		}
		a.refreshSettings()
		return 0

	case win32.WM_DESTROY:
		win32.PostQuitMessage(0)
		return 0
	}
	if msg == a.taskbarCreated && a.taskbarCreated != 0 {
		a.tray.add(a.controller)
		return 0
	}
	return win32.DefWindowProc(hwnd, msg, wp, lp)
}

// dark reports whether the editor should use the dark theme.
func (a *App) dark() bool {
	switch a.cfg.Theme {
	case config.ThemeDark:
		return true
	case config.ThemeLight:
		return false
	default:
		return systemUsesDarkMode()
	}
}

// find returns the note with the given ID, or nil.
func (a *App) find(id string) *note.Note {
	for _, n := range a.notes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// active returns the notes shown in the deck, i.e. those not archived.
func (a *App) active() []*note.Note {
	var out []*note.Note
	for _, n := range a.notes {
		if !n.Archived {
			out = append(out, n)
		}
	}
	return out
}

// syncDeck pushes the current notes to the deck and repaints it.
func (a *App) syncDeck() {
	notes := a.active()
	items := make([]deck.Item, len(notes))
	for i, n := range notes {
		items[i] = deck.Item{ID: n.ID, Title: n.Title(), Color: n.Color}
	}
	a.deck.model.SetItems(items)
	if a.deck.visible() {
		a.deck.refresh()
	}
}

// updateHold keeps the deck expanded while a note is being edited next to it
// or one of its menus is open.
func (a *App) updateHold() {
	editing := a.editor.shown && !a.editor.pinned
	a.deck.model.SetHeld(editing || a.menuHoldsDeck)
	active := ""
	if a.editor.shown && a.editor.note != nil {
		active = a.editor.note.ID
	}
	a.deck.model.SetActive(active)
	a.deck.wake()
}

// saveNote writes a note to disk, telling the user if that fails.
func (a *App) saveNote(n *note.Note) {
	conflict, err := a.store.Save(n)
	if conflict != "" {
		// The note was also changed elsewhere. That version now lives on as
		// a note of its own; bring it into the deck.
		log.Printf("save: %s was changed elsewhere; kept that version as %q", n.ID, conflict)
		win32.SetTimer(a.controller, changesTimer, 0)
	}
	if err == nil {
		a.saveFailed = false
		return
	}
	log.Printf("save: %v", err)
	if !a.saveFailed {
		a.saveFailed = true
		win32.MessageBox(0, fmt.Sprintf("%s could not save your note.\n\n%v", appName, err), appName,
			win32.MB_OK|win32.MB_ICONERROR)
	}
}

func (a *App) saveConfig() {
	if err := a.cfg.Save(a.paths.config); err != nil {
		log.Printf("config: %v", err)
	}
}

// newNote creates a note and opens it for editing. The note is only written
// to disk once it has content.
func (a *App) newNote() {
	color := note.DefaultColor
	if notes := a.active(); len(notes) > 0 {
		// Rotate colours so neighbouring notes are easy to tell apart.
		color = notes[len(notes)-1].Color.Next()
	}
	n := note.New(time.Now(), color)
	a.notes = append(a.notes, n)
	a.deck.setSuppressed(false)
	a.deck.show()
	a.syncDeck()
	a.openNote(n.ID)
}

// openNote opens a note in the editor, next to its tab.
func (a *App) openNote(id string) {
	n := a.find(id)
	if n == nil {
		return
	}
	if n.Archived {
		a.setArchived(id, false)
	}
	scale := a.deck.model.Scale()
	a.editor.open(n, a.editorRect(id, scale), scale)
	a.updateHold()
}

// toggleNote opens a note, or closes it if it is the one already open.
func (a *App) toggleNote(id string) {
	if a.editor.showing(id) {
		a.editor.hide()
		return
	}
	a.openNote(id)
}

// stepNote switches the editor to the next (+1) or previous (-1) note.
func (a *App) stepNote(delta int) {
	notes := a.active()
	if a.editor.note == nil || len(notes) < 2 {
		return
	}
	i := slices.Index(notes, a.editor.note)
	if i < 0 {
		return
	}
	next := notes[(i+delta+len(notes))%len(notes)]
	// Read the ID first: opening the next note may discard the current one
	// if it is empty.
	a.openNote(next.ID)
}

// editorRect computes where the editor should appear for a note: beside the
// note's tab, on the inner side of the deck, kept within the work area.
func (a *App) editorRect(id string, scale float64) win32.RECT {
	work := primaryDisplay().work
	margin := int32(math.Round(screenMargin * scale))
	gap := int32(math.Round(editorGap * scale))

	w := min(int32(math.Round(float64(a.cfg.EditorWidth)*scale)), work.Width()-2*margin)
	h := min(int32(math.Round(float64(a.cfg.EditorHeight)*scale)), work.Height()-2*margin)

	anchor := a.deck.anchor(id)
	x := anchor.Left - gap - w
	if a.cfg.Edge == config.EdgeLeft {
		x = anchor.Right + gap
	}
	x = max(work.Left+margin, min(x, work.Right-margin-w))
	y := max(work.Top+margin, min(anchor.Top, work.Bottom-margin-h))
	return win32.RECT{Left: x, Top: y, Right: x + w, Bottom: y + h}
}

// pickUpChanges brings in notes that other programs added, changed or
// deleted, for example a sync client delivering edits from another computer.
func (a *App) pickUpChanges() {
	// Pending edits are saved first. Should the open note have changed on
	// disk too, the save keeps the other version as a separate note, which
	// the scan below then picks up: neither side's words are lost.
	a.editor.flush()

	changes, err := a.store.Changes()
	if err != nil {
		log.Printf("changes: %v", err)
	}
	if len(changes) == 0 {
		return
	}
	for _, c := range changes {
		current := a.find(c.ID)
		switch {
		case c.Note == nil:
			// Deleted elsewhere.
			if current == nil {
				continue
			}
			if a.editor.showing(c.ID) {
				a.editor.hide()
			}
			a.forget(current)
		case current == nil:
			a.notes = append(a.notes, c.Note)
		default:
			// Update in place: the editor and the deck hold this pointer.
			*current = *c.Note
			if a.editor.showing(c.ID) {
				if current.Archived {
					a.editor.hide()
				} else {
					a.editor.reload()
				}
			}
		}
	}
	slices.SortStableFunc(a.notes, func(x, y *note.Note) int { return x.Created.Compare(y.Created) })
	a.syncDeck()
}

// noteClosed runs after the editor lets go of a note. Notes that were never
// given any content are discarded.
func (a *App) noteClosed(n *note.Note) {
	if n.Empty() {
		a.forget(n)
		if err := a.store.Remove(n.ID); err != nil {
			log.Printf("remove: %v", err)
		}
	}
	a.syncDeck()
	a.updateHold()
}

// noteEdited runs after the editor saved changes, to refresh the tab title.
func (a *App) noteEdited() {
	a.syncDeck()
}

// forget drops a note from memory without touching the disk.
func (a *App) forget(n *note.Note) {
	a.notes = slices.DeleteFunc(a.notes, func(other *note.Note) bool { return other == n })
}

// deleteNote moves a note to the trash folder.
func (a *App) deleteNote(id string) {
	n := a.find(id)
	if n == nil {
		return
	}
	if a.editor.showing(id) {
		a.editor.hide()
		// Closing the editor already discarded the note if it was empty.
		if a.find(id) == nil {
			return
		}
	}
	if err := a.store.Trash(id); err != nil {
		log.Printf("delete: %v", err)
		return
	}
	a.lastTrashed = id
	a.forget(n)
	a.syncDeck()
}

// restoreDeleted brings back the most recently deleted note.
func (a *App) restoreDeleted() {
	if a.lastTrashed == "" {
		return
	}
	n, err := a.store.Restore(a.lastTrashed)
	a.lastTrashed = ""
	if err != nil {
		log.Printf("restore: %v", err)
		return
	}
	a.notes = append(a.notes, n)
	slices.SortStableFunc(a.notes, func(x, y *note.Note) int { return x.Created.Compare(y.Created) })
	a.syncDeck()
	a.deck.peek()
}

// setArchived hides a note from the deck or returns it there.
func (a *App) setArchived(id string, archived bool) {
	n := a.find(id)
	if n == nil || n.Archived == archived {
		return
	}
	if archived && a.editor.showing(id) {
		a.editor.hide()
		if a.find(id) == nil {
			return
		}
	}
	n.Archived = archived
	a.saveNote(n)
	a.syncDeck()
	if !archived {
		a.deck.peek()
	}
}

// setColor changes a note's colour.
func (a *App) setColor(id string, c note.Color) {
	n := a.find(id)
	if n == nil || n.Color == c {
		return
	}
	n.Color = c
	// An empty note has no file yet; its colour is saved with its first
	// content.
	if !n.Empty() {
		a.saveNote(n)
	}
	if a.editor.showing(id) {
		a.editor.applyTheme()
	}
	a.syncDeck()
}

// cycleColor advances the open note to the next colour.
func (a *App) cycleColor() {
	if n := a.editor.note; n != nil {
		a.setColor(n.ID, n.Color.Next())
	}
}

// toggleDeck hides the deck entirely, or brings it back.
func (a *App) toggleDeck() {
	if a.deck.hidden {
		a.deck.peek()
		return
	}
	a.editor.hide()
	a.deck.hide()
}

func (a *App) setEdge(e config.Edge) {
	a.cfg.Edge = e
	a.saveConfig()
	a.editor.hide()
	a.deck.refresh()
	a.deck.peek()
	a.refreshSettings()
}

func (a *App) setPosition(p float64) {
	a.cfg.Position = p
	a.saveConfig()
	a.editor.hide()
	a.deck.refresh()
	a.deck.peek()
	a.refreshSettings()
}

func (a *App) setTheme(t config.Theme) {
	a.cfg.Theme = t
	a.saveConfig()
	a.editor.applyTheme()
	a.refreshSettings()
}

// popup shows a menu while keeping the deck and the editor open.
func (a *App) popup(m *menu, at win32.POINT) {
	a.menuOpen = true
	a.menuHoldsDeck = a.deck.model.Open()
	win32.SetDarkMenus(a.dark())
	a.updateHold()
	// A popup menu only closes on an outside click if its owner is the
	// foreground window.
	win32.SetForegroundWindow(a.controller)
	m.show(a.controller, at)
	a.menuOpen, a.menuHoldsDeck = false, false

	if a.editor.shown {
		win32.SetForegroundWindow(a.editor.hwnd)
	}
	a.updateHold()
}

// showNoteMenu shows the context menu of a single note.
func (a *App) showNoteMenu(id string, at win32.POINT) {
	n := a.find(id)
	if n == nil {
		return
	}
	m := newMenu()
	m.item("Open", func() { a.openNote(id) })
	m.separator()
	dot := int(win32.GetSystemMetrics(win32.SM_CXSMICON))
	for _, c := range note.Colors {
		m.item(c.Label(), func() { a.setColor(id, c) })
		m.image(render.ColorDot(dot, c, n.Color == c, a.dark()))
	}
	m.separator()
	m.item("Archive", func() { a.setArchived(id, true) })
	m.item("Delete\tCtrl+Shift+Backspace", func() { a.deleteNote(id) })
	a.popup(m, at)
}

// showMainMenu shows the app menu, reached from the tray icon or by
// right-clicking the deck.
func (a *App) showMainMenu(at win32.POINT) {
	m := newMenu()
	m.item("New note\t"+a.cfg.Hotkeys.NewNote, a.newNote)
	if a.deck.hidden {
		m.item("Show deck\t"+a.cfg.Hotkeys.ToggleDeck, a.toggleDeck)
	} else {
		m.item("Hide deck\t"+a.cfg.Hotkeys.ToggleDeck, a.toggleDeck)
	}
	m.separator()

	archived := m.submenu("Archived notes")
	count := 0
	for _, n := range a.notes {
		if !n.Archived {
			continue
		}
		count++
		title := n.Title()
		if title == "" {
			title = "Untitled"
		}
		archived.item(menuLabel(title), func() { a.setArchived(n.ID, false) })
	}
	if count == 0 {
		archived.disabled("No archived notes")
	}
	if a.lastTrashed != "" {
		m.item("Restore deleted note", a.restoreDeleted)
	}
	m.item("Open notes folder", func() { win32.ShellOpen(a.store.Dir()) })
	m.separator()
	m.item("Settings…", a.showSettings)
	m.separator()
	m.disabled(appName + " " + a.version)
	m.item("Quit", a.quit)
	a.popup(m, at)
}

// menuLabel prepares arbitrary text for use as a menu item: it is shortened
// and its ampersands are escaped so they are not read as accelerator marks.
func menuLabel(s string) string {
	const limit = 40
	if r := []rune(s); len(r) > limit {
		s = string(r[:limit]) + "…"
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '&' {
			out = append(out, '&')
		}
		if r == '\t' {
			r = ' '
		}
		out = append(out, r)
	}
	return string(out)
}

// Compile-time check that the renderer accepts the GDI text drawer.
var _ render.TextDrawer = (*gdiText)(nil)
