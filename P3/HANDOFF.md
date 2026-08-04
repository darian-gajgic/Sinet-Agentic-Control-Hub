# P3 handoff — last rewritten 2026-08-04: **THE BUILD IS FINISHED and the B6 GATE IS CLOSED. The directed UI batch is open — the design-approach declaration comes FIRST and needs operator approval.**

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* to orient a fresh session in two minutes. It goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If they disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1, tag `spec-v1`; drafts in `Spec/drafts/` are canonical text)** → `Spec/benchmark-preregistration-v1.md` (BENCH-REG, signed) and `Spec/frontend-components-v1.md` (FC-v1) → `P3/CONVENTIONS.md` → `P3/STATE.md` → this file. `Research/` is a **closed archive**; `Docs/` is **read-only**.

---

## 1. Where the build is

**B0–B5 CLOSED. B6 COMPLETE — 13 of 13 packets built and validated. There is no unbuilt route left in the SPA (`built` = 11 = the whole table).**

**The B6 gate is CLOSED (2026-08-04) and work RESUMES under its directions** — the UI batch: design-approach declaration (operator-approved) → the fourteen controls → the D8 pass → the D6 upgrade (coordinator-executed, `sudo -v`) → bring-up. The gate files:

- `P3/gates/B6-report.md` — evidence, deviations, **eight decisions, ALL ANSWERED — §9 is the FINAL record**, a fourteen-item carried block (ratified en bloc), standing operator items, and the click-through findings A-1/A-2/C-1.
- `P3/gates/B6-clickthrough.sh` — **one command**, tested end to end, refuses to touch production.

Nothing is mid-pipeline. Tree clean apart from the long-standing operator files. **Everything pushed (re-verified 2026-08-04, after the module-rename roll-forward — see STATE).**

---

## 2. This session's first acts, in order

1. **The gate is CLOSED and the batch is directed.** First act: the **design-approach proposal** — the Nexus-frontend survey + live GitHub sourcing (status/results in the STATE log), synthesized and presented to the operator. **OPERATOR APPROVAL of that declaration is required before any D3 packet is cut** — this is the operator's own D3 condition, and presenting it is the next sanctioned pause.
2. **After approval:** cut and run the D3 packets in the declared design language (order in STATE: cancel → pause → budget → benchmark opt-in → memory family → history ask/search), folding in the C-1 login fix, the D5 timestamp render and the two cosmetic S00.9 amendments; then the D8 pass over the pre-existing routes; then the D6 upgrade ceremony (coordinator-executed, `sudo -v`, prod-DB backup first); then bring-up (S19.6) with the D2 sweep.
3. **Do not re-open closed phases or re-litigate ratified dispositions.** B6's OQ sets are all ratified and recorded in STATE's log.
4. Session-entry battery, so you build on measured ground:
   `gofmt -l internal/ cmd/ tools/` · `go vet ./...` · `go build ./...` · `go test ./... -count=1` · `go run ./tools/lockgate` · `cd web && npm run typecheck && npm run test && npm run build`

---

## 3. Counters (assert these before changing anything — all measured 2026-07-30)

| Thing | Value |
|---|---|
| Go test packages | **45** green, 0 failures, **14 sanctioned pre-existing skips** (env-gated live/GPU/paid legs — **the correct number is 14, not 0**) |
| Web tests | **432 / 23 files** (vitest, offline) |
| Bundle | **976.99 kB / gzip 292.96** |
| `components.lock` | **29** entries; lockgate also covers **240 npm packages** |
| Event inventory | **102 minted / 5 declare-only = 107 registered** |
| ⚙ settings | **118 keys / 33 domains** — `internal/settings/index.go` byte-unchanged through all of B5 and B6 |
| Migrations | **0001–0021** (`user_version` 21) |
| Conformance seed rows | **12** |
| Layer-0 views / catalog | **11 / 50** — the catalog is **AT its 30–50 band ceiling** |
| `built` routes | **11 — the whole table; no stub remains** |
| Escape scan | allowlist **EMPTY**, banned tokens **8**, floor **50** |
| Shared fixture `cursor` | **86** — regeneration must produce zero drift |
| CONVENTIONS | **§1–§46** (§45-B = B6-8 part B, §46 = B6-9, each with drain/post-cap subsections) |
| Paid spend, all of B6 | **$0**; zero pushes left this machine |

---

## 4. What the gate DECIDED (2026-08-04 — the full record is `P3/gates/B6-report.md` §9)

All eight answered: **D1/D4/D7 ratified** (D7 incl. the two cosmetic S00.9 amendments, applied in the UI-batch window) · **D2 authorized** — runs at bring-up after D6 ($2.10 proj / $5.00 stop) · **D3 conditional: declare the HOW first** — the Nexus frontend + other GitHub tools as copy/reference sources; all fourteen controls then built in the declared language (order: cancel → pause → budget → benchmark opt-in → memory family → history ask/search) · **D5 as recommended** (relative-beside-UTC on live surfaces, UTC-only in audit; taste delegated) · **D6 authorized, coordinator-executed via `sudo -v`** — one upgrade after the UI batch, prod-DB backup first · **D8 ratified**, its declaration pulled BEFORE D3 by the D3 condition.

---

## 5. How the machinery works (do not reinvent it)

Four-stage pipeline per packet, all stages coordinator-launched as fresh background agents, strictly sequential: **grounding** → `P3/briefs/…` → **executor** (always Opus) → **evaluation** (fresh, judge ≠ executor, told to find what is wrong) → **drain** (triage, fix, re-check; **hard cap two rounds**, then coordinator-inline and recorded as reviewed by no fresh pass).

What earned its keep across B6-8 and B6-9 — the strongest evidence in the phase:

- **Name the specific hazards you want attacked**, in the eval prompt and again in every re-check. Every serious defect this phase was found because it was named in advance. Highest-yield class: **"passes in jsdom / passes against a fixture, false in production."**
- **A number asserted is not a number measured.** Two coordinator figures were wrong this session ("0 skips", "three new gaps") and both were caught by the passes below. Count it, or do not claim it. Prove "nothing was weakened" by **assertion counts**, not by reading.
- **Refusal-with-evidence outranks compliance — six upheld in P3**, three against coordinator instructions. The best two: a review door that could never have worked, and a **23-input differential** proving a predicate refactor moved the renderer and not the executor.
- **A tautological test is worse than no test**, and its subtlest form is **a web assertion that reads a committed fixture** — it cannot fail on a server change no matter what its message claims. Instances remain in landed B5/B6 tests (gate list).
- **Fix the class, not the instance.** B6-8 shipped the same fabricated-figure defect in four currencies; B6-9's detector was fixed in one file and survived in the twentieth.
- **Verify before accusing** — a file in a diff range may be your own bookkeeping commit (`git show --stat <sha>` settles it).
- **Never commit while a stage agent is live in the tree**; use `git commit --only P3/STATE.md`. Update STATE before and after every step. Push after milestone commits.

---

## 6. Invariants that must never be broken

- **The spec wins** — over model memory, reports, and existing code. A conflict, gap or impossibility is *never* resolved silently: stop, write an S00.9 amendment proposal, present it.
- **D1–D10 are fixed.** Never re-derive or relitigate.
- **Adopt, don't fork.** Exact pins; `components.lock` + lockgate cover every dependency, Go and npm.
- **Every ⚙ number ships through the settings registry.** Structural constants carry named reasons and are interim under the settings-tab directive.
- **No load-bearing metered paths; money is read, never computed — and never fabricated.** Honest absence over fabrication, and absence must fail loud.
- **Real-world facts are live-verified at time of use** — never from memory. B6-9 corrected three brief facts this way.
- **No auto-kill anywhere.** **No silent caps** — a bound that truncates must say so where the number renders.
- **Content-vs-telemetry line** — the operator sees all *machinery* but not other members' personal *content*. Workforce data is machinery (B6-8 OQ3, and D4 above asks you to ratify that).
- **Escape-by-default**: the allowlist is EMPTY and stays empty; the preview iframe is the one raw-HTML channel and is sandboxed with top-navigation withheld.
- **A capability URL is a secret.** Push endpoints are stored, never rendered, and never written into an audit row.
- **A content-addressed route must not serve bytes whose digest contradicts its own URL.**
- Secrets are never committed. Host-level changes are proposed and approved, never applied unilaterally — units are **generated, not installed**.

---

## 7. Host and environment state you inherit

- **Nothing on the host changed this session** — pure repo work. $0, no sudo, no apt, no unit, no secret, no canary. Migrations applied only to test and throwaway databases.
- **Production front chain LIVE:** `tailscale serve` → Caddy on 127.0.0.1:8481 → the production unit on 8482; `sinet-control` + `sinet-broker` active. **Verified still active and still owning their ports while the click-through ran on 8483.**
- **The installed production binary is `/usr/local/bin/sinet`, dated 20 July** (the B2-gate install, never rebuilt) — which is why no production DB has applied 0020 or 0021. **That is gate decision D6.**
- **The click-through leaves a throwaway state dir at `~/.sinet-b6-clickthrough`** (binary + fresh DB + logs). `./P3/gates/B6-clickthrough.sh --clean` removes it.
- ⚠ **GitHub push identity** — `~/.gitconfig` routes credentials through `gh auth git-credential`, using gh's **active** account. `sinet-ai` cannot see this private repo and its pushes fail with a misleading `Repository not found`. The active account is `darian-gajgic` and pushes work.
- Local inference tier live-capable, user-level: llama-swap v241, llama.cpp b10085 CUDA sm_120, weights in `~/.sinet-b45` (~38 GB, outside the repo). No driver/kernel/system-CUDA change, ever.
- **B5 organs:** promptfoo 0.121.19 + changedetection.io 0.55.8 installed (user-level); changedetection.io not running; `sinet-watchlist.service` generated-only; all four canaries disarmed.
- Service-context confined runs stay blocked by the AppArmor userns finding until the **hardening session** (needs sudo).

---

## 8. Model routing

Executors and finalizers are **always Opus**. Grounding and evaluation inherit, with a lossless Opus fallback on any safeguard trip — and **Opus outright on crypto/capability-URL vocabulary** (B6-9's evaluation ran there by that rule). *Judge ≥ executor* ranks first, so with an Opus coordinator "inherit" resolves to Opus; independence comes from fresh context plus hazards named in advance, and it has worked — the judge returned FAIL on every pass across B6-8 and B6-9, each time with demonstrated defects, and corrected its own verdict four times. **Round 2 of a drain goes to a FRESH finalizer** when the executor's own blind spot is what the round is fixing. Coordinator sessions run at max effort.
