package verify_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// S07.2 acceptance: the deterministic pre-gates kill malformed output at
// zero cost (hard kill, one regeneration, never a judge call), and a gate
// TOOL failure fails open to V1 with a logged incident.

func TestV0Gates(t *testing.T) {
	base := verify.Deliverable{Domain: verify.DomainSoftware, Revision: 1}
	cases := []struct {
		name   string
		d      verify.Deliverable
		reason string // "" = pass
	}{
		{name: "clean", d: verify.Deliverable{Revision: 1, Content: "func main() {}\n"}},
		{name: "empty", d: verify.Deliverable{Revision: 1, Content: "   \n"}, reason: "empty"},
		{name: "truncated fence", d: verify.Deliverable{Revision: 1, Content: "ok\n```bash\ncurl example.com"}, reason: "truncated"},
		{name: "placeholder", d: verify.Deliverable{Revision: 1, Content: "func Upload() {\n\t// rest of the implementation goes here\n}"}, reason: "placeholder"},
		{name: "diff-empty", d: verify.Deliverable{Revision: 2, Content: "same", PrevContent: "same"}, reason: "diff-empty"},
		{name: "schema-invalid json", d: verify.Deliverable{Revision: 1, Type: "json", Content: "{broken"}, reason: "shape"},
		{name: "revision 1 never diff-empty", d: verify.Deliverable{Revision: 1, Content: "same", PrevContent: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := base
			d.Type, d.Revision, d.Content, d.PrevContent = tc.d.Type, tc.d.Revision, tc.d.Content, tc.d.PrevContent
			res := verify.RunV0(verify.DefaultPreGates(nil), d)
			if tc.reason == "" {
				if res.Malformed {
					t.Fatalf("clean deliverable killed: %v", res.Reasons)
				}
				return
			}
			if !res.Malformed {
				t.Fatalf("want malformed (%s), got pass", tc.reason)
			}
			found := false
			for _, r := range res.Reasons {
				if strings.HasPrefix(r, tc.reason) {
					found = true
				}
			}
			if !found {
				t.Fatalf("want reason from gate %q, got %v", tc.reason, res.Reasons)
			}
		})
	}
}

func TestV0AllGatesRunOnMalformed(t *testing.T) {
	// One regeneration should learn everything at once: multiple defects
	// are all reported.
	d := verify.Deliverable{Revision: 1, Type: "json", Content: "insert code here ```"}
	res := verify.RunV0(verify.DefaultPreGates(nil), d)
	if len(res.Reasons) < 3 { // truncated + placeholder + shape
		t.Fatalf("want all tripped gates reported, got %v", res.Reasons)
	}
}

func TestV0ToolFailureFailsOpen(t *testing.T) {
	// The gate crashed, not the artifact: no verdict, an incident, the
	// layer fails open (Spec S07.2).
	gates := append(verify.DefaultPreGates(nil), verify.PreGate{
		ID: "crashing", Check: func(verify.Deliverable) (string, error) {
			return "", errors.New("gate binary segfaulted")
		},
	})
	res := verify.RunV0(gates, verify.Deliverable{Revision: 1, Content: "fine content"})
	if res.Malformed {
		t.Fatalf("tool failure must not be a malformed verdict: %v", res.Reasons)
	}
	if len(res.Incidents) != 1 || res.Incidents[0].GateID != "crashing" {
		t.Fatalf("want one logged incident for the crashing gate, got %+v", res.Incidents)
	}
}

func TestV0HardKillNeverJudgesAndCards(t *testing.T) {
	// Malformed artifact, no regeneration seam: the drain terminates in a
	// human-visible card without ever invoking the judge (Spec S07.2 "never
	// a judge call"; S07.7 P46).
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	d := deliverable("t1", "r1")
	d.Content = "// rest of the implementation goes here"
	out, err := v.Verify(context.Background(), input(d))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if j.complianceCalls != 0 || j.sanityCalls != 0 {
		t.Fatalf("V0 hard kill reached the judge: compliance=%d sanity=%d", j.complianceCalls, j.sanityCalls)
	}
	if out.Card == nil || out.Card.Category != verify.CatCapHit {
		t.Fatalf("want CAP-HIT card (regeneration bound), got %+v", out.Card)
	}
	if len(f.events(verify.EventV0)) == 0 {
		t.Fatal("V0 verdict not recorded (Spec S07.11)")
	}
	f.oneOpenCard() // human-visible in the inbox
}

func TestV0RegenerationOnceThenShip(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	regens := 0
	v.Regenerate = func(_ context.Context, d verify.Deliverable, reasons []string) (verify.Deliverable, error) {
		regens++
		if len(reasons) == 0 {
			t.Fatal("regeneration without the V0 reasons")
		}
		d.Content = "package main // regenerated, complete\n"
		return d, nil
	}
	d := deliverable("t1", "r1")
	d.Content = "lorem ipsum"
	out, err := v.Verify(context.Background(), input(d))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if regens != 1 {
		t.Fatalf("want exactly one regeneration (Spec S07.2), got %d", regens)
	}
	if out.Verdict != verify.VerdictShip {
		t.Fatalf("want SHIP after regeneration, got %s", out.Verdict)
	}
}

func TestV0SecondMalformedCards(t *testing.T) {
	// The regeneration also comes back malformed: one regeneration is the
	// bound; the terminal is a card, still no judge call.
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	regens := 0
	v.Regenerate = func(_ context.Context, d verify.Deliverable, _ []string) (verify.Deliverable, error) {
		regens++
		d.Content = "still lorem ipsum"
		return d, nil
	}
	d := deliverable("t1", "r1")
	d.Content = "lorem ipsum"
	out, err := v.Verify(context.Background(), input(d))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if regens != 1 {
		t.Fatalf("regeneration bound is ONE, got %d", regens)
	}
	if j.complianceCalls != 0 {
		t.Fatal("malformed artifact reached the judge")
	}
	if out.Card == nil {
		t.Fatal("second malformed must terminate in a card")
	}
}

func TestV0ToolFailurePipelineProceeds(t *testing.T) {
	// A crashing gate through the full pipeline: fail open to V1/V2, the
	// incident rides the recorded verify.v0 row, and the drain still ships.
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	v.PreGates = append(verify.DefaultPreGates(nil), verify.PreGate{
		ID: "crashing", Check: func(verify.Deliverable) (string, error) {
			return "", errors.New("gate tool crashed")
		},
	})
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictShip {
		t.Fatalf("fail-open drain should ship, got %s", out.Verdict)
	}
	if j.complianceCalls != 1 {
		t.Fatal("fail-open did not reach V2")
	}
	events := f.events(verify.EventV0)
	if len(events) != 1 || !strings.Contains(string(events[0].Payload), "gate tool crashed") {
		t.Fatalf("incident not logged on the verify.v0 row: %v", events)
	}
}
