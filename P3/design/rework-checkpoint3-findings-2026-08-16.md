# Operator findings — checkpoint-3-era live test (2026-08-16)

The operator's live test on the :8483 world, resumed on a current-tree build (all of RW-10/11/12 in the binary, migrations 0001–0024, local stack wired and live). Free-text findings are authoritative per the standing gate rule. These seed the frontend builder pass (FRONTEND.md); none of them re-opens a landed backend packet.

Context for the record: the operator asked whether this was "the old version". It is not — the SPA bundle and binary were built from the current tree at launch (verified at start: bundle `index-dVRqrw5y.js`, DB `user_version=24`, `local: S12 duty surface wired configured=true`). Findings F2/F3 are the *deferred* SPA scope (RW-11 OQ5/OQ6 + the composing face) meeting its first real user; F1 is new.

## Findings

| # | Finding (operator's words in quotes) | Triage |
|---|---|---|
| F1 | **SPA black-screen crash class.** "When I cancel a task, I see a black screen. I have to press F5." Also strikes sporadically elsewhere: "sometimes else, when I press on something, I see a black screen." Browser back does NOT recover — only a full reload. | **NEW DEFECT, top of the fix round.** Server log clean at the crash timestamps → frontend-only: an unhandled render/runtime exception kills the React tree, and nothing catches it (no app-level error boundary), so the dead root persists across SPA navigation. Builder pass must (a) reproduce on the seeded world starting with the cancel-a-task path, root-cause each crasher, and (b) add an app-level error boundary with an in-place recover affordance so no crash ever needs F5. The never-stall-silently rule applies: a black screen is the worst possible answer. |
| F2 | Mid-journey family question is a placeholder: described a goal, pressed plan, got "A decision.family card — its controls are below. This card kind has no form here yet — answer it from its inbox card, and the journey resumes in place." Operator verdict: reads as a bug, unacceptable. | **Known deferred scope, priority now RATIFIED by first contact.** This is the RW-11 OQ5/OQ6 deferral (family form + create-door family selector deferred to the builder pass; backend fire-path verified via Inbox). The operator's reaction binds the builder pass: the `decision.family` card renders its six plain-language choices IN the journey, in place; the Inbox detour dies. The create-door family selector lands with it. |
| F3 | The family card's Inbox rendering: "even more buggy user interface buttons all over the place, not well formatted." | **NEW presentation defect.** The new card kind fell through to generic rendering and lacks the ratified card anatomy (what's being decided, plain-language context, then the verbs — the UI-6 inbox anatomy). Fixed either by F2 making the Inbox detour unnecessary, or by the card kind getting the full anatomy wherever it renders — both, ideally: any card kind with no bespoke form must still render cleanly. |

## Already-queued items this test re-confirms (no new decision needed)

- The composing/"planning…" face (RW-10 finding, builder-pass scope) — the plan step still blocks with no surface state.
- The three cosmetic defects from the 2026-08-12 builder walk (onboard-card digest checkboxes, stray "verify" stage token on the Done card, bare "(15.6)" prose).

## Process note

The 18:24:38 `intake-triage UNMETERED-CALL DEFECT` log line during this test is the documented RW-13 gap firing exactly as designed (Stage-0 classify fails closed → the family question is asked). Not a finding; RW-13 closes it.
