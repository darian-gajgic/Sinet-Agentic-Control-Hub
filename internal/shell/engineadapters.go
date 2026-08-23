package shell

// The control plane's substrate → adapter registration (Spec S03.2).
//
// This map is the only place a substrate becomes DISPATCHABLE. Until LN-2 it
// held exactly one entry, and that single entry — not any rule in the code —
// was what "one agentic lane" meant in practice. Registering the second
// adapter is therefore a deliberate act with a stated consequence, not a
// wiring detail: two paid agentic engine lanes become reachable and the S08.8
// flat-lane pressure rule becomes live for the first time.
//
// What does NOT change: no lane is COMMISSIONED by this. A lane exists when a
// person holds a provider entry and its credential, and both are placed by the
// operator's key ceremony. Until then the second adapter is present, selected
// by nothing, and refuses by name if something asks it to run an
// uncommissioned lane.

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
)

// engineAdapterDeps are the platform-owned inputs the registration needs.
type engineAdapterDeps struct {
	Settings claudecli.Settings
	Logger   *slog.Logger
	// StateDir roots the per-user engine trees (0700 each, one per person).
	StateDir string
	// Commissioned maps user → that person's lane provider entries. EMPTY at
	// v0, and empty is the honest state rather than a gap: see the file
	// comment. The map is the seam the key ceremony fills.
	Commissioned map[string]opencode.ProviderConfig
}

// opencodeRoot is where the per-user XDG trees of the opencode substrate live.
func opencodeRoot(stateDir string) string {
	return filepath.Join(stateDir, "engines", "opencode")
}

// engineLanes loads the lane documents the platform ships. A document that
// will not load is not a reason to run without lanes — it is a reason to say
// so: the adapter still registers, so a dispatch onto an uncommissioned lane
// still refuses by name rather than falling through to another substrate.
func engineLanes(logger *slog.Logger) []opencode.LaneConfig {
	lanes, err := opencode.SeedLaneConfigs()
	if err != nil {
		logger.Warn("opencode: seed lane documents did not load; no lane can be commissioned until they do", "err", err)
	}
	return lanes
}

// brokerSocketFor is the per-user broker UDS (one per person, SO_PEERCRED-
// guarded) — the same path the accept/push seams dial.
func brokerSocketFor(stateDir, userID string) string {
	return filepath.Join(stateDir, "broker", userID+".sock")
}

// laneCredInjector is the production stage.Config.CredInject seam: per user,
// the spawn-time injector for every lane that person is actually commissioned
// on, composed left to right.
//
// It returns nil for a person with nothing commissioned, which is the
// unchanged dev posture — and the reason a lane with no credential reports
// itself uncommissioned instead of spawning an engine that authenticates as
// nobody. Resolution happens per spawn, inside the closure: the material is
// never captured here, never stored on the request, and never persisted.
func laneCredInjector(stateDir string, lanes []opencode.LaneConfig,
	commissioned map[string]opencode.ProviderConfig) func(string) func([]string) ([]string, error) {
	return func(userID string) func([]string) ([]string, error) {
		entries := commissioned[userID]
		if len(entries) == 0 {
			return nil
		}
		socket := brokerSocketFor(stateDir, userID)
		var injectors []func([]string) ([]string, error)
		for _, lane := range lanes {
			if _, ok := entries[lane.ProviderID]; !ok {
				continue
			}
			if inject := laneCredInject(socket, lane); inject != nil {
				injectors = append(injectors, inject)
			}
		}
		if len(injectors) == 0 {
			return nil
		}
		return func(base []string) ([]string, error) {
			env := base
			for _, inject := range injectors {
				next, err := inject(env)
				if err != nil {
					return nil, err
				}
				env = next
			}
			return env, nil
		}
	}
}

// engineAdapters builds the registration map.
func engineAdapters(deps engineAdapterDeps) map[string]adapters.Adapter {
	lanes := engineLanes(deps.Logger)
	commissioned := deps.Commissioned
	return map[string]adapters.Adapter{
		// The Anthropic lane (Spec S03.2): the pinned `claude` CLI resolved
		// via PATH; conformance vs the components.lock pin is the adapter
		// suite's duty (S03.3).
		adapters.SubstrateClaudeCLI: &claudecli.Adapter{Settings: deps.Settings, Log: deps.Logger},
		// The opencode substrate (LN-1), carrying its lanes as config DATA
		// (S03.6). Instances is the dev-posture per-user serve manager; the D6
		// `sinet-engine@<user>` unit replaces the PROVIDER, not the adapter.
		adapters.SubstrateOpencode: &opencode.Adapter{
			Instances: &opencode.Manager{Log: deps.Logger},
			Root:      opencodeRoot(deps.StateDir),
			Lanes:     lanes,
			Log:       deps.Logger,
			ProvidersFor: func(userID string) (opencode.ProviderConfig, error) {
				return commissioned[userID], nil
			},
		},
	}
}

// laneCredInject builds the S11.5 spawn-time credential injector for one lane:
// the broker resolves the lane document's engine-cred auth PROFILE and
// delivers the material as the environment variable that same document names.
// Nothing else ever holds it — the worker record holds the reference, the
// resolution is fresh per spawn, and the value is never stored on the request,
// in a park record, in the event log or in the DB (D2/S11.5).
//
// It returns nil when the lane is not commissionable yet — no broker socket,
// no profile, no variable — because a lane with no credential must report
// itself uncommissioned rather than spawn an engine that will authenticate as
// nobody. laneCredInjector composes it into the production CredInject seam;
// the key ceremony's only job is to put the material behind the profile and
// the entry in the commissioned map.
func laneCredInject(socket string, lane opencode.LaneConfig) func(base []string) ([]string, error) {
	if socket == "" || lane.Credential.Profile == "" || lane.Credential.EnvVar == "" {
		return nil
	}
	return broker.EnvInjector(socket, lane.Credential.Profile, lane.Credential.EnvVar)
}

// closeEngineAdapters reaps anything an adapter owns at shutdown. The opencode
// substrate runs per-user server PROCESSES, and a control plane that exits
// without reaping them leaves engines holding this person's ports and XDG tree.
func closeEngineAdapters(reg map[string]adapters.Adapter, logger *slog.Logger) {
	oc, ok := reg[adapters.SubstrateOpencode].(*opencode.Adapter)
	if !ok {
		return
	}
	mgr, ok := oc.Instances.(*opencode.Manager)
	if !ok {
		return
	}
	if err := mgr.Close(); err != nil {
		logger.Warn("opencode: reaping the per-user serve instances failed", "err", fmt.Sprint(err))
	}
}
