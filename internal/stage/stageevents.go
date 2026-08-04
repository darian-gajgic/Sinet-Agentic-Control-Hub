package stage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// stageevents.go is the S14.2 family-1 stage-boundary producer. The boundary is
// Skeleton.Session — the S05.3 fresh-context stage session — so EVERY engine
// stage session across every role announces itself and records how it ended:
// ceremony legs, execute steps, verify, compose, helper sessions and
// split successors alike. The intake pipeline's own phases keep their
// `intake.state` rows (family 1 already) and the role legs' kanban flips are
// untouched; those are pipeline state, not engine stage sessions.
//
// A session whose PROCESS dies gets no synthetic finish: an ending nobody
// observed is not an ending the log may assert, and the crash story is already
// told by the run-lifecycle rows (S02.5).

// The two S14.2 run-lifecycle stage types (§29: registered in
// internal/eventlog/contract.go as minted, with this file as their provenance).
const (
	EventStageStarted  = "stage.started"
	EventStageFinished = "stage.finished"
)

// The `outcome` vocabulary on stage.finished. It is closed: a session either
// completed its stage, ended at a stage-split boundary, or ended short.
const (
	// StageOutcomeCompleted — the engine finished the stage.
	StageOutcomeCompleted = "completed"
	// StageOutcomeSplit — an auto-accepted stage-split proposal executed at a
	// checkpoint boundary (Spec S05.3): not a completion and not an error; the
	// caller continues the planned stage in a successor sub-stage session.
	StageOutcomeSplit = "split"
	// StageOutcomeError — the session ended short of completion (parked on an
	// ask, ceiling hit, engine failure, driver error).
	StageOutcomeError = "error"
)

// StageEventSchemaVersion versions the stage.* payloads (schema_version 1 at
// v0, §29).
const StageEventSchemaVersion = 1

// stageDetailCap bounds the `detail` line carried on a non-happy outcome. It is
// a structural constant, not a ⚙ (Spec S18 ratifies no such key — the §7
// sseBatchSize precedent, interim under the standing settings-tab directive).
// Its reason: `detail` exists so a board card can say WHY a stage ended, not to
// carry the failure trace — the trace is the engine's own events and the
// transcript copy-aside (P-T07-5 refs-not-blobs), which is why the cap sits far
// below adapters.ExcerptCap rather than beside it.
const stageDetailCap = 512

// stageMarkerPrefix heads every stage prompt (stageMarker); the word after it
// is the session's duty.
const stageMarkerPrefix = "SINET-STAGE: "

// stageKindUnmarked labels a session whose instructions carry no stage marker.
// It is an honest absence label, never a guess at the duty.
const stageKindUnmarked = "unmarked"

// stagePayload is the S14.2 family-1 contract minimum for a stage boundary:
// stage id, stage kind, stage-brief hash — plus the outcome on the finish row.
type stagePayload struct {
	Stage     string `json:"stage"`
	Kind      string `json:"kind"`
	BriefHash string `json:"brief_hash"`
	Outcome   string `json:"outcome,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// sessionKind names the session's duty. The CALLER names it — the kind is a
// fact the dispatching stage knows, and re-reading it out of prompt text would
// let model-authored brief content name the record. A caller that names nothing
// falls back to the stage-marker line its own platform-authored instruction
// block opens with.
func sessionKind(in SessionInput) string {
	if k := strings.TrimSpace(in.Kind); k != "" {
		return k
	}
	return stageKind(in.Instructions)
}

// stageKind reads the session's duty from the stage-marker line every stage
// prompt opens with (stageMarker). An unmarked prompt reports the absence.
func stageKind(instructions string) string {
	if !strings.HasPrefix(instructions, stageMarkerPrefix) {
		return stageKindUnmarked
	}
	rest := instructions[len(stageMarkerPrefix):]
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[:i]
	}
	if k := strings.TrimSpace(rest); k != "" {
		return k
	}
	return stageKindUnmarked
}

// stageBriefHash fingerprints WHAT THIS STAGE READ: sha256 over the exact
// prompt text the session received — the assembled brief plus the stage
// instructions, or the instructions alone for an artifact-only session
// (Assemble=false: Spec S06.8 critique, S07 judge/rework). It is computed in
// ONE place, at dispatch, so every session's fingerprint is over the same
// bytes and is never empty.
//
// It deliberately does NOT reuse ledger.PlacePinned's sha256: that hash is over
// the PINNED body (ledger.PinnedText), which is a different byte string than
// the prompt, and a hash is only reusable when the hashed bytes are identical.
func stageBriefHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

// stageOutcome classifies a finished session exactly as Session's own
// disposition switch does — one place, so the recorded outcome cannot drift
// from the behavior it describes.
func stageOutcome(out adapters.Outcome, splitRequested bool) (outcome, detail string) {
	switch {
	case splitRequested && out.Kind == adapters.OutcomeParked && out.Ask == nil:
		return StageOutcomeSplit, ""
	case out.Kind != adapters.OutcomeCompleted:
		return StageOutcomeError, out.Detail
	}
	return StageOutcomeCompleted, ""
}

// appendStageStarted records the opening of one stage session.
func (s *Skeleton) appendStageStarted(ctx context.Context, r run.Run, stg, kind, briefHash string) {
	s.appendStageEvent(ctx, r, EventStageStarted, stagePayload{Stage: stg, Kind: kind, BriefHash: briefHash})
}

// appendStageFinished records how one stage session ended.
func (s *Skeleton) appendStageFinished(ctx context.Context, r run.Run, stg, kind, briefHash, outcome, detail string) {
	s.appendStageEvent(ctx, r, EventStageFinished, stagePayload{
		Stage: stg, Kind: kind, BriefHash: briefHash,
		Outcome: outcome, Detail: boundDetail(detail),
	})
}

// appendStageEvent appends one run-scoped stage row at the run's generation
// (the B5-1 envelope contract: validation, payload cap, fencing). Best-effort
// in the recordCompiled sense — a failed append is LOUD and the session still
// runs; a stage record is trace, and losing one must never fail the stage.
func (s *Skeleton) appendStageEvent(ctx context.Context, r run.Run, typ string, pay stagePayload) {
	payload, err := json.Marshal(pay)
	if err != nil {
		s.logger().Error("stage: marshal "+typ, "err", err)
		return
	}
	// The append rides context.WithoutCancel (drain D11, the §37-C audit-append
	// precedent). A session that ends BECAUSE its context was cancelled is
	// precisely the ending whose record matters most, and writing it on the
	// cancelled context would lose exactly those rows. This does not weaken
	// OQ3's rule: a session whose PROCESS dies still writes nothing, because
	// nothing is left running to write it.
	ctx = context.WithoutCancel(ctx)
	if _, err := s.cfg.Log.Append(ctx, eventlog.Append{
		RunID: r.ID, Generation: r.Generation, UserID: r.UserID,
		Type: typ, SchemaVersion: StageEventSchemaVersion, Payload: payload,
	}); err != nil {
		s.logger().Error("stage: append "+typ, "run", r.ID, "stage", pay.Stage, "err", err)
	}
}

// boundDetail truncates on a RUNE boundary (drain D7). Cutting at a byte offset
// can split a multi-byte rune, and the fragment marshals as U+FFFD — a payload
// whose last character is corruption, in the one field that exists to say why a
// stage ended.
func boundDetail(s string) string {
	if len(s) <= stageDetailCap {
		return s
	}
	cut := stageDetailCap
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
