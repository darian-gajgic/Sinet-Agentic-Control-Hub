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
import historySource from './History.tsx?raw'

/** Declarations for one selector block. */
function block(selector: string): string {
  const at = css.indexOf(`\n${selector} {`)
  expect(at, `no rule for ${selector}`).toBeGreaterThan(-1)
  return css.slice(at, css.indexOf('}', at))
}

test('the stylesheet is really the shell stylesheet', () => {
  expect(css.length).toBeGreaterThan(500)
  for (const selector of ['.shell', '.shell-side', '.shell-body', '.shell-head', '.shell-nav', '.shell-main']) {
    expect(css, `${selector} is not styled`).toContain(`${selector} {`)
  }
})

test('no layout container carries a fixed pixel width', () => {
  for (const selector of [
    '.shell',
    // The sidebar restyle (UI-1) is held to the same rule: at PHONE width it
    // carries no width at all, and takes --sidebar-w only inside the breakpoint.
    '.shell-side',
    '.shell-body',
    '.shell-head',
    '.shell-nav',
    '.nav-group',
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
    // The B6-6 decision surface.
    '.cards',
    '.card-row',
    '.batch-bar',
    '.pair',
    '.pair-side',
    // The B6-6 settings tab.
    '.settings-cats',
    '.cat-panel',
    '.setting',
    '.setting-group',
  ]) {
    const rules = block(selector)
    expect(rules, `${selector} pins a pixel width — a phone would scroll sideways`).not.toMatch(
      /(^|[^-])width:\s*\d+px/,
    )
  }
})

test('the phone nav is an off-canvas drawer, and every surface stays reachable at phone width', () => {
  // REWRITTEN 2026-08-05 (rework step 1). The nav grew to fifteen entries, so
  // the phone base is no longer a wrapping band — it is the Nexus mobile pass,
  // min-width-first: an off-canvas drawer the topbar's menu button opens, with
  // a scrim that closes it. The reachability CONTRACT is unchanged: every nav
  // link is in the drawer's markup at every width, and nothing scrolls the
  // page sideways (asserted below).
  const side = block('.shell-side')
  expect(side, 'the drawer is not taken out of flow at phone width').toContain('position: fixed')
  expect(side, 'the drawer is not off-canvas at rest').toContain('transform: translateX(')
  expect(block(".shell-side[data-open='true']"), 'opening the drawer does not bring it on screen').toContain(
    'transform: none',
  )
  // The opener exists at the base and disappears at the breakpoint, where the
  // drawer becomes the static column; the scrim covers the page under it.
  expect(block('.menu-btn')).toContain('display: flex')
  const wide = css.slice(css.indexOf('@media (min-width:'))
  const wideMenu = wide.slice(wide.indexOf('.menu-btn {'), wide.indexOf('}', wide.indexOf('.menu-btn {')))
  expect(wideMenu, 'the menu button survives onto the desktop, where there is no drawer').toContain('display: none')
  expect(block('.shell-scrim')).toContain('position: fixed')
  // The nav is a column that scrolls INSIDE the drawer, so no entry is ever
  // clipped off a short screen.
  expect(block('.shell-nav')).toContain('overflow-y: auto')
  expect(block('.shell-nav')).toContain('flex-direction: column')
})

test('the sidebar column appears at the breakpoint and the phone gets the same markup as a drawer', () => {
  // Phone-first, applied to the shell chrome itself: the base is a column (the
  // topbar over the content, the nav off-canvas), and the side-by-side layout
  // exists only inside the min-width query (S1.10).
  expect(block('.shell')).toContain('flex-direction: column')
  const wide = css.slice(css.indexOf('@media (min-width:'))
  const wideShell = wide.slice(wide.indexOf('.shell {'), wide.indexOf('}', wide.indexOf('.shell {')))
  expect(wideShell, 'the sidebar never lays out beside the content on a wide screen').toContain('flex-direction: row')
  // The one width the sidebar ever takes is the design token, declared at the
  // base (the drawer and the column are the same markup at the same width).
  expect(block('.shell-side')).toContain('width: var(--sidebar-w)')
  const wideSide = wide.slice(wide.indexOf('.shell-side {'), wide.indexOf('}', wide.indexOf('.shell-side {')))
  expect(wideSide, 'the drawer does not become a static column at the breakpoint').toContain('position: sticky')
  // The content column can shrink below its content's intrinsic width, which is
  // what stops a wide table inside it from widening the whole page.
  expect(block('.shell-body')).toContain('min-width: 0')
})

test('the shell is phone-first and widens at a breakpoint, not the other way round', () => {
  // A max-width query would mean the desktop layout is the base and the phone
  // is the exception — the "two products" shape S1.10 rules out.
  expect(css).toContain('@media (min-width:')
  expect(css).not.toContain('@media (max-width:')
})

test('the body cannot scroll sideways and oversized content is contained', () => {
  // `clip`, not the old value: both keep the body from scrolling sideways,
  // but the old value additionally made body a SCROLL CONTAINER and killed
  // wheel input on whole surfaces — the two-walk bug pinned at the bottom of
  // this file. The property this test guards ("cannot scroll sideways") is
  // exactly what clip does, minus the trap.
  expect(block('body')).toContain('overflow-x: clip')
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
  expect(block('.card-face dd'), 'a long model-written title would widen the page').toContain(
    'overflow-wrap: anywhere',
  )

  // RE-POINTED, not dropped (P3-UI-5): the generated-SQL block's `.audit pre`
  // rule shed with the history panel's restyle, so the SAME property is now
  // asserted where it lives — on the element that renders it. A Layer-2 audit
  // carries a generated statement that is genuinely wider than a phone, and it
  // still scrolls inside its own box AND wraps rather than widening the page.
  const auditPre = /<pre className="([^"]*)">\{answer\.audit\.sql_generated\}/.exec(historySource)
  expect(auditPre, 'the generated-statement block moved — this assertion no longer reaches it').not.toBeNull()
  expect(auditPre![1], 'a generated statement would push the page sideways').toContain('overflow-x-auto')
  expect(auditPre![1], 'a generated statement would not wrap').toContain('whitespace-pre-wrap')
  // Probe: the matcher really discriminates, so "it contains it" is a finding
  // rather than a pattern that matches anything.
  expect(/<pre className="([^"]*)">\{answer\.audit\.sql_generated\}/.test('<pre>{answer.audit.sql_generated}')).toBe(
    false,
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

test('an inbox card is readable and answerable at phone width', () => {
  // A card id is an opaque string with no spaces in it (kind:native-id), and a
  // blind-pair response is genuinely wider than a phone. Neither may widen the
  // page: the id wraps, and the response scrolls inside its own box.
  expect(block('.card-id'), 'a long card id would push the page sideways').toContain('overflow-wrap: anywhere')
  expect(block('.render'), 'a rendered response would push the page sideways').toContain('overflow-x: auto')
  // The card head and the act buttons wrap rather than overflowing, so every
  // control stays reachable at 375px.
  expect(block('.card-head')).toContain('flex-wrap: wrap')
  expect(block('.acts .buttons')).toContain('flex-wrap: wrap')
  // The blind pair stacks on a phone and only sits side by side at the
  // breakpoint — the phone is the base, not the exception (S1.10).
  expect(block('.pair')).toContain('flex-direction: column')
  const wide = css.slice(css.indexOf('@media (min-width:'))
  expect(wide, 'the blind pair never lays out side by side on a wide screen').toContain('.pair {')
})

test('the settings tab reaches all 33 domains at phone width', () => {
  // The tab strip is the only navigation into 33 domains, so it must wrap
  // rather than overflow — the same rule the shell nav is held to.
  expect(block('.cat-tabs')).toContain('flex-wrap: wrap')
  expect(block('.cat-tabs')).not.toContain('overflow-x: hidden')
  // A setting's own editors stack on a phone and lay out in a row only at the
  // breakpoint. `.add-price-row` is the last selector of the grouped rule that
  // covers them all — the block helper matches the selector before the brace.
  expect(block('.add-price-row')).toContain('flex-direction: column')
  const wide = css.slice(css.indexOf('@media (min-width:'))
  expect(wide, 'the settings acts never lay out in a row on a wide screen').toContain('.setting-acts')
  // Long help text, long keys and long audit values wrap instead of widening
  // the page (`.deferred li` closes that grouped rule).
  expect(block('.deferred li')).toContain('overflow-wrap: anywhere')
})

test('the assistant is usable at phone width: three regions that stack, content that wraps', () => {
  // S1.10 names chat as phone-complete, so its three regions stack on a phone
  // and lay out side by side only at the breakpoint.
  expect(block('.chat-body')).toContain('flex-direction: column')
  // The wide rule must actually SET something: asserting only that the selector
  // appears after the breakpoint would pass over an empty block (drain r1 D10).
  const wide = css.slice(css.indexOf('@media (min-width:'))
  const wideBody = wide.slice(wide.indexOf('.chat-body {'), wide.indexOf('}', wide.indexOf('.chat-body {')))
  expect(wideBody, 'the assistant does not lay its regions out side by side on a wide screen').toContain(
    'flex-direction: row',
  )
  // Every act stays reachable: the verb picker and the composer row wrap, and
  // nothing is hidden behind a horizontal scroll of the page.
  expect(block('.chat-composer-row')).toContain('flex-direction: column')
  // `.chat-intake-acts` closes the grouped rule that also covers the session
  // acts and the rename row — the block helper matches the selector before the
  // brace, so the last one names the whole group.
  expect(block('.chat-intake-acts')).toContain('flex-wrap: wrap')
  // Content that is genuinely wider than a phone scrolls INSIDE its own box: an
  // answer is a table, and a sha256 is 64 characters with no break in it.
  expect(block('.chat-intake')).toContain('overflow-x: auto')
  expect(block('.chat-file-sha')).toContain('overflow-wrap: anywhere')
  expect(block('.chat-turn'), 'a long message would push the page sideways').toContain('overflow-wrap: anywhere')
  expect(block('.chat-input')).toContain('max-width: 100%')
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

test('the review surface reads at phone width and nothing widens the page', () => {
  // A diff is genuinely wider than a phone, so it scrolls INSIDE its own box
  // rather than pushing the page sideways — the same answer the receipt table and
  // the audit block already give.
  expect(block('.diff-file'), 'a diff would push the page sideways').toContain('overflow-x: auto')
  // The three long opaque strings on this surface — a content hash, a quoted diff
  // line, and a commit trailer block — each have their own answer: hashes wrap,
  // and the pre-formatted blocks scroll.
  // `.deliverable .object-sha` and `.frames` are the LAST selectors of their
  // grouped rules, which is what the block helper matches.
  expect(block('.deliverable .object-sha'), 'a content hash would push the page sideways').toContain(
    'overflow-wrap: anywhere',
  )
  expect(block('.merge-card pre'), 'a conflict list would push the page sideways').toContain('overflow-x: auto')
  // A comment body is somebody's own prose and a door reason is the server's: both
  // wrap rather than overflow, in the same grouped rule.
  expect(block('.signing'), 'a comment body, a door reason and the signing statement all wrap').toContain(
    'overflow-wrap: anywhere',
  )

  // Every control cluster stacks at phone width and only lays out in a row at the
  // breakpoint — the phone is the base, not the exception (S1.10).
  for (const selector of ['.merge-options', '.frame-path', '.frames']) {
    expect(block(selector), `${selector} does not stack at phone width`).toContain('flex-direction: column')
  }
  const wide = css.slice(css.indexOf('@media (min-width:'))
  for (const selector of ['.two-up', '.frame-path', '.image-modes']) {
    expect(wide, `${selector} never widens on a large screen`).toContain(selector)
  }

  // An image and a preview frame both fit the viewport they are in: an image is a
  // deliverable's own content and could be any size, and a frame is somebody
  // else's whole application.
  expect(block('.image-side img'), 'a full-size image would widen the page').toContain('max-width: 100%')
  expect(block('.frame iframe')).toContain('width: 100%')
})

test('the body clips sideways overflow WITHOUT becoming a scroll container (W1-13/W2-12/RA)', () => {
  // Two cold walks reported wheel and PageDown dead while script scrolling
  // worked; the RA round reproduced it live and bisected it to ONE declaration:
  // `body { overflow-x: hidden }`. A non-visible overflow on body makes body a
  // scroll container and is PROPAGATED to the viewport, and Chrome was left
  // with no scrollable node for wheel input on some surfaces. `clip` clips the
  // same sideways overflow without creating a scroll container. This pin is
  // what keeps the one-word regression from ever shipping again.
  expect(block('body'), 'body must clip sideways overflow, never hide it').toContain('overflow-x: clip')
  expect(block('body'), 'overflow-x: hidden on body is the two-walk scroll bug').not.toContain('overflow-x: hidden')
})
