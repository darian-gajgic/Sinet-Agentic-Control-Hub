import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/**
 * cn joins conditional class names and resolves Tailwind conflicts last-wins.
 *
 * Both halves matter. clsx is what lets a variant be expressed as a condition;
 * tailwind-merge is what lets a CALLER's utility beat the variant's, which is
 * the whole reason a kit component may take a `className` at all — without it,
 * `<Button className="w-full">` would lose to whichever width the variant
 * happened to emit, depending on stylesheet order rather than on intent.
 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}
