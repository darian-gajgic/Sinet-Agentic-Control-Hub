# 06 — Orchestration & multi-agent architecture (within D6)

**Topic:** T03 · **Wave:** A2 · **Depth:** FULL · **Date:** 2026-07-17
**Method:** deep-research harness — 5 fan-out search agents (debate evolution, deep-agents pattern, context passing, dynamic composition/failure semantics, cost/mesh evidence), primary-source fetching, 35 load-bearing claims put through 3-vote adversarial verification (0 killed; 3 corrected in detail). All URLs accessed 2026-07-17 unless noted.

---

## 1. Scope

Feature-list items covered: **D6** (single coordinator per task, isolated helpers, sub-helper depth cap default 2, no lateral messaging, every spawn logged with reason), **2.4** (automatic orchestration within the fixed topology — best practice chooses structure and depth per task *within* the cap), **7.7** (routing accountability), **14.1** (no chatting swarms), **14.4** (no standing army — machinery only when a task earns it).

Binding context: the topology itself is FIXED — this report researches everything inside it: when to spawn helpers at all, how to decompose, how context goes down and results come up without the ~15×-class token multiplication the Nexus post-mortem measured, and how per-task dynamic composition should work. Substrate assumption per report 01 §4 (standing, pending G1): helpers are engine **sibling sessions** spawned and logged by Sinet's own control plane (pinned `opencode serve` per user + wrapped `claude` CLI per user); engine-native nesting stays at its safe one-level default. Feeds: **G1** (default orchestration policy), coordinator/helper lifecycle spec, **T14** (worker composition).

---

## 2. Current state of the art (mid-2026)

### 2.1 Where the single-vs-multi-agent debate landed

The debate has a precise arc, and it ends in convergence, not victory:

- **2025-06-12 — Cognition, "Don't Build Multi-Agents"** (Walden Yan): parallel subagents are fragile because "actions carry implicit decisions, and conflicting decisions carry bad results"; prescribed single-threaded agents that "share context, and share full agent traces, not just individual messages" (cognition.com/blog/dont-build-multi-agents; practitioner tier; verified verbatim).
- **2025-06-13 — Anthropic, multi-agent research system**: orchestrator-worker (Opus 4 lead + Sonnet 4 subagents) "outperformed single-agent Claude Opus 4 by 90.2% on our internal research eval" — but the same post reports that on BrowseComp "token usage by itself explains 80% of the variance" (three factors explain 95%), that "agents typically use about 4× more tokens than chat interactions, and multi-agent systems use about 15× more tokens than chats", and that shared-context/high-dependency domains — explicitly "most coding tasks" — are a poor fit (anthropic.com/engineering/multi-agent-research-system; production tier; internal eval, no replication — single-source for the 90.2%).
- **2025-06-10/16 — LangChain**: the reconciliation that stuck — "read actions are inherently more parallelizable than write actions" (langchain.com/blog/how-and-when-to-build-multi-agent-systems), plus benchmark evidence that a supervisor's paraphrase relay is a "telephone game" and that removing handoff noise + relaying worker output **verbatim** (`forward_message`) was worth ~50% supervisor improvement (langchain.com/blog/benchmarking-multi-agent-architectures; benchmark tier).
- **2026-01-23 — Anthropic's successor guidance** is notably more conservative than its 2025 post: subagents are justified by exactly three criteria — **context protection** (isolating polluting subtasks), **parallelization** of independent facets, and **specialization** (agents with "often 20+" tools mis-select); overhead restated as "3-10× more tokens than single-agent approaches for equivalent tasks"; and it names the role-based pipeline (separate planner/executor/reviewer agents) an anti-pattern that loses context at every handoff — prescribing **context-centric decomposition** instead (claude.com/blog/building-multi-agent-systems-when-and-how-to-use-them; official-docs tier).
- **2026-04-22 — Cognition revises**: Yan now ships multi-agent in production, but only "a narrower class of patterns": **"writes stay single-threaded"**; working patterns are a clean-context code-review agent (self-reported "~2 bugs per PR", "roughly 58% are severe" — and reviewers perform *better without* shared context, Cognition's own counterexample to its 2025 share-everything principle), a callable frontier expert, and manager-child delegation in isolated VMs. Parallel writers and "unstructured peer-to-peer agent networks" remain "mostly a distraction" (cognition.com/blog/multi-agents-working; production tier, self-reported numbers).
- **2026 academic wave**: (a) Tran & Kiela — under **equal thinking-token budgets**, "single-agent systems consistently match or outperform MAS on multi-hop reasoning"; reported MAS advantages are "better explained by unaccounted computation and context effects" (arxiv.org/abs/2604.02460). (b) **Google Research, "Towards a Science of Scaling Agent Systems"** (arxiv.org/abs/2512.08296, v3 2026-04-08; 260 configurations, 5 architectures, 3 model families — the most rigorous controlled study to date): multi-agent vs single-agent performance ranges **+80.8% (decomposable financial reasoning) to −70.0% (sequential planning)**; independent multi-agent systems **amplified errors 17.2×** while centralized (hub-and-spoke) systems **contained amplification to 4.4×**; above **~45% single-agent accuracy, adding agents yields diminishing-to-negative returns**; token overhead vs a single agent on the same task: **independent +58%, centralized +285%, decentralized +263%, hybrid +515%** (paper-body verified). (c) MAST failure taxonomy (arxiv.org/abs/2503.13657, v3): 41–86.7% failure rates across 7 open-source MAS on 1,600+ traces — with the caveat that this measured 2024–25-era frameworks, several of which no longer exist in that form.

**Where it landed:** every first-party position converged on the same shape — **one coordinator, ephemeral isolated helpers, compressed reports back, no lateral messaging, single-agent by default**. That is D6, independently re-derived by Anthropic (its Managed Agents product hard-codes depth 1), Cognition (manager-child, single-threaded writes), Google (centralized beats independent on error containment), LangChain/deepagents, and Microsoft (Agent Framework 1.0 replaced AutoGen's group chat with explicit graph workflows). The genuine open dispute is only *how much* delegation buys at equal budget — and for Sinet, flat-rate subscription lanes change that calculus (see §2.4).

### 2.2 Task shapes: where delegation wins and loses (the read/write asymmetry)

Delegation demonstrably **wins** (≥2 independent sources each):

| Task shape | Evidence | Magnitude |
|---|---|---|
| Breadth-first read fan-out (multi-source research, verification sweeps, large read-only surveys) | Anthropic 2025 (production), Google 2512.08296 (controlled) | +90.2% (token-confounded, internal) / up to +80.8% on decomposable tasks |
| Context protection — even a *single* helper for a noisy read subtask (grep sweeps, log trawls, doc digestion) | Anthropic 2026-01 guidance; Claude Code docs worked example | helper read 6,100 tokens of files → 420-token result to parent (~14:1 context compression) |
| Clean-context judging/review of finished work | Cognition 2026-04 (reviewers better *without* shared context); debate-for-LLM-judges niche (arxiv.org/abs/2510.12697) | qualitative production claim + narrow benchmark wins |

Delegation demonstrably **loses**:

- **Sequential / tightly-coupled work**: Google measured −39% to −70% across multi-agent variants on sequential planning — "communication overhead fragmented the reasoning process."
- **Most coding/write work**: Anthropic (2025, verbatim), Cognition (2025 and 2026), and the entire tooling field agree.
- **Anything a single agent already does well**: Google's ~45% accuracy threshold; ChromaFlow's small negative ablation (aggressive orchestration dropped GAIA L1 accuracy 54.7%→50.9% while adding operational noise; arxiv.org/abs/2605.14102 — single-author, 53 tasks, low weight but directionally consistent).
- **Equal-budget multi-hop reasoning**: Tran & Kiela.

**The write side, mid-2026:** parallel *writers in a shared workspace* remain evidenced-unreliable; an adversarial search found zero credible successes. Every production idiom achieves write parallelism by **removing sharing**, not coordinating it: Cursor 2.0 runs parallel agents "powered by git worktrees or remote machines" (and its headline win is best-of-N on the *same* task, not decomposition; cursor.com/blog/2-0, 2025-10-29); Devin's "Managed Devins" (2026-03-19) put each child in its own isolated VM; Claude Code ships `isolation: worktree` and `/batch` (5–30 worktree-isolated subagents); its multi-session "agent teams" feature — the only lateral-messaging surface in the field's leading harness — is **experimental and disabled by default** (code.claude.com/docs/en/agents, live). Practitioner ceilings cluster at 3–10 parallel agents before merge/review overhead eats gains, with shared hotspot files (routes, configs, registries) still producing conflicts even under worktrees (htdocs.dev, 2026-04-04; practitioner tier). **Single-writer-per-workspace is the standing rule; parallel writing is a separate-task/worktree concern, not a helper concern.**

### 2.3 The deep-agents pattern today

The pattern (detailed long prompt + planning artifact + filesystem + subagents), named by LangChain in July 2025 as a generalization of Claude Code / Deep Research / Manus, is now the reference architecture of every major harness:

- **Reference implementations:** Claude Code / Claude Agent SDK (origin; subagents with description-based routing, TODO tools, plan mode); **deepagents** — now "The batteries-included agent harness", stable 0.6.12 (2026-06-25), 0.7.0 alphas through a7 (2026-07-14), with middleware, pluggable backends, skills, and published default numbers (github.com/langchain-ai/deepagents); **Microsoft Agent Framework** shipped an "Agent Harness" at Build 2026 with the same component set (compaction, FileMemoryProvider, TodoProvider, plan/execute modes, background child agents) — convergence by imitation (devblogs.microsoft.com, 2026-06-03).
- **Measured results are bundle-level, not ablations.** The harness variable is real and quantified: Terminal-Bench 2.1 same-model deltas — Claude Code + Fable 5 at 83.8%±1.2 vs the minimal Terminus 2 harness at 80.4%±1.2 (+3.4 pts); Codex + GPT-5.5 83.1%±1.1 vs 78.0%±1.2 (+5.1 pts) (tbench.ai/leaderboard/terminal-bench/2.1, live; do not read the 83.8-vs-83.1 cross-harness gap as a ranking — error bars overlap). LangChain reports harness-layer changes alone moved gpt-5.2-codex 52.8%→66.5% on Terminal-Bench 2.0 (langchain.com/blog/improving-deep-agents-with-harness-engineering and /deep-agents-0-6; vendor-benchmark tier; the two posts' publication dates were reported inconsistently by verification — Feb vs May/June 2026 — the numbers themselves verified verbatim twice).
- **The planning artifact is contested inside its own pattern.** Manus canonized todo-recitation (2025-07-18), then measured "roughly one-third of all actions were spent updating the todo list" and moved to a **planner sub-agent returning a structured Plan object** (rlancemartin.github.io/2025/10/15/manus/; corroborated by philschmid.de, 2025-12-04). Counter-evidence exists on the accuracy axis (TodoEvolve, arxiv.org/abs/2602.07839: global TODO planning improves accuracy) — so the retreat was token-economics, not effectiveness. No public on/off ablation of the todo tool exists. Every major implementation still ships one.
- **Documented failure modes:** evidence integration/verification and plan resilience — not decomposition — dominate deep-research agent failures (FINDER/DEFT, arxiv.org/abs/2512.01948); MAST's top modes are spec-disobedience, step repetition, and missing/incorrect verification; deepagents live tracker shows the sharp edges: parallel subagent calls are **all cancelled when one fails** — confirmed intentional fail-fast, not a bug, per the maintainer (github.com/langchain-ai/deepagents/issues/694) — and subagents starting blank by design drives recurring "let helpers inherit parent history" feature pressure. Practitioner-reported recurring failures: self-grading bias, early stopping, poor decomposition (Addy Osmani, 2026-04-19; secondary, uncited numbers).

### 2.4 Cost structure: the real multipliers and what actually moves them

**Anatomy of the multiplier (this section is the anti-15× design basis):**

- Anthropic's famous numbers compare against *chat*: agents ≈4× chat; multi-agent ≈15× chat. So multi-agent ≈ **3–4× a single agent on the same task** — matching Anthropic's own 2026 restatement ("3-10× more tokens than single-agent approaches") and Google's controlled per-task overheads (**centralized +285%** ≈ 3.85×; independent +58%; hybrid +515%).
- The Nexus post-mortem's ~15×-class blowup came from *re-sending context per pipeline stage* — exactly the mechanism the field's mitigations target. A 15×-class outcome is what an **undesigned** context contract produces (full re-sends, paraphrase chains, cold caches); ~2–4× is the designed-for expectation of a coordinator + helpers run.
- Drivers, per all sources: per-helper context re-loading, tool-result duplication across contexts, coordination turns, and paraphrase-based relaying.

**Mitigations, ranked by evidence:**

1. **Prompt caching** — the biggest measured lever on API-priced lanes: cache reads bill ~0.1× base input, writes 1.25× (5-min TTL) / 2× (1-hr); measured end-to-end savings in agentic settings are **41–80%** (arxiv.org/abs/2601.06007, 500+ sessions — below the "up to 90%" marketing number). Two multi-agent-specific traps are documented in the provider docs: a cache entry "only becomes available after the first response begins" — so **N simultaneously-launched identical-prefix helpers all pay full price** (stagger the first); and caches are prefix-scoped, so each helper *template* is its own prefix to warm (platform.claude.com/docs/en/build-with-claude/prompt-caching). Manus's production doctrine: KV-cache hit rate is "the single most important metric", input:output ≈ 100:1, append-only stable prefixes (manus.im blog, 2025-07-18).
2. **Report-out compression + filesystem references**: helpers return condensed reports "often 1,000-2,000 tokens" (anthropic.com/engineering/effective-context-engineering-for-ai-agents, 2025-09-29) and "store their work in external systems, then pass lightweight references back" (Anthropic 2025). deepagents productizes the thresholds: tool results >20,000 tokens are offloaded to the backend and replaced by a file path + first-10-lines preview; history summarization triggers at 85% of the model's input window (docs.langchain.com/oss/python/deepagents/context-engineering, live).
3. **Verbatim relay instead of paraphrase**: the supervisor "telephone game" fix (~50% performance improvement, and it also deletes a regeneration cost) — LangChain benchmark + langgraph-supervisor's built-in forward tool.
4. **Model tiering**: Opus-lead + Sonnet-workers is the production pattern (Anthropic 2025); cheap/local helper models for mechanical subtasks have obvious headroom but **no controlled 2026 study quantifies the quality loss** — flagged as a gap.
5. **Context editing/compaction**: Anthropic's vendor evals: context editing cut token consumption **84%** in a 100-turn eval; memory+editing improved agentic search 39% (claude.com/blog/context-management, 2025-09-29; vendor-benchmark tier). One 2026 caution: stale-observation masking follows an inverted-U — gains mid-capacity, collapse when overdone (arxiv.org/abs/2606.00408).

**D5 nuance for Sinet:** on flat-rate lanes the mitigation currency is *consumption pressure and provider-limit headroom*, not dollars. Caching discounts are an API-billing construct; whether cached tokens weigh less against **subscription** quota windows is publicly unspecified — logged as an open question (§7). Local models are the permanent free tier for mechanical helper work regardless.

### 2.5 Mechanisms in the wild: spawn decisions, budgets, depth, failure, observability

- **How systems decide to spawn:** universally, the model decides against **description fields** of available helper types (Claude Code/Agent SDK `description`-based routing; opencode subagent `description` with auto/@mention/task-tool invocation), steered by **rubrics in the coordinator prompt** because models judge effort badly on their own. Anthropic's production rubric is explicit: "simple fact-finding requires just 1 agent with 3-10 tool calls; direct comparisons might need 2-4 subagents with 10-15 calls each; complex research might use more than 10 subagents" — added after early versions spawned ~50 subagents for trivial queries. The 2026-01 guidance adds the three-criteria test (context protection / parallelization / specialization). Programmatic **factory patterns** (build helper definitions at query time) are documented in the Agent SDK.
- **Depth caps have no industry consensus — and engines don't reliably enforce them:** Anthropic Managed Agents: **depth 1, hard** ("depth > 1 is ignored"; roster 1–20; max 25 concurrent threads; platform.claude.com/docs/en/managed-agents/multi-agent, live, beta). Claude Code: depth **5**, fixed, non-configurable (since v2.1.172). OpenAI Codex: default max_depth=1, configurable (openai/codex #9912). opencode: a 1-level guard that a global `"task": "allow"` **silently disables** (github.com/anomalyco/opencode/issues/17721), with a documented runaway to 47 sessions across 20 nesting levels and the depth-limit request **closed as not-planned** (issue #18100, closed 2026-07-04). D6's cap of 2 sits inside the field envelope (no production system demonstrates value at depth ≥3), but the enforcement lesson is blunt: **the cap must live in Sinet's control plane, not in engine config.**
- **Budgets:** per-helper step/turn caps are standard (opencode `steps`; Claude Code `maxTurns` + `effort`; OpenAI Agents SDK `max_turns`, default 10). Concurrency: Claude Code ~10 concurrent observed with queueing (practitioner-measured, undocumented, not user-tunable — cuong.io, 2025-06-24, with open feature requests confirming); Managed Agents 25; `/batch` 5–30. Observed per-helper spend in a real research fan-out: 45k–70k tokens and 17–28 tool calls each (single practitioner source).

  The mechanism field at a glance (why D6 enforcement must be Sinet-owned):

  | System | Spawn decision | Depth cap | Concurrency | Per-helper budget | Spawn *reason* logged? |
  |---|---|---|---|---|---|
  | Claude Code / Agent SDK | model vs `description` + prompt rubric | 5, fixed, non-configurable | ~10 observed, queued, not tunable | `maxTurns`, `effort`, tool allowlist | no — prompt + parent lineage only |
  | Managed Agents (beta) | coordinator vs declared roster (≤20) | 1, hard (deeper ignored) | 25 threads | per-thread stats/usage | no — thread events only |
  | opencode | model vs `description` / @mention / task tool | 1-level guard, silently bypassable; no enforced limit | undocumented | `steps` cap | no — session tree only |
  | OpenAI Agents SDK / Codex | handoffs or agents-as-tools | Codex default 1, configurable | n/a | `max_turns` (default 10) | no — traces only |
  | deepagents | model vs registered descriptions; scripted `task()` | undocumented | async since v0.5 | middleware-level | no |
  | **Sinet target (D6)** | **control-plane spawn API + trigger rubric** | **2, control-plane-enforced** | **proposed 4/task ⚙** | **turns + token ceiling** | **REQUIRED — own schema (§4.3)** |
- **Failure semantics:** the field's best current pattern is Claude Code's: an API error mid-helper returns **partial output plus a note it didn't finish** (v2.1.199), helpers are resumable by id, and since v2.1.210 the harness **scans helper final messages for instruction-shaped patterns** and neutralizes them (marker insertion, never rewording) — with the docs' own caveat that scanning "isn't a substitute for restricting what a subagent can reach" (code.claude.com/docs/en/sub-agents, live). Anthropic's production reliability kit: resume-from-error-point, checkpoints, rainbow deployments. deepagents cancels siblings on one failure by default (intentional). MAST's structural finding: most multi-agent failures are **specification and verification failures, not model failures** — the empirical case for spawn-time contracts and acceptance checks on helper output. No public production postmortem of a cascading multi-agent failure exists; the widely quoted 41–86.7% figures are benchmark correctness rates, not incident data.
- **Observability / spawn-reason logging:** OpenTelemetry GenAI conventions define `create_agent`/`invoke_agent`/`execute_tool` spans with `gen_ai.agent.*` attributes — still **Development** status mid-migration, and **no attribute for spawn reason exists anywhere** (grep-confirmed; corroborated by arxiv.org/abs/2606.09692 on audit schemas lacking delegation semantics). Nearest equivalents are the persisted task `description`/`prompt` and parent-child span lineage. **D6's "every spawn logged with its reason" exceeds current industry practice — Sinet must define its own schema.**
- **The mid-2026 frontier is script-driven orchestration:** LangChain's dynamic subagents (2026-06-29) have the model write a short QuickJS script that loops/branches over a `task()` primitive instead of issuing per-helper tool calls — "deterministic coverage at scale"; Claude Code's `Workflow` tool / dynamic workflows do the same for dozens-to-hundreds of subagents. Notably, neither documents budgets, depth limits, or failure handling for the scripted path yet. The convergence signal matters for Sinet: the field is moving *toward* deterministic code orchestrating model-proposed decomposition — which is exactly what a control plane spawning logged sibling sessions is.

---

## 3. Candidate approaches

**A. Always-single-agent (never spawn).** Maximally cheap and simple; trivially D6-compliant. Rejected as a *policy* (kept as the *default formation*): it forfeits the two cheapest, best-evidenced wins — context protection on noisy reads (~14:1 compression) and read fan-out on genuinely parallel facets — and Sinet's flat-rate lanes mean marginal helper cost is consumption pressure, not dollars.

**B. Static role pipelines (planner→executor→reviewer agents as the standing shape).** Named anti-pattern in Anthropic's 2026 guidance ("lost context at each handoff", "more tokens on coordination than actual work"); MAST's failure categories are dominated by exactly this shape's coordination failures; it is the Nexus 15×-trap topology; 14.4 forbids it as a default. Rejected.

**C. Coordinator + earned, per-task helpers, spawned by the control plane as engine sibling sessions, under a strict context contract.** The field's convergent shape (Anthropic production + Managed Agents, Cognition manager-child, Google's centralized-wins data, deepagents/Claude Code defaults), implementable today on both report-01 substrates with per-helper model/tool/budget control, and it gives D6's logging/cap/no-lateral guarantees in Sinet-owned code rather than engine config. **Chosen — detailed in §4.**

**D. Engine-native nested delegation as the D6 mechanism** (coordinator's engine session spawns its own subagents via the engine's task tool). Rejected as the *primary* mechanism: opencode's guard is silently disableable and its depth-limit request is closed not-planned; Claude Code's cap is 5 and non-configurable (wrong number, unownable); logging-with-reason would be reconstructed from event streams rather than enforced at spawn. A narrow exception is worth a spike (§7): allowing the coordinator's *own* engine-native subagents at depth 1 for micro-fanouts, **only if** the adapter can observe and log every native spawn from the event stream — otherwise disable via config (no `task` permission / no `Agent` tool) and route all delegation through control-plane siblings.

**E. Script-driven mass orchestration (dynamic-workflows lane).** The frontier for 100+-item mechanical fan-outs (migrations, sweep-verify jobs). Premature for v0 — the pattern is weeks old, ships without documented budget/failure semantics, and Sinet's control plane can add a deterministic fan-out primitive of its own when a real workload earns it. Revisit post-benchmark-gate.

---

## 4. Recommendation for Sinet

**Primary: single-agent-first, coordinator-mediated, earned helper fan-out under a context-firewall contract — all topology mechanics owned by the control plane.** Concretely, as G1 defaults (numbers marked ⚙ are proposed defaults for the operator to ratify, not evidence-fixed constants):

**4.1 When to spawn (the earned-helper test).** Default formation is ONE agent (14.4). The coordinator may propose helpers only when at least one evidence-backed trigger holds, and the trigger class is recorded as the spawn reason (7.7):

- **T-CTX (context protection):** the subtask would flood the coordinator with material it won't reference again (grep/log/doc trawls, large read-ins). Even one helper pays here.
- **T-PAR (read fan-out):** ≥2 genuinely independent, read-only facets whose results merge only at synthesis. Never for write work.
- **T-SPEC (specialization/quarantine):** the subtask needs a tool/permission set the coordinator shouldn't hold (including: parsing untrusted content — the injection blast-radius case), or a materially different model/effort tier.
- **Effort rubric in the coordinator prompt** (models can't self-judge effort — Anthropic production lesson): simple lookup → 0 helpers, do it inline; one noisy read → 1 helper; independent facets → 2–4 helpers; beyond that requires the task's spawn budget (below). **Writes are single-threaded, always**: one writer per workspace; parallel write *tasks* exist only as separate tasks in isolated worktrees (D9-adjacent), never as helpers of one task.

Formation rubric (2.4's "best practice chooses structure and depth per task" made concrete — this table is the default policy):

| Task shape | Default formation | Evidence anchor |
|---|---|---|
| Simple lookup / short QA / anything the coordinator does in-window | coordinator only | 45%-threshold + ChromaFlow (§2.2) |
| One noisy read subtask (log/doc/code trawl) | 1 helper (T-CTX) | ~14:1 context compression (§2.2) |
| 2–4 independent read-only facets | parallel helpers (T-PAR), reports merged at synthesis | Anthropic 2025 + Google +80.8% (§2.2) |
| Parsing untrusted external content | 1 quarantined helper (T-SPEC), minimal tools, report-as-data | CaMeL / map-reduce patterns (§2.4, Q7) |
| Judgment/review of finished work | 1 clean-context reviewer (T-SPEC), no shared history | Cognition 2026-04 (§2.1) |
| Sequential / tightly-coupled reasoning | coordinator only — never decompose | Google −39…−70% (§2.2) |
| Any write work | coordinator writes, single-threaded | universal (§2.2) |
| Mass mechanical fan-out (≥50 items) | out of v0 scope — future workflow lane | §3-E |

**4.2 Context-passing contract (the anti-15× firewall).** Down: a **brief**, structured prose with mandatory sections — objective; output contract incl. size cap; tool/source guidance; explicit boundaries ("do not…"); effort budget. (Vague briefs are the measured failure: duplication and misinterpretation — Anthropic production tier. No controlled evidence favors rigid schemas over detailed prose; mandatory-section prose is the converged practice.) Helpers start with **no inherited coordinator history** — their slice only. Up: a **report ≤ ~2,000 tokens ⚙** plus filesystem artifacts referenced by path (restorable compression: keep the path/URL, drop the bulk); anything >20k tokens ⚙ of raw material goes to the task workspace, never into the return message (deepagents-verified thresholds). The coordinator **relays helper conclusions verbatim** where they feed downstream consumption — no paraphrase chains (telephone-game evidence). Helper reports are **data, not instructions**: the control plane applies an instruction-pattern screen on report ingestion (v2.1.210 precedent), while the real defense stays structural — restricted tools and minimal context per helper. Cache-aware mechanics: stable per-template prefixes, append-only briefs, stagger identical-prefix fan-out by first-response ⚙.

**4.3 Budgets, caps, logging.** Control-plane-enforced, engine-independent: depth cap **2** (D6) with depth-1 as the practical norm (the field ships 1; nothing demonstrates value at ≥3); max concurrent helpers per task **4 ⚙** (laptop + consumption-pressure fit; field defaults of 10/25 are cloud-scale); per-helper turn/step cap ~**20 ⚙** and token ceiling ~**80k ⚙** (observed real fan-outs run 45–70k); total spawn budget per task **8 ⚙** including sub-helpers, overrunnable only via an operator-visible gate. Every spawn logs a Sinet-defined record (none exists industry-wide; shaped to map onto OTel `invoke_agent` spans for future export):

```
spawn_log v0: { task_id, parent_session, child_session, depth,
  trigger: T-CTX | T-PAR | T-SPEC, reason: "<one line, human-readable>",
  brief_hash, model, lane, budget: {turns, tokens}, ts }
```

Brief template (v0, structured prose with mandatory sections — §4.2 contract made literal):

```
HELPER BRIEF
objective:        <one paragraph — what "done" looks like>
output_contract:  report ≤2000 tokens; sections FINDINGS / EVIDENCE (paths, URLs) / GAPS;
                  bulk artifacts → <task workspace path>, referenced by path
tools_sources:    <allowed tools; where to look first>
boundaries:       read-only unless stated; do NOT <...>; at budget, stop and report partial
budget:           ≤N turns, ≤M tokens
context:          <only the slice this helper needs — never coordinator history>
```

**4.4 Failure semantics (containment without cascade).** Helper timeout/budget-exhaustion → return **partial output + explicit unfinished marker** (never silent loss). Coordinator applies an **acceptance check** before consuming any report — contract conformance (sections present, size, boundaries respected) plus a cheap plausibility screen on a *second, spec-independent axis* (P45; local model is the free tier for this; depth of check is a T04-adjacent open question). Junk/failed report → **one retry as a fresh helper with a revised brief** (fork-don't-poison), then the coordinator either proceeds without that facet (recording the gap in its own report) or escalates to the human gate — and the escalation route is test-proven (P46). **Sibling isolation is mandatory**: one helper's failure never cancels siblings or the task (the field's default is the opposite — fail-fast — so this is Sinet-owned logic). Helper death is a scheduling event, not a task failure (D4 parallels).

**4.5 D7/D5 integration.** A helper is paid work: the control plane checkpoints brief-at-spawn, report-at-return, and usage per paid call, so a dead helper costs a resume, never spent tokens (D7). Helper model choice routes by consumption pressure between flat-rate lanes (D5): mechanical fan-out (extraction, classification, screening) defaults to **local models** — the permanent free tier — frontier flat-rate for judgment-bearing reads; per-spawn model+lane lands in the receipt (7.7, 9.3).

**4.6 Helper lifecycle (the states the spec should encode).** PROPOSED (coordinator emits spawn request with trigger + reason) → SPAWNED (control plane validates depth/budget, writes spawn_log, starts sibling session) → RUNNING (streams; checkpoints per paid call, D7) → RETURNED (report received) → acceptance check → ACCEPTED (report integrated; verbatim relay downstream) | REJECTED (one retry: fresh helper, revised brief) | SALVAGED (budget/timeout/API death → partial output + explicit unfinished marker). Terminal alternatives: FAILED (facet gap recorded in coordinator report; siblings unaffected) and ESCALATED (second rejection → human gate; route test-proven, P46). Every transition is a logged event; nothing exits RETURNED without an acceptance verdict — a helper finding that dies unread is a platform defect, not a shrug (P46's spirit applied to delegation).

**What would change this decision:** (1) equal-budget evidence extending from reasoning benchmarks to tool-use/research workloads would narrow the spawn triggers further toward T-CTX-only; (2) a first-party, subscription-covered orchestration surface GAing on a Sinet lane (Managed-Agents-shaped) would be evaluated as the Anthropic-lane *mechanism* behind the same Sinet contract — the policy layer is deliberately mechanism-agnostic; (3) credible measured evidence of reliable parallel *writers* would unfreeze the single-writer rule (currently zero such evidence); (4) clarified subscription-quota semantics for cached tokens could reweight the fan-out staggering policy; (5) scripted-orchestration maturing with budget/failure semantics would add a deterministic mass-fan-out lane post-gate.

---

## 5. What NOT to use and why

- **Peer-mesh / group-chat / free-form lateral topologies** (excluded by D6; re-validated — see §6 verdict): centralized containment beats independent by ~4× on error amplification (17.2× vs 4.4×, controlled); multi-agent *discussion* "erases up to 72% of issue-critical facts" with stance homogenization and single-malicious-agent poisoning (arxiv.org/abs/2606.03032); MAST's inter-agent misalignment category; and the market itself retreated — Microsoft folded AutoGen's group chat into graph workflows (1.0, 2026-04-03; predecessors maintenance-only), OpenAI deprecated Swarm into the supervised Agents SDK, CrewAI's production story became deterministic Flows. Honest counter-evidence, none of which reopens D6: A2A's real adoption (150+ orgs) is *structured cross-system task delegation*, not conversational mesh — and its production claims are press-release tier with no independent postmortem; bounded few-round debate has a measured niche for **LLM-as-judge evaluation only**; MoA-style ensembles and one hierarchical-team security result are boundary cases at the "spend more compute" frontier, not topology evidence.
- **Standing specialist rosters / role-based default formations** (Nexus's 18-specialist shape): the named 2026 anti-pattern (Anthropic), the shape MAST's failure taxonomy eats, and 14.4 already forbids it. Specialists are palette entries a task may earn, never a formation.
- **Full-history handoffs as the default context contract** (OpenAI Agents SDK default: the receiving agent "gets to see the entire previous conversation history"): this is the 15×-trap encoded as an SDK default; passing full content is consistently expensive and rarely best-performing (arxiv.org/abs/2606.05304). Sinet's brief/report contract is the inverse; note OpenAI itself now ships opt-in history-collapse mitigations.
- **Engine-native nesting as the D6 enforcement mechanism**: opencode's guard is silently defeated by a common config line and the depth-limit issue is closed not-planned; Claude Code's cap is 5, fixed; Managed Agents is beta and depth-1. No engine implements "depth 2, logged, no lateral" — Sinet's control plane does (per report 01's sibling-session direction).
- **Unbounded or rubric-free spawning at model discretion**: the two documented runaways (Anthropic's ~50-subagent trivial-query anecdote; opencode's 47-session/20-level recursion) both trace to exactly this.
- **Paraphrase-relay coordination** (coordinator restates helper output): measured "telephone game" degradation; relay verbatim.
- **Claude Code agent teams / any lateral-messaging feature**: experimental, disabled by default at the vendor itself; out of scope under D6 regardless.
- **deepagents as a dependency**: see §6 — the engines already are Sinet's harness; adopting a second, fast-churning harness (0.7 alphas breaking 0.6 behavior within weeks) buys duplicate machinery and a LangChain-stack coupling for zero capability Sinet lacks.

---

## 6. Harvest-map verdicts

| Item | Verdict | Detail |
|---|---|---|
| Anti-harvest: **peer-mesh / group-chat topologies** ("evidence-excluded — cascade falsehoods, cost multiplication") | **CONFIRM — strengthened** | 2026 evidence is better than what the exclusion was written on: controlled error-amplification numbers (17.2× vs 4.4×), fact-erasure measurements (72%), highest token overheads in the class (decentralized +263%, hybrid +515%), and revealed-preference retreats by every group-chat-native vendor. Rider: A2A-style structured cross-org delegation is a *different object* (RPC-shaped, hub-compatible) — its adoption does not weaken the exclusion, and it stays a watchlist item, not a topology change. |
| Anti-harvest: **Nexus 18-specialist standing roster** | **CONFIRM** | Single-agent-first is now the *documented* posture of Anthropic (three-criteria test), Cognition ("narrower class of patterns"), and Google (≥45%-accuracy threshold for negative returns). Role-based standing formations are the named anti-pattern of 2026. Specialists survive only as palette options a spawn trigger can justify per task. |
| Reference row: **deepagents (LangChain)** — "vocabulary and sanity check, redundant as a dependency" | **CONFIRM as reference, still redundant as dependency — upgraded as evidence source** | Redundancy confirmed: Sinet's adopted engines natively provide the whole deep-agents component set, and deepagents churns fast (0.6.12 → 0.7.0a7 with breaking changes inside 3 weeks). Upgrade: it is now the field's best-documented reference implementation — its **published defaults (85% summarization trigger; 20k-token offload with path+preview; single-final-report subagents) are concrete numbers Sinet's own contract should mirror**, and its issue tracker is a free failure-mode canary (fail-fast siblings, blank-start context pressure). Watch, mirror numbers, never depend. |
| **N22 — specialist prompt bodies as palette raw material** (sanity check only; full composition is T14) | **CONFIRM at sanity level** | Consistent with everything above: prompt bodies as a *palette* the composer may draw from when a T-SPEC trigger fires matches the field's "specialization only when earned" doctrine. One steer for T14: 2026 evidence favors specialization by **context/tool slice** (what the helper sees and may touch) over persona prose — palette entries should be tool+context bundles first, role text second. |

Adjacent rows touched by this evidence (no verdict owed here): A4 (fresh-context-per-iteration with plan-artifact handoff) and S2 (plan-strong/execute-cheap) both align with the verified pattern set (fresh helper contexts; lead-strong/worker-cheap tiering).

---

## 7. Open questions

**Operator decisions (G1):**
1. Ratify the §4 defaults: spawn triggers as stated; concurrent-helper cap 4; per-helper ~20 turns / ~80k tokens; per-task spawn budget 8; report cap ~2k tokens. All ⚙ numbers are proposals with field-envelope rationale, not measured optima. *Owner: operator at G1.*
2. Engine-native micro-fanout exception (§3-D): permit coordinator-session native subagents at depth 1 *if* the adapter can log every native spawn from the event stream? Needs a 1-day spike on both substrates (opencode task events; claude stream-json subagent events) before deciding. *Owner: adapter spike (report 01 G1 rider).*
3. Acceptance-check depth on helper reports: contract-conformance only, or plus local-model plausibility screen by default? (P45's second axis; cost is free-tier but latency isn't.) Interlocks with report 04's verification ladder. *Owner: operator, with T04 cross-reference.*

**Later research / watchlist:**
4. **NEW PLATFORM PROBLEM — subscription-quota semantics of prompt caching:** cache discounts are documented for API billing only; whether cached reads weigh less against *subscription* limit windows is unpublished for every lane. Materially changes fan-out staggering policy and D5 consumption accounting. *Owner: provider watchlist (report 02 machinery).*
5. **NEW PLATFORM PROBLEM — sibling-failure containment is Sinet-owned:** the field defaults to fail-fast sibling cancellation (deepagents, intentionally) or undefined behavior; D6/D7 require isolation + partial-result salvage. Must be an explicit, tested control-plane behavior (P46-style: prove the containment path with a killed-helper test). *Owner: spec author (coordinator/helper lifecycle).*
6. **NEW PLATFORM PROBLEM — no engine enforces D6:** depth caps, spawn logging, and no-lateral are unenforceable via engine config alone (opencode guard bypass; fixed caps elsewhere). D6 enforcement = control-plane spawn API + conformance tests that *attempt* violations (recursion, lateral send, unlogged spawn) and assert refusal. *Owner: spec author; test plan at v0.*
7. **NEW PLATFORM PROBLEM — helper-report injection surface:** helper output is untrusted input to the coordinator (the reverse of context quarantine). Pattern-scanning exists (v2.1.210) but is explicitly insufficient; Sinet needs report-as-data discipline + per-helper tool restriction as the structural defense, and S5 confinement classes should say which helper classes may read untrusted external content at all. *Owner: S5/T-security adjacency.*
8. Managed-Agents-shaped first-party orchestration reaching subscription lanes (currently beta, API): if it GAs with subscription auth, evaluate as the Anthropic-lane mechanism behind Sinet's contract. *Owner: watchlist.*
9. Scripted mass-orchestration lane (dynamic workflows): revisit when a real ≥50-item mechanical fan-out workload exists post-benchmark-gate. *Owner: later research.*
10. Model-tiering quality tax: no controlled study quantifies quality loss from cheap/local helper models on judgment-bearing subtasks — worth a small pre-registered internal eval once the benchmark practice (N21) is running, since local-helper routing is a core D5 lever. *Owner: benchmark practice.*

**Contradiction notice (per shared-context rule):** none of this report's findings contradict reports 01–04. The brief's addendum (control-plane sibling sessions) is *reinforced* by the engine-enforcement evidence (§2.5, Q6).

---

## 8. Sources

Access date for all: **2026-07-17** (live fetches during this session's search/verification passes). Tier noted per source. Load-bearing claims above carry ≥2 independent sources unless explicitly flagged single-source.

**First-party doctrine & production accounts:**
1. https://www.anthropic.com/engineering/multi-agent-research-system — production; 90.2% (internal, single-source), 4×/15×, 80%-variance, effort rubric, coding poor-fit, reference-passing. (2025-06-13)
2. https://claude.com/blog/building-multi-agent-systems-when-and-how-to-use-them — official; three subagent criteria, 3-10× vs single agent, role-pipeline anti-pattern, context-centric decomposition. (2026-01-23)
3. https://cognition.com/blog/dont-build-multi-agents — practitioner/production; original share-context principles. (2025-06-12)
4. https://cognition.com/blog/multi-agents-working — production; 2026 revision: single-threaded writes, clean-context reviewer, manager-child. (2026-04-22)
5. https://x.com/walden_yan/status/2047054554433462360 — corroboration of #4's stance shift. (2026-04-22)
6. https://cognition.ai/blog/devin-can-now-manage-devins — production; Managed Devins, per-child isolated VMs. (2026-03-19)
7. https://manus.im/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus — production; KV-cache doctrine, 100:1, todo recitation, restorable compression. (2025-07-18)
8. https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents — official; compaction/notes/subagents, 1–2k-token reports. (2025-09-29)
9. https://claude.com/blog/context-management — vendor benchmark; 84% token cut, 39%/29% improvements. (2025-09-29)

**Controlled studies & academic evidence:**
10. https://arxiv.org/abs/2512.08296 (+ https://research.google/blog/towards-a-science-of-scaling-agent-systems-when-and-why-agent-systems-work/) — academic, controlled; +80.8%/−70.0%, 17.2× vs 4.4×, 45% threshold, per-architecture token overheads (paper-body verified), 87% of held-out configurations (R²≈0.37 caveat). (v3 2026-04-08; blog 2026-01-28)
11. https://arxiv.org/abs/2604.02460 — academic; single agents match/beat MAS at equal thinking budgets (Tran & Kiela). (2026-04-02)
12. https://arxiv.org/abs/2503.13657 — academic; MAST taxonomy: 41–86.7% failure, 14 modes, 1,600+ traces, interventions ≤+15.6% (v3 2025-10-26; 2024–25-era frameworks — aging caveat).
13. https://arxiv.org/abs/2606.03032 — academic; Deliberative Illusion: 72% fact erasure, homogenization, single-agent poisoning. (2026-06-02)
14. https://arxiv.org/abs/2605.14102 — academic, small-n; ChromaFlow negative orchestration ablation (54.7%→50.9%). (2026-05)
15. https://arxiv.org/abs/2512.01948 — academic; FINDER/DEFT deep-research failure modes (integration/verification/plan resilience). (2025-12-01)
16. https://arxiv.org/abs/2604.18071 — academic; 70-project harness survey (deeper nesting pairs with heavier context services). (2026-04-20)
17. https://arxiv.org/abs/2511.07784 — academic; debate drivers = model strength/diversity, majority pressure suppresses correction. (2025-11-11)
18. https://arxiv.org/abs/2510.12697 — academic; debate-for-LLM-judges niche positive. (2025-10)
19. https://arxiv.org/abs/2503.18813 — academic; CaMeL provable injection defense, 77% vs 84% AgentDojo (v2). (2025-06-24)
20. https://arxiv.org/abs/2601.06007 — academic benchmark; caching saves 41–80% cost, 13–31% TTFT in agent sessions. (2026-01)
21. https://arxiv.org/abs/2606.05304 — academic; full-content handoffs expensive, rarely best. (2026-06)
22. https://arxiv.org/abs/2602.07839 — academic; TodoEvolve: todo planning helps accuracy (counterweight to Manus retreat). (2026-02)
23. https://arxiv.org/abs/2605.01566 — academic; MoA compute-efficiency boundary case. (2026-05)
24. https://arxiv.org/abs/2606.00408 — academic; stale-context masking inverted-U caution. (2026-05-29)
25. https://arxiv.org/abs/2606.09692 — academic; audit schemas lack delegation semantics (spawn-reason gap corroboration). (2026-06)
26. https://arxiv.org/abs/2603.12229 — academic; LM teams as distributed systems (framing). (2026-03-12)

**Live platform docs (mechanisms):**
27. https://code.claude.com/docs/en/sub-agents — official; nesting v2.1.172/depth-5 fixed, output scanning v2.1.210 + "not a substitute" caveat. (live)
28. https://code.claude.com/docs/en/agent-sdk/subagents — official; description routing, AgentDefinition budgets, factory pattern, partial-output v2.1.199. (live)
29. https://code.claude.com/docs/en/context-window — official; 6,100→420-token worked example. (live)
30. https://code.claude.com/docs/en/agents — official; subagents vs teams (experimental, default-off), /batch 5–30 worktree-isolated, dynamic workflows. (live)
31. https://platform.claude.com/docs/en/managed-agents/multi-agent — official, beta; roster ≤20, depth-1 hard, 25 threads, filesystem-shared/history-not, cross-posted permissions. (live; vendor-docs-only — no independent verification in the wild)
32. https://platform.claude.com/docs/en/build-with-claude/prompt-caching — official; 0.1×/1.25×/2×, first-response cache availability (fan-out race). (live)
33. https://opencode.ai/docs/agents/ — official; description/mode/steps/permission.task. (live, updated 2026-07-16)
34. https://github.com/anomalyco/opencode/issues/17721 — bug report, source-verified; global task:allow silently disables nesting guard. (closed 2026-03)
35. https://github.com/anomalyco/opencode/issues/18100 — bug report; 47-session/20-level runaway; depth limit closed not-planned. (closed 2026-07-04)
36. https://openai.github.io/openai-agents-python/handoffs/ (+ /multi_agent/, /running_agents/) — official; full-history handoff default, input_filter, max_turns=10. (live)
37. https://github.com/openai/codex/issues/9912 — official tracker; Codex default max_depth=1, configurable. (2026)
38. https://github.com/open-telemetry/semantic-conventions-genai — official, pre-stable; agent spans, no spawn-reason attribute. (live)

**deepagents / harness line:**
39. https://github.com/langchain-ai/deepagents (+ PyPI) — official; "batteries-included agent harness", 0.6.12 stable, 0.7.0a7. (live)
40. https://docs.langchain.com/oss/python/deepagents/context-engineering — official; 85% summarization trigger, 20k offload + path/preview, single final report. (live)
41. https://docs.langchain.com/oss/python/releases/changelog — official; v0.4–v0.7 evolution (sandboxes, async subagents, DeltaChannel, HarnessProfile). (live)
42. https://www.langchain.com/blog/deep-agents — practitioner; pattern named/defined. (2025-07-30)
43. https://www.langchain.com/blog/doubling-down-on-deepagents — official; v0.2 backends, large-result eviction. (2025-10-28)
44. https://www.langchain.com/blog/improving-deep-agents-with-harness-engineering + https://www.langchain.com/blog/deep-agents-0-6 — vendor benchmark; 52.8%→66.5% harness-only delta; DeltaChannel storage claims. (2026; publication dates reported inconsistently across fetches — numbers verified verbatim twice, dates flagged)
45. https://www.langchain.com/blog/introducing-dynamic-subagents-in-deep-agents — official; scripted orchestration (QuickJS task()). (2026-06-29)
46. https://github.com/langchain-ai/deepagents/issues/694 — tracker; fail-fast sibling cancellation, maintainer-confirmed intentional. (closed 2026-05-13)
47. https://www.langchain.com/blog/how-and-when-to-build-multi-agent-systems — practitioner; read/write asymmetry. (2025-06-16)
48. https://www.langchain.com/blog/benchmarking-multi-agent-architectures — benchmark; telephone game, forward_message ≈50%. (2025-06-10)
49. https://www.langchain.com/blog/context-engineering-for-agents — practitioner; write/select/compress/isolate taxonomy. (2025-07-02)
50. https://reference.langchain.com/python/langgraph-supervisor — official; handoff tools, forward-verbatim option. (live)

**Ecosystem status & consolidation:**
51. https://devblogs.microsoft.com/agent-framework/migrate-your-semantic-kernel-and-autogen-projects-to-microsoft-agent-framework-release-candidate/ (+ Build-2026 announce post) — official; AutoGen/SK maintenance-mode, graph workflows, Agent Harness clone. (2025-10 / 2026-06-03)
52. https://visualstudiomagazine.com/articles/2026/04/06/microsoft-ships-production-ready-agent-framework-1-0-for-net-and-python.aspx — secondary corroboration of MAF 1.0 GA 2026-04-03.
53. https://github.com/openai/swarm — official; deprecated → Agents SDK. (status verified live)
54. https://www.linuxfoundation.org/press/a2a-protocol-surpasses-150-organizations-lands-in-major-cloud-platforms-and-sees-enterprise-production-use-in-first-year — marketing/consortium tier; A2A adoption counter-case. (2026-04-09)
55. https://cursor.com/blog/2-0 — production; worktree/remote-machine parallel agents, best-of-N. (2025-10-29)
56. https://www.tbench.ai/leaderboard/terminal-bench/2.1 — benchmark; same-model harness deltas 3.4/5.1 pts. (live)

**Practitioner (weighted accordingly):**
57. https://rlancemartin.github.io/2025/10/15/manus/ — practitioner; Manus todo retreat (~1/3 actions), planner subagent, tiered context passing. (2025-10-15)
58. https://www.philschmid.de/context-engineering-part-2 — practitioner corroboration of #57. (2025-12-04)
59. https://cuong.io/blog/2025/06/24-claude-code-subagent-deep-dive — practitioner measurement; ~10-concurrent cap, 45–70k tokens/helper (single-source). (2025-06-24)
60. https://simonwillison.net/2025/Jun/13/prompt-injection-design-patterns/ (reviewing arXiv 2506.08837) — practitioner/academic; map-reduce, dual-LLM, context-minimization patterns. (2025-06-13)
61. https://www.dbreunig.com/2025/06/26/how-to-fix-your-context.html — practitioner; "context quarantine" vocabulary origin. (2025-06-26)
62. https://htdocs.dev/posts/from-conductor-to-orchestrator-a-practical-guide-to-multi-agent-coding-in-2026/ — practitioner survey; worktree standard, 3–10-agent ceilings (asserted, not benchmarked). (2026-04-04)
63. https://agentmarketcap.ai/blog/2026/04/10/devin-parallel-sessions-multi-agent-concurrency — secondary; Devin product timeline. (2026-04-10)
64. Addy Osmani, "Agent Harness Engineering" — practitioner, uncited numbers; harness failure-mode list. (2026-04-19; directional corroboration only)
65. https://www.flowhunt.io/blog/multi-agent-ai-system/ — survey tier; convergence snapshot; its AORCHESTRA/OneFlow/incident-response numbers remain UNVERIFIED single-source and are not load-bearing here. (2026)
66. https://www.secondtalent.com/resources/how-enterprises-are-using-autogen/ — practitioner, soft numbers; group chat <15% of AutoGen production. (2026)
67. https://github.com/anthropics/claude-code/issues/15487 (+ #63938) — tracker; parallelism cap not user-configurable. (2026)
