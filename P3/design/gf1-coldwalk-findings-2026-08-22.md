# Cold-walk findings — P3-GF1 inbox rework exit walk (2026-08-22)

Fresh-context walker (fable), operator-persona knowledge only, world `~/.sinet-inbox-builder`:8484 at `a5ae161`. Five errands mirroring the operator's gate-stopping scenario. **All five PASS.**

| # | Errand | Result |
|---|---|---|
| 1 | Create a project + first work through the UI | PASS — "New project" found in 2 clicks; onboarding dialog self-explaining; interview plain-language (clearance meter, answers played back, skips become listed assumptions) |
| 2 | Find + answer YOUR project's card in the Inbox | **PASS — 2 actions, ~5 seconds** (PROJECT filter → "Showing 1 of 15 — 14 hidden"); the card explained the decision and its recommendation; answered; badge and queue reconciled |
| 3 | Identify a different project's card without opening anything technical | PASS — read straight off the list faces (project chip + task title + issue line), zero opens |
| 4 | Understand a platform-wide "(no project)" card before deciding | PASS — memory-lesson conflict decoded in plain words; knew what to check before resolving |
| 5 | Who cancelled a task and why | PASS — History own-words ask → `status.tasks_cancelled` → who/when/why + plain-words cause, ~4 actions |

**Verdict (walker's words):** the three-hours-ago abandoner would get their card answered in seconds now — the project filter collapses 15 cards to exactly theirs in one dropdown pick, and every card leads with a project chip plus a plain-words summary.

## G-notes (frictions, none blocking — deferred ledger)

- **GF1-W1** Project creation: "Remote to clone" is jargon (optional field, survivable) — copy touch, next builder pass.
- **GF1-W2** The Register button shifts position when it enables, eating a click — layout-stability polish, next builder pass.
- **GF1-W3** "STAKES: HIGH — EXTRA CARE, PIN TO APPROVE" on a birthday-dinner project reads alarmingly heavy — served tier, presentation tone only; copy/tone G-family.
- **GF1-W4** `k-fixture-…` memory-entry names read technical — seeded fixture naming, dev-world only; no action.
- **GF1-W5** The free-text History ask cannot auto-run its query (local tier not wired) so it is a two-step — known; resolves at bring-up (S19.6 local-tier wiring).

Screenshot evidence: walker shots under `/tmp/claude-chrome-screenshots-vSYASK/` (ephemeral); builder evidence `~/.sinet-inbox-builder/shots/01…17`.
