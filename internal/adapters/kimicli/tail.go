package kimicli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The transcript tail. See transcript.go for WHY the usage source is the run's
// own wire.jsonl rather than the stream.
//
// Two properties make this a tail rather than a post-hoc read, and both were
// measured: the file is created and appended within ~2 s of the spawn, well
// before the first turn ends, and each `usage.record` is flushed as its call
// completes. So checkpoints fire per paid call while the run is alive, which is
// what D7 asks for and what makes a mid-run crash lose nothing already spent.

// tailPollInterval is how often the tail looks for new bytes. A structural
// constant: it trades checkpoint latency against wakeups on a file that gets a
// few dozen lines per turn, and it is a mechanic rather than an operator
// choice (the §61 precedent).
const tailPollInterval = 200 * time.Millisecond

// tailUsage follows the run's transcript until stop closes, emitting one Usage
// event per paid model call.
func (s *session) tailUsage(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case <-time.After(tailPollInterval):
			s.drainUsage()
		}
	}
}

// drainUsage reads whatever transcript bytes have appeared since the last call
// and forwards the Usage events they complete.
//
// It is called from the tail goroutine AND once more after cmd.Wait, so the
// final call's record — which the engine may write microseconds after its last
// stdout frame — is never lost. The transcript's own replay guard makes the
// second read safe: a record already emitted is not emitted again, so a re-read
// cannot re-bill a call that was already checkpointed.
func (s *session) drainUsage() {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()

	if s.wireRefused {
		return
	}
	if s.wirePath == "" {
		p, err := s.resolveTranscript()
		if err != nil {
			// A store this adapter cannot identify UNAMBIGUOUSLY is one it
			// refuses to bill from. Silence here would be the failure mode
			// that matters: a forged record billed as a real paid call.
			s.wireRefused = true
			s.a.warnf("kimicli: refusing to read usage for run %s: %v", s.req.RunID, err)
			return
		}
		if p == "" {
			return // the engine has not created its session yet
		}
		s.wirePath = p
		s.mu.Lock()
		// The engine keeps a full replayable record per session, so unlike the
		// opencode substrate the cursor's transcript path is real here and the
		// Driver's copy-aside has something to copy. The engine's own warning
		// stands: nothing may hand-edit this store.
		s.cursor.TranscriptPath = p
		s.mu.Unlock()
	}
	f, err := os.Open(s.wirePath)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(s.wireOffset, io.SeekStart); err != nil {
		return
	}
	if s.tr == nil {
		s.tr = newTranscript(s.a.logf)
	}
	br := bufio.NewReaderSize(f, 64<<10)
	for {
		line, n, skipped, err := readCappedLine(br, scanBufCap)
		// A partial trailing line means the engine is mid-write: leave the
		// offset before it so the next drain re-reads it whole. Advancing over
		// half a JSON object would drop that call's usage silently.
		if err == io.EOF && len(line) > 0 {
			break
		}
		s.wireOffset += int64(n)
		if !skipped && len(line) > 0 {
			for _, ev := range s.tr.feed(line) {
				s.events <- ev
			}
		}
		if err != nil {
			break
		}
	}
}

// resolveTranscript pins the ONE transcript this run may be billed from.
//
// It replaces a lexicographic glob over sessions/, and the difference is not
// cosmetic. The engine's tools run inside the same filesystem namespace as its
// data root, so the run's own work can CREATE directories under sessions/. A
// glob taking the first match let a planted `sessions/aaa_planted/.../wire.jsonl`
// become a real Usage event carrying an attacker-chosen ModelID — which is the
// price-table key. The usage source was forgeable by the thing being metered.
//
// The pin has two independent legs and a cross-check:
//
//  1. On RESUME the engine-reported session id is already known (it is what
//     `--session` names), so the path is computed and nothing is searched.
//  2. On START the id is not known until the stream's LAST frame, so the path
//     comes from the engine's OWN index, `session_index.jsonl`, filtered to
//     records whose workDir is this run's cwd. The engine writes that record
//     when it creates the session — before the first model call, and therefore
//     before any tool could run and plant a rival. More than one match is
//     REFUSED rather than resolved: an ambiguous store is one where something
//     other than the engine has been writing, and the safe answer is to bill
//     nothing and say so.
//  3. When the stream finally reports its session id, confirmSession compares
//     it with the pinned one and stops billing on a mismatch.
func (s *session) resolveTranscript() (string, error) {
	if id := s.pinnedSessionID(); id != "" {
		return s.transcriptFor(id)
	}
	// The stream may report its session id BEFORE the store is resolvable. That
	// id does not become the pin on its own: if the engine's own index then
	// names a different session, the two disagree and neither can be trusted —
	// which is the same refusal as below, reached in the other order.
	reported := s.reportedSessionID()
	idx := filepath.Join(s.low.home, "session_index.jsonl")
	raw, err := os.ReadFile(idx)
	if err != nil {
		return "", nil // not created yet; not an error
	}
	cwd := s.req.Cwd
	var hits []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			SessionID  string `json:"sessionId"`
			SessionDir string `json:"sessionDir"`
			WorkDir    string `json:"workDir"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec.SessionID == "" || !sameDir(rec.WorkDir, cwd) {
			continue
		}
		if !alreadyHave(hits, rec.SessionID) {
			hits = append(hits, rec.SessionID)
		}
	}
	switch len(hits) {
	case 0:
		return "", nil
	case 1:
		if reported != "" && reported != hits[0] {
			return "", fmt.Errorf("the engine reported session %q but its own index names %q for this run's cwd — "+
				"the two disagree, so neither identifies a store that can be trusted as a paid call", reported, hits[0])
		}
		s.mu.Lock()
		s.pinnedSession = hits[0]
		s.mu.Unlock()
		return s.transcriptFor(hits[0])
	default:
		return "", fmt.Errorf("session_index.jsonl names %d sessions for this run's cwd (%v) — a per-run engine "+
			"home must hold exactly one, so something other than the engine has written to the store and no "+
			"record in it can be trusted as a paid call", len(hits), hits)
	}
}

// transcriptFor is the path a session id resolves to. The workDirKey bucket is
// not enumerated: the id is unique within the home, so one glob segment is
// enough and the LEAF is exact.
//
// AMBIGUITY IS AN ERROR HERE, exactly as it is on the index leg, and the
// asymmetry that used to exist between them was a real hole. Returning "not
// found" for two directories sharing the pinned id meant drainUsage simply
// returned: no usage, no refusal flag, no warning, forever — a SILENT billing
// stall that the run's own work could trigger, since it knows KIMI_CODE_HOME
// and can read session_index.jsonl. Round 1 closed over-billing and opened
// under-billing in the same shape. Both legs now fail closed and LOUD.
//
// A zero-match is the one benign case and stays quiet: the engine has not
// created its session directory yet, and the tail simply polls again.
func (s *session) transcriptFor(sessionID string) (string, error) {
	pattern := filepath.Join(s.low.home, "sessions", "*", sessionID, "agents", "main", "wire.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("resolving the transcript for session %q: %w", sessionID, err)
	}
	switch len(matches) {
	case 0:
		return "", nil // not created yet
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("session %q resolves to %d transcript directories under different workDirKey buckets "+
			"(%v) — a per-run engine home must hold exactly one, so something other than the engine has written to "+
			"the store and no record in it can be trusted as a paid call", sessionID, len(matches), matches)
	}
}

func (s *session) pinnedSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pinnedSession != "" {
		return s.pinnedSession
	}
	// On a resume the id is not inferred at all: it is the one `--session`
	// names, which the platform took from its own park record.
	if s.low.resume {
		return s.low.sessionID
	}
	return ""
}

func (s *session) reportedSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reportedSession
}

// confirmSession is the cross-check: the engine's own reported id against the
// one this run has been billing from. A mismatch means the pin was wrong, so
// billing stops rather than continuing on a store that is not this session's.
// It records the reported id rather than adopting it as the pin. Adopting it
// would silently retarget the tail at a store the index never named — zero
// usage, no refusal, no warning — which is the same silent stall the ambiguity
// legs exist to prevent, reached by a different route.
func (s *session) confirmSession(reported string) {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()
	s.mu.Lock()
	s.reportedSession = reported
	pinned := s.pinnedSession
	if pinned == "" && s.low.resume {
		pinned = s.low.sessionID
	}
	s.mu.Unlock()
	if pinned == "" || pinned == reported {
		return
	}
	s.wireRefused = true
	s.a.warnf("kimicli: run %s billed from session %q but the engine reported %q — refusing further usage from "+
		"a store that is not this session's", s.req.RunID, pinned, reported)
}

func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		ra = filepath.Clean(a)
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		rb = filepath.Clean(b)
	}
	return ra == rb
}

func alreadyHave(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
