package api_test

// projectsvocab_test.go — P3-RW-8 acceptance on the TRANSPORT side: the inbox
// serves the onboarding card's own declared vocabulary, the SPA's composed body
// reaches the answer path through the pinned door, and the create door's prose
// names the surface the operator uses rather than an API path.
//
// Committed RED per P3/briefs/P3-RW-8.md §7 (T-A1, T-A2, T-A3).
//
// WHAT IS ASSERTED HERE AND WHAT IS NOT (declared, brief §7 T-A1's own
// allowance): `internal/api` never imports `internal/stage`, so this file seeds
// the snapshot literal in the shape `dispatchOnboard` writes it and asserts what
// the TRANSPORT derives from that shape — the served actions, tier, batching,
// step-up, pin, answerability and card body. The producer's side of the tie —
// that the shape seeded here is the shape the producer actually writes, and that
// the declared verb is the one its answer path applies — is pinned in
// internal/stage (onboardvocab_e2e_test.go), which drives the real dispatch.
//
// The projects fixture's answer seam (projects_test.go `onboardSeam.Answer`,
// pre-existing and unmodifiable) approves without reading the answer body and
// files no board move, so T-A2 asserts here exactly what this composition can
// honestly observe — the transport's 200 / applied / state, the activation, the
// retry, the stale pin and the ownership discipline. The envelope semantics and
// the task-to-Done move are proven end to end over the REAL stage composition in
// internal/stage's TestOnboardApproveFromTheServedCardEndToEnd.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// onboardVerb is the ONE verb S13.7 defines for this card (R1/R3). No
// decline/deny/reject verb exists anywhere: the answer path refuses a
// non-approval outright, so declaring one would be a control with nothing
// behind it.
const onboardVerb = "approve"

// newShapeOnboardSnapshot is the durable snapshot as `dispatchOnboard` writes
// it after this packet: the top-level fields the answer path and the audit read,
// PLUS the S06.7 `decision` body the inbox derives verbs from and the SPA
// composes {choice} for. Hand-written here because of the import wall; its
// fidelity to the producer is the stage-side test's assertion.
func newShapeOnboardSnapshot(projectID, owner string) string {
	return fmt.Sprintf(`{"kind":"onboard","project_id":%q,"owner":%q,`+
		`"draft":{"conventions":["Go project: gofmt-clean, go vet-clean"],"commands":{"build":"go build ./..."}},`+
		`"issued_ts":"2026-08-12T00:00:00Z",`+
		`"decision":{"summary":"Approve the drafted capture for project %s.",`+
		`"detail":["Convention: Go project: gofmt-clean, go vet-clean","Build command: go build ./..."],`+
		`"choices":[{"label":"Approve — activate this project","value":%q}],`+
		`"help":{"what":"Approving files the drafted conventions, commands and danger zones as this project's active entry.",`+
		`"wrong":"If the draft is wrong, the platform works from wrong conventions until you re-scan.",`+
		`"recommend":"Approve if the draft matches the project. To not onboard it at all, cancel the onboarding task."}}}`,
		projectID, owner, projectID, onboardVerb)
}

// seedDispatchedOnboarding puts the project's durable owner-approval ask in the
// world exactly as the scheduler's dispatch of the onboarding run leaves it.
func (e *projEnv) seedDispatchedOnboarding(t *testing.T, projectID, owner string) {
	t.Helper()
	exec(t, e.b, `INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?, ?, ?, ?, 'open', ?)
	              ON CONFLICT (ask_id) DO UPDATE SET snapshot = excluded.snapshot, status = 'open'`,
		"onboard:"+projectID, "onboard-"+projectID+".onboard", owner,
		newShapeOnboardSnapshot(projectID, owner), time.Now().UTC().Format(time.RFC3339Nano))
}

// inboxItem is the slice of the served card these tests read.
type inboxItem struct {
	ID                  string          `json:"id"`
	Kind                string          `json:"kind"`
	Tier                string          `json:"tier"`
	Answerable          bool            `json:"answerable"`
	NotAnswerableReason string          `json:"not_answerable_reason"`
	Batchable           bool            `json:"batchable"`
	StepUpRequired      bool            `json:"step_up_required"`
	PayloadHash         string          `json:"payload_hash"`
	Actions             []string        `json:"actions"`
	Card                json.RawMessage `json:"card"`
}

func (e *projEnv) inboxCard(t *testing.T, who, id string) inboxItem {
	t.Helper()
	code, out := e.do(t, who, "GET", "/api/approvals", "")
	if code != http.StatusOK {
		t.Fatalf("GET /api/approvals as %s: %d %s", who, code, out)
	}
	var list struct {
		Items []inboxItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("decode inbox: %v: %s", err, out)
	}
	for _, it := range list.Items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("%s is not in %s's inbox: %s", id, who, out)
	return inboxItem{}
}

// ── T-A1: the served vocabulary ─────────────────────────────────────────────

// TestOnboardCardServesItsApproveVerb — brief T-A1, checklist 1. The card the
// owner is served carries the one verb its answer path applies, at the tier the
// landed answer path actually demands, with the pin an answer quotes back — and
// everyone else reads it without being able to decide it (D10).
func TestOnboardCardServesItsApproveVerb(t *testing.T) {
	e := newProjEnv(t)
	if code, out := e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-new","name":"New Proj"}`); code != http.StatusOK {
		t.Fatalf("POST /api/projects: %d %s", code, out)
	}
	e.seedDispatchedOnboarding(t, "p-new", "alice")

	it := e.inboxCard(t, "alice", "ask:onboard:p-new")
	if it.Kind != "ask" {
		t.Errorf("served kind = %q, want ask", it.Kind)
	}
	if !it.Answerable {
		t.Errorf("the owner's own onboarding card is not answerable: %s", it.NotAnswerableReason)
	}
	if strings.Join(it.Actions, "|") != onboardVerb {
		t.Errorf("served actions = %v, want exactly [%q] — the SPA renders controls strictly from this list, so an "+
			"empty one is a card the operator cannot answer from any surface (R1)", it.Actions, onboardVerb)
	}
	if it.Tier != "medium" {
		t.Errorf("served tier = %q, want medium — the untiered onboarding card reads Medium by the transport's own "+
			"ratified default (R6)", it.Tier)
	}
	if it.Batchable || it.StepUpRequired {
		t.Errorf("served batchable/step_up = %v/%v, want false/false — Medium is individual and un-stepped (S15.6)",
			it.Batchable, it.StepUpRequired)
	}
	if it.PayloadHash == "" {
		t.Errorf("the served card carries no payload_hash: an answer is pinned to the card it was shown for (S15.2)")
	}
	// The card body travels verbatim, help block included — that is what the
	// person deciding reads.
	var card struct {
		Decision *struct {
			Summary string   `json:"summary"`
			Detail  []string `json:"detail"`
			Choices []struct {
				Label string `json:"label"`
				Value string `json:"value"`
			} `json:"choices"`
			Help struct {
				What      string `json:"what"`
				Wrong     string `json:"wrong"`
				Recommend string `json:"recommend"`
			} `json:"help"`
		} `json:"decision"`
	}
	if err := json.Unmarshal(it.Card, &card); err != nil {
		t.Fatalf("decode served card: %v: %s", err, it.Card)
	}
	if card.Decision == nil {
		t.Fatalf("the served card carries no decision body: %s", it.Card)
	}
	if card.Decision.Summary == "" || len(card.Decision.Detail) == 0 {
		t.Errorf("the served card shows nothing of what is being approved: %+v", card.Decision)
	}
	if card.Decision.Help.What == "" || card.Decision.Help.Wrong == "" || card.Decision.Help.Recommend == "" {
		t.Errorf("the 13.5 help block is incomplete on the served card: %+v", card.Decision.Help)
	}
	for _, ch := range card.Decision.Choices {
		if ch.Label == "" {
			t.Errorf("choice %q is served without a label", ch.Value)
		}
	}

	// D10 as the transport states it: the operator SEES another person's card
	// and cannot decide it, with the reason said out loud.
	op := e.inboxCard(t, "op", "ask:onboard:p-new")
	if op.Answerable {
		t.Errorf("the operator can answer another person's onboarding card — D10 is authority over what the platform " +
			"DOES, never authorship of somebody else's decision")
	}
	if !strings.Contains(strings.ToLower(op.NotAnswerableReason), "owner") {
		t.Errorf("the read-only reason does not name the owner rule: %q", op.NotAnswerableReason)
	}
}

// ── T-A2: the SPA's composed body through the pinned door ───────────────────

// TestOnboardApproveThroughTheApprovalsDoor — brief T-A2, checklist 2. The body
// the SPA composes for a `decision` card, on the approvals answer route, with
// the served pin: it applies, it activates, and a repeat changes nothing.
func TestOnboardApproveThroughTheApprovalsDoor(t *testing.T) {
	e := newProjEnv(t)
	if code, out := e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-new","name":"New Proj"}`); code != http.StatusOK {
		t.Fatalf("POST /api/projects: %d %s", code, out)
	}
	e.seedDispatchedOnboarding(t, "p-new", "alice")
	const routeID = "ask:onboard:p-new"
	served := e.inboxCard(t, "alice", routeID)

	// A non-owner is refused before anything is applied; an id that does not
	// exist is a 404 to everyone (404-before-403, §38).
	if code, out := e.do(t, "bob", "POST", "/api/approvals/"+routeID+"/answer",
		fmt.Sprintf(`{"payload_hash":%q,"answer":{"choice":%q}}`, served.PayloadHash, onboardVerb)); code != http.StatusForbidden {
		t.Errorf("a non-owner answer = %d, want 403 (D10): %s", code, out)
	}
	if code, out := e.do(t, "bob", "POST", "/api/approvals/ask:onboard:p-nothing/answer",
		`{"payload_hash":"x","answer":{"choice":"approve"}}`); code != http.StatusNotFound {
		t.Errorf("an unknown ask = %d, want 404: %s", code, out)
	}

	// The pin is the card the answerer was shown: a stale one refuses AND hands
	// back the current card rather than a bare error.
	code, out := e.do(t, "alice", "POST", "/api/approvals/"+routeID+"/answer",
		fmt.Sprintf(`{"payload_hash":"sha256:not-the-card","answer":{"choice":%q}}`, onboardVerb))
	if code != http.StatusConflict {
		t.Errorf("a stale pin = %d, want 409: %s", code, out)
	}
	if !strings.Contains(out, "stale_payload") || !strings.Contains(out, served.PayloadHash) {
		t.Errorf("the stale refusal does not carry the fresh card back: %s", out)
	}
	if body := e.mustGet(t, "alice", "/api/projects/p-new"); strings.Contains(body, `"state":"active"`) {
		t.Errorf("a refused answer activated the entry anyway: %s", body)
	}

	// The exact body the SPA composes for the family this card now declares.
	answer := fmt.Sprintf(`{"payload_hash":%q,"answer":{"choice":%q}}`, served.PayloadHash, onboardVerb)
	code, out = e.do(t, "alice", "POST", "/api/approvals/"+routeID+"/answer", answer)
	if code != http.StatusOK {
		t.Fatalf("the body the SPA composes was refused: %d %s", code, out)
	}
	var res struct {
		Applied bool   `json:"applied"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode answer result: %v: %s", err, out)
	}
	if !res.Applied || res.State != "answered" {
		t.Errorf("answer result = %+v, want applied w/ state answered", res)
	}
	if body := e.mustGet(t, "alice", "/api/projects/p-new"); !strings.Contains(body, `"state":"active"`) {
		t.Errorf("after the served answer the entry does not read active: %s", body)
	}

	// Retry-safety (S15.2): the identical POST fires NOTHING a second time.
	//
	// The assertion is "never applied twice" rather than a status, because the
	// two honest already-resolved answers differ by whether the answer path
	// RECORDED the answer body: production records it (stage's AnswerOnboarding
	// writes the `answer` column) and reads the identical retry back at 200
	// applied:false; this fixture's seam records only the status, so the same
	// retry is the equally-correct 409 already_answered. The exact production
	// shape is pinned over the real composition in internal/stage's
	// TestOnboardApproveFromTheServedCardEndToEnd.
	code, out = e.do(t, "alice", "POST", "/api/approvals/"+routeID+"/answer", answer)
	if code == http.StatusOK {
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatalf("decode retry result: %v: %s", err, out)
		}
		if res.Applied {
			t.Errorf("the retry re-applied the answer — a phone that sends twice must not fire twice: %s", out)
		}
	} else if code != http.StatusConflict {
		t.Errorf("the retry = %d, want an already-resolved answer (200 not-applied, or 409 already-answered): %s", code, out)
	}
}

func (e *projEnv) mustGet(t *testing.T, who, path string) string {
	t.Helper()
	code, out := e.do(t, who, "GET", path, "")
	if code != http.StatusOK {
		t.Fatalf("GET %s as %s: %d %s", path, who, code, out)
	}
	return out
}

// ── T-A3: the create door speaks operator language ──────────────────────────

// TestProjectsDetailNamesTheInboxNotTheApiPath — brief T-A3, checklist 5 (R7).
// The create door's answer is read by a PERSON. It must name the surface where
// the card lands — the Inbox — and never an API path; the machine reference
// already rides the structured `ask_ref` field, so prose and reference stay
// cleanly split.
func TestProjectsDetailNamesTheInboxNotTheApiPath(t *testing.T) {
	e := newProjEnv(t)
	var started struct {
		AskRef string `json:"ask_ref"`
		TaskID string `json:"task_id"`
		Detail string `json:"detail"`
	}
	code, out := e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-new","name":"New Proj"}`)
	if code != http.StatusOK {
		t.Fatalf("POST /api/projects: %d %s", code, out)
	}
	if err := json.Unmarshal([]byte(out), &started); err != nil {
		t.Fatalf("decode create response: %v: %s", err, out)
	}
	if !strings.Contains(started.Detail, "Inbox") {
		t.Errorf("the create door does not name the surface the card lands on. The operator's word for it is Inbox "+
			"(web/src/routes.ts); an API path is not a place a person goes (R7): %q", started.Detail)
	}
	if strings.Contains(started.Detail, "/api/") {
		t.Errorf("an API path appears in operator prose: %q", started.Detail)
	}
	if started.AskRef != "onboard:p-new" {
		t.Errorf("ask_ref = %q, want onboard:p-new — the machine reference rides the structured field, not the prose",
			started.AskRef)
	}

	// The in-flight sentence is the same kind of claim and gets the same
	// standard: its facts stay, its spec citation goes.
	code, out = e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-new","name":"New Proj"}`)
	if code != http.StatusOK {
		t.Fatalf("the repeat POST: %d %s", code, out)
	}
	if err := json.Unmarshal([]byte(out), &started); err != nil {
		t.Fatalf("decode in-flight response: %v: %s", err, out)
	}
	if !strings.Contains(started.Detail, "already started") {
		t.Fatalf("the repeat POST did not answer with the in-flight sentence: %q", started.Detail)
	}
	if strings.Contains(started.Detail, "/api/") {
		t.Errorf("an API path appears in the in-flight prose: %q", started.Detail)
	}
	if strings.Contains(started.Detail, "S15.2") {
		t.Errorf("a spec citation appears in operator prose — the person reading this does not have the spec (R7): %q",
			started.Detail)
	}
}
