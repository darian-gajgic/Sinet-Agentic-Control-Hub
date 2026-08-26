package kimicli

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
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

	if s.wirePath == "" {
		p, ok := findWireTranscript(s.low.home)
		if !ok {
			return
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

// findWireTranscript resolves the run's own transcript inside its own home.
//
// A per-RUN KIMI_CODE_HOME holds exactly one session directory, which is what
// makes this a glob rather than a lookup: the session id is only reported in
// the stream's LAST frame, and a run that fails before any model reply never
// reports one at all, so keying on the id would lose the usage of exactly the
// runs worth investigating.
func findWireTranscript(home string) (string, bool) {
	matches, err := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "agents", "main", "wire.jsonl"))
	if err != nil || len(matches) == 0 {
		return "", false
	}
	return matches[0], true
}
