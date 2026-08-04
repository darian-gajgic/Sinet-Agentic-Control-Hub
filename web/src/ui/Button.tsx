import { cva, type VariantProps } from 'class-variance-authority'
import type { ButtonHTMLAttributes } from 'react'

import { cn } from '../lib/utils'

/**
 * The one button. Primary carries the identity gradient and its violet glow;
 * everything else recedes, because a screen where three things shout has no
 * primary act (P3/CONVENTIONS.md §47).
 */
const button = cva(
  'inline-flex items-center justify-center gap-2 rounded-(--radius-sm) border font-medium ' +
    'transition-[background,border-color,box-shadow,transform] ' +
    'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring ' +
    'disabled:pointer-events-none disabled:opacity-45',
  {
    variants: {
      variant: {
        primary:
          'border-transparent bg-(image:--grad) text-primary-foreground shadow-[0_6px_18px_var(--accent-glow)] ' +
          'hover:shadow-[0_10px_26px_var(--accent-glow)] motion-safe:hover:-translate-y-px',
        secondary: 'border-border bg-secondary text-foreground hover:bg-accent',
        ghost: 'border-transparent bg-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
        danger:
          'border-[color-mix(in_srgb,var(--red)_25%,transparent)] bg-[color-mix(in_srgb,var(--red)_9%,transparent)] ' +
          'text-[var(--red)] hover:bg-[color-mix(in_srgb,var(--red)_16%,transparent)]',
      },
      size: {
        sm: 'h-8 px-3 text-xs',
        md: 'h-9 px-4 text-sm',
      },
    },
    defaultVariants: { variant: 'secondary', size: 'md' },
  },
)

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & VariantProps<typeof button>

export function Button({ className, variant, size, type = 'button', ...rest }: ButtonProps) {
  // `type` defaults to "button": an unqualified <button> inside a form submits
  // it, which is how a kit turns a cancel into a submit at somebody else's call
  // site.
  return <button type={type} className={cn(button({ variant, size }), className)} {...rest} />
}
