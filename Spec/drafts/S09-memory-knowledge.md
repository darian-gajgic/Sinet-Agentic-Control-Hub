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

Detection of *substantive* conflicts is a write-time duty, cheapest at this corpus size: on adoption/edit/approval the control plane looks up same-`topic_key` entries across the scopes the writer can see, plus an advisory local-model contradiction screen (duty alias [XREF:S12]; precision/recall measured at TBD-BRINGUP(contradiction-screen P/R) [G3 Def.8]). Hits create a `conflicts_with` edge and a **question card** to the affected owner — surfaced, never silently resolved. At assembly time, a known-conflicting pair entering one frame is flagged in the trace manifest and the question re-raised if unresolved [R11 §4.6].

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
