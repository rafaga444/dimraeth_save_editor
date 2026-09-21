package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func rngFixture(t *testing.T) []byte {
	t.Helper()
	// Minimal PE64 accepted by the format validator, with real patch instructions.
	b := make([]byte, 0x10BC000)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 0x80)
	copy(b[0x80:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(b[0x84:], 0x8664)
	binary.LittleEndian.PutUint16(b[0x86:], 1)
	binary.LittleEndian.PutUint16(b[0x94:], 0xf0)
	binary.LittleEndian.PutUint16(b[0x98:], 0x20b)
	for i, o := range patchOffsets {
		copy(b[o:], originals[i])
	}
	for i, o := range rngOffsets {
		copy(b[o:], rngOriginals[i])
	}
	return b
}

func TestRNGEliminator(t *testing.T) {
	original := rngFixture(t)
	var last []byte
	for stars := 1; stars <= 9; stars++ {
		input := original
		if last != nil {
			input = last
		}
		out, e := patchedLootDLL(input, 3, 5, rngEnable, stars)
		if e != nil {
			t.Fatal(e)
		}
		for i, o := range rngOffsets {
			want := bytes.Repeat([]byte{0x90}, len(rngOriginals[i]))
			if i >= 2 {
				want[0] = 0xba
				binary.LittleEndian.PutUint32(want[1:5], uint32(stars))
			}
			if !bytes.Equal(out[o:o+len(want)], want) {
				t.Fatalf("stars %d site %x", stars, o)
			}
		}
		last = out
	}
	kept, e := patchedLootDLL(last, 2, 1, rngKeep, 0)
	if e != nil {
		t.Fatal(e)
	}
	for i, o := range rngOffsets {
		if !bytes.Equal(kept[o:o+len(rngOriginals[i])], last[o:o+len(rngOriginals[i])]) {
			t.Fatal("keep changed RNG")
		}
	}
	restored, e := patchedLootDLL(last, 3, 5, rngDisable, 0)
	if e != nil {
		t.Fatal(e)
	}
	lootOnly, _ := patchedDLL(original, 3, 5)
	if !bytes.Equal(restored, lootOnly) {
		t.Fatal("disable did not restore original RNG instructions")
	}
	for _, star := range []int{0, 10, -1} {
		if _, e := patchedLootDLL(original, 3, 5, rngEnable, star); e == nil {
			t.Fatal("invalid stars accepted")
		}
	}
	// A failure at the last site must not partially write any earlier sites.
	bad := bytes.Clone(original)
	bad[rngOffsets[3]] = 0
	path := filepath.Join(t.TempDir(), "GameAssembly.dll")
	if e := os.WriteFile(path, bad, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := patchLootFile(path, 3, 5, rngEnable, 6); e == nil {
		t.Fatal("unknown bytes accepted")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, bad) {
		t.Fatal("file changed after rejection")
	}
	matches, _ := filepath.Glob(path + ".backup-*")
	if len(matches) != 0 {
		t.Fatal("backup made before validation")
	}
}

func TestRNGFileBackup(t *testing.T) {
	original := rngFixture(t)
	path := filepath.Join(t.TempDir(), "GameAssembly.dll")
	if e := os.WriteFile(path, original, 0600); e != nil {
		t.Fatal(e)
	}
	b, e := patchLootFile(path, 2.5, 2, rngEnable, 6)
	if e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(b)
	if e != nil || !bytes.Equal(got, original) {
		t.Fatal("backup does not match original", e)
	}
	got, _ = os.ReadFile(path)
	want, _ := patchedLootDLL(original, 2.5, 2, rngEnable, 6)
	if !bytes.Equal(got, want) {
		t.Fatal("written DLL differs")
	}
}

func TestRNGPythonFixture(t *testing.T) {
	path := os.Getenv("DIMRAETH_TEST_RNG_EXPECTED")
	if path == "" {
		t.Skip("set DIMRAETH_TEST_RNG_EXPECTED to the reference Python patcher's output")
	}
	original, e := os.ReadFile(filepath.Join("..", "GameAssembly.dll"))
	if e != nil {
		t.Fatal(e)
	}
	want, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	got, e := patchedLootDLL(original, 3, 5, rngEnable, 6)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("output differs from patch_dimraeth_rng_eliminator.py")
	}
}
