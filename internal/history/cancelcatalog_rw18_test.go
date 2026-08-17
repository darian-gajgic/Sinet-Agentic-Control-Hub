package history_test

// cancelcatalog_rw18_test.go — P3-RW-18 D4-R1: the catalog's cancel
// discriminator, pinned to the one place that writes it.
//
// `status.tasks_cancelled` selects human cancels by matching the structured
// cause the cancel-detail constructor writes. The catalog is SQL DATA, so that
// value can only appear there as a literal — and a literal that must equal a
// constant in another package is a contract with nothing enforcing it. This is
// the enforcement: rename the cause and the query stops finding the cancels it
// exists to find, loudly, here.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

func TestTheCancelledQueryMatchesTheProducersCause(t *testing.T) {
	q, ok := history.QueryByName("status.tasks_cancelled")
	if !ok {
		t.Fatal("status.tasks_cancelled is missing from the catalog")
	}
	if !strings.Contains(q.SQL, "'"+run.CancelCauseHuman+"'") {
		t.Errorf("the query does not match internal/run's cause %q — it would list no post-parity cancel:\n%s",
			run.CancelCauseHuman, q.SQL)
	}
	// The compat leg: rows minted before the structured detail existed carry
	// the cancel's "(4.5)" in their reason and nothing else. Dropping this leg
	// would make the answer start at the fix rather than at the beginning.
	if !strings.Contains(q.SQL, "'(4.5)'") {
		t.Errorf("the query lost its pre-parity compat leg — cancels older than P3-RW-18 would vanish from the answer:\n%s", q.SQL)
	}
}
