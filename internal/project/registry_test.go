package project

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

func TestRegisterPendingThenActivate(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	src := f.fixtureRepo(map[string]string{"README.md": "hi"})

	e, _, err := f.store.Onboard(ctx, OnboardInput{ProjectID: "shop", Owner: "alice", Name: "shop", Source: src})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if e.State != StatePending {
		t.Fatalf("state = %q, want pending", e.State)
	}
	if e.Active() {
		t.Fatal("a freshly onboarded entry must not be active")
	}
	// A draft (capture v1) exists before approval.
	if e.CaptureVersion != 1 {
		t.Fatalf("capture version = %d, want 1", e.CaptureVersion)
	}

	act, err := f.store.Activate(ctx, "shop", "alice")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !act.Active() {
		t.Fatal("entry must be active after owner approval")
	}
}

func TestActivateNonOwnerRefused(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	src := f.fixtureRepo(map[string]string{"README.md": "hi"})
	if _, _, err := f.store.Onboard(ctx, OnboardInput{ProjectID: "shop", Owner: "alice", Name: "shop", Source: src}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	// D10: only the owner approves their own object.
	if _, err := f.store.Approve(ctx, "shop", "bob", nil); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("non-owner approve: err = %v, want ErrNotOwner", err)
	}
	if _, err := f.store.Activate(ctx, "shop", "bob"); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("non-owner activate: err = %v, want ErrNotOwner", err)
	}
	e, _ := f.store.Get(ctx, "shop")
	if e.Active() {
		t.Fatal("a refused approval must leave the entry pending")
	}
}

func TestCaptureVersionedNeverOverwritten(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{"go.mod": "module x\n"})
	v1 := e.CaptureVersion

	// Re-capture (edit) bumps the version and preserves history — a silent
	// in-place overwrite is inadmissible (Spec S13.7).
	c2, err := f.store.Capture(ctx, CaptureInput{
		ProjectID: "shop", By: "alice",
		Conventions: []string{"edited convention"},
		Commands:    Commands{Test: "go test ./..."},
	})
	if err != nil {
		t.Fatalf("re-capture: %v", err)
	}
	if c2.Version != v1+1 {
		t.Fatalf("re-capture version = %d, want %d", c2.Version, v1+1)
	}
	// The prior capture row is still readable (traceable history).
	prior, err := f.store.captureAt(ctx, "shop", v1)
	if err != nil {
		t.Fatalf("read prior capture: %v", err)
	}
	if prior.Version != v1 {
		t.Fatalf("prior capture version = %d, want %d", prior.Version, v1)
	}

	// The schema itself refuses in-place mutation and deletion of a capture.
	err = f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE repo_registry_captures SET conventions = '["poison"]' WHERE project_id = ? AND version = ?`, "shop", v1)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("capture in-place edit: err = %v, want immutability abort", err)
	}
	err = f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM repo_registry_captures WHERE project_id = ?`, "shop")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "never dropped") {
		t.Fatalf("capture delete: err = %v, want delete abort", err)
	}
}

func TestRegistryEntryImmutabilityWalls(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"README.md": "hi"})

	// No delete.
	err := f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM repo_registry WHERE project_id = ?`, "shop")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "never deleted") {
		t.Fatalf("entry delete: err = %v, want delete abort", err)
	}
	// Identity immutable.
	err = f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE repo_registry SET user_id = 'mallory' WHERE project_id = ?`, "shop")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "identity is immutable") {
		t.Fatalf("owner change: err = %v, want identity abort", err)
	}
	// Active never reverts to pending.
	err = f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE repo_registry SET state = 'pending' WHERE project_id = ?`, "shop")
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "never reverts") {
		t.Fatalf("state revert: err = %v, want one-way abort", err)
	}
}

func TestProtectedRefsDefaultIsDefaultBranch(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"README.md": "hi"})
	refs, err := f.store.ProtectedRefs(ctx, "shop")
	if err != nil {
		t.Fatalf("ProtectedRefs: %v", err)
	}
	if len(refs) != 1 || refs[0] != "main" {
		t.Fatalf("protected refs = %v, want [main] (default = default branch, Spec S13.6)", refs)
	}
	// A pending (non-active) entry does not feed the broker.
	if _, _, err := f.store.Onboard(ctx, OnboardInput{ProjectID: "draft", Owner: "alice", Name: "draft", Source: f.fixtureRepo(map[string]string{"a": "b"})}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if _, err := f.store.ProtectedRefs(ctx, "draft"); !errors.Is(err, ErrNotActive) {
		t.Fatalf("pending ProtectedRefs: err = %v, want ErrNotActive", err)
	}
}

func TestMatchForIntakeOnlyActiveByAlias(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"go.mod": "module shop\n"})

	// A pending project must never match (Spec S13.7: only active entries
	// feed intake resolution).
	if _, _, err := f.store.Onboard(ctx, OnboardInput{ProjectID: "warehouse", Owner: "alice", Name: "warehouse", Source: f.fixtureRepo(map[string]string{"a": "b"})}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}

	got, ok, err := f.store.MatchForIntake(ctx, MatchHint{UserID: "alice", Title: "fix the shop backend", Text: "the checkout is broken"})
	if err != nil {
		t.Fatalf("MatchForIntake: %v", err)
	}
	if !ok || got.ProjectID != "shop" {
		t.Fatalf("match = %+v ok=%v, want shop", got, ok)
	}
	if e := got.Capture.Commands.Test; e != "go test ./..." {
		t.Fatalf("matched entry commands.test = %q, want go test ./...", e)
	}

	// The pending "warehouse" is not matched even when named.
	if _, ok, _ := f.store.MatchForIntake(ctx, MatchHint{UserID: "alice", Title: "update the warehouse", Text: ""}); ok {
		t.Fatal("a pending project must not match intake")
	}
	// A user who neither owns nor is a member sees nothing.
	if _, ok, _ := f.store.MatchForIntake(ctx, MatchHint{UserID: "carol", Title: "fix the shop backend", Text: ""}); ok {
		t.Fatal("a non-member must not match a project they cannot see")
	}
}

func TestDriftDetectedNeverSilentlyOverwritten(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{
		"go.mod": "module shop\n",
		".env":   "SECRET=1\n",
	})
	// The captured .env danger zone carries a source hash; fresh capture is
	// not drifted.
	rep, err := f.store.DriftCheck(ctx, "shop")
	if err != nil {
		t.Fatalf("DriftCheck: %v", err)
	}
	if rep.Drifted {
		t.Fatalf("fresh capture reports drift: %+v", rep)
	}

	// Change the guarded secret file → drift surfaces, the capture is NOT
	// overwritten.
	f.writeFiles(e.StorePath, map[string]string{".env": "SECRET=2\n"})
	rep, err = f.store.DriftCheck(ctx, "shop")
	if err != nil {
		t.Fatalf("DriftCheck: %v", err)
	}
	if !rep.Drifted || len(rep.Reasons) == 0 {
		t.Fatalf("expected drift after source change, got %+v", rep)
	}
	after, _ := f.store.Get(ctx, "shop")
	if after.CaptureVersion != e.CaptureVersion {
		t.Fatalf("drift silently bumped the capture: %d -> %d", e.CaptureVersion, after.CaptureVersion)
	}

	// The explicit re-scan verb produces a NEW version (never in place).
	c, err := f.store.Rescan(ctx, "shop", "alice")
	if err != nil {
		t.Fatalf("Rescan: %v", err)
	}
	if c.Version != e.CaptureVersion+1 {
		t.Fatalf("rescan version = %d, want %d", c.Version, e.CaptureVersion+1)
	}
}
