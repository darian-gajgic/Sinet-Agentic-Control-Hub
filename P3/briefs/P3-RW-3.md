# P3-RW-3 grounded brief — pre-approval project attribution (the durable intake-time pin reaches the task list)

Grounded 2026-08-10 against `Spec/core-architecture-v1.md` (drafts canonical) and the live tree.
This brief is a SINGLE-USE artifact: it expires when the packet lands. Code + spec are the only truth.

**Acceptance headline (coordinator, STATE 2026-08-10):** a task with a durable intake-time
project pin serves that project on `GET /api/tasks` from birth — before any plan approval —
while `(no project)` stays honest for genuinely unmatched tasks. Money-attribution semantics
are settled EXPLICITLY as an OQ, never changed silently.

**PROCESS EXCEPTION (coordinator-logged in STATE):** this grounding round commits NO failing
tests and touches NO Go/SQL file — the concurrent P3-RW-2 drain requires the full battery
green. §8 below is the acceptance-test SPECIFICATION; the executor materializes it red-first
when the packet runs, after RW-2 lands.

## 1. What exists (verified in code this session)

- **The pin is durable from the task's first instant.** `intake.Pipeline.Start`
  (`internal/intake/pipeline.go:125–274`) runs the S06.2 registry match at triage and writes
  task row + intake run + FIRST `intake.state` event in ONE transaction (lines 236–251). The
  matched slice lands as `State.Registry` (`json:"registry"`, `state.go:105`) whose `.Project`
  (`json:"project"`, `intake.go:163`) is the registry id — so the pin's durable JSON path in
  the latest `intake.state` payload is `$.registry.project`. The P3-RW-1 rule is in the same
  function (pipeline.go:148–168): a PINNED request (`Request.Project`, `intake.go:102–108`)
  never degrades — any failure to resolve it fails `Start`, so no task is born believing it
  got a project it did not. `LoadState` (`state.go:282–300`) reads the latest event across the
  task's runs by `event_seq`.
- **Claims exist only at plan approval.** `writeClaimsTx` (`internal/intake/answer.go:553–624`)
  inserts the S02.8 write-set claims into `artifact_claims` inside the approval transaction,
  with `project := st.Registry.Project` (lines 575–578). **The claim's project and the pin are
  the same value by construction** — they can never disagree; the linkage gap is purely
  temporal (birth → approval).
- **The view collapses claims only.** Migration `0016_queryable_history.sql:196–215` creates
  `task_project` over `artifact_claims` (`''` rows excluded, one row per task, `project_choices`
  carried), with the RECORDED READING that claims are "the only POPULATED relational edge" —
  true when written; the pin is a payload edge in `run_events`, not a relational column.
  `cost_per_project` (lines 221–237) LEFT JOINs it and carries the `linkage` note string
  `'via artifact_claims.project (intake-resolved); …'`.
- **The defect, live:** `handleTaskList` (`internal/api/reads.go:551–645`) LEFT JOINs
  `task_project` and COALESCEs to `(no project)` (lines 572–604), so a pinned task reads
  "(no project)" on the operator's board until approval. `taskLineage` (reads.go:1211–1224)
  reads the same view for the task detail. The list serves through `writeReadRedacted`
  (line 644); the task detail goes out UNWRAPPED (§38 ruling (b)) with payload-derived strings
  redacted per value (§38 D10 precedent, `redactStageProgress`).
- **The board SSE snapshot carries no project** (`BoardTask`, `internal/api/projection.go:216–228`)
  — and the SPA's ratified wiring makes that survivable: `web/src/live.ts:19–24` records
  "REST IS THE TRUTH, FRAMES ARE THE TRIGGER — the server's per-topic snapshot states are
  face-incomplete for S15.5 (no owner, no project, no stage…)"; views re-read REST on frames,
  and `intake.state` is ALREADY in `boardEventTypes` (live.ts:49), so the board re-reads
  `/api/tasks` at the exact moment the pin is born. `TaskListRun`'s comment (reads.go:510–512)
  pins that the `board` topic snapshot "stays byte-for-byte as it was".
- **Migrations 0001–0021 are committed and immutable** (CONVENTIONS §6; `storage/migrate.go`).
  0022 is free — but `internal/preview/conformance_test.go:124` FAILS on any migration
  `>= "0022"` (the never-adds-a-migration guard), and two exact `user_version` pins want 21
  (`internal/conformance/conformance_test.go:116`, `internal/evals/evals_test.go:37`).
  `internal/history/layer0_test.go:19` reads `0016_queryable_history.sql` by literal path for
  the no-money-by-generation scan.
- **The §40-B wall, as the code actually pins it:** `internal/api` imports neither
  `internal/project` nor `internal/broker` — AST-scanned by
  `TestAPIReachesTheOutwardWorldOnlyThroughTheAccepter` (`internal/api/acceptapi_test.go:527+`)
  and restated in `internal/api/projects.go:30–37`. NOTE: `internal/api` DOES import
  `internal/intake` (reads.go:15 — artifact types, `*intake.FollowUp`, the intake surface);
  the packet-cut shorthand "neither internal/project nor internal/intake" is corrected here
  to the code fact. What the wall actually forbids a fallback: reaching the registry or the
  pipeline for linkage. The sanctioned seam is SQL over rows/views the projector already
  reads — reads.go already parses `intake.*` event payloads by SQL + json_extract
  (the decision derive, reads.go:1192–1202).
- **Golden-fixture tie (§42):** `web/src/fixtures/api/tasks.json` (9 × `"(no project)"`,
  1 × `"release-notes"`) is asserted against the REAL handler in
  `internal/api/apifixtures_test.go` (regeneration only via `SINET_WRITE_API_FIXTURES=1`,
  line 20/73). The fixture world's one `intake.state` seed is synthetic
  (`{"stage":"drafting","kind":"intake"}`, apifixtures_test.go:306) — no `registry` key, so
  `json_extract($.registry.project)` is NULL there and fixture VALUES are expected stable
  under either fix altitude; the executor verifies rather than assumes.
- **Fencing note for tests:** api tests seed `run_events` rows by direct INSERT
  (apifixtures_test.go pattern); production appends ride the envelope.

## 2. Consumer inventory (swept this session; the OQ-(a) substrate)

**Consumers of the `task_project` VIEW — five sites, nothing else in the tree:**

1. `cost_per_project` (0016:235) — **money attribution**. Itself consumed by:
   history Layer-0 `SelectView` (owner-scoped `user_id`, `internal/history/layer0.go:96–99`),
   the Layer-1 canned query `cost.for_project` (`internal/history/catalog.go:506–511`),
   the Layer-2 open-SQL allowlist (= the Layer-0 registry), the `/api/events` transport
   routes, and the S15.7 chat assistant (which consumes the history layers and nothing else).
2. `internal/api/reads.go:578` — `handleTaskList` LEFT JOIN + the `?project=` filter clause
   (line 591; `(no project)` is a selectable filter value, §38).
3. `internal/api/reads.go:1216` — `taskLineage` (task detail `lineage.project` +
   `project_choices`).
4. `internal/history/layer0.go:36,136–139` — the closed Layer-0 registry row
   `ViewTaskProjectEdge`: **no owner column ⇒ operator-only** at Layer 0 AND Layer 2
   (§37-C R27), with the §38 D5 transport 403 pre-check pinned on exactly this metadata bit
   (`historyapi_test.go`, `chatapi_test.go`).
5. Tests that pin its semantics: `internal/history/layer0_test.go` (money scan over the 0016
   file), `internal/history/injection_test.go`, `internal/api/{historyapi,chatapi}_test.go`
   (operator-only bit), `internal/api/apifixtures_test.go:221–226` (artifact_claims is "the
   only populated project edge" seed comment).

**Consumers of the SERVED tasks-list `Project` field:** `web/src/Board.tsx` (group-by-project,
filter, card face, and the app-wide project lens `useProjectScope` over `mission-control` +
`board`, `web/src/project.tsx:46`), the task detail lineage render, Mission control's
filterable list, and the golden fixture `tasks.json`. `web/src/Intake.tsx:110–139` names the
RW-1 pin field explicitly. **No SPA code change is required by this packet**: the field, the
`(no project)` handling and the `intake.state`-triggered re-read all exist.

**Consumers of the `board` SSE snapshot:** the SPA subscribes topicless and renders from REST
(live.ts:9–24); no view renders `BoardTask` content as truth. `internal/api/sse.go` serves it;
`reads.go:510–512` promises it stays byte-identical.

## 3. Requirements (each numbered, each with its S-ref)

- **R1 — attribution from birth.** A task whose latest `intake.state` payload carries a
  non-empty `$.registry.project` serves that registry id as `project` on `GET /api/tasks`
  from the moment `Start`'s birth transaction commits, before any plan approval.
  [S06.2 (registry match at triage, S1.6); S15.2 (tasks family exposes "lineage (project,
  follow-ups)"); S15.5 (board cards group by project)]
- **R2 — the honest bucket.** A task with NO resolved registry slice (no pin, no claim)
  serves `(no project)` exactly as today, and `(no project)` remains a selectable filter
  value. Never a fabricated id, never a dropped row. [S14.10 ¶1 honesty posture; CONVENTIONS
  §37 (honest absence), §38 ("absences are rendered, never errors that hide the subject")]
- **R3 — claim mechanics untouched.** `writeClaimsTx` remains the only `artifact_claims`
  writer and S02.8 semantics (claims at plan approval, glob-intersection, waiting/active,
  ⚙ `claims.default_write`) are byte-untouched. This packet is read-side attribution only.
  [S02.8; CONVENTIONS §14]
- **R4 — filter parity.** `?project=<id>` on `GET /api/tasks` matches pinned-unapproved
  tasks; the filter clause and the served column use ONE resolved expression so they cannot
  disagree. [S15.2; S14.10 preamble (filterable by project)]
- **R5 — one attribution, two surfaces.** The task list and the task detail lineage serve
  the SAME attribution from the same source expression — the §38 "called rather than copied"
  discipline; the two surfaces may never disagree about a task's project. [S15.2; S15.5]
- **R6 — continuity across approval.** For a pinned task, approval does not change the served
  value (`project` stays the pin's id; `project_choices` stays 1) — grounded fact: the claim's
  project IS `st.Registry.Project`, so pin and claim cannot conflict. A value flicker at
  approval is a defect. [S06.9; S02.8]
- **R7 — money settled explicitly.** `cost_per_project`'s meaning changes only per the
  coordinator's OQ-(b) disposition, and under EVERY option the 0016 sum-preservation
  invariant holds: the sum over `cost_per_project` equals the sum over `cost_per_run` — no
  spend goes missing and no spend double-counts. The `linkage` note string must remain TRUE
  for whatever ships. [S14.10 ¶1; S10.1 honesty; CONVENTIONS §37]
- **R8 — migration discipline.** Migrations 0001–0021 stay byte-untouched. If the fix is a
  view change it is migration `0022` (DROP VIEW + re-CREATE, one transaction,
  `PRAGMA user_version = 22`). [S02.1; CONVENTIONS §6]
- **R9 — the wall and the connection.** `internal/api` gains no import of `internal/project`
  or `internal/broker` (the AST pin stays green); any api-layer read is SQL over
  rows/views on the projector's connection; list handlers keep the explicit
  `rows.Close()`-before-decorate discipline on the ONE shared connection. [S01.3; S02.1;
  CONVENTIONS §38]
- **R10 — redaction at the edge.** The pin value is lifted from a `run_events` PAYLOAD, so it
  is payload-derived: on the list it rides the existing `writeReadRedacted` edge (already
  wrapped); on the UNWRAPPED task detail it must pass the redaction primitive per value (the
  §38 D10 `redactStageProgress` shape). Stored rows stay verbatim (store-raw/serve-redacted).
  [S14.3/Research 18 §7-C2; CONVENTIONS §30/§38]
- **R11 — owner scope unchanged.** The list stays owner-scoped server-side; if `task_project`
  is re-created it still carries NO owner column and stays operator-only in the history
  registry at Layers 0 and 2 (the §38 D5 pre-check keeps answering 403). [S01.9; S14.10 ¶3]
- **R12 — no new anything.** No new ⚙ key (S18 ratifies none for read-side attribution — this
  packet declares NONE; `internal/settings/index.go` byte-unchanged), no new dependency
  (`components.lock` unchanged), no new event type, BENCH-REG untouched. [S18; CONVENTIONS §38
  precedent]
- **R13 — the SSE board snapshot** changes only per the OQ-(d) disposition; if unchanged, the
  `reads.go:510–512` byte-for-byte promise and the §42 fixture posture stay literally true.
  [S15.3; S15.5]
- **R14 — index honesty.** The latest-`intake.state`-per-task probe must be EXPLAIN-checked
  (the §38 D3 full-index-name pinning precedent). Candidate indexes exist: 0015
  `run_events_run_type_idx (run_id, type, event_seq)` and 0001 `run_events_run_idx`; `runs`
  has no task_id index — the task→runs hop is a scan of a small table (the existing
  `taskListRun` does the same); a new index enters only through migration 0022 and only on
  EXPLAIN evidence. [S14.10 (indexing bullet); CONVENTIONS §38 D3/D8]

## 4. Seams to respect

- **The §40-B wall (as pinned in code):** `internal/api` ↛ `internal/project`, ↛
  `internal/broker` (`acceptapi_test.go:527+`). The fallback seam for reading the pin from
  the api layer is SQL + `json_extract` over `run_events` joined through `runs` — the
  reads.go:1192 decision-derive precedent — never a call into the intake pipeline from the
  read path and never a registry read.
- **The one-writer connection / cursor rule (S02.1, §38):** the pool is ONE connection;
  list handlers close cursors explicitly before per-row decorates; no long-lived readers.
  A correlated-subquery view keeps everything inside the one SELECT — the safest shape.
- **`internal/history` freeze posture:** the package was byte-unchanged through B6-1 and its
  drains; under option A its `View.Description` strings for `task_project`/`cost_per_project`
  (layer0.go:97,137) and the layer0_test 0016-file scan need honest updates — a deliberate,
  named touch, not a drive-by (list in §6, coordinator sees it in OQ-(a)).
- **The golden-fixture tie (§42):** `apifixtures_test.go` asserts fixtures against the live
  handler; regeneration is env-gated. Expected stable (the seed's `intake.state` payload has
  no `registry` key) — verify, and regenerate deliberately if a legitimate value changes.

## 5. ⚙ settings

**None.** This packet introduces no key, consumes no new key, and S18 ratifies no key for
read-side attribution. Any number that appears (caps, limits) is an existing structural
constant. (Say so in the packet report; the standing settings-tab directive notes interim
constants.)

## 6. Files expected to change

**Option A — migration 0022 (view re-create):**
- `internal/storage/migrations/0022_pin_attribution.sql` (name indicative): DROP VIEW
  `task_project`, `cost_per_project`; re-CREATE `task_project` as
  COALESCE(claims-collapse, pin-edge) where the pin edge is a
  latest-`intake.state`-per-task `json_extract(payload, '$.registry.project')` subselect
  (non-empty only); re-CREATE `cost_per_project` over it with a TRUE `linkage` note.
- `internal/preview/conformance_test.go` — the `>= "0022"` guard moves to `0023`.
- `internal/conformance/conformance_test.go:116`, `internal/evals/evals_test.go:37` —
  `user_version` pins 21 → 22.
- `internal/history/layer0_test.go` — the no-money scan extends to the 0022 file (or scans
  both); `internal/history/layer0.go` — the two Description strings updated to name the pin
  edge (registry metadata otherwise unchanged: still `OwnerColumn: ""`).
- `internal/api/reads.go` — likely UNCHANGED (both sites already read the view); comment
  updates only (reads.go:572–574, 716–718 name the claims-only edge).
- New tests per §8; fixtures verified/regenerated per §4.

**Option B — api-layer fallback only:**
- `internal/api/reads.go` — both sites gain one shared COALESCE(claims, pin) SQL expression;
  0016 and `internal/history` byte-untouched; `cost_per_project` and the assistant keep
  claims-only linkage (the defect stays visible on the operator's history/assistant surface —
  weigh in OQ-(a)).
- New tests per §8; fixtures verified.

## 7. Adopted components

None. No new dependency under either option.

## 8. Acceptance-test SPECIFICATIONS (executor materializes red-first; NONE committed this round)

Seeding shape: direct INSERTs in the api/history test harnesses (the apifixtures precedent) —
task row, `<task>.intake` run row, `run_events` row `type='intake.state'` whose payload embeds
`{"registry":{"project":"proj-a","ref":"…"}}`; NO `artifact_claims` rows for the pre-approval
cases. Control task with payload lacking `registry`.

1. `TestTaskListServesThePinBeforeApproval` (internal/api/reads_test.go) — GET `/api/tasks`
   as the owner: the pinned task's `project == "proj-a"` with zero `artifact_claims` rows;
   the unpinned control serves `"(no project)"`. Non-tautological: the pinned task's row is
   PRESENT and asserted by id.
2. `TestTaskListProjectFilterMatchesPinnedUnapproved` — `?project=proj-a` returns the pinned
   task and not the control; `?project=(no project)` returns the control and not the pinned
   task (R4).
3. `TestTaskDetailLineageAgreesWithTheList` — GET `/api/tasks/{task}`:
   `lineage.project == "proj-a"`, `project_choices == 1`, and the value equals the list's for
   the same task in the same world (R5).
4. `TestApprovalDoesNotFlickerAttribution` — seed the pin, assert `proj-a`; then insert the
   approval-time claim row (`project='proj-a'`, mode W, active); re-read: still `proj-a`,
   `project_choices == 1` (R6).
5. `TestNoProjectBucketStaysHonest` — a task with an `intake.state` payload whose
   `$.registry.project` is empty/absent, and a task with a claim row `project=''`, both serve
   `"(no project)"`; neither inherits another task's pin (R2).
6. `TestCostPerProjectAccountsForAllMoney` (per OQ-(b) disposition; internal/history or api
   harness) — with a receipt-bearing run on a pinned-unapproved task present:
   `SUM(cost_per_project.priced_usd)` (incl. the bucket) equals `SUM(cost_per_run.priced_usd)`
   and total run counts match (R7). If B1 (money follows the pin): the run lands in the
   `proj-a` bucket. If B2 (money stays claims-only): it lands in `(no project)` and the
   SPLIT-VIEW variant is asserted instead.
7. `TestPinDerivedProjectRedactsAtTheEdge` — plant `sk-ant-…` as the pin value in a synthetic
   payload: list AND detail serve `[REDACTED:…]`, the stored `run_events.payload` keeps it
   verbatim (R10; the §38 D10 shape — fails when the redact call is removed).
8. `TestTaskProjectStaysOperatorOnly` (option A) — the history registry bit is unchanged:
   member `SelectView("task_project")` refused, transport answers 403 via the metadata
   pre-check; operator answers (R11; pins the D5 behavior against the re-created view).
9. `TestPinProbeSeeks` (option A) — EXPLAIN over the view's latest-per-task probe names the
   full index used (the §38 D3 precedent); a scan on `run_events` is a fail (R14).
10. `TestMigrationLedgerAdvances` (option A) — `user_version == 22`, 0001–0021 byte-untouched
    (sha over files), the re-created views parse and the money-scan covers the 0022 text (R8).
11. Fixture check — `TestWebAPIFixtures` stays green without regeneration, or the regenerated
    diff is reviewed and committed deliberately (§4).

## 9. Acceptance checklist (the evaluation rubric)

- [ ] Pinned task serves its project on `GET /api/tasks` from birth, pre-approval (R1).
- [ ] `(no project)` honest + selectable for genuinely unmatched tasks (R2).
- [ ] `artifact_claims` writers and S02.8 mechanics byte-untouched (R3).
- [ ] Filter parity, one expression (R4); list/detail agree (R5); no flicker at approval (R6).
- [ ] Money semantics match the coordinator's OQ-(b) disposition; sum preservation asserted;
      `linkage` note true (R7).
- [ ] 0001–0021 untouched; view change (if any) is migration 0022 with version pins and the
      preview guard moved (R8).
- [ ] §40-B AST pin green; single-connection cursor discipline kept (R9).
- [ ] Pin value redacts at both serving edges; stored rows verbatim (R10).
- [ ] `task_project` still operator-only in the history registry; owner scope unchanged (R11).
- [ ] No new ⚙ / dependency / event type (R12); board SSE per OQ-(d) (R13); EXPLAIN pinned (R14).
- [ ] Golden fixtures verified (regeneration, if any, deliberate).
- [ ] Full battery green (`go test ./...` + web tests); the RW-2-landed surface untouched.

## 10. Binding CONVENTIONS constraints

§5 (commit discipline), §6 (migration immutability; append-only; event-payload discipline),
§7 (API/SSE shape), §14 (intake conventions — claims at approval, state event-sourced),
§37 + §37-C (weakness stated in ONE place; money is READ; honest absences; operator-only
no-owner-column views; Layer-2 allowlist = Layer-0 registry), §38 (owner scope fail-closed;
single connection; redaction edge enumeration; EXPLAIN full-name pinning; no money computed
in api; absences rendered), §42 (golden-fixture tie; REST-is-truth board wiring).

## 11. Open questions — NOT resolved here; the coordinator dispositions each

- **OQ-(a) Fix altitude: migration-0022 view re-create vs api-layer-only fallback.**
  *For 0022:* the linkage weakness stays stated in ONE place (§37's own discipline — an
  api-layer COALESCE puts linkage logic in a second place); ALL five view consumers heal at
  once, including the operator's Layer-0/Layer-2/assistant surfaces and (per OQ-(b))
  `cost_per_project`; reads.go likely needs no query change. *Cost:* migration ceremony —
  preview guard move, two version-pin moves, layer0_test scan extension, two honest
  Description edits in the otherwise-frozen `internal/history`, and the money view changes
  implicitly unless OQ-(b) chooses the split. *For api-only:* zero migration ceremony, history
  frozen, money semantics untouched by construction. *Cost:* the same task answers "proj-a"
  on the board and "(no project)" to the assistant and on every cost view — the defect is
  half-fixed and the ONE-place discipline is broken. Grounding lean (not a decision): 0022 is
  the coherent altitude; pin and claim share one source value so the COALESCE is purely
  temporal completion.
- **OQ-(b) Money attribution semantics.** B1 — money follows attribution: a pinned task's
  intake-ceremony spend lands under its project pre-approval (coherent with "what did this
  project cost", includes never-approved intakes; the `linkage` note names the pin edge).
  B2 — money stays claims-derived: requires SPLITTING the view (a linkage view for the read
  surfaces, a claims-only edge for `cost_per_project`) — two places for one weakness, more
  ceremony, but approval-gated money. B3 — defer money entirely (= api-layer option for the
  lists). Note: under A+B1 nothing double-counts (COALESCE yields one row per task; sum
  preservation is asserted by spec-test 6).
- **OQ-(c) Served-field provenance.** One indistinguishable `project` (grounded fact: pin and
  claim can never disagree, so a `pin|claim` discriminator encodes only "approved yet?" —
  already visible as kanban/stage) vs an explicit `project_source` field or a view-level
  provenance column (the 0016 `linkage`-note idiom; could ride the view without reaching the
  wire). Cheapest honest shape: indistinguishable on the wire, provenance named in the view
  comment/note only.
- **OQ-(d) The `board` SSE snapshot.** Stay as-is (grounded: the SPA renders from REST, frames
  are triggers, per-topic snapshots are DOCUMENTED face-incomplete including project, and
  `intake.state` is already in `boardEventTypes`, so attribution is live at birth with zero
  SPA change; the reads.go:510–512 byte-for-byte promise stays true) vs add the field (no
  rendering consumer exists today; it would touch the §42 posture and the promise comment for
  no observable behavior change). Grounding lean: stay as-is.

## 12. Process notes (coordinator-facing)

- **No spec conflict found.** S06.2 puts the registry match at triage; S02.8 puts claims at
  approval; S15.2 lists project lineage on the tasks family; S14.10's indexing bullet
  sanctions views over event JSON (0016 already ships two: `limit_event_history`,
  `routing_quality`). The 0016 RECORDED READING ("only populated relational edge") was true
  when written and stays true — the pin is a payload edge; a 0022 view lifting it is an
  honest extension, not a contradiction.
- The packet-cut wall phrasing is corrected by §1: the AST pin forbids `internal/project` +
  `internal/broker`; `internal/intake` is a landed import of `internal/api`.
- Executor sequencing: this packet runs strictly after the RW-2 drain lands (STATE
  2026-08-10); red tests first, then the fix, then the battery.
