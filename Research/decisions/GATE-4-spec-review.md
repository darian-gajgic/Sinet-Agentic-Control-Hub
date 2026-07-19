# GATE-4 — Spec review (freeze v1, end the research phase)

**Opened:** 2026-07-19 · **Wave covered:** P2 (core-architecture spec) · **Status:** OPEN
**Review object:** `Spec/core-architecture-v1.md` @ commit `f3694a9` (3,816 lines; deterministic concatenation of the 20 committed drafts `Spec/drafts/S00…S19`, drafts canonical) + binding siblings `Spec/benchmark-preregistration-v1.md` (signed `5fb7082`; untouched — its numbers change only via its own §17) and `Spec/frontend-components-v1.md`.

## Findings digest

### Assembly & integrity
- The assembled file is **provably the complete concatenation** of the 20 committed drafts: a fresh re-derivation hashes byte-identical (sha256 `d156e6a2…`); 3,816 lines = 3,797 draft lines + 19 seam newlines; S01–S19 headings each once, in order; one document title; balanced code fences. No partial/earlier version ever existed (the file was born at `f3694a9`, this session).
- Draft completeness (crash audit, operator-requested): every numbered section carries all five mandatory closing blocks and terminates with its complete "Open items for G4" line; the one draft whose agent died mid-flight (S15) flushed complete to disk and passed full-input coordinator review before commit.

### Document state
- **Zero open design questions.** All 19 numbered sections' "Open items for G4" read **none**; zero quality flags anywhere.
- **S17 — Known-problems register:** all **64 P-\* ids** filed across G1/G2/G3 (29+28+7) are registered with owner, one-line disposition, and standing-machinery registrations; **zero orphans**. Four register findings (R1–R4: one id restored, two ownership adoptions, one dedup) were resolved by the only reading consistent with the ratified record and back-filled into the owning sections.
- **S18 — Settings registry:** **118 operator-editable keys across 33 domains**, provably exhaustive over S01–S16 (per-owner counts sum); G1 rider 1 (every number a bounded, audited setting) is mechanical corpus-wide. Sweep executed 10 reconciliations (R1–R10), notably: SLA cadences keep `verification.*` names; `intake.approval_stale_hours` folded into `freshness.max_age` (alias recorded, split path parked with named re-entry). Four dormant-at-v0 keys ship with ratified defaults.
- **S19 — Boundary, coverage, build order:** **166 of 182** feature-list items carry a direct coverage row; the **9 items with no row** are dispositioned as covered-in-substance (table in S19.2); the remainder are items the feature list itself phases beyond v0 (S19.3). All **KP-1..14** known problems map to owning countermeasures. The consolidated parked register holds **104 entries, none lacking an owner or re-entry trigger**. P3 gets a walking-skeleton build order **B0–B6** and a sequenced 13-measurement bring-up battery.

### What G4 is actually confirming (S19.7)
- **49 `[coordinator-draft]` judgment calls** enumerated across the section Open-items lines — settings defaults and clamp ranges, naming choices, a few clarifying readings, interlock refinements, and the register/sweep resolutions above. (Raw tag occurrences: 153 — the excess is the same calls mirrored in S18's registry rows and S17's register.) Each is asserted by its section, and verified by the S17/S18/S19 sweeps, to be **the option directly implied by already-ratified decisions** — none reopens a gate decision or a D-constraint.
- **Four sections resolved a tension inline** instead of "none": **S02** workspace storage — *operator-decided 2026-07-18* (git-worktree + overlayfs; loopback-XFS pre-registered upgrade); **S03** native micro-fanout — *operator-decided 2026-07-18* (keep disabled; re-entry precondition recorded); **S07** rework cap = 3 — coordinator-resolved from the ratified text (G1 D1.1(d) ratifies R04 §4, whose rule reads "default 3, config"); **S10** — two recorded supersessions the gates already made (genai-prices seed per G2 Def.16; R16's TTL-warm shape over R09's resident-slot sketch).
- The two genuinely contested calls in that set were therefore **already decided by the operator during drafting**; nothing in this gate re-asks them.

## Decisions required

### DECISION 4.1 — Ratify `core-architecture-v1.md` as the binding core-architecture spec v1
- **Context:** the G4 agenda is confirmation, not open design (S19.7). Ratification bulk-approves the 49 coordinator-draft judgment calls, the S17 R1–R4 and S18 R1–R10 sweep resolutions, the 9 coverage dispositions, and the GitHub-Pro parked-list reconciliation (S19.4). Tags stay in the text as provenance; this gate record is what marks them ratified.
- **Options:** A) Ratify as-is. B) Ratify with named amendments — you name changes (any flag can be pulled out and altered); coordinator applies them to the owning drafts, re-runs the S18 re-sweep if a ⚙ is touched, reassembles deterministically, commits, freeze follows. C) Targeted walkthrough first — plain-language tour of any section(s) or the 49-flag list before deciding.
- **Recommendation:** **A** — every flagged call is the option the ratified record already implies, three independent sweeps (S17/S18/S19) found the corpus consistent and complete, and zero open items remain.
- **Forecloses:** nothing permanently under any option — post-freeze changes use S00.9 amendment mechanics (dated changelog + operator approval). B/C only add a round-trip.

### DECISION 4.2 — End the research phase (the operator's explicit act; contingent on 4.1)
- **Context:** per CLAUDE.md, G3 D3.1, and S00.9, only this explicit operator act ends the research phase. Approval freezes **v1** and hands P3 the S19.5 build order (B0 spine → B6 frontend) plus the S19.6 bring-up sequence. Remaining unknowns are, by construction, bring-up measurements, P3 spikes, or operator hands-on items — nothing researchable is left.
- **Options:** A) End research now — P3 implementation becomes permitted. B) Ratify the spec (4.1) but hold phase-end (e.g., you want to read the assembled document at leisure first; it stays frozen meanwhile).
- **Recommendation:** **A** (given 4.1) — the campaign's exit criterion is met in full.
- **Forecloses:** nothing; B only delays the start of P3.

### Defaults unless objected (adopted silently at gate close if unflagged)
1. **Tag handling:** `[coordinator-draft]` markers remain in the text as provenance; this gate record is the ratification instrument (no 20-file sweep).
2. **S18 R2 fold stands:** one staleness key (`freshness.max_age`); the split into two independently-tunable thresholds re-enters only on operator demand post-bring-up.
3. **Amendment re-sweep rule:** any future section amendment touching a ⚙ table re-runs the S18 sweep as part of the S00.9 step.
4. **Post-gate housekeeping (on 4.2 = A):** coordinator updates CLAUDE.md's phase flag and closes out `Research/STATE.md` + campaign memory files; standing operator action items (Z.AI prompt-unit calibration, ts.net hostname before first cert, deferred host probes, week-one push drill, optional GitHub badge) carry into P3 at their S19-named slots — none blocks this gate.

## Decisions taken (filled at close)

| # | Decision | Chosen | By | Date | Notes |
|---|---|---|---|---|---|

## Follow-ups spawned

- (filled at close)
