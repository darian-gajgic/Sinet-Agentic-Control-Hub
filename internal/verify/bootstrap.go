package verify

import (
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// The bootstrap verification posture [A14, 2026-08-27] — Spec S07.8. A
// launch-domain deliverable whose registered project has NO captured
// build/test/lint command (every fresh scaffold's first task) is never a
// verification refusal and never parks the run. V0 runs unchanged; V1 runs the
// S07.3 stage-contract checks with every executable-ladder rung recording
// UNVERIFIABLE-HERE rather than being skipped or reported as PASS; V2 runs
// with its verdict advisory and visibly marked non-authoritative; requester
// review is mandatory at every stakes tier.
//
// Bootstrap is COMPUTED from the registry's current capture, never a default
// for an unwired seam: a launch domain whose pack machinery is simply not
// there stays the loud ErrNoCheckPack refusal, and a capture the platform
// cannot read stays ErrBadPack. Honest absence still fails loud; what changes
// is that "this project has nothing to run yet" is an absence with a defined
// posture rather than an outage.

// Posture names the verification posture a round ran under (Spec S07.8). The
// empty posture is the full one: the domain's check pack executed.
type Posture string

// PostureBootstrap marks a round whose launch domain had no executable rung to
// run [A14, 2026-08-27]. Its V2 verdict is advisory and non-authoritative, so
// no code path may mark its work verified or deliver it without a human act.
const PostureBootstrap Posture = "bootstrap"

// BootstrapAttribution is the stable attribution marker on every record the
// bootstrap posture writes — the ladder rungs' outcomes and the PLAN steps'
// contracts alike. It names the absent substrate rather than an upstream
// check, which is exactly what ContractUnverifiable means when "the deciding
// substrate is not wired" (Spec S07.3).
const BootstrapAttribution = "check-pack:absent"

// BootstrapPostureNote is the requester-facing disclosure the posture carries
// onto the round record, the round's findings, every card terminal raised
// under it, and the receipt (Spec S07.8: "the verdict card and receipt name
// the bootstrap posture in plain words, including that capturing the project's
// commands restores the full ladder"). Plain words for a person, never an
// error chain, and it promises no door it does not have.
const BootstrapPostureNote = "This project has no build, test or lint command captured yet, so the checks that would prove this work correct could not run. Nothing was passed off as checked: every check rung is recorded as unverifiable here, the judge's verdict is advisory only, and your review is what decides this work. Capturing the project's commands restores the full ladder from the next revision on."

// BootstrapPack is the check-pack resolution for a registered project whose
// capture holds no executable rung [A14, 2026-08-27]. It carries no checks by
// construction — bootstrap invents nothing on a project's behalf — and it is
// deliberately NOT a valid pack: Validate still refuses a pack without checks,
// and the drain branches on the posture instead of running it.
func BootstrapPack(domain string, version int) *CheckPack {
	return &CheckPack{Domain: domain, Version: version, Posture: PostureBootstrap}
}

// bootstrap reports whether p is the bootstrap resolution.
func (p *CheckPack) bootstrap() bool { return p != nil && p.Posture == PostureBootstrap }

// executes reports whether p has rungs to run — the condition a CheckRunner is
// required for. A bootstrap resolution has none.
func (p *CheckPack) executes() bool { return p != nil && len(p.Checks) > 0 }

// packPosture names the posture a round resolving to pack runs under.
func packPosture(pack *CheckPack) Posture {
	if pack.bootstrap() {
		return PostureBootstrap
	}
	return ""
}

// bootstrapPostureFinding carries the posture into the round's findings, from
// where it reaches the requester on the review surface (through reviewable's
// posture exemption), the judge's prior-findings scope, and the durable round
// record. Note severity by construction — a permanent blocker would drive
// REVISE to the cap and park the run, which is the wall S07.8 abolishes —
// under the CHECK-INTEGRITY category (the ratified reading: it is a fact about
// the suite, like the quarantine-skip note). That category's card raiser fires
// only on blockers, so the disclosure spams no inbox, and ComputeVerdict
// excludes the category from its note count, which is why the drain states the
// advisory SHIP-with-notes downgrade itself.
func bootstrapPostureFinding() Finding {
	return Finding{
		Severity: SeverityNote,
		Category: CatCheckIntegrity,
		Anchor:   BootstrapAttribution,
		Text:     BootstrapPostureNote,
	}
}

// bootstrapPostureKey is the disclosure's stable identity (criterion + anchor
// + category). It is fixed, so the note is the SAME finding in every round it
// is raised in — which is what both exemptions below key on, by IDENTITY and
// never by category.
var bootstrapPostureKey = bootstrapPostureFinding().Key()

// isPostureDisclosure reports whether f is the bootstrap posture disclosure.
//
// Two rules exempt exactly this finding, and nothing else:
//
//   - the S07.6 new-note suppression (validateFindings). Suppression exists to
//     stop goalposts drifting round by round; the posture is not a new goalpost
//     but a fact about how THIS round was verified, and which round the
//     bootstrap posture first appears in must never decide whether the
//     requester is told about it.
//   - the review-stream strip (reviewable). Suite defects stay out of the
//     deliverable's review channel because regenerating the deliverable cannot
//     fix a broken check; the posture is the requester's answer to "why was
//     nothing checked", and the review surface is exactly where the mandatory
//     V3 decision is made (Spec S07.8). It rides as a note, so it still
//     triggers no rework round.
func isPostureDisclosure(f Finding) bool { return f.Key() == bootstrapPostureKey }

// bootstrapV1 is the V1 result of a bootstrap round (Spec S07.8): every
// executable-ladder rung the missing commands would have populated records
// UNVERIFIABLE-HERE, and every PLAN step's "Done when" contract records the
// same state attributed to the absent pack. Nothing is fabricated — no
// evidence ref, no exit status, no invented executable check — and nothing is
// skipped silently.
func bootstrapV1(pack *CheckPack, steps []intake.Step) V1Result {
	res := V1Result{Findings: []Finding{bootstrapPostureFinding()}}
	if pack != nil {
		res.PackVersion = pack.Version
	}
	for _, stage := range ladderOrder {
		res.Checks = append(res.Checks, CheckOutcome{
			CheckID:      "ladder:" + string(stage),
			Stage:        stage,
			State:        CheckUnverifiable,
			AttributedTo: BootstrapAttribution,
			Detail: fmt.Sprintf("the %s rung has no captured command to run, so it is recorded unverifiable here rather than passed (Spec S07.8)",
				stage),
		})
	}
	for _, s := range steps {
		res.Steps = append(res.Steps, StepContract{
			StepID:       s.ID,
			DoneWhen:     s.DoneWhen,
			State:        ContractUnverifiable,
			AttributedTo: BootstrapAttribution,
			Category:     CatACBlocker,
			Route:        RouteTable[CatACBlocker].Sink,
		})
	}
	return res
}

// postureDetail appends the disclosure to a card's detail lines so every
// terminal raised under the bootstrap posture names it (Spec S07.8).
func postureDetail(detail []string, p Posture) []string {
	if p != PostureBootstrap {
		return detail
	}
	out := make([]string, 0, len(detail)+1)
	out = append(out, detail...)
	return append(out, BootstrapPostureNote)
}
