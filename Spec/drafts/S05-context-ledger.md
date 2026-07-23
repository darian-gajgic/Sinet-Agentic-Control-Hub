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
