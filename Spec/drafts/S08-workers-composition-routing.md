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
