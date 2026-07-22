package portpool

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

// mode.go is the `sinet portpool` daemon entry (Spec S01.5 multi-call: the same
// binary in the portpool mode; S01.2 sinet-portpool.service). It binds a
// loopback/UDS listener ONLY (Spec S01.1) and self-asserts loopback for any TCP
// bind before serving (the P-T13-2 posture). Dev posture is the absence of
// systemd env (the B0-3/broker precedent): dev flags override paths per
// invocation; under systemd the socket + reservation dir live in
// RuntimeDirectory/StateDirectory. No host change is made here (the daemon is
// generated-never-installed; nothing starts it in tests).

// Main runs one `sinet portpool` invocation and returns its exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sinet portpool", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stateDir := fs.String("state-dir", defaultStateDir(), "daemon state root (reservation dir + socket)")
	socket := fs.String("socket", "", "UDS path to serve on (default: <state-dir>/portpool/portpool.sock)")
	addr := fs.String("addr", "", "loopback TCP address to serve on instead of a UDS (asserted loopback-only)")
	dir := fs.String("reservation-dir", "", "port reservation dir (default: <state-dir>/portpool/reservations)")
	lo := fs.Int("range-lo", DefaultRangeLo, "low end of the fixed preview port range (inclusive)")
	hi := fs.Int("range-hi", DefaultRangeHi, "high end of the fixed preview port range (inclusive)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "sinet portpool: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	resDir := *dir
	if resDir == "" {
		resDir = filepath.Join(*stateDir, "portpool", "reservations")
	}
	alloc, err := New(Config{Dir: resDir, Lo: *lo, Hi: *hi})
	if err != nil {
		fmt.Fprintf(stderr, "sinet portpool: %v\n", err)
		return 1
	}

	log := slog.New(slog.NewTextHandler(stderr, nil))
	ln, target, cleanup, err := listen(*socket, *addr, *stateDir)
	if err != nil {
		fmt.Fprintf(stderr, "sinet portpool: %v\n", err)
		return 1
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	rlo, rhi := alloc.Range()
	fmt.Fprintf(stdout, "sinet portpool: serving range %d-%d on %s (reservations %s)\n", rlo, rhi, target, resDir)
	srv := NewServer(alloc, log)
	if err := srv.Serve(ctx, ln); err != nil {
		fmt.Fprintf(stderr, "sinet portpool: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "sinet portpool: stopped")
	return 0
}

// listen binds the daemon's listener: a UDS by default, or a loopback TCP addr
// when -addr is given (asserted loopback-only before binding, P-T13-2). It
// returns a display target for logging and a cleanup that removes a UDS on stop.
func listen(socket, addr, stateDir string) (net.Listener, string, func(), error) {
	if addr != "" {
		if err := assertLoopback(addr); err != nil {
			return nil, "", nil, err
		}
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, "", nil, err
		}
		return ln, ln.Addr().String(), func() {}, nil
	}
	sock := socket
	if sock == "" {
		sock = filepath.Join(stateDir, "portpool", "portpool.sock")
	}
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		return nil, "", nil, err
	}
	// A stale socket from a crashed predecessor would block the bind; a UDS is
	// host-only so removing it is safe (never a live front-chain port).
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, "", nil, err
	}
	return ln, sock, func() { _ = os.Remove(sock) }, nil
}

// defaultStateDir mirrors the dev-posture path convention (no systemd env, the
// broker precedent): $STATE_DIRECTORY under a unit, else an XDG-ish dev path.
func defaultStateDir() string {
	if d := os.Getenv("STATE_DIRECTORY"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "sinet")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state", "sinet")
	}
	return filepath.Join(os.TempDir(), "sinet-state")
}
