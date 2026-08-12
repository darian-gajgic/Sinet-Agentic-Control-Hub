package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// rw12seeds.go — the P3-RW-12 taxonomy governance leg (Spec S09.10 row 1,
// S09.4 station 4, S09.8). Deep-Plan interview depth ships question sets for
// every family in the vocabulary; this is their 8.3-gate entry, built on the
// EnsureB2SeedGovernance discipline.
//
// It does two things a boot after this packet needs done, and nothing else:
// it GOVERNS the four new family sets, and it brings the software set's
// governed content up to what the code now ships by writing a NEW VERSION
// that supersedes the old one. Supersession is the only honest move here: the
// old version stays in the record, retired, still attributed to the gate that
// approved it, and the new one carries its own provenance (S09.8 — edits are
// new versions, never in-place mutation; the migration triggers refuse the
// alternative even to a raw-SQL holder).

// rw12GateProvenance is the ratification record the RW-12 taxonomy entries
// carry. The approval is the operator's free-text answer at the P3-RW-12
// packet gate — the same N20 pattern the B2 seeds record (operator
// ratification IS the approval), pointing at the dated STATE landing entry
// rather than a separate ceremony.
type rw12GateProvenance struct {
	Packet   string `json:"packet"`
	Record   string `json:"record"`
	Decision string `json:"decision"`
	Ratified string `json:"ratified"`
}

func rw12OriginRef() (string, error) {
	raw, err := json.Marshal(rw12GateProvenance{
		Packet:   "P3-RW-12",
		Record:   "P3/STATE.md — the P3-RW-12 landing entry",
		Decision: "Deep-Plan taxonomy content ratification (five family question sets at interview depth)",
		Ratified: "operator free-text answer at the P3-RW-12 packet gate, 2026-08-13",
	})
	if err != nil {
		return "", fmt.Errorf("memory: marshal RW-12 seed provenance: %w", err)
	}
	return string(raw), nil
}

// rw12NewFamilies are the families whose question sets P3-RW-12 seeds for the
// first time. Software and generic already have governed entries from B2 and
// are reached through the supersession leg instead.
var rw12NewFamilies = []intake.Family{
	intake.FamilyResearch, intake.FamilyContent, intake.FamilyData, intake.FamilyChore,
}

// rw12SupersedeFamilies are the families B2 already governs, whose governed
// content is re-checked against the in-code seed on every boot. Generic is
// listed even though P3-RW-12 left it unchanged: the check is content-driven,
// so it costs one comparison today and closes the drift the moment a later
// packet edits it.
var rw12SupersedeFamilies = []intake.Family{intake.FamilySoftware, intake.FamilyGeneric}

func taxonomyTopicKey(fam intake.Family) string { return "intake/taxonomy/" + string(fam) }
func taxonomyFileName(fam intake.Family) string { return "intake-taxonomy-" + string(fam) + ".json" }
func taxonomyEntryID(fam intake.Family) string  { return "seed-intake-taxonomy-" + string(fam) }

// taxonomyTitles name the governed objects for a human reading the memory
// surface. They say what the object IS, not which packet wrote it.
var taxonomyTitles = map[intake.Family]string{
	intake.FamilySoftware: "Interview must-know taxonomy — software family (Deep-Plan v2)",
	intake.FamilyResearch: "Interview must-know taxonomy — research family (Deep-Plan)",
	intake.FamilyContent:  "Interview must-know taxonomy — content family (Deep-Plan)",
	intake.FamilyData:     "Interview must-know taxonomy — data family (Deep-Plan)",
	intake.FamilyChore:    "Interview must-know taxonomy — chore family (Deep-Plan)",
	intake.FamilyGeneric:  "Interview must-know taxonomy — generic fallback family (B2-2 seed)",
}

// taxonomyContent renders one family's in-code seed as the strict JSON the
// governed file holds — the SAME form intake.LoadTaxonomy parses, so the
// governed file IS the operator-editable override input (CONVENTIONS §17).
func taxonomyContent(fam intake.Family) (string, error) {
	tax, ok := intake.SeedTaxonomies()[fam]
	if !ok {
		return "", fmt.Errorf("memory: no in-code taxonomy seed for the %s family", fam)
	}
	raw, err := json.MarshalIndent(tax, "", "  ")
	if err != nil {
		return "", fmt.Errorf("memory: marshal %s taxonomy: %w", fam, err)
	}
	return string(raw) + "\n", nil
}

func taxonomyDraft(fam intake.Family, content string) Draft {
	return Draft{
		Scope: ScopeHouse, Kind: KindTaxonomy, Title: taxonomyTitles[fam],
		Content:  content,
		TopicKey: taxonomyTopicKey(fam),
		// The machinery marker keeps governed pipeline objects out of stage
		// briefs (conservative selector matching): their consumer is the S06
		// interview, not a prompt.
		Selectors:  Selectors{TaskType: "machinery:intake"},
		FileBacked: true, FileName: taxonomyFileName(fam),
	}
}

// EnsureRW12TaxonomyGovernance brings the Deep-Plan interview taxonomies under
// S09.10 governance, idempotently.
//
// Creates: the four families seeded for the first time. An entry that exists
// in ANY status — active, retired, removed, tombstoned — is left alone, so a
// removal is never undone by a boot.
//
// Supersedes: a family B2 already governs whose ACTIVE governed content no
// longer matches the in-code seed gets a new version attributed to the
// operator, chained by supersedes_id, with the old version retired in the same
// transaction. Content equality is the trigger, so the second boot is a no-op
// and the function never needs to know which packet ran last.
//
// House scope needs its D10 holder: with no operator account the call returns
// ErrNoOperator and the caller retries at a later boot.
//
// It runs AFTER EnsureB2SeedGovernance, which is what puts the software and
// generic entries there to supersede in the first place.
func (g *Gate) EnsureRW12TaxonomyGovernance(ctx context.Context) (created, superseded int, err error) {
	var operator string
	err = g.s.db.QueryRowContext(ctx,
		`SELECT user_id FROM users WHERE role = 'operator' ORDER BY created_ts, user_id LIMIT 1`).Scan(&operator)
	if err == sql.ErrNoRows {
		return 0, 0, ErrNoOperator
	}
	if err != nil {
		return 0, 0, fmt.Errorf("memory: resolve operator: %w", err)
	}
	originRef, err := rw12OriginRef()
	if err != nil {
		return 0, 0, err
	}

	for _, fam := range rw12NewFamilies {
		entryID := taxonomyEntryID(fam)
		var n int
		if err := g.s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM knowledge_entries WHERE entry_id = ?`, entryID).Scan(&n); err != nil {
			return created, superseded, fmt.Errorf("memory: taxonomy existence check: %w", err)
		}
		if n > 0 {
			continue
		}
		content, err := taxonomyContent(fam)
		if err != nil {
			return created, superseded, err
		}
		if _, err := g.writeEntry(ctx, operator, taxonomyDraft(fam, content), writeOpts{
			origin: OriginImported, originRef: originRef, fixedID: entryID,
		}); err != nil {
			return created, superseded, fmt.Errorf("memory: govern %s taxonomy: %w", fam, err)
		}
		created++
	}

	for _, fam := range rw12SupersedeFamilies {
		cur, err := g.s.HouseObject(ctx, taxonomyTopicKey(fam))
		if errors.Is(err, ErrNotFound) {
			// Nothing active to supersede: either B2 governance has not run
			// yet (it runs first at boot) or the operator removed the object.
			// Neither is this function's to repair.
			continue
		}
		if err != nil {
			return created, superseded, err
		}
		content, err := taxonomyContent(fam)
		if err != nil {
			return created, superseded, err
		}
		if cur.Content == content {
			continue
		}
		// The new version reuses the canonical file name, so the file at
		// house/intake-taxonomy-<family>.json always holds the CURRENT
		// content — which is what an operator-editable override input has to
		// do. The retired version's own bytes live in git history, which is
		// where S09.8 puts superseded content and why the D9 commit-on-
		// approval seam exists.
		if _, err := g.writeEntry(ctx, operator, taxonomyDraft(fam, content), writeOpts{
			origin: OriginImported, originRef: originRef, supersedes: cur.ID,
		}); err != nil {
			return created, superseded, fmt.Errorf("memory: supersede %s taxonomy: %w", fam, err)
		}
		superseded++
	}
	return created, superseded, nil
}
