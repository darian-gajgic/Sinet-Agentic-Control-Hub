package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// tq4governance.go — the P3-TQ-4a composer-playbook governance leg (Spec
// S09.10, S09.4 station 4, S09.8), minted rather than extended.
//
// CONVENTIONS §57 leaves one instruction for the packet that edits a governed
// object: do not update the previous record's digests and do not extend its
// Ensure — mint your own, with your own provenance. This is that, for the
// composer playbook. The seed moves to seed-2, which adds the web-deliverable
// feedback section [A16, 2026-09-17], and this record supersedes the B3 entry
// to it. The B3 version stays in the record, retired, still attributed to the
// gate that approved it (S09.8 — edits are new versions, never in-place
// mutation).
//
// Its predecessor is a FROZEN snapshot (tq4playbook_seed1.go), so "what does
// this replace" is answered by a constant rather than by whatever the code
// says today.

// tq4GateProvenance is the ratification record the P3-TQ-4a playbook entry
// carries. It is a separate type from the identical-looking ones before it on
// purpose: a provenance record belongs to the packet that wrote it, and
// sharing the struct is the first step towards sharing the record.
type tq4GateProvenance struct {
	Packet   string `json:"packet"`
	Record   string `json:"record"`
	Decision string `json:"decision"`
	Ratified string `json:"ratified"`
}

func tq4OriginRef() (string, error) {
	raw, err := json.Marshal(tq4GateProvenance{
		Packet: "P3-TQ-4a",
		Record: "Spec/drafts/S00-front-matter.md — amendment A16, 2026-09-17; P3/briefs/P3-TQ-4.md — the grounding brief; " +
			"P3/design/taskquality-webshop-findings-2026-09-16.md — finding TQ-F7",
		Decision: "composer playbook seed-2: a Web deliverables section stating four rules for work that produces a page or " +
			"an app — correctness feedback is in the document on the first frame rather than delivered by an animation " +
			"completing; one view never mounts behind another view's exit transition; the user's reduced-motion preference " +
			"collapses transitions to an immediate state change; and every action leaves state observable in the document, " +
			"so a cart badge is a number and not only a pop. The evidence is the sitting's own webshop task, where the app's " +
			"state was entirely correct while the screen showed nothing, and a person reading it called the work broken. The " +
			"last rule is also what makes this section and the platform's own acceptance walk agree: the walk asserts on " +
			"document state and never on rendered pixels. Nothing else in the playbook moves — every other section is " +
			"verbatim seed-1. The reach is INDIRECT and the section says so: the playbook steers the S08.6 composer, the " +
			"composer drafts the worker template, and the template is what reaches an executor.",
		Ratified: "PENDING operator ratification at the P3-TQ-4 packet gate. Seed-content ratification for this object was a B3 " +
			"gate item and this revision inherits that posture: the content is governed and readable, and the gate is owed.",
	})
	if err != nil {
		return "", fmt.Errorf("memory: marshal P3-TQ-4a playbook provenance: %w", err)
	}
	return string(raw), nil
}

// tq4PlaybookDigest pins the EXACT governed content this packet's originRef
// attests to: sha256 over the seed-2 playbook bytes.
//
// Same doctrine as rw12ContentDigest and the taxonomy digests after it,
// applied to this packet's own record. Without it the Ensure below would write
// whatever ComposerPlaybookSeed() returns under a fixed ratification record, so
// a later packet editing the playbook would have its content superseded into
// governance at the next boot, attributed to a gate that never read it, with
// nothing anywhere to notice.
//
// FOR THE NEXT PACKET THAT EDITS THE PLAYBOOK: do not update this digest, and
// do not extend this Ensure. Freeze this content as a snapshot the way
// tq4playbook_seed1.go freezes B3's, mint your own Ensure with your own
// provenance, and call it after this one.
// A var, not a const, so the drift refusal itself is testable — the v41/GF7
// precedent one file over.
var tq4PlaybookDigest = "ce90c1cb3b8b5daed72cbacfda0a1492dd7746767a6c9add430bf0f4a2d08dd5"

// verifyTQ4Snapshot checks the revised playbook against the ratified digest,
// before anything is written. Divergence is ErrSeedDiverged — the same loud,
// never-fatal skip its predecessors take, and the same forcing function: the
// next editor writes its own Ensure instead of quietly borrowing this record.
func verifyTQ4Snapshot() error {
	if got := contentHash(worker.ComposerPlaybookSeed()); got != tq4PlaybookDigest {
		return fmt.Errorf("%w: the composer playbook has changed since the P3-TQ-4a record was written (have %s, recorded %s) — "+
			"a later edit needs its OWN governance function and provenance record, not this one's", ErrSeedDiverged, got, tq4PlaybookDigest)
	}
	return nil
}

// PlaybookGovernanceResult reports what one composer-playbook governance pass
// did. Booleans rather than counts, because this leg governs exactly one
// object. Repaired and Unverifiable carry the same meanings they do on
// TaxonomyGovernanceResult: a governed file whose bytes did not match the
// content its own row was committed with, and a row whose committed content
// hash could not be read at all (compacted or pruned provenance — a healthy
// old world, not a broken one).
type PlaybookGovernanceResult struct {
	Superseded   bool
	Repaired     bool
	Unverifiable bool
}

// EnsureTQ4KnowledgeGovernance brings the seed-2 composer playbook under S09.10
// governance, idempotently.
//
// It runs AFTER EnsureComposerPlaybook, which is what puts the version it
// supersedes there. A world that already holds seed-1 receives seed-2 by this
// path at its next boot; a fresh world gets seed-1 from the B3 record first and
// seed-2 from this one immediately after, so the version chain reads true
// either way.
//
// House scope needs its D10 holder: with no operator account the call returns
// ErrNoOperator and the caller retries at a later boot.
func (g *Gate) EnsureTQ4KnowledgeGovernance(ctx context.Context) (PlaybookGovernanceResult, error) {
	var res PlaybookGovernanceResult
	var operator string
	err := g.s.db.QueryRowContext(ctx,
		`SELECT user_id FROM users WHERE role = 'operator' ORDER BY created_ts, user_id LIMIT 1`).Scan(&operator)
	if err == sql.ErrNoRows {
		return res, ErrNoOperator
	}
	if err != nil {
		return res, fmt.Errorf("memory: resolve operator: %w", err)
	}
	if err := verifyTQ4Snapshot(); err != nil {
		return res, err
	}
	originRef, err := tq4OriginRef()
	if err != nil {
		return res, err
	}

	cur, err := g.s.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
	if errors.Is(err, ErrNotFound) {
		// Nothing active to supersede: either the B3 seeding has not run yet
		// (it runs first at boot) or the operator removed the object. Neither
		// is this function's to repair.
		return res, nil
	}
	if err != nil {
		return res, err
	}
	content := worker.ComposerPlaybookSeed()
	want := contentHash(content)

	// DECIDE FROM THE ROW, NEVER FROM THE FILE (see committedContentHash).
	// HouseObject fills Content from disk, and disk is the half of a governed
	// object that a crash can leave ahead of the record.
	committed, verifiable, err := g.committedContentHash(ctx, cur.ID)
	if err != nil {
		return res, err
	}
	if !verifiable {
		res.Unverifiable = true
		return res, nil
	}
	onDisk := contentHash(cur.Content)
	if committed == want {
		// The record already holds this packet's content; only the disk may
		// still need putting right. The repair runs whatever the version
		// history says, because a torn file is not a decision.
		if committed != onDisk {
			res.Repaired = true
			if err := g.repairGovernedFile(cur, content); err != nil {
				return res, err
			}
		}
		return res, nil
	}
	// Applied once, and later decisions stand (see decisionRecorded).
	recorded, err := g.decisionRecorded(ctx, worker.ComposerPlaybookTopicKey, originRef)
	if err != nil {
		return res, err
	}
	if recorded {
		return res, nil
	}
	if committed != contentHash(tq4PlaybookSeed1()) {
		// Not this packet's to move: the active version holds content neither
		// this record nor the one it replaces attests to — a later packet's
		// supersession, or an operator's own governed edit. Both stand;
		// superseding here would revert them at every boot.
		return res, nil
	}
	if committed != onDisk {
		res.Repaired = true
	}
	if _, err := g.writeEntry(ctx, operator, Draft{
		Scope: ScopeHouse, Kind: KindPlaybook, Title: worker.ComposerPlaybookTitle,
		Content:  content,
		TopicKey: worker.ComposerPlaybookTopicKey,
		// Machinery consumer: the S08.6 composer reads it by topic key; it
		// never injects into stage briefs.
		Selectors:  Selectors{TaskType: "machinery:worker-composer"},
		FileBacked: true, FileName: "composer-playbook.md",
	}, writeOpts{
		origin: OriginImported, originRef: originRef, supersedes: cur.ID,
	}); err != nil {
		return res, fmt.Errorf("memory: supersede composer playbook: %w", err)
	}
	res.Superseded = true
	return res, nil
}
