package watchlist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// seed.go — the watch-row seed set, in code (the §14/§15/§17/§32 in-code-seed
// precedent). Content is the settled report-02 §6 tiered list, every standing
// row S16.8 registered into this executor, the G4 [schema] STUDY row, and one
// review row per components.lock entry so every entry's `watch` reference
// resolves to a real row (S16.4 #9, asserted by test).
//
// Every row cites its ratifying source in `notes`. Rows are seeded idempotently
// at shell boot, UNCONDITIONALLY: platform obligations need no D10 holder.
//
// Operator editing goes through LoadRows (strict JSON — an unknown field fails
// LOUDLY, never silently ignored) with WriteSeed dumping the in-code set for
// gate review.

// LockEntryNames is the components.lock entry set this seed materializes a
// review row for. A test asserts it matches the real manifest exactly, so a new
// adoption without its watch row fails the build (S16.4 #9).
var LockEntryNames = []string{
	"Go toolchain",
	"actions/checkout",
	"actions/setup-go",
	"SQLite via pure-Go driver",
	"golang.org/x/crypto",
	"claude CLI (engine)",
	"sandbox-runtime",
	"OS mechanisms: bubblewrap/seccomp/Landlock/netns+nftables",
	"PDF text extraction",
	"git (host CLI)",
	"ttyd (terminal-over-WebSocket)",
	"age (snapshot encryption)",
	"zstd (pure-Go compression)",
	"Caddy",
	"llama-swap",
	"llama.cpp (llama-server)",
	"Ollama",
	"promptfoo",
	"DeepEval",
	"changedetection.io",
	"genai-prices data.json",
}

// LockRowID is the deterministic watch-row id for a components.lock entry. The
// test that checks manifest coverage uses this same function, so the two can
// never drift.
func LockRowID(entryName string) string { return "lock-" + slug(entryName) }

func slug(s string) string {
	var b strings.Builder
	prevDash := true
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// SeedRows returns the complete in-code seed set.
func SeedRows() []Row {
	rows := make([]Row, 0, 96)
	rows = append(rows, tier1Rows()...)
	rows = append(rows, tier2Rows()...)
	rows = append(rows, tier3Rows()...)
	rows = append(rows, tier4Rows()...)
	rows = append(rows, standingRows()...)
	rows = append(rows, studyRows()...)
	rows = append(rows, lockReviewRows()...)
	return rows
}

const (
	report02Cite = "report-02 §6 (the settled source list; G2 Def.12)"
	s168Cite     = "Spec S16.8 standing watchlist row, registered into the S14.6 executor"
)

// ── Tier 1: authoritative pages, poll/diff weekly (report-02 §6) ───────────
//
// These ride the changedetection.io page tier. Every one carries the P-T11-3
// migrate bias: a scrape target moves to a feed or API the moment one exists.

func tier1Rows() []Row {
	page := func(id, url, lane, note string) Row {
		return Row{
			ID: id, Kind: KindPage, URL: url, Lane: lane, Tier: 1,
			Cadence: CadenceWeekly, Enabled: true, Group: GroupReport02,
			Notes: note + " — " + report02Cite, MigrateBias: true,
		}
	}
	return []Row{
		// Pricing / plan pages, diffed verbatim.
		page("t1-anthropic-pricing", "https://claude.com/pricing", "anthropic",
			"Anthropic pricing/plan page; provider support articles and pricing pages are ground zero for drift and sometimes the only record"),
		page("t1-zai-devpack", "https://docs.z.ai/devpack/overview", "zai",
			"Z.AI coding-plan overview (GLM plan quota mechanics)"),
		page("t1-openai-pricing", "https://learn.chatgpt.com/docs/pricing", "openai",
			"OpenAI plan pricing docs (watch-only candidate lane)"),
		page("t1-minimax-plans", "https://platform.minimax.io/", "minimax",
			"MiniMax token-plan pages (candidate); report-02 names the token-plan pages on this host without pinning a path"),
		page("t1-kimi-coding", "https://kimi.com/coding", "moonshot",
			"Kimi coding + membership pages (candidate)"),
		page("t1-cerebras-code", "https://cerebras.ai/code", "cerebras",
			"Cerebras code plan — the sell-out/restock cycle lives here and was a pricing-page-flip-only event"),
		page("t1-synthetic-pricing", "https://synthetic.new/pricing", "synthetic",
			"Synthetic pricing (candidate)"),
		page("t1-nanogpt-pricing", "https://nano-gpt.com/pricing", "nano-gpt",
			"nano-gpt pricing/blog (candidate)"),
		page("t1-stepfun-plans", "https://platform.stepfun.ai/", "stepfun",
			"StepFun step-plan docs (candidate); report-02 names the plan docs on this host without pinning a path"),
		page("t1-byteplus-codingplan", "https://www.byteplus.com/", "byteplus",
			"BytePlus coding-plan activity page (candidate); report-02 names the activity page on this host without pinning a path"),
		page("t1-xai-pricing", "https://x.ai/", "x-ai",
			"x.ai pricing-class pages (candidate); the pricing path 403s to direct fetches, so the host root is diffed"),
		page("t1-alibaba-codingplan", "https://www.alibabacloud.com/", "alibaba",
			"Alibaba Cloud coding-plan help (candidate); report-02 names the help pages on this host without pinning a path"),
		// Provider release notes / changelogs.
		page("t1-anthropic-release-notes", "https://support.claude.com/en/articles/12138966", "anthropic",
			"Anthropic release-notes support article; there is no first-party RSS for it"),
		page("t1-openai-help-6825453", "https://help.openai.com/en/articles/6825453", "openai",
			"OpenAI help-centre article tracked by report-02"),
		page("t1-openai-help-9624314", "https://help.openai.com/en/articles/9624314", "openai",
			"OpenAI help-centre article tracked by report-02"),
		page("t1-codex-changelog", "https://developers.openai.com/codex/changelog", "openai",
			"Codex changelog"),
		page("t1-zai-release-notes", "https://docs.z.ai/release-notes/new-released", "zai",
			"Z.AI release notes — GLM plan-quota mechanics appear here"),
		page("t1-minimax-release-notes", "https://platform.minimax.io/docs/release-notes/models", "minimax",
			"MiniMax model release notes (candidate)"),
		page("t1-cerebras-changelog", "https://inference-docs.cerebras.ai/support/change-log", "cerebras",
			"Cerebras inference change log (candidate)"),
		page("t1-deepseek-news", "https://api-docs.deepseek.com/news", "deepseek",
			"DeepSeek API news (candidate; the pre-registered designated metered exception should one ever be enabled, S03.6)"),
	}
}

// ── Tier 2: repo canaries, event-driven feeds (report-02 §6) ───────────────
//
// GitHub publishes Atom for releases and commits; it publishes NO public Atom
// feed for issues, so the issues leg report-02 names is covered by the
// releases/commits feeds plus the tier-1 pages rather than by a fabricated URL
// that would decay on its first fetch.

func tier2Rows() []Row {
	feed := func(id, url, lane, note string) Row {
		return Row{
			ID: id, Kind: KindFeed, URL: url, ParserHint: "atom", Lane: lane, Tier: 2,
			Cadence: CadenceContinuous, Enabled: true, Group: GroupReport02,
			Notes: note + " — " + report02Cite,
		}
	}
	return []Row{
		feed("t2-opencode-releases", "https://github.com/anomalyco/opencode/releases.atom", "",
			"opencode releases — the strongest cross-provider canary; its commits surfaced Anthropic's OAuth block weeks before press"),
		feed("t2-opencode-commits", "https://github.com/anomalyco/opencode/commits.atom", "",
			"opencode commits (the issues leg has no public Atom feed at GitHub; commits+releases carry it)"),
		feed("t2-claude-code-releases", "https://github.com/anthropics/claude-code/releases.atom", "anthropic",
			"claude-code releases + CHANGELOG movement; the adopted engine's own drift canary"),
		feed("t2-claude-code-commits", "https://github.com/anthropics/claude-code/commits.atom", "anthropic",
			"claude-code commits (issues have no public Atom feed)"),
		feed("t2-codex-releases", "https://github.com/openai/codex/releases.atom", "openai",
			"openai/codex releases"),
		feed("t2-qwen-code-commits", "https://github.com/QwenLM/qwen-code/commits.atom", "qwen",
			"qwen-code commits; Qwen's free-tier kill notice arrived as a GitHub issue, so this repo front-runs the announcement"),
		feed("t2-gemini-cli-commits", "https://github.com/google-gemini/gemini-cli/commits.atom", "google",
			"gemini-cli commits; the Gemini CLI deprecation mechanics lived in repo issues"),
		feed("t2-grok-build-commits", "https://github.com/xai-org/grok-build/commits.atom", "x-ai",
			"xai-org/grok-build commits (candidate)"),
		feed("t2-modelsdev-commits", "https://github.com/anomalyco/models.dev/commits.atom", "",
			"models.dev commits — new provider/model rows appear as TOML commits before they reach the API"),
	}
}

// ── Tier 3: structured aggregators, weekly API poll (report-02 §6) ─────────

func tier3Rows() []Row {
	return []Row{
		{
			ID: "t3-modelsdev-api", Kind: KindAPI, URL: "https://models.dev/api.json",
			ParserHint: "json", Tier: 3, Cadence: CadenceWeekly, Enabled: true,
			Group: GroupReport02,
			Notes: "models.dev public API — API pricing/capabilities; it does NOT track subscription plans, so it is a model/pricing feed only. Diffed structurally (added/removed/changed keys), never as free text — " + report02Cite,
		},
		{
			ID: PriceRowID, Kind: KindAPI,
			URL:        "https://raw.githubusercontent.com/pydantic/genai-prices/main/prices/data.json",
			ParserHint: "json", Tier: 3, Cadence: CadenceWeekly, Enabled: true,
			Group: GroupReport02,
			Notes: "genai-prices data.json refresh. Diffed against the VENDORED pinned baseline (" + VendoredPricePin + ") and ALWAYS landing as a proposal with effective-dated rows — never an overwrite, never a user-overlay clobber, never an auto-applied table; a billing-regime change never auto-flips a flag. Staleness past ⚙ adoption.price_data_stale_days proposes the LiteLLM-primary fallback — G2 Def.16; Spec S10.3; S16.8",
		},
		{
			ID: "t3-openrouter-blog", Kind: KindFeed, URL: "https://openrouter.ai/blog/feed.xml",
			ParserHint: "rss", Tier: 3, Cadence: CadenceWeekly, Enabled: true,
			Group: GroupReport02,
			Notes: "OpenRouter blog feed; its model list carries stealth aliases (Owl-Alpha→LongCat), making it an early-warning channel — " + report02Cite,
		},
		{
			ID: "t3-usagepricing-activity", Kind: KindPage, URL: "https://usagepricing.com/",
			Tier: 3, Cadence: CadenceWeekly, Enabled: true, Group: GroupReport02, MigrateBias: true,
			Notes: "usagepricing.com activity log — maintained, and it caught the Cerebras cycle — " + report02Cite,
		},
		{
			ID: "t3-artificial-analysis", Kind: KindStudy, Tier: 3, Cadence: CadenceWeekly,
			Enabled: true, Group: GroupReport02,
			Notes: "Artificial Analysis Data API (free tier 1,000 req/day — evals + a per-provider pricing snapshot). Registered but NOT yet a polled row: the endpoint needs an operator-supplied API key, and report-02 pins no URL. It activates through a LoadRows override once the operator supplies both — a fabricated endpoint would decay on its first fetch — " + report02Cite,
		},
	}
}

// ── Tier 4: community keyword feeds, low-noise (report-02 §6) ──────────────
//
// hnrss.org is CONSUMED AS HOSTED and never self-hosted: the project ships no
// license file (S14.6 ¶T2).

func tier4Rows() []Row {
	hn := func(id, query, lane string) Row {
		return Row{
			ID: id, Kind: KindFeed,
			URL:        "https://hnrss.org/newest?q=" + query + "&points=50",
			ParserHint: "rss", Lane: lane, Tier: 4, Cadence: CadenceContinuous,
			Enabled: true, Group: GroupReport02,
			Notes: "hnrss.org keyword feed, points-threshold filtered. HN is same-day for anything Anthropic/OpenAI-shaped. Consumed as HOSTED — hnrss is never self-hosted (no license file) — " + report02Cite,
		}
	}
	return []Row{
		hn("t4-hn-anthropic", "Anthropic", "anthropic"),
		hn("t4-hn-claude", "Claude+Code", "anthropic"),
		hn("t4-hn-openai", "OpenAI", "openai"),
		hn("t4-hn-zai", "Z.ai+GLM", "zai"),
		{
			ID: "t4-localllama", Kind: KindFeed, URL: "https://www.reddit.com/r/LocalLLaMA/.rss",
			ParserHint: "rss", Lane: "local", Tier: 4, Cadence: CadenceContinuous,
			Enabled: true, Group: GroupReport02,
			Notes: "r/LocalLLaMA via /.rss — the local-tier community channel — " + report02Cite,
		},
	}
}

// ── S16.8 standing rows registered by the gates ────────────────────────────
//
// These are REGISTRATIONS: polling machinery, cadence and alert routing are
// S14's, and each row records what it watches and what a hit fires. They carry
// no fabricated URL where the gate text names none.

func standingRows() []Row {
	std := func(id, url, note string) Row {
		return Row{
			ID: id, Kind: KindStudy, URL: url, Tier: 2, Cadence: CadenceQuarterly,
			Enabled: true, Group: GroupS168, Notes: note + " — " + s168Cite,
		}
	}
	return []Row{
		std("s168-pat-collaborator-gap", "",
			"Fine-grained-PAT collaborator gap: watches GitHub PAT capability change; fires → broker/push posture options widen [G2 §Follow-ups; XREF:S13]"),
		std("s168-free-plan-rulesets", "",
			"Free-plan ruleset availability: watches enforced rulesets on Free private repos; fires → a server-side second guardrail beyond the broker (P-T12-1)"),
		std("s168-diffity-maturity", "",
			"diffity maturity: watches project maturity; fires → S13 diff-tooling candidate re-eval"),
		std("s168-jj-compat-matrix", "",
			"jj compat matrix: watches jj/git compatibility; fires → S13 git-machinery re-eval (no jj at v0) [G2 D2.1(e)]"),
		std("s168-xwing-age-plugin", "",
			"X-Wing age-CLI plugin: watches plugin status; fires → broker/backup crypto options [XREF:S11; S13]"),
		std("s168-dbos-parity", "",
			"DBOS TS/SQLite parity: watches fallback health; fires → the durable-execution fallback stays credible"),
		std("s168-mcp-final", "",
			"MCP 2026-07-28 final (Tasks/Extensions): watches the protocol restructure landing; fires → the P-T16-3 dated migration trigger + inbox-adapter seam re-eval [XREF:S03]. This row's cadence is deliberately the standing quarterly review — the DATED trigger is the protocol version pinned in components.lock, not a poll"),
		std("s168-acp-v1", "",
			"ACP v1 line: watches verb-set/transport convergence; fires → an adapter-contract alignment check (Sinet's verbs are already a superset) [S03.1]"),
		std("s168-litestream-1083", "",
			"Litestream #1083: watches silent-replication-failure triage; fires → re-decide the Litestream unit at implementation [G2 D2.5; XREF:S01; S13]"),
		std("s168-redlines-pandiff", "",
			"Python-Redlines + pandiff: watches license/maintenance, verified at the first quarterly pass (the P3 dependency audit, R13-OQ7); fires → S13 doc-diff candidates admitted or dropped"),
		std("s168-dndkit-react-1", "",
			"@dnd-kit/react 1.0: watches 1.0 with React-19-clean peers; fires → re-evaluate the board pick with a dated note [frontend-components-v1]"),
		std("s168-assistant-ui-1", "",
			"assistant-ui 1.0: watches a stability promise or a hostile 1.0, and the 0.x churn rate; fires → re-pin, or the owned-primitives fallback [frontend-components-v1]"),
		{
			ID: "s168-awesome-harness-engineering", Kind: KindStudy, Tier: 2,
			Cadence: CadenceQuarterly, Enabled: true, Group: GroupS168,
			Notes: "awesome-harness-engineering referents: watches BOTH leading referents — `walkinglabs/…` AND `ai-boost/…` — for activity; fires → DROP THIS ROW IF BOTH STALL [G2 Def.15]. One row carries both referents deliberately: the gate ratified a single drop-if-both-stall decision, not two independent watches. The gate text elides the repo paths beyond those prefixes, so no URL is fabricated here; the operator supplies the two concrete feeds through a LoadRows override — " + s168Cite,
		},
		std("s168-crush-fsl-mit", "",
			"Crush FSL→MIT conversions: watches per-version MIT from ~mid-2027; fires → the operator may revisit the patterns-only policy (S16.5) [G2 D2.2]"),
		std("s168-genai-prices-staleness", "",
			"genai-prices staleness: watches for >⚙ adoption.price_data_stale_days without updates while providers reprice; fires → flip to LiteLLM-primary + a manual table. The MEASUREMENT for this row runs on the "+PriceRowID+" refresh job, which reads that ⚙ by dotted key at evaluation time"),
		std("s168-opencode-v2", "",
			"opencode v2: watches GA / a declared server-API break; fires → the bump is held at the S03.3 conformance gate"),
	}
}

// ── The G4 [schema] STUDY row ──────────────────────────────────────────────

func studyRows() []Row {
	return []Row{
		{
			ID: "study-schema-arc-agi-3-harness", Kind: KindStudy,
			URL: "https://schema-harness.github.io", Tier: 4, Cadence: CadenceQuarterly,
			Enabled: true, Group: GroupStudy,
			Notes: "STUDY-class watch row onboarded at S14 watch setup per GATE-4 §[schema]: " +
				"\"[schema] ARC-AGI-3 harness — world-model-as-code (mechanism-as-executable-program) as a candidate future skill/technique pattern for domain workers; " +
				"treat its numbers per report-12 discipline (self-reported, public-set, scaffold-specific). Source: schema-harness.github.io.\" " +
				"The report-12 caveat is binding on any use of its published figures — Research/decisions/GATE-4-spec-review.md",
		},
	}
}

// ── One review row per components.lock entry (S16.4 #9) ────────────────────

func lockReviewRows() []Row {
	out := make([]Row, 0, len(LockEntryNames))
	for _, name := range LockEntryNames {
		out = append(out, Row{
			ID: LockRowID(name), Kind: KindStudy, Tier: 3, Cadence: CadenceQuarterly,
			Enabled: true, Group: GroupLockReview,
			Notes: "components.lock entry " + name + ": the S16.4 #9 watch registration every adopted component must carry. " +
				"Read by the S16.7 quarterly dependency pass through PendingReview; its outputs are PROPOSALS, never silent bumps [S16.7; G2 Def.16]",
		})
	}
	return out
}

// ── Operator override ──────────────────────────────────────────────────────

// LoadRows parses an operator-supplied strict-JSON watch-row set. Unknown
// fields fail LOUDLY (the §14/§15/§17 seed precedent): a typo in a hand-edited
// seed file is a refusal, never a silently ignored line. Every row is validated,
// and duplicate ids are refused.
func LoadRows(data []byte) ([]Row, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var rows []Row
	if err := dec.Decode(&rows); err != nil {
		return nil, fmt.Errorf("watchlist: parse watch-row seed: %w", err)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if err := r.Validate(); err != nil {
			return nil, err
		}
		if seen[r.ID] {
			return nil, fmt.Errorf("%w: duplicate row id %q", ErrInvalidRow, r.ID)
		}
		seen[r.ID] = true
	}
	return rows, nil
}

// WriteSeed renders the in-code seed set as the same strict JSON LoadRows
// parses, for gate review and as the starting point of an operator override.
func WriteSeed() ([]byte, error) {
	out, err := json.MarshalIndent(SeedRows(), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("watchlist: render watch-row seed: %w", err)
	}
	return append(out, '\n'), nil
}
