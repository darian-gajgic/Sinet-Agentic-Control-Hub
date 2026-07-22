package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// client.go — the Spec S12.1 platform-plane `/v1` client (brief R16). This IS
// the platform plane (S12.6): loopback, direct to llama-swap, no per-run
// tokens, no sandbox bridge (that plane is B4-6). It speaks the
// OpenAI-compatible surface llama-swap fronts (llama-swap → llama-server).
//
// Request shaping enforces the S12.4 cross-cutting rules: temperature 0 /
// greedy for classification-shaped duties; the schema appears in the PROMPT
// (the caller composes it) AND is enforced at the engine via
// `response_format: json_schema` (engine-enforced validity — GBNF where a
// schema cannot express it; the conformance suite proves the constraint holds
// at the pin, R26, never assumed from docs); logprobs requested/exposed (the
// only v0 logprob lane — S03.2/SPIKE P2-S1; consumers S12.5 margins B4-7 +
// the drift canary B5). Per-duty length caps ride ChatSpec.MaxTokens
// (structural constants set by the duty layer). The abstain member lives in
// the caller's schema; duty.go asserts every duty schema carries one.

// ChatSpec is one shaped /v1/chat/completions request (the duty layer builds
// it; the client applies temp-0/logprobs/json_schema).
type ChatSpec struct {
	Model  string
	System string // free-text reasoning-then-constrained framing precedes the constrained region
	User   string // the duty prompt (carries the schema in-prompt too, S12.4)
	// Messages, when non-nil, is the verbatim message list — used by the S12.6
	// sandboxed plane, which forwards a worker's own multi-turn request rather
	// than composing System/User. Ignored on the platform plane (System/User).
	Messages  []chatMessage
	Schema    json.RawMessage // the json_schema enforced at the engine (nil ⇒ free text, e.g. utility drafting)
	Name      string          // json_schema name
	MaxTokens int64           // per-duty length cap (structural constant)
	// Logprobs requests the label-token logprobs (S12.5 margin input). On for
	// classification-shaped duties; off for free-text drafting.
	Logprobs bool
	// NoThink disables a reasoning model's <think> phase for this call
	// (chat_template_kwargs enable_thinking:false — honored by Qwen-class
	// thinking templates, ignored by others). Set for classification-shaped
	// duties: they are temp-0 greedy label emitters, and an unbounded thinking
	// phase would consume the length cap before the constrained JSON (observed
	// live at the tier-L smoke — the schema's `reason` field IS the lightweight
	// reasoning). Free-text drafting leaves it off.
	NoThink bool
}

// TokenLogprob is one decoded token's logprob + its top alternatives — the
// S12.5 margin (top1−top2) input (B4-7 consumes it; not computed here).
type TokenLogprob struct {
	Token   string       `json:"token"`
	Logprob float64      `json:"logprob"`
	Top     []topLogprob `json:"top_logprobs"`
}

type topLogprob struct {
	Token   string  `json:"token"`
	Logprob float64 `json:"logprob"`
}

// Completion is the parsed /v1 response the duty layer consumes.
type Completion struct {
	Content      string
	Logprobs     []TokenLogprob
	InputTokens  int64
	OutputTokens int64
	FinishReason string
}

// Client is the loopback /v1 + management client for the pinned llama-swap.
type Client struct {
	base string // e.g. http://127.0.0.1:8791 (loopback, structural config)
	http *http.Client
}

// NewClient returns a client for the llama-swap base URL (loopback). The
// timeout is a generous fixed default — local generation can be slow on a cold
// load (the S12.8 ~2–7 s reloads plus generation).
func NewClient(base string) *Client {
	base = strings.TrimRight(base, "/")
	return &Client{base: base, http: &http.Client{Timeout: 5 * time.Minute}}
}

// Base is the client's endpoint (test/observability).
func (c *Client) Base() string { return c.base }

type chatRequest struct {
	Model              string          `json:"model"`
	Messages           []chatMessage   `json:"messages"`
	Temperature        float64         `json:"temperature"`
	MaxTokens          int64           `json:"max_tokens,omitempty"`
	ResponseFormat     *responseFormat `json:"response_format,omitempty"`
	Logprobs           bool            `json:"logprobs,omitempty"`
	TopLogprobs        int             `json:"top_logprobs,omitempty"`
	ChatTemplateKwargs map[string]any  `json:"chat_template_kwargs,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type       string          `json:"type"`
	JSONSchema *jsonSchemaSpec `json:"json_schema,omitempty"`
}

type jsonSchemaSpec struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Logprobs *struct {
			Content []TokenLogprob `json:"content"`
		} `json:"logprobs"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat runs one /v1/chat/completions with the S12.4 shaping applied: greedy
// (temperature 0), the free-text-then-constrained framing (System precedes
// User), engine-enforced json_schema (when Schema != nil), and logprobs when
// requested. Returns the parsed Completion.
func (c *Client) Chat(ctx context.Context, spec ChatSpec) (Completion, error) {
	req := chatRequest{
		Model:       spec.Model,
		Temperature: 0, // greedy for every classification-shaped duty (S12.4)
		MaxTokens:   spec.MaxTokens,
	}
	if spec.Messages != nil {
		req.Messages = spec.Messages // the S12.6 sandboxed plane forwards verbatim
	} else {
		if spec.System != "" {
			req.Messages = append(req.Messages, chatMessage{Role: "system", Content: spec.System})
		}
		req.Messages = append(req.Messages, chatMessage{Role: "user", Content: spec.User})
	}
	if len(spec.Schema) > 0 {
		name := spec.Name
		if name == "" {
			name = "duty"
		}
		req.ResponseFormat = &responseFormat{
			Type:       "json_schema",
			JSONSchema: &jsonSchemaSpec{Name: name, Schema: spec.Schema, Strict: true},
		}
	}
	if spec.Logprobs {
		req.Logprobs = true
		req.TopLogprobs = 2 // top1/top2 — the S12.5 margin input
	}
	if spec.NoThink {
		// Disable a reasoning model's <think> phase so the length cap is spent
		// on the constrained JSON, not reasoning (tier-L observed).
		req.ChatTemplateKwargs = map[string]any{"enable_thinking": false}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return Completion{}, fmt.Errorf("local: marshal chat request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Completion{}, fmt.Errorf("local: build chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Completion{}, fmt.Errorf("%w: %v", ErrStackAbsent, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Completion{}, fmt.Errorf("local: read chat response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Completion{}, fmt.Errorf("local: llama-swap /v1 returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return Completion{}, fmt.Errorf("local: decode chat response: %w", err)
	}
	if cr.Error != nil {
		return Completion{}, fmt.Errorf("local: llama-swap /v1 error: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return Completion{}, fmt.Errorf("local: llama-swap /v1 returned no choices")
	}
	out := Completion{
		Content:      cr.Choices[0].Message.Content,
		InputTokens:  cr.Usage.PromptTokens,
		OutputTokens: cr.Usage.CompletionTokens,
		FinishReason: cr.Choices[0].FinishReason,
	}
	if lp := cr.Choices[0].Logprobs; lp != nil {
		out.Logprobs = lp.Content
	}
	return out, nil
}

// Unload calls llama-swap's management API to unload a specific model
// (POST /api/models/unload/:model) — the eager-unload verb's unload leg
// (R12; route verified at the pinned build). Management verbs exist ONLY on
// this platform plane (loopback), S12.6.
func (c *Client) Unload(ctx context.Context, model string) error {
	return c.post(ctx, "/api/models/unload/"+url(model))
}

// UnloadAll calls POST /api/models/unload — unload every running model (the
// eager-unload operator-wins switch, S12.2 §4.5).
func (c *Client) UnloadAll(ctx context.Context) error {
	return c.post(ctx, "/api/models/unload")
}

func (c *Client) post(ctx context.Context, path string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, nil)
	if err != nil {
		return fmt.Errorf("local: build %s: %w", path, err)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStackAbsent, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("local: %s returned %d", path, resp.StatusCode)
	}
	return nil
}

// url path-escapes a model id for the unload route (model ids may carry
// spaces, e.g. "Gemma 4 12B QAT").
func url(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == '~':
			b.WriteRune(r)
		default:
			for _, by := range []byte(string(r)) {
				fmt.Fprintf(&b, "%%%02X", by)
			}
		}
	}
	return b.String()
}
