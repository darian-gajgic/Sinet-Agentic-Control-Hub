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
- **per-run sandbox** — the composed kernel-primitive jail (systemd→bwrap→seccomp→Landlock) wrapping one run's engine process [S11].
- **credential-injection proxy** — the host-side TLS-terminating proxy that substitutes the real subscription token only on the pinned model-egress request, keeping engine credentials outside the sandbox [S11].
- **auth-profile** — a named, broker-resolved credential reference in a worker's control-plane record; never a secret [S11].
- **GPU broker** — the control-plane-mediated local-inference data plane (distinct from the credential broker) [S12].
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

### Post-G4 changelog

| # | Date | Change | Approval |
|---|---|---|---|
| A1 | 2026-07-19 | S01.8: `TBD-OPERATOR(ts.net hostname pick)` closed — machine hostname is **`sinet`** (bland + permanent; to be set as the Tailscale machine name before the first cert, per the S01.8 rationale). Closes G3 Def.5. No ⚙ setting touched → no S18 re-sweep. | operator, 2026-07-19 session |
| A2 | 2026-07-22 | TBD-P3(Claude-lane auto-memory containment, R11-OQ6) closed: S09.9 containment implemented and conformance-tested at P3-B3-1 (platform-supplied config-root `memory/` wiped at session start, resume exempt); engine-native memory default-off posture confirmed by the B1-4 spike (result file `P3/measurements/2026-07-20-auto-memory-containment.md`). Marker sites annotated closed: S03 TBD list, S09.9, S09 P-T10-1 + deferred rows, S17 register row. No ⚙ touched → no S18 re-sweep. | operator, 2026-07-22 B3 gate (D2) |
| A3 | 2026-07-22 | TBD-BRINGUP(parallel-gate fallback rate on the default worker model) closed: serialize-by-deny reconfirmed PASS on the S08-designated default worker at engine 2.1.216 (P3-B3-3, result file `P3/measurements/2026-07-21-serialize-by-deny-reconfirm-s08.md`; B1-4 spike-1 PASS-PROVISIONAL made final). S03.4 fallback stands ratified; GateFallback detector retained as a dormant per-pin canary. Marker sites annotated closed: S03 TBD list + §S03.4 trap paragraph. No ⚙ touched → no S18 re-sweep. | operator, 2026-07-22 B3 gate (D2) |
| A4 | 2026-07-23 | TBD-P3(PreCompact-blocking + Claude-lane injection-mechanics spike) closed: spike PASS at P3-B1-4 (PreCompact can veto compaction; SessionStart source:'compact' re-injection confirmed; containment stays primary per S05) — result file `P3/measurements/2026-07-20-precompact-injection-mechanics.md`. Marker sites annotated: S03 TBD list, S05 deferred row, S19 spike list. No ⚙ touched → no S18 re-sweep. | operator, 2026-07-23 B4 gate (D5) |
| A5 | 2026-07-23 | TBD-P3(GameMode user-context probe) closed: ran live at gamemoded v1.8.1 (P3-B4-5) — GameRegistered/GameUnregistered fire on the per-user SESSION bus and are NOT reachable from the `sinet` system subscriber without an operator-supplied `DBUS_SESSION_BUS_ADDRESS`; the `gamemode.ini [custom]` scripts leg is the always-works pause/resume layer, the busctl subscription is deferred-with-finding. Result file `P3/measurements/2026-07-22-gamemode-user-context-probe.md`. Marker sites annotated: S12.2, S19 spike list. No ⚙ touched → no S18 re-sweep. | operator, 2026-07-23 B4 gate (D5) |
| A6 | 2026-07-23 | TBD-BRINGUP(contradiction-screen P/R) closed for the SHIPPED one-stage shape: P=R=1.0 on the 12-pair synthetic Sinet seed at the one-stage workhorse confirm (P3-B4-7). The spec's two-stage DeBERTa→workhorse pre-screen is not the shipped shape (DeBERTa `Servable:false` at the pin — no GGUF serving path); the pre-screen re-enters when a serving path exists, and recall sharpens under golden-set governance. Result file `P3/measurements/2026-07-22-contradiction-screen-pr.md`. Marker narrowed at the S12 bring-up list (#4). No ⚙ touched → no S18 re-sweep. | operator, 2026-07-23 B4 gate (D5) |
| A7 | 2026-07-23 | TBD-BRINGUP(per-duty confidence calibration) closed for the COVERED duties: the S12.5 margin→isotonic→threshold fit ran at the DEPLOYED seats (P3-B4-7) — calibrated (meets_bar): intent-filling@4B + contradiction-screen@9B (+ entailment@Guardian, A8). watchdog/watchlist do not meet the bar at the deployed 4B seat and intake-triage reports a can't-separate result (subjective `size`) → they carry `calibrated=false` honestly (ungated v0 behavior continues). Durable `calibration_records` writes (migration 0010) at the deployed-seat keys are a bring-up act. Result file `P3/measurements/2026-07-22-per-duty-confidence-calibration.md`. Marker narrowed at the S12 bring-up list (#6). No ⚙ touched → no S18 re-sweep. | operator, 2026-07-23 B4 gate (D5) |
| A8 | 2026-07-23 | TBD-BRINGUP(entailment thresholds + mandatory-coverage bar) — the carried B2 TPR/TNR deferral — closed at the MAIN bar, conservative on the load-bearing bar: Granite Guardian 4.1 measured 152/156 (97.4%) on the 156-pair Sinet-domain set (P3-B4-7), clearing the pre-registered ≥0.90 MAIN bar (per-side TPR 0.949 / TNR 1.000); the load-bearing 0.95 sub-bar is NOT met on the entailed side (TPR 0.949 < 0.95) → mandatory-coverage stays CONSERVATIVE and the `EntailmentGate` stays idle (it also requires the web-research domain + `Calibrated`). ⚙ `verification.entailment_sample_rate` DERIVED = 0.20; its LIVE write is a bring-up operator `Registry.Set` (audited; never a code-default edit), so the S07/S18 ⚙ rows are annotated value-derived-write-pending, NOT fully closed; the setting's declared default/clamp are UNCHANGED here → no S18 re-sweep now (any sweep rides the eventual live write). Guardian-3.3 leg + the 3.3-vs-4.1 head-to-head + the entailed-side floor re-measure = bring-up. Result file `P3/measurements/2026-07-22-entailment-thresholds.md`. Markers narrowed at the S12 bring-up list (#5), S07.4, and the S07/S18 ⚙ rows. | operator, 2026-07-23 B4 gate (D5) |
| A9 | 2026-08-05 | S15.2 REST family table: the two landed route families it never listed are added — **`chat`** (`/api/chat`: per-user assistant sessions, turns and exchange files; verbs = session create/rename/delete, turn submit/stop, file upload/delete) and **`push`** (`/api/push`: Declarative Web Push subscriptions per identity+device; verbs = subscribe, remove). Cosmetic: the spec catches up to shipped, ratified reality rather than changing it. Ground = the landed registrations in `internal/api/api.go` (chat at :600–609, push at :632–634), built at P3-B6-7 and P3-B6-9 and recorded in CONVENTIONS §44/§46. No new route, no contract change. No ⚙ default or clamp touched → no S18 re-sweep. | operator, 2026-08-04 B6 gate (D7 en bloc rider) |
| A10 | 2026-08-05 | S15.5 task-detail paragraph: the aggregate done-directly label is aligned to the registration it paraphrases — `"measured, n=…"` → **`"measured (benchmark n=…)"`**, byte-matching BENCH-REG §13.2. The registration is the authority and §13.3 forbids its label semantics floating, so the SPEC’s paraphrase moves to match it and never the reverse; BENCH-REG is untouched. The per-run label `"direct-use estimate (heuristic)"` already matched §13.1 and is unchanged. One string; the sentence is otherwise as frozen. No ⚙ default or clamp touched → no S18 re-sweep. | operator, 2026-08-04 B6 gate (D7 en bloc rider) |
| A11 | 2026-08-24 | **The Kimi (Moonshot) lane is added to the lane set ahead of its post-v0 slot, on explicit operator order (2026-08-23).** S03.6 registers Kimi among the parked post-v0 providers that "join post-v0 config-only via the report-02 §5 onboarding checklist" [G1 rider 3]; this entry records that the operator, holding a Kimi membership that serves K3, ordered the addition early because the Anthropic lane alone is insufficient for reliable testing (Sinet's task execution shares the operator's Anthropic subscription with the build pipeline itself). **The rider-3 mechanism is unchanged and was followed, not bypassed:** the R02 §5 Gate A–C audit ran live on 2026-08-24 against primary sources and is committed at `P3/measurements/2026-08-24-kimi-lane-gate-audit.md` — Gate A **PASS** (class 2, tool-scoped with no unattended-use ban; OpenCode is a named sanctioned tool, so no new engine is forced), Gate B **PASS** (`https://api.kimi.com/coding/v1`, OpenAI- and Anthropic-compatible; opencode ships a first-party `kimi-for-coding` provider), Gate C **PASS** with `overflow_mode: opt-in-credits` on a **proven disable** (satisfying 3.10 / P-T17-2) and a **mandatory no-household-personal-data routing rider** (C5: Beijing operating entity, mainland-China data residency, training-on-inputs by default, no GDPR/EEA language). The audit's A1 is **partial** — the international USD plan-price pages are JavaScript shells — so the per-tier prices and allowance multipliers are carried as UNVERIFIED and close from the operator's own account console at bring-up (P-T17-3), not from documentation. The metered-exception list stays **EMPTY** [G1 P7]: this is a flat subscription lane, and DeepSeek remains the sole pre-registered designated exception. No ⚙ setting's default or clamp is touched — the lane's numbers live on the S18.3 data surfaces (the lane and plan documents) with no dotted key → **no S18 re-sweep**. The S18 tally stays **118 keys / 33 domains**. Marker sites annotated: S03.6 lane roadmap, S03.6 deferred "Post-v0 lanes" row, S16 onboarding manifest. | operator, 2026-08-23 order; presented for veto at the LN gate batch |
| A12 | 2026-08-26 | **S03.2's "v0 ships exactly two substrates carrying three lanes" grows a THIRD substrate — `kimi-cli`, Moonshot's first-party Kimi Code CLI (`@moonshot-ai/kimi-code`, npm, MIT, pin 0.38.0 EXACT) — on explicit operator order (2026-08-26).** The operator holds a Kimi Code membership that serves BOTH the API endpoint `https://api.kimi.com/coding/v1` (lane `kimi`, opencode substrate, added at A11) AND the first-party CLI, **from one shared quota pool** — vendor verbatim, captured 2026-08-26: *"Kimi Code is the developer-focused AI coding service within Kimi membership benefits, provided together with a Kimi membership subscription and sharing the same quota. Requests from the CLI, VS Code, and third-party tools all count toward that quota."* The order is to run K3 by both paths so the operator can measure which performs better, which is a comparison the platform can only make honestly if the second path is a real substrate rather than a relabelled lane. **The rider-3 mechanism is NOT what governs here and the difference is stated rather than blurred:** S03.6's checklist adds LANES config-only and explicitly "never a new substrate", so a third substrate is a genuine amendment to S03.2's pin, not an onboarding. What rides the checklist is the lane `kimi-cli` (a lane DOCUMENT: substrate `kimi-cli`, the same `kimi-code` broker profile, credential variable `KIMI_MODEL_API_KEY` — the CLI does not read `KIMI_API_KEY` from the shell — and the same membership pool, declared ONCE and never as two). **Gate A was RE-AUDITED at this amendment and the re-audit is the reason this entry exists in the form it does:** the 2026-08-24 audit passed Gate A as class 2 without reading `https://www.kimi.com/code/docs/en/kimi-code/community-guidelines.html`, which states verbatim *"Kimi Code subscriptions are for interactive use only"* and *"Don't use Kimi Code for non-interactive automation"* — S03.6's class-3 language, which its own pre-registered would-change trigger names and answers with "immediate re-audit and lane freeze". That text binds the EXISTING `kimi` lane identically. Operator ruling, 2026-08-26: **PROCEED, acceptance recorded** — coordinator-drafted option text, operator-selected, in-session. The recorded reasoning: personal interactive use through agent frameworks the guidelines page itself sanctions (Kimi CLI, VS Code, Claude Code, OpenCode); the banned examples (scripted batch execution, data-annotation pipelines) do not describe this use; stated enforcement is graduated (concurrency limiting); accepted as a recorded gray zone, the same posture as the Anthropic lane's G1 P2 note. Evidence: `P3/measurements/2026-08-24-kimi-lane-gate-audit.md` (with its 2026-08-26 Gate-A addendum) and `P3/measurements/2026-08-26-kimi-cli-print-mode-spike.md` (the $0 loopback capture that establishes the engine's real flag surface, stream envelope, boundedness and native-spawn disable — the 2026-08-26 coordinator sweep's `--print`/`--afk`/`--input-format`/`--final-message-only` list does not exist at this pin and is superseded). Sole-controller posture UNCHANGED and structural, verified by execution: `[tools] disabled = ["Agent","AgentSwarm"]` strips both PRE-inference (26 tools offered → 24). S03.4 parking is **structurally unavailable** on this substrate — print mode requests no approval, hooks are fail-open, and static `[permission] deny` rules were MEASURED inert in print mode — so a gated-tool worker is REFUSED by name rather than auto-approved, and that limitation is written on the lane document and the conformance row. The metered-exception list stays **EMPTY** [G1 P7]. No ⚙ setting's default or clamp is touched — the lane's numbers live on the S18.3 data surfaces with no dotted key → **no S18 re-sweep**; the S18 tally stays **118 keys / 33 domains** and the data-surface count stays **3**. Marker sites annotated: S03.2 substrate count, S03.6 lane roadmap, the `adapters.go` substrate-const comment, and the S16 lane-onboarding manifest (second row). | operator, 2026-08-26 order; Gate-A ruling same date; presented for veto at the LN gate batch |
| A13 | 2026-08-26 | **S08.8's "Visible and overridable" gains a second expression point and names its axis: a task may carry an operator-declared PINNED LANE, declared at task CREATION ahead of the first selection rather than only as a re-route on the plan/approval card, which selection honors in place of the step-3 consumption-pressure comparison among covered flat-rate lanes — on explicit operator order (2026-08-26).** S08.8 already ratifies the override itself ("the requester can re-route or pin before execution; overrides are recorded with their actor"), and the shipped machinery already implements that sentence for the WORKER axis alone: `intake.RouteOverride{Target, Pin}` selects a template id or the generalist, and the lane is re-derived from the chosen worker's seat on every recompute. What this entry amends is therefore narrow and is stated rather than blurred: **(a)** the pinnable AXIS is named as the LANE, and **(b)** the pin may be declared at task creation — the S15.2 `tasks` family's create verb, which at v0 is `POST /api/intake/requests` — because a pin expressible only on the card cannot precede the first selection, and per-task-at-creation is the ordered use. **Why now, and it is a capability the platform provably lacked:** P3-LN-8 established that the operator's ordered comparison of the two Kimi paths (lane `kimi` on the opencode substrate [A11]; lane `kimi-cli` on Moonshot's own CLI [A12]) is NOT RUNNABLE by any existing mechanism — the two lanes draw ONE membership quota pool (`kimi-code-membership`, declared once, never as two allowances), so their consumption-pressure ratios are identical BY CONSTRUCTION; `chooseFlatLane` takes the strictly less-consumed lane and keeps the earlier candidate on a tie, and the candidate order puts `kimi` first; one placed credential commissions both, so neither can be held alone; and no surface anywhere takes a lane from a person. **Operator ruling, 2026-08-26: BUILD THE LANE PIN NOW** (option A, recommended-and-taken) — coordinator-drafted option text, operator-selected, in-session. **What does NOT change, each load-bearing:** S08.8 step 3's "subscription coverage binds every choice" is UNCHANGED and OUTRANKS the pin — a pin can never select a lane the owner does not hold flat-rate; it is refused at the system boundary before a task is born, the refusal verdict is computed once and carried rather than re-derived, and selection re-checks it so a pin arriving by any other route cannot steer dispatch. **D5 is untouched**: a pin is a person's named choice and carries no money — it REPLACES the pressure comparison rather than adding a currency to it, and no price, cost or dollar figure enters selection's inputs, which remains a structural property rather than a discipline. The local tier is NOT pinnable at v0 — its engine lane has no commissioned provider entry (S12.1 class (a)) — and refuses in its own words rather than borrowing the 2.7 subscription-gap wording. The benchmark direct arm is deliberately exempt: its lane is a structural constant naming the requester's frontier surface and the arm's identity IS its lane. The metered-exception list stays **EMPTY** [G1 P7]. **No ⚙ setting's default or clamp is touched** — the pinnable set is derived from what an operator has actually placed and has no dotted key → **no S18 re-sweep**; the S18 tally stays **118 keys / 33 domains** and the data-surface count stays **3**. Marker sites annotated: S08.8's "Visible and overridable" paragraph, the S15.2 `tasks` family row, and the `internal/worker/routing.go` head comment where the selection pipeline's inputs are enumerated. | operator, 2026-08-26 ruling; presented for veto at the LN gate batch (item 15) |
| A14 | 2026-08-27 | **S07.8 gains the BOOTSTRAP VERIFICATION POSTURE for command-less launch-domain tasks — the defined landing S07.8 lacked for a launch-domain deliverable whose project has no captured check pack — on explicit operator order.** Before this entry, S07.8's degraded-mode rule covered only NON-launch domains; a launch-domain deliverable with no captured build/test/lint commands (every fresh scaffold's first task) had no defined posture, so the implementation's only honest landing was a verification refusal (`internal/verify` `ErrNoCheckPack`), which parks the run behind a retry that can never succeed. The operator hit this wall twice on real projects (round-3 F8, the car-parts webshop; round-4 F1, the GPU-hardware shop) and ordered it fixed "for good" — the operator record is `P3/design/b6-gate-operator-findings-r4-2026-08-23.md` §F1a, which names this amendment (drafted at P3-RW-14, brief OQ3) and states that the order to fix IS the order to execute it. **What changes:** S07.8 adds the bootstrap-posture bullet — for a launch-domain deliverable with no captured check pack, V0 runs unchanged; V1 runs the S07.3 stage-contract checks with every executable-ladder rung that would need the missing commands recording `UNVERIFIABLE-HERE` (ratified S07.3 vocabulary), never silent-skip, never PASS; V2 runs advisory-marked non-authoritative (the existing degraded-mode treatment); requester review becomes mandatory (V3 blocks at every stakes tier, including trivial-band); the verdict card and receipt name the posture in plain words including what capturing commands restores; the posture is computed per revision from the registry's current capture, so captured commands resume the full ladder with no residue; refusal terminals remain only for S07.7 integrity cases, never pack absence. **What does NOT change:** the S07.3 ladder, its MUST rules, and the graduation rule are untouched — bootstrap is a posture within S07.8's scope rules, not a maturity level; the S07.8 degraded-domain bullet still governs non-launch domains at v1; V3's blocking-ness elsewhere still follows the band rule. No ⚙ setting's default or clamp is touched → **no S18 re-sweep**; the S18 tally stays **118 keys / 33 domains**. Marker sites annotated: the S07.8 bullet itself (carries the A14 tag); the implementing code annotates `internal/verify` at its packet. | operator, 2026-08-23 order ("Fix it for good now", r4 record §F1a); executed 2026-08-27 |

## S01 — Process architecture & platform shell

**Scope:** The process/unit topology of the owned core and its adopted organs, the five replacement seams, backend language and release artifact, platform lifecycle (startup, shutdown, maintenance/drain, sleep/wake), deploy/CI/logs, the authentication stack, and the settings-registry architecture.
**Binding inputs:** R17 (primary) · G1 Def.6, Def.7, riders 1–2 · G2 D2.1, D2.2, D2.5, Def.12 · G3 D3.2, D3.3, Def.1, Def.2, Def.5, Def.7 · [SPIKE P2-S3] · feature list D1, D2, 4.5, 13.4, 13.5, 15.6, Operating reality · `Spec/frontend-components-v1.md` (picks referenced, never restated).

### S01.1 Shape: small owned core + adopted organs

Sinet is one small owned daemon (`sinet-control`) surrounded by single-purpose organs, every one a systemd **system** unit with its own journal identity [R17 §4.1; G2 D2.1]. Internally `sinet-control` is a modular monolith with enforced module seams (storage / scheduler / gates / ledger / event-log / adapters); decomposition means *seams*, not network services [R17 §4.1]. The web surface is not a process: the SPA, the chat surface, and any future CLI are all clients of the same HTTP API [R17 §4.1; XREF:S15]. This shape is the direct encoding of the Nexus anti-monolith lesson (`Docs/nexus-post-mortem.md`) and the survivor-cohort pattern [R17 §2.1, §6].

Shell-level invariants (violations are platform defects):

- `sinet-control` is the **sole writer** of `platform.db`; run units NEVER touch the DB [G2 D2.1; R17 §4.1].
- Every sinet unit binds **127.0.0.1 or a unix socket only**; the only sanctioned non-loopback listener on the host is `tailscaled` (front door; see S01.8, P-T13-2) [R17 §4.1].
- Credentials never enter run units (D2); only the broker holds decrypted secrets [R17 §4.1; SPIKE P2-S3].
- All platform units are system units running as one **static `sinet` user**; `DynamicUser=` is NEVER used for state-owning units (UID recycling vs decade-lived DB files) [R17 §4.1, §2.2].
- journald is the ops log only; the `platform.db` event log is the only audit truth [R17 §4.8].

### S01.2 Unit map

| Unit | Kind | Role | Key directives |
|---|---|---|---|
| `sinet-control.service` | owned core | Sole `platform.db` writer; scheduler + queue claiming [XREF:S10]; D7 checkpoint/gate machinery + effect journal [XREF:S02]; event log; HTTP API + the one SSE endpoint [XREF:S15]; adapter supervision [XREF:S03] | `Type=notify`, `WatchdogSec=` ⚙, `sd_notify` heartbeat, `Restart=on-failure`, `StateDirectory=sinet`, `ConfigurationDirectory=sinet`, binds 127.0.0.1 only [R17 §4.1] |
| `sinet-broker.service` | owned | Credential broker: ssh-agent-shaped, operation-in/result-out; secrets at rest via systemd-creds + sops/age [G2 D2.2]; performs git signing/pushes [XREF:S13]. Separate process so the large-attack-surface control plane never holds decrypted member credentials [R17 §4.1]. Internals: [XREF:S11] | UDS with peer-cred auth; no TCP listener |
| `sinet-engine@<user>.service` | owned template | Per-user pinned `opencode serve` instance (Z.AI lane); `claude` CLI runs are per-run subprocesses inside run units, never standing services [R17 §4.1; XREF:S03] | Per-user localhost port; `After=sinet-broker.service` (credentials injected at start, outside any sandbox [SPIKE P2-S3]); `Restart=on-failure` |
| **run units** (template instances) | owned, per run | See below | `sinet-run@<run_id>.service` instances of a root-installed fixed-`ExecStart` template [XREF:S11]; `Restart=no` |
| `sinet-portpool.service` | owned | Port-pool daemon for preview allocation [G2 D2.1; XREF:S13] | Loopback only |
| `caddy.service` | adopted organ | Front router: `/api` + `/events` proxying (`/events` unbuffered), preview subdomain routes via admin API [R17 §4.1, §4.6; XREF:S13] | Localhost bind behind `tailscale serve` |
| `changedetection.io` unit | adopted organ | Watchlist executor [G2 Def.12; XREF:S14] | Own unit, own journal identity |
| local-model unit(s) | adopted organ | llama-swap front + llama-server backends [XREF:S12] | Own unit(s) [XREF:S12] |
| `tailscaled.service` | host-managed | Tailnet + TLS front door; not sinet-managed, but sinet health-checks it (P-T13-1, S01.7) | — |

**Run unit** (term coined here): the per-run, ephemeral systemd unit that hosts one run's engine process inside its sandbox — the process form of a run executing on a lane — realized as an instance `sinet-run@<run_id>.service` of a root-installed template [G1 Def.6; R17 §4.1; XREF:S11 — S11.8 Shape B]. Run units compose the sandbox stack [XREF:S11] around the run's workspace [XREF:S02], carry PID-1-enforced time ceilings and cgroup accounting (the cost-ceiling backstop [G1 Def.6; XREF:S10]), and stream events and checkpoints to `sinet-control` over the local API — the control plane persists, so sole-writer holds, each run is crash-isolated, and every run is visible in `systemctl`/journald under its own identity [R17 §4.1]. Run units are never auto-restarted by PID 1: a dead run is recovered by the S02 recovery ladder (fork-from-checkpoint), a decision the control plane owns [XREF:S02]. Unit names are the template-instance names `sinet-run@<run_id>.service`, so journal attribution is mechanical (`%i` identity). The mechanism by which the unprivileged `sinet` user starts these system-scope instances — a fixed-`ExecStart` template plus a single name-scoped polkit grant (Shape B) — is S11's privileged-surface design, alongside the resume-remediation path [G3 Def.7; XREF:S11]. *(Wording aligned to S11.8 at assembly, 2026-07-19 [coordinator-draft].)*

The standard hardening set (`ProtectSystem=strict` + `ReadWritePaths`/`StateDirectory`, `NoNewPrivileges=`, `PrivateTmp=`, syscall filter) applies to every owned unit as far as compatible with its duty; per-unit exceptions are recorded in the unit file with a reason [R17 §4.1]. System units (not user units + linger) are load-bearing: user units lack parts of the namespacing hardening and `WakeSystem=` timers, and boot-start needs no login plumbing [R17 §2.2, §5].

### S01.3 The five seams

The five named replacement boundaries [R17 §4.1; G3 digest]. Every future "replace X" conversation happens *at a seam*; anything that cannot be swapped at a seam is a design defect to raise.

| Seam | What it isolates | What swapping at it costs |
|---|---|---|
| **Storage** | The persistence engine behind the control plane's storage module: one writer, read-only opens for everything else | SQLite → client/server DB = rewrite one module + the backup lane; schema and API untouched. Reopened only by named triggers: second host, sustained >100 writes/s, multi-process writers [R17 §4.10] |
| **Process** | Each organ's lifecycle, failure, and logs (own unit, own unforgeable `_SYSTEMD_UNIT` journal identity, own restart policy) | Replace the unit + its `components.lock` entry; core untouched (e.g. watchlist executor → Miniflux fallback [G2 Def.12]) |
| **API** | Every surface from the core: SPA, chat, future CLI are equal clients of the HTTP API + SSE endpoint | Rebuild a surface (e.g. the HTMX re-entry conditions [G3 D3.3]) with zero core change; the cost is the surface itself [XREF:S15] |
| **Adapter** | Engine/provider specifics behind the D3 contract verbs | New engine or provider = one new adapter; orchestration, metering, state untouched [XREF:S03] |
| **Adoption** | Every third-party organ behind a pin + replacement path + abandonment criteria in `components.lock` | Pre-planned per entry; exit plans exist before they are needed [G2 D2.2; XREF:S16] |

### S01.4 Front chain & IPC

**Front chain (the trust chain):** `tailscale serve` (tailnet-only TLS on the ts.net name, injects `Tailscale-User-*` identity headers, strips them from inbound requests) → Caddy (routing; `/events` unbuffered) → `sinet-control` on 127.0.0.1 [R17 §4.1, §2.8]. **HTTP/2 terminates at `tailscale serve`** so the browser leg multiplexes and multi-tab SSE never hits the 6-connection limit [R17 §4.6]. The SPA's static assets are embedded in the control-plane binary and served through this same chain (S01.5); Caddy's role is routing, not asset hosting. Exposure beyond the tailnet does not exist (D1); LAN access rides the tailnet's direct LAN paths.

**IPC map** [R17 §4.1]: browser → serve → Caddy → control (HTTPS + SSE); run units → control (local API: UDS or localhost HTTP — the invariant is *never the DB*, the exact transport is P3's choice within that bound); control → engines (localhost HTTP, per-user ports); control + run units → broker (UDS, operation-in/result-out); queue = SQLite tables claimed only inside the control plane [G2 D2.1]. No gRPC, no message broker, no Redis — every added standing service is bus-factor-1 ops surface with no single-host advocate in the field [R17 §3, §5].

### S01.5 Backend language & release artifact

**The backend language is Go** — for `sinet-control`, `sinet-broker`, and `sinet-portpool` [G3 D3.2]. Grounds as ratified: the go1compat decade-stability promise, compiler-enforced typing at bus factor 1, single-static-binary deploy with embedded assets, pure-Go SQLite (no cgo), GA `anthropic-sdk-go` v1, survivor-cohort convergence [R17 §4.2]. No language couples to adopted organs: engines are subprocesses/HTTP servers behind adapters (D3), evals run via adopted runners, and Python/JS organs are consumed as processes or data files, never linked libraries [R17 §4.2].

**Release artifact:** one static Go binary with the built SPA assets embedded (`go:embed` posture) [R17 §4.2–4.3; G3 D3.3]. The broker and port-pool daemons are the same binary invoked in dedicated modes (multi-call), so deploy verifies exactly one checksummed artifact while the *process* separation of S01.2 is fully preserved [coordinator-draft]. The exact SQLite driver pin lands in `components.lock` [XREF:S16].

**The Python/Litestar fallback** documented in R17 §4.2 remains exactly that: documentation of the considered alternative and its posture. It is not a live option, not a parallel track, and imposes no compatibility duty on this spec [G3 D3.2 — operator decided outright, spike declined].

### S01.6 Startup, shutdown & maintenance mode

**Startup ordering.** `tailscaled` and `caddy` start independently (front door tolerates a not-yet-ready backend). `sinet-broker` starts before `sinet-control` and before any `sinet-engine@` unit (`After=`/`Wants=` — engines receive credentials at start [SPIKE P2-S3]). Organs (`portpool`, watchlist executor, local-model units) start independently; `sinet-control` tolerates their absence as a degraded state surfaced by the watchdog suite [XREF:S14]. On start, `sinet-control` executes, in order:

1. Load bootstrap config (`/etc/sinet`) + settings registry (S01.10).
2. **Listener-binding lint** (P-T13-2): assert its own listeners are loopback-only — failure here is fail-closed (the unit refuses to start with a named error); audit the sinet unit set for non-loopback listeners — a foreign violation surfaces immediately as a High-severity flag [R17 §7; XREF:S14].
3. Run the S02 recovery ladder over runs, effects, and leases [XREF:S02].
4. `sd_notify(READY)`; begin the `WatchdogSec` heartbeat.
5. Resume admission (scheduler claiming) [XREF:S10].

**Shutdown.** `systemctl stop sinet-control` → SIGTERM → stop admission, O(1) flush (identical to the pre-sleep path, S01.7), exit. `TimeoutStopSec` MUST exceed the flush budget. Host shutdown additionally stops run units; any loss is bounded by the last checkpoint (D7) and harvested by the recovery ladder at next start [XREF:S02]. A hard restart is therefore always *safe*, merely impolite — bounded re-spend, never a repeated outward effect (D7).

**Maintenance mode (4.5).** One operator switch:

- **Enter:** admission stops — the scheduler claims nothing new [XREF:S10]; surfaces stay readable; approvals remain answerable but answered items queue rather than launching resumes.
- **Drain:** in-flight runs continue for ⚙ `shell.drain_grace` = **15 min** [G1 Def.7]. Runs that finish, finish normally.
- **Grace expiry:** still-running runs are **parked**: the run unit is stopped and the run's next resume starts from its last checkpoint; loss is bounded by D7. Never a kill of record — parked, flagged, resumable [G1 D1.3 posture].
- **Exit:** admission resumes; parked runs resume per scheduler priority [XREF:S10].

Planned restarts and deploys SHOULD pass through maintenance mode; the deploy script treats it as the polite path, not a precondition (S01.11).

### S01.7 Sleep/wake as a first-class duty

The host is a laptop that sleeps and travels; the shell owns suspend/resume as a lifecycle event, not an anomaly [feature list Operating reality; R17 §2.11].

**Pre-sleep.** `sinet-control` holds a **delay-mode inhibitor lock** on `sleep` (the sanctioned mechanism; hook scripts are "hacks" per systemd) [R17 §4.9]. On `PrepareForSleep(true)`: stop queue claiming, checkpoint, `wal_checkpoint(TRUNCATE)`, mark in-flight run units parked. This is an **O(1) flush designed to fit even the stock 5 s inhibitor window** — recovery work is wake-side, never pre-sleep [R17 §4.9; G2 D2.1]. The v0 host's measured logind state is already 30 s via an existing drop-in [SPIKE P2-S2]; ⚙ `shell.inhibit_delay_max` = 30 s records that reality (applied as host logind configuration; restart-required; the flush never depends on more than 5 s).

**Post-resume.** Resume is detected by `PrepareForSleep(false)` plus wall-vs-monotonic clock-jump detection as backup (the Tailscale netmon pattern) [R17 §4.9]. Then, in order:

1. The S02 recovery ladder (suspend-aware grace applies) [XREF:S02].
2. **Network-identity reconcile** (P-T13-1): health-check `tailscaled` and `tailscale serve` state, verify tailnet connectivity; on failure, remediate through the deliberately designed privileged path — a scoped polkit rule vs minimal root helper decision that S11 owns with T09 review [G3 Def.7; XREF:S11]. `tailscaled` failing to reconnect after wake is documented reality, not a hypothetical [R17 §7 P-T13-1].
3. Re-run the listener-binding audit (P-T13-2) — resume is a config-drift opportunity.
4. The watchdog suite's resume-reconcile check confirms all of the above completed [R17 §4.9; XREF:S14].

**Timers.** Platform-internal schedules (snapshots, quarterly passes, canaries) run as system-manager calendar timers with `Persistent=true`, so a slot missed in suspend fires once on next activation [R17 §4.9]. v0 schedules never wake a sleeping host — availability is best-effort while the host is up [feature list Operating reality]; `WakeSystem=` remains available and unused. User-facing schedules and missed-slot policies are v1 [S00.2; XREF:S10]. `Persistent=` catch-up and cgroup freeze/thaw behavior across a real suspend: TBD-BRINGUP(operator suspend-session probe — reboot-survival + `Persistent=` catch-up + user.slice freeze/thaw).

### S01.8 Trust-chain invariants & accepted external observables

**Listener-binding audit (P-T13-2).** The identity story (serve headers, S01.9) and the tailnet wall (D1) both collapse silently if any backend unit ever binds beyond localhost. Therefore: deterministic startup lint (S01.6 step 2) + recurring watchdog check — "no sinet process listens on non-loopback; the only sanctioned exceptions are the front door (`tailscaled`, and `caddy` only if an explicit front-door binding is ever configured)" — with an explicit, operator-visible allowlist [R17 §7; XREF:S14]. Config drift here is a cousin of the config-poisoning escape class [XREF:S11].

**Accepted-external-observables register (P-T13-3).** Three metadata channels leave the LAN by design; none carries content. The register below is the complete v0 list; the operator signs it once, and 13.5 approval-help text references it [R17 §7]:

| # | Observable | Where it goes | What leaves | What never leaves |
|---|---|---|---|---|
| 1 | ts.net machine hostname | Public Certificate Transparency logs | The hostname string | Anything else about the tailnet |
| 2 | Push timing/volume | Browser-vendor push services (Apple/Google relays) | Timing, volume, endpoint metadata | Payload content (encrypted to subscription keys) |
| 3 | TLS issuance events | Let's Encrypt / CT | Issuance cadence for the hostname | Content, traffic, client identity |

Adding any fourth observable requires amending this register through the S00.9 amendment mechanics — silence would violate the platform's own honesty standard [R17 §7]. Operator sign-off: TBD-OPERATOR(observables-register sign-off — one signature, at G4 or first deploy).

**Hostname prerequisite.** **DECIDED — operator, 2026-07-19: the ts.net machine hostname is `sinet`** (bland + permanent; set as the Tailscale machine name before the first cert — the rename itself is a B0-gate step) [G3 Def.5 closed; amendment A1, S00.9]. Rationale: the name lands in the public CT ledger the moment the first cert is issued (register row 1) and anchors the WebAuthn RP-ID for any future passkeys; renaming later strands credentials and re-publishes the name [R17 §2.8, §7].

### S01.9 Identity: the authentication stack (15.6)

Three layers; the outer two are walls and hints, only the third is authoritative [R17 §4.4].

1. **Network wall.** The app exists only behind `tailscale serve` (D1); nothing listens beyond localhost (S01.1 invariant). Reaching a login screen already proves tailnet membership.
2. **Device hint.** `Tailscale-User-Login` — spoof-resistant given the S01.4 chain, because serve strips inbound copies — *suggests* the account: it prefills the login picker, and on personal devices with an operator grant it may complete login. It is device identity, not person identity: one active Tailscale account per device means a shared tablet surfaces as its registered user [R17 §2.8, §4.4].
3. **Person identity (authoritative).** Server-side sessions as owner-attributed rows in `platform.db` [15.6; schema XREF:S02]. Login = user picker + per-user PIN/password (argon2id). **Shared-device policy [G3 Def.1]:** PIN always required on shared devices; per-device trusted auto-login exists only on personal devices and only by explicit operator grant — the default for every device is shared. High-tier approvals (S3.2 High) re-prompt the PIN: approval identity is NEVER inherited from an idle session [R17 §4.4]. Every auth event (login, failure, grant, re-prompt) lands on the event log [R17 §4.4; XREF:S14].

**AuthZ.** Enforced in the control plane's data layer: every query passes through owner-scoped accessors keyed on the 15.6 owner columns [XREF:S02]; one role bit (operator vs member) implements D10 co-approval. No policy engine at household n [R17 §4.4]. SQLite has no row-level security and needs none: the sole-writer control plane is architecturally the only gatekeeper.

**Parked:** passkeys as an optional v1 enhancement (platform-sync gaps; RP-ID pins to the ts.net hostname — another reason S01.8's hostname pick precedes the first cert); tsidp watch-only [R17 §4.4; G3 follow-ups].

### S01.10 Settings-registry architecture (13.4 + G1 rider 1, made mechanical)

One **settings registry in code**. Every setting is declared exactly once with: key (dotted, per section domain), type, default, group/section, title, plain-language help (13.5), ⚙ flag, optional `(floor, ceiling)` bounds, restart-required flag [R17 §4.5]. The registry emits a JSON Schema that drives all three consumers, so UI, validation, and docs cannot drift apart:

- (a) validation-on-write in the control plane;
- (b) the generated settings UI — grouping, categorization, conditional visibility; renderer per the frontend workshop's binding pick [XREF:S15];
- (c) the generated settings reference docs.

**Storage** is the convergent hybrid [R17 §4.5, §2.9]: bootstrap-only config (bind address, DB path, broker key via systemd-creds) lives in `/etc/sinet/`; everything else is rows in `platform.db` — the default lives in code, the row stores only the override, reset-to-default = delete the row [schema XREF:S02].

**G1 rider 1 as schema:** every ⚙ number ratified anywhere in this spec seeds a registry entry with the ratified value as default. Auto-adjustable settings store `(value, floor, ceiling)`: automation (complexity-based adjustment) may move *value* strictly within bounds; only the operator edits bounds; auto-raises are visible on receipts [G1 rider 1; R17 §4.5; XREF:S10]. Every write — human or automated — appends a `settings_events` row `{actor, key, old, new, timestamp, reason}` AND emits a `settings.changed` event on the main event log [G3 Def.2; XREF:S14]. The D5 price table is its own owner-attributed table under the same audit pattern [XREF:S10].

This section defines the architecture only; the full settings index is S18's sweep of every section's ⚙ table [XREF:S18].

### S01.11 Deploy, CI & logs

**Native deploy under systemd — the platform itself is never containerized.** The control plane's job is host management (transient run units, sandbox composition, `tailscale` CLI, journald, GPU arbitration [XREF:S12]); containerizing it inverts the design. Sandboxes are S11's separate, ratified machinery [R17 §4.8, §3; XREF:S11]. Layout: `/etc/sinet` via `ConfigurationDirectory=`, `/var/lib/sinet` via `StateDirectory=` (home of `platform.db` [XREF:S02]) — zero custom path code [R17 §4.8].

**CI (D9).** GitHub Actions on tag: build → test → release artifact + `SHA256SUMS` + **signed tag**. The host deploy script: verify checksum + verify tag signature → install → (SHOULD: maintenance-mode drain, S01.6) → `systemctl restart` → `is-active` gate [R17 §4.8]. **Artifact attestations are not available to this repo**: on private/internal repos they require GitHub Enterprise Cloud (Free/Pro/Team plans: public repos only) — the documented reason the checksum + signed-tag fallback is the mechanism; it preserves most of the value and is revisited only on a plan change [R17 §4.8, §2.10]. CI additionally fails if any running unit or bundled dependency lacks a `components.lock` entry — the adoption seam enforced mechanically [G2 D2.2; XREF:S16].

**Logs.** Every unit logs stdout → journald; journald is the ops log and never the audit record — the `platform.db` event log is the only audit truth [R17 §4.8]. Underscore journal fields give unforgeable per-unit attribution; run units carry per-unit `LogRateLimitIntervalSec/Burst` overrides (chatty workers); total journal use is capped ⚙ [R17 §2.10, §4.8].

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `shell.drain_grace` | 15 min | (1 min, 24 h) [coordinator-draft] | G1 Def.7 |
| `shell.watchdog_sec` | 30 s [coordinator-draft] | (10 s, 300 s) [coordinator-draft] | R17 §4.1 (⚙ flagged unnumbered) |
| `shell.inhibit_delay_max` | 30 s (v0 host measured [SPIKE P2-S2]; logind stock default 5 s) | (5 s, 60 s) [coordinator-draft]; restart-required | R17 §4.9; SPIKE P2-S2 |
| `shell.journal_max_use` | 4 GB [coordinator-draft] | (512 MB, 32 GB) [coordinator-draft]; restart-required | R17 §4.8 (⚙ flagged unnumbered) |

**Known problems owned here:**

- **P-T13-1** — post-resume network-identity reconcile: owned at S01.7 (detection + reconcile duty); the privileged remediation path is designed in S11 with T09 review [G3 Def.7; XREF:S11].
- **P-T13-2** — listener-binding audit: owned at S01.6/S01.8 (fail-closed startup lint + explicit allowlist); the recurring check registers into S14's watchdog suite [XREF:S14].
- **P-T13-3** — accepted-external-observables register: owned at S01.8 (complete v0 register + amendment rule); operator sign-off pending (TBD-OPERATOR).

**Deferred / parked:**

- `litestream.service` unit slot → re-decided at implementation once its silent-replication-failure issue is triaged [G2 D2.5]; the committed backup lane is S13's snapshot pipeline [XREF:S13].
- GitHub artifact attestations → re-entry on a GitHub plan change [R17 §4.8].
- Passkeys → v1 enhancement; re-entry after the hostname commitment (S01.8) and platform-sync maturity [R17 §4.4].
- tsidp as SSO → watchlist only [R17 §4.4].
- `WakeSystem=` wake-the-host scheduling → unused at v0; re-entry only with a user-facing-schedule decision at v1 [S00.2].
- Suspend-cycle probes (`Persistent=` catch-up, freeze/thaw) → TBD-BRINGUP(operator suspend-session probe), tracked in STATE follow-ups.

**Coverage:**

| Feature-list item | Where |
|---|---|
| 4.5 maintenance mode + clean stop | S01.6 |
| D1 exposure (tailnet-only, no other listeners) | S01.1, S01.4, S01.8 |
| D2 credential isolation (broker as separate unit; secrets never in run units) | S01.1, S01.2 |
| 13.4 settings surface (architecture; index → S18) | S01.10 |
| 13.5 plain-language help (as registry field; surface → S15) | S01.10 |
| 15.6 owner on every record (enforcement point: owner-scoped accessors, owner-attributed sessions) | S01.9 |
| 3.8 crash/restart/sleep survival (shell duties; ladder → S02) | S01.6, S01.7 |
| 4.6 nothing fails silently (control-plane self-supervision via watchdog; run watching → S14) | S01.2, S01.6 |
| Operating reality: unattended-while-up on a sleeping, traveling host | S01.7 |
| 11.1 audit substrate posture (journald ops-only; event log = audit truth) | S01.11 |

**Open items for G4:** none. The three drafting-time sub-choices are flagged inline as [coordinator-draft] for G4 attention: the multi-call single-artifact packaging (S01.5), the `shell.watchdog_sec` = 30 s number, and the `shell.journal_max_use` = 4 GB number (plus proposed clamp ranges in the ⚙ table).

## S02 — Durable state, checkpointing & recovery

**Scope:** The authoritative durable-state layer beneath the harness — `platform.db`, checkpoint-per-paid-call, the run-lifecycle FSM, the startup/wake recovery ladder, the two-phase effect journal, and artifact-level project coordination — specifying what MUST survive crash, sleep, and disk death so that no paid work is lost, no outward effect is ever repeated, and every record is owner-attributed from day one.
**Binding inputs:** R08 §4 [G2 D2.1]; G1 D1.1 (run FSM + SQLite-WAL event log); G1 Def.4 (hold-vs-park), G1 Def.5 (freshness fingerprint); G2 Def.1 (`synchronous=FULL`), Def.2 (lease/liveness set), Def.3 (whole-project claim), D2.4 (effect-channel policy), D2.5 (backup posture); SPIKE P2-S2 (systemd harvest matrix + storage seam), SPIKE P2-S4 (serialize-by-deny); D2, D6, D7, 15.6; P-T07-1..5, P-T02-3, P-T01-1.

The vocabulary of this section: **harvest** = recovering the *result* of work that finished during an outage instead of re-running it; **generation** = a per-run monotonic fencing counter, bumped on every takeover or resume, stamped into every event append; **recovery ladder** = the ordered ALIVE / WEDGED / FINISHED-DURING-OUTAGE / DEAD classification run at start and wake; **transcript copy-aside** = a per-checkpoint file copy of a Claude-lane engine transcript segment, kept as harvest/insurance material only. These four terms are coined here.

### S02.1 The state substrate — `platform.db`

One SQLite database file in WAL mode is the authoritative state of the platform [R08 §4.1; G2 D2.1]. The `sinet-control` process **MUST be the sole writer**; dashboards, the CLI, and read-model followers open read-only connections (P-T01-1 discipline at the storage layer) [R08 §4.1].

- **Durability.** `⚙ state.synchronous = FULL` [G2 Def.1] — at Sinet's write rate (a handful of writes per paid call, seconds apart, tens/min at peak fan-out) the per-commit fsync is unmeasurable and it deletes the power-loss window entirely; `NORMAL` is the documented fallback if measurement ever disagrees, and the schema already tolerates a last-commits rollback because every cross-table invariant is written in a single transaction [R08 §2.5.1/§4.1].
- **Write discipline.** Every writing transaction **MUST** use `BEGIN IMMEDIATE` — a deferred read→write upgrade against a moved DB fails `SQLITE_BUSY` *immediately* (the busy handler is deliberately bypassed); `BEGIN IMMEDIATE` honors `⚙ state.busy_timeout = 5 s` and, once it succeeds, returns no `SQLITE_BUSY` through COMMIT [R08 §2.5.2]. `foreign_keys=ON` on every connection.
- **Read hygiene.** No long-lived read transactions — event tailing reads in short batches by `event_seq` cursor; a long reader starves the WAL checkpoint and grows the WAL without bound. Periodic `wal_checkpoint(TRUNCATE)` at `⚙ state.wal_truncate_interval = 1 h`, with a WAL-size watch feeding [XREF:S14] [R08 §2.5.3/§4.1].
- **Integrity.** `PRAGMA integrity_check` at every platform start (cheap at this size); it is also the crash-harness postcondition (S02.9) [R08 §4.1].
- **Migrations.** `PRAGMA user_version` + numbered one-transaction SQL files; event payloads carry a per-type `schema_version` and are upcast on read. **History is NEVER rewritten** — append-only [R08 §2.5.8/§4.1].
- **Event-payload discipline** (P-T07-5): payloads capped at `⚙ state.event_payload_cap = 64 KB`; bulky tool output and artifacts live as files (or engine stores) referenced by path+hash, not inlined; events are validated against their `schema_version` **before** persist — never forward an unvalidated payload into a later turn (the LangGraph corruption class) [R08 §2.2/§4.1].
- **Corruption etiquette**, enforced by convention + lint: nothing ever file-copies the live DB (snapshots go through `VACUUM INTO`, S02.9); the `-wal`/`-shm` sidecars are never deleted or renamed; the DB is never opened read-write by any process but the control plane; WAL is same-host only (never a network filesystem) [R08 §2.5.4/§5].

### S02.2 Schema core — the ~12 owner-attributed table families

The minimal durable set that covers every in-scope feature item, ratified as R08 §4.2 [G2 D2.1] and sanity-checked against Archon's 7-table reference (the delta over Archon is exactly the machinery the spec demands and Archon lacks). **Every row carries its owning `user_id` (15.6)** — identity is in the schema from day one even though v0 operates single-user. Names are indicative; exact DDL is the P3 schema workshop's job, jointly with the Ledger schema [XREF:S05].

| Table family | Holds | Anchor |
|---|---|---|
| `users` | identity, per-person credential-store refs (D2) | 15.6, 10.x |
| `tasks` | user-facing task + kanban status, orthogonal to run machinery | 9.1, S1.3 |
| `runs` | FSM `state` (S02.3), owner, task, substrate/lane, **lease** {holder, wall-clock deadline, heartbeat cursor}, **generation** (fencing counter), systemd unit name, workspace/worktree ref, **ceilings** (time/steps/cost — 3.7) | 3.7/3.8, D6 |
| `run_events` | append-only log: `event_seq INTEGER PRIMARY KEY` (the sole ordering authority — never timestamps), run_id, generation, type, `schema_version`, JSON payload, wall-clock ts. **This is the D7 event record and the observability substrate** [XREF:S14] | 4.8, 11.1 |
| `checkpoints` | one row per paid model call (S02.4) | D7, 4.3 |
| `asks` | every gate/question the moment observed: full invocation-reconstruction snapshot, status, observed/answered ts, answer payload, engine-expiry watch (P-T02-1) | 4.2/4.3 |
| `effects` | the two-phase journal (S02.7): proposal payload + normalized `payload_hash`, class A–D, approval record, state, `idempotency_key`, provider window ref, attempts, result | 4.2, 3.8 |
| `queue` / claim columns | CAS claiming: status, `claimed_by`, lease columns, priority lane | 3.3 |
| `lanes` / `slots` | per-(user, model/lane) concurrency caps as data | 3.11, D4 |
| `artifact_claims` | S1.11 registry: task, project, path/glob set, mode R/W, declared-at-plan, status | S1.11 |
| `engine_sessions` | session registry incl. copied-aside transcript path — the reboot-case mining index (P-T07-2) | 3.8 |
| `receipts` / usage | derived from checkpoint usage rows, materialized per run-end (design [XREF:S10]) | 3.6, D5 |

The Task Context Ledger is not a separate table family here: its revisions are persisted as `run_events` (type `ledger_update`, full content — it is small by design), so the D7 checkpoint payload is self-contained in the DB even if the workspace is destroyed; the working copy in the task workspace is a projection [R08 §4.2]. The Ledger's internal schema is [XREF:S05].

### S02.3 Run-lifecycle FSM

The run FSM ratified at G1 D1.1 (R05 §4.1), with the storage-layer precisions R08 adds. The current state is a column on `runs`; **every transition is a `run_events` append in the same transaction as the `runs.state` update** [R08 §4.2].

**Stored states:** `new` → `queued` (admitted to the claim queue) → `claimed` (CAS-claimed, lease held) → `running` (engine subprocess live, event cursor advancing). From `running`: `parked` (suspended on a limit event or an open gate/ask; resumes on provider signal, schedule, or approval — the limit-event taxonomy and park verbs are [XREF:S10] / [XREF:S03]); `draining` (maintenance mode — finishing the current paid call to a checkpoint boundary before parking; 4.5); and the terminals `completed`, `crashed`, `finalized` (finalize-with-card on stale resume), and `tombstoned` (a repeat-offender wedged run). `died-at-gate` is a first-class terminal for the measured case where a budget/ceiling preempts a park before it can resume [R08 §4.2, from SPIKE-measured behavior].

**Derived, NEVER stored** (computed at reconcile from unit liveness + event cursor): **stalled / wedged** — unit active but the event cursor has not advanced past `⚙ recovery.dead_after`. Keeping "stalled" derived rather than stored is a ratified FSM discipline: a stored stall would go stale across a nap and mislabel a live worker [R05 via G1 D1.1; R08 §2.3]. The recovery classes (S02.5) are likewise derived per pass.

**Transitions (summary):** `new→queued→claimed→running`; `running↔parked` (gate/limit, resume); `running→draining→parked` (maintenance drain, past the ⚙15-min grace — policy [XREF:S10]); a paid call appends a checkpoint event and stays `running`; `running→completed`; `running→crashed`, then superseded by **fork-from-last-checkpoint as a new run with `generation+1`** (S02.5); `parked→died-at-gate` when a budget preempts; `{crashed | finished-during-outage}→completed` via harvest; a repeat-offender wedged run → `tombstoned`. Effect sub-states (`executing`/`unknown`) are first-class on `effects`, not on `runs` (S02.7).

### S02.4 Checkpoint-per-paid-call

**D7 invariant, at the storage layer:** a checkpoint row **MUST** be written after every paid model call, in the same transaction as its run-event append; re-spend after any disaster is then bounded *structurally* to "work since the last paid call," and *economically* by the subscription cache TTL a warm resume rides (scheduler input [XREF:S10]) [R08 §2.2/§4.3; D7]. A checkpoint row contains, identically across substrates (measured schema parity):

- **(a) usage block** — input/output tokens, `cache_read`/`cache_creation` by TTL bucket, cost fields;
- **(b) session cursor** — substrate, engine session id *as reported by the engine's `system/init`* (never the requested id), message index, cwd key, transcript path;
- **(c) Ledger revision** — the Task Context Ledger content or content-hash; the D7 context payload [XREF:S05];
- **(d) artifact snapshot ref** — opencode lane: the native per-step snapshot id from its store; Claude lane: a platform-owned snapshot commit in the run worktree (jj is a post-v0 candidate);
- **(e) version fields** — model id, invocation-config fingerprint (settings hash + permission mode + model), tool/prompt schema versions — so recovery can detect that a resume would run under changed assumptions and route to the freshness pass (S02.6) instead of replaying blind.

**Transcript copy-aside** (Claude lane): at each checkpoint the engine transcript segment is copied aside and indexed in `engine_sessions` — cheap insurance against the ~30-day session sweep, cwd-key breakage, and crash-truncated JSONL, and the raw material for reboot-case harvest [R08 §4.3].

**Engine transcripts are NEVER durable checkpoints** (P-T01-1): the platform checkpoint row is authoritative; the engine session store and the copy-aside are a resume optimization and a harvest source, nothing more. The vendor's own doctrine agrees ("sessions persist the conversation, not the filesystem"; a resume silently returns a fresh session on cwd mismatch) [R08 §2.2].

### S02.5 The startup / wake recovery ladder

One level-triggered reconcile pass — the same code at platform start, at wake, and on a periodic sweep `⚙ recovery.sweep_interval = 5 min` — following the kubelet rule: re-list actual state, never trust supervisor memory [R08 §4.4].

**Pre-sleep** (P-T07-1): the control plane holds a logind sleep **delay** inhibitor and, on `PrepareForSleep(true)`, does only an O(1) durable flush (commit open work, `wal_checkpoint`, append a `suspend` event) inside the inhibitor budget. That budget is short and host-configurable — 5 s systemd default, 30 s on the v0 host via an unattended-upgrades drop-in [SPIKE P2-S2] — so **nothing heavy is ever sized to it**: D7 already keeps state durable at all times; all real reconcile is wake-side, enqueued by `PrepareForSleep(false)` [R08 §2.6/§4.4].

**Suspend/resume is a crash-equivalent** and the double-resume factory of P-T02-3: a suspended laptop is the GC-pause scenario made physical. Two mechanisms make it harmless — generation fencing and suspend-aware leases (below). All TCP/SSE connections are presumed dead after wake; a streamed response in flight before the nap may have completed *and been paid for* server-side, so the pass reconciles against the engine session store before any re-spend.

**The pass, per non-terminal run:**

1. **Observe actuals** — DB rows ⇄ systemd units (live + `--remain-after-exit` corpses, reading `ExecMainStatus`/`Result`/`CPUUsageNSec`/`MemoryPeak`; the lane recipe is `RemainAfterExit=yes` + `ExitType=cgroup` + `Type=exec`, **never `--collect`** — the sandbox binary exiting ≠ the process tree being dead) ⇄ engine session state ⇄ worktrees [SPIKE P2-S2; unit model [XREF:S01]/[XREF:S03]].
2. **Classify (the ladder):** **ALIVE** (unit active, cursor advancing) → reattach streams, re-arm watchdogs. **WEDGED** (unit active, cursor stalled past threshold) → pause-and-flag, **never auto-kill** (D1.3). **FINISHED-DURING-OUTAGE** (unit corpse exited 0, or the engine store shows a terminal result newer than the last checkpoint) → **harvest**: mine the result/session store, append the missing events/usage **deduplicated by (session_id, message_id)**, deliver the produced result instead of redoing the work, mark `completed`. **DEAD** (unit gone/failed with nothing newer to harvest) → mark `crashed`, then supersede by fork-from-last-checkpoint as a new run with `generation+1`.
3. **Bound recovery:** attempts reuse **one durable dispatch id** per interruption (an ambiguous failure cannot double-start); `⚙ recovery.max_attempts = 3` with backoff; repeat offenders are `tombstoned`; a run interrupted longer than `⚙ recovery.stale_finalize = 24 h` is finalized-with-card rather than blindly resumed (the freshness pass, S02.6, fires first regardless).
4. **Leases + fencing:** lease deadlines are wall-clock DB columns; expiry evaluation is **suspend-aware** — on wake, apply `⚙ recovery.wake_grace = 120 s` before declaring any lease dead (the BOOTTIME/MONOTONIC trap, P-T07-4); every event append carries the run's `generation`, and the writer **rejects stale-generation appends** — the fencing that renders double-resume inert. Ordering comes only from `event_seq`; no single clock is trusted (P-T07-4).
5. **Asks reconcile:** platform `asks` rows are authoritative (engine-side asks are volatile in memory); re-hydrate the engine where it still holds the ask, re-prompt or re-issue full reconstruction where it lost it — the ask row's invocation snapshot is the resume input.
6. **Effects reconcile:** any `executing` row is in-doubt → per-class resolution (S02.7): replay with the idempotency key (A/B), search-before-retry (C), or flip to `unknown` + card (D). **This closes 3.8's "outward actions never repeated" across the crash window.**
7. **GC:** unit corpses `reset-failed` after harvest; orphan worktrees flagged, **never auto-deleted** while they hold uncommitted work; engine session files past retention; tombstone-review cards.

**Reboot asymmetry** (P-T07-2): unit corpses survive control-plane restart, `daemon-reload`, and `daemon-reexec` (measured on the v0 host's systemd 259) but **not reboot** — the journal is persistent and survives reboot, but corpses do not [SPIKE P2-S2]. Therefore the run wrapper **MUST** write its own exit record (a `run_events` append as its last act) as the durable evidence; corpses and engine stores are enrichment, and the ladder must classify correctly from records + mining alone.

**Lease/liveness numbers** (all `⚙`, ratified as a set) [G2 Def.2]: heartbeat 60 s, dead-after 5 min, wake grace 120 s, ≤3 recovery attempts, stale-finalize 24 h, approval expiry 7 d.

### S02.6 Freshness re-validation on stale resume (4.3)

"Blocked is not failed" — an answered run resumes from its checkpoint; but if it slept past a threshold or its target moved, the remaining plan is re-validated against current reality at low cost **before spending anything significant**, then continues, adjusts-with-a-note, or escalates as an explicit decision. S02 owns the durable *inputs* to that decision; the low-cost re-plan *action* is [XREF:S06], and the hold-vs-park mechanics are [XREF:S03].

- **Freshness fingerprint (the durable set)** [G1 Def.5]: `{repo HEAD, source content hashes/ETags, spec+plan version, price-table version}` — plus, when a plan's assumptions cite project-truth knowledge entries, the **cited entry versions** (ratified extension [R11 §4.7; XREF:S09]). The checkpoint's version fields (S02.4e) plus these are what a resume compares against.
- **Triggers:** age `> ⚙ freshness.max_age = 24 h` **OR** any fingerprint drift **OR** a sibling-accept event in the project (S02.8). **Price-table drift alone triggers** — it re-prices the remaining plan, so a changed price table is a first-class staleness cause, not a cosmetic one [G1 Def.5].
- **Hold-vs-park:** a pause shorter than `⚙ freshness.hold_vs_park = 10 min` holds the process (cheap resume, no staleness); past it the run parks and is therefore subject to the freshness pass on resume [G1 Def.4].

`freshness.max_age` and `freshness.hold_vs_park` are G1-ratified and cross-cutting (fed here by checkpoint version fields, enforced by intake/adapter); they are listed in S02's settings table for completeness and deduped by [XREF:S18].

### S02.7 The two-phase effect journal (exactly-once outward effects)

The largest net-new specification in this section and the storage-layer realization of D7's gate (4.2) and 3.8's "outward actions never repeated." Guarantee vocabulary (adopted from DBOS wording): **journal/state writes are exactly-once** (same transaction as the FSM transition); **provider calls are at-least-once** (classes A–C) or at-most-once (class D); dedup lives at the effect boundary and is what makes the difference invisible [R08 §2.4/§4.5].

**Lifecycle** (`effects` row): `proposed` (created by the gate; payload normalized + hashed) → `approved` (approver, timestamp, **`payload_hash` pinned**, `⚙ effects.approval_expiry = 7 d`; Terraform saved-plan semantics — drift or expiry sends it back to re-approval, integrating the S02.6 triggers) → `executing` (own transaction, **written before the provider call**, `payload_hash` re-verified inside that transaction, `idempotency_key` = the effect UUID) → `succeeded` / `failed` / `unknown`. A restart scans in-doubt `executing` rows (S02.5 step 6).

**Per-provider idempotency registry** — data, not code (like the price table): the IETF `Idempotency-Key` draft expired with no successor and provider windows/semantics diverge widely, so uniform assumptions are forbidden [R08 §2.4/§5].

| Class | Example | Strategy | Crash-window disposition |
|---|---|---|---|
| **A** natively idempotent | `git push --force-with-lease` (ref-level CAS) | blind retry | safe |
| **B** key-dedupable | Stripe-class APIs; **Resend-class email** | retry with the stored key, respecting the provider's window (Stripe 24 h incl. error replay; Svix success-only 12 h; Adyen ≥7 d; PayPal ≤45 d) | deduped by provider |
| **C** query-before-retry | GitHub comments/issues (no idempotency) | deterministic effect-id marker embedded in the payload + search-before-retry | deduped by search |
| **D** no idempotency | plain SMTP | at-most-once execution | flip to `unknown` + a decision card showing the human what to check (5.6) |

**Idempotent-capable-channel policy** [G2 D2.4]: email-class outward effects **require** idempotent-capable providers (Resend-class keys); plain SMTP is admissible **only** as an explicit per-channel exception. Channel choice is thereby a durability design input, and the class-D `unknown` card path stays exceptional rather than routine (P-T07-3). MCP `destructiveHint`/`idempotentHint` are spec-mandated-untrusted hints — classification input at most, never a gate bypass [R08 §2.4].

### S02.8 Artifact-level coordination inside a project (S1.11)

Concurrent follow-up tasks in one project proceed in parallel when they touch disjoint artifacts and are sequenced when they would collide — detected at planning time, never resolved by silent overwrite [S1.11; R08 §4.9].

- **Write-set claims at plan time.** At plan approval a plan declares its write-set (paths/globs) and optional read-set into `artifact_claims`. The registry check is glob-intersection against active claims in the same project: disjoint → run; overlap → **sequence** (queue behind the holder) or **explicit branch** as a decision card — surfaced at plan time, which no shipped product does.
- **Whole-project claim when unbounded** [G2 Def.3]: `⚙ claims.default_write = whole-project` — when a plan cannot bound its write-set it claims the whole project (serializes more, never overwrites). The conservative default; `declared-set` is the parallel-friendlier alternative.
- **Enforcement is the control plane, not an OS lock** (claims are rows, consistent with the sole-controller posture). The ratified enforcement primitive at the tool-call gate is **serialize-by-deny** [SPIKE P2-S4]: a control-plane PreToolUse deny with the reason "re-issue the gated call alone" makes the engine re-issue the contended call by itself, which then parks cleanly with a *faithful single-call* ask record. P2-S4 demonstrated it VIABLE end-to-end (deny the batch → single serialized retry → clean park, ~+1 model turn, ~$0.013 on the cheap lane); the same primitive serializes a run behind an active write-claim. Its distinct value over the default always-defer park ([XREF:S03] owns the park default) is the faithful ask-record fidelity — always-defer parks 1-of-N and re-derives the siblings on resume.
  - **Detection binding:** classify a parallel park by **post-park transcript reconstruction**, never by reading the transcript at fire time (a flush-lag hazard that misfires); the adapter-side detection contract is [XREF:S03].
  - **Carry-forward caveat:** serialize-by-deny was demonstrated only on `claude-haiku-4-5`; it **MUST** be reconfirmed on the default worker model — the model that actually drives the ~20% parallel-batch rate — before it is relied upon. `TBD-P3(reconfirm serialize-by-deny on the default worker model)` [SPIKE P2-S4 caveat].
- **Sibling-accept** fires the ratified freshness trigger (S02.6) as an event to all active runs in the project. **At accept time**, the first gate is applies-cleanly-on-current-HEAD (the merge-queue idiom); a collision surfaces as a reviewable merge card — S1.11's "never silently overwritten," verbatim.

### S02.9 Crash-test practice and the durable-set snapshot

**The kill-9 harness is a standing conformance-suite entry**, same standing as the adapter suites [R08 §4.7; P-T01-2]: synthetic load on a mock adapter; random `kill -9` of the control plane and run units biased to the nasty windows (mid-claim, mid-journal-append, between paid call and checkpoint); restart; then assert *application* invariants — `integrity_check` ok; no per-run `event_seq` gaps; one reconcile pass classifies every non-terminal run; **zero double-executed mock effects**; `asks` ⊇ engine pending. With `synchronous=FULL` this covers the whole loss model (SQLite tests the layer below the app harder than anyone). A companion **suspend-cycle test** injects fake `PrepareForSleep` signals + clock deltas and asserts the wake-grace/fencing behavior.

**What must BE durable for 11.3** (state survives disk death): the S02.2 table set is the snapshot payload; the shape is `VACUUM INTO` a temp file (consistent against the live DB) → text-first `.dump` (diffable, deleted-content-purged) → client-side encrypt → snapshot repo, with raw `run_events` payload bodies past the 11.1 compaction horizon excluded (traces stay local). **Snapshot/backup hooks live here, but the pipeline — encryption, keep-N, rotation, and the scheduled verified-restore drill — is [XREF:S13]** [G2 D2.5]. Pinned Litestream v0.5.x is the recommended *continuous* replication addition, deferred to implementation pending bug #1083; the dump-based snapshot is the load-bearing mechanism.

### S02.10 Workspace storage seam

Each run works in an isolated workspace (a clone/worktree of the registered project store); parallel jobs cannot interfere with each other or with live files [4.1, S1.6]. The v0 host imposes a hard constraint the spec must route around, measured directly: the project volume is a **single ~420 GB ext4 root, ~91% full (~39 GB free), with no reflink support** — so copy-on-write per-run workspaces (the Devin-class fast-clone shape) are unavailable as-is; the `xfs.ko`/`btrfs.ko` modules ship, but there is no free space or partition to host a reflink-capable volume without repartitioning (the Windows NTFS partitions are the only reclaimable space) or a loopback image [SPIKE P2-S2]. A workspace/snapshot-heavy design needs disk headroom regardless of which path is chosen. **DECIDED — operator, 2026-07-18 (at S02 review): v0 workspaces use git-worktree + overlayfs on the existing ext4 volume** (lowerdir = shared project base, upperdir = per-run writes; workspace GC is a platform duty). **Loopback-XFS is the pre-registered measured upgrade** (trigger: workspace-creation latency or disk pressure becomes a real problem); **repartition is reserved for a host rebuild.** Disk-headroom management is a standing platform duty on the ~91%-full volume. (Sandbox/confinement of the workspace is [XREF:S11].)

---

**Settings introduced (⚙):** (all operator-editable with audit trail per G1 rider 1; auto-adjust only within operator ceilings)

| ⚙ setting | default | clamp / range | ratified by |
|---|---|---|---|
| `state.synchronous` | `FULL` | {FULL, NORMAL} | G2 Def.1 |
| `state.busy_timeout` | 5 s | ≥ 5 s | R08 §4.1 |
| `state.wal_truncate_interval` | 1 h | 5 min – 24 h | R08 §4.1 |
| `state.event_payload_cap` | 64 KB | 4 KB – 1 MB | R08 §4.1 / P-T07-5 |
| `recovery.heartbeat` | 60 s | 15 s – 5 min | G2 Def.2 |
| `recovery.dead_after` | 5 min | ≥ 2× heartbeat | G2 Def.2 |
| `recovery.wake_grace` | 120 s | 30 s – 10 min | G2 Def.2 |
| `recovery.max_attempts` | 3 | 1 – 5 | G2 Def.2 |
| `recovery.stale_finalize` | 24 h | ≥ 1 h | G2 Def.2 |
| `recovery.sweep_interval` | 5 min | 1 – 30 min | R08 §4.4 |
| `effects.approval_expiry` | 7 d | 1 h – 30 d | G2 Def.2 / R08 §4.5 |
| `claims.default_write` | whole-project | {declared-set, whole-project} | G2 Def.3 |
| `freshness.max_age` † | 24 h | ≥ 1 h | G1 Def.5 |
| `freshness.hold_vs_park` † | 10 min | 1 – 60 min | G1 Def.4 |

† G1-ratified, cross-cutting (4.3); consumed here (fed by checkpoint version fields), enforced via [XREF:S06]/[XREF:S03]; listed for completeness and deduped by [XREF:S18].

**Known problems owned here:**
- **P-T07-1** — pre-sleep inhibitor budget is short and host-configurable (5 s systemd default; 30 s on the v0 host [SPIKE P2-S2]) → pre-sleep is an O(1) durable flush; all reconcile is wake-side (D7 keeps state durable always).
- **P-T07-2** — reboot destroys the finished-during-outage unit-corpse evidence that restart preserves → the run wrapper's own exit-record append is mandatory durable evidence; corpses + engine stores are enrichment; journald survives reboot, corpses do not [SPIKE P2-S2].
- **P-T07-3** — idempotency-less channels leave an irreducible unknown-outcome window → `unknown` is a first-class effect state with a human card; channel choice is a durability input (D2.4).
- **P-T07-4** — no single clock is trustworthy on a sleeping laptop → ordering only from `event_seq`; deadlines are wall-clock DB columns evaluated suspend-aware (wake grace).
- **P-T07-5** — the event log is itself a growth/bloat failure mode → payload caps, refs-not-blobs, validate-before-persist, and an event-log size watch distinct from 11.1 trace retention [XREF:S14].
- **P-T02-3** (filed by T02, *mitigated here*) — suspend/wake = double-resume factory → generation fencing (stale-generation appends rejected) + suspend-aware lease grace make it inert.
- **P-T01-1** (filed by T01, *enforced here*) — engine transcripts are not durable checkpoints → platform checkpoint rows are authoritative; the transcript copy-aside is enrichment/harvest only.

**Deferred / parked:**
- serialize-by-deny on the default worker model → `TBD-P3` re-measurement before reliance [SPIKE P2-S4 caveat].
- Litestream continuous replication → implementation phase, once bug #1083 is triaged [G2 D2.5].
- jj as the workspace-snapshot engine → post-v0 workspace-restore candidate [R08 §2.2].
- DBOS-on-SQLite as the state spine → pre-registered fallback if steps ever become discrete request/response; watch TS/SQLite parity + an external-process step mode [R08 §3-B].
- MCP Tasks/Extensions (final 2026-07-28) → durable task-handle convergence for parking/cancel; S2.8 watch [XREF:S14].
- Postgres + River/DBOS-class migration → only at multi-host growth (12.7); SQLite's same-host WAL is the hard wall; the schema is deliberately portable SQL.

**Coverage:** (Scope → subsection)
| feature-list item | subsection |
|---|---|
| 3.7 run ceilings (silent/looping detection) | S02.2 (`runs.ceilings`); detection [XREF:S14] |
| 3.8 runs survive disasters | S02.4, S02.5, S02.7 |
| 4.2 effect-gating between approved & executed | S02.7 |
| 4.3 blocked-not-failed + freshness | S02.6 |
| 4.5 maintenance mode | S02.3 (`draining`); policy/grace [XREF:S10] |
| 4.8 / D7 checkpoint-and-gate | S02.4, S02.7 |
| S1.11 parallel same-project coordination | S02.8 |
| 11.3 state survives disk death (durable set) | S02.2, S02.9; pipeline [XREF:S13] |
| 15.6 owner-attributed records | S02.2 |
| D6 lease / generation per run | S02.2, S02.5 |

**Open items for G4:** none remaining.
1. ~~Workspace-storage sub-choice~~ **DECIDED — operator, 2026-07-18 (presented at S02 review from the P2-S2 storage-seam finding): (C) git-worktree + overlayfs at v0.** Rejected-for-now with re-entry: (B) loopback-XFS image — pre-registered measured upgrade on workspace-latency/disk-pressure evidence; (A) repartition to native XFS/btrfs — reserved for a host rebuild. Bound in S02.10; G4 reviews as settled.

## S03 — Execution engines & adapters

**Scope:** The D3 adapter contract and its two v0 substrates — how every provider connection (start, stream progress, checkpoint, pause, resume, cancel, report usage) is delivered by a wrapped `claude` CLI (Anthropic lane) and a pinned `opencode serve` (Z.AI + local lanes), how engines are pinned/bumped/conformance-gated, how gate-parking works per lane, and how a Sinet run is *lowered* so that compiled config is the only config an engine sees and all spawning stays control-plane-owned.

**Binding inputs:** D3, D2, D6, D7, Operating reality (subscription coverage) · [R01 §2–7], [R02 §2–6], [R05 §2–4] · [G1 P1, P2, P5, P6; D1.6; riders 2, 3] · [G2 D2.2; P6] · [G3 D3.2; §Follow-ups R06-OQ2 reminder] · [SPIKE G1-S1] CLI-vs-SDK · [SPIKE G1-S2] defer drill · [SPIKE G1-S3] opencode park + XDG · [SPIKE P2-S1] Z.AI live-auth + calibration · [SPIKE P2-S5] engine lowering · [SPIKE P2-S4 headline] serialize-by-deny · R06-OQ2 (open question, §7). Boundary siblings: [XREF:S02] checkpoint store/recovery · [XREF:S04] helper topology/spawn triggers · [XREF:S10] metering/limit taxonomy · [XREF:S12] local-lane internals · [XREF:S14] canary/eval machinery.

**New term coined here:** **engine lowering** — compiling a Sinet run down to the exact per-lane invocation an engine receives, such that Sinet-compiled config is the *only* config in effect (S03.5). **Config channel** — one independent path (settings, MCP, skills, tools, cross-read instruction files, cwd project-config, env) through which ambient environment could reach a worker; each needs its own closing knob.

### S03.1 The adapter contract (D3 verbs)

Every lane sits behind one Sinet-owned adapter interface. The adapter is the **only** component that touches engine specifics; everything above it — scheduler, watchdogs, the run FSM [XREF:S02], orchestration [XREF:S04], metering [XREF:S10] — speaks this contract and nothing else [R01 §4]. Verbs are shaped as a strict superset of ACP's stable verb set so a future transport convergence is a swap, not a re-architecture [R01 §2.4, §4].

Interface sketch (spec pseudocode; not application code):

```
adapter.start(run_id, compiled_worker, model, owner_cred_ref) -> session_cursor
adapter.stream(session_cursor) -> iter<event>      # event ∈ {message, usage, rate_limit, gate_ask, tool_result, done}
adapter.checkpoint(session_cursor) -> checkpoint_ref   # per paid call (D7); payload/store owned by [XREF:S02]
adapter.pause(session_cursor) -> park_record        # interrupt-at-safe-boundary + park (no true suspend exists)
adapter.resume(park_record) -> session_cursor       # FULL invocation re-supply; freshness-gated by [XREF:S02]
adapter.cancel(session_cursor) -> disposition       # boundary → engine-abort → group-TERM → KILL ladder
adapter.report_usage(event|session) -> usage_row    # per paid call; priced by Sinet's table, never the engine, [XREF:S10]
```

Verb → substrate mapping (updated from [R01 §4] with the spike measurements):

| Verb | Anthropic lane (wrapped `claude -p`) | Z.AI / local lane (`opencode serve`) | Adapter obligation |
|---|---|---|---|
| **start** | spawn process, control-plane-chosen `--session-id <uuid>` (honored exactly, [SPIKE G1-S1 F1]) | `POST /session` + `POST /session/{id}/prompt_async` (per-message `model:{providerID,modelID}`, `agent` by name) [SPIKE G1-S3] | per-run owner credential in the engine process, never the sandbox (D2) |
| **stream progress** | stdout JSONL `stream-json` (identical schema to the SDK; carries `assistant.usage`, `rate_limit_event`, `result`) [SPIKE G1-S1 F1] | v1 SSE `GET /event` for parks; both event generations exist — parse tolerantly [SPIKE P2-S1 Probe 6-B] | **forward-tolerant parsing MUST**: unknown event types logged and skipped, never fatal (GitButler lesson, [R01 §2.3]) |
| **checkpoint** | `result`/`assistant` usage rows per call; JSONL transcript is a *resume optimization only* | per-message token/cost rows; SQLite `event`/`event_sequence` durable log | Sinet's store is authoritative (P-T01-1); payload + store = [XREF:S02] |
| **pause** | PreToolUse `defer` (exit-park) below/above the hold threshold; message-boundary interrupt otherwise | `permission.asked` parks the turn at **zero cost** on an in-process deferred; abort + re-prompt as the restart path | never interrupt mid-tool-call (P-T01-4); durable ask-record on observation (S03.4) |
| **resume** | `--resume <id>` (same id on the defer path, no fork — [SPIKE G1-S2 F2]); `--fork-session` for retry-without-poisoning | re-prompt the surviving session id (a paid turn) | **re-supply the entire invocation** (S03.4); freshness-gate first [XREF:S02] |
| **cancel** | process-group TERM→KILL (spawn each engine as its own group leader); health-probe before reuse | `POST /session/{id}/abort` (cooperative) then group kill | boundary-first; fork-from-checkpoint is the standard recovery, not transcript repair (P-T01-4) |
| **report usage** | `result.total_cost_usd`/`usage`/`modelUsage` — emitted under subscription auth (D5 reporting figure) [SPIKE G1-S1 F1] | per-message `.tokens.{input,output,reasoning,cache.*}`; opencode's `.cost` is **$0** (flat-rate) [SPIKE P2-S1 Probe 7] | Sinet meters independently (D4) and prices from its own table (D5); engine figures are cross-checks [XREF:S10] |

Limit signals ride this contract as data (D4): Anthropic emits in-band `rate_limit_event` carrying `resetsAt`, `rateLimitType`, `overageStatus` [SPIKE G1-S1 F1]; opencode surfaces `GET /session/status` `retry` variant (`attempt`, `next`, provider `action`) [SPIKE G1-S3]. The adapter forwards them; the five-class taxonomy and park scheduling are [XREF:S10]. **Multi-host awareness (12.7/12.8):** every substrate surface is already network-transparent; if Sinet ever grows past one host the adapter contract survives unchanged [R01 §2.4] — no v0 design work.

### S03.2 The v0 lanes

v0 ships **three substrates carrying four lanes** [G1 P6; rider 3; G2 D2.2; **S00.9 A12**, which raised the count from two substrates and three lanes by adding `kimi-cli` on explicit operator order, 2026-08-26]. Adding a LANE stays config-only (S03.6); adding a SUBSTRATE is an amendment, and A12 is the one that happened.

- **Anthropic lane — wrapped `claude` CLI, pinned, one process per user.** Invocation core: `claude -p --output-format stream-json --input-format stream-json --verbose --include-partial-messages`, per-user `CLAUDE_CONFIG_DIR`, auth via each member's own `claude setup-token` (1-year, inference-scoped — the surface Anthropic's docs bless for scripts) [R01 §2.1, §4]. Model per invocation (`--model`); resume by id / fork by `--fork-session`; ceilings `--max-turns` + `--max-budget-usd` as belts [R05 §2.6]; **never** `--dangerously-skip-permissions`. Gate via PreToolUse hooks (S03.4). Compliance posture: gray-zone as-is, D2/3.4 only [G1 P2] — self-hosted, per-person-authenticated through Anthropic's own tool sits closest to the encouraged personal-use category [R01 §2.1].
- **Z.AI lane — pinned `opencode serve`, one instance per user, per-user XDG isolation.** Pin `opencode-ai@1.18.3` **exact** (npm `latest` is still 1.x but flips at 2.0 GA; `@1` floats) [SPIKE G1-S3]. Each instance runs under fully disjoint `XDG_{CONFIG,DATA,STATE,CACHE}_HOME` — measured to fully contain auth.json, the SQLite DB, config, cache and logs with **zero cross-leak and zero strays** on the serve path (#18633 did not reproduce) [SPIKE G1-S3 F1]. Bind `127.0.0.1`; set a per-instance `OPENCODE_SERVER_PASSWORD` (enforced as HTTP **Basic** auth on *every* endpoint including `/api/health` — the adapter client and health-watcher both send it) [SPIKE P2-S1]. Provider `zai-coding-plan`, endpoint `https://api.z.ai/api/coding/paas/v4`; the operator holds GLM Coding Max [G1 D1.5]. Per-user first boot pulls a ~62 MB provider dependency tree and needs network once [SPIKE G1-S3 F1]. Z.AI is a class-2 (tool-scoped, no unattended-use ban) plan run inside a whitelisted tool — accepted as-is under one uniform policy [G1 P5; R02 §2.4].
- **Local lane — local models exposed to the same `opencode serve` adapter as OpenAI-compatible providers** (llama-swap → llama-server). Model set, GPU arbitration, and platform-internal local duties (ceremony, watchdogs, utility models) are [XREF:S12]. Adapter-relevant fact: it is the **only** v0 lane exposing logprobs, hence the only one with a logprob drift-canary; both subscription lanes fall back to behavioral evals [SPIKE P2-S1 Probe 8; XREF:S14]. Otherwise it inherits the opencode adapter wholesale.

**Agent-SDK-as-in-adapter-alternative.** The Anthropic adapter is built against Sinet's own contract; the *mechanism* is a v0 sub-choice, not an architecture fork [R01 §3-D]. Spike verdict: keep **CLI-wrap** for v0; the Agent SDK is a verified near-drop-in fallback kept documented inside the same adapter [SPIKE G1-S1 §4]. Every mechanical criterion measured at parity — identical stream-json schema and per-call usage (D7 rows), identical session substrate/resume, **healthy cache fidelity on both** (the #39732 "SDK caching disabled" state is not current — [R05 §2.2]'s "CLI has the healthier caching story" is stale), equivalent gating incl. typed `defer`, and subscription auth on both (`apiKeySource:"none"`, no API-key demand) [SPIKE G1-S1 F3–F5]. The residual margin is **policy** (CLI+`setup-token` is the blessed scripts surface) and **drift-surface** (the SDK adds a second pre-1.0 API layer over the same engine). Flip conditions, pre-registered: a written Anthropic personal-use safe harbor; CLI stream-json churn outpacing the SDK's typed layer; the defer economics favoring held-process parking; or a cache/`defer` regression on the CLI surface only [SPIKE G1-S1 §4].

### S03.3 Engine version policy & drift

Both substrates are **pinned versioned dependencies**; schema and behavior drift is *when*, not *if* — 321 `claude` releases/yr on one lane, an announced V1→V2 API break on the other [R01 §2.3, P-T01-2]. Pins live in the adoption manifest / `components.lock` [XREF:S16]; the bump procedure lives here:

1. **Pin exact** (`claude` 2.1.214, `opencode-ai@1.18.3` today); never track `latest` [SPIKE G1-S3, P2-S5].
2. A **deliberate bump** is an operator-gated release event, never automatic. `opencode`'s `POST /global/upgrade` self-upgrade endpoint MUST never be exposed or called [SPIKE G1-S3 F2].
3. **Conformance gate before a bump lands:** the per-substrate conformance suite MUST pass on the candidate version *and* a before/after **quality** probe on a small task battery must show no regression — the Apr-2026 Claude-Code postmortem class (silent effort/context-handling drift) is invisible to schema tests and uptime checks [R05 §2.8, P-T02-5]. Suite/probe machinery is [XREF:S14]; the gate policy is normative here.
4. Conformance suites **assert behavior, never docs** — the vendor's own docs run months stale (Ollama logprobs; opencode defer-page truncation) [G3/R16 finding; SPIKE G1-S2 §5].
5. A bump is a **worker-revalidation trigger**: every worker version tuned against the bumped engine is flagged for revalidation before further unsupervised use, and Sinet's bump *is* the announcement clock — no provider clock announces it (P-T14-1) [XREF:S08].

Four adapter-facing drift facts bind the design:

- **Engine transcripts are not durable state (P-T01-1).** Claude JSONL is CWD-keyed, absent from the sessions index for `-p`, corruptible, and swept after `cleanupPeriodDays`; opencode keeps sessions in SQLite but **not** pending asks. → Sinet's store is authoritative; engine sessions are a resume optimization; copy-aside is [XREF:S02]; `⚙ adapter.claude.cleanup_period_days` is raised well above the park horizon [SPIKE G1-S2 F5].
- **Schema drift is permanent even first-party (P-T01-2 / P-T02-2).** → Forward-tolerant parsing (S03.1) + conformance suites + a standing cache-fidelity alarm (`cache_read` stuck flat across an in-TTL resume; healthy baseline calibrated in [SPIKE G1-S1 F3]) whose machinery is [XREF:S10/S14].
- **Billing-regime shifts are ~30-day ops events (P-T01-3).** Handled as *data* — per-model flat/metered flags with a rehearsed flip and receipts that visibly change currency [XREF:S10]. The Anthropic un-pause response is **pre-registered [G1 P1]**: the Anthropic lane demotes to interactive-only and headless weight shifts to Z.AI-class/local lanes — a policy/data flip, **no architecture change**; detection is the S2.8 watch [XREF:S14].
- **Cancel corrupts JSONL mid-tool-call (P-T01-4).** → The cancel ladder prefers message-boundary interrupts, runs a session-health probe before any reuse, and treats fork-from-checkpoint as the standard recovery (opencode's mid-abort corruption family was fixed 2026-04; Claude's is canon) [R05 §2.7].

### S03.4 Gate parking & defer mechanics per lane

Proposals (4.2) are collected at the gate and the run parks losing nothing (4.3). No engine offers true mid-run suspension; "pause" = interrupt-at-safe-boundary + park + external-event resume [R01 §2.3]. The **durable ask-record is a platform record from the moment the ask is observed** on *both* lanes — engine-side park state is volatile (P-T01-1, P-T02-1), confirmed at source and live.

**Anthropic lane — `defer` (PreToolUse) is the exit-park primitive** [R05 §2.3; SPIKE G1-S2]. The headless process exits `stop_reason:"tool_deferred"` carrying a `deferred_tool_use{id,name,input}` object — the adapter builds its ask-record from the exit JSON alone, no transcript parsing [SPIKE G1-S2 F1]. `--resume` re-fires PreToolUse on the same `tool_use_id`; the hook returns allow + `updatedInput` (the human's answer) with no fork; poll-resume while parked is genuinely free ($0, zero tokens) [SPIKE G1-S2 F1]. Two traps and two obligations bind the adapter:

- **Trap — single-tool-call-only.** `defer` is honored only when the turn contains one tool call; parallel-tool-call turns fall back to normal permission flow **silently** (no stderr warning in `-p` json mode). Measured fallback rate ≈ **20% of gated turns on the default worker model** (8.5% on opus-4-8) — a first-class path, not an edge case (P-T02-4) [SPIKE G1-S2 F3]. Detection is the adapter's job: >1 PreToolUse fire for one turn + no `tool_deferred` exit ⇒ fallback happened. Chosen fallback: **serialize-by-deny** — deny all calls of a parallel gated turn with reason "re-issue the gated call alone", converting fallback into ~one extra cheap turn rather than a held process; measured viable on haiku (~+1 turn) [SPIKE P2-S4 headline] `[coordinator-draft]`, with `⚙ adapter.parallel_gate_fallback` and a bring-up reconfirm on the default worker model, TBD-BRINGUP(parallel-gate fallback rate on default worker model) — **closed A3 (2026-07-22)**: reconfirmed PASS at P3-B3-3.
- **Trap — cleanup-sweep evaporation.** A deferred session is still subject to the `cleanupPeriodDays` startup sweep (default 30 d) — a parked call silently evaporates past retention (P-T02-1) [SPIKE G1-S2 F5]. → `⚙ adapter.claude.cleanup_period_days` raised above the park horizon; the platform ask-record is authoritative regardless.
- **Obligation — full config re-supply on every resume.** The engine restores **nothing** — not hooks, and (measured, correcting [R05 §2.3 S41]) **not permission mode**; a resume that forgets `--settings` silently executes the parked call unreviewed [SPIKE G1-S2 F2/F3]. The park-record snapshots the entire invocation (settings content+fingerprint, permission mode, model, tool allowlist) and re-supplies all of it. Schema = [XREF:S02].
- **Obligation — ceiling ordering.** A same-turn `--max-budget-usd` trip pre-empts the park (dies `error_max_budget_usd`, no `deferred_tool_use`), so the engine ceiling MUST sit ≥ `⚙ adapter.engine_ceiling_backstop_mult` × the platform ceiling; the platform ledger is the real ceiling [SPIKE G1-S2 F4; XREF:S10].

**Z.AI / local lane — `permission.asked` + REST answer** [SPIKE G1-S3, P2-S1]. Live-confirmed round-trip: the turn parks `busy` at zero cost on an in-process deferred; enumerate **all** entries of the flat `GET /permission` array (`permission.list`); answer via `POST /permission/{id}/reply {reply, message?}` (keeps reject-with-feedback `CorrectedError`, unlike the legacy `permission.respond`); key everything by `requestID` — one `always`-class reply fans out to N `permission.replied` events [SPIKE P2-S1 Probes 1, 6]. Binding constraints:

- **Subscribe to v1 `GET /event`** for `permission.asked`/`permission.replied`; the global v2 `/api/event` does not carry them in 1.18.3 [SPIKE P2-S1 Probe 6-B].
- **Never send `"always"` through opencode** — v1 `always` rules are in-memory **and server-wide**, leaking approvals across a person's sessions and vanishing on restart; the adapter emits only `once`/`reject` and keeps persistent allow-policy in Sinet's own layer [SPIKE P2-S1 Probe 6-A; impl. #2].
- **Restart-ask-loss + containment.** Pending permission *and* question asks are in-memory `Map`s, rejected on graceful shutdown and lost on crash — source-confirmed and reproduced live (#36347, unfixed; PRs closed unmerged) [SPIKE G1-S3 F4; P2-S1 Probe 2]. Containment: persist on `permission.asked`; treat a serve restart as "ask gone, session intact, **re-prompt the surviving session to recover** (a normal paid turn)"; the adapter marks the orphaned call cancelled in its **own** checkpoint — opencode leaves the transcript part cosmetically `running` and must not be trusted [SPIKE P2-S1 Probe 2]. Shutdown rejection is **silent** (no bus event); the adapter infers a dead park by diffing its ask-record against `permission.list` after any restart/health blip [SPIKE P2-S1 Probe 5].
- **HTTP-only origination.** Originate every turn over HTTP and never attach a TUI to a Sinet-owned serve — #16367 (attach hang under `ask`) is orthogonal to the HTTP path (measured immune), but #36835 makes TUI-initiated asks unanswerable over REST [SPIKE P2-S1 Probe 3].

### S03.5 Engine lowering & the sole-controller posture

**Engine lowering** — the run is compiled down to the exact per-lane invocation, and **compiled config is the ONLY config an engine sees** [SPIKE P2-S5; R15 §4.1]. The load-bearing spike finding: isolating *settings* is necessary but **not sufficient** — MCP servers, skills, tools, HOME-relative `.claude/` cross-reads, and walked-up cwd project-config are **separate config channels** that each bleed the ambient/operator environment into a worker unless explicitly closed. The guarantee is therefore **COMPOUND: one knob per channel**, enforced as a per-lane conformance-suite checklist entry (attempt-the-leak tests that assert the decoy did *not* take effect) [SPIKE P2-S5 §"Adapter requirements"; XREF:S14].

| Config channel | Anthropic lane knob (2.1.214) | Z.AI/local lane knob (1.18.3) |
|---|---|---|
| settings / hooks / CLAUDE.md | `--settings <sinet.json>` + `--setting-sources ""` | isolated `opencode.jsonc` **or** inline `OPENCODE_CONFIG_CONTENT`; per-user disjoint XDG |
| MCP / connectors | `--strict-mcp-config` (+ `--mcp-config` for Sinet servers only) | (opencode MCP configured only in the isolated file) |
| skills / slash-commands | `--disable-slash-commands` (ship Sinet skills as pointed-at dirs, not discovery) | — |
| **tools incl. native spawn** | `--tools "<allowlist>"` (default-deny; **MUST exclude `Task` + `TaskCreate/Get/List/Output/Stop/Update`**) | `permission.task:"deny"` (blanket) |
| HOME-relative cross-reads | (closed by `--setting-sources ""`) | `OPENCODE_DISABLE_CLAUDE_CODE=1` — kills `~/.claude/CLAUDE.md` + project `CLAUDE.md` + `.claude/skills` scans (HOME is unchanged under XDG, so required) |
| cwd project-config | (isolated cwd; no project `.claude` write) | `OPENCODE_DISABLE_PROJECT_CONFIG=1` — an adversarial walked-up `opencode.json` can otherwise re-enable `task:"allow"` (leak reproduced) |
| plugins | (none loaded) | `OPENCODE_PURE=1` + `OPENCODE_DISABLE_DEFAULT_PLUGINS=1` |
| agent / prompt body | `--agents '<json>' --agent <name>` (+ `--append-system-prompt`); inline, no disk write | agent map in the isolated config, selected per turn by **name** in `prompt_async` (no inline per-request agent *definition*) |
| nested-session env | scrub `CLAUDECODE`, `CLAUDE_CODE_*`, `AI_AGENT`, `CLAUDE_EFFORT`, `CLAUDE_PID` | (XDG env set on every invocation, G1-S3) |

The compiled worker body+config is hash-pinned per run [R15 §4.1]. D8 worker templates compile to these targets; the guardrail split (behavioral content in files, all enforcement state in control-plane tables) is [XREF:S08].

**Sole-controller posture [G1 rider 2].** All agent spawning is control-plane-owned: helpers are control-plane-spawned sibling sessions, briefed-in and reported-out, with D6's depth-cap/spawn-logging/no-lateral enforced in `sinet-control` — **no engine enforces D6** [R06 §5; XREF:S04]. Engine-native subagent features are **disabled on every substrate**, and the spike proved this is *structural*, not merely behavioral: on the Anthropic lane the `--tools` allowlist excludes the whole `Task*` family; on the opencode lane `permission.task:"deny"` **strips the tool from the model's toolset pre-inference** (bypass-proof: not model-settable, inherited by any subagent, `subagent_depth` defaults to 1) [SPIKE P2-S5 Probes 2, 5]. R06-OQ2 (re-enabling native micro-fanout) is *deferred, not open*, and is surfaced for the operator at G4 with the current binding posture (disabled) — see Open items.

### S03.6 Lane roadmap & onboarding

v0 = Anthropic + Z.AI (+ local); every other provider is **parked/deferred and joins post-v0 config-only** via the report-02 §5 onboarding checklist [G1 P6, P7; rider 3; R02 §5]. **Kimi (Moonshot) joined this set early, on explicit operator order, through that checklist rather than around it [S00.9 A11, 2026-08-24]** — the rider-3 mechanism working as written, not an exception to it. **A fourth lane `kimi-cli` followed on 2026-08-26 [S00.9 A12]**, and it is the worked example of the sentence below being true in one direction only: adding a lane never forces a new substrate, but this lane's engine IS a new substrate, so the lane rode the checklist while the substrate rode an amendment. It draws the SAME membership quota pool as `kimi` — one pool, declared once, never two allowances — and its Gate A is a RECORDED GRAY ZONE on the operator's 2026-08-26 acceptance of the Community Guidelines' interactive-only clause, which binds both kimi lanes identically. Adding a lane is a provider entry per user plus billing flags — never a new substrate: sanctioned providers ride opencode as OpenAI- or Anthropic-compatible config (xAI via native OAuth, Synthetic/StepFun/etc. via endpoints) [R02 §4]. The onboarding checklist's manifest home is [XREF:S16].

- **Three-class ToS taxonomy as spec vocabulary** [R02 §2.4]: class 1 open-programmatic → pass; class 2 tool-scoped without an unattended-use ban → pass with a gray-zone note only if run inside a Sinet-run tool; class 3 explicit interactive-only/automation-banned → **auto-disqualifying**. A lane must never force adopting a new engine.
- **Auth canary is distinct from limit handling per lane (P-T17-1)** [R02 §9; XREF:S14]. Sanction is allowlist-shaped and revocable server-side without notice; an auth-shaped failure classifies as *policy-revocation-suspected* → operator alert + lane freeze, **never** an infinite 3.2 retry-park. The limit-event taxonomy that the canary is distinct *from* is [XREF:S10]; the canary's scheduled machinery is [XREF:S14].
- **Two required per-model attributes** learned in research [R02 §4]: `overflow_mode` (`hard-stop` / `opt-in-credits` / `auto-metered` — the last acceptable only with a proven disable/zero-balance, else reject; 3.10 — **P-T17-2**, id restored per S17-R1 [coordinator-draft]) and `region_model_gate` — the adapter periodically **diffs each account's *observed* model list against config** (P-T17-3); routing and 2.7 gap advice consume the observed list with `verified-on` dates. The metered-exception list is **empty at v0** [G1 P7]; DeepSeek is the pre-registered designated exception should one ever be enabled [R02 §4].

### S03.7 Z.AI adapter-binding specifics

Two Z.AI facts, measured live, bind the adapter and cascade to other sections by XREF [SPIKE P2-S1 Probes 7, 8]:

- **No wire-side usage endpoint, no per-prompt counter.** Response bodies carry only `[choices, created, id, model, object, request_id, usage]`; headers carry no `x-ratelimit-*`; `…/usage`, `…/user/usage`, `…/dashboard/usage`, `…/v1/usage` all 404. Exact per-request token `usage` (incl. cached-token detail) is available, but the 5-hour-window prompt-unit % is retrievable **only from the operator dashboard** → prompt-unit consumption stays derived/approximation-tier and dashboard-calibrated; consequence lands in [XREF:S10]. `request_id` is the reconciliation key to the dashboard. Corollary: **never meter dollars from opencode** (it prices `zai-coding-plan` at $0) — price from Sinet's metered `zai` rows [XREF:S10].
- **No logprobs on the Z.AI coding endpoint** (`logprobs:true` → 200 but no `logprobs` key, endpoint-wide) → the Z.AI lane is **behavioral-eval-only** for drift detection; only the local lane gets the logprob canary [XREF:S14; S12].
- **`thinking:{type:disabled}` is a first-class Z.AI effort knob** (~50× token reduction on trivial work); the adapter exposes it as a Z.AI-lane lever for the Eco/Balanced effort ladder [XREF:S10].

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `adapter.claude.cleanup_period_days` | 365 | integer ≥ 30; MUST exceed the ask-expiry horizon (7 d, G2 Def.2) | P-T02-1 mitigation; [SPIKE G1-S2 F5]; G1 rider 1 |
| `adapter.engine_ceiling_backstop_mult` | 2 | ≥ 2.0 | [SPIKE G1-S2 F4]; G1 rider 1 |
| `adapter.parallel_gate_fallback` | `serialize-by-deny` | {`serialize-by-deny`, `hold-process`} | [SPIKE G1-S2 §Verdict; P2-S4 headline]; `[coordinator-draft]` |

Engine version pins (`claude` 2.1.214, `opencode-ai@1.18.3`) are operator-editable via the deliberate-bump procedure (S03.3) but their manifest home and audit trail are `components.lock` [XREF:S16], not duplicated here. Every ⚙ number ships as an operator-editable setting with audit trail (G1 rider 1).

**Known problems owned here:** P-T01-1 (transcripts not durable → authoritative platform store + copy-aside [XREF:S02]; `cleanup_period_days` raised) · P-T01-2 (schema drift → pinning + forward-tolerant parsing + conformance suites) · P-T01-3 (billing shifts → data-not-architecture; pre-registered un-pause response [G1 P1]; flip mechanics [XREF:S10]) · P-T01-4 (cancel corrupts JSONL → boundary-interrupt + fork-from-checkpoint) · P-T02-1 (parked state has engine expiry → durable ask-records authoritative on observation) · P-T02-4 (defer single-tool-call bound → serialize-by-deny fallback; PreToolUse-hook gating, never `canUseTool`) · P-T02-5 (engine harness is a quality-drift source → bump = release event with before/after quality probes [XREF:S14]) · P-T02-2 (resume-cache fidelity drifts silently → standing cache-fidelity alarm vs the calibrated healthy baseline [SPIKE G1-S1 F3]; ownership adopted per S17-R2 [coordinator-draft]; alarm machinery [XREF:S10/S14]) · P-T17-1 (allowlist-revocable sanction → auth canary distinct from limit handling) · P-T17-2 (auto-overflow-to-metered violates 3.10 → `overflow_mode` classified at onboarding, unprovable-disable rejected; id restored per S17-R1) · P-T17-3 (region/account-dependent model list → observed-vs-config diff; `overflow_mode`/`region_model_gate`). Shared/referenced: P-T14-1 (engine-pin bumps = mass revalidation events [XREF:S08]).

**Deferred / parked:**
- ~~TBD-P3(PreCompact-blocking + Claude-lane injection-mechanics spike)~~ — **CLOSED A4 (2026-07-23, B4 gate)**: spike PASS at P3-B1-4 (PreCompact can veto compaction; SessionStart source:'compact' re-injection confirmed; containment stays primary per S05) — result file `P3/measurements/2026-07-20-precompact-injection-mechanics.md`.
- ~~TBD-P3(Claude-lane auto-memory containment, R11-OQ6)~~ — **CLOSED A2 (2026-07-22)**: containment implemented + conformance-tested at P3-B3-1 (platform-supplied config-root `memory/` wiped at session start, resume exempt); engine-native memory default-off confirmed by the B1-4 spike (result file 2026-07-20). Engine memory features remain a S2.8 canary-suite row [XREF:S14; G2 §Follow-ups].
- ~~TBD-BRINGUP(parallel-gate fallback rate on the default worker model)~~ — **CLOSED A3 (2026-07-22)**: serialize-by-deny reconfirmed PASS on the S08-designated default worker at engine 2.1.216 (P3-B3-3, result file 2026-07-21; B1-4 spike-1 PASS-PROVISIONAL made final); fallback stands ratified, GateFallback detector retained as a dormant per-pin canary [SPIKE P2-S4 carry-forward].
- TBD-OPERATOR(Z.AI dashboard prompt-unit calibration) — 5-step recipe feeds [XREF:S10]; re-entry: operator convenience [SPIKE P2-S1 §Blocked].
- Post-v0 lanes (xAI, Synthetic, StepFun, DeepSeek metered exception, …) — parked; re-entry: operator holds a sanctioned plan → onboarding checklist (S03.6). **Kimi (Moonshot) has LEFT this row**: the operator holds a membership serving K3 and ordered the lane early, so it ran the checklist on 2026-08-24 and is commissioned [S00.9 A11]. It is the worked example of the re-entry path the rest of this row still describes.

**Coverage:**

| Feature-list item | Subsection |
|---|---|
| D3 dual substrate behind one adapter contract | S03.1, S03.2 |
| D2 per-run owner credential in engine process, never sandbox | S03.1 (start), S03.2, S03.5 |
| D6 sole-controller / no engine-native spawn | S03.5 + Open items (R06-OQ2) |
| Operating reality subscription coverage; un-pause response | S03.2, S03.3 (P-T01-3; [G1 P1]) |
| 3.2 limit events surfaced at the adapter (taxonomy [XREF:S10]) | S03.1 |
| 2.7 gap-advice / lane roadmap | S03.6 |
| S2.8 adapter behavior-drift watch (machinery [XREF:S14]) | S03.3, S03.6 |
| 12.7/12.8 multi-host awareness | S03.1 (note) |

**Open items for G4:**

> ### OPERATOR DECISION (standing G1-rider-2 reminder): engine-native micro-fanout
> *The coordinator raises this explicitly at G4 per the operator's standing request [G1 rider 2; G2 §Follow-ups; G3 §Follow-ups]. It is **deferred, not open** — the surrounding spec (S03.5) already binds the CURRENT posture: disabled. This box asks only whether to keep it that way.*
>
> **What it is.** "Native micro-fanout" = letting a coordinator's *engine session* spawn its own subagents through the engine's built-in task tool (opencode `task` / Claude Code `Task`) at depth 1 for a small parallel read fan-out — instead of `sinet-control` spawning, briefing, and harvesting separate sibling sessions [R06 §3-D].
>
> **What re-enabling it inside one adapter call would buy.** The engine handles a tiny 2–4-facet read fan-out inline within a single turn — lower orchestration overhead and latency than the control-plane sibling-session path (spawn → brief → stream → report round-trip) for the smallest fan-outs; the model decides the fan-out itself.
>
> **What it costs / what sole-control loses by keeping it off.** The verifying observability spike (G1 **S4**) was **removed** at [G1 D1.6] when the operator adopted the sole-controller posture — so there is no measured guarantee that native spawns are loggable at spawn; they would be *reconstructed* from event streams, not enforced [R06 §3-D, §5]. The engines don't enforce D6: opencode's guard is silently disableable — a walked-up cwd `opencode.json` re-enabling `task:"allow"` **was reproduced live** [SPIKE P2-S5 Probe 4] — and its documented runaway is 47 sessions / 20 levels [R06 §5]; Claude Code's native cap is 5 and non-configurable. Re-enabling moves D6's spawn-logging and depth-cap guarantees from control-plane-enforced to engine-config-trusted, which the evidence shows is unreliable. Keeping it **off** costs only a modest latency/overhead saving on micro-fanouts — and on flat-rate lanes marginal helper cost is consumption pressure, not dollars [R06 §4.5]; the control-plane sibling path already delivers the two cheapest wins (context protection, read fan-out) with D6's guarantees in Sinet-owned code [XREF:S04].
>
> **Recommendation (consistent with ratified evidence): keep DISABLED.** It is the ratified stance [G1 rider 2], and P2-S5 proved native spawn is cleanly, structurally disablable (tool stripped pre-inference) [SPIKE P2-S5 Probe 5]. Precondition for any future revisit: an S4-class observability spike passing **and** an engine config that guarantees every native spawn is logged-at-spawn and depth-capped — which no v0-pinned engine currently provides.
>
> **DECIDED — operator, 2026-07-18 (at S03 review, per the standing reminder): KEEP DISABLED.** The sole-controller posture (S03.5) is confirmed as final spec text; the G1-rider-2 standing reminder is discharged. Re-entry only via the precondition above, as a dated amendment (S00.9).

## S04 — Orchestration within D6

**Scope:** How task work is and is not split across agents: the coordinator/helper topology made operational, the earned-helper triggers, the brief/report context firewall, spawn budgets and the spawn log, the helper lifecycle with acceptance screening and sibling-failure containment, artifact-collision serialization, and cost shape/attribution.
**Binding inputs:** D6 · feature list 2.4, 7.7, 14.1, 14.4 (+ 3.4, 4.6, 5.6 slices) · R06 §4 as ratified [G1 D1.1] · G1 Def.8, Def.9, Def.10, riders 1–2 · G3 Def.3 · [SPIKE P2-S4], [SPIKE P2-S5] · S00.7 glossary (coordinator, helper).

### S04.1 Topology: D6 made operational

The unit of delegation is the **(coordinator, helpers)** tree of one task:

- **One coordinator per task.** The coordinator is the single engine session driving the task's current stage; at no moment does a task have two coordinators (D6). Stage transitions replace the coordinator's context, not its singular role — fresh-context-per-stage runs on the Task Context Ledger [G1 D1.1; XREF:S05].
- **Helpers are isolated.** A helper is a separate engine sibling session with its own confinement-class-appropriate sandbox [XREF:S11], its own context built only from its brief (S04.3), and no shared conversational state with anything. Helpers are ephemeral: they exist from spawn to terminal outcome and are never standing formations (14.4).
- **Sub-helpers within the depth cap.** Depth counts levels below the coordinator: helpers run at depth 1, sub-helpers at depth 2. `⚙ orchestration.depth_cap = 2` (D6 default); **depth 1 is the operating norm** — nothing in the evidence base demonstrates value at depth ≥3 [R06 §4.3; G1 Def.8]. A helper may propose a sub-helper through the same control-plane spawn API, subject to the same triggers, budgets, and logging; a spawn request at depth > cap is refused.
- **No lateral messaging at any depth** (D6; 14.1). The only communication edges are the spawn edges: brief down, report up. A sub-helper reports to its spawning helper, which integrates the result into its own report — the coordinator never receives a sub-helper report directly, and no helper ever sees a sibling's report, brief, or existence. The control plane provides **no** helper-to-helper channel; a conformance test attempts a lateral send and asserts refusal (S04.5).
- **The control plane is the only enforcement point.** No engine enforces D6: surveyed engines ship fixed or silently-bypassable depth guards, no spawn-reason logging, and no lateral-messaging prohibition — the cap, budgets, logging, and no-lateral rule are therefore control-plane-owned and engine-independent [R06 §2.5, §7; G1 D1.1]. Enforcement is structural (the spawn API is the only way a helper can come to exist), not advisory.
- **Engine-native spawning is disabled on every substrate** [G1 rider 2]. Sinet holds the sole-controller posture: all agent spawning is control-plane-owned; engine-native subagent features stay off on every lane. A helper exists only as a control-plane spawn lowered through the adapter's D3 verbs — full stop. [SPIKE P2-S5] confirmed native spawning is fully disablable via compiled config but that the guarantee is **compound** (one knob per channel); the per-lane disablement knobs and their conformance probes are S03's [XREF:S03]. An engine-native spawn observed at runtime is a platform defect, not a policy option. R06-OQ2 (a narrow native micro-fanout exception) is deferred, not open; it resurfaces only at S03 review with an explicit operator reminder [G1 rider 2].

**Boundary lines.** S03 owns lowering spawns onto engines and the native-spawning disablement mechanics. S05 owns context/ledger internals (what a brief's context slice is built from). S07 owns verification of helper-derived deliverable content beyond the S04.5 acceptance screen. S08 owns which worker/model/lane a helper runs on. S10 owns scheduling, pressure accounting, and receipt mechanics. S11 owns helper confinement classes.

### S04.2 Single-agent-first: the earned-helper test

The default formation for every task is **one agent** — the coordinator working alone (14.4; single-agent-first [G1 D1.1]). Helpers are never a default and never decorative: a spawn is legal only when at least one of **exactly three** triggers holds, and the trigger is recorded in the spawn log as the machine-readable spawn reason (D6; 7.7) [R06 §4.1]:

- **T-CTX — context protection.** The subtask would flood the coordinator with material it will not reference again (grep/log/doc trawls, large read-ins). Even a single helper pays here.
- **T-PAR — read fan-out.** ≥2 genuinely independent, read-only facets whose results merge only at synthesis. NEVER for write work.
- **T-SPEC — specialization quarantine.** The subtask needs a tool/permission set the coordinator must not hold (including parsing untrusted content — the injection blast-radius case [XREF:S11]) or a materially different model/effort tier.

Anything that matches no trigger runs single-agent. The coordinator's prompt carries the effort rubric (models misjudge effort unaided [R06 §2.5]): simple lookup → 0 helpers, inline; one noisy read → 1 helper; independent facets → 2–4 helpers; more requires spawn-budget headroom (S04.4).

Formation rubric — 2.4's "best practice chooses structure and depth within the cap" made concrete [R06 §4.1; G1 D1.1]:

| Task shape | Default formation |
|---|---|
| Simple lookup / anything the coordinator does in-window | coordinator only |
| One noisy read subtask (log/doc/code trawl) | 1 helper (T-CTX) |
| 2–4 independent read-only facets | parallel helpers (T-PAR), reports merged at synthesis |
| Parsing untrusted external content | 1 quarantined helper (T-SPEC), minimal tools, report-as-data [XREF:S11] |
| Judgment/review subtask on finished work | 1 clean-context helper (T-SPEC), no shared history (platform verification stages proper are S07's [XREF:S07]) |
| Sequential / tightly-coupled reasoning | coordinator only — never decompose |
| Any write work | coordinator writes, single-threaded |
| Mass mechanical fan-out (≥50 items) | out of v0 scope — parked (see Deferred) |

**Writes are single-threaded, always** [R06 §4.1]: one writer per workspace; helpers take write work NEVER. Parallel write work exists only as separate sibling *tasks* in isolated workspaces, coordinated by artifact claims [XREF:S02] — never as helpers of one task.

Spawning within budgets is not an outward effect and needs no human approval; it is logged, not gated. The only human gates in this section are the spawn-budget overrun gate (S04.4) and operator ceiling changes [G1 rider 1].

### S04.3 The context firewall: brief in, report out

The helper context contract is the anti-15× mechanism: Nexus's ~15×-class token blowup came from re-sending context per stage, and an undesigned contract (full re-sends, paraphrase chains, cold caches) reproduces it; the designed shape below yields ≈2–4× single-agent cost (S04.7) [R06 §2.4].

**Down: the brief.** A **brief** is the complete context a helper receives — structured prose with mandatory sections; helpers start with **no inherited coordinator history**, ever [R06 §4.2]. Vague briefs are the measured failure mode (duplication, misinterpretation), so every section is mandatory:

```
HELPER BRIEF (v0 contract)
objective:        one paragraph — what "done" looks like
output_contract:  report ≤ ⚙ orchestration.report_tokens; sections FINDINGS / EVIDENCE (paths, URLs) / GAPS;
                  bulk artifacts → task workspace, referenced by path
tools_sources:    allowed tools; where to look first
boundaries:       read-only unless stated; explicit "do NOT …" lines; at budget, stop and report partial
budget:           ≤ N turns, ≤ M tokens (this spawn's effective values)
context:          only the slice this helper needs — drawn from the Task Context Ledger [XREF:S05], never raw history
```

The brief is hashed at spawn (`brief_hash` in the spawn record) and checkpointed [XREF:S02].

**Up: the report.** A **helper report** is ≤ `⚙ orchestration.report_tokens = 2000` [G1 Def.8] plus filesystem artifact references: restorable compression — keep the path/URL, drop the bulk. Raw material above `⚙ orchestration.bulk_offload_tokens = 20000` goes to the task workspace as files (under a per-spawn subpath [coordinator-draft]), never into the return message [R06 §4.2]. Helpers never see the task's full history and the task never ingests a helper's full transcript — the report and its artifacts are the entire upward surface.

**Relay verbatim.** Where a helper conclusion feeds downstream consumption, the coordinator relays it verbatim — paraphrase relay is a measured degradation ("telephone game") and a regeneration cost [R06 §4.2].

**Reports are data, not instructions.** On ingestion the control plane runs an instruction-pattern screen over the report; hits annotate (mark the span as data), never reword. The screen is advisory by design — the structural defense is minimal tools and minimal context per helper, and confinement classes decide which helper classes may read untrusted content at all [R06 §4.2, §7; XREF:S11]. Verification of anything a steered helper contributed to a deliverable is S07's [XREF:S07].

**Cache-aware mechanics** [R06 §4.2]: helper templates keep stable per-template prefixes; briefs are append-only; identical-prefix fan-outs are staggered by first response when `⚙ orchestration.stagger_identical_prefix = on`, because simultaneously launched identical-prefix helpers each pay a full cache-cold cost on API-priced semantics — whether that also holds against subscription windows is exactly the P-T03-1 unknown (S04.7).

### S04.4 Budgets, caps, and the spawn log

All numbers are ⚙ operator-editable settings with the ratified value as default [G1 Def.8; G1 rider 1]; the settings registry carries the `(value, floor, ceiling)` clamps and audit trail [XREF:S18]. The scheduler MAY auto-scale a task's effective values by task complexity **only within the operator ceilings**; any per-task effective value above the ratified default (possible only where the operator has raised the ceiling) is an auto-raise and is itemized on the task receipt [G1 rider 1; XREF:S10].

- `⚙ orchestration.max_concurrent_helpers = 4` per task — counting all live helpers of the task across depths [coordinator-draft on the all-depths reading]. Field defaults of 10–25 are cloud-scale; 4 fits one laptop plus consumption pressure [R06 §4.3].
- `⚙ orchestration.helper_turns = 20` and `⚙ orchestration.helper_tokens = 80000` per helper, any depth — envelope over observed real fan-outs (45–70k tokens) [R06 §4.3; G1 Def.8].
- `⚙ orchestration.spawn_budget = 8` per task **including sub-helpers and retries** (a retry is a fresh spawn; see S04.5). Exhausting it stops further spawning; overrunning it happens only through an operator-visible gate — an approval card, never silently [R06 §4.3].

**The spawn log.** Every spawn writes a **spawn record** before the helper session starts; no code path creates a helper without one (D6: "every spawn logged with its reason"). No industry schema exists for spawn reasons — Sinet defines its own, shaped to map onto OTel `invoke_agent` spans for future export [R06 §2.5, §4.3]:

| Field | Content |
|---|---|
| `spawn_id`, `ts` | record identity, spawn time |
| `task_id`, `owner` | the task and the person billed (15.6; 3.4) — *who* |
| `parent_session` → `child_session` | spawner and spawned sessions; depth-lineage — *who* |
| `depth` | 1 or 2 |
| `trigger` | `T-CTX \| T-PAR \| T-SPEC` — *why*, machine-readable |
| `reason` | one human-readable line (7.7) — *why*, auditable |
| `brief_hash` | hash of the brief as issued |
| `model`, `lane` | what the helper runs on (selection itself is S08's [XREF:S08]) |
| `budget` | `{turns, tokens}` — this spawn's effective (post-auto-scale) values |
| `outcome` | terminal lifecycle state (S04.5): `ACCEPTED \| REJECTED \| SALVAGED \| FAILED \| ESCALATED`, written at the terminal transition |

Spawn-request refusals (depth over cap, budget exhausted, missing trigger/brief, lateral attempt) are logged as refusal events naming the failed check — refusals are evidence, and the conformance tests assert them (S04.5). Spawn records and lifecycle transitions are event-log rows in `platform.db` [XREF:S02]; receipts join helper usage through the metering ledger [XREF:S10].

### S04.5 Helper lifecycle, acceptance screen, and containment

Lifecycle — every transition a logged event [R06 §4.6]:

| State | Meaning | Exit |
|---|---|---|
| PROPOSED | coordinator (or helper, for a sub-helper) emits a spawn request: trigger + reason + brief + budget | validation → SPAWNED, or logged refusal |
| SPAWNED | control plane validated depth/budget/trigger, wrote the spawn record, started the sibling session via the adapter [XREF:S03] | → RUNNING |
| RUNNING | helper streams; checkpoints per paid call (D7) [XREF:S02] | clean report → RETURNED; budget/timeout/engine death → SALVAGED |
| RETURNED | report received; acceptance screen runs | → ACCEPTED or REJECTED |
| ACCEPTED | report integrated; verbatim relay downstream | terminal |
| REJECTED | screen failed; up to `⚙ orchestration.helper_retry_limit = 1` retry as a **fresh helper with a revised brief** (fork-don't-poison) [R06 §4.4] | retry → PROPOSED (new spawn record); second failure → FAILED or ESCALATED |
| SALVAGED | partial output carried with an **explicit unfinished marker** — never silent loss [R06 §4.4] | terminal (partial screened and consumed as partial) |
| FAILED | facet abandoned; the gap is recorded in the coordinator's own output — a dropped facet is never silent (1.4 spirit) | terminal |
| ESCALATED | unresolvable → human gate; the escalation route is test-proven, because a finding that dies in a log is a platform defect (5.6) | terminal |

Nothing exits RETURNED without an acceptance verdict — a helper report that dies unread is a platform defect [R06 §4.6].

**Acceptance screen — conformance-only at v0** [G1 Def.9 as amended at close; G3 Def.3]. Before any report is consumed, the control plane checks contract conformance: mandatory sections present, size within `⚙ orchestration.report_tokens`, declared boundaries respected, unfinished marker consistent with the exit path. It judges form, not truth. A **local-model plausibility screen** is NOT part of v0: it becomes admissible only when the local-tier battery clears the pre-registered bar on real helper outputs [G3 Def.3; XREF:S12] — and even then it annotates, never auto-kills [G1 Def.9]. Everything beyond the screen — whether helper-derived content is *correct* — is S07's two-axis verification at the deliverable level [XREF:S07].

**Sibling-failure containment (Sinet-owned).** The field's default is the opposite — fail-fast sibling cancellation or undefined behavior — so containment is explicit control-plane logic, not an engine property [R06 §4.4, §7]:

- One helper's death, derailment, budget exhaustion, or rejection NEVER cancels siblings and NEVER fails the task. Siblings run to their own outcomes; the coordinator proceeds with what it has.
- Helper death is a scheduling event, not a task failure (the D4 posture applied to helpers). A dead helper costs a resume, never spent tokens: brief-at-spawn, report-at-return, and per-paid-call usage are all checkpointed (D7) [R06 §4.5; XREF:S02].
- Derailment (loop/silence/runaway flagged by the watchdogs [XREF:S14]) follows the platform rule: pause-and-flag, never auto-kill [G1 D1.3].
- Containment and enforcement are proven, not assumed — standing conformance-suite entries [XREF:S14] MUST attempt: depth-3 recursion; a lateral send; an engine-native spawn (per lane [XREF:S03]); an over-budget spawn; and a mid-run helper kill — asserting refusal for the first four and clean containment-with-salvage for the fifth [R06 §7].

### S04.6 Artifact collisions: serialize-by-deny

Helpers hold no write claims on project artifacts (writes are coordinator-only, S04.2), but claimed-artifact collisions can still arise around a task's tree — e.g., a proposed helper whose declared reads require a claim a sibling task holds, or workspace-artifact overlap. The rule: **when parallel work would collide on claimed artifacts, the scheduler serializes it** — the claim is denied, the spawn/work queues until release, and nothing is ever overwritten. Deny-and-retry is live-probed viable at ≈ +1 turn of overhead [SPIKE P2-S4]; TBD-P3(re-confirm P2-S4 serialize-by-deny behavior on the default worker model — spike caveat). Claim semantics are S02's [XREF:S02]; queueing/priorities are S10's [XREF:S10].

### S04.7 Cost shape, attribution, and the caching unknown

- **Designed cost shape:** a coordinator + helpers run under the S04.3 contract costs ≈2–4× the same task single-agent; 15×-class multipliers are the signature of an undesigned context contract (full re-sends, paraphrase chains, cold caches) and their absence is what S04.3 exists to guarantee [R06 §2.4]. The formation rubric keeps the multiplier paid only where delegation demonstrably wins.
- **Attribution:** every helper is paid work billed to the task's requester (3.4); per-spawn model + lane + usage lands itemized on the receipt (7.7) via the metering ledger [XREF:S10]. Helper lane choice among flat-rate lanes routes by consumption pressure, never dollars (D5); mechanical helper duties default to the local lane — the permanent free tier — with selection machinery in S08 [R06 §4.5; XREF:S08].
- **The caching unknown (P-T03-1):** cache discounts are documented for API billing only; whether cached reads weigh less against *subscription* quota windows is publicly unspecified for every lane [R06 §7]. Until resolved, consumption pressure weights cache reads at 0.1×, labeled **"assumed"**, with receipts keeping raw counts [G1 Def.10; XREF:S05]. Resolution (provider watchlist [XREF:S14]) re-opens the stagger policy (S04.3) and the pressure weighting — as a settings change with audit trail, not a redesign.

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `orchestration.depth_cap` | 2 | 0–2 at v0; >2 unevidenced, requires deliberate operator ceiling change | D6; G1 Def.8 |
| `orchestration.max_concurrent_helpers` | 4 per task (all depths) | 0–ceiling; auto-scale per complexity within ceilings, auto-raises on receipts | G1 Def.8 + rider 1 |
| `orchestration.helper_turns` | 20 per helper | 1–ceiling; same auto-scale rule | G1 Def.8 + rider 1 |
| `orchestration.helper_tokens` | 80,000 per helper | auto-scale within ceilings | G1 Def.8 + rider 1 |
| `orchestration.spawn_budget` | 8 per task incl. sub-helpers + retries | overrun only via operator-visible gate | G1 Def.8; R06 §4.3 |
| `orchestration.report_tokens` | 2,000 per report | screen-enforced (S04.5) | G1 Def.8 |
| `orchestration.bulk_offload_tokens` | 20,000 | raw material above this → workspace files, path-referenced | R06 §4.2; G1 D1.1 |
| `orchestration.helper_retry_limit` | 1 | 0–ceiling | R06 §4.4; G1 D1.1 |
| `orchestration.stagger_identical_prefix` | on | on/off | R06 §4.2; G1 D1.1 |

**Known problems owned here** (ids assigned here as P-T03-1..4 for the R06 §7 problem set, per the P-T##-# convention; S17 register reconciles):

- **P-T03-1 — caching-vs-subscription-quota unknown:** mitigated by the 0.1× "assumed" weighting + raw counts on receipts [G1 Def.10]; resolution owned by the provider watchlist [XREF:S14]; re-opens stagger + weighting as settings changes (S04.7).
- **P-T03-2 — sibling-failure containment is Sinet-owned:** discharged by the S04.5 containment rules + the mandatory killed-helper conformance test.
- **P-T03-3 — no engine enforces D6:** discharged by control-plane-only enforcement (S04.1/S04.4) + violation-attempt conformance tests (S04.5); native spawning disabled per lane [G1 rider 2; XREF:S03].
- **P-T03-4 — helper-report injection surface:** report-as-data ingestion screen owned here (S04.3, advisory); the structural defense (per-helper tool restriction, confinement classes for untrusted-content helpers) is S11's [XREF:S11]; verification of steered output is S07's [XREF:S07].

**Deferred / parked:**

- R06-OQ2 engine-native micro-fanout exception → re-entry ONLY at S03 adapter-spawning review, with the explicit operator reminder [G1 rider 2] (queued in campaign state).
- Local-model plausibility screen on helper reports → re-entry when the T15 battery meets the pre-registered bar on real helper outputs [G3 Def.3; XREF:S12].
- Script-driven mass fan-out lane (≥50-item mechanical) → re-entry post-benchmark-gate (15.3) when a real workload exists [R06 §3].
- Managed-Agents-shaped first-party orchestration on a subscription lane → watchlist item; if it GAs subscription-covered, evaluate as the Anthropic-lane *mechanism* behind this same contract — the policy layer is mechanism-agnostic [R06 §4].
- Parallel-writer unfreeze → only on credible measured evidence of reliable parallel writers (currently zero) [R06 §4].

**Coverage:**

| Feature-list item | Where |
|---|---|
| D6 (topology, depth cap, no lateral, logged spawns) | S04.1, S04.4 |
| 2.4 automatic orchestration within the topology | S04.2 (triggers, formation rubric) |
| 7.7 routing accountability — spawn-reason slice | S04.4 spawn record; receipts S04.7 (worker/model choice [XREF:S08]) |
| 14.1 no chatting swarms | S04.1 (no lateral, structural) |
| 14.4 no standing army / machinery-when-earned | S04.2 (earned-helper test) |
| 3.4 / 3.6 slices — helper billing + receipt itemization | S04.7 [XREF:S10] |
| 4.6 slice — helper failure noticed, contained, surfaced | S04.5 |
| 5.6 slice — tested escalation from delegation | S04.5 (ESCALATED; conformance tests) |

**Open items for G4:** none — every load-bearing choice in this section is ratified (D6, G1 D1.1/Def.8–10/riders 1–2, G3 Def.3); the two [coordinator-draft] tags (all-depths concurrency counting; per-spawn workspace subpaths) are clarifying readings, flagged for G4 attention, not open choices.

## S05 — Context engineering & the Task Context Ledger

**Scope:** How task context is built, carried, injected, budgeted, and protected across a task's stages: the Task Context Ledger schema (finalized here per [G1 Def.12]; persistence is S02's), the fresh-context-per-stage session model, deterministic injection with a trace manifest, stage-fit budgets, cache posture, convention files, and the compaction containment stance.
**Binding inputs:** R07 (full report); [G1 D1.1] (fresh-context-per-stage on the ledger, ratified), [G1 D1.3], [G1 D1.6] (S5 PreCompact spike → P3), [G1 Def.5], [G1 Def.10], [G1 Def.11], [G1 Def.12], [G1 rider 1], [G1 P10]; [G2 D2.1] (memory package R11 §4: deterministic selection, no agent-supplied identifiers, injection budgets), [G2 D2.2] (adoption list: AGENTS.md + CLAUDE.md shim), [G2 D2.7], [G2 Def.7], [G2 Def.11]; [SPIKE P2-S5] (compiled config can be the sole config an engine sees). Boundaries: S02 owns persistence/checkpoints/fingerprint; S06 owns how spec and plan enter the ledger; S04 owns helper briefs; S09 owns memory layers, scopes, and knowledge governance; S10 owns the pressure gauge.

Terms coined here (first use): **stage brief** — the assembled context package a stage's fresh engine session starts from (ledger projection + injected slices + stage instructions + compiled worker equipment). **Trace manifest** — the run-trace record enumerating every item injected into a brief, with source, hash, version, and selector rule.

### S05.1 The Task Context Ledger — v0 schema (finalized)

The Task Context Ledger is the platform-owned, engine-agnostic context artifact of a task: exactly one per task, schema-versioned, authoritative copy under control-plane write authority in `platform.db` [XREF:S02]. Engines NEVER write it directly; stage sessions mutate it only through platform-provided ledger verbs whose per-section write rules the control plane enforces, and every accepted write is an event-log entry [XREF:S02]. The section set is ratified [G1 Def.12]; field-level structure below is finalized by this workshop, with drafting-time choices tagged [coordinator-draft].

**Ledger header** [coordinator-draft]: `{task_id, owner, ledger_version (monotonic, bumped on every accepted write), spec_version, plan_version, updated_at}`. Every stage brief and every checkpoint records the `ledger_version` it was built from.

| # | Section | Content (fields) | Writers | Mutation rule | Lifetime |
|---|---|---|---|---|---|
| 1 | `objective_ac` | Confirmed objective verbatim + numbered acceptance criteria, each in dual phrasing — plain always, structured sub-line where machine-checkable [G1 P10]; spec version ref | Control plane, from the confirmed specification [XREF:S06] | **Pinned.** Immutable; changes only by a new spec version through re-approval [XREF:S06], which bumps `spec_version` + `ledger_version` | Task; final state keep-forever in the task record [G2 Def.11] |
| 2 | `constraints_danger_zones` | Task-level constraints from spec/plan + snapshot of the registered repo's danger zones (path/action + rule + source hash) from the project registry (S1.6) | Control plane at task setup (registry snapshot) + intake pipeline (task constraints) [XREF:S06] | **Pinned.** Same rule as §1; registry drift while running is caught by the 4.3 pass (S05.6) | Task |
| 3 | `decisions` | Append-only entries `{seq, ts, stage, author: coordinator\|human\|platform, text, one-line reason, supersedes?}` [coordinator-draft fields] | Stage sessions via `ledger.decide`; control plane records human answers/approvals and platform adjustments | **Append-only.** Never edited, never summarized; a reversal is a new entry citing `supersedes` | Task; decisions are keep-forever class [G2 Def.11] |
| 4 | `state` | Work items `{id, summary, ac_refs[], status: pending\|in_progress\|done_unverified\|verified\|blocked, evidence_ref}` + `current`, ordered `next_actions[]`, `blockers[]` [coordinator-draft enum] | Stage sessions via `ledger.state`; the `verified` status is set ONLY by the control plane from a recorded verification verdict [XREF:S07] | Sessions may claim at most `done_unverified`; a session-claimed "verified" is rejected at the write layer. Verified-status discipline is ratified [G1 Def.12; R07 §4.2] | Task; final state feeds the run summary [XREF:S02] |
| 5 | `artifacts` | Restorable references `{id, ref (path/URL/object id), kind, one-line description, sha256, producing_stage}` | Stage sessions via `ledger.artifact`; hash recorded/verified by the control plane at checkpoint [XREF:S02] | **Restorable-reference rule:** bulk content is never inlined; the ref must suffice to restore the item [R07 §4.2]. Append + describe; refs are never deleted mid-task | Task; referenced bulk follows workspace/deliverable retention [XREF:S02][XREF:S13] |
| 6 | `learned_this_task` | Free-form instance notes `{note, ts, stage}` — observations, gotchas, candidate lessons | Stage sessions via `ledger.note` | Never injected into any other task; never auto-persists [G1 Def.12]. At v0 discarded after the run summary [G2 D2.7]; at v1 it becomes the evidence feed for the proposal pipeline [XREF:S09] | **Expires with the task** |

**Ledger verbs** [coordinator-draft naming]: `ledger.decide(text, reason)`, `ledger.state(item_updates)`, `ledger.artifact(ref, description, hash?)`, `ledger.note(text)` — exposed to stage sessions as a platform tool; schema-validated; writes to pinned sections and to the `verified` flag are structurally impossible from a session.

**Pinned** means: injected verbatim and whole into every stage brief and into every post-compaction re-injection; never summarized, never compacted, never paraphrased by any machinery. This is ConstraintRot's constraint-pinning result made mechanism [R07 §2.5, §4.2] — what the gates depend on must live outside any summarizer's reach.

**Stage-close gate** [R07 §4.2; coordinator-draft mechanism]: a stage cannot close until its `state` update accounts for every work item the stage was assigned (done_unverified/verified/blocked/handed-forward). Underspecified handoffs are the known failure mode of fresh-context designs; the control plane enforces handoff completeness structurally, not by convention.

### S05.2 One artifact, three duties

The ledger is a single artifact serving three ratified duties [R07 §4.2; G1 D1.1]:

1. **D7 checkpoint payload.** Every checkpoint embeds the current ledger state (by `ledger_version`); recovery forks a fresh session from the checkpointed ledger [XREF:S02]. Consequence: crash recovery and normal stage start are the *same* brief-assembly code path — recovery is not a special mode.
2. **4.3 freshness input.** The re-validation pass on stale or drifted resume compares the ledger against live reality (S05.6); triggers and the freshness fingerprint are owned by S02 [G1 Def.5; XREF:S02].
3. **Stage brief source.** The next stage's context is always built from the ledger, never carried in a transcript (S05.3).

There is no second context artifact. Engine session stores (JSONL transcripts, engine resume caches) are lane-local optimizations and never the record [G1 D1.1; R07 §5]. At task level this is 8.7's one-shared-truth: every stage and every helper derives its view from the same current ledger.

### S05.3 Fresh-context-per-stage — the session model

Every pipeline stage (intake → spec → plan → execute-N → verify, per [XREF:S06][XREF:S07]) starts a **fresh engine session** built from the stage brief; a continued or resumed transcript is never the mechanism for crossing a stage boundary [G1 D1.1; R07 §4.1]. The evidence basis is the controlled 95.1% restoration result for fresh-session-plus-consolidated-brief and the multi-turn degradation it avoids [R07 §2.2] — cited as ratified, not re-argued here. Within a stage the session is continuous.

- **Recitation.** In long stages the coordinator re-reads the ledger's `state`/`next_actions` every ⚙ `context.recitation_interval_turns` [R07 §4.2; coordinator-draft number].
- **Stage-fit budgets** [G1 Def.11]. Stages are *planned to fit*: target ≤ ⚙ `context.stage_fit_target` (default 50%) of the lane's pinned-model context window at stage start, measured by the adapter's usage accounting (D4/D3 report-usage verb [XREF:S03]). The stage brief's own footprint counts inside the budget; a brief that cannot fit the target is a plan-shape defect and raises a re-plan proposal [XREF:S06].
- **Overflow.** At ⚙ `context.stage_overflow_threshold` (default 70%) the control plane emits a `context.overflow` event that **proposes a stage split**: consolidate-to-ledger (stage-close gate) → end session → successor stage brief from the updated ledger. The split executes automatically at the next checkpoint boundary [coordinator-draft: auto-accepted default — it is risk-free platform-internal scheduling, fully logged]; a wedged session falls into the normal recovery ladder [XREF:S02]. A second overflow within one planned stage escalates to a re-plan proposal [coordinator-draft].
- **Recalibration.** Budget defaults are re-validated per model generation via the eval machinery [R07 §4.1; XREF:S14]; auto-adjustment stays within operator ceilings [G1 rider 1].
- **Lane note.** On the Z.AI lane the quota unit is prompts, not tokens; stage granularity there additionally batches work per prompt. Cross-lane normalization of consumption units is owned by the metering layer [R07 §4.5; XREF:S10].
- **Helpers.** Helper sub-sessions inside a stage follow the D6 brief-in/report-out contract unchanged; helper briefs are *derived from the ledger through that firewall* and are owned by S04 [XREF:S04].

### S05.4 Deterministic injection & the trace manifest

The control plane assembles every stage brief by **pure lookup** over platform-owned facts: the task record (domain, stakes, stage type), the project registry key, the requester's identity, and the assigned worker template + overlay. Selection is registry keys and path/glob rules — no embeddings in v0 assembly, and **no agent-supplied identifier ever selects memory** [G2 D2.1; R07 §4.3; XREF:S09]: free text or engine output never keys a lookup. An engine reading files inside its workspace is workspace access under its confinement class [XREF:S11], not memory selection; the firewall sits at the platform's injection layer.

**What a stage brief contains:**
- the ledger projection (pinned sections verbatim; `decisions`/`state`/`artifacts` current view; `learned_this_task` for this task only);
- knowledge slices per scope, within per-scope injection budgets [G2 Def.7; XREF:S09];
- repo conventions + danger zones from the registered project (S05.5);
- compiled worker equipment (template → overlay → instance compilation is S08's) [XREF:S08];
- stage instructions from the approved plan [XREF:S06].

**Assembly order is stability-sorted** — house/static → project → user overlay → task ledger → stage brief — serving cache-prefix stability and a deterministic frame; 8.9's precedence (task spec > project > personal > house) is expressed by explicit precedence labels on each block, never by ordering [R07 §4.3].

**Trace manifest.** Every injected item is logged to the run trace as `{item_id, source_path, content_hash, version, selector_rule, precedence_label}` — one manifest per assembly, plus entries for any mid-stage injection (including post-compaction re-injection, S05.7). This is S4.6/11.1 "auditable memory use" made concrete, and it is the influence-tracing substrate for reversible learning [G2 D2.1; XREF:S09]. The schema is Sinet-owned; the event-type contract it lands in is S14's [XREF:S14]. OTel GenAI export is parked (S05 Deferred).

**Injection channels per lane:** placed workspace files (native convention loading — the platform places them, so they are manifested like everything else), session prompt assembly, and the opencode `instructions` array; the Claude-lane per-stage channel choice (CLAUDE.md shim vs prompt assembly vs SessionStart `additionalContext`) is TBD-P3(S5 PreCompact/injection spike) [G1 D1.6; R07 §4.3]. The platform ensures placed/compiled config is the **only** config the engine sees — verified feasible, as a compound per-channel guarantee [SPIKE P2-S5]; the adapter conformance suite asserts it per lane [XREF:S03].

**Clean-context exception.** Verification stages receive the acceptance criteria and the deliverable diff — never the executor's overlay, history, or `learned_this_task` [R07 §4.3; XREF:S07]. The second verification axis keeps its independence by construction.

### S05.5 Convention files: AGENTS.md + the CLAUDE.md import shim

Per registered repo the canonical convention file is **AGENTS.md**, with **CLAUDE.md as a one-line import shim** (`@AGENTS.md`) for the Claude lane — an adopted mechanism on the ratified adoption list [G2 D2.2; R07 §4.4]. Both files are generated by the S1.6 repo-onboarding task; the project registry records their paths + content hashes, and they flow into stage briefs through the registry as manifested injections (S05.4).

- **Content contract** [R07 §4.4]: ≤ ⚙ `context.conventions_max_lines` (default 150); verified commands (build/test/lint with expected outputs), non-standard conventions only, danger zones, closure criteria ("what command proves done"). **No architecture overviews or directory trees** — measurably unhelpful ballast; the architecture map lives in the project registry for humans and planners, not in prompts.
- **Human-prune gate.** Generated drafts require owner approval with mandatory human pruning before adoption (unedited LLM-generated convention files are measurably net-negative) [R07 §4.4]; D10-consistent.
- **Upkeep.** Staleness is a scheduled platform concern: repo diffs touching referenced commands/paths trigger a re-validation task that lands as an update *proposal*. A **shim-drift check** (hash compare of CLAUDE.md against the canonical one-line shim) runs at every stage assembly; divergence is a platform-detected defect card, not a silent condition (P-T04-2).

### S05.6 Freshness re-validation — what the ledger contributes to 4.3

When a run resumes past the freshness threshold, on fingerprint drift, or on sibling-accept [G1 Def.5; XREF:S02], the platform re-validates the remaining plan **before spending anything significant** (4.3). The pass runs on the local tier (free, per Operating reality) and consumes the ledger as its input [R07 §4.2]:

| Ledger section | Contribution to the 4.3 pass |
|---|---|
| `objective_ac` (pinned) | What "still valid" means — the criteria the remaining plan is re-checked against |
| `constraints_danger_zones` (pinned) | Whether registry/danger-zone snapshots still match the live registry and repo |
| `state` | What work actually remains; verified items are not re-done |
| `artifacts` | Concrete refs + hashes to re-check against live reality (repo HEAD, sources) |
| `decisions` | The assumptions made en route — the list of things drift can invalidate |

Outcome: **continue / adjust-with-note / escalate** [4.3]. An adjustment is appended to `decisions` by the control plane (author: platform); an escalation is an approval card [XREF:S02 for resume mechanics].

**Context-rot ownership split** [R07 §7.8]: *within-run* rot is owned by this section's session model and stage budgets (S05.3); *across-pause* staleness by the 4.3 pass (here + S02); memory-layer hygiene by S09. No single mechanism claims to solve rot; each dimension has its named owner.

### S05.7 Compaction: an audited safety net, never the design path

Stages are budgeted so engine compaction does not fire (S05.3). It is never a planned mechanism; engine compaction internals are never rebuilt, patched, or fought (adopt-don't-fork) [R07 §4.6, §5].

**When it fires anyway, it is an anomaly, and it is audited:**
1. The control plane logs a compaction anomaly event: `{stage, lane, engine version, window fill at trigger, pinned sections at risk (in/out of post-compact context), summary artifact ref}` — "what was at risk" is a required field, not prose.
2. A **post-compaction fidelity check** runs on the local tier: verify the pinned ledger sections still govern behavior (canary-constraint method) [R07 §4.6]. Failure → pause-and-flag, never auto-kill [G1 D1.3].
3. The pinned sections are **re-injected verbatim** immediately after any compaction (on the Claude lane wired to `SessionStart(source:"compact")`), and the re-injection is manifested (S05.4).
4. Every firing counts as a **stage-design defect signal** feeding budget retuning — within operator ceilings, visible on receipts [G1 rider 1].

**Claude lane — containment stance.** The compactor is undisableable and version-unstable; thresholds are a property of the pinned engine version, never a constant [R07 §2.5]. Sinet therefore contains rather than controls: stage-fit budgets keep working sets far from the trigger; re-injection is wired regardless; whether PreCompact can *block* compaction long enough for a ledger flush is TBD-P3(S5 PreCompact/injection spike) [G1 D1.6]. Compaction behavior joins the S2.8 canary suite and is re-measured on every engine pin bump [XREF:S14][XREF:S03].

**opencode lane.** `compaction.auto: true` stays enabled with its pinned defaults as the net; the values are adapter-pinned engine config, recorded with the engine pin [XREF:S03].

**Pinned-survival is tested behavior, not an assumption** (P-T04-1): the canary-constraint → compact → adherence-check probe is a standing entry in the platform conformance suite [XREF:S14].

### S05.8 Cache posture — prefix hygiene, lane-aware weighting

The platform's job is **prefix hygiene, not cache management** [R07 §4.5]:
- stability-sorted, deterministic stage frames (S05.4);
- append-only context within a stage; injected files are never mutated mid-stage;
- one workspace cwd per task on the Claude lane (cache scopes to cwd);
- tool sets never churn mid-stage — mask, don't remove; enforced at the compiled-config layer [XREF:S03];
- `cache_read_input_tokens` / `cache_creation_input_tokens` recorded per paid call into the metering ledger and receipts (D4) [XREF:S10].

**Weighting.** Consumption pressure counts uncached input at 1.0 and cache reads at ⚙ `pressure.cache_read_weight` (default 0.1×; the setting is owned by the pressure gauge [XREF:S10]) [G1 Def.10]. Receipts always keep raw counts, and the weighting carries a visible **"assumed"** label wherever it is shown, until the provider publishes subscription quota semantics [G1 Def.10; R07 §2.8]. The pressure gauge consuming this weight, and per-lane normalization of heterogeneous consumption units (tokens vs prompts vs requests), are owned by S10 [XREF:S10]. Lane meanings differ by design [R07 §2.8]: Anthropic-subscription caching is automatic and free (probable window-headroom benefit, unverified); Z.AI-lane caching buys latency only (quota is prompt-count); metered exceptions follow published cache-aware limit rules — moot at v0 while the metered-exception list is empty [G1 P7].

**Settings introduced (⚙):** — every value operator-editable with audit trail; auto-adjust only within operator ceilings, visible on receipts [G1 rider 1]

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `context.stage_fit_target` | 0.50 of lane window | 0.20–0.70; must be < overflow threshold [coordinator-draft clamp] | [G1 Def.11] |
| `context.stage_overflow_threshold` | 0.70 of lane window | (target+0.05)–0.85 [coordinator-draft clamp] | [G1 Def.11] |
| `pressure.cache_read_weight` † | 0.1× | 0.0–1.0; label "assumed" until provider semantics publish | [G1 Def.10]; owned by [XREF:S10], listed here for the S05.8 consumer |
| `context.conventions_max_lines` | 150 | 50–400 [coordinator-draft clamp] | [R07 §4.4] |
| `context.recitation_interval_turns` | 10 | 5–50; 0 = off | [coordinator-draft] |

**Known problems owned here:**
- **P-T04-1 — compaction safety as tested behavior:** pinned-survival canary specified in S05.7; runs as a standing conformance-suite entry [XREF:S14].
- **P-T04-2 — injected-knowledge drift:** shim-drift check at every assembly + repo-diff-triggered staleness proposals (S05.5); detection scheduled, never assumed.
- **P-T04-3 — lane-heterogeneous consumption units:** owned by S10; S05 contributes lane-aware stage granularity (S05.3) [XREF:S10].
- **P-T04-4 — engine-compaction version drift:** containment stance in S05.7; behavior re-measured per engine pin via the canary suite [XREF:S14][XREF:S03].
- **Context rot (feature-list known problem):** within-run dimension owned here (S05.3 session model + budgets); across-pause by S05.6 + S02; memory hygiene by S09 [R07 §7.8].
*(P-T04 ids assigned here in R07 §7.7 order, matching the P-T##-N convention; S17 consolidates.)*

**Deferred / parked:**
- ~~PreCompact blocking + Claude-lane per-stage injection channel → TBD-P3(S5 PreCompact/injection spike)~~ — **CLOSED A4 (2026-07-23, B4 gate)**: spike PASS at P3-B1-4 (result file `P3/measurements/2026-07-20-precompact-injection-mechanics.md`); containment stays the primary stance.
- Trace-manifest export to OTel GenAI semconv → re-entry: content-capture semantics stabilize (watchlist row) [R07 §4.3].
- Retiring the CLAUDE.md shim → re-entry: Claude engine reads AGENTS.md natively (engine watchlist) [R07 §4.7].
- Removing the "assumed" cache-weight label → re-entry: provider publishes subscription quota semantics; ⚙ re-ratified then [G1 Def.10].
- Embedding-ranked candidate selection for injection → parked behind S09's vector post-gate; embeddings only ever rank candidates for trace-manifested selection [G2 Def.8; XREF:S09].
- Engine-native cross-session memory absorbing ledger duties → re-entry: config-only availability on *both* lanes, evaluated behind the Sinet contract [R07 §4.7]; Claude-lane auto-memory containment spike rides adapter work [XREF:S03].
- Relaxing stage lengths → re-entry: a controlled fresh-vs-continuous head-to-head at equal budget showing parity [R07 §4.7], via S14 evals.

**Coverage:**

| Feature-list item | Where |
|---|---|
| 8.4 / S4.6 — right knowledge arrives by itself (assembly side; storage is S09's) | S05.4 |
| S4.6 / 11.1 / S2.1 — auditable memory use, injected-context trace | S05.4 (trace manifest) |
| 4.3 — freshness re-validation (ledger contribution; mechanics S02) | S05.6 |
| 4.8 / D7 — checkpoint context payload | S05.2 [XREF:S02] |
| S1.6 — repo conventions/danger-zones capture + upkeep | S05.5 |
| S4.2 / S4.3 — defined writers and lifetimes for task-scoped context | S05.1 |
| 8.7 — one shared task truth | S05.2 |
| 8.9 / S4.11 — declared precedence in assembled context | S05.4 |
| Known problem "context rot" | S05.3, S05.6, S05.7 |
| 3.1/D4-adjacent — cache telemetry into receipts; D5 weighting input | S05.8 [XREF:S10] |

**Open items for G4:** None. Drafting-time sub-choices are carried inline as [coordinator-draft] tags (ledger field structures and verb names, status enum, overflow auto-accept + second-overflow re-plan rule, recitation default, clamp ranges) — each is the option directly implied by ratified decisions, listed here for G4 attention rather than left open.

## S06 — Intake: interview → specification → plan

**Scope:** Everything between "a request arrives" and "an approved plan exists": triage, the clarity-driven interview, the specification-with-numbered-AC contract, the plan artifact, the deterministic spine, plan self-attack, and the approval/freshness contract — with ceremony scaled by stakes.
**Binding inputs:** R03 (T05, whole report; §4 is the ratified pipeline shape per G1 findings digest + D1.1); G1 P8/P10/P11, D1.1/D1.3/D1.7, Def.5/Def.11/Def.12, riders 1–3; G2 Def.2/Def.3, D2.7; G3 T15 digest (ceremony cut line), D3.4 (compose-when-earned boundary); feature list 1.1–1.10, 2.3, 2.5, 4.3, S1.6, S3.2, 13.5; Nexus traps P45/P46/P47 as carried in R03 §1.

### S06.1 Pipeline shape and stage boundaries

Every task passes through five stages. Only two touch a paid model; the rest are platform code and local duty aliases [R03 §4; G1 D1.1].

| # | Stage | Runs on | Confinement | Emits → Task Context Ledger [XREF:S05] | Boundary |
|---|---|---|---|---|---|
| 0 | Triage | local duty aliases + deterministic rules | control plane (no task sandbox) | intake record (family, stakes tier + floor reasons, size/cost guess, data-bearing flags, registry-slice ref) → artifacts + decisions | automatic |
| 1 | Interview → SPEC → PLAN | planning model (S06.10); utility model for phrasing duties | C1 read-only sandbox, helper-inherited (S06.6) | SPEC + PLAN artifact pair, status `draft` → artifact refs | D7 checkpoint |
| 2 | Deterministic spine | platform code — no model | — | spine findings (coverage table, floor re-check, size delta, research-node presence) → decisions | automatic; may bounce once to Stage 1 |
| 3 | Critique (self-attack) | planning-model class, fresh context | artifact-only input | verdict (PASS / REVISE / SPEC-DOUBT / TIER-UP) + numbered findings → decisions + artifacts | verdict-routed |
| 4 | Approval | the requester [D10] | — | approval record (who/when/card version/choices) → decisions; objective + frozen ACs + constraints/danger zones pinned [G1 Def.12] | human gate — waits |

- Every stage boundary is a D7 checkpoint; an interrupted intake resumes from its last artifact, never by replaying conversation [D7; XREF:S02].
- **No stage ever receives another stage's transcript.** Inter-stage context is the artifact pair plus ledger slices; this kills the measured multi-stage token-multiplication trap structurally, not by tuning [R03 §3-C, §4 "Ceremony economics"; P45-adjacent Nexus 3.5×-spec-stage evidence per R03 §1]. Stage-fit context budgets apply [G1 Def.11; XREF:S05].
- **Gates wait.** No intake gate auto-proceeds on a timer, at any tier [R03 §5 (timed soft gates rejected); D7; D10]. Answering a card resumes the pipeline in place (4.3).
- The restate-and-confirm loop of 1.3 is realized as: SPEC restatement on the approval card, iterated through **Re-plan** / **Re-interview** actions until the requester approves; approval is the confirmation (S06.9).
- After approval, worker/model/lane selection for execution is S08's [XREF:S08]; verification against the frozen ACs is S07's [XREF:S07]; card rendering is S15's [XREF:S15]. This section owns pipeline logic and artifact contracts.

### S06.2 Stage 0 — Triage

On every request, before any paid model runs [R03 §4 Stage 0]:

1. **Task-family detection** (2.3): software / research / content / data / chore, by local duty alias [XREF:S12]. v0 ships the software family plus a generic fallback set; further family question sets arrive with their domains (S00.2 scope) [R03 §4; XREF:S19].
2. **Registry match** (S1.6): the matched project/repository registry entry — conventions, commands, danger zones — is injected so the interview never asks what the platform already knows; danger zones feed the stakes floor [R03 §4 Stage 0; registry home XREF:S13].
3. **Stakes + size guess** (1.8, 2.5): a local classifier proposes a stakes tier (S06.4) and a size/cost estimate in API-equivalent USD (D5 reporting currency; price table [XREF:S10]).
4. **Data-bearing detection** (1.9/P47): the deterministic trigger list of S06.3 first; a local classifier second, which may only *add* the flag, never remove it [R03 §2.7, §4].

**Deterministic floors override everything upward.** Any of: a planned outward effect, new spend, credential touch, shared-asset write, or a named regulated domain forces the high tier regardless of classifier output and regardless of the requester's convenience settings (1.8) [R03 §2.6 (OWASP rule-floor pattern), §4 Stage 0]. Classifier failure fails closed: the task is treated as high-stakes [R03 §2.6]. The requester's own accept-time commit of a reviewed deliverable (6.3, D9) is the requester's action, not a task-issued outward effect, and does not by itself trip the floor. Floors size *ceremony only* — safety never depends on them, because D7 gates every outward effect as a proposal no matter what intake decided [D7; R03 §2.6].

Misclassification is cheap by construction: underestimates are caught by the Stage-2 size-delta rule before expensive work; overestimates are caught by the preview and the run ceilings (2.5, 3.7) [R03 §4 Stage 0]. At run end, actual consumption is recorded against the Stage-0 estimate; classifier calibration is owned by the eval practice [P-T05-4; XREF:S14].

### S06.3 The P47 research-trigger list `[coordinator-draft]` (discharges R03-OQ5)

Tasks whose output depends on facts in the world always include live research as a required step (1.9); the trigger layer is deterministic rules first, classifier second [R03 §2.7, §4; G1 follow-ups: R03-OQ5 assigned to coordinator at P2]. The v1 rule file ships as an operator-editable, versioned platform file (`intake/p47-triggers`, git-tracked like knowledge objects [XREF:S09]); seed content:

| Id | Trigger class | Cue examples (non-exhaustive) |
|---|---|---|
| P47-1 | Prices & costs | currency amounts; price/fee/tariff/subscription/tier/"cheapest" |
| P47-2 | Products & vendors | named commercial products, models, SKUs, services; availability; "best X"/comparisons |
| P47-3 | Laws, regulation & terms | legal/tax/licensing/compliance references; provider ToS |
| P47-4 | Technical interfaces & versions | API/SDK/library/package names whose version, capability, or deprecation status the work depends on; model capabilities |
| P47-5 | Schedules & external events | dates, timetables, opening hours, deadlines not set by the requester |
| P47-6 | Churn-prone named entities | organizations, people-in-role, live services (fast-changing class per R03 §2.7 freshness typology) |
| P47-7 | Temporal cues | "current", "latest", "today", "now", "recent", "this year" |
| P47-8 | Locality cues | "near me"; regional availability, shipping, or pricing |
| P47-9 | Explicit lookup requests | "look up", "check online", "verify", "find out" |
| P47-10 | Prior art & external corpora | "existing solutions", "competitors", "state of the art" |
| P47-11 | Requester-asserted external facts | the request states a checkable claim about the world that the plan depends on → verify node at standard tier and above (false-premise guard) |

Semantics of a hit:
- The **data-bearing flag** is set on the intake record; it excludes the task from the zero-interaction band (S06.4) and obliges research nodes in the PLAN (S06.6).
- A hit is dismissible **only by the requester** explicitly supplying or owning the fact, recorded on the SPEC as a requester-supplied input — never by a model [1.9; R03 §5 "model-initiative research" rejected].
- Maintenance: v0 additions are manual operator edits to the rule file; miss-driven auto-proposals enter through the 8.1 lesson gate only when the proposal pipeline activates at v1 [G2 D2.7]. Classifier evaluation belongs to the eval practice [XREF:S14].

### S06.4 Stakes tiers and the zero-interaction band

Planning effort scales with stakes automatically, regardless of the requester's convenience settings (1.8). Four tiers; automatic adjustments are monotone upward within a task (TIER-UP, floors, size-delta); lowering happens only by explicit requester action and never below a floor [R03 §4].

| Tier | Interview | Critique pass | Approval |
|---|---|---|---|
| **Trivial** (= the zero-interaction band) | none — unresolved slots become auto-listed assumptions | none | no blocking gate; completion card only |
| **Low** | only while Clearance < ⚙ `intake.clearance_floor.low` | none — spine only `[coordinator-draft]` (R03 §4 Stage 3 mandates critique from standard up) | one-tap card; inbox risk tier Low, batchable [S3.2] |
| **Standard** | while Clearance < ⚙ `intake.clearance_floor.standard` | mandatory, one pass | full two-layer card; risk tier Medium [S3.2] |
| **High** | while Clearance < ⚙ `intake.clearance_floor.high`; uncapped, batched | mandatory, one pass | full card; risk tier High — never batchable, individually approved [S3.2] |

**The zero-interaction band** [G1 P11]: a task is Trivial if and only if all four conditions hold —

1. **read-only** — the plan's declared write-set outside the run workspace is empty and no outward-effect proposals are expected;
2. **no data-bearing flag** (S06.3);
3. **estimated cost** < ⚙ `intake.zero_interaction_cost_usd` (API-equivalent, D5 currency) [G1 D1.7];
4. **no new tools, workers, grants, or automations** — existing validated workers with existing grants only [XREF:S08; G3 D3.4 boundary].

Conditions are evaluated conservatively at Stage 0 and re-verified deterministically at Stage 2 from the PLAN's actual contents; failing any re-check ejects the task from the band, and band exit is one-way for that task [R03 §4 Stage 2(b)]. Inside the band the pipeline still runs — auto-generated SPEC (restatement + ACs + listed assumptions) and a degenerate PLAN exist in the ledger, ungated — and completion delivers one non-blocking card carrying restatement, assumptions, deliverable, and receipt. Trivial tasks never require a confirmation click [1.8, 2.5]. The band cannot leak: membership is rule-decided, floors override upward, and D7 gates any outward effect even on a floor miss [R03 §2.6, §4 Stage 0].

### S06.5 Stage 1 — the clarity-driven interview and the Clearance indicator

The platform refuses to run on vagueness (1.2): below its tier's Clearance floor a task is either interviewed or explicitly force-proceeded by the requester with the vagueness converted into visible, approval-gated assumptions — never silently executed.

- **Question sets are per kind-of-task** (1.2): each family carries a versioned must-know taxonomy (software seed: the ClarifyCodeBench 10-type taxonomy; the generic fallback set covers unmatched requests) [R03 §2.3, §4 Stage 1]. Taxonomies are knowledge objects: drafted at implementation time with the strongest available frontier model, entering through the 8.3 knowledge gate — versioned, attributable, removable [5.7 pattern; XREF:S09]. TBD-P3(seed interview-taxonomy drafting session, 8.3-gated).
- **Clearance (0–100%)** is computed deterministically from the active question set: each must-know slot carries a weight (shipped in the taxonomy file, operator-editable); a slot is *resolved* when registry-supplied, requester-answered, or converted to an explicit assumption; Clearance = 100 × resolved weight / total weight [G1 P8]. It is displayed live during intake, rises as the requester answers, and appears on the approval card.
- **There are no fixed question caps, per tier or otherwise** [G1 P8 — fixed per-tier caps rejected]. The interview continues while Clearance is below the tier floor and unresolved slots remain; it stops when the floor is reached, slots are exhausted, or the requester force-proceeds. Force-proceeding converts every unresolved must-know slot into a listed assumption that lands on the approval card's centerpiece (S06.9).
- **Question delivery**: batched option cards — up to 4 questions per card, each with 2–4 labeled options plus free text — highest-weight unresolved slots first [R03 §2.7, §4 Stage 1; card component and transport XREF:S15, XREF:S03]. Unanswered interview cards follow the platform ask SLAs and expiry [G1 Def.3 superseded set; G2 Def.2; XREF:S02].
- **Ask-don't-assume (1.7) as one-question escalations**: outside the interview — during planning, critique, or execution — a *consequential* ambiguity raises exactly one clarifying question as a single-question card; the run blocks-not-fails and resumes on answer (4.3). Consequential = would change an AC, the declared write-set, an outward effect, the stakes tier, or the cost estimate beyond the S06.7 size-delta factor. Non-consequential ambiguities become logged assumptions, visible on the card and in the ledger [R03 §4 Stage 1].
- The interview runs inside the Stage-1 planning session on the planning model; the utility model phrases and summarizes but does not decide what must be asked — the taxonomy carries the coverage [R03 §2.3; G3 T15 digest].

### S06.6 The artifact pair: SPEC and PLAN

Stage 1 emits one artifact pair — Sinet-owned markdown + YAML frontmatter (ids, owner, stakes tier, model/version provenance, status), stable keys, git-committable [D9], checkpointed at every stage boundary [D7], referenced from the ledger [XREF:S05; R03 §4 "Artifact format"].

**SPEC — the contract all later work is measured against (1.3):**
- Plain-language goal restatement in the requester's terms.
- **Numbered acceptance criteria `AC-1..n`** on stable keys: atomic (one behavior each), binary-decidable, measurable, free of vague adjectives [R03 §2.2 anti-ambiguity idioms, §4].
- **Dual phrasing** [G1 P10]: every AC has a plain-language line, always; a structured machine-checkable sub-line (EARS phrasing for behavioral criteria, GWT example scenarios where examples communicate better) exists **only where the criterion is machine-checkable**. Where a structured sub-line exists, verification binds to it; otherwise verification judges the plain line [XREF:S07]. The requester is never required to read notation.
- Technology-agnostic outcome criteria (SC-style) so a non-expert can recognize their goal [R03 §4].
- An explicit **assumptions list**; an explicit **out-of-scope / "will NOT do" list**; requester-supplied inputs (S06.3).
- Unresolved points carry `NEEDS-CLARIFICATION` markers; **an artifact with open markers cannot reach approval** — each marker is either asked (S06.5) or converted to a listed assumption [R03 §4 Stage 1].

**PLAN:**
- Numbered steps, each with a per-step **"Done when"** stage contract, consumed by verification [R03 §4; harvest S3 pattern; XREF:S07].
- The **AC coverage map**: every `AC-k` → owning step id(s). This is the 1.4 traceability substrate (S06.7).
- **Research nodes** for every data-bearing flag — present by policy before any model drafted the plan, never left to model initiative [1.9/P47; R03 §4]. At v0 research nodes execute via provider-side search tools on lanes that offer them — task sandboxes get no egress (C3 confined-fetch arrives v0.1 [XREF:S11; XREF:S19]); routing to a search-capable lane is S08's [XREF:S08]; the runtime did-research-actually-run check via usage counters, with retry-then-escalate, is verification's [R03 §4 Stage 2(d); XREF:S07].
- **Confinement declaration**: each step/worker declares its confinement class (C0–C2 at v0) [XREF:S11]. **The class declared at plan time flows to every helper**: the planning stage itself runs at C1 with a read-only project snapshot, enforced at sandbox level — never prompt level — and inherited by any helper the planner spawns; at execution every helper inherits its declaring step's class or tighter, never wider. Widening confinement is a plan change: delta re-plan + re-approval [P-T05-1; R03 §2.1 (prompt-level lockouts demonstrably fail); XREF:S11 mechanics, XREF:S04 spawn mechanics].
- **Write-set declaration** per step; a plan that cannot bound its write-set takes the whole-project claim [G2 Def.3; claim/parallelism mechanics XREF:S02].
- Risk notes, and a size/cost estimate explicitly compared to the Stage-0 guess (2.5) — surfaced, never silent [R03 §4].
- The plan declares *requirements* (class, tools, family); selecting which worker, model, and lane executes it is S08's [XREF:S08].

### S06.7 Stage 2 — the deterministic spine (platform code, no model)

Runs after every Stage-1 emission and after every revision [R03 §4 Stage 2]:

(a) **Coverage check (1.4).** Keyed AC↔step cross-reference on stable keys. Any uncovered AC triggers at most ⚙ `intake.coverage_autofix_rounds` bounded auto-fix round(s) with the planner, then a decision card to the requester — an agreed criterion never disappears silently, at intake or ever after: any later AC removal is a REMOVED delta requiring re-approval (S06.9). Drop-detection is deterministic on keys; a local-model semantic spot-check ("does step S plausibly deliver AC-3?") runs advisory-only [R03 §4, §5 (LLM-only coverage rejected)]. This escalation route is covered by an end-to-end test that forces it [P46; 5.6].
(b) **Floor re-check against the plan's contents.** If the PLAN introduces outward effects, spend, credential touches, shared-asset writes, or new tools/workers the request didn't mention, the tier rises now, and zero-interaction band membership is re-decided (S06.4) [R03 §4 Stage 2(b)].
(c) **Size-delta rule (2.5).** Plan-derived estimate > Stage-0 guess × ⚙ `intake.size_recheck_factor` → reclassify size, re-run tier assignment, and upgrade ceremony *before* expensive work; plan ≪ guess → noted on the approval card. Display is stakes-gated: non-trivial tasks show the classification on the card for verify-or-change; trivial tasks never require a confirmation click [2.5; 1.8].
(d) **Research-node presence check** for every data-bearing flag; a missing node is a defect that bounces the plan once, then cards [R03 §4 Stage 2(d)].

### S06.8 Stage 3 — plan self-attack (1.5)

A separate fresh-context session that reads **only the artifact pair** — never the interview transcript; context separation is the measured active ingredient and keeps the pass at artifact-sized cost [R03 §2.5, §4 Stage 3]. Explicit devil's-advocate role, per-family rubric, premortem frame: "assume this failed — why?" [1.5; R03 §2.5]. Runs at planning-model class; **local models never hold this duty** — small-model critique capitulates [R03 §2.5, §5; G3 T15 digest]. Mandatory from the standard tier up (S06.4).

Four terminal verdicts, each written to the ledger:
- **PASS** → Stage 4.
- **REVISE** — numbered blocker findings; the planner fixes within ⚙ `intake.critique_revise_rounds` round(s); re-critique checks only the named findings; criteria do not drift. One pass, never loops — critique is a filter, not a guarantee [R03 §4 Stage 3 calibration honesty].
- **SPEC-DOUBT** — the P45 antidote: the critique concludes the *specification itself* is a bad idea and says why in plain language → a mandatory decision card to the requester (**proceed anyway / adjust spec / rethink**). Never absorbed, never softened into a note; the route is covered by an end-to-end test that forces it [1.5; P45/P46; 5.6; R03 §2.5 (no product wires this — Sinet-built), §4 Stage 3].
- **TIER-UP** — the plan reveals stakes the classifier missed; the tier rises and the pipeline re-enters at the new tier's requirements [R03 §4 Stage 3].

### S06.9 Stage 4 — approval, freshness, and deltas (1.6)

The cheap preview before expensive execution: reviewing a plan costs seconds; a wrong execution burns hours of quota (1.6). The requester approves their own spec and plan [D10]; nothing self-approves (5.8).

**Card content contract** (rendering XREF:S15; inbox mechanics S3.2):
- **Layer 1 — one phone screen**: what I understood (restatement); what you'll get; what I'll do (numbered plain steps); **what I will NOT do**; **the assumptions list as the centerpiece** — the one RCT-evidenced overreliance mitigation; no additional forcing functions are stacked (stacking measured detrimental) [R03 §2.4]; what could go wrong + expected cost/time + Clearance + size class; and the 13.5 help block: what this decision does, what could go wrong, what the platform recommends and why — drafted by the utility model from the artifacts at non-IT reading level [13.5; R03 §2.7 (white space — Sinet-built)].
- **Layer 2 — expandable**: full numbered ACs (both phrasings), plan with coverage map, critique verdict and findings, research findings, estimate detail.
- **Actions**: **Approve** · **Re-plan** — structured entry: tap the AC, assumption, or step being contested; produces a bounded delta re-plan [S3.2: plans always carry both Approve and Re-plan] · **Re-interview** — returns to Stage 1 with artifacts intact. Cancel is always available (4.5).
- Approval UX is tuned by measured outcomes through the benchmark/eval practice, never by user preference [R03 §5; XREF:S14].

**On Approve**: the ACs freeze under their numbers — later verification and re-review judge against exactly these [5.4; XREF:S07]; objective, frozen ACs, and constraints/danger zones are pinned into the ledger [G1 Def.12; XREF:S05]; the spec+plan version joins the freshness fingerprint [G1 Def.5; XREF:S02].

**Freshness of pending approvals**: a card pending past ⚙ `intake.approval_stale_hours`, or whose freshness fingerprint drifts (repo HEAD, source hashes, spec+plan version, price table), or whose project accepted a sibling task, is auto-flagged **"assumptions may be stale"** with a one-click re-plan [S3.2; 4.3; G1 Def.5; fingerprint mechanics XREF:S02]. The flag never blocks the requester from approving; it makes staleness visible. Answering resumes the pipeline in place (4.3); unanswered asks expire per platform policy [G2 Def.2; XREF:S02].

**Delta re-approval**: every post-approval change to SPEC or PLAN — freshness re-validation findings (4.3), sibling-collision re-plans (S1.11), contested-card fixes, confinement widening — is expressed as an ADDED / MODIFIED / REMOVED delta against the frozen artifacts and approved on a **delta-only card** showing exactly what changed; a silently disappearing criterion is structurally impossible [1.4; R03 §2.2 (OpenSpec pattern), §4 Stage 4]. Delta cards carry rubber-stamp risk with no field evidence either way (P-T05-2), so each one ships the **measurement hook**: the event log records presented-delta size, time-to-decision, decision, and outcome linkage; the eval practice owns the analysis, and measured rubber-stamping proposes a card-format retune [P-T05-2; XREF:S14].

### S06.10 Ceremony engines and billing (1.10)

Ceremony has a designated engine per duty. 1.10's designated-engine requirement is satisfied by a per-person **ceremony duty map** with a uniform recommended default and per-user override — best result per unit of allowance, not merely cheapest [1.10]. The v0 cut line keeps interviewing, plan critique, and verification review on paid frontier-class models; light duties ride the local free tier [G1 P9; G3 T15 digest; R03 §2.6].

New term — **planning model**: the requester's designated frontier-class model for the Stage-1 planning session and the Stage-3 critique pass; recommended platform-wide like the utility model, user-overridable [1.10; R03 §4 Stage 1]. Distinct from execution routing [XREF:S08].

| Duty | Engine | Provenance |
|---|---|---|
| Family / stakes / size classification; data-bearing classifier | local duty aliases (free tier) [XREF:S12] | R03 §2.6; G1 P9 |
| Interview conduct, restatement, SPEC/PLAN drafting | planning model | R03 §4 Stage 1; G3 T15 digest |
| Question/card phrasing, 13.5 help drafting, summaries | utility model (local, ≤8B at v0) | 1.10; G1 P9 |
| Coverage / floor / size / research-presence checks | platform code — no model | R03 §4 Stage 2 |
| Advisory semantic coverage spot-check | local duty alias | R03 §4 Stage 2(a) |
| Critique / premortem / SPEC-DOUBT | planning-model class, fresh context — never local | R03 §2.5; G3 T15 digest |

Every ceremony consumption is billed to the requester and itemized separately from execution on the receipt [1.10; 3.4; 3.6; XREF:S10]. Local-tier duties consume no allowance and appear as zero-allowance receipt lines [Operating reality; XREF:S12]. The structural cost property: at most two heavyweight paid passes per intake (planning session + critique) plus bounded single revisions; everything else is local or deterministic [R03 §4 "Ceremony economics"] — which is what lets the 11.2 benchmark price intake honestly against direct use [XREF:S14].

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `intake.zero_interaction_cost_usd` | 0.50 | ≥ 0; per-user | G1 P11 + D1.7 |
| `intake.clearance_floor.low` | 60 | 0–100 | G1 P8 mechanism; default `[coordinator-draft]` |
| `intake.clearance_floor.standard` | 75 | 0–100 | G1 P8 mechanism; default `[coordinator-draft]` |
| `intake.clearance_floor.high` | 90 | 0–100 | G1 P8 mechanism; default `[coordinator-draft]` |
| `intake.size_recheck_factor` | 2.0 | > 1.0 | 2.5; R03 §4 Stage 2(c); default `[coordinator-draft]` |
| `intake.approval_stale_hours` | 24 | > 0 | 4.3 default; G1 Def.5 |
| `intake.coverage_autofix_rounds` | 1 | 0–2 | R03 §4 Stage 2(a) ("bounded") |
| `intake.critique_revise_rounds` | 1 | 0–2 | R03 §4 Stage 3 ("fixes once") |

All per G1 rider 1: operator-editable, audit-trailed, auto-adjustment only within operator ceilings, visible on receipts.

**Known problems owned here:**
- **P-T05-1** — plan-stage confinement must bind helpers: dispositioned in S06.6 (sandbox-level read-only planning inherited by planning helpers; declared class flows to every execution helper, tighter-only); enforcement mechanics [XREF:S11].
- **P-T05-2** — delta re-approval rubber-stamp risk: dispositioned in S06.9 (delta-only cards + mandatory measurement hook); analysis owned by the eval practice [XREF:S14].
- **P-T05-3** — clarification as an injection surface: v0 intake surface is the authenticated workspace only; quoted/pasted material in requests is treated as data, never instructions (4.7); full C3-class treatment of channel-ingested content parks with 15.5 multi-channel ingress, jointly owned with [XREF:S11].
- **P-T05-4** — no published stakes-classification accuracy: tolerated by design (deterministic floors + cheap wrongness, S06.2); a pre-registered classifier eval is required before v1 household use [XREF:S14].

**Deferred / parked:**
- High-stakes comprehension friction (embedded-real-choice card, R03-OQ6) → operator decision at the v1 household pilot [G1 follow-ups].
- Channel-ingress intake hardening (P-T05-3 full treatment) → 15.5 multi-channel ingress.
- Per-family question sets beyond software + generic → arrive with each domain (research at v0.1; degraded domains at v1).
- Miss-driven auto-proposal of P47 rule additions → v1 memory proposal pipeline [G2 D2.7]; v0 is manual operator edits.
- Structured sub-line format winner (EARS vs GWT) → adopt if a real format A/B appears [R03 §4 change-trigger 2]; both admissible at v0 under G1 P10.
- Local-model critique duty → re-test when a T15-class eval shows non-capitulating critique in the 24 GB class [R03 §4 change-trigger 3].
- Zero-interaction band widening → 11.2 evidence that intake overhead goes unpaid on Eco-tier tasks [R03 §4 change-trigger 5].

**Coverage:**

| Feature-list item | Subsection |
|---|---|
| 1.1 natural-language intake | S06.1, S06.5 |
| 1.2 refuses vagueness / kind-of-task questions | S06.5 |
| 1.3 restate-confirm, numbered-AC contract | S06.6, S06.9 |
| 1.4 criterion traceability, no silent drops | S06.6, S06.7(a), S06.9 (deltas) |
| 1.5 plan self-attack + spec-is-bad escalation | S06.8 |
| 1.6 cheap preview | S06.9 |
| 1.7 ask-don't-assume | S06.5 |
| 1.8 stakes-scaled effort, zero-interaction band | S06.4 |
| 1.9 mandatory live research (P47) | S06.3, S06.6 |
| 1.10 ceremony engine + billing | S06.10 |
| 2.3 task-type detection | S06.2 |
| 2.5 size-guess self-correction | S06.2, S06.7(c), S06.9 |
| 4.3 (approval-freshness slice) | S06.9 |
| S1.6 (registry-fed intake slice) | S06.2 |
| S3.2 (plan-card slice: Approve+Re-plan, stale flag) | S06.9 |
| 13.5 (card content contract; rendering S15) | S06.9 |

**Open items for G4:** none. Coordinator-drafted sub-choices are tagged inline for G4 attention: the P47 trigger table (S06.3), Clearance floor defaults, the low-tier critique skip (S06.4), and the size-recheck factor.

## S07 — Verification & quality

**Scope:** Everything between "execution produced a deliverable revision" and "the requester sees it": the two-axis, four-layer verification pipeline on every deliverable, judge routing and independence, bounded rework with carried findings, typed and test-proven escalation with the ratified SLA set, the verification-practice failure modes as first-class problems, and rubric/judge governance.
**Binding inputs:** R04 (whole report; §4 is the ratified pipeline per G1 D1.1(d)) · G1 D1.1/D1.2, P10, Def.1, Def.2, Def.3 as superseded at close, rider 1; D1.3 + Def.9 as boundaries · G2 D2.1(d) (event-type contract), Def.2, Def.11 · G3 Def.3 (boundary), Def.4, Def.8, T15 digest (ceremony cut line) · feature list 5.1–5.8; slices of 1.9, 2.1/7.6, 3.4/3.6, 4.7/S5.4/S5.6, S2.3, 11.2 · siblings: S04 (acceptance-screen boundary), S05 (clean-context exception), S06 (frozen ACs, dual phrasing, research nodes, SPEC-DOUBT) · `Spec/benchmark-preregistration-v1.md` (cited, never restated).

### S07.1 Shape: two axes, four V-layers, every deliverable

**Nothing is delivered unverified (5.1) — 100% of deliverables, never sampled.** Industry samples its quality gates for volume economics; Sinet's delivery gate covers every deliverable, and sampling exists only inside the meta-eval and benchmark practice (11.2) [R04 §2.9; XREF:S14].

Verification asks **two independent questions** of every deliverable [5.2; G1 D1.1(d)]:

- **Axis 1 — spec compliance:** does it meet the frozen numbered ACs (S06.6)? Judged criterion by criterion, binding to the structured sub-line where one exists [G1 P10].
- **Axis 2 — outcome sanity:** is it actually good? A separately prompted judgment (S07.5) — formal compliance and a bad product can coexist, and an axis-2 finding can reopen the specification itself as an explicit human decision (5.2), never as a model's edit.

New term — **V-layer**: one of the four verification layers below ("V" prefix avoids collision with the memory layers L0–L2 [S00.7]). The cascade is cheap-first (5.3): each layer is strong exactly where the next is weak, and nothing reaches paid judging that a free check could kill [R04 §2.3, §4].

| V-layer | What runs | Runs on | Allowance cost |
|---|---|---|---|
| **V0** pre-gates | deterministic structural checks | platform code | zero |
| **V1** domain checks | executable/mechanical checks per domain + plan-conformance checks | local free tier, task sandbox | zero |
| **V2** judge pass | one dual-axis model review | paid flat-rate lane, planning-model class (S07.5) | the only scarce verification spend |
| **V3** human gate | the requester accepts or rejects (5.8) | human, via the approval inbox | — |

Effort scales with survivors and stakes (5.3): V0/V1 kill broken output before any paid call; axis 2 is stakes-gated outside launch domains (S07.8); entailment coverage splits mandatory/sampled (S07.4); rework is round-capped (S07.6). **The quality gate is never the effects gate:** sequence is quality verdict (V0–V2) → human accept (V3) → gated effect execution under D7 — verdicts release nothing outward, and the judge is never an implicit release authority [R04 §6-G1; D7; D10].

**Boundary lines.** S04 owns the helper acceptance screen — form-conformance on helper reports, never content truth [S04.5; G1 Def.9; G3 Def.3]; helper-derived content is verified here at deliverable level. S06 owns intake-side critique — SPEC-DOUBT is the pre-execution spec challenge; REOPEN-SPEC (S07.5) is its deliverable-time counterpart. S13 owns deliverable/revision/comment mechanics including anchor storage and re-anchoring. S14 owns eval/benchmark machinery, watchdogs, and canary scheduling infrastructure. S08 owns worker/judge model selection mechanics. S11 owns the sandbox mechanics verification relies on.

### S07.2 V0 — deterministic pre-gates (free, every domain, always)

Empty, truncated, placeholder-marker, diff-empty, and schema-invalid output dies instantly at zero cost, plus artifact-shape checks per deliverable type (5.3). This layer is load-bearing, not hygiene: superficial/truncated/placeholder output is precisely the input class model judges most reliably rubber-stamp (master-key false-positive rates to 80%), so the free gate removes exactly what the paid gate cannot be trusted with [R04 §2.1, §2.3, §4].

- A V0 **verdict** of "artifact malformed" is a hard kill: it costs one regeneration, never a judge call.
- A V0 **tool failure** (the gate crashed, not the artifact) fails open to V1 with a logged incident: a screen never blocks on its own failure, a screen outage escalates rather than approves, and no screen's SHIP is ever final [R04 §2.3, §5].

### S07.3 V1 — domain check packs and plan-conformance checks

**Code (v0 launch domain).** The executable ladder, run per revision in the task sandbox with evidence artifacts retained: lint + typecheck + build → unit/integration tests → runtime smoke → drive-the-feature e2e [5.1; R04 §2.4, §4]. Hardened execution rules, each a **MUST** [R04 §4]:

1. **Network off by default; answer-bearing VCS history stripped** from the verification workspace (P-T06-2) — otherwise the pass signal measures lookup ability, not work (63% of one frontier model's benchmark successes were retrieved, not derived [R04 §2.4]). Confinement mechanics [XREF:S11].
2. **Tests are read-only to the executor** — direct test editing is the dominant measured cheat class for Claude-family models [R04 §2.4].
3. **The pass/fail verdict is computed outside every agent-accessible environment** — in-band success signals (exit codes from inside the agent's process) are never trusted [R04 §2.4].
4. **Test provenance:** acceptance tests originate from the frozen ACs — from the structured sub-line where one exists [G1 P10; S06.6] — and are written in a separate context (different session or model) from the implementation; doer-written tests pass by construction [R04 §2.4].
5. **Sanctioned challenge instead of edit access:** the executor gets an explicit "these tests look wrong" escalation (→ CHECK-INTEGRITY card, S07.7) — a measured anti-hacking device, while prompt text is never the load-bearing defense; structure is [R04 §2.4].
6. **Flaky checks are quarantined** with an owner and a fix-by date, never retried-until-green [R04 §2.4].
7. Every check suite carries a **`verified-on` stamp** and a periodic planted-defect audit (P-T06-1, S07.9).

**Web research (specified now; idles until the domain arrives at v0.1 [XREF:S19]).** Mechanical citation pass — extract claim–URL pairs, fetch-and-check with fabricated-vs-rotted disambiguation, staleness stamps against the task's recency requirement — run as a **platform check under its own egress-allowlisted confinement** (the executor's verification sandbox stays network-off; C3 arrives with the domain [XREF:S11]). Link validity is never treated as quality: >94% link validity coexists with 39% claim accuracy — entailment (S07.4) is the load-bearing check [R04 §2.5, §4]. The research rubric names which failure rate each check measures (fabricated-URL vs claim-support vs response-level) [R04 §2.5].

**Stage contracts.** Every PLAN step's "Done when" line (S06.6) is checked at its stage boundary; contract states are PASS / FAIL / **N-A** / **UNVERIFIABLE-HERE**, with first-upstream-failure attribution. A stage contract is incomplete unless it declares its finding categories and their escalation routes — definition-of-done includes definition-of-cannot-be-done-here [R04 §6-S3].

**Did-research-actually-run (1.9).** For every research node in the PLAN (S06.6), a deterministic post-step check reads the metering ledger's per-step usage counters [XREF:S10] and requires ≥1 search/research-tool invocation. Zero invocations → the node re-runs once in a fresh session (`⚙ verification.research_rerun_limit = 1`), then a RESEARCH-NOT-RUN decision card — correctness on world-facts is never left to model initiative or memory [1.9; S06.6 hand-off].

### S07.4 Entailment coverage for research-domain claims

Claim–citation entailment coverage is **mandatory for load-bearing claims and sampled for the rest** (`⚙ verification.entailment_sample_rate`) [G1 Def.2]. Claims are judged against the **fetched source content**, never against the deliverable's own citation text — judges are flippable by fabricated authority [R04 §2.1, §2.5]. Entailment is a narrow task validated at expert grade (judge agreement with physician consensus exceeded inter-physician agreement) and runs on the local tier [R04 §2.5]: default seat Granite-Guardian-8B, CPU floor Flan-T5-0.8B for sampled checks [G3 Def.4; seat mechanics XREF:S12].

Thresholds and the mandatory-coverage bar (which claims count as load-bearing) are TBD-BRINGUP(entailment calibration set — planted supported/unsupported pairs + first real outcomes; pre-registered TPR/TNR bar before entailment gates unsupervised) [G3 Def.4, Def.8]. **A8 (2026-07-23, B4 gate):** the pre-registered bar is now MEASURED — Granite Guardian 4.1 on the 156-pair Sinet set gives per-side TPR 0.949 / TNR 1.000: the ≥0.90 MAIN bar is met, the load-bearing 0.95 sub-bar is NOT (entailed side 0.949), so entailment stays conservative and idle; ⚙ `verification.entailment_sample_rate` DERIVED 0.20, live write pending at bring-up (result file `P3/measurements/2026-07-22-entailment-thresholds.md`). **v0 status:** the launch domain is software, so this machinery is specified and ships idle; it activates with the web-research domain at v0.1 [G1 D1.2; XREF:S19].

### S07.5 V2 — the dual-axis judge pass

**Judge engine and routing.** The V2 judge runs at **planning-model class on a paid flat-rate lane**: G1 Def.1 named the utility model as default judge before the G3 ceremony cut line measured ≤8B-class generalists judging below random on hard pairs; the cut line governs the engine class, and Def.1's pairing and flagging rules apply to it unchanged [G1 Def.1; G3 T15 digest; S06.10]. The pairing rule [G1 Def.1]:

- Default judge = the requester's designated verification-review engine (recommended platform-wide, user-overridable, per 1.10).
- When the judge's model family equals the executor's and another flat-rate lane is connected, D5 consumption-pressure routing MAY swap the judge to the dissimilar lane — same-family judges inflate own-style output through familiarity, and model errors grow more correlated as capability rises [R04 §2.1, §2.3].
- **Self-family judging is always flagged on the receipt** [G1 Def.1]. Selection mechanics are S08's [XREF:S08].

**Independence.** The judge is a fresh, clean-context platform session — never the executor's session, never a continuation of its context [G1 D1.1]. It receives the frozen ACs and the deliverable diff and never the executor's overlay, history, or `learned_this_task` [XREF:S05 clean-context exception]. Judgment-shaped helper work inside a task uses the same clean-context rule [S04.2]; the deliverable gate specified here is always a platform stage, not a helper.

**Judge input slice** (the whole world the judge sees): the artifact + its diff against the previous revision [XREF:S13] + the frozen ACs + the rubric version + V1 check outcomes as evidence + prior-round findings — **never the execution transcript** (transcript re-sends are the measured O(M²) prefill trap, and the transcript is the executor's frame) [R04 §2.9, §4]. Trajectory/transcript audit exists only as a special-purpose side-effect audit, never the default [R04 §2.9].

**Axis 1 — spec compliance.** One binary verdict per numbered AC with a mandatory extractive evidence quote and an Unknown escape [R04 §2.1, §3-B]. Where a structured sub-line exists, the verdict binds to it; otherwise it judges the plain line [G1 P10; S06.6]. For sub-lines already executed at V1, the judge consumes the V1 outcome as evidence and does not re-decide the mechanical fact; a judge–check disagreement is a CHECK-INTEGRITY finding, not an override `[coordinator-draft]`. Findings are emitted as numbered **`[F1..Fn]`**, each anchored to the exact place it concerns (file:line / section / claim id; anchor mechanics [XREF:S13]) and tagged **blocker | note** [5.5; R04 §4]. Every blocker MUST cite the frozen criterion (or axis-2 rubric item) it violates; a finding that cites none can only be a note or a REOPEN-SPEC matter — that is what keeps goalposts structurally fixed (5.4).

**Axis 2 — outcome sanity (the P45 axis).** Separately prompted with its own mini-rubric, never folded into the compliance pass — models spontaneously notice what they weren't asked about ≈28% of the time, so an unprompted second axis does not exist [R04 §2.2, §3-C]. Its four probes: the reasonable-user test ("would a reasonable user consider this done and good?"), the implicit-expectations scan ("what would a well-informed person expect that is absent?"), the side-effect scan (unrequested changes are failures, not bonuses), and the expert-standard comparison [R04 §3-C].

**Verdicts:** **SHIP / SHIP-with-notes / REVISE (blockers only) / ESCALATE**, plus axis 2's unique power **REOPEN-SPEC** [R04 §3-B, §4]. REOPEN-SPEC files a decision card to the requester — proceed as specified / adjust the specification / rethink — and can never change the spec itself (5.2; D10). An accepted adjustment lands as an S06.9 ADDED/MODIFIED/REMOVED delta and re-freezes the ACs; later rounds judge against the amended frozen set. The only sanctioned way criteria ever change is through that human-approved delta — never through the judge.

Both axes fit one prompted call each; both are ceremony-billed to the requester and itemized (S07.11).

### S07.6 Bounded rework (5.4/5.5)

- **Only blockers trigger another round.** Notes ride along to the requester as review comments [XREF:S13] and never spin loops (5.4).
- **The retry package** is exactly: the original SPEC + the frozen ACs + the numbered findings `[F1..Fn]` with their anchors + the deliverable. Findings are carried verbatim as numbered points; the retry fixes named problems and never regenerates blind — intrinsic self-correction degrades strong models, and changes to already-correct work are ~always harmful [R04 §2.6; 5.5]. Requester comments (6.2) enter the retry through the same numbered-anchored-point channel [XREF:S13].
- Each round's executor is a **fresh session** built from the retry package (fork-don't-poison, consistent with the S04.5 helper-retry rule and fresh-context-per-stage [G1 D1.1]).
- **Re-review judges against the ORIGINAL criteria:** same frozen ACs, same rubric version, prior findings in scope. After round 1, new note-class findings are suppressed to a count (goalpost-drift suppression); new blockers are admissible only citing the same frozen criteria [5.4; R04 §2.6].
- New term — **finding key**: the stable identity of a finding (criterion + anchor + failure class). Recurrence of a finding key across rounds is the convergence signal.
- **Stop rules:** hard cap `⚙ verification.rework_rounds = 3`; convergence = the same finding keys recur unresolved OR artifact similarity holds for `⚙ verification.convergence_patience_rounds = 2` consecutive rounds ("model inertia" — cosmetic edits in response to findings) [R04 §2.6, §3-D]. No judge-every-round continuation gating: the smart stopper costs 2.3× tokens for zero quality gain [R04 §2.6, §5].
- **Cap-hit or convergence without SHIP → ESCALATE decision card** carrying best-effort state and the full round history — never a silent stop, never an error [R04 §3-D]. Every round's consumption bills to the requester, itemized per round (3.4; S07.11).

### S07.7 Escalation: typed routes, proven live (5.6, P46)

**Every verification finding terminates in a human-visible sink — a decision card, an operator alert, or a worker-flaw ticket. No other sink type exists**, so "a finding that died in a log" is structurally impossible — and the claim is proven by tests, not assumed (5.6; P46) [R04 §4]. Escalations enter the risk-ranked, batchable approval inbox (S3.2) as structured, resumable asks; answering resumes the pipeline in place (4.3). Queue curation is a verification requirement, not UX polish: past reviewer capacity, additional escalations measurably deplete attention and the human gate rots [R04 §2.7].

**Route table** (categories and routes; the enum is `[coordinator-draft]`, the sink-types-only rule is ratified [R04 §3-E, §4]):

| Category | Raised by | Route | SLA class |
|---|---|---|---|
| AC-BLOCKER | axis 1 | rework round (S07.6); at cap → requester decision card | approval |
| SANITY-BLOCKER | axis 2, short of spec doubt | rework round; at cap → requester decision card | approval |
| REOPEN-SPEC | axis 2 | requester decision card → S06.9 delta on accept (S07.5) | approval |
| CHECK-INTEGRITY | executor test-challenge; judge–check disagreement; flake quarantine; failed suite audit | requester decision card + suite quarantined pending fix | approval |
| RESEARCH-NOT-RUN | S07.3 counter check, after its retry | requester decision card | approval |
| CAP-HIT | S07.6 stop rules | requester decision card + best-effort state | approval |
| WORKER-FLAW | recurring defect pattern attributable to a worker | worker-flaw ticket (7.2/7.3) [XREF:S08] | approval |
| SAFETY | watchdog pause-and-flag [G1 D1.3; XREF:S14]; confinement violation [XREF:S11] | operator alert | safety |

**The ratified SLA set** [G1 Def.3 as superseded at close]: approval-class cards remind at `⚙ verification.card_remind_hours = 4` and push-notify at `⚙ verification.card_push_hours = 24`; safety-class items push **immediately** and re-ping at `⚙ verification.safety_reping_hours = 1`. Unanswered asks expire per platform policy [G2 Def.2; XREF:S02]. These are inbox-wide numbers ratified at G1 close, registered here and consumed by the inbox surface [XREF:S15]; the S18 sweep reconciles naming.

**Liveness is proven three ways, all mandatory** [R04 §2.7, §4; P-T06-4]:

1. **Planted-defect e2e test class.** For every route-table category, a synthetic task plants one defect of that category and the test asserts a human-visible card/alert reaches the inbox within its SLA. Standing conformance-suite entries, run in CI and on schedule [XREF:S14] — the same prove-by-violation pattern as S04.5.
2. **Dead-man canary, daily.** A scheduled trivial always-escalating synthetic task runs the real pipeline end-to-end on the local tier (costs no allowance) at `⚙ verification.canary_interval_hours = 24`; a watchdog **external to the pipeline** alerts the operator when the canary's card stops arriving. Alert-on-silence: the death of the escalation path is itself what fires [G1 Def.3 as superseded; R04 §2.7].
3. **Quarterly full drill.** At `⚙ verification.drill_interval_days = 90`, a Human-Handoff-style drill: a planted defect must traverse to the right person carrying facts, sources, uncertainty, action history, and a recommended next step; pass/fail recorded [G1 Def.3 as superseded; R04 §2.7]. TBD-OPERATOR(answer the quarterly drill card end-to-end, from a phone).

### S07.8 Scope rules: stakes gating, steered output, degraded domains, graduation

- **Launch domains get axis 2 on every deliverable** [G1 D1.2]: software at v0; web research joins at v0.1. Elsewhere axis 2 is stakes-gated: it runs at stakes tier ≥ `⚙ verification.sanity_stakes_floor = standard` `[coordinator-draft default]` [G1 D1.2; tiers S06.4].
- **The zero-interaction band skips ceremony, never verification.** Trivial-band tasks pass V0–V2 like everything else (5.1 has no exceptions); the tier changes only V3's blocking-ness — trivial delivers the non-blocking completion card [S06.4], every other tier requires explicit accept (5.8).
- **Steered-output backstop (4.7).** A deliverable any part of which flowed through an untrusted-content reader (a T-SPEC quarantined helper today [S04.2]; a C3 worker at v0.1) never skips V2 and never skips axis 2, regardless of stakes gating — injected content can steer reasoning inside any sandbox, and the verification gate judging steered output is the guaranteed blast-radius layer [4.7; S5.4; S5.6].
- **Degraded-mode domains** (arrive v1 [XREF:S19]): V0 and V3 are universal from day one; V1 is empty until the domain's purpose-built check pack exists; V2 runs a generic rubric with its verdict **advisory and visibly marked non-authoritative**; mandatory requester review is the real gate; receipts carry the domain's verification-maturity level [2.1; 7.6; R04 §4].
- **Bootstrap posture — the command-less launch-domain task [A14, 2026-08-27].** A launch-domain deliverable whose project has NO captured check pack (the fresh-scaffold case: the S13.7 registry capture holds no build/test/lint commands, so S07.3's executable ladder has nothing it could run) is NEVER a verification refusal and never parks the run. Instead: V0 runs unchanged; **V1 runs the S07.3 stage-contract checks**, with every executable-ladder rung that would need the missing commands recording **`UNVERIFIABLE-HERE`** (the ratified S07.3 contract vocabulary) rather than being skipped silently or reported as PASS; **V2 runs with its verdict advisory and visibly marked non-authoritative** (the degraded-mode treatment above); **requester review is mandatory** — V3 blocks at every stakes tier, including trivial-band. The verdict card and receipt name the bootstrap posture in plain words, including that capturing the project's commands restores the full ladder. The posture is computed per revision from the registry's current capture — once commands exist, the full ladder resumes and the advisory marking drops; no user action is ever a precondition for verification itself to run. A verification refusal terminal remains only for genuine integrity cases (S07.7 CHECK-INTEGRITY), never for pack absence [operator record `P3/design/b6-gate-operator-findings-r4-2026-08-23.md` F1a; drafted at P3-RW-14 OQ3].
- **Graduation rule — 7.6 made falsifiable:** a domain gains full-pipeline status only when its purpose-built check pack exists AND passes its planted-defect suite [R04 §4]. Suite execution is benchmark/eval machinery [XREF:S14].

### S07.9 Verification-practice failure modes (P-T06-1..5, first-class)

Verification is a platform practice with its own failure modes, not a stage [G1 findings digest, T06]. Each is owned here with a normative countermeasure:

| Id | Failure mode | Countermeasure (binding) |
|---|---|---|
| **P-T06-1** verifier rot | check suites and rubrics decay silently (30% of a professional benchmark broken; 345 falsely-passing patches) [R04 §2.4] | `verified-on` stamp on every check suite and rubric bundle; planted-defect audit every `⚙ verification.check_audit_interval_days`; a suite past its audit interval flags the verdict card stale; audit failure → CHECK-INTEGRITY quarantine (S07.7) |
| **P-T06-2** retrieval-contaminated verification | executor passes checks by lookup, not work [R04 §2.4] | verification-run confinement defaults: network-off + answer-bearing VCS history stripped (S07.3 rule 1); research nodes are the sanctioned, PLAN-declared lookup path [S06.6]; mechanics [XREF:S11] |
| **P-T06-3** style-inflated judging | well-formatted mediocrity over-scores (style bias 0.10–0.76, dwarfing position bias) [R04 §2.1] | judges receive extracted claims/diffs, not presentation, wherever the deliverable type allows; behaviorally anchored rubrics with evidence quotes (S07.10); per-judge-model length-bias noted in the rubric bundle and re-measured on judge change |
| **P-T06-4** escalation-path death | the route exists in config but not in fact — P46 in new clothes | the three S07.7 liveness proofs (planted-defect e2e class + daily dead-man canary + quarterly drill), wired into observability [XREF:S14] |
| **P-T06-5** judge/rubric calibration invalidated by model change | provider swaps silently change judge behavior mid-stream [R04 §2.8] | judge model pinned per rubric version; any judge-model change — including local duty-alias swaps [XREF:S12] — gates on a golden-set re-run (TPR/TNR re-measured) before unsupervised judging resumes (the 7.3-analog); every local-tier swap mandates threshold recalibration [G3 T15 digest] |

### S07.10 Rubric and judge governance (5.7)

- **Rubrics are versioned knowledge objects entering through the 8.3 knowledge gate** — drafted frontier-assisted at implementation time, versioned, attributable, removable [5.7; XREF:S09]. TBD-P3(seed rubric drafting session per launch domain, 8.3-gated).
- **Rubric engineering rules** [R04 §2.5, §4]: binary verdict per criterion; one construct per criterion; behavioral anchors per level; immutable versioned bundles; extractive-quote grounding so a high score is impossible without evidence.
- **Each rubric version pins its judge model**; changes follow P-T06-5 (S07.9).
- **Golden sets:** 25–50 human-labeled cases per launch domain, refreshed from real incidents; the judge is validated as a classifier (TPR/TNR) against them, and judge-reported rates are statistically corrected rather than taken raw [R04 §2.1, §2.8, §4]. Seeding: TBD-BRINGUP(golden-set seed per launch domain — planted-defect cases + first real outcomes). Storage and runners are eval machinery [XREF:S14].
- **Falsifiability:** the benchmark practice tests whether rubric-driven review actually catches real and planted defects, so the rubrics themselves stay falsifiable [5.7; 11.2]; protocol per `Spec/benchmark-preregistration-v1.md`, cited never restated [XREF:S14]. Planted defects are the standard falsification method for judges, rubrics, AND executable checks — checks rot too (P-T06-1) [R04 §2.8].
- **No prefab scores:** imported "hallucination/quality scores" whose mechanics Sinet cannot inspect are banned; every check is a named, versioned, falsifiable object [R04 §5; 5.7].

### S07.11 Recording, receipts, cost shape, and the watchdog boundary

- **Every verdict is recorded with its reasons, per round (S2.3):** what was checked, against which criteria (AC ids + rubric version + judge model + its current golden-set error rates), what passed, what failed, what was flagged note-vs-blocker — as event-log rows under the ratified event-type contract [G2 D2.1(d); XREF:S14], so progression across revisions is visible. Verdicts are keep-forever [G2 Def.11]. Verdicts also land in the ledger's verified-status section [G1 Def.12; XREF:S05], and every V2 verdict is checkpointed (D7) [XREF:S02].
- **Cost shape:** per-deliverable paid verification is bounded to ≤ rounds × 2 judge calls (one compliance + one sanity); V0, V1, entailment, and any future screens run on the free tier; the judge sees the S07.5 input slice, never history [R04 §2.9, §4]. Verification consumption is ceremony-billed to the requester and itemized on the receipt (3.4/3.6) [XREF:S10] — the platform's verification tax is a permanently measured number, and the self-family-judge flag rides the same receipt [G1 Def.1].
- **Watchdog boundary:** stalled/looping/silent/runaway detection belongs to the watchdogs [4.6; XREF:S14]; containment is pause-and-flag, never auto-kill [G1 D1.3]. Verification never watches liveness, and a watchdog flag is never a quality verdict. The single crossing point: SAFETY-class routing (S07.7) and the shared run identifiers in the event log, so a paused run's verification history is one query away [XREF:S14].

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `verification.rework_rounds` | 3 | 0–ceiling | R04 §4 via G1 D1.1(d); see Open item 1 |
| `verification.convergence_patience_rounds` | 2 | 1–`rework_rounds` | R04 §2.6/§3-D via G1 D1.1(d) |
| `verification.sanity_stakes_floor` | standard | tier enum (S06.4) | G1 D1.2 mechanism; default `[coordinator-draft]` |
| `verification.entailment_sample_rate` | TBD-BRINGUP → derived 0.20 (A8, 2026-07-23); live write pending bring-up | 0–1 | G1 Def.2; G3 Def.4/Def.8 |
| `verification.research_rerun_limit` | 1 | 0–2 | 1.9; S06.6 hand-off (R03 §4 Stage 2(d) as cited there) |
| `verification.check_audit_interval_days` | 90 | > 0 | P-T06-1; R04 §4 "periodic"; default `[coordinator-draft]`, aligned to the drill cadence |
| `verification.canary_interval_hours` | 24 | > 0 | G1 Def.3 as superseded at close ("canary daily") |
| `verification.drill_interval_days` | 90 | > 0 | G1 Def.3 as superseded at close ("drill quarterly") |
| `verification.card_remind_hours` | 4 | > 0 | G1 Def.3 as superseded at close |
| `verification.card_push_hours` | 24 | > 0 | G1 Def.3 as superseded at close |
| `verification.safety_reping_hours` | 1 | > 0 | G1 Def.3 as superseded at close |

All per G1 rider 1: operator-editable with audit trail; auto-adjustment only within operator ceilings, visible on receipts. The four SLA values are inbox-wide (registered here, consumed by S15; S18 reconciles naming).

**Known problems owned here:**
- **P-T06-1** verifier rot → `verified-on` stamps + scheduled planted-defect audits + stale-flagging + audit-failure quarantine (S07.3, S07.9).
- **P-T06-2** retrieval-contaminated verification → network-off, history-stripped verification confinement defaults (S07.3); enforcement mechanics [XREF:S11].
- **P-T06-3** style-inflated judging → claims/diffs-not-presentation judge inputs + anchored rubrics + per-judge length-bias tracking (S07.5, S07.9, S07.10).
- **P-T06-4** escalation-path death → three liveness proofs (S07.7); observability wiring [XREF:S14].
- **P-T06-5** judge calibration invalidated by model change → judge-pin per rubric version + golden-set re-run gate before unsupervised judging resumes (S07.9, S07.10).

**Deferred / parked:**
- Local calibrated-abstention screen tier (the N11 cascade middle) → re-entry when the local battery clears a pre-registered TPR/TNR bar on Sinet golden sets on real outputs AND consumption pressure demands relief; expectation pinned to window relief only — production cascades are accuracy-neutral [R04 §6-N11; G3 Def.3 pattern; XREF:S12].
- Axis-2 demotion to stakes-gated in launch domains → 11.2 evidence that axis 2 adds no catches over compliance-plus-human across 3 months of real use [R04 §4; G1 D1.2].
- V2 migration to the free tier → a local model reaches frontier-band agreement on the golden sets [R04 §4].
- Aggregated programmatic weak verifiers with learned weights → a labeled-outcome corpus exists (post-launch) [R04 §3-A4].
- Research-domain V1 pack + entailment activation → v0.1 domain arrival [XREF:S19].
- Standing trajectory/side-effect audit machinery → only if a real side-effect incident class emerges; admissible today solely as the special-purpose audit named in S07.5 [R04 §2.9].

**Coverage:**

| Feature-list item | Subsection |
|---|---|
| 5.1 nothing delivered unverified; type-matched checks | S07.1, S07.2, S07.3 |
| 5.2 two independent questions; reopen-the-spec as human decision | S07.1, S07.5 |
| 5.3 cheap-first; effort scales with survivors and stakes | S07.1, S07.2, S07.3 |
| 5.4 bounded rework; blockers-vs-notes; original criteria | S07.5 (blocker rule), S07.6 |
| 5.5 concrete, carried-forward feedback | S07.5 (`[F1..Fn]`), S07.6 |
| 5.6 tested escalation paths | S07.7 |
| 5.7 per-domain judges; falsifiable rubrics | S07.10 (+ S07.8 graduation) |
| 5.8 human final gate | S07.1 (V3), S07.7 (inbox), S07.8 (band rule); accept mechanics [XREF:S13] |
| 1.9 slice — did-research-actually-run | S07.3 |
| 2.1/7.6 slice — verification maturity, degraded mode, graduation | S07.8 |
| 3.4/3.6 slice — verification billing and itemization | S07.11 |
| 4.7/S5.4/S5.6 slice — verification gate on steered output | S07.8 |
| S2.3 verdicts recorded with reasons, per round | S07.11 |
| 11.2 slice — rubric falsifiability via the benchmark practice | S07.10 [XREF:S14] |

**Open items for G4:** none.
1. ~~Rework-round default~~ **Coordinator-resolved (2026-07-18): default 3 stands.** The apparent conflict is a binding-layer question the contract already answers: G1 D1.1(d)'s operative text ratifies **"(report 04 §4)"** — whose stopping rule reads verbatim "hard cap (default 3 rounds, config)" [R04 §4-D, §4 rework-loop] — while "(one retry)" appears only in the same gate file's findings-digest compression, which is not the ratified object. Blockers-only re-entry makes rounds 2–3 rare in practice; a ⚙ setting either way. Flagged for G4 attention as a resolution note, not an open choice.

## S08 — Workers, composition, routing & automations

**Scope:** The D8 worker ontology made concrete: the Sinet-owned worker store and its guardrail split, per-invocation compilation onto engines, template → overlay → instance mechanics with versioning and promotion, the specialization content policy, the composer and its validation battery under compose-when-earned, worker/model/effort routing with recorded reasons, deterministic C0 automations, and the revalidation regime.
**Binding inputs:** D8, D10 · feature list §7 complete; 2.1/2.3/2.6 + slices of 2.7, 3.4, 4.4, 5.7, 8.8, 9.4, 12.5; 14.2–14.4; 15.6; S2.6 · [R15 §4] consumed as ratified direction per the G3 findings digest (gate CLOSED, no exception taken; its open numbers ratified as [G3 D3.4]) · [G2 D2.1] (R11 §4 memory, R12 §4.1/§4.7 settled), [G2 D2.2] (R14 §6.1/§6.2 steers, A2 dialect verdict), [G2 Def.7] · [G1 D1.1] (R06 §4), [G1 P7, P9, D1.7, riders 1–2] · [SPIKE P2-S5] as carried by S03's lowering. Siblings: [XREF:S03] compile targets, pins, lowering · [XREF:S04] spawn mechanics · [XREF:S05] stage-brief assembly, trace manifest · [XREF:S06] plan requirements, duty maps · [XREF:S07] verification, rubrics, quality checks · [XREF:S09] knowledge/lesson storage · [XREF:S10] pressure, receipts, scheduling · [XREF:S11] confinement enforcement · [XREF:S12] local duty seats · [XREF:S14] runbooks, watchlist, canaries.

**New terms coined here:** **gap record** — the persistent record of a no-fit routing outcome {selector signature, family, task ref, disposition}, accumulated so recurrence can earn composition (S08.6). **Composer playbook** — the versioned best-practice knowledge object that steers the composer (S08.6). **Task-class ceiling table** — the per-(domain, task family) table of maximum tool/confinement/egress powers enforced by the permission audit (S08.6); 4.4's instrument.

### S08.1 The worker store: rows plus files in a Sinet-owned superset schema

A worker is **registry rows plus a git-versioned template file** in a schema Sinet owns — the same row-index/file-content/git-history pattern ratified for knowledge [R15 §4.1; R11 §4; G2 D2.1]. **Engine formats are compile targets, never the store**: no cross-vendor agent format exists, engine vocabularies carry breaking renames inside single major versions, and Sinet-only fields (confinement class, provenance, maturity, eval hooks) are inexpressible in every engine format [R15 §2.1, §3-A2]. The store is multi-user from day one; every row carries its owner (15.6).

Tables (field lists; control plane sole writer [XREF:S02]):

| Table | Fields |
|---|---|
| `worker_templates` | template_id · name · owner_user · scope `personal\|household` · kind `agentic\|automation` (S08.9) · domain → `domains` row · status `draft\|validated\|active\|flagged\|retired` · `active_version` — the **only** mutable pointer |
| `worker_template_versions` | version_id · version · supersedes · file_ref {git path, commit hash} · provenance block {author: human \| composer(model+version, playbook version); evidence refs (gap record, spec); approver + date; origin: composed \| human-written \| imported \| adopted-from(source)} · graduation event (first-N complete) |
| `worker_guardrails` | keyed by version_id — the enforcement state of S08.2, exclusively: granted tools · permission map · confinement class (C0–C2 at v0) · egress class · budget/ceiling set · gate policy · first_n_remaining · schedule_attachable |
| `validation_records` | (version_id × model × engine pin) → {lint result, audit result, dry-run ref, approver, date}; for kind=automation the key's engine-pin slot holds the dialect version (S08.9) |
| `domains` | domain · verification maturity `full\|degraded` · rubric/quality-check ref [XREF:S07]. Day-one rows: software = full; web-research = full at v0.1; all others degraded [2.1; XREF:S19] |
| `gap_records` | selector signature · family · task refs · occurrence count · last_seen · disposition (S08.6, S08.8) |

**The template file** (markdown + YAML frontmatter) carries the *behavioral* definition only [R15 §4.1]:

- **identity/purpose:** name; delegation-grade `description` (the routing selector text; compiles to engine description fields); task selectors (family per the S06 vocabulary, task-class keys, trigger phrases — deterministic rule inputs, never agent-supplied lookups [R11 §4 selector discipline]);
- **execution profile:** duty class + effort floor, lane-agnostic (routing stays D5 consumption-pressure); a concrete model pin only with a recorded reason — that pin is what 7.3 flags (S08.10);
- **equipment (requested):** tool list; skill refs; knowledge selectors (L2 topic keys resolved by the injection manifest at run time [XREF:S05; XREF:S09]); connector/MCP *names* — broker-resolved references, NEVER secrets [D2; XREF:S11];
- **prompt body:** procedural instructions — objective template, method, boundaries, output contract, escalation triggers; at most ⚙ `workers.persona_lines_max` tone/behavior lines (S08.5); no security prose pretending to be enforcement;
- **eval hooks:** golden_set_ref, planted_defect_ref [XREF:S07; XREF:S14];
- **maturity:** the domain ref whose `domains` row drives 7.6 marking (S08.7).

Skills ride the **Agent Skills directory format** — the one genuinely portable layer; both v0 engines read it natively — with one Sinet restriction: skill files are static; **no load-time-executing dynamic content**, ever [R15 §4.1; P-T14-2].

### S08.2 The guardrail split (14.2 made structural)

Behavioral content lives in template files; **ALL enforcement state — effective tool grants, permission maps, confinement class, egress class, budgets and ceilings, gate policy, the first-N counter, schedule attachability — lives EXCLUSIVELY in `worker_guardrails`**, written only by the control plane on a human approval, present in no file, and **recompiled into the engine invocation on every run** [R15 §4.1; 14.2]. The file may *request* equipment; approval copies requested → granted only after the S08.6 permission audit.

The enforcement is structural, never advisory:

- The template store and `platform.db` sit outside every task sandbox — a running worker can write neither [XREF:S11]. Workers can NEVER alter their own permissions, budgets, or approval gates (14.2); the change surface does not exist inside a run.
- A guardrail-class field appearing in any definition file is a **structural lint reject** (S08.6 station 1) — not a warning.
- Because guardrails are recompiled from control-plane tables every run, a tampered or stale compiled artifact cannot carry privilege forward; the hash check (S08.3) catches the tamper, the recompile removes the staleness.

Evidence basis, carried as design fact: self-modifying artifacts sabotage their own validators when guardrails are reachable — the DGM record is the case for making 14.2 structural [R15 §2.3]; an engine vendor independently strips guardrail-class fields (`hooks`, `mcpServers`, `permissionMode`) from definitions crossing a trust boundary [R15 §2.1]; agent-writable config is a CVE-backed escape surface [R15 §1; G2 §Follow-ups]. The line to hold: **instructions steer, config enforces — the channels are never confused** [R15 §2.6].

### S08.3 Compiled per engine, per invocation, hash-pinned

At spawn the control plane compiles **(template version + granted guardrails + the requester's overlay slice + instance refs)** through S03's engine lowering into the lane's native artifacts, so that Sinet-compiled config is the ONLY config the engine sees [XREF:S03; SPIKE P2-S5 via S03.5]. Properties:

- **Fresh every run.** Compiled artifacts are per-invocation and never persist as engine-side state between runs; enforcement state cannot drift from its tables [R15 §4.1].
- **Hash-pinned, body + config together.** The compiled artifact is hashed as one unit and recorded on the run — which compiled configuration produced which outcome is 7.7's audit extended to configuration. **No half of the definition is runtime-editable**; the instruction body escaping the compile gate is a named industry hole Sinet closes by construction [R15 §4.1, §2.6].
- **Per-engine capability matrix.** Lowering rules live per pinned engine version [XREF:S03; manifest home XREF:S16]; a template feature the target lane cannot express fails loudly at validation time (the dry run executes on the actual target lane), never silently dropped [R15 §2.1 compile-down discipline].
- **Sole-controller preserved.** Every compiled artifact disables engine-native spawning by construction [G1 rider 2; knobs XREF:S03; topology XREF:S04].
- Assembly of the compiled equipment into the stage brief, and the trace manifest recording every injected item, are S05's [XREF:S05].

### S08.4 Template → overlay → instance (7.8); versioning, promotion, sharing

**Versions.** Every edit creates a new immutable version row plus a new file commit — history is never rewritten [XREF:S02 posture]; `active_version` is the only mutable pointer; **rollback = repoint** [R15 §4.5]. Any version change re-runs the S08.6 battery (cheap, structural); first-N resets when the diff touches body or equipment [G3 D3.4]. Version→outcome is a join, not a system: `routing.decided` records worker + version per run [R12 §4.1 settled]; joined to verdicts and receipts it yields per-version quality/cost views; the workforce map reads them (9.4) [XREF:S15].

**Overlays are the settled memory machinery — no second system** [R15 §4.6; R11 §4; G2 D2.1]. Per user × template: L1 observations (TTL) and L2 lessons/preferences in the worker-overlay scope, human-gated, provenance-carrying; storage, lifecycle, and adoption mechanics are S09's [XREF:S09]. The injection contract at instance compile: template body (base) + the user's overlay L2 slice + relevant L1 history (labeled as history, never rules) + the instance's L0 ledger section — concatenated deterministically, most-specific-last, under the 8.9 precedence (task spec > project truth > personal overlay > template baseline > house defaults). Conflicts between overlay lessons and template instructions are surfaced by the settled write-time machinery [XREF:S09], never left to the model's arbitrary pick [R15 §2.6]. Invariants: the template file is **never mutated** by an overlay; overlays carry **zero guardrail fields** — a lesson can say "prefer British spelling," never "skip the review gate"; the guardrail split makes the latter structurally homeless [R15 §4.6]. **Overlay staleness:** each lesson records the template version it was learned on; a body/equipment version bump marks affected lessons for the standing re-verify cycle instead of silently carrying them [R15 §4.6; G2 Def.7; XREF:S09].

**Instances** are per-run: L0 scratch + the run workspace, expiring with the task; nothing persists except through the gated lesson path [XREF:S09]. Two people on the same shared template run disjoint instances with disjoint overlays by construction — nothing leaks between them (10.1).

**Personal by default; promotion by operator approval (D10, 12.5).** Promotion to household-shared is an operator approval card carrying: the full definition diff, the provenance block, validation records, first-N history, and a **personal-data scan** (a lint rule: the template must not embed user-scope content — overlays hold the personal part by construction) [R15 §4.9]. Shared templates are **read-only references**: adopters attach their own overlays; edits to a shared template are operator-gated new versions; members' overlays persist across versions via the staleness mechanic; un-sharing or retiring surfaces cards to affected users. Lessons cross users only by 8.8 adoption — copy-on-adopt, origin visible [XREF:S09]. v0 operates single-user; this flow is day-one schema, exercised at v1 onboarding [15.6; XREF:S19].

### S08.5 What makes a worker a specialist (7.5 content policy)

Specialization budget goes where evidence says it pays, in this order [R15 §4.2, §2.2]:

1. **Curated tools** scoped to the task class — the first-order lever;
2. **2–3 concise curated skills** — never comprehensive dumps and never unreviewed self-generated content, both measured negative [R15 §2.2];
3. **Domain knowledge deterministically injected** via the manifest — force-loaded, never discovery-gambled; engine-side skill discovery measurably under-delivers [R15 §2.2; XREF:S05];
4. **Output contract + domain rubric ref** [XREF:S07];
5. At most ⚙ `workers.persona_lines_max` tone/behavior lines where the duty warrants it (e.g., a brand-voice writer).

**Persona prose is excluded by policy**: a replicated null for capability with measured accuracy harm from long personas — a tone lever only; station-1 lint warns beyond the cap [R15 §2.2, §4.2]. Honesty rule: in strong-pretraining domains specialization yields little — software workers stay **thin** (conventions, danger zones, verify commands), and the platform MUST NOT equate more template with better worker [R15 §4.2]. Templates grow only through the gated learning pipeline (lessons → L2 → distillation proposals [XREF:S09]), never by silent accretion. Palette raw material (retired Nexus prompt bodies; SAW stage contracts) enters composer input **re-cut as tool+context bundles, persona stripped — options, never defaults; never a standing formation** (14.4) [G2 D2.2; R14 §6.2; R15 §6].

### S08.6 The composer and the validation battery (7.1, 7.2)

**Compose-when-earned [G3 D3.4].** Composition is never the first response to a gap. A one-off task runs as **generalist-with-injected-knowledge** (coordinator + injected domain slice; degraded-marked where the domain is unverified). The platform composes a worker only when the work is **recurring**, **schedule/trigger-registered** (7.4), or **explicitly requested**. Every no-fit outcome writes a gap record; at ⚙ `workers.gap_proposal_count` occurrences of a task family, a composition proposal card surfaces (14.4 operationalized). Composition never rides the zero-interaction band — a new worker is exactly its condition-4 exclusion [XREF:S06].

**The composer** is **one-shot generation** — never archive/search evolution: search-based meta-agents are uneconomical at household n, archive-learning measures harmful, and self-referential optimizers game their validators [R15 §2.3, §3-C2]. It runs as ceremony on a frontier-class flat-rate lane per the requester's duty map (planning-model class — drafting is heavyweight ceremony under the G3 cut line [XREF:S06; R15 §3-C2]), billed and itemized to the requester (1.10; 3.4; 3.6 [XREF:S10]). Inputs, injected by policy: the task spec + gap record; the **composer playbook** — a versioned L2 house-knowledge object holding current worker-authoring practice with cited sources, refreshed via watchlist-triggered proposals on the house-playbook re-verify cycle, updated only through the D10 knowledge gate [R15 §4.3; G2 Def.7; XREF:S09; XREF:S14]; the palette (S08.5); the template schema, lint rules, and ceiling table. Output: a template file draft + requested grants + a proposed dry-run sample task + a draft golden-case seed.

**The four-station battery** — 7.2's "validation = structural checks passed plus human sign-off," made into stations [R15 §4.3]:

1. **Schema lint** (deterministic): frontmatter schema validation; name/selector conventions; unresolved tool/skill/knowledge references; persona-length warning; **instruction-pattern screen over body and skill files** — definitions are untrusted content (S08.10).
2. **Permission audit** (deterministic — a table, not a model; the least-privilege audit no public linter performs [R15 §2.3]): requested grants diffed against the **task-class ceiling table**; any above-ceiling request becomes a flagged line on the approval card; Rule-of-Two admission check on the resulting class combination [XREF:S11]; **structural reject** if any guardrail-class field appears in a file. v0 ceiling rows `[coordinator-draft]` (further families ship with their domains [XREF:S19]):

   | Task class | Max tools | Max confinement | Max egress |
   |---|---|---|---|
   | software · read/analyze (review, triage, Q&A) | read/search/workspace-read set | C1 | none |
   | software · implement/fix (write, build, test) | + workspace-write, build/test exec | C2 | allowlisted package registries |
   | chore · connector automation | fixed connector verbs only | C0 | the one named service |
   | generic fallback (unmatched family) | read-only set | C1 | none |

3. **Sandboxed dry run:** one sample task in an effects-impossible sandbox (C1, or the worker's class capped at C2) on the requester's lane, capped at ⚙ `workers.dryrun_cost_cap_usd`; transcript and output attach to the card [XREF:S11]. Sample authorship `[coordinator-draft]` (discharges R15-OQ4): the composer proposes the sample task + golden seed; the requester curates it on the approval card (edit/replace); on approval the dry-run pair seeds the worker's golden set, labeled unverified until the first accepted real outcome confirms it [XREF:S07; XREF:S14 growth loop].
4. **Approval-as-diff** (D10; 13.5): the definition as a readable diff, the requested-vs-ceiling powers table, the dry-run result, and the provenance block. The owner approves personal workers; the operator approves household promotion.

**Validation is explicitly NOT a quality guarantee** (7.2); it asserts structure, powers, and a witnessed run — nothing about output quality, which only S07's verification and the 11.2 practice measure [XREF:S07; XREF:S14]. Each pass writes a `validation_records` row per (version × model × engine pin).

**Supervised first-N [G3 D3.4]:** after approval, the first ⚙ `workers.first_n` outputs in full-pipeline domains require requester review regardless of oversight settings; degraded domains review everything anyway (7.6). The counter is **count-based** — a Sinet-specific formalization with no public prior art [R15 §2.3] — and **resets on any body/equipment version change**; graduation is recorded on the template version.

### S08.7 Honest capability marking and degraded mode (7.6, 2.1)

Degraded mode is **structural, not advisory** [R15 §4.4]. The template's domain ref resolves to a `domains` row; the scheduler and gate layer enforce, for maturity = degraded: the worker **cannot deliver without requester review**, **cannot attach to any schedule whose results auto-accept**, and its cards and receipts carry the visible marking [2.1; XREF:S10]. A degraded-domain worker can NEVER graduate to unsupervised operation while the domain lacks a real quality check (7.6).

- **Domain graduation** = a purpose-built rubric/quality check exists and passes its planted-defect falsifiability floor [XREF:S07; XREF:S14] → the operator flips the `domains` row through D10.
- **Worker graduation** = first-N complete AND domain = full.
- v0 operates the software domain (full); degraded-domain *operation* activates at v1 per 15.4 — the schema and enforcement above are day one [15.6; S00.2; XREF:S19].

### S08.8 Routing: a visible, overridable classifier (2.3, 7.7)

**Boundary:** the plan declares *requirements* — confinement class, tools, family, research nodes, effort mode [XREF:S06]; **selection of worker, model, and lane happens here**; spawn mechanics are S04's; admission, scheduling, and pressure accounting are S10's. The judge, planning, and utility duty maps are referenced, never restated [XREF:S06; XREF:S07; XREF:S12].

Selection pipeline — deterministic first, all free-tier [R15 §4.7]:

1. **Selector match** over the template registry: task selectors + FTS5 over delegation descriptions, filtered by status=active, domain, kind, required grants ⊆ granted, and confinement compatibility with the plan's declared class (equal or tighter only [XREF:S11]).
2. **Tie-break** among multiple candidates by a local duty alias with a one-line reason [XREF:S12].
3. **Model + effort:** the template's execution profile (duty class + effort floor) resolves against the requester's duty maps and the task's effort mode; effort-ladder mechanics and disclosed downgrades are S10's (3.5) [XREF:S10]. **Subscription coverage binds every choice:** routing selects only models the owner declared flat-rate, or the local tier; a metered model is selectable only under an explicit 3.10 flag — and the metered-exception list is EMPTY at v0 [G1 P7; Operating reality; D5]. Among flat-rate lanes, selection uses consumption pressure, never dollars [D5; XREF:S10].
4. **Research nodes** route to a search-capable lane [XREF:S06; lane facts XREF:S03].
5. **Helpers:** helper worker/model/lane selection uses this same pipeline with the spawn trigger as an input (the S04 boundary); mechanical helper duties default to the local lane — the permanent free tier [R06 §4.5; XREF:S04; XREF:S12].

**Accountability (7.7, S2.6).** Every worker/model/effort decision emits the settled `routing.decided` event {cause, score, signals, worker + version, model, lane, effort, plain_reason} [R12 §4.1; event contract XREF:S14] — inspectable per task, analyzable across tasks. **Visible and overridable:** the selected worker and its plain-language reason appear on the plan/approval card; the requester can re-route or pin before execution; overrides are recorded with their actor. The pinnable AXIS includes the LANE, and a lane pin has a second expression point: it may be declared at task CREATION, ahead of the first selection, and selection then honors it IN PLACE OF the step-3 consumption-pressure comparison **[S00.9 A13]**. Step 3's subscription coverage still binds and outranks it — a pin naming a lane the owner does not hold flat-rate is REFUSED at the system boundary before a task is born, never degraded onto another lane. **No trained router at household n** — every public router measures short of oracle, silent routing demonstrably destroys trust, and classification machinery below ~15 targets is overhead [R15 §2.5]; no silent switching, ever (14.3).

**No-fit is two-stage** [R15 §4.7]: (1) the task interpretation is confirmed (the S06 restatement); (2) one card offers **run-as-generalist** (default for one-offs; degraded-marked where applicable) / **compose-a-worker** (when earned, S08.6) / **subscription gap advice** (2.7 — when the gap is a *model*, not a worker; advice consumes the observed per-account model lists [XREF:S03]). A gap record is written in every case; the roster specializes because recurring work earned it, never because a formation was pre-staffed (14.4, 7.5).

### S08.9 Deterministic automations as C0 workers (2.6)

Same schema, same lifecycle, no second subsystem [R15 §4.8]. An automation is a worker with kind=automation: its body is a **deterministic workflow definition in Sinet's own versioned dialect** — mirrored A2 vocabulary, Sinet's own parser, explicit approval nodes for outward effects [G2 D2.2 Layer C] — executed by the platform with **no model in the loop**; equipment = the one named service plus the narrowest-scope broker-held credential; confinement C0, egress to that one service only [S5.1; XREF:S11]; guardrails identical (S08.2).

- **Born through the same composer path and battery** (S08.6); station 3 for automations = test execution on a sample payload with every outward effect materializing as a gated proposal — D7 makes this free. The **definition is presented at birth as a reviewable diff** with a preview, commentable — reviewed like code, managed like a worker (2.6).
- **Versioning discipline, anti-patterns banned by name** [R15 §4.8]: EVERY field change versions (settings changes included); ONE source of truth (rows + files — never a DB-versions-vs-git split); the whole definition is inside the compile hash — no runtime-editable body (S08.3).
- **Supervised first run** (2.6): the sample-payload execution plus requester review of the first ⚙ `workers.first_n` real results — the first-N counter applies kind-uniformly `[coordinator-draft]`; outward effects remain gated proposals regardless [D7].
- **Schedule attachment** (7.4): requires status=active, a current validation record (keyed on the dialect version — automations have no model or engine pin), a missed-slot policy (2.8), and the `schedule_attachable` guardrail; results then appear finished, verified, receipted; triggered runs bill the registrant (3.4). User-facing schedules and event triggers arrive at v1; at v0 automations run on demand [15.4; XREF:S19; scheduler XREF:S10].
- **Rollback** = alias repoint; **retirement** = status flip + schedule detachment + cards to affected users.
- A dialect-version bump is a revalidation trigger for all automations, with the same mass-flag semantics as an engine-pin bump (S08.10).

### S08.10 Revalidation triggers and definition hygiene (7.3; P-T14-1, P-T14-2)

**Revalidation triggers, extended beyond 7.3's model wording** [R15 §4.5]:

- **(a) Model change, deprecation, or replacement** (provider-announced or watchlist-detected [XREF:S14]): every template version whose validation record or model pin references the model is flagged and MUST be revalidated before further unsupervised use (7.3). The re-run is the settled runbook — golden set + planted-defect suite on the new model [R12 §4.7; XREF:S07; XREF:S14]: green → dated revalidation stamp; red → status=flagged + owner card. Flagged workers may run *supervised*, never unsupervised. Aggregate-green is insufficient — improved averages hide regressed slices; the planted-defect suite is the slice guard [R15 §2.4].
- **(b) Engine-pin bump = a mass revalidation event (P-T14-1).** Compiled artifacts and delegation/permission semantics are engine-version-coupled; a bump flags every active template version compiled against the old pin, exactly like a model change — and **no provider clock announces it: Sinet's own deliberate-bump procedure is the announcement clock** [XREF:S03 §S03.3 step 5]. Engine-behavior canaries carry detection between bumps [XREF:S14].
- **(c) Rubric or golden-set version change** → re-stamp affected workers [R12 §4.7; XREF:S07].

**Worker definitions are a prompt-injection carrier class (P-T14-2).** Template bodies, skill files, and imported bundles are instruction-bearing content that future runs obey; the measured registry record (91% of ClawHub's malicious skills carried injection; skill files that execute at load time) makes 4.7's hostility posture apply to the workforce's own source [R15 §2.6, §7]. Mitigations, all already structural in this section: the instruction-pattern screen at station 1 over every draft and import; static-only skill packaging (S08.1); review-as-diff gates (D10); and the guardrail split — a steered definition still cannot touch enforcement state (S08.2).

**Imports (P-T16-4).** **No auto-import from any public registry, EVER** [G2 §Follow-ups; R15 §4.9]. A human-initiated import lands as a draft with origin=imported, passes the FULL battery — lint + instruction-pattern screen + permission audit + dry run — and enters only through the D10 gate; four independent incident classes are the basis [R15 §2.6]. **Template signing: not at v0** — the store is control-plane-owned on one host and off-host integrity rides the 11.3 snapshot hash ledger [XREF:S13]; signing re-enters at v1 household onboarding or the first host-to-host template movement [G3 §Follow-ups; R15 §4.9].

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `workers.first_n` | 3 | integer ≥ 1; count-based; resets on body/equipment version change | G3 D3.4 |
| `workers.gap_proposal_count` | 2 | integer ≥ 2 | G3 D3.4 ("second occurrence"); G1 rider 1 |
| `workers.persona_lines_max` | 2 | integer ≥ 0; station-1 lint warn threshold | R15 §4.2 (⚙ there); G1 rider 1 |
| `workers.dryrun_cost_cap_usd` | 0.50 | > 0; API-equivalent (D5 currency) | R15 §4.3 (⚙ there); default anchored to G1 D1.7 `[coordinator-draft]` |

All per G1 rider 1: operator-editable, audit-trailed, auto-adjustment only within operator ceilings, visible on receipts; registry home [XREF:S18].

**Known problems owned here:**

- **P-T14-1** — engine-pin bumps are mass revalidation events: dispositioned S08.10(b); S03's bump procedure is the clock; canaries [XREF:S14].
- **P-T14-2** — worker definitions are a prompt-injection carrier class: dispositioned S08.10 (station-1 screen, static skills, diff gates, guardrail split).
- **P-T16-4** — registry supply chain / no-public-registry-imports: dispositioned S08.10 (import battery + D10 gate; no relaxation).
- **Shared:** config-poisoning-as-escape-surface (T09-owned, unnumbered in the G2 memo; S17 reconciles): the schema side is discharged by the guardrail split (S08.2); sandbox-side enforcement [XREF:S11]. Referenced: P-T16-1 pins/replacement paths for the capability matrix rows [XREF:S16].

**Deferred / parked:**

- Template signing → re-entry: v1 household onboarding, or the first host-to-host template movement [G3 §Follow-ups; R15-OQ7].
- Degraded-domain operation + user-facing schedules/triggers (7.4 surface) → v1 per 15.4 [XREF:S19].
- Learned/embedding routing → re-entry: roster persistently above ~15 active candidates with measured misroute pain; even then rank-only and visible [R15 §2.5, §4.7].
- Search-based composition → re-entry: evidence of a composer beating one-shot at household n [R15 §"What would change" 5].
- Engine-side skill discovery (relaxing force-injection) → re-entry: measured reliable self-selection, low-stakes duties only [R15 §"What would change" 4].
- Cross-vendor agent-definition standard → re-entry: an AAIF-class agent-layer standard landing; Sinet's schema re-evaluated as *its* superset, guardrail split kept regardless [R15 §"What would change" 1].

**Coverage:**

| Feature-list item | Subsection |
|---|---|
| D8 / 7.8 template → overlay → instance | S08.1, S08.4 |
| 14.2 workers never alter own permissions/budgets/gates | S08.2 |
| 7.1 self-composed workers (+ 2.6 born here) | S08.6, S08.9 |
| 7.2 validation-that-means-something; supervised birth | S08.6 |
| 7.3 versioned assets; model-change revalidation | S08.4, S08.10 |
| 7.4 recurring registration (requirements; surface v1) | S08.9 |
| 7.5 the workforce accumulates and specializes | S08.5, S08.8 (gap records) |
| 7.6 honest capability marking | S08.7 |
| 7.7 / S2.6 accountable, explainable routing | S08.8; S08.3 (config hash) |
| 2.1 slice — domain acceptance with maturity honesty | S08.7 |
| 2.3 task types → best-suited worker | S08.8 |
| 2.6 external-service chores as machinery | S08.9 |
| 2.7 slice — gap advice at no-fit | S08.8 |
| 4.4 minimal powers per worker | S08.6 station 2 (ceiling table), S08.2 |
| 5.7 slice — per-domain judge hooks on workers | S08.1 (eval hooks), S08.7 |
| 8.8 slice — overlay adoption, origin visible | S08.4 |
| 9.4 slice — workforce map reads version→outcome | S08.4 [XREF:S15] |
| 12.5 template sharing with provenance | S08.4 |
| 14.4 no standing army; machinery-when-earned | S08.6, S08.5, S08.8 |
| 15.6 multi-user schema day one | S08.1, S08.4 |
| D10 slice — own-object approval; operator promotion | S08.4, S08.6 |

**Open items for G4:** none. Coordinator-drafted sub-choices are tagged inline for G4 attention: the v0 task-class ceiling rows (S08.6), dry-run sample curation + golden-seed flow (S08.6, discharges R15-OQ4), the dry-run cost-cap default anchored to the D1.7 number, and first-N applying kind-uniformly to automations (S08.9).

## S09 — Memory & knowledge

**Scope:** The platform's memory and knowledge subsystem — layers, scopes, and writers; storage and retrieval; the gated learning pipeline and its v0/v1 activation boundary; provenance, influence tracing, and removal; sharing, shared project truth, and precedence; forgetting; the engine-memory containment posture; and governance of every knowledge object other sections declare. This section owns the **storage** side; brief assembly and the trace manifest are S05's [XREF:S05].
**Binding inputs:** R11 (whole report; §4 ratified as the memory package [G2 D2.1]); [G2 D2.7] (v0 memory surface), [G2 Def.7] (lifecycle set), [G2 Def.8] (vector post-gate), [G2 Def.9] (true deletion); [G1 Def.5], [G1 Def.12], [G1 rider 1]; [G3 Def.8] (contradiction-screen P/R in the bring-up set); D2, D6, D8, D9, D10; feature list 8.1–8.9, S4.1–S4.11, 7.8, 10.1, 11.3, 15.4, 15.6; committed siblings S05 (injection, manifest, `learned_this_task`), S06 (interview taxonomies, P47 trigger file), S02/S03 (fingerprint set; engine-config lowering); [SPIKE P2-S5]. Boundaries: S05 owns injection assembly + trace manifest; S13 owns git mechanics, snapshots, and backup; S08 owns what a worker overlay *is* structurally; S14 owns the canary suite and eval practice; S02 owns `platform.db` mechanics, retention, and the freshness pass; S10 owns the scheduler that runs this section's timed duties.

Terms coined here (first use): **lesson proposal** — a drafted candidate knowledge entry awaiting a human decision in the approval inbox; while pending it is read into no run context whatsoever (distinct from the glossary **proposal**, a gated outward effect). **Adoption** — copying another person's visible knowledge entry into one's own scope with the origin recorded and displayed. **Tombstone row** — the minimal audit stub left behind by true deletion [G2 Def.9]. **Knowledge object** — any L2 artifact under this section's governance: lesson, preference, playbook, rubric, style guide, exemplar, convention file content, taxonomy, trigger-rule file.

### S09.1 The layer model — defined writers, enforced in the schema (8.6, S4.2, S4.3)

Three layers, distinguished by lifetime and writer [R11 §4.1; G2 D2.1]. The writer rule is enforced by **capability withholding at the storage layer**: the control plane is the sole `platform.db` writer [XREF:S02], workers never hold store handles (D6), and each actor reaches only the write verbs its layer grants. A write into a layer the actor does not own is structurally impossible, not prompt-forbidden [R11 §2.3, §4.1].

| Layer | Content | Writers | Lifetime | Home |
|---|---|---|---|---|
| **L0 task scratch** (7.8 instance) | working notes; ledger `learned_this_task`; contained engine memory dirs (S09.9) | the run's stage sessions, via ledger verbs [XREF:S05]; helpers write nothing durable — their reports land in the coordinator's L0 [D6; XREF:S04] | expires with the task; at v0 discarded after the run summary [G2 D2.7; XREF:S05] | ledger section + run workspace [XREF:S02] |
| **L1 descriptive observations** (7.8 overlay memory; D8) | *observations only*: compact descriptive run summaries — what was done, encountered, failed; never instructions | platform distiller (local duty alias [XREF:S12]) from run evidence; workers may append observation notes. **Activates at v1** (S09.4) | TTL ⚙ `memory.l1_ttl_days` (90 d), prunable, distill-then-delete [G2 Def.7] | rows in `platform.db`, per-user × per-worker-template |
| **L2 knowledge** (permanent, human-gated) | lessons, preferences, playbooks, rubrics, style guides, exemplars, conventions, taxonomies, trigger rules, project truth | **humans only** — every entry enters via an approval (8.1) or is directly human-written. The distiller NEVER writes L2; workers NEVER write L2 | until removed/superseded; re-verify intervals per kind (S09.8) | markdown files in git-versioned knowledge dirs + registry rows (S09.2) |

**The descriptive-vs-prescriptive boundary** resolves the S4.3-vs-8.1 tension [R11 §4.1; G2 findings digest]: S4.3 lets workers write their own experience records; 8.1 forbids learning without approval. Resolution: L1 holds *descriptive* content only — facts about what happened, injected (from v1) solely as clearly-labeled history — while anything *behavior-steering* (a lesson, a preference, a rule, a playbook line) exists only in L2, whose single write path is the human gate. Workers therefore write L0 and descriptive L1 notes only; **prescriptive content from a non-human source anywhere in the store is a platform defect.** This is also the write-side poisoning defense: nothing a worker writes can steer any future run — its own or anyone else's — without a human decision in between [R11 §2.3, §4.1, §4.8].

Rolling summaries that steer behavior (the harvested `lts` pattern) are explicitly banned; consolidation happens only as gated distillation (S09.4, station 5) [R11 §6 N17].

### S09.2 Scopes and the schema core (15.6, S4.1, S4.5)

Scopes are orthogonal to layers: **user** (personal memory, S4.1) · **worker-overlay** (user × template, D8; structure [XREF:S08]) · **project** (S4.4) · **house** (S4.5, operator-gated D10). Every row carries `{owner_user, scope, scope_ref, layer, kind, status}` from day one, single-user operation notwithstanding (15.6). Schema core, extending S02's table set [R11 §4.2; field lists final; names `[coordinator-draft]`]:

- **`knowledge_entries`** — id, owner_user, scope, scope_ref, layer, kind (`lesson | preference | playbook | rubric | style | exemplar | convention | observation | taxonomy | trigger_rules` — last two added for S09.10 objects `[coordinator-draft]`), title, content (text) **or** file_ref (knowledge-dir path + git commit hash), `topic_key` (normalized subject, the conflict-lookup key), selectors (domain, project/repo, task-type, trigger phrases — rule inputs, never embeddings), status (`proposed | active | retired | removed`), version + `supersedes_id`, provenance fields (S09.5), verification fields (`verified_by, verified_at, reverify_interval`), `expires_at` (L1), `last_injected_at`.
- **`lesson_proposals`** — the pending queue: evidence refs (run ids, diff/comment/verdict refs), drafted content, proposed scope/kind, risk rank, batch id, status (`open | approved | edited_approved | dismissed`), decided_by/at. **Proposals are not memory: no assembly, tool, or search path reads them into any run context** [R11 §4.2].
- **`knowledge_adoptions`** — adopter_user, source_entry_id, adopted_entry_id, adopted_at (S09.6).
- **Knowledge files** live in per-scope git-versioned directories — `knowledge/house/`, `knowledge/users/<user>/`, `knowledge/projects/<project>/` — committed by the control plane **on approval, with the approver as author** (D9 attribution discipline; git mechanics [XREF:S13]). The row is the selector index, the file is the content, git is the history [R11 §4.2]. Everything is text-first by construction, so 11.3 snapshots carry memory as dumps + files with zero conversion [XREF:S13].

Personal-memory rights (S4.1): every person can list everything stored about them (their rows across scopes they own), correct it (a new human-direct version), and remove it — including true deletion (S09.5). Per-user knowledge *directories* are the physical-partition bonus; per-user *databases* are rejected — isolation is structural via the sole-writer control plane and server-side scoping, and cross-scope features (adoption, house, project truth) need one store [R11 §4.8; XREF:S02].

### S09.3 Storage, retrieval & isolation — deterministic-first (8.4 storage side, 10.1)

**Selection into runs is not a memory-system feature; it is [S05]'s registry-keyed injection.** The control plane selects knowledge slices by pure lookup over platform-owned facts (task record, project registry key, requester identity, worker template + overlay), within per-scope budgets, every item manifested [XREF:S05; R11 §4.3]. The load-bearing invariant, restated from the ratified package because this section is its home: **no agent-supplied identifier ever selects memory** [G2 D2.1; R11 §4.8]. Free text and engine output never key a lookup; same task + same registry state ⇒ same slice.

Within-run knowledge access has exactly two paths:
1. **Native grep over mounted slices.** The injected knowledge files sit read-only in the workspace under the run's confinement class [XREF:S11]; an engine grepping them is workspace file access, already manifested at placement [XREF:S05].
2. **`knowledge_search`** — one platform tool backed by **SQLite FTS5** over entry content. The adapter injects the run id; the control plane derives owner + project and appends the scope predicate **server-side** — scope is never a caller parameter [R11 §4.3, §4.8]. No embeddings, no similarity ranking, no index pipeline at v0: assembly stays reproducible, auditable, cache-stable [R11 §2.2, §4.3]. The query path lives in one module, and a standing conformance test *attempts* cross-user reads and must fail [R11 §4.8; XREF:S14].

Isolation, layered so leakage requires multiple independent failures [R11 §4.8; 10.1]: server-side scope resolution from `runs.owner_user` (the forgotten-namespace leak class is structurally absent) → server-bound search tool → per-user directories and per-user sandbox mounts exposing only the owner's slices [XREF:S11] → write-side gating (S09.1) → scope + origin labels on every injected item in the assembly frame [XREF:S05].

**Vector retrieval sits behind a pre-registered gate** [G2 Def.8]. Every `knowledge_search` miss and every "knowledge existed but was not injected" incident found in retro or verification is logged as a **retrieval-miss event** (metric definition owned by the eval practice [XREF:S14]). Evaluating an embedding lane (local model + sqlite-vec, RRF-fused with FTS5) becomes admissible only when, after the 15.3 benchmark gate, lexical misses affect ≥ ⚙ `memory.vector_gate.task_miss_rate` of tasks or the corpus exceeds ⚙ `memory.vector_gate.corpus_entries`. If ever adopted, **embeddings only rank candidates for trace-manifested selection — they never silently inject** [G2 Def.8; XREF:S05 Deferred].

### S09.4 The gated learning pipeline (8.1–8.3, S4.7) — architecture now, activation split v0/v1

The pipeline is five stations; its full architecture is normative now, and its automated stations activate at v1 [G2 D2.7; 15.4]. The governing rule at every station: **proposed from real outcomes, adopted only with approval** (8.1) — nothing skips station 3, including "obvious" fixes [R11 §4.4].

1. **Evidence collection (deterministic, free).** The event log and review machinery already capture the signals: requester edits (accepted-diff vs draft), review comments, accept/reject verdicts, verification findings, retro notes, task-end L0 content (`learned_this_task`, harvested engine memory dirs per S09.9). No new instrumentation [R11 §4.4; XREF:S02, XREF:S14].
2. **Proposal drafting (local tier, costs nobody allowance [XREF:S12]).** A local-model pass drafts lesson proposals from evidence under the quality rules: **a LESSON requires a correction** (something went wrong and was fixed — no invented insights); **a WIN requires an accepted outcome**; drafts carry their evidence refs verbatim. Cap ⚙ `memory.proposals_per_task_max`; batched into risk-ranked review sets (digest every ⚙ `memory.digest_interval_days` + on-demand); one-keystroke dismissal [R11 §4.4; G2 Def.7].
3. **The gate.** Owners approve own-scope entries; the project owner approves project entries; **house promotion requires the operator** [D10; 10.x]. Approval may edit or regenerate the draft. Inbox mechanics and risk tiers are S3.2's [XREF:S15]; own-store writes are tier Medium.
4. **Versioned adoption.** Approval writes the entry — row plus file commit where file-backed — with full provenance (S09.5). Supersession is a new version carrying `supersedes_id`, never in-place mutation; LLM auto-reconciliation of existing entries (auto UPDATE/DELETE) is banned [R11 §4.4, §5].
5. **Verification lifecycle & distillation (8.3).** Behavior-steering entries carry re-verify intervals (S09.8); expiry raises a review card, never silent death; anyone can flag an entry stale; `last_injected_at` identifies dead weight for pruning proposals. When ≥ ⚙ `memory.distill_threshold_lessons` lessons share a `topic_key`, a distillation proposal offers a playbook delta plus retirement of the constituents — itemized delta updates with the human gate, distill-then-delete [R11 §4.4; G2 Def.7].

**Activation boundary** [G2 D2.7; R11 §7.4; 15.4] — the schema ships whole at v0; what runs differs:

| Capability | v0 | v1 (15.4) |
|---|---|---|
| Full schema — all layers × scopes, owner-attributed (15.6) | live | — |
| Knowledge injection: **house / project / user** scopes | live | adds worker-overlay scope (with L1) |
| L0 scratch | live (discarded after run summary [XREF:S05]) | becomes the station-1 evidence feed |
| **Manual L2** — human-direct entries via workspace UI [XREF:S15] or file import; N20 house-KB day-one import (operator import = the approval) [R11 §4.2] | live | unchanged |
| `knowledge_search` + grep over mounted slices; per-scope budgets; removal, true deletion, influence tracing; precedence labels + conflict cards | live | unchanged |
| L1 writers (distiller, worker observation notes) | schema only — no writer active | live |
| Stations 1–2 (evidence harvest → drafting), digest | dormant | live |
| Station-5 automation (re-verify cards, staleness flags, distillation, pruning proposals) | dormant — operator curates manually; `verified_at` recorded so intervals apply retroactively at activation | live |
| Sharing surfaces — browse/adopt UI (S09.6) | schema live; moot single-user | live with household onboarding |

The v0 surface therefore exercises the full injection path end-to-end on human-written knowledge while the evidence stream accumulates in stores that exist anyway — exactly the ratified rationale [G2 D2.7]. Pipeline activation at v1 is a config flip plus UI, not a schema change.

### S09.5 Provenance, influence tracing & removal (8.2, S4.8, S2.9)

Every L2 entry carries: **origin** (`proposed_from` evidence refs | `human_direct` | `adopted_from` entry + user | `imported`), proposer (model + version if drafted), approver + timestamp, version chain, git commit hash where file-backed [R11 §4.5]. S2.9's forward trace is a **join, not a system**: `influence(entry) = runs whose trace manifest contains it` [XREF:S05] — every injection already logs `{item_id, content_hash, version, selector_rule, precedence_label}` per assembly.

- **Removal** (any L2 entry, individually): status → `removed`; the entry leaves **every future assembly immediately** — deterministic selection makes removal exact, which is why retrieval-side memory is the reversibility mechanism [R11 §2.5, §4.5]. Content stays in git history and the row stays for audit. On removal the platform surfaces the influence list (runs, verdicts, receipts) and offers to queue re-verification of still-live deliverables it touched.
- **True deletion** (owner's right, S4.1): content purged — file purge from the working tree plus DB content purge via the `VACUUM INTO` snapshot path [XREF:S02] — and a **tombstone row** retained: id, dates, "removed at owner request" [G2 Def.9]. Git-history purge of the owner's knowledge dir is a [XREF:S13] operation.
- **Honest limits, stated in the UI:** accepted artifacts already shaped by a removed lesson remain git history (D9), and unlearning-from-weights is never needed because **no fine-tuning exists anywhere in the learning loop — lessons live only as retrievable, injectable, removable text.** Standing platform invariant [R11 §4.5, §5].

### S09.6 Sharing by choice & one shared project truth (8.7, 8.8, S4.4, S4.9)

**Individual by default.** A lesson learned on a worker applies to that user's overlay on that template unless deliberately generalized; a personal entry reaches another person only by **adoption**: members can browse what exists (titles + provenance, per visibility rules), and adopting **copies** the entry into the adopter's own scope with `origin` visible — never a live shared reference, so later edits or poisoned updates cannot propagate silently [R11 §4.2, §4.8]. Project knowledge is shared among that project's members by scope membership; **house changes require the operator** [D10]. Single-user v0 makes the surfaces moot; semantics and schema are day-one (15.6).

**One shared project truth (8.7/S4.4)** is DB-backed current state, not a synchronization protocol: project-scope entries and task state live in `platform.db` behind the single serialization point (sole-writer control plane) with **optimistic versioning** — an update carries the expected version; mismatch → reject-and-retry or a question card. No CRDTs, no event bus [R11 §4.7]. "Immediately the truth for all" composes from ratified machinery: every stage assembles fresh from current state [XREF:S05], so updates are picked up at the next stage boundary; for long stages, a project-knowledge update emits the same freshness-event class as sibling-accept, routing active runs through the 4.3 pass [G1 Def.5; XREF:S02]. Knowledge updates are not artifact writes and never collide with workspace claims [XREF:S02]; but a plan whose assumptions cite project-truth entries records those **entry versions in its freshness fingerprint** — a ratified extension of the [G1 Def.5] set [R11 §4.7; G2 D2.1] (composition note: S02.6 lists the base set; this line adds cited-entry versions to it [XREF:S02]).

### S09.7 Precedence & conflicts (8.9, S4.11)

The total order is fixed: **explicit task specification > project truth > personal (user + overlay) > house defaults.** Precedence is **data, not prose**: every injected item carries its precedence label in the assembly frame, and the frame states the order once — [S05.4] is the enforcement point [XREF:S05; R11 §4.6]. Engine tie-breaking is documented to be arbitrary; Sinet never leaves a conflict to it [R11 §2.6].

Detection of *substantive* conflicts is a write-time duty, cheapest at this corpus size: on adoption/edit/approval the control plane looks up same-`topic_key` entries across the scopes the writer can see, plus an advisory local-model contradiction screen (duty alias [XREF:S12]; precision/recall measured at TBD-BRINGUP(contradiction-screen P/R) [G3 Def.8]) **[NARROWED A6, 2026-07-23: shipped one-stage workhorse shape P=R=1.0 on the synthetic seed; the two-stage DeBERTa pre-screen re-enters when a serving path exists]**. Hits create a `conflicts_with` edge and a **question card** to the affected owner — surfaced, never silently resolved. At assembly time, a known-conflicting pair entering one frame is flagged in the trace manifest and the question re-raised if unresolved [R11 §4.6].

### S09.8 Forgetting by design (S4.10) — lifecycle duties and injection budgets

Expiry, pruning, and supersession are **scheduled platform duties** on the ratified scheduler [XREF:S10], not accidents; the memory stays curated rather than growing into a landfill [R11 §4.10]. All numbers ship as operator-editable settings [G2 Def.7; G1 rider 1]; station-5-dependent rows activate at v1 (S09.4).

| Layer / kind | Mechanism | Default |
|---|---|---|
| L0 scratch | deleted with the task workspace (GC [XREF:S02]); ledger `learned_this_task` per S05 row 6 | task end |
| L1 observations | TTL expiry + distill-then-delete (consolidation proposes distillations → gate; residue deleted) | ⚙ 90 d |
| L2 lessons | never TTL-deleted; re-verify expiry → review card; `last_injected_at` staleness → pruning proposal | re-verify ⚙ 90 d |
| L2 preferences | no interval unless flagged by a human | flagged-only |
| L2 house playbooks / rubrics / taxonomies / trigger rules | re-verify expiry → operator card; superseded versions retired, never deleted | re-verify ⚙ 6 mo |
| Removed / retired entries | out of assembly instantly; audit row + git history retained; true deletion on owner request | — |

**Per-scope injection budgets** are the anti-landfill enforcement inside [S05]'s stage-fit target: injected knowledge per stage brief is capped per scope (⚙ `memory.injection_budget_tokens.*`). Over budget, assembly drops by deterministic priority — lowest selector specificity first, then oldest `last_injected_at` `[coordinator-draft]` — records every dropped item in the trace manifest as `over_budget_dropped`, and raises a curation card to the scope owner (v1: a distillation proposal instead). **Silent truncation is banned**; budget pressure converts knowledge bloat into a visible curation task [R11 §4.10, §7.10; G2 Def.7]. Enforcement is conformance-tested [XREF:S14].

### S09.9 Engine-native memory: containment posture (the 8.1-bypass problem)

Engine memory features are never the memory system [R11 §4.9; G2 D2.1]. An engine-native store that persists agent-written state across sessions is, structurally, an ungated L2 — an 8.1 bypass — so the posture is disable-or-contain, implemented through the compiled-config guarantee: Sinet-compiled config is the only config an engine sees, per-channel [SPIKE P2-S5; XREF:S03].

- **Claude lane:** auto-memory (MEMORY.md-class) is **disabled where the pinned version allows; otherwise contained** — the memory dir is workspace-scoped, treated as L0, wiped with the task workspace; from v1, its task-end content is *harvested* into the station-1 evidence pool. It never persists as behavior-steering memory. The GA memory tool stays off at v0. Exact disable/redirect mechanics: settled — **closed A2 (2026-07-22, P3-B3-1)**: config-root `memory/` wipe at session start, resume exempt; carried identically in S03 [XREF:S03].
- **Z.AI lane (opencode):** no native auto-memory on the pinned version; community memory plugins are not adopted (adopt-don't-fork; young third-party code) [R11 §4.9].
- **Convention files** (AGENTS.md/CLAUDE.md) remain a *projection* surface generated from the registry — never a store [XREF:S05].

**Drift is the standing risk (P-T10-1):** a pinned-version bump can ship new engine memory behavior that silently persists agent-written state. Engine memory behavior is therefore a standing **canary-suite entry**, re-checked on every engine pin bump with this posture re-applied [XREF:S14; XREF:S03; G2 follow-ups].

### S09.10 Knowledge objects declared by other sections — one governance regime

Every knowledge object another section declares **is an L2 entry at house or project scope under this section's governance**: versioned, attributable, individually removable, entering through the 8.3 knowledge gate (operator approval at house scope, D10), stored per S09.2 (row + git-versioned file), injected only through [S05] with manifest entries, and subject to the S09.8 lifecycle. The owning section defines the content and its consumers; S09 defines how it lives, changes, and dies [5.7; R11 §4.2].

| Object | Declared in | Scope | Notes |
|---|---|---|---|
| Interview must-know taxonomies (per task family) | [XREF:S06] S06.5 | house | seed drafting is TBD-P3 per S06, 8.3-gated on entry; slot weights operator-editable |
| P47 research-trigger rule file (`intake/p47-triggers`) | [XREF:S06] S06.3 | house | kind `trigger_rules`; v0 changes = manual operator edits; miss-driven additions ride the v1 pipeline |
| Verification rubrics (per domain) + reference exemplars | [XREF:S07]; 5.7, 8.3 | house (rubrics), house/project (exemplars) | rubric changes are 8.3-gated and benchmark-falsifiable (11.2 [XREF:S14]); exemplar promotion records which rubric criteria the exemplar evidences [R11 §6 N14] |
| Composer best-practice playbook (worker drafting) | [XREF:S08]; R15 | house | versioned; composer reads the current approved version; edits operator-gated |
| House KB day-one content (N20 import) | this section | house | operator import = the approval; each document gets selectors, `topic_key`, size budget on entry [R11 §6 N20] |

A knowledge object's removal or supersession follows S09.5 exactly — including influence tracing over the runs it was injected into. No object in this table has a private update path around the gate.

**Settings introduced (⚙):** — all operator-editable with audit trail; auto-adjust only within operator ceilings, visible on receipts [G1 rider 1]

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `memory.l1_ttl_days` | 90 | 7–365 | [G2 Def.7] |
| `memory.reverify_lessons_days` | 90 | 30–365 | [G2 Def.7] |
| `memory.reverify_house_days` | 180 (6 mo) | 90–730 | [G2 Def.7] |
| `memory.proposals_per_task_max` | 2 | 0–5 | [G2 Def.7] |
| `memory.digest_interval_days` | 7 | 1–30 | [G2 Def.7] |
| `memory.distill_threshold_lessons` | 3 | 2–10 | [G2 Def.7] |
| `memory.injection_budget_tokens.house` | 2,000 | 500–10,000 | budgets ratified [G2 Def.7]; numbers `[coordinator-draft]` |
| `memory.injection_budget_tokens.project` | 3,000 | 500–10,000 | as above |
| `memory.injection_budget_tokens.user` | 1,500 | 500–10,000 | as above |
| `memory.injection_budget_tokens.worker_overlay` | 1,500 | 500–10,000 | as above (scope activates v1) |
| `memory.vector_gate.task_miss_rate` | 0.05 | 0.01–0.25; pre-registered trigger — evaluated post-15.3 | [G2 Def.8] |
| `memory.vector_gate.corpus_entries` | 5,000 | 1,000–50,000; pre-registered trigger | [G2 Def.8] |

**Known problems owned here:** *(ids assigned in R11 §7.8–7.10 order, per the P-T##-N convention; S17 consolidates)*
- **P-T10-1 — engine-native memory drift (8.1 bypass class):** disable-or-contain posture in S09.9; standing canary-suite entry re-checked per engine pin bump [XREF:S14]; mechanics settled — closed A2 (2026-07-22, P3-B3-1).
- **P-T10-2 — proposal-noise economics unmeasured field-wide:** accept/dismiss rates instrumented from the pipeline's first day (v1); sustained low acceptance is a station-2 defect proposing drafter/cap retune, never a reason to weaken the gate; metrics owned by the eval practice [XREF:S14], reviewed at 11.2 checkpoints.
- **P-T10-3 — knowledge-injection budget pressure:** per-scope budgets with visible over-budget handling (S09.8) inside [S05]'s stage-fit target; conformance-level enforcement [XREF:S14].

**Deferred / parked:**
- Auto-proposal pipeline (stations 1–2, 5 automation; L1 writers; worker-overlay injection; sharing UI) → activates at v1 per [G2 D2.7]/15.4; architecture fixed here (S09.4), activation is config + UI.
- Embedding lane (local model + sqlite-vec, RRF with FTS5) → re-entry: a `memory.vector_gate.*` trigger fires post-15.3; rank-candidates-only rule binds regardless [G2 Def.8].
- Claude-lane auto-memory disable/redirect mechanics → settled, closed A2 (2026-07-22, P3-B3-1) (rode the adapter+memory builds) [XREF:S03].
- Engine-native cross-session memory absorbing L0/L1 mechanics → re-entry: configurable, gateable primitives on *both* lanes behind the Sinet contract; the gate and L2 stay Sinet-owned regardless [R11 §4 change-triggers].
- Graph/temporal supersession store → re-entry: a real household-scale temporal-supersession need; met first by `supersedes_id` + status fields in plain SQL [R11 §4 change-trigger 3].
- Memory-sidecar re-evaluation → re-entry: a system shipping schema-level write authorization + human-gated writes + text-first storage as first-class features; none exists mid-2026 [R11 §4 change-trigger 2].
- FTS5-only search + weekly digest cadence revisit → re-entry: household scale-out, corpus ≫ thousands of entries [R11 §4 change-trigger 5].

**Coverage:**

| Feature-list item | Where |
|---|---|
| 8.1 permissioned learning | S09.4 |
| 8.2 traceable, reversible learning | S09.5 |
| 8.3 distillation into durable knowledge | S09.4 (station 5), S09.10 |
| 8.4 / S4.6 house knowledge flows in (storage side; assembly [XREF:S05]) | S09.2, S09.3 |
| 8.5 / S4.1 personal memory + owner rights | S09.2, S09.5 |
| 8.6 / S4.2 / S4.3 layered lifetimes, defined writers | S09.1 |
| 8.7 / S4.4 one shared project truth | S09.6 |
| 8.8 / S4.9 sharing by choice, origin visible | S09.6 |
| 8.9 / S4.11 declared precedence, surfaced conflicts | S09.7 |
| S4.5 house knowledge base (curated, versioned, operator-gated) | S09.2, S09.10 |
| S4.7 outcome memory: wins and lessons | S09.4 |
| S4.8 / S2.9 reversible, attributable, auditable learning | S09.5 |
| S4.10 forgetting is a feature | S09.8 |
| 7.8 (memory aspects of overlay/instance; structure [XREF:S08]) | S09.1, S09.2 |
| 10.1 (memory-isolation slice) | S09.1, S09.3 |
| 11.3 (text-first memory stores; snapshot mechanics [XREF:S13]) | S09.2 |
| 15.6 multi-user data model day one | S09.2, S09.4 (activation table) |

**Open items for G4:** none. Drafting-time sub-choices are carried inline as `[coordinator-draft]` tags for G4 attention: injection-budget default numbers, the over-budget drop order, table/kind naming (incl. the `taxonomy`/`trigger_rules` kinds). One cross-section composition note flagged to the coordinator: S02.6's fingerprint set gains cited project-truth entry versions per the ratified R11 §4.7 (S09.6).

## S10 — Metering, budgets, scheduling & limit events

**Scope:** The consumption layer beneath every run — the exact usage ledger (one row per paid call at the D7 checkpoint), the two-currency model (consumption pressure vs the effective-dated price table), per-person automation budgets and interactive headroom, the five-class limit-event taxonomy that drives retry/park/freeze, effort modes as disclosed depletion ladders, the one SQLite run scheduler (priorities, cache-window-aware resumes, missed-slot policy), per-run hard ceilings, local-resource arbitration policy, honest receipts including the done-directly figure, and never-fully-offline behavior — specifying **how** the platform meters exactly, reacts to (never models) provider limits, and schedules everything so automation never starves a person's interactive use.

**Binding inputs:** R09 §1–7 (primary) [G2 D2.1]; **D4, D5, D7**; feature-list 3.1–3.11, 2.8, S2.5, 3.4; G1 P1/P7, D1.7, Def.10, riders 1 & 3; G2 D2.1, D2.4, D2.8, Def.4/5/6/16; G3 D3.1; `Spec/benchmark-preregistration-v1.md` §13 (done-directly formula — cited, numbers never restated); [SPIKE P2-S1] Z.AI calibration + logprob; [SPIKE P2-S3] `anthropic-ratelimit-*` inventory (R09-OQ6); [SPIKE P2-S4] serialize-by-deny cost; [SPIKE G1-S1/S2/S3] usage/park/ceiling wire facts; P-T08-1..5; P-T17-1, P-T01-3, P-T02-2 (shared). Boundary siblings: [XREF:S01] units/slices/timers · [XREF:S02] checkpoint/FSM/queue storage · [XREF:S03] wire mechanics & engine knobs · [XREF:S06] intake cost-gate · [XREF:S08] worker/routing ceiling defaults · [XREF:S11] credential-injection proxy · [XREF:S12] GPU/local internals · [XREF:S14] watchdog alarms & benchmark machinery · [XREF:S15] meter/receipt surfaces · [XREF:S19] v1 schedule/trigger boundary.

The vocabulary of this section: **usage row** = the single measured record written for one paid model call, carrying usage, pricing, purpose, and approximation tier; **approximation tier** = the 1–5 honesty rank of how a row's consumption was obtained (measured → unknown), stamped on every row and shown on receipts; **consumption-pressure gauge** = the per-(person, lane) instrument reading weighted consumption against the operator-declared automation budget (the D5 flat-rate currency, extending the glossary's *consumption pressure*); **limit-event class** = one of the five wire-observable failure kinds (§S10.5) that selects a retry/park/freeze action; **workload class** = a run's scheduling priority band (interactive / human-blocked resume / scheduled / background / probe). These five terms are coined here.

### S10.1 The consumption ledger (3.1, 3.4, D4)

The platform meters its **own** consumption exactly and never trusts a provider to report remaining quota (D4). One **usage row** is written **per paid model call, in the same transaction as its D7 checkpoint** [R09 §4.1; D7; S02.4a] — the checkpoint's usage block *is* the ledger row; the `receipts`/usage table family materializes them per run-end [S02.2]. Storage is S02's; the row's content, pricing, and approximation semantics are specified here.

**Usage row (indicative fields; exact DDL is the P3 schema workshop's, jointly with S02):**

| Group | Fields |
|---|---|
| identity | `run_id, task_id, requester (D2 owner), owner_lane, substrate, model, ts` |
| usage | `input, output, cache_read, cache_creation (+ TTL buckets), thinking_tokens, server_tool_use{count}` — stored pass-through [R09 §2.1; SPIKE G1-S1 F1] |
| pricing | `engine_cost_estimate` (cross-check only), `priced_cost{table_version, effective_date}` (§S10.3), `currency ∈ {api-equivalent, real}` |
| classification | `purpose_tag ∈ {ceremony, execution, verification, probe}`, `approximation_tier ∈ {1..5}` |

**Metering rules (normative):**
- **Accounting math is encoded, not assumed.** On the Anthropic surface `input_tokens` counts only tokens after the last cache breakpoint — total prompt = `cache_read + cache_creation + input_tokens` [R09 §2.1]; thinking tokens are billed as output; server tools (web search/fetch) are priced per use, outside token math [R09 §2.6]; the batch flag halves token prices. A row that fails to normalize these double- or under-counts (P-T08-1).
- **Dedup parallel-tool-call messages by message `id`** — one message id can emit multiple assistant messages with identical usage [R09 §2.1]. Engine-native subagents are disabled (sole-controller rider [G1 rider 2; S03.5]), which removes the nested-`usage`-undercount hazard, but the `modelUsage`/`total_cost_usd` cross-check and the dedup rule still apply.
- **Engine dollars are cross-checks, never billing truth.** `total_cost_usd`/`costUSD` are vendor-labeled client-side estimates with a live ~10× bug record [R09 §2.1/§2.6]; they are stored and cross-checked, never authoritative. Sinet prices from its own table (§S10.3, D5).
- **No row silently prices to $0.** An unpriceable row renders **UNPRICED** on the receipt and raises a drift alert; the LiteLLM silent-$0 incident is the cautionary record [R09 §2.1]. This is the load-bearing guard against the no-load-bearing-metered-paths rule at the metering layer.

**Aggregations** per person × model × task × day/week/month are **queries over the ledger, not new state** [R09 §4.1]. Every run **bills its requester** (3.4), ceremony itemized separately from execution (1.10); the `/usage` per-capability-percentage breakdown is the presentation model [R09 §2.6; XREF:S15]. Trigger-/webhook-registered tasks bill the **registrant** — a v1 surface, since triggers are v1 [3.4; XREF:S19].

**The honest approximation hierarchy** (best-first; every row labeled) [R09 §2.1]:

1. **Measured per-call usage** from the substrate stream/DB — both v0 subscription lanes and local, the normal case; exact on every surface, confirmed live incl. per-message cache detail [SPIKE P2-S1 §Impl-metering-1].
2. **Engine-computed aggregates** — cross-check tier; alarm when tier-1 × price table diverges from the engine estimate past `⚙ meter.value_divergence_alarm` (catches both price staleness and engine bugs; machinery [XREF:S14]).
3. **Derived plan units** — the Z.AI GLM Coding plan is denominated in "prompts" (~15–20 model calls each) but exposes **no per-prompt counter, no usage endpoint, and no rate-limit headers on the wire** (all `…/usage` variants 404; response body carries only per-request token `usage`) [SPIKE P2-S1 Probe 7; S03.7]. Prompt-unit consumption therefore stays **tier-3 derived** (requests-as-prompt-proxy with documented peak 3× / off-peak 2× multipliers applied as data), reconcilable only against the operator dashboard by `request_id` — the calibration recipe is `TBD-OPERATOR(Z.AI dashboard prompt-unit calibration)`, the 5-step dashboard procedure [SPIKE P2-S1 §Blocked].
4. **Tokenizer estimates** — pre-run cost gates only (the 2.5 size guess and the $0.50 zero-interaction band [G1 D1.7; XREF:S06]), never a ledger fact.
5. **Unknown** — flagged UNPRICED (tier-5), never a silent zero (P-T08-1).

### S10.2 Two currencies (D5, 3.10)

Between flat-rate options, scheduling and routing use **consumption pressure**, never dollars (marginal cost is zero there); **dollars are the reporting currency** and a routing input only for explicitly enabled metered use [D5].

- **Flat/metered is declared per model by each user** (3.1) as a data flag on the price table (§S10.3); a flip is a rehearsed data change with visibly changing receipt currency, not an architecture change [P-T01-3; S03.3].
- **Metered spending is opt-in only** (3.10): per-token billing is enterable only by an explicit per-use flag — never a default, never a silent fallback [14.3]. The **metered-exception list is EMPTY at v0** [G1 P7]; DeepSeek is the pre-registered designated exception should one ever be enabled [S03.6]. A model whose `overflow_mode` is `auto-metered` without a proven disable/zero-balance is rejected at onboarding [S03.6; 3.10].
- **Local lane has no pressure and no budget** — it is arbitrated, not budgeted (§S10.9); its rows price at $0 with utilization noted [R09 §4.2].

Dollar-based routing *between* flat-rate lanes is a **D5 violation and NEVER done** — marginal dollars are zero on both v0 subscription lanes, so any dollar-ranked choice between them is noise [R09 §5].

### S10.3 The price table (3.1, D5) — user-maintained, effective-dated

Dollars come only from an **effective-dated price table** [D5; R09 §4.6]. Shipped defaults are the **pinned genai-prices `data.json`, vendored** [G2 Def.16; D2.2 Layer B] — this **supersedes R09 §2.6's models.dev lean** (the gate examined the price-table area report 09 left as a follow-up and ratified genai-prices); models.dev and the LiteLLM map are retained only as CI cross-checks. Semantics:

- **Rows carry `{model, lane, unit_prices…, effective_from, verified_on, source}` with future-dated rows first-class** — a table without effective dates is *guaranteed* wrong on a known future date (e.g., a pre-announced list-price flip), and a promotional "limited-time free" cache rate is the same shape unbounded (P-T08-3). The provider-watch cadence diffs announced *future* prices, not only current ones [S03.6; XREF:S14].
- **Never price a flat-rate lane's usage from a $0 flat-rate row.** The vendored defaults must be reconciled at ship so Z.AI usage prices API-equivalent from the **metered per-token `zai` rows**, never the zero-rate coding-plan rows, which would render every receipt $0 [SPIKE P2-S1 §Impl-metering-3; R09 §4.6]. Never meter dollars from opencode (it prices the coding plan at $0).
- **Never quote aggregator/cheapest-host prices** — the D5 table quotes the provider actually serving the lane, from primary pages or reconciled genai-prices rows [R09 §5].
- **User-editable per 13.4.** Refresh lands as a **proposal, never an overwrite**; the user overlay is never clobbered [G2 Def.16]. **Price-table drift is a first-class staleness cause** — it re-prices the remaining plan, so any change fires the freshness re-validation trigger [G1 Def.5; S02.6].

### S10.4 Consumption pressure, budgets & interactive headroom (3.3, D5)

The **consumption-pressure gauge** per (person, lane) = weighted consumption ÷ the **operator-declared automation budget** for a Sinet-defined period, overlaid with provider-signaled observed events [R09 §4.2, §3-D2]. This is D4-clean: the denominator is Sinet's own budget (a 3.3 construct the operator edits, a 13.4 setting), **never an inferred provider window**; reset times come only from provider signals (§S10.5).

- **Weighting.** Anthropic lane: tokens with cache-read weighted `⚙ pressure.cache_read_weight = 0.1×`, labeled **"assumed"** until subscription quota semantics are published [G1 Def.10]. This is the single most impactful gauge assumption on cache-heavy work — live sampling saw cache reads dominate (≈8320/8668 tokens on one Z.AI turn) [SPIKE P2-S1 §Impl-metering-1] — and the gauge inherits it (P-T08-5). Z.AI lane: requests-as-prompt-proxy with the documented multipliers applied as data (tier-3). Every gauge reading carries its approximation tier.
- **Budget denominators seed from plan marketing shape** at a conservative fraction — e.g., the GLM Coding Max ~1,600 prompts/5 h shape [G1 D1.5] — labeled **"assumed"**, which fires sooner than a calibration week and stays honest [G2 Def.5]. Background budgets default to `⚙ budget.background_window_fraction = 0.5` (≤50% of the advertised window, "assumed") [G2 Def.4/5].
- **Headroom rule (SRE-shaped).** Interactive use is CRITICAL_PLUS — **the platform's own consumption NEVER blocks a person's interactive use** (3.3); background classes shed lowest-first as pressure rises, and **background admission stops at `⚙ pressure.bg_admit_stop = 0.7`**, leaving the remainder as interactive headroom (aligned with the ratified 70% stage-overflow event) [G2 Def.4; R09 §4.4].
- **Observed-state overlay (D4-clean, measured).** On the Anthropic subscription wire the model-egress request returns the **`anthropic-ratelimit-unified-*` family (16 headers: 5h / 7d / 7d_oi × reset/utilization/status, plus overage/fallback fields)** — the classic per-request/token buckets are **absent** on the subscription surface [SPIKE P2-S3 §4; R09-OQ6 confirmed]. These are provider-*signaled* facts (the provider telling us, not us modeling), harvested at the **credential-injection proxy** [XREF:S11] — one choke point, two purposes, mirroring the D7 "one write, two purposes" shape. They (a) supply Class-2 park resume times (`-reset`, §S10.5), (b) enrich the fleet/consumption meters as an observed overlay (`-utilization`, `-status`, `-overage-status`) [XREF:S15], and (c) feed the P-T08-5 quarterly cache-weight calibration. The **denominator stays the operator budget** regardless — utilization headers are overlay and cross-check, never the pressure denominator (D4). Z.AI exposes no such headers; its limit state rides error-code bodies (§S10.5) [SPIKE P2-S3 §4].
- **Enforcement is two-gated** [R09 §4.4; C2]: a **spawn gate** (estimated cost vs remaining budget + pressure band; the $0.50 zero-interaction band rides this estimate [G1 D1.7; XREF:S06]) and a **per-checkpoint gate** (a breach parks the run `blocked_budget` at the checkpoint — the finest granularity that exists anywhere — resuming when the period rolls or the owner raises the budget; both are inbox cards). Admission against a post-call-updated counter is *not* a hard ceiling — the per-checkpoint gate is what makes budgets bind (§S10.8). Notification-only budgets are NEVER used — a budget that only emails is not a budget [R09 §5].
- **One-switch pause-my-automation** stops the person's background admission and parks in-flight runs at their next checkpoint (halt-with-finish-current-stage option), and **MUST preserve everything queued and parked**. Destroying parked/queued work on pause is the named anti-behavior with a mandatory test (P-T08-4) [R09 §4.4/§5].

### S10.5 The five-class limit-event taxonomy (3.2, D4)

Limit events are **normal, recoverable scheduling events**, not errors (D4). The wire's signals sort into five **limit-event classes**; each selects a fixed action. **The classifier is a TESTED component** with per-lane fixtures — budget, policy, and auth events masquerade as one another on the wire (budget-as-HTTP-401, policy-ban on valid credentials, Z.AI 1113 on endpoint misconfig), and misclassifying an auth event as depletion (retry-parking a revoked lane) is the named worst case (P-T08-2) [R09 §2.2/§4.3; XREF:S14]. Adapters forward the raw signals as data [S03.1]; the taxonomy and scheduling live here.

| Class | Wire signals (per lane) | Action |
|---|---|---|
| **1 Transient shed** | Anthropic 529 / subscriber transient-429; Z.AI 1302/1305 | **Retry in place**: full jitter, per-request cap `⚙ limit.retry_cap = 3`, per-lane retry budget `⚙ limit.retry_budget_ratio = 0.10`, circuit breaker on repeat [R09 §4.3]. Never park; never count against quota. |
| **2 Depletion + signal** | `rate_limit_event.resetsAt`; `anthropic-ratelimit-unified-*-reset` [P2-S3]; Z.AI 1308/1310 `next_flush_time`; opencode `retry.next` | Park **`blocked_quota`** at checkpoint with the **provider-signaled** resume time; auto-resume; prefer resuming inside the 1 h cache window when pressure allows (§S10.7). |
| **3 Depletion − signal** | Z.AI 1113 (after endpoint self-check); undocumented concurrency caps | Park with a jittered probe schedule (interval cap `⚙ limit.probe_interval_max = 30 min`); probe resumes are zero-cost and never count as attempts or spend [SPIKE G1-S2]. |
| **4 Auth / policy** | 401/402/403; policy-ban text on *valid* credentials | **Lane freeze + operator alert** via the P-T17-1 auth canary [S03.6]; **NEVER retry-park** — a policy event that dies in a retry loop is a platform defect (P-T08-2). |
| **5 Engine ceiling** | `error_max_budget_usd` / `budget_exhausted` | Died-at-gate handling — Sinet's own backstop tripping; the wrapper ask-record is authoritative [S02.3]; engine flags sized ≥2× so this class stays rare (§S10.8). |

**Park economics and discipline.** Parking is cheap and lossless on both lanes — defer-park exits carry the full pending call and poll-resumes cost $0 [SPIKE G1-S2 F1]; opencode sessions survive restart in SQLite, with ask-loss contained by Sinet's authoritative ask-record [SPIKE P2-S1 Probe 2; S03.4]. The run **parks at its checkpoint losing nothing and resumes without anyone babysitting** (3.2/3.8) — from the provider signal where available, else on the probe schedule. **Once-per-storm logging**: one drift/park card per storm per lane, not per attempt; interrupted verification rounds never count as rework rounds [R09 §6 (N3)]. Engine-native retry is NEVER the policy layer — Claude Code hard-stops on window exhaustion and opencode retries unboundedly at ~1 s; engines get minimal retry config and the wrapper classifies [R09 §5; S03.3].

### S10.6 Effort modes as disclosed depletion ladders (3.5)

Effort modes are policy bundles (duty-map model + vendor effort param + verification depth + retry allowance + lane/park preference), each a **depletion ladder whose steps are disclosed states** [R09 §4.5; D5]:

- **Eco** — an adequate result at the lowest consumption: smallest capable model, effort *low*, mandatory-only verification, no retries, off-hours placement. The Z.AI **`thinking:{type:disabled}` lever (~50× token reduction on trivial work) is an Eco/Balanced rung** on that lane [SPIKE P2-S1 Probe 7; S03.7].
- **Balanced** — the best result per unit of consumption: duty-map default model, effort *medium*, cheap-first verification cascade, one retry.
- **Smart** — the best result within the person's automation budgets: frontier lane, effort *high/max*, full two-axis verification, retries within ceilings, and **parking on depletion rather than degrading below its quality floor**.

**The downgrade ladder** — flat-lane switch by pressure → in-lane model demotion → effort-param reduction → local fallback (3.9) → park with resume time — moves **only with disclosure as state** (3.5 "downgrades gracefully **and says so**"): the task card shows the active mode/degradation like a Battery-Saver indicator, and the receipt carries a mode-change line (§S10.10). A **mode change is a visible state, not a log line** — the documented silent Opus→Sonnet fallback is the exact failure mode this prevents [R09 §2.5/§4.5]. Routing between flat lanes uses the pressure gauge, effort mode, and task classification — **never dollars, never keywords** [R09 §6 (N16); D5].

### S10.7 The one run scheduler (2.2, 2.8, 3.3)

A **single SQLite-backed run scheduler** in the control plane owns run admission, priority, park/resume timing, and budget gating [R09 §4.7; G2 D2.1]. It reads and claims the S02 `queue`/`lanes`/`slots` tables (CAS claiming, per-lane/per-model slot gates from *observed* concurrency, not documented promises); storage and claim mechanics are S02's [S02.2], the policy is here.

- **Priority ladder by workload class** [R09 §4.7]: **interactive > human-blocked resumes (answered gate/ask parks, resumes-due) > scheduled-due > background > probes**, with **aging so background never starves**. Interactive is never starved by automation (3.3).
- **Cache-window-aware resume timing.** An in-TTL resume outranks same-priority cold work — a cold resume costs ~6–16× a cached one, and the subscription cache TTL is ~1 h, which is why the `⚙ freshness.hold_vs_park = 10 min` default sits below that cliff [SPIKE G1-S1 F3; G1 Def.4; S02.6].
- **Missed-slot policy (Quartz-style vocabulary):** `run-once-late` (default — fire-once-now + coalesce) / `skip` / `notify-only` (a card, not a run) [R09 §4.7]. **At v0 the only schedules are platform-internal timers**, which run as systemd `Persistent=true` calendar timers owned by [XREF:S01] (snapshots, canaries, quarterly passes); the **user-facing recurring-schedule and event-trigger surface that consumes this policy is v1** [2.8, 15.4; XREF:S19]. `⚙ scheduler.missed_slot_default = run-once-late` ships now as the ratified default, activated at v1. systemd is NEVER the task-schedule authority (no per-schedule missed-slot policy, no billing integration, `WakeSystem` unavailable to user units) — it starts Sinet and enforces `RuntimeMaxSec` transient-scope ceilings only [R09 §5; S01.2].
- **The scheduler is needed even single-user** for limit events (parks/resumes/budgets) — it is v0 machinery regardless of the v1 schedule surface [S00.2].

### S10.8 Hard ceilings & anomaly-as-scheduling (3.7)

**No run can quietly burn a week's allowance** (3.7). Per-run **time / steps / cost ceilings are columns on `runs`** [S02.2]; their default values are worker/stakes-derived and operator-editable [XREF:S08; G1 rider 1]. Enforcement facts:

- **Ceiling granularity is one model call — everywhere.** A ledger check runs at every paid-call checkpoint (the finest granularity that exists); a run admitted under budget can still overshoot by up to one call. The live evidence is the **19× overshoot on a $0.001 cap** [R09 §2.4/§4.8; R05]. The platform ledger is the *real* ceiling.
- **Engine-side ceilings are backstops, ordered outside the platform ceiling.** The wrapped-CLI `--max-budget-usd`/`--max-turns` sit at `⚙ adapter.engine_ceiling_backstop_mult ≥ 2×` the platform remainder so a same-turn engine trip does not pre-empt a park before the ledger acts; ordering and mechanics are S03's [S03.4].
- **systemd PID-1 time ceilings are the outermost backstop** — run units carry `RuntimeMaxSec` and cgroup accounting as the time-ceiling backstop the ledger cannot provide mid-call [S01.2; G1 Def.6].
- **Loop and silence detection are scheduling actions, not kills.** A flagged run is **contained → parked at its checkpoint → surfaced as a card** (pause-and-flag, **NEVER auto-kill** — the Gemini-CLI/OpenHands false-positive record is the evidence base) [D1.3; R09 §4.8/§5]. The card always offers "resume, I was wrong." Detection thresholds (loop 5×, ping-pong 6, error-loop 3×, silence budgets) and abnormal-spend alarms are the watchdog's [G2 Def.10; XREF:S14]; the park response is the scheduler's.

Re-spend after any disaster is bounded *structurally* to "work since the last paid call" (D7) and *economically* by the cache TTL a warm resume rides — the disaster economics of 3.8, whose durable mechanism is [XREF:S02].

### S10.9 Local arbitration (3.11) & never fully offline (3.9)

Local GPU/VRAM, RAM, and CPU are shared between sandboxes, local inference, and the operator's own interactive use; the platform arbitrates them, and **the operator's interactive use always wins** (3.11) [R09 §4.9; G2 Def.6]. This section owns the **admission policy**; the systemd-slice mechanism is [XREF:S01] and the GPU broker / VRAM-ledger internals are [XREF:S12].

- **CPU/RAM/IO:** systemd slices in priority order — operator session (uresourced-style active-session memory guarantee + CPU boost) > `sinet-control` > local inference > sandbox batch at `⚙ arbitration.background_cpuweight = idle` with `MemoryHigh` fences; **PSI pressure triggers pause background admission** before the desktop notices [R09 §2.8/§4.9; mechanism XREF:S01].
- **GPU:** no partitioning exists on consumer NVIDIA (MIG datacenter-only; MPS is an opt-in allocation cap, not isolation) — serialization is at the inference server, and **Sinet admission-checks a local run against the VRAM ledger** (measured model footprints + measured compositor headroom) as the **POLICY hook** before dispatch; the ledger/broker mechanics are [XREF:S12] [R09 §4.9/§5]. Operator-wins at v0 = a **manual eager-unload switch + GameMode start/end hook**; idle-detection auto-pause is post-v0 novel work with no prior art [G2 Def.6].
- **Never fully offline (3.9).** With every paid allowance exhausted, **local-feasible work continues on local models and the rest parks with resume times** [R09 §4.9]. The local watchdog/intelligence floor's residency mechanics are [XREF:S12] — **R16 revised R09's always-resident-slot sketch** (an always-resident model and deep GPU sleep are mutually exclusive; the ratified shape is TTL-warm-on-AC / CPU-tier-on-battery, so the floor is never dark and never cloud). This section binds only the policy: exhaustion demotes to local, never to silence.

### S10.10 Honest receipts (3.6, S2.5)

Every job ends with an account of what it consumed [3.6]. Receipt content (the surface is [XREF:S15]):

- **Consumed units per model × purpose**, ceremony itemized separately from execution and verification (1.10/3.4) — the `/usage` per-capability breakdown is the pattern [R09 §2.6].
- **Currency = API-equivalent for flat-rate** (from the §S10.3 table, labeled), **real dollars for metered** — the currency **visibly flips** for any metered-flagged run (P-T01-3 rehearsed flip) [R09 §4.6].
- **Mode/degradation lines** (§S10.6) and **park history** ("parked 2 h 14 m, resumed on provider signal").
- **The done-directly figure** — the honesty keystone (3.6): "what this would have cost done directly, without the platform." It follows the **two-stage formula** (per-run heuristic → per-domain measured median once ≥10 benchmark pairs exist) [G2 D2.8], whose **registered, non-retunable text lives in `Spec/benchmark-preregistration-v1.md` §13** — this spec cites that form and **does not restate its numbers** [CONTRACT §binding-layer-3]. No shipped tool presents this counterfactual; it is Sinet's to compute against the registered formula [R09 §2.6]. Every altitude of cost (run/task/project/person/period) is observable, always including this figure [S2.5; XREF:S15].

---

**Settings introduced (⚙):** (all operator-editable with audit trail per G1 rider 1; auto-adjust only within operator ceilings; every number visible on receipts)

| ⚙ setting | default | clamp / range | ratified by |
|---|---|---|---|
| `pressure.cache_read_weight` | 0.1× | 0–1 (labeled "assumed") | G1 Def.10 |
| `pressure.bg_admit_stop` | 0.7 | 0.1 – 0.95 | G2 Def.4 |
| `budget.background_window_fraction` | 0.5 | 0 – 1 (labeled "assumed") | G2 Def.4/5 |
| `meter.value_divergence_alarm` | 20 % | 5 – 100 % | R09 §4.1 / P-T08-1 |
| `limit.retry_cap` | 3 | 1 – 5 | G2 D2.1 / R09 §4.3 |
| `limit.retry_budget_ratio` | 0.10 | 0.01 – 0.5 | G2 D2.1 / R09 §4.3 |
| `limit.probe_interval_max` | 30 min | 1 – 120 min | G2 D2.1 / R09 §4.3 |
| `scheduler.missed_slot_default` † | run-once-late | {run-once-late, skip, notify-only} | R09 §4.7 |
| `arbitration.background_cpuweight` ‡ | idle | systemd CPUWeight | R09 §4.9 |

† ships now as the ratified default; the user-facing schedule surface that consumes it activates at v1 [XREF:S19]. ‡ policy value; the slice mechanism is [XREF:S01].

Per-person **automation budgets** (per lane, per period) are operator-declared 13.4 settings seeded from plan marketing shape [G2 Def.5]; they are data, not a single default. Cross-cutting settings consumed here but owned elsewhere (deduped by [XREF:S18]): `freshness.hold_vs_park`/`freshness.max_age` [G1 Def.4/5; S02/S06], `adapter.engine_ceiling_backstop_mult` [S03], the $0.50 zero-interaction band [G1 D1.7; S06], per-run `runs.ceilings` [S02.2], `RuntimeMaxSec` [S01.2].

**Known problems owned here:**
- **P-T08-1** — engine usage/cost *values* drift and break, not just schemas (3–8× usage duplication, 10× cost inflation on the primary lane within 12 months) → ledger sanity bounds + tier-1-vs-tier-2 divergence alarm (`⚙ meter.value_divergence_alarm`); conformance suites assert *values* on known fixtures, not only field presence [XREF:S14]. Extends P-T01-2.
- **P-T08-2** — budget, policy, and auth events masquerade as each other on the wire → the five-class classifier (§S10.5) is a tested component with per-lane fixtures; misclassifying Class-4 as 2/3 (retry-parking a revoked lane) is the named worst case; auth-shaped → lane-freeze [S03.6].
- **P-T08-3** — prices carry effective dates, not just values → price-table `effective_from` + scheduled-change support; the watcher diffs announced future prices [S03.6; XREF:S14].
- **P-T08-4** — pause semantics can destroy deferred work (the Zapier drop-on-expiry shape) → "pause preserves queued + parked work" is a tested invariant of one-switch pause and maintenance mode.
- **P-T08-5** — the pressure gauge inherits the `⚙0.1×` cache-weighting assumption → keep the "assumed" label; add a quarterly calibration check using *observed depletion events* (never window modeling) — measured window exhaustion vs gauge prediction at exhaustion — and alarm on systematic divergence; joins P-T02-2's cache-fidelity alarm suite [XREF:S14].
- **P-T04-3** — consumption units are lane-heterogeneous (tokens vs prompts vs requests) → the approximation-tier hierarchy (§S10.1/§S10.4); Z.AI prompt units stay tier-3 derived and dashboard-reconciled; S05 contributes lane-aware stage granularity [XREF:S05]. Ownership acknowledged per S17-R3 [coordinator-draft] (S05's block had assigned it here unreciprocated).
- Shared/referenced: **P-T17-1** (auth-shaped sanction → Class-4 lane freeze, never retry-park; owned S03.6) · **P-T01-3** (billing-regime flip → data-not-architecture currency flip; mechanics land in §S10.2/§S10.10, owned S03.3) · **P-T02-2** (cache-fidelity drift alarm; shared suite [XREF:S14]; owned S03.3 per S17-R2).

**Deferred / parked:**
- `TBD-OPERATOR(Z.AI dashboard prompt-unit calibration)` — the 5-step dashboard recipe closing the request→prompt ratio + multiplier-posting question; re-entry: operator convenience [SPIKE P2-S1 §Blocked; S03.7].
- `TBD-BRINGUP(anthropic unified-header utilization scale + 7d_oi semantics)` — P2-S3 confirmed the header *names*; the value scale and the weekly-Opus-input sub-limit behavior need one bring-up observation before the overlay is trusted for park timing [SPIKE P2-S3 §4].
- Idle-detection GPU auto-pause → post-v0 novel work; re-entry: post-v0 [G2 Def.6].
- OmniRoute-class OSS matures a D2/D4-compatible per-person pressure router → re-run adopt-vs-build for the routing layer only; the ledger/scheduler stay Sinet-owned (D7/D9-entangled) [R09 §4].
- Metered-exception lane (DeepSeek pre-registered designate) → parked while the exception list is empty at v0; re-entry: operator enables a metered exception [G1 P7; S03.6].
- systemd `Persistent=` suspend catch-up sanity test → folded into S01.7's suspend-session bring-up probe, not duplicated [XREF:S01].

**Coverage:** (Scope → subsection)

| feature-list item | subsection |
|---|---|
| 3.1 exact consumption metering | S10.1, S10.3 |
| 3.2 limit events as scheduling events | S10.5, S10.7 |
| 3.3 automation never starves the human | S10.4, S10.7 |
| 3.4 billed to requester (ceremony itemized) | S10.1 |
| 3.5 effort modes as disclosed depletion | S10.6 |
| 3.6 honest receipts + done-directly figure | S10.10 |
| 3.7 hard ceilings + anomaly | S10.8 |
| 3.8 disaster economics (bounded re-spend) | S10.1, S10.5; mechanism [XREF:S02] |
| 3.9 never fully offline | S10.9 |
| 3.10 metered spending opt-in only | S10.2 |
| 3.11 local-resource arbitration | S10.9 |
| 2.8 missed-slot policies | S10.7 (v1 surface [XREF:S19]) |
| D4 measure-not-model; reactive limits | S10.1, S10.4, S10.5 |
| D5 two currencies | S10.2, S10.3 |
| S2.5 cost observable at every altitude | S10.10 (surface [XREF:S15]) |

**Open items for G4:** none remaining.
- *Resolved inline (recorded per CONTRACT, not left open):* price-table seed source — R09 §2.6 leaned models.dev; **the gate ratified genai-prices** [G2 Def.16; D2.2], which binds (§S10.3); models.dev/LiteLLM demoted to CI cross-checks. The never-fully-offline local-floor residency — R09's always-resident-slot sketch is **superseded by R16's TTL-warm/CPU-tier shape** [XREF:S12] (§S10.9). Both are gate/later-report supersessions, not live tensions.

## S11 — Sandboxing & confinement

**Scope:** The per-run sandbox that is D2's load-bearing isolation boundary — the composed kernel-primitive stack, the host-verified prerequisites it runs on, how the workspace and sanctioned sharing mount inside it, the credential broker and engine-credential injection that keep every secret outside it, the declarative confinement classes C0–C4 with their admission check, the two escape-surface problems this section owns, the deliberate privileged surface the unprivileged control plane needs, sandbox teardown, and the honest blast-radius invariant that closes the section.
**Binding inputs:** R10 (primary) [G2 D2.3]; SPIKE P2-S2 (host sandbox prerequisites + systemd harvest matrix), SPIKE P2-S3 (engine model-egress credential-injection wire probes); G2 D2.2 (`sandbox-runtime`, systemd-creds+sops/age adopted), D2.3 (sandbox stack ratified, host-contingent — now cleared); G3 Def.7 (privileged resume-remediation designed at spec time with T09 review); G1 Def.6 (systemd transient units), D1.3 (pause-and-flag, never auto-kill), rider 1 (settings-not-constants); feature list D2, 4.1, 4.2, 4.4, 4.7, S5.1–S5.6, S1.6, 12.1/12.2, 15.1, Operating reality; siblings `S01` (unit map, `sinet-broker`, run units, listener lint, sleep/wake reconcile) and `S02` §S02.10 (the DECIDED overlayfs+git-worktree workspace) + §S02.5 (recovery ladder / GC). Problems: P-T09-1, P-T09-2 (this section), P-T05-1, P-T13-1, P-T01-2.

*New terms coined here, defined on first use:* **per-run sandbox** (the composed jail wrapping one run's engine process); **credential-injection proxy** (the host-side TLS-terminating proxy on the pinned model-egress path); **auth-profile** (a named, broker-resolved credential reference held in a worker's control-plane record — never a secret); **run launcher** (`sinet-run@`'s fixed `ExecStart`).

### S11.1 The composed per-run sandbox stack

Every run executes inside a **per-run sandbox**: a composition of no-daemon kernel primitives layered on the adopted systemd baseline [R10 §4; G2 D2.3; G1 Def.6]. Composed outward-to-inward:

```
sinet-run@<run_id>.service   PID-1 time ceiling + cgroup accounting (the S10 cost backstop) [XREF:S01]
  └─ bubblewrap               user/mount/PID/UTS/IPC + empty per-run netns; workspace + ro-bind caches
       └─ seccomp-BPF          static allowlist profile (kills ptrace/process_vm_readv/odd execve)
            └─ Landlock         filesystem allowlist + TCP bind/connect scoping (ABI 8)
                 └─ engine      claude -p / opencode session — unprivileged, NoNewPrivileges
```

This is the exact stack both shipping first-party harnesses (Claude Code, Codex) independently converged on [R10 §2.1, §4]: it starts in single-digit milliseconds (fits seconds-lifetime runs), every layer is a kernel primitive a bus-factor-1 maintainer can hold in their head, and it needs **zero engine modification** (engines are configured, never forked). seccomp and Landlock are **defense-in-depth, not the boundary** — the boundary is the namespace isolation plus the network policy (S11.4) [R10 §2.1, §3.1].

**Adoption.** Anthropic's Apache-2.0 `sandbox-runtime` ("srt") is adopted for the **bwrap + seccomp + proxy core** [G2 D2.2; R10 §2.1]: it wraps any process (no container image, single-JSON config) and supplies the netns-removed + UDS-to-host-proxy egress shape used in S11.4. Adopt-don't-fork holds — srt wraps processes externally; the engines are configured. Its pin, replacement path, and abandonment criteria live in `components.lock` [XREF:S16].

**Two structural rules, from the escape record** [R10 §5]:

- **Allowlist-only, never denylist.** Agents reason around denylists (`/proc/self/root/usr/bin/npx`, the dynamic linker loading a binary past an exec gate). Mounts, tools, and egress are allowlist-only; there is no denylist to defeat.
- **Empty-env, deny-by-default — not scrub.** The productized default (Claude Code inherits the environment and reads `~/.ssh`/`~/.aws` unless each is denied) is the wrong default for D2. Sinet builds fresh from an empty environment with nothing bound, so there is nothing to scrub and nothing to forget [R10 §3.6, §5].

**Substrate granularity is load-bearing** [R10 §3.4]. `claude -p` is per-run → each invocation is wrapped in its own per-run sandbox. `opencode serve` is a per-user persistent HTTP server that sandboxes nothing itself → the whole server runs inside a **per-user** bwrap+netns jail scoped to that user's workspaces and model-egress, each session gets its own workspace (S11.3), and opencode's app-level `permission`/`allowRead`/`denyRead` config is an **inner soft layer only** — configured through S03's engine lowering [XREF:S03], never the boundary. The **cross-user boundary (D2's load-bearing one, since OS-user separation is deliberately omitted) is preserved on both lanes** by the per-user server + per-user XDG (spike G1-S3) + per-user jail; per-run OS-level isolation is strictly stronger on the claude lane.

### S11.2 Host prerequisites — verified on the v0 host

The D2.3 ratification contingency [G2 D2.3] is **discharged** for everything checkable without root, reboot, or a real suspend [SPIKE P2-S2]. The three blocking facts and what each settles:

| Prerequisite | Measured on the v0 host | What it enables / which fallback is now moot |
|---|---|---|
| **Unprivileged userns under AppArmor restriction** | `apparmor_restrict_unprivileged_userns=1` enforced (plain `unshare -U -r` blocked); **bwrap 0.11.1** at `/usr/bin/bwrap`; `/etc/apparmor.d/bwrap-userns-restrict` **ships and functions** (bwrap gets its userns, child capabilities stripped under label `bwrap//&unpriv_bwrap (enforce)`) | The full stack runs with **no fallback needed** — R10's "per-binary AppArmor profile" fix is shipped and proven, so that fallback is moot as a worry. **Rider (binding):** Sinet MUST invoke the *system* `/usr/bin/bwrap` (the profile attaches to that path), never a vendored or renamed copy, or ship its own `userns`-granting profile |
| **Landlock ABI level** | **ABI 8** — TSYNC multithreaded enforcement present (engine processes are multithreaded → the bar is met); **ABI 10 UDP scoping absent** | Landlock filesystem allowlist + TCP bind/connect scoping are available and used; Landlock cannot scope UDP at ABI 8 → **nftables remains the egress boundary anyway** (S11.4), which was always the design — the "nftables fallback" is simply the boundary |
| **Filesystem / reflink CoW** | Single ~420 GB **ext4** root, **no reflink**, 91% full (~39 GB free) | Reflink-CoW per-run workspaces are unavailable as-is → this is precisely why S02.10 **DECIDED overlayfs + git-worktree** rather than reflink; the sandbox mounts that choice (S11.3) [XREF:S02] |

**Remaining deferred host probes** (need root / reboot / a real suspend — operator-assisted, implementation phase): system-unit reboot survival of run-unit corpses; `ExitType=cgroup` exercised against the *real* bwrap+engine+children run wrapper (vs the synthetic probe); system-scope `systemd-run`/template-unit variants. `TBD-OPERATOR(sandbox host-probe close-out — system-scope run-unit + ExitType=cgroup-against-real-tree, batched with the S01/S02 suspend session)`. None changes the stack's shape; each only confirms a sub-behavior the S02 recovery ladder already tolerates.

### S11.3 Workspace composition inside the sandbox

S02.10 DECIDED the v0 workspace as **git-worktree + overlayfs on the existing ext4 volume** [XREF:S02]. This section specifies how that mounts *inside* the per-run sandbox; the workspace *lifecycle and GC policy* remain S02's.

- **Mount shape.** `lowerdir` = a **read-only shared project base** (a worktree of the registered project store, S1.6, shared across runs and never mutated during a run); `upperdir` = the **per-run writable diff**. The upperdir *is* the run's reviewable change set, aligning with the accept-work-as-attributed-commit flow (D9) [XREF:S13]. bwrap ≥ 0.11 wraps overlays natively (`--overlay`/`--ro-overlay`/`--tmp-overlay`); host bwrap 0.11.1 qualifies [R10 §3.2; SPIKE P2-S2].
- **Two overlay gotchas, both binding** [R10 §3.2]: (a) mutating the `lowerdir` while it is mounted is undefined behavior → the shared base is only ever updated **between** runs, by S02's GC/refresh, never by a live run; (b) **Landlock rules do not propagate through overlay layers** → every Landlock filesystem rule MUST target the **mounted overlay path**, not the lower or upper directories.
- **Sanctioned sharing → mounts** — the complete enumerated set of 4.1's exceptions, everything else isolated (D2):

  | Sanctioned share (4.1) | Mount |
  |---|---|
  | Read-only common caches (package caches, model weights) | `--ro-bind` (`BindReadOnlyPaths=`). A **writable** cross-run cache is the poisoning anti-pattern (one compromised run seeds the next); where a tool insists on writing, give it a shared ro lower + a per-run throwaway `--tmp-overlay` upper [R10 §3.2] |
  | The registered project store via workspace-clone (S1.6) | the overlay above — shared worktree base `lowerdir` + per-run `upperdir` [S02.10] |
  | Resources a project explicitly shares | mounted per the worker's declarative `mounts` list, `ro` or `rw-no-delete` [R10 §3.6; XREF:S08] |
  | Everything else | **isolated** — not bound; the credential stores are **structurally denied** (empty-env, deny-by-default), never reachable [R10 §3.6; D2] |

### S11.4 Egress: the firewall is the boundary, the proxy is convenience

The real egress boundary is a **per-run empty network namespace backed by a host-level nftables default-DROP**; the allowlisting proxy is a **convenience layer**, not a security boundary [R10 §2.3, §3.3, §4]. Composition:

1. **Per-run empty netns** (created unprivileged inside bwrap; loopback only, no route) — the primary default-drop: with no route, nothing egresses. Classes that need egress reach out **only through a bind-mounted Unix-domain socket** to a host-side proxy (srt's netns-removed shape) — the sandbox holds no IP route to exfil through [R10 §3.3].
2. **Host-level nftables default-DROP + IP deny-CIDRs** — the standing boundary that backs the host proxy and any residual path: cloud-metadata (`169.254.169.254`) and RFC1918 ranges are dropped, closing SSRF/rebinding to the host and metadata service. Installed once at boot by a privileged unit (S11.8), never per-run [R10 §2.3].
3. **Host-side allowlisting proxy** — hostname allowlisting by SNI / HTTP CONNECT with **no TLS interception**. This is **convenience only**; it decides *which* allowed host, it does not *contain*.
4. **Block outbound DoH resolvers; restrict HTTP methods** where the class allows (GET/HEAD/OPTIONS for read-only fetch) [R10 §2.3].

**Why the proxy is not the boundary — the CVE record that is the basis for this posture** [R10 §2.3, §5]: CVE-2025-66479 (an empty allowlist read as allow-all disabled srt's proxy for ~30 releases); the SOCKS5 null-byte bypass (`attacker.com\x00.google.com` passed the string check, ~5.5 months); CVE-2025-55284 (DNS-tunnel exfil via auto-approved `ping`/`dig`); CamoLeak / CVE-2025-59145 (exfil through GitHub's *own trusted* proxy); harden-runner CVE-2026-32947 (DoH tunneling defeated an eBPF DNS allowlist). Every one proves hostname/string allowlisting fails as a boundary → it must sit **behind** the firewall's IP:port default-drop.

**TLS interception is OFF by default** — installing a trusted CA in every sandbox breaks cert-pinned tools, enlarges the trust base, and forces the proxy to re-implement TLS [R10 §5]. The **one** justified MITM is the engine→model-egress path, where Sinet owns both ends (S11.5).

Per-class egress (feeds the S11.6 table): **C0** = one named service host + IP deny-CIDR guard; **C1 / verification runs** = empty netns, **no proxy at all** (strongest and simplest); **C2** = default-drop + proxy allowlisting package-registry hostnames **and their CDN hosts** (the Fastly fan-out gotcha — `files.pythonhosted.org`, `static.crates.io`; an on-host caching mirror can later narrow C2 to one internal host); **C3** (v0.1) = **no raw egress**, only one fetch-broker host that fetches on the sandbox's behalf and returns results as data [R10 §3.3].

### S11.5 Credentials outside the sandbox — the broker and engine injection

**D2 invariant (MUST):** no credential ever enters a task sandbox; the sandbox is *the* isolation boundary (there is no OS-user separation) [D2; R10 §2.2]. Two credential kinds are handled distinctly: **owner secrets** (git, provider connectors, outward-effect channels) via the broker, and **the engine's own model-endpoint credential** via injection.

**The broker** (`sinet-broker.service`; S01 owns the unit, S11 the internals) is an **ssh-agent-shaped** host-side daemon on a per-user UDS: the sandbox submits a *typed operation*, the broker attests the caller by **UDS peer credentials (SO_PEERCRED)** — authenticate the asker, never hand it the secret — evaluates policy, executes with the owner's credential **outside** the sandbox, and returns **only the result** [R10 §2.2, §3.4; S01.2]. It is never modeled on git-credential helpers (which return the secret — the anti-model). Three invariants are adopted: per-credential **destination constraints** (ssh-agent), **never pass tokens through** + **audience binding** (MCP spec MUSTs), and a policy-decision/credential-delivery split kept as an internal code boundary (the decision path holds zero secrets) [R10 §3.4]. Every worker holds only **auth-profile** references (named, broker-resolved) in its control-plane record — never raw secrets [XREF:S08]. Outward-effect operations (push/publish/send) route to the 4.2 gated-proposal flow and the broker performs signing/pushes only on approval [XREF:S13] — the broker is the single choke point to log, revoke, and rate-limit. **Secrets at rest** use systemd-creds + sops/age [G2 D2.2]; the broker signing key and any per-user store are `0700`, host-only, never in any sandbox (opencode stores tokens plaintext in `auth.json` — the store is encrypted at rest regardless) [R10 §6; SPIKE P2-S3 §5].

**Engine credential injection — resolved wire-side.** The engine (`claude -p`, `opencode serve`) needs *its own* subscription credential to reach the model endpoint, and adopt-don't-fork forbids splitting engine auth from tool execution. **Pattern-1** keeps it out of the sandbox: the sandbox holds a **sentinel**; a **credential-injection proxy** (TLS-terminating, per-process-trusted CA) substitutes the real subscription token **only** on the pinned model-egress request [R10 §3.4]. **P2-S3 proved pattern-1 VIABLE on both v0 lanes** — `claude` 2.1.214 and `opencode` 1.18.3 neither cert-pins its model endpoint against a per-process-trusted CA (`NODE_EXTRA_CA_CERTS`; Bun-compat honors it too); a Z.AI **401→200 purely from proxy-side substitution** was demonstrated end-to-end [SPIKE P2-S3]. Therefore **the engine's own credential is kept fully outside the task sandbox on both lanes at v0**; R10 §4 decision-changer #2 (cert-pinning → pattern-2 scoped-egress) does **not** fire for either lane, and the pattern-2 fallback is not required at v0. Binding wire facts:

- **Inject on the model path, not the host** — `claude` fans out to ~8 ancillary `api.anthropic.com` paths (bootstrap, oauth, mcp-registry, event_logging, eval). Inject the real token **only** on `/v1/messages` (Anthropic) / `/api/coding/paas/v4/chat/completions` (Z.AI), auth header `Authorization: Bearer`, and pass every other request untouched — injecting on telemetry/oauth endpoints is needless secret exposure [SPIKE P2-S3].
- **Per-process trust only** — the injection CA is Sinet-owned and trusted per-process (`NODE_EXTRA_CA_CERTS`); the system trust store is never touched. This does not weaken the engines against an *untrusted* MITM (they correctly reject that); pattern-1's security rests on the CA being Sinet-owned [SPIKE P2-S3].
- **One choke point, two purposes** — because the engine tolerates termination, the same proxy harvests the `anthropic-ratelimit-unified-*` headers as provider-signaled observed state (D4-clean, no window modeling), enriching the S10 park-timing and consumption meters [XREF:S10; SPIKE P2-S3].
- **Pin-regression canary (P-T01-2)** — the per-substrate conformance suite asserts, per engine version, that a trusted-CA terminating proxy on the model path still yields a 200, so a future engine release introducing cert-pinning is caught at upgrade time, not in production; if a lane ever regresses, `⚙ sandbox.model_egress_tls_terminate = false` for that lane falls back to pattern-2 (scoped-egress-only: the subscription credential sits in the sandbox but egress is pinned to the model host and no other credential/effect is present) [SPIKE P2-S3; XREF:S14].

### S11.6 Confinement classes as declarative data

A **confinement class** is a rung C0–C4 of the isolation ladder (S5), **declared per worker and carried declaratively** in the control-plane tables — behavioral content lives in template files, but **all enforcement state (class, grants, egress, credentials) lives exclusively in control-plane tables and is recompiled every run** (the guardrail split; workers can never alter it — 14.2) [R10 §3.6; XREF:S08]. The class is compiled into the sandbox launcher's parameters, srt-style: `filesystem` (workspace clone/none, `mounts` ro|rw|rw-no-delete, `denyRead` — credential stores **always** structurally denied), `network` (mode none|registries|fetch-broker|single-host + hostname allow-list, default-deny), `tools` (default-deny per role), `credentials` (auth-profile refs only), `outputs` (proposal-type caps), and `rule_of_two`.

**Rule-of-Two admission check.** Meta's Agents Rule of Two: within a session an agent may hold **at most two of** {processes untrusted input, accesses sensitive data/systems, can change state or communicate externally}. The control plane **statically refuses** a worker whose declared record asserts all three without a supervision gate (human-in-the-loop, i.e. the 4.2 proposal path, or another reliable validation) [R10 §2.4, §3.6]. A run may safely *transition* between two-property phases when the transition breaks the attack chain. This is a compile-time gate on the worker record, enforced outside agent code and prompt reach.

**Per-class concrete profiles** (v0 ships **C0–C2**; C3 at v0.1, C4 future — [G2 D2.3; XREF:S19]):

| Class | Model in loop | Filesystem | Network egress | Credentials | Output gating | Isolation tech |
|---|---|---|---|---|---|---|
| **C0** connectors | no | none (host-side deterministic code) | one named service host + IP deny-CIDR | one narrowest-scope key, **broker-held** (auth-profile) | deterministic; outward effects = proposals (4.2) | egress pin + scoped key |
| **C1** trusted reasoning | yes | **ro** workspace clone | **none** — empty netns, no route, no proxy | **none** | — | bwrap + seccomp + Landlock, no-route netns |
| **C2** workspace-write | yes | **rw** overlay workspace (upper=diff) + ro caches | registries allowlist (+ CDN hosts) via proxy | none in sandbox; git push = proposal via broker | every push/publish = proposal (4.2) | + registries proxy behind host default-drop |
| **C3** web-reading *(v0.1)* | yes | rw overlay workspace + ro caches | **fetch-broker host only**; raw web via broker, returned as data | none in sandbox | data-only; **tighter verification** of output | + **gVisor step-up** for hostile input |
| **C4** web-acting *(future, 12.2)* | yes | disposable profile | per-site scope | none in sandbox | per-action approval; full action log | tightest; nothing graduates in silently |

The threat-detection pass over agent output + patch + original intent (gh-aw safe-outputs shape) is the concrete mechanism for C3's "tighter verification" and for two-axis verification of steered output [R10 §2.5, §6; XREF:S07].

**Plan-declared class flows to helpers tighter-only (P-T05-1).** A coordinator's approved plan declares the run's confinement class; a spawned helper **inherits the coordinator's class and may only be tightened, never loosened** — the admission check rejects any helper spawn requesting a looser class. S11 owns the *mechanics* (the class is a control-plane field; the check is a compile-time comparison at spawn); the *policy* — which class attaches to which plan or stage — is [XREF:S06] §S06.6 [P-T05-1; XREF:S08].

### S11.7 Two owned escape-surface problems

**P-T09-1 — agent-writable configuration is an escape surface** (CVE-backed). CVE-2026-25725 (in-sandbox `settings.json` creation → SessionStart hook running with host privilege on restart), CVE-2025-53773 (Copilot `chat.tools.autoApprove` config-poisoning RCE), and DuneSlide/CVE-2026-50548/9 (working-directory/symlink escape) are one class: an agent poisons its own config or guardrails to escape — sharpened by the spike finding that engines leak operator settings and forget permission config on resume [R10 §5, §7-5]. **S11's mitigations (filesystem/sandbox side):** deny writes to **all** settings/config paths with **symlink resolution** (resolve, then deny — the DuneSlide vector); empty-env deny-by-default so there is nothing to scrub; and confinement **never depends on anything the engine "remembers"** (a resume that did not re-supply settings silently executed a parked call). This problem's **cousins are owned next door**: S03's engine-lowering channel checklist closes the config *channels* (`settingSources:[]`, explicit `--settings`, invocation-config re-supply on every resume) [XREF:S03], and S01's listener-binding lint closes the config-drift-as-escape channel at the process boundary [XREF:S01]; T14/S08 owns the permission schema the class compiles from [XREF:S08]. S11 owns the sandbox-side deny + empty-env; together they close the class. Registered in S17.

**P-T09-2 — allowlisted egress is an exfil channel** (residual). Even a correct firewall+proxy leaves exfil through *legitimately allowed* endpoints — CamoLeak through GitHub's own proxy; exfil-via-`api.anthropic.com` with an attacker key [R10 §7-6]. This is **uncontained by egress control alone** for C2/C3. Compensating **posture** (not a fix): minimal reachable surface (fewest allowed hosts); HTTP-method restriction (GET/HEAD/OPTIONS where the class allows); an on-host caching mirror to narrow C2 toward one internal host; and, for the model endpoint, the S11.5 injection proxy so an attacker holding only a sentinel cannot ride the channel with a real key. **Monitoring hook:** egress volume/pattern anomaly detection registers into the watchdog suite — the machinery is S14's; S11 supplies the posture and the hook [XREF:S14]. Honest disposition: bounded, not closed → S11.10. Registered in S17.

### S11.8 The privileged surface — G3 Def.7 discharge (T09-informed)

Def.7 requires the privileged resume-remediation path to be *designed at spec time with T09 review* [G3 Def.7]. The unprivileged `sinet` control plane needs two privileged capabilities: **(a)** start/stop **system-scope** run units, and **(b)** post-resume network remediation (restart `tailscaled` when wedged, re-assert `tailscale serve` — P-T13-1, S01.7). The design goal, applying T09's own principles (allowlist-not-denylist, no setuid, single-choke-point auditability [R10 §3.4, §5]), is the **smallest fixed-verb surface** that delivers both.

**Why system scope at all.** The per-run sandbox itself is **fully unprivileged** — bwrap under the shipped AppArmor profile, an *empty* netns, seccomp and Landlock all compose without privilege (S11.1–S11.4), and egress reaches a host proxy over a bind-mounted UDS, so run units carry no per-run root. System scope (not `--user`) is chosen only for **io-controller delegation, unit-level AppArmor confinement, and independence from the (degraded) user manager** [SPIKE P2-S2] — and starting a *system* unit is polkit-gated even when the unit runs `User=sinet`.

**Exact privileged verbs (the complete grant — nothing else):**

| Verb | On unit | Purpose |
|---|---|---|
| `start` / `stop` / `reset-failed` | `sinet-run@<run_id>.service` (system scope, runs `User=sinet`) | launch / park / cancel / GC a per-run sandbox |
| `start` | `sinet-netremediate.service` (root oneshot, fixed `ExecStart`) | post-resume `tailscaled` restart + `tailscale serve` re-assert (P-T13-1) |

Explicitly **not** granted: `manage-unit-files` (install/edit/enable/mask), `reload-daemon`, generic `StartTransientUnit` with caller-set properties, and start/stop of any unit outside those two names. The host nftables default-drop + proxy (S11.4) are installed once at boot by a root `sinet-egress-setup.service` that `sinet` cannot trigger — standing config, outside the on-demand grant.

**Two shapes compared:**

| | Shape A — broad polkit on transient units | **Shape B — fixed-`ExecStart` template unit + narrow polkit grant (CHOSEN)** |
|---|---|---|
| Mechanism | grant `sinet` `StartTransientUnit`/manage-units, scoped by unit-name pattern | root-install `sinet-run@.service` (fixed hardening + fixed `ExecStart=/usr/lib/sinet/run-launch %i`, `User=sinet`) + `sinet-netremediate.service`; grant `start`/`stop`/`reset-failed` on just those names |
| Least privilege | **FAILS** — polkit sees the unit *name* but not the *properties*; the caller sets `ExecStart`/`User`/hardening, so `sinet` could start `sinet-run-x` with `ExecStart=/bin/sh` as root | **HOLDS** — the privileged command line is fixed in the shipped unit; `sinet` supplies only *data* (the compiled confinement record at `/run/sinet/jobs/<run_id>.json`, read and schema-validated by `run-launch`), never properties |
| setuid | none | none — systemd units, not a setuid binary (avoids the firejail anti-pattern [R10 §5]) |
| Auditability | polkit + journald | polkit + journald **+ every trigger emits an event on the platform event log** [XREF:S14] |
| Verdict | REJECT | **ADOPT** [coordinator-draft] |

**Chosen (Shape B).** The run unit is realized as a **root-installed template `sinet-run@<run_id>.service`** whose `ExecStart` (the **run launcher**) is fixed and non-agent-reachable; the control plane, as unprivileged `sinet`, writes the per-run confinement record to a `sinet`-owned spool file and `start`s the instance via a **single scoped polkit rule** authorizing `org.freedesktop.systemd1.manage-units` for exactly `^sinet-run@.*\.service$` and `sinet-netremediate.service`. The launcher validates the spool record, composes the S11.1 stack (unprivileged), and execs the engine. Network remediation is a fixed-verb root oneshot: `sinet-netremediate.service` health-checks `tailscaled`, `systemctl restart tailscaled.service` if wedged, re-asserts `serve`, and writes its outcome to the event log — `sinet` can *trigger* it but never parameterize it, so there is no generic "restart tailscaled" grant. The decisive least-privilege argument is that **polkit cannot constrain transient-unit properties** — only fixing the `ExecStart` in a shipped unit makes a name-scoped grant safe.

*Reconciliation with S01.2* [coordinator-draft]: S01.2 describes run units as `systemd-run` transients named `sinet-run-<id>`; S11 refines the realization to a **template instance** `sinet-run@<id>.service` precisely so the polkit grant is property-safe. Functionally identical to S01's intent (per-run, ephemeral, `systemctl`-visible, `Restart=no`, own journal identity via `%i`) — S01.2 wording aligned at assembly (2026-07-19).

**G4 note.** This is the deliberate new-privileged-surface decision Def.7 demands. The recommended surface is the two-unit / one-polkit-rule design above; G4 acknowledges it as the sole privileged grant to the `sinet` user, with the run units themselves unprivileged.

### S11.9 Cleanup and teardown as a sandbox property

Teardown is part of what the sandbox *is*; S11 owns namespace/netns/upperdir teardown, S02 owns the workspace lifecycle/GC *policy* and the recovery ladder [XREF:S02].

- **Run teardown.** On run end / park / cancel, `stop sinet-run@<id>` (via the S11.8 grant) tears the sandbox down with the unit: the empty **per-run netns is destroyed with the unit**, and the bwrap process tree dies with the cgroup — the lane recipe `RemainAfterExit=yes` + `ExitType=cgroup` + `Type=exec` (never `--collect`) ensures the *tree*, not merely the sandbox binary exiting, is what the reconcile pass reads as dead [SPIKE P2-S2].
- **Overlay upperdir disposal.** The per-run `upperdir` is either **captured** (its diff committed/harvested as the deliverable, D9/S13) or **discarded**; disposal is linked to S02's workspace GC — an orphan worktree/upperdir holding **uncommitted** work is flagged, **never auto-deleted** [S02 §S02.5; XREF:S02].
- **Corpse GC.** After harvest the unit corpse is `reset-failed` (S11.8 grant) — the run-unit side of the S02.5 recovery-ladder GC step [XREF:S02].
- **Orphan handling.** A sandbox whose control plane died is reconciled by the S02 recovery ladder (ALIVE/WEDGED/FINISHED-DURING-OUTAGE/DEAD), never by PID-1 auto-restart (`Restart=no`); a WEDGED run is **paused-and-flagged, never auto-killed** (D1.3) [XREF:S02; S01.2].

### S11.10 Blast radius — the closing invariant

Injected content can still **steer a model's reasoning inside its sandbox** — that risk is **not fully removable** by any confinement, and the verification gates that judge steered output are themselves probabilistic (adaptive attacks beat 12 published defenses at >90%, 100% under human red-teaming) [R10 §2.4, §7; 4.7]. What remains uncontained even with the full stack, stated honestly: (a) in-sandbox reasoning steering (sabotaged code / poisoned analysis within the worker's own powers); (b) exfil through allowed egress (P-T09-2); (c) human-approver degradation (~93% approval rates; nothing anomalous to catch when the user typed the instruction); (d) persistent-memory/CLAUDE.md poisoning that reloads every session; (e) below-trifecta harms (misinformation, denial-of-wallet) [R10 §7].

**The guarantee — this section's closing invariant (MUST):** Sinet's confinement delivers **blast radius, not immunity — no credential access, no un-proposed outward effect, and a verification gate judging potentially steered output.** Containment, not immunity. This is the mid-2026 consensus posture (Rule of Two + "contain at the environment layer first" + "the attacker moves second"), not a hedge [4.7; S5.6; R10 §2.4, §4, §7].

---

**Settings introduced (⚙):** (all operator-editable with audit trail per G1 rider 1; auto-adjust only within operator ceilings)

| Name | Default | Clamp / range | Ratified by |
|---|---|---|---|
| `sandbox.egress_deny_cidrs` | {`169.254.169.254/32`, `169.254.0.0/16`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7`} | editable list; the metadata IP `169.254.169.254/32` is a non-removable floor [coordinator-draft] | R10 §2.3/§3.3 |
| `sandbox.block_outbound_doh` | `true` | {true, false}; false requires a recorded reason | R10 §2.3 |
| `sandbox.c2_registry_allowlist` | curated npm/pypi/crates/apt/go/maven/nuget/rubygems + their CDN hosts (Copilot/Codex preset) | editable list (data, like the price table) | R10 §3.3 |
| `sandbox.model_egress_tls_terminate` | `true` (per lane) | {true, false}; false ⇒ pattern-2 scoped-egress fallback for that lane | SPIKE P2-S3 / R10 §3.4 |

The seccomp-BPF profile, the Landlock ruleset, and the per-class profile defaults (S11.6) are **structural**, not ⚙ — versioned in `components.lock`/the worker schema, not operator dials.

**Known problems owned here:**
- **P-T09-1** — agent-writable configuration is an escape surface → sandbox-side deny-writes-to-config-paths (symlink-resolved) + empty-env; channel-closing cousins in S03 (engine lowering) and S01 (listener lint); schema in S08 (S11.7).
- **P-T09-2** — allowlisted egress is an exfil channel → uncontained by egress control alone; posture = minimal surface + method restriction + injection proxy; anomaly-monitoring hook in S14; honest residual in S11.10 (S11.7).
- **P-T05-1** (filed by T05, *mechanics here*) — plan-stage confinement must bind helpers → class is a control-plane field; admission check refuses looser-than-coordinator helper spawns; policy in S06.6 (S11.6).
- **P-T13-1** (filed by T13, *privileged path designed here*) — post-resume network reconcile → the fixed-verb `sinet-netremediate.service` triggered under the S11.8 grant; detection/duty in S01.7 (S11.8).
- **P-T01-2** (filed by T01, *extended here*) — engine schema/behavior drift → the per-substrate conformance suite gains a model-egress-MITM-tolerance canary so a cert-pinning regression is caught at upgrade (S11.5).

**Deferred / parked:**
- **C3 web-reading** (fetch-broker host + gVisor step-up) → v0.1 [G2 D2.3; S00.2]; the ladder is specified now so the v0 tech choice is not reversed to add it.
- **C4 web-acting** → future (12.2); parked behind the 15.3 gate [S00.2].
- **gVisor / microVM step-up** for a single-user run needing zero-trust isolation → re-entry only if a class must execute genuinely adversarial code, not merely read hostile web content [R10 §4].
- **Per-run veth + per-run nftables** (finer per-run egress than the host-level default-drop + UDS-proxy) → post-v0 option; would add a privileged per-run setup step [R10 §3.3].
- **GPU-in-sandbox** (12.1 future image-gen) → GPU work stays behind the T15 broker service outside every sandbox; direct `/dev/nvidia*` binding is never used at v0 (NVIDIAScape/CVE-2025-33219 ioctl LPE surface) [R10 §3.5; XREF:S12].
- **opencode native OS sandboxing** (issue #5529) → if it lands, reduce the per-user-jail workaround to configuration and reconsider per-run confinement on that lane [R10 §4].
- **Deferred host probes** (system-scope run-unit + real-tree `ExitType=cgroup`, reboot survival) → `TBD-OPERATOR`, batched with the S01/S02 suspend session [SPIKE P2-S2].

**Coverage:** (Scope → subsection)

| Feature-list item | Where |
|---|---|
| S5 confinement ladder (C0–C4) + S5.6 honest caveat | S11.6, S11.10 |
| 4.1 isolation with enumerated exceptions | S11.3 |
| 4.4 minimal powers per worker | S11.6 (declarative class, default-deny) |
| 4.7 untrusted content hostile; blast-radius guarantee | S11.10 |
| 4.2 outward effects only as proposals (no send/publish/push in sandbox) | S11.5, S11.6 |
| D2 credentials never in task sandboxes; sandbox = the boundary | S11.5 (broker + injection), S11.1 |
| S1.6 registered store → workspace clone | S11.3 |
| 12.1/12.2 future GPU / web-acting (sockets left) | Deferred/parked |
| 15.1 v0 ships C0–C2; sandbox stack; broker | S11.1, S11.5, S11.6 |
| G3 Def.7 privileged resume-remediation path | S11.8 |

**Open items for G4:** none open. Three drafting-time sub-choices are flagged inline as `[coordinator-draft]` for G4 attention: (1) the **privileged-surface design** (S11.8 Shape B — the deliberate new-privileged-surface decision Def.7 demands; recommended as the sole `sinet` grant, run units unprivileged — G4 note in S11.8); (2) the **run-unit realization** as a template instance `sinet-run@<id>.service` refining S01.2's `systemd-run` wording (coordinator reconciles at assembly); (3) the `sandbox.egress_deny_cidrs` non-removable-floor choice. The two problem ids **P-T09-1 / P-T09-2** are assigned here (the G2 follow-up list named them descriptively) for S17 to adopt.

## S12 — Local-models tier

**Scope:** The permanent free tier: the local serving architecture, the v0 model set and duty-alias registry, the GPU broker data plane, VRAM-ledger and preemption mechanics, power/residency policy, the bring-up battery, and the lifecycle rules (re-evaluation, swaps, recalibration) for everything local.

**Binding inputs:** Operating reality (local free tier) · 3.9 · 3.11 · S2.7 · 1.10 · 12.1 · 7.3 · [R16 §2, §4, §5, §7] (T15 recommendation as ratified, G3 digest) · [G1 P9; Def.2; Def.9] · [G2 D2.1 (VRAM-ledger admission inside the R09 §4 package); Def.6; Def.8; Def.11; Def.12] · [G3 D3.5; Def.3; Def.4; Def.8; §Follow-ups] · [SPIKE P2-S1 headline] (logprobs exist only on the local lane) · siblings: [S03.2] lane exposure · [S06.10] ceremony duty map · [S01.2] unit-map slot.

Boundary lines: S03 owns adapter mechanics [XREF:S03]; S10 owns pressure/budgets/admission *policy* and all CPU/RAM arbitration [XREF:S10]; S14 owns canary/eval machinery [XREF:S14]; S06/S07 own which duties exist [XREF:S06; XREF:S07]. This section owns the serving stack, which models serve which duties, and the GPU-specific mechanics.

**Term note:** the **GPU broker** ([R16 §4.7], discharging R10-OQ4) is the control-plane-mediated local-inference data plane. It is distinct from the credential **broker** of the S00 glossary; this section always writes "GPU broker".

### S12.1 Position: the permanent free tier

- The platform's own background intelligence — health watching, change detection, inbox risk-ranking, routine classification — runs on local models only, costs nobody any allowance, and keeps working when every paid window is empty [Operating reality; 3.9]. The local tier is the permanent floor of D5 pressure routing.
- The local `/v1` surface has **two consumer classes**: (a) the **D3 local lane** — runs executing on local models ride the pinned `opencode serve` adapter as OpenAI-compatible provider entries pointing at the S12.2 endpoint [S03.2]; (b) **platform duty calls** — the control plane invokes the S12.4 duty aliases directly over the same OpenAI surface, with no engine session involved (intake triage, watchdog verdicts, and their peers are not runs) [R16 §4.2; S06.2].
- The local lane is the **only v0 lane exposing logprobs** [SPIKE P2-S1 headline; S03.2] — it therefore hosts the logprob drift canary [XREF:S14] and carries the S12.5 confidence-margin signal. Both subscription lanes are behavioral-eval-only.
- Model weights live in the shared **read-only model cache** (4.1's sanctioned sharing); the OS page cache is the residency layer (S12.8).
- The free tier is metered, just free: every local call — either consumer class — writes a D7 ledger usage row (model hash + engine build + tokens) priced at $0 and counting into consumption pressure as the D5 floor [D4; R16 §4.2]; local duties appear as zero-allowance receipt lines [S06.10; XREF:S10].

### S12.2 Serving architecture

**One pinned `llama-swap` instance (MIT) is the single OpenAI-compatible local endpoint, fronting pinned `llama.cpp llama-server` backend processes** (pinned `b`-tag; Linux CUDA ships no prebuilt binaries — one-time source build or the project's container image) [R16 §4.1, §2.3; G3 digest]. llama-swap spawns and kills **one backend process per loaded model**; process death returns VRAM fully to zero, CUDA context included [R16 §2.3]. Both components run unmodified, configured only (adopt-don't-fork); pins, replacement path, and abandonment criteria live in `components.lock` [XREF:S16; G2 D2.2].

- **Placement is by GPU UUID**, never index (`env: CUDA_VISIBLE_DEVICES=<uuid>` per model — indices renumber on eGPU attach) [R16 §4.1, §4.4].
- **Groups:** `pool12` (swap group, the 12 GB internal GPU) · `pool24` (swap group; configured only when the eGPU is enrolled, S12.11) · `resident` (persistent group — **empty by default**; S12.8) [R16 §4.1]. Members of a swap group hold their pool exclusively — loading one unloads the others; concurrency comes from the CPU tier and queueing, never co-residency [R16 §2.3].
- **Idle:** per-model TTL expiry kills the backend → full VRAM release (⚙ `local.ttl.*`; defaults S12-table) [R16 §4.1].
- **Manual eager-unload verb = the ratified operator-wins switch [G2 Def.6]:** a control-plane action that (1) stops local-lane admissions and (2) calls `POST /api/models/unload` — surfaced as a one-tap card and CLI verb [R16 §4.5].
- **GameMode hook [G2 Def.6]:** `gamemode.ini [custom]` start/end scripts call the same two verbs, **plus** a control-plane D-Bus subscription to the GameMode `GameRegistered`/`GameUnregistered` signals — scripts fire even while the control plane restarts; signals cover the daemon's own restarts [R16 §4.5]. TBD-P3(GameMode user-context probe — per-user `gamemoded` hooks/signals must reach the system-scope control plane; R16 §7-OQ7) **[CLOSED A5, 2026-07-23: probe ran — signals are session-bus-only, unreachable from system scope without an operator `DBUS_SESSION_BUS_ADDRESS`; the scripts leg carries pause/resume, the busctl subscription is deferred-with-finding]**. Contention detection beyond GameMode: NVML graphics-process delta against the known-desktop set + device-utilization corroboration (robust in hybrid display mode, degraded in MUX); Wayland fullscreen heuristics are rejected as a signal. Automatic idle-detection (auto-pause/resume) is post-v0 novel work [G2 Def.6; R16 §4.5].
- **Embeddings ride the same stack**: a `llama-server --embedding` member serving the 0.6B embedder is configured **only when the G2 Def.8 vector gate opens** — no second serving framework, ever [R16 §4.1; G2 Def.8].
- **Unit placement:** llama-swap runs as its own adopted-organ system unit, `sinet-llamaswap.service` `[coordinator-draft]` name, loopback-bind per the S01.1 invariant, own journal identity; backends are its child processes, not units, so the whole tier sits in the local-inference slice (the designated systemd-oomd victim; slice policy [XREF:S10]) [S01.2; R16 §4.5].
- **Fallback ladder (pre-registered)** [R16 §4.1]: llama-swap's bus factor 1 is an accepted risk — it is a small MIT Go proxy whose YAML/endpoint contract the conformance suite asserts on every bump. If it goes unmaintained or its contract breaks: (1) `llama-server` router mode + a Sinet-side kill-idle timer, accepting ~600 MiB retained per sleeping model; or (2) **Ollama — the named, capability-complete fallback**, including `/v1` logprobs since v0.12.11 (a fact its own compat docs mis-stated for ~8 months — the live specimen behind P-T15-2: **conformance suites assert behavior, never docs**) [R16 §2.3, §7-OQ9]. Ollama is not primary for structural reasons only: no per-model GPU pinning (two pools ⇒ two daemons), no rerank endpoint, none of the group/persistent/alias primitives this design leans on [R16 §5].
- Not used, for cause [R16 §5]: vLLM as primary (0.92 VRAM preallocation at init; dev-gated partial sleep — it is only the pre-registered pool24 batch backend, S12.11); router-mode sleep as the idle story (~600 MiB retained per model); LM Studio (closed source, restrictive terms).

### S12.3 v0 model set

| Seat | v0 model | Quant · pool | License | Basis |
|---|---|---|---|---|
| Workhorse | **Qwen3.5-9B** | Q5_K_M · pool12 | Apache 2.0 | default alias target unless the bakeoff flips it [G3 Def.8; R16 §4.2] |
| Workhorse alternate | Gemma 4 12B QAT | first-party int4 QAT · pool12 | Apache 2.0 | bakeoff alternate — scores within noise of the 9B; better grounding lineage; less KV headroom [R16 §2.1, §4.2] |
| Fast tier | Qwen3.5-4B | Q4_K_M · pool12 + CPU tier | Apache 2.0 | burst/classification seat; CPU-viable; same family/dialect as the workhorse [R16 §4.2] |
| Entailment | **Granite Guardian 8B** | pool12 | Apache 2.0 | default seat [G3 Def.4]; benchmark row is 3.3 (AggreFact 76.5 — frontier-matching for grounding screens); evaluate 4.1 and verify its card license at adoption [R16 §2.5, §7-OQ3] |
| Entailment accuracy-alternate | Bespoke-MiniCheck-7B | pool12 | **CC-BY-NC 4.0** | alternate seat only — acceptable for this household deployment, never the default (license landmine; blocked if Sinet ever commercializes) [R16 §5] |
| Entailment CPU floor | MiniCheck-Flan-T5-0.8B | CPU | per card | sampled checks only, pending the G3 Def.4 measurement (S12.9) |
| Contradiction pre-screen | DeBERTa-class NLI cross-encoder | CPU (~100 MB) | per card | high-precision advisory pre-screen [R16 §2.5, §4.2] |
| Embedder | Qwen3-Embedding-0.6B (GGUF Q8_0) | pool12 | Apache 2.0 | **post-gate only** [G2 Def.8]; rank-only, selection stays trace-manifested [XREF:S09] |
| Layer-2 open SQL | **Arctic-Text2SQL-R1-7B** | pool12 | Apache 2.0 | fits the 12 GB pool at ~70% BIRD-class (open SOTA ~74%; human ~93%); ships at v0 flagged lower-confidence [G3 D3.5; R16 §2.1] |

- **Envelope reading:** G1 P9's "≤8B" binds as the **≤8B-class / 12-GB-pool envelope** — the reading R16 held fixed and the gates ratified by naming the 9B (G3 Def.8) and the 7B (G3 D3.5) seats; post-v0 the per-duty ceiling is the RTX 3090 24 GB envelope (S12.11) [G1 P9; R16 §1].
- **Quant policy** [R16 §2.1]: Q5/Q6 where VRAM allows (≤9B on pool12); first-party QAT where offered; Q4_K_M for 27–36B on pool24; **KV cache stays fp16/q8_0 — KV quantization is the risky axis**. Deployed quants are validated by the S12.9 KL check.
- **License rule:** Apache-2.0/MIT by default; any exception is carried per-seat with its reason (the sole v0 exception is the MiniCheck alternate). Licenses are verified **on the model card itself** at adoption — aggregator roundups hallucinate models and licenses [R16 §4.10].
- **Layer-2 SQL guardrails, restated by reference [G3 D3.5]:** read-only connection, allowlisted views, single-statement parse, LIMIT + timeout, audit-logged, every answer flagged lower-confidence; canned queries remain the reliability floor. The query surface itself is [XREF:S14].

### S12.4 Duty-alias registry

A **duty alias** (S00 glossary) is a named capability slot mapping to a swappable model; **workers, templates, and platform callers only ever address aliases** — model swaps are invisible to them, which also protects D8 configs from deprecation churn (7.3) [R16 §4.7]. Which duties exist is owned by their sections; this registry binds each to its serving model. Alias names `[coordinator-draft]`; the alias→model map is settings-backed (⚙ `local.alias.<duty>`), changed only through the S12.10 swap gate.

| Alias | Serves [owner] | v0 target | Enforcement | On low confidence |
|---|---|---|---|---|
| `utility` (per-person, 1.10) | card/question phrasing, 13.5 help drafting, summaries, receipt-line drafting over deterministic numbers, inbox risk-ranking, lesson/observation drafting [S06.10; XREF:S09] | workhorse (recommended platform-wide default; per-user override per 1.10) | per-duty schema; length caps | outputs are human-gated downstream; this seat never decides |
| `intake-triage` | task-family/stakes/size classification, data-bearing classifier, advisory coverage spot-check [XREF:S06] | fast; workhorse re-check on low margin | `json_schema` enums | S06's deterministic floors override; classifier failure fails closed at S06's layer |
| `watchdog-disambiguator` | {loop\|productive\|unclear} verdicts on Tier-0 triggers [XREF:S14] | fast (GPU); CPU tier when the dGPU is contended or asleep | `json_schema` enum + evidence string; greedy | low margin ⇒ the verdict **is** `unclear`; annotates, never gates; **NEVER a cloud fallback** (S2.7) |
| `watchlist-triage` | pass 1 "does this change matter?", pass 2 {lane, class, severity, summary} [G2 Def.12; XREF:S14] | fast; severity ≥ high re-checked by workhorse; pass 1 runs via changedetection.io's native LLM rules against the local `/v1` surface [R16 §4.2] | `json_schema` enums | margin-gated workhorse re-check; price/billing hits are always carded to a human |
| `intent-filling` | canned-query Layer-1 slot filling [XREF:S14] | fast; workhorse on low margin | grammar-constrained slot types | below threshold ⇒ a "which of these did you mean?" card — never a guess |
| `sql-open` | Layer-2 open SQL escalation [G3 D3.5] | Arctic-Text2SQL-R1-7B | the S12.3 guardrail stack | every answer flagged lower-confidence by construction |
| `entailment` | claim–citation entailment: mandatory on load-bearing claims + sampled rest [G1 Def.2; XREF:S07] | Guardian 8B; CPU floor for sampled checks pending [G3 Def.4] | binary + P(yes) from logprobs | below threshold ⇒ flagged into the paid verification pass — never a silent pass |
| `contradiction-screen` | lesson/memory contradiction screen [XREF:S09] | DeBERTa CPU pre-screen → workhorse confirm on hits | `json_schema` | high-precision advisory: question cards only, never silent resolution |
| `distill-summarize` | L1 distiller; run summary at run end [G2 Def.11; XREF:S09; XREF:S14] | workhorse | grounding instructions, extract-then-abstract, event-id citation-forcing, length caps, section schema | sampled outputs screened by `entailment` (generate-then-verify); persistent failure escalates the model choice, never the run |
| `embedder` | retrieval candidate ranking, post-gate [G2 Def.8; XREF:S09] | Qwen3-Embedding-0.6B | rank-only; selection stays trace-manifested | n/a |

**Cross-cutting duty rules** [R16 §4.2, §2.4]: temperature 0 / greedy decoding for every classification-shaped duty (determinism + a defined margin); every schema carries an explicit abstain/`unclear` member — a model is never schema-forced to fabricate a label; the schema appears in the prompt *and* is enforced at the engine (`json_schema`/GBNF — validity is an engine property); free-text reasoning precedes the constrained region; every call is ledgered per S12.1.

**The ceremony cut line** [R16 §4.9, §2.5; G3 digest; consistent with S06.10]: local takes every fixed-schema screen, annotation, triage, and human-gated draft — the registry above. **Interviewing (1.2), restatement-until-confirmed (1.3), plan critique (1.5), verification review (5.x), and gap advice (2.7) run on paid frontier-class models and are NEVER assigned to local aliases**: 8B-class generalists judge below the random floor on hard pairwise judging (JudgeBench 40.86% vs the 50% floor) and pair worst-in-class position bias with near-perfect self-consistency — a biased small judge repeats its bias reliably. Helper-report screening stays conformance-only at v0 [G1 Def.9; G3 Def.3]; a local plausibility screen becomes admissible post-v0 only when the S12.9 battery shows ≥ the pre-registered bar on real helper outputs.

### S12.5 Confidence & escalation discipline

Verbalized confidence from 4–8B models is not a routing signal. The supported cheap signal is the **label-token logprob margin (top1−top2) under greedy decoding**, post-hoc calibrated per duty [R16 §2.4]: (1) a labeled set (~30–50 items bootstrap, grown from production under golden-set governance [XREF:S14]); (2) margins via `/v1` logprobs; (3) an isotonic map margin→P(error) on a calibration split; (4) the escalation threshold picked on a validation split — minimize local-only cost subject to the duty's acceptance bar (recipe shape adopted; every number re-derived locally) [R16 §4.3]. All thresholds TBD-BRINGUP(per-duty confidence calibration) **[NARROWED A7, 2026-07-23: fit ran at the DEPLOYED seats — intent-filling@4B + contradiction-screen@9B calibrated; fast watchdog/watchlist do not meet the bar at the 4B seat → `calibrated=false` honestly]**; calibrated values are platform data keyed by (duty, model hash, engine build) — which is exactly what the S12.10 swap gate keeps valid.

Escalation targets: registry duties escalate to the requester's paid ceremony/verification seat *when a window is open* — billed to the requester, itemized (1.10, 3.4) [XREF:S06; XREF:S07; XREF:S10]. With every paid window empty, everything in the registry keeps running at local quality and escalations queue as parked work — **this table is 3.9's "whatever remains feasible" boundary, made concrete** [R16 §4.3]. The `watchdog-disambiguator` never escalates beyond `unclear` (S2.7). If a duty's acceptance bar proves unreachable at this scale, the duty runs degraded-with-disclosure locally and only its *mandatory* path may shift to a paid seat; watchdog duties instead widen their `unclear` band — they never gain a cloud dependency [R16 §4 "What would change"].

### S12.6 The GPU broker (R10-OQ4 discharged)

Two caller planes [R16 §4.7, §3.4]:

- **Platform plane:** `sinet-control` and adopted watch organs reach llama-swap directly on loopback [S01.1]. All management verbs (load/unload/config) exist **only** on this plane.
- **Sandboxed plane:** sandboxes never see `/dev/nvidia*` and never a routable host address [R16 §4.7; XREF:S11]. A worker granted local inference receives exactly: `POST /v1/chat/completions` + `POST /v1/embeddings`; `/v1/models` returns only the run's allowlist; transport is a per-sandbox loopback/unix-socket bridge injected by the sandbox runtime [XREF:S11].

Binding properties:

- **Identity:** a per-run bearer token minted by the control plane at spawn (virtual-key *pattern*; no proxy product adopted) — model-alias allowlist + rate/size limits + expiry = run lifetime. The GPU broker resolves token → run → owner and writes every call to the D7 ledger ($0, pressure floor, itemized) [R16 §4.7; XREF:S10].
- **The model field carries a duty alias, never a raw model id** (S12.4).
- **Excluded by construction:** management verbs; logprobs for sandboxed callers (⚙ `local.broker.sandbox_logprobs`, default off — side-channel reduction); rerank/completions until a template needs them [R16 §4.7].
- **Confinement interaction:** C1 workers may be granted the bridge; C3 workers (v0.1) get tighter rate/size budgets — the broker's logs make steered inference auditable; C0 connectors never get it [R16 §4.7; XREF:S11]. gVisor nvproxy stays a non-path unless a future capability genuinely needs in-sandbox CUDA (pre-registered probe, unchanged) [R16 §4.7].
- **Admission:** before any model load, the GPU broker runs the S12.7 VRAM-ledger check — **these are the mechanics behind the scheduler's GPU-admission policy hook**; whether the *work* is admitted at all (pressure, budgets, background caps, priorities) is S10's decision [G2 D2.1; XREF:S10].

### S12.7 VRAM ledger & GPU preemption

- **Ledger values are per-machine measurements, never documented constants** [R16 §4.4]. They are produced by the R09-OQ8 protocol — 7 steps, carried by reference [R16 §4.4]: (1) pool enumeration by UUID; (2) **sleep-gate every read** — check `power/runtime_status` first; a suspended GPU is recorded as "asleep, 0 client bytes" and never polled (polling wakes it); (3) device truth from device-level `memory.free`, never per-process sums; (4) per-process attribution including graphics clients (compute queries miss every graphics process); (5) per-(model, quant, context, slots, engine build) footprint measured through a warm-up generation (KV + CUDA graphs only fully materialize then) + guard band ⚙; (6) compositor headroom logged per display mode — hybrid and MUX separately; **hybrid is the recommended, VRAM-maximizing mode**; (7) live-admission verification. Re-run on driver upgrade, engine bump, model/quant change, or display-mode change. TBD-BRINGUP(VRAM-ledger calibration run).
- **Admission math:** a load is admitted iff live `memory.free` ≥ ledger(model) + ⚙ `local.vram.guard_band_mb` — always the live reading, never the ledger's belief; after any forced kill, `memory.free` recovery MUST be verified before the next admission (zombie VRAM exists) [R16 §4.4].
- **Preemption is kill-not-freeze.** Freezing is categorically wrong for VRAM: a stopped process keeps its CUDA context and allocations; the driver frees memory only at teardown. Freeze/thaw remains the CPU/RAM tool [XREF:S10]; **unload/kill is the GPU tool** [R16 §4.5, §5]. Backend stop discipline: SIGTERM → ⚙ grace → SIGKILL — the kill path is mandatory, not a fallback (`llama-server` can wedge and ignore SIGTERM) — then verify recovery per the admission rule [R16 §4.5].
- **Granularity:** generation cancels between decode steps (~ms); a running prefill batch cannot be interrupted — worst-case *voluntary* preemption ≈ one prefill batch. When the operator needs VRAM now, the process is killed (the eager-unload verb, S12.2) [R16 §4.5]. One backend process per loaded model keeps CUDA-context overhead (~300–800 MB class) inside the measured footprint [R16 §4.4].

### S12.8 Power & residency policy

**Revision, stated:** report 09 §4.9 sketched a small always-resident GPU slot for background intelligence. **That sketch is REVISED by R16** [G3 digest]: NVIDIA RTD3's deepest state (VRAM off) requires used VRAM below the `NVreg_DynamicPowerManagementVideoMemoryThreshold` — default 200 MB, max 1024 MB — so **an always-resident multi-GB model and deep GPU sleep are mutually exclusive**; a resident slot would buy 7–18 W-class idle-awake (hundreds of Wh/day) for latency no duty needs — every registry duty is an event-driven annotation or batch pass, none latency-critical below ~10 s [R16 §2.6, §3.3, §4.6].

Policy instead (all ⚙) [R16 §4.6]:

- **On AC, active platform hours:** TTL warm residency — models stay loaded while duties flow, unload on TTL expiry; the OS page cache is the true residency layer (~2 s reloads for the 4B, ~3–7 s for the 9B, including spawn). The operator may pin the fast tier into the `resident` group during heavy runs (an operator verb, empty by default).
- **On battery:** nothing resident on the dGPU. Watchdog verdicts and the tiniest screens run on the **CPU tier** (burst CPU cost is fan-invisible); larger duties load-on-demand (accepting a wake) or park until AC per urgency class (⚙ `local.battery.gpu_admission`).
- **Batch passes** (compaction summaries, contradiction sweeps, benchmark evals) are **AC-only** (`ConditionACPower=` + a udev power-supply trigger for immediate reaction) and SHOULD run in the operator's daytime off-hours window — sustained batch is what raises fans; bursts are energetically trivial (~0.03–0.05 Wh) [R16 §2.6, §4.6].
- **Monitoring discipline:** every GPU read sleep-gates on `runtime_status` (S12.7); Tier-1 watching is event-driven with **no periodic GPU wake anywhere** — monitoring never defeats RTD3 [R16 §4.8].
- **Clock/power hygiene:** background inference runs under `-lgc` clock caps (`-pl` is unavailable on mobile GPUs); `nvidia-persistenced` in UVM persistence mode is NEVER run (it disables RTD3 outright) [R16 §2.6, §5].
- **The watchdog floor is never dark and never cloud:** Tier 0 = deterministic counters over the event log — no GPU, always on; Tier 1 = event-driven local verdicts, falling to the CPU tier when the dGPU is contended or asleep. **S2.7 and 3.9 hold by construction, not by a reserved slot** [R16 §4.5, §4.8]. Watchdog thresholds and machinery are [XREF:S14].

### S12.9 Bring-up battery (G3 Def.8)

The **T15 battery** is this section's acceptance instrument: built once at bring-up, versioned in the platform repo, run via the pinned eval runner against the *exact deployed quant and engine build*, results recorded in `platform.db` [R16 §4.9; XREF:S14 for runner + golden-set governance]. Contents: ~30–50 labeled cases per registry duty (synthetic seeds + real traces as they accumulate) plus the quant sanity check (`llama-perplexity --kl-divergence` on ~250K tokens of Sinet-domain text against the FP16 baseline). Named measurements:

1. TBD-BRINGUP(workhorse bakeoff) — Qwen3.5-9B vs Gemma 4 12B QAT on the battery over real Sinet-domain traces; **the 9B ships as default alias target unless the bakeoff flips it** [G3 Def.8].
2. TBD-BRINGUP(VRAM-ledger calibration) — the S12.7 protocol on the real machine: per-model footprints, both compositor-headroom figures (hybrid + MUX), unload/kill→free latencies [R16 §7-OQ4].
3. TBD-BRINGUP(CPU-tier throughput floor) — 3–4B Q4 tokens/s on the host CPU (the ~20–35 t/s figure is a class estimate with no citable laptop source); decides how much of the battery-mode policy is real and whether a 2B CPU downshift is needed [R16 §2.6, §7-OQ5].
4. TBD-BRINGUP(contradiction-screen P/R) — measured operating point of the DeBERTa→workhorse two-stage screen on realistic lesson pairs (R11-OQ7, narrowed); literature predicts high precision / weak recall with temporal/numeric blind spots [R16 §2.5, §7-OQ2]. **NARROWED A6 (2026-07-23, B4 gate)**: closed for the SHIPPED one-stage workhorse shape — P=R=1.0 on the 12-pair synthetic seed (P3-B4-7); DeBERTa `Servable:false` at the pin (no GGUF), so the two-stage pre-screen re-enters when a serving path exists; recall against hard real pairs sharpens under golden-set governance (result file `P3/measurements/2026-07-22-contradiction-screen-pr.md`).
5. TBD-BRINGUP(entailment thresholds + mandatory-coverage bar) — Guardian (3.3 vs 4.1) vs MiniCheck on ~200 Sinet-domain claim–citation pairs; sets the thresholds and the coverage bar, and decides whether the Flan-T5-0.8B CPU floor is adequate for sampled checks [G3 Def.4; R16 §7-OQ3]. **NARROWED A8 (2026-07-23, B4 gate)**: Guardian 4.1 = 152/156 (97.4%) on the 156-pair set clears the ≥0.90 MAIN bar (per-side TPR 0.949 / TNR 1.000); the load-bearing 0.95 sub-bar is NOT met (TPR 0.949) → mandatory-coverage stays conservative, gate idle; Flan-T5 serves but the generic path can't discriminate (CPU floor rests on Guardian); ⚙ `verification.entailment_sample_rate` DERIVED 0.20, LIVE write = bring-up; 3.3 leg + 3.3-vs-4.1 head-to-head = bring-up (result file `P3/measurements/2026-07-22-entailment-thresholds.md`).
6. TBD-BRINGUP(per-duty confidence calibration) — the S12.5 margin→isotonic→threshold fit for every registry duty [R16 §4.3]. **NARROWED A7 (2026-07-23, B4 gate)**: fit ran at the DEPLOYED seats (P3-B4-7) — calibrated: intent-filling@4B + contradiction-screen@9B (+ entailment@Guardian); watchdog/watchlist do not meet the bar at the deployed 4B seat and intake-triage can't-separate (subjective `size`) → `calibrated=false` honestly; durable `calibration_records` writes (migration 0010) at the deployed-seat keys are a bring-up act (result file `P3/measurements/2026-07-22-per-duty-confidence-calibration.md`).

### S12.10 Lifecycle: re-evaluation, swaps, recalibration

- **Cadence:** full re-evaluation every ⚙ `local.reeval.cadence_months` (6), or sooner on watchlist drift events — "new open ≤40B family" is a registered drift class on the provider watch [R16 §4.10; XREF:S14].
- **Method** [R16 §4.10]: shortlist from the current small-open index + adoption signals, **license verified on the HF card itself**; duty-relevant leaderboard pass (summarization grounding, entailment, SQL, structured/tool, judging — leaderboards churn as fast as models, so the method outranks any snapshot [R16 §2.2]); local KL quant check; the T15 battery at the deploy quant; **promotion rule:** a challenger replaces an incumbent only if it wins ≥2 duty suites with non-overlapping 95% Wilson intervals at equal-or-better tokens/s.
- **Swap ⇒ recalibrate is a hard gate (P-T15-1).** Every model, quant, or engine-build swap silently invalidates per-duty confidence thresholds and calibration maps. A swap MUST re-run the S12.5 calibration and the battery's affected duty suites *before* the new target goes live behind its alias; a swap without recalibration is a silent-quality regression [R16 §7-OQ8, §4.10].
- Every swap also fires the **7.3 revalidation trigger**: worker versions tuned against the swapped alias are flagged and revalidated before further unsupervised use [7.3; XREF:S08].
- **Serving-engine bumps** (llama.cpp `b`-tag, llama-swap release, Ollama if ever activated) follow the S03.3 deliberate-bump procedure; the local lane's conformance entries assert `/v1` logprobs, `json_schema` enforcement, and the llama-swap YAML/endpoint contract **behaviorally** on every bump — documentation is never the check (P-T15-2) [XREF:S03; XREF:S14; pins XREF:S16].
- Aliases make every swap invisible to workers, templates, and platform callers (S12.4, S12.6) [R16 §4.7].

### S12.11 Post-v0 envelope

- **eGPU pool:** the RTX 3090 24 GB enrolls as a second, separate VRAM pool (`pool24`); the per-duty ceiling rises to the 24 GB envelope post-v0 [G1 P9]. It is a **resident-model pool by design** — TB4 model load is ~7× slower while the runtime penalty is small; it is not a swap pool [R16 §2.6, §4.6].
- **Pre-registered pool24 seats** (re-run through S12.10 at enrollment, never auto-adopted): Qwen3.6-27B (quality), Qwen3.6-35B-A3B (throughput), Gemma 4 31B QAT (summarization-leaning), Arctic-Text2SQL-R1-14B/32B (SQL quality path) [R16 §2.1, §4.2].
- **Attach/detach:** `boltctl enroll` once; attach = plug → verify enumeration → start `pool24` members. Detach = choreographed runbook: unload pool → verify no `/dev/nvidia*` holders → unplug; cable-yank = assume reboot. Absence = health-check failure ⇒ pool marked absent ⇒ duties fall back to pool12/CPU or park — **no schedule may require the eGPU** (optional hardware) [R16 §4.6]. TBD-OPERATOR(eGPU on-rig validation at enrollment — token-rate penalty, load times, detach rehearsal; R16 §7-OQ6).
- **vLLM** is the pre-registered `pool24` batch backend under llama-swap, installed only if a real throughput need materializes [R16 §4.1].
- **Parked:** speculative decoding (first-party draft models exist; a battery entry only if duty latency ever matters) [G3 §Follow-ups; R16 §7-OQ12] · local image generation (12.1) — a separate SD-class serving stack; nothing here blocks it and the broker's alias pattern extends to it; no v0 work [G3 §Follow-ups; R16 §7-OQ11].

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `local.ttl.fast_s` | 120 | 0–3600 `[coordinator-draft]` clamp | [R16 §4.1]; G1 rider 1 |
| `local.ttl.workhorse_s` | 300 | 0–7200 `[coordinator-draft]` clamp | [R16 §4.1]; G1 rider 1 |
| `local.ttl.egpu_s` | 1800 | 0–86400; dormant until pool24 | [R16 §4.1]; G1 rider 1 |
| `local.vram.guard_band_mb` | 512 | ≥128 `[coordinator-draft]` clamp | [R16 §4.4 step 5] |
| `local.unload.term_grace_s` | 5 | 1–30 `[coordinator-draft]` clamp | [R16 §4.5] |
| `local.gamemode_hook` | on | {on, off} | [G2 Def.6] |
| `local.battery.gpu_admission` | `urgent-only` | {never, urgent-only, always} — name `[coordinator-draft]` | [R16 §4.6] |
| `local.batch.ac_only` | true | {true, false} | [R16 §4.6] |
| `local.broker.sandbox_logprobs` | off | {off, on}, per-template | [R16 §4.7] |
| `local.reeval.cadence_months` | 6 | 1–12 | [R16 §4.10] |
| `local.alias.<duty>` (map) | per the S12.4 table; workhorse default = Qwen3.5-9B | changes only via the S12.10 swap gate | [R16 §4.7]; [G3 Def.8] |

All per G1 rider 1: operator-editable, audit-trailed, auto-adjustment only within operator ceilings, visible on receipts.

**Known problems owned here:**
- **P-T15-1 — model churn invalidates calibration** (id coined here for R16's first filed problem [G3 §Follow-ups]): dispositioned as the swap ⇒ recalibrate + revalidate hard gate (S12.10); no alias retarget goes live uncalibrated.
- **P-T15-2 — stack-capability drift; docs lie, probe behavior** (id coined here for R16's second filed problem [G3 §Follow-ups]): logprobs/`json_schema` are load-bearing for canaries and confidence; dispositioned as behavioral conformance probes on `/v1` at every engine bump (S12.2, S12.10); machinery [XREF:S14].
- Referenced, owned elsewhere: P-T01-2 (schema drift; [XREF:S03]) · P-T14-1 (engine-pin bumps = mass revalidation; [XREF:S08]) · llama-swap bus-factor-1 = accepted risk with the pre-registered fallback ladder (S12.2) and a `components.lock` exit plan [XREF:S16].

**Deferred / parked:**
- eGPU `pool24` activation + per-duty 24 GB ceiling → re-entry: hardware enrolled (S12.11) [G1 P9].
- vLLM batch backend → re-entry: measured throughput need on pool24 [R16 §4.1].
- Speculative decoding → re-entry: a duty latency requirement appears [G3 §Follow-ups].
- Local image generation (12.1) → re-entry: post-benchmark-gate content work; separate serving stack [G3 §Follow-ups].
- Local helper-report plausibility screen → re-entry: battery ≥ pre-registered bar on real helper outputs [G3 Def.3].
- Local fine-tuning → re-entry: the battery proves a persistent, material gap no off-the-shelf specialist closes [R16 §5].
- Automatic idle-detection (GPU auto-pause/resume) → re-entry: post-v0 [G2 Def.6].
- Ollama / router-mode fallback activation → re-entry: llama-swap unmaintained or contract break (S12.2 ladder) [R16 §4.1].
- 2B CPU downshift for the fast tier → re-entry: TBD-BRINGUP(CPU-tier throughput floor) outcome (S12.9).
- ~~TBD-P3(GameMode user-context probe)~~ — **CLOSED A5 (2026-07-23, B4 gate)**: ran live at gamemoded v1.8.1 (P3-B4-5) — signals are per-user SESSION-bus only, not reachable from the `sinet` system subscriber without an operator `DBUS_SESSION_BUS_ADDRESS`; the `gamemode.ini [custom]` scripts leg carries pause/resume, the busctl subscription is deferred-with-finding (result file `P3/measurements/2026-07-22-gamemode-user-context-probe.md`).

**Coverage:**

| Feature-list item | Subsection |
|---|---|
| Operating reality — local GPU as permanent free tier; background intelligence local-only, zero allowance, works when windows are empty | S12.1, S12.4 |
| 3.9 never fully offline | S12.4, S12.5, S12.8 |
| 3.11 local resources scheduled; operator's interactive use always wins (GPU part; CPU/RAM [XREF:S10]) | S12.2 (eager-unload + GameMode), S12.6–S12.8 |
| S2.7 self-watching runs local, no allowance | S12.4 (`watchdog-disambiguator`), S12.8 (floor) |
| 1.10 utility model + ceremony cut line (map at S06.10) | S12.4 |
| 7.3 model-swap revalidation trigger (local side) | S12.10 |
| 12.1 local image gen (awareness only) | S12.11 (parked) |
| D4/D5 — free tier measured exactly, pressure floor (policy [XREF:S10]) | S12.1, S12.6 |

**Open items for G4:** none. Drafting-time notes flagged for G4 attention: (a) `[coordinator-draft]` items — the alias names (S12.4), the `sinet-llamaswap.service` unit name (S12.2), the clamp ranges and the `local.battery.gpu_admission` knob name (⚙ table); (b) resolved-tension note: G1 P9's "≤8B" is applied as the ≤8B-class / 12-GB-pool envelope — the reading R16 held fixed and G3 ratified by naming the 9B (Def.8) and 7B (D3.5) seats (S12.3); (c) P-T15-1/-2 ids are coined here for R16's two filed problems, for pickup by the S17 register.

## S13 — Deliverables, review, git & backup

**Scope:** Everything a task produces on its way out of the platform: the deliverable/revision/comment schema, per-type review behavior, the findings→retry drain, git topology and the broker-mediated accept flow, the project/repository registry, disposable previews, follow-up lineage, and the 11.3 encrypted-snapshot + restore-drill pipeline.
**Binding inputs:** R13 §4 as ratified [G2 D2.1(e)] · `Spec/frontend-components-v1.md` §2 (the carried N15 behavior spec — binding; cited below as [FC-v1 §2]) · G2 D2.5 (backup posture), D2.6 (git identity), Def.13 (PDF), Def.14 (binaries) · G1 rider 1 · feature list D2, D7, D9, 5.4–5.5, 6.1–6.4, S1.2, S1.6, S1.8–S1.9, S1.11, S3.5, 2.6, 11.3 · siblings S01 (units, portpool, Caddy, timers, CI) and S02 (durable set, claims, effect journal, workspace seam) · P-T12-1..4.

Terms coined here: **deliverable** — the long-lived reviewable entity a task produces (one per task output). **revision** — one immutable numbered snapshot of a deliverable (1..N). **orphan** — the explicit anchor state of a comment that no longer attaches anywhere in the current revision. **drain point** — the single code path that hands review feedback to a retry. **snapshot commit** — a platform-owned tree-level commit on a run branch (the safety net and revision raw material). **merge card** — the reviewable decision card produced when an accept does not apply cleanly. **snapshot ledger** — the SHA-256 integrity record of every 11.3 snapshot. **escrow identity** — the off-host recovery copy of the operator's age identity (paper + passphrase-encrypted file).

### S13.1 Deliverable & revision schema

The Gerrit/Phorge shape [R13 §4.1; G2 D2.1]: a **deliverable** is a long-lived entity; its content history is **revisions 1..N**, immutable once minted, every revision hop persisted. Lineage is NEVER compressed (GitHub's force-push timeline compression is the named anti-pattern) [R13 §2.2/§4.1].

Table families (rows in `platform.db`, owner-attributed per 15.6, under all S02.1 disciplines; indicative names — exact DDL at the P3 schema workshop jointly with S02.2):

| Table family | Holds |
|---|---|
| `deliverables` | task ref, project ref, type, current revision, state (in-review / accepted / superseded), owner |
| `deliverable_revisions` | deliverable ref, number 1..N, minting run/attempt ref, content pin (S13.1 below), verification-verdict ref [XREF:S07], created-at |
| `review_comments` | one schema for human comments and verification findings (S13.3): anchor record, anchor status, body, lifecycle, consumption stamps; findings add `{severity, description, suggested_change?, finding_number}` [R13 §4.1/§4.4] |
| `repo_registry` | the S1.6 registry (S13.7) |
| `snapshot_ledger` | the 11.3 integrity ledger (S13.10) |

These families join the S02.9 durable set by reference — they are part of the 11.3 snapshot payload.

- **Everything arrives as a reviewable change (6.1).** Revision 1 is presented against the pre-task state (project HEAD at branch base); revision N against N−1 by default (the default rework view [FC-v1 §2]); any revision pair is diffable on demand — revision-over-revision navigation is a schema capability, not a UI nicety [R13 §4.1].
- **Content pin.** A revision pins its content immutably: repo-backed types pin a snapshot-commit sha (S13.5); binary types pin content-addressed object-dir hashes (S13.2). Minted revision refs are retained in the platform-owned project store under a platform ref namespace (`refs/sinet/deliverable/<id>/rev-<n>` [coordinator-draft]); workspace GC MUST NOT delete the only copy of a minted revision's objects [XREF:S02].
- **Minting.** A revision is minted when a candidate passes to review — one per round; the verification handoff that triggers minting is [XREF:S07].
- **Automation definitions ride the same machinery.** A newly composed worker/automation definition presents at birth as a deliverable — readable diff, commentable, previewable (2.6); the composer's approval-as-diff station consumes this schema [XREF:S08].

### S13.2 Review surfaces per type (6.1, S3.5 — behavior normative here, rendering [XREF:S15])

Every deliverable type has a defined comparison behavior at v0; no type is refused, and a type without a rich surface gets the honest fallback, visibly labeled (the 2.1 honesty posture).

| Type | v0 comparison behavior | Source |
|---|---|---|
| Code / text (incl. markdown source) | Side-by-side and unified line diff with intraline highlighting; the unified diff is computed host-side (`git diff` between revision pins) and rendered by the S15 diff widget | [R13 §4.3; FC-v1] |
| PDF | **Extracted-text diff only at v0** (MIT-licensed extraction; AGPL extractors excluded as defaults); pixel-overlay lane is post-v0 | [G2 Def.13; R13 §4.3] |
| Images | Visual compare, GitHub's trio: 2-up / swipe / onion-skin over the two revisions; optional server-side pixel-diff as an aid | [R13 §2.1/§4.3] |
| Binaries / other opaque | Content-addressed **local object dir + hash refs from committed text**; the review surface shows a metadata card (name, size, hash, type) per side and a changed/unchanged verdict by hash; download to inspect | [G2 Def.14; R13 §4.6] |
| Any other text-extractable | Plain extracted-text diff, labeled as the fallback surface | [R13 §4.3] |

- Binary bytes are host-local review material outside the 11.3 payload; the committed hash refs make loss detectable and regeneration targeted. A binary that must be reviewable on GitHub itself is promoted to LFS — the only sanctioned exception [G2 Def.14].
- Semantic docx redlining (Python-Redlines, OOXML-embedded comments) and pandiff prose diffs are the parked enhancement lane, gated on the R13-OQ7 license/maintenance audit [XREF:S16] (see Deferred).
- Cross-format compare has no OSS equivalent (buy-not-build territory); Sinet stays per-format by design [R13 §2.1].

### S13.3 Comment & anchor model — the carried N15 behavior contract

This subsection is the normative behavior contract for anchored review; the widget that renders it is S15's binding pick [FC-v1; XREF:S15]. Human review comments (6.2) and verification findings [XREF:S07] share this one schema [R13 §4.1].

- **Anchor record** (binding shape): `{file_path, side ∈ {old|new}, line_no, line_text}`, bound to (deliverable, revision) [FC-v1 §2]. `line_text` is the quote selector, `(side, line_no)` the position selector — the W3C redundancy principle ("selectors hint each other") in compact, closed-world form [R13 §2.2].
- **Server-side re-validation.** Anchors are re-validated server-side on every render and every port to a new revision; client- or agent-supplied positions are never trusted as placement authority [FC-v1 §2].
- **Re-anchoring ladder** — porting a comment onto revision N+1 tries, in order [R13 §4.2; FC-v1 §2]:
  1. Map `(side, line_no)` through the known N→N+1 diff (Sinet's closed world always has the diff — strictly easier than the open-web problem).
  2. Exact `line_text` match at the mapped position.
  3. `line_text` search within `⚙ review.anchor_drift_lines` (default ±2) of the mapped position.
  4. Degrade to a **file-level comment** (Gerrit's next-best-location contract, line→file), keeping the original quote visible.
  5. **ORPHAN** — an explicit schema state that keeps the quote and the original-revision link, stays visible on the review surface, and still drains (as file-level). Never silently dropped. Hypothesis's ~22% open-web orphan rate is the proof this state is mandatory even closed-world (P-T12-2) [R13 §2.2/§4.2].
- **Anchor status is recorded** per comment per revision (`exact / mapped / drifted / file / orphan`). Silent mis-anchoring is worse than orphaning; anchoring quality gets an 11.2 spot-check (P-T12-2) [XREF:S14].
- **Lifecycle:** `open → consumed`. Consumption is batch-stamped — one `consumed_at` plus the consuming attempt ref for the whole drained batch — so "what did that rework receive" is auditable after the fact [FC-v1 §2]. New round → new comments; a consumed finding re-review deems unfixed produces a fresh finding [XREF:S07].
- **Synthetic render surface.** Any comment without a live anchor in the current view (file-level, orphaned, cross-cutting) renders in a dedicated, always-visible strip of the review surface. Invariant: **no comment may exist without a render location** — the class of the real 44-invisible-comments failure this rule was born from [FC-v1 §2].
- **Escape-first rendering.** All deliverable and comment content renders escaped/text-first on every surface; exactly **one** sanctioned raw-HTML channel exists — the sandboxed rendered-document view [XREF:S15] — and nothing else may render raw markup [FC-v1 §2]. The contract is normative here; enforcement mechanics are S15's.
- For docx deliverables (post-audit lane), comments are additionally embedded natively in OOXML so users' own tools see them [R13 §4.2] (parked with the docx lane).

### S13.4 The findings→retry drain point (5.5)

Exactly **one** code path hands review feedback to a retry [FC-v1 §2]. At rework dispatch it:

1. Collects every `open` comment/finding on the deliverable's current revision.
2. Numbers them `[F1..Fn]` (stable within the attempt), each carrying anchor (or its file/orphan degradation) + severity + description + optional suggested change — the finding schema of S13.1 [R13 §4.4].
3. Marks the batch `consumed` with the shared `consumed_at` + attempt ref (S13.3).
4. Delivers the numbered points into the next attempt's brief [XREF:S05].

- Orphaned and file-degraded findings are **still delivered** — delivery is never conditional on anchoring success (P-T12-2).
- Severity distinguishes blockers from notes: only genuine blockers trigger another round; polish travels as notes (5.4). Rework bounds, re-review against the frozen ACs, and the verification ladder that generates machine findings are [XREF:S07].
- **Pre-registered benchmark question (R13-OQ6):** whether anchored `[F#]` findings measurably beat file-level notes for retry quality is empirically open field-wide; it is registered with the 11.2 practice [XREF:S14]. If the answer is no, the ladder simplifies to file-level numbered findings [R13 §4.10] — the schema above degrades to that shape without migration.

### S13.5 Git topology: branch-per-pipeline + platform snapshot commits

- **Branch-per-pipeline.** Each pipeline gets one long-lived run branch in its workspace (substrate: worktree + overlayfs per S02.10 [XREF:S02]; sandbox composition [XREF:S11]), accumulating commits across stages and review rounds. During a review round-trip: same branch, new commits — the platform NEVER amends or force-pushes a branch under review (zero field support for mid-review rewrites) [R13 §4.6/§2.5].
- **Fresh branch per attempt** when the base moved or an attempt is abandoned (the attempt model); the old attempt's branch stays until revision-retention GC clears it [R13 §4.6].
- **Snapshot commits** are platform-owned, tree-level (they capture bash side effects, not just tool edits), junk-excluded via platform ignore rules, written at checkpoint boundaries — where they serve as the Claude-lane checkpoint artifact ref [XREF:S02 §S02.4d] — and at stage/round boundaries, where they are revision raw material (S13.1). They are squashed away at accept (S13.6); they never reach a user-facing remote.
- **The net is the commits — never:** engine checkpoints (bash-blind, session-scoped, 30-day expiring), `git stash` (untracked-file skips, documented data loss), or jj (unsupported worktrees/hooks/submodules/LFS; zero mainstream adoption; parked at S02) [R13 §2.6/§4.6].
- **Platform-owned utility checkouts** (accept staging, preview-at-rev) are plain worktrees with the shipped lifecycle rules: `worktree lock` while in use, `worktree prune` sweeps, never auto-delete dirty [R13 §4.6; XREF:S02 GC].
- A Gemini-style shadow repo for mid-step capture is deliberately not built; it is a cheap bolt-on if mid-step capture ever proves necessary (Deferred) [R13 §4.6].

### S13.6 Accept flow (6.3, D9) — one action, broker-mediated

Accepting is one action on the deliverable's approval card (risk tier High — outward push [S3.2]). The push executes through the effect journal as a **class-A effect** — `git push --force-with-lease=<ref>:<expect>` is S02.7's own natively-idempotent exemplar; the CAS expect-sha and the candidate revision pin are the payload hash pinned at approval [XREF:S02]. The flow, host-side [R13 §4.5]:

1. **Applies-cleanly gate.** The candidate revision is applied to current project HEAD on a throwaway ref (a depth-1 merge queue; a second in-flight accept validates against HEAD-plus-first). A collision surfaces as a reviewable **merge card** — options: agent-auto-resolve (a bounded rework [XREF:S07]) / resolve manually / abort to a new attempt — never a silent overwrite (S1.11 verbatim; consistent with S02.8, which owns claims and the sibling-accept freshness trigger) [XREF:S02].
2. **One clean attributed commit:** author = committer = accepting user, set per-invocation (env/`-c` — no global host state); committer email is the ID-based noreply form (survives renames) or a connected address from the per-user store, so contribution graphs attribute; Conventional-Commit subject; body carries the task/session link as provenance. The run branch's snapshot commits are squashed by the **platform** — never by GitHub UI (the squash-authorship hazard is thereby structurally avoided) [R13 §2.4/§4.5].
3. **Attribution trailers**, deterministic and displayed on the card **before** accept (the VS Code mis-attribution lesson): `Co-Authored-By: <engine> <model> <vendor-noreply>` (GitHub renders it) plus `Assisted-by: <engine> (<model>) via Sinet` as the machine-parseable provenance line. Trailer text is fixed platform config in the settings registry (string entries, audit-trailed), never per-run improvisation [R13 §4.5].
4. **Push as the user** over broker-held SSH keys with explicit CAS: `--force-with-lease=<ref>:<sha-at-approval>`; multi-ref updates use `--atomic`; pacing respects GitHub's documented ≤6 pushes/min/repo and 2 GB push cap (household volume is orders below both). A lease failure is a normal collision → back through step 1 to a merge card; NEVER a blind retry against a new sha [R13 §2.4/§4.5].
5. **Signing [G2 D2.6]:** the operator SSH-signs from day one; members opt in at enrollment. All-or-nothing per user — a user whose commits are not all signed MUST NOT enable vigilant mode. The broker performs signing (`gpg.format=ssh`); keys never leave it [XREF:S11]. gitsign/Sigstore is NEVER used (renders Unverified on GitHub — worse than no signature). No GitHub Pro at v0; revisited when a second member joins [G2 D2.6].
6. **Enrollment (one-time per member):** keypair generated inside the person's store on the host (never leaves it; store hygiene [XREF:S11]); the user uploads the pubkey to their GitHub account (auth + optionally signing); the ID-based noreply address is captured into the store; repo owners invite collaborators (only owners can — onboarding steps route to the owning user). Owner-exit path: repository transfer (1-day acceptance; stored remotes are rewritten — redirects are never trusted) [R13 §4.5].

**P-T12-1 — THE BROKER IS THE GUARDRAIL.** Fine-grained PATs cannot push to repos where the holder is an invited collaborator (documented resource-owner gap) — SSH is forced, not preferred. And on Free private repos GitHub enforces **zero** branch protection — any collaborator credential could force-push main. Therefore, as a security property of the platform:

- Member git credentials exist ONLY inside the broker (D2); they never enter sandboxes and are never exported [XREF:S11].
- The broker refuses any push to a protected ref that does not originate from an approved accept effect; protected-ref policy is registry data (S13.7), default = the project's default branch, "main only via accepts."
- Every broker push is CAS (`--force-with-lease=<ref>:<expect>`; the bare form is forbidden — trivially defeated by background ref updates).
- **This property is tested:** a conformance-suite entry attempts a non-accept push to a protected ref and MUST observe a broker-side refusal [R13 P-T12-1].

ToS posture: one GitHub account per human, each person's runs pushed with that person's own credentials for that person's work — several humans' accounts operated from one host violates nothing found; the tripwire is pooling one person's token for others' work, which D2 already forbids [R13 §2.4].

### S13.7 Project & repository registry (S1.6)

The registry of known projects and repositories lives in `repo_registry` (S13.1): per entry — name/alias, local project-store path, remote URL, owning user, invited members, default branch, **protected refs** (broker policy input, S13.6), captured **conventions**, **commands** (build/test/lint/run/preview), and **danger zones**. Rows are owner-attributed; captured content is versioned.

- **Onboarding a repository is itself a task** the platform performs: register → clone into the project store → scan → draft conventions, commands, and danger zones → the owner approves the draft (D10 — their own object) → entry goes active. Re-scan on demand or when drift is detected.
- **The registry feeds:** intake resolution ("in the shop backend" needs no path-explaining) [XREF:S06]; conventions/danger-zones injection into stage briefs and the Ledger's pinned constraints [XREF:S05]; workspace creation [XREF:S02]; broker protected-ref policy (S13.6); preview commands (S13.8). Project-scope knowledge beyond repo facts is S09's [XREF:S09].
- **Documents ride the identical machinery:** D9's "document store" is the same per-project git repo — no surveyed platform maintains a separate versioning model for documents, and neither does Sinet [R13 §4.6]. A v0 project without a repo gets one created/registered by the onboarding task.

### S13.8 Previews (6.4, S1.9)

One click launches a built application from a deliverable revision in a disposable environment; nothing touches the live copy or the reviewed workspace.

- **Runner:** heuristic host-toolchain detection — `package.json` → pnpm/npm, `pyproject.toml` → uv, `mise.toml` → mise, `index.html` → static server (providers cribbed from Railpack) — executed **inside the settled sandbox stack** [XREF:S11]. No container daemon per preview; containers are not the substrate [R13 §4.8].
- **Zero mutation:** installs and preview writes land in a discardable overlay upper (or a throwaway clone when dependency-dir-sized) over the read-only revision checkout; teardown deletes the upper. The preview CANNOT mutate the reviewed workspace — sandbox composition is [XREF:S11] [R13 §2.8/§4.8].
- **Ports & routing:** dev server binds 0.0.0.0 inside the run-sandbox netns; the port comes from `sinet-portpool` [XREF:S01]; the platform **probes the netns for listening ports** rather than trusting config (multi-port → a picker). Routing is a Caddy admin-API route per live preview — subdomain/port-based, NEVER path-prefix (breaks arbitrary apps); WebSocket upgrade headers forwarded and `allowedHosts` injected, or Vite HMR silently dies. Household access rides the tailnet front chain [XREF:S01] [R13 §4.8].
- **Lifecycle:** `systemd-socket-proxyd --exit-idle-time` + `StopWhenUnneeded=` give on-demand start and idle-stop for free from the substrate — `⚙ preview.idle_stop` (default 15 min [coordinator-draft]); teardown releases the port and removes the Caddy route. Concurrency is capped at `⚙ preview.max_concurrent` (default 3 [coordinator-draft]) — preview dev servers compete with interactive use of the host (3.11; arbitration [XREF:S10]).
- **Before-vs-after (S1.9):** two instances — the accepted revision via an immutable worktree-at-rev checkout (S13.5 utility checkouts) and the candidate — in a dual-iframe view with synced navigation [XREF:S15]. The port-pool daemon [XREF:S01] and this comparison UI [XREF:S15] are the only novel code in the whole preview feature [R13 §4.8].
- **Non-web:** ttyd serves CLIs over the same routing; notebooks self-preview; static output gets the static server. Anything else (desktop/embedded) shows an explicit, honest **"no preview available for this type"** state — never a broken iframe [R13 §2.8/§4.8].
- **Sidecar-needing projects** (Postgres/Redis-class) surface as a labeled "requires container tier" state at v0; the rootless-podman tier is the named escape hatch, never the default (Deferred) [R13 §4.8].
- Previews are read-only with respect to the world: launching one is a Low-tier, reversible action (4.2 boundary untouched — no proposal needed); exposure exists only through the tailnet front chain [XREF:S01].

### S13.9 Follow-ups from deliverables (S1.2)

Any finished deliverable can spawn a successor task in one action. The successor row carries a `successor_of` link to (deliverable, revision); lineage is visible in both directions on the task detail and board cards [XREF:S15]. The successor enters normal intake with the project's inherited context plus the predecessor deliverable ref as brief material [XREF:S06; XREF:S05]. Revision / extension / counterpart framings ("now the English version") are intake presets over the same link, not schema variants. Concurrent follow-ups in one project are coordinated by the S02.8 claims machinery, and an accepted sibling fires the freshness trigger [XREF:S02].

### S13.10 Platform-state snapshots & the restore drill (11.3)

**Pipeline, per scheduled snapshot** [G2 D2.5; R13 §4.9]: the S02.9 durable set — `VACUUM INTO` a temp file → text-first `.dump` (raw trace payload bodies excluded; full traces stay local under 11.1 retention [XREF:S02]) — plus the platform's git-versioned file stores (worker templates, knowledge/memory files [XREF:S08; XREF:S09]) as text-first exports → `tar | zstd | age -r <operator recipients>` → **one encrypted blob per snapshot**, committed to the operator-owned private snapshot repo (D9), keep-N on one branch, pushed CAS by the broker. Cadence `⚙ backup.interval` = daily; retention `⚙ backup.keep` = 30 [G2 D2.5]. The daily timer rides S01.7's persistent calendar timers (a slot missed in suspend fires once on next activation) [XREF:S01].

- **Client-side encryption is unconditional:** a private repo is access-controlled, not end-to-end encrypted, and this data includes every member's memories and receipts (11.3). Nothing leaves the host unencrypted.
- **Secrets are vaulted separately and are NEVER in the snapshot payload** [11.3; G2 D2.5]: per-person credential stores and broker secrets ride the broker's own encrypted-at-rest mechanics; their backup/escrow is a broker duty [XREF:S11], deliberately decoupled from this pipeline.
- **Blob discipline:** stay under GitHub's enforced 100 MB file limit — chunk the archive at ~90 MB if ever needed; pushes sit far below the 2 GB cap. If snapshots outgrow repo comfort, the named escape hatch is release assets (<2 GiB/file, unlimited total) — also the closer-to-true-delete lane [R13 §4.9].
- **History bounding is client-side make-believe at the remote (P-T12-3):** GitHub retains rewritten-away blobs indefinitely (true deletion is a Support-run GC). Acceptable ONLY because every blob is age-encrypted — with the consequence that key compromise is retroactive over everything ever pushed. Mitigations: escrow hygiene (below), **annual snapshot-repo rotation** `⚙ backup.repo_rotation` = 12 months [G2 D2.5] (a fresh repo bounds what any one old key's remnants cover; the old repo is archived), and a Support-ticket purge as the nuclear option.
- **Recovery material (escrow identity):** age's spec forbids mixing passphrase and key recipients in one archive, so recovery is **a paper copy of the operator's age identity + a passphrase-encrypted copy of the identity file stored off-host**; snapshots encrypt to key recipients only. TBD-OPERATOR(age identity escrow created — paper + passphrase copy off-host — before the first snapshot push). An X-Wing post-quantum hybrid recipient is an optional add once the Go CLI plugin status verifies (watchlist [XREF:S14]) [R13 §4.9].
- **Integrity — the ledger is load-bearing (P-T12-4):** age has NO sender authentication; encryption proves who can read, never who wrote. Every snapshot's SHA-256 + size + created-at is recorded in `snapshot_ledger` (in `platform.db`) AND inside each subsequent archive. A compromised remote swapping blobs is defeated by the ledger check, nothing else.
- **Restore drill — scheduled and TESTED** (`⚙ backup.drill_interval`, default 3 months [coordinator-draft]): fetch the newest blob from the remote → verify against the ledger (**fail-closed on mismatch** — the drill aborts and raises a High flag [XREF:S14]) → decrypt with the **escrow** identity (proving the recovery material, not just the live key) → unpack → rebuild the DB from the dump → `integrity_check` + the S02.9 invariant assertions [XREF:S02]. Backup that is not restore-tested does not exist; the drill result is an event-log record either way.
- Litestream continuous replication remains the deferred *additive* lane (implementation phase, pending its silent-failure bug triage) [G2 D2.5; XREF:S01]; the dump-snapshot lane above is the load-bearing 11.3 mechanism.

---

**Settings introduced (⚙):** (all operator-editable with audit trail per G1 rider 1)

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `review.anchor_drift_lines` | 2 | 0 – 10 [coordinator-draft clamp] | FC-v1 §2 (carried N15 behavior) |
| `backup.interval` | 24 h (daily) | 6 h – 7 d [coordinator-draft clamp] | G2 D2.5 |
| `backup.keep` | 30 | 7 – 365 [coordinator-draft clamp] | G2 D2.5 |
| `backup.repo_rotation` | 12 mo | 6 – 24 mo [coordinator-draft clamp] | G2 D2.5 (annual) |
| `backup.drill_interval` | 3 mo [coordinator-draft] | 1 – 12 mo | R13 §4.9 (⚙ flagged unnumbered) |
| `preview.idle_stop` | 15 min [coordinator-draft] | 1 min – 24 h | R13 §4.8 (⚙ flagged unnumbered) |
| `preview.max_concurrent` | 3 [coordinator-draft] | 1 – 10 | 3.11 posture [coordinator-draft] |

**Known problems owned here:**

- **P-T12-1** — the broker is the only ref protection on Free private repos → stated as a security property (S13.6): broker-only keys, protected-ref policy from registry data, CAS-only pushes, and a mandatory broker-side refusal test in the conformance suite. Pro re-entry when a second member joins [G2 D2.6]; PAT-gap closure on the watchlist reopens transport choice [XREF:S14].
- **P-T12-2** — comment orphaning is guaranteed at some rate (~22% open-web baseline) → explicit ORPHAN state, synthetic render surface (no comment without a render location), orphaned-findings-still-delivered, all with tests (S13.3–S13.4); silent mis-anchoring gets an 11.2 spot-check [XREF:S14].
- **P-T12-3** — encrypted remnants make key compromise retroactive → escrow-identity hygiene, annual snapshot-repo rotation, Support-purge as last resort (S13.10).
- **P-T12-4** — snapshot substitution is undetectable by encryption alone → the SHA-256 snapshot ledger (DB + in-archive) and the fail-closed ledger verify in the restore drill are load-bearing (S13.10).

**Deferred / parked:**

- PDF pixel-overlay lane → post-v0; re-entry when PDF deliverables become routine (v0.1 web-research domain or operator demand) [G2 Def.13].
- docx native redlining (Python-Redlines/Docxodus + OOXML-embedded comments) and pandiff prose diffs → re-entry: R13-OQ7 license/maintenance audit passes [XREF:S16] AND a document-heavy domain activates (v1).
- LFS for binaries → only when a binary must be reviewable on GitHub itself [G2 Def.14].
- Rootless-podman container tier for sidecar-needing previews → re-entry: first real project that needs sidecar services; until then the labeled "requires container tier" state ships [R13 §4.8].
- Desktop-GUI preview (xpra-class) → v1+ at most; v0 ships the explicit no-preview state [R13 §2.8].
- Gemini-style shadow repo (mid-step capture) → only if mid-step capture proves necessary [R13 §4.6].
- jj as workspace/snapshot engine → parked at S02 (compat matrix + mainstream adoption are its re-entry, on the watchlist) [XREF:S02; XREF:S14].
- diffity (or successor) as a turnkey review component → re-entry per R13 §4.10 if it matures license-clean with comments+agents coverage; watchlist [XREF:S14].
- X-Wing PQ recipient for snapshots → re-entry when the age Go CLI plugin status verifies; watchlist [XREF:S14].
- Fine-grained-PAT collaborator gap / Free-plan enforced rulesets → watchlist; closure reopens broker transport choice / demotes the broker to defense-in-depth respectively [R13 §4.10; XREF:S14].

**Coverage:**

| Feature-list item | Where |
|---|---|
| 6.1 reviewable change + revision-over-revision navigation | S13.1, S13.2 |
| 6.2 comments on exact places feeding the next revision | S13.3, S13.4 |
| 6.3 accept = one action → attributed commit (D9) | S13.6 |
| 6.4 / S1.9 instant try-out + before-vs-after | S13.8 |
| 5.5 findings carried forward as numbered points (logic → S07) | S13.4 |
| 5.4 notes-vs-blockers travel (bounds → S07) | S13.4 |
| S1.2 follow-ups with visible lineage | S13.9 |
| S1.6 project/repository registry; onboarding-as-task | S13.7 |
| S1.8 native review in the workspace (surface → S15) | S13.1–S13.3 |
| S1.11 accept-time collision → reviewable merge (plan-time claims → S02) | S13.6 step 1 |
| S3.5 review surfaces per type (behavior; widgets → S15) | S13.2, S13.3 |
| 2.6 automation definitions presented as deliverables (governance → S08) | S13.1 |
| D9 per-project git, per-user remotes, platform snapshot repo | S13.5–S13.7, S13.10 |
| D2 credentials broker-held, never in sandboxes (git scope) | S13.6 |
| 11.3 encrypted snapshots + tested restore + secrets separation | S13.10 |

**Open items for G4:** none. Drafting-time sub-choices are flagged inline as [coordinator-draft]: the `refs/sinet/…` revision-retention namespace (S13.1), the restore-drill cadence (3 mo), the preview idle-stop (15 min) and concurrency cap (3), and the proposed clamp ranges in the ⚙ table. The anchor-model shape follows the binding frontend workshop artifact (layer 3) where it compacts R13 §4.1's selector sketch — recorded here for traceability, no conflict left open.

## S14 — Observability, evals, benchmark & provider watch

**Scope:** How Sinet sees itself and the outside world: the event-type contract that makes the D7 event log the complete observability stack, live-inspection semantics over the one SSE endpoint, the watchdog suite, the conformance-suite registry, the watchlist executor with the lane-canary layer, the machinery around the signed benchmark pre-registration, regression evals and the revalidation runbook, trace retention, and queryable history. Boundary lines: storage is [XREF:S02]; transport and units are [XREF:S01]; surfaces are [XREF:S15]; meters/receipts are [XREF:S10]; local model seats are [XREF:S12]; verification *logic* is [XREF:S07] — this section records and detects, it never judges.

**Binding inputs:** R12 §4 (observability package) [G2 D2.1]; G2 Def.10 (watchdog/inbox numbers), Def.11 (retention), Def.12 (watchlist executor), Def.15 (awesome-harness watch), Def.16 (genai-prices refresh); G1 D1.3 (pause-and-flag), G1 Def.3-as-superseded (SLA set: canary daily, drill quarterly); G3 D3.5 (Layer-2 SQL), G3 Def.2 (`settings.changed`); `Spec/benchmark-preregistration-v1.md` (signed commit `5fb7082`, verified via `Spec/allowed-signers`) — cited below as **BENCH-REG**, amendable only via its §17; [SPIKE P2-S1] (Z.AI no logprobs — headline via S03.7); registrations from siblings: S01.6–S01.8 (listener-binding audit, resume-reconcile), S02.9 (kill-9 + suspend-cycle), S03.3/S03.5 (bump gate, leak tests), S04.5 (D6 violation attempts), S05.7 (compaction canary), S06.7–S06.9 (forced escalations, delta-card measurement hook); P-T11-1..5.

**Term coined here:** **conformance registry** — the control-plane table of standing conformance suites and drills (id, owning section, fixtures, trigger set, schedule, last run, last result), the single scheduling home for every "proven, not assumed" obligation the spec declares (S14.5).

### S14.1 The event log IS the observability stack

`run_events` in `platform.db` [S02.2] is the **only** observability substrate: everything in S2.1–S2.11 is an event type, a view, or a subscriber on that log [R12 §4; G2 D2.1]. No external trace platform ever becomes a system of record: the self-hosted trace market is sized 3–5 orders of magnitude above household volume and is churning through M&A (Helicone → maintenance mode, Langfuse → ClickHouse, promptfoo → OpenAI within six months) — a system of record MUST NOT carry another company's M&A risk (P-T11-4) [R12 §2.1, §5].

- **Full trace per run (S2.1, 11.1).** The complete story — every step, tool call, artifact, gate, decision, verdict, and cost, in `event_seq` order — is reconstructable from `run_events` + `checkpoints` for the whole retention period (S14.9). journald is the ops log, never the audit record [S01.11].
- **OTel posture:** event field naming mirrors `gen_ai.*` vocabulary (operation kind, model, token counts, tool name, parent linkage) so a mechanical OTLP projector remains an afternoon's work; **no OTel dependency ships at v0** — the GenAI semconv is Development-status with renaming rights reserved [R12 §2.1, §4.1]. The projector is parked (see Deferred).
- **Adoption criteria for this whole layer** (P-T11-4, binding on S16 rows): permissive/forkable license, pinned version, pre-registered swap, and **the record always lives in Sinet's DB — tools are runners or projectors, never stores** [R12 §7].

### S14.2 The event-type contract (v0, complete)

Report 08 gave the envelope (`event_seq`, run_id, generation, type, `schema_version`, ts, payload — caps, refs-not-blobs, validate-before-persist all [S02.1/S02.2]); this section completes the **v0 event-type families and their contract-level required fields** [R12 §4.1; G2 D2.1; G3 Def.2]. Exact DDL is the P3 schema workshop's [S02.2].

| Family | v0 types | Required payload fields (contract minimum) | Anchor |
|---|---|---|---|
| Run lifecycle | `run.state_changed` · `stage.started/finished` | from→to + cause (every FSM transition appends [S02.3]); stage id/kind, stage-brief hash, outcome | S2.1; [XREF:S05] |
| Checkpoint | `checkpoint.written` | checkpoint id + the S02.4 (a)–(e) refs (usage block, session cursor, ledger revision, artifact snapshot, version fields) | D7; [S02.4] |
| Usage & limits | `usage.recorded` · `limit.event` · `run.parked/resumed` | per-paid-call token/cache fields + price-table version; limit class (five-class taxonomy [XREF:S10]) + provider signal + resets-at; park cause + parked-until | D4/D5; S2.5 |
| Gate / ask | `ask.observed` · `ask.answered` | ask id, kind (gate\|question), invocation-snapshot ref, answer ref — projected from `asks` [S02.2] | 4.2/4.3 |
| Human decision | `decision.recorded` | actor, card id + type, decision, presented-at → decided-at (latency); effect approvals additionally journaled [S02.7] | S2.4 |
| Verification verdict | `verdict.recorded` | rubric id + version, axis (spec-compliance\|outcome-sanity), per-criterion results, findings `[F1..Fn]` with anchors, blocker/note split, **round number**, judge model id + its golden-set error rates at judging time | S2.3; logic [XREF:S07] |
| Routing | `routing.decided` | `{cause enum, score, signals, routed worker/model/lane, effort mode, plain_reason}` — one row per routed call; plain-language reason is mandatory, not prose-optional | S2.6/7.7 [R12 §4.1] |
| Knowledge injection | `knowledge.injected` | the trace-manifest entries `{item_id, source_path, content_hash, version, selector_rule, precedence_label}`, incl. post-compaction re-injection | S4.6/S2.9; [S05.4]; [XREF:S09] |
| Orchestration | `helper.spawned` · `spawn.refused` | spawn-record ref (trigger, reason, depth, budgets, brief hash [S04.4]); refusals name the failed check | D6; [S04.4/S04.5] |
| Watchdog | `watchdog.flagged/annotated/suppressed` | rule id, trigger evidence (signature, counts, spend vs baseline), Tier-1 annotation ref; suppressions are tuning signals (S14.4) | S2.7 |
| Drift & canary | `drift.finding` · `canary.result` | source, lane(s), change class (price/terms/limits/models/endpoints/billing-regime), severity, one-line summary, incident fingerprint; canary kind + lane + pass/fail/delta | S2.8 |
| Platform | `settings.changed` [G3 Def.2] · `auth.event` · `compaction.anomaly` · `retention.compacted` | settings: `{actor, key, old, new, reason}` (mirrors `settings_events` [S01.10]); auth: kind/user/device [S01.9]; compaction anomaly: `{stage, lane, engine version, window fill at trigger, pinned sections at risk, summary artifact ref}` [S05.7]; compaction pass logs itself | 13.4; S2.1 |
| Tools & artifacts | `tool.called/completed` · `artifact.produced` | tool name, args digest, duration, artifact refs; artifact ref + hash | S2.1 |
| Benchmark & eval | `benchmark.pair_recorded` · `eval.score_recorded` | the BENCH-REG §14 record schema verbatim; suite id + version, asset id + version, metrics, runner + runner version | S2.11; S14.7/S14.8 |
| Run summary | `run.summary_written` | summary artifact ref + generation inputs digest | 11.1; S14.9 |

**Contract rules.** (1) *Completeness:* an S2 capability that cannot be answered from these families is a contract defect — fix the contract, never bolt on a side store. (2) *Evolution:* new types/fields enter only by dated spec amendment (S00.9); every type carries `schema_version`, upcast-on-read [S02.1]. (3) *Consumers parse forward-tolerantly* — unknown types are logged and skipped, never fatal (the same discipline S03.1 imposes on engine streams).

### S14.3 Live inspection (S2.2, S3.9)

Semantics live here; the front chain, HTTP/2 termination, and `/events` unbuffering are [S01.4]; what each surface renders is [XREF:S15].

- **One SSE endpoint** on the control plane [S01.2]; one multiplexed stream per client, topic-tagged (run detail, board, fleet/meters, inbox). SSE `id:` = `event_seq`. Resume honors **both** `Last-Event-ID` and explicit `?after_seq=` — resume is one indexed SELECT over `run_events`, the entire cloud "resumable streaming" cost class deleted by the durable log [R12 §2.3, §4.3].
- **Reconnect = snapshot-then-tail:** current state projected from the DB, then tail events after the cursor (the AG-UI StateSnapshot→Delta pattern) [R12 §4.3]. Keepalive comments every `⚙ obs.sse_keepalive` (within the researched 15–30 s band).
- **Polling fallback is sanctioned:** clients MAY poll the same cursor (`GET …?after_seq=`) — transport is a client detail, never an architecture fork [R12 §3-B]. No engine SSE replay is ever relied on (opencode ignores `Last-Event-ID`, fix declined upstream) — a standing conformance assertion (S14.5) [R12 §7.9].
- **Progress semantics — never percent-complete** (no shipped product does it, for good reason): cards show FSM state incl. first-class *waiting-on-human* and *parked-until*, current stage, current tool + args digest, monotonic counters (tokens, API-equivalent cost so far, elapsed, steps), and the last-activity line; the derived `wedged` state [S02.3] surfaces here too [R12 §2.3, §4.3].

### S14.4 The watchdog suite (S2.7, 4.6, 3.7)

Three tiers; runs entirely on platform code + the local tier — **costs no allowance and keeps working when every paid window is empty** (Operating reality). Local seats ride duty aliases with a CPU floor, so the watchdog is never dark [XREF:S12].

**Tier 0 — deterministic counters over the event log, always-on, zero-cost.** Thresholds ship as ⚙ per G2 Def.10, seeded from shipped prior art [R12 §2.4, §4.4]:
- **Loop:** identical tool-call signature (hash of tool name + serialized args) `⚙ watchdog.loop_repeat = 5`× consecutive.
- **Ping-pong:** two-pair alternation `⚙ watchdog.pingpong_cycles = 6` cycles.
- **Error-loop:** same action → error `⚙ watchdog.error_loop = 3`×.
- **Silence:** no event past the run-type's budget `⚙ watchdog.silence_budget.<run_type>` — seeded from `recovery.dead_after` (5 min [G2 Def.2]) until per-type calibration; TBD-BRINGUP(per-run-type silence budgets from observed event cadence).
- **Spend:** per-run ceilings are enforced at [S02.2]/[XREF:S10]; the watchdog adds daily per-person total > `⚙ watchdog.spend_median_mult = 3`× the trailing-14-day median, **armed only after `⚙ watchdog.spend_arm_days = 14` days of history** — below that, ceilings only (every learned-baseline product documents a cold-start floor; fixed thresholds are methodologically correct at household cadence, not a compromise) [R12 §2.4].
- **Suspicious completion:** run ended with anomalously low tool/verification activity for its class — the silent-failure class [R12 §4.4].

**Tier 1 — local-model disambiguator [XREF:S12].** Invoked **only** on Tier-0 triggers, never per-event: last-N-turns + original request → grammar-constrained verdict `{loop | productive | unclear}` + confidence + one-line evidence, with retrieval over the run's own recent events. **It ANNOTATES the alert — it never gates, never kills** [R12 §4.4; G1 Def.9 discipline].

**Tier 2 — pause-and-flag, always [G1 D1.3].** Contain (stop admitting the next paid call) → park at checkpoint → card. Every card carries the trigger evidence, the Tier-1 annotation, **"resume — I was wrong"** as a first-class action, and per-rule **suppress** whose usage is logged: a rule suppressed `⚙ watchdog.suppress_retune_count = 2`× proposes its own threshold raise as an admin card [G2 Def.10]. Auto-kill does not exist anywhere in this suite — the Gemini-CLI/OpenHands false-positive record is the ratified evidence [R12 §2.4; G1 D1.3].

**Alert discipline (one reader, bus factor 1).** Two severities: **flag-now** (stall/loop/spend/auth-canary on active runs; foreign listener violation [S01.6]) and **daily digest** (everything else); dedup one alert per (run, anomaly class) with updates folded in; target `⚙ watchdog.flag_now_target = 2`/day — sustained breach is itself a digest-tier meta-alert ("your watchdog is too chatty") [G2 Def.10; R12 §4.4]. These severities are alert-routing classes, distinct from the approval inbox's Low/Medium/High risk tiers [XREF:S15]. The watchdog's own liveness rides the dead-man canary, daily [G1 Def.3 superseded set].

**Registered platform-health checks** (recurring watchdog entries declared by siblings, scheduled here):
1. **Resume-reconcile check** — confirms every S01.7 wake step completed (flush, network-identity reconcile, listener re-audit) [S01.7; P-T13-1 detection side].
2. **Listener-binding recurring audit** — no sinet process listens beyond loopback; explicit operator-visible allowlist; foreign violation = flag-now [S01.6/S01.8; P-T13-2].
3. **Organ-absence degraded state** — adopted organs down (watchlist executor, local-model units, port-pool) surface as degraded-mode flags [S01.6].
4. **Event-log size watch** — WAL size + table growth meters feed Tier-0 (P-T07-5 [S02.1]).

Post-v0 evolution (parked): CodeAD-shaped offline rule synthesis — repeated Tier-1 annotations or operator suppressions draft new deterministic rules as proposal cards; detection improves while the free-tier property is preserved by construction [R12 §4.4].

### S14.5 The conformance registry

Every "proven, not assumed" obligation in this spec is a **conformance registry** row; this section owns the registry and its scheduling — the suites' *content* stays with the declaring section. Results land as `eval.score_recorded` events in Sinet's DB (P-T11-4); any red raises a card (flag-now when lane- or storage-affecting). Suites **assert behavior, never docs** [S03.3].

| Registry entry | Declared by | Triggers / schedule |
|---|---|---|
| kill-9 crash harness (application invariants post-crash) + suspend-cycle test (fake `PrepareForSleep` + clock deltas) | [S02.9] | before any storage-touching component bump; quarterly sweep |
| Adapter per-lane conformance suites — D3 verb behavior, stream schema *and values* on fixtures, park/resume mechanics, **engine-lowering leak tests** (attempt-the-leak per config channel; assert the decoy did NOT take effect), native-spawn-disabled probes, no-reliance-on-engine-SSE-replay | [S03.1/S03.5]; [R12 §4.5(b), §7.9] | on any engine/CLI version change + weekly |
| D6 violation attempts: depth-3 recursion, lateral send, engine-native spawn per lane, over-budget spawn, mid-run helper kill — refusal asserted for the first four, containment-with-salvage for the fifth | [S04.5] | quarterly + on engine bump |
| Compaction canary: canary-constraint → compact → adherence check (pinned-survival is tested behavior, P-T04-1); compaction behavior re-measured per engine pin | [S05.7] | on engine bump + quarterly |
| Forced-escalation end-to-end tests (5.6: a finding that dies in a log is a platform defect): intake coverage-check card [S06.7], plan self-attack spec-is-bad [S06.8], helper ESCALATED path [S04.5], verification escalation [XREF:S07] | [S06]/[S04]/[XREF:S07] | quarterly, with the full escalation drill [G1 Def.3 superseded set] |
| Verified-restore drill (11.3) — registered here for visibility; pipeline and drill content owned by [XREF:S13] | [S02.9]/[XREF:S13] | per S13's schedule |

**Bump gating tie [S03.3]:** an engine bump lands only after (a) the candidate passes its per-lane conformance suite **and** (b) the before/after quality probe (S14.8) shows no regression — the Apr-2026 silent-drift class is invisible to schema tests and uptime checks (P-T02-5). A landed bump is a mass worker-revalidation trigger (P-T14-1) executed by the S14.8 runbook [XREF:S08].

### S14.6 The watchlist executor & the canary layer (S2.8)

Detect outside-world change **before failures or costs spread** — formats, limits, billing status, adapter behavior (D3). Executor posture per [G2 Def.12]; sources and cadence are the settled report-02 §6 list, held as config rows `{url|feed, type, parser-hint, lane|candidate}`.

- **T1 — page diffs: pinned changedetection.io** (Apache-2.0; own unit [S01.2]). Watch list managed via its REST API from Sinet's config rows (config-as-code); region-filtered verbatim diffs; Playwright fetcher for JS-walled targets; its **native LLM rules pointed at Sinet's local OpenAI-compatible endpoint** as first-pass "does this diff matter?" triage; hits POST via `json://` webhook to the control plane [R12 §4.5].
- **T2/T4 — feeds: Sinet-native feed poller** on the ratified scheduler [XREF:S10] — conditional GET, content-hash dedup, ~15 feeds (GitHub releases/issues Atom, models.dev commits, hnrss.org — never self-hosted hnrss (no license file) — authenticated Reddit feed URLs). **Miniflux is the pre-registered fallback** if feed handling proves gnarly; its HMAC-signed webhook shape is the drop-in contract [G2 Def.12; R12 §4.5]. **T3 — aggregator APIs:** weekly scheduler jobs (models.dev `api.json` diff, price/model-list sources); the genai-prices `data.json` refresh runs here and always lands as a *proposal* — price-table drift then fires the G1 Def.5 freshness trigger [G2 Def.16; XREF:S10].
- **Second-pass classification (local tier):** every hit — page diff, feed entry, API diff — is classified `{relevant lane(s), change class, severity, one-line summary}`, grammar-constrained; relevant hits become **drift cards** in the approval inbox with fingerprint-per-incident-window dedup (one card per storm); price hits carry proposed price-table rows with effective dates (P-T08-3 [XREF:S10]); **billing-regime changes never auto-flip flags** — operator confirms, then the rehearsed 3.10 flip runs [XREF:S10; R12 §4.5].
- **The API canary layer (Sinet-built — the one genuinely novel piece):** per lane —
  - **Auth canary, distinct from limit handling (P-T17-1):** scheduled cheap real request at `⚙ canary.auth_interval` (default daily [G1 Def.3 set]), error-class discrimination via the five-class classifier [XREF:S10]; an auth-shaped failure classifies *policy-revocation-suspected* → lane freeze + flag-now, **never** an infinite retry-park [S03.6]. Canary consumption is metered like everything (D4) and itemized on the lane owner's meters — negligible by construction [XREF:S10].
  - **Conformance canaries:** the S14.5 adapter suites on their weekly schedule.
  - **Behavioral canaries:** scheduled fixed-prompt mini-eval per lane (promptfoo-runnable), alert on score drop, at `⚙ canary.behavioral_interval` (default weekly, aligned with the conformance cadence `[coordinator-draft]`) — the **only** drift detection available to the subscription lanes.
  - **Logprob canary — LOCAL LANE ONLY:** one-token logprob drift tracking at ~zero cost. Z.AI exposes no logprobs endpoint-wide and the wrapped Anthropic CLI exposes none either → both subscription lanes are behavioral-eval-only [S03.7; SPIKE P2-S1 Probe 8; R12 §4.5].
  - **Model-list drift:** observed-vs-config diff per account (P-T17-3) rides the same schedule [S03.6]. **Engine memory features and compaction behavior join the canary set** [G2 §Follow-ups; S05.7].
- **Decay posture (P-T11-3):** per-source fetcher escalation ladder (plain → Playwright → flagged-decayed card proposing an alternative source); a meta-watch flags fetch-failure streaks ≥ `⚙ watchlist.fetch_fail_streak = 3` cycles — a dead watch MUST announce itself; standing bias to migrate any scrape target to a feed/API the moment one exists [R12 §4.5].

**Standing watch items registered by the gates** (rows in the same config store): fine-grained-PAT collaborator gap · Free-plan ruleset availability · diffity maturity · jj compat matrix · X-Wing age-CLI plugin status · DBOS TS/SQLite parity · MCP 2026-07-28 final (Tasks/Extensions — the S02 durable-task-handle watch) · Litestream #1083 · Python-Redlines + pandiff license/maintenance verify [G2 §Follow-ups; S02 Deferred] · awesome-harness-engineering both referents, drop if both stall [G2 Def.15].

### S14.7 The benchmark practice (11.2 / S2.11 / 15.3)

**The protocol is BENCH-REG** — the signed registration (commit `5fb7082`), not this spec. This section builds the machinery around it and cites it; **none of its registered numbers is this spec's to change** — amendments happen only via BENCH-REG §17, and silent drift between that text and the running platform is a platform defect (its own clause). What S14 owns:

- **Sampling hook:** fires at eligibility (BENCH-REG §4.2 rules; uniform-random among eligible tasks — the uniformity is frozen, the rate is ⚙ per its §1); declines are logged and the decline rate reported. Duplicate (direct-arm) runs are admitted as background work drawn from the requester's automation budget under their standing opt-in — they throttle under pressure like any background work [G2 Def.4; XREF:S10].
- **Blind-pair machinery:** both arms rendered through the one uniform presentation template (tells stripped, length never truncated); position randomized; verdict A/B/tie/both-bad **plus the mandatory arm-guess in the same form** — blindness is measured, never assumed (P-T11-1; BENCH-REG §3/§5). Reveal only after the §14 record is written to Sinet's store (keep-forever class). Card/render surfaces are [XREF:S15].
- **Epoch tracker (P-T11-2):** the direct arm's observed model identity is recorded per pair; an identity change ends the epoch; **no decision statistic ever pools across epochs** (BENCH-REG §9). Nexus bench-02 is a closed historical epoch, never pooled.
- **Decision statistic:** the Beta-Binomial posterior `G` per (domain, epoch), updated per pair, anytime-valid with optional stopping — the only decision input; fixed-n views are readouts (BENCH-REG §7/§16). **Alarm** (`1 − G` past the registered threshold): flag-now card + **expansion freeze** (14.5 hold), operator disposition logged as a decision event (BENCH-REG §12). **Gate** (15.3): the four registered limbs (minimum non-tied pairs, `G` floor, no active alarm, regression suites green at registered floors) evaluated per launch domain over the current epoch (BENCH-REG §10/§11) — this section evaluates and displays; it sets nothing.
- **Small-n honesty as a product surface (P-T11-5):** wherever a win rate renders, the honest-claims table renders with it (BENCH-REG §15), and every published rate carries n, `G`, tie/both-bad rates, decline rate, and guess accuracy. There is no benchmark number without its epistemics.
- **Done-directly feed [XREF:S10]:** receipts carry the per-run heuristic line from day one; once a domain accrues the registered minimum of measured pairs, S14 computes and publishes the **measured median direct-arm consumption per domain** — the aggregate honesty figure S10's dashboards consume, labels verbatim per BENCH-REG §13.
- **Standing benchmark/eval questions register** (questions the practice must eventually answer, recorded so they cannot evaporate): (a) **R13-OQ6** — do anchored `[F#]` findings beat file-level notes on retry quality? [G2 §Follow-ups]; (b) **P-T05-2** — delta-card rubber-stamp measurement: S06.9's hook records presented-delta size, time-to-decision, decision, outcome linkage; the analysis runs here, and measured rubber-stamping proposes a card-format retune [S06.9]; (c) **P-T05-4** — pre-registered stakes-classifier eval before v1 household use [S06]; (d) intake approval-UX changes are justified only by measured outcomes [S06.9].

### S14.8 Regression evals & the revalidation runbook (7.3, 5.7)

- **Runner: pinned promptfoo** (MIT; pin in `components.lock` [XREF:S16]); **DeepEval is the pre-registered swap** if post-acquisition governance drifts. Tools are runners, never stores: every score/verdict lands in Sinet's DB as `eval.score_recorded` (P-T11-4) [R12 §4.7]. Local models run via the local lane [XREF:S12].
- **Per-asset eval objects:** every worker template and every rubric carries an eval set — golden cases (25–50, grown from real outcomes) + a **planted-defect suite** the judge must fail (code: test-adequacy defects + known-bad artifacts; research at v0.1: planted unsupported citations) [R12 §4.7]. Floors register per rubric/eval-set version at 8.3 knowledge-gate entry [BENCH-REG §10.1(d); XREF:S09].
- **Revalidation runbook** (trigger: a watchlist drift finding, an engine bump, or a model/duty-alias swap): flag dependent workers/judges/rubrics (7.3) → run affected golden sets + planted-defect suites → compare TPR/TNR and task metrics against the pinned baseline → green: release with a dated revalidation stamp; red: asset stays flagged, card to its owner. **Judge-model changes block unsupervised judging until golden-set re-measurement passes** (P-T06-5; judge logic [XREF:S07]). **Every local-tier seat swap mandates threshold recalibration** — model churn invalidates calibration [G3 §Follow-ups; XREF:S12]. Cadence: on-trigger + `⚙ eval.sweep_interval = quarterly` full sweep [R12 §4.7].
- **Engine-bump quality probes [S03.3] run here:** the before/after small-task battery that gates a bump (S14.5) — schema tests catch schema drift; only quality probes catch effort/context-handling drift (P-T02-5).
- **Rubric falsifiability (5.7):** a rubric "works" iff its planted-defect suite catches at least its registered floor, measured per rubric version; the benchmark practice is the outer falsification loop — does rubric-passing work actually win blind pairs? [R12 §4.7].

### S14.9 Retention & compaction (11.1) [G2 Def.11]

- **At run end** (never later): the **run summary** — a compact structured story (objective, stages, tool-call counts, verdicts, decisions, receipts, final state) from deterministic aggregation + the local tier [XREF:S12]. Incremental-per-run is the documented safe shape; bulk summarization over months of trace is the documented failure shape [R12 §4.2].
- **At `⚙ retention.compaction_horizon = 6 months`** (per-user, 13.4): the scheduled compaction pass strips bulky event payloads and transcript copy-asides. **Keep-forever set:** run summaries, verdicts, decisions, receipts, routing records, drift events, benchmark records [G2 Def.11]. The pass logs itself (`retention.compacted`) — the audit trail records its own compaction.
- **11.3 boundary by construction:** the snapshot exporter [XREF:S13] reads an allowlisted view containing only the keep-forever set; raw trace payloads are structurally unreachable from it. The event-log growth watch (P-T07-5) feeds S14.4.

### S14.10 Queryable history (S2.10)

Everything above is filterable and searchable — by project, person, status, date range, worker, domain, lane, run/task id — through three layers; the conversational assistant consumes these layers and nothing else [XREF:S15]:

- **Layer 0 — deterministic views (no model):** every S2.5 cost question (per run/task/project/person/period, budget remainders, burn rate, limit-event history, done-directly figures) is a named SQL view over the ledger/receipts (meter semantics [XREF:S10]). The assistant *selects* views — **it never computes money by generation**.
- **Layer 1 — canned parameterized catalog:** ~30–50 named queries (status, failures, verdicts, routing quality, drift history) with typed slots; the local model classifies intent and fills slots, grammar-constrained [XREF:S12]; answers render with the query named. This is the reliability floor.
- **Layer 2 — open SQL, enabled at v0 [G3 D3.5]:** Arctic-Text2SQL-R1-7B generates against **allowlisted views only**, under the full guardrail stack — read-only connection (`query_only`), single-statement parse rejecting DDL/DML, LIMIT + timeout injection, every generated query audit-logged — and every answer is **flagged lower-confidence** in the UI. Seat and hardware path are [XREF:S12]; TBD-BRINGUP(Layer-2 open-SQL acceptance measurement, G3 Def.8 battery). Canned queries remain the floor; Layer 2 is escalation, never default [R12 §4.8].
- **Indexing (DDL detail is P3's [S02.2]):** generated columns over event-JSON hot fields; a `dotted_order`-style generated column for ordered-tree queries; FTS5 over run summaries, verdict texts, and drift summaries; rollup tables maintained by scheduler jobs. DuckDB read-only ATTACH is the documented later analytics sidecar — not v0 [R12 §4.8].
- **Routing-quality view (S2.6 analyzable-in-aggregate):** a periodic view joins `routing.decided` causes to outcomes (rework rates, verdicts, receipts) so routing itself is auditable over time [R12 §4.1].

---

**Settings introduced (⚙):** (all operator-editable with audit trail per G1 rider 1; auto-adjust only within operator ceilings)

| ⚙ setting | default | clamp / range | ratified by |
|---|---|---|---|
| `obs.sse_keepalive` | 20 s | 15–30 s | R12 §4.3 |
| `watchdog.loop_repeat` | 5 | 3–10 | G2 Def.10 |
| `watchdog.pingpong_cycles` | 6 | 3–12 | G2 Def.10 |
| `watchdog.error_loop` | 3 | 2–10 | G2 Def.10 |
| `watchdog.silence_budget.<run_type>` | = `recovery.dead_after` (5 min) seed | ≥ 1 min; per-type | G2 Def.10; TBD-BRINGUP |
| `watchdog.spend_median_mult` | 3× | ≥ 1.5× | G2 Def.10 |
| `watchdog.spend_arm_days` | 14 d | ≥ 7 d | G2 Def.10 |
| `watchdog.flag_now_target` | 2/day | 1–10 | G2 Def.10 |
| `watchdog.suppress_retune_count` | 2 | 1–5 | G2 Def.10 |
| `watchlist.fetch_fail_streak` | 3 cycles | 2–10 | R12 §4.5 / P-T11-3 |
| `canary.auth_interval` | daily | 4/day – weekly | G1 Def.3 set; P-T17-1 |
| `canary.behavioral_interval` | weekly | daily – monthly | R12 §4.5 `[coordinator-draft]` |
| `retention.compaction_horizon` | 6 months | ≥ 1 month; per-user | G2 Def.11; 13.4 |
| `eval.sweep_interval` | quarterly | monthly – semi-annual | R12 §4.7 |
| `benchmark.sampling_rate` | BENCH-REG §4.1 schedule (100% pre-gate / 25% maintenance) | 0–100% | BENCH-REG §4.1 (rate ⚙; uniformity frozen) |

**Registered-number rule:** the benchmark's frozen values (gate limbs, alarm threshold, done-directly minimum, labels) surface in the settings registry **marked "registered — changing this value requires a re-registration commit"**, with the audit trail linking to the registration hash [BENCH-REG §1; S01.10].

**Known problems owned here:**
- **P-T11-1** — blinding partially fails at household scale → uniform render template + measured blindness; guess accuracy reported beside every win rate (machinery S14.7; obligation BENCH-REG §5).
- **P-T11-2** — the direct arm is un-pinnable → epoch tracker, per-pair model identities, no cross-epoch pooling (S14.7; BENCH-REG §9).
- **P-T11-3** — watcher sources decay structurally → fetcher escalation ladder + fetch-failure meta-watch + migrate-to-feed bias (S14.6).
- **P-T11-4** — observability/eval tooling ownership churn → runners-never-stores, permissive licenses, pins, pre-registered swaps; all records in Sinet's DB (S14.1, S14.5, S14.8).
- **P-T11-5** — small-n honesty is a product surface → honest-claims table + full epistemics on every published rate (S14.7; BENCH-REG §15).

Executed/consumed here for owners elsewhere: P-T07-5 (size watch, S14.4), P-T02-5 (bump quality probes, S14.8), P-T17-1 (auth canary, S14.6), P-T17-3 (model-list diff schedule, S14.6), P-T04-1 (compaction canary home, S14.5), P-T05-2/P-T05-4 (analysis + pre-v1 eval, S14.7), P-T06-5 (judge-change block, S14.8), P-T14-1 (bump revalidation execution, S14.8), P-T03-1 (cache-weighting resolution watch, S14.6), P-T08-3 (price-drift proposals, S14.6), P-T13-1/P-T13-2 (recurring checks, S14.4).

**Deferred / parked:**
- OTLP projector (approach A3) → re-entry: OTel GenAI semconv reaches Stable, or a one-container SQLite OTLP UI matures [R12 §4].
- CodeAD-shaped watchdog rule synthesis → post-v0, as proposal cards [R12 §4.4].
- Miniflux feed backend → re-entry: native poller proves gnarly; webhook contract is the drop-in [G2 Def.12].
- DeepEval swap → re-entry: promptfoo governance drift manifests [R12 §4.7].
- DuckDB ATTACH analytics sidecar → re-entry: query latency becomes real [R12 §4.8].
- Layer-2 SQL on 14–32B (eGPU) → re-entry: T15 stretch bar + hardware present [XREF:S12].
- Blindness-calibration pilot (~10 mock pairs) → v0.1, before member-facing benchmark use [BENCH-REG §5, non-binding note].
- Learned anomaly baselines → only if history floors are met AND fixed thresholds measurably underperform [R12 §5].
- Low-n catastrophic-only gate variant → only via dated re-registration if accrual < ~5 pairs/month [BENCH-REG §11].

**Coverage:** (Scope → subsection)
| feature-list item | subsection |
|---|---|
| 11.1 full audit trail + retention | S14.1, S14.2, S14.9 |
| 11.2 / S2.11 / 15.3 benchmark practice + gate | S14.7 (protocol = BENCH-REG) |
| S2.1 full traceability | S14.1, S14.2 |
| S2.2 / S3.9 live inspection | S14.3 |
| S2.3 verdict records per round | S14.2 (`verdict.recorded`); logic [XREF:S07] |
| S2.4 human decisions recorded | S14.2 (`decision.recorded`) |
| S2.5 cost observability at every altitude | S14.10 Layer 0; meters [XREF:S10] |
| S2.6 / 7.7 routing explainability | S14.2 (`routing.decided`), S14.10 |
| S2.7 / 4.6 / 3.7 self-health watching | S14.4 |
| S2.8 outside-world drift detection | S14.6 |
| S2.9 auditable learning (trace side) | S14.2 (`knowledge.injected`); lifecycle [XREF:S09] |
| S2.10 queryable history | S14.10 |
| 5.6 escalation paths proven by tests | S14.5 |
| 5.7 rubric falsifiability | S14.8 |
| 7.3 model-change revalidation | S14.8 |

**Open items for G4:** none. (Two flagged drafting sub-choices ride their markers: the behavioral-canary cadence `[coordinator-draft]` and the silence-budget seeds TBD-BRINGUP — both operator-tunable settings, not open decisions.)

## S15 — Frontend & API surface

**Scope:** The web workspace (all S3 surfaces plus PWA/push), the four binding frontend component picks, and the HTTP-API/SSE consumption contract under which every surface — SPA, chat, any future CLI or ingress channel — is an equal client of the control plane.
**Binding inputs:** G3 D3.3 + discipline riders · `Spec/frontend-components-v1.md` (cited **FC-v1**) · R17 §2.4–2.6, §4.3, §4.6, §4.10 · G2 D2.2 (Layer C patterns), D2.8 (receipt labels), Def.2, Def.13, Def.14 · G1 Def.3 (richer close set) · G3 D3.5, Def.1, Def.6 · feature list §6, §9, §13, S1–S3 · siblings: S01 (front chain, auth, settings registry), S13 (deliverable/comment contracts), S14 (event semantics, query layers), S10 (meter/receipt data), S06 (intake, stale-replan threshold).

### S15.1 Architecture: a disciplined React SPA

The frontend is a **plain React 19 + Vite SPA in TypeScript — no meta-framework, no RSC, no SSR layer — installed as a PWA** [G3 D3.3; R17 §4.3]. The RSC/SSR exclusion is load-bearing, not stylistic: the Dec-2025 critical-RCE class lived in RSC, which a plain SPA does not ship, and a tailnet-only app has no SEO or first-paint case for server rendering [R17 §2.4, §4.3]. The built assets are embedded in the control-plane binary and served through the S01.4 front chain; the web surface is not a process [XREF:S01].

**Discipline riders (bind)** [G3 D3.3; R17 §4.3]:

- Lockfile-pinned minimal dependency tree; every frontend component carries a `components.lock` entry — pin, license, funeral plan [XREF:S16].
- Scheduled dependency pass every ⚙ `frontend.dependency_pass_interval` = 90 d (quarterly); SHOULD be co-scheduled with S16's manifest review [XREF:S16].
- Vite majors on a lag: adopted only at a scheduled pass, never day-one.
- The Nexus anti-pattern (11.7k-line untyped hand-rolled SPA) is structurally excluded: typed framework + adopted organ-grade components only [R17 §4.3].

**HTMX re-entry — the named conditions [R17 §4.10], the only sanctioned path back:** (a) the S15.4 picks prove replaceable by framework-agnostic widgets at acceptable cost → HTMX 2.x re-opens as the architecture candidate (it holds the field's strongest written longevity commitment); (b) Datastar earns a re-look only at the first major frontend overhaul, and only if its 1.x line has proven stable through ~2027.

### S15.2 The API surface: one seam, equal clients

Every surface is a client of the same HTTP API and the one SSE endpoint — the SPA, the conversational assistant, any future CLI, and any 15.5 ingress channel; nothing renders from private access [XREF:S01 — the API seam]. The browser is a display, never an authority: authorization and visibility are enforced server-side through the S01.9 owner-scoped accessors and session identity [XREF:S01]; client-side filtering is presentation only.

**REST resource families (contract level; full endpoint schemas are P3 work within these bounds):**

| Family | Root | Exposes | Mutating verbs (all owner-scoped) | Data owner |
|---|---|---|---|---|
| tasks | `/api/tasks` | task objects: spec + numbered AC, plan, stage progress, lineage (project, follow-ups), receipt view | create (opens intake [XREF:S06]), carrying the OPTIONAL per-task lane pin **[S00.9 A13]** — validated server-side and refused rather than dropped; follow-up spawn (S1.2); cancel (4.5) | S06/S02; receipt figures S10 |
| runs | `/api/runs` | run FSM state, live-activity refs, spawn records, routing records (S2.6) | cancel (4.5) | S02/S03/S10 |
| approvals | `/api/approvals` | inbox items — proposals, questions, sign-offs, escalations — with risk tier, expiry, and 13.5 help fields | answer (approve / deny / answer / re-plan); Low-tier batch answer | S02 effect journal; D7/D10 gates |
| deliverables | `/api/deliverables` | immutable numbered revisions, diffs, anchored comments, preview sessions | comment CRUD (own comments); request bounded revision; accept (6.3 → S13 flow) | S13 |
| settings | `/api/settings` | registry schema + values, price table, per-setting audit history | validated set/reset (clamped [XREF:S01]); price-table edits | S01.10; price table S10 |
| meters | `/api/meters` | consumption, pressure, budgets, burn rates, limit-event status per (person, lane, period) | budget edits (own); pause-my-automation switch (3.3) | S10 |
| memory [coordinator-draft] | `/api/memory` | scoped memory/knowledge entries (person / project / house) with provenance and gate status | manual entry create/edit — S09's write gate applies (own-store writes tier Medium; house promotion operator-only, D10) | S09 |
| events | `/events` (SSE) + `/api/events` (history) | the live stream; filterable history (S2.10) through the S14 query layers | — (append is control-plane-internal) | S14 |
| chat | `/api/chat` | per-user assistant sessions, their turns, and the exchange files a conversation carries (S15.7); sessions are platform-owned, and work is handed to intake (S06) rather than created here | session create / rename / delete; turn submit / stop; file upload / delete | S15.7; handoff S06; file confinement S11 |
| push | `/api/push` | Declarative Web Push subscriptions, one per (identity, device) (S15.11); an endpoint is a capability URL and is never served back | subscribe; remove | S15.11 (register row 2 [XREF:S01 — S01.8]) |

Login/session endpoints are S01.9's and are not restated here [XREF:S01]. Contract rules binding every family:

- **No direct outward-effect verb exists anywhere on the API.** Anything non-idempotent and outward enters as a proposal and exits only through `approvals` (D7) [XREF:S02].
- **Writes are retry-safe.** Approval answers and accepts are pinned to the payload hash they were shown for: a stale-payload answer is rejected, a repeated answer returns the already-resolved state — a phone retry can never double-fire [G2 D2.1; XREF:S02].
- **Every mutation lands on the event log** and reaches clients over SSE; the REST response confirms, the event feed is the truth surfaces render from [XREF:S14].
- **Versioning is lockstep and unversioned at v0** [coordinator-draft]: SPA and API ship in one binary (S01.5), so no `/v1` prefix exists; evolution is additive-first, and a future non-bundled client pins to platform releases.

### S15.3 Live data: SSE consumption

Client-side consumption discipline for the one SSE endpoint (event semantics, event types, and the query layers are S14's [XREF:S14]):

- **Snapshot-then-tail:** a view loads its REST snapshot, then tails `/events` from the snapshot's `event_seq` cursor; disconnects resume via `Last-Event-ID` / `?after_seq` [G2 D2.1 (R12 §4.3)].
- A tab SHOULD hold one EventSource and fan events out in-app; multi-tab rides HTTP/2 terminated at the front door, so the 6-connection limit never binds [R17 §4.6; XREF:S01].
- On cursor gap or eviction the client re-snapshots — it never patches blind. The polling fallback is S14's transport concern [XREF:S14].
- Connection state is always user-visible (S15.12): a disconnected or catching-up surface says so rather than posing as live.

**AG-UI is vocabulary, not runtime** [G2 D2.2 Layer C]: the event-type names in S14's contract mirror AG-UI's vocabulary pattern (run lifecycle, streaming deltas, tool events, state sync) so chat, board, and activity feeds consume one recognizable dialect; no AG-UI runtime, SDK, or wire protocol is adopted, ever [FC-v1].

### S15.4 Binding component picks

The four picks are **binding** [FC-v1; G3 D3.3]. Versions are as recorded at binding (2026-07-18); exact pins live in `components.lock` [XREF:S16]. Adopt-don't-fork applies: the carried Nexus behaviors below are specs *over* these widgets, never modifications *of* them.

| Surface | Pick (binding) | Version at binding | License | Pre-registered fallback |
|---|---|---|---|---|
| Board drag-drop (S1.3) | **@hello-pangea/dnd** | 18.0.1 (peers `^18\|\|^19`) | Apache-2.0 | pinned @dnd-kit/core 6.3.1 (frozen-but-working) |
| Diff review (S3.5) | **react-diff-view** + gitdiff-parser, rendering the ported N15 behavior spec | 3.3.3 | MIT | monaco diff via @monaco-editor/react 4.7.0 — pre-registered upgrade path |
| Chat (S3.6) | **@assistant-ui/react** via LocalRuntime/ExternalStoreRuntime — assistant-cloud is never used | 0.14.27, pinned | MIT | owned chat primitives on the same SSE cursor |
| Settings forms (13.4/13.5) | **JSON Forms** (@jsonforms/core + react) with a Sinet-owned custom renderer set | 3.8.0 (react peer admits `^19`) | MIT | RJSF v6 (documented runner-up) |

Pick-level riders [FC-v1 §1–4]:

- The @dnd-kit 0.x rewrite family is excluded outright; re-evaluate only at its 1.0 with a dated note.
- react-diff-view's single-maintainer risk is bounded by the pin, MIT forkability, and containment to one view; syntax highlighting is client-side via its tokenizer — no server-side highlighter exists in the platform.
- @assistant-ui/react is 0.x: the pinned lockfile + quarterly pass absorb its churn; the ExternalStoreRuntime boundary keeps message/session state platform-owned, so a widget swap loses no data.
- JSON Forms renderers are Sinet-owned (no theme-package dependency); RJSF v6 re-enters only if JSON Forms stalls >18 months without a maintenance release or a registry need exceeds its rule/layout vocabulary.

### S15.5 Oversight surfaces: mission control, board, task detail, fleet, filters

**Mission control (S3.1)** is the home surface: one live screen of everything running, queued, parked (with "parked until…" times), blocked-on-a-human, and recently finished; consumption and automation-budget meters with burn rates per person and model [data XREF:S10]; who owns what; filterable history by project, person, status, and date [query layers XREF:S14]. Any item drills into its task detail and full trace.

**Live board (9.1, S1.3).** Tasks are cards moving through their stages in real time from the SSE feed; cards group by project with follow-up lineage visible (S1.1–S1.2 semantics [XREF:S06]). A card face shows exactly the S1.3 set: what it is, whose it is, current stage, effort mode — with any disclosed downgrade note (3.5) [G2 D2.1; XREF:S10] — cost so far, and waiting-on-human. Drag-drop uses @hello-pangea/dnd; **stage columns are never writable by drag** — stage is FSM state owned by the control plane [XREF:S02]. The sanctioned v0 drag interaction is reordering one's *own queued* tasks as a scheduler priority hint [coordinator-draft; XREF:S10].

**Task detail (9.2, S1.7)** shows the confirmed specification with its numbered acceptance criteria, the approved plan, per-stage progress with the live activity feed (S2.2), every human decision along the way (S2.4), every deliverable revision [XREF:S13], and the receipt: ceremony-vs-execution itemization and the done-directly figure under its ratified labels — "direct-use estimate (heuristic)" / "measured (benchmark n=…)" [G2 D2.8; data XREF:S10].

**Fleet overview (9.3)** answers what is running on whose account at what burn rate: per-person/per-model consumption meters and automation-budget remainders (S2.5), limit-event status and history ("parked until…"), all filterable [XREF:S10; XREF:S14]. Accounts are always distinguished (S3.10; D2).

**Personal filters (S1.4)** are first-class saved views, identity-scoped: **what-needs-me** (approvals, questions, review-ready deliverables), **mine**, **running**, **finished-today**. All four are phone-complete (S1.10).

### S15.6 Approval inbox

The single queue where "the platform needs you" lives (S3.2); the deep-link target of every push (S15.11). Cards arrive ranked by risk — proposals, questions (incl. the S14 blind-pair verdict forms with their mandatory arm-guess field [XREF:S14; coordinator-draft]), sign-offs, escalations — and carry the 13.5 help fields — what this decision does, what could go wrong, what the platform recommends and why; for settings-class approvals that text is registry-sourced [XREF:S01].

**Risk tiers and batching (ratified rules, S3.2):**

- **Low** — read-only or reversible-inside-the-workspace → batchable: one action answers a selected set; each item stays individually listed and individually logged.
- **Medium** — writes to the person's own stores → owner approves individually; never batched.
- **High** — outward or irreversible (send, publish, push to shared, metered spend, permission changes) → **never batchable**; owner approves; **operator co-approves when platform-level** (D10), both approver states visible on the card; the PIN is re-prompted — approval identity is never inherited from an idle session [G3 Def.1; XREF:S01].

**Plan cards always carry both Approve and Re-plan** (S3.2). A card pending past the S06.9 staleness threshold auto-flags "assumptions may be stale" with one-click re-plan [XREF:S06]. Cards display their expiry countdown (ask-approval expiry [G2 Def.2; XREF:S02]). **Answering resumes the paused work in place** from its last checkpoint (4.3) [XREF:S02]. Re-nags follow the ratified SLA set — approval remind 4 h, re-push 24 h; safety escalations push immediately and re-ping hourly — with cadence ⚙ and delivery owned by the notifier [G1 Def.3 (richer close set); XREF:S14]. The approval-card payload contract follows the LangGraph interrupt/approval schema as pattern only — no runtime [G2 D2.2 Layer C].

### S15.7 Conversational assistant

A first-class surface inside the dashboard and on mobile (S3.6, 9.5): ask what's running, what's blocked on you, what something consumed — or start a task by chatting, which hands the conversation to the S06 intake pipeline in place [XREF:S06]. Platform-state answers ride the S14 query layers: deterministic views → canned queries (intent-filling is a local duty alias [XREF:S12]) → Layer-2 open SQL, whose answers MUST visibly carry their lower-confidence flag [G3 D3.5; XREF:S14].

The widget is @assistant-ui/react (S15.4) on LocalRuntime/ExternalStoreRuntime with a Sinet adapter speaking to the platform API; message and session state are platform-owned. **Carried Jarvis-tab behavior specs (bind) [FC-v1 §3]:**

- **Stream-survives-navigation inventory:** an explicit inventory of what a view switch may shed versus what MUST survive — the in-flight turn, its stream, and its produced-files accumulation survive navigation; hard-stop is a separate explicit user action, never a navigation side effect.
- **Error humanization:** the SSE alias/error-humanization table — provider/transport error classes mapped to plain language; empty-stream vs transport-error distinguished; 429/overload humanized; model fallback announced in-feed.
- **File exchange (v0 carry — explicitly not parked):** drag-drop + click-browse upload sidebar; uploaded-file list with delete; post-turn **produced-files chips** computed as an exchange-folder diff around the turn. Exchanged files are owner-attributed artifacts (15.6), fail-closed on ownership; files handed to a launched task enter as task inputs through intake [XREF:S06]; confinement rules govern anything a run touches [XREF:S11].
- **Sessions:** server-side per-user session registry, fail-closed; **first-message auto-titling** runs as a local-tier duty [Operating reality; XREF:S12].

### S15.8 Review surfaces (per deliverable type)

Everything arrives as a reviewable change (6.1). The behavior contract — the anchor schema (`file_path + side + line_no + line_text`), server-side re-validation with ±2-line drift tolerance, degrade-to-file-anchor and the explicit orphan state, the comment lifecycle `open → consumed` with batch `consumed_at`, the single findings→retry drain point — is S13's [XREF:S13; FC-v1 §2]. S15 renders it:

- **Code/text:** react-diff-view side-by-side and inline views over unified git diffs, gutter/widget hooks carrying anchored comments at exact places (6.2); client-side highlighting. **Round-over-round diff is the default rework view**, with revision-over-revision navigation across rounds (S3.5).
- **Unanchorable findings** render on the **synthetic render surface** — a finding is never invisible [FC-v1 §2].
- **Images:** visual compare (6.1). **PDFs:** extracted-text diff at v0 [G2 Def.13]. **Binaries:** hash refs + download from the local object dir [G2 Def.14].
- **Try-it (S1.9, 6.4):** one-click preview renders as the dual-iframe before/after surface; ports, disposable environments, and preview lifecycle are S13's stack [XREF:S13].
- **Accept (6.3, S1.8)** is one action on the reviewed revision; the attributed-commit flow is S13's [XREF:S13].

monaco diff is the pre-registered upgrade if editor-grade needs emerge [FC-v1 §2].

### S15.9 Settings UI

Generated, never hand-built: JSON Forms consumes the registry-emitted JSON Schema + UISchema (S01.10) with Group/Categorization tabs and rule-based SHOW/HIDE visibility — core, theme-independent vocabulary rendered by the Sinet-owned renderer set [FC-v1 §4; XREF:S01]. **Every setting** is present and editable here (13.4): the per-user flat-rate/metered flags, the API-equivalent price table [XREF:S10], automation budgets, freshness thresholds, depth cap, retention, missed-slot defaults, and every ⚙ in this spec [index XREF:S18]. The UI shows each setting's clamp bounds and enforces the split — automation moves *value* only within bounds, only the operator edits bounds; restart-required settings are badged; per-setting audit history renders from `settings_events` [XREF:S01]. 13.5 help text renders inline from the registry help field.

### S15.10 Workforce map (view-only at v0)

A readable graphical view of the machinery (9.4, S3.3): every worker, what each is equipped with (tools, knowledge, permissions, helpers), and how multi-stage procedures connect, rendered from the S08 worker registry [XREF:S08]. The map also surfaces S08's per-version quality/cost views (the version→outcome joins) alongside each worker [XREF:S08; coordinator-draft]. Its purpose is audit and understanding; identity rules apply (personal workers to their owner, the shared roster to all). **Editing through the map is parked to 15.5** — the v0 surface has no mutation affordances. Rendering uses owned components at v0; a dedicated graph-layout dependency may enter only through the manifest: TBD-P3(workforce-map rendering approach — owned rendering vs manifest-admitted layout library).

### S15.11 Notifications: Declarative Web Push

Decisions reach the right person on a phone (S3.7, 9.6). The channel is **Declarative Web Push**: JSON payload, no service-worker requirement, the required `navigate` field deep-links straight to the answerable card, and `app_badge` carries the identity's pending-approvals count as the home-screen badge [R17 §2.5, §4.6]. iOS requires Home-Screen PWA install and ≥18.4 for the declarative form; Android is standard push [R17 §2.5]. Every approval card and task has a stable URL, so deep links and bookmarks always land. The loop target is normative: **notified → glance → decide < 10 s on a phone, while the host is up** (S3.7; Operating reality).

Accepted facts, tied to the observables register [XREF:S01 — S01.8; P-T13-3]: no self-hostable standard Web Push service can exist — the user agent picks its push service (RFC 8030) [R17 §2.5]; payloads are encrypted to subscription keys, so vendor relays see **timing, volume, and endpoint metadata — never content** (register row 2). The frontend MUST NOT add any further external observable without a prior register amendment [XREF:S01].

**Week-one push drill:** TBD-OPERATOR(week-one real-device drill on household phones — iOS Home-Screen install + declarative push + `navigate` deep-link; Android push; the tap-with-tailnet-down edge; acceptance = notified → glance → decide < 10 s) [G3 Def.6]. **Flip condition [R17 §4.10]:** drill failure on household iPhones → notifications move to ntfy-with-relay; Web Push stays for badges only.

### S15.12 Cross-cutting duties

- **Live by default (S3.9).** Every surface updates from the SSE feed; manual refresh is never required for currency. A surface whose feed is disconnected or catching up MUST show that state visibly — stale data never poses as live; with the host asleep or the tailnet down, the PWA renders an honest unreachable state (the push-arrives-but-link-fails edge belongs to the S15.11 drill) [Operating reality].
- **Multi-user aware (S3.10).** Identity comes from the S01.9 session; every view is owner-scoped server-side; people see their own work and what is shared with them; approvals route to the decision owner (D10); the fleet always shows whose account burns (D2, 3.4). Shared-device PIN policy and High-tier re-prompts render as specified [G3 Def.1; XREF:S01].
- **Mobile/desktop (S1.10).** One responsive workspace, not two products. Phone-complete: the inbox and every decision, personal filters, board and fleet glancing, task status, chat, push handling. Desktop-comfortable: diff review, the workforce map, settings bulk edits. Both speak the identical API (S15.2).
- **Escape-by-default.** Model- and web-derived content renders escaped everywhere; the only sanctioned raw-HTML channel is S15.8's preview surface [FC-v1 §2].

### S15.13 Parked v1+ (behind the 15.3 benchmark gate) and open sockets

Per 14.5, no extra surfaces ship before the benchmark gate passes; the operator re-affirmed valuing this set on 2026-07-18 [FC-v1 §3]:

- **Avatar hologram + memory-galaxy backdrop** — the most portable pieces of the old SPA; plan: host in a React wrapper at v1, re-verify then [FC-v1 §3].
- **Conversation mode + STT/TTS** (incl. the clause-latency progressive-speech pattern) — re-enter as **satellite tools** under the 12.4 framing, never core platform features [FC-v1 §3].
- **JARVIS file exchange is NOT in this set** — it ships at v0 (S15.7).

Sockets the architecture leaves open: **multi-channel ingress (S3.8, 10.3)** at 15.5 — a bot channel is just another API client at the S15.2 seam, subject to the same intake, attribution, and gates; **workforce-map editing** at 15.5 (S15.10).

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `frontend.dependency_pass_interval` | 90 d (quarterly) | (30 d, 365 d) [coordinator-draft] | G3 D3.3 rider; R17 §4.3 |

**Known problems owned here:**

- **P-T13-3** — accepted-external-observables: register owned at S01.8; S15 binds the frontend contribution (push metadata = register row 2) and the no-new-observables-without-amendment duty (S15.11).
- **P-T16-1** — component onboarding: owner S16; enforced for the frontend tree here — the four picks and any future frontend dependency enter `components.lock` (S15.1, S15.4).

**Deferred / parked:**

- Avatar hologram + memory galaxy → 15.3 benchmark gate passes; host-in-React-wrapper plan, re-verify at re-entry [FC-v1 §3].
- Conversation mode + STT/TTS → 15.3 gate; re-enters as 12.4 satellite tools, never core [FC-v1 §3].
- Workforce-map editing → 15.5.
- Multi-channel ingress (S3.8) → 15.5; the socket is the API seam (S15.2).
- HTMX 2.x → re-entry only via the S15.1 named conditions [R17 §4.10].
- Datastar → re-look at the first major frontend overhaul if its 1.x line proves stable through ~2027 [R17 §4.10].
- monaco diff → pre-registered upgrade on editor-grade needs [FC-v1 §2].
- @dnd-kit/react → re-evaluate at its 1.0 with a dated note [FC-v1 §1].
- RJSF v6 → re-entry if JSON Forms stalls (>18 months) or a registry need exceeds its rule/layout vocabulary [FC-v1 §4].
- ntfy-with-relay → flips in only on push-drill failure; Web Push then keeps badges only [G3 Def.6; R17 §4.10].

**Coverage:**

| Feature-list item | Where |
|---|---|
| S3.1 mission control | S15.5 |
| 9.1 / S1.3 live board (card fields, real-time movement) | S15.5 |
| 9.2 / S1.7 task detail (incl. S2.2 live inspection, S2.4 decisions, 3.6 receipt display) | S15.5 |
| 9.3 fleet overview (+ S2.5 cost-altitude display) | S15.5 |
| S1.4 personal filters | S15.5 |
| 9.6 / S3.2 approval inbox (tiers, batching, Approve+Re-plan, resume-in-place) | S15.6 |
| 13.5 approvals explain themselves (display; field source S01.10) | S15.6 |
| 9.5 / S3.6 conversational assistant (+ S2.10 questions over history) | S15.7 |
| 6.1/6.2 / S3.5 review surfaces; S1.8 native review | S15.8 |
| 6.4 / S1.9 try-it preview (UI; machinery S13) | S15.8 |
| 6.3 accept as one action (UI; flow S13) | S15.8 |
| 13.4 settings UI (every setting; registry S01.10) | S15.9 |
| 13.2/13.3 see-everything, configure-everything frontend | S15.5–S15.10 |
| 9.4 / S3.3 workforce map view-only | S15.10 |
| S3.7 / 9.6 notifications carrying the decision | S15.11 |
| S3.9 live by default | S15.3, S15.12 |
| S3.10 multi-user aware throughout | S15.12 |
| S1.10 same workspace everywhere | S15.12 |
| S3.8 multi-channel ingress (parked 15.5; socket noted) | S15.13 |
| 12.4 / 14.5 parked satellite set | S15.13 |

**Open items for G4:** none. Six drafting-time sub-choices are flagged inline as [coordinator-draft] for G4 attention — three from the original draft: the unversioned-lockstep API posture (S15.2), the sanctioned board-drag interaction — reordering one's own queued tasks only (S15.5), and the `frontend.dependency_pass_interval` clamp range (⚙ table); three interlock refinements added at coordinator review (2026-07-19), each closing a committed sibling's expectation into this section: the `/api/memory` resource family (S15.2 ↔ S09's manual-L2 workspace-UI seam), the S14 blind-pair verdict forms as inbox question cards (S15.6 ↔ S14), and the per-version quality/cost views on the workforce map (S15.10 ↔ S08).

## S16 — Adoption manifest & components.lock

**Scope:** The binding inventory of everything Sinet adopts: the `components.lock` format and its CI enforcement, the complete v0 manifest, the component-onboarding checklist every future adoption passes, the patterns-only (Layer C) list, the binding no-adopt register, the quarterly dependency pass, and the standing watch rows this section registers into S14's executor.
**Binding inputs:** G2 D2.2 (+ Def.15, Def.16; §Follow-ups watchlist adds) · G3 D3.3 riders · [R14 §2, §4.1, §5, §6.1 (the citable harvest reference), §6.2, §6.4, §6.5, §7] · [R17 §4.8] · `Spec/frontend-components-v1.md` (binding picks) · [R12 §2, §4] · [R16 §2.3, §4.1] · siblings S01.3/S01.5/S01.11, S03.2/S03.3/S03.6 · G1 P1, P2, P6, P7, rider 3 · feature list D3, 3.10, 4.7, 7.3, S2.8, 14.1/14.4/14.5.

Boundary lines: S03 owns the engine bump procedure and lane onboarding; S01 owns CI mechanics and the unit map; S14 owns watch/canary machinery; S15 owns how components are used. S16 owns the **inventory**, the **gate**, and the **pass**.

### S16.1 Adoption law

- **Report 14 §6.1 is the citable adoption reference** for the whole harvest, superseding `Docs/component-harvest-map-proposal-v1.md` [G2 D2.2; R14 §1]. Pattern/STUDY rows live with their consuming sections; this section binds the ADOPT consolidation and the exclusions.
- **Adopt, don't fork.** Every adopted component runs unmodified and is integrated by configuration, wrapping, or data consumption only. A component that would need patching is not adopted — it is built, or its pattern is copied (S16.5) [G2 D2.2; CLAUDE.md hard rule].
- **Organ-grade only.** Adoptions are data files, OS mechanisms, and single-purpose components — replaceable, pinnable, carrying no orchestration opinions. Nothing spine-shaped (session-owning, board-owning, orchestration-owning) is ever adopted: the 2025 control-plane cohort died at ~14-month half-life, and build-the-control-plane / adopt-the-organs is what that mortality teaches [R14 §4.1, §2.2; G1 D1.1].
- **Every adoption sits behind the S01.3 adoption seam:** pin + replacement path + abandonment criteria exist *before* first use — the exit plan pre-dates the need for it [S01.3; R14 §7 P-T16-1]. Replacement path + abandonment criteria together are the entry's **funeral plan** [R14 §2.2].
- Local **models** are not manifest entries: the serving stack is adopted here; the model set, its 6-monthly re-evaluation, and the swap⇒recalibrate gate are S12's registry, decoupled from workers by duty aliases [G3 §T15 digest; XREF:S12].

### S16.2 `components.lock` — format & enforcement

One manifest file, `components.lock`, in the platform repo [R17 §4.8; P-T16-1]. The **fields are normative; the serialization is P3's choice** (TBD-P3(lock file serialization), any text format diffable in review). Every entry carries, mandatorily:

| Field | Content |
|---|---|
| `name` | canonical component name |
| `kind` | `engine` \| `organ-unit` \| `library` \| `data` \| `frontend` \| `toolchain` \| `os-mechanism` \| `host-managed` \| `mechanism` \| `standby` |
| `pin` | exact version/tag/commit — never `latest`, never a floating range [S03.3]; content hash where vendored |
| `license` | SPDX id + **path scope** of the directories actually consumed + license-check date [R14 §6.4; P-T16-2] |
| `role` | one line + owning spec section |
| `replacement` | the named replacement-or-rebuild path (funeral plan, part 1) [P-T16-1] |
| `abandonment` | exit-trigger criteria (funeral plan, part 2); defaults to ⚙ `adoption.abandonment_months` (S16.7), overridable per entry [R17 §4.8] |
| `watch` | reference to the S14 watch/canary row(s) covering this entry [XREF:S14] |
| `last_review` | date of last quarterly-pass review [R17 §4.8] |

Rules with normative force:

- **CI fails when any running unit or bundled dependency lacks an entry** — the adoption seam enforced mechanically; enforcement mechanics are S01's [S01.11; G2 D2.2]. The S16.3 table is the complete v0 set: anything not listed there and not added through the S16.4 gate is a CI failure, not a judgment call.
- **`standby` entries** (term used as defined here: a pre-registered fallback that is pinned/named but not running or bundled) are exempt from the CI running-unit rule but MUST exist for every replacement path that names a concrete third-party component, so the funeral plan stays executable.
- **OS-mechanism entries are tracked differently from packaged dependencies.** systemd (transient units, timers, slices, systemd-creds), bubblewrap, seccomp, Landlock, and netns+nftables ride the host OS (Ubuntu 26.04 LTS); CI cannot pin the kernel. Their entries record: mechanism name, host-OS release, **last probe result + date** (the D2.3 probe set — cleared [SPIKE P2-S2]), and the ratified per-mechanism fallback (per-binary AppArmor profile; nftables-only egress — which is the boundary anyway; plain-copy workspaces) [G2 D2.3; XREF:S11]. An OS release upgrade is a deliberate event that re-runs the probes before the platform resumes normal admission.
- **`host-managed` entries** (tailscaled) are inventory-complete but not sinet-pinned; Sinet health-checks them (P-T13-1) and never manages their versions [S01.2].
- **Path-scoped licensing (P-T16-2).** Repo-level SPDX labels lie (`MIT except enterprise/` is now a standard shape); license verification is per-directory at the exact pinned ref, at adoption time and on every pin bump [R14 §2.5, §6.4]. Upstream LICENSE/NOTICE files and file headers are kept in-tree for everything vendored or copied — zero cost now, automatic compliance if anything is ever distributed [R14 §6.4]. NOTICE hygiene includes the SAW attribution line (honored though not legally required) [R14 §6.4].
- **Removal discipline.** An entry leaves the lock only by executing its funeral plan, recorded as a dated lock edit with the reason — never by silent deletion.
- **Engine pins are recorded here; the bump procedure is S03.3's.** A bump lands only through the operator-gated release event with conformance + quality probes, and every bump is a worker-revalidation trigger (P-T14-1) [S03.3; XREF:S08].

### S16.3 The v0 manifest

The consolidated ADOPT set this spec has bound [G2 D2.2 (R14 §4.1 Layers A+B); G3 D3.3; sibling sections as cited]. TBD-P3 pins are versions whose exact value is recorded when the lock file is first materialized at P3; the *entries* are binding now.

| Entry (kind) | v0 pin | License | Role → owner | Replacement path (funeral plan) | Watch |
|---|---|---|---|---|---|
| `claude` CLI (engine) | **2.1.214** [S03.3] | proprietary — Consumer ToS; gray-zone posture as-is, compliance quotes per R14 §7-OQ8 [G1 P2] | Anthropic lane → S03.2 | Agent SDK as verified in-adapter alternative, flip conditions pre-registered [S03.2]; billing un-pause → interactive-only demotion [G1 P1] | S2.8 drift canary + legal-text watch [XREF:S14] |
| `opencode-ai` (engine) | **1.18.3 exact** npm [S03.2] | MIT [R14 §2.1] | Z.AI + local lanes (`serve`) → S03.2 | OpenHands `software-agent-sdk` = pre-registered fallback spike target [G2 P4; R14 §6.2]; Goose re-eval after its 2.0/ACP consolidation [R14 §6.2] | v2 declared API break: pin holds until conformance passes [R14 §4.2] |
| llama-swap (organ-unit) | TBD-P3(release pin; v240 observed current) [R16 §2.3, §4.1] | MIT [R16 §2.3] | local-lane front, TTL unload, manual-unload verb → [XREF:S12] | llama-server router mode + Sinet kill-idle timer, or Ollama [R16 §4.1] | bus factor 1 → conformance suite asserts its contract on every bump [R16 §4.1] |
| llama.cpp `llama-server` (engine) | TBD-P3(`b`-tag pin) [R16 §4.1] | MIT [R16 §2.3] | local model server per GPU-UUID → [XREF:S12] | Ollama (standby row) | `/v1` logprobs + `json_schema` asserted **behaviorally** every bump — docs are never the check [R16 §7-OQ9; S03.3] |
| Ollama (standby) | v0.32.1 observed; pin on activation [R16 §2.3] | MIT core [R16 §2.3] | capability-complete local-lane fallback (incl. `/v1` logprobs) [R16 §4.1] | — (is itself the fallback) | re-run stack comparison if it gains per-model GPU pinning [R16 §4.10] |
| `sandbox-runtime` (library) | TBD-P3(pin at sandbox compose) | Apache-2.0 [R14 §6.4] | bwrap+seccomp+proxy core inside the composed sandbox stack → [XREF:S11] | rebuild: direct composition of the same kernel primitives the stack already ratifies [G2 §T09 digest, D2.3] | S2.8 watch [XREF:S14] |
| OS mechanisms: systemd (units/timers/slices/creds) · bubblewrap · seccomp · Landlock · netns+nftables (os-mechanism) | host OS = Ubuntu 26.04 LTS; probes cleared [SPIKE P2-S2; G2 D2.3] | OS packages, various [R14 §6.4] | shell + sandbox + broker-injection substrate → [XREF:S01; S11] | per-mechanism ratified fallbacks [G2 D2.3; XREF:S11] | re-probe on OS upgrade + quarterly pass |
| tailscaled (host-managed) | host-managed | — | tailnet front door (D1) → [S01.2] | — (D1-constitutive) | post-resume reconcile, P-T13-1 [XREF:S01] |
| Caddy (organ-unit) | TBD-P3(pin at onboarding) | TBD-P3(path-scoped verify at onboarding — no research input records it) | front router; admin-API preview routes → [S01.2; XREF:S13] | process-seam swap; routing config only, admin-API usage contained in the preview module [S01.3; XREF:S13] | quarterly pass |
| changedetection.io (organ-unit) | TBD-P3(pin; v0.55.8 observed) [R12 §2] | Apache-2.0 [R12 §2] | watchlist page-diff tier → [G2 Def.12; XREF:S14] | process-seam swap [S01.3]; native feed poller + canary layer carry the watch duty through any gap [G2 Def.12] | quarterly pass |
| promptfoo (library, runner) | TBD-P3(pin; v0.121.19 observed) [R12 §4] | MIT [R12 §4] | regression-eval runner; runner-never-store — all scores land in `platform.db` [G2 D2.1(d); P-T11-4] → [XREF:S14] | **DeepEval swap pre-registered** (standby; Apache-2.0) [R12 §4; G2 §T11 digest] | OpenAI-acquisition drift watch [R12 §2] |
| genai-prices `data.json` (data, vendored) | vendored + hash-pinned; TBD-P3(exact; v0.0.71 line observed) [R14 §3.2] | MIT [R14 §3.2] | shipped price-table defaults; user edits overlay, never overwritten → [XREF:S10] | LiteLLM JSON as primary + manual table (the original posture) [R14 §4.2] | scheduled refresh lands as a **proposal**, never a silent overwrite [G2 Def.16]; staleness ⚙ (S16.7) |
| LiteLLM `model_prices…json` (data, cross-check) | pinned per refresh | MIT **root only — path-scoped** (`enterprise/` carve-out) [R14 §6.4; P-T16-2] | price-breadth cross-check → [XREF:S10] | drop — cross-check is optional | rides the price-refresh task |
| sops (library/tool) | TBD-P3(pin) | MPL-2.0 [R14 §6.4] | per-person encrypted store format, broker mechanics → [XREF:S11] | gopass (same role, GPG+git flavor) [R14 §3.1] | quarterly pass |
| age (library/tool) | TBD-P3(pin) | BSD-3 [R14 §6.4] | store + snapshot encryption → [XREF:S11; S13] | gopass-class store alternative [R14 §3.1]; snapshot-lane crypto swap at S13's pipeline seam [XREF:S13] | X-Wing age-CLI plugin row (S16.8) |
| @hello-pangea/dnd (frontend) | **18.0.1** | Apache-2.0 | board drag-drop (S1.3) → [XREF:S15] | pinned @dnd-kit/core 6.3.1 (standby, frozen-but-working) | archive/unmaintained watch; @dnd-kit/react 1.0 row (S16.8) [frontend-components-v1] |
| react-diff-view + gitdiff-parser (frontend) | **3.3.3** | MIT | diff review surface, rendering the ported N15 review behavior (S3.5) → [XREF:S15] | monaco diff via @monaco-editor/react 4.7.0 (standby upgrade path) | single-maintainer watch [frontend-components-v1] |
| @assistant-ui/react (frontend) | **0.14.27** pinned | MIT | chat surface via LocalRuntime/ExternalStoreRuntime, no assistant-cloud (S3.6) → [XREF:S15] | owned chat primitives on the same SSE cursor — ExternalStoreRuntime keeps message state platform-owned, so the swap loses no data | 0.x churn vs quarterly-pass absorption; 1.0 row (S16.8) [frontend-components-v1] |
| JSON Forms (@jsonforms/core+react) (frontend) | **3.8.0** | MIT | generated settings UI renderer (13.4/13.5) → [S01.10; XREF:S15] | RJSF v6 (documented runner-up); re-entry condition: stall >18 months or a rule/layout vocabulary gap | stall watch [frontend-components-v1] |
| React 19 + Vite + TypeScript tree (toolchain) | lockfile-pinned; exact tree TBD-P3(SPA scaffold) | recorded at P3 onboarding | SPA build; dev-time only, no runtime service exposure [R17 §4.3, §5] | HTMX 2.x re-entry only via its named conditions [G3 D3.3; R17 §4.10; XREF:S15] | Vite majors adopted on a lag, never day-one [G3 D3.3 rider] |
| Go toolchain (toolchain) | TBD-P3(pin at scaffold) | recorded at P3 onboarding | backend build → [S01.5] | none live — Python/Litestar posture is documentation only [S01.5; G3 D3.2] | go1compat; quarterly pass |
| SQLite via pure-Go driver (library) | TBD-P3(driver pin) [S01.5] | SQLite public domain [R14 §6.4]; driver license recorded at P3 onboarding | `platform.db` engine + driver → [XREF:S02] | driver swap inside the storage module (storage seam) [S01.3] | currency policy: pin the release line, take patch releases deliberately [R17 §4.7] |
| AGENTS.md + CLAUDE.md shim (mechanism) | format as consumed at the pinned engine versions | — (format, no dependency) | repo-onboarding instruction mechanism [G2 D2.2 Layer A; R14 §6.1 N18] → [XREF:S05] | own instruction assembly (mechanism adoption — nothing to abandon) | format drift rides the engine conformance suites [S03.3] |

### S16.4 The component-onboarding checklist (P-T16-1 — the report-02 §5 twin)

Every future adoption passes this gate; it is the component twin of the provider/lane onboarding checklist that S03.6 owns [R14 §7 P-T16-1; G1 rider 3]. A lane addition never forces a new component (S03.6); a component addition never creates a lane. Checks, in order — all MUST pass:

1. **Niche & seam.** The role is named in one line, with the owning spec section and which S01.3 seam the component sits behind [S01.3].
2. **Organ-grade screen.** Single-purpose; no orchestration, session-ownership, or store-of-record opinions. Spine-shaped candidates fail here regardless of quality — build instead [R14 §4.1, §2.2].
3. **Health check from primary sources.** Maintenance cadence, bus-factor/commit concentration, security-response record, governance — checked at the candidate ref, the way R14 checked every current entry [R14 §2, method].
4. **License, path-scoped.** Verified per-directory at the exact candidate ref (carve-out scan: `enterprise/`, `ee/`, dual-license trailers); date recorded; LICENSE/NOTICE retained in-tree [P-T16-2; R14 §6.4].
5. **Capability claims verified behaviorally.** Probe the pinned version; conformance suites assert behavior, never docs [S03.3; R16 §7-OQ9].
6. **Adopt-unmodified check.** Integration is config/wrapping/data only; any needed patch disqualifies adoption [G2 D2.2].
7. **Runner-never-store.** Whatever the component computes, the system of record stays `platform.db` [G2 §Follow-ups, P-T11-4 criteria].
8. **Exact pin + funeral plan.** Pin (never `latest` [S03.3]); named replacement-or-rebuild path; abandonment criteria — all written before first use [P-T16-1; R17 §4.8].
9. **Watch registration.** A drift/canary/feed row registered into S14's executor; protocol touchpoints additionally pin the protocol version with a dated migration trigger [P-T16-3; XREF:S14].
10. **Lock entry + approval.** Entry written; CI green [S01.11]; the adoption lands as an operator-approved proposal — a platform-level change under D10, matching the precedent that every current entry was gate-ratified [G2 D2.2; G3 D3.3].

#### Lane onboarding records (the S03.6 twin's manifest home)

S03.6 owns the provider/lane onboarding checklist and names [XREF:S16] as its manifest home; a lane addition never forces a new component, so lanes are recorded here rather than in the `components.lock` table above. One row per onboarded lane, dated, with the audit that produced it.

| Lane | Provider / plan | Substrate → protocol | Billing regime | Overflow | API-equivalent price (D5) | Data-routing rider (C5) | Audit | Verified |
|---|---|---|---|---|---|---|---|---|
| `kimi` | Moonshot — Kimi Code membership (`kimi-for-coding`) | `opencode` → **`@ai-sdk/anthropic`** (base `https://api.kimi.com/coding/v1`) | **flat** | **`opt-in-credits`** on a PROVEN disable — verbatim: *"You can turn it off at any time: your balance stays in your account and the system pauses spending from it; turn it back on to resume."* **Operator posture: Extra Usage OFF**, under which the lane behaves as a hard stop and the ¥-denominated top-up rails are moot; turning it on is a deliberate, reversible operator act through the rehearsed S10.2 kill-switch, never automatic and never silent (3.10; P-T17-2) | **$3.00** in / **$15.00** out per M tokens, cache-hit **$0.30**/M, USD, 1,048,576 ctx. The plan is flat and opencode prices this provider at $0.00, so receipts price from these rows and never from the engine (the S03.7 corollary) | **`no-household-personal-data`** — approved for code and general technical work only; household personal data, personal correspondence and identity-bearing content must never route to it. RECORDED AND SURFACED, **not machine-enforced**: no per-lane data-policy enforcement point exists yet, and it lands with the routing-policy seam | `P3/measurements/2026-08-24-kimi-lane-gate-audit.md` | 2026-08-24 |
| `kimi-cli` | Moonshot — the SAME Kimi Code membership, reached through the first-party CLI (`@moonshot-ai/kimi-code@0.38.0`, npm, MIT) | **`kimi-cli`** → the pinned CLI itself, driven headless (`kimi -p --output-format stream-json`); no ai-sdk provider package, so the lane document's `npm` field is deliberately EMPTY and the engine is a `components.lock` entry instead | **flat** — and the SAME POOL as `kimi`: vendor verbatim 2026-08-26, *"provided together with a Kimi membership subscription and sharing the same quota. Requests from the CLI, VS Code, and third-party tools all count toward that quota."* The pool is declared ONCE on the plan document and a budget for it is refused on this lane in favour of the canonical one | **`opt-in-credits`**, inherited from the membership rather than the client path (Kimi on the web and Kimi Code share one Extra Usage balance). Operator posture: Extra Usage OFF | unchanged from `kimi` — one membership, one price table; receipts price from Sinet's rows and never from the engine | **`no-household-personal-data`**, identical to `kimi` because the constraint is a property of the PROVIDER and not of the client path. RECORDED AND SURFACED, not machine-enforced | `P3/measurements/2026-08-24-kimi-lane-gate-audit.md` (2026-08-26 Gate-A addendum) + `P3/measurements/2026-08-26-kimi-cli-print-mode-spike.md` | 2026-08-26 |

The metered-exception list stays **EMPTY** [G1 P7] — this is a flat subscription lane, and DeepSeek remains the sole pre-registered designated exception. Added by [S00.9 A11]; the `kimi-cli` row by [S00.9 A12], which also carries the THIRD SUBSTRATE this lane runs on — the one case in this table where a lane addition arrived with an engine, and the reason A12 is an amendment rather than an onboarding.

### S16.5 Layer C — patterns-only adoptions

Copied shapes, zero dependencies: no lock entries exist because nothing runs or is bundled. Each pattern's normative home is its consuming section; they are listed here so no future session mistakes them for adoptable runtimes [G2 D2.2 Layer C].

| Pattern | What is copied | What is NOT taken | Consumer |
|---|---|---|---|
| LangGraph interrupt/approval schema | The approval-item contract, verbatim: action request + `allow_ignore/allow_respond/allow_edit/allow_accept` + response types `accept/edit/response/ignore` [R14 §2.3, §3.4] | agent-inbox as a dependency (LangGraph+LangSmith-locked). The approval-inbox niche is **empty** in mid-2026 — S3.2 is **built, not adopted** [R14 §2.3] | approval inbox [XREF:S15] |
| gh-aw safe-outputs | Read-only-default effects gating, structured action requests, per-handler caps, sanitization, **default-on threat-detection pass** over proposed outputs; effects gate ≠ quality gate [R14 §6.1 G1 row] | the GitHub Actions runtime | effect journal + verification [XREF:S02; S07] |
| AG-UI event vocabulary | Event shapes mirrored in the SSE contract [R14 §3.3; frontend-components-v1] | CopilotKit runtime | live surfaces [XREF:S15] |
| Archon A2 dialect vocabulary | Own **versioned dialect** mirroring the proven vocabulary (prompt/bash nodes, `depends_on`, loop/until), with corrections baked in: fresh-context ON at stage boundaries; explicit approval node [R14 §6.2 A2] | their parser or runtime, ever — Sinet's control plane executes workflows [G1 rider 2] | workflow/automation dialect [XREF:S08] |
| Agent Vault egress-substitution | STUDY input to the broker workshop only (single-host voids its premise) [G2 P3; R14 §3.1] | the Infisical stack | broker design [XREF:S11] |
| Crush K1/K2 | Durable ask-record + pull-based enumeration; default-deny ToolPolicy **atop** the OS sandbox, never instead [R14 §6.1 K1/K2] | any Crush code — see below | already embedded [S03.4; XREF:S11] |

**Crush patterns-only is POLICY, not legal necessity.** FSL-1.1-MIT legally permits private, non-competing use including code — the harvest map's "legally required" rationale was corrected by R14. The operator ratified patterns-only anyway (adopt-don't-fork hygiene; Go/TUI stack mismatch; K1/K2 patterns suffice) [G2 D2.2; R14 §6.5]. Revisitable **per-version** as FSL→MIT conversions land from ~mid-2027 — a dated watch row (S16.8), operator decision only.

### S16.6 Anti-harvest register — the binding no-adopt list

These rows are settled; a future session re-litigates one only through S00.9 amendment mechanics with new primary evidence. Terse by design [R14 §6.5, §5; G2 D2.2].

| No-adopt | Reason (one line) | Binding rule |
|---|---|---|
| Engine forks / Nexus core-mods | adopt-don't-fork; the keystone need (per-session models) is native upstream [R14 §6.5] | no modification to any adopted engine, ever |
| Standing specialist roster | single-agent-first is the ratified, field-convergent posture [G1 D1.1; R14 §6.5] | machinery only when earned (14.4) [XREF:S04; S08] |
| Monolith topology | bus-factor-1 death shape; cohort mortality [R14 §6.5; R17 §6] | the five seams bind [S01.3] |
| Peer-mesh / group-chat topologies | 17.2× vs 4.4× error amplification; 72% fact erasure [R14 §6.5] | D6 / 14.1: no lateral messaging at any depth |
| Judge-only-frontier / cheap-executor-by-default | the bench-02 failure; inversion #2 [R14 §6.2 S2 row, §6.5] | executor floors ride effort modes [XREF:S10] |
| JARVIS breadth (voice/avatar/galaxy) | 14.5 freeze; validate before breadth [R14 §6.5; frontend-components-v1 §3] | re-entry only via the 15.3 benchmark gate, as satellites (12.4) |
| Vector-first memory sidecars (mem0/qdrant-class) | benchmark basis collapsed under audit; files+FTS5 win [R14 §6.5] | vector post-gate only, rank-only [G2 Def.8; XREF:S09] |
| Live-tree symlinks / machine-coupled deploys | CI-verified artifacts + transient units ratified [R14 §6.5] | deploy per S01.11 only |
| Fire-and-forget triggers | every ingress spawns under the scheduler with owner attribution (3.4) [R14 §6.5] | no unattributed execution path exists [XREF:S10] |
| Load-bearing metered paths | two of three majors tightened in H1-2026; revocable forbearance is the fragility [R14 §6.5] | 3.10; metered-exception list EMPTY at v0 [G1 P7] |
| Crush code | policy, per S16.5 | patterns-only until a dated operator revisit |

**Dependency classes rejected with cause** (each with its named replacement-in-kind already in this spec) [R14 §5]: the dead control-plane cohort (HumanLayer, Vibe Kanban, Omnara, Terragon, Crystal — STUDY/archival only); agent-inbox (deployment-locked); MCP elicitation as approval spine (mid-restructure; at most a future inbox *adapter* behind a seam [P-T16-3]); container-use (superseded by the ratified kernel-primitive stack); OpenMeter/Lago/LiteLLM-proxy as metering (proxies are structurally blind to CLI-wrapped subscription traffic); Vault/OpenBao/Infisical core (one-host overkill; OpenBao noted as the license-safe successor if a vault is ever needed); tokencost (stale); trace platforms as system of record (P-T11-4; projection targets at most); n8n (condition lapsed — own dialect + C0 connectors); durable-execution spines LangGraph/Temporal/Restate/Inngest/DBOS (shape mismatch; **DBOS = pre-registered fallback only** [R14 §3.5]).

**No-public-registry-imports (P-T16-4).** The self-extending workforce NEVER auto-imports public skills/templates/workers (ClawHavoc poisoned ~12% of a public registry). Anything a human imports is untrusted content under 4.7 and enters only through D10 gates with provenance recorded. Enforcement lives in the composer/import path [R14 §2.6, §7 P-T16-4; XREF:S08].

### S16.7 The quarterly dependency pass ⚙

A scheduled, operator-visible pass over every lock entry [G3 D3.3 rider; R17 §4.3, §4.8], co-scheduled with the snapshot restore drill [R17 §4.8; XREF:S13]. Per entry it checks:

- **Version currency** — upstream releases and security advisories vs the pin; candidate bumps identified (engines route through S03.3's gate; components through their conformance/behavior checks).
- **License drift** — path-scoped re-check at the candidate ref on any proposed bump [P-T16-2].
- **Archive/abandonment** — observed activity vs the entry's abandonment criteria; a breach proposes activating the funeral plan.
- **Peer-range / toolchain health** — frontend peers vs pinned React/Vite; Vite-major lag respected [G3 D3.3; frontend-components-v1].
- **Watch-row hygiene** — S16.8 rows reviewed for staleness; `last_review` updated.

**Outputs are proposals, never silent bumps** [S03.3; G2 Def.16 as the shipped precedent]: bump proposals, exit-plan activations, and watch-row retirements, each an operator-approved, dated lock edit. The first pass (at P3) additionally records the TBD-P3 onboarding facts left open in S16.3 (Caddy + toolchain licenses, exact pins) and runs the R13-OQ7 verification (S16.8).

### S16.8 Watchlist adds — standing rows registered into S14's executor

Registration only; polling machinery, cadence, and alert routing are S14's [G2 Def.12; XREF:S14].

| Row | Watches | Fires → |
|---|---|---|
| Fine-grained-PAT collaborator gap | GitHub PAT capability change | broker/push posture options widen [G2 §Follow-ups; XREF:S13] |
| Free-plan ruleset availability | enforced rulesets on Free private repos | server-side second guardrail beyond the broker (P-T12-1) [G2 §Follow-ups; XREF:S13] |
| diffity maturity | project maturity | S13 diff-tooling candidate re-eval [G2 §Follow-ups] |
| jj compat matrix | jj/git compatibility | S13 git-machinery re-eval (no jj at v0) [G2 §Follow-ups; G2 D2.1(e)] |
| X-Wing age-CLI plugin | plugin status | broker/backup crypto options [G2 §Follow-ups; XREF:S11; S13] |
| DBOS TS/SQLite parity | fallback health | durable-execution fallback stays credible [G2 §Follow-ups; R14 §3.5] |
| MCP 2026-07-28 final (Tasks/Extensions) | protocol restructure landing | P-T16-3 dated migration trigger; inbox-adapter seam re-eval [R14 §2.3, §7; XREF:S03] |
| ACP v1 line | verb-set/transport convergence | adapter-contract alignment check (verbs already a superset) [R14 §6.1 O4; S03.1] |
| Litestream #1083 | silent-replication-failure triage | re-decide the Litestream unit at implementation [G2 D2.5; XREF:S01; S13] |
| Python-Redlines + pandiff | license/maintenance — verified at the first quarterly pass (the P3 dependency audit, R13-OQ7) | S13 doc-diff candidates admitted or dropped [G2 §Follow-ups] |
| @dnd-kit/react 1.0 | 1.0 with React-19-clean peers | re-evaluate the board pick with a dated note [frontend-components-v1] |
| assistant-ui 1.0 | stability promise or hostile 1.0; 0.x churn rate | re-pin, or owned-primitives fallback [frontend-components-v1] |
| awesome-harness-engineering referents | `walkinglabs/…` + `ai-boost/…` activity | drop the row if both stall [G2 Def.15] |
| Crush FSL→MIT conversions | per-version MIT from ~mid-2027 | operator may revisit patterns-only policy (S16.5) [G2 D2.2; R14 §4.2] |
| genai-prices staleness | >⚙ `adoption.price_data_stale_days` without updates while providers reprice | flip to LiteLLM-primary + manual table [R14 §4.2] |
| opencode v2 | GA / declared server-API break | bump held at the S03.3 conformance gate [R14 §4.2] |

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `adoption.dependency_pass_months` | 3 | (1, 6) [coordinator-draft] | G3 D3.3 rider; R17 §4.8 |
| `adoption.abandonment_months` | 6 [coordinator-draft] | (2, 24) [coordinator-draft]; per-entry overridable | R17 §4.8 (⚙-flagged, unnumbered); P-T16-1 |
| `adoption.price_data_stale_days` | 60 | (14, 180) [coordinator-draft] | R14 §4.2 |

**Known problems owned here:**

- **P-T16-1** — ecosystem mortality (~14-month half-life): discharged — onboarding checklist (S16.4) + mandatory funeral-plan fields (S16.2) + CI presence rule [S01.11].
- **P-T16-2** — per-directory license splits: discharged — path-scoped license field + checklist #4 + quarterly re-check on bumps (S16.2/S16.7).
- **P-T16-3** — protocol-surface churn as a scheduled event: protocol touchpoints pin their protocol version in the lock with dated migration triggers (MCP/ACP rows, S16.8); canary machinery [XREF:S14]; adapter mechanics [XREF:S03].
- **P-T16-4** — registry supply chain: the no-public-registry-imports rule bound at S16.6; enforcement in the composer/import path [XREF:S08].

Referenced, owned elsewhere: P-T14-1 (bump = mass revalidation event — S03.3 [XREF:S08]); P-T11-4 (runner-never-store — checklist #7, promptfoo row); P-T12-1 (broker-is-the-guardrail — watch rows 1–2 [XREF:S13]).

**Deferred / parked:**

- Litestream unit → re-entry: #1083 triaged at implementation [G2 D2.5].
- Crush code harvest → re-entry: per-version FSL→MIT conversions from ~mid-2027, operator policy revisit (S16.5/S16.8).
- @dnd-kit/react family → re-entry: its 1.0 with clean React-19 peers, dated note [frontend-components-v1].
- vLLM as eGPU batch backend → re-entry: a measured throughput need; llama-swap manages it unchanged [R16 §4.1].
- xAI-via-opencode lane entry → re-entry: a member holds the plan; S03.6 onboarding checklist, config-only [G1 P6; R14 §4.1].
- HTMX 2.x architecture re-entry → only via its named conditions [G3 D3.3; R17 §4.10; XREF:S15].

**Coverage:**

| Feature-list item | Where |
|---|---|
| D3-adjacent adoption law (adopt-don't-fork, one manifest) | S16.1, S16.2 |
| S2.8 external-change watch — component/protocol drift rows (machinery → S14) | S16.3 watch column, S16.8 |
| 7.3 model/engine deprecation vs stored configurations — the pin registry as the revalidation clock | S16.2, S16.3 (procedure [XREF:S03; S08]) |
| 13.4 settings surface (pass cadence, abandonment, staleness as ⚙) | S16.7 |
| 14.1 / 14.4 / 14.5 deliberate non-goals as binding no-adopt rows | S16.6 |
| 3.10 + Operating reality (no load-bearing metered paths) | S16.6 |
| 4.7 untrusted imported definitions (P-T16-4) | S16.6 |

**Open items for G4:** none. Drafting-time sub-choices flagged inline as [coordinator-draft]: the `adoption.abandonment_months` = 6 default and the three clamp ranges (R17 §4.8 ⚙-flags the number without fixing it). The TBD-P3 cells in S16.3 (Caddy + toolchain licenses, exact pins) are recording tasks at the first quarterly pass, not decisions.

## S17 — Known-problems register

**Scope:** The single consolidated register of every known platform problem (`P-*`) filed anywhere in the campaign — id, problem, origin, owning section, disposition, and the checks it feeds — plus the named problems that carry no `P-*` id. This section consolidates and reconciles; it introduces no new problems and re-litigates no dispositions.
**Binding inputs:** the **Known problems owned here** blocks and inline `P-*` dispositions of S01–S16 (committed drafts) · gate follow-up lists: [G1 §Follow-ups] (P-T01-1..4, P-T17-1..3, P-T05-1..4, P-T06-1..5, P-T02-1..5, R06×4, R07×4), [G2 §Follow-ups] (P-T07-1..5, P-T08-1..5, T09×2 + T10×3 descriptive, P-T11-1..5, P-T12-1..4, P-T16-1..4), [G3 §Follow-ups] (P-T13-1..3, P-T14-1..2, R16×2 descriptive) · `Research/STATE.md` (drafting-coined id list; spike conclusion headlines) · [R02 §9] (T17 problem statements) · feature list §"Known problems this program must solve" · id-coinage notes in S04, S05, S09, S11, S12.

### S17.1 Register conventions

- **Id convention:** `P-T<topic>-<n>` — topic numbers per the campaign topic board (`Research/STATE.md` §Topic board); topic↔report numbering differs because reports are numbered in completion order. The **Origin** column names the filing gate and report; `→S##` marks a family whose descriptive gate filing received its ids in that drafting section (each such section carries its own coinage note).
- **Owner** = the section whose *Known problems owned here* block (or coinage note) holds the disposition; a `·`-joined pair is a deliberate split home, both sides cross-referencing each other. Three ownership adoptions made by this register are tagged `[coordinator-draft]` and justified in S17.6.
- **Disposition** is a one-line summary; the owner's text is normative and is never restated beyond that line. **Registrations** lists the standing machinery the problem feeds — conformance-registry entries, watchdog checks, canaries, eval-practice hooks, watch rows — by [XREF].
- Rows are grouped by filing gate. Family counts are counts of what the corpus actually contains, verified by sweep (S17.6-R7); the only id filed by a gate and absent from the drafts was P-T17-2, restored in S17.2/S17.6. The register totals **64 ids**:

| Gate | Families (count) | Subtotal |
|---|---|---|
| G1 (S17.2) | T01(4) T02(5) T03(4) T04(4) T05(4) T06(5) T17(3) | 29 |
| G2 (S17.3) | T07(5) T08(5) T09(2) T10(3) T11(5) T12(4) T16(4) | 28 |
| G3 (S17.4) | T13(3) T14(2) T15(2) | 7 |

### S17.2 G1-filed problems (Wave A topics: T01, T02, T03, T04, T05, T06, T17 — 29 ids)

| Id | Problem (one line) | Origin | Owner | Disposition (one line) | Registrations |
|---|---|---|---|---|---|
| P-T01-1 | Engine transcripts are not durable checkpoints (cwd-keyed, sweepable, corruptible) | G1·R01 | S03 §S03.3; enforced S02 §S02.4 | Platform checkpoint rows authoritative; engine sessions + transcript copy-aside are resume optimization and harvest material only | copy-aside indexed in `engine_sessions` [XREF:S02]; ask-records durable on observation [XREF:S03] |
| P-T01-2 | Engine schema/behavior drift is permanent, even first-party | G1·R01 | S03 §S03.3; extended S11 §S11.5 | Pin exact + operator-gated deliberate bump + forward-tolerant parsing | per-lane conformance suites, kill-9 harness standing entry [XREF:S14]; model-egress MITM-tolerance canary (cert-pinning regression) [XREF:S11][XREF:S14] |
| P-T01-3 | Headless-subscription billing shifts are ~30-day ops events | G1·R01 | S03 §S03.3 | Flat/metered as data; pre-registered un-pause response = interactive-only demotion [G1 P1]; rehearsed flip, never an architecture change | receipt currency visibly flips [XREF:S10]; S2.8 billing/legal watch [XREF:S14] |
| P-T01-4 | Cancel corrupts engine JSONL mid-tool-call | G1·R01 | S03 §S03.1/§S03.3 | Boundary-first cancel ladder + session-health probe before reuse; fork-from-checkpoint is the standard recovery | adapter-suite park/cancel/resume assertions [XREF:S14] |
| P-T02-1 | Parked engine state silently expires (cleanup-sweep evaporation) | G1·R05 | S03 §S03.4 | Platform ask-record authoritative from the moment observed; engine cleanup horizon raised above the park horizon | ⚙ `adapter.claude.cleanup_period_days` [XREF:S18]; park/resume conformance [XREF:S14] |
| P-T02-2 | Resume-cache fidelity drifts silently | G1·R05 | S03 §S03.3 `[coordinator-draft]` (S17.6-R2) | Standing cache-fidelity alarm: `cache_read` stuck flat across an in-TTL resume vs the calibrated healthy baseline [SPIKE G1-S1 F3] | alarm machinery [XREF:S10][XREF:S14]; P-T08-5's quarterly calibration joins the same suite [XREF:S10] |
| P-T02-3 | Suspend/wake is a crash-equivalent double-resume factory | G1·R05 | S02 §S02.5 | Generation fencing (stale-generation appends rejected) + suspend-aware lease grace render double-resume inert | suspend-cycle test (fake `PrepareForSleep` + clock deltas) [XREF:S14] |
| P-T02-4 | `defer` is single-tool-call-only; parallel gated turns fall back silently (~20%) | G1·R05; SPIKE G1-S2 | S03 §S03.4 | Adapter-side fallback detection + serialize-by-deny as the chosen fallback [SPIKE P2-S4] | ⚙ `adapter.parallel_gate_fallback`; TBD-BRINGUP(fallback rate on default worker model); TBD-P3(serialize-by-deny reconfirm) [XREF:S02] |
| P-T02-5 | The engine harness is itself a quality-drift source invisible to schema tests | G1·R05 | S03 §S03.3 | Engine bump = release event gated on before/after quality probes, not only conformance | bump-gating quality probes + engine-behavior canaries [XREF:S14] |
| P-T03-1 | Whether cache reads weigh less against subscription quotas is publicly unspecified | G1·R06 §7 →S04 | S04 §S04.7 | 0.1× "assumed" weighting + raw counts on receipts [G1 Def.10]; resolution re-opens stagger + weighting as settings changes | provider-watchlist resolution watch [XREF:S14]; weighting consumers [XREF:S05][XREF:S10] |
| P-T03-2 | Sibling-failure containment is Sinet-owned (field default = fail-fast cancellation) | G1·R06 §7 →S04 | S04 §S04.5 | One helper's death/rejection never cancels siblings and never fails the task; salvage carries explicit unfinished markers | mid-run helper-kill conformance test (containment-with-salvage) [XREF:S14] |
| P-T03-3 | No engine enforces D6 (depth caps, spawn logging, no-lateral) | G1·R06 §7 →S04 | S04 §S04.1/§S04.4 | Control-plane-only structural enforcement; engine-native spawning disabled per lane [G1 rider 2] | D6 violation-attempt suite: depth-3, lateral send, native spawn, over-budget spawn [XREF:S14][XREF:S03] |
| P-T03-4 | Helper reports are an injection surface | G1·R06 §7 →S04 | S04 §S04.3 | Reports are data: advisory instruction-pattern screen; structural defense = minimal tools/context per helper | untrusted-content helper confinement [XREF:S11]; steered-output verification backstop [XREF:S07] |
| P-T04-1 | Compaction survival of pinned content is an assumption until tested | G1·R07 §7 →S05 | S05 §S05.7 | Pinned ledger sections re-injected verbatim after any compaction; post-compaction fidelity check on the local tier | canary-constraint→compact→adherence probe, standing conformance entry, re-measured per engine pin [XREF:S14] |
| P-T04-2 | Injected knowledge drifts (stale conventions, shim divergence) | G1·R07 §7 →S05 | S05 §S05.5 | Shim-drift hash check at every assembly + repo-diff-triggered staleness proposals; detection scheduled, never assumed | convention-update proposals via the knowledge gate [XREF:S09] |
| P-T04-3 | Consumption units are lane-heterogeneous (tokens vs prompts vs requests) | G1·R07 §7 →S05 | S10 §S10.1/§S10.4 `[coordinator-draft]` (S17.6-R3) | Approximation-tier hierarchy: Z.AI prompt units stay tier-3 derived and dashboard-reconciled; S05 contributes lane-aware stage granularity | approximation tier stamped on every row and receipt [XREF:S10]; TBD-OPERATOR(Z.AI dashboard prompt-unit calibration) |
| P-T04-4 | Engine-compaction behavior drifts per version (undisableable, version-unstable) | G1·R07 §7 →S05 | S05 §S05.7 | Containment stance: stage-fit budgets keep working sets clear of the trigger; the compactor is never fought or rebuilt | compaction behavior in the canary suite per engine bump [XREF:S14][XREF:S03] |
| P-T05-1 | Plan-stage confinement must bind helpers (prompt-level lockouts fail) | G1·R03 | S06 §S06.6 (policy) · S11 §S11.6 (mechanics) | Declared class flows to every helper tighter-only, enforced at sandbox level; widening is a delta re-plan + re-approval | spawn admission check refuses looser-than-coordinator classes [XREF:S11][XREF:S04] |
| P-T05-2 | Delta re-approval cards carry rubber-stamp risk (no field evidence either way) | G1·R03 | S06 §S06.9 | Delta-only cards + mandatory measurement hook (delta size, time-to-decision, decision, outcome linkage) | analysis owned by the eval practice; measured rubber-stamping proposes a card-format retune [XREF:S14] |
| P-T05-3 | Clarification/intake is an injection surface | G1·R03 | S06 (v0 posture) · joint S11 | v0 intake surface = authenticated workspace only; quoted/pasted material is data, never instructions (4.7) | full C3-class treatment re-enters with 15.5 multi-channel ingress [XREF:S11] |
| P-T05-4 | No published stakes-classification accuracy exists | G1·R03 | S06 §S06.2 | Tolerated by design: deterministic floors override upward, misclassification is cheap by construction, failure fails closed | run-end estimate-vs-actual recording; pre-registered classifier eval before v1 household use [XREF:S14] |
| P-T06-1 | Verifier rot: check suites and rubrics decay silently | G1·R04 | S07 §S07.9 | `verified-on` stamps + scheduled planted-defect audits; stale suites flag verdict cards; audit failure quarantines | ⚙ `verification.check_audit_interval_days`; planted-defect suites in the eval machinery [XREF:S14] |
| P-T06-2 | Retrieval-contaminated verification (pass by lookup, not work) | G1·R04 | S07 §S07.3/§S07.9 | Verification confinement defaults: network off + answer-bearing VCS history stripped; research nodes are the sanctioned lookup path | enforcement mechanics [XREF:S11] |
| P-T06-3 | Style-inflated judging (well-formatted mediocrity over-scores) | G1·R04 | S07 §S07.9/§S07.10 | Judges receive claims/diffs, not presentation; behaviorally anchored rubrics with extractive evidence quotes | per-judge length-bias noted in the rubric bundle, re-measured on judge change [XREF:S14] |
| P-T06-4 | Escalation-path death (the route exists in config but not in fact) | G1·R04 | S07 §S07.7/§S07.9 | Every finding terminates in a human-visible sink; no other sink type exists | three liveness proofs: planted-defect e2e class, daily dead-man canary, quarterly drill [XREF:S14] |
| P-T06-5 | Judge/rubric calibration is invalidated by model change | G1·R04 | S07 §S07.9/§S07.10 | Judge model pinned per rubric version; any change gates on a golden-set re-run before unsupervised judging resumes | revalidation-runbook block [XREF:S14]; local duty-alias swaps included [XREF:S12] |
| P-T17-1 | Provider sanction is allowlist-shaped and revocable server-side without notice | G1·R02 §9 | S03 §S03.6 | Auth canary distinct from limit handling; auth-shaped failure = policy-revocation-suspected → lane freeze + operator alert, never retry-park | ⚙ `canary.auth_interval` daily schedule [XREF:S14]; Class-4 action in the limit taxonomy [XREF:S10] |
| P-T17-2 | Auto-overflow-to-metered plan designs silently violate 3.10 | G1·R02 §9; id restored here `[coordinator-draft]` (S17.6-R1) | S03 §S03.6 | `overflow_mode` classified per model at onboarding; `auto-metered` without a proven disable/zero-balance is rejected | rejection rule [XREF:S10 §S10.2]; receipts visibly change currency on any metered overflow [XREF:S10 §S10.10] |
| P-T17-3 | A lane's model list is region- and account-dependent, not what provider docs say | G1·R02 §9 | S03 §S03.6 | Adapter diffs each account's observed model list against config; routing and 2.7 gap advice consume the observed list with `verified-on` dates | model-list drift on the canary schedule [XREF:S14] |

### S17.3 G2-filed problems (Wave B topics: T07, T08, T09, T10, T11, T12, T16 — 28 ids)

| Id | Problem (one line) | Origin | Owner | Disposition (one line) | Registrations |
|---|---|---|---|---|---|
| P-T07-1 | The pre-sleep inhibitor budget is short and host-configurable (5 s stock / 30 s v0 host) | G2·R08; SPIKE P2-S2 | S02 §S02.5 | Pre-sleep is an O(1) durable flush; all reconcile is wake-side; nothing heavy is ever sized to the window | suspend-cycle test [XREF:S14]; TBD-BRINGUP(operator suspend-session probe) [XREF:S01] |
| P-T07-2 | Reboot destroys the unit-corpse evidence that restart preserves | G2·R08; SPIKE P2-S2 | S02 §S02.5 | The run wrapper's own exit-record append is the mandatory durable evidence; corpses + engine stores are enrichment only | kill-9 harness classification assertions [XREF:S14]; TBD-OPERATOR(reboot-survival host probe) [XREF:S11] |
| P-T07-3 | Idempotency-less channels leave an irreducible unknown-outcome window | G2·R08; G2 D2.4 | S02 §S02.7 | `unknown` is a first-class effect state with a human card; idempotent-capable providers required, plain SMTP an explicit exception | zero-double-executed-effects asserted by the kill-9 harness [XREF:S14] |
| P-T07-4 | No single clock is trustworthy on a sleeping laptop | G2·R08 | S02 §S02.5 | Ordering only from `event_seq`; deadlines are wall-clock DB columns evaluated suspend-aware (wake grace) | suspend-cycle test asserts wake-grace + fencing behavior [XREF:S14] |
| P-T07-5 | The event log is itself a growth/bloat failure mode | G2·R08 | S02 §S02.1 | Payload caps, refs-not-blobs, validate-before-persist | event-log size watch feeding the Tier-0 watchdog [XREF:S14]; ⚙ `state.event_payload_cap` [XREF:S18] |
| P-T08-1 | Engine usage/cost *values* drift and break, not just schemas (3–8× dup, 10× inflation) | G2·R09 | S10 §S10.1 | Ledger sanity bounds + tier-1-vs-tier-2 divergence alarm; no row silently prices to $0 (UNPRICED is visible) | ⚙ `meter.value_divergence_alarm`; conformance suites assert values on known fixtures [XREF:S14] |
| P-T08-2 | Budget, policy, and auth events masquerade as one another on the wire | G2·R09 | S10 §S10.5 | The five-class limit classifier is a tested component; the named worst case is retry-parking a revoked lane | per-lane classifier fixtures [XREF:S14]; Class-4 routes to the P-T17-1 lane freeze [XREF:S03] |
| P-T08-3 | Prices carry effective dates, not just values | G2·R09 | S10 §S10.3 | Effective-dated price table with future-dated rows first-class; drift fires the freshness trigger [G1 Def.5] | watcher diffs announced *future* prices → proposal cards [XREF:S14] |
| P-T08-4 | Pause semantics can destroy deferred work (the drop-on-expiry shape) | G2·R09 | S10 §S10.4 | "Pause preserves everything queued and parked" is a named invariant of one-switch pause and maintenance mode | mandatory test on the pause paths [XREF:S14] |
| P-T08-5 | The pressure gauge inherits the ⚙0.1× cache-weight assumption | G2·R09 | S10 §S10.4 | "Assumed" label stays visible; quarterly calibration against observed depletion events, never window modeling | joins P-T02-2's cache-fidelity alarm suite [XREF:S14]; unified-header observed overlay [XREF:S11] |
| P-T09-1 | Agent-writable configuration is an escape surface (CVE-backed) | G2·R10 (descriptive) →S11 | S11 §S11.7 | Sandbox-side deny on all config paths (symlink-resolved) + empty-env deny-by-default; confinement never depends on engine memory | engine-lowering leak tests per config channel [XREF:S03][XREF:S14]; listener lint [XREF:S01]; guardrail-split schema [XREF:S08] |
| P-T09-2 | Allowlisted egress is an exfil channel (residual) | G2·R10 (descriptive) →S11 | S11 §S11.7/§S11.10 | Bounded, not closed: minimal reachable surface + method restriction + injection proxy; honest residual in the blast-radius invariant | egress volume/pattern anomaly hook in the watchdog suite [XREF:S14] |
| P-T10-1 | Engine-native memory drift is an 8.1-bypass class | G2·R11 (descriptive) →S09 | S09 §S09.9 | Disable-or-contain via compiled config; contained memory dirs are L0, wiped with the task | engine-memory canary entry re-checked per engine pin bump [XREF:S14]; mechanics closed A2 (2026-07-22, P3-B3-1) [XREF:S03] |
| P-T10-2 | Proposal-noise economics are unmeasured field-wide | G2·R11 (descriptive) →S09 | S09 §S09.4 (v1 activation) | Accept/dismiss rates instrumented from the pipeline's first day; low acceptance is a drafter/cap defect, never a gate weakening | metrics owned by the eval practice, reviewed at 11.2 checkpoints [XREF:S14] |
| P-T10-3 | Knowledge-injection budget pressure (landfill risk) | G2·R11 (descriptive) →S09 | S09 §S09.8 | Per-scope budgets, deterministic drop order, over-budget drops manifested + carded; silent truncation banned | budget-enforcement conformance test [XREF:S14]; trace-manifest `over_budget_dropped` entries [XREF:S05] |
| P-T11-1 | Benchmark blinding partially fails at household scale | G2·R12 | S14 §S14.7 | Uniform render template; blindness measured, never assumed (BENCH-REG §5) | mandatory arm-guess per verdict; guess accuracy beside every published rate |
| P-T11-2 | The benchmark's direct arm is un-pinnable (consumer surfaces auto-upgrade) | G2·R12 | S14 §S14.7 | Epoch tracker: an observed identity change ends the epoch; no decision statistic pools across epochs (BENCH-REG §9) | per-pair model identity recorded |
| P-T11-3 | Watcher sources decay structurally | G2·R12 | S14 §S14.6 | Per-source fetcher escalation ladder; a dead watch must announce itself | ⚙ `watchlist.fetch_fail_streak` meta-watch; standing migrate-to-feed bias |
| P-T11-4 | Observability/eval tooling ownership churn (M&A risk) | G2·R12 | S14 §S14.1 | Tools are runners or projectors, never stores; the record always lives in `platform.db` | binding adoption criteria on manifest rows + onboarding checklist #7 [XREF:S16] |
| P-T11-5 | Small-n statistical honesty is a product surface | G2·R12 | S14 §S14.7 | The honest-claims table renders wherever a win rate renders; every rate carries n, `G`, tie/decline/guess rates (BENCH-REG §15) | — |
| P-T12-1 | The broker is the ONLY ref protection on Free private repos | G2·R13 | S13 §S13.6 | Stated security property: broker-only keys, protected-ref policy as registry data, CAS-only pushes | broker-side refusal conformance test [XREF:S14]; PAT-gap + Free-ruleset watch rows [XREF:S16] |
| P-T12-2 | Comment orphaning is guaranteed at some rate (~22% open-web baseline) | G2·R13 | S13 §S13.3/§S13.4 | Explicit ORPHAN state; no comment without a render location; orphaned findings still delivered | anchoring-quality 11.2 spot-check [XREF:S14] |
| P-T12-3 | Encrypted remnants make key compromise retroactive | G2·R13 | S13 §S13.10 | Escrow-identity hygiene + annual snapshot-repo rotation; Support-ticket purge as last resort | ⚙ `backup.repo_rotation`; TBD-OPERATOR(age identity escrow before the first snapshot push) |
| P-T12-4 | Snapshot substitution is undetectable by encryption alone (age has no sender auth) | G2·R13 | S13 §S13.10 | The SHA-256 snapshot ledger (DB + in-archive) is load-bearing integrity | fail-closed ledger verify in the scheduled restore drill [XREF:S14] |
| P-T16-1 | Ecosystem mortality: ~14-month half-life for agent components | G2·R14 §7 | S16 §S16.2/§S16.4 | Component-onboarding checklist + mandatory funeral-plan fields before first use | CI lock-presence rule [XREF:S01]; quarterly dependency pass (S16.7); frontend-tree enforcement [XREF:S15] |
| P-T16-2 | Repo-level licenses lie; per-directory splits are standard | G2·R14 | S16 §S16.2 | Path-scoped license field verified per-directory at the exact pinned ref | checklist #4; re-check on every pin bump (S16.7) |
| P-T16-3 | Protocol-surface churn is a scheduled event, not a surprise | G2·R14 | S16 §S16.8 | Protocol touchpoints pin their protocol version with dated migration triggers | MCP 2026-07-28 + ACP v1 watch rows [XREF:S14][XREF:S03] |
| P-T16-4 | Registry supply chain: public skill/template registries are poisoned (~12% observed) | G2·R14 §7 | S16 §S16.6 (rule) · S08 §S08.10 (enforcement) (S17.6-R4) | No auto-import from any public registry, ever; human imports pass the full battery through the D10 gate | import battery: lint + instruction-pattern screen + permission audit + dry run [XREF:S08] |

### S17.4 G3-filed problems (Wave C topics: T13, T14, T15 — 7 ids)

| Id | Problem (one line) | Origin | Owner | Disposition (one line) | Registrations |
|---|---|---|---|---|---|
| P-T13-1 | Post-resume network-identity reconcile (`tailscaled` documented to wedge after wake) | G3·R17 §7 | S01 §S01.7 (detection/duty) · S11 §S11.8 (privileged path) | Wake-side health-check + fixed-verb `sinet-netremediate.service` under the scoped polkit grant [G3 Def.7] | resume-reconcile watchdog check [XREF:S14]; every remediation trigger event-logged [XREF:S11] |
| P-T13-2 | Listener-binding drift silently collapses the identity story | G3·R17 §7 | S01 §S01.6/§S01.8 | Fail-closed startup lint + explicit operator-visible allowlist; re-run at every resume | recurring watchdog audit; foreign violation = flag-now [XREF:S14] |
| P-T13-3 | Three metadata observables leave the LAN by design (CT logs, push metadata, TLS issuance) | G3·R17 §7 | S01 §S01.8 | Complete signed register; a fourth observable only via S00.9 amendment | TBD-OPERATOR(observables-register sign-off); frontend no-new-observables duty [XREF:S15] |
| P-T14-1 | Engine-pin bumps are mass revalidation events with no provider clock | G3·R15 §7 | S08 §S08.10(b) | Sinet's deliberate-bump procedure IS the announcement clock; every affected template flagged before further unsupervised use | bump gate [XREF:S03]; revalidation runbook executes the re-runs [XREF:S14]; engine-behavior canaries between bumps [XREF:S14] |
| P-T14-2 | Worker definitions are a prompt-injection carrier class (91% of malicious registry skills) | G3·R15 §7 | S08 §S08.10 | Station-1 instruction-pattern screen on every draft/import; static-only skills; review-as-diff; the guardrail split keeps enforcement unreachable | battery re-runs on every version change [XREF:S08] |
| P-T15-1 | Model churn invalidates calibration (thresholds, confidence maps) | G3·R16 ("model-churn invalidates calibration") →S12 | S12 §S12.10 | Swap ⇒ recalibrate + revalidate is a hard gate; no alias retarget goes live uncalibrated | recalibration mandate on every local-seat swap [XREF:S14]; 7.3 worker revalidation flags [XREF:S08] |
| P-T15-2 | Stack-capability drift — docs lie, probe behavior | G3·R16 ("stack-capability drift") →S12 | S12 §S12.2/§S12.10 | Behavioral conformance probes (`/v1` logprobs, `json_schema`, llama-swap contract) at every engine bump; documentation is never the check | conformance-registry entries [XREF:S14]; pins [XREF:S16] |

### S17.5 Named problems without P-* ids

| Item | Named by | Owner(s) | Disposition |
|---|---|---|---|
| **Context rot** | feature list §Known-problems; carried in S05's owned block | S05 §S05.3 (within-run) · S05 §S05.6 + S02 §S02.6 (across-pause) · S09 §S09.8 (memory hygiene) | No single mechanism claims it; each dimension has its named owner [R07 §7.8 as cited in S05]. Deliberately carries no P-* id — it is a feature-list-owned problem whose split disposition this row makes auditable. |
| **llama-swap bus-factor-1** | S12 §S12.2 (owned block, "referenced") | S12 §S12.2 | Accepted risk, not an open problem: pre-registered fallback ladder (router mode + kill-idle timer, or Ollama) + `components.lock` exit plan [XREF:S16]; its YAML/endpoint contract conformance-asserted on every bump [XREF:S14]. |

The feature list's remaining known-problem rows (deferred error correction, session amnesia, silent failures, quota-state opacity, provider drift, model deprecation, injection/exfiltration, auto-composed-worker validation, host availability, resource contention, state loss, write collisions, permission-config drift) each carry their owner in the feature list itself; verifying their spec coverage is S19's sweep [XREF:S19]. This register consolidates the research-phase `P-*` set that extends that list, per the feature list's own instruction that research keep hunting and assign owners.

### S17.6 Reconciliations — register findings and adopted readings

Sweep findings, each resolved by the reading the ratified record implies; none re-litigates a disposition.

- **R1 — P-T17-2 id restored.** G1 filed P-T17-1..3 [G1 §Follow-ups]; the drafts carry -1 and -3 but nowhere tag -2. Its disposition nonetheless survived intact and in substance: the `overflow_mode` onboarding attribute with reject-on-unprovable-disable is S03 §S03.6 text; the rejection rule and the visible currency flip are S10 §S10.2/§S10.10 text — exactly the R02 §9 remedy. The register therefore records P-T17-2 with owner S03 §S03.6 `[coordinator-draft]`; the coordinator adds the id tag to S03.6 at assembly (a tag, not a text change).
- **R2 — P-T02-2 owner adopted.** Dispositioned in S03 §S03.3 (the standing cache-fidelity alarm) but claimed by no owned block — S03 and S10 both list it "shared/referenced". Adopted owner: **S03 §S03.3** `[coordinator-draft]` — S03 hosts the countermeasure text and owns every other engine-behavior member of the T02 family except P-T02-3 (explicitly S02's); the alarm machinery and the P-T08-5 calibration remain S10/S14 registrations.
- **R3 — P-T04-3 ownership completed.** S05's owned block assigns it to S10 ("owned by S10"), but S10's block does not list it. The disposition substance is S10 text (tier-3 derived prompt units, per-lane normalization, approximation labels), so the assignment is correct and merely unreciprocated. Adopted owner: **S10 §S10.1/§S10.4** `[coordinator-draft]`.
- **R4 — P-T16-4 dual listing split.** Both S08 and S16 list it under *owned here*. The contents agree; the homes differ by role. Register reading: **S16 §S16.6 owns the binding rule** (no-public-registry-imports, an anti-harvest row), **S08 §S08.10 owns the enforcement** (the import battery in the composer/import path). Recorded as a deliberate split, mirroring P-T05-1 and P-T13-1.
- **R5 — descriptive filings ↔ ids.** Gate follow-up lists that filed problems descriptively map one-to-one onto drafting-coined ids, per each section's own coinage note: G1's "R06×4" = P-T03-1..4 (S04); G1's "R07×4" = P-T04-1..4 (S05, R07 §7.7 order); G2's "config-poisoning-as-escape-surface / allowlisted-egress-exfil" = P-T09-1/-2 (S11); G2's "engine-memory drift = 8.1 bypass / proposal-noise economics / knowledge-injection budget pressure" = P-T10-1/-2/-3 (S09, R11 §7.8–7.10 order); G3's "model-churn invalidates calibration / stack-capability drift" = P-T15-1/-2 (S12). S08's "config-poisoning… unnumbered in the G2 memo" note resolves to P-T09-1. No ambiguity remains; the drafting-coined id set matches `Research/STATE.md` exactly (P-T03-1..4, P-T04-1..4, P-T10-1..3, P-T15-1..2, P-T09-1..2).
- **R6 — deliberate dual homes (not defects).** P-T05-1 (S06 policy · S11 mechanics) and P-T13-1 (S01 detection/duty · S11 privileged path) are split by design; each side cross-references the other. P-T16-4 joins this class via R4.
- **R7 — family counts verified.** Register totals are counts of the corpus, not of assumed ranges: T01=4, T02=5, T03=4, T04=4, T05=4, T06=5, T07=5, T08=5, T09=2, T10=3, T11=5, T12=4, T13=3, T14=2, T15=2, T16=4, T17=3 — **64 ids**. The gate follow-up ranges match these counts everywhere; the only gap found anywhere was P-T17-2 (R1).

### S17.7 Register invariants

- **Zero orphans.** Every one of the 64 registered ids has an owning section and a disposition; three carry `[coordinator-draft]` ownership adoptions (R1–R3), none is undispositioned, and no *Open item* was needed for any id.
- **No coinage, no re-litigation.** This register introduces no new problem and modifies no disposition; where a disposition spans sections, the owner's text is normative and the other homes are registrations.
- **Registration completeness.** Every check named in a Registrations cell exists as declared machinery in its owning section — conformance-registry entries and watchdog checks in [XREF:S14], watch rows in [XREF:S16], settings in [XREF:S18]; this register adds pointers, never machinery.
- **Maintenance rule (post-G4).** A new `P-*` enters only through S00.9 amendment mechanics and must land as a row here in the same amendment — id, one-line problem, origin, owner, disposition, registrations. A `P-*` named anywhere in this spec without a row in this register is a spec defect. Id convention stays `P-T<topic>-<n>`; new problems in an existing topic take the next free `<n>`.

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| — none — | | | S17 introduces no settings; every ⚙ cited in a row is owned by its disposition's section and indexed by [XREF:S18] |

**Known problems owned here:** none — S17 owns no problems and no dispositions; it is the register. The three ownership adoptions (S17.6 R1–R3) assign owners in S03/S10, tagged `[coordinator-draft]` for G4.

**Deferred / parked:**

- Id-tag backfill into the owning drafts — P-T17-2 tag in S03.6; owned-block acknowledgments for P-T02-2 (S03) and P-T04-3 (S10) → **APPLIED 2026-07-19** by the coordinator, together with the S11.8↔S01.2 run-unit wording alignment; R4 needed no edit (the deliberate split was already recorded on both sides).
- Register upkeep → post-G4 S00.9 amendment mechanics (S17.7 maintenance rule); no standing schedule of its own.

**Coverage:**

| Scope input | Where consolidated |
|---|---|
| G1 §Follow-ups known-problems list (incl. "R06×4", "R07×4") | S17.2; S17.6-R5 |
| G2 §Follow-ups known-problems list (incl. T09/T10 descriptive filings) | S17.3; S17.6-R5 |
| G3 §Follow-ups known-problems additions (incl. R16's two descriptive) | S17.4; S17.6-R5 |
| Every S01–S16 "Known problems owned here" block + inline P-* dispositions | Owner + Disposition + Registrations columns, S17.2–S17.4 |
| Drafting-coined id list (`Research/STATE.md`) | S17.2–S17.4 rows marked `→S##`; S17.6-R5 |
| R02 §9 T17 problem statements | S17.2 (P-T17-1..3); S17.6-R1 |
| Feature list §"Known problems this program must solve" | S17.5 (context rot row + boundary note; coverage sweep [XREF:S19]) |
| Named non-id problems in owned blocks (context rot; llama-swap bus factor) | S17.5 |

**Open items for G4:** none. The four register findings that required a reading (R1–R4) are resolved with `[coordinator-draft]` tags and queued as assembly touch-ups — each is the only reading consistent with the ratified record, flagged for G4 attention rather than left open.

## S18 — Settings-registry index

**Scope:** The single consolidated index of every operator-editable ⚙ setting introduced in S01–S16 — one row per setting, grouped by domain prefix, deduplicated to its introducing section — plus the reconciliation log of naming/dedup decisions the sweep executed.
**Binding inputs:** G1 rider 1 (settings-not-constants) · G3 Def.2 (`settings.changed` event) · S01.10 (registry architecture — never restated here [XREF:S01]) · the ⚙ tables and section bodies of S01–S16 (the swept corpus) · `Research/decisions/GATE-{1,2,3}` for ratification cross-checks · BENCH-REG §1/§4.1 via [XREF:S14].

### S18.1 What this index is, and how to read it

This section is an **index, not an owner**: every setting below is normatively defined by its introducing section; a row here never overrides its owning row. The registry architecture — declare-once in code, JSON-Schema emission, `/etc/sinet` bootstrap vs DB-row overrides, `settings_events` + `settings.changed` audit on every write — is S01.10's [XREF:S01]; the editing surface is S15.9's [XREF:S15]. Per G1 rider 1, **every ⚙ number ships as an operator-editable setting with audit trail; automation may move *value* only within operator-set `(floor, ceiling)` bounds; auto-raises are visible on receipts** [G1 rider 1; S01.10].

Reading rules:

- **Dedup rule.** A setting introduced in one section and consumed in others appears **once**; owner = the introducing section (group heading). Cross-section consumers are noted in the row. Two source tables self-flagged rows for this sweep (S02's † freshness rows; S05's † `pressure.cache_read_weight`); their dispositions are S18.4 R3–R4.
- **Columns:** `⚙ key` (dotted, per owning domain) · `default` (with unit as ratified) · `clamp / range` (the registry `(floor, ceiling)` or enum; scope qualifiers as stated by the owner) · `R` = restart-required (✓; blank = live-apply — the S01.10 flag, badged in the UI [XREF:S15]) · `auto` = `val` where the owning section states automation moves the value within bounds (G1 rider 1); blank = operator-only writes · `ratified by` (provenance as carried by the owning table).
- **Map-valued keys** (`watchdog.silence_budget.<run_type>`, `local.alias.<duty>`) count as one entry each, as their owners count them.
- **Count:** **118 dotted registry keys** across **33 domains** and 16 owning sections (S18.5 tallies), plus **3 data-valued settings surfaces** with no dotted key (S18.3).

### S18.2 The index, by domain

**S01 — `shell.` (4)** [XREF:S01]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `shell.drain_grace` | 15 min | (1 min, 24 h) [coordinator-draft] | | | G1 Def.7 |
| `shell.watchdog_sec` | 30 s [coordinator-draft] | (10 s, 300 s) [coordinator-draft] | | | R17 §4.1 (⚙ flagged unnumbered) |
| `shell.inhibit_delay_max` | 30 s (host-measured; logind stock 5 s) | (5 s, 60 s) [coordinator-draft] | ✓ | | R17 §4.9; SPIKE P2-S2 |
| `shell.journal_max_use` | 4 GB [coordinator-draft] | (512 MB, 32 GB) [coordinator-draft] | ✓ | | R17 §4.8 (⚙ flagged unnumbered) |

**S02 — `state.` · `recovery.` · `effects.` · `claims.` · `freshness.` (14)** [XREF:S02]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `state.synchronous` | `FULL` | {FULL, NORMAL} | | | G2 Def.1 |
| `state.busy_timeout` | 5 s | ≥ 5 s | | | R08 §4.1 |
| `state.wal_truncate_interval` | 1 h | 5 min – 24 h | | | R08 §4.1 |
| `state.event_payload_cap` | 64 KB | 4 KB – 1 MB | | | R08 §4.1 / P-T07-5 |
| `recovery.heartbeat` | 60 s | 15 s – 5 min | | | G2 Def.2 |
| `recovery.dead_after` | 5 min | ≥ 2× heartbeat | | | G2 Def.2 (seeds `watchdog.silence_budget.*` [XREF:S14]) |
| `recovery.wake_grace` | 120 s | 30 s – 10 min | | | G2 Def.2 |
| `recovery.max_attempts` | 3 | 1 – 5 | | | G2 Def.2 |
| `recovery.stale_finalize` | 24 h | ≥ 1 h | | | G2 Def.2 |
| `recovery.sweep_interval` | 5 min | 1 – 30 min | | | R08 §4.4 |
| `effects.approval_expiry` | 7 d | 1 h – 30 d | | | G2 Def.2 / R08 §4.5 |
| `claims.default_write` | whole-project | {declared-set, whole-project} | | | G2 Def.3 |
| `freshness.max_age` | 24 h | ≥ 1 h | | | G1 Def.5; feature 4.3 — cross-cutting; consumers S03/S06/S10; **alias folded: `intake.approval_stale_hours` (S06 ⚙ table)** — S18.4 R2 [coordinator-draft] |
| `freshness.hold_vs_park` | 10 min | 1 – 60 min | | | G1 Def.4 — cross-cutting; consumer S10 (cache-cliff resume timing) |

**S03 — `adapter.` (3)** [XREF:S03] — engine version pins are deliberately *not* rows: manifest home + audit trail = `components.lock` (S18.4 R9) [XREF:S16]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `adapter.claude.cleanup_period_days` | 365 d | integer ≥ 30; MUST exceed ask-expiry (7 d, G2 Def.2) | | | P-T02-1 mitigation; SPIKE G1-S2 F5 |
| `adapter.engine_ceiling_backstop_mult` | 2 | ≥ 2.0 | | | SPIKE G1-S2 F4 — consumer S10 (ceiling ordering) |
| `adapter.parallel_gate_fallback` | `serialize-by-deny` | {serialize-by-deny, hold-process} | | | SPIKE G1-S2 §Verdict; P2-S4 headline; [coordinator-draft] |

**S04 — `orchestration.` (9)** [XREF:S04] — the registry stores `(value, floor, ceiling)` for every row per S04.4; auto-movement is stated for the three marked rows

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `orchestration.depth_cap` | 2 | 0–2 at v0; >2 = deliberate operator ceiling change | | | D6; G1 Def.8 |
| `orchestration.max_concurrent_helpers` | 4 per task (all depths) | 0–ceiling | | val | G1 Def.8 + rider 1 |
| `orchestration.helper_turns` | 20 per helper | 1–ceiling | | val | G1 Def.8 + rider 1 |
| `orchestration.helper_tokens` | 80,000 per helper | within ceilings | | val | G1 Def.8 + rider 1 |
| `orchestration.spawn_budget` | 8 per task (incl. sub-helpers + retries) | overrun only via operator-visible gate | | | G1 Def.8; R06 §4.3 |
| `orchestration.report_tokens` | 2,000 per report | screen-enforced (S04.5) | | | G1 Def.8 |
| `orchestration.bulk_offload_tokens` | 20,000 | — | | | R06 §4.2; G1 D1.1 |
| `orchestration.helper_retry_limit` | 1 | 0–ceiling | | | R06 §4.4; G1 D1.1 |
| `orchestration.stagger_identical_prefix` | on | {on, off} | | | R06 §4.2; G1 D1.1 |

**S05 — `context.` (4)** [XREF:S05] — S05's fifth table row, `pressure.cache_read_weight` †, is owned by S10 (S18.4 R3)

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `context.stage_fit_target` | 0.50 of lane window | 0.20–0.70; < overflow threshold [coordinator-draft clamp] | | val | G1 Def.11 (per-generation recalibration, S05.2) |
| `context.stage_overflow_threshold` | 0.70 of lane window | (target+0.05)–0.85 [coordinator-draft clamp] | | val | G1 Def.11 (same recalibration) |
| `context.conventions_max_lines` | 150 | 50–400 [coordinator-draft clamp] | | | R07 §4.4 |
| `context.recitation_interval_turns` | 10 | 5–50; 0 = off | | | [coordinator-draft] |

**S06 — `intake.` (7)** [XREF:S06] — an eighth table row, `intake.approval_stale_hours`, is folded into `freshness.max_age` (S18.4 R2)

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `intake.zero_interaction_cost_usd` | $0.50 API-equivalent | ≥ 0; per-user | | | G1 P11 + D1.7 (⚙ per-user at the gate) |
| `intake.clearance_floor.low` | 60 | 0–100 | | | G1 P8 mechanism; default [coordinator-draft] |
| `intake.clearance_floor.standard` | 75 | 0–100 | | | G1 P8 mechanism; default [coordinator-draft] |
| `intake.clearance_floor.high` | 90 | 0–100 | | | G1 P8 mechanism; default [coordinator-draft] |
| `intake.size_recheck_factor` | 2.0× | > 1.0 | | | feature 2.5; R03 §4 Stage 2(c); default [coordinator-draft] |
| `intake.coverage_autofix_rounds` | 1 | 0–2 | | | R03 §4 Stage 2(a) ("bounded") |
| `intake.critique_revise_rounds` | 1 | 0–2 | | | R03 §4 Stage 3 ("fixes once") |

**S07 — `verification.` (11)** [XREF:S07] — the SLA cadence rows are inbox-wide: consumers S15 (inbox display/re-nag) + S14 (notifier delivery); naming resolved at S18.4 R1

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `verification.rework_rounds` | 3 | 0–ceiling | | | R04 §4 via G1 D1.1(d); S07 resolution note stands |
| `verification.convergence_patience_rounds` | 2 | 1–`rework_rounds` | | | R04 §2.6/§3-D via G1 D1.1(d) |
| `verification.sanity_stakes_floor` | standard | tier enum (S06.4) | | | G1 D1.2 mechanism; default [coordinator-draft] |
| `verification.entailment_sample_rate` | TBD-BRINGUP → derived 0.20 (A8, 2026-07-23); live write pending bring-up | 0–1 | | | G1 Def.2; G3 Def.4/Def.8 |
| `verification.research_rerun_limit` | 1 | 0–2 | | | feature 1.9; R03 §4 Stage 2(d) via S06.6 |
| `verification.check_audit_interval_days` | 90 d | > 0 | | | P-T06-1; R04 §4; default [coordinator-draft] |
| `verification.canary_interval_hours` | 24 h | > 0 | | | G1 Def.3 (superseded set: "canary daily") — dead-man escalation canary; see S18.4 R5 |
| `verification.drill_interval_days` | 90 d | > 0 | | | G1 Def.3 (superseded set: "drill quarterly"); see S18.4 R6 |
| `verification.card_remind_hours` | 4 h | > 0 | | | G1 Def.3 (superseded set) — inbox-wide |
| `verification.card_push_hours` | 24 h | > 0 | | | G1 Def.3 (superseded set) — inbox-wide |
| `verification.safety_reping_hours` | 1 h | > 0 | | | G1 Def.3 (superseded set) — inbox-wide |

**S08 — `workers.` (4)** [XREF:S08]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `workers.first_n` | 3 | integer ≥ 1; count-based; resets on body/equipment version change | | | G3 D3.4 |
| `workers.gap_proposal_count` | 2 | integer ≥ 2 | | | G3 D3.4 ("second occurrence") |
| `workers.persona_lines_max` | 2 | integer ≥ 0; station-1 lint warn threshold | | | R15 §4.2 (⚙ there) |
| `workers.dryrun_cost_cap_usd` | $0.50 API-equivalent | > 0 (D5 currency) | | | R15 §4.3 (⚙ there); anchor to G1 D1.7 [coordinator-draft] |

**S09 — `memory.` (12)** [XREF:S09]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `memory.l1_ttl_days` | 90 d | 7–365 | | | G2 Def.7 (L1 scope activates v1) |
| `memory.reverify_lessons_days` | 90 d | 30–365 | | | G2 Def.7 |
| `memory.reverify_house_days` | 180 d (6 mo) | 90–730 | | | G2 Def.7 |
| `memory.proposals_per_task_max` | 2 | 0–5 | | | G2 Def.7 |
| `memory.digest_interval_days` | 7 d | 1–30 | | | G2 Def.7 |
| `memory.distill_threshold_lessons` | 3 | 2–10 | | | G2 Def.7 |
| `memory.injection_budget_tokens.house` | 2,000 tok | 500–10,000 | | | budgets G2 Def.7; numbers [coordinator-draft] |
| `memory.injection_budget_tokens.project` | 3,000 tok | 500–10,000 | | | as above |
| `memory.injection_budget_tokens.user` | 1,500 tok | 500–10,000 | | | as above |
| `memory.injection_budget_tokens.worker_overlay` | 1,500 tok | 500–10,000; scope activates v1 | | | as above |
| `memory.vector_gate.task_miss_rate` | 0.05 | 0.01–0.25; pre-registered trigger, evaluated post-15.3 | | | G2 Def.8 |
| `memory.vector_gate.corpus_entries` | 5,000 | 1,000–50,000; pre-registered trigger | | | G2 Def.8 |

**S10 — `pressure.` · `budget.` · `meter.` · `limit.` · `scheduler.` · `arbitration.` (9)** [XREF:S10]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `pressure.cache_read_weight` | 0.1× | 0–1; labeled "assumed" until provider publishes quota semantics | | | G1 Def.10 — consumer S05 (weighting display); dedup S18.4 R3 |
| `pressure.bg_admit_stop` | 0.7 | 0.1 – 0.95 | | | G2 Def.4 |
| `budget.background_window_fraction` | 0.5 | 0 – 1; labeled "assumed" | | | G2 Def.4/5 |
| `meter.value_divergence_alarm` | 20 % | 5 – 100 % | | | R09 §4.1 / P-T08-1 |
| `limit.retry_cap` | 3 | 1 – 5 | | | G2 D2.1 / R09 §4.3 |
| `limit.retry_budget_ratio` | 0.10 | 0.01 – 0.5 | | | G2 D2.1 / R09 §4.3 |
| `limit.probe_interval_max` | 30 min | 1 – 120 min | | | G2 D2.1 / R09 §4.3 |
| `scheduler.missed_slot_default` | run-once-late | {run-once-late, skip, notify-only}; ships v0, consumer surface v1 | | | R09 §4.7 |
| `arbitration.background_cpuweight` | idle | systemd CPUWeight; slice mechanism [XREF:S01] | | | R09 §4.9 |

**S11 — `sandbox.` (4)** [XREF:S11] — seccomp-BPF profile, Landlock ruleset, per-class profile defaults are structural, not ⚙ (S18.4 R9)

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `sandbox.egress_deny_cidrs` | {169.254.169.254/32, 169.254.0.0/16, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7} | editable list; metadata IP is a non-removable floor [coordinator-draft] | | | R10 §2.3/§3.3 |
| `sandbox.block_outbound_doh` | true | {true, false}; false requires a recorded reason | | | R10 §2.3 |
| `sandbox.c2_registry_allowlist` | curated npm/pypi/crates/apt/go/maven/nuget/rubygems + CDN hosts (Copilot/Codex preset) | editable list (data, like the price table) | | | R10 §3.3 |
| `sandbox.model_egress_tls_terminate` | true (per lane) | {true, false}; false ⇒ pattern-2 scoped-egress fallback for that lane | | | SPIKE P2-S3 / R10 §3.4 |

**S12 — `local.` (11)** [XREF:S12]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `local.ttl.fast_s` | 120 s | 0–3600 [coordinator-draft clamp] | | | R16 §4.1 |
| `local.ttl.workhorse_s` | 300 s | 0–7200 [coordinator-draft clamp] | | | R16 §4.1 |
| `local.ttl.egpu_s` | 1800 s | 0–86400; dormant until pool24 | | | R16 §4.1 |
| `local.vram.guard_band_mb` | 512 MB | ≥ 128 [coordinator-draft clamp] | | | R16 §4.4 step 5 |
| `local.unload.term_grace_s` | 5 s | 1–30 [coordinator-draft clamp] | | | R16 §4.5 |
| `local.gamemode_hook` | on | {on, off} | | | G2 Def.6 |
| `local.battery.gpu_admission` | urgent-only | {never, urgent-only, always}; name [coordinator-draft] | | | R16 §4.6 |
| `local.batch.ac_only` | true | {true, false} | | | R16 §4.6 |
| `local.broker.sandbox_logprobs` | off | {off, on}; per-template | | | R16 §4.7 |
| `local.reeval.cadence_months` | 6 mo | 1–12 | | | R16 §4.10 |
| `local.alias.<duty>` (map) | per S12.4 table; workhorse = Qwen3.5-9B | changes only via the S12.10 swap gate | | | R16 §4.7; G3 Def.8 |

**S13 — `review.` · `backup.` · `preview.` (7)** [XREF:S13]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `review.anchor_drift_lines` | ±2 lines | 0 – 10 [coordinator-draft clamp] | | | FC-v1 §2 (carried N15 behavior) |
| `backup.interval` | 24 h (daily) | 6 h – 7 d [coordinator-draft clamp] | | | G2 D2.5 |
| `backup.keep` | 30 snapshots | 7 – 365 [coordinator-draft clamp] | | | G2 D2.5 |
| `backup.repo_rotation` | 12 mo | 6 – 24 mo [coordinator-draft clamp] | | | G2 D2.5 (annual) |
| `backup.drill_interval` | 3 mo [coordinator-draft] | 1 – 12 mo | | | R13 §4.9 (⚙ flagged unnumbered); see S18.4 R6 |
| `preview.idle_stop` | 15 min [coordinator-draft] | 1 min – 24 h | | | R13 §4.8 (⚙ flagged unnumbered) |
| `preview.max_concurrent` | 3 [coordinator-draft] | 1 – 10 | | | feature 3.11 posture [coordinator-draft] |

**S14 — `obs.` · `watchdog.` · `watchlist.` · `canary.` · `retention.` · `eval.` · `benchmark.` (15)** [XREF:S14]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `obs.sse_keepalive` | 20 s | 15–30 s | | | R12 §4.3 |
| `watchdog.loop_repeat` | 5× | 3–10 | | | G2 Def.10 |
| `watchdog.pingpong_cycles` | 6 cycles | 3–12 | | | G2 Def.10 |
| `watchdog.error_loop` | 3× | 2–10 | | | G2 Def.10 |
| `watchdog.silence_budget.<run_type>` (map) | seed = `recovery.dead_after` (5 min) | ≥ 1 min; per-type; TBD-BRINGUP(per-run-type calibration) | | | G2 Def.10 |
| `watchdog.spend_median_mult` | 3× trailing-14-d median | ≥ 1.5× | | | G2 Def.10 |
| `watchdog.spend_arm_days` | 14 d | ≥ 7 d | | | G2 Def.10 |
| `watchdog.flag_now_target` | 2/day | 1–10 | | | G2 Def.10 |
| `watchdog.suppress_retune_count` | 2 | 1–5; retune lands as a proposal card, never an auto-move | | | G2 Def.10 |
| `watchlist.fetch_fail_streak` | 3 cycles | 2–10 | | | R12 §4.5 / P-T11-3 |
| `canary.auth_interval` | daily | 4/day – weekly | | | G1 Def.3 set; P-T17-1 — auth-revocation canary; see S18.4 R5 |
| `canary.behavioral_interval` | weekly | daily – monthly | | | R12 §4.5 [coordinator-draft] |
| `retention.compaction_horizon` | 6 months | ≥ 1 month; per-user | | | G2 Def.11; feature 13.4 |
| `eval.sweep_interval` | quarterly | monthly – semi-annual | | | R12 §4.7 |
| `benchmark.sampling_rate` | BENCH-REG §4.1 schedule (100% pre-gate / 25% maintenance) | 0–100%; uniformity frozen | | | BENCH-REG §4.1 — registered-number rule applies (S18.4 R9) |

**S15 — `frontend.` (1)** [XREF:S15]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `frontend.dependency_pass_interval` | 90 d (quarterly) | (30 d, 365 d) [coordinator-draft] | | | G3 D3.3 rider; R17 §4.3; see S18.4 R7 |

**S16 — `adoption.` (3)** [XREF:S16]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `adoption.dependency_pass_months` | 3 mo (quarterly) | (1, 6) [coordinator-draft] | | | G3 D3.3 rider; R17 §4.8; see S18.4 R7 |
| `adoption.abandonment_months` | 6 mo [coordinator-draft] | (2, 24) [coordinator-draft]; per-entry overridable | | | R17 §4.8 (⚙ flagged unnumbered); P-T16-1 |
| `adoption.price_data_stale_days` | 60 d | (14, 180) [coordinator-draft] | | | R14 §4.2 |

### S18.3 Data-valued settings surfaces (13.4-editable, no dotted key)

Three operator-editable surfaces are **tables/flags, not single registry keys** — their owners declare them settings ("data, not a single default") and they follow the same audit pattern as registry writes [S01.10; XREF:S10]:

| Surface | Shape | Owner | Ratified by |
|---|---|---|---|
| **Price table** | effective-dated per-model rows; vendored genai-prices defaults; user edits overlay, refresh-as-proposal | S10.3 | D5; G2 Def.16 |
| **Per-person automation budgets** | per (person, lane, period); seeded from plan marketing shape, labeled "assumed" | S10.4 | G2 Def.5 |
| **Per-model attributes** | flat/metered flag (per user, feature 3.1); `overflow_mode` {hard-stop, opt-in-credits, auto-metered}; `region_model_gate` observed-vs-config diff state | S10.2 (flag); S03.6 (attribute pair) | D5; R02 §4; P-T17-3 |

These carry no dotted names by design (row-keyed data); the settings UI surfaces all three [XREF:S15]. No index rows are minted for them.

### S18.4 Reconciliation log (decisions this sweep executed)

- **R1 — Inbox SLA naming seam (the S07-recorded duty): RESOLVED, no rename.** S07 registers the escalation SLA cadences under `verification.*` (card remind 4 h / push 24 h / safety re-ping 1 h, with the daily canary and quarterly drill from the same G1 Def.3 superseded set) and says "the S18 sweep reconciles naming"; S15 re-states the set with "cadence ⚙ and delivery owned by the notifier [XREF:S14]". No competing dotted name exists anywhere in the corpus, and the contract binds a key's prefix to its introducing section's domain — so the canonical names stay `verification.card_remind_hours` / `verification.card_push_hours` / `verification.safety_reping_hours`, owner S07, with **scope: inbox-wide** carried on the rows; S15's phrase is read as *delivery* ownership (the notifier), not registry ownership [coordinator-draft].
- **R2 — `intake.approval_stale_hours` ≡ `freshness.max_age`: FOLDED (one setting, alias recorded).** Same concept (the feature-4.3 "assumptions may be stale" horizon), same default (24 h), same ratification (G1 Def.5) at two enforcement points — S02.6's parked-run freshness pass and S06.9's pending-card flag. S02's own † footnote calls the setting cross-cutting and "enforced via S06", and both sections delegated dedup to this sweep. Canonical key: `freshness.max_age`, owner S02; `intake.approval_stale_hours` (S06 ⚙ table) is recorded as its alias and mints no separate key [coordinator-draft]. If G4 wants the two enforcement points independently tunable, the alias becomes a second key — flagged for that attention.
- **R3 — `pressure.cache_read_weight` dual listing: deduped to S10** as S05's own † directs ("owned by the pressure gauge"). One row, owner S10, consumer S05.
- **R4 — `freshness.*` rows listed in both S02 (†) and S10's consumed-elsewhere note: owner S02** (the freshness-pass and fingerprint mechanics live in S02.6); consumers noted on the rows. Executes the dedup both tables requested.
- **R5 — Two canaries, not one.** `verification.canary_interval_hours` (S07: dead-man escalation-path canary, alert-on-silence) and `canary.auth_interval` (S14: provider auth-revocation canary, P-T17-1) both default to "daily" and both cite G1 Def.3's close set — distinct duties, deliberately separate keys; no merge. `canary.behavioral_interval` (weekly) is a third, unrelated cadence.
- **R6 — Two drills, not one.** `verification.drill_interval_days` (G1 Def.3 quarterly escalation drill) vs `backup.drill_interval` (R13 §4.9 restore drill). Distinct; no merge.
- **R7 — Dependency-pass siblings, not duplicates.** `frontend.dependency_pass_interval` = 90 d (S15) and `adoption.dependency_pass_months` = 3 mo (S16) both cite the G3 D3.3 rider and both mean "quarterly"; S15 itself says the passes SHOULD be co-scheduled with S16's manifest review. Two deliberate keys for two activities (SPA dependency bumps vs `components.lock` review); the day-vs-month unit heterogeneity is noted as cosmetic.
- **R8 — Settings-shaped inline items that are NOT registry keys.** S10's consumed-elsewhere list names per-run `runs.ceilings` [S02.2] and `RuntimeMaxSec` [S01.2]: `runs.ceilings` is a per-run column set (time/steps/cost, feature 3.7) seeded at dispatch, and `RuntimeMaxSec` is the transient-unit directive carrying the run's time ceiling — per-run enforcement data, not operator dials; no rows minted. This disposition completes S10's dedup request.
- **R9 — Deliberate non-rows, confirmed by their owners.** Engine version pins (`claude` 2.1.214, `opencode-ai@1.18.3`) — operator-editable via the S03.3 deliberate-bump procedure, manifest home `components.lock` [XREF:S16]. Sandbox seccomp/Landlock/per-class profiles — structural, versioned, not dials [S11]. Benchmark frozen values (gate limbs, alarm threshold, done-directly minimum, labels) — surface in the registry marked **"registered — changing this value requires a re-registration commit"** [BENCH-REG §1; S14]; only `benchmark.sampling_rate` is a live dial.
- **R10 — Naming observations (cosmetic; no renames proposed).** Unit-suffix style is heterogeneous (`_s`/`_mb`/`_days`/`_hours`/`_months`/`_usd` in `local.*`, `memory.*`, `verification.*`, vs unitless keys elsewhere with units in the default) — the registry's typed declarations [S01.10] make this cosmetic. Multi-level keys (`adapter.claude.*`, `intake.clearance_floor.*`, `memory.injection_budget_tokens.*`, `memory.vector_gate.*`, `local.ttl.*`) and the two map-valued families are consistent with declare-once. Domain→owner is many-to-one for S02, S10, S13, S14; no domain prefix straddles two owners — the CONTRACT prefix rule holds corpus-wide.

### S18.5 Exhaustiveness

Per-owner key counts: S01 4 · S02 14 · S03 3 · S04 9 · S05 4 · S06 7 · S07 11 · S08 4 · S09 12 · S10 9 · S11 4 · S12 11 · S13 7 · S14 15 · S15 1 · S16 3 = **118**.

Per-domain: shell 4 · state 4 · recovery 6 · effects 1 · claims 1 · freshness 2 · adapter 3 · orchestration 9 · context 4 · intake 7 · verification 11 · workers 4 · memory 12 · pressure 2 · budget 1 · meter 1 · limit 3 · scheduler 1 · arbitration 1 · sandbox 4 · local 11 · review 1 · backup 4 · preview 2 · obs 1 · watchdog 8 · watchlist 1 · canary 2 · retention 1 · eval 1 · benchmark 1 · frontend 1 · adoption 3 (33 domains).

**Sections with no ⚙:** none — every committed section S01–S16 introduces at least one; S00 introduces none by design. S17 and S19 are not yet drafted and are expected to introduce none (register and boundary sections); any section amendment before assembly that touches a ⚙ table or adds an inline ⚙ re-runs this sweep as part of the S00.9 assembly step [coordinator-draft].

**Dormant-at-v0 keys ship in the registry with their ratified defaults** (G1 rider 1 applies from day one): `scheduler.missed_slot_default` (consumer surface v1), `local.ttl.egpu_s` (pool24), `memory.injection_budget_tokens.worker_overlay` (L1/overlay scope v1), `memory.vector_gate.*` (trigger evaluated post-15.3).

---

**Settings introduced (⚙):** none — this section is the index; every entry above is owned by its introducing section.

**Known problems owned here:** none — P-* ownership stays with the owning sections (P-* ids appearing in provenance cells are citations, not ownership); S17 consolidates the register.

**Deferred / parked:**

- Dormant-key activations (S18.5 list) → triggers: v1 schedule surface [XREF:S10/S19]; pool24 [XREF:S12]; v1 memory scopes [XREF:S09]; post-15.3 vector gate [XREF:S09].
- Independent tunability of the two R2 enforcement points (parked-run vs pending-card staleness) → re-entry: G4 objection to the fold, or operator demand for split thresholds post-bring-up.
- Re-sweep on amendment → the S00.9 assembly step (S18.5).

**Coverage:**

| Feature-list item | Where |
|---|---|
| 13.4 every number an operator-editable setting — the consolidated index | S18.2 (dotted keys), S18.3 (data surfaces) |
| G1 rider 1 mechanics visible in one place (bounds, auto-movement, audit) | S18.1 legend; `auto` column; [XREF:S01] |
| 4.3 staleness threshold single-sourced | S18.4 R2 |
| Sweep exhaustiveness (provably complete over S01–S16) | S18.5 |

**Open items for G4:** none. Sweep resolutions tagged [coordinator-draft] for G4 attention: R1 (keep `verification.*` names for the inbox-wide SLA set), R2 (fold `intake.approval_stale_hours` into `freshness.max_age`, alias recorded, split path parked), and the assembly re-sweep rule (S18.5).

## S19 — v0 boundary, coverage proof & build order

**Scope:** The closing meta-section. Fixes the v0 shipping boundary (what ships / what is socketed for later), proves every in-scope feature-list item maps to an owning section, dispositions the items that carry no direct coverage row, consolidates the whole parked-forward register by re-entry trigger, and hands P3 a build order plus a bring-up measurement sequence. Authored by the coordinator; carries no new normative design — it indexes and sequences what S01–S18 already decided.
**Binding inputs:** feature list §15.1 / §15.6 (v0 scope) + §15.2 / §15.4 / §15.5 (later phases) · S00.2 (boundary statement) · S00.8 (§15 → S19 mapping) · the Coverage / Deferred-parked / Open-items blocks of every section S01–S18 · S17 (known-problems register) · S18 (settings index) · G3 D3.1 (P2 approved; research phase ends only at G4) · the Binding-inputs dependency adjacency of S01–S16.

### S19.1 The v0 boundary

**v0 ships (feature list §15.1 + §15.6), single-user (operator only):**

- Software development end-to-end: intake → specification → plan → execution → verification → review → accept → receipt [XREF:S06, S07, S08, S13].
- Consumption/metering with the two-currency rule and honest receipts including the done-directly figure [XREF:S10; benchmark formula BENCH-REG §13].
- Checkpoint-and-gate invariant (4.8 / D7); durable state and recovery [XREF:S02].
- Confinement classes **C0–C2**, the credential broker, the sandbox stack [XREF:S11].
- Approval inbox with risk tiers; phone notifications [XREF:S15].
- Local-models tier as the permanent free tier — ceremony, watchdogs, and background intelligence run local [XREF:S12].
- The run scheduler (parks, resumes, priorities, budgets) — required for limit events even single-user [XREF:S10].
- The web workspace (mission control, board, task detail, fleet, review surfaces, settings, chat, workforce-map view) [XREF:S15].
- The observability spine: event log, full trace, watchdog suite, conformance registry; the benchmark **machinery** exists at v0 though the gate governs expansion [XREF:S14].
- The data model is **multi-user from day one** (15.6) — every record carries its owner even while operation is single-user [XREF:S01, S02, S08, S09].

**Socketed for later (the architecture leaves the seam; nothing is built):** v0.1 web-research domain + confinement class C3; the 15.3 benchmark **gate execution**; v1 household onboarding, degraded-mode domains, task templates, user-facing schedules/triggers, the memory auto-proposal pipeline; 15.5 multi-channel ingress, workforce-map editing, web-acting C4, and the voice/avatar satellites. Each socket is a named seam [XREF:S01.3 five seams], not a stub — the consolidated re-entry register is S19.4.

The v0 line is deliberately drawn at **one full pipeline proven end-to-end** before breadth: 14.5's "no decorative breadth before the benchmark gate" is the binding rule, and 15.3 is the named unlock for the parked set.

### S19.2 Coverage proof — every in-scope item has an owner

Mechanical sweep of the 18 Coverage blocks against the 182-item feature-list inventory: **166 of 182 items** carry a direct coverage row; the remainder are dispositioned below. No in-scope item is unowned.

**The nine items with no direct coverage row — each covered in substance, by design, not by omission** [coordinator-draft; G4 confirms]:

| Item | Disposition | Realized by |
|---|---|---|
| 2.2 all task shapes | **Split by phase.** v0 covers one-shot / iterative / long-running shapes (the pipeline itself); scheduled + event-triggered shapes are the v1 boundary via 2.8 | pipeline [XREF:S06, S02]; triggered shapes → S19.3 (v1) |
| 10.2 fair attribution per person | **Principle realized across sections** — every run billed to its requester, every record owner-stamped, cost visible at every altitude | [XREF:S10.1 billing; S01.9/S02.2 owner columns; S10.10/S14.10 altitudes] |
| 13.1 handles everything automatically per best practice | **The pipeline principle itself** — intake → routing → orchestration → verification, guided by versioned best-practice playbooks | [XREF:S06, S08, S04, S07; composer playbook S08.6] |
| 14.3 no silent metered billing / provider switching | **Two-half realization** — metered spend is opt-in only, and model/lane choice is always visible and logged (never silent) | [XREF:S10.2 opt-in; S16.6 no-metered-paths; S12 visible duty aliases; S08.8/S14.2 accountable routing] |
| S1.1 projects as living containers | **Realized across the project stack** — registered project/repo entries, inherited context, visible follow-up lineage | [XREF:S13.7 registry; S05.4 + S09.6 inherited context; S13.9 lineage] |
| S3.4 task board + detail daily driver | **Pointer item** — it names S1.3–S1.7, which are owned | [XREF:S15.5] |
| OR-3 quota-state opacity | **= KP-6** — providers expose no remaining-window state; Sinet self-meters | [XREF:S10 measure-not-model; S03 limit taxonomy] |
| OR-4 users range operator → no-IT | **Realized in the intake + help surfaces** — plain-language intake, the Clearance indicator, registry-sourced approval help text | [XREF:S06 intake; S01.10/S06.9/S15.6 help text] |
| OR-6 human control as a product requirement | **The gating spine** — outward effects gate as proposals, the approval inbox is the single human-decision surface, the human is the final accept gate | [XREF:S02 D7 gating; S15.6 inbox; S07 human-final-gate; S08 D10] |

**Known-problems coverage (KP-1..14 — S17 defers this sweep to S19):** every feature-list known-problem maps to the section(s) that own its countermeasure — KP-1 deferred-error-correction → S07/S02; KP-2 permission-config-replaces-prompts → S11/S08/S15; KP-3 session-amnesia → S02/S05; KP-4 context-rot → S05/S09/S02; KP-5 silent-failures → S14/S01/S02; KP-6 quota-opacity → S10/S03; KP-7 provider-drift → S03/S14/S16; KP-8 model-deprecation-vs-stored-workers → S08/S16/S12; KP-9 injection/exfiltration → S11/S04/S06/S07 (the confinement + verification + intake-hardening stack); KP-10 validating-composed-workers → S08; KP-11 host-availability → S01/S10/S02; KP-12 local-contention → S12/S10; KP-13 disk-death → S13/S02; KP-14 parallel-write-collisions → S02/S13. The research-phase `P-*` set that extends this list is the S17 register [XREF:S17].

**Boundary note (2.8):** S10.7 owns the missed-slot mechanism at v0 (`scheduler.missed_slot_default` ships in the registry) but the user-facing schedule/trigger surface is v1 — that boundary is drawn here and in S19.3, discharging S10's `[XREF:S19]` marker.

### S19.3 Items the feature list itself defers beyond v0

Not gaps — phased by the list's own text; the architecture leaves each a seam:

- **10.3 multi-channel ingress** → 15.5 (a bot channel is just another API client at the S15.2 seam) [XREF:S15.13].
- **12.3 ambient repository upkeep**, **12.6 supervised self-tuning**, and the rest of **Section 12** (image gen, web-acting workers, template sharing, voice/vision satellites, multi-host growth, satellite runners) → 15.5, behind the 15.3 gate [XREF:S11, S12, S15 parked blocks].
- **S1.5 task templates** → v1 (§15.4 names them) [XREF:S00.2].
- **§15.2 / §15.4 / §15.5** are phase declarations, not buildable items; S19.1 + S19.4 are their spec home per S00.8.

### S19.4 Consolidated parked-forward register (by re-entry trigger)

The union of all 18 Deferred/parked blocks — **104 unique entries** — grouped by what re-opens each. Full per-item detail (mechanism, exact trigger) lives in each owning section's Deferred/parked block; this is the trigger-level index. Written at boundary altitude by design.

- **Behind the 15.3 benchmark gate (~8):** avatar/memory-galaxy backdrop, conversation mode + voice satellites (12.4 framing), decorative-breadth freeze, script-driven mass fan-out lane, local image-gen awareness [XREF:S15, S04, S12].
- **v0.1 — web-research domain + class C3 (~7):** the C3 web-reading confinement class (ladder already specified), research-domain verification pack + entailment activation, per-family question sets for research, the blindness-calibration pilot, PDF pixel-overlay if it becomes routine [XREF:S11, S07, S06, S14, S13].
- **v1 — household (~12):** passkeys, template signing, the memory auto-proposal pipeline (architecture fixed, activation is config + UI), degraded-mode domains, task templates, user-facing schedules/triggers (2.8), high-stakes comprehension friction, worker-overlay memory injection, `WakeSystem=` host-waking schedules [XREF:S01, S06, S08, S09].
- **15.5 — later (~9):** Section 12 items, multi-channel ingress, workforce-map editing, the web-acting C4 confinement class [XREF:S11, S12, S15].
- **Watchlist — adopt-when-earned (~30):** each is a pre-planned substitution at a named seam with its own trigger — persistence (Litestream, jj, DBOS, Postgres-at-multi-host), transport/protocol (MCP Tasks, OTel projector, HTMX/Datastar re-entry, `@dnd-kit` 1.0, RJSF, monaco), tooling (Miniflux backend, DeepEval, DuckDB, OmniRoute router, diffity, vLLM), and provider/lane additions (post-v0 lanes, metered-exception designate). Full list + triggers in the S02/S13/S14/S15/S16 blocks and the S16 manifest [XREF:S16].
- **Bring-up measurements + P3 spikes (~15):** sequenced in S19.5–S19.6.
- **Register-mechanics (S17/S18):** post-G4 upkeep via S00.9 amendment; dormant-key activations tied to their phase triggers [XREF:S17, S18].

No parked item lacks an owner or a trigger. One gate-parked item — **GitHub Pro for a second member** [G3 §Follow-ups] — lives only in S13's known-problems block (P-T12-1), not a Deferred block; recorded here so the gate list reconciles [coordinator-draft].

### S19.5 P3 build order

The principle is **walking skeleton first**: stand up a thin intake → execute → checkpoint → receipt path end-to-end, then thicken each stage. Ordering follows the Binding-inputs dependency adjacency — S01, S02, and S06 are foundational (every other section depends on one of them; they depend on none).

- **B0 — Spine.** `sinet-control` process + `platform.db` + the event log + settings registry + auth stack + the five seams [S01]; durable state, run FSM, checkpoint/effect journal, recovery ladder [S02]. Adoption manifest + CI lock-gate stood up now and enforced from the first dependency [S16].
- **B1 — Execution substrate.** One adapter + one lane behind the D3 contract [S03]; metering ledger + limit taxonomy + scheduler skeleton [S10]; the per-run sandbox stack (C0–C2) and the credential broker — runs are confined before they touch real work [S11].
- **B2 — The pipeline.** Intake → spec → plan [S06]; the context ledger + fresh-context-per-stage [S05]; verification two-axis + findings→retry drain + tested escalation [S07]. This closes the thin end-to-end path.
- **B3 — Workforce & memory.** Worker registry + composer battery + routing [S08]; memory/knowledge layers with the write gate [S09].
- **B4 — Deliverables & local tier.** Deliverable/revision/comment schema, review behavior, git topology + broker-mediated accept, previews, encrypted-snapshot backup [S13]; the local-model serving stack + GPU broker + duty aliases [S12].
- **B5 — Observability & evals.** Watchdog suite, conformance registry, the benchmark machinery, queryable-history layers [S14] — much of it registers checks defined in earlier phases, so it lands once those exist.
- **B6 — Frontend.** The React SPA consumes every API built above; it is last because it depends on S01/S13/S14/S10/S06 [S15].

TBD-P3 implementation spikes attach to their phase: serialize-by-deny reconfirm + PreCompact/injection-mechanics spike + Claude-lane auto-memory containment at B1 [XREF:S03, S05, S09]; interview-taxonomy and rubric seed sessions at B2 [XREF:S06, S07]; GameMode probe at B4 [XREF:S12]; workforce-map rendering choice + lockfile serialization at B6 [XREF:S15, S16]; the S16 pin cells resolve at the first quarterly manifest pass. **STATUS 2026-07-23:** the B1 spikes (serialize-by-deny → A3; PreCompact/injection → A4; auto-memory containment → A2) and the B4 GameMode probe (→ A5) have all EXECUTED and closed via the S00.9 changelog.

### S19.6 Bring-up measurement sequence

The 13 distinct `TBD-BRINGUP` markers, sequenced against the build phases; each records its result in the platform repo (the G3 Def.8 discipline):

- **At B0/B1 (shell + substrate):** the operator suspend-session probe (reboot-survival + `Persistent=` catch-up + user.slice freeze/thaw) [S01]; the anthropic unified-header utilization-scale + `7d_oi` observation before the overlay is trusted for park timing [S10]; the parallel-gate fallback-rate measurement on the default worker model — this **is** the serialize-by-deny reconfirm (carried as both `TBD-BRINGUP` by S03 and `TBD-P3` by S02/S04; sequenced here as **one** task) [coordinator-draft; XREF:S03, S02].
- **At B2 (verification):** the entailment calibration set (planted pairs + TPR/TNR bar, which also sets the mandatory-coverage bar and `verification.entailment_sample_rate`) and the golden-set seed per launch domain [S07].
- **At B4 (local tier):** the workhorse bakeoff, the VRAM-ledger calibration, the CPU-tier throughput floor, the per-duty confidence calibration, and the contradiction-screen precision/recall — the consolidated G3 Def.8 battery [S12, S09].
- **At B5 (observability, once run history exists):** the per-run-type silence budgets, derived from observed event cadence [S14].

### S19.7 Readiness for G4

The assembled `core-architecture-v1.md` (concatenation of S00–S19, drafts canonical) is the G4 review object. The G4 agenda is **not** open design — it is confirmation of the coordinator-draft judgment calls surfaced during drafting: the **49 `[coordinator-draft]` flags** enumerated across the section Open-items lines (settings clamp ranges, naming choices, a handful of interlock refinements) plus the four sections whose Open-items line resolved a tension inline rather than reading "none" (S02 storage = git-worktree+overlayfs; S03 micro-fanout = keep disabled; S07 rework cap = 3; S10 price-seed + local-floor supersessions). None reopens a ratified gate decision or a D-constraint. The operator's approval at G4 freezes v1 and ends the research phase (CLAUDE.md flag); it is the only act that does so.

---

**Settings introduced (⚙):** none — S19 introduces no settings; the consolidated index is S18.

**Known problems owned here:** none — the register is S17; S19 only proves KP-1..14 coverage (S19.2) and consolidates parked re-entry (S19.4).

**Deferred / parked:** this section *is* the consolidation (S19.4); it parks nothing new.

**Coverage:**

| Feature-list item | Where |
|---|---|
| §15.1 v0 scope definition | S19.1 |
| §15.6 multi-user-from-day-one boundary | S19.1 (+ enforcement [XREF:S01.9]) |
| §15.2 / §15.4 / §15.5 phase definitions | S19.1, S19.3, S19.4 |
| 15.3 benchmark **gate** (governs expansion; machinery [XREF:S14]) | S19.1, S19.4 |
| Coverage proof — all 182 items owned | S19.2 |
| KP-1..14 known-problems coverage (S17 sweep discharged) | S19.2 |
| 2.8 missed-slot boundary (v0 mechanism / v1 surface) | S19.2 |
| Build order (D3.1 P2 → P3 handoff) | S19.5, S19.6 |

**Open items for G4:** none. The coverage dispositions of the nine no-row items (S19.2) and the GitHub-Pro reconciliation (S19.4) are tagged `[coordinator-draft]` for G4 confirmation; the full `[coordinator-draft]` flag census and the assembled document are the G4 memo's subject (S19.7).
