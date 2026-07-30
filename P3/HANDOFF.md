# P3 handoff — last rewritten 2026-07-30 (end of session), mid-B6: **B6-8 part A LANDED; part B (the workforce map) is the next act and the LAST build work of the phase**

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* to orient a fresh session in two minutes. It goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If they disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1, tag `spec-v1`; drafts in `Spec/drafts/` are canonical text)** → `Spec/benchmark-preregistration-v1.md` (BENCH-REG, signed — registered numbers change only via its own §17) and `Spec/frontend-components-v1.md` (FC-v1) → `P3/CONVENTIONS.md` → `P3/STATE.md` → this file. `Research/` is a **closed archive**; `Docs/` is **read-only**.

---

## 1. Where the build is

**B0–B5 CLOSED. B6 (frontend — S15 + FC-v1) is OPEN and is the LAST phase.**

Of B6's **13 queue rows**, **11 are closed** (B6-1 · 2A · 2B · 2C · 3A · 3B · 3C · 4 · 5 · 6 · 7). **B6-8 is half-landed: part A (review surfaces + try-it + accept) landed and validated 2026-07-30; part B (the workforce map) is unbuilt.** **B6-9** (push + the S15.12 sweep) is untouched. *(Earlier handoffs said "12/13" by counting A/B halves as units — the row count above is the unambiguous version.)*

**`/workforce` is the ONLY remaining stub in the whole SPA.** Everything else is live.

Nothing is mid-pipeline. Tree clean, battery green, **everything pushed — `origin/main` = `5bd5e16`**.

---

## 2. This session's first acts, in order

1. **Session-entry checks:** read `P3/STATE.md`; confirm `git status` shows only the five long-standing operator items (`Research/02-…md` modified; untracked `Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/` — **never stage, revert, or "clean" these**); re-run the landing battery so you build on measured ground:
   `gofmt -l internal/ cmd/ tools/` · `go vet ./...` · `go build ./...` · `go test ./... -count=1` · `go run ./tools/lockgate` · `cd web && npm run typecheck && npm run test && npm run build`
2. **Launch the P3-B6-8 part-B EXECUTOR** (pipeline stage 2, fresh background agent, **`model: opus`**). **No grounding is needed and none remains anywhere in B6** — the brief `P3/briefs/P3-B6-8.md` serves A **and** B, and **OQ1–8 are ratified** (dispositions in the STATE log entry dated 2026-07-29). Hand it:
   - **Scope:** brief §1 **R11–R15** plus their slice of **R16–R21**. **Rubric = brief §7 items 14–18** plus the shared **19–22**. Items 1–13 are part A's, already landed and independently validated — **name them as barred from re-litigation.**
   - **Part B's ratified dispositions (OQ3–OQ7)** — restate them, they are not open for re-argument:
     - **OQ3** — a narrow exported `Roster` read in `internal/worker` (the `browse.go` declared-widening precedent) + `GET /api/workforce`; **visibility = the runs/telemetry precedent, so the OPERATOR SEES ALL** — workers are audited platform machinery with D10 gates, **not** personal content. The content-vs-telemetry line covers conversations and personal memory, not machinery.
     - **OQ4** — an api-side **derived** per-version read (the §42-B precedent: a bounded join over `routing.decided` ⋈ `verdict.recorded` ⋈ `RunMeter` money-**READ**), honest absences, empty-at-v0 truthful. This exists **because the catalog is at its 30–50 band ceiling and a catalog query is NOT addable.**
     - **OQ5** — render the registry's own vocabulary; **"helpers" is an honest structural statement** (per-run routing, no standing roster, never fabricated); helper-spawn causes optional via OQ4's read.
     - **OQ6** — **OWNED rendering. This disposition IS the dated resolution of TBD-P3(workforce-map rendering approach), resolved 2026-07-29** = owned semantic HTML/CSS, plus owned inline SVG only if a connection drawing is genuinely needed (household scale, empty-at-v0, view-only; a layout library would be an indefensible full-rail adoption). **It must land in §45-B verbatim**, and no unratified dependency may appear.
     - **OQ7** — multi-stage connections = automation workflow step chains + selector/duty facts. The task-pipeline story stays B6-5's.
   - **The flip is exactly:** `web/src/routes.ts` `workforce` row `owner: 'B6-8'` → `''`, **and** `'workforce'` into `routes.test.ts`'s `built` array (which currently holds **10** ids). **Patterns, titles and nav placement are never renamed** (§41-B).
   - **CONVENTIONS: append §45-B** (the §44-B / §43-B convention). Earlier sections are never rewritten; a corrected sentence must read truthfully **standing alone** (the §44 D12 lesson).
   - The standard executor guardrails and the launch-restated counters in §3 below.
3. **Then eval → triage → drain (cap 2) → land part B**, and **B6-8 closes**.
4. **Then B6-9** (push + the S15.12 sweep — owned stdlib RFC 8291 crypto, migration **0021**, push-only service worker admitted, VAPID interim custody). Brief written and dispositioned (`c1471ef`, OQ1–14 ratified).
5. **Then the B6 GATE** — `P3/gates/B6-report.md` + the phase decision batch + **the operator's own UI click-through** (their 2026-07-20 directive). §4 is the batch.

---

## 3. Counters (assert these before changing anything)

| Thing | Value |
|---|---|
| Event inventory | **99 minted / 5 declare-only = 104 registered** (pin `internal/eventlog/contract_test.go:259`) |
| ⚙ settings | **118 keys / 33 domains** — `internal/settings/index.go` byte-unchanged through ALL of B5+B6 |
| `components.lock` | **29 entries**; lockgate also covers **240 npm packages** |
| Migrations | **0001–0020** (`user_version` 20). **0021 is RESERVED for B6-9's `push_subscriptions`** |
| Conformance seed rows | **12** |
| Go test packages | **44** green |
| Web tests | **324 / 19 files** green (vitest, offline) |
| Shared fixture `cursor` | **64** — regeneration must produce **zero drift** |
| Escape scan | allowlist **EMPTY**, banned tokens **8** (incl. `srcdoc`), floor **42**, all 8 probed |
| Layer-0 views / catalog | **11 / 50** — the catalog is **AT its 30–50 band ceiling; a catalog query is NOT addable** |
| `built` routes array | **10** ids — part B flips `workforce`, the last stub |
| Bundle | **951.88 kB / gzip 286.74** (grew 851.73 → 951.88 in part A; a phone-complete question for the gate, not a defect) |
| CONVENTIONS | **§1–§45** (§44 = B6-7 part A, §44-B = its part B, §45 = B6-8 part A, each with drain/post-cap subsections) — **part B appends §45-B** |
| Paid spend, all of B6 | **$0** (every stage fake-engine/hermetic) |

---

## 4. What the B6 gate batch must carry (accumulated all phase — do not lose these)

- **S16.4 #10 ratifications** for every frontend adoption: the four FC-v1 picks at their pins (@hello-pangea/dnd 18.0.1, react-diff-view 3.3.3, @assistant-ui/react 0.14.27, JSON Forms 3.8.0), the React/Vite/TS toolchain tree, Vitest + jsdom, `actions/setup-node`. **Note for the record: the closure is NOT all-MIT — `diff-match-patch` is Apache-2.0** (`components.lock` never claimed otherwise).
- **D4(b) paid golden sweep** — its surface exists; the $2.10-projected / $5.00-stop run is registered and unexecuted, awaiting the operator's say-so.
- **The aggregate done-directly read gap** — no REST route serves the §13.2 measured-label aggregate (`benchmark.DomainReadout` unreachable); confirmed independently twice.
- **The Layer-1 catalog is AT its band ceiling (50/50)** — the next catalog query breaches it and forces a decision.
- **Gate-side authority narrowing** — `Gate.authorize` lacks station-3 project-membership narrowing and `ResolveConflict` lacks affected-owner narrowing; both v0-contained because the HTTP transport is the sole production write path.
- **The per-unit unpriced-trace shape** — `PricedCost` cannot distinguish "free" from "unknown" per unit.
- **The S10.6 disclosure key** — the downgrade-note render reads a disclosure key no producer emits yet.
- **VAPID key custody** — control-plane-side at v0 (StateDirectory 0600); broker-side is a hardening item.
- **Optional cosmetic S00.9 amendments** — add the chat and push family rows to the S15.2 table; align S15.5's "measured, n=…" paraphrase with BENCH-REG §13.2. The **family-12 admission for the eight `chat.*` types was UPHELD** independently. For the record: **S14.2's own contract rule (2)** is what the landed §29 same-family reading overrides — settled precedent since B5-1.
- **Retention + timestamp presentation** — exchange-file retention is keep-until-deleted with NO sweeper by ratified decision; relative/local times beside verbatim UTC is deliberately an **operator-taste question for the click-through**.
- **Produced-files chips are honestly sparse at v0** — only uploads write the exchange folder; the platform-side producer socket is named-not-built.
- **An interview card is served with NO answer verbs**, so the landed inbox renders it *"nothing here to press"* while the chat surface answers it fine. Neither surface is buggy — `intake.CardInterview` genuinely declares no card-level verbs and `readCardShape` faithfully derives none — but it is the same shape as the delta-card defect the B6-6 drain fixed **at the producer**. The decision (projector derives interview verbs / the inbox grows the slot editor / the card says what closes it) is the gate's.
- **The double read on tab mount is landed `useLive`/`EventStream` behaviour** — a subscriber that JOINS an existing source gets a synchronous `connected` resnapshot after its own mount read, so it reads twice; the opener reads once. Demonstrated twice with no packet code in the picture. Untouched by decision; the door, if it ever matters, is `EventStream.subscribe`.
- **`mission.test.tsx`'s queued-bucket `.sort()`** de-pins served order and predates B6-7 — landed B6-5 code, left alone on the same reasoning as the double read.
- **`writeSurface` logs every family's errors under the label `"intake surface"`** (`internal/api/intake_handlers.go`), so the object route and others are mislabelled. One-line landed fix, deliberately not taken locally (that would trade a cosmetic label for a real inconsistency across sibling routes).
- **Bundle growth** — 951.88 kB / gzip 286.74 after part A; worth a phone-complete look at the click-through.
- **Fleet seat/GPU fill**, the hardening session's list (AppArmor userns, `sinet-watchlist.service` install, changedetection.io start with its broker-custody key, socat/egress/run@/Landlock), and the standing operator housekeeping items (B5 report §7).

---

## 5. How the machinery works (do not reinvent it)

Every packet runs a **four-stage pipeline**, all stages launched by the coordinator as fresh-context background agents, strictly sequential within a packet: **grounding** → writes `P3/briefs/…` (handoff artifact *and* evaluation rubric) → **executor** (always Opus) → **evaluation** (fresh agent, judge ≠ executor, told to find what is wrong) → **drain** (coordinator triages, executor/finalizer fixes, evaluator re-checks; **hard cap two rounds**, then the coordinator implements the remainder inline and records that no fresh pass reviewed it).

What has repeatedly earned its keep — the 2026-07-30 session is the strongest evidence yet:

- **Name the specific hazards you want attacked**, in the eval prompt and again in each re-check. Every serious defect this session was found because it was named in advance. The highest-yield class: **"passes in jsdom / passes against a fixture, false in production"** — a render sourced from a frozen snapshot, an inventory table compared to itself, a scan whose predicate does not enforce its own message, a downgrade justified by an untested architectural claim.
- **Refusal-with-evidence outranks compliance, and four have now been upheld** (B6-4 D5; B6-7 part B r2; B6-8 part A twice). The best one: a coordinator prescription to reorder two writes was refused because `ValidateAnswer` refuses that answer shape with empty guidance — proving **the review door as landed could never have worked**, with its own test passing against a body the server rejects. **A coordinator premise is a claim, not evidence.** Invite refusal explicitly in every drain message.
- **A drain can widen, and a fix can be worse than the bug.** B6-7 part B's r1 fixed a fake fixture by seeding rows into the *shared* world with a run state the pipeline cannot produce — recreating the root cause it quoted. **Sweep the ripple after every drain that touches shared fixtures**, and ask what a fix breaks, not only what it repairs.
- **Fixing the record is not fixing the code.** Two corrections landed only in CONVENTIONS while the false sentence sat untouched at the assertion site. For a declared downgrade **the assertion site is the only place that matters**. Relatedly: when a fix falsifies a sentence already written, correct it **in place**.
- **Prove "nothing was weakened" by counting**, not by reading — assertion counts per touched test file. Two packets this phase weakened assertions inside a drain.
- **A tautological test is worse than no test.** Two were found this session: an inventory table checked against itself, and an invariant asserting that a `.map` ran. Both hid a live defect. Break the behaviour and watch the suite.
- **Evaluators that correct themselves are working.** One reported PASS, followed its own earlier pointer, found the defect still live, and reversed its verdict. Ask for that posture.
- **Every fix carries a test that fails on pre-fix code**, verified by reverting it. Prose-only corrections are the honest exception — do not bolt on a grep-for-a-sentence guard, which is the tautology above.
- **Never commit while a stage agent is live in the tree**; use `git commit --only P3/STATE.md` (or `P3/HANDOFF.md`) for coordinator bookkeeping meanwhile. **Verify before accusing** — a file appearing in a diff range may be your own bookkeeping commit inside that range (`git show --stat <commit>` settles it).
- Update STATE **before and after every step**. Push after milestone commits.

---

## 6. Invariants that must never be broken

- **The spec wins** — over model memory, reports, and existing code. A conflict, gap, or impossibility is *never* resolved silently: stop the packet, write an S00.9 amendment proposal, present it.
- **D1–D10 are fixed.** Never re-derive or relitigate.
- **Adopt, don't fork.** Exact pins; `components.lock` + lockgate cover every dependency, Go and npm; a new adoption needs its watch row and gate ratification; re-verify liveness at the pin at time of use.
- **Every ⚙ number ships through the settings registry**; structural constants carry named reasons and are interim under the settings-tab directive.
- **No load-bearing metered paths**; the subscription-coverage rule holds; **money is read, never computed** — and never fabricated.
- **Real-world facts are live-verified at time of use** — never from memory.
- **No auto-kill anywhere** (S14.4 / G1-D1.3).
- **Content-vs-telemetry line** — the operator sees all *machinery* (runs, meters, workers, subscriptions-as-metadata) but **not** other members' personal *content* (memory entries, chat transcripts, conflict cards addressed to someone else). In `internal/chat` this is **structural**: every read takes a viewer and has **no role parameter at all**. **Workforce/registry data is machinery, so the operator sees all of it** (B6-8 OQ3).
- **Escape-by-default**: the allowlist is **EMPTY and stays empty** — B6-8's OQ1 settled that it never needed to widen (an `<iframe src>` trips no banned token) and added `srcdoc` to the banned set instead. **The preview iframe is the ONE raw-HTML channel and it must be `sandbox`ed** (S13.3 and the platform's own `SanctionedRawHTML` both name it sandboxed) with top-navigation withheld.
- **A served snapshot is a RECORD, not live state** — read live state live. **Stale never poses as live**; derive-from-log; no tickers; no client-side side-truths; **honest absence over fabrication**, and absence must fail *loud*.
- **A content-addressed route must not serve bytes whose digest contradicts the sha in its own URL** — verify before serving (`review.OpenObjectVerified`; the `content_drift` posture).
- Secrets are never committed. Host-level changes are proposed and approved, never applied unilaterally — units are **generated, not installed**.

---

## 7. Host and environment state you inherit

- ⚠️ **GitHub push identity.** `~/.gitconfig` routes credentials through `gh auth git-credential`, which uses gh's **active** account. `sinet-ai` **cannot see this private repo**, so pushes under it fail with a misleading `Repository not found`. **As of 2026-07-30 the active account is `dariannixda-eng` (the repo owner) and pushes work** — verified with `gh auth status`. If a push 404s, check the active account first; do not misread it as a missing repo.
- **The 2026-07-28/29/30 sessions changed NOTHING else on the host** — pure repo work (Go/TS, migrations applied only to test DBs, $0, no sudo/apt/unit/secret/canary). `web/node_modules` exists inside the repo (git-ignored); nothing installed globally.
- **Production front chain LIVE on this machine:** `tailscale serve` → Caddy on 127.0.0.1:8481 → the production unit on 8482; `sinet-control` + `sinet-broker` active. **Packet tests stay hermetic and never touch the live Caddy admin API/config.** B6-8 part A verified this holds for preview sessions and the dual iframes (`TestAPITestsNeverNameTheLiveFrontChain`, non-tautological). The Vite dev proxy **requires** `VITE_SINET_ADDR` and fails fast when unset, precisely because 8482 is production's port here. **Preview portpool: production default 47600–47619; `internal/api`'s test range is 47900–47919** (don't confuse them — a security premise cited the wrong one once).
- **The installed production binary is `/usr/local/bin/sinet`, dated Jul 20 20:35 (the B2-gate install, never rebuilt)** — which is why migration 0020 could be edited in place during B6-7: no production DB has ever applied it. Direct reads of `/var/lib/sinet/platform.db` are blocked by the sandbox classifier.
- **Local inference tier live-capable, user-level:** llama-swap v241; llama.cpp b10085 CUDA sm_120; weights in `~/.sinet-b45` (~38 GB, outside the repo). No driver/kernel/system-CUDA change, ever.
- **B5 organs:** promptfoo 0.121.19 + changedetection.io 0.55.8 installed (user-level); changedetection.io not running; `sinet-watchlist.service` generated-only; all four canaries disarmed.
- Service-context confined runs stay blocked by the AppArmor userns finding until the **hardening session** (needs sudo).

---

## 8. Model routing

Executors and finalizers are **always Opus**. Grounding and evaluation inherit the session model, with a **lossless Opus fallback** on any safeguard trip. B6's frontend scope is not security-dense. **Reading logged 2026-07-30:** with an Opus coordinator "inherit" resolves to Opus, and *judge ≥ executor* (the D3-ratified rule, which ranks first) was taken to outweigh cross-model diversity — independence came from fresh context plus hazards named in advance. It worked: the judge returned FAIL on every pass this session, each time with demonstrated defects, and once corrected its own verdict. **Round 2 of a drain goes to a FRESH finalizer** rather than the executor when the executor's own blind spot is what the round is fixing. Coordinator sessions run at max effort.
