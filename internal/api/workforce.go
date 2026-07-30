package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// workforce.go — GET /api/workforce, the one read behind the Spec S15.10
// workforce map (B6-8 part B; OQ3/OQ4 as dispositioned).
//
// Two halves, and the split is deliberate:
//
//   - THE ROSTER comes from internal/worker's own narrow Roster read. The
//     equipment a worker requests lives in a hash-verified FILE and the
//     delivery policy is already one expression of S08.7's enforcement, so
//     assembling either here would have made this transport a second reader of
//     the registry's own identity (worker/roster.go says the rest).
//   - THE PER-VERSION OUTCOMES are DERIVED here, because S08.4 says so:
//     "version→outcome is a JOIN, not a system". routing.decided names the
//     worker and version per run, verdict.recorded carries that run's verdicts,
//     and the money is READ through the MeterReader seam. The join lives at the
//     transport for the reason the §42-B decisions block does: it spans the
//     worker registry, the verify family and metering, and none of those is the
//     others' home. It is NOT a Layer-1 catalog query — that band is at its
//     ceiling and moving it is a gate matter, not this read's.
//
// The map is VIEW-ONLY (S15.10: editing is parked to 15.5), so there is no verb
// in this file and no act to audit.

// The four bounds this read carries. All are STRUCTURAL constants with the
// readPageDefault reasoning (Spec S18 ratifies no such key; interim under the
// standing settings-tab directive), and each bounds a DIFFERENT cost:
//
//   - workforceRosterCap — a roster is read by a person and a page is the unit a
//     person reads. It changes how much arrives at once, never which rows are
//     eligible.
//   - workforceRoutingCap — the SQL scan behind the version→outcome join. The
//     control plane shares ONE writer connection (S02.1), so a read that
//     materializes an unbounded result set is a liveness risk to everything else
//     on the handle.
//   - workforceRunsPerVersion — how many routed runs one VERSION serves, newest
//     first. Without it a single hot version consumes the whole scan budget and
//     every other version reads as "never routed to", which is the shape of lie
//     this surface exists to avoid. The exhaustive trace is the event log, which
//     /api/events answers.
//   - workforceVerdictsPerRun — how many verdict rounds one RUN serves, newest
//     first. It is per RUN and not per request on purpose: a global bound over a
//     multi-run scan lets one talkative run consume it and leaves every other run
//     rendering "no verdict recorded", which is a false statement about verified
//     work rather than a smaller one. Partitioned, no run can starve another.
//   - workforceMeterCap — how many runs get a METER READING in one request, and
//     this one is not about response size. `Ledger.RunConsumption` opens a WRITE
//     transaction per run on that same single connection, so the fan-out here is
//     the expensive part of this whole read; beyond the bound a row says its
//     figure was not read rather than carrying a number nobody took.
const (
	workforceRosterCap      = 200
	workforceRoutingCap     = 1000
	workforceRunsPerVersion = 20
	workforceVerdictsPerRun = 20
	workforceMeterCap       = 100
)

// The two event types this read joins over. Literals rather than imports,
// because internal/api imports no producer package for its derives — the
// derive-from-log discipline the inbox and decision projections already follow.
const (
	typeRoutingDecided = "routing.decided"
	typeVerdictRecord  = "verdict.recorded"
)

// WorkforceView is the whole map's data.
type WorkforceView struct {
	Workers []WorkforceWorker `json:"workers"`
	// RosterScope states WHOSE workers are in the list, and OutcomeScope whose
	// RUNS the outcome figures below cover. They are separate because they
	// genuinely differ: a member sees the household roster, and the runs routed
	// to a shared worker still belong to whoever launched them (D2). Saying so
	// on the wire is what stops one reading posing as the other.
	RosterScope  string `json:"roster_scope"`
	OutcomeScope string `json:"outcome_scope"`
	// Truncated reports the roster cap cutting the list short.
	Truncated bool `json:"truncated"`
	// OutcomesTruncated reports the routing-scan cap being reached, so a sparse
	// outcome list is never read as "this version was never routed to".
	OutcomesTruncated bool  `json:"outcomes_truncated"`
	Cursor            int64 `json:"cursor"`
}

// WorkforceWorker is one worker_templates row with everything the map renders.
type WorkforceWorker struct {
	TemplateID    string          `json:"template_id"`
	Name          string          `json:"name"`
	Owner         string          `json:"owner"`
	Scope         string          `json:"scope"`
	Kind          string          `json:"kind"`
	Status        string          `json:"status"`
	ActiveVersion string          `json:"active_version"`
	CreatedTS     string          `json:"created_ts"`
	UpdatedTS     string          `json:"updated_ts"`
	Domain        WorkforceDomain `json:"domain"`
	// Delivery is the S08.7 policy VERBATIM from the registry's own read: what
	// this worker may deliver without a person looking, and why not.
	//
	// There is deliberately no "unread" state beside it. The zero policy says
	// `requires_review: false`, which would tell a reader that a worker needs no
	// human review — the one direction this field must never fail in — so the
	// roster read FAILS rather than serving one, and it can afford to for the
	// schema reason worker/roster.go states: the domains row and the guardrails
	// row behind this policy cannot be missing for a template that exists.
	Delivery worker.DeliveryPolicy `json:"delivery"`
	// Definition is the ACTIVE version's behavioral facts; absent with its
	// reason for a draft, or when the S08.3 hash check refused the file.
	Definition        *WorkforceDefinition `json:"definition"`
	DefinitionAbsent  string               `json:"definition_absent,omitempty"`
	Versions          []WorkforceVersion   `json:"versions"`
	VersionsTruncated bool                 `json:"versions_truncated"`
}

// WorkforceDomain is the domains row behind a worker's maturity marking.
type WorkforceDomain struct {
	Name string `json:"name"`
	// Maturity is `full` or `degraded` today. It is served as the raw stored
	// value so a maturity this build has not heard of still reaches the screen.
	Maturity  string `json:"maturity"`
	RubricRef string `json:"rubric_ref,omitempty"`
}

// WorkforceDefinition is the active version's parsed file.
type WorkforceDefinition struct {
	Description string                  `json:"description,omitempty"`
	Selectors   worker.SelectorSet      `json:"selectors"`
	Profile     worker.ExecutionProfile `json:"profile"`
	// RequestedEquipment is what the FILE asks for (Spec S08.1). What was
	// GRANTED is each version's `granted` block below; the S08.2 split is what
	// this surface exists to make visible, so the two are never one field.
	RequestedEquipment worker.Equipment `json:"requested_equipment"`
	Eval               worker.EvalHooks `json:"eval"`
	Persona            []string         `json:"persona,omitempty"`
	// Workflow is the S08.9 multi-stage chain (kind=automation only).
	Workflow       *worker.RosterWorkflow `json:"workflow,omitempty"`
	WorkflowAbsent string                 `json:"workflow_absent,omitempty"`
}

// WorkforceVersion is one immutable version row: its provenance block, the
// powers it REQUESTED, the enforcement state that was GRANTED, its dated
// records, and its outcomes.
type WorkforceVersion struct {
	VersionID  string `json:"version_id"`
	Version    int64  `json:"version"`
	Supersedes string `json:"supersedes,omitempty"`
	Active     bool   `json:"active"`
	FilePath   string `json:"file_path"`
	FileSHA256 string `json:"file_sha256"`
	FileCommit string `json:"file_commit,omitempty"`
	// The S08.1 provenance block: who or what authored this version, on what
	// evidence, approved by whom and when, and whether it graduated.
	AuthorKind      string `json:"author_kind"`
	Composer        string `json:"composer,omitempty"`
	PlaybookVersion string `json:"playbook_version,omitempty"`
	EvidenceRef     string `json:"evidence_ref,omitempty"`
	Origin          string `json:"origin"`
	OriginRef       string `json:"origin_ref,omitempty"`
	CreatedBy       string `json:"created_by"`
	CreatedTS       string `json:"created_ts"`
	ApprovedBy      string `json:"approved_by,omitempty"`
	ApprovedTS      string `json:"approved_ts,omitempty"`
	GraduatedTS     string `json:"graduated_ts,omitempty"`
	// RequestedGrants is the version row's own request for enforcement powers,
	// served verbatim from the registry's type.
	RequestedGrants worker.RequestedGrants `json:"requested_grants"`
	Granted         *WorkforceGrants       `json:"granted"`
	// GrantedAbsent is why nothing was granted: approval is the ONLY writer of
	// enforcement state (S08.2), so an unapproved version has no row — not an
	// empty one.
	GrantedAbsent      string                 `json:"granted_absent,omitempty"`
	Validation         *WorkforceValidation   `json:"validation"`
	ValidationAbsent   string                 `json:"validation_absent,omitempty"`
	Revalidation       *WorkforceRevalidation `json:"revalidation"`
	RevalidationAbsent string                 `json:"revalidation_absent,omitempty"`
	Outcomes           WorkforceOutcomes      `json:"outcomes"`
}

// WorkforceGrants is the worker_guardrails row — the S08.2 enforcement state,
// exclusively: what this version may actually do.
type WorkforceGrants struct {
	GrantedTools     []string `json:"granted_tools"`
	GatedTools       []string `json:"gated_tools,omitempty"`
	PermissionMode   string   `json:"permission_mode,omitempty"`
	ConfinementClass string   `json:"confinement_class"`
	Egress           string   `json:"egress"`
	EgressHosts      []string `json:"egress_hosts,omitempty"`
	// The ceilings are NIL-ABLE, and 0 is what makes them so. `worker_guardrails`
	// stores both NOT NULL with a >= 0 check and `Approve` copies the version's
	// REQUEST verbatim, so a version that asked for no ceiling is granted 0 —
	// and the registry's own request type says as much, carrying `omitempty` on
	// both fields (worker.go RequestedGrants). Serving that 0 put `USD 0` on
	// screen for a ceiling nobody set, which is the same fabricated figure the
	// meters surface already refuses to print (api.go LaneMeter: "no operator
	// budget is persisted anywhere … the ABSENCE is what the meters surface
	// serves — never a zero", S10.1).
	BudgetUSD          *float64 `json:"budget_usd"`
	BudgetSteps        *int64   `json:"budget_steps"`
	GatePolicy         string   `json:"gate_policy,omitempty"`
	FirstNRemaining    int64    `json:"first_n_remaining"`
	ScheduleAttachable bool     `json:"schedule_attachable"`
	UpdatedTS          string   `json:"updated_ts"`
}

// WorkforceValidation is the newest validation_records row, keyed (version ×
// model × engine pin) — for kind=automation the pin slot holds the dialect
// version (Spec S08.1, S08.9).
//
// TWO COLUMNS ARE DELIBERATELY NOT HERE, and the second was found by the
// fixture stability check rather than by reasoning, so both halves are said:
//
//   - the battery's lint/audit FINDINGS. Validation asserts structure, powers
//     and a witnessed run (S08.6: it is explicitly not a quality guarantee), and
//     the findings behind a red belong to the S08.6 station-4 approval card,
//     which is where a person acts on them.
//   - `dryrun_ref`. It is the transcript POINTER the same approval card owns —
//     and for kind=automation the column does not hold a pointer at all: the
//     dry run has no transcript, so what is stored is the whole automation
//     Report inline, step OUTPUTS and the journaled proposal's id included. A
//     view-only map has no business surfacing a run's outputs, and the inlined
//     id is why /api/workforce was not byte-stable until this field went.
type WorkforceValidation struct {
	Model      string `json:"model,omitempty"`
	EnginePin  string `json:"engine_pin"`
	Green      bool   `json:"green"`
	Approver   string `json:"approver,omitempty"`
	ApprovedTS string `json:"approved_ts,omitempty"`
	CreatedTS  string `json:"created_ts"`
}

// WorkforceRevalidation is the newest dated revalidation stamp (Spec S14.8 ¶3):
// who, when, which suites, against which pinned baseline — and a RED stamp is
// on the record too, with the version still flagged.
type WorkforceRevalidation struct {
	TriggerKind    string `json:"trigger_kind"`
	TriggerSubject string `json:"trigger_subject,omitempty"`
	Suites         string `json:"suites"`
	Baseline       string `json:"baseline"`
	Green          bool   `json:"green"`
	Actor          string `json:"actor"`
	StampedTS      string `json:"stamped_ts"`
}

// WorkforceOutcomes is the S08.4 version→outcome join for one version.
type WorkforceOutcomes struct {
	// Runs are the runs routed to this version, newest routing decision first.
	Runs []WorkforceRoutedRun `json:"runs"`
	// Truncated reports the per-version bound cutting the list short, so a
	// reading of the newest runs is never read as the whole history.
	Truncated bool `json:"truncated"`
	// RunsRouted is how many runs this reading found routed to this version —
	// the count BEFORE the per-version bound, so it stays true when the list
	// above is cut. A count of rows is a reading somebody can take; it is not
	// money, and nothing here sums a figure.
	RunsRouted int `json:"runs_routed"`
	// VerdictTally is the S08.4 "verdict outcomes" half: how many recorded
	// rounds carried each verdict, across the runs above. Rows counted, sorted
	// by verdict so two identical requests answer identically.
	VerdictTally []WorkforceVerdictCount `json:"verdict_tally"`
	// Absent says why there is nothing, so an empty list is never read as a
	// measurement somebody took (S10.1; §42 honest absence). At v0 this is what
	// most versions will carry, and it is the truthful render.
	Absent string `json:"absent,omitempty"`
}

// WorkforceRoutedRun is one routed run's outcome: the routing decision as
// recorded, that run's verdicts, and its meter reading.
type WorkforceRoutedRun struct {
	RunID       string             `json:"run_id"`
	Owner       string             `json:"owner"`
	Cause       string             `json:"cause"`
	Model       string             `json:"model,omitempty"`
	Lane        string             `json:"lane,omitempty"`
	Effort      string             `json:"effort,omitempty"`
	PlainReason string             `json:"plain_reason,omitempty"`
	Degraded    bool               `json:"degraded,omitempty"`
	Generalist  bool               `json:"generalist,omitempty"`
	RoutedTS    string             `json:"routed_ts"`
	Verdicts    []WorkforceVerdict `json:"verdicts"`
	// VerdictsAbsent says why a routed run carries no verdict: nothing has
	// verified it yet. It can say only that because the verdict scan is bounded
	// PER RUN — no run's rounds are lost to another run's, so an empty list here
	// is a fact about this run and never an artifact of the reading.
	VerdictsAbsent string `json:"verdicts_absent,omitempty"`
	// VerdictsTruncated reports this run's own rounds being cut by that bound, so
	// the newest rounds are never read as all of them.
	VerdictsTruncated bool `json:"verdicts_truncated,omitempty"`
	// Tokens and APIEquivCostUSD are READ from the metering seam, as served.
	// Nil is the honest absence of a reading — never a zero, which would be a
	// measurement nobody took (§37). Nothing here is summed across runs or
	// across versions: a total would be arithmetic this transport must not do.
	//
	// The nil arm has THREE causes and `MeterAbsent` says which: the seam is not
	// wired, the seam refused, or the seam answered with nothing recorded yet —
	// see meterReading, which exists because that third case is what the
	// production ledger returns and it is NOT an error.
	Tokens          *int64   `json:"tokens"`
	APIEquivCostUSD *float64 `json:"api_equiv_cost_usd"`
	// Unpriced carries the seam's own subscription-lane marking.
	Unpriced bool `json:"unpriced,omitempty"`
	// MeterAbsent is why there is no reading for this run.
	MeterAbsent string `json:"meter_absent,omitempty"`
}

// WorkforceVerdictCount is one verdict value and how many rounds carried it.
type WorkforceVerdictCount struct {
	Verdict string `json:"verdict"`
	Rounds  int    `json:"rounds"`
}

// WorkforceVerdict is one verdict.recorded row of a routed run.
type WorkforceVerdict struct {
	Round    int    `json:"round"`
	Verdict  string `json:"verdict"`
	Revision int    `json:"revision,omitempty"`
	Domain   string `json:"domain,omitempty"`
	TS       string `json:"ts"`
}

// handleWorkforceRead serves GET /api/workforce.
func (s *Server) handleWorkforceRead(w http.ResponseWriter, r *http.Request) {
	if s.workforce == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the workforce map reads the S08 worker registry, which is not wired in this process"})
		return
	}
	if !s.projReady(w) {
		return
	}
	scope := s.readScope(r)
	entries, err := s.workforce.Roster(r.Context(), worker.RosterQuery{
		Viewer: scope.UserID, Operator: scope.Operator, Limit: workforceRosterCap + 1,
	})
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("worker roster: %w", err))
		return
	}
	view := WorkforceView{
		Workers:      []WorkforceWorker{},
		RosterScope:  rosterScopeStatement(scope),
		OutcomeScope: outcomeScopeStatement(scope),
	}
	if len(entries) > workforceRosterCap {
		entries, view.Truncated = entries[:workforceRosterCap], true
	}

	outcomes, err := s.proj.versionOutcomes(r.Context(), scope)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	view.OutcomesTruncated = outcomes.truncated
	for _, e := range entries {
		view.Workers = append(view.Workers, workforceWorker(e, outcomes))
	}
	if view.Cursor, err = s.head(r.Context()); err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	// The body carries run_events payload content (a routing cause, a plain
	// reason, a verdict), so it goes out through the codor-C2 primitive — the
	// store-raw / serve-redacted rule every payload-bearing read follows (§30).
	s.writeReadRedacted(w, view)
}

func rosterScopeStatement(scope ownerScope) string {
	if scope.Operator {
		return "every worker in the registry (the operator reads the whole roster — D10)"
	}
	return "your own personal workers plus the household roster (S15.10)"
}

func outcomeScopeStatement(scope ownerScope) string {
	if scope.Operator {
		return "runs of every owner"
	}
	return "your own runs only — a shared worker's other runs belong to whoever launched them (D2)"
}

// workforceWorker projects one roster entry onto the wire.
func workforceWorker(e worker.RosterEntry, outcomes versionOutcomeSet) WorkforceWorker {
	t := e.Template
	out := WorkforceWorker{
		TemplateID: t.ID, Name: t.Name, Owner: t.Owner,
		Scope: string(t.Scope), Kind: string(t.Kind), Status: string(t.Status),
		ActiveVersion: t.ActiveVersion, CreatedTS: t.CreatedTS, UpdatedTS: t.UpdatedTS,
		Domain: WorkforceDomain{
			Name: e.Domain.Name, Maturity: string(e.Domain.Maturity), RubricRef: e.Domain.RubricRef,
		},
		Delivery:          e.Delivery,
		DefinitionAbsent:  e.DefinitionError,
		Versions:          []WorkforceVersion{},
		VersionsTruncated: e.VersionsTruncated,
	}
	if d := e.Definition; d != nil {
		out.Definition = &WorkforceDefinition{
			Description: d.Description, Selectors: d.Selectors, Profile: d.Profile,
			RequestedEquipment: d.Equipment, Eval: d.Eval, Persona: d.Persona,
			Workflow: d.Workflow, WorkflowAbsent: d.WorkflowError,
		}
	}
	for _, rv := range e.Versions {
		out.Versions = append(out.Versions, workforceVersion(rv, outcomes))
	}
	return out
}

func workforceVersion(rv worker.RosterVersion, outcomes versionOutcomeSet) WorkforceVersion {
	v := rv.Version
	out := WorkforceVersion{
		VersionID: v.ID, Version: v.Version, Supersedes: v.Supersedes, Active: rv.Active,
		FilePath: v.FilePath, FileSHA256: v.FileSHA256, FileCommit: v.FileCommit,
		AuthorKind: v.AuthorKind, Composer: v.Composer, PlaybookVersion: v.PlaybookVer,
		EvidenceRef: v.EvidenceRef, Origin: string(v.Origin), OriginRef: v.OriginRef,
		CreatedBy: v.CreatedBy, CreatedTS: v.CreatedTS,
		ApprovedBy: v.ApprovedBy, ApprovedTS: v.ApprovedTS, GraduatedTS: v.GraduatedTS,
		RequestedGrants: v.Requested,
		Outcomes:        WorkforceOutcomes{Runs: []WorkforceRoutedRun{}},
	}
	if g := rv.Granted; g != nil {
		out.Granted = &WorkforceGrants{
			GrantedTools: g.GrantedTools, GatedTools: g.GatedTools, PermissionMode: g.PermissionMode,
			ConfinementClass: g.Class, Egress: string(g.Egress), EgressHosts: g.EgressHosts,
			GatePolicy:      g.GatePolicy,
			FirstNRemaining: g.FirstNRemaining, ScheduleAttachable: g.ScheduleAttachable,
			UpdatedTS: g.UpdatedTS,
		}
		if g.BudgetUSD != 0 {
			usd := g.BudgetUSD
			out.Granted.BudgetUSD = &usd
		}
		if g.BudgetSteps != 0 {
			steps := g.BudgetSteps
			out.Granted.BudgetSteps = &steps
		}
	} else {
		out.GrantedAbsent = "never approved — approval is the only writer of enforcement state (S08.2)"
	}
	if rec := rv.Validation; rec != nil {
		out.Validation = &WorkforceValidation{
			Model: rec.Model, EnginePin: rec.EnginePin, Green: rec.Green,
			Approver: rec.Approver, ApprovedTS: rec.ApprovedTS, CreatedTS: rec.CreatedTS,
		}
	} else {
		out.ValidationAbsent = "the S08.6 battery has not run for this version"
	}
	if st := rv.Revalidation; st != nil {
		out.Revalidation = &WorkforceRevalidation{
			TriggerKind: st.TriggerKind, TriggerSubject: st.TriggerSubject, Suites: st.Suites,
			Baseline: st.Baseline, Green: st.Green, Actor: st.Actor, StampedTS: st.StampedTS,
		}
	} else {
		out.RevalidationAbsent = "never revalidated — no S08.10 trigger has fired for this version"
	}
	out.Outcomes.RunsRouted = outcomes.counts[v.ID]
	out.Outcomes.VerdictTally = outcomes.tally[v.ID]
	if out.Outcomes.VerdictTally == nil {
		out.Outcomes.VerdictTally = []WorkforceVerdictCount{}
	}
	if runs := outcomes.runs[v.ID]; len(runs) > 0 {
		out.Outcomes.Runs = runs
		out.Outcomes.Truncated = outcomes.cutAt[v.ID]
		return out
	}
	// The two readings are different facts and get different sentences. The
	// operator's covers every owner, so its empty answer is about the version
	// itself; a member's covers their own runs, so its empty answer is about
	// what THEY ran — which is also the most it can honestly say, because
	// "there is a run here you cannot see" would disclose the existence of
	// another owner's run to somebody the scope exists to keep it from.
	switch {
	case outcomes.truncated:
		out.Outcomes.Absent = "no routed run within the bounded routing scan — the exhaustive trace is /api/events"
	case outcomes.scoped:
		out.Outcomes.Absent = "no run of yours has been routed to this version"
	default:
		out.Outcomes.Absent = "no run has been routed to this version"
	}
	return out
}

// versionOutcomeSet is one read's join result: the routed runs per version, the
// versions whose list the per-version bound cut, and whether the SQL scan itself
// hit its bound. The two truncations are separate facts and the wire keeps them
// separate — one says "this version has more", the other says "this reading saw
// less of everything".
type versionOutcomeSet struct {
	runs   map[string][]WorkforceRoutedRun
	cutAt  map[string]bool
	counts map[string]int
	tally  map[string][]WorkforceVerdictCount
	// scoped says the caller's reading covers only their OWN runs, which is what
	// makes the two empty-outcome answers different sentences (D6).
	scoped    bool
	truncated bool
}

// versionOutcomes is the S08.4 join, keyed by version id.
//
// THE ROUTING AND VERDICT ROWS ARE TWO QUERIES FOR THE WHOLE READ, not two per
// version: they are scanned once and grouped in memory. That is a property of
// THIS JOIN and of nothing else — the roster path above it is frankly an N+1
// (per template: a domain read, a delivery policy that re-reads the template,
// and a versions query; per version: guardrails, latest validation, latest
// revalidation), which at the declared caps is a large number of small indexed
// SELECTs. It is left that way deliberately: the roster is household-scale and
// empty at v0, and the alternative is hand-written joins duplicating reads the
// registry already owns. The real bound is the caps, not the query count.
//
// The METER is the one part that cannot be batched — the seam is per run, and
// each call opens a WRITE transaction on the single writer connection — so it is
// bounded by workforceMeterCap, memoized per run id, and spent newest-first.
func (p *projector) versionOutcomes(ctx context.Context, scope ownerScope) (versionOutcomeSet, error) {
	q := `SELECT event_seq, run_id, user_id, ts, payload FROM run_events
	       WHERE type = ?
	         AND json_extract(payload, '$.version') IS NOT NULL
	         AND json_extract(payload, '$.version') <> ''`
	args := []any{typeRoutingDecided}
	if !scope.Operator {
		// The RUN's owner decides, not the worker's: a member reading the
		// household roster still may not read another person's runs or their
		// money (D2; the readPerson rule). The worker's own visibility was
		// already settled by the roster read.
		q += ` AND user_id = ?`
		args = append(args, scope.UserID)
	}
	q += ` ORDER BY event_seq DESC LIMIT ?`
	args = append(args, workforceRoutingCap+1)
	scoped := !scope.Operator

	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return versionOutcomeSet{}, fmt.Errorf("projection: workforce routing scan: %w", err)
	}
	// routed pairs one routing row with the version it named, so the grouping
	// below needs no second lookup.
	type routed struct {
		ver string
		out WorkforceRoutedRun
	}
	var scanned []routed
	raw := 0
	for rows.Next() {
		var (
			seq     int64
			runID   *string
			owner   string
			ts      string
			payload string
		)
		if err := rows.Scan(&seq, &runID, &owner, &ts, &payload); err != nil {
			rows.Close()
			return versionOutcomeSet{}, fmt.Errorf("projection: workforce routing scan: %w", err)
		}
		raw++
		pay := json.RawMessage(payload)
		version := firstString(pay, "version")
		if version == "" {
			continue
		}
		r := routed{ver: version, out: WorkforceRoutedRun{
			Owner: owner, RoutedTS: ts,
			Cause: firstString(pay, "cause"), Model: firstString(pay, "model"),
			Lane: firstString(pay, "lane"), Effort: firstString(pay, "effort"),
			PlainReason: firstString(pay, "plain_reason"),
			Degraded:    firstBool(pay, "degraded"), Generalist: firstBool(pay, "generalist"),
			Verdicts: []WorkforceVerdict{},
		}}
		if runID != nil {
			r.out.RunID = *runID
		}
		scanned = append(scanned, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return versionOutcomeSet{}, fmt.Errorf("projection: workforce routing scan: %w", err)
	}
	if err := rows.Close(); err != nil {
		return versionOutcomeSet{}, err
	}
	// The cap is measured over the rows READ, not the rows kept: a row skipped
	// for naming no version still consumed the bound, and reporting otherwise
	// would understate how much of the history this reading saw.
	truncated := raw > workforceRoutingCap
	if len(scanned) > workforceRoutingCap {
		scanned = scanned[:workforceRoutingCap]
	}

	// Bound per version, KEEPING THE SCAN'S OWN ORDER. `scanned` is globally
	// newest-first, so walking it once and dropping the rows past a version's
	// bound leaves every surviving row in newest-first order — which is what the
	// meter budget below is then spent in. Grouping into a map first and
	// iterating THAT would have spent the budget in Go's randomized map order,
	// so two identical requests could disagree about which rows carry a figure.
	kept := make([]routed, 0, len(scanned))
	perVersion := map[string]int{}
	counts := map[string]int{}
	cut := map[string]bool{}
	for _, r := range scanned {
		counts[r.ver]++ // the true count in this reading, before the bound
		if perVersion[r.ver] >= workforceRunsPerVersion {
			cut[r.ver] = true
			continue
		}
		perVersion[r.ver]++
		kept = append(kept, r)
	}

	runIDs := make([]string, 0, len(kept))
	seen := map[string]bool{}
	for _, r := range kept {
		if r.out.RunID != "" && !seen[r.out.RunID] {
			seen[r.out.RunID] = true
			runIDs = append(runIDs, r.out.RunID)
		}
	}
	verdicts, cutRuns, err := p.verdictsByRun(ctx, runIDs)
	if err != nil {
		return versionOutcomeSet{}, err
	}

	metered := map[string]WorkforceRoutedRun{}
	tally := map[string]map[string]int{}
	out := map[string][]WorkforceRoutedRun{}
	for _, r := range kept {
		row := r.out
		if row.RunID == "" {
			// A routing decision attributed to no run cannot be joined to a
			// verdict or a meter reading; it is still a routing fact about this
			// version, so it renders with both absences stated.
			row.VerdictsAbsent = "this routing decision names no run, so nothing verifies it"
			row.MeterAbsent = "this routing decision names no run, so no meter reading exists"
			out[r.ver] = append(out[r.ver], row)
			continue
		}
		if vs := verdicts[row.RunID]; len(vs) > 0 {
			row.Verdicts, row.VerdictsTruncated = vs, cutRuns[row.RunID]
			if tally[r.ver] == nil {
				tally[r.ver] = map[string]int{}
			}
			for _, v := range vs {
				tally[r.ver][v.Verdict]++
			}
		} else {
			row.VerdictsAbsent = "no verdict recorded for this run yet"
		}
		// One reading per run, however many versions it was routed to — and the
		// budget is spent newest-first, because `kept` is in scan order.
		if prior, ok := metered[row.RunID]; ok {
			row.Tokens, row.APIEquivCostUSD, row.Unpriced, row.MeterAbsent =
				prior.Tokens, prior.APIEquivCostUSD, prior.Unpriced, prior.MeterAbsent
		} else {
			p.readMeter(ctx, &row, len(metered) >= workforceMeterCap)
			metered[row.RunID] = row
		}
		out[r.ver] = append(out[r.ver], row)
	}
	return versionOutcomeSet{
		runs: out, cutAt: cut, counts: counts, tally: tallyRows(tally),
		scoped: scoped, truncated: truncated,
	}, nil
}

// tallyRows turns the per-version verdict counts into sorted rows, so two
// identical requests serve identical bytes.
func tallyRows(tally map[string]map[string]int) map[string][]WorkforceVerdictCount {
	out := map[string][]WorkforceVerdictCount{}
	for ver, counts := range tally {
		rows := make([]WorkforceVerdictCount, 0, len(counts))
		for verdict, n := range counts {
			rows = append(rows, WorkforceVerdictCount{Verdict: verdict, Rounds: n})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Verdict < rows[j].Verdict })
		out[ver] = rows
	}
	return out
}

// readMeter fills one row's figures from the metering seam, or states why it
// could not — and the THIRD arm is the one that matters, because it is the one
// production takes most often and no fixture meter produces it.
//
// `Ledger.RunConsumption` folds a run's checkpoint rows. A run that exists but
// has recorded no usage yet — which is every run between its routing decision
// and its first checkpoint — folds to zero tokens, zero cost and zero unpriced
// calls with NO error. Serving that as `0` would print "USD 0" for work whose
// cost nobody has measured, which is exactly the fabricated figure §37 forbids
// and which the seam gives no other way to tell apart. So an entirely empty
// reading renders as the absence it is.
func (p *projector) readMeter(ctx context.Context, row *WorkforceRoutedRun, overCap bool) {
	if p.meter == nil {
		row.MeterAbsent = "the metering seam is not wired in this process"
		return
	}
	if overCap {
		row.MeterAbsent = "not read in this reading — this run's own receipt carries its figure"
		return
	}
	m, priced := p.meterReading(ctx, row.RunID)
	if !priced {
		// The seam refused and the seam answering with nothing recorded are
		// different facts, so they get different sentences. meterReading folds
		// both into one bool, so the refusal is re-asked here rather than
		// inferred — which keeps ONE expression of "is there a reading" while
		// still saying WHICH absence this is.
		if _, err := p.meter.RunMeter(ctx, row.RunID); err != nil {
			row.MeterAbsent = "no meter reading for this run"
			return
		}
		row.MeterAbsent = "no usage recorded for this run yet"
		return
	}
	tokens, cost := m.Tokens, m.APIEquivCostUSD
	row.Tokens, row.APIEquivCostUSD, row.Unpriced = &tokens, &cost, m.Unpriced
}

// verdictsByRun reads the verify verdicts of the named runs, newest rounds
// first, bounded PER RUN. The second return names the runs whose own rounds the
// bound cut.
//
// The partition is what makes the bound safe: a plain `LIMIT n` over a multi-run
// scan lets one talkative run consume it, and the runs it starves then render
// "no verdict recorded" — a claim about verified work that the reading invented.
// A window function keeps every run's newest rounds reachable regardless of what
// its siblings carry.
func (p *projector) verdictsByRun(ctx context.Context, runIDs []string) (map[string][]WorkforceVerdict, map[string]bool, error) {
	out := map[string][]WorkforceVerdict{}
	cut := map[string]bool{}
	if len(runIDs) == 0 {
		return out, cut, nil
	}
	args := []any{typeVerdictRecord}
	args = append(args, toAny(runIDs)...)
	// One past the bound, so a run AT the bound is distinguishable from a run cut
	// by it.
	args = append(args, workforceVerdictsPerRun+1)
	rows, err := p.db.QueryContext(ctx,
		`SELECT run_id, ts, payload FROM (
		   SELECT run_id, ts, payload, event_seq,
		          ROW_NUMBER() OVER (PARTITION BY run_id ORDER BY event_seq DESC) AS rn
		     FROM run_events
		    WHERE type = ? AND run_id IN (`+placeholders(len(runIDs))+`)
		 ) WHERE rn <= ? ORDER BY event_seq`, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("projection: workforce verdict scan: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var runID, ts, payload string
		if err := rows.Scan(&runID, &ts, &payload); err != nil {
			return nil, nil, fmt.Errorf("projection: workforce verdict scan: %w", err)
		}
		pay := json.RawMessage(payload)
		out[runID] = append(out[runID], WorkforceVerdict{
			Round:    firstInt(pay, "round"),
			Verdict:  firstString(pay, "verdict"),
			Revision: firstInt(pay, "revision"),
			Domain:   firstString(pay, "domain"),
			TS:       ts,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("projection: workforce verdict scan: %w", err)
	}
	for run := range out {
		if len(out[run]) > workforceVerdictsPerRun {
			// The extra row was read only to detect the cut; it is not served.
			out[run] = out[run][len(out[run])-workforceVerdictsPerRun:]
			cut[run] = true
		}
		sort.SliceStable(out[run], func(i, j int) bool { return out[run][i].Round < out[run][j].Round })
	}
	return out, cut, nil
}

// firstInt reads a payload integer, joining firstString/firstBool. A verdict's
// round and revision are numbers on the wire and reading them as strings would
// have rendered every round as blank.
func firstInt(payload json.RawMessage, keys ...string) int {
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return 0
	}
	for _, k := range keys {
		if f, ok := m[k].(float64); ok {
			return int(f)
		}
	}
	return 0
}
