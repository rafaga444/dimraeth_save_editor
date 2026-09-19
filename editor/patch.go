package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

var patchOffsets = []int{0xD32320, 0x10BA98C, 0x10BBEF4}
var originals = [][]byte{{0x48, 0x83, 0xec, 0x28, 0x33, 0xd2, 0xe8, 0x25, 0xf8, 0xff}, {0x41, 0x8b, 0xd7}, {0x41, 0x8b, 0xd7}}

func patchedDLL(data []byte, multiplier float64, rarity int) ([]byte, error) {
	if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || math.IsInf(float64(float32(multiplier)), 0) || float32(multiplier) == 0 {
		return nil, errors.New("Multiplier must be a positive, finite float32 value")
	}
	if rarity < 0 || rarity > 5 {
		return nil, errors.New("Rarity must be between 0 and 5")
	}
	if _, e := parsePE(data); e != nil {
		return nil, e
	}
	for i, o := range patchOffsets {
		n := len(originals[i])
		if o+n > len(data) {
			return nil, errors.New("The DLL is too short for this patch")
		}
		p := data[o : o+n]
		known := bytes.Equal(p, originals[i])
		if i == 0 {
			known = known || (p[0] == 0xb8 && bytes.Equal(p[5:], []byte{0x66, 0x0f, 0x6e, 0xc0, 0xc3}))
		} else {
			known = known || (p[0] == 0x6a && p[1] <= 5 && p[2] == 0x5a)
		}
		if !known {
			return nil, fmt.Errorf("Unsupported GameAssembly.dll version: unexpected bytes at 0x%X. The file has not been changed", o)
		}
	}
	out := bytes.Clone(data)
	p := []byte{0xb8, 0, 0, 0, 0, 0x66, 0x0f, 0x6e, 0xc0, 0xc3}
	binary.LittleEndian.PutUint32(p[1:5], math.Float32bits(float32(multiplier)))
	copy(out[patchOffsets[0]:], p)
	for _, o := range patchOffsets[1:] {
		copy(out[o:], []byte{0x6a, byte(rarity), 0x5a})
	}
	return out, nil
}
func patchFile(path string, m float64, r int) (string, error) {
	data, e := readLimited(path, 512<<20)
	if e != nil {
		return "", e
	}
	out, e := patchedDLL(data, m, r)
	if e != nil {
		return "", e
	}
	b, e := backup(path)
	if e != nil {
		return "", e
	}
	return b, replaceChecked(path, out, sha256.Sum256(data))
}
