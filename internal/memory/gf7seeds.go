package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf7seeds.go — the P3-GF7 taxonomy governance leg (Spec S09.10 row 1,
// S09.4 station 4, S09.8), minted rather than extended.
//
// CONVENTIONS §57 leaves one instruction for the packet that edits a question
// set: do not update the previous packet's digests and do not extend its
// Ensure, mint your own with your own provenance. This is that, one packet
// after gf3seeds.go said the same thing to whoever came next. The SOFTWARE set
// moved to v4 — the operator's W2 rebuild, driven by three real interview
// transcripts and a live benchmark walk — and this record supersedes GF3's
// entry to it, saying plainly whose revision it is and that the operator has
// not seen it yet.
//
// The GENERIC set is deliberately untouched: no operator finding names it, so it
// stays on GF3's record at v3 and this leg never looks at it.

// gf7GateProvenance is the ratification record the GF7 taxonomy entry carries.
// It is a separate type from GF3's and RW-12's identical-looking ones on
// purpose: a provenance record belongs to the packet that wrote it, and sharing
// the struct is the first step towards sharing the record.
type gf7GateProvenance struct {
	Packet   string `json:"packet"`
	Record   string `json:"record"`
	Decision string `json:"decision"`
	Drafting string `json:"drafting"`
	Ratified string `json:"ratified"`
}

func gf7OriginRef() (string, error) {
	raw, err := json.Marshal(gf7GateProvenance{
		Packet: "P3-GF7",
		Record: "P3/briefs/P3-GF7.md — the grounding brief; operator records " +
			"P3/design/b6-gate-operator-findings-r5-2026-08-23.md (§A per-slot verdicts, §C the seven hard rules, §D the W2 order) " +
			"and P3/design/b6-gate-operator-findings-r4-2026-08-23.md §F2; live benchmark walk " +
			"P3/design/w1-nexus-live-harvest-2026-08-27.md (H2, H3, H10, H18, H23)",
		Decision: "interview taxonomy v4, software family: behavior and terminology are NEVER ASKED and invert — the platform states " +
			"its understanding and the requester corrects it; indices_ranges and numerical_precision are settled internally and " +
			"disclosed as assumptions; all four keep their ids and weights and resolve at interview entry, so the Clearance meter " +
			"still counts them and each lands on the approval card where it can be contested. comparison_rules, output_format and " +
			"the units why-line are FIXED to the operator's bar; every surviving asked slot now carries a plain purpose, concrete " +
			"options, an effect line on each and exactly one recommended default. TWO slots are added from the live walk: " +
			"language_locale (9) and quality_bar (8), weights reasoned and recorded in the set's Source. Surviving ids and weights " +
			"are VERBATIM from v3. The generic set is unchanged and stays on the P3-GF3-BE1 record.",
		Drafting: "drafted at implementation time with Claude Opus 4.8 on 2026-08-27, per Spec S06.5 (\"drafted at implementation " +
			"time with the strongest available frontier model\")",
		Ratified: "PENDING: operator ratification is due at the planning-rework exit gate. The content ships governed under this " +
			"record so the governed file and the runtime seed cannot diverge; a refusal at the gate is executed as its own " +
			"supersession back to the P3-GF3-BE1 v3 content.",
	})
	if err != nil {
		return "", fmt.Errorf("memory: marshal GF7 seed provenance: %w", err)
	}
	return string(raw), nil
}

// gf7ContentDigest pins the EXACT governed content this packet's originRef
// attests to: sha256 over the software family's governed file bytes.
//
// Same doctrine as rw12ContentDigest and gf3ContentDigest, applied to this
// packet's own record. FOR THE NEXT PACKET THAT EDITS A QUESTION SET: do not
// update this digest, and do not extend this Ensure. Freeze this content as a
// snapshot the way gf3taxonomy_v3.go freezes GF3's, mint your own Ensure with
// your own provenance, and call it after this one.
var gf7ContentDigest = map[intake.Family]string{
	intake.FamilySoftware: "c37a0ab41509bad52192ac42eae0b1f6e6b731a6a505fff1a6b33b8e8911988f",
}

// gf7SupersedeFamilies is the one set this packet revised. It already has a
// governed entry (B2 created it; RW-12 and GF3 superseded it), so this leg only
// ever supersedes: it creates nothing and never resurrects a removed entry.
var gf7SupersedeFamilies = []intake.Family{intake.FamilySoftware}

// gf7TaxonomyTitles name the governed object at v4 for a human reading the
// memory surface.
var gf7TaxonomyTitles = map[intake.Family]string{
	intake.FamilySoftware: "Interview must-know taxonomy — software family (v4)",
}

// gf7TaxonomyContent renders the v4 content: the LIVE in-code seed, which is
// what this packet ships and what its digest pins.
func gf7TaxonomyContent(fam intake.Family) (string, error) {
	return taxonomyContentOf(fam, intake.SeedTaxonomies()[fam])
}

// verifyGF7Snapshot checks the revised family against the ratified digest,
// before anything is written. Divergence is ErrSeedDiverged — the same loud,
// never-fatal skip its predecessors take, and the same forcing function: the
// next editor writes its own Ensure instead of quietly borrowing this record.
func verifyGF7Snapshot() error {
	for _, fam := range gf7SupersedeFamilies {
		content, err := gf7TaxonomyContent(fam)
		if err != nil {
			return err
		}
		want, ok := gf7ContentDigest[fam]
		if !ok {
			return fmt.Errorf("%w: the %s family has no ratified digest", ErrSeedDiverged, fam)
		}
		if got := contentHash(content); got != want {
			return fmt.Errorf("%w: the %s question set has changed since the P3-GF7 record was written (have %s, recorded %s) — "+
				"a later edit needs its OWN governance function and provenance record, not this one's", ErrSeedDiverged, fam, got, want)
		}
	}
	return nil
}

// EnsureGF7TaxonomyGovernance brings the v4 software question set under S09.10
// governance, idempotently.
//
// It runs AFTER EnsureGF3TaxonomyGovernance, at the same boot call site. The
// order is what makes the version chain read true in a fresh world: B2's
// ratified v1, then RW-12's ratified v2, then GF3's pending v3, then this
// revision, each a version under the record of the gate that owns it (S09.8 —
// edits are new versions, never in-place mutation).
//
// Ratification is PENDING and the record says so. Waiting for the gate instead
// would leave the governed file at v3 while the runtime serves v4, which is the
// live-pointer divergence §57 closed; shipping under an honest pending record
// keeps the two halves together and leaves the gate a clean move either way.
//
// House scope needs its D10 holder: with no operator account the call returns
// ErrNoOperator and the caller retries at a later boot.
func (g *Gate) EnsureGF7TaxonomyGovernance(ctx context.Context) (TaxonomyGovernanceResult, error) {
	var res TaxonomyGovernanceResult
	var operator string
	err := g.s.db.QueryRowContext(ctx,
		`SELECT user_id FROM users WHERE role = 'operator' ORDER BY created_ts, user_id LIMIT 1`).Scan(&operator)
	if err == sql.ErrNoRows {
		return res, ErrNoOperator
	}
	if err != nil {
		return res, fmt.Errorf("memory: resolve operator: %w", err)
	}
	if err := verifyGF7Snapshot(); err != nil {
		return res, err
	}
	originRef, err := gf7OriginRef()
	if err != nil {
		return res, err
	}

	for _, fam := range gf7SupersedeFamilies {
		cur, err := g.s.HouseObject(ctx, taxonomyTopicKey(fam))
		if errors.Is(err, ErrNotFound) {
			// Nothing active to supersede: either the earlier governance has not
			// run yet (it runs first at boot) or the operator removed the
			// object. Neither is this function's to repair.
			continue
		}
		if err != nil {
			return res, err
		}
		content, err := gf7TaxonomyContent(fam)
		if err != nil {
			return res, err
		}
		want := contentHash(content)
		// What this supersession replaces: the content the GF3 record covers
		// (gf3taxonomy_v3.go). Anything else active on this topic belongs to a
		// record that is not this one — see rw12PredecessorContent for why an
		// Ensure moves only the content it was written against.
		previous, err := gf3TaxonomyContent(fam)
		if err != nil {
			return res, err
		}

		// Decide from the ROW, never from the file (see committedContentHash).
		committed, verifiable, err := g.committedContentHash(ctx, cur.ID)
		if err != nil {
			return res, err
		}
		if !verifiable {
			res.Unverifiable++
			continue
		}
		onDisk := contentHash(cur.Content)
		if committed == want {
			// The record already holds this packet's content; only the disk may
			// still need putting right. The repair runs whatever the version
			// history says, because a torn file is not a decision.
			if committed != onDisk {
				res.Repaired++
				if err := g.repairGovernedFile(cur, content); err != nil {
					return res, err
				}
			}
			continue
		}
		// Applied once, and later decisions stand (see decisionRecorded). This
		// leg needs it exactly as GF3's did: ratification is PENDING, so the
		// operator may REFUSE v4 at the gate, and the refusal is executed as a
		// supersession back to the byte-exact v3 content. Comparing content
		// alone, the next boot would read that refusal as an un-revised
		// predecessor and re-apply the revision it rejected.
		recorded, err := g.decisionRecorded(ctx, taxonomyTopicKey(fam), originRef)
		if err != nil {
			return res, err
		}
		if recorded {
			continue
		}
		if committed != contentHash(previous) {
			continue
		}
		if committed != onDisk {
			res.Repaired++
		}
		draft := taxonomyDraft(fam, content)
		draft.Title = gf7TaxonomyTitles[fam]
		if _, err := g.writeEntry(ctx, operator, draft, writeOpts{
			origin: OriginImported, originRef: originRef, supersedes: cur.ID,
		}); err != nil {
			return res, fmt.Errorf("memory: supersede %s taxonomy: %w", fam, err)
		}
		res.Superseded++
	}
	return res, nil
}
