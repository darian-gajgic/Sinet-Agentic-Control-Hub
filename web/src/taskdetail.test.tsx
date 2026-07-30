import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { ReceiptView } from './TaskDetail'
import type { Receipt, RunDetail, TaskDetail as Detail } from './api'
import { FakeSource, fixtures, oversightRoutes, scriptedFetch } from './doubles'
import { EventStream } from './events'
import { flush, mount } from './testing'

/**
 * Task detail (Spec S15.5 ¶3; 9.2; S2.2; S2.4; §38 ruling (a); G2 D2.8),
 * driven against the golden fixtures — including BOTH artifact states, because
 * "a draft is labelled a draft" cannot be tested with only approved data.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

async function task(id: string, extra: Record<string, { body?: unknown; status?: number }> = {}) {
  const routes = { ...oversightRoutes(), ...extra }
  const log = scriptedFetch(routes)
  window.history.replaceState(null, '', `/tasks/${id}`)
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}

const detailRoutes = () => ({
  'GET /api/tasks/t-ship': { body: fixtures.taskDetail() },
  'GET /api/tasks/t-ops': { body: fixtures.taskDetailOps() },
  'GET /api/tasks/t-triage': { body: fixtures.taskDetailDraft() },
  'GET /api/tasks/t-archive': { body: fixtures.taskDetailBare() },
})

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── spec + ACs + plan, with the §38 ruling-(a) honesty (R10) ──────────────

test('an approved pair reads as confirmed and its numbered ACs render in order', async () => {
  const { view } = await task('t-ship', detailRoutes())
  const statuses = [...view.container.querySelectorAll('[data-artifact-status]')].map((n) =>
    n.getAttribute('data-artifact-status'),
  )
  expect(statuses).toEqual(['approved', 'approved'])
  expect(view.container.textContent).toContain('confirmed')

  const acs = [...view.container.querySelectorAll('.acs li')].map((li) => li.getAttribute('data-ac'))
  expect(acs).toEqual(['AC-1', 'AC-2'])
  expect(view.container.textContent).toContain('Every merged change since the last release is listed once.')
  // The plan's coverage map is rendered, so a reader can see which step owns
  // which criterion.
  expect(view.container.querySelector('.steps')?.textContent).toContain('AC-1')
})

test('a DRAFT pair is labelled a draft and never presented as the confirmed specification', async () => {
  const { view } = await task('t-triage', detailRoutes())
  const statuses = [...view.container.querySelectorAll('[data-artifact-status]')].map((n) =>
    n.getAttribute('data-artifact-status'),
  )
  expect(statuses).toEqual(['draft', 'draft'])
  const text = view.container.textContent ?? ''
  expect(text).toContain('not approved — nobody has signed this off yet')
  expect(text, 'a draft was presented as confirmed').not.toContain('confirmed')
})

test('a task with no drafted pair renders the served reason, not an error', async () => {
  const { view } = await task('t-archive', detailRoutes())
  const bare = fixtures.taskDetailBare() as unknown as Detail
  expect(bare.spec, 'the fixture is not actually the artifacts-absent case').toBeNull()
  expect(bare.artifacts_absent).not.toBe('')
  expect(view.container.textContent).toContain(bare.artifacts_absent ?? '')
  // The task itself still reads.
  expect(view.container.textContent).toContain('Archive last quarter')
})

// ── stage story + lineage (R11, R7) ───────────────────────────────────────

test('the stage story renders from served rows, with no derived percentage', async () => {
  const { view } = await task('t-ship', detailRoutes())
  const stages = [...view.container.querySelectorAll('.stages li')]
  expect(stages.length).toBeGreaterThan(0)
  expect(stages[0].getAttribute('data-stage')).toBe('execute')
  const text = (view.container.textContent ?? '').toLowerCase()
  for (const banned of ['%', 'percent', 'eta']) {
    expect(text, `the task detail rendered "${banned}"`).not.toContain(banned)
  }
})

test('lineage renders both directions, and a multi-project claim renders as the ambiguity it is', async () => {
  const { view } = await task('t-ship', detailRoutes())
  expect(view.container.querySelector('.lineage')?.textContent).toContain('release-notes')

  // The ambiguity direction, from a served body with more than one claim.
  const ambiguous = fixtures.taskDetail() as unknown as Detail
  ambiguous.lineage.project_choices = 2
  ambiguous.lineage.succeeded_by = [
    { task_id: 't-followup', deliverable_id: 'd-notes', revision_n: 2, created_ts: '2026-07-20T09:09:00Z' },
  ]
  const { view: v2 } = await task('t-ship', {
    ...detailRoutes(),
    'GET /api/tasks/t-ship': { body: ambiguous },
  })
  const node = v2.container.querySelector('[data-ambiguous="project"]')
  expect(node, 'a multi-project claim was collapsed instead of shown').not.toBeNull()
  expect(node?.textContent).toContain('2')
  expect(v2.container.querySelector('[data-lineage="succeeded-by"]')?.textContent).toContain('t-followup')
})

// ── human decisions (R12; OQ5) ────────────────────────────────────────────

test('every human decision on this task renders with its actor and when — and no other task&apos;s', async () => {
  const { view } = await task('t-ship', detailRoutes())
  const rows = [...view.container.querySelectorAll('.decisions li')]
  // The three shapes the REAL producers mint: an effect approval
  // (`decision.recorded`), a deliverable accept (platform-scoped, reached
  // through the deliverables join) and an intake delta answer (run-scoped,
  // naming no actor of its own).
  expect(rows.map((r) => r.getAttribute('data-card-type'))).toEqual(['effect', 'deliverable', 'intake_delta'])

  const text = view.container.textContent ?? ''
  expect(text).toContain('approve')
  expect(text).toContain('accept')
  expect(text).toContain('effect:e-publish')
  expect(text).toContain('deliverable:d-notes')
  // The subject filter really filters: a decision about another task is not
  // this task's, and the served body proves the server excluded it.
  expect(text, "another task's decision reached this page").not.toContain('t-elsewhere')
})

test('a task with no decisions says so rather than showing an empty list', async () => {
  const { view } = await task('t-archive', detailRoutes())
  expect(view.container.textContent).toContain('Nobody has had to decide anything on this task yet')
})

// ── the receipt and its registered labels (R14; OQ3) ──────────────────────

test('the receipt itemizes ceremony separately from execution and never hides an unpriced call', async () => {
  const { view } = await task('t-ship', detailRoutes())
  const purposes = [...view.container.querySelectorAll('[data-purpose]')].map((n) => n.getAttribute('data-purpose'))
  expect(purposes).toEqual(['ceremony', 'execution'])
  const text = view.container.textContent ?? ''
  expect(text).toContain('USD 1.42')
  expect(text, 'an unpriced call was folded silently into the priced total').toContain('UNPRICED')
})

test('the park history and the S10.6 mode note render verbatim', async () => {
  const { view } = await task('t-ship', detailRoutes())
  expect(view.container.querySelector('.parks')?.textContent).toContain('weekly quota reached')
  const note = view.container.querySelector('[data-mode-note="verbatim"]')?.textContent ?? ''
  const served = (fixtures.receipt() as unknown as Receipt).mode.note
  expect(served.length).toBeGreaterThan(0)
  expect(note, 'the seam note was paraphrased').toBe(served)
})

test('the done-directly label is the string the API served, byte for byte', async () => {
  const { view } = await task('t-ship', detailRoutes())
  const served = (fixtures.receipt() as unknown as Receipt).direct_use.label
  // The registered §13.1 per-run form, from the fixture the Go suite pins.
  expect(served).toBe('direct-use estimate (heuristic)')
  const rendered = view.container.querySelector('.direct-use-label')?.textContent
  expect(rendered).toBe(served)
})

test('the AGGREGATE measured label renders verbatim too, because nothing about it is a client constant', () => {
  // The registered §13.2 form. No landed read serves the aggregate yet (the
  // done-directly Layer-0 view cites each receipt's own per-run block), so this
  // drives the render path directly: whatever label the API puts on the wire is
  // what reaches the screen, including this one, without any UI translation.
  const receipt = fixtures.receipt() as unknown as Receipt
  receipt.direct_use.label = 'measured (benchmark n=12)'
  receipt.direct_use.heuristic_usd = 0.94
  const view = mount(<ReceiptView receipt={receipt} />)
  expect(view.container.querySelector('.direct-use-label')?.textContent).toBe('measured (benchmark n=12)')
  view.unmount()
})

test('an unpriced done-directly figure renders its reason instead of a dollar amount', () => {
  const receipt = fixtures.receipt() as unknown as Receipt
  receipt.direct_use.unpriced = true
  receipt.direct_use.reason = 'this lane is flat-rate, so no per-call dollar figure exists'
  receipt.direct_use.heuristic_usd = undefined
  const view = mount(<ReceiptView receipt={receipt} />)
  const text = view.container.querySelector('.direct-use')?.textContent ?? ''
  expect(text).toContain('this lane is flat-rate')
  expect(text, 'an unpriced figure was rendered as a dollar amount').not.toContain('USD')
  view.unmount()
})

test('a run with no receipt renders the served reason', async () => {
  const detail = fixtures.taskDetail() as unknown as Detail
  detail.runs = [{ run_id: 'r-new', state: 'queued', created_ts: '2026-07-20T09:10:00Z', receipt_absent: 'no receipt yet' }]
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
  expect(view.container.textContent).toContain('no receipt yet')
})

// ── live activity (R11; drain r1 D4) ──────────────────────────────────────

test('the task detail renders the active run&apos;s live activity, not just stage boundaries', async () => {
  const { view, log } = await task('t-ship', detailRoutes())
  expect(log.calls.some((c) => c.path === '/api/runs/r-ship'), 'the run card is never read').toBe(true)

  const panel = view.container.querySelector('.activity')!
  expect(panel, 'there is no live-activity panel — stage boundaries alone are not S2.2').not.toBeNull()
  expect(panel.getAttribute('data-run')).toBe('r-ship')

  const served = fixtures.runDetail() as unknown as RunDetail
  const text = panel.textContent ?? ''
  // The last-activity line and the monotonic counters, as served.
  expect(text).toContain(served.card.last_activity?.type ?? '')
  expect(text).toContain(`${String(served.card.counters.tokens)} tokens`)
  expect(text).toContain(`${String(served.card.counters.elapsed_s)} s elapsed`)
  expect(text).toContain('USD 1.42')
  // Monotonic counters, never a denominator.
  expect(text.toLowerCase()).not.toContain('%')
})

test('the live-activity panel re-reads on its own run&apos;s frames and settles the debt', async () => {
  const routes = detailRoutes()
  const log = scriptedFetch({ ...oversightRoutes(), ...routes })
  window.history.replaceState(null, '', '/tasks/t-ship')
  const stream = new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })
  const view = mount(<App stream={stream} />)
  await flush()
  act(() => FakeSource.last().open())
  await flush()
  expect(stream.status, 'a view still owes a re-snapshot').toBe('live')

  const before = log.calls.filter((c) => c.path === '/api/runs/r-ship').length
  act(() =>
    FakeSource.last().send('tool.completed', {
      seq: 900,
      run_id: 'r-ship',
      user_id: 'alice',
      type: 'tool.completed',
      schema_version: 1,
      topics: ['run'],
      payload: {},
      ts: '2026-07-20T09:10:00Z',
    }),
  )
  await flush()
  expect(log.calls.filter((c) => c.path === '/api/runs/r-ship').length, 'a run frame did not re-read the card').toBe(
    before + 1,
  )

  // A frame for ANOTHER run is not this panel's business.
  const held = log.calls.filter((c) => c.path === '/api/runs/r-ship').length
  act(() =>
    FakeSource.last().send('tool.completed', {
      seq: 901,
      run_id: 'r-elsewhere',
      user_id: 'alice',
      type: 'tool.completed',
      schema_version: 1,
      topics: ['run'],
      payload: {},
      ts: '2026-07-20T09:11:00Z',
    }),
  )
  await flush()
  expect(log.calls.filter((c) => c.path === '/api/runs/r-ship').length).toBe(held)
  view.unmount()
})

test('a task with no run says so rather than rendering an empty activity panel', async () => {
  const detail = fixtures.taskDetail() as unknown as Detail
  detail.runs = []
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
  expect(view.container.textContent).toContain('nothing running to watch')
})

// ── deliverables (R13; drain r1 D5) ───────────────────────────────────────

test('the task&apos;s REAL deliverables list with their immutable numbered revisions', async () => {
  const { view, log } = await task('t-ship', detailRoutes())
  expect(log.calls.some((c) => c.path === '/api/deliverables?task=t-ship')).toBe(true)

  const blocks = [...view.container.querySelectorAll('[data-deliverable]')].map((n) =>
    n.getAttribute('data-deliverable'),
  )
  // Every deliverable the SERVED list carries renders, in served order — stated
  // against the body rather than against a hand-copied set, so growing the fixture
  // world cannot make this pass while a row silently stops rendering.
  const served = fixtures.deliverablesOfTask() as { deliverables: { deliverable_id: string }[] }
  expect(served.deliverables.length, 'the served list is empty, so the check below proves nothing').toBeGreaterThan(1)
  expect(blocks, 'the task&apos;s own deliverables are not listed').toEqual(
    served.deliverables.map((d) => d.deliverable_id),
  )

  // The revisions are the SERVED numbered lineage, not a 1..N count inferred
  // from current_revision.
  const notes = view.container.querySelector('[data-deliverable="d-notes"]')!
  expect([...notes.querySelectorAll('[data-revision]')].map((n) => n.getAttribute('data-revision'))).toEqual(['1', '2'])
  expect(notes.querySelector('a')?.getAttribute('href')).toBe('/deliverables/d-notes')
})

test('follow-up lineage renders as lineage, and never as a deliverable revision', async () => {
  const detail = fixtures.taskDetail() as unknown as Detail
  detail.lineage.succeeded_by = [
    { task_id: 't-followup', deliverable_id: 'd-notes', revision_n: 2, created_ts: '2026-07-20T09:09:00Z' },
  ]
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
  // The follow-up TASK is in the lineage block…
  expect(view.container.querySelector('[data-lineage="succeeded-by"]')?.textContent).toContain('t-followup')
  // …and not among the deliverables, which are a different fact.
  const deliverables = view.container.querySelector('[data-deliverable="d-notes"]')!
  expect(deliverables.textContent, 'a follow-up task was listed as a deliverable revision').not.toContain('t-followup')
})

test('the D10 operator limb renders as "(as operator)" (drain r2 R2)', async () => {
  const { view } = await task('t-ops', detailRoutes())
  const rows = [...view.container.querySelectorAll('.decisions li')]
  expect(rows.length, 'the operator task has no decisions').toBeGreaterThan(0)

  const text = view.container.textContent ?? ''
  expect(text, 'the co-approval limb is invisible — an operator act reads as a member&apos;s').toContain('(as operator)')
  expect(text).toContain('priority_hint:t-ops')

  // The other direction, from the served body: alice's own decisions on her
  // own task carry no operator marker.
  const { view: mine } = await task('t-ship', detailRoutes())
  expect(mine.container.textContent, 'a member act was marked as the operator&apos;s').not.toContain('(as operator)')
})
