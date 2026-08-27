package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf3seeds.go — the P3-GF3-BE1 taxonomy governance leg (Spec S09.10 row 1,
// S09.4 station 4, S09.8), minted rather than extended.
//
// CONVENTIONS §57 leaves one instruction for the packet that edits a question
// set: do not update the previous packet's digests and do not extend its
// Ensure, mint your own with your own provenance. This is that. The software
// and generic sets moved to v3 (plain questions, options on every slot, a
// plain-words why); RW-12's Ensure keeps writing the frozen v2/v1 content its
// gate ratified, and this one supersedes those entries to v3 under a record
// that says whose revision it is and that the operator has not seen it yet.

// gf3GateProvenance is the ratification record the GF3 taxonomy entries carry.
// It is a separate type from RW-12's identical-looking one on purpose: a
// provenance record belongs to the packet that wrote it, and sharing the struct
// is the first step towards sharing the record.
type gf3GateProvenance struct {
	Packet   string `json:"packet"`
	Record   string `json:"record"`
	Decision string `json:"decision"`
	Ratified string `json:"ratified"`
}

func gf3OriginRef() (string, error) {
	raw, err := json.Marshal(gf3GateProvenance{
		Packet:   "P3-GF3-BE1",
		Record:   "P3/STATE.md — the P3-GF3-BE1 landing entry; design note P3/design/gf3-planning-rework-design-2026-08-23.md §2.A",
		Decision: "taxonomy v3: the software and generic question sets revised to plain-language questions with concrete options and a why line (ids, weights, count and order verbatim from v2)",
		Ratified: "PENDING: operator ratification is due at the resumed B6 gate. The content ships governed under this record so the governed file and the runtime seed cannot diverge; a refusal at the gate is executed as its own supersession back to the RW-12 content.",
	})
	if err != nil {
		return "", fmt.Errorf("memory: marshal GF3 seed provenance: %w", err)
	}
	return string(raw), nil
}

// gf3ContentDigest pins the EXACT governed content this packet's originRef
// attests to: sha256 over each revised family's governed file bytes.
//
// Same doctrine as rw12ContentDigest, applied to this packet's own record. FOR
// THE NEXT PACKET THAT EDITS A QUESTION SET: do not update these digests, and
// do not extend this Ensure. Freeze this content as a snapshot the way
// rw12taxonomy_v2.go freezes RW-12's, mint your own Ensure with your own
// provenance, and call it after this one.
var gf3ContentDigest = map[intake.Family]string{
	intake.FamilySoftware: "dd41cf1accd549ef9c55d41b237c3674e346a006042ae7ccf7d0edac1864f14f",
	intake.FamilyGeneric:  "6c647bcb57daea5170f23bf031e1b6344a673357f92bf2227b7e54326345de9f",
}

// gf3SupersedeFamilies are the two sets this packet revised. Both already have
// governed entries (B2 created them, RW-12 superseded them), so this leg only
// ever supersedes: it creates nothing and never resurrects a removed entry.
var gf3SupersedeFamilies = []intake.Family{intake.FamilySoftware, intake.FamilyGeneric}

// gf3TaxonomyTitles name the governed objects at v3 for a human reading the
// memory surface.
var gf3TaxonomyTitles = map[intake.Family]string{
	intake.FamilySoftware: "Interview must-know taxonomy — software family (v3)",
	intake.FamilyGeneric:  "Interview must-know taxonomy — generic fallback family (v3)",
}

// gf3TaxonomyContent renders the GF3-RATIFIED content of one family — the
// frozen snapshot, never the live seed (gf3taxonomy_v3.go).
//
// This function used to read intake.SeedTaxonomies(), which made every GF3
// governed entry a pointer at whatever the code says today; P3-GF7 revises the
// software set to v4, so the pointer would now write v4 content under this
// packet's record. The v4 content enters as its own supersession instead
// (gf7seeds.go). It is the same move this packet made to RW-12's
// taxonomyContent, one packet later and for the same reason — the ONE edit a
// later packet makes to an earlier packet's governance file.
func gf3TaxonomyContent(fam intake.Family) (string, error) {
	return taxonomyContentOf(fam, gf3TaxonomySnapshot()[fam])
}

// verifyGF3Snapshot checks both revised families against the ratified digest,
// before anything is written. Divergence is ErrSeedDiverged — the same loud,
// never-fatal skip RW-12 takes, and the same forcing function: the next editor
// writes its own Ensure instead of quietly borrowing this record.
func verifyGF3Snapshot() error {
	for _, fam := range gf3SupersedeFamilies {
		content, err := gf3TaxonomyContent(fam)
		if err != nil {
			return err
		}
		want, ok := gf3ContentDigest[fam]
		if !ok {
			return fmt.Errorf("%w: the %s family has no ratified digest", ErrSeedDiverged, fam)
		}
		if got := contentHash(content); got != want {
			return fmt.Errorf("%w: the %s question set has changed since the P3-GF3-BE1 record was written (have %s, recorded %s) — "+
				"a later edit needs its OWN governance function and provenance record, not this one's", ErrSeedDiverged, fam, got, want)
		}
	}
	return nil
}

// EnsureGF3TaxonomyGovernance brings the v3 software and generic question sets
// under S09.10 governance, idempotently.
//
// It runs AFTER EnsureRW12TaxonomyGovernance, at the same boot call site. The
// order is what makes the version chain read true in a fresh world: B2's
// ratified v1, then RW-12's ratified v2, then this revision, each a version
// under the record of the gate that owns it (S09.8 — edits are new versions,
// never in-place mutation).
//
// Ratification is PENDING and the record says so. Waiting for the gate instead
// would leave the governed file at v2 while the runtime serves v3, which is the
// live-pointer divergence §57 closed; shipping under an honest pending record
// keeps the two halves together and leaves the gate a clean move either way.
//
// House scope needs its D10 holder: with no operator account the call returns
// ErrNoOperator and the caller retries at a later boot.
func (g *Gate) EnsureGF3TaxonomyGovernance(ctx context.Context) (TaxonomyGovernanceResult, error) {
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
	if err := verifyGF3Snapshot(); err != nil {
		return res, err
	}
	originRef, err := gf3OriginRef()
	if err != nil {
		return res, err
	}

	for _, fam := range gf3SupersedeFamilies {
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
		content, err := gf3TaxonomyContent(fam)
		if err != nil {
			return res, err
		}
		want := contentHash(content)
		// What this supersession replaces: the content the RW-12 gate ratified
		// (rw12taxonomy_v2.go). Anything else active on this topic belongs to a
		// record that is not this one — see rw12PredecessorContent for why an
		// Ensure moves only the content it was written against.
		previous, err := taxonomyContent(fam)
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
		// is the leg that needs it most: ratification is PENDING, so the operator
		// may REFUSE v3 at the gate, and the refusal is executed as a supersession
		// back to the byte-exact frozen v2 content. Comparing content alone, the
		// next boot would read that refusal as an un-revised predecessor and
		// re-apply the revision it rejected.
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
		draft.Title = gf3TaxonomyTitles[fam]
		if _, err := g.writeEntry(ctx, operator, draft, writeOpts{
			origin: OriginImported, originRef: originRef, supersedes: cur.ID,
		}); err != nil {
			return res, fmt.Errorf("memory: supersede %s taxonomy: %w", fam, err)
		}
		res.Superseded++
	}
	return res, nil
}
