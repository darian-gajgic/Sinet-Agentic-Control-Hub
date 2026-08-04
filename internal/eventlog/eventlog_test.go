package eventlog_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

func newLog(t *testing.T) (*storage.DB, *eventlog.Log) {
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
	return db, eventlog.New(db, reg)
}

func insertRun(t *testing.T, db *storage.DB, runID string, generation int64) {
	t.Helper()
	err := db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO runs (run_id, user_id, state, generation, created_ts, updated_ts)
			 VALUES (?, 'u1', 'running', ?, '2026-07-19T00:00:00Z', '2026-07-19T00:00:00Z')`,
			runID, generation)
		return err
	})
	if err != nil {
		t.Fatalf("insert run %s: %v", runID, err)
	}
}

func platformAppend(payload string) eventlog.Append {
	return eventlog.Append{
		UserID:        "u1",
		Type:          "test.event",
		SchemaVersion: 1,
		Payload:       json.RawMessage(payload),
	}
}

func TestAppendMonotonicSeq(t *testing.T) {
	ctx := context.Background()
	_, log := newLog(t)

	for want := int64(1); want <= 3; want++ {
		seq, err := log.Append(ctx, platformAppend(`{"n":1}`))
		if err != nil {
			t.Fatalf("append %d: %v", want, err)
		}
		if seq != want {
			t.Errorf("seq = %d, want %d (event_seq is the sole ordering authority)", seq, want)
		}
	}
	head, err := log.Head(ctx)
	if err != nil || head != 3 {
		t.Errorf("Head = %d, %v; want 3", head, err)
	}
}

func TestOrderingIgnoresTimestamps(t *testing.T) {
	ctx := context.Background()
	_, log := newLog(t)

	late := platformAppend(`{}`)
	late.Time = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	early := platformAppend(`{}`)
	early.Time = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	if _, err := log.Append(ctx, late); err != nil {
		t.Fatalf("append late-clock event: %v", err)
	}
	if _, err := log.Append(ctx, early); err != nil {
		t.Fatalf("append early-clock event: %v", err)
	}
	events, err := log.After(ctx, 0, 10)
	if err != nil {
		t.Fatalf("After: %v", err)
	}
	if len(events) != 2 || events[0].Seq != 1 || events[1].Seq != 2 {
		t.Fatalf("After returned wrong order: %+v (ordering must come from event_seq, never timestamps — P-T07-4)", events)
	}
}

func TestGenerationFencing(t *testing.T) {
	ctx := context.Background()
	db, log := newLog(t)
	insertRun(t, db, "r1", 3)

	runAppend := func(gen int64) eventlog.Append {
		a := platformAppend(`{}`)
		a.RunID, a.Generation = "r1", gen
		return a
	}

	if _, err := log.Append(ctx, runAppend(3)); err != nil {
		t.Fatalf("current-generation append rejected: %v", err)
	}
	if _, err := log.Append(ctx, runAppend(2)); !errors.Is(err, eventlog.ErrStaleGeneration) {
		t.Errorf("stale append err = %v, want ErrStaleGeneration (Spec S02.5 step 4)", err)
	}
	if _, err := log.Append(ctx, runAppend(4)); !errors.Is(err, eventlog.ErrFutureGeneration) {
		t.Errorf("future append err = %v, want ErrFutureGeneration", err)
	}

	unknown := platformAppend(`{}`)
	unknown.RunID, unknown.Generation = "ghost", 0
	if _, err := log.Append(ctx, unknown); !errors.Is(err, eventlog.ErrUnknownRun) {
		t.Errorf("unknown-run append err = %v, want ErrUnknownRun", err)
	}
}

func TestValidateBeforePersist(t *testing.T) {
	ctx := context.Background()
	_, log := newLog(t)

	cases := []struct {
		name string
		mut  func(*eventlog.Append)
		want error
	}{
		{"empty type", func(a *eventlog.Append) { a.Type = "" }, eventlog.ErrInvalidEvent},
		{"empty user", func(a *eventlog.Append) { a.UserID = "" }, eventlog.ErrInvalidEvent},
		{"schema_version 0", func(a *eventlog.Append) { a.SchemaVersion = 0 }, eventlog.ErrInvalidEvent},
		{"generation on platform event", func(a *eventlog.Append) { a.Generation = 1 }, eventlog.ErrGenerationOnPlatformEvent},
		{"invalid JSON", func(a *eventlog.Append) { a.Payload = json.RawMessage(`{"x":`) }, eventlog.ErrInvalidPayload},
		{"empty payload", func(a *eventlog.Append) { a.Payload = nil }, eventlog.ErrInvalidPayload},
	}
	for _, tc := range cases {
		a := platformAppend(`{}`)
		tc.mut(&a)
		if _, err := log.Append(ctx, a); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}

	// Payload over ⚙ state.event_payload_cap (default 65536 bytes).
	big := platformAppend(`{"d":"` + strings.Repeat("a", 66000) + `"}`)
	if _, err := log.Append(ctx, big); !errors.Is(err, eventlog.ErrPayloadTooLarge) {
		t.Errorf("oversized payload err = %v, want ErrPayloadTooLarge (P-T07-5)", err)
	}

	if head, _ := log.Head(context.Background()); head != 0 {
		t.Errorf("rejected events persisted: head = %d, want 0 (validate before persist, Spec S02.1)", head)
	}
}

func TestAppendTxComposesWithCallerTransaction(t *testing.T) {
	ctx := context.Background()
	db, log := newLog(t)

	sentinel := errors.New("boom")
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := log.AppendTx(ctx, tx, platformAppend(`{}`)); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WriteTx err = %v", err)
	}
	if head, _ := log.Head(ctx); head != 0 {
		t.Errorf("append survived a rolled-back transaction: head = %d", head)
	}

	err = db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := log.AppendTx(ctx, tx, platformAppend(`{}`))
		return err
	})
	if err != nil {
		t.Fatalf("committed AppendTx: %v", err)
	}
	if head, _ := log.Head(ctx); head != 1 {
		t.Errorf("head = %d, want 1", head)
	}
}

func TestAfterCursorBatches(t *testing.T) {
	ctx := context.Background()
	db, log := newLog(t)
	insertRun(t, db, "r1", 0)

	for i := 0; i < 3; i++ {
		if _, err := log.Append(ctx, platformAppend(`{}`)); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	runScoped := platformAppend(`{"k":"v"}`)
	runScoped.RunID, runScoped.Generation = "r1", 0
	if _, err := log.Append(ctx, runScoped); err != nil {
		t.Fatalf("run-scoped append: %v", err)
	}

	batch, err := log.After(ctx, 2, 2)
	if err != nil {
		t.Fatalf("After: %v", err)
	}
	if len(batch) != 2 || batch[0].Seq != 3 || batch[1].Seq != 4 {
		t.Fatalf("After(2,2) = %+v, want seqs 3,4", batch)
	}
	if batch[0].RunID != "" || batch[0].Generation != 0 {
		t.Errorf("platform event scanned wrong: %+v", batch[0])
	}
	if batch[1].RunID != "r1" || string(batch[1].Payload) != `{"k":"v"}` || batch[1].UserID != "u1" {
		t.Errorf("run event scanned wrong: %+v", batch[1])
	}

	if tail, _ := log.After(ctx, 4, 10); len(tail) != 0 {
		t.Errorf("After(head) returned %d events, want 0", len(tail))
	}
	if _, err := log.After(ctx, 0, 0); err == nil {
		t.Error("After with limit 0 must fail (short-batch cursor reads, Spec S02.1)")
	}
}
