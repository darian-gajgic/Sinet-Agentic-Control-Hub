package sandbox

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunLaunchMode exercises the S11.8 host entry (`sinet run-launch`): a
// spool job record is composed into the sandbox and the engine runs isolated.
// Sanctioned skip when the boundary cannot be composed (S16.3).
func TestRunLaunchMode(t *testing.T) {
	if !Probe().Available() {
		t.Skipf("SANCTIONED SKIP (S16.3): host cannot compose the boundary")
	}
	dir := t.TempDir()
	ws := filepath.Join(dir, "ws")
	if err := os.MkdirAll(ws, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "marker.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	hostSecret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(hostSecret, []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	script := `if cat "$HOST_SECRET" >/dev/null 2>&1; then echo leak; else echo isolated; fi
if [ "$(grep -v Iface /proc/net/route 2>/dev/null | wc -l)" -eq 0 ]; then echo no-routes; else echo has-routes; fi`

	rec := JobRecord{
		Class:     "C1",
		Argv:      []string{"/bin/sh", "-c", script},
		Env:       []string{"PATH=/usr/bin:/bin", "HOST_SECRET=" + hostSecret, "WS=" + ws},
		Workspace: ws,
	}
	blob, _ := json.Marshal(rec)
	jobPath := filepath.Join(dir, "job.json")
	if err := os.WriteFile(jobPath, blob, 0o600); err != nil {
		t.Fatal(err)
	}

	// run-launch inherits os.Stdout (host path: the engine streams to the
	// unit's journal). Capture it via a pipe for the assertion.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := RunLaunch([]string{"--job", jobPath}, io.Discard, oldStdout)
	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)

	t.Logf("run-launch (S11.8) exit=%d output=%q", code, strings.TrimSpace(string(out)))
	if code != 0 {
		t.Errorf("run-launch exit = %d, want 0", code)
	}
	if !strings.Contains(string(out), "isolated") || !strings.Contains(string(out), "no-routes") {
		t.Errorf("run-launch did not isolate: %q", out)
	}
}

func TestRunLaunchRejectsUnknownJobField(t *testing.T) {
	dir := t.TempDir()
	jobPath := filepath.Join(dir, "job.json")
	// An unexpected field must be rejected (schema-validate, DisallowUnknownFields).
	if err := os.WriteFile(jobPath, []byte(`{"class":"C1","argv":["/bin/true"],"rogue":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := RunLaunch([]string{"--job", jobPath}, io.Discard, io.Discard); code == 0 {
		t.Error("run-launch accepted a job record with an unknown field")
	}
}

func TestRunLaunchMissingJobFlag(t *testing.T) {
	if code := RunLaunch(nil, io.Discard, io.Discard); code == 0 {
		t.Error("run-launch without --job should fail")
	}
}
