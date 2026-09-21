package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func progressionFixture(t *testing.T) []byte {
	b := append(rngFixture(t), make([]byte, 0x114B400-0x10BC000)...)
	for _, group := range [][]instructionPatch{editableAttributePatches, noAttributeCapPatches, editableSkillPointPatches, levelCapPatches} {
		for _, p := range group {
			copy(b[p.offset:], p.original)
		}
	}
	return b
}

func TestProgressionPatchesRoundTrip(t *testing.T) {
	original := progressionFixture(t)
	input := original
	for mask := 0; mask < 8; mask++ {
		for _, cap := range []int{25, 26, 60, 127} {
			settings := progressionSettings{mask&1 != 0, mask&2 != 0, mask&4 != 0, cap}
			out, e := patchedGameDLL(input, 3, 5, rngEnable, 9, settings)
			if e != nil {
				t.Fatal(settings, e)
			}
			got, e := readProgressionSettings(out)
			if e != nil || got != settings {
				t.Fatalf("got %+v, want %+v: %v", got, settings, e)
			}
			input = out
		}
	}
	// Restore every progression and RNG site; only the requested loot patch remains.
	restored, e := patchedGameDLL(input, 3, 5, rngDisable, 1, progressionSettings{MaxLevel: 25})
	if e != nil {
		t.Fatal(e)
	}
	want, _ := patchedDLL(original, 3, 5)
	if !bytes.Equal(restored, want) {
		t.Fatal("restore changed unrelated bytes")
	}
	if got, e := readProgressionSettings(original); e != nil || got != (progressionSettings{MaxLevel: 25}) {
		t.Fatal("input was mutated", got, e)
	}
	for _, cap := range []int{-1, 0, 24, 128, 256} {
		if _, e := patchedGameDLL(original, 3, 5, rngKeep, 6, progressionSettings{MaxLevel: cap}); e == nil {
			t.Fatal("invalid cap accepted", cap)
		}
	}
}

func TestProgressionRejectsUnknownAndPartialPatches(t *testing.T) {
	original := progressionFixture(t)
	for _, group := range [][]instructionPatch{editableAttributePatches, noAttributeCapPatches, editableSkillPointPatches, levelCapPatches} {
		for _, p := range group {
			bad := bytes.Clone(original)
			bad[p.offset+len(p.original)-1] ^= 0xff
			if _, e := patchedGameDLL(bad, 3, 5, rngEnable, 6, progressionSettings{MaxLevel: 60}); e == nil {
				t.Fatalf("unknown instructions accepted at %x", p.offset)
			}
		}
	}
	for _, group := range [][]instructionPatch{editableAttributePatches, noAttributeCapPatches, editableSkillPointPatches} {
		bad := bytes.Clone(original)
		copy(bad[group[0].offset:], group[0].patched)
		if _, e := readProgressionSettings(bad); e == nil {
			t.Fatal("partial toggle accepted")
		}
	}
	bad := bytes.Clone(original)
	bad[levelCapPatches[0].offset+2] = 60
	if _, e := readProgressionSettings(bad); e == nil {
		t.Fatal("inconsistent caps accepted")
	}
	if _, e := readProgressionSettings(original[:100]); e == nil {
		t.Fatal("truncated DLL accepted")
	}
}

func TestProgressionFileBackupAndRejection(t *testing.T) {
	original := progressionFixture(t)
	path := filepath.Join(t.TempDir(), "GameAssembly.dll")
	if e := os.WriteFile(path, original, 0600); e != nil {
		t.Fatal(e)
	}
	settings := progressionSettings{true, true, true, 60}
	b, e := patchGameFile(path, 2, 4, rngEnable, 8, settings)
	if e != nil {
		t.Fatal(e)
	}
	backupData, e := os.ReadFile(b)
	if e != nil || !bytes.Equal(backupData, original) {
		t.Fatal("invalid backup", e)
	}
	got, _ := os.ReadFile(path)
	want, _ := patchedGameDLL(original, 2, 4, rngEnable, 8, settings)
	if !bytes.Equal(got, want) {
		t.Fatal("incorrect DLL written")
	}
	// Unknown bytes in the final cap site must prevent all changes and backups.
	badPath := filepath.Join(t.TempDir(), "GameAssembly.dll")
	original[levelCapPatches[3].offset] = 0
	if e := os.WriteFile(badPath, original, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := patchGameFile(badPath, 2, 4, rngEnable, 8, settings); e == nil {
		t.Fatal("unknown DLL written")
	}
	got, _ = os.ReadFile(badPath)
	if !bytes.Equal(got, original) {
		t.Fatal("DLL partially changed on error")
	}
	entries, _ := os.ReadDir(filepath.Dir(badPath))
	if len(entries) != 1 {
		t.Fatal("backup created before validation")
	}
}

func TestProgressionPythonReference(t *testing.T) {
	path := os.Getenv("DIMRAETH_TEST_PROGRESSION_EXPECTED")
	if path == "" {
		t.Skip("set DIMRAETH_TEST_PROGRESSION_EXPECTED to the combined Python output at level cap 60")
	}
	original, e := os.ReadFile(filepath.Join("..", "GameAssembly.dll"))
	if e != nil {
		t.Fatal(e)
	}
	expected, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	// The UI also applies the independently tested loot settings.
	want, e := patchedLootDLL(expected, 3, 5, rngEnable, 6)
	if e != nil {
		t.Fatal(e)
	}
	got, e := patchedGameDLL(original, 3, 5, rngEnable, 6, progressionSettings{true, true, true, 60})
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("differs from the supplied Python scripts")
	}
}
