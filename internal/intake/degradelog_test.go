package intake_test

// degradelog_test.go — PH-1 F3: a degradation the requester can SEE must never
// be invisible to the operator.
//
// The phrase seat failed on every card of cold walk 1 and the control log held
// zero lines about it: `if err != nil { return card }` and nothing else. The
// card honestly told the requester it was showing stock wording, and the one
// person who could have fixed it had nothing to grep for
// (P3/design/ph1-phrase-fallback-diagnosis-2026-08-17.md §4). The fallback
// posture is right; the silence was the defect.

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// TestPhraseDegradeIsLogged (PH-1 F3): the seat fails, the card still ships the
// taxonomy's own words with no added click — and a WARN names the run, the
// card, and the cause.
func TestPhraseDegradeIsLogged(t *testing.T) {
	f := newFix(t)
	var buf bytes.Buffer
	f.p.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f.p.Classifier = nil
	f.p.Registry = registryFamily(intake.FamilySoftware, map[string]string{"units": "millimetres"})
	f.p.Phraser = &fakePhraser{err: errors.New("local: duty reply hit its length cap (truncated, not finished)")}

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("card = %q, want interview", st.OpenAskKind)
	}
	_, card := f.openAsk(st.RunID)

	// The degrade posture is unchanged: stock wording, no extra click.
	for _, q := range card.Questions {
		if q.Phrased != "" {
			t.Errorf("question %q carries a phrasing though the seat failed", q.ID)
		}
	}

	logged := buf.String()
	if !strings.Contains(logged, "level=WARN") {
		t.Fatalf("the phrase seat failed and the operator log says nothing — that silence is PH-1 itself:\n%s", logged)
	}
	for _, want := range []string{st.RunID, "interview", "length cap"} {
		if !strings.Contains(logged, want) {
			t.Errorf("degrade WARN does not carry %q (run id, card, cause):\n%s", want, logged)
		}
	}
	// The requester's own words are material, never log content (S01.11).
	if strings.Contains(logged, "Please fix the widget in the repo") {
		t.Errorf("requester text leaked into the degrade log (S01.11):\n%s", logged)
	}
}

// TestPhraseSuccessIsQuiet: the loud path stays rare — a working seat writes no
// warning at all.
func TestPhraseSuccessIsQuiet(t *testing.T) {
	f := newFix(t)
	var buf bytes.Buffer
	f.p.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f.p.Classifier = nil
	f.p.Registry = registryFamily(intake.FamilySoftware, nil)
	f.p.Phraser = &fakePhraser{fn: func(in intake.PhraseInput) (intake.PhraseResult, error) {
		out := intake.PhraseResult{Phrasings: map[string]string{}, Summary: "You want the widget fixed."}
		for _, q := range in.Questions {
			out.Phrasings[q.ID] = "plainer: " + q.Text
		}
		return out, nil
	}}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if _, card := f.openAsk(st.RunID); card.Questions[0].Phrased == "" {
		t.Fatal("the working seat's phrasing did not reach the card")
	}
	if strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("a working phrase seat produced a WARN:\n%s", buf.String())
	}
}
