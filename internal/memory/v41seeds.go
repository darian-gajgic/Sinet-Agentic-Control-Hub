package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// v41seeds.go — the P3-V41 taxonomy governance leg (Spec S09.10 row 1,
// S09.4 station 4, S09.8), minted rather than extended.
//
// CONVENTIONS §57 leaves one instruction for the packet that edits a question
// set: do not update the previous packet's digests and do not extend its
// Ensure, mint your own with your own provenance. This is that, one packet
// after gf7seeds.go said the same thing to whoever came next. The SOFTWARE set
// moves to v4.1 — one weight, quality_bar 8 to 12, on the operator's ruling —
// and this record supersedes GF7's entry to it.
//
// Two things separate this leg from the three before it. Its predecessor is a
// FROZEN snapshot from the start (gf7taxonomy_v4.go, this packet's own work),
// so the "what does this replace" question is answered by a constant rather
// than by whatever the code says today. And its record is RATIFIED, not
// pending: the operator ruled before the content shipped, so there is no gate
// still owed and no refusal path to leave open.
//
// The GENERIC set is untouched: the ruling names the software family alone, so
// it stays on GF3's record at v3 and this leg never looks at it.

// v41GateProvenance is the ratification record the P3-V41 taxonomy entry
// carries. It is a separate type from GF7's, GF3's and RW-12's
// identical-looking ones on purpose: a provenance record belongs to the packet
// that wrote it, and sharing the struct is the first step towards sharing the
// record.
//
// It carries no Drafting field, unlike the three records before it. Nothing was
// drafted at v4.1 — it is a DATA change to one weight that the operator ruled
// (Spec S06.5: slot weights ship in the taxonomy file and are
// operator-editable) — and a record that claimed a drafting pass would be
// claiming work nobody did.
type v41GateProvenance struct {
	Packet   string `json:"packet"`
	Record   string `json:"record"`
	Decision string `json:"decision"`
	Ratified string `json:"ratified"`
}

func v41OriginRef() (string, error) {
	raw, err := json.Marshal(v41GateProvenance{
		Packet: "P3-V41",
		Record: "P3/gates/rework-sitting-gate.md — the rework sitting of 2026-09-17, items A1 and A2 and the operator's " +
			"answers; P3/briefs/P3-V41.md — the grounding brief",
		Decision: "interview taxonomy v4.1, software family: quality_bar moves from weight 8 to weight 12, and nothing else " +
			"moves — the slot's name, must-know, question, why-line, recommended default and options are verbatim from v4, as " +
			"are the ids, weights, order and ask posture of every other slot. The evidence is the sitting's own webshop task, " +
			"which failed on the one axis nobody was asked about: look_feel was answered and quality_bar was never asked, so " +
			"placeholder photos shipped as finished work. At the standard tier the interview stopped before output_format, " +
			"quality_bar and units and all three rode into the planner's assumptions; at 12 the slot reaches the first card at " +
			"every tier. 12 equals the three measured slots (collection semantics, comparison rules, ordering & atomicity) and " +
			"never outranks them, which is the v4 record's own rule. The necessary consequence is recorded with it: a 12 on the " +
			"first card pushes the lowest-weighted asked slot off the second, so language_locale (9) becomes a listed assumption " +
			"at the standard tier and is asked at the high tier only. Weights are taxonomy data the spec makes " +
			"operator-editable, so no tunable setting moved — the clearance floors stay 60/75/90 — and no S18 sweep or S00.9 " +
			"amendment was owed. Item A1 of the same sitting ratified v4 AS SHIPPED, so the v4 record it supersedes states that " +
			"in the operator's words rather than waiting on a gate. The generic set is unchanged and stays on the P3-GF3-BE1 " +
			"record.",
		Ratified: "RATIFIED for P3-V41 by the operator on 2026-09-17 at the rework sitting recorded in P3/gates/rework-sitting-gate.md: " +
			"item A2, quality_bar 8 to 12, in the operator's words \"ok do it\"; item A1, taxonomy v4 as shipped, in the " +
			"operator's words \"ok for now, will need some refinement later\". The refinement A1 defers is a later v4.x " +
			"revision, opened when the operator names what it is. Nothing here is owed to a later gate.",
	})
	if err != nil {
		return "", fmt.Errorf("memory: marshal P3-V41 seed provenance: %w", err)
	}
	return string(raw), nil
}

// v41ContentDigest pins the EXACT governed content this packet's originRef
// attests to: sha256 over the software family's governed file bytes.
//
// Same doctrine as rw12ContentDigest, gf3ContentDigest and gf7ContentDigest,
// applied to this packet's own record. FOR THE NEXT PACKET THAT EDITS A QUESTION
// SET: do not update this digest, and do not extend this Ensure. Freeze this
// content as a snapshot the way gf7taxonomy_v4.go freezes GF7's, mint your own
// Ensure with your own provenance, and call it after this one.
var v41ContentDigest = map[intake.Family]string{
	intake.FamilySoftware: "8c5e548f35447bb295488b0b89d7e51476403d323153e2f81c940f52c3db2a6a",
}

// v41SupersedeFamilies is the one set this ruling names. It already has a
// governed entry (B2 created it; RW-12, GF3 and GF7 superseded it), so this leg
// only ever supersedes: it creates nothing and never resurrects a removed entry.
var v41SupersedeFamilies = []intake.Family{intake.FamilySoftware}

// v41TaxonomyTitles name the governed object at v4.1 for a human reading the
// memory surface.
var v41TaxonomyTitles = map[intake.Family]string{
	intake.FamilySoftware: "Interview must-know taxonomy — software family (v4.1)",
}

// v41TaxonomyContent renders the v4.1 content: the LIVE in-code seed, which is
// what this packet ships and what its digest pins. The next packet to edit the
// software set repoints this at a frozen snapshot, as this one did to GF7's.
func v41TaxonomyContent(fam intake.Family) (string, error) {
	return taxonomyContentOf(fam, intake.SeedTaxonomies()[fam])
}

// verifyV41Snapshot checks the revised family against the ratified digest,
// before anything is written. Divergence is ErrSeedDiverged — the same loud,
// never-fatal skip its predecessors take, and the same forcing function: the
// next editor writes its own Ensure instead of quietly borrowing this record.
func verifyV41Snapshot() error {
	for _, fam := range v41SupersedeFamilies {
		content, err := v41TaxonomyContent(fam)
		if err != nil {
			return err
		}
		want, ok := v41ContentDigest[fam]
		if !ok {
			return fmt.Errorf("%w: the %s family has no ratified digest", ErrSeedDiverged, fam)
		}
		if got := contentHash(content); got != want {
			return fmt.Errorf("%w: the %s question set has changed since the P3-V41 record was written (have %s, recorded %s) — "+
				"a later edit needs its OWN governance function and provenance record, not this one's", ErrSeedDiverged, fam, got, want)
		}
	}
	return nil
}

// EnsureV41TaxonomyGovernance brings the v4.1 software question set under S09.10
// governance, idempotently.
//
// It runs AFTER EnsureGF7TaxonomyGovernance, at the same boot call site. The
// order is what makes the version chain read true in a fresh world: B2's
// ratified v1, then RW-12's ratified v2, then GF3's v3, then GF7's v4, then this
// ruling, each a version under the record of the gate that owns it (S09.8 —
// edits are new versions, never in-place mutation). A world that already holds
// v4 receives v4.1 by the same path at its next boot.
//
// House scope needs its D10 holder: with no operator account the call returns
// ErrNoOperator and the caller retries at a later boot.
func (g *Gate) EnsureV41TaxonomyGovernance(ctx context.Context) (TaxonomyGovernanceResult, error) {
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
	if err := verifyV41Snapshot(); err != nil {
		return res, err
	}
	originRef, err := v41OriginRef()
	if err != nil {
		return res, err
	}

	for _, fam := range v41SupersedeFamilies {
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
		content, err := v41TaxonomyContent(fam)
		if err != nil {
			return res, err
		}
		want := contentHash(content)
		// What this supersession replaces: the content the GF7 record covers
		// (gf7taxonomy_v4.go). Anything else active on this topic belongs to a
		// record that is not this one — see rw12PredecessorContent for why an
		// Ensure moves only the content it was written against.
		previous, err := gf7TaxonomyContent(fam)
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
		// Applied once, and later decisions stand (see decisionRecorded).
		// Content alone cannot tell "never applied here" from "applied, then
		// the operator moved it somewhere else through the gate": both read as
		// a committed hash that is not this packet's. The originRef can, so it
		// is what the guard asks about.
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
		draft.Title = v41TaxonomyTitles[fam]
		if _, err := g.writeEntry(ctx, operator, draft, writeOpts{
			origin: OriginImported, originRef: originRef, supersedes: cur.ID,
		}); err != nil {
			return res, fmt.Errorf("memory: supersede %s taxonomy: %w", fam, err)
		}
		res.Superseded++
	}
	return res, nil
}
