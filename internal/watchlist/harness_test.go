package watchlist_test

// harness_test.go — the shared hermetic fixture for the S14.6 battery
// (CONVENTIONS §3): a t.TempDir() DB, stdlib testing only, ZERO network. Every
// outbound call in this package's suites goes to an httptest stub bound to
// loopback; nothing dials a live host and nothing is installed.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
)

const repoRoot = "../../"

type harness struct {
	db  *storage.DB
	log *eventlog.Log
	reg *settings.Registry
	st  *watchlist.Store
	em  *watchlist.Emitter
}

func newHarness(t *testing.T) *harness {
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
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := db.ReapplySettings(ctx); err != nil {
		t.Fatalf("ReapplySettings: %v", err)
	}
	return &harness{db: db, log: log, reg: reg, st: watchlist.NewStore(db), em: watchlist.NewEmitter(db, log)}
}

// seedOne seeds a single row, so a suite states exactly the shape it exercises.
func (h *harness) seedOne(t *testing.T, r watchlist.Row) {
	t.Helper()
	if _, err := h.st.EnsureSeeded(context.Background(), []watchlist.Row{r}); err != nil {
		t.Fatalf("EnsureSeeded: %v", err)
	}
}

func (h *harness) row(t *testing.T, id string) watchlist.Row {
	t.Helper()
	r, err := h.st.Row(context.Background(), id)
	if err != nil {
		t.Fatalf("Row(%q): %v", id, err)
	}
	return r
}

// feedRow is a minimal valid feed row for suites that only need a pollable row.
func feedRow(id, url string) watchlist.Row {
	return watchlist.Row{
		ID: id, Kind: watchlist.KindFeed, URL: url, ParserHint: "atom",
		Lane: "anthropic", Tier: 2, Cadence: watchlist.CadenceContinuous,
		Enabled: true, Group: watchlist.GroupReport02, Notes: "test row",
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

// readPackageSource concatenates every non-test source file of the package, for
// the source-scan assertions (the §12/§23 conformance-scan precedent).
func readPackageSource(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "internal/watchlist")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	files := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b.Write(body)
		files++
	}
	if files == 0 {
		t.Fatal("the source scan read no files — it would pass vacuously")
	}
	return b.String()
}

// clock is the movable test clock. Dueness derives from stored state, so a
// suite that wants a second poll advances the clock past the row's cadence
// rather than calling RunDue twice — which is exactly the production behaviour:
// a failing source is retried on its cadence, never in a hot loop.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

// exec wires an executor and the store to one clock.
func (h *harness) exec(clk *clock, d watchlist.Deps) *watchlist.Executor {
	d.Store, d.Emitter, d.Settings = h.st, h.em, h.reg
	x := watchlist.New(d)
	x.Now = clk.now
	h.st.Now = clk.now
	h.em.Now = clk.now
	return x
}

// fakeClassifier returns a fixed verdict, so a suite can exercise the emitter's
// own derivations without a local stack.
type fakeClassifier struct {
	class string
	lanes []string
}

func (f *fakeClassifier) Classify(ctx context.Context, h watchlist.Hit) watchlist.Classification {
	return watchlist.Classification{
		Lanes: f.lanes, Class: f.class, Summary: h.Title, Classified: true,
	}
}
