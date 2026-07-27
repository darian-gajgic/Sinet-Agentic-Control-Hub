package units_test

import (
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/units"
)

func gen(t *testing.T, p units.Params) map[string]units.File {
	t.Helper()
	files, err := units.Files(settings.New(), p)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	out := make(map[string]units.File, len(files))
	for _, f := range files {
		out[f.Name] = f
	}
	return out
}

func TestUnitSetIsComplete(t *testing.T) {
	files, err := units.Files(settings.New(), units.Params{})
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	// The owned S01.2 unit set plus the journald cap drop-in (Spec S01.11).
	want := []string{
		"sinet-control.service",
		"sinet-broker.service",
		"sinet-engine@.service",
		"sinet-run@.service",
		"sinet-portpool.service",
		"sinet-snapshot.service",
		"sinet-snapshot.timer",
		"sinet-restore-drill.service",
		"sinet-restore-drill.timer",
		"sinet-llamaswap.service",
		"sinet-watchlist.service",
		"sinet-local.slice",
		"journald-sinet.conf",
	}
	if len(files) != len(want) {
		t.Fatalf("%d files, want %d", len(files), len(want))
	}
	for i, name := range want {
		if files[i].Name != name {
			t.Errorf("file %d = %s, want %s (deterministic order)", i, files[i].Name, name)
		}
	}
}

func TestBackupTimersPersistentCalendar(t *testing.T) {
	files := gen(t, units.Params{})
	// Snapshot: one-shot service + a PERSISTENT calendar timer (Spec S01.7 —
	// suspend catch-up), NOT an in-process ticker. ⚙ backup.interval default is
	// daily.
	svc := files["sinet-snapshot.service"]
	for _, want := range []string{"Type=oneshot", "ExecStart=/usr/local/bin/sinet snapshot", "EnvironmentFile=-/etc/sinet/snapshot.env", "User=sinet"} {
		if !strings.Contains(svc.Content, want) {
			t.Errorf("sinet-snapshot.service lacks %q", want)
		}
	}
	if !svc.Draft {
		t.Error("snapshot service should be draft (live config is a bring-up step)")
	}
	timer := files["sinet-snapshot.timer"]
	for _, want := range []string{"OnCalendar=*-*-* 00:00:00", "Persistent=true", "Unit=sinet-snapshot.service", "WantedBy=timers.target"} {
		if !strings.Contains(timer.Content, want) {
			t.Errorf("sinet-snapshot.timer lacks %q:\n%s", want, timer.Content)
		}
	}
	// Restore drill: default ⚙ backup.drill_interval = 3 months → quarterly.
	drill := files["sinet-restore-drill.timer"]
	for _, want := range []string{"OnCalendar=*-1/3-01 00:00:00", "Persistent=true", "Unit=sinet-restore-drill.service"} {
		if !strings.Contains(drill.Content, want) {
			t.Errorf("sinet-restore-drill.timer lacks %q:\n%s", want, drill.Content)
		}
	}
	if !strings.Contains(files["sinet-restore-drill.service"].Content, "ExecStart=/usr/local/bin/sinet restore-drill") {
		t.Error("restore-drill service lacks its ExecStart")
	}
}

func TestControlUnitDirectives(t *testing.T) {
	f := gen(t, units.Params{})["sinet-control.service"]
	if f.Draft {
		t.Fatal("control unit marked draft")
	}
	for _, want := range []string{
		"Type=notify",
		"ExecStart=/usr/local/bin/sinet control",
		"WatchdogSec=30", // ⚙ shell.watchdog_sec declared default
		"Restart=on-failure",
		"StateDirectory=sinet",
		"ConfigurationDirectory=sinet",
		"After=network.target sinet-broker.service",
		"TimeoutStopSec=90",
		"User=sinet",
		"ProtectSystem=strict",
		"NoNewPrivileges=yes",
		"PrivateTmp=yes",
		"SystemCallFilter=@system-service",
		"WantedBy=multi-user.target",
		units.FQDN, // A1 hostname in the front-chain header
	} {
		if !strings.Contains(f.Content, want) {
			t.Errorf("sinet-control.service lacks %q", want)
		}
	}
}

func TestEveryUnitStaticUserNeverDynamic(t *testing.T) {
	for name, f := range gen(t, units.Params{}) {
		// The journald drop-in, .timer, and .slice units have no [Service]
		// section and therefore no User= (a timer runs its Unit=, which
		// carries the user; a .slice is a cgroup grouping, S12.2).
		if name == "journald-sinet.conf" || strings.HasSuffix(name, ".timer") || strings.HasSuffix(name, ".slice") {
			continue
		}
		if !strings.Contains(f.Content, "User=sinet") {
			t.Errorf("%s lacks User=sinet (Spec S01.1 static user)", name)
		}
		if strings.Contains(f.Content, "DynamicUser") {
			t.Errorf("%s uses DynamicUser (NEVER, Spec S01.1)", name)
		}
	}
}

func TestBrokerAndPortpoolUnits(t *testing.T) {
	files := gen(t, units.Params{})
	broker := files["sinet-broker.service"]
	if broker.Draft {
		t.Error("broker unit marked draft (ExecStart is stable from B0-1)")
	}
	for _, want := range []string{"ExecStart=/usr/local/bin/sinet broker", "Before=sinet-control.service", "Restart=on-failure", "StateDirectory=sinet"} {
		if !strings.Contains(broker.Content, want) {
			t.Errorf("sinet-broker.service lacks %q", want)
		}
	}
	pool := files["sinet-portpool.service"]
	if pool.Draft {
		t.Error("portpool unit marked draft")
	}
	if !strings.Contains(pool.Content, "ExecStart=/usr/local/bin/sinet portpool") {
		t.Error("sinet-portpool.service lacks its ExecStart")
	}
}

func TestDraftTemplates(t *testing.T) {
	files := gen(t, units.Params{})

	engine := files["sinet-engine@.service"]
	if !engine.Draft {
		t.Error("engine template not marked draft (ExecStart is Spec S03's, B1)")
	}
	if strings.Contains(engine.Content, "\nExecStart=") {
		t.Error("engine template invents an ExecStart")
	}
	if !strings.Contains(engine.Content, "After=sinet-broker.service") {
		t.Error("engine template lacks After=sinet-broker.service (Spec S01.2)")
	}

}

// TestRunTemplateExecStart: the run@ ExecStart lands at B1-3 (Spec S11.8) — the
// fixed run launcher — so the template is no longer a draft.
func TestRunTemplateExecStart(t *testing.T) {
	run := gen(t, units.Params{})["sinet-run@.service"]
	if run.Draft {
		t.Error("run template still marked draft after B1-3 (S11.8 ExecStart lands)")
	}
	for _, want := range []string{
		"ExecStart=/usr/local/bin/sinet run-launch --job /run/sinet/jobs/%i.json", // S11.8 launcher, multi-call binary
		"Restart=no",          // never auto-restarted by PID 1 (Spec S01.2)
		"RemainAfterExit=yes", // harvest lane recipe (Spec S02.5)
		"ExitType=cgroup",
		"Type=exec",
		"S11.8", // provenance in the header comment
	} {
		if !strings.Contains(run.Content, want) {
			t.Errorf("sinet-run@.service lacks %q", want)
		}
	}
}

func TestJournaldDropInCarriesCap(t *testing.T) {
	f := gen(t, units.Params{})["journald-sinet.conf"]
	// ⚙ shell.journal_max_use declared default: 4 GB in bytes.
	if !strings.Contains(f.Content, "SystemMaxUse=4294967296") {
		t.Error("journald drop-in lacks the ⚙ shell.journal_max_use cap")
	}
	if !strings.Contains(f.Content, "[Journal]") {
		t.Error("journald drop-in lacks its section header")
	}
}

func TestBinaryPathOverride(t *testing.T) {
	f := gen(t, units.Params{BinaryPath: "/opt/sinet/bin/sinet"})["sinet-control.service"]
	if !strings.Contains(f.Content, "ExecStart=/opt/sinet/bin/sinet control") {
		t.Error("BinaryPath override not rendered")
	}
}

func TestEveryFileCarriesGeneratedHeader(t *testing.T) {
	for name, f := range gen(t, units.Params{}) {
		if !strings.Contains(f.Content, "Generated by 'sinet units'") {
			t.Errorf("%s lacks the generated header", name)
		}
		if !strings.Contains(f.Content, "B0-gate operator decision") {
			t.Errorf("%s lacks the never-installed notice", name)
		}
	}
}

// TestWatchlistUnitIsGeneratedForAnOrganWithNoUnit pins the P3-B5-6A extension
// of the B4-5 carve-out: changedetection.io ships no unit file of its own
// (verified by a recursive tree read at tag 0.55.8), so Sinet generates one —
// but the unit is CONFIGURATION for the unmodified adopted binary, so its
// ExecStart must run the operator-installed organ and never the sinet binary,
// it must bind loopback (the organ's own default is 0.0.0.0), and it stays a
// draft because generation never installs.
func TestWatchlistUnitIsGeneratedForAnOrganWithNoUnit(t *testing.T) {
	f := gen(t, units.Params{})["sinet-watchlist.service"]
	if !f.Draft {
		t.Error("sinet-watchlist.service must be draft — generation never installs; the host install is a B5-gate act")
	}
	for _, want := range []string{
		"ExecStart=/usr/local/bin/changedetection.io -d /var/lib/sinet/watchlist -h 127.0.0.1 -p 5000",
		"User=sinet",
		"NoNewPrivileges=yes",
		"PrivateTmp=yes",
		"SystemCallFilter=@system-service",
		"StateDirectory=sinet/watchlist",
	} {
		if !strings.Contains(f.Content, want) {
			t.Errorf("sinet-watchlist.service lacks %q", want)
		}
	}
	if strings.Contains(f.Content, "ExecStart=/usr/local/bin/sinet") {
		t.Error("the watchlist unit must never ExecStart the sinet binary — it runs the adopted organ")
	}
	for _, line := range strings.Split(f.Content, "\n") {
		if !strings.HasPrefix(line, "#") && strings.Contains(line, "0.0.0.0") {
			t.Errorf("the watchlist unit must bind loopback (S01.1); directive %q carries the organ's 0.0.0.0 default", line)
		}
	}
}

// TestWatchlistUnitHonoursStructuralOverrides proves the composition-root
// passthrough is real config, not a hardcoded path.
func TestWatchlistUnitHonoursStructuralOverrides(t *testing.T) {
	f := gen(t, units.Params{
		WatchlistBinary:    "/opt/cdio/bin/changedetection.io",
		WatchlistDatastore: "/srv/watch",
		WatchlistListen:    "127.0.0.1:5999",
	})["sinet-watchlist.service"]
	if !strings.Contains(f.Content, "ExecStart=/opt/cdio/bin/changedetection.io -d /srv/watch -h 127.0.0.1 -p 5999") {
		t.Errorf("overrides not rendered:\n%s", f.Content)
	}
}

// TestWatchlistUnitCarriesTheLocalLLMEndpoint is the drain-r2 R4 lock. S14.6 T1
// wants the organ's native triage pointed at Sinet's local endpoint; the packet
// first recorded that as "inexpressible, an operator UI step". It is not: at
// 0.55.8 `changedetectionio/llm/evaluator.py` get_llm_config resolves
// LLM_MODEL / LLM_API_KEY / LLM_API_BASE BEFORE the datastore, so the pointing
// rides Environment= on this generated unit. Both variables must be present —
// the env branch is chosen only on a non-empty LLM_MODEL — and the api_base
// must track the llama-swap unit generated beside it, not a restated literal.
func TestWatchlistUnitCarriesTheLocalLLMEndpoint(t *testing.T) {
	f := gen(t, units.Params{})["sinet-watchlist.service"]
	for _, want := range []string{
		"Environment=LLM_API_BASE=http://127.0.0.1:8791/v1",
		"Environment=LLM_MODEL=openai/" + local.AliasWatchlist,
	} {
		if !strings.Contains(f.Content, want) {
			t.Errorf("the watchlist unit does not point the organ at the local endpoint (missing %q):\n%s", want, f.Content)
		}
	}
	// A secret never rides a generated unit (S11.5): the local endpoint needs
	// no key, so none is rendered.
	if strings.Contains(f.Content, "LLM_API_KEY") {
		t.Error("the watchlist unit renders LLM_API_KEY — a generated unit is never a place for a credential")
	}
	// The endpoint follows the llama-swap unit rather than a second literal.
	moved := gen(t, units.Params{LlamaSwapListen: "127.0.0.1:9911"})["sinet-watchlist.service"]
	if !strings.Contains(moved.Content, "Environment=LLM_API_BASE=http://127.0.0.1:9911/v1") {
		t.Errorf("moving llama-swap did not move the organ's api_base:\n%s", moved.Content)
	}
	// And the model string is deployment config, not a constant.
	seat := gen(t, units.Params{WatchlistLLMModel: "openai/qwen3-4b"})["sinet-watchlist.service"]
	if !strings.Contains(seat.Content, "Environment=LLM_MODEL=openai/qwen3-4b") {
		t.Errorf("the model override is not rendered:\n%s", seat.Content)
	}
}
