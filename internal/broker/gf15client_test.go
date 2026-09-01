package broker

import (
	"bufio"
	"errors"
	"io"
	"net"
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

// TestSendFailureMidConversationClassifiesUnavailable is P3-GF15 T5, the other
// half of the transport marker: a broker that answered the dial and then died
// on the wire is the same fact to the person as one that was never there — the
// helper is not running — so BOTH of send's limbs carry the sentinel.
//
// Each limb is pinned by the cause it wraps, so neither can pass on the other's
// account: the read limb's cause is io.EOF, the write limb's is net.ErrClosed.
func TestSendFailureMidConversationClassifiesUnavailable(t *testing.T) {
	// The READ limb: the daemon takes the request and goes away without
	// answering. Reading the WHOLE request line before closing is what makes
	// the limb deterministic — the write cannot lose a race to the close, and
	// a close over unread bytes would reset the connection instead of ending
	// it, which is the same class by a different errno.
	socket := filepath.Join(t.TempDir(), "dying.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = bufio.NewReader(conn).ReadBytes('\n')
		conn.Close()
	}()
	c, err := Dial(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, _, err := c.Resolve("model-endpoint"); err == nil {
		t.Fatal("a broker that closes without answering must fail the op")
	} else if !errors.Is(err, io.EOF) {
		t.Fatalf("this limb must be the READ — the request went out and nothing came back: %v", err)
	} else if !errors.Is(err, ErrUnavailable) {
		t.Errorf("a connection dying mid-conversation is a transport outage and must classify as ErrUnavailable: %v", err)
	}

	// The WRITE limb: the wire is gone before the request reaches it. A closed
	// connection is the deterministic stand-in for a socket that died under a
	// live client — send takes the same path either way.
	c2, err := Dial(socket)
	if err != nil {
		t.Fatal(err)
	}
	c2.Close()
	if _, _, err := c2.Resolve("model-endpoint"); err == nil {
		t.Fatal("an op on a dead connection must fail")
	} else if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("this limb must be the WRITE — the request never reached the wire: %v", err)
	} else if !errors.Is(err, ErrUnavailable) {
		t.Errorf("a broker connection that is gone is a transport outage and must classify as ErrUnavailable: %v", err)
	}
}
