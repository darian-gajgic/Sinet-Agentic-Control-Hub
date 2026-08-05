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
  expect(text).toContain('ask:ask-ship')
  // The review-ready feed.
  expect(text).toContain('d-changelog')
  expect(text).toContain('Ready to review')

  // The honest doors: answering is the inbox's, reviewing is the review
  // surface's — this filter says what is waiting and takes you there.
  const hrefs = [...view.container.querySelectorAll('.rows a')].map((a) => a.getAttribute('href'))
  expect(hrefs).toContain('/inbox/ask%3Aask-ship')
  expect(hrefs).toContain('/deliverables/d-changelog')
})

test('a card that is not yours to answer says so, in the server&apos;s own words', async () => {
  // Read as somebody who does NOT own the cards: `answerable` is computed per
  // caller, so this is a different served body, not a different render.
  const { view } = await open('/?view=what-needs-me', {
    'GET /api/approvals': { body: fixtures.approvals() },
  })
  // The operator's body carries cards of both kinds; the one under test is a
  // card the operator can SEE and cannot ANSWER, which is exactly the D10 line.
  const served = (fixtures.approvals() as { items: { not_answerable_reason?: string }[] }).items.find(
    (i) => i.not_answerable_reason !== undefined,
  )
  expect(served, 'the served body carries no unanswerable card, so this asserts nothing').toBeTruthy()
  expect(view.container.textContent).toContain(served?.not_answerable_reason ?? '')
})

test('a card that IS yours to answer says so — the other direction (drain r2 R4b)', async () => {
  const { view } = await open('/?view=what-needs-me')
  const served = (fixtures.approvalsMine() as { items: { id: string; answerable: boolean }[] }).items
  expect(served.some((i) => i.answerable), 'the fixture carries no answerable card').toBe(true)

  const text = view.container.textContent ?? ''
  expect(text, 'the answerable branch never renders').toContain('yours to answer')
  // And the honest absence is NOT rendered for a card that is answerable.
  const rows = [...view.container.querySelectorAll('.rows .row')]
  const mine = rows.find((r) => r.textContent?.includes('ask:ask-ship'))!
  expect(mine.textContent).toContain('yours to answer')
  expect(mine.querySelector('.absent'), 'an answerable card carried a not-answerable reason').toBeNull()
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

// ── D5: relative BESIDE verbatim UTC, on every live site here (P3-UI-4) ───

/**
 * assertLiveStamp is the D5 live contract read off one rendered call site.
 *
 * THE ONE FORBIDDEN OUTCOME is a relative label that REPLACED the instant it
 * describes: "3m ago" is computed against a device clock nobody controls, and
 * between frames it freezes with the data it describes — understating age on a
 * past stamp and overstating the time left on a future one — so the served
 * string it sits beside is what stays true.
 *
 * The check is clock-independent on purpose: it asserts the rendered text ENDS
 * with the served instant and is longer than it, rather than naming a label that
 * would depend on when the suite runs.
 */
function assertLiveStamp(stamp: Element | null | undefined, served: string, what: string): void {
  expect(stamp, `${what}: no timestamp rendered`).toBeTruthy()
  expect(stamp!.getAttribute('dateTime'), `${what}: dateTime is not the served instant`).toBe(served)
  expect(stamp!.textContent, `${what}: the verbatim UTC was dropped`).toBe(served)
  const beside = stamp!.parentElement!.parentElement!.textContent ?? ''
  expect(beside.endsWith(served), `${what}: the instant is not rendered beside its label`).toBe(true)
  expect(beside.length, `${what}: no relative label renders beside the instant`).toBeGreaterThan(served.length)
}

test('what-needs-me renders relative time beside the verbatim instant, never instead of it', async () => {
  const { view } = await open('/?view=what-needs-me')
  const approvals = fixtures.approvalsMine() as unknown as { items: { id: string; observed_ts: string }[] }
  const item = approvals.items[0]
  const row = [...view.container.querySelectorAll('.rows .row')].find((r) => r.textContent?.includes(item.id))!
  expect(row, 'the served card did not reach the screen').toBeDefined()
  assertLiveStamp(row.querySelector('time'), item.observed_ts, 'an approval’s observed_ts')

  const dels = fixtures.deliverablesInReview() as unknown as {
    deliverables: { deliverable_id: string; updated_ts: string }[]
  }
  const d = dels.deliverables[0]
  const drow = [...view.container.querySelectorAll('.rows .row')].find((r) => r.textContent?.includes(d.deliverable_id))!
  assertLiveStamp(drow.querySelector('time'), d.updated_ts, 'a deliverable’s updated_ts')
  view.unmount()
})

test('a parked run carries BOTH live instants: its horizon and its last activity', async () => {
  const served = fixtures.runs() as unknown as {
    runs: { run_id: string; task_id: string; parked_until: string | null; last_activity_ts: string | null }[]
  }
  const parked = served.runs.find((r) => r.parked_until)!
  expect(parked, 'the fixture world no longer parks a run with a horizon').toBeDefined()
  const { view } = await open('/?view=mine', { 'GET /api/runs?person=alice': { body: fixtures.runs() } })

  const row = [...view.container.querySelectorAll('[data-filter] .row')].find((r) =>
    r.textContent?.includes(parked.task_id),
  )!
  expect(row, 'the served run did not reach the screen').toBeDefined()
  const stamps = [...row.querySelectorAll('time')]
  // The park horizon is a FUTURE instant, and a frozen future label overstates
  // the time left — which is precisely why the instant never leaves.
  assertLiveStamp(stamps[0], parked.parked_until!, 'a park horizon')
  expect(row.textContent).toContain('parked until')
  assertLiveStamp(stamps[1], parked.last_activity_ts!, 'a run’s last_activity_ts')
  view.unmount()
})

test('an unrecorded instant says so, and an unreadable one renders verbatim with no label', async () => {
  // R20, both arms, on a live surface. An absence is RENDERED, not filled; and
  // an instant this client could not parse still renders exactly as served,
  // because inventing "3m ago" for something we could not read is the one thing
  // the primitive must never do.
  const blank = fixtures.runs() as unknown as { runs: Record<string, unknown>[] }
  blank.runs = [{ ...blank.runs[0], parked_until: null, last_activity_ts: '' }]
  const { view } = await open('/?view=mine', { 'GET /api/runs?person=alice': { body: blank } })
  const row = [...view.container.querySelectorAll('[data-filter] .row')][0]
  expect(row.querySelector('time'), 'an empty instant rendered a <time> anyway').toBeNull()
  const absences = [...row.querySelectorAll('.absent')].map((n) => n.textContent)
  expect(absences, 'the primitive’s absence arm did not render').toContain('not recorded')
  // ParkedUntil's own absence arm is byte-kept through the migration: a park
  // with no horizon still says so rather than inventing a time to be live about.
  expect(absences, 'the park-with-no-horizon arm moved').toContain('parked, no horizon given')
  view.unmount()

  const odd = fixtures.runs() as unknown as { runs: Record<string, unknown>[] }
  odd.runs = [{ ...odd.runs[0], parked_until: null, last_activity_ts: 'whenever' }]
  const { view: v2 } = await open('/?view=mine', { 'GET /api/runs?person=alice': { body: odd } })
  const stamp = v2.container.querySelector('[data-filter] .row time')!
  expect(stamp.getAttribute('dateTime')).toBe('whenever')
  expect(stamp.textContent).toBe('whenever')
  expect(stamp.parentElement!.textContent, 'a label was invented for an instant nobody could read').toBe('whenever')
  v2.unmount()
})
