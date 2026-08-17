package api

// cancelliterals_rw18_test.go — P3-RW-18 D1: the cancel derive's cross-package
// literals, pinned to their producers BY TEST.
//
// internal/api imports no producer package in production, so the discriminators
// the decisions derive matches on are literals with cross-reference comments
// (the reads.go precedent, and internal/history's own eventlog-registry pins).
// A literal with a comment is a promise; this test is what makes it a contract.
// If a producer renames its constant, this fails here rather than the served
// task detail quietly losing every cancel.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

func TestCancelDeriveLiteralsMatchTheirProducers(t *testing.T) {
	for _, c := range []struct {
		what, got, want string
	}{
		{"the cancel transition type", runStateChangedType, run.EventState},
		{"the human-cancel discriminator", humanCancelCause, run.CancelCauseHuman},
		{"the ledger event type", ledgerUpdateType, ledger.EventLedgerUpdate},
		{"the ledger decide verb", ledgerDecideVerb, ledger.VerbDecide},
		{"the ledger human author", ledgerAuthorHuman, ledger.AuthorHuman},
	} {
		if c.got != c.want {
			t.Errorf("%s: reads.go holds %q, its producer defines %q", c.what, c.got, c.want)
		}
	}
}

// TestTheCancelDetailConstructorWritesTheDiscriminator — the other end of the
// same contract: whatever run.CancelDetail marshals is what the derive's SQL
// looks for at `$.detail.cause`. Read here through the produced JSON, so a
// change to the field NAME (not just the value) trips too.
func TestTheCancelDetailConstructorWritesTheDiscriminator(t *testing.T) {
	got := string(run.CancelDetail("alice", false, "ask-1"))
	for _, want := range []string{
		`"cause":"` + humanCancelCause + `"`,
		`"actor":"alice"`,
		`"ask_id":"ask-1"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("run.CancelDetail produced %s, which does not carry %s — the derive reads that key", got, want)
		}
	}
	// The verb path has no card, and an empty ask_id must be ABSENT rather than
	// present-and-blank: the served decision names a card only when there was
	// one.
	if noCard := string(run.CancelDetail("alice", true, "")); strings.Contains(noCard, "ask_id") {
		t.Errorf("a cardless cancel detail claims a card: %s", noCard)
	}
}
