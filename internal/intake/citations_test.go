package intake_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
)

// TestCitedEntryDriftFires: a plan citing project-truth knowledge entries
// records those entry versions at approval (StoredFingerprint), and a later
// new-version / supersession / removal of a cited entry fires a stale flag
// with a cited-entry reason at CheckApprovalStale (Spec S09.6, R31/R32; rubric
// items 20/21). internal/intake never imports internal/memory — the resolver
// is a func seam wired by the shell.
func TestCitedEntryDriftFires(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()

	// The planner cites project-truth entry "know-1".
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Plan.CitedEntries = []string{"know-1"}
		return p, nil
	}
	// The cited-entry resolver (the shell wires this to the memory store).
	versions := map[string]string{"know-1": "1"}
	f.p.CitedEntryVersions = func(_ context.Context, keys []string) (map[string]string, error) {
		out := map[string]string{}
		for _, k := range keys {
			if v, ok := versions[k]; ok {
				out[k] = v
			}
		}
		return out, nil
	}

	st, _, _ := approvalCard(t, f)
	// R31: the cited entry version landed in the stored fingerprint.
	if st.StoredFingerprint == nil || st.StoredFingerprint.CitedEntryVersions["know-1"] != "1" {
		t.Fatalf("stored fingerprint missing the cited entry version: %+v", st.StoredFingerprint)
	}

	citedReason := func(reasons []string) bool {
		for _, r := range reasons {
			if strings.Contains(r, "cited knowledge entry") {
				return true
			}
		}
		return false
	}

	// new-version: the cited entry's version bumps → drift with a cited reason.
	versions["know-1"] = "2"
	got, err := f.p.CheckApprovalStale(ctx, st.TaskID, false)
	if err != nil {
		t.Fatalf("CheckApprovalStale (new-version): %v", err)
	}
	if !got.StaleFlag || !citedReason(got.StaleReasons) {
		t.Fatalf("new-version drift not flagged with a cited reason: %+v", got.StaleReasons)
	}

	// supersession / removal: the cited entry no longer resolves active → its
	// key vanishes → mapDrift's vanished-key = drift (retirement semantics).
	delete(versions, "know-1")
	got, err = f.p.CheckApprovalStale(ctx, st.TaskID, false)
	if err != nil {
		t.Fatalf("CheckApprovalStale (removed): %v", err)
	}
	if !got.StaleFlag || !citedReason(got.StaleReasons) {
		t.Fatalf("removal drift not flagged with a cited reason: %+v", got.StaleReasons)
	}
}

// TestCitedEntryFreshWhenUnchanged: a cited entry still at its captured
// version produces no cited-entry drift — the seam must not false-positive.
func TestCitedEntryFreshWhenUnchanged(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Plan.CitedEntries = []string{"know-1"}
		return p, nil
	}
	f.p.CitedEntryVersions = func(_ context.Context, keys []string) (map[string]string, error) {
		return map[string]string{"know-1": "1"}, nil
	}
	st, _, _ := approvalCard(t, f)
	got, err := f.p.CheckApprovalStale(ctx, st.TaskID, false)
	if err != nil {
		t.Fatalf("CheckApprovalStale: %v", err)
	}
	for _, r := range got.StaleReasons {
		if strings.Contains(r, "cited knowledge entry") {
			t.Fatalf("unchanged cited entry falsely flagged: %+v", got.StaleReasons)
		}
	}
}
