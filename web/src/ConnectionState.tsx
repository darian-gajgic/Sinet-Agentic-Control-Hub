import type { Status } from './events'

/**
 * Connection state is ALWAYS visible (Spec S15.12/S15.3): a surface whose feed
 * is disconnected or catching up says so, and stale data never poses as live.
 * The indicator renders on every route, login included.
 */
const labels: Record<Status, { label: string; detail: string }> = {
  connecting: { label: 'Connecting', detail: 'opening the live feed' },
  'catching-up': { label: 'Catching up', detail: 'loading current state before tailing' },
  live: { label: 'Live', detail: 'tailing the event stream' },
  disconnected: { label: 'Disconnected', detail: 'the control plane answered but the feed dropped' },
  'logged-out': { label: 'Signed out', detail: 'sign in to resume the live feed' },
  unreachable: { label: 'Unreachable', detail: 'the host is asleep or the tailnet is down' },
}

export function ConnectionState({ status }: { status: Status }) {
  const { label, detail } = labels[status]
  return (
    <p className="conn" data-status={status} title={detail} aria-live="polite">
      <span className="conn-dot" aria-hidden="true" />
      <span className="conn-label">{label}</span>
      <span className="conn-detail">{detail}</span>
    </p>
  )
}
