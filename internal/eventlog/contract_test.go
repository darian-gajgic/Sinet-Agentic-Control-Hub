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
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/backup"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/benchmark"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/chat"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/retention"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/shell"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchdog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// producerTypes is the complete inventory of the 87 event types minted by the
// B0–B4 producers, the three B5-3 watchdog types, the B5-4 conformance
// eval.score_recorded, the B5-6A watchlist drift.finding, the B5-6B
// canary.result, the three B5-7 benchmark-practice types and the two B5-8A
// retention types, referenced
// through each package's exported Event*
// constant so a producer-side value change (or a deleted constant) is caught
// here at compile or assertion time. The 13 worker-asset lifecycle constants
// are package-private (internal/worker/store.go:43-55, evTemplateCreated…), so
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
		// stage (6): the S04 spawn/helper trio, the S05.3 overflow record, and
		// the B6-1 S14.2 family-1 stage-session boundary pair — one row per
		// fresh engine stage session across every role, its close carrying the
		// outcome (stageevents.go).
		stage.EventSpawn, stage.EventSpawnRefused, stage.EventHelper, stage.EventContextOverflow,
		stage.EventStageStarted, stage.EventStageFinished,
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
		// watchdog (3): the B5-3 S14.4 producers
		watchdog.EventFlagged, watchdog.EventAnnotated, watchdog.EventSuppressed,
		// conformance (1): the B5-4 S14.5 recording surface
		conformance.EventScoreRecorded,
		// watchlist (2): the B5-6A S14.6 outside-world drift producer and the
		// B5-6B S14.6 ¶3 API-canary producer — the two halves of family 11
		watchlist.EventDriftFinding, watchlist.EventCanaryResult,
		// benchmark (3): the B5-7 S14.7 practice around BENCH-REG. The pair
		// record and the alarm complete family 14; decision.recorded flips from
		// declare-only because the §12 alarm disposition and the §4.2.1 opt-in
		// consent flip are both human decisions and both are logged as one.
		benchmark.EventPairRecorded, benchmark.EventAlarm, benchmark.EventDecision,
		// retention (2): the B5-8A S14.9 producers. run.summary_written is
		// appended at the run's terminal transition (family 15 becomes fully
		// minted); retention.compacted is the compaction pass logging itself.
		retention.EventSummaryWritten, retention.EventCompacted,
		// history (1): the B5-8B C-half S14.10 ¶3 producer — the Layer-2
		// open-SQL surface auditing every generated query, INCLUDING every
		// refusal. It is the packet's only new type and the only reason the
		// registry grows past B5-8A's 94.
		history.EventQueryAudited,
		// chat (8): the B6-7 S15.7 conversational assistant. Session
		// create/rename/delete, the turn lifecycle (start/settle/abandon) and
		// the file exchange (upload/delete) — every mutation the family has, so
		// "every mutation lands on the event log" (S15.2 rule 3) is complete
		// rather than sampled. All eight ADMIT into family 12 under the same
		// broadened-Platform reading that homes worker./registry./preview./local.
		// and history.query_audited: a session is an owner-attributed
		// control-plane asset and a turn is a surface auditing itself. No new
		// family, so no S00.9 amendment. Payloads are refs-not-blobs — message
		// bodies live in chat_messages (migration 0020) and never in the log.
		chat.EventSessionCreated, chat.EventSessionRenamed, chat.EventSessionDeleted,
		chat.EventTurnStarted, chat.EventTurnSettled, chat.EventTurnAbandoned,
		chat.EventFileUploaded, chat.EventFileDeleted,
		// api (1): the B6-2A S15.6 approval-inbox CO-PRODUCER of the already
		// registered decision.recorded — the effect-card approve/deny, the two
		// answer acts whose journal transitions leave no run_events row of
		// their own. It mints no NEW type, so the inventory is unchanged; it is
		// listed here so the declare-once assertion covers the second producer
		// of a shared type as strictly as it covers the first (CONVENTIONS §29).
		api.EventDecisionRecorded,
		// metering (1): the B6-3A S10.3 price-table producer. The operator
		// appending one effective-dated row to the durable price table
		// (migration 0019) — the ONE genuinely new type this packet registers,
		// and the only reason the registry grows past B6-1's 95. It is not a
		// settings.changed because that type's contract minimum mirrors
		// settings_events, whose `key` is a declared dotted registry key and
		// whose old→new is a value replacement; an effective-dated append has
		// neither (see its contract.go note).
		metering.EventPriceRowAdded,
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

// TestInventoryTotals pins the reconciliation counts (§2): B5-3 minted the
// three watchdog types, B5-4 minted eval.score_recorded, B5-6A minted
// drift.finding, B5-6B minted canary.result, B5-7 minted
// benchmark.pair_recorded + benchmark.alarm and flipped decision.recorded, and
// B5-8A flipped run.summary_written + retention.compacted — 87 minted +
// 7 declare-only = 94. B5-8B's C half then REGISTERS ONE GENUINELY NEW TYPE,
// history.query_audited (§29: "a new minted type is a contract.go entry + a
// producerTypes() inventory entry in the same packet"; a new FAMILY would have
// been an S00.9 amendment and none is needed — it lands in family 12 under the
// same OQ2-(A) reading as the other platform-operational groups). B6-1 then
// FLIPS stage.started + stage.finished declare→minted (§29 flip mechanics: the
// producer is internal/stage's Session boundary; no new type, no new family),
// so 90 minted + 5 declare-only = 95 registered types — the registry does not
// grow, the minted share does. Families 1 (Run lifecycle), 11 (Drift & canary),
// 14 (Benchmark & eval) and 15 (Run summary) are all fully minted.
// B6-2A adds no type at all: internal/api joins internal/benchmark as a
// CO-PRODUCER of decision.recorded, which is what its contract row
// pre-authorized. The inventory is therefore counted over DISTINCT types — a
// type produced from two places is still one registered type, and counting the
// producer LIST would have turned a co-production into a phantom growth.
// B6-2B and B6-2C likewise add no type — both extend the same co-production.
// B6-3A REGISTERS ONE GENUINELY NEW TYPE, price.row_added (§29: "a new minted
// type is a contract.go entry + a producerTypes() inventory entry in the same
// packet"; no new FAMILY, so no S00.9 amendment — it lands in family 12 under
// the same OQ2-(A) reading as the other platform-operational groups), so the
// registry moves 90/5/95 → 91 minted + 5 declare-only = 96 registered.
// B6-7 REGISTERS EIGHT, the chat.* set of the S15.7 conversational assistant —
// its session, turn and exchange lifecycle, which is every mutation the family
// has. No new FAMILY: all eight ADMIT into family 12 under the same OQ2-(A)
// broadened-Platform reading that already homes the worker./registry./preview./
// local. asset groups and history.query_audited, so no S00.9 amendment is
// needed. The registry moves 91/5/96 → 99 minted + 5 declare-only = 104
// registered.
func TestInventoryTotals(t *testing.T) {
	distinct := map[string]bool{}
	for _, typ := range producerTypes() {
		distinct[typ] = true
	}
	if n := len(distinct); n != 99 {
		t.Errorf("producer inventory = %d distinct types, want 99 (§2; +3 watchdog at B5-3, +1 eval.score_recorded at B5-4, +1 drift.finding at B5-6A, +1 canary.result at B5-6B, +3 at B5-7, +2 at B5-8A, +1 history.query_audited at B5-8B, +2 stage.* at B6-1, +1 price.row_added at B6-3A, +8 chat.* at B6-7; B6-2A/B/C co-produce decision.recorded and add none)", n)
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
	if minted != 99 {
		t.Errorf("registered minted types = %d, want 99", minted)
	}
	if declareOnly != 5 {
		t.Errorf("declare-only types = %d, want 5", declareOnly)
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
	// The S14.2 "Required payload fields (contract minimum)" column, verbatim
	// and frozen here so any future edit to a registry descriptor is caught as
	// drift — the enforcement B5 packets depend on (D2 verbatim lock).
	wantFields := map[eventlog.Family]string{
		eventlog.FamilyRunLifecycle:        "from→to + cause (every FSM transition appends [S02.3]); stage id/kind, stage-brief hash, outcome",
		eventlog.FamilyCheckpoint:          "checkpoint id + the S02.4 (a)–(e) refs (usage block, session cursor, ledger revision, artifact snapshot, version fields)",
		eventlog.FamilyUsageLimits:         "per-paid-call token/cache fields + price-table version; limit class (five-class taxonomy [XREF:S10]) + provider signal + resets-at; park cause + parked-until",
		eventlog.FamilyGateAsk:             "ask id, kind (gate|question), invocation-snapshot ref, answer ref — projected from asks [S02.2]",
		eventlog.FamilyHumanDecision:       "actor, card id + type, decision, presented-at → decided-at (latency); effect approvals additionally journaled [S02.7]",
		eventlog.FamilyVerificationVerdict: "rubric id + version, axis (spec-compliance|outcome-sanity), per-criterion results, findings [F1..Fn] with anchors, blocker/note split, round number, judge model id + its golden-set error rates at judging time",
		eventlog.FamilyRouting:             "{cause enum, score, signals, routed worker/model/lane, effort mode, plain_reason} — one row per routed call; plain-language reason is mandatory, not prose-optional",
		eventlog.FamilyKnowledgeInjection:  "the trace-manifest entries {item_id, source_path, content_hash, version, selector_rule, precedence_label}, incl. post-compaction re-injection",
		eventlog.FamilyOrchestration:       "spawn-record ref (trigger, reason, depth, budgets, brief hash [S04.4]); refusals name the failed check",
		eventlog.FamilyWatchdog:            "rule id, trigger evidence (signature, counts, spend vs baseline), Tier-1 annotation ref; suppressions are tuning signals (S14.4)",
		eventlog.FamilyDriftCanary:         "source, lane(s), change class (price/terms/limits/models/endpoints/billing-regime), severity, one-line summary, incident fingerprint; canary kind + lane + pass/fail/delta",
		eventlog.FamilyPlatform:            "settings: {actor, key, old, new, reason} (mirrors settings_events [S01.10]); auth: kind/user/device [S01.9]; compaction anomaly: {stage, lane, engine version, window fill at trigger, pinned sections at risk, summary artifact ref} [S05.7]; compaction pass logs itself",
		eventlog.FamilyToolsArtifacts:      "tool name, args digest, duration, artifact refs; artifact ref + hash",
		eventlog.FamilyBenchmarkEval:       "the BENCH-REG §14 record schema verbatim; suite id + version, asset id + version, metrics, runner + runner version",
		eventlog.FamilyRunSummary:          "summary artifact ref + generation inputs digest",
	}
	if len(wantFields) != 15 {
		t.Fatalf("expected 15 verbatim S14.2 descriptors, listed %d", len(wantFields))
	}
	seen := make(map[eventlog.Family]bool)
	for _, f := range fams {
		seen[f.Name] = true
		want, ok := wantFields[f.Name]
		if !ok {
			t.Errorf("unexpected family %q (not in the S14.2 table)", f.Name)
			continue
		}
		if f.RequiredFields != want {
			t.Errorf("family %q RequiredFields drifted from S14.2 verbatim:\n got: %q\nwant: %q", f.Name, f.RequiredFields, want)
		}
		if strings.TrimSpace(f.Anchor) == "" {
			t.Errorf("family %q has no S14.2 anchor", f.Name)
		}
	}
	for fam := range wantFields {
		if !seen[fam] {
			t.Errorf("S14.2 family %q missing from the registry", fam)
		}
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

// TestDeclareOnlyFutureTypes (§2, seams): the 7 types declared now with
// producers in later packets are present and marked declare-only. The three
// watchdog types left this set when B5-3 became their producer;
// eval.score_recorded left when B5-4 became its producer; drift.finding left
// when B5-6A (the S14.6 watchlist executor) became its producer;
// canary.result left when B5-6B (the S14.6 ¶3 API canary layer) became its;
// benchmark.pair_recorded + decision.recorded left when B5-7 (the S14.7
// benchmark practice) became theirs; and run.summary_written +
// retention.compacted left when B5-8A (the S14.9 retention substrate) became
// theirs; and stage.started + stage.finished left when B6-1 put the producer on
// internal/stage's Session boundary — so family 1 joins 11, 14 and 15 as fully
// minted. The remaining five are the ones B6-1 deliberately did NOT mint:
// run.parked/resumed are manifested as run.state_changed transitions,
// ask.observed/answered are projected from the asks table, and a
// tool-use-START producer does not exist (the engine surfaces tool_result).
func TestDeclareOnlyFutureTypes(t *testing.T) {
	reg := eventlog.Registry()
	declareOnly := []string{
		"run.parked", "run.resumed",
		"ask.observed", "ask.answered",
		"tool.called",
	}
	if len(declareOnly) != 5 {
		t.Fatalf("expected 5 declare-only types, listed %d", len(declareOnly))
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
		// B5-8B drain D9: the Layer-2 query audit rides the same OQ2-(A)
		// admission, so it is PINNED here rather than left as prose.
		history.EventQueryAudited,
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
			// Family 1's stage limb: "stage id/kind, stage-brief hash,
			// outcome". The opening row carries the identity and the hash; the
			// outcome is the CLOSE's field and is deliberately absent here — a
			// stage that has only started has no outcome to report (B6-1).
			"stage.started",
			`{"stage":"step-2","kind":"execute","brief_hash":"2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"}`,
			[]string{"stage", "kind", "brief_hash"},
		},
		{
			"stage.finished",
			`{"stage":"step-2","kind":"execute","brief_hash":"2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae","outcome":"completed"}`,
			[]string{"stage", "kind", "brief_hash", "outcome"},
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
		{
			"usage.recorded",
			`{"message_id":"msg_1","message_index":2,"model":"claude-x","usage":{"input_tokens":120,"output_tokens":48,"cache_read_input_tokens":10,"cache_creation_input_tokens":0}}`,
			[]string{"message_id", "message_index", "usage"},
		},
		{
			"engine.gate_ask",
			`{"ask_id":"toolu_1","tool_name":"Bash","tool_input":{"command":"ls"},"engine_expiry":"2026-07-23T00:00:00Z"}`,
			[]string{"ask_id", "tool_name"},
		},
		{
			// tool.completed carries the completion ref + result excerpt; the
			// tool name + args digest ride the tool-use side (tool.called,
			// declare-only), linked back here via tool_use_id.
			"tool.completed",
			`{"tool_use_id":"toolu_1","excerpt":"exit 0","truncated":false}`,
			[]string{"tool_use_id", "excerpt"},
		},
		{
			"artifact.produced",
			`{"deliverable_id":"dlv_1","n":1,"pin_kind":"content","content_sha256":"abc123","platform_ref":"obj/ref","attempt_ref":"att_1","objects":2}`,
			[]string{"deliverable_id", "content_sha256"},
		},
		{
			// auth.event: kind = the event type, user = the envelope user_id
			// (15.6), device = device_login; via names the login method.
			"auth.login",
			`{"via":"grant","device_login":"alice@tailnet"}`,
			[]string{"via", "device_login"},
		},
		{
			// watchdog.flagged (B5-3): rule id + trigger evidence
			// {signature, counts, spend vs baseline} + Tier-1 annotation ref —
			// the FamilyWatchdog descriptor, keyed on the real FlagPayload fields.
			"watchdog.flagged",
			`{"rule":"watchdog.loop","anomaly_class":"watchdog.loop","severity":"flag-now","signature":"a1b2c3d4","counts":{"repeats":5},"annotation_ref":42,"detail":"identical tool-call signature repeated","parked":true,"actor":"platform"}`,
			[]string{"rule", "anomaly_class", "severity", "signature", "counts"},
		},
		{
			// watchdog.annotated (B5-3): the Tier-1 verdict referenced by a flag.
			"watchdog.annotated",
			`{"rule":"watchdog.loop","verdict":"loop","confidence":0.88,"evidence":"Bash d1 ×5","reason":"repeats the same call","model":"qwen","actor":"platform"}`,
			[]string{"rule", "verdict", "confidence", "evidence"},
		},
		{
			// watchdog.suppressed (B5-3): a per-rule tuning signal carrying the
			// acting principal (drain D13) and the running count.
			"watchdog.suppressed",
			`{"rule":"watchdog.loop","actor":"op","count":1,"reason":"operator suppression (tuning signal, S14.4)","propose_retune":false}`,
			[]string{"rule", "actor", "count"},
		},
		{
			// eval.score_recorded (B5-4): the S14.5 conformance-registry recording
			// surface — the FamilyBenchmarkEval contract minimum verbatim: suite
			// id + version, asset id + version, metrics, runner + runner version
			// (internal/conformance RecordResult emits exactly this shape).
			"eval.score_recorded",
			`{"suite_id":"adapter-anthropic","suite_version":"claudecli-conformance@2.1.215","asset_id":"engine:claude-cli","asset_version":"2.1.215","metrics":{"cases_total":42,"cases_passed":42},"runner":"go test ./internal/adapters/claudecli/","runner_version":"go1.26.5","result":"green"}`,
			[]string{"suite_id", "suite_version", "asset_id", "asset_version", "metrics", "runner", "runner_version"},
		},
		{
			// drift.finding (B5-6A): the FamilyDriftCanary contract minimum's
			// drift half verbatim — source, lane(s), change class, severity,
			// one-line summary, incident fingerprint. Keyed on the real
			// watchlist.FindingPayload fields. The canary half (canary kind +
			// lane + pass/fail/delta) arrives with canary.result at B5-6B.
			"drift.finding",
			`{"source":"https://claude.com/pricing","lanes":["anthropic"],"change_class":"price","severity":"flag-now","summary":"Opus input price moved","fingerprint":"9f2c1ab34de5f607","row_id":"t1-anthropic-pricing","kind":"page","classified":true,"revalidation":{"triggered":false,"reason":"no revalidation triggered: change class \"price\" names no model (OQ4(a))"}}`,
			[]string{"source", "lanes", "change_class", "severity", "summary", "fingerprint"},
		},
		{
			// canary.result (B5-6B): the FamilyDriftCanary contract minimum's
			// CANARY half verbatim — canary kind + lane + pass/fail/delta. Keyed
			// on the real watchlist.CanaryPayload fields. The two halves of the
			// family are now both minted and both asserted here.
			"canary.result",
			`{"canary_kind":"auth","lane":"anthropic","result":"fail","delta":0,"summary":"auth canary on lane anthropic classified policy-revocation-suspected — lane-freeze","change_class":"terms","action":"lane-freeze","limit_class":4,"purpose_tag":"probe","workload_class":"probe","verified_on":"2026-07-27T09:00:00Z"}`,
			[]string{"canary_kind", "lane", "result", "delta"},
		},
		{
			// benchmark.pair_recorded (B5-7): the FamilyBenchmarkEval contract
			// minimum is "the BENCH-REG §14 record schema verbatim", so the keys
			// asserted here ARE §14's list. Fields beyond it are additive
			// provenance — §14 registers minimums.
			"benchmark.pair_recorded",
			`{"pair_id":"bp-42","domain":"software-development","task_class":"software/standard","sampled_ts":"2026-08-01T12:00:00Z","recorded_ts":"2026-08-01T13:00:00Z","requester_id":"alice","epoch_id":"software-development#e1","platform_model":"claude-opus-4-8","direct_model":"claude-opus-4-8-consumer","platform_consumption_usd":1.2,"direct_consumption_usd":0.4,"direct_measured":true,"artifact_a_ref":"benchmark://pair/bp-42/a","artifact_b_ref":"benchmark://pair/bp-42/b","position":"platform-a","verdict":"A","platform_guess":"A","guess_correct":true,"declined":false,"task_id":"t1","deliverable_id":"t1.d1","platform_run_id":"t1.r1","direct_run_id":"bp-42.direct","domain_source":"verdict.recorded","phase":"pre-gate","rate_pct":100,"length_platform":2048,"length_direct":1400,"retention":"keep-forever","protocol_ref":"Spec/benchmark-preregistration-v1.md (BENCH-REG v1, signed 2026-07-18)"}`,
			[]string{
				"pair_id", "domain", "task_class", "sampled_ts", "recorded_ts",
				"platform_model", "direct_model",
				"platform_consumption_usd", "direct_consumption_usd",
				"artifact_a_ref", "artifact_b_ref", "position",
				"verdict", "platform_guess", "declined", "epoch_id", "requester_id",
			},
		},
		{
			// benchmark.alarm (B5-7): the BENCH-REG §12 alarm, carrying the full
			// record it tripped on plus the standing expansion freeze. It kills
			// nothing — the freeze binds by visibility and by gate limb (c).
			"benchmark.alarm",
			`{"action":"raise","domain":"software-development","epoch_id":"software-development#e1","severity":"flag-now","summary":"the platform is losing in software-development: 1−G = 0.969 exceeds the registered 0.95 at w=0, m=4 (BENCH-REG §12)","loss_g":0.969,"threshold":0.95,"rate":{"domain":"software-development","epoch_id":"software-development#e1","win_rate":0,"n":4,"w":0,"g":0.031,"loss_g":0.969,"tie_rate":0,"both_bad_rate":0,"decline_rate":0,"guess_accuracy":0.5,"guess_n":4,"honest_claims":[],"honest_claims_readout":"","arm_parity":[]},"expansion_freeze":true,"retention":"keep-forever"}`,
			[]string{"action", "domain", "epoch_id", "severity", "loss_g", "threshold", "rate", "expansion_freeze"},
		},
		{
			// decision.recorded (B5-7): the S14.2 Human-decision contract minimum
			// verbatim — "actor, card id + type, decision, presented-at →
			// decided-at (latency)".
			"decision.recorded",
			`{"actor":"op","card_id":"benchmark-alarm:software-development:412","card_type":"benchmark.alarm","decision":"fix-and-continue-accruing","presented_at":"2026-08-01T13:00:00Z","decided_at":"2026-08-01T14:30:00Z","latency_s":5400,"reason":"rubric bug found and fixed","subject":"software-development","ref":"BENCH-REG §12"}`,
			[]string{"actor", "card_id", "card_type", "decision", "presented_at", "decided_at", "latency_s"},
		},
		{
			// run.summary_written (B5-8A): the FamilyRunSummary contract minimum
			// is exactly "summary artifact ref + generation inputs digest". The
			// structured story lives in the run_summaries row the ref names —
			// the summary must SURVIVE the compaction that strips the trace it
			// was computed from, so it cannot ride the payload it outlives.
			"run.summary_written",
			`{"summary_ref":"run_summary:t1.r1","inputs_digest":"sha256:0f4b1c9d2e5a6b78c9d0e1f2a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6","final_state":"completed","aggregate_only":false}`,
			[]string{"summary_ref", "inputs_digest"},
		},
		{
			// retention.compacted (B5-8A): the S14.9 ¶2 pass logging itself. The
			// FamilyPlatform descriptor ends "compaction pass logs itself"; the
			// payload records WHAT it compacted — the horizon applied, the owner,
			// the boundary event_seq, per-family strip counts and bytes
			// reclaimed, one leg per owner because the horizon is per-user.
			"retention.compacted",
			`{"as_of":"2026-08-01T03:00:00Z","owners":[{"user_id":"alice","horizon_months":6,"boundary_ts":"2026-02-01T03:00:00Z","boundary_event_seq":9412,"events_stripped":318,"bytes_reclaimed":1048576,"by_family":{"tools_artifacts":301,"orchestration":17},"transcripts_stripped":4}],"events_stripped":318,"bytes_reclaimed":1048576}`,
			[]string{"as_of", "owners", "events_stripped", "bytes_reclaimed"},
		},
		{
			// history.query_audited (B5-8B C half): S14.10 3's "every generated
			// query audit-logged". The contract minimum is what makes the record
			// REVIEWABLE — what was asked, what the seat produced, what actually
			// executed, and under which bounds — plus the outcome, because a
			// refusal is a row too and is the interesting kind.
			"history.query_audited",
			`{"question":"which runs cost the most?","outcome":"refused","sql_generated":"SELECT payload FROM run_events","sql_executed":"","views":[],"limit_injected":200,"timeout_ms":5000,"refusal":"history: refused by the Layer-2 guardrail: \"run_events\" is not an allowlisted view","row_count":0,"truncated":false,"model":"Arctic-Text2SQL-R1-7B","alias":"sql-open","as_operator":true,"owner_scoped":0,"raw_output_len":412}`,
			[]string{"question", "outcome", "sql_generated", "limit_injected", "timeout_ms", "alias"},
		},
		{
			// price.row_added (B6-3A): the operator appending one effective-dated
			// row to the S10.3 price table. The contract minimum is the S10.3 row
			// shape — a price is only auditable if you can see WHAT was priced, at
			// WHAT rates, FROM WHEN, and on WHOSE authority — plus the source and
			// verification date, because D5 quotes the provider actually serving
			// the lane and a row whose provenance is unstated cannot be checked.
			// There is no `old`: an effective-dated append replaces nothing.
			"price.row_added",
			`{"row_id":1,"model":"claude-sonnet-4-5","lane":"anthropic","unit_prices":{"input_usd":0.000003,"output_usd":0.000015,"cache_read_usd":0.0000003,"cache_creation_usd":0.00000375,"server_tool_usd":0},"effective_from":"2026-08-01T00:00:00Z","verified_on":"2026-07-28T00:00:00Z","source":"anthropic pricing page","actor":"op","reason":"list price flip announced"}`,
			[]string{"row_id", "model", "lane", "unit_prices", "effective_from", "verified_on", "source", "actor"},
		},
		// The eight chat.* types (B6-7). Their shared contract minimum is
		// OWNER + SUBJECT: every one names the session, turn or file it is
		// about, and every one is owner-attributed on the envelope (15.6).
		// None carries a message body, a title string or an answer — a
		// transcript is CONTENT and lives in chat_messages / chat_turns
		// (migration 0020), so these payloads are refs, counts and closed
		// vocabularies (S02.2 refs-not-blobs).
		{
			"chat.session_created",
			`{"session":"cs-9a1f2b3c4d5e6f70","owner":"alice"}`,
			[]string{"session", "owner"},
		},
		{
			// The title itself is deliberately absent: what is auditable is
			// which KIND of thing named the session, not what it is called.
			"chat.session_renamed",
			`{"session":"cs-9a1f2b3c4d5e6f70","source":"human"}`,
			[]string{"session", "source"},
		},
		{
			// A hard delete's record is the audit that the rows went, so it
			// carries COUNTS of what was removed and none of the content.
			"chat.session_deleted",
			`{"session":"cs-9a1f2b3c4d5e6f70","messages":6,"turns":3}`,
			[]string{"session", "messages", "turns"},
		},
		{
			// `kind` is the client-routed verb from the closed set; `message`
			// is the REF to the body and `text_len` the only thing said about
			// what it contained.
			"chat.turn_started",
			`{"session":"cs-9a1f2b3c4d5e6f70","turn":"ct-1122334455667788","kind":"ask","message":"cm-aabbccddeeff0011","text_len":42}`,
			[]string{"session", "turn", "kind", "message", "text_len"},
		},
		{
			// Which verb answered, which ladder layer and named query, the born
			// task for a handoff, and the count of files the exchange diff
			// attributed to the turn.
			"chat.turn_settled",
			`{"session":"cs-9a1f2b3c4d5e6f70","turn":"ct-1122334455667788","kind":"ask","outcome":"answer","layer":1,"query":"cost_per_run","produced":0}`,
			[]string{"session", "turn", "kind", "outcome", "produced"},
		},
		{
			"chat.turn_abandoned",
			`{"session":"cs-9a1f2b3c4d5e6f70","turn":"ct-1122334455667788","kind":"open_sql"}`,
			[]string{"session", "turn", "kind"},
		},
		{
			// The manifest row's identifying fields — the artifact's identity
			// rather than its contents (the artifact.produced precedent).
			"chat.file_uploaded",
			`{"file":"cf-00ff11ee22dd33cc","owner":"alice","name":"quarterly.csv","size_bytes":8241,"sha256":"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08","origin_session":"cs-9a1f2b3c4d5e6f70"}`,
			[]string{"file", "owner", "name", "size_bytes", "sha256"},
		},
		{
			// A delete whose record does not say WHAT was deleted cannot be
			// reviewed; the hash identifies the departed object without
			// retaining it.
			"chat.file_deleted",
			`{"file":"cf-00ff11ee22dd33cc","owner":"alice","name":"quarterly.csv","sha256":"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"}`,
			[]string{"file", "owner", "name", "sha256"},
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
