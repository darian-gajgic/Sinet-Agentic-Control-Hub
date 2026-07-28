package stage

// pause_resume_internal_test.go — the S10.4 pause park (OQ5(ii)) and S14.4's
// "resume — I was wrong" (R15), asserted against REAL FSM rows. $0: nothing
// here spawns a process or dials anything.

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// ── the pause park (OQ5(ii)) ────────────────────────────────────────────────

// TestPauseParksAtTheStageSessionBoundary: with the owner's switch on, the leg's
// boundary check parks the run — and parks it, rather than ending it. The
// generation is unchanged (a park takes no resume edge, §8) and the transition
// records the pause as the cause.
func TestPauseParksAtTheStageSessionBoundary(t *testing.T) {
	e := newCancelEnv(t)
	r := e.seedRun(t, "r-1", "t-1", "alice", run.StateRunning)
	e.sk.cfg.Paused = func(context.Context, string) (bool, error) { return true, nil }

	if !e.sk.pauseParkPoint(context.Background(), r, "step-2") {
		t.Fatal("the boundary check did not park a paused owner's run")
	}
	if got := e.state(t, "r-1"); got != run.StateParked {
		t.Fatalf("run is %s, want parked — a pause parks, it never kills (S10.4; §31)", got)
	}
	after, err := e.runs.Get(context.Background(), "r-1")
	if err != nil {
		t.Fatal(err)
	}
	if after.Generation != r.Generation {
		t.Errorf("generation moved %d → %d on a park — only a resume bumps it (§8)", r.Generation, after.Generation)
	}
	// The record says WHY, and names the remaining S10.8 seam so the v0
	// approximation is visible in the log rather than only in a comment.
	pay := lastStatePayload(t, e, "r-1")
	for _, want := range []string{"automation paused", "alice", "step-2", "S10.8"} {
		if !strings.Contains(pay, want) {
			t.Errorf("the park's run.state_changed row does not mention %q: %s", want, pay)
		}
	}
}

// TestPauseParkPointIsInertWhenNothingIsPaused covers both inert cases: no seam
// wired (every process composed before migration 0017 behaved this way), and a
// seam that says the owner is not paused.
func TestPauseParkPointIsInertWhenNothingIsPaused(t *testing.T) {
	for _, tc := range []struct {
		name string
		seam func(context.Context, string) (bool, error)
	}{
		{"no seam wired", nil},
		{"owner not paused", func(context.Context, string) (bool, error) { return false, nil }},
		{
			// A failed read must not park healthy work: the honest direction for a
			// switch that STOPS things is to keep going and log.
			"seam read fails",
			func(context.Context, string) (bool, error) { return false, errors.New("db hiccup") },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newCancelEnv(t)
			r := e.seedRun(t, "r-1", "t-1", "alice", run.StateRunning)
			e.sk.cfg.Paused = tc.seam
			if e.sk.pauseParkPoint(context.Background(), r, "step-1") {
				t.Fatal("the boundary check parked a run nobody paused")
			}
			if got := e.state(t, "r-1"); got != run.StateRunning {
				t.Fatalf("run is %s, want running", got)
			}
		})
	}
}

// TestTheExecuteLegChecksThePauseBetweenSessions is the STRUCTURAL half of
// OQ5(ii): the check has to sit between stage sessions, and "between" means
// before the session starts. A behavioral test of pauseParkPoint alone would
// still pass if the call site had been dropped, so the call site is asserted
// directly.
func TestTheExecuteLegChecksThePauseBetweenSessions(t *testing.T) {
	src, err := os.ReadFile("skeleton.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	loop := strings.Index(body, "for i, step := range pair.Plan.Steps {")
	check := strings.Index(body, "s.pauseParkPoint(ctx, r, step.ID)")
	session := strings.Index(body, "s.runPlannedStage(ctx, r, er, step)")
	if loop < 0 || check < 0 || session < 0 {
		t.Fatalf("the scan cannot find what it is asserting about (loop %d, check %d, session %d) — it would pass vacuously",
			loop, check, session)
	}
	if !(loop < check && check < session) {
		t.Errorf("the pause check is not between the loop head and the stage session (loop %d, check %d, session %d): a pause must park BEFORE the next session, never inside one",
			loop, check, session)
	}
}

// TestANonExecuteLegParksAtItsOwnBoundaryAndResumesCleanly is drain D4: OQ5(ii)
// says the leg loop checks the flag BETWEEN SESSIONS, and "the leg" is whichever
// leg the run is in — not the execute leg alone.
//
// The verify leg is the proof, and the fixture is deliberately bare: there is no
// deliverable to verify, so if the pause boundary did NOT fire, verifyInput
// would fail and the leg would crash the run. Parked-and-clean is therefore only
// reachable by the boundary actually working.
func TestANonExecuteLegParksAtItsOwnBoundaryAndResumesCleanly(t *testing.T) {
	e := newCancelEnv(t)
	r := e.seedRun(t, "r-1", "t-1", "alice", run.StateClaimed)
	e.sk.cfg.Paused = func(context.Context, string) (bool, error) { return true, nil }

	if err := e.sk.dispatchVerify(context.Background(), r); err != nil {
		t.Fatalf("the verify leg returned an error on a pause park: %v — a pause is not a failure", err)
	}
	if got := e.state(t, "r-1"); got != run.StateParked {
		t.Fatalf("run is %s after the verify leg met a paused owner, want parked", got)
	}
	pay := lastStatePayload(t, e, "r-1")
	if !strings.Contains(pay, "automation paused") || !strings.Contains(pay, "verify") {
		t.Errorf("the verify leg's park does not name the pause and the boundary: %s", pay)
	}
	// …and it RESUMES cleanly: an ask-less park is exactly what the S14.4 verb
	// releases.
	out, err := e.sk.ResumeRun(context.Background(), "alice", "r-1")
	if err != nil {
		t.Fatalf("ResumeRun after a pause park: %v", err)
	}
	if out.To != string(run.StateRunning) || out.Generation != r.Generation+1 {
		t.Fatalf("resume outcome = %+v, want running at a bumped generation", out)
	}
}

// TestEveryDispatchLegHasAPauseBoundary is the structural half of D4: a leg
// added later without a boundary would silently ignore the switch, and no
// behavioral test would notice because the leg would not exist yet. The scan
// pins the four boundaries that exist and names where they are.
func TestEveryDispatchLegHasAPauseBoundary(t *testing.T) {
	want := map[string]int{
		"skeleton.go": 2, // the execute step loop + the verify drain
		"compose.go":  1, // the composition ceremony's one shot
		"split.go":    1, // each split successor INSIDE a planned stage
	}
	total := 0
	for file, n := range want {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Count(string(src), "s.pauseParkPoint(ctx, r,")
		if got != n {
			t.Errorf("%s has %d pause boundaries, want %d — every dispatch leg checks the switch between its sessions (OQ5(ii))", file, got, n)
		}
		total += got
	}
	if total == 0 {
		t.Fatal("the scan found no pause boundaries at all — it would pass vacuously")
	}
}

// TestPauseParkTakesNoTerminalEdge is the NO-AUTO-KILL complement in this file's
// own terms (§31; the tree-wide cancel-reachability wall covers the rest): the
// pause path's only transition target is `parked`, so no code path here can end
// a run. A pause preserves work; it never disposes of it.
func TestPauseParkTakesNoTerminalEdge(t *testing.T) {
	src, err := os.ReadFile("pause.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if !strings.Contains(body, "run.StateParked") {
		t.Fatal("pause.go no longer parks — the scan would pass vacuously")
	}
	for _, terminal := range []string{
		"run.StateCompleted", "run.StateFinalized", "run.StateTombstoned",
		"run.StateDiedAtGate", "run.StateCrashed",
	} {
		if strings.Contains(body, terminal) {
			t.Errorf("pause.go names %s — a pause parks and never ends a run (S10.4; §31)", terminal)
		}
	}
}

// ── R15: "resume — I was wrong" ─────────────────────────────────────────────

// TestResumeTakesTheRatifiedEdgeAndBumpsGeneration: parked→running, generation
// bumped, the acting PERSON on the transition. The generation bump is what makes
// a double-resume inert (P-T02-3) and what renders any stale fence dead (§8).
func TestResumeTakesTheRatifiedEdgeAndBumpsGeneration(t *testing.T) {
	e := newCancelEnv(t)
	before := e.seedRun(t, "r-1", "t-1", "alice", run.StateParked)

	out, err := e.sk.ResumeRun(context.Background(), "alice", "r-1")
	if err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	if out.From != string(run.StateParked) || out.To != string(run.StateRunning) || !out.Applied {
		t.Fatalf("outcome = %+v, want the ratified parked→running edge applied", out)
	}
	if out.Generation != before.Generation+1 {
		t.Errorf("generation %d → %d, want a bump of exactly 1 (§8)", before.Generation, out.Generation)
	}
	if got := e.state(t, "r-1"); got != run.StateRunning {
		t.Fatalf("run is %s, want running", got)
	}
	// The actor is the PERSON. That is what the landed open-flag derivation
	// reads as "resume, I was wrong" — and it is the difference between a human
	// release and a platform one.
	pay := lastStatePayload(t, e, "r-1")
	if !strings.Contains(pay, "alice") || !strings.Contains(pay, "human resume") {
		t.Errorf("the resume's run.state_changed row does not name the person who took it: %s", pay)
	}
}

// TestResumeRefusesAnOpenAskParkAndPointsAtTheAsk: for a run parked on a card,
// ANSWERING the card is the resume (§16; S07.7). Flipping the state underneath
// an open ask would leave a card nobody can act on attached to a run nobody is
// driving, so the verb refuses — and refuses usefully, by naming the ask.
func TestResumeRefusesAnOpenAskParkAndPointsAtTheAsk(t *testing.T) {
	e := newCancelEnv(t)
	e.seedRun(t, "r-1", "t-1", "alice", run.StateParked)
	seedOpenAsk(t, e, "ask-7", "r-1", "alice")

	_, err := e.sk.ResumeRun(context.Background(), "alice", "r-1")
	var open *ResumeOpenAskError
	if !errors.As(err, &open) {
		t.Fatalf("ResumeRun err = %v, want the open-ask refusal", err)
	}
	if open.AskID != "ask-7" {
		t.Errorf("the refusal names ask %q, want ask-7 — the pointer IS the useful part", open.AskID)
	}
	if got := e.state(t, "r-1"); got != run.StateParked {
		t.Fatalf("the refused resume moved the run to %s — a refusal fires nothing", got)
	}
	// …and once the ask is answered, the ask-less park resumes normally.
	answerAsk(t, e, "ask-7")
	if _, err := e.sk.ResumeRun(context.Background(), "alice", "r-1"); err != nil {
		t.Fatalf("ResumeRun after the ask closed: %v", err)
	}
}

// TestResumeRefusesANonParkedRun: only a parked run can be resumed, and the
// refusal is a conflict the caller resolves by re-reading — never a forced edge.
func TestResumeRefusesANonParkedRun(t *testing.T) {
	e := newCancelEnv(t)
	for _, tc := range []struct {
		runID string
		state run.State
	}{
		{"r-running", run.StateRunning},
		{"r-queued", run.StateQueued},
		{"r-done", run.StateCompleted},
	} {
		e.seedRun(t, tc.runID, "t-1", "alice", tc.state)
		if _, err := e.sk.ResumeRun(context.Background(), "alice", tc.runID); !errors.Is(err, ErrResumeNotParked) {
			t.Errorf("resume of a %s run: err = %v, want ErrResumeNotParked", tc.state, err)
		}
		if got := e.state(t, tc.runID); got != tc.state {
			t.Errorf("the refused resume moved the %s run to %s", tc.state, got)
		}
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func lastStatePayload(t *testing.T, e *cancelEnv, runID string) string {
	t.Helper()
	var payload string
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT payload FROM run_events WHERE run_id = ? AND type = ? ORDER BY event_seq DESC LIMIT 1`,
		runID, run.EventState).Scan(&payload); err != nil {
		t.Fatalf("read last state event of %s: %v", runID, err)
	}
	return payload
}

func seedOpenAsk(t *testing.T, e *cancelEnv, askID, runID, owner string) {
	t.Helper()
	if err := e.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?,?,?,?,?,?)`,
			askID, runID, owner, `{"kind":"approval"}`, "open", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("seed ask: %v", err)
	}
}

func answerAsk(t *testing.T, e *cancelEnv, askID string) {
	t.Helper()
	if err := e.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`UPDATE asks SET status='answered', answered_ts=?, answer='{}' WHERE ask_id=?`,
			time.Now().UTC().Format(time.RFC3339Nano), askID)
		return err
	}); err != nil {
		t.Fatalf("answer ask: %v", err)
	}
}
