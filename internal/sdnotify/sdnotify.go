// Package sdnotify implements the systemd service-notification protocol for
// the control plane's Type=notify unit: sd_notify(READY) at the end of the
// startup sequence and the WatchdogSec heartbeat (Spec S01.2, S01.6 step 4).
//
// The protocol is a plain datagram write to the unix socket systemd passes
// in NOTIFY_SOCKET, so it is implemented here on the stdlib — adopting a
// library for it would be a dependency without a duty (stdlib-first,
// P3/CONVENTIONS.md §2). systemd is detected via NOTIFY_SOCKET and is never
// required: in dev mode and in tests the disabled Notifier is a no-op, and
// the shell runs identically (Spec S01.6).
package sdnotify

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

// Notifier sends service-state notifications to the systemd manager. The
// zero value and a nil *Notifier are disabled no-ops, so callers never
// branch on systemd's presence.
type Notifier struct {
	socket   string
	watchdog time.Duration // WatchdogSec= budget as announced by systemd; 0 = no watchdog
}

// FromEnv builds the Notifier from the systemd-provided environment:
// NOTIFY_SOCKET selects the notification socket (absent = disabled, the dev
// posture), WATCHDOG_USEC announces the WatchdogSec= budget, and
// WATCHDOG_PID scopes that budget to one process — a mismatch means the
// watchdog is armed for a different process, never for this one.
func FromEnv() *Notifier {
	n := &Notifier{socket: os.Getenv("NOTIFY_SOCKET")}
	if n.socket == "" {
		return n
	}
	if pid := os.Getenv("WATCHDOG_PID"); pid != "" {
		if p, err := strconv.Atoi(pid); err != nil || p != os.Getpid() {
			return n
		}
	}
	if usec := os.Getenv("WATCHDOG_USEC"); usec != "" {
		if u, err := strconv.ParseInt(usec, 10, 64); err == nil && u > 0 {
			n.watchdog = time.Duration(u) * time.Microsecond
		}
	}
	return n
}

// New builds a Notifier against an explicit socket path and watchdog budget
// — the test seam. An empty socket is disabled.
func New(socket string, watchdog time.Duration) *Notifier {
	return &Notifier{socket: socket, watchdog: watchdog}
}

// Enabled reports whether a notification socket is present (i.e. the
// process runs under a Type=notify unit).
func (n *Notifier) Enabled() bool { return n != nil && n.socket != "" }

// HeartbeatInterval returns the interval at which Heartbeat must be sent and
// whether a watchdog is armed at all. The interval is half the announced
// WatchdogSec= budget — the systemd-recommended cadence, so one missed beat
// never trips the watchdog.
func (n *Notifier) HeartbeatInterval() (time.Duration, bool) {
	if !n.Enabled() || n.watchdog <= 0 {
		return 0, false
	}
	return n.watchdog / 2, true
}

// Ready sends READY=1 — Spec S01.6 step 4, after the recovery ladder.
func (n *Notifier) Ready() error { return n.send("READY=1") }

// Stopping sends STOPPING=1 at the start of the S01.6 shutdown path.
func (n *Notifier) Stopping() error { return n.send("STOPPING=1") }

// Heartbeat sends WATCHDOG=1, the WatchdogSec keep-alive.
func (n *Notifier) Heartbeat() error { return n.send("WATCHDOG=1") }

// Status sends a free-text STATUS= line (surfaced by systemctl status).
func (n *Notifier) Status(text string) error { return n.send("STATUS=" + text) }

func (n *Notifier) send(state string) error {
	if !n.Enabled() {
		return nil
	}
	addr := &net.UnixAddr{Net: "unixgram", Name: socketName(n.socket)}
	conn, err := net.DialUnix("unixgram", nil, addr)
	if err != nil {
		return fmt.Errorf("sdnotify: dial %s: %w", n.socket, err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(state)); err != nil {
		return fmt.Errorf("sdnotify: send %q: %w", state, err)
	}
	return nil
}

// socketName translates the NOTIFY_SOCKET convention: a leading '@' names an
// abstract-namespace socket, addressed with a leading NUL byte.
func socketName(s string) string {
	if len(s) > 0 && s[0] == '@' {
		return "\x00" + s[1:]
	}
	return s
}
