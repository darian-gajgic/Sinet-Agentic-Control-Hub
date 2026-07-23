package watchdog

import (
	"context"
	"errors"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// tier0.go — the S14.4 Tier-0 deterministic counters (brief R1–R8). Each is a
// PURE function over a bounded slice of the run's recent tool-completion events,
// re-derived STATELESS every evaluation (no durable cursor, no state table —
// Spec S14.1). Every threshold is read by dotted key at evaluation time
// (live-apply — an operator raise takes effect next evaluation; §6/§7).

// trigger carries a tripped counter's rule id + evidence for the flag (R17).
type trigger struct {
	Rule      string
	Signature string
	Counts    map[string]int
	Detail    string
	Spend     *SpendEvidence
}

// Tail is the event-driven Tier-0 driver (R28): it reads new run_events past
// the cursor, collects the runs that got a relevant new event, and re-evaluates
// each. The shell owns the goroutine that calls it on a nudge/poll (the
// After+Nudge precedent, §7). Returns the advanced cursor. Errors on individual
// runs are logged and never abort the batch (forward-tolerant, S14.2 rule 3).
func (w *Watchdog) Tail(ctx context.Context, afterSeq int64) (int64, error) {
	events, err := w.log.After(ctx, afterSeq, 256)
	if err != nil {
		return afterSeq, err
	}
	cursor := afterSeq
	touched := map[string]bool{}
	for _, e := range events {
		cursor = e.Seq
		if e.RunID == "" {
			continue // platform-scope events never drive Tier-0
		}
		switch e.Type {
		case "tool.completed", "engine.tool_result", // a new tool completion (loop/error signals)
			"run.state_changed", "run.state": // a completion transition (suspicious-completion)
			touched[e.RunID] = true
		}
	}
	for runID := range touched {
		if err := w.EvaluateRun(ctx, runID); err != nil {
			w.logger.Warn("watchdog: Tier-0 evaluation", "run", runID, "err", err)
		}
	}
	return cursor, nil
}

// behavioralWatchStates is the POSITIVE state allowlist the Tier-0
// loop/ping-pong/error-loop counters watch (drain D3/D8): a run actively
// executing. Naming ONLY the good states keeps the forbidden terminal idents out
// of the watchdog (importwall) and — the point of D8 — keeps a crashed run OUT
// of the behavioral watch set (run.IsTerminal(StateCrashed)=false by design, so
// the old IsTerminal gate let a crashed run receive an unresolvable loop flag).
// A crashed/parked/queued/terminal run is simply not in the set.
var behavioralWatchStates = map[run.State]bool{
	run.StateRunning:  true,
	run.StateDraining: true,
}

// completedState is the clean-completion terminal's VALUE (Spec S02.3). The
// suspicious-completion check reads it by value (isCleanCompletion), never via
// the run.StateCompleted ident — that ident is deadman.go's sanctioned
// self-termination exception (importwall D10). A drift guard test asserts this
// value matches run.StateCompleted.
const completedState = "completed"

// isCleanCompletion reports whether s is the clean-completion terminal — the
// only terminal the suspicious-completion check watches (R7/D8). A
// crashed/tombstoned/finalized/died-at-gate run is recovery's, never the
// watchdog's, and is not this value.
func isCleanCompletion(s run.State) bool { return string(s) == completedState }

// EvaluateRun runs Tier-0 over one run's recent events (R1), keyed on a POSITIVE
// state allowlist (drain D3/D8). A run in behavioralWatchStates (running/
// draining) runs the loop / ping-pong / error-loop counters (the first trip
// flags and parks; once parked the run leaves the set, so a re-evaluation is
// inert). A cleanly-COMPLETED run runs the suspicious-completion check. Every
// other state — queued/claimed/new (nothing to watch yet), parked (contained),
// and the recovery-owned terminals (crashed/tombstoned/finalized/died-at-gate) —
// is in NEITHER set and receives no Tier-0 flag.
func (w *Watchdog) EvaluateRun(ctx context.Context, runID string) error {
	r, err := w.runs.Get(ctx, runID)
	if errors.Is(err, run.ErrNotFound) {
		return nil // a deleted/absent run — nothing to evaluate
	}
	if err != nil {
		return err
	}
	if !behavioralWatchStates[r.State] {
		if isCleanCompletion(r.State) {
			// Suspicious-completion reads the WHOLE run UNFENCED (drain-R1): a run
			// that did real work before a resume (generation N) then completed
			// quietly at generation N+1 is not a silent failure — counting only
			// the post-resume generation would falsely flag it (and burn a Tier-1
			// advisory call). The generation fence is BEHAVIORAL-only.
			events, err := w.readRecentEvents(ctx, runID, allGenerations, recentWindow)
			if err != nil {
				return err
			}
			return w.checkSuspiciousCompletion(ctx, r, events)
		}
		return nil // not a Tier-0 watch state (crashed/terminal are recovery's)
	}

	// The behavioral counters read the run's CURRENT generation only (the D1 fence).
	events, err := w.readRecentEvents(ctx, runID, r.Generation, recentWindow)
	if err != nil {
		return err
	}
	tools := toolCompleted(events)

	loopN, err := w.settings.Int(keyLoopRepeat)
	if err != nil {
		return err
	}
	if trig, ok := loopTrip(tools, int(loopN)); ok {
		return w.flag(ctx, r, trig)
	}

	ppN, err := w.settings.Int(keyPingPongCycles)
	if err != nil {
		return err
	}
	if trig, ok := pingPongTrip(tools, int(ppN)); ok {
		return w.flag(ctx, r, trig)
	}

	elN, err := w.settings.Int(keyErrorLoop)
	if err != nil {
		return err
	}
	if trig, ok := errorLoopTrip(tools, int(elN)); ok {
		return w.flag(ctx, r, trig)
	}
	return nil
}

// loopTrip trips when the last n tool completions share one non-empty signature
// (an identical tool call repeated, R2).
func loopTrip(tools []recentEvent, n int) (trigger, bool) {
	if n < 1 || len(tools) < n {
		return trigger{}, false
	}
	last := tools[len(tools)-n:]
	sig := toolSignature(last[0].Payload)
	if sig == "" {
		return trigger{}, false
	}
	for _, e := range last[1:] {
		if toolSignature(e.Payload) != sig {
			return trigger{}, false
		}
	}
	return trigger{
		Rule:      RuleLoop,
		Signature: sig,
		Counts:    map[string]int{"repeats": n},
		Detail:    "identical tool-call signature repeated consecutively",
	}, true
}

// pingPongTrip trips when the last 2*cycles tool signatures form a strict A,B
// alternation (two-pair ping-pong, R3). A "cycle" is one A,B pair.
func pingPongTrip(tools []recentEvent, cycles int) (trigger, bool) {
	if cycles < 1 {
		return trigger{}, false
	}
	need := cycles * 2
	if len(tools) < need {
		return trigger{}, false
	}
	last := tools[len(tools)-need:]
	sigs := make([]string, need)
	for i, e := range last {
		s := toolSignature(e.Payload)
		if s == "" {
			return trigger{}, false // an unsignatured call breaks the pattern (safe)
		}
		sigs[i] = s
	}
	a, b := sigs[0], sigs[1]
	if a == b {
		return trigger{}, false // not an alternation
	}
	for i, s := range sigs {
		want := a
		if i%2 == 1 {
			want = b
		}
		if s != want {
			return trigger{}, false
		}
	}
	return trigger{
		Rule:      RulePingPong,
		Signature: a + "/" + b,
		Counts:    map[string]int{"cycles": cycles},
		Detail:    "two tool-call signatures alternating (ping-pong)",
	}, true
}

// errorLoopTrip trips when the last n tool completions are all errors sharing
// one tool NAME — the same action erroring repeatedly (R4). Keyed on the action
// (tool name), not the full signature, so a retry with drifting args still
// trips.
func errorLoopTrip(tools []recentEvent, n int) (trigger, bool) {
	if n < 1 || len(tools) < n {
		return trigger{}, false
	}
	last := tools[len(tools)-n:]
	name := firstString(last[0].Payload, "tool", "name", "tool_name")
	if name == "" {
		return trigger{}, false
	}
	for _, e := range last {
		if !isToolError(e.Payload) {
			return trigger{}, false
		}
		if firstString(e.Payload, "tool", "name", "tool_name") != name {
			return trigger{}, false
		}
	}
	return trigger{
		Rule:      RuleErrorLoop,
		Signature: name,
		Counts:    map[string]int{"errors": n},
		Detail:    "the same action erroring repeatedly",
	}, true
}

// checkSuspiciousCompletion flags a completed run that produced anomalously low
// tool/verification activity for its class (R7). The v0 floor is a conservative
// STRUCTURAL constant (no ⚙): a run whose class EXPECTS activity (an execution
// or verification role) that ended with ZERO tool completions AND ZERO recorded
// verdicts — the silent-failure class. Ceremony runs (intake/compose) and
// platform runs are exempt. Calibrated post-v0.
func (w *Watchdog) checkSuspiciousCompletion(ctx context.Context, r run.Run, events []recentEvent) error {
	if !classExpectsActivity(r) {
		return nil
	}
	var tools, verdicts int
	for _, e := range events {
		switch e.Type {
		case "tool.completed", "engine.tool_result":
			tools++
		case "verdict.recorded", "verify.round":
			verdicts++
		}
	}
	if tools == 0 && verdicts == 0 {
		return w.flag(ctx, r, trigger{
			Rule:   RuleSuspiciousCompletion,
			Counts: map[string]int{"tool_completed": tools, "verdicts": verdicts},
			Detail: "run completed with zero tool calls and zero verdicts where its class expects them (silent-failure class)",
		})
	}
	return nil
}

// classExpectsActivity reports whether a run's class is expected to produce
// tool/verification activity (R7). Conservative v0 reading: execution and
// verification role runs (the .execute/.verify id suffixes, the B2 walking-
// skeleton naming, CONVENTIONS §11) expect activity; ceremony (intake/compose)
// and platform runs do not.
func classExpectsActivity(r run.Run) bool {
	if r.UserID == run.ActorPlatform {
		return false
	}
	base := stripForkSuffix(r.ID)
	return strings.HasSuffix(base, ".execute") || strings.HasSuffix(base, ".verify")
}

// stripForkSuffix removes trailing `.g<n>` recovery-fork segments so a forked
// role run keeps its role (mirrors metering.stripForkSuffix; that symbol is
// package-private to metering, and the wall keeps this read local).
func stripForkSuffix(runID string) string {
	for {
		i := strings.LastIndex(runID, ".g")
		if i < 0 || i+2 >= len(runID) {
			return runID
		}
		allDigits := true
		for _, c := range runID[i+2:] {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			return runID
		}
		runID = runID[:i]
	}
}
