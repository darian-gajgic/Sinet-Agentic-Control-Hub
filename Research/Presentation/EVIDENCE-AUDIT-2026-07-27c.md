# EVIDENCE AUDIT — 2026-07-27c (v3.4 — ALL FINDINGS APPLIED, same day)

Full-deck verification of `Research/Presentation/index.html` against `Research/NN-*.md`.
Method: 12 parallel read-only auditors — one per chapter (p06 excluded; verified and fixed earlier
this session), one for framing + blueprint + close, one for cross-chapter consistency. Security (p08)
and metering (p12) audited by Opus-pinned agents. Every load-bearing claim checked on five axes:
number exact · scope as measured · newest result leads · §-pointer contains the claim · chapter advice
matches the report's own Recommendation / What-NOT-to-use sections, with open questions kept open.

**STATUS: all 168 findings were applied on 2026-07-27 (operator-approved), in four passes: the p05/Fable+report-02
capture by the coordinator, then three sequential fix agents (p01–p03+framing · p04/p07/p09/p10+mechanics ·
p08/p11/p12/p13 on Opus). H3 was resolved by LIVE-VERIFYING both vendor sources and capturing them as report 02 §11
(dated addendum): documented per-request fallback (help-center) vs observed session-sticky (issue 78888, open —
319 consecutive Opus messages/~23 h; 40 flagged sessions since 2026-06-30); the deck's billing claim was dropped as
unsourced. An independent adversarial verifier CONFIRMED all 7 HIGH fixes and 8/8 sampled MED fixes, with 0 markup
errors; its three minor residuals (p03 mixed-baseline sentence, uncredited intro-pane spike figure, close card 12)
were closed by the coordinator. Full QA battery + textfit green (only the intentional p08 ✕). The findings below are
kept as the audit record.**

## Verdict in one paragraph

The deck's substance survives the audit: no predecessor references anywhere, the twice-renumbered
structure is fully consistent, ~35 repeated statistics agree at every occurrence, nearly every number
is verbatim-exact against the reports, currency discipline (2026 result leads, old studies demoted to
history bands) held in all but a handful of places, and every chapter's recommendation matches its
report's own recommendation section. What the audit found instead: **7 HIGH defects** (claims that
could mislead a decision), **76 MED**, **85 LOW** — clustering into five systemic patterns, the
biggest being a broken citation-pointer layer (~80 sites) of exactly the kind found in p06 this
morning.

Direct answer to the operator's question — *are recommendations resting on obsolete facts?* —
**almost nowhere**. The one old-study-leads case was p06's judge pane (fixed this morning). The
residue: p10's summary lines reuse the 2024 below-random judge magnitude in present tense (the
gauge/table sites pair it correctly with the 2026 study); two "current frontier models" labels sit on
Nov-2025-model measurements (p03 chart legend, close recap); and one date ("lands July 24, 2026") is
now in the past. That's the complete stale-as-current list.

---

## THE 7 HIGH FINDINGS

**H1 · p02 Intake (L1249, 1423–5, 1536–7, 1556) + close card (L6236–7): the flagship RCT number
misstates its comparator.** "Surfacing the assumptions cut wrong-plan approvals from 42% to 22%" /
"halve bad approvals". Report 03 §2.4: 22% vs 42% compares against the *what-if forcing-function
variant* — a competing design, not an untreated baseline. The report gives no unmitigated baseline,
so the before→after framing invents one. Fix: "22% erroneous acceptance vs 42% for the best
competing variant."

**H2 · p04 Swarms (L2077 + 2093): the before/after cost graphic mixes the two baselines the
chapter's own chip warns against.** Before: "≈15× chat tokens"; after: "designed cost ≈2–4× one
agent". 15×-vs-chat ≈ 3.75×-vs-one-agent — *inside* the after range, so the implied saving is a
baseline artifact; and the Google data says the coordinator topology costs *more* tokens (its win is
error containment). This is the residual site of the previously corrected 15× framing. Fix: put both
panels on the single-agent baseline, or recast the contrast as error amplification.

**H3 · p05 The right model (L2490–5, 2690–2, 2771): the Fable 5 silent-fallback row cites "Report
02 / live vendor record" — and no research file contains it.** The specifics (sticky fallback, 840
messages / 23 h, issue 78888, the help-center article) exist only in the *sales deck*, whose live
vendor sourcing was never captured into `Research/`. The only research-documented fallback is a
different phenomenon (report 09 §2.5: Opus→Sonnet at a usage threshold, issue #3434). The internal
records also differ on characterization (sales-deck record: sticky session-level; safeguard memory:
lossless per-turn). **Operator decision needed:** either capture the vendor sources into a research
file with URLs + access dates and reconcile the characterization, or rebuild the row on report 09
§2.5.

**H4 · p08 Security (L3981–93): 12 of 13 evidence-table rows point at the wrong report-10 section**,
including one pointer to a section that does not exist (§5.6). Full corrected map is in the p08
detail below.

**H5 · p08 Security (L3937, 3839, 3846–7, 3688): "no credential ever inside the jail" asserted as
settled.** Report 10 §3.4 calls the engine-credential tension "the sharpest design question in this
topic" — open, pending a per-engine cert-pinning spike, with a documented fallback where the engine's
subscription credential *does* sit in the sandbox behind pinned egress. Fix: "no *task* credential
inside the jail; the engine's own credential is pending a per-engine spike, with a scoped-egress
fallback."

**H6 · p12 The bill (L5240–1, 5307–16, 5340): the 19× overshoot is presented as a notify-only
budget failure.** The lede joins it to "'budget caps' that only send an email" and the graphic puts
the bar inside a pane headed "THE BUDGET THAT ONLY WARNS" — but the measured cap (Claude Code's
`--max-budget-usd`) *does* enforce and hard-stop; it overshoots because enforcement is post-call.
The chapter's own .why block states the correct mechanism, so this is also an internal
contradiction. Fix: name the artifact and retitle/move the pane.

**H7 · p12 The bill (L5501, echoed 5251, 5400): the 1/6–1/16 cached-resume figure is sourced to
"Anthropic prompt-caching documentation" — it is the campaign's own spike measurement** (G1-S1,
Haiku 4.5, tiny prompts, ~4 probes). Anthropic docs support only the TTL facts. Fix: re-source to
"Sinet spike G1-S1 — own probes, small n" + SCOPE-LIMITED tag; qualify visible prose ("in our own
probes"). Same misattribution appears in p07's evidence row (L3586).

---

## THE 5 SYSTEMIC PATTERNS (what actually needs fixing, beyond the 7 above)

**P1 — The citation-pointer layer is broken deck-wide (~80 sites, every chapter except p06-as-fixed).**
Wrong § numbers in chips and evidence tables; 4 wrong-FILE citations (p07's Trigger.dev → report 05,
p07's cache figure → report 08, p11's half-life → report 14 with the wrong cohort named, p12's
Sonnet-5 pricing → report 09, p04's crewAI/LangGraph row → report 08); 3 pointers to nonexistent
sections (report 10 §5.6, report 16 §2.7, report 15 §2.7). Numbers are almost always right — the
pointers under them are not. This is one mechanical sweep with a verification pass.

**P2 — Scope inflation (~25 sites).** Measured-on-one-thing stated as general. Worst offenders:
p10's "~2 seconds" model-load (floor of a 2–10 s range, five sites); p10's lede "frontier prices buy
nothing measurable" (parity measured only for entailment, vs a 2024-gen model, on a semi-stale
board); p05's "frontier judge" for Claude Sonnet 4 (chip discloses, prose doesn't); p05's "top four
entries inside a 0.2-point spread" (real spread 1.8 — own .why says so); p02/p13-style "humans wave
through 93%" (one product's telemetry); p11's invented "a month before" watchlist window; p13's
logprob canary presented per-lane when subscription CLI lanes can't run it (exactly where silent
swaps live); framing's "six current frontier models" for mostly mini/Flash-class Dec-2025 models.

**P3 — Design defaults and open questions presented as settled (~12 sites).** The reports mark these
⚙/OQ; the deck states them as fact: p03's 50%/70% stage budgets; p04's helper/spawn/report-cap
knobs; p01's first-3-human-reviewed; p12's done-directly formula (report 09 OQ3: awaits operator
pre-registration) and >20%/≥2× thresholds under "MEASURED" headings; p05's "stakes" triage under a
"PROVEN" band (report 03: no published figure — own eval required) and the dissimilarity pairing
policy (report 04 OQ1); p08's Landlock-ABI/reflink host prerequisites; p11's xAI/Kimi lanes (open
operator decisions). Fix: visible "proposed default / design decision / operator decision" labels —
the deck's own UNMEASURED tag exists for exactly this.

**P4 — Attribution and provenance blurs (~10 sites).** Campaign self-measurements credited to vendor
docs (H7); a third-party April test credited to the campaign (p07's 8.4×); vendor-documented examples
called "measured" (team's 6,100→420); "OpenAI's own budget documentation" for help-center text
relayed via forum (primary 403'd); the polling-wake fact attributed to NVIDIA docs (2021 community
thread, age-flagged); p12 has *no ref chips at all* (legend promises them); four quoted report titles
don't match the files' H1s.

**P5 — Residual stale-as-current (4 sites — the direct answer to the operator's question).**
p10 summary lines reuse the 2024 below-random magnitude in present tense (L4455, 4510, 4514, 4702);
"current frontier models" on OOLONG's Nov-2025 models (p03 legend L1623/1631; close card L6240–1);
DeepSeek deletion "lands July 24, 2026" now past (L2697). Everything else current-leads-history.

---

## PER-CHAPTER DETAIL

### p01 Skills (report 15) — 0 H / 4 M / 7 L — substantively faithful
MED: "no gain on any model class" vs report's flagged unreplicated GPQA counterexample (L892, 1100) ·
invented "eight-model / eight families, three years apart" aggregate (L932, 1131–2; real: 4 families
2023, six models Dec 2025, ~2 yrs) · 71.6→66.3 pane missing 7–8B scope tag + over-generalized caption
(L1046–52) · 38.4% conflated with below-baseline result (L1073–4; 38.4 is *above* its own 35.4
baseline; below-baseline = weaker models w/ irrelevant retrieval).
LOW: 9 of 14 §-pointers (§2.3→§2.2 ×4, §2.4→§2.2 ×4, §2.7→§2.3, §2.5→§2.1) · "+16.2 from 2–3 files"
mislabel (avg vs +18.6 dose; also in close card L6231) · first-3/composer-trigger as fixed (⚙/OQ) ·
Cursor "silent" memories (were auto-proposed; report 11's lesson is proposal-noise economics).

### p02 Intake (report 03) — 1 H / 3 M / 6 L — faithful except H1
MED: 27.9% used uncited (lives in report 04 §2.2; add chip + "best single model" scope) · "capped at
3 rounds total" not in report 03 (label design decision) · "more than one thing ambiguous" (measured
at exactly 3, asking all three; tile correct).
LOW: 14 §-pointers off-by-one (§2.2→§2.3, §2.3→§2.4, §2.4→§2.5, §2.5→§2.2) · "humans" for Claude
Code users (93%) · SVG "~20% of new users" missing "sessions" · "pipelines" plural for one Spec Kit
measurement · Copilot Workspace date needs "reported" · evb row bundles two source dates.

### p03 Context (report 07) — 0 H / 6 M / 4 L — sound core
MED: evb §2.6→§2.7 (convention ablation) · evb 07 §2.4→§2.8 (caching) · 0.1×/41–80% cache savings
are metered-lane facts (subscription weighting "assumed, unverifiable" — qualify) · "single biggest
driver" unranked in report + chip-only caveat · 50%/70% ⚙ defaults as fixed · −28.6%/−16.6%
attributed to "short and curated" (study measured presence-vs-absence, confounded).
LOW: "one silent compaction" (no cycle count in report) · "current frontier" on OOLONG models ·
"every model" (18 tested, single-source) · ~35%-fire chip drops "one report of" hedge.

### p04 Swarms (report 06) — 1 H / 6 M / 4 L — correctly re-headlined; H2 above
MED: 17.2× reassigned to mesh (measured on independent agents; own chart correct) · "shape most
templates ship today" (market retreated; <15% production) · telephone-game graphic renders the 72%
discussion result as per-hop relay attrition (relay evidence is ~50% verbatim-relay) · crewAI/
LangGraph double-fire row sourced to report 06 (lives in report 08; "LangGraph Cloud"; only row with
no chip/evb entry) · knobs as implementation (⚙ G1 proposals; depth 2 is D6-fixed and fine) · "none
enforces an operator depth cap" (Codex max_depth configurable).
LOW: 3.9× vs 3.85× · chip §2.4 for a §2.1 number · runaway-tree per-level counts are invented
interpolation (documented: 47 sessions / 20 levels) · "one narrow win" vs report's three wins.

### p05 The right model — 1 H / 6 M / 6 L — duty table faithful; H3 above
MED: report 15 §2.5→§2.4 (58.8% CAIN + lifecycle, chip + row) · "0.2-point spread" misattached
(L2396–7, 2577, 2703) · "frontier judge" for Sonnet 4 (L2406–7, 2583, 2711–2) · 1.5B Arch-Router
claim has no chip/evb row (add: report 03 §2.6) · "stakes" under PROVEN band (unmeasured per report
03 §2.6) · "LOCAL 9B … 4.4–4.8%" (measured on 8B and 12B; 9B workhorse has no HHEM entry).
LOW: DeepSeek date past tense · lede "8B level with a frontier model" (2024-gen comparator; caveat
exists elsewhere — qualify lede) · MiniMax drains credits ≠ per-token billing · 78×-row missing THIN
(single-author A†) · FRONTIER row cites topology study as tier evidence · dissimilarity policy is
OQ1 unless spec ratified.

### p06 Verification — fixed this session (three-band judge pane, unsolved band, §-pointers, local-
entailment unbundling). Cross-audit adds: rename the evb row subject "small model (GPT-4o Mini)" so
it can't be merged with the 1/15 Gemini-Flash study two rows down; optionally align SVG layer
numbers (0–3) with impl-rail steps (1–5).

### p07 Machinery (reports 05/08) — 0 H / 5 M / 13 L — faithful; sloppy evidence layer
MED: Trigger.dev 3,800+ → report 05 §2.8 (wrong file) · 1/6–1/16 → report 08 §2.2 + own-spike credit
(H7 sibling) · 8.4× is a third-party April test (only 19× is the campaign's) · 85.3% + 40 GB merged
row (R08 §2.2 issue / R05 §2.2 flagged secondary) · "$0.03→$0.35 ~20×" (report: $0.02–0.05 range;
$0.03→$0.35 is ~11.7× — arithmetic visibly broken).
LOW: 5 more §-pointers · crewAI in-doubt cell overstates human gating (per-class resolution) · 74
Chrome orphans mis-attributed (agent browsers; 18 node = dead sessions) · ~20× scoped to "every
message" (it's --print --resume) · "the meter that caught it was the user's" unsupported · "waits
forever" (re-prompt works, paid) · three flagged figures need THIN qualifiers · Tailscale/PocketBase
confirmed for WAL only (FULL fsync is Sinet ⚙) · evb header vs 6 missing rows.

### p08 Security (report 10) — 2 H / 4 M / 7 L — H4+H5 above
MED: Landlock is Codex-only (not "both converged") · "whole ladder = the exact stack both shipped"
(only the isolation core; C0–C4 is Sinet's) · GET/HEAD/OPTIONS is Codex *cloud*, not CLI · host
prerequisites (Landlock ABI probe, btrfs/XFS reflink, AppArmor userns) drawn as settled.
LOW: "both silently fixed" (one had versioned fix; the *absence* was silent) · "v2.0.24–v2.1.89"
missing tilde · CVE chip merges two incidents (SOCKS5 has no CVE) · srt "Beta Research Preview"
status dropped · "stolen key" (mechanism blocks attacker's *own* key) · gh-aw is technical preview ·
omissions: opencode-lane per-run isolation asymmetry (OQ2), config-poisoning escape surface (OQ5).
Corrected §-map for the 13 evb rows: 12-defences §2.4 · Opus-4.7 §2.4 · CamoLeak §2.3 · allowlist
§2.3 · SOCKS5 §2.3 · DNS/DoH §2.3 · Rule-of-Two §2.4 (correct) · Codex-engine §2.2 · gh-aw §2.5 ·
firejail §5 · bwrap/gVisor §3.1 · 84% §2.1 · residuals §7.

### p09 Memory (report 11) — 0 H / 6 M / 3 L — content faithful; 7/12 pointers shifted
MED §-pointers: experience-following + 39→13%/248→2,400 §2.4→§2.3 (×4 sites) · MINJA §2.4→§2.3 ·
mem0 dates §2.2→§2.1 · OWASP ASI06 §2.5→§2.6 · Cursor §2.3→§2.4 · ChatGPT memory §2.3→§2.1.
LOW: "audit trail doesn't exist anywhere" (logging exists; lesson→runs surfacing doesn't) · SVG
omits "worker may append observation notes" · "<2% utility drop" row drops "3 of 4 testbeds" scope.

### p10 Local tier (report 16) — 0 H / 11 M / 4 L — 2024/2026 pairing right; range + pointer defects
MED: §-pointers ×6 (incl. nonexistent §2.7; AggreFact §2.4→§2.5; Phi-4-mini §2.6→§2.1; vLLM rows
§2.2→§2.3; Wh §4.6→§2.6) · polling-wake source misattributed (2021 community thread, not NVIDIA
docs) · "~2 seconds" load ×5 sites (range is 2–10 s; 9B ≈3–7 s) · "0.9-point spread between three
specialists" (=1.8; own .why correct) · lede over-generalizes entailment parity to all local duties ·
"wrong about as often as right" present-tense reuse of 2024 magnitude (×4 sites) · impl "measured"
vs own UNMEASURED row for Wh figure.
LOW: "2026's small models became…" class-wide from one model · LM Studio §3→§2.3 · "Two caveats"
then (1)(2)(3) · €/$ mixed in one pane.

### p11 Providers (reports 01/02) — 0 H / 5 M / 5 L — sound; hedges preserved
MED: half-life → report 14 §2.2 + real cohort (wrong file + wrong cohort) · Qwen-ban §2.2→§2.1/§2.4 ·
Chutes §2.2→§2.5/§7 · board-disagreement §2.4→§2.6/§7 · invented "a month before" notice window.
LOW: 321-releases and Gemini-CLI pointers · source dates are effective dates · "enforcement is real"
over-scoped for sharing bans · xAI/Kimi lanes are open operator decisions (OQ1/OQ5).

### p12 The bill (report 09) — 2 H / 6 M / 8 L — H6+H7 above; NO ref chips in whole chapter
MED: evb §-pointers ×5 (19× §2.5→§2.4 · unknown-model §2.2→§2.1/§2.6 · hard-stop §2.1→§2.3 ·
fallback §2.1→§2.5 · 6–16× §2.6→§2.3) · Sonnet-5 pricing cited to report 02 (wrong file; report 09
§2.6) · done-directly figure as shipped (OQ3 open) · "no per-purpose split" (Claude Code /usage has
per-skill/subagent/MCP attribution — narrow the claim) · "can't be tied to a person" ×3 (LiteLLM
key/user/team budgets exist; scope to first-party CLIs) · "silently downgraded… quietly ships worse
results" (one-line notice documented; quality loss explicitly unquantified).
LOW: "industry norm" vs own OpenRouter counter-example · "tools'" plural · OpenRouter "before the
call" is flagged inference · "OpenAI's own documentation" for forum-relayed text · 19× needs
proportionality bound (8.4× at $0.01) · >20%/≥2× under "MEASURED" headings unlabelled · illustrative
receipt implausible vs Haiku list price · "token ≈ ¾ word" unsourced heuristic.

### p13 Flying blind (reports 12/15) — 0 H / 7 M / 3 L — content exemplary; pointer bug on both chips
MED: MAST chip+row §2.3→§2.4 · CAIN chip+row report 15 §2.5→§2.4 · promptfoo §2.1→§2.7 · OpenAI
Evals §2.1→§2.7 · loop-detector rows §2.3→§2.4 ×2 · 17-Elo §2.5→§2.6 · logprob canary needs lane
caveat (unavailable on subscription CLI surfaces — behavioral canaries are the fallback there).
LOW: "catastrophic" for the ≥75/25 tier (report vocab: "large"; catastrophic = ≥85/15 monthly) ·
auto-metered "(reject)" flat (acceptable with proven disable) · "anytime-valid" method label on an
exact-binomial table.

### Framing + blueprint + close — 0 H / 4 M / 10 L
MED: "six current frontier models" (mini/Flash-class, Dec 2025) · SkillsBench numbers visible-
unqualified in team vignette (THIN flag chip-only; p01 carries it visibly) · blueprint "agents
messaging laterally amplify 17.2×" (independent agents; p04 labels correctly) · close card repeats
H1's 42→22 misframing.
LOW: chips §3→§2.5 (URLs) and 15 §2.3→§2.2 (SkillsBench) · squeeze bar column mixes baselines
(label "≈3–10× of one agent") · "studies" → one study · "measured 6,100→420" is a vendor-doc
example · close "current frontier under 50% at 128K" (name the Nov-2025 models) · close "local
models fail as judges" (8B-class) · "$40k / 18.6×" without "reportedly" · four report titles ≠ file
H1s · "3–4 orders below SQLite's ceiling" (referent is sqlite.org's client/server threshold).

### Cross-chapter — 0 H / 3 M / 5 L — structure fully consistent
MED: JudgeBench 40.86% CURRENT in p05 evb vs HISTORICAL in p10 (split p05's row) · 78× study "small
model" (p05) vs "mid-tier judge" (p06 row; also near-identical wording to the separate 1/15 study —
double-count risk) · "+16.2 from 2–3 skills" in p01 tile + close card vs +18.6 dose in p01's own
graphic.
LOW: @keyframes names in 10 chapters still carry pre-resequencing numbers (not reader-visible; no
collisions; mechanical rename) · p06 layer 0–3 vs rail 1–5 · p13 data-title "Observability" vs
"Flying blind" · team "as of April 2026" → "measured April 2026" · MAST tag facets differ p04/p13.

---

## TOTALS

**7 HIGH · 76 MED · 85 LOW = 168 findings.**
Verified clean everywhere: predecessor purge (0 hits), running order + all cross-references + team
rails + recap cards, repeated statistics (~35 checked at every occurrence), status-tag vocabulary,
internal anchors, terminology stability, quote verbatim-ness (spot set), and recommendation-match
against every report's own Recommendation / What-NOT-to-use section.

## RECOMMENDED FIX ORDER

1. **The 7 HIGHs** — each is a targeted rewrite of one pane/row/sentence cluster. H3 needs an
   operator decision (capture the Fable 5 vendor sources into a research file, or rebuild on report
   09 §2.5).
2. **P1 pointer sweep** — one mechanical pass over ~80 chip/evb pointers using the corrected maps
   above, then re-verify with a spot-check agent.
3. **P3 settled-vs-open labels** — add the deck's own UNMEASURED/design-decision tags at ~12 sites.
4. **P2/P4 scope + attribution rewordings** — sentence-level edits at ~35 sites.
5. **P5 + LOW hygiene** — stale labels, dates, keyframe rename, data-title, unit/currency nits.
6. Re-run the full QA battery + textfit; update HANDOFF.md and persist this file as
   `Research/Presentation/EVIDENCE-AUDIT-2026-07-27c.md` alongside the applied changes.
