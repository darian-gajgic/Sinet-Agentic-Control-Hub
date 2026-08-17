package stage

// P3-RW-16 R2 property tests (executor-added, alongside the committed
// acceptance tests): the bounded delimiter completion is swept against
// EVERY byte cut of a rich valid document. Two properties carry the walls:
//
//	P1 — the completion fires only where the cut is a value-complete
//	     resting place OUTSIDE any string (never mid-string, never after a
//	     dangling ',' or ':', never inside a number literal);
//	P2 — a completion NEVER changes a value that was present: whatever
//	     parses out of the completed prefix is contained, leaf for leaf, in
//	     the original document.
//
// P2 is the one that matters: a delimiter appended to a truncated NUMBER
// would produce perfectly valid JSON carrying a different number (12345 cut
// to 1), which is fabrication wearing a syntax check's clothes.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// rw16SweepDoc carries every shape the completion must reason about:
// nested objects and arrays, a string value holding escaped quotes and bare
// braces/brackets, a multi-digit number, an exponent number, true, null,
// and a trailing member after the deep nest.
const rw16SweepDoc = `{"spec":{"restatement":"a \"quoted\" {brace} and [bracket] inside","outcome":["email ready","done"],` +
	`"n":12345,"ok":true,"nil":null,"nested":{"deep":{"deeper":["x",{"y":-1.5e3}]}}},"tail":"end"}`

// rw16Contained reports whether sub is full with (only) trailing content
// missing: every leaf present in sub is byte-identical to full's.
func rw16Contained(sub, full any) bool {
	switch s := sub.(type) {
	case map[string]any:
		f, ok := full.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range s {
			fv, ok := f[k]
			if !ok || !rw16Contained(v, fv) {
				return false
			}
		}
		return true
	case []any:
		f, ok := full.([]any)
		if !ok || len(s) > len(f) {
			return false
		}
		for i := range s {
			if !rw16Contained(s[i], f[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(sub, full)
	}
}

// restingPlace reports whether the prefix ends where a JSON value is
// complete — the only cut points the completion may fire on, stated
// independently of the implementation's own state machine.
func restingPlace(prefix string) bool {
	p := strings.TrimRight(prefix, " \t\r\n")
	if p == "" {
		return false
	}
	switch p[len(p)-1] {
	case '}', ']', '"':
		return true
	}
	return strings.HasSuffix(p, "true") || strings.HasSuffix(p, "false") || strings.HasSuffix(p, "null")
}

func TestRW16CompletionSweepHoldsItsWalls(t *testing.T) {
	var full any
	if err := json.Unmarshal([]byte(rw16SweepDoc), &full); err != nil {
		t.Fatalf("corpus is not valid JSON: %v", err)
	}
	fired := 0
	for k := 1; k <= len(rw16SweepDoc); k++ {
		prefix := rw16SweepDoc[:k]
		var got any
		n, err := parseJSONCompleting(prefix, &got)
		if err != nil {
			if !strings.HasPrefix(err.Error(), "stage: session output is not the contracted JSON object") {
				t.Fatalf("cut %d: error is not the house seam error verbatim: %v", k, err)
			}
			continue
		}
		if k == len(rw16SweepDoc) {
			if n != 0 {
				t.Fatalf("the intact document was completed with %d closers — the completion must not fire on valid JSON", n)
			}
			continue
		}
		fired++
		// P1: only at a value-complete resting place, bounded.
		if !restingPlace(prefix) {
			t.Fatalf("cut %d fired at a non-resting place: %q", k, prefix[max(0, k-24):k])
		}
		if n <= 0 || n > maxJSONCompletionClosers {
			t.Fatalf("cut %d: closer count %d out of bounds", k, n)
		}
		// P2: nothing present was changed.
		if !rw16Contained(got, full) {
			t.Fatalf("cut %d: completion changed a value that was present\n got %#v", k, got)
		}
		// The appended bytes are closing delimiters and nothing else.
		closers, ok := jsonPrefixClosers(strings.TrimSpace(prefix))
		if !ok || len(closers) != n || strings.Trim(closers, "}]") != "" {
			t.Fatalf("cut %d: appended %q, want only closing delimiters (n=%d ok=%v)", k, closers, n, ok)
		}
	}
	// The corpus has 13 value-complete out-of-string cut points; the floor
	// only guards against a sweep that silently stops exercising anything.
	if fired < 12 {
		t.Fatalf("the sweep only fired %d times — the corpus is not exercising the completion", fired)
	}
}

// TestRW16CompletionNeverFiresInsideAString sweeps the cuts that land
// inside the corpus's escaped string value: every one of them needs a QUOTE
// closed, and the platform never closes a quote — the string's content is
// model-authored and its end is unknowable.
func TestRW16CompletionNeverFiresInsideAString(t *testing.T) {
	const val = `"a \"quoted\" {brace} and [bracket] inside"`
	start := strings.Index(rw16SweepDoc, val)
	if start < 0 {
		t.Fatal("corpus lost its escaped string value")
	}
	for k := start + 1; k < start+len(val); k++ {
		var got any
		if _, err := parseJSONCompleting(rw16SweepDoc[:k], &got); err == nil {
			t.Fatalf("cut %d inside a string value was completed: %q", k, rw16SweepDoc[start:k])
		}
	}
}

// TestRW16CompletionNeverFinishesANumber pins the rule the sweep's P2
// property exists for: a truncated number is a DIFFERENT number, so it is
// never a completion candidate — only genuinely missing delimiters are.
func TestRW16CompletionNeverFinishesANumber(t *testing.T) {
	for _, cut := range []string{`{"a":1`, `{"a":123`, `{"a":-1.5e`, `{"a":[1,2`, `{"a":tru`} {
		var got any
		if _, err := parseJSONCompleting(cut, &got); err == nil {
			t.Fatalf("completed a cut literal: %q", cut)
		}
	}
}

// TestRW16CompletionDepthBound pins the ≤8 closer bound: a reply that
// stopped 9 containers deep is not the proven truncation class.
func TestRW16CompletionDepthBound(t *testing.T) {
	doc := func(arrays int) string { return `{"a":` + strings.Repeat("[", arrays) + `"x"` }
	var got any
	if n, err := parseJSONCompleting(doc(7), &got); err != nil || n != 8 { // 1 object + 7 arrays
		t.Fatalf("at the bound: n=%d err=%v, want 8 closers appended", n, err)
	}
	if _, err := parseJSONCompleting(doc(8), &got); err == nil {
		t.Fatal("9 missing closers were appended — the completion must stay bounded")
	}
}

// TestRW16CompletionComposesWithTheFenceRule: R4 and R2 meet on the same
// reply — a string value carrying a ``` fence, once truncated (the live
// shape: bare object, no wrapper fence) and once inside a properly closed
// fenced block.
func TestRW16CompletionComposesWithTheFenceRule(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		closers int
	}{
		{"truncated bare object with a fence in a string",
			`{"a":{"b":"run ` + "```" + `bash here"}`, 1},
		{"intact fenced block with a fence in a string",
			"```json\n" + `{"a":{"b":"run ` + "```" + `bash here"}}` + "\n```", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]any
			n, err := parseJSONCompleting(tc.text, &got)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if n != tc.closers {
				t.Fatalf("closers %d, want %d", n, tc.closers)
			}
			inner, _ := got["a"].(map[string]any)
			if inner["b"] != "run ```bash here" {
				t.Fatalf("string content mangled: %#v", got)
			}
		})
	}
}

// TestRW16CompletionRefusesAnEmptyOpenContainer: a cut immediately after an
// opening bracket recovers NOTHING — completing it would trade the bounded
// re-ask for an empty object the semantic validation then rejects with a
// worse error than the truthful one.
func TestRW16CompletionRefusesAnEmptyOpenContainer(t *testing.T) {
	for _, cut := range []string{`{`, `[`, `{"a":{`, `{"a":1,"c":[`, `{"a":[{`} {
		var got any
		if _, err := parseJSONCompleting(cut, &got); err == nil {
			t.Fatalf("completed a container that holds nothing yet: %q", cut)
		}
	}
}

// TestRW16CompletionIsReported pins R3's plumbing: a fired completion is
// reported exactly once, with its closer count, to the caller that knows
// the run/stage/kind (jsonSession logs the Warn from there); a reply that
// parsed as sent reports nothing.
func TestRW16CompletionIsReported(t *testing.T) {
	t.Run("truncated reply reports its closers once", func(t *testing.T) {
		var reports []int
		var v map[string]any
		err := jsonWithRetryReporting(func(string) (string, error) { return `{"a":{"b":1}`, nil },
			&v, func(n int) { reports = append(reports, n) })
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if !reflect.DeepEqual(reports, []int{1}) {
			t.Fatalf("reports %v, want exactly one report of 1 closer", reports)
		}
	})
	t.Run("intact reply reports nothing", func(t *testing.T) {
		n := 0
		var v map[string]any
		if err := jsonWithRetryReporting(func(string) (string, error) { return `{"a":1}`, nil },
			&v, func(int) { n++ }); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if n != 0 {
			t.Fatalf("%d reports for a reply that needed no completion", n)
		}
	})
	t.Run("a completion on the RE-ASK is reported too", func(t *testing.T) {
		var reports []int
		var v map[string]any
		call := 0
		err := jsonWithRetryReporting(func(string) (string, error) {
			call++
			if call == 1 {
				return "I could not produce it.", nil
			}
			return `{"a":[1,{"b":"x"}`, nil
		}, &v, func(n int) { reports = append(reports, n) })
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if !reflect.DeepEqual(reports, []int{2}) {
			t.Fatalf("reports %v, want one report of 2 closers", reports)
		}
	})
}
