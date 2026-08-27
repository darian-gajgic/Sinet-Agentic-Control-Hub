package stage

// P3-RW-16 acceptance tests (grounding-committed RED, Amendment-A window):
// the plan-stage "session output is not the contracted JSON object:
// unexpected end of JSON input" crash class. Root cause (drain-r1 evidence,
// lineage t-611e038e722d95ed, 2026-08-17): the engine's own transcripts show
// the planner's final assistant text ending exactly one closing brace short
// at stop_reason end_turn (6 of 11 plan-stage sessions in the walk) — the
// missing byte never reached the adapter, so the fix lives HERE, at the
// stage's JSON-contract seam (parseJSONOutput), never in the adapter
// (Outcome.ResultText: verbatim, never repaired — RW-9).
//
// Contract under test (brief P3/briefs/P3-RW-16.md R2/R4):
//   - a truncation-class failure whose only missing bytes are closing
//     delimiters (computable from the open-bracket stack, EOF outside any
//     string at a value-complete position) is completed and parsed; the
//     caller's semantic validation downstream is unchanged;
//   - anything else (empty, mid-string cut, after-comma cut, prose,
//     quote-close needed) keeps today's loud error;
//   - a ``` fence INSIDE a JSON string value is content, never a fence.

import (
	"strings"
	"testing"
)

// rw16FullPair is a compact valid pair-shaped document mirroring the
// crashed sessions' shape: the document ends in three consecutive closing
// braces (basis string -> est -> plan -> pair), the depth profile the
// engine repeatedly got one brace wrong on.
const rw16FullPair = `{"spec":{"restatement":"Draft a GPU restock announcement","outcome":["email ready"],"acs":[{"n":1,"plain":"email drafted"}],"constraints":[],"assumptions":[],"out_of_scope":[],"clarifications":[]},"plan":{"steps":[{"id":"S-1","title":"draft","done_when":"exists","class":"C1","approach":"I draft the announcement in one pass and keep it to a few short paragraphs."}],"coverage":{"AC-1":["S-1"]},"risks":[],"est":{"size_class":"S","usd":0,"known":true,"basis":"content drafting only."}}}`

type rw16Pair struct {
	Spec struct {
		Restatement string `json:"restatement"`
	} `json:"spec"`
	Plan struct {
		Est struct {
			Basis string `json:"basis"`
		} `json:"est"`
	} `json:"plan"`
}

// TestRW16ParseJSONOutputRecoversDelimiterOnlyTruncation is the packet's
// heart (RED until R2 lands): the exact evidence signature — a valid JSON
// prefix missing only its closing delimiters — parses, with deep content
// intact for the downstream semantic validation to judge.
func TestRW16ParseJSONOutputRecoversDelimiterOnlyTruncation(t *testing.T) {
	arrayTail := `{"kind":"revise","findings":["f1","f2"` // needs "]}": mixed bracket+brace
	cases := []struct {
		name  string
		text  string
		basis string // expected surviving deep field ("" = skip)
	}{
		{"one brace short (the drain-r1 signature)", rw16FullPair[:len(rw16FullPair)-1], "content drafting only."},
		{"two braces short", rw16FullPair[:len(rw16FullPair)-2], "content drafting only."},
		{"array then object closers", arrayTail, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var into map[string]any
			if err := parseJSONOutput(tc.text, &into); err != nil {
				t.Fatalf("parseJSONOutput on delimiter-only truncation: %v (R2: bounded structural completion)", err)
			}
			if tc.basis != "" {
				var p rw16Pair
				if err := parseJSONOutput(tc.text, &p); err != nil {
					t.Fatalf("re-parse into pair shape: %v", err)
				}
				if p.Plan.Est.Basis != tc.basis {
					t.Fatalf("deep field lost across completion: got %q want %q", p.Plan.Est.Basis, tc.basis)
				}
			}
		})
	}
}

// TestRW16ParseJSONOutputCompletionBounds guards the completion's walls
// (GREEN today and after: these must keep failing loudly — the completion
// appends closing delimiters only, never quotes, values, or commas, and
// never invents content for an empty reply).
func TestRW16ParseJSONOutputCompletionBounds(t *testing.T) {
	midString := rw16FullPair[:strings.Index(rw16FullPair, "content draft")+7]
	cases := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"whitespace", "   \n\t"},
		{"prose", "I could not produce the plan."},
		{"cut mid-string (quote close needed)", midString},
		{"cut after comma (not value-complete)", `{"a":1,`},
		{"cut after colon (not value-complete)", `{"a":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var into map[string]any
			if err := parseJSONOutput(tc.text, &into); err == nil {
				t.Fatalf("parseJSONOutput accepted %s input — the completion must stay delimiter-only (R2 bounds)", tc.name)
			}
		})
	}
}

// TestRW16ParseJSONOutputFencedFormsStillParse pins today's working forms
// (GREEN today and after): fenced output, prose-then-fenced output, and
// leading prose before a bare object.
func TestRW16ParseJSONOutputFencedFormsStillParse(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"bare object", rw16FullPair},
		{"fenced", "```json\n" + rw16FullPair + "\n```"},
		{"prose then fenced", "Here is the pair:\n```json\n" + rw16FullPair + "\n```"},
		{"leading prose then bare object", "Here is the pair: " + rw16FullPair},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p rw16Pair
			if err := parseJSONOutput(tc.text, &p); err != nil {
				t.Fatalf("parseJSONOutput: %v", err)
			}
			if p.Plan.Est.Basis != "content drafting only." {
				t.Fatalf("deep field lost: %q", p.Plan.Est.Basis)
			}
		})
	}
}

// TestRW16ParseJSONOutputFenceInsideStringIsContent (RED until R4 lands):
// a ``` occurring inside a JSON string value is content — today's
// fence-stripping cuts the document at it and destroys a perfectly valid
// reply (same "not the contracted JSON object" signature, different door).
func TestRW16ParseJSONOutputFenceInsideStringIsContent(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"one fence in a string value",
			`{"plan":{"steps":[{"id":"S-1","done_when":"README gains a ` + "```" + `bash fence block"}]}}`},
		{"paired fences in a string value",
			`{"plan":{"steps":[{"id":"S-1","done_when":"wrap the code in ` + "```" + ` and close with ` + "```" + ` after"}]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var into map[string]any
			if err := parseJSONOutput(tc.text, &into); err != nil {
				t.Fatalf("parseJSONOutput mangled a valid object carrying a fence inside a string value: %v (R4)", err)
			}
		})
	}
}
