# RW-19 rewalk — cold walk findings (2026-08-18)

Walker: cold walker, persona = operator's spouse, signed in as **alice**.
World: throwaway B6 clickthrough on `:8485`, state `~/.sinet-rw19-walk` (left in place).
Evidence: screenshots copied to `~/.sinet-rw19-walk/walk-shots/` (names cited below; prefixes `1787030…`/`1787031…` are this walk).
Window note: the window manager pinned the browser at **584×661** — resize requests (1280, 1200, 616) were accepted and ignored. The entire walk is therefore a *narrow* walk, already below the ~616px re-check target. Harness circumstance, not a product finding.

## VERDICT: PASS

The previously-failed test — "who cancelled that task, and WHY?", answered by both routes in words a non-programmer trusts — now passes:

- **Route 1, History desk** (`status.tasks_cancelled` view): `cancelled_by` **alice** on both rows; `reason` **"we changed our minds about this one"** (Draft the release notes) and **"no reason given"** (Archive last quarter); `cause` in genuinely plain words: **"called off before it went any further."** No rule citations in the answer rows. (shots …-55, …-57)
- **Route 2, task page from the board**: banner on t-chatborn — **"This task was cancelled by alice — 'we changed our minds about this one'. Nothing runs on it and nothing more is spent; the record below stays."** Banner on t-archive — **"This task was cancelled by alice — no reason was given. …"** My own words came back exactly where I gave them; an honest "no reason was given" where I didn't. (shots …-40, …-42, …-60)
- **The two routes AGREE** — same who, same reasons word-for-word (trivial "no reason given" vs "no reason was given" difference), compatible mechanical story ("called off before it went any further" / "nothing runs on it and nothing more is spent", receipts: "cancelled … before any call was made").

The PASS is real but not spotless: rule citations DO appear on surfaces I read during the errand (findings 1 and 2 below) — just not inside the cancel story either route told me.

## Walk narration, in order

1. **Bring-up + sign-in gate.** Home greets signed-out visitors with "Sign in before you start" in plain words and an inline form; Alice preselected; PIN explained honestly ("cleared the moment the answer comes back"). Signed in, page unlocked in place. Felt respectful, not bureaucratic. (shots …-35, …-36)
2. **Board.** Five stages; Backlog held three of my tasks, all "WAITING ON A PERSON". Picked *Draft the release notes*. (shot …-37)
3. **Cancel #1, with my reason.** Task page has a red **Cancel this task** button in plain sight. The dialog is excellent: says exactly what stops ("parked — It closes now, and any open question card on it closes with it. It will not resume afterwards."), the why-box is labeled "**Why? — optional, one line, kept on the record of every run this stops, so anyone who finds this task later knows**", and the safe exit is "Leave it alone". Typed *"we changed our minds about this one"*, confirmed. Banner instantly showed my words back, attributed to alice. (shots …-38, …-39, …-40)
4. **Board chip confusion.** Back on the board the card wears **"CANCELLED — OPEN FOR WHY"**. I had *given* a why — so my first read was "it thinks the why is still open / my reason didn't save." No tooltip. Only after cancel #2 produced the *identical* chip did I decode it as "open [the task] for [the] why". (shots …-41, …-59)
5. **Cancel #2, declining to say why.** *Archive last quarter*, same dialog, left the box empty, confirmed. Banner: "cancelled by alice — **no reason was given**." Exactly the honesty I wanted. (shot …-42)
6. **The record below** (both task pages): "Human decisions" line carries who + why; deliverables section explains its own emptiness; receipts say "Nothing itemized — … A run cancelled or ended before any call was made leaves exactly this." All trustworthy — until the receipts *fine print* (finding 2). (shots …-43, …-45)
7. **History desk, my own words.** Typed *"who cancelled the release notes task, and why?"* — the desk could NOT match it: "It could not turn what you asked into one of them, so nothing was run. Pick one and it becomes your question — you still ask it yourself." Honest, and the right catalog entry was described in perfectly plain words ("tasks that were cancelled or stopped — who cancelled each one, why, and when, most recent first") — but it's a bounce, and the surrounding dress is machine-speak ("catalog.disambiguation layer 1 confidence: canned", "the local tier is not wired here, so intent cannot be classified"). (shots …-48, …-49, …-51)
8. **The cancelled view answers.** A table: both my cancels, `cancelled_by` alice, my reason word-for-word, "no reason given" for the other, `cause` "called off before it went any further". The data is the whole truth; the presentation is a database dump — `task_id`, `user_id`, `event_seq`, `cancelled_ts` headers, and at this width the who/why live past a sideways scroll. (shots …-53, …-55, …-57)
9. **Route 2 fresh read.** Board → t-chatborn: banner tells who + my why + "nothing more is spent" in one sentence. Routes agree. (shot …-60)
10. **The stale card.** The cancelled task's page still shows a bright **"Answer its open card"** button — the same dialog had promised the card "closes with it". Pressing it opens an Approval page chipped **"YOURS TO ANSWER"**, saying "**still waiting on you**", plus a yellow strip citing "**pending longer than ⊙ freshness.max_age — … re-plan is one click (S06.9)**" and a closing note that the card "**declares no answer verbs, so there is nothing here to press**". Three surfaces, three stories: dialog says the card closes, task page says answer it, card says it's mine yet unpressable. (shot …-61)
11. **Narrow re-checks** (window already 584px, below the 616 target): task-page cancel story wraps cleanly, fully readable, no page-level sideways scroll (shot …-62); History cancelled view keeps its table inside its own scroller but hides `reason`/`cause` behind the swipe (shots …-65/-66).
12. **Sign-out.** One click; History re-locked in place with a plain explanation. No session left behind for the next visitor. (shot …-67)

## Ranked frictions (worst first)

1. **The cancelled task still dangles its question card.** Cancel dialog: "any open question card on it closes with it." Task page after cancel: an inviting "Answer its open card" button. The card itself: "YOURS TO ANSWER … still waiting on you: passing the date withdrew nothing and answered nothing." This contradiction lands exactly where the feature just earned my trust — I cancelled, was told everything stops, then the platform kept a question open in my name. (shots …-40 vs …-61)
2. **Rule citations and settings names on household surfaces.** Task-page receipts fine print: "no mode change (**S10.6** downgrade ladder lands with routing **S08**/local tier **S12**)", "direct-use estimate (heuristic): no price table loaded at v0 (UNPRICED, never a silent $0 — **S10.1**) · **Spec/benchmark-preregistration-v1.md §13** … benchmark machinery (**S14, B5**) …". Card page: "**freshness.max_age**", "**(S06.9)**", "declares no **answer verbs**". Per the errand's own rule, each of these is a finding. None of them touch the cancel story itself, which is why the verdict survives. (shots …-45, …-61)
3. **The own-words desk didn't understand a plain question.** "who cancelled the release notes task, and why?" is the most natural question this errand has, and it bounced to a catalog pick. The bounce is honest and the recovery is one obvious click — but a non-programmer's first free-text attempt failing is still a stumble, and the explanation ("the local tier is not wired here, so intent cannot be classified") is engineer-speak. (shot …-49)
4. **History answers dressed as a database.** Headers `task_id`/`user_id`/`event_seq`/`cancelled_ts`, card header "status.tasks_cancelled layer 1 confidence: canned", "Layer 0 — a named view"/"Layer 1 — a catalog question" form labels. At narrow width the two columns I came for (who, why) are the ones hidden behind the sideways scroll. The content passes; the costume is for programmers. (shots …-47, …-55)
5. **"CANCELLED — OPEN FOR WHY" chip is ambiguous.** Reads as "the why is still an open question"; shown identically whether a reason exists or not; no tooltip. I doubted my own reason had saved. (shot …-41)
6. **Machine residue in the record sections.** Nanosecond UTC timestamps ("2026-08-18T05:28:46.616184271Z") beside every relative time, event names like "run.summary_written", run ids like "t-chatborn.intake", and "ran for 28 d 20 h" on a task that never ran anything (it counted record-to-record, and says so — but the headline reads odd). (shots …-43, …-60)

## Delights

- **The cancel dialog** is the best-written destructive-action dialog I've seen in this product: exact consequences, an optional why with a human justification for asking, and "Leave it alone" as the no-op.
- **My words came back.** Banner and record both show *"we changed our minds about this one"* attributed to alice — nothing paraphrased, nothing dropped.
- **Honest absence, everywhere.** "no reason was given", "Nothing itemized", "This answer carries no rows. The layer answered — it simply matched nothing. A refusal and a disambiguation card are answers too."
- **`cause` in plain words** — "called off before it went any further" is exactly how a person would say it.
- **Sign-in/sign-out both unlock/relock the page in place**, with prose that explains *why* the gate exists instead of just barring the door.
