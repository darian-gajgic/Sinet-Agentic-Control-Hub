package run_test

import (
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// TestFireSiblingAccept: the S02.8 sibling-accept PRODUCER (Spec S13.9) feeds
// EvaluateFreshness with SiblingAccept=true for each active run, so every one
// routes to S02.6 re-validation even when its own fingerprint is unchanged.
func TestFireSiblingAccept(t *testing.T) {
	reg := settings.New()
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	runs := []run.SiblingAcceptRun{
		{RunID: "r1", CheckpointTime: now.Add(-time.Minute)},
		{RunID: "r2", CheckpointTime: now.Add(-time.Minute)},
	}
	routed, err := run.FireSiblingAccept(reg, runs, now)
	if err != nil {
		t.Fatalf("FireSiblingAccept: %v", err)
	}
	if len(routed) != 2 || routed[0] != "r1" || routed[1] != "r2" {
		t.Errorf("sibling-accept routed %v, want [r1 r2] (all active runs re-validate)", routed)
	}
	// No active runs → nothing routed, no error.
	if r, err := run.FireSiblingAccept(reg, nil, now); err != nil || len(r) != 0 {
		t.Errorf("empty FireSiblingAccept = %v, %v", r, err)
	}
}

func TestEvaluateFreshness(t *testing.T) {
	reg := settings.New() // ⚙ freshness.max_age default 24 h
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	base := run.FreshnessInput{
		CheckpointTime: now.Add(-time.Hour),
		Stored: run.Fingerprint{
			RepoHead:          "abc",
			SourceHashes:      map[string]string{"doc": "h1"},
			SpecPlanVersion:   "plan-3",
			PriceTableVersion: "prices-7",
		},
		StoredVersions: run.VersionFields{ModelID: "m1", InvocationFingerprint: "f1", ToolSchemaVersion: "t1", PromptSchemaVersion: "p1"},
	}
	base.Current = base.Stored
	base.CurrentVersions = base.StoredVersions

	cases := []struct {
		name       string
		mutate     func(*run.FreshnessInput)
		wantFresh  bool
		wantReason string
	}{
		{"identical and recent", func(in *run.FreshnessInput) {}, true, ""},
		{"age past max_age", func(in *run.FreshnessInput) {
			in.CheckpointTime = now.Add(-25 * time.Hour)
		}, false, "age"},
		{"price-table drift alone triggers", func(in *run.FreshnessInput) {
			in.Current.PriceTableVersion = "prices-8"
		}, false, "price-table"},
		{"repo HEAD moved", func(in *run.FreshnessInput) {
			in.Current.RepoHead = "def"
		}, false, "repo HEAD"},
		{"source vanished", func(in *run.FreshnessInput) {
			in.Current.SourceHashes = nil
		}, false, "no longer observable"},
		{"sibling accept", func(in *run.FreshnessInput) {
			in.SiblingAccept = true
		}, false, "was accepted since this was planned"},
		{"model id changed", func(in *run.FreshnessInput) {
			in.CurrentVersions.ModelID = "m2"
		}, false, "model id"},
		{"invocation fingerprint changed", func(in *run.FreshnessInput) {
			in.CurrentVersions.InvocationFingerprint = "f2"
		}, false, "invocation-config"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mutate(&in)
			got, err := run.EvaluateFreshness(reg, in, now)
			if err != nil {
				t.Fatalf("EvaluateFreshness: %v", err)
			}
			if got.Fresh != tc.wantFresh {
				t.Fatalf("Fresh = %v (reasons %v), want %v", got.Fresh, got.Reasons, tc.wantFresh)
			}
			if tc.wantReason != "" {
				found := false
				for _, r := range got.Reasons {
					if strings.Contains(r, tc.wantReason) {
						found = true
					}
				}
				if !found {
					t.Fatalf("reasons %v missing %q", got.Reasons, tc.wantReason)
				}
			}
		})
	}
}
