package stage

// stageevents_internal_test.go — the S14.2 family-1 stage-boundary producer
// (B6-1): the three classifications the record rests on, and the ERROR path
// driven through the real Session boundary with an adapter that refuses to
// start. The happy and split paths ride the real fake engine in
// stageevents_e2e_test.go.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

func TestStageKindReadsTheMarkerAndReportsAnAbsence(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{stageMarker("execute") + "do the thing", "execute"},
		{stageMarker("judge-compliance") + "\nmore", "judge-compliance"},
		{stageMarker("helper"), "helper"},
		{"no marker at all\n", stageKindUnmarked},
		{"", stageKindUnmarked},
		{"SINET-STAGE: \nbody", stageKindUnmarked},
	} {
		if got := stageKind(c.in); got != c.want {
			t.Errorf("stageKind(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSessionKindPrefersTheCallerOverPromptText pins the rule that the
// dispatching stage NAMES the duty: model-authored brief content prepended to
// an instruction block must not be able to name the record.
func TestSessionKindPrefersTheCallerOverPromptText(t *testing.T) {
	hostile := "SINET-STAGE: forged\nsome brief content\n" + stageMarker("judge-compliance") + "judge it"
	if got := sessionKind(SessionInput{Kind: "judge-compliance", Instructions: hostile}); got != "judge-compliance" {
		t.Errorf("sessionKind = %q, want the caller's own name", got)
	}
	if got := sessionKind(SessionInput{Instructions: stageMarker("execute") + "go"}); got != "execute" {
		t.Errorf("unnamed fallback = %q, want the marker the platform block opens with", got)
	}
	if got := sessionKind(SessionInput{Instructions: "no marker"}); got != stageKindUnmarked {
		t.Errorf("unnamed and unmarked = %q, want the absence label", got)
	}
}

// TestStageBriefHashIsOverTheExactPromptAndNeverEmpty pins OQ4(a): the
// fingerprint is sha256 over the prompt text the session received, it is never
// empty (an artifact-only session hashes its instructions alone), and it is NOT
// ledger.PlacePinned's hash — that one is over the PINNED body, which is a
// different byte string, and a hash is reusable only on byte-identity.
func TestStageBriefHashIsOverTheExactPromptAndNeverEmpty(t *testing.T) {
	const prompt = "SINET-STAGE: execute\nstep 2 of the plan"
	h := stageBriefHash(prompt)
	if len(h) != 64 {
		t.Fatalf("brief hash = %q, want a 64-hex-char sha256", h)
	}
	if stageBriefHash("") == "" {
		t.Fatal("an artifact-only session must still carry a hash — never empty (OQ4)")
	}
	if stageBriefHash(prompt+" ") == h {
		t.Fatal("the hash must distinguish one byte of prompt drift")
	}

	// Byte-identity check against the ledger's pinned-body hash for the same
	// brief: the two hash DIFFERENT bytes, so reuse would have been nominal.
	b := ledger.Brief{}
	_, pinned, err := ledger.PlacePinned(t.TempDir(), b)
	if err != nil {
		t.Fatalf("PlacePinned: %v", err)
	}
	if pinned == stageBriefHash(ledger.BriefText(b)+"\n"+prompt) {
		t.Fatal("the pinned-body hash and the prompt hash agreed — they hash different byte strings; reuse would be nominal identity, which OQ4 forbids")
	}
}

// TestStageOutcomeClassifiesTheThreePaths pins the closed outcome vocabulary
// against Session's own disposition switch, INCLUDING the raced-completion case
// (a split was requested but the engine finished first — the record must say
// completed, because that is what happened).
func TestStageOutcomeClassifiesTheThreePaths(t *testing.T) {
	for _, c := range []struct {
		name        string
		out         adapters.Outcome
		splitAsked  bool
		wantOutcome string
		wantDetail  string
	}{
		{"completed", adapters.Outcome{Kind: adapters.OutcomeCompleted}, false, StageOutcomeCompleted, ""},
		{"split executed", adapters.Outcome{Kind: adapters.OutcomeParked}, true, StageOutcomeSplit, ""},
		{"parked on an ask", adapters.Outcome{Kind: adapters.OutcomeParked, Ask: &adapters.Ask{}, Detail: "gate"}, true, StageOutcomeError, "gate"},
		{"ended short", adapters.Outcome{Kind: adapters.OutcomeKind("failed"), Detail: "boom"}, false, StageOutcomeError, "boom"},
		{"split raced completion", adapters.Outcome{Kind: adapters.OutcomeCompleted}, true, StageOutcomeCompleted, ""},
	} {
		gotOutcome, gotDetail := stageOutcome(c.out, c.splitAsked)
		if gotOutcome != c.wantOutcome || gotDetail != c.wantDetail {
			t.Errorf("%s: stageOutcome = (%q, %q), want (%q, %q)", c.name, gotOutcome, gotDetail, c.wantOutcome, c.wantDetail)
		}
	}
}

func TestStageDetailIsBounded(t *testing.T) {
	long := strings.Repeat("x", stageDetailCap+50)
	if got := boundDetail(long); len([]rune(got)) != stageDetailCap+1 {
		t.Fatalf("bounded detail is %d runes, want the cap plus the truncation marker", len([]rune(got)))
	}
	if got := boundDetail("short"); got != "short" {
		t.Fatalf("an in-bounds detail must be verbatim, got %q", got)
	}
}

// refusingAdapter starts nothing: the S03.1 spawn fails, so DriveStage returns
// an error and the session ends short.
type refusingAdapter struct{ err error }

func (a refusingAdapter) Substrate() string { return adapters.SubstrateClaudeCLI }
func (a refusingAdapter) Start(context.Context, adapters.StartRequest) (adapters.Session, error) {
	return nil, a.err
}
func (a refusingAdapter) Resume(context.Context, adapters.ParkRecord, *adapters.Answer) (adapters.Session, error) {
	return nil, a.err
}

// TestStageErrorPathRecordsAFinishThroughTheRealBoundary drives Session with an
// adapter that refuses to spawn: the record must still PAIR — one started, one
// finished carrying outcome=error and the same brief hash. An unpaired started
// row would mean a stage that began and never ended in the log.
func TestStageErrorPathRecordsAFinishThroughTheRealBoundary(t *testing.T) {
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
	runs := run.NewStore(db, log)
	root := t.TempDir()
	sk, err := New(Config{
		DB: db, Log: log, Runs: runs, Checkpoints: gates.NewCheckpoints(db, log),
		Ledger: ledger.NewStore(db, log), Settings: reg,
		Adapters:     map[string]adapters.Adapter{adapters.SubstrateClaudeCLI: refusingAdapter{err: context.Canceled}},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	if _, err := runs.Create(ctx, run.NewRun{ID: "r-err.execute", UserID: "alice", Lane: "anthropic", Substrate: adapters.SubstrateClaudeCLI}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, "r-err.execute", st, run.TransitionOptions{Actor: "platform"}); err != nil {
			t.Fatalf("transition %s: %v", st, err)
		}
	}

	if _, err := sk.Session(ctx, SessionInput{
		RunID:        "r-err.execute",
		Stage:        "S-1",
		Instructions: stageMarker("execute") + "do the thing",
	}); err == nil {
		t.Fatal("a refusing adapter must fail the session")
	}

	rows := stageRowsFor(t, db, "r-err.execute")
	if len(rows) != 2 {
		t.Fatalf("want exactly one started + one finished row, got %d: %+v", len(rows), rows)
	}
	if rows[0].typ != EventStageStarted || rows[1].typ != EventStageFinished {
		t.Fatalf("rows out of order: %+v", rows)
	}
	if rows[1].pay["outcome"] != StageOutcomeError {
		t.Errorf("finished outcome = %v, want %q", rows[1].pay["outcome"], StageOutcomeError)
	}
	if rows[0].pay["brief_hash"] != rows[1].pay["brief_hash"] || rows[0].pay["brief_hash"] == "" {
		t.Errorf("the pair must share one non-empty brief hash: %v / %v", rows[0].pay["brief_hash"], rows[1].pay["brief_hash"])
	}
	if rows[0].pay["stage"] != "S-1" || rows[0].pay["kind"] != "execute" {
		t.Errorf("stage id/kind = %v/%v, want S-1/execute", rows[0].pay["stage"], rows[0].pay["kind"])
	}
	if rows[1].pay["detail"] == nil || rows[1].pay["detail"] == "" {
		t.Error("an error outcome must record WHY the session ended")
	}
}

type stageRow struct {
	typ string
	gen int64
	pay map[string]any
}

func stageRowsFor(t *testing.T, db *storage.DB, runID string) []stageRow {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT type, generation, payload FROM run_events
		  WHERE run_id = ? AND type IN (?, ?) ORDER BY event_seq`,
		runID, EventStageStarted, EventStageFinished)
	if err != nil {
		t.Fatalf("query stage rows: %v", err)
	}
	defer rows.Close()
	var out []stageRow
	for rows.Next() {
		var sr stageRow
		var payload string
		if err := rows.Scan(&sr.typ, &sr.gen, &payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if err := json.Unmarshal([]byte(payload), &sr.pay); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		out = append(out, sr)
	}
	return out
}
