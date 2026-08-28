# GF9 exit-gauntlet LIVE DESIGN REVIEW — the planning surfaces, 2026-08-28

**Reviewer:** fresh-context agent (never the builder), real Chrome, per FRONTEND.md rule 5.
**Scope:** correctness + requirement gaps + Design Quality / Originality / Craft / Functionality on the
planning surfaces the GF9 round rebuilt, judged against the r5 §C seven hard rules, the r4 appendix
RA-1..RA-16, and the two walk friction lists (walkf1, e5rw2). GF12's R11 headline (the live
draft→contest→redraft journey) rode this review.
**Rig:** binary from HEAD `a42a21ca30ba+dirty` (`sinet v0.0.0-dev`), fresh world `~/.sinet-gf9-review`
on `127.0.0.1:8487` (8481-8486/8488-8490 never touched), broker running
(`~/.sinet-gf9-review/broker/sinep.sock`), local tier wired (`SINET_LOCAL_ENDPOINT=http://127.0.0.1:39617`,
control.log `local: S12 duty surface wired … configured=true`), dev posture. Seed operator `darian`
minted via the anonymous first-user bootstrap (201); PIN in `~/.sinet-gf9-review/rig-notes.txt`.
Real engine calls sanctioned, all subscription-lane UNPRICED, $0 cash.
**Screenshots:** `P3/design/rework-screens/gf9-review/` (index at the end). Every claim below is
something I saw first-hand; where a tool artifact intervened I say so.

---

## VERDICT

**SHIP-BAR MET** — with one carve-out the rig forced (the 390px pass, below) and a three-item fix
list (M1–M3) for the drain round.

**Grades: Design Quality A- · Originality A- · Craft B+ · Functionality B+ · Blocking findings: 0**

The journey the operator has failed twice ran end to end, live, first try: onboard `wick-wax` →
family guess + correction affordance → two interview rounds (recommendations, own-words, skip,
per-item "fix this" correction that rode the send) → an earned open point (the plan refused to
guess prices) → the full plan with per-step HOW drawers → a REAL two-target contest with per-target
note boxes → **one redraft, no crash, no confirm-loop (GF12 R11 verified live)** → a legible
WHAT-CHANGED diff block → approve without PIN at LOW stakes → execute → verify → review → **a real
accept: one PIN submit, commit `0588fcb5093b10bf9aad67bd6a26a6ac97353dca` in wick-wax's own
remote-less store, trailers truthful (`Co-Authored-By: claude-cli claude-sonnet-5 …` /
`Assisted-by: claude-cli (claude-sonnet-5) via Sinet`), $0.** Ground truth checked in the store:
author Darian, both trailers byte-exact, `git remote -v` empty.

## The GF9 claims, audited one by one

| Claim (queue row / log) | Verdict — what I saw |
|---|---|
| Family chip + correction affordance | **VERIFIED.** "I'm treating this as **write or create content** — my guess from your goal · not right? change it"; the expansion explains WHY it matters ("a wrong kind means wrong questions"), plain kind chips, disabled switch until a different kind is picked, "everything you already answered is kept", effect line "it re-plans its questions right away" (shot 10). On task 2 the classifier honestly fell back to the full family-decision card — options with one-line meanings, "It changes nothing else: not what gets built, not what it costs", result-not-method guidance, and the submit relabels to "This is: Write or create content" (shot 29). |
| Understood block per-item "fix this" riding the send | **VERIFIED.** Round 2's understood block is itemized with plain labels + provenance chips ("you answered" / "assumed — out loud"); "fix this" opens an inline editor with Correct it / Never mind; my correction rendered green as "changes to: …", the send button counted it ("Send: 3 answered, 1 skipped, 1 corrected"), and the corrected text arrived in the plan's POINT BY POINT as "you answered" (shots 12–15). One wart: **M5**. |
| Open points at interview standard, RA-4 dead | **VERIFIED as a surface.** The one open point was genuinely earned ("What are the six candle names, and what is the exact price for each? If you can also tell me the scent…"), held the plan honestly ("it refused to guess them; answer at least one to move"), no raw NEEDS-CLARIFICATION marker, no "Info: Nothing", free text legitimate for genuinely open data (shot 16). But the canonical currency catch did not fire — **M3**. |
| Plan card serves every field; per-step approach/decisions/ordering under Layer 2 | **VERIFIED.** Layer 1: understanding → point-by-point provenance → what-you'll-get → steps with done-when → WHAT I WILL NOT DO → assumptions-first → what-could-go-wrong → honest cost prose → WHO DOES IT in plain words. Layer 2 per step: an approach paragraph, decisions as *chosen — instead of: [alternatives] — because [reason]*, and "Why here in the order" (S-2). The full-contract drawer serves plain finish-line checks + "what it believes it must work within" (shots 17–19). |
| Field-targeted contest w/ per-target notes (r4-F4) | **VERIFIED.** "Change the plan…" puts a "not right?" on **23 fields** of this small plan — understanding, deliverable bullets, steps, approach paragraphs, not-dos, assumptions, checks, constraints. Each tap opens its own note box **with a proper accessible name** ("what should be different about: how S-1 will be done"); the general box stays; the verb counts taps ("Redraft the plan (2 tapped)") (shot 20). |
| RA-1 what-changed block after a redraft | **VERIFIED — the round's best new surface.** The fresh card LEADS with "WHAT CHANGED SINCE THE LAST DRAFT": was/now with red strikethrough and green now-text per field, added rows, and removed rows kept visible — "removed, and this page keeps it visible so nothing disappears silently". Both my per-target notes were integrated faithfully in ONE redraft (euro signs; scent-first-then-feeling), and the un-contested decisions did not regress (shots 21–22). |
| GF12 R11: draft→contest→redraft completes live, no cap crash, no confirm loop | **VERIFIED.** Contest sent 05:52, redraft served 05:55:30 (`card=approval ask=:5`), zero ERROR lines for the leg, no re-confirmation of settled facts anywhere in the journey. (The one crash of the session was a different class and viewer-triggered — M2.) |
| Accept honesty (GF10/GF11) | **VERIFIED.** Door state leads with the served reason ("open: read the card for the credit lines and the pin… this is a high-stakes act"); the card's landing statement is the GF11 sentence ("The official copy becomes a commit in **wick-wax's own store on this machine**, in your name — this project has no shared remote, so **nothing is sent anywhere**"); trailers printed exactly as written; PIN input named "Your PIN"; one submit → ACCEPTED, honest door flip ("this work is accepted: only work that is still in review can be accepted"), post-accept record with commit/effect/superseded/re-validation rows. |
| Riders | **W2** sign-in copy fixed ("These are this platform's household accounts"). **W3** Home reconciles ("0 runs paused for a person · 1 Inbox card for you" = the badge). **W9** meter phrasing fixed — past the floor it reads "**80 settled — past the 60 needed**". **W10** first paint self-labels ("STAKES: HIGH · first reading — it settles as the questions are answered/chosen") and the correction explains itself ("was high — the platform refined its reading…"). **W12** answered twice: the Done column header ("A finished task's deliverable can still be waiting on your review — its task card says so and holds the door") and the deliverable row copy. **W11/RA-15** nearly drained — see L6. **RA-6** dead: WHO DOES IT is plain words, no S08.8/D5/2.5 anywhere. **RA-4** dead as a surface. |
| Carried-over rows read-only-with-why (GF8-eval-F6) | **NOT WITNESSED** — a fresh project with an empty capture resolves no slots, so no carried-over row ever arose in this world. Not judged either way. |

## Findings — by severity

### Blocking

None.

### Medium (the fix list, in order)

- **M1 — a served card sat unpainted for 3+ minutes while the page claimed it was listening.**
  First-hand timeline: log `05:23:19 stage: intake gate open … card=clarification ask=…:3`; DOM read
  at ~05:26:30 still showed "Answers recorded — it is working … listening · 0 s"; no card ever
  painted without a manual reload. "Stale never poses as live" is a binding cross-cutting rule and
  the flagship journey broke it live. (The same live channel painted the plan card instantly at
  05:40 and 05:55 — the wedge is intermittent, which is worse for trust.)
- **M2 — reloading the page killed the working session server-side.** The reload M1 forced
  coincided to the second with `ERROR stage: intake answer died mid-drive; crashing the run for the
  recovery ladder … err="storage: begin immediate: context canceled"` (05:28:01.680). The drive
  evidently rides the page's request context, so the exact action the card invites ("You can leave;
  nothing is lost") can crash the run when taken mid-drive. The promise held in SUBSTANCE — the
  healing card is honest and plain ("The working session broke — it heals itself… Your answers are
  all kept"), the ladder forked at 05:30 and the correct plan landed at 05:40, answers intact — but
  a phone browser that reloads backgrounded tabs will hit this routinely, and each hit costs a
  crash + a ~10-minute heal. FE/wire seam: detach the drive's lifetime from the viewer's connection.
- **M3 — the canonical currency ambiguity passed silently.** I deliberately supplied six prices as
  bare numbers with no currency anywhere. No currency open point arrived (the r5 §C6 worked
  example), and no "assumed — out loud" currency assumption was listed: the drafted plan carried
  "price (8, 9, 9, 10, 8, 11 respectively)" with the currency simply unstated. Only my contest made
  it euros. The rebuilt open-points SURFACE is at the bar; the planner's gap-finding missed the
  exact case the operator named. Backend/planner content, not the card.
- **M4 — understood-item VALUES are still machine tokens (RA-7/W6, third record).** "What you want
  made — **document**", "Tone and voice — **warm**", "Length and shape — **medium**", "Pictures and
  other media — **none**" — while the labels are now plain and my own-words answers echo verbatim,
  option-backed answers render the token, not the chosen label ("A document or report", "Warm and
  personal"…). On the interview understood block, the pre-draft items, and the plan's POINT BY
  POINT alike.
- **M5 — the "fix this" editor and the assumptions list treat provenance narration as the value.**
  Tapping "fix this" on an assumed item pre-seeds the input with the WHOLE sentence "you skipped
  this one, so I am going with what was suggested on the card: …" — one edit-slip from submitting
  boilerplate as an answer. The same narration-as-value ships on the plan: the skipped question's
  assumption is listed TWICE (walkf1-W7 confirmed intact), the second copy carrying the narration
  sentence and the raw slot token — "assumed because you skipped **"references"**" (the question's
  plain label is "Examples to follow").
- **M6 — the EARS echo on every acceptance criterion (GF13-R10, FE-owned, confirmed).** Task page,
  all four ACs: "… — The price list shall display … **(written as strict when-X-then-Y requirements
  (the EARS form))**" — the jargon parenthetical ×4, plus each criterion shown twice (plain, then
  formal restatement).
- **M7 — raw ISO nanosecond timestamps on requester surfaces (W5/GF13-R11 FE-owned, confirmed).**
  "revision 1 content e0b2f3d0… · **2026-08-28T03:57:08.443328577Z**"; Parks rows
  "**2026-08-28T03:40:20.61008537Z → 2026-08-28T03:45:01.004195136Z**"; comment stamps likewise.
- **M8 — the receipts drawer still ships the citation soup verbatim (e5rw2-F2 confirmed):**
  "direct-use estimate (heuristic): no price table loaded at v0 (UNPRICED, never a silent $0 —
  S10.1) · Spec/benchmark-preregistration-v1.md §13 · aggregate switches to the measured-median
  stage per Spec/benchmark-preregistration-v1.md §13 once the benchmark machinery (S14, B5) has
  enough pairs; threshold registered there, not restated".
- **M9 — the "rendered" result view does not render markdown tables.** The review page's flagship
  view ("Shown: the delivered document (deliverable.md), **rendered**") shows the H1 rendered and
  the price table — the deliverable's entire point — as raw pipe syntax (`| Candle | Description |
  Price | |---|---|---|…`). e5rw2-F1's renderer-honesty seam, now on the simplest possible case
  (shot 28).
- **M10 — the crashed run's missing-receipt line explains the wrong cause:** "this run ended
  without a recorded receipt — nothing was itemized for it. A real run leaves its receipt when it
  ends; **a demo-seeded task can be minted finished without one**" — shown on a REAL run that
  crashed (M2); the demo-seed clause misattributes on a fresh world.
- **M11 — one card, two stakes.** The force-proceed plan's own understanding says "This is a small,
  low-stakes piece of writing … **treated as a light task, not a high-stakes one**" and an
  assumption row says the stakes critique resolved it down — while the header chip says
  "**STAKES: HIGH** — handled with extra care" and both verbs demand the PIN. The band never
  re-classified after the planner's own critique; the direction is safe (PIN demanded), but the
  card contradicts itself on the one number the chip exists to state.

### Low

- **L1** — the waiting panel's "listening · N s" readout is effectively dead: it ticked 4–5 s once,
  then read "0 s" through every subsequent wait, including the M1 wedge. Either make it mean
  something or drop it.
- **L2** — the waiting panel is state-blind: after a CONTEST it still says "it is choosing the next
  questions, or … drafting the full plan"; after force-proceed likewise. One line naming the actual
  state ("redrafting the plan with your changes") would keep the register.
- **L3** — the what-changed provenance sentence is garbled: "Compared by this page between the card
  you sent changes from and this fresh draft — both the platform's own cards, **the comparison this
  page's**."
- **L4** — one machine token on the what-changed block: "**Check AC-2** was:" (everywhere else the
  checks are plain "finish-line checks").
- **L5** — the progress rail opens with six visually identical "asking its questions · planning
  moved" rows and duplicated "cross-checking the spec" rows — noise with no round labels.
- **L6** — a11y residue (probe: every `button/input/textarea/select/a/[role=button]`, name from
  aria-label/label/text): plan surface 26/26 named, review+accept 44/44 named, sign-in and accept
  PINs named — the drain held; still placeholder-only: the interview's own-words input
  (`or say it in your own words…`) and the three New-project dialog inputs.
- **L7** — the review header still reads "Accepting makes a revision the official version — **a
  commit pushed under your own credentials**" and the technical block labels the target "**Pushes
  to** wick-wax · refs/heads/main" on a store with no remote (e5rw2-F4 confirmed) — one sentence
  later both say "nothing is sent anywhere". Cosmetic, but it is the first sentence of the page.
- **L8** — the ops-jargon watchlist card still sits verbatim in the household Inbox queue
  ("adopted organ \"watchlist\" is down (degraded mode): changedetection.io is not configured
  (SINET_CDIO_URL unset)…") — e5rw2-F3 confirmed.
- **L9** — the full-contract drawer title promises "…what you supplied" but renders no such section
  (nothing was supplied); the empty-state rule wants "you supplied nothing" said, not the section
  silently absent.
- **L10** — after the accept, the preview-compare door still says "With nothing accepted yet to
  compare against…" — stale one line below the ACCEPTED banner.
- **L11** — the cancel trigger and its confirm carry the identical label "Cancel the task" (while
  both are on the page), and when the PIN panel arms for cancel it sits BELOW the still-rendered
  verb list — two same-named controls with opposite effects plus an armed state that is easy to
  scroll past. I misread it once myself (see the task-2 tail correction); a distinct confirm label
  ("Yes — cancel it") and collapsing the verb list while armed would close it.

## The rig walls (disclosed, not judged against the product)

- **The 390px phone pass remains NOT PERFORMABLE from this rig — third session running (walkf1-W13,
  e5rw2-F6, now here).** `resize_window` reports success but the CSS viewport stays 958px
  (`window.innerWidth` 958 after 390×844 and 400×850 attempts; `outerWidth` 0 — the extension's
  display is virtualized); `window.open('…','width=390…')` is popup-blocked. I did not fake a
  narrow pass and report none. **The debt stays owed to the operator's real browser at the gate
  sitting.** Desktop-width overflow was clean on every surface crossed (no horizontal page scroll
  at 958px; wide content scrolls inside its own containers on the board).
- The known capture wedge (black band, shell displaced ~40–60%) reproduced on long-page scrolls and
  card expansions, exactly as both walk records describe; DOM and page text stayed sane throughout
  and every decisive interaction ran by element reference or the page's own input machinery
  (disclosed at each point above). A real mouse would not have this problem.
- Two synthetic-typing drops (a text append and a fresh goal) — my tooling, not the product; both
  recovered via the page's own input events.
- One find-tool hallucination caught and discarded: it "quoted" a listitem as "Rose Garden 12";
  the DOM contains "Rose Garden" zero times, "Winter Pine" seven. No product claim rests on
  find-tool paraphrases.

## What this round proves against the r5 stake

The operator's escalation stake ("considering scrapping the Sinet frontend") was about THESE
surfaces. Judged cold against the seven §C rules: (1) the interview found the real gap the prompt
hid (it refused to guess prices and asked for exactly them) ✓; (2) nothing self-resolvable was
asked — no indices, no terminology introspection ✓; (3) the system states its understanding and
takes corrections, item by item, at every stage ✓; (4) every question carried purpose, fitted
options, a starred recommendation, and an effect line — with "You choose for me and show me what
you picked" present ✓; (5) multiple choice wherever the set is finite; own-words everywhere ✓
(currency itself never surfaced — M3); (6) the open point held the plan honestly and RA-4's
pattern is dead ✓; (7) the understanding is shown, editable, and re-shown — pre-draft,
post-answers, at the plan, and diffed after the redraft ✓. The register is plain words end to end
on the flagship cards; the residue that remains is enumerated above and is thin, known, and
mostly wire-side.

## Spend and world disposition

Engine calls, all subscription-lane, UNPRICED by design, $0 cash. Platform's own receipts (task 1):
intake ceremony 1× claude-opus-4-8; execute 7 calls claude-sonnet-5; verify 1× claude-opus-4-8;
plus the second task's ceremony passes and $0 local seats. Realistic API-equivalent for the whole
review: well under $2. Control plane and broker STOPPED at review end; `~/.sinet-gf9-review`
(platform.db, control.log with the one ERROR line, rig-notes.txt, wick-wax store with the accept
commit) kept intact as evidence.

## Task-2 tail — the family card, force-proceed, the PIN ceremony, and the cancel's who/why

A second small task ("A short thank-you note to include with each candle order",
`t-cccd5dc7d00a4f64`) drove the surfaces the first journey did not:

- **The family-decision fallback card** (the classifier honestly unsure): plain option chips each
  with a one-line meaning, honest why ("it is not filed under a project that declares one, and the
  platform could not tell from the request itself"), a real effect statement ("It changes nothing
  else: not what gets built, not what it costs…"), result-not-method guidance, and a submit that
  relabels to "This is: Write or create content" (shot 29).
- **Force-proceed ("Stop the questions: go straight to the plan")** produced a plan whose
  POINT-BY-POINT opens with "**11 points run on defaults**" and whose ASSUMPTIONS list spells out
  every default with honest provenance ("assumed default — you asked to go ahead without answering
  who it's for", …). The S06.6 second arm works as a surface. The draft took ~15 minutes — slow but
  clean, no crash.
- **PIN arming witnessed live, on BOTH high-stakes verbs.** Approve → "Armed: 'Approve: start the
  work' — your PIN confirms it … Nothing has happened yet … the PIN rides that one request — it is
  never stored", named PIN field, "Confirm with PIN" (shot 30). Cancel → the same ceremony (shot
  32). Nothing was approved; the cancel was completed with the PIN.
- **The cancel's who/why, end to end (RW-19 machinery on the planning card):** "Cancel the task"
  opens the ceremony panel ("It stops here… with your why on it, if you give one", input named
  "Why cancel it (optional)", shot 31); the typed reason SURVIVED the 401→arm→confirm-with-PIN
  round trip; the task page reads "**This task was cancelled by darian — "Just testing the door -
  not needed after all.". Nothing runs on it and nothing more is spent; the record below stays**";
  the Board shows it in Backlog wearing "**CANCELLED — THE TASK SAYS WHY**" (shot 33) — the RW-19
  stale "OPEN FOR WHY" chip is gone and the D-B contract (cancelled-in-Backlog, why on the record)
  is met.
- **One correction on my own read, for the record:** my first cancel attempt looked like a silent
  failure (POST `/api/asks/…/answer` → 401, no banner). It was not silent — the 401 is the
  high-tier arm cycle and the page had raised the "Armed: your PIN confirms it" panel; I navigated
  away instead of entering the PIN. Working design, one legibility wart (L11).

## Screenshot index (`P3/design/rework-screens/gf9-review/`)

01 projects-empty-two-doors · 02 new-project-dialog · 03 onboarding-started · 04 inbox-two-cards ·
05 onboarding-card-expanded · 06 project-active · 07 describe-goal-door-pinned ·
08 task-born-stakes-first-reading · 09 interview-round1 · 10 family-chip-correction-open ·
11 round1-counting-send · 12 round2-understood-items-fixthis · 13 fixthis-inline-editor ·
14 fixthis-changes-to · 15 round2-send-counts-corrected · 16 openpoint-earned-meter-past ·
17 plan-layer1-top · 18 plan-layer2-how-drawers · 19 plan-assumptions-contract ·
20 contest-mode-who-does-it · 21 what-changed-block-diff · 22 what-changed-steps-was-now ·
23 approved-no-pin-low · 24 board-executing-s2 · 25 board-columns-scrolled ·
26 board-done-header-note · 27 task-overlay-onboard · 28 review-result-first ·
29 family-decision-card · 30 approve-armed-pin-high · 31 cancel-why-ceremony ·
32 cancel-armed-pin · 33 board-cancelled-says-why

---

**verdict: SHIP-BAR MET** — grades A- / A- / B+ / B+, 0 blocking, 11 medium (M1 stale-live wedge,
M2 reload-kills-the-drive, M3 silent currency — the new fix list; M4–M11 known-family residue +
the stakes self-contradiction), 11 low; the 390px pass rig-blocked a third time and still owed to
the operator's real browser.
