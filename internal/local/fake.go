package local

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// fake.go — the tier-F httptest fake of llama-swap's surface (brief R25),
// shipped as a shared test double so BOTH internal/local and internal/stage
// tests run the client/seam/metering/config paths hermetically (zero network,
// zero paid calls). It fakes /v1/chat/completions (honoring the
// constrained-decoding request shape, model field, logprobs) and the unload
// endpoints (/api/models/unload[/:model]) sufficient to exercise the eager-
// unload logic and every duty schema round-trip. The REAL behavioral asserts
// (json_schema enforcement actually constrains, logprobs really present) are
// tier R/L against the pinned stack (conformance.go); this fake exercises the
// Sinet-side logic.

// FakeRequest captures a /v1/chat/completions request for shape assertions
// (temp-0, json_schema present, logprobs requested — proving the client
// SHAPES per S12.4).
type FakeRequest struct {
	Model         string
	Temperature   float64
	HasJSONSchema bool
	SchemaName    string
	Schema        json.RawMessage
	Logprobs      bool
	MaxTokens     int64
	Messages      []struct{ Role, Content string }
}

// FakeResponse is the configurable /v1 response a test wants the fake "model"
// to return.
type FakeResponse struct {
	Content      string
	Logprobs     []TokenLogprob
	InputTokens  int64
	OutputTokens int64
	// Status, when non-zero, overrides the 200 OK (unhealthy-stack tests).
	Status int
}

// FakeServer is an httptest fake of the llama-swap surface.
type FakeServer struct {
	*httptest.Server
	mu       sync.Mutex
	lastReq  FakeRequest
	response FakeResponse
	unloaded []string
	byModel  map[string]FakeResponse // per-model response override
}

// NewFakeServer starts a fake with a default schema-valid response.
func NewFakeServer() *FakeServer {
	fs := &FakeServer{
		response: FakeResponse{Content: `{"abstain":true}`, InputTokens: 12, OutputTokens: 3},
		byModel:  map[string]FakeResponse{},
	}
	fs.Server = httptest.NewServer(http.HandlerFunc(fs.handle))
	return fs
}

// SetResponse sets the default /v1 response.
func (fs *FakeServer) SetResponse(r FakeResponse) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.response = r
}

// SetModelResponse sets a per-model /v1 response override (duty round-trips).
func (fs *FakeServer) SetModelResponse(model string, r FakeResponse) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.byModel[model] = r
}

// LastRequest returns the last /v1 request seen (shape assertions).
func (fs *FakeServer) LastRequest() FakeRequest {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.lastReq
}

// Unloaded returns the models unloaded via the management API ("*" = all).
func (fs *FakeServer) Unloaded() []string {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return append([]string(nil), fs.unloaded...)
}

func (fs *FakeServer) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		fs.handleChat(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/models/unload":
		fs.mu.Lock()
		fs.unloaded = append(fs.unloaded, "*")
		fs.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/models/unload/"):
		fs.mu.Lock()
		fs.unloaded = append(fs.unloaded, strings.TrimPrefix(r.URL.Path, "/api/models/unload/"))
		fs.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}

func (fs *FakeServer) handleChat(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	var req struct {
		Model          string        `json:"model"`
		Temperature    float64       `json:"temperature"`
		MaxTokens      int64         `json:"max_tokens"`
		Logprobs       bool          `json:"logprobs"`
		Messages       []chatMessage `json:"messages"`
		ResponseFormat *struct {
			Type       string `json:"type"`
			JSONSchema *struct {
				Name   string          `json:"name"`
				Schema json.RawMessage `json:"schema"`
			} `json:"json_schema"`
		} `json:"response_format"`
	}
	_ = json.Unmarshal(raw, &req)

	captured := FakeRequest{Model: req.Model, Temperature: req.Temperature, Logprobs: req.Logprobs, MaxTokens: req.MaxTokens}
	if req.ResponseFormat != nil && req.ResponseFormat.JSONSchema != nil {
		captured.HasJSONSchema = req.ResponseFormat.Type == "json_schema"
		captured.SchemaName = req.ResponseFormat.JSONSchema.Name
		captured.Schema = req.ResponseFormat.JSONSchema.Schema
	}
	for _, m := range req.Messages {
		captured.Messages = append(captured.Messages, struct{ Role, Content string }{m.Role, m.Content})
	}

	fs.mu.Lock()
	fs.lastReq = captured
	resp, ok := fs.byModel[req.Model]
	if !ok {
		resp = fs.response
	}
	fs.mu.Unlock()

	if resp.Status != 0 && resp.Status != http.StatusOK {
		w.WriteHeader(resp.Status)
		_, _ = io.WriteString(w, `{"error":{"message":"fake unhealthy"}}`)
		return
	}

	out := map[string]any{
		"choices": []map[string]any{{
			"message":       map[string]any{"role": "assistant", "content": resp.Content},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{
			"prompt_tokens":     resp.InputTokens,
			"completion_tokens": resp.OutputTokens,
		},
	}
	if req.Logprobs && len(resp.Logprobs) > 0 {
		out["choices"].([]map[string]any)[0]["logprobs"] = map[string]any{"content": resp.Logprobs}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
