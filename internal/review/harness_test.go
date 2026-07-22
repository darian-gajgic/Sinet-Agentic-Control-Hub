package review_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S13 acceptance battery harness: a real platform.db with the ratified
// schema (migration 0007 applied by Migrate), the real event log, the real
// object dir under t.TempDir().

type fix struct {
	t     *testing.T
	db    *storage.DB
	log   *eventlog.Log
	runs  *run.Store
	reg   *settings.Registry
	store *review.Store
}

// drift overlays ⚙ review.anchor_drift_lines for one fixture.
type driftSettings struct {
	base  *settings.Registry
	drift *int64
}

func (s driftSettings) Int(key string) (int64, error) {
	if key == "review.anchor_drift_lines" && s.drift != nil {
		return *s.drift, nil
	}
	return s.base.Int(key)
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
	f := &fix{t: t, db: db, log: log, runs: run.NewStore(db, log), reg: reg}
	f.store = &review.Store{
		DB: db, Log: log, Settings: driftSettings{base: reg},
		Root: filepath.Join(t.TempDir(), "review"),
	}
	return f
}

func (f *fix) withDrift(n int64) {
	f.store.Settings = driftSettings{base: f.reg, drift: &n}
}

func (f *fix) task(taskID, userID string) {
	f.t.Helper()
	err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, ?, ?, ?)`,
			taskID, userID, "t", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		f.t.Fatalf("insert task: %v", err)
	}
}

func (f *fix) run(runID, taskID string) run.Run {
	f.t.Helper()
	r, err := f.runs.Create(context.Background(), run.NewRun{
		ID: runID, UserID: "u1", TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic",
	})
	if err != nil {
		f.t.Fatalf("create run: %v", err)
	}
	return r
}

// seed creates task t1 + run r1 and the deliverable dlv-t1.
func (f *fix) seed() review.Deliverable {
	f.t.Helper()
	f.task("t1", "u1")
	f.run("r1", "t1")
	d, err := f.store.EnsureDeliverable(context.Background(), review.EnsureInput{
		ID: "dlv-t1", Owner: "u1", TaskID: "t1", Type: "markdown",
	})
	if err != nil {
		f.t.Fatalf("EnsureDeliverable: %v", err)
	}
	return d
}

// mint mints revision n with one file body.
func (f *fix) mint(n int, content string) review.Revision {
	f.t.Helper()
	rev, err := f.store.MintRevision(context.Background(), review.MintInput{
		DeliverableID: "dlv-t1", N: n, RunID: "r1",
		AttemptRef: fmt.Sprintf("r1#round-%d", n),
		Files:      map[string]string{"deliverable.md": content},
	})
	if err != nil {
		f.t.Fatalf("MintRevision %d: %v", n, err)
	}
	return rev
}

// events returns all events of a type, in seq order.
func (f *fix) events(eventType string) []eventlog.Event {
	f.t.Helper()
	all, err := f.log.After(context.Background(), 0, 10000)
	if err != nil {
		f.t.Fatalf("read events: %v", err)
	}
	var out []eventlog.Event
	for _, e := range all {
		if e.Type == eventType {
			out = append(out, e)
		}
	}
	return out
}

// exec runs raw SQL expecting success.
func (f *fix) exec(query string, args ...any) {
	f.t.Helper()
	err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), query, args...)
		return err
	})
	if err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

// mustAbort runs raw SQL expecting a trigger/constraint refusal.
func (f *fix) mustAbort(query string, args ...any) {
	f.t.Helper()
	err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), query, args...)
		return err
	})
	if err == nil {
		f.t.Fatalf("exec %q: want a constraint/trigger refusal, got success", query)
	}
}

// minimalPDF builds a one-page PDF with a single text run — a
// deterministic in-tree fixture asserting the adopted extractor's behavior
// at the pin (S16.4 #5).
func minimalPDF(text string) []byte {
	var buf bytes.Buffer
	offsets := make([]int, 6)
	obj := func(n int, body string) {
		offsets[n] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", n, body)
	}
	buf.WriteString("%PDF-1.4\n")
	obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	obj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	obj(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)
	obj(4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	obj(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	xref := buf.Len()
	buf.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return buf.Bytes()
}
