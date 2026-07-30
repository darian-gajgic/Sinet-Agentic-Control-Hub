# P3 handoff — last rewritten 2026-07-30, mid-B6 (12/13 packets landed + validated; B6-7 CLOSED both parts, B6-8 part A is in flight)

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* meant to get a fresh session oriented in two minutes. It is deliberately short and it goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If the two disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1, tag `spec-v1`; drafts in `Spec/drafts/` are canonical text)** → `Spec/benchmark-preregistration-v1.md` (BENCH-REG, signed — its registered numbers change only via its own §17) and `Spec/frontend-components-v1.md` (FC-v1) → `P3/CONVENTIONS.md` → `P3/STATE.md` → this file. `Research/` is a **closed archive**; `Docs/` is **read-only**.

---

## 1. Where the build is

**B0–B5 CLOSED. B6 (frontend — S15 + FC-v1) is OPEN, is the LAST phase, and stands at 12/13 landed + validated.**

**Landed + validated:** B6-1 (read-side APIs) · B6-2A (approvals core) · B6-2B (budgets/oversight/drag-hint, mig 0017) · B6-2C (benchmark driver + `.direct` leg, mig 0018) · B6-3A (settings API + UISchema emitter + price surface, mig 0019) · B6-3B (deliverables/accept/preview) · B6-3C (memory) · B6-4 (the SPA scaffold) · B6-5 (oversight surfaces) · B6-6 (the nine-kind approval inbox + the settings tab) · **B6-7 BOTH PARTS — part A the assistant backend, part B the `/chat` widget.**

**IN FLIGHT: P3-B6-8 part A** (review surfaces + try-it + accept UI) — executor launched 2026-07-30, `model: opus`. Its brief `P3/briefs/P3-B6-8.md` (`b384d30`) serves A **and** B, and **OQ1–8 are ratified** (log entry 2026-07-29). If that agent died, relaunch the same stage; §2 has the launch shape.

**Pending after that: B6-8 part B (the workforce map) → B6-9 → the B6 gate.** B6-9's brief is written and dispositioned (`c1471ef`, OQ1–14 ratified).

---

## 2. This session's first acts, in order

1. **Session-entry checks:** read `P3/STATE.md`; confirm `git status` shows only the five long-standing operator items (`Research/02-…md` modified; untracked `Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/` — **never stage, revert, or "clean" these**); re-run the landing battery so you build on measured ground:
   `gofmt -l internal/ cmd/ tools/` · `go vet ./...` · `go build ./...` · `go test ./... -count=1` · `go run ./tools/lockgate` · `cd web && npm run typecheck && npm run test && npm run build`
2. **Find out where B6-8 part A stands** — check `git log --oneline -5` for a `P3-B6-8 part A` commit and read the latest STATE log entry. Then continue its pipeline from whatever stage is next: eval → triage → drain (cap 2) → land.
3. **Then B6-8 part B** (the workforce map): OQ3/OQ4/OQ5/OQ6/OQ7 are its ratified dispositions, and **OQ6 IS the dated resolution of TBD-P3(workforce-map rendering) = owned semantic HTML/CSS** (a layout library would be an indefensible full-rail adoption at household scale) — that text goes into §45 verbatim.
4. **Then B6-9** (push + the S15.12 sweep — owned stdlib RFC 8291 crypto, migration **0021**, push-only service worker admitted, VAPID interim custody).
5. **Then the B6 GATE** — `P3/gates/B6-report.md` + the phase decision batch + **the operator's own UI click-through** (their 2026-07-20 directive). §4 lists what the batch must carry.

---

## 3. Counters (assert these before changing anything)

| Thing | Value |
|---|---|
| Event inventory | **99 minted / 5 declare-only = 104 registered** (pin at `internal/eventlog/contract_test.go:259`) |
| ⚙ settings | **118 keys / 33 domains** — `internal/settings/index.go` byte-unchanged through ALL of B5+B6 |
| `components.lock` | **29 entries** (21 Go-side + 8 frontend/toolchain; lockgate also covers **240 npm packages**) |
| Migrations | **0001–0020** (`user_version` 20). **0021 is RESERVED for B6-9's `push_subscriptions`** |
| Conformance seed rows | **12** |
| Go test packages | **44** green |
| Web tests | **269 / 18 files** green (vitest, offline) |
| Shared fixture `cursor` | **46** — regeneration must produce ZERO drift |
| Layer-0 views / catalog | **11 / 50** — the catalog is **AT its 30–50 band ceiling; a catalog query is NOT addable** |
| `built` routes array | **9** ids (B6-8 flips `deliverable`, then `workforce`) |
| CONVENTIONS | §1–§44 (§44 = B6-7 part A, **§44-B** = part B, each with drain/post-cap subsections) — **B6-8 appends §45**; earlier sections are never rewritten |
| Paid spend, all of B6 | **$0** (every stage fake-engine/hermetic) |

Git: `origin/main` = **`992a299`** at the time of writing. Push identity: see §7.

---

## 4. What the B6 gate batch must carry (accumulated all phase — do not lose these)

- **S16.4 #10 ratifications** for every frontend adoption: the four FC-v1 picks at their pins (@hello-pangea/dnd 18.0.1, react-diff-view 3.3.3, @assistant-ui/react 0.14.27, JSON Forms 3.8.0), the React/Vite/TS toolchain tree, Vitest + jsdom, `actions/setup-node`.
- **D4(b) paid golden sweep** — its surface exists (the settings eval-results panel); the $2.10-projected / $5.00-stop run is registered and unexecuted, awaiting the operator's say-so.
- **The aggregate done-directly read gap** — no REST route serves the §13.2 measured-label aggregate (`benchmark.DomainReadout` unreachable); confirmed independently twice.
- **The Layer-1 catalog is AT its 30–50 band ceiling (50/50)** — the next catalog query breaches the band and forces a decision.
- **Gate-side authority narrowing**: `Gate.authorize` lacks station-3 project-membership narrowing and `ResolveConflict` lacks affected-owner narrowing — both v0-contained because the HTTP transport is the sole production write path, both fixed properly Gate-side later.
- **The per-unit unpriced-trace shape** — `PricedCost` cannot distinguish "free" from "unknown" per unit.
- **The S10.6 disclosure key** — the downgrade-note render reads an explicit disclosure key no producer emits yet.
- **VAPID key custody** — control-plane-side at v0 (StateDirectory 0600); moving it broker-side is a hardening item.
- **Optional cosmetic S00.9 amendments** — add the chat and push family rows to the S15.2 table; align S15.5's "measured, n=…" paraphrase with BENCH-REG §13.2. The **family-12 admission for the eight `chat.*` types was UPHELD** by the independent evaluator. For the record: **S14.2's own contract rule (2)** ("new types enter only by dated S00.9 amendment") is what the landed §29 same-family reading overrides — settled precedent since B5-1.
- **Retention + timestamp presentation** — exchange-file retention is keep-until-deleted with NO sweeper by ratified decision; relative/local times beside verbatim UTC is deliberately an **operator-taste question for the click-through**.
- **Produced-files chips are honestly sparse at v0** — only uploads write the exchange folder; the platform-side producer socket is named-not-built.
- **NEW (B6-7 part B): an interview card is served with NO answer verbs, so the landed inbox renders it "nothing here to press"** while the chat surface answers it fine. Nothing is a bug in either surface — an `intake.CardInterview` genuinely declares no card-level verbs and `readCardShape` faithfully derives none — but it is the same shape as the delta-card defect the B6-6 drain fixed **at the producer**. The decision (projector derives interview verbs / the inbox grows the slot editor / the card says what closes it) is the gate's.
- **NEW (B6-7 part B): the double read on tab mount is landed `useLive`/`EventStream` behaviour**, demonstrated with no chat code in the picture: a subscriber that JOINS an existing source gets a synchronous `connected` resnapshot after its own mount read, so it reads twice. Untouched by decision; the door, if it ever matters, is `EventStream.subscribe`.
- **NEW (B6-7 part B): `mission.test.tsx`'s queued-bucket `.sort()`** de-pins served order and predates the packet (present at `8d04d2d`) — landed B6-5 code, left alone on the same reasoning as the double read.
- **Fleet seat/GPU fill**, the hardening session's accumulated list (AppArmor userns, `sinet-watchlist.service` install, changedetection.io start with its broker-custody key, socat/egress/run@/Landlock), and the standing operator housekeeping items (B5 report §7).

---

## 5. How the machinery works (do not reinvent it)

Every packet runs a **four-stage pipeline**, all stages launched by the coordinator as fresh-context background agents, strictly sequential within a packet: **grounding** → writes `P3/briefs/P3-<phase>-<n>.md` (handoff artifact *and* evaluation rubric) → **executor** (always Opus) → **evaluation** (fresh agent, judge ≠ executor, told to find what is wrong) → **drain** (coordinator triages [F1..Fn], executor/finalizer fixes, evaluator re-checks; **hard cap two rounds**, then the coordinator implements the remainder inline).

What repeatedly earned its keep this phase:

- **Open questions get coordinator dispositions before the executor launches.** Every remaining B6 packet is already dispositioned.
- **Name the specific hazards you want attacked** in the eval and re-check prompts. B6-7's two worst defects were both found this way. The highest-yield class to name: **"passes in jsdom, false in production"** — a render sourced from a client cache or a frozen snapshot, an inventory table that compares to itself, a scan whose predicate does not enforce its own message.
- **A fix can be worse than the bug, and a drain can widen.** B6-7 part B's drain r1 fixed a fake fixture by seeding rows into the SHARED fixture world with a run state the pipeline cannot produce — recreating the very root cause it quoted. **Every drain that touches shared fixtures gets its ripple swept.**
- **Refusal-with-evidence is legitimate and has been upheld twice** (B6-4 D5; B6-7 part B r2, where the finalizer refuted a coordinator premise drawn from a CONVENTIONS paraphrase). **A coordinator premise is not evidence** — if you assert how the code works when directing a fix, mark it as a claim to verify.
- **Probe your own assertions, and correct your own record.** Post-cap coordinator work must break the production code and watch a named test fail; and when a fix falsifies a sentence already written in CONVENTIONS, correct that sentence **in place** so it reads truthfully standing alone (the §44 D12 lesson).
- **Fixtures must be driven through REAL verbs** — but check what "real" reaches: `internal/api`'s fixture world reaches the pipeline through a **hand-written double** (`fixtureIntake`), so "real handlers" is true of the transport and silent about the seam behind it.
- **Never commit while a stage agent is live in the tree**; use `git commit --only P3/STATE.md` (or `P3/HANDOFF.md`) for coordinator bookkeeping meanwhile.
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
- **Content-vs-telemetry line**: the operator sees all *machinery* (runs, meters, workers, subscriptions-as-metadata) but **not** other members' personal *content* — memory entries, chat transcripts, conflict cards addressed to someone else. In `internal/chat` this is **structural**: every read takes a viewer and has **no role parameter at all**. Workforce/registry data is machinery, not content — the operator sees all of it (B6-8 OQ3).
- **Escape-by-default**: the web escape-scan allowlist is **EMPTY and stays empty** — B6-8's OQ1 settled that it never needed to widen (an `<iframe src>` trips no banned token) and adds `srcdoc` to the banned set instead. The preview iframe is the ONE raw-HTML channel.
- **Stale never poses as live**; derive-from-log, no tickers, no client-side side-truths; **honest absence over fabrication**, and absence must fail *loud*, not silent. **A served snapshot is a RECORD, not live state** — read live state live (the B6-7 part-B RES-1 lesson).
- Secrets are never committed. Host-level changes are proposed and approved, never applied unilaterally — units are **generated, not installed**.

---

## 7. Host and environment state you inherit

- ⚠️ **GitHub push identity.** `~/.gitconfig` routes credentials through `gh auth git-credential`, which uses gh's **active** account. `sinet-ai` **cannot see this private repo**, so pushes under it fail with a misleading `Repository not found` (GitHub 404s private repos you lack access to). **As of 2026-07-30 the active account is `dariannixda-eng` (the repo owner) and pushes work** — verified with `gh auth status` this session. If a push 404s, check the active account first; do not misread it as a missing or renamed repo.
- **The 2026-07-28/29/30 sessions changed NOTHING else on the host** — pure repo work (Go/TS code, migrations applied only to test DBs, $0 spend, no sudo/apt/unit/secret/canary). `web/node_modules` exists inside the repo (git-ignored); nothing installed globally.
- **Production front chain LIVE on this machine:** `tailscale serve` → Caddy on 127.0.0.1:8481 → the production unit on 8482; `sinet-control` + `sinet-broker` active. **Packet tests stay hermetic and never touch the live Caddy admin API/config** — a real hazard, and **B6-8's preview sessions are exactly where it bites**. The Vite dev proxy **requires** `VITE_SINET_ADDR` and fails fast when unset, precisely because 8482 is production's port here.
- **The installed production binary is `/usr/local/bin/sinet`, dated Jul 20 20:35 (the B2-gate install, never rebuilt)** — which is why migration 0020 could be edited in place during B6-7: no production DB has ever applied it. Direct reads of `/var/lib/sinet/platform.db` are blocked by the sandbox classifier.
- **Local inference tier live-capable, user-level:** llama-swap v241; llama.cpp b10085 CUDA sm_120; weights in `~/.sinet-b45` (~38 GB, outside the repo). No driver/kernel/system-CUDA change, ever.
- **B5 organs:** promptfoo 0.121.19 + changedetection.io 0.55.8 installed (user-level); changedetection.io not running; `sinet-watchlist.service` generated-only; all four canaries disarmed.
- Service-context confined runs stay blocked by the AppArmor userns finding until the **hardening session** (needs sudo).

---

## 8. Model routing

Executors and finalizers are **always Opus**. Grounding and evaluation inherit the session model, with a **lossless Opus fallback** on any safeguard trip. B6's frontend scope is not security-dense. **Reading logged 2026-07-30:** with an Opus coordinator, "inherit" resolves to Opus, and *judge ≥ executor* (the D3-ratified rule that ranks first) was taken to outweigh cross-model diversity on the phase's heaviest widget — independence came from fresh context plus a hazard list named in advance, and it worked (the judge returned FAIL three times running with demonstrated defects). Coordinator sessions run at max effort.
