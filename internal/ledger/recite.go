package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// Recitation (Spec S05.3): in long stages the platform re-reads the
// ledger's state/next_actions every ⚙ context.recitation_interval_turns
// and delivers the re-read into the running session. This file owns the
// ledger's two halves of that mechanism: the platform-authored recitation
// body (RecitationText — content by pure lookup at a pinned revision,
// Research/18 §7-C1 condition 2) and the delivery manifest
// (AppendRecitationManifest — every mid-stage injection is manifested,
// Spec S05.4; condition 3: nothing silent). Dueness computation and the
// pending-file transport live in the stage runtime; the delivery valve in
// the engine adapter.

// Recitation manifest dispositions (ManifestEntry.Disposition on the
// recitation entry; empty = the delivered bytes match the platform
// rendering at the recorded revision).
const (
	// RecitationHashMismatch: the delivered content hash does not equal
	// the platform re-rendering at the recorded ledger_version — the fire
	// claims bytes the platform never authored (e.g. in-sandbox tampering
	// with the pending file). Recorded loudly, never dropped.
	RecitationHashMismatch = "hash_mismatch"
	// RecitationUnknownVersion: the fire cites a ledger_version the event
	// stream does not contain.
	RecitationUnknownVersion = "unknown_version"
)

// RecitationText renders the platform-authored recitation body from a
// ledger document: the §4 state section and next_actions, headed by the
// task and revision identity (Spec S05.3 "re-reads the ledger's
// state/next_actions"). Deterministic over the document — the manifest
// path re-renders at the pinned revision and compares hashes, so the
// rendering carries no clock or environment input.
func RecitationText(doc Document) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "SINET RECITATION — platform re-read of the task ledger (Spec S05.3)\n")
	fmt.Fprintf(&sb, "task: %s  ledger_version: %d\n\n", doc.TaskID, doc.LedgerVersion)
	sb.WriteString("## state\n")
	if doc.State.Current != "" {
		fmt.Fprintf(&sb, "current: %s\n", doc.State.Current)
	}
	if len(doc.State.Items) > 0 {
		sb.WriteString("items:\n")
		for _, it := range doc.State.Items {
			fmt.Fprintf(&sb, "- [%s] %s — %s\n", it.Status, it.ID, it.Summary)
		}
	}
	if len(doc.State.Blockers) > 0 {
		sb.WriteString("blockers:\n")
		for _, b := range doc.State.Blockers {
			fmt.Fprintf(&sb, "- %s\n", b)
		}
	}
	sb.WriteString("\n## next_actions\n")
	if len(doc.State.NextActions) == 0 {
		sb.WriteString("(none recorded)\n")
	}
	for _, n := range doc.State.NextActions {
		fmt.Fprintf(&sb, "- %s\n", n)
	}
	return sb.String()
}

// AppendRecitationManifest records one recitation DELIVERY in the run
// trace (Spec S05.4 "entries for any mid-stage injection"; Research/18
// §7-C1 condition 3: every delivery manifested and journaled as run
// events). The inputs are the delivery-valve fire facts — the pinned
// ledger_version the pending file carried and the hash the hook recomputed
// over the bytes it actually emitted. The platform re-renders the
// recitation at that revision and compares: a mismatch or an unknown
// revision is recorded as the entry's Disposition, never silently
// accepted and never dropped.
func (s *Store) AppendRecitationManifest(ctx context.Context, runID, stage string, ledgerVersion int64, contentSHA256, sessionID, toolUseID string) (int64, error) {
	if contentSHA256 == "" {
		return 0, fmt.Errorf("%w: recitation manifest without a content hash", ErrInvalidWrite)
	}
	var seq int64
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		r, err := readRun(ctx, tx, runID)
		if err != nil {
			return err
		}
		disposition := ""
		doc, err := s.atVersionTx(ctx, tx, r.taskID, ledgerVersion)
		switch {
		case errors.Is(err, ErrVersionNotFound), errors.Is(err, ErrNoLedger):
			disposition = RecitationUnknownVersion
		case err != nil:
			return err
		default:
			sum := sha256.Sum256([]byte(RecitationText(doc)))
			if hex.EncodeToString(sum[:]) != contentSHA256 {
				disposition = RecitationHashMismatch
			}
		}
		payload, err := json.Marshal(manifestPayload{
			Kind: "recitation", Stage: stage, TaskID: r.taskID,
			LedgerVersion: ledgerVersion, SessionID: sessionID, ToolUseID: toolUseID,
			Items: []ManifestEntry{{
				ItemID:          "ledger.recitation",
				SourcePath:      fmt.Sprintf("ledger://%s@v%d#state", r.taskID, ledgerVersion),
				ContentHash:     contentSHA256,
				Version:         fmt.Sprintf("v%d", ledgerVersion),
				SelectorRule:    "recitation (Spec S05.3, ⚙ context.recitation_interval_turns)",
				PrecedenceLabel: string(PrecedenceTask),
				Disposition:     disposition,
			}},
		})
		if err != nil {
			return fmt.Errorf("ledger: marshal recitation manifest: %w", err)
		}
		seq, err = s.log.AppendTx(ctx, tx, eventlog.Append{
			RunID:         runID,
			Generation:    r.generation,
			UserID:        r.userID,
			Type:          EventContextManifest,
			SchemaVersion: contextManifestSchemaVersion,
			Payload:       payload,
		})
		return err
	})
	if err != nil {
		return 0, err
	}
	return seq, nil
}
