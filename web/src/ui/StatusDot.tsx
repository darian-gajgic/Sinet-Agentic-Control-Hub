import type { HTMLAttributes } from 'react'

import { cn } from '../lib/utils'
import { toneStyle, type Tone } from './tone'

export type StatusDotProps = HTMLAttributes<HTMLSpanElement> & {
  tone?: Tone
  /**
   * Pulse. Reserved for things that are GENUINELY live — a pulsing dot beside
   * stale data is the stale-poses-as-live failure S15.12 forbids, drawn in
   * animation. Gated by `motion-safe`, so a reduce reader gets the same glowing
   * dot standing still.
   */
  live?: boolean
}

/** An 8px dot that carries its severity in its glow, so state is readable
 *  peripherally without reading the label beside it. */
export function StatusDot({ tone = 'accent', live = false, className, style, ...rest }: StatusDotProps) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'inline-block size-2 flex-none rounded-full bg-(--tone) shadow-[0_0_8px_var(--tone)]',
        live && 'motion-safe:animate-pulse',
        className,
      )}
      style={{ ...toneStyle(tone), ...style }}
      {...rest}
    />
  )
}
