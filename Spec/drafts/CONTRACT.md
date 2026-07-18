# Spec drafting contract (P2 per-section workshops)

Binding instructions for every section draft of `Spec/core-architecture-v1.md`. The coordinator enforces this contract at review; a non-conforming draft gets one bounded revision.

## What is being written

- The **binding core-architecture spec for Sinet v0** (feature list §15.1 scope; §15.6 data-model rule). Audience: (a) the operator at gate G4; (b) P3 implementation sessions that will build from this document with no other context. Write for them.
- Research is DONE. Reports 01–17 are the evidence base; gates G1–G3 ratified the decisions. A section drafter does **no new research**: no web access, no model-memory facts. Every load-bearing statement is a ratified decision, a report finding, or a feature-list requirement — carried by a provenance tag.
- Research-phase rule: **no application code.** Interface sketches, schema tables, unit-file field lists, state machines, and contract pseudocode are spec content and welcome; runnable code is not.
- Adopt-don't-fork and no-load-bearing-metered-paths bind every line. The subscription-coverage rule (Operating reality) binds every routing statement.

## Binding layers (highest wins)

1. **D1–D10 + the feature list** (`Docs/agent-platform-feature-list-v1.md`) — the capability contract. Never contradicted, never re-derived.
2. **Gate records** `Research/decisions/GATE-{1,2,3}-*.md` — numbered decisions, defaults, operator riders. A gate default is as binding as a numbered decision.
3. **Signed/binding Spec siblings:** `Spec/benchmark-preregistration-v1.md` (amend only via its §17 — cite it, never restate its registered numbers as if they were this spec's to change) and `Spec/frontend-components-v1.md`.
4. **Report recommendations as ratified** (cite report §).
5. **Spike results** (G1-S1..S3, P2-S1..S5; conclusion headlines live in `Research/STATE.md`).

Conflict between layers → the higher layer wins in your text AND the tension is recorded under *Open items for G4* with both sources named. Never resolve silently; never bend D1–D10.

## Section shape (uniform, in this order)

```
## S## — <Title>
**Scope:** <one line>
**Binding inputs:** <reports / gate items / spikes / siblings consumed>

### S##.1 <subsection>   (the normative body)
…

**Settings introduced (⚙):**  table: name | default | clamp/range | ratified by
**Known problems owned here:** P-* ids, one-line disposition each
**Deferred / parked:** item → re-entry trigger
**Coverage:** feature-list item → subsection map, for everything in Scope
**Open items for G4:** (aim for zero; genuinely-open only, each with a recommendation)
```

## Style

- Declarative present tense; decisions stated as facts of the design ("The control plane is the sole writer of `platform.db`"), each load-bearing one tagged: `[R08 §4; G2 D2.1]`.
- No research narration and no options-weighing in the body. Where a genuine unratified sub-choice exists: adopt it only if one option is clearly implied by ratified decisions, tagging `[coordinator-draft]` for G4 attention; otherwise put it under *Open items* with a recommendation.
- Terminology: use the S00 glossary names exactly; never invent synonyms (no "sub-agent" for helper, no "sandbox profile" for confinement class). A genuinely new term your section must coin: define it on first use and flag it in your final message.
- Notation:
  - ⚙ settings: `⚙ domain.name = default` (dotted names within your section's domain; every ⚙ number from the gates ships as an operator-editable setting with audit trail — G1 rider 1).
  - Measurement-pending values: `TBD-BRINGUP(<measurement>)` · implementation-phase spikes: `TBD-P3(<spike>)` · operator hands-on items: `TBD-OPERATOR(<item>)`.
  - Cross-section references: `[XREF:S##]` — use them instead of restating another section's material.
- Length: 150–400 lines. Dense beats long: a P3 session must be able to hold several sections at once.

## Security-content isolation (campaign rule; applies unless your assignment lifts it)

Do **not** open `Research/09-*`, `Research/10-*`, or `Research/spikes/*`. The facts from those live at conclusion level in `Research/STATE.md` and in the gate memos — cite those, and leave `[XREF:S10]` / `[XREF:S11]` markers where metering/sandboxing detail belongs. The assignments for S02, S03, S10, S11 explicitly lift this rule for their named inputs only.

## Output

- Write exactly **one** file: `Spec/drafts/S##-<slug>.md`. Create nothing else; edit nothing else.
- Final message (≤12 lines, no section text pasted): decisions drafted (headline list) · ⚙ settings introduced · P-* owned · TBD-*/XREF markers left · Open items for G4 · anything the contract forced you to leave out.
