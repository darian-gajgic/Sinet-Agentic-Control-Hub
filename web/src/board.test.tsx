import { act } from 'react'
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

  // The chat-born task is the second card in this state, and the one whose served
  // run state this suite had no assertion on at all: its own copy of it lived only
  // in the Go byte-compare. A task holding an open interview card is parked and
  // waiting on a person — the pipeline parks the run in the transaction that
  // issues the card — so the face must say so here, where a person reads it.
  const born = (oversightRoutes()['GET /api/tasks'].body as { tasks: TaskListItem[] }).tasks
    .find((t) => t.task_id === 't-chatborn')
  expect(born, 'the served board no longer carries the chat-born task').toBeDefined()
  expect(born!.latest_run?.state, 'a run holding an open interview card cannot read running').toBe('parked')
  expect(born!.latest_run?.waiting_on_human, 'an open interview card IS a person in the way').toBe(true)
  const bornFace = [...view.container.querySelectorAll('.card-face')].find((c) =>
    c.textContent?.includes('Draft the release notes'),
  )!
  expect(bornFace, 'the chat-born card is not on the board').toBeDefined()
  expect(bornFace.querySelector('.waiting-human'), 'the born card hides that it waits on a person').not.toBeNull()
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
    body: { task_id: 't-archive', rank: 20, runs: ['r-archive'], applied: true, detail: 'hint recorded' },
  }
  const { log } = await board(routes)
  const queue = ownQueued(fixtureTasks(), 'alice')
  // Plain ascending, exactly as internal/scheduler/workload.go sorts: rank 0
  // is the NEUTRAL MIDDLE, so t-archive (0) sits ahead of t-triage (10).
  expect(queue.map((t) => t.task_id), 'the lane is not in the comparator&apos;s order').toEqual([
    't-archive',
    't-triage',
  ])

  // Move t-triage to the front. Both cards are then written: t-triage because
  // it moved, t-archive because it was sitting at 0 — and a 0 left behind
  // after a drag would claim the neutral middle while rendering wherever the
  // drop put it.
  await applyDrag(queue, {
    draggableId: 't-triage',
    type: 'DEFAULT',
    source: { droppableId: ownQueueDroppableId, index: 1 },
    destination: { droppableId: ownQueueDroppableId, index: 0 },
    reason: 'DROP',
    mode: 'FLUID',
    combine: null,
  })

  // t-triage already holds rank 10 and lands at position 0, which IS rank 10 —
  // so it is not rewritten. t-archive moves from the neutral 0 to 20. Writing
  // only the difference is what makes a drag idempotent.
  const posts = log.calls.filter((c) => c.method === 'POST')
  expect(posts.map((p) => [p.path, p.body])).toEqual([['/api/tasks/t-archive/priority-hint', { rank: 20 }]])
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
    draggableId: 't-triage',
    type: 'DEFAULT',
    source: { droppableId: ownQueueDroppableId, index: 1 },
    destination: { droppableId: ownQueueDroppableId, index: 0 },
    reason: 'DROP',
    mode: 'FLUID',
    combine: null,
  })
  expect(answers.every((a) => !a.applied)).toBe(true)
  expect(answers[0].detail).toBe(detail)
  expect(log.calls.filter((c) => c.method === 'POST').length).toBe(1)
})

test('the lane renders in the SCHEDULER&apos;s order — 0 is the neutral middle, not last', () => {
  const tasks = fixtureTasks()
  // t-triage carries rank 10; t-archive carries 0. Plain ascending puts the 0
  // FIRST, which is what the claim loop will actually do — the earlier reading
  // ("unranked goes last") would have rendered the opposite of the truth.
  expect(ownQueued(tasks, 'alice').map((t) => t.task_id)).toEqual(['t-archive', 't-triage'])

  // A NEGATIVE rank sorts ahead of the neutral middle, as the comparator says.
  const promoted = tasks.map((t) =>
    t.task_id === 't-triage' && t.latest_run
      ? { ...t, latest_run: { ...t.latest_run, queue_hint_rank: -5 } }
      : t,
  )
  expect(ownQueued(promoted, 'alice').map((t) => t.task_id)).toEqual(['t-triage', 't-archive'])
})

test('a drag leaves NO task at rank 0, so nothing silently claims the neutral middle', () => {
  const queue = ownQueued(fixtureTasks(), 'alice')
  // The lane really does start with a card at the neutral 0, or the assertion
  // below would prove nothing.
  expect(queue.some((t) => (t.latest_run?.queue_hint_rank ?? 0) === 0)).toBe(true)

  const posts = hintPostsFor(queue, {
    draggableId: 't-triage',
    type: 'DEFAULT',
    source: { droppableId: ownQueueDroppableId, index: 1 },
    destination: { droppableId: ownQueueDroppableId, index: 0 },
    reason: 'DROP',
    mode: 'FLUID',
    combine: null,
  })

  // Apply the writes and check the PROPERTY rather than a literal list: after
  // the drag every card in the lane holds a positive rank, so none is left
  // sitting in the neutral middle claiming an order it does not render.
  const after = new Map(queue.map((t) => [t.task_id, t.latest_run?.queue_hint_rank ?? 0]))
  for (const p of posts) after.set(p.task, p.rank)
  for (const [task, rank] of after) {
    expect(rank, `${task} was left at the neutral middle after a drag`).toBeGreaterThan(0)
    expect(Math.abs(rank)).toBeLessThanOrEqual(1000)
  }
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

// ── the component path (drain r1 D10/D11) ─────────────────────────────────

/**
 * drop drives the REAL component through the library's own KEYBOARD drag —
 * focus the handle, space to lift, arrow to move, space to drop.
 *
 * That is the point of this helper: `applyDrag` is unit-tested above, but the
 * component path (the notes, the `data-hint-applied` marker, the re-read) is
 * only exercised if @hello-pangea/dnd itself calls `onDragEnd`. Nothing here
 * synthesizes a DropResult.
 */
function key(el: HTMLElement, k: string, code: number) {
  el.dispatchEvent(new KeyboardEvent('keydown', { key: k, keyCode: code, bubbles: true, cancelable: true }))
}

async function drop(view: { container: HTMLElement }, from: number, direction: 'up' | 'down') {
  const handles = [...view.container.querySelectorAll('[data-rfd-drag-handle-draggable-id]')] as HTMLElement[]
  const handle = handles[from]
  await act(async () => {
    handle.focus()
    key(handle, ' ', 32) // lift
  })
  await act(async () => {
    key(handle, direction === 'up' ? 'ArrowUp' : 'ArrowDown', direction === 'up' ? 38 : 40)
  })
  await act(async () => {
    key(handle, ' ', 32) // drop
  })
  await flush(8)
}

test('a drag through the component renders the verb&apos;s answer and re-reads the board', async () => {
  const routes = oversightRoutes()
  routes['POST /api/tasks/t-archive/priority-hint'] = {
    body: {
      task_id: 't-archive',
      rank: 20,
      runs: ['r-archive'],
      applied: true,
      detail: 'hint recorded: it breaks ties among your own same-class queued work',
    },
  }
  const { view, log } = await board(routes)
  const before = log.calls.filter((c) => c.method === 'GET' && c.path === '/api/tasks').length

  await drop(view, 1, 'up')

  const note = view.container.querySelector('[data-hint-applied]')
  expect(note, 'the verb&apos;s answer was not rendered').not.toBeNull()
  expect(note?.getAttribute('data-hint-applied')).toBe('true')
  expect(note?.textContent).toContain('breaks ties among your own same-class queued work')
  expect(
    log.calls.filter((c) => c.method === 'GET' && c.path === '/api/tasks').length,
    'the board did not re-read after the drag',
  ).toBeGreaterThan(before)
})

test('the stale-board answer renders through the component as the honest answer it is', async () => {
  const routes = oversightRoutes()
  const detail =
    'nothing to reorder: this task has no queued run any more (it was claimed, finished or cancelled), so the board you dragged is stale'
  routes['POST /api/tasks/t-archive/priority-hint'] = {
    body: { task_id: 't-archive', rank: 20, runs: [], applied: false, detail },
  }
  const { view } = await board(routes)
  await drop(view, 1, 'up')

  const note = view.container.querySelector('[data-hint-applied]')
  expect(note?.getAttribute('data-hint-applied')).toBe('false')
  expect(note?.textContent).toBe(detail)
})

test('a drag whose write FAILS says so instead of silently snapping back', async () => {
  const routes = oversightRoutes()
  routes['POST /api/tasks/t-archive/priority-hint'] = { status: 503, body: { error: 'not_wired', detail: 'no scheduler' } }
  const { view } = await board(routes)
  await drop(view, 1, 'up')

  const note = view.container.querySelector('[data-hint-applied]')
  expect(note, 'a failed re-rank left the board silent').not.toBeNull()
  expect(note?.textContent).toContain('The re-rank did not land')
  expect(note?.textContent).toContain('503')
})

// ── D5 applied: the card face's park horizon (P3-UI-4) ────────────────────

test('a card parked on a CLOCK renders its horizon verbatim with the label beside it', async () => {
  // The landed fixture's parked card is also waiting on a person, and that
  // branch wins on the face — so the horizon limb has to be driven. A run parked
  // with nobody in the way is exactly the state this line exists for.
  const tasks = { tasks: fixtureTasks(), cursor: 89, truncated: false }
  tasks.tasks = tasks.tasks.map((t) =>
    t.task_id === 't-audit' && t.latest_run
      ? { ...t, latest_run: { ...t.latest_run, waiting_on_human: false, state: 'parked' } }
      : t,
  )
  const { view } = await board({ ...oversightRoutes(), 'GET /api/tasks': { body: tasks } })

  const face = [...view.container.querySelectorAll('.card-face')].find((c) =>
    c.textContent?.includes('Audit the price table'),
  )!
  expect(face, 'the parked card did not reach the board').toBeDefined()
  const served = '2026-07-20T12:00:00Z'
  const stamp = face.querySelector('.parked-until time')!
  expect(stamp, 'the park horizon rendered no instant').not.toBeNull()
  expect(stamp.getAttribute('dateTime')).toBe(served)
  // A FUTURE horizon is the freeze direction that OVERSTATES: between frames the
  // label reads as more time left than there is, and only this instant corrects
  // it. The server enforces the real horizon regardless.
  expect(stamp.textContent, 'the verbatim UTC was dropped').toBe(served)
  const beside = stamp.parentElement!.parentElement!.textContent ?? ''
  expect(beside.endsWith(served), 'the instant is not beside its label').toBe(true)
  expect(beside.length, 'a relative label replaced the instant').toBeGreaterThan(served.length)
  expect(face.querySelector('.parked-until')!.textContent).toContain('parked until')

  // THE CARD FACE IS STILL THE PINNED SET: ParkedUntil's internal render moving
  // to the primitive adds nothing to the face.
  expect(face.querySelectorAll('dt')).toHaveLength(5)
  view.unmount()
})

// ── the D8 self-teaching layer (P3-UI-5) ──────────────────────────────────

test('the board teaches what it is, and the line never promises the drag more than it does', async () => {
  const { view } = await board()
  const line = view.container.querySelector('[data-surface-what]')?.textContent ?? ''
  expect(line, 'the board carries no "what this is" line').not.toBe('')
  expect(line.toLowerCase()).toContain('live from the feed')
  // THE TRUTH CONSTRAINT. Stage is FSM state owned by the control plane (S02),
  // and a hint only breaks ties among your own same-class queued work — so the
  // line must claim neither a stage move nor a priority change.
  expect(line.toLowerCase(), 'the header line does not scope the drag to your own queued work').toContain(
    'your own queued work',
  )
  expect(line.toLowerCase(), 'the header line claims the drag moves a stage').toContain('never moves a card to another')
  for (const overclaim of ['change the priority', 'set the priority', 'move it to', 'reassign']) {
    expect(line.toLowerCase(), `the header line overclaims: ${overclaim}`).not.toContain(overclaim)
  }
  view.unmount()
})

test('the empty queue lane and an empty column each teach what would fill them', async () => {
  const routes = oversightRoutes()
  routes['GET /api/tasks'] = { body: { tasks: [], cursor: 1, truncated: false } }
  const { view } = await board(routes)

  const lane = view.container.querySelector('.queue-lane')!
  expect(lane.textContent).toContain('Nothing of yours is queued.')
  expect(lane.textContent, 'the lane does not teach that it is the one draggable place').toContain(
    'the only place a drag does anything',
  )
  const column = view.container.querySelector('.column')!
  expect(column.textContent).toContain('No card is here.')
  // The teaching half says how a card ARRIVES — which is the fact the drag
  // affordance must never contradict.
  expect(column.textContent, 'an empty column does not teach how a card reaches it').toContain('never by being dragged')
  view.unmount()
})

test('the drag affordance is on the own-queued lane and nowhere else', async () => {
  // §42: a card that cannot be reordered must not LOOK draggable. The lane's
  // cards carry the grab cursor and the lift; a stage column's do not.
  const { view } = await board()
  const lane = [...view.container.querySelectorAll('.queue-lane .card')]
  expect(lane.length, 'the own-queued lane is empty, so this asserts nothing').toBeGreaterThan(0)
  for (const card of lane) expect(card.className, 'a reorderable card has no drag affordance').toContain('cursor-grab')

  const columnCards = [...view.container.querySelectorAll('.column .card')]
  expect(columnCards.length).toBeGreaterThan(0)
  for (const card of columnCards) {
    expect(card.className, 'a card that cannot be reordered looks draggable').not.toContain('cursor-grab')
  }
})
