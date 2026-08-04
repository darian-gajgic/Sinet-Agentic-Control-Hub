import type { ReactNode } from 'react'

import { cn } from '../lib/utils'

export type EmptyStateProps = {
  /** WHAT will appear here. */
  what: string
  /** WHY it will appear — the teaching half, and the reason this exists. */
  why?: ReactNode
  /** A faint glyph above the sentence. Decoration, hidden from readers. */
  glyph?: ReactNode
  /** One act, where there genuinely is one. */
  action?: ReactNode
  className?: string
}

/**
 * The teaching empty state (design proposal §3, the A-1/A-2 fix).
 *
 * An empty surface is the one moment a reader has no evidence of what the
 * surface is for, so "Nothing here" is the least useful sentence it could
 * print. `what` names what will appear and `why` says what causes it to —
 * Nexus's own pattern ("No pending approvals — agents will queue risky actions
 * here for your sign-off").
 *
 * Per-surface adoption is UI-5..7; this packet ships the primitive.
 */
export function EmptyState({ what, why, glyph, action, className }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center gap-2 px-4 py-10 text-center', className)}>
      {glyph !== undefined && (
        <span aria-hidden="true" className="text-[26px] text-[var(--text-faint)]">
          {glyph}
        </span>
      )}
      <p className="m-0 text-sm text-foreground">{what}</p>
      {why !== undefined && <p className="m-0 max-w-prose text-xs text-muted-foreground">{why}</p>}
      {action !== undefined && <div className="mt-2">{action}</div>}
    </div>
  )
}
