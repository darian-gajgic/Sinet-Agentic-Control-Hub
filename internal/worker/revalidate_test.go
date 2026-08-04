package worker_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// The Spec S14.8 ¶3 / S08.10(a) green stamp: a flagged template version returns
// to active service ONLY through a dated revalidation record. Approval and
// guardrails stand, so there is no second approval — the dated record is what
// releases it.

func TestRevalidateReleasesFlaggedVersionOnlyWithDatedStamp(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	ctx := context.Background()
	tpl, v, _ := draftValidated(t, f, "alice")
	if _, err := f.store.Approve(ctx, "alice", v.ID, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	flagged, err := f.store.FlagByModel(ctx, "claude-haiku-4-5", "S14.8 revalidation trigger")
	if err != nil || len(flagged) != 1 {
		t.Fatalf("FlagByModel: %v %v", flagged, err)
	}

	stamp := worker.RevalidationStamp{
		TemplateID: tpl.ID, VersionID: v.ID,
		TriggerKind: "model_swap", TriggerSubject: "claude-haiku-4-5",
		Suites:   "golden-software@v1 + golden-software@v1",
		Baseline: "TPR 1.000 / TNR 0.500 measured 2026-07-22",
	}

	// A RED revalidation stamps the record and changes nothing: the version
	// stays flagged, so it may run supervised but never unsupervised.
	red := stamp
	red.Green = false
	if _, err := f.store.Revalidate(ctx, "alice", red); err != nil {
		t.Fatalf("Revalidate (red): %v", err)
	}
	got, err := f.store.Template(ctx, tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != worker.StatusFlagged {
		t.Fatalf("a red revalidation must leave the version flagged, got %q", got.Status)
	}
	pol, err := f.store.Delivery(ctx, tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !pol.RequiresReview {
		t.Error("a flagged worker runs supervised, never unsupervised (S08.10(a))")
	}

	// A GREEN revalidation releases it — and the release is a dated record with
	// who, when, which suites, and against which baseline.
	green := stamp
	green.Green = true
	rec, err := f.store.Revalidate(ctx, "alice", green)
	if err != nil {
		t.Fatalf("Revalidate (green): %v", err)
	}
	if rec.StampedTS == "" || rec.Actor != "alice" {
		t.Fatalf("the stamp must be dated and attributed: %+v", rec)
	}
	if got, err = f.store.Template(ctx, tpl.ID); err != nil || got.Status != worker.StatusActive {
		t.Fatalf("a green revalidation must return the version to active, got %q (%v)", got.Status, err)
	}
	latest, err := f.store.LatestRevalidation(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !latest.Green || latest.Suites != stamp.Suites || latest.Baseline != stamp.Baseline {
		t.Fatalf("the persisted stamp lost its content: %+v", latest)
	}

	// No new event type: the release rides the existing worker family, marked
	// as a revalidation on the payload.
	types := f.eventTypes()
	found := false
	for _, ty := range types {
		if ty == "worker.validated" {
			found = true
		}
		if strings.HasPrefix(ty, "worker.reval") || strings.Contains(ty, "revalidation") {
			t.Errorf("a new event type %q was minted — the release rides worker.validated", ty)
		}
	}
	if !found {
		t.Error("the revalidation appended no worker.validated event")
	}

	// A stamp that names no suites or no baseline is not a revalidation record.
	bare := stamp
	bare.Suites, bare.Green = "", true
	if _, err := f.store.Revalidate(ctx, "alice", bare); !errors.Is(err, worker.ErrInvalid) {
		t.Fatalf("a stamp without its suites: %v, want ErrInvalid", err)
	}
}

func TestRevalidationStampsAreAppendOnly(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	ctx := context.Background()
	tpl, v, _ := draftValidated(t, f, "alice")
	if _, err := f.store.Approve(ctx, "alice", v.ID, worker.ApproveOpts{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Revalidate(ctx, "alice", worker.RevalidationStamp{
		TemplateID: tpl.ID, VersionID: v.ID, TriggerKind: "engine_bump", TriggerSubject: "2.1.219",
		Suites: "golden-software@v1", Baseline: "TPR 1.000", Green: true,
	}); err != nil {
		t.Fatal(err)
	}
	// A raw-SQL holder cannot rewrite or delete history.
	for _, stmt := range []string{
		`UPDATE revalidation_stamps SET green = 0`,
		`DELETE FROM revalidation_stamps`,
	} {
		err := f.db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, stmt)
			return err
		})
		if err == nil {
			t.Errorf("%q succeeded — a revalidation stamp is append-only (Spec S14.8 ¶3)", stmt)
		}
	}
}
