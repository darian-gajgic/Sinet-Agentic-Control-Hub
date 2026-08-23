# Operator findings — B6 gate, round 5 (2026-08-23): the interview/planning phase, benchmarked live against Nexus

The operator ran the SAME goal (a BMW/Audi car-parts webshop, full prompt below) through Sinet's
landed GF3 interview AND through Nexus's Deep Plan, side by side, and reported free-text findings
(authoritative). Verdict: **Sinet's interview asks some genuinely good questions, but is riddled
with generic, purposeless, and confusing ones; Nexus's planning phase is "in a lot of ways better,
in some ways worse" — the ordered work is a MERGE of the best of both, raised to the operator's
stated target, not a swap.**

**Escalation stake, recorded verbatim in substance: the operator is considering scrapping the
Sinet frontend entirely and replacing it with the Nexus frontend.** ("All you produce is shit,
buggy, unorganized, weird text nobody understands. I'm really thinking about to screw this
frontend and just use the frontend from Nexus.") This is the second consecutive round where the
planning surface failed the operator; the stake is real and belongs in every prioritization call
until this is fixed.

This round DEEPENS r4-F2 (interview taxonomy) and r4-F3 (plan black box / editable understanding)
with transcript evidence, confirms the review's RA-4 (the post-draft textbox regression) from the
operator's own hands, and adds one new mandatory evidence step: **a LIVE click-through of Nexus's
planning phase** (the docs-side study `gf3-nexus-deepplan-study-2026-08-23.md` exists and was not
enough — the landed build proves it).

F1 (the verification dead end) is untouched by this round and STAYS FIRST — the operator opened
with "this is just a small part of what needs to be fixed."

---

## The test goal (both platforms, identical prompt)

Project "Create a webshop for car replacement parts and tuning parts": BMW/Audi focus, ≥30 seeded
items with picture/price/info/compatibility, item detail view, cart without payment, modern design
with basic animations, brand+model search filter, v1 fully working.

## A. The Sinet transcript — per-question operator verdicts

Slot IDs are from `internal/intake/taxonomy.go` (software family) so the taxonomy pass can target
code directly.

### Round 1 — "overall not bad"

| Slot | Question | Verdict |
|---|---|---|
| `collection_semantics` | things it keeps track of / duplicates | KEEP |
| `comparison_rules` | what order things come in | **FIX the options, keep the slot.** "Closest to what the person was looking for" is unanswerable: how does the system know what the person is looking for? Does this govern the search results or the default catalog view? The user cannot tell what they are choosing. This is the recurring class: recommendations that never say what they refer to or what effect the choice has. |
| `ordering_atomicity` | strict order / all-or-nothing | KEEP, with the note: a non-IT operator CANNOT answer this — the "system decides and shows you" path is the only honest default and must be the recommended one. (It exists; the summary's "you skipped this one, so I will pick something sensible" landing was accepted.) |
| `technology_stack` | what should this be built with | KEEP |

The "What it understood so far" summary on this page: "generally not bad."

### Round 2 — two kills, two keeps

| Slot | Question | Verdict |
|---|---|---|
| `behavior` | "What new ability will this give your customers that they don't have now?" | **KILL.** "Complete bullshit… stupid generic question which does not fit to the project itself. I have no idea what to answer on this question and what the purpose of this question even is." The options are abstract category labels. This is eval-F6 ("the behavior slot's options are low-content categories") now independently condemned by the operator. |
| `terminology` | "Are there any words in your request that have a special meaning in your industry?" | **KILL as a question; INVERT the mechanism.** "How the fuck should I know… Tell me what you understand and then I can correct it or confirm it." The user must never be asked to introspect on what the system MIGHT misread — the system states its understanding, the user corrects. |
| `edge_cases` | unexpected happens: loud or quiet | KEEP (good) |
| `assets_media` | where should pictures come from | **KEEP — "the first good question asked."** Concrete, prevents a wrong assumption, options are real. The template for what every question should be. |

### Round 3 — one keep, one kill, two fixes

| Slot | Question | Verdict |
|---|---|---|
| `look_feel` | how should it look and feel (with a project-specific recommended option: "like a sleek, modern car dealership website…") | KEEP — the recommendation being written FOR this goal is exactly right. |
| `indices_ranges` | "do we count both the start and the end number?" | **KILL.** "Is this a fucking joke? … 30 items mean 30 items! This is embarrassing if this question gets asked to a user." Resolve internally; never ask. |
| `output_format` | "How should the final website code and files be organized and presented to you?" | **FIX or KILL.** The user cannot tell WHAT is being asked (the website's design? the folders and files?) nor what problem it solves. |
| `units` | measurements / metric vs imperial | **FIX the why-line.** The question itself was understood; the info line "Mixed-up units stay invisible: everything looks right until a number is ten times off" confused the operator — "It's asking and describing nonsense here." Why-lines must explain in plain project terms or say nothing. |

### Round 4 — the textbox regression (= review RA-4, now operator-confirmed)

Two post-draft open points arrived as **bare textboxes with "Info: Nothing"**:

1. Currency (assumed EUR) — the question is RIGHT, the form is WRONG: "why I can't select euro or
   dollar… by a multiple choice? Why I have a text box all of a sudden?" A finite set must be
   choices; currency is the canonical case.
2. "The request mentions 'version' — confirm whether part records need explicit part/version
   numbers or interface-version metadata…" — jargon a non-technical user cannot parse or answer,
   with no effect explanation, as free text.

Operator: "the feeling the system is even more lazy and bored by itself… it looks like a stupid
questionnaire." This is exactly RA-4 from the GF3-FE review appendix (r4): the post-draft
open-points surface regresses to the pre-GF3 pattern (prose-only, no options, no recommendation,
no skip). Both records now point at the same defect from two independent witnesses.

## B. The Nexus benchmark — what it does better (operator's live run, same goal)

1. **The understood spec is a first-class, EDITABLE surface.** Goal / stack / acceptance criteria /
   users / constraints / data-integrations / out-of-scope / risks / notes-decisions are each shown
   as text the operator can hand-edit — before questioning, again after answers, and again at the
   drafted spec before start. "Everything he assumed and wrote by itself… displayed as a text box
   where I can edit it myself, which is completely missing in Sinet." This SETTLES the r4
   "raw-plan-editing question (operator ruling owed)": the operator has ruled by evidence — they
   want it.
2. **It demonstrates understanding instead of interrogating.** From the same prompt Nexus derived a
   detailed, correct, testable acceptance-criteria list (EUR formatting, filter-clears-restores,
   cart quantity-change, no reachable payment, responsive + console-clean), plausible risks, an
   out-of-scope list, and sensible assumptions (EUR because the operator is in Germany) — all
   BEFORE asking anything.
3. **Questions only where it is genuinely unsure, each tied to a named spec field**, with
   project-specific concrete suggestions (Next.js + Tailwind; static catalog JSON + localStorage
   cart; Pexels/Unsplash cached locally) and real why-lines ("Repo is empty so the stack is unset;
   it shapes how v2 (payments, accounts) can extend the code").
4. **The loop is understand → show → correct → commit** (answers update the displayed
   understanding, which stays editable; then draft-plan → editable spec → start), not
   questionnaire → assume.
5. **Nexus's weakness — why this is a merge, not a swap:** it asked only 3 questions for this goal
   ("too few questions to make an actual plan"). It misses Sinet's genuinely good ones
   (edge-case posture, media sourcing, the duplicates rule, look-and-feel). "In a lot of ways
   better, in some ways worse, and of course not as good as the recommended state-of-the-art."

## C. The operator's target state (their words, distilled into hard rules)

1. The system must UNDERSTAND the task and work out which information the work needs BEFORE it
   starts — and catch what the user forgot in the initial prompt. Gap-finding is the point of the
   interview, not slot-filling. ("The user might also make mistakes and forgot some important
   information; the system has to catch that missing information by asking.")
2. Never ask what the system can resolve itself (`indices_ranges`).
3. Never ask the user to introspect on the system's possible misunderstanding (`terminology`) —
   show the understanding, let them correct or confirm it.
4. Every question carries: a plain purpose (what breaks if unanswered), project-specific options,
   a recommended default, WHY it is recommended, and what EFFECT the choice has. No cryptic
   why-lines, no options whose referent is ambiguous (`comparison_rules`).
5. Multiple choice wherever a finite set exists (currency). A bare textbox is the exception for
   genuinely open answers, never the fallback — and "I will describe it in my own words" stays
   available everywhere.
6. Mandatory open points (post-draft markers) get the SAME treatment: options, recommendation,
   skip/system-decides where honest (RA-4).
7. The understood spec is shown editable — before drafting, after answers, and at the drafted
   plan. "Guide the user by the hand, tell him what it understands, make recommendations, have a
   discussion with him." 

## D. The ordered work this round adds

- **W1 — LIVE Nexus click-through (evidence step, before building).** A session with Chrome drives
  the SAME goal through Nexus's full planning phase to a produced task (Nexus at
  `~/Nexus-Agentic-Coding-Setup`; the operator's run used repo `~/Projects/car-webshop-test`).
  Record every question, every displayed/editable surface, every assumption and where it surfaces.
  Harvest list in hand BEFORE the taxonomy pass. (Internal engineering study only — Nexus is never
  cited in any deliverable, per standing rule.)
- **W2 — taxonomy v4** from the now-THREE real transcripts (car-parts r3, GPU-shop r4, car-parts
  r5) + the Nexus harvest: kill `behavior`-as-asked, `terminology`-as-asked, `indices_ranges`;
  fix `comparison_rules` option clarity, `output_format` intelligibility, `units` why-line; keep
  `edge_cases`, `assets_media`, `look_feel`, `technology_stack`, `collection_semantics`,
  `ordering_atomicity` (system-decides recommended). Folds into the pending v3 ratification (now a
  v4, same PENDING machinery). Backend four-stage for taxonomy/planner changes.
- **W3 — the editable understood-spec surface** (the Nexus centerpiece): full derived
  understanding shown as editable fields at the pre-draft, post-answers, and drafted-plan stages.
  Settles the r4 raw-plan-editing ruling in the affirmative. FE per FRONTEND.md + whatever wire
  the artifact needs (four-stage).
- **W4 — the open-points surface rebuilt to interview standard** (RA-4 + this round's round-4
  evidence): options, recommendation, why, effect, skip — currency becomes the worked example.
- Ordering: **F1a+F1b remain first and alone** (operator: "this is just a small part"). W1–W4
  then form the merged F2+F3 act — they subsume r4's items 3 (F3) and 4 (F2) — ahead of F6/F7/F5.
  r4-F4 (per-item contest notes) and the RA-1..RA-3 honesty fixes ride in the same FE rounds.

## E. Coordinator notes

- The `comparison_rules` ambiguity (search vs default view) is a real content defect: the option
  text never binds itself to a surface. The fix is per-goal option phrasing (the `look_feel` slot
  already proves per-goal phrasing works: its round-3 recommendation was written for this project).
- The round-4 textboxes are the planner's NEEDS-CLARIFICATION markers surfacing raw — same family
  as RA-4/RA-5 (machine tokens on requester surfaces); one rebuild covers both records.
- S06.6/S06.10 spec reads owed before W2/W3 land (r4-F3's "already implied vs amendment owed"
  question stands).
- The operator's praise fixes the quality bar: `assets_media` in round 2 and `look_feel` in
  round 3 are the reference questions — concrete, project-specific, assumption-preventing, with a
  real recommendation. Every surviving slot must meet that bar or die.
