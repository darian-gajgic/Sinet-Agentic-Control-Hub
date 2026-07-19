package sandbox

import (
	"errors"
	"testing"
)

func TestAdmitRuleOfTwo(t *testing.T) {
	all := Props{UntrustedInput: true, SensitiveAccess: true, StateOrExternal: true}
	if err := AdmitRuleOfTwo(all, false); !errors.Is(err, ErrRuleOfTwo) {
		t.Errorf("all three properties unsupervised: want ErrRuleOfTwo, got %v", err)
	}
	// A supervision gate permits all three (S11.6).
	if err := AdmitRuleOfTwo(all, true); err != nil {
		t.Errorf("all three properties with supervision: want admit, got %v", err)
	}
	// Any two of three is always fine.
	for _, p := range []Props{
		{UntrustedInput: true, SensitiveAccess: true},
		{UntrustedInput: true, StateOrExternal: true},
		{SensitiveAccess: true, StateOrExternal: true},
		{},
	} {
		if err := AdmitRuleOfTwo(p, false); err != nil {
			t.Errorf("props %+v: want admit, got %v", p, err)
		}
	}
}

func TestAdmitHelperClass(t *testing.T) {
	c1, _ := Profile(C1)
	c2, _ := Profile(C2)
	c0, _ := Profile(C0)

	// Equal class admits.
	if err := AdmitHelperClass(c2, c2); err != nil {
		t.Errorf("C2 helper of C2 coordinator: want admit, got %v", err)
	}
	// Tightening (C2 coordinator → C1 helper) admits: drops rw + egress.
	if err := AdmitHelperClass(c2, c1); err != nil {
		t.Errorf("C1 helper of C2 coordinator (tighten): want admit, got %v", err)
	}
	// Loosening (C1 coordinator → C2 helper) is refused: gains rw + egress.
	if err := AdmitHelperClass(c1, c2); !errors.Is(err, ErrHelperLooser) {
		t.Errorf("C2 helper of C1 coordinator (loosen): want ErrHelperLooser, got %v", err)
	}
	// Cross-axis corner: a C1 coordinator (no egress, no creds) spawning a C0
	// helper (single-host egress + a scoped credential) is refused even though
	// C0's ladder rank is lower — the helper gains egress the coordinator lacks.
	if err := AdmitHelperClass(c1, c0); !errors.Is(err, ErrHelperLooser) {
		t.Errorf("C0 helper of C1 coordinator: want ErrHelperLooser (gains egress/creds), got %v", err)
	}
	// A helper that adds an auth-profile the coordinator lacks is refused.
	c1WithCred := c1
	c1WithCred.AuthProfiles = []string{"git-signing"}
	if err := AdmitHelperClass(c1, c1WithCred); !errors.Is(err, ErrHelperLooser) {
		t.Errorf("helper gaining a credential: want ErrHelperLooser, got %v", err)
	}
}

func TestClassLadderTotalOrder(t *testing.T) {
	for _, c := range []Class{C0, C1, C2, C3, C4} {
		if _, ok := c.ladderRank(); !ok {
			t.Errorf("class %s has no ladder rank", c)
		}
	}
	if _, ok := Class("C9").ladderRank(); ok {
		t.Error("unknown class returned a ladder rank")
	}
}
