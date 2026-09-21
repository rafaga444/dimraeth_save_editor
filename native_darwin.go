//go:build darwin

package main

/*
#cgo CFLAGS: -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "native_darwin.h"
*/
import "C"
import (
	"encoding/json"
	"unsafe"
)

type cocoaUI struct{}

func newDesktopUI() desktopUI { return cocoaUI{} }
func (cocoaUI) init()         { C.nativeInit() }
func (cocoaUI) add(kind string, id, page, x, y, w, h int, text string) {
	k, t := C.CString(kind), C.CString(text)
	defer C.free(unsafe.Pointer(k))
	defer C.free(unsafe.Pointer(t))
	C.nativeAdd(k, C.int(id), C.int(page), C.int(x), C.int(y), C.int(w), C.int(h), t)
}
func (cocoaUI) text(id int) string {
	p := C.nativeText(C.int(id))
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}
func (cocoaUI) setText(id int, text string) {
	p := C.CString(text)
	defer C.free(unsafe.Pointer(p))
	C.nativeSetText(C.int(id), p)
}
func (cocoaUI) options(id int, items []string) {
	if items == nil {
		items = []string{}
	}
	b, _ := json.Marshal(items)
	p := C.CString(string(b))
	defer C.free(unsafe.Pointer(p))
	C.nativeOptions(C.int(id), p)
}
func (cocoaUI) selection(id int) int      { return int(C.nativeSelection(C.int(id))) }
func (cocoaUI) selectIndex(id, index int) { C.nativeSelect(C.int(id), C.int(index)) }
func (cocoaUI) enable(id int, enabled bool) {
	v := 0
	if enabled {
		v = 1
	}
	C.nativeEnable(C.int(id), C.int(v))
}
func (cocoaUI) show(id int, visible bool) {
	v := 0
	if visible {
		v = 1
	}
	C.nativeShow(C.int(id), C.int(v))
}
func (cocoaUI) pick(folder bool) string {
	v := 0
	if folder {
		v = 1
	}
	p := C.nativePick(C.int(v))
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}
func (cocoaUI) alert(title, message string) {
	t, m := C.CString(title), C.CString(message)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(m))
	C.nativeAlert(t, m)
}
func (cocoaUI) confirm(title, message string) bool {
	t, m := C.CString(title), C.CString(message)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(m))
	return C.nativeConfirm(t, m) != 0
}
func (cocoaUI) run()  { C.nativeRun() }
func (cocoaUI) stop() { C.nativeStop() }

//export onNativeEvent
func onNativeEvent(id C.int) {
	if desktop != nil {
		desktop.event(int(id))
	}
}
