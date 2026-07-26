package watchlist_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchdog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// findings reads every emitted drift.finding payload from the log.
func (h *harness) findings(t *testing.T) []watchlist.FindingPayload {
	t.Helper()
	rows, err := h.db.QueryContext(context.Background(),
		`SELECT payload, run_id, user_id, schema_version FROM run_events WHERE type = ? ORDER BY event_seq`,
		watchlist.EventDriftFinding)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []watchlist.FindingPayload
	for rows.Next() {
		var payload, user string
		var runID any
		var sv int
		if err := rows.Scan(&payload, &runID, &user, &sv); err != nil {
			t.Fatal(err)
		}
		if runID != nil {
			t.Errorf("a drift finding carries run_id %v — a watch hit has no run (platform scope)", runID)
		}
		if user != "platform" {
			t.Errorf("a drift finding is owned by %q, want platform", user)
		}
		if sv != 1 {
			t.Errorf("drift finding schema_version = %d, want 1", sv)
		}
		var p watchlist.FindingPayload
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

// TestFindingSatisfiesTheFamilyContractMinimum (acceptance item 5): the payload
// carries source, lane(s), change class, severity, one-line summary and the
// incident fingerprint — the S14.2 FamilyDriftCanary descriptor verbatim.
func TestFindingSatisfiesTheFamilyContractMinimum(t *testing.T) {
	h := newHarness(t)
	got, err := h.em.Emit(context.Background(), watchlist.Hit{
		RowID: "t1-anthropic-pricing", Kind: watchlist.KindPage,
		Source: "https://example.invalid/pricing", Lane: "anthropic",
		Class: watchlist.ClassPrice, Summary: "input price moved",
		Detail: "- $3\n+ $4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source == "" || len(got.Lanes) == 0 || got.ChangeClass == "" ||
		got.Severity == "" || got.Summary == "" || got.Fingerprint == "" {
		t.Fatalf("payload misses a contract-minimum field: %+v", got)
	}
	if got.Severity != watchlist.SeverityFlagNow {
		t.Errorf("a price change on an ACTIVE lane must be flag-now, got %q", got.Severity)
	}
	if len(h.findings(t)) != 1 {
		t.Error("the finding was not appended")
	}
}

// TestSeverityDerivesFromClassAndRowLane (S14.4): flag-now exactly for an active
// lane's price/terms/limits/models/endpoints, and for ANY billing-regime change;
// daily digest otherwise. A watch-only CANDIDATE is not an active lane.
func TestSeverityDerivesFromClassAndRowLane(t *testing.T) {
	h := newHarness(t)
	cases := []struct {
		class, lane, want string
	}{
		{watchlist.ClassPrice, "anthropic", watchlist.SeverityFlagNow},
		{watchlist.ClassLimits, "zai", watchlist.SeverityFlagNow},
		{watchlist.ClassModels, "local", watchlist.SeverityFlagNow},
		{watchlist.ClassEndpoints, "anthropic", watchlist.SeverityFlagNow},
		{watchlist.ClassTerms, "anthropic", watchlist.SeverityFlagNow},
		{watchlist.ClassPrice, "cerebras", watchlist.SeverityDigest},
		{watchlist.ClassNone, "anthropic", watchlist.SeverityDigest},
		{watchlist.ClassUnclear, "anthropic", watchlist.SeverityDigest},
		// A billing-regime change is load-bearing even on a candidate.
		{watchlist.ClassBillingRegime, "cerebras", watchlist.SeverityFlagNow},
	}
	for _, c := range cases {
		got, err := h.em.Emit(context.Background(), watchlist.Hit{
			RowID: "r", Kind: watchlist.KindPage, Source: "https://x.invalid/" + c.class,
			Lane: c.lane, Class: c.class, Summary: "s",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.Severity != c.want {
			t.Errorf("class %s on lane %s → %s, want %s", c.class, c.lane, got.Severity, c.want)
		}
	}
}

// TestFingerprintIsStableOverSourceLaneClass (R17).
func TestFingerprintIsStableOverSourceLaneClass(t *testing.T) {
	a := watchlist.Fingerprint("https://x.invalid/p", "anthropic", watchlist.ClassPrice)
	if a != watchlist.Fingerprint("https://x.invalid/p", "anthropic", watchlist.ClassPrice) {
		t.Error("the fingerprint is not stable")
	}
	for _, other := range []string{
		watchlist.Fingerprint("https://y.invalid/p", "anthropic", watchlist.ClassPrice),
		watchlist.Fingerprint("https://x.invalid/p", "zai", watchlist.ClassPrice),
		watchlist.Fingerprint("https://x.invalid/p", "anthropic", watchlist.ClassLimits),
	} {
		if a == other {
			t.Error("fingerprints collide across {source, lane, change class}")
		}
	}
}

// TestRevalidationEdgeMatchesOQ4 is acceptance item 26 and the packet's central
// honesty check: the S14.8 runbook fires ONLY for change class `models` carrying
// a model-id subject. Every other class must NOT call Flag, and must record on
// the card that no revalidation was triggered.
func TestRevalidationEdgeMatchesOQ4(t *testing.T) {
	h := newHarness(t)
	var calls []string
	h.em.Revalidate = func(ctx context.Context, modelID, reason string) ([]string, error) {
		calls = append(calls, modelID)
		return []string{"tmpl-1"}, nil
	}

	// (i) models + a model-id subject: the ONE case that fires.
	got, err := h.em.Emit(context.Background(), watchlist.Hit{
		RowID: "r", Kind: watchlist.KindAPI, Source: "https://x.invalid/api.json",
		Lane: "anthropic", Class: watchlist.ClassModels, Subject: "claude-haiku-4-5",
		Summary: "model list moved",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "claude-haiku-4-5" {
		t.Fatalf("the runbook was called %v, want exactly [claude-haiku-4-5]", calls)
	}
	if !got.Revalidation.Triggered || got.Revalidation.Subject != "claude-haiku-4-5" {
		t.Errorf("revalidation not recorded as triggered: %+v", got.Revalidation)
	}
	if len(got.Revalidation.Flagged) != 1 {
		t.Errorf("the flagged set was not recorded: %+v", got.Revalidation)
	}

	// (ii) every OTHER class must not call Flag, and must SAY so on the card.
	calls = nil
	for _, class := range []string{
		watchlist.ClassPrice, watchlist.ClassTerms, watchlist.ClassLimits,
		watchlist.ClassEndpoints, watchlist.ClassBillingRegime,
		watchlist.ClassNone, watchlist.ClassUnclear,
	} {
		got, err := h.em.Emit(context.Background(), watchlist.Hit{
			RowID: "r", Kind: watchlist.KindPage, Source: "https://x.invalid/" + class,
			Lane: "anthropic", Class: class, Subject: "claude-haiku-4-5", Summary: "s",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.Revalidation.Triggered {
			t.Errorf("class %s triggered revalidation — only `models` may", class)
		}
		if got.Revalidation.Reason == "" {
			t.Errorf("class %s recorded no reason — a non-triggering class must be VISIBLY recorded, never silently dropped", class)
		}
	}
	if len(calls) != 0 {
		t.Errorf("non-model classes called the runbook %v — they must not reach the hook at all", calls)
	}

	// (iii) models WITHOUT a model-id subject: no call, and the gap is stated.
	got, err = h.em.Emit(context.Background(), watchlist.Hit{
		RowID: "r", Kind: watchlist.KindAPI, Source: "https://x.invalid/nosubject",
		Lane: "anthropic", Class: watchlist.ClassModels, Summary: "s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Revalidation.Triggered || len(calls) != 0 {
		t.Error("a models-class finding with no model-id subject fired the runbook — it would flag nothing")
	}
	if got.Revalidation.Reason == "" {
		t.Error("the missing-subject case recorded no reason")
	}
}

// TestRevalidationFailureNeverSuppressesTheCard: a runbook error is recorded and
// the finding still lands.
func TestRevalidationFailureNeverSuppressesTheCard(t *testing.T) {
	h := newHarness(t)
	h.em.Revalidate = func(ctx context.Context, modelID, reason string) ([]string, error) {
		return nil, errors.New("worker store unavailable")
	}
	got, err := h.em.Emit(context.Background(), watchlist.Hit{
		RowID: "r", Kind: watchlist.KindAPI, Source: "https://x.invalid/a", Lane: "anthropic",
		Class: watchlist.ClassModels, Subject: "m1", Summary: "s",
	})
	if err != nil {
		t.Fatalf("a runbook failure must not fail the emit: %v", err)
	}
	if got.Revalidation.Error == "" || got.Revalidation.Triggered {
		t.Errorf("the failure was not recorded honestly: %+v", got.Revalidation)
	}
	if len(h.findings(t)) != 1 {
		t.Error("the finding was suppressed by a runbook failure")
	}
}

// TestUnwiredRevalidationSeamIsRecorded: with no seam composed, the card says so
// rather than implying the edge ran.
func TestUnwiredRevalidationSeamIsRecorded(t *testing.T) {
	h := newHarness(t)
	got, err := h.em.Emit(context.Background(), watchlist.Hit{
		RowID: "r", Kind: watchlist.KindAPI, Source: "https://x.invalid/a", Lane: "anthropic",
		Class: watchlist.ClassModels, Subject: "m1", Summary: "s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Revalidation.Triggered {
		t.Error("an unwired seam reported a triggered revalidation")
	}
	if got.Revalidation.Reason == "" {
		t.Error("an unwired seam recorded no reason")
	}
}

// TestUnclassifiedHitStillBecomesACard (R15): with no local tier the hit is
// recorded UNCLASSIFIED — honestly marked, never dropped and never labelled.
func TestUnclassifiedHitStillBecomesACard(t *testing.T) {
	h := newHarness(t)
	h.em.Classifier = &watchlist.Classifier{} // no Duty, no Meter
	got, err := h.em.Emit(context.Background(), watchlist.Hit{
		RowID: "f1", Kind: watchlist.KindFeed, Source: "https://x.invalid/feed",
		Lane: "anthropic", Title: "v2.4.0 released", Detail: "notes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Classified {
		t.Error("a hit with no local tier was marked classified")
	}
	if got.ChangeClass != watchlist.ClassUnclear {
		t.Errorf("change class = %q, want the abstain member %q — a label is never fabricated", got.ChangeClass, watchlist.ClassUnclear)
	}
	if got.ClassifierNote == "" {
		t.Error("the reason the pass did not run was not recorded")
	}
	if got.Summary != "v2.4.0 released" {
		t.Errorf("the fallback summary must be the platform-authored observation, got %q", got.Summary)
	}
	if len(h.findings(t)) != 1 {
		t.Error("an unclassified hit did not become a card")
	}
}

// TestDriftFindingIsMintedAndDistinctFromRegistryDrift (acceptance item 4): the
// type is minted in the S14.2 contract under Drift & canary, and registry.drift
// (project-topology drift, Platform family) is NOT conflated with it.
func TestDriftFindingIsMintedAndDistinctFromRegistryDrift(t *testing.T) {
	reg := eventlog.Registry()
	ts, ok := reg.TypeSpec(watchlist.EventDriftFinding)
	if !ok {
		t.Fatal("drift.finding is not registered")
	}
	if ts.Status != eventlog.StatusMinted {
		t.Errorf("drift.finding status = %q, want minted", ts.Status)
	}
	if ts.Family != eventlog.FamilyDriftCanary {
		t.Errorf("drift.finding family = %q, want %q", ts.Family, eventlog.FamilyDriftCanary)
	}
	if fam, _ := reg.Classify("registry.drift"); fam == eventlog.FamilyDriftCanary {
		t.Error("registry.drift classified as Drift & canary — it is S13.7 project-topology drift under Platform")
	}
	// canary.result stays declare-only: it is the sibling half's to mint.
	if cs, ok := reg.TypeSpec("canary.result"); !ok || cs.Status != eventlog.StatusDeclareOnly {
		t.Errorf("canary.result status = %+v, want declare-only until the API-canary layer", cs)
	}
}

// TestClassificationSchemaCarriesTheAbstainMember (S12.4): the second-pass
// schema is built here, reason-first, with `unclear` present by construction.
func TestClassificationSchemaCarriesTheAbstainMember(t *testing.T) {
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	raw := watchlist.WatchlistSchema()
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("the duty schema is not valid JSON: %v", err)
	}
	if len(schema.Required) == 0 || schema.Required[0] != "reason" {
		t.Errorf("required = %v, want reason first (the ordered-schema discipline)", schema.Required)
	}
	var cls struct {
		Enum []string `json:"enum"`
	}
	if err := json.Unmarshal(schema.Properties["change_class"], &cls); err != nil {
		t.Fatal(err)
	}
	var hasUnclear, hasNone bool
	for _, v := range cls.Enum {
		hasUnclear = hasUnclear || v == watchlist.ClassUnclear
		hasNone = hasNone || v == watchlist.ClassNone
	}
	if !hasUnclear {
		t.Error("the classification schema has no `unclear` abstain member — a model must never be schema-forced to fabricate a label")
	}
	if !hasNone {
		t.Error("the classification schema has no `none` member — an irrelevant change must be expressible")
	}
}

// TestSeverityVocabularyMatchesTheWatchdog: the two S14.4 alert-routing
// producers emit the SAME wire strings, so a single inbox renderer can route
// both without a per-producer special case.
func TestSeverityVocabularyMatchesTheWatchdog(t *testing.T) {
	if watchlist.SeverityFlagNow != watchdog.SeverityFlagNow {
		t.Errorf("flag-now vocabulary diverges: %q vs %q", watchlist.SeverityFlagNow, watchdog.SeverityFlagNow)
	}
	if watchlist.SeverityDigest != watchdog.SeverityDailyDigest {
		t.Errorf("digest vocabulary diverges: %q vs %q", watchlist.SeverityDigest, watchdog.SeverityDailyDigest)
	}
}
