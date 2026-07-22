// Package preview is the Spec S13.8 preview module: one click launches a built
// application from a deliverable revision in a disposable try-out environment,
// and nothing touches the live copy or the reviewed workspace. It owns the
// per-type dispositions, the host-toolchain runner heuristics, the
// zero-mutation substrate (a worktree-at-rev utility checkout plus a
// discardable throwaway clone), netns port probing, the Caddy admin-API route
// per live preview, the systemd-socket-proxyd idle-stop unit TEXT, the
// lifecycle manager, and the before-vs-after synced-navigation data contract.
//
// The preview is a READ-ONLY CONSUMER of the settled substrate (Spec S13.8
// "executed inside the settled sandbox stack"): it composes the runner argv
// through the EXISTING internal/sandbox surface without a new class or a
// loosened profile, reads revision pins and accepted state through internal/
// review, and stages read-only checkouts through internal/project — it never
// mints, drains, accepts, or widens confinement. Launching a preview is a
// Low-tier reversible action (Spec S13.8): no effect journal, no approval, no
// PIN step-up; exposure exists only through the tailnet front chain (S01.4).
//
// Host hazard: the module NEVER dials the live Caddy admin API, never binds
// non-loopback, never touches tailscale or systemd state, and never installs
// units. The Caddy admin endpoint and the port range are STRUCTURAL config
// (composition-root override; no ⚙ key is invented). Rendering — the
// dual-iframe widget, picker UI, preview-state surfaces — is S15's (B6); this
// package's output is DATA.
//
// Import discipline (CONVENTIONS §25; R24): imports internal/{project,review,
// sandbox,units,eventlog,settings-interface} + internal/portpool + stdlib ONLY;
// NEVER internal/{adapters,stage,intake,memory,broker,gates,verify,worker,
// scheduler}. AST-pinned by conformance_test.go.
package preview

import (
	"errors"
	"time"
)

// Lane is the resolved preview mechanism for a deliverable revision.
type Lane string

const (
	// LaneStatic serves an index.html tree with the platform static server —
	// the ONE lane that runs host-side platform code, not a sandboxed toolchain
	// (Spec S13.8; R11).
	LaneStatic Lane = "static"
	// LaneDevServer runs a detected/registry runner (pnpm/npm/uv/mise) inside
	// the sandbox as a dev server (Spec S13.8; R2).
	LaneDevServer Lane = "dev-server"
	// LaneNotebook self-previews — the notebook file is its own artifact
	// (Spec S13.8; R4).
	LaneNotebook Lane = "notebook"
	// LaneCLI serves a CLI over ttyd inside the sandbox, over the same routing
	// (Spec S13.8; R20). CLI entry is data-driven, never heuristic-guessed.
	LaneCLI Lane = "cli"
	// LaneSidecar labels a Postgres/Redis-class project that needs the deferred
	// container tier (Spec S13.8; R4).
	LaneSidecar Lane = "sidecar"
	// LaneNone is the honest no-preview disposition for a type with no defined
	// launchable surface (desktop/embedded/unknown) — never a broken iframe
	// (Spec S13.8; R4).
	LaneNone Lane = "none"
)

// State is the preview session's defined state — every type gets one; there is
// no refusal branch (Spec S13.8 "never a broken iframe"; R4). Each carries a
// machine-readable Reason; rendering is S15's.
type State string

const (
	// StateLive: a backend is serving and (optionally) routed.
	StateLive State = "live"
	// StateSelfPreview: the notebook file is its own preview (R4).
	StateSelfPreview State = "self-preview"
	// StateRequiresContainer: a sidecar-needing project needs the deferred
	// container tier (R4).
	StateRequiresContainer State = "requires-container-tier"
	// StateNoPreview: no preview available for this type, or a required runner
	// tool / ttyd is absent (R4/R20). Never an install.
	StateNoPreview State = "no-preview"
	// StateUnavailable: the host cannot compose the sandbox boundary, so a
	// sandboxed lane cannot run here — never an unconfined run (R7).
	StateUnavailable State = "unavailable"
	// StateAtCapacity: the ⚙ preview.max_concurrent cap is reached (R15).
	StateAtCapacity State = "at-capacity"
)

// Role tags a before-vs-after side (Spec S13.8; R17).
type Role string

const (
	// RoleSingle is a standalone preview (no before-vs-after pairing).
	RoleSingle Role = "single"
	// RoleBefore is the accepted revision instance (the "before").
	RoleBefore Role = "before"
	// RoleAfter is the candidate revision instance (the "after").
	RoleAfter Role = "after"
)

// Port is one probed listening port, with a best-effort label — the multi-port
// picker DATA the S15 widget consumes when a dev server exposes more than one
// (Spec S13.8; R8).
type Port struct {
	Number int    `json:"number"`
	Label  string `json:"label,omitempty"`
}

// HostInjection is how the preview host is injected into a dev server's
// allowed-hosts surface (Spec S13.8: "allowedHosts injected, or Vite HMR
// silently dies"; R13). Provider-specific DATA carried on the runner
// invocation; the exact wire mechanism (config write vs flag vs env) is
// confirmed at the deferred live smoke (the srt-config precedent, CONVENTIONS
// §12 — generated faithfully-but-minimally, tuned live, never guessed blind).
type HostInjection struct {
	// Mechanism is "env" | "flag" | "config".
	Mechanism string `json:"mechanism"`
	// Key is the env-var name / flag / config path.
	Key string `json:"key"`
	// Value is the preview host, filled at launch.
	Value string `json:"value"`
}

// Runner is a resolved preview runner invocation.
type Runner struct {
	// Tool names the resolved runner: pnpm|npm|uv|mise|ttyd|static|registry.
	Tool string `json:"tool"`
	// Argv is the base launch command (empty for static/self-preview/none).
	Argv []string `json:"argv,omitempty"`
	// Source is "registry" (owner-approved preview command, R3) or "heuristic"
	// (host-toolchain detection, R2).
	Source string `json:"source"`
}

// Session is one preview instance — the DATA the S15 launch/list surfaces and
// the before-vs-after contract consume (rendering is B6's). Unexported live
// bookkeeping (paths, stop hook) is not serialized.
type Session struct {
	ID            string `json:"id"`
	DeliverableID string `json:"deliverable"`
	Revision      int    `json:"revision"`
	Role          Role   `json:"role"`
	Lane          Lane   `json:"lane"`
	State         State  `json:"state"`
	Reason        string `json:"reason,omitempty"`
	// URL is the user-facing view URL — the tailnet front-chain subdomain when
	// routed, else the loopback backend in dev (routing disabled). Empty for the
	// non-backed states (self-preview points URL at the notebook artifact).
	URL string `json:"url,omitempty"`
	// BackendAddr is the loopback address the route proxies to (the pool port);
	// empty for non-backed states.
	BackendAddr string `json:"backend_addr,omitempty"`
	PoolPort    int    `json:"pool_port,omitempty"`
	// RouteID is the Caddy admin-API route id; Routed reports whether a route
	// was added (false when the admin endpoint is unconfigured — dev default).
	RouteID string `json:"route_id,omitempty"`
	Routed  bool   `json:"routed"`
	// Ports is the multi-port picker DATA (R8); populated when >1 port listens.
	Ports []Port `json:"ports,omitempty"`
	// Runner records the resolved runner (evidence; empty for non-runner lanes).
	Runner *Runner `json:"runner,omitempty"`
	User   string  `json:"user"`
	// HostInjections records the allowedHosts injection applied to the dev
	// server (R13; evidence + the S15 surface).
	HostInjections []HostInjection `json:"host_injections,omitempty"`
	CreatedTS      string          `json:"created_ts"`

	// live bookkeeping — not serialized.
	projectID string
	checkout  string // the read-only worktree-at-rev utility checkout (R5)
	clone     string // the discardable throwaway clone absorbing writes (R6)
	stop      func() // stops a host-side backend (static server); nil otherwise
}

// backed reports whether the session holds a host-side port (and therefore
// counts against the ⚙ preview.max_concurrent cap, R15).
func (s *Session) backed() bool { return s.PoolPort != 0 }

// View is the lean per-side data the S15 dual-iframe widget consumes (Spec
// S13.8 synced-navigation contract; R18).
type SessionView struct {
	Role          Role   `json:"role"`
	DeliverableID string `json:"deliverable"`
	Revision      int    `json:"revision"`
	URL           string `json:"url"`
	State         State  `json:"state"`
	Reason        string `json:"reason,omitempty"`
}

// View projects a Session to its S15 dual-iframe side.
func (s *Session) View() SessionView {
	return SessionView{
		Role: s.Role, DeliverableID: s.DeliverableID, Revision: s.Revision,
		URL: s.URL, State: s.State, Reason: s.Reason,
	}
}

// SyncSemantics is the synced-navigation contract the S15 dual-iframe widget
// honors (Spec S13.8; R18). DATA only — behavior is S15's.
type SyncSemantics struct {
	// Mode is how navigation mirrors across the two iframes ("path" = mirror
	// the URL path+query across both sides).
	Mode string `json:"mode"`
	// Enabled is false for the single-instance case (no "before" to sync to).
	Enabled bool `json:"enabled"`
}

// Comparison is the before-vs-after synced-navigation DATA contract (Spec
// S13.8; R18) — the packet's API surface for the S15 dual-iframe widget (B6).
// Before is nil when the deliverable has no accepted revision yet (the honest
// single-instance state — revision 1 has no "before"; its diff-against-base
// story is S13.1's, not a preview fake).
type Comparison struct {
	DeliverableID  string        `json:"deliverable"`
	Before         *SessionView  `json:"before,omitempty"`
	After          SessionView   `json:"after"`
	SingleInstance bool          `json:"single_instance"`
	Sync           SyncSemantics `json:"sync"`
}

// Event types — provisional pending the S14 event contract (B5), extending the
// standing CONVENTIONS §7/§8/§13–§24 naming note. Platform-scope (run_id NULL,
// no generation — previews are not runs, R22/R23), owner-attributed to the
// launching user (15.6), refs + ids not blobs (P-T07-5).
const (
	// EventStarted records a preview launch.
	EventStarted = "preview.started"
	// EventStopped records a preview teardown.
	EventStopped = "preview.stopped"
)

const eventSchemaVersion = 1

// Errors callers branch on.
var (
	// ErrBadInput covers malformed launch input.
	ErrBadInput = errors.New("preview: invalid input")
	// ErrNoRevision rejects a preview of a deliverable with no minted revision.
	ErrNoRevision = errors.New("preview: deliverable has no minted revision")
	// ErrUnknownSession rejects a stop/teardown of an unknown session.
	ErrUnknownSession = errors.New("preview: unknown session")
)

// clock defaults to time.Now when a Manager has no injected clock.
func nowOr(now func() time.Time) time.Time {
	if now != nil {
		return now()
	}
	return time.Now()
}
