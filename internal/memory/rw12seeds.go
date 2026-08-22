package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// ErrSeedDiverged is returned when the in-code taxonomy seeds no longer match
// the content P3-RW-12's provenance record covers. It is not a boot failure:
// the caller logs it loudly and carries on, because writing content this
// packet's gate never saw under this packet's originRef is the one outcome
// worse than an ungoverned entry.
var ErrSeedDiverged = errors.New("memory: in-code taxonomy seeds diverge from the ratified P3-RW-12 snapshot")

// rw12ContentDigest pins the EXACT governed content the P3-RW-12 originRef
// attests to: sha256 over each family's governed file bytes.
//
// This is the snapshot half of the doctrine b2taxonomy_v1.go states, applied
// to this packet's own provenance (drain r1, F3). Without it the Ensure below
// would marshal whatever SeedTaxonomies() returns and write it under a fixed
// ratification record — so a later packet editing a question set would have
// its content superseded into governance at the next boot, attributed to a
// gate that never read it, with nothing anywhere to notice.
//
// Digests rather than committed copies of the content, deliberately: five
// taxonomies are ~550 lines, and a second hand-maintained copy of them would
// be two things that must agree — the very drift this is meant to prevent —
// while doubling what the operator must read at the gate. A digest pins the
// same fact, is derived rather than transcribed, and fails loudly the moment
// it stops being true. TestRW12DigestsMatchTheShippedSeeds is the build-time
// tripwire; this map is the runtime one.
//
// FOR THE NEXT PACKET THAT EDITS A QUESTION SET: do not update these digests.
// They record what P3-RW-12's gate ratified. Mint your own Ensure with your
// own originRef and your own snapshot, exactly as this packet did to B2's.
var rw12ContentDigest = map[intake.Family]string{
	intake.FamilySoftware: "533bee92363337244c8f5ad09cb6c61e2e39863b2b20dbc05a20103bc8c912b1",
	intake.FamilyResearch: "03589fd135aedd7e252aec44ce7172755fa32f5263ff261bbf3d6f1989061c57",
	intake.FamilyContent:  "e25b39871edfbf1a2cbbe3ed33c42a8777450c5ce95bfda086c542e4d6d44f7f",
	intake.FamilyData:     "48797cde9c255ea5739fb0b9d88e4f32f04eda9cdbfd27ea650f2ea593de4962",
	intake.FamilyChore:    "536163f4722e32de37156fe5d4de95d385d7886449068295c55bef5c46903b24",
	intake.FamilyGeneric:  "491f40d3d8ad58f7bfa84b7e58113dff598c5a14d0cb54ba413f405f1e3af05d",
}

// verifyRW12Snapshot checks every family this packet governs against the
// ratified digest, before anything is written.
func verifyRW12Snapshot() error {
	for _, fam := range append(append([]intake.Family{}, rw12NewFamilies...), rw12SupersedeFamilies...) {
		content, err := taxonomyContent(fam)
		if err != nil {
			return err
		}
		want, ok := rw12ContentDigest[fam]
		if !ok {
			return fmt.Errorf("%w: the %s family has no ratified digest", ErrSeedDiverged, fam)
		}
		if got := contentHash(content); got != want {
			return fmt.Errorf("%w: the %s question set has changed since the P3-RW-12 gate (have %s, ratified %s) — "+
				"a later edit needs its OWN governance function and provenance record, not this one's", ErrSeedDiverged, fam, got, want)
		}
	}
	return nil
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

// taxonomyContentOf renders one question set as the strict JSON the governed
// file holds — the SAME form intake.LoadTaxonomy parses, so the governed file
// IS the operator-editable override input (CONVENTIONS §17).
func taxonomyContentOf(fam intake.Family, tax *intake.Taxonomy) (string, error) {
	if tax == nil {
		return "", fmt.Errorf("memory: no taxonomy content for the %s family", fam)
	}
	raw, err := json.MarshalIndent(tax, "", "  ")
	if err != nil {
		return "", fmt.Errorf("memory: marshal %s taxonomy: %w", fam, err)
	}
	return string(raw) + "\n", nil
}

// taxonomyContent renders the RW-12-RATIFIED content of one family — the
// frozen snapshot, never the live seed (rw12taxonomy_v2.go). This function used
// to read intake.SeedTaxonomies(), which made every RW-12 governed entry a
// pointer at whatever the code says today; P3-GF3-BE1 revises two of those sets,
// so the pointer would now write v3 content under this packet's ratification
// record. The v3 content enters as its own supersession instead (gf3seeds.go).
func taxonomyContent(fam intake.Family) (string, error) {
	return taxonomyContentOf(fam, rw12TaxonomySnapshot()[fam])
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

// TaxonomyGovernanceResult reports what one governance pass did.
type TaxonomyGovernanceResult struct {
	Created    int
	Superseded int
	// Repaired counts governed files whose bytes did not match the content
	// their own row was committed with — the torn write described on
	// committedContentHash. Non-zero is worth a human's attention: something
	// happened between placing a knowledge file and committing its row.
	Repaired int
	// Unverifiable counts governed entries whose committed content hash could
	// not be read at all — the provenance event's payload was elided by S14.9
	// compaction, or its row is gone. Those entries are LEFT ALONE: the file
	// cannot stand in for the record (that is the posture drain r1 removed),
	// so the equality check and any supersession are skipped and the boot says
	// so. It is not an error and never fails a boot.
	Unverifiable int
}

// committedContentHash returns the content hash recorded by the
// knowledge.write event that committed in the SAME transaction as this
// version's row.
//
// It exists because the FILE cannot be trusted to say what a version holds.
// writeEntry deliberately places the file BEFORE its transaction, so that a
// crash leaves an orphan file rather than a row with no content — a good
// trade, but it leaves a window in which the new bytes are on disk and the OLD
// row is still active. A boot that decided from the file would find those
// bytes equal to the in-code seed, skip the supersession, and go on skipping
// it at every future boot: the active row would attest content it does not
// have, permanently and silently (P3-RW-12 drain r1, F4). The write event
// carries the hash and commits inside the row's own transaction, so it is the
// one record that cannot be torn.
//
// ok=false means UNVERIFIABLE, and the caller must treat it as such rather
// than falling back to the file. Three ways to get there, all expected on a
// long-lived world: the provenance event's payload was elided by the S14.9
// compaction pass past ⚙ retention.compaction_horizon (knowledge events are
// keep:false, retention/keepforever.go — a compacted body carries no
// `entry_id` either, so the row simply stops matching); the row was pruned;
// or the hash was never recorded. None of them is an error and none of them
// may fail a boot — an old healthy world is the normal case here, not a
// broken one (drain r2, R1).
func (g *Gate) committedContentHash(ctx context.Context, entryID string) (string, bool, error) {
	// NullString, not string: json_extract over an elided payload yields SQL
	// NULL, and scanning that into a string is an error that would take the
	// whole boot down with it.
	var hash sql.NullString
	err := g.s.db.QueryRowContext(ctx, `
		SELECT json_extract(payload, '$.content_hash') FROM run_events
		 WHERE type = ? AND json_extract(payload, '$.entry_id') = ?
		 ORDER BY event_seq DESC LIMIT 1`, EventWrite, entryID).Scan(&hash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil // pruned, or elided so the id no longer matches
	case err != nil:
		return "", false, fmt.Errorf("memory: read committed content hash for %s: %w", entryID, err)
	case !hash.Valid || hash.String == "":
		return "", false, nil // the row is there; its body is not
	}
	return hash.String, true, nil
}

// rw12PredecessorContent renders what THIS packet's supersession replaces: the
// content the B2 gate ratified, rendered exactly as EnsureB2SeedGovernance
// writes it.
//
// A supersession trigger of "the active content is not mine" is right for the
// first boot and wrong for every boot after a LATER packet has moved the topic
// on: this Ensure would find v3 unequal to its v2 and write v2 back, the later
// one would write v3 again, and every boot would mint two versions forever. So
// an Ensure moves the record forward from the content it was written against,
// and leaves anything it does not recognize to whichever record owns it. The
// same rule keeps a boot from silently reverting an operator's own governed
// edit, which the content-only trigger did.
func rw12PredecessorContent(fam intake.Family) (string, error) {
	switch fam {
	case intake.FamilySoftware:
		return taxonomyContentOf(fam, b2SoftwareTaxonomyV1())
	case intake.FamilyGeneric:
		return taxonomyContentOf(fam, b2GenericTaxonomyV1())
	}
	return "", fmt.Errorf("memory: no B2-ratified content for the %s family", fam)
}

// repairGovernedFile rewrites a governed file to the content its row is
// committed to. It changes no row and mints no version: the record was already
// right and the disk was not, so this is not an in-place content edit (which
// S09.8 and the migration triggers both forbid) — it is making the disk agree
// with the record.
func (g *Gate) repairGovernedFile(e Entry, content string) error {
	dir, err := g.s.scopeDir(e.Scope, e.ScopeRef, e.Owner)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(g.s.root, e.FilePath), []byte(content), 0o600); err != nil {
		return fmt.Errorf("memory: repair governed file %s (dir %s): %w", e.FilePath, dir, err)
	}
	return nil
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
func (g *Gate) EnsureRW12TaxonomyGovernance(ctx context.Context) (TaxonomyGovernanceResult, error) {
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
	// Nothing is written unless the content still IS what this packet's
	// provenance record covers. All-or-nothing on purpose: partial governance
	// under a stale attestation is harder to reason about than none, and the
	// refusal is the forcing function that sends the next editor to write its
	// own Ensure.
	if err := verifyRW12Snapshot(); err != nil {
		return res, err
	}
	originRef, err := rw12OriginRef()
	if err != nil {
		return res, err
	}

	for _, fam := range rw12NewFamilies {
		entryID := taxonomyEntryID(fam)
		var n int
		if err := g.s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM knowledge_entries WHERE entry_id = ?`, entryID).Scan(&n); err != nil {
			return res, fmt.Errorf("memory: taxonomy existence check: %w", err)
		}
		if n > 0 {
			continue
		}
		content, err := taxonomyContent(fam)
		if err != nil {
			return res, err
		}
		if _, err := g.writeEntry(ctx, operator, taxonomyDraft(fam, content), writeOpts{
			origin: OriginImported, originRef: originRef, fixedID: entryID,
		}); err != nil {
			return res, fmt.Errorf("memory: govern %s taxonomy: %w", fam, err)
		}
		res.Created++
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
			return res, err
		}
		content, err := taxonomyContent(fam)
		if err != nil {
			return res, err
		}
		want := contentHash(content)
		// What this packet's supersession replaces: the content the B2 gate
		// ratified. Anything else active on this topic belongs to a record that
		// is not this one — see supersedable.
		previous, err := rw12PredecessorContent(fam)
		if err != nil {
			return res, err
		}

		// DECIDE FROM THE ROW, NEVER FROM THE FILE (drain r1, F4 — see
		// committedContentHash). HouseObject fills Content from disk, and disk
		// is the half of a governed object that a crash can leave ahead of the
		// record.
		committed, verifiable, err := g.committedContentHash(ctx, cur.ID)
		if err != nil {
			return res, err
		}
		if !verifiable {
			// The record cannot be read, so there is nothing to compare
			// against. The file is NOT a substitute — trusting it here is
			// exactly the posture that let a torn write hide forever. Skip
			// this entry, count it, and let the boot say what happened. A
			// world old enough to have compacted its provenance events is
			// healthy, not broken: it simply cannot be checked from here.
			res.Unverifiable++
			continue
		}
		onDisk := contentHash(cur.Content)
		if committed == want {
			// The RECORD is already this packet's. Nothing to supersede; a
			// disk that disagrees is repaired to what the row attests, which
			// mints no version because nothing about the record changed.
			if committed != onDisk {
				res.Repaired++
				if err := g.repairGovernedFile(cur, content); err != nil {
					return res, err
				}
			}
			continue
		}
		if committed != contentHash(previous) {
			// Not this packet's to move: the active version holds content
			// neither this record nor the one it replaces attests to — a later
			// packet's supersession, or an operator's own governed edit. Both
			// stand; superseding here would revert them at every boot.
			continue
		}
		if committed != onDisk {
			// The bytes on disk are not the bytes this row was committed
			// with. Count it so a boot says so out loud; the supersession
			// below rewrites the file anyway, which resolves the divergence
			// and records it as a version.
			res.Repaired++
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
			return res, fmt.Errorf("memory: supersede %s taxonomy: %w", fam, err)
		}
		res.Superseded++
	}
	return res, nil
}
