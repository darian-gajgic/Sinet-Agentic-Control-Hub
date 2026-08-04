import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { ReceiptView } from './TaskDetail'
import type { Receipt, RunDetail, TaskDetail as Detail } from './api'
import { FakeSource, fixtures, oversightRoutes, scriptedFetch } from './doubles'
import { EventStream } from './events'
import { click, flush, mount } from './testing'

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

test('a subscription-lane run card says the figure is API-equivalent instead of printing a bare USD 0', async () => {
  // The unpriced lane prices UNPRICED, so the served cost is 0 — and this card
  // rendered it as a bare "USD 0", which asserts the run was free when what is
  // true is that nobody priced it. That is the same fabrication class the
  // workforce map's routed rows were already labelling; `meterReading` is one
  // expression across three surfaces and this was the surface that could not
  // say the word.
  const served = fixtures.runDetail() as unknown as RunDetail
  const unpriced = {
    ...served,
    card: { ...served.card, counters: { ...served.card.counters, api_equiv_cost_usd: 0, unpriced: true } },
  }
  const { view } = await task('t-ship', {
    ...detailRoutes(),
    'GET /api/runs/r-ship': { body: unpriced },
  })
  const panel = view.container.querySelector('.activity')!
  expect(panel.textContent, 'a served unpriced figure renders with no marking at all').toContain(
    'subscription lane, so this is the API-equivalent figure',
  )
  view.unmount()
})

test('NO money span on the run card prints a bare zero, whatever the seam serves', async () => {
  // Quantified over every money span this surface produces, the way the
  // workforce map's guard now is — a guard scoped to one cell is a guard
  // pointed away from wherever the next zero lands.
  const served = fixtures.runDetail() as unknown as RunDetail
  const zeroed = {
    ...served,
    card: { ...served.card, counters: { ...served.card.counters, api_equiv_cost_usd: 0, unpriced: true } },
  }
  for (const body of [served, zeroed]) {
    const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/runs/r-ship': { body } })
    const spans = [...view.container.querySelectorAll('.money')]
    expect(spans.length, 'the surface renders no money at all, so this asserts nothing').toBeGreaterThan(0)
    for (const el of spans) {
      const line = el.parentElement?.textContent ?? el.textContent ?? ''
      expect(line, 'a money figure reads as USD 0 with nothing saying why').not.toMatch(
        /^USD\s*0(\.0*)?\s*$/,
      )
    }
    view.unmount()
  }
  // Probe: the matcher really does catch the shape that shipped.
  expect('USD 0').toMatch(/^USD\s*0(\.0*)?\s*$/)
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

// ── feature 4.5: cancel, state-computed and honest (P3-UI-2 R3–R8) ────────
//
// EVERY SCRIPTED BODY BELOW IS TRANSCRIBED FROM ITS GO SOURCE, cited inline. A
// hand-written response that drifts from the handler is a test that proves
// nothing about production (§42's own root-cause lesson), so the shapes come
// from `stage.CancelOutcome` / `stage.TaskCancelOutcome`
// (internal/stage/cancel.go:128–148) and the refusal codes from `mapCancelErr`
// (internal/stage/surface.go:257–274).

/** withRuns re-serves the golden task detail with a chosen set of run states.
 *  The BODY is the fixture's; only the FSM states vary, and they are
 *  internal/run/run.go:40–52's own vocabulary. */
function withRuns(states: string[]): Detail {
  const detail = fixtures.taskDetail() as unknown as Detail
  const seed = detail.runs[0]
  detail.runs = states.map((state, i) => ({ ...seed, run_id: `r-${state}-${String(i)}`, state }))
  return detail
}

const allStates = [
  'new',
  'queued',
  'claimed',
  'running',
  'draining',
  'parked',
  'completed',
  'crashed',
  'finalized',
  'tombstoned',
  'died-at-gate',
]

/** The mapping's own detail strings, verbatim from internal/stage/cancel.go. */
const detailRunningCancelled =
  'running work cancelled: the run completes carrying the cancel reason (§14 reading 9)' // cancel.go:281
const detailParkedCancelled = 'parked work cancelled: finalize-with-card, no resume edge (§14 reading 9)' // cancel.go:305
const detailAlreadyEnded = 'the run had already ended; nothing was cancelled' // cancel.go:255
const detailCrashed = 'the run already crashed; the recovery ladder owns its disposition (S02.5)' // cancel.go:262

test('cancel renders on exactly the six non-terminal states and on no terminal one', async () => {
  const detail = withRuns(allStates)
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })

  const offered = [...view.container.querySelectorAll('[data-cancel-run]')].map((n) =>
    n.getAttribute('data-cancel-run'),
  )
  // The six the ratified mapping has an edge for — and NOT completed, crashed,
  // finalized, tombstoned or died-at-gate, each of which the verb answers with
  // an honest no-op rather than a cancellation.
  expect(offered).toEqual([
    'r-new-0',
    'r-queued-1',
    'r-claimed-2',
    'r-running-3',
    'r-draining-4',
    'r-parked-5',
  ])
  // Non-tautological control: every state really is on the page, so the six
  // above are a filter rather than a short fixture.
  expect([...view.container.querySelectorAll('.run-receipt')].length).toBe(allStates.length)
})

test('the task-level control appears only while some run of the task has not ended', async () => {
  const live = withRuns(['completed', 'parked'])
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: live } })
  expect(view.container.querySelector('[data-cancel="task"]')).not.toBeNull()

  const over = withRuns(['completed', 'crashed', 'tombstoned'])
  const { view: done } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: over } })
  expect(
    done.container.querySelector('[data-cancel="task"]'),
    'a task whose work has all ended still offered a cancel',
  ).toBeNull()
  expect(done.container.querySelector('[data-cancel-run]')).toBeNull()
})

test('nothing fires until the confirm is pressed, and then exactly once', async () => {
  const { view, log } = await task('t-ship', detailRoutes())
  const before = log.calls.filter((c) => c.method === 'POST').length

  click(view.container.querySelector('[data-cancel-run="r-ship"]'))
  await flush()
  // The dialog is open and says, in the platform's words, what cancelling a
  // RUNNING run does — and still nothing has fired.
  const dialog = document.querySelector('[role="dialog"]')!
  expect(dialog.textContent).toContain('It stops now')
  expect(dialog.textContent).toContain('shutdown ladder')
  expect(log.calls.filter((c) => c.method === 'POST').length, 'opening a confirm fired the verb').toBe(before)

  scriptedCancel(log, 'r-ship', {
    run_id: 'r-ship',
    from: 'running',
    to: 'completed',
    applied: true,
    ladder_invoked: true,
    detail: detailRunningCancelled,
  })
  click(dialog.querySelector('[data-act="confirm"]'))
  await flush()
  const posts = log.calls.filter((c) => c.method === 'POST' && c.path === '/api/runs/r-ship/cancel')
  expect(posts.length, 'the confirm fired the verb once').toBe(1)
})

/** scriptedCancel answers one run's cancel with a body in the served shape. */
function scriptedCancel(log: { set(k: string, r: { body?: unknown; status?: number }): void }, run: string, body: unknown, status?: number) {
  log.set(`POST /api/runs/${run}/cancel`, status === undefined ? { body } : { body, status })
}

test('an applied cancel renders the edge and the served sentence, then re-reads', async () => {
  const { view, log } = await task('t-ship', detailRoutes())
  const readsBefore = log.calls.filter((c) => c.path === '/api/tasks/t-ship').length
  scriptedCancel(log, 'r-ship', {
    run_id: 'r-ship',
    from: 'running',
    to: 'completed',
    applied: true,
    ladder_invoked: true,
    detail: detailRunningCancelled,
  })
  click(view.container.querySelector('[data-cancel-run="r-ship"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  const line = view.container.querySelector('[data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('applied')
  expect(line.textContent).toContain('running → completed')
  expect(line.textContent, 'the served sentence was paraphrased').toContain(detailRunningCancelled)
  // The re-read is what makes the rendered state true: this client never flips
  // a run's state itself (§42 — REST is the truth).
  expect(log.calls.filter((c) => c.path === '/api/tasks/t-ship').length).toBe(readsBefore + 1)
  // And the state on screen is still the SERVED one, because the re-read
  // answered with the same body.
  expect(view.container.querySelector('.run-receipt[data-run="r-ship"] .muted')?.textContent).toBe('running')
})

test('the parked and queued edges render their own served sentences', async () => {
  for (const leg of [
    { state: 'parked', to: 'finalized', detail: detailParkedCancelled },
    {
      state: 'queued',
      to: 'finalized',
      // cancel.go:329
      detail:
        'queued work cancelled before it ran: finalize-with-card, queue row settled in the same transaction (OQ1)',
    },
  ]) {
    const detail = withRuns([leg.state])
    const run = `r-${leg.state}-0`
    const { view, log } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
    scriptedCancel(log, run, {
      run_id: run,
      from: leg.state,
      to: leg.to,
      applied: true,
      ladder_invoked: false,
      detail: leg.detail,
    })
    click(view.container.querySelector(`[data-cancel-run="${run}"]`))
    await flush()
    // The confirm says what THIS state's cancel does, not a generic sentence.
    const said = document.querySelector('[role="dialog"]')!.textContent ?? ''
    expect(said).toContain(leg.state === 'parked' ? 'question card on it closes with it' : 'withdrawn before it starts')
    click(document.querySelector('[data-act="confirm"]'))
    await flush()
    const line = view.container.querySelector('[data-outcome]')!
    expect(line.getAttribute('data-outcome')).toBe('applied')
    expect(line.textContent).toContain(`${leg.state} → ${leg.to}`)
    expect(line.textContent).toContain(leg.detail)
    // The ladder is reported honestly: it did not run here, so nothing claims it did.
    expect(line.textContent, 'a ladder that never ran was reported').not.toContain('shutdown ladder')
    view.unmount()
  }
})

test('an applied:false answer is the platform being honest, not an error', async () => {
  // Both no-op arms of the mapping, driven on a run the fixture serves as
  // non-terminal so the control is offered and the SERVER is what says no.
  for (const leg of [
    { note: detailAlreadyEnded, from: 'completed' },
    { note: detailCrashed, from: 'crashed' },
  ]) {
    const { view, log } = await task('t-ship', detailRoutes())
    scriptedCancel(log, 'r-ship', {
      run_id: 'r-ship',
      from: leg.from,
      to: leg.from,
      applied: false,
      ladder_invoked: false,
      detail: leg.note,
    })
    click(view.container.querySelector('[data-cancel-run="r-ship"]'))
    await flush()
    click(document.querySelector('[data-act="confirm"]'))
    await flush()

    const line = view.container.querySelector('[data-outcome]')!
    expect(line.getAttribute('data-outcome'), 'an honest no-op was rendered as a failure').toBe('noop')
    expect(line.className, 'a no-op took the error styling').not.toContain('error')
    expect(line.textContent).toContain('nothing was cancelled')
    expect(line.textContent).toContain(leg.note)
    view.unmount()
  }
})

test('both 409s read as re-read-and-decide-again rather than as failures', async () => {
  for (const leg of [
    // internal/stage/surface.go:262 / :266, with the sentinels' own messages
    // (internal/stage/cancel.go:151–160).
    {
      code: 'claim_in_flight',
      detail: 'stage: the run is being dispatched right now — retry the cancel in a moment',
    },
    {
      code: 'cancel_raced',
      detail: 'stage: the run changed state while the cancel was being applied — re-read it and decide again',
    },
  ]) {
    const { view, log } = await task('t-ship', detailRoutes())
    const readsBefore = log.calls.filter((c) => c.path === '/api/tasks/t-ship').length
    scriptedCancel(log, 'r-ship', { error: leg.code, detail: leg.detail }, 409)
    click(view.container.querySelector('[data-cancel-run="r-ship"]'))
    await flush()
    click(document.querySelector('[data-act="confirm"]'))
    await flush()

    const line = view.container.querySelector('[data-outcome]')!
    expect(line.getAttribute('data-outcome')).toBe('retry')
    expect(line.className, 'a re-read was rendered in the failure style').not.toContain('error')
    expect(line.textContent).toContain('re-read it and decide again')
    expect(line.textContent).toContain(leg.detail)
    // A 409 is precisely the case where the reader needs fresh state.
    expect(log.calls.filter((c) => c.path === '/api/tasks/t-ship').length).toBe(readsBefore + 1)
    view.unmount()
  }
})

test('a refusal renders the server’s own code and sentence', async () => {
  for (const leg of [
    { status: 503, code: 'not_wired', detail: 'the cancel choreography is not wired in this process' },
    // BYTE-EXACT from the source each one comes from. The cancel verbs
    // authorize through `authorizeOwner` (internal/api/reads.go:444–452), whose
    // refusal is the bare word — NOT `readPerson`'s S01.9 sentence, which
    // belongs to the `?person=` filter on the read family and never reaches
    // here — and whose 404 is `fmt.Errorf("%s not found", what)` with what="run".
    { status: 403, code: 'forbidden', detail: 'forbidden' },
    { status: 404, code: 'not_found', detail: 'run not found' },
  ]) {
    const { view, log } = await task('t-ship', detailRoutes())
    scriptedCancel(log, 'r-ship', { error: leg.code, detail: leg.detail }, leg.status)
    click(view.container.querySelector('[data-cancel-run="r-ship"]'))
    await flush()
    click(document.querySelector('[data-act="confirm"]'))
    await flush()

    const line = view.container.querySelector('[data-outcome]')!
    expect(line.getAttribute('data-outcome')).toBe('failed')
    expect(line.textContent).toContain(leg.code)
    expect(line.textContent).toContain(leg.detail)
    view.unmount()
  }
})

test('a task cancel itemizes every run, and claims the board only when something was cancelled', async () => {
  const detail = withRuns(['running', 'completed'])
  const { view, log } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
  // stage.TaskCancelOutcome, internal/stage/cancel.go:143–148.
  log.set('POST /api/tasks/t-ship/cancel', {
    body: {
      task_id: 't-ship',
      kanban_status: 'cancelled',
      applied: true,
      runs: [
        {
          run_id: 'r-running-0',
          from: 'running',
          to: 'completed',
          applied: true,
          ladder_invoked: false,
          detail: detailRunningCancelled,
        },
        {
          run_id: 'r-completed-1',
          from: 'completed',
          to: 'completed',
          applied: false,
          ladder_invoked: false,
          detail: detailAlreadyEnded,
        },
      ],
    },
  })
  click(view.container.querySelector('[data-cancel="task"]'))
  await flush()
  // The confirm counts what it is about to act on, from served state.
  expect(document.querySelector('[role="dialog"]')?.textContent).toContain('1 right now')
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  const items = [...view.container.querySelectorAll('.cancel-runs li')]
  expect(items.length, 'the task outcome was summarized instead of itemized').toBe(2)
  expect(items[0].getAttribute('data-outcome')).toBe('applied')
  expect(items[0].textContent).toContain('running → completed')
  expect(items[1].getAttribute('data-outcome')).toBe('noop')
  expect(items[1].textContent).toContain(detailAlreadyEnded)
  expect(view.container.querySelector('.task-actions [data-outcome]')?.textContent).toContain(
    'the board reads cancelled',
  )
})

test('a task cancel that cancelled nothing never says the task is cancelled', async () => {
  const detail = withRuns(['completed', 'parked'])
  const { view, log } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
  log.set('POST /api/tasks/t-ship/cancel', {
    body: {
      task_id: 't-ship',
      // The server sends the ZERO VALUE when nothing fired: `TaskCancelOutcome`
      // is built with no Kanban and the field is only ever set inside the
      // `if out.Applied` branch (cancel.go:221–241, drain D4 — a no-op cancel
      // never rewrites the board), and the json tag carries no omitempty. So
      // production serves "", and a body claiming the task's real column here
      // would be a shape no handler produces.
      kanban_status: '',
      applied: false,
      runs: [
        {
          run_id: 'r-completed-0',
          from: 'completed',
          to: 'completed',
          applied: false,
          ladder_invoked: false,
          detail: detailAlreadyEnded,
        },
      ],
    },
  })
  click(view.container.querySelector('[data-cancel="task"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  const line = view.container.querySelector('.task-actions [data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('noop')
  expect(line.textContent).toContain('nothing was cancelled')
  expect(line.textContent, 'a no-op cancel reported a board move that never happened').not.toContain('the board reads')
  // And the task is still rendered as what it is.
  expect(view.container.textContent, 'the task was relabelled cancelled by a cancel that cancelled nothing').not.toContain(
    'executing · cancelled',
  )
})

test('the act is the kit’s danger button, and the dismiss fires nothing', async () => {
  const { view, log } = await task('t-ship', detailRoutes())
  const trigger = view.container.querySelector('[data-cancel-run="r-ship"]')!
  expect(trigger.className, 'the cancel trigger is not the kit danger variant').toContain('--red')
  click(trigger)
  await flush()
  const confirm = document.querySelector('[data-act="confirm"]')!
  expect(confirm.className).toContain('--red')

  const before = log.calls.filter((c) => c.method === 'POST').length
  click(document.querySelector('[data-act="dismiss"]'))
  await flush()
  expect(log.calls.filter((c) => c.method === 'POST').length, 'dismissing the confirm fired the verb').toBe(before)
  expect(document.querySelector('[data-act="confirm"]'), 'the dialog stayed open after dismissing').toBeNull()
})

test('a member acts on their OWN run, and the verb is called with their subject', async () => {
  // NOT the default session: `signedIn` is alice as OPERATOR, so a scoping
  // test that forgets this override proves nothing about members (§38).
  const asBob = {
    body: { authenticated: true, user: { user_id: 'bob', display_name: 'Bob', role: 'member', pin_set: true } },
  }
  const detail = fixtures.taskDetail() as unknown as Detail
  detail.owner = 'bob'
  const { view, log } = await task('t-ship', {
    ...detailRoutes(),
    'GET /api/auth/session': asBob,
    'GET /api/tasks/t-ship': { body: detail },
  })
  scriptedCancel(log, 'r-ship', {
    run_id: 'r-ship',
    from: 'running',
    to: 'completed',
    applied: true,
    ladder_invoked: false,
    detail: detailRunningCancelled,
  })
  click(view.container.querySelector('[data-cancel-run="r-ship"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  // Path-only: the run IS the subject, and the acting person is the session's.
  expect(log.calls.filter((c) => c.path === '/api/runs/r-ship/cancel' && c.method === 'POST').length).toBe(1)
  expect(view.container.querySelector('[data-outcome]')?.getAttribute('data-outcome')).toBe('applied')
})

test('a member never reaches another owner’s cancel, because they never reach the task', async () => {
  const asBob = {
    body: { authenticated: true, user: { user_id: 'bob', display_name: 'Bob', role: 'member', pin_set: true } },
  }
  // The READ is owner-scoped (S01.9), so the cross-owner direction is refused
  // one layer before any control could render. That is the structural leg: no
  // body, no control, nothing to press.
  const { view } = await task('t-ship', {
    ...detailRoutes(),
    'GET /api/auth/session': asBob,
    'GET /api/tasks/t-ship': { status: 403, body: { error: 'forbidden', detail: 'not yours to read' } },
  })
  expect(view.container.querySelector('[data-cancel-run]')).toBeNull()
  expect(view.container.querySelector('[data-cancel="task"]')).toBeNull()
  expect(view.container.textContent).toContain('not yours to read')
})

test('a run.state_changed frame re-reads the task, so a cancel elsewhere lands here too', async () => {
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

  const before = log.calls.filter((c) => c.path === '/api/tasks/t-ship').length
  act(() =>
    FakeSource.last().send('run.state_changed', {
      seq: 950,
      run_id: 'r-ship',
      user_id: 'alice',
      type: 'run.state_changed',
      schema_version: 1,
      topics: ['board'],
      payload: {},
      ts: '2026-07-20T09:12:00Z',
    }),
  )
  await flush()
  // A cancel mints no decision row: it RIDES run.state_changed (§39), which
  // this surface already subscribes to — so the act lands here with no
  // client-side patching and no live.ts change.
  expect(
    log.calls.filter((c) => c.path === '/api/tasks/t-ship').length,
    'a cancel’s own frame did not re-read the task',
  ).toBe(before + 1)
  view.unmount()
})

test('the cancel controls are reachable and operable at phone width', async () => {
  const { view } = await task('t-ship', detailRoutes())
  expect(view.container.querySelector('[data-cancel="task"]')).not.toBeNull()
  expect(view.container.querySelector('[data-cancel-run="r-ship"]')).not.toBeNull()
  click(view.container.querySelector('[data-cancel-run="r-ship"]'))
  await flush()
  const dialog = document.querySelector('[role="dialog"]')!
  expect(dialog.querySelector('[data-act="confirm"]')).not.toBeNull()

  // The §41-B method: jsdom has no layout, so the checkable property is that
  // nothing here pins a pixel width for a 375px viewport to scroll past.
  const pinned = /w-\[\d+px\]|min-w-\[\d+px\]/
  const scope = [...view.container.querySelectorAll('.task-actions *, .run-actions *'), ...dialog.querySelectorAll('*'), dialog]
  expect(scope.length).toBeGreaterThan(5)
  expect(
    scope.filter((n) => pinned.test(n.className.toString()) || /\d+px/.test(n.getAttribute('style') ?? '')).length,
    'a cancel control pins a pixel width a phone cannot fit',
  ).toBe(0)
  expect(pinned.test('w-[520px] flex')).toBe(true)
})

// ── drain r1: the confirm tells the truth about a NON-ATOMIC verb (D1) ────

test('the task confirm does not promise an atomicity the backend does not have', async () => {
  const detail = withRuns(['running', 'claimed'])
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
  click(view.container.querySelector('[data-cancel="task"]'))
  await flush()
  const said = document.querySelector('[role="dialog"]')!.textContent ?? ''
  // `stage.CancelTask` walks its runs and each cancel COMMITS ON ITS OWN
  // (cancel.go:221–233), so a mid-dispatch refusal stops the walk with
  // everything already cancelled still cancelled.
  expect(said).toContain('the request stops there')
  expect(said).toContain('the runs already cancelled stay cancelled')
  expect(said, 'the confirm promised an all-or-nothing the verb does not give').not.toContain('nothing is cancelled')
})

test('a task-cancel 409 says the walk stopped part-way; a run-cancel 409 says nothing fired', async () => {
  const detail = withRuns(['running', 'claimed'])
  const { view, log } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
  const inFlight = 'stage: the run is being dispatched right now — retry the cancel in a moment'
  log.set('POST /api/tasks/t-ship/cancel', { status: 409, body: { error: 'claim_in_flight', detail: inFlight } })
  click(view.container.querySelector('[data-cancel="task"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  const line = view.container.querySelector('.task-actions [data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('retry')
  expect(line.textContent).toContain('the request stopped where it was refused')
  expect(line.textContent).toContain('anything already cancelled stays cancelled')
  expect(line.textContent, 'the multi-subject verb claimed nothing fired').not.toContain('nothing fired')
  expect(line.textContent).toContain(inFlight)

  // The single-subject verb DOES fire nothing on a 409, and still says so.
  scriptedCancel(log, 'r-running-0', { error: 'claim_in_flight', detail: inFlight }, 409)
  click(view.container.querySelector('[data-cancel-run="r-running-0"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  expect(view.container.querySelector('.run-receipt[data-run="r-running-0"] [data-outcome]')?.textContent).toContain(
    'nothing fired',
  )
})

// ── drain r1: the two confirm arms that had copy and no assertion (D4/N10) ─

test('the mid-dispatch and unrecognized-state confirms each say their own truth', async () => {
  for (const leg of [
    { state: 'claimed', says: 'being dispatched right this moment' },
    { state: 'new', says: 'being dispatched right this moment' },
    // A state this client has never seen is OFFERED, and its confirm says what
    // is actually true: the platform decides which ending applies.
    { state: 'quiescing', says: 'The platform decides which ending applies' },
  ]) {
    const detail = withRuns([leg.state])
    const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
    click(view.container.querySelector(`[data-cancel-run="r-${leg.state}-0"]`))
    await flush()
    const said = document.querySelector('[role="dialog"]')!.textContent ?? ''
    expect(said, `the ${leg.state} confirm does not say what a cancel does to it`).toContain(leg.says)
    // Nothing invented: no deletion, no rollback, on any arm.
    expect(said.toLowerCase()).not.toContain('delete')
    expect(said.toLowerCase()).not.toContain('roll back')
    view.unmount()
  }
})

// ── drain r1: the operator's act on ANOTHER owner's run, fired (D4) ───────

test('the operator cancels another owner’s run, and the act really fires', async () => {
  // The default session IS alice-as-operator; the SUBJECT is bob's. That is the
  // operator limb of `authorizeOwner` (reads.go:444–452) exercised as an act
  // rather than as a render.
  const detail = fixtures.taskDetail() as unknown as Detail
  detail.owner = 'bob'
  const { view, log } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
  expect(view.container.querySelector('.owner')?.textContent).toContain('bob')
  scriptedCancel(log, 'r-ship', {
    run_id: 'r-ship',
    from: 'running',
    to: 'completed',
    applied: true,
    ladder_invoked: false,
    detail: detailRunningCancelled,
  })
  click(view.container.querySelector('[data-cancel-run="r-ship"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  expect(log.calls.filter((c) => c.method === 'POST' && c.path === '/api/runs/r-ship/cancel').length).toBe(1)
  expect(view.container.querySelector('[data-outcome]')?.getAttribute('data-outcome')).toBe('applied')
})
