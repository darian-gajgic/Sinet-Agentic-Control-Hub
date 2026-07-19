package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// sseBatchSize bounds one cursor read of the tail loop — the short-batch
// read hygiene of Spec S02.1 (no long-lived read transactions). It is not a
// ⚙ setting: no such key is ratified in Spec S18.
const sseBatchSize = 256

// wireEvent is the SSE data frame: the event-log row, payload embedded
// verbatim. Event-type semantics and the query layers over history are Spec
// S14's (B5); this transport relays rows as appended.
type wireEvent struct {
	Seq           int64           `json:"seq"`
	RunID         string          `json:"run_id,omitempty"`
	Generation    *int64          `json:"generation,omitempty"`
	UserID        string          `json:"user_id"`
	Type          string          `json:"type"`
	SchemaVersion int             `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
	TS            time.Time       `json:"ts"`
}

// handleEvents is the one SSE endpoint, /events (Spec S01.2, S15.2): the
// live stream every surface tails. Frames carry id: event_seq so clients
// resume via Last-Event-ID; ?after_seq=N is the equivalent explicit cursor
// (Spec S15.3 snapshot-then-tail). Without either, the stream tails from
// the current head — backlog is always reachable with an explicit cursor.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	cursor, err := s.startCursor(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	// The front chain keeps /events unbuffered (Caddy, Spec S01.4); no
	// proxy-specific headers are needed here.
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	keepalive, err := s.settings.Duration(keySSEKeepalive)
	if err != nil {
		// ⚙ read failure is a build defect (key declared in Spec S18);
		// close the stream rather than serve without keepalives.
		s.logger.Error("sse: read ⚙ "+keySSEKeepalive, "err", err)
		return
	}
	lastWrite := time.Now()

	poll := time.NewTicker(s.poll)
	defer poll.Stop()

	for {
		nudged := s.nudge.wait()
		n, err := s.writeBatches(r.Context(), w, &cursor)
		if err != nil {
			s.logger.Warn("sse: stream ends", "err", err, "cursor", cursor)
			return
		}
		if n > 0 {
			flusher.Flush()
			lastWrite = time.Now()
		} else if time.Since(lastWrite) >= keepalive {
			// Comment-frame keepalive so idle proxies never drop the
			// stream (⚙ obs.sse_keepalive, Spec S14 via S18).
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
			lastWrite = time.Now()
			// Live-apply: the next cadence re-reads the ⚙ value.
			if ka, kerr := s.settings.Duration(keySSEKeepalive); kerr == nil {
				keepalive = ka
			}
		}

		select {
		case <-r.Context().Done():
			return
		case <-s.stopping:
			// Shell shutdown: deliver what is already appended (the
			// platform.stopping lifecycle event in particular), then end
			// the stream so graceful HTTP shutdown completes (Spec S01.6).
			if _, err := s.writeBatches(context.Background(), w, &cursor); err == nil {
				flusher.Flush()
			}
			return
		case <-nudged:
		case <-poll.C:
		}
	}
}

// startCursor resolves the resume cursor: Last-Event-ID header first (the
// EventSource reconnect contract), then ?after_seq, else the current head.
func (s *Server) startCursor(r *http.Request) (int64, error) {
	parse := func(src, v string) (int64, error) {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("bad %s %q: want a non-negative event_seq", src, v)
		}
		return n, nil
	}
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		return parse("Last-Event-ID", v)
	}
	if v := r.URL.Query().Get("after_seq"); v != "" {
		return parse("after_seq", v)
	}
	return s.log.Head(r.Context())
}

// writeBatches drains the log from *cursor in short batches (Spec S02.1
// read hygiene) until it is exhausted, returning how many events were
// written. The cursor advances past every written event.
func (s *Server) writeBatches(ctx context.Context, w http.ResponseWriter, cursor *int64) (int, error) {
	total := 0
	for {
		batch, err := s.log.After(ctx, *cursor, sseBatchSize)
		if err != nil {
			return total, fmt.Errorf("read after %d: %w", *cursor, err)
		}
		for _, e := range batch {
			if err := writeFrame(w, e); err != nil {
				return total, err
			}
			*cursor = e.Seq
			total++
		}
		if len(batch) < sseBatchSize {
			return total, nil
		}
	}
}

// writeFrame writes one SSE frame: id = event_seq (the sole ordering
// authority, Spec S02.5), event = the stored type, data = the row as JSON.
func writeFrame(w http.ResponseWriter, e eventlog.Event) error {
	we := wireEvent{
		Seq:           e.Seq,
		UserID:        e.UserID,
		Type:          e.Type,
		SchemaVersion: e.SchemaVersion,
		Payload:       e.Payload,
		TS:            e.Time,
	}
	if e.RunID != "" {
		we.RunID = e.RunID
		gen := e.Generation
		we.Generation = &gen
	}
	data, err := json.Marshal(we)
	if err != nil {
		return fmt.Errorf("marshal event %d: %w", e.Seq, err)
	}
	// json.Marshal output never contains raw newlines, so one data: line
	// carries the whole frame.
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.Seq, e.Type, data)
	return err
}
