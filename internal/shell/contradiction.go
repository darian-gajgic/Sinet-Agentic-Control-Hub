package shell

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
)

// contradiction.go — the S09.7 contradiction-screen adapter (brief R16), LIVE
// on the contradiction-screen duty. It lives at the composition ROOT (the
// shell) rather than internal/stage because it needs internal/memory, and the
// §17 wall bars stage from importing memory (engine-facing code holds no
// knowledge-write capability). The shell already imports memory + local + stage,
// so the adapter composes cleanly here, reusing stage.AdvisoryMeter +
// stage.BeginAdvisory for the $0 D7 metering (R18).

type contradictionScreen struct {
	duty  *local.Duty
	meter stage.AdvisoryMeter
}

var _ memory.ContradictionScreen = (*contradictionScreen)(nil)

// newContradictionScreen wraps the duty caller as the S09.7 advisory screen
// (R16). Nil-safe: a nil duty (unconfigured stack) yields a nil screen, and the
// gate's deterministic same-topic_key detection runs regardless (never
// suppressed).
func newContradictionScreen(duty *local.Duty, meter stage.AdvisoryMeter) memory.ContradictionScreen {
	if duty == nil {
		return nil
	}
	return &contradictionScreen{duty: duty, meter: meter}
}

// Screen runs the advisory contradiction confirm on two entries (R16).
// ONE-STAGE per OQ4(a): the workhorse CONFIRM — the contradiction-screen
// alias's default seat (a DeBERTa-class NLI cross-encoder) is Servable:false at
// the pin (no GGUF serving path), so the one-stage screen confirms directly on
// the workhorse seat under the contradiction-screen duty label (honest
// degradation, §19; the DeBERTa pre-screen re-enters as stage 1 when a serving
// path exists, S12.3). High-precision: contradicts=true ONLY on a confident
// "yes"; the rationale becomes the refined question text — the gate never
// silently resolves (S09.7). An error/abstain degrades to the deterministic hit
// alone (the §14 absent-seam precedent).
func (c *contradictionScreen) Screen(ctx context.Context, a, b memory.Entry) (bool, string, error) {
	// F6a: resolve the contradiction-screen ALIAS first (platform callers address
	// aliases, S12.4; swap-invisible, R15) — use its seat when servable (a future
	// pre-screen/servable pick, or an operator retarget). Degrade to the workhorse
	// one-stage confirm ONLY when the resolved seat is non-servable (the DeBERTa
	// default at the pin).
	seat, err := c.duty.ResolveSeat(local.AliasContradiction)
	if err != nil || !seat.Servable {
		ws, ok := local.SeatByKey(local.WorkhorseSeatKey)
		if !ok || !ws.Servable {
			return false, "", nil // no servable confirm seat ⇒ deterministic hit stands
		}
		seat = ws
	}
	runID, settle := stage.BeginAdvisory(ctx, c.meter, "contradiction-screen")
	defer settle()
	schema := local.ContradictionSchema()
	res, err := c.duty.CallSeat(ctx, runID, local.AliasContradiction, seat, local.DutyRequest{
		Alias:          local.AliasContradiction,
		System:         "You screen two stored lessons for a direct contradiction. HIGH PRECISION: answer contradicts=yes ONLY for a genuine, direct conflict; otherwise 'no'. Reason briefly first, then the label. This is advisory — a human confirms; you never resolve anything.",
		User:           contradictionPrompt(a, b, schema),
		Schema:         schema,
		Name:           "contradiction-screen",
		MaxTokens:      300,
		Classification: true,
	})
	if err != nil {
		return false, "", err
	}
	var out struct {
		Contradicts string `json:"contradicts"`
		Reason      string `json:"reason"`
		Abstain     bool   `json:"abstain"`
	}
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		return false, "", fmt.Errorf("shell: decode contradiction-screen output: %w", err)
	}
	if out.Abstain || out.Contradicts != "yes" {
		return false, "", nil // only a confident yes is a hit (high precision)
	}
	rationale := out.Reason
	if rationale == "" {
		rationale = "the advisory screen judged these lessons to contradict"
	}
	return true, rationale, nil
}

func contradictionPrompt(a, b memory.Entry, schema json.RawMessage) string {
	return fmt.Sprintf("STATEMENT A (%s): %s\n%s\n\nSTATEMENT B (%s): %s\n%s\n\nDo these directly contradict? Respond with JSON matching this schema:\n%s\n",
		a.Kind, entryText(a), stage.TruncateText(a.Content, 1200),
		b.Kind, entryText(b), stage.TruncateText(b.Content, 1200), schema)
}

func entryText(e memory.Entry) string {
	if e.Title != "" {
		return e.Title
	}
	return e.ID
}
