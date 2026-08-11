package adapters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// Driver is the platform side of the adapter seam: it drives one run
// through an Adapter and persists everything the contract yields — engine
// events into run_events (fenced), a checkpoint row per paid call
// (Spec S02.4, D7), the engine_sessions registry row with its transcript
// copy-aside (S02.4, P-T07-2), and the durable ask-record the moment a gate
// ask is observed (S03.4).
//
// At B1-1 the Driver is invoked directly by tests and bring-up demos; the
// scheduler (Spec S10, B1-2) becomes its caller when admission lands.
type Driver struct {
	Runs        *run.Store
	Checkpoints *gates.Checkpoints
	Log         *eventlog.Log
	DB          *storage.DB

	// CopyAsideDir roots the per-run transcript copy-aside tree
	// (S02.4 "transcript copy-aside", Claude lane; cheap insurance against
	// the engine session sweep and crash-truncated JSONL). Empty disables
	// copy-aside (fixture-driven tests without a transcript on disk).
	CopyAsideDir string

	// Snapshot is the S02.4(d) artifact-snapshot seam: a platform-owned
	// snapshot of the run workspace taken at each checkpoint, returning
	// the snapshot ref. The Claude lane's ref is a platform-owned snapshot
	// commit in the run worktree — worktree machinery is B2's; nil leaves
	// the block empty (the lane genuinely provides none yet).
	Snapshot func(ctx context.Context, runID string) (string, error)

	// LedgerRevision is the S02.4(c) ledger-revision seam: the Task Context
	// Ledger revision ref (ledger_version + content hash) recorded on every
	// checkpoint (Spec S05.2 duty 1 — every checkpoint embeds the current
	// ledger state by ledger_version; the full content is self-contained in
	// run_events as ledger_update revisions, Spec S02.2). Wired to
	// (*ledger.Store).CheckpointRef; nil, or a run without a task ledger,
	// leaves the block empty.
	LedgerRevision func(ctx context.Context, runID string) (string, error)

	// Leases renews the run's lease while this driver is pumping an engine
	// session (Spec S02.2 lease block at ⚙ recovery.heartbeat cadence;
	// P3-RW-5 R3). It is the one holder that covers a LONG QUIET call — a
	// paid engine turn whose stream says nothing for minutes leaves no cursor
	// evidence at all, so only holder liveness can speak for it. Nil is inert
	// (unwired compositions behave exactly as before).
	Leases *run.LeaseKeeper

	// Now is the clock seam (tests). Nil = time.Now.
	Now func() time.Time
}

// leaseHolder names the driver in the lease block while it pumps.
const leaseHolder = "engine-driver"

// Driver errors.
var (
	// ErrNotDrivable rejects driving a run that is not in the state the
	// entry point requires (claimed for Drive, parked for DriveResume).
	ErrNotDrivable = errors.New("adapters: run is not in a drivable state")
)

func (d *Driver) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// Drive takes a CAS-claimed run (Spec S02.3 claimed: lease held) through
// one engine invocation: claimed→running, spawn, pump, terminal
// transition. It returns the session Outcome; FSM mapping per outcome:
//
//	completed    → running→completed
//	parked       → running→parked (+ durable ask row when gate-parked)
//	died-at-gate → running→parked→died-at-gate (S02.3: the budget preempt
//	               edge leaves parked — the gate moment was real even
//	               though the engine died before the park could resume)
//	crashed      → running→crashed (fork-from-checkpoint is recovery's,
//	               Spec S02.5)
//	canceled     → NO transition here: S02.3 ratifies no cancel edge; the
//	               canceling surface (S10/S15 policy, later packets) owns
//	               the disposition of the row.
func (d *Driver) Drive(ctx context.Context, a Adapter, req StartRequest) (Outcome, error) {
	r, err := d.Runs.Get(ctx, req.RunID)
	if err != nil {
		return Outcome{}, err
	}
	if r.State != run.StateClaimed {
		return Outcome{}, fmt.Errorf("%w: %q is %s, want %s", ErrNotDrivable, r.ID, r.State, run.StateClaimed)
	}
	// claimed→running before spawn: the transition and its event commit
	// first so every engine event lands at the running generation; a spawn
	// failure immediately crashes the run (S02.3).
	r, err = d.Runs.Transition(ctx, req.RunID, run.StateRunning, run.TransitionOptions{
		Reason: "engine start (S03.1)", Actor: run.ActorPlatform,
	})
	if err != nil {
		return Outcome{}, err
	}
	// Held from here: the engine call is the platform's own advance, and the
	// hold is taken AFTER the transition so it beats at the generation the
	// events will carry (Spec S02.5 step 4).
	defer d.Leases.Hold(ctx, r.ID, leaseHolder)()
	sess, err := a.Start(ctx, req)
	if err != nil {
		_, terr := d.Runs.Transition(ctx, req.RunID, run.StateCrashed, run.TransitionOptions{
			Reason: "engine spawn failed", Actor: run.ActorPlatform,
			Detail: reasonDetail(err.Error()),
		})
		if terr != nil {
			return Outcome{}, fmt.Errorf("adapters: spawn failed (%v) and crash transition failed: %w", err, terr)
		}
		return Outcome{Kind: OutcomeCrashed, Detail: err.Error()}, nil
	}
	if req.OnSession != nil {
		req.OnSession(sess)
	}
	return d.pump(ctx, sess, r, req)
}

// DriveResume takes a parked run through resume: parked→running (the
// S02.3 resume edge bumps the generation — the fence against double
// resume), full invocation re-supply via the adapter, pump. The caller
// runs the S02.6 freshness pass BEFORE this; ans answers the open ask on
// the gate path (marked answered in the same transaction as the resume
// transition).
func (d *Driver) DriveResume(ctx context.Context, a Adapter, rec ParkRecord, ans *Answer) (Outcome, error) {
	r, err := d.Runs.Get(ctx, rec.RunID)
	if err != nil {
		return Outcome{}, err
	}
	if r.State != run.StateParked {
		return Outcome{}, fmt.Errorf("%w: %q is %s, want %s", ErrNotDrivable, r.ID, r.State, run.StateParked)
	}
	err = d.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if ans != nil && ans.AskID != "" {
			if err := d.markAskAnsweredTx(ctx, tx, ans); err != nil {
				return err
			}
		}
		r, err = d.Runs.TransitionTx(ctx, tx, rec.RunID, run.StateRunning, run.TransitionOptions{
			Reason: "resume (S03.1)", Actor: run.ActorPlatform,
		})
		return err
	})
	if err != nil {
		return Outcome{}, err
	}
	// The resume edge bumped the generation, so the hold is taken after the
	// transaction commits — and it beats immediately, which is what closes the
	// un-park window the recovery ladder used to reap through (R5).
	defer d.Leases.Hold(ctx, r.ID, leaseHolder)()
	sess, err := a.Resume(ctx, rec, ans)
	if err != nil {
		_, terr := d.Runs.Transition(ctx, rec.RunID, run.StateCrashed, run.TransitionOptions{
			Reason: "engine resume failed", Actor: run.ActorPlatform,
			Detail: reasonDetail(err.Error()),
		})
		if terr != nil {
			return Outcome{}, fmt.Errorf("adapters: resume failed (%v) and crash transition failed: %w", err, terr)
		}
		return Outcome{Kind: OutcomeCrashed, Detail: err.Error()}, nil
	}
	if rec.Start.OnSession != nil {
		rec.Start.OnSession(sess)
	}
	return d.pump(ctx, sess, r, rec.Start)
}

// DriveStage runs ONE engine stage session inside an already-running run —
// the fresh-context-per-stage session model of Spec S05.3: a run whose work
// spans several stage sessions (intake ceremony, execute-N, verification)
// stays `running` across them, and each session is one engine invocation
// whose paid calls checkpoint against that run (D7 per paid call; the
// checkpoint store enforces the running/draining writability rule, Spec
// S02.4). No FSM transition happens here: session outcomes are stage
// results the CALLER disposes of — the run-level terminal mapping belongs
// to the dispatch path (Drive) or the owning pipeline. Stage sessions
// carry no gated tools, so a parked outcome is a caller-handled defect,
// never a run park.
func (d *Driver) DriveStage(ctx context.Context, a Adapter, req StartRequest) (Outcome, error) {
	r, err := d.Runs.Get(ctx, req.RunID)
	if err != nil {
		return Outcome{}, err
	}
	if r.State != run.StateRunning && r.State != run.StateDraining {
		return Outcome{}, fmt.Errorf("%w: stage session on %q in %s (want running/draining, S02.4)", ErrNotDrivable, r.ID, r.State)
	}
	// A stage session is an in-process engine call on an already-running run —
	// the exact shape (planner, critic, verifier) whose multi-minute quiet
	// stretches used to outlive the one-shot claim lease.
	defer d.Leases.Hold(ctx, r.ID, leaseHolder)()
	sess, err := a.Start(ctx, req)
	if err != nil {
		return Outcome{}, fmt.Errorf("adapters: stage session spawn: %w", err)
	}
	if req.OnSession != nil {
		req.OnSession(sess)
	}
	var pumpErr error
	for ev := range sess.Events() {
		if err := d.persistEvent(ctx, sess, r, req, ev); err != nil && pumpErr == nil {
			// Same discipline as pump: never wedge the engine subprocess on
			// a persistence failure; drain, remember the first error.
			pumpErr = err
		}
	}
	out, err := sess.Wait(ctx)
	if err != nil {
		return out, err
	}
	return out, pumpErr
}

// pump consumes the session's event stream, persisting as it goes, then
// maps the Outcome onto the run FSM.
func (d *Driver) pump(ctx context.Context, sess Session, r run.Run, req StartRequest) (Outcome, error) {
	var pumpErr error
	for ev := range sess.Events() {
		if err := d.persistEvent(ctx, sess, r, req, ev); err != nil && pumpErr == nil {
			// Persistence failures must not wedge the engine subprocess:
			// keep draining the channel, remember the first error.
			pumpErr = err
		}
	}
	out, err := sess.Wait(ctx)
	if err != nil {
		return out, err
	}
	if pumpErr != nil {
		return out, pumpErr
	}
	switch out.Kind {
	case OutcomeCompleted:
		_, err = d.Runs.Transition(ctx, r.ID, run.StateCompleted, run.TransitionOptions{
			Reason: "engine result (S03.1 done)", Actor: run.ActorPlatform,
		})
	case OutcomeParked:
		err = d.parkTx(ctx, r, out, run.StateParked)
	case OutcomeDiedAtGate:
		// S02.3: died-at-gate is reachable from parked only — the run
		// parks on the observed gate moment, then the engine budget
		// preempt kills the park (S03.4 ceiling ordering, measured:
		// no deferred_tool_use survives the budget exit).
		if err = d.parkTx(ctx, r, out, run.StateParked); err == nil {
			_, err = d.Runs.Transition(ctx, r.ID, run.StateDiedAtGate, run.TransitionOptions{
				Reason: "engine ceiling preempted the park (S03.4)", Actor: run.ActorPlatform,
			})
		}
	case OutcomeCrashed:
		_, err = d.Runs.Transition(ctx, r.ID, run.StateCrashed, run.TransitionOptions{
			Reason: "engine ended abnormally", Actor: run.ActorPlatform,
			Detail: reasonDetail(out.Detail),
		})
	case OutcomeCanceled:
		// No ratified S02.3 edge for cancel: the canceling surface owns
		// the row's disposition (see Drive doc comment).
	default:
		return out, fmt.Errorf("adapters: unknown outcome kind %q", out.Kind)
	}
	return out, err
}

// persistEvent lands one contract event durably: the run_events append
// (fenced at the run's generation), plus the per-kind side effects —
// checkpoint row + engine_sessions upkeep on a paid call, ask row on a
// gate ask. The invocation's OnEvent observer (Spec S05.3 stage-runtime
// seam) fires after the durable write succeeded — observers see only
// persisted facts.
func (d *Driver) persistEvent(ctx context.Context, sess Session, r run.Run, req StartRequest, ev Event) error {
	err := d.persistEventInner(ctx, sess, r, req, ev)
	if err == nil && req.OnEvent != nil {
		req.OnEvent(ev)
	}
	return err
}

func (d *Driver) persistEventInner(ctx context.Context, sess Session, r run.Run, req StartRequest, ev Event) error {
	payload := ev.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	switch {
	case ev.Kind == KindUsage && ev.Usage != nil && !ev.Usage.Total:
		// One paid call: checkpoint row in the same transaction as its
		// event append (Spec S02.4, D7), then session registry upkeep.
		return d.checkpoint(ctx, sess, r, req, ev, payload)
	case ev.Kind == KindGateAsk && ev.Ask != nil:
		// Durable ask-record from the moment observed (S03.4).
		return d.DB.WriteTx(ctx, func(tx *sql.Tx) error {
			if _, err := d.Log.AppendTx(ctx, tx, eventlog.Append{
				RunID: r.ID, Generation: r.Generation, UserID: r.UserID,
				Type: eventType(ev.Kind), SchemaVersion: engineEventSchemaVersion,
				Payload: payload,
			}); err != nil {
				return err
			}
			return d.upsertAskTx(ctx, tx, r, ev.Ask)
		})
	default:
		_, err := d.Log.Append(ctx, eventlog.Append{
			RunID: r.ID, Generation: r.Generation, UserID: r.UserID,
			Type: eventType(ev.Kind), SchemaVersion: engineEventSchemaVersion,
			Payload: payload,
		})
		return err
	}
}

// checkpoint writes the S02.4 checkpoint row for one paid call and then
// maintains the engine_sessions row + transcript copy-aside.
func (d *Driver) checkpoint(ctx context.Context, sess Session, r run.Run, req StartRequest, ev Event, payload json.RawMessage) error {
	cur := sess.Cursor()
	usageJSON := ev.Usage.Raw
	if len(usageJSON) == 0 {
		var err error
		if usageJSON, err = json.Marshal(ev.Usage); err != nil {
			return fmt.Errorf("adapters: marshal usage block: %w", err)
		}
	}
	snapshotRef := ""
	if d.Snapshot != nil {
		ref, err := d.Snapshot(ctx, r.ID)
		if err != nil {
			return fmt.Errorf("adapters: artifact snapshot (S02.4d): %w", err)
		}
		snapshotRef = ref
	}
	ledgerRev := ""
	if d.LedgerRevision != nil {
		rev, err := d.LedgerRevision(ctx, r.ID)
		if err != nil {
			return fmt.Errorf("adapters: ledger revision (S02.4c): %w", err)
		}
		ledgerRev = rev
	}
	var cp gates.Checkpoint
	err := d.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		// The usage event append rides the checkpoint write transaction
		// (S02.4: same transaction as its run-event append).
		if _, err := d.Log.AppendTx(ctx, tx, eventlog.Append{
			RunID: r.ID, Generation: r.Generation, UserID: r.UserID,
			Type: eventType(ev.Kind), SchemaVersion: engineEventSchemaVersion,
			Payload: payload,
		}); err != nil {
			return err
		}
		var err error
		cp, err = d.Checkpoints.WriteTx(ctx, tx, gates.NewCheckpoint{
			RunID: r.ID,
			Usage: usageJSON,

			SessionSubstrate: cur.Substrate,
			SessionID:        cur.SessionID,
			MessageIndex:     cur.MessageIndex,
			CwdKey:           cur.CwdKey,
			TranscriptPath:   cur.TranscriptPath,

			// (c) Ledger revision via the LedgerRevision seam (Spec S05.2).
			LedgerRevision: ledgerRev,
			// (d) artifact snapshot ref via the Snapshot seam.
			ArtifactSnapshotRef: snapshotRef,

			ModelID:               ev.Usage.ModelID,
			InvocationFingerprint: sess.Fingerprint(),
			ToolSchemaVersion:     req.Worker.ToolSchemaVersion,
			PromptSchemaVersion:   req.Worker.PromptSchemaVersion,
		})
		return err
	})
	if err != nil {
		return err
	}
	return d.maintainEngineSession(ctx, r, cur, cp)
}

// maintainEngineSession keeps the engine_sessions registry row current
// (Spec S02.2/S02.4: the reboot-case mining index, P-T07-2) and performs
// the per-checkpoint transcript copy-aside when a transcript exists.
func (d *Driver) maintainEngineSession(ctx context.Context, r run.Run, cur Cursor, cp gates.Checkpoint) error {
	copyPath := ""
	if d.CopyAsideDir != "" && cur.TranscriptPath != "" {
		p, err := d.copyAside(r.ID, cp.EventSeq, cur.TranscriptPath)
		if err != nil {
			// Copy-aside is insurance, not truth (P-T01-1: the platform
			// checkpoint row is authoritative); a failed copy must not
			// fail the paid-call bookkeeping. Record the miss.
			copyPath = ""
		} else {
			copyPath = p
		}
	}
	now := d.now().UTC().Format(time.RFC3339Nano)
	return d.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE engine_sessions
			    SET engine_session_id = ?, transcript_copy_path = CASE WHEN ? <> '' THEN ? ELSE transcript_copy_path END,
			        updated_ts = ?
			  WHERE run_id = ? AND substrate = ?`,
			cur.SessionID, copyPath, copyPath, now, r.ID, cur.Substrate)
		if err != nil {
			return fmt.Errorf("adapters: update engine_sessions: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO engine_sessions (run_id, user_id, substrate, engine_session_id, transcript_copy_path, created_ts, updated_ts)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				r.ID, r.UserID, cur.Substrate, cur.SessionID, copyPath, now, now); err != nil {
				return fmt.Errorf("adapters: insert engine_sessions: %w", err)
			}
		}
		return nil
	})
}

// copyAside copies the engine transcript to the platform-owned tree,
// named by the checkpoint's event_seq (event_seq is the sole ordering
// authority, Spec S02.5).
func (d *Driver) copyAside(runID string, eventSeq int64, transcriptPath string) (string, error) {
	dir := filepath.Join(d.CopyAsideDir, runID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, fmt.Sprintf("cp-%d.jsonl", eventSeq))
	src, err := os.Open(transcriptPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return dst, nil
}

// parkTx persists the park: the final ask/park snapshot on the asks row
// and the FSM transition, one transaction (Spec S02.3 discipline).
func (d *Driver) parkTx(ctx context.Context, r run.Run, out Outcome, to run.State) error {
	reason := "engine parked"
	if out.Park != nil {
		reason = "park: " + out.Park.Reason + " (S03.4)"
	}
	var detail json.RawMessage
	if out.Ask != nil {
		detail = reasonDetail(out.Ask.ID)
	}
	return d.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if out.Ask != nil {
			if err := d.upsertAskTx(ctx, tx, r, out.Ask); err != nil {
				return err
			}
		}
		_, err := d.Runs.TransitionTx(ctx, tx, r.ID, to, run.TransitionOptions{
			Reason: reason, Actor: run.ActorPlatform, Detail: detail,
		})
		return err
	})
}

// upsertAskTx writes the durable ask-record (Spec S02.2 asks family;
// S03.4: authoritative from the moment observed). Re-observation of the
// same engine ask (poll-resume re-defer) refreshes the snapshot and keeps
// the first observed_ts.
func (d *Driver) upsertAskTx(ctx context.Context, tx *sql.Tx, r run.Run, ask *Ask) error {
	if ask.ID == "" {
		return errors.New("adapters: gate ask without an id")
	}
	snapshot, err := json.Marshal(ask)
	if err != nil {
		return fmt.Errorf("adapters: marshal ask snapshot: %w", err)
	}
	now := d.now().UTC().Format(time.RFC3339Nano)
	engineExpiry := any(nil)
	if !ask.EngineExpiry.IsZero() {
		engineExpiry = ask.EngineExpiry.UTC().Format(time.RFC3339Nano)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts, engine_expiry_ts)
		 VALUES (?, ?, ?, ?, 'open', ?, ?)
		 ON CONFLICT (ask_id) DO UPDATE SET snapshot = excluded.snapshot,
		                                    status = 'open',
		                                    engine_expiry_ts = excluded.engine_expiry_ts`,
		ask.ID, r.ID, r.UserID, string(snapshot), now, engineExpiry)
	if err != nil {
		return fmt.Errorf("adapters: upsert ask %q: %w", ask.ID, err)
	}
	return nil
}

// markAskAnsweredTx records the answer on the ask row (answered_ts +
// answer payload) inside the resume transaction.
func (d *Driver) markAskAnsweredTx(ctx context.Context, tx *sql.Tx, ans *Answer) error {
	answer, err := json.Marshal(ans)
	if err != nil {
		return fmt.Errorf("adapters: marshal answer: %w", err)
	}
	now := d.now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx,
		`UPDATE asks SET status = 'answered', answered_ts = ?, answer = ? WHERE ask_id = ?`,
		now, string(answer), ans.AskID)
	if err != nil {
		return fmt.Errorf("adapters: answer ask %q: %w", ans.AskID, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("adapters: answer for unknown ask %q", ans.AskID)
	}
	return nil
}

func reasonDetail(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	b, err := json.Marshal(struct {
		Cause string `json:"cause"`
	}{s})
	if err != nil {
		return nil
	}
	return b
}
