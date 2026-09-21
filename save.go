package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

type Field struct {
	Path  []string `json:"path"`
	Type  string   `json:"type"`
	Value string   `json:"value"`
	Group string   `json:"group"`
}
type Edit struct {
	Path  []string `json:"path"`
	Value string   `json:"value"`
}
type Save struct {
	path string
	hash [32]byte
	doc  map[string]any
}

func decodeJSON(b []byte) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v map[string]any
	if e := d.Decode(&v); e != nil {
		return nil, e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return nil, errors.New("Unexpected data after JSON")
	}
	if _, ok := v["playerData"].(map[string]any); !ok {
		return nil, errors.New("The save file has no playerData object")
	}
	return v, nil
}
func loadSave(path string) (*Save, error) {
	p, e := canonical(path)
	if e != nil {
		return nil, e
	}
	b, e := readLimited(p, maxSave)
	if e != nil {
		return nil, e
	}
	j, e := decrypt(b)
	if e != nil {
		return nil, e
	}
	doc, e := decodeJSON(j)
	if e != nil {
		return nil, e
	}
	return &Save{p, sha256.Sum256(b), doc}, nil
}
func fields(doc any) []Field {
	result := []Field{}
	var walk func(any, []string)
	walk = func(v any, p []string) {
		if len(p) > 100 {
			return
		}
		switch t := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(t[k], append(append([]string{}, p...), k))
			}
		case []any:
			for i, x := range t {
				walk(x, append(append([]string{}, p...), strconv.Itoa(i)))
			}
		default:
			kind, value := "null", "null"
			switch x := v.(type) {
			case string:
				kind, value = "string", x
			case json.Number:
				kind, value = "number", string(x)
			case bool:
				kind, value = "boolean", strconv.FormatBool(x)
			}
			group := "General"
			if len(p) > 0 {
				group = p[0]
			}
			if len(p) > 2 {
				group = strings.Join(p[:2], ".")
			}
			result = append(result, Field{p, kind, value, group})
		}
	}
	walk(doc, nil)
	return result
}
func child(v any, k string) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		x, ok := t[k]
		if ok {
			return x, nil
		}
	case []any:
		i, e := strconv.Atoi(k)
		if e == nil && i >= 0 && i < len(t) {
			return t[i], nil
		}
	}
	return nil, errors.New("Field not found: " + k)
}
func applyEdits(doc map[string]any, edits []Edit) (map[string]any, error) {
	b, e := json.Marshal(doc)
	if e != nil {
		return nil, e
	}
	out, e := decodeJSON(b)
	if e != nil {
		return nil, e
	}
	for _, ed := range edits {
		if len(ed.Path) == 0 || len(ed.Path) > 100 {
			return nil, errors.New("Invalid field path")
		}
		var parent any = out
		for _, k := range ed.Path[:len(ed.Path)-1] {
			parent, e = child(parent, k)
			if e != nil {
				return nil, e
			}
		}
		key := ed.Path[len(ed.Path)-1]
		old, e := child(parent, key)
		if e != nil {
			return nil, e
		}
		var val any
		switch old.(type) {
		case string:
			val = ed.Value
		case bool:
			v, err := strconv.ParseBool(ed.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: expected true or false", key)
			}
			val = v
		case json.Number:
			var n any
			dec := json.NewDecoder(strings.NewReader(ed.Value))
			dec.UseNumber()
			if dec.Decode(&n) != nil {
				return nil, fmt.Errorf("%s: invalid number", key)
			}
			num, ok := n.(json.Number)
			if !ok || dec.Decode(new(any)) != io.EOF {
				return nil, fmt.Errorf("%s: invalid number", key)
			}
			f, err := num.Float64()
			if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
				return nil, fmt.Errorf("%s: number out of range", key)
			}
			if !strings.ContainsAny(string(old.(json.Number)), ".eE") {
				if _, err = num.Int64(); err != nil {
					if _, err = strconv.ParseUint(string(num), 10, 64); err != nil {
						return nil, fmt.Errorf("%s: expected an integer", key)
					}
				}
			}
			val = num
		case nil:
			if ed.Value != "null" {
				return nil, errors.New("Null fields are read-only")
			}
			val = nil
		default:
			return nil, errors.New("Edit individual object fields")
		}
		switch t := parent.(type) {
		case map[string]any:
			t[key] = val
		case []any:
			i, _ := strconv.Atoi(key)
			t[i] = val
		}
	}
	return out, nil
}
func number(v any) int64 {
	if n, ok := v.(json.Number); ok {
		i, _ := n.Int64()
		return i
	}
	return 0
}
func num(n int64) json.Number { return json.Number(strconv.FormatInt(n, 10)) }
func addItem(doc map[string]any, id, quantity int, catalog *Catalog) error {
	if quantity < 1 || quantity > 1000000 {
		return errors.New("Quantity must be an integer between 1 and 1,000,000")
	}
	if catalog == nil {
		return errors.New("Select the game folder first")
	}
	found := false
	for _, it := range catalog.Items {
		if it.ID == id {
			found = true
			break
		}
	}
	if !found {
		return errors.New("The item is not in the game catalog")
	}
	p := doc["playerData"].(map[string]any)
	inv, ok := p["characterInventory"].([]any)
	if !ok {
		return errors.New("The save file has no characterInventory")
	}
	// Reuse a known entry's runtime properties; do not invent item categories from names.
	var template map[string]any
	var order int64
	for _, key := range []string{"characterInventory", "characterEquippedTools", "characterQuickSlots"} {
		a, _ := p[key].([]any)
		for _, x := range a {
			v, ok := x.(map[string]any)
			if !ok {
				continue
			}
			if n := number(v["AcquiredOrder"]); n > order {
				order = n
			}
			if number(v["Kind"]) == 1 && number(v["Item"]) == int64(id) {
				template = v
			}
		}
	}
	if order > math.MaxInt64-int64(quantity) {
		return errors.New("AcquiredOrder overflow")
	}
	// Unknown entries are normalized by Inventory.ValidateInventorySubkinds on load.
	base := map[string]any{"Kind": num(1), "Subkind": num(1), "Item": num(int64(id)), "RuneUUID": "", "PetID": "", "Amount": num(int64(quantity)), "Durability": num(0), "MaxDurability": num(0), "DecayTimer": num(-1), "AcquiredOrder": num(order + 1), "IsEmpty": false, "IsRune": false, "IsPet": false, "IsItemEntry": true, "IsTool": false, "IsBanner": false, "IsRefillable": false, "HasCharges": false}
	if template != nil {
		for k, v := range template {
			base[k] = v
		}
		base["Amount"] = num(int64(quantity))
		base["AcquiredOrder"] = num(order + 1)
		base["Durability"] = base["MaxDurability"]
	}
	// Use existing empty slots, preserving backpack size and other inventories.
	slots := []int{}
	for i, x := range inv {
		v, ok := x.(map[string]any)
		if ok && number(v["Kind"]) == 0 {
			slots = append(slots, i)
		}
	}
	count := 1
	if number(base["Subkind"]) == 2 || number(base["Subkind"]) == 4 {
		count = quantity
	}
	if len(slots) < count {
		return fmt.Errorf("Not enough empty slots: need %d, available %d", count, len(slots))
	}
	for i := 0; i < count; i++ {
		v := map[string]any{}
		for k, x := range base {
			v[k] = x
		}
		v["AcquiredOrder"] = num(order + int64(i) + 1)
		if count > 1 {
			v["Amount"] = num(1)
		}
		inv[slots[i]] = v
	}
	return nil
}
func (s *Save) write(doc map[string]any) (string, error) {
	j, e := json.MarshalIndent(doc, "", "  ")
	if e != nil {
		return "", e
	}
	b, e := encrypt(j)
	if e != nil {
		return "", e
	}
	check, e := decrypt(b)
	if e != nil || !bytes.Equal(check, j) {
		return "", errors.New("Encrypted save verification failed")
	}
	current, e := readLimited(s.path, maxSave)
	if e != nil {
		return "", e
	}
	if sha256.Sum256(current) != s.hash {
		return "", errors.New("The save file has changed on disk. Reload it")
	}
	bak, e := backup(s.path)
	if e != nil {
		return "", e
	}
	if e = replaceChecked(s.path, b, s.hash); e != nil {
		return "", e
	}
	s.doc = doc
	s.hash = sha256.Sum256(b)
	return bak, nil
}
