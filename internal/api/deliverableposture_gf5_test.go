package api_test

// deliverableposture_gf5_test.go — P3-GF5 R9: the machine-readable
// verification posture on the deliverable detail read.
//
// COMMITTED RED (CONVENTIONS §3 Amendment-A): `DeliverableDetail` carries no
// `verification` member yet, so the bootstrap direction fails against the
// pre-GF5 handler; the packet's implementation commit closes the window. New
// file, because the packet may not modify pre-existing test files.
//
// WHY THE WIRE NEEDS IT (S07.8 [A14] "the verdict card … names the bootstrap
// posture"; the GF2-ratified rule that every answerable card carries its real
// door). The posture, its plain-words note and the mandatory-review obligation
// live on the verdict EVENT row and on the receipt as prose. A surface that
// wanted to render the Commands door on a bootstrap revision could therefore
// only string-match prose or the raw `check-pack:absent` token — the fragile
// jargon-coupling §38's ban-on-matching-text condemns. This member is the
// closed-vocabulary structural fact instead: a posture string and a boolean,
// no note text, which is §38 ruling (b)'s exempt shape and opens no new
// redaction edge.
//
// KEY ABSENCE, NOT ZERO VALUES (the GF4 drain-F3c lesson). The ordinary
// posture must not serve `"verification":{"posture":"","review_mandatory":false}`
// — a client cannot tell that apart from a posture it does not know. The
// assertion below is over the RAW served JSON for that reason.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// gf5RecordRound appends a REAL verify.round verdict row through the landed
// recorder (§42-B: producer-minted, never author-imagined) and pins it to the
// revision the way the stage's review sink does.
func gf5RecordRound(t *testing.T, e *dlvEnv, deliverableID, taskID, runID string, n int, rec verify.RoundRecord) int64 {
	t.Helper()
	rc := &verify.Recorder{DB: e.b.db, Log: e.b.log}
	seq, err := rc.RecordRound(e.ctx, runID,
		verify.Deliverable{TaskID: taskID, RunID: runID, Domain: verify.DomainSoftware, Revision: n},
		rec, verify.JudgeMeta{Model: "gf5-judge-1", SelfFamily: true}, verify.GoldenSetRates{},
		"rubric-software", 1, []string{"AC-1"}, "idle")
	if err != nil {
		t.Fatalf("RecordRound: %v", err)
	}
	if err := e.rev.SetVerdictRef(e.ctx, deliverableID, n, seq); err != nil {
		t.Fatalf("SetVerdictRef: %v", err)
	}
	return seq
}

// TestGF5DeliverableDetailServesThePostureBothDirections [brief T8]: a
// bootstrap revision's detail carries `verification.posture == "bootstrap"` and
// `review_mandatory: true`; an ordinary revision's raw JSON does not contain
// the `"verification"` key at all.
func TestGF5DeliverableDetailServesThePostureBothDirections(t *testing.T) {
	e := newDlvEnv(t)

	e.mkRun("t-boot", "r-boot", "alice")
	e.mkDeliverable("d-boot", "alice", "t-boot", "r-boot", "p-boot", "code",
		map[string]string{"main.go": "package main\n"}, "")
	gf5RecordRound(t, e, "d-boot", "t-boot", "r-boot", 1, verify.RoundRecord{
		Round: 1, Verdict: verify.VerdictShip, Revision: 1,
		Posture:         verify.PostureBootstrap,
		PostureNote:     verify.BootstrapPostureNote,
		ReviewMandatory: true,
		ContentSHA:      "sha-boot",
	})

	e.mkRun("t-full", "r-full", "alice")
	e.mkDeliverable("d-full", "alice", "t-full", "r-full", "p-full", "code",
		map[string]string{"main.go": "package main // full\n"}, "")
	gf5RecordRound(t, e, "d-full", "t-full", "r-full", 1, verify.RoundRecord{
		Round: 1, Verdict: verify.VerdictShip, Revision: 1, ContentSHA: "sha-full",
	})

	// ── the bootstrap direction ──────────────────────────────────────────
	code, body := e.do(t, "alice", http.MethodGet, "/api/deliverables/d-boot", "")
	if code != http.StatusOK {
		t.Fatalf("bootstrap detail = %d; body: %s", code, body)
	}
	var boot struct {
		Verification *struct {
			Posture         string `json:"posture"`
			ReviewMandatory bool   `json:"review_mandatory"`
		} `json:"verification"`
	}
	if err := json.Unmarshal([]byte(body), &boot); err != nil {
		t.Fatalf("decode bootstrap detail: %v: %s", err, body)
	}
	if boot.Verification == nil {
		t.Fatalf("the bootstrap revision's detail carries no `verification` member — GF6 can only render the Commands door by string-matching prose; body: %s", body)
	}
	if boot.Verification.Posture != string(verify.PostureBootstrap) {
		t.Errorf("verification.posture = %q, want %q", boot.Verification.Posture, verify.PostureBootstrap)
	}
	if !boot.Verification.ReviewMandatory {
		t.Errorf("verification.review_mandatory = false — S07.8 makes requester review the real gate for an advisory verdict")
	}
	// The NOTE text stays off the wire: the plain-words disclosure already
	// reaches the review comments and the receipt, and a closed-vocabulary
	// structural field is what keeps this member out of the redaction edge.
	if strings.Contains(body, "restores the full ladder") {
		t.Errorf("the detail carries the posture NOTE prose; the member is a closed vocabulary, not a second copy of the disclosure: %s", body)
	}

	// ── the ordinary direction: the KEY is absent, not zero-valued ───────
	code, body = e.do(t, "alice", http.MethodGet, "/api/deliverables/d-full", "")
	if code != http.StatusOK {
		t.Fatalf("ordinary detail = %d; body: %s", code, body)
	}
	if strings.Contains(body, `"verification"`) {
		t.Errorf("an ordinary revision's detail contains the `verification` key; absence is the answer, and a zero-valued member is indistinguishable from a posture the client does not know: %s", body)
	}
}

// TestGF5PostureIsReadFromTheCurrentRevisionsVerdictRow [R9]: the member is a
// fact about the CURRENT revision, not about the deliverable's history. A
// deliverable whose first round was advisory and whose second ran the real
// ladder serves NO posture — which is exactly the escape r4-F1b buys, visible
// on the wire.
func TestGF5PostureIsReadFromTheCurrentRevisionsVerdictRow(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-esc", "r-esc", "alice")
	e.mkDeliverable("d-esc", "alice", "t-esc", "r-esc", "p-esc", "code",
		map[string]string{"main.go": "package main\n"}, "")
	gf5RecordRound(t, e, "d-esc", "t-esc", "r-esc", 1, verify.RoundRecord{
		Round: 1, Verdict: verify.VerdictShip, Revision: 1,
		Posture: verify.PostureBootstrap, PostureNote: verify.BootstrapPostureNote,
		ReviewMandatory: true, ContentSHA: "sha-1",
	})
	if code, body := e.do(t, "alice", http.MethodGet, "/api/deliverables/d-esc", ""); code != http.StatusOK ||
		!strings.Contains(body, `"posture"`) {
		t.Fatalf("revision 1 does not serve the posture (%d): %s", code, body)
	}

	// The owner captures commands; the next round runs the real ladder.
	e.mintNext("d-esc", "r-esc", 2, map[string]string{"main.go": "package main // v2\n"}, "")
	gf5RecordRound(t, e, "d-esc", "t-esc", "r-esc", 2, verify.RoundRecord{
		Round: 2, Verdict: verify.VerdictShip, Revision: 2, ContentSHA: "sha-2",
	})

	code, body := e.do(t, "alice", http.MethodGet, "/api/deliverables/d-esc", "")
	if code != http.StatusOK {
		t.Fatalf("detail = %d; body: %s", code, body)
	}
	var out struct {
		Deliverable review.Deliverable `json:"deliverable"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Deliverable.CurrentRevision != 2 {
		t.Fatalf("current revision = %d, want 2", out.Deliverable.CurrentRevision)
	}
	if strings.Contains(body, `"verification"`) {
		t.Errorf("the detail still carries a posture after the escape round — the member must be read from the CURRENT revision's verdict row: %s", body)
	}
}
