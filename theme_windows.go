package main

import (
	"syscall"
	"unsafe"
)

// The palette follows the macOS dark appearance while controls retain Win32 input handling.
const (
	colorWindow   = 0x242c30
	colorPanel    = 0x2c3438
	colorField    = 0x242c30
	colorButton   = 0x3b4347
	colorStripe   = 0x30383c
	colorBorder   = 0x485155
	colorText     = 0xe3e5e6
	colorMuted    = 0xa8adaf
	colorDisabled = 0x737b7f
	colorAccent   = 0x0085ff
)

type drawItem struct {
	Kind, ID, Item, Action, State uint32
	Window, DC                    uintptr
	Rect                          winRect
	Data                          uintptr
}
type measureItem struct {
	Kind, ID, Item, Width, Height uint32
	Data                          uintptr
}
type paintInfo struct {
	DC              uintptr
	Erase           int32
	Rect            winRect
	Restore, Update int32
	Reserved        [32]byte
}

var themedControlCallback = syscall.NewCallback(themedControlProc)

func rgb(c uint32) uintptr { return uintptr((c&0xff)<<16 | (c & 0xff00) | c>>16) }
func fillColor(dc uintptr, rect winRect, color uint32) {
	brush, _, _ := gdi32.NewProc("GetStockObject").Call(18) // DC_BRUSH
	gdi32.NewProc("SetDCBrushColor").Call(dc, rgb(color))
	user32.NewProc("FillRect").Call(dc, uintptr(unsafe.Pointer(&rect)), brush)
}
func roundedBox(dc uintptr, r winRect, fill, border uint32, radius uintptr) {
	brush, _, _ := gdi32.NewProc("GetStockObject").Call(18)
	pen, _, _ := gdi32.NewProc("GetStockObject").Call(19) // DC_PEN
	oldBrush, _, _ := gdi32.NewProc("SelectObject").Call(dc, brush)
	oldPen, _, _ := gdi32.NewProc("SelectObject").Call(dc, pen)
	gdi32.NewProc("SetDCBrushColor").Call(dc, rgb(fill))
	gdi32.NewProc("SetDCPenColor").Call(dc, rgb(border))
	gdi32.NewProc("RoundRect").Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), radius, radius)
	gdi32.NewProc("SelectObject").Call(dc, oldPen)
	gdi32.NewProc("SelectObject").Call(dc, oldBrush)
}
func drawText(dc uintptr, text string, r winRect, color uint32, font, flags uintptr) {
	gdi32.NewProc("SetBkMode").Call(dc, 1)
	gdi32.NewProc("SetTextColor").Call(dc, rgb(color))
	old, _, _ := gdi32.NewProc("SelectObject").Call(dc, font)
	user32.NewProc("DrawTextW").Call(dc, uintptr(unsafe.Pointer(wide(text))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags|0x800) // DT_NOPREFIX
	gdi32.NewProc("SelectObject").Call(dc, old)
}
func windowText(h uintptr) string {
	n, _, _ := user32.NewProc("GetWindowTextLengthW").Call(h)
	b := make([]uint16, int(n)+1)
	user32.NewProc("GetWindowTextW").Call(h, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	return syscall.UTF16ToString(b)
}
func (w *windowsUI) initTheme() {
	w.backgroundBrush, _, _ = gdi32.NewProc("CreateSolidBrush").Call(rgb(colorWindow))
	w.panelBrush, _, _ = gdi32.NewProc("CreateSolidBrush").Call(rgb(colorPanel))
	w.fieldBrush, _, _ = gdi32.NewProc("CreateSolidBrush").Call(rgb(colorField))
	height := int32(-16 * w.scale)
	w.boldFont, _, _ = gdi32.NewProc("CreateFontW").Call(uintptr(height), 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(wide("Segoe UI"))))
}
func (w *windowsUI) darkTitleBar() {
	// Older Windows and Wine may not implement this optional DWM attribute.
	p := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	if p.Find() == nil {
		enabled := int32(1)
		p.Call(w.root, 20, uintptr(unsafe.Pointer(&enabled)), 4)
	}
}
func (w *windowsUI) styleControl(id int, kind string) {
	h := w.controls[id]
	if kind == "heading" {
		msg(h, 0x30, w.boldFont, 1)
	}
	if kind == "combo" {
		msg(h, 0x153, ^uintptr(0), w.px(24)) // CB_SETITEMHEIGHT, selection field
		msg(h, 0x153, 0, w.px(24))
	}
	if kind == "entry" || kind == "list" {
		p := syscall.NewLazyDLL("uxtheme.dll").NewProc("SetWindowTheme")
		if p.Find() == nil {
			p.Call(h, uintptr(unsafe.Pointer(wide(""))), uintptr(unsafe.Pointer(wide(""))))
		}
	}
	if kind == "combo" || kind == "toggle" || kind == "entry" || kind == "list" {
		comctl32.NewProc("SetWindowSubclass").Call(h, themedControlCallback, 1, uintptr(id))
	}
}
func (w *windowsUI) themeMessage(h uintptr, m uint32, wp, lp uintptr) (uintptr, bool) {
	switch m {
	case 0x14: // WM_ERASEBKGND
		var rect winRect
		user32.NewProc("GetClientRect").Call(h, uintptr(unsafe.Pointer(&rect)))
		color := uint32(colorPanel)
		if h == w.root {
			color = colorWindow
		}
		fillColor(wp, rect, color)
		if h == w.root {
			panel := winRect{int32(w.px(15)), int32(w.px(126)), int32(w.px(1115)), int32(w.px(834))}
			roundedBox(wp, panel, colorPanel, colorBorder, w.px(10))
		}
		return 1, true
	case 0x132, 0x133, 0x134, 0x135, 0x138: // WM_CTLCOLOR*
		id, _, _ := user32.NewProc("GetDlgCtrlID").Call(lp)
		kind := w.kinds[int(id)]
		color, text, brush := uint32(colorPanel), uint32(colorMuted), w.panelBrush
		if h == w.root {
			color, brush = colorWindow, w.backgroundBrush
		}
		if kind == "entry" || kind == "list" || m == 0x134 {
			color, text, brush = colorField, colorText, w.fieldBrush
		}
		if kind == "heading" || kind == "toggle" {
			text = colorText
		}
		enabled, _, _ := user32.NewProc("IsWindowEnabled").Call(lp)
		if enabled == 0 {
			text = colorDisabled
		}
		gdi32.NewProc("SetTextColor").Call(wp, rgb(text))
		gdi32.NewProc("SetBkColor").Call(wp, rgb(color))
		return brush, true
	case 0x2c: // WM_MEASUREITEM
		if lp != 0 {
			item := (*measureItem)(unsafe.Pointer(lp))
			if item.Kind == 2 || item.Kind == 3 {
				item.Height = uint32(w.px(24))
				return 1, true
			}
		}
	case 0x2b: // WM_DRAWITEM
		if lp != 0 {
			w.drawItem((*drawItem)(unsafe.Pointer(lp)))
			return 1, true
		}
	}
	return 0, false
}
func (w *windowsUI) drawItem(item *drawItem) {
	dc, rect := item.DC, item.Rect
	saved, _, _ := gdi32.NewProc("SaveDC").Call(dc)
	defer gdi32.NewProc("RestoreDC").Call(dc, saved)
	textColor, background := uint32(colorText), uint32(colorField)
	if item.State&4 != 0 {
		textColor = colorDisabled
	} // ODS_DISABLED
	if item.Kind == 4 { // ODT_BUTTON
		parent, _, _ := user32.NewProc("GetParent").Call(item.Window)
		base := uint32(colorPanel)
		if parent == w.root {
			base = colorWindow
		}
		fillColor(dc, rect, base)
		background = colorButton
		selectedTab := (item.ID == 200 || item.ID == 201) && int(item.ID)-200 == w.activePage
		if selectedTab {
			background = colorAccent
		}
		if item.State&1 != 0 {
			background = 0x255e88
		}
		if item.State&4 != 0 {
			background = colorStripe
		}
		border := background
		if item.State&0x10 != 0 {
			border = colorAccent
		}
		roundedBox(dc, rect, background, border, w.px(10))
		drawText(dc, windowText(item.Window), rect, textColor, w.font, 0x25) // CENTER | VCENTER | SINGLELINE
		if item.State&0x10 != 0 && item.State&0x200 == 0 {
			rect.Left += 3
			rect.Top += 3
			rect.Right -= 3
			rect.Bottom -= 3
			user32.NewProc("DrawFocusRect").Call(dc, uintptr(unsafe.Pointer(&rect)))
		}
		return
	}
	if item.Kind == 2 && item.Item%2 == 1 {
		background = colorStripe
	}
	if item.State&1 != 0 {
		background = 0x1766a6
	}
	fillColor(dc, rect, background)
	rows := w.rows[int(item.ID)]
	if int32(item.Item) >= 0 && uint64(item.Item) < uint64(len(rows)) {
		rect.Left += int32(w.px(7))
		flags := uintptr(0x24) // VCENTER | SINGLELINE
		if item.Kind == 3 {
			flags |= 0x8000
		} // DT_END_ELLIPSIS
		drawText(dc, rows[item.Item], rect, textColor, w.font, flags)
	}
	if item.State&0x10 != 0 && item.State&0x200 == 0 {
		user32.NewProc("DrawFocusRect").Call(dc, uintptr(unsafe.Pointer(&item.Rect)))
	}
}
func themedControlProc(h uintptr, m uint32, wp, lp, subclass, data uintptr) uintptr {
	id := int(data)
	kind := win.kinds[id]
	if (m == 0xf || m == 0x318) && (kind == "combo" || kind == "toggle") {
		var paint paintInfo
		dc := wp
		if m == 0xf {
			dc, _, _ = user32.NewProc("BeginPaint").Call(h, uintptr(unsafe.Pointer(&paint)))
		}
		win.paintControl(h, dc, id, kind)
		if m == 0xf {
			user32.NewProc("EndPaint").Call(h, uintptr(unsafe.Pointer(&paint)))
		}
		return 0
	}
	if m == 0x85 && (kind == "entry" || kind == "list") { // WM_NCPAINT
		// Preserve native scrollbars on lists. Paint only the thin outer frame.
		comctl32.NewProc("DefSubclassProc").Call(h, uintptr(m), wp, lp)
		dc, _, _ := user32.NewProc("GetWindowDC").Call(h)
		var r winRect
		user32.NewProc("GetWindowRect").Call(h, uintptr(unsafe.Pointer(&r)))
		r.Right -= r.Left
		r.Bottom -= r.Top
		r.Left = 0
		r.Top = 0
		gdi32.NewProc("ExcludeClipRect").Call(dc, 2, 2, uintptr(r.Right-2), uintptr(r.Bottom-2))
		focus, _, _ := user32.NewProc("GetFocus").Call()
		border := uint32(colorBorder)
		if focus == h {
			border = colorAccent
		}
		fillColor(dc, r, border)
		user32.NewProc("ReleaseDC").Call(h, dc)
		return 0
	}
	r, _, _ := comctl32.NewProc("DefSubclassProc").Call(h, uintptr(m), wp, lp)
	if m == 7 || m == 8 || m == 0xa || m == 0xf1 { // Focus, enabled or checked state changed.
		user32.NewProc("RedrawWindow").Call(h, 0, 0, 0x401)
	}
	return r
}
func (w *windowsUI) paintControl(h, dc uintptr, id int, kind string) {
	saved, _, _ := gdi32.NewProc("SaveDC").Call(dc)
	defer gdi32.NewProc("RestoreDC").Call(dc, saved)
	var rect winRect
	user32.NewProc("GetClientRect").Call(h, uintptr(unsafe.Pointer(&rect)))
	fillColor(dc, rect, colorPanel)
	enabled, _, _ := user32.NewProc("IsWindowEnabled").Call(h)
	focus, _, _ := user32.NewProc("GetFocus").Call()
	textColor, border := uint32(colorText), uint32(colorButton)
	if enabled == 0 {
		textColor = colorDisabled
	}
	if focus == h {
		border = colorAccent
	}
	if kind == "combo" {
		roundedBox(dc, rect, colorButton, border, w.px(10))
		selection := int(int32(msg(h, 0x147, 0, 0)))
		rows := w.rows[id]
		textRect := rect
		textRect.Left += int32(w.px(9))
		textRect.Right -= int32(w.px(26))
		if selection >= 0 && selection < len(rows) {
			drawText(dc, rows[selection], textRect, textColor, w.font, 0x8024)
		}
		arrow := rect
		arrow.Left = arrow.Right - int32(w.px(24))
		drawText(dc, "⌄", arrow, textColor, w.font, 0x25)
	} else {
		size := int32(w.px(18))
		box := winRect{0, (rect.Bottom - size) / 2, size, (rect.Bottom + size) / 2}
		background := uint32(colorField)
		checked := msg(h, 0xf0, 0, 0) == 1
		if checked {
			background, border = colorAccent, colorAccent
		}
		if enabled == 0 {
			background, border = colorStripe, colorBorder
		}
		roundedBox(dc, box, background, border, w.px(6))
		if checked {
			drawText(dc, "✓", box, textColor, w.font, 0x25)
		}
		rect.Left += int32(w.px(28))
		drawText(dc, windowText(h), rect, textColor, w.font, 0x24)
		if focus == h {
			user32.NewProc("DrawFocusRect").Call(dc, uintptr(unsafe.Pointer(&rect)))
		}
	}
}
