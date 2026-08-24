package opencode

// lane_ln2b_test.go — P3-LN-2B drain r2 R4: the two validation gates the lane
// document gained after 2A, refused by name.
//
// Both exist because the failure they prevent is SILENT. A lane with no
// substrate is seated by routing and then dispatched to whichever engine the
// run row was stamped with; a default model the document does not list seats
// routing on a model the lane never declared. Neither produces an error at the
// point of damage, so both are refused at load.

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// laneDocWith renders the shipped seed document with one field edited, so the
// negative cases differ from a KNOWN-GOOD document by exactly one thing.
//
// The seed is read off disk rather than out of a per-file embed variable: the
// lane documents became a DIRECTORY embed at P3-LN-3, so there is no longer one
// []byte per lane to reach for.
func laneDocWith(t *testing.T, edit func(map[string]any)) []byte {
	t.Helper()
	raw, err := os.ReadFile("lanedata/zai.json")
	if err != nil {
		t.Fatalf("read the shipped zai lane document: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode the seed document: %v", err)
	}
	edit(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	return out
}

func TestLaneDocumentRefusesAnUndispatchableOrUnseatableLane(t *testing.T) {
	// The control: unedited, the seed loads.
	if _, err := LoadLaneConfig(laneDocWith(t, func(map[string]any) {})); err != nil {
		t.Fatalf("the unedited seed must load, or the refusals below prove nothing: %v", err)
	}

	for _, tc := range []struct {
		name  string
		edit  func(map[string]any)
		wants []string
	}{
		{
			name:  "no substrate",
			edit:  func(m map[string]any) { delete(m, "substrate") },
			wants: []string{"substrate", "process default"},
		},
		{
			name:  "an empty substrate is not a substrate",
			edit:  func(m map[string]any) { m["substrate"] = "" },
			wants: []string{"substrate"},
		},
		{
			name:  "a default model the document does not list",
			edit:  func(m map[string]any) { m["default_model"] = "glm-not-served" },
			wants: []string{"default_model", "glm-not-served"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadLaneConfig(laneDocWith(t, tc.edit))
			if err == nil {
				t.Fatal("the document loaded — the gate does not exist")
			}
			if !errors.Is(err, ErrLaneConfig) {
				t.Errorf("error = %v, want it to wrap ErrLaneConfig so callers can branch on it", err)
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal does not name %q — an unnamed refusal is one nobody can act on: %v", want, err)
				}
			}
		})
	}

	// An ABSENT default model is allowed: a lane may ship without an execution
	// seat, and routing simply never seats it. Only a WRONG one is refused.
	if _, err := LoadLaneConfig(laneDocWith(t, func(m map[string]any) { delete(m, "default_model") })); err != nil {
		t.Errorf("a document with no default model was refused (%v) — declaring no seat is a valid choice, "+
			"and refusing it would make the gate about something other than correctness", err)
	}
}
