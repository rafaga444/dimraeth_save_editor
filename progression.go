package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// PlayerStats.XPRequiredForLevelUp in the supported game build uses single
// precision Pow(level, 1.5) * 100 + 150, followed by ties-to-even rounding.
// ValidateLoadedXPData requires all-time XP = prior level costs + accumulated XP.
// Spendable XP is a separate balance and does not determine character level.
// Reference DLL file offsets: XPRequiredForLevelUp 0xA2D700,
// ValidateLoadedXPData 0x933040, ValidateAttributeXPConsistency 0x932970.
// Attribute validation computes race/class upgrade costs from game assets and
// calls ForceRespec if that budget exceeds earned XP (with a 5% tolerance).
// This helper repairs the level/XP relation; it does not bypass that validation.
func xpRequiredForLevel(level int64) int64 {
	power := float32(math.Pow(float64(level), 1.5))
	product := float32(power * 100)
	return int64(math.RoundToEven(float64(float32(product + 150))))
}

func progressionInt(p map[string]any, key string) (int64, error) {
	n, ok := p[key].(json.Number)
	if !ok {
		return 0, fmt.Errorf("Cannot synchronize this save: missing numeric %s", key)
	}
	v, e := n.Int64()
	if e != nil || v < 0 || v > math.MaxInt32 {
		return 0, fmt.Errorf("%s must be an integer from 0 to %d", key, math.MaxInt32)
	}
	return v, nil
}

func synchronizeProgression(doc map[string]any) (map[string]any, error) {
	return synchronizeProgressionAtCap(doc, 25)
}

func synchronizeProgressionAtCap(doc map[string]any, cap int) (map[string]any, error) {
	if cap < 25 || cap > 127 {
		return nil, fmt.Errorf("Max level cap must be from 25 to 127")
	}
	out, e := applyEdits(doc, nil)
	if e != nil {
		return nil, e
	}
	p := out["playerData"].(map[string]any)
	level, e := progressionInt(p, "characterLevel")
	if e != nil {
		return nil, e
	}
	if level < 1 || level > int64(cap) {
		return nil, fmt.Errorf("Choose a level from 1 to %d before synchronization, or increase Max level cap in Patcher and apply it with Save", cap)
	}
	progress, e := progressionInt(p, "characterAccumulatedXP")
	if e != nil {
		return nil, e
	}
	if level < int64(cap) && progress >= xpRequiredForLevel(level) {
		return nil, fmt.Errorf("XP toward next level must be below %d at level %d. Adjust characterAccumulatedXP or characterLevel first", xpRequiredForLevel(level), level)
	}
	if _, e = progressionInt(p, "characterAllTimeXP"); e != nil {
		return nil, e
	}
	highest, e := progressionInt(p, "characterHighestLevel")
	if e != nil {
		return nil, e
	}
	total := progress
	for i := int64(1); i < level; i++ {
		total += xpRequiredForLevel(i)
	}
	if total > math.MaxInt32 {
		return nil, fmt.Errorf("Total XP would exceed the game's 32-bit integer limit")
	}
	p["characterAllTimeXP"] = json.Number(strconv.FormatInt(total, 10))
	p["characterHighestLevel"] = json.Number(strconv.FormatInt(min(int64(cap), max(highest, level)), 10))
	return out, nil
}

func fieldHelp(f Field) string {
	path := strings.Join(f.Path, ".")
	switch path {
	case "playerData.characterLevel", "playerData.characterHighestLevel":
		return "For levels above 25, set Max level cap in Patcher and click Save there. Synchronize level / XP uses that cap to reconcile progression fields."
	case "playerData.characterXP":
		return "Spendable XP is separate from level progress and all-time XP. Editable attributes in Patcher disables XP/attribute validation."
	case "playerData.characterAccumulatedXP", "playerData.characterAllTimeXP":
		return "All-time XP must equal prior level costs plus XP toward the next level. Synchronize level / XP recalculates all-time XP from your level and progress."
	}
	if isAttributePath(f.Path) {
		return "Editable attributes in Patcher disables XP/attribute validation. No attribute cap removes the upgrade limit of 99. Click Save in Patcher to apply toggles."
	}
	return "Type: " + f.Type + "\nChanges stay in memory until you click Save."
}
