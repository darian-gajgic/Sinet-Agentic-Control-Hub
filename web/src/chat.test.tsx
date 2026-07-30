import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { ApiError, Unreachable } from './api'
import type {
  Answer,
  ApprovalItem,
  ChatBornTask,
  ChatFile,
  ChatMessage,
  ChatSessionView,
  ChatTurn,
  HistoryRegistry,
} from './api'
import { survivesNavigation, describeChatFailure, turnFact } from './chatFacts'
import { argumentReady, chatFeed, composerVerbs, convertChatItem, emptyPick, turnRequestFor } from './chatRuntime'
import { chatEmptySessionID, chatRoutes, chatSessionID, FakeSource, fixtures, scriptedFetch, type Scripted } from './doubles'
import { EventStream } from './events'
import { chatIntakeEventTypes, chatSessionEventTypes, chatTurnEventTypes } from './live'
import { navigate } from './router'
import { hrefFor, matchRoute, routes } from './routes'
import { click, choose, flush, mount, typeInto } from './testing'

/**
 * The conversational assistant (Spec S15.7; S15.4; FC-v1 §3; S14.10), driven
 * against the GOLDEN FIXTURES — the same bytes an internal/api test asserts the
 * real handlers still serve, produced in that world by the REAL verbs.
 *
 * The committed session body is doing a lot of work and every part of it is load
 * bearing: a Layer-0 deterministic answer, a disambiguation card whose choices are
 * real catalog queries, the S06 handoff with its OPEN intake card and a non-empty
 * produced chip, a Layer-2 escalation carrying `lower-confidence` with its audit
 * block at 200, and a turn left RUNNING for the in-flight re-attach. Where a
 * render needs a body no committed fixture carries, the body is DERIVED from the
 * served one and the derivation is stated — never a payload somebody imagined
 * (the B6-5 root cause).
 */

const scheduled = vi.fn()

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: scheduled,
    cancel: () => {},
  })

const detailPath = `/api/chat/sessions/${chatSessionID}`
const chatHref = `${hrefFor('chat')}?session=${chatSessionID}`
/**
 * The EMPTY session is where a driven send belongs.
 *
 * One turn at a time is a per-SESSION rule, and the rich body deliberately holds
 * a RUNNING turn — so the composer refusing to send a second turn into it is the
 * widget being right, not a harness problem. Both bodies are committed fixtures
 * produced by the real handler.
 */
const emptyPath = `/api/chat/sessions/${chatEmptySessionID}`
const emptyHref = `${hrefFor('chat')}?session=${chatEmptySessionID}`

async function open(path: string, extra: Record<string, Scripted> = {}) {
  const stream = inertStream()
  const log = scriptedFetch({ ...chatRoutes(), ...extra })
  window.history.replaceState(null, '', path)
  const view = mount(<App stream={stream} />)
  await flush(8)
  return { view, log, stream }
}

/** The served bodies, re-parsed per call so one test cannot reach another. */
const served = () => fixtures.chatSession() as unknown as ChatSessionView
const servedFiles = () => (fixtures.chatFiles() as unknown as { files: ChatFile[] }).files
const catalog = () => fixtures.historyCatalog() as unknown as HistoryRegistry

function turnByKind(view: ChatSessionView, kind: string, state = 'settled'): ChatTurn {
  const found = view.turns.find((t) => t.kind === kind && t.state === state)
  if (!found) throw new Error(`the committed body carries no ${state} ${kind} turn — the test would assert nothing`)
  return found
}

function messageOf(view: ChatSessionView, turn: string): ChatMessage {
  const found = view.messages.find((m) => m.turn_id === turn)
  if (!found) throw new Error(`no served message opened turn ${turn}`)
  return found
}

/**
 * turnPost is the POST response for a driven turn, assembled from rows the
 * committed body already carries.
 *
 * Every field therefore came out of the real handler; what the test arranges is
 * WHICH recorded turn comes back, never what a turn looks like.
 */
function turnPost(view: ChatSessionView, turn: ChatTurn) {
  return { body: { turn, message: messageOf(view, turn.turn_id), session: view.session } }
}

/** The served inbox as ALICE reads it — the same body the inbox suite drives,
 *  carrying the chat-born card as the real projector emitted it. */
const chatbornItem = () => (fixtures.approvalsMine() as unknown as { items: ApprovalItem[] }).items

function card(items: ApprovalItem[], nativeAsk: string): ApprovalItem {
  const found = items.find((i) => i.id.endsWith(`:${nativeAsk}`))
  if (!found) throw new Error(`the served queue carries no card for ask ${nativeAsk} — the test would assert nothing`)
  return found
}

const node = (view: { container: HTMLElement }, sel: string) => view.container.querySelector(sel)
const nodes = (view: { container: HTMLElement }, sel: string) => [...view.container.querySelectorAll(sel)]
const turnNode = (view: { container: HTMLElement }, id: string) =>
  view.container.querySelector(`[data-turn="${CSS.escape(id)}"]`) as HTMLElement
const buttonNamed = (view: { container: HTMLElement }, text: string) =>
  nodes(view, 'button').find((b) => (b.textContent ?? '').includes(text)) as HTMLButtonElement | undefined

const appSources = () => {
  const all = import.meta.glob('./**/*.{ts,tsx}', { query: '?raw', import: 'default', eager: true }) as Record<
    string,
    string
  >
  return Object.fromEntries(Object.entries(all).filter(([p]) => !p.endsWith('.test.ts') && !p.endsWith('.test.tsx')))
}

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
  scheduled.mockClear()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── R11: the route flip and the URL contract ──────────────────────────────

test('/chat is live, and it is the surface that answers the URL rather than a stub', async () => {
  const { view } = await open(hrefFor('chat'))
  const text = view.container.textContent ?? ''
  expect(text).toContain('Assistant')
  expect(text, '/chat still renders the not-built-yet stub').not.toContain('Not built yet')
  // Nothing is picked, so the surface says so instead of guessing a conversation.
  expect(text).toContain('No conversation open')
  view.unmount()
})

test('the built/unbuilt table moved by exactly one row, and the pattern did not move at all', () => {
  const chat = routes.find((r) => r.id === 'chat')!
  expect(chat.owner, 'the chat row still names an owning packet').toBe('')
  expect(chat.pattern, 'the route was renamed — a deep link would break (§41-B fill-never-rename)').toBe('/chat')
  expect(chat.title).toBe('Assistant')
  expect(chat.nav).toBe(true)
  // Every OTHER unbuilt surface still names its packet: this packet fills one
  // surface and touches no other row.
  // B6-8 part A filled the deliverable row after this packet landed, so the one
  // remaining unbuilt surface is the workforce map.
  expect(routes.filter((r) => r.owner !== '').map((r) => `${r.id} ${r.owner}`)).toEqual(['workforce B6-8'])
})

test('the open conversation is a URL fact that round-trips through the route table', async () => {
  const { view } = await open(hrefFor('chat'))
  const link = view.container.querySelector(
    `[data-session-id="${CSS.escape(chatSessionID)}"] a`,
  ) as HTMLAnchorElement
  // The path comes from hrefFor and the session rides the query string — the
  // landed `/?view=…` shape, filling the route table rather than adding to it.
  expect(link.getAttribute('href')).toBe(chatHref)
  expect(matchRoute(new URL(link.href, 'http://x').pathname).route.id).toBe('chat')

  click(link)
  await flush(6)
  expect(window.location.pathname + window.location.search).toBe(chatHref)
  expect(view.container.textContent).toContain('what did each run cost?')
  view.unmount()
})

// ── R12: the runtime, the adapter, and the cloud negative ─────────────────

test('the whole served transcript renders through the widget, every turn in served order', async () => {
  const { view, log } = await open(chatHref)
  const body = served()

  expect(nodes(view, '[data-turn]').map((n) => n.getAttribute('data-turn-kind'))).toEqual(
    body.turns.map((t) => t.kind),
  )
  // The person's own words are rendered by the widget's own text part.
  for (const said of body.messages) {
    expect(view.container.textContent, `the transcript dropped a message`).toContain(said.body)
  }
  // Reload restores from SERVED state — there is nothing in this tab to restore
  // from, and nothing was written to web storage to try.
  expect(log.calls.filter((c) => c.path === detailPath && c.method === 'GET').length).toBeGreaterThan(0)
  view.unmount()
})

test('a whole driven turn round-trips with EVERY request scripted, and the client routes the verb', async () => {
  const body = served()
  const askTurn = turnByKind(body, 'ask')
  const { view, log } = await open(emptyHref, { [`POST ${emptyPath}/turns`]: turnPost(body, askTurn) })

  // The nothing-said-yet render, before anything is said.
  expect(view.container.textContent).toContain('Nothing said in this conversation yet')

  typeInto(node(view, '.chat-input') as HTMLTextAreaElement, 'what is running right now?')
  click(node(view, '.chat-send'))
  await flush(8)

  const posted = log.calls.filter((c) => c.method === 'POST' && c.path === `${emptyPath}/turns`)
  expect(posted).toHaveLength(1)
  // Typed prose with no gesture is an `ask` — the Layer-1 floor. It is a DEFAULT,
  // not a classification: no model decided it.
  expect(posted[0].body).toEqual({ kind: 'ask', text: 'what is running right now?' })
  // REST is the truth: the POST confirms and the surface re-reads.
  const after = log.calls.slice(log.calls.indexOf(posted[0]) + 1)
  expect(after.some((c) => c.method === 'GET' && c.path === emptyPath), 'the turn did not re-read the session').toBe(
    true,
  )
  // The zero-unscripted-network limb: scriptedFetch throws on anything nobody
  // scripted, so a green test IS the assertion that every request was a platform
  // call. Stated as well as relied upon.
  for (const call of log.calls) expect(call.path.startsWith('/api/')).toBe(true)
  view.unmount()
})

test('assistant-cloud is never wired: no import, no cloud runtime, no cloud config anywhere in web/src', () => {
  const all = import.meta.glob('./**/*.{ts,tsx}', { query: '?raw', import: 'default', eager: true }) as Record<
    string,
    string
  >
  // The PACKAGE can enter only through an import line, so those are scanned over
  // the whole tree, tests included.
  const pkg = 'assistant-' + 'cloud'
  const importHits: string[] = []
  for (const [path, raw] of Object.entries(all)) {
    for (const line of raw.split('\n')) {
      if (!/^\s*(import|export)\b|require\(/.test(line)) continue
      if (line.includes(pkg)) importHits.push(`${path}: ${line.trim()}`)
    }
  }
  expect(importHits, 'assistant-cloud was imported — it is a dependency of the pin, and it is never wired').toEqual([])

  // The cloud SEAMS are re-exported by the pinned package under its own names, so
  // naming one would wire the cloud without ever naming the package.
  const seams = ['Assistant' + 'Cloud', 'useCloudThreadList' + 'Runtime', 'useCloudThreadList' + 'Adapter', 'CloudFileAttachment' + 'Adapter']
  const seamHits = (files: Record<string, string>) => {
    const hits: string[] = []
    for (const [path, raw] of Object.entries(files)) {
      for (const seam of seams) if (raw.includes(seam)) hits.push(`${path}: ${seam}`)
    }
    return hits
  }
  expect(seamHits(appSources()), 'a cloud runtime or adapter is referenced in app code').toEqual([])

  // BOTH scans get a probe that RUNS THE SCAN, not a re-statement of its predicate
  // on a literal — a "probe" that re-evaluates `raw.includes(x)` proves only that
  // `includes` works (drain r1 D9).
  expect(seamHits({ './planted.tsx': `const r = ${seams[0]}.create({ apiKey: k })` })).toHaveLength(1)
  const plantedImport = { './planted.ts': `import { AssistantCloud } from '${pkg}'\n` }
  const plantedHits: string[] = []
  for (const [path, raw] of Object.entries(plantedImport)) {
    for (const line of raw.split('\n')) {
      if (!/^\s*(import|export)\b|require\(/.test(line)) continue
      if (line.includes(pkg)) plantedHits.push(`${path}: ${line.trim()}`)
    }
  }
  expect(plantedHits, 'the import-line scan cannot catch a planted import').toHaveLength(1)
  // …and the line matcher really does ignore a NON-import mention, which is what
  // makes the scan usable over a tree whose prose explains what it forbids.
  expect(/^\s*(import|export)\b|require\(/.test(` * this file never touches ${pkg}`)).toBe(false)
})

test('the adapter is pure at its edges: the feed and the converter are the served rows', () => {
  const body = served()
  const feed = chatFeed(body)
  // One item per message plus one per turn, and each turn follows the message
  // that opened it — the pairing is the platform's `turn_id`, not a position.
  expect(feed).toHaveLength(body.messages.length + body.turns.length)
  expect(feed[0]).toMatchObject({ role: 'user', key: body.messages[0].message_id })
  expect(feed[1]).toMatchObject({ role: 'assistant', key: body.messages[0].turn_id })

  // The message identity IS the platform's identity.
  const user = convertChatItem(feed[0])
  expect(user.role).toBe('user')
  expect(user.id).toBe(body.messages[0].message_id)
  expect(user.content).toEqual([{ type: 'text', text: body.messages[0].body }])

  // An assistant item carries NO text part: a turn's outcome is a structured
  // record the platform's own renderer draws, never a sentence this client wrote.
  const running = feed.find((f) => f.role === 'assistant' && f.turn.state === 'running')!
  expect(convertChatItem(running)).toMatchObject({ role: 'assistant', content: [], status: { type: 'running' } })
  const settled = feed.find((f) => f.role === 'assistant' && f.turn.state === 'settled')!
  expect(convertChatItem(settled).status).toEqual({ type: 'complete', reason: 'stop' })
})

// ── R11/rubric 4: answers render through the platform's own renderer ──────

test('a Layer-0 answer renders its query name, its confidence and its served notes verbatim', async () => {
  const body = served()
  const t = turnByKind(body, 'view')
  const answer = t.outcome as Answer
  const { view } = await open(chatHref)
  const cell = turnNode(view, t.turn_id)

  expect(cell.querySelector('.answer-query')?.textContent).toBe(answer.query)
  expect(cell.querySelector('[data-confidence]')?.getAttribute('data-confidence')).toBe('deterministic')
  expect(cell.querySelector('.warn-flag'), 'a deterministic answer was flagged lower-confidence').toBeNull()
  for (const note of answer.notes ?? []) expect(cell.textContent).toContain(note)
  // The rows are the served rows, at the served grain.
  expect(cell.querySelectorAll('tbody tr')).toHaveLength(answer.rows.length)
  view.unmount()
})

test('a disambiguation card renders as a pickable ANSWER whose choices are real catalog queries', async () => {
  const body = served()
  const card = turnByKind(body, 'ask')
  const answer = card.outcome as Answer
  const known = new Set((catalog().queries ?? []).map((q) => q.name))
  const { view, log } = await open(chatHref, {
    [`POST ${detailPath}/turns`]: turnPost(body, turnByKind(body, 'view')),
  })
  const cell = turnNode(view, card.turn_id)

  expect(cell.querySelector('[data-card="disambiguation"]'), 'the card was swallowed into an error').not.toBeNull()
  expect(cell.textContent).toContain(answer.card!.reason)
  const choices = [...cell.querySelectorAll('[data-card="disambiguation"] button')]
  expect(choices.map((b) => b.textContent)).toEqual(answer.card!.choices.map((c) => c.query))
  for (const c of answer.card!.choices) {
    expect(known.has(c.query), `the card offers ${c.query}, which the served catalog does not carry`).toBe(true)
  }

  // Picking one RUNS it — as a new turn, recorded like any other.
  click(choices[0])
  await flush(8)
  const posted = log.calls.filter((c) => c.method === 'POST' && c.path === `${detailPath}/turns`)
  expect(posted).toHaveLength(1)
  expect(posted[0].body).toEqual({
    kind: 'query',
    text: messageOf(body, card.turn_id).body,
    query: answer.card!.choices[0].query,
  })
  view.unmount()
})

test('a second turn refused while one is running renders as the in-flight fact it is', async () => {
  // The choose and escalate controls stay armed while a turn runs, deliberately:
  // the act is legitimate the moment that turn settles, and the server's own 409
  // is an informative refusal this table humanizes — which is better than a
  // control that looks pressable and does nothing.
  const body = served()
  const card = turnByKind(body, 'ask')
  const { view } = await open(chatHref, {
    [`POST ${detailPath}/turns`]: {
      status: 409,
      body: { error: 'turn_running', detail: 'chat: a turn is already running in this session' },
    },
  })
  click(turnNode(view, card.turn_id).querySelector('[data-card="disambiguation"] button'))
  await flush(8)

  const line = node(view, '[data-failure]')!
  expect(line.getAttribute('data-failure')).toBe('turn-running')
  expect(line.textContent).toContain('already has a turn in flight')
  expect(line.textContent).toContain('chat: a turn is already running in this session')
  view.unmount()
})

test('a Layer-2 answer renders its lower-confidence flag AND its audit block, at 200', async () => {
  const body = served()
  const t = turnByKind(body, 'open_sql')
  const answer = t.outcome as Answer
  expect(answer.layer, 'the committed body is not a Layer-2 answer').toBe(2)
  const { view } = await open(chatHref)
  const cell = turnNode(view, t.turn_id)

  // G3 D3.5: the flag renders wherever the answer renders.
  expect(cell.querySelector('.warn-flag')?.textContent).toContain(answer.confidence)
  expect(cell.querySelector('[data-audit="open-sql"]'), 'a Layer-2 answer rendered without its audit').not.toBeNull()
  expect(cell.textContent).toContain(answer.audit!.outcome)
  expect(cell.textContent).toContain(answer.audit!.refusal)
  // A LADDER DEGRADATION renders as the answer it is, not as an error banner.
  expect(cell.getAttribute('data-turn-fact')).toBe('answer')
  for (const note of answer.notes ?? []) expect(cell.textContent).toContain(note)
  view.unmount()
})

test('an audited REFUSAL renders as the answer it is, with its audit, at 200', async () => {
  const body = served()
  const t = turnByKind(body, 'open_sql')
  const answer = t.outcome as Answer
  // DERIVED from the real Layer-2 answer, and only in the audit's own outcome
  // fields: the guardrail's other refusal outcome needs a live local tier to
  // produce, which no hermetic world has. Everything structural — the layer, the
  // confidence label, the audit block, the 200 — is the real handler's.
  const refused: ChatTurn = {
    ...t,
    outcome: {
      ...answer,
      audit: {
        ...answer.audit!,
        outcome: 'refused',
        refusal: 'the generated statement was not a single read-only SELECT',
      },
    },
  }
  const { view } = await open(chatHref, {
    [`GET ${detailPath}`]: { body: { ...body, turns: body.turns.map((x) => (x.turn_id === t.turn_id ? refused : x)) } },
  })

  const cell = turnNode(view, t.turn_id)
  // A refusal is an ANSWER, not an error banner — and it arrives with the audit
  // that is the whole artifact the guardrail exists to produce.
  expect(cell.getAttribute('data-turn-fact')).toBe('answer')
  expect(cell.querySelector('[data-audit="open-sql"]')).not.toBeNull()
  expect(cell.textContent).toContain('refused')
  expect(cell.textContent).toContain('the generated statement was not a single read-only SELECT')
  expect(cell.querySelector('.warn-flag')?.textContent).toContain('lower-confidence')
  expect(node(view, '[data-failure]'), 'an audited refusal was rendered as a request failure').toBeNull()
  view.unmount()
})

test('a reload restores the transcript from served state, because it was never in this tab', async () => {
  const body = served()
  const { view, log } = await open(chatHref)
  expect(view.container.textContent).toContain(body.messages[0].body)
  // A reload is a fresh mount over the same URL with nothing carried across: no
  // web storage, no module cache of messages, no client-side transcript at all.
  view.unmount()
  const reloaded = mount(<App stream={inertStream()} />)
  await flush(8)

  expect(reloaded.container.textContent, 'the transcript did not come back').toContain(body.messages[0].body)
  expect(nodes(reloaded, '[data-turn]')).toHaveLength(body.turns.length)
  // And it came back by ASKING: the second mount re-read the session.
  expect(log.calls.filter((c) => c.method === 'GET' && c.path === detailPath).length).toBeGreaterThan(1)
  reloaded.unmount()
})

// ── OQ3: open SQL is an ACT, and a task start is an ACT ───────────────────

test('open SQL is reachable only by pressing the escalation control on an answer', async () => {
  const body = served()
  const layer0 = turnByKind(body, 'view')
  const layer2 = turnByKind(body, 'open_sql')
  const { view, log } = await open(chatHref, { [`POST ${detailPath}/turns`]: turnPost(body, layer2) })

  // It is NOT one of the composer's verbs: a typed sentence can never fall into
  // Layer 2, whatever it says.
  expect(composerVerbs.map((v) => v.kind)).toEqual(['ask', 'view', 'query', 'task'])
  expect(nodes(view, '[data-verb]').map((n) => n.getAttribute('data-verb'))).toEqual(['ask', 'view', 'query', 'task'])
  expect(turnRequestFor({ ...emptyPick, verb: 'ask' }, 'delete everything').kind).toBe('ask')

  // And an answer that is ALREADY Layer 2 offers no further escalation.
  expect(turnNode(view, layer2.turn_id).querySelector('.chat-escalate')).toBeNull()

  const escalate = turnNode(view, layer0.turn_id).querySelector('.chat-escalate')
  expect(escalate, 'no escalation control renders on a Layer-0 answer').not.toBeNull()
  click(escalate)
  await flush(8)
  const posted = log.calls.filter((c) => c.method === 'POST' && c.path === `${detailPath}/turns`)
  expect(posted).toHaveLength(1)
  // It escalates the question the person actually asked, verbatim.
  expect(posted[0].body).toEqual({ kind: 'open_sql', text: messageOf(body, layer0.turn_id).body })
  view.unmount()
})

test('starting a task is an explicit gesture, and the handed files are exchange refs', async () => {
  const body = served()
  const born = turnByKind(body, 'task')
  const file = servedFiles()[0]
  const { view, log } = await open(emptyHref, { [`POST ${emptyPath}/turns`]: turnPost(body, born) })

  click(node(view, '[data-verb="task"] input'))
  await flush(2)
  typeInto(nodes(view, '.chat-arg input[type="text"]')[0] as HTMLInputElement, 'Draft the release notes')
  click(node(view, `[data-input="${CSS.escape(file.file_id)}"] input`))
  typeInto(node(view, '.chat-input') as HTMLTextAreaElement, 'pull the merged PRs and draft release notes')
  click(node(view, '.chat-send'))
  await flush(8)

  const posted = log.calls.filter((c) => c.method === 'POST' && c.path === `${emptyPath}/turns`)
  expect(posted).toHaveLength(1)
  expect(posted[0].body).toEqual({
    kind: 'task',
    text: 'pull the merged PRs and draft release notes',
    title: 'Draft the release notes',
    inputs: [file.file_id],
  })

  // THE GESTURE DIES WITH ITS TURN. If the task pick outlived it, the next thing
  // typed here would start a task nobody asked for — the misfire OQ3 keeps a
  // model away from, arriving through a sticky radio instead.
  const checked = nodes(view, '[data-verb] input').filter((i) => (i as HTMLInputElement).checked)
  expect(checked).toHaveLength(1)
  expect(checked[0].getAttribute('value'), 'the task gesture outlived its turn').toBe('ask')
  view.unmount()
})

test('a verb whose argument is unpicked does not arm, and says why', async () => {
  const { view } = await open(emptyHref)
  click(node(view, '[data-verb="view"] input'))
  await flush(2)
  typeInto(node(view, '.chat-input') as HTMLTextAreaElement, 'what did each run cost?')
  await flush(2)

  expect(view.container.textContent).toContain('Pick one from the list above before sending')
  expect((node(view, '.chat-send') as HTMLButtonElement).disabled).toBe(true)
  expect(argumentReady({ ...emptyPick, verb: 'view' })).toBe(false)
  expect(argumentReady({ ...emptyPick, verb: 'view', view: 'cost_per_run' })).toBe(true)

  // Picking one arms it, and the argument rides the turn.
  choose(nodes(view, '.chat-arg select')[0] as HTMLSelectElement, 'cost_per_run')
  await flush(2)
  expect((node(view, '.chat-send') as HTMLButtonElement).disabled).toBe(false)
  expect(turnRequestFor({ ...emptyPick, verb: 'view', view: 'cost_per_run' }, 'q')).toEqual({
    kind: 'view',
    text: 'q',
    view: 'cost_per_run',
  })
  view.unmount()
})

test('the choice surfaces are the SERVED registries, not a list typed into this view', async () => {
  const { view } = await open(emptyHref)
  const registry = fixtures.historyViews() as unknown as HistoryRegistry

  click(node(view, '[data-verb="view"] input'))
  await flush(2)
  const options = nodes(view, '.chat-arg select option').slice(1)
  expect(options).toHaveLength((registry.views ?? []).length)
  expect(options.map((o) => o.getAttribute('value'))).toEqual((registry.views ?? []).map((v) => v.Name))
  view.unmount()
})

// ── R14: survives navigation, and the stop act ────────────────────────────

/**
 * The must-survive half, named once and used twice: this list is what the
 * partition test pins AND what the behavioral test walks, so a row cannot be
 * declared to survive without something driving it.
 */
const mustSurvive = [
  'the in-flight turn',
  "the in-flight turn's progress",
  'the produced-files accumulation',
  'the transcript',
  'which conversation is open',
  "the tab's one EventSource",
]

test('the inventory is a PINNED PARTITION: flipping any row fails this test', () => {
  // The whole table, verdict by verdict and in order. A subset check plus a
  // "something is shed" count is not falsifiable — a shed row flipped to
  // `survives: true` (with any reason at all) passes it, which is the one thing
  // R14 asked this artifact not to be. The prose reasons are deliberately NOT
  // machine-checked: no honest scan can tell a real reason from a plausible
  // sentence. What carries is this partition plus the behavioral drives below.
  expect(survivesNavigation.map((r) => `${r.survives ? 'survives' : 'shed'}: ${r.thing}`)).toEqual([
    'survives: the in-flight turn',
    "survives: the in-flight turn's progress",
    'survives: the produced-files accumulation',
    'survives: the transcript',
    'survives: which conversation is open',
    "survives: the tab's one EventSource",
    'shed: the composer draft',
    "shed: the composer's verb pick",
    'shed: the uploaded-file list',
    'shed: a request-level notice (sending, or a failure)',
  ])
  // Every must-survive row is DRIVEN, and the two lists are held to each other so
  // a new claim cannot be added to the table without a drive to back it.
  expect(survivesNavigation.filter((r) => r.survives).map((r) => r.thing)).toEqual(mustSurvive)
  for (const row of survivesNavigation) {
    expect(row.because.length, `${row.thing} states no reason at all`).toBeGreaterThan(20)
  }
})

test('a view-switch round trip keeps ALL SIX must-survive things, and sheds the draft', async () => {
  // Every row the inventory claims survives is driven HERE, in one round trip, so
  // the claim and the evidence cannot come apart. The turn is genuinely mid-flight
  // and NOT because this tab remembers it: the committed body serves it as
  // `running`, which is the server-side row the turn verb opened.
  const body = served()
  const inflight = body.running!
  const withChips = body.turns.find((t) => (t.produced ?? []).length > 0)!
  const { view, log, stream } = await open(chatHref)

  expect(turnNode(view, inflight.turn_id).getAttribute('data-turn-fact')).toBe('running')
  const openedBefore = stream.opened
  const callsBefore = log.calls.length

  // A draft nobody sent — the shed half, so the partition is driven in BOTH
  // directions rather than only where survival is convenient.
  typeInto(node(view, '.chat-input') as HTMLTextAreaElement, 'a half-typed question')
  expect((node(view, '.chat-input') as HTMLTextAreaElement).value).toBe('a half-typed question')

  click(nodes(view, '.shell-nav a').find((a) => a.textContent === 'Board')!)
  await flush(6)
  expect(view.container.textContent, 'the board did not take over').toContain('Board')
  expect(turnNode(view, inflight.turn_id), 'the assistant is still mounted').toBeNull()

  // Back to the SAME URL — what a bookmark, the back button and the shell's own
  // rail all do. The conversation is identified by the URL, so coming back is
  // just asking for it again.
  act(() => navigate(chatHref))
  await flush(8)

  // (1) the in-flight turn — still there, because it was never in this tab.
  const back = turnNode(view, inflight.turn_id)
  expect(back, 'the in-flight turn was lost across a view switch').not.toBeNull()
  // (2) its progress — the RECORDED state, not merely the row's existence, and
  // with the act that can end it still offered.
  expect(back.getAttribute('data-turn-state'), "the turn's progress did not survive").toBe('running')
  expect(back.getAttribute('data-turn-fact')).toBe('running')
  expect(buttonNamed(view, 'Stop this turn'), 'the stop act is not on the running turn').toBeDefined()
  // (3) the produced-files accumulation.
  expect(
    turnNode(view, withChips.turn_id).querySelectorAll('[data-chip]'),
    'the produced-files accumulation did not survive',
  ).toHaveLength(withChips.produced!.length)
  // (4) the transcript — every word of it.
  for (const said of body.messages) {
    expect(view.container.textContent, 'the transcript did not survive the round trip').toContain(said.body)
  }
  expect(nodes(view, '[data-turn]')).toHaveLength(body.turns.length)
  // (5) which conversation is open — in the URL and marked in the rail.
  expect(window.location.pathname + window.location.search).toBe(chatHref)
  expect(
    view.container
      .querySelector(`[data-session-id="${CSS.escape(chatSessionID)}"]`)
      ?.getAttribute('data-active'),
    'the rail lost track of which conversation is open',
  ).toBe('true')
  // (6) ONE EventSource for the tab, across the whole round trip.
  expect(stream.opened, 'a view switch re-opened the tab connection').toBe(openedBefore)
  expect(stream.opened).toBe(1)
  expect(FakeSource.made).toHaveLength(1)

  // SHED, as the inventory says: unsent text is not platform state, and carrying
  // it across would be a side-truth no re-snapshot could correct.
  expect((node(view, '.chat-input') as HTMLTextAreaElement).value, 'the composer draft outlived the view').toBe('')

  // NAVIGATION CALLS NOTHING (OQ6): no stop, no cancel, no unload handler.
  const during = log.calls.slice(callsBefore)
  expect(during.filter((c) => c.method === 'POST'), 'navigation called a verb').toEqual([])
  view.unmount()
})

test('a sent turn is asked for straight away, and this tab says what IT is doing meanwhile', async () => {
  // The turn verb is synchronous, so the POST does not answer until the act has
  // settled — but the server wrote the message and opened the turn before the act
  // began. So the surface re-reads immediately instead of making a person watch
  // nothing happen, and until that lands it states its OWN outstanding request
  // rather than drawing a copy of the message it does not own yet.
  const { view, log } = await open(emptyHref, { [`POST ${emptyPath}/turns`]: { body: new Promise(() => {}) } })
  const before = log.calls.length

  typeInto(node(view, '.chat-input') as HTMLTextAreaElement, 'what is running?')
  click(node(view, '.chat-send'))
  await flush(6)

  const since = log.calls.slice(before).map((c) => `${c.method} ${c.path}`)
  expect(since[0], 'the turn was not the first thing sent').toBe(`POST ${emptyPath}/turns`)
  expect(since, 'the surface did not ask the platform for the turn it just opened').toContain(`GET ${emptyPath}`)
  expect(node(view, '[data-sending]')?.textContent).toContain('recording your message')
  view.unmount()
})

test('an outstanding POST holds the next turn back, and offers NO stop it cannot perform', async () => {
  // The other half of "in-flight": the request is still out and no `running` row
  // has come back yet. The widget's own running state is what holds the second
  // turn back, which is the same rule the server enforces per session.
  //
  // AND NOTHING OFFERS TO STOP IT. In this window no turn id exists, so a stop
  // control would be a door onto nothing — and OQ6 rejected client-only forget,
  // so there is no honest local abort to put behind it either. The notice says so
  // in words instead (§43: never a dead button).
  const { view } = await open(emptyHref, { [`POST ${emptyPath}/turns`]: { body: new Promise(() => {}) } })

  typeInto(node(view, '.chat-input') as HTMLTextAreaElement, 'what is running?')
  click(node(view, '.chat-send'))
  await flush(6)

  expect((node(view, '.chat-send') as HTMLButtonElement).disabled, 'a second turn could be raced in').toBe(true)
  expect(node(view, '.chat-cancel'), 'a stop control renders while nothing is stoppable').toBeNull()
  expect(buttonNamed(view, 'Stop'), 'a stop affordance renders while nothing is stoppable').toBeUndefined()
  expect(node(view, '[data-sending]')?.textContent).toContain('nothing to stop yet')
  view.unmount()
})

test('the widget cancel appears once a turn IS stoppable, and calls the abandon verb', async () => {
  // The same affordance, in the window where it can act: a served `running` turn
  // has an id, so the widget's cancel and the in-feed stop control are two
  // affordances on ONE verb.
  const inflight = served().running!
  const { view, log } = await open(chatHref, {
    [`POST /api/chat/turns/${inflight.turn_id}/stop`]: { body: { turn: inflight, applied: true } },
  })

  const cancel = node(view, '.chat-cancel')
  expect(cancel, "the widget's own cancel does not render for a stoppable turn").not.toBeNull()
  click(cancel)
  await flush(8)
  expect(log.calls.filter((c) => c.path === `/api/chat/turns/${inflight.turn_id}/stop`)).toHaveLength(1)
  view.unmount()
})

test('nothing in the app tree stops a turn on unload, and no ticker polls for one', () => {
  const hits: string[] = []
  for (const [path, raw] of Object.entries(appSources())) {
    // Comments out, code in: naming the handler this contract forbids is exactly
    // what the inventory's own doc comment does, and prose cannot register one.
    const code = raw.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/(^|[^:])\/\/.*$/gm, '$1')
    for (const pattern of [/beforeunload/, /pagehide/, /'unload'/, /setInterval/]) {
      if (pattern.test(code)) hits.push(`${path}: ${String(pattern)}`)
    }
  }
  expect(hits, 'a navigation side effect or a ticker exists — a turn must end only by an act (OQ6, §32)').toEqual([])
  // A healthy feed schedules nothing: the coalescing is a dirty bit, not a timer.
  expect(scheduled, 'something scheduled a callback on a healthy feed').not.toHaveBeenCalled()
})

test('the stop act ends the turn visibly, and it is the abandon verb that ends it', async () => {
  const body = served()
  const inflight = body.running!
  const stopped: ChatTurn = { ...inflight, state: 'abandoned', settled_ts: inflight.started_ts }
  // The re-read after the act returns the turn as the server now records it: the
  // outcome is the SERVED state, not something the button believed.
  const after: ChatSessionView = {
    ...body,
    running: undefined,
    turns: body.turns.map((t) => (t.turn_id === inflight.turn_id ? stopped : t)),
  }
  const { view, log } = await open(chatHref, {
    [`POST /api/chat/turns/${inflight.turn_id}/stop`]: { body: { turn: stopped, applied: true } },
  })

  expect(turnNode(view, inflight.turn_id).getAttribute('data-turn-fact')).toBe('running')
  log.set(`GET ${detailPath}`, { body: after })
  click(buttonNamed(view, 'Stop this turn'))
  await flush(8)

  expect(log.calls.filter((c) => c.path === `/api/chat/turns/${inflight.turn_id}/stop`)).toHaveLength(1)
  const cell = turnNode(view, inflight.turn_id)
  expect(cell.getAttribute('data-turn-state')).toBe('abandoned')
  expect(cell.textContent).toContain('You stopped this turn')
  expect(buttonNamed(view, 'Stop this turn'), 'a stopped turn still offers to stop').toBeUndefined()
  view.unmount()
})

// ── R15: the error-humanization table ─────────────────────────────────────

test('the failure table keys on class, status and code — never on message text', () => {
  const src = appSources()['./chatFacts.ts']
  expect(src, 'the failure table is not in the tree').toBeDefined()
  // It branches on what the SERVER classified.
  expect(src).toContain('err.status')
  expect(src).toContain('err.code')
  // And never re-classifies by reading prose. `err.message` may only be CARRIED.
  for (const pattern of [/message\.includes/, /message\.match/, /message\.startsWith/, /message\.toLowerCase/, /detail\.includes/]) {
    expect(pattern.test(src), `the table matches on message text: ${String(pattern)}`).toBe(false)
  }

  // The classes exist and are distinct.
  expect(describeChatFailure(new (class extends Error {})()).class).toBe('server')
  const cases: [number, string, string][] = [
    [401, '', 'signed-out'],
    [404, '', 'not-found'],
    [409, 'turn_running', 'turn-running'],
    [409, 'too_many', 'over-quota'],
    [413, 'too_large', 'too-large'],
    [429, '', 'overloaded'],
    [503, '', 'overloaded'],
    [500, '', 'server'],
  ]
  for (const [status, code, want] of cases) {
    const failure = describeChatFailure(new ApiError(status, 'served detail', code, {}))
    expect(failure.class, `${status}/${code}`).toBe(want)
    // The served detail is PRESERVED, in the platform's own words.
    expect(failure.detail).toBe('served detail')
  }
})

test('the FORWARD-TOLERANT overload arm is humanized with the served detail kept', async () => {
  // R15 mandates 429/overload humanization, so the arm exists — but no chat route
  // reaches it today and the test says so rather than implying otherwise (drain r1
  // D5): the only 429 in internal/api is the login PIN lockout, which chat never
  // calls, and every 503 a chat route produces carries `not_wired`, which the table
  // gives its own class on purpose. This drives the arm against the status it is
  // written for, so the day a lane refusal surfaces here it is already humane.
  const { view } = await open(emptyHref, {
    [`POST ${emptyPath}/turns`]: {
      status: 429,
      body: { error: 'rate_limited', detail: 'the anthropic lane is over its budget until 10:00Z' },
    },
  })
  typeInto(node(view, '.chat-input') as HTMLTextAreaElement, 'what is running?')
  click(node(view, '.chat-send'))
  await flush(8)

  const line = node(view, '[data-failure]')!
  expect(line.getAttribute('data-failure')).toBe('overloaded')
  expect(line.textContent).toContain('busy or unavailable')
  expect(line.textContent, 'the served detail was dropped').toContain(
    'the anthropic lane is over its budget until 10:00Z',
  )
  view.unmount()
})

test('an empty turn and an unreachable host are two different rendered facts', async () => {
  const body = served()
  const settled = turnByKind(body, 'view')
  // DERIVED, and stated: a turn that settled with no recorded outcome is a real
  // served state (the record of an act that produced nothing), and the honest way
  // to drive it is to take a real settled turn and remove the outcome it carried.
  const emptied: ChatTurn = { ...settled, outcome: undefined, outcome_kind: undefined }
  expect(turnFact(emptied).kind).toBe('empty')
  const withEmpty: ChatSessionView = {
    ...body,
    turns: body.turns.map((t) => (t.turn_id === settled.turn_id ? emptied : t)),
  }
  const { view } = await open(chatHref, { [`GET ${detailPath}`]: { body: withEmpty } })

  const cell = turnNode(view, settled.turn_id)
  expect(cell.getAttribute('data-turn-fact')).toBe('empty')
  expect(cell.textContent).toContain('settled without recording an outcome')
  expect(node(view, '[data-failure]'), 'an empty turn was rendered as a request failure').toBeNull()
  view.unmount()

  // The OTHER fact: the host goes away between the read and the send. A turn that
  // yielded nothing and a request that never landed look nothing alike.
  const sender = await open(emptyHref)
  vi.stubGlobal('fetch', () => Promise.reject(new Error('the tailnet is down')))
  typeInto(node(sender.view, '.chat-input') as HTMLTextAreaElement, 'still there?')
  click(node(sender.view, '.chat-send'))
  await flush(8)

  const line = node(sender.view, '[data-failure]')!
  expect(line.getAttribute('data-failure')).toBe('transport')
  expect(line.textContent).toContain('unreachable')
  // And it says the thing that is actually true about a platform-owned transcript.
  expect(line.textContent).toContain('nothing is lost')
  sender.view.unmount()
})

test("a failed turn prints the platform's own error envelope and diagnoses nothing itself", async () => {
  const body = served()
  const t = turnByKind(body, 'view')
  // DERIVED from a real turn: only the recorded outcome is replaced, with the
  // exact {error, detail} envelope the transport records for a failed act.
  const failed: ChatTurn = {
    ...t,
    outcome_kind: 'error',
    outcome: { error: 'not_wired', detail: 'the S14.10 query layers are not wired in this process' },
  }
  const { view } = await open(chatHref, {
    [`GET ${detailPath}`]: { body: { ...body, turns: body.turns.map((x) => (x.turn_id === t.turn_id ? failed : x)) } },
  })
  const cell = turnNode(view, t.turn_id)
  expect(cell.getAttribute('data-turn-fact')).toBe('error')
  expect(cell.querySelector('[data-error-code]')?.getAttribute('data-error-code')).toBe('not_wired')
  expect(cell.textContent).toContain('the S14.10 query layers are not wired in this process')
  view.unmount()
})

test('no view fabricates a model-fallback announcement — it rides the SERVED notes', async () => {
  const banned = [/switched to/i, /falling back to/i, /fell back to/i, /using .{0,20}instead/i, /downgraded to/i]
  const hits: string[] = []
  for (const [path, raw] of Object.entries(appSources())) {
    // Comments come out first, string literals stay: a file is allowed to EXPLAIN
    // a fallback in prose, and only a literal could put one on a screen (the §43
    // ⚙-key scan's own rule).
    const code = raw.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/(^|[^:])\/\/.*$/gm, '$1')
    for (const pattern of banned) if (pattern.test(code)) hits.push(`${path}: ${String(pattern)}`)
  }
  expect(hits, 'a view writes its own model-switch sentence — the announcement is a served note').toEqual([])
  expect(banned.some((p) => p.test('the platform switched to a smaller model'))).toBe(true)

  // The positive half: the served notes DO reach the feed, verbatim. Without
  // this, "no hits" could mean the mechanism is missing rather than honest.
  const body = served()
  const { view } = await open(chatHref)
  const announced = (turnByKind(body, 'open_sql').outcome as Answer).notes ?? []
  expect(announced.length, 'the committed body carries no served note to render').toBeGreaterThan(0)
  for (const note of announced) expect(view.container.textContent).toContain(note)
  view.unmount()
})

// ── R16: the session UI ───────────────────────────────────────────────────

test('the session list renders the honest untitled state and never invents a label', async () => {
  const body = fixtures.chatSessions() as unknown as { sessions: { session_id: string; title: string }[] }
  const untitled = body.sessions.find((s) => s.title === '')!
  const { view } = await open(chatHref)

  const row = view.container.querySelector(`[data-session-id="${CSS.escape(untitled.session_id)}"]`)!
  expect(row.textContent).toContain('Untitled')
  expect(row.querySelector('.absent'), 'an untitled conversation was given a placeholder title').not.toBeNull()
  // The named one shows the served title, and says how it got it.
  const named = body.sessions.find((s) => s.title !== '')!
  expect(view.container.textContent).toContain(named.title)
  expect(view.container.textContent).toContain('the local titling seat is not wired')
  view.unmount()
})

test('create, rename and delete go through the landed verbs, and delete is two-pressed', async () => {
  const body = served()
  const fresh = { session_id: 'cs-new', owner: 'alice', title: '', title_source: '', created_ts: 'x', updated_ts: 'x' }
  const { view, log } = await open(chatHref, {
    'POST /api/chat/sessions': { body: { session: fresh } },
    [`POST /api/chat/sessions/${chatSessionID}/rename`]: { body: { session: { ...body.session, title: 'Costs' } } },
    [`POST /api/chat/sessions/${chatSessionID}/delete`]: {
      body: { session_id: chatSessionID, messages: 5, turns: 5, applied: true },
    },
    'GET /api/chat/sessions/cs-new': { body: { ...body, session: fresh, messages: [], turns: [], running: undefined } },
  })

  click(buttonNamed(view, 'New'))
  await flush(8)
  expect(log.calls.filter((c) => c.method === 'POST' && c.path === '/api/chat/sessions')).toHaveLength(1)
  // A new conversation is opened by URL, so it is bookmarkable the moment it exists.
  expect(window.location.search).toBe('?session=cs-new')

  window.history.replaceState(null, '', chatHref)
  const row = view.container.querySelector(`[data-session-id="${CSS.escape(chatSessionID)}"]`)!
  click([...row.querySelectorAll('button')].find((b) => b.textContent === 'Rename'))
  typeInto(row.querySelector('.chat-rename input') as HTMLInputElement, 'Costs')
  click([...row.querySelectorAll('button')].find((b) => b.textContent === 'Save'))
  await flush(8)
  const renamed = log.calls.find((c) => c.path === `/api/chat/sessions/${chatSessionID}/rename`)
  expect(renamed?.body).toEqual({ title: 'Costs' })

  // Delete takes a whole thread and there is no undo verb: one press arms it.
  click([...row.querySelectorAll('button')].find((b) => b.textContent === 'Delete'))
  await flush(2)
  expect(log.calls.filter((c) => c.path.endsWith('/delete')), 'the first press deleted a thread').toEqual([])
  click([...row.querySelectorAll('button')].find((b) => b.textContent === 'Confirm delete'))
  await flush(8)
  expect(log.calls.filter((c) => c.path === `/api/chat/sessions/${chatSessionID}/delete`)).toHaveLength(1)
  view.unmount()
})

// ── R13: the exchange sidebar and the produced chips ─────────────────────

test('both upload gestures drive the ONE upload verb, with the raw object as the body', async () => {
  const { view, log } = await open(chatHref, {
    'POST /api/chat/files?name=notes.md': { body: { file: servedFiles()[0], applied: true } },
  })
  const file = new File(['# notes\n'], 'notes.md', { type: 'text/markdown' })

  // Gesture one: click-browse.
  const input = node(view, '.chat-browse input') as HTMLInputElement
  Object.defineProperty(input, 'files', { value: [file], configurable: true })
  input.dispatchEvent(new Event('change', { bubbles: true }))
  await flush(8)

  // Gesture two: drag-drop onto the sidebar.
  const drop = new Event('drop', { bubbles: true }) as Event & { dataTransfer?: unknown }
  Object.defineProperty(drop, 'dataTransfer', { value: { files: [file] } })
  node(view, '.chat-exchange')!.dispatchEvent(drop)
  await flush(8)

  const posts = log.calls.filter((c) => c.method === 'POST' && c.path.startsWith('/api/chat/files?'))
  expect(posts, 'the two gestures do not both reach the upload verb').toHaveLength(2)
  for (const post of posts) {
    expect(post.path).toBe('/api/chat/files?name=notes.md')
    // The bytes are the object itself: this client never decodes or re-encodes a
    // person's file, so there is nothing in the middle to corrupt it.
    expect(post.body).toBeInstanceOf(Blob)
    expect((post.body as Blob).size).toBe(file.size)
  }
  view.unmount()
})

test('the uploaded list renders as served, and delete is the landed verb', async () => {
  const files = servedFiles()
  const { view, log } = await open(chatHref, {
    [`POST /api/chat/files/${files[0].file_id}/delete`]: { body: { file: files[0], applied: true } },
  })
  expect(nodes(view, '[data-file]').map((n) => n.getAttribute('data-file'))).toEqual(files.map((f) => f.file_id))
  const row = view.container.querySelector(`[data-file="${CSS.escape(files[0].file_id)}"]`)!
  expect(row.textContent).toContain(files[0].name)
  expect(row.textContent).toContain(files[0].sha256)
  expect(row.textContent).toContain(String(files[0].size_bytes))

  click([...row.querySelectorAll('button')].find((b) => b.textContent === 'Delete'))
  await flush(8)
  expect(log.calls.filter((c) => c.path === `/api/chat/files/${files[0].file_id}/delete`)).toHaveLength(1)
  view.unmount()
})

test('produced-files chips render the served attribution, and an empty one says so', async () => {
  const body = served()
  const withChips = body.turns.find((t) => (t.produced ?? []).length > 0)!
  const withoutChips = body.turns.filter((t) => t.state === 'settled' && (t.produced ?? []).length === 0)
  expect(withoutChips.length, 'the committed body drives only one chip state').toBeGreaterThan(0)
  const { view } = await open(chatHref)

  const chips = turnNode(view, withChips.turn_id).querySelectorAll('[data-chip]')
  expect([...chips].map((c) => c.getAttribute('data-chip'))).toEqual(withChips.produced)
  // A chip names a file the exchange list actually carries.
  const named = servedFiles().find((f) => f.file_id === withChips.produced![0])!
  expect(chips[0].textContent).toContain(named.name)

  // Honestly sparse: at v0 only uploads write the folder, and a turn that
  // produced nothing says nothing came out of it rather than showing an
  // encouraging blank.
  for (const t of withoutChips) {
    const cell = turnNode(view, t.turn_id)
    expect(cell.querySelector('[data-chips="none"]')).not.toBeNull()
    expect(cell.textContent).toContain('No files came out of this turn')
  }
  view.unmount()
})

// ── OQ8(i): the intake card, answered in place ────────────────────────────

test('the born task renders its OPEN intake card in place, with deep links out', async () => {
  const body = served()
  const born = turnByKind(body, 'task')
  const task = born.outcome as ChatBornTask
  const { view } = await open(chatHref)
  const cell = turnNode(view, born.turn_id)

  expect(cell.textContent).toContain(task.title)
  expect(cell.querySelector(`[data-task="${CSS.escape(task.task_id)}"]`)).not.toBeNull()
  // The deep link is built from the route table, never assembled here.
  const link = [...cell.querySelectorAll('a')].map((a) => a.getAttribute('href'))
  expect(link).toContain(hrefFor('task', { id: task.task_id }))
  expect(link).toContain(hrefFor('inbox'))

  // The card is the pipeline's own snapshot, rendered as data.
  const card = cell.querySelector('[data-intake-ask]')!
  expect(card.getAttribute('data-intake-ask')).toBe(task.open_ask_id)
  expect(card.getAttribute('data-intake-kind')).toBe(task.open_card!.kind)
  for (const q of task.open_card!.questions ?? []) {
    expect(card.textContent).toContain(q.text)
    for (const o of q.options ?? []) expect(card.textContent).toContain(o.label)
  }
  view.unmount()
})

test('answering the intake card in place uses the LANDED verb with the card own pin', async () => {
  const body = served()
  const born = turnByKind(body, 'task')
  const task = born.outcome as ChatBornTask
  const question = task.open_card!.questions![0]
  // THE QUEUE ROW IS THE REAL PROJECTOR'S (drain r1 D2). The fixture world seeds
  // the born card as a real `asks` row — the same `intake.Card` value the handoff
  // view carries — so `GET /api/approvals` serves it through the same projection
  // production uses: a bare 64-hex `gates.CanonicalHash` for the pin, whatever
  // action vocabulary an interview card actually derives (none), and the tier,
  // expiry and staleness the platform computes. A hand-built row got all three of
  // those wrong, which is the B6-5 root cause in miniature.
  const queued = card(chatbornItem(), task.open_ask_id!)
  expect(queued.payload_hash, 'the served pin is not a canonical hash').toMatch(/^[0-9a-f]{64}$/)
  expect(queued.answerable, 'the platform is not offering this card for an answer').toBe(true)
  const { view, log } = await open(chatHref, {
    [`POST /api/approvals/${encodeURIComponent(queued.id)}/answer`]: {
      body: { id: queued.id, state: 'answered', applied: true, cursor: 47 },
    },
  })
  const inline = turnNode(view, born.turn_id).querySelector('[data-intake-ask]') as HTMLElement

  // Nothing is sent until the person has answered something.
  expect((buttonNamed(view, 'Answer here') as HTMLButtonElement).disabled).toBe(true)
  choose(inline.querySelector(`[data-question="${CSS.escape(question.id)}"] select`) as HTMLSelectElement,
    question.options![0].value)
  await flush(2)
  click(buttonNamed(view, 'Answer here'))
  await flush(8)

  const answered = log.calls.find((c) => c.method === 'POST' && c.path.includes('/answer'))
  expect(answered, 'the in-place answer did not reach the landed approvals verb').toBeDefined()
  expect(answered!.path).toBe(`/api/approvals/${encodeURIComponent(queued.id)}/answer`)
  expect(answered!.body).toEqual({
    payload_hash: queued.payload_hash,
    answer: { answers: [{ id: question.id, value: question.options![0].value }] },
  })
  // The pin was READ from the queue, not invented: the queue was asked first.
  const order = log.calls.map((c) => `${c.method} ${c.path}`)
  expect(order.indexOf('GET /api/approvals')).toBeLessThan(order.indexOf(`POST ${answered!.path}`))
  view.unmount()
})

test('a card that demands a PIN is sent to the surface that has one, never answered here', async () => {
  // The served card is the authority. This surface has no PIN field and must never
  // grow one — a High-tier step-up exists so approval identity is not inherited
  // from an idle session — so a card declaring step-up is handed to the inbox with
  // the reason, and no answer is posted. At v0 no intake interview card declares
  // it, which is exactly why the assumption needs driving rather than assuming.
  const body = served()
  const born = turnByKind(body, 'task')
  const task = born.outcome as ChatBornTask
  const question = task.open_card!.questions![0]
  const real = card(chatbornItem(), task.open_ask_id!)
  const { view, log } = await open(chatHref, {
    'GET /api/approvals': { body: { items: [{ ...real, step_up_required: true }], cursor: 46 } },
  })
  const inline = turnNode(view, born.turn_id).querySelector('[data-intake-ask]') as HTMLElement

  choose(inline.querySelector(`[data-question="${CSS.escape(question.id)}"] select`) as HTMLSelectElement,
    question.options![0].value)
  await flush(2)
  click(buttonNamed(view, 'Answer here'))
  await flush(8)

  expect(view.container.textContent).toContain('needs your PIN')
  expect(log.calls.filter((c) => c.path.includes('/answer')), 'a step-up card was answered without a PIN').toEqual([])
  // And the way out is offered: the card's own served id, as a link.
  expect(nodes(view, 'a').map((a) => a.getAttribute('href'))).toContain(hrefFor('inbox-item', { id: real.id }))
  view.unmount()
})

test('a card the queue no longer carries says so plainly instead of guessing a pin', async () => {
  const body = served()
  const born = turnByKind(body, 'task')
  const question = (born.outcome as ChatBornTask).open_card!.questions![0]
  const { view, log } = await open(chatHref)
  const inline = turnNode(view, born.turn_id).querySelector('[data-intake-ask]') as HTMLElement

  // The card was open when this was rendered and is gone by the time the person
  // presses Answer — somebody resolved it in the inbox meanwhile. That race is
  // why the pin is re-read at the moment of answering instead of being held from
  // the render, and it is the only way this branch can be reached now that the
  // render itself comes from the queue.
  log.set('GET /api/approvals', { body: { items: [], cursor: 46 } })

  choose(inline.querySelector(`[data-question="${CSS.escape(question.id)}"] select`) as HTMLSelectElement,
    question.options![0].value)
  await flush(2)
  click(buttonNamed(view, 'Answer here'))
  await flush(8)

  expect(view.container.textContent).toContain('no longer in your decision queue')
  expect(log.calls.filter((c) => c.path.includes('/answer')), 'an answer was posted with a guessed pin').toEqual([])
  view.unmount()
})

// ── R17: live wiring ─────────────────────────────────────────────────────

test('the subscription declares event TYPES and no topic, and a frame triggers a re-read', async () => {
  const { view, log } = await open(chatHref)
  const src = FakeSource.last()
  // One connection for the tab, and its URL carries no topic filter.
  expect(FakeSource.made).toHaveLength(1)
  expect(src.url).not.toContain('topics')

  const before = log.calls.filter((c) => c.method === 'GET' && c.path === detailPath).length
  src.open()
  await flush(6)
  src.send('chat.turn_settled', {
    seq: 47,
    user_id: 'alice',
    type: 'chat.turn_settled',
    schema_version: 1,
    topics: [],
    payload: {},
    ts: '2026-07-20T09:05:00Z',
  })
  await flush(8)
  const after = log.calls.filter((c) => c.method === 'GET' && c.path === detailPath).length
  expect(after, 'a chat frame did not trigger the REST re-read (§42)').toBeGreaterThan(before)

  // A type this view does not declare is a frame it never hears.
  const unheard = log.calls.length
  src.send('usage.recorded', { seq: 48, user_id: 'alice', type: 'usage.recorded', schema_version: 1, topics: [], payload: {}, ts: 'x' })
  await flush(6)
  expect(log.calls.length, 'an undeclared type reached the assistant view').toBe(unheard)
  view.unmount()
})

/** A born task whose turn outcome carries NO card and NO ask id: what the real
 *  `Submit` returns, because `Start` births the task with no ask and the pipeline
 *  issues the interview card in a later transaction of its own. */
function cardlessBorn() {
  const body = served()
  const born = turnByKind(body, 'task')
  const task = born.outcome as ChatBornTask
  const askID = task.open_ask_id!
  born.outcome = { ...task, open_ask_id: undefined, open_card: undefined }
  return { body, born, askID }
}

test('the born card renders from the QUEUE, so it appears on the outcome production actually returns', async () => {
  const { body, born, askID } = cardlessBorn()
  const queued = card(chatbornItem(), askID)
  const { view } = await open(chatHref, { [`GET /api/chat/sessions/${chatSessionID}`]: { body } })
  const cell = turnNode(view, born.turn_id)

  // Nothing in the served outcome names a card — the pipeline had not issued one
  // when the handoff answered — and the card is on screen anyway, because the
  // person's own decision queue is what it is read from.
  expect((born.outcome as ChatBornTask).open_card, 'the probe stopped being the production shape').toBeUndefined()
  const inline = cell.querySelector('[data-intake-ask]') as HTMLElement | null
  expect(inline, 'a born task with a queued card rendered none').not.toBeNull()
  expect(inline!.getAttribute('data-intake-ask')).toBe(askID)
  for (const q of (queued.card as { questions: { id: string }[] }).questions) {
    expect(inline!.querySelector(`[data-question="${CSS.escape(q.id)}"]`), `${q.id} is not rendered`).not.toBeNull()
  }
  view.unmount()
})

test('a born task with nothing of yours open says so, and asks the queue for no pin', async () => {
  const { body, born } = cardlessBorn()
  const { view, log } = await open(chatHref, {
    [`GET /api/chat/sessions/${chatSessionID}`]: { body },
    'GET /api/approvals': { body: { items: [], cursor: 46 } },
  })
  const cell = turnNode(view, born.turn_id)

  expect(cell.querySelector('[data-intake-ask]'), 'a card was rendered with nothing to render it from').toBeNull()
  expect(cell.textContent).toContain('Nothing is waiting on you for this task right now')
  expect(cell.textContent, 'the born task itself vanished with its card').toContain('t-chatborn')
  expect(log.calls.filter((c) => c.path.includes('/answer')), 'an answer was posted for a card nobody has').toEqual([])
  view.unmount()
})

test('a card resolved elsewhere leaves the feed — the turn outcome never poses as the live card', async () => {
  // The served outcome DOES carry a card here (the fixture's own shape), and the
  // queue does not. Stale never poses as live: what is rendered is what is open.
  const body = served()
  const born = turnByKind(body, 'task')
  expect((born.outcome as ChatBornTask).open_card, 'this probe needs an outcome-borne card').toBeDefined()
  const { view } = await open(chatHref, { 'GET /api/approvals': { body: { items: [], cursor: 46 } } })

  const cell = turnNode(view, born.turn_id)
  expect(cell.querySelector('[data-intake-ask]'), 'an answered card is still being offered').toBeNull()
  expect(cell.textContent).toContain('Nothing is waiting on you for this task right now')
  view.unmount()
})

test('the intake frame that issues the card is declared, so the feed hears it', async () => {
  const { body } = cardlessBorn()
  const { view, log } = await open(chatHref, { [`GET /api/chat/sessions/${chatSessionID}`]: { body } })
  const src = FakeSource.last()
  src.open()
  await flush(6)
  const before = log.calls.filter((c) => c.path === '/api/approvals').length

  src.send('intake.state', {
    seq: 47,
    user_id: 'alice',
    type: 'intake.state',
    schema_version: 1,
    topics: [],
    payload: { task_id: 't-chatborn' },
    ts: 'x',
  })
  await flush(6)

  expect(
    log.calls.filter((c) => c.path === '/api/approvals').length,
    'the frame that announces the card triggered no re-read',
  ).toBeGreaterThan(before)
  view.unmount()
})

test('the declared types are the chat family, each of them able to move what is rendered', () => {
  expect([...chatSessionEventTypes]).toEqual(['chat.session_created', 'chat.session_renamed', 'chat.session_deleted'])
  expect([...chatTurnEventTypes]).toEqual([
    'chat.turn_started',
    'chat.turn_settled',
    'chat.turn_abandoned',
    'chat.file_uploaded',
    'chat.file_deleted',
  ])
  // The card a handoff renders is issued by the PIPELINE, after the turn that
  // started the task has already settled — so the two types that move it are
  // intake's and the decision's, not chat's own.
  expect([...chatIntakeEventTypes]).toEqual(['intake.state', 'decision.recorded'])
  // No view declares a topic: the tab rides the unfiltered relay and selects by
  // type (§42 OQ1(b)).
  for (const [path, raw] of Object.entries(appSources())) {
    if (!path.includes('Chat') && !path.includes('chat')) continue
    expect(/topics\s*:/.test(raw), `${path} passes a topic filter`).toBe(false)
  }
})

test('a view that owes a re-snapshot says so rather than letting stale data look live', async () => {
  const { view, log } = await open(chatHref)
  const src = FakeSource.last()
  src.open()
  await flush(6)
  const frame = (seq: number) => ({
    seq,
    user_id: 'alice',
    type: 'chat.turn_settled',
    schema_version: 1,
    topics: [],
    payload: {},
    ts: '2026-07-20T09:05:00Z',
  })

  // The re-read is left outstanding, so the debt is observable: a view whose
  // re-read has not landed is showing pre-gap data and has to say so.
  log.set(`GET ${detailPath}`, { body: new Promise(() => {}) })
  src.send('chat.turn_settled', frame(9))
  await flush(4)
  // A frame at or below the last delivered sequence is a REWIND: the stream is not
  // where we thought, so continuity is not proven and everyone re-snapshots.
  src.send('chat.turn_settled', frame(2))
  await flush(4)

  const marks = nodes(view, '[data-freshness]')
  expect(marks.length, 'a re-snapshot debt is invisible — stale data is posing as live').toBeGreaterThan(0)
  expect(marks.some((m) => (m.textContent ?? '').includes('Catching up'))).toBe(true)
  view.unmount()
})

// ── R15/rubric 15: the side-truth sweep over this surface ────────────────

test('a hostile message body renders as text, never as markup', async () => {
  const body = served()
  const hostile = '<img src=x onerror="alert(1)"> <script>steal()</script>'
  // Planted CONTENT in a real served body: message bodies are the one place a
  // person's own words (and, on other surfaces, model output) reach this view.
  const planted: ChatSessionView = {
    ...body,
    messages: body.messages.map((m, i) => (i === 0 ? { ...m, body: hostile } : m)),
  }
  const { view } = await open(chatHref, { [`GET ${detailPath}`]: { body: planted } })

  expect(view.container.textContent, 'the hostile body did not render as text').toContain(hostile)
  expect(view.container.querySelector('img'), 'a planted tag became an element').toBeNull()
  expect(view.container.querySelector('script'), 'a planted script became an element').toBeNull()
  view.unmount()
})

test('the assistant keeps no side-truth: no web storage, no money arithmetic, no progress figure', () => {
  const chatFiles = Object.entries(appSources()).filter(([p]) => /chat|Chat/i.test(p))
  expect(chatFiles.length, 'the sweep read no assistant sources').toBeGreaterThan(2)
  for (const [path, raw] of chatFiles) {
    expect(/localStorage|sessionStorage|document\.cookie/.test(raw), `${path} keeps a client-side side-truth`).toBe(
      false,
    )
    for (const pattern of [/percent/i, /\bpct/i, /progress_?bar/i]) {
      expect(pattern.test(raw), `${path} renders a figure the API does not serve`).toBe(false)
    }
  }
})

test('a transport failure and a server answer are different classes at the table level', () => {
  // The two classes are constructed from the landed module, so a rename of either
  // fails at compile time rather than silently passing.
  expect(describeChatFailure(new Unreachable(new Error('down'))).class).toBe('transport')
  expect(describeChatFailure(new ApiError(503, 'seat down', 'not_wired', {})).class).toBe('not-wired')
})
