package recovery_test

// The kill-9 harness v1 (Spec S02.9): a standing conformance-suite entry.
// The test binary re-invokes itself as a crash helper (standard Go re-exec
// pattern): the child opens the same platform.db, drives real machinery to
// a nasty window — mid-transition write, or the paid-call/checkpoint and
// effect `executing` windows — and dies by SIGKILL. The parent then asserts
// the application invariants: PRAGMA integrity_check ok, committed state
// never lost, the interrupted transaction invisible, one reconcile pass
// classifying every non-terminal run, fenced double-resume inert, zero
// double-executed effects, and asks surviving as authoritative rows.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

const (
	envScenario = "SINET_CRASH_SCENARIO"
	envDir      = "SINET_CRASH_DIR"
)

// TestMain interposes the crash-helper role: when the scenario env is set,
// this process is the child that must die mid-write.
func TestMain(m *testing.M) {
	if scenario := os.Getenv(envScenario); scenario != "" {
		if err := crashHelper(scenario, os.Getenv(envDir)); err != nil {
			fmt.Fprintf(os.Stderr, "crash helper %q: %v\n", scenario, err)
			os.Exit(3)
		}
		// Every scenario ends in SIGKILL; reaching here is a defect.
		fmt.Fprintf(os.Stderr, "crash helper %q returned instead of dying\n", scenario)
		os.Exit(4)
	}
	os.Exit(m.Run())
}

// die delivers SIGKILL to this process — nothing below the syscall runs, no
// deferred cleanup, no buffered flush: the closest software stand-in for
// yanked power over a durable store.
func die() {
	_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
	select {} // unreachable; SIGKILL cannot be handled
}

func crashHelper(scenario, dir string) error {
	ctx := context.Background()
	s, err := openStack(dir)
	if err != nil {
		return err
	}
	switch scenario {
	case "midtx":
		// Committed prefix: a run driven to running with one checkpoint.
		if _, err := mkRunning(s, "t1"); err != nil {
			return err
		}
		if _, err := s.cps.Write(ctx, mkCheckpoint("t1")); err != nil {
			return err
		}
		// The nasty window: die INSIDE an open transition transaction —
		// runs.state already updated, the event already inserted, commit
		// never reached. Recovery must see none of it.
		return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
			if _, err := s.runs.TransitionTx(ctx, tx, "t1", run.StateParked, run.TransitionOptions{Reason: "gate"}); err != nil {
				return err
			}
			die()
			return nil
		})

	case "postcommit":
		// Committed: run + checkpoint, and two effects journaled
		// `executing` — written BEFORE the provider call (Spec S02.7). The
		// crash lands exactly in the provider-call window.
		if _, err := mkRunning(s, "t2"); err != nil {
			return err
		}
		if _, err := s.cps.Write(ctx, mkCheckpoint("t2")); err != nil {
			return err
		}
		ids := map[string]string{}
		for class, payload := range map[gates.Class]string{
			gates.ClassB: `{"channel":"email","to":"x@example.com"}`,
			gates.ClassD: `{"channel":"smtp","to":"y@example.com"}`,
		} {
			eff, err := s.jrnl.Propose(ctx, gates.Proposal{
				RunID: "t2", UserID: "u1", Class: class, Payload: json.RawMessage(payload),
			})
			if err != nil {
				return err
			}
			if _, err := s.jrnl.Approve(ctx, eff.ID, "operator"); err != nil {
				return err
			}
			if _, err := s.jrnl.BeginExecute(ctx, eff.ID); err != nil {
				return err
			}
			ids[string(class)] = eff.ID
		}
		// An open ask, observed pre-crash: must survive as the
		// authoritative row ("asks ⊇ engine pending", Spec S02.9).
		err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts)
				 VALUES ('ask-1', 't2', 'u1', '{"invocation":"full"}', 'open', '2026-07-19T00:00:00Z')`)
			return err
		})
		if err != nil {
			return err
		}
		manifest, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "effects.json"), manifest, 0o600); err != nil {
			return err
		}
		die()
		return nil

	default:
		return fmt.Errorf("unknown scenario %q", scenario)
	}
}

// spawnCrash re-execs this test binary as the crash child and asserts it
// actually died by SIGKILL.
func spawnCrash(t *testing.T, scenario, dir string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=none")
	cmd.Env = append(os.Environ(), envScenario+"="+scenario, envDir+"="+dir)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("crash child: err = %v (output %q), want SIGKILL death", err, out)
	}
	ws, ok := ee.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
		t.Fatalf("crash child exit = %v (output %q), want killed by SIGKILL", ee, out)
	}
}

// assertIntegrity runs the S02.9 postcondition explicitly (storage.Open
// also runs it at every open).
func assertIntegrity(t *testing.T, s *stack) {
	t.Helper()
	var res string
	if err := s.db.QueryRowContext(context.Background(), "PRAGMA integrity_check").Scan(&res); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if res != "ok" {
		t.Fatalf("integrity_check = %q, want ok", res)
	}
}

// assertNoSeqGaps asserts the event log is gap-free: COUNT == MAX(event_seq)
// (nothing ever deletes from the append-only log).
func assertNoSeqGaps(t *testing.T, s *stack) {
	t.Helper()
	var count, max int
	err := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*), COALESCE(MAX(event_seq), 0) FROM run_events`).Scan(&count, &max)
	if err != nil {
		t.Fatalf("seq gap query: %v", err)
	}
	if count != max {
		t.Fatalf("event_seq gaps: %d rows, max seq %d", count, max)
	}
}

func mkCheckpoint(runID string) gates.NewCheckpoint {
	return gates.NewCheckpoint{
		RunID:                 runID,
		Usage:                 json.RawMessage(`{"input_tokens":100,"output_tokens":40,"cost_usd":0.02}`),
		SessionSubstrate:      "mock",
		SessionID:             "sess-" + runID,
		MessageIndex:          1,
		CwdKey:                "/w",
		TranscriptPath:        "/w/t.jsonl",
		LedgerRevision:        "rev-1",
		ArtifactSnapshotRef:   "snap-1",
		ModelID:               "mock-model",
		InvocationFingerprint: "fp-1",
		ToolSchemaVersion:     "ts-1",
		PromptSchemaVersion:   "ps-1",
	}
}

// TestKill9MidTransitionInvisible: SIGKILL inside an uncommitted transition
// transaction. The interrupted transaction must be invisible, committed
// state intact, and one reconcile pass must classify and supersede the run
// — with the old generation fenced.
func TestKill9MidTransitionInvisible(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	spawnCrash(t, "midtx", dir)

	s, err := openStack(dir) // storage.Open runs integrity_check
	if err != nil {
		t.Fatalf("reopen after kill: %v", err)
	}
	defer s.db.Close()
	assertIntegrity(t, s)
	assertNoSeqGaps(t, s)

	// The interrupted transaction is invisible: still running, and exactly
	// the committed events exist (created + 3 transitions + checkpoint).
	r := getRun(t, s, "t1")
	if r.State != run.StateRunning {
		t.Fatalf("state = %s, want running (uncommitted parked transition must not survive)", r.State)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_events WHERE run_id = 't1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("t1 events = %d, want 5 (created, →queued, →claimed, →running, checkpoint)", n)
	}
	cp, ok, err := s.cps.Last(ctx, "t1")
	if err != nil || !ok {
		t.Fatalf("committed checkpoint lost: %v %v", ok, err)
	}

	// One reconcile pass classifies the run (no unit machinery at B0 →
	// DEAD) and supersedes it by fork-from-last-checkpoint.
	l := newLadder(t, s, nil, nil, nil)
	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Scanned != 1 || rpt.Forked != 1 {
		t.Fatalf("report = %+v, want 1 scanned / 1 forked", rpt)
	}
	parent := getRun(t, s, "t1")
	succ := getRun(t, s, "t1.g1")
	if parent.State != run.StateCrashed || parent.Generation != 1 {
		t.Fatalf("parent = %s gen %d, want crashed gen 1", parent.State, parent.Generation)
	}
	if succ.State != run.StateQueued || succ.Generation != 1 || succ.RecoveryAttempts != 1 {
		t.Fatalf("successor = %+v", succ)
	}
	ev := lastOfType(t, s, "t1.g1", run.EventForked)
	var p struct {
		Detail struct {
			FromCheckpointID int64 `json:"from_checkpoint_id"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(ev.Payload, &p); err != nil || p.Detail.FromCheckpointID != cp.ID {
		t.Fatalf("fork cites checkpoint %d (%v), want %d — re-spend bounded to work since the last paid call", p.Detail.FromCheckpointID, err, cp.ID)
	}
	// Fenced double-resume inert: an append from the dead incarnation's
	// generation is rejected and lands nothing.
	before := eventCount(t, s)
	_, err = s.log.Append(ctx, eventlog.Append{
		RunID: "t1", Generation: 0, UserID: "u1", Type: "zombie.write",
		SchemaVersion: 1, Payload: json.RawMessage(`{}`),
	})
	if !errors.Is(err, eventlog.ErrStaleGeneration) {
		t.Fatalf("zombie append err = %v, want ErrStaleGeneration", err)
	}
	if eventCount(t, s) != before {
		t.Fatal("fenced append still landed a row")
	}
	assertNoSeqGaps(t, s)
}

// TestKill9EffectCrashWindow: SIGKILL in the provider-call window, after
// the `executing` rows committed. Recovery must resolve in-doubt effects
// per class with zero double-execution: class B returns to approved with
// its idempotency key and attempt count intact (the replay dedups at the
// provider); class D flips to unknown with a card. Committed checkpoints
// survive; the open ask survives as the authoritative row.
func TestKill9EffectCrashWindow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	spawnCrash(t, "postcommit", dir)

	s, err := openStack(dir)
	if err != nil {
		t.Fatalf("reopen after kill: %v", err)
	}
	defer s.db.Close()
	assertIntegrity(t, s)

	manifest, err := os.ReadFile(filepath.Join(dir, "effects.json"))
	if err != nil {
		t.Fatalf("effects manifest: %v", err)
	}
	var ids map[string]string
	if err := json.Unmarshal(manifest, &ids); err != nil {
		t.Fatalf("manifest: %v", err)
	}

	// No lost committed state: the checkpoint is there.
	if _, ok, err := s.cps.Last(ctx, "t2"); err != nil || !ok {
		t.Fatalf("committed checkpoint lost: %v %v", ok, err)
	}

	l := newLadder(t, s, nil, nil, nil)
	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.EffectsReplayable != 1 || rpt.EffectsUnknown != 1 {
		t.Fatalf("report = %+v, want 1 replayable / 1 unknown", rpt)
	}
	effB, err := s.jrnl.Get(ctx, ids["B"])
	if err != nil {
		t.Fatal(err)
	}
	if effB.State != gates.EffectApproved {
		t.Fatalf("class B = %s, want approved (replay with the stored key)", effB.State)
	}
	if effB.IdempotencyKey != effB.ID || effB.Attempts != 1 {
		t.Fatalf("class B replay identity lost: key %q attempts %d — double execution would not dedup", effB.IdempotencyKey, effB.Attempts)
	}
	effD, err := s.jrnl.Get(ctx, ids["D"])
	if err != nil {
		t.Fatal(err)
	}
	if effD.State != gates.EffectUnknown || len(effD.Result) == 0 {
		t.Fatalf("class D = %s (result %s), want unknown + card (P-T07-3)", effD.State, effD.Result)
	}

	// The run itself was mid-flight → superseded from its checkpoint.
	if rpt.Forked != 1 {
		t.Fatalf("report = %+v, want the run forked", rpt)
	}
	// Ask survived the crash as the authoritative row (Spec S02.9:
	// asks ⊇ engine pending) and was re-observed.
	if rpt.AsksObserved != 1 {
		t.Fatalf("asks observed = %d, want 1", rpt.AsksObserved)
	}
	var snapshot string
	if err := s.db.QueryRowContext(ctx, `SELECT snapshot FROM asks WHERE ask_id = 'ask-1'`).Scan(&snapshot); err != nil {
		t.Fatalf("ask row lost: %v", err)
	}
	if snapshot == "" {
		t.Fatal("ask invocation snapshot lost")
	}
	assertNoSeqGaps(t, s)
}

func eventCount(t *testing.T, s *stack) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM run_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
