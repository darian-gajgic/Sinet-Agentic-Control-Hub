package shell

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadBootstrapDefaultsWhenAbsent(t *testing.T) {
	b, err := loadBootstrap(t.TempDir()) // empty dir, no bootstrap.conf
	if err != nil {
		t.Fatalf("loadBootstrap: %v", err)
	}
	if b.HTTPAddr != DefaultHTTPAddr || b.DBPath != "" {
		t.Fatalf("defaults = %+v", b)
	}
}

func TestLoadBootstrapParsesFile(t *testing.T) {
	dir := t.TempDir()
	content := "# bootstrap-only config (Spec S01.10)\n\nhttp_addr = 127.0.0.1:9999\ndb_path=/tmp/x/platform.db\n"
	if err := os.WriteFile(filepath.Join(dir, BootstrapFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := loadBootstrap(dir)
	if err != nil {
		t.Fatalf("loadBootstrap: %v", err)
	}
	if b.HTTPAddr != "127.0.0.1:9999" || b.DBPath != "/tmp/x/platform.db" {
		t.Fatalf("parsed = %+v", b)
	}
}

func TestLoadBootstrapRejectsDrift(t *testing.T) {
	for name, content := range map[string]string{
		"unknown key":    "listen_addr = 127.0.0.1:1\n",
		"not key=value":  "http_addr 127.0.0.1:1\n",
		"stray fragment": "d1e9c0de\n",
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, BootstrapFileName), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadBootstrap(dir); err == nil {
			t.Errorf("%s: parsed silently, want fail-closed error", name)
		}
	}
}

func TestResolveConfigDirPrecedence(t *testing.T) {
	t.Setenv("CONFIGURATION_DIRECTORY", "/etc/sinet:/etc/other")
	if d := resolveConfigDir(""); d != "/etc/sinet" {
		t.Errorf("env dir = %q", d)
	}
	if d := resolveConfigDir("/custom"); d != "/custom" {
		t.Errorf("explicit dir = %q", d)
	}
	t.Setenv("CONFIGURATION_DIRECTORY", "")
	if d := resolveConfigDir(""); d != "/etc/sinet" {
		t.Errorf("default dir = %q", d)
	}
}

func TestAssertLoopbackAddr(t *testing.T) {
	for _, ok := range []string{"127.0.0.1:0", "127.0.0.1:8482", "[::1]:9000", "127.5.5.5:1"} {
		if err := assertLoopbackAddr(ok); err != nil {
			t.Errorf("%s: %v, want ok", ok, err)
		}
	}
	for _, bad := range []string{"0.0.0.0:80", "192.168.1.10:80", "[::]:80", "[2001:db8::1]:80", "localhost:80", "example.com:80", "127.0.0.1", "nonsense"} {
		err := assertLoopbackAddr(bad)
		if !errors.Is(err, ErrNonLoopback) {
			t.Errorf("%s: %v, want ErrNonLoopback (fail-closed, P-T13-2)", bad, err)
		}
	}
}

func TestAssertLoopbackListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := assertLoopbackListener(ln); err != nil {
		t.Errorf("loopback listener: %v", err)
	}
}

type countTruncater struct{ n atomic.Int64 }

func (c *countTruncater) CheckpointTruncate(context.Context) error {
	c.n.Add(1)
	return nil
}

type fixedSettings struct{ d time.Duration }

func (f fixedSettings) Duration(string) (time.Duration, error) { return f.d, nil }

func TestWALTruncateLoopFiresOnInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr := &countTruncater{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		walTruncateLoop(ctx, fixedSettings{d: 10 * time.Millisecond}, tr, discard())
	}()
	deadline := time.After(5 * time.Second)
	for tr.n.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("truncations = %d, want >= 2", tr.n.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done
}

type errSettings struct{}

func (errSettings) Duration(string) (time.Duration, error) {
	return 0, errors.New("undeclared")
}

func TestWALTruncateLoopStopsOnSettingsDefect(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		walTruncateLoop(context.Background(), errSettings{}, &countTruncater{}, discard())
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not stop on a ⚙ read defect")
	}
}

func discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
