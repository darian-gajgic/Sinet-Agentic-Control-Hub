package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Dialect parser + executor unit battery (Spec S08.9). The journal
// integration (outward steps → gated proposals) is proven end-to-end in
// the worker store suite; here the deterministic core is pinned.

const wfDoc = `{"dialect":"sinet-automation/1","service":"calendar","steps":[
  {"id":"fetch","verb":"calendar.list","args":{"day":{"$from":"payload.day"}}},
  {"id":"post","verb":"calendar.post","args":{"digest":{"$from":"steps.fetch.summary"}},"approval":true}
]}`

func verbs() VerbMap {
	return VerbMap{
		"calendar.list": {Fn: func(_ context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"summary":"digest for ` + strings.Trim(string(args["day"]), `"`) + `"}`), nil
		}},
		"calendar.post": {Outward: true, Class: "C"},
	}
}

func TestParseStrict(t *testing.T) {
	if _, err := Parse(wfDoc); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cases := map[string]string{
		"unknown field":      `{"dialect":"sinet-automation/1","service":"s","steps":[{"id":"a","verb":"s.x"}],"extra":1}`,
		"wrong dialect":      `{"dialect":"sinet-automation/2","service":"s","steps":[{"id":"a","verb":"s.x"}]}`,
		"no service":         `{"dialect":"sinet-automation/1","service":"","steps":[{"id":"a","verb":"s.x"}]}`,
		"no steps":           `{"dialect":"sinet-automation/1","service":"s","steps":[]}`,
		"duplicate id":       `{"dialect":"sinet-automation/1","service":"s","steps":[{"id":"a","verb":"s.x"},{"id":"a","verb":"s.y"}]}`,
		"foreign verb":       `{"dialect":"sinet-automation/1","service":"s","steps":[{"id":"a","verb":"other.x"}]}`,
		"trailing content":   `{"dialect":"sinet-automation/1","service":"s","steps":[{"id":"a","verb":"s.x"}]} extra`,
		"unknown step field": `{"dialect":"sinet-automation/1","service":"s","steps":[{"id":"a","verb":"s.x","permissionMode":"y"}]}`,
	}
	for name, doc := range cases {
		if _, err := Parse(doc); !errors.Is(err, ErrDialect) {
			t.Errorf("%s: err = %v, want ErrDialect", name, err)
		}
	}
}

func TestValidateApprovalNodes(t *testing.T) {
	wf, err := Parse(wfDoc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := Validate(wf, verbs()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// Outward without approval refuses.
	noApproval, err := Parse(strings.Replace(wfDoc, `,"approval":true`, "", 1))
	if err != nil {
		t.Fatalf("Parse(noApproval): %v", err)
	}
	if err := Validate(noApproval, verbs()); !errors.Is(err, ErrUnapprovedOutward) {
		t.Fatalf("Validate(noApproval): %v, want ErrUnapprovedOutward", err)
	}
	// Unknown verb refuses.
	v := verbs()
	delete(v, "calendar.post")
	if err := Validate(wf, v); !errors.Is(err, ErrUnknownVerb) {
		t.Fatalf("Validate(unknown verb): %v, want ErrUnknownVerb", err)
	}
}

func TestExecuteReadChainAndOutwardGuard(t *testing.T) {
	wf, err := Parse(wfDoc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Outward step with no journal wired: refused, nothing executes
	// blindly.
	_, err = Execute(context.Background(), ExecInput{
		Workflow: wf, Payload: json.RawMessage(`{"day":"mon"}`), Verbs: verbs(), UserID: "u",
	})
	if err == nil || !strings.Contains(err.Error(), "no effect journal") {
		t.Fatalf("outward without journal: %v", err)
	}

	// Read-only chain executes deterministically with $from resolution.
	readOnly, err := Parse(`{"dialect":"sinet-automation/1","service":"calendar","steps":[
	  {"id":"fetch","verb":"calendar.list","args":{"day":{"$from":"payload.day"}}}]}`)
	if err != nil {
		t.Fatalf("Parse(readOnly): %v", err)
	}
	rep, err := Execute(context.Background(), ExecInput{
		Workflow: readOnly, Payload: json.RawMessage(`{"day":"mon"}`), Verbs: verbs(), UserID: "u",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(rep.Steps) != 1 || rep.Steps[0].Kind != "executed" ||
		!strings.Contains(string(rep.Steps[0].Output), "digest for mon") {
		t.Fatalf("report = %+v", rep)
	}

	// A $from miss fails loudly.
	_, err = Execute(context.Background(), ExecInput{
		Workflow: readOnly, Payload: json.RawMessage(`{"other":"x"}`), Verbs: verbs(), UserID: "u",
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing payload key: %v", err)
	}
}
