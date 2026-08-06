import { TriangleAlert } from 'lucide-react'

import type { RouteID } from './routes'

/**
 * The fence rule (process P1, bound 2026-08-06 after checkpoint 2 FAILED).
 *
 * At checkpoint 2 the operator walked the whole app and judged old rejected
 * surfaces as if they were the rework — because nothing in the app marked
 * which was which. This banner is the boundary: every surface still showing
 * the OLD (rejected) UI declares itself, in plain words, with its scheduled
 * rebuild step from the product map §7 (v3), and names its already-known
 * defects UP FRONT so a documented issue is never re-discovered as a fresh
 * failure.
 *
 * A surface leaves this registry in the same commit that replaces it —
 * keeping a fence on a rebuilt surface would be the opposite lie.
 */
type Fence = {
  /** The map §7 (v3) build-order step that rebuilds this surface. */
  step: number
  /** What that step is called on the map, in the operator's words. */
  act: string
  /** Defects already on the books for this surface, stated up front. */
  known?: string[]
}

export const fencedSurfaces: Partial<Record<RouteID, Fence>> = {
  // `board` and `task` LEFT the registry 2026-08-06 in the commits that
  // replaced them (the real Kanban; the overlay card) — the fence rule's own
  // clause.
  inbox: {
    step: 3,
    act: 'Decide — the inbox in plain words',
    known: ['the row language is jargon ("opt in / opt out", benchmark verdict cards) — the rebuild renders every row in plain words'],
  },
  'inbox-item': { step: 3, act: 'Decide — the inbox in plain words' },
  deliverable: {
    step: 4,
    act: 'Judge — reviews with Run-it previews',
  },
  fleet: { step: 5, act: 'Steer & insight — Fleet, Health, Wins & Lessons, Memory, History' },
  memory: { step: 5, act: 'Steer & insight — Fleet, Health, Wins & Lessons, Memory, History' },
  'memory-entry': { step: 5, act: 'Steer & insight — Fleet, Health, Wins & Lessons, Memory, History' },
  workforce: {
    step: 6,
    act: 'Specialists, Settings, Manual, Assistant',
    known: ['text can break one letter per line in narrow cards — recorded at checkpoint 2, fixed by the rebuild'],
  },
  settings: { step: 6, act: 'Specialists, Settings, Manual, Assistant' },
  chat: {
    step: 6,
    act: 'Specialists, Settings, Manual, Assistant',
    known: [
      'the inline interview’s "Answer here" submit looks dead and the mode radios collide (finding 5) — the NEW "Describe a goal" door is the working way to hand over work',
      'the assistant’s conversational brain arrives at bring-up — this surface cannot chat yet, by scope',
    ],
  },
}

/**
 * The banner itself. Unmissable by design: it leads the page, carries the
 * warning glyph, and says three plain things — this is the old version, the
 * rebuild is scheduled (step N), and here is what is already known to be
 * wrong with it.
 */
export function OldFence({ route }: { route: RouteID }) {
  const f = fencedSurfaces[route]
  if (!f) return null
  return (
    <aside className="old-fence" role="note" data-fence={route}>
      <TriangleAlert className="fence-ico" size={17} strokeWidth={2} aria-hidden="true" />
      <div className="fence-body">
        <p className="fence-head">
          Old version — not the rework. Its rebuild is scheduled: <b>step {String(f.step)}</b> ({f.act}).
        </p>
        <p className="fence-sub">
          What you see below is the previous UI, kept reachable so no data or verb disappears meanwhile. Please judge
          the rework on the rebuilt surfaces only.
        </p>
        {(f.known ?? []).map((k) => (
          <p className="fence-known" key={k}>
            Known already: {k}
          </p>
        ))}
      </div>
    </aside>
  )
}
