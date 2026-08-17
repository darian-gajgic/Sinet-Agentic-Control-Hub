import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import type { Meters } from './api'
import { FakeSource, fixtures, oversightRoutes, type Scripted, scriptedFetch } from './doubles'
import { EventStream } from './events'
import appSource from './App.tsx?raw'
import fleetSource from './Fleet.tsx?raw'
import { click, flush, mount, typeInto } from './testing'

/** The shipped app tree, for the two source-level negatives below: a read the
 *  render happened not to trigger is invisible to a DOM walk. */
const appSources = import.meta.glob('./**/*.{ts,tsx}', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

/**
 * Fleet overview (Spec S15.5 ¶4; 9.3; S2.5; S3.10; D2), driven against the
 * golden meters fixture.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

async function fleet(extra: Record<string, Scripted> = {}) {
  scriptedFetch({ ...oversightRoutes(), ...extra })
  window.history.replaceState(null, '', '/fleet')
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return view
}

const setSelect = (el: HTMLSelectElement, value: string) => {
  act(() => {
    const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!
    setter.call(el, value)
    el.dispatchEvent(new Event('change', { bubbles: true }))
  })
}

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

test('every fleet row says whose account it is', async () => {
  const view = await fleet()
  const rows = [...view.container.querySelectorAll('.fleet-lanes tbody tr')]
  expect(rows.length).toBeGreaterThan(0)
  for (const row of rows) {
    expect(row.getAttribute('data-owner'), 'a fleet row does not name its owner (D2/S3.10)').toBeTruthy()
    expect(row.querySelector('.owner')).not.toBeNull()
  }
  // Three owners are present, which is what makes the no-cross-owner-total
  // rule meaningful rather than vacuous.
  expect(new Set(rows.map((r) => r.getAttribute('data-owner')))).toEqual(new Set(['alice', 'bob', 'op']))
})

test('nothing is summed across owners: there is no total row and no total figure', async () => {
  const view = await fleet()
  const text = (view.container.textContent ?? '').toLowerCase()
  // The receipt has a per-run total, which is the server's own figure; the
  // fleet has none, because a cross-person total would be the client doing
  // arithmetic over money.
  expect(text, 'the fleet rendered a cross-owner total').not.toContain('household total')
  expect(text).not.toContain('all owners')
  const meters = fixtures.meters() as unknown as Meters
  expect(meters.lanes.length).toBeGreaterThan(1)
})

test('a parked lane whose latest park carries no marker says so, and an unparked lane shows nothing', async () => {
  const view = await fleet()
  const parked = view.container.querySelector('.fleet-lanes tbody tr[data-lane="zai"]')!
  // The served truth: this lane has parked runs, and the most recent park
  // carried no reset time. The honest absence is the whole point — inventing a
  // horizon here is exactly the failure ParkedUntil exists to prevent.
  const served = (fixtures.meters() as unknown as Meters).lanes.find((l) => l.lane === 'zai')!
  expect(served.parked_runs).toBeGreaterThan(0)
  expect(served.parked_until).toBeNull()
  expect(parked.textContent).toContain('parked, no horizon given')

  const idle = view.container.querySelector('.fleet-lanes tbody tr[data-lane="local"]')!
  expect(idle.textContent, 'an unparked lane was given a park horizon').not.toContain('parked')
})

test('a lane whose park DOES carry a horizon renders it verbatim', async () => {
  const meters = fixtures.meters() as unknown as Meters
  const lane = meters.lanes.find((l) => l.lane === 'zai')!
  lane.parked_until = '2026-07-20T12:00:00Z'
  const view = await fleet({ 'GET /api/meters': { body: meters } })
  const row = view.container.querySelector('.fleet-lanes tbody tr[data-lane="zai"]')!
  expect(row.textContent).toContain('parked until')
  expect(row.textContent).toContain('2026-07-20T12:00:00Z')
})

test('the person and lane filters narrow the display only', async () => {
  const view = await fleet()
  const selects = [...view.container.querySelectorAll('.fleet-filters select')] as HTMLSelectElement[]
  expect(selects.length).toBe(2)

  setSelect(selects[0], 'bob')
  await flush()
  const owners = [...view.container.querySelectorAll('.fleet-lanes tbody tr')].map((r) => r.getAttribute('data-owner'))
  expect(owners).toEqual(['bob'])

  setSelect(selects[0], '')
  setSelect(selects[1], 'local')
  await flush()
  const lanes = [...view.container.querySelectorAll('.fleet-lanes tbody tr')].map((r) => r.getAttribute('data-lane'))
  expect(lanes).toEqual(['local'])
})

test('limit-event history and the per-person / per-period cost views render at their own grains', async () => {
  const view = await fleet()
  const text = view.container.textContent ?? ''
  expect(text).toContain('Limit events')
  expect(text).toContain('Per person')
  expect(text).toContain('Per period')
  // Served at the view's own grain: the burn-rate answer's own columns.
  expect(text).toContain('usd_per_day')
})

test('the local seat and GPU seams render as the NAMED absences they are', async () => {
  const view = await fleet()
  const text = view.container.textContent ?? ''
  expect(text).toContain('Local duty seats')
  expect(text).toContain('GPU / VRAM')
  // The distinction that matters: an unwired instrument, not an idle tier.
  expect(text).toContain('not wired at v0')
  expect(text, 'an unwired seat count was rendered as a reading').not.toMatch(/0 seats|0 GB/)
})

test('an unreadable meters surface leaves the fleet honest rather than empty-looking', async () => {
  const view = await fleet({ 'GET /api/meters': { status: 503, body: { error: 'not_wired', detail: 'no meters here' } } })
  expect(view.container.textContent).toContain('no meters here')
  // The seams still render: they are facts about this build, not about the read.
  expect(view.container.textContent).toContain('Local duty seats')
})

// ── the 3.3 pause switch and the S10.4 budget editor (P3-UI-2 R9–R16) ─────
//
// The scripted VERB responses below are transcribed from their Go types with
// the source cited beside each: `api.AutomationPause`
// (internal/api/meters_verbs.go:241–249), `api.BudgetDeclared` + `BudgetRecord`
// (:111–116 / :34–45), and the served refusal texts from the handler's own
// branches (:152–165, :232–238). A body that drifts from the handler is a probe
// that proves nothing (§42's root-cause lesson), so none of them is invented.

const asMember = (id: string) => ({
  body: { authenticated: true, user: { user_id: id, display_name: id, role: 'member', pin_set: true } },
})

/** The verb's own served sentences — meters_verbs.go:293–298. They carry the
 *  P-T08-4 story, so the surface renders them verbatim and writes none of its
 *  own. */
const pausedDetail =
  'paused: no background work is admitted for this person, and an in-flight run parks at its next stage-session ' +
  'boundary. Nothing queued or parked is discarded, and interactive work is not gated at all (S10.4)'
const resumedDetail =
  'resumed: background admission is open again and the preserved queue proceeds. A run that parked while paused is ' +
  'released by its own resume (S14.4)'

/** meters_verbs.go:189–190, with the fixture world's own lane. */
const declaredDetail =
  "declared: the S10.4 gauge measures this person's anthropic consumption against it immediately, in " +
  'weighted-consumption units (S10.4)'

test('the pause switch renders the SERVED position, per person, for whoever may act', async () => {
  const view = await fleet()
  const switches = [...view.container.querySelectorAll('.automation-switch')]
  // The operator is served the household's switches, so the operator gets the
  // administer path without this surface ever guessing at a person.
  expect(switches.map((s) => s.getAttribute('data-owner'))).toEqual(['op', 'alice', 'bob'])
  // The position is the one the READ served — bob's switch was flipped through
  // the real verb when the fixture world was seeded.
  const served = (fixtures.meters() as unknown as Meters).automation.states ?? []
  expect(served.find((s) => s.owner === 'bob')?.paused).toBe(true)
  expect(switches[2].getAttribute('data-paused')).toBe('true')
  expect(switches[2].textContent).toContain('background work is paused')
  expect(switches[1].getAttribute('data-paused')).toBe('false')
  expect(switches[1].textContent).toContain('background work is running')
})

test('a member is offered their own switch and nobody else’s', async () => {
  // The served body IS a member's: the same read, taken as alice.
  const view = await fleet({
    'GET /api/auth/session': asMember('alice'),
    'GET /api/meters': { body: fixtures.metersMember() },
  })
  const switches = [...view.container.querySelectorAll('.automation-switch')]
  expect(switches.map((s) => s.getAttribute('data-owner'))).toEqual(['alice'])
  const rows = [...view.container.querySelectorAll('.budget-row')].map((r) => r.getAttribute('data-owner'))
  expect(new Set(rows), 'a member was offered another owner’s budget editor').toEqual(new Set(['alice']))
})

test('and a member handed a household body is STILL offered nobody else’s controls', async () => {
  // The sabotage direction: the server never serves this body to a member, and
  // if it ever did, the control would still not appear — the authority rule the
  // verbs enforce (own + operator-any) is the one this surface renders by.
  const view = await fleet({ 'GET /api/auth/session': asMember('alice'), 'GET /api/meters': { body: fixtures.meters() } })
  const owners = [...view.container.querySelectorAll('.automation-switch')].map((s) => s.getAttribute('data-owner'))
  expect(owners).toEqual(['alice'])
  const budgets = [...view.container.querySelectorAll('.budget-row')].map((r) => r.getAttribute('data-owner'))
  expect(new Set(budgets)).toEqual(new Set(['alice']))
  // The rows themselves still RENDER for every owner — narrowing a control is
  // not narrowing a reading (D2/S3.10).
  expect([...view.container.querySelectorAll('.fleet-lanes tbody tr')].length).toBeGreaterThan(budgets.length)
})

test('the switch is two position verbs and never a toggle, and it posts the position it names', async () => {
  const view = await fleet()
  const alice = view.container.querySelector('.automation-switch[data-owner="alice"]')!
  // BOTH buttons exist on BOTH positions: the verb refuses a request that did
  // not say which way it wanted the switch, so this control never sends one.
  expect(alice.querySelector('[data-pause="true"]')).not.toBeNull()
  expect(alice.querySelector('[data-pause="false"]')).not.toBeNull()
  const bob = view.container.querySelector('.automation-switch[data-owner="bob"]')!
  expect(bob.querySelector('[data-pause="true"]')).not.toBeNull()
  expect(bob.querySelector('[data-pause="false"]')).not.toBeNull()
})

test('pausing renders the served preservation sentence verbatim, and re-reads', async () => {
  const log = scriptedFetch({
    ...oversightRoutes(),
    'POST /api/meters/pause': {
      body: { owner: 'alice', paused: true, changed: true, detail: pausedDetail },
    },
  })
  window.history.replaceState(null, '', '/fleet')
  const view = mount(<App stream={inertStream()} />)
  await flush()
  const readsBefore = log.calls.filter((c) => c.path === '/api/meters').length

  click(view.container.querySelector('.automation-switch[data-owner="alice"] [data-pause="true"]'))
  await flush()

  const sent = log.calls.filter((c) => c.path === '/api/meters/pause')
  expect(sent.length).toBe(1)
  expect(sent[0].body, 'the switch guessed at a subject or a position').toEqual({ person: 'alice', paused: true })

  const line = view.container.querySelector('.automation-switch[data-owner="alice"] [data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('applied')
  // P-T08-4, in the platform's own words, on the screen.
  expect(line.textContent).toContain('Nothing queued or parked is discarded')
  expect(line.textContent).toContain('interactive work is not gated at all')
  expect(line.textContent).toContain(pausedDetail)
  expect(log.calls.filter((c) => c.path === '/api/meters').length, 'the act did not re-read the meters').toBe(
    readsBefore + 1,
  )
  view.unmount()
})

test('resuming carries the release story, and an already-there flip says so honestly', async () => {
  const log = scriptedFetch({
    ...oversightRoutes(),
    'POST /api/meters/pause': {
      body: { owner: 'bob', paused: false, changed: true, detail: resumedDetail },
    },
  })
  window.history.replaceState(null, '', '/fleet')
  const view = mount(<App stream={inertStream()} />)
  await flush()

  const bob = () => view.container.querySelector('.automation-switch[data-owner="bob"]')!
  click(bob().querySelector('[data-pause="false"]'))
  await flush()
  expect(log.calls.filter((c) => c.path === '/api/meters/pause')[0].body).toEqual({ person: 'bob', paused: false })
  expect(bob().querySelector('[data-outcome]')?.textContent).toContain(resumedDetail)
  // The run-level release lives on the card that is holding it: this points at
  // the inbox and re-implements nothing.
  expect(bob().querySelector('a')?.getAttribute('href')).toBe('/inbox')

  // The honest repeat: the switch was already there, so nothing changed.
  log.set('POST /api/meters/pause', {
    body: { owner: 'bob', paused: false, changed: false, detail: resumedDetail },
  })
  click(bob().querySelector('[data-pause="false"]'))
  await flush()
  const line = bob().querySelector('[data-outcome]')!
  expect(line.getAttribute('data-outcome'), 'an unchanged switch claimed to have moved').toBe('noop')
  expect(line.className).not.toContain('error')
  expect(line.textContent).toContain('it was already there')
  view.unmount()
})

test('a refused flip renders the server’s own code and sentence', async () => {
  for (const leg of [
    // meters_verbs.go:277–279 / :457–469 / :253–255.
    { status: 403, code: 'forbidden', detail: "pausing another person's automation is the operator's (D10)" },
    { status: 404, code: 'not_found', detail: 'no such person "ghost": a budget or a pause is declared for somebody' },
    { status: 503, code: 'not_wired', detail: 'the S10.4 pause switch is not wired in this process' },
  ]) {
    const view = await fleet({ 'POST /api/meters/pause': { status: leg.status, body: { error: leg.code, detail: leg.detail } } })
    click(view.container.querySelector('.automation-switch[data-owner="alice"] [data-pause="true"]'))
    await flush()
    const line = view.container.querySelector('.automation-switch[data-owner="alice"] [data-outcome]')!
    expect(line.getAttribute('data-outcome')).toBe('failed')
    expect(line.textContent).toContain(leg.code)
    expect(line.textContent).toContain(leg.detail)
    view.unmount()
  }
})

test('the pause explainer says what the switch does before anybody presses it', async () => {
  const view = await fleet()
  const said = view.container.querySelector('.automation-explainer')?.textContent ?? ''
  expect(said).toContain('stops the platform from starting any new background work')
  expect(said).toContain('parks itself at its next safe boundary')
  expect(said).toContain('Nothing queued and nothing parked is thrown away')
  expect(said).toContain('Talking to the platform yourself is never held back')
})

test('an unreadable switch position renders the served absence and offers no control', async () => {
  const meters = fixtures.meters() as unknown as Meters
  meters.automation = { absent: 'the S10.4 pause switch is not wired in this process, so its current position cannot be read here' }
  const view = await fleet({ 'GET /api/meters': { body: meters } })
  expect(view.container.textContent).toContain('its current position cannot be read here')
  expect(
    view.container.querySelector('.automation-switch'),
    'a switch was offered over a position the platform cannot see',
  ).toBeNull()
})

// ── the budget editor ─────────────────────────────────────────────────────

test('the undeclared state is the landed one, and the editor changes nothing about it', async () => {
  const view = await fleet()
  const served = (fixtures.meters() as unknown as Meters).lanes
  expect(served.every((l) => !l.budget_declared), 'the fixture already has a declared budget').toBe(true)
  const text = view.container.textContent ?? ''
  // Both landed absence texts, byte-identical (MissionControl.tsx:205/212).
  expect(text).toContain('no budget declared')
  expect(text).toContain('no declared budget, so there is no denominator')
  // And the editor is offered per served (owner, lane) row.
  const rows = [...view.container.querySelectorAll('.budget-row')].map((r) => `${r.getAttribute('data-owner') ?? ''}/${r.getAttribute('data-lane') ?? ''}`)
  expect(rows).toEqual(served.map((l) => `${l.owner}/${l.lane}`))
  expect(view.container.querySelector('[data-declare-budget="alice/anthropic"]')?.textContent).toBe('Declare a budget')
})

/** open the editor for one row and fill the two required fields. */
async function openEditor(view: { container: HTMLElement }, row: string, tokens: string, days: string) {
  click(view.container.querySelector(`[data-declare-budget="${row}"]`))
  await flush()
  typeInto(document.querySelector('[data-field="period_tokens"]') as HTMLInputElement, tokens)
  typeInto(document.querySelector('[data-field="period_days"]') as HTMLInputElement, days)
  await flush()
}

test('declaring sends the wire body, renders the served answer, and re-reads the gauge', async () => {
  const declared = {
    // api.BudgetRecord — internal/api/meters_verbs.go:34–45.
    budget: {
      owner: 'alice',
      lane: 'anthropic',
      period_tokens: 250000,
      unit: 'weighted-consumption units (S10.4)',
      period_start: '2026-08-05T00:00:00Z',
      period_days: 30,
      declared_ts: '2026-08-05T00:00:00Z',
      declared_by: 'alice',
    },
    detail: declaredDetail,
  }
  const log = scriptedFetch({ ...oversightRoutes(), 'POST /api/meters/budget': { body: declared } })
  window.history.replaceState(null, '', '/fleet')
  const view = mount(<App stream={inertStream()} />)
  await flush()
  const readsBefore = log.calls.filter((c) => c.path === '/api/meters').length

  await openEditor(view, 'alice/anthropic', '250000', '30')
  // Nothing has fired yet: the editor is a form, and a form is not an act.
  expect(log.calls.filter((c) => c.path === '/api/meters/budget').length).toBe(0)
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  const sent = log.calls.filter((c) => c.path === '/api/meters/budget')
  expect(sent.length).toBe(1)
  expect(sent[0].body).toEqual({ person: 'alice', lane: 'anthropic', period_tokens: 250000, period_days: 30 })

  const line = view.container.querySelector('.budget-row[data-lane="anthropic"] [data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('applied')
  // A FIRST declaration has no `prior`, and the absence is what says so.
  expect(line.textContent).toContain('there was no budget on this lane before')
  expect(line.textContent).toContain(declaredDetail)
  expect(log.calls.filter((c) => c.path === '/api/meters').length, 'the declaration did not re-read the gauge').toBe(
    readsBefore + 1,
  )
  view.unmount()
})

test('a replacement renders old→new from the served prior, and the re-read gauge is what shows the budget', async () => {
  const prior = {
    owner: 'alice',
    lane: 'anthropic',
    period_tokens: 100000,
    unit: 'weighted-consumption units (S10.4)',
    period_start: '2026-07-01T00:00:00Z',
    period_days: 30,
    declared_ts: '2026-07-01T00:00:00Z',
    declared_by: 'alice',
  }
  // The POST-declaration READ: the gauge reports the declared budget and a real
  // pressure figure. §39-B drain D2 is what makes the two halves of this body
  // agree — internal/shell's HTTP-level test declares through the real verb and
  // requires exactly that, over the real gauge this world's fixture meter
  // stands in for.
  const after = fixtures.meters() as unknown as Meters
  const lane = after.lanes.find((l) => l.owner === 'alice' && l.lane === 'anthropic')!
  lane.budget_declared = true
  lane.pressure_applicable = true
  lane.pressure = 0.05
  lane.budget_remaining = 249987.5

  const log = scriptedFetch({
    ...oversightRoutes(),
    'POST /api/meters/budget': {
      body: { budget: { ...prior, period_tokens: 250000 }, prior, detail: declaredDetail },
    },
  })
  window.history.replaceState(null, '', '/fleet')
  const view = mount(<App stream={inertStream()} />)
  await flush()

  await openEditor(view, 'alice/anthropic', '250000', '30')
  log.set('GET /api/meters', { body: after })
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  const line = view.container.querySelector('.budget-row[data-lane="anthropic"] [data-outcome]')!
  expect(line.textContent).toContain('replacing 100000 weighted-consumption units (S10.4)')
  // The FIGURES come from the re-read, never from the editor: the gauge row now
  // reports the declared budget, its pressure and its remainder. The remainder
  // renders through the RA-7 gauge formatter (group separators, one decimal) —
  // a reading for eyes, in the unit the header now names.
  const text = view.container.textContent ?? ''
  expect(text).toContain('249,987.5')
  expect(text).toContain('0.05')
  // The label the editor offers is the served declaration state, re-read.
  expect(view.container.querySelector('[data-declare-budget="alice/anthropic"]')?.textContent).toBe(
    'Change this budget',
  )
  view.unmount()
})

test('the budget refusal renders the server’s own text, pause pointer and all', async () => {
  // meters_verbs.go:157–161 — the refusal is load-bearing text and is served,
  // never re-typed here as a client message.
  const refusal =
    'bad "period_tokens" -5: a budget is a positive figure in weighted-consumption units (S10.4). ' +
    'To stop automation entirely, pause it (POST /api/meters/pause)'
  const view = await fleet({
    'POST /api/meters/budget': { status: 400, body: { error: 'bad_request', detail: refusal } },
  })
  await openEditor(view, 'alice/anthropic', '250000', '30')
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  const line = view.container.querySelector('.budget-row[data-lane="anthropic"] [data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('failed')
  expect(line.textContent, 'the server’s own refusal was replaced by a client message').toContain(refusal)
})

test('client pre-validation holds the act back without ever answering for the server', async () => {
  const view = await fleet()
  click(view.container.querySelector('[data-declare-budget="alice/anthropic"]'))
  await flush()
  // Empty, then non-positive: the verb's own bound, mirrored so an
  // obviously-refused request does not need a round trip.
  expect((document.querySelector('[data-act="confirm"]') as HTMLButtonElement).disabled).toBe(true)
  typeInto(document.querySelector('[data-field="period_tokens"]') as HTMLInputElement, '0')
  typeInto(document.querySelector('[data-field="period_days"]') as HTMLInputElement, '30')
  await flush()
  expect((document.querySelector('[data-act="confirm"]') as HTMLButtonElement).disabled).toBe(true)
  typeInto(document.querySelector('[data-field="period_tokens"]') as HTMLInputElement, '10')
  await flush()
  expect((document.querySelector('[data-act="confirm"]') as HTMLButtonElement).disabled).toBe(false)
  // And no message of this client's own has replaced the server's answer,
  // because nothing fired.
  expect(view.container.querySelector('.budget-row [data-outcome]')).toBeNull()
})

test('the budget figure is never money, and the unit comes off the wire', async () => {
  const view = await fleet()
  click(view.container.querySelector('[data-declare-budget="alice/anthropic"]'))
  await flush()
  const editor = document.querySelector('[role="dialog"]')!
  const said = editor.textContent ?? ''
  for (const token of ['$', 'usd', 'dollar', '€', '£', 'cost']) {
    expect(said.toLowerCase(), `the budget editor rendered "${token}"`).not.toContain(token)
  }
  expect(said).toContain('weighted-consumption units')
  expect(said).toContain('not money')
  // Nothing about the platform's own threshold is turned into a figure here.
  for (const banned of ['%', 'percent', 'eta']) {
    expect(said.toLowerCase(), `the budget editor rendered "${banned}"`).not.toContain(banned)
  }

  // The FIELD's unit label is the SERVED string, and before any declaration
  // there is none — so the field carries no unit rather than a made-up one.
  const label = editor.querySelector('label')?.textContent ?? ''
  expect(label).toBe('How much')
  const declaredBody = fixtures.meters() as unknown as Meters
  const at = declaredBody.budgets.answer?.columns.indexOf('budget_unit') ?? -1
  expect(at, 'the budgets view no longer carries budget_unit').toBeGreaterThan(-1)
  expect(declaredBody.budgets.answer?.rows.every((r) => (r as unknown[])[at] === null)).toBe(true)
})

test('a served unit label is rendered verbatim on the field', async () => {
  const meters = fixtures.meters() as unknown as Meters
  const at = meters.budgets.answer!.columns.indexOf('budget_unit')
  ;(meters.budgets.answer!.rows[0] as unknown[])[at] = 'weighted-consumption units (S10.4)'
  const view = await fleet({ 'GET /api/meters': { body: meters } })
  click(view.container.querySelector('[data-declare-budget="alice/anthropic"]'))
  await flush()
  expect(document.querySelector('[role="dialog"] label')?.textContent).toBe(
    'How much (weighted-consumption units (S10.4))',
  )
})

test('a decision.recorded frame re-reads the meters, so an act lands where decisions render', async () => {
  const log = scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', '/fleet')
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

  const before = log.calls.filter((c) => c.path === '/api/meters').length
  act(() =>
    FakeSource.last().send('decision.recorded', {
      seq: 960,
      user_id: 'alice',
      type: 'decision.recorded',
      schema_version: 1,
      topics: ['board'],
      payload: {},
      ts: '2026-08-05T00:00:00Z',
    }),
  )
  await flush()
  // Both meters verbs mint `decision.recorded` (§39-B), which this surface
  // already subscribes to — so the acts land with no live.ts change at all.
  expect(log.calls.filter((c) => c.path === '/api/meters').length, 'a decision frame did not re-read the meters').toBe(
    before + 1,
  )
  view.unmount()
})

test('every new control is reachable and operable at phone width', async () => {
  // jsdom has no layout engine, so what is checkable is the STRUCTURE that
  // makes a phone-width surface work (the §41-B method, and the sweep's own
  // phone-complete spirit): the control is present, and nothing in or around it
  // pins a pixel width that a 375px viewport would have to scroll sideways past.
  const view = await fleet()
  expect(view.container.querySelector('.automation-switch [data-pause="true"]')).not.toBeNull()
  expect(view.container.querySelector('.automation-switch [data-pause="false"]')).not.toBeNull()
  expect(view.container.querySelector('[data-declare-budget]')).not.toBeNull()

  click(view.container.querySelector('[data-declare-budget="alice/anthropic"]'))
  await flush()
  const dialog = document.querySelector('[role="dialog"]')!
  expect(dialog.querySelector('[data-field="period_tokens"]')).not.toBeNull()
  expect(dialog.querySelector('[data-act="confirm"]')).not.toBeNull()

  const pinned = /w-\[\d+px\]|min-w-\[\d+px\]/
  const scope = [
    ...view.container.querySelectorAll('.automation-switches *, .budget-rows *'),
    ...dialog.querySelectorAll('*'),
    dialog,
  ]
  expect(scope.length).toBeGreaterThan(10)
  const fixed = scope.filter((n) => pinned.test(n.className.toString()) || /\d+px/.test(n.getAttribute('style') ?? ''))
  expect(fixed.map((n) => n.className.toString()), 'a control pins a pixel width a phone cannot fit').toEqual([])
  // Probe: the detector really matches what it forbids.
  expect(pinned.test('w-[520px] flex')).toBe(true)
})

// ── drain r1: both authority directions FIRED, not only rendered (D4) ─────

test('a member flips their OWN switch, and the request names them', async () => {
  const log = scriptedFetch({
    ...oversightRoutes(),
    'GET /api/auth/session': asMember('alice'),
    'GET /api/meters': { body: fixtures.metersMember() },
    'POST /api/meters/pause': { body: { owner: 'alice', paused: true, changed: true, detail: pausedDetail } },
  })
  window.history.replaceState(null, '', '/fleet')
  const view = mount(<App stream={inertStream()} />)
  await flush()

  click(view.container.querySelector('.automation-switch[data-owner="alice"] [data-pause="true"]'))
  await flush()
  const sent = log.calls.filter((c) => c.path === '/api/meters/pause')
  expect(sent.length, 'a member could not flip their own switch').toBe(1)
  expect(sent[0].body).toEqual({ person: 'alice', paused: true })
  expect(view.container.querySelector('.automation-switch[data-owner="alice"] [data-outcome]')?.getAttribute('data-outcome')).toBe('applied')
  view.unmount()
})

test('a member declares their OWN budget, and the operator declares another owner’s', async () => {
  const answer = (owner: string, lane: string) => ({
    body: {
      budget: {
        owner,
        lane,
        period_tokens: 250000,
        unit: 'weighted-consumption units (S10.4)',
        period_start: '2026-08-05T00:00:00Z',
        period_days: 30,
        declared_ts: '2026-08-05T00:00:00Z',
        declared_by: owner,
      },
      detail: declaredDetail,
    },
  })

  // (a) the member, on their own lane — the own limb of own+operator-any.
  const memberLog = scriptedFetch({
    ...oversightRoutes(),
    'GET /api/auth/session': asMember('alice'),
    'GET /api/meters': { body: fixtures.metersMember() },
    'POST /api/meters/budget': answer('alice', 'anthropic'),
  })
  window.history.replaceState(null, '', '/fleet')
  const mine = mount(<App stream={inertStream()} />)
  await flush()
  await openEditor(mine, 'alice/anthropic', '250000', '30')
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  const asMine = memberLog.calls.filter((c) => c.path === '/api/meters/budget')
  expect(asMine.length, 'a member could not declare their own budget').toBe(1)
  expect(asMine[0].body).toEqual({ person: 'alice', lane: 'anthropic', period_tokens: 250000, period_days: 30 })
  mine.unmount()

  // (b) the operator, on BOB's lane — the administer limb, fired.
  const opLog = scriptedFetch({ ...oversightRoutes(), 'POST /api/meters/budget': answer('bob', 'zai') })
  window.history.replaceState(null, '', '/fleet')
  const theirs = mount(<App stream={inertStream()} />)
  await flush()
  await openEditor(theirs, 'bob/zai', '9000', '7')
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  const asOp = opLog.calls.filter((c) => c.path === '/api/meters/budget')
  expect(asOp.length, 'the operator could not declare another owner’s budget').toBe(1)
  expect(asOp[0].body).toEqual({ person: 'bob', lane: 'zai', period_tokens: 9000, period_days: 7 })
  expect(theirs.container.querySelector('.budget-row[data-owner="bob"] [data-outcome]')?.getAttribute('data-outcome')).toBe('applied')
  theirs.unmount()
})

// ── drain r1: the figure is read exactly as typed, or refused (D5/D6) ─────

test('a figure that is not a whole number is refused with its reason, and nothing fires', async () => {
  for (const typed of ['3.5', '3e5']) {
    const log = scriptedFetch(oversightRoutes())
    window.history.replaceState(null, '', '/fleet')
    const view = mount(<App stream={inertStream()} />)
    await flush()
    click(view.container.querySelector('[data-declare-budget="alice/anthropic"]'))
    await flush()
    typeInto(document.querySelector('[data-field="period_tokens"]') as HTMLInputElement, typed)
    typeInto(document.querySelector('[data-field="period_days"]') as HTMLInputElement, '30')
    await flush()

    // `Number.parseInt` reads both of these as 3 — a DIFFERENT figure from the
    // one that was typed. Firing that silently is the one thing a budget field
    // must not do, so the act is held and the reason is on screen.
    // Non-vacuous: the field really holds what was typed, so the refusal is
    // about the VALUE rather than about an input the harness never filled.
    expect((document.querySelector('[data-field="period_tokens"]') as HTMLInputElement).value).toBe(typed)
    const held = document.querySelector('[data-held]')
    expect(held, `"${typed}" was accepted instead of refused`).not.toBeNull()
    expect(held?.textContent).toContain('whole number of units')
    expect((document.querySelector('[data-act="confirm"]') as HTMLButtonElement).disabled).toBe(true)
    expect(log.calls.filter((c) => c.path === '/api/meters/budget').length, `"${typed}" fired something`).toBe(0)
    view.unmount()
  }
})

test('served and typed figures take the mono, tabular treatment the token contract gives numbers', async () => {
  const prior = {
    owner: 'alice',
    lane: 'anthropic',
    period_tokens: 100000,
    unit: 'weighted-consumption units (S10.4)',
    period_start: '2026-07-01T00:00:00Z',
    period_days: 30,
    declared_ts: '2026-07-01T00:00:00Z',
    declared_by: 'alice',
  }
  const view = await fleet({
    'POST /api/meters/budget': {
      body: { budget: { ...prior, period_tokens: 250000 }, prior, detail: declaredDetail },
    },
  })
  await openEditor(view, 'alice/anthropic', '250000', '30')
  for (const field of ['period_tokens', 'period_days']) {
    const input = document.querySelector(`[data-field="${field}"]`)!
    expect(input.className, `${field} does not render its figure mono`).toContain('font-mono')
    expect(input.className).toContain('tabular-nums')
  }
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  const figure = view.container.querySelector('.budget-row[data-lane="anthropic"] [data-outcome] .font-mono')!
  expect(figure.textContent, 'the served prior figure is not the one rendered mono').toBe('100000')
  expect(figure.className).toContain('tabular-nums')
  // The sentence still reads as one sentence.
  expect(view.container.querySelector('.budget-row[data-lane="anthropic"] [data-outcome]')?.textContent).toContain(
    'replacing 100000 weighted-consumption units (S10.4)',
  )
})

// ── D5 applied: the lane table's park horizon (P3-UI-4) ───────────────────

test('a lane with parked runs and no served horizon says so; one with a horizon renders it live', async () => {
  // The served world's own arm first: lanes carry parked runs and NO horizon,
  // so the line says the platform was not told for how long rather than
  // inventing a time for a relative label to be about. That arm is byte-kept
  // through the migration.
  const served = fixtures.meters() as unknown as Meters
  const lane = served.lanes.find((l) => l.parked_runs > 0)!
  expect(lane, 'no lane in the fixture world has a parked run').toBeDefined()
  expect(lane.parked_until ?? null, 'the fixture lane now carries a horizon').toBeNull()
  const view = await fleet()
  expect([...view.container.querySelectorAll('.absent')].map((n) => n.textContent)).toContain(
    'parked, no horizon given',
  )
  view.unmount()

  // And the other arm, driven: with a horizon served, the instant renders
  // verbatim with the relative label BESIDE it — never in place of it.
  const withHorizon = fixtures.meters() as unknown as Meters
  const stamp = '2026-07-20T12:00:00Z'
  withHorizon.lanes = withHorizon.lanes.map((l) => (l.parked_runs > 0 ? { ...l, parked_until: stamp } : l))
  const v2 = await fleet({ 'GET /api/meters': { body: withHorizon } })
  const time = v2.container.querySelector('.parked-until time')!
  expect(time, 'the served horizon rendered no instant').not.toBeNull()
  expect(time.getAttribute('dateTime')).toBe(stamp)
  expect(time.textContent, 'the verbatim UTC was dropped').toBe(stamp)
  const beside = time.parentElement!.parentElement!.textContent ?? ''
  expect(beside.endsWith(stamp), 'the instant is not beside its label').toBe(true)
  expect(beside.length, 'a relative label replaced the instant').toBeGreaterThan(stamp.length)
  v2.unmount()
})

// ── the session-read hoist (P3-UI-5; closes the §48 recorded deviation) ────

test('the fleet reads no session of its own — one read, one source', async () => {
  // THE DEVIATION THIS CLOSES. `useMayAct` used to call `GET /api/auth/session`
  // from this surface because `App.tsx` was frozen for P3-UI-2. It was a second
  // READ of one source, never a second source, and the shell now hands the
  // identity down exactly as it does to Board, MissionControl and Deliverable.
  const log = scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', '/fleet')
  const view = mount(<App stream={inertStream()} />)
  await flush()

  const reads = log.calls.filter((c) => c.path === '/api/auth/session')
  expect(reads.length, 'the fleet route reads the session more than once').toBe(1)
  // Non-vacuous: the surface really rendered, and it really rendered controls
  // that DEPEND on the identity — so "one read" is not "the page never loaded".
  expect(view.container.querySelector('.fleet-lanes'), 'the fleet did not render').not.toBeNull()
  expect(view.container.querySelector('[data-pause]'), 'no identity-dependent control rendered').not.toBeNull()
  view.unmount()

  // And the source says so too: the DOM cannot see a read that a render happened
  // not to trigger, so the file is scanned for the call it no longer makes.
  expect(fleetSource, 'Fleet.tsx still reads the session itself').not.toMatch(/api\s*\.\s*session\s*\(/)
  expect(fleetSource, 'the scan does not reach a file that calls the API at all').toMatch(/api\s*\.\s*meters\s*\(/)
  // Probe: the matcher really discriminates.
  expect(/api\s*\.\s*session\s*\(/.test('void api.session().then(')).toBe(true)
})

test('the hoisted operator predicate is dev-inclusive, and is NOT the memory surface’s role-only flag', async () => {
  // The two predicates DIFFER and must stay different: this one mirrors the
  // server's own `isOperatorRead` — the predicate the read on the other side of
  // these verbs used — while the S09 gate resolves its actor against the users
  // row alone. Collapsing them would invent an authority in one direction and a
  // refusal in the other (the B6-6 D9 lesson).
  expect(appSource, 'the fleet arm no longer states the dev-inclusive predicate').toContain(
    "operator={session.dev === true || session.user?.role === 'operator'}",
  )
  expect(appSource, 'the memory arm no longer states the users-row-role-only flag').toContain(
    "operator={session.user?.role === 'operator'}",
  )

  // Driven, not only read: under the dev fallback there is no `user` object at
  // all, so a role-only predicate would offer NOTHING. The dev identity is
  // offered every switch the read serves, which is byte-equivalent to what the
  // surface's own read produced before the hoist.
  const view = await fleet({ 'GET /api/auth/session': { body: { authenticated: true, dev: true } } })
  const served = (fixtures.meters() as unknown as Meters).automation.states ?? []
  expect(served.length, 'the served body carries no switch, so this asserts nothing').toBeGreaterThan(1)
  expect(view.container.querySelectorAll('.automation-switch')).toHaveLength(served.length)
  view.unmount()
})

test('a member handed the household body is still offered only their own, through the hoisted props', async () => {
  // The sabotage direction, re-fired through the new plumbing rather than
  // merely re-rendered: the served body is the OPERATOR's, and the identity is
  // a member's.
  const view = await fleet({ 'GET /api/auth/session': asMember('alice') })
  const switches = [...view.container.querySelectorAll('.automation-switch')].map((n) => n.getAttribute('data-owner'))
  expect(switches, 'a member was offered somebody else’s switch').toEqual(['alice'])
  const budgets = [...view.container.querySelectorAll('.budget-row')].map((n) => n.getAttribute('data-owner'))
  expect(new Set(budgets), 'a member was offered somebody else’s budget editor').toEqual(new Set(['alice']))
  // Non-vacuous: the body really carried other people's rows.
  const served = fixtures.meters() as unknown as Meters
  expect(new Set((served.automation.states ?? []).map((s) => s.owner)).size).toBeGreaterThan(1)
  view.unmount()
})

// ── the D8 layer on the fleet (P3-UI-5) ───────────────────────────────────

test('the fleet teaches what it is, and the line never implies a household total', async () => {
  const view = await fleet()
  const line = view.container.querySelector('[data-surface-what]')?.textContent ?? ''
  expect(line, 'the fleet carries no "what this is" line').not.toBe('')
  expect(line.toLowerCase()).toContain('whose account')
  // Accounts are never summed (D2), so the line must not promise one.
  expect(line.toLowerCase(), 'the header line implies a total this platform never computes').toContain(
    'never added up across people',
  )
  for (const overclaim of ['total spend', 'household total', 'combined']) {
    expect(line.toLowerCase()).not.toContain(overclaim)
  }
  view.unmount()
})

test('parked lanes say so with the horizon and the inbox door, and the banner fires nothing', async () => {
  const log = scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', '/fleet')
  const view = mount(<App stream={inertStream()} />)
  await flush()

  const served = fixtures.meters() as unknown as Meters
  expect(served.lanes.some((l) => l.parked_runs > 0), 'no served lane is parked — this asserts nothing').toBe(true)
  const banner = view.container.querySelector('[data-stall="parked"]')!
  expect(banner, 'a lane holding parked runs stalls silently').not.toBeNull()
  expect(banner.textContent).toContain('nothing queued or parked is thrown away')
  // The door POINTS: a parked run's own release is `api.resumeRun`, and its one
  // call site is the inbox, where the card holding it lives (§48 OQ2).
  expect(banner.querySelector('a')?.getAttribute('href')).toBe('/inbox')
  expect(banner.querySelector('button'), 'the banner offers a verb of its own').toBeNull()
  // NO NEW VERB. A door is a link and nothing else: the banner carries no
  // control of its own, and rendering the whole surface mutated nothing.
  expect(banner.querySelectorAll('button, input, form, select')).toHaveLength(0)
  expect(log.calls.filter((c) => c.method !== 'GET'), 'an oversight surface fired a verb on render').toEqual([])
  view.unmount()
})

test('api.resumeRun still has exactly one call site, and it is the inbox', () => {
  // The never-stall doors POINT at the release rather than re-implementing it.
  // If a second surface ever fires it, the "one verb, one home" claim above
  // silently stops being true.
  const callers = Object.entries(appSources)
    .filter(([p]) => !p.includes('.test.'))
    .filter(([, src]) => /api\s*\.\s*resumeRun\s*\(/.test(src))
    .map(([p]) => p)
    .sort()
  expect(callers, 'a second surface re-implements the parked-run release').toEqual(['./Inbox.tsx'])
  // Non-vacuous: the scan really reached the tree and the verb really exists.
  expect(Object.keys(appSources).length).toBeGreaterThan(20)
  expect(appSources['./api.ts'], 'the verb is gone — this list would be empty for the wrong reason').toMatch(
    /resumeRun/,
  )
})

test('the fleet’s empty arms teach what would fill them', async () => {
  const empty = fixtures.meters() as unknown as Meters
  empty.lanes = []
  const view = await fleet({ 'GET /api/meters': { body: empty } })
  const text = view.container.textContent ?? ''
  expect(text).toContain('No lane matches this filter.')
  expect(text, 'the empty lane state does not teach what fills it').toContain('A lane appears once work has run on it')
  expect(text).toContain('No lane here is yours to put a budget on.')
  expect(text, 'the empty budget state does not teach the grain a budget is declared at').toContain(
    'declared per person per lane',
  )
  view.unmount()
})

test('NONE-VS-NOT-LOADED: a pending meters read shows the catching-up marker, never a teaching empty', async () => {
  // The session LANDS and the fleet's own read never answers (drain r1, D1).
  // "No lane matches this filter" over a pending read is a statement about the
  // filter that the platform has not yet had the data to make.
  const view = await fleet({ 'GET /api/meters': { pending: true } })
  expect(view.container.querySelector('[data-surface-what]'), 'no surface mounted, so this proves nothing').not.toBeNull()
  expect(view.container.textContent, 'the loading affordance is missing').toContain('Catching up')
  expect(view.container.textContent, 'the no-lane-match empty taught over a pending read').not.toContain(
    'No lane matches this filter.',
  )
  view.unmount()

  // …and a served empty lane list teaches, so the gate is not simply always off.
  const none = fixtures.meters() as unknown as Meters
  none.lanes = []
  const landed = await fleet({ 'GET /api/meters': { body: none } })
  expect(landed.container.textContent).toContain('No lane matches this filter.')
  landed.unmount()
})
