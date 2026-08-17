package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// Engine-session implementations of the pipeline model seams (Spec S06.10
// duty table; Spec S07.5–S07.6 judge/rework wiring). Each is a prompt
// contract + strict parse over ONE fresh stage session — trust stays in
// the platform validation (the intake spine, the judge validators), never
// in the seam (Spec S06.6). Sessions are tool-less (`--tools ""`): the
// duty is to REASON and emit a structured result, and the S06.6 C1
// read-only posture composes when confinement is active.

// Stage marker kinds (the prompt-frame duty header, runner.go).
const (
	markerPlanDraft  = "plan-draft"
	markerPlanRevise = "plan-revise"
	markerCritique   = "critique"
	markerExecute    = "execute"
	markerCompliance = "judge-compliance"
	markerSanity     = "judge-sanity"
	markerRevise     = "revise"
)

// parseJSONOutput parses a session's structured output: the final message
// must be one JSON object (optionally fenced). A parse failure is a seam
// error the pipeline surfaces — never silently absorbed. It is the plain
// form of parseJSONCompleting, for callers with nobody to report a
// structural completion to.
func parseJSONOutput(text string, into any) error {
	_, err := parseJSONCompleting(text, into)
	return err
}

// maxJSONCompletionClosers bounds the delimiter-only structural completion
// (P3-RW-16 R2): a reply missing more than this many closing delimiters is
// not the proven truncation class, it is a reply that stopped somewhere
// arbitrary, and it keeps the loud error. Plain constant, not a ⚙ — S03
// ratifies no parse-repair key (CONVENTIONS §7, the sseBatchSize/
// cancelGrace precedent).
const maxJSONCompletionClosers = 8

// parseJSONCompleting is parseJSONOutput plus the report R3's loudness
// needs: how many closing delimiters the bounded completion had to append
// (0 = the reply parsed exactly as the engine sent it).
//
// The completion exists because the engine side itself truncates: on the
// drain-r1 walk 6 of 11 plan/critique sessions ended their final assistant
// text exactly one closing brace short at stop_reason end_turn — the byte
// never reached the adapter, which handed on what it was given, verbatim
// and unrepaired (S03.1/RW-9). Trust stays in the platform validation, not
// in the seam (S06.6): the completion adds NOTHING but computable closing
// delimiters, and every downstream semantic check still runs on the result.
func parseJSONCompleting(text string, into any) (int, error) {
	t := strings.TrimSpace(text)
	// A ``` inside a JSON string value is CONTENT, not a fence (R4): strip
	// only when the first fence precedes the first '{' of the reply, i.e.
	// when it really is the opening fence of a fenced block.
	if i := strings.Index(t, "```"); i >= 0 {
		if b := strings.IndexByte(t, '{'); b < 0 || i < b {
			t = t[i+3:]
			t = strings.TrimPrefix(t, "json")
			if j := strings.LastIndex(t, "```"); j >= 0 {
				t = t[:j]
			}
		}
	}
	t = strings.TrimSpace(t)
	if i := strings.IndexByte(t, '{'); i > 0 {
		t = t[i:]
	}
	raw, closers := []byte(t), 0
	if !json.Valid(raw) {
		if c, ok := jsonPrefixClosers(t); ok {
			if completed := append([]byte(t), c...); json.Valid(completed) {
				raw, closers = completed, len(c)
			}
		}
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return 0, fmt.Errorf("stage: session output is not the contracted JSON object: %w", err)
	}
	return closers, nil
}

// jsonPrefixClosers reports the closing delimiters that finish t — and
// reports them only when t is a syntactically valid JSON document truncated
// by nothing but its own trailing delimiters (R2's walls):
//
//   - the cut lies OUTSIDE any string (a cut inside one would need a quote
//     closed, and the platform never closes a quote — the string's content
//     is model-authored and its end is unknowable);
//   - the cut lies at a value-complete resting place: immediately after a
//     complete value or a closed container. Never after a dangling ',' or
//     ':' (a value would have to be invented), never after an object key
//     awaiting its ':', and never on a container that was OPENED and holds
//     nothing yet — closing that recovers no content at all while spending
//     the bounded re-ask the seam would otherwise get;
//   - never inside a number literal: a cut number is a DIFFERENT number
//     (12345 cut to 1), and completing it would change a value that was
//     present rather than restore one that was missing;
//   - every open container closes from the bracket stack alone, bounded by
//     maxJSONCompletionClosers.
//
// Empty, whitespace and prose inputs open no container, so they never reach
// an eligible state and keep today's error.
func jsonPrefixClosers(t string) (string, bool) {
	const (
		atStart = iota // before the document's one value
		atOpen         // just after '{' or '['
		atKey          // just after an object key string (awaiting ':')
		atColon        // just after ':'
		atValue        // just after a complete value (or a closed container)
		atComma        // just after ','
	)
	var stack []byte
	state := atStart
	topIs := func(b byte) bool { return len(stack) > 0 && stack[len(stack)-1] == b }
	// valueStart reports whether a fresh value may begin at this resting
	// place: the top-level value, an object member's value, or an array
	// element.
	valueStart := func() bool {
		switch state {
		case atStart:
			return len(stack) == 0
		case atColon:
			return topIs('{')
		case atOpen, atComma:
			return topIs('[')
		}
		return false
	}
	for i := 0; i < len(t); {
		switch c := t[i]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '{' || c == '[':
			if !valueStart() {
				return "", false
			}
			stack = append(stack, c)
			state, i = atOpen, i+1
		case c == '}' || c == ']':
			open := byte('{')
			if c == ']' {
				open = '['
			}
			if !topIs(open) || (state != atOpen && state != atValue) {
				return "", false
			}
			stack = stack[:len(stack)-1]
			state, i = atValue, i+1
		case c == ':':
			if state != atKey {
				return "", false
			}
			state, i = atColon, i+1
		case c == ',':
			if state != atValue || len(stack) == 0 {
				return "", false
			}
			state, i = atComma, i+1
		case c == '"':
			end, ok := scanJSONString(t, i)
			if !ok {
				return "", false // the cut lies inside a string
			}
			key := topIs('{') && (state == atOpen || state == atComma)
			if !key && !valueStart() {
				return "", false
			}
			if key {
				state = atKey
			} else {
				state = atValue
			}
			i = end
		case c == '-' || (c >= '0' && c <= '9'):
			if !valueStart() {
				return "", false
			}
			end := i + 1
			for end < len(t) && isJSONNumberByte(t[end]) {
				end++
			}
			if end == len(t) {
				return "", false // a cut number is a different number
			}
			state, i = atValue, end
		default:
			var lit string
			switch c {
			case 't':
				lit = "true"
			case 'f':
				lit = "false"
			case 'n':
				lit = "null"
			default:
				return "", false
			}
			if !valueStart() || !strings.HasPrefix(t[i:], lit) {
				return "", false
			}
			state, i = atValue, i+len(lit)
		}
	}
	if len(stack) == 0 || len(stack) > maxJSONCompletionClosers {
		return "", false // nothing to complete, or not the truncation class
	}
	if state != atValue {
		return "", false
	}
	closers := make([]byte, 0, len(stack))
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == '{' {
			closers = append(closers, '}')
		} else {
			closers = append(closers, ']')
		}
	}
	return string(closers), true
}

// scanJSONString returns the index just past the string starting at t[i]
// (which must be its opening quote), or false when the text ends before the
// closing quote — including a text ending mid-escape.
func scanJSONString(t string, i int) (int, bool) {
	for j := i + 1; j < len(t); j++ {
		switch t[j] {
		case '\\':
			j++ // whatever it escapes is content
		case '"':
			return j + 1, true
		}
	}
	return 0, false
}

func isJSONNumberByte(c byte) bool {
	return (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-'
}

// jsonRetryLimit bounds the re-asks of a JSON-contract session whose
// reply failed to parse — the house bounce-once pattern (cf. intake's
// autofix bounds and verify's v0RegenerationLimit). Engines occasionally
// answer prose instead of the contracted object; one fresh session with
// the contract restated recovers that, and a second violation errors up
// to the caller's ladder — never absorbed, never an unbounded loop.
// (Found live at the B2 gate demo, 2026-07-20: the axis-1 judge opened
// with prose and the whole verify run crashed for it.)
const jsonRetryLimit = 1

const jsonRetryNote = "\nYour previous reply was rejected: it was not the contracted JSON object. " +
	"Reply with ONLY the JSON object — no prose, no preamble, no trailing text.\n"

// jsonWithRetry drives a JSON-contract session through runSession (called
// with a retry note to append on re-asks) and parses the reply into
// `into`, re-asking at most jsonRetryLimit times on parse failure. The
// plain form: a caller that can name the session uses
// jsonWithRetryReporting so a structural completion stays loud.
func jsonWithRetry(runSession func(retryNote string) (string, error), into any) error {
	return jsonWithRetryReporting(runSession, into, nil)
}

// jsonWithRetryReporting is jsonWithRetry with the R2 completion reported to
// the caller that knows who the session was (run/stage/kind): a structural
// completion is never silently absorbed (S06.6), and onCompletion fires once
// per attempt whose reply had to be completed.
func jsonWithRetryReporting(runSession func(retryNote string) (string, error), into any, onCompletion func(closers int)) error {
	parse := func(text string) error {
		n, err := parseJSONCompleting(text, into)
		if err == nil && n > 0 && onCompletion != nil {
			onCompletion(n)
		}
		return err
	}
	text, err := runSession("")
	if err != nil {
		return err
	}
	perr := parse(text)
	for try := 0; perr != nil && try < jsonRetryLimit; try++ {
		if text, err = runSession(jsonRetryNote); err != nil {
			return err
		}
		perr = parse(text)
	}
	return perr
}

// jsonSession runs one stage session under the JSON output contract with
// the bounded re-ask.
func (s *Skeleton) jsonSession(ctx context.Context, in SessionInput, into any) error {
	return jsonWithRetryReporting(func(retryNote string) (string, error) {
		att := in
		att.Instructions = in.Instructions + retryNote
		res, err := s.Session(ctx, att)
		if err != nil {
			return "", err
		}
		return res.Text, nil
	}, into, func(closers int) {
		// Counts and platform-authored names only — nothing the model wrote
		// lands in the log line (S01.11).
		s.logger().Warn("stage: session reply was truncated mid-delimiter; completed it structurally (P3-RW-16 R2)",
			"run", in.RunID, "stage", in.Stage, "kind", sessionKind(in), "closers", closers)
	})
}

// ---- intake.Planner (Spec S06.10: planning-model duty; wired B2-4) ----

// EnginePlanner drives the Stage-1 drafting/revision sessions on the
// task's intake run. The session input is the ledger-assembled stage brief
// plus the DraftInput serialized as a manifested Extra item (Spec S05.4:
// every injected item logged); the pipeline validates whatever comes back.
type EnginePlanner struct{ s *Skeleton }

var _ intake.Planner = (*EnginePlanner)(nil)

const pairSchema = `Output EXACTLY one JSON object, nothing else, shaped:
{"spec":{"restatement":string, "outcome":[string...],
  "acs":[{"n":1,"plain":string,"structured":string?,"structured_kind":"ears"|"gwt"|""}...],
  "constraints":[string...], "assumptions":[{"text":string,"origin":string}...],
  "out_of_scope":[string...], "clarifications":[string...]},
 "plan":{"steps":[{"id":"S-1","title":string,"done_when":string,"class":"C1"|"C2",
   "write_set":[glob...], "outward_effects":[string...], "new_spend":bool,
   "credential_touch":bool, "shared_asset_write":bool, "research":bool}...],
  "coverage":{"AC-1":["S-1"],...}, "research_nodes":[{"rule_id":string,"step_id":"S-n","query":string}...],
  "risks":[string...], "est":{"size_class":string,"usd":number,"known":bool,"basis":string}}}
Rules: acceptance criteria numbered 1..n contiguously, plain phrasing on every
criterion; steps keyed S-1..n, each with a testable done_when and a confinement
class; EVERY AC key appears in coverage; a research node per open data-bearing
flag; unresolved consequential ambiguities become NEEDS-CLARIFICATION entries
in clarifications (they block approval); prefer the smallest honest plan.`

func (p *EnginePlanner) Draft(ctx context.Context, in intake.DraftInput) (intake.Pair, error) {
	extra, err := draftInputItem(in)
	if err != nil {
		return intake.Pair{}, err
	}
	instructions := stageMarker(markerPlanDraft) + fmt.Sprintf(
		"You are the planning model for task %s (family %s, tier %s): conduct Stage-1 drafting (Spec S06.6).\n"+
			"Read the interview record in the %s block above; honor registry-supplied facts and recorded assumptions.\n"+
			"Emit SPEC version %d and PLAN version %d (plan.spec_version %d).\n%s",
		in.Request.TaskID, in.Family, in.Tier, itemDraftInput, in.SpecVersion, in.PlanVersion, in.SpecVersion, pairSchema)
	return p.pairSession(ctx, in.RunID, in.Request.TaskID, in.Request.UserID, in.Tier, extra, markerPlanDraft, instructions,
		in.SpecVersion, in.PlanVersion)
}

func (p *EnginePlanner) Revise(ctx context.Context, in intake.ReviseInput) (intake.Pair, error) {
	extra, err := reviseInputItem(in)
	if err != nil {
		return intake.Pair{}, err
	}
	instructions := stageMarker(markerPlanRevise) + fmt.Sprintf(
		"You are the planning model revising the SPEC/PLAN pair for task %s (revision reason: %s).\n"+
			"Address EXACTLY the numbered findings/resolutions in the %s block above — criteria do not drift (Spec S06.7(a)/S06.8).\n"+
			"Emit SPEC version %d and PLAN version %d (plan.spec_version %d).\n%s",
		in.Pair.Spec.TaskID, in.Reason, itemReviseInput, in.SpecVersion, in.PlanVersion, in.SpecVersion, pairSchema)
	return p.pairSession(ctx, in.RunID, in.Pair.Spec.TaskID, in.Pair.Spec.Owner, in.Pair.Spec.Tier, extra, markerPlanRevise, instructions,
		in.SpecVersion, in.PlanVersion)
}

// pairSession runs one planning session on the CONSUMING run the pipeline
// handed us (P3-RW-6 R7). It is never suffix-composed from the task id: on a
// recovery-fork intake run that composition names the SUPERSEDED PARENT, whose
// generation the event-log fence rejects and whose crashed state makes a
// checkpoint unwritable — the judge seam states the same rule (CONVENTIONS §16).
// An unset run id keeps the birth composition (an unrebound caller, tests).
func (p *EnginePlanner) pairSession(ctx context.Context, runID, taskID, owner string, tier intake.Tier,
	extra ledger.Item, kind, instructions string, specV, planV int) (intake.Pair, error) {
	if runID == "" {
		runID = taskID + RunSuffixIntake
	}
	var pair intake.Pair
	err := p.s.jsonSession(ctx, SessionInput{
		RunID:    runID,
		Stage:    "plan",
		Assemble: true,
		// Knowledge wired at B3-1 (Spec S09.3 house/project/user slices);
		// conventions/worker sources remain B4/B3-2 seams.
		Sources:      ledger.Sources{Knowledge: p.s.cfg.Knowledge},
		Extra:        []ledger.Item{extra},
		Instructions: instructions,
		Kind:         kind,
		Class:        "C1", // read-only planning sandbox (Spec S06.6, P-T05-1)
	}, &pair)
	if err != nil {
		return intake.Pair{}, fmt.Errorf("planner session: %w", err)
	}
	// Platform-owned bookkeeping fields are stamped here, not trusted from
	// the engine (the spine re-validates content; identity/version
	// bookkeeping is the platform's).
	pair.Spec.TaskID, pair.Plan.TaskID = taskID, taskID
	pair.Spec.Owner, pair.Plan.Owner = owner, owner
	pair.Spec.Version, pair.Plan.Version = specV, planV
	pair.Plan.SpecVersion = specV
	pair.Spec.Status, pair.Plan.Status = intake.StatusDraft, intake.StatusDraft
	pair.Spec.Tier, pair.Plan.Tier = tier, tier
	prov := "planning-model session (" + p.s.modelFor("") + ")"
	pair.Spec.Provenance, pair.Plan.Provenance = prov, prov
	if err := pair.Validate(); err != nil {
		return intake.Pair{}, fmt.Errorf("planner output: %w", err)
	}
	return pair, nil
}

// Draft-input item identities on the assembly manifest (Spec S05.4).
const (
	itemDraftInput  = "intake/draft-input"
	itemReviseInput = "intake/revise-input"

	selectorIntake = "intake pipeline stage input (S06.5/S06.6) — interview record / revision findings"
)

func draftInputItem(in intake.DraftInput) (ledger.Item, error) {
	// The serialized interview record: everything Stage 1 receives (Spec
	// S06.5–S06.6). The taxonomy is reduced to slot id → question so the
	// resolutions read meaningfully without the full weight table.
	slots := map[string]string{}
	if in.Taxonomy != nil {
		for _, s := range in.Taxonomy.Slots {
			slots[s.ID] = s.Question
		}
	}
	body, err := json.MarshalIndent(struct {
		Request     intake.Request            `json:"request"`
		Family      intake.Family             `json:"family"`
		Tier        intake.Tier               `json:"tier"`
		Slots       map[string]string         `json:"slots,omitempty"`
		Resolutions []intake.SlotResolution   `json:"resolutions,omitempty"`
		DataHits    []intake.TriggerHit       `json:"data_hits,omitempty"`
		Supplied    []intake.SuppliedFact     `json:"supplied,omitempty"`
		Escalations []intake.EscalationAnswer `json:"escalations,omitempty"`
		Prior       *intake.Pair              `json:"prior,omitempty"`
	}{in.Request, in.Family, in.Tier, slots, in.Resolutions, in.DataHits, in.Supplied, in.Escalations, in.Prior}, "", "  ")
	if err != nil {
		return ledger.Item{}, fmt.Errorf("stage: marshal draft input: %w", err)
	}
	return ledger.Item{
		ItemID:       itemDraftInput,
		SourcePath:   "platform:" + itemDraftInput,
		Content:      string(body),
		Version:      fmt.Sprintf("spec-v%d", in.SpecVersion),
		SelectorRule: selectorIntake,
		Precedence:   ledger.PrecedenceStage,
	}, nil
}

func reviseInputItem(in intake.ReviseInput) (ledger.Item, error) {
	body, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return ledger.Item{}, fmt.Errorf("stage: marshal revise input: %w", err)
	}
	return ledger.Item{
		ItemID:       itemReviseInput,
		SourcePath:   "platform:" + itemReviseInput,
		Content:      string(body),
		Version:      fmt.Sprintf("plan-v%d", in.PlanVersion),
		SelectorRule: selectorIntake,
		Precedence:   ledger.PrecedenceStage,
	}, nil
}

// ---- intake.Critic (Spec S06.8: planning-model class, fresh context,
// artifact-only input) ----

// EngineCritic drives the Stage-3 self-attack session. ARTIFACT-ONLY: the
// prompt is built from the pair alone — no ledger assembly, no interview
// record, no transcript; the fresh session IS the context separation
// (Spec S06.8; the seam signature admits nothing else).
type EngineCritic struct{ s *Skeleton }

var _ intake.Critic = (*EngineCritic)(nil)

const verdictSchema = `Output EXACTLY one JSON object:
{"kind":"pass"|"revise"|"spec_doubt"|"tier_up",
 "findings":[string...],   // numbered blocker findings (revise)
 "doubt":string,           // plain-language why (spec_doubt)
 "proposed_tier":"low"|"standard"|"high"}  // (tier_up)`

func (c *EngineCritic) Critique(ctx context.Context, pair intake.Pair) (intake.Verdict, error) {
	return c.session(ctx, pair, nil)
}

func (c *EngineCritic) Recheck(ctx context.Context, pair intake.Pair, findings []string) (intake.Verdict, error) {
	return c.session(ctx, pair, findings)
}

func (c *EngineCritic) session(ctx context.Context, pair intake.Pair, recheck []string) (intake.Verdict, error) {
	pairJSON, err := json.MarshalIndent(pair, "", "  ")
	if err != nil {
		return intake.Verdict{}, fmt.Errorf("stage: marshal pair: %w", err)
	}
	var b strings.Builder
	b.WriteString(stageMarker(markerCritique))
	b.WriteString("You are the plan critic (Spec S06.8): fresh context, artifact-only input — attack this SPEC/PLAN pair.\n")
	b.WriteString("Hunt: unmet spec intent, untestable done-when contracts, hidden effects/spend, wrong stakes tier, plans that cannot work.\n")
	b.WriteString("SPEC-DOUBT is mandatory when the specification itself is the problem — never soften it into a note.\n")
	if len(recheck) > 0 {
		fmt.Fprintf(&b, "RE-CHECK ONLY these prior findings (judge nothing new): %s\n", strings.Join(recheck, "; "))
	}
	b.WriteString("\n=== the pair ===\n")
	b.Write(pairJSON)
	b.WriteString("\n\n" + verdictSchema + "\n")

	// The Critic seam admits nothing but the pair — that signature IS the S06.8
	// context separation and this packet does not widen it. So the CONSUMING run
	// is read from the pipeline's current state instead (the engineRevise
	// LoadState precedent below): after a recovery-fork rebind that is the fork,
	// never the superseded parent (P3-RW-6 R7; CONVENTIONS §16 run-id rule).
	var v intake.Verdict
	err = c.s.jsonSession(ctx, SessionInput{
		RunID:        c.s.currentIntakeRun(ctx, pair.Spec.TaskID),
		Stage:        "critique",
		Assemble:     false, // artifact-only (Spec S06.8)
		Instructions: b.String(),
		Kind:         markerCritique,
		Class:        "C1",
	}, &v)
	if err != nil {
		return intake.Verdict{}, fmt.Errorf("critic session: %w", err)
	}
	switch v.Kind {
	case intake.VerdictPass, intake.VerdictRevise, intake.VerdictSpecDoubt, intake.VerdictTierUp:
	default:
		return intake.Verdict{}, fmt.Errorf("critic output: unknown verdict kind %q", v.Kind)
	}
	return v, nil
}

// ---- verify.Judge (Spec S07.5: two separately prompted axes; wired B2-4) ----

// EngineJudge drives the V2 judge sessions on the task's verify run. The
// input slice is EXACTLY JudgeInput — built by verify.BuildJudgeInput
// through the ledger's clean-context assembly; there is no transcript
// anywhere (Spec S07.5). Selection/pairing mechanics are Spec S08's (B3):
// at B2-4 the judge is the configured dev model, honestly reported
// self-family (executor and judge share the one wired lane).
type EngineJudge struct{ s *Skeleton }

var _ verify.Judge = (*EngineJudge)(nil)

const axis1Schema = `Output EXACTLY one JSON object:
{"verdicts":[{"key":"AC-1","pass":bool,"unknown":bool,"evidence":string}...],
 "findings":[{"severity":"blocker"|"note","category":"ac-blocker","criterion":"AC-n","anchor":string,"text":string}...],
 "escalate":string}  // non-empty ONLY to escalate instead of another round
Rules: one verdict per numbered AC; evidence MUST be an EXACT substring of the
artifact (extractive quote) — a PASS without one is forced Unknown; every
blocker cites the frozen criterion it violates.`

const axis2Schema = `Output EXACTLY one JSON object:
{"probe_notes":{"reasonable-user":string,"implicit-expectations":string,"side-effects":string,"expert-standard":string},
 "findings":[{"severity":"blocker"|"note","category":"sanity-blocker","criterion":string,"anchor":string,"text":string}...],
 "reopen_spec":string,  // non-empty ONLY when the SPECIFICATION itself is wrong (files a human decision card)
 "escalate":string}
Rules: all four probes answered; unrequested changes are failures, not bonuses.`

func (j *EngineJudge) Compliance(ctx context.Context, in verify.JudgeInput) (verify.Axis1Result, error) {
	instructions := stageMarker(markerCompliance) +
		"You are the spec-compliance judge (Spec S07.5 axis 1): one binary verdict per numbered AC over the input slice above.\n" +
		axis1Schema + "\n"
	var out verify.Axis1Result
	if err := j.session(ctx, in, markerCompliance, instructions, &out); err != nil {
		return verify.Axis1Result{}, err
	}
	return out, nil
}

func (j *EngineJudge) Sanity(ctx context.Context, in verify.JudgeInput) (verify.Axis2Result, error) {
	instructions := stageMarker(markerSanity) +
		"You are the outcome-sanity judge (Spec S07.5 axis 2), separately prompted with your own mini-rubric.\n" +
		"Probes: would a reasonable user consider this done and good? what would a well-informed person expect that is absent? " +
		"any unrequested side effects? does it meet the expert standard?\n" +
		axis2Schema + "\n"
	var out verify.Axis2Result
	if err := j.session(ctx, in, markerSanity, instructions, &out); err != nil {
		return verify.Axis2Result{}, err
	}
	return out, nil
}

func (j *EngineJudge) session(ctx context.Context, in verify.JudgeInput, kind, instructions string, out any) error {
	// BuildJudgeInput already ran the clean-context assembly (manifest
	// appended); the brief is re-used for pinned placement so a compaction
	// inside the judge session re-injects the frozen ACs (Spec S05.7). The
	// session rides the brief's own run id — never a suffix-composed one,
	// which would name the superseded parent on a recovery-fork verify run.
	brief := in.Brief
	err := j.s.jsonSession(ctx, SessionInput{
		RunID:        in.Brief.RunID,
		Stage:        "verify",
		Assemble:     false,
		PlaceBrief:   &brief,
		Instructions: in.BriefText + "\n" + instructions,
		Kind:         kind,
		Class:        "C1",
	}, out)
	if err != nil {
		return fmt.Errorf("judge output: %w", err)
	}
	return nil
}

// Meta implements verify.Judge. SelfFamily is honestly TRUE at B2-4: the
// one wired lane serves executor and judge alike, and the flag rides the
// receipt (G1 Def.1); the dissimilar-lane swap is S08 selection (B3).
func (j *EngineJudge) Meta() verify.JudgeMeta {
	return verify.JudgeMeta{Model: j.s.modelFor(""), SelfFamily: true}
}

// ---- verify.Revise (Spec S07.6: fresh-session rework executor) ----

// engineRevise is the fresh-session executor seam: each round builds a NEW
// session from EXACTLY the retry package (fork-don't-poison; the package
// members are the whole world — no ledger assembly, no history).
func (s *Skeleton) engineRevise(ctx context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
	pkgJSON, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return verify.Deliverable{}, fmt.Errorf("stage: marshal retry package: %w", err)
	}
	instructions := stageMarker(markerRevise) + fmt.Sprintf(
		"You are a fresh executor session for rework round %d (Spec S07.6).\n"+
			"The retry package below is your ENTIRE input: the original SPEC, the frozen ACs, the numbered findings with anchors, and the current deliverable.\n"+
			"Fix EXACTLY the named problems; never regenerate blind; change nothing already correct.\n"+
			"Output ONLY the complete corrected deliverable content — no commentary, no JSON wrapper.\n\n=== retry package ===\n%s\n",
		pkg.Round, pkgJSON)
	// Rework regenerates the DELIVERABLE — it rides the task's recorded
	// S08.8 selection (the execution seat), not the ceremony seat; absent a
	// record (test posture) modelFor falls through as documented.
	reviseModel, reviseWindow := "", int64(0)
	if st, err := s.pipe.LoadState(ctx, pkg.Deliverable.TaskID); err == nil && st.Routing != nil {
		reviseModel, reviseWindow = st.Routing.Model, st.Routing.WindowTokens
	}
	res, err := s.Session(ctx, SessionInput{
		RunID:        pkg.Deliverable.RunID,
		Stage:        fmt.Sprintf("revise-r%d", pkg.Round),
		Assemble:     false,
		Instructions: instructions,
		Kind:         markerRevise,
		Class:        "C1",
		Model:        reviseModel,
		WindowTokens: reviseWindow,
	})
	if err != nil {
		return verify.Deliverable{}, fmt.Errorf("revise session: %w", err)
	}
	d := pkg.Deliverable
	d.PrevContent = d.Content
	d.Content = res.Text
	d.Revision = pkg.Deliverable.Revision + 1
	d.Diff = "" // revision diff mechanics are Spec S13's (B4); the judge receives the full artifact
	// Every rework revision persists to the artifact store: the round
	// record's (revision, sha256) pin must denote retrievable content — the
	// card's best-effort state is durable and the S07.7 resume retry
	// package reads it back (B2-5; full revision mechanics are Spec S13's).
	if _, _, err := s.writeDeliverable(d.TaskID, d.Revision, d.Content); err != nil {
		return verify.Deliverable{}, fmt.Errorf("stage: persist rework revision: %w", err)
	}
	return d, nil
}
