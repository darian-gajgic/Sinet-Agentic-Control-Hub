package shell

// checkpack_rw14_test.go — P3-RW-14 R5: the wire from the S13.7 registry
// capture to the S07.3 check pack.
//
// The live world's whole software family crashed at verify because this wire
// did not exist: `stage.Config.CheckPacks` shipped empty, every launch-domain
// deliverable hit ErrNoCheckPack, and the ladder forked the deterministic
// refusal to a tombstone. What the resolver must NOT do is equally load-
// bearing: with no commands captured it invents nothing and degrades nothing —
// it names exactly what to capture, and that sentence becomes the operator's
// card (OQ3(i)).

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// registeredProject registers one entry with the given commands and returns it
// as the registry stores and reads it back (the real JSON round-trip).
func registeredProject(t *testing.T, cmds project.Commands) project.Entry {
	t.Helper()
	ctx := context.Background()
	db, log, _ := seamDB(t)
	proj, err := project.New(project.Config{DB: db, Log: log, Root: t.TempDir()})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	if _, err := proj.Register(ctx, project.RegisterInput{
		ProjectID: "shop", Owner: "u1", Name: "shop", StorePath: t.TempDir(),
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := proj.Capture(ctx, project.CaptureInput{
		ProjectID: "shop", By: "u1", Commands: cmds, Family: "software",
	}); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	e, err := proj.Get(ctx, "shop")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return e
}

// TestRW14CapturedCommandsBecomeTheLadderRungs: the captured commands land on
// the S07.3 rungs — lint/build static, tests unit — with the capture's own date
// as the rule-7 verified-on stamp, and the pack passes its contract.
func TestRW14CapturedCommandsBecomeTheLadderRungs(t *testing.T) {
	e := registeredProject(t, project.Commands{
		Lint:    "ruff check .",
		Build:   "python -m compileall app",
		Test:    "pytest -q",
		Run:     "flask run", // NOT a check: it starts something and waits
		Preview: "flask run --port 5000",
	})
	pack, err := packFromCapture(verify.DomainSoftware, e)
	if err != nil {
		t.Fatalf("packFromCapture: %v", err)
	}
	if err := pack.Validate(); err != nil {
		t.Fatalf("the built pack fails its own S07.3 contract: %v", err)
	}
	if pack.VerifiedOn.IsZero() || pack.Version != e.CaptureVersion {
		t.Fatalf("pack stamp = %v v%d, want the capture's date and version %d", pack.VerifiedOn, pack.Version, e.CaptureVersion)
	}
	want := map[string]verify.LadderStage{
		"lint":  verify.StageStatic,
		"build": verify.StageStatic,
		"test":  verify.StageUnit,
	}
	if len(pack.Checks) != len(want) {
		t.Fatalf("checks = %+v, want exactly %d (run/preview are previews, not verdicts)", pack.Checks, len(want))
	}
	for _, c := range pack.Checks {
		stage, ok := want[c.ID]
		if !ok {
			t.Fatalf("unexpected check %q — the resolver invented a rung", c.ID)
		}
		if c.Stage != stage {
			t.Fatalf("check %q on rung %q, want %q (S07.3 cheap-first ladder)", c.ID, c.Stage, stage)
		}
		if c.ACKey != "" {
			t.Fatalf("check %q claims to execute frozen criterion %q — a project convention is not an acceptance check (S07.3 rule 4)", c.ID, c.ACKey)
		}
		if _, routed := verify.RouteTable[c.FindingCategory]; !routed {
			t.Fatalf("check %q declares an unrouted finding category %q", c.ID, c.FindingCategory)
		}
	}
}

// TestRW14NoCapturedCommandsNamesWhatToCapture: the live `shop` shape
// (commands = {}). The refusal carries ErrNoCheckPack — the class the verify
// leg turns into a card — and says exactly what is missing and what to do
// (OQ3(i): invent nothing, degrade nothing).
func TestRW14NoCapturedCommandsNamesWhatToCapture(t *testing.T) {
	e := registeredProject(t, project.Commands{})
	pack, err := packFromCapture(verify.DomainSoftware, e)
	if pack != nil {
		t.Fatalf("a command-less project produced a pack: %+v — that is an invented inventory", pack)
	}
	if !errors.Is(err, verify.ErrNoCheckPack) {
		t.Fatalf("err = %v, want the ErrNoCheckPack class (the card's own class)", err)
	}
	for _, want := range []string{"commands", "shop", "retry"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal %q does not tell the operator about %q", err, want)
		}
	}
}

// TestRW14NonLaunchDomainKeepsItsDegradedMode: only launch domains demand a
// pack; everything else keeps the ratified V1-empty degraded mode (Spec S07.8)
// and never sees a card about it.
func TestRW14NonLaunchDomainKeepsItsDegradedMode(t *testing.T) {
	s := &projectSeams{}
	pack, err := s.CheckPackFor(context.Background(), verify.DomainWebResearch, "t-anything")
	if pack != nil || err != nil {
		t.Fatalf("pack=%v err=%v, want the honest (nil, nil) absence", pack, err)
	}
}

// TestRW14SoftwareTaskWithNoProjectRefusesWithTheDoor: a software task on no
// project at all — the second half of OQ3(i). The refusal is the same class,
// so it is the same card, and it says what to do.
func TestRW14SoftwareTaskWithNoProjectRefusesWithTheDoor(t *testing.T) {
	// pipe nil ⇒ projectForTask resolves no project, exactly as it does for a
	// task whose intake matched no registry entry.
	s := &projectSeams{}
	pack, err := s.CheckPackFor(context.Background(), verify.DomainSoftware, "t-unattached")
	if pack != nil {
		t.Fatalf("an unattached software task produced a pack: %+v", pack)
	}
	if !errors.Is(err, verify.ErrNoCheckPack) {
		t.Fatalf("err = %v, want the ErrNoCheckPack class", err)
	}
	if !strings.Contains(err.Error(), "registered project") {
		t.Fatalf("refusal %q does not name the missing project", err)
	}
}
