package kimicli

import (
	"encoding/json"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// The stream-json parser. Contract (S03.1, MUST): forward-tolerant — unknown
// frames are logged and skipped, never fatal, and never minted as a new
// platform kind (P-T01-2).
//
// That obligation matters more here than anywhere else in the tree. The entire
// published description of this format is two prose sentences: there is no type
// enumeration, no example object, no error shape and no terminal envelope
// anywhere in the vendor's 22 English doc pages. The shapes below are the SIX
// this platform observed by execution at pin 0.38.0
// (P3/measurements/2026-08-26-kimi-cli-print-mode-spike.md), and the engine
// publishes ~70 versions a quarter.

// streamLine is the tolerant top-level decode of one stdout JSONL line.
type streamLine struct {
	// Role is the OpenAI-chat-shaped discriminator: "assistant", "tool", or
	// "meta". Type further discriminates the meta frames.
	Role string `json:"role"`
	Type string `json:"type"`

	Content   json.RawMessage `json:"content"`
	ToolCalls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
	ToolCallID string `json:"tool_call_id"`

	// meta/system.version — the engine's own version, first frame of every
	// run. It makes a pin↔installed check free and in-band.
	Version string `json:"version"`

	// meta/session.resume_hint — the LAST frame of a run that reached the
	// model, and the only in-stream source of the engine-reported session id.
	// It is ABSENT when a run fails before any model reply (measured on a
	// provider 403), which is why the session store is the fallback.
	SessionID string `json:"session_id"`

	// meta/turn.step.retrying — the engine's own retry narration. It is the
	// only structured error signal on stdout, and it carries the wire status.
	StatusCode   int    `json:"status_code"`
	ErrorName    string `json:"error_name"`
	ErrorMessage string `json:"error_message"`
	MaxAttempts  int    `json:"max_attempts"`
}

// parser folds stdout JSONL lines into contract events plus terminal facts.
type parser struct {
	logf func(format string, args ...any)

	// signals is the lane document's own wire-signal extractor, wired at the
	// composition root. The PAYLOAD is the contract (scheduler.SignalFromPayload
	// decodes it): no shared type crosses, and this adapter classifies nothing.
	// A nil seam forwards raw facts with no documented class — the honest
	// degrade.
	signals func(bodyText string, httpStatus int) (json.RawMessage, bool)

	onSession func(sessionID string)
	onFlush   func()

	// sessionID and version are the cursor facts this stream carries.
	sessionID string
	version   string

	// finalText retains the LAST text-bearing assistant message VERBATIM.
	//
	// On this substrate that is the WHOLE of Outcome.ResultText: the §60 order
	// puts the terminal envelope's own result text first, and this engine emits
	// no terminal envelope at all, so limb (1) has no source here and limb (2)
	// is the entire story. Nothing repairs, completes or fabricates it.
	finalText string

	messageIndex int64
	unknownLines int
}

func newParser(logf func(format string, args ...any)) *parser {
	return &parser{logf: logf}
}

// feed consumes one line and returns the contract events it completes.
func (p *parser) feed(line []byte) []adapters.Event {
	if len(strings.TrimSpace(string(line))) == 0 {
		return nil
	}
	var sl streamLine
	if err := json.Unmarshal(line, &sl); err != nil {
		p.unknownLines++
		p.logf("kimicli: skipping malformed stream line: %v", err)
		return nil
	}
	switch sl.Role {
	case "assistant":
		return p.feedAssistant(sl)
	case "tool":
		return p.feedTool(sl)
	case "meta":
		return p.feedMeta(sl)
	default:
		p.unknownLines++
		p.logf("kimicli: skipping unknown stream frame role %q type %q", sl.Role, sl.Type)
		return nil
	}
}

func (p *parser) feedAssistant(sl streamLine) []adapters.Event {
	text := decodeText(sl.Content)
	// A text-LESS assistant frame is the ordinary tail of a tool-using turn.
	// Zeroing the retained answer there would discard the answer the engine
	// really did stream and then report it as "no assistant text" — the
	// P3-RW-9 drain r1 lesson, which cost a packet once already.
	if text != "" {
		p.finalText = text
	}
	p.messageIndex++
	if p.onFlush != nil {
		p.onFlush()
	}

	type toolRef struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	refs := make([]toolRef, 0, len(sl.ToolCalls))
	for _, c := range sl.ToolCalls {
		refs = append(refs, toolRef{c.ID, c.Function.Name})
	}
	excerpt, truncated := excerptOf(text)
	payload, err := json.Marshal(struct {
		MessageIndex int64     `json:"message_index"`
		Excerpt      string    `json:"excerpt"`
		Truncated    bool      `json:"truncated,omitempty"`
		ToolUses     []toolRef `json:"tool_uses,omitempty"`
	}{p.messageIndex, excerpt, truncated, refs})
	if err != nil {
		return nil
	}
	return []adapters.Event{{Kind: adapters.KindMessage, Payload: payload}}
}

func (p *parser) feedTool(sl streamLine) []adapters.Event {
	excerpt, truncated := excerptOf(decodeText(sl.Content))
	payload, err := json.Marshal(struct {
		ToolUseID string `json:"tool_use_id,omitempty"`
		Excerpt   string `json:"excerpt"`
		Truncated bool   `json:"truncated,omitempty"`
	}{sl.ToolCallID, excerpt, truncated})
	if err != nil {
		return nil
	}
	return []adapters.Event{{Kind: adapters.KindToolResult, Payload: payload}}
}

func (p *parser) feedMeta(sl streamLine) []adapters.Event {
	switch sl.Type {
	case "system.version":
		p.version = sl.Version
		return nil
	case "session.resume_hint":
		if sl.SessionID != "" {
			p.sessionID = sl.SessionID
			if p.onSession != nil {
				p.onSession(sl.SessionID)
			}
		}
		return nil
	case "turn.step.retrying":
		return p.limitEvent(sl)
	default:
		p.unknownLines++
		p.logf("kimicli: skipping unknown meta frame type %q", sl.Type)
		return nil
	}
}

// limitEvent forwards the engine's retry narration as a rate_limit observation.
//
// It is the only structured error signal this transport puts on stdout, and it
// is worth forwarding because the alternative is worse: the engine retries
// internally, so without this the platform sees nothing at all until the
// attempts are exhausted and the run dies with a message on stderr.
func (p *parser) limitEvent(sl streamLine) []adapters.Event {
	if sl.StatusCode == 0 && sl.ErrorMessage == "" {
		return nil
	}
	payload := p.signalPayload(sl.ErrorMessage, sl.StatusCode)
	if payload == nil {
		raw, err := json.Marshal(struct {
			HTTPStatus int    `json:"http_status,omitempty"`
			BodyText   string `json:"body_text,omitempty"`
			Lane       string `json:"lane,omitempty"`
		}{sl.StatusCode, sl.ErrorMessage, adapters.LaneKimiCLI})
		if err != nil {
			return nil
		}
		payload = raw
	}
	return []adapters.Event{{Kind: adapters.KindRateLimit, Payload: payload}}
}

// signalPayload asks the lane document what it makes of this (status, message).
// A nil seam yields nothing and the caller forwards the raw facts instead.
func (p *parser) signalPayload(body string, status int) json.RawMessage {
	if p.signals == nil || (body == "" && status == 0) {
		return nil
	}
	payload, ok := p.signals(body, status)
	if !ok {
		return nil
	}
	return payload
}

// decodeText tolerates a string content member and anything else.
func decodeText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// excerptOf bounds free text at adapters.ExcerptCap (refs-not-blobs, P-T07-5).
func excerptOf(s string) (string, bool) {
	if len(s) <= adapters.ExcerptCap {
		return s, false
	}
	return s[:adapters.ExcerptCap], true
}

// bounded caps a raw JSON value for event payloads.
func bounded(raw json.RawMessage) json.RawMessage {
	if len(raw) <= adapters.ExcerptCap {
		return raw
	}
	stub, err := json.Marshal(struct {
		Oversize bool   `json:"oversize"`
		Bytes    int    `json:"bytes"`
		Note     string `json:"note"`
	}{true, len(raw), "full content in the engine transcript / copy-aside (P-T07-5)"})
	if err != nil {
		return json.RawMessage(`{"oversize":true}`)
	}
	return stub
}
