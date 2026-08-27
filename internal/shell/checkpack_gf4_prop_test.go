package shell

// checkpack_gf4_prop_test.go — P3-GF4 property P1 (Spec S07.8 bootstrap
// posture, A14 2026-08-27): the bootstrap CONDITION, pinned exhaustively
// rather than by example.
//
// A14 defines the posture on a single fact — "no captured build/test/lint
// commands" — so the resolver's answer must be a total, exact function of the
// five captured command slots: bootstrap exactly when build, test and lint are
// all blank, and a real (possibly partial) pack carrying exactly the non-blank
// ones otherwise. run/preview never move it either way: they start something
// and wait, which is a preview (Spec S13.8), not a verdict.
//
// packFromCapture is pure over one registry entry (its own doc), so the
// enumeration constructs entries directly and covers all 3^5 = 243
// blank/whitespace/command assignments — whitespace counts as blank because
// packChecks trims.

import (
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// commandValues are the three cases every slot can be in: absent, present but
// blank (the trim case), and a real command.
var commandValues = []string{"", " \t ", "echo run-me"}

func TestGF4PropBootstrapIsExactlyNoExecutableRung(t *testing.T) {
	capturedTS := time.Now().UTC().Format(time.RFC3339Nano)
	for _, build := range commandValues {
		for _, test := range commandValues {
			for _, lint := range commandValues {
				for _, runCmd := range commandValues {
					for _, preview := range commandValues {
						cmds := project.Commands{
							Build: build, Test: test, Lint: lint, Run: runCmd, Preview: preview,
						}
						e := project.Entry{
							ProjectID: "shop", Owner: "u1", Name: "shop", CaptureVersion: 2,
							Capture: project.Capture{
								Version: 2, Commands: cmds, CapturedTS: capturedTS,
							},
						}
						pack, err := packFromCapture(verify.DomainSoftware, e)
						if err != nil {
							t.Fatalf("packFromCapture(%+v): %v — a registered project's capture always resolves to an answer", cmds, err)
						}
						if pack == nil {
							t.Fatalf("packFromCapture(%+v) resolved nothing — bootstrap must stay distinguishable from the (nil, nil) no-pack-machinery answer", cmds)
						}
						wantRungs := map[string]verify.LadderStage{}
						if strings.TrimSpace(lint) != "" {
							wantRungs["lint"] = verify.StageStatic
						}
						if strings.TrimSpace(build) != "" {
							wantRungs["build"] = verify.StageStatic
						}
						if strings.TrimSpace(test) != "" {
							wantRungs["test"] = verify.StageUnit
						}
						if len(wantRungs) == 0 {
							assertBootstrapResolution(t, pack, cmds)
							continue
						}
						assertFullResolution(t, pack, cmds, wantRungs)
					}
				}
			}
		}
	}
}

func assertBootstrapResolution(t *testing.T, pack *verify.CheckPack, cmds project.Commands) {
	t.Helper()
	if pack.Posture != verify.PostureBootstrap {
		t.Fatalf("capture %+v has no executable rung but resolved posture %q, want %q (Spec S07.8, A14)",
			cmds, pack.Posture, verify.PostureBootstrap)
	}
	if len(pack.Checks) != 0 {
		t.Fatalf("capture %+v produced checks %+v — bootstrap invents no inventory", cmds, pack.Checks)
	}
	if err := pack.Validate(); err == nil {
		t.Fatalf("the bootstrap resolution for %+v passes CheckPack.Validate — a pack without checks must still fail its own S07.3 contract; the drain branches on the posture instead", cmds)
	}
}

func assertFullResolution(t *testing.T, pack *verify.CheckPack, cmds project.Commands, wantRungs map[string]verify.LadderStage) {
	t.Helper()
	if pack.Posture != "" {
		t.Fatalf("capture %+v has %d executable rung(s) but resolved posture %q — one captured command is a real pack, not bootstrap",
			cmds, len(wantRungs), pack.Posture)
	}
	if err := pack.Validate(); err != nil {
		t.Fatalf("the pack built from %+v fails its own S07.3 contract: %v", cmds, err)
	}
	if len(pack.Checks) != len(wantRungs) {
		t.Fatalf("capture %+v produced checks %+v, want exactly %v (run/preview are previews, not verdicts)", cmds, pack.Checks, wantRungs)
	}
	for _, c := range pack.Checks {
		stage, ok := wantRungs[c.ID]
		if !ok {
			t.Fatalf("capture %+v produced an unexpected rung %q", cmds, c.ID)
		}
		if c.Stage != stage {
			t.Fatalf("rung %q on stage %q, want %q (S07.3 cheap-first ladder)", c.ID, c.Stage, stage)
		}
	}
}
