package local

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
)

// gpubroker.go — the Spec S12.6 GPU BROKER: the control-plane-mediated
// local-inference DATA plane, in TWO caller planes (brief R1–R5/R17).
//
// TERM DISCIPLINE (S12.6, absolute): this is the "GPU broker" — a DISTINCT
// component from the credential BROKER (internal/broker, sinet-broker.service,
// the S00-glossary broker). The GPU broker's per-run token mints at the SAME
// spawn point where the credential broker's CredInject fires, but it is a
// different mechanism. The distinctness is a CODE FACT: this package NEVER
// imports internal/broker (AST-pinned, importwall_test.go) — the GPU broker
// EXTENDS internal/local, not the credential broker. No proxy product is
// adopted (virtual-key PATTERN only) → zero new components.lock entries.
//
// The two planes (S12.6):
//   - PLATFORM plane (B4-5, client.go): sinet-control + adopted watch organs
//     reach llama-swap directly on loopback. ALL management verbs (load/unload/
//     config) exist ONLY here (R2).
//   - SANDBOXED plane (this file): a worker granted local inference reaches
//     EXACTLY POST /v1/chat/completions + POST /v1/embeddings through a per-
//     sandbox loopback/unix bridge; /v1/models returns ONLY the run's
//     allowlist; NO management verb, NO /dev/nvidia*, NO routable host address
//     (R4). Identity is a per-run bearer token minted at spawn (R3); logprobs
//     are excluded unless the run's template flips ⚙ local.broker.sandbox_logprobs
//     (R5, side-channel reduction).
//
// SEAM (BINDING): S12.1 class (a) — a run executing ON a local model via the
// opencode adapter — has NO v0 consumer (no second engine adapter this cut).
// So NOTHING mints a token from a real spawn and the sandboxed plane has no
// live consumer: it is delivered as a designed, HERMETICALLY-TESTED seam, never
// a live-exercised path (faking a consumer would violate the BINDING reading +
// CONVENTIONS §19). Every sandboxed call writes ONE $0 D7 row keyed to the
// SANDBOXED run (token→run→owner) reusing the B4-5 usage.go wire contract (R17).

// SandboxGrant is a per-run bridge grant (S12.6/R3). Expiry = run lifetime; the
// registry is in-memory (no durable table — R21 NO migration). The class-derived
// rate/size/logprobs budgets are decided at the MINT site by the confinement
// class ladder (internal/sandbox owns LadderRank — the C0-never/C1-grant/C3-
// tighter policy, R6; this package holds only the resulting values, never
// re-deriving the ladder across the import wall).
type SandboxGrant struct {
	RunID string // the sandboxed run (token→run→owner, R3/R17)
	Owner string // the run's owner (15.6 attribution)
	Class string // the compiled confinement class ("C0".."C3"); C0 is never granted (R6)
	// ModelAllowlist is the run's allowed duty ALIASES — the `model` field of a
	// sandboxed call must name one; /v1/models returns exactly this (R3/R4).
	ModelAllowlist []string
	// RateLimitPerMin / MaxRequestBytes are the class-derived budgets (R3/R6).
	RateLimitPerMin int
	MaxRequestBytes int64
	// LogprobsAllowed is the per-template ⚙ local.broker.sandbox_logprobs value
	// captured at mint; the handler ALSO re-reads the live ⚙ (both must be on).
	LogprobsAllowed bool
	ExpiresAt       time.Time // zero = run-lifetime (revoked explicitly on run end)
}

func (g SandboxGrant) allows(alias string) bool {
	a := normalizeAlias(alias)
	for _, m := range g.ModelAllowlist {
		if normalizeAlias(m) == a {
			return true
		}
	}
	return false
}

// tokenState is a live grant plus its fixed-window rate counter.
type tokenState struct {
	grant       SandboxGrant
	windowStart time.Time
	windowCount int
}

// TokenRegistry mints and resolves per-run bearer tokens (S12.6/R3). In-memory,
// run-lifetime (R21). A token is a 256-bit random hex string; it resolves to
// its grant → run → owner. Safe for concurrent use.
type TokenRegistry struct {
	mu     sync.Mutex
	tokens map[string]*tokenState
	now    func() time.Time
}

// NewTokenRegistry returns an empty registry.
func NewTokenRegistry() *TokenRegistry {
	return &TokenRegistry{tokens: map[string]*tokenState{}, now: time.Now}
}

// Mint issues a per-run bearer token for a grant (S12.6/R3). C0 grants are
// REFUSED — C0 connectors never get the bridge (R6). The token expiry is the
// run lifetime (RevokeRun clears it).
func (tr *TokenRegistry) Mint(grant SandboxGrant) (string, error) {
	if strings.EqualFold(grant.Class, "C0") {
		return "", fmt.Errorf("local: C0 connectors are never granted the GPU bridge (S12.6/S11.6)")
	}
	if grant.RunID == "" {
		return "", fmt.Errorf("local: a sandbox grant needs a run (token→run→owner, R3)")
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("local: mint token: %w", err)
	}
	token := hex.EncodeToString(raw[:])
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.tokens[token] = &tokenState{grant: grant, windowStart: tr.now()}
	return token, nil
}

// Resolve maps a token to its grant (token→run→owner, R3). ok=false for an
// absent/expired token — the broker never serves it (ErrTokenInvalid).
func (tr *TokenRegistry) Resolve(token string) (SandboxGrant, bool) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	st, ok := tr.tokens[token]
	if !ok {
		return SandboxGrant{}, false
	}
	if !st.grant.ExpiresAt.IsZero() && tr.now().After(st.grant.ExpiresAt) {
		delete(tr.tokens, token)
		return SandboxGrant{}, false
	}
	return st.grant, true
}

// RevokeRun revokes every token for a run (run end — expiry = run lifetime, R3).
func (tr *TokenRegistry) RevokeRun(runID string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	for tok, st := range tr.tokens {
		if st.grant.RunID == runID {
			delete(tr.tokens, tok)
		}
	}
}

// rateOK applies the grant's fixed-window per-minute rate limit (R3). A zero
// limit disables the check.
func (tr *TokenRegistry) rateOK(token string) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	st, ok := tr.tokens[token]
	if !ok {
		return false
	}
	if st.grant.RateLimitPerMin <= 0 {
		return true
	}
	now := tr.now()
	if now.Sub(st.windowStart) >= time.Minute {
		st.windowStart = now
		st.windowCount = 0
	}
	st.windowCount++
	return st.windowCount <= st.grant.RateLimitPerMin
}

// SandboxBroker is the sandboxed-plane handler (S12.6/R3–R5/R17). It resolves a
// per-run token, enforces the allowlist + logprobs gate + size/rate budgets,
// resolves the duty ALIAS to a seat exactly as the platform plane does
// (Registry.SeatFor), forwards the shaped call to the loopback platform client,
// and writes ONE $0 D7 row on the SANDBOXED run (R17). It exposes EXACTLY the
// R4 routes — management verbs are absent by construction (a code + test fact).
type SandboxBroker struct {
	tokens   *TokenRegistry
	reg      *Registry
	client   *Client
	cps      *gates.Checkpoints
	log      *eventlog.Log
	settings Settings
	logger   *slog.Logger
}

// SandboxBrokerDeps wires the handler over the platform-plane stores.
type SandboxBrokerDeps struct {
	Tokens      *TokenRegistry
	Registry    *Registry
	Client      *Client
	Checkpoints *gates.Checkpoints
	Events      *eventlog.Log
	Settings    Settings
	Logger      *slog.Logger
}

// NewSandboxBroker wires the sandboxed-plane handler (R3).
func NewSandboxBroker(d SandboxBrokerDeps) *SandboxBroker {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	tokens := d.Tokens
	if tokens == nil {
		tokens = NewTokenRegistry()
	}
	return &SandboxBroker{
		tokens: tokens, reg: d.Registry, client: d.Client, cps: d.Checkpoints,
		log: d.Events, settings: d.Settings, logger: logger,
	}
}

// Tokens returns the broker's token registry (the mint site wires grants here).
func (b *SandboxBroker) Tokens() *TokenRegistry { return b.tokens }

// Handler is the sandboxed-plane HTTP surface (S12.6/R4). EXACTLY:
// POST /v1/chat/completions, POST /v1/embeddings, GET /v1/models. Every other
// path — including every /api/* management verb — is 404 by construction (R2).
func (b *SandboxBroker) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", b.handleChat)
	mux.HandleFunc("POST /v1/embeddings", b.handleEmbeddings)
	mux.HandleFunc("GET /v1/models", b.handleModels)
	return mux
}

// bearer is the shared token-validated dispatch path (F4): it parses the
// Authorization header STRICTLY as the RFC 6750 "Bearer <token>" scheme (the
// scheme matched case-insensitively; a bare or glued token is rejected),
// resolves the grant (token→run→owner), and enforces the grant's per-plane rate
// limit so EVERY token-bearing route counts against the budget — not just
// /v1/chat/completions. Writes 401/429 and returns ok=false on failure.
func (b *SandboxBroker) bearer(w http.ResponseWriter, r *http.Request) (SandboxGrant, bool) {
	scheme, tok, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		writeOpenAIError(w, http.StatusUnauthorized, ErrTokenInvalid.Error())
		return SandboxGrant{}, false
	}
	tok = strings.TrimSpace(tok)
	grant, ok := b.tokens.Resolve(tok)
	if !ok {
		writeOpenAIError(w, http.StatusUnauthorized, ErrTokenInvalid.Error())
		return SandboxGrant{}, false
	}
	if !b.tokens.rateOK(tok) {
		writeOpenAIError(w, http.StatusTooManyRequests, "rate limit exceeded for this run's grant (S12.6)")
		return SandboxGrant{}, false
	}
	return grant, true
}

type sandboxChatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Logprobs bool          `json:"logprobs"`
	MaxTok   int64         `json:"max_tokens"`
}

func (b *SandboxBroker) handleChat(w http.ResponseWriter, r *http.Request) {
	grant, ok := b.bearer(w, r)
	if !ok {
		return
	}
	limit := grant.MaxRequestBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "read request")
		return
	}
	if int64(len(raw)) > limit {
		writeOpenAIError(w, http.StatusRequestEntityTooLarge, "request exceeds the run's grant size budget (S12.6)")
		return
	}
	var req sandboxChatRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	// The `model` field carries a duty ALIAS, never a raw model id (R3/R4).
	if !grant.allows(req.Model) {
		writeOpenAIError(w, http.StatusForbidden, fmt.Sprintf("%v: %q", ErrModelNotAllowed, req.Model))
		return
	}
	// Logprobs are refused for sandboxed callers unless BOTH the run's grant and
	// the live ⚙ allow them (R5 side-channel reduction).
	if req.Logprobs {
		liveOn, err := b.settings.Bool(keySandboxLogprobs)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, "read ⚙")
			return
		}
		if !grant.LogprobsAllowed || !liveOn {
			writeOpenAIError(w, http.StatusForbidden, ErrLogprobsRefused.Error())
			return
		}
	}
	seat, err := b.reg.SeatFor(req.Model)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error()) // embedder post-gate / unknown / no seat
		return
	}
	if !seat.Servable {
		writeOpenAIError(w, http.StatusServiceUnavailable, fmt.Sprintf("%v: %q — %s", ErrNotServable, seat.Model, seat.Note))
		return
	}

	comp, err := b.client.Chat(r.Context(), ChatSpec{
		Model:     seat.Model,
		Messages:  req.Messages,
		MaxTokens: req.MaxTok,
		// The sandboxed plane never requests logprobs from the engine (side-
		// channel reduction) unless the gate above passed.
		Logprobs: req.Logprobs,
	})
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, err.Error())
		return
	}

	// ONE $0 D7 row keyed to the SANDBOXED run (token→run→owner), reusing the
	// B4-5 usage.go wire contract — a new WRITER of the same contract, NO
	// metering-package change (R17). The marker's duty is the REQUESTED,
	// allowlist-validated alias (req.Model) — never the grant's first alias —
	// so the (duty, model) pair stays internally consistent (the platform
	// plane's duty.go posture; F1).
	if err := b.meter(r.Context(), grant, req.Model, seat, comp); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "metering write failed")
		return
	}

	resp := map[string]any{
		"choices": []map[string]any{{
			"message":       map[string]any{"role": "assistant", "content": comp.Content},
			"finish_reason": comp.FinishReason,
		}},
		"usage": map[string]any{"prompt_tokens": comp.InputTokens, "completion_tokens": comp.OutputTokens},
	}
	if req.Logprobs && len(comp.Logprobs) > 0 {
		resp["choices"].([]map[string]any)[0]["logprobs"] = map[string]any{"content": comp.Logprobs}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleEmbeddings serves the R4 route for the embedder seat. The embedder is
// POST-GATE (G2 Def.8 not opened): the alias refuses with ErrEmbedderPostGate
// exactly as the platform plane does — the ROUTE exists, the seat does not
// serve (R4).
func (b *SandboxBroker) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	if _, ok := b.bearer(w, r); !ok {
		return
	}
	writeOpenAIError(w, http.StatusServiceUnavailable, ErrEmbedderPostGate.Error())
}

// handleModels returns ONLY the run's allowlist (R4) — never the full model set.
func (b *SandboxBroker) handleModels(w http.ResponseWriter, r *http.Request) {
	grant, ok := b.bearer(w, r)
	if !ok {
		return
	}
	data := make([]map[string]any, 0, len(grant.ModelAllowlist))
	for _, alias := range grant.ModelAllowlist {
		data = append(data, map[string]any{"id": alias, "object": "model"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (b *SandboxBroker) meter(ctx context.Context, grant SandboxGrant, alias string, seat SeatRecord, comp Completion) error {
	usage, err := UsageJSON(alias, seat.Model, seat.ModelHash(), LlamaCppPin, comp.InputTokens, comp.OutputTokens)
	if err != nil {
		return err
	}
	_, err = b.cps.Write(ctx, gates.NewCheckpoint{
		RunID:            grant.RunID,
		Usage:            usage,
		SessionSubstrate: "local",
		ModelID:          seat.Model,
	})
	if err != nil {
		b.surfaceUnmeteredDefect(ctx, grant.RunID, err)
	}
	return err
}

func (b *SandboxBroker) surfaceUnmeteredDefect(ctx context.Context, runID string, cause error) {
	b.logger.Error("local: UNMETERED sandboxed GPU-broker call — could not write its mandatory D7 row (R17/S12.1)",
		"run", runID, "cause", cause)
	if b.log == nil {
		return
	}
	payload, _ := json.Marshal(struct {
		Run   string `json:"run"`
		Plane string `json:"plane"`
		Cause string `json:"cause"`
	}{runID, "sandboxed", cause.Error()})
	_, _ = b.log.Append(ctx, eventlog.Append{
		UserID: "platform", Type: EventLocalUnmeteredDefect, SchemaVersion: dutyEventSchemaVersion, Payload: payload,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOpenAIError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": msg}})
}
