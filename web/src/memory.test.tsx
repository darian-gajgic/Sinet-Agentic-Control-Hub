import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import type { MemoryEntryDetail, MemoryList } from './api'
import { FakeSource, fixtures, memoryEntryID, memoryOtherEntryID, memoryRoutes, scriptedFetch } from './doubles'
import { EventStream } from './events'
import { Memory, MemoryEntryView } from './Memory'
import { choose, click, flush, mount, typeInto } from './testing'

/**
 * The S09 memory family's workspace surface (P3-UI-3; S09; S15.2 memory row;
 * S09.4's v0 activation row).
 *
 * WHERE THE BODIES COME FROM. `fixtures.memory()`, `fixtures.memoryOperator()`
 * and `fixtures.memoryEntry()` are the committed golden bodies an internal/api
 * test asserts the real handlers still serve — written by the REAL S09 store and
 * gate, conflict detection and question text included. Everything hand-scripted
 * below is an error contract or an entry state the fixture world's producers
 * cannot be made to hold reproducibly, and each one CITES the Go source it was
 * transcribed from, because a probe that drifts from its handler is worse than
 * no probe at all.
 *
 * THE AUTHORITY LEGS ARE FIRED, NOT ONLY RENDERED. The four gate verbs have
 * three DIFFERENT doors between them, so each is driven in both directions: the
 * owner's own act, the operator's act on somebody else's entry, and the refusal
 * the door actually gives.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

/** The committed member body, as its own type. */
const memberList = () => fixtures.memory() as unknown as MemoryList
const operatorList = () => fixtures.memoryOperator() as unknown as MemoryList
const entryDetail = () => fixtures.memoryEntry() as unknown as MemoryEntryDetail

/** The two entries the committed bodies carry. `a` is the one whose detail is
 *  committed; both are alice's own, user-scope, active. */
const entryA = () => entryDetail().entry

/** browse mounts the browse surface directly, so identity is a PROP and the
 *  per-verb predicates are driven explicitly rather than through a session
 *  double whose default is alice-as-operator. */
async function browse(
  routes: Record<string, { body?: unknown; status?: number }>,
  who: { me: string; operator: boolean } = { me: 'alice', operator: false },
) {
  const log = scriptedFetch(routes)
  const view = mount(<Memory me={who.me} operator={who.operator} stream={inertStream()} />)
  await flush()
  return { view, log }
}

async function detail(
  routes: Record<string, { body?: unknown; status?: number }>,
  who: { me: string; operator: boolean } = { me: 'alice', operator: false },
  id = memoryEntryID,
) {
  const log = scriptedFetch(routes)
  const view = mount(<MemoryEntryView id={id} me={who.me} operator={who.operator} stream={inertStream()} />)
  await flush()
  return { view, log }
}

const detailRoute = (body: unknown, status?: number) => ({ [`GET /api/memory/${memoryEntryID}`]: { body, status } })

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── the route contract ─────────────────────────────────────────────────────

test('/memory and /memory/:id mount their own surfaces, not the stub', async () => {
  scriptedFetch(memoryRoutes())
  window.history.replaceState(null, '', '/memory')
  const list = mount(<App stream={inertStream()} />)
  await flush()
  expect(list.container.querySelector('h1')?.textContent).toBe('Memory')
  expect(list.container.textContent, 'the route fell through to the stub').not.toContain('not built yet')
  // The nav carries it, inside a labelled group (the shell derives both from
  // the table, so this is the addition arriving at the surface).
  const hrefs = [...list.container.querySelectorAll('.shell-nav a')].map((a) => a.getAttribute('href'))
  expect(hrefs).toContain('/memory')
  list.unmount()

  window.history.replaceState(null, '', `/memory/${memoryEntryID}`)
  const one = mount(<App stream={inertStream()} />)
  await flush()
  expect(one.container.querySelector('h1')?.textContent).toBe('Memory entry')
  expect(one.container.textContent).toContain('release notes, second thoughts')
  one.unmount()
})

// ── browse ─────────────────────────────────────────────────────────────────

test('the browse renders the SERVED visibility rule verbatim, and the membership it rests on', async () => {
  const { view } = await browse({ 'GET /api/memory': { body: fixtures.memory() } })
  const rule = view.container.querySelector('[data-visibility-rule="served"]')
  expect(rule?.textContent, 'the scope rule is not the served sentence').toBe(memberList().visibility)
  // Not a paraphrase: the served sentence names the operator limb that does NOT
  // exist, and the surface prints that rather than summarising it away.
  expect(rule?.textContent).toContain('The operator role bit does not open another person')
  expect(view.container.querySelector('[data-projects="served"]')?.textContent).toContain('release-notes')
  view.unmount()
})

test('a row renders the served record: chips, owner, version and a primitive timestamp', async () => {
  const { view } = await browse({ 'GET /api/memory': { body: fixtures.memory() } })
  const rows = [...view.container.querySelectorAll('[data-entry-id]')]
  expect(rows).toHaveLength(memberList().entries.length)

  const row = view.container.querySelector(`[data-entry-id="${memoryEntryID}"]`)!
  expect(row.getAttribute('data-owner')).toBe('alice')
  expect(row.getAttribute('data-status')).toBe('active')
  expect(row.querySelector('[data-chip="kind"]')?.textContent).toBe('lesson')
  expect(row.querySelector('[data-chip="scope"]')?.textContent).toBe('user')
  expect(row.querySelector('[data-chip="status"]')?.textContent).toBe('active')
  expect(row.querySelector('a')?.getAttribute('href')).toBe(`/memory/${memoryEntryID}`)
  // Owned by the reader: "yours" rather than a repeated id.
  expect(row.textContent).toContain('yours')
  // The timestamp is the PRIMITIVE's, carrying the served UTC in dateTime —
  // never an inline formatted date (the sweep pins which files may render one).
  expect(row.querySelector('time')?.getAttribute('datetime')).toBe(entryA().updated_ts)
  view.unmount()
})

test('the filters are SERVER parameters over the closed S09.2 vocabularies', async () => {
  const { view, log } = await browse({
    'GET /api/memory': { body: fixtures.memory() },
    'GET /api/memory?status=active': { body: fixtures.memory() },
    'GET /api/memory?scope=house&status=active': { body: { ...fixtures.memory(), entries: [] } },
  })
  choose(view.container.querySelector('[data-filter="status"]')!, 'active')
  await flush()
  expect(log.calls.some((c) => c.path === '/api/memory?status=active'), 'the filter did not reach the request').toBe(true)

  choose(view.container.querySelector('[data-filter="scope"]')!, 'house')
  await flush()
  expect(log.calls.some((c) => c.path === '/api/memory?scope=house&status=active')).toBe(true)
  // Every offered value is one the handler's own closed map admits
  // (internal/api/memory.go:68–84); an unknown one is a 400 naming the
  // vocabulary, so a select that offered more would be offering a refusal.
  const scopes = [...view.container.querySelectorAll('[data-filter="scope"] option')].map((o) => o.getAttribute('value'))
  expect(scopes).toEqual(['', 'user', 'project', 'house', 'worker_overlay'])
  const kinds = [...view.container.querySelectorAll('[data-filter="kind"] option')].map((o) => o.getAttribute('value'))
  expect(kinds).toEqual([
    '',
    'lesson',
    'preference',
    'playbook',
    'rubric',
    'style',
    'exemplar',
    'convention',
    'observation',
    'taxonomy',
    'trigger_rules',
  ])
  view.unmount()
})

test('truncation is stated honestly and never given a count the read did not serve', async () => {
  const { view } = await browse({ 'GET /api/memory': { body: { ...fixtures.memory(), truncated: true } } })
  const note = view.container.querySelector('[data-truncated="true"]')
  expect(note?.textContent).toContain('one page of a longer set')
  expect(note?.textContent, 'a count was invented for a read that serves none').toMatch(/does not say how many more/)
  view.unmount()
})

test('an empty answer teaches what would appear here rather than saying nothing', async () => {
  const { view } = await browse({ 'GET /api/memory': { body: { ...fixtures.memory(), entries: [] } } })
  expect(view.container.textContent).toContain('No entry matches this reading.')
  expect(view.container.textContent).toContain('Entries appear here when somebody writes one')
  view.unmount()
})

test('the family being unwired renders the SERVER’s own sentence, not a blank page', async () => {
  // internal/api/memory.go:695–701 — one family, one readiness.
  const { view } = await browse({
    'GET /api/memory': {
      status: 503,
      body: { error: 'not_wired', detail: 'the S09 memory store and its write gate are not wired in this process' },
    },
  })
  expect(view.container.querySelector('.error')?.textContent).toContain(
    'the S09 memory store and its write gate are not wired in this process',
  )
  view.unmount()
})

// ── the content line, in the operator direction ────────────────────────────

test('the OPERATOR’s own served browse carries no member user-scope entry, and the surface adds none', async () => {
  // This body is the real handler's answer to `GET /api/memory` as the
  // operator, produced by the fixture world's real store: alice's two entries
  // exist in that database and are absent from it. Memory is personal CONTENT;
  // D10 is authority over the HOUSE scope, and §30's operator-sees-all is a rule
  // about runs and telemetry that does not reach here.
  const served = operatorList()
  expect(served.entries, 'the committed operator body is not the negative it is here to be').toEqual([])

  const { view } = await browse({ 'GET /api/memory': { body: fixtures.memoryOperator() } }, { me: 'op', operator: true })
  expect(view.container.querySelectorAll('[data-entry-id]')).toHaveLength(0)
  expect(view.container.textContent, 'the surface synthesised a row the read did not serve').not.toContain('release notes')
  // And the rule is still stated in the server's words.
  expect(view.container.querySelector('[data-visibility-rule="served"]')?.textContent).toBe(served.visibility)
  view.unmount()
})

test('a member is offered no control over anybody else’s entry', async () => {
  // bob reads a house entry alice owns: he may see it (house defaults address
  // everyone, S4.5) and he owns nothing about it, so the surface offers him
  // neither an edit, nor a retire, nor a delete.
  const house = {
    entry: { ...entryA(), entry_id: 'k-house', owner: 'alice', scope: 'house', title: 'house convention' },
    conflicts: [],
    cursor: 89,
  }
  const { view } = await detail(detailRoute(house), { me: 'bob', operator: false })
  expect(view.container.querySelector('[data-control="memory-edit"]')).toBeNull()
  expect(view.container.querySelector('[data-control="memory-retire"]')).toBeNull()
  expect(view.container.querySelector('[data-control="memory-delete"]')).toBeNull()
  expect(view.container.querySelector('[data-verb-absent="delete"]')?.textContent).toContain(
    "its owner's own right and nobody else's",
  )
  view.unmount()
})

// ── the entry ──────────────────────────────────────────────────────────────

test('the entry renders the whole served record, absences included', async () => {
  const { view } = await detail(detailRoute(fixtures.memoryEntry()))
  const e = entryA()
  const text = view.container.textContent ?? ''
  expect(text).toContain(e.title)
  expect(view.container.querySelector('[data-content="served"]')?.textContent).toBe(e.content)
  expect(text).toContain(e.entry_id)
  expect(text).toContain('release-notes-style')

  // Provenance, as the gate recorded it: the manual write's actor is both
  // approver and verifier at that instant (§40-C tier reading).
  const prov = view.container.querySelector('[data-block="provenance"]')!
  expect(prov.textContent).toContain('human_direct')
  expect(prov.textContent).toContain('alice')
  expect(prov.querySelector('time')?.getAttribute('datetime')).toBe(e.provenance.approved_ts)

  // A ZERO interval is a real answer and says so in words (memory.go:143–150).
  const ver = view.container.querySelector('[data-block="verification"]')!
  expect(ver.querySelector('[data-interval="none"]')?.textContent).toBe(
    'no re-verify interval unless a human flags one',
  )
  // Never injected is an absence with its reason, never the number 0.
  expect(ver.textContent).toContain('never injected into a run')
  expect(ver.textContent, 'an absent last-injected stamp rendered as a figure').not.toMatch(/\b0\b/)

  // No selectors is a real answer too.
  expect(view.container.querySelector('[data-block="selectors"]')?.textContent).toContain('no selectors')
  view.unmount()
})

test('a file-backed entry names its ref and its commit and fetches no bytes', async () => {
  // internal/api/memory.go:106–129: content and file_ref are exclusive by
  // construction — the row is the index, the file is the content, git is the
  // history (S09.2).
  const backed = {
    entry: {
      ...entryA(),
      content: undefined,
      file_ref: 'knowledge/user/alice/release-notes.md',
      file_commit: '9f1c2ab',
    },
    conflicts: [],
    cursor: 89,
  }
  const { view, log } = await detail(detailRoute(backed))
  const block = view.container.querySelector('[data-file-backed="true"]')!
  expect(block.textContent).toContain('knowledge/user/alice/release-notes.md')
  expect(block.textContent).toContain('9f1c2ab')
  expect(block.textContent).toContain('git holds the history')
  // No second read was invented to go and get the bytes.
  expect(log.calls.filter((c) => c.method === 'GET')).toHaveLength(1)
  expect(view.container.querySelector('[data-content="served"]')).toBeNull()
  view.unmount()
})

test('a tombstone renders as the stub it is, and offers no verb', async () => {
  // internal/api/memory.go:534–556: after a true deletion the wire carries no
  // content and no title — id, dates and the note (G2 Def.9).
  const stub = {
    entry: {
      ...entryA(),
      title: '',
      content: undefined,
      topic_key: undefined,
      status: 'removed',
      tombstone: true,
      tombstone_note: 'removed at owner request',
    },
    conflicts: [],
    cursor: 89,
  }
  const { view } = await detail(detailRoute(stub))
  expect(view.container.querySelector('[data-tombstone-note="served"]')?.textContent).toBe('removed at owner request')
  expect(view.container.textContent, 'the surface filled a purged title back in').not.toContain(
    'release notes, second thoughts',
  )
  expect(view.container.querySelector('[data-verbs="none"]')).not.toBeNull()
  expect(view.container.querySelector('[data-control="memory-retire"]')).toBeNull()
  view.unmount()
})

// ── the S09.7 edges ────────────────────────────────────────────────────────

test('a conflict edge renders the detection’s own question and points at the one place it is answered', async () => {
  const { view, log } = await detail(detailRoute(fixtures.memoryEntry()))
  const served = entryDetail().conflicts[0]
  const card = view.container.querySelector(`[data-conflict-id="${served.conflict_id}"]`)!
  expect(card.querySelector('[data-question="served"]')?.textContent, 'the question was paraphrased').toBe(
    served.question,
  )
  const links = [...card.querySelectorAll('a')].map((a) => a.getAttribute('href'))
  expect(links).toContain(`/memory/${memoryOtherEntryID}`)
  expect(links, 'the edge does not point at the card that answers it').toContain(
    `/inbox/memory_conflict%3A${served.conflict_id}`,
  )
  // The ninth kind's verb keeps its ONE home: this surface links, and never
  // resolves (§40-C OQ6).
  expect(log.calls.some((c) => c.path.includes('/resolve')), 'this surface fired the inbox’s verb').toBe(false)
  view.unmount()
})

test('an edge addressed to somebody else is an honest ABSENCE, not something to compensate for', async () => {
  // internal/api/memory.go:289–310 (drain D2): the served list carries only the
  // OPEN edges the caller is the affected owner of. A writer whose write raised
  // a question for somebody else does not see it, because that card is theirs.
  const { view } = await detail(detailRoute({ entry: entryA(), conflicts: [], cursor: 89 }))
  expect(view.container.querySelector('[data-block="conflicts"]')).toBeNull()
  expect(view.container.textContent, 'the surface invented an edge the read did not serve').not.toContain(
    'Open questions about this entry',
  )
  view.unmount()
})

// ── create ─────────────────────────────────────────────────────────────────

async function openComposer(routes: Record<string, { body?: unknown; status?: number }>, operator = false) {
  const { view, log } = await browse(routes, { me: 'alice', operator })
  click(view.container.querySelector('[data-open="create"]'))
  await flush()
  return { view, log }
}

function fillDraft(title = 'how I like changelogs', content = 'newest first, one line each') {
  const dialog = document.querySelector('[role="dialog"]')!
  typeInto(dialog.querySelector('[data-field="title"]') as HTMLInputElement, title)
  typeInto(dialog.querySelector('[data-field="content"]') as HTMLTextAreaElement, content)
  return dialog
}

test('the create fires nothing until the act is pressed, and then exactly once', async () => {
  const { view, log } = await openComposer({
    'GET /api/memory': { body: fixtures.memory() },
    'POST /api/memory': { body: fixtures.memoryEntry() },
  })
  fillDraft()
  const posts = () => log.calls.filter((c) => c.method === 'POST').length
  expect(posts(), 'opening the form fired a write').toBe(0)
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  expect(posts()).toBe(1)
  view.unmount()
})

test('the create body is the landed write body and nothing beyond it', async () => {
  const { view, log } = await openComposer({
    'GET /api/memory': { body: fixtures.memory() },
    'POST /api/memory': { body: fixtures.memoryEntry() },
  })
  const dialog = fillDraft()
  choose(dialog.querySelector('[data-field="kind"]') as HTMLSelectElement, 'preference')
  typeInto(dialog.querySelector('[data-field="topic_key"]') as HTMLInputElement, 'changelog-style')
  typeInto(dialog.querySelector('[data-field="triggers"]') as HTMLInputElement, 'changelog, release notes')
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  const sent = log.calls.find((c) => c.method === 'POST')!.body as Record<string, unknown>
  expect(sent).toEqual({
    scope: 'user',
    kind: 'preference',
    title: 'how I like changelogs',
    content: 'newest first, one line each',
    topic_key: 'changelog-style',
    selectors: { triggers: ['changelog', 'release notes'] },
  })
  // The two named narrowings, asserted as absences: `origin` omitted IS
  // human_direct (internal/api/memory.go:431–433), and the N20 import ceremony
  // and file-backed authoring get no UI at v0.
  expect(Object.keys(sent), 'this surface named an origin ceremony').not.toContain('origin')
  expect(Object.keys(sent)).not.toContain('file_backed')
  view.unmount()
})

test('the kind list offers the nine a v0 write may name, and never the dormant L1 one', async () => {
  const { view } = await openComposer({ 'GET /api/memory': { body: fixtures.memory() } })
  const kinds = [...document.querySelectorAll('[data-field="kind"] option')].map((o) => o.getAttribute('value'))
  expect(kinds).toEqual([
    'lesson',
    'preference',
    'playbook',
    'rubric',
    'style',
    'exemplar',
    'convention',
    'taxonomy',
    'trigger_rules',
  ])
  // The gate refuses `observation` — "memory: no L1 writer is active at v0"
  // (internal/memory/memory.go:180) — and that refusal IS the dormancy.
  expect(kinds).not.toContain('observation')
  view.unmount()
})

test('the scope options mirror the doors: project only where there is one, house only for the operator', async () => {
  // A member with a served project membership.
  const { view: member } = await openComposer({ 'GET /api/memory': { body: fixtures.memory() } })
  let scopes = [...document.querySelectorAll('[data-field="scope"] option')].map((o) => o.getAttribute('value'))
  expect(scopes).toEqual(['user', 'project'])
  // And the project itself comes FROM the served list, never free text.
  choose(document.querySelector('[data-field="scope"]') as HTMLSelectElement, 'project')
  await flush()
  const refs = [...document.querySelectorAll('[data-field="scope_ref"] option')].map((o) => o.getAttribute('value'))
  expect(refs).toEqual(['', 'release-notes'])
  expect(document.querySelector('[data-field="scope_ref"]')?.tagName).toBe('SELECT')
  member.unmount()
  document.querySelectorAll('body > div').forEach((n) => n.remove())

  // A member with NO project membership is not offered the scope at all.
  const { view: alone } = await openComposer({
    'GET /api/memory': { body: { ...fixtures.memory(), projects: [] } },
  })
  scopes = [...document.querySelectorAll('[data-field="scope"] option')].map((o) => o.getAttribute('value'))
  expect(scopes).toEqual(['user'])
  alone.unmount()
  document.querySelectorAll('body > div').forEach((n) => n.remove())

  // The operator, and only the operator, may write the house scope (D10).
  const { view: op } = await openComposer({ 'GET /api/memory': { body: fixtures.memory() } }, true)
  scopes = [...document.querySelectorAll('[data-field="scope"] option')].map((o) => o.getAttribute('value'))
  expect(scopes).toEqual(['user', 'project', 'house'])
  op.unmount()
})

test('N7 — an invalid form states its reason and fires NOTHING, and busy means only in flight', async () => {
  let release: (v: unknown) => void = () => {}
  const { view, log } = await openComposer({
    'GET /api/memory': { body: fixtures.memory() },
    'POST /api/memory': { body: fixtures.memoryEntry() },
  })

  // (a) invalid: the reason renders, the act is held, and the act button is NOT
  // busy-styled — the two signals are separate, which is the whole finding.
  const held = () => document.querySelector('[data-held="true"]')?.textContent ?? ''
  const confirm = () => document.querySelector('[data-act="confirm"]') as HTMLButtonElement
  expect(held()).toContain('An entry needs a title')
  expect(confirm().disabled, 'a held act was left pressable').toBe(true)
  expect(confirm().getAttribute('data-busy'), 'an unfilled form was reported as a request in flight').toBe('false')
  click(confirm())
  await flush()
  expect(log.calls.filter((c) => c.method === 'POST'), 'a held act fired anyway').toHaveLength(0)

  // The reason moves to the NEXT thing missing, so it is about the form rather
  // than a single field.
  typeInto(document.querySelector('[data-field="title"]') as HTMLInputElement, 'a title')
  expect(held()).toContain('An entry needs something to remember')

  // Project scope with no project chosen is the third reason.
  typeInto(document.querySelector('[data-field="content"]') as HTMLTextAreaElement, 'a body')
  expect(held()).toBe('')
  choose(document.querySelector('[data-field="scope"]') as HTMLSelectElement, 'project')
  await flush()
  expect(held()).toContain('has to name its project')
  choose(document.querySelector('[data-field="scope_ref"]') as HTMLSelectElement, 'release-notes')
  await flush()
  expect(held()).toBe('')

  // (b) in flight: busy is true, the act is disabled, and no invalid reason is
  // showing — the request is out, so "why can't I press this" has one answer.
  log.set('POST /api/memory', { body: fixtures.memoryEntry() })
  const slow = new Promise((resolve) => {
    release = resolve
  })
  const realFetch = globalThis.fetch
  vi.stubGlobal('fetch', async (input: string | URL | Request, init?: RequestInit) => {
    if ((init?.method ?? 'GET') === 'POST') await slow
    return (realFetch as typeof fetch)(input, init)
  })
  click(confirm())
  await flush()
  // The dialog closes when the act fires (the landed confirm idiom), so busy is
  // read where it stays visible: the door that opened it.
  const door = () => view.container.querySelector('[data-open="create"]') as HTMLButtonElement
  expect(door().getAttribute('data-busy'), 'a request in flight was not reported as busy').toBe('true')
  expect(door().disabled).toBe(true)
  expect(view.container.querySelector('[data-held="true"]'), 'busy was rendered as an invalid form').toBeNull()
  release(null)
  await flush()
  expect(door().getAttribute('data-busy'), 'busy outlived the request').toBe('false')
  expect(view.container.querySelector('[data-outcome]')?.getAttribute('data-outcome')).toBe('applied')
  expect(view.container.querySelector('[data-held="true"]'), 'a server answer was replaced by pre-validation').toBeNull()
  view.unmount()
})

test('a write that raised a question shows the question in the same breath', async () => {
  const { view } = await openComposer({
    'GET /api/memory': { body: fixtures.memory() },
    'POST /api/memory': { body: fixtures.memoryEntry() },
  })
  fillDraft()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  const result = view.container.querySelector('[data-write-result="served"]')!
  expect(result.textContent).toContain(memoryEntryID)
  expect(result.textContent).toContain('active')
  // The response's own conflicts[] — addressed to the writer — render here.
  expect(result.querySelector('[data-question="served"]')?.textContent).toBe(entryDetail().conflicts[0].question)
  view.unmount()
})

test.each([
  // Each body is transcribed from the Go source that produces it. The gate owns
  // why it refused; this surface renders the gate's own sentence.
  ['bad_request', 400, 'memory: invalid entry', 'internal/memory/memory.go:164 → memory.go:809–815'],
  ['bad_request', 400, 'memory: worker-overlay scope activates at v1', 'internal/memory/memory.go:177'],
  ['forbidden', 403, 'memory: gate actor is not a known person', 'internal/memory/memory.go:168 — the dev posture'],
  ['forbidden', 403, 'memory: house scope requires the operator', 'internal/memory/memory.go:171 — D10'],
  [
    'forbidden',
    403,
    'project "other" is not one you own or are invited to: project knowledge is shared by scope membership (S09.6), and it enters every run of that project',
    'internal/api/memory.go:485–488',
  ],
])('a refused write renders the gate’s own words: %s', async (code, status, detail) => {
  const { view } = await openComposer({
    'GET /api/memory': { body: fixtures.memory() },
    'POST /api/memory': { status, body: { error: code, detail } },
  })
  fillDraft()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  const line = view.container.querySelector('[data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('failed')
  expect(line.textContent, 'the refusal was paraphrased').toContain(detail)
  expect(line.textContent).toContain(`refused (${code})`)
  view.unmount()
})

// ── edit = a new version ───────────────────────────────────────────────────

test('an edit is a NEW VERSION, says so, and posts the full draft', async () => {
  const next = {
    entry: {
      ...entryA(),
      entry_id: 'k-v2',
      title: 'release notes, third thoughts',
      provenance: { ...entryA().provenance, version: 2, supersedes: memoryEntryID },
    },
    conflicts: [],
    cursor: 90,
  }
  const { view, log } = await detail({
    ...detailRoute(fixtures.memoryEntry()),
    [`POST /api/memory/${memoryEntryID}/new-version`]: { body: next },
  })
  click(view.container.querySelector('[data-open="edit"]'))
  await flush()

  const dialog = document.querySelector('[role="dialog"]')!
  // The form opens PREFILLED from the served entry.
  expect((dialog.querySelector('[data-field="title"]') as HTMLInputElement).value).toBe(entryA().title)
  expect((dialog.querySelector('[data-field="content"]') as HTMLTextAreaElement).value).toBe(entryA().content)
  // And the copy states the S4.1 correction right, without claiming a mutation.
  expect(dialog.textContent).toContain('not an edit in place')
  expect(dialog.textContent).toContain('retired in the same transaction')
  expect(dialog.textContent, 'the confirm let a reader think a correction overwrites something').toContain(
    'Nothing is overwritten and nothing is lost',
  )

  typeInto(dialog.querySelector('[data-field="title"]') as HTMLInputElement, 'release notes, third thoughts')
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  const sent = log.calls.find((c) => c.path.endsWith('/new-version'))!.body as Record<string, unknown>
  expect(sent.title).toBe('release notes, third thoughts')
  expect(sent.content).toBe(entryA().content)
  expect(sent.scope).toBe('user')
  const result = view.container.querySelector('[data-write-result="served"]')!
  expect(result.textContent).toContain('2')
  expect([...result.querySelectorAll('a')].map((a) => a.getAttribute('href'))).toContain(`/memory/${memoryEntryID}`)
  view.unmount()
})

test('the operator may correct another owner’s entry and the co-member may not', async () => {
  const project = {
    entry: { ...entryA(), owner: 'alice', scope: 'project', scope_ref: 'release-notes' },
    conflicts: [],
    cursor: 89,
  }
  // §40-C drain D6: a co-member who can READ a project entry is refused
  // `ErrNotOwner` on a supersession; the operator limb succeeds.
  const { view: co } = await detail(detailRoute(project), { me: 'bob', operator: false })
  expect(co.container.querySelector('[data-control="memory-edit"]')).toBeNull()
  co.unmount()

  const { view: op, log } = await detail(
    {
      ...detailRoute(project),
      [`POST /api/memory/${memoryEntryID}/new-version`]: { body: { entry: project.entry, conflicts: [], cursor: 90 } },
    },
    { me: 'op', operator: true },
  )
  click(op.container.querySelector('[data-open="edit"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  expect(log.calls.some((c) => c.path.endsWith('/new-version')), 'the operator limb never fired').toBe(true)
  expect(op.container.querySelector('[data-outcome]')?.getAttribute('data-outcome')).toBe('applied')
  op.unmount()
})

// ── retire ─────────────────────────────────────────────────────────────────

// internal/api/memory.go:504–506, transcribed byte-exact.
const removedDetail =
  'removed: the entry leaves every future assembly immediately — deterministic selection makes removal exact (S09.5). ' +
  'Its content stays in git history and its row stays for audit; true deletion is the owner’s separate right. ' +
  'The influence list names the runs it was injected into; offering to queue re-verification of the deliverables they touched is the workspace UI’s act over this data (B6-6).'

const removedBody = {
  entry_id: memoryEntryID,
  status: 'removed',
  influence: [
    { run_id: 'r-ship.g1', event_seq: 41 },
    { run_id: 'r-notes.g1', event_seq: 58 },
  ],
  detail: removedDetail.replace(/’/g, "'"),
}

test('retiring says exactly what retiring is — and never that anything is gone', async () => {
  const { view, log } = await detail({
    ...detailRoute(fixtures.memoryEntry()),
    [`POST /api/memory/${memoryEntryID}/remove`]: { body: removedBody },
  })
  click(view.container.querySelector('[data-open="retire"]'))
  await flush()
  const said = document.querySelector('[role="dialog"]')!.textContent ?? ''
  expect(said).toContain('out of every future assembly')
  expect(said).toContain('stays in git history')
  expect(said).toContain('a separate right, and it is yours — its own control is below this one.')
  // The three claims a soft removal must not make.
  expect(said, 'the retire confirm claimed a deletion').not.toContain('gone')
  expect(said).not.toContain('permanently')
  expect(said).not.toContain('undone')

  typeInto(document.querySelector('[data-field="reason"]') as HTMLInputElement, 'superseded')
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  const sent = log.calls.find((c) => c.path.endsWith('/remove'))!.body as Record<string, unknown>
  expect(sent).toEqual({ reason: 'superseded' })
  const line = view.container.querySelector('[data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('applied')
  expect(line.textContent, 'the served detail was not rendered verbatim').toContain(removedBody.detail)

  // The S2.9 forward trace, served verbatim: run ids and event sequences, and
  // no link, because the wire carries no task or deliverable ref to point at.
  const influence = view.container.querySelector('[data-block="influence"]')!
  expect(influence.textContent).toContain('r-ship.g1')
  expect(influence.textContent).toContain('58')
  expect(influence.querySelectorAll('a'), 'a link target was invented for a ref that has none').toHaveLength(0)
  view.unmount()
})

test('an optional reason really is optional', async () => {
  const { view, log } = await detail({
    ...detailRoute(fixtures.memoryEntry()),
    [`POST /api/memory/${memoryEntryID}/remove`]: { body: { ...removedBody, influence: [] } },
  })
  click(view.container.querySelector('[data-open="retire"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  expect(log.calls.find((c) => c.path.endsWith('/remove'))!.body).toEqual({})
  expect(view.container.querySelector('[data-block="influence"]')?.textContent).toContain(
    "no run's trace manifest names this entry",
  )
  view.unmount()
})

test('retiring something already retired is a re-read, not a fault', async () => {
  // internal/memory/memory.go:182 → 409 `conflict` (internal/api/memory.go:816–820).
  const { view } = await detail({
    ...detailRoute(fixtures.memoryEntry()),
    [`POST /api/memory/${memoryEntryID}/remove`]: {
      status: 409,
      body: { error: 'conflict', detail: 'memory: entry is not active' },
    },
  })
  click(view.container.querySelector('[data-open="retire"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  const line = view.container.querySelector('[data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('retry')
  // Retire is a SINGLE-SUBJECT verb, so the default note is true of it.
  expect(line.textContent).toContain('nothing fired — re-read it and decide again')
  expect(line.textContent).toContain('memory: entry is not active')
  view.unmount()
})

test('the operator may retire another owner’s entry', async () => {
  const house = { entry: { ...entryA(), owner: 'alice', scope: 'house' }, conflicts: [], cursor: 89 }
  const { view, log } = await detail(
    {
      ...detailRoute(house),
      [`POST /api/memory/${memoryEntryID}/remove`]: { body: { ...removedBody, influence: [] } },
    },
    { me: 'op', operator: true },
  )
  click(view.container.querySelector('[data-open="retire"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  expect(log.calls.some((c) => c.path.endsWith('/remove'))).toBe(true)
  expect(view.container.querySelector('[data-outcome]')?.getAttribute('data-outcome')).toBe('applied')
  view.unmount()
})

// ── true deletion ──────────────────────────────────────────────────────────

// internal/api/memory.go:548–556, transcribed byte-exact.
const deletedBody = {
  entry_id: memoryEntryID,
  tombstone: true,
  tombstone_note: 'removed at owner request',
  detail:
    'content purged: the working-tree file is gone and the row’s content, title, topic key and selectors are cleared. What remains is the tombstone stub — id, dates, "removed at owner request" (G2 Def.9).'.replace(
      /’/g,
      "'",
    ),
  limits: [
    'the knowledge file’s git history still holds the content until a git-history purge is run, which is an S13 operation on the scope’s knowledge dir'.replace(
      /’/g,
      "'",
    ),
    'the database file releases the purged bytes at the next VACUUM INTO snapshot (S02.1)',
    'artifacts already accepted while this entry was live remain exactly as they were — history is not rewritten (D9, S09.5)',
    'no unlearning-from-weights is needed or possible: no fine-tuning exists anywhere in the loop, so a lesson only ever lived as retrievable, injectable, removable text (S09.5)',
  ],
}

test('the owner’s delete is a distinct control with its own confirm, and the limits render as SERVED data', async () => {
  const { view, log } = await detail({
    ...detailRoute(fixtures.memoryEntry()),
    [`POST /api/memory/${memoryEntryID}/delete`]: { body: deletedBody },
  })
  // Two verbs, two controls, two confirms — never one button with a mode.
  expect(view.container.querySelector('[data-control="memory-retire"]')).not.toBeNull()
  expect(view.container.querySelector('[data-control="memory-delete"]')).not.toBeNull()

  click(view.container.querySelector('[data-open="delete"]'))
  await flush()
  const said = document.querySelector('[role="dialog"]')!.textContent ?? ''
  expect(said).toContain('purges the content')
  expect(said).toContain('a stub')
  expect(said).toContain('a different act from retiring')
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  const line = view.container.querySelector('[data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('applied')
  // The served sentence lives on the PAGE's block and has exactly one home
  // there, so it cannot be taken down by the re-read that follows the act
  // (drain r1, D2) and cannot be printed twice while it is still up.
  expect(view.container.querySelector('[data-purge-detail="served"]')?.textContent).toBe(deletedBody.detail)
  expect(view.container.querySelectorAll('[data-purge-detail="served"]')).toHaveLength(1)
  expect(line.textContent, 'the served sentence is printed in two places').not.toContain(deletedBody.detail)
  const limits = [...view.container.querySelectorAll('[data-limit="served"]')].map((l) => l.textContent)
  expect(limits, 'the honest limits were summarised instead of served').toEqual(deletedBody.limits)
  // And the re-read really re-read.
  expect(log.calls.filter((c) => c.method === 'GET').length).toBeGreaterThan(1)
  view.unmount()
})

test('the delete control is the OWNER’s alone — the operator who may retire still may not purge', async () => {
  const house = { entry: { ...entryA(), owner: 'alice', scope: 'house' }, conflicts: [], cursor: 89 }
  const { view } = await detail(detailRoute(house), { me: 'op', operator: true })
  // The operator can read it AND retire it, and the purge is still absent —
  // authorship, not visibility (S4.1; G2 Def.9; §40-C).
  expect(view.container.querySelector('[data-control="memory-retire"]')).not.toBeNull()
  expect(view.container.querySelector('[data-control="memory-delete"]')).toBeNull()
  expect(view.container.querySelector('[data-verb-absent="delete"]')?.textContent).toContain('may retire it')
  view.unmount()
})

test('and when the door is knocked on anyway, it answers in the gate’s own words', async () => {
  // The refusal is fired rather than assumed: `ErrNotOwner`
  // (internal/memory/memory.go:174) → 403 (internal/api/memory.go:801–808).
  const { view } = await detail({
    ...detailRoute(fixtures.memoryEntry()),
    [`POST /api/memory/${memoryEntryID}/delete`]: {
      status: 403,
      body: { error: 'forbidden', detail: 'memory: not the entry owner' },
    },
  })
  click(view.container.querySelector('[data-open="delete"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  const line = view.container.querySelector('[data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('failed')
  expect(line.textContent).toContain('memory: not the entry owner')
  view.unmount()
})

// ── liveness ───────────────────────────────────────────────────────────────

test('a knowledge frame re-reads the browse, and the note says what does NOT push', async () => {
  const stream = new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })
  const log = scriptedFetch({ 'GET /api/memory': { body: fixtures.memory() } })
  const view = mount(<Memory me="alice" operator={false} stream={stream} />)
  await flush()
  act(() => FakeSource.last().open())
  await flush()

  const before = log.calls.filter((c) => c.path === '/api/memory').length
  act(() =>
    FakeSource.last().send('knowledge.write', {
      seq: 900,
      user_id: 'alice',
      type: 'knowledge.write',
      schema_version: 1,
      topics: [],
      payload: {},
      ts: '2026-07-20T09:10:00Z',
    }),
  )
  await flush()
  expect(
    log.calls.filter((c) => c.path === '/api/memory').length,
    'a knowledge.write frame did not re-read the browse',
  ).toBeGreaterThan(before)

  const note = view.container.querySelector('[data-note="live-currency"]')!
  expect(note.textContent).toContain('a change somebody else makes')
  expect(note.textContent).toContain('re-reads when you open it and after every act')
  view.unmount()
})

// ── phone width ────────────────────────────────────────────────────────────

test('every control is reachable and operable at 375px', async () => {
  const { view } = await browse({
    'GET /api/memory': { body: fixtures.memory() },
    'POST /api/memory': { body: fixtures.memoryEntry() },
  })
  // No layout container pins a width, so nothing here can overflow a phone:
  // the surface is built from wrapping flex rows and min-width-only queries.
  const styled = [...view.container.querySelectorAll('[class]')].map((n) => n.getAttribute('class') ?? '')
  expect(styled.some((c) => /\bw-\[\d+px\]|\bmin-w-\[\d+px\]/.test(c)), 'a fixed pixel width entered the surface').toBe(
    false,
  )
  expect(styled.some((c) => c.includes('flex-wrap')), 'nothing wraps, so a narrow screen would overflow').toBe(true)
  // Any responsive utility is min-width-only (`sm:`/`md:`), never `max-`.
  expect(styled.some((c) => /\bmax-(sm|md|lg):/.test(c)), 'a max-width query entered the surface').toBe(false)

  // And the write door is genuinely operable: it opens and it fires.
  click(view.container.querySelector('[data-open="create"]'))
  await flush()
  fillDraft()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  expect(view.container.querySelector('[data-outcome]')?.getAttribute('data-outcome')).toBe('applied')
  view.unmount()
})

// ── drain r1 ───────────────────────────────────────────────────────────────

test('D1 — an entry that is not there renders the server’s own 404, on the read and on a verb', async () => {
  // internal/api/memory.go:716–718 — `memoryScopeRead` answers every
  // entry-scoped route the same way, 404 BEFORE 403, so an unknown id can never
  // become an existence oracle. The string is transcribed byte-exact.
  const notFound = { error: 'not_found', detail: 'memory entry not found' }

  // (a) the READ arm: the surface says what the control plane said, and renders
  // no entry, no verbs and no invented "deleted" story.
  const { view: gone } = await detail(detailRoute(notFound, 404))
  expect(gone.container.querySelector('.error')?.textContent).toBe(
    'The control plane answered 404: memory entry not found',
  )
  expect(gone.container.querySelector('[data-entry-id]')).toBeNull()
  expect(gone.container.querySelector('[data-verbs="entry"]')).toBeNull()
  gone.unmount()

  // (b) the fired-VERB arm: the entry was readable and had gone by the time the
  // act landed — a real race, and the outcome arm for it is `failed` carrying
  // the same served sentence.
  const { view } = await detail({
    ...detailRoute(fixtures.memoryEntry()),
    [`POST /api/memory/${memoryEntryID}/remove`]: { status: 404, body: notFound },
  })
  click(view.container.querySelector('[data-open="retire"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  const line = view.container.querySelector('[data-outcome]')!
  expect(line.getAttribute('data-outcome')).toBe('failed')
  expect(line.textContent).toContain('refused (not_found)')
  expect(line.textContent).toContain('memory entry not found')
  // A 404 is not a 409: nothing claims the act may be retried against a subject
  // that has moved.
  expect(line.textContent).not.toContain('nothing fired — re-read it and decide again')
  view.unmount()
})

test('D2 — a delete lands, the page re-reads, and what comes back renders as the tombstone', async () => {
  const { view, log } = await detail({
    ...detailRoute(fixtures.memoryEntry()),
    [`POST /api/memory/${memoryEntryID}/delete`]: { body: deletedBody },
  })
  // Before: a live entry with its content and its verbs.
  expect(view.container.querySelector('[data-content="served"]')?.textContent).toBe(entryA().content)
  expect(view.container.querySelector('[data-control="memory-delete"]')).not.toBeNull()

  // The control plane's ANSWER to the next read changes because the delete
  // happened — which is the composition the isolated arms could not prove.
  log.set(`GET /api/memory/${memoryEntryID}`, {
    body: {
      entry: {
        ...entryA(),
        title: '',
        content: undefined,
        topic_key: undefined,
        status: 'removed',
        tombstone: true,
        tombstone_note: 'removed at owner request',
      },
      conflicts: [],
      cursor: 90,
    },
  })

  const readsBefore = log.calls.filter((c) => c.method === 'GET').length
  click(view.container.querySelector('[data-open="delete"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  // The sequence, in order: the verb fired, the unconditional re-read ran, and
  // the entry now renders as the stub.
  //
  // The `[data-outcome]` marker is deliberately NOT asserted here: on this path
  // the re-read turns the entry into a tombstone, the tombstone branch drops the
  // verbs, and the control that fired goes with them. What replaces the marker
  // is stronger than it — the page's own state now says the entry is deleted,
  // and the platform's sentence about the purge is below it. The `applied` arm
  // itself is asserted where the control survives to show it (the isolated
  // delete test, whose re-read still answers with a live entry).
  expect(log.calls.some((c) => c.path.endsWith('/delete'))).toBe(true)
  expect(log.calls.filter((c) => c.method === 'GET').length).toBeGreaterThan(readsBefore)

  expect(view.container.querySelector('[data-tombstone="true"]'), 'the re-read did not reach the detail').not.toBeNull()
  expect(view.container.querySelector('[data-content="served"]'), 'purged content survived the re-read').toBeNull()
  expect(view.container.querySelector('[data-verbs="none"]')).not.toBeNull()
  expect(view.container.querySelector('[data-control="memory-delete"]'), 'a purged entry still offered a purge').toBeNull()
  // And the answer SURVIVES the re-read that removed the control which fired it
  // — the served sentence and the four honest limits are still beside the stub
  // they describe. Before this round they went down with the verbs.
  expect(view.container.querySelector('[data-purge-detail="served"]')?.textContent).toBe(deletedBody.detail)
  expect([...view.container.querySelectorAll('[data-limit="served"]')].map((l) => l.textContent)).toEqual(
    deletedBody.limits,
  )
  expect(view.container.querySelector('[data-tombstone-note="served"]')?.textContent).toBe('removed at owner request')
  view.unmount()
})

test('D3 — the retire confirm is true in the operator’s posture too', async () => {
  const house = { entry: { ...entryA(), owner: 'alice', scope: 'house' }, conflicts: [], cursor: 89 }
  const { view } = await detail(detailRoute(house), { me: 'op', operator: true })
  // There is no delete control below this one for an operator, so pointing at
  // one would be the surface inventing a door for the reader.
  expect(view.container.querySelector('[data-control="memory-delete"]')).toBeNull()
  click(view.container.querySelector('[data-open="retire"]'))
  await flush()
  const said = document.querySelector('[role="dialog"]')!.textContent ?? ''
  expect(said).toContain('a separate right, and it belongs to whoever wrote the entry')
  expect(said).toContain('there is no control here that will')
  expect(said, 'the operator was pointed at a control that is not there').not.toContain('its own control is below')
  view.unmount()
})

test('D5 — a refused write keeps the draft; an applied one starts the next entry clean', async () => {
  const { view, log } = await openComposer({
    'GET /api/memory': { body: fixtures.memory() },
    // internal/memory/memory.go:164 → 400 (internal/api/memory.go:809–815).
    'POST /api/memory': { status: 400, body: { error: 'bad_request', detail: 'memory: invalid entry' } },
  })
  fillDraft('a title worth keeping', 'a body worth keeping')
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  expect(view.container.querySelector('[data-outcome]')?.getAttribute('data-outcome')).toBe('failed')

  // Re-open: the gate's reason is on screen and the fix is usually one field,
  // so the typing survives.
  click(view.container.querySelector('[data-open="create"]'))
  await flush()
  expect(
    (document.querySelector('[data-field="title"]') as HTMLInputElement).value,
    'a refusal cost the person their draft',
  ).toBe('a title worth keeping')
  expect((document.querySelector('[data-field="content"]') as HTMLTextAreaElement).value).toBe('a body worth keeping')

  // Correct it and let it land. The draft is not re-typed — it is the same one.
  log.set('POST /api/memory', { body: fixtures.memoryEntry() })
  typeInto(document.querySelector('[data-field="title"]') as HTMLInputElement, 'a corrected title')
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  expect(view.container.querySelector('[data-outcome]')?.getAttribute('data-outcome')).toBe('applied')
  const sent = log.calls.filter((c) => c.method === 'POST').at(-1)!.body as Record<string, unknown>
  expect(sent.title).toBe('a corrected title')
  expect(sent.content).toBe('a body worth keeping')

  // And after an APPLIED write the next open is clean, because the next entry
  // is a different entry.
  click(view.container.querySelector('[data-open="create"]'))
  await flush()
  expect((document.querySelector('[data-field="title"]') as HTMLInputElement).value).toBe('')
  expect((document.querySelector('[data-field="content"]') as HTMLTextAreaElement).value).toBe('')
  view.unmount()
})

test('D6 — the fixture helper serves ONE caller’s world, session and browse together', async () => {
  // The hazard the parameter removes: two committed bodies are two different
  // callers' ANSWERS, and keying them by URL alone let a test mounted as one
  // person assert the other's body.
  scriptedFetch(memoryRoutes('op'))
  window.history.replaceState(null, '', '/memory')
  const asOp = mount(<App stream={inertStream()} />)
  await flush()
  expect(asOp.container.querySelectorAll('[data-entry-id]')).toHaveLength(0)
  asOp.unmount()

  scriptedFetch(memoryRoutes('alice'))
  window.history.replaceState(null, '', '/memory')
  const asAlice = mount(<App stream={inertStream()} />)
  await flush()
  expect(asAlice.container.querySelectorAll('[data-entry-id]')).toHaveLength(memberList().entries.length)
  asAlice.unmount()
})
