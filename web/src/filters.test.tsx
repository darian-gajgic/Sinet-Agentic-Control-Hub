import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { filterFromSearch, filters, hrefForFilter, localMidnightISO } from './Filters'
import { FakeSource, fixtures, oversightRoutes, scriptedFetch } from './doubles'
import { EventStream } from './events'
import { navigate } from './router'
import { flush, mount } from './testing'

/**
 * The four personal filters (Spec S15.5 ¶5; S1.4; S1.10).
 *
 * The suite runs in a NON-UTC zone (vitest.config.ts pins TZ), which is the
 * only way "finished today" can be tested honestly: in UTC the local-midnight
 * conversion is the identity function and would pass while broken.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

async function open(url: string, extra: Record<string, { body?: unknown; status?: number }> = {}) {
  const log = scriptedFetch({ ...oversightRoutes(), ...extra })
  window.history.replaceState(null, '', url)
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── deep-linkability (R18) ────────────────────────────────────────────────

test('each filter has a stable URL that resolves back to itself', () => {
  for (const f of filters) {
    const href = hrefForFilter(f.id)
    expect(href).toBe(`/?view=${f.id}`)
    expect(filterFromSearch(new URL(href, 'https://example.invalid').search)).toBe(f.id)
  }
  // A stale or hand-typed value is no filter, not a broken page.
  expect(filterFromSearch('?view=nonsense')).toBe('')
  expect(filterFromSearch('')).toBe('')
})

test('the filter bar is reachable and navigating to one selects it', async () => {
  const { view } = await open('/')
  const links = [...view.container.querySelectorAll('.filter-bar a')].map((a) => a.getAttribute('href'))
  expect(links).toEqual(filters.map((f) => hrefForFilter(f.id)))

  act(() => navigate('/?view=running'))
  await flush()
  const current = view.container.querySelector('.filter-bar a[aria-current="page"]')
  expect(current?.getAttribute('href')).toBe('/?view=running')
})

// ── what-needs-me (R18) ───────────────────────────────────────────────────

test('what-needs-me shows approvals, questions and review-ready work, each linking to its own surface', async () => {
  const { view } = await open('/?view=what-needs-me')
  const text = view.container.textContent ?? ''

  // The approval/question feed.
  expect(text).toContain('ask:ask-audit')
  // The review-ready feed.
  expect(text).toContain('d-notes')
  expect(text).toContain('Ready to review')

  // The honest doors: answering is the inbox's, reviewing is the review
  // surface's — this filter says what is waiting and takes you there.
  const hrefs = [...view.container.querySelectorAll('.rows a')].map((a) => a.getAttribute('href'))
  expect(hrefs).toContain('/inbox/ask%3Aask-audit')
  expect(hrefs).toContain('/deliverables/d-notes')
})

test('a card that is not yours to answer says so, in the server&apos;s own words', async () => {
  const { view } = await open('/?view=what-needs-me')
  const served = (fixtures.approvals() as { items: { not_answerable_reason?: string }[] }).items[0]
  expect(served.not_answerable_reason).toBeTruthy()
  expect(view.container.textContent).toContain(served.not_answerable_reason ?? '')
})

test('empty feeds say nothing is waiting rather than rendering nothing', async () => {
  const { view } = await open('/?view=what-needs-me', {
    'GET /api/approvals': { body: { items: [], cursor: 1 } },
    'GET /api/deliverables?state=in-review': { body: { deliverables: [], cursor: 1, truncated: false } },
  })
  expect(view.container.textContent).toContain('Nothing is waiting on you')
  expect(view.container.textContent).toContain('Nothing is waiting for a review')
})

// ── mine / running / finished-today (R18) ─────────────────────────────────

test('"mine" asks the server for the caller&apos;s own rows rather than sieving them here', async () => {
  const { log } = await open('/?view=mine', { 'GET /api/runs?person=alice': { body: fixtures.runs() } })
  expect(
    log.calls.some((c) => c.path === '/api/runs?person=alice'),
    'mine filtered client-side instead of asking the server',
  ).toBe(true)
})

test('"running" asks for the stored state, and renders what comes back', async () => {
  const running = fixtures.runs() as { runs: { state: string }[] }
  running.runs = running.runs.filter((r) => r.state === 'running')
  const { view, log } = await open('/?view=running', { 'GET /api/runs?status=running': { body: running } })
  expect(log.calls.some((c) => c.path === '/api/runs?status=running')).toBe(true)
  const rows = [...view.container.querySelectorAll('[data-filter="running"] .row')]
  expect(rows.length).toBe(1)
  expect(rows[0].textContent).toContain('t-ship')
})

test('"finished today" sends local midnight as a UTC instant, in a non-UTC zone', async () => {
  // The property that matters, and the reason it is a property rather than a
  // hard-coded string: the instant sent must BE midnight where the reader is,
  // and must be expressed in UTC on the wire.
  const now = new Date('2026-07-29T15:30:00Z')
  const iso = localMidnightISO(now)
  expect(iso.endsWith('Z'), 'a local offset on the wire compares as characters, not as an instant').toBe(true)
  expect(new Date(iso).getHours(), 'the boundary is not local midnight').toBe(0)
  expect(new Date(iso).getMinutes()).toBe(0)

  // And the conversion is doing real work rather than being the identity: in a
  // non-UTC zone, local midnight is never midnight UTC. (In UTC it would be,
  // and this whole test would pass over a broken implementation — which is why
  // vitest.config.ts pins the zone.)
  expect(new Date().getTimezoneOffset(), 'the suite is running in UTC — see vitest.config.ts').not.toBe(0)
  expect(iso, 'local midnight came out as UTC midnight — the zone conversion did nothing').not.toContain('T00:00:00')
})

test('"finished today" asks with ?since= and shows only runs that actually finished', async () => {
  const { view, log } = await open('/?view=finished-today', {
    [`GET /api/runs?since=${encodeURIComponent(localMidnightISO(new Date()))}`]: { body: fixtures.runs() },
  })
  const call = log.calls.find((c) => c.path.startsWith('/api/runs?since='))
  expect(call, 'finished-today did not use the server-side since filter').toBeTruthy()

  const rows = [...view.container.querySelectorAll('[data-filter="finished-today"] .row')]
  // The fixture world has exactly one terminal run; a queued one is not
  // "finished" however recently it moved.
  expect(rows.length).toBe(1)
  expect(rows[0].textContent).toContain('t-notes')
  expect(rows[0].textContent).toContain('completed')
})

test('a filter is live like every other view and marks itself while catching up', async () => {
  const { view } = await open('/?view=running', { 'GET /api/runs?status=running': { body: fixtures.runs() } })
  // The filter renders inside a Section, which carries the freshness marker.
  expect(view.container.querySelector('[data-filter="running"]')).not.toBeNull()
  act(() => FakeSource.last().open())
  await flush()
  expect(view.container.textContent).toContain('Running')
})
