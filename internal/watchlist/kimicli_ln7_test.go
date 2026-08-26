package watchlist

// kimicli_ln7_test.go — P3-LN-7 §10 specs T27, T28 (S16.4 item 9, §64).
//
// Two watch rows and one grammar member. The grammar is the load-bearing half:
// classify.go's lane enum is a CONSTRAINED-DECODING vocabulary, so until
// `kimi-cli` is in it the classifier literally cannot emit the lane and no hit
// could ever be attributed to it.

import (
	"strings"
	"testing"
)

// ── T28 · the classifier can NAME the lane ───────────────────────────────────

func TestClassifierGrammarCarriesKimiCLI(t *testing.T) {
	schema := string(WatchlistSchema())
	if !strings.Contains(schema, `"kimi-cli"`) {
		t.Errorf("the watchlist schema's lane enum does not carry kimi-cli — it is a constrained-decoding grammar, "+
			"so a lane missing from it can never be emitted and no hit can ever be attributed:\n%s", schema)
	}
	// The siblings must survive: a grammar edit that drops a member silently
	// stops attributing that lane.
	for _, lane := range []string{"anthropic", "zai", "kimi", "local"} {
		if !strings.Contains(schema, `"`+lane+`"`) {
			t.Errorf("the lane enum lost %q", lane)
		}
	}
}

// ── T27 · the two new watch rows ─────────────────────────────────────────────

// TestKimiCLIWatchRowsVerified pins both rows and their reasons.
//
// The community-guidelines row is the one that matters most, and its reason is
// recorded rather than implied: it is the page F0 came from, and the A11 audit
// NOT watching it is precisely how an interactive-only clause stayed invisible
// for two days while a lane was commissioned against it.
func TestKimiCLIWatchRowsVerified(t *testing.T) {
	rows := SeedRows()
	byID := map[string]Row{}
	for _, r := range rows {
		byID[r.ID] = r
	}

	// The CLI's own changelog — 70 releases in ~3 months makes this tier 1.
	changelog, ok := byID["t1-kimi-cli-changelog"]
	if !ok {
		t.Fatal("no t1-kimi-cli-changelog row — the pinned CLI publishes weekly and a pin nobody watches is a pin that rots")
	}
	if changelog.Lane != "kimi-cli" {
		t.Errorf("the CLI changelog row carries lane %q, want kimi-cli", changelog.Lane)
	}
	if changelog.Tier != 1 {
		t.Errorf("the CLI changelog row is tier %d, want 1", changelog.Tier)
	}
	if !strings.Contains(changelog.Notes, "2026-08-26") {
		t.Errorf("the CLI changelog row carries no verified-on date: %q", changelog.Notes)
	}

	// The Community Guidelines — the sanction surface, on the EXISTING kimi
	// lane, because the clause binds the membership and the audit it corrects
	// is that lane's.
	guidelines, ok := byID["t1-kimi-community-guidelines"]
	if !ok {
		t.Fatal("no t1-kimi-community-guidelines row — this is the page the interactive-only clause came from")
	}
	if guidelines.Lane != LaneKimi {
		t.Errorf("the guidelines row carries lane %q, want %q — the clause binds the membership, and the audit it "+
			"corrects is the kimi lane's", guidelines.Lane, LaneKimi)
	}
	if guidelines.Tier != 1 {
		t.Errorf("the guidelines row is tier %d, want 1", guidelines.Tier)
	}
	for _, needle := range []string{"2026-08-26", "interactive"} {
		if !strings.Contains(guidelines.Notes, needle) {
			t.Errorf("the guidelines row's notes miss %q: %q", needle, guidelines.Notes)
		}
	}
	// A row whose whole value is that somebody reads it must say what a change
	// there MEANS, or it becomes reassurance.
	if !strings.Contains(guidelines.Notes, "A11") && !strings.Contains(guidelines.Notes, "gray zone") {
		t.Errorf("the guidelines row does not record why it exists: %q", guidelines.Notes)
	}

	// The kimi lane's own row count moves by exactly one, and it is named.
	lane := 0
	for _, r := range rows {
		if r.Lane == LaneKimi {
			lane++
		}
	}
	if lane != 13 {
		t.Errorf("%d rows carry lane kimi, want 13 (12 before this packet + the community-guidelines page)", lane)
	}
	cli := 0
	for _, r := range rows {
		if r.Lane == "kimi-cli" {
			cli++
		}
	}
	if cli != 1 {
		t.Errorf("%d rows carry lane kimi-cli, want exactly 1 (the CLI changelog)", cli)
	}
}

// TestKimiCLICanaryCoverageIsRecordedNotDuplicated pins OQ-4's ratified answer.
//
// A fourth paid lane would take the disarmed-leg count from 9 to 12 at five
// pinned sites and, when armed, DOUBLE the real-request canary spend on ONE
// shared pool — for answers that are properties of the membership (auth
// sanction, the account's model list), not of the client path. Under R15 the
// two lanes cannot be entitled or revoked independently: one profile, one
// Console key.
func TestKimiCLICanaryCoverageIsRecordedNotDuplicated(t *testing.T) {
	paid := PaidLanes()
	if len(paid) != 3 {
		t.Errorf("PaidLanes() = %v, want three — kimi-cli deliberately does NOT join: its canary coverage is the "+
			"kimi lane's, by name, and arming a fourth leg would double the spend on one pool", paid)
	}
	for _, lane := range paid {
		if lane == "kimi-cli" {
			t.Error("kimi-cli joined PaidLanes() — OQ-4 ruled NO at LN-7")
		}
	}
}
