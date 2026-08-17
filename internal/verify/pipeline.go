package verify

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// The verification drain (Spec S07.1 cascade + S07.6 rework + S07.7
// escalation): V0 kills broken output before any paid call; V1 executes the
// domain pack; V2 judges both axes; blockers drain through round-capped
// rework; every terminal short of SHIP is a human-visible card. Cost shape
// (Spec S07.11): per round exactly one compliance call and at most one
// sanity call — ≤ rounds × 2 judge calls; V0/V1/entailment run free.
// Verification consumption is ceremony-billed and itemized through the
// checkpoint purpose tags when judge sessions become real paid calls
// (Spec S10; B2-4).

// v0RegenerationLimit is spec-structural, not ⚙: Spec S07.2 fixes "it costs
// ONE regeneration, never a judge call" in ratified text; S18 declares no
// key (the CONVENTIONS §7/§14 precedent).
const v0RegenerationLimit = 1

// WorkspaceProvider is the S13 seam (B4): it materializes the verification
// workspace for a revision — the deliverable tree with answer-bearing VCS
// history STRIPPED (Spec S07.3 rule 1, P-T06-2). Until S13 lands, callers
// hand a prepared directory via VerifyInput.Workspace.
type WorkspaceProvider interface {
	VerificationWorkspace(ctx context.Context, d Deliverable) (dir string, cleanup func(), err error)
}

// Verifier is the S07 pipeline composition root. Model duties and
// out-of-section machinery ride seams (the S06.10 pattern): absent REQUIRED
// seams fail loudly (ErrSeamMissing) — verification never silently skips a
// layer; absent OPTIONAL seams degrade exactly as the spec directs, with
// the degradation recorded.
type Verifier struct {
	DB       *storage.DB
	Log      *eventlog.Log
	Ledger   *ledger.Store
	Settings Settings

	// Judge is the V2 engine seam (required; Spec S07.5 — selection is
	// S08's, session wiring B2-4).
	Judge Judge
	// Rubric is the versioned rubric bundle for the deliverable's domain
	// (required for V2; Spec S07.10). Nil = SeedSoftwareRubric().
	Rubric *RubricBundle
	// Pack is the domain check pack (required for launch domains — Spec
	// S07.8 graduation; a nil pack outside launch domains is the degraded
	// mode of Spec S07.8, V1 empty, arriving v1).
	Pack *CheckPack
	// Runner executes checks in the network-off sandbox (required when a
	// pack is present).
	Runner CheckRunner
	// Workspace is the S13 seam; nil uses VerifyInput.Workspace.
	Workspace WorkspaceProvider

	// Research is the S10 metering seam for the did-research check; nil
	// records UNVERIFIABLE-HERE loudly (the per-step counter substrate is
	// B2-4's).
	Research ResearchUsage
	// ResearchRerun re-runs one research node in a fresh session (B2-4
	// wiring); nil = a rerun-requiring node goes straight to its card —
	// never a silent pass.
	ResearchRerun func(ctx context.Context, node intake.ResearchNode) error

	// Revise is the fresh-session executor seam for rework rounds (Spec
	// S07.6; B2-4 wiring). It MUST build a fresh session from the retry
	// package — fork-don't-poison. Nil = REVISE verdicts escalate.
	Revise func(ctx context.Context, pkg RetryPackage) (Deliverable, error)
	// Regenerate is the V0 hard-kill regeneration seam (one shot, Spec
	// S07.2). Nil = a malformed artifact escalates directly.
	Regenerate func(ctx context.Context, d Deliverable, reasons []string) (Deliverable, error)

	// Tickets is the S08 worker-flaw ticket seam (B3); nil files to the
	// inbox via the stub.
	Tickets FlawTickets
	// Entailment ships idle at v0 (Spec S07.4); nil is an inactive gate.
	Entailment *EntailmentGate

	// Review is the S13 review-schema seam (B4): revision minting at the
	// handoff, findings-as-comments, guidance ingress, and THE S13.4
	// drain. Nil = the pre-S13 in-memory channel (see ReviewSink).
	Review ReviewSink

	// PreGates overrides the V0 gate set (nil = DefaultPreGates) — the
	// extension point per-deployment shape checks ride.
	PreGates []PreGate

	Now func() time.Time
}

func (v *Verifier) preGates() []PreGate {
	if v.PreGates != nil {
		return v.PreGates
	}
	return DefaultPreGates(nil)
}

func (v *Verifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

func (v *Verifier) recorder() *Recorder { return &Recorder{DB: v.DB, Log: v.Log} }
func (v *Verifier) escalator() *Escalator {
	return &Escalator{DB: v.DB, Log: v.Log, Settings: v.Settings, Tickets: v.Tickets, Now: v.Now}
}

// VerifyInput is one deliverable verification request.
type VerifyInput struct {
	Deliverable Deliverable
	// Spec is the approved SPEC artifact — the retry-package member (Spec
	// S07.6).
	Spec intake.Spec
	// Steps are the approved PLAN steps (per-step Done-when contracts,
	// Spec S07.3).
	Steps []intake.Step
	// ResearchNodes are the PLAN's declared research nodes (1.9).
	ResearchNodes []intake.ResearchNode
	// Tier is the stakes tier (axis-2 gating outside launch domains, Spec
	// S07.8).
	Tier intake.Tier
	// Comments are requester review comments entering the rework channel
	// (6.2; Spec S07.6).
	Comments []RequesterComment
	// Workspace/EvidenceDir are the dev-mode paths until the S13 seam
	// lands.
	Workspace   string
	EvidenceDir string
}

// Outcome is the drain result. Exactly one of the terminal shapes holds:
// SHIP/SHIP-with-notes (VerifiedItems set), or a Card (every other
// terminal — Spec S07.7: no other sink type exists).
type Outcome struct {
	Verdict       Verdict
	Rounds        []RoundRecord
	Card          *Card
	VerifiedItems []string
	// IntegrityCards are CHECK-INTEGRITY cards raised mid-drain (the drain
	// continues past them; the mechanical fact stands).
	IntegrityCards []Card
}

// tierRank orders the S06.4 stakes tiers for the ⚙ sanity_stakes_floor
// comparison (the enum is intake's; the ordering is ratified there).
var tierRank = map[intake.Tier]int{
	intake.TierTrivial: 0, intake.TierLow: 1, intake.TierStandard: 2, intake.TierHigh: 3,
}

// axis2Required applies the S07.8 scope rules: launch domains always;
// steered output always (4.7 backstop); elsewhere stakes tier ≥
// ⚙ verification.sanity_stakes_floor.
func (v *Verifier) axis2Required(d Deliverable, tier intake.Tier) (bool, string, error) {
	if d.Steered {
		return true, "", nil
	}
	if LaunchDomain(d.Domain) {
		return true, "", nil
	}
	floor, err := v.Settings.String(keySanityStakesFloor)
	if err != nil {
		return false, "", fmt.Errorf("verify: read ⚙ %s: %w", keySanityStakesFloor, err)
	}
	if tierRank[tier] >= tierRank[intake.Tier(floor)] {
		return true, "", nil
	}
	return false, fmt.Sprintf("stakes-gated: domain %q is not a launch domain and tier %q is below ⚙ %s=%q (Spec S07.8)",
		d.Domain, tier, keySanityStakesFloor, floor), nil
}

// entailmentPosture names the round record's entailment status (Spec
// S07.4).
func (v *Verifier) entailmentPosture(domain string) string {
	if v.Entailment.Active(domain) {
		return "active"
	}
	return "idle (v0 launch domain is software; activates with web-research at v0.1 behind the TBD-BRINGUP calibration bar)"
}

// PreambleRefusal marks the DETERMINISTIC verification-infrastructure refusal
// class: the drain refused to START because a piece of verification
// infrastructure is absent — no domain check pack for a launch domain, a
// required seam unwired, an invalid pack, an unvalidated judge seat.
//
// The class exists because its disposition differs from every other drain
// error (P3-RW-14 R4). A missing screen is a SCREEN OUTAGE, and "a screen
// never blocks on its own failure, a screen outage escalates rather than
// approves" (Spec S07.2): it terminates in a durable decision card the owner
// can answer, never in a crash the recovery ladder re-forks into the same
// refusal until it tombstones the lineage (the live 2026-08-16 wedge). A
// MECHANICAL failure mid-drain — a DB error, a judge transport error — still
// crashes for the ladder, because a fork of THAT may well succeed.
//
// The marker rides the preamble only, so classification never has to guess
// from an error's text (CONVENTIONS §38): errors.Is still sees the underlying
// sentinel, and a mid-drain error wrapping the same sentinel is NOT marked.
type PreambleRefusal struct{ Err error }

func (e *PreambleRefusal) Error() string { return e.Err.Error() }
func (e *PreambleRefusal) Unwrap() error { return e.Err }

// NewPreambleRefusal marks err as the preamble refusal class. It is exported
// for the ONE other site that refuses before the drain can start: the stage
// layer's judge-construction gate (P-T06-5, Spec S14.8) and its check-pack
// resolution seam, both of which run before a Verifier exists.
func NewPreambleRefusal(err error) error {
	if err == nil {
		return nil
	}
	return &PreambleRefusal{Err: err}
}

// AsPreambleRefusal reports whether err is a verification-infrastructure
// refusal, and returns it.
func AsPreambleRefusal(err error) (*PreambleRefusal, bool) {
	var ref *PreambleRefusal
	if errors.As(err, &ref) {
		return ref, true
	}
	return nil, false
}

// validateInput checks the drain's structural preconditions and resolves
// the rubric bundle (shared by the fresh entry and the S07.7 resume entry).
// The three infrastructure absences are marked as the preamble refusal class;
// a malformed deliverable is NOT — that is a caller defect, not an outage.
func (v *Verifier) validateInput(d Deliverable) (*RubricBundle, error) {
	if d.TaskID == "" || d.RunID == "" || d.Domain == "" || d.Revision < 1 {
		return nil, fmt.Errorf("%w: deliverable requires task, run, domain, revision", ErrBadInput)
	}
	if v.Judge == nil {
		return nil, NewPreambleRefusal(fmt.Errorf("%w: V2 requires the Judge seam (Spec S07.5)", ErrSeamMissing))
	}
	rubric := v.Rubric
	if rubric == nil {
		rubric = SeedSoftwareRubric()
	}
	if LaunchDomain(d.Domain) && v.Pack == nil {
		return nil, NewPreambleRefusal(ErrNoCheckPack)
	}
	if v.Pack != nil && v.Runner == nil {
		return nil, NewPreambleRefusal(fmt.Errorf("%w: a check pack requires a CheckRunner (Spec S07.3)", ErrSeamMissing))
	}
	return rubric, nil
}

// drainSeed carries a drain invocation's starting state. The fresh entry
// (Verify) seeds only research notes and requester comments; the S07.7
// resume entry (ResumeWithGuidance) additionally carries the answered
// card's round history, its finding keys, the pre-park convergence state,
// and the guidance findings for the judge's prior-findings scope.
type drainSeed struct {
	// rounds is the carried round history: numbering continues from it and
	// every card the drain raises carries the FULL history (Spec S07.6
	// "never a silent stop"; the resume re-park keeps the whole record).
	rounds []RoundRecord
	// seenKeys seeds the note-suppression key set (Spec S07.6: after round
	// 1, new note-class findings are suppressed to a count — structurally
	// past round 1, a resumed drain suppresses from its first round).
	seenKeys map[FindingKey]bool
	// stop seeds the convergence state from the pre-park final round, so
	// model inertia across the park still trips the stop rule.
	stop stopState
	// guidance rides the judge input's prior-findings scope (Spec S07.6
	// "prior findings in scope") without counting as current-round findings
	// — the guidance described the PRE-rework artifact and already fed the
	// retry package; re-merging it into post-rework rounds would force
	// REVISE unconditionally.
	guidance      []Finding
	researchNotes []Finding
	comments      []RequesterComment
}

// Verify runs the full drain for one deliverable revision.
func (v *Verifier) Verify(ctx context.Context, in VerifyInput) (Outcome, error) {
	d := in.Deliverable
	rubric, err := v.validateInput(d)
	if err != nil {
		return Outcome{}, err
	}
	esc := v.escalator()
	owner, err := v.runOwner(ctx, d.RunID)
	if err != nil {
		return Outcome{}, err
	}

	// ---- Did-research-actually-run (1.9; Spec S07.3): deterministic,
	// pre-judge, with its bounded fresh-session re-run. ----
	researchNotes, card, err := v.researchGate(ctx, esc, owner, d, in.ResearchNodes)
	if err != nil {
		return Outcome{}, err
	}
	if card != nil {
		return Outcome{Verdict: VerdictEscalate, Card: card}, nil
	}
	return v.drain(ctx, in, d, drainSeed{researchNotes: researchNotes, comments: in.Comments}, esc, owner, rubric)
}

// drain is the one S07.6 rework loop, entered fresh (Verify) or resumed
// with carried state (ResumeWithGuidance). The ⚙ rework cap counts THIS
// invocation's judged rounds — a resume is the requester's explicit grant
// of a fresh bounded budget (the intake coverage-card "one more round"
// precedent) — while round NUMBERING continues across the carried history
// so the durable record stays one coherent progression.
func (v *Verifier) drain(ctx context.Context, in VerifyInput, d Deliverable, seed drainSeed, esc *Escalator, owner string, rubric *RubricBundle) (Outcome, error) {
	rec := v.recorder()
	cap64, err := v.Settings.Int(keyReworkRounds)
	if err != nil {
		return Outcome{}, fmt.Errorf("verify: read ⚙ %s: %w", keyReworkRounds, err)
	}
	patience, err := v.Settings.Int(keyPatienceRounds)
	if err != nil {
		return Outcome{}, fmt.Errorf("verify: read ⚙ %s: %w", keyPatienceRounds, err)
	}

	var (
		out             Outcome
		stop            = seed.stop
		seenKeys        = seed.seenKeys
		raisedIntegrity = map[FindingKey]bool{}
		regenerations   int
		reviseComments  = seed.comments
	)
	out.Rounds = append([]RoundRecord(nil), seed.rounds...)
	if seenKeys == nil {
		seenKeys = map[FindingKey]bool{}
	}
	startRound := len(seed.rounds) + 1
	// Round 0 is not a thing: rounds are 1-based; the hard cap counts
	// judged rounds (Spec S07.6: ⚙ verification.rework_rounds bounds the
	// drain; blockers-only re-entry makes late rounds rare).
	for round := startRound; ; round++ {
		record := RoundRecord{Round: round, ContentSHA: d.SHA256(), Revision: d.Revision}

		// ---- S13.1 minting: the candidate passes to review — one
		// revision per round (idempotent when the resume path re-enters on
		// the pinned revision; a V0-regenerated candidate re-mints as the
		// round's new revision, the malformed mint staying in the
		// never-compressed lineage). ----
		if v.Review != nil {
			if err := v.Review.MintCandidate(ctx, d, round); err != nil {
				return Outcome{}, fmt.Errorf("verify: mint candidate rev %d: %w", d.Revision, err)
			}
		}

		// ---- V0 (Spec S07.2): hard kill before any paid call. ----
		v0 := RunV0(v.preGates(), d)
		record.V0 = &v0
		if _, err := rec.RecordV0(ctx, d.RunID, v0); err != nil {
			return Outcome{}, err
		}
		if v0.Malformed {
			if regenerations < v0RegenerationLimit && v.Regenerate != nil {
				regenerations++
				nd, err := v.Regenerate(ctx, d, v0.Reasons)
				if err != nil {
					return Outcome{}, fmt.Errorf("verify: V0 regeneration: %w", err)
				}
				if nd.Revision <= d.Revision {
					nd.Revision = d.Revision + 1
				}
				nd.PrevContent = d.Content
				d = nd
				round-- // the regeneration is not a judged round
				continue
			}
			// The one-regeneration bound is exhausted (or no regeneration
			// seam): the S07.6 stop-rule terminal carries it to a human —
			// never a judge call, never a silent stop.
			record.Verdict = VerdictEscalate
			out.Rounds = append(out.Rounds, record)
			c, err := esc.Raise(ctx, Escalation{
				Category: CatCapHit, TaskID: d.TaskID, RunID: d.RunID, Owner: owner,
				Summary: "artifact malformed after the one-regeneration V0 bound (Spec S07.2)",
				Detail:  v0.Reasons, Rounds: out.Rounds,
				BestEffort: fmt.Sprintf("revision %d (sha256 %s) is the best-effort state; V0 reasons attached", d.Revision, record.ContentSHA),
			})
			if err != nil {
				return Outcome{}, err
			}
			out.Verdict, out.Card = VerdictEscalate, &c
			return out, nil
		}

		// ---- V1 (Spec S07.3). ----
		var v1res *V1Result
		if v.Pack != nil {
			ws, cleanup, err := v.workspace(ctx, d, in.Workspace)
			if err != nil {
				return Outcome{}, err
			}
			res, err := RunV1(ctx, v.Pack, v.Runner,
				CheckRequest{RunID: d.RunID, Workspace: ws, EvidenceDir: in.EvidenceDir},
				in.Steps, v.now(), v.Settings)
			if cleanup != nil {
				cleanup()
			}
			if err != nil {
				return Outcome{}, err
			}
			v1res = &res
			record.V1 = v1res
			if _, err := rec.RecordV1(ctx, d.RunID, res); err != nil {
				return Outcome{}, err
			}
		}

		// ---- V2 (Spec S07.5): one compliance call, at most one sanity
		// call. ----
		input, err := BuildJudgeInput(ctx, v.Ledger, d, rubric, v1res,
			append(priorFindings(out.Rounds), seed.guidance...), round)
		if err != nil {
			return Outcome{}, err
		}
		ax1, err := v.Judge.Compliance(ctx, input)
		if err != nil {
			return Outcome{}, fmt.Errorf("verify: axis-1 judge: %w", err)
		}
		v1Facts := map[string]CheckOutcomeState{}
		if v1res != nil {
			v1Facts = v1res.ACOutcomes()
		}
		verdicts, integrity := ValidateAxis1(ax1, input.ACs, v1Facts, d.Content)
		record.Axis1 = verdicts

		var ax2 *Axis2Result
		need2, skipReason, err := v.axis2Required(d, in.Tier)
		if err != nil {
			return Outcome{}, err
		}
		if need2 {
			r2, err := v.Judge.Sanity(ctx, input)
			if err != nil {
				return Outcome{}, fmt.Errorf("verify: axis-2 judge: %w", err)
			}
			ax2 = &r2
		} else {
			record.Axis2Skipped = skipReason
		}
		record.Axis2 = ax2

		// ---- Findings: axis 1 + integrity + axis 2 + V1 platform findings
		// + requester comments, validated and numbered (Spec S07.5/S07.6).
		var raw []Finding
		raw = append(raw, ax1.Findings...)
		raw = append(raw, UnknownEscapes(verdicts)...)
		raw = append(raw, integrity...)
		if ax2 != nil {
			raw = append(raw, ax2.Findings...)
		}
		if v1res != nil {
			raw = append(raw, v1res.Findings...)
		}
		if round == startRound {
			raw = append(raw, seed.researchNotes...)
		}
		raw = AddRequesterFindings(raw, reviseComments)
		reviseComments = nil
		findings, suppressed := validateFindings(round, raw, input.ACs, rubric, seenKeys)
		for _, f := range findings {
			seenKeys[f.Key()] = true
		}
		record.Findings = findings
		record.SuppressedNotes = suppressed

		// ---- Verdict (Spec S07.5). ----
		verdict := ComputeVerdict(ax2, findings, ax1.Escalate)
		if verdict == VerdictShip && suppressed > 0 {
			verdict = VerdictShipWithNotes
		}
		record.Verdict = verdict

		seq, err := rec.RecordRound(ctx, d.RunID, d, record, v.Judge.Meta(), rubric.GoldenSet, rubric.ID, rubric.Version,
			acIDs(input.ACs), v.entailmentPosture(d.Domain))
		if err != nil {
			return Outcome{}, err
		}
		record.EventSeq = seq
		out.Rounds = append(out.Rounds, record)

		// ---- S13 landing: the verdict ref pins onto the judged revision
		// and the round's findings become durable review comments — notes
		// ride to the requester on the review surface (Spec S07.6), one
		// schema with human comments (Spec S13.1). Suite defects
		// (check-integrity) stay out: their sink is the card. ----
		if v.Review != nil {
			if err := v.Review.RecordVerdict(ctx, d, seq); err != nil {
				return Outcome{}, fmt.Errorf("verify: record verdict ref: %w", err)
			}
			if err := v.Review.RecordFindings(ctx, d, reviewable(findings)); err != nil {
				return Outcome{}, fmt.Errorf("verify: record round findings: %w", err)
			}
		}

		// CHECK-INTEGRITY raiser (Spec S07.7): every blocker-severity
		// check-integrity finding of the round — judge–check disagreements
		// and runner failures alike — gets its decision card (+ quarantine
		// where a suite check is implicated), once per finding key. The
		// drain continues past them: the route is a card, never a rework
		// round (regenerating the deliverable cannot fix a broken suite),
		// and no screen's SHIP is ever final — V3 still gates.
		for _, f := range findings {
			if f.Category != CatCheckIntegrity || f.Severity != SeverityBlocker || raisedIntegrity[f.Key()] {
				continue
			}
			raisedIntegrity[f.Key()] = true
			c, err := v.raiseIntegrity(ctx, esc, owner, d, f)
			if err != nil {
				return Outcome{}, err
			}
			out.IntegrityCards = append(out.IntegrityCards, c)
		}

		switch verdict {
		case VerdictShip, VerdictShipWithNotes:
			verified, err := VerifyLedgerItems(ctx, v.Ledger, d.RunID, d.TaskID, verdicts, seq)
			if err != nil {
				return Outcome{}, err
			}
			out.Verdict, out.VerifiedItems = verdict, verified
			return out, nil

		case VerdictReopenSpec:
			c, err := esc.Raise(ctx, Escalation{
				Category: CatReopenSpec, TaskID: d.TaskID, RunID: d.RunID, Owner: owner,
				Summary: "axis 2 reopens the specification: " + ax2.ReopenSpec,
				Detail: []string{
					"REOPEN-SPEC is a human decision; the judge can never change the spec itself (Spec S07.5; D10).",
					"An accepted adjustment lands as an S06.9 ADDED/MODIFIED/REMOVED delta and re-freezes the ACs; later rounds judge against the amended frozen set.",
				},
				Findings: findings, Rounds: out.Rounds,
			})
			if err != nil {
				return Outcome{}, err
			}
			out.Verdict, out.Card = VerdictReopenSpec, &c
			return out, nil

		case VerdictEscalate:
			cat := CatACBlocker
			reason := ax1.Escalate
			if reason == "" && ax2 != nil {
				cat, reason = CatSanityBlocker, ax2.Escalate
			}
			c, err := esc.Raise(ctx, Escalation{
				Category: cat, TaskID: d.TaskID, RunID: d.RunID, Owner: owner,
				Summary:  "judge escalated instead of another round: " + reason,
				Findings: findings, Rounds: out.Rounds,
				BestEffort: fmt.Sprintf("revision %d (sha256 %s)", d.Revision, record.ContentSHA),
			})
			if err != nil {
				return Outcome{}, err
			}
			out.Verdict, out.Card = VerdictEscalate, &c
			return out, nil
		}

		// verdict == REVISE: apply the S07.6 stop rules before another
		// round. The cap counts this invocation's judged rounds (see drain).
		converged, why := stop.observe(findings, d.Content, patience)
		atCap := int64(round-startRound+1) >= cap64
		if atCap || converged || v.Revise == nil {
			reason := why
			switch {
			case atCap:
				reason = fmt.Sprintf("hard cap ⚙ %s=%d reached", keyReworkRounds, cap64)
			case v.Revise == nil:
				reason = "rework unavailable (fresh-session executor seam not wired; B2-4)"
			}
			c, err := esc.Raise(ctx, Escalation{
				Category: CatCapHit, TaskID: d.TaskID, RunID: d.RunID, Owner: owner,
				Summary:  "rework stopped without SHIP: " + reason,
				Detail:   []string{"Every round's findings and verdicts are attached — never a silent stop (Spec S07.6)."},
				Findings: findings, Rounds: out.Rounds,
				BestEffort: fmt.Sprintf("revision %d (sha256 %s) is the best-effort state", d.Revision, record.ContentSHA),
			})
			if err != nil {
				return Outcome{}, err
			}
			out.Verdict, out.Card = VerdictEscalate, &c
			return out, nil
		}

		// Build the exact retry package and run the fresh-session executor
		// (Spec S07.6). With the S13 sink wired, the package findings come
		// from THE drain (Spec S13.4): every open comment on the current
		// revision — this round's findings plus any human comments sitting
		// open — numbered [F1..Fn], batch-consumed to the attempt,
		// severity carried (notes travel; the round trigger was the
		// blocker verdict above). Without it, the pre-S13 in-memory shape:
		// the round's blockers.
		var pkg RetryPackage
		if v.Review != nil {
			drained, err := v.Review.DrainOpen(ctx, d, AttemptRef(d.RunID, round+1))
			if err != nil {
				return Outcome{}, fmt.Errorf("verify: drain for round %d: %w", round+1, err)
			}
			for i := range drained {
				drained[i].Round = round + 1 // the round the batch feeds
			}
			pkg = RetryPackage{Spec: in.Spec, ACs: input.ACs, Findings: drained, Deliverable: d, Round: round + 1}
		} else {
			pkg = BuildRetryPackage(in.Spec, input.ACs, findings, d, round+1)
		}
		nd, err := v.Revise(ctx, pkg)
		if err != nil {
			return Outcome{}, fmt.Errorf("verify: rework round %d: %w", round+1, err)
		}
		d = fixupRevision(nd, d)
	}
}

// fixupRevision applies the platform-owned bookkeeping to an executor's
// rework output: identity, monotone revision, and the diff basis are never
// trusted from the session.
func fixupRevision(nd, prev Deliverable) Deliverable {
	if nd.Revision <= prev.Revision {
		nd.Revision = prev.Revision + 1
	}
	nd.PrevContent = prev.Content
	if nd.TaskID == "" {
		nd.TaskID = prev.TaskID
	}
	if nd.RunID == "" {
		nd.RunID = prev.RunID
	}
	if nd.Domain == "" {
		nd.Domain = prev.Domain
	}
	return nd
}

// researchGate runs the 1.9 check with its bounded re-run, terminating in a
// RESEARCH-NOT-RUN card when the budget is exhausted (Spec S07.3/S07.7).
// The returned notes carry UNVERIFIABLE-HERE outcomes into the round record
// — loud, never a fake pass.
func (v *Verifier) researchGate(ctx context.Context, esc *Escalator, owner string, d Deliverable, nodes []intake.ResearchNode) ([]Finding, *Card, error) {
	if len(nodes) == 0 {
		return nil, nil, nil
	}
	reruns := map[string]int{}
	for {
		outcomes, err := CheckResearch(ctx, v.Research, d.TaskID, nodes, reruns, v.Settings)
		if err != nil {
			return nil, nil, err
		}
		var (
			notes     []Finding
			rerunNow  []intake.ResearchNode
			cardNodes []ResearchOutcome
		)
		for _, o := range outcomes {
			switch {
			case o.NeedsCard:
				cardNodes = append(cardNodes, o)
			case o.RerunRequested:
				if v.ResearchRerun == nil {
					// No re-run path: the node cannot be fixed here — card,
					// never a silent pass (Spec S07.7 P46).
					o.Detail += " (re-run seam not wired; B2-4)"
					cardNodes = append(cardNodes, o)
					continue
				}
				rerunNow = append(rerunNow, o.Node)
			case o.State == ContractUnverifiable:
				// The note names its RULE as well as its step (P3-RW-18 D3-R2).
				// A step can carry more than one research node — the walk's plan
				// put P47-1 and P47-8 both on S-1 — and a text naming only the
				// step made two different facts byte-identical, which is what
				// filed the same sentence twice. Two rules were never one fact.
				// The anchor stays `step:<id>`: it is what the finding Key's
				// cross-round suppression is computed over (S07 goalpost drift),
				// and that semantics is untouched here.
				notes = append(notes, Finding{
					Severity: SeverityNote, Category: CatResearchNotRun,
					Anchor: "step:" + o.Node.StepID,
					Text: fmt.Sprintf("did-research-actually-run undecidable for %s [%s]: %s",
						o.Node.StepID, o.Node.RuleID, o.Detail),
				})
			}
		}
		if len(cardNodes) > 0 {
			detail := make([]string, 0, len(cardNodes))
			for _, o := range cardNodes {
				detail = append(detail, fmt.Sprintf("%s (%s): %s", o.Node.RuleID, o.Node.StepID, o.Detail))
			}
			c, err := esc.Raise(ctx, Escalation{
				Category: CatResearchNotRun, TaskID: d.TaskID, RunID: d.RunID, Owner: owner,
				Summary: "research node(s) never invoked a research tool (1.9) — correctness on world-facts is never left to model memory",
				Detail:  detail,
			})
			if err != nil {
				return nil, nil, err
			}
			return nil, &c, nil
		}
		if len(rerunNow) == 0 {
			return notes, nil, nil
		}
		for _, node := range rerunNow {
			if err := v.ResearchRerun(ctx, node); err != nil {
				return nil, nil, fmt.Errorf("verify: research re-run %s: %w", node.StepID, err)
			}
			reruns[node.RuleID]++
		}
	}
}

// raiseIntegrity raises one CHECK-INTEGRITY card and quarantines the
// implicated check pending fix (Spec S07.7 route). The quarantine owner is
// the card holder and the fix-by date is the card's push deadline
// [coordinator-draft reading: the route fixes the sink, not the dates].
func (v *Verifier) raiseIntegrity(ctx context.Context, esc *Escalator, owner string, d Deliverable, f Finding) (Card, error) {
	quarantined := ""
	if v.Pack != nil && f.Criterion != "" {
		for _, c := range v.Pack.Checks {
			if c.ACKey == f.Criterion {
				push, err := v.Settings.Int(keyCardPushHours)
				if err != nil {
					return Card{}, fmt.Errorf("verify: read ⚙ %s: %w", keyCardPushHours, err)
				}
				if err := v.Pack.QuarantineCheck(Quarantine{
					CheckID: c.ID, Owner: owner, Reason: f.Text,
					FixBy: v.now().Add(time.Duration(push) * time.Hour),
				}); err != nil {
					return Card{}, err
				}
				quarantined = c.ID
			}
		}
	}
	return esc.Raise(ctx, Escalation{
		Category: CatCheckIntegrity, TaskID: d.TaskID, RunID: d.RunID, Owner: owner,
		Summary:     "check-integrity: " + f.Text,
		Findings:    []Finding{f},
		Quarantined: quarantined,
	})
}

// ChallengeCheck is the sanctioned executor challenge (Spec S07.3 rule 5):
// "these tests look wrong" escalates as a CHECK-INTEGRITY card instead of
// test edit access — a measured anti-hacking device; structure, not prompt
// text, is the defense.
func (v *Verifier) ChallengeCheck(ctx context.Context, d Deliverable, checkID, challenge string) (Card, error) {
	owner, err := v.runOwner(ctx, d.RunID)
	if err != nil {
		return Card{}, err
	}
	return v.escalator().Raise(ctx, Escalation{
		Category: CatCheckIntegrity, TaskID: d.TaskID, RunID: d.RunID, Owner: owner,
		Summary: fmt.Sprintf("executor challenges check %q: %s", checkID, challenge),
		Detail:  []string{"Sanctioned challenge in place of test edit access (Spec S07.3 rule 5); the suite is under human review."},
	})
}

// RecordAuditFailure quarantines a check that failed its planted-defect
// audit and raises the CHECK-INTEGRITY card (P-T06-1; audit RUNNERS are S14
// machinery, B5 — this is the raiser they call).
func (v *Verifier) RecordAuditFailure(ctx context.Context, d Deliverable, checkID, detail string) (Card, error) {
	owner, err := v.runOwner(ctx, d.RunID)
	if err != nil {
		return Card{}, err
	}
	if v.Pack != nil {
		push, err := v.Settings.Int(keyCardPushHours)
		if err != nil {
			return Card{}, fmt.Errorf("verify: read ⚙ %s: %w", keyCardPushHours, err)
		}
		if err := v.Pack.QuarantineCheck(Quarantine{
			CheckID: checkID, Owner: owner,
			Reason: "planted-defect audit failure: " + detail,
			FixBy:  v.now().Add(time.Duration(push) * time.Hour),
		}); err != nil {
			return Card{}, err
		}
	}
	return v.escalator().Raise(ctx, Escalation{
		Category: CatCheckIntegrity, TaskID: d.TaskID, RunID: d.RunID, Owner: owner,
		Summary:     fmt.Sprintf("check %q failed its planted-defect audit (P-T06-1)", checkID),
		Detail:      []string{detail},
		Quarantined: checkID,
	})
}

// RaiseWorkerFlaw routes a recurring-defect report to the worker-flaw
// ticket sink (Spec S07.7; the recurrence DETECTOR across tasks is S08/S14
// machinery — B3).
func (v *Verifier) RaiseWorkerFlaw(ctx context.Context, d Deliverable, flaw WorkerFlaw) (Card, error) {
	owner, err := v.runOwner(ctx, d.RunID)
	if err != nil {
		return Card{}, err
	}
	return v.escalator().Raise(ctx, Escalation{
		Category: CatWorkerFlaw, TaskID: d.TaskID, RunID: d.RunID, Owner: owner,
		Summary: fmt.Sprintf("recurring defect pattern attributable to worker %q: %s", flaw.Worker, flaw.Pattern),
		Flaw:    &flaw,
	})
}

// RaiseSafety routes a SAFETY-class item to the operator alert sink (Spec
// S07.7): watchdog pause-and-flag (G1 D1.3; S14) and confinement violations
// (S11) call this — the single verification/watchdog crossing point.
func (v *Verifier) RaiseSafety(ctx context.Context, d Deliverable, summary string, detail []string) (Card, error) {
	owner, err := v.runOwner(ctx, d.RunID)
	if err != nil {
		return Card{}, err
	}
	return v.escalator().Raise(ctx, Escalation{
		Category: CatSafety, TaskID: d.TaskID, RunID: d.RunID, Owner: owner,
		Summary: summary, Detail: detail,
	})
}

// RunCanary drives the dead-man canary plant through the escalation path
// (Spec S07.7 proof 2): a deterministic RESEARCH-NOT-RUN card, zero model
// calls, zero allowance. SCHEDULING is S14's (B5); the external silence
// watch is CanaryWatch.
func (v *Verifier) RunCanary(ctx context.Context, taskID, runID string) (Card, error) {
	d, _, nodes := CanaryPlan(taskID, runID)
	owner, err := v.runOwner(ctx, d.RunID)
	if err != nil {
		return Card{}, err
	}
	esc := v.escalator()
	outcomes, err := CheckResearch(ctx, CanaryResearchUsage(), d.TaskID, nodes,
		map[string]int{"canary": int(mustInt(v.Settings, keyResearchRerun))}, v.Settings)
	if err != nil {
		return Card{}, err
	}
	detail := make([]string, 0, len(outcomes))
	for _, o := range outcomes {
		detail = append(detail, fmt.Sprintf("%s: %s", o.Node.StepID, o.Detail))
	}
	return esc.Raise(ctx, Escalation{
		Category: CatResearchNotRun, TaskID: d.TaskID, RunID: d.RunID, Owner: owner,
		Summary: "dead-man canary: this card arriving IS the proof the escalation path is alive",
		Detail:  detail,
		Canary:  true,
	})
}

func mustInt(s Settings, key string) int64 {
	n, err := s.Int(key)
	if err != nil {
		return 0
	}
	return n
}

// workspace resolves the verification workspace: the S13 seam when wired,
// else the caller-prepared dev path.
func (v *Verifier) workspace(ctx context.Context, d Deliverable, devPath string) (string, func(), error) {
	if v.Workspace != nil {
		dir, cleanup, err := v.Workspace.VerificationWorkspace(ctx, d)
		if err != nil {
			return "", nil, err
		}
		if dir != "" {
			return dir, cleanup, nil
		}
		// An empty dir is the seam's HONEST ABSENCE: this deliverable is not
		// repo-backed, so there is no revision tree to materialize. That is
		// not a failure and not an empty workspace — the caller-prepared path
		// (the producing run's own cwd) is the tree under review. A wired
		// seam that answers "nothing here" must not starve the checks.
		if cleanup != nil {
			cleanup()
		}
	}
	if devPath == "" {
		return "", nil, fmt.Errorf("%w: V1 needs a verification workspace (S13 seam or VerifyInput.Workspace)", ErrBadInput)
	}
	return devPath, nil, nil
}

// runOwner reads the run's owner for ask attribution (15.6).
func (v *Verifier) runOwner(ctx context.Context, runID string) (string, error) {
	var owner string
	if err := v.DB.QueryRowContext(ctx,
		`SELECT user_id FROM runs WHERE run_id = ?`, runID).Scan(&owner); err != nil {
		return "", fmt.Errorf("verify: read run %q owner: %w", runID, err)
	}
	return owner, nil
}

// priorFindings collects every prior round's findings for the judge input
// (Spec S07.6: prior findings in scope, carried verbatim).
func priorFindings(rounds []RoundRecord) []Finding {
	var out []Finding
	for _, r := range rounds {
		out = append(out, r.Findings...)
	}
	return out
}

func acIDs(acs []ledger.AcceptanceCriterion) []string {
	out := make([]string, len(acs))
	for i, ac := range acs {
		out[i] = fmt.Sprintf("AC-%d", ac.N)
	}
	return out
}
