# 07 — Context engineering: assembly, rot, and the per-stage cost structure

**Topic:** T04 · **Wave:** A2 · **Depth:** FULL · **Written:** 2026-07-17
**Method:** deep-research harness — 5 fan-out search angles (canon, rot evidence, session architecture, convention files/injection, caching/evals), ~70 primary fetches, 34 load-bearing claims put through a 3-vote adversarial verification pass (result: 34 survive, 0 refuted; 3 claims carry framing corrections, noted inline). All URLs accessed 2026-07-17 unless dated otherwise.

---

## 1. Scope

From the brief: **8.4/S4.6** (the right knowledge arrives by itself — *assembly* side; storage is T10), **4.3** (blocked-is-not-failed freshness re-validation), **S1.6** (repo conventions capture — the onboarding task), known-problem **"context rot"**, and the post-mortem cost finding (per-stage context re-send ≈ the ~15× token-multiplication trap). Constraints binding this topic: D6 (helpers get sliced context by design — settled mechanics in report 06, not re-derived here), D7 (checkpoints must capture enough context state to resume), 11.1/S2.1 (whatever is injected must appear in the trace), D5 (flat-rate lanes optimize consumption pressure, never dollars), adopt-don't-fork (compaction/caching only via engine config/APIs). Engine substrate per report 01: pinned `opencode serve` per user + wrapped `claude` CLI per user.

This report fixes the **per-stage/per-session dimension**: session-per-stage vs continuous, what survives compaction, how knowledge slices are selected and injected, and what caching means on each lane. Helper topology (the *per-worker* dimension) is report 06's; the two contracts compose.

---

## 2. Current state of the art (mid-2026)

### 2.1 The discipline's canonical shape — agreed vs contested

The canon formed in 2025 and was *extended, not revised*, through mid-2026. The load-bearing texts:

- **Anthropic, "Effective context engineering for AI agents" (2025-09-29)** — the field's reference. Context is a finite "attention budget" with diminishing returns (grounded in n² attention); the goal is the "smallest possible set of high-signal tokens that maximize the likelihood of some desired outcome"; system prompts at the "right altitude" (heuristics, not hardcoded logic); **just-in-time retrieval** — agents hold "lightweight identifiers (file paths, stored queries, web links)" and load data at runtime; three named long-horizon techniques: **compaction, structured note-taking, multi-agent architectures** (sub-agents returning "often 1,000–2,000 tokens"), each with an explicit niche and an explicit compaction warning: "overly aggressive compaction can result in the loss of subtle but critical context whose importance only becomes apparent later." (Vendor-blog tier; verified verbatim.)
- **Anthropic, "Effective harnesses for long-running agents" (2025-11-26)** — the multi-window sequel, and the most Sinet-shaped first-party document found: **"compaction isn't sufficient"** alone; the prescribed harness is an *initializer agent* that sets up the environment plus *fresh coding sessions* that onboard from **disk artifacts** — a JSON feature list (features marked failing until verified), a progress file, `init.sh`, git history ("read the git logs and progress files to get up to speed"). Every session start re-validates against live state (run the app, smoke-test) *before* new work. (Vendor-blog tier; verified.)
- **LangChain (2025-07-02)** — the **write / select / compress / isolate** taxonomy; canonical vocabulary, still current. Also the source of the widely-recycled "Claude Code auto-compacts at 95%" figure, which is now version-stale (see §2.5). (Vendor-blog tier.)
- **Manus (2025-07-18)** — the production cache doctrine: "the KV-cache hit rate is the single most important metric for a production-stage AI agent" (input:output ≈ 100:1, ~50 tool calls/task); append-only stable prefixes; never add/remove tools mid-iteration (mask, don't remove); **the file system as the ultimate context** — compression must be *restorable* ("the content of a web page can be dropped … as long as the URL is preserved"); **recitation** — rewriting `todo.md` to "push the global plan into the model's recent attention span"; keep the wrong turns in context. (Production tier.)
- **12-Factor Agents, Factor 3** ("own your context window") — context format is an application-owned concern; don't accept the default message schema. (Practitioner-methodology tier.)
- **The 2026 layer** extends this: Anthropic shipped context management as **API primitives** — context editing (beta, `context-management-2025-06-27`; `clear_tool_uses_20250919` default trigger 100K input tokens), **server-side compaction** (beta, `compact-2026-01-12`, default trigger 150K, typed compaction block echoed back; the SDKs' client-side compaction is deprecated in its favor), and the **memory tool now GA** (`memory_20250818`, no beta header; the API *auto-injects* an "ASSUME INTERRUPTION: your context window might be reset at any moment" instruction when the tool is present — official acknowledgment that state-outside-the-window is the design center). Agent Skills' progressive disclosure (markdown + frontmatter, ~80 tokens/skill at discovery) spread across vendors. No principle from 2025 was retracted. (Official-docs tier throughout; all verified.)

**Notable absence:** no OpenAI or Google first-party context-engineering doctrine equivalent to Anthropic's exists (cookbook-level fragments only) — the canon is effectively Anthropic + framework blogs + production practitioners.

**Still contested mid-2026** (credible sources actively disagree):

1. **Embeddings-RAG vs agentic search** for code/document corpora. The practitioner wave ("RAG is dead" essays, 2026-Q2) favors grep/glob + JIT retrieval as default, embeddings only for large stable corpora and sub-second latency. No controlled resolution.
2. **Tool-RAG**: LangChain reports 3× tool-selection accuracy from retrieving tool definitions; Manus/Schmid forbid dynamic tool loading outright (breaks KV-cache, confuses the model). Practical reconciliation: masking for stable toolsets, retrieval only for very large catalogs.
3. **Share-vs-isolate** resolved *by scope*, not by winner: collaborating agents share maximally (Cognition), verification agents get deliberately clean context (Cognition's own 2026 revision — reviewers "perform better without" shared history; report 06 §2.1 covers this arc).
4. **Memory architecture** (compaction-tiers vs CRUD-vector vs graph vs provider-managed): no winner; T10's problem.
5. **Effective vs advertised window**: vendors sell 1M-token windows while their own tooling compacts at 80–180K and practitioners budget 128–256K; no vendor publishes an "effective length" number — only benchmarks do (§2.2).

### 2.2 Context rot — measured, real, task-shaped, improving

The evidence base is now strong enough to design against (all independently verified this run):

- **Chroma "Context Rot" (2025-07-14; 18 models incl. GPT-4.1, Claude 4, Gemini 2.5):** performance degrades with input length even on trivially simple tasks; "even a single distractor reduces performance," four compound it; **focused prompts beat full prompts across all families**; and the unresolved oddity — models score *better on shuffled than logically structured* haystacks [single-source]. (Vendor-benchmark tier, methodology open.)
- **NoLiMa (arXiv:2502.05167, ICML 2025; semantic needles, no lexical overlap):** "At 32K … 11 models drop below 50% of their strong short-length baselines" (of 13 tested); GPT-4o falls 99.3% → 69.7%. CoT does not rescue it. (Independent benchmark; pre-2026 models.)
- **OOLONG (arXiv:2511.02817, CMU; aggregation/reasoning over the *whole* context):** "GPT-5, Claude-Sonnet-4, and Gemini-2.5-Pro all achieving less than 50% accuracy on both splits at 128K." The strongest evidence that **whole-context integration still rots at the frontier** even where needle retrieval is near-solved. (Independent benchmark.)
- **The frontier is improving fast but unevenly:** Epoch AI — the input length where top models hit 80% accuracy rose "over 250x in the past 9 months," yet "most models are below 80% accuracy" by 192K (2025-06, pre-Gemini-3/GPT-5.x). MRCR v2: Opus 4.6 93% at 256K / 76% at 1M (8-needle; vendor-derived via Vellum; 1M cross-model comparisons circulate only in aggregators — unverified). **Design consequence: rot is a moving constant, not a solved problem — budget below the window and re-calibrate per model generation.**
- **Multi-turn rot is distinct from length rot and worse-behaved (arXiv:2505.06120, Microsoft/Salesforce, 15 models, 200K+ simulated conversations):** average **−39%** across six tasks when instructions arrive sharded over turns; decomposition: aptitude −16%, **unreliability +112%**; temperature 0 leaves ~30% unreliability. Mechanism: early premature assumptions the model then self-conditions on — "when LLMs take a wrong turn in a conversation, they get lost and do not recover."
- **The same paper contains the strongest controlled result FOR Sinet's architecture:** concatenating all sharded information into **one fresh, consolidated turn restores 95.1%** of full-context performance (RECAP variant lifts GPT-4o 59.1% → 76.6%). A fresh session with a well-built consolidated brief is not a workaround; it is the measured best case.
- **Agentic counterpoint (LoCoBench-Agent, arXiv:2511.13998):** interactive tool-using agents show "remarkable long-context robustness" — agents convert long-context problems into iterated short retrieval by re-finding information. Rot bites hardest where the model must integrate everything at once; agents with retrieval tools partially route around it.
- **Folklore correction (arXiv:2604.02547; 9,374 trajectories):** "the widely reported correlation between trajectory length and failure **reverses direction once task difficulty is controlled**" — long trajectories per se don't cause failure; hard tasks cause both. Don't build policy on "kill long sessions" alone. (Single paper; flagged.) Separately, ReAct-style agents are reported to saturate/degrade after ~60 interaction rounds on SWE-Bench (arXiv:2512.22087; method-paper tier, single-source).

### 2.3 The mitigation hierarchy, ranked by evidence strength

1. **Consolidate-and-restart (fresh session + consolidated brief).** Controlled, multi-model: 95.1% restoration (§2.2). Field practice converges (Anthropic harness; Ralph loop; Agent SDK guidance §2.4). **Strongest rung.**
2. **Just-in-time / focused retrieval over pre-loading.** Two independent lines: LongMemEval (arXiv:2410.10813) — ~30% accuracy drop reading full interaction history vs targeted retrieval; Chroma — focused > full prompts across all families.
3. **Observation masking (drop/placeholder stale tool outputs, keep recent K).** Controlled head-to-head (arXiv:2508.21433, JetBrains): masking "halves cost … while matching, and sometimes slightly exceeding, the solve rate of LLM summarization" on SWE-bench Verified. Caveat with its own controlled evidence (arXiv:2606.00408): gains follow an **inverted-U** — masking collapses when it removes evidence still needed (report 06 flagged the same).
4. **Compaction / LLM summarization.** Helps vs nothing at exhaustion; **not better than masking in the only head-to-head, and costlier**. Its failure modes are now quantified — see §2.5 (ConstraintRot; cascade loss). Vendor evals (context editing −84% tokens over 100 turns; +29% alone, +39% with memory) are settled in report 06 §2.4 and remain vendor-internal-tier.
5. **Structured external memory / notes.** Weakest *controlled* base (memory's increment ≈ +10pts on a vendor-internal eval) but universal in production practice and load-bearing in every fresh-restart design — the notes are what makes rung 1 work. Evidence tier: production-consensus more than benchmark.

### 2.4 Session architecture in practice: fresh-per-stage vs continuous

- **The fresh-context pole** is now first-party doctrine for multi-window work: Anthropic's harness post (initializer + clean sessions onboarding from artifacts); the Agent SDK sessions docs explicitly advise that capturing "analysis output, decisions, file diffs … as application state" and passing them "into a fresh session's prompt … is often more robust" than transcript resume; the **Ralph Wiggum loop** (Huntley, mid-2025→2026) is the canonical practitioner form — a bash loop restarting the agent each iteration, "specifications, implementation plans, and progress logs remain accessible between runs, while the LLM's internal context resets," motivated by explicit compaction distrust ("details get lost … the model loses the thread"). One practitioner shipped a 16-phase app in ~4h/~€70 this way. (Vendor-blog + official-docs + practitioner tiers.)
- **The continuous pole**: Cognition 2025 ("the simplest way … a single-threaded linear agent," with a bespoke compressor-LLM for overflow — "hard to get right"); softened by 2026 but still the intra-task position.
- **The reconciliation** — and the actual consensus: the two camps agree on the invariant, **durable state lives outside the window**. Continuous-with-compaction optimizes intra-task coherence on one window; fresh-per-stage optimizes multi-window endurance and error decorrelation. **No controlled head-to-head exists** (flagged as a gap). The controlled evidence that does exist (CONCAT 95.1%, multi-turn self-conditioning, ConstraintRot, cascade loss) all points at bounded sessions with consolidated handoffs for work that spans stages.
- **Handoff artifacts in the wild** (union of Anthropic harness, memory-tool docs, Manus, HANDOFF.md practitioners, opencode's own summary schema): task list with verified-status ("mark a feature complete only after end-to-end verification … not when the code is written"), decisions made, acceptance criteria, file paths (restorable references), blockers, next actions, learned constraints — plus git history as ground truth. Known failure mode: underspecified/unbounded tasks per iteration (the plan-loop weakness).
- **Resume freshness**: no engine (Claude Code, opencode, Codex) natively re-validates a resumed plan against current repo/world state. Codex has a documented stale-resume failure class (issue #31982: resumes "from a stale conversational checkpoint … while VCS state is newer," risking duplicate commits/overwrites). Anthropic's harness bakes re-validation into session start by convention (git log + progress file + smoke-test before new work). **Spec 4.3 is confirmed platform-layer work with a first-party template but no engine primitive.**

### 2.5 Engine-native context machinery (what Sinet's two engines already do)

**Claude Code / wrapped CLI lane.** The arXiv reverse-engineering ("Dive into Claude Code," 2604.14228) verifies a **five-stage native pipeline** before each call: tool-result budgeting → snip → microcompaction (bulky tool results offloaded to disk, path reference stays inline) → context collapse → auto-compact (model-generated summary). Operationally decisive facts, all verified:

- **Auto-compact cannot be disabled**: `autoCompactEnabled: false` is "parsed and silently discarded" everywhere; issue #42817 closed *not planned* (2026-05-18); the threshold override env var is capped by `Math.min` — it can only *lower* the trigger. One report of firing at ~35% of a 1M window (single-source). Thresholds are **version-unstable** — treat any percentage as a property of the pinned version.
- **Hooks**: current docs say PreCompact **can block** compaction (exit-2 / `decision:"block"`; matchers `manual`/`auto`), a PostCompact event now exists, and SessionStart fires with `source:"compact"` after compaction — a documented re-grounding injection point. The April-2026 issue claimed blocking didn't work; docs may have shipped ahead of or after behavior. **Empirical test on the pinned version required before relying on it.**
- Compaction summaries are steerable (`/compact <focus>`), human-readable; reported quality failures cluster on mid-debug auto-compacts ("suggest changes to the wrong file").
- **Caching**: automatic unless env-disabled; **on subscription auth Claude Code requests the 1-hour cache TTL automatically at no extra cost**; billed-per-token overflow drops to 5-min TTL; **subagents build separate caches at 5-min TTL even on subscription**; caches scope to machine + working directory (different cwd = different prefix). Cache telemetry is fully local (`cache_read_input_tokens` / `cache_creation_input_tokens` per response; JSONL/statusline/OTel).

**opencode lane.** Compaction is cleanly configured and documented (v2 docs, verified): `compaction.auto` default true; trigger when estimated tokens > context limit − max(requested output, `buffer` default 20000); `keep.tokens` default 8000 kept verbatim; summary covers "objective, important details, completed and active work, blockers, next moves, and relevant files"; tool outputs capped at 2000 chars in the summary prompt; manual `/compact` + REST endpoint; `prune` is schema-accepted but **currently a no-op**; no separate compaction-model setting; earlier messages remain stored after removal from active context. Instructions injection: native AGENTS.md (root + global) + CLAUDE.md fallback + a deterministic `instructions` array (paths, globs, remote URLs with 5s timeout). Nested per-directory AGENTS.md auto-selection is an open request (#7576).

**Compaction quality — the two numbers that matter for platform design** (both 2026, both verified):

- **ConstraintRot (arXiv:2606.22528):** in-context policy constraints are "silently removed by compaction" — violations go **0% with the policy in full context → ~30% after compaction (59% for some models)**; when the constraint survives the summary, violations stay ≈0%; **"constraint pinning" (quarantining constraints from lossy compaction) restores ≈0%**.
- **Cascading compaction (arXiv:2605.24657):** after three compaction cycles, knowledge retention falls to 36.8±3.0% against a 90.1% full-context ceiling — **≈53pp lost in three cycles** (derived from the paper's published numbers, not a verbatim figure).

Together: compaction is a safety net whose *unmanaged* use silently deletes exactly what Sinet's gates depend on (constraints, acceptance criteria, decisions). Anything that must survive must live outside the summarizer's reach.

### 2.6 Deterministic injection and provenance

- **Every major coding agent selects conventions deterministically** (nearest-file-wins nesting per the AGENTS.md spec; Codex root-down concatenation with 32 KiB cap; Cursor rule modes incl. glob-scoped and nested rules; opencode instructions globs; Claude Code subdirectory CLAUDE.md load-on-touch and `.claude/rules/*.md` with `paths:` glob frontmatter). **None uses embedding similarity for conventions**; the only non-deterministic mode in convention tooling is description-keyed model choice (Cursor "Agent decides", skills). Embedding retrieval lives in episodic-memory products (mem0/Mastra-class), whose own practitioners warn mistimed injection "make[s] the agent worse." **No head-to-head eval of deterministic vs retrieval convention injection exists** — the convergence is by engineering logic (auditable, reproducible, cache-stable), not measurement. (Official-docs tier, convergent across 6+ vendors.)
- **Provenance**: Claude Code ships purpose-built injection audit — the `InstructionsLoaded` hook reports "exactly which instruction files are loaded, when they load, and why"; `/context` shows the live picture. OTel GenAI semantic conventions are the future export target but are **Development-status**, moved to a dedicated repo, with **content capture opt-in and off by default** and the mechanism (span attributes `gen_ai.system_instructions`/`gen_ai.input.messages` vs events) still in flux mid-2026. Langfuse-class tracing does version-linkage (which prompt version produced this trace), not content diffing. **Conclusion: Sinet must own its injection-provenance schema** (path + content hash + version per injected item) and map to OTel later.

### 2.7 Convention files (AGENTS.md-class): standard, value, size, upkeep

- **The standard**: AGENTS.md, released Aug 2025 (OpenAI-led, multi-vendor credited), adopted by "more than 60,000 open source projects," stewarded since 2025-12-09 by the **Linux Foundation's Agentic AI Foundation** (Anthropic is a platinum member). Spec: plain Markdown, any headings, nearest-file precedence. 24+ tools read it natively — **including opencode**. **Claude Code still does not** (docs: "Claude Code reads `CLAUDE.md`, not `AGENTS.md`" — prescribed workaround is a CLAUDE.md whose first line imports `@AGENTS.md`, or a symlink); feature request #6235 open since 2025-08-21 with no official response. Gemini CLI likewise closed AGENTS.md-by-default as not-planned — **treat the agents.md adopter list as "configurable support," marketing-adjacent**.
- **Measured value — the two 2026 papers point opposite ways, and the split is the finding**:
  - Controlled ablation (ETH Zurich/LogicStar, arXiv:2602.11988, rev. 2026-06): "providing context files does not generally improve task success rates, while increasing inference cost by over 20% on average"; **developer-written files ≈ +4% avg** (AGENTbench; 2 of 3 verifiers located the figure in the body — treat as approximate) vs **LLM-generated −0.5% to −2%**; "repository overviews, although popular and recommended by model providers, are not helpful"; instructions ARE well-followed — value concentrates in "specifying non-standard coding practices."
  - Observational field study (arXiv:2601.20404; 10 repos, 124 agent PRs): AGENTS.md presence associated with **−28.6% median runtime, −16.6% output tokens** at comparable completion. Methodologically weaker (confounded); the controlled study wins where they conflict.
  - **Net reading:** a *short, human-curated file of non-inferable facts* (commands, non-standard conventions, danger zones) helps modestly and cuts waste; auto-generated-and-unedited or overview-heavy files are net-negative ballast.
- **Size discipline is now official**: Anthropic — "target under 200 lines per CLAUDE.md"; its `/doctor` trims "content Claude can derive from the codebase (directory layouts, dependency lists, architecture overviews)" and keeps "pitfalls, rationale, and conventions that differ from tool defaults"; Cursor — under 500 lines; Codex — 32 KiB hard cap; practitioner norms 100–300 lines.
- **Generation & upkeep**: both engines ship `/init`-class generators (opencode `/init` writes AGENTS.md; Claude Code `/init` generates or improves CLAUDE.md and ingests existing AGENTS.md/.cursorrules). Given the ablation result, generator output requires **human pruning to the non-inferable residue**. Staleness automation exists as CI pattern (diff-triggered doc-update PRs), not product standard.

### 2.8 Caching on Sinet's two lanes — and what it means under flat rate

Report 06 §2.4 settled API-lane economics (reads ~0.1×, writes 1.25×/2×, prefix-scoped, first-response availability, 41–80% measured agentic savings). New, verified, lane-specific facts:

- **Subscription lane (wrapped `claude` CLI)**: caching is automatic, free, and *better* than API defaults (1h TTL auto-requested; "usage is included in your plan rather than billed per token, so the longer TTL costs you nothing extra"). **Whether cached tokens weigh less against Pro/Max 5-hour/weekly windows is publicly unspecified** — the help-center limits page defines usage qualitatively, names no token unit, and never mentions caching (absence verified by all 3 verifiers). Indirect signals that internal metering discounts cache reads: the Enterprise seat-pool page states turns after the first bill "at the much cheaper cache-read rate" [single-source], and the caching docs exist to explain "usage looks high" when caches break. Circulating claims that "cache reads don't count toward subscription limits" project the API ITPM rule without primary basis.
- **API/metered lane (Anthropic)**: **cache-aware rate limits are primary-source confirmed** — "only uncached input tokens count toward your ITPM" for all current models (cache reads excluded, cache writes count; retired Haiku 3.5 was the exception); official worked example: 2M ITPM at 80% hit rate ⇒ ~10M effective input tokens/min. On the metered lane caching multiplies **headroom**, not just dollars.
- **Third-party OAuth is dead as a lane** — mechanism corrected by verification: the March-2026 opencode 400s were root-caused not to `cache_control` but to **Anthropic blocking third-party OAuth clients generally; opencode dropped Anthropic OAuth support at Anthropic's request** (issue #17910 thread). This independently re-confirms report 01's architecture: the wrapped first-party CLI is the *only* subscription-covered Anthropic path, caching included.
- **Z.AI GLM coding plan** (opencode lane): quota is **prompt-count, not tokens** — ~80 (Lite) / ~400 (Pro) / ~1600 (Max) prompts per 5h, "each prompt typically allows 15–20 model calls," 7-day weekly cap, peak deducts 3× vs off-peak 2×; the FAQ never mentions caching. **On this lane caching buys latency only — quota stretches by batching more work per prompt, a different currency unit entirely.** API-side Z.AI does cache automatically (billed lower; discount magnitude not primary-verified).
- **OpenAI (watchlist context)**: caching automatic ≥1024 tokens, but "caching does not affect rate limits" — cached tokens **still count toward TPM**, the exact inverse of Anthropic's policy. GPT-5.6+ cache writes 1.25×, TTL fixed "30m". DeepSeek: disk cache on by default, best-effort, hit/miss usage fields.
- **Latency (the flat-rate payoff)**: controlled agentic study (arXiv:2601.06007, 500+ sessions): TTFT −13.0% (GPT-5.2), −22.9% (Claude Sonnet 4.5), −30.9% (GPT-4o), −6.1% (Gemini 2.5 Pro); cost −41…−80%; **naive full-context caching "can paradoxically increase latency"** (writes with no reusable hits). OpenAI's own runs: 7% faster at 1024 tokens → 67% faster TTFT at 150K+.
- **D5 synthesis**: on flat-rate lanes the cache currencies are **latency + (probably) window headroom, with the quota weighting officially unknown**; on metered lanes they are dollars + ITPM headroom (Anthropic) or dollars only (OpenAI-style). Cache hit rate is precisely measurable on every lane from local usage fields; its *quota meaning* differs per lane and must be modeled per lane (D4/D5).

### 2.9 Budgets and measurement

- **Per-stage token budgeting as an enforced architectural control barely exists in the field.** deepagents enforces percentage-of-window triggers (summarization at 85% of max input, keep 10% recent; tool results >20,000 tokens offloaded to files with path + first-10-lines preview; an agent-callable `compact_conversation` tool for compacting *between tasks* rather than at capacity) — but "no explicit per-turn or per-stage token budget controls are documented." Anthropic's API is the only server-enforced budget mechanism (trigger thresholds executed server-side; cookbook example bounds peak context 335K → 173K). Manus achieves budget-like control as a *side effect* of cache economics (append-only, restorable offloading). Everyone budgets *interfaces* (sub-agent reports 1–2K tokens) more than *inputs*. **A platform that plans stages to fit a budget, rather than reacting at 85–95%, is ahead of the field, not behind it.**
- **Measuring context quality solo** (SQ9): (a) **compaction-fidelity checks** are now evidence-backed — ConstraintRot's design (inject known constraints/decisions, compact, test behavior or grep the summary) is directly reusable as a checklist audit; (b) **RAG-triad-class LLM-judge metrics** (TruLens: context relevance + groundedness + answer relevance; Ragas context precision/recall) run without per-query ground truth — reusable against injected slices ("was the injected playbook relevant to this task?"); (c) **promptfoo**-class harnesses do config-axis ablations over agent backends with `llm-rubric`, cost/latency assertions, trajectory assertions, `--repeat N` for variance — the right shape for "conventions on/off, ledger on/off" A/Bs; (d) **ccusage**-class local JSONL parsing gives cache-hit ratio and 5-hour-window burn with zero instrumentation. The caching paper's design (identical sessions per condition, 40/condition) is a copyable ablation template.

---

## 3. Candidate approaches

**A. Continuous session per task + engine compaction (the default the engines hand you).**
One engine session lives as long as the task; auto-compact absorbs overflow. *For*: zero platform machinery; maximal intra-task coherence; best cache reuse (one growing prefix). *Against*: measured compaction hazards (constraints silently dropped 0%→30–59%; ≈53pp knowledge loss over three cascades); Claude Code's compactor is neither disable-able nor version-stable; multi-turn self-conditioning (−39%, +112% unreliability) accumulates; a mid-debug compact is the field's canonical quality complaint; D7 resume then depends on engine transcript state (report 01 already demoted engine stores to resume-optimization). Viable only for short tasks that never approach the threshold.

**B. Fresh-context-per-stage with a platform-owned handoff ledger (fresh session per pipeline stage; state on disk).**
Each stage = one engine session sized to fit; the platform assembles the next stage's context from a structured ledger + injected knowledge slices; compaction is a never-expected safety net. *For*: the strongest controlled evidence in this report (CONCAT 95.1%; fresh-restart is mitigation rung 1); first-party doctrine (Anthropic harness, Agent SDK advice); decorrelates errors (kills self-conditioning); makes D7 checkpoints and 4.3 freshness re-validation natural (the ledger *is* the checkpoint's context payload); per-stage cost becomes ledger+slice (~2–6K tokens) instead of transcript re-send (~100K+) — the structural anti-15× fix at the stage dimension, complementing report 06's helper firewall. *Against*: handoff quality becomes load-bearing (underspecified handoffs are the plan-loop failure mode); stage boundaries forfeit cache prefix reuse (a deliberate, priced trade: boundaries are rare relative to turns; flat-rate lanes price this in latency, not dollars); slightly more platform machinery.

**C. Retrieval-based assembly (embedding index over house knowledge/conventions; similarity-select per task).**
*For*: no rules to maintain; scales to huge corpora. *Against*: the entire convention-injection field is deterministic; retrieval quality failures inject wrong context silently (S4.6's trace requirement gets noisy); non-reproducible assembly breaks 11.1 auditability and cache stability; Sinet's corpus (per-project conventions, a curated house KB) is registry-sized, not web-sized; the post-mortem explicitly flags the two-retrieval-philosophies trap. Retrieval may earn a place inside T10's episodic memory later — not in v0 assembly.

**D. Engine-native memory/compaction as the platform mechanism (memory tool + server-side compaction / Managed Agents memory).**
*For*: adopt-don't-fork purity; vendor-maintained. *Against*: the GA memory tool is an *API-lane* feature (client-side backend, Sinet would implement the file ops anyway); server-side compaction is beta and Anthropic-only — a portability trap against D3's two-lane reality; Managed Agents memory is beta on a different product surface; none of it exists on the GLM/local lanes. Use these as *lane-local optimizations* where free, never as the platform's state model.

---

## 4. Recommendation for Sinet

**Primary: B — fresh-context-per-stage on a platform-owned Task Context Ledger, with deterministic registry-driven injection, lane-aware cache posture, and compaction demoted to an audited safety net.** Numbers marked ⚙ are proposed defaults for G1 ratification, not evidence-fixed constants.

**4.1 Session/stage model (feeds G1 with T02).** A task's pipeline stages (intake → plan → execute-N → verify, per reports 03/04) each run as a **fresh engine session**; within a stage the session is continuous. Stage working sets are *planned to fit*: target ≤ ⚙50% of the lane's window at stage start, with a platform **overflow event at ⚙70%** (measured by the adapter's token accounting, which D4 requires anyway). On overflow the platform proposes a stage split — consolidate-to-ledger + fresh session — rather than waiting for engine compaction (the field reacts at 85–95%; the evidence says plan below the effective window: NoLiMa/OOLONG/Epoch all place degradation well under advertised lengths). Helper sub-sessions inside a stage follow report 06's contract unchanged.

**4.2 The Task Context Ledger (the D7-integrated handoff artifact).** One structured markdown/JSON document per task in the task workspace, platform-owned, engine-agnostic — the synthesis of Anthropic's feature-list+progress-file, HANDOFF.md practice, Manus recitation, and opencode's summary schema. Sections (v0 ⚙):

- `objective & acceptance criteria` — verbatim from the spec stage; **pinned** (never summarized, always re-injected whole — ConstraintRot's constraint-pinning result made mechanism);
- `constraints & danger zones` — task-level rules + the repo's danger zones; **pinned**;
- `decisions` — append-only log with one-line reasons (the "actions carry implicit decisions" record);
- `state` — done items with *verified* status (never "written" — Anthropic's rule), current item, next actions, blockers;
- `artifacts` — file paths/URLs with one-line descriptions (restorable references, Manus rule: drop bulk, keep the pointer);
- `learned-this-task` — instance notes that expire with the task (7.8's per-run instance layer, feeds T10's proposal pipeline, never auto-persists — S4.3).

Written at every stage end and updated at D7 checkpoints (the ledger is the "context state" a checkpoint must capture); the next stage's context = ledger + injected slices + stage brief. Long *within-stage* sessions recite: the coordinator re-reads state/next-actions at intervals (recitation evidence). **The ledger is also the resume medium**: 4.3 freshness re-validation is a platform pass — a cheap local-model comparison of ledger `state`/`artifacts`/`decisions` against live reality (git log since checkpoint, source re-checks) → continue / adjust-with-note / escalate. No engine has this; the Anthropic harness session-start protocol is the template, the Codex stale-resume bug the cautionary class.

**4.3 Deterministic injection with a trace manifest (S4.6/11.1/S2.1).** The control plane resolves, per stage, a **rules-based selection**: project registry → that repo's AGENTS.md + danger zones (S1.6); task domain → the matching playbook slice from the house KB (S4.5); user → their overlay (7.8); task → the ledger. Selection is pure lookup (registry keys, path/glob rules) — no embeddings in v0 (§2.6: field-universal for conventions, auditable, reproducible, cache-stable). Injection mechanics per lane: opencode `instructions` array / session prompt assembly; Claude lane via the workspace's CLAUDE.md shim + prompt assembly (and SessionStart-hook `additionalContext` where the wrapped CLI supports it — spike at implementation). **Assembly order is stability-sorted** (house/static → project → user overlay → task ledger → stage brief): most-stable-first serves both cache prefix stability (Manus append-only doctrine) and a deterministic frame; 8.9's precedence (task > project > personal > house) is handled by explicit precedence labels in the frame, not by ordering. **Every injected item is logged to the run trace as an injection manifest: `{item, source path, content hash, version, selector rule}`** — this is 11.1/S2.1's "auditable memory use" made concrete, and it is Sinet-owned schema (OTel GenAI is Development-status with content capture off by default; map `gen_ai.system_instructions` later, don't bet on it now). P45 note: injected conventions are spec-side context; report 06's clean-context reviewer keeps its second axis by receiving the acceptance criteria and the diff, *not* the executor's overlay/history.

**4.4 Convention files (S1.6, N18 revised).** Canonical per-repo file: **AGENTS.md** (LF standard; opencode-native), with a one-line **CLAUDE.md import shim** (`@AGENTS.md`) for the Claude lane; the platform's repo-registration task generates both. Content contract (⚙): ≤150 lines; verified commands (build/test/lint with expected outputs), non-standard conventions only, danger zones, closure criteria ("what command proves done"); **no architecture overviews or directory trees** (measurably unhelpful — ETH ablation; Anthropic's own `/doctor` trims exactly this). Generation: the S1.6 onboarding task drafts it (engine `/init`-class), then a **mandatory human-prune gate** before adoption (LLM-generated-unedited is measurably negative; gate = D10-consistent approval). Upkeep: staleness check as a scheduled platform task (repo-diff-triggered re-validation proposing an update PR), plus a **shim-drift check** (AGENTS.md/CLAUDE.md divergence is a platform-detectable defect).

**4.5 Cache posture per lane (D5).** The platform's job is **prefix hygiene, not cache management**: stable stage frames (stability-sorted assembly, 4.3), append-only within a stage, never mutate injected files mid-stage, one workspace cwd per task on the Claude lane (cache scopes to cwd), don't churn tool sets mid-stage (mask, don't remove — report 06 contract). Record `cache_read/creation_input_tokens` per call into receipts (D4); **consumption pressure counts uncached input at 1.0 and cache reads at ⚙0.1** (mirrors the API ITPM rule and the Enterprise seat-pool wording) **while the receipt keeps raw counts** — the weighting is explicitly marked "assumed, unverifiable" until Anthropic publishes subscription quota semantics (open question #1). GLM lane: pressure unit is **prompts, not tokens** — stage design there batches work per prompt (15–20 calls each) and caching is latency-only. Metered exceptions: report 06's economics apply unchanged; verify opencode's `cache_control` breakpoints actually land per provider (known non-Claude bug #14642) in the adapter conformance suite.

**4.6 Compaction stance (adopt-don't-fork applied).** Compaction is a **safety net with an audit, never a planned mechanism**: (a) stages are budgeted so it should not fire (4.1); (b) opencode lane — keep `compaction.auto: true` with defaults (keep.tokens 8000 / buffer 20000) as the net; (c) Claude lane — accept auto-compact as uncontrollable-by-design, wire **SessionStart(source:"compact") to re-inject the pinned ledger sections** (objective, acceptance criteria, constraints) immediately after any compaction, and spike PreCompact-blocking on the pinned version (docs say blockable; April issue disagreed); (d) **every compaction event is logged as a stage-design defect signal** with a post-compaction fidelity check: a local-model pass confirming pinned items still govern behavior (ConstraintRot's canary method — inject a known constraint, verify post-compact adherence — goes into the platform's conformance suite, per P45's spirit: verify the verifier's memory). Engine compaction internals are never rebuilt or patched (deepclaude boundary, §6).

**4.7 Measurement (solo-scale, T11 handoff).** Three cheap loops: (1) **ablation A/Bs** on the added-value benchmark suite (post-mortem inversion 3) with config axes — conventions on/off, ledger on/off, stage-split thresholds — promptfoo-shaped, `--repeat` for variance; (2) **compaction/handoff fidelity checklist** (constraints/decisions/paths present and governing?) run by a local model — the permanent free tier; (3) **context-relevance audit** (RAG-triad-style LLM-judge over the injection manifest: was each injected slice used/relevant?) sampled, not universal. Cache-hit and window-burn from local usage parsing (ccusage-class) feed the D4 meters.

**What would change the decision:** (1) a controlled fresh-vs-continuous head-to-head showing continuous+compaction matching fresh-stage quality at equal budget would relax 4.1 toward longer sessions; (2) Claude Code reading AGENTS.md natively (issue #6235) removes the shim; (3) Anthropic publishing subscription quota semantics fixes the ⚙0.1 weighting; (4) OTel GenAI content-capture stabilizing converts the manifest to standard export; (5) engine-native cross-session memory arriving on *both* lanes as config-only could absorb parts of the ledger — evaluated then, behind the same Sinet contract; (6) a next-generation OOLONG-class eval showing whole-context integration solved at 256K+ would raise stage-fit targets substantially.

---

## 5. What NOT to use and why

- **Continuous-session-plus-compaction as the primary architecture.** The engines' default, and the measured worst idea here: constraints silently dropped (0%→30–59%), ≈53pp knowledge loss over three cascades, uncontrollable and version-unstable on the Claude lane, self-conditioning accumulation. Compaction is the airbag, not the steering.
- **Embedding-RAG for conventions/playbook selection.** Field-universal deterministic practice, auditability (11.1), reproducibility, cache stability, corpus size, and the post-mortem's two-retrieval-philosophies trap all point one way. Revisit only inside T10 for episodic memory.
- **Tool-RAG / mid-task dynamic tool loading.** Manus/Schmid prohibition; breaks caches and confuses selection; report 06's mask-don't-remove already governs.
- **Unedited auto-generated AGENTS.md / architecture-map-heavy convention files.** Measurably ≤0 value at +20% cost (controlled ablation); Anthropic's own tooling trims what generators love to write. Human-prune gate is mandatory, and the map goes in the registry, not the prompt.
- **Betting injection provenance on OTel GenAI semconv today.** Development-status, content capture off by default, attributes-vs-events unsettled. Own the schema; export later.
- **Any third-party client on Anthropic subscription auth (opencode-on-Claude-sub).** Blocked at the API since 2026-03; opencode removed OAuth at Anthropic's request. Wrapped first-party CLI only (re-confirms report 01/02).
- **Trusting engine session stores as the checkpoint or the handoff.** Report 01 settled this; the Codex stale-resume failure class and cwd-keyed lookup hazards reinforce it. Engine resume is an optimization; the ledger + Sinet's event log are the record (D7).
- **A mem0/Letta-class memory sidecar in v0 assembly.** No winning architecture (§2.1); assembly-side needs are met deterministically; storage-side evaluation belongs to T10 with S4's layering as the spec.
- **Fighting Claude Code's compactor** (disable hacks, threshold raising). Verified impossible by design; design stages below it instead.

---

## 6. Harvest-map verdicts

- **A4 — Archon `fresh_context` per loop iteration (context half): CONFIRM, and promote.** The pattern ("fresh session per iteration, plan artifact as the handoff") is no longer just elegant — it is mitigation rung 1 with controlled evidence (CONCAT restores 95.1%; multi-turn −39%/+112% is what it avoids) and first-party doctrine (Anthropic harness; Agent SDK sessions guidance). Promote from "pattern for long-running workflow tasks" to **the platform's default session model at stage boundaries**, with one sharpening: fresh context *without* a structured, pinned handoff artifact is where plan-loops fail — the ledger (4.2) is the load-bearing half. (Orchestration half was verdicted in report 06.)
- **N18 — Repo onboarding template: REVISE.** Keep: read-only gated task; ruthless-concise <150 lines; verified commands, conventions, danger zones; one-gated-task onboarding (S1.6). Revise: (1) target file is **AGENTS.md + CLAUDE.md import shim**, not a proprietary format — the standard won (Linux Foundation, 60K+ repos, opencode-native); (2) **drop "architecture map" from the template** — repo overviews are measurably unhelpful (controlled ablation) and Anthropic's `/doctor` trims exactly them; the architecture map belongs in the project registry for *humans and planners*, not in every prompt; (3) add a **mandatory human-prune gate** (LLM-generated-unedited ≈ negative value) and a staleness/shim-drift re-check task.
- **deepclaude / "Design Space" reference row: CONFIRM the boundary, with a corollary.** The five-stage compaction pipeline is real and verified (arXiv:2604.14228: budget → snip → microcompact → collapse → auto-compact) and engine-native — Sinet builds none of it. Corollary sharpened by this run: because the Claude lane's pipeline is *uncontrollable* (no disable, capped thresholds, version drift), "don't rebuild it" implies "**design around it**" — stage-fit budgets, pinned re-injection on `source:"compact"`, compaction-as-defect-signal — and engine compaction behavior joins the report-01 canary suite (S2.8).

---

## 7. Open questions

1. **Cache-read weighting in consumption pressure (extends report-06 OQ).** Subscription quota semantics are confirmed unpublished. Proposed ⚙ default: pressure counts cache reads at 0.1×, receipts keep raw; label "assumed." Ratify at G1; revisit on any Anthropic publication. *Owner: operator (G1); T08 implements.*
2. **Stage-fit budget numbers.** ⚙50% target / ⚙70% overflow-event are proposed from effective-window evidence, not measured on Sinet's tasks. Ratify at G1; calibrate per model generation via T11's benchmark (OOLONG-class whole-context checks per lane model). *Owner: operator (G1) + T11.*
3. **PreCompact/PostCompact blocking on the pinned Claude Code version.** Docs say blockable; an April issue said not; version-dependent. One-day spike at implementation start; result decides whether the platform can *hold* compaction for ledger flush or only react post-hoc. *Owner: implementation spike; log into the canary suite.*
4. **Ledger schema v0 ratification.** Section set (4.2), pinning rules, and its exact relation to the D7 checkpoint payload and T07's durable-state design. *Owner: spec workshop; consumes this report + T07.*
5. **Claude-lane injection mechanics.** CLAUDE.md-shim vs prompt-assembly vs SessionStart `additionalContext` for per-stage slices — cache and precedence implications differ; needs a spike on the wrapped CLI. *Owner: implementation spike.*
6. **GLM-lane stage granularity.** Prompt-count quota (15–20 calls/prompt) inverts the economics of fine-grained stages on that lane; stage-split policy may need lane-aware cost weighting. *Owner: T08 (metering/scheduling), with report-02 lane data.*
7. **New platform problems found (for the spec's Known-problems list):**
   - **Compaction safety as tested behavior**: pinned-item survival must be conformance-tested (canary constraint → compact → adherence check), not assumed — P45's lesson applied to memory. *Owner: T11/spec.*
   - **Injected-knowledge drift**: AGENTS.md↔CLAUDE.md shim divergence and conventions-vs-repo staleness are silent quality leaks; detection is a schedulable platform task. *Owner: spec (S1.6/S2.8).*
   - **Lane-heterogeneous consumption units** (tokens vs prompts vs requests) complicate D5's cross-lane pressure comparison — pressure needs per-lane normalization. *Owner: T08.*
   - **Engine compaction version drift**: thresholds/behavior change under pinned-version upgrades; compaction behavior joins the adapter canary suite. *Owner: spec (S2.8).*
8. **Prior-report consistency check**: no contradictions found. Report 06's caching-vs-subscription-quota open question is *sharpened* (absence now primary-verified, weighting proposal added), and report 01's engine direction is independently reinforced (third-party OAuth blocking). The spec's known-problem mapping for context rot (4.3, S4.2, S4.10) is **incomplete**: rot is primarily a *within-run* phenomenon — the session/stage model and stage budgets (this report) should be added as owners alongside the freshness/memory items. *Owner: spec edit at G1.*

---

## 8. Sources

All accessed 2026-07-17. Tier noted where not obvious. (95 numbered entries — a few lines carry two URLs, ≈98 unique URLs — plus 2 internal reports.)

**First-party doctrine & API (Anthropic)**
1. https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents — canon: attention budget, minimal high-signal tokens, JIT retrieval, compaction/notes/subagents (2025-09-29).
2. https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents — "compaction isn't sufficient"; initializer + fresh sessions + disk artifacts (2025-11-26).
3. https://www.anthropic.com/engineering/multi-agent-research-system — 4×/15× token multipliers, subagent isolation (2025-06; standing via report 06).
4. https://platform.claude.com/docs/en/build-with-claude/context-editing — context editing beta, clear_tool_uses 100K default, SDK compaction deprecation.
5. https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool — memory tool GA; auto-injected "ASSUME INTERRUPTION"; multisession pattern.
6. https://platform.claude.com/cookbook/tool-use-context-engineering-context-engineering-tools — server-side compaction `compact-2026-01-12`, 150K default trigger, worked budget example (2026-03-20).
7. https://claude.com/blog/context-management — context-editing/memory launch evals (+29%/+39%/−84%) (2025-09-29; vendor-internal tier).
8. https://code.claude.com/docs/en/hooks — PreCompact can block; PostCompact; SessionStart source:"compact".
9. https://code.claude.com/docs/en/agent-sdk/sessions — JSONL persistence, resume/fork, fresh-session-with-app-state advice.
10. https://code.claude.com/docs/en/memory — CLAUDE.md-not-AGENTS.md, @AGENTS.md import, <200 lines, /init, /doctor trim heuristic, `.claude/rules` paths:, InstructionsLoaded hook.
11. https://code.claude.com/docs/en/prompt-caching — auto-caching; subscription 1h TTL free; subagent 5-min separate caches; cwd-scoped prefixes; usage fields.
12. https://platform.claude.com/docs/en/api/rate-limits — cache-aware ITPM (reads excluded), Haiku 3.5 exception, 2M→10M worked example.
13. https://claude.com/blog/token-saving-updates — cache-aware ITPM origin (2025-03-13).
14. https://support.claude.com/en/articles/11647753-how-do-usage-and-length-limits-work — Pro/Max limits qualitative; no token unit; no cache mention (absence, 3/3 verified).
15. https://support.claude.com/en/articles/14552983-models-usage-and-limits-in-claude-code — Enterprise seat-pool cache-read-rate wording [single-source].
16. https://support.claude.com/en/articles/12429409-manage-extra-usage-for-paid-claude-plans — extra usage at standard API rates.

**Framework & practitioner canon**
17. https://www.langchain.com/blog/context-engineering-for-agents — write/select/compress/isolate; Breunig failure modes (2025-07-02; >12 mo, flagged).
18. https://www.langchain.com/blog/how-and-when-to-build-multi-agent-systems — read-parallel/write-serial (2025-06; standing via report 06).
19. https://docs.langchain.com/oss/python/deepagents/context-engineering — 85% trigger, 20K offload + path/preview, compact-between-tasks tool; no per-stage budgets.
20. https://github.com/humanlayer/12-factor-agents/blob/main/content/factor-03-own-your-context-window.md — own-your-context-window.
21. https://manus.im/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus — KV-cache doctrine, restorable compression, recitation, file-system-as-context (2025-07-18; production tier).
22. https://cognition.com/blog/dont-build-multi-agents — continuous single-thread pole; compressor-LLM "hard to get right" (2025-06).
23. https://cognition.com/blog/multi-agents-working — 2026 revision; clean-context reviewers (2026-04-22).
24. https://www.philschmid.de/context-engineering-part-2 — effective-window practice (~128–256K), anti-tool-RAG (2025-12-04).
25. https://www.newsletter.swirlai.com/p/state-of-context-engineering-in-2026 — 2026 convergence + live disputes; per-component token costs (2026-03-22) [single-source].
26. https://arxiv.org/abs/2507.13334 — 1,400-paper context-engineering survey (2025-07).

**Context-rot & mitigation evidence**
27. https://www.trychroma.com/research/context-rot — 18-model degradation; distractors; shuffled>structured; focused>full (2025-07-14; vendor-benchmark).
28. https://arxiv.org/abs/2502.05167 — NoLiMa: 11/13 <50% of baseline at 32K; GPT-4o 99.3→69.7 (ICML 2025).
29. https://github.com/adobe-research/NoLiMa — per-model results table (updated 2025-07).
30. https://arxiv.org/abs/2511.02817 — OOLONG: frontier <50% at 128K on whole-context reasoning (2025-11).
31. https://arxiv.org/abs/2505.06120 — multi-turn: −39%, +112% unreliability, CONCAT 95.1%, RECAP (2025-05; ICLR 2026).
32. https://arxiv.org/abs/2410.10813 — LongMemEval: ~30% drop, full-history vs retrieval (2024-10; flagged dated).
33. https://arxiv.org/abs/2404.06654 — RULER effective-vs-claimed length (2024; historical baseline, superseded models).
34. https://epoch.ai/data-insights/context-windows — 250× 9-month trendline; <80% at 192K (2025-06).
35. https://www.vellum.ai/blog/claude-opus-4-6-benchmarks — MRCR v2 256K/1M figures (2026-02; vendor-derived).
36. https://arxiv.org/abs/2508.21433 — Complexity Trap: masking ≈ summarization at half cost (2025-08).
37. https://arxiv.org/abs/2606.00408 — masking inverted-U regime map (2026-05).
38. https://arxiv.org/abs/2604.02547 — trajectory-length/failure correlation reverses under difficulty control (2026-04) [single-source].
39. https://arxiv.org/abs/2512.22087 — ReAct saturation ~60 rounds; bounded-context agent (2025-12; method-paper tier) [single-source].
40. https://arxiv.org/abs/2511.13998 — LoCoBench-Agent: agentic long-context robustness counterpoint (2025-11).
41. https://arxiv.org/abs/2606.11213 — compaction failure-mode taxonomy (2026-06; abstract-level).
42. https://arxiv.org/abs/2606.22528 — ConstraintRot: 0%→30–59% violations post-compaction; constraint pinning (2026-06).
43. https://arxiv.org/pdf/2605.24657 — cascading compaction: 90.1%→36.8% over 3 cycles (≈53pp, derived) (2026).
44. https://dev.to/gabrielanhaia/lost-in-the-middle-is-still-real-in-2026-even-on-1m-token-models-2ehj — position-bias replication on Sonnet 4.5 (2026-04; practitioner, small-N).

**Session architecture & engines**
45. https://v2.opencode.ai/compaction — opencode compaction config/formula/summary schema; prune no-op.
46. https://opencode.ai/docs/rules/ — AGENTS.md native, CLAUDE.md fallback, instructions array (globs/remote), /init.
47. https://github.com/anomalyco/opencode/issues/7576 — nested AGENTS.md auto-selection open [single-source].
48. https://arxiv.org/html/2604.14228v1 — "Dive into Claude Code": five-stage native pipeline (2026-04).
49. https://github.com/anthropics/claude-code/issues/42817 — auto-compact cannot be disabled; Math.min cap; closed not-planned (2026-05).
50. https://codex.danielvaughan.com/2026/04/14/context-compaction-deep-dive-codex-cli-claude-code-opencode/ — cross-engine compaction comparison; compact-at-boundaries advice (2026-04, upd. 2026-07).
51. https://decodeclaude.com/compaction-deep-dive/ — microcompaction internals (practitioner).
52. https://claudefa.st/blog/guide/mechanics/context-buffer-management — reserved-buffer figures (practitioner, snippet-tier).
53. https://docs.bswen.com/blog/2026-03-21-claude-code-auto-compact-settings/ — threshold env var (practitioner, snippet-tier).
54. https://docs.bswen.com/blog/2026-02-09-claude-context-loss-compaction/ — context-loss reports (practitioner, snippet-tier).
55. https://bytebell.ai/blog/claude-code-compacting-losing-work/ — mid-debug compaction failures (practitioner, snippet-tier).
56. https://www.codecentric.de/en/knowledge-hub/blog/the-ralph-wiggum-loop-autonomous-code-generation-with-a-fresh-context — Ralph loop mechanics/results (2026-04).
57. https://ralph-wiggum.ai/ — Ralph reference (snippet).
58. https://fazm.ai/blog/claude-code-architecture-handoff-pattern — HANDOFF.md contents (2026-03).
59. https://github.com/openai/codex/issues/31982 — stale-checkpoint resume failure class [single-source].
60. https://knightli.com/en/2026/07/10/ai-agent-long-task-resume-guide/ — resume re-validation practice (2026-07, snippet-tier).
61. https://medium.com/@ThinkingLoop/agent-caches-that-quietly-go-stale-8c0b0d4ea1af — verify-live-state-before-decisions (snippet-tier).
62. https://x.com/walden_yan/status/2047054554433462360 — Cognition position update (2026) [single-source, snippet].
63. https://www.edtechinnovationhub.com/news/anthropic-brings-persistent-memory-to-claude-managed-agents-in-public-beta — Managed Agents memory beta (2026-04).

**Convention files & injection provenance**
64. https://agents.md/ — spec, nearest-file precedence, adopter list (living; adopter list is marketing-adjacent).
65. https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation — AAIF stewardship; 60K+ projects (2025-12-09).
66. https://github.com/anthropics/claude-code/issues/6235 — AGENTS.md request open since 2025-08-21.
67. https://gist.github.com/yurukusa/d36197848911f025add142abefcde685 — fallback-myth debunk; sync patterns (practitioner).
68. https://learn.chatgpt.com/docs/agent-configuration/agents-md — Codex root-down merge, 32 KiB cap.
69. https://cursor.com/help/customization/rules — rule modes, nesting, <500 lines, AGENTS.md native.
70. https://geminicli.com/docs/cli/gemini-md/ + https://github.com/google-gemini/gemini-cli/issues/12345 — GEMINI.md default; AGENTS.md-by-default declined.
71. https://arxiv.org/abs/2602.11988 — controlled context-file ablation: no general gain, +20% cost; human ≈+4% (2/3 verifiers located) vs LLM-gen negative; overviews unhelpful (2026-02, rev. 2026-06).
72. https://arxiv.org/abs/2601.20404 — observational: −28.6% runtime, −16.6% tokens with AGENTS.md (2026-01).
73. https://academy.dair.ai/blog/agents-md-evaluation — secondary write-up of 71 (numbers split).
74. https://blakecrosley.com/blog/agents-md-patterns — command-first patterns; <150 lines (2026-02; practitioner).
75. https://codersera.com/blog/agents-md-complete-guide-2026/ — ecosystem snapshot, frontmatter proposal (2026-05).
76. https://dosu.dev/blog/how-to-catch-documentation-drift-claude-code-github-actions — doc-drift CI pattern (2026-03).
77. https://opentelemetry.io/blog/2026/genai-observability/ — GenAI semconv status; content capture off by default; gen_ai.system_instructions.
78. https://oneuptime.com/blog/post/2026-02-06-capture-genai-prompt-completion-events-opentelemetry/view — event-based capture in practice; PII warning.
79. https://langfuse.com/docs/prompt-management/features/link-to-traces — prompt-version↔trace linkage.
80. https://mastra.ai/articles/agent-memory + https://mem0.ai/blog/memory-retrieval-strategies-for-ai-agents — retrieval-side memory practice and its injection-timing warning.

**Caching (non-Anthropic lanes) & measurement**
81. https://github.com/anomalyco/opencode/issues/17910 — third-party OAuth blocked at Anthropic API (2026-03-17); opencode dropped OAuth (thread-verified mechanism).
82. https://github.com/anomalyco/opencode/issues/14642 — opencode cache_control breakpoint logic + non-Claude bug [single-source].
83. https://developers.openai.com/api/docs/guides/prompt-caching — auto ≥1024; no rate-limit relief; GPT-5.6+ writes 1.25×; ttl "30m".
84. https://developers.openai.com/cookbook/examples/prompt_caching_201 — measured TTFT 7%→67% (2026-02).
85. https://docs.z.ai/devpack/faq — GLM coding-plan prompt-count quota; 15–20 calls/prompt; 3×/2× peak multipliers; no caching mention.
86. https://docs.z.ai/guides/capabilities/cache — Z.AI API-lane automatic caching.
87. https://api-docs.deepseek.com/guides/kv_cache/ — DeepSeek disk cache, best-effort, usage fields.
88. https://api-docs.deepseek.com/news/news0802/ — DeepSeek cache launch economics (2024; flagged dated).
89. https://arxiv.org/html/2601.06007v2 — "Don't Break the Cache": TTFT 6–31% by provider; 41–80% cost; naive-caching latency paradox (2026-01).
90. https://docs.ragas.io/en/stable/concepts/metrics/available_metrics/ — context precision/recall metric definitions.
91. https://www.trulens.org/getting_started/core_concepts/rag_triad/ — RAG triad, LLM-judge, no per-query ground truth.
92. https://www.promptfoo.dev/docs/guides/evaluate-coding-agents/ — agent evals: rubrics, trajectory/cost/latency assertions, repeats (upd. 2026-07-16).
93. https://github.com/ryoppippi/ccusage — local JSONL usage/cache/window parsing (v20, 2026-07).
94. https://www.mindstudio.ai/blog/anthropic-prompt-caching-claude-subscription-limits — corroborates subscription-weighting absence (2026-04; practitioner).
95. https://www.morphllm.com/compaction-vs-summarization — compaction-fidelity scoring precedent [single-source, secondary].

**Internal:** Research/01-execution-engines-and-adapters.md (substrate, adapter verbs, checkpoint doctrine); Research/06-orchestration-and-multiagent.md (helper firewall, API cache economics, context-editing evals, caching-vs-quota OQ).
