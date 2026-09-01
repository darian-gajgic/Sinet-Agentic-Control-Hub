import { beforeEach, expect, test, vi } from 'vitest'

import { Unreachable } from './api'
import {
  EventStream,
  type EventSourceLike,
  type ResnapshotReason,
  type Snapshot,
  type Status,
  type Subscription,
  type WireEvent,
} from './events'

/** A scripted EventSource. Nothing here opens a connection to anything: the
 *  harness is offline by construction (no test dials the network, the live
 *  front chain, or any unit). */
class FakeSource implements EventSourceLike {
  static made: FakeSource[] = []
  readonly url: string
  readyState = 0
  private listeners = new Map<string, ((ev: Event) => void)[]>()

  constructor(url: string) {
    this.url = url
    FakeSource.made.push(this)
  }

  addEventListener(type: string, listener: (ev: Event) => void): void {
    const list = this.listeners.get(type) ?? []
    list.push(listener)
    this.listeners.set(type, list)
  }

  close(): void {
    this.readyState = 2
  }

  open(): void {
    this.readyState = 1
    this.fire(new Event('open'))
  }

  /** fail simulates a drop; `fatal` is the browser giving up (a non-2xx, e.g.
   *  the 401 a fail-closed /events answers with no session). */
  fail(fatal: boolean): void {
    this.readyState = fatal ? 2 : 0
    this.fire(new Event('error'))
  }

  send(type: string, data: unknown): void {
    this.fire(new MessageEvent(type, { data: JSON.stringify(data) }))
  }

  sendRaw(type: string, data: string): void {
    this.fire(new MessageEvent(type, { data }))
  }

  private fire(ev: Event): void {
    for (const l of this.listeners.get(ev.type) ?? []) l(ev)
  }

  static last(): FakeSource {
    return FakeSource.made[FakeSource.made.length - 1]
  }
}

type Harness = {
  stream: EventStream
  statuses: Status[]
  timers: { fn: () => void; ms: number }[]
}

function newStream(probe?: () => Promise<{ authenticated: boolean }>): Harness {
  const statuses: Status[] = []
  const timers: { fn: () => void; ms: number }[] = []
  const stream = new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: probe ?? (() => Promise.resolve({ authenticated: true })),
    schedule: (fn, ms) => timers.push({ fn, ms }),
    cancel: () => {},
  })
  stream.onStatus((s) => statuses.push(s))
  return { stream, statuses, timers }
}

const event = (seq: number, type: string, topics: string[]): WireEvent => ({
  seq,
  user_id: 'alice',
  type,
  schema_version: 1,
  topics,
  payload: {},
  ts: '2026-07-29T00:00:00Z',
})

beforeEach(() => {
  FakeSource.made = []
})

// ── one connection per tab (Spec S15.3) ───────────────────────────────────

test('a second subscriber joins the fan-out instead of opening a second connection', () => {
  const { stream } = newStream()
  const a: WireEvent[] = []
  const b: WireEvent[] = []

  stream.subscribe({ types: ['run.state'], onEvent: (e) => a.push(e), onResnapshot: (_r, done) => done() })
  stream.subscribe({ types: ['run.state'], onEvent: (e) => b.push(e), onResnapshot: (_r, done) => done() })

  expect(FakeSource.made).toHaveLength(1)
  expect(stream.opened).toBe(1)

  FakeSource.last().open()
  FakeSource.last().send('run.state', event(1, 'run.state', ['board']))

  expect(a).toHaveLength(1)
  expect(b).toHaveLength(1)
})

test('a widening filter re-opens the one connection rather than adding a second', () => {
  const { stream } = newStream()
  stream.subscribe({ topics: ['board'], types: ['run.state'], onResnapshot: (_r, done) => done() })
  stream.subscribe({ topics: ['inbox'], types: ['run.state'], onResnapshot: (_r, done) => done() })

  // Two opens over time, but never two live sources.
  expect(stream.opened).toBe(2)
  expect(FakeSource.made[0].readyState).toBe(2)
  expect(FakeSource.last().url).toContain('topics=board%2Cinbox')
})

// ── fan-out (S14.3 topic tags) ────────────────────────────────────────────

test('frames route by the topics field, not by the connection filter', () => {
  const { stream } = newStream()
  const board: WireEvent[] = []
  const inbox: WireEvent[] = []
  stream.subscribe({ topics: [], types: ['thing.happened'], onEvent: (e) => board.push(e), onResnapshot: (_r, done) => done() })
  // The second subscriber shares the unfiltered relay and filters in-app.
  stream.subscribe({
    topics: ['inbox'],
    types: ['thing.happened'],
    onEvent: (e) => inbox.push(e),
    onResnapshot: (_r, done) => done(),
  })
  const src = FakeSource.last()
  src.open()

  src.send('thing.happened', event(1, 'thing.happened', ['board']))
  src.send('thing.happened', event(2, 'thing.happened', ['inbox']))

  expect(board.map((e) => e.seq)).toEqual([1, 2])
  expect(inbox.map((e) => e.seq)).toEqual([2])
})

test('the GF14 routing fact: a gate-open frame rides board+run and NEVER inbox — the views\' types-only wiring hears it; a pane pinned to the inbox topic would not', () => {
  // P3-GF14 R2 / drain r1 F1 declared it on the record: intake.state (the
  // frame that announces a fresh card) routes topics [board, run] — never
  // inbox. Every view in this tree subscribes with TYPES on the unfiltered
  // relay and no topic filter (live.ts, B6-5 OQ1(b) ratified), which is
  // exactly why the card wakes arrive. This pin makes the trap explicit: a
  // future pane that "helpfully" filtered itself to the inbox topic would go
  // deaf to the very frames the inbox card set exists for.
  const { stream } = newStream()
  const typesOnly: WireEvent[] = []
  const inboxPinned: WireEvent[] = []
  stream.subscribe({ types: ['intake.state'], onEvent: (e) => typesOnly.push(e), onResnapshot: (_r, done) => done() })
  stream.subscribe({
    topics: ['inbox'],
    types: ['intake.state'],
    onEvent: (e) => inboxPinned.push(e),
    onResnapshot: (_r, done) => done(),
  })
  const src = FakeSource.last()
  src.open()

  src.send('intake.state', event(1, 'intake.state', ['board', 'run']))

  expect(typesOnly.map((e) => e.seq), 'the types-only subscriber missed the gate-open frame').toEqual([1])
  expect(inboxPinned, 'an inbox-topic-pinned pane received a frame the wire never tags inbox').toEqual([])
})

test('a subscriber receives only the event types it declared', () => {
  const { stream } = newStream()
  const seen: string[] = []
  stream.subscribe({ types: ['run.state'], onEvent: (e) => seen.push(e.type), onResnapshot: (_r, done) => done() })
  stream.subscribe({ types: ['run.created'], onResnapshot: (_r, done) => done() })
  const src = FakeSource.last()
  src.open()

  src.send('run.state', event(1, 'run.state', []))
  src.send('run.created', event(2, 'run.created', []))

  expect(seen).toEqual(['run.state'])
})

// ── resume (§30) ──────────────────────────────────────────────────────────

test('the first connect is cursorless — it tails from head', () => {
  const { stream } = newStream()
  stream.subscribe({ types: [], onResnapshot: (_r, done) => done() })

  expect(FakeSource.last().url).toBe('/events')
})

test('a transient drop is left to the browser, which carries Last-Event-ID itself', () => {
  const { stream } = newStream()
  stream.subscribe({ types: [], onResnapshot: (_r, done) => done() })
  FakeSource.last().open()

  FakeSource.last().fail(false) // readyState CONNECTING: the browser is retrying

  expect(stream.opened).toBe(1) // no manual re-open competing with the native one
  expect(stream.status).toBe('connecting')
})

test('a manual re-open carries ?after_seq from the last delivered frame', async () => {
  const { stream, timers } = newStream()
  stream.subscribe({ types: ['run.state'], onResnapshot: (_r, done) => done() })
  const src = FakeSource.last()
  src.open()
  src.send('run.state', event(42, 'run.state', []))

  src.fail(true) // fatal: the browser gave up
  await vi.waitFor(() => expect(stream.status).toBe('disconnected'))

  expect(timers[0].ms).toBe(1000) // first retry is prompt
  timers[0].fn()
  expect(FakeSource.last().url).toBe('/events?after_seq=42')
})

test('with no observed cursor a re-open tails from head rather than replaying from zero', async () => {
  const { stream, timers } = newStream()
  stream.subscribe({ types: [], onResnapshot: (_r, done) => done() })
  FakeSource.last().open()
  FakeSource.last().fail(true)
  await vi.waitFor(() => expect(stream.status).toBe('disconnected'))

  timers[0].fn()
  expect(FakeSource.last().url).toBe('/events')
})

test('reopen backoff grows and is capped while the control plane stays down', async () => {
  const { stream, timers } = newStream()
  stream.subscribe({ types: [], onResnapshot: (_r, done) => done() })

  // Attempt after attempt that never reaches `open`: the host is down.
  for (let i = 0; i < 7; i++) {
    FakeSource.last().fail(true)
    await vi.waitFor(() => expect(timers).toHaveLength(i + 1))
    timers[i].fn()
  }

  expect(timers.map((t) => t.ms)).toEqual([1000, 2000, 5000, 10000, 30000, 30000, 30000])
})

test('a connection that actually opened resets the backoff', async () => {
  const { stream, timers } = newStream()
  stream.subscribe({ types: [], onResnapshot: (_r, done) => done() })

  FakeSource.last().fail(true)
  await vi.waitFor(() => expect(timers).toHaveLength(1))
  timers[0].fn()
  FakeSource.last().fail(true)
  await vi.waitFor(() => expect(timers).toHaveLength(2))
  expect(timers[1].ms).toBe(2000)

  // Now one succeeds: the next drop is treated as fresh, not as attempt three.
  timers[1].fn()
  FakeSource.last().open()
  FakeSource.last().fail(true)
  await vi.waitFor(() => expect(timers).toHaveLength(3))
  expect(timers[2].ms).toBe(1000)
})

// ── snapshots REPLACE, never merge (§30) ──────────────────────────────────

test('a snapshot frame replaces state and is never merged into a stale copy', () => {
  const { stream } = newStream()
  let state: unknown = { rows: ['STALE', 'ALSO STALE'] }
  const snapshots: Snapshot[] = []
  stream.subscribe({
    topics: ['board'],
    types: [],
    onSnapshot: (s) => {
      snapshots.push(s)
      state = s.state // replacement, by contract
    },
    onResnapshot: (_r, done) => done(),
  })
  const src = FakeSource.last()
  src.open()

  src.send('snapshot', { topic: 'board', cursor: 17, state: { rows: ['fresh'] } })

  expect(snapshots).toHaveLength(1)
  expect(state).toEqual({ rows: ['fresh'] })
})

test('a snapshot is delivered only to subscribers of that topic', () => {
  const { stream } = newStream()
  const board: Snapshot[] = []
  const inbox: Snapshot[] = []
  stream.subscribe({ topics: ['board'], types: [], onSnapshot: (s) => board.push(s), onResnapshot: (_r, done) => done() })
  stream.subscribe({ topics: ['inbox'], types: [], onSnapshot: (s) => inbox.push(s), onResnapshot: (_r, done) => done() })
  const src = FakeSource.last()
  src.open()

  src.send('snapshot', { topic: 'inbox', cursor: 3, state: {} })

  expect(board).toHaveLength(0)
  expect(inbox).toHaveLength(1)
})

// ── never patch blind ─────────────────────────────────────────────────────

/** A subscriber that owes its snapshot debt until the test pays it, so the
 *  catching-up → live transition is observable rather than instantaneous. */
function debtor(extra: Partial<Subscription> = {}) {
  const reasons: ResnapshotReason[] = []
  let settle: (() => void) | null = null
  const sub: Subscription = {
    ...extra,
    onResnapshot: (reason, done) => {
      reasons.push(reason)
      settle = done
    },
  }
  return {
    sub,
    reasons,
    pay(): void {
      settle?.()
    },
  }
}

test('every (re)connect tells subscribers to re-snapshot', () => {
  const { stream } = newStream()
  const view = debtor()
  stream.subscribe(view.sub)

  FakeSource.last().open()
  expect(view.reasons).toEqual(['connected'])
  expect(stream.status).toBe('catching-up')

  view.pay()
  expect(stream.status).toBe('live')

  FakeSource.last().open() // a reconnect
  expect(view.reasons).toEqual(['connected', 'connected'])
  expect(stream.status).toBe('catching-up')
})

// Snapshot debt is PER SUBSCRIBER: one view finishing its REST re-read must
// never declare another view caught up, or the indicator reads "live" while a
// surface is still showing what it had before the gap.
test('the stream is live only once every subscriber has re-snapshotted', () => {
  const { stream } = newStream()
  const fast = debtor()
  const slow = debtor()
  stream.subscribe(fast.sub)
  stream.subscribe(slow.sub)
  FakeSource.last().open()

  expect(stream.status).toBe('catching-up')

  fast.pay()
  expect(stream.status, 'one subscriber cannot clear another subscriber s debt').toBe('catching-up')

  slow.pay()
  expect(stream.status).toBe('live')
})

test('a subscriber that leaves takes its debt with it', () => {
  const { stream } = newStream()
  const staying = debtor()
  const leaving = debtor()
  stream.subscribe(staying.sub)
  const stop = stream.subscribe(leaving.sub)
  FakeSource.last().open()
  staying.pay()
  expect(stream.status).toBe('catching-up')

  stop()

  expect(stream.status, 'a departed subscriber left its debt behind').toBe('live')
})

test('a rewound stream re-snapshots and the frame is DROPPED, never applied', () => {
  const { stream } = newStream()
  const applied: number[] = []
  const view = debtor({ types: ['run.state'], onEvent: (e) => applied.push(e.seq) })
  stream.subscribe(view.sub)
  const src = FakeSource.last()
  src.open()
  view.pay()

  src.send('run.state', event(10, 'run.state', []))
  src.send('run.state', event(7, 'run.state', [])) // behind the cursor

  expect(applied).toEqual([10])
  expect(view.reasons).toContain('stream-rewound')
  expect(stream.status).toBe('catching-up')
})

// The non-tautological control: delivered sequences are legitimately sparse
// (owner scope and topic filters drop rows), so a forward jump is NOT loss and
// must not fire the signal — otherwise consumers learn to ignore it.
test('a forward gap in sequence is normal and does not fire the signal', () => {
  const { stream } = newStream()
  const applied: number[] = []
  const view = debtor({ types: ['run.state'], onEvent: (e) => applied.push(e.seq) })
  stream.subscribe(view.sub)
  const src = FakeSource.last()
  src.open()
  view.pay()

  src.send('run.state', event(5, 'run.state', []))
  src.send('run.state', event(91, 'run.state', []))

  expect(applied).toEqual([5, 91])
  expect(view.reasons).toEqual(['connected'])
  expect(stream.status).toBe('live')
})

// The one frame the client knowingly skips. Under the sparse-sequence rule the
// next frame's check cannot notice the hole, so the drop announces itself —
// otherwise it would be the client's only silent loss of data.
test('an unreadable delta is dropped AND says so', () => {
  const { stream } = newStream()
  const applied: number[] = []
  const view = debtor({ types: ['run.state'], onEvent: (e) => applied.push(e.seq) })
  stream.subscribe(view.sub)
  const src = FakeSource.last()
  src.open()
  view.pay()
  expect(stream.status).toBe('live')

  src.sendRaw('run.state', '{not json')

  expect(applied).toEqual([])
  expect(view.reasons).toContain('frame-unreadable')
  expect(stream.status, 'a dropped frame left the stream claiming to be live').toBe('catching-up')

  // And the stream keeps working: the drop is a re-read demand, not a wedge.
  view.pay()
  src.send('run.state', event(1, 'run.state', []))
  expect(applied).toEqual([1])
})

// ── the three fatal-close answers are different facts (S15.12) ────────────

test('a fatal close with no session reads as signed out, and does not retry', async () => {
  const { stream, timers } = newStream(() => Promise.resolve({ authenticated: false }))
  stream.subscribe({ types: [], onResnapshot: (_r, done) => done() })
  FakeSource.last().open()

  FakeSource.last().fail(true)

  await vi.waitFor(() => expect(stream.status).toBe('logged-out'))
  expect(timers).toHaveLength(0)
})

test('a fatal close with the host unreachable says so rather than blaming the feed', async () => {
  const { stream } = newStream(() => Promise.reject(new Unreachable(new Error('offline'))))
  stream.subscribe({ types: [], onResnapshot: (_r, done) => done() })
  FakeSource.last().open()

  FakeSource.last().fail(true)

  await vi.waitFor(() => expect(stream.status).toBe('unreachable'))
})

test('dropping the last subscriber closes the connection', () => {
  const { stream } = newStream()
  const stop = stream.subscribe({ types: [], onResnapshot: (_r, done) => done() })
  FakeSource.last().open()

  stop()

  expect(FakeSource.last().readyState).toBe(2)
})
