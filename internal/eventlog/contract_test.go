package eventlog_test

// contract_test.go is the external cross-check of the S14.2 event-type
// contract (internal/eventlog/contract.go, B5-1). Being an external test
// package it MAY import every producer (the registry itself must not — that
// would cycle), which is exactly where the declare-once enforcement lives:
// TestDeclareOnce_EveryProducerTypeIsRegistered iterates the producer
// constants and fails if any minted type is absent from the registry. This
// replaces the 20+ scattered "provisional pending S14" comments with one
// enforced assertion (CONVENTIONS §29).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/backup"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/shell"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// producerTypes is the complete inventory of the 76 event types minted by the
// B0–B4 producers, referenced through each package's exported Event* constant
// so a producer-side value change (or a deleted constant) is caught here at
// compile or assertion time. The 13 worker-asset lifecycle constants are
// package-private (internal/worker/store.go:43-55, evTemplateCreated…), so
// they appear here as literals — the one place Go cannot couple by identifier.
func producerTypes() []string {
	types := []string{
		// shell (2): platform lifecycle
		shell.EventPlatformStarted, shell.EventPlatformStopping,
		// run (5)
		run.EventCreated, run.EventState, run.EventForked, run.EventWedged, run.EventHarvest,
		// gates (1)
		gates.EventCheckpoint,
		// adapters (6): the closed S03.1 engine event set, canonical names
		adapters.EventTypeMessage, adapters.EventTypeUsage, adapters.EventTypeRateLimit,
		adapters.EventTypeGateAsk, adapters.EventTypeToolResult, adapters.EventTypeDone,
		// auth (9)
		auth.EventLogin, auth.EventLoginFailed, auth.EventLogout, auth.EventPINSet,
		auth.EventGrant, auth.EventGrantRevoked, auth.EventReprompt, auth.EventRepromptFailed,
		auth.EventUserCreated,
		// settings (1)
		settings.EventSettingsChanged,
		// ledger (2)
		ledger.EventContextManifest, ledger.EventLedgerUpdate,
		// intake (3)
		intake.EventState, intake.EventDeltaDecision, intake.EventFollowUp,
		// stage (4)
		stage.EventSpawn, stage.EventSpawnRefused, stage.EventHelper, stage.EventContextOverflow,
		// verify (4)
		verify.EventV0, verify.EventV1, verify.EventRound, verify.EventEscalation,
		// worker (2 exported: routing + compiled)
		worker.EventRoutingDecided, worker.EventWorkerCompiled,
		// memory (6)
		memory.EventWrite, memory.EventRemove, memory.EventDelete,
		memory.EventConflict, memory.EventCuration, memory.EventSearchMiss,
		// review (4)
		review.EventMinted, review.EventComment, review.EventDrained, review.EventAccepted,
		// project (4)
		project.EventRegistered, project.EventCaptured, project.EventActivated, project.EventDrift,
		// preview (2)
		preview.EventStarted, preview.EventStopped,
		// metering (1)
		metering.EventUtilization,
		// backup (3)
		backup.EventSnapshot, backup.EventDrill, backup.EventDrillFlag,
		// local (4)
		local.EventLocalUnmeteredDefect, local.EventLocalAdmissionsStopped,
		local.EventLocalAdmissionsResumed, local.EventLocalAliasRetargeted,
	}
	// worker-asset lifecycle (13) — package-private constants, by value.
	types = append(types,
		"worker.template_created", "worker.version_created", "worker.validated",
		"worker.approved", "worker.promoted", "worker.repointed", "worker.flagged",
		"worker.retired", "worker.review", "worker.graduated", "worker.gap",
		"worker.domain_maturity", "worker.automation_run",
	)
	return types
}

// TestDeclareOnce_EveryProducerTypeIsRegistered is the enforcement mechanism:
// every type a producer mints MUST be registered as a canonical minted type
// (R9). A newly-minted type absent from the registry fails here. Proven
// non-tautological during B5-1: appending an unregistered scratch string to
// producerTypes trips this assertion.
func TestDeclareOnce_EveryProducerTypeIsRegistered(t *testing.T) {
	reg := eventlog.Registry()
	for _, typ := range producerTypes() {
		fam, known := reg.Classify(typ)
		if !known {
			t.Errorf("producer type %q is NOT registered in the S14.2 contract — a new minted type MUST register (CONVENTIONS §29)", typ)
			continue
		}
		ts, ok := reg.TypeSpec(typ)
		if !ok {
			t.Errorf("producer type %q resolves only as a legacy alias (family %s), not a canonical minted type", typ, fam)
			continue
		}
		if ts.Status != eventlog.StatusMinted {
			t.Errorf("producer type %q is registered %s, want minted (a producer emits it today)", typ, ts.Status)
		}
	}
}

// TestEveryMintedTypeHasAProducer is the reverse: the registry's minted set
// has no orphan — every minted type maps to a producer in the inventory. A
// type with no producer must be StatusDeclareOnly, not minted.
func TestEveryMintedTypeHasAProducer(t *testing.T) {
	producers := make(map[string]bool)
	for _, typ := range producerTypes() {
		producers[typ] = true
	}
	for _, ts := range eventlog.Registry().Types() {
		if ts.Status != eventlog.StatusMinted {
			continue
		}
		if !producers[ts.Type] {
			t.Errorf("registered minted type %q has no producer — declare-only types must use StatusDeclareOnly", ts.Type)
		}
	}
}

// TestInventoryTotals pins the reconciliation counts (§2): 76 minted + 17
// declare-only = 93 registered types.
func TestInventoryTotals(t *testing.T) {
	if n := len(producerTypes()); n != 76 {
		t.Errorf("producer inventory = %d, want 76 (§2)", n)
	}
	var minted, declareOnly int
	for _, ts := range eventlog.Registry().Types() {
		switch ts.Status {
		case eventlog.StatusMinted:
			minted++
		case eventlog.StatusDeclareOnly:
			declareOnly++
		default:
			t.Errorf("type %q has unknown status %q", ts.Type, ts.Status)
		}
	}
	if minted != 76 {
		t.Errorf("registered minted types = %d, want 76", minted)
	}
	if declareOnly != 17 {
		t.Errorf("declare-only types = %d, want 17", declareOnly)
	}
}

// TestEveryTypeInExactlyOneFamily (R9): each registered type maps to exactly
// one declared family, and Classify agrees with its TypeSpec.
func TestEveryTypeInExactlyOneFamily(t *testing.T) {
	reg := eventlog.Registry()
	declared := make(map[eventlog.Family]bool)
	for _, f := range reg.Families() {
		declared[f.Name] = true
	}
	seen := make(map[string]bool)
	for _, ts := range reg.Types() {
		if seen[ts.Type] {
			t.Errorf("type %q registered more than once", ts.Type)
		}
		seen[ts.Type] = true
		if !declared[ts.Family] {
			t.Errorf("type %q references undeclared family %q", ts.Type, ts.Family)
		}
		fam, known := reg.Classify(ts.Type)
		if !known || fam != ts.Family {
			t.Errorf("Classify(%q) = %q,%v; want %q,true", ts.Type, fam, known, ts.Family)
		}
	}
}

// TestFifteenFamiliesWithRequiredFields (item 1): all 15 S14.2 families are
// declared, each with a verbatim contract-minimum required-field descriptor
// and an anchor.
func TestFifteenFamiliesWithRequiredFields(t *testing.T) {
	fams := eventlog.Registry().Families()
	if len(fams) != 15 {
		t.Fatalf("families = %d, want 15 (S14.2)", len(fams))
	}
	want := map[eventlog.Family]bool{
		eventlog.FamilyRunLifecycle: true, eventlog.FamilyCheckpoint: true,
		eventlog.FamilyUsageLimits: true, eventlog.FamilyGateAsk: true,
		eventlog.FamilyHumanDecision: true, eventlog.FamilyVerificationVerdict: true,
		eventlog.FamilyRouting: true, eventlog.FamilyKnowledgeInjection: true,
		eventlog.FamilyOrchestration: true, eventlog.FamilyWatchdog: true,
		eventlog.FamilyDriftCanary: true, eventlog.FamilyPlatform: true,
		eventlog.FamilyToolsArtifacts: true, eventlog.FamilyBenchmarkEval: true,
		eventlog.FamilyRunSummary: true,
	}
	for _, f := range fams {
		if !want[f.Name] {
			t.Errorf("unexpected family %q", f.Name)
		}
		if strings.TrimSpace(f.RequiredFields) == "" {
			t.Errorf("family %q has no contract-minimum required-field descriptor (S14.2)", f.Name)
		}
		if strings.TrimSpace(f.Anchor) == "" {
			t.Errorf("family %q has no S14.2 anchor", f.Name)
		}
	}
	// Spot-check a few descriptors verbatim against S14.2.
	assertFieldContains(t, eventlog.FamilyVerificationVerdict, "round number")
	assertFieldContains(t, eventlog.FamilyRouting, "plain_reason")
	assertFieldContains(t, eventlog.FamilyKnowledgeInjection, "precedence_label")
	assertFieldContains(t, eventlog.FamilyPlatform, "{actor, key, old, new, reason}")
}

func assertFieldContains(t *testing.T, fam eventlog.Family, substr string) {
	t.Helper()
	spec, ok := eventlog.Registry().FamilySpec(fam)
	if !ok {
		t.Errorf("family %q not declared", fam)
		return
	}
	if !strings.Contains(spec.RequiredFields, substr) {
		t.Errorf("family %q required fields missing %q (must be verbatim from S14.2): %q", fam, substr, spec.RequiredFields)
	}
}

// TestEverySchemaVersionIsOne (item 2): all v0 types are schema_version 1.
func TestEverySchemaVersionIsOne(t *testing.T) {
	for _, ts := range eventlog.Registry().Types() {
		if ts.SchemaVersion != 1 {
			t.Errorf("type %q schema_version = %d, want 1 (all v0 types are v1)", ts.Type, ts.SchemaVersion)
		}
	}
}

// TestUpcastHook (R6, item 3): a v1 payload of a real type round-trips
// unchanged; a fixture type with a registered v1→v2 upcast fires on a
// stored-v1 row. No real type's payload is upcast (none exists at v0).
func TestUpcastHook(t *testing.T) {
	reg := eventlog.Registry()

	in := json.RawMessage(`{"from":"running","to":"completed","reason":"done"}`)
	out, err := reg.Upcast("run.state_changed", 1, in)
	if err != nil {
		t.Fatalf("v1 round-trip Upcast: %v", err)
	}
	if string(out) != string(in) {
		t.Errorf("v1 payload of a v1 type changed on upcast: got %s, want %s", out, in)
	}

	// A fixture contract with one type at schema_version 2 and a registered
	// v1→v2 upcast. Built via the exported NewContract so it exercises the
	// exact production Upcast path without polluting the v0 registry.
	fixtureFam := eventlog.FamilySpec{Name: "fixture_family", RequiredFields: "n", Anchor: "test"}
	fixtureType := eventlog.TypeSpec{
		Type: "fixture.upcast", Family: "fixture_family", SchemaVersion: 2,
		Status: eventlog.StatusMinted, Verdict: eventlog.VerdictConforms,
		Upcasts: map[int]eventlog.UpcastFunc{
			1: func(p json.RawMessage) (json.RawMessage, error) {
				var m map[string]any
				if err := json.Unmarshal(p, &m); err != nil {
					return nil, err
				}
				m["upcast_applied"] = true // the v1→v2 shape change
				return json.Marshal(m)
			},
		},
	}
	fc, err := eventlog.NewContract([]eventlog.FamilySpec{fixtureFam}, []eventlog.TypeSpec{fixtureType}, nil)
	if err != nil {
		t.Fatalf("NewContract(fixture): %v", err)
	}
	got, err := fc.Upcast("fixture.upcast", 1, json.RawMessage(`{"n":1}`))
	if err != nil {
		t.Fatalf("fixture Upcast: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("fixture upcast output invalid JSON: %v", err)
	}
	if applied, _ := m["upcast_applied"].(bool); !applied {
		t.Errorf("registered v1→v2 upcast did not fire: %s", got)
	}
	if _, ok := m["n"]; !ok {
		t.Errorf("upcast dropped the v1 payload content: %s", got)
	}
}

// TestForwardTolerantClassify (R7, item 4): an unregistered type returns
// known=false with no error/panic; Upcast passes it through unchanged.
func TestForwardTolerantClassify(t *testing.T) {
	fam, known := eventlog.Classify("totally.unknown")
	if known || fam != "" {
		t.Errorf("Classify(unknown) = %q,%v; want \"\",false (log-and-skip, S14.2 rule 3)", fam, known)
	}
	in := json.RawMessage(`{"x":1}`)
	out, err := eventlog.Upcast("totally.unknown", 1, in)
	if err != nil {
		t.Errorf("Upcast(unknown) errored: %v — forward-tolerant reads are never fatal", err)
	}
	if string(out) != string(in) {
		t.Errorf("Upcast(unknown) changed the payload: %s", out)
	}
}

// TestReaderYieldsUnknownTypeRow (R7): a reader over run_events containing an
// unknown-type row returns it (eventlog.After is already forward-tolerant);
// the consumer classifies-and-skips, and the read never fails.
func TestReaderYieldsUnknownTypeRow(t *testing.T) {
	ctx := context.Background()
	_, log := newLog(t)
	if _, err := log.Append(ctx, eventlog.Append{
		UserID: "u1", Type: "future.unknown", SchemaVersion: 1, Payload: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("append unknown-type row: %v", err)
	}
	rows, err := log.After(ctx, 0, 10)
	if err != nil {
		t.Fatalf("After over an unknown-type row failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Type != "future.unknown" {
		t.Fatalf("After did not yield the unknown-type row: %+v", rows)
	}
	if _, known := eventlog.Classify(rows[0].Type); known {
		t.Errorf("unknown type unexpectedly classified as known")
	}
}

// TestLegacyAliasesClassify (R14, item 7): each renamed type's old name still
// classifies to its family (so any pre-existing dev/test row stays readable),
// and none of the old names is a canonical type.
func TestLegacyAliasesClassify(t *testing.T) {
	reg := eventlog.Registry()
	cases := map[string]eventlog.Family{
		"run.state":                   eventlog.FamilyRunLifecycle,
		"run.checkpoint":              eventlog.FamilyCheckpoint,
		"engine.usage":                eventlog.FamilyUsageLimits,
		"engine.rate_limit":           eventlog.FamilyUsageLimits,
		"verify.round":                eventlog.FamilyVerificationVerdict,
		"context.manifest":            eventlog.FamilyKnowledgeInjection,
		"orchestration.spawn":         eventlog.FamilyOrchestration,
		"orchestration.spawn_refused": eventlog.FamilyOrchestration,
		"context.overflow":            eventlog.FamilyPlatform,
		"engine.tool_result":          eventlog.FamilyToolsArtifacts,
		"deliverable.minted":          eventlog.FamilyToolsArtifacts,
	}
	if len(cases) != 11 {
		t.Fatalf("expected 11 legacy aliases (the 11 renames), have %d", len(cases))
	}
	for old, wantFam := range cases {
		fam, known := reg.Classify(old)
		if !known || fam != wantFam {
			t.Errorf("Classify(legacy %q) = %q,%v; want %q,true", old, fam, known, wantFam)
		}
		if _, isCanonical := reg.TypeSpec(old); isCanonical {
			t.Errorf("legacy name %q must be an alias, never a canonical minted type", old)
		}
	}
}

// TestRenamedTypesAreCanonical (item 7): each rename's NEW name is registered
// as a canonical minted type carrying the RENAME verdict.
func TestRenamedTypesAreCanonical(t *testing.T) {
	reg := eventlog.Registry()
	renamed := []string{
		"run.state_changed", "checkpoint.written", "usage.recorded", "limit.event",
		"verdict.recorded", "knowledge.injected", "helper.spawned", "spawn.refused",
		"compaction.anomaly", "tool.completed", "artifact.produced",
	}
	for _, typ := range renamed {
		ts, ok := reg.TypeSpec(typ)
		if !ok {
			t.Errorf("renamed canonical type %q not registered", typ)
			continue
		}
		if ts.Verdict != eventlog.VerdictRename {
			t.Errorf("type %q verdict = %q, want RENAME", typ, ts.Verdict)
		}
	}
}

// TestDeclareOnlyFutureTypes (§2, seams): the 17 types declared now with
// producers in later packets are present and marked declare-only.
func TestDeclareOnlyFutureTypes(t *testing.T) {
	reg := eventlog.Registry()
	declareOnly := []string{
		"stage.started", "stage.finished", "run.parked", "run.resumed",
		"ask.observed", "ask.answered", "decision.recorded",
		"watchdog.flagged", "watchdog.annotated", "watchdog.suppressed",
		"drift.finding", "canary.result", "retention.compacted", "tool.called",
		"benchmark.pair_recorded", "eval.score_recorded", "run.summary_written",
	}
	if len(declareOnly) != 17 {
		t.Fatalf("expected 17 declare-only types, listed %d", len(declareOnly))
	}
	for _, typ := range declareOnly {
		ts, ok := reg.TypeSpec(typ)
		if !ok {
			t.Errorf("declare-only type %q not registered", typ)
			continue
		}
		if ts.Status != eventlog.StatusDeclareOnly {
			t.Errorf("type %q status = %q, want declare_only (no producer yet)", typ, ts.Status)
		}
	}
}

// TestOQ2AdmittedUnderBroadenedScope (item 10): the un-homed B3/B4 machinery
// types are admitted under the broadened Platform scope (worker.*/registry.*/
// preview.*/backup platform.*/local.*), with worker.compiled riding Routing —
// per the OQ2-(A) disposition (a formalization, no Spec edit). The Platform
// family carries the scope note documenting that reading.
func TestOQ2AdmittedUnderBroadenedScope(t *testing.T) {
	reg := eventlog.Registry()
	plat, ok := reg.FamilySpec(eventlog.FamilyPlatform)
	if !ok || !strings.Contains(plat.Scope, "asset-lifecycle") {
		t.Errorf("Platform family must document the OQ2-(A) broadened scope; got %q", plat.Scope)
	}
	underPlatform := []string{
		"worker.template_created", "worker.automation_run", "worker.graduated",
		"registry.registered", "registry.drift", "preview.started", "preview.stopped",
		"platform.snapshot", "platform.restore_drill_flag",
		"local.unmetered_defect", "local.alias_retargeted",
	}
	for _, typ := range underPlatform {
		fam, known := reg.Classify(typ)
		if !known || fam != eventlog.FamilyPlatform {
			t.Errorf("%q admitted family = %q,%v; want Platform (OQ2-(A))", typ, fam, known)
		}
	}
	if fam, _ := reg.Classify("worker.compiled"); fam != eventlog.FamilyRouting {
		t.Errorf("worker.compiled family = %q; want Routing", fam)
	}
	// registry.drift is project-topology drift, NOT the outside-world
	// Drift&canary family (the B5-6 name-collision flag).
	if fam, _ := reg.Classify("registry.drift"); fam == eventlog.FamilyDriftCanary {
		t.Errorf("registry.drift must NOT map to Drift&canary — it is project-topology drift (Platform)")
	}
}

// TestCompletenessEveryCapabilityHasAFamily (R5, item 9): every S2.1–S2.11 /
// S14 Coverage capability maps to at least one registered family — a
// capability with no family is a contract defect (S14.2 rule 1). No side
// store: every capability is answered from run_events + its in-substrate
// sibling rows (checkpoints, asks), never a table outside the log.
func TestCompletenessEveryCapabilityHasAFamily(t *testing.T) {
	declared := make(map[eventlog.Family]bool)
	for _, f := range eventlog.Registry().Families() {
		declared[f.Name] = true
	}
	coverage := []struct {
		capability string
		family     eventlog.Family
	}{
		{"11.1 audit trail + retention", eventlog.FamilyRunSummary},
		{"11.2/S2.11/15.3 benchmark", eventlog.FamilyBenchmarkEval},
		{"S2.1 full traceability", eventlog.FamilyRunLifecycle},
		{"S2.2/S3.9 live inspection", eventlog.FamilyRunLifecycle},
		{"S2.3 verdict records per round", eventlog.FamilyVerificationVerdict},
		{"S2.4 human decisions recorded", eventlog.FamilyHumanDecision},
		{"S2.5 cost observability", eventlog.FamilyUsageLimits},
		{"S2.6/7.7 routing explainability", eventlog.FamilyRouting},
		{"S2.7/4.6/3.7 self-health watching", eventlog.FamilyWatchdog},
		{"S2.8 outside-world drift detection", eventlog.FamilyDriftCanary},
		{"S2.9 auditable learning (trace side)", eventlog.FamilyKnowledgeInjection},
		{"S2.10 queryable history", eventlog.FamilyRunSummary},
		{"5.6 escalation paths proven by tests", eventlog.FamilyOrchestration},
		{"5.7 rubric falsifiability", eventlog.FamilyBenchmarkEval},
		{"7.3 model-change revalidation", eventlog.FamilyBenchmarkEval},
	}
	for _, c := range coverage {
		if !declared[c.family] {
			t.Errorf("S14 capability %q has no registered family (%q missing) — a contract defect (S14.2 rule 1)", c.capability, c.family)
		}
	}
	// The remaining families back capabilities that also live in-substrate.
	for _, f := range []eventlog.Family{
		eventlog.FamilyCheckpoint, eventlog.FamilyGateAsk,
		eventlog.FamilyPlatform, eventlog.FamilyToolsArtifacts,
	} {
		if !declared[f] {
			t.Errorf("family %q missing", f)
		}
	}
}

// TestRequiredFieldConformance (R10): representative payloads faithful to the
// producers contain their family's contract-minimum identifying keys. Keys
// whose data lives in a sibling row (checkpoints/asks) or a later S10 seam are
// not asserted on the event payload (S02.2 in-substrate; OQ4 test-time only,
// so a B5-8 compaction-stripped payload never fails a runtime gate).
func TestRequiredFieldConformance(t *testing.T) {
	reg := eventlog.Registry()
	cases := []struct {
		typ    string
		golden string
		keys   []string
	}{
		{
			"run.state_changed",
			`{"from":"running","to":"completed","reason":"done","actor":"platform"}`,
			[]string{"from", "to"},
		},
		{
			"routing.decided",
			`{"cause":"selector-match","score":0.9,"model":"m","lane":"anthropic","window_tokens":200000,"plain_reason":"best fit for the domain"}`,
			[]string{"cause", "model", "lane", "plain_reason"},
		},
		{
			"knowledge.injected",
			`{"kind":"assembly","stage":"plan","task_id":"t1","ledger_version":3,"items":[{"item_id":"i1","source_path":"p","content_hash":"h","version":"v1","selector_rule":"r","precedence_label":"pinned"}]}`,
			[]string{"kind", "stage", "items"},
		},
		{
			"settings.changed",
			`{"key":"obs.sse_keepalive","scope":"global","user":"","change":"set","old":20,"new":25,"actor":{"kind":"operator","id":"op"},"reason":"tune"}`,
			[]string{"key", "old", "new", "actor", "reason"},
		},
		{
			"verdict.recorded",
			`{"round":2,"verdict":"pass","ac_ids":["AC1"],"rubric_id":"rb","rubric_version":1,"judge_model":"j","golden_set":{"measured":false},"domain":"software","revision":1,"content_sha256":"abc","retention":"keep-forever"}`,
			[]string{"round", "ac_ids", "golden_set"},
		},
		{
			"helper.spawned",
			`{"spawn_id":"sp1","ts":"t","task_id":"tk","owner":"o","parent_session":"coordinator","child_session":"cs","depth":1,"trigger":"tg","reason":"r","brief_hash":"bh","model":"m","lane":"l","budget":{"turns":8,"tokens":1000}}`,
			[]string{"spawn_id", "depth", "trigger", "reason", "brief_hash", "budget"},
		},
		{
			"intake.delta_decision",
			`{"id":"d1","origin":"coverage","items":[],"ask_id":"a1","presented_bytes":128,"issued_ts":"t","decision":"approve"}`,
			[]string{"id", "ask_id", "presented_bytes"},
		},
		{
			"checkpoint.written",
			`{"model_id":"claude","session_id":"s1","message_index":4,"artifact_snapshot_ref":"ref"}`,
			[]string{"model_id", "session_id"},
		},
	}
	for _, c := range cases {
		if _, ok := reg.TypeSpec(c.typ); !ok {
			t.Errorf("required-field golden references unregistered type %q", c.typ)
			continue
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(c.golden), &m); err != nil {
			t.Errorf("%s golden is not valid JSON: %v", c.typ, err)
			continue
		}
		for _, k := range c.keys {
			if _, ok := m[k]; !ok {
				t.Errorf("%s representative payload missing contract-minimum key %q (S14.2)", c.typ, k)
			}
		}
	}

	// knowledge.injected: each manifest entry carries the six S14.2
	// trace-manifest fields verbatim.
	var manifest struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(cases[2].golden), &manifest); err != nil {
		t.Fatalf("manifest golden: %v", err)
	}
	if len(manifest.Items) == 0 {
		t.Fatal("knowledge.injected golden has no manifest items")
	}
	for _, field := range []string{"item_id", "source_path", "content_hash", "version", "selector_rule", "precedence_label"} {
		if _, ok := manifest.Items[0][field]; !ok {
			t.Errorf("knowledge.injected manifest entry missing S14.2 field %q", field)
		}
	}
}

// TestLedgerUpdateNamingException (§2): the S02.2-named ledger_update keeps
// its underscore name (a documented exception to family.type dotting) and
// classifies to Knowledge injection.
func TestLedgerUpdateNamingException(t *testing.T) {
	if ledger.EventLedgerUpdate != "ledger_update" {
		t.Errorf("ledger_update renamed to %q — S02.2 binds the name verbatim", ledger.EventLedgerUpdate)
	}
	fam, known := eventlog.Classify("ledger_update")
	if !known || fam != eventlog.FamilyKnowledgeInjection {
		t.Errorf("Classify(ledger_update) = %q,%v; want KnowledgeInjection,true", fam, known)
	}
}
