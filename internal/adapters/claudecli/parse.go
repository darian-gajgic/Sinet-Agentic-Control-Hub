package claudecli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
)

// The stream-json parser. Contract (S03.1, MUST): forward-tolerant —
// unknown event types are logged and skipped, never fatal (P-T01-2: schema
// drift is permanent even first-party; the GitButler lesson). The event
// shapes asserted here are the recorded behavior of the pinned engine
// (SPIKE G1-S1 F1; SPIKE G1-S2 F1), re-recorded in testdata fixtures — the
// conformance suite is their tripwire (S03.3).

// streamLine is the tolerant top-level decode of one stdout JSONL line.
type streamLine struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`

	// system/init fields (SPIKE G1-S1: session_id, model, permissionMode,
	// cwd, apiKeySource).
	SessionID      string `json:"session_id"`
	Model          string `json:"model"`
	PermissionMode string `json:"permissionMode"`
	CWD            string `json:"cwd"`
	APIKeySource   string `json:"apiKeySource"`
	// TranscriptPath is captured when the stream carries it (hook payloads
	// do; init may — tolerant either way, S03.3 rule 4).
	TranscriptPath string `json:"transcript_path"`

	// assistant / user frames.
	Message json.RawMessage `json:"message"`

	// rate_limit_event (verbatim shape, SPIKE G1-S1 F1).
	RateLimitInfo json.RawMessage `json:"rate_limit_info"`

	// stream_event frames (--include-partial-messages): only the
	// underlying event type is inspected (message_stop = safe boundary).
	Event json.RawMessage `json:"event"`

	// result envelope (SPIKE G1-S1 F1 key set; G1-S2 F1 defer fields).
	IsError         bool            `json:"is_error"`
	Result          string          `json:"result"`
	StopReason      string          `json:"stop_reason"`
	TerminalReason  string          `json:"terminal_reason"`
	NumTurns        int64           `json:"num_turns"`
	TotalCostUSD    float64         `json:"total_cost_usd"`
	Usage           json.RawMessage `json:"usage"`
	ModelUsage      json.RawMessage `json:"modelUsage"`
	DeferredToolUse json.RawMessage `json:"deferred_tool_use"`
}

// assistantMessage is the tolerant decode of an assistant frame's message
// object. One API message can span several JSONL lines (SPIKE G1-S2
// methodology: counting lines overstates turns) — the parser groups by
// message id and flushes one Usage event per paid call.
type assistantMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content []contentBlock  `json:"content"`
	Usage   json.RawMessage `json:"usage"`
}

type contentBlock struct {
	Type string `json:"type"`
	// text blocks
	Text string `json:"text"`
	// tool_use blocks
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	// tool_result blocks (user frames)
	Content json.RawMessage `json:"content"`
}

// engineUsage is the tolerant decode of the engine usage object
// (S02.4a fields as recorded at the pin).
type engineUsage struct {
	InputTokens              int64            `json:"input_tokens"`
	OutputTokens             int64            `json:"output_tokens"`
	CacheReadInputTokens     int64            `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64            `json:"cache_creation_input_tokens"`
	CacheCreation            map[string]int64 `json:"cache_creation"`
}

// deferredToolUse is the exit-JSON pending-call object (SPIKE G1-S2 F1:
// the ask record is built from the exit JSON alone, no transcript
// parsing).
type deferredToolUse struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// parser folds JSONL lines into contract events plus terminal facts.
type parser struct {
	logf func(format string, args ...any)

	// cursor facts (owned by the session under its mutex; the parser
	// reports via callbacks).
	onInit  func(sessionID, model, permissionMode, cwd, transcriptPath string)
	onTurn  func() // first frame of a new assistant message id (gate-ctl turn boundary)
	onFlush func(messageIndex int64)

	pending      *assistantMessage
	pendingText  strings.Builder
	pendingTools []contentBlock
	messageIndex int64

	// terminal facts.
	sawResult bool
	result    streamLine

	unknownLines int
}

// feed consumes one line and returns the contract events it completes.
func (p *parser) feed(line []byte) []adapters.Event {
	if len(strings.TrimSpace(string(line))) == 0 {
		return nil
	}
	var sl streamLine
	if err := json.Unmarshal(line, &sl); err != nil {
		// Malformed line: skipped, never fatal (S03.1).
		p.unknownLines++
		p.logf("claudecli: skipping malformed stream line: %v", err)
		return nil
	}
	switch sl.Type {
	case "system":
		if sl.Subtype == "init" {
			if p.onInit != nil {
				p.onInit(sl.SessionID, sl.Model, sl.PermissionMode, sl.CWD, sl.TranscriptPath)
			}
		}
		// Other system subtypes (hook_started, hook_response,
		// thinking_tokens, …) are engine-internal narration: skipped.
		return nil
	case "assistant":
		return p.feedAssistant(sl.Message)
	case "user":
		return p.feedUser(sl.Message)
	case "rate_limit_event":
		// Limit signals ride the contract as data (D4); the five-class
		// taxonomy and park scheduling are Spec S10's (S03.1).
		payload := sl.RateLimitInfo
		if len(payload) == 0 {
			payload = json.RawMessage("{}")
		}
		return []adapters.Event{{Kind: adapters.KindRateLimit, Payload: bounded(payload)}}
	case "stream_event":
		// Partial-message frames (--include-partial-messages): live
		// progress only, never persisted — checkpoints are message-level
		// (D7 per paid call). One exception: a message_stop marks the
		// paid call complete, so the pending call flushes NOW (prompt
		// checkpointing + the pause-safe boundary) instead of waiting
		// for the next frame boundary.
		if len(sl.Event) > 0 {
			var ev struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(sl.Event, &ev); err == nil && ev.Type == "message_stop" {
				return p.flush()
			}
		}
		return nil
	case "result":
		evs := p.flush()
		p.sawResult = true
		p.result = sl
		return evs
	default:
		// Forward-tolerant MUST (S03.1): unknown type — log and skip.
		p.unknownLines++
		p.logf("claudecli: skipping unknown stream event type %q", sl.Type)
		return nil
	}
}

func (p *parser) feedAssistant(raw json.RawMessage) []adapters.Event {
	var m assistantMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		p.unknownLines++
		p.logf("claudecli: skipping unparsable assistant frame: %v", err)
		return nil
	}
	var evs []adapters.Event
	if p.pending == nil || p.pending.ID != m.ID {
		evs = p.flush()
		p.pending = &assistantMessage{ID: m.ID}
		if p.onTurn != nil {
			p.onTurn()
		}
	}
	// Later frames of the same message id refine the same paid call: the
	// last-seen usage/model win; text and tool_use blocks accumulate.
	if m.Model != "" {
		p.pending.Model = m.Model
	}
	if len(m.Usage) > 0 {
		p.pending.Usage = m.Usage
	}
	for _, b := range m.Content {
		switch b.Type {
		case "text":
			p.pendingText.WriteString(b.Text)
		case "tool_use":
			p.pendingTools = append(p.pendingTools, b)
		}
	}
	return evs
}

func (p *parser) feedUser(raw json.RawMessage) []adapters.Event {
	evs := p.flush()
	var m assistantMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		p.unknownLines++
		p.logf("claudecli: skipping unparsable user frame: %v", err)
		return evs
	}
	for _, b := range m.Content {
		if b.Type != "tool_result" {
			continue
		}
		excerpt, truncated := excerptOf(string(b.Content))
		payload, err := json.Marshal(struct {
			ToolUseID string `json:"tool_use_id,omitempty"`
			Excerpt   string `json:"excerpt"`
			Truncated bool   `json:"truncated,omitempty"`
		}{b.ID, excerpt, truncated})
		if err != nil {
			continue
		}
		evs = append(evs, adapters.Event{Kind: adapters.KindToolResult, Payload: payload})
	}
	return evs
}

// flush completes the pending paid call: one message event, then one usage
// event (the S02.4 checkpoint trigger).
func (p *parser) flush() []adapters.Event {
	if p.pending == nil {
		return nil
	}
	m := p.pending
	text := p.pendingText.String()
	tools := p.pendingTools
	p.pending = nil
	p.pendingText.Reset()
	p.pendingTools = nil
	p.messageIndex++
	if p.onFlush != nil {
		p.onFlush(p.messageIndex)
	}

	var evs []adapters.Event
	excerpt, truncated := excerptOf(text)
	type toolRef struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	refs := make([]toolRef, 0, len(tools))
	for _, t := range tools {
		refs = append(refs, toolRef{t.ID, t.Name})
	}
	msgPayload, err := json.Marshal(struct {
		MessageID string    `json:"message_id"`
		Model     string    `json:"model,omitempty"`
		Excerpt   string    `json:"excerpt"`
		Truncated bool      `json:"truncated,omitempty"`
		ToolUses  []toolRef `json:"tool_uses,omitempty"`
	}{m.ID, m.Model, excerpt, truncated, refs})
	if err == nil {
		evs = append(evs, adapters.Event{Kind: adapters.KindMessage, Payload: msgPayload})
	}

	usage := usageFromRaw(m.Usage, m.Model, m.ID, p.messageIndex, false, 0)
	usagePayload, err := json.Marshal(struct {
		MessageID    string          `json:"message_id"`
		MessageIndex int64           `json:"message_index"`
		Model        string          `json:"model,omitempty"`
		Usage        json.RawMessage `json:"usage"`
	}{m.ID, p.messageIndex, m.Model, boundedOrEmpty(m.Usage)})
	if err == nil {
		evs = append(evs, adapters.Event{Kind: adapters.KindUsage, Payload: usagePayload, Usage: usage})
	}
	return evs
}

// usageFromRaw normalizes the engine usage object into the contract's
// S02.4(a) block.
func usageFromRaw(raw json.RawMessage, model, messageID string, index int64, total bool, engineCost float64) *adapters.Usage {
	u := &adapters.Usage{
		ModelID:       model,
		MessageID:     messageID,
		MessageIndex:  index,
		Total:         total,
		EngineCostUSD: engineCost,
		Raw:           boundedOrEmpty(raw),
	}
	if len(raw) > 0 {
		var eu engineUsage
		if err := json.Unmarshal(raw, &eu); err == nil {
			u.InputTokens = eu.InputTokens
			u.OutputTokens = eu.OutputTokens
			u.CacheReadTokens = eu.CacheReadInputTokens
			u.CacheCreationTokens = eu.CacheCreationInputTokens
			if len(eu.CacheCreation) > 0 {
				u.CacheCreationByTTL = eu.CacheCreation
			}
		}
	}
	return u
}

// donePayload builds the bounded engine.done payload from the terminal
// envelope.
func donePayload(sl streamLine, gateFallback bool) json.RawMessage {
	p, err := json.Marshal(struct {
		Subtype        string          `json:"subtype,omitempty"`
		StopReason     string          `json:"stop_reason,omitempty"`
		TerminalReason string          `json:"terminal_reason,omitempty"`
		IsError        bool            `json:"is_error,omitempty"`
		NumTurns       int64           `json:"num_turns,omitempty"`
		TotalCostUSD   float64         `json:"total_cost_usd,omitempty"`
		Usage          json.RawMessage `json:"usage,omitempty"`
		ModelUsage     json.RawMessage `json:"model_usage,omitempty"`
		GateFallback   bool            `json:"gate_fallback,omitempty"`
	}{sl.Subtype, sl.StopReason, sl.TerminalReason, sl.IsError, sl.NumTurns,
		sl.TotalCostUSD, boundedOrEmpty(sl.Usage), boundedOrEmpty(sl.ModelUsage), gateFallback})
	if err != nil {
		return json.RawMessage("{}")
	}
	return p
}

// excerptOf bounds free text at adapters.ExcerptCap (refs-not-blobs,
// P-T07-5; full content lives in the transcript + copy-aside).
func excerptOf(s string) (string, bool) {
	if len(s) <= adapters.ExcerptCap {
		return s, false
	}
	return s[:adapters.ExcerptCap], true
}

// bounded caps a raw JSON value for event payloads; oversize values are
// replaced by a ref-style stub rather than truncated into invalid JSON.
func bounded(raw json.RawMessage) json.RawMessage {
	if len(raw) <= adapters.ExcerptCap {
		return raw
	}
	stub, err := json.Marshal(struct {
		Oversize bool   `json:"oversize"`
		Bytes    int    `json:"bytes"`
		Note     string `json:"note"`
	}{true, len(raw), "full content in engine transcript / copy-aside (P-T07-5)"})
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

// parseDeferred decodes the exit-JSON deferred_tool_use object.
func parseDeferred(raw json.RawMessage) (*deferredToolUse, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("claudecli: defer exit without deferred_tool_use")
	}
	var d deferredToolUse
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("claudecli: parse deferred_tool_use: %w", err)
	}
	if d.ID == "" {
		return nil, fmt.Errorf("claudecli: deferred_tool_use without id")
	}
	return &d, nil
}
