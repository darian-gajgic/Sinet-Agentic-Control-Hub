package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// settings.go — the S15.9 settings family (P3-B6-3A): the one registry read
// (schema + UISchema + values + effective clamp bounds + the S18.4 R9 registered
// block), the per-setting audit history, the validated write verbs, and the
// S18.3 price-table surface.
//
// NOTHING HERE COMPUTES. The registry owns what a setting's effective value is,
// what its effective bounds are, what a write is allowed to do to it and what
// the audit row says; internal/metering owns the S10.3 row shape and every
// figure in it. This file resolves the caller's authority, hands the act to its
// owner, and serves what comes back. A second implementation of any of those
// rules is the drift S01.10's one-artifact-set mandate exists to prevent.
//
// AUTHORITY — settings writes are OPERATOR-ONLY (OQ8), and that is structural
// rather than a policy this transport chose. The registry's actor model admits
// exactly two kinds, `operator` and `automation` (settings.Actor.validate), and
// S18.1 calls the whole index operator-editable. There is no member actor a
// write could be attributed to, so a member cannot edit even a per-user key's
// own override — the operator administers those through `for_user`. Widening
// that is a change to the registry's actor model with its own audit semantics,
// not something a transport may do by constructing a different actor.
//
// AUTOMATION NEVER ENTERS HERE. Automation writes are in-process registry calls
// (the S12.10 swap gate is the landed example); this transport constructs an
// operator actor and only an operator actor, so the G1 rider 1 split — automation
// moves values within bounds, only the operator moves bounds — cannot be
// crossed by anything arriving over HTTP.

// PriceSurface is the S10.3 stored-price seam (migration 0019). The shell adapts
// the composed live table to it; nil leaves the price routes answering 503.
//
// Rows cross this seam as the metering package's OWN marshaled shape, in both
// directions. That is deliberate: S10 owns what a row is, what its fields mean
// and which of them are admissible, and a transport that re-declared the shape
// would be a second definition of the platform's money. Here a row is opaque.
type PriceSurface interface {
	// PriceRows returns the stored effective-dated rows verbatim.
	PriceRows(ctx context.Context) (json.RawMessage, error)
	// AddPriceRow appends one operator-authored row and re-composes the live
	// table, returning the stored row. The row body is validated by its owner;
	// a malformed one comes back as a *SurfaceError.
	AddPriceRow(ctx context.Context, actor, reason string, row json.RawMessage) (json.RawMessage, error)
	// TableVersion identifies the composed row set (S10.1's stamped version).
	TableVersion() string
}

// settingsHistoryCap bounds the per-setting audit read. It reuses the read
// surface's page default rather than inventing a bound: one read surface, one
// page size (the §38 precedent). Structural, not ⚙ — S18 ratifies no such key,
// interim under the standing settings-tab directive.
const settingsHistoryCap = readPageDefault

// ── GET /api/settings — the one registry read ───────────────────────────────

// SettingsView is the whole settings surface in one read: the two generated
// artifacts JSON Forms consumes (S15.9), the per-key values block, and the
// read-only registered block.
type SettingsView struct {
	// Schema and UISchema are the registry's own emissions, served verbatim as
	// embedded JSON. S01.10 (b): one artifact set drives validation, the UI and
	// the docs, so they cannot drift apart.
	Schema   json.RawMessage `json:"schema"`
	UISchema json.RawMessage `json:"uischema"`
	// Values is every declared key with its effective value, effective clamp
	// bounds, badges and help.
	Values []settings.Value `json:"values"`
	// Domains is the S18 domain list, in the order the UISchema tabs them.
	Domains []string `json:"domains"`
	// Registered is the S18.4 R9 read-only block: values frozen by a
	// registration, each carrying its own marker. Absent (with its reason) when
	// no practice is composed in this process.
	Registered       json.RawMessage `json:"registered,omitempty"`
	RegisteredAbsent string          `json:"registered_absent,omitempty"`
	// Editable states, on the response rather than only in the docs, whether
	// THIS caller may write here — so a surface renders a read-only view for a
	// member instead of offering controls that will 403.
	Editable       bool   `json:"editable"`
	EditableReason string `json:"editable_reason"`
}

func (s *Server) handleSettingsRead(w http.ResponseWriter, r *http.Request) {
	if !s.settingsReady(w) {
		return
	}
	schema, err := s.registry.JSONSchema()
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("emit settings schema: %w", err))
		return
	}
	ui, err := s.registry.UISchema()
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("emit settings uischema: %w", err))
		return
	}
	scope := s.readScope(r)
	out := SettingsView{
		Schema:   schema,
		UISchema: ui,
		Values:   scopeUserValues(s.registry.Values(), scope),
		Domains:  s.registry.Domains(),
	}
	out.Editable, out.EditableReason = s.settingsWriteAuthority(r)
	out.Registered, out.RegisteredAbsent = s.registeredBlock(r.Context())
	s.writeReadJSON(w, out)
}

// scopeUserValues narrows the per-user override maps to what the caller may
// read. The registry's declarations, defaults, bounds and help are the same for
// everybody — they are the platform's own configuration, not anybody's data —
// but a per-user override IS about a person, so a member sees only their own
// and the operator sees all (the §30 owner-scope shape).
func scopeUserValues(vals []settings.Value, scope ownerScope) []settings.Value {
	if scope.Operator {
		return vals
	}
	for i := range vals {
		if len(vals[i].UserValues) == 0 {
			continue
		}
		mine, ok := vals[i].UserValues[scope.UserID]
		if !ok {
			vals[i].UserValues = nil
			continue
		}
		vals[i].UserValues = map[string]any{scope.UserID: mine}
	}
	return vals
}

// registeredBlock reads the S18.4 R9 frozen values through the benchmark seam.
//
// NOT ONE OF THOSE NUMBERS IS NAMED IN internal/api. R9 says they "surface in
// the registry marked 'registered — changing this value requires a
// re-registration commit'", and the marker travels on each value from the
// package that owns it, so a display can never disagree with the machinery
// (the §39-C source-scan discipline). Engine version pins are NOT served here:
// R9's own words give them the manifest home, `components.lock`.
func (s *Server) registeredBlock(ctx context.Context) (json.RawMessage, string) {
	if s.benchmark == nil {
		return nil, "no benchmark practice is composed in this process, so the registered-value block has nothing to read (S18.4 R9)"
	}
	raw, err := s.benchmark.RegisteredValues(ctx)
	if err != nil {
		return nil, "the registered-value block could not be read: " + err.Error()
	}
	return raw, ""
}

// ── GET /api/settings/{key}/history — the per-setting audit trail ───────────

// SettingsHistory is one key's audit rows, newest first.
type SettingsHistory struct {
	Key     string                `json:"key"`
	Entries []settings.AuditEntry `json:"entries"`
	Limit   int                   `json:"limit"`
}

func (s *Server) handleSettingsHistory(w http.ResponseWriter, r *http.Request) {
	if !s.settingsReady(w) {
		return
	}
	key := r.PathValue("key")
	if _, ok := s.registry.Decl(key); !ok {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusNotFound, Code: "not_found",
			Msg: fmt.Sprintf("no setting %q is declared (S18 index)", key)})
		return
	}
	limit, err := readLimit(r)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if limit > settingsHistoryCap {
		limit = settingsHistoryCap
	}
	entries, err := s.registry.History(r.Context(), key, limit)
	if err != nil {
		s.writeSurface(w, nil, s.settingsErr(err))
		return
	}
	// Audit rows are CONTENT, not an observability projection: the old and new
	// values go out exactly as the registry stored them (§38 ruling (b)).
	s.writeReadJSON(w, SettingsHistory{Key: key, Entries: entries, Limit: limit})
}

// ── POST /api/settings/{key} — validated set / reset ────────────────────────

// settingsSetBody is one value write. A null (or absent) `value` is
// reset-to-default, which DELETES the override row (S01.10) — so "reset" needs
// no verb of its own and cannot drift from what the registry means by it.
type settingsSetBody struct {
	Value   json.RawMessage `json:"value"`
	ForUser string          `json:"for_user,omitempty"`
	Reason  string          `json:"reason,omitempty"`
}

// SettingsWritten is the write's outcome: the key as it now reads.
type SettingsWritten struct {
	Key   string         `json:"key"`
	Reset bool           `json:"reset"`
	Value settings.Value `json:"value"`
	// ForUser names the per-user scope the write targeted, empty for platform.
	ForUser string `json:"for_user,omitempty"`
	Detail  string `json:"detail"`
}

func (s *Server) handleSettingsSet(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireSettingsOperator(w, r)
	if !ok {
		return
	}
	key := r.PathValue("key")
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body settingsSetBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	reset := len(body.Value) == 0 || string(body.Value) == "null"
	req := settings.SetRequest{
		Key:     key,
		ForUser: strings.TrimSpace(body.ForUser),
		Actor:   settings.Actor{Kind: settings.ActorOperator, ID: actor},
		Reason:  body.Reason,
	}
	if !reset {
		req.Value = body.Value
	}
	if err := s.registry.Set(r.Context(), req); err != nil {
		s.writeSurface(w, nil, s.settingsErr(err))
		return
	}
	// The write already landed {override cell + settings_events row +
	// settings.changed event} in ONE transaction inside the registry. Nothing
	// is minted here: an act with a landed canonical audit row is never
	// double-minted (§39 OQ8).
	out := SettingsWritten{Key: key, Reset: reset, ForUser: req.ForUser}
	if v, ok := s.valueOf(key); ok {
		out.Value = v
	}
	if reset {
		out.Detail = "reset to the ratified default: the override row is deleted, so the setting follows the default from here (S01.10)"
	} else {
		out.Detail = "set: the new value is in force for the next read — consumers read the registry by key, so nothing restarts"
		if d, ok := s.registry.Decl(key); ok && d.Restart {
			out.Detail = "set and recorded, but this setting is restart-required: the stored value is the new one and the RUNNING process keeps the old until it restarts (S01.10)"
		}
	}
	s.writeReadJSON(w, out)
}

// ── POST /api/settings/{key}/bounds — the operator-owned bounds edit ────────

// settingsBoundsBody is one bounds edit. Both members null resets the bounds to
// the ratified clamp.
type settingsBoundsBody struct {
	Floor   *float64 `json:"floor"`
	Ceiling *float64 `json:"ceiling"`
	Reason  string   `json:"reason,omitempty"`
}

func (s *Server) handleSettingsBounds(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireSettingsOperator(w, r)
	if !ok {
		return
	}
	key := r.PathValue("key")
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body settingsBoundsBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	err := s.registry.SetBounds(r.Context(), settings.SetBoundsRequest{
		Key: key, Floor: body.Floor, Ceiling: body.Ceiling,
		Actor:  settings.Actor{Kind: settings.ActorOperator, ID: actor},
		Reason: body.Reason,
	})
	if err != nil {
		s.writeSurface(w, nil, s.settingsErr(err))
		return
	}
	out := SettingsWritten{Key: key, Reset: body.Floor == nil && body.Ceiling == nil,
		Detail: "bounds recorded: automation may move this value only inside them, and only the operator moves them (G1 rider 1)"}
	if v, ok := s.valueOf(key); ok {
		out.Value = v
	}
	s.writeReadJSON(w, out)
}

// ── the S18.3 price-table surface ───────────────────────────────────────────

// StoredPricesView is the stored price table as the settings surface reads it.
//
// The type is named for what it CARRIES rather than for the metering construct
// it comes from, because the §38 R8 money scan bans that construct's name in
// this package outright — internal/api reads figures, it never reaches the
// layer that computes them.
type StoredPricesView struct {
	// Rows are the stored effective-dated rows, verbatim from their owner.
	Rows json.RawMessage `json:"rows"`
	// Version identifies the composed row set (S10.1).
	Version string `json:"version"`
	// Posture states what an EMPTY table means, because the honest answer to
	// "why does nothing have a price?" is a sentence, not an empty array.
	Posture string `json:"posture,omitempty"`
	// Deferred names the S18.3 surfaces this one does NOT carry, with the
	// reason, so their absence is a stated decision rather than a gap.
	Deferred []string `json:"deferred"`
	Editable bool     `json:"editable"`
}

// priceRowsEmpty reports whether the served rows are the empty set. It reads the
// marshaled form because the rows are opaque here by design.
func priceRowsEmpty(raw json.RawMessage) bool {
	t := strings.TrimSpace(string(raw))
	return t == "" || t == "null" || t == "[]"
}

// emptyTablePosture is the ratified v0 reading of an empty table, served as
// data. An empty table prices every usage row UNPRICED — never a silent $0
// (S10.1; P-T08-1) — and the platform ships that way on purpose.
const emptyTablePosture = "this table is EMPTY, which is the shipped posture, not a fault: with no row for a {model, lane} at a date, " +
	"every usage row prices UNPRICED and says so on the receipt, rather than silently pricing to $0 (S10.1). " +
	"Filling it is the operator's act — a refresh from the vendored provider data lands as a PROPOSAL and is never applied automatically (G2 Def.16)."

// deferredPriceSurfaces names the two S18.3 rows this packet did not build, each
// with the reason. Their consumer does not exist at v0 — the metered-exception
// list is ratified EMPTY (G1 P7) — and a flag whose consumer is absent is a
// capability the surface would be claiming to have.
var deferredPriceSurfaces = []string{
	"per-model flat/metered flag (S18.3; S10.2) — deferred: the metered-exception list is ratified EMPTY at v0 (G1 P7), so the flag has no consumer to change the behavior of",
	"per-model overflow_mode and region_model_gate (S18.3; S03.6) — deferred: their machinery is absent at v0, and a stored setting nothing reads is a dial connected to nothing",
}

func (s *Server) handlePriceRead(w http.ResponseWriter, r *http.Request) {
	if !s.pricesReady(w) {
		return
	}
	rows, err := s.prices.PriceRows(r.Context())
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	editable, _ := s.settingsWriteAuthority(r)
	out := StoredPricesView{
		Rows: rows, Version: s.prices.TableVersion(),
		Deferred: deferredPriceSurfaces, Editable: editable,
	}
	if priceRowsEmpty(rows) {
		out.Posture = emptyTablePosture
	}
	s.writeReadJSON(w, out)
}

// priceAddBody wraps the row. The row itself is passed through untouched: its
// shape belongs to S10.3 and is validated by its owner (see PriceSurface).
type priceAddBody struct {
	Row    json.RawMessage `json:"row"`
	Reason string          `json:"reason,omitempty"`
}

// PriceRowAdded is the append's outcome.
type PriceRowAdded struct {
	Row json.RawMessage `json:"row"`
	// Version is the table version AFTER the append — the figure S10.1 stamps
	// on a priced cost and the input to the S02.6 freshness fingerprint, which
	// is why a change here re-validates parked plans (S10.3; G1 Def.5).
	Version string `json:"version"`
	Detail  string `json:"detail"`
}

func (s *Server) handlePriceAdd(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireSettingsOperator(w, r)
	if !ok {
		return
	}
	if !s.pricesReady(w) {
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body priceAddBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	if len(body.Row) == 0 {
		s.writeSurface(w, nil, badRequest(`missing "row": a price-table edit adds one effective-dated row (S10.3)`))
		return
	}
	stored, err := s.prices.AddPriceRow(r.Context(), actor, body.Reason, body.Row)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	s.writeReadJSON(w, PriceRowAdded{
		Row: stored, Version: s.prices.TableVersion(),
		Detail: "row appended and in force: the table re-composed in place, so the next priced call uses it without a restart. " +
			"Earlier rows are untouched — an effective-dated append never rewrites what a past receipt was charged at (S10.3).",
	})
}

// ── shared authority + readiness ────────────────────────────────────────────

// settingsReady reports the registry handle, or writes the not-wired answer.
func (s *Server) settingsReady(w http.ResponseWriter) bool {
	if s.registry == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the settings registry is not wired in this process"})
		return false
	}
	return true
}

func (s *Server) pricesReady(w http.ResponseWriter) bool {
	if s.prices == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the stored price table is not wired in this process"})
		return false
	}
	return true
}

// settingsWriteAuthority answers "may this caller write here, and why not".
//
// Two conditions, each with its own reason. The operator role bit is the
// registry's actor model showing through (OQ8). The dev-identity refusal is
// §9's standing rule: the dev-posture fallback identity is resolution-only —
// it reads, and every admin verb refuses it — so a dev process cannot mutate
// the platform's configuration just by having no login.
func (s *Server) settingsWriteAuthority(r *http.Request) (bool, string) {
	id, _ := IdentityFrom(r.Context())
	if id.Dev {
		return false, "the dev-posture identity reads but never administers (§9): settings writes need a real operator session"
	}
	if !s.readScope(r).Operator {
		return false, "settings are operator-edited (S18.1): the registry records an operator or an automation actor and admits no member kind, so a member's own per-user override is set for them by the operator (D10)"
	}
	return true, "operator: every write lands its clamp check, its audit row and its settings.changed event in one transaction (S01.10)"
}

// requireSettingsOperator resolves the writing actor or writes the refusal.
func (s *Server) requireSettingsOperator(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !s.settingsReady(w) {
		return "", false
	}
	if ok, why := s.settingsWriteAuthority(r); !ok {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusForbidden, Code: "forbidden", Msg: why})
		return "", false
	}
	id, _ := IdentityFrom(r.Context())
	return id.UserID, true
}

// valueOf reads one key back after a write, so the response says what the
// setting now IS rather than what the request asked for.
func (s *Server) valueOf(key string) (settings.Value, bool) {
	for _, v := range s.registry.Values() {
		if v.Key == key {
			return v, true
		}
	}
	return settings.Value{}, false
}

// settingsErr maps the registry's own refusals to statuses ON THE ERROR'S TYPE,
// never on its message text.
//
// The distinction the registry exports is caller-fault vs platform failure, and
// it matters here because EVERY error that package returns is prefixed
// "settings: " — including the ones wrapping a storage or event-log failure
// mid-write. Classifying by that prefix answered 400 "bad_request" to a caller
// whose request was fine and whose write half-failed, which is the same defect
// shape the §39 drain fixed on the follow-up path and the reason §38 bans
// error-string matching (drain D1).
//
// A caller fault carries the registry's own message: it knows why it refused,
// and a paraphrase would be a second, worse explanation. An UNMARKED error is a
// platform fault — the safe default — and answers a generic 500 with the real
// cause in the ops log, because the caller cannot act on it and it may name
// platform internals.
func (s *Server) settingsErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, settings.ErrUnknownKey):
		return &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: err.Error()}
	case errors.Is(err, settings.ErrNotAttached):
		return &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired", Msg: err.Error()}
	case errors.Is(err, settings.ErrOutOfBounds),
		errors.Is(err, settings.ErrAutomation),
		errors.Is(err, settings.ErrInvalidRequest):
		return &SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	}
	s.logger.Error("settings: the registry refused a write for a reason it did not class as the caller's", "err", err)
	return &SurfaceError{Status: http.StatusInternalServerError, Code: "internal",
		Msg: "the settings write did not complete"}
}
