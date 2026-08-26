package kimicli

// live_smoke_test.go — tier L: ONE minimal PAID call on a REAL commissioned
// kimi-cli lane.
//
// Three gates stand in front of it and each prints a named skip:
//
//  1. `SINET_LIVE_SMOKE=1` — THE tier-L opt-in ratified by CONVENTIONS §10.
//     This suite reuses it and mints NO second env name.
//  2. The pinned engine installed on this host.
//  3. A credential actually PLACED in this person's broker store under the
//     profile the kimi-cli lane document names.
//
// A fourth condition is specific to this lane and outranks the other three:
// under the recorded Gate-A gray zone (the Community Guidelines' interactive-
// only clause, and the operator's 2026-08-26 PROCEED ruling), the FIRST live
// call on this lane is the operator's own door run — not a test's. So this
// suite stays structurally unreachable at landing by design, not by accident.

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
)

func TestLiveSmoke(t *testing.T) {
	// Gate 1 — the ratified opt-in.
	if os.Getenv("SINET_LIVE_SMOKE") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): tier-L live smoke runs only under SINET_LIVE_SMOKE=1 (one paid call)")
	}
	// Gate 2 — the engine.
	if _, err := exec.LookPath(DefaultBinary); err != nil {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): the pinned `%s` CLI is not installed on this host", DefaultBinary)
	}
	// Gate 3 — a credential actually placed, read the way production reads it:
	// the plaintext `kind` of a broker record, never a decryption.
	lanes, err := opencode.SeedLaneConfigs()
	if err != nil {
		t.Fatalf("SeedLaneConfigs: %v", err)
	}
	var lane opencode.LaneConfig
	for _, l := range lanes {
		if l.Lane == adapters.LaneKimiCLI {
			lane = l
		}
	}
	if lane.Lane == "" {
		t.Fatal("no kimi-cli lane document ships — tier L has no lane to spend on")
	}
	who := "sinet"
	if u, err := user.Current(); err == nil && u.Username != "" {
		who = u.Username
	}
	stateDir := os.Getenv("SINET_STATE_DIR")
	if stateDir == "" {
		stateDir = filepath.Join(os.Getenv("HOME"), ".local", "state", "sinet")
	}
	profiles, err := broker.PlacedEngineCreds(broker.StoreRoot(stateDir), who)
	if err != nil || !profiles[lane.Credential.Profile] {
		t.Skipf("SANCTIONED SKIP: lane %q is not commissioned on this host — no engine-cred is placed under profile "+
			"%q for %q, so there is nothing to spend and nothing to spend it with",
			lane.Lane, lane.Credential.Profile, who)
	}

	// Gate 4 — the Gate-A posture. Reaching here means the first three opened,
	// which on this lane is exactly the situation that needs a person.
	t.Skipf("SANCTIONED SKIP: lane %q carries a RECORDED GRAY-ZONE Gate-A posture (the Kimi Code Community "+
		"Guidelines' interactive-only clause; operator PROCEED ruling 2026-08-26, recorded in "+
		"P3/measurements/2026-08-24-kimi-lane-gate-audit.md). The first live call on this membership through "+
		"this path is the operator's own door run, made deliberately — not a test's, and not one a battery "+
		"can make on their behalf.", lane.Lane)
}
