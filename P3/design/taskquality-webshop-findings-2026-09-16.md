# Task-quality findings — the interview-test1 webshop (t-3120e8e3d14591d3), 2026-09-16

The first real end-to-end quality probe of the platform's own task pipeline, run by the operator during the interview-rework sitting. Operator verdict on the product: FAIL ("worse than the predecessor's output on a similar goal"). Coordinator forensics below; every claim traces to a tool result from the 2026-09-16 session (world `~/.sinet-rework-sitting`, DB read-only copies, headless-Chrome CDP repro, artifacts dir, deliverable snapshot `083b174`).

## What happened (verified timeline)

1. Intake/interview completed normally (8 plain questions; SPEC-v2 approved with gwt ACs; PLAN-v2 approved with per-step HOW). Stakes standard.
2. `execute` (claude-cli / anthropic / claude-sonnet-5) ran S-1..S-4. **S-4's engine session SUCCEEDED** (engine.done end_turn, 24 turns), then the platform's own post-stage artifact snapshot failed: `S02.4d: git -c (exit 128): fatal: unable to stat 'src/components/PartGrid.jsx.tmp.107557.…'` — a vanishing engine temp file raced `git add`. Stage marked error → run `crashed` (event 1557).
3. Recovery forked `execute.g1` (same worktree), which **re-drove the plan from S-1** over the dead run's partial tree; all 8 stages completed. Re-spend: the crashed run's 4 sessions = **$1.19 API-equivalent discarded** of $3.51 total execute spend (~34%), ~10 wall minutes.
4. `verify`: judge-compliance and judge-sanity, both **claude-opus-4-8, num_turns=1, ZERO tool events** — no build, no tests, no browser, nothing executed. Both passed. Deliverable in-review: dtype markdown, one object (`deliverable.md`, the S-8 step report), snapshot pin = the 23-file app.
5. Operator ran the app (via `try-deliverable.sh`): random non-car-part images, thin bare-grid feel, perceived dead interactions → FAIL verdict.

## Root causes, ranked

**TQ-F1 — artifact-snapshot race (platform bug, HIGH).** S02.4d snapshot runs `git add` while the engine's atomic-write `.tmp.<pid>.<hash>` files can still vanish → exit 128 → a HEALTHY stage marked error → whole-run crash. The engine did nothing wrong. Fix direction: snapshot excludes/retries transient `*.tmp.*` paths (or quiesces before add). Candidate micro-packet; cheap, high value (this class kills any long task probabilistically).

**TQ-F2 — whole-plan re-drive on a dirty worktree (recovery design, HIGH).** g1 restarted at S-1 despite a written checkpoint at S-4/msg21, on the same worktree holding S-1..S-4 output. Cost: the $1.19/10-min re-spend, plus coherence risk — each redo session meets unexplained existing files and satisfies its Done-when minimally (likely contributor to the image downgrade, TQ-F4). Fix direction: stage-granular resume from the last good stage snapshot, or reset the worktree to that snapshot before re-drive.

**TQ-F3 — verification executes nothing (THE HEADLINE, spec-level).** Two one-shot Opus reads (~$0.16 each) are the entire verify story. The project had no captured check pack (fresh scaffold; A14 bootstrap posture behaved as designed for COMMANDS), but the class gap is: **launch-domain work needs an execution rung — build + tests + a browser walk of the ACs**. The plan even wrote machine-walkable gwt lines per AC; nobody walked them. Note: the preview machinery already composes a sandbox for exactly this app class (the deferred dev-server substrate, `internal/preview/manager.go` deferredReason) — verifier and preview want the SAME plumbing; one substrate, two consumers (pairs with sitting finding SIT-F3). Spec amendment territory (S07/S09 verify duty + the deferred host substrate's priority).

**TQ-F4 — plan-fidelity unchecked (HIGH).** PLAN S-2 contracted: "representative" free stock photos **downloaded** into `public/images/**`, "Research current model series and realistic euro price ranges so the data is believable", decision row recorded. Shipped: hotlinked `picsum.photos/seed/...` random placeholders, no `public/images/` at all. AC-1 as written ("shows a picture") was satisfiable by anything, and the compliance judge never diffed delivery against the plan's Done-when/Approach/Writes rows. The deliverable.md's own phrase "Verified by construction" went unchallenged. Fix direction: compliance judge receives the plan's Done-when + Writes as an explicit checklist verified against the TREE (not the executor's report — self-report bias; the fresh-verifier lesson from our own build pipeline applies to the platform's runtime too).

**TQ-F5 — nobody owns "would a human call this good".** look_feel was answered "modern with basic animations"; `quality_bar` (the slot the W1 harvest recorded as verdict-changing) was NOT asked — at unchanged floors, standard tier stops before it (the GF7 executor's gate note). This live FAIL is direct evidence for gate-batch item 2: ask quality_bar at standard. The same browser-walk rung as TQ-F3 is where a "walk it as a user" persona check would live.

**TQ-F6 — model/duty observation (INFO).** Executor claude-sonnet-5: mechanically competent output (clean React, real tests, error handling, a11y). The gap was contract enforcement + verification, not raw codegen — per-duty tier advice stands; fix verification before considering executor upgrades. Judges were correctly tiered ABOVE the executor (opus-4-8); the method (one read, no tools), not the tier, is the defect.

**TQ-F7 — operator-perceived dead interactions: NOT reproduced in clean Chrome (OPEN).** CDP real-mouse repro: card click navigates `/` → `/parts/p01` AND renders the detail; add-to-cart increments the corner badge; zero JS exceptions. The operator's dead clicks are environment/state-specific (browser identity/extensions/stale tab — unresolved; needs their console or a `/chrome`-paired look). Independent of that, real UX weaknesses stand: add-to-cart feedback is a subtle corner badge (no drawer opens), card-click vs button affordance unclear, detail renders with a scroll jump. These fold into the deliverable-surface rework alongside SIT-F1..F4.

**TQ-F8 — run-surface honesty (LOW).** The operator watched S-1 start again mid-task with no narrative: the stage list shows two unconnected sequences, "execute stands at crashed" beside "execute.g1 completed" with no connective sentence, and the stage-end copy on an S-4 ERROR is a guess-list ("parked on an ask, a ceiling reached, or the engine stopping"). The surface should say plainly: the platform's snapshot step failed (not the work); a successor re-ran the plan. RW-19-style plain-words work.

## The comparison the operator supplied (:8790, predecessor output, GLM-5.8, similar-class goal)

Headless dump: Next.js, 18 products (vs our 32), **real per-product LOCAL images** (`/products/nvidia-geforce-rtx-4090.jpg` — exactly what our plan promised and our executor skipped), hero/landing copy, brand+category+price filters, per-product routes, /cart page, stock badges. Our output has MORE engineering hygiene (tests, error handling, a11y, honest empty states) and MORE catalog rows; theirs wins the axis a human judges first: images and landing polish — precisely the dimension our chain neither contracted tightly nor verified. A single long engine session also keeps whole-product coherence that our staged pipeline currently trades away without the verification story that justifies the trade.

## Disposition

- TQ-F1, TQ-F2: candidate backend packets (four-stage), post-sitting triage.
- TQ-F3 (+SIT-F3): the big one — spec amendment path (verify execution rung + the deferred preview/serve substrate promoted); operator call at the gate.
- TQ-F4: judge-brief change (packet-sized), pairs with TQ-F3.
- TQ-F5: strengthens gate item 2 (quality_bar at standard) — decided in the open gate batch.
- TQ-F7: OPEN pending operator browser evidence; UX halves fold into the deliverable-surface rework (SIT-F1/F2/F4).
- TQ-F8: copy/narrative packet, small.
- Evidence kept: `~/.sinet-code-tryout/dlv-t-3120e8e3d14591d3` (checkout + logs), scratchpad CDP scripts (transient), this file. The world stays the sitting's live world — do not reseed.
