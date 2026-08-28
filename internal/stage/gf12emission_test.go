package stage

// gf12emission_test.go — P3-GF12 R2: the seam's bounded re-emission for a
// CONTRACT-invalid emission, driven at the helper rather than through a
// Skeleton, so the machinery itself is what is under test: how many sessions it
// spends, what it puts in the re-ask, and what it does with a refusal it cannot
// clear.
//
// The class this closes was witnessed live (GF9 evidence world, control.log
// 2026-08-27T23:33..2026-08-28T01:14): eleven over-cap emissions, every one of
// them returned straight up as `planner output: … approach is N characters (cap
// 1200)`, every one crashing the intake drive into a recovery ladder that
// re-drove the SAME prompt with zero new information until the lineage
// tombstoned. Twice. The seam had a bounded re-ask for a malformed ENVELOPE
// (jsonRetryLimit) and none at all for a malformed CONTRACT.
//
// No engine runs here and none may: the scripted session func IS the seat.

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// scriptedPair returns a pair whose S-1 approach is exactly n characters — the
// witnessed refusal's own axis. Everything else is a minimal honest emission
// that passes the artifact contract once the caller's stamp has run.
func scriptedPair(approachRunes int) intake.Pair {
	return intake.Pair{
		Spec: intake.Spec{
			Restatement: "Requester wants the widget fixed",
			ACs:         []intake.AC{{N: 1, Plain: "the widget works"}},
		},
		Plan: intake.Plan{
			Steps: []intake.Step{{
				ID: "S-1", Title: "Fix it", DoneWhen: "the check passes", Class: "C1",
				Approach: strings.Repeat("a", approachRunes),
			}},
			Coverage: map[string][]string{"AC-1": {"S-1"}},
		},
	}
}

// testStamp is the platform-owned bookkeeping the real seam applies before it
// validates — versions and identity are the platform's, never the engine's.
func testStamp(p *intake.Pair) {
	p.Spec.TaskID, p.Plan.TaskID = "t-1", "t-1"
	p.Spec.Owner, p.Plan.Owner = "u1", "u1"
	p.Spec.Version, p.Plan.Version, p.Plan.SpecVersion = 1, 1, 1
	p.Spec.Status, p.Plan.Status = intake.StatusDraft, intake.StatusDraft
}

// TestGF12ReEmissionFeedsTheRefusalBackVerbatim (brief R2): the first emission
// is refused for the A15 cap; the SECOND session must carry the refusal's own
// specifics — the step, the count and the cap — because a re-ask that does not
// say what was wrong is the ladder's zero-information re-drive wearing a
// cheaper hat.
func TestGF12ReEmissionFeedsTheRefusalBackVerbatim(t *testing.T) {
	var notes []string
	runSession := func(note string) (intake.Pair, error) {
		notes = append(notes, note)
		if len(notes) == 1 {
			return scriptedPair(intake.ApproachMaxRunes + 94), nil // the witnessed 1294
		}
		return scriptedPair(intake.ApproachMaxRunes), nil
	}
	pair, err := pairWithRetry(runSession, testStamp, nil)
	if err != nil {
		t.Fatalf("the bounded re-emission never recovered: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("sessions spent = %d, want 2 (one refusal, one corrected emission)", len(notes))
	}
	if notes[0] != "" {
		t.Errorf("the FIRST session carried a re-ask note %q — nothing had been refused yet", notes[0])
	}
	for _, want := range []string{
		fmt.Sprintf("step S-1 approach is %d characters (cap %d)", intake.ApproachMaxRunes+94, intake.ApproachMaxRunes),
		"REFUSED",
	} {
		if !strings.Contains(notes[1], want) {
			t.Errorf("the re-ask does not carry %q — it reads:\n%s", want, notes[1])
		}
	}
	// Never a silent trim (§60): what came back is what the seat wrote, at the
	// length the seat wrote it.
	if got := len([]rune(pair.Plan.Steps[0].Approach)); got != intake.ApproachMaxRunes {
		t.Fatalf("approach is %d characters — the platform edited model output", got)
	}
}

// TestGF12ReEmissionIsBoundedAndRefusesHonestly (brief R2/R3): a seat that keeps
// overrunning gets exactly emissionRetryLimit re-asks and no more, and what
// comes out is the refusal in the shape the pipeline's landing branches on
// (`planner output:` wrapping an ErrBadArtifact chain) — never an unbounded
// loop, and never a swallowed error.
func TestGF12ReEmissionIsBoundedAndRefusesHonestly(t *testing.T) {
	sessions, refusals, retried := 0, 0, 0
	runSession := func(string) (intake.Pair, error) {
		sessions++
		return scriptedPair(intake.ApproachMaxRunes + 1), nil
	}
	onRefusal := func(attempt int, retrying bool, refusal error) {
		refusals++
		if attempt != refusals {
			t.Errorf("refusal reported for attempt %d, want %d", attempt, refusals)
		}
		if retrying {
			retried++
		}
		if refusal == nil {
			t.Error("a refusal was reported with no refusal")
		}
	}
	_, err := pairWithRetry(runSession, testStamp, onRefusal)
	if want := 1 + emissionRetryLimit; sessions != want {
		t.Fatalf("sessions spent = %d, want %d (the first emission plus emissionRetryLimit re-asks)", sessions, want)
	}
	if refusals != sessions {
		t.Errorf("refusals reported = %d, want %d — every refusal is logged, the exhausting one included", refusals, sessions)
	}
	if retried != emissionRetryLimit {
		t.Errorf("re-asks announced = %d, want %d — the last refusal is not a re-ask", retried, emissionRetryLimit)
	}
	if !errors.Is(err, intake.ErrBadArtifact) {
		t.Fatalf("exhaustion returned %v — the pipeline's honest landing branches on ErrBadArtifact", err)
	}
	if !strings.HasPrefix(err.Error(), "planner output: ") {
		t.Fatalf("exhaustion returned %q — the wrapping the un-bounced seam always returned must not move", err)
	}
}

// TestGF12ReEmissionNeverRetriesASessionError (brief R2; §60): a session that
// FAILED is infrastructure death, and the S02.5 ladder stays its sole owner. The
// bounded re-emission is for an engine that answered badly, never for one that
// did not answer — retrying here would spend a paid round on a lane that is
// down and hide the crash the ladder exists to handle.
func TestGF12ReEmissionNeverRetriesASessionError(t *testing.T) {
	boom := errors.New("planner session: adapter: lane unreachable")
	sessions := 0
	runSession := func(string) (intake.Pair, error) {
		sessions++
		return intake.Pair{}, boom
	}
	_, err := pairWithRetry(runSession, testStamp, func(int, bool, error) {
		t.Error("a session error was reported as a contract refusal")
	})
	if sessions != 1 {
		t.Fatalf("sessions spent = %d, want 1 — a dead lane is not re-asked", sessions)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("session error came back as %v — it must reach the caller unchanged", err)
	}
}

// TestGF12ACleanEmissionSpendsExactlyOneSession (the control): the ordinary path
// is untouched — one session, no note, no refusal, no extra spend.
func TestGF12ACleanEmissionSpendsExactlyOneSession(t *testing.T) {
	sessions := 0
	runSession := func(note string) (intake.Pair, error) {
		sessions++
		if note != "" {
			t.Errorf("a clean first emission was asked with a re-ask note %q", note)
		}
		return scriptedPair(64), nil
	}
	if _, err := pairWithRetry(runSession, testStamp, func(int, bool, error) {
		t.Error("a valid emission was reported as refused")
	}); err != nil {
		t.Fatalf("a valid emission errored: %v", err)
	}
	if sessions != 1 {
		t.Fatalf("sessions spent = %d, want 1", sessions)
	}
}
