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
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

//go:embed lanedata/zai.json
var zaiLaneSeed []byte

// ErrLaneConfig reports a lane document that cannot be commissioned.
var ErrLaneConfig = errors.New("opencode: lane configuration")

// LaneConfig is one lane's commissioning document.
type LaneConfig struct {
	// Lane is the platform lane name (runs.lane); ProviderID is the provider
	// key inside opencode's own config, and the first half of the platform
	// model string `<providerID>/<modelID>`.
	Lane        string `json:"lane"`
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
	ResetMarker string          `json:"reset_marker"`
	Signals     []LaneSignalRow `json:"signals"`
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
	Note       string   `json:"note"`
}

// LaneCredential names the broker auth-profile and the environment variable
// its spawn-time injection fills. A NAME and a variable name — never material
// (S11.5: workers hold auth-profile references, never raw secrets).
type LaneCredential struct {
	Profile string `json:"profile"`
	EnvVar  string `json:"env_var"`
	Note    string `json:"note"`
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
	// SemanticsVerified is false for a band the provider documents only as a
	// group; the row is honest about what it does not know.
	SemanticsVerified bool   `json:"semantics_verified"`
	Note              string `json:"note"`
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
	c, err := LoadLaneConfig(zaiLaneSeed)
	if err != nil {
		return nil, err
	}
	return []LaneConfig{c}, nil
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
func (c LaneConfig) ExtractSignal(text string, observedStatus int) (LaneSignal, bool) {
	code, message, ok := decodeProviderError(text)
	if !ok {
		return LaneSignal{}, false
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
	sig.ResetAt = c.parseResetAt(message)
	return sig, true
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
