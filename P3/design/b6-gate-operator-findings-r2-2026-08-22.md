# Operator findings — B6 gate click-through, round 2 (2026-08-22, post-GF1)

The operator restarted their gate world (:8483, now on the GF1 build) and continued. Free text authoritative. Verdict: **not satisfied — rework again**; plus one hard bug. "A non-technical user should be able to operate this platform" is the bar.

## Findings (with coordinator investigation)

| # | Finding (operator words) | Investigation |
|---|---|---|
| 1 | **BUG:** Board → the gpu-hardware-shop task → "Answer its open card" → lands in the Inbox → "I can't answer anything there" | **REPRODUCED** on the review world (garden-plan interview task, same shape). The door lands correctly on the single-card view; the card says YOURS TO ANSWER, lists the interview questions, then renders *"this card declares no answer verbs, so there is nothing here to press."* (`Inbox.tsx:2236`, landed at B6-6 — pre-existing, not GF1's). The card's only link is "Open the task this came from"; the task's only affordance is "Answer its open card" — **a closed circle**. The REAL answering surface exists and works: `/new?task=<task_id>` resumes the interview with option chips + free-text + Send (verified live). Nothing links to it. The operator's world has TWO such open intake asks (`intake:t-4513468dd5bc948d:1` decision.family, `intake:t-cbe4d8955937f7cc:4` interview, both owner `op`). Fix: FE wiring — every answerable card carries its real door; the task-page door for intake asks goes straight to the resume journey |
| 2 | "Opt-in and opt-out… nobody knows what this shit means" | The benchmark panel's switch renders buttons "Opt in"/"Opt out" (`Inbox.tsx:805/810`) and outcome copy "opted in/opted out". Jargon; the operator's founding bar is non-technical operability. Fix: plain-words rewrite of the whole panel's verbs + a jargon sweep over every button label on the inbox surfaces |
| 3 | The GPU-hardware-shop ticket is pure information, nothing pressable — "why you spam my tickets full of that… I have 72 inbox tickets" | The flood is real live watch activity in their world: **447 `drift.finding` events since 2026-08-12** across seeded watch rows (modelsdev ×95, localllama ×94, qwen-code ×71, opencode ×43, …), each fingerprint folding into a NEW card every 24h incident window inside the 30-day horizon → dozens of standing drift cards. Dismiss EXISTS and is served for the operator (`POST /api/approvals/{id}/dismiss`, B6-2B) but is buried. Fix (presentation): actionable/notice split — see the ordered rework |
| 4 | "If there is nothing to do or decide, or no important information to read, don't spam my inbox" | Ordered rule for the inbox's information architecture (below) |

## The ordered rework (round 2 — all frontend)

1. **The inbox is the queue of what needs YOU.** Default view + the sidebar badge count = only cards the signed-in person can actually act on (a decision, an answer, a sign-off). 
2. **Info-only cards leave the main queue.** Drift/notice-class cards move to a separate collapsed "Notices" area (or secondary tab on the same surface): grouped by source in plain words, count per group, expandable. Nothing is dropped — every served card stays reachable (forward tolerance holds).
3. **Notices are clearable.** Surface the served dismiss as a plain-words act ("Got it — clear this"), with a per-group clear-all that issues the served per-card verb sequentially (presentation over served actions, no new wire verbs).
4. **No dead "yours to answer" cards, ever.** A card marked yours-to-answer must carry a working door. Intake interview/family cards: the door is "Continue answering its questions" → `/new?task=<task_id>` (verified working); the task page's "Answer its open card" goes there directly for intake asks. If a kind genuinely has no route, it must not claim to be answerable.
5. **Jargon sweep.** "Opt in/Opt out" → plain words; audit every verb label and chip on the inbox + card details for operator words. Fold in the GF1 walk G-notes while at it: GF1-W1 "Remote to clone" copy, GF1-W2 Register-button layout shift, GF1-W3 the HIGH-stakes badge tone on trivial projects.

## What stays binding

Backend untouched (dismiss/answers are landed verbs; the split, the doors, the copy are presentation); the card is the authority (no invented verbs); tier/PIN/batch mechanics untouched; every served card reachable; honest absence; escape-by-default. The deeper drift-flood triage (server-side suppression/frequency) stays on the deferred ledger — the split addresses the symptom at the surface; the wire-side ranking gap (W2-10 family) stays listed.
