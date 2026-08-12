package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// B2 seed governance (Spec S09.10): every knowledge object another section
// declares is an L2 entry at house scope under S09's governance —
// versioned, attributable, individually removable, stored as row +
// git-versioned file. The B2 packets shipped the intake taxonomies, the
// P47 trigger file, and the verification rubric/golden/calibration seeds
// in code (CONVENTIONS §14/§15) with formal gate entry deferred to S09;
// EnsureB2SeedGovernance is that entry. The recorded provenance is the B2
// gate's ratification of the seed content (P3/gates/B2-report.md §7 D3,
// operator, 2026-07-20) — the operator ratification IS the approval (the
// N20 import pattern: operator import = the approval).
//
// The in-code seeds remain the runtime source for the B2 pipelines (their
// operator-editable Load* override paths unchanged, per the S06.3 v0 rule
// "changes = manual operator edits"); these governed entries are the S09
// lifecycle home — provenance, re-verify intervals, removability — and
// carry machinery-consumer selectors so they never inject into stage
// briefs (their consumers are the owning sections' machinery, not
// prompts).

// b2GateProvenance is the ratification record the governed entries carry.
type b2GateProvenance struct {
	Gate     string `json:"gate"`
	Record   string `json:"record"`
	Decision string `json:"decision"`
	Ratified string `json:"ratified"`
}

// b2Seed names one governed object.
type b2Seed struct {
	entryID  string
	kind     Kind
	title    string
	topicKey string
	taskType string // machinery consumer marker (never matches a stage query)
	fileName string
	object   any
}

// b2Seeds enumerates the B2-ratified seed objects in governance order.
//
// The taxonomy entries record what the B2 gate RATIFIED, which is not always
// what the code says today: the software set carries its frozen v1 snapshot
// (b2taxonomy_v1.go) because P3-RW-12 revised the runtime seed to v2, and a
// provenance line pointing at a gate that never saw v2 would be a false
// record. Bringing the governed content up to date is a SUPERSESSION under
// its own provenance — EnsureRW12TaxonomyGovernance — never a quiet rewrite
// of what a past gate is said to have approved.
func b2Seeds() ([]b2Seed, error) {
	taxonomies := intake.SeedTaxonomies()
	generic, ok := taxonomies[intake.FamilyGeneric]
	if !ok {
		return nil, fmt.Errorf("memory: generic taxonomy seed missing")
	}
	return []b2Seed{
		{
			entryID: "seed-intake-taxonomy-software", kind: KindTaxonomy,
			title:    "Interview must-know taxonomy — software family (ClarifyCodeBench 10-type, B2-2 seed)",
			topicKey: "intake/taxonomy/software", taskType: "machinery:intake",
			fileName: "intake-taxonomy-software.json", object: b2SoftwareTaxonomyV1(),
		},
		{
			entryID: "seed-intake-taxonomy-generic", kind: KindTaxonomy,
			title:    "Interview must-know taxonomy — generic fallback family (B2-2 seed)",
			topicKey: "intake/taxonomy/generic", taskType: "machinery:intake",
			fileName: "intake-taxonomy-generic.json", object: generic,
		},
		{
			entryID: "seed-intake-p47-triggers", kind: KindTriggerRules,
			title:    "P47 research-trigger rule file (S06.3 table, B2-2 seed)",
			topicKey: "intake/p47-triggers", taskType: "machinery:intake",
			fileName: "intake-p47-triggers.json", object: intake.SeedTriggers(),
		},
		{
			entryID: "seed-verify-rubric-software", kind: KindRubric,
			title:    "Verification rubric bundle — software domain (S07.10, B2-3 seed)",
			topicKey: "verify/rubric/software", taskType: "machinery:verify",
			fileName: "verify-rubric-software.json", object: verify.SeedSoftwareRubric(),
		},
		{
			entryID: "seed-verify-golden-software", kind: KindExemplar,
			title:    "Judge golden set — software domain, 26 planted/clean cases (S07.10, B2-3 seed)",
			topicKey: "verify/golden-set/software", taskType: "machinery:verify",
			fileName: "verify-golden-software.json", object: verify.SeedGoldenSet(),
		},
		{
			entryID: "seed-verify-calibration-entailment", kind: KindExemplar,
			title:    "Entailment calibration set — 10 planted pairs (S07.4, B2-3 seed)",
			topicKey: "verify/calibration/entailment", taskType: "machinery:verify",
			fileName: "verify-calibration-entailment.json", object: verify.SeedCalibrationSet(),
		},
	}, nil
}

// EnsureB2SeedGovernance re-homes the B2 seed objects under S09.10
// governance, idempotently: an entry that exists in ANY status — active,
// retired, removed, tombstoned — is left alone, so a later removal or true
// deletion is never resurrected by a boot. House scope needs its D10
// holder: with no operator account the call returns ErrNoOperator and the
// caller retries at a later boot.
func (g *Gate) EnsureB2SeedGovernance(ctx context.Context) (created int, err error) {
	var operator string
	err = g.s.db.QueryRowContext(ctx,
		`SELECT user_id FROM users WHERE role = 'operator' ORDER BY created_ts, user_id LIMIT 1`).Scan(&operator)
	if err == sql.ErrNoRows {
		return 0, ErrNoOperator
	}
	if err != nil {
		return 0, fmt.Errorf("memory: resolve operator: %w", err)
	}
	seeds, err := b2Seeds()
	if err != nil {
		return 0, err
	}
	originRef, err := json.Marshal(b2GateProvenance{
		Gate: "B2", Record: "P3/gates/B2-report.md §7", Decision: "D3 seed-content ratification",
		Ratified: "2026-07-20",
	})
	if err != nil {
		return 0, fmt.Errorf("memory: marshal seed provenance: %w", err)
	}
	for _, s := range seeds {
		var n int
		if err := g.s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM knowledge_entries WHERE entry_id = ?`, s.entryID).Scan(&n); err != nil {
			return created, fmt.Errorf("memory: seed existence check: %w", err)
		}
		if n > 0 {
			continue
		}
		raw, err := json.MarshalIndent(s.object, "", "  ")
		if err != nil {
			return created, fmt.Errorf("memory: marshal seed %s: %w", s.entryID, err)
		}
		d := Draft{
			Scope: ScopeHouse, Kind: s.kind, Title: s.title,
			Content:  string(raw) + "\n",
			TopicKey: s.topicKey,
			// The machinery marker keeps governed pipeline objects out of
			// stage briefs (conservative selector matching, source.go):
			// their consumers are S06/S07 machinery, not prompts.
			Selectors:  Selectors{TaskType: s.taskType},
			FileBacked: true, FileName: s.fileName,
		}
		if _, err := g.writeEntry(ctx, operator, d, writeOpts{
			origin: OriginImported, originRef: string(originRef), fixedID: s.entryID,
		}); err != nil {
			return created, fmt.Errorf("memory: govern seed %s: %w", s.entryID, err)
		}
		created++
	}
	return created, nil
}

// ComposerPlaybookEntryID is the stable governance id of the composer
// playbook house object (Spec S09.10 "Composer best-practice playbook";
// declared in Spec S08.6, content owned by internal/worker).
const ComposerPlaybookEntryID = "seed-composer-playbook"

// EnsureComposerPlaybook seeds the composer playbook as a governed S09.10
// house object, idempotently — the EnsureB2SeedGovernance discipline: an
// entry that exists in ANY status (active, retired, removed, tombstoned)
// is left alone, so a removal or supersession is never resurrected by a
// boot; without an operator account the call returns ErrNoOperator and the
// caller retries at a later boot (house scope needs its D10 holder).
// Unlike the B2 seeds, this content's ratification is PENDING — a B3 gate
// item — which the recorded provenance states; post-ratification edits are
// ordinary gated new versions (Spec S09.10: no private update path).
func (g *Gate) EnsureComposerPlaybook(ctx context.Context) (created bool, err error) {
	var operator string
	err = g.s.db.QueryRowContext(ctx,
		`SELECT user_id FROM users WHERE role = 'operator' ORDER BY created_ts, user_id LIMIT 1`).Scan(&operator)
	if err == sql.ErrNoRows {
		return false, ErrNoOperator
	}
	if err != nil {
		return false, fmt.Errorf("memory: resolve operator: %w", err)
	}
	var n int
	if err := g.s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_entries WHERE entry_id = ?`, ComposerPlaybookEntryID).Scan(&n); err != nil {
		return false, fmt.Errorf("memory: playbook existence check: %w", err)
	}
	if n > 0 {
		return false, nil
	}
	originRef, err := json.Marshal(b2GateProvenance{
		Gate: "B3", Record: "P3/gates/B3-report.md §7", Decision: "composer-playbook seed ratification",
		Ratified: "ratified at the B3 gate 2026-07-22 (operator D4, free-text)",
	})
	if err != nil {
		return false, fmt.Errorf("memory: marshal playbook provenance: %w", err)
	}
	d := Draft{
		Scope: ScopeHouse, Kind: KindPlaybook, Title: worker.ComposerPlaybookTitle,
		Content:  worker.ComposerPlaybookSeed(),
		TopicKey: worker.ComposerPlaybookTopicKey,
		// Machinery consumer: the S08.6 composer reads it by topic key; it
		// never injects into stage briefs.
		Selectors:  Selectors{TaskType: "machinery:worker-composer"},
		FileBacked: true, FileName: "composer-playbook.md",
	}
	if _, err := g.writeEntry(ctx, operator, d, writeOpts{
		origin: OriginImported, originRef: string(originRef), fixedID: ComposerPlaybookEntryID,
	}); err != nil {
		return false, fmt.Errorf("memory: govern composer playbook: %w", err)
	}
	return true, nil
}
