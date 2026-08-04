/**
 * The primitive kit — ONE home, and the export contract UI-2..7 build on
 * (P3/CONVENTIONS.md §47).
 *
 * The kit styles surfaces. It never re-opens §42's honesty rules: absence,
 * money and freshness stay `parts.tsx`'s, and there is no progress, percentage
 * or horizon primitive here — the API serves none, so no component could be
 * rendering one from data.
 *
 * A rename after this packet is a recorded deviation, not a drift.
 */
export { cn } from '../lib/utils'
export { Button, type ButtonProps } from './Button'
export { Chip, type ChipProps } from './Chip'
export { Drawer, type DrawerProps } from './Drawer'
export { EmptyState, type EmptyStateProps } from './EmptyState'
export { Modal, type ModalProps } from './Modal'
export { Panel, type PanelProps } from './Panel'
export { Skeleton } from './Skeleton'
export { StatTile, type StatTileProps } from './StatTile'
export { StatusDot, type StatusDotProps } from './StatusDot'
export { Timestamp, type TimestampProps } from './Timestamp'
export { ToastProvider, useToast, type ToastSeverity } from './Toast'
export { toneStyle, tones, type Tone } from './tone'
