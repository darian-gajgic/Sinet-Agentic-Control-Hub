> **EXPIRED 2026-08-27 (landed).** Single-use artifact — later grounding reads code + spec only. Post-brief corrections: the regeneration needs `TestRouteInventoryFixture` too (R8's single-command claim was wrong); R9's "declaredButUncalled stays `['health']`" was stale — it is `[]` since 2026-08-05 (eval F1).

# P3-LN-10a — the pinnable-lanes read (grounding brief)

**One verb.** Serve `intake.Pipeline.PinnableLanes` — the startup-composed set of lanes a task-creation pin may name, each row carrying the platform's own verdict — as `GET /api/intake/pinnable-lanes`, so the LN-10 lane picker enumerates from the running world and the set it offers IS the set the boundary honors. Grounded 2026-08-27 against the tree at `c0a06fc`+; every file:line below was read this session.

## Why this exists (verified, not inherited)

P3-LN-9 added `Request.PinnedLane` (`internal/intake/intake.go:139-148`, wire key `pinned_lane`) validated at the boundary by `Pipeline.refuseLanePin` (`internal/intake/route.go:174-192`). The set it validates against is composed ONCE at `stage.New`: one `worker.Coverage` value (`internal/stage/skeleton.go:115-122`) feeds both the router and `PinnableLanes: lanePinOptions(coverage)` (`skeleton.go:199`, adapter at `:535-544` over `worker.PinnableLanes`, `internal/worker/routing.go:325-342`). Its ONLY consumers today are the boundary refusal and its message helper (`route.go:178, :198-199`) — **no API serves the set**; the picker would otherwise hardcode a second spelling, which is exactly what §65 ("a rule spelled twice drifts") and §66/§66's one-predicate discipline forbid. The coordinator ratified option A: a new GET under the intake family serving the composed value VERBATIM.

S15.2 check (`Spec/drafts/S15-frontend-api.md`): the family table is contract-level — "full endpoint schemas are P3 work within these bounds"; evolution is additive-first, unversioned lockstep at v0. The tasks family already names the per-task lane pin [S00.9 A13]. No spec text contradicts a new family read; no amendment is needed. Lane names are not secret (`route.go:194-196`, `routing.go:360-363`: they ship in the lane documents; no existence-oracle concern).

## Requirements

1. **Route.** `GET /api/intake/pinnable-lanes`, registered in `Handler()` via `protected(...)` beside `POST /api/intake/requests` (`internal/api/api.go:605`) — per-session auth exactly like every sibling (S01.9 layer 2; S15.2 one-seam). Kebab-case per `/api/meters/plan-budget`. This brief FIXES the spelling; the picker and the boundary need one.
2. **The value is the skeleton's own, never re-derived.** `api.Config` gains `PinnableLanes []intake.LanePinOption` (internal/api already imports internal/intake — `api.go:32`; zero new import edges). The skeleton exposes it (e.g. `func (s *Skeleton) PinnableLaneOptions() []intake.LanePinOption` returning `s.pipe.PinnableLanes`), and `internal/shell/shell.go` (api.New call, `:962-991`) wires that accessor's value. NOTHING may call `worker.PinnableLanes` a second time in production — the adaptation keeps its one spelling at `skeleton.go:535` (§65; §66 D1: the boundary never re-derives, and neither does the transport).
3. **Not `IntakeSurface`.** Do NOT add a method to `api.IntakeSurface`: it would break every fake (`fakeSurface`, `fixtureIntake`), and a surface returning pre-assembled JSON would let the golden fixture pin a FAKE's bytes — vacating the §63-R3 contract. The set is startup-bound data (§65: commissioning is startup-bound, no reload seam), so it crosses as data, the `PlanBudgets`/`Meter` Config-seam precedent.
4. **Wire shape** (snake_case per `pinned_lane`; api-side wire struct in `internal/api/intake_handlers.go`, the family file — `intake.LanePinOption` has no json tags and must not grow any):
   ```json
   {"lanes": [
     {"lane": "anthropic", "pinnable": true},
     {"lane": "local", "pinnable": false, "not_pinnable": "<the platform's own sentence, verbatim>"}
   ]}
   ```
   Top-level OBJECT, never a bare array (additive-first: an array can never grow a sibling). Rows are the composed rows verbatim — `Lane`→`lane`, `Pinnable`→`pinnable`, `NotPinnable`→`not_pinnable` with `omitempty` (a pinnable row carries no empty-string key). The `not_pinnable` sentence is CARRIED, never composed or trimmed here — it is `worker.LanePinRefusal`'s text (`routing.go:295-308`), the same words `refuseLanePin` quotes.
5. **Ordering: the composed order, verbatim — no sort.** `worker.PinnableLanes` emits FlatRateLanes in composition order (skeleton prepends the configured lane, `skeleton.go:116`, then commissioned lanes), deduped, local lane appended last (`routing.go:326-341`). That order is already deterministic per running config AND meaningful (first row = the platform's own flat-rate lane; local last). Sorting would be a second spelling of an order the composition owns, and the handler must not mutate the shared slice in place. The golden fixture pins the order.
6. **Empty is honest, never an error.** Intake surface wired but zero rows → `200 {"lanes":[]}` — a real state the refusal path already speaks about ("none — this platform holds no pinnable lane at all", `route.go:204-205`). The serializer must emit `[]`, never `null` (initialize the slice). No prose is added for the empty state — that sentence has one home already.
7. **Not-wired is the family's 503.** The handler gates on `surfaceReady` (`intake_handlers.go:110-117`): a process with no intake surface answers `503 not_wired`, because serving `[]` there would claim "no pinnable lanes" about a world where Submit itself 503s — the dishonest-absence class this platform refuses everywhere (§65). Responses ride `writeSurface`/`writeSurfaceErr` (Content-Type, `Cache-Control: no-store`) like every sibling.
8. **Golden fixture, exercising BOTH row kinds** (§63-R3: "a member no fixture exercises is a contract nobody agreed to" — `apifixtures_test.go:131`). `fixtureServer` (`apifixtures_test.go:2013`) wires `PinnableLanes` with rows for the world it already speaks: `anthropic` (configured-first), `kimi`, `zai` pinnable; `local` not-pinnable. The rows are produced by the REAL producer — build a `worker.Coverage{FlatRateLanes: [anthropic, kimi, zai], LocalLane: adapters.LaneLocal}` and call `worker.PinnableLanes`, field-copying to `intake.LanePinOption` (a 3-field test-local copy is harness; the SENTENCE is the rule and must never be typed by hand). Add `{"intake-pinnable-lanes", "/api/intake/pinnable-lanes", ""}` to `webAPIFixtures`; regenerate ONLY via `SINET_WRITE_API_FIXTURES=1 go test ./internal/api -run TestWebAPIFixtures` (this also refreshes `route-inventory.json` with the new route — `webfixtures_test.go:149`). Review the diff: expected new file `web/src/fixtures/api/intake-pinnable-lanes.json` + one route-inventory row; no other committed body may move.
9. **Sweep posture: a REPORTED gap naming its consumer** (the LN-6 `POST /api/meters/plan-budget` precedent, `web/src/sweep.test.tsx:525-534`). After regeneration the sweep fails ("served and nothing in the SPA names them") — add to `exceptions`: `{ method: 'GET', path: '/api/intake/pinnable-lanes', gap: true, why: 'REPORTED GAP (P3-LN-10a, <landing date>): ...' }`. The `why` must exceed 40 chars, contain the literal `REPORTED GAP`, name **P3-LN-10 (the lane picker on the create form)** as the consumer, and note the plan-budget precedent of a backend half sitting served-but-unconsumed until its surface lands. Move the pinned counts DELIBERATELY: `gaps.length` 1→2 and the shape-set size 1→2 (`sweep.test.tsx:616-617`), extending (not rewriting) the surrounding comment — its own failure text orders the CONVENTIONS §46 update. `gapRoutes.length toBe(gaps.length)` then holds because the route IS served. `declaredButUncalled` stays exactly `['health']` — **no `api.ts` member lands here**; LN-10 adds the client verb with its caller.
10. **CONVENTIONS §46 gap record:** dated cross-note that the pins moved 1→2 routes / 1→2 shapes, naming P3-LN-10a and the consumer, in the §66-precedent voice (see the 2026-08-25 LN-6 note). The packet's own CONVENTIONS section is written at landing per standing process.
11. **No per-person filtering.** The set served is the PROCESS-WIDE set, deliberately: coverage is the union across people who placed a credential, the recorded over-approximation (`routing.go:315-324`); selection re-checks and the boundary still refuses. Per-person coverage is B6/v1 and rides LN gate-batch item 8 — inventing a filter here would settle an unsettled household question.

## Files to change

- `internal/api/api.go` — Config + Server field, copy in `New`, route registration.
- `internal/api/intake_handlers.go` — handler + wire struct (the family file).
- `internal/stage/skeleton.go` or `surface.go` — the accessor (one line + doc).
- `internal/shell/shell.go` — wire the accessor's value into `api.Config`.
- `internal/api/pinnablelanes_ln10a_test.go` — committed RED by this grounding; goes green, and the executor's full tests land beside it.
- `internal/api/apifixtures_test.go` — fixture wiring + `webAPIFixtures` row; regenerated `web/src/fixtures/api/intake-pinnable-lanes.json` + `route-inventory.json`.
- `web/src/sweep.test.tsx` — gap entry + pin moves (test file; `web/src` APPLICATION source stays untouched).
- `P3/CONVENTIONS.md` — §46 dated cross-note.

## Acceptance tests

**Committed RED already (this commit):** `TestLN10aPinnableLanesRouteIsServed` — authenticated GET on a wired server answers 200 with a `lanes` member. Verified red 2026-08-27: fails `status 404 (want 200)`, compiles clean, right reason (route absent).

**Executor lands (Go, `internal/api`):** (a) full-shape test — three pinnable rows + local not-pinnable, asserting exact JSON keys, `not_pinnable` byte-equal to `worker.LanePinRefusal`'s sentence, `omitempty` absence on pinnable rows, and ORDER = composed order (mutation: feeding rows reversed must flip the body); (b) empty set → `200 {"lanes":[]}` (literal `[]`, not `null`); (c) `Intake: nil` → `503 {"error":"not_wired"}`; (d) unauthenticated → the `requireIdentity` refusal like siblings; (e) `TestWebAPIFixtures` green against the committed body. **Shell/stage:** one test proving the served rows are the SKELETON's composed value (compose a skeleton with a commissioned lane, serve through its accessor, assert the boundary's `refuseLanePin` admits exactly the lanes the body marks pinnable — the one-set property stated end to end). **Web:** vitest fully green with the moved pins (owed at landing).

## Checklist

- [ ] Route served, `protected`, kebab-case, family error/caching posture (R1, R7).
- [ ] Value flows skeleton→shell→Config; no second `worker.PinnableLanes` call in production; `IntakeSurface` untouched (R2, R3).
- [ ] Wire: object envelope, snake_case, `not_pinnable` verbatim + `omitempty`, composed order unsorted, `[]` never `null` (R4-R6).
- [ ] Fixture exercises pinnable AND not-pinnable; regenerated only via the sanctioned env; route-inventory row present (R8).
- [ ] Sweep: gap entry with `REPORTED GAP (P3-LN-10a, ...)` naming P3-LN-10; pins 1→2/1→2; `declaredButUncalled` still `['health']`; §46 cross-note (R9, R10).
- [ ] Red test green; batteries `go test ./... -p 1` green; vitest green; `tsc` green.
- [ ] No new ⚙ (tally 118/33 untouched), no migration, no new dependency (`components.lock` untouched), no amendment, `Docs/`/`Research/` untouched, `web/src` app source untouched, $0 (no live call anywhere; tier L never armed).

## Open questions

None — the surface, shape, ordering, auth, empty and not-wired postures are all determined by verified precedent above. If the executor finds `web/src/api.ts` or any application source forced to change, STOP: that is LN-10's packet, not this one.
