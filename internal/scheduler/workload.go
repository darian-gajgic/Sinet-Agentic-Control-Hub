package scheduler

import "time"

// WorkloadClass is a run's scheduling priority band (Spec S10.7 vocabulary,
// coined in S10). The ratified priority ladder is:
//
//	interactive > human-blocked resumes > scheduled-due > background > probes
//
// with aging so background never starves, and interactive never starved by
// automation (3.3). The ladder itself is ratified FIXED policy (S10.7 text),
// not a ⚙ setting — S18 declares no scheduler-priority key.
type WorkloadClass string

const (
	// ClassInteractive is a person's live interactive use — CRITICAL_PLUS,
	// never starved by automation (3.3).
	ClassInteractive WorkloadClass = "interactive"
	// ClassHumanBlocked is an answered gate/ask resume or a resume-due run —
	// a person is waiting on it (Spec S10.7).
	ClassHumanBlocked WorkloadClass = "human-blocked"
	// ClassScheduled is a scheduled-due run (the v1 schedule surface; at v0
	// the only schedules are platform-internal timers, Spec S10.7/S19).
	ClassScheduled WorkloadClass = "scheduled"
	// ClassBackground is ordinary automation (Spec S10.7).
	ClassBackground WorkloadClass = "background"
	// ClassProbe is a zero-cost provider-window probe resume (Spec S10.5/S10.7:
	// probe resumes never count as attempts or spend).
	ClassProbe WorkloadClass = "probe"
)

// DefaultWorkloadClass is what an unclassified enqueue lands as. Background is
// the conservative default: it sheds first under pressure and never competes
// with a person's interactive use (Spec S10.7).
const DefaultWorkloadClass = ClassBackground

// validClass reports whether c is a ratified class.
func validClass(c WorkloadClass) bool {
	switch c {
	case ClassInteractive, ClassHumanBlocked, ClassScheduled, ClassBackground, ClassProbe:
		return true
	}
	return false
}

// classBias is the per-class head-start of the aging score, expressed in
// seconds so it composes with wait time (below). These are FIXED scheduler
// policy derived from the S10.7 ladder, NOT ⚙ (S18 ratifies no scheduler-aging
// key — the sseBatchSize/auth-constant precedent, CONVENTIONS §7/§9). They are
// flagged to the B1 gate; making aging operator-tunable would be an S00.9
// amendment adding S18 rows.
//
// The gaps are large enough that a fresher higher class normally wins, yet
// FINITE so a long-waiting lower class eventually overtakes — which is exactly
// "aging so background never starves" without starving anyone above it (S10.7).
// Interactive is handled separately (it wins outright, never via the score),
// so its bias is only a tiebreak ceiling.
var classBias = map[WorkloadClass]float64{
	ClassInteractive:  4 * agingGap, // never actually contends via score (see priorityLess)
	ClassHumanBlocked: 3 * agingGap,
	ClassScheduled:    2 * agingGap,
	ClassBackground:   1 * agingGap,
	ClassProbe:        0,
}

// agingGap is the wait (seconds) after which a class's aged score reaches the
// next class up. One hour: a background item waiting an hour competes with
// fresh scheduled work; two hours, with fresh human-blocked work. Fixed policy
// (see classBias), not ⚙.
const agingGap = 3600.0

// candidate is one queued run considered for admission in a claim pass.
type candidate struct {
	queueID    int64
	runID      string
	userID     string
	lane       string
	class      WorkloadClass
	enqueuedTS time.Time
}

// score is the aging score: class head-start plus accumulated wait. Higher
// wins. Interactive is not ranked by score (priorityLess handles it), so its
// score is used only among interactive peers (FIFO by wait).
func (c candidate) score(now time.Time) float64 {
	wait := now.Sub(c.enqueuedTS).Seconds()
	if wait < 0 {
		wait = 0
	}
	return classBias[c.class] + wait
}

// priorityLess reports whether a should be admitted before b (a "less" in the
// max-first sense: returns true when a outranks b). Interactive strictly
// outranks every automation class regardless of age (3.3: never starved by
// automation); among interactive peers, and among all automation classes, the
// aging score orders them (oldest-effective first), so background never
// starves (S10.7).
func priorityLess(a, b candidate, now time.Time) bool {
	ai, bi := a.class == ClassInteractive, b.class == ClassInteractive
	if ai != bi {
		return ai // interactive first
	}
	sa, sb := a.score(now), b.score(now)
	if sa != sb {
		return sa > sb
	}
	// Stable tiebreak: older enqueue, then lower queue id.
	if !a.enqueuedTS.Equal(b.enqueuedTS) {
		return a.enqueuedTS.Before(b.enqueuedTS)
	}
	return a.queueID < b.queueID
}
