import type { HTMLAttributes } from 'react'

import { cn } from '../lib/utils'

/**
 * A loading placeholder — a shape, never a figure.
 *
 * It carries no number, no fraction and no horizon: the platform cannot know
 * how far along a read is, and a bar that filled would be inventing one (§42).
 * The shimmer is `motion-safe`, so a reduce reader gets the same static block.
 * `aria-hidden` because a screen reader gains nothing from a grey rectangle;
 * the region it stands in announces its own busy state.
 */
export function Skeleton({ className, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      aria-hidden="true"
      className={cn(
        'rounded-(--radius-sm) bg-muted',
        'motion-safe:animate-pulse',
        'h-4 w-full',
        className,
      )}
      {...rest}
    />
  )
}
