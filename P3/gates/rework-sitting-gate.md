# Interview-rework sitting — gate record (opened 2026-09-16, re-presented 2026-09-17)

**Status: PARTIALLY ANSWERED 2026-09-17 (A1/A2/A5/B7/B8/B10/B11 closed; A3/A4/A6/B9 open pending the coordinator's explanations — see §Answers).** Operator free-text answers are authoritative (standing convention); the coordinator records them under §Answers + in `P3/STATE.md`, executes, and closes. The sitting of 2026-09-16 was interrupted by a host reboot after the six standing items were presented in chat and before any answer landed; this file is the durable re-presentation plus the sitting's own findings, triaged.

**What the operator sees when reading this:** Part A = six decisions that were already due at the planning-rework exit gate. Part B = five new decisions produced by the sitting's webshop probe. Part C = every finding from the sitting mapped to a packet (nothing is dropped). Part D = the order the coordinator recommends, and which packets need no answer to start.

---

## Part A — the six standing items (presented 2026-09-16, unanswered)

### A1. Ratify taxonomy v4 (the software interview's question set; P3-GF7, landed 2026-08-27)

The interview's question set for software tasks was rebuilt from the operator's three real transcripts and the live benchmark walk. It ships governed under its own supersession from first boot (S09.8) with ratification recorded PENDING; a refusal here executes as a supersession back to v3.

What v4 is, in one table (weights verbatim from the measured v3 where the slot survived):

| Slot | Asked as | Weight | Posture |
|---|---|---|---|
| collection_semantics | The things it keeps track of | 12 | asked |
| comparison_rules | The order things are listed in | 12 | asked (option referents clarified) |
| ordering_atomicity | Steps that must not be left half-done | 12 | asked (system-decides recommended) |
| technology_stack | Technology choice | 11 | asked |
| edge_cases | When something unexpected happens | 10 | asked |
| assets_media | Pictures and other media | 10 | asked (the bar every slot was held to) |
| look_feel | Look and feel | 10 | asked |
| **language_locale** | The language it speaks | **9 (NEW)** | asked — every seeded string is written in the language named; changing it later rewrites all of them |
| output_format | How you get it | 8 | asked (intelligibility fixed) |
| **quality_bar** | What finished has to pass | **8 (NEW)** | asked — decides what verification gates on |
| units | Units and measurements | 6 | asked (why-line fixed) |
| behavior | What it should do | 10 | **never asked** — inverted: the platform states its understanding, the requester corrects it |
| terminology | Words that mean something specific | 10 | **never asked** — same inversion |
| indices_ranges | Counting and ranges | 8 | **never asked** — settled internally, disclosed as an assumption |
| numerical_precision | Rounding | 6 | **never asked** (OQ1: ruled with the killed slots by the same reasoning) |

Every asked slot meets the r5 §C bar: a plain purpose saying what breaks if unanswered, 2–4 concrete options a non-programmer can pick between, exactly one recommended default, an effect line per option. Provenance is in code (`gf7Provenance`, model + date).

**Ask:** ratify v4 as shipped (incl. OQ1 and the two new slots), or name what to change.

### A2. The quality_bar floor call (the one item with LIVE evidence now)

At the unchanged Clearance floors, a standard-tier interview stops before it reaches `output_format`, `quality_bar` and `units` (22 weight rides into the planner's listed assumptions — S06.5's design, the GF7 executor's gate note). The Nexus harvest recorded `quality_bar` as the one verdict-changing question. The sitting's webshop task then FAILED on exactly the axis nobody asked about: `look_feel` was answered "modern with basic animations", `quality_bar` was never asked, and the product shipped random placeholder photos that a human judges first (TQ-F5).

Options:

- **(a) Raise `quality_bar`'s weight 8 → 12 (RECOMMENDED).** Weights are taxonomy DATA the spec makes operator-editable (S06.5); the slot joins the first card at standard. No ⚙ key moves, no S18 sweep, no amendment; lands as a v4.1 supersession through the same knowledge gate. The v4 record's own rule was "never OUTRANKING the measured 12s" — 12 equals, it does not outrank.
- (b) Raise `intake.clearance_floor.standard` (⚙, default 75). Asks more of everything; a ⚙ default move → S18 sweep.
- (c) Leave it — quality stays an assumption at standard.

### A3. Amendment A14 — veto window

Applied 2026-08-27 on the operator's explicit "fix it for good" order (r4 §F1a). S07.8 gained the **bootstrap verification posture**: a software task whose project has no captured build/test/lint commands is never a verification refusal; the ladder rungs that would need the missing commands record UNVERIFIABLE-HERE (never a silent skip, never PASS), the judge pass runs advisory and visibly marked so, requester review is mandatory at every stakes tier, the card and receipt say so in plain words, and capturing commands later restores the full ladder with no residue. No ⚙ moved (118/33). **Ask:** veto or let stand. (Item B8 proposes its successor A16 — the posture as applied was correct for what nobody knew, and wrong for what the task itself created; see there.)

### A4. Amendment A15 — veto window

Applied 2026-08-27 on the operator's order (r4-F3, r5 §C rule 7: "I don't see HOW he wants to build the detail view"). S06.6's PLAN gained a required **per-step approach** (plain words: the method, the material decisions with alternatives and why the winner won, ordering rationale where load-bearing) and Layer 2 renders it under each step. Verification still binds to the frozen ACs and Done-when contracts only; the approach informs the human judgment. No ⚙ moved. **Ask:** veto or let stand.

### A5. The `decision.emission` card kind + two structural constants (P3-GF12)

The drafting/revising seat routinely exceeded the 1200-rune approach cap (11 over-cap refusals witnessed live), and the old path was refusal → crash → fork → tombstone with zero new information. Landed: the prompt now states the bound; a contract-invalid emission gets a bounded re-emission with validation feedback (`emissionRetryLimit = 2`, extending the §60 never-repair rule — feedback, never repair); exhausted → a SERVED card of the new kind `decision.emission` on a PARKED run (re-drive the same operation, or contest). Same packet: the content-family planner re-confirming settled facts is bounded by `clarificationRoundLimit = 2` (leftovers become listed assumptions — S06.6's own second arm). `approachMaxRunes` stays 1200. Both constants are structural code constants → the standing settings-tab clamped-⚙ ledger.

**Ask:** ratify the card kind and the two constants (or move either number).

### A6. Recorded readings — en bloc (B0–B5 precedent)

Coordinator readings taken during the planning-rework campaign, each "clearly implied by the text", none an amendment:

- GF4 OQ1: a software task with NO project at all stays a refusal card (A14's scope is "whose PROJECT has no captured pack"; the register+attach door exists). GF4 OQ2: no amendment beyond A15 (S06.9 is structure, not enumeration).
- GF10: never-sign-as-nobody (S13.6) satisfied by truth — the minted revision carries the EXECUTING run's substrate/lane/model, never bypassed.
- GF11 S-6: for a remote-less project store the local commit IS the official landing; push = the outward arm's transport (extends the RW-17 two-arms cut). OQ1–OQ5 vocabulary (`landing: local`, `remote-push`/`local-store`/`decision-record`, derive-from-log, attach/detach out of scope).
- GF12: S06.4 bounds ceremony over a working pipeline, never honesty — a refused emission has nothing to auto-approve, so band tasks get the card too; §3(e) argued from S06.5/S06.6/S06.10 → no amendment.
- GF14 §8a: (1) the §56 drive-attachment bullet REVERSED (the drive outlives the viewer's request); (2) the currency backstop is a pair-acceptance validator riding S06.5/S06.6's marker machinery — the S06.7 spine stays closed at (a)–(d); (3) R4.1 abstain-settle: fail-closed is a posture, floors clamp, band never re-entered, one shot.
- GF15 deviations (a) four-files, (b) same-site comment — accepted; the boot-re-drive posture disclosure (a boot-re-driven approved accept with the broker down now terminally fails, loud and recorded).
- GF13: the per-string keep ledger `P3/briefs/P3-GF13-keep-ledger.md` (7 keep-with-reason families) stays LIVE.
- W1: all 23 harvest verdicts (12 fit / 8 with modification / 3 reject) ratified by coordinator triage; no contested item.

**Ask:** ratify en bloc, or pull any one out.

---

## Part B — new items from the sitting (2026-09-16)

### B7. The model-routing directive (TQ-F9) — ratify the specifics

The order (chat, 2026-09-16, authoritative): retire `claude-sonnet-5` and `claude-opus-4-8` from the task pipeline; use Kimi K3 and Opus 5. The spec names duty CLASSES, not models (S06.10 "planning model", S07.5 "planning-model class on a paid flat-rate lane"); the ratification objects are the B3 gate D3 record and the code's provenance — so this is a gate ratification that supersedes D3's seat mix, not an S00.9 row (one can be added as bookkeeping on request). What the packet (P3-TQ-5) will do, for the operator to confirm or change:

1. Planning seat → `claude-opus-5` on `anthropic`. Judge seat → `claude-opus-5` on `anthropic`.
2. Execution seat → **K3 on the Kimi lane** as the configured first choice, `claude-opus-5` on `anthropic` as the ordered alternate. **Sub-choice:** first choice `kimi` (API via opencode) or `kimi-cli`? Both share one pool; the LN sitting's A/B recipe is what answers "which performs best" — proposal: `kimi` first, `kimi-cli` alternate, swapped by data edit after the A/B.
3. **Until a Kimi key is placed, execution resolves to the alternate = Opus 5, and the judge is ALSO Opus 5.** S07.5 permits same-family judging only with the flag on every receipt — so every interim receipt will carry the self-family-judging flag. This is why the LN key ceremony became top priority: it is the only thing that ends the interim.
4. P-T06-5 is a hard gate: a judge-model change re-runs the 26-case golden set (~52 judge calls) on Opus 5 BEFORE unsupervised judging resumes → `rubric-software` v3 with `JudgePin` = Opus 5, TPR/TNR + length bias re-measured, the eval floors re-derived. On the subscription lane this is $0 cash (API-equivalent tracked, pre-registered stop line as in 2026-07-22). The serialize-by-deny E3 leg re-runs on the new executor seat (the D3 rider, cheap).
5. Judge ≥ executor across vendors (Opus 5 judging K3 output) is recorded as a judgment call, not measured — the honest statement.
6. Model ids live-verified at implementation time (Anthropic's current model list; the Kimi account's observed list settles `k3` per P-T17-3 — the seed id is documented-primary, not yet observed on this account).

**Ask:** confirm 1–6 (or change the sub-choice in 2 / the interim posture in 3).

### B8. Amendment proposal **A16** — execution at bootstrap (TQ-F3 + TQ-F4 + TQ-F7's platform half)

**The finding (TQ-F3, the headline):** the webshop's whole verification was two one-shot judge reads — no build, no tests, no browser — although the plan wrote machine-walkable given/when/then lines per AC and the executor's own scaffold declared build and test commands. That was A14 behaving exactly as written: the project had no captured commands, so every rung recorded UNVERIFIABLE-HERE. A14 is right for a project nobody knows; it is wrong for a task that CREATES the project's build system.

**Code fact found 2026-09-17 (TQ-F4's mechanism):** `bootstrapV1` marks EVERY plan step's Done-when contract UNVERIFIABLE-HERE, attributed to the absent pack. A14's text says that only for "executable-ladder rungs that would need the missing commands" — a Done-when such as "representative photos downloaded into public/images/**" needs no command; it is decidable from the tree. That over-application is a conformance defect (packet P3-TQ-3), and A16 restates the clause so the packet has an unambiguous anchor.

**Draft changelog row (not applied until approved):**

> | A16 | (date of approval) | **S07.8's bootstrap posture gains the platform's own execution rungs for a command-less launch-domain task, and S13.7's capture may be refreshed from the task's own tree — on the operator's 2026-09-16 FAIL verdict (`P3/design/taskquality-webshop-findings-2026-09-16.md` TQ-F3/TQ-F4/TQ-F7).** Before this entry the posture's only landing was "every rung UNVERIFIABLE-HERE, V2 advisory, V3 mandatory" — the platform had no rung of its own to run, and the registry capture (S13.7) could only be filled by hand, so a task that created the project's build system was verified by two judge reads and nothing else. **What changes:** (1) at the execute→verify boundary of a bootstrap-posture round, the platform RESCANS the produced tree with the S13.7 onboarding-scan heuristics and enters detected build/test/lint/dev commands into the registry capture with provenance `detected` — requester-visible and editable through the Commands door, a hand-captured command always outranking a detected one; the ladder then runs them in the verification sandbox as EVIDENCE rungs (executor-authored tests are evidence under S07.3 rule 4, never the AC verdict). (2) For web launch-domain deliverables, S07.3's "drive-the-feature e2e" rung is a PLATFORM-AUTHORED walk of the frozen given/when/then acceptance lines: the deliverable is served by its detected dev command inside the C1 verification sandbox (empty netns — server and headless browser both inside it, no egress, no host routing) and every AC is driven and asserted on DOM state, never on rendered pixels; a walk step the platform cannot compose records UNVERIFIABLE-HERE with its reason, never PASS. (3) Restated as A14 already means it: only rungs that need missing commands record UNVERIFIABLE-HERE — every Done-when contract decidable from the tree (write-set globs, named files, structural facts) is DECIDED at V1 by a deterministic or fresh-context model read of the tree, never from the executor's report. **What does NOT change:** the S07.3 ladder, its seven MUST rules and the graduation rule; V2's advisory marking and V3's mandatory review at bootstrap stay until a hand-captured pack exists (detected commands do not graduate a project); the judge's input slice (S07.5) is untouched — plan fidelity is V1's, not the judge's; the S13.8 preview and its S11.4 host substrate are a separate consumer of the same launch/probe code and stay on their own row. No ⚙ default or clamp moves → **no S18 re-sweep** (tally 118/33); at most two structural constants (walk step timeout, per-AC walk cap) → the settings-tab ledger. Marker sites: the S07.8 bootstrap bullet, S07.3's e2e line, S13.7's capture paragraph. | operator, 2026-09-16 FAIL verdict + "include that too in the fix"; presented 2026-09-17 |

The executor-side quality rule from TQ-F7 (generated frontends: correctness feedback visible on the first frame, no `mode="wait"` view gating, respect prefers-reduced-motion) is worker-template/rubric CONTENT (S08 playbook + the S07.10 rubric, knowledge objects through the 8.3 gate) — it rides P3-TQ-4, no spec text needed.

**Ask:** approve A16 as drafted (or amend), which opens P3-TQ-4.

### B9. Promote the preview substrate now? (SIT-F3 — a build-order call, not an amendment)

The operator's finding: for web work the missing live preview is accept-blocking in practice ("I cannot test before approving"), and the honest-absence copy fails the walker. The spec already specifies the preview fully (S13.8); what is deferred is the host substrate (S11.4 nftables default-drop unit + S11.8 privileged unit, socket-proxyd on-demand/idle-stop lifecycle, Caddy admin-API routes per preview) and the spawn/probe/route activation code in `internal/preview`. The dual-iframe compare surface exists (§45). `P3/gates/try-deliverable.sh` is the stopgap today.

Options: **(a) promote now (RECOMMENDED)** — P3-SIT-3: two backend packets + one hands-on host session under the operator's safety gates (propose → approve → apply; nothing touches the NVIDIA/kernel/boot path); A16's in-sandbox dev server shares the launch/probe code, so the two land cheaper together. (b) keep it deferred; fix only the copy (rides P3-SIT-2) and keep the script as the door.

### B10. The webshop task `t-3120e8e3d14591d3` — accept or deny with a reason

Confirmed at HEAD of the sitting world (read-only copy, 2026-09-17): deliverable `dlv-t-3120e8e3d14591d3`, type `markdown`, state **in-review**, revision 1, snapshot `083b174`, produced by `execute.g1`. The operator's verdict on the product was FAIL. Recommendation: **deny with a reason** through the platform (not by leaving it) — it exercises the deny path and the reason lands as walk data. Suggested reason text, in the operator's own words or this: "Placeholder photos instead of the representative product photos the plan promised; the shop reads thin and unfinished; nothing was actually run before it reached me."

### B12. Engine pin drift (found 2026-09-17 by the TQ-1 grounding): lock pins claude CLI 2.1.218, installed is 2.1.274

The claude CLI auto-updates on this host; `components.lock` and `claudecli.Pin` still say 2.1.218 (the B4-gate bump). The sitting's webshop ran on 2.1.273; today's binary is 2.1.274. Per CONVENTIONS §10 this is reported loudly, never silently retargeted; the standing rule from RW-16 is that the next S03.3 bump re-runs the RW-9 T1–T4 brace-short tripwire AND checks OQ-e (whether the engine bump changed the brace-short emission), plus the tier-R conformance check (installed matches pin). The two P3-TQ-5 measurement riders run on the installed binary either way and record its version.

**Ask:** approve an S03.3 pin bump to the installed version (a light packet: lock + `claudecli.Pin` in lockstep, tier-R conformance run, the RW-9 T1–T4 tripwire, the OQ-e check, P-T14-1 worker-revalidation against the production worker set). Recommendation: yes, right after P3-TQ-5 lands — every packet since B4 has been tested against a pin the host no longer runs.

### B11. One open question for the operator

Which browser produced the original dead-click experience on the webshop — **Brave or Chrome**? The coordinator reproduced a frame-starved tab in Chrome (TQ-F7); the host investigation (memory `chromium-frame-starvation`) needs to know whether both are affected.

---

## Part C — the triage: every sitting finding → its disposition and packet

| Finding | Root cause (verified) | Disposition | Packet |
|---|---|---|---|
| **TQ-F1** snapshot race crashed a healthy run | S02.4d snapshot `git add` races the engine's vanishing `*.tmp.<pid>.<hash>` files → exit 128 → stage error → run crash ($1.19 discarded) | conformance defect, no amendment | **P3-TQ-1** backend, small |
| **TQ-F2** whole-plan re-drive on a dirty worktree | S02.5 ladder says DEAD → "fork-from-last-checkpoint"; the fork restarted at S-1 over S-1..S-4's output despite an S-4 checkpoint | conformance defect, no amendment | **P3-TQ-2** backend, medium (resume at the last good stage; worktree reset to that snapshot first) |
| **TQ-F4** plan fidelity unchecked | `bootstrapV1` marks every Done-when contract UNVERIFIABLE-HERE (A14 says only command-needing rungs) | conformance defect; A16 (3) restates | **P3-TQ-3** backend |
| **TQ-F3** verification executes nothing; **TQ-F7** platform half | spec gap: no platform-owned rung at bootstrap; capture never refreshed from the task's tree; no DOM-asserting AC walk | amendment **A16** (B8) | **P3-TQ-4** backend, large (after A16) |
| **TQ-F5** quality_bar unasked | floor/weight | gate item A2 | data micro (v4.1) after A2 |
| **TQ-F6** method not tier | informational | none | — |
| **TQ-F7** host half | Chromium frame starvation on this host | outside Sinet; standalone host investigation | — (memory `chromium-frame-starvation`) |
| **TQ-F8** run surface never narrates crash→fork | copy/narrative | small | rides **P3-SIT-2** (FE) + one connective backend line in P3-TQ-2 |
| **TQ-F9** model directive | operator order | gate item B7 | **P3-TQ-5** backend data + rubric v3 + P-T06-5 re-run rider |
| **SIT-F1** code deliverable shows only the report; **SIT-F2** no revision diff | S13.1/S13.2: a repo-backed deliverable is minted as `markdown` with ONE content object (the step report); the tree diff between snapshot pins (rev-1 vs pre-task base, N vs N−1) is never served | conformance defect, no amendment | **P3-SIT-1** backend (mint code work as `code` pinned to the snapshot; serve file inventory + per-file host-side unified diff from the platform store refs; the report becomes a companion) |
| **SIT-F1/F2/F4** the surface; **SIT-F3** copy; **TQ-F8** copy | presentation | FRONTEND.md | **P3-SIT-2** frontend (file inventory, per-file diff, revision navigation, the report demoted to "what the worker says it did", honest preview-absence copy, crash→fork narrative) |
| **SIT-F3** live preview deferred | S11.4 substrate + activation code | gate item B9 | **P3-SIT-3** backend ×2 + host session (if B9 = a) |

### Deferred-ledger adds from the P3-TQ-1 grounding (§9, coordinator-triaged 2026-09-17)

- **O1** a paid call's usage event is lost when the per-call checkpoint fails (`driver.go:339-355`: snapshot runs before the WriteTx; on error neither the D7 row nor the `engine.usage` event lands) — a metering-honesty gap; candidate follow-up with the stage-endings question below.
- **Stage-endings reading (TQ-1 §2(4)):** S02.3/S02.5 imply a platform bookkeeping failure on a COMPLETED engine session is not a crash cause (FINISHED-DURING-OUTAGE harvests, never redoes); today it lands as `stage.finished outcome=error` → `crashed` → fork. TQ-1 removes the transient trigger; the residual redesign (engine outcome on `stage.finished` + a platform checkpoint-failure event) is a candidate packet, pairing with TQ-F8's run-surface honesty and P3-TQ-2.
- **O2** the stage-close snapshot failure is swallowed into a Warn line (`skeleton.go:781-785`); the verify mint then reads a stale HEAD with no durable record — a silent degrade (§14-adjacent).
- **O3** `commit`/`diff --cached` may meet `index.lock` contention from the engine's own git use (also exit 128); unevidenced, no retry specified.
- **O5** every false crash consumed one of ⚙ `recovery.max_attempts` (lineage-cumulative); other platform-side persistence failures on a live session still do.
- **Settings-tab ledger:** `snapshotAddAttempts = 3` (structural constant, TQ-1 R5).

## Part D — recommended order (and what needs no answer)

0. **The LN key-ceremony sitting** (operator hands-on, one command `./P3/gates/lane-test-door.sh`, two key pastes, then the A/B clicks) — top priority since TQ-F9: nothing else ends the self-family-judging interim. The LN gate batch (items 1–15, unchanged) is presented at that sitting.
1. Answers to Parts A–B (free text, in any order).
2. **P3-TQ-5** rerouting — first, so every later probe runs on the directed models.
3. **P3-TQ-1 → P3-TQ-2 → P3-TQ-3** — cheap correctness; **need no gate answer** (conformance defects), can start on "go".
4. **P3-SIT-1 → P3-SIT-2** — the operator's MAJOR (code in the deliverable); SIT-1 needs no gate answer.
5. **P3-TQ-4** — the verification rung (after A16).
6. **P3-SIT-3** — the preview substrate (after B9 = a; host session scheduled with the operator).

## Answers (operator, chat, 2026-09-17 — verbatim, authoritative)

> "1. Ok for now, will need some refinement later. 2. ok do it. 3. & 4. what you mean by that, what effect does it has? 5. ok. 6. More details please. 7. First Kimi K3 CLI. 8. ok. 9. more details please. 10. ok, I do it later. 11. I used firefox."

| Item | Answer | Coordinator reading + execution |
|---|---|---|
| A1 taxonomy v4 | ok for now, refinement later | **RATIFIED** as shipped; "refinement later" → deferred ledger (a v4.x revision when the operator names what to refine). Provenance comment flips from PENDING to ratified 2026-09-17 in the v4.1 packet. |
| A2 quality_bar | ok do it | **(a) RATIFIED**: weight 8 → 12 as a v4.1 supersession (packet `v4.1`, no ⚙, no amendment). |
| A3 A14 veto | question asked | **OPEN** — explanation owed (what a veto window is; A14's effect). A14 stays applied meanwhile (it was the operator's own order). |
| A4 A15 veto | question asked | **OPEN** — explanation owed; A15 stays applied meanwhile. |
| A5 decision.emission | ok | **RATIFIED**: the card kind + `emissionRetryLimit = 2` + `clarificationRoundLimit = 2`; `approachMaxRunes` stays 1200; both constants on the settings-tab clamped-⚙ ledger. |
| A6 readings | more details | **OPEN** — each reading explained in plain words, owed. |
| B7 model directive | First Kimi K3 CLI | **RATIFIED with the sub-choice**: execution first choice = K3 on `kimi-cli`, then K3 on `kimi`, then Opus 5 on `anthropic`; planning + judge = Opus 5; interim self-family-judging flag on receipts until a Kimi key is placed; P-T06-5 golden-set re-run on Opus 5 (rubric v3) before unsupervised judging resumes; E3 rider on the new executor seat; ids live-verified. Packet **P3-TQ-5**. |
| B8 amendment A16 | ok | **APPROVED + APPLIED 2026-09-17**: changelog row A16 in `S00`, marker sites annotated (S07.3 code-ladder paragraph, S07.8 bootstrap bullet, S13.7 re-scan line), assembled spec regenerated and verified byte-exact against the drafts; no ⚙ moved → no S18 sweep (118/33). Opens **P3-TQ-4**. |
| B9 preview substrate | more details | **OPEN** — explanation owed (what promoting it means on the host, what the operator gets, cost). P3-SIT-3 waits. |
| B10 webshop task | ok, later | Operator will deny-with-reason through the platform at a later sitting; world `~/.sinet-rework-sitting` stays as is (never reseed). |
| B11 browser | Firefox | **RECORDED**: the original dead-click experience was in Firefox, not Chromium. TQ-F7's host half widens: the Chrome frame-starvation repro stands on its own, and whether Firefox froze the same way is untested — the host investigation must measure Firefox first (memory `chromium-frame-starvation` updated). Not a Sinet packet. |

Order confirmed as Part D: the LN key ceremony remains the operator's next hands-on act (not scheduled by the operator yet); P3-TQ-5 starts now; the gate-independent TQ-1/2/3 + SIT-1 follow; v4.1 rides beside them; TQ-4 after TQ-5; SIT-2 after SIT-1; SIT-3 after B9.
