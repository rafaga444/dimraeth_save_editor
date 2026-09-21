package main

import (
	"runtime"
	"testing"
)

func TestWindowsTabPageStacking(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ui := newDesktopUI().(*windowsUI)
	desktop = &desktopApp{ui: ui}
	ui.init()
	defer user32.NewProc("DestroyWindow").Call(ui.root)
	ui.add("entry", savePathID, 0, 15, 32, 850, 28, "test.jrf")
	ui.add("entry", multiplierID, 1, 22, 174, 400, 32, "3")
	ui.add("toggle", editableAttributesID, 1, 22, 342, 700, 28, "Editable attributes")
	user32.NewProc("ShowWindow").Call(ui.root, 5)
	for _, index := range []int{0, 1, 0, 1, 0} {
		msg(ui.tabs, 0x130c, uintptr(index), 0)
		ui.selectPage(index)
		// A hit in the tab's content area must reach the page, not its
		// opaque sibling tab control. This catches the original gray overlay.
		point := ui.px(100) | ui.px(300)<<32
		hit, _, _ := user32.NewProc("ChildWindowFromPoint").Call(ui.root, point)
		if hit != ui.pages[index] {
			t.Fatalf("tab %d: hit %x, want page %x", index, hit, ui.pages[index])
		}
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(ui.pages[1-index])
		if visible != 0 {
			t.Fatalf("inactive page %d is visible", 1-index)
		}
	}
	ui.selectPage(1)
	for _, state := range []int{1, 0, 1} {
		ui.selectIndex(editableAttributesID, state)
		if ui.selection(editableAttributesID) != state {
			t.Fatal("checkbox state not set")
		}
		msg(ui.controls[editableAttributesID], 0xf5, 0, 0) // BM_CLICK
		if ui.selection(editableAttributesID) != 1-state || desktop.requestEditableAttributes != (state == 0) {
			t.Fatal("checkbox click did not update application state")
		}
	}
}
