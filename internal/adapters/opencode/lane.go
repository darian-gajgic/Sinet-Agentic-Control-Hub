package opencode

// Lane commissioning on this substrate (Spec S03.6): "adding a lane is a
// provider entry per user plus billing flags — never a new substrate."
//
// Everything a lane IS lives in this file as a shape and in lanedata/ as
// DATA. No endpoint string and no model id is a Go constant here: the values
// below arrive from a configuration document that carries its own
// `verified_on` dates, because those facts move — three of the Z.AI seed's
// rows changed inside five weeks — and a constant would quietly go stale
// where a dated row goes visibly stale. The seed is SEED DATA, not the
// authority; the account's own observed model list is (P-T17-3).

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// The lane documents ship as a DIRECTORY embed, not one file at a time:
// S03.6's "adding a lane is config-only" is only true if dropping a document
// in genuinely adds a lane (the internal/storage/migrate.go precedent).
//
//go:embed lanedata/*.json
var laneSeeds embed.FS

// ErrLaneConfig reports a lane document that cannot be commissioned.
var ErrLaneConfig = errors.New("opencode: lane configuration")

// LaneConfig is one lane's commissioning document.
type LaneConfig struct {
	// Lane is the platform lane name (runs.lane); ProviderID is the provider
	// key inside opencode's own config, and the first half of the platform
	// model string `<providerID>/<modelID>`.
	Lane string `json:"lane"`
	// Substrate names the D3 engine substrate this lane runs ON.
	//
	// It is on the DOCUMENT because a lane is the only thing that knows it:
	// "adding a lane is a provider entry per user" (S03.6), and which engine
	// serves that entry is part of the entry. Without it the platform can seat
	// a model on lane zai and still dispatch the run to whichever substrate
	// happened to be the process default — the run would execute on the wrong
	// engine and meter against the wrong lane, with nothing anywhere
	// contradicting it.
	Substrate   string `json:"substrate"`
	ProviderID  string `json:"provider_id"`
	NPM         string `json:"npm"`
	DisplayName string `json:"display_name"`
	VerifiedOn  string `json:"verified_on"`
	Source      string `json:"source"`

	// BaseURL is the endpoint the provider entry points at. EndpointMarker is
	// the substring that proves it is the SUBSCRIPTION endpoint rather than a
	// sibling that spends a different balance (R11's self-check input).
	BaseURL        string `json:"base_url"`
	EndpointMarker string `json:"endpoint_marker"`
	EndpointNote   string `json:"endpoint_note"`

	// RecordedEndpoints are dated facts about endpoints that exist and are
	// deliberately NOT wired.
	RecordedEndpoints []RecordedEndpoint `json:"recorded_endpoints"`

	Credential LaneCredential `json:"credential"`
	Models     []LaneModel    `json:"models"`

	// DefaultModel is the model this lane's execution seat takes (S08.8).
	// On the document because the lane owns its own model facts.
	DefaultModel     string `json:"default_model"`
	DefaultModelNote string `json:"default_model_note,omitempty"`

	ModelRoutingNote string `json:"model_routing_note"`

	// EcoOptions are the provider options this lane's low-consumption rung
	// adds to its models. Data, because the lever is the vendor's shape.
	EcoOptions           map[string]json.RawMessage `json:"eco_options"`
	EcoOptionsVerifiedOn string                     `json:"eco_options_verified_on"`

	// EcoOptionEfforts names the S10.6 effort modes that get EcoOptions.
	//
	// It is DATA rather than a rule in code because the spec calls the Z.AI
	// thinking lever "an Eco/Balanced rung" and does not settle whether
	// Balanced takes it by default — a judgement about how much quality a
	// person trades for consumption, which belongs to the operator and to a
	// data row they can change, not to a constant an executor picked. Empty
	// defaults to Eco alone, the conservative reading.
	EcoOptionEfforts []string `json:"thinking_disabled_efforts"`

	// ReasoningEffort is a RECORDED, unwired vendor knob (see the type).
	ReasoningEffort RecordedKnob `json:"reasoning_effort"`

	// ResetMarker is the verbatim phrase the provider's depletion messages put
	// before the resume time.
	ResetMarker string `json:"reset_marker"`

	// OverflowNote records the lane-wide overflow posture in prose: the proven
	// disable that clears 3.10, and the operator posture recommended with it.
	// The per-model enum value is the machine-readable half (LaneModel).
	OverflowNote string `json:"overflow_note,omitempty"`

	// SignalsNote is the honesty label on the whole signal table — whether its
	// rows are OBSERVED wire bodies or DOCUMENTED message strings, and what
	// closes the difference.
	SignalsNote string `json:"signals_note,omitempty"`

	// DataPolicy is the per-lane routing constraint the report-02 §5 C5 gate
	// produces (S03.6; the DeepSeek precedent, R02 §4).
	DataPolicy LaneDataPolicy `json:"data_policy,omitempty"`

	// InstalledCatalogue records what the INSTALLED engine's own provider
	// catalogue says about this lane, and where it diverges from the document.
	// The engine's record is authoritative over documentation for what the
	// engine will accept, and still not authoritative over the ACCOUNT — that
	// is the model-list canary's job (P-T17-3).
	InstalledCatalogue LaneCatalogueRecord `json:"installed_catalogue_record,omitempty"`

	Signals []LaneSignalRow `json:"signals"`
}

// LaneDataPolicy is a lane's C5 data-routing constraint, carried as DATA with
// its citation and its enforcement state.
//
// Enforced is on the row because the honest answer at v0 is FALSE: no per-lane
// data-policy enforcement point exists in the tree (grep-verified 2026-08-24),
// so the constraint is recorded and surfaced to the operator rather than
// machine-enforced. Inventing an enforcement seam here was out of scope; what
// this row does is give the routing-policy seam, when it lands, one place to
// read instead of a sentence in a document nobody parses.
type LaneDataPolicy struct {
	// Constraint is the machine-readable constraint name.
	Constraint string `json:"constraint,omitempty"`
	// Statement is the rider in the operator's own words.
	Statement string `json:"statement,omitempty"`
	// Basis is the primary-sourced finding the constraint rests on.
	Basis string `json:"basis,omitempty"`
	// Enforced reports whether anything in the tree ENFORCES this. False is
	// not a placeholder: it is the state, and saying otherwise would let a
	// reader assume the code stops what only a person can.
	Enforced bool `json:"enforced"`
	// EnforcementNote says plainly what the false above means.
	EnforcementNote string `json:"enforcement_note,omitempty"`
	VerifiedOn      string `json:"verified_on,omitempty"`
	Source          string `json:"source,omitempty"`
	// Audit is the repo path of the onboarding audit this row came from.
	Audit string `json:"audit,omitempty"`
}

// RecordedEndpoint is an endpoint that exists and is not wired, kept dated so
// a later packet does not have to re-derive it.
type RecordedEndpoint struct {
	Protocol   string `json:"protocol"`
	BaseURL    string `json:"base_url"`
	Wired      bool   `json:"wired"`
	VerifiedOn string `json:"verified_on"`
	Note       string `json:"note"`
}

// RecordedKnob is a vendor parameter recorded as a dated fact and deliberately
// not wired: no named spec section sanctions it, and inventing behavior around
// a real parameter is how an unratified rung gets onto an effort ladder.
type RecordedKnob struct {
	Wired      bool     `json:"wired"`
	VerifiedOn string   `json:"verified_on"`
	Values     []string `json:"values"`
	// ValuesGrade is the evidence grade of Values, same meaning as
	// LaneModel.NoteGrade: an unverified list stays unwired AND says so.
	ValuesGrade string `json:"values_grade,omitempty"`
	Note        string `json:"note"`
}

// LaneCredential names the broker auth-profile and the environment variable
// its spawn-time injection fills. A NAME and a variable name — never material
// (S11.5: workers hold auth-profile references, never raw secrets).
type LaneCredential struct {
	Profile string `json:"profile"`
	EnvVar  string `json:"env_var"`
	Note    string `json:"note"`
}

// LaneCatalogueRecord is a dated read of the installed engine's own provider
// catalogue: what it AGREES with, what it DIVERGES on, and what it adds. A
// divergence is recorded rather than silently resolved, the same treatment the
// endpoint discrepancies get — a fact somebody quietly reconciled is a fact
// nobody can re-check.
type LaneCatalogueRecord struct {
	VerifiedOn string `json:"verified_on,omitempty"`
	Source     string `json:"source,omitempty"`
	// ModelsVerbatim is the engine record's model id list, exactly as read.
	// It is the field that makes the rest checkable: a claim about what an
	// engine accepts is worth what its source listing is worth.
	ModelsVerbatim     []string `json:"models_verbatim,omitempty"`
	ModelsVerbatimNote string   `json:"models_verbatim_note,omitempty"`
	Agrees             string   `json:"agrees,omitempty"`
	Diverges           string   `json:"diverges,omitempty"`
	Limits             string   `json:"limits,omitempty"`
	ReasoningOptions   string   `json:"reasoning_options,omitempty"`
	// SecondSourceDisagrees records another record on the same host that
	// contradicts this one, dated, rather than picking a winner.
	SecondSourceDisagrees string `json:"second_source_disagrees,omitempty"`
	Additional            string `json:"additional,omitempty"`
	Note                  string `json:"note,omitempty"`
}

// LaneModel is one model's per-model attribute row (S03.6/S18.3: a data-valued
// settings surface with no dotted key).
type LaneModel struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	VerifiedOn      string `json:"verified_on"`
	Billing         string `json:"billing"`
	OverflowMode    string `json:"overflow_mode"`
	RegionModelGate string `json:"region_model_gate"`
	Note            string `json:"note,omitempty"`
	// NoteGrade is the evidence grade of Note. "unverified-primary" marks a
	// line that reached this document from something other than the onboarding
	// audit's own captures — the packet's brief, or a secondary source — so a
	// later reader never mistakes it for a primary quote.
	NoteGrade string `json:"note_grade,omitempty"`
	// ObservationGrade marks an id this document declares that the pinned
	// engine's own record does NOT carry. It is not a deletion: the id stays,
	// flagged, because the account's observed list is what settles it
	// (P-T17-3) and dropping a documented id on an engine's say-so would lose
	// a fact the vendor publishes.
	ObservationGrade string `json:"observation_grade,omitempty"`
	// MeteredDisableProven records that the account's pay-as-you-go spill has
	// been proven disabled (or its balance proven zero). Only such a model may
	// declare overflow_mode auto-metered.
	MeteredDisableProven bool `json:"metered_disable_proven"`
}

// Billing values (S10.2 flat/metered flag).
const (
	BillingFlat    = "flat"
	BillingMetered = "metered"
)

// Overflow modes — the ratified S03.6 enum, verbatim (Spec S03.6; S18.3's
// per-model attribute row). The set is fixed by the spec, not by this lane:
// inventing a vocabulary here would make a document that validates against
// Sinet and means nothing to anyone reading the spec.
//
// TBD-S10.2(currency-flip receipts): with the kimi lane, Sinet holds its first
// non-`hard-stop` lane, so 3.10's "receipts must visibly change currency when
// overflow triggers" (report-01 P-T01-3) stops being theoretical. The value is
// carried end to end from the document and is distinguishable between lanes at
// every surface that reads it — but NO surface reads billing regime yet, so
// "end to end" ends at the validated document, and the currency-changing
// receipt path belongs to whichever packet lands S10.2's flip mechanics (the
// ProposePlanBudget/13.4 precedent: build the honest half, name the consumer,
// do not invent the surface). The flip itself is an operator act through the
// rehearsed kill-switch, never automatic and never silent (R02 §6).
const (
	// OverflowHardStop stops at the plan's own ceiling.
	OverflowHardStop = "hard-stop"
	// OverflowOptInCredits spends further credits only on an explicit
	// per-use opt-in (3.10: metered spending is never a default).
	OverflowOptInCredits = "opt-in-credits"
	// OverflowAutoMetered spills to per-token billing automatically —
	// acceptable ONLY with a proven disable/zero balance, refused at
	// onboarding otherwise (P-T17-2).
	OverflowAutoMetered = "auto-metered"
)

// LaneSignalRow is one wire signal the lane can emit: the provider's own error
// code and the HTTP status it arrives on. A row may name a BAND (code_from /
// code_to) whose members the provider documents only collectively — the status
// is still known, which is what lets the band classify on its signal rather
// than on a code nobody has published.
type LaneSignalRow struct {
	Code       string `json:"code"`
	CodeFrom   string `json:"code_from"`
	CodeTo     string `json:"code_to"`
	HTTPStatus int    `json:"http_status"`
	VerifiedOn string `json:"verified_on"`
	Message    string `json:"message"`
	// MessageContains keys a row on a MESSAGE SUBSTRING rather than on a
	// provider error code, for a lane whose published taxonomy is
	// (HTTP status x message string) and carries no numeric code at all.
	// Additive and optional: a document with no such row behaves exactly as it
	// did before this member existed, and code-keyed rows keep winning.
	MessageContains string `json:"message_contains,omitempty"`
	// DocumentedClass is the NARROW OQ1(a) exemption vocabulary. A row may
	// declare that its (status, message) pair means one of a CLOSED set of
	// things on this lane's wire — and the set deliberately excludes `auth`
	// and `balance`, because those are the directions in which a document edit
	// could suppress a lane freeze. It is an exemption list, never a
	// re-ordering: a row that declares nothing classifies on its status
	// exactly as before.
	DocumentedClass string `json:"documented_class,omitempty"`
	// SemanticsVerified is false for a band the provider documents only as a
	// group; the row is honest about what it does not know.
	SemanticsVerified bool   `json:"semantics_verified"`
	Note              string `json:"note"`
}

// The documented-class vocabulary a lane document may declare on a signal row
// (the OQ1(a) exemption set). It is CLOSED, and it deliberately has no `auth`
// and no `balance` member: those would let a data edit suppress a lane freeze,
// and the fuller ratified `semantics` member is the pre-registered later path
// (OQ1(b), an S00.9 amendment when a third coded lane arrives).
//
// The classifier reads the same four strings from its own package, because
// neither package may import the other; internal/shell is where the two
// vocabularies are pinned to agree.
const (
	// DocumentedTransient: capacity shed — retry in place, never park.
	DocumentedTransient = "transient"
	// DocumentedDepletion: an allowance window is spent — park.
	DocumentedDepletion = "depletion"
	// DocumentedModelDrift: the account does not serve the model asked for —
	// surfaced, never parked and never a lane freeze (P-T17-3).
	DocumentedModelDrift = "model-drift"
	// DocumentedEndpointDefect: the request reached the wrong path — a
	// configuration defect, surfaced rather than parked (P-T08-2).
	DocumentedEndpointDefect = "endpoint-defect"
)

// documentedClasses is the closed set validation accepts.
var documentedClasses = map[string]bool{
	DocumentedTransient:      true,
	DocumentedDepletion:      true,
	DocumentedModelDrift:     true,
	DocumentedEndpointDefect: true,
}

// LaneSignal is the normalized wire signal this adapter forwards as DATA on a
// rate_limit event (S03.1: "adapters forward the raw signals as data"; the
// five-class taxonomy is the scheduler's, never the adapter's). Its members are
// exactly the inputs Spec S10.5's classifier takes, and the JSON names are the
// contract between the two — this package cannot import the scheduler
// (CONVENTIONS §12 import discipline) and does not classify anything.
type LaneSignal struct {
	Lane       string    `json:"lane"`
	ErrorCode  string    `json:"error_code,omitempty"`
	HTTPStatus int       `json:"http_status,omitempty"`
	ResetAt    time.Time `json:"reset_at,omitempty"`
	BodyText   string    `json:"body_text,omitempty"`
	// EndpointVerified reports whether the lane's CONFIGURED endpoint is the
	// subscription endpoint. The classifier needs it as an INPUT because the
	// same code means "the plan is spent" on the right endpoint and "you are
	// pointed at the wrong one" on another, and a classifier that went looking
	// for the answer itself would stop being pure and total (S10.5).
	EndpointVerified bool `json:"endpoint_verified"`
	// Known reports whether the lane's signal table names this code. An
	// unknown code is forwarded as data, never dropped and never fatal.
	Known bool `json:"known"`
	// DocumentedClass is the class the lane DOCUMENT names for this exact
	// (status, message) pair, or empty. It is resolved here, where the document
	// is, and forwarded as DATA — because the classifier is pure and total and
	// must never go looking for a document (S10.5; the EndpointVerified
	// precedent).
	DocumentedClass string `json:"documented_class,omitempty"`
	// EngineRetry is the engine's OWN retry context, verbatim, when the signal
	// was lifted out of one (type/attempt/next). Normalizing a signal must not
	// cost the raw facts it was normalized from: the platform classifies from
	// the normalized members and an operator still sees what the engine said
	// (S03.1 forward-as-data).
	EngineRetry json.RawMessage `json:"engine_retry,omitempty"`
}

// MarshalJSON omits a zero ResetAt ENTIRELY.
//
// `omitempty` does nothing for a time.Time — it is a struct, never "empty" —
// so the default encoding puts "0001-01-01T00:00:00Z" on the wire for every
// signal that carries no resume time. A consumer that reads a park horizon out
// of that member (internal/api's string-fallback read does exactly this) would
// then show a person a park until the year 1 where the honest answer is "no
// resume time was signalled". Absent has to mean absent.
func (s LaneSignal) MarshalJSON() ([]byte, error) {
	// laneSignalWire has no methods, so marshalling it cannot recurse.
	type laneSignalWire LaneSignal
	if s.ResetAt.IsZero() {
		return json.Marshal(struct {
			laneSignalWire
			// Shadows the embedded member at depth 0 and encodes to nothing.
			ResetAt json.RawMessage `json:"reset_at,omitempty"`
		}{laneSignalWire: laneSignalWire(s)})
	}
	return json.Marshal(laneSignalWire(s))
}

// SeedLaneConfigs returns the lane documents that ship with the platform.
// They are seed DATA — a starting point an operator's configuration replaces
// per user — not a compiled-in lane.
func SeedLaneConfigs() ([]LaneConfig, error) {
	return loadLaneDocs(laneSeeds, "lanedata")
}

// loadLaneDocs walks a directory of lane documents, validates each, and returns
// them SORTED BY LANE NAME.
//
// The sort is not cosmetic: with a single-file embed the order was whatever one
// call site wrote, and a caller indexing lanes[0] happened to be right. A
// directory walk has no such luck, so the order is DECLARED — and every caller
// that cared was moved to select by lane name in the same packet.
//
// TWO GATES ride this, and neither existed while one document shipped:
// a duplicate LANE name and a duplicate PROVIDER ID are refused BY NAME. Both
// failure modes are silent otherwise — laneFor takes the first provider-id
// match and laneConfiguredModels overwrites out[l.Lane] — and a config-only
// lane system whose failure mode is silent shadowing is not one.
func loadLaneDocs(fsys fs.FS, dir string) ([]LaneConfig, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrLaneConfig, dir, err)
	}
	var out []LaneConfig
	byLane := map[string]string{}
	byProvider := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		path := dir + "/" + name
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("%w: read %s: %w", ErrLaneConfig, path, err)
		}
		c, err := LoadLaneConfig(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if first, dup := byLane[c.Lane]; dup {
			return nil, fmt.Errorf("%w: %s and %s both declare lane %q — lane resolution takes the FIRST match, "+
				"so the second document would be silently unreachable (S03.6)", ErrLaneConfig, first, path, c.Lane)
		}
		if first, dup := byProvider[c.ProviderID]; dup {
			return nil, fmt.Errorf("%w: %s and %s both declare provider id %q — a provider id resolves to exactly one "+
				"lane, so the second would silently shadow the first (S03.6)", ErrLaneConfig, first, path, c.ProviderID)
		}
		byLane[c.Lane], byProvider[c.ProviderID] = path, path
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: %s holds no lane documents", ErrLaneConfig, dir)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lane < out[j].Lane })
	return out, nil
}

// LoadLaneConfig parses and VALIDATES one lane document. Validation is the
// onboarding gate: a lane that cannot be described honestly is refused here
// rather than discovered at the first paid call.
func LoadLaneConfig(raw []byte) (LaneConfig, error) {
	var c LaneConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return LaneConfig{}, fmt.Errorf("%w: %w", ErrLaneConfig, err)
	}
	if err := c.validate(); err != nil {
		return LaneConfig{}, err
	}
	return c, nil
}

func (c LaneConfig) validate() error {
	switch {
	case c.Lane == "":
		return fmt.Errorf("%w: no lane name", ErrLaneConfig)
	case c.Substrate == "":
		return fmt.Errorf("%w (lane %q): no substrate — a lane that does not name the engine it runs on can be "+
			"seated by routing and then dispatched to whichever substrate is the process default (S03.6)", ErrLaneConfig, c.Lane)
	case c.ProviderID == "":
		return fmt.Errorf("%w (lane %q): no provider id", ErrLaneConfig, c.Lane)
	case c.BaseURL == "":
		return fmt.Errorf("%w (lane %q): no endpoint", ErrLaneConfig, c.Lane)
	case c.VerifiedOn == "":
		return fmt.Errorf("%w (lane %q): the endpoint carries no verified-on date", ErrLaneConfig, c.Lane)
	case len(c.Models) == 0:
		return fmt.Errorf("%w (lane %q): no models", ErrLaneConfig, c.Lane)
	}
	if _, err := time.Parse(dateLayout, c.VerifiedOn); err != nil {
		return fmt.Errorf("%w (lane %q): verified-on %q is not a %s date", ErrLaneConfig, c.Lane, c.VerifiedOn, dateLayout)
	}
	seen := map[string]bool{}
	for _, m := range c.Models {
		switch {
		case m.ID == "":
			return fmt.Errorf("%w (lane %q): a model has no id", ErrLaneConfig, c.Lane)
		case seen[m.ID]:
			return fmt.Errorf("%w (lane %q): model %q is listed twice", ErrLaneConfig, c.Lane, m.ID)
		case m.VerifiedOn == "":
			return fmt.Errorf("%w (lane %q): model %q carries no verified-on date", ErrLaneConfig, c.Lane, m.ID)
		case m.Billing != BillingFlat && m.Billing != BillingMetered:
			return fmt.Errorf("%w (lane %q): model %q declares billing %q, want %q or %q",
				ErrLaneConfig, c.Lane, m.ID, m.Billing, BillingFlat, BillingMetered)
		}
		if _, err := time.Parse(dateLayout, m.VerifiedOn); err != nil {
			return fmt.Errorf("%w (lane %q): model %q verified-on %q is not a %s date",
				ErrLaneConfig, c.Lane, m.ID, m.VerifiedOn, dateLayout)
		}
		// S03.6/3.10: a model that spills to metered usage without a PROVEN
		// disable or zero balance is refused at onboarding. Accepting it would
		// make an unbounded bill reachable from a flat-rate plan by default,
		// which is the one thing the flat-rate posture exists to prevent.
		if m.OverflowMode == OverflowAutoMetered && !m.MeteredDisableProven {
			return fmt.Errorf("%w (lane %q): model %q declares overflow_mode %q without a proven metered "+
				"disable or zero balance — refused at onboarding (S03.6, P-T17-2)", ErrLaneConfig, c.Lane, m.ID, m.OverflowMode)
		}
		switch m.OverflowMode {
		case OverflowHardStop, OverflowOptInCredits, OverflowAutoMetered:
		default:
			return fmt.Errorf("%w (lane %q): model %q declares overflow_mode %q, want one of %q/%q/%q (S03.6)",
				ErrLaneConfig, c.Lane, m.ID, m.OverflowMode, OverflowHardStop, OverflowOptInCredits, OverflowAutoMetered)
		}
		// region_model_gate is the OTHER required per-model attribute (S03.6):
		// the model list is region- and account-dependent, so a model that
		// declares no gate state cannot be diffed against what the account
		// actually serves (P-T17-3).
		if m.RegionModelGate == "" {
			return fmt.Errorf("%w (lane %q): model %q declares no region_model_gate, which S03.6 requires "+
				"per model (P-T17-3)", ErrLaneConfig, c.Lane, m.ID)
		}
		seen[m.ID] = true
	}
	// A default model that is not in the model list would seat routing on a
	// model this lane does not declare — a seat pointing at nothing.
	if c.DefaultModel != "" && !seen[c.DefaultModel] {
		return fmt.Errorf("%w (lane %q): default_model %q is not one of the lane's models", ErrLaneConfig, c.Lane, c.DefaultModel)
	}
	// A lane with no credential reference cannot be commissioned, and a
	// document that omits it would sail past the spawn-time check and let the
	// engine authenticate as nobody — the silent unauthenticated call this
	// lane's whole commissioning posture exists to prevent (S11.5).
	if c.Credential.EnvVar == "" || c.Credential.Profile == "" {
		return fmt.Errorf("%w (lane %q): no credential reference — a lane document must name the broker "+
			"auth profile and the environment variable its injection fills, or nothing can prove the lane "+
			"is commissioned (S11.5)", ErrLaneConfig, c.Lane)
	}
	if c.EndpointMarker == "" {
		// Without a marker every endpoint self-check answers "unverified",
		// which turns every 1113 into a configuration defect and hides real
		// depletion behind a permanent false alarm (R11).
		return fmt.Errorf("%w (lane %q): no endpoint_marker — the subscription-endpoint self-check would "+
			"answer 'unverified' for every signal and mask real depletion (S10.5)", ErrLaneConfig, c.Lane)
	}
	for _, row := range c.Signals {
		name := row.Code
		if name == "" {
			name = row.CodeFrom + "-" + row.CodeTo
		}
		if name == "-" && row.MessageContains != "" {
			name = row.MessageContains
		}
		// The exemption vocabulary is CLOSED. A row declaring something outside
		// it would be inert at the classifier, which is the worst outcome: the
		// document would look like it said something and nothing would read it.
		// `auth` and `balance` are absent on purpose — a data edit must never be
		// able to suppress a lane freeze (OQ1(a) constraint ii).
		if row.DocumentedClass != "" && !documentedClasses[row.DocumentedClass] {
			return fmt.Errorf("%w (lane %q): signal row %q declares documented_class %q, want one of %q/%q/%q/%q — "+
				"the exemption vocabulary is closed, and it carries no auth or balance member by design (S10.5)",
				ErrLaneConfig, c.Lane, name, row.DocumentedClass,
				DocumentedTransient, DocumentedDepletion, DocumentedModelDrift, DocumentedEndpointDefect)
		}
		// A message-keyed row with no status cannot be keyed at all: this
		// taxonomy is (status x message), and half of a key is not a key.
		if row.MessageContains != "" && row.HTTPStatus == 0 {
			return fmt.Errorf("%w (lane %q): signal row %q keys on a message but names no http_status — "+
				"a message-keyed taxonomy is (status x message), and a row with only half of that key matches nothing",
				ErrLaneConfig, c.Lane, name)
		}
		if row.VerifiedOn == "" {
			return fmt.Errorf("%w (lane %q): signal row %q carries no verified-on date", ErrLaneConfig, c.Lane, name)
		}
		if _, err := time.Parse(dateLayout, row.VerifiedOn); err != nil {
			return fmt.Errorf("%w (lane %q): signal row %q verified-on %q is not a %s date",
				ErrLaneConfig, c.Lane, name, row.VerifiedOn, dateLayout)
		}
	}
	return nil
}

// dateLayout is the verified-on date shape. A mechanic, not a ⚙ value.
const dateLayout = "2006-01-02"

// EcoOptionApplies reports whether an effort mode takes this lane's
// low-consumption options. The document decides; empty means Eco alone.
func (c LaneConfig) EcoOptionApplies(effort string) bool {
	if len(c.EcoOptionEfforts) == 0 {
		return effort == adapters.EffortEco
	}
	for _, e := range c.EcoOptionEfforts {
		if e == effort {
			return true
		}
	}
	return false
}

// CredentialInjectionFacts records what the S11.5 credential-injection proxy
// will need when it is built, so the packet that builds it does not re-derive
// provider wire facts from scratch. DEFERRED, by coordinator disposition
// (OQ-1): the proxy does not exist, and the broker's existing spawn-time
// engine-cred delivery is the v0 dev-posture-correct path, because S11.5's D2
// invariant guards the TASK sandbox and a per-user serve is not one.
//
// The facts, verified 2026-08-23 alongside this lane's document:
//
//   - inject ONLY on the coding endpoint's completion path
//     (<base_url>/chat/completions); every other request passes untouched;
//   - the header is `Authorization: Bearer <token>`;
//   - CA trust is PER-PROCESS via NODE_EXTRA_CA_CERTS — the system trust
//     store is never touched;
//   - the P-T01-2 pin-regression canary asserts, per engine version, that a
//     trusted-CA terminating proxy on the model path still yields 200; on
//     regression, ⚙ sandbox.model_egress_tls_terminate = false for that lane
//     falls back to pattern-2 scoped egress;
//   - SPIKE P2-S3 demonstrated a Z.AI 401→200 from proxy-side substitution
//     alone, so the mechanism is proven for this provider;
//   - Z.AI publishes NO x-ratelimit-* headers, so the proxy's second purpose
//     on the Anthropic lane — harvesting anthropic-ratelimit-unified-* — has
//     no analogue here. This lane's limit state rides error-code BODIES, which
//     is what ExtractSignal reads.
const CredentialInjectionFacts = "S11.5 injection proxy: deferred to the D6/host batch (OQ-1); see the doc comment on this constant"

// Providers renders the lane as the provider block opencode compiles into its
// config: one entry, keyed by the provider id.
//
// The credential rides `options.apiKey` as an ENV REFERENCE, never a value:
// the material is resolved from the broker at spawn into the serve
// environment, and the config the engine reads names the variable only.
func (c LaneConfig) Providers() ProviderConfig {
	return ProviderConfig{c.ProviderID: c.ProviderEntry()}
}

// ProviderEntry renders the lane's single provider entry.
func (c LaneConfig) ProviderEntry() ProviderEntry {
	models := make(map[string]ModelEntry, len(c.Models))
	for _, m := range c.Models {
		models[m.ID] = ModelEntry{Name: m.Name}
	}
	opts := map[string]any{"baseURL": c.BaseURL}
	if c.Credential.EnvVar != "" {
		opts["apiKey"] = "{env:" + c.Credential.EnvVar + "}"
	}
	return ProviderEntry{
		NPM:     c.NPM,
		Name:    c.DisplayName,
		Options: opts,
		Models:  models,
	}
}

// Commissionable reports whether this document CAN be commissioned at all: it
// must name both an auth profile to resolve and an environment variable to
// deliver the material as.
//
// It is one predicate on purpose. The spawn-time injector refuses to build on
// exactly this conjunction, and commissioning must refuse on the same one — a
// lane registered as held whose credential path is missing gets seated by
// routing and then authenticates as nobody. Two spellings of the same rule in
// two packages is that failure waiting for one of them to be edited.
func (c LaneConfig) Commissionable() bool {
	return c.Credential.Profile != "" && c.Credential.EnvVar != ""
}

// Commission composes the provider entries each person actually holds, from
// the lane documents and what that person has PLACED (S03.6: "adding a lane is
// a provider entry per user"; S11.5).
//
// placed maps a person to the auth profiles that person has an engine
// credential under — a placement fact and never a secret, so nothing on this
// path can hold credential material. The result is person → (providerID →
// entry): the map a control plane hands to the adapter registration and to the
// spawn-time credential injector, so a lane can never be dispatchable without
// its credential path.
//
// A document declaring no auth profile or no environment variable is not
// commissionable and never enters the map, whatever is placed. That is the same
// conjunction the spawn-time injector refuses to build on: a lane registered as
// held with no credential path would be seated by routing and then authenticate
// as nobody, which is the state ErrLaneNotCommissioned exists to make visible.
func Commission(lanes []LaneConfig, placed map[string]map[string]bool) map[string]ProviderConfig {
	out := map[string]ProviderConfig{}
	for who, profiles := range placed {
		entries := ProviderConfig{}
		for _, l := range lanes {
			if !l.Commissionable() {
				continue
			}
			if profiles[l.Credential.Profile] {
				entries[l.ProviderID] = l.ProviderEntry()
			}
		}
		if len(entries) > 0 {
			out[who] = entries
		}
	}
	return out
}

// CommissionedLanes reports the lane names commissioned across every person,
// sorted — the S08.8 coverage input.
//
// Coverage is per-owner in S08.8 and a Router is built once per control plane,
// so this is the UNION: the honest over-approximation at v0 (one household, one
// operator placing keys), with per-person coverage arriving on the per-person
// duty-map surface (1.10, B6/v1).
func CommissionedLanes(lanes []LaneConfig, commissioned map[string]ProviderConfig) []string {
	byProvider := make(map[string]string, len(lanes))
	for _, l := range lanes {
		byProvider[l.ProviderID] = l.Lane
	}
	seen := map[string]bool{}
	var out []string
	for _, entries := range commissioned {
		for providerID := range entries {
			lane, ok := byProvider[providerID]
			if !ok || seen[lane] {
				continue
			}
			seen[lane] = true
			out = append(out, lane)
		}
	}
	sort.Strings(out)
	return out
}

// CommissionedSubstrates maps each commissioned lane to the substrate its own
// document names (S03.2) — which engine actually serves the work.
//
// Only commissioned lanes appear: mapping a lane nobody holds would describe a
// dispatch that cannot happen. With nothing commissioned this is nil and every
// dispatch takes its pre-commissioning path.
func CommissionedSubstrates(lanes []LaneConfig, commissioned map[string]ProviderConfig) map[string]string {
	live := map[string]bool{}
	for _, lane := range CommissionedLanes(lanes, commissioned) {
		live[lane] = true
	}
	if len(live) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, l := range lanes {
		if live[l.Lane] && l.Substrate != "" {
			out[l.Lane] = l.Substrate
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CommissionedSeat is one commissioned lane's execution seat: the model that
// lane fronts, as its own document records it (with that fact's verified-on
// date beside it). The routing package's seat type is composed FROM these at
// the composition root, so no model id is ever a constant there.
type CommissionedSeat struct {
	Lane  string
	Model string
}

// CommissionedSeats reports the execution seat of every commissioned lane, in
// document order. A lane whose document ships no default_model has no seat —
// a lane may ship without one, and refusing that would make the gate about
// something other than correctness.
func CommissionedSeats(lanes []LaneConfig, commissioned map[string]ProviderConfig) []CommissionedSeat {
	live := map[string]bool{}
	for _, lane := range CommissionedLanes(lanes, commissioned) {
		live[lane] = true
	}
	var out []CommissionedSeat
	for _, l := range lanes {
		if !live[l.Lane] || l.DefaultModel == "" {
			continue
		}
		out = append(out, CommissionedSeat{Lane: l.Lane, Model: l.DefaultModel})
	}
	return out
}

// VerifyEndpoint is the R11 self-check: does the entry a user actually holds
// point at the SUBSCRIPTION endpoint? A lane that declares no marker cannot
// answer, and says so rather than claiming a verification it never made.
func (c LaneConfig) VerifyEndpoint(entry ProviderEntry) (bool, string) {
	base, _ := entry.Options["baseURL"].(string)
	switch {
	case c.EndpointMarker == "":
		return false, fmt.Sprintf("lane %q declares no subscription-endpoint marker, so its configured endpoint cannot be verified", c.Lane)
	case base == "":
		return false, fmt.Sprintf("lane %q has no configured endpoint to verify", c.Lane)
	case !strings.Contains(base, c.EndpointMarker):
		return false, fmt.Sprintf("lane %q is configured against %q, which is not the subscription endpoint (expected a URL containing %q): %s",
			c.Lane, base, c.EndpointMarker, c.EndpointNote)
	}
	return true, ""
}

// statusFor reports the HTTP status the lane's dated signal table records for
// a provider error code, and whether the table names it at all.
func (c LaneConfig) statusFor(code string) (int, bool) {
	for _, row := range c.Signals {
		if row.Code != "" && row.Code == code {
			return row.HTTPStatus, true
		}
	}
	for _, row := range c.Signals {
		if row.CodeFrom == "" || row.CodeTo == "" {
			continue
		}
		if inCodeBand(code, row.CodeFrom, row.CodeTo) {
			// A band member is KNOWN to arrive on this status and NOT known
			// individually: the status is what makes the safe direction
			// reachable, so it is reported while the code stays unnamed.
			return row.HTTPStatus, false
		}
	}
	return 0, false
}

func inCodeBand(code, from, to string) bool {
	if len(code) != len(from) || len(code) != len(to) {
		return false
	}
	return code >= from && code <= to
}

// ExtractSignal pulls this lane's wire signal out of an engine-surfaced error.
//
// The provider's body shape is `{"error":{"code":"1308","message":"…"}}` —
// with `code` a JSON STRING, which is exactly the shape a numeric decode would
// reject. Engines wrap that body in their own error text, so the body is
// SEARCHED for rather than assumed to be the whole input, and anything
// unparseable yields no signal instead of a guess.
//
// observedStatus wins when the surface actually carried one; otherwise the
// lane's dated signal table supplies it. An unknown code is still returned —
// forwarded as data, never dropped, never fatal (S03.1).
// A lane whose taxonomy is MESSAGE-keyed has no code to decode at all, so
// producing nothing would leave only a bare HTTP status reaching the classifier
// — and on a wire where quota exhaustion arrives on 403, a bare status is the
// false-freeze. Such a lane therefore yields a signal from (status, message),
// and its Known says whether a documented row actually matched.
func (c LaneConfig) ExtractSignal(text string, observedStatus int) (LaneSignal, bool) {
	code, message, ok := decodeProviderError(text)
	if !ok {
		// No decodable envelope. Only a lane that PUBLISHES message-keyed rows
		// gets a signal out of that — for a code-keyed lane this is unchanged
		// and still yields nothing rather than a guess.
		if !c.hasMessageRows() || observedStatus <= 0 {
			return LaneSignal{}, false
		}
		sig := LaneSignal{Lane: c.Lane, HTTPStatus: observedStatus, BodyText: strings.TrimSpace(text)}
		if row, matched := c.messageRowFor(observedStatus, text); matched {
			sig.Known, sig.DocumentedClass = true, row.DocumentedClass
		}
		sig.ResetAt = c.parseResetAt(text)
		return sig, true
	}
	sig := LaneSignal{
		Lane:      c.Lane,
		ErrorCode: code,
		BodyText:  message,
	}
	status, known := c.statusFor(code)
	sig.Known = known
	sig.HTTPStatus = status
	if observedStatus > 0 {
		sig.HTTPStatus = observedStatus
	}
	// Code-keyed rows keep winning. The message match runs only where the code
	// table said nothing, which for a code-keyed document is exactly the
	// unnamed-code case — and those documents declare no message rows, so this
	// is a no-op for them and their behavior is byte-identical.
	if !known {
		if row, matched := c.messageRowFor(sig.HTTPStatus, message); matched {
			sig.Known, sig.DocumentedClass = true, row.DocumentedClass
		}
	}
	sig.ResetAt = c.parseResetAt(message)
	return sig, true
}

// hasMessageRows reports whether this lane's taxonomy is message-keyed at all.
func (c LaneConfig) hasMessageRows() bool {
	for _, row := range c.Signals {
		if row.MessageContains != "" {
			return true
		}
	}
	return false
}

// messageRowFor finds the documented row for one (status, message) pair.
//
// The match is case-insensitive on the substring and EXACT on the status,
// because the status is half the key: the same words on a different status are
// a different event on this wire, and matching across statuses would let a
// documented depletion exempt an undocumented auth failure.
//
// UNCLASSED ROWS WIN (P3-LN-3 drain r1). A real body is not one phrase, so more
// than one same-status row can match it — and a first-match-wins scan in
// DOCUMENT ORDER then decides a genuine revocation by where somebody happened to
// put a line in a JSON file. "Access terminated. Your account was suspended; you
// have also reached your usage limit for this billing cycle." matches both the
// 403 revocation row and the 403 depletion row; the depletion row is listed
// first, so the scan reported an exemption for an account suspension.
//
// A row carrying NO documented class is one that classifies on its status —
// which on 401/402/403 means it FREEZES. Preferring those is therefore the safe
// resolution of an ambiguous body, and it is where the tie-break belongs: at the
// document layer, before any verdict exists. It is a first line, not the net —
// the net is the scheduler's belt, which does not care which row won.
func (c LaneConfig) messageRowFor(status int, message string) (LaneSignalRow, bool) {
	if status <= 0 || message == "" {
		return LaneSignalRow{}, false
	}
	low := strings.ToLower(message)
	match := func(wantClassed bool) (LaneSignalRow, bool) {
		for _, row := range c.Signals {
			if row.MessageContains == "" || row.HTTPStatus != status {
				continue
			}
			if (row.DocumentedClass != "") != wantClassed {
				continue
			}
			if strings.Contains(low, strings.ToLower(row.MessageContains)) {
				return row, true
			}
		}
		return LaneSignalRow{}, false
	}
	if row, ok := match(false); ok {
		return row, true
	}
	return match(true)
}

// providerError is the verbatim provider error envelope.
type providerError struct {
	Error struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	} `json:"error"`
}

// maxErrorScanStarts bounds how many candidate JSON starts are tried inside a
// wrapped engine message, so a hostile blob cannot turn parsing into work.
const maxErrorScanStarts = 8

func decodeProviderError(text string) (code, message string, ok bool) {
	tries := 0
	for i := strings.Index(text, "{"); i >= 0; {
		if tries++; tries > maxErrorScanStarts {
			return "", "", false
		}
		var pe providerError
		// A Decoder stops at the end of the first complete JSON value, so
		// trailing engine prose after an embedded body is simply ignored.
		if err := json.NewDecoder(strings.NewReader(text[i:])).Decode(&pe); err == nil {
			if c, ok := decodeErrorCode(pe.Error.Code); ok {
				return c, pe.Error.Message, true
			}
		}
		next := strings.Index(text[i+1:], "{")
		if next < 0 {
			return "", "", false
		}
		i += 1 + next
	}
	return "", "", false
}

// decodeErrorCode accepts the documented STRING shape and tolerates a number:
// forward tolerance is a MUST, and the shape of a field is exactly the kind of
// thing that moves under a platform (S03.1).
func decodeErrorCode(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return s, true
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil && n.String() != "" {
		return n.String(), true
	}
	return "", false
}

// resetLayouts are the time shapes a provider resume stamp is accepted in.
// Parsing mechanics, not provider facts: an unparseable stamp yields no resume
// time at all, which classifies in the SAFE direction (a probe-park rather
// than a wait on a time nobody can read).
var resetLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04Z07:00",
	"2006-01-02 15:04",
}

// parseResetAt lifts the provider-signaled resume time out of a depletion
// message ("… Your limit will reset at `<next_flush_time>`").
func (c LaneConfig) parseResetAt(message string) time.Time {
	if c.ResetMarker == "" || message == "" {
		return time.Time{}
	}
	i := strings.Index(strings.ToLower(message), strings.ToLower(c.ResetMarker))
	if i < 0 {
		return time.Time{}
	}
	rest := strings.TrimSpace(message[i+len(c.ResetMarker):])
	rest = strings.Trim(rest, "`\"'")
	rest = strings.TrimRight(rest, ".` \"'")
	for _, layout := range resetLayouts {
		if t, err := time.Parse(layout, rest); err == nil {
			return t
		}
	}
	return time.Time{}
}
