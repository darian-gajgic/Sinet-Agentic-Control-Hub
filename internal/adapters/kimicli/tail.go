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
		p, ok := s.transcriptFor(id)
		if !ok {
			return "", nil
		}
		return p, nil
	}
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
		s.mu.Lock()
		s.pinnedSession = hits[0]
		s.mu.Unlock()
		p, ok := s.transcriptFor(hits[0])
		if !ok {
			return "", nil
		}
		return p, nil
	default:
		return "", fmt.Errorf("session_index.jsonl names %d sessions for this run's cwd (%v) — a per-run engine "+
			"home must hold exactly one, so something other than the engine has written to the store and no "+
			"record in it can be trusted as a paid call", len(hits), hits)
	}
}

// transcriptFor is the path a session id resolves to. The workDirKey bucket is
// not enumerated: the id is unique within the home, so one glob segment is
// enough and the LEAF is exact.
func (s *session) transcriptFor(sessionID string) (string, bool) {
	matches, err := filepath.Glob(filepath.Join(s.low.home, "sessions", "*", sessionID, "agents", "main", "wire.jsonl"))
	if err != nil || len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

func (s *session) pinnedSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pinnedSession != "" {
		return s.pinnedSession
	}
	if s.low.resume {
		return s.low.sessionID
	}
	return ""
}

// confirmSession is the cross-check: the engine's own reported id against the
// one this run has been billing from. A mismatch means the pin was wrong, so
// billing stops rather than continuing on a store that is not this session's.
func (s *session) confirmSession(reported string) {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()
	s.mu.Lock()
	pinned := s.pinnedSession
	if pinned == "" {
		s.pinnedSession = reported
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
