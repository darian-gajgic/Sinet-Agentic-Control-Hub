package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
)

// projectcommands_gf5_test.go — P3-GF5 acceptance tests for the S13.7
// project-Commands write route, COMMITTED RED at grounding (CONVENTIONS §3
// Amendment-A: `POST /api/projects/{project}/commands` does not exist yet —
// every test here fails on the mux's own 404 — and the packet's implementation
// commit closes the window; the RW-2 red-battery precedent one file over).
//
// WHAT BINDS AND WHAT DOES NOT: the ASSERTIONS bind — derived from Spec S13.7
// ("Rows are owner-attributed; captured content is versioned"), S15.2
// (server-side authority; writes are retry-safe; every mutation lands on the
// event log), S07.8 [A14] ("once commands exist, the full ladder resumes" —
// which is why this door must mint a capture the landed CheckPackFor picks up),
// and the operator record P3/design/b6-gate-operator-findings-r4-2026-08-23.md
// §F1b (the card's missing door). Response FIELD NAMES inside bodies are
// asserted at substring level only; the executor owns the exact wire shapes
// within P3/briefs/P3-GF5.md. The projEnv harness (projects_test.go) is the
// seam: the executor wires the new Config surface in `projEnv.server`, exactly
// as the RW-2 battery instructed for the onboard door.

// commandsPath is the R1 route under test (brief OQ2: POST, the family's one
// mutating vocabulary).
func commandsPath(id string) string { return "/api/projects/" + id + "/commands" }

// seedCommandless seeds one ACTIVE project for alice with owner-approved
// conventions, a danger zone, a family and a scan hash — and NO commands: the
// r4-F1 fresh-scaffold shape, whose verification runs under the GF4 bootstrap
// posture until this packet's door captures commands.
func seedCommandless(t *testing.T, e *projEnv, id string, members []string) {
	t.Helper()
	e.seedActive(t, id, "alice", members, project.CaptureInput{
		Conventions: []string{"tabs, not spaces"},
		DangerZones: []project.DangerZone{{Path: "deploy/", Rule: "never touched by tasks", SourceHash: "abc123"}},
		ScanHash:    "scanhash-v1",
		Family:      project.FamilySoftware,
	})
}

// TestGF5CommandsWriteMintsANewCaptureVersion — Spec S13.7: the owner puts
// real build/test/lint commands into the registry over HTTP; the write is a
// NEW immutable capture version (never an overwrite) attributed to the owner,
// with conventions, danger zones, scan hash and family carried forward
// byte-equal (the Rescan carry-forward discipline: an edit of one member must
// not silently unset owner-approved content). The detail read round-trips the
// commands, so the GF6 editor reads back what was written.
func TestGF5CommandsWriteMintsANewCaptureVersion(t *testing.T) {
	e := newProjEnv(t)
	seedCommandless(t, e, "p-cmd", nil)

	code, body := e.do(t, "alice", http.MethodPost, commandsPath("p-cmd"),
		`{"commands":{"build":"go build ./...","test":"go test ./...","lint":"gofmt -l ."}}`)
	if code != http.StatusOK {
		t.Fatalf("owner commands write = %d (want 200); body: %s", code, body)
	}

	if n := e.count(t, `SELECT COUNT(*) FROM repo_registry_captures WHERE project_id = ?`, "p-cmd"); n != 2 {
		t.Fatalf("capture rows after write = %d (want 2: the seeded v1 plus the edit's NEW version)", n)
	}
	entry, err := e.proj.Get(e.ctx, "p-cmd")
	if err != nil {
		t.Fatalf("Get after write: %v", err)
	}
	if entry.CaptureVersion != 2 {
		t.Errorf("capture pointer = v%d (want v2)", entry.CaptureVersion)
	}
	cap := entry.Capture
	if cap.Commands.Build != "go build ./..." || cap.Commands.Test != "go test ./..." || cap.Commands.Lint != "gofmt -l ." {
		t.Errorf("commands did not land verbatim: %+v", cap.Commands)
	}
	if cap.CapturedBy != "alice" {
		t.Errorf("captured_by = %q (want the caller %q — S13.7 owner-attributed)", cap.CapturedBy, "alice")
	}
	// The carry-forward: everything that is not commands is byte-equal to v1.
	if len(cap.Conventions) != 1 || cap.Conventions[0] != "tabs, not spaces" {
		t.Errorf("conventions not carried forward: %v", cap.Conventions)
	}
	if len(cap.DangerZones) != 1 || cap.DangerZones[0].Path != "deploy/" || cap.DangerZones[0].SourceHash != "abc123" {
		t.Errorf("danger zones not carried forward: %v", cap.DangerZones)
	}
	if cap.ScanHash != "scanhash-v1" {
		t.Errorf("scan hash not carried forward: %q (DriftCheck would compare against nothing)", cap.ScanHash)
	}
	if cap.Family != project.FamilySoftware {
		t.Errorf("family not carried forward: %q", cap.Family)
	}

	code, detail := e.do(t, "alice", http.MethodGet, "/api/projects/p-cmd", "")
	if code != http.StatusOK {
		t.Fatalf("detail read = %d; body: %s", code, detail)
	}
	if !strings.Contains(detail, "go test ./...") {
		t.Errorf("detail read does not round-trip the captured test command; body: %s", detail)
	}
}

// TestGF5CommandsWriteIsOwnerOnly — Spec S13.7 (owner-attributed rows; D10
// "their own object") + S15.2 (authority server-side): only the owner writes.
// An invited MEMBER gets an honest 403; a caller with no standing gets the ONE
// 404 that is byte-identical to the unknown-id refusal (the noSuchPin
// discipline — this door must not become an existence oracle); the dev-posture
// identity browses but never files (the create door's own rule). Nobody but
// the owner ever mints a capture row.
func TestGF5CommandsWriteIsOwnerOnly(t *testing.T) {
	e := newProjEnv(t)
	seedCommandless(t, e, "p-own", []string{"bob"})
	body := `{"commands":{"test":"go test ./..."}}`

	code, resp := e.do(t, "bob", http.MethodPost, commandsPath("p-own"), body)
	if code != http.StatusForbidden {
		t.Errorf("invited member write = %d (want 403: visible, but not theirs to write); body: %s", code, resp)
	}
	if code == http.StatusForbidden && !strings.Contains(resp, "alice") {
		t.Errorf("the member's refusal should name the owner they must ask; body: %s", resp)
	}

	strangerCode, strangerBody := e.do(t, "carol", http.MethodPost, commandsPath("p-own"), body)
	unknownCode, unknownBody := e.do(t, "carol", http.MethodPost, commandsPath("p-none"), body)
	if strangerCode != http.StatusNotFound {
		t.Errorf("stranger write = %d (want the one 404)", strangerCode)
	}
	if strangerCode != unknownCode || strangerBody != unknownBody {
		t.Errorf("invisible entry and unknown id must be ONE indistinguishable answer:\n  invisible: %d %s\n  unknown:   %d %s",
			strangerCode, strangerBody, unknownCode, unknownBody)
	}

	devCode, devBody := e.do(t, auth.DevUserID, http.MethodPost, commandsPath("p-own"), body)
	if devCode != http.StatusForbidden {
		t.Errorf("dev-posture write = %d (want 403: browsing is not filing); body: %s", devCode, devBody)
	}

	if n := e.count(t, `SELECT COUNT(*) FROM repo_registry_captures WHERE project_id = ?`, "p-own"); n != 1 {
		t.Errorf("capture rows = %d (want 1: no refused caller may have minted a version)", n)
	}
}

// TestGF5CommandsWriteValidatesAtTheBoundary — S15.2 validate-at-the-boundary:
// a captured command is ONE shell line the verification sandbox will later run
// (S07.3; project_seams packChecks). A multi-line submission smuggles a script
// body past the "one rung, one line" shape and is refused; an absurdly long
// one is refused by the structural cap. Refusals are 400s that name what was
// wrong, and neither mints a capture version. Nothing is executed at capture
// time on any path.
func TestGF5CommandsWriteValidatesAtTheBoundary(t *testing.T) {
	e := newProjEnv(t)
	seedCommandless(t, e, "p-val", nil)

	code, body := e.do(t, "alice", http.MethodPost, commandsPath("p-val"),
		`{"commands":{"build":"make build\nrm -rf /"}}`)
	if code != http.StatusBadRequest {
		t.Errorf("multi-line command = %d (want 400); body: %s", code, body)
	}
	if code == http.StatusBadRequest && !strings.Contains(strings.ToLower(body), "line") {
		t.Errorf("the multi-line refusal should name the one-line rule; body: %s", body)
	}

	oversize := strings.Repeat("x", 10000)
	code, body = e.do(t, "alice", http.MethodPost, commandsPath("p-val"),
		`{"commands":{"test":"`+oversize+`"}}`)
	if code != http.StatusBadRequest {
		t.Errorf("oversize command = %d (want 400); body: %s", code, body)
	}

	if n := e.count(t, `SELECT COUNT(*) FROM repo_registry_captures WHERE project_id = ?`, "p-val"); n != 1 {
		t.Errorf("capture rows = %d (want 1: a refused write mints nothing)", n)
	}
}

// TestGF5RepeatedCommandsWriteMintsNoSecondVersion — S15.2 "a repeated answer
// returns the already-resolved state — a phone retry can never double-fire":
// resubmitting the byte-identical command set answers 200 with the current
// state and mints neither a capture version nor a registry.captured event.
func TestGF5RepeatedCommandsWriteMintsNoSecondVersion(t *testing.T) {
	e := newProjEnv(t)
	seedCommandless(t, e, "p-again", nil)
	body := `{"commands":{"build":"go build ./...","test":"go test ./..."}}`

	if code, resp := e.do(t, "alice", http.MethodPost, commandsPath("p-again"), body); code != http.StatusOK {
		t.Fatalf("first write = %d; body: %s", code, resp)
	}
	if code, resp := e.do(t, "alice", http.MethodPost, commandsPath("p-again"), body); code != http.StatusOK {
		t.Fatalf("repeated write = %d (want 200: retry-safe, never an error); body: %s", code, resp)
	}

	if n := e.count(t, `SELECT COUNT(*) FROM repo_registry_captures WHERE project_id = ?`, "p-again"); n != 2 {
		t.Errorf("capture rows = %d (want 2: seed v1 + ONE edit — the retry minted nothing)", n)
	}
	// Seeding minted one registry.captured (the seedActive Capture); the door's
	// first write minted the second; the retry appended NOTHING.
	if n := e.count(t, `SELECT COUNT(*) FROM run_events WHERE type = 'registry.captured' AND user_id = 'alice'`); n != 2 {
		t.Errorf("registry.captured events = %d (want 2: seed + one real edit; the retry is silent)", n)
	}
}
