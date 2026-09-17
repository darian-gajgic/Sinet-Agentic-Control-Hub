package verify

// evidence.go — the execution rung at bootstrap [A16, 2026-09-17].
//
// Spec S07.8's bootstrap posture is not execution-less: at the execute→verify
// boundary of a bootstrap round the platform rescans the produced tree (Spec
// S13.7) and runs any DETECTED build/test/lint command in the verification
// sandbox as an EVIDENCE rung. Evidence is the operative word — executor-
// authored tests are evidence under Spec S07.3 rule 4, never the AC verdict —
// and detected commands never graduate the project: V2 stays advisory and V3
// mandatory until a hand-captured pack exists.
//
// For a web launch-domain deliverable the S07.3 e2e rung is a PLATFORM-AUTHORED
// walk of the frozen given/when/then acceptance lines, driven and asserted on
// DOM state and never on rendered pixels (TQ-F7). The SCHEMA of that walk lives
// here; the driver that executes it does not exist, because no browser is
// adopted for verification walks yet (Spec S16.2–S16.4: a new component needs a
// components.lock entry and gate ratification). Until one is, the walk rung
// records UNVERIFIABLE-HERE with that reason — never PASS.
//
// P3-TQ-4a ships this file as the schema plus the honest absence; P3-TQ-4b
// plugs the driver in behind WalkDriver once the gate ratifies a browser.

import (
	"fmt"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// Provenance names where a resolved check pack's commands came from. The empty
// provenance is the ordinary one: the owner captured these commands by hand
// through the S13.7 Commands door.
//
// Distinct from Check.Provenance, which is a different axis entirely — that
// field records the SEPARATE AUTHORING CONTEXT of an acceptance check (Spec
// S07.3 rule 4). This one records who supplied the command line.
type Provenance string

// ProvenanceDetected marks a pack whose commands the platform detected by
// scanning the produced tree rather than receiving from its owner [A16]. A
// detected pack's outcomes are evidence; its posture stays bootstrap.
const ProvenanceDetected Provenance = "detected"

// DetectedUnrunnable is the attribution on a detected rung the verification
// workspace cannot supply the tool for — the command's own dependencies were
// never installed into the tree under review and the sandbox has no network
// (Spec S07.3 rule 1: network off, no exception). Recorded UNVERIFIABLE-HERE
// with its reason; never run, never PASS, never FAIL.
const DetectedUnrunnable = "command:unrunnable"

// WalkUnavailable is the attribution on the e2e walk rung while no browser is
// adopted for verification walks.
const WalkUnavailable = "walk:no-browser-adopted"

// WalkUnavailableReason is the requester-facing sentence the walk rung carries
// in that state. Plain words for a person, and it promises no door it does not
// have (the BootstrapPostureNote register).
const WalkUnavailableReason = "the platform could not drive this deliverable in a browser and check each acceptance line for itself: no browser has been adopted for verification walks yet, so this rung is recorded as unproven here rather than passed"

// WalkStepKind is one move of a platform-authored acceptance walk.
type WalkStepKind string

const (
	// WalkNavigate opens the deliverable at a starting point (the "given").
	WalkNavigate WalkStepKind = "navigate"
	// WalkAct performs the user action (the "when").
	WalkAct WalkStepKind = "act"
	// WalkAssert asserts on DOM STATE — never on rendered pixels (TQ-F7: a
	// frame-starved tab renders nothing while the DOM is entirely correct,
	// and a pixel assertion would call that app broken; the inverse failure,
	// an animation-gated app that never paints, is the executor quality rule's
	// job, not this rung's).
	WalkAssert WalkStepKind = "assert"
)

// WalkStep is one step of the walk for one frozen AC.
type WalkStep struct {
	Kind WalkStepKind `json:"kind"`
	// Clause is the verbatim given/when/then clause this step came from, so
	// the recorded outcome can be read against the frozen line it walked.
	Clause string `json:"clause"`
	// Target is the DOM target the step acts or asserts on, and Expect the
	// asserted state. Both are empty on a step the platform could describe
	// but not compose — which is why Composable is a separate fact.
	Target string `json:"target,omitempty"`
	Expect string `json:"expect,omitempty"`
	// Composable reports whether the platform could turn this clause into a
	// step it can actually execute. A step it cannot compose records
	// UNVERIFIABLE-HERE with its reason, never PASS [A16].
	Composable bool `json:"composable"`
}

// WalkState is one AC's walk outcome state — the S07.3 contract vocabulary,
// unchanged: PASS / FAIL / N-A / UNVERIFIABLE-HERE.
type WalkState string

const (
	WalkPass WalkState = "PASS"
	WalkFail WalkState = "FAIL"
	// WalkNA: the walk RAN and this criterion is not one it covers (the AC
	// carries no given/when/then sub-line, so verification judges the plain
	// line at V2 — Spec S06.6). N-A is the ladder's word for "a suite ran and
	// no check covered this", so it is only ever reachable once the walk rung
	// has actually run.
	WalkNA WalkState = "N-A"
	// WalkUnverifiable: the walk could not decide this criterion here — the
	// rung did not run, or a step could not be composed. Carries its reason.
	WalkUnverifiable WalkState = "UNVERIFIABLE-HERE"
)

// ACWalk is the platform's walk plan for one frozen AC: the given/when/then
// sub-line decomposed into steps. Derived from the SPEC's structured sub-line
// only (Spec S06.6 dual phrasing) — never from the plain line, never from the
// executor's report, and never from the deliverable text.
type ACWalk struct {
	ACKey string `json:"ac_key"`
	// Walkable reports whether this AC carries a given/when/then sub-line at
	// all. An AC without one is not a walk failure: it is a criterion the walk
	// does not cover.
	Walkable bool       `json:"walkable"`
	Steps    []WalkStep `json:"steps,omitempty"`
	// Reason states, in plain words, why an AC is not walkable or why a step
	// could not be composed.
	Reason string `json:"reason,omitempty"`
}

// WalkOutcome is one AC's recorded walk result (Spec S07.3 recording; S07.11).
type WalkOutcome struct {
	ACKey string    `json:"ac_key"`
	State WalkState `json:"state"`
	// FailedStep indexes the first step that failed, -1 when none did — the
	// first-upstream-failure attribution S07.3 requires, at walk granularity.
	FailedStep int `json:"failed_step"`
	// AttributedTo names what decided the state (a step, or WalkUnavailable).
	AttributedTo string `json:"attributed_to,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

// WalkPlanFor decomposes every frozen AC carrying a given/when/then sub-line
// into a walk plan. Pure and deterministic over the SPEC: same spec, same plan.
//
// The clause split is the deterministic half — "Given X, when Y, then Z"
// carries three spans, and reading them out is arithmetic. Turning a clause
// into a step the platform can execute is NOT deterministic from that prose,
// which is exactly why WalkStep.Composable exists and why an uncomposable step
// records UNVERIFIABLE-HERE rather than a verdict [A16].
func WalkPlanFor(spec intake.Spec) []ACWalk {
	plan := make([]ACWalk, 0, len(spec.ACs))
	for _, ac := range spec.ACs {
		w := ACWalk{ACKey: fmt.Sprintf("AC-%d", ac.N)}
		given, when, then, split := splitGivenWhenThen(ac.Structured)
		switch {
		case ac.StructuredKind != "gwt":
			w.Reason = "this criterion is written as a plain sentence with no given/when/then line, so the walk does not cover it — a person's reading of it decides it instead"
		case !split:
			w.Reason = "this criterion's given/when/then line does not read as three separate clauses, so the walk could not tell where to start, what to do and what to check"
		default:
			w.Walkable = true
			// Composable is FALSE on every step here, and that is the honest
			// value rather than a placeholder: composing a clause into a move
			// the platform can execute means reading the served page, and no
			// browser is adopted to serve or read one (Spec S16.4; TQ-4b).
			// Target and Expect stay empty with it — an uncomposable step
			// names no state it could not have read.
			w.Steps = []WalkStep{
				{Kind: WalkNavigate, Clause: given},
				{Kind: WalkAct, Clause: when},
				{Kind: WalkAssert, Clause: then},
			}
			w.Reason = WalkUnavailableReason
		}
		plan = append(plan, w)
	}
	return plan
}

// splitGivenWhenThen reads the three spans of a "Given X, when Y, then Z" line.
// This is the deterministic half of the walk: the clauses are where the line
// says they are, and reading them out is arithmetic. Matching is
// case-insensitive on the three keywords and on nothing else, so the clauses
// come back VERBATIM — the recorded outcome has to be readable against the
// frozen line it walked.
func splitGivenWhenThen(line string) (given, when, then string, ok bool) {
	lower := strings.ToLower(line)
	g := strings.Index(lower, "given ")
	w := strings.Index(lower, " when ")
	t := strings.Index(lower, " then ")
	if g < 0 || w <= g || t <= w {
		return "", "", "", false
	}
	clause := func(s string) string { return strings.Trim(strings.TrimSpace(s), " ,.") }
	given = clause(line[g+len("given ") : w])
	when = clause(line[w+len(" when ") : t])
	then = clause(line[t+len(" then "):])
	if given == "" || when == "" || then == "" {
		return "", "", "", false
	}
	return given, when, then, true
}

// WalkDriver executes a composed walk against the deliverable served by its
// captured (or detected) dev command inside the C1 verification sandbox —
// server and headless browser both inside the empty netns, no egress, no host
// routing [A16; Spec S11.4 C1].
//
// No implementation exists: the browser it needs is an S16.4 adoption decision
// that has not been taken (P3-TQ-4b). A nil driver is the honest absence, not a
// degraded mode — every AC records UNVERIFIABLE-HERE with WalkUnavailableReason.
type WalkDriver interface {
	Walk(plan []ACWalk, workspace, devCommand string) ([]WalkOutcome, error)
}
