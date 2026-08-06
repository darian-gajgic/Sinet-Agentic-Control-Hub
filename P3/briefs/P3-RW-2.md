# P3-RW-2 grounded brief — the projects HTTP family (S13.7 over S15.2)

Grounded 2026-08-06 against the live tree (post-P3-RW-1, post-B6/UI batch) and `Spec/drafts/` (canonical).
Origin: product map v3, `P3/design/product-map.md` §5 (▲ v3 paragraph) + §3 Projects — checkpoint-2 grounding: **no `/api/projects` route exists at all** (re-verified this session: the §2 route table in `internal/api/api.go` `Handler()` registers none, and the three committed red tests below all fail on the mux's own `404 page not found`).

The packet: **(a) the read door** — list/detail the S13.7 registry entries visible to the caller; **(b) the create/onboard door** — start the S13.7 onboarding flow over the existing stage seam. Additive S15.2 posture. Task-derived aggregates (counts, spend) are **out of scope** — client-side over already-served rows.

## 1. What exists (verified in code this session)

- `internal/project` — the S13.7 registry is complete: `Register`/`Get`/`List`/`Capture`/`Activate` (`registry.go`), pending→active lifecycle (one-way, trigger-enforced), immutable versioned captures, `visibleTo` (owner-or-member) + `PinForIntake` with the one-refusal discipline (`noSuchPin`: unknown id and invisible entry are the SAME `ErrNotFound`; visible-but-pending is `ErrNotActive` — the requester may know that honestly). `Onboard`/`OnboardStart`/`OnboardApprove`/`Rescan`/`DriftCheck` (`onboard.go`): register → clone/init → scan → draft, idempotent under re-dispatch (an existing entry returns its current drafted capture, never re-clones).
- `internal/stage/onboard.go` — the run-substrate half is BUILT: `Skeleton.StartOnboarding(ctx, owner, projectID, name, source) (taskID, error)` runs `OnboardStart` synchronously, creates task `onboard-<project>` + run `onboard-<project>.onboard`, enqueues; `dispatchOnboard` surfaces the draft on durable ask `onboard:<project>` and parks; `AnswerOnboarding` (D10 owner-only, `errOnboardNotOwner`→403) activates and completes. The ANSWER path is already served: `Surface.Answer` routes `IsOnboardAskID` asks (`surface.go:145–154`) via the existing `POST /api/asks/{ask}/answer`; the card lists in `GET /api/approvals` as an untiered ask reading Medium (§39 ruling).
- Seam wiring: `internal/shell/shell.go:520–525` closes `OnboardStart`/`OnboardApprove` over `internal/project`; `internal/shell/project_seams.go:88–91` is the sentinel-translation precedent (shell maps `project.ErrNotFound`/`ErrNotActive` onto consumer-side typed errors — the import walls stay clean).
- `internal/api` — never imports `internal/project` (AST scan `acceptapi_test.go:528`, §40-B): the registry is read as bounded SQL off `repo_registry` — the landed helper is `visibleProjects` (`memory.go:753`, owner+members columns, `projectRegistryCap` bound). Identity: `IdentityFrom` + `requireIdentity`; errors: `*SurfaceError` type-mapped; reads: `writeReadJSON`, `readLimit`, `readPageDefault`, `head()` cursor; shape precedent for a content family: `memory.go` (list + detail + visibility sentence + cursor).
- Scheduler edge that shapes OQ7: `scheduler.Enqueue` refuses any run not `new`/`queued` — a second `StartOnboarding` for the same project errors at the enqueue step while the first onboarding is parked (and after completion), even though `OnboardStart` itself is idempotent.
- `OnboardInput` carries **no Members field** — onboarding over the existing seam creates member-less entries (bears on OQ9).

## 2. Requirements (each with its S-ref)

- **R1 — a new additive resource family under `/api/projects`** (S15.2; S13.7 is the owning section). The S15.2 family table has no projects row; the landed resolution shape is the `/api/chat` / `/api/push` precedent (§44/§46): the family exists under S15.2's contract RULES — owner-scoped server-side, every mutation on the event log, additive-first, no `/v1`, no outward-effect verb — and the cosmetic S15.2 table amendment naming the row is **carried to the gate list, never edited by the packet** (§8; S00.9).
- **R2 — the list door is caller-scoped server-side with ONE predicate expression** (S13.7 "owning user, invited members"; S15.2 "the browser is a display, never an authority"; S01.9 owner-scoped accessors). Owner-or-member, the `PinForIntake`/`visibleTo` discipline re-expressed as SQL in the api layer (the §40-B wall). The predicate must exist ONCE in `internal/api` and be shared by list and detail (the `memoryVisible` delegation / drain-D3 rule: a list and a detail cannot disagree structurally) — extending or reusing the landed `visibleProjects` read is the expected shape. Session-required; bounded (`projectsListCap = readPageDefault`, structural, reason named); cursor drained/closed explicitly before any per-row derive (§38 single-connection).
- **R3 — the detail door refuses with ONE answer, 404-before-403** (S15.2; the landed `noSuchPin` + §38 discipline): an entry the caller cannot see answers EXACTLY like an id that does not exist — same status, no distinguishing text; no captured content on any refusal.
- **R4 — the S13.7 capture is the served content** (S13.7; S06.2 names what it feeds; product map §3: cards need capture summary/version, conventions, commands, danger zones). The owner reads the full current capture (conventions, commands incl. preview slot, danger zones with path/action/rule, scan hash, captured-by/ts, version) plus entry facts (name, owner, members, state, default branch, remote URL presence, protected refs, created/updated). Member depth and list-vs-detail split are OQ4/OQ1. Content is served UNWRAPPED (§38 ruling (b): S13 product read from its OWN tables, not a run_events projection — `redactionEdgeFiles` stays at three).
- **R5 — the create door starts onboarding over the EXISTING seam** (S13.7 "onboarding a repository is itself a task the platform performs"; D10; 15.6). `POST /api/projects` reaches `Skeleton.StartOnboarding`; the owner is the SESSION identity, never a body field (the `handleIntakeSubmit` precedent). The response carries the drafted PENDING entry (read back through the R2/R3 read path) + the onboarding task reference + the approval-ask reference (see §3 note: the ask row lands at DISPATCH, asynchronously — the response must be honest about that, never fake an open card).
- **R6 — no second activation door, no outward-effect verb** (D7; S15.2). Activation stays exclusively on the landed `POST /api/asks/{ask}/answer` → `IsOnboardAskID` path; this packet routes NO approve verb, NO rescan verb (S13.7 "re-scan on demand" is real but out of this packet's scope — a stated deliberate absence), NO member-management verb, NO delete. Nothing here proposes or executes an outward effect.
- **R7 — the create door is retry-safe** (S15.2 "writes are retry-safe … a repeated answer returns the already-resolved state — a phone retry can never double-fire"; `OnboardStart`'s idempotency comment). Binding invariant regardless of OQ7's resolution: a repeated POST for the same project id never re-clones, never creates a second task/run, never corrupts the pending entry. The transport disposition (200-with-existing vs 409-with-reference) is OQ7.
- **R8 — errors map on TYPE through `*api.SurfaceError`, never on message text** (§38's ban; §40-C shape). Project-store sentinels cross the seam via shell translation (the `project_seams.go` precedent) or an equivalent typed path — `errors.Is` on sentinels, with the honest split: invisible/unknown → 404 `not_found`; visible-but-relevant-state → 409 with a named code; caller fault → 400 naming the field (§39-B drain-D6); unmarked → 500 with the ops log carrying the cause. (The landed `mapOnboardErr` contains one `strings.Contains` limb — a landed wart, NOT a license.)
- **R9 — nothing is co-minted** (S15.2 "every mutation lands on the event log" is satisfied by the LANDED producers; §39 OQ8 double-mint ban). The registry's own `registry.registered`/`registry.captured` events + the task/run lifecycle rows ARE the audit; `internal/api` mints no event, writes no registry SQL mutation, and the every-mutation walk counts **zero `decision.recorded`** and zero api-minted `registry.*`.
- **R10 — no aggregates, no money** (product map §5 ▲ v3: counts/spend stay client-side; §38 R8). No task counts, no spend figures, no receipts joined into any projects response; the landed `TestNoMoneyIsComputedInTheAPI` scan must pass over the new file unchanged.
- **R11 — ⚙ posture: NONE consumed** (S01.10; §23 "this packet declares NO new ⚙ key" carried forward). See §5.
- **R12 — no migration, no dependency** (S02.1 §6; S16). `repo_registry`/`repo_registry_captures` landed in 0008; `user_version` does not move; `components.lock` count unchanged; `go.mod` unchanged.
- **R13 — the S15.12 completeness machinery is honored, with coordination** (S19.5 "the SPA consumes every API"; §46). New routes drift `web/src/fixtures/api/route-inventory.json` (generated by `TestRouteInventoryFixture` from the mux's own registration calls) and face `web/src/sweep.test.tsx`'s consumed-or-excepted check. See §12 — this is a coordinator-sequenced touch, not a silent one.
- **R14 — the test posture of §38 applies to every route**: three-way cross-owner tests (owner / other member / operator — operator direction per OQ3's resolution, pinned in BOTH directions), non-tautological presence assertions, 404-before-403 probes, and the property sweep of §8.

## 3. Proposed route shapes (S15.2/S15.3 grounding; field NAMES are proposals — the committed red tests assert substrings, not shapes)

**`GET /api/projects`** → 200
```json
{
  "projects": [ { "project_id": "...", "name": "...", "owner": "...", "members": ["..."],
                  "state": "pending|active", "capture_version": 2, "default_branch": "main",
                  "has_remote": true, "created_ts": "...", "updated_ts": "...",
                  "capture": { "...": "per OQ1 — summary on the list, full on the detail" } } ],
  "visibility": "one sentence naming the rule that produced the set (memory-family precedent)",
  "cursor": 12345
}
```
S15.3: `cursor` = event head for snapshot-then-tail. Order: active-first then id (the store's `List` order) or by name — executor's, stated in code. Registry-fed danger zones/conventions on the LIST row are OQ1.

**`GET /api/projects/{project}`** → 200 `{ "project": <entry + full current capture + protected_refs>, "cursor": N }`; refusals per R3. `store_path` is proposed NOT served (a host filesystem path has no client use — S15.2 the browser is a display; OQ4 records the fork).

**`POST /api/projects`** body `{ "project_id": "p-new", "name": "New Proj", "source": "", "remote_url": "" }` → 200
```json
{
  "project": { "<the pending entry with its drafted capture v1>" : "..." },
  "task_id": "onboard-p-new",
  "ask_ref": "onboard:p-new"
}
```
Grounding: `StartOnboarding` returns the task id; task `onboard-<id>` and ask `onboard:<id>` are the landed deterministic names (`internal/stage/onboard.go:36–39`). **The ask ROW lands when the scheduler dispatches the run** — the response names the reference and must not claim an open card exists yet (the card arrives in `GET /api/approvals`; answering it is the landed activation door). `project_id` client-supplied vs derived, `source` semantics, and status-code choices are OQ9/OQ5/OQ6. 200 (not 201) is the family-wide precedent — no landed route answers 201.

## 4. Seams

- **Consumed (existing, untouched semantics):** `Config.OnboardStart`/`OnboardApprove` (stage ⇄ project, `stage.go:309–315`); `Skeleton.StartOnboarding` + `dispatchOnboard` + `AnswerOnboarding`; the ask machinery; the scheduler `Enqueue` ingress; the api identity middleware; `visibleProjects`-style bounded registry SQL.
- **One genuinely new seam, flagged as required:** the api layer has no door to `Skeleton.StartOnboarding`. The grounded shape: a narrow api-side interface (the `api.IntakeSurface`/`CancelSurface`/`ResumeSurface` precedent) implemented by `*stage.Surface` with a transport-shaped `StartOnboarding` method (stage-side error mapping beside `mapOnboardErr`), held as a new `api.Config` field and wired at the composition root beside `intakeSurface = surface` (`shell.go:584–592`). This is transport plumbing over the existing capability seam — no new capability crosses any wall. Project-sentinel translation for R8 rides the shell closure (the `project_seams.go:88–91` precedent) or a stage-side sentinel set; NOT string matching.
- **No new stubs needed**; no phase-not-yet-come seam is touched.

## 5. ⚙ settings consumed

**None.** No S13.7/S15.2 text ratifies a ⚙ key for this surface and the registry declares none — the settings index (`internal/settings/index.go`) stays **byte-unchanged** (tally proof). Structural bounds with named reasons only: `projectsListCap = readPageDefault` (one read surface, one page size — the §38/§40 precedent); interim under the standing settings-tab directive.

## 6. Files expected to change

- `internal/api/projects.go` — NEW: the family (handlers, wire shapes, the single visibility predicate, error mapping).
- `internal/api/projects_test.go` — committed RED this grounding (see §8); the executor extends it (env wiring + the §8 to-write tests) without weakening the committed assertions.
- `internal/api/api.go` — the new Config field + route registrations (`GET /api/projects`, `GET /api/projects/{project}`, `POST /api/projects`) in `Handler()`.
- `internal/stage/surface.go` and/or `internal/stage/onboard.go` — the api-facing `StartOnboarding` transport method + typed error mapping (sanctioned: stage is the Surface's home; the walls of §23 are untouched).
- `internal/shell/shell.go` (± `project_seams.go`) — one wiring assignment + any sentinel translation.
- `web/src/fixtures/api/route-inventory.json` — REGENERATED Go-side output that lives under `web/` — **coordinator-sequenced, see §12.**
- Nothing else. `internal/project` is expected byte-untouched (any widening — e.g. OQ9 members — is sanctioned-narrow ONLY via the coordinator).

## 7. Adopted components

**None.** No new Go module, no npm change, `components.lock` untouched.

## 8. Acceptance tests

**Committed RED this session** (`internal/api/projects_test.go`, Amendment-A §3: `go build ./...` green, red is the packet's own paths, window declared in the commit). Evidence — run this session: all three fail on the mux's own `404 page not found` body; the full `go test ./internal/api` run shows exactly these three failing and nothing else; `go test ./internal/project` green.

1. **`TestProjectsListIsCallerScoped`** — seeds `p-alpha` (alice owner, bob member, ACTIVE, real capture) + `p-gamma` (carol owner, ACTIVE) at the data layer (Register→Capture→Activate, the `pin_test.go` shape — no git). Asserts per caller {alice, bob, carol}: `GET /api/projects` is 200, the caller's own entry PRESENT, the other's ABSENT (S13.7/S15.2; both directions non-tautological).
2. **`TestProjectDetailServesCapturedContentAndRefusesWithOneAnswer`** — owner detail 200 carrying the seeded convention, command and danger-zone path (S13.7 capture is the card's content); member 200 on the ENTRY (depth deliberately unasserted — OQ4); stranger-on-existing and stranger-on-unknown BOTH 404 (one answer, no oracle) with no captured content on the refusal.
3. **`TestOnboardStartDoorDraftsAPendingEntryWithItsOnboardingTask`** — `POST /api/projects` as alice with `{project_id, name}` is 200 naming the entry; afterwards the STORE holds `p-new` PENDING with `capture_version ≥ 1` owned by alice, and the `tasks` row `onboard-p-new` exists owned by alice (S13.7; 15.6). The env builder's `server()` is the executor's ONE wiring seam; the DB-level effects may not be faked — the sanctioned green shape is the real seam (a real skeleton rig or the shell-shaped closure over the real `project.Store`).

**Executor-written (from spec text; exact assertions bind once OQs resolve):**

4. `TestPendingEntriesInTheOwnersOwnList` — per OQ2's resolution, both directions pinned (a pending entry of ANOTHER person never appears regardless).
5. `TestProjectsOperatorDirection` — per OQ3, pinned BOTH ways (the §40-C/§44 two-direction shape: presence for whoever may see, absence/404 for whoever may not).
6. `TestOnboardStartRepeatDoesNotDoubleFire` — R7: second POST same id → no re-clone (store dir mtime/capture_version unchanged), no second task/run row, disposition per OQ7; assert via DB + store.
7. `TestOnboardStartSourceSemantics` — per OQ5: the accepted form onboards from a local fixture repo (the `prepareProject` git shape) with the scan's draft reflecting its content, and the refused forms answer 400 with the reason; nothing network-shaped is ever dialed (hermetic env, §23).
8. `TestOnboardStartRefusesWhatItMust` — missing name/project_id → 400 naming the field; already-registered id → per OQ6 with NOTHING cloned (probe: no store dir); dev identity per OQ8.
9. `TestOnboardApprovalStillActivatesThroughTheOneDoor` — regression at the transport: after dispatch, the ask is in `GET /api/approvals`, `POST /api/asks/onboard:<id>/answer` (owner) activates, and the projects detail now reads `active`; a non-owner answer stays 403 (D10). No new route participates.
10. **Property-based (marked, the P3-RW-1 `pin_test.go` sweep precedent lifted to the transport):** `TestProjectsVisibilitySweepHTTP` — exhaustive requester-class {owner, member, stranger, operator} × entry-state {pending, active} × route {list, detail}; the universal invariant: **no answer ever contains an entry the caller neither owns nor belongs to** (operator limb per OQ3), and every refusal is the one-answer 404. Spec-stated invariants suited to property treatment: the R2/R3 predicate (S13.7/S15.2) and the R7 no-double-fire invariant (S15.2).
11. `TestRouteInventoryFixture`/sweep disposition green per §12; the landed §40-B import scan, R8 money scan, and never-percent scan pass over the new file.

## 9. Acceptance checklist (the evaluation rubric)

1. `go build ./...` + `go test ./...` green; the three committed red tests pass UNMODIFIED in their assertions (env wiring may change; assertion edits are a FAIL).
2. Routes exist exactly as landed in `api.go` `Handler()` with `record()` entries; no `/v1`; all session-required.
3. Visibility: one predicate expression in `internal/api`, shared by list+detail (structural, not two copies); three-way cross-owner tests on EVERY route; presence direction non-tautological.
4. 404-before-403 + one-refusal: stranger vs unknown indistinguishable (status AND body class), probed.
5. The create door drives the REAL register→clone/init→scan→draft path; pending entry + capture v1 + task row observable; response carries entry + task/ask refs honestly (no claimed-open card).
6. No second activation door: route table carries no approve/rescan/member/delete verb under `/api/projects`; activation regression (§8 test 9) passes.
7. R7 idempotency probed (no re-clone, no duplicate rows).
8. Error mapping on sentinels/types only — no `strings.Contains` on error text in any NEW code.
9. Zero api-minted events; the every-mutation walk over the POST counts zero `decision.recorded`; registry events counted are the store's own.
10. No money/aggregates in any projects shape; landed R8 + never-percent scans green over the new file.
11. `internal/settings/index.go` byte-unchanged (tally green); no migration (`user_version` unmoved); `components.lock` count unchanged; §40-B import scan green (api still never imports `internal/project`/`internal/broker`).
12. Content served UNWRAPPED with `redactionEdgeFiles` still exactly three.
13. Route-inventory fixture regenerated; web-sweep disposition recorded per §12 with the coordinator's sanction noted in the packet report.
14. Red window closed and declared in the implementation commit message; every OQ resolution recorded in the code where it lands (comment citing this brief's OQ number).

## 10. Binding CONVENTIONS constraints

§2 (module path, stdlib-first, ⚙ discipline, sentinel errors where callers branch); §3 (stdlib testing, hermetic `t.TempDir()`, Amendment-A red-window mechanics — scope: this packet's own paths, build stays green); §5 (commit subject `P3-RW-2: … (S13.7, S15.2–S15.3)`, explicit pathspec staging, never push, fences); §7 (unversioned API, identity middleware, SSE cursor semantics); §14 (the intake pin edge this family sits beside — untouched); §23 (project-package walls: api never imports it; stage never imports it; hermetic git; registry/onboarding semantics); §38 (404-before-403, `authorizeOwner`-shape scoping, single-connection cursor discipline, type-mapped errors, R8 money scan, redaction-edge enumeration, honest absences); §39 (double-mint ban; the onboarding card's Medium reading; step-up NOT demanded — registry activation releases nothing outward); §40-B/§40-C (held-dependency vs SQL-read walls; content-vs-observability visibility lines); §46 (route-inventory + sweep machinery).

## 11. Open questions — NOT resolved here; each needs an operator/coordinator disposition

- **OQ1 — list-vs-detail split.** What rides a list row vs the detail? (a) list = entry facts + `capture_version` + a summary (counts of conventions/zones, the test command), detail = full capture — the S15.3 snapshot-economy reading; (b) list carries the full capture — household scale (a handful of projects, `projectRegistryCap` exists because "expectation is not a bound"). Bearing: S13.7 (field census), product map §3 (cards show "capture summary/version, conventions, commands, danger zones" — ambiguous between row and expanded card), S15.3 snapshot-then-tail.
- **OQ2 — do PENDING entries appear in the caller's OWN list?** (a) Yes, with `state:"pending"` — the honest lifecycle; `ErrNotActive`'s "a state the requester may know honestly" reading, and the Projects tab needs to render an in-flight onboarding card; (b) active-only — S13.7 "the entry feeds nothing until active" read as a visibility rule. Bearing: S13.7 lifecycle; `PinForIntake` doc; product map §3 (create/onboard door lives ON this tab — an invisible in-flight onboarding is a worse surface). Another person's pending entry is invisible under EVERY reading (R2).
- **OQ3 — does the operator see all entries?** (a) Owner/member only — the packet origin says "same predicate discipline as `PinForIntake`", which has no operator limb; the §40-C/§44 content line (registry captures are project content). (b) Operator sees all — the registry is audited platform machinery like the workforce roster (§30's observability line; "a worker is audited machinery gated by D10 acts and not personal content" — an S13.7 entry is arguably the same kind of thing), and D10/S01.9's one role bit implements co-approval over platform objects. Bearing: S01.9 AuthZ ¶; §30; §40-C OQ5; §44 OQ1. Both directions must be test-pinned whichever way it falls.
- **OQ4 — member-vs-owner detail depth.** Do members read the full capture (danger zones, commands, conventions)? (a) Yes — S13.7 injects exactly this content into stage briefs of EVERY run in the project including members' runs (S06.2/S05), so hiding it from the member the platform shows it to is theater; (b) owner-only capture, members get entry facts + summary. Sub-fork either way: `store_path` (a host filesystem path) and `protected_refs` — proposed not served / owner-only. Bearing: S13.7 feeds; S15.2 server-side visibility; S11 host-hygiene posture.
- **OQ5 — what may `source` be at v0?** (a) A local host path (what `OnboardInput.Source` is: "a LOCAL fixture repo path"; `GIT_ALLOW_PROTOCOL=file` and the §23 never-dials posture make anything else impossible today) — **but over HTTP this is a host-filesystem read primitive**: any path the `sinet` user can read becomes cloneable into a project store and then readable through this family's own read door (S11 confinement bearing; S01.9 trusted-household mitigation); (b) empty-only at v0 — every onboarding creates a fresh store (S13.7 "a v0 project without a repo gets one created/registered"); safest, and the Projects-tab journey needs nothing more; (c) accept a git URL as STORED `remote_url` data, never dialed (S13.7 remote is data; §23), with clone-from-remote a later, brokered packet. Candidates may compose (empty-or-stored-URL now, path behind an operator-only limb). Bearing: S13.5/S13.7, §23 hermetic-git rules, S11, S02.10.
- **OQ6 — error semantics + status codes.** Already-registered project id → 409 `already_registered` (the `ErrAlreadyRegistered` sentinel; a conflict the caller resolves by reading) vs 400; store-path collision (`ErrBadInput` today) → 409 vs 500-class; visible-but-pending detail interactions; create success 200 (family precedent) vs 201. Bearing: S15.2 retry-safety text; the landed `mapIntakeErr` table (404 `not_found` / 409 `project_not_active` / 400 `bad_body`).
- **OQ7 — idempotent re-onboard transport behavior.** Today a second `StartOnboarding` fails at `Enqueue` ("cannot enqueue run in state parked/completed"). (a) Transport pre-reads the entry: pending → answer WITH the existing entry + task/ask refs (200 already-in-flight, the S15.2 "repeated answer returns the already-resolved state" reading) or 409 carrying them; active → 409 `already_active`; (b) surface the enqueue refusal as a plain 409; (c) sanctioned-narrow stage change making `StartOnboarding` idempotent end-to-end (skip enqueue when the run is past queued) — touches landed stage code, coordinator's call. Bearing: S15.2 retry-safe writes; `OnboardStart`'s comment (idempotency there is a RE-DISPATCH reading, not an HTTP-retry sanction).
- **OQ8 — the dev identity at the create door.** The project store has no users-table wall (unlike the memory gate's `ErrNotHuman`): a dev-posture POST would mint rows owned by `dev`. (a) Refuse the dev identity on POST (the settings-family dev-refusal limb; 15.6 attribution demands a real person); (b) allow it — dev-mode demo ergonomics (intake Submit accepts the dev identity today). Bearing: 15.6 owner attribution; S01.9; §40 OQ8; §7 dev-posture definition.
- **OQ9 — the onboard-start request shape.** `project_id` client-supplied (what the seam takes; the red test's shape) vs server-derived slug-of-name (then the id must come back in the response and collisions need semantics); `name` required (the seam requires it); `remote_url` accepted-as-data now or not at all; **members-at-create is impossible over the existing seam** (`OnboardInput` has no Members) — (a) ship member-less creation, members arrive with a later verb/packet; (b) sanctioned-narrow widening of `OnboardInput`+`Onboard` (touches `internal/project` — coordinator's call). Bearing: S13.7 field census; §23; the F16e collision-free `pathComponent` rule (ids are stored verbatim, paths are hashed — any id shape is safe on disk).

## 12. Process notes (coordinator-facing)

- **The route-inventory fixture crosses the web fence.** `TestRouteInventoryFixture` (internal/api) compares against committed `web/src/fixtures/api/route-inventory.json`; adding routes turns that Go test red until regenerated, and regeneration WRITES under `web/` — fenced for this packet, with a frontend builder concurrently active there. Additionally `web/src/sweep.test.tsx` requires every served route to be consumed by the SPA or named on its exception list with a reason (>40 chars). Neither file is this packet's to edit silently. **Referral:** the coordinator sequences (a) sanctioning the executor's single-file fixture regeneration (it is Go-generated output that happens to live under `web/`), and (b) either the concurrent frontend packet consuming the three routes or an exception-list entry landed by whoever owns `web/` that phase. The grounding stage touched neither.
- **Red window (Amendment-A, declared):** `internal/api/projects_test.go` is committed FAILING with this brief — three tests, all red on the absent routes (mux 404), `go build ./...` green, no non-test file touched. The packet's implementation commit closes the window.
- Git: explicit pathspec staging only; retry on `index.lock`; never push (coordinator pushes). Fences: `web/`, `P3/STATE.md`, `Spec/`, `Docs/`, `Research/` untouched by every stage of this packet.
- Spec conflicts found while grounding: **none.** (The S15.2 family-table gap is the ratified additive-family shape, not a conflict; the `mapOnboardErr` string-match limb and the dev-identity asymmetry are landed warts recorded under R8/OQ8, not spec conflicts.)
