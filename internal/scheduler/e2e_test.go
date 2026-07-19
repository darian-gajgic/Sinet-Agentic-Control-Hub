package scheduler_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// TestEndToEndDevMode is the B1-2 acceptance evidence: a run created → queued →
// CAS-claimed by the scheduler under a lane cap → driven through the B1-1
// engine path (real adapters.Driver + a fake engine, no paid calls) → checkpoint
// usage → receipt materialized at run-end, with the whole event trail readable
// on the /events SSE endpoint.
func TestEndToEndDevMode(t *testing.T) {
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	runs := run.NewStore(db, log)
	cps := gates.NewCheckpoints(db, log)

	// The B1-1 engine path: the real Driver behind a fake in-memory engine.
	driver := &adapters.Driver{Runs: runs, Checkpoints: cps, Log: log, DB: db,
		CopyAsideDir: filepath.Join(t.TempDir(), "copy-aside")}
	dispatcher := &e2eDispatcher{driver: driver, adapter: newFakeEngine()}

	priceTable := metering.NewEffectiveDatedTable("empty-v0")
	exceptions := metering.NoMeteredExceptions()
	ledger := metering.NewLedger(db, priceTable, exceptions, reg)
	sched, err := scheduler.New(scheduler.Config{
		DB: db, Runs: runs, Settings: reg, Dispatcher: dispatcher,
		Pressure: metering.NewPressureGauge(db, reg),
		Receipts: metering.NewReceipts(db, ledger, exceptions),
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}

	// Create the run and cap the lane at 1 concurrent run (Spec S10.7 slots).
	if _, err := runs.Create(ctx, run.NewRun{ID: "run-e2e", UserID: "alice",
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sched.SetLaneCap(ctx, "alice", adapters.LaneAnthropic, 1); err != nil {
		t.Fatalf("SetLaneCap: %v", err)
	}
	// Enqueue (the sole ingress, owner-attributed) then admit via one claim pass.
	if err := sched.Enqueue(ctx, "run-e2e", scheduler.ClassInteractive); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	n, err := sched.Tick(ctx)
	if err != nil || n != 1 {
		t.Fatalf("Tick claimed %d err=%v, want 1", n, err)
	}
	sched.WaitInFlight()

	// The run completed, a checkpoint carries the TTL-bucketed usage, and a
	// receipt materialized at run-end.
	r, _ := runs.Get(ctx, "run-e2e")
	if r.State != run.StateCompleted {
		t.Fatalf("run state %s, want completed", r.State)
	}
	cp, ok, err := cps.Last(ctx, "run-e2e")
	if err != nil || !ok {
		t.Fatalf("checkpoint: ok=%v err=%v", ok, err)
	}
	t.Logf("checkpoint usage: %s (model %s)", cp.Usage, cp.ModelID)

	var receiptJSON string
	if err := db.QueryRowContext(ctx, `SELECT usage_json FROM receipts WHERE run_id='run-e2e'`).Scan(&receiptJSON); err != nil {
		t.Fatalf("receipt not materialized: %v", err)
	}
	var receipt metering.Receipt
	if err := json.Unmarshal([]byte(receiptJSON), &receipt); err != nil {
		t.Fatalf("receipt decode: %v", err)
	}
	if receipt.UserID != "alice" || len(receipt.Items) != 1 || receipt.Items[0].Calls != 1 {
		t.Fatalf("receipt = %+v", receipt)
	}
	if !receipt.DirectUse.Unpriced || receipt.DirectUse.Label != metering.DirectUseLabel {
		t.Fatalf("done-directly line = %+v (want labelled + UNPRICED at v0)", receipt.DirectUse)
	}
	t.Logf("receipt: billed=%s items=%d currency=%s worst_tier=%d total_priced=$%.4f unpriced_calls=%d",
		receipt.UserID, len(receipt.Items), receipt.Currency, receipt.WorstTier, receipt.TotalPricedUSD, receipt.TotalUnpricedCalls)
	t.Logf("done-directly: %q ref=%s unpriced=%v", receipt.DirectUse.Label, receipt.DirectUse.FormulaRef, receipt.DirectUse.Unpriced)

	// The whole trail is readable on /events (dev-posture identity).
	srv := api.New(api.Config{
		Log: log, Sessions: auth.New(db, log), DevPosture: true, Settings: reg,
		HealthFn: func() api.Health { return api.Health{Ready: true, Mode: "running", Version: "test"} },
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	trail := readEventTrail(t, ts.URL+"/events?after_seq=0", 10)
	t.Logf("== /events trail (%d frames) ==", len(trail))
	for _, f := range trail {
		t.Logf("  id=%s %-16s %s", f.id, f.event, f.data)
	}
	assertTrail(t, trail, []string{
		"run.created",    // new
		"run.state",      // new→queued (Enqueue)
		"run.state",      // queued→claimed (scheduler CAS)
		"run.state",      // claimed→running (Driver)
		"engine.message", // engine event
		"engine.usage",   // paid call
		"run.checkpoint", // S02.4 checkpoint, same tx as engine.usage
		"engine.rate_limit",
		"engine.done",
		"run.state", // running→completed
	})
}

// e2eDispatcher drives a claimed run through the real adapters.Driver with the
// fake engine — the scheduler → Driver seam (Spec S03/S10). Worker/model
// selection is Spec S08's (B3); here they are supplied directly.
type e2eDispatcher struct {
	driver  *adapters.Driver
	adapter adapters.Adapter
}

func (d *e2eDispatcher) Dispatch(ctx context.Context, r run.Run) error {
	_, err := d.driver.Drive(ctx, d.adapter, adapters.StartRequest{
		RunID: r.ID, UserID: r.UserID, Model: "claude-haiku-4-5",
		Worker: adapters.CompiledWorker{ToolSchemaVersion: "ts1", PromptSchemaVersion: "ps1"},
	})
	return err
}

// fakeEngine is a minimal adapters.Adapter scripting one paid call to a clean
// completion (no real engine, no paid calls — the B1-1 fixture path).
type fakeEngine struct{}

func newFakeEngine() *fakeEngine { return &fakeEngine{} }

func (fakeEngine) Substrate() string { return adapters.SubstrateClaudeCLI }

func (fakeEngine) Start(ctx context.Context, req adapters.StartRequest) (adapters.Session, error) {
	evs := make(chan adapters.Event, 8)
	evs <- adapters.Event{Kind: adapters.KindMessage, Payload: json.RawMessage(`{"excerpt":"ok"}`)}
	evs <- adapters.Event{Kind: adapters.KindUsage, Payload: json.RawMessage(`{"message_id":"msg_1"}`),
		Usage: &adapters.Usage{ModelID: "claude-haiku-4-5", MessageID: "msg_1", MessageIndex: 1,
			InputTokens: 42, OutputTokens: 3, CacheReadTokens: 8000,
			CacheCreationByTTL: map[string]int64{"ephemeral_5m_input_tokens": 120},
			Raw:                json.RawMessage(`{"input_tokens":42,"output_tokens":3,"cache_read_input_tokens":8000}`)}}
	evs <- adapters.Event{Kind: adapters.KindRateLimit, Payload: json.RawMessage(`{"status":"allowed"}`)}
	evs <- adapters.Event{Kind: adapters.KindDone, Payload: json.RawMessage(`{"subtype":"success"}`)}
	close(evs)
	return &fakeSession{events: evs}, nil
}

func (fakeEngine) Resume(ctx context.Context, rec adapters.ParkRecord, ans *adapters.Answer) (adapters.Session, error) {
	return nil, nil
}

type fakeSession struct{ events chan adapters.Event }

func (s *fakeSession) Events() <-chan adapters.Event { return s.events }
func (s *fakeSession) Cursor() adapters.Cursor {
	return adapters.Cursor{Substrate: adapters.SubstrateClaudeCLI, SessionID: "reported-sid", MessageIndex: 1}
}
func (s *fakeSession) Fingerprint() string          { return "fp-e2e" }
func (s *fakeSession) Pause(context.Context) error  { return nil }
func (s *fakeSession) Cancel(context.Context) error { return nil }
func (s *fakeSession) Wait(context.Context) (adapters.Outcome, error) {
	return adapters.Outcome{Kind: adapters.OutcomeCompleted}, nil
}

// ── SSE trail reader ───────────────────────────────────────────────────────

type frame struct{ id, event, data string }

func readEventTrail(t *testing.T, url string, want int) []frame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/events status %d", resp.StatusCode)
	}
	br := bufio.NewReader(resp.Body)
	var out []frame
	var cur frame
	for len(out) < want {
		line, err := br.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "id: "):
			cur.id = line[len("id: "):]
		case strings.HasPrefix(line, "event: "):
			cur.event = line[len("event: "):]
		case strings.HasPrefix(line, "data: "):
			cur.data = line[len("data: "):]
		case line == "":
			if cur.event != "" {
				out = append(out, cur)
				cur = frame{}
			}
		}
	}
	return out
}

func assertTrail(t *testing.T, got []frame, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("trail has %d frames, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].event != w {
			t.Fatalf("frame[%d] = %s, want %s", i, got[i].event, w)
		}
	}
}
