package portpool

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// serveOverUDS starts a daemon on a UDS under t.TempDir() and returns a client.
// Hermetic: a host-local socket, no TCP port, nothing installed or started as a
// systemd unit (Spec S13.8 R10; the daemon is exercised in-process here).
func serveOverUDS(t *testing.T) *Client {
	t.Helper()
	alloc := newAlloc(t, 47600, 47603)
	sock := filepath.Join(t.TempDir(), "portpool.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = NewServer(alloc, nil).Serve(ctx, ln)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	return DialUDS(sock)
}

// TestDaemonServeRoundTrip: the daemon serves the same allocator over its
// loopback/UDS API — allocate/release/list all work behind the generated unit
// (Spec S13.8 R10; the daemon mode is real, the reserved table shrinks).
func TestDaemonServeRoundTrip(t *testing.T) {
	c := serveOverUDS(t)
	p, err := c.Allocate("preview-1")
	if err != nil {
		t.Fatalf("client allocate: %v", err)
	}
	if p < 47600 || p > 47603 {
		t.Fatalf("port %d out of range", p)
	}
	list, err := c.List()
	if err != nil {
		t.Fatalf("client list: %v", err)
	}
	if len(list) != 1 || list[0].Port != p || list[0].Owner != "preview-1" {
		t.Fatalf("unexpected list %+v", list)
	}
	if err := c.Release(p); err != nil {
		t.Fatalf("client release: %v", err)
	}
	list, err = c.List()
	if err != nil {
		t.Fatalf("client list after release: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list after release, got %+v", list)
	}
}

// TestDaemonExhaustionStatus: an exhausted pool returns ErrPoolExhausted over
// the wire (409), so a caller sees the honest at-capacity signal.
func TestDaemonExhaustionStatus(t *testing.T) {
	alloc := newAlloc(t, 47600, 47600)
	sock := filepath.Join(t.TempDir(), "p.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = NewServer(alloc, nil).Serve(ctx, ln) }()
	c := DialUDS(sock)
	if _, err := c.Allocate("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Allocate("b"); err != ErrPoolExhausted {
		t.Fatalf("expected ErrPoolExhausted over the wire, got %v", err)
	}
}

// TestAssertLoopback: the daemon's bind self-assert rejects any non-loopback
// TCP address and every hostname, including localhost (Spec S01.1; P-T13-2).
func TestAssertLoopback(t *testing.T) {
	ok := []string{"127.0.0.1:47600", "[::1]:47600", "127.0.0.5:0"}
	for _, a := range ok {
		if err := assertLoopback(a); err != nil {
			t.Errorf("assertLoopback(%q) = %v, want nil", a, err)
		}
	}
	bad := []string{"0.0.0.0:47600", "localhost:47600", "192.168.1.10:47600", "sinet:47600", "8481"}
	for _, a := range bad {
		if err := assertLoopback(a); err == nil {
			t.Errorf("assertLoopback(%q) = nil, want rejection", a)
		}
	}
}
