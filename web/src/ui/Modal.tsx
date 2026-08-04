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
 */
export function Modal({ open, onOpenChange, title, description, children, footer, className }: ModalProps) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop
          className={cn(
            'fixed inset-0 z-40 bg-black/55 backdrop-blur-[var(--blur-overlay)]',
            'motion-safe:transition-opacity duration-200 data-[ending-style]:opacity-0 data-[starting-style]:opacity-0',
          )}
        />
        <Dialog.Popup
          className={cn(
            'fixed top-1/2 left-1/2 z-50 w-[calc(100%-2rem)] max-w-lg -translate-x-1/2 -translate-y-1/2',
            'rounded-(--radius) border border-border bg-popover text-popover-foreground shadow-(--shadow)',
            'p-(--density-pad)',
            'motion-safe:transition-[opacity,scale] duration-200 ease-[cubic-bezier(0.34,1.56,0.64,1)]',
            'data-[starting-style]:scale-95 data-[starting-style]:opacity-0',
            'data-[ending-style]:scale-95 data-[ending-style]:opacity-0',
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
