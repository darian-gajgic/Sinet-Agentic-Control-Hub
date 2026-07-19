package scheduler

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// waitState blocks until a run reaches want (or the deadline), for tests that
// synchronize with an asynchronous dispatch goroutine.
func waitState(t *testing.T, runs *run.Store, runID string, want run.State) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, err := runs.Get(context.Background(), runID)
		if err == nil && r.State == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("run %q did not reach state %s within the deadline", runID, want)
}
