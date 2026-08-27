package verify

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// Recording (Spec S07.11): every verdict is recorded with its reasons, per
// round — what was checked, against which criteria (AC ids + rubric version
// + judge model + its current golden-set error rates), what passed, what
// failed, what was flagged note-vs-blocker — as event-log rows, so
// progression across revisions is visible. Verdicts are KEEP-FOREVER (G2
// Def.11): the 11.1 compaction horizon never touches verify.round rows
// (retention machinery is Spec S14's; the class is noted here and on the
// payload). Verdicts also land in the ledger's verified-status section via
// SetVerified (G1 Def.12), and every V2 verdict is checkpointed through the
// adapter Driver when judge sessions become real paid calls (D7; B2-4).
//
// Event type names are provisional pending the S14 event contract (B5),
// extending the CONVENTIONS §7/§8/§13/§14 naming note.
const (
	// EventV0 records one V0 pre-gate execution (verdicts AND tool-failure
	// incidents — the fail-open evidence, Spec S07.2).
	EventV0 = "verify.v0"
	// EventV1 records one V1 check-pack execution with evidence refs
	// (refs-not-blobs, P-T07-5).
	EventV1 = "verify.v1"
	// EventRound is the per-round verdict record — the keep-forever class
	// (G2 Def.11).
	EventRound = "verdict.recorded"
	// EventEscalation records one routed escalation with its ask id.
	EventEscalation = "verify.escalation"
)

// verifyEventSchemaVersion versions the verify.* payloads.
const verifyEventSchemaVersion = 1

// GoldenSetRates is the judge's current golden-set error profile, carried
// on every verdict record (Spec S07.11): judge-reported rates are
// statistically corrected rather than taken raw, and an unmeasured judge
// says so honestly (the TPR/TNR measurement is the B4 deferral).
type GoldenSetRates struct {
	TPR      *float64 `json:"tpr,omitempty"`
	TNR      *float64 `json:"tnr,omitempty"`
	Measured bool     `json:"measured"`
	// MeasuredOn dates the last golden-set run (P-T06-5: any judge-model
	// change gates on a re-run before unsupervised judging resumes).
	MeasuredOn string `json:"measured_on,omitempty"`
}

// roundPayload is the verify.round event payload (Spec S07.11 contract).
type roundPayload struct {
	Round   int     `json:"round"`
	Verdict Verdict `json:"verdict"`

	// Against which criteria.
	ACIDs         []string `json:"ac_ids"`
	RubricID      string   `json:"rubric_id,omitempty"`
	RubricVersion int      `json:"rubric_version,omitempty"`
	JudgeModel    string   `json:"judge_model,omitempty"`
	// SelfFamilyJudge is always flagged (G1 Def.1); it also rides the
	// receipt (Spec S10).
	SelfFamilyJudge bool           `json:"self_family_judge,omitempty"`
	GoldenSet       GoldenSetRates `json:"golden_set"`

	// What passed / failed / was unknown (axis 1, per AC id).
	Passed  []string `json:"passed,omitempty"`
	Failed  []string `json:"failed,omitempty"`
	Unknown []string `json:"unknown,omitempty"`

	// Findings with their note-vs-blocker flags, verbatim.
	Findings        []Finding `json:"findings,omitempty"`
	SuppressedNotes int       `json:"suppressed_notes,omitempty"`
	Axis2Skipped    string    `json:"axis2_skipped,omitempty"`

	// StaleAudit flags a check suite past its audit interval (P-T06-1).
	StaleAudit bool `json:"stale_audit,omitempty"`

	// Posture / PostureNote / ReviewMandatory mark a round the domain's check
	// pack did not decide (Spec S07.8 [A14, 2026-08-27]): the verdict is
	// advisory and non-authoritative, the plain-words disclosure is carried
	// verbatim, and requester review is the real gate. Absent for the ordinary
	// posture, so progression across revisions shows exactly which rounds were
	// advisory — and a later completion surface reads the obligation off the
	// row rather than re-deriving it.
	Posture         Posture `json:"posture,omitempty"`
	PostureNote     string  `json:"posture_note,omitempty"`
	ReviewMandatory bool    `json:"review_mandatory,omitempty"`

	Domain     string `json:"domain"`
	Revision   int    `json:"revision"`
	ContentSHA string `json:"content_sha256"`
	// Entailment records the coverage posture ("idle" at v0 for the
	// software domain — Spec S07.4).
	Entailment string `json:"entailment,omitempty"`
	// Retention notes the keep-forever class on the row itself (G2 Def.11).
	Retention string `json:"retention"`
}

// retentionKeepForever is the G2 Def.11 class marker on verdict rows.
const retentionKeepForever = "keep-forever"

// Recorder appends the verify.* event rows. All appends are control-plane
// acts at the run's current generation, attributed to the run's owner
// (15.6; the CONVENTIONS §13 fencing split).
type Recorder struct {
	DB  *storage.DB
	Log *eventlog.Log
}

// append writes one verify event in its own write transaction and returns
// its event_seq.
func (r *Recorder) append(ctx context.Context, runID, eventType string, payload any) (int64, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("verify: marshal %s: %w", eventType, err)
	}
	var seq int64
	err = r.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		var (
			gen    int64
			userID string
		)
		if err := tx.QueryRowContext(ctx,
			`SELECT generation, user_id FROM runs WHERE run_id = ?`, runID).Scan(&gen, &userID); err != nil {
			return fmt.Errorf("verify: read run %q: %w", runID, err)
		}
		seq, err = r.Log.AppendTx(ctx, tx, eventlog.Append{
			RunID:         runID,
			Generation:    gen,
			UserID:        userID,
			Type:          eventType,
			SchemaVersion: verifyEventSchemaVersion,
			Payload:       raw,
		})
		return err
	})
	if err != nil {
		return 0, err
	}
	return seq, nil
}

// RecordV0 appends the verify.v0 row (verdict or fail-open incidents).
func (r *Recorder) RecordV0(ctx context.Context, runID string, res V0Result) (int64, error) {
	return r.append(ctx, runID, EventV0, res)
}

// RecordV1 appends the verify.v1 row.
func (r *Recorder) RecordV1(ctx context.Context, runID string, res V1Result) (int64, error) {
	return r.append(ctx, runID, EventV1, res)
}

// RecordRound appends the keep-forever verify.round verdict row (Spec
// S07.11).
func (r *Recorder) RecordRound(ctx context.Context, runID string, d Deliverable, rec RoundRecord, meta JudgeMeta, rates GoldenSetRates, rubricID string, rubricVersion int, acIDs []string, entailment string) (int64, error) {
	p := roundPayload{
		Round:           rec.Round,
		Verdict:         rec.Verdict,
		ACIDs:           acIDs,
		RubricID:        rubricID,
		RubricVersion:   rubricVersion,
		JudgeModel:      meta.Model,
		SelfFamilyJudge: meta.SelfFamily,
		GoldenSet:       rates,
		Findings:        rec.Findings,
		SuppressedNotes: rec.SuppressedNotes,
		Axis2Skipped:    rec.Axis2Skipped,
		Domain:          d.Domain,
		Revision:        d.Revision,
		ContentSHA:      rec.ContentSHA,
		Entailment:      entailment,
		Posture:         rec.Posture,
		PostureNote:     rec.PostureNote,
		ReviewMandatory: rec.ReviewMandatory,
		Retention:       retentionKeepForever,
	}
	if rec.V1 != nil {
		p.StaleAudit = rec.V1.StaleAudit
	}
	for _, v := range rec.Axis1 {
		switch {
		case v.Unknown:
			p.Unknown = append(p.Unknown, v.Key)
		case v.Pass:
			p.Passed = append(p.Passed, v.Key)
		default:
			p.Failed = append(p.Failed, v.Key)
		}
	}
	return r.append(ctx, runID, EventRound, p)
}

// VerifyLedgerItems flips the ledger's verified status for every work item
// the SHIP verdict verifies (Spec S07.11; G1 Def.12): a done_unverified
// item whose cited ACs all passed becomes verified, evidenced by the
// verdict row ("run_events/<seq>"). SetVerified is the ONLY path to
// verified status; items without AC refs are not AC-satisfying and stay for
// the stage-close gate.
func VerifyLedgerItems(ctx context.Context, store *ledger.Store, runID, taskID string, verdicts []ACVerdict, verdictSeq int64) ([]string, error) {
	doc, found, err := store.Current(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("verify: read task ledger: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("%w: task %q has no ledger", ErrBadInput, taskID)
	}
	passed := map[int]bool{}
	for _, v := range verdicts {
		if !v.Unknown && v.Pass {
			var n int
			if _, err := fmt.Sscanf(v.Key, "AC-%d", &n); err == nil {
				passed[n] = true
			}
		}
	}
	evidence := fmt.Sprintf("run_events/%d", verdictSeq)
	var verified []string
	for _, item := range doc.State.Items {
		if item.Status != ledger.StatusDoneUnverified || len(item.ACRefs) == 0 {
			continue
		}
		all := true
		for _, n := range item.ACRefs {
			if !passed[n] {
				all = false
				break
			}
		}
		if !all {
			continue
		}
		if _, err := store.SetVerified(ctx, runID, ledger.AuthorPlatform, item.ID, evidence); err != nil {
			return verified, fmt.Errorf("verify: set verified %q: %w", item.ID, err)
		}
		verified = append(verified, item.ID)
	}
	return verified, nil
}
