# Cold-walk findings — 2026-08-17 (exit gate; walker reports verbatim-compressed, coordinator triage inline)

## Walk 1 — the blind give-work journey (bicycle-shop persona, fresh world, dev-fallback start)

**Outcome: errand 1 FAILED — WALK BLOCKS RELEASE.** Errand 2 (cost) completed. World kept at `~/.sinet-coldwalk-w1` (task `t-9df1e64daff428d1` parked at its unapprovable card — evidence).

### W1-B1 (BLOCKING) — the dev-fallback approval circle

A task created while browsing under the dev fallback is permanently unapprovable through the UI: dev answers the card but 403s at step-up ("the dev identity cannot step up (S01.9); log in as a real user"); alice cannot see the task (owner-scoped to dev); op sees it but the card is D10 owner-only ("you can read it but not decide it"); the 403's own advice routes the circle. Sub-findings: sign-in dumps to Home instead of returning to the plan; the journey page then says "could not be resumed… its card is in your Inbox" pointing at an inbox that doesn't hold it for the new identity; the login picker (alice/bob/op) gives no clue who you are or who owns what.

**Triage:** dev-posture-only trap (production has no dev fallback) — but demo worlds ARE the operator's test surface, and the builder/reviewer avoided it only by signing in first. Every backend behaved per spec (S01.9, D10 owner-scope) — this is a FRONTEND flow fix: (a) the create door (and any work-creating door) under the dev fallback requires sign-in FIRST — a sign-in step in place, before work exists, with the picker explaining the seeded people; (b) post-login return-to-origin; (c) the resumed-journey message must be identity-aware. → **builder round, blocking.**

### W1 ranked frictions (walker's order, triage inline)

| # | Finding | Triage |
|---|---|---|
| W1-2 | Login picker gives no guidance (who am I? whose task is whose?) | builder round (rides B1c) |
| W1-3 | Round-1 intake questions in programmer language ("collections/state", "deduplicated… ties", "atomicity… interleave") at a bike mechanic; page admits "standard wording… not rephrased". Round 2 phrased fine. | **backend diagnosis item PH-1**: why did round-1 phrasing fall back? 6 local $0 rows exist on the intake run (classify + a 400-token call at 05:18:49 that looks like a phrase attempt); no phrase logging exists to tell. Diagnose + fix + ADD LOGGING; candidate causes: cold-load timeout, batch-vs-per-card mismatch, silent error. |
| W1-4 | "STAKES: HIGH" at task birth, unexplained until a footnote 25 min later (personal-data cause) | builder round: the badge carries its why at first sight |
| W1-5 | Cost written in five dialects ("—", "USD 0 priced", "~0.00 USD (API-equivalent, D5)", "USD undefined", "UNPRICED"); never says WHY it's free; "COST AND TIME" has no time | builder round: one vocabulary + the subscription-lane why + the time half |
| W1-6 | Skipping one question became "10 points you skipped" (fan-out into never-seen sub-points) | builder round: honest counting (questions skipped vs points assumed, linked) |
| W1-7 | WHO-DOES-IT box is internal radio-chatter (S08.8, D5, duty seats) — post-§17 residue | builder round (sweep continues; wire-side strings already on the RW-16-era gap list) |
| W1-8 | Dev banner in Inbox ("Old version — not the rework… step 3") | builder: retire the stale fence banner |
| W1-9 | Clearance meter STILL confusing (unexplained at birth; 100/100 while markers open) — survives the #18 help text | builder round: rethink the meter (hide until it means something?) |
| W1-10 | Send-vs-Proceed ambiguity (does Proceed keep typed answers? yes — but only the NEXT screen says so) | builder: the button says it |
| W1-11 | One typed answer lost to a mid-page layout shift | builder: bug, reproduce and pin |
| W1-12 | Transient all-black repaint after PIN confirm | builder: investigate (error-boundary-adjacent?) |
| W1-13 | Home page refuses to scroll | builder: bug |
| W1-14 | SPEND TODAY tile looks pressable, isn't | builder: make it a door or make it flat |

### W1 delights (keep these — they are the product's voice)

Plan restatement ("better than a human contractor ever has"); honest ticking wait faces, every timing promise kept; assumptions out loud + contestable; the PIN ceremony's weight; round-2 questions "genuinely smart" (the phone-number → submissions-live-where question).

### RW-16 live proof (from the walk world's own log)

The engine's brace-short defect fired THREE times during this one walk (plan-draft + two revises, closers=1 each) and healed in-session structurally every time — zero crashes, zero 5-minute recovery stalls, loud Warn per completion. The walker felt only honest ~4-minute plan waits.

---

## Walk 2 — the oversight walk (spouse persona, kept world `~/.sinet-b6-final`, ~616px throughout)

**Outcome: 4 of 5 errands completed — errand 3 (find why something was cancelled) FAILED → WALK BLOCKS RELEASE.** Verdict as the persona: "I'd trust it to *do* the work; I don't yet trust it to *account* for the work."

### W2-B1 (BLOCKING, coordinator-diagnosed regression) — cancelled tasks vanished from the Board

The walker could not find any cancelled thing across Board (14 tasks), Home, Inbox, Projects, and the History query desk asked four ways. Coordinator verified: the world's DB holds `t-fe5ff6c325967c3e` `kanban_status='cancelled'` ("Welcome note for contributors") — but post-drain-D1 the view serves the STORED column for ended tasks, and the Board has no `cancelled` mapping → the task falls out of every column and off every surface. Regression from the drain-r1 D1 fix (pre-drain builds showed "CANCELLED — OPEN FOR WHY" in Backlog — the ratified rendering). **Fix: map `cancelled` → Backlog with the CANCELLED chip at the Board view (and anywhere else ended tasks list); regression test pinning a cancelled task's visibility.**

**CORRECTION 2026-08-17 (builder, read-only verification — the coordinator's mechanism above is DISPROVEN):** the wire is clean (`/api/tasks` on the kept world serves all 14 tasks incl. `t-fe5ff6c325967c3e` with `kanban_status='cancelled'` and a benign `latest_run`; reads.go has no exclusion), and BOTH the world binary's embedded bundle and current `kanban.ts`/`Board.tsx` contain the "cancelled — open for why" chip and the cancelled→Backlog mapping. The walker's observation stands (no trace on any surface at ~616px) but the vanishing mechanism is subtler — render-time, narrow-width, or scope-related — and needs real-Chrome pixels to pin. The finding stays BLOCKING; the fix direction is now: reproduce as pixels first, then fix what is actually broken.

### W2 ranked frictions (walker's order, triage inline)

| # | Finding | Triage |
|---|---|---|
| W2-2 | "Done" task with NO deliverable and NO receipt ("receipts materialize at the run's terminal transition" on a completed run) — worst trust hit | builder: seeded-task story honesty — the seed mints Done without artifacts; either the seed produces a complete Done or the view says why nothing is shown, in plain words |
| W2-3 | One task shown EXECUTING + "stands at PARKED" + blocked-on-human at once | builder: one state story per task (root-cause the three sources) |
| W2-4 | Permission cards "expires 20d ago (past)" still sit as live waiting-on-you work; no stated consequence; signed-out shows no answer verbs and never says "sign in to act" | builder: expiry means something on the card + sign-in-to-act line |
| W2-5 | History/query desk answers with header-only EMPTY tables; text search returns a non-sequitur about "a query for a secret's plaintext"; page reflows under the cursor; Layer-0 pick erases previous answer; date "to" exclusive | builder: honest empty answers ("nothing matches — and here's why/what to try"), reflow fix, inclusive-or-labeled dates; the secret-plaintext canned line = wire text (gap list) |
| W2-6 | Engineer-speak on lay surfaces: receipt notes leak spec refs, "Purpose: ceremony", raw POST endpoints printed on the deliverable page, duplicated machine-dialect judge comments, "intent label margin 8.315" | builder sweep + the standing wire-side gap list (dup judge comments already G-listed) |
| W2-7 | Narrow: per-person money clips mid-number; Board shows ~1.2 columns with NO cue more exist; wide tables hide their key column behind unsignaled sideways scroll (nearly missed the verdict) | builder: narrow affordances (scroll cues, money never clips) |
| W2-8 | "verdict #275" cited but not a link; Reviews room is an IOU while Health badge shows "17" and the room shows none | builder: linkify verdict cites; placeholder rooms carry honest badges/copy |
| W2-9 | Blocked list rows = jargon confetti ("r-claim · no cost reading · anthropic · quick"); PARKED badge does double duty for blocked-on-human AND parked | builder: plain-words rows + distinct badge |
| W2-10 | First thing in the Inbox queue = an opt-in experiment pitch above urgent cards; month-old HIGH alarm ("losing its own blind comparison") unanswered | builder: queue ordering honesty; the alarm itself = operator-visible at the session (real card, real state) |
| W2-11 | "106 runs finished" mostly platform housekeeping runs, inflating the work story | builder: task-work vs housekeeping split in the counts |
| W2-12 | Scrolling fought the walker on Home/overlay/Board (wheel+PageDown dead; script scroll fine) — second report (W1-13) | builder: reproduce and fix for real this time (two walks hit it) |

### W2 delights

Plain-words banners; stage timeline; the checkable contract; live home; itemized receipts; the accept warning; consent; board scroll-position. *(full list retained below)*

---

## Re-walk A — blind journey, photographer persona, fresh world, post-fix build (2026-08-17 18:15–19:01)

**Outcome: the JOURNEY is release-quality — no crash, sign-in-first worked in place, phrased questions "the best I've ever been asked by software", every time promise kept, self-checker caught a real CSS-only-delivery defect and forced the fix, $0 — but the walk BLOCKS on the LAST MILE: the owner cannot view, download, or accept her finished website.** World kept at `~/.sinet-rewalk-a` (alice's in-review website deliverable = the builder's live fixture).

### RA-B1 (BLOCKING) — the deliver-the-result moment fails on every live surface

DONE ends at a markdown deliverable wearing a raw code diff behind "Old version — not the rework" dev banners; "Launch a preview" answers `no-preview — no repo-backed revision to preview (S13.8)` in spec-speak; NO download control exists; the Accept card speaks payload pins/commit trailers/git-ssh-keys; "in-review" has no reviewer path (not in Inbox, Reviews page is an under-construction poster, request-revision CLOSED); machine findings are SIGNED "alice" (fifteen findings in words she never said). **Triage: the step-4 deliverable-surface rebuild is pulled FORWARD by coordinator decision — the walks prove it is the journey's final moment, not a later pass.** Scope: rendered view for markdown/HTML deliverables (the RESULT first, the diff one click away), a download door, the accept card in plain words (the mechanics stay; the language changes), in-review state honesty (who reviews, what happens next, a real door), dev banners retired or reworded for households, findings never signed as a person (attribution = wire gap, REPORT: the drain's findings rows carry the requester's identity as author).

### RA ranked frictions (walker's order, triage)

| # | Finding | Triage |
|---|---|---|
| RA-2 | Dev banners on Deliverable/Fleet/Assistant incl. confessions of known-dead controls | rides RA-B1 |
| RA-3 | The plan's promised "price" reads `UNPRICED — no per-call dollar price exists on the anthropic lane… (2.5)` — breaks the door's "plan with a price" promise | builder: one humane sentence ("included in your subscription — no extra charge"), detail one click down |
| RA-4 | "DEGRADED-MARKED" + spec citations (S08.7, D5, S13.8, S10.1, EARS) in consumer surfaces | builder sweep + wire-side strings on the G-list |
| RA-5 | Machine findings signed "alice" | wire gap (attribution) — REPORT + view label ("the platform's checker") |
| RA-6 | STAKES flips HIGH→LOW→HIGH with no reason given | builder: the badge says why it changed (classification refined) |
| RA-7 | Money in four costumes + Fleet `Weighted consumption 169378.20000000004` unit-less under "budgets" + Active-5 vs Home 0-running contradiction | builder + G-list |
| RA-8 | "In-review" with nobody reviewing; expired card "waiting on you" | rides RA-B1 / builder |
| RA-9 | Board sideways-scroll dead from the card area (the "MORE STAGES" pill is the secret door) | builder: drag/scroll from anywhere |
| RA-10 | Inbox greets with the opt-in essay before mail (third report) | builder (was W2-10; verify the fix actually landed) |
| RA-11 | Q1 asked if the website is "an email or message" right under a recap proving it knows better; `t-… · generic` tag at birth | observation: generic round-1 set before classify refines — pairs with the classifier misfiling a WEBSITE goal as CONTENT family (which also made the deliverable markdown-not-repo → fed RA-B1's no-preview). Record for the taxonomy/calibration gate read. |

### RA delights (the journey's proof)

Sign-in unlocked in place exactly as promised; flawless understanding playback with provenance; round-2 questions in her language with "You choose for me and show me what you picked"; honest waits with kept promises; safe skipping priced in words; live step numbers + progress log; the verifier's perfect sentence ("As shipped, a couple cannot view or use the page… or this is not the requested website"); the PIN ceremony. Walker's verdict: "With the asking and the making — yes, more than any software I've used. With the finishing — no."

Plain-words banners ("The honest bucket", "nothing is thrown away while it waits"); the stage timeline in human phrasing; the shipping-note spec as a checkable contract that MATCHED the result; live-updating home; receipts as real itemized tables; the accept warning ("pushes a commit under your own credentials, and nothing on this page takes one back"); consent handled seriously; board scroll-position kept.
