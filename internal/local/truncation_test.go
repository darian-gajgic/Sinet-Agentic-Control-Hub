package local_test

// truncation_test.go — PH-1 F2: a reply the engine STOPPED at the length cap is
// a named failure, not a success with odd content. Before this, the duty layer
// handed the caller a half-written JSON object and the caller reported "decode
// failed" — a sentence that describes the model as sloppy when the platform is
// the one that ran out of budget. The phrase seat lived at 0% for a cold walk
// behind exactly that mistranslation
// (P3/design/ph1-phrase-fallback-diagnosis-2026-08-17.md).

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// dutyEnvWithLogger is newDutyEnv with the duty caller's log captured, so the
// truncation WARN can be asserted as text (the operator's search surface).
func dutyEnvWithLogger(t *testing.T) (*local.Duty, *local.FakeServer, *storage.DB, *bytes.Buffer) {
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
	runs := run.NewStore(db, log)
	if _, err := runs.Create(ctx, run.NewRun{ID: "t1.intake", UserID: "alice", Lane: "anthropic", Substrate: "claude-cli"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, "t1.intake", st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition: %v", err)
		}
	}
	fake := local.NewFakeServer()
	t.Cleanup(fake.Close)
	var buf bytes.Buffer
	duty := local.NewDuty(local.DutyDeps{
		Registry: local.NewRegistry(reg), Client: local.NewClient(fake.URL),
		Checkpoints: gates.NewCheckpoints(db, log), Events: log,
		Logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	return duty, fake, db, &buf
}

// TestTruncatedReplyIsTypedError (PH-1 F2): finish_reason "length" makes the
// call fail with a cause the caller can NAME — and the D7 row still lands,
// because the tokens were really spent (R18 meters what happened).
func TestTruncatedReplyIsTypedError(t *testing.T) {
	ctx := context.Background()
	duty, fake, db, logbuf := dutyEnvWithLogger(t)

	// The workhorse answers with a JSON object cut off mid-string at the cap —
	// the exact shape the phrase seat received on every card of cold walk 1.
	fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{
		Content:      `{"reason":"the requester wants a shop, so I should first consider what "`,
		InputTokens:  598,
		OutputTokens: 4000,
		FinishReason: "length",
	})

	res, err := duty.Call(ctx, "t1.intake", local.DutyRequest{
		Alias: local.AliasUtility, User: "reword these questions", MaxTokens: 4000,
	})
	if !errors.Is(err, local.ErrTruncated) {
		t.Fatalf("a reply stopped at the cap returned err=%v, want ErrTruncated — an opaque failure is what hid PH-1 for a whole walk", err)
	}
	if res.FinishReason != "length" {
		t.Errorf("FinishReason = %q, want %q carried through to the caller", res.FinishReason, "length")
	}
	// The call happened; the tokens were spent; R18 meters it regardless.
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checkpoints WHERE run_id = ?`, "t1.intake").Scan(&rows); err != nil {
		t.Fatalf("count checkpoints: %v", err)
	}
	if rows != 1 {
		t.Errorf("D7 rows = %d after a truncated call, want 1 — a truncated call still burned the GPU (R18)", rows)
	}

	// The WARN names the platform's own facts, and NOTHING the model wrote
	// (S01.11: model text never enters the log).
	logged := logbuf.String()
	for _, want := range []string{"utility", "4000", "length", "t1.intake"} {
		if !strings.Contains(logged, want) {
			t.Errorf("truncation WARN does not mention %q: %s", want, logged)
		}
	}
	if strings.Contains(logged, "the requester wants a shop") {
		t.Errorf("model-written text leaked into the log (S01.11): %s", logged)
	}
}

// TestCapReachedWithoutFinishReasonIsAlsoTyped: the belt. If an engine build
// ever reports the stop badly, output tokens sitting exactly on the cap is the
// same fact — that single digit is what solved PH-1 in the first place.
func TestCapReachedWithoutFinishReasonIsAlsoTyped(t *testing.T) {
	ctx := context.Background()
	duty, fake, _, _ := dutyEnvWithLogger(t)
	fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{
		Content: `{"what":"partial`, InputTokens: 200, OutputTokens: 700, FinishReason: "",
	})
	if _, err := duty.Call(ctx, "t1.intake", local.DutyRequest{
		Alias: local.AliasUtility, User: "draft help", MaxTokens: 700,
	}); !errors.Is(err, local.ErrTruncated) {
		t.Fatalf("output tokens landing ON the cap returned err=%v, want ErrTruncated", err)
	}
}

// TestFinishedReplyIsUntouched: the fix must not make a normal reply loud. An
// uncapped call (MaxTokens 0) and a short capped one both stay successes.
func TestFinishedReplyIsUntouched(t *testing.T) {
	ctx := context.Background()
	duty, fake, _, logbuf := dutyEnvWithLogger(t)
	fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{
		Content: `{"pick":"abstain"}`, InputTokens: 100, OutputTokens: 12,
	})
	for _, cap := range []int64{0, 200} {
		res, err := duty.Call(ctx, "t1.intake", local.DutyRequest{
			Alias: local.AliasUtility, User: "pick one", MaxTokens: cap,
		})
		if err != nil {
			t.Fatalf("a finished reply under cap %d errored: %v", cap, err)
		}
		if res.FinishReason != "stop" {
			t.Errorf("FinishReason = %q under cap %d, want \"stop\"", res.FinishReason, cap)
		}
	}
	if strings.Contains(logbuf.String(), "level=WARN") {
		t.Errorf("a finished reply produced a WARN — the loud path must stay rare: %s", logbuf.String())
	}
}
