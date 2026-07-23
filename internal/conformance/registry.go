package conformance

import "strings"

// platformUser is the owning user_id for every registry row and every
// eval.score_recorded event: the registry is platform state (15.6; Spec S02.2
// "every row carries its owning user_id").
const platformUser = "platform"

// bumpGateNote records the S03.3 bump-gating tie as ROW DATA on every
// engine_bump-triggered row (R19; S14.5 closing ¶): a bump lands only after
// (a) the candidate passes its per-lane conformance suite AND (b) the S14.8
// before/after quality probe shows no regression (P-T02-5) — limb (b) is
// B5-5's, referenced here as data. Querying the engine_bump rows
// (BumpGatingRows) surfaces both limbs; this packet builds no bump executor and
// no quality-probe machinery.
const bumpGateNote = " S03.3 bump-gating tie (engine_bump row): a bump lands only after (a) the candidate passes its per-lane conformance suite AND (b) the S14.8 before/after quality probe (B5-5) shows no regression (P-T02-5)."

// last_result vocabulary.
const (
	ResultGreen   = "green"
	ResultRed     = "red"
	ResultUnknown = "unknown"
)

// affect_class vocabulary: the R14 predicate — a red result is flag-now exactly
// when the row is lane- or storage-affecting; 'none' is an ordinary card.
const (
	AffectLane    = "lane"
	AffectStorage = "storage"
	AffectNone    = "none"
)

// derive_kind vocabulary: ” = a RECORDED row (RecordResult writes its result;
// a red result fires a conformance card). A non-empty value = a DERIVE row
// whose last run/result come from existing events, registered for visibility
// only (its own subsystem owns the alert).
const (
	DeriveNone         = ""
	DeriveRestoreDrill = "restore_drill"
	DeriveDeadMan      = "dead_man"
)

// Structural cadence tokens (OQ6(a): quarterly = 3 months, weekly = 7 days —
// structural row seed data per the §31 posture, not new settings) and the
// dead-man daily probe (OQ5(c)/§31: 24h structural constant).
const (
	CadenceQuarterly = "quarterly"
	CadenceWeekly    = "weekly"
	CadenceDaily     = "daily"
	// A settings-backed cadence is "setting:<dotted-key>:<unit>" — the ⚙ value
	// is read live at evaluation time (never a hardcoded constant).
	cadenceSettingPrefix = "setting:"
)

// Trigger tokens (R18): the machine trigger kinds, covering the six spec rows'
// trigger text. Naming is P3 discretion.
const (
	TriggerEngineBump           = "engine_bump"
	TriggerStorageComponentBump = "storage_component_bump"
	TriggerWeekly               = "weekly"
	TriggerQuarterly            = "quarterly"
	TriggerWithEscalationDrill  = "with_escalation_drill"
	TriggerPerS13Schedule       = "per_s13_schedule"
	TriggerDaily                = "daily"
)

// The ⚙ dotted keys this package reads by dotted key (CONVENTIONS §2; §5 of the
// brief). No new key is declared — the settings tally stays 118/33.
const (
	keyBackupDrillInterval = "backup.drill_interval"              // months (S13); restore-drill dueness (OQ2(a))
	keyVerifyDrillDays     = "verification.drill_interval_days"   // days (S07); forced-escalation dueness (OQ4(a))
	keyVerifyCanaryHours   = "verification.canary_interval_hours" // hours (S07); the visible-only escalation-canary schedule (OQ4(a))
	// keyEvalSweepInterval is the S14.8 regression-sweep knob. It is
	// REFERENCED-NOT-CONSUMED here (OQ6(a)): it belongs to B5-5's revalidation
	// sweep, not registry dueness — the bare quarterly/weekly literals are
	// structural row data instead. Named for documentation; never read.
	keyEvalSweepInterval = "eval.sweep_interval"
)

// settingCadence builds a settings-backed cadence token from a dotted key and
// its unit.
func settingCadence(key, unit string) string {
	return cadenceSettingPrefix + key + ":" + unit
}

// Fixture is one named in-tree run handle as DATA (the F17 one-named-run-handle
// precedent): a package path + test name (source-scan resolvable), or a
// repo-relative file (for a non-test probe procedure). The Handle is the
// human-readable run handle stored in the fixtures column; Pkg/Run and File are
// the machine-checkable references the fixtures-resolve test scans (rubric 4).
type Fixture struct {
	// Handle is the display run handle (stored, composed into the DB column).
	Handle string
	// Pkg is a repo-relative package dir whose _test.go files must contain a
	// Go test function named Run; "" for a File-based fixture.
	Pkg string
	Run string
	// File is a repo-relative file that must exist (a probe procedure that is
	// not a Go test, e.g. the compaction row's spike measurement); "" for a
	// Pkg/Run fixture.
	File string
}

// Row is one seed registry entry (the static, immutable structure of a
// conformance_registry row; last_run/last_result are runtime state).
type Row struct {
	ID            string
	OwningSection string
	Fixtures      []Fixture
	TriggerSet    []string
	Schedule      string
	Cadence       string
	// CanaryCadence is an OPTIONAL secondary ⚙ key registered as VISIBLE DATA
	// only (read for display, never dueness) — the forced-escalation row's
	// daily escalation-canary schedule (OQ4(a)); "" for every other row.
	CanaryCadence string
	AffectClass   string
	DeriveKind    string
	Notes         string
}

// fixturesText composes the DB fixtures column from a row's fixtures: one run
// handle per line.
func fixturesText(fs []Fixture) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = f.Handle
	}
	return strings.Join(parts, "\n")
}

// SeedRows returns the S14.5 registry seed: the six spec row-groups (adapter
// per-lane split into the lanes whose suites EXIST — anthropic + local, R7),
// the no-engine-SSE-replay standing assertion as its own row (CF2), and the
// dead-man canary drill (OQ5(c)). Every fixture names a REAL in-tree
// test/procedure; every schedule and declared-by matches the S14.5 table
// verbatim for the six spec rows.
func SeedRows() []Row {
	return []Row{
		{
			// kill-9 crash harness + suspend-cycle test [S02.9].
			ID:            "kill9-suspend-cycle",
			OwningSection: "S02.9",
			Fixtures: []Fixture{
				{Handle: "go test ./internal/recovery/ -run TestKill9MidTransitionInvisible (kill-9 harness; self-re-exec via SINET_CRASH_SCENARIO)", Pkg: "internal/recovery", Run: "TestKill9MidTransitionInvisible"},
				{Handle: "go test ./internal/recovery/ -run TestKill9EffectCrashWindow", Pkg: "internal/recovery", Run: "TestKill9EffectCrashWindow"},
				{Handle: "go test ./internal/recovery/ -run TestSuspendCycleWakeGrace (suspend-cycle: fake PrepareForSleep + clock deltas)", Pkg: "internal/recovery", Run: "TestSuspendCycleWakeGrace"},
			},
			TriggerSet:  []string{TriggerStorageComponentBump, TriggerQuarterly},
			Schedule:    "before any storage-touching component bump; quarterly sweep",
			Cadence:     CadenceQuarterly,
			AffectClass: AffectStorage,
			Notes:       "application invariants post-crash + the suspend-cycle companion (CONVENTIONS §8). Storage-affecting ⇒ a red result routes flag-now (R14).",
		},
		{
			// Adapter per-lane — Anthropic [S03.1/S03.5].
			ID:            "adapter-anthropic",
			OwningSection: "S03.1/S03.5",
			Fixtures: []Fixture{
				{Handle: "go test ./internal/adapters/claudecli/ -run TestParseHappyFixture (tier F: D3 verb behavior + stream schema and VALUES on fixtures; always run, CI has no engine)", Pkg: "internal/adapters/claudecli", Run: "TestParseHappyFixture"},
				{Handle: "go test ./internal/adapters/claudecli/ -run TestResumeLoweringFullResupply (park/resume mechanics)", Pkg: "internal/adapters/claudecli", Run: "TestResumeLoweringFullResupply"},
				{Handle: "go test ./internal/adapters/claudecli/ -run TestLoweringRefusesNativeSpawnTools (native-spawn-disabled probe + engine-lowering leak)", Pkg: "internal/adapters/claudecli", Run: "TestLoweringRefusesNativeSpawnTools"},
				{Handle: "go test ./internal/adapters/claudecli/ -run TestLoweringEnvScrub (per-channel leak: attempt-the-leak, assert the decoy did NOT take effect)", Pkg: "internal/adapters/claudecli", Run: "TestLoweringEnvScrub"},
				{Handle: "go test ./internal/adapters/claudecli/ -run TestPinMatchesLock (pin↔components.lock coupling)", Pkg: "internal/adapters/claudecli", Run: "TestPinMatchesLock"},
				{Handle: "go test ./internal/adapters/claudecli/ -run TestRealEngineVersionAgainstPin (tier R: auto when the engine binary is installed; SANCTIONED SKIP otherwise)", Pkg: "internal/adapters/claudecli", Run: "TestRealEngineVersionAgainstPin"},
				{Handle: "SINET_LIVE_SMOKE=1 go test ./internal/adapters/claudecli/ -run TestLiveSmoke (tier L: one minimal paid call)", Pkg: "internal/adapters/claudecli", Run: "TestLiveSmoke"},
			},
			TriggerSet:  []string{TriggerEngineBump, TriggerWeekly},
			Schedule:    "on any engine/CLI version change + weekly",
			Cadence:     CadenceWeekly,
			AffectClass: AffectLane,
			Notes:       "the Anthropic lane (claudecli battery). The weekly RUN of this suite is the S14.6 canary layer's (B5-6); this row is the scheduling home. Lane-affecting ⇒ flag-now." + bumpGateNote,
		},
		{
			// Adapter per-lane — local [S03.2/S12.10]. The zai lane is OMITTED.
			ID:            "adapter-local",
			OwningSection: "S03.2/S12.10",
			Fixtures: []Fixture{
				{Handle: "go test ./internal/local/ -run TestBumpProcedurePinsMatchLock (pin↔lock coupling)", Pkg: "internal/local", Run: "TestBumpProcedurePinsMatchLock"},
				{Handle: "go test ./internal/local/ -run TestBumpProcedureTierR (real llama-swap /v1 when installed; SANCTIONED SKIP otherwise)", Pkg: "internal/local", Run: "TestBumpProcedureTierR"},
				{Handle: "SINET_LIVE_SMOKE=1 go test ./internal/local/ -run TestBumpProcedureTierLSmoke (tier L: json_schema + logprobs + unload + reason-first order)", Pkg: "internal/local", Run: "TestBumpProcedureTierLSmoke"},
			},
			TriggerSet:  []string{TriggerEngineBump, TriggerWeekly},
			Schedule:    "on any engine/CLI version change + weekly",
			Cadence:     CadenceWeekly,
			AffectClass: AffectLane,
			Notes:       "the local lane (the S12.10 deliberate-bump battery, the F17 one-named-run-handle TestBumpProcedure*). The zai lane is OMITTED and has NO row: internal/adapters holds only claudecli and S12.1 class (a) has no v0 consumer — a row claiming a nonexistent suite would fake (R7; the honest-dormancy discipline). Its row lands with its adapter. Lane-affecting ⇒ flag-now." + bumpGateNote,
		},
		{
			// The no-engine-SSE-replay standing assertion [S14.3 ¶3; S14.5] — a
			// named bullet of the adapter row whose test lives in internal/api,
			// so it carries its own row (CF2, executor discretion).
			ID:            "no-engine-sse-replay",
			OwningSection: "S14.3/S14.5",
			Fixtures: []Fixture{
				{Handle: "go test ./internal/api/ -run TestConformance_NoEngineSSEReplay_ResumeFromLogAlone (/events resume reconstructs the exact sequence from run_events ALONE — no engine/adapter in the loop)", Pkg: "internal/api", Run: "TestConformance_NoEngineSSEReplay_ResumeFromLogAlone"},
			},
			TriggerSet:  []string{TriggerEngineBump, TriggerWeekly},
			Schedule:    "with the adapter per-lane suites (on any engine/CLI version change + weekly)",
			Cadence:     CadenceWeekly,
			AffectClass: AffectNone,
			Notes:       "the S14.3 ¶3 no-reliance-on-engine-SSE-replay assertion (opencode ignores Last-Event-ID, fix declined upstream). The cross-lane log-reconstruction guarantee is an inspection-surface invariant (not a per-lane freeze or storage-corruption class) ⇒ an ordinary approval-class card." + bumpGateNote,
		},
		{
			// D6 violation attempts [S04.5].
			ID:            "d6-violation-attempts",
			OwningSection: "S04.5",
			Fixtures: []Fixture{
				{Handle: "go test ./internal/stage/ -run TestHelperDepth3RecursionRefused (depth-3 recursion refused)", Pkg: "internal/stage", Run: "TestHelperDepth3RecursionRefused"},
				{Handle: "go test ./internal/stage/ -run TestHelperRefusalsAreLoggedEvidence (native-or-lateral tool refusal, logged evidence)", Pkg: "internal/stage", Run: "TestHelperRefusalsAreLoggedEvidence"},
				{Handle: "go test ./internal/stage/ -run TestHelperSpawnBudgetExhaustionRefuses (over-budget spawn refused)", Pkg: "internal/stage", Run: "TestHelperSpawnBudgetExhaustionRefuses"},
				{Handle: "go test ./internal/stage/ -run TestHelperKillSalvagesAndSiblingsContained (mid-run kill → salvage-with-marker, sibling containment)", Pkg: "internal/stage", Run: "TestHelperKillSalvagesAndSiblingsContained"},
				{Handle: "go test ./internal/adapters/claudecli/ -run TestLoweringRefusesNativeSpawnTools (per-lane engine-native spawn refusal)", Pkg: "internal/adapters/claudecli", Run: "TestLoweringRefusesNativeSpawnTools"},
			},
			TriggerSet:  []string{TriggerQuarterly, TriggerEngineBump},
			Schedule:    "quarterly + on engine bump",
			Cadence:     CadenceQuarterly,
			AffectClass: AffectLane,
			Notes:       "the S04.5 D6 standing battery (CONVENTIONS §19): refusal asserted for depth-3/lateral/native-spawn/over-budget, containment-with-salvage for mid-run kill. Lane-affecting ⇒ flag-now." + bumpGateNote,
		},
		{
			// Compaction canary [S05.7] — OQ1(a).
			ID:            "compaction-canary",
			OwningSection: "S05.7",
			Fixtures: []Fixture{
				{Handle: "re-run the B1-4 pre-compact injection-mechanics probe (PreCompact veto + SessionStart source:\"compact\" re-injection) per engine bump; result ingested via RecordResult", File: "P3/measurements/2026-07-20-precompact-injection-mechanics.md"},
				{Handle: "go test ./internal/adapters/claudecli/ -run TestRunSessionStartHookEmitsPinnedContextVerbatim (pinned-survival re-injection fixture, P-T04-1)", Pkg: "internal/adapters/claudecli", Run: "TestRunSessionStartHookEmitsPinnedContextVerbatim"},
				{Handle: "SINET_B3_4=1 go test ./internal/adapters/claudecli/ -run TestLivePostToolUseCanary (per-pin live canary)", Pkg: "internal/adapters/claudecli", Run: "TestLivePostToolUseCanary"},
			},
			TriggerSet:  []string{TriggerEngineBump, TriggerQuarterly},
			Schedule:    "on engine bump + quarterly",
			Cadence:     CadenceQuarterly,
			AffectClass: AffectLane,
			Notes:       "OQ1(a): NO standing canary-constraint→compact→adherence suite exists in-tree — the fixture is the B1-4 spike-method probe (the re-runnable reference) plus the nearest standing behavior checks (the SessionStart re-injection fixture + the SINET_B3_4 per-pin canary). The missing standing suite is FLAGGED to the B5 gate as an S05.7/adapter follow-up, not silently absorbed. Lane-affecting ⇒ flag-now." + bumpGateNote,
		},
		{
			// Forced-escalation end-to-end [S06/S04/S07] — OQ4(a).
			ID:            "forced-escalation-e2e",
			OwningSection: "S06/S04/S07",
			Fixtures: []Fixture{
				{Handle: "go test ./internal/intake/ -run TestCoverageEscalationForced (S06.7 intake coverage-check card, forced e2e)", Pkg: "internal/intake", Run: "TestCoverageEscalationForced"},
				{Handle: "go test ./internal/intake/ -run TestSpecDoubtForced (S06.8 plan self-attack spec-is-bad)", Pkg: "internal/intake", Run: "TestSpecDoubtForced"},
				{Handle: "go test ./internal/stage/ -run TestHelperEscalationOpensDurableAsk (S04.5 helper ESCALATED path)", Pkg: "internal/stage", Run: "TestHelperEscalationOpensDurableAsk"},
				{Handle: "go test ./internal/verify/ -run TestPlantedACBlockerEscalates (S07 verification escalation, one planted defect per route category)", Pkg: "internal/verify", Run: "TestPlantedACBlockerEscalates"},
				{Handle: "go test ./internal/verify/ -run TestRouteTableTotalAndSinkTypesOnly (the escalation route table is total over the 8 categories)", Pkg: "internal/verify", Run: "TestRouteTableTotalAndSinkTypesOnly"},
			},
			TriggerSet:    []string{TriggerQuarterly, TriggerWithEscalationDrill},
			Schedule:      "quarterly, with the full escalation drill (G1 Def.3 superseded set)",
			Cadence:       settingCadence(keyVerifyDrillDays, "days"),
			CanaryCadence: keyVerifyCanaryHours,
			AffectClass:   AffectNone,
			Notes:         "OQ4(a): dueness reads ⚙ verification.drill_interval_days (the full escalation drill cadence); the daily escalation-canary schedule registers as VISIBLE DATA (⚙ verification.canary_interval_hours). The canary RUNNER and the CanaryWatch.Silent→operator-alert wiring stay in B5-6/B6; internal/verify logic is untouched. A finding that dies in a log is a platform defect (5.6). Neither lane- nor storage-affecting ⇒ an ordinary approval-class card.",
		},
		{
			// Verified-restore drill [S02.9/S13] — OQ2(a), DERIVE.
			ID:            "verified-restore-drill",
			OwningSection: "S02.9/S13",
			Fixtures: []Fixture{
				{Handle: "the RestoreDrill production procedure (internal/backup/drill.go), timer-driven per ⚙ backup.drill_interval; exercised hermetically by go test ./internal/backup/ -run TestSnapshotAndDrillRoundTrip", Pkg: "internal/backup", Run: "TestSnapshotAndDrillRoundTrip"},
				{Handle: "go test ./internal/backup/ -run TestDrillFailsClosedOnSwappedBlob (ledger-mismatch fail-closed + High flag)", Pkg: "internal/backup", Run: "TestDrillFailsClosedOnSwappedBlob"},
			},
			TriggerSet:  []string{TriggerPerS13Schedule},
			Schedule:    "per S13's schedule",
			Cadence:     settingCadence(keyBackupDrillInterval, "months"),
			AffectClass: AffectStorage,
			DeriveKind:  DeriveRestoreDrill,
			Notes:       "OQ2(a): registered here for visibility; pipeline and drill content owned by S13. last_run/last_result DERIVE from the existing platform.restore_drill events (internal/backup untouched, no re-emission — a fail-closed drill already records platform.restore_drill either way, plus a High platform.restore_drill_flag on ledger mismatch). Storage-affecting; the backup's own High flag is the alert, so this DERIVE row mirrors and never double-fires a conformance card.",
		},
		{
			// Dead-man canary drill [S14.4] — OQ5(c), DERIVE.
			ID:            "dead-man-canary",
			OwningSection: "S14.4",
			Fixtures: []Fixture{
				{Handle: "the dead-man canary probe (internal/watchdog/deadman.go DeadManProbe, R28-DMC): a daily $0 in-tree probe driving the full Tier-0→Tier-1→flag path on a synthetic platform.deadman.* run + a $0 local liveness call; exercised by go test ./internal/watchdog/ -run TestDeadManDuenessFromLog", Pkg: "internal/watchdog", Run: "TestDeadManDuenessFromLog"},
				{Handle: "go test ./internal/watchdog/ -run TestDeadManAlarmWhenLocalDark (the alarm path fires when the watchdog is dark)", Pkg: "internal/watchdog", Run: "TestDeadManAlarmWhenLocalDark"},
			},
			TriggerSet:  []string{TriggerDaily},
			Schedule:    "daily",
			Cadence:     CadenceDaily,
			AffectClass: AffectNone,
			DeriveKind:  DeriveDeadMan,
			Notes:       "OQ5(c): the watchdog's own liveness probe is a genuine scheduled drill (the same G1 Def.3 superseded set the escalation row cites), so it earns a registry row — last-run DERIVED from the deadman events (the latest platform.deadman.* birth), NO import edge to internal/watchdog. The four watchdog platform-health checks (resume-reconcile/listener/organ-absence/event-log-size) are continuous DETECTION, S14.4-scheduled, and get NO rows (not fixture-backed proof suites). A red dead-man already raises the watchdog's own flag-now watchdog.dead_man card (§31), so this DERIVE row is dueness/visibility only, never a double-fire.",
		},
	}
}
