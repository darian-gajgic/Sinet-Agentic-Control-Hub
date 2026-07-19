package sandbox

import (
	"errors"
	"fmt"
)

// admit.go holds the two compile-time admission checks S11.6 owns as
// mechanics: the Rule-of-Two check on a worker's declared record, and the
// P-T05-1 helper-tightening check. Both are enforced OUTSIDE agent code and
// prompt reach (S11.6): a worker can never alter its own confinement.

// Admission errors.
var (
	// ErrRuleOfTwo: a record asserts all three Rule-of-Two properties with no
	// supervision gate (S11.6 Agents Rule of Two).
	ErrRuleOfTwo = errors.New("sandbox: Rule-of-Two — all three properties held without a supervision gate")
	// ErrHelperLooser: a helper spawn requests a class looser than its
	// coordinator on some capability axis (P-T05-1: inherit and tighten only).
	ErrHelperLooser = errors.New("sandbox: helper confinement is looser than the coordinator (P-T05-1)")
)

// Props are the three Meta Agents Rule-of-Two properties (Spec S11.6): within
// a session an agent may hold at most two of {processes untrusted input,
// accesses sensitive data/systems, can change state or communicate
// externally}.
type Props struct {
	UntrustedInput  bool `json:"untrusted_input,omitempty"`
	SensitiveAccess bool `json:"sensitive_access,omitempty"`
	StateOrExternal bool `json:"state_or_external,omitempty"`
}

func (p Props) count() int {
	n := 0
	for _, b := range []bool{p.UntrustedInput, p.SensitiveAccess, p.StateOrExternal} {
		if b {
			n++
		}
	}
	return n
}

// AdmitRuleOfTwo statically refuses a worker whose declared record asserts
// all three properties without a supervision gate (human-in-the-loop, i.e.
// the 4.2 proposal path, or another reliable validation) — S11.6. A run may
// still safely transition between two-property phases when the transition
// breaks the attack chain; that is the caller's phase policy (S06.6), not
// this static gate.
func AdmitRuleOfTwo(p Props, supervised bool) error {
	if p.count() == 3 && !supervised {
		return ErrRuleOfTwo
	}
	return nil
}

// capVector is a confinement's capability on each independent axis. "Looser"
// (P-T05-1) is greater capability on ANY axis; "tighter-or-equal" is per-axis
// domination. Reading, section-cited (S11.6/P-T05-1): the spec calls this "a
// compile-time comparison" over "the isolation ladder C0–C4", but the classes
// are not totally ordered by capability (C0 carries a scoped credential +
// single-host egress that C1 lacks). Per-axis domination is the faithful,
// safe realization of "may only be tightened, never loosened": a helper may
// not exceed its coordinator on the ladder, on egress reach, on credential
// access, or on workspace-write.
type capVector struct {
	ladder  int // S5 ladder rank (C0=0 … C4=4)
	reach   int // egress reach: none=0, single-host=1, registries=2, fetch-broker=3
	creds   int // credential access: 0 none, 1 present (C0 inherent, or any auth-profile)
	wsWrite int // workspace-write: 0 ro/none, 1 rw
}

func capOf(c Confinement) (capVector, error) {
	lr, ok := c.Class.ladderRank()
	if !ok {
		return capVector{}, fmt.Errorf("%w: %q", ErrUnknownClass, c.Class)
	}
	v := capVector{ladder: lr}
	switch c.Network {
	case NetNone:
		v.reach = 0
	case NetSingleHost:
		v.reach = 1
	case NetRegistries:
		v.reach = 2
	case NetFetchBroker:
		v.reach = 3
	default:
		v.reach = 0
	}
	if c.Class == C0 || len(c.AuthProfiles) > 0 {
		v.creds = 1
	}
	if c.WorkspaceMode == "rw" {
		v.wsWrite = 1
	}
	return v, nil
}

// AdmitHelperClass enforces P-T05-1: a spawned helper inherits the
// coordinator's class and may only be tightened. It rejects any helper that
// is looser than the coordinator on any capability axis. S11 owns these
// mechanics; which class attaches to which plan/stage is policy (S06.6).
func AdmitHelperClass(coordinator, helper Confinement) error {
	cv, err := capOf(coordinator)
	if err != nil {
		return err
	}
	hv, err := capOf(helper)
	if err != nil {
		return err
	}
	switch {
	case hv.ladder > cv.ladder:
		return fmt.Errorf("%w: class %s (rank %d) > coordinator %s (rank %d)", ErrHelperLooser, helper.Class, hv.ladder, coordinator.Class, cv.ladder)
	case hv.reach > cv.reach:
		return fmt.Errorf("%w: egress reach %d > coordinator %d", ErrHelperLooser, hv.reach, cv.reach)
	case hv.creds > cv.creds:
		return fmt.Errorf("%w: helper gains credential access the coordinator lacks", ErrHelperLooser)
	case hv.wsWrite > cv.wsWrite:
		return fmt.Errorf("%w: helper gains workspace-write the coordinator lacks", ErrHelperLooser)
	}
	return nil
}
