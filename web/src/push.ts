/**
 * The device half of S15.11 Web Push (B6-9).
 *
 * WHAT THIS FILE IS AND IS NOT. It decides what this device CAN do and drives
 * the browser's own subscribe/unsubscribe; it holds no policy, invents no
 * state, and stores nothing anywhere. The authority on which devices exist is
 * the server (`GET /api/push/subscriptions`), and nothing identity-shaped is
 * ever written to web storage (the landed scan binds).
 *
 * TWO PATHS, ONE SUBSCRIPTION. Declarative Web Push exposes `window.pushManager`
 * so a subscription can exist with no service worker at all — that is the Apple
 * path, and it is Apple-only (re-verified 2026-07-30: no Chromium feature entry,
 * both Firefox bugs still NEW). Everywhere else the subscription lives on a
 * service-worker registration, which is why /sw.js exists. WebKit's own note is
 * that a root-scoped worker SHARES the window subscription, so the two paths are
 * one subscription rather than two — the code prefers the declarative path where
 * it exists and falls back to the registration where it does not.
 *
 * EVERY UNAVAILABLE STATE RENDERS ITS OWN REASON. There is no "push is off"
 * catch-all: not signed in, no push API, permission refused and
 * not-installed-on-iOS are four different facts a person acts on differently,
 * and the panel says which one it is.
 */

import { api, type PushSubscriptionsView } from './api'

/** The environment this module reads. Injected so the whole state machine can
 *  be driven in tests without pretending jsdom is a browser with push. */
export type PushEnv = {
  /** The declarative (Safari) manager, when the browser has one. */
  windowPushManager?: PushManagerLike
  /** The classic path's registrar. */
  serviceWorker?: ServiceWorkerContainerLike
  /** Whether the browser exposes the Push API constructor at all. */
  hasPushManager: boolean
  /** Notification.permission, or '' when the API is absent. */
  permission: '' | 'default' | 'granted' | 'denied'
  requestPermission?: () => Promise<'default' | 'granted' | 'denied'>
  /** True when the page is running as an installed app (standalone display). */
  installed: boolean
  /** This page's own origin — what a deep link for this device must land on. */
  origin: string
}

export type PushSubscriptionLike = {
  endpoint: string
  toJSON(): { endpoint?: string; keys?: { p256dh?: string; auth?: string } }
  unsubscribe(): Promise<boolean>
}

export type PushManagerLike = {
  getSubscription(): Promise<PushSubscriptionLike | null>
  subscribe(options: { userVisibleOnly: boolean; applicationServerKey: Uint8Array }): Promise<PushSubscriptionLike>
}

export type ServiceWorkerContainerLike = {
  register(script: string, options?: { scope?: string }): Promise<{ pushManager: PushManagerLike }>
  ready: Promise<{ pushManager: PushManagerLike }>
}

/** The service worker's URL. Root-scoped: it must cover every deep link the
 *  notifier can send, and WebKit shares a root-scoped worker's subscription with
 *  the window's. It is a plain file in web/public, copied verbatim into the
 *  build — no plugin, no bundler entry, no dependency. */
export const swPath = '/sw.js'

/** The states the panel renders. Each is a different fact. */
export type PushAvailability =
  | { state: 'unsupported'; reason: string; installHint?: string }
  | { state: 'denied'; reason: string }
  | { state: 'available' }

/**
 * readEnv describes the real browser. It is the ONLY place globals are touched,
 * so everything below is drivable.
 */
export function readEnv(): PushEnv {
  const w = window as unknown as {
    pushManager?: PushManagerLike
    PushManager?: unknown
    Notification?: { permission: 'default' | 'granted' | 'denied'; requestPermission(): Promise<'default' | 'granted' | 'denied'> }
    matchMedia?: (q: string) => { matches: boolean }
    navigator: { serviceWorker?: ServiceWorkerContainerLike; standalone?: boolean }
  }
  const notification = w.Notification
  const standalone = w.matchMedia?.('(display-mode: standalone)').matches === true || w.navigator.standalone === true
  return {
    windowPushManager: w.pushManager,
    serviceWorker: w.navigator.serviceWorker,
    hasPushManager: typeof w.PushManager !== 'undefined',
    permission: notification ? notification.permission : '',
    requestPermission: notification ? () => notification.requestPermission() : undefined,
    installed: standalone,
    origin: window.location.origin,
  }
}

/**
 * availability answers what this device can do RIGHT NOW, with the true reason
 * when the answer is no.
 *
 * The iOS limb needs no user-agent sniffing, and deliberately: the signal that
 * matters is whether the page is running as an INSTALLED app, because Home
 * Screen installation is still a hard prerequisite for any web push on iOS as of
 * iOS 26 (re-verified 2026-07-30). A browser with no push API that is not
 * installed gets the instruction; one that IS installed gets the honest "this
 * browser does not offer it", because the instruction would be wrong there.
 */
export function availability(env: PushEnv): PushAvailability {
  const canSubscribe = Boolean(env.windowPushManager) || Boolean(env.serviceWorker && env.hasPushManager)
  if (!canSubscribe) {
    if (!env.installed) {
      return {
        state: 'unsupported',
        reason: 'This browser does not offer web push on this page.',
        installHint:
          'On iPhone and iPad it only works in an installed web app: open the Share menu, choose “Add to Home Screen”, ' +
          'and open Sinet from the icon it creates. iOS 16.4 or later is needed for web push at all, and 18.4 or later ' +
          'for the direct form Sinet sends.',
      }
    }
    return {
      state: 'unsupported',
      reason:
        'This installed app’s browser does not offer web push. On iPhone and iPad that needs iOS 16.4 or later — and ' +
        '18.4 or later for the direct form Sinet sends.',
    }
  }
  if (env.permission === 'denied') {
    return {
      state: 'denied',
      reason:
        'Notifications are blocked for this site, so the browser will not let Sinet ask again. Allow them in the ' +
        'browser’s own settings for this site, then reload.',
    }
  }
  return { state: 'available' }
}

/** managerFor resolves the subscribe surface: the declarative one where the
 *  browser has it, else the service worker's — registering it if needed. */
export async function managerFor(env: PushEnv): Promise<PushManagerLike | null> {
  if (env.windowPushManager) return env.windowPushManager
  if (!env.serviceWorker || !env.hasPushManager) return null
  const registration = await env.serviceWorker.register(swPath, { scope: '/' })
  return registration.pushManager
}

/**
 * enrol subscribes this device and hands the subscription to the platform.
 *
 * IT MUST BE CALLED FROM A USER GESTURE. Safari has required that of a
 * permission request since web push landed there, and asking outside one either
 * throws or is silently refused. Nothing in this module runs on mount — the
 * panel calls this from a click handler and from nowhere else.
 */
export async function enrol(env: PushEnv, vapidPublicKey: string, label: string): Promise<void> {
  const manager = await managerFor(env)
  if (!manager) throw new Error('this browser cannot hold a push subscription')
  if (env.permission !== 'granted') {
    if (!env.requestPermission) throw new Error('this browser has no Notification permission to ask for')
    const answer = await env.requestPermission()
    if (answer !== 'granted') throw new Error('notifications were not allowed, so nothing was enrolled')
  }
  const existing = await manager.getSubscription()
  // `userVisibleOnly: true` is mandatory rather than polite: Chrome and Edge
  // reject a subscribe without it, and Safari revokes permission for a site that
  // pushes invisibly.
  const subscription =
    existing ??
    (await manager.subscribe({ userVisibleOnly: true, applicationServerKey: decodeVAPIDKey(vapidPublicKey) }))
  const body = enrolmentBody(subscription, env.origin, label)
  if (!body) throw new Error('the browser returned a subscription with no key material')
  await api.enrolPush(body)
}

/** unenrol drops the subscription here AND on the platform. The server call
 *  goes first: a device the platform still lists but the browser has forgotten
 *  is a row that can never be cleaned up from this page again. */
export async function unenrol(env: PushEnv): Promise<void> {
  const manager = await managerFor(env)
  if (!manager) return
  const subscription = await manager.getSubscription()
  if (!subscription) return
  await api.removePush(subscription.endpoint)
  await subscription.unsubscribe()
}

/** thisDevice reports whether the browser currently holds a subscription, and
 *  whether the PLATFORM knows about that exact one. The two can disagree — a
 *  browser rotation, another person's session — and the panel says so rather
 *  than picking one to believe. */
export async function thisDevice(
  env: PushEnv,
  view: PushSubscriptionsView | null,
): Promise<{ subscribed: boolean; knownToPlatform: boolean }> {
  const manager = await managerFor(env).catch(() => null)
  const subscription = manager ? await manager.getSubscription().catch(() => null) : null
  if (!subscription) return { subscribed: false, knownToPlatform: false }
  const hash = await sha256Hex(subscription.endpoint)
  const known = (view?.subscriptions ?? []).some((s) => s.endpoint_hash === hash)
  return { subscribed: true, knownToPlatform: known }
}

/** enrolmentBody is the shape POST /api/push/subscriptions takes — the same one
 *  the service worker composes on a `pushsubscriptionchange`. */
export function enrolmentBody(
  subscription: PushSubscriptionLike,
  origin: string,
  label: string,
): { endpoint: string; keys: { p256dh: string; auth: string }; origin: string; label: string } | null {
  const raw = subscription.toJSON()
  const keys = raw.keys ?? {}
  if (!raw.endpoint || !keys.p256dh || !keys.auth) return null
  return { endpoint: raw.endpoint, keys: { p256dh: keys.p256dh, auth: keys.auth }, origin, label }
}

/** decodeVAPIDKey turns the SERVED base64url application server key into the
 *  BufferSource `subscribe` takes. The key is served rather than compiled in, so
 *  a rotated key needs no new build. */
export function decodeVAPIDKey(base64url: string): Uint8Array {
  const padded = base64url.replace(/-/g, '+').replace(/_/g, '/')
  const binary = atob(padded + '='.repeat((4 - (padded.length % 4)) % 4))
  const out = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i)
  return out
}

/** sha256Hex matches the server's own endpoint reference, so this page can ask
 *  "is THIS device one of the rows you listed" without ever being served an
 *  endpoint back. */
export async function sha256Hex(value: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value))
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}
