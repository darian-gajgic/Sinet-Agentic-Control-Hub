package shell_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sdnotify"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/shell"
)

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

type fixedSettings struct{ d time.Duration }

func (f fixedSettings) Duration(string) (time.Duration, error) { return f.d, nil }

// recAdmission records admission-seam calls.
type recAdmission struct {
	mu    sync.Mutex
	calls []string
}

func (r *recAdmission) record(s string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, s)
	return nil
}

func (r *recAdmission) ResumeAdmission(context.Context) error  { return r.record("resume") }
func (r *recAdmission) StopAdmission(context.Context) error    { return r.record("stop") }
func (r *recAdmission) ParkInFlightRuns(context.Context) error { return r.record("park") }

func (r *recAdmission) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for %s", what)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestMaintenanceDrainParkExit(t *testing.T) {
	rec := &recAdmission{}
	m := shell.NewMaintenance(fixedSettings{d: 30 * time.Millisecond}, rec, discard())
	ctx := context.Background()

	if m.Mode() != shell.ModeRunning {
		t.Fatalf("mode = %s", m.Mode())
	}
	if err := m.Enter(ctx); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if m.Mode() != shell.ModeDraining {
		t.Fatalf("mode after enter = %s, want draining", m.Mode())
	}
	if err := m.Enter(ctx); err == nil {
		t.Fatal("second Enter succeeded, want error")
	}
	// Grace expiry parks in-flight runs (Spec S01.6).
	waitFor(t, "drain grace expiry", func() bool {
		calls := rec.snapshot()
		return m.Mode() == shell.ModeMaintenance && len(calls) > 0 && calls[len(calls)-1] == "park"
	})
	if got := rec.snapshot(); !slicesEqual(got, []string{"stop", "park"}) {
		t.Fatalf("admission calls = %v, want [stop park]", got)
	}
	if err := m.Exit(ctx); err != nil {
		t.Fatalf("exit: %v", err)
	}
	if m.Mode() != shell.ModeRunning {
		t.Fatalf("mode after exit = %s", m.Mode())
	}
	if got := rec.snapshot(); !slicesEqual(got, []string{"stop", "park", "resume"}) {
		t.Fatalf("admission calls = %v, want [stop park resume]", got)
	}
	if err := m.Exit(ctx); err == nil {
		t.Fatal("Exit while running succeeded, want error")
	}
}

func TestMaintenanceExitBeforeGraceCancelsPark(t *testing.T) {
	rec := &recAdmission{}
	m := shell.NewMaintenance(fixedSettings{d: time.Hour}, rec, discard())
	ctx := context.Background()
	if err := m.Enter(ctx); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if err := m.Exit(ctx); err != nil {
		t.Fatalf("exit: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := rec.snapshot(); !slicesEqual(got, []string{"stop", "resume"}) {
		t.Fatalf("admission calls = %v, want [stop resume] (no park)", got)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// listenNotify is a fake systemd notify socket.
func listenNotify(t *testing.T) (string, <-chan string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "n.sock")
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Net: "unixgram", Name: path})
	if err != nil {
		t.Fatalf("listen unixgram: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	ch := make(chan string, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			select {
			case ch <- string(buf[:n]):
			default:
			}
		}
	}()
	return path, ch
}

func waitGram(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case g := <-ch:
			if g == want {
				return
			}
		case <-deadline:
			t.Fatalf("notify socket never received %q", want)
		}
	}
}

// readFrame reads one SSE frame; comment-only frames are skipped.
func readFrame(r *bufio.Reader) (event string, data string, err error) {
	for {
		var line string
		line, err = r.ReadString('\n')
		if err != nil {
			return "", "", err
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			event = line[len("event: "):]
		case strings.HasPrefix(line, "data: "):
			data = line[len("data: "):]
		case line == "" && (event != "" || data != ""):
			return event, data, nil
		}
	}
}

func TestRunLifecycle(t *testing.T) {
	stateDir := t.TempDir()
	sock, grams := listenNotify(t)
	notifier := sdnotify.New(sock, 100*time.Millisecond) // heartbeat every 50ms
	readyCh := make(chan net.Addr, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() {
		runErr <- shell.Run(ctx, shell.Options{
			ConfigDir: t.TempDir(),
			StateDir:  stateDir,
			HTTPAddr:  "127.0.0.1:0",
			Logger:    discard(),
			Notifier:  notifier,
			ReadyFunc: func(a net.Addr) { readyCh <- a },
		})
	}()

	var addr net.Addr
	select {
	case addr = <-readyCh:
	case err := <-runErr:
		t.Fatalf("run exited before ready: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for readiness")
	}
	base := "http://" + addr.String()

	// The startup sequence completed: platform.db exists in the state dir,
	// READY=1 was notified (Spec S01.6 step 4), the watchdog beats.
	if _, err := os.Stat(filepath.Join(stateDir, "platform.db")); err != nil {
		t.Fatalf("platform.db: %v", err)
	}
	waitGram(t, grams, "READY=1")
	waitGram(t, grams, "WATCHDOG=1")

	// Health answers ready with the lifecycle event on the log.
	resp, err := http.Get(base + "/api/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	var h api.Health
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		t.Fatalf("health decode: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !h.Ready || h.Mode != "running" || h.EventHead < 1 {
		t.Fatalf("health = %d %+v", resp.StatusCode, h)
	}

	// The SSE stream replays the platform.started lifecycle event from an
	// explicit cursor.
	sctx, scancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer scancel()
	req, err := http.NewRequestWithContext(sctx, http.MethodGet, base+"/events?after_seq=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	sresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	defer sresp.Body.Close()
	sr := bufio.NewReader(sresp.Body)
	event, data, err := readFrame(sr)
	if err != nil {
		t.Fatalf("first frame: %v", err)
	}
	if event != shell.EventPlatformStarted {
		t.Fatalf("first event = %s %s, want %s", event, data, shell.EventPlatformStarted)
	}

	// SIGTERM-equivalent: cancel. The open stream receives the
	// platform.stopping event from the shutdown flush, then ends; Run
	// returns clean; STOPPING=1 was notified.
	cancel()
	event, _, err = readFrame(sr)
	if err != nil {
		t.Fatalf("stopping frame: %v", err)
	}
	if event != shell.EventPlatformStopping {
		t.Fatalf("final event = %s, want %s", event, shell.EventPlatformStopping)
	}
	if _, _, err := readFrame(sr); err == nil {
		t.Fatal("stream still open after shutdown")
	}
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("run did not return after cancel")
	}
	waitGram(t, grams, "STOPPING=1")
}

func TestRunRefusesNonLoopback(t *testing.T) {
	err := shell.Run(context.Background(), shell.Options{
		ConfigDir: t.TempDir(),
		StateDir:  t.TempDir(),
		HTTPAddr:  "0.0.0.0:0",
		Logger:    discard(),
		Notifier:  sdnotify.New("", 0),
	})
	if !errors.Is(err, shell.ErrNonLoopback) {
		t.Fatalf("err = %v, want ErrNonLoopback (fail-closed, P-T13-2)", err)
	}
}

func TestRunReadsBootstrapConfig(t *testing.T) {
	configDir := t.TempDir()
	stateDir := t.TempDir()
	dbPath := filepath.Join(stateDir, "custom.db")
	conf := "http_addr = 127.0.0.1:0\ndb_path = " + dbPath + "\n"
	if err := os.WriteFile(filepath.Join(configDir, shell.BootstrapFileName), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	readyCh := make(chan net.Addr, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() {
		runErr <- shell.Run(ctx, shell.Options{
			ConfigDir: configDir,
			Logger:    discard(),
			Notifier:  sdnotify.New("", 0),
			ReadyFunc: func(a net.Addr) { readyCh <- a },
		})
	}()
	select {
	case a := <-readyCh:
		tcp, ok := a.(*net.TCPAddr)
		if !ok || !tcp.IP.IsLoopback() {
			t.Fatalf("bound %v, want loopback", a)
		}
	case err := <-runErr:
		t.Fatalf("run exited before ready: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for readiness")
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("bootstrap db_path not honored: %v", err)
	}
	cancel()
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}
