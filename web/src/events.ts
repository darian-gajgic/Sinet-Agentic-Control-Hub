import { api, Unreachable } from './api'

/**
 * The one live-data client (Spec S15.3).
 *
 * A tab holds AT MOST ONE EventSource and fans frames out in-app; multi-tab
 * rides HTTP/2 terminated at the front door, so the browser's 6-connection
 * limit never binds and no SharedWorker is needed. Views load a REST snapshot
 * and then tail from its cursor; on anything the client cannot prove was
 * lossless, it says so and the view re-snapshots — it never patches blind.
 */

/** The S14.3 topic filter. An empty topic list is the unfiltered relay. */
export type Topic = 'run' | 'board' | 'fleet' | 'inbox'

/** One delta frame: the stored row, as the transport relays it. */
export type WireEvent = {
  seq: number
  run_id?: string
  generation?: number
  user_id: string
  type: string
  schema_version: number
  topics: string[]
  payload: unknown
  ts: string
}

/** One `snapshot` frame: current state for a topic, at a cursor. */
export type Snapshot = {
  topic: string
  cursor: number
  state: unknown
}

export type Status =
  | 'connecting'
  | 'catching-up'
  | 'live'
  | 'disconnected'
  | 'logged-out'
  | 'unreachable'

/**
 * Why a subscriber must re-load its REST snapshot. Every reason is a case where
 * continuity is NOT PROVEN — never a guess that data was lost.
 */
export type ResnapshotReason =
  /** First connect, or any reconnect: what happened while we were away is
   *  unknown, and on a topic subscription the server's own snapshot frames
   *  supersede Last-Event-ID by contract. */
  | 'connected'
  /** A frame arrived at or below the last sequence we delivered. The stream is
   *  not where we think it is, so the frame is DROPPED rather than applied. */
  | 'stream-rewound'

export type Subscription = {
  /** Server-side filter. Empty = the unfiltered relay (still owner-scoped). */
  topics?: Topic[]
  /** Required when topics includes 'run'. */
  runID?: string
  /**
   * The event TYPES this subscriber consumes.
   *
   * EventSource has no catch-all listener and the transport names every delta
   * frame with its stored event type (`event: run.state`), so somebody has to
   * name the types. Naming them HERE, next to the view that reads them, is the
   * one placement that cannot drift: the alternative is a copy of the server's
   * 96-type inventory maintained by hand in TypeScript.
   */
  types?: string[]
  onSnapshot?: (s: Snapshot) => void
  onEvent?: (e: WireEvent) => void
  /** Obligatory: a subscriber that ignores this renders stale data as live. */
  onResnapshot: (reason: ResnapshotReason) => void
}

/** The minimum of EventSource this client uses, so tests can drive it. */
export interface EventSourceLike {
  addEventListener(type: string, listener: (ev: Event) => void): void
  close(): void
  readonly readyState: number
}

const CONNECTING = 0
const CLOSED = 2

/**
 * Backoff for the MANUAL re-open path only — the browser's own EventSource
 * retry handles transient drops and carries Last-Event-ID natively; we re-open
 * by hand only after a fatal close, where we own the URL because we must carry
 * ?after_seq.
 *
 * Structural constants with named reasons (no ⚙ key exists, and the browser
 * cannot read the registry before login): 1s first, so a control plane that
 * just restarted is picked up almost immediately; 30s ceiling, so a host that
 * slept for hours is not hammered on wake while a returning tab still
 * reconnects inside half a minute.
 */
const reopenBackoffMs = [1_000, 2_000, 5_000, 10_000, 30_000]

type Registered = Subscription & { pending: boolean }

export type StreamOptions = {
  /** Injected so tests drive the stream with a scripted double and never open
   *  a real connection to anything. */
  createEventSource?: (url: string) => EventSourceLike
  /** Injected for the same reason; the default asks the real session route. */
  probeSession?: () => Promise<{ authenticated: boolean }>
  schedule?: (fn: () => void, ms: number) => number
  cancel?: (handle: number) => void
}

export class EventStream {
  private readonly make: (url: string) => EventSourceLike
  private readonly probe: () => Promise<{ authenticated: boolean }>
  private readonly schedule: (fn: () => void, ms: number) => number
  private readonly cancel: (handle: number) => void

  private source: EventSourceLike | null = null
  private query = ''
  private subs = new Set<Registered>()
  private listening = new Set<string>()
  private watchers = new Set<(s: Status) => void>()
  private state: Status = 'disconnected'
  private lastSeq = 0
  private attempt = 0
  private timer: number | null = null
  private closed = false

  constructor(opts: StreamOptions = {}) {
    this.make = opts.createEventSource ?? ((url) => new EventSource(url, { withCredentials: true }))
    this.probe = opts.probeSession ?? (() => api.session())
    this.schedule = opts.schedule ?? ((fn, ms) => window.setTimeout(fn, ms))
    this.cancel = opts.cancel ?? ((h) => window.clearTimeout(h))
  }

  get status(): Status {
    return this.state
  }

  /** connections is the count of EventSource objects ever opened — the
   *  one-per-tab invariant, made observable. */
  opened = 0

  onStatus(fn: (s: Status) => void): () => void {
    this.watchers.add(fn)
    fn(this.state)
    return () => this.watchers.delete(fn)
  }

  /**
   * subscribe joins the fan-out. It opens the connection if there is none;
   * if one is already open and still carries what this subscriber needs, the
   * subscriber JOINS it — a second EventSource is never created.
   */
  subscribe(sub: Subscription): () => void {
    const reg: Registered = { ...sub, pending: true }
    this.subs.add(reg)

    const want = this.desiredQuery()
    if (!this.source) {
      this.open(want)
    } else if (want !== this.query) {
      // The filter widened. Re-open the ONE connection rather than adding a
      // second: every subscriber re-snapshots, which the reconnect signal says.
      this.reopen(want)
    } else {
      this.registerTypes(reg)
      if (this.state === 'live') this.setStatus('catching-up')
      sub.onResnapshot('connected')
    }
    return () => {
      this.subs.delete(reg)
      if (this.subs.size === 0) this.stop()
    }
  }

  /** markApplied is how a subscriber says it has re-loaded its snapshot; the
   *  stream is 'live' only once nobody owes one. */
  markApplied(): void {
    for (const s of this.subs) s.pending = false
    if (this.source && this.state === 'catching-up') this.setStatus('live')
  }

  /** close tears the stream down for good (tab teardown / logout). */
  close(): void {
    this.closed = true
    this.stop()
  }

  // ── internals ────────────────────────────────────────────────────────────

  private stop(): void {
    if (this.timer !== null) {
      this.cancel(this.timer)
      this.timer = null
    }
    this.source?.close()
    this.source = null
    this.listening.clear()
    this.query = ''
  }

  /**
   * desiredQuery is the union of every subscriber's filter.
   *
   * If ANY subscriber wants the unfiltered relay the connection is unfiltered:
   * it carries a superset of every topic's events, and each frame's own
   * `topics` field still routes it in-app. Such a connection carries no server
   * snapshot frames, which is why every subscriber is told to re-snapshot on
   * connect — the S15.3 "load the REST snapshot, then tail" shape.
   */
  private desiredQuery(): string {
    const topics = new Set<Topic>()
    const runs = new Set<string>()
    for (const s of this.subs) {
      if (!s.topics || s.topics.length === 0) return ''
      for (const t of s.topics) topics.add(t)
      if (s.runID) runs.add(s.runID)
    }
    if (topics.size === 0) return ''
    // The run topic is per-run. Two different runs cannot share one filtered
    // connection, so fall back to the unfiltered relay and route in-app.
    if (topics.has('run') && runs.size !== 1) return ''
    const params = new URLSearchParams({ topics: [...topics].sort().join(',') })
    if (topics.has('run')) params.set('run', [...runs][0])
    return params.toString()
  }

  private url(query: string, resume: boolean): string {
    const params = new URLSearchParams(query)
    // Resume explicitly on a MANUAL re-open. With no observed cursor we tail
    // from head instead of replaying from zero — and everyone re-snapshots.
    if (resume && this.lastSeq > 0) params.set('after_seq', String(this.lastSeq))
    const qs = params.toString()
    return qs ? `/events?${qs}` : '/events'
  }

  private open(query: string, resume = false): void {
    if (this.closed) return
    this.query = query
    this.setStatus('connecting')
    const source = this.make(this.url(query, resume))
    this.opened++
    this.source = source
    this.listening.clear()

    source.addEventListener('open', () => {
      if (source !== this.source) return
      this.attempt = 0
      this.requireResnapshot('connected')
    })
    source.addEventListener('error', () => {
      if (source !== this.source) return
      this.onError()
    })
    source.addEventListener('snapshot', (ev) => {
      if (source !== this.source) return
      this.onSnapshotFrame(ev)
    })
    for (const s of this.subs) this.registerTypes(s)
  }

  private reopen(query: string): void {
    this.source?.close()
    this.source = null
    this.open(query, true)
  }

  private registerTypes(sub: Registered): void {
    const source = this.source
    if (!source) return
    for (const type of sub.types ?? []) {
      if (this.listening.has(type)) continue
      this.listening.add(type)
      source.addEventListener(type, (ev) => {
        if (source !== this.source) return
        this.onDelta(ev)
      })
    }
  }

  private requireResnapshot(reason: ResnapshotReason): void {
    for (const s of this.subs) s.pending = true
    this.setStatus('catching-up')
    for (const s of this.subs) s.onResnapshot(reason)
  }

  private onSnapshotFrame(ev: Event): void {
    const snap = parse<Snapshot>(ev)
    if (!snap) return
    // A snapshot REPLACES state; it is never merged into a stale copy. The
    // server supersedes Last-Event-ID on a topic subscribe precisely so this
    // is safe (§30).
    if (snap.cursor > this.lastSeq) this.lastSeq = snap.cursor
    for (const s of this.subs) {
      if (s.topics && s.topics.length > 0 && !s.topics.includes(snap.topic as Topic)) continue
      s.onSnapshot?.(snap)
    }
  }

  private onDelta(ev: Event): void {
    const frame = parse<WireEvent>(ev)
    if (!frame) return

    // Sequence discipline. Delivered sequences are legitimately SPARSE — the
    // server drops what the caller may not see and what the filter excludes —
    // so a forward jump proves nothing and is NOT treated as loss. A frame at
    // or below the last delivered sequence is different: the stream is not
    // where we think it is, so the frame is dropped and consumers re-snapshot
    // rather than patch blind.
    if (frame.seq <= this.lastSeq) {
      this.requireResnapshot('stream-rewound')
      return
    }
    this.lastSeq = frame.seq

    for (const s of this.subs) {
      if (s.types && !s.types.includes(frame.type)) continue
      if (s.topics && s.topics.length > 0 && !frame.topics.some((t) => s.topics?.includes(t as Topic))) continue
      s.onEvent?.(frame)
    }
  }

  private onError(): void {
    const source = this.source
    if (source && source.readyState === CONNECTING) {
      // The browser is retrying on its own and carries Last-Event-ID with it.
      this.setStatus('connecting')
      return
    }
    if (source && source.readyState !== CLOSED) return
    // A fatal close tells us nothing about WHY, so ask. The three answers are
    // genuinely different states and the indicator shows them as such (S15.12).
    this.source = null
    void this.probe().then(
      (session) => {
        if (this.closed) return
        if (!session.authenticated) {
          this.setStatus('logged-out')
          return
        }
        this.setStatus('disconnected')
        this.scheduleReopen()
      },
      (err) => {
        if (this.closed) return
        this.setStatus(err instanceof Unreachable ? 'unreachable' : 'disconnected')
        this.scheduleReopen()
      },
    )
  }

  private scheduleReopen(): void {
    if (this.closed || this.timer !== null || this.subs.size === 0) return
    const wait = reopenBackoffMs[Math.min(this.attempt, reopenBackoffMs.length - 1)]
    this.attempt++
    this.timer = this.schedule(() => {
      this.timer = null
      if (this.closed || this.subs.size === 0) return
      this.open(this.desiredQuery(), true)
    }, wait)
  }

  private setStatus(next: Status): void {
    if (this.state === next) return
    this.state = next
    for (const w of this.watchers) w(next)
  }
}

/**
 * The tab's one stream. Holding it at module scope is what makes "at most one
 * EventSource per tab" (S15.3) structural rather than a convention every future
 * view has to remember.
 */
let shared: EventStream | null = null

export function sharedStream(opts?: StreamOptions): EventStream {
  shared ??= new EventStream(opts)
  return shared
}

export function closeSharedStream(): void {
  shared?.close()
  shared = null
}

function parse<T>(ev: Event): T | null {
  const data = (ev as MessageEvent<string>).data
  if (typeof data !== 'string') return null
  try {
    return JSON.parse(data) as T
  } catch {
    // A frame we cannot parse is logged nowhere and dropped: the transport is
    // not a place to invent state. The next frame's sequence check will notice
    // if anything actually moved.
    return null
  }
}
