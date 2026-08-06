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
 *
 * REWRITTEN 2026-08-06 (rework step 2, map §3 v3): the board became a real
 * Kanban — five bounded columns labelled Backlog · Executing · Verifying ·
 * Needs attention · Done, cancelled tasks IN Backlog under a cancelled sign,
 * Nexus card anatomy with the D-A expansion, and the queue strip as the one
 * drag surface. The DOM selectors here follow that anatomy; every behavior
 * contract the old suite pinned (drag negatives, honesty rules, S1.3 face,
 * none-vs-not-loaded) is re-asserted against the new markup.
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

  const cards = [...view.container.querySelectorAll('.task-card')]
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

  // And the honest absences: a run with no cost reading shows no figure.
  const archive = cards.find((c) => c.textContent?.includes('Archive last quarter'))!
  expect(archive.textContent).toContain('no cost reading')
  expect(archive.textContent, 'an absent cost was rendered as zero').not.toContain('USD 0')
})

test('waiting-on-a-human and parked-until are distinguished on the face', async () => {
  const { view } = await board()
  const audit = [...view.container.querySelectorAll('.task-card')].find((c) =>
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
  const bornFace = [...view.container.querySelectorAll('.task-card')].find((c) =>
    c.textContent?.includes('Draft the release notes'),
  )!
  expect(bornFace, 'the chat-born card is not on the board').toBeDefined()
  expect(bornFace.querySelector('.waiting-human'), 'the born card hides that it waits on a person').not.toBeNull()
})

test('no view renders a percentage, a completion fraction or an ETA', async () => {
  const { view } = await board()
  const text = (view.container.textContent ?? '').toLowerCase()
  for (const banned of ['%', 'percent', 'complete:', 'estimated time', 'remaining time']) {
    expect(text, `the board rendered "${banned}" — the API serves no such figure (§30)`).not.toContain(banned)
  }
  // 'eta' is scanned as a WORD: adjacent spans concatenate in textContent, and
  // the shell's brand row reads "sinetagentic…" — letters, not a figure.
  expect(text, 'the board rendered "eta" — the API serves no such figure (§30)').not.toMatch(/\beta\b/)
  // Non-tautology: the scan is reading a board with real cards on it.
  expect(text).toContain('ship the release notes')
})

// ── columns and grouping (R5, R7; OQ7) ────────────────────────────────────

test('the five declared columns render with their labels, and an unknown stored value gets its own', async () => {
  const { view } = await board()
  const columns = [...view.container.querySelectorAll('.kanban-col')].map((c) => c.getAttribute('data-status'))
  for (const known of ['intake', 'executing', 'verifying', 'attention', 'done']) {
    expect(columns, `${known} is not a column`).toContain(known)
  }
  // ▲ v3 (operator D-B): no Cancelled column — a cancelled task lives in
  // Backlog under a cancelled sign — and the first column reads "Backlog"
  // over the stored `intake`.
  expect(columns, 'cancelled is a column again — D-B says it must not be').not.toContain('cancelled')
  expect(view.container.querySelector('.kanban-col[data-status="intake"] .col-title')?.textContent).toBe('Backlog')
  // The fixture carries a producer string the board has never seen. It must
  // not vanish the card.
  expect(columns, 'an unknown stored status was dropped').toContain('moonshot')
  const other = view.container.querySelector('.kanban-col[data-status="moonshot"]')!
  expect(other.getAttribute('data-known')).toBe('false')
  expect(other.textContent).toContain('Archive last quarter')
})

test('columnsFor keeps every card and invents no display name for an unknown value', () => {
  const cols = columnsFor(fixtureTasks())
  const known = cols.filter((c) => c.known).map((c) => c.status)
  expect(known).toEqual(['intake', 'executing', 'verifying', 'attention', 'done'])
  const unknown = cols.filter((c) => !c.known)
  expect(unknown.map((c) => c.status)).toEqual(['moonshot'])
  expect(unknown[0].label, 'a friendly name was invented for a value the platform has not decided on').toBe('moonshot')
})

test('a cancelled task renders IN Backlog wearing the cancelled sign (D-B)', async () => {
  // The fixture world has no cancelled task, so one is driven into that state
  // — the same technique the parked-horizon test uses for its limb.
  const tasks = { tasks: fixtureTasks(), cursor: 89, truncated: false }
  tasks.tasks = tasks.tasks.map((t) => (t.task_id === 't-triage' ? { ...t, kanban_status: 'cancelled' } : t))
  const { view } = await board({ ...oversightRoutes(), 'GET /api/tasks': { body: tasks } })

  expect(
    [...view.container.querySelectorAll('.kanban-col')].map((c) => c.getAttribute('data-status')),
    'a cancelled column appeared',
  ).not.toContain('cancelled')
  const backlog = view.container.querySelector('.kanban-col[data-status="intake"]')!
  const card = [...backlog.querySelectorAll('.task-card')].find((c) => c.textContent?.includes('Triage the inbox backlog'))!
  expect(card, 'the cancelled card left the board').toBeDefined()
  expect(card.getAttribute('data-cancelled')).toBe('true')
  expect(card.textContent).toContain('cancelled')
  view.unmount()
})

test('cards group by project inside a column, and the honest (no project) bucket renders as its own label', async () => {
  // Grouping labels appear when a column genuinely holds more than one
  // project bucket — drive one fixture task into a second project so the
  // Backlog column carries both.
  const tasks = { tasks: fixtureTasks(), cursor: 89, truncated: false }
  tasks.tasks = tasks.tasks.map((t) => (t.task_id === 't-triage' ? { ...t, project: 'release-notes' } : t))
  const { view } = await board({ ...oversightRoutes(), 'GET /api/tasks': { body: tasks } })

  const backlog = view.container.querySelector('.kanban-col[data-status="intake"]')!
  const labels = [...backlog.querySelectorAll('.col-proj')].map((g) => g.textContent)
  expect(labels).toContain('release-notes')
  expect(labels, 'the honest bucket was dropped rather than rendered').toContain('(no project)')
  view.unmount()
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

test('a member who owns nothing queued gets NO queue strip rather than someone else&apos;s work', async () => {
  const routes = oversightRoutes()
  routes['GET /api/auth/session'] = {
    body: { authenticated: true, user: { user_id: 'carol', display_name: 'Carol', role: 'member', pin_set: true } },
  }
  const { view } = await board(routes)
  // The strip renders only over the caller's OWN queued work: for carol there
  // is none, so there is no drag surface at all — and therefore nothing that
  // could ever show her someone else's cards as reorderable.
  expect(view.container.querySelector('[data-queue-strip]')).toBeNull()
  expect(view.container.querySelector('[data-rfd-drag-handle-draggable-id]')).toBeNull()
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

  const face = [...view.container.querySelectorAll('.task-card')].find((c) =>
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
  expect(face.querySelector('.parked-until')!.textContent).toContain('parked until')

  // THE CARD FACE IS STILL THE PINNED S1.3 SET on the new anatomy: whose and
  // effort beside the park line (this fixture run carries no stage marker,
  // and none is invented for it).
  const text = face.textContent ?? ''
  expect(text).toContain('bob')
  expect(text).toContain('standard')
  view.unmount()
})

// ── the D8 self-teaching layer (P3-UI-5) ──────────────────────────────────

test('the queue strip teaches the drag honestly, and never promises more than it does', async () => {
  const { view } = await board()
  const line = view.container.querySelector('.queue-sub')?.textContent ?? ''
  expect(line, 'the queue strip carries no teaching line').not.toBe('')
  // THE TRUTH CONSTRAINT. Stage is FSM state owned by the control plane (S02),
  // and a hint only breaks ties among your own same-class queued work — so the
  // line must claim neither a stage move nor a priority change.
  expect(line.toLowerCase(), 'the line does not scope the drag to your own queued work').toContain('your own queued work')
  expect(line.toLowerCase(), 'the line claims the drag moves a stage').toContain('never moves a card to another stage')
  expect(line.toLowerCase(), 'the line does not scope the drag to your own queue').toContain(
    "never reaches another person's queue",
  )
  for (const overclaim of ['change the priority', 'set the priority', 'move it to', 'reassign']) {
    expect(line.toLowerCase(), `the line overclaims: ${overclaim}`).not.toContain(overclaim)
  }
  view.unmount()
})

test('an empty column teaches how a card arrives, and Backlog teaches the door', async () => {
  const routes = oversightRoutes()
  routes['GET /api/tasks'] = { body: { tasks: [], cursor: 1, truncated: false } }
  const { view } = await board(routes)

  // No queued work → no drag surface at all (the strip renders only over the
  // caller's own queued cards; an empty dashed box teaching "drag here" would
  // be an affordance for a gesture with nothing to act on).
  expect(view.container.querySelector('[data-queue-strip]')).toBeNull()

  const exec = view.container.querySelector('.kanban-col[data-status="executing"] .col-empty')!
  expect(exec, 'an empty column renders no teaching line').not.toBeNull()
  // The teaching half says how a card ARRIVES — which is the fact the drag
  // affordance must never contradict.
  expect(exec.textContent, 'an empty column does not teach how a card reaches it').toContain('never by drag')
  const backlog = view.container.querySelector('.kanban-col[data-status="intake"] .col-empty')!
  expect(backlog.textContent, 'the empty Backlog does not point at the give-work door').toContain('Describe a goal')
  view.unmount()
})

test('the drag affordance is on the own-queued lane and nowhere else', async () => {
  // §42: a card that cannot be reordered must not LOOK draggable. The lane's
  // cards carry the library's lift handles; a stage column's cards carry none.
  const { view } = await board()
  const handles = [...view.container.querySelectorAll('[data-rfd-drag-handle-draggable-id]')]
  expect(handles.length, 'the own-queued lane is empty, so this asserts nothing').toBeGreaterThan(0)
  for (const h of handles) {
    expect(h.closest('.queue-lane'), 'a drag handle exists outside the queue lane').not.toBeNull()
  }
  const columnCards = [...view.container.querySelectorAll('.kanban-col .task-card')]
  expect(columnCards.length).toBeGreaterThan(0)
  for (const card of columnCards) {
    expect(card.getAttribute('data-rfd-drag-handle-draggable-id'), 'a stage-column card looks draggable').toBeNull()
  }
})

test('NONE-VS-NOT-LOADED: a pending board read shows the catching-up marker, never a teaching empty', async () => {
  // The session LANDS and the board's own read never answers, so this observes
  // the surface's own pending window (drain r1, D1). `Freshness` owns it; a
  // teaching empty over it would say "you have nothing queued" to somebody
  // whose queue simply has not arrived yet.
  const routes = oversightRoutes()
  routes['GET /api/tasks'] = { pending: true }
  const { view } = await board(routes)
  expect(view.container.querySelector('.kanban-toolbar'), 'no surface mounted, so this proves nothing').not.toBeNull()
  expect(view.container.textContent, 'the loading affordance is missing').toContain('catching up')
  expect(view.container.textContent, 'a column empty taught over a pending read').not.toContain('Nothing is at this stage')
  expect(view.container.textContent, 'the Backlog empty taught over a pending read').not.toContain('Nothing waits here')
  view.unmount()

  // …and a served empty board teaches, so the gate is not simply always off.
  const served = oversightRoutes()
  served['GET /api/tasks'] = { body: { tasks: [], cursor: 1, truncated: false } }
  const { view: landed } = await board(served)
  expect(landed.container.textContent).toContain('Nothing waits here')
  landed.unmount()
})
