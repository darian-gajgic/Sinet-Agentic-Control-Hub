# P3 handoff — corrected 2026-08-05 (later): **THE UI BATCH WAS REJECTED at the operator's first real walk — complete frontend rework ordered. The next act is the REWORK PRODUCT MAP under the NEW frontend workflow `.claude/skills/p3-implementation/FRONTEND.md` (read it before any frontend work; the four-stage pipeline is now BACKEND-ONLY). Operator findings: `P3/design/rework-operator-findings.md`; research provenance: `P3/design/frontend-workflow-research-2026-08-05.md`. Ratified: recreate the Nexus look; two operator checkpoints (map approval, then screenshots at shell + first surfaces). The scripted click-through, D6 ceremony and S19.6 bring-up are DEFERRED until the rework lands. §1–§8 below describe the batch as built and remain accurate as history; the STATE log entry of this date outranks anything here.**

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* to orient a fresh session in two minutes. It goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If they disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1 + applied amendments A1–A10; drafts in `Spec/drafts/` are canonical text)** → `Spec/benchmark-preregistration-v1.md` (BENCH-REG, signed, untouched) and `Spec/frontend-components-v1.md` (FC-v1) → `P3/CONVENTIONS.md` (§1–§53) → **`P3/design/design-approach-proposal.md` (APPROVED 2026-08-04; its §7 carries the three dated A1 narrowings authored 2026-08-05)** → `P3/STATE.md` → this file. `Research/` is a **closed archive**; `Docs/` is **read-only**.

---

## 1. Where the build is

**B0–B6 ALL CLOSED. The post-gate UI batch is COMPLETE — all seven packets landed + validated 2026-08-04/05:**

- **UI-1** foundation (tokens/kit/fonts/shell) · **UI-2/3/4** all fourteen D3 controls (two sanctioned narrow reads; A9/A10 amendments; C-1 login fix; D5 timestamps) · **UI-5** oversight D8 pass (SurfaceHead/StallBanner/EmptyState idioms, the stage-grain timeline rail, ThresholdRing, the Fleet session hoist) · **UI-6** decision/system D8 pass (approval-row anatomy, the sanctioned `task_id` door, the A-2 settings explanation, login/push) · **UI-7 in six seams**: the cascade-layer fix (the owned sheet in `@layer base` — the tree-wide un-suppression of the kit look), the chat restyle, the A-2 assistant-card rework in operator words, the verb-walled tour + once-per-tab hints + teaching empties, background-run etiquette (dots/toasts on the ratified sets) + ToastProvider's mount, **the dev-only seeded demo world** (env-gated test-package entrypoint; compile-boundary wall both ways), and the review-surface restyle **visually verified per-region in real Chrome over the seeded world**, closed by the C-3 exit battery + §53.
- **The ledgers at close:** sanctioned narrow backend changes THREE (§48 pause `automation` block · §49 `opt_in` block · §52 `task_id` door); proposal amendments A1.1–A1.3 (degraded-sentence shape · hints once-per-tab · completion-toasts = chat lifecycle at v0); `api.ts` byte-identical across all six UI-7 seams.
- Nothing is mid-pipeline. No background agents live. Tree clean apart from the four long-standing operator files (`Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/` — never staged). **Everything pushed** (origin `darian-gajgic/Sinet-Agentic-Control-Hub`).

## 2. The next acts, in order (the queue's exit row)

1. **OPERATOR CLICK-THROUGH (hands-on — the pause point).** One command: `./P3/gates/B6-clickthrough.sh` (add `--clean` afterward to wipe). It refuses production posture, seeds the demo world (prints op/alice/bob + the demo PIN), and serves the restyled app on its own throwaway port. Walk it per the script's own printed walk doc; sign in as alice for the owner-scoped surfaces (chat, memory, the review surface at `/deliverables/d-site` with the real diff + anchored comments). The walk doc names the two honest differences (meters/prices show the real stores; tour/hints are the app, not the seed). Free-text feedback is authoritative; taste fixes return as dated amendments.
2. **The D6 upgrade ceremony** (coordinator-executed AFTER the operator's walk): `sudo -v` window from the operator, **prod-DB backup first**, then `/usr/local/bin/sinet` upgraded from the 20-July B2-gate binary to the batch-close build; production units restarted; the week-one push drill follows at first deploy.
3. **Bring-up per S19.6**: the D2 paid sweep (~$2.10 projected, $5.00 ratified stop), local-tier wiring (llama-swap v241 + llama.cpp b10085 in `~/.sinet-b45`, NOT running), calibration writes (the B4-7 values at the deployed seat keys), the remaining standing operator items (B6 report §8: hardening session, fleet/GPU fill honest-absent, suspend-probe leg, demo-dir `rm`s, optional Verified badge).

## 3. Counters (the batch-close battery, 2026-08-05 — verify at entry)

| Thing | Value |
|---|---|
| Web tests | **709 / 31 files** (vitest, offline) |
| Go test packages | **45** green, **15 sanctioned env-gated skips** (the 15th = the seed's gate) |
| Bundle | **JS 1174.07 kB / gzip 354.02 · CSS 57.61 / 11.99 · index.html 1.80 · 15 fonts / 417.30** — hashes `index-8ixY4gSm.js` / `index-kiCmlWw5.css`; the §53 close chains every seam's delta to this figure exactly |
| `components.lock` | **38 entries**; lockgate covers 574 npm + 15 go.mod + 3 actions |
| ⚙ settings | **118 keys / 33 domains** — untouched by the whole batch |
| Migrations | **0001–0021** (none added post-B6) |
| Golden fixtures | 52 files, `cursor` **89**, zero-drift regeneration |
| Layer-1 catalog | **50/50 — AT its band ceiling** |
| Escape scan | allowlist **EMPTY** |
| CONVENTIONS | **§1–§53** (§53 = the whole UI-7 packet incl. its close) |
| Spec | v1 + A1–A10; BENCH-REG byte-untouched; proposal §7 A1.1–A1.3 |

## 4. Standing rules and candidates the batch minted (binding or carried)

- **TAILWIND SCANS PROSE (standing rule):** a bare utility-shaped token in ANY scanned file — test comments included — ships a rule into the production sheet. Plant-proven both directions (`P3/CONVENTIONS.md` is NOT scanned; the web tree is). Rebuild and diff the built `@layer utilities` before believing a doc-only change moved nothing.
- **The cascade-collision class:** an owned rule in `@layer base` (media-gated or not) LOSES to any competing utility on the same element. Four jsdom-blind defects this batch came from exactly this family — verification of anything cascade-adjacent happens in the BUILT artifact (real Chrome; the clickthrough script is the harness).
- **Cites into files under active edit go by TEST TITLE / enclosing structure, never line numbers** (the :845→:852→:853 drift bit three rounds).
- **Standing candidates (recorded, not scheduled):** the general cascade-collision scanner; Tailwind test-glob `.container` dead-ladder exclusion; woff-fallback/non-latin-subset tightening; `ui/Timestamp.tsx`'s historical-comment cross-note (file still byte-unchanged); run-grain completion toasts (needs a served state fact on the frame); a served cause code for the disambiguation card; a seed lineage edge as an exit-walk nicety; the settings-tab clamped-S18 amendment (the operator's founding requirement, deferred).

## 5. How the machinery works (do not reinvent it)

Four-stage pipeline per packet: **grounding** → `P3/briefs/` (self-contained, doubles as the rubric; OQs to the coordinator) → **executor (always opus)** → **evaluation** (fresh, judge ≠ executor, hazards named in advance) → **drain** (triage [D1..Dn], cap two rounds; the dead-executor rule sends a lost-context drain to a fresh opus finalizer; trivial record-only residuals go coordinator-inline at landing, dated). Landing checklist every packet: re-check PASS (or drained), the coordinator's own battery green, spot-diff, STATE, commit, push. **The batch's proof of the design: every defect class was caught by a fresh context, never by its author — including two coordinator errors caught by executors and one executor claim disproven by an evaluator's plant.**

## 6. Invariants that must never be broken

Everything from the campaign stands: **the spec wins**; D1–D10 fixed; adopt-don't-fork with exact pins + lockgate; every ⚙ through the registry; no load-bearing metered paths; money read never computed; honest absence fails loud; live-verify real-world facts; **no auto-kill; no silent caps**; content-vs-telemetry; escape-by-default (allowlist EMPTY; the preview iframe is the one raw-HTML channel); capability URLs are secrets; secrets never committed; host changes proposed-then-approved. Batch rules now standing: bundled assets only; **AGPL ideas-only, forever**; the approved proposal (+ its §7 amendments) is the design authority, subordinate to the spec; the seeded world is dev-only by compile boundary and never in the shipped binary.

## 7. Host and environment state you inherit

- **Production untouched all batch** — `tailscale serve` → Caddy 127.0.0.1:8481 → the production unit on 8482; `sinet-control` + `sinet-broker` active; `/usr/local/bin/sinet` still the 20-July B2-gate binary (**the D6 ceremony upgrades it AFTER the operator walk**; prod-DB backup first; `sudo -v` at execution time).
- The click-through harness: `./P3/gates/B6-clickthrough.sh` (one command; `--clean` wipes; seeds the demo world; prints users + PIN).
- Local inference tier live-capable user-level, NOT running (wired at bring-up). B5 organs installed user-level, not running; canaries disarmed; AppArmor userns finding still parks confined service-context runs (hardening session).
- **GitHub identity:** active `gh` account must be `darian-gajgic`.

## 8. Model routing

Coordinator sessions at **max effort**. Executors/finalizers **always opus**. Grounding/evaluation inherit (Fable); lossless opus fallback on any classifier trip (**zero trips across the entire batch**). Judge ≥ executor; independence = fresh context + hazards named in advance.
