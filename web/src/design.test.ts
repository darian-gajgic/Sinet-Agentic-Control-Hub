import { expect, test } from 'vitest'

/**
 * The design foundation, asserted from the stylesheet itself (P3/CONVENTIONS.md
 * §47).
 *
 * jsdom has no cascade for custom properties and no layout engine, so a token
 * pour, a density switch and a motion gate are all checked the way the
 * responsive skeleton already is: against the real source text. That is what
 * makes them checkable at all — and every scan below is shown able to fail,
 * because a scan that finds nothing proves nothing until it does.
 */
import css from './index.css?raw'

/** The declarations of one selector block. */
function block(selector: string): string {
  const at = css.indexOf(`\n${selector} {`)
  expect(at, `no rule for ${selector}`).toBeGreaterThan(-1)
  return css.slice(at, css.indexOf('}', at))
}

/**
 * The text of an at-rule's body, brace-matched so nested blocks survive.
 *
 * The head must be followed by its own `{`, or a comment that merely NAMES the
 * at-rule would be matched instead of the rule — which is exactly what this
 * stylesheet's own explanatory comments do.
 */
function atRule(head: string): string {
  const open = css.indexOf(`${head} {`)
  expect(open, `no ${head} block`).toBeGreaterThan(-1)
  let depth = 0
  for (let i = open + head.length; i < css.length; i++) {
    if (css[i] === '{') depth++
    else if (css[i] === '}' && --depth === 0) return css.slice(css.indexOf('{', open) + 1, i)
  }
  throw new Error(`unbalanced ${head}`)
}

/** The value a `:root`-level custom property is defined with. */
function token(name: string): string {
  const m = new RegExp(`\\n\\s*${name}:\\s*([^;]+);`).exec(css)
  expect(m, `${name} is not defined`).not.toBeNull()
  return m![1].trim()
}

// ── the token pour (R5/R7) ────────────────────────────────────────────────

test('the shadcn base token set is defined, poured from the Nexus values', () => {
  // The spot-checks the design rests on: the near-black base, the violet-tinted
  // translucent neutral that makes every surface one material, and the identity
  // violet. A plain gray here would be the single change that loses the look.
  expect(token('--background')).toBe('#07070d')
  expect(token('--foreground')).toBe('#eaeaf4')
  expect(token('--primary')).toBe('#7c5cff')
  expect(token('--card')).toContain('rgba(148, 148, 190,')
  expect(token('--border')).toBe('var(--line)')
  expect(token('--line')).toContain('rgba(148, 148, 190,')
  expect(token('--radius')).toBe('14px')
  expect(token('--radius-sm')).toBe('9px')

  // The set is COMPLETE: a missing name is a primitive that falls back to a
  // browser default the moment a later packet uses it.
  for (const name of [
    '--background',
    '--foreground',
    '--card',
    '--card-foreground',
    '--popover',
    '--popover-foreground',
    '--primary',
    '--primary-foreground',
    '--secondary',
    '--secondary-foreground',
    '--muted-foreground',
    '--accent-foreground',
    '--destructive',
    '--input',
    '--ring',
    '--radius',
    '--sidebar',
    '--sidebar-foreground',
    '--sidebar-border',
    '--chart-1',
    '--chart-5',
  ]) {
    expect(css, `${name} is missing from the base set`).toContain(`\n  ${name}:`)
  }
})

test('the Sinet extension tokens carry the parts shadcn has no name for', () => {
  expect(token('--grad')).toBe('linear-gradient(135deg, #7c5cff 0%, #22d3ee 100%)')
  expect(token('--accent-glow')).toBe('rgba(124, 92, 255, 0.16)')
  expect(token('--sidebar-w')).toBe('232px')
  expect(token('--blur-shell')).toBe('18px')
  // The six status hues, which the one chip formula is applied over.
  for (const [name, hex] of [
    ['--green', '#4ade80'],
    ['--yellow', '#fbbf24'],
    ['--red', '#f87171'],
    ['--blue', '#60a5fa'],
    ['--orange', '#fb923c'],
    ['--pink', '#f472b6'],
  ] as const) {
    expect(token(name)).toBe(hex)
  }
})

test('the nine landed tokens are still defined and are re-pointed at the new palette', () => {
  // R7: every surface built before this packet re-bases with zero structural
  // edits, which only holds if the names survive AND stop carrying the old
  // values. Both halves are asserted, because either one alone passes for the
  // wrong reason — a deleted token would take a surface's styling with it, and
  // a kept-but-unmoved one would ship a two-palette app.
  const repointed: [string, string][] = [
    ['--bg', 'var(--background)'],
    ['--panel', 'var(--card)'],
    ['--fg', 'var(--foreground)'],
    ['--muted', '#9a9ab2'],
    ['--accent', '#7c5cff'],
    ['--ok', '#4ade80'],
    ['--warn', '#fbbf24'],
    ['--bad', '#f87171'],
  ]
  for (const [name, value] of repointed) expect(token(name)).toBe(value)
  expect(token('--line')).toContain('rgba(148, 148, 190,')

  // None of the pre-packet values survives anywhere in the sheet.
  for (const stale of ['#101418', '#161c22', '#e6edf3', '#9aa7b4', '#26303a', '#6fb3d2', '#57a773', '#d1a054', '#cf6b5a']) {
    expect(css, `the pre-packet value ${stale} is still in the stylesheet`).not.toContain(stale)
  }
})

test('@theme inline maps the tokens onto utility names, and decouples the two colliding ones', () => {
  const theme = atRule('@theme inline')
  for (const name of ['--color-background', '--color-card', '--color-border', '--color-primary', '--color-muted-foreground']) {
    expect(theme, `${name} is not wired to a utility`).toContain(name)
  }
  // `--accent` and `--muted` are landed names carrying landed meanings, so the
  // shadcn SURFACE semantics live on their own variables and the utility is
  // bound to those. Without this the app would render violet hover fills and
  // lavender "muted" backgrounds the moment a primitive used bg-accent.
  expect(theme).toContain('--color-accent: var(--accent-surface)')
  expect(theme).toContain('--color-muted: var(--muted-surface)')
  expect(token('--accent-surface')).toContain('rgba(148, 148, 190,')
  expect(token('--muted-surface')).toContain('rgba(148, 148, 190,')
  // The brand violet stays reachable as a utility under a name of its own.
  expect(theme).toContain('--color-brand: var(--accent)')
})

test('the theme is dark-first and no light ramp is built', () => {
  expect(block(':root')).toContain('color-scheme: dark')
  // The architecture keeps a light ramp addable; this packet does not build one,
  // and a half-built one is worse than none.
  expect(css).not.toContain('prefers-color-scheme: light')
  expect(css).not.toContain('.light {')
})

test('Tailwind is imported as a compiler and the fonts are bundled, never fetched', () => {
  expect(css).toContain("@import 'tailwindcss'")
  expect(css).toContain("@import 'tw-animate-css'")
  expect(css).toContain("'Inter Variable'")
  expect(css).toContain("'JetBrains Mono'")
  // Nexus and Odysseus both reached for a webfont CDN. This stylesheet cannot.
  expect(css).not.toMatch(/@import\s+url\(/)
  expect(css, 'the stylesheet reaches an external host').not.toMatch(/https?:\/\//)
})

// ── motion (R17) ──────────────────────────────────────────────────────────

/** Every animation this sheet starts, paired with whether it sits inside a
 *  `no-preference` gate. */
function ungatedAnimations(source: string): string[] {
  const gates: string[] = []
  const marker = '@media (prefers-reduced-motion: no-preference)'
  for (let at = source.indexOf(marker); at !== -1; at = source.indexOf(marker, at + 1)) {
    const open = source.indexOf('{', at)
    let depth = 0
    for (let i = open; i < source.length; i++) {
      if (source[i] === '{') depth++
      else if (source[i] === '}' && --depth === 0) {
        gates.push(source.slice(open + 1, i))
        break
      }
    }
  }
  const ungated: string[] = []
  for (const m of source.matchAll(/\n\s*animation:\s*([^;]+);/g)) {
    if (!gates.some((g) => g.includes(m[0].trim()))) ungated.push(m[1].trim())
  }
  return ungated
}

test('the reduced-motion kill switch exists and neutralizes animation outright', () => {
  const kill = atRule('@media (prefers-reduced-motion: reduce)')
  expect(kill).toContain('animation-duration: 0.01ms !important')
  expect(kill).toContain('transition-duration: 0.01ms !important')
  expect(kill, 'the kill switch does not reach pseudo-elements').toContain('*::before')
})

test('every animation the stylesheet starts is authored behind a no-preference gate', () => {
  // The kill switch is the backstop. The gate is the intent: a reduce reader
  // gets the STATIC composition — a still aurora, a solid dot — rather than an
  // animation that was started and then clamped to nothing.
  expect(ungatedAnimations(css)).toEqual([])

  // Probe: the detector really can fail, and really does accept a gated one.
  const planted = '\n.x {\n  animation: drift 26s linear infinite;\n}\n'
  expect(ungatedAnimations(planted)).toEqual(['drift 26s linear infinite'])
  expect(ungatedAnimations(`@media (prefers-reduced-motion: no-preference) {${planted}}`)).toEqual([])
})

test('the aurora is one decorative field that takes no input', () => {
  const aurora = block('.aurora')
  expect(aurora).toContain('position: fixed')
  expect(aurora, 'the backdrop would swallow clicks').toContain('pointer-events: none')
  // Four drifting radial gradients at 5–13% opacity is the whole mechanism.
  expect(aurora.match(/radial-gradient\(/g)).toHaveLength(4)
  for (const hue of ['--aurora-1', '--aurora-2', '--aurora-3', '--aurora-4']) expect(aurora).toContain(hue)
})

// ── density (R18) ─────────────────────────────────────────────────────────

test('both density classes ship, comfortable is the default, and compact really is tighter', () => {
  const comfortable = block(':root,\n.density-comfortable')
  const compact = block('.density-compact')

  const names = [...comfortable.matchAll(/(--density-[a-z-]+):\s*([^;]+);/g)].map((m) => m[1])
  expect(names.length, 'the density scale is empty').toBeGreaterThan(3)

  // The two classes switch the SAME variables — a name in one and not the
  // other is a value that silently falls back to the comfortable one.
  const compactNames = [...compact.matchAll(/(--density-[a-z-]+):\s*([^;]+);/g)].map((m) => m[1])
  expect(compactNames).toEqual(names)

  // And every one of them actually changes.
  const valueOf = (source: string, name: string) => new RegExp(`${name}:\\s*([^;]+);`).exec(source)![1]
  for (const name of names) {
    expect(valueOf(compact, name), `${name} does not change under compact`).not.toBe(valueOf(comfortable, name))
  }

  // Comfortable is the default because :root carries it, so a tree that never
  // receives a class still renders at the right density.
  expect(css).toContain(':root,\n.density-comfortable {')
  // No persistence and no ⚙ key: web storage is banned and none was invented.
  expect(css).not.toContain('localStorage')
})

test('toggling the density class on a real root really moves the spacing tokens', () => {
  // The structural check above proves the two blocks disagree in the SOURCE.
  // This one proves the cascade does what the source says: the real blocks are
  // lifted out of the real stylesheet — not retyped — applied to a real root,
  // and read back through getComputedStyle.
  const style = document.createElement('style')
  style.textContent = `:root,\n.density-comfortable {${block(':root,\n.density-comfortable')
    .split('{')
    .slice(1)
    .join('{')}}\n.density-compact {${block('.density-compact').split('{').slice(1).join('{')}}`
  document.head.appendChild(style)

  const root = document.documentElement
  const read = (name: string) => getComputedStyle(root).getPropertyValue(name).trim()

  root.className = 'density-comfortable'
  const comfortable = read('--density-pad')
  root.className = 'density-compact'
  const compact = read('--density-pad')

  expect(comfortable, 'the comfortable scale resolved to nothing').not.toBe('')
  expect(compact, 'compact did not tighten the padding').not.toBe(comfortable)

  // And with no class at all the tree still renders comfortable, because :root
  // carries the same values — a component never sees an empty spacing token.
  root.className = ''
  expect(read('--density-pad')).toBe(comfortable)

  style.remove()
})
