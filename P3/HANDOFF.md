# P3 handoff — last rewritten 2026-07-29, mid-B6 (10/13 packets landed; B6-7 part A committed-but-unevaluated; session ended on the operator's context-budget stop directive)

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* meant to get a fresh session oriented in two minutes. It is deliberately short and it goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If the two disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1, tag `spec-v1`; drafts in `Spec/drafts/` are canonical text)** → `Spec/benchmark-preregistration-v1.md` (BENCH-REG, signed — its registered numbers change only via its own §17) and `Spec/frontend-components-v1.md` (FC-v1) → `P3/CONVENTIONS.md` → `P3/STATE.md` → this file. `Research/` is a **closed archive**; `Docs/` is **read-only**.

---

## 1. Where the build is

**B0–B5 CLOSED. B6 (frontend — S15 + FC-v1) is OPEN, is the LAST phase, and stands at 10/13 landed + validated**, with an eleventh packet mid-pipeline.

**Landed + validated (all pushed):** B6-1 (read-side APIs) · B6-2A (approvals core) · B6-2B (budgets/oversight/drag-hint, mig 0017) · B6-2C (benchmark driver + `.direct` leg, mig 0018) · B6-3A (settings API + UISchema emitter + price surface, mig 0019) · **B6-3B** (deliverables/accept/preview — 11 routes) · **B6-3C** (memory — 7 gate-routed routes) · **B6-4** (the SPA scaffold: `web/` Vite 8.1.5 + React 19.2.8 + TS 6.0.3, the four FC-v1 picks at their exact pins, the npm rail + lockgate over 240 packages, `go:embed` serving through the front chain, the app shell) · **B6-5** (oversight surfaces: mission control, live board, task detail, fleet, the four personal filters) · **B6-6** (decision surfaces: the nine-kind approval inbox + the full settings tab over JSON Forms).

**MID-PIPELINE — this is the next session's first act.** **P3-B6-7 part A (the assistant backend) is COMMITTED (`56b93c4`) but its EVALUATION NEVER RAN.** The executor finished part A, committed, and stopped without its final report. The coordinator verified the inherited tree afterwards: **gofmt/build clean · `go test ./...` 44 packages, 0 failures · lockgate 29/240 · web typecheck clean + 217/217 vitest + build ok** — green and consistent, nothing half-applied.

**Pending after that: B6-7 part B (the widget — `/chat` is still a STUB) → B6-8 → B6-9 → the gate.**
**Groundings and dispositions are DONE for every remaining packet** (B6-7 OQ1–10, B6-8 OQ1–8, B6-9 OQ1–14 — all ratified in the STATE log, all binding). No grounding work remains in B6.

---

## 2. This session's first acts, in order

1. **Session-entry checks:** read `P3/STATE.md`; confirm `git status` shows only the five long-standing operator items (`Research/02-…md` modified; untracked `Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/` — **never stage, revert, or "clean" these**); re-run the landing battery so you build on measured ground:
   `gofmt -l internal/ cmd/ tools/` · `go vet ./...` · `go build ./...` · `go test ./... -count=1` · `go run ./tools/lockgate` · `cd web && npm run typecheck && npm run test && npm run build`
2. **Launch the B6-7 part-A EVALUATION** (pipeline stage 3, fresh agent, inherit model). Range **`7399c46..56b93c4`**. Rubric = `P3/briefs/P3-B6-7.md` §7 part-A items + the OQ1–10 dispositions in STATE. Tell it to independently verify: the **family-12 admission** for all eight `chat.*` types (§29 same-packet mechanics, 91/5/96 → **99/5/104**, every pin incl. the third registry-size pin in `internal/history`'s test); **migration 0020** + the six-site lockstep sweep; the **owner-only** transcript walls in BOTH directions (the operator must not read another member's chat — the store takes a viewer with no role parameter, which is the structural guarantee; probe it adversarially); the client-routed turn dispatch (no model classifies a turn; nothing falls through to Layer 2 — the audit-count assertions); the `utility`-seat titling (tier-F `FakeServer`, $0, degradation path); the OQ8 additive `inputs` shape; the exchange confinement (resolve-then-deny); fixtures driven through real verbs. Then triage → drain (cap 2 rounds) → land part A.
3. **Then B6-7 part B** (the widget): fresh executor (opus) — ExternalStoreRuntime + the Sinet adapter, the ladder affordances (explicit-act escalation only), the file-exchange UI, produced-file chips, stop/abandon, the born task's intake card rendered inline. A **running turn is already staged in the `chat-session` fixture** for the re-attach render. assistant-cloud NEVER wired; plain-text transcript (OQ10); zero new npm deps.
4. **Then B6-8** (review surfaces + try-it + accept UI + workforce map; brief `b384d30`, OQ1–8 ratified — note: the escape allowlist stays EMPTY and gains the `srcdoc` banned token; the TBD-P3 map rendering is RESOLVED as owned).
5. **Then B6-9** (push + the S15.12 sweep; brief `c1471ef`, OQ1–14 ratified — owned stdlib RFC 8291 crypto, migration **0021**, the push-only service worker ADMITTED, VAPID interim custody with a named hardening item).
6. **Then the B6 GATE** — `P3/gates/B6-report.md` + the phase decision batch + **the operator's own UI click-through** (their 2026-07-20 directive). §4 below lists what the batch must carry.

---

## 3. Counters (assert these before changing anything)

| Thing | Value |
|---|---|
| Event inventory | **99 minted / 5 declare-only = 104 registered** (B6-7 added eight `chat.*` under family 12) |
| ⚙ settings | **118 keys / 33 domains** — `internal/settings/index.go` byte-unchanged through ALL of B5+B6 |
| `components.lock` | **29 entries** (21 Go-side + the 8 frontend/toolchain rows from B6-4; lockgate also covers **240 npm packages**) |
| Migrations | **0001–0020** (`user_version` 20; 0020 = chat). B6-9 opens **0021** (push subscriptions) |
| Conformance seed rows | **12** (B6-4 added the frontend dependency-pass row) |
| Go test packages | **44** green |
| Web tests | **217 / 17 files** green (vitest, offline) |
| CONVENTIONS | §1–§44 (+ §40-B/-C, §41-B, §42-B, §43-B and dated drain appendices — earlier sections are never rewritten) |
| Paid spend, all of B6 | **$0** (every stage fake-engine/hermetic) |

Git: everything through `7399c46` is pushed; **`56b93c4` (B6-7 part A) is committed locally and NOT pushed** — push it when part A lands after its evaluation.

---

## 4. What the B6 gate batch must carry (accumulated all phase — do not lose these)

- **S16.4 #10 ratifications** for every frontend adoption: the four FC-v1 picks at their pins (@hello-pangea/dnd 18.0.1, react-diff-view 3.3.3, @assistant-ui/react 0.14.27, JSON Forms 3.8.0), the React/Vite/TS toolchain tree, Vitest + jsdom, `actions/setup-node`.
- **D4(b) paid golden sweep** — its surface now exists (the settings eval-results panel); the $2.10-projected / $5.00-stop run is registered and unexecuted, awaiting the operator's say-so.
- **The aggregate done-directly read gap** — no REST route serves the §13.2 measured-label aggregate (`benchmark.DomainReadout` is unreachable); confirmed independently twice.
- **The Layer-1 catalog is AT its 30–50 band ceiling (50/50)** — the next catalog query breaches the band and forces a decision.
- **Gate-side authority narrowing** (memory): `Gate.authorize` lacks station-3 project-membership narrowing, and `ResolveConflict` lacks affected-owner narrowing — both v0-contained because the HTTP transport is the sole production write path (enumerated + revert-probed), both fixed properly Gate-side later.
- **The per-unit unpriced-trace shape** — `PricedCost` cannot distinguish "free" from "unknown" per unit (a float64 + the frozen S10.3 row shape); the UI path is closed, the semantics question is an S00.9-adjacent metering follow-up.
- **The S10.6 disclosure key** — the downgrade-note render reads an explicit disclosure key no producer emits yet; the future ladder packet must mint the documented key.
- **VAPID key custody** — held control-plane-side at v0 (StateDirectory 0600); moving it broker-side is a hardening-session item.
- **Optional cosmetic S00.9 amendments** — add the chat and push family rows to the S15.2 table; align S15.5's "measured, n=…" paraphrase with BENCH-REG §13.2's registered text.
- **Push retention + timestamp presentation** — exchange-file retention has no sweeper by ratified decision; and whether surfaces show relative/local times beside the verbatim UTC is deliberately left as an **operator-taste question for the click-through**.
- **Fleet seat/GPU fill**, the hardening session's accumulated list (AppArmor userns, `sinet-watchlist.service` install, changedetection.io start with its broker-custody key, socat/egress/run@/Landlock), and the standing operator housekeeping items (B5 report §7).

---

## 5. How the machinery works (do not reinvent it)

Every packet runs a **four-stage pipeline**, all stages launched by the coordinator as fresh-context background agents, strictly sequential within a packet: **grounding** → writes `P3/briefs/P3-<phase>-<n>.md` (handoff artifact *and* evaluation rubric; numbered requirements with spec refs, seams, ⚙ by name, acceptance checklist, open questions) → **executor** (always Opus) → **evaluation** (fresh agent, judge ≠ executor, told to find what is wrong) → **drain** (coordinator triages [F1..Fn], executor fixes, evaluator re-checks; **hard cap two rounds**, then the coordinator implements the remainder inline).

What repeatedly earned its keep this phase:

- **Open questions get coordinator dispositions before the executor launches.** Nothing is resolved silently. All remaining B6 packets are already dispositioned.
- **Evaluations are adversarial and reproduce their own claims** (revert-probes, planted canaries, independent traces). Two findings this phase were *retracted by the evaluator's own experiment* after an executor refused a drain item with evidence — the refusal path works; honor it.
- **Fixtures must be driven through REAL verbs.** B6-5's worst finding was a fixture world seeded with raw SQL and invented payload keys — tests passing against a world that did not exist. The golden-fixture mechanism (Go compare-only assertor + vitest doubles importing the same files) exists to close exactly that.
- **A declared deviation is a referral, never an acceptance** — the coordinator rules. Several were sound; several were real defects.
- **Sanctioned-narrow outside-range fixes** are legitimate when a landed defect blocks the packet's own surface (this phase: the verbs/choices tag, the delta card's missing vocabulary, the store-side zero-price refusal) — ratify explicitly, keep the diff surgical, pin a regression.
- **Never commit while a stage agent is live in the tree**; use `git commit --only P3/STATE.md` for coordinator bookkeeping meanwhile.
- **Grounding for packet N+1 may overlap execution of packet N** (read-only + `--only` brief commit). Two writers on one path never overlap.
- Update STATE **before and after every step**. Push after milestone commits.

---

## 6. Invariants that must never be broken

- **The spec wins** — over model memory, reports, and existing code. A conflict, gap, or impossibility is *never* resolved silently: stop the packet, write an S00.9 amendment proposal, present it.
- **D1–D10 are fixed.** Never re-derive or relitigate.
- **Adopt, don't fork.** Exact pins; `components.lock` + lockgate cover every dependency, Go and npm; a new adoption needs its watch row (test-enforced) and gate ratification.
- **Every ⚙ number ships through the settings registry**; structural constants carry named reasons and are interim under the settings-tab directive.
- **No load-bearing metered paths**; the subscription-coverage rule holds; **money is read, never computed** — and never fabricated (the $0-price lesson).
- **Real-world facts are live-verified at time of use** — never from memory.
- **No auto-kill anywhere** (S14.4 / G1-D1.3).
- **Content-vs-telemetry line** (three packets deep now): the operator sees all *machinery* (runs, meters, workers, subscriptions-as-metadata) but **not** other members' personal *content* — memory entries, chat transcripts, and conflict-card metadata addressed to someone else. Deliberate, stated, and test-pinned in both directions.
- **Escape-by-default**: the web escape-scan allowlist is **EMPTY** and B6-8 keeps it empty (the preview iframe rides `src`, which trips no banned construct); B6-8 adds `srcdoc` to the banned set.
- **Stale never poses as live**; derive-from-log, no tickers, no client-side side-truths; honest absence over fabrication.
- Secrets are never committed. Host-level changes are proposed and approved, never applied unilaterally — units are **generated, not installed**.

---

## 7. Host and environment state you inherit

- **The 2026-07-28/29 sessions changed NOTHING on the host** — pure repo work (Go/TS code, migrations applied only to test DBs, $0 spend, no sudo/apt/unit/secret/canary). `web/node_modules` exists inside the repo (git-ignored); nothing installed globally.
- **Production front chain LIVE on this machine:** `tailscale serve` → Caddy on 127.0.0.1:8481 → the production unit on 8482; `sinet-control` + `sinet-broker` active. **Packet tests stay hermetic and never touch the live Caddy admin API/config** — real hazard, own tripwire (now self-scanning). The Vite dev proxy **requires** `VITE_SINET_ADDR` and fails fast when unset, precisely because 8482 is production's port here.
- **Local inference tier live-capable, user-level:** llama-swap v241; llama.cpp b10085 CUDA sm_120; weights in `~/.sinet-b45` (~38 GB, outside the repo). No driver/kernel/system-CUDA change, ever.
- **B5 organs:** promptfoo 0.121.19 + changedetection.io 0.55.8 installed (user-level); changedetection.io not running; `sinet-watchlist.service` generated-only; all four canaries disarmed.
- Service-context confined runs stay blocked by the AppArmor userns finding until the **hardening session** (needs sudo).

---

## 8. Model routing

Executors and finalizers are **always Opus**. Grounding and evaluation inherit the session model, with a **lossless Opus fallback** on any safeguard trip. B6's frontend scope is not security-dense; the whole phase ran clean on the inherited model. Coordinator sessions run at max effort.
