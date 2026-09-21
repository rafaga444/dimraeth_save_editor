package main

import (
	"fmt"
	"strconv"
	"strings"
)

func isAttributePath(path []string) bool {
	return len(path) >= 2 && strings.EqualFold(path[0], "playerData") && strings.EqualFold(path[1], "characterAttributes")
}

func (a *desktopApp) ensureEditableAttributes() {
	a.requestEditableAttributes = true
	if a.ui.selection(editableAttributesID) == 1 {
		return
	}
	a.ui.selectIndex(editableAttributesID, 1)
	a.ui.alert("Edited attributes require a game patch", "Enable Editable attributes to prevent the game from reverting your attribute changes. This toggle has been turned on automatically.\n\nSelect your game folder and click Save in Patcher to apply it to GameAssembly.dll. Saving the save file alone does not apply the patch.")
}

func (a *desktopApp) maxLevelCap() (int, error) {
	cap, e := strconv.Atoi(strings.TrimSpace(a.ui.text(maxLevelID)))
	if e != nil || cap < 25 || cap > 127 {
		return 0, fmt.Errorf("Max level cap must be an integer from 25 to 127 (25 restores the original cap)")
	}
	return cap, nil
}

func (a *desktopApp) loadPatchSettings(path string) error {
	a.patchReady = false
	data, e := readLimited(path, 512<<20)
	if e != nil {
		return e
	}
	s, e := readProgressionSettings(data)
	if e != nil {
		return e
	}
	old := a.updating
	a.updating = true
	defer func() { a.updating = old }()
	for _, setting := range []struct {
		id int
		on bool
	}{
		{editableAttributesID, s.EditableAttributes || a.requestEditableAttributes},
		{noAttributeCapID, s.NoAttributeCap},
		{editableSkillPointsID, s.EditableSkillPoints},
	} {
		value := 0
		if setting.on {
			value = 1
		}
		a.ui.selectIndex(setting.id, value)
	}
	a.ui.setText(maxLevelID, strconv.Itoa(s.MaxLevel))
	a.ui.setText(dllInfoID, path)
	a.patchReady = true
	return nil
}
