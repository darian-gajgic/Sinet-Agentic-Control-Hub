package verify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Answering a verification card resumes the pipeline in place (4.3; Spec
// S07.7: escalations enter the inbox as structured, RESUMABLE asks). This
// file owns the card-answer contract — the three verbs of the rework-family
// decision cards and their exact semantics — plus the resume entry into the
// S07.6 drain. Run-FSM choreography (park/resume transitions, kanban,
// receipts) is the stage layer's; the verbs' quality/record semantics are
// owned here:
//
//   - accept_best_effort: the requester takes the pinned best-effort state
//     as the outcome. The quality record stays honest — no SHIP verdict is
//     minted, work items stay done_unverified (SetVerified is the only path
//     to verified and demands a verdict evidence ref, Spec S05.1/S07.11) —
//     and the effects gate is untouched (the quality gate is never the
//     effects gate, Spec S07.1).
//   - revise_with_guidance: the guidance becomes numbered findings entering
//     the retry through the ratified requester-comment channel (Spec S07.6
//     "requester comments enter the retry through the same
//     numbered-anchored-point channel"), feeding a FRESH bounded rework
//     round; the drain re-enters with round caps and convergence checks
//     binding, and a recurrent stop re-parks with the full round history.
//   - cancel: the ratified S02.3 mapping (CONVENTIONS §14 reading 9) —
//     the parked run finalizes with its card; a running run completes with
//     the cancel reason.

// AnswerVerb is one S07.7 card answer verb.
type AnswerVerb string

const (
	VerbAcceptBestEffort   AnswerVerb = "accept_best_effort"
	VerbReviseWithGuidance AnswerVerb = "revise_with_guidance"
	VerbCancel             AnswerVerb = "cancel"
)

// answerableCategories is the B2-5 answerable card family: exactly the
// rework-family categories whose issued choices are the three verbs (Spec
// S07.7 route table — CAP-HIT first-class, plus the AC-/SANITY-BLOCKER
// judge-escalation terminals that share the same choice set and park
// shape). Other categories' verbs (REOPEN-SPEC's delta flow, the
// CHECK-INTEGRITY suite verbs, RESEARCH-NOT-RUN) land with their owning
// packets; until then their cards reject loudly — never silently absorbed.
var answerableCategories = map[Category]bool{
	CatCapHit:        true,
	CatACBlocker:     true,
	CatSanityBlocker: true,
}

// Answer is the decoded card-answer payload.
type Answer struct {
	Choice AnswerVerb `json:"choice"`
	// Guidance carries the revise_with_guidance points — the requester's
	// named problems, entering the retry as numbered findings.
	Guidance []GuidancePoint `json:"guidance,omitempty"`
	// Note is an optional free-text remark recorded with the decision.
	Note string `json:"note,omitempty"`
}

// GuidancePoint is one guidance point (the 6.2 comment ingress shape:
// numbered, optionally anchored, optionally citing a frozen criterion).
type GuidancePoint struct {
	Text      string `json:"text"`
	Anchor    string `json:"anchor,omitempty"`
	Criterion string `json:"criterion,omitempty"`
}

// Comments maps the guidance onto the S07.6 requester-comment channel. A
// guidance point always names a problem to fix, so it enters blocking.
func (a Answer) Comments() []RequesterComment {
	out := make([]RequesterComment, 0, len(a.Guidance))
	for _, g := range a.Guidance {
		out = append(out, RequesterComment{
			Text: g.Text, Anchor: g.Anchor, Criterion: g.Criterion, Blocking: true,
		})
	}
	return out
}

// Errors callers branch on (the stage surface maps them to statuses).
var (
	// ErrUnknownAsk: no OPEN ask row under that id.
	ErrUnknownAsk = errors.New("verify: no open ask with that id")
	// ErrBadAnswer covers malformed answer payloads.
	ErrBadAnswer = errors.New("verify: invalid card answer")
	// ErrUnsupportedAnswer: the verb (or the card's category) has no
	// implemented answer semantics yet — rejected loudly, never absorbed.
	ErrUnsupportedAnswer = errors.New("verify: answer not supported for this card")
	// ErrNotRequester: verification decision cards are requester cards (D10).
	ErrNotRequester = errors.New("verify: only the requester answers their decision card (D10)")
	// ErrNotResumable: the card's run is not parked on the ask (e.g. a card
	// issued before the park-on-card build against a completed run).
	ErrNotResumable = errors.New("verify: the card's run is not parked on this ask")
)

// IsVerifyAskID reports whether an ask id is verify-minted (the
// `ask-verify-` / `canary-` prefixes, CONVENTIONS §15) — the answer-surface
// routing key.
func IsVerifyAskID(id string) bool {
	return strings.HasPrefix(id, "ask-verify-") || strings.HasPrefix(id, "canary-")
}

// LoadOpenCard reads one OPEN verify ask row and decodes its card snapshot.
// The returned owner is the ask row's user_id (15.6) — the requester the
// answer must come from.
func LoadOpenCard(ctx context.Context, db *storage.DB, askID string) (Card, string, error) {
	var snapshot, owner string
	err := db.QueryRowContext(ctx,
		`SELECT snapshot, user_id FROM asks WHERE ask_id = ? AND status = 'open'`, askID).
		Scan(&snapshot, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, "", fmt.Errorf("%w: %q", ErrUnknownAsk, askID)
	}
	if err != nil {
		return Card{}, "", fmt.Errorf("verify: read ask %q: %w", askID, err)
	}
	var c Card
	if err := json.Unmarshal([]byte(snapshot), &c); err != nil {
		return Card{}, "", fmt.Errorf("verify: decode ask %q snapshot: %w", askID, err)
	}
	return c, owner, nil
}

// DecodeAnswer decodes a card-answer payload.
func DecodeAnswer(raw json.RawMessage) (Answer, error) {
	var a Answer
	if err := json.Unmarshal(raw, &a); err != nil {
		return Answer{}, fmt.Errorf("%w: %v", ErrBadAnswer, err)
	}
	return a, nil
}

// ValidateAnswer checks an answer against the durable card: the verb must
// be one of the three implemented verbs, the card's category must be in the
// answerable family, and the verb must be among the choices the card was
// issued with. revise_with_guidance requires at least one guidance point
// with text.
func ValidateAnswer(card Card, ans Answer) error {
	switch ans.Choice {
	case VerbAcceptBestEffort, VerbReviseWithGuidance, VerbCancel:
	case "":
		return fmt.Errorf("%w: missing \"choice\"", ErrBadAnswer)
	default:
		return fmt.Errorf("%w: verb %q (B2-5 implements accept_best_effort / revise_with_guidance / cancel)",
			ErrUnsupportedAnswer, ans.Choice)
	}
	if !answerableCategories[card.Category] {
		return fmt.Errorf("%w: category %s answers land with their owning packet", ErrUnsupportedAnswer, card.Category)
	}
	listed := false
	for _, c := range card.Choices {
		if c == string(ans.Choice) {
			listed = true
			break
		}
	}
	if !listed {
		return fmt.Errorf("%w: %q is not among the card's choices %v", ErrBadAnswer, ans.Choice, card.Choices)
	}
	if ans.Choice == VerbReviseWithGuidance {
		if len(ans.Guidance) == 0 {
			return fmt.Errorf("%w: revise_with_guidance needs at least one guidance point", ErrBadAnswer)
		}
		for i, g := range ans.Guidance {
			if strings.TrimSpace(g.Text) == "" {
				return fmt.Errorf("%w: guidance point %d has no text", ErrBadAnswer, i+1)
			}
		}
	}
	return nil
}

// BestEffortPin returns the card's best-effort revision pin — the final
// round's (revision, content sha) — which the resume path loads back from
// the artifact store. A card without a pinned round (or from a build that
// predates revision persistence) is not resumable.
func (c Card) BestEffortPin() (revision int, contentSHA string, err error) {
	if len(c.Rounds) == 0 {
		return 0, "", fmt.Errorf("%w: card %s carries no round history", ErrNotResumable, c.AskID)
	}
	last := c.Rounds[len(c.Rounds)-1]
	if last.Revision < 1 || last.ContentSHA == "" {
		return 0, "", fmt.Errorf("%w: card %s predates the revision-persisting build (no best-effort pin)", ErrNotResumable, c.AskID)
	}
	return last.Revision, last.ContentSHA, nil
}

// CloseAnsweredTx records the answer on the open ask row (status answered +
// payload + timestamp), inside the caller's transaction — composed with the
// run's resume/finalize transition so a card can never be consumed without
// its run acting (the intake closeAskTx shape).
func CloseAnsweredTx(ctx context.Context, tx *sql.Tx, askID string, answer json.RawMessage, now time.Time) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE asks SET status = 'answered', answered_ts = ?, answer = ? WHERE ask_id = ? AND status = 'open'`,
		now.UTC().Format(time.RFC3339Nano), string(answer), askID)
	if err != nil {
		return fmt.Errorf("verify: close ask %q: %w", askID, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("verify: close ask %q: %w", askID, err)
	} else if n != 1 {
		return fmt.Errorf("%w: %q", ErrUnknownAsk, askID)
	}
	return nil
}

// ResumeWithGuidance re-enters the S07.6 drain after a revise_with_guidance
// answer: the guidance becomes numbered blocker findings entering the RETRY
// PACKAGE (Spec S07.6 requester channel), a fresh executor session reworks
// the pinned best-effort revision, and the drain continues with the card's
// full round history carried — caps and convergence binding on the fresh
// budget, a recurrent stop re-parking with the whole record.
//
// in.Deliverable MUST be the card's pinned best-effort revision (the caller
// loads it from the artifact store; the pin hash is verified here). The
// caller has already resumed the run (parked→running, generation bumped);
// every session and event of the resumed drain rides the new generation,
// and the fence rejects any superseded stage session (Spec S02.5 step 4).
func (v *Verifier) ResumeWithGuidance(ctx context.Context, in VerifyInput, card Card, guidance []RequesterComment) (Outcome, error) {
	d := in.Deliverable
	rubric, err := v.validateInput(d)
	if err != nil {
		return Outcome{}, err
	}
	if !answerableCategories[card.Category] {
		return Outcome{}, fmt.Errorf("%w: category %s", ErrUnsupportedAnswer, card.Category)
	}
	if len(guidance) == 0 {
		return Outcome{}, fmt.Errorf("%w: revise_with_guidance needs guidance", ErrBadAnswer)
	}
	if v.Revise == nil {
		return Outcome{}, fmt.Errorf("%w: revise_with_guidance needs the fresh-session executor seam (Spec S07.6)", ErrSeamMissing)
	}
	if d.TaskID != card.TaskID || d.RunID != card.RunID {
		return Outcome{}, fmt.Errorf("%w: deliverable %s/%s does not match card %s/%s",
			ErrBadInput, d.TaskID, d.RunID, card.TaskID, card.RunID)
	}
	pinRev, pinSHA, err := card.BestEffortPin()
	if err != nil {
		return Outcome{}, err
	}
	if d.Revision != pinRev || d.SHA256() != pinSHA {
		return Outcome{}, fmt.Errorf("verify: deliverable rev%d sha %s does not match the card's best-effort pin rev%d sha %s",
			d.Revision, d.SHA256(), pinRev, pinSHA)
	}

	// The frozen ACs — the pinned §1 set every round judges against (Spec
	// S07.6 "same frozen ACs"; G1 Def.12).
	doc, _, err := currentLedgerDoc(ctx, v.Ledger, d.TaskID)
	if err != nil {
		return Outcome{}, err
	}
	acs := doc.ObjectiveAC.AcceptanceCriteria
	if len(acs) == 0 {
		return Outcome{}, fmt.Errorf("%w: task %q has no frozen ACs pinned", ErrBadInput, d.TaskID)
	}

	esc := v.escalator()
	owner, err := v.runOwner(ctx, d.RunID)
	if err != nil {
		return Outcome{}, err
	}
	// The 1.9 research gate runs per drain invocation (deterministic,
	// pre-judge); an exhausted budget cards exactly as on the fresh path.
	researchNotes, cardOut, err := v.researchGate(ctx, esc, owner, d, in.ResearchNodes)
	if err != nil {
		return Outcome{}, err
	}
	if cardOut != nil {
		return Outcome{Verdict: VerdictEscalate, Card: cardOut}, nil
	}

	startRound := len(card.Rounds) + 1
	gfs := AddRequesterFindings(nil, guidance)
	for i := range gfs {
		// Guidance always names a problem to fix: blocker, numbered, and
		// stamped with the round it feeds. The S07.5 citation rule governs
		// judge findings — the requester's own instruction enters the retry
		// as-is (Spec S07.6 6.2 channel); re-review still binds to the
		// frozen criteria, so goalposts cannot drift through it.
		gfs[i].Severity = SeverityBlocker
		gfs[i].N = i + 1
		gfs[i].Round = startRound
	}

	pkg := BuildRetryPackage(in.Spec, acs, gfs, d, startRound)
	nd, err := v.Revise(ctx, pkg)
	if err != nil {
		return Outcome{}, fmt.Errorf("verify: guidance rework round %d: %w", startRound, err)
	}
	nd = fixupRevision(nd, d)

	last := card.Rounds[len(card.Rounds)-1]
	seed := drainSeed{
		rounds:   card.Rounds,
		seenKeys: historyKeys(card.Rounds),
		stop: stopState{
			prevBlockerKeys: keySet(blockers(last.Findings)),
			prevContent:     d.Content,
		},
		guidance:      gfs,
		researchNotes: researchNotes,
	}
	return v.drain(ctx, in, nd, seed, esc, owner, rubric)
}

// historyKeys collects every finding key across a carried round history —
// the note-suppression seed of a resumed drain.
func historyKeys(rounds []RoundRecord) map[FindingKey]bool {
	keys := map[FindingKey]bool{}
	for _, r := range rounds {
		for _, f := range r.Findings {
			keys[f.Key()] = true
		}
	}
	return keys
}
