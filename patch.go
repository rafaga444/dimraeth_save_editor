package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const (
	rngKeep = iota
	rngDisable
	rngEnable
)

type lootSettings struct {
	Multiplier          float64
	Rarity, Stars, Mode int
	ForcedStars         bool
}

func validMultiplier(m float64) bool {
	return m > 0 && !math.IsNaN(m) && !math.IsInf(m, 0) && !math.IsInf(float64(float32(m)), 0) && float32(m) != 0
}

func (l *patchLayout) readLootSettings(data []byte) (lootSettings, error) {
	s := lootSettings{Multiplier: 3, Rarity: 5, Stars: 3, Mode: rngKeep}
	if _, e := parsePE(data); e != nil {
		return s, e
	}
	for i, o := range l.patchOffsets {
		p := instructionPatch{offset: o, original: l.originals[i]}
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
			return s, fmt.Errorf("Unsupported v14 loot instructions at 0x%X. Restore a clean DLL backup", o)
		}
	}
	onCount := 0
	for _, p := range l.rngPatches {
		on, e := readPatchGroup(data, []instructionPatch{p})
		if e != nil {
			return s, e
		}
		if on {
			onCount++
		}
	}
	actual, e := l.starPatch.window(data)
	if e != nil {
		return s, e
	}
	if !bytes.Equal(actual, l.starPatch.original) {
		// Recognize known files created externally with stars 4–9 so they can
		// be restored or reduced. This application only writes stars 1–3.
		if actual[0] != 0xb8 || binary.LittleEndian.Uint32(actual[1:5]) > 8 {
			return s, fmt.Errorf("Unsupported v14 star instructions at 0x%X. Restore a clean DLL backup", l.starPatch.offset)
		}
		s.Stars = int(binary.LittleEndian.Uint32(actual[1:5])) + 1
		s.ForcedStars = true
	}
	if onCount == 0 && !s.ForcedStars {
		s.Mode = rngDisable
	}
	if onCount == len(l.rngPatches) && s.ForcedStars && s.Stars <= 3 {
		s.Mode = rngEnable
	}
	return s, nil
}

func (l *patchLayout) patchedDLL(data []byte, multiplier float64, rarity int) ([]byte, error) {
	if !validMultiplier(multiplier) {
		return nil, errors.New("Multiplier must be a positive, finite float32 value")
	}
	if rarity < 0 || rarity > 5 {
		return nil, errors.New("Rarity must be between 0 and 5")
	}
	if _, e := l.readLootSettings(data); e != nil {
		return nil, e
	}
	out := bytes.Clone(data)
	p := []byte{0xb8, 0, 0, 0, 0, 0x66, 0x0f, 0x6e, 0xc0, 0xc3}
	binary.LittleEndian.PutUint32(p[1:5], math.Float32bits(float32(multiplier)))
	copy(out[l.patchOffsets[0]:], p)
	out[l.patchOffsets[1]] = 0xb8
	binary.LittleEndian.PutUint32(out[l.patchOffsets[1]+1:], uint32(rarity))
	return out, nil
}

func (l *patchLayout) patchedLootDLL(data []byte, multiplier float64, rarity, mode, stars int) ([]byte, error) {
	if mode < rngKeep || mode > rngEnable {
		return nil, errors.New("Select an RNG eliminator mode")
	}
	if mode == rngEnable && (stars < 1 || stars > 3) {
		return nil, errors.New("Stars must be between 1 and 3")
	}
	current, e := l.readLootSettings(data)
	if e != nil {
		return nil, e
	}
	if mode == rngKeep && current.ForcedStars && current.Stars > 3 {
		return nil, errors.New("The DLL forces more than 3 stars. Enable RNG eliminator and select 1–3 stars, or disable it to restore the original star selection")
	}
	out, e := l.patchedDLL(data, multiplier, rarity)
	if e != nil || mode == rngKeep {
		return out, e
	}
	writePatchGroup(out, l.rngPatches, mode == rngEnable)
	copy(out[l.starPatch.offset:], l.starPatch.original)
	if mode == rngEnable {
		out[l.starPatch.offset] = 0xb8
		// The Stars enum is zero-based: 1★ = 0, 2★ = 1, 3★ = 2.
		binary.LittleEndian.PutUint32(out[l.starPatch.offset+1:], uint32(stars-1))
	}
	return out, nil
}
