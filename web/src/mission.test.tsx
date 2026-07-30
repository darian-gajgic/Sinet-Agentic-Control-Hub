import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { AnswerView } from './History'
import { MetersPanel, bucketRuns } from './MissionControl'
import type { Answer, Meters, RunList } from './api'
import { FakeSource, fixtures, oversightRoutes, scriptedFetch } from './doubles'
import { EventStream } from './events'
import { flush, mount } from './testing'

/**
 * Mission control (Spec S15.5 ¶1; S3.1; S2.5; S2.10), driven against the golden
 * fixtures. Every row these assertions read is a row the Go handler serves.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

/** The fixture world's clock, so "recently finished" is a decidable question
 *  rather than one that depends on when the suite runs. */
const fixtureNow = Date.parse('2026-07-20T10:00:00Z')

async function mission() {
  const log = scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', '/')
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}

const servedRuns = () => (fixtures.runs() as unknown as RunList).runs
const servedMeters = () => fixtures.meters() as unknown as Meters

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
  vi.spyOn(Date, 'now').mockReturnValue(fixtureNow)
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── the buckets (R1) ──────────────────────────────────────────────────────

test('every bucket is filled from served rows, and no run falls off the screen', () => {
  const runs = servedRuns()
  const buckets = bucketRuns(runs, fixtureNow)
  const by = (id: string) => buckets.find((b) => b.id === id)!.runs.map((r) => r.run_id)

  // Two running runs: the release run and the intake run of the task the S15.7
  // handoff gave birth to (B6-7 D2 made that task and its run real rows, because
  // `asks.run_id` is a foreign key and its born card had to be a real ask).
  expect(by('running').sort()).toEqual(['r-ship', 't-chatborn.intake'])
  // Three queued runs: two of alice's and the operator's own.
  expect(by('queued').sort()).toEqual(['r-archive', 'r-ops', 'r-triage'])
  expect(by('blocked'), 'the run with an open ask is not under the human bucket').toEqual(['r-audit'])
  expect(by('parked'), 'blocked-on-a-human must not also appear as merely parked').toEqual(['r-stall'])
  expect(by('finished')).toEqual(['r-notes'])
  // `claimed` is named by none of the five buckets — and is still on screen.
  expect(by('other')).toEqual(['r-claim'])

  const placed = buckets.reduce((n, b) => n + b.runs.length, 0)
  expect(placed, 'a served run vanished from mission control').toBe(runs.length)
})

test('a finished run leaves the recently-finished bucket once it is no longer recent', () => {
  const runs = servedRuns()
  const later = Date.parse('2026-07-25T10:00:00Z')
  const finished = bucketRuns(runs, later).find((b) => b.id === 'finished')!
  expect(finished.runs, 'the recency window is not applied').toEqual([])
})

test('parked items show "parked until…" when the row carries one, and no time when it does not', async () => {
  const { view } = await mission()
  const text = view.container.textContent ?? ''
  expect(text, 'a park horizon the platform recorded was dropped').toContain('2026-07-20T12:00:00Z')
  expect(text, 'a park with no recorded horizon was given a fabricated one').toContain('parked, no horizon given')
})

test('who owns what is visible on every item, and every item drills through', async () => {
  const { view } = await mission()
  const rows = [...view.container.querySelectorAll('.row')]
  expect(rows.length).toBeGreaterThan(0)
  for (const row of rows) {
    expect(row.querySelector('.owner'), 'an item does not say whose it is (D2/S3.10)').not.toBeNull()
  }
  // Drill-through resolves through the URL contract, never a built string.
  const hrefs = rows.map((r) => r.querySelector('a')?.getAttribute('href'))
  expect(hrefs).toContain('/tasks/t-ship')
  expect(hrefs).toContain('/tasks/t-audit')
})

// ── the meters (R2) ───────────────────────────────────────────────────────

test('the gauge assumption is stated, pressure is absent without a denominator, and an undeclared budget shows no number', () => {
  const meters = servedMeters()
  const view = mount(<MetersPanel meters={meters} stale={false} error="" />)
  const text = view.container.textContent ?? ''

  expect(meters.lanes.some((l) => l.assumed)).toBe(true)
  expect(text, 'the S10.4 "assumed" label was dropped from the reading').toContain('assumed cache-read weight')

  expect(meters.lanes.every((l) => !l.pressure_applicable)).toBe(true)
  expect(text, 'a pressure figure was rendered with no declared denominator').toContain(
    'no declared budget, so there is no denominator',
  )

  expect(meters.lanes.every((l) => !l.budget_declared)).toBe(true)
  expect(text).toContain('no budget declared')
  view.unmount()
})

test('burn rates render at the served per-person grain and are never re-grained onto lanes', () => {
  const meters = servedMeters()
  const view = mount(<MetersPanel meters={meters} stale={false} error="" />)
  const text = view.container.textContent ?? ''

  // The burn-rate view's own columns are per person; the lane table has no
  // burn-rate column at all, which is the negative that matters.
  expect(text).toContain('Burn rate (per person, per observed day)')
  const laneHeaders = [...view.container.querySelectorAll('table.meters th')].map((th) => th.textContent)
  expect(laneHeaders, 'a per-lane burn rate would have to be computed by dividing').not.toContain('Burn rate')
  view.unmount()
})

test('a Layer-0 view that could not be read renders its reason, not an empty table', () => {
  const meters = servedMeters()
  meters.burn_rates = { absent: 'the S14.10 query surface is not wired in this process' }
  const view = mount(<MetersPanel meters={meters} stale={false} error="" />)
  expect(view.container.textContent).toContain('not wired in this process')
  view.unmount()
})

// ── the history panel (R3) ────────────────────────────────────────────────

test('the choice surface comes from the served registries, not a hand-written list', async () => {
  const { view } = await mission()
  const selects = [...view.container.querySelectorAll('.history-choices select')]
  expect(selects.length).toBe(2)

  const servedViews = (fixtures.historyViews() as { views: { Name: string }[] }).views.map((v) => v.Name)
  const rendered = [...selects[0].querySelectorAll('option')].map((o) => o.getAttribute('value')).filter((v) => v !== '')
  expect(rendered, 'the Layer-0 choices are not the served registry').toEqual(servedViews)

  const servedQueries = (fixtures.historyCatalog() as { queries: { name: string }[] }).queries.map((q) => q.name)
  const renderedQueries = [...selects[1].querySelectorAll('option')]
    .map((o) => o.getAttribute('value'))
    .filter((v) => v !== '')
  expect(renderedQueries).toEqual(servedQueries)
  expect(servedQueries.length).toBeGreaterThan(10)
})

test('choosing a view asks for it and renders the answer with its layer and confidence', async () => {
  const routes = oversightRoutes()
  routes['GET /api/events/views/cost_per_run'] = { body: fixtures.historyViewAnswer() }
  scriptedFetch(routes)
  window.history.replaceState(null, '', '/')
  const view = mount(<App stream={inertStream()} />)
  await flush()

  const select = view.container.querySelector('.history-choices select') as HTMLSelectElement
  act(() => {
    const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!
    setter.call(select, 'cost_per_run')
    select.dispatchEvent(new Event('change', { bubbles: true }))
  })
  await flush()

  // Scoped to the panel: the meters render answers of their own, and matching
  // the first `.answer` on the page would assert about the wrong one.
  const answer = view.container.querySelector('.history .answer')!
  expect(answer.getAttribute('data-layer')).toBe('0')
  expect(answer.getAttribute('data-confidence')).toBe('deterministic')
  expect(answer.textContent).toContain('r-ship')
  // The view's own plain-language note rides the answer.
  expect(answer.textContent).toContain('NO-RECEIPT')
  view.unmount()
})

test('a disambiguation card is rendered as an ANSWER, with its choices', () => {
  const card: Answer = {
    layer: 1,
    query: 'ask',
    confidence: 'canned',
    columns: [],
    rows: [],
    row_count: 0,
    truncated: false,
    card: {
      question: 'what did the deployment cost?',
      reason: 'two catalog questions match "cost"',
      choices: [
        { query: 'cost.per_run', category: 'cost', description: 'what one run consumed' },
        { query: 'cost.per_task', category: 'cost', description: 'what one task consumed' },
      ],
    },
  }
  const view = mount(<AnswerView answer={card} />)
  const node = view.container.querySelector('[data-card="disambiguation"]')
  expect(node, 'a disambiguation card was swallowed instead of rendered').not.toBeNull()
  expect(node?.textContent).toContain('two catalog questions match')
  expect([...view.container.querySelectorAll('button')].map((b) => b.textContent)).toEqual([
    'cost.per_run',
    'cost.per_task',
  ])
  view.unmount()
})

test('a refusal renders with its audit, and open SQL always shows its lower-confidence flag', () => {
  const refusal: Answer = {
    layer: 2,
    query: 'open-sql',
    confidence: 'lower',
    columns: [],
    rows: [],
    row_count: 0,
    truncated: false,
    audit: {
      question: 'delete everything',
      outcome: 'refused',
      refusal: 'the generated statement was not a single read-only SELECT',
      row_count: 0,
      truncated: false,
      alias: 'sql-open',
      as_operator: true,
    },
  }
  const view = mount(<AnswerView answer={refusal} />)
  const text = view.container.textContent ?? ''
  expect(view.container.querySelector('[data-audit="open-sql"]'), 'a refusal was rendered without its audit').not.toBeNull()
  expect(text).toContain('the generated statement was not a single read-only SELECT')
  // G3 D3.5: the flag is VISIBLE on this layer's answers.
  expect(view.container.querySelector('.warn-flag')?.textContent).toContain('lower')
  view.unmount()
})

test('the history panel says it is a query instrument, not a live projection', async () => {
  const { view } = await mission()
  const panel = view.container.querySelector('.history')!
  expect(panel.getAttribute('data-live')).toBe('query-instrument')
  expect(panel.textContent).toContain('not a live projection')
})
