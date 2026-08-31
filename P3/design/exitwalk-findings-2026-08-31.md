# Exit cold walk — 2026-08-31

Fresh-persona walk over both landed fix lanes (`821c872` frontend, `a390e81`+`7bc53a7` backend), from HEAD `5fc245a`.
Walker: a fresh-context session playing a non-programmer household member; had read none of the code.
World: brand-new throwaway at `~/.sinet-exitwalk`, port 8497, binary built from a clean HEAD worktree
(the operator's uncommitted working-tree edits were deliberately NOT in the build). Local tier via
`./P3/gates/local-stack.sh` (llama-swap on 127.0.0.1:41741, six seats). First user minted through the
platform's own first-run door (operator `darian`). Production (:8481/:8482) and every existing
`~/.sinet-*` world untouched.

**Verdict: 8/8 errands PASS** — one of them (accept) only after a rig-side fix that itself exposed a
real wording defect. Findings below, graded. Nothing here is a talk-myself-into-a-pass: the two
mid-grade wording defects and the missing-choices currency card are called out exactly as felt.

---

## The errands

### E1 — Give real work · PASS
Signed in, made project `soap-stand` (the onboarding approval card is a model of plain speech:
danger zones, "what could go wrong", "Approve is the only verb this card has"). Described the goal in
one sentence; the interview came as a short form with one-click recommendations and an honest
"standard wording throughout: this round was not rephrased for your goal" note when the local phrase
seat degraded (llama-swap 500 behind the scenes — the card said so instead of faking it).
Gave the six soaps as bare numbers, no currency anywhere.

**The catch happened, unprompted and fail-closed.** A dedicated round arrived: *"The prices are given
as bare numbers like 4.50 and 7.00, but no currency is named. Which currency should the prices show —
for example dollars, euros, or pounds — and should each price include the currency symbol (like
$4.50)?"* — and the plan explicitly refused to move without it: "these open points hold the plan — it
refused to guess them". I typed "leave the numbers just as they are" and the plan said it out loud in
three places, best of all as a first-class assumption: *"The prices are written as plain numbers with
two decimal places and no currency sign, because you told me to leave the numbers exactly as they are
with no currency anywhere. · confirmed by you during intake"* plus "Any currency symbol on the
prices" under WHAT I WILL NOT DO.

- **F2 (medium-low):** the currency question offers NO click choices — every other question has
  option chips, this one is a bare free-text box. The errand's expected "leave the numbers as they
  are" option had to be composed by hand. No skip either (deliberate — it refuses to guess — and the
  refusal is honestly worded; but chips like "dollars / euros / leave them bare" belong here).

### E2 — Skip and read yourself back · PASS
Skipped "what should it achieve" (round 1) — the understanding panel showed it as *"assumed — out
loud · you skipped this one, so it picks something sensible and shows it on the plan"*. Every other
item read back in MY words with a "you answered" tag — the choices I clicked, no codes anywhere
("A document or report", not `document`). The "Change my answers…" editor reopened every question
with my actual text pre-filled (the six-soaps line came back verbatim, editable), a labeled
"RECOMMENDED FOR THIS GOAL" value one click away ("Start from this"), and "Keep it as it is" to back
out. Pending edits get a pencil and "changes to: …" before the redraft; the button counts them
("Save 1 change: redraft the plan").

- **F1 (medium):** on the first plan, the ASSUMPTIONS block said the skipped fact TWICE — once as the
  picked assumption ("The list's job is to let people walking past…") and once as the stale
  placeholder ("you skipped this one, so I **will** pick something sensible…"), future tense after it
  had already picked. Two rows, one fact, genuinely confusing for a first-time reader. Same root on
  the editor: the skipped item's "Now:" line shows the canned placeholder instead of the picked
  value. This is the disclosed GF9 M5 residue (third planner origin-spelling renders both rows) — it
  is real, I felt it, and it should stay on the ledger. It self-healed after I answered the question:
  the redraft removed both rows and SAID so ("removed, and this page keeps it visible so nothing
  disappears silently").

### E3 — The waiting face · PASS
Every wait was narrated truthfully with honest time promises ("a question round takes about a minute
on the local models; the plan is drafted in one piece and can take a few minutes"), a live elapsed
readout ("listening · 18 s on this page"), and "You can leave; nothing is lost". **The deliberate
mid-wait reload survived completely**: same card, answers kept, work continuing — and the elapsed
honestly restarted as "3 s on this page" because it is page time and labeled as such. The task card's
stage rail later narrated every park in plain words with durations ("parked 12m ago · resumed after
1 min 51 s — waiting for your answer on the interview questions — nothing else happens until that
card is answered").

- **F5 (low):** after a change, the waiting face reuses the generic "choosing the next questions,
  or — once it knows enough — drafting the full plan" copy. The button that got me there said "the
  plan is redrafted with these"; the waiting card never says *redrafting*. (The plan-sent-back path
  DOES have its own copy — "it is re-planning what to ask you" — so the gap is only on the
  save-changes redraft.)
- (nit) While waiting, the header drops the stakes chip and must-knows meter until the next card.

### E4 — Stakes, one truth · PASS
One truth, told coherently through its life: born *"STAKES: HIGH — handled with extra care —
approving the plan asks for your PIN"*, then settled *"STAKES: LOW — was high — the platform refined
its reading of the goal as it learned more"* (the GF14 abstain-settle, watched happening live), and
task 2 was even honest about provisionality: *"first reading — it settles as the questions are
chosen"*. No surface contradicted the level anywhere — and the LOW approval indeed asked no PIN,
consistent with the HIGH-only PIN sentence.

- **F7 (low):** on the plan and task cards the chip stands alone — no "why" in words, none on hover
  (the why only ever appears on the intake header). And the accept door calls accepting "a
  high-stakes act" two screens under "STAKES: LOW" — different concepts (act vs task), coherent on a
  careful read, but the vocabulary collision is avoidable.

### E5 — Approve and receive · PASS
Approved; the run executed on claude-sonnet-5 (anthropic subscription lane) and verified on
claude-opus-4-8 inside a minute. The deliverable page rendered the price list as a REAL table —
title, Soap/Price columns, all six rows, light readable ground — at desktop, and at a real 390px
pass (same-origin iframe): no horizontal overflow (scrollWidth == clientWidth == 376), table 350px
wide, phone nav kicked in, diff pane wraps. Download: "Download deliverable.md (175 bytes)" produced
exactly that file in ~/Downloads, byte-honest content. Timestamps read human ("opened 12m ago",
"just now") with exact ISO stamps on hover (`2026-08-31T20:18:43Z`).

### E6 — The money question · PASS, with reservations
Receipts were findable from the task card, per run, with a Purpose column in actual words
("planning & questions (ceremony)", "doing the work", "verification"), model and lane named, "USD 0"
totals, and the unpriced situation explained honestly: *"subscription lane — no dollar price exists
for these calls; the receipt lists them as UNPRICED"* and *"the platform leaves the cost blank rather
than quietly showing nothing owed"*. Home's tile: "SPEND TODAY darian USD 0 · per person, 2026-08-31
(UTC)". A non-IT person follows all of that.

- **F6 (medium-low):** the same receipt block then cites **`Spec/benchmark-preregistration-v1.md
  §13`** — a repository file path with a section sign, on a money surface, meaningless and slightly
  alarming to a household reader. And "currency api-equivalent · worst approximation tier 1" is
  engineer dialect. The sentences AROUND these are plain; these tokens are not.
- (nit) "1 steps" in the live-activity summary.

### E7 — Accept, then compare · PASS (after a rig-side fix; one real defect found)
The ceremony explains itself fully before asking anything: accepting revision 1 = "a commit in
soap-stand's own store on this machine, in your name — this project has no shared remote, so nothing
is sent anywhere", technical record one fold away, PIN typed as part of the same request, "nothing on
this page takes an accept back… never an undo".

- **F3 (medium): first accept attempt failed with a raw internals string** on the operator surface:
  *"The control plane answered 500: accept: resolve signing posture: broker: dial unix
  /home/sinep/.sinet-exitwalk/broker/sinep.sock: connect: no such file or directory"*. Loud and
  fail-closed — correct posture — but a Unix socket path is not an operator sentence. Cause was
  rig-side: no per-person broker daemon runs in a bare `control` world (lane-test-door.sh documents
  exactly this wiring fact), so signing could not resolve. The DEFECT that stays: the error reaches
  the requester unhumanized. Wanted: "a helper the platform needs for signing is not running on this
  machine" + where to look.
- After starting `sinet broker --state-dir ~/.sinet-exitwalk --user sinep`, the same click accepted
  cleanly. Landing statement exemplary: *"one commit, credited to you, is now on this project's main
  branch in its own store on this machine… This project has no remote address, so that commit is the
  official copy and nothing was sent anywhere"*, with State/Commit/Effect/"nothing was superseded"/
  "no active run needed re-validation" rows. Store verified: commit `0a0f71b`, author Darian,
  honest Co-Authored-By trailer naming claude-sonnet-5.
- **The compare door told the truth after accepting**: pre-accept it said "With nothing accepted yet
  to compare against, this shows the one version there is rather than a pair"; post-accept it says
  *"The version you accepted is shown beside this one, so you can see what changed"*. No stale
  nothing-accepted claim anywhere (the GF14 F8/L10 fix, seen working live). Accept flipped CLOSED
  with a plain reason; request-revision flipped OPEN as a follow-up. 
- **F4 (low):** in-place residue after the retry: the failed attempt's 500 string stays rendered
  above the success record until reload, and the now-closed accept door still shows its leftover
  title/note/PIN form. Both truths on one screen reads as "did it fail or work?".

### E8 — Change your mind · PASS
Second tiny task born; "Cancel this task" opened a visually distinct red card — *"Cancel this task?
It stops here — nothing more is asked, started or spent, and the task keeps its record, marked
cancelled"* — with an optional reason line "kept with the record of what was stopped" and a separate
"Cancel the task" / "Keep going" pair (confirm clearly distinct from the door). The reason survived
verbatim, attributed: *"This task was cancelled by darian — 'Changed my mind, I will write the note
myself.'"*, and the stage rail records "resumed on cancelled by darian".

- (nit) The Board card says "CANCELLED — THE TASK SAYS WHY · USD 0 priced" — a pointer, not the
  reason. One click away, and honest about it, but the reason itself on the card would end the trip.

---

## Other small warts (all nits)
- "MUST-KNOWS — 0 of the 60 needed": "the 60" reads at first sight like sixty questions; it is a
  score threshold. One word ("points") would fix it.
- The header family tag reads `generic` before classification settles, next to the card's "I'm
  treating this as write or create content" — two different vocabularies for the same slot; the tag
  later flips to `content`.
- "What it should achieve — — [assumed — out loud]": the empty value renders as a second dash.
- The review comment strip shows `category RESEARCH-NOT-RUN` and `as supplied: step:S-1` — code
  tokens inside an otherwise plain (and admirably honest) checker note.
- The checker's note itself is the platform on its best behavior: "the platform does not yet keep a
  per-step record of what was looked up, so this could not be checked either way — recorded, not
  silently passed".

## Rig artifacts — NOT platform findings
- The Chrome extension's synthesized mouse events never reached the page (verified: zero mousedown
  listeners fired; elementFromPoint showed targets unobstructed) — fractional display scaling
  (dpr 1.467) plus a background window. The walk was driven by real keystrokes where they landed and
  programmatic clicks elsewhere; hover states went untested. The one thing left unverdicted: Enter
  in the sign-in PIN field appearing not to submit (twice) — likely the same rig fault, but worth one
  human keystroke at the operator sitting.
- Screenshots crop a 1745px viewport at 1392px (window manager refused resizes); right-edge
  "clipped text" in the shots is the capture, not the page (page scrollWidth == clientWidth).
- The 390px pass ran through the same-origin-iframe technique, same as the GF9 rig.

## Screenshot index — `~/.sinet-exitwalk/shots/` (57 frames, walk-04 … walk-60)
- walk-04..09: first-run door, operator mint, sign-in card
- walk-10..15: signed-in home, projects empty state, New project dialog, onboarding started
- walk-16..18: Inbox, onboarding card expanded, approved (badge 2→1)
- walk-19..27: Describe a goal; interview round 1 (stakes HIGH, guess card, skip, "Send: 3 answered,
  1 skipped")
- walk-28..30: round 2 answered; waiting face; **post-reload waiting face ("3 s on this page")**
- walk-31..34: **stakes settled LOW ("was high")**, understanding panel, **currency question**,
  answer saved
- walk-35..39: redraft wait; plan v1 (assumption dup visible in walk-38/39, cost, who-does-it)
- walk-40..43: answers editor (pre-filled facts, skipped item's editor, pencil "changes to")
- walk-44..45: **was/now change view (strikethrough)**, approved card
- walk-46..47: task card DONE, deliverable page with **rendered table**
- walk-48..51: **390px iframe passes**, accept ceremony open
- walk-52..54: first accept (stale pre-broker frame), **ACCEPTED banner + full table**
- walk-55..59: task 2 interview ("Take this" recommendation), **cancel door + reason**, cancelled
  card, board with CANCELLED tag
- walk-60: home after the walk (SPEND TODAY darian USD 0)

## Spend
USD 0 cash. All engine work rode the subscription lane (anthropic: 2× claude-sonnet-5 execute,
1× claude-opus-4-8 verify, 3× opus intake) or the local tier (7 calls, $0); task 2 consumed a few
local intake calls before cancellation. Receipts on both task cards agree: "Total priced USD 0",
remainder UNPRICED subscription calls.

## World disposition
- State dir `~/.sinet-exitwalk` KEPT as evidence (db, logs, shots/, broker store, project store with
  accept commit `0a0f71b`, downloaded `~/Downloads/deliverable.md`).
- All processes STOPPED and verified down: control plane (:8497 free), broker, llama-swap
  (:41741 free, model unloaded before kill). Production :8481/:8482 never touched.
- Build worktree removed; binary kept at `~/.sinet-exitwalk/bin/sinet` (HEAD `5fc245a`, web bundle
  built from the same tree).

## What the drains bought (seen live)
The abstain-settle landing LOW with its "was high" sentence; the currency backstop holding the plan
hostage until answered and the choice landing verbatim in the assumptions; the compare door telling
the truth on both sides of an accept; plain-word stamps with exact time on hover; the receipt
refusing to claim "nothing owed". The remaining wounds are wording at the edges (F1 dup row, F3 raw
broker error, F6 spec citation on a receipt), not structure.
