# HANDOFF — research presentation deck ("AI That Pays Off")

> **⚠ CHAPTER NUMBERS IN THIS FILE ARE PRE-REORDER.** The thirteen chapters were resequenced by importance twice on
> 2026-07-27 — v3.1 (the big move) and v3.2 (chapters 2–5 rotated into the operator's final order). Both passes
> renumbered section ids, `.pNN-*` CSS prefixes, comment banners, evidence-table captions, nav, team rails, close
> recap and the visible numbers together. Everything below — and both `EVIDENCE-AUDIT-*.md` files — still uses the
> **original** numbering. Translate before acting on any `pNN` reference (original → current):
> **1→12 · 2→11 · 3→3 · 4→2 · 5→6 · 6→7 · 7→4 · 8→8 · 9→9 · 10→13 · 11→1 · 12→10 · 13→5**.
> Current running order: 1 Skills · 2 Intake · 3 Context · 4 Swarms · 5 The right model · 6 Verification ·
> 7 Machinery · 8 Security · 9 Memory · 10 Local tier · 11 Providers · 12 The bill · 13 Flying blind.
> (Translating something written against v3.1 numbering instead? Only four moved: 2→5 · 3→4 · 4→2 · 5→3.)

**State: v4 (reading-layout rework) 2026-10-07 — see the v4 section first.** Previously: **v3.4 COMPLETE, 2026-07-27.** v1 was built 2026-07-23 from the approved plan; v2 is the evidence-currency and
restructure pass; v2.1 is the graphics-polish pass; **v3 is the predecessor-purge + task-dependence pass**; v3.1/v3.2
resequenced the chapters; v3.3 is the judge-seat coherence fix in the Verification chapter; **v3.4 is the full-deck
verification pass** (below). Any future session: read this file, then `README.md` (serving + content rules — rules 8
and 9 and the evidence apparatus are load-bearing), then **`EVIDENCE-AUDIT-2026-07-27c.md` (v3.4 — the most complete
audit)**, then `EVIDENCE-AUDIT-2026-07-27b.md` (v3) and `EVIDENCE-AUDIT-2026-07-27.md` (v2), then the plan file for
structural work.

> **⚠ THE DECK FILE WAS RENAMED `index.html` → `Sinet-Research-Presentation.html`** (operator, outside Claude,
> 2026-07-28 ~00:22). Every `index.html` in this file, in `README.md` and in the `EVIDENCE-AUDIT-*.md` files means
> that file — translate before running any grep or serve command. `qa/run-textfit.sh` was patched to fall back to
> the single `*.html` in this directory, so it still runs unchanged.

## v4 (2026-10-07) — the reading-layout rework (structure + style only, content untouched)

Operator finding: people who read the published page found it "interesting but very hard to read" — content right,
structure wrong. Measured on v3.5: ~45,000 words (≈3¼ h of reading) in one scroll, chapters 7–12 screens each,
87% of visible text ≤13.5px, 22% monospace, a 16-link horizontal nav clipping at 1440px, Space/PageDown hijacked
to jump whole 7-screen sections, 13 evidence tables (~300 rows) fully expanded inline, ~60 looping animations
with no pause control (WCAG 2.2.2 Level A). Contrast was NOT the problem (body #9aa4b8 on #070a10 = 7.9:1).

A sourced research pass (NN/g scrolling/F-pattern/progressive-disclosure/accordions/TOC/scrolljacking/dark-mode,
GOV.UK type scale + details + contents list, WCAG 2.2 1.4.3/1.4.10/1.4.12/1.4.13/2.2.2, Butterick, Baymard line
length, Distill/IPCC SPM/OWID/Gwern exemplars) set the rules; `README.md` → "v4 reading layout" lists what was
built. **What is deliberately NOT done** (would change content, operator's call): IPCC-style confidence verbs in
the prose, cutting figures per chapter (research rule "does each chapter need all five?"), numbered footnotes
instead of `ref` chips.

**Content preservation is machine-checked**: every text node of v3.5 is present in v4 (`qa/textcheck.py`). The
only removed strings are the 7 old short nav labels (Skills/Split/Verify/Local/Bill/Blind/Blueprint) and the
"01 · " / "02 · " prefixes of the two intro overlines (they collided with "PROBLEM 01"). Added text is navigation
and labels only (contents list, "The chapter in four steps", reading time + claim counts, the at-a-glance intro
and reading paths, theme/motion/top buttons). The at-a-glance list reuses the close-recap cards verbatim.

How it was built: a one-off migration script (`_rework-v4/build.py` + `global.css`, `theme.py`, `app.js`,
`chrome.html`, `glance.html`, `head.html`) transformed v3.5 → v4. **The HTML file is the source of truth again**;
the script is provenance, not a pipeline — edit the HTML directly. The v3.5 file is kept at
`archive/Sinet-Research-Presentation.v3.5.html`.

Editing rules that are new in v4:
1. **Colours are theme tokens.** Chapter-local CSS uses `var(--c-xxxxxx)` (one per original hex) and
   `rgba(var(--rgb-xxxxxx),a)`; semantic tokens (`--ink --prose --mut --dim --cy --gn --am --rs --vi --panel …`)
   are defined twice in the first `<style>` (dark in `:root`, light under `[data-theme="light"]` and the
   `prefers-color-scheme:light` query). A NEW literal hex will not switch with the theme — use a semantic token, or
   add a `--c-` token to both blocks. SVG `fill="#…"`/`stroke="#…"` attributes are themed by generated
   `[fill="#hex"]{fill:var(--c-hex)}` rules — a new SVG hex needs its own rule there.
2. **Motion**: each chapter's `prefers-reduced-motion` block has an `html.calm …` mirror right below it (the
   Pause-motion switch). Add both when adding an animation.
3. **Chapter skeleton now**: `<header class="ch-head">` [overline div + h2 + lede] + `.ch-meta` [who chips + reading
   time] + `.sum` [h3.sumh + `.beats`] `</header>` → `.explain` → five `.g30.fig` with `id="pNN-<el>"`, each
   `h3.gt` then `.take` then the body → `.asof` → `.tablewrap#pNN-tools` (role=region) → why/quote/… →
   `details.evd#pNN-evidence` [summary: `.et` caption + `.evc` counts] containing `.evb` → `.srcline`. The `.evc`
   counts are static text — update them if you add evidence rows. The contents list is static HTML in `#toc`.
4. Chapter-local font sizes were bumped at build time; `.p07-dc` (hand-positioned card stack) was excluded.

QA after any edit (in addition to the v3 commands below; replace `index.html` with `Sinet-Research-Presentation.html`):
```bash
./qa/run-textfit.sh                       # still exactly 1 finding: the deliberate p08 ✕
python3 qa/textcheck.py archive/Sinet-Research-Presentation.v3.5.html Sinet-Research-Presentation.html   # LOST must list only the 9 strings above
python3 qa/contrast.py Sinet-Research-Presentation.html dark light     # residue: p08 ✕ + p12 CAP tick (false positive) + 3 borderline ≥4.28 labels
python3 qa/overflow.py Sinet-Research-Presentation.html                # expect 0 at 1440 and 1280
```
(The python QA tools need `playwright` with `/usr/bin/google-chrome`; they read the file over `file://`.)

Grep-count notes for v4: `class="st …"` counts are higher than v3 because each evidence summary repeats its status
labels (13 summaries); `data-el` stays 13 × 5; `class="beats reveal"` 13; `class="evb reveal"` 13; `<details` 13.

Deployment: **v4 is live** since 2026-10-08 — `darian-gajgic/agentic-control-hub-research` commit `703463c`
(single file at the repo root, no local clone; Pages rebuilds in ~1 min). To redeploy: clone that repo, copy
`Sinet-Research-Presentation.html` over the root file, commit, push, then confirm the live URL's sha256 matches.

## v3.5 (2026-07-28) — operator logo in the nav

The Sinet logo (`Sinet-Logo.jpeg`, embedded as a base64 JPEG data URI — the deck stays single-file and offline) now
sits at the far right of the top nav, after the "evidence as of July 2026" chip, using the exact markup and CSS the
sales deck at `Presentation/` uses: `.topnav .logo{height:34px;width:auto;display:block;mix-blend-mode:screen;
-webkit-user-drag:none}` (screen blend drops the logo's black backdrop into the nav bar). Text-fit battery re-run
green — the single expected p08 `✕` finding, nothing new.

Known cosmetic consequence: the nav is a horizontally scrolling flex row and was **already** clipping its last two
links (`Blind`, `Blueprint`) at a 1440px viewport before this change; the logo costs ~52px more, so at 1440px it now
clips from `Providers` on. At 1920px the whole nav, chip and logo fit with room to spare. If a future session needs
1440px to fit, the two cheap levers are tightening `.topnav nav a` padding below ~1600px, or extending the existing
`@media (max-width:760px){.topnav .tag{display:none}}` breakpoint upward — the second one trades away the
evidence-date chip, so it is an operator call, not a styling call.

## v3.4 (2026-07-27) — the full-deck verification pass

Operator asked for a complete coherence check of every chapter against `Research/NN-*.md` (current facts leading, no
obsolete-fact recommendations, unsolved things named as unsolved). Twelve parallel read-only auditors (one per chapter,
one for framing/blueprint/close, one cross-chapter; security+metering on Opus) produced **168 findings — 7 HIGH,
76 MED, 85 LOW** — all applied same-day with operator approval. `EVIDENCE-AUDIT-2026-07-27c.md` is the full record.
The headline classes:

1. **~80 wrong citation pointers** (chips + evidence tables citing a report § that doesn't hold the claim, four
   wrong-file cites, three pointers to nonexistent sections) — all corrected against the reports' real section maps.
2. **Baseline/comparator misframings**: the p02 "42%→22%" RCT (42% is the best competing variant, not a baseline),
   p04's before/after (15×-vs-chat compared against ×-vs-one-agent), p12's 19× overshoot (an *enforcing* cap with
   post-call granularity, not a notify-only budget).
3. **Provenance**: the campaign's own spike measurements (1/6–1/16 cached resume; 19× probe) are now credited as own
   probes, third-party tests as third-party; the **Fable 5 fallback row is now grounded in report 02 §11**, a dated
   post-campaign addendum written after LIVE verification of both vendor sources (help-center article = documented
   per-request re-run on Opus 4.8; open Claude Code issue 78888 = observed session-sticky, 319 consecutive Opus
   messages/~23 h). Billing-during-fallback is stated by neither source and is no longer claimed anywhere.
4. **Settled-vs-open labels** at ~12 sites (⚙ proposed defaults, operator open questions — incl. p08's engine-credential
   question, now named open at every site) and **4 residual stale-as-current labels** (p10 present-tense 2024 judge
   magnitude, "current frontier" on Nov-2025 models, a past DeepSeek date).
5. Cross-chapter: the pre-resequencing `@keyframes` names in 10 chapters were renamed to current prefixes (single-pass
   scripted, no collisions); p06's evb 78×-row renamed "A small model (GPT-4o Mini)…" to prevent double-counting with
   the separate 1/15 study; p06 SVG layers renumbered 1–4 to match its rail; p13 `data-title` fixed to "Flying blind".

An independent adversarial verifier confirmed all 7 HIGH fixes + 8/8 sampled MEDs, 0 markup errors; QA battery +
textfit fully green (only the intentional p08 `✕`). Note for future editors: chapter p10 standardized its cost pane on
"€0" (local-tier electricity framing) — deliberate, not a typo.

## v3.3 (2026-07-27) — judge-seat coherence fix (CURRENT-numbering p06 · original ch05)

Operator finding: the Verification chapter's before/after pane offered only the Dec-2025 study (GPT-4o Mini beat
GPT-5-high at 78× lower cost) as its judge-choice guidance — reading as "the cheaper model is the better judge", a
universal tier rule the research contradicts and exactly the single-rule regression Rule 2 forbids. Fixed, all in
**current** numbering p06:

- The before/after right pane is now a **three-band per-duty answer consistent with p05's duty table**: cheap seat wins
  on short structured rubric-scored output (the **April 2026** debiased mid-tier study leads per rule 8; the 78× study is
  the earlier supporting point, scope-limited; calibrated-abstention guard named) · strong reasoning seat **required** on
  hard pairs and report-length work (JudgeBench near-chance, LongJudgeBench 0.563/0.672, ~29% F1 plan critique) ·
  either tier **family-dissimilar from the worker** (CAPA + self-preference), pointing to Problem 05.
- Left pane gained today's other default (report 04 §3: a single frontier judge per deliverable, baseline everywhere).
- A visible `.corr` band names what the research does **not** settle: no measured task-type→judge-tier map exists
  (unmatched duties default to the strong seat — a stated design decision, tagged UNMEASURED), and the local 14B-class
  screen's judge bar on consumer hardware is unproven (golden-set gate).
- Fixed wrong report-04 section pointers across the chapter (judge cluster said §2.2, is **§2.1**; web-research and
  self-correction said §3, are **§2.5/§2.6**) in inline chips and the evidence table; the evidence table gained 12 rows.
- The "work that has no tests" card no longer conflates SourceCheckup's 88.7% (a frontier-pipeline measurement) with the
  local seat; local ≤8B grounding screens are cited scope-limited (pre-2026, news-domain board) behind a golden-set gate.
- The impl rail's judge step now states the seat rule (per duty, strong for hard/long, family-dissimilar).

QA re-run green: 13×5 `data-el`, div depth 0, no `S##` refs, textfit unchanged (only the intentional p08 `✕`).

## v3 (2026-07-27) — the two rules that must never be broken again

**⛔ RULE 1 — the predecessor platform (Nexus) is never mentioned, and never cited.** The operator's ruling: it was an
unrepresentative self-test run without proper setup, so it is not evidence for anything. This is absolute — not "cite it
with a caveat". `grep -ci "nexus\|predecessor" index.html` must stay **0**. Every point it used to carry has been
re-founded on independent published evidence; the replacements are listed row by row in
`EVIDENCE-AUDIT-2026-07-27b.md` §A. Do not reintroduce it, in any chapter, under any framing.

**⛔ RULE 2 — model-tier advice is per-duty and runs BOTH directions.** The old chapter 13 said "frontier does the work,
the mid tier judges". That is wrong in both directions and the operator flagged it: *sometimes a frontier model is needed
as the judge and a weaker model does the implementation — it depends on the task.* Chapter 13 is now built on a **duty
table** with three bands (cheap-model-is-right-worker / expensive-model-required-including-as-checker / no-model-at-all),
each row carrying its own measurement and its own guard. Any future edit that collapses this back into one rule is a
regression, however tidy it reads.

Chapter 13's new headline measurement is the Google 260-configuration scaling study (+80.8% to −70.0% by task) — a far
stronger result than what it replaced. The frontier-as-checker cases are named explicitly: **plan critique**
(fresh-context, ~29% F1), **objectively-hard pairs** (non-reasoning judges near chance), **report-length deliverables**
(mean 0.563). A third axis the old chapter missed is now in: the checker must be **family-dissimilar from the worker**,
because model errors grow more correlated as capability rises — so "just use the best model to check" can be wrong even
when budget is not the constraint.

Nine other defects were found and fixed across the framing sections and chapters 1–12 (C1–C9 in the v3 audit): trade-press
figures presented as measurements, a 15×-vs-chat number contradicting the corrected 3–10×-vs-single-agent framing two
paragraphs away, two telemetry stats conflated in one line, a merged/misattributed source row in p07's evidence table, and
two stale-vintage caveats that lived only inside citation chips where a skimming reader never saw them.

**Graphics note:** the p13 solution diagram was rebuilt from a 3-lane layout into six full-width vertical bands after the
first splice produced 17 text-fit defects. The v2.1 lesson held exactly: vertical space is free, horizontal space is not.

## v2.1 (2026-07-27) — graphics polish

Operator finding: graphics were sometimes badly designed — text cut off or overflowing its boxes. A headless
measurement pass found **73 real defects** across 12 sections: labels wider than the box they sat in, text clipped at
the `svg` frame, boxes overlapping each other, and labels drawn across arcs, needles and flow lines. All fixed;
`./qa/run-textfit.sh` now re-checks it in one command (see `README.md` → Graphics QA).

The load-bearing lesson, applied throughout: **vertical space in these diagrams is free, horizontal space is not.**
Crowded diagrams were re-banded into more rows rather than compressed sideways. Widening a `viewBox` is the wrong
fix — it shrinks every label on the page.

What changed structurally (content and numbers untouched):

- **p10 solution** — was the worst: the watchdog staircase ran diagonally off the right edge, half of TIER 2 clipped,
  boxes overlapping. Rebuilt as three bands: canaries → record + its two projections → a horizontal escalation ladder
  ending in the deliberately-not-built AUTO-KILL.
- **p05 solution** — the layered verification stack became a single full-width vertical cascade; the two side-boxes
  that overflowed the frame folded into band captions, and REOPEN-SPEC became a dashed branch band.
- **p09 solution** — the 3×4 memory matrix put each row's description inside one 126-unit cell, spilling across its
  neighbours. Cells now carry the lifetime (which is what varies), the description spans the row beneath it.
- **p07 solution** — SPAWN GATE pill on two lines; helper boxes widened; the brief/report flow labels moved out of
  the spoke lines into a legend band below the figure.
- **p06 solution** — the SQLite cylinder was narrower than its own row labels (a `path`, so the first detector missed
  it); widened, with the fencing/idempotency pills moved clear.
- **p13 impact** — both gauges restructured to one shape: axis label above, three colour-coded zone labels in a row
  below. Labels no longer cross the arc bands.
- **p12** — see-saw pans re-sized for their labels; gauge value moved off the needle; the `≥50%` floor marker was
  being clipped by its own bar's `overflow:hidden` and never showed.
- **p01, p02, p08** — ledger/alarm annotations, provider-lane text and the nested-walls labels re-wrapped and re-placed.
- **`.flowrow` (framework)** — chains wrapped mid-sequence and orphaned their `→` arrows; now `flex:1 1 0` keeps a
  chain on one line and stacks it cleanly under 700px. Affects every chapter that uses one.
- **`.p01-cur` (framework)** — the two dials rendered side-by-side inside a narrow rail step and hung outside it;
  now stacked.

Two findings are intentional and must stay: the `✕` across p08's 6-unit firewall bar, and p02's two cross-fading
`p02-swapA`/`swapB` label layers (which sit on top of each other by design).

## v2 (2026-07-27) — what changed and why

Operator finding: the deck contained false reasoning, and **older studies were being presented as current best
practice** where newer work had moved the answer. `EVIDENCE-AUDIT-2026-07-27.md` is the full row-by-row audit against
`Research/NN-*.md` — 11 stale-as-current claims (A1–A11), 12 reasoning/scope defects (B1–B12), and the four chapters
that were verified sound (p01, p02, p06, p08 — do not "fix" those).

Three structural additions, all documented in `README.md`:

1. **Four-beat spine** (`.beats`) after every chapter's `.who` chips — ① the problem as of 2026 · ② what it costs ·
   ③ the fix · ④ what the fix buys. The impact/solution/before-after graphics are numbered ②③④ to match.
2. **Inline citation chips** (`.ref`) on every load-bearing claim — report + §, primary source + date, what it actually
   says, status tag, and scope/correction note. 251 status tags across the deck.
3. **Per-chapter evidence tables** (`.evb`, one per chapter before the `.srcline`) — the research-paper apparatus.
   Plus `.hist` "how we got here" bands where an old study was demoted, and `.corr` inline scope notices.

**The rule that must not drift:** old evidence stays visible as history so the reader sees how the field arrived here,
but the **2026 result always sets the recommendation**. When re-verifying, if a newer study lands, promote it and move
the current one down a band — never delete.

⚠ **Do not rename the status-tag classes to `cur`/`hist`** — they collide with `.p01-term .cur` and the `.hist` band.
They are `evnow` / `evpast`.

## What exists

| Path | What it is |
|---|---|
| `index.html` | The deck. 6,279 lines, single self-contained file. Hero → `squeeze` → `team` (4 archetype vignettes) → chapters `p01`–`p13` → `blueprint` → `close`. |
| `README.md` | Serving instructions, controls, deep links, the 7 operator content rules. Keep rules for ANY edit. |
| `_build/` | Chapter fragment sources (`chNN.html`, `blueprint.html`, `close.html`) + `qa-*.html` screenshot-offset copies. Reference only — **already spliced in**; safe to delete (a `rm -rf` was permission-denied during the build, so it was left). |
| `qa/` | `run-textfit.sh` + `textfit.js` — the graphics text-fit audit added in v2.1. Run after any SVG edit. |
| `~/.claude/plans/lovely-percolating-emerson.md` | The approved build plan: chapter content digests (per research report), the 60-graphic storyboard, the **full consistency contract**, QA plan. The authoritative spec for this deck. |
| Memory `research-presentation-deck` | Durable summary + pointers (auto-memory dir). Old product deck (`Presentation/`, :8080) is a SEPARATE deliverable — never merge them. |

## The one invariant that must never drift

**This deck sells the operator's research and skills to their employer. Sinet is only "the worked example (Sinet)" /
"the reference build" — never a product pitch.** The employer plans to build its own internal tool; the blueprint
section speaks to that.

## Open items (in priority order)

1. **Real-browser animation pass — the only unfinished QA.** Headless shots freeze CSS keyframes at frame 0, so the
   ~60 keyframe animations were never seen running (transitions + layout WERE verified). Needs a session with `/chrome`
   enabled (browser extension wasn't connected on build day): step through `?go=team`, `?go=p01`…`p13`, watch each
   `.g30`, check `read_console_messages` for JS errors. Fix anything janky; contract says animations are `pNN`-prefixed
   and `.in`-gated.
2. **Optional: systemd unit for :8081** (offered to operator, no answer yet) — mirror the `sinet-presentation` user unit
   that serves the old deck on :8080. Until then: `python3 -m http.server 8081` in this dir; phone access via
   `http://100.114.127.2:8081` (tailnet, host `sinet`).
3. **`_build/` cleanup** — operator's call (delete was denied once; don't retry without asking).
4. **Commit** — deck is uncommitted, like the old deck; committing is the operator's call. Note `Research/Presentation/`
   and `Presentation/` are both untracked.
5. **Graphics QA is now mechanical** — `./qa/run-textfit.sh` after any SVG edit; expect exactly 1 (intentional)
   finding. It measures fit only; it cannot tell you a diagram explains its point badly, so still look at it.
6. **Content maintenance** — as-of dates say July 2026. On vendor fixes: update the table row, never delete
   ("the pattern stays, the status changes"). Underlying URLs/access dates live in `Research/NN-*.md`.

## How to edit a chapter (the markers are gone)

Splice markers (`<!-- @CHNN -->`) were consumed during the build. To rewrite chapter NN now: replace the inner content
of `<div class="wrap">` between `<section … id="pNN">` and the next `<section`. Every chapter must keep, in order:
overline (`PROBLEM NN / 13 · NAME`) + h2 + lede → `.who` chips → `.explain` → five `.g30` graphics with
`data-el="impact|cause|solution|beforeafter|impl"` (exact `.gt` labels — copy from chapter p03, the exemplar) →
`.asof` banner → `table.tools` → `.why` → `.srcline`. `data-el` is machine-counted: **exactly 13 of each across the
file, chapters only**. Chapter-local CSS lives in a `<style>` block at the top of each section's content, classes
prefixed `pNN-`, keyframes `pNN…`, ending with a `prefers-reduced-motion` override. Numbers: only from the plan
digests or the source reports, units included; unmeasured → say so, never invent. No `S##`/`R##`/`D#` refs, no
external URLs, no `<script>` outside the one at file end.

## QA commands (run after any edit)

```bash
cd ~/Sinet-Agentic-Control-Hub/Research/Presentation
grep -o 'data-el="[a-z]*"' index.html | sort | uniq -c          # expect 13 × 5
grep -c 'class="beats reveal"' index.html                        # expect 13  (v2)
grep -c 'class="evb reveal"' index.html                          # expect 13  (v2)
grep -o 'class="st [a-z]*"' index.html | sort | uniq -c          # v2 status tags; none may be "st cur"/"st hist"
grep -o '@keyframes [a-zA-Z0-9]*' index.html | sort | uniq -d   # expect empty
grep -cE '\b[SRD][0-9]{2}\b' index.html                          # expect 0
python3 -c "from html.parser import HTMLParser
class P(HTMLParser):
    d=0
    def handle_starttag(s,t,a):
        if t=='div': s.d+=1
    def handle_endtag(s,t):
        if t=='div': s.d-=1
p=P(); p.feed(open('index.html').read()); print('div depth (expect 0):', p.d)"
```

Headless section screenshots (the `?go=` anchor races headless capture — use the offset trick instead):
make a copy of `index.html` with `<style>body{margin-top:-NNNNpx}</style>` injected before `</head>`, serve it, then
`google-chrome --headless=new --screenshot=out.png --window-size=1440,3000 'http://localhost:8081/copy.html?flat=1'`.
Full flat page is ~73,000 px tall. `?flat=1` also forces all `.reveal`/`.cnt` to final state.

## Build provenance (who wrote what)

Coordinator wrote: skeleton (CSS framework incl. `.fx2`/`.who`/`.tline`/`.cnt`, JS), hero, squeeze, `team`, exemplar
chapter `p03`, README. Subagent writers (fragments in `_build/`): ch01 + ch08 by **Opus-pinned** agents (source reports
09/10 are security-dense — keep routing deep-reads of those reports to Opus, see memory `fable5-safeguard-false-positive`);
ch02/04, ch05/06, ch07/09, ch10/11/12, ch13, blueprint+close by standard agents. Known post-splice fixes already applied:
receipt "amounts illustrative" caption (p01), "a year of research" → "systematic, fully-sourced research campaign"
(hero + close — the campaign actually ran July 2026, don't reintroduce duration claims).
