package sdnotify

import (
	"net"
	"path/filepath"
	"testing"
	"time"
)

// listen binds a unixgram socket in a temp dir and returns received
// datagrams on a channel.
func listen(t *testing.T) (path string, got <-chan string) {
	t.Helper()
	// Socket paths have a low length cap; keep the name short.
	path = filepath.Join(t.TempDir(), "n.sock")
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Net: "unixgram", Name: path})
	if err != nil {
		t.Fatalf("listen unixgram: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	ch := make(chan string, 16)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			ch <- string(buf[:n])
		}
	}()
	return path, ch
}

func recv(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("no datagram received")
		return ""
	}
}

func TestNotificationsReachSocket(t *testing.T) {
	path, got := listen(t)
	n := New(path, 30*time.Second)
	if !n.Enabled() {
		t.Fatal("notifier with socket reports disabled")
	}
	for _, tc := range []struct {
		send func() error
		want string
	}{
		{n.Ready, "READY=1"},
		{n.Heartbeat, "WATCHDOG=1"},
		{n.Stopping, "STOPPING=1"},
		{func() error { return n.Status("draining") }, "STATUS=draining"},
	} {
		if err := tc.send(); err != nil {
			t.Fatalf("send %s: %v", tc.want, err)
		}
		if s := recv(t, got); s != tc.want {
			t.Errorf("received %q, want %q", s, tc.want)
		}
	}
}

func TestDisabledNotifierIsNoOp(t *testing.T) {
	var nilN *Notifier
	for _, n := range []*Notifier{nilN, {}, New("", time.Second)} {
		if n.Enabled() {
			t.Error("empty-socket notifier reports enabled")
		}
		if err := n.Ready(); err != nil {
			t.Errorf("disabled Ready: %v", err)
		}
		if _, ok := n.HeartbeatInterval(); ok {
			t.Error("disabled notifier reports an armed watchdog")
		}
	}
}

func TestHeartbeatIntervalIsHalfBudget(t *testing.T) {
	n := New("/tmp/x.sock", 30*time.Second)
	iv, ok := n.HeartbeatInterval()
	if !ok || iv != 15*time.Second {
		t.Fatalf("HeartbeatInterval = %v, %v; want 15s, true", iv, ok)
	}
	if _, ok := New("/tmp/x.sock", 0).HeartbeatInterval(); ok {
		t.Error("zero watchdog budget reports armed")
	}
}

func TestFromEnvDevMode(t *testing.T) {
	// The dev posture: no NOTIFY_SOCKET, no systemd required (Spec S01.6).
	t.Setenv("NOTIFY_SOCKET", "")
	t.Setenv("WATCHDOG_USEC", "")
	t.Setenv("WATCHDOG_PID", "")
	if FromEnv().Enabled() {
		t.Fatal("FromEnv without NOTIFY_SOCKET reports enabled")
	}
}

func TestFromEnvWatchdogParsing(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "/run/systemd/notify")
	t.Setenv("WATCHDOG_USEC", "30000000")
	t.Setenv("WATCHDOG_PID", "")
	n := FromEnv()
	if !n.Enabled() {
		t.Fatal("FromEnv with NOTIFY_SOCKET reports disabled")
	}
	if iv, ok := n.HeartbeatInterval(); !ok || iv != 15*time.Second {
		t.Fatalf("HeartbeatInterval = %v, %v; want 15s, true", iv, ok)
	}
}

func TestFromEnvWatchdogPIDMismatch(t *testing.T) {
	// WATCHDOG_PID naming another process: the watchdog is not ours.
	t.Setenv("NOTIFY_SOCKET", "/run/systemd/notify")
	t.Setenv("WATCHDOG_USEC", "30000000")
	t.Setenv("WATCHDOG_PID", "1")
	if _, ok := FromEnv().HeartbeatInterval(); ok {
		t.Fatal("mismatched WATCHDOG_PID must disarm the heartbeat")
	}
}

func TestAbstractSocketNaming(t *testing.T) {
	if got := socketName("@abstract"); got != "\x00abstract" {
		t.Errorf("socketName(@abstract) = %q", got)
	}
	if got := socketName("/run/notify"); got != "/run/notify" {
		t.Errorf("socketName(/run/notify) = %q", got)
	}
}
