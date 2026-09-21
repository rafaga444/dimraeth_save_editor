package main

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const (
	quitID                = 1
	gamePathID            = 10
	browseGameID          = 11
	loadGameID            = 12
	catalogInfoID         = 13
	savePathID            = 20
	browseSaveID          = 21
	loadSaveID            = 22
	backupID              = 23
	saveID                = 24
	saveInfoID            = 25
	syncXPID              = 26
	groupID               = 30
	searchID              = 31
	fieldsID              = 32
	fieldPathID           = 33
	valueID               = 34
	boolID                = 35
	applyID               = 36
	fieldInfoID           = 37
	countID               = 38
	itemSearchID          = 40
	itemID                = 41
	quantityID            = 42
	addID                 = 43
	inventoryID           = 44
	slotsID               = 45
	multiplierID          = 50
	rarityID              = 51
	patchID               = 52
	dllInfoID             = 53
	rngModeID             = 54
	starsID               = 55
	editableAttributesID  = 56
	noAttributeCapID      = 57
	editableSkillPointsID = 58
	maxLevelID            = 59
	statusID              = 60
)

type desktopUI interface {
	init()
	add(kind string, id, page, x, y, w, h int, text string)
	text(id int) string
	setText(id int, text string)
	options(id int, items []string)
	selection(id int) int
	selectIndex(id, index int)
	enable(id int, enabled bool)
	show(id int, visible bool)
	pick(folder bool) string
	alert(title, message string)
	confirm(title, message string) bool
	run()
	stop()
}

type desktopApp struct {
	ui                        desktopUI
	save                      *Save
	catalog                   *Catalog
	allFields, visibleFields  []Field
	groups                    []string
	matchingItems             []Item
	inventoryPaths            [][]string
	selected                  *Field
	dirty, updating           bool
	patchReady                bool
	requestEditableAttributes bool
}

var desktop *desktopApp

func main() {
	runtime.LockOSThread()
	desktop = &desktopApp{ui: newDesktopUI()}
	desktop.ui.init()
	desktop.build()
	desktop.ui.run()
}
func (a *desktopApp) build() {
	a.updating = true
	defer func() { a.updating = false }()
	u := a.ui
	u.add("heading", 100, -1, 20, 10, 900, 26, "DIMRAETH  /  SAVE WORKSHOP")
	u.add("label", 101, -1, 20, 42, 110, 24, "Game folder")
	u.add("entry", gamePathID, -1, 130, 40, 690, 28, "")
	u.add("button", browseGameID, -1, 830, 40, 100, 30, "Browse…")
	u.add("button", loadGameID, -1, 940, 40, 170, 30, "Load item catalog")
	u.add("label", catalogInfoID, -1, 20, 77, 1090, 25, "Select a game installation to load item IDs and names.")
	u.add("label", 102, 0, 15, 8, 150, 22, "Save file path")
	u.add("entry", savePathID, 0, 15, 32, 850, 28, "")
	u.add("button", browseSaveID, 0, 875, 31, 95, 30, "Browse…")
	u.add("button", loadSaveID, 0, 980, 31, 95, 30, "Open")
	u.add("label", saveInfoID, 0, 15, 70, 500, 30, "No save file is open.")
	u.add("button", syncXPID, 0, 550, 69, 220, 30, "Synchronize level / XP")
	u.add("button", backupID, 0, 785, 69, 145, 30, "Create backup")
	u.add("button", saveID, 0, 940, 69, 135, 30, "Save")
	u.add("label", 103, 0, 15, 112, 260, 22, "Parameter section")
	u.add("combo", groupID, 0, 15, 135, 340, 28, "")
	u.add("label", 104, 0, 370, 112, 275, 22, "Search parameters")
	u.add("entry", searchID, 0, 370, 135, 290, 28, "")
	u.add("list", fieldsID, 0, 15, 178, 645, 236, "")
	u.add("heading", 105, 0, 685, 135, 390, 26, "Edit selected parameter")
	u.add("label", fieldPathID, 0, 685, 179, 390, 60, "Select a parameter or an inventory row.")
	u.add("entry", valueID, 0, 685, 253, 390, 30, "")
	u.add("combo", boolID, 0, 685, 253, 390, 30, "")
	u.options(boolID, []string{"False", "True"})
	u.add("button", applyID, 0, 685, 296, 180, 32, "Apply value")
	u.add("label", fieldInfoID, 0, 685, 342, 390, 72, "Changes stay in memory until you click Save.")
	u.add("label", countID, 0, 15, 420, 640, 24, "")
	u.add("heading", 106, 0, 15, 452, 440, 25, "Inventory  /  Add an item")
	u.add("label", 107, 0, 15, 483, 260, 22, "Search name or ID")
	u.add("label", 108, 0, 300, 483, 470, 22, "Item")
	u.add("label", 109, 0, 790, 483, 110, 22, "Quantity")
	u.add("entry", itemSearchID, 0, 15, 506, 270, 28, "")
	u.add("combo", itemID, 0, 300, 506, 475, 28, "")
	u.add("entry", quantityID, 0, 790, 506, 100, 28, "1")
	u.add("button", addID, 0, 905, 505, 170, 30, "Add item")
	u.add("list", inventoryID, 0, 15, 548, 1060, 99, "")
	u.add("label", slotsID, 0, 15, 654, 1060, 25, "Select an inventory row to edit its quantity. New items use empty slots.")
	u.add("heading", 110, 1, 22, 20, 800, 30, "Patch settings")
	u.add("label", 111, 1, 22, 67, 1020, 54, "Configure loot and character progression in GameAssembly.dll.")
	u.add("label", 112, 1, 22, 138, 440, 25, "Drop chance multiplier")
	u.add("entry", multiplierID, 1, 22, 174, 400, 32, "3")
	u.add("label", 113, 1, 470, 138, 520, 25, "Rune and equipment rarity")
	u.add("combo", rarityID, 1, 470, 174, 500, 32, "")
	u.options(rarityID, []string{"Rarity.Common = 0", "Rarity.Uncommon = 1", "Rarity.Rare = 2", "Rarity.Mythical = 3", "Rarity.Heroic = 4", "Rarity.Ancient = 5"})
	u.selectIndex(rarityID, 5)
	u.add("label", 116, 1, 22, 230, 400, 25, "RNG eliminator")
	u.add("combo", rngModeID, 1, 22, 262, 400, 32, "")
	u.options(rngModeID, []string{"Keep existing RNG patch", "Disable RNG eliminator", "Enable RNG eliminator"})
	u.selectIndex(rngModeID, rngKeep)
	u.add("label", 117, 1, 470, 230, 500, 25, "Forced stars (runes and equipment)")
	u.add("combo", starsID, 1, 470, 262, 500, 32, "")
	u.options(starsID, []string{"1★ — requires level 1", "2★ — requires level 5", "3★ — requires level 10", "4★ — requires level 15", "5★ — requires level 20", "6★ — requires level 25", "7★ — requires level 30", "8★ — requires level 35", "9★ — requires level 40"})
	u.selectIndex(starsID, 5)
	u.add("heading", 120, 1, 22, 305, 700, 25, "Progression")
	u.add("toggle", editableAttributesID, 1, 22, 342, 700, 28, "Editable attributes (disables XP/attribute validation)")
	u.add("toggle", noAttributeCapID, 1, 22, 380, 700, 28, "No attribute cap")
	u.add("toggle", editableSkillPointsID, 1, 22, 418, 700, 28, "Editable skill points")
	u.add("label", 121, 1, 760, 310, 260, 25, "Max level cap")
	u.add("entry", maxLevelID, 1, 760, 342, 210, 32, "25")
	u.add("label", 122, 1, 760, 390, 260, 48, "25–127; 25 restores the original cap.")
	u.add("label", dllInfoID, 1, 22, 454, 1020, 48, "Select the game folder first.")
	u.add("button", patchID, 1, 22, 520, 210, 36, "Save")
	u.add("label", 114, 1, 22, 574, 1020, 72, "Close the game before saving. A backup is created before GameAssembly.dll is replaced.\nTurning a progression toggle off restores its original instructions.\nUnknown patch instructions are rejected before any changes are written.")
	u.add("label", statusID, -1, 20, 845, 1090, 42, "Ready. All file operations run inside this application.")
	u.show(boolID, false)
	a.refreshEnabled()
}
func (a *desktopApp) refreshEnabled() {
	u := a.ui
	for _, id := range []int{backupID, saveID, syncXPID, groupID, searchID, fieldsID, inventoryID} {
		u.enable(id, a.save != nil)
	}
	u.enable(applyID, a.selected != nil && a.selected.Type != "null")
	u.enable(valueID, a.selected != nil && a.selected.Type != "null")
	u.enable(boolID, a.selected != nil)
	u.enable(addID, a.save != nil && len(a.matchingItems) > 0)
	u.enable(patchID, a.catalog != nil && a.patchReady)
	u.enable(starsID, a.ui.selection(rngModeID) == rngEnable)
}
func (a *desktopApp) status(s string) { a.ui.setText(statusID, s) }
func (a *desktopApp) fail(e error) {
	if e == nil {
		return
	}
	a.status(e.Error())
	a.ui.alert("Dimraeth Editor", e.Error())
}
func (a *desktopApp) pending() bool {
	if a.selected == nil || a.selected.Type == "null" {
		return false
	}
	return a.value() != a.selected.Value
}
func (a *desktopApp) value() string {
	if a.selected != nil && a.selected.Type == "boolean" {
		return strconv.FormatBool(a.ui.selection(boolID) == 1)
	}
	return a.ui.text(valueID)
}
func (a *desktopApp) commit() error {
	if !a.pending() {
		return nil
	}
	value := a.value()
	doc, e := applyEdits(a.save.doc, []Edit{{Path: a.selected.Path, Value: value}})
	if e != nil {
		return e
	}
	if isAttributePath(a.selected.Path) {
		a.ensureEditableAttributes()
	}
	a.save.doc = doc
	a.dirty = true
	a.selected.Value = value
	a.allFields = fields(doc)
	path := strings.Join(a.selected.Path, ".")
	for i := range a.visibleFields {
		if strings.Join(a.visibleFields[i].Path, ".") == path {
			a.visibleFields[i].Value = value
		}
	}
	name := a.save.doc["playerData"].(map[string]any)["characterName"]
	a.ui.setText(saveInfoID, fmt.Sprintf("%v  •  Unsaved changes", name))
	return nil
}
func (a *desktopApp) refreshSave() {
	a.updating = true
	defer func() { a.updating = false }()
	a.allFields = fields(a.save.doc)
	old := "playerData"
	if n := a.ui.selection(groupID); n >= 0 && n < len(a.groups) {
		old = a.groups[n]
	}
	groups := map[string]bool{}
	for _, f := range a.allFields {
		groups[f.Group] = true
	}
	a.groups = []string{"All sections"}
	rest := []string{}
	for g := range groups {
		rest = append(rest, g)
	}
	sort.Strings(rest)
	a.groups = append(a.groups, rest...)
	a.ui.options(groupID, a.groups)
	sel := 0
	for i, g := range a.groups {
		if g == old {
			sel = i
		}
	}
	a.ui.selectIndex(groupID, sel)
	a.ui.setText(savePathID, a.save.path)
	name := a.save.doc["playerData"].(map[string]any)["characterName"]
	state := "Saved"
	if a.dirty {
		state = "Unsaved changes"
	}
	a.ui.setText(saveInfoID, fmt.Sprintf("%v  •  %s", name, state))
	a.filterFields()
	a.refreshInventory()
	a.refreshEnabled()
}
func labelFor(f Field) string {
	key := f.Path[len(f.Path)-1]
	names := map[string]string{"characterName": "Character name", "characterGold": "Gold", "characterLevel": "Level", "characterXP": "Spendable XP", "characterAccumulatedXP": "XP toward next level", "characterAllTimeXP": "All-time XP", "characterHighestLevel": "Highest level", "characterHealth": "Health", "characterSkillPoints": "Skill points", "characterStamina": "Stamina", "characterConcentration": "Concentration"}
	if n := names[key]; n != "" {
		return n
	}
	return key
}
func (a *desktopApp) filterFields() {
	old := a.updating
	a.updating = true
	defer func() { a.updating = old }()
	q := strings.ToLower(a.ui.text(searchID))
	group := "All sections"
	if n := a.ui.selection(groupID); n >= 0 && n < len(a.groups) {
		group = a.groups[n]
	}
	selectedPath := ""
	if a.selected != nil {
		selectedPath = strings.Join(a.selected.Path, ".")
	}
	a.visibleFields = nil
	rows := []string{}
	sel := -1
	for _, f := range a.allFields {
		path := strings.Join(f.Path, ".")
		if group != "All sections" && f.Group != group {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(path+" "+labelFor(f)), q) {
			continue
		}
		if path == selectedPath {
			sel = len(rows)
		}
		a.visibleFields = append(a.visibleFields, f)
		v := strings.ReplaceAll(f.Value, "\n", " ")
		if len(v) > 110 {
			v = v[:110] + "…"
		}
		rows = append(rows, labelFor(f)+"   =   "+v+"   ["+path+"]")
	}
	a.ui.options(fieldsID, rows)
	a.ui.selectIndex(fieldsID, sel)
	a.ui.setText(countID, fmt.Sprintf("%d parameters. Select a row, enter a value, then click Apply value or Save.", len(rows)))
}
func (a *desktopApp) showField(f Field) {
	a.selected = &f
	a.ui.setText(fieldPathID, strings.Join(f.Path, "."))
	a.ui.setText(valueID, f.Value)
	a.ui.show(valueID, f.Type != "boolean")
	a.ui.show(boolID, f.Type == "boolean")
	if f.Type == "boolean" {
		n := 0
		if f.Value == "true" {
			n = 1
		}
		a.ui.selectIndex(boolID, n)
	}
	a.ui.setText(fieldInfoID, fieldHelp(f))
	a.refreshEnabled()
}
func (a *desktopApp) filterItems() {
	old := a.updating
	a.updating = true
	defer func() { a.updating = old }()
	q := strings.ToLower(a.ui.text(itemSearchID))
	a.matchingItems = nil
	rows := []string{}
	if a.catalog != nil {
		for _, item := range a.catalog.Items {
			if q == "" || strings.Contains(strings.ToLower(item.Name+" "+item.Symbol+" "+strconv.Itoa(item.ID)), q) {
				a.matchingItems = append(a.matchingItems, item)
				rows = append(rows, fmt.Sprintf("%s   #%d", item.Name, item.ID))
			}
		}
	}
	a.ui.options(itemID, rows)
	a.ui.selectIndex(itemID, 0)
	a.refreshEnabled()
}
func (a *desktopApp) refreshInventory() {
	old := a.updating
	a.updating = true
	defer func() { a.updating = old }()
	p := a.save.doc["playerData"].(map[string]any)
	inv, _ := p["characterInventory"].([]any)
	a.inventoryPaths = nil
	rows := []string{}
	for i, x := range inv {
		v, ok := x.(map[string]any)
		if !ok || number(v["Kind"]) == 0 {
			continue
		}
		id := int(number(v["Item"]))
		name := fmt.Sprintf("Item #%d", id)
		if number(v["Kind"]) != 1 {
			name = "Rune / pet"
		} else if a.catalog != nil {
			for _, item := range a.catalog.Items {
				if item.ID == id {
					name = item.Name
					break
				}
			}
		}
		rows = append(rows, fmt.Sprintf("Slot %d   |   %s   |   ID %d   |   Quantity %v", i+1, name, id, v["Amount"]))
		a.inventoryPaths = append(a.inventoryPaths, []string{"playerData", "characterInventory", strconv.Itoa(i), "Amount"})
	}
	a.ui.options(inventoryID, rows)
	a.ui.setText(slotsID, fmt.Sprintf("%d used / %d empty slots. Select a row to edit its quantity. Item names come from ItemType.", len(rows), len(inv)-len(rows)))
}
func (a *desktopApp) event(id int) {
	if a.updating {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			a.fail(fmt.Errorf("Operation failed: %v", r))
		}
	}()
	switch id {
	case quitID:
		if (a.dirty || a.pending()) && !a.ui.confirm("Unsaved changes", "Quit without saving your changes?") {
			return
		}
		a.ui.stop()
	case browseGameID:
		if p := a.ui.pick(true); p != "" {
			a.ui.setText(gamePathID, p)
			a.event(loadGameID)
		}
	case loadGameID:
		a.status("Reading game metadata and DLL…")
		c, e := loadCatalog(strings.TrimSpace(a.ui.text(gamePathID)))
		if e != nil {
			a.fail(e)
			return
		}
		a.catalog = c
		a.ui.setText(catalogInfoID, fmt.Sprintf("%d items  •  Metadata v%d  •  Assembly-CSharp DLL verified", len(c.Items), c.Version))
		patchError := a.loadPatchSettings(c.DLL)
		a.filterItems()
		if a.save != nil {
			a.refreshInventory()
		}
		a.refreshEnabled()
		if patchError != nil {
			a.ui.setText(dllInfoID, "Patching unavailable: "+patchError.Error())
			a.status("Item catalog loaded. Patching unavailable for this DLL.")
		} else {
			a.status("Item catalog and patch settings loaded.")
		}
	case browseSaveID:
		if p := a.ui.pick(false); p != "" {
			a.ui.setText(savePathID, p)
			a.event(loadSaveID)
		}
	case loadSaveID:
		if (a.dirty || a.pending()) && !a.ui.confirm("Unsaved changes", "Open another file and discard unsaved changes?") {
			return
		}
		s, e := loadSave(strings.TrimSpace(a.ui.text(savePathID)))
		if e != nil {
			a.fail(e)
			return
		}
		a.save = s
		a.dirty = false
		a.selected = nil
		a.ui.setText(valueID, "")
		a.ui.setText(fieldPathID, "Select a parameter or an inventory row.")
		a.ui.show(valueID, true)
		a.ui.show(boolID, false)
		a.refreshSave()
		a.status("Save file decrypted and opened.")
	case groupID, searchID:
		if e := a.commit(); e != nil {
			a.fail(e)
			return
		}
		if a.save != nil {
			a.allFields = fields(a.save.doc)
			a.filterFields()
		}
	case fieldsID:
		n := a.ui.selection(fieldsID)
		if n < 0 || n >= len(a.visibleFields) {
			return
		}
		f := a.visibleFields[n]
		if e := a.commit(); e != nil {
			a.fail(e)
			return
		}
		a.showField(f)
		a.filterFields()
		a.refreshInventory()
	case inventoryID:
		n := a.ui.selection(inventoryID)
		if n < 0 || n >= len(a.inventoryPaths) {
			return
		}
		if e := a.commit(); e != nil {
			a.fail(e)
			return
		}
		for _, f := range fields(a.save.doc) {
			if strings.Join(f.Path, ".") == strings.Join(a.inventoryPaths[n], ".") {
				a.showField(f)
				break
			}
		}
	case applyID:
		if e := a.commit(); e != nil {
			a.fail(e)
			return
		}
		a.refreshSave()
		a.status("Value applied. Click Save to write the encrypted file.")
	case saveID:
		if a.save == nil {
			return
		}
		if e := a.commit(); e != nil {
			a.fail(e)
			return
		}
		b, e := a.save.write(a.save.doc)
		if e != nil {
			a.fail(e)
			return
		}
		a.dirty = false
		a.refreshSave()
		message := "Save file encrypted and written. Backup: " + b
		if a.requestEditableAttributes {
			message += "  Apply Editable attributes using Save in Patcher before loading the game."
		}
		a.status(message)
	case backupID:
		if a.save == nil {
			return
		}
		b, e := backup(a.save.path)
		if e != nil {
			a.fail(e)
			return
		}
		a.status("Backup created: " + b)
	case itemSearchID:
		a.filterItems()
	case addID:
		if a.save == nil {
			return
		}
		n := a.ui.selection(itemID)
		if n < 0 || n >= len(a.matchingItems) {
			a.fail(fmt.Errorf("Select an item"))
			return
		}
		q, e := strconv.Atoi(strings.TrimSpace(a.ui.text(quantityID)))
		if e != nil {
			a.fail(fmt.Errorf("Quantity must be an integer"))
			return
		}
		if e = a.commit(); e != nil {
			a.fail(e)
			return
		}
		doc, e := applyEdits(a.save.doc, nil)
		if e == nil {
			e = addItem(doc, a.matchingItems[n].ID, q, a.catalog)
		}
		if e != nil {
			a.fail(e)
			return
		}
		a.save.doc = doc
		a.dirty = true
		a.refreshSave()
		a.status("Item added. Click Save to write the encrypted file.")
	case syncXPID:
		if a.save == nil {
			return
		}
		if e := a.commit(); e != nil {
			a.fail(e)
			return
		}
		cap, e := a.maxLevelCap()
		if e != nil {
			a.fail(e)
			return
		}
		doc, e := synchronizeProgressionAtCap(a.save.doc, cap)
		if e != nil {
			a.fail(e)
			return
		}
		a.save.doc = doc
		a.dirty = true
		a.refreshSave()
		if a.selected != nil {
			for _, f := range a.allFields {
				if strings.Join(f.Path, ".") == strings.Join(a.selected.Path, ".") {
					a.showField(f)
					break
				}
			}
		}
		a.status("Level, level progress and all-time XP synchronized. Spendable XP and attributes are unchanged. Click Save to write.")
	case editableAttributesID:
		a.requestEditableAttributes = a.ui.selection(editableAttributesID) == 1
	case rngModeID:
		a.refreshEnabled()
	case patchID:
		if a.catalog == nil || !a.patchReady {
			return
		}
		m, e := strconv.ParseFloat(strings.TrimSpace(a.ui.text(multiplierID)), 64)
		if e != nil {
			a.fail(fmt.Errorf("Multiplier must be a positive number"))
			return
		}
		cap, e := a.maxLevelCap()
		if e != nil {
			a.fail(e)
			return
		}
		settings := progressionSettings{
			EditableAttributes:  a.ui.selection(editableAttributesID) == 1,
			NoAttributeCap:      a.ui.selection(noAttributeCapID) == 1,
			EditableSkillPoints: a.ui.selection(editableSkillPointsID) == 1,
			MaxLevel:            cap,
		}
		a.status("Checking and patching GameAssembly.dll…")
		b, e := patchGameFile(a.catalog.DLL, m, a.ui.selection(rarityID), a.ui.selection(rngModeID), a.ui.selection(starsID)+1, settings)
		if e != nil {
			a.fail(e)
			return
		}
		a.requestEditableAttributes = false
		a.status("Patch applied. Backup: " + b)
	}
}
