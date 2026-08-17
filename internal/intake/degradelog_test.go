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
	"context"
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

// ---- The 13.5 help seam fake ----

type fakeUtility struct {
	calls int
	block intake.HelpBlock
	err   error
}

func (f *fakeUtility) Help(context.Context, intake.Pair) (intake.HelpBlock, error) {
	f.calls++
	return f.block, f.err
}

// TestHelpDegradeIsLogged (PH-1 F3, drain): the approval card's Help block is
// the SAME seat on the SAME alias as phrasing, and it failed the same way on
// cold walk 1 — cp 13, 700 output tokens against a 700-token cap, falling back
// byte-identically to defaultHelp() with nothing in the log. This is the phrase
// probe's mirror: the fallback is honest and free, and it is now audible.
func TestHelpDegradeIsLogged(t *testing.T) {
	f := newFix(t)
	var buf bytes.Buffer
	f.p.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	util := &fakeUtility{err: errors.New("local: duty reply hit its length cap (truncated, not finished)")}
	f.p.Utility = util

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("card = %q, want the approval card", st.OpenAskKind)
	}
	_, card := f.openAsk(st.RunID)
	if util.calls == 0 {
		t.Fatal("the help seat was never consulted — this probe would prove nothing")
	}

	// The degrade is honest and costs nothing: the deterministic block, verbatim,
	// on the same single card.
	if card.Approval == nil {
		t.Fatal("no approval body on the approval card")
	}
	got := card.Approval.Layer1.Help
	if got.What == "" || got.Recommend == "" {
		t.Errorf("the help block came back empty instead of falling back to the platform's own words: %+v", got)
	}
	if !strings.HasPrefix(got.What, "Approving starts the work exactly as planned") {
		t.Errorf("help.What = %q, want the deterministic defaultHelp() text — a failed seat must not invent prose", got.What)
	}

	logged := buf.String()
	if !strings.Contains(logged, "level=WARN") {
		t.Fatalf("the help seat failed and the operator log says nothing — the same silence PH-1 shipped:\n%s", logged)
	}
	for _, want := range []string{st.RunID, "approval", "length cap", "seat=help"} {
		if !strings.Contains(logged, want) {
			t.Errorf("help degrade WARN does not carry %q (run id, card, cause, seat):\n%s", want, logged)
		}
	}
	if strings.Contains(logged, "Please fix the widget in the repo") {
		t.Errorf("requester text leaked into the degrade log (S01.11):\n%s", logged)
	}
}

// TestHelpSuccessIsQuiet: a working help seat writes no warning, and its block
// is the one that reaches the card.
func TestHelpSuccessIsQuiet(t *testing.T) {
	f := newFix(t)
	var buf bytes.Buffer
	f.p.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f.p.Utility = &fakeUtility{block: intake.HelpBlock{
		What: "This starts the widget repair.", Wrong: "The wrong widget could be touched.", Recommend: "Approve if the widget named is the broken one.",
	}}

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("card = %q, want the approval card", st.OpenAskKind)
	}
	_, card := f.openAsk(st.RunID)
	if card.Approval.Layer1.Help.What != "This starts the widget repair." {
		t.Errorf("the working seat's help did not reach the card: %+v", card.Approval.Layer1.Help)
	}
	if strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("a working help seat produced a WARN:\n%s", buf.String())
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
