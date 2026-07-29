import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import type { Meters } from './api'
import { FakeSource, fixtures, oversightRoutes, scriptedFetch } from './doubles'
import { EventStream } from './events'
import { flush, mount } from './testing'

/**
 * Fleet overview (Spec S15.5 ¶4; 9.3; S2.5; S3.10; D2), driven against the
 * golden meters fixture.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

async function fleet(extra: Record<string, { body?: unknown; status?: number }> = {}) {
  scriptedFetch({ ...oversightRoutes(), ...extra })
  window.history.replaceState(null, '', '/fleet')
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return view
}

const setSelect = (el: HTMLSelectElement, value: string) => {
  act(() => {
    const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!
    setter.call(el, value)
    el.dispatchEvent(new Event('change', { bubbles: true }))
  })
}

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

test('every fleet row says whose account it is', async () => {
  const view = await fleet()
  const rows = [...view.container.querySelectorAll('.fleet-lanes tbody tr')]
  expect(rows.length).toBeGreaterThan(0)
  for (const row of rows) {
    expect(row.getAttribute('data-owner'), 'a fleet row does not name its owner (D2/S3.10)').toBeTruthy()
    expect(row.querySelector('.owner')).not.toBeNull()
  }
  // Three owners are present, which is what makes the no-cross-owner-total
  // rule meaningful rather than vacuous.
  expect(new Set(rows.map((r) => r.getAttribute('data-owner')))).toEqual(new Set(['alice', 'bob', 'op']))
})

test('nothing is summed across owners: there is no total row and no total figure', async () => {
  const view = await fleet()
  const text = (view.container.textContent ?? '').toLowerCase()
  // The receipt has a per-run total, which is the server's own figure; the
  // fleet has none, because a cross-person total would be the client doing
  // arithmetic over money.
  expect(text, 'the fleet rendered a cross-owner total').not.toContain('household total')
  expect(text).not.toContain('all owners')
  const meters = fixtures.meters() as unknown as Meters
  expect(meters.lanes.length).toBeGreaterThan(1)
})

test('a parked lane whose latest park carries no marker says so, and an unparked lane shows nothing', async () => {
  const view = await fleet()
  const parked = view.container.querySelector('.fleet-lanes tbody tr[data-lane="zai"]')!
  // The served truth: this lane has parked runs, and the most recent park
  // carried no reset time. The honest absence is the whole point — inventing a
  // horizon here is exactly the failure ParkedUntil exists to prevent.
  const served = (fixtures.meters() as unknown as Meters).lanes.find((l) => l.lane === 'zai')!
  expect(served.parked_runs).toBeGreaterThan(0)
  expect(served.parked_until).toBeNull()
  expect(parked.textContent).toContain('parked, no horizon given')

  const idle = view.container.querySelector('.fleet-lanes tbody tr[data-lane="local"]')!
  expect(idle.textContent, 'an unparked lane was given a park horizon').not.toContain('parked')
})

test('a lane whose park DOES carry a horizon renders it verbatim', async () => {
  const meters = fixtures.meters() as unknown as Meters
  const lane = meters.lanes.find((l) => l.lane === 'zai')!
  lane.parked_until = '2026-07-20T12:00:00Z'
  const view = await fleet({ 'GET /api/meters': { body: meters } })
  const row = view.container.querySelector('.fleet-lanes tbody tr[data-lane="zai"]')!
  expect(row.textContent).toContain('parked until')
  expect(row.textContent).toContain('2026-07-20T12:00:00Z')
})

test('the person and lane filters narrow the display only', async () => {
  const view = await fleet()
  const selects = [...view.container.querySelectorAll('.fleet-filters select')] as HTMLSelectElement[]
  expect(selects.length).toBe(2)

  setSelect(selects[0], 'bob')
  await flush()
  const owners = [...view.container.querySelectorAll('.fleet-lanes tbody tr')].map((r) => r.getAttribute('data-owner'))
  expect(owners).toEqual(['bob'])

  setSelect(selects[0], '')
  setSelect(selects[1], 'local')
  await flush()
  const lanes = [...view.container.querySelectorAll('.fleet-lanes tbody tr')].map((r) => r.getAttribute('data-lane'))
  expect(lanes).toEqual(['local'])
})

test('limit-event history and the per-person / per-period cost views render at their own grains', async () => {
  const view = await fleet()
  const text = view.container.textContent ?? ''
  expect(text).toContain('Limit events')
  expect(text).toContain('Per person')
  expect(text).toContain('Per period')
  // Served at the view's own grain: the burn-rate answer's own columns.
  expect(text).toContain('usd_per_day')
})

test('the local seat and GPU seams render as the NAMED absences they are', async () => {
  const view = await fleet()
  const text = view.container.textContent ?? ''
  expect(text).toContain('Local duty seats')
  expect(text).toContain('GPU / VRAM')
  // The distinction that matters: an unwired instrument, not an idle tier.
  expect(text).toContain('not wired at v0')
  expect(text, 'an unwired seat count was rendered as a reading').not.toMatch(/0 seats|0 GB/)
})

test('an unreadable meters surface leaves the fleet honest rather than empty-looking', async () => {
  const view = await fleet({ 'GET /api/meters': { status: 503, body: { error: 'not_wired', detail: 'no meters here' } } })
  expect(view.container.textContent).toContain('no meters here')
  // The seams still render: they are facts about this build, not about the read.
  expect(view.container.textContent).toContain('Local duty seats')
})
