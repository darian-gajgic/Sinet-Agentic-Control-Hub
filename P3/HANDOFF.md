# P3 handoff — last rewritten 2026-08-05 (end of the product-map session): **THE PRODUCT MAP IS APPROVED (`P3/design/product-map.md` v2.1 — the rework's binding product contract). The next session BUILDS: launch the single Fable builder per FRONTEND.md on shell + Home, and run the one queued backend packet P3-RW-1 (intake project pin) through the four-stage pipeline. The D6 upgrade ceremony and S19.6 bring-up stay DEFERRED until the rework lands.**

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* to orient a fresh session in two minutes. It goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If they disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1 + A1–A10; drafts canonical)** → BENCH-REG (signed, untouched) and FC-v1 (binding picks) → `P3/CONVENTIONS.md` (§1–§53) → **for frontend work: `.claude/skills/p3-implementation/FRONTEND.md`** (the process authority) → **`P3/design/product-map.md` v2.1 (the APPROVED product contract — what gets built)** → `P3/STATE.md` → this file. `Research/` closed archive; `Docs/` read-only.

---

## 1. Where the build is

- **B0–B6 closed; the UI batch was REJECTED 2026-08-05 and the rework is CONTRACTED.** Checkpoint 1 (FRONTEND.md rule 3) closed same day: map v1 presented → operator feedback (the full Nexus tab set was missing) → **v2 grounded against the real Nexus nav (23 views) and the backend → APPROVED** with three operator deferrals (GPU/VRAM seam wiring · CPU/RAM amendment · move-task-between-projects) and one added requirement (project context flows into new tasks — map §5).
- **The backend is healthy and untouched.** Nothing mid-pipeline; no background agents. Tree clean apart from the long-standing operator files (`Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/`, operator's `P3/gates/B6-clickthrough.sh` edit + `B6-walkthrough.html` — never staged). Everything pushed.
- Key grounding already done (don't re-derive): the S06.5 interview IS questionnaire-shaped; `POST /api/intake/requests` served but unconsumed by the old UI; projects served everywhere (`?project=` filters, "(no project)" bucket) with NO reassign verb; `FleetSnapshot.gpu`/`local_seats` are declared honest-empty seams; `MatchForIntake` = deterministic name-token match (`internal/project/registry.go:300`); memory entries carry `kind`+scope (Wins & Lessons are real reads); `GET/POST /api/auth/users` served (Household onboarding).

## 2. The next acts, in order

1. **Launch the single long-lived Fable builder** (FRONTEND.md rules 1–4) on **map §7 step 1: shell + Home** — 15-entry grouped nav (map §2; 7 NEW routes `/projects` `/new` `/reviews` `/lessons` `/history` `/health` `/manual`; zero renames), topbar project selector, footer meters as honest placeholders, the Home glance. In context: the real Nexus source (`~/Nexus-Agentic-Coding-Setup/app/static/style.css`, `index.html`, relevant `app.js` view functions — the harvest doc `P3/design/nexus-frontend-harvest.md` is a *locator*, never the design source), the approved map, `web/src/api.ts`. Screenshot loop in real Chrome on the seeded demo world (`./P3/gates/B6-clickthrough.sh`), full pages, two widths. **Session must have Chrome tools live: start with `claude --chrome` or type `/chrome` before launching.**
2. **Run P3-RW-1 concurrently** (four-stage backend pipeline, SKILL.md with amendments A–E): optional `project` field on the intake Submit body (`internal/stage/surface.go` `submitBody` → `intake.Request` → pin the `RegistrySlice` when the requester owns/belongs to the ACTIVE entry; text match stays for unpinned). Additive-first per S15.2, `Inputs`-field precedent (B6-7 OQ8), **no S00.9 amendment**. Go-only paths — no overlap with the builder's `web/` tree. The builder consumes it at map §7 step 2.
3. **Checkpoint 2 (operator):** rendered screenshots after shell + Home land. Then autonomous through map §7 steps 2–7.
4. **Exit per FRONTEND.md rule 5:** live design review (fresh agent, browser, four criteria) → operator-persona cold walks (a failed walk blocks) → machine battery as filter → **operator click-through on the seeded world** (note: operator has local edits to the clickthrough script — read before trusting its printed walk doc).
5. **Then the deferred queue:** D6 upgrade ceremony (`sudo -v`, prod-DB backup first, binary still the 20-July B2-gate build) → bring-up per S19.6 (D2 paid sweep ~$2.10/$5.00 stop, local-tier wiring in `~/.sinet-b45`, calibration writes, B6 report §8 standing items).

## 3. Operator-deferred (do NOT do these unless the operator asks)

- GPU/VRAM + local-seat fleet-seam wiring (declared seams exist; surfaces show honest placeholders).
- CPU/RAM host monitoring (would need an S00.9 amendment; not to be re-proposed unprompted).
- Move-task-between-projects verb (v0: project set at intake, follow-ups inherit).
- Also standing: the settings-tab constants-as-clamped-⚙ amendment (`settings-tab-see-change-everything`).

## 4. Machinery (two pipelines — do not mix)

- **Frontend:** FRONTEND.md — ONE Fable author (continued via SendMessage, never re-derived), reference-over-prose, screenshot-in-the-loop, live review + cold walks, operator eyes the only final gate. The "Banned" list is load-bearing.
- **Backend (P3-RW-1):** SKILL.md four-stage pipeline with A–E (grounding writes acceptance tests committed RED, executor `opus` cannot modify them, evaluator runs ≥3 held-out probes + tamper-diff, briefs single-use/EXPIRED, light path for no-behavior packets — a submit-field packet is NOT light-path: it changes behavior).
- STATE discipline unchanged: update before/after every step; commit; push after milestones.

## 5. Invariants (unchanged, load-bearing)

The spec wins; D1–D10 fixed; adopt-don't-fork + exact pins + lockgate; every ⚙ through the registry; no load-bearing metered paths; money read never computed; honest absence fails loud; live-verify real-world facts; no auto-kill; content-vs-telemetry; escape-by-default (allowlist EMPTY; preview iframe = the one raw-HTML channel); capability URLs are secrets; secrets never committed; host changes proposed-then-approved; bundled assets only; AGPL ideas-only; seeded world dev-only by compile boundary; TAILWIND SCANS PROSE; cascade-collision class → verify in the BUILT artifact in real Chrome.

## 6. Host and environment state you inherit

- **Production untouched** — `tailscale serve` → Caddy 127.0.0.1:8481 → unit on 8482; `sinet-control`+`sinet-broker` active; `/usr/local/bin/sinet` = 20-July B2-gate binary until D6.
- Seeded demo world: `./P3/gates/B6-clickthrough.sh` (refuses production posture; `--clean` wipes).
- Local inference tier installed user-level, NOT running. B5 organs installed, canaries disarmed. AppArmor userns finding parks confined service-context runs (hardening session).
- Counters at the batch close (presentation-side will move during the rework): Go 45 pkgs/15 sanctioned skips · web 709/31 (presentation-coupled tests rewritten after the design settles; behavior-contract tests stay authoritative) · lock 38 · ⚙ 118/33 · migrations 0001–0021 · escape allowlist EMPTY · CONVENTIONS §1–§53.
- GitHub identity: active `gh` account must be `darian-gajgic`.

## 7. Model routing

Coordinator at max effort. Frontend: builder/reviewers/walkers = `fable`; lossless `opus` relaunch on an actual classifier trip (zero trips so far). Backend P3-RW-1: executor/finalizer `opus`, grounding/evaluation inherit Fable (not S10/S11-dense). Judge ≥ executor everywhere; independence = fresh context + hazards named in advance.
