package settings_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

var operator = settings.Actor{Kind: settings.ActorOperator, ID: "op1"}

func attached(t *testing.T) (*settings.Registry, *storage.DB, *eventlog.Log) {
	t.Helper()
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
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	return reg, db, log
}

// TestIndexMatchesS18Tallies pins the declaration index to the S18.5
// exhaustiveness sweep: 118 dotted keys, 33 domains, and the per-owner and
// per-domain counts exactly as ratified.
func TestIndexMatchesS18Tallies(t *testing.T) {
	reg := settings.New()
	decls := reg.Decls()
	if len(decls) != 118 {
		t.Fatalf("index has %d keys, want 118 (S18.5)", len(decls))
	}

	perOwner := map[string]int{}
	perDomain := map[string]int{}
	for _, d := range decls {
		perOwner[d.Section]++
		perDomain[d.Domain()]++
	}

	wantOwner := map[string]int{
		"S01": 4, "S02": 14, "S03": 3, "S04": 9, "S05": 4, "S06": 7,
		"S07": 11, "S08": 4, "S09": 12, "S10": 9, "S11": 4, "S12": 11,
		"S13": 7, "S14": 15, "S15": 1, "S16": 3,
	}
	for owner, want := range wantOwner {
		if perOwner[owner] != want {
			t.Errorf("owner %s has %d keys, want %d (S18.5)", owner, perOwner[owner], want)
		}
	}
	if len(perOwner) != len(wantOwner) {
		t.Errorf("owners = %v, want exactly 16 sections", perOwner)
	}

	wantDomain := map[string]int{
		"shell": 4, "state": 4, "recovery": 6, "effects": 1, "claims": 1,
		"freshness": 2, "adapter": 3, "orchestration": 9, "context": 4,
		"intake": 7, "verification": 11, "workers": 4, "memory": 12,
		"pressure": 2, "budget": 1, "meter": 1, "limit": 3, "scheduler": 1,
		"arbitration": 1, "sandbox": 4, "local": 11, "review": 1,
		"backup": 4, "preview": 2, "obs": 1, "watchdog": 8, "watchlist": 1,
		"canary": 2, "retention": 1, "eval": 1, "benchmark": 1,
		"frontend": 1, "adoption": 3,
	}
	if len(reg.Domains()) != 33 {
		t.Errorf("domains = %d, want 33 (S18.5)", len(reg.Domains()))
	}
	for dom, want := range wantDomain {
		if perDomain[dom] != want {
			t.Errorf("domain %s has %d keys, want %d (S18.5)", dom, perDomain[dom], want)
		}
	}
}

// TestEveryDeclaredDefaultReadable walks the whole index through the typed
// getters, proving every declared default round-trips pre-attach.
func TestEveryDeclaredDefaultReadable(t *testing.T) {
	reg := settings.New()
	for _, d := range reg.Decls() {
		var err error
		switch d.Type {
		case settings.TypeBool:
			_, err = reg.Bool(d.Key)
		case settings.TypeInt:
			_, err = reg.Int(d.Key)
		case settings.TypeFloat:
			_, err = reg.Float(d.Key)
		case settings.TypeDuration:
			_, err = reg.Duration(d.Key)
		case settings.TypeString, settings.TypeEnum:
			_, err = reg.String(d.Key)
		case settings.TypeList:
			_, err = reg.Strings(d.Key)
		case settings.TypeMap:
			_, err = reg.StringMap(d.Key)
		}
		if err != nil {
			t.Errorf("%s: default not readable: %v", d.Key, err)
		}
	}
}

func TestRatifiedDefaultsSpotChecks(t *testing.T) {
	reg := settings.New()

	if v, _ := reg.Duration("shell.drain_grace"); v != 15*time.Minute {
		t.Errorf("shell.drain_grace = %v, want 15m (G1 Def.7)", v)
	}
	if v, _ := reg.String("state.synchronous"); v != "FULL" {
		t.Errorf("state.synchronous = %q, want FULL (G2 Def.1)", v)
	}
	if v, _ := reg.Int("state.event_payload_cap"); v != 65536 {
		t.Errorf("state.event_payload_cap = %d, want 65536 (64 KiB)", v)
	}
	if v, _ := reg.Duration("effects.approval_expiry"); v != 7*24*time.Hour {
		t.Errorf("effects.approval_expiry = %v, want 168h (7 d)", v)
	}
	if v, _ := reg.String("claims.default_write"); v != "whole-project" {
		t.Errorf("claims.default_write = %q, want whole-project (G2 Def.3)", v)
	}
	if v, _ := reg.Float("pressure.cache_read_weight"); v != 0.1 {
		t.Errorf("pressure.cache_read_weight = %v, want 0.1 (G1 Def.10)", v)
	}
	if v, _ := reg.Strings("sandbox.egress_deny_cidrs"); len(v) != 6 || v[0] != "169.254.169.254/32" {
		t.Errorf("sandbox.egress_deny_cidrs = %v", v)
	}
	if v, _ := reg.StringMap("local.alias"); v["utility"] != "workhorse" || len(v) != 10 {
		t.Errorf("local.alias = %v, want the 10 S12.4 duties with utility→workhorse", v)
	}
	if v, _ := reg.StringMap("benchmark.sampling_rate"); v["pre-gate"] != "100" || v["maintenance"] != "25" {
		t.Errorf("benchmark.sampling_rate = %v (BENCH-REG §4.1)", v)
	}
	d, ok := reg.Decl("shell.inhibit_delay_max")
	if !ok || !d.Restart {
		t.Error("shell.inhibit_delay_max must be restart-required (S18.2)")
	}
}

func TestSetRoundTripWithAuditAndEvent(t *testing.T) {
	ctx := context.Background()
	reg, db, log := attached(t)

	err := reg.Set(ctx, settings.SetRequest{
		Key:    "state.wal_truncate_interval",
		Value:  json.RawMessage(`7200`),
		Actor:  operator,
		Reason: "test override",
	})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if v, _ := reg.Duration("state.wal_truncate_interval"); v != 2*time.Hour {
		t.Errorf("effective = %v, want 2h", v)
	}

	// Audit row (Spec S01.10 {actor, key, old, new, timestamp, reason}).
	var actor, oldV, newV, reason string
	err = db.QueryRowContext(ctx,
		`SELECT actor, old, new, reason FROM settings_events WHERE key = 'state.wal_truncate_interval'`).
		Scan(&actor, &oldV, &newV, &reason)
	if err != nil {
		t.Fatalf("audit row: %v", err)
	}
	if actor != "operator:op1" || oldV != "3600" || newV != "7200" || reason != "test override" {
		t.Errorf("audit row = %q %q→%q (%q)", actor, oldV, newV, reason)
	}

	// settings.changed on the main event log (G3 Def.2).
	events, err := log.After(ctx, 0, 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected exactly one event, got %d (%v)", len(events), err)
	}
	if events[0].Type != settings.EventSettingsChanged || events[0].RunID != "" {
		t.Errorf("event = %+v, want platform-scope settings.changed", events[0])
	}
	var payload struct {
		Key    string `json:"key"`
		Change string `json:"change"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil ||
		payload.Key != "state.wal_truncate_interval" || payload.Change != "value" {
		t.Errorf("event payload = %s", events[0].Payload)
	}

	// Reset-to-default deletes the row (Spec S01.10).
	if err := reg.Set(ctx, settings.SetRequest{Key: "state.wal_truncate_interval", Actor: operator, Reason: "reset"}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if v, _ := reg.Duration("state.wal_truncate_interval"); v != time.Hour {
		t.Errorf("post-reset effective = %v, want default 1h", v)
	}
	var rows int
	db.QueryRowContext(ctx, `SELECT count(*) FROM settings`).Scan(&rows)
	if rows != 0 {
		t.Errorf("settings rows after reset = %d, want 0 (row stores only the override)", rows)
	}
	if head, _ := log.Head(ctx); head != 2 {
		t.Errorf("event head = %d, want 2 (every write emits settings.changed)", head)
	}
}

func TestClampAndEnumEnforcement(t *testing.T) {
	ctx := context.Background()
	reg, _, _ := attached(t)

	err := reg.Set(ctx, settings.SetRequest{Key: "recovery.wake_grace", Value: json.RawMessage(`10000`), Actor: operator})
	if !errors.Is(err, settings.ErrOutOfBounds) {
		t.Errorf("over-ceiling err = %v, want ErrOutOfBounds", err)
	}
	err = reg.Set(ctx, settings.SetRequest{Key: "recovery.wake_grace", Value: json.RawMessage(`29`), Actor: operator})
	if !errors.Is(err, settings.ErrOutOfBounds) {
		t.Errorf("under-floor err = %v, want ErrOutOfBounds", err)
	}
	if err := reg.Set(ctx, settings.SetRequest{Key: "recovery.wake_grace", Value: json.RawMessage(`30`), Actor: operator}); err != nil {
		t.Errorf("floor value rejected: %v", err)
	}

	err = reg.Set(ctx, settings.SetRequest{Key: "state.synchronous", Value: json.RawMessage(`"EXTRA"`), Actor: operator})
	if err == nil || !strings.Contains(err.Error(), "not one of") {
		t.Errorf("bad enum err = %v", err)
	}
	if err := reg.Set(ctx, settings.SetRequest{Key: "state.synchronous", Value: json.RawMessage(`"NORMAL"`), Actor: operator}); err != nil {
		t.Errorf("valid enum rejected: %v", err)
	}

	// Exclusive floor: intake.size_recheck_factor > 1.0.
	err = reg.Set(ctx, settings.SetRequest{Key: "intake.size_recheck_factor", Value: json.RawMessage(`1.0`), Actor: operator})
	if !errors.Is(err, settings.ErrOutOfBounds) {
		t.Errorf("exclusive floor err = %v, want ErrOutOfBounds", err)
	}

	err = reg.Set(ctx, settings.SetRequest{Key: "nope.nope", Value: json.RawMessage(`1`), Actor: operator})
	if !errors.Is(err, settings.ErrUnknownKey) {
		t.Errorf("unknown key err = %v, want ErrUnknownKey", err)
	}
}

func TestAutomationDiscipline(t *testing.T) {
	ctx := context.Background()
	reg, _, _ := attached(t)
	auto := settings.Actor{Kind: settings.ActorAutomation, ID: "tuner"}

	// Auto-flagged key, within bounds: allowed (G1 rider 1).
	if err := reg.Set(ctx, settings.SetRequest{Key: "context.stage_fit_target", Value: json.RawMessage(`0.55`), Actor: auto, Reason: "complexity"}); err != nil {
		t.Errorf("automation move on auto key rejected: %v", err)
	}
	// Non-auto key: rejected.
	err := reg.Set(ctx, settings.SetRequest{Key: "state.busy_timeout", Value: json.RawMessage(`10`), Actor: auto})
	if !errors.Is(err, settings.ErrAutomation) {
		t.Errorf("automation on non-auto key err = %v, want ErrAutomation", err)
	}
	// Outside bounds: rejected.
	err = reg.Set(ctx, settings.SetRequest{Key: "context.stage_fit_target", Value: json.RawMessage(`0.9`), Actor: auto})
	if !errors.Is(err, settings.ErrOutOfBounds) {
		t.Errorf("automation outside bounds err = %v, want ErrOutOfBounds", err)
	}
	// Automation never resets.
	err = reg.Set(ctx, settings.SetRequest{Key: "context.stage_fit_target", Actor: auto})
	if !errors.Is(err, settings.ErrAutomation) {
		t.Errorf("automation reset err = %v, want ErrAutomation", err)
	}
}

func TestRelationalRules(t *testing.T) {
	ctx := context.Background()
	reg, _, _ := attached(t)

	// dead_after must stay ≥ 2× heartbeat, from both directions.
	err := reg.Set(ctx, settings.SetRequest{Key: "recovery.dead_after", Value: json.RawMessage(`100`), Actor: operator})
	if err == nil || !strings.Contains(err.Error(), "heartbeat") {
		t.Errorf("dead_after rule err = %v", err)
	}
	err = reg.Set(ctx, settings.SetRequest{Key: "recovery.heartbeat", Value: json.RawMessage(`200`), Actor: operator})
	if err == nil || !strings.Contains(err.Error(), "heartbeat") {
		t.Errorf("heartbeat-raise rule err = %v", err)
	}

	// overflow ≥ fit + 0.05.
	err = reg.Set(ctx, settings.SetRequest{Key: "context.stage_overflow_threshold", Value: json.RawMessage(`0.52`), Actor: operator})
	if err == nil || !strings.Contains(err.Error(), "fit target") {
		t.Errorf("overflow rule err = %v", err)
	}

	// The metadata-IP egress floor is non-removable.
	err = reg.Set(ctx, settings.SetRequest{Key: "sandbox.egress_deny_cidrs", Value: json.RawMessage(`["10.0.0.0/8"]`), Actor: operator})
	if err == nil || !strings.Contains(err.Error(), "non-removable") {
		t.Errorf("egress floor err = %v", err)
	}
	if err := reg.Set(ctx, settings.SetRequest{Key: "sandbox.egress_deny_cidrs", Value: json.RawMessage(`["169.254.169.254/32","10.0.0.0/8"]`), Actor: operator}); err != nil {
		t.Errorf("valid egress list rejected: %v", err)
	}

	// recitation interval: 0 (off) or 5–50.
	if err := reg.Set(ctx, settings.SetRequest{Key: "context.recitation_interval_turns", Value: json.RawMessage(`3`), Actor: operator}); err == nil {
		t.Error("recitation 3 accepted, want reject (0 or 5-50)")
	}
	if err := reg.Set(ctx, settings.SetRequest{Key: "context.recitation_interval_turns", Value: json.RawMessage(`0`), Actor: operator}); err != nil {
		t.Errorf("recitation 0 (off) rejected: %v", err)
	}

	// cpuweight: idle or 1-10000.
	if err := reg.Set(ctx, settings.SetRequest{Key: "arbitration.background_cpuweight", Value: json.RawMessage(`"turbo"`), Actor: operator}); err == nil {
		t.Error("cpuweight \"turbo\" accepted, want reject")
	}
	if err := reg.Set(ctx, settings.SetRequest{Key: "arbitration.background_cpuweight", Value: json.RawMessage(`"100"`), Actor: operator}); err != nil {
		t.Errorf("cpuweight 100 rejected: %v", err)
	}
}

func TestOperatorBounds(t *testing.T) {
	ctx := context.Background()
	reg, _, _ := attached(t)

	// Raising the depth-cap ceiling is the deliberate operator bounds
	// change S18.2 names.
	three := 3.0
	if err := reg.SetBounds(ctx, settings.SetBoundsRequest{Key: "orchestration.depth_cap", Ceiling: &three, Actor: operator, Reason: "deliberate raise"}); err != nil {
		t.Fatalf("SetBounds: %v", err)
	}
	if _, c, _ := reg.Bounds("orchestration.depth_cap"); c == nil || *c != 3 {
		t.Errorf("ceiling = %v, want 3", c)
	}
	if err := reg.Set(ctx, settings.SetRequest{Key: "orchestration.depth_cap", Value: json.RawMessage(`3`), Actor: operator}); err != nil {
		t.Errorf("value inside widened bounds rejected: %v", err)
	}

	// Narrowing bounds below a current value is rejected.
	two := 2.0
	err := reg.SetBounds(ctx, settings.SetBoundsRequest{Key: "orchestration.depth_cap", Ceiling: &two, Actor: operator})
	if err == nil || !strings.Contains(err.Error(), "outside the new bounds") {
		t.Errorf("narrowing err = %v", err)
	}

	// Only the operator edits bounds (G1 rider 1).
	auto := settings.Actor{Kind: settings.ActorAutomation, ID: "tuner"}
	err = reg.SetBounds(ctx, settings.SetBoundsRequest{Key: "orchestration.depth_cap", Ceiling: &three, Actor: auto})
	if !errors.Is(err, settings.ErrAutomation) {
		t.Errorf("automation bounds edit err = %v, want ErrAutomation", err)
	}

	// Bounds on a non-numeric key are meaningless.
	err = reg.SetBounds(ctx, settings.SetBoundsRequest{Key: "state.synchronous", Ceiling: &three, Actor: operator})
	if err == nil {
		t.Error("bounds on enum accepted, want reject")
	}
}

func TestPerUserScope(t *testing.T) {
	ctx := context.Background()
	reg, _, _ := attached(t)

	err := reg.Set(ctx, settings.SetRequest{Key: "intake.zero_interaction_cost_usd", ForUser: "u1", Value: json.RawMessage(`1.25`), Actor: operator})
	if err != nil {
		t.Fatalf("per-user set: %v", err)
	}
	if v, _ := reg.FloatFor("intake.zero_interaction_cost_usd", "u1"); v != 1.25 {
		t.Errorf("user effective = %v, want 1.25", v)
	}
	if v, _ := reg.Float("intake.zero_interaction_cost_usd"); v != 0.50 {
		t.Errorf("platform effective = %v, want unchanged 0.50", v)
	}
	if v, _ := reg.FloatFor("intake.zero_interaction_cost_usd", "u2"); v != 0.50 {
		t.Errorf("other-user effective = %v, want platform 0.50", v)
	}

	err = reg.Set(ctx, settings.SetRequest{Key: "state.busy_timeout", ForUser: "u1", Value: json.RawMessage(`10`), Actor: operator})
	if err == nil || !strings.Contains(err.Error(), "per-user") {
		t.Errorf("per-user on platform key err = %v", err)
	}
}

func TestPersistenceReload(t *testing.T) {
	ctx := context.Background()
	reg, db, _ := attached(t)

	if err := reg.Set(ctx, settings.SetRequest{Key: "backup.keep", Value: json.RawMessage(`60`), Actor: operator}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	nine := 9.0
	if err := reg.SetBounds(ctx, settings.SetBoundsRequest{Key: "watchdog.loop_repeat", Ceiling: &nine, Actor: operator}); err != nil {
		t.Fatalf("SetBounds: %v", err)
	}
	if err := reg.Set(ctx, settings.SetRequest{Key: "retention.compaction_horizon", ForUser: "u1", Value: json.RawMessage(`12`), Actor: operator}); err != nil {
		t.Fatalf("per-user Set: %v", err)
	}

	reg2 := settings.New()
	if err := reg2.Attach(ctx, db, eventlog.New(db, reg2)); err != nil {
		t.Fatalf("re-Attach: %v", err)
	}
	if v, _ := reg2.Int("backup.keep"); v != 60 {
		t.Errorf("reloaded backup.keep = %d, want 60", v)
	}
	if _, c, _ := reg2.Bounds("watchdog.loop_repeat"); c == nil || *c != 9 {
		t.Errorf("reloaded ceiling = %v, want 9", c)
	}
	if v, _ := reg2.IntFor("retention.compaction_horizon", "u1"); v != 12 {
		t.Errorf("reloaded per-user = %d, want 12", v)
	}
}

func TestWritesRequireAttach(t *testing.T) {
	reg := settings.New()
	err := reg.Set(context.Background(), settings.SetRequest{Key: "backup.keep", Value: json.RawMessage(`60`), Actor: operator})
	if !errors.Is(err, settings.ErrNotAttached) {
		t.Errorf("unattached Set err = %v, want ErrNotAttached", err)
	}
}

func TestJSONSchemaEmission(t *testing.T) {
	reg := settings.New()
	raw, err := reg.JSONSchema()
	if err != nil {
		t.Fatalf("JSONSchema: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("emitted schema is not valid JSON: %v", err)
	}

	var leaves int
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		if _, ok := node["x-sinet"]; ok {
			leaves++
		}
		if props, ok := node["properties"].(map[string]any); ok {
			for _, child := range props {
				if m, ok := child.(map[string]any); ok {
					walk(m)
				}
			}
		}
	}
	walk(doc)
	if leaves != 118 {
		t.Errorf("schema has %d x-sinet leaves, want 118", leaves)
	}

	// Spot-check one leaf end to end.
	shell := doc["properties"].(map[string]any)["shell"].(map[string]any)
	drain := shell["properties"].(map[string]any)["drain_grace"].(map[string]any)
	if drain["type"] != "integer" || drain["minimum"] != float64(60) || drain["maximum"] != float64(86400) {
		t.Errorf("shell.drain_grace leaf = %v", drain)
	}
	if drain["default"] != float64(900) {
		t.Errorf("shell.drain_grace default = %v, want 900", drain["default"])
	}
	x := drain["x-sinet"].(map[string]any)
	if x["section"] != "S01" || x["unit"] != "seconds" {
		t.Errorf("x-sinet = %v", x)
	}
	inhibit := shell["properties"].(map[string]any)["inhibit_delay_max"].(map[string]any)
	if x := inhibit["x-sinet"].(map[string]any); x["restartRequired"] != true {
		t.Error("inhibit_delay_max missing restartRequired in schema")
	}
	sync := doc["properties"].(map[string]any)["state"].(map[string]any)["properties"].(map[string]any)["synchronous"].(map[string]any)
	if enum, ok := sync["enum"].([]any); !ok || len(enum) != 2 {
		t.Errorf("state.synchronous enum = %v", sync["enum"])
	}
}
