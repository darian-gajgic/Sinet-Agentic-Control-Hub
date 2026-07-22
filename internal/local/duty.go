package local

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
)

// duty.go — the S12.1 class-(b) duty call: resolve the alias, refuse if the
// operator-wins eager-unload switch is engaged, shape + issue the /v1 call,
// and write ONE D7 ledger usage row on the consuming run (brief R16/R17/R18).
// This is the ONE place a local call is metered (the marker in usage.go, the
// $0 pricing in internal/metering). A call site with no run in a
// checkpointable state is a SURFACED DEFECT (loud log + event), never a
// silently unmetered call (R18).

// Provisional event names (pending the S14 event contract, B5 — extending the
// standing CONVENTIONS note). refs-not-blobs, owner-attributed.
const (
	// EventLocalUnmeteredDefect surfaces a local call that could not land its
	// mandatory D7 row (R18) — never silent.
	EventLocalUnmeteredDefect = "local.unmetered_defect"
	// EventLocalAdmissionsStopped / Resumed record the eager-unload
	// operator-wins switch (R12).
	EventLocalAdmissionsStopped = "local.admissions_stopped"
	EventLocalAdmissionsResumed = "local.admissions_resumed"
)

const dutyEventSchemaVersion = 1

// DutyRequest is one duty-alias call the caller composes (the schema + prompt
// carry the S12.4 shaping; the client applies temp-0/logprobs/json_schema).
type DutyRequest struct {
	Alias  string
	System string          // free-text framing preceding the constrained region (S12.4)
	User   string          // the duty prompt (also carries the schema in-prompt, S12.4)
	Schema json.RawMessage // engine-enforced json_schema (classification duties); nil for drafting
	Name   string          // json_schema name
	// MaxTokens is the per-duty length cap (a structural constant set by the
	// caller — S18 ratifies no key, §8 reading 7).
	MaxTokens int64
	// Classification marks a classification-shaped duty: greedy + logprobs +
	// the abstain-member assertion (S12.4). False = free-text drafting
	// (utility Help), no forced-label abstain.
	Classification bool
}

// DutyResult is a completed duty call (the stage adapter maps it to the
// intake/worker seam type).
type DutyResult struct {
	Alias        string
	Model        string
	Seat         SeatRecord
	Content      string
	Logprobs     []TokenLogprob
	InputTokens  int64
	OutputTokens int64
	CheckpointID int64
}

// Duty is the platform-plane duty caller + D7 metering. Nil-safe: a nil Duty
// (the stack unconfigured) makes every call ErrStackAbsent so the stage
// seams degrade exactly per each duty's S12.4/S06 row (R17).
type Duty struct {
	reg      *Registry
	client   *Client
	cps      *gates.Checkpoints
	log      *eventlog.Log
	adm      *Admissions
	admitter *VRAMAdmitter // OQ2 live GPU-admission pre-flight; nil ⇒ no gate
	logger   *slog.Logger
}

// DutyDeps are the wired dependencies of a live Duty.
type DutyDeps struct {
	Registry    *Registry
	Client      *Client
	Checkpoints *gates.Checkpoints
	Events      *eventlog.Log
	Admissions  *Admissions
	// Admitter is the S12.7 VRAM/battery admitter run as the LIVE platform-plane
	// pre-flight before a duty call triggers a llama-swap load (OQ2). Nil ⇒ no
	// gate (the dev default is unchanged; an unconfigured stack never gates).
	Admitter *VRAMAdmitter
	// AdmissionsFlag is the durable cross-process admission flag path (F4/F11);
	// used only when Admissions is nil. "" = the in-memory dev fallback.
	AdmissionsFlag string
	Logger         *slog.Logger
}

// NewDuty wires a live duty caller (called by the shell when the local stack
// is configured).
func NewDuty(d DutyDeps) *Duty {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	adm := d.Admissions
	if adm == nil {
		adm = NewAdmissions(d.AdmissionsFlag)
	}
	return &Duty{reg: d.Registry, client: d.Client, cps: d.Checkpoints, log: d.Events, adm: adm, admitter: d.Admitter, logger: logger}
}

// Admissions returns the caller's admission gate (the eager-unload switch),
// so the eager-unload verb shares it.
func (d *Duty) Admissions() *Admissions {
	if d == nil {
		return nil
	}
	return d.adm
}

// Client returns the underlying /v1 client (the eager-unload verb uses its
// unload leg).
func (d *Duty) Client() *Client {
	if d == nil {
		return nil
	}
	return d.client
}

// ResolveSeat resolves a duty alias to its serving seat over ⚙ local.alias +
// the manifest, WITHOUT the servable gate — the caller decides how to degrade a
// non-servable seat (the R16 contradiction screen resolves its alias first and
// degrades to the workhorse one-stage confirm only when the resolved seat is
// non-servable; F6a). This keeps platform callers addressing ALIASES (S12.4)
// and swap-invisible (R15): an operator retarget of the alias to a servable
// seat is honored. Nil-safe.
func (d *Duty) ResolveSeat(alias string) (SeatRecord, error) {
	if d == nil || d.reg == nil {
		return SeatRecord{}, ErrStackAbsent
	}
	return d.reg.SeatFor(alias)
}

// Call runs one duty-alias call on the consuming run, writing ONE $0 D7 usage
// row (R18). runID is the consuming run (intake seams ride <task>.intake,
// tie-break rides the routing/intake run). Errors are for the caller to
// degrade per its S12.4/S06 row (R17) — never faked.
func (d *Duty) Call(ctx context.Context, runID string, in DutyRequest) (DutyResult, error) {
	if d == nil || d.client == nil || d.reg == nil || d.cps == nil {
		return DutyResult{}, ErrStackAbsent
	}
	if runID == "" {
		return DutyResult{}, fmt.Errorf("local: duty %q call needs a consuming run (R18)", in.Alias)
	}
	if d.adm != nil && d.adm.Stopped() {
		return DutyResult{}, ErrAdmissionsStopped
	}
	seat, err := d.reg.SeatFor(in.Alias)
	if err != nil {
		return DutyResult{}, err // embedder post-gate / unknown alias / no seat
	}
	return d.callOnSeat(ctx, runID, in.Alias, seat, in)
}

// CallSeat runs a duty request on an EXPLICIT seat, bypassing alias resolution
// (R4): the intake-triage low-margin re-check re-runs the SAME duty on the
// workhorse seat, so the D7 marker records duty=<alias> model=<workhorse>. The
// alias is carried for the marker/duty label only.
func (d *Duty) CallSeat(ctx context.Context, runID, alias string, seat SeatRecord, in DutyRequest) (DutyResult, error) {
	if d == nil || d.client == nil || d.cps == nil {
		return DutyResult{}, ErrStackAbsent
	}
	if runID == "" {
		return DutyResult{}, fmt.Errorf("local: duty %q call needs a consuming run (R18)", alias)
	}
	if d.adm != nil && d.adm.Stopped() {
		return DutyResult{}, ErrAdmissionsStopped
	}
	return d.callOnSeat(ctx, runID, alias, seat, in)
}

// callOnSeat is the shared call core (servable check → abstain belt → admission
// pre-flight → /v1 call → D7 metering). alias labels the D7 marker; seat picks
// the model.
func (d *Duty) callOnSeat(ctx context.Context, runID, alias string, seat SeatRecord, in DutyRequest) (DutyResult, error) {
	if !seat.Servable {
		return DutyResult{}, fmt.Errorf("%w: %q — %s", ErrNotServable, seat.Model, seat.Note)
	}
	if in.Classification && len(in.Schema) > 0 {
		if err := assertAbstain(in.Schema); err != nil {
			return DutyResult{}, err
		}
	}

	// OQ2 LIVE pre-flight: run the S12.7 VRAM + S12.8 battery admission check
	// BEFORE the call triggers a llama-swap load, so operator-wins holds live (a
	// game/compositor holding VRAM refuses the load; battery policy parks
	// non-urgent duties). Nil-safe: no admitter ⇒ no gate (dev default). A
	// refusal surfaces as ErrGPUAdmissionRefused and the caller degrades per its
	// S12.4/S06 row (R17) — never a faked load.
	if d.admitter != nil {
		ok, reason, err := d.admitter.AdmitDuty(ctx, seat.Model, aliasUrgent(alias))
		if err != nil {
			return DutyResult{}, fmt.Errorf("local: GPU admission pre-flight for duty %q: %w", alias, err)
		}
		if !ok {
			return DutyResult{}, fmt.Errorf("%w: duty %q (%s)", ErrGPUAdmissionRefused, alias, reason)
		}
	}

	comp, err := d.client.Chat(ctx, ChatSpec{
		Model:     seat.Model,
		System:    in.System,
		User:      in.User,
		Schema:    in.Schema,
		Name:      in.Name,
		MaxTokens: in.MaxTokens,
		Logprobs:  in.Classification,
		// Classification duties are greedy label emitters — disable any
		// reasoning-model <think> phase so the length cap yields the JSON.
		NoThink: in.Classification,
	})
	if err != nil {
		return DutyResult{}, err
	}

	// D7: ONE checkpoint row on the consuming run with the local marker (R18).
	usage, err := UsageJSON(alias, seat.Model, seat.ModelHash(), LlamaCppPin, comp.InputTokens, comp.OutputTokens)
	if err != nil {
		return DutyResult{}, err
	}
	cp, err := d.cps.Write(ctx, gates.NewCheckpoint{
		RunID:            runID,
		Usage:            usage,
		SessionSubstrate: "local",
		ModelID:          seat.Model,
	})
	if err != nil {
		d.surfaceUnmeteredDefect(ctx, runID, alias, err)
		return DutyResult{}, fmt.Errorf("local: D7 metering write failed for duty %q on run %q: %w", alias, runID, err)
	}

	return DutyResult{
		Alias:        alias,
		Model:        seat.Model,
		Seat:         seat,
		Content:      comp.Content,
		Logprobs:     comp.Logprobs,
		InputTokens:  comp.InputTokens,
		OutputTokens: comp.OutputTokens,
		CheckpointID: cp.ID,
	}, nil
}

// surfaceUnmeteredDefect logs LOUDLY and records a platform-scope event when a
// local call could not land its mandatory D7 row (R18) — never silent.
func (d *Duty) surfaceUnmeteredDefect(ctx context.Context, runID, alias string, cause error) {
	d.logger.Error("local: UNMETERED-CALL DEFECT — a local duty call could not write its mandatory D7 row (R18/S12.1)",
		"run", runID, "duty", alias, "cause", cause)
	if d.log == nil {
		return
	}
	payload, _ := json.Marshal(struct {
		Run   string `json:"run"`
		Duty  string `json:"duty"`
		Cause string `json:"cause"`
	}{runID, alias, cause.Error()})
	if _, err := d.log.Append(ctx, eventlog.Append{
		UserID: "platform", Type: EventLocalUnmeteredDefect, SchemaVersion: dutyEventSchemaVersion, Payload: payload,
	}); err != nil {
		d.logger.Error("local: could not record the unmetered-call defect event", "run", runID, "err", err)
	}
}

// abstainTokens are the sanctioned abstain/unclear member spellings (S12.4).
var abstainTokens = []string{"abstain", "unclear", "unknown"}

// assertAbstain is the belt enforcing S12.4's "every [classification] schema
// carries an explicit abstain/unclear member" (R16) — the schema builders in
// schema.go add one by construction; this catches a hand-built schema that
// forgot it. A model is never schema-forced to fabricate a label.
func assertAbstain(schema json.RawMessage) error {
	if !json.Valid(schema) {
		return fmt.Errorf("local: duty schema is not valid JSON")
	}
	s := string(schema)
	for _, t := range abstainTokens {
		if strings.Contains(s, `"`+t+`"`) {
			return nil
		}
	}
	return fmt.Errorf("local: classification duty schema carries no abstain/unclear member (S12.4: a model is never schema-forced to fabricate a label)")
}
