package settings_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// read_test.go — the bulk reads the S15.9 settings surface serves from
// (P3-B6-3A R1/R3).

func valueOf(t *testing.T, reg *settings.Registry, key string) settings.Value {
	t.Helper()
	for _, v := range reg.Values() {
		if v.Key == key {
			return v
		}
	}
	t.Fatalf("Values() has no entry for %q", key)
	return settings.Value{}
}

// TestValuesCoversTheWholeIndexWithItsMetadata: R1's values block is complete
// and carries what the UI renders — help, unit, badges, ratification and the
// EFFECTIVE clamp bounds.
func TestValuesCoversTheWholeIndexWithItsMetadata(t *testing.T) {
	reg := settings.New()
	vals := reg.Values()
	if len(vals) != 118 {
		t.Fatalf("Values() has %d entries, want 118 (S18.5)", len(vals))
	}
	seen := map[string]bool{}
	bounded := 0
	for _, v := range vals {
		if seen[v.Key] {
			t.Errorf("key %q appears twice in the values block", v.Key)
		}
		seen[v.Key] = true
		if v.Title == "" || v.Help == "" || v.Ratified == "" || v.Section == "" || v.Type == "" {
			t.Errorf("%s: values block is missing mandatory metadata: %+v", v.Key, v)
		}
		if v.Default == nil {
			t.Errorf("%s: no default served", v.Key)
		}
		if v.Overridden {
			t.Errorf("%s: reports an override on a fresh registry", v.Key)
		}
		if v.Numeric && (v.Floor != nil || v.Ceiling != nil) {
			bounded++
		}
		if !v.Numeric && (v.Floor != nil || v.Ceiling != nil) {
			t.Errorf("%s is %s and carries bounds", v.Key, v.Type)
		}
	}
	if bounded == 0 {
		t.Fatal("no key served clamp bounds — S15.9 requires the UI to show them")
	}
	// The effective value equals the declared default before any write.
	if v := valueOf(t, reg, "orchestration.helper_turns"); v.Effective != v.Default {
		t.Errorf("effective %v != default %v on a fresh registry", v.Effective, v.Default)
	}
}

// TestValuesFollowWritesAndResets: the values block is a READ of the live
// registry, so a set shows up on the next read (live-apply) and a reset takes
// the override away again.
func TestValuesFollowWritesAndResets(t *testing.T) {
	reg, _, _ := attached(t)
	ctx := context.Background()
	const key = "orchestration.helper_turns"

	if err := reg.Set(ctx, settings.SetRequest{
		Key: key, Value: json.RawMessage(`30`), Actor: operator, Reason: "more room",
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v := valueOf(t, reg, key)
	if v.Effective != int64(30) {
		t.Errorf("effective = %v, want 30 — a set must be visible on the next read with no restart", v.Effective)
	}
	if !v.Overridden {
		t.Error("overridden = false after a set")
	}
	if v.Default == int64(30) {
		t.Error("the default moved with the write — the default lives in code (S01.10)")
	}

	if err := reg.Set(ctx, settings.SetRequest{Key: key, Actor: operator, Reason: "back"}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	v = valueOf(t, reg, key)
	if v.Overridden {
		t.Error("overridden = true after a reset — reset-to-default deletes the row (S01.10)")
	}
	if v.Effective != v.Default {
		t.Errorf("after reset effective = %v, want the default %v", v.Effective, v.Default)
	}
}

// TestValuesCarryPerUserOverrides: a per-user override is served on its key,
// keyed by the person it belongs to.
func TestValuesCarryPerUserOverrides(t *testing.T) {
	reg, _, _ := attached(t)
	ctx := context.Background()
	const key = "retention.compaction_horizon"

	if err := reg.Set(ctx, settings.SetRequest{
		Key: key, ForUser: "alice", Value: json.RawMessage(`9`), Actor: operator,
	}); err != nil {
		t.Fatalf("Set for user: %v", err)
	}
	v := valueOf(t, reg, key)
	if got := v.UserValues["alice"]; got != int64(9) {
		t.Errorf("alice's override = %v, want 9", got)
	}
	if len(v.UserValues) != 1 {
		t.Errorf("user_values = %v, want just alice's", v.UserValues)
	}
	// The platform-scope effective value is untouched by a per-user override.
	if v.Effective != v.Default {
		t.Errorf("platform effective = %v, want the default %v — a per-user override is not a platform write", v.Effective, v.Default)
	}
	// A key with no per-user override carries none rather than an empty map.
	if u := valueOf(t, reg, "orchestration.helper_turns").UserValues; u != nil {
		t.Errorf("a key with no per-user override serves %v, want absent", u)
	}
}

// TestValuesServeEffectiveBoundsNotJustDeclaredOnes: R1 serves the bounds
// actually in force, so an operator bounds edit is visible to the UI that has
// to enforce it.
func TestValuesServeEffectiveBoundsNotJustDeclaredOnes(t *testing.T) {
	reg, _, _ := attached(t)
	ctx := context.Background()
	const key = "orchestration.helper_turns"

	before := valueOf(t, reg, key)
	if before.Ceiling != nil {
		t.Fatalf("fixture assumption broken: %s already has a declared ceiling", key)
	}
	ceiling := 40.0
	if err := reg.SetBounds(ctx, settings.SetBoundsRequest{Key: key, Ceiling: &ceiling, Actor: operator}); err != nil {
		t.Fatalf("SetBounds: %v", err)
	}
	after := valueOf(t, reg, key)
	if after.Ceiling == nil || *after.Ceiling != 40 {
		t.Fatalf("effective ceiling = %v, want 40 — the values block must serve the bounds in force", after.Ceiling)
	}
	// And the registry's own clamp agrees with what was served.
	if err := reg.Set(ctx, settings.SetRequest{Key: key, Value: json.RawMessage(`41`), Actor: operator}); !errors.Is(err, settings.ErrOutOfBounds) {
		t.Fatalf("Set past the served ceiling: %v, want ErrOutOfBounds", err)
	}
}

// TestHistoryIsNewestFirstAndBounded is R3: the per-setting audit history reads
// from settings_events, newest first, capped, with the S01.10 columns verbatim.
func TestHistoryIsNewestFirstAndBounded(t *testing.T) {
	reg, _, _ := attached(t)
	ctx := context.Background()
	const key = "orchestration.helper_turns"

	for _, v := range []string{`21`, `22`, `23`} {
		if err := reg.Set(ctx, settings.SetRequest{
			Key: key, Value: json.RawMessage(v), Actor: operator, Reason: "step " + v,
		}); err != nil {
			t.Fatalf("Set %s: %v", v, err)
		}
	}
	// A write on ANOTHER key must not appear in this key's history.
	if err := reg.Set(ctx, settings.SetRequest{
		Key: "orchestration.helper_tokens", Value: json.RawMessage(`90000`), Actor: operator,
	}); err != nil {
		t.Fatalf("Set other key: %v", err)
	}

	entries, err := reg.History(ctx, key, 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("history has %d entries, want 3 (and none from another key)", len(entries))
	}
	if string(entries[0].New) != "23" {
		t.Errorf("newest entry new = %s, want 23 — history is newest-first", entries[0].New)
	}
	if string(entries[2].New) != "21" {
		t.Errorf("oldest entry new = %s, want 21", entries[2].New)
	}
	if string(entries[0].Old) != "22" {
		t.Errorf("newest entry old = %s, want 22 — old→new is the pair S01.10 records", entries[0].Old)
	}
	for _, e := range entries {
		if e.Key != key {
			t.Errorf("entry key = %q, want %q", e.Key, key)
		}
		if e.Actor != operator.String() {
			t.Errorf("entry actor = %q, want %q", e.Actor, operator.String())
		}
		if e.TS == "" || e.Reason == "" {
			t.Errorf("entry missing ts/reason: %+v", e)
		}
	}

	// Bounded.
	short, err := reg.History(ctx, key, 2)
	if err != nil {
		t.Fatalf("History bounded: %v", err)
	}
	if len(short) != 2 {
		t.Errorf("bounded history returned %d, want 2", len(short))
	}
	if string(short[0].New) != "23" {
		t.Errorf("the bound kept the wrong end: %s", short[0].New)
	}

	// A per-user write records its scope on the row.
	if err := reg.Set(ctx, settings.SetRequest{
		Key: "retention.compaction_horizon", ForUser: "alice", Value: json.RawMessage(`9`), Actor: operator,
	}); err != nil {
		t.Fatalf("per-user Set: %v", err)
	}
	perUser, err := reg.History(ctx, "retention.compaction_horizon", 10)
	if err != nil {
		t.Fatalf("History per-user: %v", err)
	}
	if len(perUser) != 1 || perUser[0].UserID != "alice" {
		t.Errorf("per-user audit row = %+v, want one scoped to alice", perUser)
	}
}

// TestHistoryRefusesAnUndeclaredKeyAndABadBound: the read's own boundary.
func TestHistoryRefusesAnUndeclaredKeyAndABadBound(t *testing.T) {
	reg, _, _ := attached(t)
	ctx := context.Background()
	if _, err := reg.History(ctx, "no.such_key", 10); !errors.Is(err, settings.ErrUnknownKey) {
		t.Errorf("History on an undeclared key: %v, want ErrUnknownKey", err)
	}
	if _, err := reg.History(ctx, "orchestration.helper_turns", 0); err == nil {
		t.Error("History accepted a non-positive limit")
	}
	// Unattached: writes need the audit trail, and so does reading it.
	if _, err := settings.New().History(ctx, "orchestration.helper_turns", 10); !errors.Is(err, settings.ErrNotAttached) {
		t.Errorf("History before Attach: %v, want ErrNotAttached", err)
	}
}

// TestBoundsEditLandsItsOwnAuditRow: the bounds edit is audited on the same
// trail as a value write (G1 rider 1), so an operator moving a ceiling is as
// visible as an operator moving a value.
func TestBoundsEditLandsItsOwnAuditRow(t *testing.T) {
	reg, _, _ := attached(t)
	ctx := context.Background()
	const key = "orchestration.helper_turns"
	ceiling := 40.0
	if err := reg.SetBounds(ctx, settings.SetBoundsRequest{
		Key: key, Ceiling: &ceiling, Actor: operator, Reason: "cap it",
	}); err != nil {
		t.Fatalf("SetBounds: %v", err)
	}
	entries, err := reg.History(ctx, key, 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("bounds edit left %d audit rows, want 1", len(entries))
	}
	var got struct {
		Ceiling *float64 `json:"ceiling"`
	}
	if err := json.Unmarshal(entries[0].New, &got); err != nil {
		t.Fatalf("bounds audit new side is not the bounds object: %s", entries[0].New)
	}
	if got.Ceiling == nil || *got.Ceiling != 40 {
		t.Errorf("bounds audit new = %s, want the new ceiling", entries[0].New)
	}
}
