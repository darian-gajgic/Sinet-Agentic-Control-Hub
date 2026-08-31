package broker

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestDialFailureClassifiesUnavailable is P3-GF15 T4: the client's own
// transport/refusal distinction, made classifiable for callers.
//
// (a) A dial against a socket nothing listens on is an OUTAGE — errors.Is finds
// ErrUnavailable — and the socket path survives in the error string, because
// the ops log is where that fact belongs (the requester surface serves the
// sentinel's plain answer instead).
//
// (b) A served refusal is NOT an outage. The broker answering !OK means the
// daemon is up and its guardrail held, which is a different fact and must not
// be reported to a person as "the helper is not running".
func TestDialFailureClassifiesUnavailable(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "gone", "op.sock")
	c, err := Dial(socket)
	if err == nil {
		c.Close()
		t.Fatal("dialing a socket nothing listens on must fail")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("a dead socket is a transport outage and must classify as ErrUnavailable: %v", err)
	}
	if !strings.Contains(err.Error(), socket) {
		t.Errorf("the ops detail must survive in the error string — socket path missing: %v", err)
	}

	live, _ := startServer(t, true)
	lc, err := Dial(live)
	if err != nil {
		t.Fatal(err)
	}
	defer lc.Close()
	if _, _, err := lc.Resolve("no-such-profile"); err == nil {
		t.Fatal("resolving an unknown profile must be refused")
	} else if errors.Is(err, ErrUnavailable) {
		t.Errorf("a served refusal is the guardrail holding, not an outage: %v", err)
	}
}
