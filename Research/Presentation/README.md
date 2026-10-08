# AI That Pays Off — evidence briefing for IT leadership

`Sinet-Research-Presentation.html` (renamed from `index.html` on 2026-07-28; published on GitHub Pages at
`darian-gajgic.github.io/agentic-control-hub-research/Sinet-Research-Presentation.html`) is a single self-contained
page (no build step, no network requests, works offline):
a problem-led marketing/sales presentation of the Sinet research campaign for the operator's employer.
It sells the **research and the operator's engineering judgment** — Sinet appears only as the worked example,
never as a product pitch.

## v4 reading layout (2026-10-07) — same content, re-structured for reading

Readers found v3 "interesting but very hard to read": ~45,000 words (≈3¼ h) in one undifferentiated scroll,
87% of the text at ≤13.5px, 22% in monospace caps, no short path. v4 changes **structure and style only** —
every text node of v3.5 survives (machine-checked, see QA). Design rules come from a sourced research pass
(NN/g, GOV.UK, WCAG 2.2, Butterick, Distill/IPCC/OWID exemplars); the full brief is summarised in `HANDOFF.md`.

Reading layers, top to bottom:
1. **At a glance** (`#glance`, after the hero) — the 13 problems one line each (reuses the close-recap cards), plus
   three reading paths (skim / read / check). BLUF: the whole argument in two minutes.
2. **Per chapter**: header (overline, claim headline, lede, who-feels-it chips, reading time + claim count) →
   **"The chapter in four steps"** (the four beats as a vertical summary list) → "In plain terms" → five figures,
   each `h3` heading followed directly by its **takeaway** (moved up from the bottom: action-title pattern) →
   fairness note → named-tool table (`#pNN-tools`) → why / quote → **evidence table collapsed in `<details>`**
   (`#pNN-evidence`, summary shows claim count + status-label counts) → sources line.
3. Verify layer: `ref` chips (unchanged apparatus) + the collapsed evidence tables + the status legend (`#legend`).

Chrome: fixed **left contents list** with scroll-spy (chapter + sub-section highlight, "seen" chapters marked),
reading-progress bar, light/dark switch (default follows the OS; light = role-inverted palette, generated), a
**Pause motion** switch (WCAG 2.2.2; mirrors every `prefers-reduced-motion` block via `html.calm`), "Top". Below
1100px the list becomes a drawer behind a sticky "Contents" bar. Sections are a document now (no 100vh slides);
Space/PageUp/PageDown scroll natively, ←/→ still jump section to section for presenting.

Type: body 18px, prose ≤68ch, card/pane text 16px, labels 12.5–13px sans caps (monospace kept for numbers and
terminal mock-ups), all text ≥4.5:1 in both themes (`qa/contrast.py`). Step rows inside half-width panes stack
vertically; implementation rails are vertical step lists; the provider timeline is vertical; callouts are sized
to their text (`--callout`). Chapter-local CSS font sizes 10–13.5px were scaled up (+1 to +2.5px) at build time;
SVG text was not touched (its boxes are hand-measured — textfit still shows only the intentional p08 ✕).

## Structure (v3 description — element order above supersedes the list below)

Hero → **01 The squeeze** (business case: included-usage → per-token metering) → **02 Meet your team**
(four employee-archetype vignettes: HR / Marketing / Research / Development — today's naive agent builds vs the
corrected patterns) → **thirteen problem chapters**, each with the same seven elements:

1. animated impact graphic (with a "for your employee / for your company" pair)
2. animated cause graphic (tools and vendors named, dated)
3. solution graphic (what the research supports)
4. before/after graphic incl. an honest-downsides band
5. implementation rail
6. named-tool comparison table + detail prose (side-by-side: today's tools vs the researched solution)
7. "In plain terms" explainers for non-technical readers

→ **The blueprint** (what building such a platform takes) → **Close** (13-problem recap + the knowledge pitch).

Chapters: 1 Skills, not personas · 2 Intake · 3 Context · 4 Swarms · 5 The right model · 6 Verification ·
7 Machinery · 8 Security · 9 Memory · 10 The local tier · 11 Providers · 12 The bill · 13 Flying blind.

**Chapter order changed twice on 2026-07-27** — resequenced by operator judgment of importance, so the chapters
that decide whether an agent programme works at all lead, and the cost/plumbing chapters follow. v3.1 did the
big move; **v3.2 rotated chapters 2–5 into the operator's final order** (`Right model · Swarms · Intake · Context`
→ `Intake · Context · Swarms · Right model`). Both times the section ids, chapter-local CSS prefixes (`.pNN-*`),
comment banners, evidence-table captions, nav, team rails, close recap and the visible numbers were renumbered
together, so **`pNN` always equals the printed chapter number**, and the `sec`/`sec alt` band alternation follows
position, not chapter.

**`HANDOFF.md` and the two `EVIDENCE-AUDIT-*.md` files predate both reorders — every `pNN` in them is an original
(pre-v3.1) number.** Translation (original → current): 1→12 · 2→11 · 3→3 · 4→2 · 5→6 · 6→7 · 7→4 · 8→8 · 9→9 ·
10→13 · 11→1 · 12→10 · 13→5. If you are reading something written against v3.1 numbering instead, only four
chapters moved: 2→5 · 3→4 · 4→2 · 5→3.

Also fixed in v3.2: all thirteen `EVIDENCE TABLE — CHAPTER NN` captions still carried their original numbers
(v3.1 renumbered everything else but missed them). They now match the printed chapter number.

## Serving / presenting

```bash
cd ~/Sinet-Agentic-Control-Hub/Research/Presentation
python3 -m http.server 8081     # → http://localhost:8081  (8080 stays with the older Sinet sales deck)
```

Controls (v4): native scrolling (Space/PageDown scroll one screen) · `→`/`←` jump to the next/previous section ·
`f` fullscreen · `Esc` closes a citation or the contents drawer · left contents list jumps anywhere (the right-edge
dots and the horizontal top nav are gone).

**Fixed 2026-07-27** — the right-edge dots tracked position with an IntersectionObserver at `threshold:.4`.
Intersection *ratio* is measured against the target's own height, and these chapters run 3,000–6,000px against
a ~900px viewport, so their ratio tops out around 0.15–0.3 and the observer never fired for them: only `hero`,
`squeeze` and `close` could ever light, so scrolling down froze the marker on the second dot (and `active`
drives the ←/→ keys, so those stuck too). Position is now tracked by whichever section last crossed a line 40%
down the viewport, computed in a rAF-throttled `scroll` handler — height-independent. **Any future scroll-spy
here must not depend on intersection ratio.**

Deep links: `?go=p07` or `#p07` opens at a chapter (`glance`, `squeeze`, `team`, `p01`–`p13`, `blueprint`, `close`);
v4 adds `#pNN-impact|cause|solution|beforeafter|impl|tools|evidence` (an evidence link opens its collapsed table).
`?flat=1` renders every graphic in its finished state and opens all evidence tables (screenshots, PDF print);
printing also opens them.

QA caveat: CSS *keyframe* animations freeze at frame 0 in headless screenshots (transitions render fine) —
verify animated pieces in a real browser.

### Graphics QA — run after touching any SVG

```bash
./qa/run-textfit.sh          # 1440px viewport; pass a width to test others, e.g. ./qa/run-textfit.sh 860
```

SVG text does not wrap and is clipped at the `svg` frame, so a label that outgrows its box breaks silently.
The script renders the deck headless and measures every `<text>` against the shape it sits in, its frame, and
its neighbours. **Expected output: exactly 1 finding** — the `✕` drawn across the 6-unit firewall bar in p08's
before/after graphic, which is deliberate. Anything else is a real spill, clip, or collision.

Rule of thumb when a label doesn't fit: **vertical space in these diagrams is free, horizontal space is not.**
Re-band the diagram (more rows, full-width bands) or wrap the label — don't widen the `viewBox`, which
shrinks every label on the page.

## Content rules (operator-set; keep for any edit)

1. NAME tools/vendors/models with what each does wrong, dated "as of July 2026", with the fairness banner.
2. Sources are named in **plain language** ("Sinet research report 07 — 'Context engineering', §2.2") — never bare
   `S##`/`R##`/`D#` codes. Since v2 every load-bearing claim also carries an inline citation chip (see below).
3. Every chapter: the four-beat strip + "In plain terms" box + the five graphics before the detail text.
   (v4: the four beats render as "The chapter in four steps"; the evidence table is collapsed, never removed.)
4. Explain the causal WHY, CEO-readable.
5. Business case + per-feature employee/company effects (the `.fx2` pairs).
6. **Sinet is the worked example, not the product** — the deck sells the research and skills.
7. Every number traces to a research report (`Research/NN-*.md`) or a measured run — invent nothing;
   if a vendor fixes something, update the table row, don't delete it (the pattern stays, the status changes).
8. **Currency rule (v2, the load-bearing one).** Never present an older study as the current state of the art.
   Where a finding has been re-measured, the **2026 result leads** and the older one moves into a
   `HOW WE GOT HERE` history band, labelled, with the newer result named beside it. Old work is shown as history so
   the reader understands how the field arrived here — never as the recommendation. Nothing is deleted.
9. **Scope rule.** A number may only be stated as broadly as it was measured. If a result came from 7–8B open models,
   a single model family, one benchmark variant, or a single vendor's internal eval, the text says so.
10. **No predecessor, ever (v3).** The predecessor platform (Nexus) is never named and never cited — it was an
    unrepresentative self-test run without proper setup, so it is not evidence. `grep -ci "nexus\|predecessor"` must
    stay **0**. If a point can only be made with it, the point does not go in the deck. Replacements for every point it
    used to carry are listed in `EVIDENCE-AUDIT-2026-07-27b.md` §A.
11. **Per-duty, both directions (v3).** Model-tier advice is never one rule. The evidence has a cheap model matching
    frontier on some duties and falling below chance on others, and a frontier model being *required in the checking
    seat* for plan critique and hard adjudication. Any tier claim names its duty, and chapter 5's three-band duty table
    ("The right model") is the canonical shape. Collapsing it back into a slogan is a regression, however tidy it reads.
12. **Caveats belong in the visible prose (v3).** A staleness or scope caveat that lives only inside a citation chip is
    invisible to a skimming reader — if a headline number is GPT-3.5-era, single-source, or trade press, the sentence
    around it says so, and the chip carries the detail.

## The evidence apparatus (added in v2, 2026-07-27)

Three components carry it — keep all three in sync when editing a claim:

| Component | Markup | Purpose |
|---|---|---|
| Citation chip | `<span class="ref" tabindex="0"><span class="rn">ref</span><span class="rbox">…</span></span>` | Inline, click/hover-expandable. Inside `.rbox`, in order: `<b>` report + section · `<i>` primary source + date · `<u>` what it actually says · status tag(s) · `<em>` scope, caveats, or the correction note. |
| Status tag | `<span class="st evnow\|evpast\|scope\|thin\|vend\|unm">` | CURRENT · HISTORICAL · SCOPE-LIMITED · THIN EVIDENCE · VENDOR-MEASURED · UNMEASURED. Defined in the reading legend at the end of the `squeeze` section. **Do not rename to `cur`/`hist`** — those collide with `.p12-term .cur` and the `.hist` history band. |
| Evidence table | `<div class="evb">` before each chapter's `.srcline` | Every load-bearing claim in that chapter: claim · research file + § · original source + date · status. This is the research-paper apparatus; the chips are the presentation layer over it. |

Plus `.beats` (the four-beat strip: ① the problem, as of 2026 · ② what it costs · ③ the fix · ④ what the fix buys) after
each chapter's `.who` chips, `.hist` history bands, and `.corr` inline correction/scope notices.

Page maintenance: bump the "as of" dates when re-verifying claims; the underlying URLs + access dates live in
`Research/NN-*.md`.
