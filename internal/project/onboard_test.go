package project

import (
	"context"
	"testing"
)

func TestOnboardingAsTaskEndToEnd(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	// register → clone (local fixture) → scan → draft conventions/commands/
	// danger-zones → owner (D10) approves → active (Spec S13.7).
	src := f.fixtureRepo(map[string]string{
		"go.mod":        "module shop\n",
		".editorconfig": "root = true\n",
		".env":          "TOKEN=abc\n",
	})
	e, draft, err := f.store.Onboard(ctx, OnboardInput{
		ProjectID: "shop", Owner: "alice", Name: "shop", Source: src,
		RemoteURL: "git@github.com:alice/shop.git", // stored, never dialed
	})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if draft.Commands.Test != "go test ./..." {
		t.Fatalf("scanned test command = %q, want go test ./...", draft.Commands.Test)
	}
	if len(draft.Conventions) == 0 {
		t.Fatal("scan drafted no conventions from marker files")
	}
	var sawEnv bool
	for _, z := range draft.DangerZones {
		if z.Path == ".env" {
			sawEnv = true
			if z.SourceHash == "" {
				t.Fatal(".env danger zone carries no source hash")
			}
		}
	}
	if !sawEnv {
		t.Fatal("scan missed the .env secrets danger zone")
	}
	if e.State != StatePending {
		t.Fatalf("onboarded state = %q, want pending", e.State)
	}
	// The remote URL is stored but never dialed (no network in this packet).
	if e.RemoteURL == "" {
		t.Fatal("remote URL not stored")
	}

	// Owner approves the (optionally edited) draft → active.
	edited := draft
	edited.Conventions = append(edited.Conventions, "PRs require a green CI")
	edited.ScanHash = scanHash(edited)
	act, err := f.store.Approve(ctx, "shop", "alice", &edited)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if !act.Active() {
		t.Fatal("entry not active after owner approval")
	}
	// The edited draft was captured as a new version (v2) before activation.
	if act.CaptureVersion != 2 {
		t.Fatalf("capture version after edited approval = %d, want 2", act.CaptureVersion)
	}
	found := false
	for _, c := range act.Capture.Conventions {
		if c == "PRs require a green CI" {
			found = true
		}
	}
	if !found {
		t.Fatal("owner's edited convention did not land in the active capture")
	}
}

func TestOnboardNoRepoCreatesOne(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	// A v0 project without a repo gets one created + registered by the same
	// task — documents ride the identical machinery (Spec S13.7, R7).
	e, _, err := f.store.Onboard(ctx, OnboardInput{ProjectID: "notes", Owner: "alice", Name: "notes"})
	if err != nil {
		t.Fatalf("Onboard (no repo): %v", err)
	}
	if !f.store.isRepo(ctx, e.StorePath) {
		t.Fatal("no-repo onboarding did not create a git repository")
	}
	// The created repo has a HEAD so run-branch worktrees can base on it.
	head, err := f.store.headSHA(ctx, e.StorePath)
	if err != nil || head == "" {
		t.Fatalf("created repo has no HEAD: sha=%q err=%v", head, err)
	}
	if _, err := f.store.Approve(ctx, "notes", "alice", nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	// The created repo is usable as a workspace substrate.
	if _, err := f.store.EnsureWorkspace(ctx, "notes", "notes-task-1"); err != nil {
		t.Fatalf("EnsureWorkspace on created repo: %v", err)
	}
}

func TestScanDeterministicHeuristics(t *testing.T) {
	f := newFix(t)
	dir := t.TempDir()
	f.writeFiles(dir, map[string]string{
		"package.json":             `{"name":"web"}`,
		"pnpm-lock.yaml":           "lockfileVersion: 9\n",
		".github/workflows/ci.yml": "on: push\n",
	})
	d, err := f.store.Scan(dir, "main")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	// pnpm detected (lockfile present) over the plain npm fallback.
	if d.Commands.Test != "pnpm test" {
		t.Fatalf("commands.test = %q, want pnpm test", d.Commands.Test)
	}
	// The CI workflow directory is a danger zone.
	var sawCI bool
	for _, z := range d.DangerZones {
		if z.Path == ".github/workflows" {
			sawCI = true
		}
	}
	if !sawCI {
		t.Fatal("scan missed the .github/workflows danger zone")
	}
	// The scan is deterministic: a second scan of the same tree agrees.
	d2, _ := f.store.Scan(dir, "main")
	if d.ScanHash != d2.ScanHash {
		t.Fatalf("scan hash not deterministic: %q vs %q", d.ScanHash, d2.ScanHash)
	}
}
