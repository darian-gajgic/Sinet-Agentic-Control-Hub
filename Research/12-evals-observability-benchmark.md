# 12 — Evals, observability & the benchmark practice

**Topic:** T11 · **Wave:** B2 · **Depth:** FULL · **Written:** 2026-07-17
**Method:** deep-research harness — 5 fan-out search angles (tracing infrastructure & run-story schema / live inspection & queryable history / health watching & alert discipline / watchlist-executor & provider canaries / benchmark methodology & regression evals), ~150 extracted findings, primary-source fetching, then a **3-vote adversarial verification pass over the 12 most load-bearing claim bundles: 36 votes — 26 SUPPORT / 10 PARTIAL / 0 REFUTE**; every PARTIAL's correction is folded into this text. Every statistical claim (binomial power, Wilson intervals, Beta-Binomial posteriors) was additionally **reproduced by direct computation in this session**. All URLs accessed 2026-07-17. Settled inputs consumed and cited, never re-researched: G1 decision record, reports 02 (§6 watchlist sources/cadence), 04 (verification/judge calibration), 08 (event-log state layer), 09 (ledger/receipts). Engine/inference spend: $0 (local computation only).

---

## 1. Scope

Feature-list items covered (per brief T11):

- **11.1** — full audit trail per run; retention (default 6 months) → compaction; summaries/verdicts/receipts forever.
- **11.2 / S2.11 / 15.3** — the added-value benchmark: blind A/B vs direct frontier use, pre-registered per-domain metrics and thresholds, sampled, opt-in budget — the gate that blocks all expansion.
- **S2.1–S2.11** — traceability, live inspection, verdict records, human-decision records, cost observability (with 3.4/S2.5), routing explainability (with 7.7), self-health watching on local models, outside-world drift detection, auditable learning, queryable history.
- **5.7** — rubrics stay falsifiable via the benchmark; **7.3** — model-change revalidation needs executable eval sets.
- **Addendum (report 02 OQ#7, assigned by coordinator):** the S2.8 watchlist **executor** architecture — the source list and cadence are settled in report 02 §6; this report designs what runs them.
- **Inherited assignments:** report 04 OQ6 (benchmark hooks: planted-defect suites, golden-set governance, judge-drift revalidation runbook — T11 owns the standing practice) and report 09 OQ3 (done-directly formula ratification — resolved as a recommendation in §4.6).

Binding constraints researched within: Operating reality (all background intelligence on local models — S2.7 costs no allowance; ≤8B at v0 per G1/P9), 11.3 (raw traces stay local; only summaries/verdicts/receipts enter snapshots), D4/D5 (observed signals only; two currencies), D7 (checkpoint rows are the trace backbone), adopt-don't-fork. G1-ratified architecture treated as fixed: dual substrate behind Sinet's adapter; **the platform-owned SQLite-WAL append-only event log + run-lifecycle FSM is the observability substrate** (report 08); engine-native subagents disabled (sole-controller rider).

Harvest-map items to verdict: **N21** (benchmark methodology), **N16** (routing telemetry — extending report 09's verdict), **C4** (window surfacing — extending report 09's verdict).

---

## 2. Current state of the art (mid-2026)

### 2.1 Tracing: the standard isn't ready, the platforms are oversized, and the field converged on Sinet's own answer

**OTel GenAI semantic conventions are still not stable.** As of today the conventions carry "Development" status throughout, moved to a dedicated repo in 2026 (`open-telemetry/semantic-conventions-genai`) which has **zero published releases** and an explicit right to rename attributes without a major version bump [1, 2, verified 3/3]. What *is* settled in practice: the span taxonomy — `gen_ai.operation.name` ∈ {`chat`, `execute_tool`, `invoke_agent`, `create_agent`, …} with token-usage and duration metrics — and real adoption pressure (the GenAI SIG names Claude Code, OpenAI Codex, and VS Code Copilot as emitters; Sentry, Honeycomb, MLflow, Langfuse, Traceloop map to it) [1, 3]. Design consequence: **mirror the vocabulary, never couple the schema** — name event fields so a mechanical projection to `gen_ai.*` spans stays trivial, and treat OTLP as an export format, not a storage decision.

**The self-hosted trace-platform market is sized for volumes 3–5 orders of magnitude above a household — and is churning.** Verified footprints and status [4–10, verified 3/3]:

| Platform | Self-host footprint | License | Status mid-2026 |
|---|---|---|---|
| Langfuse v3 | ~6 services (web, worker, Postgres, ClickHouse, Redis/Valkey, S3/MinIO); OTLP ingest at `/api/public/otel` (HTTP only) | MIT (+ gated EE) | **Acquired by ClickHouse 2026-01-16**; stated commitment to stay MIT |
| Arize Phoenix | **single container, SQLite default**; OTLP-native | Elastic License 2.0 | Active (17.25.0, 2026-07-11) |
| Helicone | 4 containers | MIT/Apache core | **Acquired by Mintlify 2026-03-03; maintenance mode** — no new features |
| SigNoz | app + collector + ClickHouse + Keeper + Postgres; ≥4 GB RAM | Apache-2.0 + EE | Active; general APM, not agent-aware |
| Laminar | 5 services (incl. ClickHouse, Quickwit) | Apache-2.0 | Active; YC-stage |
| MLflow 3.x tracing | `mlflow server`, **SQLite default backend**; spans ≈ OTel objects | Apache-2.0 | Active |

Two readouts. First, **the credible lightweight options converge on SQLite themselves** (Phoenix, MLflow; an HN-stage entrant markets "no Postgres/no Redis, single SQLite file" as the product [11, C-tier single-source]) — the market validates the G1-ratified substrate, not platform adoption. Second, **ownership churn is a live risk class**: two of the field's best-known names changed hands within six months of this report, one into maintenance mode. A system of record must not carry another company's M&A risk (new problem P-T11-4).

**Practitioner prior art for "SQLite is the trace store" exists in exactly Sinet's shape**: Simon Willison's `llm` CLI logs every model call to a documented SQLite schema (conversations/responses with prompt/response JSON, token fields, durations; tool-call audit tables; Datasette for queries) [12]. Academically, the central open problem is stated as "the lack of unified trace schemas for LLM agents" — there is no standard to adopt, only proposals [13, 14].

**Retention practice: the industry ships delete, not distill.** Langfuse — indefinite by default, per-project hard-delete policies; Phoenix — infinite by default, N-day auto-delete; SigNoz — ClickHouse TTL [15, 16]. **No mainstream platform ships rollup/summarization of old traces.** The blessed OTel pattern is the closest analog: compute aggregates (spanmetrics) *before* tail-sampling/deleting raw spans so derivatives survive raw-data loss [17]. LLM-summarization as a retention tier has prior art only *inside* agent frameworks (context compaction), with one documented hazard: summarization quality collapses on very long low-signal histories — so compaction must be per-run and incremental, never a bulk pass over months of trace [18]. Sinet's 11.1 design (6-month raw + forever-summaries) is therefore novel as a shipped feature but is the natural composition of two proven patterns.

### 2.2 The run story: what the engines already emit, and the schema patterns worth stealing

**Both v0 engines emit nearly everything S2.1–S2.4 needs; the adapter only has to persist it.** Verified against current primary docs [19, 20, verified 3/3]:

- **Claude lane** (`claude -p --output-format stream-json`): `system/init` first event (model, tools, capabilities array); `system/api_retry` with typed error categories (`rate_limit`, `overloaded`, `billing_error`, `authentication_failed`, …) — the T08 five-class classifier's raw feed; assistant/user messages with `parent_tool_use_id` lineage; final `result` envelope (`total_cost_usd`, `modelUsage`, `num_turns`, `permission_denials[]`); token deltas via `--include-partial-messages`.
- **Z.AI lane** (`opencode serve`): typed SSE bus at `GET /event`, first event `server.connected`; `session.idle/error/compacted`, `message.part.updated` (streaming), `permission.asked/replied`, `question.asked/replied` — human decisions and compaction events are first-class bus events. **Confirmed gap:** the SSE stream ignores `Last-Event-ID` — no replay on reconnect; the fix PR was closed "not planned" (#25657, `/global/event`) [21]. The engine cannot be the durable event source; Sinet's adapter-persisted log is (consistent with P-T01-1).

**Schema patterns from shipped systems** [22–26]: OpenHands is the strongest architectural precedent — one append-only chronological event log as the single source of truth, everything else reads/appends, "replay, fork, and full audit trails come for free" (matching report 08's ratified design). Claude Code transcripts are parent-pointer event lists (`parentUuid`), not span trees. LangSmith's `dotted_order` (a sortable `{time}{uuid}` path key encoding tree position + order in one string) is a cheap trick for ordered-tree queries. Langfuse's model — scores attachable to traces, observations, or sessions — is the closest shipped analog to "verdicts and human decisions as first-class rows." For routing records, **LiteLLM's auto-router is the pattern to copy verbatim**: every routing decision emits one greppable record `{cause enum, tier, score, signals, routed_model}` [27]; OpenRouter's auto-router, by contrast, discloses *which* model answered but never *why* [28] — the field's floor is low, and Sinet's S2.6 requirement (plain-language reason, analyzable over time) slightly exceeds the best shipped practice.

### 2.3 Live inspection: SSE over an event cursor is both the consensus and, for Sinet, nearly free

- **SSE is the de facto transport for LLM/agent streaming** (≥4 independent sources; OpenAI and Anthropic APIs stream SSE natively; opencode's own multi-client architecture is one SSE bus; PocketBase — the canonical small self-hosted SQLite app — ships realtime as SSE; MCP's Streamable HTTP is POST + SSE) [29–31]. WebSocket buys nothing when client→server actions are ordinary POSTs.
- **Resumability is spec-blessed and trivially cheap here.** MCP Streamable HTTP mandates per-stream SSE event IDs + `Last-Event-ID` replay, with an `EventStore` interface (`storeEvent`/`replayEventsAfter`) in the SDKs [31, verified 3/3]. Cloud implementations report weeks of work because stream state lives nowhere durable (Vercel's resume requires Redis; Cloudflare's requires Durable Objects) [32, 33] — **the entire cost class disappears when the source of truth is an append-only log with `event_seq`**: resume = `SELECT … WHERE event_seq > ? ORDER BY event_seq`. The opencode #25657 record is the cautionary tale for shipping SSE without replay [21].
- **Operational pitfalls are well-documented**: HTTP/1.1's 6-connections-per-origin limit (mitigate: one multiplexed stream per client, or HTTP/2), proxy buffering (`no-transform`, buffering off), 15–30 s keepalive comments [34, 35].
- **Progress semantics converged across four independent taxonomies** (MCP progress notifications: monotonic `progress` + optional `total` + message; A2A task states with `input-required` as a first-class interrupted state; AG-UI's event taxonomy — 17 core types at launch, ~28–30 in the current spec — lifecycle/step/tool-call/state-snapshot-then-delta; Copilot coding-agent session logs) [36–39, correction folded]. Common denominator: **run lifecycle + step boundaries + current-tool-with-args + a paused/needs-input state + monotonic counters (tokens, cost, elapsed). Nobody ships "percent complete" for open-ended agent work.** The AG-UI StateSnapshot-then-StateDelta reconnect pattern maps 1:1 onto "SELECT current state, then tail events after seq."

### 2.4 Health watching: deterministic-first with published thresholds; the local model is a disambiguator, not a detector

**Every shipped loop/stall detector's first line is deterministic, with thresholds Sinet can copy** [40–43, verified 3/3]:

| Detector | Mechanism | Threshold |
|---|---|---|
| Gemini CLI tool-call loop | SHA-256 of (tool name + serialized args), consecutive count | **5** identical |
| Gemini CLI content "chanting" | 50-char chunk recurrence in a 5000-char window; disabled inside code blocks/tables/lists (known FP source) | **10** occurrences |
| Gemini CLI LLM double-check | second-stage only, after **30** turns, every 5–15 turns, prompted as a diagnostic agent over the last 20 turns; >0.9 confidence triggers second-model confirmation | — |
| OpenHands StuckDetector | same action→same observation **4×**; same action→error **3×**; alternating two-pair ping-pong **6** cycles; monologue **3×** | all deterministic |
| Harness practitioner norm | fingerprint (tool, result-preview) hashing | 3 in a row; step/cost/time ceilings wired as stop conditions |

**The false-positive record is the design driver.** Gemini CLI FP'd on legitimate edit→build→edit cycles; Google's mitigation was a *session-scoped user dialog to disable the detector* (PR #8231), not a better killer [41]. OpenHands' canonical FP: agents legitimately polling long-running processes killed as "stuck" [43] — the direct evidence base for the ratified pause-and-flag policy (G1 D1.3). Base rates justify the investment: in MAST's 1,600+ annotated failure traces, **Step Repetition is the single largest failure mode (15.7%)**, with unawareness-of-termination second (12.4%) — the top classes are exactly the trace-visible ones [44].

**What genuinely needs a model, and what size.** Supervised log-anomaly detectors (LogLLM class, F1 0.9+) train on 37k–460k labeled sequences per dataset — inapplicable at household scale [45]. Where small local models demonstrably work without training: **triage/severity classification with retrieval** — Qwen3-4B hit **95.64%** on real journalctl severity classification with RAG; reasoning-tuned small models were actively harmful (>228 s/entry at <10% accuracy) [46, single paper, flagged]. Structured-output reliability at 7–8B is a solved problem with grammar-constrained decoding; Q4 throughput on 12 GB-class hardware runs ~60–70 tok/s, so a watchdog verdict costs ~2–3 s GPU time [47, 48]. **CodeAD is the shape to steal for evolution**: LLMs synthesize deterministic Python detection rules *offline*; rules run online — +3.6% F1 over SOTA at ~4× speed, total LLM cost <$4/dataset [49, single paper but consistent with the compile-don't-infer trend].

**Spend anomalies need no model at this scale.** LangSmith alerts, LiteLLM alerting, and Langfuse spend alerts are all static-threshold products (LiteLLM: hang-detection default 300 s; outage alerting at 5/10 errors per 60 s window; a 86,400 s dedup TTL + digest mode as anti-spam) [50–52]. AWS Cost Anomaly Detection — the ML gold standard — **requires ≥10 days of history per monitored dimension** before detecting anything [53]; Datadog's learned monitors need ~3× the seasonality period of history ("at least three times as much historical data as the chosen seasonality", verbatim) [54]. Correction from verification: Datadog does *not* flatly recommend static thresholds for sparse metrics (it recommends its `basic` algorithm and wider windows) — but the cold-start literature does: the accepted framing is zero-shot rules/priors set before data exists, adapting slowly as observations arrive [55]. **At household cadence, operator-set fixed thresholds and zero-history count rules are the methodologically correct starting point, not a compromise.**

**Alert discipline is existential at bus factor 1.** Google SRE's budget: at most ~2 attention-demanding incidents per shift before investigation quality collapses [56]; the Ewaschuk canon: every page urgent/important/actionable/real; err on the side of *removing* noisy alerts [57]. What indiscipline produces: 46–63% FP/ignore rates in real alert systems [58]. Shipped anti-fatigue mechanics worth copying: dedup TTL + digest mode (LiteLLM), preview-against-history before enabling (LangSmith), session-scoped operator suppression whose usage is itself the tuning signal (Gemini CLI) [50, 40, 41].

### 2.5 Outside-world drift watch: the executor tooling exists — except the API-canary layer

The source list and cadence are settled (report 02 §6). What executes them:

**Page-diff tier (T1): changedetection.io is the whole architecture, shipped.** Apache-2.0, actively maintained (v0.55.8, 2026-07-13); CSS/XPath/JSONPath/jq region filters, verbatim text diffs with history, Playwright fetcher + "Browser Steps" for JS-walled pages, Apprise notifications including `json://` webhook POST; **a full REST API** (watch CRUD, `/history/{timestamp}` snapshots, `/difference/{from}/{to}` diffs) so the watch list is config-as-code; and — decisive, verified 3/3 against the changelog and source tree — **native "AI change detection rules" and AI change summaries that run against self-hosted OpenAI-compatible endpoints (Ollama, vLLM, LM Studio, llama.cpp)** in the open-source app (0.55.1–0.55.5; `LLM_FEATURES_DISABLED` exists to *hide* the feature on their hosted service) [59–61]. First-pass "does this diff matter?" triage on Sinet's free tier is a configuration exercise, not a build. The Python alternatives (urlwatch, and its actively-developed fork webchanges) are cron CLI tools without daemon/API, and webchanges' AI differ is Gemini-API-only [62]; Huginn is heavyweight and of uncertain maintenance [63, flagged].

**Feed tier (T2/T4):** Miniflux v2.3.2 (Apache-2.0, single Go binary, PostgreSQL-only) ships a native feed→webhook pipe — new entries POSTed as JSON, HMAC-SHA256-signed (`X-Miniflux-Signature`); the webhook is global, not per-feed [64]. RSS-Bridge covers feedless-but-list-shaped pages [65]. Corrections from verification: **hnrss has no license file at all** (source-available Go, not open-source — prefer consuming hnrss.org or the Algolia HN API directly) [66]; Reddit cut feed rate limits in mid-2026 — r/LocalLLaMA polling needs authenticated feed URLs [67]. The genuine question is whether ~15 feeds justify a Postgres-backed reader at all (§3-D, §4.5).

**API-behavior canary tier: no off-the-shelf home — Sinet-native probes composed from verified parts.** Pact is a documented anti-fit for third-party APIs (consumer-driven contracts pay off only when the provider runs verification) [68]. The composable parts: **oasdiff** (diffs OpenAPI spec snapshots, ~500 change types classified breaking/non-breaking, exit-code CI shape) [69]; **schemathesis** (schema-conformance property tests against live endpoints where specs exist; v4.22.4, MIT, active) [70]; **scheduled promptfoo evals as the published LLM-provider drift practice** (daily cron against live prompts, webhook alerts on score drops) [71]; and the standout cheap primitive — **one-output-token log-probability tracking** (arXiv 2512.03816: a permutation test on average token logprobs detects provider-side changes as small as a single fine-tuning step at ~1000× lower cost than prior audits; ~$0.14/year per lane; TinyChange benchmark) [72, verified 3/3]. Applicability caveat (Sinet-specific, this report's analysis): logprob tracking needs a `logprobs` parameter — available on OpenAI-compatible API lanes and local models, **not** on subscription CLI surfaces; behavioral canaries (scheduled fixed-prompt evals) are the fallback for wrapped-CLI lanes. Auth canaries follow documented OAuth-monitoring practice: a scheduled cheap *real* request with error-class discrimination (revocation vs expiry vs limit) — exactly P-T17-1's requirement [73]. CLI drift: no renovate-for-CLIs exists; version pinning + release-feed watching (T2) + Sinet's conformance suites on version change (P-T01-2, settled) is the composition; nvchecker is the utility if needed [74].

**Alert routing/dedup:** Apprise (BSD-2, v1.9.8) is the delivery multiplexer; ntfy/Gotify the self-hosted phone-push transports. **None of them dedup, group, or suppress storms — only Prometheus Alertmanager does, and it drags a Prometheus-shaped stack in** [75, 76]. Since drift alerts already terminate in Sinet's approval inbox, fingerprint-per-incident-window dedup belongs there (consistent with T08's once-per-storm rule).

**Operational decay is structural.** TLS/JA3 fingerprinting flags non-browser fetchers before HTML is served (changedetection.io's own tutorial + long-running evasion issue); publishers are withdrawing from the Wayback Machine over AI-scraping concerns (Guardian, FT), and Google cache is dead as a fallback [77, 78]. The settled source list already hit 403s (freedesktop, help.openai.com). Consequence: per-source fetcher escalation (plain → Playwright → flag decayed) plus a standing bias to migrate any scrape target to a feed/API equivalent the moment one exists (new problem P-T11-3).

### 2.6 The benchmark: Arena mechanics at household n — what statistics can honestly support

**Pairwise blind comparison methodology is mature** [79–81]: anonymized pairs, position randomized (or double-judged with swapped positions), **ties and "both bad" allowed** — forced choice "artificially eliminates" genuine preference uncertainty, and draw rates vary systematically by task category [80]; identity revealed only *after* the vote. Known human-rater confounds, measured: **length is the dominant style factor** in LMArena's style-control regression (coefficient 0.249 vs 0.019–0.031 for markdown/bold/headers; style control moved models 5–12 rank positions) [81]; and the noise floor is real — two *identical* checkpoints submitted under different aliases scored **17 Elo points apart** on the Arena ("The Leaderboard Illusion", which also documents what unregistered evals permit: 27 privately tested variants, selective disclosure) [82, verified 3/3].

**What small n supports — computed exactly this session (V-tier, reproduced from first principles):**

| Question | Answer (exact binomial, one-sided α=0.05 vs 50/50) |
|---|---|
| n for 80% power at true preference 60/40 | **158** |
| … at 65/35 | **69** (reject at ≥42 wins) |
| … at 70/30 · 75/25 · 80/20 | 37 · 23 · 18 |
| n=20: rejection needs | **≥15/20 (75%)** |
| n=20 power vs true 65/35 · 75/25 · 85/15 | **0.25 · 0.62 · 0.93** |
| n=30: rejection ≥20/30; power vs 65/35 · 75/25 | 0.51 · 0.89 |
| Wilson 95% CI, 14/20 wins | [0.48, 0.85] — includes 50% |
| Beta-Binomial (uniform prior) P(p>0.5): 7/10 · 14/20 · 20/30 · 35/50 | 0.89 · **0.96** · 0.97 · 0.998 |

Readout: **a household quarter (n≈20–30) reliably detects only large effects (≥75/25); a month detects only catastrophic ones (≥85/15).** The predecessor's failure (28/50 vs 43–44/50) was a catastrophic-scale effect — detectable at n≈15–20. Two honest framings exist for the standing question: fixed-batch exact tests aimed at catastrophic regressions, or — better for a drip-feed of 5–30 pairs/month — **anytime-valid/Bayesian running rules** (confidence sequences / mSPRT-family), which permit continuous monitoring with optional stopping and no α-inflation and are deployed practice at Adobe, Amazon, and Netflix [83, 84]. Their cost: ~20–40% wider intervals than fixed-n. Ties reduce effective n and must be planned for (conditional sign test; impact "large" at n≤50) [85].

**Pre-registration practice:** 2025–26 eval papers commit confirmatory metrics, thresholds, and analysis rules to version control or OSF *before* data exists, with an explicit "thresholds are not re-tuned in response to results" clause [86, 87]; frontier safety frameworks are the industry analog (pre-committed thresholds triggering predefined actions) [88]. Git-commit-as-preregistration is the solo-maintainer form. The Leaderboard Illusion is the demonstration of why: post-hoc selection freedom inflates scores even with honest intentions [82].

**Blinding will partially fail — plan for measurement, not pretense.** Annotators who frequently use LLMs detect AI-generated text near-perfectly (majority of 5 experts: **1 of 300** misclassified, beating *most* commercial detectors — Pangram comparable — robust to paraphrase/humanization) [89, verified 3/3, correction folded]; model style fingerprints survive prompted style changes [90]. Sinet's case is milder — both arms may run the same frontier model, so the tell is platform *formatting/scaffolding* style — and presentation-time style normalization is **not** an established shipped practice (Arena controls style post-hoc via regression covariates, infeasible at household n) [negative finding, multi-search]. The honest protocol: render both arms through one uniform template (strip formatting tells; never truncate length), **and measure blindness** — ask the requester to guess which arm was the platform's, log guess accuracy, and report it next to the win rate (new problem P-T11-1).

**The direct arm cannot be frozen.** Provider retirement cycles bound any pinned baseline (Anthropic ≥60 days notice; OpenAI 3–6 months for API snapshots) — and Sinet's direct arm runs on the requester's *consumer subscription*, where models auto-upgrade and cannot be pinned at all [91, 92]. No standing benchmark solves a moving baseline; the practice is **epoch stamping**: record both arms' exact model identity per pair, analyze within epochs, never pool win rates across a baseline change [93]. Scaffold-vs-bare-model ablations are standard in 2025–26 agent papers (fairness controls: same model version, same context access, declared turn-parity) [94]; **no published prior art exists for "platform vs the user's own subscription surface" specifically** — Sinet is mildly novel here, and the parity gaps (consumer surface may route to different model versions; the direct arm lacks household memory — arguably the thing being tested) must be *declared* in the pre-registration, not hidden (new problem P-T11-2).

### 2.7 Regression evals: the harness market consolidated — and one incumbent is shutting down

Verified landscape [95–99, verified 3/3]:

- **promptfoo** — MIT, extremely active (v0.121.19, 2026-07-14), YAML-first matrix comparison, model-graded + judge-free assertions, local models via Ollama, CI-native. **Acquired by OpenAI (announced 2026-03-09)** with a public "remains open source under the current license" commitment. OpenAI's own Evals-deprecation guidance points migrants to promptfoo.
- **DeepEval** — Apache-2.0, active (v4.1.0, 2026-07-12), pytest-native (evals fail CI on threshold breach), configurable/pinned judge models, golden-dataset support; cross-run comparison pushes toward its cloud, but the OSS core runs standalone.
- **Inspect AI** (UK AISI) — MIT, very active; safety-grade rigor, heavier learning curve.
- **OpenAI Evals platform — shutting down**: read-only 2026-10-31, gone 2026-11-30.
- **Braintrust** — self-host is Enterprise-only; wrong shape for one maintainer.

**Model-change revalidation is now documented vendor practice**: Anthropic's migration guide is per-model breaking-change checklists + behavioral-shift retuning + explicit verification steps; OpenAI's guidance: comparative evals on "representative tasks from your own application," one variable at a time; AWS's model-agility playbook adds shadow mode → canary → rollback gates [91, 92, 100]. Planted-defect testing of judges has nothing fundamentally new since report 04's settled method (golden sets, TPR/TNR); one useful 2025 addition formalizes *reporting* judge-error-corrected scores — directly compatible with small-n honesty [101, single-source]. The promptfoo-owned-by-OpenAI wrinkle matters for a benchmark whose subject includes OpenAI-competitor models: MIT forkability and pinned versions are the mitigation, and the *scoring data* must live in Sinet's store, not the tool (P-T11-4).

### 2.8 Queryable history: canned queries first; open text-to-SQL is the escalation path

- **On small known schemas, the problem is nearly solved — with caveats.** BIRD (moderately messy, known schema): top single-model ~80%, best pipeline ~82%, human 92.96% [102]. Under human-review grading on simple schemas, frontier models reach ~94–95% — "good data modeling *is* the semantic layer" [103]. Enterprise-scale unknown schemas are a different regime: Spider 2.0's 2024-era paper numbers were 5–31%; the mid-2026 leaderboard's self-reported tops have since risen to ~66–97% per track (correction folded — cite dated) [104]. Benchmark annotation error rates of 52–63% mean leaderboard decimals are noise [105].
- **Small local models are competitive on known schemas but non-deterministic.** Arctic-Text2SQL-R1-7B: 68.5% BIRD-test, open weights, only ~3.4 points behind its 32B sibling [106]. The most Sinet-shaped practitioner result: Qwen3-4B on a documented small schema hit **90–95% accuracy — but "the same question, asked twice, can produce correct SQL one run and wrong SQL the next"**; the author landed on deterministic query compilers with the LLM doing intent-understanding only [107, single-source but highly specific]. Grammar-constrained decoding is mature (XGrammar <40 µs/token, default backend in vLLM; GBNF in llama.cpp) and guarantees syntax, never semantics [108].
- **The guardrail stack is settled consensus**: read-only at the connection level (never prompt-enforced), allowlisted *views* over raw tables, single-statement parse check rejecting DDL/DML, hard row limit + timeout, audit-log every generated query [109]. LLM-facing DSL middlemen (Malloy-class) documented to backfire — models write SQL better than niche DSLs [110, single-source].
- **SQLite analytics at event-log scale is proven territory**: generated columns over JSON payloads for indexing; FTS5 trigram search at 18.2M rows in 14 ms (2.4 GiB index cost); DuckDB's official `sqlite` extension ATTACHes the live DB read-only as an analytics sidecar — a documented, zero-migration option if ever needed [111–113].

---

## 3. Candidate approaches

**A. Observability substrate.**
- **A1 — Adopt a trace platform (Langfuse/Phoenix-class) as the system of record:** rejected. Footprints are sized for 10⁶–10⁹ spans (Langfuse: 6 services incl. ClickHouse); the platforms can't hold Sinet's non-trace record (verdicts, human decisions, receipts, effects) without becoming a second source of truth beside the D7 event log; ownership churn is live (P-T11-4); and 11.3's traces-stay-local rule plus per-person cost queries are platform features, not dashboards.
- **A2 — The G1 event log IS the trace store, projected into views (chosen):** the run story is already being written (one checkpoint row per paid call + typed events per report 08); T11 adds event *types* and *views*, not a store. Cost: Sinet owns its dashboards — mitigated by A3.
- **A3 — OTLP projector as the optional escape hatch (designed-in, not v0):** a mechanical replay of the event log as `gen_ai.*`-shaped OTLP spans lights up Phoenix (single container, SQLite, OTLP-native) for ad-hoc deep-dive UI in an afternoon — kept possible by naming event fields compatibly, never by coupling.

**B. Live inspection.** SSE on the control plane, `event_seq` as the SSE event id, `Last-Event-ID` + explicit `?after_seq=` resume, snapshot-then-tail per the AG-UI pattern (chosen) vs WebSocket (rejected: no bidirectional need) vs client polling of `GET /events?after=` (kept as the degenerate same-cursor fallback — transport becomes a client detail, not an architecture fork).

**C. Watchdog architecture.** Three tiers (chosen): T0 deterministic counters over the event log (loop/ping-pong/error/silence/spend — published thresholds, §2.4); T1 local-model disambiguator + triage annotator, invoked only on T0 triggers (Gemini CLI's template), grammar-constrained verdicts; T2 pause-and-flag cards with evidence, "resume — I was wrong," and per-rule suppression whose usage auto-tunes thresholds. Post-v0 evolution: CodeAD-shaped offline rule synthesis. Rejected: LLM-first detection (supervised detectors need 10⁴–10⁵ labeled sequences; per-event model calls violate the free-tier economy for nothing), learned baselines at v0 (below every documented data floor), auto-kill (D1.3, FP record).

**D. Watchlist executor.**
- **D1 — Fully Sinet-native watcher:** rejected — re-implements changedetection.io's decade of fetcher/diff/anti-bot machinery plus a shipped local-LLM triage layer.
- **D2 — Adopt changedetection.io + Miniflux + Sinet-native canaries (angle recommendation):** strongest adopt posture; costs a PostgreSQL instance for ~15 feeds.
- **D3 — Adopt changedetection.io; native feed poller on the ratified scheduler; Sinet-native canaries (chosen, lean):** changedetection.io carries T1 (the genuinely hard part — JS walls, diffs, first-pass LLM triage, REST API config-as-code); T2/T4 feeds are structured data consumed by a small poller (conditional GET, hash dedup) on the scheduler Sinet already owns, feeding the same local-model classifier — no Postgres, no second reader. Miniflux is the documented fallback if feed handling proves gnarly (its webhook shape drops in cleanly). Canary probes (auth, conformance, behavioral eval, logprob-where-available) have no off-the-shelf home in any case. Operator ratifies D2 vs D3 (§7).

**E. Benchmark statistical protocol.** E1 fixed quarterly exact-binomial tests (honest but only catches catastrophic effects at achievable n) vs **E2 — anytime-valid running rule + annual pooled check (chosen)**: Beta-Binomial posterior updated per pair with a pre-registered alarm and gate rule; exactly matches drip-feed accrual, is the deployed industry practice for this regime, and degrades gracefully. E1's quarterly readout is kept as a reporting view, not a decision rule.

**F. Regression-eval harness.** promptfoo (chosen: MIT, most active, local-model support, judge-free assertions for deterministic checks; OpenAI ownership mitigated by version pinning + forkability + scores stored in Sinet's DB) vs DeepEval (pre-registered alternative; pytest-native CI gating, Apache-2.0, no acquirer conflict) vs Inspect AI (kept for future safety-grade needs). Custom pytest-only: rejected as primary — re-implements matrix running and judge plumbing the tools ship.

**G. Queryable history.** Layered (chosen): deterministic views/rollups for every S2.5 cost query → canned parameterized query catalog selected+filled by the local model (grammar-constrained) → open SQL generation as the escalation tier under the full guardrail stack, flagged as lower-confidence in the answer. Open-ended text-to-SQL as the default: rejected (run-to-run nondeterminism at 4–8B; the assistant must not be a slot machine over the household's receipts).

---

## 4. Recommendation for Sinet

**One integrated design: the D7 event log is the single observability substrate — everything in S2 is an event type, a view, or a subscriber on it; the watchdog and watchlist run on the free tier; the benchmark is a pre-registered protocol whose statistics match household volume.**

### 4.1 Trace layer = event-log completion (S2.1–S2.4, S4.6, 7.7)

Report 08's schema already carries runs/events/checkpoints/asks/effects. T11 completes the **event-type contract** so the run story is total:

- `stage.started/finished` (plan-stage lineage), `tool.called/completed` (name, args-digest, duration, artifact refs), `helper.spawned` (D6 reason — already ratified), `artifact.produced` (ref + hash).
- `verdict.recorded` (S2.3): rubric id+version, per-criterion binary results, findings `[F1..Fn]` with anchors, blocker/note split, round number, judge model id + its golden-set error rates at judging time (report 04's context-attachment rule).
- `decision.recorded` (S2.4): who, what card, what answer, when — the asks table projected as events; approvals also live in the effects journal (settled).
- `knowledge.injected` (S4.6): the R07 injection manifest — registry ids + versions of every knowledge slice a run received.
- `routing.decided` (S2.6/7.7): **{cause enum, score, signals, routed_model, effort_mode, plain_reason}** — the LiteLLM pattern extended with the spec's plain-language reason. One row per routed call; a periodic view joins routing causes to outcomes (rework rates, verdicts, receipts) so routing quality is auditable over time (the N16 habit, now with a field-validated storage shape).
- Field naming mirrors `gen_ai.*` vocabulary (operation kind, model, token counts, tool name, parent linkage) so the **A3 OTLP projector stays mechanical**; no OTel dependency ships at v0 (the conventions are Development-status with renaming rights reserved).

Payload discipline is settled (report 08: 64 KB caps, refs-not-blobs, validate-before-persist). LangSmith's `dotted_order` is adopted as a generated column for ordered-tree queries.

### 4.2 Retention → compaction (11.1, 11.3)

- **At run end** (not later): write the **run summary** — a compact, structured story (objective, stages, tool-call counts, verdicts, decisions, receipts, final state) generated per-run by the local model + deterministic aggregation. Incremental-per-run is the documented safe shape; bulk summarization over months of trace is the documented failure shape.
- **At the 6-month horizon (⚙, per-user 13.4):** a scheduled compaction pass strips bulky event payloads and transcript copy-asides, keeping forever: run summaries, verdict records, decision records, receipts, routing records, drift events, benchmark records. This is the OTel aggregate-before-drop pattern generalized. Deletion is logged as an event (the audit trail records its own compaction).
- **11.3 boundary enforced by construction:** the snapshot exporter (T12) reads an allowlisted view containing only the keep-forever set; raw payloads are structurally unreachable from it. The event-log growth watch (P-T07-5) feeds S2.7 with WAL-size and table-size meters.

### 4.3 Live inspection (S2.2, S3.9, S1.3)

One SSE endpoint on the control plane: per-client single multiplexed stream, topic-tagged events, SSE `id:` = `event_seq`, `Last-Event-ID` + `?after_seq=` both honored (resume = one indexed SELECT); reconnect = state snapshot from DB, then tail (AG-UI pattern). Keepalive comments every 15–30 s; `no-transform`/buffering-off headers; HTTP/2 via the reverse proxy for multi-tab. Client polling of the same cursor is the sanctioned fallback. **Progress semantics (no percentages):** task cards show FSM state incl. first-class *waiting-on-human* and *parked-until* (C4 meter), current stage, current tool + args digest, monotonic counters (tokens, API-equivalent cost so far, elapsed, steps), and the last activity line — the four-taxonomy common denominator. Sinet's derived `wedged` state (report 05) surfaces here too, which exceeds every shipped product's progress vocabulary.

### 4.4 The watchdog suite (S2.7, 3.7, 4.6)

**Tier 0 — deterministic, always-on, zero-cost** (counters over the event log, thresholds as ⚙ settings seeded from shipped prior art): tool-call loop = signature hash repeated **5×** consecutive; ping-pong = 2-pair alternation **6** cycles; error-loop = same action→error **3×**; silence = no event past the run-type's heartbeat budget (echoes T08's WatchdogSec shape); spend = per-run ceilings (settled) + daily per-person total > ⚙3× trailing 14-day median (activated only after 2 weeks of history — below that, ceilings only); suspicious-completion = run ended with anomalously low tool/verification activity for its class (the silent-failure class from the taxonomy literature [43-class, 114]). No learned baselines at v0 — AWS/Datadog floors and cold-start research all say fixed thresholds are correct at this scale, and G1's settings-not-constants rider makes every number operator-tunable.

**Tier 1 — local-model disambiguator (free tier, ≤8B per P9):** invoked *only* on Tier-0 triggers, never per-event; Gemini CLI's diagnostic-agent template (last N turns + original request → "productive batch work or true loop?"), grammar-constrained verdict {loop, productive, unclear} + confidence + one-line evidence; RAG over the run's own recent events (the pattern that put Qwen3-4B at 95.6% on log triage). Verdict annotates the alert — it never gates and never kills.

**Tier 2 — pause-and-flag (D1.3, settled):** contain (stop admitting the next call) → park at checkpoint → card. Every card carries the trigger evidence (which signature, which counts, spend vs baseline), the Tier-1 annotation, **"resume — I was wrong"** as a first-class action, and **"suppress this rule for this run/worker"** whose usage is logged as the tuning signal: a rule dismissed twice proposes its own threshold raise as an admin card (Ewaschuk's remove-noisy-alerts doctrine, operationalized).

**Inbox discipline (the S2.7 contract with one reader):** two severities — *flag-now* (stall/loop/spend/auth-canary on active runs) and *daily digest* (everything else); dedup one alert per (run, anomaly class) with updates folded in; target ⚙ ≤2 flag-now alerts/day — sustained breach is itself a digest-tier meta-alert ("your watchdog is too chatty"). The watchdog's own liveness rides the settled dead-man canary (report 04 Def.3/G1 SLA set); no new machinery.

**Post-v0 evolution:** CodeAD-shaped rule synthesis — when Tier-1 repeatedly annotates the same pattern or the operator repeatedly suppresses the same FP, a local-model session drafts a new/adjusted deterministic rule as a proposal card. Detection improves; the free-tier property (S2.7: costs no allowance) is preserved by construction.

### 4.5 The watchlist executor (S2.8 — resolves report 02 OQ#7)

- **T1 (page diffs):** adopt **changedetection.io** (pinned; Apache-2.0), one container. Watch list managed via its REST API from Sinet's config rows (report 02 §6 shape: `{url|feed, type, parser-hint, lane|candidate}`); region-filtered verbatim diffs; Playwright fetcher for JS-walled targets; its **native LLM rules pointed at Sinet's local OpenAI-compatible endpoint** as the first-pass "does this diff matter?" filter; hits POST via `json://` webhook to the control plane.
- **T2/T4 (feeds):** v0 = **Sinet-native feed poller** on the ratified scheduler (conditional GET, content-hash dedup, ~15 feeds incl. GitHub releases/issues Atom, models.dev commits, HN keyword feeds via hnrss.org — *not* self-hosted hnrss: no license file — and authenticated Reddit feed URLs). Miniflux (+Postgres) is the pre-registered fallback if feed handling proves gnarly; its webhook contract is the drop-in shape. **T3 (aggregator APIs):** weekly scheduler jobs (models.dev api.json diff, AA API, OpenRouter model list — settled sources).
- **Second-pass classification (free tier):** every hit — page diff, feed entry, API diff — goes through Sinet's local-model classifier: {relevant lane(s), change class (price/terms/limits/models/endpoints/billing-regime), severity, one-line summary}, grammar-constrained. Relevant hits become **drift alert cards** in the approval inbox with fingerprint-per-incident-window dedup (one card per storm, T08 rule); price hits carry proposed price-table rows with effective dates (P-T08-3). **Billing-regime changes never auto-flip flags** — operator confirms, then the rehearsed 3.10 kill-switch runs (settled, report 02 §6).
- **API-behavior canaries (Sinet-native cron probes, the one genuinely built layer):** per lane — (a) **auth canary**: scheduled cheap real request, error-class discrimination per T08's five-class classifier; auth-shaped failure → lane freeze + operator alert, never retry-park (P-T17-1, settled); (b) **conformance suite** run on any engine/CLI version change and weekly, asserting stream-event schema *and values* on fixtures (P-T01-2/P-T08-1, settled — this is their scheduler home); (c) **behavioral canary**: scheduled fixed-prompt mini-eval per lane (promptfoo-runnable), alert on score drop — the published provider-drift practice; (d) **logprob drift probe** where the lane exposes `logprobs` (API lanes, local models): one-token logprob tracking at ~zero cost; CLI-wrapped subscription lanes rely on (b/c). Model-list drift (P-T17-3, settled) rides the same probe schedule.
- **Decay posture (P-T11-3):** per-source fetcher escalation ladder (plain → Playwright → flagged-decayed card proposing an alternative source); a meta-watch alerts on fetch-failure streaks ≥ ⚙3 cycles so a silently dead watch cannot rot (P46's shape, applied to the watcher itself); standing bias to migrate scrape targets to feeds/APIs.

### 4.6 The benchmark practice (11.2 / S2.11 / 15.3 — the pre-registration package)

**Protocol (per sampled, opted-in task):**
1. **Direct arm:** the same confirmed task statement, run once against the requester's own frontier surface/lane (their subscription, their budget, opt-in per 11.2) — single-shot with the same attachments the spec had. Parity limits are *declared, not hidden*: the platform arm's memory/knowledge injection and multi-turn loop are the treatment under test; consumer-surface model identity is recorded as observed, not controlled (P-T11-2).
2. **Blind pairing:** both deliverables rendered through one **uniform presentation template** (formatting tells stripped; length never truncated — the LMArena length-confound is reported alongside, not "corrected" at this n); position randomized per pair; verdicts: **A / B / tie / both-bad** (ties are signal); arm identity revealed only after the vote.
3. **Blindness is measured, never assumed (P-T11-1):** with each vote the requester also guesses which response was the platform's; guess accuracy is reported next to every win rate. If guesses beat chance, the number is still reported — as a partially-unblinded preference, honestly labeled.
4. **Record per pair (epoch stamping):** both arms' exact model identities + versions, domain, task class, consumption of both arms (the benchmark pair is also the **measured done-directly comparison**), verdict, guess, timestamps. **Win rates are never pooled across a baseline-arm model change.**

**Statistics (pre-registered, computed honestly for household n):**
- **Running rule:** per domain, Beta-Binomial posterior over non-tied pairs (uniform prior). **Alarm** (platform-losing): P(direct ≻ platform) > 0.95 → flag-now card + expansion freeze pending investigation. **Gate** (15.3): opens when, per launch domain, (a) ≥ ⚙20 non-tied pairs accrued, (b) P(platform ⪰ direct) ≥ ⚙0.90, (c) no active alarm, (d) the domain's regression suite (§4.7) is green at its pre-registered floors. Numbers are ⚙ and frozen in the pre-registration commit.
- **Honest-claims table ships with the protocol** (the §2.6 table): the operator sees, at any n, what effect sizes the current sample could even detect — small-n humility as UI, not fine print (P-T11-5).
- **Reporting views:** quarterly exact-binomial + Wilson interval per domain/epoch as a *readout*; the running rule is the only *decision* input.
- **Pre-registration mechanics:** one signed git commit before v0 ships containing: per-domain metrics + thresholds, the statistical rule verbatim, tie handling, sampling rate + opt-in flow, the blindness-measurement plan, arm-parity declaration, and the no-retuning clause. Changes require a new registration commit with rationale (dated, never destructive).

**Done-directly formula (ratifies/refines T08 OQ3):** receipts ship with T08's heuristic — final-accepted-attempt execution usage priced at list, labeled "direct-use estimate (heuristic)". Once a domain accrues ≥ ⚙10 measured benchmark pairs, the *aggregate* honesty figure (S2.5, dashboards) switches to the measured median direct-arm consumption per domain, labeled "measured (benchmark n=…)"; per-run receipts keep the heuristic line with both labels available. The formula text is part of the pre-registration commit — the honesty keystone does not float.

### 4.7 Regression evals & golden-set governance (7.3, 5.7 — consumes report 04 OQ6)

- **Runner:** promptfoo (pinned; MIT) as the eval harness; DeepEval pre-registered as the swap if promptfoo's OpenAI ownership ever manifests as drift (P-T11-4 discipline: pinned versions, fork-tolerant licenses, and **all scores/verdicts land in Sinet's DB** — the tool is a runner, never the record).
- **Per-asset eval objects (versioned knowledge, 8.3):** every worker template and every rubric carries an eval set — golden cases (25–50, grown from real outcomes per report 04) + **planted-defect suite** (research domain: planted unsupported citations, SourceCheckup method; code domain: UTBoost-style test-adequacy defects + known-bad artifacts the judge must fail — all settled mechanisms from report 04, given their standing home here).
- **The revalidation runbook (7.3-analog, triggered by the §4.5 watchlist or an engine version change):** detect model/engine change → flag dependent workers/judges/rubrics (7.3, settled) → run affected golden sets + planted-defect suites via the runner → compare TPR/TNR and task metrics against the pinned baseline → green: release with a dated revalidation stamp; red: worker stays flagged, card to owner. Judge-model changes additionally block unsupervised judging until golden-set re-measurement passes (P-T06-5, settled). Cadence: on-trigger + quarterly full sweep (aligns with the settled re-audit cadence).
- **Rubric falsifiability (5.7):** a rubric "works" iff its planted-defect suite catches ≥ its pre-registered floor — measured by the same runner, recorded per rubric version; the benchmark practice (§4.6) is the outer falsification loop (does rubric-passing work actually win blind comparisons?).

### 4.8 Queryable history (S2.10)

- **Layer 0 — deterministic views** (no LLM): every S2.5 cost question (per run/task/project/person/period, budget remainders, burn rate, limit-event history, done-directly figures) is a named SQL view over the ledger/receipts — the assistant *selects* views, it never computes money by generation.
- **Layer 1 — canned parameterized query catalog:** ~30–50 named queries (status, failures, verdicts, routing, drift history) with typed slots; the local model classifies intent and fills slots (grammar-constrained); results render with the query named — deterministic, auditable, and the reliability floor the small-model evidence supports.
- **Layer 2 — open SQL as escalation:** when no canned query fits, generation against allowlisted views only, under the settled guardrail stack (read-only connection/`query_only`, single-statement parse check, LIMIT + timeout injection, every generated query audit-logged), answer flagged lower-confidence. A 14–32B model on the eGPU is the quality upgrade path (T15).
- **Indexing:** generated columns over event JSON for hot fields; FTS5 over run summaries + verdict texts + drift summaries (proven far beyond Sinet's scale); rollup tables maintained by scheduler jobs. DuckDB-ATTACH is the documented later sidecar if analytics ever slow — not v0.

### What would change this decision

- **OTel GenAI semconv reaches Stable with a real release** → revisit emitting OTLP natively alongside the event log; schema unchanged (vocabulary already mirrored).
- **A one-container, SQLite-backed, OTLP-ingesting trace UI matures** (Phoenix-class without ELv2 concerns, or the Torrix-class entrants) → adopt as the A3 projector target sooner; system of record unchanged.
- **changedetection.io's LLM layer or REST API regresses** (it is one maintainer too) → webchanges + Sinet-side triage is the fallback; the watch-item config rows make the swap mechanical.
- **The household's benchmarkable volume lands well below ~5 pairs/month** → the gate's minimum-n becomes multi-quarter; pre-register a lower-n catastrophic-only gate variant rather than silently waiting.
- **A local model passes T15's judge bar** → Tier-1 watchdog annotations and Layer-2 SQL both upgrade in place; architecture unchanged.
- **promptfoo's post-acquisition governance drifts** → execute the pre-registered DeepEval swap; scores live in Sinet's DB so nothing is lost.

---

## 5. What NOT to use and why

- **Any trace platform as the system of record** (Langfuse, SigNoz, Laminar, Helicone, MLflow-server): oversized footprints for household volume, second-source-of-truth conflict with the D7 event log, 11.3 boundary violations by default, and live M&A churn — Helicone is already maintenance-mode. Langfuse/Phoenix remain acceptable *projection targets* (A3), never stores.
- **Coupling the schema to OTel GenAI semconv today**: Development status, dedicated repo with zero releases, renaming rights reserved. Mirror vocabulary only.
- **An OTel Collector pipeline at v0**: infrastructure for a fan-in problem Sinet doesn't have (two adapters, one writer).
- **OpenAI Evals platform** (shutdown 2026-11-30) and **Braintrust** (self-host Enterprise-only): dead end and wrong shape respectively.
- **LLM-first anomaly detection**: supervised detectors need 37k–460k labeled sequences; per-event model inference burns the free tier for negative value (reasoning SLMs measured actively harmful on log triage). Deterministic-first with a sparse disambiguator is what every shipped system converged on.
- **Learned anomaly baselines at v0**: below AWS's 10-day and Datadog's 3×-seasonality floors at household cadence; fixed operator thresholds are the methodologically correct start (cold-start literature), not a stopgap.
- **Auto-kill watchdog responses**: reaffirmed — Gemini CLI and OpenHands FP records are the evidence; D1.3 pause-and-flag stands (settled, re-validated).
- **Prometheus Alertmanager for dedup**: correct feature, wrong-sized stack; dedup lives in the inbox.
- **Forced-choice-only A/B voting**: draws are systematic signal; forcing choices manufactures preferences (measured).
- **Pooling benchmark win rates across baseline-arm model changes**: the direct arm is un-pinnable; pooled rates conflate model drift with platform value (P-T11-2). Epoch-stamp everything.
- **Post-hoc threshold selection for the 15.3 gate**: the Leaderboard Illusion shows selection freedom inflates results even honestly; the gate is a dated git commit or it is theater.
- **Open-ended text-to-SQL as the assistant's default**: 4–8B nondeterminism on repeat questions; canned catalog first, generation as flagged escalation. **Malloy/semantic-DSL middlemen**: models write SQL better than niche DSLs.
- **Self-hosting hnrss**: no license file (consume hnrss.org or the Algolia API). **Huginn**: heavyweight Rails+MySQL, uncertain maintenance. **webchanges' AI differ** as the triage layer: Gemini-API-only — violates the local-only rule for background intelligence.
- **Pact for provider contract testing**: consumer-driven contracts require provider participation; frontier providers will never verify. oasdiff/schemathesis snapshots instead.
- **Percent-complete progress bars for agent runs**: no shipped product does it, for good reason — show current activity + monotonic counters.

---

## 6. Harvest-map verdicts

| Item | Verdict | Detail |
|---|---|---|
| **N21 — benchmark methodology** (frozen baselines, blind scoring, pre-registered thresholds; bench-02's frozen direct-arm results as the gate's baseline arm) | **CONFIRM the practice, REVISE two mechanics** | Blind scoring and pre-registered thresholds are now externally validated as the correct core (Arena mechanics [79–82]; pre-registration practice [86–88]). Revisions the evidence demands: (1) **"frozen baselines" is unimplementable as stated** — the direct arm rides consumer surfaces that auto-upgrade, and provider retirements bound any pin to 1–2 years; bench-02's frozen results seed the *first epoch* only, and the standing practice is **epoch stamping with no cross-epoch pooling** (P-T11-2). (2) Nexus-era single-batch scoring is superseded by the **anytime-valid running rule** — the only statistics honest at 5–30 pairs/month (computed, §2.6). Additions beyond N21: measured blindness (P-T11-1) and the honest-claims table as UI (P-T11-5). |
| **N16 — routing telemetry + collapse watch** | **CONFIRM (extends report 09's verdict)** | Report 09 settled: keep rationale-storage, `routing_outcomes`, threshold-tweak proposal cards; the keyword router itself is superseded. This report adds the field-validated storage shape — **LiteLLM's `{cause enum, score, signals, routed_model}` per-decision record [27]** — plus the finding that the field's best shipped practice stops short of plain-language reasons (OpenRouter discloses *which*, never *why* [28]): S2.6's plain_reason field slightly exceeds industry practice and is cheap to keep. Routing-quality analysis lands as a §4.1 view joining causes to outcomes. |
| **C4 — quota-window surfacing as observed state** | **CONFIRM (extends report 09's verdict)** | Report 09 settled the meter semantics (measured consumption vs own budgets + provider-signaled facts only; window *estimation* stays banned). This report places C4 in the observability surface: parked-until times and budget meters are first-class card/dashboard state fed by the same SSE stream (§4.3), and the `anthropic-ratelimit-unified-*` probe lead (report 09 §2.9) stays on the implementation-phase probe list. No new mechanics needed from T11. |

---

## 7. Open questions

**Operator decisions needed:**

1. **Ratify the benchmark pre-registration package before v0 ships (gates 15.3).** The §4.6 numbers to freeze: per-domain metrics, gate minimums (⚙20 non-tied pairs, P ≥ 0.90), alarm threshold (P > 0.95), sampling rate, opt-in default, tie handling, blindness-measurement plan, done-directly formula text. This is a dedicated session with a signed commit — the one deliverable 15.3 cannot start without. *Owner: operator (pre-registration session at spec time).*
2. **Done-directly formula (closes T08 OQ3):** ratify §4.6's two-stage form — per-run heuristic (T08's proposal) + measured-median aggregate per domain once ≥10 benchmark pairs exist. *Owner: operator; same pre-registration commit.*
3. **Watchlist executor posture (D2 vs D3):** adopt Miniflux (+Postgres) for feeds, or the recommended Sinet-native poller on the ratified scheduler (no new DB; Miniflux as fallback)? Pure operational-footprint call; changedetection.io adoption and the native canary layer are common to both. *Owner: operator at G2.*
4. **Watchdog + inbox numbers (⚙ set):** Tier-0 thresholds (5× loop, 6-cycle ping-pong, 3× error, silence budgets per run type, 3× median daily-spend multiple), flag-now vs daily-digest routing, the ≤2 flag-now/day target, suppress-twice-proposes-retune rule. All ship as settings per the G1 rider. *Owner: operator at G2.*
5. **Retention/compaction numbers:** 6-month default is spec'd; ratify the compaction keep-set (§4.2) and the run-summary generation duty on the local tier. *Owner: operator at G2.*

**Spikes / later research:**

6. **T15 local-model duty benchmarks** now have three concrete new duties with acceptance bars: watchdog disambiguation (Gemini-CLI-template classification), watchlist diff/entry triage (changedetection.io's LLM rules + Sinet's second pass), and canned-query intent-filling (Layer 1) — all within the ≤8B v0 envelope; Layer-2 SQL quality at 14–32B on the eGPU as the stretch goal. *Owner: T15.*
7. **Logprob-canary applicability probe:** confirm `logprobs` parameter availability per lane (Z.AI OpenAI-compatible endpoint; local servers) — determines which lanes get the ~free drift probe vs behavioral-eval-only. *Owner: implementation-phase probe battery.*
8. **Blindness pilot at v0.1:** run ~10 mock pairs with household members before the real benchmark to measure guess accuracy and calibrate the uniform render template. *Owner: operator at v0.1.*
9. **opencode SSE replay gap:** the adapter already persists events (settled design); add "no reliance on engine SSE replay" as an explicit conformance-suite assertion against the pinned version (#25657 will not be fixed upstream). *Owner: conformance suite (implementation).*

**New platform problems (for the spec's Known-problems list):**

- **P-T11-1 — Benchmark blinding partially fails at household scale.** Daily operators are exactly the expert-user profile that detects AI/platform style near-perfectly (1/300 misclassification); presentation normalization is not shipped practice anywhere. → Uniform render template + **measured blindness**: log the requester's arm-guess per vote and report guess accuracy beside every win rate. *(Feeds 11.2 spec.)*
- **P-T11-2 — The benchmark's direct arm is un-pinnable.** Consumer surfaces auto-upgrade; API snapshots retire on 60-day/3–6-month cycles; "frozen baseline" is unimplementable as standing practice. → Epoch stamping (both arms' model identities per pair), no cross-epoch pooling, gate defined per-epoch; bench-02's frozen results seed epoch 1 only. *(Feeds 11.2 spec; revises N21.)*
- **P-T11-3 — Watcher sources decay structurally.** TLS-fingerprint bot walls, publishers withdrawing from archives, feeds tightening rate limits — the settled source list already hit 403s. → Per-source fetcher escalation ladder, fetch-failure-streak meta-alerts (a dead watch must announce itself — P46 applied to the watcher), standing migrate-to-feed bias. *(Feeds S2.8 spec.)*
- **P-T11-4 — Observability/eval tooling has ownership-churn risk.** Within six months: Langfuse→ClickHouse, Helicone→maintenance mode, promptfoo→OpenAI. → Adoption criteria for this layer: permissive license (forkable), pinned versions, and **the record always lives in Sinet's DB** — tools are runners/projectors, never stores; swaps pre-registered (DeepEval for promptfoo). *(Feeds platform-stack criteria, T13.)*
- **P-T11-5 — Small-n statistical honesty is a product surface, not a footnote.** At household volume most effect sizes are undetectable, and a gate whose threshold ignores accrual volume is theater; naive readings of early win rates will mislead (17 Elo between identical models). → The honest-claims table ships in the benchmark UI; the running rule is the only decision input; every reported rate carries n, interval, and blindness measurement. *(Feeds 11.2 spec + S2.11 UI.)*

**Contradiction records:** none. Report 09's N16/C4 verdicts are extended, not contradicted; report 04's judge/golden-set/planted-defect machinery is consumed as settled and given its scheduling home; report 08's event-log design is confirmed as sufficient substrate (T11 adds event types and views only); the report 02 §6 watchlist design is executed, not altered.

---

## 8. Sources

Tiers: **P** = primary (official docs/repo/spec/paper) · **A** = academic · **PR** = practitioner · **V** = vendor claim · **C** = community. All accessed 2026-07-17. Single-source claims flagged inline in text. Internal settled inputs: `Research/decisions/GATE-1-architecture-direction.md`, `Research/02-provider-watchlist-and-onboarding-criteria.md` §6, `Research/04-verification-and-quality-loops.md`, `Research/08-durable-state-checkpointing-recovery.md`, `Research/09-metering-quota-scheduling.md`, `Docs/component-harvest-map-proposal-v1.md`, `Docs/nexus-post-mortem.md`.

**Tracing, OTel & trace stores**
1. P — https://opentelemetry.io/blog/2026/genai-observability/ — GenAI semconv status, adopters (Claude Code, Codex, Copilot), span taxonomy.
2. P — https://github.com/open-telemetry/semantic-conventions-genai — dedicated repo; zero releases; Development status; `gen_ai.operation.name` values (verified 3/3).
3. P — https://opentelemetry.io/blog/2025/ai-agent-observability/ — GenAI SIG formation, agent track.
4. P — https://langfuse.com/self-hosting + https://langfuse.com/integrations/native/opentelemetry — v3 ~6-service architecture; OTLP ingest (HTTP only).
5. P — https://clickhouse.com/blog/clickhouse-acquires-langfuse-open-source-llm-observability (2026-01-16) — acquisition; MIT commitment.
6. P — https://www.helicone.ai/blog/joining-mintlify (2026-03-03) — acquisition; maintenance mode.
7. P — https://arize.com/docs/phoenix/self-hosting + /configuration + https://github.com/Arize-ai/phoenix — single container, SQLite default, ELv2, OTLP.
8. P — https://signoz.io/docs/install/docker/ — SigNoz footprint.
9. P — https://github.com/lmnr-ai/lmnr — Laminar 5-service stack, Apache-2.0.
10. P — https://mlflow.org/docs/latest/genai/tracing/data-model — OTel-compatible spans; SQLite default backend.
11. C — https://news.ycombinator.com/item?id=48120912 — "Torrix" single-SQLite-file observability entrant (single source; trend evidence only).
12. P — https://llm.datasette.io/en/stable/logging.html — Willison's SQLite per-call log schema (the DIY prior art).
13. A — https://arxiv.org/abs/2606.04990 — "no unified trace schemas for LLM agents" as the named open problem.
14. A — https://arxiv.org/abs/2602.10133 — AgentTrace proposal (a proposal, not a standard).
15. P — https://langfuse.com/docs/administration/data-retention + https://arize.com/docs/phoenix/settings/data-retention — retention = binary delete.
16. P/C — SigNoz ClickHouse TTL docs — trace retention practice.
17. P — https://opentelemetry.io/blog/2022/tail-sampling/ + https://grafana.com/docs/opentelemetry/collector/sampling/tail/ — aggregate-before-drop (spanmetrics) pattern.
18. P/A — Microsoft Agent Framework compaction docs + https://arxiv.org/abs/2605.23296 + https://arxiv.org/abs/2607.08032 — in-agent compaction prior art; long-history summarization degradation.

**Engine emissions & run-story schemas**
19. P — https://code.claude.com/docs/en/headless — stream-json event contract: system/init, api_retry error categories, parent_tool_use_id, result envelope, --include-partial-messages (verified 3/3).
20. P — https://opencode.ai/docs/server/ — SSE /event, server.connected, bus event types (full enumeration partially C-tier via https://deepwiki.com/chriswritescode-dev/opencode-manager/3.3-real-time-streaming-and-sse — verify against pinned version's /doc at build time).
21. P — https://github.com/anomalyco/opencode/issues/25657 — /global/event ignores Last-Event-ID; closed "not planned" (repo now under anomalyco org).
22. P — https://docs.openhands.dev/sdk/arch/events + https://arxiv.org/abs/2511.03690 — append-only event log as single source of truth; replay/fork/audit for free.
23. P — https://code.claude.com/docs/en/sessions + https://github.com/simonw/claude-code-transcripts — JSONL transcript structure (parentUuid lineage).
24. P — https://docs.langchain.com/langsmith/run-data-format — Run model; dotted_order.
25. P — https://langfuse.com/docs/observability/data-model — traces/observations/scores-attached-to-anything.
26. C — https://deepwiki.com/sst/opencode/2.9-storage-and-database + https://github.com/anomalyco/opencode/issues/13654 — opencode SQLite store; migration data-loss record.
27. P — https://docs.litellm.ai/docs/proxy/auto_routing — per-decision `{cause, tier, score, signals, routed_model}` record (the N16 storage shape).
28. P — https://openrouter.ai/docs/guides/routing/routers/auto-router — discloses which model, not why.

**Live inspection & streaming**
29. C×4 — https://procedure.tech/blogs/the-streaming-backbone-of-llms-why-server-sent-events-(sse)-still-wins-in-2025 + https://www.buildmvpfast.com/blog/streaming-llm-responses-sse-vs-websockets-2026 + https://dev.to/smkulkarni/why-chatgpt-uses-sse-instead-of-websockets-and-why-you-probably-should-too-202i + https://speedtesthq.com/guides/ai/streaming-llm-responses-sse-vs-websockets — SSE consensus + operational pitfalls.
30. P — https://pocketbase.io/docs/api-realtime/ — SSE realtime in the canonical small SQLite app (verified).
31. P — https://modelcontextprotocol.io/specification/2025-11-25/basic/transports — Streamable HTTP; SSE ids + Last-Event-ID; EventStore interface (verified 3/3).
32. A/V — https://ably.com/blog/resume-tokens-last-event-id-llm-streaming-reconnection + PR https://zknill.io/posts/everyone-said-sse-token-streaming-was-easy/ — resume cost in cloud architectures (disappears over a durable log).
33. P — https://ai-sdk.dev/docs/ai-sdk-ui/chatbot-resume-streams + https://developers.cloudflare.com/changelog/post/2025-11-26-agents-resumable-streaming/ — ecosystem resumable-stream support (Redis/DO-backed).
34. A — https://textslashplain.com/2019/12/04/the-pitfalls-of-eventsource-over-http-1-1/ — 6-connection limit (old but structural).
35. A/V — https://ably.com/topic/server-sent-events — mobile/battery behavior.
36. P — https://modelcontextprotocol.io/specification/2025-03-26/basic/utilities/progress — progress notifications (monotonic + total + message).
37. P — https://a2a-protocol.org/latest/topics/life-of-a-task/ + /latest/specification/ — task states; input-required first-class (verified).
38. P — https://docs.ag-ui.com/concepts/events + A https://www.copilotkit.ai/blog/master-the-17-ag-ui-event-types-for-building-agents-the-right-way — event taxonomy (17 core at launch, ~28–30 current; correction folded).
39. P — https://docs.github.com/en/copilot/how-tos/use-copilot-agents/cloud-agent/track-copilot-sessions + https://github.blog/changelog/2026-03-19-more-visibility-into-copilot-coding-agent-sessions/ — shipped session-progress surfaces.

**Health watching & alert discipline**
40. P — https://raw.githubusercontent.com/google-gemini/gemini-cli/main/packages/core/src/services/loopDetectionService.ts — thresholds 5/10/30/5–15 verified in source (3/3).
41. P — https://github.com/google-gemini/gemini-cli/issues/11002 + https://github.com/google-gemini/gemini-cli/pull/8231 — FP record; session-scoped disable dialog.
42. P — https://docs.openhands.dev/sdk/guides/agent-stuck-detector — 4×/3×/6×/3× deterministic patterns (verified in SDK source).
43. P — https://github.com/All-Hands-AI/OpenHands/issues/5355 — killed-while-polling FP (the pause-and-flag evidence base).
44. A — https://arxiv.org/abs/2503.13657 — MAST: Step Repetition 15.7%, termination-unawareness 12.4% (1,600+ traces, κ=0.88).
45. A — https://arxiv.org/html/2411.08561v4 — LogLLM: F1 0.9+ needs 37k–460k labeled sequences.
46. A — https://arxiv.org/abs/2601.07790 — Qwen3-4B 95.64% log-severity with RAG; reasoning SLMs harmful (single paper, flagged; verified verbatim 3/3).
47. PR — https://singhajit.com/llm-inference-speed-comparison/ + https://www.tyolab.com/blog/2026/05/11-64gb-ram-12gb-vram-the-honest-local-llm-benchmark/ — 7–8B Q4 ≈ 60–70 tok/s on 12 GB class.
48. PR/A — https://ascentcore.com/2026/04/01/small-llm-performance-benchmark/ + https://arxiv.org/html/2605.19645 — structured-output reliability at 7–8B; quantization effects.
49. A — https://arxiv.org/abs/2510.22986 — CodeAD: offline LLM-synthesized rules, +3.6% F1, 4× faster, <$4 (verified 3/3).
50. P — https://docs.litellm.ai/docs/proxy/alerting — hang threshold 300 s; outage 5/10-error windows; dedup TTL + digest mode.
51. P — https://docs.langchain.com/langsmith/alerts — static-threshold alerting; preview-before-enable guidance.
52. P — https://langfuse.com/docs/administration/spend-alerts — fixed-threshold spend alerts.
53. P — https://aws.amazon.com/aws-cost-management/aws-cost-anomaly-detection/faqs/ — ≥10 days history required (verified 3/3).
54. P — https://docs.datadoghq.com/monitors/types/anomaly/ — 3× seasonality history verbatim (static-threshold framing corrected to inference by verification).
55. A — https://arxiv.org/abs/2405.20341 — ColdFusion: zero-shot priors as the correct cold-start method.
56. P — https://sre.google/sre-book/monitoring-distributed-systems/ — ~2 incidents/shift budget.
57. P — Ewaschuk, "My Philosophy on Alerting" (docs.google.com/document/d/199PqyG3UsyXlwieHaqbGiWVa8eMWi8zzAn0YfcApr8Q) — pages urgent/actionable/real; remove noisy alerts.
58. PR — https://www.stamus-networks.com/blog/what-the-2025-sans-detection-response-survey-reveals-false-positives-alert-fatigue-are-worsening + https://worldinformatixcs.com/2026/07/02/soc-alert-fatigue-causes/ — 46–63% FP/ignore rates (survey secondary reporting).

**Watchlist executor & provider canaries**
59. P — https://github.com/dgtlmoon/changedetection.io — Apache-2.0; selectors; Playwright; Apprise; `changedetectionio/llm/` module with local-endpoint client (verified in source 3/3).
60. P — https://changedetection.io/CHANGELOG.txt — v0.55.8 (2026-07); 0.55.1 LLM rules/summaries; 0.55.4 self-hosted OpenAI-compatible endpoints; LLM_FEATURES_DISABLED.
61. P — https://changedetection.io/docs/api_v1/index.html — REST API: watch CRUD, history, difference endpoints.
62. P — https://pypi.org/project/webchanges/ + https://webchanges.readthedocs.io/en/stable/differs.html — active urlwatch fork; ai_google differ is Gemini-only BETA.
63. P/C — https://github.com/huginn/huginn + https://automationatlas.io/tools/huginn/ — footprint; stalled-activity signals (conflicting; flagged).
64. P — https://github.com/miniflux/v2 + https://miniflux.app/docs/webhooks.html + https://miniflux.app/releases.html — Apache-2.0; PostgreSQL-only; HMAC-signed global webhook (verified 3/3).
65. P — https://github.com/RSS-Bridge/rss-bridge — maintained; feeds for feedless pages.
66. P — https://github.com/hnrss/hnrss (+ GitHub license API) — Go, self-hostable, **no LICENSE file** (correction, verified 2 verifiers).
67. PR/P — https://lapcatsoftware.com/articles/2026/6/3.html + https://github.com/FreshRSS/FreshRSS/issues/8928 — Reddit feed rate-limit change; authenticated feed URLs.
68. A — https://lirantal.com/blog/a-comprehensive-guide-to-contract-testing-apis-in-a-service-oriented-architecture-5695ccf9ac5a — Pact anti-fit for third-party APIs.
69. P — https://github.com/oasdiff/oasdiff — OpenAPI diff, breaking-change classification.
70. P — https://github.com/schemathesis/schemathesis — v4.22.4, MIT; live-endpoint conformance tests.
71. P — https://www.promptfoo.dev/docs/red-team/model-drift/ — scheduled drift scans as published practice.
72. P — https://arxiv.org/abs/2512.03816 — one-token logprob tracking; single-finetuning-step sensitivity; ~1000× cheaper; TinyChange (verified verbatim 3/3).
73. V/PR — https://www.dotcom-monitor.com/blog/monitoring-oauth-2-client-credentials-flow/ + https://oneuptime.com/blog/post/2026-01-24-oauth2-token-revoked-errors/view — OAuth canary pattern; revocation error taxonomies.
74. P — https://github.com/lilydjwg/nvchecker — version-watch utility (CLI-drift composition).
75. P — https://github.com/caronc/apprise — notification multiplexer, BSD-2.
76. A — https://www.pistack.xyz/posts/2026-05-07-prometheus-alertmanager-vs-ntfy-vs-gotify-self-hosted-alert-routing-guide/ — only Alertmanager dedups; ntfy/Gotify comparison.
77. P/V — https://github.com/dgtlmoon/changedetection.io/issues/2198 + https://changedetection.io/tutorial/what-are-main-types-anti-robot-mechanisms — bot-wall flakiness is structural.
78. PR/P — https://www.niemanlab.org/2026/01/news-publishers-limit-internet-archive-access-due-to-ai-scraping-concerns/ + https://archive.org/help/wayback_api.php — archive fallbacks weakening.

**Benchmark methodology & statistics**
79. A — https://arxiv.org/html/2403.04132v1 — Chatbot Arena protocol (anonymized pairs, post-vote reveal).
80. A — https://arxiv.org/pdf/2510.02306 — "Drawing Conclusions from Draws": ties are signal; forced choice harmful.
81. P — https://www.lmsys.org/blog/2024-08-28-style-control/ — length coefficient 0.249 dominant; markdown second-order; rank shifts (verified 3/3).
82. A — https://arxiv.org/abs/2504.20879 — Leaderboard Illusion: 17-Elo gap between identical checkpoints; selective-disclosure inflation (verified 3/3).
83. A — https://arxiv.org/pdf/2302.10108 — anytime-valid inference (SAVI): deployed at Adobe/Amazon/Netflix.
84. A — https://arxiv.org/pdf/2606.18366 + https://arxiv.org/pdf/2504.00593 — 2026 sequential-testing refinements; AVI width penalty.
85. A — https://pmc.ncbi.nlm.nih.gov/articles/PMC3005845/ — sign-test tie handling; power impact at n≤50.
86. A — https://arxiv.org/pdf/2603.10044 — protocols frozen pre-collection ("not re-tuned in response to failure rates"); scaffolding conditions measured capability.
87. A — https://arxiv.org/pdf/2606.15887 — OSF-registered hypotheses/rubrics/margins in ML eval.
88. A — https://arxiv.org/pdf/2503.04746 — frontier safety frameworks as pre-committed threshold practice.
89. A — https://aclanthology.org/2025.acl-long.267/ (arXiv 2501.15654) — expert LLM users: 1/300 misclassified by 5-rater majority; beats *most* detectors (correction folded; verified 3/3).
90. A — https://arxiv.org/pdf/2504.14871 — model style fingerprints survive prompted style changes.
91. P — https://platform.claude.com/docs/en/about-claude/models/migration-guide — per-model checklists; verification steps; ≥60-day deprecation notice.
92. P — https://developers.openai.com/api/docs/deprecations + /guides/latest-model — Evals shutdown 2026-11-30; migration eval-rerun guidance; 3–6-month API-snapshot cycles (verified 3/3).
93. A — https://arxiv.org/html/2507.06434v1 + https://arxiv.org/pdf/2606.13715 — benchmark deprecation criteria; version-stamped longitudinal re-runs (epoch-stamping practice).
94. A — https://arxiv.org/pdf/2510.11977 + https://arxiv.org/pdf/2511.01668 — scaffold-vs-bare ablations; fairness controls (same model/context/decoding).

**Eval harnesses & revalidation**
95. P — https://github.com/promptfoo/promptfoo — MIT; v0.121.19 (2026-07-14); matrix + judge-free assertions; Ollama.
96. P — https://openai.com/index/openai-to-acquire-promptfoo/ + https://www.promptfoo.dev/blog/promptfoo-joining-openai/ (2026-03-09) — acquisition; open-source commitment (verified 3/3).
97. P — https://github.com/confident-ai/deepeval — Apache-2.0; v4.1.0 (2026-07-12); pytest-native; configurable judges.
98. P — https://github.com/UKGovernmentBEIS/inspect_ai — MIT; active; AISI rigor.
99. P/V — https://www.braintrust.dev/articles/best-self-hosted-ai-evals-tools-2026 + https://langfuse.com/faq/all/best-braintrustdata-alternatives — Braintrust self-host Enterprise-only (mutually corroborating vendors).
100. P — https://aws.amazon.com/blogs/machine-learning/aws-generative-ai-model-agility-solution-a-comprehensive-guide-to-migrating-llms-for-generative-ai-production/ — shadow mode → canary → rollback playbook.
101. A — https://arxiv.org/pdf/2511.21140 — judge-error-corrected score reporting (single-source; compatible with settled golden-set method).

**Queryable history & SQLite analytics**
102. P/C — https://bird-bench.github.io/ — top pipeline 81.95%, single-model 80.04%, human 92.96% (verified).
103. A/V — https://motherduck.com/blog/bird-bench-and-data-models/ — ~94–95% under human-review grading on clean small schemas; "data modeling is the semantic layer."
104. P — https://spider2-sql.github.io/ + A https://arxiv.org/abs/2411.07763 — 2024 paper numbers 5–31%; mid-2026 leaderboard tops ~66–97% self-reported (dated per verification correction).
105. A — https://github.com/uiuc-kang-lab/text_to_sql_benchmarks — 52.8–62.8% annotation error rates in BIRD/Spider 2.0 subsets.
106. P/A — https://huggingface.co/Snowflake/Arctic-Text2SQL-R1-7B + https://arxiv.org/abs/2505.20315 — 68.5% BIRD-test at 7B, open weights (verified).
107. PR — https://datamonkeysite.com/2026/03/31/text-to-sql-using-semantic-models-and-small-language-models/ — Qwen3-4B 90–95% but run-to-run nondeterministic; intent-only conclusion (single-source, highly specific).
108. A/P — https://arxiv.org/abs/2411.15100 (XGrammar) — <40 µs/token; default vLLM structured-output backend (verified).
109. A/PR — https://arxiv.org/pdf/2604.16511 + enterprise NL2SQL-guardrails deployment reports — read-only connection, views, parse checks, limits, audit logging as consensus stack.
110. PR — https://builder.aws.com/content/2nHgBx9YiFp5Dm2kUb9mpRH3foM/i-gave-my-ai-agent-a-semantic-layer-instead-of-raw-sql — LLMs write SQL better than niche DSLs (single-source).
111. P/PR — https://sqlite.org/json1.html + https://antonz.org/sqlite-generated-columns/ — JSON functions + generated-column indexing.
112. PR — https://andrewmara.com/blog/faster-sqlite-like-queries-using-fts5-trigram-indexes — FTS5 trigram: 18.2M rows, 1.75 s → 14 ms, +2.4 GiB.
113. P — https://duckdb.org/docs/lts/core_extensions/sqlite — official ATTACH-SQLite analytics sidecar.
114. A — https://arxiv.org/pdf/2606.14589 — silent-failure taxonomy: agents fail without exceptions; watchdog-with-semantic-expectations recommendation (single-source).
