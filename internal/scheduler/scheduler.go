// Package scheduler is the scheduler module seam of Spec S01.1: queue
// claiming, lane policy, metering-aware admission (Spec S10). Its owning
// build phase is B1; this B0 stub exists so the shell's lifecycle calls the
// admission seam from the first build (Spec S01.6 steps: "Resume admission
// (scheduler claiming)" at startup, "stop admission" at shutdown and on
// maintenance enter) without the shell ever reaching around the boundary.
//
// Everything beyond the admission switch — CAS claiming, lanes/slots,
// limit taxonomy, park policy — lands here at B1 per Spec S10.
package scheduler

import (
	"context"
	"log/slog"
)

// StubAdmission is the B0 admission seam: it satisfies the shell's Admission
// interface, records the switch position, and claims nothing — there is no
// queue to claim from until B1 (Spec S10). It is safe for concurrent use by
// way of doing nothing but logging.
type StubAdmission struct {
	Logger *slog.Logger // nil = silent
}

// ResumeAdmission opens admission (Spec S01.6 startup step 5, maintenance
// exit). B0: nothing claims, so this only logs the switch.
func (s *StubAdmission) ResumeAdmission(ctx context.Context) error {
	s.log(ctx, "admission resumed (B0 stub: no queue until Spec S10 lands at B1)")
	return nil
}

// StopAdmission closes admission (Spec S01.6 shutdown and maintenance
// enter: "the scheduler claims nothing new").
func (s *StubAdmission) StopAdmission(ctx context.Context) error {
	s.log(ctx, "admission stopped (B0 stub)")
	return nil
}

// ParkInFlightRuns parks still-running runs at maintenance-drain grace
// expiry: stop the run unit; the next resume starts from the last checkpoint
// — parked, flagged, resumable, never a kill of record (Spec S01.6). B0: no
// run machinery exists (run FSM is B0-4, run units are B1), so there is
// nothing to park.
func (s *StubAdmission) ParkInFlightRuns(ctx context.Context) error {
	s.log(ctx, "park in-flight runs (B0 stub: run machinery lands at B0-4/B1)")
	return nil
}

func (s *StubAdmission) log(ctx context.Context, msg string) {
	if s.Logger != nil {
		s.Logger.InfoContext(ctx, msg, "seam", "scheduler")
	}
}
