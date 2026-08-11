# P3-RW-7 grounded brief — honest board presence for onboarding tasks (kanban status + project attribution)

**Packet class:** rework (builder-reported on the live demo world, coordinator-verified).
**Headline:** *an onboarding task renders on the board with an honest declared column and its project's name from the moment it exists, through approval, with no fabricated status anywhere.*
**Scope:** backend (Go + possibly one migration). `web/src` is reference-only for this packet — the board already renders whatever honest values the backend serves; **no frontend file changes**, and `web/src/kanban.ts` is NEVER moved (a Go seed guard parses it in place — §1.6 below).

All file/line references verified against the working tree on 2026-08-11 (HEAD `278524d`, P3-RW-5 landed; `internal/stage/onboard.go` last touched by P3-RW-2 commit `0fc74d2`).

## 1. What exists (verified in code this session)

1. **The gap, half A — no kanban status.** `Skeleton.StartOnboarding` (internal/stage/onboard.go:94–130) inserts the onboarding task row at :108–111 as `INSERT INTO tasks (task_id, user_id, title, created_ts) … ON CONFLICT (task_id) DO NOTHING` — **no `kanban_status`**, so the column takes 0001's schema default `''` (0001_core_schema.sql:27, `TEXT NOT NULL DEFAULT ''`). Every other task producer writes a status: intake inserts with `'intake'` in the INSERT itself (internal/intake/pipeline.go:249–250); the stage skeleton moves it via `setKanban` (skeleton.go:797) — `'cancelled'` :361, `'executing'` :388, `'verifying'` :563, `'done'` :759, `'attention'` :770; also answer.go:94/:133 and cancel.go:192/:239. Nothing in the onboarding lifecycle (StartOnboarding → dispatchOnboard → AnswerOnboarding, onboard.go) ever touches `kanban_status`.
2. **The gap, half B — no project attribution.** Migration `0022_pin_attribution.sql` defines `task_project` as COALESCE(claims-collapse, intake-pin-edge) (:87–112). An onboarding task has neither edge: it is not an intake-pipeline task (no `intake.state` event, so no `$.registry.project` pin) and never writes `artifact_claims`. So `GET /api/tasks` (reads.go:592–595, LEFT JOIN + COALESCE to `noProjectBucket` = `"(no project)"`, reads.go:73) serves `project: "(no project)"`, and the task-detail lineage (reads.go:1230–1243) serves the same — for the very task whose id names its project: `stage.OnboardTaskID(projectID)` = `onboard-<project>` (onboard.go:39,53).
3. **The board contract** (`web/src/kanban.ts`, map §3 v3 — P3/design/product-map.md:48, checkpoint-2 decision D-B at :143): five declared columns — stored `intake` renders as **Backlog**, then `executing`/**Executing**, `verifying`/**Verifying**, `attention`/**Needs attention**, `done`/**Done** — plus the absorbed `cancelled` (renders IN Backlog with a sign). **Done stays visible** (operator-ratified D-B). Forward tolerance (kanban.ts:51–63): an unknown stored value gets a raw column of its own; `''` renders as the column **"(no status recorded)"** (kanban.ts:60). That raw column in front of the operator is exactly the checkpoint-2 C2-6 wound the seed hygiene fixed for seeded tasks (seedworld_test.go:133–149) — but the guard runs at seed time only; a project created LIVE re-creates it.
4. **Lifecycle today** (onboard.go): StartOnboarding runs the deterministic register→clone→scan→draft FIRST (OnboardStart at :101 — internal/project/onboard.go:59–128 registers the entry `pending` BEFORE the task row exists), then inserts task+run in one tx, enqueues. dispatchOnboard (:134–170) transitions the run RUNNING, re-reads the draft (idempotent), surfaces the durable `onboard:<project>` ask, parks. AnswerOnboarding (:175–237) enforces D10, activates the entry (OnboardApprove → registry state `active`, registry.go:252), marks the ask answered, resumes then COMPLETES the run ("onboarding complete: entry active"). The task's `kanban_status` stays `''` through all of it.
5. **The durable relational link already exists.** `repo_registry.project_id` (owner-attributed, state `pending`→`active`) is written by `Register` inside `Store.Onboard` — i.e. it exists BEFORE the task row is inserted, and no production code path ever deletes a registry row (only a test does, registry_test.go:112; internal/project/gc.go is worktree-GC only). The onboarding task→project edge is therefore derivable relationally: `t.task_id = 'onboard-' || rr.project_id`.
6. **The seed guard reads kanban.ts in place.** `declaredKanbanStatuses` (internal/api/seedworld_test.go:494–524) regex-parses `../../web/src/kanban.ts` for the declared statuses + the absorbed `cancelled`, and `assertNoUndeclaredKanbanStatus` (:457–487) fails the seed if any task sits in an undeclared column. **Never propose moving kanban.ts** — the path is load-bearing.
7. **Run-role plumbing is complete** — `.onboard` is a routable suffix (seedworld_test.go:434 keeps it in the demo-inertness predicate); waiting-on-human already derives from the parked run + open ask on the list read (reads.go TaskListRun, :526–555), so "awaiting the owner" is already visible on the card face without any column fabrication.
8. **The task overlay card opens sanely today** on an onboarding task (reference check only): `GET /api/tasks/{task}` (reads.go:819–879) finds the row, serves absent spec/plan with `ArtifactsAbsent`, and web/src/TaskDetail.tsx renders the honest `Absent` reason (:224–227) and lineage project via `detail.lineage` (:778–784). No crash; only the two dishonest values (blank status, "(no project)") are wrong.

## 2. Consumer inventories (swept this session — briefs are expired artifacts; this sweep supersedes RW-3's)

**`kanban_status` consumers (non-test Go + SQL + the guard):**
- Writers: intake/pipeline.go:249 (`'intake'` at INSERT); stage/skeleton.go setKanban call sites :361/:388/:563/:759/:770; stage/answer.go:94/:133; stage/cancel.go:192/:239; **stage/onboard.go:109 — the one INSERT that omits it (the gap)**.
- Readers: api/reads.go:592 (task list; `?status=` filter :607) and :832 (task detail); api/projection.go:232 (the SSE `board` topic snapshot); stage/surface.go:376 + deriveKanban :457 (demo/watch surface; overlays `attention` on a tombstoned lineage — stage surface only, NOT the API reads); history/catalog.go:171–174 (canned query `status.tasks_by_kanban` — a raw `''` is queryable there too); 0016_queryable_history.sql:81/:90 (`cost_per_task` carries the column).
- Vocabulary: web/src/kanban.ts (owns it); Go seed guard seedworld_test.go:457/:494 (parses it). Schema: 0001:27.

**`task_project` consumers (current tree):** api/reads.go:595 (task list) and :1235 (task-detail lineage); internal/history/layer0.go:36 (`ViewTaskProjectEdge` — the Layer-0/Layer-2 allowlist registry, whose `Description` string is served and frozen into the golden fixture web/src/fixtures/api/history-views.json, "Name": "task_project" block); `cost_per_project` (defined over it in 0022:120–136; consumed via layer0.go:28 + history/catalog.go:510). Test pins that read the view or the 0022 file: history/pinattribution_test.go, api/pinattribution_test.go, chatapi_test.go, historyapi_test.go, injection_test.go, reads_test.go, apifixtures_test.go.

**Migration-ceremony pins (all verified individually; the set GREW since RW-3 — five now, not four):**
- `user_version` want-22 pins: internal/evals/evals_test.go:37–38; internal/api/meters_verbs_test.go:769–770; internal/conformance/conformance_test.go:116–117; internal/api/benchmark_test.go:653–654; internal/history/pinattribution_test.go:430–431 (added BY RW-3).
- Benchmark migration-glob ceiling: api/benchmark_test.go:665 `> "0022_zzz"`.
- Preview no-migration guard: internal/preview/conformance_test.go:125 `if name >= "0023"` (+ its enumerating comment).
- NOT a pin to move: history/pinattribution_test.go:382 asserts the 0022 FILE text contains `PRAGMA user_version = 22` — stays as-is (0022 is immutable). storage_test.go:70 counts files vs version — self-adjusting. settings read_test.go:187/193 and api/settings_test.go:717/725 "want 22/23" are settings-history VALUES, unrelated (verified).
- Money-scan: history/layer0_test.go:18–21 `moneyViewMigrations` lists every migration that creates/re-creates a Layer-0 money view; a 0023 that re-creates `cost_per_project` must join this list.
- Plan pin: `TestPinProbeSeeks` (internal/history) pins the 0022 pin-probe's index usage by full index name; a re-created `task_project` must keep the CROSS JOIN plan (0022:79–82 says the join order is load-bearing).

## 3. Requirements (numbered, each with its S-ref)

- **R1 — an honest declared column from the first instant.** The onboarding task row is born with a `kanban_status` drawn from the board's declared vocabulary (S13.7: "onboarding a repository is itself a task"; S15.5 ¶2 board; map §3 v3). Nothing anywhere renders or stores a fabricated stage — the value written must be TRUE of the task at that moment under the declared columns' meaning. The CONVENTIONS §8-precedent single transaction at StartOnboarding (task+run+status in one tx) is the shape intake set (pipeline.go:245–260).
- **R2 — the status advances through the lifecycle by the existing path.** On owner approval (AnswerOnboarding), the task's status moves via the landed `setKanban` mechanism (or an in-tx UPDATE, the intake INSERT precedent — builder's choice; both shapes exist in tree), landing in a declared column consistent with S13.7 ("the owner approves … entry goes active" — activation ends the onboarding task's work; the run already transitions to Completed at :227). No new status string enters the vocabulary; kanban.ts is untouched.
- **R3 — the task→project edge resolves `onboard-<project>`.** The linkage answers `<project>` for an onboarding task through whichever option the coordinator picks (§11 OQ1), honoring 0022's single-place rule: the board, the assistant (Layer 0/2 over `task_project`), the task-detail lineage and `cost_per_project` must all give the same answer, because they all read the one view (S06.2, S14.10 ¶1, S15.2 tasks family; 0022's own "the weakness stays stated in ONE place").
- **R4 — money follows attribution** (0022's recorded disposition): the onboarding ceremony's spend (its run's receipts) lands under the project in `cost_per_project` once the edge exists, and 0016's sum-preservation invariant holds — one row per task out of `task_project`, sum over `cost_per_project` equals sum over `cost_per_run` (S14.10 ¶1; asserted by the existing pinattribution tests).
- **R5 — full migration ceremony if the migration option is chosen** (§6 option B lists it exhaustively; CONVENTIONS §6: migration files immutable, contiguous, one transaction, `user_version` sole authority). 0001–0022 stay byte-untouched.
- **R6 — every derived truth statement stays true.** 0022's `cost_per_project.linkage` note "must stay TRUE for whatever ships" (0022:116); layer0.go's `task_project` Description (served AND frozen in history-views.json) currently says claims-else-pin and must gain the third edge in the same breath; the reads.go comments at :585 and :733 likewise.
- **R7 — no regression of the honest bucket.** A task with no claims, no pin and no onboard shape still lands in `"(no project)"` — rendered, never dropped, never fabricated (§37; 0022:74–77); `''`-status forward tolerance in kanban.ts stays functional for genuinely unknown producers (it is a designed proof surface — B6-5 OQ7 — not a bug).

## 4. Seams to respect

- `stage.Skeleton` onboarding trio (StartOnboarding / dispatchOnboard / AnswerOnboarding) and the existing `setKanban` path — the producers. `OnboardTaskID`/`OnboardAskID` stay the single naming authority (onboard.go:47–54).
- `internal/project` registry: `repo_registry.project_id` + `state` is the durable relational anchor; no new column, no new table needed. Do not touch registry semantics.
- The storage seam: any view change is a NEW numbered migration under `internal/storage/migrations/` (CONVENTIONS §6). `internal/storage` remains SQL's only home for the edge.
- `internal/history` Layer-0 registry (layer0.go): view names constant; Description strings truthful; Layer 2 allowlist unchanged in membership.
- The API read seam (reads.go): under the migration option it changes NOTHING structurally — the LEFT JOIN + COALESCE already serves whatever the view resolves. The api-side COALESCE option (§6 option C) would break this seam's discipline; it is listed only to be weighed.
- `web/src/kanban.ts` — the declared-column contract, parsed in place by the Go seed guard. NEVER moved, never edited by this packet.

## 5. ⚙ settings

**None.** No new setting, no consumed setting changes — the column vocabulary is not a registry key, and the view change is structural. No new dependency of any kind (pure Go stdlib + existing SQL). Expect the S18 sweep untouched.

## 6. Files expected to change, per option

**Half A (kanban status — needed under EVERY option):**
- `internal/stage/onboard.go` — the INSERT at :109 gains `kanban_status` with the chosen birth column (OQ2); AnswerOnboarding gains the approval-time advance (OQ2/OQ3).
- New acceptance tests (executor materializes §8): `internal/stage` (internal or e2e) and/or `internal/api` (door-to-list).

**Half B, option A — producer-side only (no migration):** REJECTED-shaped, stated for completeness: fabricating an `intake.state` event (or a claims row) for a non-intake task would forge a record of a ceremony that never ran — the exact class of dishonesty this packet exists to remove. No file list.

**Half B, option B — migration `0023` (a third edge in `task_project`):**
- `internal/storage/migrations/0023_<slug>.sql` — NEW: `DROP VIEW cost_per_project; DROP VIEW task_project;` re-create `task_project` with a third COALESCE arm resolving the onboarding shape, re-create `cost_per_project` (its `linkage` string updated to stay true, otherwise column-for-column as 0022); `PRAGMA user_version = 23`. Two candidate arm shapes for the coordinator/builder (OQ1): **(i) registry-join** — `(SELECT rr.project_id FROM repo_registry rr WHERE t.task_id = 'onboard-' || rr.project_id)` — attributes only to a project that durably exists (the row predates the task, §1.5) and yields honest NULL otherwise; **(ii) id-surgery** — `CASE WHEN t.task_id LIKE 'onboard-%' THEN substr(t.task_id, 9) END` — cheaper, but attributes to a name with no relational anchor. Grounding leans (i); `project_choices` = 1 for the arm either way (a registry id resolves exactly one project, the 0022 pin precedent).
- `internal/preview/conformance_test.go:125` — guard `>= "0023"` → `>= "0024"`, comment enumerates 0023.
- The five want-22 `user_version` pins → 23 (§2 list: evals_test.go:37, meters_verbs_test.go:769, conformance/conformance_test.go:116, api/benchmark_test.go:653, history/pinattribution_test.go:430) + benchmark glob ceiling :665 `0022_zzz` → `0023_zzz` (and its :666 message).
- `internal/history/layer0_test.go` — `moneyViewMigrations` gains the 0023 file (it re-creates a money view; the no-money-by-generation scan must cover it).
- `internal/history/layer0.go` — `task_project` Description string gains the onboard edge (R6).
- `web/src/fixtures/api/history-views.json` — regenerated (`SINET_WRITE_API_FIXTURES=1`, apifixtures_test.go:69–75 mechanism) because the Description string is frozen there. `web/src/fixtures/api/tasks.json` regenerates ONLY if the fixture world itself changes (OQ4) — the fixture world currently onboards no project, and grep confirms no `onboard-*` task in tasks.json.
- Comment truth: api/reads.go:585/:733 (R6).
- `TestPinProbeSeeks` and the pinattribution suites re-run green — the CROSS JOIN plan pin must survive the added arm (the new arm is a plain correlated subquery over the tiny `repo_registry`; it must not disturb the runs/run_events legs).
- pinattribution_test.go:382 (0022 text pin) is NOT edited.

**Half B, option C — api-side COALESCE (reads.go + taskLineage):** disfavored by 0022's own recorded reasoning (:25–27): it heals the board while the assistant's Layer-0/Layer-2 answers and `cost_per_project` keep saying "(no project)" about the same task — linkage logic in a second place is the documented wound. The honest case FOR it: no migration ceremony, zero SQL risk. The case dies on R3's same-answer-everywhere requirement.

## 7. Adopted components

None touched. No engine, no new module, no lock change.

## 8. Acceptance-test SPECIFICATIONS (standing PROCESS EXCEPTION: the grounding round commits NO tests and touches NO Go/SQL file — a sibling grounding runs concurrently; the executor materializes these)

- **T1 (door → board, the headline):** through the real projects door (POST create, api/projects.go:645 path) on a fresh world: the very first `GET /api/tasks` after create shows `onboard-<p>` with the chosen declared `kanban_status` (not `''`) and `project = <p>` (not `"(no project)"`). Also assert the SSE `board` topic snapshot (projection.go:230) carries the same status.
- **T2 (through approval):** answer the `onboard:<p>` ask as the owner (approve) → the task's `kanban_status` lands the chosen terminal column (OQ3 disposition; expected `done`), the row is STILL on the list (Done stays visible, D-B), `project` unchanged.
- **T3 (lineage + overlay backend):** `GET /api/tasks/onboard-<p>` → `lineage.project = <p>`, `lineage.project_choices = 1`; spec/plan absent with `ArtifactsAbsent` set (no fabricated artifacts).
- **T4 (money follows attribution, migration option):** an onboarding run with a receipt → `cost_per_project` shows the spend under `<p>` with the registry name/state joined (0022:121–123 shape); sum over `cost_per_project` == sum over `cost_per_run` (extend the existing sum-preservation test in history/pinattribution_test.go with an onboard-edge case).
- **T5 (the honest bucket survives):** a plain task with no claims/pin/onboard shape still serves `"(no project)"` on list and lineage; a task literally NAMED `onboard-nonexistent` with no registry row resolves NO project under arm (i) (registry-join) — assert whichever arm ships behaves as documented.
- **T6 (filters):** `GET /api/tasks?project=<p>` returns the onboarding task; `?status=<birth column>` returns it pre-approval and not post-approval.
- **T7 (ceremony, migration option):** `user_version` = 23 contiguous; 0001–0022 byte-untouched (existing immutability guards); money-scan covers 0023; `TestPinProbeSeeks` green; preview guard at 0024.
- **T8 (crash-relaunch idempotence):** StartOnboarding re-run after the ErrExists path (:120–121) neither duplicates the task nor regresses an advanced status (the `ON CONFLICT DO NOTHING` insert must not overwrite a later column — note DO NOTHING already guarantees this; assert it stays that way).

## 9. Acceptance checklist (the headline, decomposed — the evaluation rubric)

1. An onboarding task NEVER serves `kanban_status: ""` — from the create response's first consistent read onward.
2. The status it serves is a column `web/src/kanban.ts` declares, at every lifecycle stage, and its meaning is TRUE at that stage (no fabrication).
3. An onboarding task NEVER serves `project: "(no project)"` — list, lineage, and (under the migration option) cost views all name `<p>`, from the same single place.
4. Approval advances the column; the completed task remains visible per D-B.
5. The seed-hygiene guard (`assertNoUndeclaredKanbanStatus`) would pass on a world where a project was onboarded live.
6. No fabricated records: no synthetic intake.state, no synthetic claims, no invented status strings.
7. Honest buckets unchanged for everything that is not an onboarding task (R7).
8. If a migration shipped: the FULL §6-option-B ceremony is complete — no stale pin, no uncovered money view, no stale Description/fixture/comment.
9. `web/src/kanban.ts` untouched, unmoved; no frontend file changed.
10. No new ⚙, no new dependency; S18 untouched.

## 10. Binding CONVENTIONS constraints

- §5 commit/process (subject format, explicit staging, no push from packet sessions).
- §6 storage: migrations immutable/contiguous/one-tx, `user_version` sole authority; append-only discipline.
- §8 precedent: the task exists in the D7 record from its first instant — one tx for task+run(+status).
- §13 ledger-never-nests (AnswerOnboarding's OnboardApprove-outside-tx shape stays).
- §16 issueCard/closeAndResume precedent (the ask machinery is not re-invented).
- §37 queryable-history: honest "(no project)" bucket; single-place linkage; Layer-0 descriptions truthful; no money by generation (the scan enforces it on any new money-view migration).
- §38 typed errors on the surface; EXPLAIN-evidence discipline for any new query leg (0022's measured-plan precedent).
- §42 board/golden-fixture tie: fixtures regenerate only as a deliberate act with the diff in front of you (apifixtures_test.go:18–27).

## 11. Open questions — NOT resolved here; the coordinator dispositions each

- **OQ1 — the option choice for attribution (half B).** Migration 0023 third edge (option B) vs api COALESCE (option C) vs producer-side fabrication (option A, rejected-shaped). Within option B: arm (i) registry-join vs arm (ii) id-surgery. Grounding leans B(i): the registry row is a durable, owner-attributed, pre-existing relational record — the linkage stays in the one place, is anchored to a real project, and money follows for free.
- **OQ2 — the column per lifecycle stage.** Grounding leans: birth-through-parked = `intake` (Backlog) — the map's own words for Backlog are "record of intent" pre-execution, the intake precedent keeps a task in `intake` through its entire waiting-on-human ceremony with the ask carrying the urgency (waiting_on_human already derives true for the parked onboarding run), and `attention` is escalation vocabulary (verify-terminal :770, tombstones), not "awaiting a scheduled approval". Alternative weighed: a transient `executing` during the scan/draft — honest but a flicker; the deterministic prep completes inside StartOnboarding before the run even dispatches, so there is nothing for the board to catch. The operator may word it differently — their answer is authoritative.
- **OQ3 — completed-onboarding disposition.** Grounding leans `done`, staying visible: D-B is operator-ratified ("I want to see what is already done"), S13.7 ends the task at activation, and the skeleton's own completion precedent is `setKanban(done)` (:759). Leaving the board would need a vocabulary the platform doesn't have (no archived state exists at v0) and would contradict D-B.
- **OQ4 — fixture-world onboarding.** Should the golden fixture world gain an onboarding task (churning tasks.json + possibly task-detail goldens) so the contract is frozen in a fixture, or do the Go acceptance tests suffice with fixtures untouched? Grounding leans: tests suffice; fixtures churn is a deliberate act (§42) and T1–T8 pin the behavior. history-views.json regeneration is NOT optional under option B (the Description string changes).
- **OQ5 — the tombstoned-onboarding column.** The API reads serve the STORED status; `deriveKanban`'s attention-overlay lives only on the stage demo surface (surface.go:457). A crashed onboarding would sit in its birth column on the real board — the same recorded B0-4 deferral every task class shares. Out of scope here unless the coordinator says otherwise; noted so nobody mistakes it for RW-7 regression.

## 12. Process notes (coordinator-facing)

- Sibling grounding runs concurrently: this brief touches NO Go/SQL file and commits ONLY itself.
- The RW-3 brief's "5 consumers" figure is superseded by §2's sweep (the pin set grew to five want-22 pins because RW-3's own test added one; the RW-3 brief predates it).
- No spec conflict found: S13.7, S15.2, S15.5, S14.10, the map §3 v3 and the landed code tell one consistent story; the gap is pure omission at the producer plus a view predating the onboarding task class.
