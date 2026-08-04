package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker/automation"
)

// kind=automation end-to-end (Spec S08.9): same schema, same lifecycle,
// same battery — the body is a dialect document, station 3 is a test
// execution with outward effects as gated proposals, and NO model is ever
// in the loop.

const automationSrc = `---
name: calendar-digest
description: Posts a daily digest of calendar events by email
kind: automation
domain: chore
selectors:
  family: connector-automation
equipment:
  connectors: [calendar]
---
{"dialect":"sinet-automation/1","service":"calendar","steps":[
  {"id":"fetch","verb":"calendar.list","args":{"day":{"$from":"payload.day"}}},
  {"id":"post","verb":"calendar.post","args":{"digest":{"$from":"steps.fetch.summary"}},"approval":true}
]}
`

func automationGrants() worker.RequestedGrants {
	return worker.RequestedGrants{
		Tools: []string{"calendar.list", "calendar.post"}, Class: "C0",
		Egress: worker.EgressSingleHost, EgressHosts: []string{"calendar.example.com"},
	}
}

func calendarVerbs() automation.VerbMap {
	return automation.VerbMap{
		"calendar.list": {Fn: func(_ context.Context, args map[string]json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"summary":"3 events on ` + strings.Trim(string(args["day"]), `"`) + `"}`), nil
		}},
		"calendar.post": {Outward: true, Class: gates.ClassC},
	}
}

func TestAutomationLifecycleNoModelInLoop(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	f.user("op", "operator")
	ctx := context.Background()
	journal := f.journal()

	if err := f.store.CreateDomain(ctx, "op", "chore"); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}
	_, v, err := f.store.CreateDraft(ctx, "alice", automationSrc, automationGrants(), humanProv())
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	// Battery: NO DryEngine — station 3 for an automation is the sample-
	// payload test execution; the validation record keys on the dialect
	// version in the engine-pin slot (Spec S08.1, S08.9).
	res, err := f.store.RunBattery(ctx, v.ID, worker.BatteryInput{
		Actor: "alice", SampleTask: `{"day":"2026-07-20"}`,
		Verbs: calendarVerbs(), Journal: journal,
	})
	if err != nil {
		t.Fatalf("RunBattery: %v", err)
	}
	if !res.Green {
		t.Fatalf("automation battery red: %+v %+v", res.Lint, res.Audit)
	}
	if res.EnginePin != automation.DialectVersion || res.Model != "" {
		t.Fatalf("record key = (%q, %q), want dialect version in the pin slot", res.Model, res.EnginePin)
	}
	// The test execution journaled the outward step as a proposal —
	// nothing executed (Spec S08.9 station 3; D7 makes this free).
	proposals, err := journal.InState(ctx, gates.EffectProposed)
	if err != nil {
		t.Fatalf("InState: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("dry execution proposals = %d, want 1", len(proposals))
	}

	// Approval and on-demand execution (v0 surface).
	if _, err := f.store.Approve(ctx, "alice", v.ID, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	rep, err := f.store.RunAutomation(ctx, "alice", v.TemplateID, json.RawMessage(`{"day":"2026-07-21"}`), calendarVerbs(), journal)
	if err != nil {
		t.Fatalf("RunAutomation: %v", err)
	}
	if len(rep.Steps) != 2 || rep.Steps[0].Kind != "executed" || rep.Steps[1].Kind != "proposed" {
		t.Fatalf("report = %+v", rep)
	}
	if !strings.Contains(string(rep.Steps[0].Output), "2026-07-21") {
		t.Fatalf("payload did not flow into the read verb: %s", rep.Steps[0].Output)
	}
	proposals, err = journal.InState(ctx, gates.EffectProposed)
	if err != nil {
		t.Fatalf("InState: %v", err)
	}
	if len(proposals) != 2 {
		t.Fatalf("proposals after real run = %d, want 2", len(proposals))
	}
	// The outward proposal carries the resolved args, chained from the
	// read step's output.
	var body struct {
		Verb string                     `json:"verb"`
		Args map[string]json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(proposals[0].Payload, &body); err != nil {
		t.Fatalf("proposal payload: %v", err)
	}
	if body.Verb != "calendar.post" || !strings.Contains(string(body.Args["digest"]), "3 events") {
		t.Fatalf("proposal payload wrong: %s", proposals[0].Payload)
	}

	if !strings.Contains(strings.Join(f.eventTypes(), ","), "worker.automation_run") {
		t.Fatalf("automation run not on the event trail")
	}
}

func TestAutomationDialectRejections(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	f.user("op", "operator")
	ctx := context.Background()
	if err := f.store.CreateDomain(ctx, "op", "chore"); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	// An outward step without its explicit approval node is a red battery
	// (Spec S08.9: explicit approval nodes for outward effects).
	src := strings.Replace(automationSrc, `,"approval":true`, "", 1)
	_, v, err := f.store.CreateDraft(ctx, "alice", src, automationGrants(), humanProv())
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	res, err := f.store.RunBattery(ctx, v.ID, worker.BatteryInput{
		Actor: "alice", SampleTask: `{"day":"d"}`, Verbs: calendarVerbs(), Journal: f.journal(),
	})
	if err != nil {
		t.Fatalf("RunBattery: %v", err)
	}
	if res.Green {
		t.Fatalf("unapproved outward step passed the battery")
	}
	dialectErr := false
	for _, e := range res.Lint.Errors {
		if e.Code == worker.FindingDialect && strings.Contains(e.Message, "approval") {
			dialectErr = true
		}
	}
	if !dialectErr {
		t.Fatalf("no dialect finding for the missing approval node: %+v", res.Lint.Errors)
	}

	// Compiling an automation onto an engine is structurally refused (no
	// model in the loop).
	if _, err := f.store.Approve(ctx, "alice", v.ID, worker.ApproveOpts{}); !errors.Is(err, worker.ErrNotValidated) {
		t.Fatalf("red approve: %v", err)
	}
}
