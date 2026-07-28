// contract.go is the v0 event-type contract of Spec S14.2 — the declare-once
// registry that makes run_events the complete observability substrate
// (Spec S14.1). It records, as data, the fifteen v0 event-type families with
// their contract-minimum required fields, every event type minted by the
// B0–B4 producers reconciled to a family, the schema_version + upcast-on-read
// hook (Spec S14.2 rule 2, S02.1), and the forward-tolerant Classify helper
// (Spec S14.2 rule 3, S03.1).
//
// The registry is DATA: it declares the canonical type strings itself and
// does NOT import the producer packages (they import eventlog; an import back
// would cycle). The producer↔registry cross-check lives in the external
// contract_test.go. This mirrors the settings-index precedent — declare once
// here, pin the tallies in tests (CONVENTIONS §6, §29).
//
// This file adds no runtime field validation to Append: the S02.1
// validate-before-persist gate stays envelope-only, and required-field
// conformance is asserted at test time over representative payloads (a later
// compaction pass strips payloads past ⚙ retention.compaction_horizon, so a
// runtime required-field gate would fail on a compacted row — Spec S14.9).

package eventlog

import (
	"encoding/json"
	"fmt"
)

// schemaV0 is the schema_version every v0 type carries (Spec S14.2 rule 2);
// the envelope gate already requires schema_version ≥ 1 (eventlog.go).
const schemaV0 = 1

// Family is one of the fifteen v0 event-type families of Spec S14.2. Families
// are binding; their "v0 types" columns are representative — a family admits a
// type by scope (the reconciliation reading, CONVENTIONS §29).
type Family string

const (
	FamilyRunLifecycle        Family = "run_lifecycle"
	FamilyCheckpoint          Family = "checkpoint"
	FamilyUsageLimits         Family = "usage_limits"
	FamilyGateAsk             Family = "gate_ask"
	FamilyHumanDecision       Family = "human_decision"
	FamilyVerificationVerdict Family = "verification_verdict"
	FamilyRouting             Family = "routing"
	FamilyKnowledgeInjection  Family = "knowledge_injection"
	FamilyOrchestration       Family = "orchestration"
	FamilyWatchdog            Family = "watchdog"
	FamilyDriftCanary         Family = "drift_canary"
	FamilyPlatform            Family = "platform"
	FamilyToolsArtifacts      Family = "tools_artifacts"
	FamilyBenchmarkEval       Family = "benchmark_eval"
	FamilyRunSummary          Family = "run_summary"
)

// Verdict is a type's reconciliation disposition against Spec S14.2 (the crux
// of B5-1): how a B0–B4-minted type was mapped to a family.
type Verdict string

const (
	// VerdictConforms: the minted name and fields already match S14.2.
	VerdictConforms Verdict = "CONFORMS"
	// VerdictRename: the minted name was changed to the S14.2-canonical name
	// (constant value change + legacy alias, R14; no run_events migration).
	VerdictRename Verdict = "RENAME"
	// VerdictAdmit: the type fits a family by scope; S14.2 names no distinct
	// canonical type for it (families are representative, CONVENTIONS §29).
	VerdictAdmit Verdict = "ADMIT"
	// VerdictField: a required field is added or seamed for a later producer.
	VerdictField Verdict = "FIELD"
	// VerdictDeclare: declared now for a producer in a later packet; nothing
	// emits it yet.
	VerdictDeclare Verdict = "DECLARE"
)

// TypeStatus records whether a producer emits a type today.
type TypeStatus string

const (
	// StatusMinted: a B0–B4 producer emits this type today.
	StatusMinted TypeStatus = "minted"
	// StatusDeclareOnly: the contract declares it; the producer is a later
	// packet (B5-2/3/6/7/8 or B6). No producer emits it yet.
	StatusDeclareOnly TypeStatus = "declare_only"
)

// UpcastFunc upgrades one event payload across a single schema_version step
// (vN → vN+1). At v0 every registered type is v1, so no real UpcastFunc
// exists; the hook is exercised by a fixture type in the contract test
// (Spec S14.2 rule 2).
type UpcastFunc func(json.RawMessage) (json.RawMessage, error)

// FamilySpec is one S14.2 family: its contract-minimum required payload
// fields (verbatim from the S14.2 table), its spec anchor, and — where the
// reconciliation reading broadens the family scope (CONVENTIONS §29) — the
// scope note documenting that reading.
type FamilySpec struct {
	Name Family
	// RequiredFields is the S14.2 "Required payload fields (contract minimum)"
	// column, verbatim. It is the contract minimum, satisfiable from
	// run_events + the in-substrate sibling rows (checkpoints, asks); it is
	// NOT a per-append runtime gate (S02.1 validate-before-persist stays
	// envelope-only) and NOT the exact per-type key set (payload encodings
	// stay each producer's, S02.2).
	RequiredFields string
	// Anchor is the S14.2 "Anchor" column.
	Anchor string
	// Scope, when set, documents the reconciliation reading that broadens the
	// family beyond the S14.2 "v0 types" column (OQ2-(A); CONVENTIONS §29).
	Scope string
}

// TypeSpec is one registered event type mapped to its family, with its
// reconciliation verdict and provenance, its current schema_version, and its
// per-version upcast chain (empty at v0).
type TypeSpec struct {
	Type          string
	Family        Family
	SchemaVersion int
	Status        TypeStatus
	Verdict       Verdict
	// Provenance is the producer site (file:line) for a minted type, or the
	// owning later packet for a declare-only type.
	Provenance string
	// Upcasts[n] upgrades a vN payload to v(n+1). nil/empty at v0 (identity);
	// the decode hook (Upcast) chains them fromVersion→SchemaVersion.
	Upcasts map[int]UpcastFunc
	// Note carries a reconciliation flag: a rename's old name, a dual-home, a
	// naming exception, or a cross-packet ownership warning.
	Note string
}

// Contract is the declare-once event-type registry (Spec S14.2). The v0
// instance is Registry(); NewContract builds fixture instances for tests and
// future amendment tooling.
type Contract struct {
	families map[Family]FamilySpec
	types    map[string]TypeSpec
	aliases  map[string]Family
	// famOrder / typeOrder give deterministic iteration.
	famOrder  []Family
	typeOrder []string
}

// NewContract assembles a Contract from family specs, type specs, and legacy
// aliases, validating referential integrity: every type's family is declared,
// no type or alias is defined twice, no alias shadows a canonical type, and a
// type at schema_version > 1 supplies the upcast steps its chain needs. It is
// exported so the contract test can build a fixture Contract to exercise the
// upcast hook without polluting the v0 registry.
func NewContract(families []FamilySpec, types []TypeSpec, aliases map[string]Family) (*Contract, error) {
	c := &Contract{
		families: make(map[Family]FamilySpec, len(families)),
		types:    make(map[string]TypeSpec, len(types)),
		aliases:  make(map[string]Family, len(aliases)),
	}
	for _, f := range families {
		if _, dup := c.families[f.Name]; dup {
			return nil, fmt.Errorf("eventlog: family %q declared twice", f.Name)
		}
		c.families[f.Name] = f
		c.famOrder = append(c.famOrder, f.Name)
	}
	for _, ts := range types {
		if ts.Type == "" {
			return nil, fmt.Errorf("eventlog: empty type string")
		}
		if _, dup := c.types[ts.Type]; dup {
			return nil, fmt.Errorf("eventlog: type %q registered twice", ts.Type)
		}
		if _, ok := c.families[ts.Family]; !ok {
			return nil, fmt.Errorf("eventlog: type %q references undeclared family %q", ts.Type, ts.Family)
		}
		if ts.SchemaVersion < 1 {
			return nil, fmt.Errorf("eventlog: type %q has schema_version %d < 1", ts.Type, ts.SchemaVersion)
		}
		for v := 1; v < ts.SchemaVersion; v++ {
			if _, ok := ts.Upcasts[v]; !ok {
				return nil, fmt.Errorf("eventlog: type %q missing upcast v%d→v%d", ts.Type, v, v+1)
			}
		}
		c.types[ts.Type] = ts
		c.typeOrder = append(c.typeOrder, ts.Type)
	}
	for old, fam := range aliases {
		if _, ok := c.types[old]; ok {
			return nil, fmt.Errorf("eventlog: legacy alias %q shadows a canonical type", old)
		}
		if _, ok := c.families[fam]; !ok {
			return nil, fmt.Errorf("eventlog: legacy alias %q references undeclared family %q", old, fam)
		}
		c.aliases[old] = fam
	}
	return c, nil
}

// Classify returns the family of a type and whether it is known. A registered
// canonical type or a registered legacy alias (a renamed type's old name,
// R14) returns its family; any other type returns known=false so a consumer
// logs and skips it — never an error, never fatal (Spec S14.2 rule 3, S03.1).
func (c *Contract) Classify(typeName string) (Family, bool) {
	if ts, ok := c.types[typeName]; ok {
		return ts.Family, true
	}
	if fam, ok := c.aliases[typeName]; ok {
		return fam, true
	}
	return "", false
}

// Upcast applies a type's registered upcast chain to a stored payload, from
// the payload's own schema_version up to the type's current schema_version
// (Spec S14.2 rule 2, S02.1 upcast-on-read). At v0 every type is v1, so a
// v1 payload of a registered type round-trips unchanged. An unregistered type
// is forward-tolerant: its payload passes through unchanged (the consumer
// skips it via Classify).
func (c *Contract) Upcast(typeName string, fromVersion int, payload json.RawMessage) (json.RawMessage, error) {
	ts, ok := c.types[typeName]
	if !ok {
		return payload, nil
	}
	if fromVersion < 1 || fromVersion > ts.SchemaVersion {
		return nil, fmt.Errorf("eventlog: %q payload version %d out of range [1,%d]", typeName, fromVersion, ts.SchemaVersion)
	}
	out := payload
	for v := fromVersion; v < ts.SchemaVersion; v++ {
		fn, ok := ts.Upcasts[v]
		if !ok {
			return nil, fmt.Errorf("eventlog: %q has no upcast v%d→v%d", typeName, v, v+1)
		}
		var err error
		if out, err = fn(out); err != nil {
			return nil, fmt.Errorf("eventlog: upcast %q v%d→v%d: %w", typeName, v, v+1, err)
		}
	}
	return out, nil
}

// FamilySpec returns a family's spec and whether it is declared.
func (c *Contract) FamilySpec(name Family) (FamilySpec, bool) {
	f, ok := c.families[name]
	return f, ok
}

// TypeSpec returns a type's spec and whether it is registered (canonical
// types only; aliases are not TypeSpecs).
func (c *Contract) TypeSpec(typeName string) (TypeSpec, bool) {
	ts, ok := c.types[typeName]
	return ts, ok
}

// Families returns every family spec in declaration order.
func (c *Contract) Families() []FamilySpec {
	out := make([]FamilySpec, 0, len(c.famOrder))
	for _, name := range c.famOrder {
		out = append(out, c.families[name])
	}
	return out
}

// Types returns every registered type spec in registration order.
func (c *Contract) Types() []TypeSpec {
	out := make([]TypeSpec, 0, len(c.typeOrder))
	for _, name := range c.typeOrder {
		out = append(out, c.types[name])
	}
	return out
}

// Aliases returns a copy of the legacy-alias map (old name → family, R14).
func (c *Contract) Aliases() map[string]Family {
	out := make(map[string]Family, len(c.aliases))
	for k, v := range c.aliases {
		out[k] = v
	}
	return out
}

// Registry returns the v0 event-type contract (Spec S14.2), built once.
func Registry() *Contract { return v0Contract }

// Classify resolves a type to its family over the v0 registry (Spec S14.2
// rule 3): the package-level helper a consumer calls to log-and-skip unknown
// types.
func Classify(typeName string) (Family, bool) { return v0Contract.Classify(typeName) }

// Upcast applies the v0 registry's upcast-on-read hook (Spec S14.2 rule 2).
func Upcast(typeName string, fromVersion int, payload json.RawMessage) (json.RawMessage, error) {
	return v0Contract.Upcast(typeName, fromVersion, payload)
}

// v0Contract is the assembled v0 registry; a defect in its static data panics
// at init (the MustCompile precedent), which the contract test catches.
var v0Contract = mustBuildV0()

func mustBuildV0() *Contract {
	c, err := NewContract(v0Families, v0Types, v0Aliases)
	if err != nil {
		panic("eventlog: v0 contract is inconsistent: " + err.Error())
	}
	return c
}

// minted builds a minted type spec (a B0–B4 producer emits it today).
func minted(typ string, fam Family, v Verdict, provenance string, note ...string) TypeSpec {
	return typeSpec(typ, fam, v, StatusMinted, provenance, note...)
}

// declare builds a declare-only type spec (producer is a later packet).
func declare(typ string, fam Family, provenance string, note ...string) TypeSpec {
	return typeSpec(typ, fam, VerdictDeclare, StatusDeclareOnly, provenance, note...)
}

func typeSpec(typ string, fam Family, v Verdict, status TypeStatus, provenance string, note ...string) TypeSpec {
	ts := TypeSpec{Type: typ, Family: fam, SchemaVersion: schemaV0, Status: status, Verdict: v, Provenance: provenance}
	if len(note) > 0 {
		ts.Note = note[0]
	}
	return ts
}

// v0Families are the fifteen S14.2 families with their contract-minimum
// required-field descriptors, verbatim from the S14.2 table.
var v0Families = []FamilySpec{
	{
		Name:           FamilyRunLifecycle,
		RequiredFields: "from→to + cause (every FSM transition appends [S02.3]); stage id/kind, stage-brief hash, outcome",
		Anchor:         "S2.1; [XREF:S05]",
	},
	{
		Name:           FamilyCheckpoint,
		RequiredFields: "checkpoint id + the S02.4 (a)–(e) refs (usage block, session cursor, ledger revision, artifact snapshot, version fields)",
		Anchor:         "D7; [S02.4]",
	},
	{
		Name:           FamilyUsageLimits,
		RequiredFields: "per-paid-call token/cache fields + price-table version; limit class (five-class taxonomy [XREF:S10]) + provider signal + resets-at; park cause + parked-until",
		Anchor:         "D4/D5; S2.5",
	},
	{
		Name:           FamilyGateAsk,
		RequiredFields: "ask id, kind (gate|question), invocation-snapshot ref, answer ref — projected from asks [S02.2]",
		Anchor:         "4.2/4.3",
		Scope:          "stored trace event (engine.gate_ask) + the projected canonical types (ask.observed/ask.answered) materialized over the asks table by B5-2/B5-8 (OQ5)",
	},
	{
		Name:           FamilyHumanDecision,
		RequiredFields: "actor, card id + type, decision, presented-at → decided-at (latency); effect approvals additionally journaled [S02.7]",
		Anchor:         "S2.4",
	},
	{
		Name:           FamilyVerificationVerdict,
		RequiredFields: "rubric id + version, axis (spec-compliance|outcome-sanity), per-criterion results, findings [F1..Fn] with anchors, blocker/note split, round number, judge model id + its golden-set error rates at judging time",
		Anchor:         "S2.3; logic [XREF:S07]",
	},
	{
		Name:           FamilyRouting,
		RequiredFields: "{cause enum, score, signals, routed worker/model/lane, effort mode, plain_reason} — one row per routed call; plain-language reason is mandatory, not prose-optional",
		Anchor:         "S2.6/7.7 [R12 §4.1]",
	},
	{
		Name:           FamilyKnowledgeInjection,
		RequiredFields: "the trace-manifest entries {item_id, source_path, content_hash, version, selector_rule, precedence_label}, incl. post-compaction re-injection",
		Anchor:         "S4.6/S2.9; [S05.4]; [XREF:S09]",
		Scope:          "the S2.9 injection side (knowledge.injected) + the S09 knowledge-lifecycle events, admitted by the knowledge. prefix; the S02.2-named ledger_update rides here as a naming exception",
	},
	{
		Name:           FamilyOrchestration,
		RequiredFields: "spawn-record ref (trigger, reason, depth, budgets, brief hash [S04.4]); refusals name the failed check",
		Anchor:         "D6; [S04.4/S04.5]",
	},
	{
		Name:           FamilyWatchdog,
		RequiredFields: "rule id, trigger evidence (signature, counts, spend vs baseline), Tier-1 annotation ref; suppressions are tuning signals (S14.4)",
		Anchor:         "S2.7",
	},
	{
		Name:           FamilyDriftCanary,
		RequiredFields: "source, lane(s), change class (price/terms/limits/models/endpoints/billing-regime), severity, one-line summary, incident fingerprint; canary kind + lane + pass/fail/delta",
		Anchor:         "S2.8",
	},
	{
		Name: FamilyPlatform,
		// RequiredFields is VERBATIM from the frozen S14.2 table and is pinned
		// byte-for-byte by TestFifteenFamiliesWithRequiredFields (§29). The
		// B5-8B drain asked for the Layer-2 audit's contract minimum to be
		// added here; it is NOT, because that string is spec text and this
		// packet may not author spec text (§8: Spec wins; a conflict is a
		// referral, never a silent resolution). The type's contract minimum is
		// carried where a type's own minimum belongs — on its TypeSpec Note
		// below, and in the required-field golden in contract_test.go.
		RequiredFields: "settings: {actor, key, old, new, reason} (mirrors settings_events [S01.10]); auth: kind/user/device [S01.9]; compaction anomaly: {stage, lane, engine version, window fill at trigger, pinned sections at risk, summary artifact ref} [S05.7]; compaction pass logs itself",
		Anchor:         "13.4; S2.1",
		Scope:          "platform-scope operational + asset-lifecycle events: platform lifecycle/settings/auth/compaction/retention, plus the worker.* (S08), registry.* (S13), preview.* (S13.8), backup platform.* (S13.9), local.* (S12) asset-lifecycle events, and history.* (S14.10) query-surface audit events admitted here by the reconciliation reading (OQ2-(A); CONVENTIONS §29)",
	},
	{
		Name:           FamilyToolsArtifacts,
		RequiredFields: "tool name, args digest, duration, artifact refs; artifact ref + hash",
		Anchor:         "S2.1",
	},
	{
		Name:           FamilyBenchmarkEval,
		RequiredFields: "the BENCH-REG §14 record schema verbatim; suite id + version, asset id + version, metrics, runner + runner version",
		Anchor:         "S2.11; S14.7/S14.8",
	},
	{
		Name:           FamilyRunSummary,
		RequiredFields: "summary artifact ref + generation inputs digest",
		Anchor:         "11.1; S14.9",
	},
}

// v0Types is the reconciliation of every event type minted by B0–B4 (76) plus
// the three B5-3 watchdog types, the B5-4 conformance eval.score_recorded, the
// B5-6A watchlist drift.finding, the B5-6B canary.result, the three B5-7
// benchmark-practice types (benchmark.pair_recorded, benchmark.alarm, and
// decision.recorded flipped from declare) and the two B5-8A retention types
// (run.summary_written, retention.compacted) — 87 minted total, each mapped to
// a family with its verdict and provenance, plus the declare-only types whose
// producers are later packets. The declare-once contract test (contract_test.go)
// asserts every producer Event* constant appears here; a newly-minted type
// absent from this table fails the build.
var v0Types = []TypeSpec{
	// ── Family 1: Run lifecycle (S2.1) ──────────────────────────────────
	minted("run.created", FamilyRunLifecycle, VerdictAdmit, "internal/run/run.go:121", "the ∅→new birth edge"),
	minted("run.state_changed", FamilyRunLifecycle, VerdictRename, "internal/run/run.go:125", "renamed from run.state; payload {from,to,reason=cause,actor,detail}"),
	minted("run.forked", FamilyRunLifecycle, VerdictAdmit, "internal/run/run.go:128", "fork-birth (S02.5 step 2)"),
	minted("run.wedged", FamilyRunLifecycle, VerdictAdmit, "internal/run/run.go:131", "recovery-side pause-and-flag (S02.5); distinct from watchdog.flagged — B5-3 must not double-own wedged"),
	minted("run.harvest", FamilyRunLifecycle, VerdictAdmit, "internal/run/run.go:135", "finished-during-outage harvest append (S02.5)"),
	minted("intake.state", FamilyRunLifecycle, VerdictAdmit, "internal/intake/state.go:26", "the S06 intake-pipeline FSM state (pipeline lifecycle)"),
	minted("stage.started", FamilyRunLifecycle, VerdictConforms, "internal/stage/stageevents.go:137", "the S05.3 fresh-context stage-session boundary opening — one row per engine session across every role (ceremony leg, execute step, verify, compose, helper, split successor); payload {stage, kind, brief_hash}"),
	minted("stage.finished", FamilyRunLifecycle, VerdictConforms, "internal/stage/stageevents.go:142", "the same boundary's close; payload adds outcome ∈ {completed, split, error}. A session whose PROCESS dies gets no synthetic finish — the crash story is the run-lifecycle rows (S02.5)"),

	// ── Family 2: Checkpoint (D7) ───────────────────────────────────────
	minted("checkpoint.written", FamilyCheckpoint, VerdictRename, "internal/gates/checkpoints.go:26", "renamed from run.checkpoint; event carries refs, the S02.4(a)–(e) blocks live in the checkpoints row (in-substrate)"),

	// ── Family 3: Usage & limits (S2.5) ─────────────────────────────────
	minted("usage.recorded", FamilyUsageLimits, VerdictRename, "internal/adapters/adapters.go:65 (KindUsage)", "renamed from engine.usage; price-table version is added by S10 metering (B1-2) — field-addition seam, no S10 producer built here (OQ5)"),
	minted("limit.event", FamilyUsageLimits, VerdictRename, "internal/adapters/adapters.go:66 (KindRateLimit)", "renamed from engine.rate_limit; five-class limit-class + provider signal + resets-at are the S10 field seam (OQ5)"),
	minted("meter.utilization", FamilyUsageLimits, VerdictAdmit, "internal/metering/utilization.go:42", "S10.4 rate-limit-window utilization overlay (platform-scope, a limit-window reading)"),
	declare("run.parked", FamilyUsageLimits, "no distinct producer", "manifested as run.state_changed rows with cause=park (S02.3); S14.2 lists it here"),
	declare("run.resumed", FamilyUsageLimits, "no distinct producer", "manifested as run.state_changed rows with cause=resume (S02.3); S14.2 lists it here"),

	// ── Family 4: Gate / ask (4.2/4.3) ──────────────────────────────────
	minted("engine.gate_ask", FamilyGateAsk, VerdictAdmit, "internal/adapters/adapters.go:67 (KindGateAsk)", "the stored trace event carrying the durable ask-record input (S03.4); stays engine.gate_ask (OQ5)"),
	declare("ask.observed", FamilyGateAsk, "producer: B5-2/B5-8 (projected from asks)", "S14.2 projected canonical type over the asks table (OQ5)"),
	declare("ask.answered", FamilyGateAsk, "producer: B5-2/B5-8 (projected from asks)", "S14.2 projected canonical type over the asks table (OQ5)"),

	// ── Family 5: Human decision (S2.4) ─────────────────────────────────
	minted("intake.delta_decision", FamilyHumanDecision, VerdictAdmit, "internal/intake/state.go:31", "the delta-card human decision (S06.9 / P-T05-2); homes under decision.recorded's family"),
	minted("deliverable.accepted", FamilyHumanDecision, VerdictAdmit, "internal/review/review.go:227", "dual-home: the accept IS a human decision (primary) AND a deliverable-lifecycle marker (S13.6); registered once under Human decision"),
	minted("decision.recorded", FamilyHumanDecision, VerdictConforms,
		"internal/benchmark/benchmark.go (EventDecision), emitted internal/benchmark/gate.go DisposeAlarm and internal/benchmark/optin.go SetOptIn; CO-PRODUCER internal/api/approvals.go:1209 (EventDecisionRecorded), emitted internal/api/approvals.go:1254 recordDecision",
		"the S14.2 canonical generic human-decision type, MINTED at B5-7 (S14.7) by the two acts the benchmark practice needs: the BENCH-REG §12 alarm disposition and the §4.2.1 standing-opt-in consent flip. Payload is the family's contract minimum verbatim {actor, card_id + card_type, decision, presented_at → decided_at + latency_s}. B6-2A JOINED as the pre-authorized co-producer when the S15.6 approval-inbox verbs landed: it covers exactly the acts with no landed canonical audit row of their own — the effect-card approve and deny, whose journal transitions are effects-row updates rather than run_events appends (S02.7, CONVENTIONS §8). Acts that already mint a canonical row (intake.state / intake.delta_decision, the S07.7 verify events, watchdog.suppressed, the benchmark types, and run.state_changed which carries cancel) are NEVER double-minted. B6-2B extends the SAME co-production through the SAME mint site (recordDecision) to the five further acts with no landed canonical row: the S10.4 budget declaration and the pause-my-automation flip (card types `budget` / `automation_pause`, carrying the edit's old→new), the S15.5 board-drag hint (`priority_hint`), the S14.6 drift-card dismissal (`drift_card`) and the S14.5 conformance acknowledgement (`conformance_card`) — the last two also being READ BACK by their card derivations, which is what makes dismissal and acknowledgement derive-from-log facts rather than side-store columns (S14.1). Its suppress verb and its resume verb add nothing here: they ride watchdog.suppressed and run.state_changed respectively"),

	// ── Family 6: Verification verdict (S2.3) ───────────────────────────
	minted("verdict.recorded", FamilyVerificationVerdict, VerdictRename, "internal/verify/record.go:36", "renamed from verify.round; roundPayload fields conform exactly (round, rubric, judge, golden_set, findings)"),
	minted("verify.v0", FamilyVerificationVerdict, VerdictAdmit, "internal/verify/record.go:30", "V0 pre-gate execution (axis/evidence)"),
	minted("verify.v1", FamilyVerificationVerdict, VerdictAdmit, "internal/verify/record.go:33", "V1 check-pack execution"),
	minted("verify.escalation", FamilyVerificationVerdict, VerdictAdmit, "internal/verify/record.go:38", "routed escalation with ask id (Gate/ask cross-ref)"),

	// ── Family 7: Routing (S2.6/7.7) ────────────────────────────────────
	minted("routing.decided", FamilyRouting, VerdictConforms, "internal/worker/routing.go:51", "name + fields built to S14.2 (CONVENTIONS §19)"),
	minted("worker.compiled", FamilyRouting, VerdictAdmit, "internal/worker/routing.go:57", "the compiled-worker record used in routing (routing-adjacent)"),

	// ── Family 8: Knowledge injection (S2.9) ────────────────────────────
	minted("knowledge.injected", FamilyKnowledgeInjection, VerdictRename, "internal/ledger/assemble.go:32", "renamed from context.manifest; trace-manifest entries conform exactly, incl. post-compaction re-injection"),
	minted("ledger_update", FamilyKnowledgeInjection, VerdictAdmit, "internal/ledger/store.go:23", "the Task Context Ledger revision — S02.2-named verbatim (the one underscore, non-dotted type): a NAMING EXCEPTION kept because S02.2 binds it; its 8 verbs are payload sub-kinds, not types"),
	minted("knowledge.write", FamilyKnowledgeInjection, VerdictAdmit, "internal/memory/store.go:45", "S09 knowledge lifecycle; admitted by the knowledge. prefix"),
	minted("knowledge.remove", FamilyKnowledgeInjection, VerdictAdmit, "internal/memory/store.go:46"),
	minted("knowledge.delete", FamilyKnowledgeInjection, VerdictAdmit, "internal/memory/store.go:47"),
	minted("knowledge.conflict", FamilyKnowledgeInjection, VerdictAdmit, "internal/memory/store.go:48"),
	minted("knowledge.curation", FamilyKnowledgeInjection, VerdictAdmit, "internal/memory/store.go:49"),
	minted("knowledge.search_miss", FamilyKnowledgeInjection, VerdictAdmit, "internal/memory/store.go:50"),

	// ── Family 9: Orchestration (D6) ────────────────────────────────────
	minted("helper.spawned", FamilyOrchestration, VerdictRename, "internal/stage/helpers.go:78", "renamed from orchestration.spawn; spawn-record ref (trigger/reason/depth/budgets/brief_hash, S04.4)"),
	minted("spawn.refused", FamilyOrchestration, VerdictRename, "internal/stage/helpers.go:79", "renamed from orchestration.spawn_refused; the refusal names the failed check"),
	minted("orchestration.helper", FamilyOrchestration, VerdictAdmit, "internal/stage/helpers.go:80", "the helper report/result (a third Orchestration member)"),
	minted("intake.followup_spawned", FamilyOrchestration, VerdictAdmit, "internal/intake/followup.go:46", "a follow-up task spawned (S06/S13.6); spawn-class"),

	// ── Family 10: Watchdog (S2.7) — MINTED by B5-3 (S14.4) ─────────────
	minted("watchdog.flagged", FamilyWatchdog, VerdictConforms, "internal/watchdog/watchdog.go (EventFlagged); emitted internal/watchdog/tier2.go", "Tier-2 pause-and-flag: {rule, anomaly_class, severity, signature, counts, spend_vs_baseline, annotation_ref}; satisfies the FamilyWatchdog descriptor"),
	minted("watchdog.annotated", FamilyWatchdog, VerdictConforms, "internal/watchdog/watchdog.go (EventAnnotated); emitted internal/watchdog/tier1.go", "Tier-1 annotation: {rule, verdict∈loop|productive|unclear, confidence, evidence}; low margin ⇒ unclear (S12.4), never gates"),
	minted("watchdog.suppressed", FamilyWatchdog, VerdictConforms, "internal/watchdog/watchdog.go (EventSuppressed); emitted internal/watchdog/suppress.go", "per-rule suppression tuning signal; the retune proposal is a card, never an auto-move (S14.4)"),

	// ── Family 11: Drift & canary (S2.8) — BOTH halves minted: drift.finding
	// at B5-6A (the S14.6 watchlist executor) and canary.result at B5-6B (the
	// S14.6 ¶3 API canary layer). The A/B split was taken at exactly this
	// event-type seam. ───────────────────────────────────────────────────
	minted("drift.finding", FamilyDriftCanary, VerdictConforms,
		"internal/watchlist/watchlist.go (EventDriftFinding); emitted internal/watchlist/drift.go Emit",
		"outside-world drift from the S14.6 watchlist executor: {source, lanes, change_class, severity, summary, fingerprint} — the FamilyDriftCanary contract minimum verbatim; platform-scope (a watch hit has no run). Severity derives server-side from the change class and the WATCH ROW's lane (S14.4), never from the hit body"),
	minted("canary.result", FamilyDriftCanary, VerdictConforms,
		"internal/watchlist/canary.go (EventCanaryResult); emitted internal/watchlist/canary.go Canaries.record",
		"one API-canary run (S14.6 ¶3): {canary_kind ∈ auth|behavioral|logprob|model-list, lane, result ∈ pass|fail, delta} — the FamilyDriftCanary contract minimum's canary half verbatim; platform-scope (a canary run has no run). A FAILING canary additionally raises its card as a drift.finding, so cards keep deriving from one query (S14.6 ¶2); a canary NEVER kills, tombstones or gates a run (S14.4 / G1 D1.3)"),

	// ── Family 12: Platform (13.4; S2.1) ────────────────────────────────
	minted("settings.changed", FamilyPlatform, VerdictConforms, "internal/settings/settings.go:36", "spec-named (G3 Def.2); payload {actor,key,old,new,reason}"),
	minted("platform.started", FamilyPlatform, VerdictAdmit, "internal/shell/seams.go:68", "platform lifecycle (platform-scope, user_id=platform)"),
	minted("platform.stopping", FamilyPlatform, VerdictAdmit, "internal/shell/seams.go:69", "platform lifecycle (platform-scope, user_id=platform)"),
	minted("compaction.anomaly", FamilyPlatform, VerdictRename, "internal/stage/stage.go:121", "renamed from context.overflow (S05.7 compaction/overflow anomaly); FIELD-reconcile against the S14.2 compaction-anomaly minima at the B5-8 compaction packet"),
	minted("auth.login", FamilyPlatform, VerdictAdmit, "internal/auth/auth.go:76", "realizes S14.2 auth.event; the 9 auth.* admit by the auth. prefix (OQ2-(A))"),
	minted("auth.login_failed", FamilyPlatform, VerdictAdmit, "internal/auth/auth.go:77"),
	minted("auth.logout", FamilyPlatform, VerdictAdmit, "internal/auth/auth.go:78"),
	minted("auth.pin_set", FamilyPlatform, VerdictAdmit, "internal/auth/auth.go:79"),
	minted("auth.grant", FamilyPlatform, VerdictAdmit, "internal/auth/auth.go:80"),
	minted("auth.grant_revoked", FamilyPlatform, VerdictAdmit, "internal/auth/auth.go:81"),
	minted("auth.reprompt", FamilyPlatform, VerdictAdmit, "internal/auth/auth.go:82"),
	minted("auth.reprompt_failed", FamilyPlatform, VerdictAdmit, "internal/auth/auth.go:83"),
	minted("auth.user_created", FamilyPlatform, VerdictAdmit, "internal/auth/auth.go:84"),
	minted("platform.snapshot", FamilyPlatform, VerdictAdmit, "internal/backup/backup.go:63", "platform-health/backup (S13.9/S02.9); platform. prefix, asset-lifecycle scope (OQ2-(A))"),
	minted("platform.restore_drill", FamilyPlatform, VerdictAdmit, "internal/backup/backup.go:65"),
	minted("platform.restore_drill_flag", FamilyPlatform, VerdictAdmit, "internal/backup/backup.go:68"),
	minted("local.unmetered_defect", FamilyPlatform, VerdictAdmit, "internal/local/duty.go:27", "S12 local-tier op (platform-scope, OQ2-(A))"),
	minted("local.admissions_stopped", FamilyPlatform, VerdictAdmit, "internal/local/duty.go:30"),
	minted("local.admissions_resumed", FamilyPlatform, VerdictAdmit, "internal/local/duty.go:31"),
	minted("local.alias_retargeted", FamilyPlatform, VerdictAdmit, "internal/local/swap.go:175", "duty-alias swap (S12.10); a swap fires the S14.8 revalidation runbook — tie noted for B5-7"),
	minted("registry.registered", FamilyPlatform, VerdictAdmit, "internal/project/project.go:184", "S13.5 project-registry lifecycle (asset-lifecycle scope, OQ2-(A))"),
	minted("registry.captured", FamilyPlatform, VerdictAdmit, "internal/project/project.go:186"),
	minted("registry.activated", FamilyPlatform, VerdictAdmit, "internal/project/project.go:188"),
	minted("registry.drift", FamilyPlatform, VerdictAdmit, "internal/project/project.go:191", "project-topology drift (S13.7), NOT outside-world drift — B5-6 Drift&canary must not mis-own it"),
	minted("preview.started", FamilyPlatform, VerdictAdmit, "internal/preview/preview.go:224", "S13.8 preview-service lifecycle (asset-lifecycle scope, OQ2-(A))"),
	minted("preview.stopped", FamilyPlatform, VerdictAdmit, "internal/preview/preview.go:226"),
	// S08 worker-asset lifecycle (13) — the largest un-homed group; ADMIT
	// under the broadened Platform scope per OQ2-(A) (no Spec edit).
	minted("worker.template_created", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:43", "S08 worker-asset lifecycle (asset-lifecycle scope, OQ2-(A))"),
	minted("worker.version_created", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:44"),
	minted("worker.validated", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:45"),
	minted("worker.approved", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:46"),
	minted("worker.promoted", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:47"),
	minted("worker.repointed", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:48"),
	minted("worker.flagged", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:49"),
	minted("worker.retired", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:50"),
	minted("worker.review", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:51"),
	minted("worker.graduated", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:52"),
	minted("worker.gap", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:53"),
	minted("worker.domain_maturity", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:54"),
	minted("worker.automation_run", FamilyPlatform, VerdictAdmit, "internal/worker/store.go:55"),
	minted("retention.compacted", FamilyPlatform, VerdictConforms,
		"internal/retention/retention.go (EventCompacted); emitted internal/retention/compact.go Compact",
		"the S14.9 ¶2 compaction pass logging ITSELF — \"the audit trail records its own compaction\". Platform-scope, ONE row per pass, with per-owner legs in the payload {as_of, owners:[{user_id, horizon_months, boundary_ts, boundary_event_seq, events_stripped, bytes_reclaimed, by_family, transcripts_stripped}]} — the operator sees the whole pass and no member's audit view gains another member's counts. The horizon is per-user (⚙ retention.compaction_horizon, read with IntFor), so one pass legitimately carries several boundaries. The pass elides payload BODIES only: 0015 narrowed the append-only UPDATE trigger to that one transition and left run_events_no_delete untouched"),
	minted("history.query_audited", FamilyPlatform, VerdictAdmit,
		"internal/history/layer2.go (EventQueryAudited); emitted internal/history/layer2.go AskOpenSQL",
		"S14.10 ¶3's \"every generated query audit-logged\" — the Layer-2 open-SQL escalation surface logging ITSELF, one row per ATTEMPT including every refusal (a guardrail whose refusals are not recorded cannot be reviewed, and the refusals are the interesting rows). Payload {question, outcome (executed|refused|no_sql|unavailable|failed), sql_generated, sql_executed, views, limit_injected, timeout_ms, refusal, row_count, model, alias, as_operator, owner_scoped}. Scope is the ASKER — user_id is the member, or 'platform' for the operator — so a member's audit view never gains another member's questions. Admitted under the broadened Platform scope by the same OQ2-(A) reading that admits the worker./registry./preview./local. groups: it is a platform-operational audit of a surface, not a run event"),

	// ── Family 13: Tools & artifacts (S2.1) ─────────────────────────────
	minted("tool.completed", FamilyToolsArtifacts, VerdictRename, "internal/adapters/adapters.go:68 (KindToolResult)", "renamed from engine.tool_result; a SECOND emitter is the B5-3 dead-man canary, which injects synthetic tool.completed rows on its platform.deadman.* run to exercise Tier-0 detection end-to-end (internal/watchdog/deadman.go injectLoopTrace) — same type, synthetic provenance"),
	minted("engine.message", FamilyToolsArtifacts, VerdictAdmit, "internal/adapters/adapters.go:64 (KindMessage)", "raw assistant/user message trace step; the softest engine.* mapping — ADMIT as a trace step"),
	minted("engine.done", FamilyToolsArtifacts, VerdictAdmit, "internal/adapters/adapters.go:69 (KindDone)", "the terminal envelope (run-total usage rides it); ADMIT as a trace/completion step"),
	minted("artifact.produced", FamilyToolsArtifacts, VerdictRename, "internal/review/review.go:216", "renamed from deliverable.minted (a deliverable is a produced artifact, S13); artifact ref + hash"),
	minted("review.comment", FamilyToolsArtifacts, VerdictAdmit, "internal/review/review.go:219", "review-of-artifact lifecycle (S13)"),
	minted("review.drained", FamilyToolsArtifacts, VerdictAdmit, "internal/review/review.go:221", "review-drain event (S13)"),
	declare("tool.called", FamilyToolsArtifacts, "no distinct producer today", "S14.2 names tool.called/completed; the engine surfaces tool_result → tool.completed; a tool-use-start producer is a later addition"),

	// ── Family 14: Benchmark & eval (S2.11) — FULLY MINTED at B5-7.
	// eval.score_recorded was minted at B5-4 (the S14.5 conformance-registry
	// recording surface) and co-produced by B5-5 (the S14.8 regression evals);
	// benchmark.pair_recorded and benchmark.alarm are B5-7's (the S14.7
	// benchmark practice around BENCH-REG). The registration's numbers remain
	// READ-ONLY to the platform: they are marked data in internal/benchmark
	// carrying their §-refs, and they change only via BENCH-REG §17. ───
	minted("benchmark.pair_recorded", FamilyBenchmarkEval, VerdictConforms,
		"internal/benchmark/benchmark.go (EventPairRecorded); emitted internal/benchmark/pair.go record",
		"the BENCH-REG §14 record VERBATIM in its minimums: pair id · domain · task class · timestamps · both arms' exact observed model identities · both arms' measured consumption · refs to both rendered-blind artifacts · position assignment · verdict · requester's platform-guess · decline flag · epoch id · requester id. Owner-attributed to the REQUESTER (the pair is theirs, 15.6). Keep-forever (Spec S14.9); the class is marked on the payload, enforcement is B5-8's. The record and the pair row's move to `recorded` commit in ONE WriteTx, and arm identity is revealed only by reading this committed record (BENCH-REG §3.4)"),
	minted("benchmark.alarm", FamilyBenchmarkEval, VerdictAdmit,
		"internal/benchmark/benchmark.go (EventAlarm); emitted internal/benchmark/gate.go evaluateAlarmTx (raise) and DisposeAlarm (clear)",
		"the BENCH-REG §12 alarm: raised at any per-pair update when 1−G passes the registered threshold in a launch domain's current epoch, cleared only by an operator disposition. Platform-scope. Alarm history is keep-forever, which is why it is an EVENT and not a table: raise/clear payloads carry the full record and the standing expansion freeze, and the card DERIVES from them (the drift.finding precedent). It NEVER kills, parks or gates a run (§12; G1 D1.3) — the freeze binds by visibility and by gate limb (c)"),
	minted("eval.score_recorded", FamilyBenchmarkEval, VerdictConforms, "internal/conformance/conformance.go (EventScoreRecorded); emitted internal/conformance RecordResult, driven by internal/evals (S14.8)", "the S14.5 conformance-registry recording surface (B5-4), co-produced by the S14.8 regression evals through the same verb (B5-5). B5-7 added NO new producer: S14.7's v0 scope is pair-shaped, not suite-shaped — gate limb (d) READS the existing floor/sweep records through a shell seam, and the standing questions are registrations. The family surface is satisfied by benchmark.pair_recorded for pairs and by the B5-4/B5-5 producers for suite results"),

	// ── Family 15: Run summary (11.1) — FULLY MINTED at B5-8A ───────────
	minted("run.summary_written", FamilyRunSummary, VerdictConforms,
		"internal/retention/retention.go (EventSummaryWritten); emitted internal/retention/summary.go WriteAtRunEndTx",
		"the S14.9 ¶1 run summary, appended at the run's TERMINAL transition in the SAME transaction as the state change (S02.3; composed onto run.Store's terminal hook at the shell root, so all ten terminal call sites are covered by one edge). Payload is the family's contract minimum verbatim {summary_ref, inputs_digest} plus {final_state, aggregate_only}; the structured story itself lives in the run_summaries row the ref names (migration 0015), because the summary must SURVIVE the compaction that strips the trace it was computed from. The deterministic aggregate is the record and is frozen by trigger; the local tier enriches it afterwards and never gates — a platform-scope run mints no summary (no objective, no receipts, and the narrator's own advisory run would not terminate)"),
}

// v0Aliases maps each renamed type's OLD name to its family so a pre-existing
// dev/test run_events row still classifies (R14; Spec S14.2 rule 3, S02.2
// append-only). This is upcast-on-read applied to type names: new writes use
// the canonical name, reads recognize both. No run_events migration is needed
// (no production rows exist).
var v0Aliases = map[string]Family{
	"run.state":                   FamilyRunLifecycle,        // → run.state_changed
	"run.checkpoint":              FamilyCheckpoint,          // → checkpoint.written
	"engine.usage":                FamilyUsageLimits,         // → usage.recorded
	"engine.rate_limit":           FamilyUsageLimits,         // → limit.event
	"verify.round":                FamilyVerificationVerdict, // → verdict.recorded
	"context.manifest":            FamilyKnowledgeInjection,  // → knowledge.injected
	"orchestration.spawn":         FamilyOrchestration,       // → helper.spawned
	"orchestration.spawn_refused": FamilyOrchestration,       // → spawn.refused
	"context.overflow":            FamilyPlatform,            // → compaction.anomaly
	"engine.tool_result":          FamilyToolsArtifacts,      // → tool.completed
	"deliverable.minted":          FamilyToolsArtifacts,      // → artifact.produced
}
