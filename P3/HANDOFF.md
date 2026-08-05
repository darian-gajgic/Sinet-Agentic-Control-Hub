# P3 handoff — last rewritten 2026-08-05 (end of session): **THE UI BATCH IS 4/7 LANDED AND ALL FOURTEEN D3 CONTROLS ARE BUILT — the sweep's gap list reads 0 routes / 0 shapes. Next act: cut and ground packet UI-5 (D8 pass I — the oversight surfaces restyle). No gate is open, no approval is pending.**

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* to orient a fresh session in two minutes. It goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If they disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1 + applied amendments A1–A10; drafts in `Spec/drafts/` are canonical text)** → `Spec/benchmark-preregistration-v1.md` (BENCH-REG, signed, untouched) and `Spec/frontend-components-v1.md` (FC-v1) → `P3/CONVENTIONS.md` (§1–§50) → **`P3/design/design-approach-proposal.md` (APPROVED 2026-08-04, operator verbatim in its §6 — the batch's design authority, subordinate to the spec on all behavior)** → `P3/STATE.md` → this file. `Research/` is a **closed archive**; `Docs/` is **read-only**.

---

## 1. Where the build is

**B0–B6 ALL CLOSED. The post-gate UI batch (queue in STATE above "Operator hands-on items") stands 4/7:**

- **UI-1 foundation LANDED** (`13e7f5d`+`b10999d`+`c66f164`+`6b36b87`): Tailwind v4 + shadcn copy-in through the rail, the Nexus token pour on `:root` (shadcn names + Sinet extensions + re-pointed legacy nine), bundled Fontsource Inter/JetBrains Mono + lucide (zero CDN), aurora + shell restyle, the eleven-primitive kit in `web/src/ui/` + `cn()`, density root classes, the D5 `Timestamp` primitive, reduced-motion kill-switch + kit-motion/kit-color scans.
- **UI-2 controls I LANDED** (`84a4875`+`7344791`+`094abb9`): cancel (task detail, state-computed, confirm tells the sequential-abort truth), pause (fleet, P-T08-4 visible), budget editor (no client figure, strict integer parse); the §48 controls idioms (`useAct`/`refusalOf`/`nothingFired`/confirm pattern); sanctioned narrow read #1 — the `automation` block on `GET /api/meters` from the pre-existing `PauseStore.Paused`.
- **UI-3 controls II LANDED** (`357a555`+`b710204`+`5642ebd`+`8d3312c`): `/memory` + `/memory/:id` routes (sanctioned addition), the six-verb memory family (gate-honest writes, retire-vs-delete confirms word-true to S09, content-vs-telemetry negative asserted on a SERVED empty body), the benchmark opt-in as a persistent inbox panel (BENCH-REG-faithful, figure-free); sanctioned narrow read #2 — the `opt_in` block on the verdicts GET via `BenchmarkSurface.OptedIn`; §49's ActConfirm disabled≠busy + draft-preserved-after-refusal.
- **UI-4 controls III LANDED** (`82c711a`+`d45f958`+`b2b1544`+`c855a40`): history ask/search (card choice SELECTS — no blind-fire; `q` verbatim; layer-2 lower-confidence flag both directions; band wall held — catalog exactly 50), the C-1 fix (dev posture reaches the login picker, Sign out only when real, dev Sign-in affordance ships; App.tsx wall held), the D5 map applied (13 live migrations + ParkedUntil relative-beside-UTC; 18 audit sites stay `Stamp`; UTC-never-dropped asserted per surface), the two S00.9 amendments **A9/A10 applied with independent byte-proof** (concat sha `9ceb914d…`; exactly three Spec/ files moved; BENCH-REG untouched), and **the fourteenth control — the S13.9 follow-up spawn on the deliverable's doors-as-data** (it was in NO queue row; the original cut missed it against the sweep's 14-route count — coordinator miss, logged, folded in by OQ1(a)).

**Remaining queue: UI-5** (D8 pass I — mission control, board, task detail + the tool-call timeline rail, fleet, workforce) → **UI-6** (D8 pass II — inbox, settings, login/push) → **UI-7** (D8 pass III — chat + review restyle, assistant-card rework in operator words, the self-teaching layer app-wide: tour + one-shot hints + teaching empty states, background-run etiquette, ToastProvider production mount, **the dev-only seeded demo world**) → **exit** (full battery + bundle report → operator click-through on the seeded world → D6 upgrade ceremony (`sudo -v`, prod-DB backup first) → bring-up per S19.6 with the D2 paid sweep ~$2.10/$5.00 stop).

Nothing is mid-pipeline. No background agents live. Tree clean apart from the four long-standing operator files (`Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/` — never staged). **Everything pushed through `27f2d2e`** (origin `darian-gajgic/Sinet-Agentic-Control-Hub`).

## 2. This session's first acts, in order

1. Session-entry battery before building on measured ground: `cd web && npm run typecheck && npm run test && npm run build` · `gofmt -l internal/ cmd/ tools/` · `go vet ./...` · `go build ./...` · `go test ./... -count=1` (over the real dist) · `go run ./tools/lockgate`. Expect the §3 counters exactly.
2. **Cut and ground packet UI-5** (D8 pass I). Grounding read-first: the approved proposal (§3 pattern vocabulary — Nexus anatomy + Odysseus behavior ideas reimplemented; §5 constraints) + S15.5/S15.10 + the existing B6-5/B6-8B surface code + CONVENTIONS §47–§50 + the carried notes (§4 below). The D8 pass restyles and makes self-explaining WITHOUT changing behavior (surface headers, teaching empty states ride each surface's pass; the full tour/hints layer is UI-7's).
3. Standard pipeline per packet: grounding → executor (opus, always) → evaluation (fresh, hazards named) → drain (cap 2). Coordinator dispositions OQs between grounding and executor. Pause only at spec conflicts, phase-gate-class decisions, and operator hands-on items.
4. Do not re-open B-phases, the B6 gate, the G-series, or the D3/D8 batch structure — all FINAL. The proposal is approved; never re-ask.

## 3. Counters (reference = the UI-4 landing battery, 2026-08-05 — verify at entry)

| Thing | Value |
|---|---|
| Web tests | **592 / 27 files** (vitest, offline) |
| Go test packages | **45** green, **14 sanctioned env-gated skips** |
| Bundle | **JS 1113.19 kB / gzip 335.21 · CSS 54.99 / 11.75 · index.html 1.80 · 15 font assets / 417.30 kB** — measured at every landing, growth itemized, no silent growth; SPA un-split |
| `components.lock` | **38 entries**; lockgate also covers **574 npm packages** + 15 go.mod deps + 3 pinned workflow actions |
| Sweep | **gap routes 0 / shapes 0**; `declaredButUncalled() === ['health']` |
| ⚙ settings | **118 keys / 33 domains** — `internal/settings/index.go` untouched through the whole batch |
| Migrations | **0001–0021** (`user_version` 21) — none added in the batch |
| Layer-1 catalog | **50 / 50 — AT its band ceiling** (any new canned entry = the band decision, stop-and-flag) |
| Golden fixture `cursor` | **89** — regeneration must produce zero drift |
| Escape scan | allowlist **EMPTY**, banned tokens 8, floor 50 |
| Event inventory | **102 minted / 5 declare-only = 107** (unchanged) |
| CONVENTIONS | **§1–§50** (§47 UI-1 · §48 UI-2 · §49 UI-3 · §50 UI-4) |
| Spec | v1 + amendments **A1–A10** applied; assembled sha `9ceb914d…`; BENCH-REG byte-untouched |

## 4. What the batch has established (binding vocabulary + carried notes)

- **Design language:** the one token set themes everything; kit primitives only; no raw colors outside the token block (scanned); motion authored `motion-safe:` + kill-switch backstop (scanned); density root classes; JetBrains Mono `tabular-nums` on figures.
- **Controls idioms (§48/§49):** `useAct`/`refusalOf` with `conflictNote` for multi-subject verbs; confirm-before-fire with per-state TRUE consequences (never claim atomicity/permanence the backend lacks — twice-caught class); busy≠invalid; drafts preserved after refusal; error strings transcribed byte-exact from Go handlers with cites; `[data-outcome]` four arms (`applied|noop|retry|failed`).
- **Two sanctioned narrow reads landed** (pause `automation` block; verdicts `opt_in` block) — the precedent: a control that cannot display honest current state gets its minimal read wired from an EXISTING store reader, never a blind toggle.
- **Fixture discipline:** golden world regenerates drift-free through REAL producers (cursor 89); hand-scripted bodies only for producer-unreachable arms, byte-exact with file:line cites; `doubles.ts` helpers are posture-explicit.
- **Carried to UI-5:** hoist Fleet's session read (App.tsx was frozen when §48 recorded it; the C-1 wall is now open history — hoist per R17(c)'s own note). Timeline rail maps onto REAL recovery-ladder/watchdog events; threshold-donut = the one capacity idiom; never-stall-silently affordances.
- **Carried to UI-7:** ToastProvider production mount (the kit's toast is tested but unmounted); the seeded demo world (dev-only, derived from golden-fixture producers, never production); the assistant-card rework in operator words (A-2's second half).
- **Standing candidates on record (not scheduled):** Tailwind test-glob `.container` dead-ladder exclusion; woff-fallback/non-latin-subset tightening (lock row note); `ui/Timestamp.tsx` historical comment gets a dated cross-note whenever the file next legitimately opens.
- **UI-4 rider ANSWERED:** frame-driven re-render suffices on every live surface; `live` dropped nowhere; two freeze-direction notes recorded in §50.

## 5. How the machinery works (do not reinvent it)

Four-stage pipeline per packet, coordinator-launched fresh background agents, strictly sequential: **grounding** → `P3/briefs/P3-UI-<n>.md` (self-contained, doubles as the rubric; OQs to the coordinator, never silent resolution) → **executor (always opus)** → **evaluation** (fresh, judge ≠ executor, hazards named in advance — the highest-yield class remains "passes in jsdom / passes against a fixture, false in production") → **drain** (triage [D1..Dn], fix, re-check; hard cap two rounds; trivial record-only residuals go coordinator-inline at landing with a dated correction — the established proportionate precedent). Landing checklist every packet: re-check PASS (or drained), coordinator's own full battery green, spot-diff, STATE row + log, commit, push.

Batch lessons that earned their keep this session: **composition tests catch what isolation hides** (the vanishing purge answer — UI-3 D2); executors keep self-catching predicted-vs-measured record slips — keep demanding measured numbers; the walls work (Spec/ three-file proof, App.tsx diff-census, zero-backend with named sanctions); a flagged-not-fixed finding under a wall is correct behavior (the `rowid` comment — sanctioned then fixed comment-only, hash-proven).

## 6. Invariants that must never be broken

Everything from the campaign stands: **the spec wins**; D1–D10 fixed; adopt-don't-fork with exact pins + lockgate (Go and npm); every ⚙ through the registry; no load-bearing metered paths; money read never computed never fabricated; honest absence fails loud; live-verify real-world facts at time of use; **no auto-kill; no silent caps**; content-vs-telemetry line; escape-by-default (allowlist EMPTY; the preview iframe is the one raw-HTML channel); capability URLs are secrets; secrets never committed; host changes proposed-then-approved, units generated-not-installed. Batch rules: bundled assets only (no CDN, no runtime fetches); **AGPL ideas-only, forever** (Odysseus — an evaluation checklist item on every packet); the approved proposal is the design authority, subordinate to the spec on all behavior; behavior-preserving except named sanctioned exceptions, each recorded; assertion counts never drop silently; bundle measured at every landing.

## 7. Host and environment state you inherit

- **Production untouched all batch** — `tailscale serve` → Caddy 127.0.0.1:8481 → the production unit on 8482; `sinet-control` + `sinet-broker` active; `/usr/local/bin/sinet` still the 20 July B2-gate binary (D6 upgrades it AFTER the UI batch; operator grants `sudo -v` at execution time; prod-DB backup first).
- **Throwaway click-through state at `~/.sinet-b6-clickthrough`** (binary + DB + logs; the real `operator` account's PIN is known to the operator, deliberately NOT in-repo). Dev posture = `./P3/gates/B6-clickthrough.sh`; with C-1 fixed, dev posture now reaches the login picker natively (may satisfy the gate's A-2 tooling wish at the exit walk). `--clean` removes everything.
- **GitHub identity:** active `gh` account must be `darian-gajgic`; `sinet-ai` is legacy (its pushes fail with a misleading `Repository not found`).
- Local inference tier live-capable user-level (llama-swap v241, llama.cpp b10085 sm_120, ~38 GB in `~/.sinet-b45`) — NOT running; wired at bring-up. No driver/kernel/system-CUDA change, ever.
- B5 organs: promptfoo 0.121.19 + changedetection.io 0.55.8 installed user-level, not running; `sinet-watchlist.service` generated-only; all four canaries disarmed. AppArmor userns finding still blocks service-context confined runs until the hardening session.
- Standing operator items (B6 report §8, none blocking): week-one push drill at first deploy; hardening session; fleet/GPU fill honest-absent; suspend-probe leg; `/tmp/llamaswap-test/` + demo dirs await operator `rm`; optional GitHub Verified badge.

## 8. Model routing

Coordinator sessions at **max effort**. Executors/finalizers **always opus**. Grounding/evaluation inherit (Fable) — UI scope is not S10/S11-dense; lossless opus fallback on any safeguard trip (none tripped this whole batch). Judge ≥ executor; independence = fresh context + hazards named in advance. Round 2 of a drain goes to a FRESH finalizer when the executor's blind spot is what's being fixed.
