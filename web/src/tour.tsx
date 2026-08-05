import { useCallback, useEffect, useState } from 'react'

import { hrefFor, type RouteID } from './routes'
import { Button } from './ui'

/**
 * The interactive do-the-thing tour (design proposal §3, "an interactive
 * do-the-thing tour"; P3-UI-7 R13).
 *
 * OWNED JSX, NO LIBRARY. No tour, spotlight or coach-mark package entered the
 * tree — §47's runtime set is closed, and this is a ring, a card and a step
 * list. It is a REIMPLEMENTATION of an idea, not a port of anybody's code.
 *
 * IT NEVER AUTO-STARTS. There is no persistence anywhere in this client
 * (§41-B bans web storage and no server-side per-person UI-state store exists),
 * so the tour cannot know whether you have seen it. A tour that cannot remember
 * and starts itself would ambush every single page load. It therefore starts
 * from ONE visible affordance in the shell header and nowhere else.
 *
 * NO TIMER. Advancing is event-driven — a click, a key, or the person pressing
 * Next. Nothing is scheduled (§32's no-ticker rule), and the ring's motion is a
 * `motion-safe:` utility, which is CSS rather than a clock.
 */

/**
 * THE VERB WALL (OQ2 as ratified). A step's target may only be one of two
 * classes, and the registry below is checked against this list structurally:
 *
 *  - `nav` — a navigation or route-changing link. It changes the URL and reads.
 *  - `disclosure` — a control that fires NO request, or at most a GET: filter
 *    links, panel openers, `details` toggles, selection pickers.
 *
 * A step may NEVER target a control that fires a non-GET, submits a form, or
 * opens a confirm dialog whose primary act is a verb. Confirm-OPENERS are
 * excluded too, even though opening one fires nothing: a tour that parks a
 * person in front of a destructive confirm has invited the next click.
 */
export const stepClasses = ['nav', 'disclosure'] as const
export type StepClass = (typeof stepClasses)[number]

export type TourStep = {
  id: string
  /** The route this step is shown on. The tour walks routes in registry order. */
  route: RouteID
  /** The real control this step spotlights. */
  target: string
  cls: StepClass
  title: string
  body: string
}

/**
 * The step registry.
 *
 * EVERY TARGET MUST EXIST ON AN EMPTY DATABASE. The tour ships to production,
 * where the seeded demo world never exists, so a step is spotlighting CHROME —
 * the sidebar, the connection pill, a surface's own header — and never a data
 * row. A step whose target is genuinely absent at runtime is SKIPPED WITH THE
 * SKIP VISIBLE, so the tour never rings an empty rectangle and never silently
 * pretends a surface has something it does not.
 */
export const tourSteps: TourStep[] = [
  {
    id: 'nav',
    route: 'mission-control',
    target: 'nav[aria-label="Surfaces"]',
    // The rail is a CONTAINER, not a link: it fires nothing, which is what
    // `disclosure` means. Only a real route-changing anchor is `nav`.
    cls: 'disclosure',
    title: 'Everything lives in this rail',
    body: 'The surfaces are grouped by what they are for: Work is the day-to-day, Results is what came back, Intelligence is the assistant and what the platform knows, System is the machinery and its settings.',
  },
  {
    id: 'connection',
    route: 'mission-control',
    target: 'p.conn',
    cls: 'disclosure',
    title: 'This says whether the page is live',
    body: 'The platform pushes changes as they happen. When this says anything other than live, what you are reading may be behind — it tells you which, rather than going quiet.',
  },
  {
    id: 'inbox',
    route: 'mission-control',
    target: `a[href="${hrefFor('inbox')}"]`,
    cls: 'nav',
    title: 'Decisions wait here',
    body: 'Anything the platform will not do without you lands in the inbox — approvals, questions, flagged work. Open it and see for yourself.',
  },
  {
    id: 'inbox-head',
    route: 'inbox',
    target: '[data-surface-what]',
    cls: 'disclosure',
    title: 'Every surface says what it is',
    body: 'This line is on all of them. It says what the page is for and what it will not do, so nothing here has to be guessed at.',
  },
  {
    id: 'chat',
    route: 'inbox',
    target: `a[href="${hrefFor('chat')}"]`,
    cls: 'nav',
    title: 'The assistant is a way in',
    body: 'Ask about the platform, hand work over as a task, or exchange files. Open it.',
  },
  {
    id: 'chat-head',
    route: 'chat',
    // Scoped to the assistant's own header. A bare `[data-surface-what]` would
    // match whatever surface the reader happens to be on, so the step could
    // never be absent and the visible-skip path would be unreachable.
    target: '.chat-head [data-surface-what]',
    cls: 'disclosure',
    title: 'Three things happen here',
    body: 'It answers questions about this platform, it hands work to intake, and it exchanges files. It does not write prose for you, and it says so rather than pretending.',
  },
  {
    id: 'settings',
    route: 'chat',
    target: `a[href="${hrefFor('settings')}"]`,
    cls: 'nav',
    title: 'Every number has one home',
    body: 'Settings is generated from the platform’s own registry, so a setting appears there because the platform declares it — never because a page hard-coded it.',
  },
]

/** The steps this route can show, in registry order. */
export function stepsFor(route: RouteID): TourStep[] {
  return tourSteps.filter((s) => s.route === route)
}

type TourState = { running: boolean; index: number }

/**
 * useTour owns the walk. It is a plain state machine over the registry: no
 * storage, no registry key, no schedule.
 *
 * There is deliberately NO per-route gate here. A step whose target is not on
 * the page — because it belongs to another surface, or because an empty
 * database has not produced it — is handled by the ONE honest mechanism the
 * overlay already has: the target is absent, so the card says there is nothing
 * to point at and offers to move on. Two mechanisms for one situation would
 * mean a step that renders nothing at all, which is the silent case this tour
 * must not have.
 */
export function useTour(authed: boolean) {
  const [{ running, index }, set] = useState<TourState>({ running: false, index: 0 })

  // A mid-tour session expiry forces /login, and a spotlight ring over a PIN
  // field points at a control the tour never registered and has nothing true to
  // say about. The tour ends with the session (drain r1 D5).
  useEffect(() => {
    if (!authed) set({ running: false, index: 0 })
  }, [authed])

  const stop = useCallback(() => {
    set({ running: false, index: 0 })
  }, [])
  const start = useCallback(() => {
    set({ running: true, index: 0 })
  }, [])
  const next = useCallback(() => {
    set((s) => (s.index + 1 >= tourSteps.length ? { running: false, index: 0 } : { ...s, index: s.index + 1 }))
  }, [])

  // Escape ends it from any step — the kit's own escape-stack discipline, and
  // the one key everybody already tries first.
  useEffect(() => {
    if (!running) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') stop()
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
    }
  }, [running, stop])

  const step = running ? tourSteps[index] : null
  return { running, step, start, stop, next }
}

/**
 * The overlay: a spotlight ring on the real control plus the step's words.
 *
 * The ring is drawn from the target's own bounding box, so it follows whatever
 * the surface actually rendered rather than a remembered position. When the
 * target is ABSENT the card says so and offers to move on — the visible skip.
 */
export function TourOverlay({
  step,
  index,
  total,
  onNext,
  onStop,
}: {
  step: TourStep
  index: number
  total: number
  onNext: () => void
  onStop: () => void
}) {
  const [box, setBox] = useState<DOMRect | null>(null)
  const [missing, setMissing] = useState(false)

  useEffect(() => {
    const el = document.querySelector(step.target)
    setMissing(el === null)
    setBox(el === null ? null : el.getBoundingClientRect())
    if (el === null) return
    // ADVANCING BY CLICKING THE REAL CONTROL. The listener is on the target
    // itself and only ever OBSERVES: it never calls the control, never
    // preventDefault()s it, and never synthesizes a second activation. The
    // control does exactly what it would have done, and the tour moves on.
    const advance = () => {
      onNext()
    }
    el.addEventListener('click', advance)
    return () => {
      el.removeEventListener('click', advance)
    }
  }, [step, onNext])

  return (
    // POINTER EVENTS ARE OFF ON THE ROOT, and this is the whole reason the
    // tour can be clicked through at all (drain r1 D1). A `fixed inset-0`
    // element defaults to `pointer-events: auto` and covers the viewport, so
    // hit-testing lands EVERY click outside the card on this div: the spotlit
    // control could never be pressed, click-to-advance could never fire, and a
    // nav step could never navigate. jsdom does no hit-testing, so all eight
    // tour tests passed over that defect — the §52-B class exactly. The
    // pass-through chrome precedent is the aurora field (index.css).
    <div
      className="pointer-events-none fixed inset-0 z-40"
      data-tour-overlay
      data-tour-step={step.id}
    >
      <div className="absolute inset-0 bg-black/45" aria-hidden="true" />

      {box !== null && (
        <div
          aria-hidden="true"
          data-tour-ring
          className="pointer-events-none absolute rounded-(--radius-sm) ring-2 ring-(--accent) motion-safe:transition-[top,left,width,height]"
          style={{
            top: box.top - 4,
            left: box.left - 4,
            width: box.width + 8,
            height: box.height + 8,
          }}
        />
      )}

      <div
        role="dialog"
        aria-label="Guided tour"
        data-tour-card
        className="pointer-events-auto absolute right-4 bottom-4 left-4 mx-auto max-w-sm rounded-(--radius) border border-border bg-popover/95 p-4 shadow-(--shadow) backdrop-blur-[var(--blur-overlay)]"
      >
        <p className="m-0 text-xs tracking-wide text-muted-foreground uppercase">
          Step {String(index + 1)} of {String(total)}
        </p>
        <p className="mt-1 mb-1 font-medium">{step.title}</p>
        <p className="m-0 text-sm text-muted-foreground">{step.body}</p>

        {missing && (
          <p className="mt-2 mb-0 text-sm text-muted-foreground" data-tour-skip="absent">
            This one is not on the page right now, so there is nothing to point at. Skipping it.
          </p>
        )}

        <div className="mt-3 flex flex-wrap gap-2">
          <Button variant="secondary" size="sm" onClick={onNext} data-tour-next>
            {missing ? 'Skip this step' : 'Next'}
          </Button>
          <Button variant="ghost" size="sm" onClick={onStop} data-tour-stop>
            End tour
          </Button>
        </div>
      </div>
    </div>
  )
}

/** The one visible affordance that starts it. Nothing else may. */
export function TourButton({ onStart }: { onStart: () => void }) {
  return (
    <Button variant="ghost" size="sm" onClick={onStart} data-tour-start>
      Show me around
    </Button>
  )
}
