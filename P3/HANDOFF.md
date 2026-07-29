# P3 handoff — last rewritten 2026-07-30, mid-B6 (11/13 packets landed + validated; B6-7 part A closed, part B is the next act)

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* meant to get a fresh session oriented in two minutes. It is deliberately short and it goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If the two disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1, tag `spec-v1`; drafts in `Spec/drafts/` are canonical text)** → `Spec/benchmark-preregistration-v1.md` (BENCH-REG, signed — its registered numbers change only via its own §17) and `Spec/frontend-components-v1.md` (FC-v1) → `P3/CONVENTIONS.md` → `P3/STATE.md` → this file. `Research/` is a **closed archive**; `Docs/` is **read-only**.

---

## 1. Where the build is

**B0–B5 CLOSED. B6 (frontend — S15 + FC-v1) is OPEN, is the LAST phase, and stands at 11/13 landed + validated.** Nothing is mid-pipeline: the tree is clean, green, and fully pushed.

**Landed + validated:** B6-1 (read-side APIs) · B6-2A (approvals core) · B6-2B (budgets/oversight/drag-hint, mig 0017) · B6-2C (benchmark driver + `.direct` leg, mig 0018) · B6-3A (settings API + UISchema emitter + price surface, mig 0019) · B6-3B (deliverables/accept/preview) · B6-3C (memory) · B6-4 (the SPA scaffold: `web/` Vite + React 19 + TS, the four FC-v1 picks at their exact pins, the npm rail + lockgate over 240 packages, `go:embed` through the front chain, the app shell) · B6-5 (oversight surfaces) · B6-6 (the nine-kind approval inbox + the settings tab over JSON Forms) · **B6-7 part A** (the assistant BACKEND — `internal/chat`, migration 0020, 10 routes under `/api/chat`, owner-only transcripts, the `utility`-seat titling duty, the file exchange, client-routed turn dispatch).

**THE NEXT ACT: P3-B6-7 part B — the widget.** `/chat` is still a STUB. No grounding is needed and none remains anywhere in B6: the brief `P3/briefs/P3-B6-7.md` serves A **and** B, and **OQ1–10 are ratified** (part B's are OQ3, OQ5, OQ6, OQ7-render-half, OQ10). Launch the executor (opus) directly — §2 below.

**Pending after that: B6-8 → B6-9 → the gate.** Both briefs are written and dispositioned (B6-8 `b384d30`, OQ1–8 ratified; B6-9 `c1471ef`, OQ1–14 ratified).

---

## 2. This session's first acts, in order

1. **Session-entry checks:** read `P3/STATE.md`; confirm `git status` shows only the five long-standing operator items (`Research/02-…md` modified; untracked `Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/` — **never stage, revert, or "clean" these**); re-run the landing battery so you build on measured ground:
   `gofmt -l internal/ cmd/ tools/` · `go vet ./...` · `go build ./...` · `go test ./... -count=1` · `go run ./tools/lockgate` · `cd web && npm run typecheck && npm run test && npm run build`
2. **Launch the P3-B6-7 part-B EXECUTOR** (pipeline stage 2, fresh agent, **`model: opus`**). Scope = brief §1 **R11–R18** + their slice of R19–R21; rubric = brief §7 items **4 and 11–16** plus 17/18/20 (items 1/2/3/5–10/19 are part A's, already validated — do not re-litigate them). Binding shape, all ratified:
   - **ExternalStoreRuntime** (OQ5-(i)) — message/session state stays platform-owned, so a widget swap loses no data; **assistant-cloud NEVER wired and NEVER called** (two limbs: no import/config, and zero unscripted network in driven turns — `scriptedFetch` throws on anything unscripted).
   - **Synchronous turn verbs + relay-driven currency** (OQ5-(a)) — answers arrive WHOLE, the adapter's async generator yields once (an honest non-stream); "in-flight" = an outstanding POST over re-attachable server-side turn state.
   - **CLIENT routes the ladder** (OQ3-(a)) — NL turns → `/api/events/ask`; registry-driven pickers for Layer-0/Layer-1-direct; **open-SQL only by an explicit per-turn user act** (act-not-fallback); **task-start is an EXPLICIT UI act** (no model, cannot misfire); a neither-verb turn gets the card-shaped honest refusal with catalog choices, rendered as an answer.
   - **Hard-stop = abandon-the-turn** (OQ6) — a rendered explicit action; navigation calls nothing; a handed-off task is never cancelled by chat.
   - **Plain-text transcript** (OQ10) — zero new npm dependency, escape allowlist stays EMPTY.
   - **Intake in place** (OQ8-(i)) — the born task's OPEN intake card rendered inline from the served snapshot, answered through the LANDED verbs, plus a deep link.
   - Ready-made fixture ground: a **running** turn is staged in `chat-session` for the re-attach render, and **both** the empty and non-empty produced-chip renders now have committed fixtures.
   - The flip is exactly: `web/src/routes.ts:48` `owner: 'B6-7'` → `''` **and** `'chat'` into the `built` array. Patterns are never renamed.
3. **Then eval → triage → drain (cap 2) → land part B**, and B6-7 closes.
4. **Then B6-8** (review surfaces + try-it + accept UI + workforce map — note: the escape allowlist stays EMPTY and gains the `srcdoc` banned token; TBD-P3 map rendering is RESOLVED as owned).
5. **Then B6-9** (push + the S15.12 sweep — owned stdlib RFC 8291 crypto, migration **0021**, push-only service worker admitted, VAPID interim custody).
6. **Then the B6 GATE** — `P3/gates/B6-report.md` + the phase decision batch + **the operator's own UI click-through** (their 2026-07-20 directive). §4 lists what the batch must carry.

---

## 3. Counters (assert these before changing anything)

| Thing | Value |
|---|---|
| Event inventory | **99 minted / 5 declare-only = 104 registered** (pin at `internal/eventlog/contract_test.go:259`) |
| ⚙ settings | **118 keys / 33 domains** — `internal/settings/index.go` byte-unchanged through ALL of B5+B6 |
| `components.lock` | **29 entries** (21 Go-side + 8 frontend/toolchain; lockgate also covers **240 npm packages**) |
| Migrations | **0001–0020** (`user_version` 20). **0021 is RESERVED for B6-9's `push_subscriptions`** |
| Conformance seed rows | **12** |
| Go test packages | **44** green (`-race` clean on `internal/chat` + `internal/api`) |
| Web tests | **217 / 17 files** green (vitest, offline) |
| CONVENTIONS | §1–§44 (§44 carries drain-r1 / r1b / r2 / post-cap subsections — earlier sections are never rewritten) |
| Paid spend, all of B6 | **$0** (every stage fake-engine/hermetic) |

Git: **everything is pushed** — `origin/main` = `cd98a2a`. Nothing is committed-but-unpushed.

---

## 4. What the B6 gate batch must carry (accumulated all phase — do not lose these)

- **S16.4 #10 ratifications** for every frontend adoption: the four FC-v1 picks at their pins (@hello-pangea/dnd 18.0.1, react-diff-view 3.3.3, @assistant-ui/react 0.14.27, JSON Forms 3.8.0), the React/Vite/TS toolchain tree, Vitest + jsdom, `actions/setup-node`.
- **D4(b) paid golden sweep** — its surface now exists (the settings eval-results panel); the $2.10-projected / $5.00-stop run is registered and unexecuted, awaiting the operator's say-so.
- **The aggregate done-directly read gap** — no REST route serves the §13.2 measured-label aggregate (`benchmark.DomainReadout` unreachable); confirmed independently twice.
- **The Layer-1 catalog is AT its 30–50 band ceiling (50/50)** — the next catalog query breaches the band and forces a decision.
- **Gate-side authority narrowing** (memory): `Gate.authorize` lacks station-3 project-membership narrowing and `ResolveConflict` lacks affected-owner narrowing — both v0-contained because the HTTP transport is the sole production write path (enumerated + revert-probed), both fixed properly Gate-side later.
- **The per-unit unpriced-trace shape** — `PricedCost` cannot distinguish "free" from "unknown" per unit; the UI path is closed, the semantics question is an S00.9-adjacent metering follow-up.
- **The S10.6 disclosure key** — the downgrade-note render reads an explicit disclosure key no producer emits yet; the future ladder packet must mint it.
- **VAPID key custody** — control-plane-side at v0 (StateDirectory 0600); moving it broker-side is a hardening-session item.
- **Optional cosmetic S00.9 amendments** — add the chat and push family rows to the S15.2 table; align S15.5's "measured, n=…" paraphrase with BENCH-REG §13.2. **NEW (2026-07-29), same item:** the **family-12 admission for the eight `chat.*` types was UPHELD** by the independent evaluator (not a semantic stretch, so the §29 stop-and-amend edge did not fire) — and note for the record that **S14.2's own contract rule (2)** ("new types enter only by dated S00.9 amendment") is exactly what the landed §29 same-family reading overrides, settled precedent since B5-1.
- **Retention + timestamp presentation** — exchange-file retention is keep-until-deleted with NO sweeper by ratified decision (retention policy = an operator decision); and whether surfaces show relative/local times beside the verbatim UTC is deliberately left as an **operator-taste question for the click-through**.
- **Produced-files chips are honestly sparse at v0** — only uploads write the exchange folder; the platform-side producer socket is named-not-built. Worth showing at the click-through so the sparseness reads as designed, not broken.
- **Fleet seat/GPU fill**, the hardening session's accumulated list (AppArmor userns, `sinet-watchlist.service` install, changedetection.io start with its broker-custody key, socat/egress/run@/Landlock), and the standing operator housekeeping items (B5 report §7).

---

## 5. How the machinery works (do not reinvent it)

Every packet runs a **four-stage pipeline**, all stages launched by the coordinator as fresh-context background agents, strictly sequential within a packet: **grounding** → writes `P3/briefs/P3-<phase>-<n>.md` (handoff artifact *and* evaluation rubric) → **executor** (always Opus) → **evaluation** (fresh agent, judge ≠ executor, told to find what is wrong) → **drain** (coordinator triages [F1..Fn], executor/finalizer fixes, evaluator re-checks; **hard cap two rounds**, then the coordinator implements the remainder inline).

What repeatedly earned its keep this phase:

- **Open questions get coordinator dispositions before the executor launches.** All remaining B6 packets are already dispositioned.
- **Evaluations are adversarial and reproduce their own claims** (revert-probes, planted canaries, independent traces). **Name the specific things you want attacked** in the eval/re-check prompt — B6-7's worst defect (a crash permanently disabling one owner's produced-files attribution) was found because the coordinator named the stuck-window hazard in advance.
- **Probe your own assertions too.** A coordinator probe during B6-7's post-cap work showed one claimed-load-bearing write was behaviourally unobservable; the honest limit is now recorded in the code and §44 instead of shipping as an implied-but-untested claim.
- **A fix can be worse than the bug.** B6-7's round-2 fix introduced a *widening* (an orphaned turn went from blocking one session to suppressing attribution across all of an owner's sessions). Ask what the fix breaks, not just what it repairs.
- **Fixtures must be driven through REAL verbs** (the B6-5 root cause: a fixture world seeded with raw SQL and invented payload keys — tests passing against a world that did not exist). Staging the *timing* of a real verb is fine; hand-building the row is not.
- **A declared deviation is a referral, never an acceptance** — the coordinator rules. Executor refusals-with-evidence are legitimate and have been upheld (B6-4: the evaluator retracted its own finding after re-running the experiment).
- **Sanctioned-narrow outside-range fixes** are legitimate when a landed defect blocks the packet's own surface — ratify explicitly, keep the diff surgical, pin a regression.
- **Never commit while a stage agent is live in the tree**; use `git commit --only P3/STATE.md` for coordinator bookkeeping meanwhile.
- Update STATE **before and after every step**. Push after milestone commits.

---

## 6. Invariants that must never be broken

- **The spec wins** — over model memory, reports, and existing code. A conflict, gap, or impossibility is *never* resolved silently: stop the packet, write an S00.9 amendment proposal, present it.
- **D1–D10 are fixed.** Never re-derive or relitigate.
- **Adopt, don't fork.** Exact pins; `components.lock` + lockgate cover every dependency, Go and npm; a new adoption needs its watch row (test-enforced) and gate ratification.
- **Every ⚙ number ships through the settings registry**; structural constants carry named reasons and are interim under the settings-tab directive.
- **No load-bearing metered paths**; the subscription-coverage rule holds; **money is read, never computed** — and never fabricated.
- **Real-world facts are live-verified at time of use** — never from memory.
- **No auto-kill anywhere** (S14.4 / G1-D1.3).
- **Content-vs-telemetry line** (four packets deep now): the operator sees all *machinery* (runs, meters, workers, subscriptions-as-metadata) but **not** other members' personal *content* — memory entries, chat transcripts, conflict-card metadata addressed to someone else. In `internal/chat` this is **structural rather than policed**: every read takes a viewer and has **no role parameter at all**, so there is no operator limb to forget — pinned by `TestStoreHasNoOperatorLimbAtAll` and driven in both directions.
- **Escape-by-default**: the web escape-scan allowlist is **EMPTY** and B6-8 keeps it empty (the preview iframe rides `src`); B6-8 adds `srcdoc` to the banned set.
- **Stale never poses as live**; derive-from-log, no tickers, no client-side side-truths; **honest absence over fabrication** — and absence must fail *loud*, not silent (the B6-7 orphan lesson: a rule about absence that breaks quietly is worse than the ambiguity it prevents).
- Secrets are never committed. Host-level changes are proposed and approved, never applied unilaterally — units are **generated, not installed**.

---

## 7. Host and environment state you inherit

- ⚠️ **GitHub push identity — READ THIS BEFORE PUSHING.** `~/.gitconfig` routes credentials through `gh auth git-credential`, which uses gh's **active** account. On 2026-07-29 the active account was `sinet-ai`, which **cannot see this private repo**, so `git push` failed with a misleading `Repository not found` (GitHub 404s private repos you lack access to). The coordinator switched the active account to the repo owner **`dariannixda-eng`**, pushed successfully, and then **could not switch back — the restore command was blocked by the sandbox classifier**. So: **the active gh account is currently `dariannixda-eng`** and pushes work. The operator was asked to run `gh auth switch --user sinet-ai` if they want the previous state; **if they did, pushes will 404 again and the fix is to switch to `dariannixda-eng` for the push.** Do not misread that 404 as a missing or renamed repo.
- **The 2026-07-28/29/30 sessions changed NOTHING else on the host** — pure repo work (Go/TS code, migrations applied only to test DBs, $0 spend, no sudo/apt/unit/secret/canary). `web/node_modules` exists inside the repo (git-ignored); nothing installed globally.
- **Production front chain LIVE on this machine:** `tailscale serve` → Caddy on 127.0.0.1:8481 → the production unit on 8482; `sinet-control` + `sinet-broker` active. **Packet tests stay hermetic and never touch the live Caddy admin API/config** — real hazard, own tripwire. The Vite dev proxy **requires** `VITE_SINET_ADDR` and fails fast when unset, precisely because 8482 is production's port here.
- **The installed production binary is `/usr/local/bin/sinet`, dated Jul 20 20:35 (the B2-gate install, never rebuilt).** This is why migration 0020 could be edited in place during B6-7's drain: no production DB has ever applied it. Direct reads of `/var/lib/sinet/platform.db` are blocked by the sandbox classifier — the binary date is the available evidence, and it was not worked around.
- **Local inference tier live-capable, user-level:** llama-swap v241; llama.cpp b10085 CUDA sm_120; weights in `~/.sinet-b45` (~38 GB, outside the repo). No driver/kernel/system-CUDA change, ever.
- **B5 organs:** promptfoo 0.121.19 + changedetection.io 0.55.8 installed (user-level); changedetection.io not running; `sinet-watchlist.service` generated-only; all four canaries disarmed.
- Service-context confined runs stay blocked by the AppArmor userns finding until the **hardening session** (needs sudo).

---

## 8. Model routing

Executors and finalizers are **always Opus**. Grounding and evaluation inherit the session model, with a **lossless Opus fallback** on any safeguard trip. B6's frontend scope is not security-dense; the whole phase has run clean on the inherited model. Coordinator sessions run at max effort.
