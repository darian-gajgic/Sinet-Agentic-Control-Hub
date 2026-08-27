import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { DescribeGoal } from './Intake'
import type { PinnableLane } from './api'
import { FakeSource, fixtures, type Scripted, scriptedFetch, signedIn } from './doubles'
import { EventStream } from './events'
import { click, flush, mount, typeInto } from './testing'

/**
 * The per-task lane-pin picker (P3-LN-10; S00.9 A13).
 *
 * BEHAVIOR-CONTRACT probes on the send envelope and the honesty states —
 * what a picked lane puts on the wire, what the default keeps OFF the wire,
 * what the picker enumerates (the P3-LN-10a fixture, never a list this file
 * spells), and what the refused/degraded/empty faces say. The pixels are
 * judged in real Chrome (FRONTEND.md rule 5).
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

/** The routes the ask box performs at mount. The pinnable set is the GOLDEN
 *  FIXTURE — the backend's own snapshot — so what these probes prove is the
 *  contract, not a shape somebody imagined. The session/task routes serve the
 *  POST-SUBMIT face (NoCardYet re-reads the born task over the SHARED stream,
 *  which the stubbed EventSource keeps inert). */
const baseRoutes = (): Record<string, Scripted> => ({
  'GET /api/auth/session': signedIn,
  'GET /api/projects': { body: { projects: [] } },
  'GET /api/intake/pinnable-lanes': { body: fixtures.pinnableLanes() },
})

/** One born task, the minimal detail the no-card face reads. */
const bornTask = (id: string): Scripted => ({
  body: { task_id: id, title: 'A haiku', kanban_status: 'intake', owner: 'alice', runs: [] },
})

async function openDoor(extra: Record<string, Scripted> = {}) {
  const log = scriptedFetch({ ...baseRoutes(), ...extra })
  const view = mount(<DescribeGoal stream={inertStream()} />)
  await flush()
  return { view, log }
}

function fixtureLanes(): PinnableLane[] {
  return (fixtures.pinnableLanes() as { lanes: PinnableLane[] }).lanes
}

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
  // The post-submit no-card face subscribes to the SHARED stream (it takes no
  // stream prop, by design); jsdom has no EventSource, so the double stands in.
  vi.stubGlobal('EventSource', FakeSource)
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

test('closed by default, the picker enumerates the SERVED set when opened — never a hardcoded list', async () => {
  const { view } = await openDoor()
  // Closed: no lane chip exists yet, and the collapsed face states the
  // default out loud (the pin is opt-in per task).
  expect(view.container.querySelector('[data-lane-choice]')).toBeNull()
  expect(view.container.querySelector('[data-ask="lane"]')?.textContent).toContain('the platform chooses')

  click(view.container.querySelector('[data-lane-open-act]'))

  // Every PINNABLE fixture row is a chip, by its served name.
  const lanes = fixtureLanes()
  const pinnable = lanes.filter((l) => l.pinnable)
  expect(pinnable.length, 'the fixture must carry pinnable lanes for this probe to mean anything').toBeGreaterThan(1)
  for (const l of pinnable) {
    expect(view.container.querySelector(`[data-lane-choice="${l.lane}"]`), `a chip for ${l.lane}`).not.toBeNull()
  }
  // The default choice stands FIRST and active: unpinned is the ordinary case.
  const chips = [...view.container.querySelectorAll('[data-lane-choice]')]
  expect(chips[0]?.getAttribute('data-lane-choice')).toBe('')
  expect(chips[0]?.getAttribute('data-active')).toBe('true')

  // An UNPINNABLE lane is never offered as a dead control (finding-5's rule)
  // — it is stated as a fact, with the platform's own sentence VERBATIM.
  const local = lanes.find((l) => !l.pinnable)
  expect(local, 'the fixture must carry the unpinnable local lane').toBeDefined()
  expect(view.container.querySelector(`[data-lane-choice="${local?.lane ?? ''}"]`)).toBeNull()
  const stated = view.container.querySelector(`[data-lane-unpinnable="${local?.lane ?? ''}"]`)
  expect(stated).not.toBeNull()
  expect(stated?.textContent).toContain(local?.not_pinnable ?? '--missing--')
})

test('the default submission carries NO pinned_lane member — byte-identical to the pre-pin envelope', async () => {
  const { view, log } = await openDoor({
    'POST /api/intake/requests': { body: { task_id: 't-1', title: 'A haiku', kanban_status: 'intake', owner: 'alice' } },
    'GET /api/approvals': { body: { items: [] } },
    'GET /api/tasks/t-1': bornTask('t-1'),
  })
  typeInto(view.container.querySelector('[data-ask="text"]') as HTMLTextAreaElement, 'Write a haiku about SQLite.')
  click(view.container.querySelector('[data-ask="submit"]'))
  await flush()
  const posted = log.calls.find((c) => c.method === 'POST' && c.path === '/api/intake/requests')
  expect(posted).toBeDefined()
  expect('pinned_lane' in (posted?.body as Record<string, unknown>)).toBe(false)
})

test('a picked lane rides the wire as pinned_lane, verbatim; re-picking the default clears the pin', async () => {
  const { view, log } = await openDoor({
    'POST /api/intake/requests': { body: { task_id: 't-2', title: 'A haiku', kanban_status: 'intake', owner: 'alice' } },
    'GET /api/approvals': { body: { items: [] } },
    'GET /api/tasks/t-2': bornTask('t-2'),
  })
  click(view.container.querySelector('[data-lane-open-act]'))
  const kimi = fixtureLanes().find((l) => l.pinnable && l.lane !== '')
  const name = kimi?.lane ?? ''
  click(view.container.querySelector(`[data-lane-choice="${name}"]`))
  // The standing pin says itself where the send is armed…
  expect(view.container.querySelector(`[data-lane-pinned="${name}"]`)).not.toBeNull()
  // …and re-picking the default withdraws it: nothing lingers half-armed.
  click(view.container.querySelector('[data-lane-choice=""]'))
  expect(view.container.querySelector('[data-lane-pinned]')).toBeNull()

  click(view.container.querySelector(`[data-lane-choice="${name}"]`))
  typeInto(view.container.querySelector('[data-ask="text"]') as HTMLTextAreaElement, 'Write a haiku about SQLite.')
  click(view.container.querySelector('[data-ask="submit"]'))
  await flush()
  const posted = log.calls.find((c) => c.method === 'POST' && c.path === '/api/intake/requests')
  expect((posted?.body as Record<string, unknown>).pinned_lane).toBe(name)
})

test('a refused pin renders the refusal in the server\'s own words and keeps the picker armed', async () => {
  // The server's whole sentence, which names the lanes that ARE pinnable —
  // rendered verbatim, never re-classified client-side (§30/§38).
  const detail =
    'lane "zai" is not one this platform holds flat-rate coverage on, and subscription coverage binds every ' +
    'choice (S08.8 step 3). The lanes a task may pin are: "anthropic", "kimi"'
  const { view } = await openDoor({
    'POST /api/intake/requests': { status: 400, body: { error: 'lane_pin_refused', detail } },
  })
  click(view.container.querySelector('[data-lane-open-act]'))
  const lane = fixtureLanes().find((l) => l.pinnable)?.lane ?? ''
  click(view.container.querySelector(`[data-lane-choice="${lane}"]`))
  typeInto(view.container.querySelector('[data-ask="text"]') as HTMLTextAreaElement, 'Write a haiku about SQLite.')
  click(view.container.querySelector('[data-ask="submit"]'))
  await flush()
  const refusal = view.container.querySelector('.door-refusal')
  expect(refusal, 'the refusal renders where the send was armed').not.toBeNull()
  expect(refusal?.textContent).toContain(`The pin to lane "${lane}" was refused — nothing was submitted.`)
  expect(refusal?.textContent).toContain(detail)
  // Nothing navigated and nothing was dropped: the choice still stands, so
  // the person can re-pick and send again.
  expect(view.container.querySelector(`[data-lane-choice="${lane}"]`)?.getAttribute('data-active')).toBe('true')
})

test('a failed read degrades OUT LOUD, and unpinned submission keeps working', async () => {
  const { view, log } = await openDoor({
    'GET /api/intake/pinnable-lanes': {
      status: 503,
      body: { error: 'not_wired', detail: 'the intake pipeline is not wired in this process' },
    },
    'POST /api/intake/requests': { body: { task_id: 't-3', title: 'A haiku', kanban_status: 'intake', owner: 'alice' } },
    'GET /api/approvals': { body: { items: [] } },
    'GET /api/tasks/t-3': bornTask('t-3'),
  })
  click(view.container.querySelector('[data-lane-open-act]'))
  const degraded = view.container.querySelector('[data-lane-degraded]')
  expect(degraded, 'the failed read is a stated state, never a vanished control').not.toBeNull()
  expect(degraded?.textContent).toContain('could not be read')
  expect(degraded?.textContent).toContain('503')
  // No lane chip is offered off a failed read — only the working default.
  expect(view.container.querySelectorAll('[data-lane-choice]')).toHaveLength(1)
  typeInto(view.container.querySelector('[data-ask="text"]') as HTMLTextAreaElement, 'Write a haiku about SQLite.')
  click(view.container.querySelector('[data-ask="submit"]'))
  await flush()
  const posted = log.calls.find((c) => c.method === 'POST' && c.path === '/api/intake/requests')
  expect(posted, 'the unpinned path survives the picker\'s own failure').toBeDefined()
  expect('pinned_lane' in (posted?.body as Record<string, unknown>)).toBe(false)
})

test('an empty served set is an honest absence with its meaning stated', async () => {
  const { view } = await openDoor({ 'GET /api/intake/pinnable-lanes': { body: { lanes: [] } } })
  click(view.container.querySelector('[data-lane-open-act]'))
  const empty = view.container.querySelector('[data-lane-empty]')
  expect(empty).not.toBeNull()
  expect(empty?.textContent).toContain('No lane on this platform can be pinned')
  expect(view.container.querySelectorAll('[data-lane-choice]')).toHaveLength(1)
})
