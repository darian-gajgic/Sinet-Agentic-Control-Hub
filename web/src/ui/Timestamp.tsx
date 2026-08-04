import { cn } from '../lib/utils'
import { Absent } from '../parts'

/**
 * The D5 timestamp primitive (B6 gate §9 D5 answer).
 *
 * Every instant this platform stores and serves is UTC RFC3339, and the
 * verbatim string is the thing that is TRUE — "3m ago" is a convenience
 * computed against a device clock nobody controls. So the two variants differ
 * in what they ADD, never in what they drop:
 *
 *   live  — relative time BESIDE the verbatim UTC. The UTC is never replaced.
 *   audit — verbatim UTC alone, which is what a record needs.
 *
 * Absence is rendered, not filled: an empty stamp says so through the landed
 * `Absent` primitive rather than printing an epoch or a blank (§42).
 *
 * THIS PACKET SHIPS THE PRIMITIVE ONLY. No landed call site is migrated —
 * `parts.tsx`'s `Stamp` still serves every audit-style site it already served —
 * and applying `live` across the live surfaces is UI-4's row.
 */
export type TimestampProps = {
  ts: string | null | undefined
  variant?: 'live' | 'audit'
  className?: string
}

export function Timestamp({ ts, variant = 'audit', className }: TimestampProps) {
  const relative = variant === 'live' ? relativeTo(ts) : ''

  if (!ts) return <Absent reason="not recorded" />

  const stamp = (
    <time dateTime={ts} className="font-mono tabular-nums">
      {ts}
    </time>
  )
  // An unparseable stamp still renders verbatim and gains nothing: the served
  // string is the record, and inventing a relative label for an instant we
  // could not read would be the one thing this primitive must not do.
  if (variant === 'audit' || relative === '') return <span className={className}>{stamp}</span>

  return (
    <span className={cn('inline-flex flex-wrap items-baseline gap-1.5', className)}>
      <span>{relative}</span>
      <span className="text-xs text-muted-foreground">{stamp}</span>
    </span>
  )
}

/**
 * relativeTo derives the label AT RENDER, against the device clock.
 *
 * REPORTED DEVIATION from the D5 brief's "refreshes on a coarse interval": it
 * holds no timer, because the client may not own one. §32's no-ticker rule is
 * scanned for tree-wide (`sweep.test.tsx`, `chat.test.tsx`) on the ground that
 * a timer is a second, competing source of currency that keeps running when the
 * feed is down — stale posing as live.
 *
 * A relative label is exactly that hazard in miniature: a ticking "3m ago"
 * beside data the platform stopped receiving is the one thing S15.12 forbids.
 * So the label re-derives whenever the surface re-renders, and on a live
 * surface the FEED is what drives that — currency comes from frames here as it
 * does everywhere else. When the feed stops, the label stops with the data it
 * describes, and the connection pill says why.
 */
function relativeTo(ts: string | null | undefined): string {
  if (!ts) return ''
  const served = Date.parse(ts)
  if (Number.isNaN(served)) return ''
  return relativeLabel(served, Date.now())
}

/** relativeLabel renders the gap between a served instant and the device clock
 *  in the coarsest unit that still says something. */
export function relativeLabel(servedMs: number, nowMs: number): string {
  const gap = nowMs - servedMs
  const ahead = gap < 0
  const seconds = Math.floor(Math.abs(gap) / 1000)
  if (seconds < 45) return ahead ? 'in a moment' : 'just now'

  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)
  const size = days >= 1 ? `${days}d` : hours >= 1 ? `${hours}h` : `${Math.max(minutes, 1)}m`
  return ahead ? `in ${size}` : `${size} ago`
}
