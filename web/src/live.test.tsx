import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { FakeSource, frame, oversightRoutes, scriptedFetch } from './doubles'
import { EventStream, type Status } from './events'
import { boardEventTypes } from './live'
import { navigate } from './router'
import { flush, mount } from './testing'

/**
 * The B6-5 live wiring, driven end to end through the REAL views
 * (P3/CONVENTIONS.md §42; Spec S15.3, S15.12).
 *
 * Everything here is offline: a scripted EventSource, a scripted control plane,
 * no timers of our own and no network. The point of driving App rather than the
 * hook is that the invariants under test — one connection per tab, per-
 * subscriber debt, stale never posing as live — are properties of several views
 * sharing one stream, which a hook in isolation cannot exhibit.
 */

type Harness = { stream: EventStream; statuses: Status[]; timers: number }

function harness(): Harness {
  const statuses: Status[] = []
  let timers = 0
  const stream = new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => {
      timers++
      return 0
    },
    cancel: () => {},
  })
  stream.onStatus((s) => statuses.push(s))
  return {
    stream,
    statuses,
    get timers() {
      return timers
    },
  }
}

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

const reads = (log: { calls: { method: string; path: string }[] }, path: string) =>
  log.calls.filter((c) => c.method === 'GET' && c.path === path).length

test('a view loads its REST snapshot and the tab opens exactly one connection', async () => {
  const log = scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', '/board')
  const h = harness()

  const view = mount(<App stream={h.stream} />)
  await flush()

  expect(reads(log, '/api/tasks'), 'the board did not read its snapshot').toBeGreaterThanOrEqual(1)
  expect(h.stream.opened, 'a tab must hold at most one EventSource').toBe(1)
  expect(view.container.textContent).toContain('Ship the release notes')
  view.unmount()
})

test('navigating between built surfaces never opens a second connection', async () => {
  scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', '/board')
  const h = harness()

  const view = mount(<App stream={h.stream} />)
  await flush()
  act(() => navigate('/'))
  await flush()
  act(() => navigate('/board'))
  await flush()

  expect(h.stream.opened, 'a navigation re-opened the stream').toBe(1)
  expect(FakeSource.made.length).toBe(1)
  view.unmount()
})

test('the subscription declares its event types, and only those frames trigger a re-read', async () => {
  const log = scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', '/board')
  const h = harness()

  const view = mount(<App stream={h.stream} />)
  await flush()
  act(() => FakeSource.last().open())
  await flush()
  const settled = reads(log, '/api/tasks')

  // A declared type is a trigger.
  act(() => FakeSource.last().send('run.state_changed', frame(101, 'run.state_changed')))
  await flush()
  expect(reads(log, '/api/tasks'), 'a declared frame did not trigger a re-read').toBe(settled + 1)

  // A type this view never declared is not — and cannot be, because the
  // subscription is what registers the listener at all.
  expect(boardEventTypes).not.toContain('memory.written')
  act(() => FakeSource.last().send('memory.written', frame(102, 'memory.written', ['inbox'])))
  await flush()
  expect(reads(log, '/api/tasks'), 'an undeclared frame reached the view').toBe(settled + 1)
  view.unmount()
})

test('a resnapshot signal makes the view re-read, and only then does the stream read live', async () => {
  const log = scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', '/board')
  const h = harness()

  const view = mount(<App stream={h.stream} />)
  await flush()
  act(() => FakeSource.last().open())
  await flush()
  expect(h.stream.status, 'the stream should be live once every view settled').toBe('live')

  const before = reads(log, '/api/tasks')
  // A frame at or below the last delivered sequence: the stream is not where
  // we think it is, so continuity is NOT PROVEN and everyone re-snapshots.
  act(() => FakeSource.last().send('run.state_changed', frame(150, 'run.state_changed')))
  await flush()
  act(() => FakeSource.last().send('run.state_changed', frame(2, 'run.state_changed')))

  // Mid-debt: the view owes a re-read and must not look fresh.
  expect(h.stream.status).toBe('catching-up')
  await flush()
  expect(reads(log, '/api/tasks'), 'the rewind did not trigger a re-read').toBeGreaterThan(before + 1)
  expect(h.stream.status, 'the stream stayed behind after every view re-read').toBe('live')
  view.unmount()
})

test('a view that owes a re-snapshot never renders its data as fresh', async () => {
  // The control plane answers this read only when the test releases it, so the
  // mid-debt window is observable rather than a race.
  let release: () => void = () => {}
  const gate = new Promise<void>((resolve) => {
    release = () => resolve()
  })
  const routes = oversightRoutes()
  const base = routes['GET /api/tasks']
  let first = true
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/tasks' && !first) await gate
      if (path === '/api/tasks') first = false
      const route = routes[`${init?.method ?? 'GET'} ${path}`] ?? base
      return { ok: true, status: 200, statusText: '', json: async () => route.body ?? {} } as Response
    }),
  )
  window.history.replaceState(null, '', '/board')
  const h = harness()

  const view = mount(<App stream={h.stream} />)
  await flush()
  expect(view.container.textContent).toContain('Ship the release notes')

  act(() => FakeSource.last().open())
  await flush()

  // The re-read is in flight. The rows are still on screen — which is right,
  // an empty board would be a worse lie — but they are MARKED.
  const block = view.container.querySelector('.block[data-stale="true"]')
  expect(block, 'a view owing a re-snapshot rendered nothing stale-marked').not.toBeNull()
  expect(view.container.textContent).toContain('Catching up')
  expect(h.stream.status).not.toBe('live')

  release()
  await flush()
  expect(view.container.querySelector('.block[data-stale="true"]')).toBeNull()
  expect(h.stream.status).toBe('live')
  view.unmount()
})

test('no view polls: nothing is scheduled while the feed is healthy', async () => {
  scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', '/')
  const h = harness()

  const view = mount(<App stream={h.stream} />)
  await flush()
  act(() => FakeSource.last().open())
  await flush()
  act(() => FakeSource.last().send('usage.recorded', frame(200, 'usage.recorded', ['fleet'])))
  await flush()

  // The only scheduling this client ever does is the reconnect backoff, which a
  // healthy feed never reaches. A ticker or a poll loop would show up here.
  expect(h.timers, 'something scheduled work on a healthy feed — a ticker or a poll loop').toBe(0)
  view.unmount()
})

test('a burst of frames coalesces instead of one read per frame', async () => {
  const log = scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', '/board')
  const h = harness()

  const view = mount(<App stream={h.stream} />)
  await flush()
  act(() => FakeSource.last().open())
  await flush()
  const settled = reads(log, '/api/tasks')

  act(() => {
    for (let i = 0; i < 8; i++) FakeSource.last().send('run.state_changed', frame(300 + i, 'run.state_changed'))
  })
  await flush()

  const added = reads(log, '/api/tasks') - settled
  expect(added, 'a frame burst produced a read per frame').toBeLessThan(8)
  expect(added, 'the burst produced no read at all').toBeGreaterThan(0)
  view.unmount()
})
