# WALK-F1 — fresh-world acceptance walk, 2026-08-27

**Acceptance under test** (operator record `b6-gate-operator-findings-r4-2026-08-23.md`, Ordered item 1):
*"a brand-new project's first task runs to a delivered, reviewable deliverable with no dead end. Prove it on a fresh world by walking it, not by tests alone."*

**Rig:** binary built from HEAD (`a5ae3420aac7+dirty`, `sinet v0.0.0-dev`), fresh world `~/.sinet-walkf1` on `127.0.0.1:8488` (never touched 8481-8486), broker running for the seed person, local tier wired (`SINET_LOCAL_ENDPOINT=127.0.0.1:39617`, `configured=true` in control.log), dev posture (engine spawns unconfined, sanctioned). Seed person `darian` (operator) minted through the anonymous first-user bootstrap (`POST /api/auth/users`, 201); PIN recorded in `~/.sinet-walkf1/rig-notes.txt`. Real engine calls on the operator's subscription, sanctioned by the walk order. The rig's empty repo `~/.sinet-walkf1-repo` went unused **by design**: the New-project door refuses host paths ("this door creates and initializes a fresh project store") and creates its own store under the state dir — correct behavior, noted so nobody hunts for the repo.

**Walker:** household member persona, not a programmer. Bike repair shop "Spoke & Sprocket"; first task: a single page listing my prices.

---

## Errand results

| # | Errand | Result |
|---|--------|--------|
| 1 | Onboard a brand-new project through the product | **PASS** |
| 2 | First task runs to a **delivered, reviewable deliverable** with no dead end en route | **PASS** — no parked state, no un-followable card anywhere in intake→plan→execute→verify; deliverable in-review, content **correct** |
| 3 | The checked-honestly disclosure found and understood | **PASS** — the strongest surface of the walk |
| 4 | The commands door followed, capture observed | **PASS** — captured `test -f index.html`, version 2 confirmed with an explicit next-round promise |
| 5 | Review and accept — reach a settled end | **FAIL** — the accept card renders but contains **no control that accepts**; the work cannot leave in-review |

## VERDICT

**ACCEPTANCE NOT MET** — the exact wall: **the accept flow cannot complete on a fresh project's first (bootstrap-verified) deliverable.** Everything up to and including review is now genuinely walkable — the old F1 dead end (verification refusing to run) is fixed and its replacement posture is honest and excellent — but the journey's settled end moved the dead end one station down the line, to the accept door.

### The wall, precisely (all first-hand)

- The review page (`/deliverables/dlv-t-7bc2336ad63fc226`) says: *"When it is right, accept it below and it becomes the official version."* The work IS right (verified by me line by line: Spoke & Sprocket heading, five services in my order at my prices, no images, plain, phone viewport meta — `19-review-rendered-work.jpg`, `20-rendered-list-complete.jpg`).
- Clicking **"Accept this work…"** only expands an inline consent card (*"You are accepting revision 1 — the version this page shows. The official copy is written into bike-shop-site's permanent record, in your name…"*) with a "What happens technically" details block (payload content-pin, signing posture, commit trailers preview). **No confirm control exists anywhere in the DOM** — the card's only interactive elements are the toggle button itself and the details summary (verified by exhaustive DOM enumeration). Clicking again just folds the card.
- Network evidence: pressing the button fires **only `GET /api/deliverables/…/accept-card` (200)**. No `POST /accept` is ever sent; `~/.sinet-walkf1/control.log` contains no accept attempt (only the startup wiring line).
- The page's own door-state panel (bottom, "technical detail") says: **`accept · closed`** — *"the minting run t-7bc2336ad63fc226.verify records no engine substrate or routing decision, so the attribution trailers cannot be rendered from platform facts — and an accept must never push a Co-Authored-By line naming nobody (S13.6 step 3)"* — and documents the endpoint (`POST /api/deliverables/…/accept · pin from GET …`). The accept-card JSON confirms: `"trailers":""`, `provenance.absent` = that same sentence, `pin_kind:"content"`.
- **Diagnosis for the fix session:** the F1a bootstrap path mints the revision from the **verify** run; a verify run records no engine substrate/model/lane, so S13.6's attribution invariant reports unrenderable trailers and the FE withholds the confirm (silently — there is no "why you cannot accept" line on the card itself; the reason lives in the technical-detail panel below). Either the bootstrap-minted revision must carry the executing run's attribution (the work WAS done by `claude-sonnet-5`/anthropic — the receipts on the task page know this), or accept-in-bootstrap-mode needs a defined trailer posture. Sibling panel for contrast, showing the pattern is intentional: `request-revision · closed` — *"closed for now: the work is in review and no rework card is open."*
- Consequence for the acceptance sentence: a card exists whose printed instruction (*"accept it below"*) **cannot be followed** — the "no dead end" clause fails at the last step. The task sits on the Board in **Done** while its deliverable is permanently in-review.

---

## What passed, walked blind (chronology, screenshots inline)

All files under `P3/design/rework-screens/walkf1/`.

1. **Sign-in** (`01-signin-gate.jpg`): Home is honestly gated ("Signed in, this page is the household's work at a glance… You are browsing under the developer fallback, which can look around but cannot own or approve anything"). Account picker + PIN, signed in as Darian without friction.
2. **Onboarding** (`02`–`08`): Projects empty state offers exactly two doors. The New-project dialog is plain (id / name / optional repo address / task-kind in plain words: "Build or change software", "Find something out"…). Registering confirms: store initialized, scanned, capture v1 "pending your approval", "The approval card lands in your Inbox… Answering it there activates the entry, and no other door does" — and the Inbox card delivers: family + two danger zones + "What could go wrong" + "What I recommend" (*"Approving is the only verb this card has: there is no decline, because nothing yet undoes a registration. To not onboard this project, cancel the onboarding task instead"* — honest). One click approved; project ACTIVE with a **Commands** button already on its card.
3. **Describe a goal** (`09`, `10`): *"Say what you want the way you'd tell a person… shows you its plan with a price before anything runs. Nothing spends until you approve."* Goal in my own words; lane left at "The platform chooses"; task born with a live listening counter and *"You can leave; nothing is lost."*
4. **Interview** (`11`, `12`): 4 + 4 questions, each with a why-line, options, a starred recommendation with one-click "Take this", own-words box, per-question skip, counting send. Settled rows dim with my words echoed back. **The stakes chip corrected itself with an explanation** — "STAKES: LOW — was high — the platform refined its reading of the goal as it learned more" — the r4/RA-2 lying-first-paint now discloses its correction (it does still paint HIGH first).
5. **Post-draft open points** (`13`): the plan refused to guess my prices — *"Please supply the amount for each. Until then the page cannot show real prices; placeholders would be misleading on a public price list."* A genuinely earned question the interview never asked (exactly the r4-F2 wish), plus currency confirmation. Both free-text; both legitimate as free-text.
6. **The plan** (`14`): faithful understanding, point-by-point provenance ("you answered" / "assumed — out loud"), three steps with concrete done-when contracts, **WHAT I WILL NOT DO** scope fence, assumptions list, honest cost (*"≈ USD 0 · size XS… size classification cannot be compared numerically (unpriced side) — surfaced, never silent"*), four doors (Approve / Change the plan / Change my answers / Cancel). Approved — **no PIN demanded, consistent with LOW stakes** (`15`).
7. **Execution** (`16`, `17`): Board shows Executing with step badge (S-2) and "USD 0 priced"; then Verifying; then **Done**. Needs-attention stayed empty the whole run. The Executing empty-state even answers r4-F6 in copy: *"Cards arrive here when the work reaches it — never by drag."*
8. **The review surface** (`18`–`21`) — the acceptance's heart, and it delivered:
   - **Bootstrap disclosure box**: *"Checked in bootstrap mode — your review is what decides here. When this round was checked, its project (bike-shop-site) had no captured commands, so the platform could not run a build, tests or lint on the work — the automated verdict is advisory only, and nothing counts as verified until the requester judges it."* With the door INSIDE it: **"Set the project's commands"**.
   - **Verdict #224**, linked from the revision line: *"Nothing was passed off as checked: every check rung is recorded as unverifiable here, the judge's verdict is advisory only, and your review is what decides this work. Capturing the project's commands restores the full ladder from the next revision on."* Findings carry categories (CHECK-INTEGRITY "Open — the next round will drain this", RESEARCH-NOT-RUN "recorded, not silently passed").
   - Rendered still-view of the actual page, source one click away, side-by-side diff vs the pre-task base, **Download deliverable.md (1.8 KB)**, receipts with **UNPRICED subscription accounting** (*"no dollar price exists for these calls; the receipt lists them as UNPRICED"*, 9 sonnet calls doing the work, 2 opus calls verifying).
9. **The commands door** (`22`, `23`): five one-line slots (build/test/lint/run/preview), each explained with examples, honest semantics (*"Captured here as data — nothing runs when you save; a command executes only inside the verification sandbox, on the next round"*). Captured `test -f index.html` → green: *"captured as version 2: the next verification round for a task in this project runs these commands as its check ladder…, and the advisory bootstrap marking drops from that round on…. Nothing was run just now."* **F1b is real and complete** — door exists, is linked from the failing surface, captures, and says exactly what changes.
10. **Accept** (`24`): the wall above.

## Friction list (verbatim where quoted)

- **W1 (the wall — blocker):** accept card has no confirm; see VERDICT. FE renders consent copy that promises an act it cannot perform, and the *reason* accept is withheld is only discoverable in the technical-detail door-state panel, not on the card.
- **W2 (copy, cheap):** the sign-in card on a FRESH world says *"These accounts are this demo world's seeded household"* — this is my household, not a demo.
- **W3 (counting, repeat-family):** Home's "WAITING ON YOU **0**" card captions "answered from the Inbox · **2 open cards**" (later "· 10 open cards") while the sidebar Inbox badge says 1 — three numbers, no reconciliation on the surface.
- **W4 (jargon, standing RA-6/RA-13 family, still present):** requester surfaces leak spec refs and machine tokens: "(S13.7)", "D10 is authority over what the platform DOES" (Projects footer); "(D2)", "(Spec S13.6)" and glob `**/*credential*` on the onboarding card; "WHO DOES IT" is still *"generalist-with-injected-knowledge (the default for one-offs, S08.8)… flat-rate coverage bound (D5). Research nodes route on the search-capable lane (S08.8 step 4)"*; commands capture confirmation cites "(S07.3)", "(S07.8)"; verdict note *"did-research-actually-run undecidable for S-1 [P47-1]: per-step usage counters not wired (S10 seam; B2-4)"*; timeline events read "intake.state", "gates wait (S06.1)", "(4.3)"; accept card cites "(S13.6 step 5)", "(S15.6; S01.9)".
- **W5 (timestamps):** raw ISO nanosecond timestamps sit beside every friendly time ("seen 1m ago `2026-08-27T17:44:37.944976164Z`").
- **W6 (RA-7 family, reduced):** settled interview rows echo tokens as values — "graceful", "simplest", "plain", "english" — under otherwise plain labels.
- **W7 (assumptions duplication, RA-11 family):** the four settle-myself slots appear TWICE in the plan's ASSUMPTIONS list (once as "interview:behavior" etc., once as "assumed because you skipped 'behavior'"), on top of the point-by-point section above it.
- **W8 (interview fit, r4-F2 follow-on):** round 1 still leans on catalog frames a static price list doesn't have — "What specific items does this page need to **track**… if the same item appears twice"; the ordering question presumes a search box ("by how closely they match what someone typed"); the atomicity why-line talks about "a payment taken with no order recorded". Round 2 (glitch behavior, images, look-and-feel, language) and both open points were genuinely fitted. Net: better than r4, catalog residue remains.
- **W9 (meter phrasing, RA-8 family):** past the floor the meter reads "**85 of the 60 needed**".
- **W10 (first-paint stakes, RA-2 residue):** every fresh paint still opens HIGH ("approving asks for your PIN") before correcting; the correction now explains itself, which is the improvement.
- **W11 (a11y):** the PIN input, the goal-form submit, and all five commands inputs are unnamed in the accessibility tree.
- **W12 (perception, F7-adjacent):** the task sits in Board **Done** while its deliverable is in-review; "the task's work on this is finished — what remains is not the platform's to do" explains it on the task page, but the board position alone reads as finished.
- **W13 (walker-tooling, flagged not judged):** deep scroll on the long review page repeatedly wedged THIS session's CDP capture (black band, shell displaced, synthetic clicks missing — `25`, `26`); DOM and a11y stayed sane throughout, and a route-bounce healed it. I could not distinguish a product CSS trigger from a capture-pipeline artifact; the operator's real-browser pass should confirm. After 3+ failed synthetic clicks on Accept I switched to JS-assisted clicks for that flow only (disclosed; a real mouse would not have this problem). The 390px phone pass was **not performable** — the capture surface stayed desktop-sized regardless of window resize; owed to a follow-up walk.

## Spend estimate

All engine calls rode the operator's subscription (anthropic lane) and are UNPRICED by design; the platform's own receipts: execution 9 calls on `claude-sonnet-5` (one run: 2 steps, 30,044 tokens, 33 s), verification 2 calls on `claude-opus-4-8`, plus the planning/intake opus passes and $0 local-tier seats. The plan card's own estimate: *"≈ USD 0 · ~0.00 USD (API-equivalent, D5)"*. Realistic API-equivalent for the whole walk: **on the order of $1–3**; subscription-metered, $0 cash.

## World disposition

Control plane and broker STOPPED at walk end; state dir `~/.sinet-walkf1` (DB, control.log, seed PIN note, project store) left intact as walk evidence, alongside `~/.sinet-walkf1-repo` (unused, see Rig).

---

**verdict: ACCEPTANCE NOT MET** — the wall is the accept door: on a bootstrap-verified first deliverable the accept card renders no confirm (FE withholds it; `accept · closed` — *"the minting run …verify records no engine substrate or routing decision… an accept must never push a Co-Authored-By line naming nobody (S13.6 step 3)"*), so the first task's correct, reviewable work can never reach its settled, accepted end. Every prior station — onboarding, interview, plan, execution, bootstrap verification, honesty disclosure, commands door — passed the walk.
