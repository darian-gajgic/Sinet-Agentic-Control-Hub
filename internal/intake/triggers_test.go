package intake

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// The P47 trigger rule layer (Spec S06.3): deterministic, word-boundary,
// seeded with the ratified table.

func TestSeedTriggersDetect(t *testing.T) {
	f := SeedTriggers()
	cases := []struct {
		text string
		rule string
	}{
		{"what is the cheapest plan for X", "P47-1"},
		{"it costs $12 per month", "P47-1"},
		{"is the GDPR relevant here", "P47-3"},
		{"bump the library to a newer version", "P47-4"},
		{"what are the opening hours", "P47-5"},
		{"use the latest release", "P47-7"},
		{"coffee shops near me", "P47-8"},
		{"please look up the spec", "P47-9"},
		{"compare against existing solutions", "P47-10"},
	}
	for _, tc := range cases {
		hits := f.Detect(tc.text)
		found := false
		for _, h := range hits {
			if h.RuleID == tc.rule {
				found = true
				if h.Source != "rule" {
					t.Fatalf("%q: source %q", tc.text, h.Source)
				}
			}
		}
		if !found {
			t.Fatalf("%q: no %s hit (got %v)", tc.text, tc.rule, hits)
		}
	}
}

func TestTriggersWordBoundary(t *testing.T) {
	f := SeedTriggers()
	// "apiary" must not trip P47-4's "api"; "nowhere" must not trip
	// P47-7's "now".
	for _, text := range []string{"visit the apiary", "nowhere to be found"} {
		for _, h := range f.Detect(text) {
			t.Fatalf("%q: unexpected hit %v", text, h)
		}
	}
}

func TestTriggersAtMostOneHitPerRule(t *testing.T) {
	f := SeedTriggers()
	hits := f.Detect("price prices pricing fee fees")
	n := 0
	for _, h := range hits {
		if h.RuleID == "P47-1" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("P47-1 hit %d times, want 1", n)
	}
}

func TestSeedCarriesFullTable(t *testing.T) {
	f := SeedTriggers()
	for i := 1; i <= 11; i++ {
		id := "P47-" + strconv.Itoa(i)
		if f.Rule(id) == nil {
			t.Fatalf("seed missing %s (S06.3 table)", id)
		}
	}
}

func TestLoadTriggersOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p47-triggers.json")
	os.WriteFile(path, []byte(`{"id":"p47-triggers","version":"v2","source":"operator edit","rules":[{"id":"P47-1","class":"Prices & costs","cues":["quid"]}]}`), 0o644)
	f, err := LoadTriggers(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(f.Detect("how many quid is it")) != 1 {
		t.Fatal("operator cue did not match")
	}
	if len(f.Detect("what is the price")) != 0 {
		t.Fatal("override must replace the seed, not extend it")
	}
}
