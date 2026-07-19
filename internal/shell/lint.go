package shell

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
)

// ErrNonLoopback is the named fail-closed error of the listener-binding
// lint: the identity story and the tailnet wall both collapse silently if
// any backend unit ever binds beyond localhost (P-T13-2, Spec S01.6 step 2,
// S01.8). The unit refuses to start rather than serve.
var ErrNonLoopback = errors.New("shell: listener beyond loopback refused (P-T13-2, Spec S01.6 step 2)")

// assertLoopbackAddr lints the configured listen address before anything
// binds. Only literal loopback IPs pass: a hostname (even "localhost") can
// drift via resolver configuration, so names are rejected fail-closed. Spec
// S01.1 sanctions "127.0.0.1 or a unix socket only"; the B0 API transport
// is TCP — the UDS option belongs to the run-unit local API whose transport
// is P3's choice at B1 (Spec S01.4).
func assertLoopbackAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%w: %q is not host:port: %v", ErrNonLoopback, addr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%w: host %q is not a literal IP (names can drift; use 127.0.0.1)", ErrNonLoopback, host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("%w: %s", ErrNonLoopback, addr)
	}
	return nil
}

// assertLoopbackListener re-asserts the invariant on the actually-bound
// listener — the configured string and the kernel's answer must agree.
func assertLoopbackListener(l net.Listener) error {
	tcp, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("%w: unexpected listener address %v", ErrNonLoopback, l.Addr())
	}
	if !tcp.IP.IsLoopback() {
		return fmt.Errorf("%w: bound %v", ErrNonLoopback, l.Addr())
	}
	return nil
}

// auditUnitListeners is the second half of the S01.6 step-2 lint: audit the
// sinet unit set for foreign non-loopback listeners and surface a violation
// as a High-severity flag. The flag machinery and the recurring form of
// this check are the S14 watchdog suite's (B5, Spec S01.8); until then the
// startup pass audits only its own listener (assertLoopback*) and this seam
// records where the unit-set audit attaches.
func auditUnitListeners(logger *slog.Logger) {
	logger.Debug("listener audit: own listener asserted; unit-set audit registers into the S14 watchdog suite (B5, Spec S01.8)")
}
