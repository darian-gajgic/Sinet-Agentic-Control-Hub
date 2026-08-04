package backup

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// mode.go is the one-shot `sinet snapshot` and `sinet restore-drill` entries
// (Spec S13.10). They are NOT daemons — a single invocation driven by the
// generated persistent calendar timers (S01.7; internal/units), producing one
// snapshot (or one drill) and exiting. The LIVE leg is PURE CONFIGURATION: the
// remote URL, the broker git-ssh-key profile, and the escrow identity path are
// all flags/config — no code change is needed to go live (R24). Dev-mode
// testing drives the Snapshotter directly over file:// fixtures; the live first
// push is a bring-up step gated on the operator prerequisites (repo designation
// + broker key enrollment).

type modeFlags struct {
	stateDir   string
	dbPath     string
	remote     string
	local      string
	branch     string
	gitProfile string
	identity   string
	brokerSock string
	fileStores []NamedDir
}

func parseFlags(name string, args []string, stderr io.Writer) (modeFlags, bool) {
	fs := flag.NewFlagSet("sinet "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var mf modeFlags
	fs.StringVar(&mf.stateDir, "state-dir", defaultStateDir(), "platform state root (platform.db home)")
	fs.StringVar(&mf.dbPath, "db", "", "platform.db path (default: <state-dir>/platform.db)")
	fs.StringVar(&mf.remote, "snapshot-remote", "", "the operator-owned private snapshot repo URL (file:// dev, ssh:// live)")
	fs.StringVar(&mf.local, "snapshot-local", "", "local working clone of the snapshot repo (default: <state-dir>/snapshot-repo)")
	fs.StringVar(&mf.branch, "snapshot-branch", "main", "the snapshot branch")
	fs.StringVar(&mf.gitProfile, "git-profile", "", "broker git-ssh-key profile for the ssh push (empty = file:// dev, no key)")
	fs.StringVar(&mf.identity, "identity", DefaultIdentityPath, "age identity file (escrow); recipient derived at runtime")
	fs.StringVar(&mf.brokerSock, "broker-socket", "", "broker UDS (default: <state-dir>/broker/<user>.sock)")
	fs.Func("file-store", "git-versioned file store to back up as name=dir (repeatable)", func(v string) error {
		n, d, ok := strings.Cut(v, "=")
		if !ok || n == "" || d == "" {
			return fmt.Errorf("want name=dir, got %q", v)
		}
		mf.fileStores = append(mf.fileStores, NamedDir{Name: n, Dir: d})
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return modeFlags{}, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "sinet %s: unexpected argument %q\n", name, fs.Arg(0))
		return modeFlags{}, false
	}
	if mf.dbPath == "" {
		mf.dbPath = filepath.Join(mf.stateDir, storage.DBFileName)
	}
	if mf.local == "" {
		mf.local = filepath.Join(mf.stateDir, "snapshot-repo")
	}
	return mf, true
}

// compose builds a Snapshotter from resolved flags: it opens platform.db, loads
// ⚙ overrides read-only, resolves the escrow identity from the config path
// (recipient derived at runtime), and dials the broker for the CAS push.
func (mf modeFlags) compose(ctx context.Context, stderr io.Writer) (*Snapshotter, func(), error) {
	reg := settings.New()
	db, err := storage.Open(ctx, mf.dbPath, reg)
	if err != nil {
		return nil, nil, err
	}
	log := eventlog.New(db, reg)
	if err := reg.Attach(ctx, db, log); err != nil {
		db.Close()
		return nil, nil, err
	}
	crypto, err := LoadCrypto(expandHome(mf.identity))
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	sock := mf.brokerSock
	if sock == "" {
		sock = filepath.Join(mf.stateDir, "broker", currentUser()+".sock")
	}
	client, err := broker.Dial(sock)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	snap, err := New(Config{
		DB: db, Log: log, Settings: reg, Crypto: crypto, Pusher: client,
		Repo:             RepoConfig{LocalDir: mf.local, Remote: mf.remote, Branch: mf.branch},
		FileStores:       mf.fileStores,
		BrokerGitProfile: mf.gitProfile,
	})
	cleanup := func() { client.Close(); db.Close() }
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return snap, cleanup, nil
}

// SnapshotMode runs one `sinet snapshot` invocation.
func SnapshotMode(args []string, stdout, stderr io.Writer) int {
	mf, ok := parseFlags("snapshot", args, stderr)
	if !ok {
		return 2
	}
	if mf.remote == "" {
		fmt.Fprintln(stderr, "sinet snapshot: --snapshot-remote is required (the operator designates the private repo; bring-up step)")
		return 2
	}
	ctx := context.Background()
	snap, cleanup, err := mf.compose(ctx, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "sinet snapshot: %v\n", err)
		return 1
	}
	defer cleanup()
	row, err := snap.Snapshot(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "sinet snapshot: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "sinet snapshot: %s (%d bytes, sha %s) committed %s\n",
		row.SnapshotID, row.SizeBytes, row.SHA256[:12], row.RepoCommit)
	return 0
}

// DrillMode runs one `sinet restore-drill` invocation.
func DrillMode(args []string, stdout, stderr io.Writer) int {
	mf, ok := parseFlags("restore-drill", args, stderr)
	if !ok {
		return 2
	}
	if mf.remote == "" {
		fmt.Fprintln(stderr, "sinet restore-drill: --snapshot-remote is required")
		return 2
	}
	ctx := context.Background()
	snap, cleanup, err := mf.compose(ctx, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "sinet restore-drill: %v\n", err)
		return 1
	}
	defer cleanup()
	res, err := snap.RestoreDrill(ctx)
	if err != nil {
		// A fail-closed drill is a real failure (High flag raised), reported LOUD.
		fmt.Fprintf(stderr, "sinet restore-drill: FAILED CLOSED: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "sinet restore-drill: %s OK — integrity + S02.9 invariants held (%d events)\n",
		res.SnapshotID, res.Check.RunEventCount)
	return 0
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// defaultStateDir mirrors the dev-posture path convention (no systemd env):
// $STATE_DIRECTORY under a unit, else an XDG-ish dev path (the broker mode.go
// precedent).
func defaultStateDir() string {
	if d := os.Getenv("STATE_DIRECTORY"); d != "" {
		return strings.SplitN(d, ":", 2)[0]
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "sinet")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state", "sinet")
	}
	return filepath.Join(os.TempDir(), "sinet-state")
}

func currentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return fmt.Sprintf("uid-%d", os.Getuid())
}
