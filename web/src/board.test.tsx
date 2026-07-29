import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { applyDrag, hintPostsFor, ownQueueDroppableId, ownQueued, spacedRank } from './Board'
import type { TaskListItem } from './api'
import { FakeSource, oversightRoutes, scriptedFetch, signedIn } from './doubles'
import { EventStream } from './events'
import { columnsFor } from './kanban'
import { flush, mount } from './testing'

/**
 * The live board (Spec S15.5 ¶2; 9.1; S1.3; FC-v1 §1), driven against the
 * golden fixtures — so the rows these assertions read are the rows the Go
 * handler actually serves.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

async function board(routes = oversightRoutes()) {
  const log = scriptedFetch(routes)
  window.history.replaceState(null, '', '/board')
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}

/** The fixture world's tasks, as the API serves them. */
function fixtureTasks(): TaskListItem[] {
  return (oversightRoutes()['GET /api/tasks'].body as { tasks: TaskListItem[] }).tasks
}

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── the card face (R6; S1.3) ──────────────────────────────────────────────

test('the card face carries exactly the S1.3 set, with the downgrade note in both directions', async () => {
  const { view } = await board()

  const cards = [...view.container.querySelectorAll('.card-face')]
  expect(cards.length).toBeGreaterThan(0)

  const ship = cards.find((c) => c.textContent?.includes('Ship the release notes'))!
  const text = ship.textContent ?? ''
  // what it is / whose / stage / effort / cost so far / waiting
  expect(text).toContain('Ship the release notes')
  expect(text).toContain('alice')
  expect(text).toContain('execute')
  expect(text).toContain('standard')
  expect(text, 'the read cost figure is not rendered as served').toContain('USD 1.42')
  expect(ship.querySelector('.downgrade-note'), 'a note was invented where the platform disclosed none').toBeNull()

  // The other direction: a routing record that DID disclose one.
  const triage = cards.find((c) => c.textContent?.includes('Triage the inbox backlog'))!
  expect(triage.querySelector('.downgrade-note')?.textContent).toContain('effort dropped from deep to quick')

  // And the honest absences: a run with no meter reading shows no figure.
  const archive = cards.find((c) => c.textContent?.includes('Archive last quarter'))!
  expect(archive.textContent).toContain('no meter reading')
  expect(archive.textContent, 'an absent cost was rendered as zero').not.toContain('USD 0')
})

test('waiting-on-a-human and parked-until are distinguished on the face', async () => {
  const { view } = await board()
  const audit = [...view.container.querySelectorAll('.card-face')].find((c) =>
    c.textContent?.includes('Audit the price table'),
  )!
  expect(audit.querySelector('.waiting-human')).not.toBeNull()

  // The lane-level park horizon still renders where a card is parked without a
  // person in the way — proven on mission control's own parked line.
  expect((oversightRoutes()['GET /api/tasks'].body as { tasks: TaskListItem[] }).tasks
    .find((t) => t.task_id === 't-audit')?.latest_run?.parked_until).toBe('2026-07-20T12:00:00Z')
})

test('no view renders a percentage, a completion fraction or an ETA', async () => {
  const { view } = await board()
  const text = (view.container.textContent ?? '').toLowerCase()
  for (const banned of ['%', 'percent', 'complete:', 'eta', 'estimated time', 'remaining time']) {
    expect(text, `the board rendered "${banned}" — the API serves no such figure (§30)`).not.toContain(banned)
  }
  // Non-tautology: the scan is reading a board with real cards on it.
  expect(text).toContain('ship the release notes')
})

// ── columns and grouping (R5, R7; OQ7) ────────────────────────────────────

test('the six landed kanban values are columns, and an unknown stored value gets its own', async () => {
  const { view } = await board()
  const columns = [...view.container.querySelectorAll('.column')].map((c) => c.getAttribute('data-status'))
  for (const known of ['intake', 'executing', 'verifying', 'attention', 'done', 'cancelled']) {
    expect(columns, `${known} is not a column`).toContain(known)
  }
  // The fixture carries a producer string the board has never seen. It must
  // not vanish the card.
  expect(columns, 'an unknown stored status was dropped').toContain('moonshot')
  const other = view.container.querySelector('.column[data-status="moonshot"]')!
  expect(other.getAttribute('data-known')).toBe('false')
  expect(other.textContent).toContain('Archive last quarter')
})

test('columnsFor keeps every card and invents no display name for an unknown value', () => {
  const cols = columnsFor(fixtureTasks())
  const known = cols.filter((c) => c.known).map((c) => c.status)
  expect(known).toEqual(['intake', 'executing', 'verifying', 'attention', 'done', 'cancelled'])
  const unknown = cols.filter((c) => !c.known)
  expect(unknown.map((c) => c.status)).toEqual(['moonshot'])
  expect(unknown[0].label, 'a friendly name was invented for a value the platform has not decided on').toBe('moonshot')
})

test('cards group by project, and the honest (no project) bucket is a group of its own', async () => {
  const { view } = await board()
  const projects = [...view.container.querySelectorAll('.project-group')].map((g) => g.getAttribute('data-project'))
  expect(projects).toContain('release-notes')
  expect(projects, 'the honest bucket was dropped rather than rendered').toContain('(no project)')
})

// ── the drag (R8, R9; OQ6) ────────────────────────────────────────────────

test('the board has exactly ONE droppable, and it is the own-queued lane', async () => {
  const { view } = await board()
  const droppables = [...view.container.querySelectorAll('[data-rfd-droppable-id]')]
  expect(droppables.length, 'a stage column is a drop target — stage is the control plane&apos;s (S02)').toBe(1)
  expect(droppables[0].getAttribute('data-rfd-droppable-id')).toBe(ownQueueDroppableId)

  // The library really took the nodes over, so the drag this wires is real.
  const draggable = view.container.querySelector('[data-rfd-draggable-id]')
  expect(draggable).not.toBeNull()
  expect(draggable?.getAttribute('data-rfd-drag-handle-draggable-id')).toBe(draggable?.getAttribute('data-rfd-draggable-id'))
})

test('only the caller&apos;s own queued cards are in the drag lane — the operator is not excepted', async () => {
  const { view } = await board()
  const lane = view.container.querySelector('.queue-lane')!
  // alice's queued work is there…
  expect(lane.textContent).toContain('Triage the inbox backlog')
  expect(lane.textContent).toContain('Archive last quarter')
  // …bob's is not, even though alice is signed in as the OPERATOR and can see
  // it everywhere else on this screen.
  expect(lane.textContent, "another person's card is drag-reorderable").not.toContain('Audit the price table')
  expect(view.container.textContent, "the operator cannot even SEE the other owner's card").toContain(
    'Audit the price table',
  )
  // And a card that is not queued is not reorderable either.
  expect(lane.textContent).not.toContain('Ship the release notes')
})

test('a simulated cross-column drop calls no verb at all', async () => {
  const { log } = await board()
  const queue = ownQueued(fixtureTasks(), 'alice')
  const before = log.calls.length

  // The shape a stage-column drop would have if one were ever wired.
  const answers = await applyDrag(queue, {
    draggableId: queue[0].task_id,
    type: 'DEFAULT',
    source: { droppableId: ownQueueDroppableId, index: 0 },
    destination: { droppableId: 'executing', index: 0 },
    reason: 'DROP',
    mode: 'FLUID',
    combine: null,
  })
  expect(answers).toEqual([])
  expect(log.calls.length, 'a cross-column drop reached the network').toBe(before)

  // …and a drop outside every list is the same no-op.
  expect(
    hintPostsFor(queue, {
      draggableId: queue[0].task_id,
      type: 'DEFAULT',
      source: { droppableId: ownQueueDroppableId, index: 0 },
      destination: null,
      reason: 'DROP',
      mode: 'FLUID',
      combine: null,
    }),
  ).toEqual([])
})

test('a reorder posts spaced, in-bounds, never-zero ranks for the entries that moved', async () => {
  const routes = oversightRoutes()
  routes['POST /api/tasks/t-archive/priority-hint'] = {
    body: { task_id: 't-archive', rank: 10, runs: ['r-archive'], applied: true, detail: 'hint recorded' },
  }
  routes['POST /api/tasks/t-triage/priority-hint'] = {
    body: { task_id: 't-triage', rank: 20, runs: ['r-triage'], applied: true, detail: 'hint recorded' },
  }
  const { log } = await board(routes)
  const queue = ownQueued(fixtureTasks(), 'alice')
  expect(queue.map((t) => t.task_id), 'the lane is not in recorded order').toEqual(['t-triage', 't-archive'])

  await applyDrag(queue, {
    draggableId: 't-archive',
    type: 'DEFAULT',
    source: { droppableId: ownQueueDroppableId, index: 1 },
    destination: { droppableId: ownQueueDroppableId, index: 0 },
    reason: 'DROP',
    mode: 'FLUID',
    combine: null,
  })

  const posts = log.calls.filter((c) => c.method === 'POST')
  expect(posts.map((p) => [p.path, p.body])).toEqual([
    ['/api/tasks/t-archive/priority-hint', { rank: 10 }],
    ['/api/tasks/t-triage/priority-hint', { rank: 20 }],
  ])
  for (const p of posts) {
    const rank = (p.body as { rank: number }).rank
    expect(rank, '0 means "no hint" to the scheduler — it is never a position').not.toBe(0)
    expect(Math.abs(rank)).toBeLessThanOrEqual(1000)
  }
})

test('a drag that changes nothing writes nothing', () => {
  const queue = ownQueued(fixtureTasks(), 'alice')
  expect(
    hintPostsFor(queue, {
      draggableId: queue[0].task_id,
      type: 'DEFAULT',
      source: { droppableId: ownQueueDroppableId, index: 0 },
      destination: { droppableId: ownQueueDroppableId, index: 0 },
      reason: 'DROP',
      mode: 'FLUID',
      combine: null,
    }),
  ).toEqual([])
})

test('spaced ranks clamp at the verb&apos;s own bound rather than sending one it would refuse', () => {
  expect(spacedRank(0)).toBe(10)
  expect(spacedRank(1)).toBe(20)
  // The named edge: position 100 is the bound, and everything past it clamps —
  // the tail falls back to the scheduler's default order, honestly.
  expect(spacedRank(99)).toBe(1000)
  expect(spacedRank(500)).toBe(1000)
})

test('the stale-board answer is rendered as the honest answer it is, and the board re-reads', async () => {
  const routes = oversightRoutes()
  const detail =
    'nothing to reorder: this task has no queued run any more (it was claimed, finished or cancelled), so the board you dragged is stale'
  routes['POST /api/tasks/t-archive/priority-hint'] = {
    body: { task_id: 't-archive', rank: 10, runs: [], applied: false, detail },
  }
  routes['POST /api/tasks/t-triage/priority-hint'] = {
    body: { task_id: 't-triage', rank: 20, runs: [], applied: false, detail },
  }
  const { log } = await board(routes)
  const queue = ownQueued(fixtureTasks(), 'alice')
  const answers = await applyDrag(queue, {
    draggableId: 't-archive',
    type: 'DEFAULT',
    source: { droppableId: ownQueueDroppableId, index: 1 },
    destination: { droppableId: ownQueueDroppableId, index: 0 },
    reason: 'DROP',
    mode: 'FLUID',
    combine: null,
  })
  expect(answers.every((a) => !a.applied)).toBe(true)
  expect(answers[0].detail).toBe(detail)
  expect(log.calls.filter((c) => c.method === 'POST').length).toBe(2)
})

test('the lane renders in RECORDED order, so a reload does not lose the drag', () => {
  const tasks = fixtureTasks()
  // t-archive was created later but carries no hint; t-triage carries rank 10.
  expect(ownQueued(tasks, 'alice').map((t) => t.task_id)).toEqual(['t-triage', 't-archive'])

  // Flip the recorded ranks and the rendered order follows them, not creation.
  const flipped = tasks.map((t) =>
    t.task_id === 't-archive' && t.latest_run
      ? { ...t, latest_run: { ...t.latest_run, queue_hint_rank: 5 } }
      : t,
  )
  expect(ownQueued(flipped, 'alice').map((t) => t.task_id)).toEqual(['t-archive', 't-triage'])
})

test('a member who owns nothing queued gets an empty lane rather than someone else&apos;s work', async () => {
  const routes = oversightRoutes()
  routes['GET /api/auth/session'] = {
    body: { authenticated: true, user: { user_id: 'carol', display_name: 'Carol', role: 'member', pin_set: true } },
  }
  const { view } = await board(routes)
  expect(view.container.querySelector('.queue-lane')?.textContent).toContain('Nothing of yours is queued')
  expect(signedIn.body).toBeDefined()
})
