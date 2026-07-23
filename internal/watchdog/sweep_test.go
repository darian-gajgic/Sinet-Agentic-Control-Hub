package watchdog

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
)

// sweep_test.go — the time-based signals + registered checks (brief R5/R6,
// R21–R27, rubric 2/3/14–19).

// TestSilenceTripsAtBudget: a running run silent past its run-type budget trips
// and parks (nit f — the exclusion cases are the separate tests below; this test
// only proves the trip, so its name no longer over-promises an exclusion).
func TestSilenceTripsAtBudget(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()

	// A 120s budget for the "anthropic" run-type (≥ the 60s floor); the run's
	// last activity is 200s old ⇒ silent.
	setMap(t, e.reg, keySilenceBudget, map[string]string{"anthropic": "120"})

	e.runningRun("silent-run", "alice") // runType = lane "anthropic"
	e.staleActivity("silent-run", 200)
	if err := w.sweepSilence(ctx); err != nil {
		t.Fatalf("sweepSilence: %v", err)
	}
	fs := e.flagsFor("silent-run")
	if len(fs) != 1 || fs[0].Rule != RuleSilence {
		t.Fatalf("silence did not trip at budget 0: %+v", fs)
	}
	if fs[0].Severity != SeverityFlagNow {
		t.Errorf("silence severity = %q, want flag-now", fs[0].Severity)
	}
	if e.state("silent-run") != run.StateParked {
		t.Errorf("silence did not park the running run")
	}
}

// TestSilenceLimitParkResumeTime (drain D2, both directions): a limit-parked run
// with a FUTURE resets_at is legitimately waiting (NOT silence-flagged); one with
// a PAST resets_at should have resumed and is flagged. The resume-time is scoped
// to the limit event — the park transition (run.state_changed) lands AFTER the
// limit.event and must not mask it.
func TestSilenceLimitParkResumeTime(t *testing.T) {
	ctx := context.Background()
	budget120 := func(e *env) { setMap(t, e.reg, keySilenceBudget, map[string]string{"anthropic": "120"}) }

	// future resets_at → NOT flagged.
	e := newEnv(t)
	budget120(e)
	w := e.wd()
	e.runningRun("future-park", "alice")
	e.limitEvent("future-park", time.Now().Add(time.Hour)) // resets in the future
	if _, err := e.runs.Transition(ctx, "future-park", run.StateParked, run.TransitionOptions{Reason: "limit", Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("park: %v", err)
	}
	e.staleActivity("future-park", 200) // old enough to trip if it weren't waiting
	if err := w.sweepSilence(ctx); err != nil {
		t.Fatalf("sweepSilence: %v", err)
	}
	if fs := e.flagsFor("future-park"); len(fs) != 0 {
		t.Fatalf("a limit-park with a FUTURE resets_at was flagged silent (false positive, F2): %+v", fs)
	}

	// past resets_at → flagged (the scheduler should have resumed it).
	e2 := newEnv(t)
	budget120(e2)
	w2 := e2.wd()
	e2.runningRun("past-park", "bob")
	e2.limitEvent("past-park", time.Now().Add(-time.Hour)) // resets in the past
	if _, err := e2.runs.Transition(ctx, "past-park", run.StateParked, run.TransitionOptions{Reason: "limit", Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("park: %v", err)
	}
	e2.staleActivity("past-park", 200)
	if err := w2.sweepSilence(ctx); err != nil {
		t.Fatalf("sweepSilence: %v", err)
	}
	fs := e2.flagsFor("past-park")
	if len(fs) != 1 || fs[0].Rule != RuleSilence {
		t.Fatalf("a limit-park with a PAST resets_at was NOT silence-flagged: %+v", fs)
	}
}

// TestSilenceExcludesWatchdogParkAndBacklog (drain D3): a park with NO
// resume-time (watchdog park / operator pause / gate / maintenance) is
// waiting-on-human — never silence. A queued backlog run is scheduler-wait —
// never silence.
func TestSilenceExcludesWatchdogParkAndBacklog(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	setMap(t, e.reg, keySilenceBudget, map[string]string{"anthropic": "120"})

	// A watchdog-parked run (parked with a watchdog reason, no resume-time).
	e.runningRun("wd-parked", "alice")
	if _, err := e.runs.Transition(ctx, "wd-parked", run.StateParked, run.TransitionOptions{Reason: "watchdog:watchdog.loop", Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("park: %v", err)
	}
	e.staleActivity("wd-parked", 200)

	// A queued backlog run (never ran) with stale activity.
	if _, err := e.runs.Create(ctx, run.NewRun{ID: "queued-run", UserID: "alice", Lane: "anthropic", Substrate: "claude-cli"}); err != nil {
		t.Fatalf("create queued: %v", err)
	}
	if _, err := e.runs.Transition(ctx, "queued-run", run.StateQueued, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("queue: %v", err)
	}
	e.staleActivity("queued-run", 200)

	if err := w.sweepSilence(ctx); err != nil {
		t.Fatalf("sweepSilence: %v", err)
	}
	for _, id := range []string{"wd-parked", "queued-run"} {
		for _, f := range e.flagsFor(id) {
			if f.Rule == RuleSilence {
				t.Errorf("%s was silence-flagged — a resume-less park / queued backlog is never silence (D3): %+v", id, f)
			}
		}
	}
}

func TestSilenceNoTripUnderSeed(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	// No map entry ⇒ the recovery.dead_after seed (5 min); a just-created run is
	// not silent.
	e.runningRun("fresh-run", "bob")
	if err := w.sweepSilence(ctx); err != nil {
		t.Fatalf("sweepSilence: %v", err)
	}
	if fs := e.flagsFor("fresh-run"); len(fs) != 0 {
		t.Fatalf("a fresh run was flagged silent under the 5-min seed: %+v", fs)
	}
}

func TestSilenceExcludesParkedWithOpenAsk(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	setMap(t, e.reg, keySilenceBudget, map[string]string{"anthropic": "120"})

	e.runningRun("waiting-run", "carol")
	// park the run and open an ask (waiting-on-human = legitimately waiting)
	if _, err := e.runs.Transition(ctx, "waiting-run", run.StateParked, run.TransitionOptions{Reason: "ask", Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("park: %v", err)
	}
	e.openAsk("waiting-run", "carol")
	e.staleActivity("waiting-run", 200) // last event old enough to trip if it weren't waiting
	if err := w.sweepSilence(ctx); err != nil {
		t.Fatalf("sweepSilence: %v", err)
	}
	if fs := e.flagsFor("waiting-run"); len(fs) != 0 {
		t.Fatalf("a run waiting on an open ask was flagged silent (false positive): %+v", fs)
	}
}

// fakeSpend is a controlled SpendReader isolating the watchdog's spend LOGIC
// from the metering money-math (which spend_test.go tests directly). SpendOwners
// returns the window's owner (the day's spender), mirroring the real derive-from
// -usage-rows set (drain D11) without touching real ledger rows.
type fakeSpend struct {
	win metering.SpendWindow
	err error
}

func (f fakeSpend) SpendWindow(context.Context, string, time.Time, int) (metering.SpendWindow, error) {
	return f.win, f.err
}

func (f fakeSpend) SpendOwners(context.Context, time.Time) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.win.UserID == "" {
		return nil, nil
	}
	return []string{f.win.UserID}, nil
}

func TestSpendDormantWhenUnpriced(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.runningRun("job", "alice")
	// AnyUnpriced=true (the v0 empty-price-table posture) ⇒ dormant, no flag even
	// with a today ≫ median.
	w := e.wd(func(d *Deps) {
		d.Spend = fakeSpend{win: metering.SpendWindow{
			UserID: "alice", TodayUSD: 100, MedianUSD: 1, DaysOfHistory: 30, AnyUnpriced: true,
		}}
	})
	if err := w.sweepSpend(ctx); err != nil {
		t.Fatalf("sweepSpend: %v", err)
	}
	if fs := e.flagsFor(""); len(fs) != 0 {
		t.Fatalf("spend flagged while dormant (unpriced): %+v", fs)
	}
}

func TestSpendArmedTripsPerPerson(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.runningRun("job", "alice") // alice is an active owner
	w := e.wd(func(d *Deps) {
		// armed (priced history, ≥ arm days) and today 10× > 3× the median.
		d.Spend = fakeSpend{win: metering.SpendWindow{
			UserID: "alice", TodayUSD: 10, MedianUSD: 1, DaysOfHistory: 30, AnyUnpriced: false,
		}}
	})
	if err := w.sweepSpend(ctx); err != nil {
		t.Fatalf("sweepSpend: %v", err)
	}
	fs := e.flagsFor("")
	if len(fs) != 1 || fs[0].Rule != RuleSpend {
		t.Fatalf("armed spend spike did not flag: %+v", fs)
	}
	if fs[0].SpendVsBaseline == nil || !fs[0].SpendVsBaseline.Armed {
		t.Errorf("spend flag missing armed spend-vs-baseline evidence: %+v", fs[0])
	}
	// per-person: the flag is owner-attributed (user_id=alice), run-less.
	var owner, runID string
	if err := e.db.QueryRowContext(ctx,
		`SELECT user_id, COALESCE(run_id,'') FROM run_events WHERE type='watchdog.flagged' LIMIT 1`).Scan(&owner, &runID); err != nil {
		t.Fatalf("read spend flag scope: %v", err)
	}
	if owner != "alice" || runID != "" {
		t.Errorf("spend flag scope = (%q,%q), want owner-attributed run-less (alice, \"\")", owner, runID)
	}
}

func TestSpendBelowMultipleNoTrip(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.runningRun("job", "alice")
	w := e.wd(func(d *Deps) {
		d.Spend = fakeSpend{win: metering.SpendWindow{
			UserID: "alice", TodayUSD: 2, MedianUSD: 1, DaysOfHistory: 30, AnyUnpriced: false,
		}} // 2 < 3× median 1
	})
	if err := w.sweepSpend(ctx); err != nil {
		t.Fatalf("sweepSpend: %v", err)
	}
	if fs := e.flagsFor(""); len(fs) != 0 {
		t.Fatalf("spend flagged within the baseline (2 < 3×1): %+v", fs)
	}
}

func TestListenerAuditFlagsForeign(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd(func(d *Deps) {
		d.Listener = func(context.Context) ([]ForeignListener, error) {
			return []ForeignListener{{Addr: "0.0.0.0:9999", Process: "sinet-control"}}, nil
		}
	})
	if err := w.checkListeners(ctx); err != nil {
		t.Fatalf("checkListeners: %v", err)
	}
	fs := e.flagsFor("")
	if len(fs) != 1 || fs[0].Rule != RuleListener || fs[0].Severity != SeverityFlagNow {
		t.Fatalf("foreign listener not flag-now(High): %+v", fs)
	}
}

func TestListenerAuditCleanNoFlag(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd(func(d *Deps) {
		d.Listener = func(context.Context) ([]ForeignListener, error) { return nil, nil }
	})
	if err := w.checkListeners(ctx); err != nil {
		t.Fatalf("checkListeners: %v", err)
	}
	if fs := e.flagsFor(""); len(fs) != 0 {
		t.Fatalf("clean listener audit flagged: %+v", fs)
	}
}

func TestOrganAbsenceDigestFlags(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd(func(d *Deps) {
		d.Organs = func(context.Context) ([]OrganStatus, error) {
			return []OrganStatus{{Organ: "watchlist", Up: false, Note: "unit inactive"}, {Organ: "portpool", Up: true}}, nil
		}
	})
	if err := w.checkOrgans(ctx); err != nil {
		t.Fatalf("checkOrgans: %v", err)
	}
	fs := e.flagsFor("")
	if len(fs) != 1 || fs[0].Rule != RuleOrganAbsence || fs[0].Severity != SeverityDailyDigest {
		t.Fatalf("organ-absence not a single daily-digest flag for the down organ: %+v", fs)
	}
}

func TestResumeReconcileFlagsGap(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd(func(d *Deps) {
		d.Wake = func(context.Context) (WakeStatus, error) {
			return WakeStatus{Pending: true, Completed: false, Missing: []string{"listener re-audit"}}, nil
		}
	})
	if err := w.checkResumeReconcile(ctx); err != nil {
		t.Fatalf("checkResumeReconcile: %v", err)
	}
	if fs := e.flagsFor(""); len(fs) != 1 || fs[0].Rule != RuleResumeReconcile {
		t.Fatalf("incomplete wake reconcile did not flag: %+v", fs)
	}
}

func TestResumeReconcileCleanNoFlag(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd(func(d *Deps) {
		d.Wake = func(context.Context) (WakeStatus, error) {
			return WakeStatus{Pending: true, Completed: true}, nil
		}
	})
	if err := w.checkResumeReconcile(ctx); err != nil {
		t.Fatalf("checkResumeReconcile: %v", err)
	}
	if fs := e.flagsFor(""); len(fs) != 0 {
		t.Fatalf("a completed wake reconcile flagged: %+v", fs)
	}
}

func TestEventLogSizeFlagsPastThreshold(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd(func(d *Deps) {
		d.WAL = func(context.Context) (WALStat, error) {
			return WALStat{WALBytes: eventLogSizeThresholdBytes + 1, EventRows: 10}, nil
		}
	})
	if err := w.checkEventLogSize(ctx); err != nil {
		t.Fatalf("checkEventLogSize: %v", err)
	}
	if fs := e.flagsFor(""); len(fs) != 1 || fs[0].Rule != RuleEventLogSize || fs[0].Severity != SeverityDailyDigest {
		t.Fatalf("event-log-size did not raise a daily-digest flag: %+v", fs)
	}
}

func TestEventLogSizeFallbackSmallDBNoFlag(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd() // no WAL seam → the run_events payload-bytes fallback
	e.runningRun("job", "alice")
	if err := w.checkEventLogSize(ctx); err != nil {
		t.Fatalf("checkEventLogSize: %v", err)
	}
	if fs := e.flagsFor(""); len(fs) != 0 {
		t.Fatalf("a tiny DB tripped the event-log-size threshold: %+v", fs)
	}
}

func TestFlagNowRateMetaAlert(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	target, _ := e.reg.Int(keyFlagNowTarget) // default 2/day

	// Produce target+1 flag-now flags (each a loop on a distinct run).
	for i := int64(0); i <= target; i++ {
		id := "loop" + string(rune('a'+i)) + ".execute"
		e.runningRun(id, "alice")
		for j := 0; j < 5; j++ {
			e.toolEvent(id, "Bash", "d1", false)
		}
		if err := w.EvaluateRun(ctx, id); err != nil {
			t.Fatalf("EvaluateRun: %v", err)
		}
	}
	if err := w.checkFlagNowRate(ctx); err != nil {
		t.Fatalf("checkFlagNowRate: %v", err)
	}
	fs := e.flagsFor("")
	var meta *FlagPayload
	for i := range fs {
		if fs[i].Rule == RuleFlagNowRate {
			meta = &fs[i]
		}
	}
	if meta == nil {
		t.Fatalf("sustained flag-now breach did not raise a meta-alert: %+v", fs)
	}
	if meta.Severity != SeverityDailyDigest {
		t.Errorf("the meta-alert must be daily-digest, never a flag-now itself (R23); got %q", meta.Severity)
	}
}

// helpers

func (e *env) openAsk(runID, owner string) {
	e.t.Helper()
	if err := e.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts)
			 VALUES (?, ?, ?, '{}', 'gate', ?)`,
			"ask-"+runID, runID, owner, "2026-07-23T00:00:00Z")
		return err
	}); err != nil {
		e.t.Fatalf("open ask: %v", err)
	}
}

func setMap(t *testing.T, reg *settings.Registry, key string, m map[string]string) {
	t.Helper()
	raw, _ := json.Marshal(m)
	if err := reg.Set(context.Background(), settings.SetRequest{
		Key: key, Value: raw, Actor: settings.Actor{Kind: settings.ActorOperator, ID: "op"}, Reason: "test",
	}); err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
}
