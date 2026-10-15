package win32

import (
	"fmt"
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gdi32 = windows.NewLazySystemDLL("gdi32.dll")

	procBitBlt             = gdi32.NewProc("BitBlt")
	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procCreateFontW        = gdi32.NewProc("CreateFontW")
	procCreateSolidBrush   = gdi32.NewProc("CreateSolidBrush")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procGdiFlush           = gdi32.NewProc("GdiFlush")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procSetBkColor         = gdi32.NewProc("SetBkColor")
	procSetBkMode          = gdi32.NewProc("SetBkMode")
	procSetTextColor       = gdi32.NewProc("SetTextColor")
)

func SelectObject(dc HDC, obj HGDIOBJ) HGDIOBJ {
	r, _, _ := procSelectObject.Call(uintptr(dc), uintptr(obj))
	return HGDIOBJ(r)
}

func DeleteObject(obj HGDIOBJ) {
	procDeleteObject.Call(uintptr(obj))
}

func SetTextColor(dc HDC, c COLORREF) {
	procSetTextColor.Call(uintptr(dc), uintptr(c))
}

func SetBkColor(dc HDC, c COLORREF) {
	procSetBkColor.Call(uintptr(dc), uintptr(c))
}

func SetBkMode(dc HDC, mode int32) {
	procSetBkMode.Call(uintptr(dc), uintptr(mode))
}

func CreateSolidBrush(c COLORREF) HBRUSH {
	r, _, _ := procCreateSolidBrush.Call(uintptr(c))
	return HBRUSH(r)
}

// CreateFont creates a font with the given pixel height (matched against the
// character height, not the cell height), weight, rendering quality and face.
func CreateFont(pixelHeight int32, weight int32, quality uint32, face string) HFONT {
	r, _, _ := procCreateFontW.Call(
		uintptr(-pixelHeight), 0, 0, 0,
		uintptr(weight),
		0, 0, 0,
		DEFAULT_CHARSET,
		0, 0,
		uintptr(quality),
		0,
		uintptr(unsafe.Pointer(UTF16(face))),
	)
	return HFONT(r)
}

// CreateMonoBitmap creates a zero-filled 1-bit bitmap, as needed for the mask
// of an alpha-blended icon.
func CreateMonoBitmap(width, height int32) HBITMAP {
	bits := make([]byte, ((int(width)+15)/16*2)*int(height))
	r, _, _ := procCreateBitmap.Call(uintptr(width), uintptr(height), 1, 1, uintptr(unsafe.Pointer(&bits[0])))
	return HBITMAP(r)
}

func BitBlt(dst HDC, x, y, w, h int32, src HDC, srcX, srcY int32) {
	procBitBlt.Call(uintptr(dst), uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(src), uintptr(srcX), uintptr(srcY), SRCCOPY)
}

// DIB is a top-down, 32-bit device-independent bitmap selected into its own
// memory device context. Its pixels are directly addressable, which makes it
// the bridge between images rendered in Go and GDI.
type DIB struct {
	// DC is a memory device context with the bitmap selected into it.
	DC HDC
	// Bitmap is the underlying GDI bitmap.
	Bitmap HBITMAP
	// Pix holds the pixels as BGRA, four bytes per pixel, top row first.
	// The memory is owned by GDI and becomes invalid after Close.
	Pix []byte
	// Width and Height are the bitmap's size in pixels.
	Width, Height int

	previous HGDIOBJ
}

// NewDIB creates a DIB of the given size.
func NewDIB(width, height int) (*DIB, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("NewDIB: invalid size %dx%d", width, height)
	}
	dc, _, err := procCreateCompatibleDC.Call(0)
	if dc == 0 {
		return nil, callErr("CreateCompatibleDC", err)
	}

	info := BITMAPINFO{Header: BITMAPINFOHEADER{
		Width:       int32(width),
		Height:      -int32(height), // negative height means top-down rows
		Planes:      1,
		BitCount:    32,
		Compression: BI_RGB,
	}}
	info.Header.Size = uint32(unsafe.Sizeof(info.Header))

	// GDI allocates the pixel memory and reports its address through bits.
	var bits unsafe.Pointer
	bmp, _, err := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&info)), DIB_RGB_COLORS,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		procDeleteDC.Call(dc)
		return nil, callErr("CreateDIBSection", err)
	}

	d := &DIB{
		DC:     HDC(dc),
		Bitmap: HBITMAP(bmp),
		Pix:    unsafe.Slice((*byte)(bits), width*height*4),
		Width:  width,
		Height: height,
	}
	d.previous = SelectObject(d.DC, HGDIOBJ(d.Bitmap))
	return d, nil
}

// Close releases the bitmap and its device context.
func (d *DIB) Close() {
	if d == nil || d.DC == 0 {
		return
	}
	SelectObject(d.DC, d.previous)
	DeleteObject(HGDIOBJ(d.Bitmap))
	procDeleteDC.Call(uintptr(d.DC))
	d.DC, d.Bitmap, d.Pix = 0, 0, nil
}

// Detach releases the device context but keeps the bitmap alive, returning
// its handle. The caller becomes responsible for deleting the bitmap.
func (d *DIB) Detach() HBITMAP {
	bmp := d.Bitmap
	SelectObject(d.DC, d.previous)
	procDeleteDC.Call(uintptr(d.DC))
	d.DC, d.Bitmap, d.Pix = 0, 0, nil
	return bmp
}

// CopyFrom copies an image of the same size into the bitmap, converting from
// Go's RGBA byte order to GDI's BGRA. Both use premultiplied alpha.
func (d *DIB) CopyFrom(img *image.RGBA) {
	b := img.Bounds()
	w, h := min(b.Dx(), d.Width), min(b.Dy(), d.Height)
	for y := 0; y < h; y++ {
		src := img.Pix[y*img.Stride : y*img.Stride+w*4]
		dst := d.Pix[y*d.Width*4 : y*d.Width*4+w*4]
		for i := 0; i+3 < len(src); i += 4 {
			dst[i+0] = src[i+2]
			dst[i+1] = src[i+1]
			dst[i+2] = src[i+0]
			dst[i+3] = src[i+3]
		}
	}
}

// Flush waits for pending GDI drawing to finish so that Pix is up to date.
func Flush() {
	procGdiFlush.Call()
}
