package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttributeEditsEnableToggleAndWarn(t *testing.T) {
	for _, key := range []string{"characterAttributes", "CharacterAttributes"} {
		doc, e := decodeJSON([]byte(`{"playerData":{"characterName":"Test","` + key + `":[4,5],"characterGold":10}}`))
		if e != nil {
			t.Fatal(e)
		}
		u := newTestUI()
		a := &desktopApp{ui: u, save: &Save{doc: doc}}
		a.showField(Field{[]string{"playerData", key, "0"}, "number", "4", "playerData." + key})
		if u.selection(editableAttributesID) != 0 || len(u.alerts) != 0 {
			t.Fatal("selection alone enabled the patch")
		}
		u.setText(valueID, "invalid")
		if e := a.commit(); e == nil {
			t.Fatal("invalid edit accepted")
		}
		if u.selection(editableAttributesID) != 0 || len(u.alerts) != 0 {
			t.Fatal("invalid edit enabled the patch")
		}
		u.setText(valueID, "100")
		if e := a.commit(); e != nil {
			t.Fatal(e)
		}
		if u.selection(editableAttributesID) != 1 || !a.requestEditableAttributes || !a.dirty {
			t.Fatal("attribute patch was not enabled")
		}
		if len(u.alerts) != 1 || !strings.Contains(u.alerts[0], "Save in Patcher") {
			t.Fatal("missing application warning", u.alerts)
		}
		u.setText(valueID, "101")
		if e := a.commit(); e != nil {
			t.Fatal(e)
		}
		if len(u.alerts) != 1 {
			t.Fatal("warning repeated while enabled")
		}
		u.selectIndex(editableAttributesID, 0)
		a.event(editableAttributesID)
		u.setText(valueID, "102")
		if e := a.commit(); e != nil {
			t.Fatal(e)
		}
		if len(u.alerts) != 2 || u.selection(editableAttributesID) != 1 {
			t.Fatal("disabled toggle not restored on next edit")
		}
	}
}

func TestLoadingPatchSettingsPreservesRequestedAttributes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "GameAssembly.dll")
	original := progressionFixture(t)
	if e := os.WriteFile(path, original, 0600); e != nil {
		t.Fatal(e)
	}
	u := newTestUI()
	a := &desktopApp{ui: u, requestEditableAttributes: true}
	if e := a.loadPatchSettings(path); e != nil {
		t.Fatal(e)
	}
	if !a.patchReady || u.selection(editableAttributesID) != 1 || u.text(maxLevelID) != "25" {
		t.Fatal("pending attribute request lost")
	}
	a.requestEditableAttributes = false
	b, e := patchedGameDLL(original, 3, 5, rngKeep, 6, progressionSettings{true, true, true, 127})
	if e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	if e := a.loadPatchSettings(path); e != nil {
		t.Fatal(e)
	}
	for _, id := range []int{editableAttributesID, noAttributeCapID, editableSkillPointsID} {
		if u.selection(id) != 1 {
			t.Fatal("patch setting not read", id)
		}
	}
	if u.text(maxLevelID) != "127" {
		t.Fatal("cap not read")
	}
	b[editableAttributePatches[0].offset+5] ^= 0xff
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	if e := a.loadPatchSettings(path); e == nil || a.patchReady {
		t.Fatal("unknown DLL remains patchable")
	}
}

func TestMaxLevelInput(t *testing.T) {
	u := newTestUI()
	a := &desktopApp{ui: u}
	for _, input := range []string{"", "24", "128", "60.5", "NaN", "2147483648"} {
		u.setText(maxLevelID, input)
		if _, e := a.maxLevelCap(); e == nil {
			t.Fatal("invalid cap accepted", input)
		}
	}
	u.setText(maxLevelID, " 60 ")
	if cap, e := a.maxLevelCap(); e != nil || cap != 60 {
		t.Fatal(cap, e)
	}
}
