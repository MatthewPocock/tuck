package win32

// Handle types. They are distinct types so the compiler catches a window
// handle being passed where a device context is expected.
type (
	HWND      uintptr
	HDC       uintptr
	HMENU     uintptr
	HICON     uintptr
	HCURSOR   uintptr
	HFONT     uintptr
	HBITMAP   uintptr
	HBRUSH    uintptr
	HGDIOBJ   uintptr
	HMONITOR  uintptr
	HINSTANCE uintptr
)

// COLORREF is a GDI colour in 0x00BBGGRR form.
type COLORREF uint32

// RGB builds a COLORREF from its components.
func RGB(r, g, b uint8) COLORREF {
	return COLORREF(r) | COLORREF(g)<<8 | COLORREF(b)<<16
}

// POINT is a point in pixels.
type POINT struct {
	X, Y int32
}

// SIZE is a width and height in pixels.
type SIZE struct {
	CX, CY int32
}

// MENUITEMINFO is the Win32 MENUITEMINFOW structure.
type MENUITEMINFO struct {
	Size            uint32
	Mask            uint32
	Type            uint32
	State           uint32
	ID              uint32
	SubMenu         HMENU
	BitmapChecked   HBITMAP
	BitmapUnchecked HBITMAP
	ItemData        uintptr
	TypeData        *uint16
	Cch             uint32
	BitmapItem      HBITMAP
}

// MARGINS is the Win32 MARGINS structure.
type MARGINS struct {
	Left, Right, Top, Bottom int32
}

// RECT is a rectangle in pixels; Right and Bottom are exclusive.
type RECT struct {
	Left, Top, Right, Bottom int32
}

// Width returns the rectangle's width.
func (r RECT) Width() int32 { return r.Right - r.Left }

// Height returns the rectangle's height.
func (r RECT) Height() int32 { return r.Bottom - r.Top }

// Contains reports whether the point lies inside the rectangle.
func (r RECT) Contains(p POINT) bool {
	return p.X >= r.Left && p.X < r.Right && p.Y >= r.Top && p.Y < r.Bottom
}

// MSG is a window message as retrieved by GetMessage.
type MSG struct {
	HWnd    HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

// WNDCLASSEX describes a window class for RegisterClassEx.
type WNDCLASSEX struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   HINSTANCE
	Icon       HICON
	Cursor     HCURSOR
	Background HBRUSH
	MenuName   *uint16
	ClassName  *uint16
	IconSm     HICON
}

// BLENDFUNCTION controls alpha blending in UpdateLayeredWindow.
type BLENDFUNCTION struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

// BITMAPINFOHEADER describes the format of a device-independent bitmap.
type BITMAPINFOHEADER struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// BITMAPINFO is a BITMAPINFOHEADER followed by a (here unused) colour table.
type BITMAPINFO struct {
	Header BITMAPINFOHEADER
	Colors [1]uint32
}

// MONITORINFO describes a display monitor.
type MONITORINFO struct {
	Size    uint32
	Monitor RECT
	Work    RECT
	Flags   uint32
}

// PAINTSTRUCT holds painting state between BeginPaint and EndPaint.
type PAINTSTRUCT struct {
	DC        HDC
	Erase     int32
	Paint     RECT
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

// TRACKMOUSEEVENT configures TrackMouseEvent.
type TRACKMOUSEEVENT struct {
	Size      uint32
	Flags     uint32
	Track     HWND
	HoverTime uint32
}

// MINMAXINFO carries a window's size limits in WM_GETMINMAXINFO.
type MINMAXINFO struct {
	Reserved     POINT
	MaxSize      POINT
	MaxPosition  POINT
	MinTrackSize POINT
	MaxTrackSize POINT
}

// ICONINFO describes an icon for CreateIconIndirect.
type ICONINFO struct {
	IsIcon   int32
	XHotspot uint32
	YHotspot uint32
	Mask     HBITMAP
	Color    HBITMAP
}

// GUID is a Windows globally unique identifier.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// NOTIFYICONDATA describes a notification area ("tray") icon.
type NOTIFYICONDATA struct {
	Size            uint32
	HWnd            HWND
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            HICON
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        GUID
	BalloonIcon     HICON
}

// CHARFORMAT describes character formatting in a rich edit control.
type CHARFORMAT struct {
	Size           uint32
	Mask           uint32
	Effects        uint32
	Height         int32
	Offset         int32
	TextColor      COLORREF
	CharSet        byte
	PitchAndFamily byte
	FaceName       [32]uint16
}

// CHARRANGE is a range of characters in a rich edit control.
type CHARRANGE struct {
	Min, Max int32
}

// LoWord returns the low 16 bits of a message parameter.
func LoWord(v uintptr) uint16 { return uint16(v) }

// HiWord returns bits 16–31 of a message parameter.
func HiWord(v uintptr) uint16 { return uint16(v >> 16) }

// PointFromLParam decodes the signed client coordinates packed into the
// LPARAM of mouse messages.
func PointFromLParam(lp uintptr) POINT {
	return POINT{X: int32(int16(LoWord(lp))), Y: int32(int16(HiWord(lp)))}
}
