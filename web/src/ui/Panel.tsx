import type { HTMLAttributes, ReactNode } from 'react'

import { cn } from '../lib/utils'

/**
 * The one glass material: a translucent violet-tinted gradient over the
 * near-black base, a 1px border of the same family, radius 14, and a hover lift
 * (P3/CONVENTIONS.md §47).
 *
 * It deliberately does NOT reuse the landed `.panel` class, which already
 * exists with its own meaning — the kit styles with utilities so a later packet
 * can shed a surface's CSS without a name collision deciding what a box looks
 * like.
 */
export type PanelProps = HTMLAttributes<HTMLDivElement> & {
  /** A panel header row, above the body and below the top border. */
  head?: ReactNode
}

export function Panel({ className, head, children, ...rest }: PanelProps) {
  return (
    <div
      className={cn(
        'rounded-(--radius) border border-border',
        'bg-(image:--panel-grad)',
        'motion-safe:transition-[border-color,transform,box-shadow]',
        'hover:border-[var(--border-l)] hover:shadow-(--shadow) motion-safe:hover:-translate-y-0.5',
        className,
      )}
      {...rest}
    >
      {head !== undefined && (
        <div className="flex flex-wrap items-center gap-2 border-b border-border px-(--density-pad) py-(--density-pad-y)">
          {head}
        </div>
      )}
      <div className="p-(--density-pad)">{children}</div>
    </div>
  )
}
