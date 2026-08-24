// Command lanekey is the LN-CEREMONY credential tool: the ONE place a lane's
// API key is handled during the guided key-placement ceremony.
//
// WHY IT EXISTS. No `sinet` verb places an engine-cred. `sinet broker` is the
// DAEMON mode only, and its socket `store` op is dev-gated behind
// --allow-store; broker.Store.Put is an in-package admin/setup call whose own
// doc comment names this exact moment — "the operator migrating a key into the
// broker at a future gate". This tool is that gate's hands and nothing else: a
// read-write TOOL under tools/, never a platform change.
//
// SECRET POSTURE — the rules it enforces on itself:
//
//   - A secret is accepted ONLY on stdin. There is no --secret flag, and the
//     tool refuses to start if a secret-shaped flag is passed anyway.
//   - A secret is NEVER printed, logged, or written anywhere except the broker
//     store. What is printed is a per-run SALTED fingerprint: it is not
//     comparable across runs and not comparable to any stored hash.
//   - put / verify / wire401 spawn NO child process at all.
//   - exec is the ONE sanctioned env-injection path, and it mirrors the
//     platform's own spawn mechanism (broker.EnvInjector: material resolved
//     from the broker at spawn INTO the process environment, while the config
//     the engine reads names the variable only). It REFUSES if the value would
//     land in the child's argv, and REFUSES if the variable is already set in
//     its own environment.
//
// Every lane fact — provider id, base URL, endpoint marker, credential profile,
// env var, default model, wire protocol — is read from the lane DOCUMENT
// through the platform's own loader (opencode.LoadLaneConfig). Nothing about a
// lane is hardcoded here.
//
// Verbs:
//
//	lanekey put     --store-dir D --user U --lane L   # secret on stdin
//	lanekey verify  --store-dir D --user U --lane L
//	lanekey exec    --store-dir D --user U --lane L -- cmd args...
//	lanekey wire401 --lane L --out FILE               # invalid key, on purpose
//	lanekey show    --lane L                          # derived facts, no secret
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
)

// invalidProbeKey is the deliberately-invalid credential the wire401 capture
// sends. It is a literal constant so it is obvious by reading that no real key
// can reach that code path: wire401 never opens the broker store at all.
const invalidProbeKey = "INVALID-KEY-FOR-CEREMONY-WIRE-CAPTURE-0000"

// secretFlagNames are argv shapes that would carry a secret on the command
// line. The tool refuses them rather than ignoring them, so a habit formed at
// another CLI fails loudly here instead of leaking into the process table.
var secretFlagNames = []string{"secret", "key", "api-key", "apikey", "value", "token", "password", "pass"}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if err := refuseSecretsInArgv(args); err != nil {
		fmt.Fprintf(stderr, "lanekey: %v\n", err)
		return 2
	}
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	verb, rest := args[0], args[1:]
	var err error
	switch verb {
	case "put":
		err = cmdPut(rest, stdout)
	case "verify":
		err = cmdVerify(rest, stdout)
	case "exec":
		return cmdExec(rest, stdout, stderr)
	case "wire401":
		err = cmdWire401(rest, stdout)
	case "show":
		err = cmdShow(rest, stdout)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "lanekey: unknown verb %q\n\n%s", verb, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "lanekey %s: %v\n", verb, err)
		return 1
	}
	return 0
}

const usage = `lanekey — LN-CEREMONY credential tool (secrets: stdin only, never argv)

  lanekey put     --store-dir D --user U --lane L    place a key (stdin) + round-trip verify
  lanekey verify  --store-dir D --user U --lane L    round-trip verify through the broker
  lanekey exec    --store-dir D --user U --lane L -- cmd args...
                                                     resolve and run cmd with the lane's env var set
  lanekey wire401 --lane L --out FILE                capture one real error body (INVALID key, on purpose)
  lanekey show    --lane L                           print the lane's derived facts (no secret)

Common flags: --socket PATH  verify through a LIVE broker daemon instead of a
private in-process one; --model ID overrides the lane's default model.
`

// refuseSecretsInArgv is the standing guard: this tool takes secrets on stdin
// and nowhere else, so a secret-shaped flag is a refusal and not a warning.
func refuseSecretsInArgv(args []string) error {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name := strings.TrimLeft(a, "-")
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		for _, bad := range secretFlagNames {
			if strings.EqualFold(name, bad) {
				return fmt.Errorf("REFUSED: --%s would put a secret in this process's argv, "+
					"which every user on the host can read. Secrets are accepted on STDIN only", name)
			}
		}
	}
	return nil
}

// ─── lane facts, all derived from the document ──────────────────────────────

type laneFacts struct {
	cfg     opencode.LaneConfig
	path    string
	base    string
	proto   string // "anthropic" or "openai-compatible"
	profile string
	envVar  string
	model   string
}

func loadLane(path, modelOverride string) (laneFacts, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return laneFacts{}, fmt.Errorf("read lane document: %w", err)
	}
	cfg, err := opencode.LoadLaneConfig(raw)
	if err != nil {
		return laneFacts{}, fmt.Errorf("load lane document: %w", err)
	}
	entry := cfg.ProviderEntry()
	// R11 endpoint self-check, through the platform's own rule, BEFORE any
	// value is derived from it: a lane pointed at a sibling endpoint spends a
	// different balance, and that must fail here rather than on the wire.
	if ok, why := cfg.VerifyEndpoint(entry); !ok {
		return laneFacts{}, fmt.Errorf("endpoint self-check FAILED: %s", why)
	}
	base, _ := entry.Options["baseURL"].(string)
	f := laneFacts{
		cfg: cfg, path: path, base: strings.TrimRight(base, "/"),
		proto:   protocolOf(cfg.NPM),
		profile: cfg.Credential.Profile,
		envVar:  cfg.Credential.EnvVar,
		model:   cfg.DefaultModel,
	}
	if modelOverride != "" {
		f.model = modelOverride
	}
	if f.profile == "" || f.envVar == "" {
		return laneFacts{}, fmt.Errorf("lane %q declares no credential profile/env var — it cannot be commissioned (S11.5)", cfg.Lane)
	}
	return f, nil
}

// protocolOf maps the lane document's npm field — the field that exists
// precisely so the substrate difference is DATA and not a second code path
// (CONVENTIONS §64) — onto the wire shape this tool speaks.
func protocolOf(npm string) string {
	if strings.Contains(npm, "anthropic") {
		return "anthropic"
	}
	return "openai-compatible"
}

// ─── broker plumbing ────────────────────────────────────────────────────────

// withBroker runs fn against a REAL broker server. Default is a private
// in-process one bound to a 0700 temp dir over the SAME store — so the
// round-trip goes through Server.dispatch, the AAD audience binding, and the
// destination constraint that refuses to resolve anything but an engine-cred.
// With --socket it goes to the live daemon instead.
func withBroker(storeDir, user, socket string, fn func(c *broker.Client) error) error {
	if socket != "" {
		c, err := broker.Dial(socket)
		if err != nil {
			return fmt.Errorf("dial live broker %s: %w", socket, err)
		}
		defer c.Close()
		return fn(c)
	}
	store, err := broker.OpenStore(storeDir, user)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "lanekey-broker-")
	if err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	sock := filepath.Join(dir, "b.sock")
	ln, err := broker.Listen(sock)
	if err != nil {
		return err
	}
	srv := broker.NewServer(store, uint32(os.Getuid()), slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); _ = srv.Serve(ctx, ln) }()

	c, err := broker.Dial(sock)
	if err != nil {
		cancel()
		<-done
		return fmt.Errorf("dial private broker: %w", err)
	}
	err = fn(c)
	c.Close()
	cancel()
	<-done
	return err
}

// fingerprint is a PER-RUN salted digest. The salt is random and discarded, so
// the printed value proves "these two bytes are equal" inside one run and
// nothing at all outside it — in particular it can never be matched against a
// leaked hash of the key.
func fingerprint(salt, secret []byte) string {
	m := hmac.New(sha256.New, salt)
	m.Write(secret)
	return hex.EncodeToString(m.Sum(nil))[:16]
}

func newSalt() ([]byte, error) {
	s := make([]byte, 16)
	_, err := rand.Read(s)
	return s, err
}

// ─── put ────────────────────────────────────────────────────────────────────

func cmdPut(args []string, out io.Writer) error {
	fs := newFlags("put")
	storeDir := fs.String("store-dir", "", "broker store root")
	user := fs.String("user", "", "person whose store this is")
	lanePath := fs.String("lane", "", "path to the lane document")
	socket := fs.String("socket", "", "verify through this live broker socket")
	if err := fs.Parse(args); err != nil {
		return err
	}
	lane, err := loadLane(*lanePath, "")
	if err != nil {
		return err
	}
	if err := requireStore(*storeDir, *user); err != nil {
		return err
	}

	secret, err := readSecretStdin()
	if err != nil {
		return err
	}
	defer wipe(secret)

	salt, err := newSalt()
	if err != nil {
		return err
	}
	want := fingerprint(salt, secret)

	store, err := broker.OpenStore(*storeDir, *user)
	if err != nil {
		return err
	}
	if err := store.Put(lane.profile, broker.KindEngineCred, string(secret)); err != nil {
		return fmt.Errorf("store credential: %w", err)
	}
	fmt.Fprintf(out, "   placed: profile %q kind %q under %s/%s\n", lane.profile, broker.KindEngineCred, *storeDir, *user)

	got, err := resolveFingerprint(*storeDir, *user, *socket, lane.profile, salt)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("ROUND-TRIP MISMATCH for profile %q — what the broker returns is not what was placed", lane.profile)
	}
	fmt.Fprintf(out, "   round-trip through the broker: MATCH (per-run fingerprint %s)\n", got)
	fmt.Fprintf(out, "   PASS: lane %q credential is placed and resolvable as %s\n", lane.cfg.Lane, lane.envVar)
	return nil
}

// resolveFingerprint resolves the profile THROUGH the broker and returns the
// salted fingerprint of what came back. The value itself never leaves this
// function.
func resolveFingerprint(storeDir, user, socket, profile string, salt []byte) (string, error) {
	var fp string
	err := withBroker(storeDir, user, socket, func(c *broker.Client) error {
		secret, kind, err := c.Resolve(profile)
		if err != nil {
			return fmt.Errorf("broker resolve %q: %w", profile, err)
		}
		if kind != broker.KindEngineCred {
			return fmt.Errorf("profile %q resolved as %q, not %q", profile, kind, broker.KindEngineCred)
		}
		b := []byte(secret)
		fp = fingerprint(salt, b)
		wipe(b)
		return nil
	})
	return fp, err
}

// ─── verify ─────────────────────────────────────────────────────────────────

func cmdVerify(args []string, out io.Writer) error {
	fs := newFlags("verify")
	storeDir := fs.String("store-dir", "", "broker store root")
	user := fs.String("user", "", "person whose store this is")
	lanePath := fs.String("lane", "", "path to the lane document")
	socket := fs.String("socket", "", "verify through this live broker socket")
	if err := fs.Parse(args); err != nil {
		return err
	}
	lane, err := loadLane(*lanePath, "")
	if err != nil {
		return err
	}
	if err := requireStore(*storeDir, *user); err != nil {
		return err
	}
	salt, err := newSalt()
	if err != nil {
		return err
	}
	fp, err := resolveFingerprint(*storeDir, *user, *socket, lane.profile, salt)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "   PASS: profile %q resolves through the broker as an %s (per-run fingerprint %s)\n",
		lane.profile, broker.KindEngineCred, fp)
	return nil
}

// ─── exec ───────────────────────────────────────────────────────────────────

// cmdExec is the ONE sanctioned env-injection path. It is the shape the
// platform itself uses at spawn (broker.EnvInjector), narrowed to one child and
// guarded in both directions.
func cmdExec(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("exec")
	storeDir := fs.String("store-dir", "", "broker store root")
	user := fs.String("user", "", "person whose store this is")
	lanePath := fs.String("lane", "", "path to the lane document")
	socket := fs.String("socket", "", "resolve through this live broker socket")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	child := fs.Args()
	if len(child) == 0 {
		fmt.Fprintln(stderr, "lanekey exec: no command given (use `-- cmd args...`)")
		return 2
	}
	lane, err := loadLane(*lanePath, "")
	if err != nil {
		fmt.Fprintf(stderr, "lanekey exec: %v\n", err)
		return 1
	}
	if err := requireStore(*storeDir, *user); err != nil {
		fmt.Fprintf(stderr, "lanekey exec: %v\n", err)
		return 1
	}
	// The variable must not already be set: an inherited value would shadow
	// the broker's, and the run would prove nothing about what was placed.
	if _, ok := os.LookupEnv(lane.envVar); ok {
		fmt.Fprintf(stderr, "lanekey exec: REFUSED: %s is already set in this environment — "+
			"the broker's value would be indistinguishable from an inherited one\n", lane.envVar)
		return 1
	}

	code := 1
	err = withBroker(*storeDir, *user, *socket, func(c *broker.Client) error {
		secret, kind, err := c.Resolve(lane.profile)
		if err != nil {
			return fmt.Errorf("broker resolve %q: %w", lane.profile, err)
		}
		if kind != broker.KindEngineCred {
			return fmt.Errorf("profile %q resolved as %q", lane.profile, kind)
		}
		// The guard that gives this verb its name: the value may travel in the
		// child's ENVIRONMENT (owner-readable /proc/<pid>/environ) and never in
		// its ARGV (world-readable process table).
		for _, a := range child {
			if strings.Contains(a, secret) {
				return errors.New("REFUSED: the resolved credential appears in the child's argv")
			}
		}
		for _, kv := range os.Environ() {
			if i := strings.IndexByte(kv, '='); i >= 0 && kv[i+1:] == secret {
				return fmt.Errorf("REFUSED: the resolved credential is already exported as %s", kv[:i])
			}
		}
		cmd := exec.Command(child[0], child[1:]...)
		cmd.Env = append(os.Environ(), lane.envVar+"="+secret)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
		runErr := cmd.Run()
		code = cmd.ProcessState.ExitCode()
		var ee *exec.ExitError
		if runErr != nil && !errors.As(runErr, &ee) {
			return runErr
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(stderr, "lanekey exec: %v\n", err)
		return 1
	}
	return code
}

// ─── wire401 ────────────────────────────────────────────────────────────────

// cmdWire401 sends ONE request carrying a deliberately-invalid credential and
// records what comes back, verbatim. It never opens the broker store: there is
// no code path from here to a real key.
func cmdWire401(args []string, out io.Writer) error {
	fs := newFlags("wire401")
	lanePath := fs.String("lane", "", "path to the lane document")
	outPath := fs.String("out", "", "capture file to write")
	model := fs.String("model", "", "model id (default: the lane's default_model)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	lane, err := loadLane(*lanePath, *model)
	if err != nil {
		return err
	}
	if *outPath == "" {
		return errors.New("--out is required")
	}

	url, body, hdr := probeRequest(lane, invalidProbeKey)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	sent := make([]string, 0, len(hdr))
	for k, v := range hdr {
		req.Header.Set(k, v)
		if isCredHeader(k) {
			sent = append(sent, k+": <deliberately-invalid probe key, redacted>")
		} else {
			sent = append(sent, k+": "+v)
		}
	}
	sort.Strings(sent)

	client := &http.Client{Timeout: 30 * time.Second}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("probe %s: %w", url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return err
	}
	elapsed := time.Since(start)

	names := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		names = append(names, k)
	}
	sort.Strings(names)

	var b strings.Builder
	fmt.Fprintf(&b, "# %s lane — live wire capture (invalid-credential probe)\n\n", lane.cfg.Lane)
	fmt.Fprintf(&b, "Captured %s by `P3/gates/lane-key-ceremony.sh` step 6 (LN-CEREMONY).\n\n", time.Now().UTC().Format(time.RFC3339))
	b.WriteString("This closes the DOCUMENTED-NOT-OBSERVED label on the lane's signal table with ONE\n")
	b.WriteString("real body. The credential sent was the literal constant `" + invalidProbeKey + "`;\n")
	b.WriteString("no valid key material was resolved, held, or transmitted on this path.\n\n")
	fmt.Fprintf(&b, "## Request\n\n- endpoint: `%s`\n- protocol: `%s` (from the lane document's `npm` field `%s`)\n",
		url, lane.proto, lane.cfg.NPM)
	fmt.Fprintf(&b, "- model: `%s`\n- endpoint marker asserted: `%s`\n- headers sent:\n", lane.model, lane.cfg.EndpointMarker)
	for _, s := range sent {
		b.WriteString("  - `" + s + "`\n")
	}
	fmt.Fprintf(&b, "- body sent:\n\n```json\n%s\n```\n\n", string(body))
	fmt.Fprintf(&b, "## Response\n\n- status: `%d %s`\n- elapsed: %s\n- headers (verbatim, all of them):\n\n```\n",
		resp.StatusCode, http.StatusText(resp.StatusCode), elapsed.Round(time.Millisecond))
	for _, k := range names {
		for _, v := range resp.Header[k] {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	b.WriteString("```\n\n")
	if hits := interestingHeaders(names); len(hits) > 0 {
		fmt.Fprintf(&b, "- headers of interest (reset / ratelimit / retry / correlation): `%s`\n\n", strings.Join(hits, "`, `"))
	} else {
		b.WriteString("- headers of interest: NONE. No reset, ratelimit, retry-after or correlation header was returned on this response.\n\n")
	}
	fmt.Fprintf(&b, "- body (verbatim, %d bytes):\n\n```\n%s\n```\n\n", len(respBody), string(respBody))
	b.WriteString("## Reconciliation owed\n\n")
	b.WriteString("The classifier fixtures for this lane are NOT edited by the ceremony. Reconciling this\n")
	b.WriteString("observed body against the lane document's signal rows (shape, code field, message\n")
	b.WriteString("grammar, reset marker) is a FOLLOW-UP PACKET, so the change is reviewed rather than\n")
	b.WriteString("made by the script that took the measurement.\n")

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*outPath, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "   status %d %s in %s\n", resp.StatusCode, http.StatusText(resp.StatusCode), elapsed.Round(time.Millisecond))
	if hits := interestingHeaders(names); len(hits) > 0 {
		fmt.Fprintf(out, "   headers of interest: %s\n", strings.Join(hits, ", "))
	} else {
		fmt.Fprintln(out, "   headers of interest: NONE (no reset/ratelimit/retry-after/correlation header)")
	}
	fmt.Fprintf(out, "   saved: %s\n", *outPath)
	return nil
}

func isCredHeader(k string) bool {
	switch strings.ToLower(k) {
	case "authorization", "x-api-key":
		return true
	}
	return false
}

func interestingHeaders(names []string) []string {
	var hits []string
	for _, n := range names {
		l := strings.ToLower(n)
		if strings.Contains(l, "reset") || strings.Contains(l, "ratelimit") || strings.Contains(l, "rate-limit") ||
			strings.Contains(l, "retry-after") || strings.Contains(l, "request-id") || strings.Contains(l, "log-id") {
			hits = append(hits, n)
		}
	}
	return hits
}

// probeRequest builds ONE minimal request for the lane's protocol. The body is
// the smallest thing the endpoint will answer; the caller decides whether the
// credential in it is real.
func probeRequest(lane laneFacts, key string) (url string, body []byte, hdr map[string]string) {
	hdr = map[string]string{"Content-Type": "application/json"}
	switch lane.proto {
	case "anthropic":
		url = lane.base + "/messages"
		hdr["x-api-key"] = key
		hdr["Authorization"] = "Bearer " + key
		hdr["anthropic-version"] = "2023-06-01"
		body, _ = json.Marshal(map[string]any{
			"model": lane.model, "max_tokens": 1,
			"messages": []map[string]string{{"role": "user", "content": "ping"}},
		})
	default:
		url = lane.base + "/chat/completions"
		hdr["Authorization"] = "Bearer " + key
		body, _ = json.Marshal(map[string]any{
			"model": lane.model, "max_tokens": 1, "stream": false,
			"messages": []map[string]string{{"role": "user", "content": "ping"}},
		})
	}
	return url, body, hdr
}

// ─── show ───────────────────────────────────────────────────────────────────

func cmdShow(args []string, out io.Writer) error {
	fs := newFlags("show")
	lanePath := fs.String("lane", "", "path to the lane document")
	model := fs.String("model", "", "model id override")
	if err := fs.Parse(args); err != nil {
		return err
	}
	lane, err := loadLane(*lanePath, *model)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "lane=%s\nprovider_id=%s\nsubstrate=%s\nnpm=%s\nprotocol=%s\nbase_url=%s\nendpoint_marker=%s\nprofile=%s\nenv_var=%s\ndefault_model=%s\nverified_on=%s\nsource=%s\n",
		lane.cfg.Lane, lane.cfg.ProviderID, lane.cfg.Substrate, lane.cfg.NPM, lane.proto,
		lane.base, lane.cfg.EndpointMarker, lane.profile, lane.envVar, lane.model,
		lane.cfg.VerifiedOn, lane.cfg.Source)
	ids := make([]string, 0, len(lane.cfg.Models))
	for _, m := range lane.cfg.Models {
		ids = append(ids, m.ID)
	}
	fmt.Fprintf(out, "models=%s\n", strings.Join(ids, ","))
	if lane.cfg.DataPolicy.Constraint != "" {
		fmt.Fprintf(out, "data_policy=%s\ndata_policy_enforced=%t\n", lane.cfg.DataPolicy.Constraint, lane.cfg.DataPolicy.Enforced)
	}
	return nil
}

// ─── small shared helpers ───────────────────────────────────────────────────

func newFlags(name string) *flag.FlagSet {
	return flag.NewFlagSet("lanekey "+name, flag.ContinueOnError)
}

func requireStore(dir, user string) error {
	if dir == "" || user == "" {
		return errors.New("--store-dir and --user are required")
	}
	return nil
}

// readSecretStdin reads the credential from stdin and nowhere else. Only a
// trailing newline is stripped: a key is taken exactly as the operator's
// clipboard held it, minus the newline their Enter added.
func readSecretStdin() ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("read secret from stdin: %w", err)
	}
	b = bytes.TrimRight(b, "\r\n")
	if len(b) == 0 {
		return nil, errors.New("no credential on stdin (empty input) — nothing was written")
	}
	if bytes.ContainsAny(b, " \t") {
		return nil, errors.New("the credential contains a space or tab — that is almost always a paste accident; nothing was written")
	}
	return b, nil
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
