package shell

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchdog"
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

// auditOwnListeners is the recurring listener-binding audit the watchdog suite
// consumes (drain D7; Spec S01.6/S01.8, R25). It is a REAL runtime check, not a
// re-lint of an immutable config value: it enumerates THIS process's open
// sockets via /proc/self/fd, matches their inodes against the LISTEN rows of
// /proc/net/tcp and /proc/net/tcp6, and reports every listening TCP socket bound
// beyond loopback. S01.8's concern is runtime drift — a socket that ends up
// bound to a routable address after startup — which the startup lint (which only
// checks cfg.HTTPAddr, a fixed in-process value) cannot detect.
//
// Scope: this audits ONLY sinet-control's own sockets. The external front chain
// (tailscale serve → Caddy → the unit) runs in OTHER processes, outside this
// process's fd table, and is deliberately out of scope here — its binding
// posture is the unit/network-topology audit's, not this in-process check's.
// Unix-domain listeners are inherently local (no network exposure), so a socket
// inode absent from tcp/tcp6 (e.g. a /proc/net/unix entry) is never foreign.
func auditOwnListeners() ([]watchdog.ForeignListener, error) {
	inodes, err := processSocketInodes()
	if err != nil {
		return nil, err
	}
	var foreign []watchdog.ForeignListener
	for _, src := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		rows, err := listenRows(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue // no IPv6 stack (or no /proc entry) — not an error
			}
			return nil, err
		}
		for _, r := range rows {
			if !inodes[r.inode] {
				continue // not one of our sockets
			}
			if r.ip == nil || !r.ip.IsLoopback() {
				foreign = append(foreign, watchdog.ForeignListener{Addr: r.addr, Process: "sinet-control"})
			}
		}
	}
	return foreign, nil
}

// processSocketInodes returns the socket inodes this process holds open, read
// from the /proc/self/fd symlinks (each a "socket:[<inode>]" for a socket fd).
func processSocketInodes() (map[string]bool, error) {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return nil, fmt.Errorf("shell: read /proc/self/fd: %w", err)
	}
	inodes := map[string]bool{}
	for _, e := range entries {
		link, err := os.Readlink("/proc/self/fd/" + e.Name())
		if err != nil {
			continue // an fd closed under us — skip
		}
		if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
			inodes[link[len("socket:["):len(link)-1]] = true
		}
	}
	return inodes, nil
}

// listenRow is one LISTEN row from /proc/net/tcp{,6}: its inode, decoded local
// IP, and a display address.
type listenRow struct {
	inode string
	ip    net.IP
	addr  string
}

// tcpStateListen is the /proc/net/tcp st column value for a LISTEN socket.
const tcpStateListen = "0A"

// listenRows parses the LISTEN rows of a /proc/net/tcp{,6} file. The local
// address column is "<hex-ip>:<hex-port>"; the inode is the 10th field.
func listenRows(path string) ([]listenRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []listenRow
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		if first { // header line
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 || fields[3] != tcpStateListen {
			continue
		}
		host, port, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		ip := parseProcHexIP(host)
		out = append(out, listenRow{
			inode: fields[9],
			ip:    ip,
			addr:  fmt.Sprintf("%s:%d", ipString(ip), hexPort(port)),
		})
	}
	return out, sc.Err()
}

// parseProcHexIP decodes a /proc/net address hex string (8 chars for IPv4, 32
// for IPv6). /proc prints each 32-bit word in host (little-endian on x86) byte
// order, so each 4-byte group is reversed to recover the wire address.
func parseProcHexIP(h string) net.IP {
	b, err := hex.DecodeString(h)
	if err != nil || (len(b) != 4 && len(b) != 16) {
		return nil
	}
	for i := 0; i+4 <= len(b); i += 4 {
		b[i], b[i+3] = b[i+3], b[i]
		b[i+1], b[i+2] = b[i+2], b[i+1]
	}
	return net.IP(b)
}

func ipString(ip net.IP) string {
	if ip == nil {
		return "?"
	}
	return ip.String()
}

// hexPort parses a big-endian hex port (the /proc port column).
func hexPort(h string) int {
	var p int
	for _, c := range h {
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		default:
			return 0
		}
		p = p*16 + d
	}
	return p
}
