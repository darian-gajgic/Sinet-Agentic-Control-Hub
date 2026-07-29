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
  for (const selector of ['.shell', '.shell-head', '.shell-nav', '.shell-main', '.panel']) {
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

test('the connection label survives at phone width, only its detail is dropped', () => {
  // The detail is hidden by default and revealed at the breakpoint; the LABEL
  // is never hidden, so connection state is visible on a phone (S15.12).
  expect(block('.conn-detail')).toContain('display: none')
  expect(css).not.toMatch(/\.conn-label\s*\{[^}]*display:\s*none/)
})
