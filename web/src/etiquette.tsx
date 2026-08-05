import { useEffect, useRef, useState } from 'react'

import { type EventStream, sharedStream } from './events'
import { navigate } from './router'
import { hrefFor, type RouteID } from './routes'
import { useToast } from './ui'

/**
 * Background-run etiquette: sidebar dots and completion toasts (design proposal
 * §3, "background-run etiquette", as narrowed by its own §7 amendment A1.3;
 * P3-UI-7 R15).
 *
 * WHAT THIS ADDS IS VISIBILITY, NOT A MECHANISM. Parked streams already behave:
 * the landed survives-navigation inventory (chatFacts.ts) IS the parked-stream
 * behaviour, and this file does not touch it. What was missing is that a person
 * on another surface had no way to know something had arrived.
 *
 * IT COSTS NO SERVER CHANGE AND NO NEW READ. `App`'s `useConnection` already
 * subscribes with no topics, which forces the UNFILTERED relay, so every
 * owner-scoped frame already arrives at this tab. The etiquette joins that same
 * stream as a second subscriber declaring the types it cares about — the
 * fan-out `EventStream.subscribe` exists for — and never opens a connection of
 * its own, never adds a topic, and never re-reads anything.
 *
 * THERE IS NO CLIENT-SIDE RE-FILTER BY USER. The relay's own scoping decided
 * this tab may see these frames; a second predicate here would be a second
 * authority model beside it, which is the §52-D1 lesson.
 */

/**
 * The chat dots, and the toast set — deliberately the same two types.
 *
 * A turn either settled or was abandoned. Both are the END of something the
 * person started and then navigated away from, which is exactly the moment a
 * notification is worth its interruption. `chat.turn_started` is NOT here: the
 * person who started it is the person who would be told.
 */
export const chatDotTypes = ['chat.turn_settled', 'chat.turn_abandoned'] as const

/**
 * The inbox dots: the card-OPENING types ONLY (OQ5 as ratified).
 *
 * Every type here PUTS a card in the queue. The closing types the inbox itself
 * subscribes to — `decision.recorded`, `run.state_changed`,
 * `watchdog.suppressed`, `watchdog.annotated`, `benchmark.pair_recorded`,
 * `eval.score_recorded` — are deliberately absent, because a dot says
 * "something is waiting for you" and a removal is not. Lighting a dot when a
 * card LEFT the queue would send somebody to look at nothing, which teaches
 * people to ignore the dot.
 */
export const inboxDotTypes = [
  'engine.gate_ask',
  'intake.state',
  'intake.delta_decision',
  'watchdog.flagged',
  'drift.finding',
  'benchmark.alarm',
] as const

/**
 * Which route each dot belongs to.
 *
 * THERE IS NO MEMORY DOT, and the reason is structural rather than a decision
 * about how much to show. Knowledge frames are owner-scoped to the WRITER, so
 * an open memory conflict reaches its addressee through NO frame at all
 * (live.ts records this for the inbox's ninth kind). A memory dot could
 * therefore light for your own writes and stay dark for exactly the case that
 * needs you — a signal that is wrong in the direction that matters. The memory
 * surface says so on screen instead, which is where the honesty already lives.
 */
export const dotTypesByRoute: Partial<Record<RouteID, readonly string[]>> = {
  chat: chatDotTypes,
  inbox: inboxDotTypes,
}

/** Every type the etiquette listens for, in one list for the subscription. */
export const etiquetteTypes = [...chatDotTypes, ...inboxDotTypes]

export type NavDots = Partial<Record<RouteID, boolean>>

/** One completion toast waiting to be shown. `id` is monotonic so the sink can
 *  tell what it has already fired without the hook having to clear anything. */
export type ToastRequest = { id: number; settled: boolean }

/**
 * The most requests kept at once.
 *
 * A structural constant with a named reason (no ⚙ key — the §41-B precedent):
 * the kit's stack shows four, so keeping a handful more than that is enough for
 * the sink to drain a burst and few enough that the list cannot grow without
 * bound on a long-lived tab.
 */
const keptRequests = 8

/**
 * useNavDots lights a route's dot when one of its OPENING frames arrives while
 * the person is somewhere else, and clears it when they arrive.
 *
 * NO COUNTS. The dot says "something arrived", never how much: the frames are
 * triggers and REST is the truth (§42), so a number derived from frames could
 * disagree with the queue the person then opens. A count that can be wrong is
 * worse than a dot that cannot be.
 *
 * NO TIMER. Nothing here schedules anything — the dot lights on a frame and
 * clears on a route change, both of which are events (§32's no-ticker rule).
 */
export function useEtiquette(
  routeID: RouteID,
  authed: boolean,
  injected?: EventStream,
): { dots: NavDots; requests: ToastRequest[] } {
  const [dots, setDots] = useState<NavDots>({})
  const [requests, setRequests] = useState<ToastRequest[]>([])
  const routeRef = useRef(routeID)
  const nextID = useRef(0)
  routeRef.current = routeID

  // Arriving on a route clears its dot. This runs on every route change, so a
  // dot cannot survive the visit that answers it.
  useEffect(() => {
    setDots((prev) => (prev[routeID] === true ? { ...prev, [routeID]: false } : prev))
  }, [routeID])

  useEffect(() => {
    if (!authed) {
      // Signed out there is no owner to scope frames to, and a dot left burning
      // across a sign-out would describe somebody else's work.
      setDots({})
      setRequests([])
      return
    }
    const stream = injected ?? sharedStream()
    const unsubscribe = stream.subscribe({
      types: etiquetteTypes,
      onEvent: (e) => {
        for (const [id, types] of Object.entries(dotTypesByRoute)) {
          if (!types.includes(e.type)) continue
          // The person is looking at it: there is nothing to announce.
          if (routeRef.current === id) continue
          setDots((prev) => ({ ...prev, [id as RouteID]: true }))
        }
        // The toast half rides the SAME subscription rather than a second one,
        // for a reason the landed tests pin: the FIRST subscriber OPENS the
        // source and reads once, and every later one JOINS and is handed an
        // immediate `connected` resnapshot, so it reads twice (§44-B drain r1
        // D7, restated at §45). A subscriber mounted inside the shell tree
        // commits its effect BEFORE the surfaces' — it would become the opener
        // and make every surface read twice. This hook runs in App's OWN
        // effect, which React commits after its children's, exactly as
        // `useConnection` does. So: one subscription, and it joins last.
        if (!(chatDotTypes as readonly string[]).includes(e.type)) return
        // SUPPRESSED ON-ROUTE: somebody reading the conversation watches it
        // settle in place, and a toast over the thing it describes is noise.
        if (routeRef.current === 'chat') return
        const id = ++nextID.current
        setRequests((prev) => [...prev, { id, settled: e.type === 'chat.turn_settled' }].slice(-keptRequests))
      },
      onResnapshot: (_reason, done) => {
        // This subscriber holds no snapshot to re-read, so it is immediately
        // caught up — the shell's own idiom. Dots are deliberately NOT cleared
        // and NOT invented here: after a gap the etiquette cannot know what it
        // missed, so it neither announces an arrival that may not have happened
        // nor erases one that did. The connection pill owns feed-down.
        done()
      },
    })
    return unsubscribe
  }, [authed, injected])

  return { dots, requests }
}

/**
 * TurnToasts fires the completion toast for the chat turn lifecycle, and
 * nothing else (proposal §7 amendment A1.3).
 *
 * WHY THE SET IS THIS SMALL, recorded so no later packet re-derives it. A frame
 * carries `seq/run_id/user_id/type/topics/ts` and an OPAQUE `payload`
 * (events.ts) — frames are triggers and REST is the truth (§42). So:
 *
 *  - `run.state_changed` gets NO toast: the new state is not a top-level fact,
 *    and a "your run finished" toast would be inventing the one word that
 *    matters. Run-grain completion needs a SERVED state fact first, which is
 *    its own sanctioned backend decision.
 *  - deliverable and artifact frames get NO toast: there is no honest link
 *    target without reading a payload the shell is not allowed to interpret.
 *
 * What is left is the pair whose TYPE NAME is itself the whole fact: a turn
 * settled, or a turn was abandoned. The copy says exactly that and no more.
 *
 * It must be mounted INSIDE `ToastProvider` — `useToast` reads the provider's
 * manager — which is why this is a component rather than part of the hook. It
 * SUBSCRIBES TO NOTHING: the one etiquette subscription lives in
 * `useEtiquette`, which runs in App's own effect and therefore joins the stream
 * after every surface has opened it. This component only drains what that hook
 * has already decided to say.
 */
export function TurnToasts({ requests }: { requests: ToastRequest[] }) {
  const toast = useToast()
  const toastRef = useRef(toast)
  toastRef.current = toast
  // The high-water mark of what has already been shown. A ref rather than
  // state: draining must not itself cause a render, or the drain would loop.
  const fired = useRef(0)

  useEffect(() => {
    for (const r of requests) {
      if (r.id <= fired.current) continue
      fired.current = r.id
      toastRef.current.add({
        type: r.settled ? 'success' : 'info',
        // The TYPE NAME is the whole fact, so the title is that type in
        // operator words and the copy adds nothing the frame does not state.
        title: r.settled ? 'A conversation turn settled' : 'A conversation turn was abandoned',
        description: 'It finished while you were on another surface.',
        // The door rides the kit's own `Toast.Action` slot (ui/Toast.tsx,
        // byte-frozen) and hands the route change to the ROUTER — §48 OQ2's
        // rule: cross-link, never re-implement.
        actionProps: {
          children: 'Open the assistant',
          onClick: () => {
            navigate(hrefFor('chat'))
          },
        },
      })
    }
  }, [requests])

  // The toasts render in the kit's portal; this component draws nothing itself.
  return null
}
