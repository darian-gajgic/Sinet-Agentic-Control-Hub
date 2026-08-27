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
