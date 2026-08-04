package shell

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// Composition-root proofs (brief R12/R11/R31): the 7.3 revalidation-trigger
// seam is satisfied by worker.Store.FlagByModel at the root — internal/local
// never imports worker (§26); the swap gate's ⚙ local.alias write flows through
// the aliasWriter adapter over *settings.Registry.Set. These compile-time
// assertions pin that the real production types satisfy the local seams.
var (
	_ local.FlagByModelFunc = (*worker.Store)(nil).FlagByModel
	_ local.AliasWriter     = aliasWriter{}
)

// TestFlagByModelSatisfies7_3Seam is the R12 composition-root proof: the worker
// store's FlagByModel method value IS a local.FlagByModelFunc, so the swap gate
// fires the 7.3 trigger through it without internal/local importing worker.
func TestFlagByModelSatisfies7_3Seam(t *testing.T) {
	var fn local.FlagByModelFunc = (*worker.Store)(nil).FlagByModel
	if fn == nil {
		t.Fatal("worker.Store.FlagByModel must satisfy the local.FlagByModelFunc 7.3 seam (R12)")
	}
}
