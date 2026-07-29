package api_test

// oversightreads_test.go — the additive B6-5 read fields (OQ4/OQ5/OQ6).
//
// The golden fixtures pin the SHAPE; these pin the DERIVATIONS and their
// negatives — what the card face is allowed to say when the platform disclosed
// nothing, and which decisions are this task's.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type wireTaskList struct {
	Tasks []struct {
		TaskID    string `json:"task_id"`
		Owner     string `json:"owner"`
		LatestRun *struct {
			RunID          string   `json:"run_id"`
			State          string   `json:"state"`
			Stage          string   `json:"stage"`
			WaitingOnHuman bool     `json:"waiting_on_human"`
			ParkedUntil    *string  `json:"parked_until"`
			CostSoFarUSD   *float64 `json:"cost_so_far_usd"`
			EffortMode     string   `json:"effort_mode"`
			DowngradeNote  string   `json:"downgrade_note"`
			QueueHintRank  *int64   `json:"queue_hint_rank"`
		} `json:"latest_run"`
	} `json:"tasks"`
}

func fixtureGet(t *testing.T, b *backend, who, path string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	fixtureServer(t, b, who).Handler().ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s as %s: %d: %s", path, who, rr.Code, rr.Body.String())
	}
	return rr.Body.String()
}

// TestCardFaceIsServedAtTheListGrain is OQ4: the S1.3 face is answerable from
// ONE list read, and every element that the platform did not record is ABSENT
// rather than filled with a plausible-looking value.
func TestCardFaceIsServedAtTheListGrain(t *testing.T) {
	b := fixtureWorld(t)
	var list wireTaskList
	if err := json.Unmarshal([]byte(fixtureGet(t, b, "op", "/api/tasks")), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byID := map[string]int{}
	for i, tk := range list.Tasks {
		byID[tk.TaskID] = i
	}
	for _, want := range []string{"t-ship", "t-triage", "t-audit", "t-archive"} {
		if _, ok := byID[want]; !ok {
			t.Fatalf("task %s missing from the list", want)
		}
	}

	ship := list.Tasks[byID["t-ship"]].LatestRun
	if ship == nil {
		t.Fatal("the shipping task has no latest run on its card")
	}
	if ship.Stage != "execute" {
		t.Errorf("stage = %q, want the run's own stage marker", ship.Stage)
	}
	if ship.EffortMode != "standard" {
		t.Errorf("effort_mode = %q, want the routing record's effort", ship.EffortMode)
	}
	// The downgrade note is ABSENT because the routing record disclosed none —
	// the mandatory plain-language routing reason must NOT be promoted into one.
	if ship.DowngradeNote != "" {
		t.Errorf("downgrade_note = %q on a routing record that disclosed none", ship.DowngradeNote)
	}
	if ship.CostSoFarUSD == nil || *ship.CostSoFarUSD != 1.42 {
		t.Errorf("cost_so_far_usd = %v, want the figure READ from the meter seam", ship.CostSoFarUSD)
	}

	// The other direction: a producer that DID disclose one.
	if note := list.Tasks[byID["t-triage"]].LatestRun.DowngradeNote; !strings.Contains(note, "effort dropped") {
		t.Errorf("downgrade_note = %q, want the disclosed note served verbatim", note)
	}

	// Waiting-on-a-human and the park horizon, both derived, both on the card.
	audit := list.Tasks[byID["t-audit"]].LatestRun
	if !audit.WaitingOnHuman {
		t.Error("a parked run with an open ask must read waiting_on_human on the card")
	}
	if audit.ParkedUntil == nil || *audit.ParkedUntil != "2026-07-20T12:00:00Z" {
		t.Errorf("parked_until = %v, want the marker's own time", audit.ParkedUntil)
	}

	// The honest nil: no meter reading is not a zero.
	if cost := list.Tasks[byID["t-archive"]].LatestRun.CostSoFarUSD; cost != nil {
		t.Errorf("cost_so_far_usd = %v for a run with no meter reading, want null", *cost)
	}

	// OQ6(i): the recorded drag order rides the list, and 0 is the scheduler's
	// own "no hint" rather than a position.
	if r := list.Tasks[byID["t-triage"]].LatestRun.QueueHintRank; r == nil || *r != 10 {
		t.Errorf("queue_hint_rank = %v, want the recorded rank", r)
	}
	if r := list.Tasks[byID["t-archive"]].LatestRun.QueueHintRank; r == nil || *r != 0 {
		t.Errorf("queue_hint_rank = %v on an un-hinted queued run, want 0 (no hint)", r)
	}
	if r := list.Tasks[byID["t-ship"]].LatestRun.QueueHintRank; r != nil {
		t.Errorf("queue_hint_rank = %v on a task with no queued run, want null", *r)
	}
}

// TestCardFaceIsOwnerScoped: the additive fields ride an already-scoped read,
// and adding them must not have widened it.
func TestCardFaceIsOwnerScoped(t *testing.T) {
	b := fixtureWorld(t)
	var mine wireTaskList
	if err := json.Unmarshal([]byte(fixtureGet(t, b, "alice", "/api/tasks")), &mine); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(mine.Tasks) == 0 {
		t.Fatal("alice sees none of her own tasks — the scan proves nothing")
	}
	for _, tk := range mine.Tasks {
		if tk.Owner != "alice" {
			t.Fatalf("a member's task list carries %s's work", tk.Owner)
		}
	}
	// And the operator sees more than one owner, so the check above is a real
	// filter rather than a fixture with one person in it.
	var all wireTaskList
	if err := json.Unmarshal([]byte(fixtureGet(t, b, "op", "/api/tasks")), &all); err != nil {
		t.Fatalf("decode: %v", err)
	}
	owners := map[string]bool{}
	for _, tk := range all.Tasks {
		owners[tk.Owner] = true
	}
	if len(owners) < 2 {
		t.Fatalf("the fixture world has %d owners — the member-scope assertion is vacuous", len(owners))
	}
}

type wireTaskDetail struct {
	Decisions []struct {
		Seq             int64  `json:"seq"`
		Type            string `json:"type"`
		RunID           string `json:"run_id"`
		Actor           string `json:"actor"`
		ActorIsOperator bool   `json:"actor_is_operator"`
		CardID          string `json:"card_id"`
		CardType        string `json:"card_type"`
		Decision        string `json:"decision"`
	} `json:"decisions"`
}

// TestTaskDecisionsAreTaskScoped is OQ5: the block covers the three subject
// kinds it declares — the task, its runs, and an EFFECT of one of its runs —
// and covers nothing else. The negative is the point: `decision.recorded`
// carries no run_id, so a matcher that was one step too loose would put every
// decision the owner ever made on every one of their task pages.
func TestTaskDecisionsAreTaskScoped(t *testing.T) {
	b := fixtureWorld(t)
	var detail wireTaskDetail
	if err := json.Unmarshal([]byte(fixtureGet(t, b, "op", "/api/tasks/t-ship")), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(detail.Decisions) != 3 {
		t.Fatalf("decisions = %d, want the effect approval (subject match), the accept (deliverables join) and the intake delta (run-scoped): %+v",
			len(detail.Decisions), detail.Decisions)
	}
	got := map[string]bool{}
	for _, d := range detail.Decisions {
		got[d.CardID] = true
		if d.CardID == "" || d.Actor == "" || d.Decision == "" {
			t.Errorf("a decision arrived without actor/card/decision: %+v", d)
		}
	}
	for _, want := range []string{"effect:e-publish", "deliverable:d-notes", "delta:delta-1"} {
		if !got[want] {
			t.Errorf("decision for %s is missing", want)
		}
	}
	if got["priority_hint:t-elsewhere"] {
		t.Error("a decision about ANOTHER task reached this task's page — the subject match is too loose")
	}

	// The three producer shapes, each proven to survive its own hazard.
	var sawRun, sawAccept, sawDeltaActor bool
	for _, d := range detail.Decisions {
		// The run-scoped producer keeps its run id.
		if d.Type == "intake.delta_decision" && d.RunID == "r-ship" {
			sawRun = true
		}
		// D1: a REAL accept is platform-scoped and names only a deliverable, so
		// it reaches this page through the deliverables join or not at all.
		if d.Type == "deliverable.accepted" && d.RunID == "" && d.Decision == "accept" && d.Actor == "alice" {
			sawAccept = true
		}
		// D2: the delta payload names no actor, so "who" is the row's owner.
		if d.Type == "intake.delta_decision" && d.Actor == "alice" {
			sawDeltaActor = true
		}
	}
	if !sawRun {
		t.Error("a run-scoped family row lost its run id")
	}
	if !sawAccept {
		t.Error("a real (platform-scoped) deliverable.accepted did not reach its task — the deliverables join is not working")
	}
	if !sawDeltaActor {
		t.Error("intake.delta_decision rendered an empty actor — the payload names none, so the row's owner is the answer")
	}

	// A task nobody has decided anything about has an empty block, not a
	// borrowed one.
	var bare wireTaskDetail
	if err := json.Unmarshal([]byte(fixtureGet(t, b, "op", "/api/tasks/t-archive")), &bare); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(bare.Decisions) != 0 {
		t.Errorf("a task with no decisions carries %d: %+v", len(bare.Decisions), bare.Decisions)
	}
}

// TestTaskDecisionsStayWithTheirOwner: bob's task detail must not surface
// alice's decisions, even though the subject matcher runs over payload text.
func TestTaskDecisionsStayWithTheirOwner(t *testing.T) {
	b := fixtureWorld(t)
	var bobs wireTaskDetail
	if err := json.Unmarshal([]byte(fixtureGet(t, b, "bob", "/api/tasks/t-audit")), &bobs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, d := range bobs.Decisions {
		if d.Actor == "alice" {
			t.Fatalf("another owner's decision reached this task: %+v", d)
		}
	}
	// And bob cannot read alice's task at all — the block rides a read that was
	// already fail-closed, which is why it needs no scope check of its own.
	rr := httptest.NewRecorder()
	fixtureServer(t, b, "bob").Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/tasks/t-ship", nil))
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusNotFound {
		t.Fatalf("a member read another owner's task detail: %d", rr.Code)
	}
}

// TestTaskDecisionsRedactAtTheEdge is drain r1 D8: the redaction call on the
// decisions block was uncovered — removing it left the suite green.
//
// Every string here is lifted out of a run_events PAYLOAD by key, and the task
// detail goes out UNWRAPPED (§7-C2·2 exempts the spec/plan/receipt content, not
// this), so the primitive has to run at this serving edge or a secret that
// reached a reason field reaches the wire. The stored row stays raw (R19).
func TestTaskDecisionsRedactAtTheEdge(t *testing.T) {
	b := fixtureWorld(t)
	// A decision whose REASON carries a secret, minted the way a real one is.
	secret := "sk-ant-api03-" + strings.Repeat("A", 40)
	exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
	            VALUES (NULL,NULL,?,?,1,?,?)`, "alice", "decision.recorded",
		`{"actor":"alice","card_id":"priority_hint:t-ship","card_type":"priority_hint","decision":"reorder",`+
			`"subject":"t-ship","reason":"pasted `+secret+` by mistake","decided_at":"2026-07-20T09:09:00Z"}`, fxT4)

	body := fixtureGet(t, b, "op", "/api/tasks/t-ship")
	if strings.Contains(body, secret) {
		t.Fatal("a payload-derived decision string reached the wire unredacted — redactTaskDecisions is not running")
	}
	// Non-tautology: the row really is on this task's page, so the assertion
	// above is about REDACTION rather than about the row being filtered out.
	var detail wireTaskDetail
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found bool
	for _, d := range detail.Decisions {
		if d.CardID == "priority_hint:t-ship" {
			found = true
		}
	}
	if !found {
		t.Fatal("the seeded decision is not on this task — the redaction assertion would pass vacuously")
	}
	// And the stored row is untouched: store-raw / serve-redacted.
	var stored string
	if err := b.db.QueryRowContext(context.Background(),
		`SELECT payload FROM run_events WHERE type = 'decision.recorded' AND payload LIKE '%by mistake%'`).Scan(&stored); err != nil {
		t.Fatalf("read stored row: %v", err)
	}
	if !strings.Contains(stored, secret) {
		t.Error("the STORED payload was rewritten — redaction belongs at the serving edge, not in the log")
	}
}

// TestOperatorCoApprovalLimbIsRecorded is drain r2 R2: the D10 co-approval limb
// (`actor_is_operator`) had no coverage after the round-1 rewrite, because the
// driven world had one person answering her own card.
//
// Two facts, and they are different facts:
//
//   - On the APPROVALS family the limb belongs to a PLATFORM-LEVEL effect —
//     one attributed to no run, which is exactly what gives the operator a
//     say (mayAnswerEffect). Both signatures are driven for real below.
//   - On a TASK page the limb arrives a different way. A platform-level effect
//     is by construction nobody's task decision (its recorded subject is the
//     card's run id, and it has none), so what a task detail can render is the
//     operator acting on their OWN task — the priority hint, whose subject is
//     the task itself.
func TestOperatorCoApprovalLimbIsRecorded(t *testing.T) {
	b := fixtureWorld(t)

	// The approvals half: two rows for one card, one per limb.
	var limbs []bool
	rows, err := b.db.QueryContext(context.Background(),
		`SELECT json_extract(payload, '$.actor_is_operator') FROM run_events
		  WHERE type = 'decision.recorded' AND json_extract(payload, '$.card_id') = 'effect:e-rotate'
		  ORDER BY event_seq`)
	if err != nil {
		t.Fatalf("read co-approval rows: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v sql.NullBool
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		limbs = append(limbs, v.Bool)
	}
	if len(limbs) != 2 {
		t.Fatalf("co-approval rows = %d, want the owner's and the operator's: %v", len(limbs), limbs)
	}
	if limbs[0] || !limbs[1] {
		t.Errorf("limbs = %v, want [owner=false, operator=true] — firstBool reads a payload BOOLEAN, and a "+
			"string read would flatten both into one", limbs)
	}

	// The task half: the operator's own hint, rendered on the operator's task.
	var detail wireTaskDetail
	if err := json.Unmarshal([]byte(fixtureGet(t, b, "op", "/api/tasks/t-ops")), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var seen bool
	for _, d := range detail.Decisions {
		if d.CardID == "priority_hint:t-ops" && d.ActorIsOperator && d.Actor == "op" {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("the operator limb is not on the operator's own task page: %+v", detail.Decisions)
	}

	// And the platform-level effect is NOT a task decision, which is the same
	// rule read from the other side rather than an accident of this fixture.
	var ship wireTaskDetail
	if err := json.Unmarshal([]byte(fixtureGet(t, b, "op", "/api/tasks/t-ship")), &ship); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, d := range ship.Decisions {
		if d.CardID == "effect:e-rotate" {
			t.Error("a platform-level effect (attributed to no run) surfaced as a task decision")
		}
	}
}

// TestDeliverablesTaskFilterIsOwnerScoped is drain r2 R4(a): the additive
// `?task=` filter narrows an already-scoped read, proven the standard three
// ways rather than asserted in a comment.
func TestDeliverablesTaskFilterIsOwnerScoped(t *testing.T) {
	b := fixtureWorld(t)
	ids := func(who, path string) []string {
		var list struct {
			Deliverables []struct {
				ID     string `json:"deliverable_id"`
				Owner  string `json:"owner"`
				TaskID string `json:"task_id"`
			} `json:"deliverables"`
		}
		if err := json.Unmarshal([]byte(fixtureGet(t, b, who, path)), &list); err != nil {
			t.Fatalf("decode %s as %s: %v", path, who, err)
		}
		out := []string{}
		for _, d := range list.Deliverables {
			if d.TaskID != "t-ship" {
				t.Errorf("?task=t-ship returned a deliverable of %s", d.TaskID)
			}
			out = append(out, d.ID)
		}
		return out
	}

	// operator: both of the task's deliverables.
	if got := ids("op", "/api/deliverables?task=t-ship"); len(got) != 2 {
		t.Errorf("operator sees %v, want both of the task's deliverables", got)
	}
	// owner: their own, present.
	if got := ids("alice", "/api/deliverables?task=t-ship"); len(got) != 2 {
		t.Errorf("the owner sees %v, want their own deliverables", got)
	}
	// another member: none — the filter narrows, it does not widen.
	if got := ids("bob", "/api/deliverables?task=t-ship"); len(got) != 0 {
		t.Errorf("another member sees %v of alice's deliverables, want none", got)
	}
}
