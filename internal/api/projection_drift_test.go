package api

import (
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// projection_drift_test.go — the B5-6A drift-card inbox derivation (S14.6 ¶2):
// open drift cards DERIVE from the drift.finding rows in run_events, folded by
// incident fingerprint into one card per storm, owner-scoped per S01.9.
// Internal test (package api) driving the projector directly — internal/api
// never imports internal/watchlist, so the findings are appended as raw event
// rows (the watchdog-flags / conformance-cards derive-from-log precedent).

func appendFinding(t *testing.T, log *eventlog.Log, at time.Time, owner, fingerprint, class, severity, summary string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"source": "https://example.invalid/pricing", "lanes": []string{"anthropic"},
		"change_class": class, "severity": severity, "summary": summary,
		"fingerprint": fingerprint, "row_id": "t1-anthropic-pricing", "kind": "page",
		"classified": true,
		"revalidation": map[string]any{
			"triggered": false,
			"reason":    "no revalidation triggered: change class " + class + " names no model (OQ4(a))",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(context.Background(), eventlog.Append{
		UserID: owner, Type: "drift.finding", SchemaVersion: 1, Payload: payload, Time: at,
	}); err != nil {
		t.Fatalf("append drift.finding: %v", err)
	}
}

// TestDriftCardsFoldOneStormIntoOneCard is acceptance item 8: N hits inside one
// incident window ⇒ ONE card; a new window ⇒ a new card.
func TestDriftCardsFoldOneStormIntoOneCard(t *testing.T) {
	ctx := context.Background()
	db, log, _ := wdTestDB(t)
	p := &projector{db: db}

	base := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)
	// Five hits of one incident, inside a single window.
	for i := 0; i < 5; i++ {
		appendFinding(t, log, base.Add(time.Duration(i)*time.Hour), "platform",
			"fp-price", "price", "flag-now", fmt.Sprintf("price moved (hit %d)", i))
	}
	snap, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.DriftCards) != 1 {
		t.Fatalf("%d cards for one storm, want 1 (fingerprint-per-incident-window dedup): %+v", len(snap.DriftCards), snap.DriftCards)
	}
	card := snap.DriftCards[0]
	if card.Hits != 5 {
		t.Errorf("card folded %d hits, want 5", card.Hits)
	}
	if card.Summary != "price moved (hit 4)" {
		t.Errorf("the card does not carry the latest summary: %q", card.Summary)
	}
	if card.Severity != "flag-now" || card.ChangeClass != "price" {
		t.Errorf("card severity/class = %s/%s", card.Severity, card.ChangeClass)
	}

	// A hit past the window opens a NEW card for the same fingerprint.
	appendFinding(t, log, base.Add(30*time.Hour), "platform", "fp-price", "price", "flag-now", "price moved again")
	snap, err = p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.DriftCards) != 2 {
		t.Fatalf("%d cards after a fresh incident window, want 2", len(snap.DriftCards))
	}

	// A DIFFERENT fingerprint is always its own card, same window or not.
	appendFinding(t, log, base.Add(time.Hour), "platform", "fp-limits", "limits", "flag-now", "rate limit changed")
	snap, err = p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.DriftCards) != 3 {
		t.Fatalf("%d cards, want 3 — a distinct fingerprint never folds into another incident", len(snap.DriftCards))
	}
}

// TestDriftCardsAreOwnerScoped (S01.9): findings are platform-scope, so the
// operator sees them and a member never does. There is no asks row anywhere.
func TestDriftCardsAreOwnerScoped(t *testing.T) {
	ctx := context.Background()
	db, log, _ := wdTestDB(t)
	p := &projector{db: db}

	base := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)
	appendFinding(t, log, base, "platform", "fp-a", "price", "flag-now", "a price moved")

	op, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(op.DriftCards) != 1 {
		t.Fatalf("the operator sees %d drift cards, want 1", len(op.DriftCards))
	}
	if !op.DriftCards[0].RevalidationTriggered && op.DriftCards[0].RevalidationNote == "" {
		t.Error("the card does not carry the revalidation note — a non-triggering class must say so VISIBLY (OQ4(a))")
	}

	member, err := p.inbox(ctx, ownerScope{Operator: false, UserID: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(member.DriftCards) != 0 {
		t.Fatalf("a member sees %d platform-scope drift cards, want 0", len(member.DriftCards))
	}
	if member.DriftCards == nil {
		t.Error("DriftCards must serialize as [] not null")
	}

	// No asks row was created for any of this (asks.run_id is NOT NULL; a watch
	// hit has no run, so a card is derivation, never a bare ask).
	var asks int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM asks`).Scan(&asks); err != nil {
		t.Fatal(err)
	}
	if asks != 0 {
		t.Errorf("%d asks rows exist — drift cards derive from the log, never from an asks row", asks)
	}
}

// TestNonConformantDriftPayloadIsSkippedNotFatal (S14.2 rule 3): a malformed
// payload never breaks the inbox.
func TestNonConformantDriftPayloadIsSkippedNotFatal(t *testing.T) {
	ctx := context.Background()
	db, log, _ := wdTestDB(t)
	p := &projector{db: db}
	if _, err := log.Append(ctx, eventlog.Append{
		UserID: "platform", Type: "drift.finding", SchemaVersion: 1,
		Payload: json.RawMessage(`"a bare string, not an object"`),
	}); err != nil {
		t.Fatal(err)
	}
	appendFinding(t, log, time.Now(), "platform", "fp-ok", "price", "daily-digest", "a good one")
	snap, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatalf("a non-conformant payload broke the inbox: %v", err)
	}
	if len(snap.DriftCards) != 1 {
		t.Errorf("%d cards, want 1 (the malformed row is skipped, the good one stands)", len(snap.DriftCards))
	}
}

// TestDriftFindingTagsInboxThroughTheExistingTopicMap is acceptance item 6: the
// SSE topic map routes drift for free — topics.go needed no edit.
func TestDriftFindingTagsInboxThroughTheExistingTopicMap(t *testing.T) {
	tags := topicsForEvent(eventlog.Event{Type: "drift.finding", SchemaVersion: 1})
	var hasInbox bool
	for _, tag := range tags {
		hasInbox = hasInbox || tag == topicInbox
		if tag == topicRun {
			t.Error("a platform-scope drift finding must carry no run tag")
		}
	}
	if !hasInbox {
		t.Errorf("drift.finding tags = %v, want the inbox topic", tags)
	}
	// canary.result is the OTHER half of family 11, minted at B5-6B. It routes
	// the same way for the same reason, so topics.go still needed no edit when
	// the API canary layer landed.
	var canaryInbox bool
	for _, tag := range topicsForEvent(eventlog.Event{Type: "canary.result", SchemaVersion: 1}) {
		canaryInbox = canaryInbox || tag == topicInbox
		if tag == topicRun {
			t.Error("a platform-scope canary result must carry no run tag")
		}
	}
	if !canaryInbox {
		t.Error("canary.result does not tag the inbox topic")
	}

	// registry.drift is S13.7 project-topology drift under Platform — it must
	// NOT route to the inbox through the Drift & canary family.
	for _, tag := range topicsForEvent(eventlog.Event{Type: "registry.drift", SchemaVersion: 1}) {
		if tag == topicInbox {
			t.Error("registry.drift routed to the inbox as drift — the name collision must not mis-route")
		}
	}
}

// TestCanaryFailureDerivesIntoADriftCard is the B5-6B checkable negative on the
// projection: a canary raises its card as a drift.finding through the SAME
// emitter path, so it derives through the SAME query with the SAME fingerprint
// dedup — internal/api gained no code and no import for the canary layer.
func TestCanaryFailureDerivesIntoADriftCard(t *testing.T) {
	ctx := context.Background()
	db, log, _ := wdTestDB(t)
	at := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)

	// Two model-list findings from ONE canary run: same source, same lane, same
	// change class, so one fingerprint and therefore ONE card.
	for _, subject := range []string{"claude-opus-4-8", "claude-sonnet-4-6"} {
		payload := []byte(`{"source":"canary:model-list:anthropic","lanes":["anthropic"],` +
			`"change_class":"models","severity":"flag-now","summary":"model-list drift on lane anthropic",` +
			`"fingerprint":"c0ffee00c0ffee00","row_id":"canary:model-list:anthropic","kind":"model-list",` +
			`"classified":true,"revalidation":{"triggered":true,"subject":"` + subject + `","reason":"models class with a model id"}}`)
		if _, err := log.Append(ctx, eventlog.Append{
			UserID: "platform", Type: "drift.finding", SchemaVersion: 1, Payload: payload, Time: at,
		}); err != nil {
			t.Fatalf("append canary drift finding: %v", err)
		}
		at = at.Add(time.Second)
	}

	p := &projector{db: db}
	snap, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	if len(snap.DriftCards) != 1 {
		t.Fatalf("%d drift cards, want 1 — the two subjects of one canary run fold into one card", len(snap.DriftCards))
	}
	if snap.DriftCards[0].Severity != "flag-now" {
		t.Errorf("card severity = %q, want flag-now", snap.DriftCards[0].Severity)
	}

	// A member sees none: canary cards are platform-scope (S01.9).
	memberSnap, err := p.inbox(ctx, ownerScope{UserID: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(memberSnap.DriftCards) != 0 {
		t.Errorf("a member sees %d platform-scope canary cards, want 0", len(memberSnap.DriftCards))
	}
}

// TestAPIDoesNotImportWatchlist is acceptance item 7: cards derive from the log;
// internal/api gains no edge to internal/watchlist.
func TestAPIDoesNotImportWatchlist(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, pkg := range pkgs {
		for name, f := range pkg.Files {
			files++
			for _, imp := range f.Imports {
				if strings.Contains(imp.Path.Value, "internal/watchlist") {
					t.Errorf("%s imports internal/watchlist — cards DERIVE from the log (§31)", filepath.Base(name))
				}
			}
		}
	}
	if files == 0 {
		t.Fatal("the scan read no files — it would pass vacuously")
	}
}

// TestDriftCardsAreBounded is the D10 lock: a drift finding has no close verb at
// v0 (dismiss is the B6 surface's), so without a bound every finding ever logged
// would derive into a forever-listed card and a re-firing condition would grow
// the inbox without limit — unlike OPEN-only watchdog flags and RED-only
// conformance rows. Findings past the horizon stay in run_events and remain
// queryable; they simply stop being open cards.
func TestDriftCardsAreBounded(t *testing.T) {
	ctx := context.Background()
	db, log, _ := wdTestDB(t)
	now := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	p := &projector{db: db, now: func() time.Time { return now }}

	// A stale incident, well past the horizon.
	appendFinding(t, log, now.Add(-90*24*time.Hour), "platform", "fp-old", "price", "flag-now", "ancient")
	// A live one.
	appendFinding(t, log, now.Add(-2*time.Hour), "platform", "fp-live", "price", "flag-now", "recent")

	snap, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.DriftCards) != 1 || snap.DriftCards[0].Fingerprint != "fp-live" {
		t.Fatalf("cards = %+v, want only the in-horizon incident", snap.DriftCards)
	}

	// A re-firing condition: far more distinct incidents than the cap, all live.
	for i := 0; i < driftCardCap+40; i++ {
		appendFinding(t, log, now.Add(-time.Duration(i)*time.Minute), "platform",
			fmt.Sprintf("fp-storm-%d", i), "limits", "daily-digest", "re-fired")
	}
	snap, err = p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.DriftCards) > driftCardCap {
		t.Fatalf("%d cards, want at most the cap %d — the inbox must be bounded", len(snap.DriftCards), driftCardCap)
	}
	// What survives is the NEWEST, never an arbitrary prefix.
	if snap.DriftCards[0].LatestSeq < snap.DriftCards[len(snap.DriftCards)-1].LatestSeq {
		t.Error("capped cards are not newest-first")
	}
}

// TestIrrelevantFindingsNeverRenderAsCards is the D4 belt at the projection: the
// producer already declines to emit a `none` classification, and any row written
// before that rule existed still never becomes a card (S14.6 ¶2: RELEVANT hits
// become drift cards).
func TestIrrelevantFindingsNeverRenderAsCards(t *testing.T) {
	ctx := context.Background()
	db, log, _ := wdTestDB(t)
	p := &projector{db: db}
	now := time.Now()

	appendFinding(t, log, now, "platform", "fp-none", "none", "daily-digest", "a docs typo")
	appendFinding(t, log, now, "platform", "fp-real", "price", "flag-now", "a price moved")

	snap, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.DriftCards) != 1 || snap.DriftCards[0].ChangeClass != "price" {
		t.Fatalf("cards = %+v, want only the relevant one", snap.DriftCards)
	}
}
