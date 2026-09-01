import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { ReceiptView, approximationWords, currencyWords, eventTypeWords, fmtDuration } from './TaskDetail'
import type { Receipt, RunDetail, TaskDetail as Detail } from './api'
import { FakeSource, fixtures, oversightRoutes, type Scripted, scriptedFetch } from './doubles'
import { intakeResumeHref } from './Inbox'
import { EventStream } from './events'
import detailSource from './TaskDetail.tsx?raw'
import { click, flush, mount } from './testing'

/**
 * Task detail (Spec S15.5 ¶3; 9.2; S2.2; S2.4; §38 ruling (a); G2 D2.8),
 * driven against the golden fixtures — including BOTH artifact states, because
 * "a draft is labelled a draft" cannot be tested with only approved data.
 *
 * REWRITTEN IN PART 2026-08-06 (rework step 2, map §3 v3): the surface became
 * a structured OVERLAY WINDOW (itself role="dialog"), so the cancel-confirm
 * queries take the LAST dialog in the document — the kit portal appends after
 * the window; durations now render human-readable through `fmtDuration` (the
 * checkpoint-2 C2-13 ban on raw seconds), and the artifacts absence renders
 * plain words, never the pipeline's internal reason string. Every behavior
 * contract stands.
 */

/** The kit confirm's popup: the LAST dialog — the window itself is one. */
function confirmDialog(): Element {
  const all = document.querySelectorAll('[role="dialog"]')
  return all[all.length - 1]
}

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

async function task(id: string, extra: Record<string, Scripted> = {}) {
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

test('the plain criterion is what the requester reads; the formal wording sits one fold below, notation named once (review M6)', async () => {
  const { view } = await task('t-ship', detailRoutes())
  // The plain row carries the plain sentence ONLY — no formal restatement,
  // no notation parenthetical repeated per criterion.
  const plain = view.container.querySelector('[data-ac="AC-2"]')
  expect(plain?.textContent).toContain('Each entry says what changed in plain language.')
  expect(plain?.textContent).not.toContain('WHEN a reader opens the notes')
  // The formal wording is still the record, one fold below…
  const fold = view.container.querySelector('[data-acs-formal]')!
  expect(fold, 'the formal wording vanished instead of folding').not.toBeNull()
  expect(fold.textContent).toContain('WHEN a reader opens the notes THEN each entry reads as a sentence')
  // …with the notation said ONCE for the set, in plain words.
  const notation = (fold.textContent ?? '').split('Given / When / Then').length - 1
  expect(notation, 'the notation is repeated per criterion (or dropped)').toBe(1)
  view.unmount()
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

test('template assumed-default rows collapse into ONE line on the spec card (re-walk B)', async () => {
  // The old-era wall: every skipped interview slot leaves a per-slot template
  // row — "<Name> — …so I assumed a sensible default." — which names nothing
  // contestable and repeats. The plan card collapsed it at blocker #15; this
  // pins the SAME collapse on TaskDetail's own render path. Substantive rows
  // render whole; unrecognized rows render verbatim (driven by the substantive
  // row below, which the recognizer must NOT eat).
  const base = fixtures.taskDetail() as Record<string, unknown>
  const spec = {
    ...(base.spec as Record<string, unknown>),
    assumptions: [
      { text: 'The tone stays neutral and factual.', basis: 'planner' },
      { text: 'Audience — you asked me to go ahead without answering, so I assumed a sensible default.' },
      { text: 'Length — you asked me to go ahead without answering, so I assumed a sensible default.' },
      { text: 'Format — you asked me to go ahead without answering, so I assumed a sensible default.' },
    ],
  }
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: { ...base, spec } } })
  const text = view.container.textContent ?? ''
  expect(text.match(/so I assumed a sensible default/g) ?? [], 'the template wall rendered').toHaveLength(0)
  const line = view.container.querySelector('[data-assume-skipped]')!
  expect(line, 'the collapsed line is missing').not.toBeNull()
  expect(line.getAttribute('data-assume-skipped')).toBe('3')
  expect(line.textContent).toContain('Audience · Length · Format')
  expect(text, 'the substantive assumption must render whole, with its basis').toContain(
    'The tone stays neutral and factual. (planner)',
  )
})

test('a task with no drafted pair renders a plain-words absence, never the internal reason string', async () => {
  // REWRITTEN 2026-08-06 (C2-13): the served `artifacts_absent` is the
  // pipeline's own internal reason ("intake: invalid artifact: …") and the
  // old page rendered it as body text — the banned defect class. The card
  // renders plain words instead, and the raw string must NOT appear.
  const { view } = await task('t-archive', detailRoutes())
  const bare = fixtures.taskDetailBare() as unknown as Detail
  expect(bare.spec, 'the fixture is not actually the artifacts-absent case').toBeNull()
  expect(bare.artifacts_absent).not.toBe('')
  expect(view.container.textContent).toContain('no confirmed spec and plan are stored yet')
  expect(
    view.container.textContent,
    'the internal reason string rendered as body text (C2-13)',
  ).not.toContain(bare.artifacts_absent ?? '§never§')
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
  for (const banned of ['%', 'percent']) {
    expect(text, `the task detail rendered "${banned}"`).not.toContain(banned)
  }
  // 'eta' as a WORD — the shell's brand row concatenates to "sinetagentic…".
  expect(text, 'the task detail rendered "eta"').not.toMatch(/\beta\b/)
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

test('a receipt whose items are null on the wire renders honest absence, never a crash (F1 2026-08-16)', () => {
  // THE OPERATOR'S BLACK SCREEN: cancelling a task finalizes its run and
  // materializes a receipt that itemizes nothing — the Go slice has no
  // omitempty, so the wire says `"items": null`, and `.map` over it unmounted
  // the whole React root. The receipt must render the zero-calls fact as a
  // sentence instead.
  const receipt = fixtures.receipt() as unknown as Receipt
  receipt.items = null
  receipt.total_calls = 0
  receipt.total_priced_usd = 0
  const view = mount(<ReceiptView receipt={receipt} />)
  const text = view.container.textContent ?? ''
  expect(text).toContain('Nothing itemized')
  expect(text, 'a null-items receipt must still show its totals line').toContain('over 0 calls')
  view.unmount()
})

test('a run with no receipt renders the served reason', async () => {
  const detail = fixtures.taskDetail() as unknown as Detail
  detail.runs = [{ run_id: 'r-new', state: 'queued', created_ts: '2026-07-20T09:10:00Z', receipt_absent: 'no receipt yet' }]
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
  expect(view.container.textContent).toContain('no receipt yet')
})

test('a TERMINAL run renders the served state-aware absence — the demo-seed guess is dead (P3-GF14 R6; review M10)', async () => {
  // The wire now speaks to the run's own ending (api/reads.go
  // receiptAbsence), naming a crashed run's successor. The client-side
  // terminal guess that stood here ("a demo-seeded task can be minted
  // finished without one") misattributed on a REAL crashed run — exactly the
  // sentence this pin forbids.
  const detail = fixtures.taskDetail() as unknown as Detail
  detail.runs = [
    {
      run_id: 'r-broke',
      state: 'crashed',
      created_ts: '2026-07-20T09:10:00Z',
      receipt_absent:
        'this attempt broke partway, so no receipt was written for it — the platform carried the work on in a fresh run (r-broke.g2), and that run carries the receipt. What this attempt used is still in the task\'s own record.',
    },
  ]
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
  const text = view.container.textContent ?? ''
  expect(text).toContain('carried the work on in a fresh run (r-broke.g2)')
  expect(text, 'the hardcoded demo-seed guess survived on a receipts absence').not.toContain('demo-seeded')
})

test('a terminal run whose snapshot predates the member gets honest fallback words, not a promise and not a guess', async () => {
  const detail = fixtures.taskDetail() as unknown as Detail
  detail.runs = [{ run_id: 'r-old', state: 'completed', created_ts: '2026-07-20T09:10:00Z' }]
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: detail } })
  const text = view.container.textContent ?? ''
  expect(text).toContain('no receipt is recorded for this run')
  expect(text).not.toContain('demo-seeded')
})

test('the receipt summary speaks plain words for the machine members; the exact values stay on data attributes (exit walk F6)', () => {
  const receipt = fixtures.receipt() as unknown as Receipt
  const view = mount(<ReceiptView receipt={receipt} />)
  const text = view.container.textContent ?? ''
  // The engineer dialect is gone from the prose…
  expect(text).not.toContain('currency api-equivalent')
  expect(text).not.toContain('worst approximation tier')
  // …the plain words stand in its place…
  expect(text).toContain(currencyWords(receipt.currency))
  expect(text).toContain(approximationWords(receipt.worst_tier))
  // …and the served values are still on the record, humanized, never hidden.
  const p = view.container.querySelector('[data-receipt-currency]')!
  expect(p.getAttribute('data-receipt-currency')).toBe(receipt.currency)
  expect(p.getAttribute('data-worst-tier')).toBe(String(receipt.worst_tier))
  view.unmount()
})

test('the registered formula ref leaves the prose line — machine provenance rides the data attribute and the label\'s hover (exit walk F6)', () => {
  const receipt = fixtures.receipt() as unknown as Receipt
  const ref = receipt.direct_use.formula_ref
  expect(ref, 'the fixture lost its formula ref').not.toBe('')
  const view = mount(<ReceiptView receipt={receipt} />)
  const text = view.container.textContent ?? ''
  // A repository path with a section sign is not a sentence for a household
  // reader on a money surface.
  expect(text).not.toContain(ref)
  const p = view.container.querySelector('.direct-use')!
  expect(p.getAttribute('data-formula-ref')).toBe(ref)
  // The registered LABEL stays verbatim (its GF13 keep row).
  expect(text).toContain(receipt.direct_use.label)
  view.unmount()
})

test('the machine-member word maps: known values speak, unknown values render as themselves (§42)', () => {
  expect(currencyWords('api-equivalent')).toContain('not a bill')
  expect(currencyWords('real')).toContain('billed money')
  expect(currencyWords('space-credits')).toBe('currency space-credits')
  expect(approximationWords(1)).toContain("provider's own per-call numbers")
  expect(approximationWords(5)).toContain('never a silent zero')
  expect(approximationWords(9)).toBe('approximation tier 9')
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
  // The last-activity line and the monotonic counters, as served — the type
  // through its plain-words map since P3-GF9 (walk W4: raw event tokens on a
  // requester surface), the fact itself never dropped.
  expect(text).toContain(eventTypeWords(served.card.last_activity?.type ?? ''))
  expect(text).toContain(`${String(served.card.counters.tokens)} tokens`)
  // Human-readable elapsed (C2-13): the monotonic counter renders through
  // fmtDuration, and the raw-seconds form is the banned class.
  expect(text).toContain(`${fmtDuration(served.card.counters.elapsed_s)} elapsed`)
  expect(text, 'raw seconds rendered (C2-13)').not.toContain(`${String(served.card.counters.elapsed_s)} s elapsed`)
  expect(text).toContain('USD 1.42')
  // Monotonic counters, never a denominator.
  expect(text.toLowerCase()).not.toContain('%')
})

test('a subscription-lane zero says UNPRICED, and only a real estimate calls itself API-equivalent', async () => {
  // Reworked at the 2026-08-17 design review (#9): the old copy called the
  // served 0 "the API-equivalent figure", which contradicted the UNPRICED
  // receipts below it — 0 is not an equivalent of anything; it is the absence
  // of a price. A zero now says so, and the API-equivalent label is reserved
  // for a genuinely computed nonzero estimate.
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
  expect(panel.textContent, 'an unpriced zero must say no price exists').toContain(
    'no dollar price exists for these calls',
  )
  expect(panel.textContent, 'a zero must not pose as an API-equivalent figure').not.toContain('API-equivalent')
  view.unmount()

  const estimated = {
    ...served,
    card: { ...served.card, counters: { ...served.card.counters, api_equiv_cost_usd: 1.42, unpriced: true } },
  }
  const second = await task('t-ship', {
    ...detailRoutes(),
    'GET /api/runs/r-ship': { body: estimated },
  })
  const panel2 = second.view.container.querySelector('.activity')!
  expect(panel2.textContent, 'a real unpriced estimate carries the API-equivalent label').toContain(
    'API-equivalent estimate',
  )
  second.view.unmount()
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
  const dialog = confirmDialog()
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
    const said = confirmDialog().textContent ?? ''
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
  expect(confirmDialog().textContent).toContain('1 right now')
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
  const dialog = confirmDialog()
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
  const said = confirmDialog().textContent ?? ''
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
    const said = confirmDialog().textContent ?? ''
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

// ── D5 applied: the task detail's three live sites (P3-UI-4) ──────────────

/** The D5 live contract at one call site: the served instant renders VERBATIM
 *  inside `<time dateTime>` and the relative label sits BESIDE it. A site that
 *  rendered relative-only is the one forbidden outcome — the label is computed
 *  against a device clock and freezes between frames, so what stays true is the
 *  string the platform served. Clock-independent by design. */
function assertBeside(stamp: Element | null | undefined, served: string, what: string): void {
  // REWRITTEN 2026-08-27 (P3-GF9; walk W5): the raw UTC string left the
  // visible line — it rides the SAME element's dateTime and title now, while
  // the visible words are the relative label.
  expect(stamp, `${what}: no timestamp rendered`).toBeTruthy()
  expect(stamp!.getAttribute('dateTime'), `${what}: dateTime is not the served instant`).toBe(served)
  expect(stamp!.getAttribute('title'), `${what}: the exact instant left the hover`).toBe(served)
  expect(stamp!.textContent, `${what}: no relative words rendered`).not.toBe('')
  expect(stamp!.textContent, `${what}: the raw stamp is back on the visible line`).not.toBe(served)
}

test('opened, last activity and each stage boundary all render relative beside verbatim', async () => {
  const served = fixtures.taskDetail() as unknown as Detail
  const run = fixtures.runDetail() as unknown as RunDetail
  const { view } = await task('t-ship', detailRoutes())

  const opened = [...view.container.querySelectorAll('p')].find((p) => p.textContent?.includes('opened'))!
  expect(opened, 'the opened line did not render').toBeDefined()
  assertBeside(opened.querySelector('time'), served.created_ts, 'the opened stamp')
  assertBeside(view.container.querySelector('.activity-line time'), run.card!.last_activity!.ts, 'the activity stamp')

  const stage = served.stage_progress[0]
  expect(stage, 'the served task no longer records a stage boundary').toBeDefined()
  assertBeside(view.container.querySelector(`.stages [data-stage="${stage.stage}"] time`), stage.ts, 'a stage boundary')

  // The AUDIT site on this same surface deliberately did NOT move: a decision
  // is a record, and a record needs the instant alone. (The revision rows DID
  // move to the live face in the GF9 drain — review M7 cited them first-hand
  // as raw nanosecond noise on a requester surface; the verbatim UTC stays on
  // the element per the D5 primitive.)
  const decision = view.container.querySelector('.decisions time')
  expect(decision, 'the decision record lost its stamp').not.toBeNull()
  expect(decision!.textContent, 'an audit record dropped its verbatim instant').toBe(decision!.getAttribute('dateTime'))
  expect(decision!.parentElement!.textContent, 'an audit record grew a relative label').not.toMatch(/\bago\b|\bin \d/)
  view.unmount()
})

// ── the timeline rail (P3-UI-5) ───────────────────────────────────────────

/**
 * The rail's node classes over HAND-SCRIPTED bodies, each transcribed from the
 * Go producer that mints it.
 *
 * The golden world carries one `stage.started` with no outcome, no park history
 * and no wedged run, so these arms cannot be driven from it — and DRIVING them
 * would mint events and move the golden `cursor`, which is out of this packet's
 * scope. The §49 error-contract precedent applies: hand-scripted, with the
 * producer cited beside each value.
 *
 * The vocabulary is CLOSED and is the platform's own: `stage.finished` carries
 * `outcome ∈ {completed, split, error}` (internal/stage/stageevents.go:35–47,
 * registered at internal/eventlog/contract.go:431). `wedged` is the run card's
 * `run.wedged` / watchdog pause-and-flag projection (contract.go:427, served at
 * api.ts:462). Park intervals are the receipt's own `park_history` rows.
 */
function railBody(over: Partial<Detail> = {}): Detail {
  return { ...(fixtures.taskDetail() as unknown as Detail), ...over }
}

const stageRow = (over: Record<string, unknown> = {}) => ({
  run_id: 'r-ship',
  seq: 10,
  type: 'stage.finished',
  stage: 'execute',
  kind: 'execute-step',
  ts: '2026-07-20T09:02:00Z',
  ...over,
})

const nodesOf = (view: { container: HTMLElement }, kind: string) => [
  ...view.container.querySelectorAll(`.stages [data-node="${kind}"]`),
]

test('the rail places every served node class, and an ordinary step acquires none of their markings', async () => {
  const body = railBody({
    stage_progress: [
      stageRow({ seq: 9, type: 'stage.started', ts: '2026-07-20T09:01:00Z' }),
      stageRow({ seq: 10, outcome: 'completed', ts: '2026-07-20T09:02:00Z' }),
      stageRow({ seq: 11, outcome: 'split', ts: '2026-07-20T09:03:00Z' }),
      stageRow({ seq: 12, outcome: 'error', ts: '2026-07-20T09:05:00Z' }),
    ] as Detail['stage_progress'],
  })
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body } })

  // Each distinct treatment renders exactly where its outcome was served…
  expect(nodesOf(view, 'error')).toHaveLength(1)
  expect(nodesOf(view, 'split')).toHaveLength(1)
  expect(nodesOf(view, 'error')[0].textContent).toContain('error')
  expect(nodesOf(view, 'split')[0].textContent).toContain('split')
  // …and the two OTHER direction: the started row and the completed row are
  // plain steps, so the marking cannot be leaking onto ordinary boundaries.
  const plain = nodesOf(view, 'step')
  expect(plain.length, 'the unmarked boundaries lost their nodes').toBe(2)
  for (const node of plain) {
    expect(node.textContent, 'an ordinary step acquired a failure marking').not.toContain('ended short')
    expect(node.textContent, 'an ordinary step acquired a recovery marking').not.toContain('split at a checkpoint')
  }
  // The recovery and failure nodes say what they MEAN, not just their token.
  expect(nodesOf(view, 'error')[0].textContent).toContain('ended short of completing')
  expect(nodesOf(view, 'split')[0].textContent).toContain('carried on in a successor session')
  view.unmount()
})

test('consecutive identical boundaries fold into ONE counted row with both instants; distinct rows never fold (review L5)', async () => {
  // The instants sit AFTER the fixture's own decisions (09:04) and park
  // (09:02–09:03): the fold merges only ADJACENT identical rows, and an
  // intervening decision or park node correctly breaks the run — a fold
  // across one would re-order the story.
  const body = railBody({
    stage_progress: [
      stageRow({ seq: 1, stage: 'interview', type: 'intake.state', kind: '', ts: '2026-07-20T10:01:00Z' }),
      stageRow({ seq: 2, stage: 'interview', type: 'intake.state', kind: '', ts: '2026-07-20T10:02:00Z' }),
      stageRow({ seq: 3, stage: 'interview', type: 'intake.state', kind: '', ts: '2026-07-20T10:03:00Z' }),
      stageRow({ seq: 4, stage: 'plan', type: 'intake.state', kind: '', ts: '2026-07-20T10:04:00Z' }),
    ] as Detail['stage_progress'],
  })
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body } })
  const steps = nodesOf(view, 'step')
  // Three identical interview boundaries are ONE row; the plan row stays its own.
  expect(steps, 'identical consecutive rows did not fold (or distinct ones did)').toHaveLength(2)
  const folded = steps.find((n) => n.getAttribute('data-stage') === 'interview')!
  expect(folded.textContent).toContain('3 of these in a row')
  // Nothing served is dropped: the FIRST and the LATEST instants both render.
  const instants = [...folded.querySelectorAll('time')].map((t) => t.getAttribute('dateTime'))
  expect(instants).toEqual(['2026-07-20T10:01:00Z', '2026-07-20T10:03:00Z'])
  // The un-folded row carries no count chip.
  const plan = steps.find((n) => n.getAttribute('data-stage') === 'plan')!
  expect(plan.textContent).not.toContain('of these in a row')
  view.unmount()
})

test('the rail runs in served-instant order, and every stage fact still renders', async () => {
  const body = railBody({
    // Deliberately out of chronological order in the served array: the rail is
    // ordered by the instants the platform recorded, never by array position.
    stage_progress: [
      stageRow({ seq: 12, stage: 'verify', ts: '2026-07-20T09:05:00Z', outcome: 'completed' }),
      stageRow({ seq: 9, stage: 'plan', type: 'stage.started', ts: '2026-07-20T09:01:00Z' }),
    ] as Detail['stage_progress'],
  })
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body } })
  const stages = [...view.container.querySelectorAll('.stages [data-stage]')].map((n) => n.getAttribute('data-stage'))
  expect(stages, 'the rail did not order by the served instants').toEqual(['plan', 'verify'])

  // Every landed fact of a stage row is still on the node: name, type, kind,
  // outcome and the instant — through the timestamp primitive, never formatted.
  const verify = [...view.container.querySelectorAll('.stages [data-stage="verify"]')][0]
  const text = verify.textContent ?? ''
  // review #17: a mapped stage family renders in plain words ("checking the
  // work"); the marker itself stays on the node as data-stage and, when the
  // full step id differs from its family, as the visible mono id beside the
  // words. The other served facts render verbatim.
  expect(text, 'the rail dropped the plain words for verify').toContain('checking the work')
  // Since P3-GF9 (walk W4) the event type renders through its plain-words map
  // — the fact is never dropped, only the raw dotted token is; kind and
  // outcome render verbatim as before.
  for (const fact of [eventTypeWords('stage.finished'), 'execute-step', 'completed']) {
    expect(text, `the rail dropped the served ${fact}`).toContain(fact)
  }
  expect(verify.querySelector('time')?.getAttribute('dateTime')).toBe('2026-07-20T09:05:00Z')
  view.unmount()
})

test('an unnamed stage renders its absence, and an unrecognized outcome renders plainly rather than vanishing', async () => {
  const body = railBody({
    stage_progress: [
      stageRow({ seq: 9, stage: '', ts: '2026-07-20T09:01:00Z' }),
      // A value the closed vocabulary has never carried. The client owns no
      // vocabulary here, so a later addition must stay on the story.
      stageRow({ seq: 10, stage: 'compose', outcome: 'abandoned', ts: '2026-07-20T09:02:00Z' }),
    ] as Detail['stage_progress'],
  })
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body } })
  expect(view.container.querySelector('.stages')?.textContent).toContain('unnamed stage')
  const unknown = [...view.container.querySelectorAll('.stages [data-stage="compose"]')][0]
  expect(unknown, 'a row with an unrecognized outcome vanished from the rail').toBeDefined()
  expect(unknown.getAttribute('data-node'), 'an unrecognized outcome was forced into a known class').toBe('step')
  expect(unknown.textContent, 'the served outcome was dropped').toContain('abandoned')
  view.unmount()
})

test('a park interval is a stop node with its reason, its served duration and the inbox door', async () => {
  const served = fixtures.taskDetail() as unknown as Detail
  const receipt = { ...(served.runs[0].receipt as Receipt) }
  receipt.park_history = [
    // A CLOSED interval renders the SERVED duration — nothing is derived here.
    {
      parked_at: '2026-07-20T09:06:00Z',
      resumed_at: '2026-07-20T09:08:00Z',
      duration_seconds: 120,
      park_reason: 'weekly quota reached',
      resume_cause: 'window rolled',
    },
    // An OPEN one says it is still parked and carries the door.
    { parked_at: '2026-07-20T09:09:00Z', park_reason: 'waiting on the price table', ongoing: true },
  ]
  const body = railBody({ runs: [{ ...served.runs[0], receipt }] })
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body } })

  const parks = nodesOf(view, 'park')
  expect(parks, 'the rail carries no park node').toHaveLength(2)
  expect(parks[0].textContent).toContain('weekly quota reached')
  // The served duration renders through fmtDuration (C2-13): 120 s reads as
  // its human form — a deterministic rendering of the served figure, nothing
  // derived.
  expect(parks[0].textContent, 'the served duration was not rendered').toContain(fmtDuration(120))
  expect(parks[0].textContent).toContain('window rolled')
  expect(parks[1].textContent, 'an open interval does not say it is still parked').toContain('still parked')
  // The door POINTS: the release is `api.resumeRun` and its one call site is the
  // inbox, where the card holding the run lives (§48 OQ2).
  const door = parks[1].querySelector('a')!
  expect(door?.getAttribute('href')).toBe('/inbox')
  expect(parks[1].querySelector('button'), 'a park node offers a verb of its own').toBeNull()
  // The other direction: the CLOSED interval is not dressed as something
  // waiting for a person.
  expect(parks[0].querySelector('a'), 'a resumed park still offers a release door').toBeNull()
  // And the receipt below is unchanged — the rail is the chronology, the receipt
  // is the record, and both read the same served array.
  expect(view.container.querySelector('.parks')?.textContent).toContain('weekly quota reached')
  view.unmount()
})

test('a wedged run gets its stop node on the live lane, in plain words, pointing at the inbox', async () => {
  const served = fixtures.runDetail() as unknown as RunDetail
  const { view, log } = await task('t-ship', {
    ...detailRoutes(),
    'GET /api/runs/r-ship': { body: { ...served, card: { ...served.card, wedged: true } } },
  })
  const banner = view.container.querySelector('.activity + [data-stall="wedged"], [data-stall="wedged"]')!
  expect(banner, 'a wedged run wears only a terse flag').not.toBeNull()
  expect(banner.textContent).toContain('Flagged and paused')
  expect(banner.querySelector('a')?.getAttribute('href')).toBe('/inbox')
  // NO NEW VERB fires from this surface: the two cancels are the landed
  // controls and a banner is a door.
  expect(log.calls.filter((c) => c.method !== 'GET'), 'a stop node fired a verb').toEqual([])
  view.unmount()

  // The other direction: a run that is NOT wedged carries no wedge node.
  const { view: calm } = await task('t-ship', detailRoutes())
  expect(calm.container.querySelector('[data-stall="wedged"]'), 'an unwedged run was marked wedged').toBeNull()
  calm.unmount()
})

test('a blocked run says what unblocks it — and an unblocked one says nothing', async () => {
  const served = fixtures.runDetail() as unknown as RunDetail
  const { view } = await task('t-ship', {
    ...detailRoutes(),
    'GET /api/runs/r-ship': { body: { ...served, card: { ...served.card, waiting_on_human: true } } },
  })
  const banner = view.container.querySelector('[data-stall="blocked"]')!
  expect(banner, 'a run waiting on a person stalls silently').not.toBeNull()
  expect(banner.textContent).toContain('nothing restarts on its own')
  expect(banner.querySelector('a')?.getAttribute('href')).toBe('/inbox')
  view.unmount()

  const { view: calm } = await task('t-ship', detailRoutes())
  expect(calm.container.querySelector('[data-stall="blocked"]')).toBeNull()
  calm.unmount()
})

test('the rail invents no node class for a history the platform never records', async () => {
  // HAZARD #2, checked as a property of the source rather than of one render:
  // `tool.called` is registered DECLARE-ONLY with no producer at all
  // (internal/eventlog/contract.go:626), and no landed read on this surface
  // serves a per-tool-call history. A rail node for one would be a story about
  // events that do not exist.
  // Comments out, code in: this file's own rail doc EXPLAINS why neither type
  // is read, which is the record the next packet needs in front of it.
  const code = detailSource.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/(^|[^:])\/\/.*$/gm, '$1')
  expect(code, 'the rail reads a per-tool-call history').not.toContain('tool.called')
  expect(code, 'the rail reads a tool-completion history').not.toContain('tool.completed')
  // Non-vacuous: the stripper left the code it is supposed to scan.
  expect(code, 'the comment stripper ate the file').toContain('railNodes')
  // Every node class the rail can emit, read off the CLOSED union that bounds
  // them — so a new class cannot be added without this list being reconsidered,
  // and each one below traces to a row or field of the two landed reads:
  // step/error/split ← stage_progress + the stage.finished outcome vocabulary,
  // decision ← decisions[], park ← receipt.park_history[], terminal ←
  // runs[].state. There is no class for anything else.
  const union = /kind: ((?:'[a-z]+'(?: \| )?)+)$/m.exec(code)
  expect(union, 'the rail’s node union moved — this list no longer bounds anything').not.toBeNull()
  const classes = [...union![1].matchAll(/'([a-z]+)'/g)].map((m) => m[1]).sort()
  expect(classes, 'the rail grew a node class').toEqual([
    'decision',
    'error',
    'park',
    'split',
    'step',
    'terminal',
  ])
  // What IS served is the CURRENT tool — one name and one args digest on the
  // run card (api.ts:445) — and it renders as exactly that: a current value,
  // never a list of past calls. Hand-scripted, because the golden card serves
  // `tool: null`, which is itself the other direction.
  const served = fixtures.runDetail() as unknown as RunDetail
  expect(served.card.tool, 'the golden card now carries a tool — the null arm below is no longer driven').toBeNull()
  const { view } = await task('t-ship', {
    ...detailRoutes(),
    'GET /api/runs/r-ship': {
      body: { ...served, card: { ...served.card, tool: { name: 'read_file', args_digest: 'sha256:abcd' } } },
    },
  })
  const activity = view.container.querySelector('.activity')!
  expect(activity.textContent, 'the served current tool did not render').toContain('read_file')
  // ONE tool line, not a list: a second would be a history the read never sent.
  expect((activity.textContent ?? '').match(/read_file/g)).toHaveLength(1)
  view.unmount()

  // And with nothing served, nothing is rendered — no placeholder history.
  const { view: none } = await task('t-ship', detailRoutes())
  expect(none.container.querySelector('.activity')?.textContent, 'a tool line appeared over a served null').not.toContain(
    'tool ',
  )
  none.unmount()
})

test('no ticker was added with the rail — elapsed and durations are served figures', () => {
  // §32, at the one place a timeline would be tempted to tick. The tree-wide
  // scans pass unmodified; what is checked here is this file in particular,
  // because a rail is exactly where a "running for…" clock would appear.
  for (const banned of ['setInterval(', 'setTimeout(', 'requestIdleCallback(']) {
    expect(detailSource, `the rail started a ${banned} currency source`).not.toContain(banned)
  }
  // The rail renders its instants through the primitives and formats none: the
  // <time> file pin is exactly two files and this is not one of them.
  expect(detailSource, 'a rail node renders its own <time> element').not.toContain('<time')
  for (const formatter of ['toLocaleString(', 'toLocaleDateString(', 'toLocaleTimeString(', 'Intl.DateTimeFormat']) {
    expect(detailSource, `the rail formats a date with ${formatter}`).not.toContain(formatter)
  }
  // Probe: the matchers really discriminate.
  expect('const t = setInterval(fn, 1000)').toContain('setInterval(')
})

// ── the D8 layer on the task detail (P3-UI-5) ─────────────────────────────

test('the task detail teaches what it is, and the line calls no draft confirmed', async () => {
  const { view } = await task('t-ship', detailRoutes())
  const line = view.container.querySelector('[data-surface-what]')?.textContent ?? ''
  expect(line, 'the task detail carries no "what this is" line').not.toBe('')
  for (const fact of ['stage', 'decision', 'deliverables', 'receipts']) {
    expect(line.toLowerCase(), `the header line drops "${fact}"`).toContain(fact)
  }
  // THE TRUTH CONSTRAINT (§38 ruling (a)): the header must claim nothing about
  // the approval status of what it holds — that is StatusLine's job, per
  // artifact, from the served status.
  for (const overclaim of ['confirmed', 'approved', 'signed off']) {
    expect(line.toLowerCase(), `the header line overclaims: ${overclaim}`).not.toContain(overclaim)
  }
  view.unmount()
})

test('the task detail’s empty arms teach, and none renders over a read that has not landed', async () => {
  // A task with nothing recorded on it at all: no stage boundary, no decision,
  // no run. The bare fixture DOES carry an intake row and a queued run, so
  // asserting the empty rail over it would have asserted nothing.
  const bare = fixtures.taskDetailBare() as unknown as Detail
  const empty = { ...bare, stage_progress: [], decisions: [], runs: [] }
  const { view } = await task('t-archive', { ...detailRoutes(), 'GET /api/tasks/t-archive': { body: empty } })
  const text = view.container.textContent ?? ''
  expect(text).toContain('No stage boundary has been recorded yet.')
  expect(text, 'the empty rail does not teach what fills it').toContain('The rail fills in as the platform')
  expect(text).toContain('Nobody has had to decide anything on this task yet.')
  expect(text, 'the empty decisions block does not teach what fills it').toContain("recorded in its own right")
  view.unmount()

  // NONE-VS-NOT-LOADED, observed on the surface's own pending window: the
  // session LANDS, the task read never answers, and the rail teaches nothing
  // over it (drain r1, D1 — the instrument this replaces failed the session
  // read, so no surface mounted and it asserted about an empty page).
  const { view: pending } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { pending: true } })
  expect(pending.container.querySelector('[data-surface-what]'), 'no surface mounted, so this proves nothing').not.toBeNull()
  expect(pending.container.textContent, 'the loading affordance is missing').toContain('Catching up')
  expect(pending.container.textContent, 'a teaching empty rendered over a pending read').not.toContain(
    'No stage boundary has been recorded yet.',
  )
  pending.unmount()
})

test('the rail reads at 375px: no pinned width, and every responsive leg widens UP', async () => {
  const { view } = await task('t-ship', detailRoutes())
  const rail = view.container.querySelector('.stages')!
  const scope = [rail, ...rail.querySelectorAll('*')]
  expect(scope.length, 'the phone leg reached nothing').toBeGreaterThan(4)
  const pinned = /w-\[\d+px\]|min-w-\[\d+px\]/
  const fixed = scope.filter(
    (n) => pinned.test(n.className.toString()) || /\d+px/.test(n.getAttribute('style') ?? ''),
  )
  expect(fixed.map((n) => n.className.toString()), 'a rail node pins a pixel width a phone cannot fit').toEqual([])
  expect(rail.className, 'a max-width query entered the rail').not.toMatch(/\bmax-(sm|md|lg):/)
  // The node content wraps rather than pushing the page sideways — a stage name
  // and a served instant on one line is wider than 375px.
  expect(rail.querySelector('[data-node]')?.querySelector('div')?.className).toContain('flex-wrap')
  expect(pinned.test('flex w-[420px]'), 'the detector does not match what it forbids').toBe(true)
  view.unmount()
})

test('the run-standing node labels its instant as the OPENING, and is marked terminal only when it is', async () => {
  // Untested until now (drain r1, D3), and misreadable: this read serves NO
  // instant for a run's standing — the same fact the rail's own ordering rule
  // rests on — so the only stamp available is `created_ts`. Bare, in the slot
  // every sibling uses for "when this happened", it read as the moment the run
  // reached the state beside it.
  const served = fixtures.taskDetail() as unknown as Detail
  const body = railBody({
    stage_progress: [],
    decisions: [],
    runs: [
      { ...served.runs[0], run_id: 'r-done', state: 'completed', created_ts: '2026-07-20T09:00:00Z' },
      { ...served.runs[0], run_id: 'r-live', state: 'running', created_ts: '2026-07-20T09:01:00Z' },
    ],
  })
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body } })

  const nodes = nodesOf(view, 'terminal')
  expect(nodes, 'the rail carries no run-standing node').toHaveLength(2)
  for (const node of nodes) {
    // The instant says WHAT IT IS. Without the word it is a creation time
    // wearing the position of a state change.
    expect(node.textContent, 'the instant is unlabelled and reads as the state’s own').toContain('opened')
    expect(node.querySelector('time')?.getAttribute('dateTime')).toMatch(/^2026-07-20T09:0/)
  }
  // Both served states render VERBATIM, and the terminal one is the one marked
  // terminal — the other direction, so the marking is not simply always on.
  expect(nodes[0].textContent).toContain('completed')
  expect(nodes[1].textContent).toContain('running')
  const steps = nodesOf(view, 'step')
  expect(steps, 'a step node was rendered, so the negative below is not about nothing').toHaveLength(0)
  view.unmount()

  // And an ordinary step is NOT marked terminal.
  const withStep = railBody({
    stage_progress: [stageRow({ seq: 9, ts: '2026-07-20T09:02:00Z' })] as Detail['stage_progress'],
    decisions: [],
    runs: [{ ...served.runs[0], run_id: 'r-live', state: 'running' }],
  })
  const { view: mixed } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body: withStep } })
  expect(nodesOf(mixed, 'step'), 'the stage boundary lost its node').toHaveLength(1)
  expect(nodesOf(mixed, 'step')[0].textContent, 'an ordinary step was marked as a run standing').not.toContain(
    'stands at',
  )
  expect(nodesOf(mixed, 'terminal')).toHaveLength(1)
  mixed.unmount()
})

test('a state the client has never seen renders verbatim on the rail, with the landed tolerance', async () => {
  // The client owns NO state vocabulary (§42/§48): the FSM's values are the
  // server's, and a state a later version adds must stay on the story rather
  // than vanish or be renamed. This arm was undriven.
  const served = fixtures.taskDetail() as unknown as Detail
  const body = railBody({
    stage_progress: [],
    decisions: [],
    runs: [{ ...served.runs[0], run_id: 'r-odd', state: 'quiesced' }],
  })
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body } })
  const node = nodesOf(view, 'terminal')[0]
  expect(node, 'a run in an unrecognized state vanished from the rail').toBeDefined()
  expect(node.textContent, 'the served state was not rendered verbatim').toContain('quiesced')
  // It takes the not-yet-ended treatment, because it is not in the stored
  // terminal list — the same list `cancellable` reads the other way round, so
  // there is no second vocabulary to drift.
  expect(node.querySelector('[data-capacity]'), 'a capacity render leaked onto a rail node').toBeNull()
  expect(view.container.querySelector('[data-cancel-run="r-odd"]'), 'an unrecognized state was silently uncancellable')
    .not.toBeNull()
  view.unmount()
})

test('a served stage outcome takes ITS OWN hue — an error never renders in the success colour', async () => {
  // Byte-faithful to the shed `.stage-outcome` rule meant painting every
  // outcome green, including `error` (drain r1, D5b). The three values are the
  // rail's own closed vocabulary and the word must agree with the colour.
  const body = railBody({
    stage_progress: [
      stageRow({ seq: 9, stage: 'plan', outcome: 'completed', ts: '2026-07-20T09:01:00Z' }),
      stageRow({ seq: 10, stage: 'execute', outcome: 'split', ts: '2026-07-20T09:02:00Z' }),
      stageRow({ seq: 11, stage: 'verify', outcome: 'error', ts: '2026-07-20T09:03:00Z' }),
      stageRow({ seq: 12, stage: 'compose', outcome: 'abandoned', ts: '2026-07-20T09:04:00Z' }),
    ] as Detail['stage_progress'],
  })
  const { view } = await task('t-ship', { ...detailRoutes(), 'GET /api/tasks/t-ship': { body } })
  const toneOf = (stage: string) =>
    view.container.querySelector(`.stages [data-stage="${stage}"] .stage-outcome`)?.getAttribute('data-outcome-tone')
  expect(toneOf('plan')).toBe('ok')
  expect(toneOf('execute')).toBe('blue')
  expect(toneOf('verify'), 'a failed stage renders in the success hue').toBe('red')
  // An unrecognized outcome takes the neutral treatment rather than a guessed
  // meaning — the client owns no vocabulary here either.
  expect(toneOf('compose')).toBe('muted')
  // The colour really reaches the element, not just the attribute.
  const failed = view.container.querySelector('.stages [data-stage="verify"] .stage-outcome') as HTMLElement
  expect(failed.getAttribute('style')).toContain('var(--red)')
  view.unmount()
})

test('a parked run on the live lane says until WHEN, and an absent horizon is not invented', async () => {
  // R16's horizon limb (drain r1, D4). `parked_until` is served on this same run
  // card (api.ts:443) and had no render on this surface at all, so a parked run
  // said why it stopped and never until when.
  const served = fixtures.runDetail() as unknown as RunDetail
  const { view } = await task('t-ship', {
    ...detailRoutes(),
    'GET /api/runs/r-ship': {
      body: { ...served, card: { ...served.card, state: 'parked', parked_until: '2026-07-20T12:00:00Z' } },
    },
  })
  const banner = view.container.querySelector('[data-stall="parked"]')!
  expect(banner, 'a parked run on the live lane carries no horizon').not.toBeNull()
  expect(banner.textContent).toContain('parked until')
  // Through the landed idiom since P3-GF9 (walk W5): the visible words are
  // relative; the verbatim UTC rides dateTime and title on the same element.
  const stamp = banner.querySelector('time')!
  expect(stamp.getAttribute('dateTime')).toBe('2026-07-20T12:00:00Z')
  expect(stamp.getAttribute('title')).toBe('2026-07-20T12:00:00Z')
  expect(banner.querySelector('a')?.getAttribute('href')).toBe('/inbox')
  view.unmount()

  // The absence arm is BYTE-KEPT: a park the platform gave no horizon for does
  // not acquire one here.
  const { view: blank } = await task('t-ship', {
    ...detailRoutes(),
    'GET /api/runs/r-ship': { body: { ...served, card: { ...served.card, state: 'parked', parked_until: null } } },
  })
  const bare = blank.container.querySelector('[data-stall="parked"]')!
  expect(bare.textContent).toContain('parked, no horizon given')
  expect(bare.querySelector('time'), 'a horizon was invented for a park that has none').toBeNull()
  blank.unmount()

  // And a run that is not parked carries no park node on the live lane.
  const { view: running } = await task('t-ship', detailRoutes())
  expect(running.container.querySelector('[data-stall="parked"]'), 'an unparked run was marked parked').toBeNull()
  running.unmount()
})

// ── gate round 2 (2026-08-22), finding 1: the task-page answer door ────────

test('r2 finding 1: an intake question card routes the task-page door to the give-work journey', async () => {
  // t-chatborn's open ask is an INTERVIEW card (approvals-mine fixture): its
  // real answering surface is /new?task=…, so the door goes straight there —
  // the inbox card it used to bounce through had nothing to press.
  const chatborn = { ...fixtures.taskDetail(), task_id: 't-chatborn', title: 'Chat-born goal' }
  const { view } = await task('t-chatborn', {
    ...detailRoutes(),
    'GET /api/tasks/t-chatborn': { body: chatborn },
  })
  const door = view.container.querySelector('[data-act="answer"]')
  expect(door, 'the task page offers no answer door').not.toBeNull()
  expect(door?.textContent).toContain('Continue answering its questions')
  // Where it goes is pinned through the one shared resolver the button calls —
  // the inbox suite drives the same door's actual navigation end to end (the
  // task window's portal teardown makes an in-test navigation flaky here).
  const ask = (fixtures.approvalsMine().items as { task_id?: string }[]).find((i) => i.task_id === 't-chatborn')
  expect(intakeResumeHref(ask as Parameters<typeof intakeResumeHref>[0])).toBe('/new?task=t-chatborn')
})

test('r2 finding 1: a plan-approval ask keeps its inbox address — its verbs live on the card', async () => {
  const { view } = await task('t-ship', detailRoutes())
  const door = view.container.querySelector('[data-act="answer"]')
  expect(door, 'the task page offers no answer door').not.toBeNull()
  expect(door?.textContent).toContain('Answer its open card')
})
