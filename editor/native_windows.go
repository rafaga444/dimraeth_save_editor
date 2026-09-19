package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var gdi32 = syscall.NewLazyDLL("gdi32.dll")
var comctl32 = syscall.NewLazyDLL("comctl32.dll")
var shell32 = syscall.NewLazyDLL("shell32.dll")
var ole32 = syscall.NewLazyDLL("ole32.dll")
var comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
var sendMessage = user32.NewProc("SendMessageW")
var win *windowsUI

type winClass struct {
	Size, Style                        uint32
	Procedure                          uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type winPoint struct{ X, Y int32 }
type winMsg struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          winPoint
	Private        uint32
}
type winRect struct{ Left, Top, Right, Bottom int32 }
type notifyHeader struct {
	From, ID uintptr
	Code     uint32
}
type tabItem struct {
	Mask, State, StateMask uint32
	Text                   *uint16
	TextMax, Image         int32
	Param                  uintptr
}
type openFileName struct {
	Size                         uint32
	Owner, Instance              uintptr
	Filter, CustomFilter         *uint16
	MaxCustomFilter, FilterIndex uint32
	File                         *uint16
	MaxFile                      uint32
	FileTitle                    *uint16
	MaxFileTitle                 uint32
	InitialDir, Title            *uint16
	Flags                        uint32
	FileOffset, FileExtension    uint16
	DefaultExtension             *uint16
	CustomData, Hook             uintptr
	TemplateName                 *uint16
	Reserved                     uintptr
	Reserved2, FlagsEx           uint32
}
type browseInfo struct {
	Owner, Root, DisplayName, Title uintptr
	Flags                           uint32
	Callback, Param                 uintptr
	Image                           int32
}
type windowsUI struct {
	root, tabs, font, instance uintptr
	pages                      [2]uintptr
	controls                   map[int]uintptr
	kinds                      map[int]string
	changing                   bool
	scale                      float64
}

func newDesktopUI() desktopUI {
	win = &windowsUI{controls: map[int]uintptr{}, kinds: map[int]string{}, scale: 1}
	return win
}
func wide(s string) *uint16 {
	p, e := syscall.UTF16PtrFromString(s)
	if e != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	return p
}
func msg(h uintptr, m uint32, w, l uintptr) uintptr {
	r, _, _ := sendMessage.Call(h, uintptr(m), w, l)
	return r
}
func (w *windowsUI) px(v int) uintptr { return uintptr(int(float64(v) * w.scale)) }
func (w *windowsUI) create(class, title string, style, ex, parent uintptr, id, x, y, width, height int) uintptr {
	h, _, e := user32.NewProc("CreateWindowExW").Call(ex, uintptr(unsafe.Pointer(wide(class))), uintptr(unsafe.Pointer(wide(title))), style, w.px(x), w.px(y), w.px(width), w.px(height), parent, uintptr(id), w.instance, 0)
	if h == 0 {
		panic(fmt.Sprintf("Could not create %s control: %v", class, e))
	}
	msg(h, 0x30, w.font, 1)
	return h
}
func windowProc(h uintptr, m uint32, wp, lp uintptr) uintptr {
	if win != nil {
		switch m {
		case 0x10:
			if h == win.root {
				desktop.event(quitID)
				return 0
			}
		case 0x2:
			if h == win.root {
				user32.NewProc("PostQuitMessage").Call(0)
				return 0
			}
		case 0x111:
			if !win.changing {
				id, code := int(wp&0xffff), int(wp>>16)
				kind := win.kinds[id]
				if (kind == "button" && code == 0) || ((kind == "combo" || kind == "list") && code == 1) || (kind == "entry" && code == 0x300) {
					desktop.event(id)
				}
			}
			return 0
		case 0x4e:
			if lp != 0 {
				n := (*notifyHeader)(unsafe.Pointer(lp))
				if n.From == win.tabs && int32(n.Code) == -551 {
					index := int(msg(win.tabs, 0x130b, 0, 0))
					for i, p := range win.pages {
						show := uintptr(0)
						if i == index {
							show = 5
						}
						user32.NewProc("ShowWindow").Call(p, show)
					}
					return 0
				}
			}
		}
	}
	r, _, _ := user32.NewProc("DefWindowProcW").Call(h, uintptr(m), wp, lp)
	return r
}
func (w *windowsUI) init() {
	// Use built-in common controls and the process's UI thread; no browser runtime.
	user32.NewProc("SetProcessDPIAware").Call()
	ole32.NewProc("CoInitializeEx").Call(0, 2)
	ic := struct{ Size, Classes uint32 }{8, 0x4008}
	comctl32.NewProc("InitCommonControlsEx").Call(uintptr(unsafe.Pointer(&ic)))
	w.instance, _, _ = kernel32.NewProc("GetModuleHandleW").Call(0)
	sw, _, _ := user32.NewProc("GetSystemMetrics").Call(0)
	sh, _, _ := user32.NewProc("GetSystemMetrics").Call(1)
	if sh < 980 {
		w.scale = float64(sh-85) / 895
	}
	if float64(sw-40)/1130 < w.scale {
		w.scale = float64(sw-40) / 1130
	}
	height := int32(-14 * w.scale)
	w.font, _, _ = gdi32.NewProc("CreateFontW").Call(uintptr(height), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(wide("Segoe UI"))))
	cursor, _, _ := user32.NewProc("LoadCursorW").Call(0, 32512)
	c := winClass{Style: 3, Procedure: syscall.NewCallback(windowProc), Instance: w.instance, Cursor: cursor, Background: 16, Name: wide("DimraethNativeWindow")}
	c.Size = uint32(unsafe.Sizeof(c))
	r, _, e := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		panic(e)
	}
	style := uintptr(0x00c00000 | 0x00080000 | 0x00020000 | 0x02000000)
	w.root = w.create("DimraethNativeWindow", "Dimraeth Editor", style, 0, 0, 0, 0, 0, 1130, 895)
	rect := winRect{Right: int32(w.px(1130)), Bottom: int32(w.px(895))}
	user32.NewProc("AdjustWindowRectEx").Call(uintptr(unsafe.Pointer(&rect)), style, 0, 0)
	width, height2 := int(rect.Right-rect.Left), int(rect.Bottom-rect.Top)
	user32.NewProc("SetWindowPos").Call(w.root, 0, uintptr((int(sw)-width)/2), uintptr((int(sh)-height2)/2), uintptr(width), uintptr(height2), 0x14)
	w.tabs = w.create("SysTabControl32", "", 0x50010000|0x04000000, 0, w.root, 200, 15, 112, 1100, 721)
	for i, title := range []string{"Save editor", "Loot patcher"} {
		item := tabItem{Mask: 1, Text: wide(title)}
		msg(w.tabs, 0x133e, uintptr(i), uintptr(unsafe.Pointer(&item)))
		w.pages[i] = w.create("DimraethNativeWindow", "", 0x40000000|0x04000000, 0x10000, w.root, 210+i, 20, 145, 1090, 686)
	}
	user32.NewProc("ShowWindow").Call(w.pages[0], 5)
}
func (w *windowsUI) add(kind string, id, page, x, y, width, height int, text string) {
	parent := w.root
	if page >= 0 {
		parent = w.pages[page]
	}
	class := "STATIC"
	style := uintptr(0x50000000)
	ex := uintptr(0)
	switch kind {
	case "button":
		class = "BUTTON"
		style |= 0x10000
	case "entry":
		class = "EDIT"
		style |= 0x10000 | 0x80
		ex = 0x200
	case "combo":
		class = "COMBOBOX"
		style |= 0x10000 | 0x00200000 | 3
		height = 300
	case "list":
		class = "LISTBOX"
		style |= 0x10000 | 0x00200000 | 0x00100000 | 1 | 0x100 | 0x800
		ex = 0x200
	case "label", "heading":
		style |= 0x80
	}
	w.controls[id] = w.create(class, text, style, ex, parent, id, x, y, width, height)
	w.kinds[id] = kind
}
func (w *windowsUI) text(id int) string {
	h := w.controls[id]
	n, _, _ := user32.NewProc("GetWindowTextLengthW").Call(h)
	b := make([]uint16, int(n)+1)
	user32.NewProc("GetWindowTextW").Call(h, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	return syscall.UTF16ToString(b)
}
func (w *windowsUI) setText(id int, text string) {
	old := w.changing
	w.changing = true
	defer func() { w.changing = old }()
	user32.NewProc("SetWindowTextW").Call(w.controls[id], uintptr(unsafe.Pointer(wide(text))))
	user32.NewProc("UpdateWindow").Call(w.controls[id])
}
func (w *windowsUI) options(id int, items []string) {
	old := w.changing
	w.changing = true
	defer func() { w.changing = old }()
	h := w.controls[id]
	reset, add := uint32(0x14b), uint32(0x143)
	if w.kinds[id] == "list" {
		reset, add = 0x184, 0x180
	}
	msg(h, 0xb, 0, 0)
	msg(h, reset, 0, 0)
	max := 0
	for _, item := range items {
		msg(h, add, 0, uintptr(unsafe.Pointer(wide(item))))
		if len(item) > max {
			max = len(item)
		}
	}
	if w.kinds[id] == "list" {
		msg(h, 0x194, w.px(max*8), 0)
	}
	msg(h, 0xb, 1, 0)
	user32.NewProc("InvalidateRect").Call(h, 0, 1)
}
func (w *windowsUI) selection(id int) int {
	code := uint32(0x147)
	if w.kinds[id] == "list" {
		code = 0x188
	}
	return int(int32(msg(w.controls[id], code, 0, 0)))
}
func (w *windowsUI) selectIndex(id, index int) {
	code := uint32(0x14e)
	if w.kinds[id] == "list" {
		code = 0x186
	}
	msg(w.controls[id], code, uintptr(index), 0)
}
func (w *windowsUI) enable(id int, enabled bool) {
	v := uintptr(0)
	if enabled {
		v = 1
	}
	user32.NewProc("EnableWindow").Call(w.controls[id], v)
}
func (w *windowsUI) show(id int, visible bool) {
	v := uintptr(0)
	if visible {
		v = 5
	}
	user32.NewProc("ShowWindow").Call(w.controls[id], v)
}
func (w *windowsUI) pick(folder bool) string {
	b := make([]uint16, 32768)
	if folder {
		display := make([]uint16, 260)
		bi := browseInfo{Owner: w.root, DisplayName: uintptr(unsafe.Pointer(&display[0])), Title: uintptr(unsafe.Pointer(wide("Select the game installation folder"))), Flags: 0x51}
		pidl, _, _ := shell32.NewProc("SHBrowseForFolderW").Call(uintptr(unsafe.Pointer(&bi)))
		runtime.KeepAlive(display)
		if pidl == 0 {
			return ""
		}
		defer ole32.NewProc("CoTaskMemFree").Call(pidl)
		r, _, _ := shell32.NewProc("SHGetPathFromIDListW").Call(pidl, uintptr(unsafe.Pointer(&b[0])))
		if r == 0 {
			return ""
		}
	} else {
		filter := utf16.Encode([]rune("Dimraeth saves (*.jrf;*.bak*)\x00*.jrf;*.bak*\x00All files (*.*)\x00*.*\x00\x00"))
		of := openFileName{Owner: w.root, Filter: &filter[0], FilterIndex: 1, File: &b[0], MaxFile: uint32(len(b)), Title: wide("Open Dimraeth save"), Flags: 0x00001000 | 0x00000800 | 0x00080000 | 0x8}
		of.Size = uint32(unsafe.Sizeof(of))
		r, _, _ := comdlg32.NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&of)))
		runtime.KeepAlive(filter)
		if r == 0 {
			return ""
		}
	}
	return syscall.UTF16ToString(b)
}
func (w *windowsUI) alert(title, message string) {
	user32.NewProc("MessageBoxW").Call(w.root, uintptr(unsafe.Pointer(wide(message))), uintptr(unsafe.Pointer(wide(title))), 0x10)
}
func (w *windowsUI) confirm(title, message string) bool {
	r, _, _ := user32.NewProc("MessageBoxW").Call(w.root, uintptr(unsafe.Pointer(wide(message))), uintptr(unsafe.Pointer(wide(title))), 0x4|0x30|0x100)
	return r == 6
}
func (w *windowsUI) run() {
	user32.NewProc("ShowWindow").Call(w.root, 5)
	user32.NewProc("UpdateWindow").Call(w.root)
	var m winMsg
	for {
		r, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		handled, _, _ := user32.NewProc("IsDialogMessageW").Call(w.root, uintptr(unsafe.Pointer(&m)))
		if handled == 0 {
			user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
			user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
		}
	}
	ole32.NewProc("CoUninitialize").Call()
}
func (w *windowsUI) stop() { user32.NewProc("DestroyWindow").Call(w.root) }
