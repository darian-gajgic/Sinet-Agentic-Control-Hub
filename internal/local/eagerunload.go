package local

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// eagerunload.go — the S12.2 / G2 Def.6 ratified operator-wins switch (brief
// R12): one control-plane action that (1) STOPS local-lane admissions (the
// duty client then refuses new calls with the eager-unload reason — every
// consumer degrades per its S12.4 row; no local-lane RUNS exist in this cut)
// and (2) calls llama-swap's unload endpoint (POST /api/models/unload). Plus
// the symmetric resume. Both idempotent, event-recorded (owner-attributed,
// refs-not-blobs). Surfaced as a control-plane surface act (card DATA for B6,
// the buildAcceptSurface precedent) AND a `sinet` CLI tool subcommand. All
// management verbs live on the platform plane, loopback (S12.6).

// Admissions is the local-lane admission gate the eager-unload switch flips.
// Stopped ⇒ the duty client refuses new calls (ErrAdmissionsStopped).
type Admissions struct {
	mu      sync.RWMutex
	stopped bool
	reason  string
}

// NewAdmissions returns an open admission gate.
func NewAdmissions() *Admissions { return &Admissions{} }

// Stopped reports whether local-lane admissions are stopped.
func (a *Admissions) Stopped() bool {
	if a == nil {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.stopped
}

// Reason returns the current stop reason ("" when open).
func (a *Admissions) Reason() string {
	if a == nil {
		return ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.reason
}

// Stop stops admissions (idempotent).
func (a *Admissions) Stop(reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopped = true
	a.reason = reason
}

// Resume reopens admissions (idempotent).
func (a *Admissions) Resume() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopped = false
	a.reason = ""
}

const eagerEventSchemaVersion = 1

// EagerUnload is the operator-wins switch verb (R12). Nil-safe.
type EagerUnload struct {
	adm    *Admissions
	client *Client
	log    *eventlog.Log
	logger *slog.Logger
}

// NewEagerUnload wires the verb over a live Duty (shares its admission gate +
// client), recording events through log.
func NewEagerUnload(d *Duty, log *eventlog.Log, logger *slog.Logger) *EagerUnload {
	if logger == nil {
		logger = slog.Default()
	}
	if d == nil {
		return &EagerUnload{adm: NewAdmissions(), log: log, logger: logger}
	}
	return &EagerUnload{adm: d.Admissions(), client: d.Client(), log: log, logger: logger}
}

// Engage stops local-lane admissions AND unloads every running model (R12).
// Idempotent; event-recorded, owner-attributed to actor. The unload leg is a
// best-effort — admissions stop regardless (the operator-wins guarantee holds
// even if the endpoint is briefly unreachable).
func (e *EagerUnload) Engage(ctx context.Context, actor string) error {
	e.adm.Stop("eager-unload engaged by " + actor)
	var unloadErr error
	if e.client != nil {
		unloadErr = e.client.UnloadAll(ctx)
	}
	e.record(ctx, actor, EventLocalAdmissionsStopped, unloadErr)
	if unloadErr != nil {
		e.logger.Warn("local: eager-unload admissions stopped, but the unload endpoint call failed (models unload on TTL regardless)", "actor", actor, "err", unloadErr)
		return fmt.Errorf("local: eager-unload admissions stopped; unload endpoint call failed: %w", unloadErr)
	}
	return nil
}

// Resume reopens local-lane admissions (R12). Idempotent; event-recorded.
func (e *EagerUnload) Resume(ctx context.Context, actor string) error {
	e.adm.Resume()
	e.record(ctx, actor, EventLocalAdmissionsResumed, nil)
	return nil
}

// Stopped reports the current admission state (surface/status).
func (e *EagerUnload) Stopped() bool { return e.adm.Stopped() }

func (e *EagerUnload) record(ctx context.Context, actor, typ string, unloadErr error) {
	if e.log == nil {
		return
	}
	payload, _ := json.Marshal(struct {
		Actor     string `json:"actor"`
		UnloadOK  bool   `json:"unload_ok"`
		UnloadErr string `json:"unload_err,omitempty"`
	}{actor, unloadErr == nil, errString(unloadErr)})
	if _, err := e.log.Append(ctx, eventlog.Append{
		UserID: actor, Type: typ, SchemaVersion: eagerEventSchemaVersion, Payload: payload,
	}); err != nil {
		e.logger.Error("local: could not record eager-unload event", "type", typ, "err", err)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// SurfaceCard is the B6 card DATA for the one-tap eager-unload control (R12;
// S12.2 "surfaced as a one-tap card"). Rendering is B6's; this is the data.
type SurfaceCard struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Engaged     bool   `json:"engaged"`
	Reason      string `json:"reason,omitempty"`
	Action      string `json:"action"` // "engage" | "resume" (the verb the tap fires)
}

// Surface is the eager-unload control-plane surface: the verb + card DATA,
// constructed at the shell composition root (the buildAcceptSurface/
// buildPreviewSurface precedent) and held by the api layer for the B6
// endpoints. Management verbs are loopback/platform-plane only (S12.6).
type Surface struct {
	unload *EagerUnload
}

// NewSurface constructs the surface over the verb.
func NewSurface(unload *EagerUnload) *Surface { return &Surface{unload: unload} }

// Card returns the current one-tap card DATA (R12; B6 renders it).
func (s *Surface) Card() SurfaceCard {
	if s == nil || s.unload == nil {
		return SurfaceCard{Title: "Local GPU", Description: "local tier not configured", Action: "engage"}
	}
	engaged := s.unload.Stopped()
	c := SurfaceCard{
		Title:   "Free up the GPU",
		Engaged: engaged,
		Reason:  s.unload.adm.Reason(),
	}
	if engaged {
		c.Description = "Local models are unloaded and local-lane admissions are stopped (operator-wins). Tap to resume."
		c.Action = "resume"
	} else {
		c.Description = "Unload all local models now and stop local-lane admissions (S12.2 eager-unload). Tap to free the GPU."
		c.Action = "engage"
	}
	return c
}

// Engage / Resume drive the verb (the B6 endpoint + the CLI call in).
func (s *Surface) Engage(ctx context.Context, actor string) error { return s.unload.Engage(ctx, actor) }
func (s *Surface) Resume(ctx context.Context, actor string) error { return s.unload.Resume(ctx, actor) }
func (s *Surface) Stopped() bool                                  { return s.unload.Stopped() }

// UnloadAllDirect is the CLI's direct llama-swap unload leg (R12): the `sinet
// local unload` tool reaches llama-swap on loopback and unloads every model.
// The admission-stop leg needs the control plane's in-process state (the B6
// endpoint) — documented honestly by the CLI. Standalone so the tool needs no
// control-plane wiring.
func UnloadAllDirect(ctx context.Context, endpoint string) error {
	return NewClient(endpoint).UnloadAll(ctx)
}
