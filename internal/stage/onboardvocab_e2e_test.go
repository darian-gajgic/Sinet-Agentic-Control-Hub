package stage_test

// onboardvocab_e2e_test.go — P3-RW-8 acceptance on the PRODUCER side: the
// durable onboarding ask declares the answer vocabulary its own answer path
// applies, so a surface that renders controls from the card can offer the one
// verb S13.7 defines — approve (D10) — and firing it actually activates.
//
// THE DEFECT these tests pin (brief §1): `onboardCard` snapshotted exactly
// {kind, project_id, owner, draft, issued_ts}. The inbox derives a card's verbs
// from the snapshot's OWN declaration (internal/api readCardShape), that shape
// matched no declared family, so the card served `actions: null` and the SPA
// rendered its honest dead end — "this card declares no answer verbs, so there
// is nothing here to press" — on a card served answerable:true. D10's owner
// approval was reachable only by hand-crafted POST.
//
// AND THE HALF A DECLARATION ALONE WOULD NOT FIX: the SPA composes the answer
// body per the declared family, so a `decision` card composes {choice:"…"}.
// Go's json.Unmarshal ignores unknown keys, so {choice:"approve"} landed in the
// old `onboardAnswer` as approve=false and was REFUSED. Declaring the verbs
// without teaching the answer path the family's envelope would have produced a
// rendered button that always fails. Card + envelope, one producer.
//
// Committed RED per P3/briefs/P3-RW-8.md §7 (T-S1, T-S2, T-S3 and the
// end-to-end companion of T-A2 at the REAL composition — the api-package
// fixture cannot compose a stage.Skeleton, so the board move and the true
// envelope only meet here).
//
// ONE LIST, TWO READERS — TESTED AS BEHAVIOR, NOT AS A GO IDENTIFIER. T-S1's
// "the card's declared values are exactly the set AnswerOnboarding validates"
// is asserted by DRIVING every declared value through the real answer path and
// requiring acceptance, and by driving an undeclared one and requiring a
// refusal that names the declared set. That is the §43 DeltaActions property
// stated over behavior, and it stays true if the vocabulary ever grows.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

// The ONE verb S13.7 defines for this card. Declared here as the EXPECTATION,
// not imported from the producer: the point of the assertion is that the card
// the operator is served and the envelope the platform accepts agree on it.
//
// There is no decline verb, and its absence is a fact this file pins (R3):
// `AnswerOnboarding` refuses a non-approval outright and S13.7 defines no
// decline, so a declared decline button would be a control with nothing behind
// it. The exit that DOES exist is cancelling the onboarding task.
const wantOnboardVerb = "approve"

// declineWords are the verbs this card must never declare, in any spelling.
var declineWords = []string{"reject", "decline", "deny", "refuse", "cancel"}

// ── the wire shape, read the way the transport reads it ─────────────────────

// The S06.7 decision family — the shape internal/api's readCardShape derives
// verbs from (`decision.choices[].value`) and the shape the SPA composes
// {choice} for. Mirrored here off the WIRE KEYS, never off Go types, because
// the wire key is the contract (the B6-6 `id` vs `value` finding).
type onbOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type onbHelp struct {
	What      string `json:"what"`
	Wrong     string `json:"wrong"`
	Recommend string `json:"recommend"`
}

type onbDecision struct {
	Summary string      `json:"summary"`
	Detail  []string    `json:"detail"`
	Choices []onbOption `json:"choices"`
	Help    onbHelp     `json:"help"`
}

type onbCard struct {
	Kind      string          `json:"kind"`
	ProjectID string          `json:"project_id"`
	Owner     string          `json:"owner"`
	Draft     json.RawMessage `json:"draft"`
	IssuedTS  string          `json:"issued_ts"`
	Decision  *onbDecision    `json:"decision"`
}

// onboardAskSnapshot reads the durable ask's stored snapshot — the resume input
// (S02.5) and the pin source (S02.2), which is the artifact under test.
func (h *projHarness) onboardAskSnapshot(t *testing.T, askID string) string {
	t.Helper()
	var snapshot string
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT snapshot FROM asks WHERE ask_id = ?`, askID).Scan(&snapshot); err != nil {
		t.Fatalf("read snapshot of ask %s: %v", askID, err)
	}
	return snapshot
}

func (h *projHarness) onboardAskStatus(t *testing.T, askID string) string {
	t.Helper()
	var status string
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT status FROM asks WHERE ask_id = ?`, askID).Scan(&status); err != nil {
		t.Fatalf("read status of ask %s: %v", askID, err)
	}
	return status
}

func (h *projHarness) onboardRunState(t *testing.T, runID string) string {
	t.Helper()
	var state string
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT state FROM runs WHERE run_id = ?`, runID).Scan(&state); err != nil {
		t.Fatalf("read state of run %s: %v", runID, err)
	}
	return state
}

// onboardCardOf unmarshals a dispatched onboarding ask's snapshot.
func (h *projHarness) onboardCardOf(t *testing.T, askID string) onbCard {
	t.Helper()
	var c onbCard
	snapshot := h.onboardAskSnapshot(t, askID)
	if err := json.Unmarshal([]byte(snapshot), &c); err != nil {
		t.Fatalf("decode onboarding snapshot: %v: %s", err, snapshot)
	}
	return c
}

// dispatchOnboarding drives the REAL producer: StartOnboarding (register →
// clone → scan → draft) then the scheduler's dispatch, which is the ONE writer
// of this snapshot.
func (h *projHarness) dispatchOnboarding(t *testing.T, owner, projectID, name, source string) {
	t.Helper()
	ctx := context.Background()
	if _, err := h.sk.StartOnboarding(ctx, owner, projectID, name, source); err != nil {
		t.Fatalf("StartOnboarding %s: %v", projectID, err)
	}
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d runs, want the onboarding run", n)
	}
}

// onboardingFixture is a repo whose scan produces content in ALL THREE draft
// limbs — a toolchain (commands + its convention), a marker file (a second
// convention) and a hazard file (a danger zone beyond the always-on defaults) —
// so "the digest reflects the draft" is checkable rather than vacuous.
func onboardingFixture(t *testing.T) string {
	t.Helper()
	return gitFixture(t, map[string]string{
		"go.mod":          "module shop\n\ngo 1.24\n",
		"CONTRIBUTING.md": "Small commits, tests first.\n",
		".env":            "SECRET=1\n",
	})
}

// ── T-S1: the snapshot round-trips AND declares its vocabulary ──────────────

// TestOnboardCardDeclaresItsAnswerVocabulary — brief T-S1, checklist 1. The
// dispatched snapshot keeps every top-level field the answer path and the audit
// read, AND carries the S06.7 decision body the inbox derives verbs from: one
// choice, the 13.5 help block, and a plain digest of what is being approved.
func TestOnboardCardDeclaresItsAnswerVocabulary(t *testing.T) {
	h := newProjectHarness(t)
	const owner = "alice"
	h.dispatchOnboarding(t, owner, "shop", "Shop backend", onboardingFixture(t))
	askID := stage.OnboardAskID("shop")
	snapshot := h.onboardAskSnapshot(t, askID)
	card := h.onboardCardOf(t, askID)

	// (a) Round-trip: the snapshot is the resume input, so nothing it already
	// carried may be displaced by the declaration.
	if card.Kind != "onboard" {
		t.Errorf("snapshot kind = %q, want onboard", card.Kind)
	}
	if card.ProjectID != "shop" || card.Owner != owner {
		t.Errorf("snapshot names project %q owner %q, want shop/%s", card.ProjectID, card.Owner, owner)
	}
	if card.IssuedTS == "" {
		t.Errorf("snapshot carries no issued_ts: %s", snapshot)
	}
	var draft project.Draft
	if len(card.Draft) == 0 {
		t.Fatalf("snapshot carries no top-level draft — the answer path and the audit read it: %s", snapshot)
	}
	if err := json.Unmarshal(card.Draft, &draft); err != nil {
		t.Fatalf("decode the snapshot's draft: %v: %s", err, card.Draft)
	}
	if len(draft.Conventions) == 0 || draft.Commands.Build == "" || len(draft.DangerZones) == 0 {
		t.Fatalf("the fixture's draft is empty in a limb the digest assertions need: %+v", draft)
	}

	// (b) The declaration itself — the family the inbox reads and the SPA fires.
	if card.Decision == nil {
		t.Fatalf("the onboarding card declares NO answer vocabulary: a card served answerable to its owner with no "+
			"verbs renders as a dead end and D10's approval is reachable only by hand-crafted POST (R1): %s", snapshot)
	}
	d := card.Decision
	if len(d.Choices) != 1 || d.Choices[0].Value != wantOnboardVerb {
		t.Errorf("declared choices = %+v, want exactly one with value %q — approve is the ONE verb S13.7 defines and "+
			"the only one AnswerOnboarding applies (R1/R3)", d.Choices, wantOnboardVerb)
	}
	for _, ch := range d.Choices {
		if ch.Label == "" {
			t.Errorf("choice %q carries no label — a surface would draw an unlabelled button", ch.Value)
		}
		for _, w := range declineWords {
			if strings.Contains(strings.ToLower(ch.Value), w) {
				t.Errorf("the card declares %q, a decline-class verb: no server behavior exists behind one "+
					"(AnswerOnboarding refuses a non-approval) and S13.7 defines none (R3)", ch.Value)
			}
		}
	}

	// (c) The owner sees WHAT they approve (R4/D10), in operator language.
	if d.Summary == "" {
		t.Errorf("the card carries no summary — the person deciding is shown no statement of what this is")
	}
	if !strings.Contains(d.Summary, "shop") {
		t.Errorf("summary %q does not name the project it activates", d.Summary)
	}
	if len(d.Detail) == 0 {
		t.Fatalf("the card carries no detail digest: the draft rides the snapshot as an unrendered blob, so the " +
			"rendered card shows nothing of what is being approved (R4)")
	}
	joined := strings.Join(d.Detail, "\n")
	for what, want := range map[string]string{
		"a convention line": "CONTRIBUTING",
		"a command entry":   draft.Commands.Build,
		"a danger zone":     ".env",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the digest shows no %s (%q missing) — it must reflect the draft's conventions, commands and "+
				"danger zones (R4):\n%s", what, want, joined)
		}
	}

	// (d) The 13.5 help block, all three sentences, with the honest exit named.
	if d.Help.What == "" || d.Help.Wrong == "" || d.Help.Recommend == "" {
		t.Errorf("the 13.5 help block is incomplete: %+v — S15.6 cards carry what this does, what could go wrong, "+
			"and what the platform recommends", d.Help)
	}
	help := strings.ToLower(d.Help.What + " " + d.Help.Wrong + " " + d.Help.Recommend)
	if !strings.Contains(help, "cancel") {
		t.Errorf("the help block never names the way to NOT onboard. There is no decline verb, so the honest card "+
			"says the exit that exists — cancelling the onboarding task (R3): %+v", d.Help)
	}

	// (e) No tier key: the untiered onboarding card reads Medium by the
	// transport's own ratified default (R6). Declaring one would be a behavior
	// change this packet does not own.
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(snapshot), &top); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if _, ok := top["tier"]; ok {
		t.Errorf("the card declares a tier — the onboarding card is untiered by ratification (R6): %s", snapshot)
	}
}

// ── T-S2: the envelope limb ─────────────────────────────────────────────────

// TestOnboardAnswerEnvelope — brief T-S2, checklist 2/3. The answer path
// accepts the envelope the declared family COMPOSES, keeps the landed raw door
// verbatim, and refuses everything else by naming the vocabulary rather than
// resolving it silently.
func TestOnboardAnswerEnvelope(t *testing.T) {
	const owner = "alice"
	askID := stage.OnboardAskID("shop")
	runID := stage.OnboardTaskID("shop") + stage.RunSuffixOnboard

	// dispatched builds a fresh world parked on an open onboarding ask.
	dispatched := func(t *testing.T) *projHarness {
		t.Helper()
		h := newProjectHarness(t)
		h.dispatchOnboarding(t, owner, "shop", "Shop backend", onboardingFixture(t))
		return h
	}
	// answered asserts the whole approval landed: entry active, run completed,
	// task Done — the S13.7 choreography the envelope must reach.
	answered := func(t *testing.T, h *projHarness) {
		t.Helper()
		e, err := h.proj.Get(context.Background(), "shop")
		if err != nil || !e.Active() {
			t.Fatalf("entry after approval = %+v err=%v, want active", e, err)
		}
		if got := h.onboardRunState(t, runID); got != "completed" {
			t.Errorf("onboarding run = %q, want completed", got)
		}
		if got := h.storedKanban(t, stage.OnboardTaskID("shop")); got != boardApprovedColumn {
			t.Errorf("onboarding task column = %q, want %q", got, boardApprovedColumn)
		}
		if got := h.onboardAskStatus(t, askID); got != "answered" {
			t.Errorf("ask status = %q, want answered", got)
		}
	}
	// refused asserts a refusal changed NOTHING — the card is still there to
	// answer properly, which is what "never silently resolved" means.
	refused := func(t *testing.T, h *projHarness, err error, names ...string) {
		t.Helper()
		if err == nil {
			t.Fatalf("the answer was ACCEPTED; want a refusal")
		}
		for _, want := range names {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal %q does not name %q — a refusal that does not say what this card accepts "+
					"leaves the answerer guessing (R2)", err, want)
			}
		}
		e, gerr := h.proj.Get(context.Background(), "shop")
		if gerr != nil || e.Active() {
			t.Errorf("a refused answer activated the entry anyway: %+v %v", e, gerr)
		}
		if got := h.onboardAskStatus(t, askID); got != "open" {
			t.Errorf("after a refusal the ask is %q, want open — the card must stay answerable", got)
		}
	}

	// The SPA's own composed body: `decision` → {choice:"<value>"}.
	t.Run("the declared choice fires the approval", func(t *testing.T) {
		h := dispatched(t)
		if _, err := h.sur.Answer(context.Background(), owner, askID,
			json.RawMessage(fmt.Sprintf(`{"choice":%q}`, wantOnboardVerb)), false); err != nil {
			t.Fatalf("the card's OWN declared verb was refused by its own answer path: %v", err)
		}
		answered(t, h)
	})

	// CriteriaPicker may append the card's detail lines; the answer path
	// tolerates them and applies none (R2).
	t.Run("criteria are tolerated and ignored", func(t *testing.T) {
		h := dispatched(t)
		if _, err := h.sur.Answer(context.Background(), owner, askID,
			json.RawMessage(fmt.Sprintf(`{"choice":%q,"criteria":["Danger zone: .env"]}`, wantOnboardVerb)), false); err != nil {
			t.Fatalf("a choice carrying criteria was refused: %v", err)
		}
		answered(t, h)
	})

	// The landed raw door, verbatim (R2) — the builder-proven POST.
	t.Run("the landed approve envelope still works", func(t *testing.T) {
		h := dispatched(t)
		if _, err := h.sur.Answer(context.Background(), owner, askID, json.RawMessage(`{"approve":true}`), false); err != nil {
			t.Fatalf("{approve:true}: %v", err)
		}
		answered(t, h)
	})

	// R5: edit-at-approval stays the API-only door, and it still edits.
	t.Run("the edited draft still reaches the store", func(t *testing.T) {
		h := dispatched(t)
		const edited = `{"conventions":["Owner's own convention"],"commands":{"build":"make shop"}}`
		if _, err := h.sur.Answer(context.Background(), owner, askID,
			json.RawMessage(`{"approve":true,"draft":`+edited+`}`), false); err != nil {
			t.Fatalf("{approve:true,draft:…}: %v", err)
		}
		answered(t, h)
		e, err := h.proj.Get(context.Background(), "shop")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if strings.Join(e.Capture.Conventions, "|") != "Owner's own convention" || e.Capture.Commands.Build != "make shop" {
			t.Errorf("the owner's edited draft did not reach the store: %+v", e.Capture)
		}
	})

	// An undeclared verb is refused BY NAME, and the refusal names what the
	// card does declare (R2) — the one-list-two-readers property from the
	// answer side.
	t.Run("an undeclared choice is refused naming the vocabulary", func(t *testing.T) {
		h := dispatched(t)
		_, err := h.sur.Answer(context.Background(), owner, askID, json.RawMessage(`{"choice":"reject"}`), false)
		refused(t, h, err, "reject", wantOnboardVerb)
	})

	t.Run("an empty choice is refused naming the vocabulary", func(t *testing.T) {
		h := dispatched(t)
		_, err := h.sur.Answer(context.Background(), owner, askID, json.RawMessage(`{"choice":""}`), false)
		refused(t, h, err, wantOnboardVerb)
	})

	// The landed refusal wording family, unchanged: there is no decline.
	t.Run("approve false is still refused", func(t *testing.T) {
		h := dispatched(t)
		_, err := h.sur.Answer(context.Background(), owner, askID, json.RawMessage(`{"approve":false}`), false)
		refused(t, h, err, "approve")
	})

	t.Run("an empty answer object is refused", func(t *testing.T) {
		h := dispatched(t)
		_, err := h.sur.Answer(context.Background(), owner, askID, json.RawMessage(`{}`), false)
		refused(t, h, err, wantOnboardVerb)
	})

	// R2's precedence rule: a contradiction is REFUSED, never resolved. Picking
	// a winner here would apply a decision the person did not make.
	t.Run("a contradictory answer is refused, never resolved", func(t *testing.T) {
		h := dispatched(t)
		_, err := h.sur.Answer(context.Background(), owner, askID,
			json.RawMessage(fmt.Sprintf(`{"choice":%q,"approve":false}`, wantOnboardVerb)), false)
		refused(t, h, err, wantOnboardVerb)
	})

	// D10, byte-identical through the new envelope: the owner answers.
	t.Run("a non-owner is refused through the new envelope too", func(t *testing.T) {
		h := dispatched(t)
		_, err := h.sur.Answer(context.Background(), "bob", askID,
			json.RawMessage(fmt.Sprintf(`{"choice":%q}`, wantOnboardVerb)), false)
		refused(t, h, err, "owner")
	})
}

// ── T-S3: the pre-packet snapshot still answers ─────────────────────────────

// TestOnboardOldSnapshotStillAnswers — brief T-S3, checklist 4 (R8). A card
// issued BEFORE this packet carries no declaration. It must degrade to exactly
// today's behavior — verb-less but answerable — never break.
//
// THE OQ5 FORK, ANSWERED AND PINNED HERE: `choice` is validated against the
// PACKAGE's vocabulary, not against the stored snapshot's declaration. So an
// old-shape card answers IDENTICALLY to a new one, the stored declaration stays
// presentation, and there is no third behavior to reason about. The rejected
// alternative (validate against the snapshot's own list) would have made an old
// card refuse the very envelope the SPA composes for a card it renders.
func TestOnboardOldSnapshotStillAnswers(t *testing.T) {
	const owner = "alice"
	// The PRE-packet snapshot literal, exactly the shape the landed api-side
	// backward-compat fence seeds (projects_test.go).
	oldShape := func(projectID string) string {
		return fmt.Sprintf(`{"kind":"onboard","project_id":%q,"owner":%q,"draft":{},"issued_ts":"2026-08-01T00:00:00Z"}`,
			projectID, owner)
	}
	downgrade := func(t *testing.T, h *projHarness, projectID string) {
		t.Helper()
		snapshot := oldShape(projectID)
		if err := h.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
			_, err := tx.ExecContext(context.Background(),
				`UPDATE asks SET snapshot = ? WHERE ask_id = ?`, snapshot, stage.OnboardAskID(projectID))
			return err
		}); err != nil {
			t.Fatalf("plant the pre-packet snapshot: %v", err)
		}
	}

	t.Run("the landed raw envelope still activates", func(t *testing.T) {
		h := newProjectHarness(t)
		h.dispatchOnboarding(t, owner, "shop", "Shop backend", onboardingFixture(t))
		downgrade(t, h, "shop")
		if _, err := h.sur.Answer(context.Background(), owner, stage.OnboardAskID("shop"),
			json.RawMessage(`{"approve":true}`), false); err != nil {
			t.Fatalf("an old-shape card must still answer {approve:true}: %v", err)
		}
		e, err := h.proj.Get(context.Background(), "shop")
		if err != nil || !e.Active() {
			t.Fatalf("entry after approval = %+v err=%v, want active", e, err)
		}
	})

	t.Run("the choice envelope answers identically", func(t *testing.T) {
		h := newProjectHarness(t)
		h.dispatchOnboarding(t, owner, "yard", "Yard service", onboardingFixture(t))
		downgrade(t, h, "yard")
		if _, err := h.sur.Answer(context.Background(), owner, stage.OnboardAskID("yard"),
			json.RawMessage(fmt.Sprintf(`{"choice":%q}`, wantOnboardVerb)), false); err != nil {
			t.Fatalf("OQ5: `choice` is validated against the PACKAGE vocabulary, so an old snapshot answers exactly "+
				"as a new one does: %v", err)
		}
		e, err := h.proj.Get(context.Background(), "yard")
		if err != nil || !e.Active() {
			t.Fatalf("entry after approval = %+v err=%v, want active", e, err)
		}
	})
}

// ── the end-to-end companion of T-A2, at the REAL composition ───────────────

// TestOnboardApproveFromTheServedCardEndToEnd — brief §6 checklist 1+2 driven
// end to end over the production composition (the real create door, the real
// scheduler dispatch, the real durable ask, the real stage Surface behind
// POST /api/approvals/{id}/answer, the real project store and the real board).
//
// WHY IT LIVES IN THIS PACKAGE: internal/api's projects fixture cannot compose a
// stage.Skeleton, so its answer seam approves regardless of the body and never
// moves the board. The exact body the SPA composes and the S13.7 choreography it
// must reach therefore only meet HERE, over the composition the shell builds.
func TestOnboardApproveFromTheServedCardEndToEnd(t *testing.T) {
	h := newProjectHarness(t)
	ctx := context.Background()
	const owner = "alice"
	taskID := h.createProject(t, owner, "shop", "Shop backend")
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the onboarding run", n)
	}
	routeID := "ask:" + stage.OnboardAskID("shop")

	// What the inbox serves the owner, and what the SPA renders from.
	served := h.servedApproval(t, owner, routeID)
	if !served.Answerable {
		t.Errorf("the owner's own onboarding card is not answerable")
	}
	if strings.Join(served.Actions, "|") != wantOnboardVerb {
		t.Fatalf("served actions = %v, want exactly [%q] — the SPA renders buttons strictly from this list, so a nil "+
			"list is the rendered dead end this packet removes", served.Actions, wantOnboardVerb)
	}
	if served.Tier != "medium" || served.Batchable || served.StepUpRequired {
		t.Errorf("served tier/batchable/step_up = %q/%v/%v, want medium/false/false (R6)",
			served.Tier, served.Batchable, served.StepUpRequired)
	}
	if served.PayloadHash == "" {
		t.Fatalf("the served card carries no payload_hash to pin an answer to")
	}

	// The EXACT body the SPA composes for a `decision` card, with the served pin.
	body := fmt.Sprintf(`{"payload_hash":%q,"answer":{"choice":%q}}`, served.PayloadHash, wantOnboardVerb)
	code, out := h.call(t, owner, "POST", "/api/approvals/"+routeID+"/answer", body)
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
	e, err := h.proj.Get(ctx, "shop")
	if err != nil || !e.Active() {
		t.Fatalf("entry after the served answer = %+v err=%v, want active", e, err)
	}
	if got := h.listBoard(t, owner, "")[taskID].KanbanStatus; got != boardApprovedColumn {
		t.Errorf("task column after approval = %q, want %q", got, boardApprovedColumn)
	}

	// Retry-safety (S15.2): the identical POST re-applies nothing.
	code, out = h.call(t, owner, "POST", "/api/approvals/"+routeID+"/answer", body)
	if code != http.StatusOK {
		t.Fatalf("the retry: %d %s", code, out)
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode retry result: %v: %s", err, out)
	}
	if res.Applied {
		t.Errorf("the retry re-applied the answer: %s", out)
	}
}

// servedApprovalItem is the slice of the inbox item this file reads.
type servedApprovalItem struct {
	ID             string   `json:"id"`
	Tier           string   `json:"tier"`
	Answerable     bool     `json:"answerable"`
	Batchable      bool     `json:"batchable"`
	StepUpRequired bool     `json:"step_up_required"`
	PayloadHash    string   `json:"payload_hash"`
	Actions        []string `json:"actions"`
}

func (h *projHarness) servedApproval(t *testing.T, who, id string) servedApprovalItem {
	t.Helper()
	var list struct {
		Items []servedApprovalItem `json:"items"`
	}
	out := h.callOK(t, who, "GET", "/api/approvals", "")
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("decode inbox: %v: %s", err, out)
	}
	for _, it := range list.Items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("%s is not in the inbox: %s", id, out)
	return servedApprovalItem{}
}
