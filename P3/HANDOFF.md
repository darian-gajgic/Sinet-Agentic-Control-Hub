# P3 handoff — last rewritten 2026-08-05 (the workflow-reset session): **THE UI BATCH WAS REJECTED at the operator's first real walk. The next act is the FRONTEND REWORK under the NEW workflow `.claude/skills/p3-implementation/FRONTEND.md`, starting with the PRODUCT MAP (operator checkpoint 1 — nothing gets built before the operator approves it). The backend four-stage pipeline was audited the same day and amended A–E. The D6 upgrade ceremony and S19.6 bring-up are DEFERRED until the rework lands.**

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* to orient a fresh session in two minutes. It goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If they disagree, STATE wins and this file gets corrected.

Authority order: **`Spec/core-architecture-v1.md` (frozen v1 + A1–A10; drafts in `Spec/drafts/` are canonical text)** → `Spec/benchmark-preregistration-v1.md` (BENCH-REG, signed, untouched) and `Spec/frontend-components-v1.md` (FC-v1, binding picks) → `P3/CONVENTIONS.md` (§1–§53) → **for frontend work: `FRONTEND.md` in the skill directory** (the process authority; read it before touching anything in `web/`) → `P3/STATE.md` → this file. The 2026-08-04 design proposal's *behavior* content (incl. its §7 amendments A1.1–A1.3) stands; its *visual* direction is superseded by the ratified Nexus-look decision. `Research/` is a **closed archive**; `Docs/` is **read-only**.

---

## 1. Where the build is

- **B0–B6 all closed.** The post-gate UI batch (UI-1…UI-7) was built and mechanically validated 2026-08-04/05 — then **REJECTED WHOLE by the operator at first real use 2026-08-05**: incoherent, spec-shaped structure; the Board unrecognizable as a Kanban board; assistant intake erroring on a plain task ask; the S13.2 image trio overlaying text on text; unlabeled dead buttons. Findings, verbatim-ish with triage: **`P3/design/rework-operator-findings.md`** (the rework's seed).
- Root cause (operator-confirmed): the backend four-stage pipeline applied to design work — fresh-context-per-packet authorship, spec-section slicing, rubric acceptance, the Nexus reference reduced to prose, no human eyes until landing. Research provenance: `P3/design/frontend-workflow-research-2026-08-05.md`.
- **The backend is healthy and untouched** — and its pipeline was itself audited against state of the art the same day (`P3/design/backend-workflow-audit-2026-08-05.md`): backbone confirmed, five flaws fixed as **amendments A–E now live in SKILL.md** (tests-first executor-immutable · mutation/property leg at phase gates · codified evaluator probe/tamper duties · briefs single-use + pre-checked · trivial-packet light path).
- Nothing is mid-pipeline. No background agents live. Tree clean apart from the long-standing operator files (`Presentation/`, `Research/Presentation/`, `Sinet-Logo.jpeg`, `tools/dbpeek/`, the operator's `P3/gates/B6-clickthrough.sh` edit + `B6-walkthrough.html` — never staged). Everything pushed (origin `darian-gajgic/Sinet-Agentic-Control-Hub`).

## 2. The next acts, in order

1. **THE PRODUCT MAP (FRONTEND.md rule 3 — operator checkpoint 1).** One page from the operator's chair: the jobs to be done, the navigation with plain names, per surface "what you see / what you can do", and a carry/adapt/drop verdict for every Nexus pattern reconciled against the CURRENT backend contract (`web/src/api.ts` + the spec's behavior sections). Seed it from `rework-operator-findings.md`. Named conflict to surface here: the operator expects a Nexus-style questionnaire intake; the spec's D3 intake is chat-shaped — present the reconciliation (adapt vs amendment), don't bury it. **The operator approves the map before any code.**
2. **The single-author rebuild (FRONTEND.md rules 1–4).** ONE long-lived `model: fable` builder agent owns the whole view layer; the real Nexus source in context (`/home/sinep/Nexus-Agentic-Coding-Setup/app/static/style.css`, `index.html`, the relevant `app.js` view functions — the harvest doc `P3/design/nexus-frontend-harvest.md` may serve as a *locator* into the 11k-line `app.js`, never as the design source); screenshot-in-the-loop in real Chrome on the seeded demo world, full pages, two widths. Keep the backend, `web/src/api.ts`, the router and data hooks where sound; the views are rewritten as one design. Behavior contracts stay binding (S13/S15 review loop, FC-v1 picks, honesty invariants, escape-by-default). **Checkpoint 2: rendered screenshots to the operator after the shell + first surfaces.** Ratified: recreate the Nexus look — the violet glass control-room language on the current React/Tailwind stack.
3. **Exit per FRONTEND.md rule 5:** live design review (fresh agent, browser, four criteria) → operator-persona cold walks (real tasks; a failed walk blocks) → the machine battery as filter → **the operator click-through on the seeded world** (`./P3/gates/B6-clickthrough.sh`, `--clean` wipes; note the operator has local edits to this script — read before assuming its printed walk doc matches).
4. **Then the deferred queue:** the D6 upgrade ceremony (`sudo -v` window, prod-DB backup first, `/usr/local/bin/sinet` upgraded from the 20-July B2-gate binary, units restarted, week-one push drill) → bring-up per S19.6 (D2 paid sweep ~$2.10 projected/$5.00 ratified stop, local-tier wiring in `~/.sinet-b45`, calibration writes, standing operator items in B6 report §8).

## 3. Counters at entry (batch-close values; presentation-side ones will move)

| Thing | Value |
|---|---|
| Go test packages | **45** green, **15 sanctioned env-gated skips** |
| Web tests | 709/31 at batch close — **presentation-coupled tests get rewritten after the design settles (FRONTEND.md rule 5); behavior-contract tests stay authoritative** |
| `components.lock` | **38 entries**; lockgate 574 npm + 15 go.mod + 3 actions |
| ⚙ settings | **118 keys / 33 domains** — untouched |
| Migrations | **0001–0021** |
| Escape scan | allowlist **EMPTY** |
| CONVENTIONS | §1–§53 |
| Spec | v1 + A1–A10; BENCH-REG byte-untouched |
| Bundle | batch-close figures describe the REJECTED view layer — they reset at the rework; the §53 chain closes the old ledger |

## 4. How the machinery works now (two pipelines — do not mix them)

- **Frontend-shaped work** (anything whose primary output is presentation/interaction): `FRONTEND.md`. One Fable author, reference-over-prose, product map first, screenshot loop, pixels judged by fresh eyes, operator gates. Its "Banned" list is load-bearing — each entry shipped a defect class in the rejected batch.
- **Backend work:** the four-stage pipeline in `SKILL.md`, now with A–E. Headlines a fresh session must not miss: acceptance tests are specified by grounding and materialized RED before implementation; the executor cannot modify them or any pre-existing test; the evaluator must run ≥3 novel held-out probes and diff test files for tampering; triage runs the claimed-broken case; briefs are single-use and stamped EXPIRED at landing; trivial no-behavior packets take the light path; phase gates add a mutation-score leg.
- STATE discipline unchanged: update before and after every step; commit; push after milestones.

## 5. Invariants that must never be broken

Everything standing: **the spec wins**; D1–D10 fixed; adopt-don't-fork with exact pins + lockgate; every ⚙ through the registry; no load-bearing metered paths; money read never computed; honest absence fails loud; live-verify real-world facts; no auto-kill; no silent caps; content-vs-telemetry; escape-by-default (allowlist EMPTY; the preview iframe is the one raw-HTML channel); capability URLs are secrets; secrets never committed; host changes proposed-then-approved; bundled assets only; AGPL ideas-only; the seeded world is dev-only by compile boundary. TAILWIND SCANS PROSE (a bare utility-shaped token in any scanned file ships a rule) and the cascade-collision class (verify cascade-adjacent work in the BUILT artifact in real Chrome) both still bite.

## 6. Host and environment state you inherit

- **Production untouched** — `tailscale serve` → Caddy 127.0.0.1:8481 → the production unit on 8482; `sinet-control` + `sinet-broker` active; `/usr/local/bin/sinet` still the 20-July B2-gate binary (upgrade only at the D6 ceremony, after the rework).
- Seeded demo world: `./P3/gates/B6-clickthrough.sh` (refuses production posture; prints op/alice/bob + the demo PIN; `--clean` wipes).
- Local inference tier installed user-level, NOT running (bring-up item). B5 organs installed, canaries disarmed; AppArmor userns finding parks confined service-context runs (hardening session, deferred).
- **GitHub identity:** active `gh` account must be `darian-gajgic`.
- The frontend builder needs real Chrome: start the session with `claude --chrome` (or `/chrome` later) so the browser tools are live for the screenshot loop, the design review and the cold walks.

## 7. Model routing

Coordinator sessions at **max effort**. Frontend per FRONTEND.md's table (builder/reviewers/walkers = `fable`; lossless `opus` relaunch only on an actual classifier trip). Backend per SKILL.md's table (executors `opus`, grounding/evaluation inherit Fable, S10/S11-dense routing rules unchanged). Judge ≥ executor everywhere; independence = fresh context + hazards named in advance.
