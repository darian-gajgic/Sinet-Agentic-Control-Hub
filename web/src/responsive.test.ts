import { expect, test } from 'vitest'

/**
 * Skeleton responsiveness (Spec S1.10 via S15.12): one workspace, not two
 * products. jsdom has no layout engine, so this asserts the STRUCTURE that
 * makes a phone-width shell work — a fixed pixel width on a layout container
 * is what forces horizontal scrolling at 375px, and there is none.
 *
 * Surface-level phone-completeness (an inbox you can actually decide from) is
 * B6-5..9's, swept at B6-9 and drilled on real devices there.
 */
import css from './index.css?raw'

/** Declarations for one selector block. */
function block(selector: string): string {
  const at = css.indexOf(`\n${selector} {`)
  expect(at, `no rule for ${selector}`).toBeGreaterThan(-1)
  return css.slice(at, css.indexOf('}', at))
}

test('the stylesheet is really the shell stylesheet', () => {
  expect(css.length).toBeGreaterThan(500)
  for (const selector of ['.shell', '.shell-head', '.shell-nav', '.shell-main']) {
    expect(css, `${selector} is not styled`).toContain(`${selector} {`)
  }
})

test('no layout container carries a fixed pixel width', () => {
  for (const selector of [
    '.shell',
    '.shell-head',
    '.shell-nav',
    '.shell-main',
    '.panel',
    // The B6-5 oversight surfaces are held to the same rule.
    '.block',
    '.columns',
    '.column',
    '.card',
    '.table-scroll',
    '.filter-bar',
    '.fleet-filters',
  ]) {
    const rules = block(selector)
    expect(rules, `${selector} pins a pixel width — a phone would scroll sideways`).not.toMatch(
      /(^|[^-])width:\s*\d+px/,
    )
  }
})

test('the nav wraps rather than overflowing, so every surface stays reachable at phone width', () => {
  const nav = block('.shell-nav')
  expect(nav).toContain('flex-wrap: wrap')
  expect(nav).not.toContain('overflow-x: hidden')
})

test('the shell is phone-first and widens at a breakpoint, not the other way round', () => {
  // A max-width query would mean the desktop layout is the base and the phone
  // is the exception — the "two products" shape S1.10 rules out.
  expect(css).toContain('@media (min-width:')
  expect(css).not.toContain('@media (max-width:')
})

test('the body cannot scroll sideways and oversized content is contained', () => {
  expect(block('body')).toContain('overflow-x: hidden')
  expect(css).toMatch(/max-width:\s*100%/)
})

test('the board stacks on a phone and only widens into columns at the breakpoint', () => {
  // Phone-first: the single-column track is the BASE, and the multi-column
  // track appears inside the min-width query. The other way round would make
  // the desktop the product and the phone the exception (S1.10).
  expect(block('.columns'), 'the board does not stack at phone width').toContain('grid-template-columns: 1fr')
  const wide = css.slice(css.indexOf('@media (min-width:'))
  expect(wide, 'the board never widens on a large screen').toContain('grid-template-columns: repeat(auto-fit')
})

test('content too wide for a phone scrolls inside its own box, not the page', () => {
  // Meters tables and history answers are genuinely wider than 375px. The one
  // place a horizontal scrollbar is allowed is inside the thing that is too
  // wide — the body itself still cannot scroll sideways (asserted above).
  expect(block('.table-scroll')).toContain('overflow-x: auto')
  expect(block('.audit pre')).toContain('overflow-x: auto')
  expect(block('.card-face dd'), 'a long model-written title would widen the page').toContain(
    'overflow-wrap: anywhere',
  )
})

test('the personal filters and the fleet filters reach everything at phone width', () => {
  // The filters are first-class views (S1.4) and must be usable on a phone, so
  // their bar wraps exactly as the shell nav does rather than overflowing.
  expect(block('.filter-bar')).toContain('flex-wrap: wrap')
  expect(block('.fleet-filters'), 'the fleet filters do not stack at phone width').toContain('flex-direction: column')
  const wide = css.slice(css.indexOf('@media (min-width:'))
  expect(wide, 'the fleet filters never lay out side by side on a wide screen').toContain('.fleet-filters')
})

test('long task-detail content wraps instead of widening the page', () => {
  // `.decisions li` is the last selector of the grouped rule that covers the
  // ACs, plan steps, stages and decisions — the block helper matches the
  // selector immediately before the brace.
  for (const selector of ['.decisions li', '.mode-note']) {
    expect(block(selector), `${selector} would push the page sideways on a phone`).toContain('overflow-wrap: anywhere')
  }
})

test('a view that owes a re-snapshot is marked in the stylesheet, not only in the DOM', () => {
  // The stale marker has to be VISIBLE, or "stale never poses as live" is a
  // data attribute nobody can see (S15.12).
  const stale = block(".block[data-stale='true']")
  expect(stale).toContain('border-inline-start')
})

test('the connection label survives at phone width, only its detail is dropped', () => {
  // The detail is hidden by default and revealed at the breakpoint; the LABEL
  // is never hidden, so connection state is visible on a phone (S15.12).
  expect(block('.conn-detail')).toContain('display: none')
  expect(css).not.toMatch(/\.conn-label\s*\{[^}]*display:\s*none/)
})
