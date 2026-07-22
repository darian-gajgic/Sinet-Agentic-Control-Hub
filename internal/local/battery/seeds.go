package battery

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
)

// seeds.go — the versioned T15 seed suites (Spec S12.9; brief R6). These are
// SYNTHETIC bring-up seeds; real Sinet-domain traces accumulate under
// golden-set governance (the S14/B5 seam) toward the ~30–50-per-duty target
// (WarnBelow reports a suite under it — machine-visible, non-blocking, since a
// bring-up seed legitimately starts smaller and grows). Seeds ship IN CODE +
// strict-JSON operator overrides (LoadSuite / WriteSuite) — the verify/intake
// Load* precedent (CONVENTIONS §14/§15). Duty coverage is honest:
//
//   - classification duties with a servable seat + defined case shape:
//     intake-triage, watchdog-disambiguator, watchlist-triage, intent-filling,
//     entailment (binary), contradiction-screen (confirm), sql-open (Arctic);
//   - generation duties: utility + distill-summarize as OUTPUT-CONTRACT suites
//     (schema/length/grounding, not margins — their quality control is their
//     own S12.4 row);
//   - embedder: post-gate (G2 Def.8) — NO suite (recorded here, not faked).
//
// sql-open's suite measures the ALIAS (does the Arctic seat emit a
// single-statement SELECT for a NL question); its guardrail-stack query surface
// is S14/B5, not measured here.

// TargetPerDuty is the governance target case count per duty (S12.9 ~30–50).
const TargetPerDuty = 30

// WarnBelow reports the suites below the governance target — surfaced by the
// runner/CLI so a thin seed is visible, never silently accepted (R6).
func WarnBelow(suites []*Suite) []string {
	var warn []string
	for _, s := range suites {
		if len(s.Cases) < TargetPerDuty {
			warn = append(warn, fmt.Sprintf("%s: %d/%d cases (bring-up seed; grows from traces under governance)", s.Duty, len(s.Cases), TargetPerDuty))
		}
	}
	return warn
}

// SeedSuites returns the versioned bring-up suite set. embedder is deliberately
// absent (post-gate). Each suite validates.
func SeedSuites() []*Suite {
	return []*Suite{
		seedTriage(),
		seedWatchdog(),
		seedWatchlist(),
		seedIntentFilling(),
		seedEntailment(),
		seedContradiction(),
		seedSQLOpen(),
		seedUtility(),
		seedDistill(),
	}
}

// SuiteByDuty returns a seed suite by duty alias.
func SuiteByDuty(duty string) (*Suite, bool) {
	for _, s := range SeedSuites() {
		if s.Duty == duty {
			return s, true
		}
	}
	return nil, false
}

func tri(id, prompt, family, stakes, size string) Case {
	return Case{ID: id, Prompt: prompt, ExpectLabels: map[string]string{"family": family, "stakes": stakes, "size": size}}
}

func seedTriage() *Suite {
	return &Suite{
		Duty: local.AliasIntakeTriage, Kind: KindClassification,
		System: "You are a task triage classifier for a personal automation platform. Output ONLY JSON matching the schema — brief reasoning in \"reason\" first, then the labels. Set abstain=true ONLY if the request is too vague to classify at all — never guess.",
		Fields: []local.LabelField{
			{Name: "family", Enum: []string{"software", "research", "content", "data", "chore", "generic"}},
			{Name: "stakes", Enum: []string{"trivial", "low", "standard", "high"}},
			{Name: "size", Enum: []string{"trivial", "small", "medium", "large", "xlarge"}},
		},
		LabelFields: []string{"family", "stakes"},
		Cases: []Case{
			tri("t-01", "Add a unit test for the parseAge function in parser.go", "software", "low", "small"),
			tri("t-02", "Delete production database and recreate it from the seed script", "software", "high", "medium"),
			tri("t-03", "Fix the typo in the README where it says 'recieve'", "software", "trivial", "trivial"),
			tri("t-04", "Rewrite the auth middleware to use the new session store across all 40 endpoints", "software", "high", "large"),
			tri("t-05", "Research the current pricing of the three major cloud GPU providers with sources", "research", "standard", "medium"),
			tri("t-06", "Look up what year the Eiffel Tower was completed", "research", "trivial", "trivial"),
			tri("t-07", "Write a comparative report on vector database options for a 5M-embedding workload", "research", "standard", "large"),
			tri("t-08", "Draft a friendly reminder email to the team about the Friday deadline", "content", "low", "small"),
			tri("t-09", "Write the full launch announcement blog post and three social variants", "content", "standard", "medium"),
			tri("t-10", "Rename the variable x to userCount in one file", "software", "trivial", "trivial"),
			tri("t-11", "Migrate all customer records to the new schema and drop the old columns", "data", "high", "large"),
			tri("t-12", "Clean up the CSV: trim whitespace and normalize the date column", "data", "low", "small"),
			tri("t-13", "Deduplicate the contacts spreadsheet by email", "data", "low", "small"),
			tri("t-14", "Book a dentist appointment for next Tuesday afternoon", "chore", "low", "small"),
			tri("t-15", "Order more printer paper and coffee for the office", "chore", "trivial", "trivial"),
			tri("t-16", "Implement rate limiting on the public API with a token bucket, tests included", "software", "standard", "medium"),
			tri("t-17", "Summarize this 40-page contract and flag any auto-renewal clauses", "research", "high", "medium"),
			tri("t-18", "Add a --json flag to the CLI that prints machine-readable output", "software", "low", "small"),
			tri("t-19", "Wire up Stripe live payments and charge real cards on checkout", "software", "high", "large"),
			tri("t-20", "Generate a weekly status digest from the project's git log", "content", "low", "small"),
			tri("t-21", "Refactor the whole codebase to a new dependency-injection framework", "software", "high", "xlarge"),
			tri("t-22", "Proofread this two-paragraph bio for grammar", "content", "trivial", "trivial"),
			tri("t-23", "Build a small script to back up the photos folder to the NAS nightly", "software", "standard", "small"),
			tri("t-24", "Analyze last quarter's sales data and chart the top-10 products", "data", "standard", "medium"),
			tri("t-25", "Update the copyright year in the footer", "software", "trivial", "trivial"),
			tri("t-26", "Design and implement the entire multi-tenant billing subsystem", "software", "high", "xlarge"),
			tri("t-27", "Find three peer-reviewed studies on intermittent fasting and summarize them", "research", "standard", "medium"),
			tri("t-28", "Write release notes for version 2.4 from the merged PRs", "content", "low", "small"),
			tri("t-29", "Reset every user's password and email them the reset links", "chore", "high", "medium"),
			tri("t-30", "Add input validation to the signup form's email field", "software", "low", "small"),
			// A genuinely ambiguous request — abstain is the correct answer.
			{ID: "t-31", Prompt: "do the thing we talked about", ExpectAbstain: true},
			{ID: "t-32", Prompt: "handle it", ExpectAbstain: true},
		},
	}
}

func lbl(id, prompt string, labels map[string]string) Case {
	return Case{ID: id, Prompt: prompt, ExpectLabels: labels}
}

func seedWatchdog() *Suite {
	// watchdog-disambiguator: does a running task's latest step look STUCK,
	// PROGRESSING, or UNCLEAR (verdict IS `unclear` on low margin — never
	// escalates beyond unclear, never a cloud fallback; S2.7/S12.5).
	f := []local.LabelField{{Name: "verdict", Enum: []string{"progressing", "stuck", "unclear"}}}
	return &Suite{
		Duty: local.AliasWatchdog, Kind: KindClassification,
		System: "You judge whether an autonomous task is making progress. Reason briefly, then verdict ∈ {progressing, stuck, unclear}. Prefer 'unclear' over guessing.",
		Fields: f, LabelFields: []string{"verdict"},
		Cases: []Case{
			lbl("w-01", "Step log: 'ran tests (12 pass), committed fix, opening PR'", map[string]string{"verdict": "progressing"}),
			lbl("w-02", "Step log: retried the same failing command 8 times, same error each time", map[string]string{"verdict": "stuck"}),
			lbl("w-03", "Step log: 'waiting for build' repeated for 40 minutes, no new output", map[string]string{"verdict": "stuck"}),
			lbl("w-04", "Step log: implemented function, wrote test, test passes, moving to next AC", map[string]string{"verdict": "progressing"}),
			lbl("w-05", "Step log: edited three files, ran linter, fixed two warnings", map[string]string{"verdict": "progressing"}),
			lbl("w-06", "Step log: 'thinking about the approach' with no file changes for 30 min", map[string]string{"verdict": "stuck"}),
			lbl("w-07", "Step log: single line 'ok'", map[string]string{"verdict": "unclear"}),
			lbl("w-08", "Step log: alternating between two edits, reverting each, for 20 iterations", map[string]string{"verdict": "stuck"}),
			lbl("w-09", "Step log: downloaded dependency, built project, all green", map[string]string{"verdict": "progressing"}),
			lbl("w-10", "Step log: (empty)", map[string]string{"verdict": "unclear"}),
			lbl("w-11", "Step log: 'refactored module, added 3 tests, coverage up 4%'", map[string]string{"verdict": "progressing"}),
			lbl("w-12", "Step log: same stack trace printed 15 times, no code change between", map[string]string{"verdict": "stuck"}),
			lbl("w-13", "Step log: 'migrated 3 of 5 endpoints, tests green after each'", map[string]string{"verdict": "progressing"}),
			lbl("w-14", "Step log: waiting on the same npm install for 25 minutes, no output", map[string]string{"verdict": "stuck"}),
			lbl("w-15", "Step log: 'found the bug, wrote a failing test, now fixing'", map[string]string{"verdict": "progressing"}),
			lbl("w-16", "Step log: printed the same 'trying approach 2' line 10 times, no diff", map[string]string{"verdict": "stuck"}),
			lbl("w-17", "Step log: 'benchmarked, p99 down 40%, documenting the change'", map[string]string{"verdict": "progressing"}),
			lbl("w-18", "Step log: single char 'k'", map[string]string{"verdict": "unclear"}),
			lbl("w-19", "Step log: 'added CI job, first run passed, wiring the badge'", map[string]string{"verdict": "progressing"}),
			lbl("w-20", "Step log: re-reading the same file for the 12th time, no edits", map[string]string{"verdict": "stuck"}),
			lbl("w-21", "Step log: 'schema migration applied, backfill running, 60% done'", map[string]string{"verdict": "progressing"}),
			lbl("w-22", "Step log: '...'", map[string]string{"verdict": "unclear"}),
			lbl("w-23", "Step log: 'wrote the parser, 8 of 8 fixtures pass, moving on'", map[string]string{"verdict": "progressing"}),
			lbl("w-24", "Step log: apologizing and restarting the same failed step, 6th time", map[string]string{"verdict": "stuck"}),
			lbl("w-25", "Step log: 'cleaned up imports, ran gofmt+vet, both clean'", map[string]string{"verdict": "progressing"}),
			lbl("w-26", "Step log: tool timed out, retried, timed out, retried — 5 cycles", map[string]string{"verdict": "stuck"}),
			lbl("w-27", "Step log: 'implemented pagination, added a boundary test, green'", map[string]string{"verdict": "progressing"}),
			lbl("w-28", "Step log: 'hmm' (nothing else for 20 min)", map[string]string{"verdict": "stuck"}),
			lbl("w-29", "Step log: 'fixed the flaky test root cause, 20 runs all green'", map[string]string{"verdict": "progressing"}),
			lbl("w-30", "Step log: prose describing the plan but zero tool calls, 15 min", map[string]string{"verdict": "stuck"}),
		},
	}
}

func seedWatchlist() *Suite {
	f := []local.LabelField{{Name: "action", Enum: []string{"notify", "ignore", "unclear"}}}
	return &Suite{
		Duty: local.AliasWatchlist, Kind: KindClassification,
		System: "You triage a monitored-source change: does it warrant notifying the operator? Reason briefly, then action ∈ {notify, ignore, unclear}.",
		Fields: f, LabelFields: []string{"action"},
		Cases: []Case{
			lbl("wl-01", "Watched page diff: the product's price changed from $20 to $35", map[string]string{"action": "notify"}),
			lbl("wl-02", "Watched page diff: a trailing whitespace change in the footer", map[string]string{"action": "ignore"}),
			lbl("wl-03", "Watched page diff: 'In Stock' changed to 'Out of Stock'", map[string]string{"action": "notify"}),
			lbl("wl-04", "Watched page diff: the copyright year advanced by one", map[string]string{"action": "ignore"}),
			lbl("wl-05", "Watched page diff: a new security advisory was posted for the dependency", map[string]string{"action": "notify"}),
			lbl("wl-06", "Watched page diff: an ad banner rotated to a different image", map[string]string{"action": "ignore"}),
			lbl("wl-07", "Watched page diff: the API deprecation date moved up by six months", map[string]string{"action": "notify"}),
			lbl("wl-08", "Watched page diff: reordered two unrelated FAQ entries", map[string]string{"action": "ignore"}),
			lbl("wl-09", "Watched page diff: entire section removed, unclear what replaced it", map[string]string{"action": "unclear"}),
			lbl("wl-10", "Watched page diff: minified JS hash changed, no visible content change", map[string]string{"action": "unclear"}),
			lbl("wl-11", "Watched page diff: the terms-of-service liability clause was rewritten", map[string]string{"action": "notify"}),
			lbl("wl-12", "Watched page diff: a CSS color value changed by one hex digit", map[string]string{"action": "ignore"}),
			lbl("wl-13", "Watched page diff: the free-tier request cap dropped from 1000 to 100/min", map[string]string{"action": "notify"}),
			lbl("wl-14", "Watched page diff: whitespace reflow, identical rendered text", map[string]string{"action": "ignore"}),
			lbl("wl-15", "Watched page diff: a new required field added to the signup API", map[string]string{"action": "notify"}),
			lbl("wl-16", "Watched page diff: the analytics tracking-pixel URL changed", map[string]string{"action": "ignore"}),
			lbl("wl-17", "Watched page diff: the SLA credit percentage was cut in half", map[string]string{"action": "notify"}),
			lbl("wl-18", "Watched page diff: a testimonial quote was swapped for another", map[string]string{"action": "ignore"}),
			lbl("wl-19", "Watched page diff: the data-residency region changed from EU to US", map[string]string{"action": "notify"}),
			lbl("wl-20", "Watched page diff: font-family fallback list reordered", map[string]string{"action": "ignore"}),
			lbl("wl-21", "Watched page diff: an endpoint marked 'deprecated, removed in 30 days'", map[string]string{"action": "notify"}),
			lbl("wl-22", "Watched page diff: the page's meta description was tweaked", map[string]string{"action": "ignore"}),
			lbl("wl-23", "Watched page diff: a security CVE for the pinned dependency was posted", map[string]string{"action": "notify"}),
			lbl("wl-24", "Watched page diff: an image alt-text typo was fixed", map[string]string{"action": "ignore"}),
			lbl("wl-25", "Watched page diff: the pricing page removed the free tier entirely", map[string]string{"action": "notify"}),
			lbl("wl-26", "Watched page diff: cookie-banner copy reworded, same behavior", map[string]string{"action": "ignore"}),
			lbl("wl-27", "Watched page diff: the DB moved to a paid-only maintenance window", map[string]string{"action": "notify"}),
			lbl("wl-28", "Watched page diff: a footer link's tracking query param changed", map[string]string{"action": "ignore"}),
			lbl("wl-29", "Watched page diff: whole section replaced by 'coming soon', unclear intent", map[string]string{"action": "unclear"}),
			lbl("wl-30", "Watched page diff: server response header order changed, no content diff", map[string]string{"action": "unclear"}),
		},
	}
}

func seedIntentFilling() *Suite {
	f := []local.LabelField{{Name: "intent", Enum: []string{"create", "modify", "delete", "query", "unclear"}}}
	return &Suite{
		Duty: local.AliasIntentFilling, Kind: KindClassification,
		System: "You infer the primary intent of a short instruction. Reason briefly, then intent ∈ {create, modify, delete, query, unclear}. Prefer 'unclear' when the instruction is genuinely ambiguous.",
		Fields: f, LabelFields: []string{"intent"},
		Cases: []Case{
			lbl("if-01", "Add a new endpoint for user profiles", map[string]string{"intent": "create"}),
			lbl("if-02", "Change the timeout from 30s to 60s", map[string]string{"intent": "modify"}),
			lbl("if-03", "Remove the deprecated legacy import path", map[string]string{"intent": "delete"}),
			lbl("if-04", "How many active users signed up last week?", map[string]string{"intent": "query"}),
			lbl("if-05", "Build a settings page", map[string]string{"intent": "create"}),
			lbl("if-06", "Drop the temp table after the migration", map[string]string{"intent": "delete"}),
			lbl("if-07", "Update the docs to match the new flag name", map[string]string{"intent": "modify"}),
			lbl("if-08", "List the failing tests", map[string]string{"intent": "query"}),
			lbl("if-09", "Handle that", map[string]string{"intent": "unclear"}),
			lbl("if-10", "Make it better", map[string]string{"intent": "unclear"}),
			lbl("if-11", "Generate a new API key for the service account", map[string]string{"intent": "create"}),
			lbl("if-12", "Rename the column created_ts to created_at", map[string]string{"intent": "modify"}),
			lbl("if-13", "Add a health-check endpoint at /healthz", map[string]string{"intent": "create"}),
			lbl("if-14", "Bump the retry limit from 3 to 5", map[string]string{"intent": "modify"}),
			lbl("if-15", "Delete the orphaned migration files", map[string]string{"intent": "delete"}),
			lbl("if-16", "What's the current error rate on checkout?", map[string]string{"intent": "query"}),
			lbl("if-17", "Scaffold a new worker service", map[string]string{"intent": "create"}),
			lbl("if-18", "Purge sessions older than 90 days", map[string]string{"intent": "delete"}),
			lbl("if-19", "Switch the logger to JSON output", map[string]string{"intent": "modify"}),
			lbl("if-20", "Show me the slowest 10 queries", map[string]string{"intent": "query"}),
			lbl("if-21", "Create a dashboard for signup funnel", map[string]string{"intent": "create"}),
			lbl("if-22", "Remove the unused feature flag", map[string]string{"intent": "delete"}),
			lbl("if-23", "Change the default page size to 50", map[string]string{"intent": "modify"}),
			lbl("if-24", "Which endpoints lack auth?", map[string]string{"intent": "query"}),
			lbl("if-25", "Add a CSV export button", map[string]string{"intent": "create"}),
			lbl("if-26", "Drop the deprecated v1 routes", map[string]string{"intent": "delete"}),
			lbl("if-27", "Tune the cache TTL up to 10 minutes", map[string]string{"intent": "modify"}),
			lbl("if-28", "How many rows are in the audit table?", map[string]string{"intent": "query"}),
			lbl("if-29", "Deal with the situation", map[string]string{"intent": "unclear"}),
			lbl("if-30", "Sort it out somehow", map[string]string{"intent": "unclear"}),
		},
	}
}

func seedEntailment() *Suite {
	// entailment: binary supported/unsupported of a claim against source text.
	f := []local.LabelField{{Name: "verdict", Enum: []string{"supported", "unsupported"}}}
	pair := func(id, claim, src, want string) Case {
		return Case{ID: id, Prompt: "CLAIM: " + claim + "\n\nSOURCE:\n" + src + "\n\nIs the claim supported by the source? Answer supported or unsupported.", ExpectLabels: map[string]string{"verdict": want}}
	}
	return &Suite{
		Duty: local.AliasEntailment, Kind: KindClassification,
		System: "You are a strict grounding checker. A claim is 'supported' ONLY if the source text entails it; otherwise 'unsupported'. Reason briefly, then verdict.",
		Fields: f, LabelFields: []string{"verdict"},
		Cases: []Case{
			pair("e-01", "The library is Apache-2.0 licensed.", "This project is licensed under the Apache License, Version 2.0.", "supported"),
			pair("e-02", "The library is MIT licensed.", "This project is licensed under the Apache License, Version 2.0.", "unsupported"),
			pair("e-03", "The default timeout is 30 seconds.", "timeout (int, default 30): seconds to wait before aborting.", "supported"),
			pair("e-04", "Requests never time out.", "timeout (int, default 30): seconds to wait before aborting.", "unsupported"),
			pair("e-05", "Free tier allows 100 requests per minute.", "Free tier: up to 100 requests/minute per API key.", "supported"),
			pair("e-06", "Free tier allows 1000 requests per minute.", "Free tier: up to 100 requests/minute per API key.", "unsupported"),
			pair("e-07", "Python 3.8 support was removed in 3.2.", "Changelog 3.2: minimum supported Python is now 3.9; 3.8 removed.", "supported"),
			pair("e-08", "Python 3.8 support was added in 3.2.", "Changelog 3.2: minimum supported Python is now 3.9; 3.8 removed.", "unsupported"),
			pair("e-09", "Data is stored in the EU by default.", "By default, all customer data is stored in our Frankfurt (EU) region.", "supported"),
			pair("e-10", "Data is stored in the US by default.", "By default, all customer data is stored in our Frankfurt (EU) region.", "unsupported"),
			pair("e-11", "The service offers a 99.9% uptime SLA.", "Our SLA guarantees 99.9% monthly uptime for paid plans.", "supported"),
			pair("e-12", "The service offers a 99.99% uptime SLA.", "Our SLA guarantees 99.9% monthly uptime for paid plans.", "unsupported"),
			pair("e-13", "The max upload size is 25 MB.", "Uploads are capped at 25 MB per file.", "supported"),
			pair("e-14", "The max upload size is 250 MB.", "Uploads are capped at 25 MB per file.", "unsupported"),
			pair("e-15", "Refunds are available within 30 days.", "You may request a refund up to 30 days after purchase.", "supported"),
			pair("e-16", "Refunds are available anytime.", "You may request a refund up to 30 days after purchase.", "unsupported"),
			pair("e-17", "The library requires Node 18 or newer.", "Requires Node.js >= 18.", "supported"),
			pair("e-18", "The library runs on Node 14.", "Requires Node.js >= 18.", "unsupported"),
			pair("e-19", "Data is encrypted at rest with AES-256.", "All stored data is encrypted at rest using AES-256.", "supported"),
			pair("e-20", "Data at rest is unencrypted.", "All stored data is encrypted at rest using AES-256.", "unsupported"),
			pair("e-21", "The webhook retries up to 5 times.", "Failed webhooks are retried at most 5 times with backoff.", "supported"),
			pair("e-22", "The webhook never retries.", "Failed webhooks are retried at most 5 times with backoff.", "unsupported"),
			pair("e-23", "The CLI supports a --dry-run flag.", "Use --dry-run to preview changes without applying them.", "supported"),
			pair("e-24", "The CLI has no dry-run option.", "Use --dry-run to preview changes without applying them.", "unsupported"),
			pair("e-25", "Sessions expire after 24 hours.", "A session token is valid for 24 hours, then re-auth is required.", "supported"),
			pair("e-26", "Sessions never expire.", "A session token is valid for 24 hours, then re-auth is required.", "unsupported"),
			pair("e-27", "The API returns JSON by default.", "Responses default to application/json.", "supported"),
			pair("e-28", "The API returns XML by default.", "Responses default to application/json.", "unsupported"),
			pair("e-29", "Rate limits reset every minute.", "The limit window is one minute and resets on the minute.", "supported"),
			pair("e-30", "Rate limits reset every hour.", "The limit window is one minute and resets on the minute.", "unsupported"),
		},
	}
}

func seedContradiction() *Suite {
	// contradiction-screen (confirm stage): do two lesson statements contradict?
	f := []local.LabelField{{Name: "contradicts", Enum: []string{"yes", "no", "unclear"}}}
	pair := func(id, a, b, want string) Case {
		return Case{ID: id, Prompt: "STATEMENT A: " + a + "\nSTATEMENT B: " + b + "\n\nDo these contradict? yes, no, or unclear.", ExpectLabels: map[string]string{"contradicts": want}}
	}
	return &Suite{
		Duty: local.AliasContradiction, Kind: KindClassification,
		System: "You screen two stored lessons for contradiction. High precision: say 'yes' only for a genuine, direct contradiction. Reason briefly, then contradicts ∈ {yes, no, unclear}.",
		Fields: f, LabelFields: []string{"contradicts"},
		Cases: []Case{
			pair("cs-01", "Always deploy on Fridays.", "Never deploy on Fridays.", "yes"),
			pair("cs-02", "Use tabs for indentation.", "Use spaces for indentation.", "yes"),
			pair("cs-03", "The staging DB is read-only.", "The staging DB accepts writes for tests.", "yes"),
			pair("cs-04", "Prefer small PRs.", "Add tests to every PR.", "no"),
			pair("cs-05", "The API base URL is api.example.com.", "The docs live at docs.example.com.", "no"),
			pair("cs-06", "Retries are capped at 3.", "Back off exponentially between retries.", "no"),
			pair("cs-07", "Secrets go in the vault.", "Never commit secrets to git.", "no"),
			pair("cs-08", "The default region is us-east-1.", "The default region is eu-west-1.", "yes"),
			pair("cs-09", "Run migrations before deploy.", "Run migrations after deploy.", "yes"),
			pair("cs-10", "The cache TTL is 5 minutes.", "The session TTL is 30 days.", "no"),
			pair("cs-11", "Feature flags default to off.", "New features ship behind flags.", "no"),
			pair("cs-12", "Log level is info in prod.", "Log level is debug in prod.", "yes"),
			pair("cs-13", "Squash-merge every PR.", "Never squash — preserve the full commit history.", "yes"),
			pair("cs-14", "Timeouts are 30 seconds.", "Timeouts are 60 seconds.", "yes"),
			pair("cs-15", "Use 2-space indentation.", "Write clear commit messages.", "no"),
			pair("cs-16", "Deploy from main only.", "Tag releases with semver.", "no"),
			pair("cs-17", "The API is versioned in the URL path.", "The API is unversioned at v0.", "yes"),
			pair("cs-18", "Keep functions under 50 lines.", "Add doc comments to exported symbols.", "no"),
			pair("cs-19", "Store timestamps in UTC.", "Store timestamps in the requester's local timezone.", "yes"),
			pair("cs-20", "Retries use exponential backoff.", "Prefer small, focused pull requests.", "no"),
			pair("cs-21", "The primary key is a UUID.", "The primary key is an auto-increment integer.", "yes"),
			pair("cs-22", "Run the linter in CI.", "Run the tests in CI.", "no"),
			pair("cs-23", "Passwords are hashed with argon2id.", "Passwords are stored in plaintext.", "yes"),
			pair("cs-24", "Use feature branches.", "Rebase before merging.", "no"),
			pair("cs-25", "The service is stateless.", "The service holds per-user session state in memory.", "yes"),
			pair("cs-26", "Cache responses for 5 minutes.", "Invalidate the cache on write.", "no"),
			pair("cs-27", "Migrations run automatically on deploy.", "Migrations must be applied manually by an operator.", "yes"),
			pair("cs-28", "The build is reproducible.", "Pin all dependency versions.", "no"),
			pair("cs-29", "The default region is us-east.", "The default region depends on the account's signup choice.", "unclear"),
			pair("cs-30", "Rate limit is per-key.", "Rate limiting applies somewhere in the stack.", "unclear"),
		},
	}
}

func seedSQLOpen() *Suite {
	// sql-open (Arctic seat): does the model emit a single SELECT for a NL
	// question? Output-contract shaped: measures the alias, not the guardrail
	// query surface (S14/B5). Grounded on a single-statement SELECT.
	sql := func(id, q string) Case {
		return Case{ID: id, Prompt: "Schema: users(id, name, email, created_at, active). Question: " + q + "\nReturn ONLY a single SQL SELECT statement.",
			Contract: &Contract{MaxChars: 600, Grounding: []string{"SELECT"}}}
	}
	return &Suite{
		Duty: local.AliasSQLOpen, Kind: KindOutputContract,
		System: "You translate a natural-language question into a single read-only SQL SELECT over the given schema. Output ONLY the SQL.",
		Cases: []Case{
			sql("sq-01", "How many active users are there?"),
			sql("sq-02", "List the emails of users created this year."),
			sql("sq-03", "What is the name of the most recently created user?"),
			sql("sq-04", "How many users signed up each month?"),
			sql("sq-05", "Find users with no email set."),
			sql("sq-06", "Count inactive users."),
			sql("sq-07", "List the 5 oldest accounts by created_at."),
			sql("sq-08", "Which users have a gmail address?"),
			sql("sq-09", "How many users are named Alex?"),
			sql("sq-10", "Show the newest active user's email."),
			sql("sq-11", "List all users ordered by name."),
			sql("sq-12", "How many users have never set a name?"),
			sql("sq-13", "Count users created before 2025."),
			sql("sq-14", "Find the email of the user with id 42."),
			sql("sq-15", "List distinct email domains."),
			sql("sq-16", "How many active vs inactive users are there?"),
			sql("sq-17", "Show users created in the last 7 days."),
			sql("sq-18", "What is the earliest created_at?"),
			sql("sq-19", "List users whose name starts with 'A'."),
			sql("sq-20", "Count users per creation year."),
			sql("sq-21", "Find active users without a gmail address."),
			sql("sq-22", "Show the 3 most recently created active users."),
			sql("sq-23", "How many distinct names are there?"),
			sql("sq-24", "List inactive users created this month."),
			sql("sq-25", "What fraction of users are active?"),
			sql("sq-26", "Find users with a duplicate email."),
			sql("sq-27", "Show the oldest active user's name and email."),
			sql("sq-28", "Count users whose email ends in .org."),
			sql("sq-29", "List users created between two dates (2025-01-01 and 2025-06-30)."),
			sql("sq-30", "How many users signed up on the first of any month?"),
		},
	}
}

func seedUtility() *Suite {
	// utility (drafting) — output-contract: the help JSON carries the three
	// required fields, within the length cap (no margin — generation shape, R6).
	help := func(id, goal string) Case {
		return Case{ID: id, Prompt: "Goal: " + goal + "\nDraft plain-language help as JSON with fields what, wrong, recommend.",
			Contract: &Contract{MaxChars: 2000, MustContainFields: []string{"what", "wrong", "recommend"}}}
	}
	return &Suite{
		Duty: local.AliasUtility, Kind: KindOutputContract,
		System: "You draft plain-language help for a non-technical reader as JSON {what, wrong, recommend}. Be concrete and brief.",
		Cases: []Case{
			help("u-01", "delete and recreate the production database"),
			help("u-02", "wire up live Stripe payments"),
			help("u-03", "reset every user's password"),
			help("u-04", "migrate customer records to a new schema"),
			help("u-05", "add a --json flag to the CLI"),
			help("u-06", "back up photos to the NAS nightly"),
			help("u-07", "rename a database column"),
			help("u-08", "add rate limiting to the public API"),
			help("u-09", "proofread a short bio"),
			help("u-10", "generate weekly release notes from git"),
			help("u-11", "rotate the production API keys"),
			help("u-12", "enable two-factor authentication for all users"),
			help("u-13", "purge inactive accounts older than a year"),
			help("u-14", "switch the payment provider"),
			help("u-15", "add rate limiting to the login endpoint"),
			help("u-16", "restore the database from last night's backup"),
			help("u-17", "deploy the new pricing page"),
			help("u-18", "merge the long-lived feature branch"),
			help("u-19", "grant an intern read-only database access"),
			help("u-20", "schedule nightly report emails"),
			help("u-21", "archive last quarter's logs to cold storage"),
			help("u-22", "enable HTTPS-only cookies"),
			help("u-23", "split the monolith's billing module out"),
			help("u-24", "add a data-export endpoint for GDPR requests"),
			help("u-25", "increase the connection-pool size"),
			help("u-26", "turn on verbose logging in production"),
			help("u-27", "delete a customer's account on request"),
			help("u-28", "migrate secrets to the vault"),
			help("u-29", "add a maintenance-mode banner"),
			help("u-30", "downgrade a dependency to fix a regression"),
		},
	}
}

func seedDistill() *Suite {
	dist := func(id, text string) Case {
		return Case{ID: id, Prompt: "Summarize in one sentence:\n" + text,
			Contract: &Contract{MaxChars: 400}}
	}
	return &Suite{
		Duty: local.AliasDistillSummarize, Kind: KindOutputContract,
		System: "You produce a single-sentence summary of the input. Be faithful and concise.",
		Cases: []Case{
			dist("d-01", "The build failed because the CUDA toolkit version exceeded the compiler's supported range; downgrading gcc fixed it."),
			dist("d-02", "The team decided to defer the billing subsystem to Q3 and prioritize the auth refactor first."),
			dist("d-03", "Users reported slow page loads traced to an unindexed query on the orders table."),
			dist("d-04", "The API deprecation was moved up six months, so clients must migrate before March."),
			dist("d-05", "A memory leak in the worker pool was caused by goroutines not being cancelled on context timeout."),
			dist("d-06", "The migration dropped a column still referenced by a report, breaking the nightly export."),
			dist("d-07", "Switching the cache from in-process to Redis cut p99 latency in half under load."),
			dist("d-08", "The security review flagged a SQL injection in the search endpoint built by string concatenation."),
			dist("d-09", "The release was rolled back after the new payment flow double-charged a small number of customers."),
			dist("d-10", "Adding request validation at the trust boundary eliminated the class of malformed-input crashes."),
			dist("d-11", "The outage was caused by a full disk on the primary log host; rotating and capping logs restored service."),
			dist("d-12", "A race in the cache-warming goroutine returned stale prices for a few seconds after each deploy."),
			dist("d-13", "The team standardized on structured logging so incidents can be queried by request id."),
			dist("d-14", "An unindexed foreign key made the delete cascade lock the table under load."),
			dist("d-15", "The new retry policy amplified a downstream outage into a thundering herd."),
			dist("d-16", "Moving image processing to a worker queue cut request latency and freed web workers."),
			dist("d-17", "A misconfigured feature flag exposed an unfinished page to 5% of users."),
			dist("d-18", "The CI cache key ignored the lockfile, so stale dependencies passed tests but broke prod."),
			dist("d-19", "Switching to prepared statements removed the injection risk and improved plan reuse."),
			dist("d-20", "The nightly export silently truncated because the temp volume filled during peak."),
			dist("d-21", "A clock skew between nodes caused JWTs to be rejected as not-yet-valid."),
			dist("d-22", "Batching the writes reduced database round-trips from thousands to dozens per request."),
			dist("d-23", "The refactor removed a defensive nil check that a rare code path actually relied on."),
			dist("d-24", "Enabling connection pooling fixed the intermittent 'too many connections' errors."),
			dist("d-25", "The alert was noisy because the threshold was set below normal daily variance."),
			dist("d-26", "A dependency's minor bump changed a default and broke timezone handling."),
			dist("d-27", "The backup restore drill revealed the restore script never recreated the FTS index."),
			dist("d-28", "Rolling the deploy region-by-region contained the blast radius of the bad config."),
			dist("d-29", "The memory leak traced to unbounded growth of an in-memory dedup map."),
			dist("d-30", "Adding idempotency keys made the payment webhook safe to retry."),
		},
	}
}

// ---- strict-JSON operator overrides (the verify/intake Load* precedent) ----

// LoadSuite decodes an operator-edited suite file, rejecting unknown fields.
func LoadSuite(path string) (*Suite, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("battery: read suite %s: %w", path, err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var s Suite
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("battery: decode suite %s: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// WriteSuite materializes a suite as pretty JSON for operator editing + gate
// review (the WriteSeed precedent).
func WriteSuite(path string, s *Suite) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("battery: marshal suite: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("battery: write suite %s: %w", path, err)
	}
	return nil
}
