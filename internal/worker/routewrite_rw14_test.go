package worker_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// P3-RW-14 R8/R9 acceptance — the S08.8 step-1 filter learns the plan's write
// posture, and a vacuous description echo stops manufacturing a preference.
//
// The live defect both bind: a webshop task whose every plan step declared a
// non-empty write_set routed to "release-notes-writer" — a template granted
// {Read, Grep, Glob} at class C1, which structurally cannot write a file — on
// a bm25 rank of -0.00. The worker then spent $1.11 arguing with its own brief
// and wrote nothing. Two independent failures, so two independent fixes: R8
// refuses equipment that cannot do the declared job (whatever its rank), R9
// stops a rank of ~0 from being read as evidence of fit.

// tmplSrc builds an agentic template source with the given identity, selector
// family and toolset.
func tmplSrc(name, desc, family, tools string) string {
	return `---
name: ` + name + `
description: ` + desc + `
kind: agentic
domain: software
selectors:
  family: ` + family + `
  task_classes: [webshop]
  triggers: [build the shop]
profile:
  duty: execution
  effort_floor: standard
equipment:
  tools: [` + tools + `]
persona: [Terse and precise.]
---
Carry out the presented plan step inside the workspace and report what
changed, with file paths.
`
}

// activeTemplate drives a custom template to ACTIVE and returns it.
func activeTemplate(t *testing.T, f *fix, actor, name, desc, family, tools, class string) worker.Template {
	t.Helper()
	ctx := context.Background()
	list := strings.Split(tools, ", ")
	tpl, v, err := f.store.CreateDraft(ctx, actor, tmplSrc(name, desc, family, tools),
		worker.RequestedGrants{Tools: list, Class: class, Egress: worker.EgressNone}, humanProv())
	if err != nil {
		t.Fatalf("CreateDraft(%s): %v", name, err)
	}
	res, err := f.store.RunBattery(ctx, v.ID, worker.BatteryInput{
		Actor: actor, SampleTask: "carry out the sample step", Engine: &fakeDry{},
		Model: "claude-haiku-4-5", EnginePin: "claude-cli@2.1.215",
	})
	if err != nil {
		t.Fatalf("RunBattery(%s): %v", name, err)
	}
	if !res.Green {
		t.Fatalf("battery not green for %s: %+v", name, res)
	}
	// Write instruments and a C2 confinement are above-ceiling grants: the
	// approver acknowledges them explicitly (S08.6 station 2), exactly as a
	// human would on the approval card.
	if _, err := f.store.Approve(ctx, actor, v.ID,
		worker.ApproveOpts{AckFlagged: res.Audit.FlaggedItems()}); err != nil {
		t.Fatalf("Approve(%s): %v", name, err)
	}
	return tpl
}

// TestRW14WritingPlanRefusesReadOnlyWorker: a plan that declares writes may
// only be taken by equipment that can write. The property is over the whole
// (class, tools) space — every write-incapable combination is refused, not
// just the one shape the live world happened to produce.
func TestRW14WritingPlanRefusesReadOnlyWorker(t *testing.T) {
	const owner = "alice"

	// Every candidate below matches the query's family selector (+2), so it
	// is a genuine candidate on every axis EXCEPT its equipment: whatever the
	// filter does is attributable to the write posture alone.
	cases := []struct {
		name      string
		class     string
		tools     string
		canWrite  bool
		whyRefuse string
	}{
		{
			name: "the live shape: read-only class, no write instrument",
			class: "C1", tools: "Read, Grep, Glob",
			whyRefuse: "neither its class nor its tools can write",
		},
		{
			name: "write instruments under a read-only class",
			class: "C1", tools: "Read, Write, Edit",
			whyRefuse: "a C1 workspace is mounted read-only however good the tools are",
		},
		{
			name: "a writable class with no write instrument",
			class: "C2", tools: "Read, Grep, Glob",
			whyRefuse: "nothing in the granted toolset can put bytes on disk",
		},
		{
			name: "fully equipped", class: "C2", tools: "Read, Write, Edit",
			canWrite: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newFix(t)
			f.user(owner, "operator")
			activeTemplate(t, f, owner, "shop-builder",
				"Builds and edits webshop application source files", "build-create", tc.tools, tc.class)
			r := newRouter(f)

			// The plan's declared envelope is the candidate's own class, so
			// the ratified equal-or-tighter filter never decides this test.
			q := worker.RouteQuery{
				Requester: owner, TaskID: "t-shop",
				TaskText: "build the shop checkout flow",
				Family:   "build-create", Domain: "software",
				Kind:    worker.KindAgentic,
				Classes: []string{tc.class},
				Writes:  true,
			}
			d, err := r.Route(ctx, q)
			if err != nil {
				t.Fatalf("Route: %v", err)
			}

			if tc.canWrite {
				if d.Generalist {
					t.Fatalf("equipped worker was refused a writing plan: %s", d.PlainReason)
				}
				if d.TemplateName != "shop-builder" {
					t.Fatalf("routed to %q, want the equipped specialist", d.TemplateName)
				}
				return
			}

			if !d.Generalist {
				t.Fatalf("a worker that cannot write took a writing plan (%s): routed to %q",
					tc.whyRefuse, d.TemplateName)
			}
			if !strings.Contains(strings.ToLower(d.PlainReason), "write") {
				t.Errorf("plain reason never names the refusal (%s): %q", tc.whyRefuse, d.PlainReason)
			}
		})
	}

	// The gate fires on PLATFORM facts only: the same read-only worker still
	// takes the same task when the plan declares no writes. A filter that
	// refused it either way would be a bug, not a fix.
	t.Run("a non-writing plan still routes to the read-only worker", func(t *testing.T) {
		ctx := context.Background()
		f := newFix(t)
		f.user(owner, "operator")
		activeTemplate(t, f, owner, "shop-reader",
			"Reviews webshop application source files", "read-analyze", "Read, Grep, Glob", "C1")
		r := newRouter(f)

		d, err := r.Route(ctx, worker.RouteQuery{
			Requester: owner, TaskID: "t-read",
			TaskText: "review the shop checkout flow",
			Family:   "read-analyze", Domain: "software",
			Kind:    worker.KindAgentic,
			Classes: []string{"C1"},
			Writes:  false,
		})
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		if d.Generalist {
			t.Fatalf("a read-only worker was refused a read-only plan: %s", d.PlainReason)
		}
	})
}

// TestRW14ZeroRankEchoIsNotAMatch: the FTS lane must contribute in proportion
// to what bm25 actually found. When a query term appears in EVERY indexed
// description its inverse document frequency is ~0 and SQLite returns a rank
// of ~-0.000001 — measured, not assumed — which says "this word does not
// distinguish anyone", the exact opposite of a match. A candidate whose only
// signal is that echo must keep score 0 and lose to the generalist default.
func TestRW14ZeroRankEchoIsNotAMatch(t *testing.T) {
	ctx := context.Background()
	const owner = "alice"
	f := newFix(t)
	f.user(owner, "operator")

	// Both descriptions carry "changelogs", so the term discriminates nobody.
	// Neither template's family/triggers/task-classes match the query, so the
	// FTS echo is the ONLY signal either can offer.
	activeTemplate(t, f, owner, "release-notes-writer",
		"Writes release notes and changelogs from merged work", "write-compose", "Read, Grep, Glob", "C1")
	activeTemplate(t, f, owner, "changelog-formatter",
		"Formats changelogs into the house style", "write-compose", "Read, Grep, Glob", "C1")

	r := newRouter(f)
	d, err := r.Route(ctx, worker.RouteQuery{
		Requester: owner, TaskID: "t-echo",
		TaskText: "changelogs",
		// A family nothing declares: no selector can fire.
		Family: "read-analyze", Domain: "software",
		Kind:    worker.KindAgentic,
		Classes: []string{"C1"},
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !d.Generalist {
		t.Fatalf("a vacuous description echo manufactured a specialist preference: routed to %q, score %.4f, reason %q",
			d.TemplateName, d.Score, d.PlainReason)
	}

	// And the lane is not merely switched off: a term that genuinely
	// discriminates still earns its bounded contribution, and the reason
	// still prints the rank it was earned on.
	for i := 0; i < 8; i++ {
		activeTemplate(t, f, owner, fmt.Sprintf("filler-%d", i),
			fmt.Sprintf("Handles routine changelogs chore number %d", i), "write-compose", "Read, Grep, Glob", "C1")
	}
	activeTemplate(t, f, owner, "kubernetes-specialist",
		"Diagnoses kubernetes deployment rollouts", "write-compose", "Read, Grep, Glob", "C1")

	d2, err := r.Route(ctx, worker.RouteQuery{
		Requester: owner, TaskID: "t-real",
		TaskText: "kubernetes",
		Family:   "read-analyze", Domain: "software",
		Kind:    worker.KindAgentic,
		Classes: []string{"C1"},
	})
	if err != nil {
		t.Fatalf("Route(discriminating): %v", err)
	}
	if d2.Generalist {
		t.Fatalf("a genuinely discriminating description match was thrown away: %s", d2.PlainReason)
	}
	if d2.TemplateName != "kubernetes-specialist" {
		t.Fatalf("routed to %q, want the discriminating match", d2.TemplateName)
	}
	if !strings.Contains(d2.PlainReason, "rank") {
		t.Errorf("plain reason stopped printing the rank it matched on: %q", d2.PlainReason)
	}
}
