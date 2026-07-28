# P3 handoff — last rewritten 2026-07-29, mid-B6 (5/13 packets landed; session ended on the operator stop directive after landing P3-B6-3A)

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* meant to get a fresh session oriented in two minutes. It is deliberately short and it goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If the two disagree, STATE wins and this file should be corrected.

Authority order for everything: **`Spec/core-architecture-v1.md` (frozen v1, tag `spec-v1`; drafts in `Spec/drafts/` are canonical text)** → `Spec/benchmark-preregistration-v1.md` (BENCH-REG, signed — its registered numbers change only via its own §17) and `Spec/frontend-components-v1.md` → `P3/CONVENTIONS.md` → `P3/STATE.md` → this file. `Research/` is a **closed archive**; `Docs/` is **read-only**.

---

## 1. Where the build is

**B0–B5 are ALL CLOSED** (B5 gate record: `P3/gates/B5-report.md` §8 FINAL). **B6 (frontend — S15 + FC-v1) is OPEN, is the LAST phase, and stands at 5/13 packets done + validated.** The queue (in STATE) grew from 9 to 13 via two ratified A/B/C splits (B6-2, B6-3). **LANDED:** B6-1 (read-side APIs: /api/runs, /api/tasks, /api/meters, /api/events query layers; stage events minted), B6-2A (approvals core: ranked 7-kind inbox, hash-pinned retry-safe answers, tiers/step-up/co-approval, cancel mapping + no-auto-kill AST wall, follow-up spawn), B6-2B (budgets/pause + oversight verbs + transitive drag-hint comparator; migration 0017), B6-2C (benchmark driver + single-shot `.direct` leg + verdict backend; NoFork recovery seam + failed-pair card; migration 0018), B6-3A (settings API: UISchema emitter, registry reads/writes/history operator-only, price surface on append-only migration 0019 shipping EMPTY; new event type `price.row_added`). **PENDING, in order: B6-3B (deliverables+accept+preview APIs) → B6-3C (memory API) → B6-4 (SPA scaffold — the frontend tree first appears here) → B6-5..8 (surfaces) → B6-9 (push/PWA + sweep).** Groundings for B6-3B/C are DONE (one brief serves all three parts); their OQ dispositions are ratified in the STATE log ("B6-3 grounding DONE" entry) — the next session launches the B6-3B EXECUTOR directly.

What the B5 gate settled (do not re-litigate any of it):

- **D1** — promptfoo `0.121.19` (MIT), changedetection.io `0.55.8` (Apache-2.0), genai-prices data `v0.0.72` (MIT) all **ratified** (S16.4 #10 closed). DeepEval `4.1.3` stays the pre-registered standby, pin-on-activation.
- **D2** — **EXECUTED, all green (11 ok / 0 failed):** both organs are **installed on this host at their pins**, user-level, no sudo/apt/system change. promptfoo via npm (`~/.npm-global`); changedetection.io via `uv tool` on managed CPython 3.11, **installed but NOT started** — its real-organ conformance leg stays a sanctioned skip until it runs with `SINET_CDIO_URL`+`SINET_CDIO_API_KEY`. `sinet-watchlist.service` remains **generated, not installed** — deferred to the hardening session.
- **D3** — **all four canary legs stay DISARMED at v0.** Auth + model-list are honestly un-composable (no per-lane HTTP endpoint or broker credential accessor exists — composing them is named work for whenever arming is wanted). The behavioral leg's **itemization question is TABLED as its arming precondition** (options recorded in the gate file: organ-reported entry / projection-only / broker-path provider). Projection ≈$5.70/yr API-equivalent; stop line = any non-zero real-dollar probe-tagged total.
- **D4** — the **0.84 eval floor is ratified**; the paid golden sweep ($2.10 projected / $5.00 stop) stays **registered and unexecuted until B6 gives its result a surface**.
- **D5** — **no S00.9 amendment** for the family-12 `RequiredFields` row (not-wrong-merely-not-restated reading stands). B5-7's **dispatch→render driver** (single-shot direct-arm capture + the B6 verdict form) is named follow-on scope — B6 is where the verdict form lands.
- **D6** — **35 readings ratified en bloc** (the report's "~30" honestly decomposed; compiled from CONVENTIONS §29–§37 + commit bodies). All are **binding precedent for B6** — including the cross-phase patterns: derive-from-log/no side stores, no tickers (dueness from stored state), structural constants with named reasons interim under the settings-tab directive.

**Counters at this handoff (assert these still hold before you change anything):**

| Thing | Value |
|---|---|
| Event inventory | **91 minted / 5 declare-only = 96 registered** (B6-1 flipped `stage.started/finished`; B6-3A added `price.row_added`) |
| ⚙ settings | **118 keys / 33 domains** — `internal/settings/index.go` byte-unchanged through ALL of B5+B6 (the UISchema emitter is a separate file) |
| `components.lock` | **21 entries** (lockgate green; no new dependency in all of B6 so far) |
| Migrations | **0001–0019** (`user_version` 19: 0017 budgets/pause/hint, 0018 benchmark capture, 0019 price_rows) |
| Test packages green | **42** (no new package dirs in B6 — new files live in existing packages) |
| CONVENTIONS | §1–§40 (earlier sections never rewritten; §38 B6-1, §39+B/C B6-2, §40 B6-3A) |
| Paid spend, all of B6 | **$0** (every stage fake-engine/hermetic) |

Git: everything through the B5-close commit is pushed to `origin/main`. The working tree carries the known **pre-existing operator items — do not stage, revert, or "clean" them**: `Research/02-provider-watchlist-and-onboarding-criteria.md` (modified, deliberately unstaged), untracked `Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/`.

---

## 2. This session's first act — open B6

Open with **"continue implementation"** (or `/p3-implementation`). Then, in order:

1. **Session-entry checks:** read `P3/STATE.md`; confirm `git status` is clean apart from the operator items above; re-run the landing battery so you build on measured ground:
   `gofmt -l internal/ cmd/` · `go vet ./...` · `go build ./...` · `go test ./... -count=1` · `go run ./tools/lockgate`
2. **Launch the B6-3B EXECUTOR directly** (opus; grounding is DONE — brief `P3/briefs/P3-B6-3.md`, part B = R7–R17 + the F block; the ratified OQ2 (comments = Create+Read only), OQ3 (bounded-revision doors-as-data), OQ4 (merge card return-only) dispositions in the STATE log bind; B6-2's landed surfaces are consumed never rebuilt; **hermetic preview tests NEVER touch the live Caddy admin chain** — standing host hazard). Then eval → triage → drain → land per the pipeline. Then B6-3C (part C, OQ5/OQ6 bind), then B6-4 onward in queue order.
3. **Carried notes that must not fall between chairs:** the B6-6 grounding absorbs the NINTH derived inbox kind (memory conflicts, B6-3 OQ6) and the D4(b) eval-results-surface unlock; the bring-up ledger carries the BENCH-REG amendment-queue items (tool-less direct arm; crash-vs-consent decline-rate separation) and the storage-down operator-resolution mislabel observation (landed B6-1 machinery, fail-closed, low urgency).
4. **Run packets through the four-stage pipeline** (§4 below). B6's gate at the end = the phase decision batch **plus the operator's own acceptance: the UI click-through per their 2026-07-20 directive**.
5. After the B6 gate: bring-up is next (the S19.6 measurement sequence + the accumulated TBD-BRINGUP rows — see the B5 report §3 table; the Layer-2 open-SQL acceptance measurement must count guardrail-conservatism refusals as an expected refusal source, per the ratified landing record).

---

## 3. What changed on the host recently

- **The 2026-07-28/29 build sessions changed NOTHING on the host** — pure repo work (Go code, migrations applied only to test DBs, $0 spend, no sudo/apt/unit/secret/canary).
- From the B5 gate (2026-07-28): **promptfoo 0.121.19** and **changedetection.io 0.55.8** are INSTALLED (user-level; undo: `npm uninstall -g promptfoo` / `uv tool uninstall changedetection.io`). changedetection.io is **not running**, no API key placed; both belong to the hardening session (with the unit install). The key is broker-custody — never via chat or shell history.

---

## 4. How the machinery works (do not reinvent it)

Every packet runs a **four-stage pipeline**, all stages launched by the coordinator as fresh-context background agents, strictly sequential within a packet:

**grounding** → writes `P3/briefs/P3-<phase>-<n>.md` (handoff artifact *and* evaluation rubric; numbered requirements with spec refs, seams, ⚙ by name, acceptance checklist, open questions for coordinator disposition) → **executor** (always Opus) → **evaluation** (fresh agent, judge ≠ executor, told to find what is wrong) → **drain** (coordinator triages findings, executor fixes, evaluator re-checks; **hard cap of two rounds**, then the coordinator implements the remainder inline).

What repeatedly earned its keep:

- **Open questions get coordinator dispositions before the executor launches.** Nothing is resolved silently.
- **Evaluations are adversarial and reproduce their own claims** (planted canaries, reverted fixes, invented attack routes). An untested security property is one refactor from silently gone.
- **A declared deviation is a referral, never an acceptance.** The coordinator rules; several referrals were real defects, several were sound readings.
- **Split a packet before an executor rushes it** — grounding's scope verdict is the trigger; a pre-authorized seam stop is a success mode.
- **Never commit while a stage agent is live in the tree**; use `git commit --only P3/STATE.md` for coordinator bookkeeping meanwhile.
- **Migration immutability**: binds landed migrations; a migration may be edited during its *own* packet's drain (no production DB exists yet); immutability re-attaches at landing.
- Update STATE **before and after every step**. Push after milestone commits.

---

## 5. Model routing

Executors and finalizers are **always Opus** (judge independence + classifier immunity mid-run). Grounding and evaluation inherit the session model, with a **lossless Opus fallback** on any safeguard trip. The standing lesson: **sessions that will summarize security-dense work (S10/S11 internals, the §37 guardrail material) should start on Opus** — a documented Fable false-positive on the operator's own authorized infrastructure work; never reword to evade the classifier. *For calibration: the entire B5 gate session ran clean on Fable 5, including the guardrail summaries — the lesson stays recorded but B6's frontend scope (S15/FC-v1) is not security-dense, so inherit is fine.*

---

## 6. Invariants that must never be broken

- **The spec wins** — over model memory, reports, and existing code. A conflict, gap, or impossibility is *never* resolved silently: stop the packet, write an S00.9 amendment proposal, present it. Any amendment touching a ⚙ setting re-runs the S18 sweep.
- **D1–D10 are fixed.** Never re-derive or relitigate.
- **Adopt, don't fork.** Pins exact; `components.lock` + CI lock-gate cover every dependency; a new adoption needs its watch row (test-enforced).
- **Every ⚙ number ships through the settings registry**; structural constants carry named reasons and are interim under the settings-tab directive.
- **No load-bearing metered paths**; subscription-coverage rule holds.
- **Real-world facts are live-verified at time of use** — never answered from memory.
- **BENCH-REG registered numbers are read-only data**; drift is a platform defect by its own clause.
- **No auto-kill anywhere** (S14.4 / G1-D1.3). Watchdogs and canaries pause and flag; they never terminate.
- Secrets are never committed. Host-level changes are proposed and approved, never applied unilaterally — units are **generated, not installed**.
- **The 35 ratified B5 readings are binding precedent** (CONVENTIONS §29–§37): derive-from-log, no tickers, cards derive bounded, severity computed never judged, proposals never auto-apply, money read never computed, model output is untrusted input, refusal never repair, every refusal audited.

---

## 7. Host and environment state you inherit

- **Local inference tier live-capable, user-level:** llama-swap v241; llama.cpp b10085 CUDA-built sm_120 (user-level CUDA 12.9 + micromamba gcc-14 — **no driver/kernel/system-CUDA change ever, and none may be**). Weights/scratch in `~/.sinet-b45` (~38 GB, outside the repo). Three model seats remain unpulled, none on the v0 default path.
- **Production front chain LIVE on this machine:** `tailscale serve` → Caddy on 127.0.0.1:8481 → production unit on 8482; `sinet-control` + `sinet-broker` active. **Packet tests stay hermetic and never touch the live Caddy admin API/config** — real hazard, own tripwire.
- **B5 organs:** promptfoo + changedetection.io **installed** (§3); watchlist unit generated-only; canaries disarmed.
- Service-context confined runs stay blocked by the AppArmor userns finding until the **hardening session** (needs sudo; its list now also carries: `sinet-watchlist.service` install + starting changedetection.io with its broker-custody API key, plus the carried `socat`/egress/run@/Landlock/`user.slice`-probe items).
- **Standing operator items, none blocking B6** (full list: B5 report §7): housekeeping `rm`s (`/tmp/llamaswap-test/`, `~/sinet-demo*`, `tools/dbpeek/`, `~/.sinet-b45/baselines/` ~26 GB); the `Research/02` unstaged addendum (operator's call); probe-batch suspend leg (pairs with the battery-drain hour); week-one push drill at first deploy (B6 brings it in reach); optional GitHub Verified badge; Z.AI calibration parked (no lane).
