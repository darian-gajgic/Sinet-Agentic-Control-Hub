import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import {
  chatDotTypes,
  dotTypesByRoute,
  etiquetteTypes,
  inboxDotTypes,
} from './etiquette'
import { FakeSource, frame, oversightRoutes, scriptedFetch } from './doubles'
import { EventStream } from './events'
import { chatTurnEventTypes, inboxEventTypes, missionEventTypes } from './live'
import { hrefFor, type RouteID } from './routes'
import { flush, mount } from './testing'
import etiquetteSource from './etiquette.tsx?raw'
import appSource from './App.tsx?raw'

/**
 * Background-run etiquette and `ToastProvider`'s production mount (P3-UI-7
 * R15/R16; design proposal §3 as narrowed by its own §7 amendment A1.3).
 *
 * Every signal asserted here keys on a type this platform really registers —
 * the sets are checked against `live.ts`'s own landed vocabulary below, so a
 * dot or a toast can never be driven by a type nobody emits.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

/**
 * Frame sequence numbers must be CONTIGUOUS with the fixture world's head.
 *
 * The golden bodies carry `cursor` 89, and a frame that skips ahead is a GAP —
 * the stream then requires every subscriber to re-snapshot, which is the
 * transport working correctly and would make the no-extra-read assertion below
 * measure the gap rather than the etiquette. Each test starts its own run.
 */
let seq = 89

beforeEach(() => {
  seq = 89
  FakeSource.reset()
  vi.stubGlobal('EventSource', FakeSource)
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

async function openAt(route: RouteID) {
  const log = scriptedFetch(oversightRoutes())
  window.history.replaceState(null, '', hrefFor(route))
  const stream = inertStream()
  const view = mount(<App stream={stream} />)
  await flush()
  const source = FakeSource.last()
  act(() => {
    source.open()
  })
  await flush()
  return { view, log, source }
}

const send = async (source: FakeSource, type: string) => {
  seq += 1
  act(() => {
    source.send(type, frame(seq, type, ['board']))
  })
  await flush()
}

const dotOn = (view: { container: HTMLElement }, route: RouteID) =>
  view.container.querySelector(`a[href="${hrefFor(route)}"][data-dot="true"]`) !== null

// ── R15: the signal sets are the REGISTERED ones ──────────────────────────

test('every etiquette type is a type the platform registers, and the inbox set is the OPENING half', () => {
  // The dot sets may only name types `live.ts` already declares — that file is
  // the client's own transcription of the server's registry, chosen family by
  // family. A type invented here would be a dot that can never light.
  for (const t of inboxDotTypes) {
    expect(inboxEventTypes as readonly string[], `${t} is not a registered inbox type`).toContain(t)
  }

  // The CLOSING types are deliberately absent: a dot says "something is waiting
  // for you", and a card LEAVING the queue is not that. This is the assertion
  // that makes the set a decision rather than a subset.
  for (const closing of [
    'decision.recorded',
    'run.state_changed',
    'watchdog.suppressed',
    'watchdog.annotated',
    'benchmark.pair_recorded',
    'eval.score_recorded',
  ]) {
    expect(inboxEventTypes as readonly string[], `${closing} is not a landed type`).toContain(closing)
    expect(
      inboxDotTypes as readonly string[],
      `${closing} closes a card — a dot keyed to it would announce a removal as an arrival`,
    ).not.toContain(closing)
  }

  // The chat pair is the turn lifecycle's two ENDINGS, checked against the REAL
  // landed list rather than against string literals (drain r1 D6c) — literals
  // would keep passing if `live.ts` renamed a type out from under them.
  for (const t of chatDotTypes) {
    expect(chatTurnEventTypes as readonly string[], `${t} is not a registered chat turn type`).toContain(t)
  }
  expect(chatTurnEventTypes as readonly string[], 'the landed list no longer carries the start type').toContain(
    'chat.turn_started',
  )
  // ...and it is excluded, because the person who started it is the person who
  // would be told.
  expect(chatDotTypes as readonly string[]).not.toContain('chat.turn_started')
  expect(chatDotTypes).toHaveLength(2)
})

test('there is NO memory dot, and the reason is structural rather than a display choice', () => {
  // Knowledge frames are owner-scoped to the WRITER, so an open memory conflict
  // reaches its addressee through no frame at all. A memory dot would light for
  // your own writes and stay dark for exactly the case that needs you.
  expect(Object.keys(dotTypesByRoute).sort()).toEqual(['chat', 'inbox'])
  expect(dotTypesByRoute.memory).toBeUndefined()
  for (const t of etiquetteTypes) {
    expect(t.startsWith('knowledge.'), `${t} is a knowledge frame and cannot reach its addressee`).toBe(false)
  }
  // The reason is written down where the set is, so a later packet does not
  // read the absence as an oversight and "fix" it.
  expect(etiquetteSource).toContain('NO MEMORY DOT')
})

// ── R15: the dots themselves ──────────────────────────────────────────────

test('an opening frame lights the dot of the route you are NOT on, and arriving clears it', async () => {
  const { view, source } = await openAt('mission-control')
  expect(dotOn(view, 'inbox'), 'a dot was lit before anything arrived').toBe(false)

  await send(source, 'engine.gate_ask')
  expect(dotOn(view, 'inbox'), 'an opening frame did not light the inbox dot').toBe(true)
  // NO COUNTS anywhere: the dot is a fact, not a figure.
  const link = view.container.querySelector(`a[href="${hrefFor('inbox')}"]`)!
  expect(link.textContent, 'the dot carries a count the frames cannot justify').not.toMatch(/\d/)

  // Arriving on the route answers it.
  act(() => {
    ;(link as HTMLAnchorElement).dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
  await flush()
  expect(dotOn(view, 'inbox'), 'the dot survived the visit that answers it').toBe(false)
  view.unmount()
})

test('a frame for the route you are ON lights nothing', async () => {
  const { view, source } = await openAt('inbox')
  await send(source, 'engine.gate_ask')
  expect(dotOn(view, 'inbox'), 'a dot announced something already on screen').toBe(false)
  view.unmount()
})

test('a CLOSING frame lights no dot — a removal is not an arrival', async () => {
  const { view, source } = await openAt('mission-control')
  await send(source, 'decision.recorded')
  expect(dotOn(view, 'inbox'), 'a closing frame lit a dot').toBe(false)
  // ...and the control: the opening frame on the same run does light it, so the
  // negative above is about the TYPE rather than about the harness.
  await send(source, 'drift.finding')
  expect(dotOn(view, 'inbox')).toBe(true)
  view.unmount()
})

test('the etiquette adds NO read and does not change the connection', async () => {
  const { view, log, source } = await openAt('mission-control')
  const before = log.calls.length

  // The type must be one THIS surface does not declare, or the test measures
  // the surface's own landed re-read instead of the etiquette. `engine.gate_ask`
  // would be exactly that mistake: it is in `boardEventTypes`, so mission
  // control re-reads on it by design. `chat.turn_settled` is in no oversight
  // set, so any read after it would be the etiquette's own.
  expect(missionEventTypes as readonly string[]).not.toContain('chat.turn_settled')
  await send(source, 'chat.turn_settled')
  expect(log.calls.length, 'the etiquette triggered a REST read').toBe(before)

  // Structural, because a behavioural check can only cover the paths it drives:
  // this module has no reader at all.
  expect(etiquetteSource, 'the etiquette imports the API client').not.toMatch(/from '\.\/api'/)
  expect(etiquetteSource).not.toContain('fetch(')
  // The connection stays as App made it: the shell subscribes with no topics,
  // which forces the unfiltered relay, and the etiquette adds no topic of its
  // own. A `topics=` query would mean a second authority model beside the
  // relay's own scoping (§52-D1).
  expect(source.url).not.toContain('topics=')
  view.unmount()
})

test('a feed-down gap neither clears a lit dot nor invents one — the pill owns feed state', async () => {
  const { view, source } = await openAt('mission-control')
  await send(source, 'engine.gate_ask')
  expect(dotOn(view, 'inbox'), 'the setup did not light a dot').toBe(true)

  // THE GAP, driven through the transport's OWN continuity rule rather than a
  // test-only hook: a frame at or below the last delivered sequence means the
  // stream is not where we think it is, so it is dropped and every subscriber
  // is required to re-snapshot (`stream-rewound`, events.ts). The etiquette has
  // no snapshot to re-read, and after a gap it cannot know what it missed.
  act(() => {
    source.send('engine.gate_ask', frame(1, 'engine.gate_ask', ['board']))
  })
  await flush()

  // NOT CLEARED: the arrival that lit it really happened, and erasing it would
  // lose a fact the person has not answered yet.
  expect(dotOn(view, 'inbox'), 'a feed-down gap erased a real arrival').toBe(true)
  // NOT INVENTED: nothing lights for a route that saw no frame. A dot after a
  // gap would announce an arrival that may never have happened.
  expect(dotOn(view, 'chat'), 'a feed-down gap invented a dot').toBe(false)
  view.unmount()
})

// ── R15: the toasts ───────────────────────────────────────────────────────

test('a settled turn toasts when you are elsewhere, with a door to the assistant', async () => {
  const { view, source } = await openAt('mission-control')
  await send(source, 'chat.turn_settled')

  const text = document.body.textContent ?? ''
  expect(text, 'no completion toast was shown').toContain('A conversation turn settled')
  // The copy states only what the TYPE name carries. No run id, no figure, no
  // claim about what the turn produced — the frame states none of that.
  expect(text).not.toMatch(/\d+ (tokens|messages|files)/)
  expect(text, 'the toast carries no door to the surface it names').toContain('Open the assistant')
  view.unmount()
})

test('an abandoned turn says abandoned, and a turn that settles ON /chat says nothing', async () => {
  const away = await openAt('mission-control')
  await send(away.source, 'chat.turn_abandoned')
  expect(document.body.textContent).toContain('A conversation turn was abandoned')
  away.view.unmount()

  document.querySelectorAll('body > div').forEach((n) => n.remove())
  const on = await openAt('chat')
  await send(on.source, 'chat.turn_settled')
  expect(
    document.body.textContent,
    'a toast fired over the conversation it describes',
  ).not.toContain('A conversation turn settled')
  on.view.unmount()
})

test('NO run or deliverable toast exists, per the proposal amendment', async () => {
  const { view, source } = await openAt('mission-control')
  await send(source, 'run.state_changed')
  await send(source, 'review.comment')
  await send(source, 'deliverable.revision_added')
  const text = document.body.textContent ?? ''
  for (const invented of ['finished', 'completed', 'is ready', 'succeeded', 'failed']) {
    expect(text.toLowerCase(), `a run-grain toast invented "${invented}" from an opaque payload`).not.toContain(
      `run ${invented}`,
    )
  }
  expect(text).not.toContain('A conversation turn')
  view.unmount()
})

test('the request queue never drops a completion silently — the overflow says how many', async () => {
  // THE DEFECT THIS REPLACES (drain r1 D4). The queue used
  // `slice(-keptRequests)`, which discarded the OLDEST undrained requests with
  // no marker — a silent cap, which is the one thing the no-silent-caps
  // invariant forbids. The kit's own stack models the right answer one layer
  // down: `data-limited` hides the overflow, it never drops it.
  const { view, source } = await openAt('mission-control')
  // The overflow only exists when a BURST lands faster than the sink drains, so
  // all twelve frames are delivered inside ONE act: React batches the state
  // updates and the effect runs once, with twelve requests queued behind it.
  act(() => {
    for (let i = 0; i < 12; i++) {
      seq += 1
      source.send('chat.turn_settled', frame(seq, 'chat.turn_settled', ['board']))
    }
  })
  await flush()

  const text = document.body.textContent ?? ''
  expect(text, 'the overflow was dropped without saying so').toContain('earlier completion')
  // The figure is the count really absorbed, not a guess: 12 driven, 8 kept.
  expect(text).toContain('4 earlier completions are not shown separately.')
  view.unmount()
})

// ── R16: the mount ────────────────────────────────────────────────────────

test('ONE ToastProvider wraps the shell, and the kit cap of four holds AT the mount', async () => {
  const { view, source } = await openAt('mission-control')

  // Five toasts, one per frame — the fifth is the one that tests the cap.
  for (let i = 0; i < 5; i++) await send(source, 'chat.turn_settled')

  const roots = [...document.querySelectorAll('[data-limited]')]
  const all = [...document.querySelectorAll('body [class*="border-l-4"]')]
  expect(all.length, 'the stack rendered fewer than the five toasts driven').toBeGreaterThanOrEqual(5)
  // NOTHING IS SILENTLY LOST: the overflow is marked `data-limited` and hidden
  // by the kit's own rule, rather than dropped from the stack.
  expect(roots.length, 'the fifth toast was dropped rather than marked as overflow').toBeGreaterThan(0)
  view.unmount()
})

test('the provider is mounted EXACTLY once, at the shell root', () => {
  // One mount, so every test that mounts <App> inherits it and no surface
  // provides its own competing stack.
  expect(appSource.match(/<ToastProvider>/g) ?? []).toHaveLength(1)
  expect(appSource).toContain('</ToastProvider>')
  // The sink subscribes to NOTHING — the one etiquette subscription lives in
  // the hook that runs in App's own effect. A subscriber mounted inside the
  // shell tree commits before the surfaces' and would become the source-OPENER,
  // making every surface read twice (§44-B drain r1 D7).
  const sink = etiquetteSource.slice(etiquetteSource.indexOf('export function TurnToasts'))
  expect(sink, 'the toast sink opened a subscription of its own').not.toContain('.subscribe(')
  expect(etiquetteSource.match(/\.subscribe\(/g) ?? [], 'the etiquette holds more than one subscription').toHaveLength(
    1,
  )
})

test('the etiquette holds no timer, no storage and no registry key', () => {
  for (const banned of ['setInterval', 'setTimeout', 'localStorage', 'sessionStorage', 'indexedDB', 'document.cookie']) {
    expect(etiquetteSource, `the etiquette reached for ${banned}`).not.toContain(banned)
  }
  // The dot's pulse is CSS motion behind the reduced-motion gate, never a timer.
  expect(appSource).toContain('motion-safe:animate-[dot-pulse')
})
