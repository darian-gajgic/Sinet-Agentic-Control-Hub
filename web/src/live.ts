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
  // The deliverable lifecycle (drain r1 D6). Without these, a surface that
  // renders review-ready work or a task's deliverables only updates when an
  // unrelated frame happens to arrive — "manual refresh is never required for
  // currency" (S15.12) failing for exactly the transitions the feed exists for.
  'artifact.produced',
  'deliverable.accepted',
  'review.drained',
] as const

/** The run-scoped types a LIVE ACTIVITY feed consumes: the trace of one run as
 *  it happens. Broader than the board's set on purpose — the activity line is
 *  "what is it doing right now", which is every run-scoped frame, not only the
 *  ones that move a card between columns. */
export const activityEventTypes = [
  ...boardEventTypes,
  'tool.completed',
  // Legacy alias of tool.completed (contract.go rename map) — not a registered
  // type; kept so a tail resumed across pre-rename stored rows still triggers.
  'engine.tool_result',
  'checkpoint.written',
  'helper.spawned',
  // The message-grain trace steps (drain r2 R3), so "every run-scoped frame"
  // is true rather than nearly true and the activity line does not lag behind
  // what the run is actually doing. All three are REGISTERED types
  // (internal/eventlog/contract.go): engine.message and engine.done are
  // minted by the adapter envelope; `tool.called` is registered DECLARE-ONLY —
  // no producer emits it today, so declaring it is inert now and correct the
  // day one lands. Naming an unregistered type here would be inventing one.
  'engine.message',
  'engine.done',
  'tool.called',
] as const

/** Mission control watches everything the board does plus the meter movements,
 *  because it renders the consumption gauges beside the work. */
export const missionEventTypes = [...boardEventTypes, 'meter.utilization'] as const

/**
 * The types that open or close an INBOX card (Spec S15.6; §42).
 *
 * Chosen family by family against the server's own registry, because the queue
 * is fed by six different producers and a type not named here is a frame this
 * view never hears — `EventSource` has no catch-all (§41-B). Each entry is
 * either what PUTS a card in the queue or what TAKES one out; a type that does
 * neither is not here, however interesting it is elsewhere.
 *
 * The ninth kind is the honest exception, and the view says so on screen rather
 * than compensating with a timer: an open memory conflict reaches its addressee
 * through NO frame at all. The Knowledge family routes to no SSE topic, and the
 * relay frames a conflicting write does produce are owner-scoped to the WRITER —
 * who, when the question is addressed to somebody else, is not the person
 * waiting. So that kind's currency comes from the re-reads below and from the
 * reader's own reload, and the idempotent already-closed answer is what makes a
 * stale conflict card harmless rather than dangerous.
 */
export const inboxEventTypes = [
  // Gate asks: the durable ask rows behind the ask cards.
  'engine.gate_ask',
  // The intake FSM issues and closes the S06 approval, delta and decision cards.
  'intake.state',
  'intake.delta_decision',
  // A decision is what removes a card from the queue — every effect approval,
  // drift dismissal and conformance acknowledgement mints one.
  'decision.recorded',
  // A run that is cancelled or resumed drops the cards that were waiting on it.
  'run.state_changed',
  // The watchdog family: a flag opens a card, a suppression closes it, and the
  // Tier-1 annotation is trigger evidence the open card renders.
  'watchdog.flagged',
  'watchdog.annotated',
  'watchdog.suppressed',
  // A drift finding opens a drift card (a dismissal rides decision.recorded).
  'drift.finding',
  // The benchmark family: a recorded verdict closes a pending pair, and an
  // alarm raise or clear is itself a card.
  'benchmark.pair_recorded',
  'benchmark.alarm',
  // A conformance result is the ONLY thing that clears a red row — an
  // acknowledgement never does (S14.5).
  'eval.score_recorded',
] as const

/**
 * The types that move the ASSISTANT's session list (Spec S15.7; §42).
 *
 * Three, and they are the whole session lifecycle: a create adds a row, a rename
 * changes the label the list shows, a delete takes the thread away. A turn does
 * not belong here — it moves the transcript, not the list — except through the
 * rename the first-message titling duty performs, which mints
 * `chat.session_renamed` like any other rename.
 *
 * These frames DO reach the person who caused them: chat mutations are the
 * owner's own, and the relay is owner-scoped to the writer. The currency they
 * buy is a second tab and a phone open on the same conversation.
 */
export const chatSessionEventTypes = ['chat.session_created', 'chat.session_renamed', 'chat.session_deleted'] as const

/**
 * The types that move one CONVERSATION's transcript.
 *
 * The turn lifecycle is obvious: a start opens the in-flight state, a settle
 * records what came back, an abandon records that somebody stopped it. The two
 * FILE types are here for a reason that is easy to miss — a turn's produced-files
 * chips are a manifest diff, so an upload landing while a turn is in flight
 * changes what that turn will list, and a file whose `origin_turn` names a turn
 * joins its chips at read time even after it settled. A file frame therefore
 * moves something this view renders.
 */
export const chatTurnEventTypes = [
  'chat.turn_started',
  'chat.turn_settled',
  'chat.turn_abandoned',
  'chat.file_uploaded',
  'chat.file_deleted',
] as const

/** The types that move the exchange sidebar: the manifest's own two mutations. */
export const chatFileEventTypes = ['chat.file_uploaded', 'chat.file_deleted'] as const

/**
 * The types that move the OPEN INTAKE CARD a handoff turn renders in place.
 *
 * Neither of these is a chat type, and that is the point. The pipeline issues an
 * interview card AFTER the handoff verb has already answered — the ask and the
 * intake run's park land in one transaction of their own — so `intake.state` is
 * the only frame that announces the card this surface is waiting to show. And
 * `decision.recorded` is what takes it away again, including when the person
 * answers it from the inbox instead: the card leaves the queue, so it must leave
 * the feed. Both move something this view renders, which is the whole test (§42).
 */
export const chatIntakeEventTypes = ['intake.state', 'decision.recorded'] as const

/**
 * The types that move a DELIVERABLE REVIEW surface (Spec S15.8; §42).
 *
 * Chosen against the registry the same way the other sets were — each one either
 * changes what the page renders or takes something off it:
 *
 *  - `artifact.produced` is a new REVISION, which moves the lineage, the default
 *    round-over-round pair and every placement on screen;
 *  - `review.comment` is the comment loop's own row, minted by the store on both
 *    ingresses (a human comment and a verification finding);
 *  - `review.drained` is what STAMPS the finding numbers and marks a batch
 *    consumed, so it is the frame that flips a comment from open to consumed;
 *  - `deliverable.accepted` is the terminal act: the state moves, the doors close
 *    and the accept card stops being acceptable;
 *  - `verdict.recorded` moves a revision's verdict ref, which the lineage shows;
 *  - `preview.started` / `preview.stopped` move the live-session list this page
 *    offers a stop on;
 *  - `engine.gate_ask` and `decision.recorded` are what OPEN and CLOSE the rework
 *    card the request-revision door hangs on — without them a door that just went
 *    live (or dead) would keep rendering its old state;
 *  - `run.state_changed` covers the minting run reaching a state that changes
 *    whether a revision is still being worked on.
 *
 * `EventSource` has no catch-all, so a type not named here is a frame this view
 * never hears (§41-B).
 */
export const deliverableEventTypes = [
  'artifact.produced',
  'review.comment',
  'review.drained',
  'deliverable.accepted',
  'verdict.recorded',
  'preview.started',
  'preview.stopped',
  'engine.gate_ask',
  'decision.recorded',
  'run.state_changed',
] as const

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
