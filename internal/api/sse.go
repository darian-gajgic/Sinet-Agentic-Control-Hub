package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/redact"
)

// sseBatchSize bounds one cursor read of the tail loop — the short-batch
// read hygiene of Spec S02.1 (no long-lived read transactions). It is not a
// ⚙ setting: no such key is ratified in Spec S18.
const sseBatchSize = 256

// wireEvent is the SSE data frame: the event-log row with its topic tags and
// its payload embedded (redacted at the serving edge, R16). Event-type
// semantics are Spec S14.2's; this transport relays rows as appended.
type wireEvent struct {
	Seq           int64  `json:"seq"`
	RunID         string `json:"run_id,omitempty"`
	Generation    *int64 `json:"generation,omitempty"`
	UserID        string `json:"user_id"`
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	// Topics are the S14.3 topic tags (brief §2), multi-valued so the client
	// routes each frame even on the unfiltered relay.
	Topics  []string        `json:"topics"`
	Payload json.RawMessage `json:"payload"`
	TS      time.Time       `json:"ts"`
}

// subscription is one client's resolved S14.3 stream posture: its owner scope
// (S01.9, OQ1) and its optional topic filter. topics==nil is the unfiltered
// all-events relay (back-compat with the B0-3 endpoint + the operator's
// full-audit view) — still owner-scoped.
type subscription struct {
	caller     string
	isOperator bool
	topics     map[string]bool // nil = unfiltered relay
	runID      string          // the ?run=<id> for the run-detail topic
}

// visible applies the S14.3 owner scope + topic filter to one tail event
// (OQ1). Owner scope is ALWAYS enforced, even on the unfiltered relay: the
// operator (and the dev-posture bootstrap identity) sees every row; a member
// sees only their own (user_id == caller), so platform-scope rows
// (user_id="platform") reach the operator alone. The topic filter, when a
// subscription is active, additionally requires a tag match — and the run
// topic matches only the subscribed run.
func (sub subscription) visible(e eventlog.Event, tags []string) bool {
	if !sub.isOperator && e.UserID != sub.caller {
		return false
	}
	if sub.topics == nil {
		return true
	}
	for _, t := range tags {
		if !sub.topics[t] {
			continue
		}
		if t == topicRun && e.RunID != sub.runID {
			continue
		}
		return true
	}
	return false
}

// handleEvents is the one SSE endpoint, /events (Spec S01.2, S14.3): the live
// stream every surface tails, topic-tagged and owner-scoped. A topic-subscribed
// connect (?topics=…) gets snapshot-then-tail — current state projected from
// the DB, then events after the snapshot cursor (OQ4). The raw ?after_seq poll
// path (no ?topics) stays delta-only, resuming via Last-Event-ID then
// ?after_seq (R6/R13). Frames carry id: event_seq (the sole ordering
// authority) so clients resume; the durable log is the only replay source (no
// engine SSE replay is ever relied on, S14.5).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	sub, err := s.resolveSubscription(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// A subscription re-snapshots on every connect (OQ4); it needs the
	// projector, and — for the run topic — the S01.9 owner check (OQ1).
	if sub.topics != nil {
		if s.proj == nil {
			http.Error(w, "snapshots unavailable", http.StatusServiceUnavailable)
			return
		}
		if sub.topics[topicRun] {
			if code, cerr := s.authorizeRun(r.Context(), sub); cerr != nil {
				http.Error(w, cerr.Error(), code)
				return
			}
		}
	}

	// The tail cursor. For a subscription it is superseded by the fresh
	// snapshot head (captured below); for the poll path it is the resume
	// cursor (Last-Event-ID, then ?after_seq, else head).
	var cursor int64
	if sub.topics == nil {
		if cursor, err = s.startCursor(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	// The front chain keeps /events unbuffered (Caddy, Spec S01.4); no
	// proxy-specific headers are needed here.
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Snapshot-then-tail (S14.3 ¶2, OQ4): capture the head cursor H, project
	// current state as of H, then tail from H so no event frame is dropped or
	// double-delivered across the boundary (R7 — the snapshot carries STATE,
	// never event frames, and the tail begins strictly after H).
	if sub.topics != nil {
		head, herr := s.log.Head(r.Context())
		if herr != nil {
			s.logger.Warn("sse: head for snapshot", "err", herr)
			return
		}
		if werr := s.writeSnapshots(r.Context(), w, sub, head); werr != nil {
			s.logger.Warn("sse: snapshot", "err", werr)
			return
		}
		flusher.Flush()
		cursor = head
	}

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
		n, err := s.writeBatches(r.Context(), w, &cursor, sub)
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
			if _, err := s.writeBatches(context.Background(), w, &cursor, sub); err == nil {
				flusher.Flush()
			}
			return
		case <-nudged:
		case <-poll.C:
		}
	}
}

// resolveSubscription parses the S14.3 subscribe/filter contract (brief §2/§3)
// and the caller's owner scope. No ?topics is the unfiltered relay (topics
// nil); an unknown topic name is a 400 (the startCursor bad-param precedent);
// the run topic requires ?run=<id>. The run owner check (OQ1) is authorizeRun's
// — it needs the projector.
func (s *Server) resolveSubscription(r *http.Request) (subscription, error) {
	id, _ := IdentityFrom(r.Context()) // present: /events is requireIdentity
	sub := subscription{caller: id.UserID, isOperator: s.isOperatorRead(r.Context(), id)}

	raw := r.URL.Query().Get("topics")
	if raw == "" {
		return sub, nil // unfiltered relay (still owner-scoped)
	}
	set := map[string]bool{}
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !knownTopics[t] {
			return subscription{}, fmt.Errorf("unknown topic %q: want run|board|fleet|inbox", t)
		}
		set[t] = true
	}
	if len(set) == 0 {
		return sub, nil
	}
	sub.topics = set
	if set[topicRun] {
		sub.runID = strings.TrimSpace(r.URL.Query().Get("run"))
		if sub.runID == "" {
			return subscription{}, errors.New("topic run requires ?run=<run_id>")
		}
	}
	return sub, nil
}

// authorizeRun enforces the S01.9 run-detail owner check (OQ1): a member may
// subscribe only to their own run — 404 if it does not exist, 403 if it
// belongs to another owner; the operator subscribes to any run. Returns
// (0, nil) when authorized.
func (s *Server) authorizeRun(ctx context.Context, sub subscription) (int, error) {
	_, owner, found, err := s.proj.runCard(ctx, sub.runID)
	if err != nil {
		return http.StatusInternalServerError, errors.New("projection error")
	}
	if !found {
		return http.StatusNotFound, errors.New("run not found")
	}
	if !sub.isOperator && owner != sub.caller {
		return http.StatusForbidden, errors.New("forbidden")
	}
	return 0, nil
}

// isOperatorRead resolves the one S01.9 role bit for read scope: the operator
// (read via the auth store) sees all rows; a member sees their own. The
// dev-posture fallback identity is operator-equivalent for READ — the
// single-operator dev relay that keeps the SSE demo working without a login
// (CONVENTIONS §7/§9); production always resolves real session roles. An
// unknown user fails closed to member scope.
func (s *Server) isOperatorRead(ctx context.Context, id Identity) bool {
	if id.Dev {
		return true
	}
	if s.sessions == nil {
		return false
	}
	u, err := s.sessions.User(ctx, id.UserID)
	if err != nil {
		return false
	}
	return u.Role == auth.RoleOperator
}

// writeSnapshots emits one `snapshot` frame per subscribed topic (S14.3 ¶2):
// event: snapshot, data: {topic, cursor, state}. The state is the §3 owner-
// scoped projection, redacted at this serving edge (R16). Frames carry no id:
// — id: stays event_seq (CONVENTIONS §7); the snapshot cursor rides the data.
func (s *Server) writeSnapshots(ctx context.Context, w http.ResponseWriter, sub subscription, cursor int64) error {
	scope := ownerScope{UserID: sub.caller, Operator: sub.isOperator}
	if sub.topics[topicRun] {
		card, _, found, err := s.proj.runCard(ctx, sub.runID)
		if err != nil {
			return err
		}
		if found {
			if err := writeSnapshotFrame(w, topicRun, cursor, card); err != nil {
				return err
			}
		}
	}
	if sub.topics[topicBoard] {
		board, err := s.proj.board(ctx, scope)
		if err != nil {
			return err
		}
		if err := writeSnapshotFrame(w, topicBoard, cursor, board); err != nil {
			return err
		}
	}
	if sub.topics[topicFleet] {
		fleet, err := s.proj.fleet(ctx, scope)
		if err != nil {
			return err
		}
		if err := writeSnapshotFrame(w, topicFleet, cursor, fleet); err != nil {
			return err
		}
	}
	if sub.topics[topicInbox] {
		inbox, err := s.proj.inbox(ctx, scope)
		if err != nil {
			return err
		}
		if err := writeSnapshotFrame(w, topicInbox, cursor, inbox); err != nil {
			return err
		}
	}
	return nil
}

// writeSnapshotFrame serializes and redacts one snapshot projection. Redaction
// runs over the whole marshaled state (RedactJSON deep-walks its string values,
// R16) — honest state is byte-unchanged, secrets become [REDACTED:…].
func writeSnapshotFrame(w http.ResponseWriter, topic string, cursor int64, state any) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal %s snapshot: %w", topic, err)
	}
	env := struct {
		Topic  string          `json:"topic"`
		Cursor int64           `json:"cursor"`
		State  json.RawMessage `json:"state"`
	}{topic, cursor, redact.RedactJSON(raw)}
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal %s frame: %w", topic, err)
	}
	_, err = fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", data)
	return err
}

// startCursor resolves the resume cursor for the poll path: Last-Event-ID
// header first (the EventSource reconnect contract), then ?after_seq, else the
// current head. A subscription supersedes this with a fresh snapshot (OQ4).
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

// writeBatches drains the log from *cursor in short batches (Spec S02.1 read
// hygiene) until it is exhausted, writing only the events the subscription can
// see (owner scope + topic filter, OQ1). The cursor advances past EVERY row
// read — visible or filtered — so the drain-to-exhaustion and keepalive logic
// is unaffected by filtering. Returns how many frames were written.
func (s *Server) writeBatches(ctx context.Context, w http.ResponseWriter, cursor *int64, sub subscription) (int, error) {
	total := 0
	for {
		batch, err := s.log.After(ctx, *cursor, sseBatchSize)
		if err != nil {
			return total, fmt.Errorf("read after %d: %w", *cursor, err)
		}
		for _, e := range batch {
			*cursor = e.Seq
			tags := topicsForEvent(e)
			if !sub.visible(e, tags) {
				continue
			}
			if err := writeFrame(w, e, tags); err != nil {
				return total, err
			}
			total++
		}
		if len(batch) < sseBatchSize {
			return total, nil
		}
	}
}

// writeFrame writes one SSE frame: id = event_seq (the sole ordering
// authority, Spec S02.5), event = the stored type (verbatim, canonical
// post-B5-1), data = the row as JSON with its topic tags and its payload
// REDACTED at this serving edge (store-raw/serve-redacted, R19 — the stored
// run_events.payload is never mutated).
func writeFrame(w http.ResponseWriter, e eventlog.Event, tags []string) error {
	if tags == nil {
		tags = []string{}
	}
	we := wireEvent{
		Seq:           e.Seq,
		UserID:        e.UserID,
		Type:          e.Type,
		SchemaVersion: e.SchemaVersion,
		Topics:        tags,
		Payload:       redact.RedactJSON(e.Payload),
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
