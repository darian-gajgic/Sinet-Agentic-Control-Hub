import { Dialog } from '@base-ui/react/dialog'
import type { ReactNode } from 'react'

import { cn } from '../lib/utils'

export type DrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  children?: ReactNode
  className?: string
}

/**
 * The right sheet: full width on a phone, 520px from the breakpoint up.
 *
 * 520px is a structural constant with a named reason (no ⚙ key exists and the
 * browser cannot read the registry before login — the §41-B precedent): it is
 * wide enough for a provenance line and a mono id to sit on one line, and
 * narrow enough that the surface behind it stays readable, which is the whole
 * point of a sheet rather than a modal. Full width below that, because a
 * 520px sheet on a 375px screen is a modal with extra steps.
 *
 * Same Base UI dialog behaviour as `Modal` — a drawer is a dialog that arrives
 * from the side, not a different set of rules about focus and escape.
 */
export function Drawer({ open, onOpenChange, title, children, className }: DrawerProps) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop
          className={cn(
            'fixed inset-0 z-40 bg-black/55 backdrop-blur-[var(--blur-overlay)]',
            'transition-opacity duration-200 data-[ending-style]:opacity-0 data-[starting-style]:opacity-0',
          )}
        />
        <Dialog.Popup
          className={cn(
            'fixed inset-y-0 right-0 z-50 flex w-full flex-col sm:w-[520px]',
            'border-l border-border bg-popover text-popover-foreground shadow-(--shadow)',
            'transition-transform duration-250 data-[ending-style]:translate-x-full data-[starting-style]:translate-x-full',
            className,
          )}
        >
          <div className="flex items-center justify-between gap-2 border-b border-border px-(--density-pad) py-(--density-pad-y)">
            <Dialog.Title className="m-0 text-[15px] font-bold">{title}</Dialog.Title>
            <Dialog.Close className="rounded-(--radius-sm) px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground">
              Close
            </Dialog.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-(--density-pad)">{children}</div>
        </Dialog.Popup>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
