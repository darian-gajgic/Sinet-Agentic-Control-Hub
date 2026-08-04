import type { CSSProperties } from 'react'

/**
 * The seven tones every status-carrying primitive speaks: the identity violet
 * plus the six status hues (P3/CONVENTIONS.md §47).
 *
 * A tone is a TOKEN NAME, never a colour. That is what keeps the palette in one
 * place — a primitive cannot invent an eighth hue, and re-pointing a token
 * re-points every chip, dot and tile that names it.
 */
export type Tone = 'accent' | 'green' | 'yellow' | 'red' | 'blue' | 'orange' | 'pink'

export const tones: Tone[] = ['accent', 'green', 'yellow', 'red', 'blue', 'orange', 'pink']

/**
 * toneStyle binds `--tone` to the token the tone names.
 *
 * Every tone-aware primitive then applies the SAME formula over `var(--tone)`
 * rather than one rule per hue — which is what "the one chip formula" means
 * mechanically, and why adding a tone is a one-line change here rather than a
 * seven-way edit at each call site.
 */
export function toneStyle(tone: Tone): CSSProperties {
  return { '--tone': `var(--${tone})` } as CSSProperties
}
