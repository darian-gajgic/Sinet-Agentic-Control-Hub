import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import type { WorkerRow, Workforce as WorkforceData } from './api'
import { FakeSource, fixtures, frame, oversightRoutes, scriptedFetch, workforceRoutes } from './doubles'
import { EventStream } from './events'
import { workforceEventTypes } from './live'
import { hrefFor, matchRoute, routes } from './routes'
import { flush, mount } from './testing'

/**
 * The workforce map (Spec S15.10; S08.1/S08.2/S08.4/S08.7/S08.8/S08.9), driven
 * against the two golden bodies the real handler serves.
 *
 * The two bodies are not two renders of one answer. `workforce` is the
 * OPERATOR's reading — the whole registry, including another member's personal
 * automation, with every owner's runs — and `workforceMember` is alice's: her
 * own draft plus the household roster, her own runs, and bob's personal worker
 * absent entirely. That absence is a property of the SERVED body, which is why
 * the member limb is driven from its own committed file rather than by hiding
 * rows in the view.
 */

const scheduled = vi.fn(() => 0)

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: scheduled,
    cancel: () => {},
  })

async function open(body?: Record<string, unknown>, extra: Record<string, { body?: unknown; status?: number }> = {}) {
  const log = scriptedFetch({ ...workforceRoutes(body ?? fixtures.workforce()), ...extra })
  window.history.replaceState(null, '', hrefFor('workforce'))
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}

const served = () => fixtures.workforce() as unknown as WorkforceData
const servedMember = () => fixtures.workforceMember() as unknown as WorkforceData

function worker(body: WorkforceData, id: string): WorkerRow {
  const w = body.workers.find((x) => x.template_id === id)
  if (w === undefined) throw new Error(`the served body carries no worker ${id}`)
  return w
}

/** The fixture world's ids, so a test names the thing it means. */
const NOTES = 'wt-notes'
const DIGEST = 'wt-digest'
const DRAFT = 'wt-audit'

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
  scheduled.mockClear()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── the route flip ──────────────────────────────────────────────────────────

test('/workforce is live, and it is the surface that answers the URL rather than a stub', async () => {
  const { view } = await open()
  const text = view.container.textContent ?? ''
  expect(text).toContain('Workforce')
  expect(text, '/workforce still renders the not-built-yet stub').not.toContain('Not built yet')
  expect(view.container.querySelector('.workforce')).not.toBeNull()
  view.unmount()
})

test('the route was filled, not renamed — and it was the last unbuilt one', () => {
  const row = routes.find((r) => r.id === 'workforce')!
  expect(row.owner, 'the workforce row still names an owning packet').toBe('')
  expect(row.pattern, 'the route was renamed — a deep link would break (§41-B fill-never-rename)').toBe('/workforce')
  expect(row.title).toBe('Workforce')
  expect(row.nav).toBe(true)
  expect(matchRoute('/workforce').route.id).toBe('workforce')
  // Nothing is unbuilt any more: this was the eleventh and last row.
  expect(routes.filter((r) => r.owner !== '')).toEqual([])
})

// ── R11: the roster, and the honest states ──────────────────────────────────

test('every visible worker renders its identity, status, domain maturity and provenance', async () => {
  const { view } = await open()
  const body = served()
  expect(body.workers.length, 'the served roster is empty, so this asserts nothing').toBeGreaterThan(2)

  for (const w of body.workers) {
    const card = view.container.querySelector(`[data-worker="${w.template_id}"]`)
    expect(card, `worker ${w.template_id} does not render`).not.toBeNull()
    expect(card!.querySelector('[data-worker-kind]')!.textContent).toBe(w.kind)
    expect(card!.querySelector('[data-worker-scope]')!.textContent).toBe(w.scope)
    expect(card!.querySelector('[data-worker-status]')!.textContent).toBe(w.status)
    expect(card!.textContent).toContain(w.name)
    expect(card!.textContent, `worker ${w.template_id} does not name its owner`).toContain(w.owner)
  }

  // The active version's provenance and its supervised first-N counter.
  const notes = worker(body, NOTES)
  const active = notes.versions.find((v) => v.active)!
  const block = view.container.querySelector(`[data-version="${active.version_id}"]`)!
  expect(block.textContent).toContain(active.approved_by ?? '')
  expect(block.textContent).toContain(active.author_kind)
  expect(block.textContent).toContain(active.origin)
  expect(active.granted!.first_n_remaining, 'the fixture graduated this version, so first-N renders nothing').toBeGreaterThan(0)
  const firstN = view.container.querySelector(`[data-worker="${NOTES}"] [data-first-n]`)!
  expect(firstN.getAttribute('data-first-n')).toBe(String(active.granted!.first_n_remaining))
  // Not graduated is an ABSENCE with its reason, never a blank.
  expect(active.graduated_ts ?? '').toBe('')
  expect(block.textContent).toContain('not graduated')
  view.unmount()
})

test('a degraded domain renders as the structural fact it is, and a full one does not', async () => {
  const { view } = await open()
  const body = served()
  const digest = worker(body, DIGEST)
  expect(digest.domain.maturity, 'the fixture carries no degraded domain, so this asserts nothing').toBe('degraded')

  const degraded = view.container.querySelector(`[data-maturity="degraded"]`)
  expect(degraded, 'a degraded domain renders no marking').not.toBeNull()
  expect(degraded!.textContent).toContain('nothing here delivers without a person reviewing it')

  // And the S08.7 consequence rides the worker itself, in the server's words.
  const card = view.container.querySelector(`[data-worker="${DIGEST}"]`)!
  expect(card.querySelector('[data-requires-review]')!.getAttribute('data-requires-review')).toBe('true')
  expect(digest.delivery, 'the served worker carries no delivery policy, so this asserts nothing').not.toBeNull()
  for (const reason of digest.delivery!.reasons ?? []) {
    expect(card.textContent, 'a delivery reason is not rendered verbatim').toContain(reason)
  }

  // The other direction: the full domain does not get the degraded language.
  const notes = worker(body, NOTES)
  expect(notes.domain.maturity).toBe('full')
  const full = view.container.querySelector(`[data-maturity="full"]`)!
  expect(full.textContent).not.toContain('nothing here delivers without a person reviewing it')
  view.unmount()
})

test('a draft with no active version renders its absences with reasons, never a blank card', async () => {
  const { view } = await open()
  const draft = worker(served(), DRAFT)
  expect(draft.definition, 'the fixture draft carries a definition, so this asserts nothing').toBeNull()

  const card = view.container.querySelector(`[data-worker="${DRAFT}"]`)!
  expect(card.querySelector('[data-definition-absent]')!.textContent).toBe(draft.definition_absent)
  // Its one version was never approved, so nothing is granted — with the
  // server's reason, not an empty permissions table.
  const v = draft.versions[0]
  expect(v.granted).toBeNull()
  const block = card.querySelector(`[data-version="${v.version_id}"]`)!
  expect(block.querySelector('[data-granted-absent]')!.textContent).toBe(v.granted_absent)
  expect(block.textContent).toContain(v.validation_absent ?? '')
  view.unmount()
})

test('an empty roster renders the no-standing-army state, which is what a fresh host serves', async () => {
  const { view } = await open({
    workers: [],
    roster_scope: 'your own personal workers plus the household roster (S15.10)',
    outcome_scope: 'your own runs only',
    truncated: false,
    outcomes_truncated: false,
    cursor: 84,
  })
  const text = view.container.textContent ?? ''
  expect(text).toContain('Nothing has been composed yet')
  expect(text).toContain('a worker is built when recurring work earns one')
  // The scope statements still render: an empty answer has to say what it
  // covered, or "none" is indistinguishable from "none that you can see".
  expect(view.container.querySelector('[data-roster-scope]')).not.toBeNull()
  expect(view.container.querySelector('.worker')).toBeNull()
  view.unmount()
})

test('the served scope sentences render as served, both ways', async () => {
  const { view } = await open()
  expect(view.container.querySelector('[data-roster-scope]')!.textContent).toContain(served().roster_scope)
  expect(view.container.querySelector('[data-outcome-scope]')!.textContent).toContain(served().outcome_scope)
  view.unmount()

  const member = await open(fixtures.workforceMember())
  expect(member.view.container.querySelector('[data-roster-scope]')!.textContent).toContain(servedMember().roster_scope)
  expect(servedMember().roster_scope, 'the two bodies carry the same scope sentence, so this asserts nothing').not.toBe(
    served().roster_scope,
  )
  member.view.unmount()
})

test('the member body carries less than the operator body, and the map shows exactly what it was served', async () => {
  const all = served().workers.map((w) => w.template_id)
  const mine = servedMember().workers.map((w) => w.template_id)
  expect(all, "the operator's body is missing another member's personal worker").toContain(DIGEST)
  expect(mine, "the member's body carries another member's personal worker — the S15.10 identity rule leaked").not.toContain(
    DIGEST,
  )

  const { view } = await open(fixtures.workforceMember())
  expect(view.container.querySelector(`[data-worker="${DIGEST}"]`), 'the map rendered a worker its body did not carry').toBeNull()
  expect(view.container.querySelector(`[data-worker="${NOTES}"]`), 'the household worker does not render for a member').not.toBeNull()
  expect(view.container.querySelector(`[data-worker="${DRAFT}"]`), "the member's own draft does not render").not.toBeNull()
  view.unmount()
})

// ── R12: equipment, requested vs granted ────────────────────────────────────

test('requested equipment and granted permissions render as two blocks, distinguishable', async () => {
  const { view } = await open()
  const notes = worker(served(), NOTES)
  const card = view.container.querySelector(`[data-worker="${NOTES}"]`)!

  const requested = card.querySelector('[data-equipment="requested"]')!
  const granted = card.querySelector('[data-equipment="granted"]')!
  expect(requested).not.toBe(granted)

  const req = notes.definition!.requested_equipment
  expect((req.tools ?? []).length, 'the definition requests no tools, so this asserts nothing').toBeGreaterThan(0)
  for (const t of req.tools ?? []) expect(requested.textContent).toContain(t)
  for (const s of req.skills ?? []) expect(requested.textContent).toContain(s)
  for (const k of req.knowledge ?? []) expect(requested.textContent).toContain(k)
  // Connectors are empty on this worker, so the block says so rather than
  // showing an empty row nobody can read.
  expect(requested.textContent).toContain('none requested')

  const active = notes.versions.find((v) => v.active)!
  for (const t of active.granted!.granted_tools) expect(granted.textContent).toContain(t)
  expect(granted.textContent).toContain(active.granted!.confinement_class)
  expect(granted.textContent).toContain(active.granted!.egress)
  view.unmount()
})

test('"helpers" renders as the structural statement it is — no helper roster is invented', async () => {
  const { view } = await open()
  const helpers = view.container.querySelector('[data-helpers]')!
  expect(helpers.textContent).toContain('no standing helper roster')
  expect(helpers.textContent).toContain('chosen per run')
  // And the served body genuinely has no helper field to have rendered: the
  // S08.1 schema carries none, so a list here could only have been fabricated.
  const raw = JSON.stringify(fixtures.workforce())
  expect(raw).not.toContain('"helpers"')
  expect(raw).not.toContain('"helper_roster"')
  view.unmount()
})

// ── R13: the multi-stage chain ──────────────────────────────────────────────

test('an automation renders its step chain in order with the approval node marked', async () => {
  const { view } = await open()
  const digest = worker(served(), DIGEST)
  const wf = digest.definition!.workflow!
  expect(wf.steps.length, 'the automation fixture has no steps, so this asserts nothing').toBe(2)

  const card = view.container.querySelector(`[data-worker="${DIGEST}"]`)!
  const steps = [...card.querySelectorAll('.chain li')]
  expect(steps.map((s) => s.getAttribute('data-step'))).toEqual(wf.steps.map((s) => s.id))
  expect(steps.map((s) => s.getAttribute('data-approval'))).toEqual(wf.steps.map((s) => String(s.approval)))
  // Both arms exist in the fixture, so "marked" means something.
  expect(new Set(wf.steps.map((s) => s.approval))).toEqual(new Set([true, false]))
  const marked = steps.find((s) => s.getAttribute('data-approval') === 'true')!
  expect(marked.textContent).toContain('becomes a card, never a call')
  const unmarked = steps.find((s) => s.getAttribute('data-approval') === 'false')!
  expect(unmarked.textContent).not.toContain('becomes a card, never a call')
  expect(card.textContent).toContain(wf.service)
  expect(card.textContent).toContain('No model is in this loop')
  view.unmount()
})

test('an agentic worker renders its selectors and profile instead of a chain', async () => {
  const { view } = await open()
  const notes = worker(served(), NOTES)
  const def = notes.definition!
  expect(def.workflow, 'the agentic fixture carries a workflow, so this asserts nothing').toBeUndefined()

  const card = view.container.querySelector(`[data-worker="${NOTES}"]`)!
  expect(card.querySelector('[data-chain="selectors"]')).not.toBeNull()
  expect(card.querySelector('.chain'), 'an agentic worker rendered a step chain it does not have').toBeNull()
  expect(card.textContent).toContain(def.selectors.family ?? '')
  for (const c of def.selectors.task_classes ?? []) expect(card.textContent).toContain(c)
  for (const trig of def.selectors.triggers ?? []) expect(card.textContent).toContain(trig)
  expect(card.textContent).toContain(def.profile.duty ?? '')
  expect(card.textContent).toContain(def.profile.effort_floor ?? '')
  view.unmount()
})

// ── R14: per-version quality and cost ───────────────────────────────────────

test('per-version outcomes render as served, and both arms of the join are driven', async () => {
  // Driven from the MEMBER body, because there ONE worker carries both arms:
  // alice's own runs put outcomes on the active version, and the superseded
  // version's only run is bob's — so it renders the honest absence beside a
  // populated sibling. Two arms on one card is a stronger check than two arms
  // on two workers, and it is what catches a join keyed on the worker rather
  // than the version.
  const { view } = await open(fixtures.workforceMember())
  const notes = worker(servedMember(), NOTES)
  const withRuns = notes.versions.find((v) => v.outcomes.runs.length > 0)!
  const without = notes.versions.find((v) => v.outcomes.runs.length === 0)
  expect(withRuns, 'no version in the fixture has routed runs — the join is never exercised').toBeTruthy()
  expect(without, 'both versions carry runs, so the honest-absence arm is undriven').toBeTruthy()

  const block = view.container.querySelector(`[data-outcomes="${withRuns.version_id}"]`)!
  const rows = [...block.querySelectorAll('.routed-run')]
  expect(rows.map((r) => r.getAttribute('data-run'))).toEqual(withRuns.outcomes.runs.map((r) => r.run_id))
  for (const r of withRuns.outcomes.runs) {
    const row = block.querySelector(`[data-run="${r.run_id}"]`)!
    expect(row.getAttribute('data-cause')).toBe(r.cause)
    expect(row.textContent, 'the recorded routing reason is not rendered verbatim').toContain(r.plain_reason)
    expect(row.textContent).toContain(r.owner)
  }

  // The empty arm renders the SERVER's reason, so "none" is never a blank.
  const empty = view.container.querySelector(`[data-outcomes="${without!.version_id}"]`)!
  expect(empty.querySelector('.routed-run')).toBeNull()
  expect(empty.textContent).toContain(without!.outcomes.absent)
  view.unmount()
})

test('a verdict renders as recorded, and a routed run without one says so', async () => {
  const { view } = await open()
  const notes = worker(served(), NOTES)
  const runs = notes.versions.flatMap((v) => v.outcomes.runs)
  const withVerdict = runs.find((r) => r.verdicts.length > 0)
  const withoutVerdict = runs.find((r) => r.verdicts.length === 0)
  expect(withVerdict, 'no routed run carries a verdict — the populated arm is undriven').toBeTruthy()
  expect(withoutVerdict, 'every routed run carries a verdict — the absence arm is undriven').toBeTruthy()

  const good = view.container.querySelector(`[data-run="${withVerdict!.run_id}"] .routed-verdicts`)!
  expect(good.getAttribute('data-verdicts')).toBe(String(withVerdict!.verdicts.length))
  expect(good.textContent).toContain(withVerdict!.verdicts[0].verdict)
  const bare = view.container.querySelector(`[data-run="${withoutVerdict!.run_id}"] .routed-verdicts`)!
  expect(bare.textContent).toBe(withoutVerdict!.verdicts_absent)
  view.unmount()
})

test('money renders as served and never as a zero, and no total exists anywhere', async () => {
  const { view } = await open()
  const notes = worker(served(), NOTES)
  const runs = notes.versions.flatMap((v) => v.outcomes.runs)
  const metered = runs.find((r) => r.api_equiv_cost_usd !== null)
  const unmetered = runs.find((r) => r.api_equiv_cost_usd === null)
  expect(metered, 'no routed run carries a meter reading — the money render is undriven').toBeTruthy()
  expect(unmetered, 'every routed run carries a reading — the honest-nil arm is undriven').toBeTruthy()

  const paid = view.container.querySelector(`[data-run="${metered!.run_id}"] .routed-cost`)!
  // The figure is printed AS SERVED: the same number, not a rounded or scaled one.
  expect(paid.textContent).toContain(String(metered!.api_equiv_cost_usd))
  expect(paid.textContent).toContain(String(metered!.tokens))
  // EVERY run without a reading, not just one: an exact-equality check on a
  // single row cannot fail on a sibling that rendered a zero.
  const unread = runs.filter((r) => r.api_equiv_cost_usd === null)
  expect(unread.length, 'no routed run lacks a reading, so this loop is empty').toBeGreaterThan(1)
  for (const r of unread) {
    const cell = view.container.querySelector(`[data-run="${r.run_id}"] .routed-cost`)!
    expect(cell.textContent, `run ${r.run_id} did not render its absence reason`).toBe(r.meter_absent)
    expect(cell.textContent, `run ${r.run_id} rendered a missing reading as a zero`).not.toMatch(/USD\s*0\b/)
  }
  // The two absences are DIFFERENT sentences — a refusal and an empty reading
  // are different facts, and the fixture carries both.
  expect(new Set(unread.map((r) => r.meter_absent)).size, 'every absence reads the same').toBeGreaterThan(1)

  // No cross-run figure anywhere on the surface: a per-version or per-worker
  // total would be arithmetic nobody performed (§37; D2).
  const text = (view.container.textContent ?? '').toLowerCase()
  for (const banned of ['total', 'average', 'sum of', 'combined']) {
    expect(text, `the map rendered a ${banned} figure`).not.toContain(banned)
  }
  view.unmount()
})

// ── R11's structural negative: no mutation affordance ───────────────────────

test('the map has zero mutation affordances — a DOM walk over the whole rendered surface', async () => {
  const { view, log } = await open()
  const root = view.container.querySelector('.workforce')!
  // A form, a submit, a text input, a checkbox, a select: every shape an edit
  // could take. The surface is view-only (S15.10 parks editing to 15.5), so
  // none of them may exist inside it.
  for (const sel of ['form', 'input', 'textarea', 'select', 'button[type="submit"]']) {
    expect([...root.querySelectorAll(sel)], `the map renders a ${sel}`).toEqual([])
  }
  // Buttons at all: the only interactive elements the map ships are the
  // <details> disclosures for superseded versions, which are not buttons.
  expect([...root.querySelectorAll('button')], 'the map renders a button').toEqual([])
  // And nothing links out to an act.
  for (const a of [...root.querySelectorAll('a')]) {
    expect(a.getAttribute('href'), 'a link on the map targets a mutation route').not.toMatch(/accept|answer|approve|resolve/)
  }
  // Not one non-GET request was made rendering the whole thing.
  expect(log.calls.filter((c) => c.method !== 'GET'), 'the map fired a mutating request').toEqual([])
  // The probe: the walk can fail. Planting each shape inside the surface is
  // caught, so "no hits" means the surface is clean rather than that the
  // selectors match nothing.
  for (const tag of ['form', 'input', 'button']) {
    const planted = document.createElement(tag)
    root.appendChild(planted)
    expect([...root.querySelectorAll(tag)]).toHaveLength(1)
    planted.remove()
  }
  view.unmount()
})

test('the map reads exactly one route, and it is a GET', async () => {
  const { view, log } = await open()
  const mine = log.calls.filter((c) => c.path.startsWith('/api/workforce'))
  expect(mine.map((c) => `${c.method} ${c.path}`)).toEqual(['GET /api/workforce'])
  view.unmount()
})

// ── R17: live wiring ────────────────────────────────────────────────────────

test('a declared frame re-reads and an undeclared one reaches nothing', async () => {
  const stream = inertStream()
  const log = scriptedFetch(workforceRoutes())
  window.history.replaceState(null, '', hrefFor('workforce'))
  const view = mount(<App stream={stream} />)
  await flush()

  const reads = () => log.calls.filter((c) => c.path === '/api/workforce').length
  // ONE read on mount. The rule is §44-B drain r1 D7 as §45 restates it: the
  // FIRST subscriber OPENS the source and reads once; a later one JOINS and gets
  // an immediate synchronous `connected` resnapshot, so it reads twice. React
  // commits child effects before the parent's, so this view's hook opens the
  // source and the shell's connection indicator is the joiner — the same
  // ordering the deliverable page's detail hook has.
  expect(reads(), 'one subscribed mount that OPENS the source costs exactly one read (§44-B drain r1 D7)').toBe(1)

  const source = FakeSource.last()
  source.open()
  await flush()
  const afterOpen = reads()

  source.send('worker.approved', frame(90, 'worker.approved', ['platform']))
  await flush()
  expect(reads(), 'a declared frame did not trigger a re-read').toBe(afterOpen + 1)

  // An undeclared type reaches nothing: EventSource has no catch-all, so a type
  // this view did not name is a frame it never hears.
  const before = reads()
  source.send('review.comment', frame(91, 'review.comment', ['deliverable']))
  await flush()
  expect(reads(), 'an undeclared frame reached this view').toBe(before)
  view.unmount()
})

test('every declared type is one this view actually renders something from', () => {
  // §42: a declared type is either what puts something on the page or what takes
  // it off. The three worker events deliberately NOT declared are the check that
  // this list was chosen rather than copied — a gap record, a review note and an
  // automation execution are all real and none of them moves this page.
  expect(workforceEventTypes).toContain('worker.approved')
  expect(workforceEventTypes).toContain('worker.domain_maturity')
  expect(workforceEventTypes).toContain('routing.decided')
  expect(workforceEventTypes).toContain('verdict.recorded')
  expect(workforceEventTypes).not.toContain('worker.gap')
  expect(workforceEventTypes).not.toContain('worker.review')
  expect(workforceEventTypes).not.toContain('worker.automation_run')
  // No duplicates, so the subscription registers each listener once.
  expect(new Set(workforceEventTypes).size).toBe(workforceEventTypes.length)
})

test('an unreachable control plane renders its failure rather than an empty roster', async () => {
  const { view } = await open(undefined, { 'GET /api/workforce': { status: 503, body: { code: 'not_wired' } } })
  const text = view.container.textContent ?? ''
  expect(text).toContain('The control plane answered 503')
  // "No registry" must not read as "no workers": the no-standing-army state is
  // an ANSWER about this platform and would be a lie here.
  expect(text, 'a failed read rendered as an empty roster').not.toContain('Nothing has been composed yet')
  view.unmount()
})

// ── R11: forward tolerance ──────────────────────────────────────────────────

test('an unknown domain, kind, status and maturity all render honestly rather than vanishing', async () => {
  const body = served()
  const alien = {
    ...worker(body, NOTES),
    template_id: 'wt-alien',
    name: 'ledger-reconciler',
    kind: 'swarm',
    status: 'quarantined',
    domain: { name: 'finance', maturity: 'provisional' },
    delivery: { deliverable: true, requires_review: false },
  }
  const { view } = await open({ ...body, workers: [...body.workers, alien] } as unknown as Record<string, unknown>)

  const card = view.container.querySelector('[data-worker="wt-alien"]')
  expect(card, 'a worker with an unknown kind vanished from the map').not.toBeNull()
  expect(card!.querySelector('[data-worker-kind]')!.textContent).toBe('swarm')
  expect(card!.querySelector('[data-worker-status]')!.textContent).toBe('quarantined')
  // A domain nobody anticipated gets its own group, labelled with the served
  // value — never dropped, and never renamed into something friendlier.
  expect(view.container.textContent).toContain('Domain: finance')
  const marking = view.container.querySelector('[data-maturity="provisional"]')
  expect(marking, 'an unknown maturity rendered nothing').not.toBeNull()
  expect(marking!.textContent).toContain('provisional')
  // And it does NOT acquire the degraded consequence it never claimed.
  expect(marking!.textContent).not.toContain('nothing here delivers without a person reviewing it')
  view.unmount()
})

test('an unknown workflow verb and an unparsable automation body both render honestly', async () => {
  const body = served()
  const digest = worker(body, DIGEST)
  const odd = {
    ...digest,
    template_id: 'wt-odd',
    definition: {
      ...digest.definition!,
      workflow: { dialect: 'sinet-automation/9', service: 'ledger', steps: [{ id: 'sweep', verb: 'ledger.teleport', approval: true }] },
    },
  }
  const broken = {
    ...digest,
    template_id: 'wt-broken',
    definition: { ...digest.definition!, workflow: undefined, workflow_absent: 'automation: dialect sinet-automation/9 (this build speaks sinet-automation/1)' },
  }
  const { view } = await open({ ...body, workers: [odd, broken] } as unknown as Record<string, unknown>)

  const oddCard = view.container.querySelector('[data-worker="wt-odd"]')!
  expect(oddCard.querySelector('[data-step="sweep"]')!.textContent).toContain('ledger.teleport')
  expect(oddCard.textContent).toContain('sinet-automation/9')

  // A body the platform's own parser refused renders the parser's reason — the
  // definition is still a worker, and hiding the failure would leave a card that
  // silently shows no stages at all.
  const brokenCard = view.container.querySelector('[data-worker="wt-broken"]')!
  expect(brokenCard.querySelector('[data-workflow-absent]')!.textContent).toContain('this build speaks')
  view.unmount()
})

// ── R19: nothing else moved ─────────────────────────────────────────────────

test('the map is the only consumer of the workforce read, and it renders under the shell', async () => {
  // The shell's own surfaces are untouched: mounting the map still shows the
  // nav and the connection state, so this is a route arm and not a takeover.
  const { view } = await open()
  expect(view.container.querySelector('.shell-nav')).not.toBeNull()
  const nav = [...view.container.querySelectorAll('.shell-nav a')].map((a) => a.getAttribute('href'))
  expect(nav, 'the workforce link left the nav').toContain('/workforce')
  view.unmount()

  // And no other view reads it: a second consumer would mean two shapes to keep
  // in step with one served body.
  const log = scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', '/')
  const mission = mount(<App stream={inertStream()} />)
  await flush()
  expect(log.calls.filter((c) => c.path.startsWith('/api/workforce'))).toEqual([])
  mission.unmount()
})

test('a per-version run list that was cut says so, so a reading is never read as a history', async () => {
  const body = served()
  const notes = worker(body, NOTES)
  const active = notes.versions.find((v) => v.active)!
  const cut = {
    ...notes,
    versions: [{ ...active, outcomes: { ...active.outcomes, truncated: true } }],
  }
  const { view } = await open({ ...body, workers: [cut] } as unknown as Record<string, unknown>)
  const note = view.container.querySelector('[data-runs-truncated]')
  expect(note, 'a truncated run list rendered as a complete one').not.toBeNull()
  expect(note!.textContent).toContain('not all of them')
  view.unmount()

  // The other direction: the committed body is NOT truncated, so the notice
  // does not render — without this the assertion above would pass on a surface
  // that always shows it.
  const whole = await open()
  expect(whole.view.container.querySelector('[data-runs-truncated]')).toBeNull()
  whole.view.unmount()
})

test("a run whose verdict rounds were cut says so, and one that was not does not", async () => {
  const body = served()
  const notes = worker(body, NOTES)
  const active = notes.versions.find((v) => v.active)!
  const [first, ...rest] = active.outcomes.runs
  const cut = {
    ...notes,
    versions: [
      {
        ...active,
        outcomes: {
          ...active.outcomes,
          runs: [{ ...first, verdicts: [{ round: 6, verdict: 'REWORK', ts: '2026-07-20T09:04:00Z' }], verdicts_truncated: true }, ...rest],
        },
      },
    ],
  }
  const { view } = await open({ ...body, workers: [cut] } as unknown as Record<string, unknown>)
  const marked = view.container.querySelector(`[data-run="${first.run_id}"] [data-verdicts-truncated]`)
  expect(marked, 'a cut round list rendered as a complete one').not.toBeNull()
  expect(marked!.textContent).toContain('not all of them')
  // The sibling runs were not cut, so nothing there claims it.
  for (const r of rest) {
    expect(
      view.container.querySelector(`[data-run="${r.run_id}"] [data-verdicts-truncated]`),
      `run ${r.run_id} claims truncation it was not served`,
    ).toBeNull()
  }
  view.unmount()
})
