import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import type { ApprovalItem, ApprovalList, BenchmarkVerdictForms } from './api'
import { FakeSource, fixtures, oversightRoutes, scriptedFetch, signedIn, type Scripted } from './doubles'
import { EventStream } from './events'
import { click, choose, flush, mount, typeInto } from './testing'

/**
 * The approval inbox (Spec S15.6; S06.9; S14.4–S14.7; BENCH-REG §3/§12; S09.7),
 * driven against the GOLDEN FIXTURES — the same bytes an internal/api test
 * asserts the real handlers still serve.
 *
 * Two bodies are used throughout and the difference between them is the point:
 * `approvals` is the OPERATOR's read and `approvals-mine` is alice's. Because
 * `answerable` is computed per caller (D10), "yours to answer" and "not yours,
 * and here is why" are two served bodies rather than two renders — and the
 * ninth kind exists in exactly one of them, because a memory-conflict question
 * reaches its addressee and nobody else, the operator included.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

/** open mounts the app at a path with a scripted control plane. `body` is the
 *  inbox read; everything else the surfaces touch comes from the landed doubles. */
async function open(path: string, body: Record<string, unknown>, extra: Record<string, Scripted> = {}) {
  const routes: Record<string, Scripted> = {
    ...oversightRoutes(),
    'GET /api/approvals': { body },
    ...extra,
  }
  const log = scriptedFetch(routes)
  window.history.replaceState(null, '', path)
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}


/** verb builds the path a card's verb really lands on. Card ids carry `:`,
 *  `#` and a unit separator, so the client percent-encodes the segment — the
 *  scripted control plane has to be keyed on the encoded form, exactly as the
 *  real mux receives it. */
const verb = (id: string, suffix: string) => `/api/approvals/${encodeURIComponent(id)}/${suffix}`

const mine = () => fixtures.approvalsMine() as unknown as ApprovalList
const all = () => fixtures.approvals() as unknown as ApprovalList

function card(list: ApprovalList, id: string): ApprovalItem {
  const found = list.items.find((i) => i.id === id)
  if (!found) throw new Error(`the fixture carries no card ${id} — the test would assert nothing`)
  return found
}

const rowIDs = (view: { container: HTMLElement }) =>
  [...view.container.querySelectorAll('[data-card-id]')].map((n) => n.getAttribute('data-card-id'))

const row = (view: { container: HTMLElement }, id: string) =>
  view.container.querySelector(`[data-card-id="${CSS.escape(id)}"]`) as HTMLElement

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── R1: one ranked queue, every kind, served order ────────────────────────

test('every served kind renders once, in the order the control plane ranked them', async () => {
  const served = all()
  const { view } = await open('/inbox', served)

  expect(rowIDs(view), 'the inbox re-ordered or dropped a served card').toEqual(served.items.map((i) => i.id))

  // All eight kinds the operator can see are present — including the EIGHTH
  // (a failed pair riding the verdict id space at Low with decline only).
  const kinds = new Set([...view.container.querySelectorAll('[data-kind]')].map((n) => n.getAttribute('data-kind')))
  for (const kind of [
    'ask',
    'effect',
    'watchdog_flag',
    'conformance_card',
    'drift_card',
    'benchmark_verdict',
    'benchmark_alarm',
  ]) {
    expect([...kinds], `the operator's inbox is missing kind ${kind}`).toContain(kind)
  }
  // WITNESSED BY ID, not just by kind. `effect` used to be uniquely witnessed by
  // e-notify; B6-8 part B added a second effect card (an automation's station-3
  // proposal), which satisfies the kind check on its own and would have let
  // e-notify disappear unnoticed. A card each kind is witnessed BY is a check;
  // a kind that any card can satisfy is a weaker one.
  for (const id of ['effect:e-notify', 'effect:e-digest']) {
    expect(rowIDs(view), `the inbox is missing the card ${id} that witnesses its kind`).toContain(id)
  }
  const failed = card(served, 'benchmark_verdict:bp-archive')
  expect(failed.tier, 'the failed-pair card is not the Low-tier eighth kind').toBe('low')
  expect(failed.actions).toEqual(['decline'])
})

test('the client never re-ranks: a shuffled body renders in ITS order, not a sorted one', async () => {
  const served = all()
  const shuffled = { ...served, items: [...served.items].reverse() }
  const { view } = await open('/inbox', shuffled)
  expect(rowIDs(view)).toEqual(shuffled.items.map((i) => i.id))
})

test('a card you cannot answer renders the served reason and exposes NO acting control', async () => {
  const served = all()
  const { view } = await open('/inbox', served)
  const notMine = served.items.find((i) => !i.answerable)
  expect(notMine, 'the operator body carries no unanswerable card').toBeTruthy()

  const node = row(view, notMine?.id ?? '')
  expect(node.querySelector('[data-acts="read-only"]')).not.toBeNull()
  expect(node.textContent).toContain(notMine?.not_answerable_reason ?? '')
  expect(node.querySelectorAll('button[data-action]'), 'a dead control was rendered on a card the verb would refuse')
    .toHaveLength(0)
})

// ── R2/R4: the ask card explains itself, from its stored snapshot ─────────

test('the plan card renders its Layer-1 screen, the 13.5 help block and Layer 2 — all from the snapshot', async () => {
  const served = mine()
  const { view } = await open('/inbox', served)
  const node = row(view, 'ask:ask-ship')
  const text = node.textContent ?? ''

  const snap = card(served, 'ask:ask-ship').card as {
    approval: {
      layer1: { restatement: string; help: { what: string; wrong: string; recommend: string }; assumptions: { text: string }[] }
      layer2: { acs: { plain: string }[] }
    }
  }
  expect(text).toContain(snap.approval.layer1.restatement)
  // The assumptions are the centerpiece, and each one is on screen.
  for (const a of snap.approval.layer1.assumptions) {
    expect(node.querySelector(`[data-assumption="${CSS.escape(a.text)}"]`), `assumption missing: ${a.text}`).not.toBeNull()
  }
  // The 13.5 block, verbatim — the view drafts none of these three sentences.
  const help = node.querySelector('[data-help="13.5"]')?.textContent ?? ''
  expect(help).toContain(snap.approval.layer1.help.what)
  expect(help).toContain(snap.approval.layer1.help.wrong)
  expect(help).toContain(snap.approval.layer1.help.recommend)
  // Layer 2 is present and expandable.
  expect(node.querySelector('details.layer2')).not.toBeNull()
  expect(node.querySelector('.acs')?.textContent).toContain(snap.approval.layer2.acs[0].plain)
})

test('a plan card always carries Approve AND Re-plan, from its OWN served vocabulary', async () => {
  const served = mine()
  const { view } = await open('/inbox', served)
  const actions = [...row(view, 'ask:ask-ship').querySelectorAll('button[data-action]')].map((b) =>
    b.getAttribute('data-action'),
  )
  expect(actions).toEqual(card(served, 'ask:ask-ship').actions)
  expect(actions).toContain('approve')
  expect(actions).toContain('replan')
  expect(actions).toContain('reinterview')
})

test("a decision card's actions come from the choice VALUES the pipeline accepts", async () => {
  // The B6-6 read fix: internal/intake marshals []Option{Label, Value} and the
  // answer validator compares against VALUE. Reading the key that was there
  // before derived one empty string per choice.
  const served = mine()
  const actions = card(served, 'ask:ask-coverage').actions ?? []
  expect(actions, 'the coverage card serves blank actions again').toEqual([
    'replan',
    'drop_criterion',
    'proceed_uncovered',
  ])
  const { view } = await open('/inbox', served)
  const rendered = [...row(view, 'ask:ask-coverage').querySelectorAll('button[data-action]')].map((b) =>
    b.getAttribute('data-action'),
  )
  expect(rendered).toEqual(actions)
})

test('a delta card renders ADDED / MODIFIED / REMOVED and is ANSWERABLE from its own vocabulary', async () => {
  const served = mine()
  // The drain fixed this at the producer: a delta card issued with no action
  // vocabulary was unanswerable from any surface that renders controls from the
  // card, while it held the task's open ask open.
  expect(card(served, 'ask:ask-delta').actions, 'the delta card serves no vocabulary again').toEqual([
    'approve',
    'reject',
  ])
  const { view, log } = await open('/inbox', served)
  const node = row(view, 'ask:ask-delta')
  expect([...node.querySelectorAll('[data-delta]')].map((n) => n.getAttribute('data-delta'))).toEqual([
    'ADDED',
    'MODIFIED',
    'REMOVED',
  ])
  const actions = [...node.querySelectorAll('button[data-action]')].map((b) => b.getAttribute('data-action'))
  expect(actions).toEqual(['approve', 'reject'])

  // …and answering one really posts the delta answer, in the card's own shape.
  log.set(`POST ${verb('ask:ask-delta', 'answer')}`, {
    body: { id: 'ask:ask-delta', applied: true, state: 'answered', detail: 'the delta is adopted' },
  })
  click(node.querySelector('button[data-action="approve"]'))
  await flush()
  const posted = log.calls.find((c) => c.path === verb('ask:ask-delta', 'answer'))
  expect(posted?.body).toMatchObject({ payload_hash: card(served, 'ask:ask-delta').payload_hash, answer: { action: 'approve' } })
  expect(row(view, 'ask:ask-delta').textContent).toContain('the delta is adopted')
})

// ── R5: staleness and expiry, from served fields only ─────────────────────

test('a stale card flags it with the served reasons and STILL offers approve', async () => {
  const served = mine()
  const stale = card(served, 'ask:ask-ship')
  expect(stale.stale, 'the fixture card is not stale, so this asserts nothing').toBe(true)
  const { view } = await open('/inbox', served)
  const node = row(view, 'ask:ask-ship')

  const flag = node.querySelector('[data-stale-flag="true"]')
  expect(flag, 'a stale card renders no flag').not.toBeNull()
  for (const reason of stale.stale_reasons ?? []) {
    expect(flag?.textContent).toContain(reason)
  }
  // The flag never blocks approving, and the one-click re-plan is beside it.
  const approve = node.querySelector('button[data-action="approve"]') as HTMLButtonElement
  expect(approve.disabled, 'a staleness flag disabled the approve').toBe(false)
  expect(node.querySelector('button[data-action="replan"]')).not.toBeNull()
})

test('expiry renders from the served instant — and renders nothing when none is served', async () => {
  vi.setSystemTime(new Date('2026-07-20T09:02:10Z'))
  const served = mine()
  const withExpiry = card(served, 'ask:ask-ship')
  expect(withExpiry.expiry_at, 'the fixture card has no expiry').toBeTruthy()

  const { view } = await open('/inbox', served)
  const node = row(view, 'ask:ask-ship')
  expect(node.querySelector('[data-expiry]')?.getAttribute('data-expiry')).toBe(withExpiry.expiry_at)
  expect(node.querySelector('.expiry')?.textContent).toContain('in 10s')

  // The other direction: a body with no expiry_at renders no countdown at all
  // rather than inventing a deadline.
  const bare = { ...served, items: served.items.map((i) => ({ ...i, expiry_at: undefined })) }
  const { view: v2 } = await open('/inbox', bare)
  expect(v2.container.querySelectorAll('[data-expiry]')).toHaveLength(0)
  vi.useRealTimers()
})

// ── R3: tier mechanics, rendered from served flags ────────────────────────

test('only batchable cards are selectable, and the action is chosen FIRST', async () => {
  const served = mine()
  const { view } = await open('/inbox', served)
  const bar = view.container.querySelector('.batch-bar')!
  expect(bar.getAttribute('data-batchable')).toBe(String(served.items.filter((i) => i.batchable).length))

  // Before an action is chosen nothing is selectable — a mixed selection is not
  // split, it is impossible.
  expect(bar.querySelectorAll('[data-batch-pick]')).toHaveLength(0)

  choose(bar.querySelector('[data-field="batch-action"]') as HTMLSelectElement, 'approve')
  const offered = [...bar.querySelectorAll('[data-batch-pick]')].map((n) => n.getAttribute('data-batch-pick'))
  const expected = served.items.filter((i) => i.batchable && (i.actions ?? []).includes('approve')).map((i) => i.id)
  expect(offered).toEqual(expected)
  expect(offered.length, 'the fixture has fewer than two batchable cards, so a "set" cannot be driven').toBeGreaterThan(1)

  // A Medium card is never in that list, whatever its vocabulary says.
  expect(offered).not.toContain('ask:ask-ship')
})

test('the OQ10 constraint BITES: a batchable card that does not accept the action is not offered', async () => {
  // The whole point of choosing the action first is that a mixed selection is
  // impossible rather than silently split. That is only observable when the
  // batchable cards DISAGREE about their vocabulary, so the fixture world
  // carries three: two approval-band cards and one decision-band card.
  const served = mine()
  const batchable = served.items.filter((i) => i.batchable)
  expect(batchable.length, 'fewer than three batchable cards — the constraint is untestable').toBe(3)
  const vocabularies = new Set(batchable.map((i) => (i.actions ?? []).join(',')))
  expect(vocabularies.size, 'every batchable card shares one vocabulary, so the filter cannot be seen to work')
    .toBeGreaterThan(1)

  const { view } = await open('/inbox', served)
  const bar = view.container.querySelector('.batch-bar')!
  const select = bar.querySelector('[data-field="batch-action"]') as HTMLSelectElement
  const offeredFor = (action: string) => {
    choose(select, action)
    return [...view.container.querySelectorAll('[data-batch-pick]')].map((n) => n.getAttribute('data-batch-pick'))
  }

  // `approve` is the approval band's; the decision card does not accept it and
  // is therefore NOT offered — this is the assertion that fails the moment the
  // filter is removed.
  const approvable = offeredFor('approve')
  expect(approvable).toEqual(['ask:ask-claim', 'ask:ask-notes'])
  expect(approvable, 'a card whose vocabulary lacks the chosen action was offered').not.toContain('ask:ask-sweep')

  // `drop_criterion` is the decision band's, and only that card accepts it.
  expect(offeredFor('drop_criterion')).toEqual(['ask:ask-sweep'])

  // `replan` is the one action all three share, so all three are offered — the
  // constraint narrows, it never hides a card that genuinely accepts the act.
  expect(offeredFor('replan')).toEqual(['ask:ask-claim', 'ask:ask-notes', 'ask:ask-sweep'])
})

test('the batch POSTs one item per selected card, each with ITS OWN pin — and renders each outcome', async () => {
  const served = mine()
  const { view, log } = await open('/inbox', served)
  const bar = view.container.querySelector('.batch-bar')!
  choose(bar.querySelector('[data-field="batch-action"]') as HTMLSelectElement, 'approve')
  for (const box of bar.querySelectorAll('[data-batch-pick]')) click(box)

  // A PARTIAL failure: one item refused beside applied siblings.
  log.set('POST /api/approvals/answer-batch', {
    body: {
      outcomes: [
        { id: 'ask:ask-claim', status: 200, result: { id: 'ask:ask-claim', applied: true, state: 'answered' } },
        { id: 'ask:ask-notes', status: 409, code: 'stale_payload', detail: 'the card changed since it was shown' },
      ],
      cursor: 30,
    },
  })
  click(bar.querySelector('[data-action="answer-batch"]'))
  await flush()

  const posted = log.calls.find((c) => c.path === '/api/approvals/answer-batch')
  expect(posted, 'no batch was posted').toBeTruthy()
  const items = (posted?.body as { items: { id: string; payload_hash: string; answer: { action: string } }[] }).items
  expect(items).toHaveLength(2)
  for (const item of items) {
    expect(item.payload_hash, `${item.id} was batched without its own pin`).toBe(card(served, item.id).payload_hash)
    expect(item.answer.action).toBe('approve')
  }

  // Each outcome renders individually — a refusal does not swallow its siblings.
  const outcomes = [...view.container.querySelectorAll('[data-outcome-id]')]
  expect(outcomes.map((n) => n.getAttribute('data-outcome-status'))).toEqual(['200', '409'])
  expect(outcomes[1].textContent).toContain('stale_payload')
})

test('a High-tier card demands the PIN, sends it in the SAME request, and keeps it nowhere', async () => {
  const served = mine()
  const effect = card(served, 'effect:e-notify')
  expect(effect.step_up_required, 'the fixture effect is not step-up').toBe(true)

  const { view, log } = await open('/inbox', served)
  const node = row(view, 'effect:e-notify')
  const pin = node.querySelector('[data-field="pin"]') as HTMLInputElement
  expect(pin, 'a step-up card renders no PIN field').not.toBeNull()
  expect(pin.type, 'the PIN is not a password field').toBe('password')

  typeInto(pin, '4242')
  log.set(`POST ${verb('effect:e-notify', 'answer')}`, {
    body: { id: 'effect:e-notify', applied: true, state: 'approved', detail: 'approved: execution is still the journal' },
  })
  click(node.querySelector('button[data-action="approve"]'))
  await flush()

  const posted = log.calls.find((c) => c.path === verb('effect:e-notify', 'answer'))
  const body = posted?.body as { pin?: string; payload_hash: string; answer: { action: string } }
  expect(body.pin, 'the PIN did not ride the answer request').toBe('4242')
  expect(body.payload_hash).toBe(effect.payload_hash)
  expect(body.answer.action).toBe('approve')

  // After the submit the field is empty, and the PIN is in no node's value and
  // in no rendered text. The DOM is WALKED rather than serialized: serializing
  // it would put a token the escape scan forbids into this file, and that
  // scan's allowlist is empty on purpose.
  const after = row(view, 'effect:e-notify').querySelector('[data-field="pin"]') as HTMLInputElement
  expect(after.value, 'the PIN survived the submit').toBe('')
  const values = [...view.container.querySelectorAll('input')].map((i) => i.value)
  expect(values, 'the PIN is still sitting in an input').not.toContain('4242')
  expect(view.container.textContent, 'the PIN was rendered as text').not.toContain('4242')
})

test('a refused PIN re-prompts honestly and nothing is left claiming it applied', async () => {
  const served = mine()
  const { view, log } = await open('/inbox', served)
  const node = row(view, 'effect:e-notify')
  typeInto(node.querySelector('[data-field="pin"]') as HTMLInputElement, '0000')
  log.set(`POST ${verb('effect:e-notify', 'answer')}`, {
    status: 401,
    body: { error: 'pin_rejected', detail: 'PIN verification failed' },
  })
  click(node.querySelector('button[data-action="approve"]'))
  await flush()

  const after = row(view, 'effect:e-notify')
  expect(after.querySelector('[data-outcome="failed"]')?.textContent).toContain('PIN verification failed')
  expect(after.querySelector('[data-field="pin"]')).not.toBeNull()
  expect((after.querySelector('[data-field="pin"]') as HTMLInputElement).value).toBe('')
})

// ── R3/OQ8: co-approval, both states, from the served block ───────────────

test('an effect renders both approver states from the served approvals block', async () => {
  const served = mine()
  const platform: ApprovalList = {
    ...served,
    items: served.items.map((i) =>
      i.id === 'effect:e-notify'
        ? {
            ...i,
            approvals: {
              platform_level: true,
              owner_approved: true,
              owner_approved_by: 'alice',
              operator_approved: false,
            },
          }
        : i,
    ),
  }
  const { view } = await open('/inbox', platform)
  const block = row(view, 'effect:e-notify').querySelector('.co-approval')!
  expect(block.getAttribute('data-platform-level')).toBe('true')
  const signed = [...block.querySelectorAll('dd')].map((d) => d.getAttribute('data-signed'))
  expect(signed, 'both limbs must be visible on a platform-level effect').toEqual(['true', 'false'])
  expect(block.textContent).toContain('alice')
})

test('a signature that completes nothing renders as recorded-and-waiting, from the response', async () => {
  const served = mine()
  const { view, log } = await open('/inbox', served)
  log.set(`POST ${verb('effect:e-notify', 'answer')}`, {
    body: {
      id: 'effect:e-notify',
      applied: true,
      state: 'proposed',
      approvals: { platform_level: true, owner_approved: true, owner_approved_by: 'alice', operator_approved: false },
      detail: 'recorded: a platform-level effect needs both the owner and the operator before it is approved',
    },
  })
  const node = row(view, 'effect:e-notify')
  typeInto(node.querySelector('[data-field="pin"]') as HTMLInputElement, '4242')
  click(node.querySelector('button[data-action="approve"]'))
  await flush()

  const outcome = row(view, 'effect:e-notify').querySelector('[data-outcome="applied"]')?.textContent ?? ''
  expect(outcome).toContain('proposed')
  expect(outcome).toContain('needs both the owner and the operator')
})

// ── R6: the answer flow ───────────────────────────────────────────────────

test('a 409 stale_payload re-renders the CARRIED card with a visible notice, and retries nothing', async () => {
  const served = mine()
  const { view, log } = await open('/inbox', served)
  const fresh: ApprovalItem = {
    ...card(served, 'ask:ask-ship'),
    payload_hash: 'a-brand-new-hash',
    card: {
      kind: 'approval',
      approval: {
        layer1: { restatement: 'The plan changed while you were reading it.', assumptions: [], help: {} },
        layer2: {},
      },
    },
  }
  log.set(`POST ${verb('ask:ask-ship', 'answer')}`, {
    status: 409,
    body: { error: 'stale_payload', detail: 'the card changed since it was shown', current: fresh },
  })
  click(row(view, 'ask:ask-ship').querySelector('button[data-action="approve"]'))
  await flush()

  const node = row(view, 'ask:ask-ship')
  expect(node.querySelector('[data-changed="stale-payload"]'), 'the swap was silent').not.toBeNull()
  expect(node.textContent).toContain('The plan changed while you were reading it.')
  // Exactly ONE answer POST: a stale hash is never retried blind.
  expect(log.calls.filter((c) => c.path === verb('ask:ask-ship', 'answer'))).toHaveLength(1)
})

test('a repeat answer renders applied:false as the already-resolved state it is', async () => {
  const { view, log } = await open('/inbox', mine())
  log.set(`POST ${verb('ask:ask-ship', 'answer')}`, {
    body: {
      id: 'ask:ask-ship',
      applied: false,
      state: 'answered',
      detail: 'already answered: the recorded answer is returned and nothing fired again',
    },
  })
  click(row(view, 'ask:ask-ship').querySelector('button[data-action="approve"]'))
  await flush()
  const text = row(view, 'ask:ask-ship').querySelector('[data-outcome="applied"]')?.textContent ?? ''
  expect(text).toContain('already answered')
  expect(text).toContain('nothing fired again')
})

test('after any verb the inbox re-reads — the POST is followed by a GET', async () => {
  const { view, log } = await open('/inbox', mine())
  log.set(`POST ${verb('ask:ask-ship', 'answer')}`, {
    body: { id: 'ask:ask-ship', applied: true, state: 'answered', detail: 'answered' },
  })
  const before = log.calls.filter((c) => c.path === '/api/approvals').length
  click(row(view, 'ask:ask-ship').querySelector('button[data-action="approve"]'))
  await flush()
  expect(log.calls.filter((c) => c.path === '/api/approvals').length, 'the view did not re-read after its own verb')
    .toBeGreaterThan(before)
})

test('Re-plan drives the S06.9 structured contest into the answer', async () => {
  const { view, log } = await open('/inbox', mine())
  const node = row(view, 'ask:ask-ship')
  typeInto(node.querySelector('[data-field="contest"]') as HTMLInputElement, 'AC-2')
  log.set(`POST ${verb('ask:ask-ship', 'answer')}`, {
    body: { id: 'ask:ask-ship', applied: true, state: 'answered', detail: 're-planning' },
  })
  click(node.querySelector('button[data-action="replan"]'))
  await flush()

  const posted = log.calls.find((c) => c.path === verb('ask:ask-ship', 'answer'))
  expect(posted?.body).toMatchObject({ answer: { action: 'replan', contest: { target: 'AC-2' } } })
})

// ── R7/R8: the oversight cards ────────────────────────────────────────────

test("suppress posts the card's OWN identifiers and renders the served retune story", async () => {
  const served = mine()
  const flag = card(served, 'watchdog_flag:r-ship\x1fwatchdog.loop')
  const { view, log } = await open('/inbox', served)
  const node = row(view, flag.id)

  log.set('POST /api/watchdog/flags/suppress', {
    body: {
      run_id: 'r-ship',
      anomaly_class: 'watchdog.loop',
      suppressed: true,
      detail: 'suppressed: every ⚙ watchdog.suppress_retune_count-th suppression proposes a threshold raise',
    },
  })
  click(node.querySelector('button[data-action="suppress"]'))
  await flush()

  const posted = log.calls.find((c) => c.path === '/api/watchdog/flags/suppress')
  const body = posted?.body as { run_id?: string; anomaly_class: string }
  const stored = flag.card as { run_id?: string; anomaly_class: string }
  expect(body.anomaly_class, 'the class was not passed through verbatim').toBe(stored.anomaly_class)
  expect(body.run_id).toBe(stored.run_id)
  expect(row(view, flag.id).textContent).toContain('proposes a threshold raise')
})

test('the watchdog card carries the resume door, and its refusal renders verbatim', async () => {
  const served = mine()
  const flag = card(served, 'watchdog_flag:r-ship\x1fwatchdog.loop')
  expect(flag.actions, 'the run-scoped flag card lost its resume door').toEqual(['suppress', 'resume'])

  const { view, log } = await open('/inbox', served)
  log.set('POST /api/runs/r-ship/resume', {
    status: 409,
    body: {
      error: 'conflict',
      detail: 'this run is parked on an OPEN ask — answer it in the inbox and the run resumes itself',
    },
  })
  click(row(view, flag.id).querySelector('button[data-action="resume"]'))
  await flush()
  expect(row(view, flag.id).textContent).toContain('parked on an OPEN ask')

  // A run-less platform flag has no run to resume, and offers no such door.
  const runless = card(all(), 'watchdog_flag:\x1fwatchdog.spend:alice')
  expect(runless.actions).toEqual(['suppress'])
})

test('a successful resume renders the SERVED outcome, and authors no story of its own', async () => {
  const served = mine()
  const flag = card(served, 'watchdog_flag:r-ship\x1fwatchdog.loop')
  const { view, log } = await open('/inbox', served)
  log.set('POST /api/runs/r-ship/resume', {
    body: {
      run_id: 'r-ship',
      from: 'parked',
      to: 'running',
      applied: true,
      generation: 3,
      detail: 'resumed by a person: the run is running again and its generation bumped to 3',
    },
  })
  click(row(view, flag.id).querySelector('button[data-action="resume"]'))
  await flush()
  const text = row(view, flag.id).querySelector('[data-outcome="applied"]')?.textContent ?? ''
  expect(text).toContain('parked → running')
  expect(text).toContain('generation bumped to 3')
  // The first cut printed a judgement the platform never made.
  expect(text, 'the view authored a story about WHY the run was released').not.toContain('false alarm')
  expect(log.calls.some((c) => c.method === 'POST' && c.path === '/api/runs/r-ship/resume')).toBe(true)
})

test('a conformance acknowledgement renders as STILL RED, never as a pass', async () => {
  const served = all()
  const { view, log } = await open('/inbox', served)
  const id = 'conformance_card:api-read-surface'
  log.set(`POST ${verb(id, 'acknowledge')}`, {
    body: {
      card_id: id,
      row_id: 'api-read-surface',
      last_run_ts: '2026-07-20T09:02:00Z',
      acknowledged: true,
      still_red: true,
      detail: 'acknowledged: this red STAYS LISTED red. Only a real green suite result clears it',
    },
  })
  click(row(view, id).querySelector('button[data-action="acknowledge"]'))
  await flush()
  const text = row(view, id).textContent ?? ''
  expect(text).toContain('STILL RED')
  expect(text).toContain('Only a real green suite result clears it')
})

test('a drift dismissal carries its reason and renders the incident-window scope', async () => {
  const { view, log } = await open('/inbox', all())
  const id = 'drift_card:fp-anthropic-1'
  const node = row(view, id)
  typeInto(node.querySelector('[data-field="reason"]') as HTMLInputElement, 'read it, nothing we send is affected')
  log.set(`POST ${verb(id, 'dismiss')}`, {
    body: {
      card_id: id,
      fingerprint: 'fp-anthropic-1',
      window_start_seq: 12,
      dismissed: true,
      detail: 'dismissed: the drift.finding rows stay queryable, and a finding in a NEW window opens a new card',
    },
  })
  click(node.querySelector('button[data-action="dismiss"]'))
  await flush()
  expect((log.calls.find((c) => c.path === verb(id, 'dismiss'))?.body as { reason?: string }).reason)
    .toContain('nothing we send is affected')
  expect(row(view, id).textContent).toContain('a finding in a NEW window opens a new card')
})

// ── R9: the blind-pair verdict form ───────────────────────────────────────

test('the verdict form renders ONLY the fields a blind voter may see', async () => {
  const { view } = await open('/inbox', mine())
  const node = row(view, 'benchmark_verdict:bp-notes')
  const forms = fixtures.benchmarkVerdicts() as unknown as BenchmarkVerdictForms
  const pair = (forms.pairs ?? [])[0]

  // The two responses are on screen, keyed by SIDE.
  expect(node.querySelector('[data-side="a"]')?.textContent).toContain(pair.render_a)
  expect(node.querySelector('[data-side="b"]')?.textContent).toContain(pair.render_b)
  expect(node.textContent).toContain(String(pair.length_a))

  // Nothing that could identify an arm is anywhere in the rendered card.
  const text = node.textContent ?? ''
  for (const leak of ['platform-a', 'platform-b', 'position', 'direct_run_id', 'platform_model', 'direct_model']) {
    expect(text, `the pre-record form leaks ${leak}`).not.toContain(leak)
  }
})

test('the verdict cannot be submitted without BOTH the pick and the arm-guess', async () => {
  const { view, log } = await open('/inbox', mine())
  const node = row(view, 'benchmark_verdict:bp-notes')
  const button = () => row(view, 'benchmark_verdict:bp-notes').querySelector('button[data-action="verdict"]') as HTMLButtonElement

  expect(button().disabled, 'an empty form could be submitted').toBe(true)
  click(node.querySelector('[data-verdict="A"]'))
  expect(button().disabled, 'a guess-less verdict could be submitted').toBe(true)
  click(row(view, 'benchmark_verdict:bp-notes').querySelector('[data-guess="B"]'))
  expect(button().disabled).toBe(false)

  log.set(`POST ${verb('benchmark_verdict:bp-notes', 'verdict')}`, {
    body: {
      pair_id: 'bp-notes',
      recorded: true,
      reveal: {
        pair_id: 'bp-notes',
        platform_side: 'B',
        platform_model: 'claude-opus-5',
        direct_model: 'claude-opus-5-direct',
        verdict: 'A',
        platform_guess: 'B',
        guess_correct: true,
        epoch_id: 'e1',
        recorded_ts: '2026-07-20T09:05:00Z',
      },
      detail: 'recorded, then revealed: the §14 record commits with the move to recorded in one transaction',
    },
  })
  click(button())
  await flush()
  expect(log.calls.find((c) => c.path === verb('benchmark_verdict:bp-notes', 'verdict'))?.body).toEqual({
    verdict: 'A',
    guess: 'B',
  })

  // THE REVEAL IS RENDERED, not merely announced. §3.4 withholds arm identity
  // until the record commits; a surface that said "revealed" and printed
  // nothing would withhold the very thing it was announcing.
  const reveal = row(view, 'benchmark_verdict:bp-notes').querySelector('[data-reveal="bp-notes"]')
  expect(reveal, 'the served reveal was dropped').not.toBeNull()
  expect(reveal?.querySelector('[data-reveal-side]')?.textContent).toBe('B')
  expect(reveal?.textContent).toContain('claude-opus-5')
  expect(reveal?.textContent).toContain('claude-opus-5-direct')
  // The §5 blindness measurement, as the platform scored it — never re-scored.
  expect(reveal?.querySelector('[data-guess-correct]')?.getAttribute('data-guess-correct')).toBe('true')
  expect(reveal?.textContent).toContain('right')
})

test('the form renders the vocabularies it is SERVED and declares none of its own', async () => {
  const forms = fixtures.benchmarkVerdicts() as unknown as BenchmarkVerdictForms
  const { view } = await open('/inbox', mine())
  const node = row(view, 'benchmark_verdict:bp-notes')
  expect([...node.querySelectorAll('[data-verdict]')].map((n) => n.getAttribute('data-verdict'))).toEqual(forms.choices)
  expect([...node.querySelectorAll('[data-guess]')].map((n) => n.getAttribute('data-guess'))).toEqual(forms.guess_sides)
})

test('a recorded verdict whose reveal could not be read back renders as late-reveal, not as a failure', async () => {
  const { view, log } = await open('/inbox', mine())
  const node = row(view, 'benchmark_verdict:bp-notes')
  click(node.querySelector('[data-verdict="tie"]'))
  click(row(view, 'benchmark_verdict:bp-notes').querySelector('[data-guess="A"]'))
  log.set(`POST ${verb('benchmark_verdict:bp-notes', 'verdict')}`, {
    body: {
      pair_id: 'bp-notes',
      recorded: true,
      detail: 'your verdict is recorded; the arm identities could not be read back just now',
    },
  })
  click(row(view, 'benchmark_verdict:bp-notes').querySelector('button[data-action="verdict"]'))
  await flush()
  const text = row(view, 'benchmark_verdict:bp-notes').querySelector('[data-outcome="applied"]')?.textContent ?? ''
  expect(text).toContain('recorded')
  expect(text).toContain('could not be read back')
  // No reveal was served, so none is rendered — the honest late-reveal branch.
  expect(row(view, 'benchmark_verdict:bp-notes').querySelector('[data-reveal]')).toBeNull()
})

test('decline is first-class beside the verdict, and the failed pair offers ONLY decline', async () => {
  const served = mine()
  const { view, log } = await open('/inbox', served)
  const pending = [...row(view, 'benchmark_verdict:bp-notes').querySelectorAll('button[data-action]')].map((b) =>
    b.getAttribute('data-action'),
  )
  expect(pending).toEqual(['verdict', 'decline'])

  const failed = [...row(view, 'benchmark_verdict:bp-archive').querySelectorAll('button[data-action]')].map((b) =>
    b.getAttribute('data-action'),
  )
  expect(failed, 'a pair that can never be shown was offered a vote').toEqual(['decline'])

  log.set(`POST ${verb('benchmark_verdict:bp-archive', 'decline')}`, {
    body: { pair_id: 'bp-archive', declined: true, detail: 'declined, and LOGGED: selective participation is visible' },
  })
  click(row(view, 'benchmark_verdict:bp-archive').querySelector('button[data-action="decline"]'))
  await flush()
  expect(row(view, 'benchmark_verdict:bp-archive').textContent).toContain('selective participation is visible')
})

test('the alarm card disposes with a SERVED disposition and a reason', async () => {
  const served = all()
  const forms = fixtures.benchmarkVerdicts() as unknown as BenchmarkVerdictForms
  const id = 'benchmark_alarm:blind-pairs#e1'
  const { view, log } = await open('/inbox', served)
  const node = row(view, id)
  expect([...node.querySelectorAll('[data-disposition]')].map((n) => n.getAttribute('data-disposition'))).toEqual(
    forms.dispositions,
  )

  const chosen = (forms.dispositions ?? [])[0]
  click(node.querySelector(`[data-disposition="${CSS.escape(chosen)}"]`))
  typeInto(row(view, id).querySelector('[data-field="reason"]') as HTMLInputElement, 'looking at it this week')
  log.set(`POST ${verb(id, 'dispose')}`, {
    body: {
      card_id: id,
      domain: 'blind-pairs',
      disposition: chosen,
      cleared: true,
      detail: 'dispositioned and logged as a human decision, with the clear kept as alarm history',
    },
  })
  click(row(view, id).querySelector('button[data-action="dispose"]'))
  await flush()
  expect(log.calls.find((c) => c.path === verb(id, 'dispose'))?.body).toEqual({
    disposition: chosen,
    reason: 'looking at it this week',
  })
  expect(row(view, id).textContent).toContain('kept as alarm history')
})

// ── R10: the ninth kind ───────────────────────────────────────────────────

test('the addressee sees their memory-conflict card and can resolve it', async () => {
  const served = mine()
  const conflict = card(served, 'memory_conflict:1')
  const { view, log } = await open('/inbox', served)
  const node = row(view, conflict.id)

  const stored = conflict.card as { question: string; entry_id: string; other_entry_id: string }
  expect(node.textContent).toContain(stored.question)
  expect(node.textContent).toContain(stored.entry_id)
  expect(node.textContent).toContain(stored.other_entry_id)
  expect([...node.querySelectorAll('button[data-action]')].map((b) => b.getAttribute('data-action'))).toEqual([
    'resolve',
  ])

  log.set('POST /api/memory/conflicts/1/resolve', {
    body: { conflict: { conflict_id: 1, status: 'resolved' }, detail: 'this question is closed and already closed' },
  })
  click(node.querySelector('button[data-action="resolve"]'))
  await flush()
  expect(row(view, conflict.id).textContent).toContain('already closed')
})

test('a conflict card with no readable id does not arm its control, and posts nothing', async () => {
  const served = mine()
  const broken: ApprovalList = {
    ...served,
    items: served.items.map((i) => (i.kind === 'memory_conflict' ? { ...i, card: { question: 'which is right?' } } : i)),
  }
  const { view, log } = await open('/inbox', broken)
  const button = row(view, 'memory_conflict:1').querySelector('button[data-action="resolve"]') as HTMLButtonElement
  expect(button.disabled, 'the control armed on a card with no conflict id').toBe(true)
  click(button)
  await flush()
  // A disabled control fires nothing, and in particular never posts id 0.
  expect(log.calls.filter((c) => c.method === 'POST'), 'a zero-value id was posted').toHaveLength(0)
})

test('nobody else sees a conflict card — the operator included, deliberately', async () => {
  // The operator's own served body is the evidence: the card is not in it.
  const operatorsInbox = all()
  expect(
    operatorsInbox.items.some((i) => i.kind === 'memory_conflict'),
    "a memory-conflict card reached the operator's read",
  ).toBe(false)
  const { view } = await open('/inbox', operatorsInbox)
  expect(view.container.querySelectorAll('[data-kind="memory_conflict"]')).toHaveLength(0)
  // …and the honest no-frame note only appears where the kind does.
  expect(view.container.querySelector('[data-note="no-live-frame"]')).toBeNull()
})

test('the ninth kind says how it stays current, and nothing polls for it', async () => {
  const { view } = await open('/inbox', mine())
  expect(view.container.querySelector('[data-note="no-live-frame"]')?.textContent).toContain('nothing pushes')
})

// ── R11: deep links ───────────────────────────────────────────────────────

test('every card id round-trips through /inbox/:id — including the ones with : and # and a separator', async () => {
  const served = mine()
  const { view } = await open('/inbox', served)
  const hrefs = new Map(
    [...view.container.querySelectorAll('a.card-id')].map((a) => [a.textContent, a.getAttribute('href')]),
  )

  for (const id of ['watchdog_flag:r-ship\x1fwatchdog.loop', 'ask:ask-ship', 'memory_conflict:1']) {
    const href = hrefs.get(id)
    expect(href, `no deep link for ${id}`).toBeTruthy()
    expect(href, 'a card id was concatenated into a path instead of encoded').toBe(
      `/inbox/${encodeURIComponent(id)}`,
    )
  }

  // And the round trip really lands on the card: navigating to the encoded
  // path renders that one card and no other.
  const target = 'watchdog_flag:r-ship\x1fwatchdog.loop'
  const { view: v2 } = await open(`/inbox/${encodeURIComponent(target)}`, served)
  expect(rowIDs(v2)).toEqual([target])
})

test('an alarm id carrying a # round-trips too', async () => {
  const served = all()
  const target = 'benchmark_alarm:blind-pairs#e1'
  const { view } = await open(`/inbox/${encodeURIComponent(target)}`, served)
  expect(rowIDs(view)).toEqual([target])
})

test('a card id that is not in your queue renders the honest not-visible state', async () => {
  const { view } = await open('/inbox/ask%3Anot-yours', mine())
  const text = view.container.textContent ?? ''
  expect(text).toContain('ask:not-yours')
  expect(text).toContain('It may have been answered already, or it may belong to somebody else')
  expect(view.container.querySelectorAll('[data-card-id]')).toHaveLength(0)
})

// ── R12: liveness ─────────────────────────────────────────────────────────

test('a declared frame re-reads the queue, and the re-snapshot debt settles on the read', async () => {
  const routes: Record<string, Scripted> = {
    ...oversightRoutes(),
    'GET /api/auth/session': signedIn,
    'GET /api/approvals': { body: mine() },
  }
  const log = scriptedFetch(routes)
  window.history.replaceState(null, '', '/inbox')
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
  expect(stream.status, 'a view still owes a re-snapshot after its read landed').toBe('live')

  const before = log.calls.filter((c) => c.path === '/api/approvals').length
  act(() =>
    FakeSource.last().send('decision.recorded', {
      seq: 900,
      user_id: 'alice',
      type: 'decision.recorded',
      schema_version: 1,
      topics: ['inbox'],
      payload: {},
      ts: '2026-07-20T09:10:00Z',
    }),
  )
  await flush()
  expect(
    log.calls.filter((c) => c.path === '/api/approvals').length,
    'a decision frame did not re-read the queue',
  ).toBeGreaterThan(before)
  view.unmount()
})

test('an empty queue says so rather than rendering nothing', async () => {
  const { view } = await open('/inbox', { items: [], cursor: 4, truncated: false })
  expect(view.container.textContent).toContain('Nothing is waiting on you.')
})

// ── the automation-step proposal (B6-8 part B's arrival in this queue) ───────

test("an automation's station-3 proposal renders through the generic card, and what that looks like is pinned", async () => {
  // This card arrived with the workforce map's fixture world: validating a
  // kind=automation worker journals its outward step as a gated proposal
  // (S08.9; D7 makes it free), so the queue genuinely gains one. `Inbox.tsx`
  // has no arm for `card.kind: "automation-step"` — it falls to the generic
  // RowCard — and NOTHING asserted on it, which is how a card can render
  // badly beside a live PIN field and armed buttons without anyone noticing.
  //
  // Growing B6-6's card vocabulary is a gate question and is NOT done here.
  // What is done here is making the current render a checked fact rather than
  // an unlooked-at one, so the gate decides against evidence.
  const served = all()
  const { view } = await open('/inbox', served)
  const item = card(served, 'effect:e-digest')

  const row = view.container.querySelector('[data-card-id="effect:e-digest"]')
  expect(row, 'the automation-step proposal does not render at all').not.toBeNull()
  expect(row!.getAttribute('data-kind')).toBe('effect')

  // What the card IS, from the served body: a High-tier outward step on one
  // named service, owned by the person whose worker it is.
  expect(item.tier).toBe('high')
  expect(item.owner).toBe('bob')
  expect(item.step_up_required).toBe(true)
  expect(item.actions).toEqual(['approve', 'deny'])

  // What renders: the generic fallback shows the payload's own fields. The
  // verb and the step are the two facts that say what would happen, and both
  // reach the screen — so the fallback is legible enough to decide against,
  // even though it is not a purpose-built arm.
  const text = row!.textContent ?? ''
  for (const fact of ['calendar.post', 'post', 'calendar']) {
    expect(text, `the card does not show ${fact}, so a person cannot tell what they are approving`).toContain(fact)
  }
})
