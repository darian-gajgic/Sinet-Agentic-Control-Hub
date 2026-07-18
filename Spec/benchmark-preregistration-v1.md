# Benchmark pre-registration — v1

> **STATUS: REGISTERED — v1, ratified by the operator 2026-07-18.** In-session ratifications: both launch domains registered; front-loaded sampling schedule (§4.1); Ed25519 SSH signing; bundled defaults accepted. The registration is the **signed commit introducing this line** (key `SHA256:peCbOEDFlkCrzVBdvaM7YhXMxVCt/pMGSLqE4ZOEiP4`); the commit hash is recorded in `Research/STATE.md`. Changes from here on follow §17 only.

**What this file is.** The pre-declared, non-retunable protocol for the platform's standing self-benchmark (feature list 11.2 / S2.11) and the definition of the benchmark gate (15.3). Per GATE-2 Decision 2.8 and GATE-3 Decision 3.1, this registration is frozen in a **signed git commit before v0 ships**; the commit is the registration (git-commit-as-preregistration, the solo-maintainer form of the practice — report 12 §2.6). Discharges R12-OQ1 and R12-OQ2; carries P-T11-1, P-T11-2, and P-T11-5 as designed-in obligations.

**Authorities:** feature list 11.2, S2.11, 14.5, 15.1–15.3, 5.7; GATE-1 D1.7 (trivial band) and rider 1 (settings-not-constants); GATE-2 D2.8 (two-stage done-directly + package numbers); GATE-3 D3.1 (session slot). Evidence base: `Research/12-evals-observability-benchmark.md` §2.6, §4.6, §4.7 and its sources [79–94].

---

## 1. What is registered vs. what is tunable

**Registered (frozen — changes require a dated re-registration commit, §17):** the protocol (§3), the eligibility rules (§4.2), the blindness-measurement plan (§5), the arm-parity declaration (§6), the statistical rule and its thresholds (§7), tie handling (§8), the epoch rule (§9), the per-domain registrations (§10), the gate and alarm definitions (§11–§12), the done-directly formula text (§13), and the record schema minimums (§14).

**Tunable ⚙ (operator-editable at runtime, no re-registration needed):** the sampling **rate** (§4.1 — affects accrual speed, never inference validity; the *uniform-randomness* of sampling is what's frozen), reporting cadence and presentation of the readout views (§16), and inbox/card wording. Rationale: a registration whose thresholds float is theater (the Leaderboard Illusion, report 12 [82]); a registration that freezes presentation trivia is bureaucracy.

Registered numbers surface in the settings registry like every other number (G1 rider 1) but are marked **"registered — changing this value requires a re-registration commit"**; the settings audit trail links any change to its registration commit hash.

## 2. Definitions

- **Pair** — one sampled task run through both arms and blind-scored once by its requester.
- **Platform arm** — the deliverable the platform produced and the requester accepted through the normal pipeline (intake → … → review → accept).
- **Direct arm** — the same confirmed task statement + the same attachments the specification had, run **once, single-shot** against the requester's own frontier surface for that domain (their consumer subscription — chat or CLI — with that surface's native defaults; no retries, no follow-up turns, take what comes).
- **Verdict** — requester's blind pick: **A / B / tie / both-bad**.
- **Non-tied pair** — verdict A or B. `m` = non-tied pairs; `w` = platform wins, within one (domain, epoch).
- **Epoch** — a maximal interval during which the direct arm's observed model identity is unchanged (§9).
- **Launch domains** — software-development (v0, 15.1) and web-research (v0.1, 15.2) (§10).

## 3. Protocol (per sampled, opted-in task)

1. **Direct arm run.** As defined in §2, at task-acceptance time (both arms answer the same frozen task statement; neither sees the other). Consumption is drawn from the requester's automation budget under their standing opt-in (11.2).
2. **Blind pairing.** Both deliverables rendered through **one uniform presentation template**: formatting and scaffolding tells stripped; **length never truncated** (the length confound is reported alongside, not "corrected" at this n — report 12 [81]). Position (left/right) randomized per pair. Arm identity revealed only after the verdict is recorded.
3. **Verdict + guess.** The requester records the verdict (A/B/tie/both-bad) **and, with the same form, guesses which response was the platform's** (§5). No verdict without a guess; the guess is never optional.
4. **Record.** The full §14 record is written before identity reveal completes. The pair's measured consumption on both arms is simultaneously the **measured done-directly datum** (§13).

## 4. Sampling and opt-in

### 4.1 Rate and mechanism

- Sampling is **uniform-random among eligible tasks** at verdict-eligibility time (frozen). The rate is ⚙; rate changes are logged settings events and never require re-registration.
- **Registered default schedule (operator ratification 2026-07-18 — front-load the evidence):**
  - **Pre-gate phase:** **100% of eligible tasks** are sampled, per domain, until that domain's gate (§11) first opens — the operator's stated intent: a small benchmark batch immediately after core functionality works, then maximum accrual "so the platform is not running unproven for long." At ~15–30 eligible tasks/month (~25% tied), this reaches the gate's n=20 in **≈1–2 months**. Allowance is backstopped by construction: duplicate runs draw the requester's automation budget under the standing admission/pressure machinery (G2 Def.4) — when allowance is tight, benchmark duplicates throttle like any background work.
  - **Maintenance phase:** after a domain's gate first opens, its rate drops to a default **25%** — this is the 11.2 "configurable low rate" regime; 25% (not lower) keeps the answer *current*, because an epoch reset (§9 — consumer surfaces auto-upgrade) empties the current-epoch record, and 25% re-establishes n=20 in about a quarter rather than a year.
  - Sampling ALL eligible tasks is trivially uniform; the phase schedule changes accrual speed only, never inference validity.
- **Bring-up seed note (non-binding intent, dated 2026-07-18):** at v0 core completion the operator intends an initial batch of real eligible tasks run through the full protocol as the first accrual burst; these are ordinary pairs — blind, guessed, recorded — with no special statistical status.

### 4.2 Eligibility (frozen)

A task is eligible iff **all** of:
1. Its requester has the standing benchmark opt-in enabled (11.2); duplicate-run consumption comes from that requester's automation budget.
2. It belongs to a registered launch domain (§10) and ended in an **accepted deliverable**.
3. It is **not** in the zero-interaction trivial band (G1 D1.7, $0.50 API-equivalent) — trivial tasks don't exercise the pipeline under test.
4. Its essence is a reviewable deliverable, not a side effect — tasks whose point is an effect (deploy, send, publish) have no comparable single-shot direct arm and are excluded.
5. The requester did not decline this specific sampled pair. **Declines are logged and the decline rate is reported next to every win rate** — selective participation must be visible, not silent.

## 5. Blindness is measured, never assumed (P-T11-1, frozen)

Expert daily users detect AI/platform style near-perfectly (1/300 misclassification, report 12 [89]); presentation normalization is not established shipped practice anywhere. Therefore:

- The uniform render template (§3.2) is the *mitigation*; the *measurement* is the per-vote guess.
- **Guess accuracy is reported beside every published win rate**, at the same n. If guess accuracy beats chance, the win rate still publishes — labeled **"partially unblinded (guess accuracy g%)"**. No result is suppressed for blindness failure; it is labeled.
- Calibration pilot (R12-OQ8, non-binding note): ~10 mock pairs with household members at v0.1 to calibrate the render template before member-facing use.

## 6. Arm-parity declaration (P-T11-2, frozen)

Declared, not hidden:

- The platform arm carries household memory, knowledge injection, multi-turn pipeline, tool use, and verification loops. **That asymmetry is the treatment under test**, not a confound.
- The direct arm runs on a consumer surface whose model identity **cannot be pinned or controlled**; it is recorded as observed per pair. The direct arm has no household memory — arguably the thing being tested.
- Both arms answer the identical frozen task statement with identical attachments; the direct arm's surface-native tools (e.g., its own web search) are allowed as-is — the baseline is "what the requester's subscription actually does", not a stripped model.
- No published prior art exists for "platform vs the user's own subscription surface" (report 12 §2.6, negative finding) — parity gaps beyond this declaration are treated as new registration material, never silently absorbed.

## 7. Statistical rule (frozen, verbatim)

Per (domain, epoch), over non-tied pairs only:

- Posterior: `p ~ Beta(1 + w, 1 + (m − w))` — Beta-Binomial with uniform prior, updated per pair.
- Define `G := P(p > 0.5 | w, m)` — computed exactly (regularized incomplete beta; for integer parameters `G = P(Binomial(m+1, ½) ≤ w)`).
- **Monitoring is continuous**: the posterior may be inspected at any time, updated per pair, with optional stopping — the anytime-valid/Bayesian running-rule regime chosen in report 12 (approach E2), matching drip-feed accrual with no α-inflation ambitions beyond what the posterior honestly states.
- `G` is the **only decision input**. Fixed-n views (§16) are readouts, never decision rules.

Verified reference points (exact computation, this session — cross-checked against report 12 §2.6: 7/10→0.887, 14/20→0.961, 35/50→0.998):

| Record (w/m) | G = P(platform better) | Meaning |
|---|---|---|
| 13/20 | 0.905 | minimal gate-clearing record at m=20 |
| 12/20 | 0.808 | does **not** clear the gate |
| 0/4 · 2/10 · 4/15 · 6/20 | 1−G = 0.969 · 0.967 · 0.962 · 0.961 | earliest alarm-zone records |

## 8. Tie handling (frozen)

- **tie** and **both-bad** verdicts are excluded from `m` and `w`; they never enter the posterior.
- Both are first-class recorded signals: **tie rate and both-bad rate are reported next to every win rate**, at the same n. Ties reduce effective accrual and are planned for in §4.1's arithmetic; no imputation, no re-votes, no re-pairing.
- A sustained high both-bad rate is a product-quality signal surfaced by the reporting views (§16); it carries no automatic decision consequence in this registration.

## 9. Epochs (P-T11-2, frozen)

- An epoch **ends** when the direct arm's observed model identity changes (consumer surfaces auto-upgrade; baselines cannot be frozen — report 12 [91–93]). Platform-arm model changes are recorded per pair but do not split epochs (the platform evolving is the treatment).
- **No decision statistic ever pools across epochs.** Gate and alarm evaluate over the **current epoch only**. Reporting views may show per-epoch history side by side.
- Predecessor data: Nexus **bench-02** results are a **closed historical epoch** — recorded for context, never pooled into any Sinet statistic (different protocol, non-blind, different system).

## 10. Per-domain registrations

Identical rule structure; per-domain deliverable definitions. Accrual for web-research starts at v0.1 ship.

### 10.1 software-development (v0; feature list 15.1)

- **Deliverable compared:** the accepted final artifact set (diff/patch/files) plus its accompanying summary, as accepted at review.
- **Direct arm surface:** the requester's own frontier coding surface (consumer subscription chat/CLI), single-shot per §2.
- **Confirmatory metric:** blind pairwise preference under §7. Thresholds: §11/§12 registered values.
- **Regression criterion (gate limb d):** the domain's rubric + worker golden sets and planted-defect suites (report 12 §4.7, report 04 machinery) are green at their per-version floors. Floors are registered **per rubric/eval-set version at 8.3 knowledge-gate entry**; the gate cannot be evaluated — and therefore cannot open — while the domain lacks registered suites or any suite is red.

### 10.2 web-research (v0.1; feature list 15.2)

- **Deliverable compared:** the accepted research report (deliverable file(s) as accepted).
- **Direct arm surface:** the requester's own frontier surface with its native web tools as-is, single-shot per §2.
- **Confirmatory metric / regression criterion:** as §10.1, with web-research rubrics/eval-sets registered at their 8.3 entry.

## 11. The benchmark gate (15.3, frozen)

Per launch domain, evaluated over the **current epoch**:

- **(a)** `m ≥ 20` non-tied pairs, and
- **(b)** `G ≥ 0.90`, and
- **(c)** no active alarm (§12), and
- **(d)** the domain's regression suites green at registered floors (§10).

**The 15.3 gate opens when every registered launch domain passes.** Until then, 14.5 breadth restrictions stand (no new domains beyond the launch ladder, no decorative surfaces). Degraded-mode domains (2.1/7.6) remain allowed from launch — they are not "breadth" in 14.5's sense and are outside this benchmark entirely.

If accrual volume lands well below ~5 pairs/month per domain, the honest path is a **dated re-registration** proposing a catastrophic-only low-n variant — pre-committing a weaker alternate gate in this document was considered and rejected (it would be a built-in goalpost move).

## 12. The alarm (frozen)

- **Trigger:** at any per-pair update, `1 − G > 0.95` (platform losing) in any launch domain's current epoch.
- **Action:** a **flag-now** card (watchdog inbox, report 12 §4.4 severity contract) carrying the full record, plus an **expansion freeze**: no new domain, surface, or breadth work while the alarm stands (14.5 hold). Nothing is auto-killed; running work is untouched (D1.3).
- **Standing/clearing:** the alarm stands while the condition holds in the current epoch; the card requires an operator disposition (logged as a decision event) — investigate, fix-and-continue accruing, or re-register with rationale. Alarm history is keep-forever (report 12 §4.2 class).

## 13. Done-directly formula (GATE-2 D2.8 two-stage, frozen text)

1. **Per-run receipts, from day one:** every receipt carries **"direct-use estimate (heuristic)"** = the final accepted attempt's execution usage priced at list price (D5 API-equivalent currency). Label always present verbatim.
2. **Aggregate honesty figure** (S2.5 dashboards, per-person/per-domain rollups): once a domain has accrued **≥ 10** measured benchmark pairs, the aggregate switches to the **measured median direct-arm consumption** for that domain, labeled **"measured (benchmark n=…)"**. Per-run receipts keep the heuristic line; both labels remain available side by side.
3. The formula text above is part of this registration: **the honesty keystone does not float** (D2.8). Changing either stage's definition, threshold, or label semantics requires re-registration.

## 14. Record schema and storage (frozen minimums)

Per pair, recorded in **Sinet's own store** (benchmark records are keep-forever; external tools are runners, never records — P-T11-4):

pair id · domain · task class · timestamps · both arms' **exact observed model identities/versions** · both arms' measured consumption · refs to both rendered-blind artifacts · position assignment · verdict · requester's platform-guess · decline flag (if vetoed at sampling) · epoch id · requester id.

## 15. Honest-claims table (P-T11-5 — ships in the benchmark UI, frozen obligation)

Small-n humility is a product surface: wherever a win rate is shown, the UI also shows what the current n could even detect. Reference table (exact binomial, one-sided α=0.05 vs 50/50; report 12 §2.6, reproduced from first principles):

| Question | Answer |
|---|---|
| n for 80% power at true 60/40 · 65/35 · 70/30 · 75/25 · 80/20 | 158 · 69 · 37 · 23 · 18 |
| n=20: fixed-test rejection needs | ≥15/20 (75%) |
| n=20 power vs true 65/35 · 75/25 · 85/15 | 0.25 · 0.62 · 0.93 |
| Wilson 95% CI at 14/20 | [0.48, 0.85] — includes 50% |
| Beta-Binomial P(p>0.5) at 7/10 · 14/20 · 20/30 · 35/50 | 0.89 · 0.96 · 0.97 · 0.998 |

Readout: a household quarter (n≈20–30) reliably detects only large effects (≥75/25); a month detects only catastrophic ones (≥85/15). The predecessor's failure was catastrophic-scale — detectable at n≈15–20. Every published rate carries **n, the posterior G, tie/both-bad rates, decline rate, and guess accuracy** (§5, §8, §4.2.5).

## 16. Reporting views (readout-only)

Quarterly per-domain/per-epoch exact-binomial + Wilson-interval summaries, tie/both-bad/decline/guess-accuracy panels, and epoch history are **views over the record** for humans. None of them is a decision input; §7's `G` is the only decision statistic. Cadence/presentation ⚙.

## 17. No-retuning clause and re-registration (frozen)

> **The thresholds, rules, formulas, and labels in this registration are not re-tuned in response to observed results.** Any change — however small — requires a **new dated amendment appended to this file** stating what changed and why, committed in a **signed registration commit**. Amendments are additive; prior text is never rewritten or deleted. Results computed under a prior registration are never retroactively recomputed under a newer one. Silent drift between this text and the running platform is a defect of the platform.

Amendment procedure: append to §19 (date, author, diff summary, rationale) → sign the commit with the registered key (§18) → push. The spec and STATE cite registrations by commit hash.

## 18. Signing (mechanics)

Registration commits are signed (GATE-2 D2.8). Ratified method: **SSH commit signing, Ed25519**.

- Signing key: `~/.ssh/git-signing-ed25519` (operator's machine only, never in the repo). Public key fingerprint: **`SHA256:peCbOEDFlkCrzVBdvaM7YhXMxVCt/pMGSLqE4ZOEiP4`** (comment `sinep-git-signing-sinet`).
- The public key is committed in-repo at **`Spec/allowed-signers`** (allowed-signers format). Anyone with a clone verifies a registration commit with:
  `git config gpg.ssh.allowedSignersFile Spec/allowed-signers && git verify-commit <hash>` (run from the repo root).
- Repo-local config: `gpg.format=ssh`, `user.signingkey=~/.ssh/git-signing-ed25519.pub`. Only registration/amendment commits require `-S`; ordinary commits stay unsigned (`commit.gpgsign=false`).
- Optional (operator, cosmetic): add the public key on GitHub as a *signing key* so registration commits show "Verified" in the web UI. Local verification works without it.

## 19. Registration & amendment log

| Date | Event | By | Commit |
|---|---|---|---|
| 2026-07-18 | v1 registered | operator (ratified in-session) + coordinator | self — the signed commit introducing this row; hash recorded in `Research/STATE.md` |

---

*Discharged by this registration: R12-OQ1, R12-OQ2 (= R09-OQ3), P-T11-1/2/5 (as designed-in obligations). Related, unaffected: report 12 §4.7 revalidation runbook (its floors register per-asset at 8.3 entry); R12-OQ8 pilot (noted §5, non-binding).*
