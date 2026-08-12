package claudecli

// resulttext_test.go — P3-RW-9 T1–T4: tier-F conformance for the terminal
// result TEXT (Spec S03.1 forward tolerance, S03.2 Anthropic lane).
//
// What these fixtures record (provenance, CONVENTIONS §10): the flake
// reproduced live in the B6-clickthrough world as task `t-59f2a72bab7f8109`,
// intake generation 4 — two back-to-back plan-draft sessions
// (`9edcfad3…`, `3f0c106d…`), read 2026-08-12 from a COPY of that world's
// platform.db and its transcript copy-asides (cp-169 / cp-177), never the
// live files. Both sessions ended `subtype:"success"`, `is_error:false`,
// `stop_reason:"end_turn"`, both copy-asides ended in a FULL final assistant
// text (4971 / 4088 bytes of the pair JSON), and the un-truncated
// `engine.message` excerpt of the second proved stream and transcript agreed
// byte-for-byte. Two observed facts drive the fixtures:
//
//  1. the terminal envelope's `result` field can be EMPTY on a clean
//     completion (the upstream empty-result bug class — issues #38805,
//     #38706, #39053, #7124; CHANGELOG fixes at 2.1.203 / 2.1.214 are BELOW
//     our pin 2.1.218 and the class still reproduces);
//  2. the recovered text is exactly ONE closing brace short of parsable —
//     engine/model-side loss, NOT our pump.
//
// So the adapter hands the text through VERBATIM and never repairs it: a
// brace-short pair still fails the planner's JSON contract, and that failure
// is the consumer's to make loud (P3-RW-9 R4's corpse), not the adapter's to
// paper over. Session ids and prose in the fixtures are synthetic
// re-creations; the byte lengths, the envelope field set and the missing
// brace are the recorded facts. These four tests are the tripwire for the
// next deliberate S03.3 pin bump.

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// loggedSession is outcomeSession with the adapter's ops log captured: the
// substitution MUST be loud (R1/R2 — text loss is never silent).
func loggedSession(t *testing.T) (*session, *bytes.Buffer) {
	t.Helper()
	s := outcomeSession(t)
	buf := &bytes.Buffer{}
	s.a.Log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return s, buf
}

// fixtureAssistantText re-reads the fixture's assistant frames and returns the
// accumulated text — the independent statement of what "the stream's final
// assistant message" holds, so the assertions never compare the parser to
// itself.
func fixtureAssistantText(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	var text strings.Builder
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var sl struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(line, &sl); err != nil {
			t.Fatalf("fixture line: %v", err)
		}
		if sl.Type != "assistant" {
			continue
		}
		for _, b := range sl.Message.Content {
			if b.Type == "text" {
				text.WriteString(b.Text)
			}
		}
	}
	return text.String()
}

// T1 — the reproduction, replayed: a clean completion whose `result` field is
// empty yields the stream's own final assistant text, verbatim and unrepaired.
func TestOutcomeEmptyResultUsesFinalAssistantText(t *testing.T) {
	s, logs := loggedSession(t)
	want := fixtureAssistantText(t, "emptyresult.jsonl")
	if len(want) != 4971 {
		t.Fatalf("fixture text is %d bytes, want the recorded 4971", len(want))
	}
	res := feedFixture(t, "emptyresult.jsonl")
	if res.p.result.Result != "" {
		t.Fatalf("fixture result field = %q, want empty (the recorded flake)", res.p.result.Result)
	}
	out, extra := s.assembleOutcome(res.p, nil)

	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q, want completed — the engine reported success (D3 honesty)", out.Kind)
	}
	if out.ResultText != want {
		t.Fatalf("ResultText = %d bytes, want the stream's final assistant text (%d bytes)",
			len(out.ResultText), len(want))
	}
	// Verbatim means UNREPAIRED: the recovered text is still one closing brace
	// short, exactly as the engine emitted it.
	var any any
	if err := json.Unmarshal([]byte(out.ResultText), &any); err == nil {
		t.Fatal("ResultText parsed as JSON — the adapter repaired the engine's text (R1: hand through, never repair)")
	}
	if err := json.Unmarshal([]byte(out.ResultText+"}"), &any); err != nil {
		t.Fatalf("ResultText + one brace still does not parse (%v) — the fixture no longer records the observed loss", err)
	}
	if got := logs.String(); !strings.Contains(got, "EMPTY result field") ||
		!strings.Contains(got, "final assistant message") {
		t.Fatalf("the substitution was silent; ops log = %q", got)
	}
	if len(extra) != 1 || extra[0].Kind != adapters.KindDone {
		t.Fatalf("extra events = %v, want one engine.done", kinds(extra))
	}
}

// T2 — precedence regression pin: whenever the engine behaves, semantics are
// byte-identical to before the fallback existed.
func TestOutcomeResultFieldWinsOverStreamText(t *testing.T) {
	s, logs := loggedSession(t)
	res := feedFixture(t, "resultwins.jsonl")
	stream := fixtureAssistantText(t, "resultwins.jsonl")
	want := res.p.result.Result
	if want == "" || want == stream {
		t.Fatalf("fixture must carry a NON-empty result field distinct from the stream text (result=%q stream=%q)", want, stream)
	}
	out, _ := s.assembleOutcome(res.p, nil)
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q, want completed", out.Kind)
	}
	if out.ResultText != want {
		t.Fatalf("ResultText = %q, want the result field %q (the fallback must not be consulted)", out.ResultText, want)
	}
	if strings.Contains(logs.String(), "EMPTY result field") {
		t.Fatalf("a healthy completion logged the empty-result notice: %q", logs.String())
	}
}

// T3 — genuinely empty stays honest: no text anywhere means no text, loudly.
// The adapter never fabricates, and never mints a crash for an outcome the
// engine reported as success (R2).
func TestOutcomeEmptyResultWithoutStreamTextStaysEmpty(t *testing.T) {
	s, logs := loggedSession(t)
	res := feedFixture(t, "emptynotext.jsonl")
	out, _ := s.assembleOutcome(res.p, nil)
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q, want completed — the adapter never mints a crash here (R2)", out.Kind)
	}
	if out.ResultText != "" {
		t.Fatalf("ResultText = %q, want empty (nothing was streamed; the adapter fabricates nothing)", out.ResultText)
	}
	if got := logs.String(); !strings.Contains(got, "EMPTY result field") ||
		!strings.Contains(got, "no assistant text") {
		t.Fatalf("the empty completion was silent; ops log = %q", got)
	}
}

// T4 — bounded persisted payloads hold: the retained text rides ONLY the
// in-process Outcome (R3, P-T07-5 refs-not-blobs, CONVENTIONS §10 ExcerptCap).
func TestEmptyResultFallbackKeepsPayloadsBounded(t *testing.T) {
	s, _ := loggedSession(t)
	res := feedFixture(t, "emptyresult.jsonl")
	out, extra := s.assembleOutcome(res.p, nil)

	if len(out.ResultText) <= adapters.ExcerptCap {
		t.Fatalf("fixture text is %d bytes — T4 needs one larger than ExcerptCap (%d)",
			len(out.ResultText), adapters.ExcerptCap)
	}
	// The engine.message event: capped excerpt + truncated flag, unchanged.
	var msg struct {
		Excerpt   string `json:"excerpt"`
		Truncated bool   `json:"truncated"`
	}
	found := false
	for _, ev := range res.events {
		if ev.Kind != adapters.KindMessage {
			continue
		}
		found = true
		if err := json.Unmarshal(ev.Payload, &msg); err != nil {
			t.Fatalf("message payload: %v", err)
		}
		if len(msg.Excerpt) != adapters.ExcerptCap || !msg.Truncated {
			t.Fatalf("message excerpt = %d bytes truncated=%v, want the %d-byte capped+flagged excerpt",
				len(msg.Excerpt), msg.Truncated, adapters.ExcerptCap)
		}
		if len(ev.Payload) > adapters.ExcerptCap+512 {
			t.Fatalf("engine.message payload grew to %d bytes — the full text leaked into a persisted payload", len(ev.Payload))
		}
	}
	if !found {
		t.Fatal("no engine.message event in the replay")
	}
	// Outcome.Result (the bounded cross-check envelope) and the engine.done
	// payload both stay small — the recovered text is nowhere near them.
	if len(out.Result) > adapters.ExcerptCap {
		t.Fatalf("Outcome.Result = %d bytes, want bounded", len(out.Result))
	}
	if strings.Contains(string(out.Result), out.ResultText[:64]) {
		t.Fatal("the recovered text leaked into Outcome.Result (R3: it rides ONLY ResultText)")
	}
	for _, ev := range extra {
		if strings.Contains(string(ev.Payload), out.ResultText[:64]) {
			t.Fatalf("the recovered text leaked into a %s payload (R3)", ev.Kind)
		}
	}
}
