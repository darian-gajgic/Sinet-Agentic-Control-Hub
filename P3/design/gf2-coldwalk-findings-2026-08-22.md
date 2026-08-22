# Cold-walk findings — P3-GF2 exit walk (2026-08-22)

Fresh-context walker (fable), operator-persona knowledge only, world `~/.sinet-inbox-builder`:8484 at `a97892e`. Five errands replaying the operator's round-2 walls. **All five PASS.**

| # | Errand | Result |
|---|---|---|
| 1 | The bug path: Board → intake task → answer what it waits on, end to end | PASS — answerable in 3 clicks, finished in 5 actions; card left the inbox, badge decremented |
| 2 | op's busy inbox (50+ served): tell needs-you from noise, work the queue, clear notices | PASS — triage read in seconds (queue on top, "Notices (25) — nothing here needs a decision", "Waiting on somebody else (13)"); all 6 decisions handled ~15 actions incl. a PIN deny; single notice cleared in 4 actions, a 10-notice group in 1 click; volume "workable" |
| 3 | The take-part panel: explain it in plain words | PASS — the walker restated the blind-comparison deal accurately from the panel copy alone |
| 4 | Project filter honesty | PASS — "Showing 3 of 29 — 26 hidden by the project filter… Nothing is dropped" + one-press restore |
| 5 | Sign out | PASS |

**Verdict (walker's words):** operable by a non-technical person — the question card was answerable end-to-end and left the inbox; the old 72-ticket noise is a grouped drawer that cleared ten-at-a-click.

## G-notes (frictions, none blocking — deferred ledger)

- **GF2-W1** Spec-refs still leak into card prose on several kinds ("(S14.5; §32)", "(S06.9)", "OQ4(a)", "adopted organ", "blind-pairs 0.96 vs 0.95") — the known wire-side served-copy G-family; owed a backend copy round.
- **GF2-W2** An ACKNOWLEDGED red self-check card still wears YOURS TO ANSWER with a live button and still counts in the badge. Red-stays-listed is the ratified B6-2B OQ6 semantics; the friction is presentation of the acknowledged state — needs a check whether "acknowledged" is served per card (FE fix) or unserved (wire-side note).
- **GF2-W3** The "set aside" flag verbs don't say what happens to the paused run afterward — copy touch, next builder pass.
- **GF2-W4** A fresh watchlist flag re-arrived mid-session — honest live re-raising; the deeper watch-frequency/drift-flood triage stays on the deferred ledger.
- Panel wording: "frontier subscription" and "the guess is not optional" took the walker two reads — minor copy candidates.

Builder deviation (logged at GF2): `tools/driftseed/` is a transient uncommitted flood-fixture seeder — safe to delete at leisure; never staged.
