# Nexus frontend harvest — design-source survey (2026-08-04)

**Input #1 to the design-approach proposal** (B6 gate §9, D3 condition: "declare how to build them first"; sources directed by the operator: the Nexus frontend + other GitHub tools).

Produced by a read-only survey agent over `~/Nexus-Agentic-Coding-Setup` and `~/nexus-monitor`; **coordinator spot-checked before filing**: the four headline file sizes byte-exact (89,105 / 15,281 / 8,277 / 30,073), the `:root` token block verbatim (the `rgba(148,148,190,…)` violet neutral over `#07070d`), the `.bg-aurora` block at its cited line. The `app.js` structural claims were read but not independently re-verified — nothing load-bearing rests on them: the port treats view functions as *specs to rewrite*, never as code to copy.

---

## The survey

Five distinct frontends found across the two roots. Ranked by value to a React 19 + Vite + TS control-room restyle.

### FRONTEND 1 — Nexus Agent OS dashboard (the primary artifact)

**Paths:** `/home/sinep/Nexus-Agentic-Coding-Setup/app/static/`
- `style.css` — **89,105 bytes**, 1,738 lines, 787 rule blocks — *the design system*
- `index.html` — **15,281 bytes** — shell + inline SVG icon set
- `app.js` — **695,225 bytes**, ~11k lines — all views, vanilla JS, no framework
- `nexus3d.js` (6,963 B), `memory3d.js` (21,280 B), `jarvis3d.js` (69,028 B) — Three.js modules
- `vendor/threejsm/`, `vendor/lipsync/`, `avatar/*.glb` — self-hosted 3D assets

**1. Stack and serving.** Zero-build vanilla SPA. No bundler, no npm, no framework. Served by FastAPI (`app/server.py:11502`, index at `:11506`), uvicorn on `127.0.0.1:8777`, auto-HTTPS when certs exist (`server.py:11520-11543`). Launched via `~/.local/bin/nexus-up` → `Start Nexus` desktop entry. Live data: one WebSocket `/ws` (`server.py:1900`) + polling `tick()`; cache-busted query strings (`style.css?v=28`) are the whole "build system". Only external runtime deps: Chart.js 4.4.1 from jsDelivr and Google Fonts (CDN — Sinet must bundle instead).

**2. Styling approach.** Hand-rolled CSS, one file, one `:root` token block, **dark-only** (no light theme anywhere). Header names it: *"NEXUS Agent OS — Design System v3 · Glass panels over an aurora field · Inter UI + JetBrains Mono data"*.
- **Fonts:** Google Fonts CDN — `Inter` 400–800 (UI) + `JetBrains Mono` 400–700 (all numbers, IDs, timestamps, metadata). `font-variant-numeric: tabular-nums` on stat values and clocks.
- **Icons:** hand-drawn inline SVG, 24×24 viewBox, `stroke-width:1.6–1.8`, round caps, `fill:none`, `stroke="currentColor"`. 20 nav icons at `index.html:30-57`; 4 stat-tile icons (`NICO`) at `app.js:77-82`. Emoji only as semantic accents, never the icon system.
- No CSS framework, no utility classes, no preprocessor.

**3. Design language (concrete).** Tokens — `style.css:5-35` (copy verbatim):

```
--bg:#07070d  --bg-2:#0b0b15  --panel-solid:#12121d
--panel:rgba(148,148,190,.055)  --panel-2:rgba(148,148,190,.09)  --panel-3:rgba(148,148,190,.14)
--border:rgba(148,148,190,.16)  --border-l:rgba(148,148,190,.28)
--text:#eaeaf4  --text-dim:#9a9ab2  --text-faint:#5e5e78
--accent:#7c5cff  --accent-2:#5eead4  --cyan:#22d3ee  --accent-glow:rgba(124,92,255,.16)
--green:#4ade80  --yellow:#fbbf24  --red:#f87171  --blue:#60a5fa  --orange:#fb923c  --pink:#f472b6
--grad:linear-gradient(135deg,#7c5cff 0%,#22d3ee 100%)
--grad-soft:linear-gradient(135deg,rgba(124,92,255,.22),rgba(34,211,238,.12))
--sidebar-w:232px  --radius:14px  --radius-sm:9px  --shadow:0 14px 40px rgba(0,0,0,.35)
```

Key insight: the neutrals are **not gray** — panels/borders are a violet-tinted translucent `rgba(148,148,190,…)` over near-black `#07070d`. That single decision is why everything reads as one material.

- **Typography scale:** body 14px; page h1 19px/700; card titles 15px/700; body copy 12–13px; metadata 10–11px mono; section labels 9.5–11px 700 uppercase at 1.6–2.2px tracking in `--text-dim`; stat values 30px/700 mono. Logo 16px/800, tracking 4px.
- **Spacing/density:** content `26px 30px`; grid gaps 14px; card padding 17–20px; panel header `13px 18px`; nav items `8px 12px`. Density from small type, not tight boxes.
- **Layout skeleton (`index.html:15-93`):** flex shell, 232px blurred sidebar (`rgba(10,10,18,.72)` + `backdrop-filter:blur(18px)`) → blurred topbar → focus bar → scrolling content (body itself `overflow:hidden`). Sidebar: gradient logo mark → grouped nav (`Command`/`Intelligence`/`System`/`Insights`) → pinned assistant entry → footer with 4 live micro-meters + version.
- **Cards:** `linear-gradient(155deg, rgba(148,148,190,.075), rgba(148,148,190,.028))` + 1px `var(--border)` + radius 14. Hover: brighter border, `translateY(-2px)`, `--shadow`.
- **Stat tiles (`style.css:229-260`):** the signature — a **2px gradient hairline across the top** via `::before`, color-coded per variant. Head row = uppercase micro-label + 32px icon chip on `--grad-soft`. Value 30px mono tabular. Count-up on mount (`animateStatValues()`, `app.js:948`).
- **Tables:** `.data-table` (9.5px tracked headers, row hover) and CSS-Grid "tables" (`.roster-table`, `grid-template-columns:24px 110px 92px 1fr 130px 130px 84px`) whose cells hold real components.
- **Status family:** 8px glowing `.status-dot` (+`pulse` when live) · `.chip` pills, 7 variants, one formula `color:X; background:rgba(X,.09); border-color:rgba(X,.25)` · `.agent-status-badge` 9px mono uppercase · card-state top hairlines via `inset 0 1px 0 rgba(X,.25)`.
- **Meters, four kinds:** SVG donut gauges (r=52, dasharray, `drop-shadow(0 0 6px currentColor)`, .8s sweep; generator `app.js:1155-1178`) · 4px sidebar micro-bars with per-metric gradients · 6px budget bars with a `.hot` variant past 85% · a split token bar (in/out segments in one 5px track).
- **Empty states:** 63 in `app.js`. Pattern: centered, 38px padding, faint 26px glyph, **and a sentence explaining what will appear here** (e.g. "No pending approvals — agents will queue risky actions here for your sign-off.", `app.js:7056`).
- **Loading:** `.skel` shimmer in a `.skel-grid`.

**4. UX patterns worth copying.**
- **Live board** (`app.js:1223-1338`; CSS `:366-431`): 5 columns, headers = glowing dot + tracked title + count pill + contextual bulk action per column. Cards carry a **left priority rail** (3px, glowing at P0) and a live chip-strip footer (run state, model, tokens, verdict, cost, per-card stop). Drag-over = accent border + glow.
- **Task detail** (`app.js:1453-1569`): one modal = form + telemetry + **state-computed action bar** (Dispatch/Stop/Retry/Create PR/Follow-up render only in the states that allow them).
- **Approval flow** (`viewAgentic()`, `app.js:7021-7141`) — the most transferable pattern: each row = what's being approved, provenance line (`agent · risk: N · 4m ago`, mono), **a plain-language instruction of what to check first**, a jump to the full deliverable, then Approve/Reject. Pending count doubles as a red glowing sidebar badge.
- **Dashboard hierarchy** (`app.js:1025-1123`): hero → 4 KPI tiles → 2-up gauges/fleet row → full-width live roster → activity feed. `updateDashboardInPlace()` patches text nodes on tick — no flicker, no lost scroll.
- **Diff/review viewer** (`style.css:1383-1468`): PR-style file list, unified/split toggle, gutters, hover "＋" comment buttons, threaded line comments; server-side Pygments tokens themed in CSS (`.k #c792ea`, `.s #a5d6a7`, `.c #6b7a99` italic, `.nf #82aaff`, `.nc #ffcb6b`, `.o #89ddff`).
- **Global focus context** (`:1359-1381`; `app.js:1852-1944`): violet-tinted breadcrumb bar with dismissible `Project › Workflow` chips scoping every view.
- **Guided tours** (`:1275-1297`): spotlight via a 4000px box-shadow + a 360px card; `?` starts a per-view walkthrough.
- **Toasts** (`:634-653`) top-right, color-coded left border, blurred, max 4. **Right drawer** (`:588-632`) 520px, tabbed. **Connection pill** + live clock in the topbar (green pulsing `LIVE`, red on WS drop).

**5. Literally copyable (pure CSS, framework-agnostic).** Full design system `app/static/style.css` (89,105 B); within it: tokens `:5-35`, aurora `:48-62`, buttons `:171-214`, panels/cards/stat-tiles `:216-260`, chips+status `:165-168`/`:350-364`/`:459-469`, kanban `:366-431`, modal/drawer/toast/skeleton/empty `:549-667`, tables `:674-688`, diff viewer `:1383-1468`, mobile pass `:1669-1712`. Plus the 20-icon SVG set (`index.html:22-57`), the gauge generator (`app.js:1155-1178` — trivially a TSX component), Chart.js dark theme options (`app.js:5305-5341`).

**Mismatch — do not port directly:** all of `app.js` is template-string `innerHTML` + global `onclick` handlers + module-level mutable state; every view is a string-returning function. **Read them as specs, rewrite as components.** The `*3d.js` Three.js modules own their canvases imperatively. `.badge` is defined twice with different meanings; `.panel`/`.stat-card`/`.data-table`/`.activity-item`/`.section-title` collide across Frontends 1/2/3 — never merge the CSS files blindly.

**6. Why it out-classes a bare hand-rolled UI** — six mechanisms, in order of contribution:
1. **The violet-tinted neutral** — one translucent material over one near-black; surfaces relate instead of stacking.
2. **A living backdrop** — `.bg-aurora`: four radial gradients at 5–13% opacity drifting (26s alternate) behind everything; dark mode reads as depth, not absence.
3. **Chrome that signals depth** — `backdrop-filter` blur tiers (18px shell / 8px overlays / 6–12px chips) + two shadow tiers.
4. **One gradient as identity** — `#7c5cff → #22d3ee` on logo, primary buttons, active nav, tile hairlines, fills, avatars — plus a matching violet glow under primary buttons.
5. **Typographic discipline** — Inter for prose, JetBrains Mono for *every* number/ID/timestamp with tabular numerals; tiny tracked uppercase labels against 30px mono values = the instrument-panel feel.
6. **Motion as feedback, never decoration** — 350ms view fade-up; pulse only on genuinely-live dots; shimmer skeletons; hover lifts; .8s gauge sweeps; overshoot modals — every animation attached to a state change.

Plus **glow-as-semantics**: `0 0 8px <statuscolor>` on dots, `0 0 12px` on running badges, red `0 0 10px` on the pending-approvals badge — severity readable peripherally.

### FRONTEND 2 — Nexus Monitor live dashboard

**Paths:** `/home/sinep/nexus-monitor/static/` — `index.html` (1,869 B), `style.css` (**8,277 B**, 442 lines), `app.js` (25,846 B). FastAPI-served, Chart.js + Google Fonts CDN, 6 views, polling.

Same palette as Frontend 1 in a **flat opaque variant** — no glass, no gradients, no aurora: `--panel:#11111d --panel-2:#161624 --border:#1e1e2e --border-hi:#2a2a40`, identical accents, same Inter/JetBrains-Mono split, dark-only. Active nav = 2px inset left rail. Worth copying: **threshold-driven color** (`barClass(pct)`: ≥90 red, ≥75 yellow, else green — meters self-annotate); the `statCard(label, value, unit, sub, pct, barColor)` helper signature; alert pills promoted into the topbar; a pinned `READ-ONLY` capability badge (good for privileged-mode display). The Chart.js dark theme (`static/app.js:148-158`: mono ticks, `#1e1e2e` grid, `maxTicksLimit:6`) and line-series recipe (`:133-145`: `borderWidth:1.5, pointRadius:0, tension:.3, fill:true, backgroundColor:color+'15'`) are directly reusable. **This file is the best starting point for a flat-surface variant** — small, complete, zero coupling. It also proves the token set survives without `backdrop-filter` — and the visible delta to Frontend 1 is exactly the six-mechanism list above.

### FRONTEND 3 — Stability Monitor static report

**Path:** `/home/sinep/nexus-monitor/dashboard.html` — **30,073 B**, self-contained (CSS inline `:7-323`; also embedded as a string in `server.py:15`, port 8999). Tokens: `--panel:rgba(20,20,35,.72)`, **accent-tinted border** `rgba(124,92,255,.15)` (a touch the main app lacks), slate text family, system font stack. No shell — a document: header with verdict → auto-fit `minmax(340px,1fr)` panel grid.

Best components: `.alert` (`:201-244`) — **the strongest single component in the harvest**: icon + title + description on `rgba(sev,.08)` tint with `rgba(sev,.2)` border, four severities. `.service-item` (`:169-199`) — status row with a 3px colored left border. `.bar-container` (`:135-161`) — labeled meters with **two-stop gradient fills** (green `#22d3ee→#4ade80`, yellow `#facc15→#fb923c`, red `#fb923c→#f87171`) that read far better than flat fills. Panel titles with a health-carrying dot. Information hierarchy worth copying wholesale: **alerts first (sorted by severity, each with concrete numbers and the remediation command inline) → resources → services → tables** — "what's wrong → why → what to type" is directly applicable to an incident surface. Entire style block copyable, no JS.

### FRONTEND 4 — Monitor v1 static report

**Path:** `/home/sinep/nexus-monitor/index.html` — 17,702 B, self-contained. Neutral-white glass variant (`--panel:rgba(255,255,255,.04)`) with a Tailwind-500 semantic triad (`#ef4444/#f59e0b/#22c55e`). Two one-liners with high return: a **gradient-clipped headline** (`background-clip:text`) and a **section title with a 3px accent bar** via `::before`. Also `.severity` pills and a `.finding` row (big mono ordinal + title + detail) good for findings/critic lists.

### FRONTEND 5 — Architecture schematic

**Path:** `/home/sinep/Nexus-Agentic-Coding-Setup/docs/ARCHITECTURE.html` — 33,968 B, static. **The only light-mode-capable artifact and the only proper theming contract in the harvest**: three parallel `:root` blocks — light default, `@media (prefers-color-scheme: dark)`, and explicit `[data-theme]` overrides (`:3-33`):
- Light: `--bg:#F2F5F9 --panel:#FFFFFF --panel2:#E9EEF5 --ink:#182432 --muted:#5B6C80 --line:#CBD6E2` + accents `#0B7D93 #2160C4 #8F6508 #1F7D4D #6D4FC9 #C24747`
- Dark: `--bg:#0A101C --panel:#111A2B --panel2:#0D1524 --ink:#E4ECF7 --muted:#8DA2BB --line:#243349` + accents `#43D6EC #7AB0FF #EFC368 #67D9A0 #B69CFF #F08484`

Accents are **darkened for light and lightened for dark — a paired ramp, not one palette forced into two backgrounds**. Also: `prefers-reduced-motion` kill-switch, `:focus-visible` outlines, a CSS-only starfield masthead, `.eyebrow` mono kicker, `clamp()` display type with `text-wrap:balance`, a 3-column sticky-rail layout (`158px / minmax(660px,1fr) / 218px`) that fits a run-detail page (lifecycle rail left, ops rail right), numbered step rails with `::before` connectors. Everything copyable. **If the SPA needs light mode, this file is the reference, not `style.css`.**

### Also present (not design sources — noted so they're not mistaken for one)

Installer payload snapshot (`files/nexus/static/`, `?v=5`); three git-worktree copies of `style.css` ~2.5 KB behind HEAD; agent-produced webshop/site deliverables under `app/workspaces/` and `benchmarks/` (Next.js/Astro/Tailwind — agent output, not Nexus UI); creative-skill HTML scaffolds under `setup/hermes/skills/`.

### Recommended harvest order for the restyle

1. Port `style.css:5-35` as `tokens.css` — verbatim, including the `rgba(148,148,190,α)` neutral family.
2. Add `.bg-aurora` as a single fixed div in the app shell.
3. Port the primitives (buttons, panel, stat-card, chips, status dots) as the first component set.
4. Rebuild the shell from `index.html:15-93` — grouped nav with inline SVG icons, blurred topbar, connection pill, sidebar micro-meters.
5. Take overlays wholesale: modal, drawer, toast, skeleton, empty.
6. Use `viewAgentic()` as the spec for the approvals surface and `viewKanban()`/`taskCard()` as the spec for the live board.
7. If light mode is required, layer `ARCHITECTURE.html:3-33`'s dual-ramp contract over step 1 rather than inventing light values for the violet-glass palette.
