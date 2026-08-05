package shell

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
)

// P3-RW-1 (brief §8): the PRODUCTION seam test. registrySeam is the real
// intake.Registry the composition root wires; the stage-level battery drives an
// inlined copy, so this is where the shipped pin path is proven — including the
// full toRegistrySlice projection a pinned slice must share with a matched one
// (R6 slice parity), and the intake sentinels the refusal must carry.
//
// Colocated in a new file rather than appended to project_seams_test.go: the
// packet may not modify pre-existing test files.

// pinSeamFixture registers three entries at the data layer:
//
//	shop  — ACTIVE, owner alice, member bob, name "shop backend"
//	attic — ACTIVE, owner dana
//	draft — PENDING, owner alice
func pinSeamFixture(t *testing.T, proj *project.Store, root string) {
	t.Helper()
	ctx := context.Background()
	entries := []struct {
		id, owner, name string
		members         []string
		activate        bool
	}{
		{"shop", "alice", "shop backend", []string{"bob"}, true},
		{"attic", "dana", "attic", nil, true},
		{"draft", "alice", "draftproj", nil, false},
	}
	for _, e := range entries {
		if _, err := proj.Register(ctx, project.RegisterInput{
			ProjectID: e.id, Owner: e.owner, Name: e.name,
			StorePath: filepath.Join(root, "stores", e.id), Members: e.members,
		}); err != nil {
			t.Fatalf("Register %s: %v", e.id, err)
		}
		if _, err := proj.Capture(ctx, project.CaptureInput{
			ProjectID: e.id, By: e.owner,
			Conventions: []string{"tabs not spaces"},
			Commands:    project.Commands{Build: "go build ./...", Test: "go test ./..."},
			DangerZones: []project.DangerZone{{
				Path: "infra/", Action: "never", Rule: "force-push the default branch",
				SourceHash: "sha256:" + e.id,
			}},
		}); err != nil {
			t.Fatalf("Capture %s: %v", e.id, err)
		}
		if e.activate {
			if _, err := proj.Activate(ctx, e.id, e.owner); err != nil {
				t.Fatalf("Activate %s: %v", e.id, err)
			}
		}
	}
}

func pinSeam(t *testing.T) registrySeam {
	t.Helper()
	db, log, _ := seamDB(t)
	root := t.TempDir()
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(root, "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	pinSeamFixture(t, proj, root)
	return registrySeam{proj: proj}
}

// TestRegistrySeamHonorsSubmittedPin: a valid pin resolves the entry DIRECTLY,
// with the request text never naming it (product map §5 — the Projects-tab door
// must not depend on the user typing the project's name).
func TestRegistrySeamHonorsSubmittedPin(t *testing.T) {
	r := pinSeam(t)
	ctx := context.Background()

	for _, who := range []struct{ role, user string }{{"owner", "alice"}, {"member", "bob"}} {
		t.Run(who.role, func(t *testing.T) {
			slice, ok, err := r.Match(ctx, intake.Request{
				UserID: who.user, Title: "wishlist", Text: "no name here", Project: "shop",
			})
			if err != nil || !ok {
				t.Fatalf("pinned Match = ok:%v err:%v, want the shop slice", ok, err)
			}
			if slice.Project != "shop" {
				t.Fatalf("slice.Project = %q, want shop", slice.Project)
			}
			// R6: the FULL projection, identical to a matched slice's.
			if slice.Ref != "shop@capture-v1" {
				t.Fatalf("slice.Ref = %q, want the capture-versioned ref shop@capture-v1", slice.Ref)
			}
			if len(slice.Conventions) != 1 || slice.Conventions[0] != "tabs not spaces" {
				t.Fatalf("slice.Conventions = %v, want the captured conventions", slice.Conventions)
			}
			if len(slice.Commands) != 2 {
				t.Fatalf("slice.Commands = %v, want build+test", slice.Commands)
			}
			if len(slice.DangerZones) != 1 ||
				slice.DangerZones[0].Rule != "never: force-push the default branch" ||
				slice.DangerZones[0].SourceHash != "sha256:shop" {
				t.Fatalf("slice.DangerZones = %+v, want the captured zone with its source hash "+
					"(the ledger §2 pin / stakes floor, S05.1/S06.2)", slice.DangerZones)
			}
		})
	}
}

// TestRegistrySeamPinSliceParity: the pinned slice IS the matched slice — same
// projection, member for member (R6). If these ever diverge, danger zones, the
// stakes floor, registry slot resolution and the ledger §2 pin diverge with them.
func TestRegistrySeamPinSliceParity(t *testing.T) {
	r := pinSeam(t)
	ctx := context.Background()

	pinned, ok, err := r.Match(ctx, intake.Request{UserID: "alice", Text: "no name here", Project: "shop"})
	if err != nil || !ok {
		t.Fatalf("pinned Match = ok:%v err:%v", ok, err)
	}
	matched, ok, err := r.Match(ctx, intake.Request{UserID: "alice", Text: "fix the shop backend checkout"})
	if err != nil || !ok {
		t.Fatalf("text Match = ok:%v err:%v", ok, err)
	}
	if !reflect.DeepEqual(pinned, matched) {
		t.Fatalf("pinned slice differs from the matched slice:\n  pinned:  %+v\n  matched: %+v", pinned, matched)
	}
}

// TestRegistrySeamRefusesInvalidPin: the security edge. Every refusal class
// carries an intake sentinel (so Start routes it around the scan-path swallow),
// pins nothing, and unknown/invisible are indistinguishable.
func TestRegistrySeamRefusesInvalidPin(t *testing.T) {
	r := pinSeam(t)
	ctx := context.Background()

	cases := []struct {
		name, user, pin string
		want            error
	}{
		{"cross_user", "alice", "attic", intake.ErrPinUnknown}, // dana's project
		{"non_member", "carol", "shop", intake.ErrPinUnknown},  // sees nothing
		{"unknown_id", "alice", "ghost", intake.ErrPinUnknown}, // no such entry
		{"pending", "alice", "draft", intake.ErrPinNotActive},  // visible, not active
		{"pending_invisible", "carol", "draft", intake.ErrPinUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			slice, ok, err := r.Match(ctx, intake.Request{UserID: c.user, Text: "no name here", Project: c.pin})
			if ok {
				t.Fatalf("refused pin still pinned %+v", slice)
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if !errors.Is(err, intake.ErrPinRefused) {
				t.Fatalf("err = %v does not carry ErrPinRefused — Start would swallow it", err)
			}
			if !reflect.DeepEqual(slice, intake.RegistrySlice{}) {
				t.Fatalf("refused pin returned a slice: %+v", slice)
			}
		})
	}

	// No existence oracle at the seam either.
	_, _, invisible := r.Match(ctx, intake.Request{UserID: "alice", Project: "attic"})
	_, _, unknown := r.Match(ctx, intake.Request{UserID: "alice", Project: "ghost"})
	if invisible == nil || unknown == nil {
		t.Fatalf("both pins must refuse: invisible=%v unknown=%v", invisible, unknown)
	}
	a := strings.ReplaceAll(invisible.Error(), "attic", "PIN")
	b := strings.ReplaceAll(unknown.Error(), "ghost", "PIN")
	if a != b {
		t.Fatalf("seam refusals are distinguishable:\n  invisible: %s\n  unknown:   %s", a, b)
	}
}

// TestRegistrySeamUnpinnedUnchanged: without a pin the seam is exactly today's
// name-token scan, and a no-match still degrades silently (err == nil) — the
// pin refusal is the ONLY loud edge (S06.2 degrade posture, CONVENTIONS §14).
func TestRegistrySeamUnpinnedUnchanged(t *testing.T) {
	r := pinSeam(t)
	ctx := context.Background()

	slice, ok, err := r.Match(ctx, intake.Request{UserID: "alice", Text: "fix the shop backend checkout"})
	if err != nil || !ok || slice.Project != "shop" {
		t.Fatalf("unpinned text match = %+v ok:%v err:%v, want shop", slice, ok, err)
	}
	if _, ok, err := r.Match(ctx, intake.Request{UserID: "alice", Text: "nothing registered here"}); ok || err != nil {
		t.Fatalf("unpinned no-match = ok:%v err:%v, want a silent no-slice", ok, err)
	}
	// A project the requester cannot see is not matched by name either.
	if _, ok, err := r.Match(ctx, intake.Request{UserID: "carol", Text: "fix the shop backend"}); ok || err != nil {
		t.Fatalf("stranger text match = ok:%v err:%v, want a silent no-slice", ok, err)
	}
}
