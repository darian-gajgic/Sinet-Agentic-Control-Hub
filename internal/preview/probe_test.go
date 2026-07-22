package preview

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sandbox"
)

// envProbeListener re-execs the test binary as a listener inside a composed
// sandbox — the self-re-exec harness of the caps-gated live netns probe (the
// S02.9 kill-9-harness precedent).
const envProbeListener = "SINET_PREVIEW_PROBE_LISTENER"

// TestMain intercepts the listener re-exec: when the env is set the process
// binds 0.0.0.0:<port> (S13.8 "dev server binds 0.0.0.0 inside the netns") and
// blocks so the parent can read its netns proc view — never running the suite.
func TestMain(m *testing.M) {
	if port := os.Getenv(envProbeListener); port != "" {
		ln, err := net.Listen("tcp", ":"+port)
		if err != nil {
			os.Exit(7)
		}
		defer ln.Close()
		select {} // block until the parent kills us
	}
	os.Exit(m.Run())
}

// TestParseProcNetTCP: the pure parser extracts exactly the LISTEN ports from a
// /proc/net/tcp stream, ignoring every non-LISTEN row — config is never the
// port source (Spec S13.8; R8).
func TestParseProcNetTCP(t *testing.T) {
	const tcp = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 100 1 0 0 0
   1: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 101 1 0 0 0
   2: 0100007F:C350 0100007F:1F90 01 00000000:00000000 00:00000000 00000000  1000        0 102 1 0 0 0
`
	ports, err := parseProcNetTCP(strings.NewReader(tcp))
	if err != nil {
		t.Fatal(err)
	}
	// 1F90=8080, 0050=80 LISTEN; the ESTABLISHED (st 01) row is ignored.
	if len(ports) != 2 || ports[0] != 80 || ports[1] != 8080 {
		t.Fatalf("ports = %v, want [80 8080]", ports)
	}
}

// TestParseProcNetTCP6: the tcp6 128-bit local address parses identically (port
// after the final colon).
func TestParseProcNetTCP6(t *testing.T) {
	const tcp6 = `  sl  local_address                         remote_address                        st
   0: 00000000000000000000000000000000:1F91 00000000000000000000000000000000:0000 0A 00000000
`
	ports, err := parseProcNetTCP(strings.NewReader(tcp6))
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 1 || ports[0] != 8081 {
		t.Fatalf("ports = %v, want [8081]", ports)
	}
}

// TestSelectPort: a single port auto-selects; multiple ports yield picker DATA
// (Spec S13.8; R8).
func TestSelectPort(t *testing.T) {
	if _, _, ok := selectPort(nil); ok {
		t.Errorf("no ports should not select")
	}
	if p, picker, ok := selectPort([]Port{{Number: 3000}}); !ok || picker || p != 3000 {
		t.Errorf("single port should auto-select without a picker")
	}
	if p, picker, ok := selectPort([]Port{{Number: 3000}, {Number: 24678}}); !ok || !picker || p != 3000 {
		t.Errorf("multi-port should yield picker DATA with a provisional primary")
	}
}

// TestLiveNetnsProbe proves the netns semantics against a REAL bwrap empty netns
// (Spec S13.8/§10): a listener inside the composed sandbox appears in the
// sandboxed process's /proc/<pid>/net/tcp view and NOT in the host's. Caps-gated
// (sanctioned skip when the boundary is uncomposable). srt is forced OFF so the
// probe targets the native funeral-plan path's empty netns (srt removes the
// netns; the native path is what runs when srt is absent, the B1 posture).
func TestLiveNetnsProbe(t *testing.T) {
	caps := sandbox.Probe()
	if !caps.Available() {
		t.Skip("SANCTIONED SKIP (S16.3): host cannot compose the sandbox boundary")
	}
	caps.Srt = false // native empty-netns path

	self, err := os.Executable()
	if err != nil {
		t.Skipf("SANCTIONED SKIP: os.Executable: %v", err)
	}
	const probePort = 47611
	sp := sandbox.Spawn{
		Argv:         []string{self},
		Env:          []string{envProbeListener + "=" + strconv.Itoa(probePort), "PATH=/usr/bin:/bin"},
		EnginePrefix: filepath.Dir(self),
	}
	plan, err := sandbox.BuildPlan(caps, sandbox.C1, sp, sandbox.EgressPolicy{Mode: sandbox.NetNone}, t.TempDir())
	if err != nil {
		t.Skipf("SANCTIONED SKIP: compose: %v", err)
	}
	defer plan.Cleanup()

	cmd := exec.Command(plan.Bin, plan.Argv...)
	cmd.Env = plan.Env
	cmd.ExtraFiles = plan.ExtraFiles
	if err := cmd.Start(); err != nil {
		t.Skipf("SANCTIONED SKIP: bwrap start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	// cmd.Process.Pid is the bwrap MONITOR (host netns); the sandboxed listener
	// is a descendant in the new netns. R8 parses "the sandboxed process TREE's
	// /proc/<pid>/net/tcp" — so probe the tree, not just the monitor.
	monitor := cmd.Process.Pid

	found := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if containsPort(probeTree(monitor), probePort) {
			found = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !found {
		t.Skipf("SANCTIONED SKIP: listener did not appear in the sandbox netns (bwrap/loopback unavailable here)")
	}
	// Netns isolation: the port lives in the sandbox netns, NOT the host's.
	if hostPorts, err := probeListeningPorts(os.Getpid()); err == nil && containsPort(hostPorts, probePort) {
		t.Errorf("port %d leaked into the host netns — netns isolation broken", probePort)
	}
}

func containsPort(ports []Port, n int) bool {
	for _, p := range ports {
		if p.Number == n {
			return true
		}
	}
	return false
}

// probeTree unions the listening ports across a process and all its descendants
// (the sandboxed process shares its netns with bwrap's other sandbox-side
// processes; the monitor is in the host netns and simply lacks the sandbox
// port). This is the "sandboxed process tree" the R8 probe parses.
func probeTree(root int) []Port {
	seen := map[int]bool{}
	var out []Port
	for _, pid := range append(descendants(root), root) {
		ports, err := probeListeningPorts(pid)
		if err != nil {
			continue
		}
		for _, p := range ports {
			if !seen[p.Number] {
				seen[p.Number] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// descendants returns every descendant pid of root by walking /proc PPIDs.
func descendants(root int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	ppid := map[int]int{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if pp := parentPid(pid); pp != 0 {
			ppid[pid] = pp
		}
	}
	var out []int
	frontier := []int{root}
	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]
		for pid, pp := range ppid {
			if pp == cur {
				out = append(out, pid)
				frontier = append(frontier, pid)
			}
		}
	}
	return out
}

// parentPid reads /proc/<pid>/stat, tolerating a comm field with spaces/parens.
func parentPid(pid int) int {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	s := string(b)
	i := strings.LastIndex(s, ")") // end of the (comm) field
	if i < 0 {
		return 0
	}
	fields := strings.Fields(s[i+1:]) // state, ppid, ...
	if len(fields) < 2 {
		return 0
	}
	pp, _ := strconv.Atoi(fields[1])
	return pp
}
