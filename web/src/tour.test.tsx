import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { FakeSource, oversightRoutes, scriptedFetch } from './doubles'
import { EventStream } from './events'
import { hrefFor, type RouteID } from './routes'
import { flush, mount } from './testing'
import { stepClasses, stepsFor, tourSteps, type TourStep } from './tour'
import tourSource from './tour.tsx?raw'

/**
 * The interactive tour (P3-UI-7 R13; design proposal §3).
 *
 * THE THREE-PART PROOF the disposition requires:
 *   (i)   a walk over EVERY registered step fires zero non-GET requests;
 *   (ii)  a structural check of the registry against the verb wall, with a
 *         planted verb-class step firing it;
 *   (iii) at least one route's walk advances by REAL clicks on the real control.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

/**
 * An EMPTY world: the same reads the fixture world answers, every list emptied.
 *
 * The tour ships to production, where the seeded demo world never exists, so
 * every step must point at CHROME that is there with no rows at all.
 */
function emptyWorld(): Record<string, { body?: unknown; status?: number }> {
  const world = { ...oversightRoutes() }
  const empties: Record<string, unknown> = {
    'GET /api/tasks': { tasks: [], cursor: 89 },
    'GET /api/runs': { runs: [], cursor: 89 },
    'GET /api/approvals': { items: [], cursor: 89 },
  }
  for (const [k, body] of Object.entries(empties)) if (k in world) world[k] = { body }
  return world
}

beforeEach(() => {
  FakeSource.reset()
  vi.stubGlobal('EventSource', FakeSource)
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

async function openAt(route: RouteID, world = oversightRoutes()) {
  const log = scriptedFetch(world)
  window.history.replaceState(null, '', hrefFor(route))
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}

const click = (el: Element | null) => {
  expect(el, 'the control the tour needs is not on the page').not.toBeNull()
  act(() => {
    el!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
}

const startTour = async (view: { container: HTMLElement }) => {
  click(view.container.querySelector('[data-tour-start]'))
  await flush()
}

// ── it never starts itself ────────────────────────────────────────────────

test('the tour NEVER auto-starts, and exactly one visible affordance can start it', async () => {
  const { view } = await openAt('mission-control')
  // Nothing persists in this client, so a tour that started itself would ambush
  // every single load with no way to remember it had.
  expect(view.container.querySelector('[data-tour-overlay]'), 'the tour started itself').toBeNull()

  const starters = [...view.container.querySelectorAll('[data-tour-start]')]
  expect(starters, 'the tour has no visible way in, or more than one').toHaveLength(1)
  expect(starters[0].textContent).toBe('Show me around')

  await startTour(view)
  expect(view.container.querySelector('[data-tour-overlay]'), 'the affordance did not start it').not.toBeNull()
  view.unmount()
})

test('Escape ends it from any step, and so does the visible End tour control', async () => {
  const { view } = await openAt('mission-control')
  await startTour(view)
  act(() => {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  })
  await flush()
  expect(view.container.querySelector('[data-tour-overlay]'), 'Escape did not end the tour').toBeNull()

  await startTour(view)
  click(view.container.querySelector('[data-tour-stop]'))
  await flush()
  expect(view.container.querySelector('[data-tour-overlay]'), 'the visible End tour control did nothing').toBeNull()
  view.unmount()
})

// ── (ii) the verb wall, structurally, with its planted bite ───────────────

/**
 * The registry rule, written once so the real registry and a planted step are
 * judged by the same predicate.
 */
function offendingSteps(steps: TourStep[]): string[] {
  return steps.filter((s) => !(stepClasses as readonly string[]).includes(s.cls)).map((s) => s.id)
}

test('every registered step declares an allowed target class, and a verb-class step FIRES the check', () => {
  expect(offendingSteps(tourSteps), 'a step declares a class outside the verb wall').toEqual([])
  expect(tourSteps.length, 'the registry is empty, so the check above proves nothing').toBeGreaterThan(4)

  // THE PLANT. A step declaring a verb class is exactly what the wall exists to
  // stop, and the check has to fail on it or it is decoration.
  const planted = [
    ...tourSteps,
    { id: 'planted-verb', route: 'inbox', target: '[data-act]', cls: 'verb', title: 'x', body: 'y' } as unknown as TourStep,
  ]
  expect(offendingSteps(planted), 'a verb-class step passed the wall').toEqual(['planted-verb'])
})

test('no step targets a form submit, a confirm opener or anything that fires a verb', async () => {
  // Checked against what the selectors really SELECT on a live page, not only
  // against the declared class — a class is a promise and this is the check.
  for (const route of ['mission-control', 'inbox', 'chat'] as RouteID[]) {
    const { view } = await openAt(route)
    for (const step of stepsFor(route)) {
      const el = view.container.querySelector(step.target) ?? document.querySelector(step.target)
      if (el === null) continue
      if (step.cls === 'nav') {
        expect(el.tagName.toLowerCase(), `${step.id} declares nav but is not a link`).toBe('a')
        expect(el.getAttribute('href'), `${step.id} is a link to nowhere`).toBeTruthy()
      }
      // Neither class may be a submit, sit inside a form, or be one of the
      // landed act surfaces (`controls.tsx` marks every one of those).
      expect((el as HTMLButtonElement).type, `${step.id} targets a form submit`).not.toBe('submit')
      expect(el.closest('form'), `${step.id} targets a control inside a form`).toBeNull()
      expect(el.closest('[data-outcome]'), `${step.id} targets an act-outcome region`).toBeNull()
      expect(el.hasAttribute('data-act'), `${step.id} targets a verb button`).toBe(false)
    }
    view.unmount()
    document.querySelectorAll('body > div').forEach((n) => n.remove())
  }
})

// ── (i) the zero-non-GET walk over EVERY registered step ─────────────────

test('a walk over EVERY registered step fires ZERO non-GET requests', async () => {
  const { view, log } = await openAt('mission-control')
  await startTour(view)

  // Walk the whole registry via the overlay's own Next, which is the path that
  // visits every step regardless of which route each one belongs to.
  for (let i = 0; i < tourSteps.length + 2; i++) {
    const next = view.container.querySelector('[data-tour-next]')
    if (next === null) break
    click(next)
    await flush()
  }

  const verbs = log.calls.filter((c) => c.method !== 'GET')
  expect(verbs.map((c) => `${c.method} ${c.path}`), 'the tour fired a verb').toEqual([])
  expect(log.calls.length, 'the walk reached nothing at all, so this proves nothing').toBeGreaterThan(0)
  view.unmount()
})

// ── (iii) a real-click walk, over the EMPTY world ────────────────────────

test('a route walk advances by REAL clicks on the real controls, over an EMPTY database', async () => {
  const { view, log } = await openAt('mission-control', emptyWorld())
  await startTour(view)

  // Step 1's target is the nav rail; it is chrome and is there with no rows.
  const first = view.container.querySelector('[data-tour-overlay]')!
  expect(first.getAttribute('data-tour-step')).toBe(tourSteps[0].id)
  for (const step of stepsFor('mission-control')) {
    expect(
      view.container.querySelector(step.target),
      `${step.id} has no target on an empty database — the tour ships where the seed never exists`,
    ).not.toBeNull()
  }

  // THE REAL CLICKS: pressing the SPOTLIT control itself advances the tour, and
  // the control does exactly what it would have done anyway. Walked in order,
  // step by step, each click landing on the step the overlay is pointing at.
  click(view.container.querySelector(tourSteps[0].target))
  await flush()
  expect(
    view.container.querySelector('[data-tour-overlay]')?.getAttribute('data-tour-step'),
    'clicking the real control did not advance the tour',
  ).toBe(tourSteps[1].id)

  click(view.container.querySelector(tourSteps[1].target))
  await flush()
  expect(view.container.querySelector('[data-tour-overlay]')?.getAttribute('data-tour-step')).toBe(tourSteps[2].id)

  // Step 3 spotlights the inbox LINK: clicking it navigates for real and the
  // tour carries on onto the next surface.
  click(view.container.querySelector(tourSteps[2].target))
  await flush()
  expect(window.location.pathname, 'the real nav link did not navigate').toBe(hrefFor('inbox'))
  expect(log.calls.filter((c) => c.method !== 'GET'), 'the real-click walk fired a verb').toEqual([])
  view.unmount()
})

test('a step whose target is absent SKIPS VISIBLY rather than ringing nothing', async () => {
  // The chat head line only exists on /chat, so on mission control that step
  // has nothing to point at. Reached by walking, the card must SAY so.
  const { view } = await openAt('mission-control')
  await startTour(view)
  for (let i = 0; i < tourSteps.length; i++) {
    const overlay = view.container.querySelector('[data-tour-overlay]')
    if (overlay === null) break
    if (overlay.querySelector('[data-tour-skip="absent"]') !== null) {
      expect(overlay.textContent, 'the skip is not visible to the reader').toContain('nothing to point at')
      expect(overlay.querySelector('[data-tour-next]')?.textContent).toBe('Skip this step')
      view.unmount()
      return
    }
    click(view.container.querySelector('[data-tour-next]'))
    await flush()
  }
  throw new Error('no step was ever absent, so the visible-skip path was never exercised')
})

test('the overlay PASSES CLICKS THROUGH: the root is inert and only the card is interactive', async () => {
  // THE DEFECT THIS PINS (drain r1 D1, HIGH). The overlay root is
  // `fixed inset-0`, which covers the viewport and defaults to
  // `pointer-events: auto`. Only the scrim and the ring carried `none`, so in a
  // REAL browser hit-testing landed every click outside the card on the root:
  // the spotlit control could never be pressed, click-to-advance could never
  // fire, and a nav step could never navigate. Every jsdom test passed over it
  // because jsdom does no hit-testing — it dispatches straight at the target.
  // The pass-through precedent in this tree is the aurora field.
  const { view } = await openAt('mission-control')
  await startTour(view)

  const root = view.container.querySelector('[data-tour-overlay]') as HTMLElement
  const card = view.container.querySelector('[data-tour-card]') as HTMLElement
  expect(root.className, 'the overlay root would swallow every click on the page').toContain('pointer-events-none')
  expect(card.className, 'the tour card itself is unclickable').toContain('pointer-events-auto')

  // The ring must not intercept either — it sits ON TOP of the spotlit control.
  const ring = view.container.querySelector('[data-tour-ring]')
  if (ring !== null) expect(ring.className).toContain('pointer-events-none')

  // The probe: the pin can fail. A root without the utility is the shipped
  // defect, and a card without it is the same defect one level in.
  expect('fixed inset-0 z-40'.includes('pointer-events-none')).toBe(false)
  view.unmount()
})

test('the tour ends with the session, so it never renders over /login', async () => {
  // A mid-tour session expiry forces /login, and a spotlight over a PIN field
  // points at a control the tour never registered (drain r1 D5).
  const { view, log } = await openAt('mission-control')
  await startTour(view)
  expect(view.container.querySelector('[data-tour-overlay]')).not.toBeNull()

  log.set('GET /api/auth/session', { body: { authenticated: false } })
  click(view.container.querySelector('[data-auth="sign-out"]'))
  await flush(6)

  expect(window.location.pathname, 'the session loss did not force the login route').toBe(hrefFor('login'))
  expect(
    view.container.querySelector('[data-tour-overlay]'),
    'the tour survived the session and is ringing the login form',
  ).toBeNull()
  expect(view.container.querySelector('[data-tour-start]'), 'the tour can be started pre-session').toBeNull()
  view.unmount()
})

// ── the standing bans ─────────────────────────────────────────────────────

test('the tour holds no timer, no storage, no registry key and no library', () => {
  for (const banned of [
    'setInterval',
    'setTimeout',
    'requestAnimationFrame',
    'localStorage',
    'sessionStorage',
    'indexedDB',
    'document.cookie',
  ]) {
    expect(tourSource, `the tour reached for ${banned}`).not.toContain(banned)
  }
  // Owned JSX: no tour/spotlight/coach-mark package entered the closed runtime
  // set, and the only imports are this tree's own.
  for (const m of tourSource.matchAll(/from '([^']+)'/g)) {
    expect(m[1].startsWith('./') || m[1] === 'react', `the tour imported ${m[1]}`).toBe(true)
  }
  // The ring's motion is CSS behind the reduced-motion gate, never a clock.
  expect(tourSource).toContain('motion-safe:')
})
