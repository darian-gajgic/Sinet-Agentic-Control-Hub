package verify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// V0 — deterministic pre-gates (Spec S07.2): free, every domain, always.
// Empty, truncated, placeholder-marker, diff-empty, and schema-invalid
// output dies instantly at zero cost, plus artifact-shape checks per
// deliverable type. The layer is load-bearing, not hygiene:
// superficial/truncated/placeholder output is precisely the input class
// model judges most reliably rubber-stamp, so the free gate removes exactly
// what the paid gate cannot be trusted with.
//
// Two outcome classes, ratified apart (Spec S07.2):
//   - a V0 VERDICT of "artifact malformed" is a hard kill — it costs one
//     regeneration, never a judge call;
//   - a V0 TOOL FAILURE (the gate crashed, not the artifact) fails OPEN to
//     V1 with a logged incident: a screen never blocks on its own failure,
//     a screen outage escalates rather than approves, and no screen's SHIP
//     is ever final.

// PreGate is one deterministic structural check. Check returns
// (reason, err): reason != "" is a malformed verdict (hard kill); err != nil
// is a tool failure (fails open, incident logged); both zero is a pass.
type PreGate struct {
	ID    string
	Check func(d Deliverable) (reason string, err error)
}

// V0Incident is one logged gate tool failure (fail-open evidence).
type V0Incident struct {
	GateID string `json:"gate_id"`
	Error  string `json:"error"`
}

// V0Result is the V0 layer outcome.
type V0Result struct {
	// Malformed is the hard-kill verdict; Reasons lists every tripped gate
	// (all gates run — one regeneration should fix everything found).
	Malformed bool     `json:"malformed"`
	Reasons   []string `json:"reasons,omitempty"`
	// Incidents are gate tool failures: the layer failed OPEN to V1 for
	// those gates, and each is a logged incident (Spec S07.2).
	Incidents []V0Incident `json:"incidents,omitempty"`
	Passed    []string     `json:"passed,omitempty"`
}

// PlaceholderMarkers is the placeholder-marker lexicon (matched
// case-insensitively). Deliberately conservative: markers that only appear
// in stand-in output, not phrases (TODO/FIXME) routine in legitimate code —
// a false V0 kill costs a regeneration, so the lexicon errs toward the
// unambiguous class the gate exists for (Spec S07.2).
var PlaceholderMarkers = []string{
	"lorem ipsum",
	"<placeholder",
	"[placeholder",
	"placeholder text",
	"your code here",
	"your text here",
	"insert code here",
	"insert text here",
	"implementation omitted",
	"implementation goes here",
	"rest of the implementation",
	"left as an exercise",
}

// ShapeCheck is one per-type artifact-shape check (Spec S07.2 "plus
// artifact-shape checks per deliverable type").
type ShapeCheck func(d Deliverable) (reason string, err error)

// DefaultShapeChecks maps deliverable types to their shape checks. The map
// is extended per deliverable type as types arrive (S13 owns the type
// registry, B4).
func DefaultShapeChecks() map[string]ShapeCheck {
	return map[string]ShapeCheck{
		"json": func(d Deliverable) (string, error) {
			if !json.Valid([]byte(d.Content)) {
				return "schema-invalid: content is not valid JSON", nil
			}
			return "", nil
		},
	}
}

// DefaultPreGates returns the ratified V0 gate set (Spec S07.2): empty,
// truncated, placeholder-marker, diff-empty, schema/shape.
func DefaultPreGates(shapes map[string]ShapeCheck) []PreGate {
	if shapes == nil {
		shapes = DefaultShapeChecks()
	}
	return []PreGate{
		{ID: "empty", Check: func(d Deliverable) (string, error) {
			if strings.TrimSpace(d.Content) == "" {
				return "empty: deliverable content is empty", nil
			}
			return "", nil
		}},
		{ID: "truncated", Check: func(d Deliverable) (string, error) {
			// Deterministic structural truncation signal: an odd number of
			// ``` fences means a code block was opened and never closed —
			// the classic mid-output cut. Per-type schema checks catch the
			// typed cases (a truncated JSON is schema-invalid).
			if strings.Count(d.Content, "```")%2 == 1 {
				return "truncated: unbalanced code fence (output cut mid-block)", nil
			}
			return "", nil
		}},
		{ID: "placeholder", Check: func(d Deliverable) (string, error) {
			lower := strings.ToLower(d.Content)
			for _, m := range PlaceholderMarkers {
				if strings.Contains(lower, m) {
					return fmt.Sprintf("placeholder-marker: %q", m), nil
				}
			}
			return "", nil
		}},
		{ID: "diff-empty", Check: func(d Deliverable) (string, error) {
			// A revision > 1 that changed nothing is a non-answer to the
			// findings it was regenerated against (Spec S07.2 diff-empty).
			if d.Revision > 1 && d.PrevContent != "" && d.Content == d.PrevContent {
				return "diff-empty: revision is byte-identical to the previous revision", nil
			}
			return "", nil
		}},
		{ID: "wrote-nothing", Check: func(d Deliverable) (string, error) {
			// The repo-backed sibling of diff-empty (Spec S07.2). The
			// byte-comparison gate above cannot see this class at all:
			// revision 1 has no previous revision, so an execute leg that
			// ran, completed, billed, and moved not one byte of the tree
			// passed straight through to the paid layers. S13.1 names
			// revision 1's basis — the pre-task state — and both terms are
			// durable PLATFORM facts (the snapshot ledger and the attempt's
			// base ref), never anything the engine reported about itself.
			//
			// Guarded on all three: no snapshot pin means not repo-backed,
			// no base means the basis is unknown (never guess one), and no
			// write claim means a read-only plan may legitimately move
			// nothing (Spec S02.8).
			if d.WriteClaimed && d.SnapshotSHA != "" && d.BaseSHA != "" && d.SnapshotSHA == d.BaseSHA {
				return "execution wrote nothing to the claimed write set", nil
			}
			return "", nil
		}},
		{ID: "shape", Check: func(d Deliverable) (string, error) {
			if chk, ok := shapes[d.Type]; ok {
				return chk(d)
			}
			return "", nil
		}},
	}
}

// RunV0 runs the pre-gates over a deliverable. Every gate runs (a
// regeneration should learn everything at once); a gate tool failure is
// collected as an incident and the layer fails open for that gate — it
// neither blocks nor approves (Spec S07.2).
func RunV0(gates []PreGate, d Deliverable) V0Result {
	var res V0Result
	for _, g := range gates {
		reason, err := g.Check(d)
		switch {
		case err != nil:
			res.Incidents = append(res.Incidents, V0Incident{GateID: g.ID, Error: err.Error()})
		case reason != "":
			res.Malformed = true
			res.Reasons = append(res.Reasons, g.ID+": "+reason)
		default:
			res.Passed = append(res.Passed, g.ID)
		}
	}
	return res
}
