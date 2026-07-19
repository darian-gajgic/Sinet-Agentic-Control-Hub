package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Bootstrap is the bootstrap-only configuration of Spec S01.10: what the
// process needs before platform.db is open — the bind address and the DB
// path. Everything else is settings-registry rows; nothing is ever read
// from both places.
type Bootstrap struct {
	// HTTPAddr is the API listen address. It must be loopback: every sinet
	// unit binds 127.0.0.1 or a unix socket only (Spec S01.1), asserted
	// fail-closed by the listener-binding lint (P-T13-2).
	HTTPAddr string
	// DBPath is the platform.db path. Empty = resolve via
	// storage.ResolvePath ($STATE_DIRECTORY, else the dev-mode XDG state
	// dir).
	DBPath string
}

// DefaultHTTPAddr is the dev-mode default bind address. In production the
// value comes from /etc/sinet (bootstrap config); the port carries no spec
// meaning — the front chain proxies to whatever is configured (Spec S01.4).
const DefaultHTTPAddr = "127.0.0.1:8482"

// BootstrapFileName is the bootstrap config file under the configuration
// directory (Spec S01.10: bootstrap-only config lives in /etc/sinet,
// populated via ConfigurationDirectory=, Spec S01.11).
const BootstrapFileName = "bootstrap.conf"

// devPosture reports the dev-mode process posture: the absence of systemd
// env — NOTIFY_SOCKET, STATE_DIRECTORY, CONFIGURATION_DIRECTORY — never a
// build flag (the B0-3 rule, P3/CONVENTIONS.md §7). Any of the three
// present reads as production: posture ambiguity fails toward the stricter
// side (no dev identity fallback, Secure cookies).
func devPosture() bool {
	return os.Getenv("NOTIFY_SOCKET") == "" &&
		os.Getenv("STATE_DIRECTORY") == "" &&
		os.Getenv("CONFIGURATION_DIRECTORY") == ""
}

// resolveConfigDir returns the configuration directory: the explicit
// override, else $CONFIGURATION_DIRECTORY (systemd ConfigurationDirectory=;
// first entry of a colon list), else /etc/sinet.
func resolveConfigDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if cd := os.Getenv("CONFIGURATION_DIRECTORY"); cd != "" {
		return strings.SplitN(cd, ":", 2)[0]
	}
	return "/etc/sinet"
}

// loadBootstrap loads bootstrap.conf from the configuration directory. A
// missing file is the dev posture and yields defaults; any other read or
// parse failure is fail-closed — configuration drift must not pass silently
// (feature 4.6).
func loadBootstrap(configDir string) (Bootstrap, error) {
	b := Bootstrap{HTTPAddr: DefaultHTTPAddr}
	path := filepath.Join(resolveConfigDir(configDir), BootstrapFileName)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return b, nil
	}
	if err != nil {
		return Bootstrap{}, fmt.Errorf("shell: read bootstrap config %s: %w", path, err)
	}
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return Bootstrap{}, fmt.Errorf("shell: %s:%d: not a key = value line: %q", path, i+1, line)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "http_addr":
			if value != "" {
				b.HTTPAddr = value
			}
		case "db_path":
			b.DBPath = value
		default:
			// Strict: an unknown key is drift, not decoration.
			return Bootstrap{}, fmt.Errorf("shell: %s:%d: unknown bootstrap key %q", path, i+1, key)
		}
	}
	return b, nil
}
