package opencode

// fakeprovider_test.go — a loopback OpenAI-compatible provider for the tier-R
// probe. It is the reason tier R costs $0: a REAL `opencode serve` drives a
// REAL turn, but the model call terminates on 127.0.0.1 with a scripted
// completion. No credential, no provider network call, no lane (§1 scope wall).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeProvider struct {
	srv *httptest.Server

	mu    sync.Mutex
	calls []fakeProviderCall
}

type fakeProviderCall struct {
	Tools []string
	Msgs  string
}

func newFakeProvider(t *testing.T) *fakeProvider {
	t.Helper()
	p := &fakeProvider{}
	p.srv = httptest.NewServer(http.HandlerFunc(p.route))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *fakeProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func (p *fakeProvider) toolsOffered() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.calls {
		if len(c.Tools) > 0 {
			return c.Tools
		}
	}
	return nil
}

func (p *fakeProvider) route(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, map[string]any{"object": "list", "data": []any{
			map[string]any{"id": "fakemodel", "object": "model", "owned_by": "fake"}}})
		return
	}
	var body struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	raw := make([]byte, 1<<20)
	n, _ := r.Body.Read(raw)
	_ = json.Unmarshal(raw[:n], &body)

	tools := make([]string, 0, len(body.Tools))
	for _, t := range body.Tools {
		tools = append(tools, t.Function.Name)
	}
	sawToolResult := false
	for _, m := range body.Messages {
		if m.Role == "tool" {
			sawToolResult = true
		}
	}
	p.mu.Lock()
	p.calls = append(p.calls, fakeProviderCall{Tools: tools, Msgs: string(raw[:n])})
	p.mu.Unlock()

	usage := map[string]any{
		"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18,
		"prompt_tokens_details":     map[string]any{"cached_tokens": 3},
		"completion_tokens_details": map[string]any{"reasoning_tokens": 2},
	}
	want := ""
	for _, name := range tools {
		if name == "bash" {
			want = name
		}
	}
	var delta map[string]any
	finish := "stop"
	if want != "" && !sawToolResult {
		delta = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{
			map[string]any{"index": 0, "id": "call_fake1", "type": "function",
				"function": map[string]any{"name": want,
					"arguments": `{"command":"echo hello-from-fake","description":"say hi"}`}}}}
		finish = "tool_calls"
	} else {
		delta = map[string]any{"role": "assistant", "content": "fake-done"}
	}
	chunk := func(o any) []byte {
		b, _ := json.Marshal(o)
		return b
	}
	if body.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		writeChunk := func(o any) {
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(chunk(o))
			_, _ = w.Write([]byte("\n\n"))
			if fl != nil {
				fl.Flush()
			}
		}
		base := map[string]any{"id": "chatcmpl-fake", "object": "chat.completion.chunk",
			"created": time.Now().Unix(), "model": "fakemodel"}
		first := map[string]any{}
		for k, v := range base {
			first[k] = v
		}
		first["choices"] = []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}
		writeChunk(first)
		last := map[string]any{}
		for k, v := range base {
			last[k] = v
		}
		last["choices"] = []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}
		last["usage"] = usage
		writeChunk(last)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if fl != nil {
			fl.Flush()
		}
		return
	}
	writeJSON(w, map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion", "created": time.Now().Unix(),
		"model": "fakemodel", "request_id": "req_fake",
		"choices": []any{map[string]any{"index": 0, "message": delta, "finish_reason": finish}},
		"usage":   usage,
	})
}

func (p *fakeProvider) baseURL() string { return strings.TrimSuffix(p.srv.URL, "/") }
