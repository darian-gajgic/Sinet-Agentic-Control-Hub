import { Toast as BaseToast } from '@base-ui/react/toast'
import type { ReactNode } from 'react'

import { cn } from '../lib/utils'
import { toneStyle, type Tone } from './tone'

/**
 * The toast stack: top-right, blurred, a tone-coloured left border, at most
 * four visible.
 *
 * No toast LIBRARY entered the tree. The runtime set the design proposal fixed
 * is closed, and `@base-ui/react` already carries the hard parts — the live
 * region, the hover/focus pause, the swipe dismissal and the stacking — so a
 * second dependency would have bought styling this file does in thirty lines.
 */

/** The severities a toast can carry, mapped onto the shared tone tokens. */
const severityTone: Record<'info' | 'success' | 'warning' | 'error', Tone> = {
  info: 'blue',
  success: 'green',
  warning: 'yellow',
  error: 'red',
}

export type ToastSeverity = keyof typeof severityTone

/**
 * At most four toasts are visible at once.
 *
 * A structural constant with a named reason (no ⚙ key exists — the §41-B
 * precedent): four is the most a reader can take in before the stack stops
 * being a notification and becomes a wall, and Base UI marks the overflow
 * `data-limited` rather than dropping it, so nothing is silently lost.
 */
const visibleToasts = 4

export function ToastProvider({ children }: { children: ReactNode }) {
  return (
    <BaseToast.Provider limit={visibleToasts}>
      {children}
      <BaseToast.Portal>
        <BaseToast.Viewport className="fixed top-4 right-4 z-50 flex w-[calc(100%-2rem)] max-w-sm flex-col gap-2">
          <ToastList />
        </BaseToast.Viewport>
      </BaseToast.Portal>
    </BaseToast.Provider>
  )
}

/**
 * useToast is the kit's toast API: severity-toned and action-capable.
 *
 * Re-exported through the kit rather than making every call site import Base
 * UI directly, so the behaviour layer stays swappable behind one name.
 */
export function useToast() {
  return BaseToast.useToastManager()
}

function ToastList() {
  const { toasts } = BaseToast.useToastManager()
  return toasts.map((toast) => {
    const tone = severityTone[(toast.type as ToastSeverity) ?? 'info'] ?? severityTone.info
    return (
      <BaseToast.Root
        key={toast.id}
        toast={toast}
        style={toneStyle(tone)}
        className={cn(
          'rounded-(--radius-sm) border border-border border-l-4 border-l-(--tone)',
          'bg-popover/85 p-3 text-sm shadow-(--shadow-soft) backdrop-blur-[var(--blur-overlay)]',
          // NO css motion here (transition or keyframe): the Modal's
          // stuck-exit finding (2026-08-16) — any declared motion makes
          // base-ui wait forever at close, which here would pile invisible
          // toasts over the corner of every page.
          // The overflow beyond the visible limit is hidden rather than dropped.
          'data-limited:hidden',
        )}
      >
        <BaseToast.Title className="m-0 font-medium" />
        <BaseToast.Description className="m-0 mt-1 text-xs text-muted-foreground" />
        <BaseToast.Action className="mt-2 text-xs text-(--tone) underline underline-offset-2" />
        <BaseToast.Close
          aria-label="Dismiss"
          className="absolute top-2 right-2 text-xs text-muted-foreground hover:text-foreground"
        >
          ×
        </BaseToast.Close>
      </BaseToast.Root>
    )
  })
}
