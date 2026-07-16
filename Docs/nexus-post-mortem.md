# Nexus Post-Mortem — Prior Art (Own)

**Date:** 2026-07-16. **Subject:** Nexus Agent OS + Hermes (github.com/dariannixda-eng/Nexus-Agentic-Coding-Setup), built ~2026-07-07 → 2026-07-15 by the owner. **Purpose of this document:** input for briefing v2.2 (runbook Step 0.2) and standing reference for the rebuild. Source basis: the repo README and `docs/DOCUMENTATION.md` (904 lines, dated 2026-07-15), including its own §13 measured self-assessment — read that file directly for full detail.
**Companion:** `component-harvest-map-v1.md` carries the item-by-item harvest with adoption modes, target slices, and effort; this document carries the system description, the failure analysis, and the strategic conclusions.

---

## 1. What Nexus is

A self-hosted control plane + execution engine pair, single machine, built in ~9–10 days:

- **Nexus Agent OS** (control plane): FastAPI + SQLite, ~29k LoC Python (`server.py` ≈ 11,476 lines, 248 routes, 32 tables) + an 11.7k-line vanilla-JS SPA (24 views) on port 8777. Kanban tasks, Wizard v3 pipelines, Deep Plan mode, judge loop, Quality Autopilot, decision cards, deliverables with PR-style review, five memory systems, multi-user (scrypt, user-scoped everything), benchmarks, JARVIS voice/vision stack, 3D memory galaxy. 488 self-counting static checks + ~20 runtime e2e gates.
- **Hermes** (execution engine): pinned upstream agent framework v0.18.0 @ 048270fa with **8 guardian-tracked core modifications** (keystone: #6 per-session model/API routing), 18 active specialists (+28 retired), ~80 skills, 6 plugins, on port 8642. SHA-256 manifest guardian repairs drift.
- **Model economics as designed:** Z.AI GLM plans (glm-5.2 / 5.1 / 4.5-air / 5-turbo) do the primary work; Anthropic Opus 4.8 is the judgment tier only, reached via bridge commands (`cjudge`/`creview`/`cspec`/`cverify`/`cexec`/`cimprove`/`cdistill`) shelling to a logged-in `claude` CLI. qdrant + ollama for memory/embeddings; optional Langfuse.

## 2. What it proved (verified in production, per its own docs)

- **Resilience:** control/execution split with an orphan-harvest ladder — Nexus restarts never kill running AI work; a respawned lane classifies an orphan as active / finished (harvest from session history) / dead. Verified across restarts, suspend/wake, overnight SSE stalls, quota storms. Zero token waste.
- **Race-proof scheduling:** atomic SQLite CAS claiming, per-model slot gates, quota-storm backoff distinguishing load-shedding from real quota.
- **Gate discipline:** spec gates, plan gates, decision cards with single-card closure; Deep Plan's scaffolded interview out-questioned both direct-agent baselines in benchmarking.
- **Verification machinery:** the overhauled judge loop (SHIP/REVISE/REWRITE + SHIP-with-notes, stage contracts with N-A/UNVERIFIABLE-HERE, delta re-judge with frozen criteria, convergence detection, $ caps) — precisely specified, though its economics were overhauled 2026-07-13 and never A/B-validated.
- **Honest accounting:** frontier ledger parsing real dollars from the `claude -p` JSON envelope; API-equivalent USD comparison figures; a benchmark program with frozen baselines, blind scoring, pre-registered thresholds — and a published miss.

## 3. What failed — bench-02, measured

Task: a webshop build (judgment-heavy, one-shot). Results from Nexus's own §13:

| Arm | Quality | Cost | Wall clock |
|---|---|---|---|
| Opus 4.8 direct | 44/50 | $13.71 | 51 min |
| Fable 5 direct | 43/50 | $16.48 | 34 min |
| **Nexus** (Smart, full-auto) | **28/50** | **$241.23 API-equiv** | **2.5–3.3 h** |

Missed its own pre-registered success criterion on both axes: ~17× the cost for ~64% of the quality. n=1 (its own caveat), with runs 2–3 and a judge-economics A/B queued but never executed.

### Failure anatomy (four findings)
1. **Spec-compliance beat product sense.** A bad early design ("sort GPUs by VRAM desc") got frozen into the SPEC; five stages, nine judge rounds, and 99 green tests then faithfully *defended* it. Verification amplifies whatever objective it is given — spec-faithfulness ≠ goodness.
2. **A stage saw the problem and had no route to a human.** Code review flagged the cost-absurdity as "spec limitation → operator decides" — and no decision card was ever raised. The mechanism existed; that finding category wasn't wired to it. A wiring failure, not a design failure.
3. **No live-research habit.** GLM fabricated product names and prices; both direct arms researched the market unprompted. Data-bearing tasks need research injected by policy, not left to the model's initiative.
4. **Structural cost overhead.** The spec stage alone cost 3.5× an entire direct run; multi-stage pipelines re-send context per stage and per judge round. The multi-agent token multiplication from the literature (~15×), measured in its own ledger.

### Structural weaknesses (beyond the benchmark)
- **Executor ceiling:** cheap model does all primary work, frontier only judges — gates can bound damage but cannot create missing quality on taste-heavy tasks.
- **Engine fork:** 8 core-mods on a pinned engine; guardian makes the fork survivable but it *is* a fork — upgrade debt, drift risk, and the keystone feature (per-session models) is native in adoptable engines (opencode).
- **Monolith, bus factor 1:** 11.5k-line `server.py`, 11.7k-line SPA — no seams, one maintainer.
- **Breadth before validation:** JARVIS voice/vision/avatar, 3D memory galaxy, dictation, MeetingMode built before the core added-value bet was tested.
- **Two retrieval philosophies:** mem0/qdrant embeddings coexisting with the system's own deterministic-retrieval principle.
- **No window-aware scheduling:** slot caps and quota backoff exist, but no window modeling, no interactive-headroom reservation, no per-user window attribution.
- **No meta-agent:** all specialists and the house pipeline are hand-authored; the wizard repairs a fixed DAG but never composes new definitions.
- Machine-coupled deployment (live-tree symlinks, pinned-base patches); learning leakage between runs makes improvements unattributable; single-machine SQLite ceiling.

## 4. The three strategic inversions (what the rebuild does differently)

1. **Adopt, don't fork:** opencode as the engine (per-session models native, MIT, maintained) instead of core-modding a pinned framework. Hard rule going forward: no engine modifications, ever.
2. **Route the work, not just the judging:** subscription economics (mid-2026) put frontier *execution* on flat-rate paths; Smart mode sends the work itself to strong models. Nexus's cheap-executor/expensive-judge split was forced by early-2026 economics that no longer hold.
3. **Validate before breadth:** a pre-registered added-value gate before any feature outside the v0 core; Nexus's frozen bench-02 direct-arm results serve as the baseline arm.

## 5. Verdict and merge thesis

"Failed" is imprecise. **The platform layers succeeded** (resilience, auditability, gating, accounting — verified in production); **one configuration lost one benchmark** (cheap-executor + judge-only-frontier, full-auto, judgment-heavy task, n=1) before its own queued fixes ran. The decisive problems live in boundaries (engine choice, routing philosophy, decomposition), which is why the successor is a rebuild-with-transplants, not an in-place update: Nexus has proven organs without the right skeleton and economics; the v2.x briefing has the skeleton and economics without implementations. Every bench-02 failure was a **policy** failure executed flawlessly by **mechanisms** — confirming that policy + domain content is the layer where the new build is won.

## 6. New problems this post-mortem adds to the briefing

- **P45 — Spec lock-in / objective amplification:** verification machinery defends whatever objective it's given. Every verifier needs two axes: (a) spec compliance AND (b) outcome sanity ("is this good, independent of the spec?"). (bench-02 finding 1)
- **P46 — Guaranteed escalation routes:** every pipeline stage must have a tested route to raise a decision card for limitation-class findings; e2e tests must force one escalation to prove the wiring. (finding 2)
- **P47 — Fabrication on data-bearing tasks:** task classes that touch real-world data (products, prices, laws, APIs) get mandatory live research injected by the router as policy. (finding 3)

## 7. Disposition of Nexus

Frozen — no new features. It remains: (1) the daily workhorse during transition, (2) the organ donor (see harvest map), (3) the benchmark baseline arm for the added-value gate. Its knowledge base (playbooks, rubrics, STYLE-VOICE, eval cases, WINS/LESSONS) copies into the new platform on day one as domain content. Retirement is an outcome of the gate, not a prerequisite of the build.
