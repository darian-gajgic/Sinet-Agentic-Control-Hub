// Package automation is the Sinet automation dialect of Spec S08.9: a
// deterministic workflow definition language with Sinet's own parser and
// an on-demand platform executor with NO model in the loop — a
// kind=automation worker's body is one of these documents, living in the
// same store, lifecycle, and battery as every other worker (same schema,
// no second subsystem).
//
// The dialect is strict JSON (the strict-loader precedent, CONVENTIONS
// §14/§15): unknown fields fail loudly, the whole definition is inside the
// compile hash, and there is no runtime-editable body (Spec S08.9
// versioning discipline — anti-patterns banned by name). Outward effects
// NEVER execute directly: every outward step is an explicit approval node
// that materializes as a gated proposal in the S02.7 effect journal —
// reviewed like any 4.2 effect (D7 makes the test execution free).
//
// This package must never import internal/adapters — no engine, no model,
// structurally (a standing conformance test pins the import graph).
package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
)

// DialectVersion is the versioned dialect identity. It fills the
// engine-pin slot of a kind=automation validation record (Spec S08.1), and
// a bump is a mass revalidation event with engine-pin-bump semantics (Spec
// S08.9, S08.10).
const DialectVersion = "sinet-automation/1"

// Errors callers branch on.
var (
	ErrDialect = errors.New("automation: invalid workflow definition")
	// ErrUnknownVerb: a step names a verb the registry does not carry.
	ErrUnknownVerb = errors.New("automation: unknown connector verb")
	// ErrUnapprovedOutward: an outward verb without its explicit approval
	// node — the definition is invalid (Spec S08.9).
	ErrUnapprovedOutward = errors.New("automation: outward step without an explicit approval node")
)

// Workflow is one parsed dialect document.
type Workflow struct {
	// Dialect must equal DialectVersion.
	Dialect string `json:"dialect"`
	// Service is the ONE named service this automation touches (Spec
	// S08.9: equipment = the one named service; egress to that service
	// only).
	Service string `json:"service"`
	Steps   []Step `json:"steps"`
}

// Step is one deterministic workflow step. Verb is "<service>.<action>".
// Approval marks the explicit approval node required on every outward
// step: at execution the step becomes a gated proposal, never a direct
// call.
type Step struct {
	ID       string                     `json:"id"`
	Verb     string                     `json:"verb"`
	Args     map[string]json.RawMessage `json:"args,omitempty"`
	Approval bool                       `json:"approval,omitempty"`
}

// Parse parses and validates a dialect document (strict: unknown fields
// are rejected).
func Parse(body string) (Workflow, error) {
	var wf Workflow
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&wf); err != nil {
		return wf, fmt.Errorf("%w: %v", ErrDialect, err)
	}
	if dec.More() {
		return wf, fmt.Errorf("%w: trailing content after the workflow document", ErrDialect)
	}
	if wf.Dialect != DialectVersion {
		return wf, fmt.Errorf("%w: dialect %q (this build speaks %s)", ErrDialect, wf.Dialect, DialectVersion)
	}
	if strings.TrimSpace(wf.Service) == "" {
		return wf, fmt.Errorf("%w: service required (the one named service, S08.9)", ErrDialect)
	}
	if len(wf.Steps) == 0 {
		return wf, fmt.Errorf("%w: at least one step required", ErrDialect)
	}
	seen := map[string]bool{}
	for i, st := range wf.Steps {
		if strings.TrimSpace(st.ID) == "" {
			return wf, fmt.Errorf("%w: step %d: id required", ErrDialect, i)
		}
		if seen[st.ID] {
			return wf, fmt.Errorf("%w: duplicate step id %q", ErrDialect, st.ID)
		}
		seen[st.ID] = true
		svc, _, ok := strings.Cut(st.Verb, ".")
		if !ok || svc != wf.Service {
			return wf, fmt.Errorf("%w: step %q: verb %q must be <service>.<action> on service %q", ErrDialect, st.ID, st.Verb, wf.Service)
		}
	}
	return wf, nil
}

// VerbSpec declares one connector verb: its outwardness (does it change
// state or communicate externally?) and, for outward verbs, the S02.7
// idempotency class of the proposal it journals.
type VerbSpec struct {
	Outward bool
	Class   gates.Class
	// Fn executes a NON-outward (read) verb deterministically. Outward
	// verbs carry no Fn here — they only journal proposals; execution of
	// an approved proposal is the effects executor's (Spec S02.7).
	Fn func(ctx context.Context, args map[string]json.RawMessage) (json.RawMessage, error)
}

// Verbs is the fixed connector-verb registry seam (Spec S08.9 "fixed
// connector verbs only"). Real connectors land with their packets (B4+);
// tests register fakes.
type Verbs interface {
	Lookup(verb string) (VerbSpec, bool)
}

// VerbMap is a map-backed Verbs.
type VerbMap map[string]VerbSpec

// Lookup implements Verbs.
func (m VerbMap) Lookup(verb string) (VerbSpec, bool) {
	v, ok := m[verb]
	return v, ok
}

// Validate checks a workflow against a verb registry: every verb must
// resolve, and every outward verb must carry its explicit approval node
// (Spec S08.9). Part of battery station 1 for kind=automation.
func Validate(wf Workflow, verbs Verbs) error {
	for _, st := range wf.Steps {
		spec, ok := verbs.Lookup(st.Verb)
		if !ok {
			return fmt.Errorf("%w: %q (step %q)", ErrUnknownVerb, st.Verb, st.ID)
		}
		if spec.Outward && !st.Approval {
			return fmt.Errorf("%w: step %q (%s)", ErrUnapprovedOutward, st.ID, st.Verb)
		}
	}
	return nil
}

// StepOutcome is one executed/proposed step in a Report.
type StepOutcome struct {
	ID string `json:"id"`
	// Kind: "executed" (read verb ran) or "proposed" (outward step
	// journaled as a gated proposal).
	Kind string `json:"kind"`
	// Output is the read verb's result (bounded by the caller's use;
	// stored outputs ride refs).
	Output json.RawMessage `json:"output,omitempty"`
	// EffectID is the journaled proposal id for an outward step.
	EffectID string `json:"effect_id,omitempty"`
}

// Report is one on-demand execution's outcome (Spec S08.9: at v0
// automations run on demand; schedule attachment is v1 surface).
type Report struct {
	Service string        `json:"service"`
	Steps   []StepOutcome `json:"steps"`
}

// ExecInput parameterizes one execution.
type ExecInput struct {
	Workflow Workflow
	// Payload is the triggering sample/real payload steps may reference
	// via {"$from":"payload.<path>"}.
	Payload json.RawMessage
	Verbs   Verbs
	// Journal receives outward steps as gated proposals. Required when the
	// workflow has outward steps.
	Journal *gates.Journal
	// UserID attributes proposals and billing (3.4: triggered runs bill
	// the registrant; on-demand runs the invoker).
	UserID string
	// RunID optionally attributes proposals to a run.
	RunID string
}

// Execute runs one workflow deterministically, sequentially, with no model
// in the loop: read verbs execute; outward steps journal proposals and do
// NOT execute (Spec S08.9). A step's args may reference the payload or a
// prior step's output via {"$from":"payload.<path>"} /
// {"$from":"steps.<id>.<path>"} — resolved by pure lookup.
func Execute(ctx context.Context, in ExecInput) (Report, error) {
	rep := Report{Service: in.Workflow.Service}
	if err := Validate(in.Workflow, in.Verbs); err != nil {
		return rep, err
	}
	outputs := map[string]json.RawMessage{}
	for _, st := range in.Workflow.Steps {
		spec, _ := in.Verbs.Lookup(st.Verb)
		args, err := resolveArgs(st.Args, in.Payload, outputs)
		if err != nil {
			return rep, fmt.Errorf("automation: step %q: %w", st.ID, err)
		}
		if spec.Outward {
			if in.Journal == nil {
				return rep, fmt.Errorf("automation: step %q is outward and no effect journal is wired", st.ID)
			}
			payload, err := proposalPayload(in.Workflow.Service, st, args)
			if err != nil {
				return rep, err
			}
			eff, err := in.Journal.Propose(ctx, gates.Proposal{
				RunID: in.RunID, UserID: in.UserID, Class: spec.Class, Payload: payload,
			})
			if err != nil {
				return rep, fmt.Errorf("automation: journal step %q: %w", st.ID, err)
			}
			rep.Steps = append(rep.Steps, StepOutcome{ID: st.ID, Kind: "proposed", EffectID: eff.ID})
			continue
		}
		out, err := spec.Fn(ctx, args)
		if err != nil {
			return rep, fmt.Errorf("automation: step %q (%s): %w", st.ID, st.Verb, err)
		}
		outputs[st.ID] = out
		rep.Steps = append(rep.Steps, StepOutcome{ID: st.ID, Kind: "executed", Output: out})
	}
	return rep, nil
}

// proposalPayload shapes the journaled proposal for an outward step.
func proposalPayload(service string, st Step, args map[string]json.RawMessage) (json.RawMessage, error) {
	body := map[string]any{
		"kind":    "automation-step",
		"service": service,
		"step":    st.ID,
		"verb":    st.Verb,
		"args":    args,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("automation: marshal proposal: %w", err)
	}
	return raw, nil
}

// Reference reports the {"$from": "<path>"} reference an argument value
// carries, and whether it carries one at all.
//
// This is THE dialect's own edge predicate and it is exported so there is ONE
// expression of it. An argument that is a reference is a DEFINITION-TIME edge —
// the only place a step's dependency on another step is written down — so a
// surface that reads a definition (the S15.10 workforce map) and the executor
// that runs it must agree about which arguments are edges. Two predicates
// hand-written to the same intent drift, and this pair had: a stricter reader
// dropped a legal edge the executor resolves, and served a `{"$from":""}`
// literal as an edge the executor never follows.
//
// The rule, deliberately permissive in the same way the executor is: the value
// is a JSON object carrying a non-empty string `$from`. Sibling keys do NOT
// disqualify it — Step.Args is map[string]json.RawMessage, so the document
// loader's DisallowUnknownFields never reaches inside an argument value and a
// document carrying one parses.
func Reference(raw json.RawMessage) (string, bool) {
	var ref struct {
		From string `json:"$from"`
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' &&
		json.Unmarshal(trimmed, &ref) == nil && ref.From != "" {
		return ref.From, true
	}
	return "", false
}

// resolveArgs resolves {"$from": ...} references; every other value passes
// through verbatim.
func resolveArgs(args map[string]json.RawMessage, payload json.RawMessage, outputs map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	if len(args) == 0 {
		return nil, nil
	}
	out := make(map[string]json.RawMessage, len(args))
	for k, raw := range args {
		if from, ok := Reference(raw); ok {
			v, err := lookupPath(from, payload, outputs)
			if err != nil {
				return nil, err
			}
			out[k] = v
			continue
		}
		out[k] = raw
	}
	return out, nil
}

// lookupPath resolves "payload.a.b" / "steps.<id>.a.b" by pure JSON
// traversal.
func lookupPath(path string, payload json.RawMessage, outputs map[string]json.RawMessage) (json.RawMessage, error) {
	parts := strings.Split(path, ".")
	var doc json.RawMessage
	switch parts[0] {
	case "payload":
		doc = payload
		parts = parts[1:]
	case "steps":
		if len(parts) < 2 {
			return nil, fmt.Errorf("$from %q: steps reference needs a step id", path)
		}
		var ok bool
		doc, ok = outputs[parts[1]]
		if !ok {
			return nil, fmt.Errorf("$from %q: step %q has no output yet (steps run in order)", path, parts[1])
		}
		parts = parts[2:]
	default:
		return nil, fmt.Errorf("$from %q: must start with payload. or steps.", path)
	}
	for _, p := range parts {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(doc, &m); err != nil {
			return nil, fmt.Errorf("$from %q: %q is not an object", path, p)
		}
		v, ok := m[p]
		if !ok {
			return nil, fmt.Errorf("$from %q: key %q not found", path, p)
		}
		doc = v
	}
	return doc, nil
}
