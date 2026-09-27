package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type aob struct {
	values       []byte
	exact        []bool
	anchor       []byte
	anchorOffset int
}

func parseAOB(pattern string) (aob, error) {
	p := aob{}
	for _, token := range strings.Fields(pattern) {
		if token == "?" || token == "??" {
			p.values = append(p.values, 0)
			p.exact = append(p.exact, false)
			continue
		}
		value, err := hex.DecodeString(token)
		if err != nil || len(value) != 1 {
			return p, fmt.Errorf("Invalid AOB token: %q", token)
		}
		p.values = append(p.values, value[0])
		p.exact = append(p.exact, true)
	}
	for i := 0; i < len(p.values); {
		if !p.exact[i] {
			i++
			continue
		}
		j := i
		for j < len(p.values) && p.exact[j] {
			j++
		}
		if j-i > len(p.anchor) {
			p.anchor, p.anchorOffset = p.values[i:j], i
		}
		i = j
	}
	if len(p.anchor) == 0 {
		return p, fmt.Errorf("AOB must contain at least one exact byte")
	}
	return p, nil
}

func (p aob) matches(data []byte, offset int) bool {
	if offset < 0 || offset > len(data)-len(p.values) {
		return false
	}
	for i, exact := range p.exact {
		if exact && data[offset+i] != p.values[i] {
			return false
		}
	}
	return true
}

type signatureMatchError struct {
	message string
	count   int
}

func (e *signatureMatchError) Error() string { return e.message }

type aobMatch struct {
	offset  int
	section section
	va      uint64
}

type aobResolver struct {
	data     []byte
	pe       *peImage
	sections []section
	cache    map[string]aobMatch
}

func newAOBResolver(data []byte) (*aobResolver, error) {
	pe, err := parsePE(data)
	if err != nil {
		return nil, err
	}
	r := &aobResolver{data: data, pe: pe, cache: map[string]aobMatch{}}
	for _, s := range pe.sections {
		if s.characteristics&0x20000000 != 0 && s.raw > 0 {
			r.sections = append(r.sections, s)
		}
	}
	if len(r.sections) == 0 {
		return nil, fmt.Errorf("PE contains no executable sections")
	}
	return r, nil
}

func (r *aobResolver) resolve(label, signature string) (aobMatch, error) {
	if found, ok := r.cache[signature]; ok {
		return found, nil
	}
	pattern, err := parseAOB(signature)
	if err != nil {
		return aobMatch{}, err
	}
	var found []aobMatch
	count := 0
	for _, s := range r.sections {
		start, end := int(s.off), int(s.off)+int(s.raw)
		for pos := start; pos < end; {
			i := bytes.Index(r.data[pos:end], pattern.anchor)
			if i < 0 {
				break
			}
			i += pos
			candidate := i - pattern.anchorOffset
			if candidate >= start && candidate <= end-len(pattern.values) && pattern.matches(r.data, candidate) {
				count++
				if len(found) < 20 {
					found = append(found, aobMatch{candidate, s, r.pe.base + uint64(s.va) + uint64(candidate-start)})
				}
			}
			pos = i + 1
		}
	}
	if count != 1 {
		details := ""
		for _, m := range found {
			details += fmt.Sprintf("\n  %s: file=0x%X, VA=0x%X", m.section.name, m.offset, m.va)
		}
		return aobMatch{}, &signatureMatchError{
			message: fmt.Sprintf("%s: expected exactly 1 AOB match, found %d.%s\nThis game build changed too much or the signature became ambiguous. No DLL modifications were written", label, count, details),
			count:   count,
		}
	}
	r.cache[signature] = found[0]
	return found[0], nil
}

type patchTarget struct {
	instructionPatch
	name, section string
	va            uint64
}

type patchLayout struct {
	editableAttributePatches  []instructionPatch
	noAttributeCapPatches     []instructionPatch
	editableSkillPointPatches []instructionPatch
	levelCapPatches           []instructionPatch
	rngPatches                []instructionPatch
	starPatch                 instructionPatch
	patchOffsets              []int
	originals                 [][]byte
	targets                   []patchTarget
	sections                  []section
	cleanHash                 [32]byte
}

// Every address is relative to a unique semantic context from the supplied v14 script.
func resolvePatchLayout(data []byte) (*patchLayout, error) {
	r, err := newAOBResolver(data)
	if err != nil {
		return nil, err
	}
	l := &patchLayout{sections: r.sections, cleanHash: sha256.Sum256(data)}
	specs := []struct {
		name, signature              string
		rel                          int
		expected, replacement, group string
		padNOP                       bool
	}{
		{"XP level-loop cap", SIG_MAX_LEVEL_PAIR, 0, "83 FB 19", "", "level", false},
		{"XP final cap", SIG_MAX_LEVEL_PAIR, 7, "B9 19 00 00 00", "", "level", false},
		{"Highest-level initialization", SIG_HIGHEST_LEVEL, 0, "BA 19 00 00 00", "", "level", false},
		{"ValidateLoadedXPData", SIG_VALIDATE_LOADED_XP, 0, "40 53 55 56 57 41 56 48", "c3", "attributes", true},
		{"ValidateAttributeXPConsistency", SIG_VALIDATE_ATTRIBUTE_XP, 0, "48 89 4C 24 08 53 56 57", "c3", "attributes", true},
		{"ValidateXPInvariants", SIG_VALIDATE_XP_INVARIANTS, 0, "48 89 4C 24 08 53 56 57", "b001c3", "attributes", true},
		{"ValidateXPState", SIG_VALIDATE_XP_STATE, 0, "48 89 4C 24 08 53 56 57", "b001c3", "attributes", true},
		{"CanReachNextLevel", SIG_ATTRIBUTE_CAP_1, 13, "83 F8 63 0F 8D D8 FE FF FF", "83f863909090909090", "cap", false},
		{"TrySpendPoint", SIG_ATTRIBUTE_CAP_2, 14, "83 F8 63 0F 8D 0A 01 00 00", "83f863909090909090", "cap", false},
		{"ApplyUpgradeAttributeInternal", SIG_ATTRIBUTE_CAP_3, 16, "83 F8 63 0F 8D 74 04 00 00", "83f863909090909090", "cap", false},
		{"UpgradeAttributeServerRpc", SIG_ATTRIBUTE_CAP_4, 16, "83 F8 63 0F 8D FC 04 00 00", "83f863909090909090", "cap", false},
		{"SkillPointOverGrantPersists", SIG_SKILL_OVER_GRANT, 0, "40 53 48 83 EC 20 48 8B D9 41 3B D0 7E 42 0F 57", "31c0c3", "skills", true},
		{"Edited skill points preserved", SIG_SKILL_RECONCILE, 13, "0F 84 99 02 00 00", "0f8e99020000", "skills", false},
		{"LootChanceMultiplier", SIG_LOOT_MULTIPLIER, 0, "48 83 EC 28 33 D2 E8 25 F8 FF", "", "loot", false},
		{"ReturnRandomRuneData rarity", SIG_RETURN_RANDOM_RUNE, 12, "E8 ?? ?? ?? ??", "", "loot", false},
		{"Positive item drops", SIG_GUARANTEED_ITEM, 14, "44 3B F0 7D AB", "85c07eac90", "rng", false},
		{"RuneDropCheck equipment drops", SIG_GUARANTEED_EQUIPMENT, 14, "33 D2 45 33 C0 0F 28 C6 E8 ?? ?? ?? ??", "31c00f57c00f2ff00f97c09090", "rng", false},
		{"GearLegality.Inspect", SIG_GEAR_LEGALITY_INSPECT, 0, "40 55 53 48 8D 6C 24 D8", "31c0c3", "rng", true},
		{"ReturnRandomRuneData stars", SIG_RETURN_RANDOM_RUNE, 31, "E8 ?? ?? ?? ??", "", "stars", false},
	}
	for _, spec := range specs {
		match, err := r.resolve(spec.name, spec.signature)
		if err != nil {
			return nil, err
		}
		expected, err := parseAOB(spec.expected)
		if err != nil {
			return nil, err
		}
		offset := match.offset + spec.rel
		if offset < int(match.section.off) || offset+len(expected.values) > int(match.section.off)+int(match.section.raw) || !expected.matches(data, offset) {
			return nil, fmt.Errorf("%s: context matched but patch-target verification failed at 0x%X. No DLL modifications were written", spec.name, offset)
		}
		original := bytes.Clone(data[offset : offset+len(expected.values)])
		replacement := bytes.Clone(original)
		if spec.replacement != "" {
			replacement, err = hex.DecodeString(spec.replacement)
			if err != nil {
				return nil, fmt.Errorf("Internal patch definition error for %s: invalid replacement encoding. No DLL modifications were written", spec.name)
			}
			if spec.padNOP && len(replacement) <= len(original) {
				// Early-return bodies contain only their opcodes; derive the NOP
				// count from the verified original instruction window.
				padded := bytes.Repeat([]byte{0x90}, len(original))
				copy(padded, replacement)
				replacement = padded
			}
			if len(replacement) != len(original) {
				return nil, fmt.Errorf("Internal patch definition error for %s: replacement has %d bytes, target has %d. No DLL modifications were written", spec.name, len(replacement), len(original))
			}
		}
		p := instructionPatch{offset, original, replacement}
		l.targets = append(l.targets, patchTarget{p, spec.name, match.section.name, match.va + uint64(spec.rel)})
		switch spec.group {
		case "level":
			l.levelCapPatches = append(l.levelCapPatches, p)
		case "attributes":
			l.editableAttributePatches = append(l.editableAttributePatches, p)
		case "cap":
			l.noAttributeCapPatches = append(l.noAttributeCapPatches, p)
		case "skills":
			l.editableSkillPointPatches = append(l.editableSkillPointPatches, p)
		case "rng":
			l.rngPatches = append(l.rngPatches, p)
		case "stars":
			l.starPatch = p
		case "loot":
			l.patchOffsets = append(l.patchOffsets, offset)
			l.originals = append(l.originals, original)
		}
	}
	ordered := append([]patchTarget(nil), l.targets...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].offset < ordered[j].offset })
	for i := 1; i < len(ordered); i++ {
		left, right := ordered[i-1], ordered[i]
		if left.offset+len(left.original) > right.offset {
			return nil, fmt.Errorf("Patch overlap: %s and %s. No DLL modifications were written", left.name, right.name)
		}
	}
	return l, nil
}

// Hashes establish backup identity and detect unrelated edits, never a build whitelist.
func (l *patchLayout) validateCurrent(data []byte) error {
	if _, err := l.readProgressionSettings(data); err != nil {
		return err
	}
	if _, err := l.readLootSettings(data); err != nil {
		return err
	}
	normalized := bytes.Clone(data)
	for _, p := range l.targets {
		copy(normalized[p.offset:], p.original)
	}
	if sha256.Sum256(normalized) != l.cleanHash {
		return fmt.Errorf("The backup does not belong to this DLL, or code outside the known patch targets changed. No DLL modifications were written")
	}
	return nil
}
