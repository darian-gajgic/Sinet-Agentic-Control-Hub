package api_test

// projectcommandsbody_gf5_test.go — P3-GF5 drain r1, F2 and F8: the commands
// door's BODY boundary, and its pending-entry branch.
//
// WHY F2 IS A BOUNDARY AND NOT A PREFERENCE. Full-replacement semantics are
// ratified (brief R1): the submitted object becomes the whole captured set, so
// an omitted slot is cleared and an all-empty submission legitimately returns
// the project to Spec S07.8's bootstrap posture. What that CANNOT mean is that
// a malformed request erases the set. `readBody` promotes an empty body to
// `{}`, so before this fix a POST that lost its payload in transit — and a
// client with a `biuld` typo — both decoded to the zero value, wiped every
// captured command and answered 200. The destructive act and the honest act
// were the same bytes on the wire; now they are not.
//
// New file: the packet may not modify pre-existing test files, and the four
// brief-specified acceptance tests one file over are untouched. The helpers
// (`newProjEnv`, `seedCommandless`, `commandsPath`, `seedPending`, `count`) are
// this package's own and are CALLED, not changed.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
)

// gf5CaptureRowsFor counts a project's capture versions.
func gf5CaptureRowsFor(t *testing.T, e *projEnv, id string) int {
	t.Helper()
	return e.count(t, `SELECT COUNT(*) FROM repo_registry_captures WHERE project_id = ?`, id)
}

// TestGF5MalformedBodyNeverDestroysTheCapturedCommands [drain r1 F2]: the four
// shapes that used to erase a project's whole command set under a 200.
//
// Each is driven against a project that ALREADY holds commands, because that is
// what makes the failure destructive rather than merely wrong: the assertion is
// not only the status but that the captured commands are still there after it.
func TestGF5MalformedBodyNeverDestroysTheCapturedCommands(t *testing.T) {
	e := newProjEnv(t)
	seedCommandless(t, e, "p-body", nil)

	// The state worth protecting: a real captured set, version 2.
	if code, out := e.do(t, "alice", http.MethodPost, commandsPath("p-body"),
		`{"commands":{"build":"go build ./...","test":"go test ./..."}}`); code != http.StatusOK {
		t.Fatalf("seed write = %d; body: %s", code, out)
	}
	if n := gf5CaptureRowsFor(t, e, "p-body"); n != 2 {
		t.Fatalf("capture rows after the seed write = %d, want 2", n)
	}

	for _, tc := range []struct {
		name, body, names string
	}{
		// The empty body is the headline: readBody promotes it to `{}`, so
		// absence of the member has to be refused rather than read as "erase".
		{"an empty body", ``, "commands"},
		{"an empty object", `{}`, "commands"},
		{"an explicit null member", `{"commands":null}`, "commands"},
		// A typo'd slot dropped the command the caller meant to set AND erased
		// the ones already captured, under a 200.
		{"a typo'd slot", `{"commands":{"biuld":"make"}}`, "biuld"},
		// The typo beside a real slot is the same act with a plausible-looking
		// request: `test` would have landed and `build` would have vanished.
		{"a typo'd slot beside a real one", `{"commands":{"biuld":"make","test":"go test ./..."}}`, "biuld"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out := e.do(t, "alice", http.MethodPost, commandsPath("p-body"), tc.body)
			if code != http.StatusBadRequest {
				t.Fatalf("%s = %d (want 400); body: %s", tc.name, code, out)
			}
			if !strings.Contains(out, tc.names) {
				t.Errorf("the refusal does not name %q, so a caller cannot fix the request: %s", tc.names, out)
			}
			if n := gf5CaptureRowsFor(t, e, "p-body"); n != 2 {
				t.Fatalf("%s minted a version (capture rows = %d, want 2) — a refused write mints nothing", tc.name, n)
			}
			entry, err := e.proj.Get(e.ctx, "p-body")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if entry.Capture.Commands.Build != "go build ./..." || entry.Capture.Commands.Test != "go test ./..." {
				t.Fatalf("%s DESTROYED the captured commands: %+v", tc.name, entry.Capture.Commands)
			}
		})
	}
}

// TestGF5ExplicitEmptyCommandsStillClears [drain r1 F2, the other direction]:
// the fix must not close the door on the honest act. `{"commands":{}}` is a
// person saying "this project captures nothing" — R4 rules that legitimate, and
// it returns the project to the bootstrap posture where their review decides
// the work. It stays a 200, it mints a version, and the answer SAYS what it
// did, because silently clearing and silently refusing are both worse than
// either.
func TestGF5ExplicitEmptyCommandsStillClears(t *testing.T) {
	e := newProjEnv(t)
	seedCommandless(t, e, "p-clear", nil)
	if code, out := e.do(t, "alice", http.MethodPost, commandsPath("p-clear"),
		`{"commands":{"test":"go test ./..."}}`); code != http.StatusOK {
		t.Fatalf("first write = %d; body: %s", code, out)
	}

	code, out := e.do(t, "alice", http.MethodPost, commandsPath("p-clear"), `{"commands":{}}`)
	if code != http.StatusOK {
		t.Fatalf("explicit clear = %d (want 200: an empty set is a legitimate act, R4); body: %s", code, out)
	}
	if n := gf5CaptureRowsFor(t, e, "p-clear"); n != 3 {
		t.Fatalf("capture rows = %d, want 3 (seed + write + clear)", n)
	}
	entry, err := e.proj.Get(e.ctx, "p-clear")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.Capture.Commands != (project.Commands{}) {
		t.Fatalf("the explicit clear left commands behind: %+v", entry.Capture.Commands)
	}
	// The answer is honest about the consequence rather than just reporting OK.
	if !strings.Contains(strings.ToLower(out), "bootstrap") {
		t.Errorf("the clear's answer does not say the project is back in the bootstrap posture: %s", out)
	}
}

// TestGF5PendingProjectPointsAtTheOnboardingCard [drain r1 F8]: a project still
// waiting on its owner's D10 approval is refused HERE and told where its real
// door is. The draft — commands included — is edited on the onboarding card in
// the Inbox, and answering it there activates the entry; a second write path
// onto a pending draft would be one act with two audit stories, which is the
// "three doors and no fourth" shape this family was built to avoid.
func TestGF5PendingProjectPointsAtTheOnboardingCard(t *testing.T) {
	e := newProjEnv(t)
	e.seedPending(t, "p-pending", "alice", nil, project.CaptureInput{
		Conventions: []string{"tabs, not spaces"},
		ScanHash:    "scanhash-v1",
		Family:      project.FamilySoftware,
	})

	code, out := e.do(t, "alice", http.MethodPost, commandsPath("p-pending"),
		`{"commands":{"test":"go test ./..."}}`)
	if code != http.StatusConflict {
		t.Fatalf("pending write = %d (want 409); body: %s", code, out)
	}
	// The refusal has to point at the door that WORKS, in the operator's own
	// word for where a card lands — a route is not a place anybody goes.
	if !strings.Contains(out, "Inbox") {
		t.Errorf("the pending refusal does not name where the real door is: %s", out)
	}
	if n := gf5CaptureRowsFor(t, e, "p-pending"); n != 1 {
		t.Errorf("capture rows = %d, want 1: a refused write mints nothing", n)
	}
	// And the entry is still pending — the refusal changed nothing about it.
	entry, err := e.proj.Get(e.ctx, "p-pending")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.State != project.StatePending {
		t.Errorf("state = %q after the refusal, want pending", entry.State)
	}
}
