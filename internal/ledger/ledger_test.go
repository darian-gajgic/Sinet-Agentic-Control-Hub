package ledger_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S05 acceptance suite: ledger_update events persisted and replayable
// (D7 self-containment — state reconstructs from the event stream alone),
// the S05.1 write rules enforced at the write layer, generation fencing,
// and the payload-cap bound on the small-by-design document.

type fix struct {
	db    *storage.DB
	log   *eventlog.Log
	runs  *run.Store
	store *ledger.Store
}

func newFix(t *testing.T) *fix {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	return &fix{db: db, log: log, runs: run.NewStore(db, log), store: ledger.NewStore(db, log)}
}

func (f *fix) task(t *testing.T, taskID, userID string) {
	t.Helper()
	err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, ?, ?, ?)`,
			taskID, userID, "t", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

func (f *fix) run(t *testing.T, runID, taskID string) run.Run {
	t.Helper()
	r, err := f.runs.Create(context.Background(), run.NewRun{
		ID: runID, UserID: "u1", TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return r
}

func objective(spec string) ledger.ObjectiveAC {
	return ledger.ObjectiveAC{
		Objective: "Ship the demo",
		AcceptanceCriteria: []ledger.AcceptanceCriterion{
			{N: 1, Plain: "the demo runs", Structured: "exit_code == 0"},
			{N: 2, Plain: "the receipt shows usage"},
		},
		SpecVersion: spec,
	}
}

func constraints(spec string) ledger.ConstraintsDangerZones {
	return ledger.ConstraintsDangerZones{
		Constraints: []string{"never push to main"},
		DangerZones: []ledger.DangerZone{{Path: "Docs/**", Rule: "read-only", SourceHash: "abc"}},
		SpecVersion: spec,
	}
}

// seedLedger drives a full write sequence and returns the final document.
func seedLedger(t *testing.T, f *fix, runID string, gen int64) ledger.Document {
	t.Helper()
	ctx := context.Background()
	if _, err := f.store.SetObjective(ctx, runID, "platform", objective("spec-v1")); err != nil {
		t.Fatalf("SetObjective: %v", err)
	}
	if _, err := f.store.SetConstraints(ctx, runID, "platform", constraints("spec-v1")); err != nil {
		t.Fatalf("SetConstraints: %v", err)
	}
	v := f.store.SessionVerbs(runID, "execute-1", gen)
	if _, err := v.Decide(ctx, "use sqlite", "ratified by spec", 0); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if _, err := v.State(ctx, ledger.StateUpdate{Upserts: []ledger.WorkItem{
		{ID: "w1", Summary: "build the thing", ACRefs: []int{1}, Status: ledger.StatusInProgress},
	}}); err != nil {
		t.Fatalf("State: %v", err)
	}
	if _, err := v.State(ctx, ledger.StateUpdate{
		Upserts:     []ledger.WorkItem{{ID: "w1", Status: ledger.StatusDoneUnverified, EvidenceRef: "test log"}},
		NextActions: &[]string{"verify w1"},
	}); err != nil {
		t.Fatalf("State 2: %v", err)
	}
	if _, err := v.Artifact(ctx, "dist/demo.tar", "file",
		"the built demo", strings.Repeat("ab", 32)); err != nil {
		t.Fatalf("Artifact: %v", err)
	}
	if _, err := v.Note(ctx, "flag X was needed"); err != nil {
		t.Fatalf("Note: %v", err)
	}
	if _, err := f.store.SetVerified(ctx, runID, "platform", "w1", "verdict:v1"); err != nil {
		t.Fatalf("SetVerified: %v", err)
	}
	if _, err := f.store.RecordDecision(ctx, runID, ledger.AuthorHuman, "u1", "execute-1",
		"approved the artifact", "operator answer", 0); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}
	doc, err := f.store.SetPlanVersion(ctx, runID, "platform", "plan-v1")
	if err != nil {
		t.Fatalf("SetPlanVersion: %v", err)
	}
	return doc
}

func docJSON(t *testing.T, d ledger.Document) string {
	t.Helper()
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal doc: %v", err)
	}
	return string(raw)
}

func TestLedgerLifecycleAndReplay(t *testing.T) {
	f := newFix(t)
	f.task(t, "t1", "u1")
	r := f.run(t, "r1", "t1")
	ctx := context.Background()

	final := seedLedger(t, f, r.ID, r.Generation)
	if final.LedgerVersion != 10 {
		t.Fatalf("ledger_version = %d, want 10 (monotonic, bumped on every accepted write)", final.LedgerVersion)
	}
	if final.SpecVersion != "spec-v1" || final.PlanVersion != "plan-v1" {
		t.Fatalf("header versions = %q/%q", final.SpecVersion, final.PlanVersion)
	}
	if len(final.Decisions) != 2 || final.Decisions[0].Author != ledger.AuthorCoordinator || final.Decisions[1].Author != ledger.AuthorHuman {
		t.Fatalf("decisions = %+v", final.Decisions)
	}
	if len(final.Artifacts) != 1 || final.Artifacts[0].ID != "a1" || final.Artifacts[0].ProducingStage != "execute-1" {
		t.Fatalf("artifacts = %+v", final.Artifacts)
	}
	if got := final.State.Items[0]; got.Status != ledger.StatusVerified || got.EvidenceRef != "verdict:v1" {
		t.Fatalf("w1 = %+v", got)
	}

	// Every accepted write is one ledger_update event with the FULL document
	// (Spec S02.2): versions dense 1..10 in event order.
	events, err := f.log.After(ctx, 0, 1000)
	if err != nil {
		t.Fatalf("After: %v", err)
	}
	var versions []int64
	var lastPayload struct {
		Change      ledger.Change   `json:"change"`
		ContentHash string          `json:"content_hash"`
		Ledger      ledger.Document `json:"ledger"`
	}
	for _, e := range events {
		if e.Type != ledger.EventLedgerUpdate {
			continue
		}
		if e.RunID != r.ID || e.UserID != "u1" {
			t.Fatalf("event attribution: run=%q user=%q", e.RunID, e.UserID)
		}
		if err := json.Unmarshal(e.Payload, &lastPayload); err != nil {
			t.Fatalf("payload: %v", err)
		}
		versions = append(versions, lastPayload.Ledger.LedgerVersion)
	}
	if len(versions) != 10 {
		t.Fatalf("ledger_update events = %d, want 10", len(versions))
	}
	for i, v := range versions {
		if v != int64(i)+1 {
			t.Fatalf("versions = %v, want dense 1..10", versions)
		}
	}
	if lastPayload.Change.Verb != ledger.VerbPlanVersion {
		t.Fatalf("last change verb = %q", lastPayload.Change.Verb)
	}

	// D7 self-containment: a fresh Store reconstructs the ledger from the
	// event stream alone and matches the last accepted write exactly.
	replayed, found, err := ledger.NewStore(f.db, f.log).Current(ctx, "t1")
	if err != nil || !found {
		t.Fatalf("Current: %v found=%v", err, found)
	}
	if docJSON(t, replayed) != docJSON(t, final) {
		t.Fatalf("replayed document diverges from the accepted write:\n%s\nvs\n%s", docJSON(t, replayed), docJSON(t, final))
	}
	if docJSON(t, replayed) != docJSON(t, lastPayload.Ledger) {
		t.Fatalf("event payload document diverges from Current")
	}

	// Point reads at any revision (the replay/recovery read).
	at5, err := f.store.AtVersion(ctx, "t1", 5)
	if err != nil {
		t.Fatalf("AtVersion(5): %v", err)
	}
	if at5.LedgerVersion != 5 || len(at5.Decisions) != 1 {
		t.Fatalf("AtVersion(5) = v%d decisions=%d", at5.LedgerVersion, len(at5.Decisions))
	}
	if _, err := f.store.AtVersion(ctx, "t1", 99); !errors.Is(err, ledger.ErrVersionNotFound) {
		t.Fatalf("AtVersion(99) err = %v", err)
	}

	// Checkpoint block (c): version + content hash of the current revision.
	ref, err := f.store.CheckpointRef(ctx, r.ID)
	if err != nil {
		t.Fatalf("CheckpointRef: %v", err)
	}
	parsed, ok, err := ledger.ParseRevisionRef(ref)
	if err != nil || !ok {
		t.Fatalf("ParseRevisionRef(%q): %v ok=%v", ref, err, ok)
	}
	if parsed.LedgerVersion != 10 || parsed.SHA256 != lastPayload.ContentHash || len(parsed.SHA256) != 64 {
		t.Fatalf("revision ref = %+v, payload hash %q", parsed, lastPayload.ContentHash)
	}
}

func TestWriteRules(t *testing.T) {
	f := newFix(t)
	f.task(t, "t1", "u1")
	r := f.run(t, "r1", "t1")
	ctx := context.Background()

	if _, err := f.store.SetObjective(ctx, r.ID, "platform", objective("spec-v1")); err != nil {
		t.Fatalf("SetObjective: %v", err)
	}
	// Pinned §1: immutable under the same spec version.
	if _, err := f.store.SetObjective(ctx, r.ID, "platform", objective("spec-v1")); !errors.Is(err, ledger.ErrPinnedImmutable) {
		t.Fatalf("re-set objective err = %v, want ErrPinnedImmutable", err)
	}
	// A new spec version through re-approval is the one sanctioned path.
	doc, err := f.store.SetObjective(ctx, r.ID, "platform", objective("spec-v2"))
	if err != nil || doc.SpecVersion != "spec-v2" {
		t.Fatalf("re-approval: %v spec=%q", err, doc.SpecVersion)
	}
	// Pinned §2: spec version must match the header; then same rule as §1.
	if _, err := f.store.SetConstraints(ctx, r.ID, "platform", constraints("spec-v1")); !errors.Is(err, ledger.ErrSpecVersionMismatch) {
		t.Fatalf("constraints wrong spec err = %v", err)
	}
	if _, err := f.store.SetConstraints(ctx, r.ID, "platform", constraints("spec-v2")); err != nil {
		t.Fatalf("SetConstraints: %v", err)
	}
	if _, err := f.store.SetConstraints(ctx, r.ID, "platform", constraints("spec-v2")); !errors.Is(err, ledger.ErrPinnedImmutable) {
		t.Fatalf("constraints re-set err = %v, want ErrPinnedImmutable", err)
	}

	v := f.store.SessionVerbs(r.ID, "execute-1", r.Generation)
	if _, err := v.State(ctx, ledger.StateUpdate{Upserts: []ledger.WorkItem{
		{ID: "w1", Summary: "work", Status: ledger.StatusDoneUnverified},
	}}); err != nil {
		t.Fatalf("State: %v", err)
	}
	// The ratified write-layer rejection: a session-claimed "verified" is
	// structurally impossible (Spec S05.1 §4, G1 Def.12).
	if _, err := v.State(ctx, ledger.StateUpdate{Upserts: []ledger.WorkItem{
		{ID: "w1", Status: ledger.StatusVerified},
	}}); !errors.Is(err, ledger.ErrVerifiedFromSession) {
		t.Fatalf("session verified err = %v, want ErrVerifiedFromSession", err)
	}
	// Verified requires done_unverified + a verdict evidence ref.
	if _, err := f.store.SetVerified(ctx, r.ID, "platform", "w1", ""); !errors.Is(err, ledger.ErrInvalidWrite) {
		t.Fatalf("SetVerified without evidence err = %v", err)
	}
	if _, err := f.store.SetVerified(ctx, r.ID, "platform", "w1", "verdict:1"); err != nil {
		t.Fatalf("SetVerified: %v", err)
	}
	if _, err := f.store.SetVerified(ctx, r.ID, "platform", "w1", "verdict:2"); !errors.Is(err, ledger.ErrNotVerifiable) {
		t.Fatalf("re-verify err = %v", err)
	}
	// A verified item is closed to session writes.
	if _, err := v.State(ctx, ledger.StateUpdate{Upserts: []ledger.WorkItem{
		{ID: "w1", Status: ledger.StatusPending},
	}}); !errors.Is(err, ledger.ErrVerifiedItemProtected) {
		t.Fatalf("session write on verified item err = %v", err)
	}
	// Decisions: supersedes must cite an existing seq; control-plane authors
	// are human|platform only.
	if _, err := v.Decide(ctx, "undo", "because", 42); !errors.Is(err, ledger.ErrUnknownSupersedes) {
		t.Fatalf("supersedes err = %v", err)
	}
	if _, err := f.store.RecordDecision(ctx, r.ID, ledger.AuthorCoordinator, "u1", "s", "x", "y", 0); !errors.Is(err, ledger.ErrInvalidWrite) {
		t.Fatalf("control-plane coordinator author err = %v", err)
	}
	// ac_refs must cite existing criteria.
	if _, err := v.State(ctx, ledger.StateUpdate{Upserts: []ledger.WorkItem{
		{ID: "w2", Summary: "more", ACRefs: []int{9}},
	}}); !errors.Is(err, ledger.ErrInvalidWrite) {
		t.Fatalf("bad ac_ref err = %v", err)
	}

	// A run without a task cannot hold a ledger.
	rt, err := f.runs.Create(ctx, run.NewRun{ID: "r-notask", UserID: "u1"})
	if err != nil {
		t.Fatalf("create taskless run: %v", err)
	}
	if _, err := f.store.SetObjective(ctx, rt.ID, "platform", objective("s")); !errors.Is(err, ledger.ErrNoTask) {
		t.Fatalf("taskless err = %v", err)
	}
	if ref, err := f.store.CheckpointRef(ctx, rt.ID); err != nil || ref != "" {
		t.Fatalf("taskless CheckpointRef = %q, %v", ref, err)
	}
}

func TestGenerationFencing(t *testing.T) {
	f := newFix(t)
	f.task(t, "t1", "u1")
	r := f.run(t, "r1", "t1")
	ctx := context.Background()
	if _, err := f.store.SetObjective(ctx, r.ID, "platform", objective("spec-v1")); err != nil {
		t.Fatalf("SetObjective: %v", err)
	}

	stale := f.store.SessionVerbs(r.ID, "execute-1", r.Generation)
	// A takeover bumps the run's generation; the superseded session's
	// asserted generation is now stale and its writes are fenced out
	// (Spec S02.5 step 4).
	err := f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := f.runs.BumpGenerationTx(ctx, tx, r.ID)
		return err
	})
	if err != nil {
		t.Fatalf("bump generation: %v", err)
	}
	if _, err := stale.Decide(ctx, "zombie write", "should be fenced", 0); !errors.Is(err, eventlog.ErrStaleGeneration) {
		t.Fatalf("stale verb err = %v, want ErrStaleGeneration", err)
	}
	// The successor session at the new generation writes normally, and the
	// control plane (current-generation acts) is unaffected.
	if _, err := f.store.SessionVerbs(r.ID, "execute-1", r.Generation+1).Decide(ctx, "successor", "at new generation", 0); err != nil {
		t.Fatalf("successor verb: %v", err)
	}
	if _, err := f.store.SetPlanVersion(ctx, r.ID, "platform", "plan-v1"); err != nil {
		t.Fatalf("control-plane write after bump: %v", err)
	}
}

func TestPayloadCapBoundsTheDocument(t *testing.T) {
	f := newFix(t)
	f.task(t, "t1", "u1")
	r := f.run(t, "r1", "t1")
	ctx := context.Background()
	if _, err := f.store.SetObjective(ctx, r.ID, "platform", objective("spec-v1")); err != nil {
		t.Fatalf("SetObjective: %v", err)
	}
	// ⚙ state.event_payload_cap (default 64 KB) is the structural bound on
	// the small-by-design ledger (Spec S02.2; P-T07-5): an oversize write is
	// rejected at the validate-before-persist gate, not truncated.
	v := f.store.SessionVerbs(r.ID, "execute-1", r.Generation)
	if _, err := v.Note(ctx, strings.Repeat("x", 70_000)); !errors.Is(err, eventlog.ErrPayloadTooLarge) {
		t.Fatalf("oversize note err = %v, want ErrPayloadTooLarge", err)
	}
	// The rejected write left no revision behind.
	doc, found, err := f.store.Current(ctx, "t1")
	if err != nil || !found || doc.LedgerVersion != 1 {
		t.Fatalf("after rejection: v%d found=%v err=%v", doc.LedgerVersion, found, err)
	}
}

func TestMultiRunContinuation(t *testing.T) {
	f := newFix(t)
	f.task(t, "t1", "u1")
	rA := f.run(t, "r1", "t1")
	ctx := context.Background()
	if _, err := f.store.SetObjective(ctx, rA.ID, "platform", objective("spec-v1")); err != nil {
		t.Fatalf("SetObjective: %v", err)
	}
	// A successor run of the same task continues the same ledger — one
	// artifact per task (Spec S05.1), reconstructed across the fork.
	rB := f.run(t, "r1.g1", "t1")
	if _, err := f.store.SessionVerbs(rB.ID, "execute-2", rB.Generation).Note(ctx, "picked up after fork"); err != nil {
		t.Fatalf("successor note: %v", err)
	}
	doc, found, err := f.store.Current(ctx, "t1")
	if err != nil || !found {
		t.Fatalf("Current: %v", err)
	}
	if doc.LedgerVersion != 2 || len(doc.LearnedThisTask) != 1 {
		t.Fatalf("continued ledger = v%d learned=%d", doc.LedgerVersion, len(doc.LearnedThisTask))
	}
	ref, err := f.store.CheckpointRef(ctx, rB.ID)
	if err != nil {
		t.Fatalf("CheckpointRef: %v", err)
	}
	parsed, _, err := ledger.ParseRevisionRef(ref)
	if err != nil || parsed.LedgerVersion != 2 {
		t.Fatalf("successor ref = %+v, %v", parsed, err)
	}
}

func TestStageCloseGate(t *testing.T) {
	doc := ledger.Document{State: ledger.WorkState{
		Items: []ledger.WorkItem{
			{ID: "w1", Status: ledger.StatusDoneUnverified},
			{ID: "w2", Status: ledger.StatusInProgress},
			{ID: "w3", Status: ledger.StatusBlocked},
			{ID: "w4", Status: ledger.StatusPending},
		},
		NextActions: []string{"w4"},
	}}
	// w2 is neither closed nor handed forward: the stage cannot close.
	err := ledger.CheckStageClose(doc, []string{"w1", "w2", "w3", "w4"})
	if !errors.Is(err, ledger.ErrStageIncomplete) || !strings.Contains(err.Error(), "w2") {
		t.Fatalf("err = %v, want ErrStageIncomplete naming w2", err)
	}
	// Handed forward via next_actions accounts for it.
	doc.State.NextActions = append(doc.State.NextActions, "w2")
	if err := ledger.CheckStageClose(doc, []string{"w1", "w2", "w3", "w4"}); err != nil {
		t.Fatalf("accounted close: %v", err)
	}
	if err := ledger.CheckStageClose(doc, []string{"nope"}); !errors.Is(err, ledger.ErrUnknownItem) {
		t.Fatalf("unknown item err = %v", err)
	}
}
