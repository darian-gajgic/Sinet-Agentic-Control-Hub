package broker

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// mode.go is the `sinet broker` daemon entry (Spec S01.5 multi-call: the same
// binary in the broker mode; S01.2 sinet-broker.service). It starts BEFORE the
// control plane and engines (S01.6): engines receive credentials at start. The
// unit binds a per-user UDS with peer-cred auth and no TCP listener (S01.2).
//
// Dev posture is the absence of systemd env (the B0-3 precedent): dev flags
// override paths per invocation; under systemd the socket/store live in
// RuntimeDirectory/StateDirectory. No host change is made here.

// Main runs one `sinet broker` invocation and returns its exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sinet broker", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stateDir := fs.String("state-dir", defaultStateDir(), "broker state root (socket + per-person stores)")
	socket := fs.String("socket", "", "per-user UDS path (default: <state-dir>/broker/<user>.sock)")
	storeDir := fs.String("store-dir", "", "per-person store root (default: <state-dir>/broker-store)")
	userName := fs.String("user", "", "person whose store this broker serves (default: current user)")
	allowStore := fs.Bool("allow-store", false, "serve the admin store op over the socket (dev; operator setup is a gate step otherwise)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "sinet broker: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	who := *userName
	if who == "" {
		who = currentUser()
	}
	sock := *socket
	if sock == "" {
		sock = filepath.Join(*stateDir, "broker", who+".sock")
	}
	sroot := *storeDir
	if sroot == "" {
		sroot = StoreRoot(*stateDir)
	}

	log := slog.New(slog.NewTextHandler(stderr, nil))
	store, err := OpenStore(sroot, who)
	if err != nil {
		fmt.Fprintf(stderr, "sinet broker: %v\n", err)
		return 1
	}
	ln, err := Listen(sock)
	if err != nil {
		fmt.Fprintf(stderr, "sinet broker: %v\n", err)
		return 1
	}
	srv := NewServer(store, uint32(os.Getuid()), log)
	srv.AllowStore = *allowStore

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	fmt.Fprintf(stdout, "sinet broker: serving user %q on %s (store %s)\n", who, sock, sroot)
	if err := srv.Serve(ctx, ln); err != nil {
		fmt.Fprintf(stderr, "sinet broker: %v\n", err)
		return 1
	}
	_ = os.Remove(sock)
	fmt.Fprintln(stdout, "sinet broker: stopped")
	return 0
}

func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "uid-" + strconv.Itoa(os.Getuid())
}

// defaultStateDir mirrors the dev-posture path convention (no systemd env):
// $STATE_DIRECTORY under a unit, else an XDG-ish dev path.
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
