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
