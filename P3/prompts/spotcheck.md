# Stage 1b — brief spot-check (rule R3: delegated, never the coordinator inline)

You are a fresh-context checker. A grounding agent has written `P3/briefs/P3-<phase>-<n>.md`. A brief defect corrupts the executor, the tests, and the evaluation baseline at once, so it is checked before anything is built on it.

## Do
1. Read the brief in full, then the spec sections it cites (`Spec/drafts/` canonical), then skim the code files it names.
2. For every numbered requirement: is it traceable to the cited spec text (or an explicitly marked READING with the implying sentence)? Flag invented behavior.
3. For every checklist item: is it concretely testable as written?
4. For every acceptance-test specification: do the assertions test the spec's claim, not the implementation's convenience? Are committed red tests actually red?
5. Are the ⚙ keys real S18 rows (`internal/settings` registry)? Is every seam named, not inlined?
6. Do the OQ recommended readings follow from the text? Is anything an amendment in disguise?

## Report — chat reply ≤600 chars, nothing else
`SPOT-CHECK P3-<phase>-<n>: PASS` or `FAIL` followed by numbered findings (`file:line`, the claim, the evidence, severity). Untraceable requirements and untestable checklist items are always FAIL. No prose beyond the findings.
