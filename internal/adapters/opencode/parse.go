package opencode

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// The v1 `GET /event` stream parser. Contract (S03.1, MUST): forward-tolerant —
// unknown event types are logged and skipped, never fatal, and never minted as
// new platform kinds outside the closed set {message, usage, rate_limit,
// gate_ask, tool_result, done}. Both event generations exist in 1.18.3
// (`EventPermissionAsked` and `EventPermissionV2Asked`); the permission events
// ride v1 ONLY, so v1 is what the adapter subscribes to and the v2 shapes are
// parsed tolerantly and ignored [SPIKE P2-S1 Probe 6-B].
//
// The frame shapes asserted here are RECORDED behavior of the pinned engine
// (testdata/PROVENANCE.md), never documentation (S03.3 rule 4).

// scanBufCap bounds one SSE line. Plain constant, the `cancelGrace` /
// `sseBatchSize` precedent (CONVENTIONS §7): S18 ratifies no such key.
const scanBufCap = 10 << 20

// Engine frame types this adapter acts on.
const (
	frameSessionCreated  = "session.created"
	frameSessionUpdated  = "session.updated"
	frameSessionStatus   = "session.status"
	frameSessionIdle     = "session.idle"
	frameMessageUpdated  = "message.updated"
	framePartUpdated     = "message.part.updated"
	framePermissionAsked = "permission.asked"
	framePermissionReply = "permission.replied"
)

// engineNarration are frames the engine emits as internal narration. They carry
// no platform meaning, and counting them as "unknown" would bury a real schema
// drift under keepalive noise (the claudecli system-subtype precedent).
var engineNarration = map[string]bool{
	"server.connected": true, "server.heartbeat": true,
	"session.diff": true, "session.deleted": true, "session.error": true,
	"message.part.delta": true, "message.removed": true, "message.part.removed": true,
	"plugin.added": true, "plugin.removed": true, "catalog.updated": true,
	"integration.updated": true, "reference.updated": true, "todo.updated": true,
	"file.edited": true, "file.watcher.updated": true, "installation.updated": true,
	"lsp.client.diagnostics": true, "ide.installed": true, "workspace.status": true,
}

// finishStop is the engine's own name for a turn that ended by the model
// stopping — as against `tool-calls` (an intermediate paid call) or an abort.
const finishStop = "stop"

// frame is the tolerant top-level decode of one `data:` payload.
type frame struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
}

type sessionProps struct {
	SessionID string         `json:"sessionID"`
	Status    *sessionStatus `json:"status"`
	Info      *sessionInfo   `json:"info"`
}

// sessionStatus is the `GET /session/status` variant set: idle | busy | retry.
// The retry variant carries the provider-limit context D4 forwards as data.
type sessionStatus struct {
	Type    string          `json:"type"`
	Attempt int64           `json:"attempt,omitempty"`
	Message string          `json:"message,omitempty"`
	Next    int64           `json:"next,omitempty"`
	Action  json.RawMessage `json:"action,omitempty"`
}

type sessionInfo struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
}

type messageProps struct {
	SessionID string      `json:"sessionID"`
	Info      messageInfo `json:"info"`
}

type messageInfo struct {
	ID         string          `json:"id"`
	SessionID  string          `json:"sessionID"`
	Role       string          `json:"role"`
	ModelID    string          `json:"modelID"`
	ProviderID string          `json:"providerID"`
	Cost       float64         `json:"cost"`
	Tokens     json.RawMessage `json:"tokens"`
	Finish     string          `json:"finish"`
	Error      json.RawMessage `json:"error"`
	Time       struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed"`
	} `json:"time"`
}

// engineTokens is opencode's own per-message accounting. `reasoning` has no
// typed home on the shared Usage block, so it is preserved VERBATIM in Raw
// rather than invented onto the shared struct (S02.4a).
type engineTokens struct {
	Total     float64 `json:"total"`
	Input     float64 `json:"input"`
	Output    float64 `json:"output"`
	Reasoning float64 `json:"reasoning"`
	Cache     struct {
		Read  float64 `json:"read"`
		Write float64 `json:"write"`
	} `json:"cache"`
}

type partProps struct {
	SessionID string   `json:"sessionID"`
	Part      partInfo `json:"part"`
}

type partInfo struct {
	ID        string          `json:"id"`
	MessageID string          `json:"messageID"`
	SessionID string          `json:"sessionID"`
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Tool      string          `json:"tool"`
	CallID    string          `json:"callID"`
	State     json.RawMessage `json:"state"`
}

type toolState struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	Output string          `json:"output"`
	Error  string          `json:"error"`
	Title  string          `json:"title"`
}

// permissionRequest is the flat `GET /permission` entry and the
// `permission.asked` payload — the same shape on both surfaces.
type permissionRequest struct {
	ID         string          `json:"id"`
	SessionID  string          `json:"sessionID"`
	Permission string          `json:"permission"`
	Patterns   []string        `json:"patterns"`
	Metadata   json.RawMessage `json:"metadata"`
	Always     []string        `json:"always"`
	Tool       struct {
		MessageID string `json:"messageID"`
		CallID    string `json:"callID"`
	} `json:"tool"`
}

type permissionReplied struct {
	SessionID string `json:"sessionID"`
	RequestID string `json:"requestID"`
	Reply     string `json:"reply"`
}

// engineErrorInfo is the AssistantMessage.error shape.
type engineErrorInfo struct {
	Name string `json:"name"`
	Data struct {
		Message string `json:"message"`
	} `json:"data"`
}

// parser folds v1 event frames into contract events plus terminal facts.
type parser struct {
	sessionID string
	logf      func(format string, args ...any)

	// onAsk lets the session decorate an observed ask with its park record
	// BEFORE the event leaves, so the durable row carries the full snapshot.
	onAsk func(ask *adapters.Ask, raw json.RawMessage)

	// cursor + terminal facts.
	directory     string
	finalText     string
	errDetail     string
	lastFinish    string
	lastCompleted string
	paidCalls     int64
	unknownFrames int
	sawBusy       bool
	sawIdle       bool
	transientErr  string

	// ask is the observed gate ask the turn parked on (the Outcome carries
	// one; siblings of a parallel park still get their own durable rows).
	ask    *adapters.Ask
	askRaw json.RawMessage

	// replied records every resolved requestID — one reply can fan out to N
	// permission.replied events.
	replied      map[string]string
	emittedAsks  map[string]bool
	runningTools map[string]bool

	// per-message accumulation.
	seenUsage map[string]bool
	partText  map[string]string
	partOrder map[string][]string
}

func newParser(sessionID string, logf func(string, ...any)) *parser {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &parser{
		sessionID:    sessionID,
		logf:         logf,
		replied:      map[string]string{},
		emittedAsks:  map[string]bool{},
		runningTools: map[string]bool{},
		seenUsage:    map[string]bool{},
		partText:     map[string]string{},
		partOrder:    map[string][]string{},
	}
}

// feed consumes one frame and returns the contract events it completes.
func (p *parser) feed(raw []byte) []adapters.Event {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	var f frame
	if err := json.Unmarshal(raw, &f); err != nil {
		// Malformed frame: skipped, never fatal (S03.1).
		p.unknownFrames++
		p.logf("opencode: skipping malformed event frame: %v", err)
		return nil
	}
	switch f.Type {
	case frameSessionCreated, frameSessionUpdated:
		var sp sessionProps
		if err := json.Unmarshal(f.Properties, &sp); err == nil && p.mine(sp.SessionID) {
			if sp.Info != nil && sp.Info.Directory != "" {
				p.directory = sp.Info.Directory
			}
		}
		return nil
	case frameSessionStatus:
		return p.feedStatus(f.Properties)
	case frameSessionIdle:
		var sp sessionProps
		if err := json.Unmarshal(f.Properties, &sp); err == nil && p.mine(sp.SessionID) {
			p.markIdle()
		}
		return nil
	case frameMessageUpdated:
		return p.feedMessage(f.Properties)
	case framePartUpdated:
		return p.feedPart(f.Properties)
	case framePermissionAsked:
		return p.feedAsked(f.Properties)
	case framePermissionReply:
		return p.feedReplied(f.Properties)
	default:
		if engineNarration[f.Type] {
			return nil
		}
		// Forward-tolerant MUST (S03.1): an unknown type — including the v2
		// permission generation — is logged and skipped, never minted as a new
		// platform kind.
		p.unknownFrames++
		p.logf("opencode: skipping unknown event type %q", f.Type)
		return nil
	}
}

func (p *parser) mine(sessionID string) bool {
	return p.sessionID == "" || sessionID == "" || sessionID == p.sessionID
}

// markIdle honors an idle only once the turn has actually started. A freshly
// created session reports idle before any prompt (recorded live), so an
// ungated idle would end the turn before it began.
func (p *parser) markIdle() {
	if p.sawBusy {
		p.sawIdle = true
	}
}

// assumeInFlight declares that the turn this parser is watching was ALREADY
// running when the stream was joined — the resume-into-a-held-ask case, where
// the serve is busy awaiting the in-process deferred and no fresh `busy` will
// be emitted for a turn that never stopped.
func (p *parser) assumeInFlight() { p.sawBusy = true }

func (p *parser) feedStatus(raw json.RawMessage) []adapters.Event {
	var sp sessionProps
	if err := json.Unmarshal(raw, &sp); err != nil || sp.Status == nil || !p.mine(sp.SessionID) {
		return nil
	}
	switch sp.Status.Type {
	case "busy":
		p.sawBusy = true
	case "idle":
		p.markIdle()
	case "retry":
		// Limit signals ride the contract as DATA (D4); the five-class taxonomy
		// and park scheduling are Spec S10's, never this adapter's (S03.1).
		payload, err := json.Marshal(sp.Status)
		if err != nil {
			return nil
		}
		return []adapters.Event{{Kind: adapters.KindRateLimit, Payload: bounded(payload)}}
	}
	return nil
}

func (p *parser) feedMessage(raw json.RawMessage) []adapters.Event {
	var mp messageProps
	if err := json.Unmarshal(raw, &mp); err != nil {
		p.unknownFrames++
		p.logf("opencode: skipping unparsable message frame: %v", err)
		return nil
	}
	m := mp.Info
	if !p.mine(mp.SessionID) || m.Role != "assistant" || m.ID == "" {
		return nil
	}
	errored := len(m.Error) > 0 && string(m.Error) != "null"
	if errored {
		p.errDetail = describeEngineError(m.Error)
	}
	// One paid call completes exactly once: the engine re-emits the completed
	// message several times, and grouping by message id is what keeps D7 at one
	// checkpoint per paid call rather than one per frame.
	//
	// An ERRORED message still books its tokens when the engine reports it
	// completed: the call was paid for whether or not it produced an answer,
	// and D7 checkpoints paid calls, not successful ones. What it must NOT do
	// is claim the turn's answer or its finish reason.
	if m.Time.Completed == 0 || p.seenUsage[m.ID] {
		return nil
	}
	p.seenUsage[m.ID] = true
	p.paidCalls++

	if !errored {
		p.lastFinish = m.Finish
		p.lastCompleted = m.ID
		// A completion the engine reports as SUCCESSFUL outranks an earlier
		// error: the turn recovered, so the run is not crashed and its real
		// answer is not thrown away. The error is demoted, never erased — it
		// rides the terminal event so an operator can still see it happened.
		if p.errDetail != "" {
			p.transientErr = p.errDetail
			p.errDetail = ""
		}
		if text := p.textFor(m.ID); text != "" {
			p.finalText = text
		}
	}
	text := p.textFor(m.ID)
	modelID := m.ModelID
	if m.ProviderID != "" && modelID != "" {
		modelID = m.ProviderID + "/" + modelID
	}

	var evs []adapters.Event
	excerpt, truncated := excerptOf(text)
	if payload, err := json.Marshal(struct {
		MessageID string `json:"message_id"`
		Model     string `json:"model,omitempty"`
		Finish    string `json:"finish,omitempty"`
		Excerpt   string `json:"excerpt"`
		Truncated bool   `json:"truncated,omitempty"`
	}{m.ID, modelID, m.Finish, excerpt, truncated}); err == nil {
		evs = append(evs, adapters.Event{Kind: adapters.KindMessage, Payload: payload})
	}

	usage := usageFromTokens(m.Tokens, modelID, m.ID, p.paidCalls, m.Cost)
	if payload, err := json.Marshal(struct {
		MessageID    string          `json:"message_id"`
		MessageIndex int64           `json:"message_index"`
		Model        string          `json:"model,omitempty"`
		Tokens       json.RawMessage `json:"tokens"`
		EngineCost   float64         `json:"engine_cost_usd"`
	}{m.ID, p.paidCalls, modelID, boundedOrEmpty(m.Tokens), m.Cost}); err == nil {
		evs = append(evs, adapters.Event{Kind: adapters.KindUsage, Payload: payload, Usage: usage})
	}
	return evs
}

func (p *parser) feedPart(raw json.RawMessage) []adapters.Event {
	var pp partProps
	if err := json.Unmarshal(raw, &pp); err != nil {
		p.unknownFrames++
		p.logf("opencode: skipping unparsable part frame: %v", err)
		return nil
	}
	part := pp.Part
	if !p.mine(pp.SessionID) {
		return nil
	}
	switch part.Type {
	case "text":
		if part.MessageID == "" || part.ID == "" {
			return nil
		}
		if _, seen := p.partText[part.ID]; !seen {
			p.partOrder[part.MessageID] = append(p.partOrder[part.MessageID], part.ID)
		}
		p.partText[part.ID] = part.Text
		// A text part can arrive AFTER its own message's completion frame — the
		// engine re-emits parts and the two orderings both occur live. Losing
		// the engine's whole answer to that race is exactly the failure R19
		// exists to end, so a late part still updates the retained text (only
		// for the message that completed last, so an older message's late frame
		// can never overwrite a newer answer).
		if part.MessageID == p.lastCompleted {
			if txt := p.textFor(part.MessageID); txt != "" {
				p.finalText = txt
			}
		}
		return nil
	case "tool":
		var st toolState
		if len(part.State) > 0 {
			_ = json.Unmarshal(part.State, &st)
		}
		key := part.CallID
		if key == "" {
			key = part.ID
		}
		switch st.Status {
		case "running", "pending":
			// Mid-tool-call: never a safe interrupt boundary (P-T01-4). A
			// parallel turn has SEVERAL calls in flight at once (the fan-out is
			// recorded live), so this is a SET: one sibling finishing does not
			// make the turn interruptible while another is still executing.
			p.runningTools[key] = true
			return nil
		case "completed", "error":
			delete(p.runningTools, key)
			body := st.Output
			if st.Status == "error" {
				body = st.Error
			}
			excerpt, truncated := excerptOf(body)
			payload, err := json.Marshal(struct {
				ToolUseID string `json:"tool_use_id,omitempty"`
				Tool      string `json:"tool,omitempty"`
				Status    string `json:"status"`
				Excerpt   string `json:"excerpt"`
				Truncated bool   `json:"truncated,omitempty"`
			}{part.CallID, part.Tool, st.Status, excerpt, truncated})
			if err != nil {
				return nil
			}
			return []adapters.Event{{Kind: adapters.KindToolResult, Payload: payload}}
		}
	}
	return nil
}

func (p *parser) feedAsked(raw json.RawMessage) []adapters.Event {
	var pr permissionRequest
	if err := json.Unmarshal(raw, &pr); err != nil {
		p.unknownFrames++
		p.logf("opencode: skipping unparsable permission.asked frame: %v", err)
		return nil
	}
	if !p.mine(pr.SessionID) {
		return nil
	}
	return p.emitAsk(pr, raw)
}

// emitAsk turns one PermissionRequest into the platform's durable ask input.
// Everything is keyed by requestID (S03.4); re-observation is idempotent.
func (p *parser) emitAsk(pr permissionRequest, raw json.RawMessage) []adapters.Event {
	// An entry without a requestID is unanswerable and unrecordable: the Driver
	// refuses an ask with no id, and refusing it MID-PUMP would abandon the
	// event loop before the FSM transition, wedging the run in `running`. Both
	// callers route through here — the frame path and the pull-based
	// GET /permission enumeration — so the guard lives here, not at one of them.
	if pr.ID == "" {
		p.unknownFrames++
		p.logf("opencode: skipping a pending permission entry with no requestID — unanswerable and unrecordable (S03.4 keys everything by requestID)")
		return nil
	}
	if p.emittedAsks[pr.ID] {
		return nil
	}
	p.emittedAsks[pr.ID] = true
	ask, askRaw := askFromRequest(pr, raw)
	if p.onAsk != nil {
		p.onAsk(ask, askRaw)
	}
	// The turn parks HERE: the ask that ends the turn is the one the Outcome
	// carries; siblings of a parallel fan-out still get their own durable rows.
	if p.ask == nil {
		p.ask = ask
		p.askRaw = askRaw
	}
	payload, err := json.Marshal(struct {
		AskID     string          `json:"ask_id"`
		ToolName  string          `json:"tool_name"`
		ToolInput json.RawMessage `json:"tool_input,omitempty"`
		CallID    string          `json:"call_id,omitempty"`
		MessageID string          `json:"message_id,omitempty"`
		Patterns  []string        `json:"patterns,omitempty"`
	}{pr.ID, pr.Permission, bounded(pr.Metadata), pr.Tool.CallID, pr.Tool.MessageID, pr.Patterns})
	if err != nil {
		return nil
	}
	return []adapters.Event{{Kind: adapters.KindGateAsk, Payload: payload, Ask: ask}}
}

// askFromRequest builds the contract Ask. EngineExpiry stays ZERO deliberately:
// this engine advertises no retention for a pending ask at all — it holds them
// in an in-memory Map that a graceful shutdown rejects SILENTLY and a crash
// loses (#36347, unfixed). The platform record outlives it regardless (S03.4).
func askFromRequest(pr permissionRequest, raw json.RawMessage) (*adapters.Ask, json.RawMessage) {
	if len(raw) == 0 {
		raw, _ = json.Marshal(pr)
	}
	return &adapters.Ask{
		ID:        pr.ID,
		ToolName:  pr.Permission,
		ToolInput: bounded(pr.Metadata),
	}, bounded(raw)
}

func (p *parser) feedReplied(raw json.RawMessage) []adapters.Event {
	var rep permissionReplied
	if err := json.Unmarshal(raw, &rep); err != nil {
		p.unknownFrames++
		p.logf("opencode: skipping unparsable permission.replied frame: %v", err)
		return nil
	}
	if !p.mine(rep.SessionID) || rep.RequestID == "" {
		return nil
	}
	// ONE reply can fan out to N replied events (an `always`-class rule matches
	// siblings). Each is resolved by its own requestID, tolerantly, and neither
	// duplicates a platform event nor crashes the stream [SPIKE P2-S1 Probe 6].
	p.replied[rep.RequestID] = rep.Reply
	if p.ask != nil && p.ask.ID == rep.RequestID {
		p.ask, p.askRaw = nil, nil
	}
	return nil
}

func (p *parser) textFor(messageID string) string {
	var b strings.Builder
	for _, partID := range p.partOrder[messageID] {
		b.WriteString(p.partText[partID])
	}
	return b.String()
}

// usageFromTokens normalizes opencode's per-message accounting into the
// contract's S02.4(a) block.
//
// EngineCostUSD is forwarded as a CROSS-CHECK figure only. opencode prices
// flat-rate plans at $0 (models.dev rates them zero), so this number is a fact
// about the engine's opinion, never a price: Sinet meters independently (D4)
// and prices from its own table (D5, Spec S10). Nothing in this package derives
// a dollar figure from it (S03.7 "never meter dollars from opencode").
func usageFromTokens(raw json.RawMessage, modelID, messageID string, index int64, engineCost float64) *adapters.Usage {
	u := &adapters.Usage{
		ModelID:       modelID,
		MessageID:     messageID,
		MessageIndex:  index,
		EngineCostUSD: engineCost,
		Raw:           boundedOrEmpty(raw),
	}
	if len(raw) > 0 {
		var t engineTokens
		if err := json.Unmarshal(raw, &t); err == nil {
			u.InputTokens = int64(t.Input)
			u.OutputTokens = int64(t.Output)
			u.CacheReadTokens = int64(t.Cache.Read)
			u.CacheCreationTokens = int64(t.Cache.Write)
		}
	}
	return u
}

func describeEngineError(raw json.RawMessage) string {
	var e engineErrorInfo
	if err := json.Unmarshal(raw, &e); err != nil || e.Name == "" {
		return "engine reported an error: " + string(bounded(raw))
	}
	if e.Data.Message != "" {
		return e.Name + ": " + e.Data.Message
	}
	return e.Name
}

// readFrames reads an SSE body and hands each `data:` payload to onFrame.
// Returning false from onFrame stops the read.
//
// An oversized line is a SKIPPED line, never a wedge (S03.1 forward tolerance;
// CONVENTIONS §60): it is discarded through its newline, logged with its byte
// length, and the read CONTINUES — so the rest of the turn still arrives and
// the server never blocks on a body nobody is draining.
func readFrames(r io.Reader, onFrame func([]byte) bool, logf func(string, ...any)) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, n, skipped, err := readCappedLine(br, scanBufCap)
		if skipped {
			logf("opencode: skipping oversized event line: %d bytes exceeds the %d-byte cap", n, scanBufCap)
		} else if payload, ok := sseData(line); ok {
			if !onFrame(payload) {
				return nil
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// sseData extracts the payload of a `data:` line. Comment lines (`:`), blank
// separators and every other SSE field are skipped: this engine sends no `id:`
// field at all, which is the wire-level reason there is nothing to resume from.
func sseData(line []byte) ([]byte, bool) {
	const prefix = "data:"
	if !strings.HasPrefix(string(line), prefix) {
		return nil, false
	}
	payload := line[len(prefix):]
	if len(payload) > 0 && payload[0] == ' ' {
		payload = payload[1:]
	}
	return payload, true
}

// readCappedLine reads one newline-terminated line (terminator stripped),
// bounded by limit. A line over the limit is consumed and DISCARDED through its
// newline and reported as skipped with its full byte length.
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

// excerptOf bounds free text at adapters.ExcerptCap (refs-not-blobs, P-T07-5).
func excerptOf(s string) (string, bool) {
	if len(s) <= adapters.ExcerptCap {
		return s, false
	}
	return s[:adapters.ExcerptCap], true
}

// bounded caps a raw JSON value for event payloads; an oversize value becomes a
// ref-style stub rather than truncated into invalid JSON.
func bounded(raw json.RawMessage) json.RawMessage {
	if len(raw) <= adapters.ExcerptCap {
		return raw
	}
	stub, err := json.Marshal(struct {
		Oversize bool   `json:"oversize"`
		Bytes    int    `json:"bytes"`
		Note     string `json:"note"`
	}{true, len(raw), "full content in the engine's own session store (P-T07-5)"})
	if err != nil {
		return json.RawMessage(`{"oversize":true}`)
	}
	return stub
}

func boundedOrEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return bounded(raw)
}
