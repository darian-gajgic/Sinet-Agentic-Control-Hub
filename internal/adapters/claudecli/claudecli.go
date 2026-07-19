package claudecli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
)

// Settings is the narrow ⚙ reader this adapter needs (CONVENTIONS §2:
// consumers read the registry by dotted key through per-package
// interfaces). *settings.Registry satisfies it.
type Settings interface {
	Int(key string) (int64, error)
	Float(key string) (float64, error)
	String(key string) (string, error)
}

// DefaultBinary is the engine binary name resolved via PATH when the
// adapter is not configured with an explicit path.
const DefaultBinary = "claude"

// cancelGrace is the TERM→KILL grace of the cancel ladder (S03.1:
// process-group TERM→KILL). Not a ⚙ — S18 ratifies no such key (the
// sseBatchSize precedent, CONVENTIONS §7).
const cancelGrace = 5 * time.Second

// scanBufCap bounds one stdout JSONL line (stream frames carry bounded
// message parts; 10 MiB is generous headroom). Plain constant, same
// precedent as cancelGrace.
const scanBufCap = 10 << 20

// Adapter is the Anthropic-lane implementation of the D3 contract
// (Spec S03.2): one spawned `claude -p` process per invocation, its own
// process group, stream-json on stdout, all platform opinion this side.
type Adapter struct {
	// Binary is the engine executable (path or PATH name). Empty =
	// DefaultBinary. The binary is the pinned engine of components.lock;
	// the conformance suite reports any installed-version delta
	// (S03.3: the pin is the contract).
	Binary string

	// HookCmd is the gate-hook command prefix compiled into lowered
	// settings (S03.4). Empty = "<this executable> engine-hook" — in the
	// running platform that is the sinet binary itself.
	HookCmd string

	// Settings reads the three ⚙ keys of the S03 domain.
	Settings Settings

	// Env overrides the ambient environment as the lowering input base
	// (tests). Nil = os.Environ().
	Env []string

	// Now is the clock seam. Nil = time.Now.
	Now func() time.Time

	// Log receives forward-tolerance skip notices and lifecycle noise.
	// Nil = slog.Default (ops log, S01.11 posture).
	Log *slog.Logger

	// CancelGrace overrides the TERM→KILL grace (tests). 0 = cancelGrace.
	CancelGrace time.Duration
}

var _ adapters.Adapter = (*Adapter)(nil)

// Substrate implements adapters.Adapter.
func (a *Adapter) Substrate() string { return adapters.SubstrateClaudeCLI }

func (a *Adapter) binary() string {
	if a.Binary != "" {
		return a.Binary
	}
	return DefaultBinary
}

func (a *Adapter) hookCommand() string {
	if a.HookCmd != "" {
		return a.HookCmd
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe + " engine-hook"
}

func (a *Adapter) environ() []string {
	if a.Env != nil {
		return a.Env
	}
	return os.Environ()
}

func (a *Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *Adapter) logf(format string, args ...any) {
	l := a.Log
	if l == nil {
		l = slog.Default()
	}
	l.Info(fmt.Sprintf(format, args...))
}

// Start implements adapters.Adapter (S03.1 start): spawn with a
// control-plane-chosen session id and fully lowered config.
func (a *Adapter) Start(ctx context.Context, req adapters.StartRequest) (adapters.Session, error) {
	l, err := a.lower(req, nil, newSessionID())
	if err != nil {
		return nil, err
	}
	return a.spawn(ctx, req, l, req.Worker.Prompt)
}

// Resume implements adapters.Adapter (S03.1 resume; S03.4 obligation):
// the ENTIRE invocation is re-supplied from the park record — settings
// content, permission mode, model, tool allowlist. The engine restores
// nothing. A gate answer is staged into the control dir so the re-fired
// PreToolUse hook returns allow (+updatedInput) on the same tool_use_id.
func (a *Adapter) Resume(ctx context.Context, rec adapters.ParkRecord, ans *adapters.Answer) (adapters.Session, error) {
	if rec.Cursor.SessionID == "" {
		return nil, fmt.Errorf("claudecli: park record without engine session id")
	}
	l, err := a.lower(rec.Start, &resumeSpec{sessionID: rec.Cursor.SessionID}, "")
	if err != nil {
		return nil, err
	}
	if ans != nil && ans.AskID != "" {
		if err := writeAnswer(l.ctlDir, ans.AskID, ans.UpdatedInput); err != nil {
			return nil, fmt.Errorf("claudecli: stage answer: %w", err)
		}
	}
	prompt := ""
	if ans != nil {
		prompt = ans.Continuation // pause-park resumes; a defer resume needs no prompt (SPIKE G1-S2 F2)
	}
	return a.spawn(ctx, rec.Start, l, prompt)
}

// spawn materializes the lowered invocation: config on disk (WorkDir
// only, never Cwd/shared dirs — S03.5), the engine as its own process
// group leader (S03.1 cancel), and the pump goroutine.
func (a *Adapter) spawn(ctx context.Context, req adapters.StartRequest, l *lowered, prompt string) (adapters.Session, error) {
	if err := os.MkdirAll(req.WorkDir, 0o700); err != nil {
		return nil, fmt.Errorf("claudecli: workdir: %w", err)
	}
	if st, err := os.Stat(req.Cwd); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("claudecli: run cwd %q is not a directory", req.Cwd)
	}
	if err := os.WriteFile(l.settingsPath, l.settingsJSON, 0o600); err != nil {
		return nil, fmt.Errorf("claudecli: write lowered settings: %w", err)
	}
	if err := writeCtlConfig(l.ctlDir, l.gateFallback); err != nil {
		return nil, fmt.Errorf("claudecli: gate ctl: %w", err)
	}
	if err := truncateFires(l.ctlDir); err != nil {
		return nil, fmt.Errorf("claudecli: reset fires log: %w", err)
	}

	// Broker credential injection at spawn (Spec S11.5; S01.6 "engines
	// receive credentials at start"): resolved FRESH, never stored. For a
	// confined engine the injected model-channel value is a sentinel; the
	// real token rides the injection proxy (D2/S11.5).
	env := l.env
	if req.CredInject != nil {
		injected, err := req.CredInject(env)
		if err != nil {
			return nil, fmt.Errorf("claudecli: inject credentials (S11.5): %w", err)
		}
		env = injected
	}

	// Confinement seam (Spec S11): with a Confiner set the engine runs inside
	// the composed per-run sandbox; without one it spawns unconfined (the
	// B1-1 dev posture, CONVENTIONS §10). Either way the process is its own
	// group leader so the S03.1 cancel ladder can signal the whole tree.
	cmd, cleanup, err := a.buildCmd(req, l, env)
	if err != nil {
		return nil, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("claudecli: stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("claudecli: stdin pipe: %w", err)
	}
	stderr := &boundedBuffer{cap: adapters.ExcerptCap}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("claudecli: spawn %s: %w", cmd.Path, err)
	}
	// The child holds its own copy of any passed fd (seccomp); the parent's
	// is disposable once bwrap has forked.
	cleanup()

	s := &session{
		a: a, req: req, low: l, cmd: cmd, stderr: stderr,
		events: make(chan adapters.Event, 64),
		done:   make(chan struct{}),
		// Before the first message nothing is mid-flight: interrupting
		// here parks with zero progress, which is safe (P-T01-4).
		atBoundary: true,
	}
	s.cursor = adapters.Cursor{
		Substrate: adapters.SubstrateClaudeCLI,
		SessionID: l.sessionID, // provisional; the REPORTED id from init wins (S02.4b)
		CwdKey:    req.Cwd,
	}

	// Prompt delivery: one user message over --input-format stream-json,
	// then EOF — the one-shot drive shape of B1 (multi-turn interactive
	// driving is orchestration's, Spec S04).
	go func() {
		defer stdin.Close()
		if prompt == "" {
			return // defer-resume: no prompt at all (SPIKE G1-S2 F2)
		}
		msg, err := json.Marshal(struct {
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}{Type: "user", Message: struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}{Role: "user", Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{{Type: "text", Text: prompt}}}})
		if err != nil {
			a.logf("claudecli: marshal prompt: %v", err)
			return
		}
		if _, err := stdin.Write(append(msg, '\n')); err != nil {
			a.logf("claudecli: write prompt: %v", err)
		}
	}()

	go s.pump(stdout)
	return s, nil
}

// buildCmd builds the engine *exec.Cmd — confined through the S11 Confiner
// when one is set, or the unconfined B1-1 dev spawn otherwise (CONVENTIONS
// §10). It always returns a non-nil cleanup (the confined path's seccomp fd
// close). SysProcAttr/pipes/Start remain the caller's (spawn), so the cancel
// ladder and stream plumbing are identical for confined and unconfined runs.
func (a *Adapter) buildCmd(req adapters.StartRequest, l *lowered, env []string) (*exec.Cmd, func(), error) {
	noop := func() {}
	if req.Confiner == nil {
		cmd := exec.Command(l.argv[0], l.argv[1:]...)
		cmd.Dir = req.Cwd
		cmd.Env = env
		return cmd, noop, nil
	}
	// Lowered config is bound read-only (the engine reads its compiled
	// settings but can never rewrite them — S11.7 P-T09-1); the gate control
	// dir, when gated tools are declared, is the read-write exchange channel
	// (S03.4). A later rw bind of the ctl subdir shadows the ro WorkDir bind.
	spec := adapters.SpawnSpec{
		Argv:      l.argv,
		Env:       env,
		Workspace: req.Cwd,
		ROConfig:  []string{req.WorkDir},
	}
	if len(req.Worker.GatedTools) > 0 {
		spec.RWExchange = []string{l.ctlDir}
	}
	cmd, cleanup, err := req.Confiner.Confine(req, spec)
	if err != nil {
		return nil, noop, fmt.Errorf("claudecli: confine (S11): %w", err)
	}
	if cleanup == nil {
		cleanup = noop
	}
	return cmd, cleanup, nil
}

// session is one live engine invocation.
type session struct {
	a      *Adapter
	req    adapters.StartRequest
	low    *lowered
	cmd    *exec.Cmd
	stderr *boundedBuffer

	events chan adapters.Event
	done   chan struct{}

	mu             sync.Mutex
	cursor         adapters.Cursor
	outcome        adapters.Outcome
	waitErr        error
	pauseRequested bool
	atBoundary     bool // between completed messages — safe to interrupt (P-T01-4)
	cancelStage    int  // 0 none, 1 TERM sent, 2 KILL sent
}

var _ adapters.Session = (*session)(nil)

func (s *session) Events() <-chan adapters.Event { return s.events }

func (s *session) Cursor() adapters.Cursor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cursor
}

func (s *session) Fingerprint() string { return s.low.fingerprint }

// Pause implements the S03.1 pause verb: interrupt-at-safe-boundary +
// park. The boundary is a completed message (never mid-tool-call,
// P-T01-4): if the session is at one now the interrupt lands
// immediately, otherwise on the next flush. The park record arrives via
// Wait.
func (s *session) Pause(ctx context.Context) error {
	s.mu.Lock()
	s.pauseRequested = true
	if s.atBoundary && s.cancelStage == 0 {
		s.signalGroup(syscall.SIGTERM)
	}
	s.mu.Unlock()
	return nil
}

// Cancel implements the S03.1 cancel ladder: process-group TERM, then
// KILL after cancelGrace. (The engine has no gentler abort on the -p
// surface; boundary-preference is Pause's job.)
func (s *session) Cancel(ctx context.Context) error {
	grace := s.a.CancelGrace
	if grace <= 0 {
		grace = cancelGrace
	}
	s.mu.Lock()
	if s.cancelStage == 0 {
		s.cancelStage = 1
		s.signalGroup(syscall.SIGTERM)
		go func() {
			select {
			case <-s.done:
			case <-time.After(grace):
				s.mu.Lock()
				s.cancelStage = 2
				s.signalGroup(syscall.SIGKILL)
				s.mu.Unlock()
			}
		}()
	}
	s.mu.Unlock()
	return nil
}

// signalGroup signals the engine's process group (spawned as its own
// group leader). Callers hold s.mu.
func (s *session) signalGroup(sig syscall.Signal) {
	if s.cmd.Process == nil {
		return
	}
	pid := s.cmd.Process.Pid
	if err := syscall.Kill(-pid, sig); err != nil {
		// Group gone or never formed: fall back to the process itself.
		_ = s.cmd.Process.Signal(sig)
	}
}

func (s *session) Wait(ctx context.Context) (adapters.Outcome, error) {
	select {
	case <-ctx.Done():
		return adapters.Outcome{}, ctx.Err()
	case <-s.done:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outcome, s.waitErr
}

// pump reads stdout to EOF, forwards contract events, then assembles the
// Outcome from the terminal envelope + process exit.
func (s *session) pump(stdout interface{ Read([]byte) (int, error) }) {
	p := &parser{
		logf: s.a.logf,
		onInit: func(sessionID, model, permissionMode, cwd, transcriptPath string) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if sessionID != "" {
				s.cursor.SessionID = sessionID // the REPORTED id (S02.4b)
			}
			if cwd != "" {
				s.cursor.CwdKey = cwd
			}
			if transcriptPath != "" {
				s.cursor.TranscriptPath = transcriptPath
			}
		},
		onTurn: func() {
			// New assistant turn: not a safe boundary anymore, and the
			// gate fire window resets (S03.4 serialize-by-deny
			// mechanics; heuristic pending the B1-4 live reconfirm).
			s.mu.Lock()
			s.atBoundary = false
			s.mu.Unlock()
			if err := truncateFires(s.low.ctlDir); err != nil {
				s.a.logf("claudecli: reset fires log: %v", err)
			}
		},
	}
	p.onFlush = func(index int64) {
		s.mu.Lock()
		s.cursor.MessageIndex = index
		if s.cursor.TranscriptPath == "" {
			if tp := s.deriveTranscriptPath(); tp != "" {
				s.cursor.TranscriptPath = tp
			}
		}
		s.atBoundary = true
		if s.pauseRequested && s.cancelStage == 0 {
			// Safe boundary reached with a pause pending: end the
			// process; outcome = parked (S03.1 pause).
			s.signalGroup(syscall.SIGTERM)
		}
		s.mu.Unlock()
	}

	scan := bufio.NewScanner(stdout)
	scan.Buffer(make([]byte, 64<<10), scanBufCap)
	for scan.Scan() {
		for _, ev := range p.feed(scan.Bytes()) {
			s.events <- ev
		}
	}
	if err := scan.Err(); err != nil {
		s.a.logf("claudecli: stdout read: %v", err)
	}
	waitErr := s.cmd.Wait()

	out, extraEvents := s.assembleOutcome(p, waitErr)
	for _, ev := range extraEvents {
		s.events <- ev
	}
	s.mu.Lock()
	s.outcome = out
	s.mu.Unlock()
	close(s.events)
	close(s.done)
}

// assembleOutcome maps {terminal envelope, exit, requested pause/cancel,
// gate fires} onto the contract Outcome (S03.4 shapes, recorded at the
// pin: defer exit → parked; error_max_budget_usd after a gate fire →
// died-at-gate; anything else abnormal → crashed).
func (s *session) assembleOutcome(p *parser, waitErr error) (adapters.Outcome, []adapters.Event) {
	s.mu.Lock()
	cur := s.cursor
	paused := s.pauseRequested
	canceled := s.cancelStage > 0
	s.mu.Unlock()

	fires, _, err := countFires(s.low.ctlDir)
	if err != nil {
		s.a.logf("claudecli: read fires log: %v", err)
	}

	switch {
	case canceled && !p.sawResult:
		// A cancel that raced a completed result loses: the engine
		// finished the work, the result stands.
		return adapters.Outcome{Kind: adapters.OutcomeCanceled, Detail: "cancel ladder (S03.1)"}, nil

	case p.sawResult && p.result.StopReason == "tool_deferred":
		// Exit-park (S03.4): build the ask record from the exit JSON
		// alone (SPIKE G1-S2 F1).
		d, err := parseDeferred(p.result.DeferredToolUse)
		if err != nil {
			return adapters.Outcome{Kind: adapters.OutcomeCrashed, Detail: err.Error()}, nil
		}
		rec := s.parkRecord(cur, adapters.ParkReasonGateAsk, p.result.DeferredToolUse)
		ask := &adapters.Ask{
			ID: d.ID, ToolName: d.Name, ToolInput: d.Input,
			EngineExpiry: s.a.now().Add(time.Duration(s.low.cleanupDays) * 24 * time.Hour),
			Park:         rec,
		}
		askPayload, _ := json.Marshal(struct {
			AskID        string          `json:"ask_id"`
			ToolName     string          `json:"tool_name"`
			ToolInput    json.RawMessage `json:"tool_input,omitempty"`
			EngineExpiry string          `json:"engine_expiry"`
		}{d.ID, d.Name, bounded(d.Input), ask.EngineExpiry.UTC().Format(time.RFC3339Nano)})
		return adapters.Outcome{
				Kind: adapters.OutcomeParked, Park: rec, Ask: ask,
				Result: bounded(rawResult(p.result)),
			}, []adapters.Event{
				{Kind: adapters.KindGateAsk, Payload: askPayload, Ask: ask},
				{Kind: adapters.KindDone, Payload: donePayload(p.result, false)},
			}

	case p.sawResult && p.result.Subtype == "error_max_budget_usd" && fires > 0:
		// The engine ceiling pre-empted the park after the gate fired
		// (SPIKE G1-S2 M2/M3: no deferred_tool_use survives) — the
		// platform ask record is the survivor; S02.3 died-at-gate.
		rec := s.parkRecord(cur, adapters.ParkReasonGateAsk, nil)
		return adapters.Outcome{
				Kind: adapters.OutcomeDiedAtGate, Park: rec,
				Detail: "engine budget preempted the park (S03.4 ceiling ordering)",
				Result: bounded(rawResult(p.result)),
			}, []adapters.Event{
				{Kind: adapters.KindDone, Payload: donePayload(p.result, false)},
			}

	case p.sawResult && !p.result.IsError:
		gateFallback := fires > 1 // S03.4 detection: >1 fire, no defer exit
		totals := usageFromRaw(p.result.Usage, "", p.result.SessionID, cur.MessageIndex, true, p.result.TotalCostUSD)
		out := adapters.Outcome{
			Kind: adapters.OutcomeCompleted, Totals: totals,
			Result: bounded(rawResult(p.result)), GateFallback: gateFallback,
		}
		if paused {
			// Pause raced completion: the engine finished first — the
			// result stands; nothing was interrupted.
			out.Detail = "pause requested but engine completed first"
		}
		return out, []adapters.Event{{Kind: adapters.KindDone, Payload: donePayload(p.result, gateFallback)}}

	case p.sawResult:
		return adapters.Outcome{
				Kind:   adapters.OutcomeCrashed,
				Detail: fmt.Sprintf("engine error result: subtype=%s terminal=%s", p.result.Subtype, p.result.TerminalReason),
				Result: bounded(rawResult(p.result)),
			}, []adapters.Event{
				{Kind: adapters.KindDone, Payload: donePayload(p.result, false)},
			}

	case paused:
		// Boundary interrupt landed before any terminal envelope: parked
		// by request (S03.1 pause; no true suspend exists).
		rec := s.parkRecord(cur, adapters.ParkReasonPause, nil)
		return adapters.Outcome{Kind: adapters.OutcomeParked, Park: rec}, nil

	default:
		detail := "engine exited without a result envelope"
		if waitErr != nil {
			detail = fmt.Sprintf("%s: %v", detail, waitErr)
		}
		if tail := s.stderr.String(); tail != "" {
			detail = fmt.Sprintf("%s; stderr: %s", detail, tail)
		}
		return adapters.Outcome{Kind: adapters.OutcomeCrashed, Detail: detail}, nil
	}
}

// parkRecord snapshots the FULL invocation for resume (S03.4 obligation;
// field set verbatim from the measured contract, SPIKE G1-S2 §T07).
func (s *session) parkRecord(cur adapters.Cursor, reason string, deferred json.RawMessage) *adapters.ParkRecord {
	return &adapters.ParkRecord{
		RunID:           s.req.RunID,
		Substrate:       adapters.SubstrateClaudeCLI,
		Cursor:          cur,
		Reason:          reason,
		DeferredToolUse: deferred,
		Start:           s.req,
		Fingerprint:     s.low.fingerprint,
		ParkedAt:        s.a.now(),
	}
}

// deriveTranscriptPath computes the engine's CWD-keyed transcript
// location under the config root when the stream did not announce one.
// The shape is verified BEHAVIORALLY: existence-checked here before use,
// and asserted against the real engine by the conformance suite's
// real-engine subset (S03.3 rule 4 — never trusted from docs). Callers
// hold s.mu.
func (s *session) deriveTranscriptPath() string {
	configDir := s.req.OwnerCredRef
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(home, ".claude")
	}
	if s.cursor.SessionID == "" {
		return ""
	}
	key := projectKey(s.cursor.CwdKey)
	path := filepath.Join(configDir, "projects", key, s.cursor.SessionID+".jsonl")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// projectKey mirrors the engine's cwd→project-dir sanitization (verified
// behaviorally per deriveTranscriptPath's contract).
func projectKey(cwd string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '.', '_', ' ':
			return '-'
		}
		return r
	}, cwd)
}

// rawResult re-marshals the terminal envelope's interesting fields for
// Outcome.Result (bounded cross-check material).
func rawResult(sl streamLine) json.RawMessage {
	raw, err := json.Marshal(struct {
		Subtype        string          `json:"subtype,omitempty"`
		IsError        bool            `json:"is_error,omitempty"`
		Result         string          `json:"result,omitempty"`
		StopReason     string          `json:"stop_reason,omitempty"`
		TerminalReason string          `json:"terminal_reason,omitempty"`
		NumTurns       int64           `json:"num_turns,omitempty"`
		TotalCostUSD   float64         `json:"total_cost_usd,omitempty"`
		SessionID      string          `json:"session_id,omitempty"`
		Usage          json.RawMessage `json:"usage,omitempty"`
		ModelUsage     json.RawMessage `json:"modelUsage,omitempty"`
	}{sl.Subtype, sl.IsError, firstN(sl.Result, adapters.ExcerptCap), sl.StopReason,
		sl.TerminalReason, sl.NumTurns, sl.TotalCostUSD, sl.SessionID,
		boundedOrEmpty(sl.Usage), boundedOrEmpty(sl.ModelUsage)})
	if err != nil {
		return json.RawMessage("{}")
	}
	return raw
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// newSessionID mints the control-plane-chosen session UUID (S03.1 start:
// honored exactly by the engine; the reported id still wins in the
// cursor).
func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is unrecoverable enough to surface loudly;
		// a zero UUID would collide across runs.
		panic(fmt.Sprintf("claudecli: crypto/rand: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// boundedBuffer keeps the tail of stderr for crash diagnostics.
type boundedBuffer struct {
	mu  sync.Mutex
	buf []byte
	cap int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.cap {
		b.buf = b.buf[len(b.buf)-b.cap:]
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.buf))
}
