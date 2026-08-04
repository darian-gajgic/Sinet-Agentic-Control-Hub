import type { HTMLAttributes } from 'react'

import { cn } from '../lib/utils'

/**
 * A loading placeholder — a shape, never a figure.
 *
 * It carries no number, no fraction and no horizon: the platform cannot know
 * how far along a read is, and a bar that filled would be inventing one (§42).
 * That is the difference between this and a progress indicator, and it is why
 * the sweep travels rather than fills — a travelling band cannot be misread as
 * a position, because it never stops anywhere.
 *
 * The sweep is authored under `motion-safe`, so a reduce reader gets a flat
 * panel-neutral block instead of a neutralized animation. `aria-hidden` because
 * a screen reader gains nothing from a grey rectangle; the region it stands in
 * announces its own busy state.
 */
export function Skeleton({ className, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      aria-hidden="true"
      className={cn(
        'rounded-(--radius-sm) bg-muted',
        'bg-(image:--skeleton-sweep) bg-size-[200%_100%]',
        'motion-safe:animate-[skeleton-shimmer_1.6s_linear_infinite]',
        'h-4 w-full',
        className,
      )}
      {...rest}
    />
  )
}
