package kimicli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// Adapter is the kimi-cli implementation of the D3 contract (Spec S03.2 as
// amended by A12): one spawned `kimi -p` process per invocation, its own
// process group, its own KIMI_CODE_HOME, stream-json on stdout.
//
// Unlike the opencode substrate this one is genuinely per-RUN, so the D3
// contract's shape and the engine's shape agree: StartRequest.Confiner is
// HONORED rather than refused, and the cancel ladder's group rung ends this run
// and no other.
type Adapter struct {
	// Binary is the engine executable (path or PATH name). Empty =
	// DefaultBinary.
	Binary string

	// Root is the per-user engine root base — <stateDir>/engines/kimi-cli.
	// Each person gets a 0700 subtree and each run its own home beneath it.
	Root string

	// BaseURL and ProviderType come from the lane DOCUMENT, wired at the
	// composition root. They are lowering, not credential: they carry no
	// secret. They are fields rather than constants because no endpoint and no
	// model id may be a Go constant in a non-test file (§62) — those facts move
	// and a constant goes stale invisibly where a dated row goes stale visibly.
	BaseURL      string
	ProviderType string

	// Signals is the lane document's wire-signal extractor, wired at the
	// composition root from the kimi-cli document's ExtractSignal +
	// MarshalJSON. The PAYLOAD is the contract; no shared type crosses this
	// seam and this package holds no opinion about classification. Nil
	// forwards raw facts with no documented class — the honest degrade.
	Signals func(bodyText string, httpStatus int) (json.RawMessage, bool)

	// Env overrides the ambient environment as the lowering input base (tests).
	Env []string

	// Now is the clock seam. Nil = time.Now.
	Now func() time.Time

	// Log receives forward-tolerance skip notices and lifecycle noise.
	Log *slog.Logger

	// CancelGrace overrides the TERM→KILL grace (tests).
	CancelGrace time.Duration
}

var _ adapters.Adapter = (*Adapter)(nil)

// Substrate implements adapters.Adapter.
func (a *Adapter) Substrate() string { return adapters.SubstrateKimiCLI }

func (a *Adapter) binary() string {
	if a.Binary != "" {
		return a.Binary
	}
	return DefaultBinary
}

func (a *Adapter) providerType() string {
	if a.ProviderType != "" {
		return a.ProviderType
	}
	return "openai"
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

func (a *Adapter) warnf(format string, args ...any) {
	l := a.Log
	if l == nil {
		l = slog.Default()
	}
	l.Warn(fmt.Sprintf(format, args...))
}

// Start implements adapters.Adapter (S03.1 start).
//
// The control plane CANNOT choose the session id on this engine: there is no
// --session-id flag, ids are server-generated, and the cursor records the id AS
// REPORTED by the engine. Nothing here fabricates one.
func (a *Adapter) Start(ctx context.Context, req adapters.StartRequest) (adapters.Session, error) {
	l, err := a.lower(req, nil)
	if err != nil {
		return nil, err
	}
	return a.spawn(ctx, req, l)
}

// Resume implements adapters.Adapter (S03.1 resume; S03.4 obligation): the
// ENTIRE invocation is re-supplied from the park record.
//
// On this engine "re-supply" is more literal than elsewhere. The invocation's
// whole config lives inside the run's own KIMI_CODE_HOME, so the home is
// REBUILT and re-verified rather than assumed intact — a resume that trusted a
// home somebody had edited would run under configuration nobody re-checked.
func (a *Adapter) Resume(ctx context.Context, rec adapters.ParkRecord, ans *adapters.Answer) (adapters.Session, error) {
	if rec.Cursor.SessionID == "" {
		return nil, fmt.Errorf("kimicli: park record without engine session id")
	}
	spec := &resumeSpec{sessionID: rec.Cursor.SessionID}
	if ans != nil {
		spec.continuation = ans.Continuation
		// There is no gate ask on this substrate to answer, so an answer
		// carrying one is a caller error rather than something to ignore: a
		// dropped decision is how a refused call gets executed anyway.
		if ans.AskID != "" {
			return nil, fmt.Errorf("%w: resume carries an answer for ask %q, but this substrate never parks on a gate",
				ErrGateParkUnsupported, ans.AskID)
		}
	}
	l, err := a.lower(rec.Start, spec)
	if err != nil {
		return nil, err
	}
	return a.spawn(ctx, rec.Start, l)
}

// spawn materializes the lowered invocation and starts the engine as its own
// process-group leader.
func (a *Adapter) spawn(ctx context.Context, req adapters.StartRequest, l *lowered) (adapters.Session, error) {
	if st, err := os.Stat(req.Cwd); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("kimicli: run cwd %q is not a directory", req.Cwd)
	}
	if err := a.assertCleanCwd(req.Cwd); err != nil {
		return nil, err
	}
	if err := a.materialize(l); err != nil {
		return nil, err
	}

	// Broker credential injection at spawn (S11.5): resolved FRESH, never
	// stored. The variable is the lane document's — KIMI_MODEL_API_KEY, which
	// is NOT the API lane's KIMI_API_KEY: a shell KIMI_API_KEY does not
	// authenticate this CLI at all, and getting it wrong produces a startup
	// failure that looks like a broken lane.
	env := l.env
	if req.CredInject != nil {
		injected, err := req.CredInject(env)
		if err != nil {
			return nil, fmt.Errorf("kimicli: inject credentials (S11.5): %w", err)
		}
		env = injected
	}

	cmd, cleanup, err := a.buildCmd(req, l, env)
	if err != nil {
		return nil, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("kimicli: stdout pipe: %w", err)
	}
	// stderr is captured SEPARATELY and never merged. Merging would corrupt
	// the JSONL — the engine puts thinking, tool progress and "resuming
	// session" notices there — and it is also the only place a terminal error
	// message appears, so it is kept for the outcome rather than discarded.
	stderr := &boundedBuffer{cap: stderrCap}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("kimicli: spawn %s: %w", cmd.Path, err)
	}
	cleanup()

	s := &session{
		a: a, req: req, low: l, cmd: cmd, stderr: stderr,
		events: make(chan adapters.Event, 64),
		done:   make(chan struct{}),
		// Before the first message nothing is mid-flight: interrupting here
		// parks with zero progress, which is safe (P-T01-4).
		atBoundary: true,
	}
	s.cursor = adapters.Cursor{
		Substrate: adapters.SubstrateKimiCLI,
		SessionID: l.sessionID, // empty on start; the REPORTED id wins (S02.4b)
		CwdKey:    req.Cwd,
	}
	go s.pump(stdout)
	return s, nil
}

// buildCmd builds the engine *exec.Cmd — confined through the S11 Confiner when
// one is set, or the unconfined dev spawn otherwise (CONVENTIONS §10).
func (a *Adapter) buildCmd(req adapters.StartRequest, l *lowered, env []string) (*exec.Cmd, func(), error) {
	noop := func() {}
	if req.Confiner == nil {
		cmd := exec.Command(l.argv[0], l.argv[1:]...)
		cmd.Dir = req.Cwd
		cmd.Env = env
		return cmd, noop, nil
	}
	// The run's own home is READ-WRITE, not read-only: unlike the Anthropic
	// lane's settings file, this engine's data root is also where it writes
	// its session store — and that store is the platform's usage source.
	spec := adapters.SpawnSpec{
		Argv:       l.argv,
		Env:        env,
		Workspace:  req.Cwd,
		RWExchange: []string{l.home, l.bounded},
		ROConfig:   []string{l.skillsDir},
		// A global npm CLI lives outside /usr and must be reachable inside the
		// sandbox — and so must Node ≥22.19, which the engine needs to run at
		// all.
		EnginePrefix: enginePrefix(a.binary()),
	}
	cmd, cleanup, err := req.Confiner.Confine(req, spec)
	if err != nil {
		return nil, noop, fmt.Errorf("kimicli: confine (S11): %w", err)
	}
	if cleanup == nil {
		cleanup = noop
	}
	return cmd, cleanup, nil
}

// enginePrefix reports the tree that must be bound into the sandbox for a
// binary installed outside the system path. Empty for a PATH name.
func enginePrefix(binary string) string {
	if !filepath.IsAbs(binary) {
		return ""
	}
	return filepath.Dir(filepath.Dir(binary))
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
	atBoundary     bool
	cancelStage    int

	// The transcript tail's own state. It has its own mutex because the tail
	// runs concurrently with the stdout pump and the two share nothing but the
	// events channel and the cursor.
	usageMu    sync.Mutex
	tr         *transcript
	wirePath   string
	wireOffset int64
}

var _ adapters.Session = (*session)(nil)

func (s *session) Events() <-chan adapters.Event { return s.events }

func (s *session) Cursor() adapters.Cursor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cursor
}

func (s *session) Fingerprint() string { return s.low.fingerprint }

// Pause implements the S03.1 pause verb: interrupt at a completed message
// boundary, never mid-tool-call (P-T01-4).
func (s *session) Pause(ctx context.Context) error {
	s.mu.Lock()
	s.pauseRequested = true
	if s.atBoundary && s.cancelStage == 0 {
		s.signalGroup(syscall.SIGTERM)
	}
	s.mu.Unlock()
	return nil
}

// Cancel implements the S03.1 cancel ladder. There is no cooperative abort on
// the `-p` surface, so the ladder is boundary → group TERM → group KILL. The
// spike measured a clean group TERM: exit 143, zero survivors in the group.
func (s *session) Cancel(ctx context.Context) error {
	grace := s.a.CancelGrace
	if grace <= 0 {
		grace = cancelGrace * time.Second
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

// signalGroup signals the engine's process group. Callers hold s.mu.
func (s *session) signalGroup(sig syscall.Signal) {
	if s.cmd.Process == nil {
		return
	}
	pid := s.cmd.Process.Pid
	if err := syscall.Kill(-pid, sig); err != nil {
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

// pump reads stdout to EOF while a sibling goroutine tails the run's own
// transcript for usage, then assembles the Outcome from the exit.
func (s *session) pump(stdout io.Reader) {
	p := newParser(s.a.logf)
	p.signals = s.a.Signals
	p.onSession = func(id string) {
		s.mu.Lock()
		s.cursor.SessionID = id
		s.mu.Unlock()
	}
	p.onFlush = func() {
		s.mu.Lock()
		s.atBoundary = true
		if s.pauseRequested && s.cancelStage == 0 {
			s.signalGroup(syscall.SIGTERM)
		}
		s.mu.Unlock()
	}

	// The usage tail runs for the life of the stream. It stops when stdout
	// closes, then drains once more so the final call's record — which the
	// engine may write microseconds after the last frame — is not lost.
	tailDone := make(chan struct{})
	go s.tailUsage(tailDone)

	br := bufio.NewReaderSize(stdout, 64<<10)
	for {
		line, n, skipped, err := readCappedLine(br, scanBufCap)
		switch {
		case skipped:
			s.a.warnf("kimicli: skipping oversized stream line: %d bytes exceeds the %d-byte line cap", n, scanBufCap)
		case len(line) > 0:
			for _, ev := range p.feed(line) {
				s.events <- ev
			}
		}
		if err != nil {
			if err != io.EOF {
				s.a.logf("kimicli: stdout read: %v", err)
			}
			break
		}
	}
	close(tailDone)
	waitErr := s.cmd.Wait()
	s.drainUsage()

	out := s.assembleOutcome(p, waitErr)
	s.mu.Lock()
	s.outcome = out
	s.mu.Unlock()
	close(s.events)
	close(s.done)
}

// assembleOutcome maps {exit, requested pause/cancel, stream facts} onto the
// contract Outcome.
//
// Three contract shapes are structurally unreachable here and NONE is faked
// (the §61 posture):
//
//   - Outcome.Ask and the gate park: this substrate refuses a gated-tool
//     worker outright, so no ask can ever be observed.
//   - Outcome.GateFallback: it reports the Anthropic lane's single-tool-call
//     defer trap, which has no analogue because there is no defer.
//   - Outcome.Totals: there is no terminal envelope on this transport to carry
//     a run total, and inventing one from the per-call rows would present a
//     derived figure as a reported one.
func (s *session) assembleOutcome(p *parser, waitErr error) adapters.Outcome {
	s.mu.Lock()
	paused := s.pauseRequested
	canceled := s.cancelStage > 0
	cur := s.cursor
	s.mu.Unlock()

	out := adapters.Outcome{ResultText: p.finalText}
	switch {
	case canceled:
		out.Kind = adapters.OutcomeCanceled
		out.Detail = "cancel ladder (S03.1)"
	case paused:
		out.Kind = adapters.OutcomeParked
		out.Park = s.parkRecord(cur)
		out.Detail = "interrupt at a completed message boundary (S03.1 pause)"
	case waitErr == nil:
		out.Kind = adapters.OutcomeCompleted
	default:
		// Exit code 1 is NOT diagnostic on this engine: auth failure, quota
		// exhaustion and overload all produce it, and the only description is
		// the stderr line. So the disposition is crashed and the detail is the
		// engine's own words rather than a classification this adapter is not
		// entitled to make (D4).
		out.Kind = adapters.OutcomeCrashed
		out.Detail = s.crashDetail(waitErr)
	}
	return out
}

// crashDetail reports the engine's own terminal message. On this transport it
// exists ONLY on stderr — a provider 403 produced no stdout frame at all — so
// a crash with an empty detail would discard the single fact worth having.
func (s *session) crashDetail(waitErr error) string {
	text := s.stderr.String()
	if text == "" {
		return waitErr.Error()
	}
	excerpt, _ := excerptOf(text)
	return excerpt
}

func (s *session) parkRecord(cur adapters.Cursor) *adapters.ParkRecord {
	return &adapters.ParkRecord{
		RunID:       s.req.RunID,
		Substrate:   adapters.SubstrateKimiCLI,
		Cursor:      cur,
		Reason:      adapters.ParkReasonPause,
		Start:       s.req,
		Fingerprint: s.low.fingerprint,
		ParkedAt:    s.a.now().UTC(),
	}
}

// readCappedLine reads one newline-terminated line bounded by limit. An
// oversized line is one more skipped-line class, never a wedge: it is discarded
// through its newline and the read CONTINUES to EOF, so the tail still arrives
// and the child never blocks on a stdout nobody is draining.
func readCappedLine(r *bufio.Reader, limit int) (line []byte, n int, skipped bool, err error) {
	for {
		chunk, rerr := r.ReadSlice('\n')
		n += len(chunk)
		if !skipped {
			if len(line)+len(chunk) > limit {
				line, skipped = nil, true
			} else {
				line = append(line, chunk...)
			}
		}
		if rerr == bufio.ErrBufferFull {
			continue
		}
		if rerr != nil {
			if rerr == io.EOF && n == 0 {
				return nil, 0, false, io.EOF
			}
			return trimLineEnd(line), n, skipped, rerr
		}
		return trimLineEnd(line), n, skipped, nil
	}
}

func trimLineEnd(line []byte) []byte {
	if i := len(line) - 1; i >= 0 && line[i] == '\n' {
		line = line[:i]
	}
	if i := len(line) - 1; i >= 0 && line[i] == '\r' {
		line = line[:i]
	}
	return line
}

// boundedBuffer captures stderr up to a cap without unbounded growth.
type boundedBuffer struct {
	mu  sync.Mutex
	cap int
	buf []byte
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.cap - len(b.buf); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		b.buf = append(b.buf, p[:room]...)
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
