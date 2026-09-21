package main

import (
	"encoding/json"
	"testing"
)

func TestSynchronizeProgression(t *testing.T) {
	// The unmodified save has level 23, progress 551, and lifetime XP 99872.
	doc, e := decodeJSON([]byte(`{"playerData":{"characterLevel":23,"characterHighestLevel":23,"characterAccumulatedXP":551,"characterAllTimeXP":0,"characterXP":5928,"characterAttributes":[4,4,6,10,4,5,50,8],"unrelated":638937474332877123}}`))
	if e != nil {
		t.Fatal(e)
	}
	out, e := synchronizeProgression(doc)
	if e != nil {
		t.Fatal(e)
	}
	p := out["playerData"].(map[string]any)
	if number(p["characterAllTimeXP"]) != 99872 || number(p["characterXP"]) != 5928 || number(p["unrelated"]) != 638937474332877123 {
		t.Fatal(p)
	}
	if number(doc["playerData"].(map[string]any)["characterAllTimeXP"]) != 0 {
		t.Fatal("source mutated")
	}
	before, _ := json.Marshal(p["characterAttributes"])
	if string(before) != "[4,4,6,10,4,5,50,8]" {
		t.Fatal("attributes changed")
	}
	for _, tc := range []struct {
		level, progress string
		valid           bool
	}{
		{"1", "0", true}, {"1", "249", true}, {"1", "250", false},
		{"25", "500000", true}, {"0", "0", false}, {"26", "0", false},
		{"25", "2147483647", false}, {"2", "-1", false},
	} {
		p["characterLevel"] = json.Number(tc.level)
		p["characterAccumulatedXP"] = json.Number(tc.progress)
		got, err := synchronizeProgression(out)
		if (err == nil) != tc.valid {
			t.Fatalf("%+v: %v", tc, err)
		}
		if err == nil {
			q := got["playerData"].(map[string]any)
			if number(q["characterHighestLevel"]) < number(q["characterLevel"]) {
				t.Fatal("highest level below level")
			}
		}
	}
}

func TestProgressionRequiresKnownFields(t *testing.T) {
	doc, _ := decodeJSON([]byte(`{"playerData":{"characterLevel":10}}`))
	if _, e := synchronizeProgression(doc); e == nil {
		t.Fatal("unknown save schema accepted")
	}
}
