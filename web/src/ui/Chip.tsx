import type { HTMLAttributes } from 'react'

import { cn } from '../lib/utils'
import { toneStyle, type Tone } from './tone'

/**
 * THE chip formula, written once: `color: X; background: rgba(X,.09);
 * border-color: rgba(X,.25)` over `--tone` (P3/CONVENTIONS.md §47).
 *
 * Seven tones share this one string. A hue that wanted its own rule would be
 * the moment the palette stopped being a palette.
 */
const chipFormula =
  'text-(--tone) bg-[color-mix(in_srgb,var(--tone)_9%,transparent)] ' +
  'border-[color-mix(in_srgb,var(--tone)_25%,transparent)]'

export type ChipProps = HTMLAttributes<HTMLSpanElement> & { tone?: Tone }

export function Chip({ tone = 'accent', className, style, children, ...rest }: ChipProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5',
        'font-mono text-[11px] tracking-wide uppercase tabular-nums',
        chipFormula,
        className,
      )}
      style={{ ...toneStyle(tone), ...style }}
      {...rest}
    >
      {children}
    </span>
  )
}
