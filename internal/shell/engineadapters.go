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
	"sort"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
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

// commissionedLanes reports the lane names an operator has actually
// commissioned across every person — the S08.8 coverage input beyond the
// configured lane (P3-LN-2B R21).
//
// Registering a substrate made a second lane DISPATCHABLE; only a provider
// entry with a credential behind it makes one SELECTABLE, and routing work
// onto the difference is exactly the "not commissioned" state the lane
// documents exist to surface by name. So the answer is derived from what is
// actually placed, never from what ships: with the empty v0 map this returns
// nothing and coverage is unchanged.
//
// Coverage is per-owner in S08.8 and the Router is built once per control
// plane, so this is the union. That is the honest over-approximation at v0
// (one household, one operator placing keys); per-person coverage arrives with
// the per-person duty-map surface (1.10, B6/v1).
func commissionedLanes(lanes []opencode.LaneConfig, commissioned map[string]opencode.ProviderConfig) []string {
	byProvider := map[string]string{}
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

// laneConfiguredModels renders the lane documents as the CONFIG side of the
// S03.6 model-list diff, keyed by lane (P3-LN-2B R19).
//
// The documents are the only written record of which models a lane is
// configured for, and they carry their own verified-on dates — which is
// exactly what P-T17-3 wants on the config side of the comparison. They are
// NOT the authority: the account's observed list is, and the canary exists to
// find the day the two disagree.
func laneConfiguredModels(lanes []opencode.LaneConfig) map[string][]string {
	if len(lanes) == 0 {
		return nil
	}
	out := make(map[string][]string, len(lanes))
	for _, l := range lanes {
		ids := make([]string, 0, len(l.Models))
		for _, m := range l.Models {
			ids = append(ids, m.ID)
		}
		if len(ids) == 0 {
			continue
		}
		sort.Strings(ids)
		out[l.Lane] = ids
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// laneSubstrates maps each COMMISSIONED lane to the substrate its document
// names — the S03.2 dispatch input that did not exist before P3-LN-2B drain r1.
//
// Only commissioned lanes appear. A lane nobody holds a provider entry for
// cannot be seated by routing, so mapping it would be describing a dispatch
// that cannot happen; and the anthropic lane is deliberately absent because its
// substrate IS the configured ceremony default, which the stage keeps using
// unchanged. With nothing commissioned this returns nil and every dispatch
// takes exactly its pre-LN-2 path.
func laneSubstrates(lanes []opencode.LaneConfig, commissioned map[string]opencode.ProviderConfig) map[string]string {
	live := map[string]bool{}
	for _, lane := range commissionedLanes(lanes, commissioned) {
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

// laneAlternateSeats renders the commissioned lanes' execution seats from
// their documents (S08.8 step 3; P3-LN-2B drain r1 D5).
//
// The document is the only written record of which model a lane fronts, and it
// carries that fact's verified-on date — so the seat is composed here, at the
// root that already reads those documents, rather than written as a constant
// in the routing package where it would go stale invisibly.
func laneAlternateSeats(lanes []opencode.LaneConfig, commissioned map[string]opencode.ProviderConfig) worker.AlternateSeats {
	live := map[string]bool{}
	for _, lane := range commissionedLanes(lanes, commissioned) {
		live[lane] = true
	}
	var seats []worker.LaneSeat
	for _, l := range lanes {
		if !live[l.Lane] || l.DefaultModel == "" {
			continue
		}
		seats = append(seats, worker.LaneSeat{Lane: l.Lane, Model: l.DefaultModel})
	}
	return worker.AlternateSeatsFor(seats...)
}
