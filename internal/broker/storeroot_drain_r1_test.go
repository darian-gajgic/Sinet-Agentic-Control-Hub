package broker

// storeroot_drain_r1_test.go — P3-LN-4 drain r1 / D1 (S11.5).
//
// The daemon WRITES a credential and the control plane READS it. While each
// spelled the store root for itself, the two could disagree and nothing in the
// tree would notice: changing the daemon's literal left every test green and
// made every placed key invisible to commissioning. This binds them.
//
// $0: the daemon is run in-process, never listens, and the only file it creates
// is a master key inside t.TempDir().

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// D1 · the `sinet broker` daemon's own default store root IS StoreRoot.
//
// Main is driven far enough to open the store — which is what creates it — and
// then made to fail at Listen, so the test needs no daemon, no socket and no
// goroutine. Where the store landed is the assertion.
func TestLN4DaemonStoreRootIsTheSharedDerivation(t *testing.T) {
	stateDir := t.TempDir()

	// A socket path that is a non-empty DIRECTORY: Listen's stale-socket
	// removal fails on it, so Main returns instead of serving. OpenStore has
	// already run by then, which is the point.
	occupied := filepath.Join(t.TempDir(), "occupied")
	if err := os.MkdirAll(filepath.Join(occupied, "child"), 0o700); err != nil {
		t.Fatalf("occupied socket path: %v", err)
	}

	code := Main([]string{
		"--state-dir", stateDir,
		"--user", "me",
		"--socket", occupied,
	}, io.Discard, io.Discard)
	if code == 0 {
		t.Fatal("the daemon reported success — it was supposed to fail at Listen, so it may have served " +
			"and this test would have hung")
	}

	// The store the daemon created must be where the control plane reads.
	want := filepath.Join(StoreRoot(stateDir), "me", "master.key")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("the daemon did not create its store at StoreRoot(stateDir) (%v).\n"+
			"The daemon WRITES a credential and the control plane READS one; if they derive the store root "+
			"separately they can disagree, and the failure is silent in the worst direction — every placed "+
			"key invisible to commissioning, with no error anywhere. `sinet broker` must call StoreRoot.", err)
	}

	// …and nowhere else under the state dir, so a second spelling cannot hide
	// behind the first one happening to be right.
	var strays []string
	err := filepath.Walk(stateDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Name() != "master.key" || path == want {
			return nil
		}
		strays = append(strays, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk the state dir: %v", err)
	}
	if len(strays) != 0 {
		t.Errorf("the daemon created a store outside StoreRoot: %v", strays)
	}
}
