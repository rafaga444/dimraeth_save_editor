package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const supportedV13SHA256 = "4129db654d1257c7e619e24490508297d5e6e66c26f059d978dc913eab7e70a5"

const (
	rngKeep = iota
	rngDisable
	rngEnable
)

// v13 replaces the full array_get CALL with MOV EAX, imm32. Replacing
// only BL leaves stale high bits in the rarity passed to GenerateRuneData.
var patchOffsets = []int{0xD26940, 0x10BCCB1}
var originals = [][]byte{
	{0x48, 0x83, 0xec, 0x28, 0x33, 0xd2, 0xe8, 0x25, 0xf8, 0xff},
	{0xe8, 0x8a, 0x1a, 0xfa, 0x00},
}

// RNG eliminator combines --guaranteed-item, --guaranteed-equipment and
// --gear-legality-bypass. The positive-chance checks remain intact.
var rngPatches = []instructionPatch{
	instruction(0xBC9C90, "443bf07dab", "85c07eac90"),
	instruction(0xBCA224, "33d24533c00f28c6e84f825700", "31c00f57c00f2ff00f97c09090"),
	instruction(0x104E4D0, "405553488d6c24d8", "31c0c39090909090"),
}
var starPatch = instruction(0x10BCCC4, "e8771afa00", "e8771afa00")

// Keep these instruction lists available to the existing source fixtures.
var rngOffsets = []int{0xBC9C90, 0xBCA224, 0x104E4D0, 0x10BCCC4}
var rngOriginals = [][]byte{rngPatches[0].original, rngPatches[1].original, rngPatches[2].original, starPatch.original}

type lootSettings struct {
	Multiplier          float64
	Rarity, Stars, Mode int
	ForcedStars         bool
}

func validMultiplier(m float64) bool {
	return m > 0 && !math.IsNaN(m) && !math.IsInf(m, 0) && !math.IsInf(float64(float32(m)), 0) && float32(m) != 0
}

func readLootSettings(data []byte) (lootSettings, error) {
	s := lootSettings{Multiplier: 3, Rarity: 5, Stars: 3, Mode: rngKeep}
	if _, e := parsePE(data); e != nil {
		return s, e
	}
	for i, o := range patchOffsets {
		p := instructionPatch{offset: o, original: originals[i]}
		actual, e := p.window(data)
		if e != nil {
			return s, e
		}
		if bytes.Equal(actual, p.original) {
			continue
		}
		known := false
		if i == 0 {
			if actual[0] == 0xb8 && bytes.Equal(actual[5:], []byte{0x66, 0x0f, 0x6e, 0xc0, 0xc3}) {
				s.Multiplier = float64(math.Float32frombits(binary.LittleEndian.Uint32(actual[1:5])))
				known = validMultiplier(s.Multiplier)
			}
		} else if actual[0] == 0xb8 {
			s.Rarity = int(binary.LittleEndian.Uint32(actual[1:5]))
			known = s.Rarity >= 0 && s.Rarity <= 5
		}
		if !known {
			return s, fmt.Errorf("Unsupported v13 loot instructions at 0x%X. Restore the clean supported DLL", o)
		}
	}
	onCount := 0
	for _, p := range rngPatches {
		on, e := readPatchGroup(data, []instructionPatch{p})
		if e != nil {
			return s, e
		}
		if on {
			onCount++
		}
	}
	actual, e := starPatch.window(data)
	if e != nil {
		return s, e
	}
	if !bytes.Equal(actual, starPatch.original) {
		// Recognize v13 files created externally with stars 4–9 so they can
		// be restored or reduced. This application only writes stars 1–3.
		if actual[0] != 0xb8 || binary.LittleEndian.Uint32(actual[1:5]) > 8 {
			return s, fmt.Errorf("Unsupported v13 star instructions at 0x%X. Restore the clean supported DLL", starPatch.offset)
		}
		s.Stars = int(binary.LittleEndian.Uint32(actual[1:5])) + 1
		s.ForcedStars = true
	}
	if onCount == 0 && !s.ForcedStars {
		s.Mode = rngDisable
	}
	if onCount == len(rngPatches) && s.ForcedStars && s.Stars <= 3 {
		s.Mode = rngEnable
	}
	return s, nil
}

// Validate the exact v13 build while allowing repeat edits of known patches.
// Normalization happens in memory only; the DLL is untouched on rejection.
func validateSupportedV13(data []byte) error {
	if _, e := readProgressionSettings(data); e != nil {
		return e
	}
	if _, e := readLootSettings(data); e != nil {
		return e
	}
	clean := bytes.Clone(data)
	for _, group := range [][]instructionPatch{editableAttributePatches, noAttributeCapPatches, editableSkillPointPatches, levelCapPatches, rngPatches, {starPatch}} {
		writePatchGroup(clean, group, false)
	}
	for i, o := range patchOffsets {
		copy(clean[o:], originals[i])
	}
	if fmt.Sprintf("%x", sha256.Sum256(clean)) != supportedV13SHA256 {
		return fmt.Errorf("Unsupported GameAssembly.dll build. Restore the clean DLL supported by diagnostic patcher v13 (SHA-256 %s). No changes were written", supportedV13SHA256)
	}
	return nil
}

func patchedDLL(data []byte, multiplier float64, rarity int) ([]byte, error) {
	if !validMultiplier(multiplier) {
		return nil, errors.New("Multiplier must be a positive, finite float32 value")
	}
	if rarity < 0 || rarity > 5 {
		return nil, errors.New("Rarity must be between 0 and 5")
	}
	if _, e := readLootSettings(data); e != nil {
		return nil, e
	}
	out := bytes.Clone(data)
	p := []byte{0xb8, 0, 0, 0, 0, 0x66, 0x0f, 0x6e, 0xc0, 0xc3}
	binary.LittleEndian.PutUint32(p[1:5], math.Float32bits(float32(multiplier)))
	copy(out[patchOffsets[0]:], p)
	out[patchOffsets[1]] = 0xb8
	binary.LittleEndian.PutUint32(out[patchOffsets[1]+1:], uint32(rarity))
	return out, nil
}

func patchedLootDLL(data []byte, multiplier float64, rarity, mode, stars int) ([]byte, error) {
	if mode < rngKeep || mode > rngEnable {
		return nil, errors.New("Select an RNG eliminator mode")
	}
	if mode == rngEnable && (stars < 1 || stars > 3) {
		return nil, errors.New("Stars must be between 1 and 3")
	}
	current, e := readLootSettings(data)
	if e != nil {
		return nil, e
	}
	if mode == rngKeep && current.ForcedStars && current.Stars > 3 {
		return nil, errors.New("The DLL forces more than 3 stars. Enable RNG eliminator and select 1–3 stars, or disable it to restore the original star selection")
	}
	out, e := patchedDLL(data, multiplier, rarity)
	if e != nil || mode == rngKeep {
		return out, e
	}
	writePatchGroup(out, rngPatches, mode == rngEnable)
	copy(out[starPatch.offset:], starPatch.original)
	if mode == rngEnable {
		out[starPatch.offset] = 0xb8
		// The Stars enum is zero-based: 1★ = 0, 2★ = 1, 3★ = 2.
		binary.LittleEndian.PutUint32(out[starPatch.offset+1:], uint32(stars-1))
	}
	return out, nil
}

func patchFile(path string, m float64, r int) (string, error) {
	return patchLootFile(path, m, r, rngKeep, 3)
}

func patchLootFile(path string, m float64, r, mode, stars int) (string, error) {
	data, e := readLimited(path, 512<<20)
	if e != nil {
		return "", e
	}
	if e = validateSupportedV13(data); e != nil {
		return "", e
	}
	out, e := patchedLootDLL(data, m, r, mode, stars)
	if e != nil {
		return "", e
	}
	b, e := backup(path)
	if e != nil {
		return "", e
	}
	return b, replaceChecked(path, out, sha256.Sum256(data))
}
