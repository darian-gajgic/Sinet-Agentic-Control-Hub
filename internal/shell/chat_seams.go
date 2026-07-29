package shell

import (
	"context"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/chat"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
)

// chat_seams.go — the composition bridge for the S15.7 assistant (P3-B6-7).
//
// chatAdvisory adapts the §31 OQ4 AdvisoryMeter to internal/chat's seam, the
// same way historyAdvisory does for the query layers.
//
// The S12.1 R18 rule is that every local duty call writes ONE $0 D7 checkpoint
// on a consuming run, and naming a chat session has no run of its own — so the
// auto-titling duty rides a short-lived platform-scope advisory run, exactly as
// the history intent legs, the S09.7 contradiction screen and the S14.9
// narrator do. internal/chat cannot import internal/stage (the import wall), so
// the shell supplies stage.BeginAdvisory here.
//
// A failure to open one yields an empty run id, which the duty layer refuses;
// internal/chat turns that refusal into the MECHANICAL title rather than into an
// error, because an unavailable labeller must never make a conversation
// unstartable.
func chatAdvisory(meter stage.AdvisoryMeter) chat.AdvisoryRun {
	return func(ctx context.Context, label string) (string, func()) {
		return stage.BeginAdvisory(ctx, meter, label)
	}
}
