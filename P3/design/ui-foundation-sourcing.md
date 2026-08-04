# UI foundation sourcing — live research (2026-08-04)

**Input #2 to the design-approach proposal** (B6 gate §9, D3 condition). Produced by a web-research agent against the live npm registry, GitHub API and vendor docs on 2026-08-04. Coordinator note: every pin below is **re-verified at adoption time** through the rail (components.lock discipline) — this file records the decision basis, not the binding pins.

## Ecosystem fact that reframes the question

**shadcn/ui switched its default primitive layer from Radix to Base UI on 2026-07-03.** New `npx shadcn init` projects wire to Base UI; Radix remains supported (`-b radix` flag); every component ships for both. Base UI (MUI's successor to Radix, ex-Radix/Floating-UI authors) went stable 1.0.0 on 2025-12-11, renamed `@base-ui-components/react` → `@base-ui/react`, at **1.7.0** (published 2026-08-04). The unified `radix-ui` package is alive (1.6.7, 2026-07-24) but momentum has moved.

## Candidate evaluations (versions/licenses live-fetched 2026-08-04)

1. **Tailwind CSS v4 + @tailwindcss/vite** — 4.3.3 (2026-07-16), MIT. Build-time only, zero runtime, Vite 8 explicitly in peer range. Utilities apply identically to hand-written JSON Forms renderers, react-diff-view markup and dnd nodes. No material risk; it's a compiler.
2. **shadcn/ui (copy-in, Base UI-backed)** — CLI 4.16.1 (2026-07-31), repo 120k★, MIT, generated code is yours. React 19 + Tailwind v4 across the registry. Offline-clean: local TSX, `lucide-react` bundled ESM SVGs, no webfonts required. Runtime deps only the primitives + small utils (`@base-ui/react`, `clsx`, `tailwind-merge`, `class-variance-authority`, `tw-animate-css`), all pinnable; CLI dev-time only. **Decisive coherence: @assistant-ui/react is shadcn-native — its Streamdown renderer assumes shadcn tokens (`--background`, `--muted-foreground`, `--border`)** — so the chat surface themes itself from the same token set. Risk: ecosystem mid-transition (Radix↔Base UI), almost fully mitigated by the vendored model.
3. **Base UI standalone** — 1.7.0, MIT, React ^17–^19. Headless behavior layer only; young 1.x but MUI-backed. The layer shadcn now rides.
4. **Ark UI / Park UI** — Ark `@ark-ui/react` 5.38.0 (2026-08-03), MIT, very active. Park (styled layer, Panda CSS): preset last published 2024-11-22, CLI 2025-11-20, repo last commit 2026-02-21 — the slowest-moving styled layer evaluated. Not shortlisted.
5. **Mantine 9** — `@mantine/core` 9.5.1 (2026-08-02), MIT, very active; **requires React ^19.2** (v9 built on React 19). Exemplary offline posture (system fonts by default, explicitly no webfont fetch; own CSS; `@tabler/icons-react` 3.46.0 bundled SVGs). Strong for dense ops UI. Risks: the anti-copy-in option (framework dependency tracking React majors tightly), and it doesn't speak shadcn tokens — the chat pane would be themed in a second vocabulary forever.
6. **tremor** — current copy-paste components (Tailwind+Radix+Recharts, Apache-2.0) but repo quiet since 2025-10-10 (post-Vercel-acquisition); legacy `@tremor/react` 3.18.7 frozen 2025-01-13, React ^18 only. Not a foundation; acceptable later as a vendored parts donor (meters/KPI tiles) on top of combo 1.
7. **daisyUI 5** — 5.7.15 (2026-08-03), MIT, very active. Pure-CSS Tailwind plugin, zero runtime, zero deps, ~34 kB, OKLCH theming. Styles any markup but carries no behavior and less token-level precision; still needs a manual bridge to assistant-ui's shadcn tokens.
8. **open-props** — 1.7.23 (2026-01-31), MIT; v2 in beta; six months quiet. Tokens only. Not shortlisted.
9. Also evaluated: **HeroUI** 3.2.3 (active; heavy npm framework, no copy-in) · **Chakra 3.36.1** (Emotion runtime CSS-in-JS — worst-fit for hand-written renderers) · **Radix Themes** 3.3.0 (slowing; wrong side of the momentum) · **Basecoat** 1.0.2 (shadcn look as plain CSS — interesting, too young) · **Tailwind Catalyst** (paid license — excluded outright).

## Ranked recommendations

**1. Tailwind v4 + shadcn/ui copy-in (Base UI track) + bundled lucide + Fontsource Inter — recommended.**
Pins at research time: `tailwindcss@4.3.3`, `@tailwindcss/vite@4.3.3` (dev), `shadcn@4.16.1` (dev-time CLI), `@base-ui/react@1.7.0`, `lucide-react@1.28.0` (ISC), `clsx@2.1.1`, `tailwind-merge@3.6.0`, `class-variance-authority@0.7.1` (Apache-2.0), `tw-animate-css@1.4.0`, optional `@fontsource-variable/inter@5.3.0` (OFL-1.1). The only option satisfying every constraint and the vendoring preference simultaneously: components arrive as owned TSX (upstream churn — including the Radix→Base-UI shift — cannot touch them), five small pinned runtime deps, all-permissive licenses, nothing phones a CDN, and it is the one foundation the pinned stack already speaks (assistant-ui's shadcn-token assumption themes chat + diff + approval cards + JSON Forms renderers from one token set and one `.dark` class). Residual risk — you maintain the copied components — is exactly the trade the platform prefers.

**2. Mantine 9 all-in (+ Tabler icons).** Best single-dependency answer; loses on the vendoring inversion and the permanent two-vocabulary theming split with assistant-ui.

**3. Tailwind v4 + daisyUI 5 + Base UI headless.** Lowest-maintenance; loses on token precision and hand-written composites — at which point combo 1 delivers more for the same effort.

## Sources

Live npm registry JSON (versions/licenses/publish-times/peers) for all packages named above; GitHub API for shadcn-ui/ui, tremorlabs/tremor, chakra-ui/ark, chakra-ui/park-ui, argyleink/open-props, saadeghi/daisyui; shadcn changelog "July 2026: Base UI as the Default" + React-19 doc; Base UI v1.0.0 release notes; Mantine 9.0 changelog + typography doc; daisyUI v5 notes; assistant-ui Streamdown doc; Vercel-acquires-Tremor announcement; Park UI docs; shadcn discussion #9562. All accessed 2026-08-04.
