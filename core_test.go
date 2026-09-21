package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestCryptoRoundTrip(t *testing.T) {
	p := []byte(`{"version":100,"playerData":{"characterGold":999,"ticks":638937474332877123}}`)
	a, e := encrypt(p)
	if e != nil {
		t.Fatal(e)
	}
	b, e := encrypt(p)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Equal(a, b) {
		t.Fatal("salt/IV reused")
	}
	got, e := decrypt(a)
	if e != nil || !bytes.Equal(got, p) {
		t.Fatalf("roundtrip %s %v", got, e)
	}
	for _, offset := range []int{0, 4, 6, 22, 34, 38, 54, len(a) - 1} {
		bad := bytes.Clone(a)
		bad[offset] ^= 1
		if _, e = decrypt(bad); e == nil {
			t.Fatalf("tamper accepted at %d", offset)
		}
	}
	for n := 0; n < 102; n++ {
		if _, e = decrypt(a[:n]); e == nil {
			t.Fatal("short accepted")
		}
	}
}
func TestSaveFixture(t *testing.T) {
	path := filepath.Join("..", "rafaga.jrf")
	if _, e := os.Stat(path); e != nil {
		t.Skip("optional user fixture")
	}
	s, e := loadSave(path)
	if e != nil {
		t.Fatal(e)
	}
	if s.doc["playerData"].(map[string]any)["characterName"] != "rafaga" {
		t.Fatal("name mismatch")
	}
	dir := t.TempDir()
	data, _ := os.ReadFile(path)
	target := filepath.Join(dir, "copy.jrf")
	os.WriteFile(target, data, 0600)
	s, e = loadSave(target)
	if e != nil {
		t.Fatal(e)
	}
	doc, e := applyEdits(s.doc, []Edit{{[]string{"playerData", "characterGold"}, "12345"}})
	if e != nil {
		t.Fatal(e)
	}
	bak, e := s.write(doc)
	if e != nil {
		t.Fatal(e)
	}
	original, _ := os.ReadFile(bak)
	if !bytes.Equal(original, data) {
		t.Fatal("backup mismatch")
	}
	out, e := loadSave(target)
	if e != nil || number(out.doc["playerData"].(map[string]any)["characterGold"]) != 12345 {
		t.Fatal(e)
	}
	os.WriteFile(target, data, 0600)
	if _, e = s.write(doc); e == nil {
		t.Fatal("external changes overwritten")
	}
}
func TestEditsPreserveNumbers(t *testing.T) {
	doc, e := decodeJSON([]byte(`{"playerData":{"ticks":638937474332877123,"gold":5,"a":[1,2],"x":true}}`))
	if e != nil {
		t.Fatal(e)
	}
	out, e := applyEdits(doc, []Edit{{[]string{"playerData", "gold"}, "9"}, {[]string{"playerData", "a", "1"}, "3"}})
	if e != nil {
		t.Fatal(e)
	}
	p := out["playerData"].(map[string]any)
	if p["ticks"].(json.Number) != "638937474332877123" || number(p["gold"]) != 9 {
		t.Fatal(p)
	}
	if number(doc["playerData"].(map[string]any)["gold"]) != 5 {
		t.Fatal("mutated source")
	}
	for _, value := range []string{"NaN", "1 2", "2.5", "null", "\"2\""} {
		if _, e = applyEdits(doc, []Edit{{[]string{"playerData", "gold"}, value}}); e == nil {
			t.Fatal("invalid accepted", value)
		}
	}
}
func TestAddItem(t *testing.T) {
	doc, e := decodeJSON([]byte(`{"playerData":{"characterInventory":[{"Kind":0},{"Kind":0},{"Kind":1,"Item":5,"Amount":1,"Subkind":2,"MaxDurability":20,"Durability":10,"AcquiredOrder":9}]}}`))
	if e != nil {
		t.Fatal(e)
	}
	c := &Catalog{Items: []Item{{ID: 5}}}
	if e = addItem(doc, 5, 2, c); e != nil {
		t.Fatal(e)
	}
	a := doc["playerData"].(map[string]any)["characterInventory"].([]any)
	for i := 0; i < 2; i++ {
		p := a[i].(map[string]any)
		if number(p["Amount"]) != 1 || number(p["Durability"]) != 20 || number(p["AcquiredOrder"]) != int64(10+i) {
			t.Fatal(p)
		}
	}
	if len(a) != 3 {
		t.Fatal("expanded inventory")
	}
	if e = addItem(doc, 5, 1, c); e == nil {
		t.Fatal("full backpack accepted")
	}
	if e = addItem(doc, 999, 1, c); e == nil {
		t.Fatal("unknown ID")
	}
}
func TestPatcherFixture(t *testing.T) {
	original, e := os.ReadFile(filepath.Join("..", "GameAssembly.dll"))
	if e != nil {
		t.Skip("optional DLL fixture")
	}
	p, e := patchedDLL(original, 3, 5)
	if e != nil {
		t.Fatal(e)
	}
	expected, e := os.ReadFile(filepath.Join("..", "GameAssembly_loot_x3_ancient.dll"))
	if e == nil && !bytes.Equal(p, expected) {
		t.Fatal("differs from Python patcher")
	}
	if sha256.Sum256(original) == sha256.Sum256(p) {
		t.Fatal("unchanged")
	}
	q, e := patchedDLL(p, 2.5, 0)
	if e != nil {
		t.Fatal(e)
	}
	for i := range q {
		allowed := false
		for j, o := range patchOffsets {
			if i >= o && i < o+len(originals[j]) {
				allowed = true
			}
		}
		if !allowed && q[i] != original[i] {
			t.Fatal("unexpected change", i)
		}
	}
	bad := bytes.Clone(original)
	bad[patchOffsets[2]] = 0
	if _, e = patchedDLL(bad, 3, 5); e == nil {
		t.Fatal("unknown DLL accepted")
	}
	for _, m := range []float64{0, -1, math.NaN(), math.Inf(1), 1e100, 1e-100} {
		if _, e = patchedDLL(original, m, 5); e == nil {
			t.Fatal("invalid multiplier", m)
		}
	}
}
func TestCatalogFixture(t *testing.T) {
	root := os.Getenv("DIMRAETH_TEST_GAME")
	if root == "" {
		t.Skip("set DIMRAETH_TEST_GAME for real metadata/DLL test")
	}
	c, e := loadCatalog(root)
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Items) != 393 {
		t.Fatalf("items %d", len(c.Items))
	}
	expected := map[int]string{1: "CookedBeastMeat", 4: "StoneWoodcuttingAxe", 35: "Stone", 393: "SpellbookFrenziedSlashes"}
	for _, i := range c.Items {
		if s, ok := expected[i.ID]; ok && s != i.Symbol {
			t.Fatalf("ID %d = %s expected %s", i.ID, i.Symbol, s)
		}
	}
	t.Logf("metadata %d, items %d, methods %d", c.Version, len(c.Items), c.Methods)
}
func TestMetadataMalformed(t *testing.T) {
	for n := 0; n < 400; n++ {
		b := make([]byte, n)
		if _, _, _, e := parseMetadata(b); e == nil {
			t.Fatal("accepted malformed metadata")
		}
	}
	for _, tc := range []struct {
		b []byte
		n int
	}{{[]byte{0}, 0}, {[]byte{2}, 1}, {[]byte{1}, -1}, {[]byte{0x80, 0x80}, 64}, {[]byte{0xff}, -2147483648}} {
		if n := compressedInt(tc.b, 0); n != tc.n {
			t.Fatal(n, tc.n)
		}
	}
}
