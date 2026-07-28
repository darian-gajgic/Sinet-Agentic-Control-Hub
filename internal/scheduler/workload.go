package scheduler

import (
	"sort"
	"time"
)

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
	substrate  string // the run's substrate — the model key for the S10.9 GPU hook (dormant)
	class      WorkloadClass
	enqueuedTS time.Time
	// hintRank is the S15.5 board-drag priority hint carried on the queue row
	// (migration 0017). Lower sorts earlier; 0 is "no hint", which is where
	// every row starts and where every row stayed before B6-2B.
	hintRank int64
	// key is the candidate's ADMISSION KEY after the S15.5 hint permutation has
	// run (permuteHints). `keyed` says whether it has been assigned; an
	// unpermuted candidate is ordered by its own natural key, which is what the
	// permutation degenerates to when nobody has dragged anything.
	key   orderKey
	keyed bool
}

// orderKey is one candidate's place in the admission order: the S10.7 aging
// score, then the stable tiebreaks. It is a TOTAL order — that is the whole
// point of expressing it as a value rather than as comparator branches, because
// the ordering the claim loop sorts by has to be transitive.
type orderKey struct {
	score      float64
	enqueuedTS time.Time
	queueID    int64
}

// before is the total order on keys: higher score first, then older enqueue,
// then lower queue id. Every limb compares a fixed per-candidate value, so the
// relation is irreflexive, asymmetric and transitive.
func (k orderKey) before(o orderKey) bool {
	if k.score != o.score {
		return k.score > o.score
	}
	if !k.enqueuedTS.Equal(o.enqueuedTS) {
		return k.enqueuedTS.Before(o.enqueuedTS)
	}
	return k.queueID < o.queueID
}

// naturalKey is the candidate's OWN key — where the S10.7 ladder and aging put
// it, before any hint moves it.
func (c candidate) naturalKey(now time.Time) orderKey {
	return orderKey{score: c.score(now), enqueuedTS: c.enqueuedTS, queueID: c.queueID}
}

// orderKeyAt returns the key the candidate is ordered by: the permuted one when
// the hint pass has run, its own otherwise.
func (c candidate) orderKeyAt(now time.Time) orderKey {
	if c.keyed {
		return c.key
	}
	return c.naturalKey(now)
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
// It is a TOTAL ORDER on (is-interactive, orderKey), which is what a sort needs
// and what a comparator with a conditional hint limb could not give. The S15.5
// drag hint does NOT appear here at all: it is applied beforehand, by permuting
// keys among a person's own same-class candidates (permuteHints), so the thing
// being sorted is a fixed value per candidate.
func priorityLess(a, b candidate, now time.Time) bool {
	ai, bi := a.class == ClassInteractive, b.class == ClassInteractive
	if ai != bi {
		return ai // interactive first
	}
	return a.orderKeyAt(now).before(b.orderKeyAt(now))
}

// permuteHints applies the S15.5 board-drag hint as a SCORE-MULTISET-PRESERVING
// PERMUTATION, which is the design the whole safety argument rests on (drain
// D1).
//
// ── Why not a comparator limb ───────────────────────────────────────────────
//
// The first shape of this feature compared hint ranks inside priorityLess when
// (and only when) two candidates shared an owner and a class. That is NOT a
// strict weak ordering: with A and B in one group whose hint order disagrees
// with their aging order, and an outsider C scoring between them, the pairwise
// relations go B≺A (hint), A≺C (score), C≺B (score) — a cycle. sort.SliceStable
// on a cyclic relation is not merely unstable, it is UNDEFINED: the evaluator
// drove three different claim winners out of three initial row orders, one of
// them the OTHER OWNER's candidate. A person dragging their own board must
// never be able to change who wins among other people's work, and a comparator
// that can do that is broken regardless of how carefully its limbs are guarded.
//
// ── What this does instead ──────────────────────────────────────────────────
//
// For each (user, class) group, the group's OWN admission keys are collected,
// sorted best-first, and handed back out to the group's members in HINT ORDER
// (best hint takes the group's best key; ties fall back to the member's own
// natural key). Then the claim pass does ONE global sort by key alone.
//
// Three properties follow, and each is asserted by a property test:
//
//   - TRANSITIVE. The sort compares a fixed per-candidate value through a total
//     order, so no cycle can exist and the claim order is identical from any
//     initial row order.
//   - CROSS-USER INVARIANT AT THE SET LEVEL. A group's multiset of keys is
//     preserved exactly — the hint only decides WHICH of the person's own runs
//     holds which of the person's own keys. Every other owner therefore faces
//     the identical multiset of competitor keys, so their own positions cannot
//     move.
//   - CLASS BOUNDARIES UNTOUCHED. Keys never move between groups, and a group
//     is (user, class). The best a hinted background run can reach is a key one
//     of that person's background runs already held, so it can never rise above
//     work of a class it could not already have been beaten by — interactive
//     included.
//
// And the hint is honored exactly as far as the person's own standing allows,
// which is what "reordering one's OWN queued tasks" means (S15.5).
func permuteHints(cands []candidate, now time.Time) {
	type groupKey struct {
		user  string
		class WorkloadClass
	}
	groups := map[groupKey][]int{}
	for i := range cands {
		cands[i].key, cands[i].keyed = cands[i].naturalKey(now), true
		g := groupKey{cands[i].userID, cands[i].class}
		groups[g] = append(groups[g], i)
	}
	for _, members := range groups {
		if len(members) < 2 {
			continue // a group of one has nothing to reorder
		}
		// The group's own keys, best first. This multiset is what is preserved.
		keys := make([]orderKey, 0, len(members))
		for _, i := range members {
			keys = append(keys, cands[i].key)
		}
		sort.Slice(keys, func(a, b int) bool { return keys[a].before(keys[b]) })
		// The group's members, in the order the person asked for. With no hints
		// this is the natural-key order, so the permutation is the identity and
		// an un-dragged queue admits exactly as it did before B6-2B.
		order := append([]int(nil), members...)
		sort.SliceStable(order, func(a, b int) bool {
			x, y := cands[order[a]], cands[order[b]]
			if x.hintRank != y.hintRank {
				return x.hintRank < y.hintRank
			}
			return x.naturalKey(now).before(y.naturalKey(now))
		})
		for pos, i := range order {
			cands[i].key = keys[pos]
		}
	}
}
