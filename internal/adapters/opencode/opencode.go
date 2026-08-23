// Package opencode is the pinned `opencode serve` substrate of Spec S03.2
// behind the D3 contract (internal/adapters). It carries the Z.AI and local
// lanes as CONFIG DATA; nothing lane-specific is compiled in (S03.6: adding a
// lane is a provider entry plus billing flags, never a new substrate).
//
// Engine pin: components.lock entry "opencode-ai (engine)". The engine runs
// UNMODIFIED (S16.1) — every platform opinion (lowering, parsing, gating,
// checkpoints) lives on this side of the seam, and `POST /global/upgrade` is
// never exposed or called (S03.3 rule 2).
//
// AMENDMENT-A INERT SURFACE (P3-LN-1 red window): this file declares the
// package's exported and package-internal type surface so the brief's §7
// acceptance tests compile and FAIL. It carries no behavior; the
// implementation commit replaces it.
package opencode

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// DefaultBinary is the engine executable resolved via PATH when the instance
// manager is not configured with an explicit path.
const DefaultBinary = "opencode"

// Pin is the binding engine version (components.lock "opencode-ai (engine)").
const Pin = "1.18.3"

// BasicAuthUser is the HTTP Basic username the pinned engine accepts.
const BasicAuthUser = "opencode"

// Errors.
var (
	ErrConfinementUnsupported  = errNotImplemented
	ErrNativeSpawnTool         = errNotImplemented
	ErrUpdatedInputUnsupported = errNotImplemented
	ErrNoEngineSession         = errNotImplemented
	ErrInstanceUnavailable     = errNotImplemented
)

var errNotImplemented = &inertError{}

type inertError struct{}

func (*inertError) Error() string { return "opencode: not implemented (P3-LN-1 red window)" }

func inert() { panic("opencode: not implemented (P3-LN-1 red window)") }

// ProviderConfig is the lane provider block compiled into the engine config as
// DATA (R15 seam).
type ProviderConfig map[string]ProviderEntry

// ProviderEntry is one opencode provider definition.
type ProviderEntry struct {
	NPM     string                `json:"npm,omitempty"`
	Name    string                `json:"name,omitempty"`
	Options map[string]any        `json:"options,omitempty"`
	Models  map[string]ModelEntry `json:"models,omitempty"`
}

// ModelEntry is one model under a provider entry.
type ModelEntry struct {
	Name string `json:"name,omitempty"`
}

// Instance is one per-user serve endpoint.
type Instance struct {
	BaseURL  string
	Password string
	Root     string
}

// InstanceSpec is the lowered request for a per-user instance.
type InstanceSpec struct {
	UserID      string
	Root        string
	Cwd         string
	Fingerprint string
	ConfigJSON  []byte
	Env         []string
}

// Instances is the per-user serve endpoint seam (R3).
type Instances interface {
	Acquire(ctx context.Context, spec InstanceSpec) (Instance, error)
	Stop(ctx context.Context, userID string) error
}

// Manager is the dev-posture per-user serve spawner.
type Manager struct {
	Binary         string
	BootTimeout    time.Duration
	StopGrace      time.Duration
	HealthInterval time.Duration
	HTTP           *http.Client
	Log            *slog.Logger
}

var _ Instances = (*Manager)(nil)

// Acquire implements Instances.
func (m *Manager) Acquire(ctx context.Context, spec InstanceSpec) (Instance, error) {
	inert()
	return Instance{}, nil
}

// Stop implements Instances.
func (m *Manager) Stop(ctx context.Context, userID string) error { inert(); return nil }

// Close reaps every live instance.
func (m *Manager) Close() error { inert(); return nil }

// HealthWatch polls /api/health with the instance's Basic credentials.
type HealthWatch struct {
	inertField int
}

// NewHealthWatch builds the health watcher for one instance.
func NewHealthWatch(inst Instance, hc *http.Client, interval time.Duration, log *slog.Logger) *HealthWatch {
	inert()
	return nil
}

// Probe runs one health poll.
func (w *HealthWatch) Probe(ctx context.Context) error { inert(); return nil }

// Run polls until ctx is done.
func (w *HealthWatch) Run(ctx context.Context) { inert() }

// Healthy reports the last poll's verdict.
func (w *HealthWatch) Healthy() bool { inert(); return false }

// Adapter is the opencode-lane implementation of the D3 contract.
type Adapter struct {
	Instances   Instances
	Providers   ProviderConfig
	Root        string
	Env         []string
	Now         func() time.Time
	Log         *slog.Logger
	HTTP        *http.Client
	CancelGrace time.Duration
}

var _ adapters.Adapter = (*Adapter)(nil)

// Substrate implements adapters.Adapter.
func (a *Adapter) Substrate() string { return adapters.SubstrateOpencode }

// Start implements adapters.Adapter.
func (a *Adapter) Start(ctx context.Context, req adapters.StartRequest) (adapters.Session, error) {
	inert()
	return nil, nil
}

// Resume implements adapters.Adapter.
func (a *Adapter) Resume(ctx context.Context, rec adapters.ParkRecord, ans *adapters.Answer) (adapters.Session, error) {
	inert()
	return nil, nil
}

// ApproveAnswer builds the approve decision for a gate ask.
func ApproveAnswer(askID string) *adapters.Answer { inert(); return nil }

// RejectAnswer builds the reject-with-feedback decision for a gate ask.
func RejectAnswer(askID, feedback string) *adapters.Answer { inert(); return nil }

// ── package-internal surface (exercised by the in-package conformance suite) ──

type modelRef struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

type lowered struct {
	root         string
	configJSON   []byte
	env          []string
	agentName    string
	systemAppend string
	prompt       string
	model        modelRef
	fingerprint  string
}

func (a *Adapter) lower(req adapters.StartRequest) (*lowered, error) { inert(); return nil, nil }

func (a *Adapter) instanceSpec(req adapters.StartRequest, l *lowered, env []string) InstanceSpec {
	inert()
	return InstanceSpec{}
}

type parser struct {
	sessionID     string
	directory     string
	finalText     string
	errDetail     string
	lastFinish    string
	paidCalls     int64
	unknownFrames int
	sawBusy       bool
	sawIdle       bool
	ask           *adapters.Ask
	replied       map[string]string
}

func newParser(sessionID string, logf func(string, ...any)) *parser { inert(); return nil }

func (p *parser) feed(frame []byte) []adapters.Event { inert(); return nil }

func readFrames(r io.Reader, onFrame func([]byte), logf func(string, ...any)) error {
	inert()
	return nil
}

type session struct {
	a           *Adapter
	req         adapters.StartRequest
	low         *lowered
	p           *parser
	cursor      adapters.Cursor
	paused      bool
	cancelStage int
	orphaned    json.RawMessage
}

var _ adapters.Session = (*session)(nil)

func (s *session) Events() <-chan adapters.Event    { inert(); return nil }
func (s *session) Cursor() adapters.Cursor          { inert(); return adapters.Cursor{} }
func (s *session) Fingerprint() string              { inert(); return "" }
func (s *session) Pause(ctx context.Context) error  { inert(); return nil }
func (s *session) Cancel(ctx context.Context) error { inert(); return nil }
func (s *session) Wait(ctx context.Context) (adapters.Outcome, error) {
	inert()
	return adapters.Outcome{}, nil
}

func (s *session) assembleOutcome() (adapters.Outcome, []adapters.Event) {
	inert()
	return adapters.Outcome{}, nil
}
