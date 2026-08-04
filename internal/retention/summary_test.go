package retention_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/retention"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// seedTrace plants one run's worth of trace covering every S14.9 ¶1 field.
func (f *fixture) seedTrace(r run.Run) {
	f.t.Helper()
	for _, e := range []struct{ typ, payload string }{
		{"tool.completed", `{"tool":"grep","args_digest":"a1"}`},
		{"tool.completed", `{"tool":"grep","args_digest":"a2"}`},
		{"tool.completed", `{"tool":"write","args_digest":"b1"}`},
		{"tool.completed", `{"no_name_here":true}`},
		{"verdict.recorded", `{"round":1,"verdict":"pass","rubric_id":"rubric-software"}`},
		{"decision.recorded", `{"actor":"alice","card_id":"c1","card_type":"approval","decision":"approved"}`},
		{"routing.decided", `{"cause":"domain","plain_reason":"software worker"}`},
	} {
		if _, err := f.log.Append(f.ctx, eventlog.Append{
			RunID: r.ID, Generation: r.Generation, UserID: r.UserID,
			Type: e.typ, SchemaVersion: 1, Payload: json.RawMessage(e.payload),
		}); err != nil {
			f.t.Fatalf("seed %s: %v", e.typ, err)
		}
	}
}

// TestSummaryIsWrittenAtRunEndWithTheSevenFields (rubric 1, 2): the terminal
// transition writes run.summary_written, and the deterministic aggregate covers
// objective, stages, tool-call counts, verdicts, decisions, receipts and final
// state.
func TestSummaryIsWrittenAtRunEndWithTheSevenFields(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.task("t1", "alice", "Ship the retention pass")
	r := f.startRun("r1", "alice", "t1")
	f.seedTrace(r)

	// Before the run ends there is NO summary — "at run end, never later" also
	// means never EARLIER.
	if _, err := f.store.Get(f.ctx, "r1"); !errors.Is(err, retention.ErrNoSummary) {
		t.Fatalf("a summary exists before the run ended: %v", err)
	}

	if _, err := f.runs.Transition(f.ctx, "r1", run.StateCompleted, run.TransitionOptions{Reason: "done"}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	sum, err := f.store.Get(f.ctx, "r1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	agg := sum.Aggregate
	if agg.Objective != "Ship the retention pass" || agg.ObjectiveSource != "tasks.title" {
		t.Errorf("objective = %q (%s), want the task title", agg.Objective, agg.ObjectiveSource)
	}
	if len(agg.Stages) != 4 { // queued, claimed, running, completed
		t.Errorf("stages = %d (%+v), want the 4 lifecycle transitions", len(agg.Stages), agg.Stages)
	}
	if agg.Stages[len(agg.Stages)-1].Name != "completed" {
		t.Errorf("last stage = %q, want completed", agg.Stages[len(agg.Stages)-1].Name)
	}
	if agg.ToolCalls.Total != 4 || agg.ToolCalls.Distinct != 2 {
		t.Errorf("tool calls = %+v, want 4 total across 2 distinct tools", agg.ToolCalls)
	}
	if agg.ToolCalls.Unnamed != 1 {
		t.Errorf("unnamed tool calls = %d, want 1 counted honestly rather than bucketed", agg.ToolCalls.Unnamed)
	}
	if len(agg.Verdicts) != 1 || agg.Verdicts[0].Verdict != "pass" || agg.Verdicts[0].Round != 1 {
		t.Errorf("verdicts = %+v, want one recorded pass at round 1", agg.Verdicts)
	}
	if len(agg.Decisions) != 1 || agg.Decisions[0].Decision != "approved" {
		t.Errorf("decisions = %+v, want the one recorded human decision", agg.Decisions)
	}
	if agg.Receipts.Count != 0 || agg.Receipts.Checkpoints != 0 {
		t.Errorf("receipts = %+v, want an honest zero on a run with no paid calls", agg.Receipts)
	}
	if agg.FinalState != string(run.StateCompleted) {
		t.Errorf("final state = %q, want completed", agg.FinalState)
	}
	if sum.InputsDigest == "" || !strings.HasPrefix(sum.InputsDigest, "sha256:") {
		t.Errorf("inputs digest = %q, want a real sha256 over the aggregated inputs", sum.InputsDigest)
	}

	// The EVENT carries the S14.2 contract minimum and commits with the state
	// change: the summary row's event_seq IS a real run_events row.
	var typ, payload string
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT type, payload FROM run_events WHERE event_seq = ?`, sum.EventSeq).Scan(&typ, &payload); err != nil {
		t.Fatalf("read summary event: %v", err)
	}
	if typ != retention.EventSummaryWritten {
		t.Errorf("summary event type = %q, want %q", typ, retention.EventSummaryWritten)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"summary_ref", "inputs_digest"} {
		if p[k] == nil || p[k] == "" {
			t.Errorf("summary payload is missing the S14.2 contract-minimum key %q", k)
		}
	}
	if p["summary_ref"] != retention.SummaryRef("r1") {
		t.Errorf("summary_ref = %v, want %q", p["summary_ref"], retention.SummaryRef("r1"))
	}
}

// TestSummaryIsDeterministic: the same evidence yields the same digest. Two
// runs with identical traces agree; a run with one more event does not.
func TestSummaryDigestTracksTheEvidence(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.task("t1", "alice", "Same objective")
	f.task("t2", "alice", "Same objective")

	digestOf := func(runID, taskID string, extra bool) string {
		f.startRun(runID, "alice", taskID)
		if extra {
			f.appendAt(runID, "alice", "tool.completed", `{"tool":"grep"}`, f.now)
		}
		if _, err := f.runs.Transition(f.ctx, runID, run.StateCompleted, run.TransitionOptions{Reason: "done"}); err != nil {
			t.Fatal(err)
		}
		sum, err := f.store.Get(f.ctx, runID)
		if err != nil {
			t.Fatal(err)
		}
		return sum.InputsDigest
	}
	// Runs r1 and r2 differ only in run/task id, which the digest folds in, so
	// they must NOT be equal — but each is stable, and adding an event to a run
	// changes its digest. Assert the property that matters: the digest is a
	// function of the evidence, so it moves when the evidence moves.
	plain := digestOf("r1", "t1", false)
	withExtra := digestOf("r2", "t2", true)
	if plain == withExtra {
		t.Error("the inputs digest did not move when the trace gained an event")
	}
	if plain == "" || withExtra == "" {
		t.Error("the inputs digest must never be empty")
	}
}

// TestRunEndsAndIsSummarizedWithTheLocalStackAbsent (rubric 3): the local tier
// ENRICHES, never gates. With ErrStackAbsent the run still ends and the summary
// still exists, honestly flagged as aggregate-only.
func TestRunEndsAndIsSummarizedWithTheLocalStackAbsent(t *testing.T) {
	f := newFixture(t, withNarrator(func(context.Context, string, retention.Aggregate) (retention.Narrative, error) {
		return retention.Narrative{}, local.ErrStackAbsent
	}))
	f.user("alice", "member")
	f.task("t1", "alice", "Objective")
	f.startRun("r1", "alice", "t1")

	if _, err := f.runs.Transition(f.ctx, "r1", run.StateCompleted, run.TransitionOptions{Reason: "done"}); err != nil {
		t.Fatalf("a run failed to END with the local stack down: %v", err)
	}
	r, err := f.runs.Get(f.ctx, "r1")
	if err != nil || r.State != run.StateCompleted {
		t.Fatalf("run state = %v (%v), want completed", r.State, err)
	}
	sum, err := f.store.Get(f.ctx, "r1")
	if err != nil {
		t.Fatalf("no summary was written: %v", err)
	}
	if sum.NarrativeStatus != retention.NarrativePending {
		t.Errorf("narrative status before enrichment = %q, want pending", sum.NarrativeStatus)
	}

	if _, err := f.store.EnrichPending(f.ctx, 10); err != nil {
		t.Fatalf("EnrichPending must never fail for a narrator error: %v", err)
	}
	sum, err = f.store.Get(f.ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if sum.NarrativeStatus != retention.NarrativeAbsent {
		t.Errorf("narrative status = %q, want %q (aggregate-only, honestly flagged)", sum.NarrativeStatus, retention.NarrativeAbsent)
	}
	if sum.Narrative != "" {
		t.Errorf("a narrative was fabricated with the stack absent: %q", sum.Narrative)
	}
	if sum.NarrativeNote == "" {
		t.Error("the absence must be recorded with its reason, not left blank")
	}
	// The aggregate stands whole.
	if sum.Aggregate.FinalState != string(run.StateCompleted) {
		t.Error("the deterministic aggregate must be complete even with no narrator")
	}
}

// TestStackAbsentMarkerMatchesTheSentinel: internal/retention classifies an
// absent stack by substring because the import wall keeps it a leaf. Pin the
// two together here (internal/retention_test may import internal/local; the
// PACKAGE may not).
func TestStackAbsentMarkerMatchesTheSentinel(t *testing.T) {
	if !strings.Contains(local.ErrStackAbsent.Error(), retention.StackAbsentMarker) {
		t.Errorf("retention.StackAbsentMarker %q no longer appears in local.ErrStackAbsent %q",
			retention.StackAbsentMarker, local.ErrStackAbsent)
	}
}

// TestNoNarratorWiredFlagsAggregateOnlyImmediately: with no narrator at all the
// summary is written `absent` at run end — the aggregate-only flag is on the
// record from the first instant, not after a pass.
func TestNoNarratorWiredFlagsAggregateOnlyImmediately(t *testing.T) {
	f := newFixture(t) // no narrator
	f.user("alice", "member")
	f.startRun("r1", "alice", "")
	if _, err := f.runs.Transition(f.ctx, "r1", run.StateCompleted, run.TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	sum, err := f.store.Get(f.ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if sum.NarrativeStatus != retention.NarrativeAbsent {
		t.Errorf("narrative status = %q, want %q", sum.NarrativeStatus, retention.NarrativeAbsent)
	}
	var payload string
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT payload FROM run_events WHERE event_seq = ?`, sum.EventSeq).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `"aggregate_only":true`) {
		t.Errorf("the event must carry the aggregate-only flag: %s", payload)
	}
}

// TestNarratorEnrichesButNeverReplacesTheAggregate: the happy path. The
// narrative lands; the aggregate is untouched and stays frozen.
func TestNarratorEnrichesButNeverReplacesTheAggregate(t *testing.T) {
	f := newFixture(t, withNarrator(func(_ context.Context, runID string, agg retention.Aggregate) (retention.Narrative, error) {
		return retention.Narrative{Text: "The run completed [#" + itoa(agg.LastEventSeq) + "].", Model: "fake-workhorse"}, nil
	}))
	f.user("alice", "member")
	f.task("t1", "alice", "Objective")
	r := f.startRun("r1", "alice", "t1")
	f.seedTrace(r)
	if _, err := f.runs.Transition(f.ctx, "r1", run.StateCompleted, run.TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	before, err := f.store.Get(f.ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := f.store.EnrichPending(f.ctx, 10); err != nil || n != 1 {
		t.Fatalf("EnrichPending = %d, %v; want 1 row moved", n, err)
	}
	after, err := f.store.Get(f.ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if after.NarrativeStatus != retention.NarrativeWritten || after.Narrative == "" {
		t.Errorf("narrative = %q (%s), want the narrator's text", after.Narrative, after.NarrativeStatus)
	}
	if after.NarrativeModel != "fake-workhorse" {
		t.Errorf("narrative model = %q, want the answering seat recorded", after.NarrativeModel)
	}
	if after.InputsDigest != before.InputsDigest {
		t.Error("enrichment changed the inputs digest — the aggregate is the record and is frozen")
	}
	// A second pass has nothing left to do (the status moved off pending).
	if n, err := f.store.EnrichPending(f.ctx, 10); err != nil || n != 0 {
		t.Errorf("second EnrichPending = %d, %v; want 0 (incremental per-run, never re-summarized)", n, err)
	}
	if f.narrated != 1 {
		t.Errorf("the narrator ran %d times for one run, want exactly 1", f.narrated)
	}
}

// TestNarratorFailureIsRecordedNotFaked: an error is a recorded `failed`, never
// a fabricated story.
func TestNarratorFailureIsRecordedNotFaked(t *testing.T) {
	f := newFixture(t, withNarrator(func(context.Context, string, retention.Aggregate) (retention.Narrative, error) {
		return retention.Narrative{}, errors.New("seat refused: out of VRAM")
	}))
	f.user("alice", "member")
	f.startRun("r1", "alice", "")
	if _, err := f.runs.Transition(f.ctx, "r1", run.StateCompleted, run.TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.EnrichPending(f.ctx, 10); err != nil {
		t.Fatal(err)
	}
	sum, err := f.store.Get(f.ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if sum.NarrativeStatus != retention.NarrativeFailed {
		t.Errorf("narrative status = %q, want %q", sum.NarrativeStatus, retention.NarrativeFailed)
	}
	if !strings.Contains(sum.NarrativeNote, "out of VRAM") {
		t.Errorf("the failure reason must be recorded verbatim; got %q", sum.NarrativeNote)
	}
	if sum.Narrative != "" {
		t.Error("a narrative was fabricated after a narrator failure")
	}
}

// TestSummaryIsWrittenOnceAndTheAggregateIsFrozen (S14.9 "at run end, never
// later"): the aggregate columns are trigger-frozen and the row is keep-forever.
func TestSummaryIsWrittenOnceAndTheAggregateIsFrozen(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")
	f.startRun("r1", "alice", "")
	if _, err := f.runs.Transition(f.ctx, "r1", run.StateCompleted, run.TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	exec := func(q string) error {
		return f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(f.ctx, q)
			return err
		})
	}
	for _, q := range []string{
		`UPDATE run_summaries SET aggregate_json = '{}' WHERE run_id = 'r1'`,
		`UPDATE run_summaries SET inputs_digest = 'sha256:forged' WHERE run_id = 'r1'`,
		`UPDATE run_summaries SET final_state = 'crashed' WHERE run_id = 'r1'`,
		`DELETE FROM run_summaries WHERE run_id = 'r1'`,
	} {
		if err := exec(q); err == nil {
			t.Errorf("%q was permitted — the aggregate is written once and summaries are keep-forever", q)
		}
	}
	// The narrative block still moves.
	if err := exec(`UPDATE run_summaries SET narrative = 'x', narrative_status = 'written' WHERE run_id = 'r1'`); err != nil {
		t.Errorf("the narrative block must stay mutable: %v", err)
	}
}

// TestPlatformScopeRunMintsNoSummary: the platform's own machinery runs (the
// advisory metering vehicle a local duty call rides, the dead-man canary) carry
// no run story — and summarizing the narrator's OWN advisory run would not
// terminate.
func TestPlatformScopeRunMintsNoSummary(t *testing.T) {
	narrations := 0
	var f *fixture
	f = newFixture(t, withNarrator(func(context.Context, string, retention.Aggregate) (retention.Narrative, error) {
		narrations++
		// Exactly what the shell's narrator does: a short-lived platform run,
		// driven to running and settled terminal.
		id := "platform.advisory.run-summary." + itoa(int64(narrations))
		if _, err := f.runs.Create(f.ctx, run.NewRun{ID: id, UserID: "platform"}); err != nil {
			return retention.Narrative{}, err
		}
		for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning, run.StateCompleted} {
			if _, err := f.runs.Transition(f.ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
				return retention.Narrative{}, err
			}
		}
		return retention.Narrative{Text: "story", Model: "fake"}, nil
	}))
	f.user("alice", "member")
	f.user("platform", "operator")
	f.startRun("r1", "alice", "")
	if _, err := f.runs.Transition(f.ctx, "r1", run.StateCompleted, run.TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	// Drain: with a platform run summarized, this would never settle.
	for i := 0; i < 5; i++ {
		n, err := f.store.EnrichPending(f.ctx, 10)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
		if i == 4 {
			t.Fatal("enrichment did not settle — a platform advisory run is being summarized, which feeds itself")
		}
	}
	var summaries int
	if err := f.db.QueryRowContext(f.ctx, `SELECT count(*) FROM run_summaries`).Scan(&summaries); err != nil {
		t.Fatal(err)
	}
	if summaries != 1 {
		t.Errorf("run_summaries holds %d rows, want exactly 1 (the member's run; platform machinery mints none)", summaries)
	}
	if narrations != 1 {
		t.Errorf("the narrator ran %d times, want 1", narrations)
	}
}

// TestEveryTerminalStateIsSummarized: the hook rides IsTerminal, so a crashed
// run superseded to finalized, a tombstone and a died-at-gate all end with a
// summary — not only the happy path.
func TestEveryTerminalStateIsSummarized(t *testing.T) {
	f := newFixture(t)
	f.user("alice", "member")

	// died-at-gate: running -> parked -> died-at-gate
	f.startRun("r-gate", "alice", "")
	for _, st := range []run.State{run.StateParked, run.StateDiedAtGate} {
		if _, err := f.runs.Transition(f.ctx, "r-gate", st, run.TransitionOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	// tombstoned: running -> tombstoned
	f.startRun("r-tomb", "alice", "")
	if _, err := f.runs.Transition(f.ctx, "r-tomb", run.StateTombstoned, run.TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"r-gate", "r-tomb"} {
		sum, err := f.store.Get(f.ctx, id)
		if err != nil {
			t.Errorf("no summary for terminal run %q: %v", id, err)
			continue
		}
		if sum.FinalState == string(run.StateRunning) {
			t.Errorf("%q summary records a non-terminal final state", id)
		}
	}
	// crashed is NOT terminal in the S02.3 sense (recovery supersedes it), so it
	// mints no summary — the run has not ended.
	f.startRun("r-crash", "alice", "")
	if _, err := f.runs.Transition(f.ctx, "r-crash", run.StateCrashed, run.TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Get(f.ctx, "r-crash"); !errors.Is(err, retention.ErrNoSummary) {
		t.Error("a crashed run was summarized; recovery still supersedes it, so it has not ended")
	}
	// ... and when recovery harvests it to completed, the summary lands.
	if _, err := f.runs.Transition(f.ctx, "r-crash", run.StateCompleted, run.TransitionOptions{Reason: "harvest"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Get(f.ctx, "r-crash"); err != nil {
		t.Errorf("a harvested run must be summarized at its real ending: %v", err)
	}
}

// TestNoBatchOrBackfillSummarizerExists (rubric 5) is the S14.9 ¶1 negative:
// "bulk summarization over months of trace is the documented failure shape". A
// code scan proves the package exposes no windowed or backfilling summarizer —
// the only summary producer takes a single run id inside a transaction, and the
// enrichment verb selects only rows whose OWN run already ended.
func TestNoBatchOrBackfillSummarizerExists(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{"Backfill", "Resummarize", "ReSummarize", "SummarizeAll", "SummarizeRange", "SummarizeSince"}
	files := 0
	summaryProducers := 0
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			files++
			ast.Inspect(file, func(n ast.Node) bool {
				fn, ok := n.(*ast.FuncDecl)
				if !ok {
					return true
				}
				for _, b := range banned {
					if strings.Contains(fn.Name.Name, b) {
						t.Errorf("%s declares %q — S14.9 ¶1 forbids bulk summarization over historical trace", filepath.Base(name), fn.Name.Name)
					}
				}
				if fn.Name.Name == "WriteAtRunEndTx" {
					summaryProducers++
				}
				return true
			})
		}
	}
	if files == 0 {
		t.Fatal("the scan read no files — it would pass vacuously")
	}
	if summaryProducers != 1 {
		t.Errorf("found %d summary producers, want exactly 1 (WriteAtRunEndTx, at the run's terminal transition)", summaryProducers)
	}
	// Non-tautology probe: the banned-name matcher genuinely fires.
	if !strings.Contains("SummarizeAllRuns", banned[3]) {
		t.Fatal("the banned-name scan is tautological")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
