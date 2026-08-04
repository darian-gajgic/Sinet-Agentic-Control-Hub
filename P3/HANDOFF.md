# P3 handoff — last rewritten 2026-08-04 (end of session): **THE BUILD IS FINISHED, THE B6 GATE IS CLOSED, and the directed UI batch waits on ONE thing: the operator's approval of the design-approach proposal. That approval is this session's first item.**

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* to orient a fresh session in two minutes. It goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If they disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1, tag `spec-v1`; drafts in `Spec/drafts/` are canonical text)** → `Spec/benchmark-preregistration-v1.md` (BENCH-REG, signed) and `Spec/frontend-components-v1.md` (FC-v1) → `P3/CONVENTIONS.md` → **`P3/design/design-approach-proposal.md` (design authority for the UI batch once approved — subordinate to the spec on all behavior)** → `P3/STATE.md` → this file. `Research/` is a **closed archive**; `Docs/` is **read-only**.

---

## 1. Where the build is

**B0–B6 ALL CLOSED.** 13/13 B6 packets, 11/11 routes, $0 across the phase. **The B6 gate was answered in full and closed 2026-08-04** — record FINAL in `P3/gates/B6-report.md` §9 (eight decisions + the click-through findings A-1/A-2/C-1).

**The directed post-gate batch (the operator's own answers define it):**
design-approach approval → **UI-1 foundation** → the **fourteen D3 controls** (cancel → pause → budget → benchmark opt-in → memory family → history ask/search) + C-1 login fix + D5 timestamps + two cosmetic S00.9 amendments → the **D8 pass** over the eleven existing routes (incl. the self-teaching layer) → exit battery + **seeded-demo click-through** → **D6 production upgrade** (coordinator-executed, `sudo -v`, prod-DB backup first) → **bring-up (S19.6)** with the D2 paid sweep (~$2.10 / $5.00 stop, authorized).

**The design work is DONE and PRESENTED, not yet approved:**
- `P3/design/design-approach-proposal.md` — **the artifact the operator must approve** (their D3 condition: "declare how to build them before we start"). §6 holds the four ask-items; the answer slot is `_pending_`.
- Its three inputs, all filed + committed: `nexus-frontend-harvest.md` (coordinator spot-checked byte-exact) · `ui-foundation-sourcing.md` (live-verified 2026-08-04) · `odysseus-pattern-study.md` (**AGPL — ideas only, forever**).
- Proposal essence: **the look** = Nexus violet-glass dark language poured into shadcn token names; **the substrate** = Tailwind v4 + shadcn copy-in (Base UI track) + bundled Fontsource Inter/JetBrains Mono + lucide, pins re-verified at adoption through the rail; **the patterns** = Nexus control-room anatomy + Odysseus behavior vocabulary (reimplemented) + the already-shipped carried specs; **self-teaching layer first-class** (tour, one-shot hints, teaching empty states, assistant-card rework in operator words); three-layer functionality carry (§3) incl. the note that side-by-side review is ALREADY shipped and the 3D/voice showpieces stay parked v1+ behind the registered BENCH-REG gate.

Nothing is mid-pipeline. No background agents live. Tree clean apart from the long-standing operator files (`Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/` — never staged). **Everything pushed** (last commits 2026-08-04; origin = `darian-gajgic/Sinet-Agentic-Control-Hub` after the module-path rename roll-forward — see STATE).

## 2. This session's first acts, in order

1. **If the operator has answered the proposal** (free text is authoritative — do not re-ask via a form): record the answer VERBATIM in `P3/design/design-approach-proposal.md` §6 and in STATE. If approved (with or without taste overrides — fold overrides into the proposal as dated amendments), **cut and ground packet UI-1 (foundation)** per proposal §4. Grounding read-first set: the proposal + its three inputs + FC-v1 + S15 + `web/src/index.css` + the B6 gate findings.
2. **If they have not answered:** re-present proposal §6 in plain language (four items, recommendations were: yes/yes/yes + taste free-text) and wait. **Never start a UI packet without the recorded approval — that is the operator's own D3 condition.**
3. Do not re-open closed phases or re-litigate the B6 gate, the G-series, or ratified dispositions. The B6 §9 answers are FINAL.
4. Session-entry battery before building on measured ground:
   `gofmt -l internal/ cmd/ tools/` · `go vet ./...` · `go build ./...` · `go test ./... -count=1` · `go run ./tools/lockgate` · `cd web && npm run typecheck && npm run test && npm run build`

## 3. Counters (reference = 2026-07-30 gate measurement; full battery re-run GREEN 2026-08-04 after the module rename)

| Thing | Value |
|---|---|
| Go test packages | **45** green, 0 failures, **14 sanctioned env-gated skips** (the correct number is 14, not 0) |
| Web tests | **432 / 23 files** (vitest, offline) |
| Bundle | **976.99 kB / gzip 292.96** (will grow in UI-1 — fonts/primitives; measure at every landing, no silent growth) |
| `components.lock` | **29** entries; lockgate also covers **240 npm packages** |
| Event inventory | **102 minted / 5 declare-only = 107** |
| ⚙ settings | **118 keys / 33 domains** — `internal/settings/index.go` untouched through B5+B6 |
| Migrations | **0001–0021** (`user_version` 21) |
| Layer-0 views / catalog | **11 / 50 — catalog AT its band ceiling**; the next catalog query forces the band decision (possibly during the history-controls packet) |
| `built` routes | **11 — the whole table** |
| Escape scan | allowlist **EMPTY**, banned tokens 8, floor 50 |
| Shared fixture `cursor` | **86** — regeneration must produce zero drift |
| CONVENTIONS | §1–§46 |
| Module path | `github.com/darian-gajgic/Sinet-Agentic-Control-Hub` (renamed from `dariannixda-eng` 2026-08-04, rolled forward, battery-proven) |

## 4. What the B6 gate decided (2026-08-04 — full record `P3/gates/B6-report.md` §9)

**D1/D4/D7 ratified** (D7 en bloc incl. two cosmetic S00.9 amendments → apply in the UI batch) · **D2 authorized** — paid sweep at bring-up after D6 · **D3 conditional** — all fourteen controls, but the HOW is declared+approved first (→ the proposal), sources = Nexus + GitHub tools · **D5 as recommended** (relative-beside-UTC on live surfaces, UTC-only audit; taste delegated) · **D6 authorized, coordinator-executed** via the established `sudo -v` window — ONE upgrade after the UI batch, prod-DB backup first · **D8 ratified**, declaration pulled before D3 by D3's own condition. Click-through findings on record: **A-1** (UI doesn't explain itself → self-teaching layer), **A-2** (dev-identity settings read-only + honestly-degraded assistant, live-verified root causes), **C-1** (dev-posture Sign out no-op / login unreachable → fix in the batch).

## 5. How the machinery works (do not reinvent it)

Four-stage pipeline per packet, coordinator-launched fresh background agents, strictly sequential: **grounding** → `P3/briefs/…` → **executor (always Opus)** → **evaluation** (fresh, judge ≠ executor, told to find what is wrong, hazards named in advance) → **drain** (triage [F1..Fn], fix, re-check; hard cap two rounds, then coordinator-inline recorded honestly). Lessons that earned their keep (full list in the 2026-07-30 handoff section preserved in git history): name the hazards you want attacked — highest-yield class "passes in jsdom / passes against a fixture, false in production" (it caught THIS session's own dev-login instruction error too); **a number asserted is not a number measured**; refusal-with-evidence outranks compliance; a tautological test is worse than none; fix the class, not the instance; never commit while a stage agent is live (`git commit --only`); update STATE before and after every step; push after milestones.

**UI-batch specifics:** packets build against the approved proposal; existing tests stay green with markup-coupled assertions updated honestly (assertion counts never drop silently); bundle measured at every landing; the AGPL boundary (Odysseus ideas-only) is an evaluation checklist item on every UI packet.

## 6. Invariants that must never be broken

Everything from the campaign stands: **the spec wins**; D1–D10 fixed; adopt-don't-fork with exact pins + lockgate (Go and npm); every ⚙ through the registry; no load-bearing metered paths; money read never computed never fabricated; honest absence fails loud; live-verify real-world facts at time of use; **no auto-kill; no silent caps**; content-vs-telemetry line; escape-by-default (allowlist EMPTY; the preview iframe is the one raw-HTML channel); capability URLs are secrets; content-addressed routes never serve contradicting bytes; secrets never committed; host changes proposed-then-approved, units generated-not-installed. **New, from this batch:** bundled assets only (no CDN, no runtime fetches); **AGPL ideas-only, forever** (Odysseus); the design proposal is the batch's design authority but subordinate to the spec on all behavior.

## 7. Host and environment state you inherit

- **Production untouched all session** — `tailscale serve` → Caddy 127.0.0.1:8481 → the production unit on 8482; `sinet-control` + `sinet-broker` active; `/usr/local/bin/sinet` still the 20 July B2-gate binary (D6 upgrades it AFTER the UI batch; the operator grants `sudo -v` at execution time; back up the prod DB first).
- **Throwaway click-through state at `~/.sinet-b6-clickthrough`** (binary + DB + logs): now contains a **real operator account** (`operator`, role operator — PIN chosen in-session, known to the operator, deliberately NOT recorded in-repo) plus a few coordinator probe chat sessions. It may still be running on 8483 in the operator's terminal (either posture). Dev posture = `./P3/gates/B6-clickthrough.sh`; real-auth posture = same binary with `STATE_DIRECTORY`/`CONFIGURATION_DIRECTORY` set (see STATE 2026-08-04 log). `--clean` removes everything.
- **GitHub identity:** active `gh` account must be `darian-gajgic` (account renamed from `dariannixda-eng`; module path + remote follow it). `sinet-ai` is legacy — its pushes fail with a misleading `Repository not found`.
- Local inference tier live-capable user-level (llama-swap v241, llama.cpp b10085 sm_120, ~38 GB weights in `~/.sinet-b45`) — NOT running; gets wired at bring-up (un-degrades the assistant's sentence-understanding). No driver/kernel/system-CUDA change, ever.
- B5 organs: promptfoo 0.121.19 + changedetection.io 0.55.8 installed user-level, not running; `sinet-watchlist.service` generated-only; all four canaries disarmed. AppArmor userns finding still blocks service-context confined runs until the **hardening session** (needs sudo; standing item 2).
- Standing operator items (gate report §8, none blocking): week-one push drill at first deploy; hardening session; fleet/GPU fill honest-absent; suspend-probe leg; `/tmp/llamaswap-test/` + demo dirs await operator `rm`; optional GitHub Verified badge.

## 8. Model routing

Coordinator sessions at **max effort**. Executors/finalizers **always Opus**. Grounding/evaluation inherit; lossless Opus fallback on any safeguard trip; Opus outright on crypto/capability-URL-dense packets (standing rule, memory `fable5-safeguard-false-positive`). Judge ≥ executor; independence = fresh context + hazards named in advance. Round 2 of a drain goes to a FRESH finalizer when the executor's blind spot is what's being fixed.
