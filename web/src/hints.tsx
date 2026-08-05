import { useState } from 'react'

import { Button } from './ui'

/**
 * One-shot first-use hints (design proposal §3, as narrowed by its own §7
 * amendment A1.2 dated 2026-08-05; P3-UI-7 R14).
 *
 * THE NARROWING, AND WHY IT IS THE ONLY HONEST SHAPE. The proposal says
 * "one-shot". A genuinely once-ever hint needs somewhere to remember it was
 * shown, and this platform has nowhere: §41-B bans web storage outright, no
 * server-side per-person UI-state store exists, and inventing a ⚙ key or a
 * migration for a dismissed tooltip is far outside the freeze. So "one-shot"
 * means ONCE PER TAB SESSION, held in the module-scope set below. A full reload
 * shows them again, and that is recorded as a dated deviation rather than
 * papered over — the same reason the tour never auto-starts.
 *
 * A HINT NEVER COVERS THE CONTROL IT NAMES. It renders in normal flow BESIDE
 * the thing it explains — no overlay, no absolute positioning, no portal — so
 * the control it points at stays clickable while the hint is on screen. A tip
 * you have to dismiss before you can do the thing it is describing is worse
 * than no tip.
 *
 * THE COUNT STAYS SMALL. The teaching layer is the tour plus the empty states;
 * hints are seasoning. Two shipped here, and both sit on surfaces this packet
 * legitimately opens — `Inbox.tsx` and `MissionControl.tsx` are byte-frozen for
 * this packet, so a hint on either would have been a wall break rather than a
 * feature, and that bound is recorded rather than worked around.
 */

/**
 * The seen set. MODULE SCOPE, deliberately: it is memory for this tab and this
 * load, and it is the whole persistence story. Nothing is written anywhere.
 */
const seen = new Set<string>()

/** Test-only reset, so a suite can drive the first-use path more than once. */
export function resetHints(): void {
  seen.clear()
}

export function hintSeen(id: string): boolean {
  return seen.has(id)
}

export function Hint({ id, children }: { id: string; children: React.ReactNode }) {
  const [dismissed, setDismissed] = useState(() => seen.has(id))
  if (dismissed || seen.has(id)) return null
  return (
    <p
      className="m-0 flex flex-wrap items-baseline gap-2 rounded-(--radius-sm) border border-border bg-card/40 px-3 py-2 text-sm text-muted-foreground"
      data-hint={id}
    >
      <span>{children}</span>
      <Button
        variant="ghost"
        size="sm"
        data-hint-dismiss={id}
        onClick={() => {
          seen.add(id)
          setDismissed(true)
        }}
      >
        Got it
      </Button>
    </p>
  )
}
