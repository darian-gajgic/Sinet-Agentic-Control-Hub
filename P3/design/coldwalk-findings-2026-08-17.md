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

## Walk 2 — the oversight walk (spouse persona, kept world) — PENDING, appended on completion.
