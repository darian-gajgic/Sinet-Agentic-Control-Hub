import type { ReactNode } from 'react'

import { cn } from '../lib/utils'
import { toneStyle, type Tone } from './tone'

export type StatTileProps = {
  /** The tracked-uppercase micro-label above the figure. */
  label: string
  /** The figure, rendered EXACTLY as given. */
  value: ReactNode
  icon?: ReactNode
  tone?: Tone
  /** One line under the figure — what the figure is of, never a second figure. */
  foot?: ReactNode
  className?: string
}

/**
 * The signature tile: a 2px tone-coloured hairline across the top via ::before,
 * a tracked-uppercase micro-label, an icon chip on the soft gradient, and a
 * 30px mono tabular figure (P3/CONVENTIONS.md §47).
 *
 * NO COUNT-UP. Nexus animated tiles from zero to their value on mount, and a
 * count-up transforms a DISPLAYED figure — for the seconds it runs, the screen
 * shows a number the platform never served. Whether that is admissible against
 * the no-fabricated-numbers invariant is a surface decision, not a kit default,
 * so the tile renders what it is given from the first frame.
 */
export function StatTile({ label, value, icon, tone = 'accent', foot, className }: StatTileProps) {
  return (
    <div
      style={toneStyle(tone)}
      className={cn(
        'relative overflow-hidden rounded-(--radius) border border-border',
        'bg-(image:--panel-grad)',
        'p-(--density-pad)',
        'before:absolute before:inset-x-0 before:top-0 before:h-0.5 before:bg-(--tone) before:content-[""]',
        className,
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="font-mono text-[9.5px] font-bold tracking-[2px] text-muted-foreground uppercase">{label}</span>
        {icon !== undefined && (
          <span className="flex size-8 flex-none items-center justify-center rounded-(--radius-sm) bg-(image:--grad-soft) text-(--tone)">
            {icon}
          </span>
        )}
      </div>
      <div className="mt-2 font-mono text-[30px] leading-none font-bold tabular-nums">{value}</div>
      {foot !== undefined && <div className="mt-2 text-xs text-muted-foreground">{foot}</div>}
    </div>
  )
}
