package worker_test

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// tq5_drain_r1_test.go — P3-TQ-5 drain r1, F5 (Spec S08.8 step 3; the brief's
// §3 seam note). internal/worker holds no lane id of its own — it never
// imports internal/adapters, so the configured order is written as plain
// strings. This test is the one place both packages are in scope, so it is
// where the two spellings are tied together: rename a lane id and the order
// fails here loudly instead of silently pointing at a lane that no longer
// exists, which selection would read as "not covered" and skip forever.
func TestTQ5LaneOrderNamesTheRealAdapterLaneIDs(t *testing.T) {
	want := []string{adapters.LaneKimiCLI, adapters.LaneKimi, adapters.LaneAnthropic}
	got := worker.DefaultLaneOrder()[worker.DutyExecution]
	if len(got) != len(want) {
		t.Fatalf("execution lane order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("execution lane order = %v, want %v (the adapter package's own lane ids)", got, want)
		}
	}
}
