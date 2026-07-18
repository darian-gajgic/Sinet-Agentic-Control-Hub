# Sinet Agentic Control Hub — Core Architecture Spec v1

**Status:** DRAFT — under P2 per-section drafting; becomes binding at gate G4 (operator approval, which also ends the research phase).
**Sources of authority:** feature list §15.1 (the v0 target), D1–D10, gate records G1–G3, signed benchmark pre-registration (`5fb7082`), frontend component picks.
**Document mechanics:** until G4, the canonical text lives in `Spec/drafts/S00…S19`; `Spec/core-architecture-v1.md` is assembled by concatenating `Spec/drafts/S[0-9][0-9]-*.md` in order when all sections have passed coordinator review. Do not hand-edit the assembled file.

## S00.1 What this document is

The binding **how** for Sinet v0. The feature list (`Docs/agent-platform-feature-list-v1.md`) states *what* the platform does and is not restated here; this spec states the architecture that delivers it, with every load-bearing choice traceable to a ratified decision or a research finding.

Precedence for any reader (including P3 implementation sessions): **D1–D10 + feature list → gate records (`Research/decisions/GATE-*.md`) → this spec → research reports (evidence).** A contradiction discovered between layers is a defect to raise, never a choice to make locally.

Provenance tags used throughout: `[R## §x]` = research report `Research/##-*.md`, section x · `[G# D#.#]` / `[G# Def.#]` / `[G# rider #]` = gate decision, default, or operator rider · `[SPIKE P2-S#]` / `[SPIKE G1-S#]` = spike result · `[coordinator-draft]` = drafting-time sub-choice flagged for G4 attention.

## S00.2 v0 scope

**In scope (feature list §15.1 + §15.6):**
- Software development end-to-end: intake → specification → plan → execution → verification → review → accept → receipt.
- Consumption/metering layer (3.1–3.6) with the two-currency rule (D5) and honest receipts including the done-directly figure [G2 D2.8].
- Checkpoint-and-gate invariant (4.8/D7); durable state and recovery.
- Confinement classes **C0–C2** (S5); the credential broker; sandbox stack.
- Approval inbox (S3.2) with risk tiers; notifications.
- Local-models tier as the permanent free tier (Operating reality) — platform ceremony, watchdogs, and background intelligence run local.
- Operated **single-user (operator only)**; the data model is **multi-user from day one** (15.6) — every record carries its owner.
- The run scheduler (parks, resumes, priorities, budgets) — needed for limit events even single-user.

**Explicitly out (later phases; the architecture leaves their sockets):**
- v0.1: web research domain as second full pipeline; confinement class C3.
- 15.3: benchmark gate *execution* (the practice's machinery and its signed protocol exist at v0; the gate governs expansion).
- v1: household onboarding; degraded-mode domains (2.1/7.6); task templates; user-facing recurring schedules and event triggers with missed-slot policies (2.8, 15.4); memory auto-proposal pipeline [G2 D2.7].
- 15.5/12.x: multi-channel ingress, workforce-map editing, web-acting C4, voice/avatar satellites (parked behind the 15.3 gate; see `Spec/frontend-components-v1.md` §3).

## S00.3 Fixed inputs — D1–D10 (compressed; full text in the feature list)

- **D1 Central topology.** All execution on the operator's host; members connect over LAN/Tailscale; no runners on member devices.
- **D2 Per-person credentials on the host.** Per-person stores; every run authenticates as its owner; never pooled. The sandbox is the load-bearing isolation boundary; credentials never enter task sandboxes.
- **D3 Dual execution substrate behind one adapter contract.** Every provider behind an adapter: start, stream progress, checkpoint, pause, resume, cancel, report usage. First-party tool wrapped where terms require; direct API otherwise (including local).
- **D4 Consumption metering, reactive limits.** Measure exactly; treat provider limit events as normal recoverable scheduling events; never model provider quota windows.
- **D5 Two currencies.** Flat-rate routing uses consumption pressure, never dollars; dollars are the reporting currency and a routing input only for explicitly enabled metered use.
- **D6 Single-coordinator topology with a depth cap.** One coordinator per task; isolated helpers; sub-helpers only within the cap (default 2); no lateral messaging; every spawn logged with its reason.
- **D7 Checkpoint-and-gate.** Checkpoint after every paid model call; non-idempotent outward effects exist only as gated proposals until approved.
- **D8 Worker model.** Versioned template (personal by default; household-shared only via operator approval) → per-user overlay → per-run instance.
- **D9 Per-project git, GitHub as the off-host home.** Accepted work = attributed commits by the accepting user; private repos under each user's own account; platform snapshots to one operator-owned private repo.
- **D10 Approval principle.** Everyone approves their own objects; promotion to household-shared requires operator approval.

## S00.4 Decision-record index

Detail lives in the gate files; this index is navigational.

**G1 — Architecture direction (CLOSED 2026-07-17)**
| Id | Decision |
|---|---|
| P1–P12 | Pre-registered: un-pause → interactive-only demotion; Anthropic gray-zone as-is; Codex skipped; dual substrate; uniform class-2 posture; v0 lanes = Anthropic + Z.AI (xAI deferred); other providers parked, metered-exception list empty; clarity-driven intake + Clearance; utility models ≤8B at v0; AC dual-phrasing; zero-interaction all-four-conditions; watchlist owner → T11 |
| D1.1 | Architecture package ratified: dual substrate; run FSM + SQLite-WAL event log; fresh-context-per-stage on the Task Context Ledger; single-agent-first D6 orchestration; two-axis verification |
| D1.2 | Outcome-sanity axis on every deliverable in launch domains; stakes-gated elsewhere |
| D1.3 | Runaway containment = pause-and-flag; never auto-kill |
| D1.4 | Feature-list operating-reality amendment executed (Docs commit) |
| D1.5 | Z.AI tier moot — operator holds GLM Coding Max (~1,600 prompts/5 h shape) |
| D1.6 | Spikes: S1–S3 then; S4 removed (sole-controller posture); S5 at P3 |
| D1.7 | Zero-interaction cost threshold $0.50 API-equivalent (⚙ per-user) |
| Def.1–12 | Judge-lane pairing; entailment coverage; escalation SLAs (superseded by richer set: approval remind 4 h / push 24 h; safety immediate + hourly; canary daily; drill quarterly); hold-vs-park 10 min; freshness fingerprint set; systemd transient units per engine process; drain grace 15 min; orchestration numbers (4 helpers, ~20 turns/~80k tokens, spawn budget 8, reports ≤2k, depth-1 norm); helper screen conformance-only; cache-read weighting 0.1× "assumed"; stage-fit budgets ≤50%/70% split proposal; ledger v0 sections |
| Riders 1–3 | **Settings-not-constants** (every ⚙ number operator-editable, audit-trailed, auto-adjust only within operator ceilings, visible on receipts) · **Sole-controller posture** (all spawning control-plane-owned; engine-native subagents disabled on every substrate; R06-OQ2 revisited only at the adapter-spawning section with explicit operator reminder) · **Lane roadmap** (v0 = Anthropic + Z.AI; additions post-v0, config-only, via the report-02 §5 onboarding checklist) |

**G2 — Substrate + adoption (CLOSED 2026-07-17)**
| Id | Decision |
|---|---|
| D2.1 | Substrate package ratified: state (R08 §4), metering/scheduling (R09 §4), memory (R11 §4), observability (R12 §4), deliverables/review/git (R13 §4) |
| D2.2 | Adoption list ratified (R14 §6.1 = citable harvest reference); Crush patterns-only as policy |
| D2.3 | Sandbox stack ratified contingent on host probes — since **cleared** [SPIKE P2-S2] |
| D2.4 | Outward effects require idempotent-capable providers; plain SMTP only as explicit per-channel exception |
| D2.5 | Backup: daily snapshots, keep-30 ⚙, annual snapshot-repo rotation; Litestream deferred to implementation |
| D2.6 | Git identity: operator SSH-signs from day one; members opt-in at enrollment; no GitHub Pro at v0 |
| D2.7 | v0 memory surface = knowledge injection + L0 scratch + manual L2; auto-proposal pipeline at v1 |
| D2.8 | Done-directly figure: two-stage (heuristic → measured median at ≥10 pairs); benchmark pre-registration session committed (executed → `5fb7082`) |
| D2.9 | Consolidated spike battery at P2 entry (executed: P2-S1..S5) |
| D2.10 | Wave C launch order |
| Def.1–16 | synchronous=FULL; lease/liveness set (heartbeat 60 s, dead 5 min, wake grace 120 s, ≤3 recovery attempts, >24 h → finalize-with-card, ask-expiry 7 d); whole-project write claim when write-set unbounded; pressure admission stop 0.7 + background ≤50% "assumed"; budget denominator from plan marketing shape; GPU operator-wins (manual eager-unload + GameMode hook); memory lifecycle set (L1 TTL 90 d, re-verify 90 d/6 mo, ≤2 proposals/task, weekly digest, distillation ≥3/topic, per-scope injection budgets); vector post-gate (≥5% misses or >5,000 entries; rank-only); true deletion = purge + tombstone; watchdog/inbox numbers (loop 5×, ping-pong 6, error-loop 3×, spend >3× 14-day median, ≤2 flags/day, suppress-twice-proposes-retune); retention (6-month compaction; keep-forever summaries/verdicts/decisions/receipts/routing/drift/benchmark); watchlist executor (native feed poller; Miniflux fallback; changedetection.io + canary layer); PDF = text diff at v0; binaries = local object dir + hash refs; awesome-harness watch; genai-prices vendored `data.json`, refresh-as-proposal |

**G3 — Spec readiness (CLOSED 2026-07-18)**
| Id | Decision |
|---|---|
| D3.1 | P2 approved; entry sequence executed (spikes → benchmark pre-registration → frontend workshop → this drafting) |
| D3.2 | Backend language = **Go** |
| D3.3 | Frontend = **React 19 + Vite SPA** ratified with discipline riders; components per `Spec/frontend-components-v1.md` |
| D3.4 | Worker composition = compose-when-earned; supervised first-N = 3, count-based (⚙, reset on body/equipment version change) |
| D3.5 | Layer-2 open SQL at v0 on Arctic-Text2SQL-R1-7B, flagged lower-confidence, full guardrail stack |
| Def.1–8 | Shared-device PIN policy; `settings.changed` event type; helper-report screen conformance-only at v0; entailment thresholds set at spec workshop procedure + bring-up calibration; ts.net hostname pick (operator); week-one push drill (operator); privileged resume-remediation designed at spec time with T09 review; bring-up measurement set (workhorse bakeoff, VRAM ledger, CPU-tier throughput, contradiction-screen P/R) |

## S00.5 Sibling binding artifacts

- `Spec/benchmark-preregistration-v1.md` — the signed 11.2 protocol (commit `5fb7082`, verified via `Spec/allowed-signers`). Amendments only via its §17 procedure. This spec cites it and never restates its registered numbers.
- `Spec/frontend-components-v1.md` — four binding component picks + carried Nexus behavior specs + parked v1+ set. Consumed by S15/S16.

## S00.6 Conventions

As defined in the drafting contract and used document-wide: provenance tags (S00.1); `⚙ domain.name = default` for operator-editable settings (G1 rider 1 applies to every one); `TBD-BRINGUP(...)` for values pending the scripted bring-up measurements [G3 Def.8]; `TBD-P3(...)` for implementation-phase spikes; `TBD-OPERATOR(...)` for operator hands-on items; `[XREF:S##]` for cross-section pointers. Normative force: unqualified present-tense statements are binding; **MUST/NEVER** marks invariants whose violation is a platform defect; **SHOULD** marks defaults an implementation may vary only with a recorded reason.

## S00.7 Glossary (canonical vocabulary)

- **engine** — an adopted third-party execution binary/tool (`claude` CLI, `opencode`, `llama-server`). Adopted, never forked.
- **lane** — one (person, provider-substrate) execution path. v0 lanes: **Anthropic lane** (wrapped `claude` CLI), **Z.AI lane** (pinned `opencode serve`), **local lane** (llama-swap → llama-server).
- **adapter** — Sinet's wrapper implementing the D3 contract verbs over a lane's engine.
- **control plane / `sinet-control`** — the owned Go service: sole `platform.db` writer, API + the one SSE endpoint, scheduler, watchdogs.
- **`platform.db`** — the single SQLite-WAL database; control plane sole writer.
- **event log** — the append-only event table in `platform.db`; the observability substrate [R12].
- **run** — one FSM-governed execution of task work on a lane, checkpointed per paid call.
- **run unit** — the per-run transient systemd unit hosting one run's engine process inside its sandbox; streams to the control plane, never touches the DB [S01].
- **checkpoint** — the durable record written after every paid model call (D7); payload includes the Task Context Ledger state.
- **Task Context Ledger** — platform-owned per-task context artifact: pinned objective/AC/constraints; append-only decisions; verified-status state; restorable artifact refs [R07; G1 Def.12].
- **effect journal** — two-phase journal of outward effects; entries exist as proposals until approved; idempotency-registry backed [R08].
- **proposal** — any non-idempotent outward action awaiting approval (4.2).
- **park / parked** — the run state on limit events; resumes on provider signal or schedule (3.2).
- **consumption pressure** — measured consumption against (person, lane) budgets; the flat-rate routing currency (D5).
- **ceremony** — the platform's own thinking around a task (interviewing, restating, plan critique, verification review, lesson proposal), run on the requester's utility model (1.10), itemized on receipts.
- **utility model** — a person's designated local model (≤8B at v0) for light ceremony duties [G1 P9].
- **planning model** — a person's designated frontier-class model for the planning session and critique pass; heavyweight ceremony per the G3 cut line [S06].
- **harvest** — recovering the *result* of work that finished during an outage instead of re-running it [S02].
- **generation** — a per-run monotonic fencing counter bumped on every takeover/resume; stale-generation appends are rejected [S02].
- **duty alias** — a named local-tier capability slot (e.g. watchdog-disambiguator) mapping to a swappable model; swaps are invisible to workers [R16].
- **broker** — the credential broker process; holds provider/git secrets outside every sandbox (D2); performs signing and pushes [R10, R13].
- **seam** — one of five named replacement boundaries: storage, process, API, adapter, adoption [R17].
- **engine lowering** — compiling a run down to the exact per-lane invocation so Sinet-compiled config is the only config the engine sees [S03].
- **config channel** — one independent path (settings, MCP, skills, tools, cross-read files, cwd config, env) through which ambient environment could reach a worker; each has its own closing knob [S03].
- **confinement class** — a rung C0–C4 of the isolation ladder, declared per worker (S5); v0 ships C0–C2.
- **worker / template / overlay / instance** — the D8 ontology; workers = rows + git-versioned files in a Sinet-owned superset schema, compiled per engine per invocation [R15].
- **coordinator / helper** — the D6 topology roles; helpers are earned, isolated, brief-in/report-out.
- **brief / helper report / spawn record** — the D6 delegation artifacts: structured context down, size-capped report up, logged spawn row with trigger + reason [S04].
- **stage brief** — the assembled context package a stage's fresh engine session starts from (ledger projection + injected slices + stage instructions + compiled worker equipment) [S05].
- **trace manifest** — the per-assembly record of every injected item (source, hash, version, selector rule, precedence label) [S05].
- **composer** — the machinery that drafts new workers (7.1) through the 4-station validation battery [R15].
- **gap record** — the persistent record of a no-fit routing outcome; accumulation earns a composition proposal [S08].
- **composer playbook** — the versioned L2 house-knowledge object steering the composer [S08].
- **task-class ceiling table** — the per-(domain, task family) maximum-powers table enforced by the permission audit; 4.4's instrument [S08].
- **effort mode** — Eco / Balanced / Smart (3.5), implemented as disclosed depletion ladders [R09].
- **done-directly figure** — the receipt's honesty comparison: what this work would have cost run directly [G2 D2.8].
- **deliverable / revision** — the long-lived reviewable entity a task produces, and its immutable numbered snapshots 1..N [S13].
- **orphan / drain point** — the explicit anchor state of a comment that no longer attaches; and the single code path handing review feedback to a retry [S13].
- **snapshot ledger / escrow identity** — the SHA-256 integrity record of every 11.3 snapshot; the off-host recovery copy of the operator's age identity [S13].
- **watchlist** — the S2.8 external-change watch: canary suites, feed poller, changedetection.io [R12].
- **workspace** — a run's isolated working directory/clone (4.1, S1.6).
- **approval inbox** — the single queue of pending human decisions (S3.2), risk-tiered Low/Medium/High.
- **benchmark practice** — the 11.2 blind A/B protocol as registered in `Spec/benchmark-preregistration-v1.md`.
- **knowledge layers / scopes** — L0 task scratch, L1 descriptive observations (TTL), L2 human-gated knowledge × scopes user / worker-overlay / project / house [R11].

## S00.8 Document map

| § | Title | Primary inputs |
|---|---|---|
| S01 | Process architecture & platform shell | R17 |
| S02 | Durable state, checkpointing & recovery | R08 · P2-S2 · P2-S4 |
| S03 | Execution engines & adapters | R01 · R02 · R05 · G1-S1..S3 · P2-S1 · P2-S5 |
| S04 | Orchestration within D6 | R06 |
| S05 | Context engineering & the Task Context Ledger | R07 |
| S06 | Intake: interview → specification → plan | R03 |
| S07 | Verification & quality | R04 |
| S08 | Workers, composition, routing & automations | R15 |
| S09 | Memory & knowledge | R11 |
| S10 | Metering, budgets, scheduling & limit events | R09 · P2-S1 · P2-S3 · P2-S4 |
| S11 | Sandboxing & confinement | R10 · P2-S2 · P2-S3 |
| S12 | Local-models tier | R16 |
| S13 | Deliverables, review, git & backup | R13 |
| S14 | Observability, evals, benchmark & provider watch | R12 · benchmark pre-registration |
| S15 | Frontend & API surface | R17 · frontend-components-v1 |
| S16 | Adoption manifest & components.lock | R14 §6.1 · R17 §4.8 |
| S17 | Known-problems register | all sections + gate follow-up lists |
| S18 | Settings registry index | sweep of all ⚙ tables |
| S19 | v0 boundary, coverage & build-order notes | §15 · S01–S18 |

## S00.9 Amendment mechanics

Pre-G4: sections are amended by the campaign coordinator through workshops; every change is committed with its reason. At G4 the operator reviews the assembled document; approval freezes **v1** and ends the research phase (the operator's explicit act). Post-G4: changes require a dated changelog entry and operator approval; the benchmark pre-registration's numbers change only via its own §17. The `Docs/` feature list remains operator-gated and is never edited from this document.
