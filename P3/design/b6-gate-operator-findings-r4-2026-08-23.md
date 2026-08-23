# Operator findings — B6 gate click-through, round 4 (2026-08-23, post-GF3)

The operator tested the landed GF3 rework on their own world and reported free-text findings
(authoritative). Verdict: **the interview improved** ("questions are a bit more specific and
individual to the project"), **but the platform still cannot complete a task** — the same
verification dead end as round 3, plus plan-detail, replan-UX, board and lifecycle gaps.

**Operator instruction: these are recorded here for a FRESH session to fix — the round-3 session's
context was full. A next session starts here, at F1.**

---

## F1 — BLOCKER, REPEAT OF R3-F8: a task still cannot finish. "Fix it for good now."

Second occurrence, new project ("GPU Hardware shop for local AI developers", task "webshop for GPU
hardware"). Run parks on an inbox card reading:

> verification cannot run — verify: launch-domain deliverable without a domain check pack
> (Spec S07.8): no build, test or lint commands are captured for project "…", so there is nothing
> to check this work with — capture them for the project (its Commands), then retry
> Verification never ran — nothing was judged, nothing was delivered, and the work is parked.

Operator's own analysis is correct: **retry cannot succeed** and the card's instruction has no
door. Two independent defects, both must land:

**F1a — the spec gap (needs the queued S00.9 amendment).** `internal/verify/verify.go:229`
`ErrNoCheckPack`. S07.8 defines degraded verification only for NON-launch domains; a launch domain
with no check pack has no defined posture, so the suite-refusal ladder (RW-14 OQ1) is the only
landing. The amendment was drafted at P3-RW-14 (brief OQ3) and has sat queued ever since:
*bootstrap verification posture for command-less first tasks — V1 runs the step-contract checks
recording `UNVERIFIABLE-HERE` (ratified S07.3 vocabulary), V2 runs advisory-marked, and requester
review becomes mandatory,* so a scaffold task can verify before any command exists. **The operator
has now hit this twice and ordered it fixed; treat "Fix it for good now" as the order to execute
the amendment (S00.9 mechanics: dated changelog entry + this operator record), then implement it as
a four-stage backend packet.** Any ⚙ touched re-runs the S18 sweep.

**F1b — the missing door (no spec change needed, build immediately).** The card says "capture them
for the project (its Commands), then retry" and **nothing in the product can capture them**:
`web/src/Projects.tsx:583` only DISPLAYS the capture ("no commands captured"), there is no write
route for project commands on `/api/projects`, and the S13.7 re-scan HTTP door is on the deferred
ledger. GF2's ratified rule applies — every answerable card carries its real door. Needs: a
project Commands editor (or the rescan door) + the card linking to it.

Until F1a+F1b land, **no fresh project can complete a task** — the platform's basic function.

## F2 — Interview: better, not right yet

Operator: "a bit better… questions are more specific and individual to the project. So that
improved, but it's still not perfect. There are still some important questions missing and some
questions are unnecessary." Needs a taxonomy pass driven by real transcripts, not by the catalog's
own idea of coverage: which slots earned their question on the two real journeys (car-parts
webshop, GPU-hardware shop), which never changed the plan, and what the plan turned out to need
that was never asked. Feeds the pending v3 ratification (the governance record is already
PENDING-ratification, so a v4 revision is the same machinery).

## F3 — The plan is still a black box: assumptions and steps carry no detail

Operator's example, verbatim shape: a step "build item detail view" whose only description is
"done when clicking an item opens a view showing its name, description and currency-formatted
price". Operator: *"I don't see HOW he wants to build the detail view, what's the order — it's
still a black box. This is the point of this question here: to see exactly how he assumes it and
how he wants to build it. You're still far away from that."*

This is the round's deepest finding and it is NOT a rendering gap: the plan artifact itself does
not carry per-step method, ordering rationale, or the concrete decisions the planner made inside a
step. S06.6 requires numbered steps with a "Done when" contract and the AC coverage map; it does
not currently require the step to state HOW. The cheap preview (1.6) is supposed to let the
requester catch a wrong approach BEFORE hours burn — a step that says only what it delivers cannot
be judged for approach. Likely shape: the planner emits, per step, its chosen approach in plain
words plus the decisions it made and their alternatives, and the plan card renders them under the
step with the assumptions treatment. Determine first whether S06.6 already implies it (the
assumptions list is "the centerpiece") or whether an amendment is owed.

## F4 — Re-plan: one text box for many selected items

The GF3 multi-select landed, but there is a single note field for the whole send. Operator: *"it
should be for each item a text box, else I have to put everything in at once and it gets messy
again and could be misunderstood."* The wire already carries `contests: [{target, note}]` — a
per-target note is ALREADY in the contract (`ContestRef.Note`); this is an FE gap: render a note
box per selected chip, keep the general "anything else" box for the target-less finding.
Cheap, FE-only.

## F5 — Three engines in the planning phase (operator question: "This seems wrong to me")

Observed: Qwen3.5-9B **and** Qwen3.5-4B **and** Claude Opus 4.8, all in planning/questions.
**Coordinator answer (by design, S06.10 duty table + the D3-ratified duty map):** 4B = fast
classification (family/stakes/size, $0 local); 9B = utility seat, question phrasing + the 13.5 help
text + suggestions ($0 local); Opus 4.8 = the planning model that conducts the interview's
substance and drafts SPEC/PLAN (paid, the only heavyweight passes). Three jobs, cheapest capable
engine each, which is what keeps intake honest against direct use (S14 benchmark). **But the
operator finds it wrong, so it is theirs to rule:** either it is explained where they can see it
(the plan card's "Who does it" only names the executor, never the ceremony seats — a receipts/
transparency gap), or the duty map is simplified by operator decision (a D3 re-open, spec-legal via
the per-person ceremony duty map). Present at the resumed gate.

## F6 — Board: drag and drop does not work

Operator cannot drag cards between columns. Needs reproduction (which columns, which task states,
console errors) — the Board's landed contract may be display-only, in which case this is a feature
request against S15's board section, not a regression. Determine which before building.

## F7 — No way to delete a task (only disable) — "over time it gets messy"

Operator wants real deletion, including for a finished/Done task sitting on the board. Today
tasks can be cancelled/disabled but never removed. This crosses retention and ledger rules
(S05 ledger immutability, retention/compaction) — deletion of a task's RECORD is very likely
spec-constrained, so the honest shapes are (a) archive/hide from board with a real "gone from my
view" affordance, or (b) a genuine delete for tasks that never ran/spent, plus retention-governed
removal for the rest. Read S05 + retention before promising deletion.

## F8 — Project visible in Firefox but not in Chrome (both signed in as operator)

**Most likely NOT a browser bug: two different worlds.** The operator's own gate world runs on
:8483 and the GF3 review world on :8486; this session verified the :8486 world's home page lists
`carpart-webshop` / `release-notes` / `(no project)` and does NOT contain a GPU-hardware project,
which is consistent with the GPU project living in the :8483 gate world opened in the other
browser. **Verify first** (compare the two worlds' project lists), and only if the same world
really renders differently per browser does this become a defect.

---

## Ordered next-session sequence

1. **F1a + F1b — the blocker, first and alone.** Amendment executed (S00.9 record), backend packet
   four-stage, plus the Commands door (FE) and the card's link to it. Acceptance: a brand-new
   project's first task runs to a delivered, reviewable deliverable with no dead end. Prove it on a
   fresh world by walking it, not by tests alone.
2. **F4** (FE-only, cheap) and **F8** (verify-then-triage) alongside it.
3. **F3** — the plan-detail question: spec read first (does S06.6 already require the how?), then
   packet or amendment.
4. **F2** — taxonomy pass from the real transcripts; folds into the pending v3/v4 ratification.
5. **F6, F7** — reproduce/spec-read, then build.
6. **F5** — present at the gate; operator rules explain-vs-simplify.

## Still queued from earlier rounds (unchanged)

The second S00.9 amendment (intake spine check (e) for write-set-vs-class contradictions); the
GF3 v3 taxonomy ratification (incl. eval-F6: the `behavior` slot's options are low-content
categories); the raw-plan-editing question (operator ruling owed); the GF3-BE2 test-harness
identity packet (blocked at the permission layer — operator decision owed); the GF3 exit
gauntlet's own review/cold-walk findings when they land.
