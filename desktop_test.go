package main

import "testing"

// A memory-backed UI exercises editing without opening a platform window.
type testUI struct {
	values       map[int]string
	choices      map[int]int
	rows         map[int][]string
	allowDiscard bool
	stopped      bool
}

func newTestUI() *testUI {
	return &testUI{values: map[int]string{}, choices: map[int]int{}, rows: map[int][]string{}}
}
func (*testUI) init()                                            {}
func (*testUI) add(string, int, int, int, int, int, int, string) {}
func (u *testUI) text(id int) string                             { return u.values[id] }
func (u *testUI) setText(id int, s string)                       { u.values[id] = s }
func (u *testUI) options(id int, s []string)                     { u.rows[id] = s; u.choices[id] = -1 }
func (u *testUI) selection(id int) int                           { return u.choices[id] }
func (u *testUI) selectIndex(id, n int)                          { u.choices[id] = n }
func (*testUI) enable(int, bool)                                 {}
func (*testUI) show(int, bool)                                   {}
func (*testUI) pick(bool) string                                 { return "" }
func (*testUI) alert(string, string)                             {}
func (u *testUI) confirm(string, string) bool                    { return u.allowDiscard }
func (*testUI) run()                                             {}
func (u *testUI) stop()                                          { u.stopped = true }
func TestNativeFieldSwitchPreservesEdits(t *testing.T) {
	doc, e := decodeJSON([]byte(`{"playerData":{"characterName":"Test","characterGold":5,"characterLevel":9,"characterInventory":[]}}`))
	if e != nil {
		t.Fatal(e)
	}
	u := newTestUI()
	a := &desktopApp{ui: u, save: &Save{doc: doc}}
	a.refreshSave()
	index := func(key string) int {
		for i, f := range a.visibleFields {
			if f.Path[len(f.Path)-1] == key {
				return i
			}
		}
		t.Fatal("missing field", key)
		return -1
	}
	u.selectIndex(fieldsID, index("characterGold"))
	a.event(fieldsID)
	u.setText(valueID, "123")
	u.selectIndex(fieldsID, index("characterLevel"))
	a.event(fieldsID)
	if !a.dirty || number(a.save.doc["playerData"].(map[string]any)["characterGold"]) != 123 {
		t.Fatal("pending edit was lost")
	}
	u.selectIndex(fieldsID, index("characterGold"))
	a.event(fieldsID)
	if u.text(valueID) != "123" {
		t.Fatal("field shows stale value", u.text(valueID))
	}
	a.event(quitID)
	if u.stopped {
		t.Fatal("discard cancellation ignored")
	}
	u.allowDiscard = true
	a.event(quitID)
	if !u.stopped {
		t.Fatal("quit did not stop UI")
	}
}
func TestNativeInvalidEditDoesNotSwitchField(t *testing.T) {
	doc, e := decodeJSON([]byte(`{"playerData":{"characterGold":5,"characterLevel":9}}`))
	if e != nil {
		t.Fatal(e)
	}
	u := newTestUI()
	a := &desktopApp{ui: u, save: &Save{doc: doc}}
	a.refreshSave()
	u.selectIndex(fieldsID, 0)
	a.event(fieldsID)
	original := a.selected.Path[len(a.selected.Path)-1]
	u.setText(valueID, "not a number")
	u.selectIndex(fieldsID, 1)
	a.event(fieldsID)
	if a.selected.Path[len(a.selected.Path)-1] != original || a.dirty {
		t.Fatal("invalid edit changed selection or document")
	}
}

func TestSynchronizeButtonCommitsPendingLevel(t *testing.T) {
	doc, e := decodeJSON([]byte(`{"playerData":{"characterName":"Test","characterLevel":22,"characterHighestLevel":22,"characterAccumulatedXP":551,"characterAllTimeXP":0,"characterXP":5928,"characterInventory":[]}}`))
	if e != nil {
		t.Fatal(e)
	}
	u := newTestUI()
	a := &desktopApp{ui: u, save: &Save{doc: doc}}
	a.refreshSave()
	for _, f := range a.allFields {
		if f.Path[len(f.Path)-1] == "characterLevel" {
			a.showField(f)
			break
		}
	}
	u.setText(valueID, "23")
	a.event(syncXPID)
	p := a.save.doc["playerData"].(map[string]any)
	if number(p["characterAllTimeXP"]) != 99872 || number(p["characterHighestLevel"]) != 23 || !a.dirty {
		t.Fatal("synchronization lost the pending edit", p)
	}
	if a.pending() || u.text(valueID) != "23" {
		t.Fatal("synchronization left stale field text")
	}
}
