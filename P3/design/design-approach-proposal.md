# Sinet UI — design-approach proposal (2026-08-04)

**Status: APPROVED 2026-08-04 — operator free-text answer recorded verbatim in §6. This document is now the UI batch's design authority, subordinate to the spec on all behavior.** *(Amended pre-approval 2026-08-04: §3 "Nexus functionality carry" + §4 seeded exit walk, on the operator's structure-and-functionality question.)* This is the declaration the operator's D3 condition requires ("we have to declare how to build them before we start"), under D8's ratified mandate (looks **and** intelligibility). Authority: B6 gate record `P3/gates/B6-report.md` §9 (D3, D8, findings A-1/A-2/C-1). Inputs: `nexus-frontend-harvest.md` · `ui-foundation-sourcing.md` · `odysseus-pattern-study.md` (all three in this directory). FC-v1's binding picks are untouched — this proposal adds a design layer; it changes no behavior contract.

Once approved (free text; recorded here + STATE), the UI batch builds against this document. Deviations found mid-build return here as dated amendments.

---

## 1. The look: Nexus's language, systematized

**The Sinet UI adopts the Nexus "Agent OS" design language** — the thing the operator points at when saying the old frontend looked good — carried over as a token set and pattern specs, not as copied files:

- **Base:** near-black `#07070d`, panels/borders in the violet-tinted translucent neutral `rgba(148,148,190,α)` — one material everywhere, never plain gray.
- **Identity:** the single gradient `#7c5cff → #22d3ee` for logo, primary buttons, active nav, stat-tile hairlines, progress fills — plus its soft-glow shadow.
- **Status hues:** green `#4ade80` / yellow `#fbbf24` / red `#f87171` / blue `#60a5fa` / orange `#fb923c` / pink `#f472b6`, always via the one chip formula (`color X, bg rgba(X,.09), border rgba(X,.25)`) and glow-as-semantics on live/severe things.
- **Type:** **Inter** for prose, **JetBrains Mono for every number, ID, timestamp and status token** (tabular numerals); tiny tracked uppercase section labels; 30px mono stat values. Both fonts **bundled** via Fontsource woff2 — never a CDN (Nexus and Odysseus both used Google Fonts CDN; we must not).
- **Depth:** the aurora backdrop (one fixed div, four drifting radial gradients) + two blur tiers (shell 18px, overlays 6–12px) + two shadow tiers.
- **Motion only as feedback:** view fade-up, pulse on genuinely-live dots, shimmer skeletons, hover lifts, gauge sweeps — all behind `prefers-reduced-motion`.
- **Dark-first at v0.** Nexus's language is dark-native and dark is the operator's lived preference. The token architecture (CSS variables on `:root`, a class-switched theme root) keeps a light ramp addable later — `ARCHITECTURE.html`'s dual-ramp contract is the filed reference — but no light theme is built in this batch.
- **Theme discipline borrowed from Odysseus:** the theme surface stays minimal (a handful of root tokens; derived values via `color-mix()`), and density (compact/comfortable) ships as root classes.

Mechanically, the Nexus values are poured into **shadcn's token names** (`--background`, `--border`, `--muted-foreground`, `--primary`, …). That single mapping themes the shadcn primitives, our hand-written JSON Forms renderers, react-diff-view, **and @assistant-ui/react — which natively assumes those very tokens** — from one place.

## 2. The substrate: Tailwind v4 + shadcn/ui copy-in (Base UI track)

Per the sourcing research's ranked recommendation (all pins re-verified live at adoption through the rail; research-time versions in `ui-foundation-sourcing.md`):

- **Dev-time:** `tailwindcss` 4.x + `@tailwindcss/vite`, the `shadcn` CLI (generates code; never a runtime dep).
- **Runtime (small, pinned):** `@base-ui/react`, `lucide-react` (tree-shaken SVG icons), `clsx`, `tailwind-merge`, `class-variance-authority`, `tw-animate-css`, `@fontsource-variable/inter`, `@fontsource/jetbrains-mono`.
- **shadcn components arrive as vendored TSX in our tree — code we own.** Upstream churn (including the July Radix→Base-UI transition) cannot touch shipped components. This is the vendoring posture the platform prefers; the adopted-engine rules apply only to the small primitive deps, which get `components.lock` entries like everything else (npm lockgate already covers the tree).
- The existing 1,409-line `index.css` migrates progressively: tokens land first and coexist; each surface sheds its old CSS as its pass lands; the file ends as tokens + the few bespoke pieces (aurora, gauges) Tailwind shouldn't own.
- **Nexus's inline-SVG icon language** (24×24, 1.6–1.8 stroke, round caps) is compatible with lucide's style; where a Nexus icon is better, we redraw it as our own component.

**Bundle honesty:** fonts + primitives will grow the bundle (current 976.99 kB / gzip 292.96). Measured at every packet landing; no silent growth. The SPA stays un-split (embedded, tailnet-served) unless measurement says otherwise.

## 3. The pattern vocabulary

What the surfaces are made of, merged from the three inputs (Odysseus items are reimplementations of ideas — never its code):

**From Nexus (look + control-room anatomy):** stat tiles with color-coded gradient hairlines and count-up; glowing status dots + the one chip formula; left priority rails on cards; state-computed action bars (verbs render only in states that allow them); the approval row anatomy — what's being approved, mono provenance line, **a plain-language "what to check first"**, jump-to-deliverable, then the verbs; grouped sidebar nav with live micro-meters; connection pill + clock; focus-context bar (dismissible scope chips); toasts/drawer/modal/skeleton/empty-state kit; CSS-grid tables whose cells hold real components; the diff-viewer chrome around react-diff-view.

**From Odysseus (behavior vocabulary, reimplemented):** the **tool-call timeline rail** for run/task detail — each step a dot-on-rail node with uppercase name, elapsed timer, collapsed live-output tail, and **visually distinct recovery/stop nodes** (maps directly onto our recovery-ladder and watchdog events); **never stall silently** — step caps, budget stops, fallbacks and takeovers each get an inline banner or "Continue ▸" affordance (matches our no-silent-caps invariant); the **threshold donut ring** (green/orange/red) as the one capacity-meter idiom (context, budgets, VRAM), with one-metric footers and click-through stats popovers; **background-run etiquette** — parked streams, sidebar notification dots, completion toasts; served-model honesty in role labels (we already record requested-vs-served; the UI now says it); density classes; the Escape-stack and shortcut discipline.

**The self-teaching layer (the A-1/A-2 fix, first-class scope):** every surface carries a one-line "what this is" header affordance; empty states teach (Nexus's sentence pattern: *what will appear here and why*); **an interactive do-the-thing tour** (spotlight steps that advance by clicking the real control) plus **one-shot first-use hints**; the assistant's disambiguation card gets grouped, plain-language rendering with its three-verb contract stated in operator words (and the degraded-mode reason written for humans: "no local model is wired, so I can't read sentences yet — pick a query"); the C-1 fix (login picker reachable in dev posture; Sign out hidden when it cannot work).

**Already binding, unchanged:** the carried S13/S15 behavior specs (review anchors/lifecycle, stream-survives-navigation, error humanization, file exchange), all FC-v1 picks, every honesty invariant (no fabricated numbers, honest absence fails loud, content-vs-telemetry).

**Nexus functionality carry — three layers** *(added 2026-08-04 on the operator's question "not just the style — also the structure and the good functionalities")*:

1. **Already carried into the frozen spec and SHIPPED** — the hard-won Nexus functionality was resolved into binding behavior specs during the campaign (the operator's own FC-v1 conditionals) and built in B6: the **entire Review v2 loop** — **side-by-side diff as the review default** (verified in shipped code: `web/src/Deliverable.tsx:68-70`, `split` default with unified toggle), round-over-round revision comparison with lineage navigation, anchored comments with ±2-line drift tolerance and the orphan state, the findings→retry drain, images visual-compare, PDF extracted-text diff, try-it dual-iframe before/after, one-click accept; the **Jarvis-tab chat behaviors** — stream-survives-navigation, the error-humanization table, drag-drop/click-browse file exchange with produced-files chips, server-side sessions, first-message auto-titling; the **approval-card contract** (agent-inbox interrupt schema). *The operator has never seen most of this because the click-through database was empty and those routes need data to exist — see the seeded exit walk below.*
2. **Structure carried by this proposal (§3 above):** the Nexus dashboard hierarchy, board anatomy, state-computed task-detail action bars, approval-row anatomy, focus-context bar, guided tours, the tile/drawer/toast kit.
3. **Parked v1+ by prior ratified decision, NOT in this batch:** the 3D showpieces (avatar hologram `jarvis3d`, memory-galaxy backdrop `memory3d`) and conversation mode / STT / TTS — parked behind the registered benchmark gate (operator re-affirmed valuing them 2026-07-18; FC-v1 §3). That gate is registered in BENCH-REG and changes only via its own §17 — this proposal neither smuggles them in nor drops them.

## 4. The work plan (packets, standard four-stage pipeline)

1. **UI-1 Foundation:** substrate adoption (lock entries, pins re-verified), token mapping (Nexus values → shadcn names), fonts/icons bundled, aurora + shell restyle (sidebar/topbar/nav), the primitive kit (button, panel, chip, dot, tile, toast, modal, drawer, skeleton, empty), density classes. Existing tests stay green; markup-coupled assertions updated honestly (assertion counts never drop silently — the B6-9 discipline).
2. **UI-2/3/4 — the fourteen controls (D3), in the ratified order** cancel → pause → budget editor → benchmark opt-in → memory family → history ask/search — each built in the new language on the new primitives, plus the C-1 login fix, D5 timestamps (relative-beside-UTC on live surfaces, UTC-only in audit), and the two cosmetic S00.9 amendments.
3. **UI-5/6/7 — the D8 pass over the eleven existing routes** (mission control + board; task detail with the timeline rail; fleet + workforce; inbox; settings; chat; review surfaces; login/push), including the self-teaching layer and the assistant-card rework.
4. **Exit:** full battery + bundle measurement + an operator click-through of the restyled app — **on a seeded demo world** *(added 2026-08-04)*: the click-through script gains a dev-only seed (derived from the golden-fixture producers — tasks, runs, a deliverable with revisions and anchored comments, meter readings, an approval card) so **every** surface is walkable, including task detail and the review surface the empty database has kept unreachable. The seed exists only in the throwaway posture, never in production. → then the D6 upgrade ceremony → bring-up.

Packet count is a cut-time decision (likely 6–8); each runs grounding → executor (Opus) → evaluation → drain as always, with this document + the three input files as the grounding's read-first set.

## 5. Binding constraints

Bundled assets only, no CDN, no runtime fetches · escape-scan rules unchanged (allowlist stays EMPTY) · **AGPL boundary: Odysseus is ideas-only, forever** · permissive licenses only, every dep pinned + locked · FC-v1 picks untouched · behavior-preserving: API contracts, events, and scoping do not change in this batch (the C-1 fix and assistant-card rework are the sanctioned exceptions, each already recorded as findings) · every ⚙ number stays registry-routed · the spec stays silent on visuals, so this document (once approved) is the design authority for the batch, subordinate to the spec on all behavior.

## 6. What the operator decides

1. **Approve the look:** Nexus violet-glass language, dark-only at v0. *(Recommended: yes.)*
2. **Approve the substrate:** Tailwind v4 + shadcn copy-in + bundled Inter/JetBrains Mono/lucide. *(Recommended: yes.)*
3. **Approve the pattern set + work plan** (§3–§4), including the self-teaching layer as first-class scope. *(Recommended: yes.)*
4. Any taste overrides (aurora on/off, glass intensity, density default, anything you want different) — free text.

**Operator answer (recorded verbatim on approval):** **APPROVED 2026-08-04** — *"ok than I aprove all design decisions."* Coordinator reading (logged in STATE): items 1–3 approved as recommended (the look · the substrate · the pattern set + work plan with the self-teaching layer first-class); item 4 — no taste overrides given, so the proposal's stated defaults stand (aurora on, glass and motion as specified, density default = comfortable with the compact root class shipped).
