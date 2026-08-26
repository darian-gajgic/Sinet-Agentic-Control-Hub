package shell

// kimicli_ln7_test.go — P3-LN-7 §10 specs T5, T7, T8, T9 (S03.6, S10.5, S11.5,
// §64, §65).
//
// The composition root is where BOTH kimi lanes become visible at once, and
// where the comparison the operator asked for either stays honest or quietly
// stops being one. Hermetic and $0: fake broker records with EMPTY ciphertext
// in a t.TempDir() store, never the operator's.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// ── T5 · the concurrent-request-limit row freezes rather than retries ────────

// TestConcurrentLimitRowFreezesRatherThanRetries drives the SHIPPED documents
// through the real path: ExtractSignal → the wire payload → Classify.
//
// The string is a trap and this is the assertion that holds it. Moonshot
// publishes `403 "You've reached your concurrent request limit"` as an ordinary
// concurrency shed AND as the enforcement action for a terms concern, verbatim:
// "we'll … take appropriate action—such as limiting concurrent access—based on
// the severity. You'll then see a You've reached your concurrent request limit
// error." A `transient` class here would make the platform retry silently
// THROUGH an enforcement action against the operator's own account.
func TestConcurrentLimitRowFreezesRatherThanRetries(t *testing.T) {
	cfg := scheduler.LimitConfig{RetryCap: 3, RetryBudgetRatio: 0.1, ProbeIntervalMax: 30 * time.Minute}
	lanes := seedLanes(t)

	classify := func(t *testing.T, lane, msg string, status int) scheduler.Action {
		t.Helper()
		c := laneByName(t, lanes, lane)
		sig, ok := c.ExtractSignal(`{"error":{"message":`+jsonString(msg)+`}}`, status)
		if !ok {
			t.Fatalf("lane %s produced no signal for %q on HTTP %d", lane, msg, status)
		}
		raw, err := json.Marshal(sig)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		decoded, err := scheduler.SignalFromPayload(raw)
		if err != nil {
			t.Fatalf("SignalFromPayload: %v", err)
		}
		decoded.EndpointVerified = true
		decoded.OnValidCredentials = true
		return scheduler.Classify(decoded, cfg)
	}

	for _, lane := range []string{adapters.LaneKimi, adapters.LaneKimiCLI} {
		got := classify(t, lane, "You've reached your concurrent request limit", 403)
		if got.Class != scheduler.ClassAuthPolicy || got.Kind != scheduler.ActionLaneFreeze {
			t.Errorf("lane %s: the concurrent-limit 403 classified %+v, want the Class-4 freeze — it is also the "+
				"vendor's stated enforcement signal, and retrying through an enforcement action is how a "+
				"gray-zone posture becomes a terminated account", lane, got)
		}
		if got.Kind == scheduler.ActionRetryInPlace {
			t.Errorf("lane %s: the platform would RETRY an enforcement action", lane)
		}

		// The control, so "freeze on everything" cannot pass as a fix: an
		// ordinary depletion on the same status still parks.
		dep := classify(t, lane, "You've reached your 5-hour usage limit", 403)
		if dep.Kind != scheduler.ActionParkQuota && dep.Kind != scheduler.ActionParkProbe {
			t.Errorf("lane %s: the 5-hour depletion 403 classified %+v, want a park — a lane that freezes when its "+
				"window empties pages the operator every week", lane, dep)
		}
		if dep.Class == scheduler.ClassAuthPolicy {
			t.Errorf("lane %s: an ordinary weekly depletion froze the lane", lane)
		}
	}
}

// ── T7 · opencode is not handed a provider entry for an engine it cannot drive ─

// TestOpencodeEntriesUnchangedByCLILane pins both directions. Without the
// substrate filter opencode compiles a provider block for a lane it does not
// drive: harmless to execution, and a LIE in a config body that gets hashed,
// logged and inspected — plus a spurious restart of that person's serve.
func TestOpencodeEntriesUnchangedByCLILane(t *testing.T) {
	stateDir := t.TempDir()
	lanes := seedLanes(t)
	for _, l := range lanes {
		ln4Place(t, stateDir, "me", l.Credential.Profile, broker.KindEngineCred)
	}
	commissioned := commissionEngineLanes(stateDir, lanes, testLogger())

	entries := commissioned["me"]
	if len(entries) == 0 {
		t.Fatal("nothing commissioned — both directions below would pass vacuously")
	}
	cliDoc := laneByName(t, lanes, adapters.LaneKimiCLI)
	// Direction 1: the platform-wide map DOES carry the CLI lane. It is
	// commissioned; it simply does not belong to opencode.
	if _, ok := entries[cliDoc.ProviderID]; !ok {
		t.Errorf("the commissioned map does not carry provider id %q — the CLI lane would never be routable", cliDoc.ProviderID)
	}

	// Direction 2: what reaches opencode carries only opencode's own lanes.
	forOpencode := opencodeProviderEntries(lanes, entries)
	if _, leaked := forOpencode[cliDoc.ProviderID]; leaked {
		t.Errorf("opencode was handed a provider entry for %q, a lane on substrate %q — it would compile a provider "+
			"block for an engine it does not drive", cliDoc.ProviderID, cliDoc.Substrate)
	}
	for _, l := range lanes {
		if l.Substrate != adapters.SubstrateOpencode {
			continue
		}
		if _, ok := forOpencode[l.ProviderID]; !ok {
			t.Errorf("opencode LOST its own lane %q — the filter over-reached", l.Lane)
		}
	}
	// Byte-identical to the pre-packet result: exactly opencode's own lanes.
	want := 0
	for _, l := range lanes {
		if l.Substrate == adapters.SubstrateOpencode {
			want++
		}
	}
	if len(forOpencode) != want {
		t.Errorf("opencode holds %d entries, want %d (its own substrate's lanes and nothing else)", len(forOpencode), want)
	}
}

// ── T8 · one placed key commissions BOTH kimi lanes ──────────────────────────

func TestOnePlacedKeyCommissionsBothKimiLanes(t *testing.T) {
	stateDir := t.TempDir()
	lanes := seedLanes(t)
	kimi := laneByName(t, lanes, adapters.LaneKimi)
	cli := laneByName(t, lanes, adapters.LaneKimiCLI)
	if kimi.Credential.Profile != cli.Credential.Profile {
		t.Fatalf("the two kimi lanes name different broker profiles (%q vs %q) — one membership is one key",
			kimi.Credential.Profile, cli.Credential.Profile)
	}

	// ONE placement, under the shared profile.
	ln4Place(t, stateDir, "me", kimi.Credential.Profile, broker.KindEngineCred)
	commissioned := commissionEngineLanes(stateDir, lanes, testLogger())

	got := commissionedLanes(lanes, commissioned)
	for _, want := range []string{adapters.LaneKimi, adapters.LaneKimiCLI} {
		if !containsString(got, want) {
			t.Errorf("one placed %q credential commissioned %v — it must commission BOTH kimi lanes, because both "+
				"documents name that profile", kimi.Credential.Profile, got)
		}
	}
	if containsString(got, adapters.LaneZAI) {
		t.Errorf("commissioning leaked to the zai lane on a kimi-only placement: %v", got)
	}

	// Each lane maps to its OWN substrate. Sharing a credential is not sharing
	// an engine — this is the map that keeps the operator's comparison honest.
	subs := laneSubstrates(lanes, commissioned)
	if subs[adapters.LaneKimi] != adapters.SubstrateOpencode {
		t.Errorf("laneSubstrates[kimi] = %q, want %q", subs[adapters.LaneKimi], adapters.SubstrateOpencode)
	}
	if subs[adapters.LaneKimiCLI] != adapters.SubstrateKimiCLI {
		t.Errorf("laneSubstrates[kimi-cli] = %q, want %q — two lanes on one pool must still reach two engines",
			subs[adapters.LaneKimiCLI], adapters.SubstrateKimiCLI)
	}

	// The control: nothing placed is byte-identical to the pre-packet world.
	emptyLanes, emptyCommissioned := seedLanes(t), commissionEngineLanes(t.TempDir(), seedLanes(t), testLogger())
	if len(emptyCommissioned) != 0 {
		t.Errorf("an empty state dir commissioned %v", emptyCommissioned)
	}
	if subs := laneSubstrates(emptyLanes, emptyCommissioned); subs != nil {
		t.Errorf("laneSubstrates = %v with nothing placed, want nil", subs)
	}
	if got := commissionedLanes(emptyLanes, emptyCommissioned); len(got) != 0 {
		t.Errorf("commissionedLanes = %v with nothing placed, want none", got)
	}
}

// ── T9 · one broker resolution per PROFILE per spawn ─────────────────────────

// TestCredResolvedOncePerProfilePerSpawn is R15a. Two lanes share profile
// `kimi-code` and name DIFFERENT variables, so a naive composition dials the
// broker twice for one secret. Resolve the profile once and fan the material
// out to each distinct variable.
func TestCredResolvedOncePerProfilePerSpawn(t *testing.T) {
	lanes := seedLanes(t)
	kimi := laneByName(t, lanes, adapters.LaneKimi)
	cli := laneByName(t, lanes, adapters.LaneKimiCLI)
	if kimi.Credential.EnvVar == cli.Credential.EnvVar {
		t.Fatalf("both kimi lanes name variable %q — the CLI does not read the API lane's variable and this test "+
			"would not be testing the fan-out", kimi.Credential.EnvVar)
	}

	rec := &recordingResolver{secret: "sk-LN7-SENTINEL"}
	inject := laneCredInjectorWith(lanes, map[string]bool{
		kimi.Credential.Profile: true,
	}, rec.mk)
	if inject == nil {
		t.Fatal("no injector was composed for a person holding the kimi-code profile")
	}
	env, err := inject([]string{"PATH=/usr/bin"})
	if err != nil {
		t.Fatalf("inject: %v", err)
	}

	if n := rec.calls[kimi.Credential.Profile]; n != 1 {
		t.Errorf("the broker was dialled %d times for profile %q in ONE spawn, want exactly 1 — two lanes sharing a "+
			"membership share a secret, and resolving it twice doubles the exposure for nothing",
			n, kimi.Credential.Profile)
	}
	if len(rec.calls) != 1 {
		t.Errorf("profiles resolved = %v, want only %q", rec.calls, kimi.Credential.Profile)
	}
	for _, name := range []string{kimi.Credential.EnvVar, cli.Credential.EnvVar} {
		if n := countEnv(env, name); n != 1 {
			t.Errorf("%s appears %d times in the lowered environment, want exactly 1", name, n)
		}
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// recordingResolver counts BROKER RESOLUTIONS, not compositions: the closure
// increments when it runs, which is once per spawn, which is the property under
// test.
type recordingResolver struct {
	secret string
	calls  map[string]int
}

func (r *recordingResolver) mk(profile string, vars []string) func([]string) ([]string, error) {
	return func(base []string) ([]string, error) {
		if r.calls == nil {
			r.calls = map[string]int{}
		}
		r.calls[profile]++
		out := append([]string{}, base...)
		for _, v := range vars {
			out = append(out, v+"="+r.secret)
		}
		return out, nil
	}
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func countEnv(env []string, name string) int {
	n := 0
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok && k == name {
			n++
		}
	}
	return n
}

var _ = opencode.LaneConfig{}

// ── drain r1 F3 · the pooled budget reaches the ROUTER's own call site ───────

// TestPooledBudgetReachesTheRouterCallSite proves the fix where §63 D5 says to
// prove it: at the production call site, not at the resolver.
//
// bindingPlanReading is what the router's pressure read and the meters surface
// both consume, and it looked the budget up under the REQUESTING lane. With the
// budget declared once on `kimi` — which is what OQ-2 requires — `kimi-cli`
// came back undeclared with pressure 0 while spending the same allowance, so
// the router saw free headroom on a pool that was already committed.
func TestPooledBudgetReachesTheRouterCallSite(t *testing.T) {
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	runs := run.NewStore(db, log)
	cps := gates.NewCheckpoints(db, log)
	gauge := metering.NewPressureGauge(db, reg)
	pb := metering.NewPlanBudgets(db)

	const who = "pooled"
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO users (user_id, role, created_ts) VALUES (?, 'operator', ?)`,
			who, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("seed person: %v", err)
	}
	doc, ok := metering.PlanDocFor(adapters.LaneKimi)
	if !ok {
		t.Fatal("no plan document for the kimi lane")
	}
	now := time.Now().UTC().Add(time.Hour)
	if _, _, err := pb.Declare(ctx, metering.PlanBudgetRow{
		UserID: who, Lane: doc.Lane, Window: metering.PlanQuotaWindow,
		PeriodUnits: 100, Unit: doc.QuotaUnit(metering.PlanQuotaWindow),
		PeriodStart: now.Add(-time.Hour), PeriodHours: 5,
		Source: metering.PlanBudgetOperatorSet, DeclaredTS: now, DeclaredBy: who,
	}); err != nil {
		t.Fatalf("declare on the canonical lane: %v", err)
	}
	// Spent entirely through the SIBLING lane.
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("r-pooled-%d", i)
		if _, err := runs.Create(ctx, run.NewRun{
			ID: id, UserID: who, Lane: adapters.LaneKimiCLI, Substrate: adapters.SubstrateKimiCLI,
		}); err != nil {
			t.Fatalf("create run: %v", err)
		}
		for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
			if _, err := runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
				t.Fatalf("transition: %v", err)
			}
		}
		if _, err := cps.Write(ctx, gates.NewCheckpoint{
			RunID: id, ModelID: "k3", Usage: json.RawMessage(`{"input_tokens":10,"output_tokens":5}`),
			SessionSubstrate: adapters.SubstrateKimiCLI, SessionID: "s-" + id,
		}); err != nil {
			t.Fatalf("checkpoint: %v", err)
		}
	}

	for _, lane := range []string{adapters.LaneKimi, adapters.LaneKimiCLI} {
		r, window, err := bindingPlanReading(ctx, gauge, pb, who, lane, doc, now)
		if err != nil {
			t.Fatalf("bindingPlanReading(%s): %v", lane, err)
		}
		if !r.Budget.Declared {
			t.Errorf("lane %s: the router's own call site reads NO declared budget — one allowance is declared once, "+
				"so a sibling that cannot see it reports free headroom on a committed pool", lane)
			continue
		}
		if !r.Applicable {
			t.Errorf("lane %s: the pooled budget did not apply (%s)", lane, r.InapplicableNote)
			continue
		}
		if r.Pressure <= 0 {
			t.Errorf("lane %s: pressure = %v at the router's call site, want the pool's spend", lane, r.Pressure)
		}
		if window == "" {
			t.Errorf("lane %s: the reading names no window", lane)
		}
	}

	// Both lanes bind the SAME number: one pool, one spend, one denominator.
	a, _, _ := bindingPlanReading(ctx, gauge, pb, who, adapters.LaneKimi, doc, now)
	b, _, _ := bindingPlanReading(ctx, gauge, pb, who, adapters.LaneKimiCLI, doc, now)
	if a.Pressure != b.Pressure {
		t.Errorf("kimi binds pressure %v and kimi-cli binds %v — one pool, one number", a.Pressure, b.Pressure)
	}
}
