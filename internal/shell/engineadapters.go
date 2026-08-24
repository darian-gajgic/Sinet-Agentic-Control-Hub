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
// What registration does NOT do: it commissions nothing. A lane exists for a
// person when that person holds a provider entry AND its credential. The
// ceremony places the credential; commissionEngineLanes below composes the
// entries from what it finds placed, at startup (corrected 2026-08-24, P3-LN-4
// — the map used to be constructed empty at the composition root, which made a
// credentialled lane unroutable). A lane nobody has placed a credential for
// leaves the second adapter present, selected by nothing, refusing by name if
// something asks it to run.

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

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
	// Lanes are the shipped lane documents, loaded ONCE at the composition root
	// and shared by every consumer of the commissioned map. The commissioning
	// read and the four consumers that SNAPSHOT the map must see the same lane
	// set, or a document that stops loading between two calls desynchronises
	// them silently. A zero value loads them here, for a caller that wants the
	// registration alone.
	Lanes []opencode.LaneConfig
	// Commissioned maps user → that person's lane provider entries, composed by
	// commissionEngineLanes from the credentials actually placed in each
	// person's broker store (corrected 2026-08-24, P3-LN-4: this was EMPTY at
	// v0). Empty stays the honest state for a host where nothing is placed.
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

// commissionEngineLanes composes the commissioned map: for every person who has
// a broker store, every shipped lane document whose credential is actually
// placed in that person's store under the document's own auth profile, as that
// lane's provider entry (S03.6, S11.5).
//
// The read is secret-free by construction — broker.PlacedEngineCreds answers
// from the record's plaintext kind and never decrypts, never writes and never
// creates the master key — and the entries come from the documents themselves,
// so no endpoint and no model id is composed here.
//
// The map is keyed by the broker `who`: the broker derives its socket name AND
// its store directory from one name, and laneCredInjector dials that socket, so
// any other key would dial a socket for a person the store knows nothing about.
// The startup line below names those strings, because the platform's own person
// ids are a different namespace and at v0 they merely coincide (LN gate item).
//
// Commissioning is STARTUP-BOUND. Four of the map's five consumers snapshot it
// at composition, so a key placed while the control plane is running is picked
// up on its NEXT start; refilling live would produce a control plane whose
// adapter believes a lane is held and whose router never heard of it.
func commissionEngineLanes(stateDir string, lanes []opencode.LaneConfig, logger *slog.Logger) map[string]opencode.ProviderConfig {
	root := broker.StoreRoot(stateDir)
	people, err := broker.StorePeople(root)
	if err != nil {
		logger.Warn("lanes: the broker store root could not be listed, so nothing is commissioned this start",
			"root", root, "err", fmt.Sprint(err))
	}
	placed := make(map[string]map[string]bool, len(people))
	for _, who := range people {
		profiles, err := broker.PlacedEngineCreds(root, who)
		if err != nil {
			// Includes a store directory whose NAME the broker refuses. Nothing
			// this platform writes can create one, so it was made by hand — and
			// dropping a person's whole store in silence is how an operator
			// spends an afternoon on a lane that was never going to commission.
			logger.Warn("lanes: a broker store directory could not be read, so that person's lanes stay uncommissioned this start",
				"dir", filepath.Join(root, who), "err", fmt.Sprint(err))
			continue
		}
		placed[who] = profiles
	}
	commissioned := opencode.Commission(lanes, placed)
	logCommissioned(logger, lanes, placed, commissioned)
	return commissioned
}

// unmatchedProfiles reports the engine-cred profiles a person has placed that
// NO shipped lane document names, sorted.
//
// This is the difference between "you placed nothing" and "you placed something
// under a name nothing reads", and without it the two produce a character-
// identical startup line — which would defeat the whole reason the line exists.
// A typo'd profile is the likeliest way a key ceremony ends with a lane that
// never commissions.
func unmatchedProfiles(lanes []opencode.LaneConfig, profiles map[string]bool) []string {
	known := make(map[string]bool, len(lanes))
	for _, l := range lanes {
		if l.Credential.Profile != "" {
			known[l.Credential.Profile] = true
		}
	}
	var out []string
	for profile := range profiles {
		if !known[profile] {
			out = append(out, profile)
		}
	}
	sort.Strings(out)
	return out
}

// logCommissioned is the one startup line that says what a placed key actually
// bought (S14.1). Auth profiles and lane names ship in the lane documents and
// are not secrets; nothing else appears. A control plane that commissioned
// silently is one where an operator cannot tell a placed key from a typo'd
// profile name.
func logCommissioned(logger *slog.Logger, lanes []opencode.LaneConfig,
	placed map[string]map[string]bool, commissioned map[string]opencode.ProviderConfig) {
	people := make([]string, 0, len(placed))
	for who := range placed {
		people = append(people, who)
	}
	sort.Strings(people)
	perPerson := make([]string, 0, len(people))
	var stray []string
	for _, who := range people {
		held := commissionedLanes(lanes, map[string]opencode.ProviderConfig{who: commissioned[who]})
		perPerson = append(perPerson, who+"="+strings.Join(held, "+"))
		for _, profile := range unmatchedProfiles(lanes, placed[who]) {
			stray = append(stray, who+":"+profile)
		}
	}
	logger.Info("lanes: commissioned from the credentials placed in each person's broker store",
		"people", len(commissioned),
		"lanes", strings.Join(commissionedLanes(lanes, commissioned), ","),
		"per_person", strings.Join(perPerson, " "),
		"placed_matching_no_lane_document", strings.Join(stray, " "),
		"note", "startup-bound: a credential placed later is picked up at the next control-plane start")
}

// engineAdapters builds the registration map.
func engineAdapters(deps engineAdapterDeps) map[string]adapters.Adapter {
	lanes := deps.Lanes
	if lanes == nil {
		lanes = engineLanes(deps.Logger)
	}
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
	if socket == "" || !lane.Commissionable() {
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
// entry with a credential behind it makes one SELECTABLE, and routing work onto
// the difference is exactly the "not commissioned" state the lane documents
// exist to surface by name.
//
// The derivation itself lives beside the lane documents it reads
// (opencode.CommissionedLanes), because internal/stage's dispatch guards must
// reach the same function this composition root uses and cannot import this
// package. These three wrappers are the shell-side names their consumers and
// their existing tests already know.
func commissionedLanes(lanes []opencode.LaneConfig, commissioned map[string]opencode.ProviderConfig) []string {
	return opencode.CommissionedLanes(lanes, commissioned)
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
// The anthropic lane is deliberately absent: its substrate IS the configured
// ceremony default, which the stage keeps using unchanged.
func laneSubstrates(lanes []opencode.LaneConfig, commissioned map[string]opencode.ProviderConfig) map[string]string {
	return opencode.CommissionedSubstrates(lanes, commissioned)
}

// laneAlternateSeats renders the commissioned lanes' execution seats from
// their documents (S08.8 step 3; P3-LN-2B drain r1 D5).
//
// The document is the only written record of which model a lane fronts, and it
// carries that fact's verified-on date, so no model id is a constant in the
// routing package. This is the type adaptation only: which lanes have a seat,
// and which model each fronts, is opencode.CommissionedSeats' answer.
func laneAlternateSeats(lanes []opencode.LaneConfig, commissioned map[string]opencode.ProviderConfig) worker.AlternateSeats {
	var seats []worker.LaneSeat
	for _, s := range opencode.CommissionedSeats(lanes, commissioned) {
		seats = append(seats, worker.LaneSeat{Lane: s.Lane, Model: s.Model})
	}
	return worker.AlternateSeatsFor(seats...)
}
