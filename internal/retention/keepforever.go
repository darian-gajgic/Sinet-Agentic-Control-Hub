package retention

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// keepforever.go — the S14.9 ¶2 keep-forever set.
//
// S14.9 names SEVEN classes: "run summaries, verdicts, decisions, receipts,
// routing records, drift events, benchmark records [G2 Def.11]". They are
// mapped onto the S14.2 FAMILY registry rather than a hand-written list of type
// strings, because the families are what S14.2 makes binding — a family admits
// a type by scope, so a type minted by a later packet lands on the correct side
// of the line without anyone remembering to add it.
//
// The mapping below is TOTAL over the fifteen families: every family carries an
// explicit decision and its reason. A new family (an S00.9 amendment to frozen
// S14.2) therefore FAILS the coverage test until someone decides, which is the
// point — the default is never silently "strip".

// keepForeverFamilies is the decision, one row per S14.2 family. The `keep`
// column is the S14.9 ¶2 answer; `why` records the reason it is that answer.
var keepForeverFamilies = []struct {
	family eventlog.Family
	keep   bool
	why    string
}{
	// ── The seven S14.9 keep-forever classes ────────────────────────────────
	{eventlog.FamilyRunSummary, true,
		"run summaries — S14.9 ¶2; the summary is precisely what remains of a run once its trace is stripped"},
	{eventlog.FamilyVerificationVerdict, true,
		"verdicts — S14.9 ¶2; the S2.3 per-round verdict record is the quality history"},
	{eventlog.FamilyHumanDecision, true,
		"decisions — S14.9 ¶2; a human decision (S2.4) is never re-derivable from anything else"},
	{eventlog.FamilyUsageLimits, true,
		"receipts — S14.9 ¶2; this family IS the receipt substrate (per-paid-call usage, limit events, meter utilization) and S10.10 makes cost observable at every altitude, which a stripped payload would end at the horizon"},
	{eventlog.FamilyRouting, true,
		"routing records — S14.9 ¶2; S2.6 requires routing be auditable over time, which is a claim about old rows"},
	{eventlog.FamilyDriftCanary, true,
		"drift events — S14.9 ¶2; S2.8 drift and canary history is the outside-world record"},
	{eventlog.FamilyBenchmarkEval, true,
		"benchmark records — S14.9 ¶2; BENCH-REG §14 records are keep-forever and §17 forbids retroactive recomputation (the class is pre-marked in internal/benchmark; this is its enforcement)"},

	// ── The eight families S14.9's list does NOT name ───────────────────────
	// These are the trace. S14.1 scopes full-trace reconstructability to "the
	// whole retention period (S14.9)"; past the horizon the keep-forever set is
	// what remains, and the run summary is why that is enough.
	{eventlog.FamilyRunLifecycle, false,
		"trace: FSM transitions. Final state survives on the runs row and the run summary carries the story; the ROW (type, ts, seq) always survives the strip"},
	{eventlog.FamilyCheckpoint, false,
		"trace: the checkpoint EVENT carries refs only — the S02.4 (a)–(e) blocks live in the checkpoints table, which compaction never touches"},
	{eventlog.FamilyGateAsk, false,
		"trace: the durable ask record lives in the asks table (S02.2 in-substrate), which compaction never touches"},
	{eventlog.FamilyKnowledgeInjection, false,
		"trace: injection manifests are refs into knowledge_entries, which compaction never touches"},
	{eventlog.FamilyOrchestration, false,
		"trace: helper spawn/lifecycle records, the bulkiest sub-run trace"},
	{eventlog.FamilyWatchdog, false,
		"trace: watchdog flags are live-health signals, resolved long before the horizon; open-flag derivation reads recent rows"},
	{eventlog.FamilyPlatform, false,
		"trace: platform-scope operational + asset-lifecycle events. This includes retention.compacted itself — the pass's own ROW always survives (deletes are aborted), so the audit trail still records that a compaction happened at that seq; only a months-old pass's counts are elided. S14.9's list is closed and Platform is not on it"},
	{eventlog.FamilyToolsArtifacts, false,
		"trace: tool calls, engine messages and artifact refs — the bulky event payloads S14.9 ¶2 names first"},
}

// KeepForeverEntry is one allowlisted type: the canonical type string (or a
// registered legacy alias of one), its family, and why the class is kept.
type KeepForeverEntry struct {
	Type   string
	Family eventlog.Family
	Note   string
}

// KeepForeverFamilies returns the family-level decision map — the authority the
// type list is derived from.
func KeepForeverFamilies() map[eventlog.Family]bool {
	out := make(map[eventlog.Family]bool, len(keepForeverFamilies))
	for _, f := range keepForeverFamilies {
		out[f.family] = f.keep
	}
	return out
}

// KeepForeverTypes derives the allowlist from the S14.2 registry: every
// registered type whose family is keep-forever, PLUS every registered legacy
// alias of such a family (a pre-rename row like `verify.round` is the same
// record and must not be stripped for wearing its old name — R14/§29). Sorted,
// so the seed order is deterministic.
func KeepForeverTypes() []KeepForeverEntry {
	keep := KeepForeverFamilies()
	notes := make(map[eventlog.Family]string, len(keepForeverFamilies))
	for _, f := range keepForeverFamilies {
		notes[f.family] = f.why
	}
	var out []KeepForeverEntry
	for _, ts := range eventlog.Registry().Types() {
		if keep[ts.Family] {
			out = append(out, KeepForeverEntry{Type: ts.Type, Family: ts.Family, Note: notes[ts.Family]})
		}
	}
	for alias, fam := range eventlog.Registry().Aliases() {
		if keep[fam] {
			out = append(out, KeepForeverEntry{Type: alias, Family: fam, Note: notes[fam] + " (legacy alias, §29 R14)"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// IsKeepForever reports whether a stored type is keep-forever. An UNKNOWN type
// (forward tolerance, S14.2 rule 3) is NOT keep-forever: an unregistered type
// has no declared family, so it is trace by default and its body is strippable.
// The row itself still survives.
func IsKeepForever(typ string) bool {
	fam, known := eventlog.Classify(typ)
	if !known {
		return false
	}
	return KeepForeverFamilies()[fam]
}

// EnsureKeepForeverSeeded seeds the retention_keep_forever allowlist (migration
// 0015) from the family registry, idempotently — the §32 SeedRows/EnsureSeeded
// precedent. It is called unconditionally at shell boot: the allowlist is what
// makes the 11.3 exporter boundary structural, so an unseeded table must never
// be a silent state (an unseeded table strips EVERY payload from the export,
// which is the safe direction, but it is not the correct one).
//
// INSERT OR IGNORE, never DELETE: the table's no-delete trigger makes
// keep-forever one-way, so a demotion in code preserves more data, never less.
func (s *Store) EnsureKeepForeverSeeded(ctx context.Context) (int, error) {
	entries := KeepForeverTypes()
	seeded := 0
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		for _, e := range entries {
			res, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO retention_keep_forever (type, family, note) VALUES (?, ?, ?)`,
				e.Type, string(e.Family), e.Note)
			if err != nil {
				return fmt.Errorf("retention: seed keep-forever %q: %w", e.Type, err)
			}
			if n, _ := res.RowsAffected(); n == 1 {
				seeded++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return seeded, nil
}
