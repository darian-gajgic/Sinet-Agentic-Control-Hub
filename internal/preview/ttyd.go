package preview

import "strconv"

// ttyd.go generates the ttyd invocation for the CLI preview lane (Spec S13.8:
// "ttyd serves CLIs over the same routing"; R20). ttyd is the packet's one
// adopted os-mechanism — probe-gated consumption (PATH + SINET_TTYD_PATH), a
// full S16.4 ten-check walk in components.lock, pin live-verified at build time
// (R21). It is consumed as a host subprocess INSIDE the sandbox (not an organ
// unit); absent ttyd is the honest no-preview state, never an install.

// ttydDefaultPort is ttyd's default listen port; the preview binds it inside
// the sandbox netns and the platform probes for it (the same probe→pool→route
// path as a dev server, R20).
const ttydDefaultPort = 7681

// ttydInvocation builds the argv that serves command over ttyd (Spec S13.8;
// R20): `ttyd -p <port> -W <command...>`. -W (writable) allows client input for
// an interactive CLI; the process runs INSIDE the sandbox, so ttyd's own web
// listener rides the netns probe. ttydPath is the probe-resolved binary (never
// a bare name — the sandbox composer needs an absolute host path).
func ttydInvocation(ttydPath string, port int, command []string) []string {
	argv := []string{ttydPath, "-p", strconv.Itoa(port), "-W"}
	return append(argv, command...)
}

// injectionEnv turns env-mechanism host injections into "KEY=VALUE" entries for
// the spawn env (Spec S13.8 allowedHosts injection; R13). Config/flag-mechanism
// injections are carried as DATA on the session and applied at the deferred live
// smoke (the srt-config precedent), not folded into env here.
func injectionEnv(inj []HostInjection) []string {
	var env []string
	for _, hi := range inj {
		if hi.Mechanism == "env" {
			env = append(env, hi.Key+"="+hi.Value)
		}
	}
	return env
}
