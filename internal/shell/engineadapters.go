package shell

// The control plane's substrate → adapter registration (Spec S03.2).
//
// This map is the only place a substrate becomes DISPATCHABLE. Until LN-2 it
// held exactly one entry, and that single entry — not any rule in the code —
// was what "one agentic lane" meant in practice. Registering a second adapter
// is therefore a deliberate act with a stated consequence, not a wiring
// detail: two paid agentic lanes become reachable, and the S08.8 flat-lane
// pressure rule becomes live for the first time.

import (
	"log/slog"
	"path/filepath"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
)

// engineAdapterDeps are the platform-owned inputs the registration needs.
type engineAdapterDeps struct {
	Settings claudecli.Settings
	Logger   *slog.Logger
	// StateDir roots the per-user engine trees (0700 each, one per person).
	StateDir string
}

// opencodeRoot is where the per-user XDG trees of the opencode substrate live.
func opencodeRoot(stateDir string) string {
	return filepath.Join(stateDir, "engines", "opencode")
}

// engineAdapters builds the registration map.
func engineAdapters(deps engineAdapterDeps) map[string]adapters.Adapter {
	return map[string]adapters.Adapter{
		// The Anthropic lane (Spec S03.2): the pinned `claude` CLI resolved
		// via PATH; conformance vs the components.lock pin is the adapter
		// suite's duty (S03.3).
		adapters.SubstrateClaudeCLI: &claudecli.Adapter{Settings: deps.Settings, Log: deps.Logger},
	}
}
