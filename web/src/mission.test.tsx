import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { AnswerView } from './History'
import { MetersPanel, bucketRuns } from './MissionControl'
import type { Answer, Meters, RunList } from './api'
import {
  FakeSource,
  fixtures,
  historyAskKey,
  historyAskPath,
  historyAskQuestion,
  historySearchQuestion,
  oversightRoutes,
  scriptedFetch,
} from './doubles'
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

  expect(by('running')).toEqual(['r-ship'])
  // Three queued runs: two of alice's and the operator's own.
  expect(by('queued').sort()).toEqual(['r-archive', 'r-ops', 'r-triage'])
  // Two runs are waiting on a person — bob's price audit and the intake run of
  // the task the S15.7 handoff gave birth to — and they are listed in the order
  // the server sent them, because `bucketRuns` pushes in served order and the
  // client never re-ranks. `waiting_on_human` is a SERVED flag (parked AND an
  // open ask), so this is what fills the bucket: `r-ship` is running with two
  // open asks of its own and is correctly not here.
  expect(by('blocked'), 'the human bucket is not the served waiting-on-human set, in served order').toEqual(
    runs.filter((r) => r.waiting_on_human).map((r) => r.run_id),
  )
  expect(by('blocked').length, 'the waiting-on-human bucket is empty, so the assertion above proves nothing')
    .toBeGreaterThan(1)
  expect(by('parked'), 'blocked-on-a-human must not also appear as merely parked').toEqual(
    runs.filter((r) => r.state === 'parked' && !r.waiting_on_human).map((r) => r.run_id),
  )
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

test('the lane figures and the per-person run counts agree with the served runs body', () => {
  // The meters and the runs list are two reads of ONE world, so their figures are
  // checkable against each other rather than taken on trust — and until now they
  // were not: MissionControl renders "{active} of {total}" and Fleet renders the
  // active count, but no assertion over the shared world touched either number,
  // so a world change moved them silently (drain r2 R6). These are the server's
  // own definitions read back, never a client computation: `total_runs` is every
  // run of an (owner, lane), `active_runs` the non-terminal ones, `parked_runs`
  // the parked ones, and `per_person.runs` every run a person owns.
  const runs = servedRuns()
  const meters = servedMeters()
  const active = new Set(['new', 'queued', 'claimed', 'running', 'parked', 'draining'])
  expect(meters.lanes.length, 'the lane table is empty, so the loop below would assert nothing').toBeGreaterThan(0)
  for (const lane of meters.lanes) {
    const own = runs.filter((r) => r.owner === lane.owner && r.lane === lane.lane)
    expect(lane.total_runs, `${lane.owner}/${lane.lane} total_runs`).toBe(own.length)
    expect(lane.active_runs, `${lane.owner}/${lane.lane} active_runs`).toBe(
      own.filter((r) => active.has(r.state)).length,
    )
    expect(lane.parked_runs, `${lane.owner}/${lane.lane} parked_runs`).toBe(
      own.filter((r) => r.state === 'parked').length,
    )
  }
  // The per-person grain, the same way. Its `unpriced_runs` column is NOT
  // checkable from here — it counts receipts, which the runs list does not carry
  // — so that one figure stays guarded by the Go byte-compare alone, said out
  // loud rather than implied.
  const answer = meters.per_person.answer!
  const person = answer.columns.indexOf('user_id')
  const count = answer.columns.indexOf('runs')
  expect(answer.rows.length, 'the per-person view is empty').toBeGreaterThan(0)
  for (const row of answer.rows) {
    const owner = row[person] as string
    expect(row[count], `${owner} runs`).toBe(runs.filter((r) => r.owner === owner).length)
  }
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
  // The SAME guard the Layer-1 sibling carries, for the same reason: this body's
  // rows grow with the shared world, and asserting only the layer and one run id
  // let the born run's state be anything at all. A run holding an open interview
  // card cannot read `running` — the pipeline parks it in the transaction that
  // issues the card — so that is pinned here where it is rendered, not only in
  // the Go byte-compare.
  const served = fixtures.historyViewAnswer() as unknown as Answer
  expect(answer.querySelectorAll('tbody tr')).toHaveLength(served.rows.length)
  const born = served.rows.find((r) => r[0] === 't-chatborn.intake')
  expect(born, 'the served body no longer carries the chat-born run').toBeDefined()
  expect(born![served.columns.indexOf('state')], 'a run holding an open interview card cannot read running')
    .toBe('parked')
  const bornRow = [...answer.querySelectorAll('tbody tr')].find((tr) => tr.textContent?.includes('t-chatborn.intake'))
  expect(bornRow?.textContent, 'the served state did not reach the screen').toContain('parked')
  view.unmount()
})

test('choosing a catalog question asks the Layer-1 route and renders its canned answer', async () => {
  // The Layer-1 DIRECT leg of the same picker, from the committed body. It had
  // no test at all, which is why `history-query-answer.json` was a golden file
  // no web assertion read — its rows grew with the world and nothing here would
  // have noticed (drain r2 R6). `status.runs_active` declares no slots, so the
  // Ask button is the whole gesture.
  const routes = oversightRoutes()
  routes['GET /api/events/query/status.runs_active'] = { body: fixtures.historyQueryAnswer() }
  scriptedFetch(routes)
  window.history.replaceState(null, '', '/')
  const view = mount(<App stream={inertStream()} />)
  await flush()

  const select = [...view.container.querySelectorAll('.history-choices select')][1] as HTMLSelectElement
  act(() => {
    const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!
    setter.call(select, 'status.runs_active')
    select.dispatchEvent(new Event('change', { bubbles: true }))
  })
  await flush()
  const ask = [...view.container.querySelectorAll('.history-slots button')][0] as HTMLButtonElement
  act(() => ask.dispatchEvent(new MouseEvent('click', { bubbles: true })))
  await flush()

  const answer = view.container.querySelector('.history .answer')!
  expect(answer.getAttribute('data-layer')).toBe('1')
  // A catalog query is CANNED, never `deterministic`: the confidence is the
  // store's own reading of the layer and this surface serves it as-is.
  expect(answer.getAttribute('data-confidence')).toBe('canned')
  // And the answer's rows are the served rows — including the intake run of the
  // chat-born task, which is `parked` because the pipeline parks a run when it
  // issues a card. A run holding an open interview card cannot read `running`.
  const served = fixtures.historyQueryAnswer() as unknown as Answer
  expect(answer.querySelectorAll('tbody tr')).toHaveLength(served.rows.length)
  const born = served.rows.find((r) => r[0] === 't-chatborn.intake')!
  expect(born, 'the served body no longer carries the chat-born run').toBeDefined()
  expect(born[served.columns.indexOf('state')], 'a run holding an open interview card cannot read running: the pipeline parks it in the same transaction that issues the card')
    .toBe('parked')
  const bornRow = [...answer.querySelectorAll('tbody tr')].find((tr) => tr.textContent?.includes('t-chatborn.intake'))
  expect(bornRow?.textContent, 'the served state did not reach the screen').toContain('parked')
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

// ── the two free-text layers: ask & search (P3-UI-4) ──────────────────────

function type(input: HTMLInputElement, value: string): void {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
  act(() => {
    setter.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

function submitForm(form: HTMLFormElement): void {
  act(() => {
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  })
}

const formOf = (view: { container: HTMLElement }, name: 'ask' | 'search') =>
  view.container.querySelector(`[data-form="${name}"]`) as HTMLFormElement
const fieldOf = (form: HTMLFormElement, name: 'ask' | 'search') =>
  form.querySelector(`[data-field="${name}"]`) as HTMLInputElement
const actOf = (form: HTMLFormElement, name: 'ask' | 'search') => form.querySelector(`[data-act="${name}"]`)!

/** Fire one of the two free-text controls with the question its committed body
 *  is the answer to. Asking a DIFFERENT question and serving these bodies would
 *  be a fixture pretending to be a reply. */
async function fire(view: { container: HTMLElement }, name: 'ask' | 'search'): Promise<void> {
  const form = formOf(view, name)
  type(fieldOf(form, name), name === 'ask' ? historyAskQuestion : historySearchQuestion)
  submitForm(form)
  await flush()
}

test('the ask layer answers with its disambiguation card, and a card IS an answer', async () => {
  const { view, log } = await mission()
  await fire(view, 'ask')

  // The request is the one the committed body answers, path and all.
  expect(log.calls.some((c) => c.method === 'GET' && c.path === historyAskPath)).toBe(true)

  const served = fixtures.historyAskAnswer() as unknown as Answer
  const answer = view.container.querySelector('.history .answer')!
  // Served at 200 with a layer and a confidence like every other layer's reply —
  // not swallowed into the panel's error banner.
  expect(view.container.querySelector('.history .error')).toBeNull()
  expect(answer.getAttribute('data-layer')).toBe('1')
  expect(answer.getAttribute('data-confidence')).toBe('canned')
  expect(answer.getAttribute('data-confidence')).toBe(served.confidence)

  const card = answer.querySelector('[data-card="disambiguation"]')!
  expect(card, 'the card was swallowed instead of rendered').not.toBeNull()
  // The reason renders VERBATIM. It is the platform's own account of why it
  // could not read the sentence, and this surface does not paraphrase it — the
  // no-local-tier posture is what every test process and every unwired dev host
  // actually gets (internal/history/layer1.go:154–157).
  expect(served.card!.reason).toContain('the local tier is not wired here')
  expect(card.textContent).toContain(served.card!.reason)
  expect(card.textContent).toContain(served.card!.question)
  // Every served choice reaches the screen with its category and description.
  const choices = [...card.querySelectorAll('li')]
  expect(choices).toHaveLength(served.card!.choices.length)
  expect(choices.length).toBeGreaterThan(1)
  expect(choices[0].textContent).toContain(served.card!.choices[0].query)
  expect(choices[0].textContent).toContain(served.card!.choices[0].category)
  expect(choices[0].textContent).toContain(served.card!.choices[0].description)
  view.unmount()
})

test('choosing from the card SELECTS that question with its own empty slots and fires nothing', async () => {
  // THE DEFECT THIS REPLACES. The landed binding fired the chosen catalog name
  // with whatever slots the picker happened to be holding. A card that came from
  // the free-text ask has no slots of its own, so the reader would have received
  // a different question's answer under the heading of the one they clicked.
  const routes = oversightRoutes()
  routes['GET /api/events/query/cost.for_run?slot_run_id=r-ship'] = { body: fixtures.historyQueryAnswer() }
  const log = scriptedFetch(routes)
  window.history.replaceState(null, '', '/')
  const view = mount(<App stream={inertStream()} />)
  await flush()
  await fire(view, 'ask')

  const before = log.calls.length
  const choice = [...view.container.querySelectorAll('[data-card="disambiguation"] button')].find(
    (b) => b.textContent === 'cost.for_run',
  )!
  expect(choice, 'the served card no longer offers a question with typed slots').toBeDefined()
  act(() => choice.dispatchEvent(new MouseEvent('click', { bubbles: true })))
  await flush()

  // NOTHING WAS ASKED.
  expect(log.calls.length, 'a card choice blind-fired a query').toBe(before)
  // The Layer-1 picker now holds the choice, and its own slot renders EMPTY.
  const picker = [...view.container.querySelectorAll('.history-choices select')][1] as HTMLSelectElement
  expect(picker.value, 'the choice did not reach the picker it names').toBe('cost.for_run')
  const slot = view.container.querySelector('.history-slots input') as HTMLInputElement
  expect(slot, 'the chosen question rendered no slot to fill').not.toBeNull()
  expect(slot.value, 'the slot arrived carrying another question’s value').toBe('')
  // The card stays on screen: it is the context the choice was made in.
  expect(view.container.querySelector('[data-card="disambiguation"]')).not.toBeNull()

  // And firing it deliberately asks THAT question with the slot the person typed.
  type(slot, 'r-ship')
  const askButton = view.container.querySelector('.history-slots button') as HTMLButtonElement
  act(() => askButton.dispatchEvent(new MouseEvent('click', { bubbles: true })))
  await flush()
  expect(log.calls.some((c) => c.path === '/api/events/query/cost.for_run?slot_run_id=r-ship')).toBe(true)
  view.unmount()
})

test('search sends the question VERBATIM and renders the store’s own rows, excerpts and notes', async () => {
  const { view, log } = await mission()
  await fire(view, 'search')

  // VERBATIM. Nothing is stripped, folded or de-punctuated here: redaction is
  // the store's property (redact-before-match, marker-strip, verify-the-hit,
  // bound the excerpt — internal/history/search.go:81–145), and a second
  // implementation on this side would be free to disagree with the one the
  // guarantee actually rests on.
  const sent = log.calls.find((c) => c.path.startsWith('/api/events/search'))!
  expect(sent, 'the search control reached no route').toBeDefined()
  expect(new URLSearchParams(sent.path.split('?')[1]).get('q')).toBe(historySearchQuestion)
  expect(historySearchQuestion, 'the question under test carries nothing a stripper would touch').toContain('?')

  const served = fixtures.historySearchAnswer() as unknown as Answer
  const answer = view.container.querySelector('.history .answer')!
  expect(answer.getAttribute('data-layer')).toBe('0')
  expect(answer.getAttribute('data-confidence')).toBe('deterministic')
  expect(answer.querySelectorAll('tbody tr')).toHaveLength(served.rows.length)
  // The corpus is REAL and the body proves it: two indexed kinds, two owners and
  // the projector's own two ref shapes (a run id, and an event ref for a row
  // with no run). An empty index would have made every assertion here vacuous
  // (§38 D12).
  expect(served.rows.length).toBeGreaterThan(1)
  expect(new Set(served.rows.map((r) => r[1]))).toEqual(new Set(['drift', 'verdict']))
  expect(new Set(served.rows.map((r) => r[3]))).toEqual(new Set(['platform', 'alice']))
  // Every served note renders, the codor-C2 statement included.
  for (const n of served.notes ?? []) expect(answer.textContent).toContain(n)
  expect((served.notes ?? []).join(' ')).toContain('codor C2')
  // And the bounded excerpt text itself reaches the screen.
  expect(answer.textContent).toContain('the messages endpoint deprecates a parameter')
  view.unmount()
})

test('a redaction marker inside an excerpt renders verbatim, and so does the secret-only note', async () => {
  // HAND-SCRIPTED, and every string is transcribed from the code that produces
  // it, because neither arm exists in the fixture world's corpus:
  //
  //  - the marker survives INTO the excerpt on purpose — internal/history's own
  //    search_test.go:88–95 requires it, "the excerpt should show that something
  //    WAS redacted, not silently omit it";
  //  - the two notes are internal/history/search.go:96 and :100, verbatim, and
  //    the second one is the arm where the store returns before matching at all
  //    (search.go:101), so there are no rows and that is the whole answer.
  const routes = oversightRoutes()
  const secretOnly: Answer = {
    layer: 0,
    query: 'search.history',
    confidence: 'deterministic',
    question: 'sk-ant-api03-XXXXXXXXXXXXXXXXXXXXXXXX',
    columns: [],
    rows: [],
    row_count: 0,
    truncated: false,
    notes: [
      'matched against the redacted corpus (codor C2): a query for a secret’s plaintext cannot confirm the secret',
      'the question carried a secret-shaped value; it was redacted and contributes no search term',
      'the question carried no searchable term',
    ],
  }
  const withMarker: Answer = {
    ...secretOnly,
    question: 'anthropic key rotation',
    columns: ['rowid', 'kind', 'ref', 'user_id', 'excerpt'],
    rows: [[7, 'run_summary', 'r-rotate', 'alice', 'rotated the [REDACTED:anthropic_key] and restarted the lane']],
    row_count: 1,
    notes: [secretOnly.notes![0]],
  }
  routes['GET /api/events/search?q=anthropic+key+rotation'] = { body: withMarker }
  routes['GET /api/events/search?q=sk-ant-api03-XXXXXXXXXXXXXXXXXXXXXXXX'] = { body: secretOnly }
  scriptedFetch(routes)
  window.history.replaceState(null, '', '/')
  const view = mount(<App stream={inertStream()} />)
  await flush()

  const form = formOf(view, 'search')
  type(fieldOf(form, 'search'), 'anthropic key rotation')
  submitForm(form)
  await flush()
  // The marker is part of the excerpt the store served, so it renders as text.
  // Hiding it would turn "something was redacted here" into "nothing was here".
  expect(view.container.querySelector('.history .answer')!.textContent).toContain('[REDACTED:anthropic_key]')

  type(fieldOf(form, 'search'), secretOnly.question!)
  submitForm(form)
  await flush()
  const answer = view.container.querySelector('.history .answer')!
  for (const n of secretOnly.notes!) expect(answer.textContent).toContain(n)
  // A question that was nothing but a secret is an ANSWER with no rows, not an
  // error and not an empty screen.
  expect(answer.querySelectorAll('tbody tr')).toHaveLength(0)
  expect(answer.textContent).toContain('This answer carries no rows.')
  expect(view.container.querySelector('.history .error')).toBeNull()
  view.unmount()
})

test('an empty question is HELD at both forms, fires nothing, and never reads as busy', async () => {
  const { view, log } = await mission()
  const before = log.calls.length

  for (const name of ['ask', 'search'] as const) {
    const form = formOf(view, name)
    const button = actOf(form, name)
    expect(button.hasAttribute('disabled'), `${name} offers a fire it would only have refused`).toBe(true)
    // HELD IS NOT BUSY. Nothing is in flight, and the button says so rather than
    // leaving "why can't I press this?" to be guessed (§49 N7).
    expect(button.getAttribute('data-busy')).toBe('false')
    expect(form.querySelector(`[data-held="${name}"]`)?.textContent).toContain('A question is required')
    submitForm(form)
  }
  await flush()
  // FETCH-LOG EMPTY: the held forms reached nothing at all.
  expect(log.calls.length, 'a held form reached the control plane').toBe(before)

  // Whitespace alone is still empty, because the transport trims before it
  // refuses (internal/api/historyapi.go:197–204) and this mirrors that ONE bound.
  const askIn = fieldOf(formOf(view, 'ask'), 'ask')
  type(askIn, '   ')
  expect(actOf(formOf(view, 'ask'), 'ask').hasAttribute('disabled')).toBe(true)
  submitForm(formOf(view, 'ask'))
  await flush()
  expect(log.calls.length, 'a whitespace question was sent').toBe(before)
  view.unmount()
})

test('busy means exactly one request in flight, and it is a different state from held', async () => {
  const { view } = await mission()
  const form = formOf(view, 'ask')
  const button = () => actOf(formOf(view, 'ask'), 'ask')

  // (1) HELD — not filled in: disabled, not busy.
  expect(button().getAttribute('data-busy')).toBe('false')
  expect(button().hasAttribute('disabled')).toBe(true)

  // (2) READY — neither held nor busy, and the held reason is gone.
  type(fieldOf(form, 'ask'), historyAskQuestion)
  expect(button().getAttribute('data-busy')).toBe('false')
  expect(button().hasAttribute('disabled')).toBe(false)
  expect(form.querySelector('[data-held="ask"]')).toBeNull()

  // (3) IN FLIGHT — busy, and disabled BY busy. The other form is untouched:
  //     busy is a fact about one request, not about the panel.
  submitForm(form)
  expect(button().getAttribute('data-busy'), 'an in-flight request did not read as busy').toBe('true')
  expect(button().hasAttribute('disabled')).toBe(true)
  expect(actOf(formOf(view, 'search'), 'search').getAttribute('data-busy')).toBe('false')

  // (4) SETTLED.
  await flush()
  expect(button().getAttribute('data-busy')).toBe('false')
  view.unmount()
})

test('a typed question survives the answer it produced AND the refusal it produced', async () => {
  const routes = oversightRoutes()
  const log = scriptedFetch(routes)
  window.history.replaceState(null, '', '/')
  const view = mount(<App stream={inertStream()} />)
  await flush()

  await fire(view, 'ask')
  expect(view.container.querySelector('[data-card="disambiguation"]')).not.toBeNull()
  // The draft is still there: fixing one word must not mean retyping the
  // sentence, and asking the same thing again is a legitimate act on a surface
  // that says answers are point-in-time.
  expect(fieldOf(formOf(view, 'ask'), 'ask').value).toBe(historyAskQuestion)

  // Now the layer refuses. The SERVER's own sentence renders, and the draft
  // survives that too.
  log.set(historyAskKey, { status: 503, body: { error: 'not_wired', detail: 'the S14.10 query surface is not wired in this process' } })
  submitForm(formOf(view, 'ask'))
  await flush()
  expect(view.container.querySelector('.history .error')?.textContent).toBe(
    'The control plane answered 503: the S14.10 query surface is not wired in this process',
  )
  expect(fieldOf(formOf(view, 'ask'), 'ask').value, 'a refusal cost the person their question').toBe(historyAskQuestion)
  view.unmount()
})

test('the lower-confidence flag belongs to layer 2 alone — absent on ask and search, present on an escalation', async () => {
  const { view } = await mission()

  // (a) ASK. It stops at its card and never falls through to Layer 2 — the
  //     transport says so at internal/api/historyapi.go:25–28 and the store is
  //     pinned by TestLayer2IsNotReachedBySilentFallthrough — so an ask answer
  //     carries its own `canned` confidence and NO warn flag.
  await fire(view, 'ask')
  const asked = view.container.querySelector('.history .answer')!
  expect(asked.getAttribute('data-confidence')).toBe('canned')
  expect(asked.querySelector('.warn-flag'), 'an ask answer wore the Layer-2 flag').toBeNull()
  expect(asked.textContent).toContain('confidence: canned')

  // (b) SEARCH is Layer 0 and deterministic, for the same reason.
  await fire(view, 'search')
  const searched = view.container.querySelector('.history .answer')!
  expect(searched.getAttribute('data-confidence')).toBe('deterministic')
  expect(searched.querySelector('.warn-flag'), 'a search answer wore the Layer-2 flag').toBeNull()
  view.unmount()

  // (c) The OTHER direction, through the same shared component the chat
  //     escalation renders with: a layer-2 body flags itself.
  const escalated: Answer = {
    layer: 2,
    query: 'open-sql',
    confidence: 'lower-confidence',
    columns: [],
    rows: [],
    row_count: 0,
    truncated: false,
  }
  const flagged = mount(<AnswerView answer={escalated} />)
  expect(flagged.container.querySelector('.warn-flag')?.textContent).toContain('lower-confidence')
  expect(flagged.container.querySelector('.answer')?.getAttribute('data-confidence')).toBe('lower-confidence')
  flagged.unmount()
})

test('the two controls are reachable and operable at 375px', async () => {
  // jsdom has no layout engine, so what is checkable is the STRUCTURE that makes
  // a phone-width surface work (the §41-B method): nothing pins a pixel width,
  // the row wraps, and every responsive utility widens UP.
  const { view } = await mission()
  const scope = [
    ...view.container.querySelectorAll('[data-form="ask"], [data-form="ask"] *, [data-form="search"], [data-form="search"] *'),
  ]
  expect(scope.length).toBeGreaterThan(8)
  const pinned = /w-\[\d+px\]|min-w-\[\d+px\]/
  const fixed = scope.filter((n) => pinned.test(n.className.toString()) || /\d+px/.test(n.getAttribute('style') ?? ''))
  expect(fixed.map((n) => n.className.toString()), 'a control pins a pixel width a phone cannot fit').toEqual([])
  // Probe: the detector really matches what it forbids.
  expect(pinned.test('w-[520px] flex')).toBe(true)
  // The two forms sit in a column at phone width and only lay out in a row at
  // the landed breakpoint — the phone is the base, not the exception (S1.10).
  const row = view.container.querySelector('[data-form="ask"]')!.parentElement!
  expect(row.className).toContain('flex-col')
  expect(row.className).toContain('md:flex-row')
  expect(row.className, 'a max-width query entered the surface').not.toMatch(/\bmax-(sm|md|lg):/)
  // And both are genuinely operable from there.
  await fire(view, 'search')
  expect(view.container.querySelector('.history .answer')?.getAttribute('data-layer')).toBe('0')
  view.unmount()
})

// ── D5 applied: relative BESIDE the verbatim instant (P3-UI-4) ────────────

test('a run line renders relative time beside its served instant, never instead of it', async () => {
  const { view } = await mission()
  const parked = servedRuns().find((r) => r.parked_until)!
  expect(parked, 'the fixture world no longer parks a run with a horizon').toBeDefined()
  const row = [...view.container.querySelectorAll('.rows .row')].find((r) => r.textContent?.includes(parked.task_id))!
  expect(row, 'the parked run did not reach the screen').toBeDefined()

  const stamps = [...row.querySelectorAll('time')]
  // ParkedUntil's horizon first, then the last-activity stamp.
  for (const [stamp, served] of [
    [stamps[0], parked.parked_until!],
    [stamps[1], parked.last_activity_ts!],
  ] as const) {
    expect(stamp.getAttribute('dateTime')).toBe(served)
    expect(stamp.textContent, 'the verbatim UTC was dropped').toBe(served)
    const beside = stamp.parentElement!.parentElement!.textContent ?? ''
    expect(beside.endsWith(served), 'the instant is not rendered beside its label').toBe(true)
    expect(beside.length, 'a relative label replaced the instant').toBeGreaterThan(served.length)
  }
  // With the clock pinned the two labels are exact, and they run in OPPOSITE
  // directions: a past stamp freezes toward understating age, a future horizon
  // toward overstating the time left. Only the instants beside them correct that.
  expect(row.textContent).toContain('58m ago')
  expect(row.textContent).toContain('in 2h')
  view.unmount()
})

test('a lane with no served horizon says so rather than inventing one to be live about', async () => {
  const { view } = await mission()
  const lane = servedMeters().lanes.find((l) => l.parked_runs > 0)!
  expect(lane.parked_until ?? null, 'the fixture lane now carries a horizon').toBeNull()
  const cell = [...view.container.querySelectorAll('.meters .absent')].map((n) => n.textContent)
  expect(cell, 'the park-with-no-horizon arm did not survive the migration').toContain('parked, no horizon given')
  view.unmount()
})
