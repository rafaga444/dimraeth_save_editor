package main

import (
	"bytes"
	"crypto/sha256"
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

// Exact instruction windows from patch_dimraeth_diag_v13.py.
// Early returns include the NOP padding used by that script.
var editableAttributePatches = []instructionPatch{
	returnPatch(0x933D60, "4053555657415648", "c3"),
	returnPatch(0x933690, "48894c2408535657", "c3"),
	returnPatch(0x934770, "48894c2408535657", "b001c3"),
	returnPatch(0x935900, "48894c2408535657", "b001c3"),
}
var noAttributeCapPatches = []instructionPatch{
	instruction(0x9BE73F, "83f8630f8dd8feffff", "83f863909090909090"),
	instruction(0x9C2611, "83f8630f8d0a010000", "83f863909090909090"),
	instruction(0xA21D76, "83f8630f8d74040000", "83f863909090909090"),
	instruction(0xA2DF69, "83f8630f8dfc040000", "83f863909090909090"),
}
var editableSkillPointPatches = []instructionPatch{
	returnPatch(0x932C10, "40534883ec20488bd9413bd07e420f57", "31c0c3"),
	instruction(0xA08D06, "0f8499020000", "0f8e99020000"),
}
var levelCapPatches = []instructionPatch{
	instruction(0x1149991, "83fb19", "83fb19"),
	instruction(0x1149998, "b919000000", "b919000000"),
	instruction(0x92BEDD, "ba19000000", "ba19000000"),
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

func readProgressionSettings(data []byte) (progressionSettings, error) {
	s := progressionSettings{}
	if _, e := parsePE(data); e != nil {
		return s, e
	}
	var e error
	if s.EditableAttributes, e = readPatchGroup(data, editableAttributePatches); e != nil {
		return s, e
	}
	if s.NoAttributeCap, e = readPatchGroup(data, noAttributeCapPatches); e != nil {
		return s, e
	}
	if s.EditableSkillPoints, e = readPatchGroup(data, editableSkillPointPatches); e != nil {
		return s, e
	}
	for i, p := range levelCapPatches {
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

func patchedGameDLL(data []byte, multiplier float64, rarity, mode, stars int, s progressionSettings) ([]byte, error) {
	if s.MaxLevel < 25 || s.MaxLevel > 127 {
		return nil, fmt.Errorf("Max level cap must be an integer from 25 to 127 (25 restores the original cap)")
	}
	if e := validateSupportedV13(data); e != nil {
		return nil, e
	}
	out, e := patchedLootDLL(data, multiplier, rarity, mode, stars)
	if e != nil {
		return nil, e
	}
	writePatchGroup(out, editableAttributePatches, s.EditableAttributes)
	writePatchGroup(out, noAttributeCapPatches, s.NoAttributeCap)
	writePatchGroup(out, editableSkillPointPatches, s.EditableSkillPoints)
	for _, p := range levelCapPatches {
		if len(p.original) == 3 {
			out[p.offset+2] = byte(s.MaxLevel)
		} else {
			binary.LittleEndian.PutUint32(out[p.offset+1:], uint32(s.MaxLevel))
		}
	}
	return out, nil
}

func patchGameFile(path string, multiplier float64, rarity, mode, stars int, s progressionSettings) (string, error) {
	data, e := readLimited(path, 512<<20)
	if e != nil {
		return "", e
	}
	out, e := patchedGameDLL(data, multiplier, rarity, mode, stars, s)
	if e != nil {
		return "", e
	}
	b, e := backup(path)
	if e != nil {
		return "", e
	}
	return b, replaceChecked(path, out, sha256.Sum256(data))
}
