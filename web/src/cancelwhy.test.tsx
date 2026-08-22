import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import type { ApprovalList, TaskDetail as Detail } from './api'
import { FakeSource, fixtures, oversightRoutes, scriptedFetch, type Scripted } from './doubles'
import { EventStream } from './events'
import { click, flush, mount, typeInto } from './testing'

/**
 * P3-RW-19-FE — the cancel's why becomes human, on screen (S15.5/S15.6).
 *
 * BEHAVIOR CONTRACTS, driven through the real surfaces against the golden
 * fixtures. What is pinned here:
 *
 *  - the two HTTP cancel verbs carry the person's words VERBATIM as
 *    `{reason}`, and an empty why stays the landed `{}` body — no reason is
 *    always accepted, the reason never blocks a cancel;
 *  - the 280 bound counts RUNES, the act is HELD (not fired) while over, the
 *    bound is SAID, and nothing is ever truncated;
 *  - the cancel-shaped card answers carry the why as `note` — and ONLY the
 *    cancel-shaped answer does: a why typed and then Approved sends nothing
 *    of it (the field's label promises exactly that);
 *  - the telling side renders the person's words where `human_reason` is
 *    served, and the honest "no reason was given" from its STRUCTURAL
 *    absence — never the mechanical rule citation the walk found standing
 *    where a motive should be.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

async function openAt(path: string, extra: Record<string, Scripted> = {}) {
  const routes = { ...oversightRoutes(), ...extra }
  const log = scriptedFetch(routes)
  window.history.replaceState(null, '', path)
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}

/** The kit confirm's popup: the LAST dialog — the task window itself is one. */
function confirmDialog(): HTMLElement {
  const all = document.querySelectorAll('[role="dialog"]')
  return all[all.length - 1] as HTMLElement
}

const detailRoutes = () => ({
  'GET /api/tasks/t-ship': { body: fixtures.taskDetail() },
})

/** One applied cancel outcome in the served shape (stage.CancelOutcome). */
const cancelledBody = {
  run_id: 'r-ship',
  from: 'running',
  to: 'completed',
  applied: true,
  ladder_invoked: true,
  detail: 'running work cancelled: the run completes carrying the cancel reason (§14 reading 9)',
}

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── the HTTP verbs carry the why ──────────────────────────────────────────

test('a run cancel carries the typed why VERBATIM as {reason}', async () => {
  const { view, log } = await openAt('/tasks/t-ship', detailRoutes())
  log.set('POST /api/runs/r-ship/cancel', { body: cancelledBody })

  click(view.container.querySelector('[data-cancel-run="r-ship"]'))
  await flush()
  const dialog = confirmDialog()
  const field = dialog.querySelector('[data-field="cancel-why"]') as HTMLInputElement
  expect(field, 'the confirm ceremony lost its why field').not.toBeNull()
  typeInto(field, 'taking a different approach ')
  click(dialog.querySelector('[data-act="confirm"]'))
  await flush()

  const post = log.calls.find((c) => c.method === 'POST' && c.path === '/api/runs/r-ship/cancel')
  // Verbatim: the trailing space is the person's own, never trimmed away.
  expect(post?.body).toEqual({ reason: 'taking a different approach ' })
})

test('a cancel with no why posts the landed {} body — no reason is always accepted', async () => {
  const { view, log } = await openAt('/tasks/t-ship', detailRoutes())
  log.set('POST /api/runs/r-ship/cancel', { body: cancelledBody })

  click(view.container.querySelector('[data-cancel-run="r-ship"]'))
  await flush()
  click(confirmDialog().querySelector('[data-act="confirm"]'))
  await flush()

  const post = log.calls.find((c) => c.method === 'POST' && c.path === '/api/runs/r-ship/cancel')
  expect(post?.body).toEqual({})
})

test('a task cancel carries the same why to its own verb', async () => {
  const { view, log } = await openAt('/tasks/t-ship', detailRoutes())
  log.set('POST /api/tasks/t-ship/cancel', {
    body: { task_id: 't-ship', kanban_status: 'cancelled', runs: [cancelledBody], applied: true },
  })

  click(view.container.querySelector('[data-cancel="task"]'))
  await flush()
  const dialog = confirmDialog()
  typeInto(dialog.querySelector('[data-field="cancel-why"]') as HTMLInputElement, 'the plan changed')
  click(dialog.querySelector('[data-act="confirm"]'))
  await flush()

  const post = log.calls.find((c) => c.method === 'POST' && c.path === '/api/tasks/t-ship/cancel')
  expect(post?.body).toEqual({ reason: 'the plan changed' })
})

// ── the bound: said, held, never applied silently ─────────────────────────

test('over 280 RUNES the act is held, the bound is said, and nothing is truncated or sent', async () => {
  const { view, log } = await openAt('/tasks/t-ship', detailRoutes())
  const posts = () => log.calls.filter((c) => c.method === 'POST').length
  const before = posts()

  click(view.container.querySelector('[data-cancel-run="r-ship"]'))
  await flush()
  const dialog = confirmDialog()
  const field = dialog.querySelector('[data-field="cancel-why"]') as HTMLInputElement

  // 280 astral-plane runes: 560 UTF-16 units. The server counts RUNES
  // (internal/api/actions.go cancelReasonMaxRunes), so this must NOT hold —
  // a .length count would refuse a sentence the platform accepts.
  const exactly280 = '🙂'.repeat(280)
  typeInto(field, exactly280)
  expect(dialog.querySelector('[data-why-over]'), 'the bound fired below itself').toBeNull()
  expect((dialog.querySelector('[data-act="confirm"]') as HTMLButtonElement).disabled).toBe(false)

  // One rune over: held, said in plain words, the typed text untouched.
  typeInto(field, exactly280 + '🙂')
  const over = dialog.querySelector('[data-why-over]')
  expect(over, 'the over-bound state renders no explanation').not.toBeNull()
  expect(over?.textContent).toContain('281 characters')
  expect(over?.textContent).toContain('280')
  expect((dialog.querySelector('[data-act="confirm"]') as HTMLButtonElement).disabled, 'the held act was fireable').toBe(true)
  expect(field.value, 'the why was silently altered').toBe(exactly280 + '🙂')

  click(dialog.querySelector('[data-act="confirm"]'))
  await flush()
  expect(posts(), 'a held act fired anyway').toBe(before)
})

// ── the cancel-shaped card answers carry the why as `note` ────────────────

const mine = () => fixtures.approvalsMine() as unknown as ApprovalList

const answered = { body: { applied: true, state: 'answered', detail: '' } }

/** askRow presses the card's compact face open first — the gate-ordered
 *  anatomy (2026-08-22) keeps the answer fields in the detail behind it. */
const askRow = async (view: { container: HTMLElement }, id: string) => {
  const node = view.container.querySelector(`[data-card-id="${CSS.escape(id)}"]`) as HTMLElement
  if (node.getAttribute('data-open') !== 'true') {
    click(node.querySelector('button[data-face]'))
    await flush()
  }
  return view.container.querySelector(`[data-card-id="${CSS.escape(id)}"]`) as HTMLElement
}

test('the verify card cancel carries the note: {choice:"cancel", note}', async () => {
  const { view, log } = await openAt('/inbox', { 'GET /api/approvals': { body: mine() } })
  log.set(`POST /api/approvals/${encodeURIComponent('ask:ask-brief')}/answer`, answered)

  const row = await askRow(view, 'ask:ask-brief')
  typeInto(row.querySelector('[data-field="cancel-note"]') as HTMLInputElement, "two rounds is enough — I'll write it myself")
  click(row.querySelector('[data-action="cancel"]'))
  await flush()

  const post = log.calls.find((c) => c.method === 'POST' && c.path.includes('ask%3Aask-brief'))
  expect((post?.body as { answer: unknown }).answer).toEqual({
    choice: 'cancel',
    note: "two rounds is enough — I'll write it myself",
  })
})

test('the note rides ONLY the cancel-shaped verb: typed and then Accepted, nothing of it is sent', async () => {
  const { view, log } = await openAt('/inbox', { 'GET /api/approvals': { body: mine() } })
  log.set(`POST /api/approvals/${encodeURIComponent('ask:ask-brief')}/answer`, answered)

  const row = await askRow(view, 'ask:ask-brief')
  typeInto(row.querySelector('[data-field="cancel-note"]') as HTMLInputElement, 'a why that belongs to the cancel alone')
  click(row.querySelector('[data-action="accept_best_effort"]'))
  await flush()

  const post = log.calls.find((c) => c.method === 'POST' && c.path.includes('ask%3Aask-brief'))
  expect((post?.body as { answer: unknown }).answer).toEqual({ choice: 'accept_best_effort' })
})

test('the intake approval cancel carries the note: {action:"cancel", note}', async () => {
  // The golden approval card serves no cancel verb, so one is added here —
  // the SHAPE under test is the contest envelope's cancel-shaped answer.
  const body = mine()
  const ship = body.items.find((i) => i.id === 'ask:ask-ship')
  if (!ship) throw new Error('the fixture lost ask:ask-ship — this asserts nothing')
  ship.actions = [...(ship.actions ?? []), 'cancel']

  const { view, log } = await openAt('/inbox', { 'GET /api/approvals': { body } })
  log.set(`POST /api/approvals/${encodeURIComponent('ask:ask-ship')}/answer`, answered)

  const row = await askRow(view, 'ask:ask-ship')
  typeInto(row.querySelector('[data-field="cancel-note"]') as HTMLInputElement, 'wrong week for this')
  click(row.querySelector('[data-action="cancel"]'))
  await flush()

  const post = log.calls.find((c) => c.method === 'POST' && c.path.includes('ask%3Aask-ship'))
  expect((post?.body as { answer: unknown }).answer).toEqual({ action: 'cancel', note: 'wrong week for this' })
})

test('the SPEC-DOUBT rethink IS a cancel and carries the note: {choice:"rethink", note}', async () => {
  const body = mine()
  const coverage = body.items.find((i) => i.id === 'ask:ask-coverage')
  if (!coverage) throw new Error('the fixture lost ask:ask-coverage — this asserts nothing')
  coverage.actions = [...(coverage.actions ?? []), 'rethink']
  const snap = coverage.card as { decision?: { choices?: { label: string; value: string }[] } }
  snap.decision?.choices?.push({ label: 'Rethink the ask', value: 'rethink' })

  const { view, log } = await openAt('/inbox', { 'GET /api/approvals': { body } })
  log.set(`POST /api/approvals/${encodeURIComponent('ask:ask-coverage')}/answer`, answered)

  const row = await askRow(view, 'ask:ask-coverage')
  typeInto(row.querySelector('[data-field="cancel-note"]') as HTMLInputElement, 'the whole ask was wrong')
  click(row.querySelector('[data-action="rethink"]'))
  await flush()

  const post = log.calls.find((c) => c.method === 'POST' && c.path.includes('ask%3Aask-coverage'))
  expect((post?.body as { answer: unknown }).answer).toEqual({ choice: 'rethink', note: 'the whole ask was wrong' })
})

test('a whitespace-only note is no note at all', async () => {
  const { view, log } = await openAt('/inbox', { 'GET /api/approvals': { body: mine() } })
  log.set(`POST /api/approvals/${encodeURIComponent('ask:ask-brief')}/answer`, answered)

  const row = await askRow(view, 'ask:ask-brief')
  typeInto(row.querySelector('[data-field="cancel-note"]') as HTMLInputElement, '   ')
  click(row.querySelector('[data-action="cancel"]'))
  await flush()

  const post = log.calls.find((c) => c.method === 'POST' && c.path.includes('ask%3Aask-brief'))
  expect((post?.body as { answer: unknown }).answer).toEqual({ choice: 'cancel' })
})

test('a card with no cancel-shaped verb renders no why field', async () => {
  const { view } = await openAt('/inbox', { 'GET /api/approvals': { body: mine() } })
  // ask:ask-delta answers {approve, reject} — nothing cancel-shaped.
  expect((await askRow(view, 'ask:ask-delta')).querySelector('[data-field="cancel-note"]')).toBeNull()
})

// ── the telling side: the person's words, or the honest absence ───────────

/** A reconstructed cancel row as reads.go serves it (decision:"cancel" is the
 *  machine code; `reason` is the record's mechanical sentence; `human_reason`
 *  is the person's words, ABSENT when none were given). */
const cancelRow = (human?: string) => ({
  seq: 900,
  type: 'run.state_changed',
  ts: '2026-08-18T05:00:00Z',
  run_id: 'r-ship',
  actor: 'alice',
  card_id: '',
  card_type: '',
  decision: 'cancel',
  reason: 'human cancel (4.5)',
  ...(human === undefined ? {} : { human_reason: human }),
})

function cancelledDetail(human?: string): Detail {
  const base = fixtures.taskDetail() as unknown as Detail
  return { ...base, kanban_status: 'cancelled', decisions: [cancelRow(human)] as unknown as Detail['decisions'] }
}

test("the banner and the decisions row tell the why in the person's own words — not the rule citation", async () => {
  const { view } = await openAt('/tasks/t-ship', {
    'GET /api/tasks/t-ship': { body: cancelledDetail('taking a different approach') },
  })

  const banner = view.container.querySelector('[data-stall="cancelled"]')!
  expect(banner, 'a cancelled task lost its banner').not.toBeNull()
  expect(banner.textContent).toContain('cancelled by')
  expect(banner.textContent).toContain('taking a different approach')

  const row = view.container.querySelector('[data-decision="cancel"]')!
  expect(row, 'the cancel decision row is gone').not.toBeNull()
  expect(row.textContent).toContain('cancelled this work')
  expect(row.querySelector('[data-cancel-why="given"]')?.textContent).toContain('taking a different approach')
  // The mechanical sentence stays served, but a rule citation must not stand
  // where the motive belongs (the walk finding this round exists to end).
  expect(row.textContent).not.toContain('human cancel (4.5)')
})

test('no reason given renders as exactly that — from the STRUCTURAL absence, in History\'s own phrase', async () => {
  const { view } = await openAt('/tasks/t-ship', {
    'GET /api/tasks/t-ship': { body: cancelledDetail() },
  })

  expect(view.container.querySelector('[data-stall="cancelled"]')?.textContent).toContain('no reason was given')
  const row = view.container.querySelector('[data-decision="cancel"]')!
  expect(row.querySelector('[data-cancel-why="absent"]')?.textContent).toContain('no reason was given')
  expect(row.querySelector('[data-cancel-why="given"]')).toBeNull()
})
