import { useCallback, useEffect, useState } from 'react'

import { api, type PushSubscriptionsView } from './api'
import type { EventStream } from './events'
import { describeError, useLive } from './live'
import { Absent, Freshness, Owner, Stamp } from './parts'
import { Panel } from './settingsForm'
import { Button, EmptyState } from './ui'
import { availability, enrol, readEnv, thisDevice, unenrol, type PushEnv } from './push'

/**
 * The "This device" panel (Spec S15.11; B6-9 OQ10).
 *
 * IT SITS BESIDE THE GENERATED FORM, NOT IN IT. The settings tab is emitted from
 * the ⚙ registry and nothing in this tree hand-builds a control over a registry
 * key (§43-B). Push enrolment is not a registry key at all — it is per-DEVICE
 * state that lives in the browser and in a table — so it renders as its own
 * panel and touches no ⚙ key whatsoever. A dedicated route was rejected: it
 * would grow the URL contract for one control.
 *
 * NOTHING RUNS ON MOUNT THAT COULD ASK FOR PERMISSION. A permission prompt has
 * to come from a user gesture (Safari has required that since web push landed
 * there), so `enrol` is called from a click handler and from nowhere else — the
 * mount only reads what already exists.
 *
 * EVERY UNAVAILABLE STATE RENDERS ITS OWN REASON. Not supported, not installed
 * on iOS, blocked in the browser, and enrolled-elsewhere are four different
 * facts, and a person acts differently on each.
 */
export function PushDevices({ stream, env }: { stream?: EventStream; env?: PushEnv }) {
  const live = useLive<PushSubscriptionsView>({
    key: '/api/push/subscriptions',
    read: () => api.pushSubscriptions(),
    types: pushEventTypes,
    stream,
  })
  return (
    <Panel title="Notifications on this device">
      {/* The panel-grain "what this is" line. This is a PANEL inside settings
          rather than a route of its own, so it gets a what-line without the
          `SurfaceHead` route treatment. It states only what enrolment is —
          per-DEVICE state, not a ⚙ setting — and leaves what actually leaves
          this machine to the SERVER's own sentence, rendered verbatim below. */}
      <p className="max-w-prose text-sm text-muted-foreground" data-panel-what="push">
        Enrolment is per DEVICE, not per person: it is this browser on this machine agreeing to receive a notification
        when a decision needs you. It is not a setting — nothing here changes the platform&apos;s configuration — and
        every device the platform has on file is listed below.
      </p>
      <Freshness stale={live.stale} error={live.error} hasData={live.data !== null} />
      {live.data && <DeviceControls view={live.data} onChanged={live.reload} env={env} />}
      {live.data && <EnrolledDevices view={live.data} />}
    </Panel>
  )
}

/**
 * The types that move this panel (§42). Both are the family's own lifecycle: a
 * device enrolling adds a row, one leaving takes it away — including when the
 * platform retires an endpoint a push service reported gone, which is exactly
 * the case a person would otherwise never learn about.
 */
export const pushEventTypes = ['push.subscribed', 'push.unsubscribed'] as const

type DeviceState = {
  subscribed: boolean
  knownToPlatform: boolean
}

function DeviceControls({
  view,
  onChanged,
  env: injected,
}: {
  view: PushSubscriptionsView
  onChanged: () => void
  env?: PushEnv
}) {
  const [env] = useState<PushEnv>(() => injected ?? readEnv())
  const [device, setDevice] = useState<DeviceState | null>(null)
  const [label, setLabel] = useState('')
  const [busy, setBusy] = useState(false)
  const [failure, setFailure] = useState('')

  const refresh = useCallback(() => {
    void thisDevice(env, view).then(setDevice, () => setDevice({ subscribed: false, knownToPlatform: false }))
  }, [env, view])

  // A READ, never a request for permission: this asks the browser what
  // subscription it already holds and nothing else.
  useEffect(refresh, [refresh])

  const can = availability(env)
  if (can.state === 'unsupported') {
    return (
      <p className="max-w-prose text-sm text-[var(--yellow)]" data-push-state="unsupported">
        {can.reason}
        {can.installHint && <span className="text-muted-foreground"> {can.installHint}</span>}
      </p>
    )
  }
  if (can.state === 'denied') {
    return (
      <p className="max-w-prose text-sm text-[var(--yellow)]" data-push-state="denied">
        {can.reason}
      </p>
    )
  }

  const act = (run: () => Promise<void>) => {
    setBusy(true)
    setFailure('')
    run().then(
      () => {
        setBusy(false)
        refresh()
        onChanged()
      },
      (err: unknown) => {
        setBusy(false)
        setFailure(err instanceof Error ? err.message : describeError(err))
        refresh()
      },
    )
  }

  const enrolled = device?.subscribed === true && device.knownToPlatform
  return (
    <div className="push-device" data-push-state={pushStateName(device)}>
      <p className="max-w-prose text-sm">{view.observable}</p>
      {device === null ? (
        <p className="text-sm text-muted-foreground">Asking this browser what it already holds…</p>
      ) : enrolled ? (
        <p className="text-sm text-[var(--green)]">
          This device is enrolled. Decisions that need you will arrive as a notification.
        </p>
      ) : device.subscribed ? (
        // The browser holds a subscription the platform does not list. Saying
        // "not enrolled" would be false and saying "enrolled" would be worse:
        // the honest answer is that they disagree, and re-enrolling fixes it.
        <p className="max-w-prose text-sm text-[var(--yellow)]" data-push-state="unknown-subscription">
          This browser already holds a push subscription that Sinet does not have on file — it may have been enrolled
          under another sign-in, or the platform’s record was removed. Enrolling again hands the same subscription back.
        </p>
      ) : (
        <p className="text-sm text-muted-foreground">This device is not enrolled. Nothing is sent to it.</p>
      )}

      <label>
        Name for this device
        <input
          type="text"
          value={label}
          placeholder="e.g. Alice’s phone"
          onChange={(e) => setLabel(e.target.value)}
          disabled={busy || enrolled}
        />
      </label>

      {!enrolled && (
        <Button
          variant="primary"
          disabled={busy || device === null}
          onClick={() => act(() => enrol(env, view.vapid_public_key, label))}
        >
          {device?.subscribed ? 'Enrol this device again' : 'Enrol this device'}
        </Button>
      )}
      {device?.subscribed === true && (
        <Button disabled={busy} onClick={() => act(() => unenrol(env))}>
          Stop notifications on this device
        </Button>
      )}
      {failure !== '' && (
        <p className="max-w-prose text-sm text-[var(--yellow)]" data-push-failure="true">
          Nothing changed: {failure}
        </p>
      )}
    </div>
  )
}

function pushStateName(device: DeviceState | null): string {
  if (device === null) return 'reading'
  if (device.subscribed && device.knownToPlatform) return 'enrolled'
  if (device.subscribed) return 'unknown-subscription'
  return 'not-enrolled'
}

/**
 * The enrolled-device list, as the server serves it.
 *
 * IT IS METADATA AND SAYS SO. There is no endpoint on the wire to render — an
 * endpoint is a capability — so a device is named by its label, its enrolling
 * origin and the hash the platform refers to it by. The scope sentence is the
 * server's own, so a member's narrower reading can never pose as the operator's
 * wider one.
 */
function EnrolledDevices({ view }: { view: PushSubscriptionsView }) {
  return (
    <div className="push-devices">
      <p className="max-w-prose text-sm text-muted-foreground" data-push-scope={view.scope}>
        {view.scope}
      </p>
      {view.subscriptions.length === 0 ? (
        <EmptyState
          what="No device is enrolled."
          why="A row appears here when a device is enrolled — this one, through the control above, or another one signed in as somebody this reading covers. Until then nothing is sent anywhere."
        />
      ) : (
        <ul className="items">
          {view.subscriptions.map((s) => (
            <li key={s.id} className="wrap-anywhere text-sm" data-subscription={s.endpoint_hash}>
              <strong>{s.label !== '' ? s.label : <Absent reason="no name given" />}</strong>{' '}
              <Owner id={s.owner} />
              <span className="font-mono text-xs tabular-nums text-muted-foreground"> {s.origin}</span>
              {s.device_login && (
                <span className="font-mono text-xs tabular-nums text-muted-foreground"> · seen as {s.device_login}</span>
              )}
              <span className="text-muted-foreground">
                {' '}
                · enrolled <Stamp ts={s.created_ts} />
              </span>
              <code className="text-xs text-muted-foreground"> {s.endpoint_hash.slice(0, 12)}…</code>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
