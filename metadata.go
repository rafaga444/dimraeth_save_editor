package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type Item struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
}
type Catalog struct {
	Items    []Item `json:"items"`
	Metadata string `json:"metadata"`
	DLL      string `json:"dll"`
	Version  int    `json:"version"`
	Methods  int    `json:"methods"`
}
type section struct{ va, size, off, raw uint32 }
type peImage struct {
	base     uint64
	sections []section
}

func parsePE(b []byte) (*peImage, error) {
	bad := errors.New("Expected a Windows x64 GameAssembly.dll (PE32+)")
	if len(b) < 64 || string(b[:2]) != "MZ" {
		return nil, bad
	}
	p := int(binary.LittleEndian.Uint32(b[60:64]))
	if p < 64 || p+136 > len(b) || string(b[p:p+4]) != "PE\x00\x00" {
		return nil, bad
	}
	u := binary.LittleEndian.Uint16
	if u(b[p+4:]) != 0x8664 || u(b[p+24:]) != 0x20b {
		return nil, bad
	}
	n := int(u(b[p+6:]))
	start := p + 24 + int(u(b[p+20:]))
	if n < 1 || n > 96 || start+n*40 > len(b) {
		return nil, bad
	}
	pe := &peImage{base: binary.LittleEndian.Uint64(b[p+48:])}
	for i := 0; i < n; i++ {
		o := start + i*40
		get := func(j int) uint32 { return binary.LittleEndian.Uint32(b[o+j:]) }
		s := section{get(12), get(8), get(20), get(16)}
		if uint64(s.off)+uint64(s.raw) > uint64(len(b)) {
			return nil, bad
		}
		pe.sections = append(pe.sections, s)
	}
	return pe, nil
}
func (p *peImage) addr(off int) uint64 {
	for _, s := range p.sections {
		if off >= int(s.off) && off < int(s.off+s.raw) {
			return p.base + uint64(s.va) + uint64(off-int(s.off))
		}
	}
	return 0
}
func (p *peImage) offset(va uint64) int {
	if va < p.base {
		return -1
	}
	r := va - p.base
	for _, s := range p.sections {
		if r >= uint64(s.va) && r < uint64(s.va)+uint64(s.raw) {
			return int(uint64(s.off) + r - uint64(s.va))
		}
	}
	return -1
}

// The code-generation module links metadata method tokens to executable addresses.
func verifyModule(dll []byte, want int) (int, error) {
	pe, e := parsePE(dll)
	if e != nil {
		return 0, e
	}
	name := []byte("Assembly-CSharp.dll\x00")
	for from := 0; from < len(dll); {
		i := bytes.Index(dll[from:], name)
		if i < 0 {
			break
		}
		i += from
		from = i + len(name)
		va := pe.addr(i)
		if va == 0 {
			continue
		}
		needle := make([]byte, 8)
		binary.LittleEndian.PutUint64(needle, va)
		for at := 0; at < len(dll); {
			j := bytes.Index(dll[at:], needle)
			if j < 0 {
				break
			}
			j += at
			at = j + 8
			if j+24 > len(dll) {
				continue
			}
			n := int(binary.LittleEndian.Uint64(dll[j+8:]))
			p := pe.offset(binary.LittleEndian.Uint64(dll[j+16:]))
			if n == want && n > 0 && p >= 0 && p+8*n <= len(dll) {
				return n, nil
			}
		}
	}
	return 0, errors.New("Metadata and DLL do not match: Assembly-CSharp module not found or method counts differ")
}

type metadataReader []byte

func (b metadataReader) part(o, n int) []byte {
	if o < 0 || n < 0 || o > len(b)-n {
		panic("Metadata table is outside the file bounds")
	}
	return b[o : o+n]
}
func (b metadataReader) u(o int) int                { return int(binary.LittleEndian.Uint32(b.part(o, 4))) }
func (b metadataReader) h(o int) int                { return int(binary.LittleEndian.Uint16(b.part(o, 2))) }
func (b metadataReader) table(o int) metadataReader { return b.part(b.u(o), b.u(o+4)) }
func (b metadataReader) str(o int) string {
	z := b.part(o, len(b)-o)
	n := bytes.IndexByte(z, 0)
	if n < 0 {
		panic("Unterminated metadata string")
	}
	return string(z[:n])
}
func compressedInt(b metadataReader, o int) int {
	c := b.part(o, 1)[0]
	var u uint32
	switch {
	case c < 0x80:
		u = uint32(c)
	case c < 0xc0:
		u = uint32(c&0x7f)<<8 | uint32(b.part(o+1, 1)[0])
	case c < 0xe0:
		p := b.part(o, 4)
		u = uint32(c&0x3f)<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3])
	case c == 0xf0:
		u = uint32(b.u(o + 1))
	case c == 0xfe:
		u = 0xfffffffe
	case c == 0xff:
		u = 0xffffffff
	default:
		panic("Unknown int32 encoding")
	}
	if u == 0xffffffff {
		return -2147483648
	}
	if u&1 != 0 {
		return -int(u>>1) - 1
	}
	return int(u >> 1)
}
func parseMetadata(data []byte) (items []Item, version, methods int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Invalid global-metadata.dat: %v", r)
		}
	}()
	b := metadataReader(data)
	if b.u(0) != 0xfab11baf {
		return nil, 0, 0, errors.New("Invalid global-metadata.dat signature")
	}
	version = b.u(4)
	if version != 29 && version != 31 {
		return nil, version, 0, fmt.Errorf("Metadata version %d is not supported; supported versions are 29 and 31", version)
	}
	stringsTable := b.table(24)
	fields := b.table(96)
	types := b.table(160)
	defaults := b.table(64)
	values := b.table(72)
	methodTable := b.table(48)
	images := b.table(168)
	if len(types)%88 != 0 || len(fields)%12 != 0 || len(defaults)%12 != 0 || len(images)%40 != 0 {
		panic("Invalid table sizes")
	}
	start, count := -1, 0
	for o := 0; o < len(images); o += 40 {
		if stringsTable.str(images.u(o)) == "Assembly-CSharp.dll" {
			start = images.u(o + 8)
			count = images.u(o + 12)
			break
		}
	}
	if start < 0 || count < 1 {
		panic("Assembly-CSharp.dll is missing from metadata")
	}
	def := map[int]int{}
	for o := 0; o < len(defaults); o += 12 {
		idx := defaults.u(o + 8)
		if idx != 0xffffffff {
			def[defaults.u(o)] = idx
		}
	}
	methodSize := 32
	if version == 31 {
		methodSize = 36
	}
	tokenOff := 20
	if version == 31 {
		tokenOff = 24
	}
	seen := map[int]bool{}
	for i := start; i < start+count; i++ {
		t := types.part(i*88, 88)
		r := metadataReader(t)
		ms, mc := r.u(36), r.h(64)
		for j := 0; j < mc; j++ {
			token := methodTable.u((ms+j)*methodSize+tokenOff) & 0xffffff
			if token > methods {
				methods = token
			}
		}
		if stringsTable.str(r.u(0)) != "ItemType" || stringsTable.str(r.u(4)) != "" || r.u(80)&2 == 0 {
			continue
		}
		fs, fc := r.u(32), r.h(68)
		for j := 0; j < fc; j++ {
			f := fs + j
			symbol := stringsTable.str(fields.u(f * 12))
			if symbol == "value__" {
				continue
			}
			idx, ok := def[f]
			if !ok {
				panic("Missing value for ItemType." + symbol)
			}
			id := compressedInt(values, idx)
			if symbol == "None" {
				continue
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			items = append(items, Item{id, prettyName(symbol), symbol})
		}
	}
	if len(items) == 0 {
		return nil, version, methods, errors.New("ItemType enum not found")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return
}
func prettyName(s string) string {
	var out strings.Builder
	r := []rune(s)
	for i, c := range r {
		if i > 0 && unicode.IsUpper(c) && (unicode.IsLower(r[i-1]) || unicode.IsDigit(r[i-1])) {
			out.WriteByte(' ')
		}
		out.WriteRune(c)
	}
	return out.String()
}
func loadCatalog(root string) (*Catalog, error) {
	root, e := canonical(root)
	if e != nil {
		return nil, e
	}
	st, e := os.Stat(root)
	if e != nil {
		return nil, e
	}
	if !st.IsDir() {
		return nil, errors.New("Select the game folder")
	}
	var dlls, metas []string
	e = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() && len(strings.Split(rel, string(filepath.Separator))) > 5 {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			if strings.EqualFold(d.Name(), "GameAssembly.dll") {
				dlls = append(dlls, p)
			}
			if strings.EqualFold(d.Name(), "global-metadata.dat") {
				metas = append(metas, p)
			}
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	// Prefer the DLL directly in the selected installation, ignoring backup subfolders.
	direct := filepath.Join(root, "GameAssembly.dll")
	if st, e := os.Stat(direct); e == nil && st.Mode().IsRegular() {
		dlls = []string{direct}
	}
	if len(dlls) != 1 || len(metas) != 1 {
		return nil, fmt.Errorf("Expected one GameAssembly.dll and one il2cpp_data/Metadata/global-metadata.dat; found %d and %d. Select the game installation root folder", len(dlls), len(metas))
	}
	b, e := readLimited(metas[0], 256<<20)
	if e != nil {
		return nil, e
	}
	items, v, want, e := parseMetadata(b)
	if e != nil {
		return nil, e
	}
	d, e := readLimited(dlls[0], 512<<20)
	if e != nil {
		return nil, e
	}
	n, e := verifyModule(d, want)
	if e != nil {
		return nil, e
	}
	return &Catalog{items, metas[0], dlls[0], v, n}, nil
}
