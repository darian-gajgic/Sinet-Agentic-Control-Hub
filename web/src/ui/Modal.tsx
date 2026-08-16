import { Dialog } from '@base-ui/react/dialog'
import type { ReactNode } from 'react'

import { cn } from '../lib/utils'

export type ModalProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  /** One line under the title saying what this asks of the reader. */
  description?: ReactNode
  children?: ReactNode
  /** The act row, bottom-right. */
  footer?: ReactNode
  className?: string
}

/**
 * A centred modal over a blurred backdrop.
 *
 * Behaviour — focus trap, the escape stack, portalling, the ARIA wiring — is
 * `@base-ui/react`'s; the look is ours. That split is the whole reason the
 * primitive layer was adopted: none of those are things a control room should
 * re-implement, and all of them are things a reader notices only when broken.
 *
 * The entry overshoots slightly under `no-preference` and is a plain appearance
 * under `reduce`, which the global kill switch guarantees regardless.
 *
 * ⚠ THE ROOT MOUNTS ONLY WHILE OPEN, AND NO CSS MOTION MAY BE DECLARED ON
 * THE POPUP OR BACKDROP (found live 2026-08-16, deterministic on this stack,
 * four probes deep): @base-ui 1.7.0's own close-unmount NEVER COMPLETES here
 * — with transition classes, with a one-shot entry keyframe (it restarts at
 * close and freezes at `currentTime: 0`), and even with zero motion declared,
 * the closed popup sat in `data-ending-style` forever. The corpse's backdrop
 * — opacity 0, pointer-events auto — stayed over the whole page as an
 * INVISIBLE CLICK SHIELD: every press died silently, the operator's "I press
 * on something and nothing happens" class. So closing is OURS: `open=false`
 * returns null and React unmounts the whole subtree synchronously; the
 * library's close-wait machinery is never on the path. Dialogs appear and
 * vanish instantly — standard, and structurally incapable of stranding a
 * shield.
 */
export function Modal({ open, onOpenChange, title, description, children, footer, className }: ModalProps) {
  if (!open) return null
  return (
    <Dialog.Root open onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop
          className="fixed inset-0 z-40 bg-black/55 backdrop-blur-[var(--blur-overlay)]"
        />
        <Dialog.Popup
          className={cn(
            'fixed top-1/2 left-1/2 z-50 w-[calc(100%-2rem)] max-w-lg -translate-x-1/2 -translate-y-1/2',
            'rounded-(--radius) border border-border bg-popover text-popover-foreground shadow-(--shadow)',
            'p-(--density-pad)',
            className,
          )}
        >
          <Dialog.Title className="m-0 text-[15px] font-bold">{title}</Dialog.Title>
          {description !== undefined && (
            <Dialog.Description className="mt-1 mb-0 text-xs text-muted-foreground">{description}</Dialog.Description>
          )}
          {children !== undefined && <div className="mt-3">{children}</div>}
          {footer !== undefined && <div className="mt-4 flex flex-wrap justify-end gap-2">{footer}</div>}
        </Dialog.Popup>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
