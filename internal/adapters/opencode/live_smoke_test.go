package opencode

// live_smoke_test.go — tier L of the conformance split (conformance_test.go):
// ONE minimal PAID call against a REAL commissioned lane on this substrate.
//
// It is DEFINED here and NEVER RUNS at LN-1. Two gates stand in front of it and
// both print a named skip:
//
//  1. `SINET_LIVE_SMOKE=1` — THE tier-L opt-in, already ratified by
//     CONVENTIONS §10. This packet reuses it and mints no second env name.
//  2. A commissioned lane on this substrate. LN-1 is substrate-only: no
//     provider entry, no credential, no lane. Z.AI/GLM commissions at LN-2 and
//     Kimi K3 at LN-3, each with its own key ceremony (secrets never via chat
//     or commit).
//
// The body below documents the intended shape — one minimal paid call through a
// real provider entry, Driver-driven, checkpoint asserted — with no reachable
// live path, so there is nothing here that can spend money by accident.

import (
	"os"
	"testing"
)

// laneCommissioned reports whether a real provider entry exists for this
// substrate. At LN-1 it is structurally false: the packet places no credential
// anywhere and registers no lane provider, so there is nothing to commission.
func laneCommissioned() bool { return false }

func TestLiveSmoke(t *testing.T) {
	if os.Getenv("SINET_LIVE_SMOKE") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): tier-L live smoke runs only under SINET_LIVE_SMOKE=1 (one paid call)")
	}
	if !laneCommissioned() {
		t.Skip("SANCTIONED SKIP: no lane commissioned on this substrate until LN-2 (key ceremony)")
	}
	// Intended shape once a lane is commissioned (LN-2+):
	//
	//	e := newE2E(t); e.claimedRun(t, "smoke1")
	//	m := &Manager{}                      // dev-posture per-user serve
	//	a := &Adapter{Root: …, Instances: m, Providers: <the commissioned lane>}
	//	req := adapters.StartRequest{RunID: "smoke1", UserID: <member>,
	//	    Model: "<providerID>/<cheapest model>", Cwd: …, WorkDir: …,
	//	    Worker: adapters.CompiledWorker{Prompt: "Reply with exactly: ok",
	//	        ToolAllowlist: []string{"read"}}}   // never `task` (S03.5)
	//	out, err := e.drv.Drive(ctx, a, req)
	//	→ assert completed, ONE checkpoint row carrying the engine-reported
	//	  session id and the invocation fingerprint, and an engine.done event.
	t.Fatal("unreachable at LN-1: laneCommissioned() is structurally false")
}
