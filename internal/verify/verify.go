// Package verify is the Spec S07 verification & quality pipeline: everything
// between "execution produced a deliverable revision" and "the requester sees
// it". Two independent axes (spec compliance, outcome sanity) run through the
// cheap-first V-layer cascade — V0 deterministic pre-gates, V1 domain check
// packs, V2 one dual-axis judge pass — on EVERY deliverable, never sampled
// (Spec S07.1); V3 is the human gate via the approval inbox (accept mechanics
// Spec S13, B4).
//
// The quality gate is never the effects gate (Spec S07.1/S02.7): the sequence
// is quality verdict (V0–V2) → human accept (V3) → gated effect execution
// under D7. Nothing in this package touches an effect-journal row — verdicts
// release nothing outward and the judge is never an implicit release
// authority. The production files import nothing from internal/gates; the
// separation is proven by test (effects_test.go), not asserted.
//
// Boundary lines (Spec S07.1): S13 owns deliverable/revision/comment
// mechanics including anchor storage — anchors are opaque strings here. S14
// owns eval/benchmark machinery, watchdogs, and canary SCHEDULING. S08 owns
// judge/worker model selection — the Judge and FlawTickets seams. S11 owns
// the sandbox mechanics V1 relies on — reached only through the
// adapters.Confiner seam. S12 owns local-tier seats — the EntailmentChecker
// seam.
package verify

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Verdict is the V2 verdict enum (Spec S07.5): SHIP / SHIP-with-notes /
// REVISE (blockers only) / ESCALATE, plus axis 2's unique power REOPEN-SPEC.
type Verdict string

const (
	VerdictShip          Verdict = "SHIP"
	VerdictShipWithNotes Verdict = "SHIP-with-notes"
	VerdictRevise        Verdict = "REVISE"
	VerdictEscalate      Verdict = "ESCALATE"
	// VerdictReopenSpec files a decision card to the requester — proceed as
	// specified / adjust the specification / rethink — and can never change
	// the spec itself (Spec S07.5; D10). An accepted adjustment lands as an
	// S06.9 delta and re-freezes the ACs.
	VerdictReopenSpec Verdict = "REOPEN-SPEC"
)

// Severity tags a finding blocker or note (Spec S07.5). Only blockers
// trigger another rework round; notes ride along to the requester as review
// comments (Spec S07.6; comment mechanics Spec S13).
type Severity string

const (
	SeverityBlocker Severity = "blocker"
	SeverityNote    Severity = "note"
)

// Finding is one numbered verification finding [F1..Fn], anchored to the
// exact place it concerns and carried VERBATIM across rework rounds (Spec
// S07.5/S07.6). Every blocker must cite the frozen criterion (or axis-2
// rubric item) it violates; a finding that cites none can only be a note or
// a REOPEN-SPEC matter — enforced by validateFindings, which demotes and
// records rather than silently dropping.
type Finding struct {
	N        int      `json:"n"` // [F1..Fn] number within its round
	Severity Severity `json:"severity"`
	// Category is the route-table category the finding escalates under
	// (Spec S07.7); stage contracts must declare it up front (Spec S07.3).
	Category Category `json:"category"`
	// Criterion cites the frozen criterion ("AC-3") or axis-2 rubric item id
	// ("axis2/side-effects") the finding violates; "" is admissible only on
	// notes.
	Criterion string `json:"criterion,omitempty"`
	// Anchor is the exact place the finding concerns (file:line / section /
	// claim id). Anchor storage and re-anchoring are Spec S13's (B4); the
	// value is opaque here.
	Anchor string `json:"anchor,omitempty"`
	// Text is carried verbatim as a numbered point (Spec S07.6): the retry
	// fixes named problems and never regenerates blind.
	Text string `json:"text"`
	// SuggestedChange is the optional suggested change of the S13.1
	// finding schema, delivered with the numbered point (Spec S13.4).
	SuggestedChange string `json:"suggested_change,omitempty"`
	// Round is the round the finding was first raised in.
	Round int `json:"round"`
	// Demoted marks a blocker demoted to note for citing no frozen
	// criterion (goalpost fixing, Spec S07.5) — recorded, never silent.
	Demoted bool `json:"demoted,omitempty"`
	// Requester marks a finding that entered through the requester-comment
	// channel (6.2): same numbered-anchored-point channel as judge findings
	// (Spec S07.6).
	Requester bool `json:"requester,omitempty"`
}

// FindingKey is the stable identity of a finding — criterion + anchor +
// failure class (Spec S07.6, new term). Recurrence of a finding key across
// rounds is the convergence signal.
type FindingKey string

// Key returns the finding's stable identity.
func (f Finding) Key() FindingKey {
	return FindingKey(f.Criterion + "|" + f.Anchor + "|" + string(f.Category))
}

// blockers filters the blocker-severity findings.
func blockers(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Severity == SeverityBlocker {
			out = append(out, f)
		}
	}
	return out
}

// keySet returns the set of finding keys.
func keySet(fs []Finding) map[FindingKey]bool {
	m := make(map[FindingKey]bool, len(fs))
	for _, f := range fs {
		m[f.Key()] = true
	}
	return m
}

// Deliverable is the revision under verification — the minimal platform
// handle Spec S07 consumes. Revision storage, diffing, and comment threads
// are Spec S13's (B4); Content/Diff here are the artifact-of-record text the
// caller supplies (extracted claims/diffs, not presentation — P-T06-3). Bulk
// trees ride the verification workspace (WorkspaceProvider seam), never this
// struct.
type Deliverable struct {
	TaskID string `json:"task_id"`
	RunID  string `json:"run_id"`
	// Domain names the deliverable's domain ("software" is the v0 launch
	// domain — Spec S07.8; G1 D1.2).
	Domain string `json:"domain"`
	// Type drives the V0 per-type artifact-shape checks (Spec S07.2):
	// "code" | "markdown" | "json" | "" (generic).
	Type string `json:"type,omitempty"`
	// Revision is 1-based; revision > 1 enables the diff-empty pre-gate.
	Revision int `json:"revision"`
	// Content is the artifact-of-record text of this revision.
	Content string `json:"content"`
	// PrevContent is the previous revision's content ("" at revision 1) —
	// the V0 diff-empty input and the V2 diff basis until S13 owns diffs.
	PrevContent string `json:"prev_content,omitempty"`
	// Diff is the caller-supplied diff against the previous revision (Spec
	// S07.5 judge input; diff mechanics Spec S13).
	Diff string `json:"diff,omitempty"`
	// Steered marks a deliverable any part of which flowed through an
	// untrusted-content reader (4.7): it never skips V2 and never skips
	// axis 2, regardless of stakes gating (Spec S07.8).
	Steered bool `json:"steered,omitempty"`

	// ---- Repo-backed platform facts (Spec S13.1 content pin; S02.8
	// claims). These are DURABLE platform knowledge — the snapshot ledger and
	// the approved plan — never an engine-supplied signal, which is what lets
	// the S07.2 wrote-nothing gate be a $0 structural kill.

	// SnapshotSHA is the revision's content pin: the snapshot commit of a
	// repo-backed revision (Spec S13.1). Empty when the deliverable is not
	// repo-backed, and an empty pin gives the wrote-nothing gate no facts.
	SnapshotSHA string `json:"snapshot_sha,omitempty"`
	// BaseSHA is the attempt's base ref (refs/sinet/base/<pipeline>) — the
	// pre-task state that revision 1 is presented against (Spec S13.1).
	// Empty means unknown, and the gate never guesses a base.
	BaseSHA string `json:"base_sha,omitempty"`
	// WriteClaimed is the plan's durable claim that this task writes: an
	// active W artifact_claims row, or any non-empty step write_set (Spec
	// S02.8). A plan that claims no writes may legitimately move no tree.
	WriteClaimed bool `json:"write_claimed,omitempty"`
}

// SHA256 returns the revision content hash (round records, similarity).
func (d Deliverable) SHA256() string {
	sum := sha256.Sum256([]byte(d.Content))
	return hex.EncodeToString(sum[:])
}

// launchDomains is the v0 launch-domain roster (Spec S07.8; G1 D1.2):
// software at v0; web research joins at v0.1 (Spec S19). A structural
// rollout roster, not a ⚙ setting — S18 declares no key for it.
var launchDomains = map[string]bool{DomainSoftware: true}

// DomainSoftware is the v0 launch domain (Spec S07.3).
const DomainSoftware = "software"

// DomainWebResearch arrives at v0.1 with the C3 confinement class and
// entailment activation (Spec S07.4; Spec S19).
const DomainWebResearch = "web-research"

// LaunchDomain reports whether a domain gets axis 2 on every deliverable
// (Spec S07.8).
func LaunchDomain(domain string) bool { return launchDomains[domain] }

// Settings is the narrow ⚙ reader of this package (CONVENTIONS §2:
// consumers read the settings registry by dotted key; *settings.Registry
// satisfies it).
type Settings interface {
	Int(key string) (int64, error)
	Float(key string) (float64, error)
	String(key string) (string, error)
}

// The S07 ⚙ keys (Spec S07.11 settings table; declared in
// internal/settings/index.go since B0-2).
const (
	keyReworkRounds      = "verification.rework_rounds"
	keyPatienceRounds    = "verification.convergence_patience_rounds"
	keySanityStakesFloor = "verification.sanity_stakes_floor"
	keyEntailmentSample  = "verification.entailment_sample_rate"
	keyResearchRerun     = "verification.research_rerun_limit"
	keyCheckAuditDays    = "verification.check_audit_interval_days"
	keyCanaryHours       = "verification.canary_interval_hours"
	keyDrillDays         = "verification.drill_interval_days"
	keyCardRemindHours   = "verification.card_remind_hours"
	keyCardPushHours     = "verification.card_push_hours"
	keySafetyRepingHours = "verification.safety_reping_hours"
)

// Errors callers branch on.
var (
	// ErrNoCheckPack rejects verifying a launch-domain deliverable without
	// its domain check pack: the launch domain has a purpose-built pack by
	// definition (Spec S07.8 graduation rule); running without one would be
	// a silent degraded mode.
	ErrNoCheckPack = errors.New("verify: launch-domain deliverable without a domain check pack (Spec S07.8)")
	// ErrBadPack covers check-pack contract violations (Spec S07.3).
	ErrBadPack = errors.New("verify: invalid check pack")
	// ErrSeamMissing is returned when a required seam is not wired (e.g. no
	// Judge for V2) — verification never silently skips a layer (Spec
	// S07.1: nothing is delivered unverified).
	ErrSeamMissing = errors.New("verify: required seam not wired")
	// ErrBadInput covers verification-input contract violations.
	ErrBadInput = errors.New("verify: invalid input")
	// ErrBadSeed covers seed-artifact contract violations (Spec S07.10).
	ErrBadSeed = errors.New("verify: invalid seed artifact")
)

// randomHex returns n random bytes hex-encoded (ask/effect id minting — the
// intake precedent).
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand read cannot fail on the target platforms; if it ever
		// does, an id collision is worse than a loud stop.
		panic(fmt.Sprintf("verify: random id: %v", err))
	}
	return hex.EncodeToString(b)
}

// similarity is the deterministic artifact-similarity measure feeding the
// convergence stop rule (Spec S07.6 "model inertia" — cosmetic edits in
// response to findings): Jaccard overlap of the non-empty trimmed line sets.
func similarity(a, b string) float64 {
	la, lb := lineSet(a), lineSet(b)
	if len(la) == 0 && len(lb) == 0 {
		return 1
	}
	inter, union := 0, len(lb)
	for l := range la {
		if lb[l] {
			inter++
		} else {
			union++
		}
	}
	if union == 0 {
		return 1
	}
	return float64(inter) / float64(union)
}

func lineSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			m[t] = true
		}
	}
	return m
}

// convergenceSimilarity is the similarity bar above which a round's edit is
// "cosmetic" for the convergence stop rule (Spec S07.6). S18 ratifies no key
// for it — a documented code constant per the CONVENTIONS §7 precedent
// (flagged to the B2 gate; the standing settings-tab directive applies).
const convergenceSimilarity = 0.9
