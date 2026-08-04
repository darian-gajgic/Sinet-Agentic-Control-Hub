# Odysseus frontend study — pattern source (2026-08-04)

**Input #3 to the design-approach proposal** (operator directive 2026-08-04: "check … this repository"). Surveyed at `odysseus-dev/odysseus` (84.7k★, pushed 2026-08-04, self-hosted AI workspace).

**LICENSE BOUNDARY (binding): Odysseus is AGPL-3.0 — STUDY/PATTERN source only.** Ideas and observations below are reimplemented in our own code; no Odysseus code or CSS is ever copied into Sinet (permissive-only lock policy; AGPL derivation would encumber the codebase). The survey agent was instructed accordingly and the report contains descriptions and plain facts only; the survey clone was deleted after reading.

## 1. Where the UI lives, stack, size, serving

The entire web UI is `static/` at repo root: `index.html` (~2,540 lines), `style.css` (~41,000 lines, one file), `app.js` (~4,700 lines), plus `static/js/` — ~132,000 lines across ~90 native-ES6 modules. **No framework, no build step**; vendored client libs (highlight.js, xlsx, mammoth, docx, html2pdf, qrcode); self-hosted fonts; FastAPI serves it (uvicorn :7000) with server-injected CSP nonces; a service worker precaches the shell (installable PWA). `static/js/MODULE_SUMMARY.md` is an authored architecture map of the frontend — a practice worth imitating. State: localStorage for UI prefs, server as source of truth, a global fetch patch routing 401 → /login.

## 2. Design language (facts)

- **Palette (dark default):** `--bg #282c34` (One-Dark gray), `--fg #9cdef2` (pale cyan), `--panel #111`, `--border #355a66`, accent `--red #e06c75`; support `--green #50fa7b`, `--warn #f0ad4e`; semantic layer (`--color-error #ff4444`, `--color-success #4caf50`, `--color-accent #00aaff`, `--color-agent-active #00ff00`); One-Dark syntax hues via `--hl-*` tokens.
- **The standout theming model: a named theme is exactly 5 tokens** (bg, fg, panel, border, accent); 16 presets ship; everything else derives via `color-mix()`. Light mode is a `:root` class applied pre-paint (not media-query-driven). Users save up to 8 custom themes, import/export as text, and generate palettes from one accent with a color-harmony tool. Plus: background-pattern layer, frosted-glass toggle, font choice (Fira Code default; Inter; OpenDyslexic), **3 density levels** as root classes, text-size select.
- **Typography:** Fira Code mono app-wide (300/400/600), all self-hosted woff2.
- **Layout:** icon-rail (16px stroke icons) ⇄ resizable collapsible sidebar (brand, new chat, Ctrl+K search, drag-reorderable sections, user bar pinned bottom) ⇄ main chat container. Chat header: thin overlay with title, message count, **session cost badge**, **context-usage ring pill**, export dropdown.
- **Cards over tables:** zero `<table>` in app chrome — every list is a status-classed card/row (`queued|running|done`) with a title line, muted meta line, inline actions.
- **Chat:** role labels include the *actual* model routed to; slim message footer shows exactly one metric (tok/s, else cost, else time) opening a full stats popover on click; sources/findings boxes.
- **Meters:** one idiom — a **14px SVG donut ring + percent** with fixed thresholds (green <70, orange 70–85, red ≥85) reused everywhere (header pill, message footers, menu items). Cost renders with adaptive precision. Two progress motifs only (wave glyphs, whirlpool spinner).
- **Empty states:** tiny line-drawn face icons + one sentence; deliberately none where an input above is the call to action.
- **Motion:** heavy but gated — 18 separate `prefers-reduced-motion` blocks.

## 3. Patterns most worth reimplementing

- **Agent runs as a timeline rail:** tool calls render as nodes on a 2px vertical accent rail between chat bubbles — dot punched on the rail (pulsing halo while running), uppercase tool name, live elapsed timer, collapsible live-output tail, done/failed mark; a "synapse" spark travels the rail while streaming. **Recovery/supervisor steps are visually distinct** (amber-italic; stop nodes red) — run health legible without reading text.
- **SSE event-typed rendering contract:** ~20 typed events each mapped to a specific UI reaction. The valuable ones: step-cap exhaustion appends "Reached the N-step limit — not finished. [Continue ▸]" instead of stalling silently; fallback toasts and rewrites the role label to the model actually used; budget-exceeded raises a banner.
- **Background-run etiquette:** switching sessions mid-run parks the stream, signals completion via sidebar dot + notification + toast, reloads history on return; notification dots are a consistent nav idiom.
- **Ask-user decision cards:** durable in-flow cards — question, described option buttons, optional multi-select, always a free-text "Other…" — answered through the normal composer, replayable after reload. Destructive acts use a promise-based styled confirm with a danger variant.
- **Layered onboarding (the fix-shape for our A-1/A-2 findings):** (a) welcome = one line + one device-aware rotating tip; (b) **conversational `/setup`** — paste a key/URL into chat, provider detected from key pattern (refusing to probe ambiguous secrets), endpoint probed, first session auto-created; (c) **interactive `/tour`** — spotlight steps that advance only when the user clicks the *real* control; (d) **one-shot contextual hints** — first-use popover with a small animated gesture demo, auto-dismisses, never repeats.
- **Settings:** one modal, 12-tab left nav; the AI tab does **per-duty model routing with ordered fallback chains** — closely parallel to Sinet's duty tiering.
- **Jobs/queues:** status-classed cards with busy-state badges and per-card actions.
- **Files:** drag-drop/paste anywhere, pending-attachment strip above the composer, client-side export pipeline.

## 4. Better / skip

**Better than a bare dashboard:** progressive disclosure (one number in the footer, stats one click away; collapse-by-default tool output) · a real interaction vocabulary (toasts with action + undo, an Escape-stack manager, global shortcuts) · self-teaching UI · 16 themes at 5 tokens each · unusual accessibility depth (aria-live chat log, reduced-motion gating, OpenDyslexic, density controls) · process artifacts (MODULE_SUMMARY.md, component variant playgrounds).

**Skip:** the scale pathology (41k-line CSS file, 500KB modules, `window.*` coupling — keep React components) · the floating-window/tiling desktop metaphor (a control room wants stable panes) · mono-for-everything and the low-contrast pale-cyan default · typewriter narration and decorative flourishes beyond one status motif · product-imitation themes (trade-dress risk) · the everything-app surface area.

## 5. Verdict — the ten pattern ideas carried into the proposal

1. Tool-call timeline rail connecting bubbles into one visual run (dot nodes, uppercase tool names, timers, collapsed output tails).
2. Visually distinct recovery/escalation nodes so run health reads at a glance.
3. Never stall silently: every terminal condition gets an inline banner or a "Continue ▸" affordance.
4. Durable ask-user decision cards with described options, multi-select, free-text escape — surviving reload.
5. One threshold-ring idiom (tiny SVG donut, green/orange/red) reused for every capacity meter, with click-through stats and one-metric footers.
6. Interactive do-the-thing tour + one-shot animated hints — the direct fix for "the UI doesn't explain itself."
7. Conversational first-run setup that never probes ambiguous secrets.
8. Five-token theme contract with density as root classes.
9. Background-run etiquette: parked streams, notification dots, completion toasts, history reload on return.
10. Status-classed job cards over tables; keep a MODULE_SUMMARY map and variant playgrounds as practice.
