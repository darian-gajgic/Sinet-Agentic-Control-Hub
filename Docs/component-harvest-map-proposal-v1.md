# Component Harvest Map — v1 (2026-07-16)

**Purpose:** the definitive list of what to copy, port, or imitate from each source system — and, just as important, what *not* to. Companion to the build runbook (slice numbers refer to its Section 6.1) and briefing v2.2+ (problem IDs refer to its Section 6). Put this file in `/briefing/`; the v2.2 harvest table should reference it as the detailed version.

**Adoption modes (legend):**
- **ADOPT** — use as a running dependency, unmodified (Constitution rule 1 applies: config/API only).
- **PORT** — take the design and/or code and rewrite it into the new codebase as owned code. Only from Nexus (your code) — everything else needs license checks first.
- **PATTERN** — reimplement the *idea* fresh; don't copy code (license, stack, or quality reasons).
- **STUDY** — read before building our equivalent; extract invariants and edge cases, write nothing yet.

**Effort:** S = < half a day at your cadence · M = ~1 day · L = multi-day.

---

## 1. Nexus (your own system — no license constraints; the organ donor)

**Rule of thumb:** Nexus's *mechanisms* are the best-specified in this entire ecosystem; its *strategy* (engine fork, judge-only frontier, breadth-first) is what the new build inverts. Harvest mechanisms, not strategy.

### 1.1 Port in v0 (these are load-bearing slices)

| # | Item | What it is | Why it adds value | Target | Effort |
|---|---|---|---|---|---|
| N1 | **Orphan-harvest ladder + control/execution split** | Lane death ≠ run death: on resume, classify the orphan run as *active* (wait), *finished* (harvest result from session history), or *dead* (supersede); restarts never lose paid work | Production-verified through restarts, suspend/wake, overnight SSE stalls, quota windows — with zero token waste. Under subscription economics, wasted tokens are wasted *windows*; this is the single highest-value mechanism in the map. No public repo articulates it this completely | Slice 12; `classify_orphan()` in the AgentRuntime contract | M |
| N2 | **Atomic CAS task claiming + per-model slot gates** | Single SQLite compare-and-swap claim (double-claims impossible); `_slot_ok` concurrency caps per model; strict lane priority ladder (resume → queued → retry-blocked → claim-new) | Race-proof multi-lane scheduling with ~300 lines; it's the concurrency half of the quota scheduler, already debugged | Slice 05 | S |
| N3 | **Quota-storm handling** | Distinguish provider load-shedding from real quota; retry once on registry fallback model, then `blocked_quota` with exponential backoff; once-per-storm logging; interrupted judge runs never count as rounds | Encodes real provider behavior (Z.AI 429 vs error 1305) you already paid to learn | Slice 05 | S |
| N4 | **Dispatch state machine** | `none→queued→dispatching→streaming→finalizing→completed` + `failed / blocked_quota / blocked_budget / cancelled`, mirrored on a dispatches table, orthogonal to the user-facing kanban status | Clean separation of "what the operator sees" from "what the machinery does"; every resilience feature hangs off these states | Slice 02 schema + slice 08 | S |
| N5 | **Judge-loop semantics (v0 subset)** | SHIP / REVISE / REWRITE with **SHIP-with-notes** (only blockers block; polish is notes); numbered `[F1..Fn]` blocker-only fix lists; **stage contracts** (judge each stage against its own "Done when", with `N-A (owned by later stage)` and `UNVERIFIABLE-HERE` states); deterministic free pre-gate (stub/short/TBD/empty-diff) | Every rule was hard-won from a real pathological loop (the unwinnable REVISE spiral). Directly solves P36's rubber-stamping and runaway-revision risks. The most precisely specified verification contract available anywhere | Slice 09 (subset); `SPEC-JUDGE-LOOP.md` is the source of truth to port from | M |
| N6 | **Deep Plan engine** | Triage + family detection (software/analysis/content/research) → scaffolded per-family SPEC interview → task-DAG draft with acceptance-criteria coverage guarantee → premortem critique → bounded revise loop; **tools-off during all planning** | It's F9 (intake), working; it out-interviewed both direct-agent baselines in bench-02. Deterministic core, unit-testable | Slice 04 (`plan_engine.py` port) | M |
| N7 | **Decision-card pattern** | Every exhausted or ambiguous path files exactly ONE answerable card; approve = accept-with-notes and close family forever; reject re-arms as a new version family | The human-gate UX that makes approval sustainable; combined with P46 (guaranteed routes) it fixes bench-02 failure #2 | Slices 04/08/09; inbox UI slice 03 | S |
| N8 | **Frontier ledger / honest cost accounting** | Real dollars parsed from the `claude -p` JSON result envelope; blended-rate pricing for flat-plan usage; **API-equivalent USD** as an explicit comparison figure, never a bill | The AgentOps ledger, built; it's also what made bench-02's honest verdict possible. Feeds gate metrics and the fleet view | Slices 07 + 11 | S |
| N9 | **worktree.py** | Idempotent worktree + branch **per pipeline** (stages share a branch so implement→review→fix→verify build on each other; parallel pipelines isolated); `snapshot_commit` (never lose uncommitted agent work, junk-excluded); three-dot `capture_diff` vs baseline; operator checkout never touched | Better-reasoned than Archon's per-run isolation for pipeline work; already debugged (`__pycache__` pollution, pathspec quirks, the truthy-"0" deadlock) | Slice 10 | S |
| N10 | **verify.sh culture** | Self-counting static checks + runtime gates, pre-commit enforced; "a feature without a check does not exist" | The reason a 10-day monolith stayed operable at all; the new build starts with it instead of growing into it | Slices 01 + 13 | S |

### 1.2 Port in Phase 5 (post-gate)

| # | Item | What it is | Why it adds value | Target | Effort |
|---|---|---|---|---|---|
| N11 | **Full judge cascade** | Cheap→expensive tiers (free pre-gate → GLM screen → frontier judge), delta re-judge with **criteria frozen at round 1**, convergence via finding-key subsets, per-task $ caps, screen-never-blocks-on-its-own-failure, screen SHIPs never count as frontier SHIPs | The economics layer of verification; port after the A/B-style validation the gate provides | Phase 5 item 3 | M |
| N12 | **Quality Autopilot two-knob** | Involvement (full_auto/assisted/manual) × Spend (eco/optimal/smart) deriving every quality/cost knob; hard **risk floor** overrides profile for high-stakes | Better formulation than the briefing's F11 alone; structural answer to approval fatigue (P42) | Phase 5 (v0 ships fixed defaults; knobs come with the router) | S |
| N13 | **Gated lessons + knowledge-base distillation** | Deterministic evidence collection from operator edits/comments/accepted diffs → one judgment-tier proposal of durable playbook/rubric deltas → **admin approval → git-committed**; proposal autonomous, writing always gated; provenance + origin-key dedupe on cross-user adoption | The Tier-5 loop, implemented with exactly the right gate discipline; solves P41 by construction | Phase 5 item 2 | M |
| N14 | **WINS/LESSONS ledger + golden exemplars** | AI-drafted from real outcomes only (never invented numbers); LESSON requires a correction; domain-matched excerpts ride into dispatch framing; wins promotable to exemplars | Cheap, high-leverage context injection with human-controlled quality | Phase 5, with N13 | S |
| N15 | **Review v2** | PR-style review of *every* output type: per-file diffs w/ server-side highlighting, per-line comments anchored to old/new positions, PDFs diffed by extracted text, images side-by-side, round-over-round diffs, judge findings as anchored comments that **drain into the next retry** as `[F#]` feedback | The findings→comments→retry loop is genuinely novel; makes non-code deliverables reviewable like code (broad-spectrum requirement) | Phase 5 item 5 | L |
| N16 | **Model routing telemetry + collapse watch** | Deterministic keyword routing with per-model "Best for / Avoid for" upgrade/veto phrases, plain-language rationale stored per task, `routing_outcomes` telemetry, periodic no-LLM threshold-tweak proposals as admin cards | Prior art for the router rubric's learning loop (RQ7); the rationale-storage habit is worth keeping from day one | Phase 5 item 1 (router); rationale field already in slice 02 schema | S |
| N17 | **Agent-lane memory scopes** | stm scratchpad (48h TTL) / experience (90d, one line per dispatch) / lts (hourly rolling summary) / longterm (operator-taught rules, always injected) | Clean writer/consumer separation per scope; adopt the scope *taxonomy* even if storage changes | Phase 5 | M |
| N18 | **Repo onboarding template** | Read-only task generating a ruthless-concise AGENTS.md (<150 lines: purpose, architecture map, verified commands, conventions, danger zones) | Turns any client repo agent-ready in one gated task; feeds the framing builder | Phase 5 palette entry | S |
| N19 | **app_runner + time-travel preview** | Launch generated apps in disposable venvs (port remap, auto-stop); materialize whole-project previews before-vs-current from branch commits without touching the live worktree | "Test the result" UX; nice-to-have after Review v2 | Phase 5, low priority | M |

### 1.3 Adopt day one (not code — content)

| # | Item | Why | Target |
|---|---|---|---|
| N20 | **Knowledge base** (`~/knowledge`): BUSINESS-CONTEXT, STYLE-VOICE, 10 domain PLAYBOOK+RUBRIC pairs, eval cases, feedback ledgers | Pure domain content — the durable home from the four-homes analysis; engine-agnostic by construction; zero porting cost | Copy into the new platform's knowledge dir at Phase 3 start |
| N21 | **Benchmark methodology** | Frozen baselines, blind scoring, pre-registered thresholds, honest §13-style reporting | Phase 4 gate is built on it; bench-02's frozen direct-arm results are the gate's baseline arm |
| N22 | **Specialist prompt bodies** (the 18 + 28 retired) | Not as a standing roster (anti-harvest A2) — as raw material for the Phase 5 role *palette*, pruned and re-cut | Phase 5 item 1 |

---

## 2. opencode (MIT — the adopted engine)

| # | Item | Mode | Why | Target |
|---|---|---|---|---|
| O1 | **The engine itself**: `opencode serve` + OpenAPI + SDK + SSE, sessions, tools, permissions, sub-agents, compaction | **ADOPT** | Fills the own-engine slot; per-session model/provider selection **natively** — the feature Nexus needed guardian core-mod #6 for. This single adoption deletes the entire fork-maintenance burden | Slice 06 via the S1 spike |
| O2 | Provider layer (75+ providers; z.ai/Qwen/MiniMax keys; OpenAI/xAI/Copilot OAuth) | **ADOPT** | Every green billing path through one config surface; replaces LiteLLM and all bespoke provider code | Slice 06; credential isolation per RQ1 §6 |
| O3 | Agent/skill/permission **config formats** | **ADOPT** | Our generated definitions compile *to* these formats (Tier-A: configuration over engine) | Slice 08 framing builder; Phase 5 creation pipeline output |
| O4 | ACP support | **STUDY** | Candidate uniform adapter protocol later (P39); not v0 | Phase 5+ |

---

## 3. OpenClaw (study target #1 — architecture, not code)

| # | Item | Mode | Why | Target |
|---|---|---|---|---|
| C1 | **Runtime-policy pattern** — per-model choice of executor (own loop vs vendor CLI backend) | **PATTERN** | The proven shape of our hybrid: opencode for green paths, `claude -p` for amber, selected per (model, billing status) | Scheduler routing, slices 05–07 |
| C2 | **claude-cli backend** | **STUDY** | The public canary for stream-json drift (P31) — when their adapter breaks or changes, ours is next; subscribe to its changes | S2 spike + standing watch |
| C3 | **Per-agent auth-profile stores** | **PATTERN** | Reference design for per-user credential isolation (P27): scoped token stores, refresh handling, no cross-run leakage | Slice 02 schema; Phase 5 multi-user hardening |
| C4 | **Quota-window surfacing** | **PATTERN** | UX for showing live window state per account/model — feeds the fleet meters | Slice 11 |
| C5 | Token-sink / OAuth-refresh handling | **STUDY** | Edge cases of long-lived OAuth in daemons | Phase 5 |

---

## 4. Archon — current, post-pivot (MIT)

| # | Item | Mode | Why | Target |
|---|---|---|---|---|
| A1 | **Web dashboard components**: Mission Control (filterable run history), drag-and-drop **DAG builder with loop nodes**, step-by-step execution view, chat with tool-call visualization, unified cross-platform sidebar | **PORT/ADAPT** (per RQ17c audit) | The closest existing codebase to our frontend; the DAG builder is F5's *editable* graph — the feature we deferred as hard — already built, MIT | Phase 5 item 6; slice 03 may borrow list/kanban patterns |
| A2 | **Workflow YAML schema** (`prompt`/`bash` nodes, `depends_on`, `loop/until`, `interactive: true`, `fresh_context`) | **ADOPT as format** (RQ17d) | A far more tractable target for the meta-agent to *emit* than n8n JSON; versionable, committable, human-readable | Phase 5 item 1 output format; workflow lane per DECISIONS |
| A3 | `interactive: true` **pause/resume persistence** | **STUDY → PATTERN** | Working reference for parking a run mid-graph and resuming on human input — exactly the approval-inbox mechanic | Slice 08 |
| A4 | `fresh_context` per loop iteration | **PATTERN** | Elegant anti-context-rot strategy: fresh session per iteration, plan artifact as the handoff | Long-running workflow tasks; verification retries |
| A5 | **7-table schema** (codebases, conversations, sessions, workflow runs, isolation environments, messages, workflow events) | **STUDY** | Sanity-check reference for slice 02's schema — theirs is the minimal viable set for this problem shape | Slice 02 |
| A6 | Platform adapters (Telegram/Slack/Discord/GitHub webhooks) | **ADOPT-later** | 4a ingress nearly free — but only wrapped under the scheduler (P44) | Phase 5+ |
| A7 | Worktree manager | **STUDY** | Compare against N9; Nexus's branch-per-pipeline reasoning wins for pipelines — take Archon's parallel-run isolation details where they're cleaner | Slice 10 review input |

**Standing caveat (P44):** never expose Archon's own fire-and-forget triggers; every Archon run spawns *under our scheduler* with owner attribution and pre-spawn window checks. Telemetry: set `ARCHON_TELEMETRY_DISABLED=1`.

---

## 5. Crush (fair-source license — **patterns only, no code**, P33)

| # | Item | Mode | Why | Target |
|---|---|---|---|---|
| K1 | **Shared-workspace permission queue** — a blocked turn is visible and approvable from *any* attached client; IsBusy/AttachedClients signals | **PATTERN** | The approval-inbox blueprint: park in DB, render everywhere on the LAN, resume the same session hours later | Slice 08 + inbox UI |
| K2 | Default-deny permission service | **PATTERN** | The ToolPolicy posture: destructive actions deny-by-default, allowlists per role | Slice 08 ToolPolicy |

---

## 6. SAW (template repo — attribution required in NOTICE)

| # | Item | Mode | Why | Target |
|---|---|---|---|---|
| S1 | **Role palette** (the 11 SAFe-style roles, pruned hard) | **PATTERN** | Typed starting inventory for the creation pipeline's composer — roles as *options*, never a default formation (single-agent-first) | Phase 5 item 1, merged with N22 |
| S2 | Plan-strong / execute-cheap stage discipline | **PATTERN** | Already generalized into mode-based routing; keep the per-stage model-floor idea | Scheduler policy |
| S3 | Gate-discipline templates (definition-of-done per stage) | **PATTERN** | Feeds stage contracts (N5) and slice-spec acceptance sections | Slice 09; S3 spec authoring |

---

## 7. gh-aw (GitHub Agentic Workflows)

| # | Item | Mode | Why | Target |
|---|---|---|---|---|
| G1 | **Safe-outputs design**: read-only default, buffered writes, output filters/caps, a threat-detection pass before anything externalizes | **PATTERN** | The write-gating philosophy for every externalizing tool (email, push, publish): agents *propose* writes, a gate *performs* them | Slices 08/09 ToolPolicy + verification |

---

## 8. Reference-only sources (STUDY, build nothing from them)

| Source | Take |
|---|---|
| deepclaude / "Design Space" reverse-engineering of Claude Code | Frontier-harness internals (five-stage compaction, subagent permission contexts, hook pipeline) — defines what we deliberately do **not** rebuild because the adopted engines already do it |
| deepagents (LangChain) | The Deep-Agents reference shape (plan file + filesystem + sub-agents + detailed prompt) — vocabulary and sanity check, redundant as a dependency |
| OpenHands, Goose | Fallback engines if the S1 spike returns NO-GO on opencode |
| n8n self-hosted-ai-starter-kit | ADOPT if DECISIONS picks n8n for the non-dev workflow lane — compose infra as-is |
| awesome-harness-engineering | Standing index for Phase-5+ Tier-B (observability-driven harness evolution) work |

---

## 9. ANTI-HARVEST — explicitly do NOT copy

The failure modes live here; this list is as protective as everything above.

| From | Do not copy | Why |
|---|---|---|
| Nexus | **8 core-mods + guardian** | The fork-the-engine antipattern; opencode provides the keystone (per-session models) natively. Guardian is excellent engineering in service of a bad strategic position — Constitution rule 1 replaces it |
| Nexus | **18-specialist standing roster** | Single-agent-first (P7); specialists become palette *entries* the composer may pick, never a default formation |
| Nexus | **server.py / app.js monolith topology** | Bus factor 1, no seams; the new build's process-per-layer decomposition (slice 01) exists to make this impossible |
| Nexus | **Judge-only frontier economics** | The executor ceiling: gates bound damage, they don't create quality. The new build routes Smart-mode *work* to frontier flat-rate paths |
| Nexus | **JARVIS voice/vision/avatar, 3D memory galaxy, dictation, MeetingMode** | Breadth before the value bet — Constitution rule 5 freezes all of it until v1 survives a month of dogfooding. (Dictation/meetings are genuinely useful to you personally — but they're *separate tools*, not platform features) |
| Nexus | **mem0/qdrant-first memory** | Two retrieval philosophies coexisted; the new build is deterministic-retrieval-first (filesystem, LSP, structured stores); vector memory is a post-gate *evaluated* addition, not a foundation |
| Nexus | Live-tree symlink deployment, machine-coupled paths, pinned-base patches | Reproducibility debt; the new build deploys from CI-verified packages |
| Archon | Its **own triggers / direct fire-and-forget path** | P44: no quota layer; everything spawns under our scheduler or not at all |
| Crush | **Any code** | License (P33) — patterns only until verified |
| All frameworks | Peer-mesh / group-chat multi-agent topologies | Evidence-excluded (cascade falsehoods, cost multiplication); hub-and-spoke only |
| All vendors | Any load-bearing metered-API path | Constitution rule 9; overflow only, explicitly flagged |

---

## 10. Coverage check — every v0 slice has its donor

| Slice | Primary donors |
|---|---|
| 01 scaffold | N10 |
| 02 schema | N4, N8, A5, C3 |
| 03 kanban | N7 (cards), A1 (patterns) |
| 04 intake | N6, N7 |
| 05 scheduler | N2, N3, C1, RQ5 design |
| 06 opencode adapter | O1–O3, S1 spike |
| 07 claude-cli adapter | N8, C2, S2 spike |
| 08 lifecycle + inbox | N4, N7, K1, K2, A3, G1, N13-pattern (framing injection) |
| 09 verification v1 | N5, S3(SAW), G1, +P45 outcome axis (new) |
| 10 deliverables/diffs | N9, A7 |
| 11 ledgers/fleet | N8, C4 |
| 12 harvest ladder | N1 |
| 13 e2e gate | N10, N21 |

Nothing in v0 is invented from zero except the two things that *should* be new: the window-aware half of the scheduler (RQ5) and the outcome-sanity verification axis (P45) — the two lessons Nexus paid for.
