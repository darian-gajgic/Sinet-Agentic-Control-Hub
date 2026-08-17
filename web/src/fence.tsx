import { TriangleAlert } from 'lucide-react'

import type { RouteID } from './routes'

/**
 * The fence rule (process P1, bound 2026-08-06 after checkpoint 2 FAILED).
 *
 * At checkpoint 2 the operator walked the whole app and judged old rejected
 * surfaces as if they were the rework — because nothing in the app marked
 * which was which. This banner is the boundary: every surface still awaiting
 * its rebuild declares itself.
 *
 * REWORDED FOR THE HOUSEHOLD (RA-2/RA-B1 item 5). The original banner spoke
 * the build process's own dialect — "not the rework", step numbers, finding
 * numbers — which reads as a confession to a customer, not a courtesy. The
 * rule now: the banner says, in plain words, that some controls here are
 * still being rebuilt, and names what may not work — WITHOUT internal
 * numbering. The step/act fields stay in this registry for the build's own
 * tracking; they no longer render.
 *
 * A surface leaves this registry in the same commit that replaces it —
 * keeping a fence on a rebuilt surface would be the opposite lie.
 */
type Fence = {
  /** The map §7 (v3) build-order step that rebuilds this surface — registry
   *  bookkeeping only, never rendered. */
  step: number
  /** What that step is called on the map — registry bookkeeping only. */
  act: string
  /** What may not work yet, in plain household words, stated up front. */
  known?: string[]
}

export const fencedSurfaces: Partial<Record<RouteID, Fence>> = {
  // `board` and `task` LEFT the registry 2026-08-06 in the commits that
  // replaced them (the real Kanban; the overlay card) — the fence rule's own
  // clause. `inbox`/`inbox-item` left 2026-08-17 (coldwalk W1-8): the step-3
  // rework had long since landed on them. `deliverable` left 2026-08-17 in
  // the RA-B1 commit that rebuilt the owner's moment (result-first render,
  // download door, plain-words accept) — the review surface is the rework now.
  fleet: { step: 5, act: 'Steer & insight — Fleet, Health, Wins & Lessons, Memory, History' },
  memory: { step: 5, act: 'Steer & insight — Fleet, Health, Wins & Lessons, Memory, History' },
  'memory-entry': { step: 5, act: 'Steer & insight — Fleet, Health, Wins & Lessons, Memory, History' },
  workforce: {
    step: 6,
    act: 'Specialists, Settings, Manual, Assistant',
    known: ['on a narrow screen, some text in the small cards can wrap one letter per line — the rebuild fixes it'],
  },
  settings: { step: 6, act: 'Specialists, Settings, Manual, Assistant' },
  chat: {
    step: 6,
    act: 'Specialists, Settings, Manual, Assistant',
    known: [
      'the "Answer here" box on this page can look unresponsive — to hand over new work, use the "Describe a goal" button instead; it is the way that works',
      'the assistant cannot hold a conversation yet — that part of the platform has not been switched on',
    ],
  },
}

/**
 * The banner itself. It says three plain things and nothing else: some
 * controls here are still being rebuilt, everything shown is real, and here —
 * in plain words — is what may not work yet.
 */
export function OldFence({ route }: { route: RouteID }) {
  const f = fencedSurfaces[route]
  if (!f) return null
  return (
    <aside className="old-fence" role="note" data-fence={route}>
      <TriangleAlert className="fence-ico" size={17} strokeWidth={2} aria-hidden="true" />
      <div className="fence-body">
        <p className="fence-head">Some controls on this page are still being rebuilt.</p>
        <p className="fence-sub">
          Everything shown here is real and current — nothing is made up — but a few controls may be clumsy or not work
          yet. The rebuilt version of this page is on its way.
        </p>
        {(f.known ?? []).map((k) => (
          <p className="fence-known" key={k}>
            Worth knowing: {k}
          </p>
        ))}
      </div>
    </aside>
  )
}
