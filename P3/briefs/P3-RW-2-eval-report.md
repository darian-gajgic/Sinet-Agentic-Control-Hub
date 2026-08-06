# P3-RW-2 evaluation report — VERDICT: FAIL (recorded verbatim at session close 2026-08-06; triage/drain = next session's first act)

Evaluation agent (Fable, fresh context) completed against executor commits `8f2a8f4` (red) + `0fc74d2` (impl), the brief `P3/briefs/P3-RW-2.md`, S13.7/S15.2–S15.3 drafts, CONVENTIONS §2/§3/§5/§7/§14/§23/§38/§39/§40/§46, and the coordinator's OQ1–9 dispositions (STATE log entry at `081f52c`). The executor context is DEAD (session close) — **any drain round goes to a fresh opus finalizer.** Coordinator preliminary leanings are appended at the end; they are leanings, not rulings.

## Toolchain (evaluator's own runs)

gofmt clean · `go vet ./...` clean · `go build ./...` clean · `go test ./...` fully green. At `8f2a8f4` (isolated worktree): build green, exactly the eleven packet tests red — the Amendment-A window real, declared, closed.

## The three declared deviations — all adjudicated SOUND

1. **`onboardDoor` at the composition root instead of `*stage.Surface`.** All three reasons verified primary: `stage.Config.OnboardStart` (stage.go:314) and `Skeleton.StartOnboarding` carry no RemoteURL; `internal/stage/round1_e2e_test.go:151–152,247` consumes both landed signatures (widening = pre-existing-test edits); OQ7 bars `StartOnboarding` semantic change. Two-call composition cannot double-clone (`onboard.go:157–165` idempotent path makes the inner call a read); race probe: 1 registry row / 1 capture / ≤1 task / ≤1 run every iteration. Residual defects F1/F2 attach to the composition, not the deviation's reasoning.
2. **`memory.go` `visibleProjects` delegation.** Parity verified by inspection (same cap, same ORDER BY, same skip-on-undecodable-members) and by the untouched memory suite passing (memory_test.go in neither commit).
3. **"redactionEdgeFiles exactly three" was a BRIEF DEFECT.** `git log -L` proves four since B6-7 (`56b93c4` added `chatapi.go`). `projects.go` does not join it; `TestRedactionObservabilityEdgeOnly` passes.

## Security fix — HOLDS

Stranger POST naming an invisible PENDING id: 409 `already_registered`, no content/owner/refs leaked, seam never entered (`starts == 0`), entry undisturbed. Visible-taken and invisible-taken answer the identical `errProjectTaken` object — no oracle beyond the sanctioned id-existence leak.

## Held-out probes (5; tree restored byte-exact after each, sha-verified)

1. Predicate revert (`projectVisibleTo` → always true): **FIVE committed tests fail** — the suite genuinely guards the rule.
2. Member-create (both entry states): clean — 409, seam not entered.
3. Torn-state pending entry (no task rows): → F1.
4. Two racing POSTs ×3 iterations: DB consistency held; → F2 (100% repro).
5. `remote_url` never dialed: traced — Register INSERT only (registry.go:57), presence-only in the event payload, never an argument to any `s.git(...)`; api serves presence, never the URL (test-pinned).

## Test-file audit (duty 2)

Only `internal/api/projects_test.go` changed in either commit; every edit to the grounding file sits outside the three grounding test functions (imports, sanctioned `projEnv`/`server()` seam, `onboardSeam` fixture, appended tests). **Grounding assertions byte-unchanged.** No pre-existing test file touched. `internal/project` byte-untouched (empty diff); `OnboardInput.RemoteURL` pre-existing since B4-2. Settings index / migrations / lock / go.mod: empty diffs.

## Checklist 1–14

All hold except as flagged: 1✓ 2✓ 3✓-structurally (one predicate `visibleProjectRows`+`projectVisibleTo` shared by list/detail/create-pre-read/memory; see F4) 4✓ 5✓ 6✓ 7✓ 8✓ (new code `errors.Is`-only; see F2's status-class aspect) 9✓ (zero api-minted events) 10✓-R8 (see F5) 11✓ 12✓ (four since B6-7) 13✓ (fixture regenerated; sweep disposition = builder's) 14✓ (all nine OQs grep-verified recorded in code).

## Findings

**F1 — MEDIUM — The OQ7 in-flight 200 can assert an onboarding that does not exist, and forecloses the seam's own healing.** `internal/api/projects.go:587` (pre-read) + `:241` (`onboardInFlightDetail`). A pending entry whose task/run rows are ABSENT — reachable because `project_seams.go:130–147` is non-atomic (`proj.OnboardStart` commits, then `sk.StartOnboarding` can fail: ctx cancel, DB error, enqueue refusal) — answers the owner's retry 200 "already started… nothing was cloned again," naming task/ask refs that resolve to nothing. Probe: 200 with refs while `tasks=0 runs=0`. Project stuck pending forever (activation needs the ask → the run; no repair verb), and the seam's deliberate re-dispatch idempotency (which a bare retry WOULD have used to heal) is foreclosed by the pre-read. Executor conformed to OQ7(a) as dispositioned — the defect is the disposition's premise meeting a non-atomic composition; brief R5's "response must be honest" fails in this state. Confidence high (reproduced).

**F2 — MEDIUM — Host store path served on the wire via the `ErrBadInput` mapping; 3/3 repro under racing POSTs.** `internal/shell/project_seams.go:156` (+ fixture twin `onboardSeamErr`): `project.Onboard`'s `"store path %q already exists"` (onboard.go:69) reaches the HTTP body verbatim as 400 `bad_request`. Every race iteration leaked the absolute host path. Defeats OQ4's store-path-to-NO-ONE through the error channel (S11 host-hygiene); also mislabels a platform race as caller fault (§39-B drain-D6) — an overlapping double-tap reads "bad request" for a create that succeeded. Confidence high (reproduced).

**F3 — MEDIUM — The packet's one new production seam has zero test coverage.** `onboardDoor`/`onboardRefusal` (`project_seams.go:107–160`) appear in no test; the api suite drives the hand-mirrored `onboardSeam` fixture (within the brief's sanctioned green shape), but the real door's two-call composition, enqueue-refusal path, RemoteURL pass-through, and sentinel mapping have never executed under test; the fixture's error translation is an uncompiled textual twin that can drift. (Contrast: `pinRefusal` beside it has dedicated coverage.) Confidence high (grep).

**F4 — LOW — R14 three-way incomplete on the POST door**: no member-direction or operator-direction test on create (evaluator's member probe passes but nothing pins it). `TestOnboardStartRefusesWhatItMust`.

**F5 — LOW — No projects `…ShapesNeverPercent` walk** (brief §8 item 11 / checklist 10; every sibling family ships one). Currently vacuously true (no ratio-shaped key in the wire structs) and unpinned.

**F6 — NIT — Three stale records:** (a) `internal/api/api.go:268–269` `Config.Onboard` comment still claims `*stage.Surface` implements it (false post-deviation; the projects.go twin was corrected); (b) `internal/api/projects.go:260–265` comment claims taken-but-invisible answers "through the seam" (false — both non-race paths answer `errProjectTaken` at the handler); (c) impl commit message "Two more executor tests join them" — only one joined at impl.

## Verdict

**FAIL** — F1, F2, F3 (medium), F4, F5 (low), F6 (nit). Core strong under adversarial probing: visibility, one-answer 404, security fix, walls, OQ dispositions, red-window mechanics all held.

---

## Coordinator preliminary leanings (recorded at session close; next-session coordinator triages with executable falsification per amendment C)

- **F1 DRAIN-leaning.** The honest fix shape: the OQ7 pre-read must VERIFY the in-flight claim before asserting it (task row exists → 200 with refs; absent → fall THROUGH to the seam so its re-dispatch idempotency heals the torn state — the healing the pre-read currently forecloses). This amends the OQ7(a) disposition's mechanics, not its intent (the phone-retry answer stays for genuinely-in-flight onboardings); log the disposition amendment at triage.
- **F2 DRAIN-leaning.** Two parts: never serve the store-path message (map that `ErrBadInput` limb to a path-free body); reclassify the race residue as the conflict it is (already_registered-shaped 409, not caller-fault 400). Both fixes at the seam mapping + the fixture twin kept in sync (F3's test should pin the real seam so the twin cannot drift).
- **F3 DRAIN-leaning.** A shell-side `onboardDoor` test (the `pinRefusal` coverage precedent): two-call composition happy path, enqueue-refusal path, RemoteURL pass-through, sentinel mapping — kills the drift risk F2's fixture twin exposed.
- **F4/F5 DRAIN-cheap** (fold into the same finalizer round: pin the member-direction on POST; ship the vacuous never-percent walk as its siblings do).
- **F6 DRAIN-trivial** (two comments + nothing for the immutable commit message beyond a STATE note).
- Likely ONE fresh-opus-finalizer round drains all six; re-check scope per drain size.
