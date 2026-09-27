package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

type progressionSettings struct {
	EditableAttributes  bool
	NoAttributeCap      bool
	EditableSkillPoints bool
	MaxLevel            int
}

type instructionPatch struct {
	offset            int
	original, patched []byte
}

func instruction(offset int, originalHex, prefixHex string) instructionPatch {
	original, e := hex.DecodeString(originalHex)
	if e != nil {
		panic(e)
	}
	prefix, e := hex.DecodeString(prefixHex)
	if e != nil {
		panic(fmt.Sprintf("Invalid patch encoding at 0x%X: %v", offset, e))
	}
	if len(prefix) > len(original) {
		panic(fmt.Sprintf("Invalid patch at 0x%X: replacement is %d bytes, original is %d", offset, len(prefix), len(original)))
	}
	patched := bytes.Clone(original)
	copy(patched, prefix)
	return instructionPatch{offset, original, patched}
}

// Size NOP padding from the original instruction window instead of manually
// counting padding bytes in each early-return patch.
func returnPatch(offset int, originalHex, returnHex string) instructionPatch {
	p := instruction(offset, originalHex, returnHex)
	for i := len(returnHex) / 2; i < len(p.patched); i++ {
		p.patched[i] = 0x90
	}
	return p
}

func (p instructionPatch) window(data []byte) ([]byte, error) {
	if p.offset < 0 || p.offset > len(data)-len(p.original) {
		return nil, fmt.Errorf("The DLL is too short for patch offset 0x%X", p.offset)
	}
	return data[p.offset : p.offset+len(p.original)], nil
}

func readPatchGroup(data []byte, patches []instructionPatch) (bool, error) {
	enabled := false
	for i, p := range patches {
		actual, e := p.window(data)
		if e != nil {
			return false, e
		}
		on := bytes.Equal(actual, p.patched)
		if !on && !bytes.Equal(actual, p.original) {
			return false, fmt.Errorf("Unsupported GameAssembly.dll: unexpected instructions at 0x%X. No changes were written", p.offset)
		}
		if i > 0 && on != enabled {
			return false, fmt.Errorf("Incomplete patch at 0x%X. Restore a consistent DLL backup before patching", p.offset)
		}
		enabled = on
	}
	return enabled, nil
}

func (l *patchLayout) readProgressionSettings(data []byte) (progressionSettings, error) {
	s := progressionSettings{}
	if _, e := parsePE(data); e != nil {
		return s, e
	}
	var e error
	if s.EditableAttributes, e = readPatchGroup(data, l.editableAttributePatches); e != nil {
		return s, e
	}
	if s.NoAttributeCap, e = readPatchGroup(data, l.noAttributeCapPatches); e != nil {
		return s, e
	}
	if s.EditableSkillPoints, e = readPatchGroup(data, l.editableSkillPointPatches); e != nil {
		return s, e
	}
	for i, p := range l.levelCapPatches {
		actual, e := p.window(data)
		if e != nil {
			return s, e
		}
		prefix, cap := 2, 0
		if len(actual) == 3 {
			cap = int(actual[2])
		} else {
			prefix = 1
			cap = int(binary.LittleEndian.Uint32(actual[1:]))
		}
		if !bytes.Equal(actual[:prefix], p.original[:prefix]) || cap < 25 || cap > 127 {
			return s, fmt.Errorf("Unsupported level-cap instructions at 0x%X. No changes were written", p.offset)
		}
		if i > 0 && s.MaxLevel != cap {
			return s, fmt.Errorf("The level-cap patches disagree. Restore a consistent DLL backup before patching")
		}
		s.MaxLevel = cap
	}
	return s, nil
}

func writePatchGroup(data []byte, patches []instructionPatch, enabled bool) {
	for _, p := range patches {
		value := p.original
		if enabled {
			value = p.patched
		}
		copy(data[p.offset:], value)
	}
}

func (l *patchLayout) patchedGameDLL(data []byte, multiplier float64, rarity, mode, stars int, s progressionSettings) ([]byte, error) {
	if s.MaxLevel < 25 || s.MaxLevel > 127 {
		return nil, fmt.Errorf("Max level cap must be an integer from 25 to 127 (25 restores the original cap)")
	}
	if e := l.validateCurrent(data); e != nil {
		return nil, e
	}
	out, e := l.patchedLootDLL(data, multiplier, rarity, mode, stars)
	if e != nil {
		return nil, e
	}
	writePatchGroup(out, l.editableAttributePatches, s.EditableAttributes)
	writePatchGroup(out, l.noAttributeCapPatches, s.NoAttributeCap)
	writePatchGroup(out, l.editableSkillPointPatches, s.EditableSkillPoints)
	for _, p := range l.levelCapPatches {
		if len(p.original) == 3 {
			out[p.offset+2] = byte(s.MaxLevel)
		} else {
			binary.LittleEndian.PutUint32(out[p.offset+1:], uint32(s.MaxLevel))
		}
	}
	return out, nil
}
