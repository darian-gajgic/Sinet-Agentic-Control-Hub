package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker/automation"
)

// roster.go — the READ a workforce map needs (Spec S15.10: "a readable
// graphical view of the machinery — every worker, what each is equipped with
// (tools, knowledge, permissions, helpers), and how multi-stage procedures
// connect, rendered from the S08 worker registry"). Everything here is a
// read: no verb, no wall and no column moves, and the S08.2 guardrail split
// is untouched — this reports what the control plane already wrote.
//
// WHY IT LIVES HERE RATHER THAN IN THE TRANSPORT (B6-8 OQ3, ratified — the
// browse.go declared-widening precedent, reported and not silent). The
// equipment a worker requests lives in its version's FILE, and the walk that
// reads one is activeDefinition, which performs the S08.3 hash check on every
// read. A transport assembling the same answer from SQL would be a second
// reader of one identity — the twin-maintained-copy hazard — and a reader with
// no tamper check at that. The delivery policy has the same property: it is
// already the ONE expression of S08.7's structural enforcement, so this read
// calls it rather than re-deriving degraded-mode consequences beside it.

// rosterVersionCap bounds one template's served version history. Version rows
// are immutable and a household roster carries few, but expectation is not a
// bound and every read here shares the control plane's one writer connection
// (Spec S02.1) — the browse.go entryConflictsCap reasoning.
const rosterVersionCap = 200

// RosterQuery bounds one workforce read. Viewer and Limit are required.
//
// The visibility line is S08's own and it is DELIBERATELY not memory's
// (browse.go has no operator limb): a worker is audited platform machinery
// whose approval, promotion and guardrails are all D10 operator acts, so the
// operator reading the roster is the runs/telemetry posture rather than a read
// into somebody's personal content. The content line still binds conversations
// and personal memory; it does not reach the machinery that does the work.
// Spec S15.10's own rule is the member limb: "personal workers to their owner,
// the shared roster to all".
type RosterQuery struct {
	Viewer string
	// Operator lifts the read to every row (D10).
	Operator bool
	// Limit bounds the templates returned, newest-updated first.
	Limit int
}

// RosterEntry is one worker as an audit surface reads it: the registry row,
// its domain and delivery posture, the ACTIVE version's behavioral facts from
// the hash-verified file, and the immutable version history with each
// version's granted guardrails and dated stamps.
type RosterEntry struct {
	Template Template
	Domain   Domain
	Delivery DeliveryPolicy
	// Definition is the active version's parsed file; nil when there is no
	// active version or the walk failed.
	Definition *RosterDefinition
	// DefinitionError is why Definition is nil — an unapproved draft has no
	// active version, and a tamper or parse failure is a fact this surface
	// must state rather than hide (S08.3).
	DefinitionError string
	Versions        []RosterVersion
	// VersionsTruncated reports the cap above cutting the history short.
	VersionsTruncated bool
}

// RosterDefinition is the active version's behavioral definition: the routing
// selector text, the execution profile, the REQUESTED equipment block, the
// eval hooks, and — for kind=automation — the parsed multi-stage chain.
type RosterDefinition struct {
	Description string
	Selectors   SelectorSet
	Profile     ExecutionProfile
	// Equipment is what the FILE requests (Spec S08.1). What is GRANTED is
	// each version's guardrails row; the two are separate fields because the
	// S08.2 split is the thing an audit surface exists to show.
	Equipment Equipment
	Eval      EvalHooks
	Persona   []string
	// Workflow is the S08.9 step chain, present only for kind=automation.
	Workflow *RosterWorkflow
	// WorkflowError is why an automation body could not be parsed.
	WorkflowError string
}

// RosterWorkflow is the S08.9 multi-stage chain as an audit surface reads it:
// the dialect, the ONE named service, the ordered steps with their explicit
// approval nodes marked, and the REFERENCES that wire the steps to each other.
type RosterWorkflow struct {
	Dialect string       `json:"dialect"`
	Service string       `json:"service"`
	Steps   []RosterStep `json:"steps"`
}

// RosterStep is one step of the chain. Approval marks the explicit approval
// node every outward step carries (Spec S08.9; D7).
//
// REFERENCES ARE SERVED, LITERAL ARGS ARE NOT, and the split is the whole point.
// A `{"$from":"steps.fetch.summary"}` arg is not a value flowing at run time —
// it is a DEFINITION-TIME edge saying this step consumes that one's output, and
// it is the only place the chain's shape is written down (S08.9's resolver:
// `{"$from":"payload.<path>"}` / `{"$from":"steps.<id>.<path>"}`, resolved by
// pure lookup). Dropping it leaves a sequence with approval flags and no
// connections at all, which under-delivers the one thing S15.10 asks for — "how
// multi-stage procedures connect". A LITERAL arg is genuinely a value, carries
// producer content, and stays off a view-only surface.
type RosterStep struct {
	ID       string `json:"id"`
	Verb     string `json:"verb"`
	Approval bool   `json:"approval"`
	// Needs maps this step's argument name to the reference it reads, verbatim
	// (`payload.day`, `steps.fetch.summary`). Empty when the step takes only
	// literals or no arguments.
	Needs map[string]string `json:"needs,omitempty"`
}

// RosterVersion is one immutable version row with its enforcement state and
// its dated records.
type RosterVersion struct {
	Version Version
	Active  bool
	// Granted is the worker_guardrails row; nil when the version was never
	// approved, because approval is the ONLY writer of enforcement state
	// (Spec S08.2).
	Granted *Guardrails
	// Validation is the newest validation_records row; nil when the battery
	// has never run for this version.
	Validation *ValidationRecord
	// Revalidation is the newest dated revalidation stamp; nil when the
	// version has never been revalidated (Spec S14.8 ¶3).
	Revalidation *RevalidationStamp
}

// Roster reads every worker VISIBLE to the caller, newest-updated first.
func (s *Store) Roster(ctx context.Context, q RosterQuery) ([]RosterEntry, error) {
	if q.Viewer == "" {
		return nil, fmt.Errorf("%w: a roster read needs a viewer (S01.9)", ErrInvalid)
	}
	if q.Limit <= 0 {
		return nil, fmt.Errorf("%w: a roster read needs a positive limit", ErrInvalid)
	}
	where, args := rosterVisible(q)
	args = append(args, q.Limit)
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+templateColumns+` FROM worker_templates`+where+
			` ORDER BY updated_ts DESC, template_id LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("worker: scan roster: %w", err)
	}
	var templates []Template
	for rows.Next() {
		t, serr := scanTemplate(rows)
		if serr != nil {
			rows.Close()
			return nil, serr
		}
		templates = append(templates, t)
	}
	// Close() reports the DRIVER close; the ITERATION error lives only in Err().
	// Without this a scan that failed mid-way would serve a short roster as a
	// complete one — the exact lie this read exists to avoid.
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("worker: scan roster: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	out := make([]RosterEntry, 0, len(templates))
	for _, t := range templates {
		e, err := s.rosterEntry(ctx, t)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// rosterVisible is THE scope predicate of this read, as one function so the
// fragment cannot be re-derived somewhere it would drift: own rows plus the
// household roster for a member, every row for the operator.
func rosterVisible(q RosterQuery) (string, []any) {
	if q.Operator {
		return "", nil
	}
	return ` WHERE (user_id = ? OR scope = ?)`, []any{q.Viewer, string(ScopeHousehold)}
}

// rosterEntry assembles one worker. Every per-worker read that can legitimately
// have nothing to say records WHY instead of failing the whole roster: a draft
// has no guardrails, an unvalidated version has no battery record, and a
// version nobody revalidated has no stamp. An infrastructure failure still
// propagates.
func (s *Store) rosterEntry(ctx context.Context, t Template) (RosterEntry, error) {
	e := RosterEntry{Template: t, Versions: []RosterVersion{}}

	// The domains row and the delivery policy are NOT nil-able here, and that is
	// a fact about the schema rather than optimism: `worker_templates.domain` is
	// a foreign key into `domains`, `domains` carries a no-delete trigger
	// (migration 0005), and Delivery's other read — the active version's
	// guardrails — exists for any version Approve or Repoint could have pointed
	// at, both of which require an approved version. So neither can be missing
	// for a template that exists, and an error from either is a real failure this
	// read reports rather than a state it papers over with a half-answer. In
	// particular there is no "policy unread" value to serve: the zero
	// DeliveryPolicy says `requires_review: false`, and inventing that is the one
	// direction this must never fail in.
	d, err := s.Domain(ctx, t.Domain)
	if err != nil {
		return RosterEntry{}, err
	}
	e.Domain = d
	if e.Delivery, err = s.Delivery(ctx, t.ID); err != nil {
		return RosterEntry{}, err
	}

	switch def, derr := s.rosterDefinition(ctx, t); {
	case derr == nil:
		e.Definition = def
	case t.ActiveVersion == "":
		// The ordinary state of a draft, said in its own words rather than
		// through the store's wrapped ErrNotActive: a worker nobody approved has
		// no active version, so there is no definition to read and no equipment
		// to show. That is not a failure.
		// S08.6: equipment is read from the APPROVED version, and a draft has none.
		e.DefinitionError = fmt.Sprintf("nobody has approved a version of this worker yet (it is %s), so it has no tools or knowledge to show", t.Status)
	default:
		e.DefinitionError = derr.Error()
	}

	versions, truncated, err := s.rosterVersions(ctx, t)
	if err != nil {
		return RosterEntry{}, err
	}
	e.Versions, e.VersionsTruncated = versions, truncated
	return e, nil
}

// rosterDefinition parses the active version's hash-verified file. The error is
// returned for the caller to STATE rather than swallow: "no active version" and
// "the file no longer hashes to its row" are both facts about this worker.
func (s *Store) rosterDefinition(ctx context.Context, t Template) (*RosterDefinition, error) {
	_, def, err := s.activeDefinition(ctx, t)
	if err != nil {
		return nil, err
	}
	rd := &RosterDefinition{
		Description: def.Description,
		Selectors:   def.Selectors,
		Profile:     def.Profile,
		Equipment:   def.Equipment,
		Eval:        def.Eval,
		Persona:     def.Persona,
	}
	if t.Kind != KindAutomation {
		return rd, nil
	}
	wf, perr := automation.Parse(def.Body)
	if perr != nil {
		rd.WorkflowError = perr.Error()
		return rd, nil
	}
	steps := make([]RosterStep, 0, len(wf.Steps))
	for _, st := range wf.Steps {
		steps = append(steps, RosterStep{
			ID: st.ID, Verb: st.Verb, Approval: st.Approval, Needs: StepReferences(st),
		})
	}
	rd.Workflow = &RosterWorkflow{Dialect: wf.Dialect, Service: wf.Service, Steps: steps}
	return rd, nil
}

// StepReferences extracts one step's `$from` edges, dropping every literal.
//
// The predicate is the DIALECT'S OWN — automation.Reference, the same function
// resolveArgs calls — so the edges this surface draws are exactly the edges the
// executor follows. It is a call rather than a copy because the copy had
// already drifted in both directions: a stricter local decoder dropped a legal
// edge whose argument carried a sibling key (Step.Args is raw JSON, so the
// document loader's strictness never reaches inside an argument value, and such
// a document parses AND resolves), and it served `{"$from":""}` — a literal the
// executor passes through untouched — as an edge to nowhere. Either error is a
// false statement about how the stages connect, which is the one thing S15.10
// asks this surface for.
func StepReferences(st automation.Step) map[string]string {
	var out map[string]string
	for name, raw := range st.Args {
		from, ok := automation.Reference(raw)
		if !ok {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[name] = from
	}
	return out
}

// rosterVersions reads the immutable version history newest first, each with
// its enforcement state and dated records.
func (s *Store) rosterVersions(ctx context.Context, t Template) ([]RosterVersion, bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+versionColumns+` FROM worker_template_versions
		  WHERE template_id = ? ORDER BY version DESC LIMIT ?`, t.ID, rosterVersionCap+1)
	if err != nil {
		return nil, false, fmt.Errorf("worker: scan versions of %q: %w", t.ID, err)
	}
	var versions []Version
	for rows.Next() {
		v, serr := scanVersion(rows)
		if serr != nil {
			rows.Close()
			return nil, false, serr
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, false, fmt.Errorf("worker: scan versions of %q: %w", t.ID, err)
	}
	if err := rows.Close(); err != nil {
		return nil, false, err
	}
	truncated := len(versions) > rosterVersionCap
	if truncated {
		versions = versions[:rosterVersionCap]
	}

	out := make([]RosterVersion, 0, len(versions))
	for _, v := range versions {
		rv := RosterVersion{Version: v, Active: v.ID == t.ActiveVersion}
		switch g, gerr := s.Guardrails(ctx, v.ID); {
		case gerr == nil:
			rv.Granted = &g
		case errors.Is(gerr, ErrNotFound): // never approved — nothing granted
		default:
			return nil, false, gerr
		}
		switch rec, verr := s.LatestValidation(ctx, v.ID); {
		case verr == nil:
			rv.Validation = &rec
		case errors.Is(verr, ErrNotValidated): // the battery has not run
		default:
			return nil, false, verr
		}
		switch st, rerr := s.LatestRevalidation(ctx, v.ID); {
		case rerr == nil:
			rv.Revalidation = &st
		case errors.Is(rerr, ErrNotFound): // never revalidated
		default:
			return nil, false, rerr
		}
		out = append(out, rv)
	}
	return out, truncated, nil
}
