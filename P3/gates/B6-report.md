# B6 gate report — the frontend, and the end of the build

**Phase B6 (S15 + `Spec/frontend-components-v1.md`) is COMPLETE: 13 of 13 packets built and validated.** With it, **P3 build work is finished** — B0 through B6, every phase in the S19.5 order, is closed or awaiting this gate.

Everything below was measured on this machine on 2026-07-30, not carried forward from a prior record. **The whole phase cost $0**: every stage ran against fake engines or hermetic fixtures, and no push ever left this machine.

---

## 1. What shipped (packet → commits)

| Packet | What it is | Commits |
|---|---|---|
| B6-1 | S15.2 read-side API completion, S14 query layers routed, stage events minted | `bd1da02` `4489385` `8ba848d` + `c2cb637` |
| B6-2A | Approvals core — inbox, hash-pinned answers, tiers, step-up, co-approval, batch, cancel | `ba8b5ba` + `46a6ed2` + `5388678` |
| B6-2B | Meters + oversight verbs, drag hint, migration 0017 | `fe6d32b` + `2d33ae2` + `afe244a` |
| B6-2C | Benchmark driver, the `.direct` engine leg, verdict backend | `eebe214` `7d4c586` `b15660c` + `30fbef6` |
| B6-3A | `/api/settings` — UISchema emitter, registry reads/writes/history, price surface, migration 0019 | `d4d1705` + `d25b5ce` |
| B6-3B | `/api/deliverables` — revisions, accept, preview sessions | `546f7f1` `d1d0098` + `1d8122e` |
| B6-3C | `/api/memory` — gate-routed writes, scoped reads, conflicts | `5860cee` + `5333849` |
| B6-4 | The SPA scaffold — Vite + React 19 + TS PWA, the npm rail, four picks, `go:embed`, app shell | `4c5810b` `6291f67` + `108a637` |
| B6-5 | Oversight surfaces — mission control, live board, task detail, fleet, personal filters | `4744e4f` `a5b4d7a` + `754916f` `68d8d11` + `1f85f6b` |
| B6-6 | Decision surfaces — the approval inbox and the settings UI | `912903f` `4c3d96a` + `5083065` `073c9c2` |
| B6-7 | The conversational assistant — backend and widget | `56b93c4` + `e6850e1` `5121dc1` `4f87c5e` + `d0f812c`; `4a858a0` + `082b488` `ee39c10` + `8880835` |
| B6-8 | Review surfaces, try-it, accept UI; the workforce map | `ac532ca` + `4a44f72` `2b238b3` + `c0d7c93`; `6b6c921` + `ec76960` `94bfc95` + `62a13aa` |
| B6-9 | Declarative Web Push, the notifier, PWA completion, the S15.12 sweep | `10d133b` `3a38202` + `6e2e114` `31b4ca8` + `28facbe` |

**`built` routes = 11 = the whole table. There is no unbuilt route left in the SPA.**

## 2. Evidence (re-run by the coordinator at landing, not relayed)

```
gofmt -l internal/ cmd/ tools/     → empty
go vet ./... ; go build ./...      → clean
go test ./... -count=1             → 45 packages, 0 failures, 14 sanctioned skips
go run ./tools/lockgate            → 29 entries; 240 npm packages covered
cd web && npm run typecheck        → clean
             npm run test          → 432 tests / 23 files
             npm run build         → 976.99 kB (gzip 292.96)
SINET_WRITE_API_FIXTURES=1 …       → zero fixture drift
```

- **The 14 skips are correct, not zero.** They are pre-existing env-gated live legs (GPU, paid, canary, organ). An earlier "0 skips" figure in this phase's records was asserted from output that never counted skips; it is corrected at every site.
- **Frozen paths byte-verified** against the pre-packet commit: `internal/settings/index.go`, `internal/eventlog/contract.go`, `go.mod`, `go.sum`, `components.lock`, `web/package.json`, `web/package-lock.json`, `internal/benchmark/registered.go`.
- Migrations **0001–0021** · event inventory **102 minted / 5 declare-only / 107** · ⚙ **118 keys / 33 domains** · conformance **12** · Layer-0 views **11** / Layer-1 catalog **50** · escape-scan allowlist **EMPTY**, banned tokens **8**, floor **50**.
- **`go.mod`, `go.sum` and `components.lock` are byte-unchanged across B6-9** — which is the "owned stdlib crypto" disposition confirmed *at the lock* rather than claimed in prose.

## 3. Measurements

- **Cost: $0 across all thirteen packets.** No paid call was made. The D4(b) paid sweep remains registered and unexecuted (§6, D2).
- **Bundle: 976.99 kB / gzip 292.96**, measured at each step. It grew from 851.73 kB when B6-7 landed; the react-diff-view closure and the push/sweep code account for the rest. Nothing is code-split, deliberately: the SPA is embedded in the one binary and served over a tailnet.
- **Push: zero pushes left this machine.** The send path was driven entirely against a fake service.
- **RFC 8291 conformance is pinned to the standard's own worked example** — and the evaluation independently recomputed all 18 Appendix A intermediates in its own stdlib-only program with zero repository code, so the vectors are the published ones rather than the implementation's own output.

## 4. The click-through — one command

```
./P3/gates/B6-clickthrough.sh
```

It builds the SPA and the binary, starts a **throwaway** control plane on port 8483 with a fresh database, verifies the app actually serves (shell, bundle, deep-route fallback, session), tells you what to click, and shuts down when you press a key. `--clean` deletes the throwaway state.

**It cannot touch production and refuses to start if it would.** I ran it end to end: the production units stayed active, the live Caddy kept 8481 and the production unit kept 8482 throughout, and migration 0021 was applied to the throwaway database only. `/usr/local/bin/sinet` is untouched.

The database is new, so most surfaces are honestly **empty** — that is the thing worth looking at. An empty surface should say it is empty and why; never a blank panel, never a fabricated zero.

**What the gate needs back from you, in your own words:** anything that looked wrong, ugly or confusing; any number that was invented instead of admitted unknown; any control that looked pressable and did nothing; anything missing you expected.

## 5. Deviations and carried notes

- **Four sanctioned-narrow touches of landed packages**, each declared and ratified rather than silent: two in `internal/review` (a diff-header fix and verify-before-serve on the object route), one declared widening in `internal/worker` (the roster read), and one in `internal/api`'s meter projection (the "no reading" vs "a reading of zero" split). Each was referred to me before it landed.
- **A predicate fix in landed B6-8 code** during B6-9's drain, to remove a submit button that rendered enabled and could only ever fail. Confined to the predicate; all five door limbs re-verified across all three fixture worlds.
- **Six refusals-with-evidence were upheld this phase**, three of them against my own instructions — including one that proved a review door as landed could never have worked, and one that refuted a concern of mine with a 23-input differential.
- **Two of my own recorded numbers were wrong and were caught downstream** ("0 skips", and "three new gaps" where the true figure was one). Both are corrected at every site. The record only worked because the passes below it checked the record.
- The shared golden-fixture `cursor` moved 64 → 86 across the phase, because fixtures are built through real producers and real producers mint events. Regeneration is stable.

## 6. Decisions for you

**D1 — Adoption ratifications (S16.4 #10).** The frontend tree needs your sign-off: the four FC-v1 picks at their pinned versions (@hello-pangea/dnd 18.0.1, react-diff-view 3.3.3, @assistant-ui/react 0.14.27, JSON Forms 3.8.0), the React/Vite/TypeScript toolchain, Vitest + jsdom, and `actions/setup-node`. **For the record: the closure is not all-MIT — `diff-match-patch` is Apache-2.0.** Both licences are permissive and compatible; `components.lock` never claimed otherwise.

**D2 — The paid golden sweep.** Its surface now exists. The run is pre-registered at ~$2.10 projected against a $5.00 stop line and has never been executed. Say the word or leave it parked.

**D3 — The fourteen route gaps.** B6-9's completeness sweep reports fourteen registered routes with no surface, and this is the phase's most valuable single output. The whole **memory family** (browse, read one, gated edit, retire, owner-delete — plus **`POST /api/memory`**, creating a knowledge entry, which counting-by-shape had hidden behind the GET at its own path); **cancel** on runs and tasks; the **budget editor**; the **pause-my-automation** switch; the benchmark **opt-in**; and history's **ask/search**. None is a defect — each is a landed, owner-scoped verb with no control. Which of these do you want built, and in what order?

**D4 — Workforce visibility.** The operator sees every worker, including another member's personal one. S15.10's own sentence says only "personal workers to their owner, the shared roster to all" and names no operator limb. My reading is that this describes ordinary member scoping and does not silently repeal the platform-wide role model — and worker identity is already operator-visible through routing records on the runs surfaces, so hiding the roster would be a fig leaf. It rests on a reading, so it wants your explicit ratification.

**D5 — Timestamps.** Everything renders verbatim UTC today. Whether relative or local times should sit beside it is deliberately your taste, decided on live surfaces during the click-through.

**D6 — The production install.** `/usr/local/bin/sinet` is still the B2-gate binary from 20 July and has never been rebuilt, which is why no production database has ever applied migration 0020 or 0021. Upgrading it means rebuilding, restarting the units, and migrating the production database. Do you want that now, at bring-up, or not yet?

**D7 — Readings en bloc.** The phase accumulated a set of smaller judgments, each recorded with its reasoning in `P3/CONVENTIONS.md` §38–§46. They are listed in §7 below; ratifying them as a block is the pattern from B3–B5.

**D8 — The visual design pass** *(added 2026-08-04, on the operator's question "is this frontend a preview or final?")*. Grounded finding: the frozen spec contains no visual design language anywhere — S15 is an API/behavior contract and FC-v1 is component picks — so B6's bar was purely behavioral, and the styling layer is exactly that deep: one hand-written 1,409-line `web/src/index.css`, no design system; the architecture is final, the appearance is v0. The pass = a real design language and layout polish across the 11 routes (repo-only, $0, behavior-preserving; any styling framework arrives through the adoption rail with lock entries; assets bundled, escape-scan rules bind). **Sequencing: after the D3 route-gap work lands, before the D6 production upgrade**, so the design covers the complete control set and bring-up deploys the finished look once.

## 7. Carried items, for ratification as a block

- **The Layer-1 catalog is at its band ceiling (50/50).** The next catalog query breaches it and forces the band decision. B6-8 avoided it deliberately by deriving its per-version read transport-side.
- **Gate-side authority narrowing** — `Gate.authorize` lacks station-3 project-membership narrowing and `ResolveConflict` lacks affected-owner narrowing. Both v0-contained: the HTTP transport is the sole production write path.
- **`PricedCost` cannot distinguish "free" from "unknown" per unit.** The surfaces now say "no meter reading" where there is none, but the per-unit shape is still coarse.
- **The S10.6 disclosure key** — the downgrade-note render reads a key no producer emits yet.
- **VAPID key custody** — control-plane-held at v0 (state directory, 0600). Moving it to the broker is a hardening-session item; rotation is documented, not automated, because it invalidates every subscription.
- **`writeSurface` labels every family's errors "intake surface"**, so several routes are mislabelled in the ops log. A one-line fix, deliberately not taken locally because that would trade a cosmetic label for a real inconsistency across siblings.
- **An interview card is served with no answer verbs**, so the inbox honestly renders "nothing here to press" while chat answers it fine. Neither surface is buggy; the fix belongs at the producer.
- **An `automation-step` approval card renders through the generic fallback** as a run-together argument dump beside a live PIN field. Legible enough to act on, ugly enough to name.
- **The roster read is an N+1** at its declared caps (~10⁵ queries). Moot at household scale; recorded rather than restructured.
- **The inert-probe shape exists in landed B6-5/B6-6 tests** — a web assertion reading a committed fixture cannot fail on a server change, whatever its message claims. Found while closing this packet's own instances; the pre-existing ones were deliberately not charged to B6-9.
- **The double read on tab mount**, and `mission.test.tsx`'s queued-bucket `.sort()` — both landed behaviour, both left alone with their reasons.
- **Produced-files chips are honestly sparse at v0** — only uploads write the exchange folder.
- **An unknown future domain maturity that ought to bar schedules would render "granted"** with no caveat. Inventing a bar for undefined vocabulary would fabricate policy, so it is recorded rather than guessed.
- **Optional cosmetic S00.9 amendments** — add the chat and push family rows to the S15.2 table; align S15.5's "measured, n=…" paraphrase with BENCH-REG §13.2.

## 8. Standing operator items (none block the gate)

| # | Item | Note |
|---|---|---|
| 1 | **Week-one push drill** on household phones | Runbook shipped: `P3/runbooks/week-one-push-drill.md`. Executes at first deploy. Carries the ntfy-with-relay flip condition verbatim |
| 2 | **The hardening session** (needs sudo) | AppArmor userns grant, `sinet-watchlist.service` install, changedetection.io start with its broker-custody key, socat/egress/run@/Landlock, VAPID broker custody |
| 3 | **Observables register sign-off** | Signed 2026-07-20 for the 3-row v0 register; row 2 (push metadata) was noted as not-yet-live and now is |
| 4 | Fleet seat / GPU fill | Named honest absence at v0 |
| 5 | Suspend-probe leg and freeze/thaw | Paired with the battery-drain hour |
| 6 | `/tmp/llamaswap-test/` and demo dirs | Awaiting your `rm` at leisure |
| 7 | (Optional) GitHub Verified badge | Signing-key upload, anytime |

## 9. Gate answers

_To be filled in from the operator's free-text response. Free-text answers are authoritative and are not re-asked via a form._

- D1 adoption ratifications: **ANSWERED 2026-08-04: "ok" — the frontend adoption set is RATIFIED as presented (four FC-v1 picks at pins, React 19/Vite/TS toolchain, Vitest + jsdom, actions/setup-node; the not-all-MIT closure noted and accepted).**
- D2 paid golden sweep: **ANSWERED 2026-08-04: "ok" — AUTHORIZED per the recommendation: executes at bring-up after the D6 upgrade; ~$2.10 projected against the $5.00 registered stop line.**
- D3 the fourteen route gaps — which, in what order: **ANSWERED 2026-08-04 (conditional yes + resequencing directive): all fourteen get built, but NOT in the current UI style — the operator directs that HOW they are built is declared and approved FIRST ("the UI is not good right now"), with the Nexus frontend and other GitHub tools as copy/reference sources. Effect: the D8 design-approach declaration moves BEFORE the D3 build; the fourteen controls are built in the declared language from the start; the D8 pass then covers the pre-existing routes. The coordinator produces the design-approach proposal (Nexus frontend survey + live GitHub sourcing through the adoption rail) for operator approval before any D3 packet is cut.**
- D4 workforce visibility reading: **ANSWERED 2026-08-04: "ok" — RATIFIED.**
- D5 timestamps: **ANSWERED 2026-08-04: "ok, whatever is best" — the recommendation becomes the decision (relative time beside verbatim UTC on live surfaces; UTC-only in audit/history detail); taste delegated to the coordinator; implementation rides the UI batch.**
- D6 the production install: **ANSWERED 2026-08-04: "you can run it, I will give you sudo -v" — the upgrade is AUTHORIZED with the COORDINATOR as executor via the established `sudo -v` window mechanism (operator grants it at execution time). Sequencing per the ratified recommendation: ONE upgrade after the UI batch (D3 + D8) lands, production DB backup first, immediately before bring-up.**
- D7 readings en bloc (§7): **ANSWERED 2026-08-04: "ok" — RATIFIED EN BLOC, including the rider: the two cosmetic S00.9 amendments (S15.2 chat/push rows; S15.5 wording) apply during the UI-batch window.**
- D8 visual design pass: **ANSWERED 2026-08-04 (operator, free text): ratified as the eighth work item, sequencing agreed (after D3, before D6). The design brief — what the operator wants and how — is deliberately deferred: the operator declares it at the pass's own gate, when D8 is reached. Do not ask for it earlier, and do not start D8 without it.**
- Click-through observations (A: wrong/ugly/confusing · B: fabricated numbers · C: dead controls · D: missing): **A-1 received 2026-08-04 (operator, mid-gate): the UI does not explain itself — "pretty bad and unintuitive, no idea what this is supposed to be." An operator lands with no orientation to what the platform is or what any surface is for. Logged as the first A-finding. It stands regardless of the empty-database context, and it widens D8's mandate: the design pass owns intelligibility, not just looks — surfaces and empty states that teach what they are, not only admit they are empty.**

  **A-2 + root causes, verified live on the operator's own instance (2026-08-04 probe + the operator's own chat transcript):** (1) *Settings showed 118/118 rows with history but no editor* because the click-through browses as the **dev-posture identity, which "reads but never administers"** (`settingsWriteAuthority`, served `editable:false` with exactly that reason) — the write surface is deliberately **absent-not-disabled** per the no-403-controls rule; the sole explanation is one banner line a person has no reason to connect to the missing editors. Editing is real behind a real operator session; the throwaway's first-boot window can bootstrap one (steps given in-session). (2) *The assistant answered every typed question with the Layer-1 disambiguation card* — reason "the local tier is not wired here, so intent cannot be classified — pick a query by name" — plus the full 50-query catalog as flat choices. Honest degraded mode (no intent-duty seat in the throwaway; and **no free-generation chat exists by S15.7/S14.10 design** — the assistant is three verbs: platform-state ask, task handoff, file exchange), but experientially opaque: jargon reason, 50 ungrouped choices, and the three-verb contract stated nowhere a person looks. **Both complaints are platform rules holding (deny-by-default identity; honest-absence-over-guessing) with no explanation layer.** D8's mandate now explicitly includes: every surface teaches its contract; a degraded mode says in operator words what would un-degrade it; the disambiguation card gets grouped, plain-language rendering. Tooling note: a click-through flag that bootstraps a throwaway operator login would make the settings write path walkable at future gates.

  **C-1 (the gate's own question C, found by the operator 2026-08-04): in dev posture the header's Sign out is a no-op and /login is unreachable.** The shell's fail-closed redirect ("no /login once there is a session", `App.tsx`) plus the dev-fallback identity (always a session) means the login picker can never render, and signing out just re-resolves to the dev identity — a rendered control that cannot ever do what it says, in dev posture only. The identity layer's own design note — "sessions still win when present, so the full login flow is exercisable in dev" — is defeated one layer up. **Production posture is unaffected** (no fallback identity; login and sign-out behave). **Workaround verified end-to-end this session:** launching the same binary with `STATE_DIRECTORY`/`CONFIGURATION_DIRECTORY` set flips `devPosture()` off — probe showed anonymous session, 401 on protected routes, then bootstrap → login → `editable:true` → logout → `authenticated:false`, all real. Fix candidate for the D3/D8 batch: render the picker on /login when the only session is the dev fallback, and hide Sign out in dev posture.

  **Walk COMPLETED 2026-08-04, including the real-login leg** (service-posture relaunch: login, sign-out and live settings editing exercised by the operator). No B (fabricated-number) or D (missing-expected) findings reported beyond the recorded set.

---

**GATE CLOSED 2026-08-04.** All eight decisions answered. The directed post-gate batch — design-approach declaration (operator-approved) → the fourteen controls in that language → the D8 pass across the pre-existing routes → the D6 production upgrade → bring-up — is queued in `P3/STATE.md`.
