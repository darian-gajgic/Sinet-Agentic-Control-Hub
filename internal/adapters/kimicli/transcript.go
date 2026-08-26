package kimicli

import (
	"encoding/json"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// The usage source, and why it is not the stream.
//
// D7 and S10.1 require one usage row per paid model call, written in the same
// transaction as its checkpoint. The R0 spike established by execution that
// `--output-format stream-json` carries NO token or cost field in ANY frame —
// six distinct shapes were observed across a plain turn, a tool turn, a denied
// turn, a retried-error turn, a terminal-error turn, a cancelled turn and two
// resumed turns, and none of them has a usage member. The engine asks its own
// provider for usage (`stream_options: {include_usage: true}` is on every
// request) and simply does not print it.
//
// It DOES record it. Each run's own session transcript,
// $KIMI_CODE_HOME/sessions/<workDirKey>/<sessionId>/agents/main/wire.jsonl,
// carries one `usage.record` per model call, already decomposed the way
// S02.4(a) wants it, and the file is appended LIVE: on a run whose first tool
// call slept 12 s the first record was on disk within ~2 s while the run was
// still in flight. So checkpoints fire per paid call as it happens, not in a
// burst at exit.
//
// That transcript sits inside the per-RUN KIMI_CODE_HOME this platform creates
// and owns, which also makes the path unambiguous: a fresh home holds exactly
// one session directory, so it resolves without the session id the stream only
// reports at the end.
//
// This is a correction to the brief, recorded rather than absorbed: the packet
// was conditioned on finding usage IN THE STREAM, with `kimi web` as the
// fallback BECAUSE an adapter that cannot emit adapters.Usage cannot satisfy
// D7. It can, here, on the transport already chosen — so no transport flip was
// taken and none was needed.

// wireLine is the tolerant decode of one transcript record.
type wireLine struct {
	Type    string `json:"type"`
	AgentID string `json:"agentId"`

	// llm.request carries the REAL model id. usage.record does not.
	Model      string `json:"model"`
	ModelAlias string `json:"modelAlias"`
	TurnStep   string `json:"turnStep"`

	// usage.record
	Usage      *wireUsage `json:"usage"`
	UsageScope string     `json:"usageScope"`
	Time       int64      `json:"time"`
}

// wireUsage is the engine's own decomposition.
//
// `inputOther` EXCLUDES cache reads — verified by arithmetic against a loopback
// provider that returned prompt_tokens 137 with cached_tokens 64: the engine
// recorded inputOther 73, inputCacheRead 64, and 73+64+0 = 137. So the Anthropic
// accounting normalization (total prompt = cache_read + cache_creation +
// input_tokens) has the SAME field semantics here. R6 required that be measured
// rather than assumed, because a mis-normalized row double- or under-counts
// (P-T08-1); it was, and it matches.
type wireUsage struct {
	InputOther         int64 `json:"inputOther"`
	Output             int64 `json:"output"`
	InputCacheRead     int64 `json:"inputCacheRead"`
	InputCacheCreation int64 `json:"inputCacheCreation"`
}

// transcript folds the run's own wire.jsonl into Usage events.
type transcript struct {
	logf func(format string, args ...any)

	// model is the last model id seen on an llm.request frame. usage.record's
	// own `model` member is the SYNTHESIZED ALIAS (`__kimi_env_model__` on the
	// KIMI_MODEL_* channel), so reading it there would meter every run of this
	// lane under a fictional model name.
	model string

	// seen counts the usage records already emitted, which is both the message
	// index and the replay guard: the file is tailed, and a re-read that
	// re-emitted would re-bill a call that was already checkpointed.
	seen int64

	unknownLines int
}

func newTranscript(logf func(format string, args ...any)) *transcript {
	return &transcript{logf: logf}
}

// feed consumes one transcript line and returns the Usage events it completes.
// One `usage.record` is exactly one paid call, which the engine's own grouping
// gives for free — a two-call turn produced exactly two records.
func (t *transcript) feed(line []byte) []adapters.Event {
	var wl wireLine
	if err := json.Unmarshal(line, &wl); err != nil {
		t.unknownLines++
		return nil
	}
	switch wl.Type {
	case "llm.request":
		if wl.Model != "" {
			t.model = wl.Model
		}
		return nil
	case "usage.record":
		if wl.Usage == nil {
			return nil
		}
		t.seen++
		return []adapters.Event{t.usageEvent(wl)}
	default:
		// Every other record type is engine-internal narration. It is skipped
		// silently rather than logged: this file carries dozens of frames per
		// turn and a log line each would drown the ops channel.
		return nil
	}
}

func (t *transcript) usageEvent(wl wireLine) adapters.Event {
	u := &adapters.Usage{
		ModelID:             t.model,
		InputTokens:         wl.Usage.InputOther,
		OutputTokens:        wl.Usage.Output,
		CacheReadTokens:     wl.Usage.InputCacheRead,
		CacheCreationTokens: wl.Usage.InputCacheCreation,
		// One paid call is located by its position in the session. This engine
		// exposes no per-call id of its own, so the index IS the identity, and
		// it is derived rather than invented.
		MessageID:    fmt.Sprintf("%s#%d", wl.AgentID, t.seen),
		MessageIndex: t.seen,
		// Total stays false on every row: there is no terminal envelope on
		// this substrate to carry a run total, so a Total row would be a
		// fabrication. Run totals ride engine.done, never a checkpoint.
		Total: false,
		// EngineCostUSD stays zero. The engine records no cost on this
		// surface, and Sinet prices from its own table regardless (D5).
		Raw: bounded(mustJSON(wl.Usage)),
	}
	payload, err := json.Marshal(struct {
		MessageID    string          `json:"message_id"`
		MessageIndex int64           `json:"message_index"`
		Model        string          `json:"model,omitempty"`
		Scope        string          `json:"usage_scope,omitempty"`
		Usage        json.RawMessage `json:"usage"`
	}{u.MessageID, u.MessageIndex, u.ModelID, wl.UsageScope, u.Raw})
	if err != nil {
		payload = json.RawMessage(`{}`)
	}
	return adapters.Event{Kind: adapters.KindUsage, Payload: payload, Usage: u}
}

func mustJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return raw
}
