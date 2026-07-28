package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/benchmark"
)

// benchmark_verbs.go — the composition-root adapter behind the B6-2C verdict
// backend (api.BenchmarkSurface).
//
// It exists because internal/api imports internal/benchmark in NEITHER direction
// (the reverse wall internal/benchmark's importwall_test asserts) and the shell
// is the one place allowed to see both vocabularies. Its whole job is two
// translations and nothing else:
//
//  1. STRINGS → the package's own constructors. The verdict and the arm-guess
//     arrive as the caller typed them and go straight into benchmark.NewAnswer,
//     which is where the §3.3 "no verdict without a guess" rule is STRUCTURAL:
//     Answer's fields are unexported and this is its only constructor, so a
//     guess-less verdict is not something a transport could choose to allow.
//     No registered value is restated on this path either — the four verdict
//     values and the §12 disposition vocabulary are checked by the package
//     against the registration file.
//
//  2. The package's SENTINELS → transport statuses. A refusal on its merits is
//     an answer, not a platform fault, so each sentinel maps to the status that
//     tells the caller what to do next.
//
// It shapes nothing: every read is the package's own readout struct marshaled
// verbatim, so the blindness properties those structs carry (PendingPair has no
// arm, no position, no run and no model) travel to the wire intact.

// benchmarkVerbs adapts the composed S14.7 practice to api.BenchmarkSurface.
type benchmarkVerbs struct{ bs *benchmarkSurface }

// benchmarkVerbSeam returns the api seam, or a true nil when the practice is not
// composed in this process (injected-admission and dev postures) — which leaves
// every benchmark route answering 503 rather than pretending a practice runs.
func benchmarkVerbSeam(bs *benchmarkSurface) api.BenchmarkSurface {
	if bs == nil || bs.Practice == nil || bs.Store == nil {
		return nil
	}
	return benchmarkVerbs{bs: bs}
}

func (b benchmarkVerbs) PendingVerdicts(ctx context.Context, requester string) (json.RawMessage, error) {
	pairs, err := b.bs.Store.PendingVerdicts(ctx, requester)
	if err != nil {
		return nil, benchmarkStatus(err)
	}
	raw, err := json.Marshal(pairs)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// RecordVerdict is the §3.3 ONE ACT. NewAnswer is called here rather than in the
// transport because it is the structural gate: a caller who omits the guess gets
// ErrGuessRequired from the constructor, and no code path exists that could build
// the argument without one.
func (b benchmarkVerbs) RecordVerdict(ctx context.Context, pairID, verdict, guess string) error {
	answer, err := benchmark.NewAnswer(benchmark.Choice(verdict), benchmark.Side(guess))
	if err != nil {
		// The constructor has exactly two failure modes and both are malformed
		// ARGUMENTS: a verdict outside the four registered values, and a missing
		// or invalid arm-guess. Neither is a platform fault, so both answer 400 —
		// and the guess gets its own machine code, because "you left the
		// mandatory guess out" is the one a form has to render differently.
		code := "bad_verdict"
		if errors.Is(err, benchmark.ErrGuessRequired) {
			code = "guess_required"
		}
		return &api.SurfaceError{Status: http.StatusBadRequest, Code: code, Msg: err.Error()}
	}
	if _, err := b.bs.Practice.RecordVerdict(ctx, pairID, answer); err != nil {
		return benchmarkStatus(err)
	}
	return nil
}

// Reveal reads the COMMITTED §14 record. It is a separate call from RecordVerdict
// on purpose: §3.4's order is "record, then reveal", and two calls make that
// order something the transport can be tested on.
func (b benchmarkVerbs) Reveal(ctx context.Context, pairID string) (json.RawMessage, error) {
	rec, err := b.bs.Practice.Reveal(ctx, pairID)
	if err != nil {
		return nil, benchmarkStatus(err)
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (b benchmarkVerbs) Decline(ctx context.Context, pairID string) error {
	if _, err := b.bs.Practice.Decline(ctx, pairID); err != nil {
		return benchmarkStatus(err)
	}
	return nil
}

func (b benchmarkVerbs) DisposeAlarm(ctx context.Context, actor, domain, disposition, reason string) error {
	return benchmarkStatus(b.bs.Practice.DisposeAlarm(ctx, actor, domain, disposition, reason))
}

func (b benchmarkVerbs) SetOptIn(ctx context.Context, actor, subject string, enabled bool) error {
	return benchmarkStatus(b.bs.Practice.SetOptIn(ctx, actor, subject, enabled))
}

// RegisteredValues is the S18.4 R9 block for the B6-3A settings surface: the
// package's OWN marked values, marshaled and passed straight through. The ctx is
// unused because there is nothing to read from storage — the values are frozen
// in code and a registration is not a query — and the signature carries it so
// the seam looks like every other read on it.
func (b benchmarkVerbs) RegisteredValues(context.Context) (json.RawMessage, error) {
	raw, err := json.Marshal(benchmark.RegisteredValues())
	if err != nil {
		return nil, fmt.Errorf("shell: marshal registered values: %w", err)
	}
	return raw, nil
}

// benchmarkStatus maps the package's sentinels onto transport statuses. Anything
// unrecognized passes through untouched and becomes a 500, which is the right
// default: an error nobody classified is a platform fault until somebody says
// otherwise.
//
// ErrBadInput lands on 400 and the package's own message travels with it. The
// two authority refusals it also carries — the §12 operator check and the §4.2.1
// self-only consent rule — are refused at the TRANSPORT first (403 there), so
// reaching them here means the request got past a gate that should have stopped
// it; the package's refusal is the belt, and it is deliberately not the only
// statement of either rule.
func benchmarkStatus(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, benchmark.ErrGuessRequired):
		return &api.SurfaceError{Status: http.StatusBadRequest, Code: "guess_required", Msg: err.Error()}
	case errors.Is(err, benchmark.ErrUnknownPair):
		return &api.SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: err.Error()}
	case errors.Is(err, benchmark.ErrUnregisteredDomain):
		return &api.SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: err.Error()}
	case errors.Is(err, benchmark.ErrPairState):
		return &api.SurfaceError{Status: http.StatusConflict, Code: "conflict", Msg: err.Error()}
	case errors.Is(err, benchmark.ErrNoAlarm):
		return &api.SurfaceError{Status: http.StatusConflict, Code: "conflict", Msg: err.Error()}
	case errors.Is(err, benchmark.ErrNoDirectCapture):
		return &api.SurfaceError{Status: http.StatusConflict, Code: "conflict", Msg: err.Error()}
	case errors.Is(err, benchmark.ErrBadInput):
		return &api.SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	}
	return err
}
