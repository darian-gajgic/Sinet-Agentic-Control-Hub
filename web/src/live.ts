import { useCallback, useEffect, useRef, useState } from 'react'

import { ApiError, Unreachable } from './api'
import { sharedStream, type EventStream, type WireEvent } from './events'

/**
 * Snapshot-then-tail, as one hook (Spec S15.3; P3/CONVENTIONS.md §42).
 *
 * THE WIRING (B6-5 OQ1(b), ratified). Views ride the tab's ONE unfiltered
 * relay and select client-side. They subscribe with the event TYPES they
 * consume and no topic filter, which matters for a mechanical reason: the shell
 * subscribes topicless for the whole authed lifetime, so the connection's query
 * is already the relay — a view that added a topic filter would not narrow the
 * connection, would not receive the server's per-topic `snapshot` frames, and
 * on every navigation could re-open the one connection and put EVERY subscriber
 * back in debt. The relay carries a superset of every topic anyway, and each
 * frame's own `topics` field routes it in-app.
 *
 * REST IS THE TRUTH, FRAMES ARE THE TRIGGER. The server's per-topic snapshot
 * states are face-incomplete for S15.5 — no owner, no project, no stage, no
 * effort, no cost — so every view needs its REST read regardless, and every
 * read carries the `cursor` the tail resumes from. A frame therefore says
 * "something you render changed", and the view RE-READS. It never patches
 * blind, which is the same rule the transport applies to itself.
 *
 * DEBT IS PER SUBSCRIBER. `onResnapshot` hands this hook a `done` bound to
 * itself; it is called when the re-read LANDS, and only then. A read that
 * failed does not clear the debt — a view that could not re-read is not caught
 * up, and letting the indicator say "live" over pre-gap data is the one thing
 * S15.12 forbids.
 */

/** The event types that change what a BOARD card shows.
 *
 *  Chosen against the server's topic map rather than by guesswork: run
 *  lifecycle moves the card between columns, routing carries the effort mode
 *  and any disclosed downgrade note, a limit event is what puts a "parked
 *  until…" on it, a gate ask is what makes it blocked-on-a-human, a recorded
 *  decision is what un-blocks it (and what a drag mints), and usage is what
 *  moves the cost figure. EventSource has no catch-all listener, so a type not
 *  named here is a frame this view never hears (§41-B). */
export const boardEventTypes = [
  'run.created',
  'run.state_changed',
  'run.forked',
  'run.wedged',
  'stage.started',
  'stage.finished',
  'intake.state',
  'routing.decided',
  'limit.event',
  'engine.gate_ask',
  'decision.recorded',
  'usage.recorded',
] as const

/** Mission control watches everything the board does plus the meter movements,
 *  because it renders the consumption gauges beside the work. */
export const missionEventTypes = [...boardEventTypes, 'meter.utilization'] as const

export type Live<T> = {
  data: T | null
  /** The failure this view could not read past — rendered, never swallowed. */
  error: string
  /**
   * True whenever this view owes a re-snapshot: before its first read lands,
   * and again from the moment the stream says continuity is not proven until
   * the re-read completes. Data on screen while this is true is PRE-GAP data
   * and every view marks it as such.
   */
  stale: boolean
  reload: () => void
}

export type LiveRead<T> = {
  /**
   * The read's identity — in practice the request it makes. Changing it means
   * a different question, so the hook re-subscribes and drops the previous
   * answer rather than showing one question's rows under another's heading.
   */
  key: string
  read: () => Promise<T>
  types: readonly string[]
  /** Which frames matter. Absent = every frame of the declared types. */
  applies?: (e: WireEvent) => boolean
  /** Injected so tests drive a scripted stream and never open a connection. */
  stream?: EventStream
}

export function useLive<T>(o: LiveRead<T>): Live<T> {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState('')
  const [stale, setStale] = useState(true)

  // The callbacks are re-created on every render by their callers; holding them
  // in refs keeps the subscription tied to the read's IDENTITY (`key`) instead
  // of re-opening it on each render.
  const readRef = useRef(o.read)
  const appliesRef = useRef(o.applies)
  const runRef = useRef<() => void>(() => {})
  readRef.current = o.read
  appliesRef.current = o.applies

  const { key, stream: injected } = o
  const types = o.types.join(',')

  useEffect(() => {
    const stream = injected ?? sharedStream()
    const declared = types === '' ? [] : types.split(',')
    let mounted = true
    let generation = 0
    let inFlight = false
    let dirty = false
    let owed: (() => void) | null = null
    // The generation the debt was recorded AT. Only a read that STARTED after
    // the signal can clear it: a read already in flight took its snapshot
    // before the stream said continuity was unproven, so settling with it
    // would declare the view caught up on data from the wrong side of the gap.
    let owedAt = 0

    setStale(true)
    setData(null)
    setError('')

    const run = () => {
      if (!mounted) return
      if (inFlight) {
        // A burst of frames coalesces into ONE follow-up read rather than a
        // read per frame: the control plane shares a single writer connection.
        // This is not a timer — nothing is scheduled, the next read starts when
        // the current one lands.
        dirty = true
        return
      }
      inFlight = true
      const mine = ++generation
      const settleNow = () => {
        if (owed === null || mine <= owedAt) return
        const settle = owed
        owed = null
        settle()
      }
      readRef.current().then(
        (next) => {
          inFlight = false
          if (!mounted) return
          setData(next)
          setError('')
          if (owed === null || mine > owedAt) setStale(false)
          settleNow()
          if (dirty) {
            dirty = false
            run()
          }
        },
        (err: unknown) => {
          inFlight = false
          if (!mounted) return
          // The debt is deliberately NOT cleared: a view that could not
          // re-read is not caught up, and the indicator must not say live.
          setError(describeError(err))
          if (dirty) {
            dirty = false
            run()
          }
        },
      )
    }
    runRef.current = run

    // The snapshot half of snapshot-then-tail starts NOW, not when the socket
    // opens. A view that waited for the stream would render nothing at all on a
    // host whose REST surface answers but whose feed does not — blank where it
    // could have been honest data plus a disconnected indicator.
    run()

    const unsubscribe = stream.subscribe({
      types: declared,
      onEvent: (e) => {
        const applies = appliesRef.current
        if (!applies || applies(e)) run()
      },
      onResnapshot: (_reason, done) => {
        owed = done
        owedAt = generation
        setStale(true)
        run()
      },
    })
    return () => {
      mounted = false
      unsubscribe()
    }
  }, [key, types, injected])

  const reload = useCallback(() => runRef.current(), [])
  return { data, error, stale, reload }
}

/** describeError says which KIND of failure happened, because an unreachable
 *  host and a server that answered are different facts and a person acts
 *  differently on each (S15.12). */
export function describeError(err: unknown): string {
  if (err instanceof Unreachable) {
    return 'The control plane is unreachable — the host may be asleep or the tailnet down.'
  }
  if (err instanceof ApiError) return `The control plane answered ${err.status}: ${err.message}`
  return 'The control plane could not be read.'
}
