package watchdog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// tier1.go — the S14.4 Tier-1 local-model disambiguator (brief R9–R14). It is
// invoked ONLY on a Tier-0 trigger (never per-event), rides the B4
// `watchdog-disambiguator` duty alias through local.Duty.Call ($0 by
// construction), and ANNOTATES — never gates, never kills (R12; the verdict
// does NOT change whether Tier-2 parks — Tier-2 is always). Low margin ⇒ the
// verdict is `unclear` (Spec S12.4: the local.Duty abstain belt enforces the
// `unclear` member for a Classification call). It never escalates beyond
// `unclear` and never gains a cloud fallback (S12.4/S12.5).

// Verdict values (the {loop|productive|unclear} enum; `unclear` is the
// mandatory abstain member — Spec S12.4).
const (
	VerdictLoop       = "loop"
	VerdictProductive = "productive"
	VerdictUnclear    = "unclear"
)

// disambigMaxTokens is the per-duty length cap (a structural constant, §8
// reading 7 — S18 ratifies no key).
const disambigMaxTokens = 320

// WatchdogSchema is the Tier-1 verdict schema (R10), built HERE in
// internal/watchdog (OQ6 — internal/local stays a read-only consumer). It
// follows the local.schema.go SHAPE: a REQUIRED free-text `reason` FIRST (the
// F5 ordered-schema discipline — the model's reasoning leads the token
// region), then the constrained {loop|productive|unclear} verdict (carrying the
// mandatory `unclear` abstain member), a confidence, and a one-line evidence
// string. Passed to local.Duty.Call, whose assertAbstain belt confirms the
// `unclear` member for a Classification call.
func WatchdogSchema() json.RawMessage {
	// Hand-built so properties emit in the given ORDER (a Go map marshals keys
	// alphabetically, which would reorder the grammar — reason must lead, F5).
	return json.RawMessage(`{"type":"object","properties":{` +
		`"reason":{"type":"string","description":"brief free-text reasoning — fill this FIRST, before the verdict"},` +
		`"verdict":{"type":"string","enum":["loop","productive","unclear"],"description":"loop = stuck repeating; productive = making genuine progress; unclear = cannot tell (abstain — never guess, S12.4)"},` +
		`"confidence":{"type":"number","description":"0.0-1.0 confidence in the verdict"},` +
		`"evidence":{"type":"string","description":"one-line evidence for the verdict"}` +
		`},"required":["reason","verdict","confidence","evidence"],"additionalProperties":false}`)
}

// annotatable reports whether a rule warrants a Tier-1 loop-vs-productive
// judgment (R9/R12): the behavioral loop family + suspicious-completion, where
// a transcript judgment is meaningful. Silence/spend/registered checks have no
// transcript loop to disambiguate — the flag fires without an annotation
// (Tier-2 is always; the verdict never gates).
func annotatable(rule string) bool {
	switch rule {
	case RuleLoop, RulePingPong, RuleErrorLoop, RuleSuspiciousCompletion:
		return true
	default:
		return false
	}
}

// annotate runs the Tier-1 disambiguator and appends watchdog.annotated,
// returning the annotation event's seq (for the flag's annotation_ref, R17), or
// nil when no annotation was produced. It is nil-safe end to end: no duty
// (unconfigured stack) ⇒ nil; a non-annotatable rule ⇒ nil; an error/abstain ⇒
// a recorded `unclear` annotation (never a fabricated verdict, R12). The $0 D7
// row lands on the watched run when it is checkpointable (running/draining —
// trace coherence), else on an AdvisoryMeter platform run (OQ4); with neither,
// the annotation is skipped HONESTLY (logged), never a silent unmetered call.
func (w *Watchdog) annotate(ctx context.Context, r run.Run, trig trigger, events []recentEvent) *int64 {
	if w.duty == nil || !annotatable(trig.Rule) {
		return nil
	}

	// Pick the checkpointable run the $0 D7 row lands on (OQ4).
	var (
		meterRunID string
		settle     func()
	)
	switch {
	case r.State == run.StateRunning || r.State == run.StateDraining:
		meterRunID = r.ID // on the watched run — the disambiguation is about it
	case w.meter != nil:
		id, s, err := w.meter(ctx, "watchdog-disambiguator")
		if err != nil {
			w.logger.Warn("watchdog: Tier-1 advisory meter", "run", r.ID, "err", err)
			return nil
		}
		meterRunID, settle = id, s
		defer settle()
	default:
		w.logger.Info("watchdog: Tier-1 annotation skipped — watched run not checkpointable and no advisory meter wired (honest degrade, R13)", "run", r.ID, "state", r.State)
		return nil
	}

	schema := WatchdogSchema()
	res, err := w.duty.Call(ctx, meterRunID, local.DutyRequest{
		Alias:          local.AliasWatchdog,
		System:         "You judge whether an agent run is STUCK IN A LOOP or making GENUINE PROGRESS from its recent turns. Reason briefly FIRST, then answer verdict ∈ {loop, productive, unclear}. Answer `unclear` when you cannot tell — NEVER guess a label. This is advisory: you annotate, you never stop or kill anything.",
		User:           w.disambiguatorPrompt(r, trig, events, schema),
		Schema:         schema,
		Name:           "watchdog-disambiguator",
		MaxTokens:      disambigMaxTokens,
		Classification: true,
	})

	ann := AnnotationPayload{Rule: trig.Rule, Actor: run.ActorPlatform}
	switch {
	case err != nil:
		// An error/abstain degrades to `unclear` — annotates, never a guess (R12).
		ann.Verdict = VerdictUnclear
		ann.Evidence = "disambiguator unavailable or abstained"
		ann.Reason = err.Error()
		ann.Abstained = true
	default:
		var out struct {
			Reason     string  `json:"reason"`
			Verdict    string  `json:"verdict"`
			Confidence float64 `json:"confidence"`
			Evidence   string  `json:"evidence"`
		}
		if jerr := json.Unmarshal([]byte(res.Content), &out); jerr != nil || !validVerdict(out.Verdict) {
			ann.Verdict = VerdictUnclear
			ann.Evidence = "unparseable disambiguator output"
			ann.Abstained = true
		} else {
			ann.Verdict = out.Verdict
			ann.Confidence = out.Confidence
			ann.Evidence = out.Evidence
			ann.Reason = out.Reason
			ann.Abstained = out.Verdict == VerdictUnclear
		}
		ann.Model = res.Model
	}

	seq, aerr := w.appendAnnotation(ctx, r, ann)
	if aerr != nil {
		w.logger.Warn("watchdog: append annotation", "run", r.ID, "err", aerr)
		return nil
	}
	return &seq
}

// appendAnnotation appends watchdog.annotated run-scoped to the WATCHED run
// (owner + generation, 15.6) so it rides the run's trace and the owner-scoped
// inbox. Re-reads the run's generation immediately before the append so a
// concurrent resume/fork does not trip the fence.
func (w *Watchdog) appendAnnotation(ctx context.Context, r run.Run, ann AnnotationPayload) (int64, error) {
	cur, err := w.runs.Get(ctx, r.ID)
	if err != nil {
		return 0, err
	}
	payload, err := json.Marshal(ann)
	if err != nil {
		return 0, err
	}
	return w.log.Append(ctx, eventlog.Append{
		RunID:         r.ID,
		Generation:    cur.Generation,
		UserID:        cur.UserID,
		Type:          EventAnnotated,
		SchemaVersion: eventSchemaVersion,
		Payload:       payload,
	})
}

// disambiguatorPrompt assembles the last-N turns (tool calls, messages, errors)
// plus the original request into the constrained-decoding prompt (R10). The
// last-N turns are retrieved from the run's OWN recent events (S14.1
// derive-from-log). This is injection-adjacent (untrusted run transcript fed to
// a local model) — the framing keeps the model to the loop/productive judgment.
func (w *Watchdog) disambiguatorPrompt(r run.Run, trig trigger, events []recentEvent, schema json.RawMessage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ORIGINAL REQUEST (task %s): %s\n\n", r.TaskID, originalRequest(events))
	fmt.Fprintf(&b, "TIER-0 TRIGGER: %s — %s\n\n", trig.Rule, trig.Detail)
	b.WriteString("RECENT TURNS (oldest → newest):\n")
	for _, line := range lastTurns(events, 14) {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "\nIs this run stuck in a loop or making genuine progress? Respond with JSON matching this schema:\n%s\n", schema)
	return b.String()
}

// originalRequest best-efforts the run's original request from its earliest
// message/state event (self-contained — the watchdog reads only run_events).
func originalRequest(events []recentEvent) string {
	for _, e := range events {
		if e.Type == "engine.message" || e.Type == "intake.state" || e.Type == "run.created" {
			if s := firstString(e.Payload, "request", "objective", "text", "content", "summary", "line"); s != "" {
				return truncate(s, 400)
			}
		}
	}
	return "(original request not recorded in the run's recent events)"
}

// lastTurns renders the last n events as compact one-liners (type + excerpt),
// oldest-first, never the raw args (the §30 digest discipline).
func lastTurns(events []recentEvent, n int) []string {
	if len(events) > n {
		events = events[len(events)-n:]
	}
	out := make([]string, 0, len(events))
	for _, e := range events {
		excerpt := firstString(e.Payload, "tool", "name", "tool_name", "excerpt", "summary", "line", "reason", "text")
		if e.Type == "tool.completed" || e.Type == "engine.tool_result" {
			tool := firstString(e.Payload, "tool", "name", "tool_name")
			errMark := ""
			if isToolError(e.Payload) {
				errMark = " [ERROR]"
			}
			out = append(out, fmt.Sprintf("- tool %s%s", firstNonEmpty(tool, "?"), errMark))
			continue
		}
		out = append(out, fmt.Sprintf("- %s %s", e.Type, truncate(excerpt, 120)))
	}
	return out
}

func validVerdict(v string) bool {
	return v == VerdictLoop || v == VerdictProductive || v == VerdictUnclear
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
