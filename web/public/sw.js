/**
 * The push-only service worker (Spec S15.11; P3 B6-9 OQ6).
 *
 * WHY THERE IS A SERVICE WORKER AT ALL, after B6-4 shipped the SPA without one.
 * Declarative Web Push is shipped on APPLE PLATFORMS ONLY — iOS/iPadOS 18.4 and
 * macOS Safari 18.5, re-verified 2026-07-30: Chromium has no such feature entry
 * and both Firefox bugs are still NEW. S15.11's own letter says "Android is
 * standard push", and standard push STRUCTURALLY requires a service-worker
 * receiver: outside Safari there is no way to hold a subscription without one.
 * So the alternative to this file is not "no service worker" — it is "no push on
 * any non-Apple device in the household", which fails the spec's own Android
 * limb.
 *
 * IT IS PUSH-ONLY, AND THAT IS WHAT MAKES IT SAFE TO ADMIT. There is NO `fetch`
 * handler here and none may be added: every hazard the B6-4 no-service-worker
 * posture excluded — stale assets served from a cache, an offline shell posing
 * as live data, a deploy that does not take effect on reload — belongs to fetch
 * interception, and this file does not intercept anything. A cold launch while
 * the host is asleep therefore renders the BROWSER'S OWN error page, which is
 * the honest unreachable state S15.12 asks for; a fake offline shell would be
 * the stale-poses-as-live failure it forbids. That posture is what the week-one
 * drill observes rather than something this file asserts.
 *
 * ONE COMPOSER, TWO RENDERERS. The payload below is the same declarative
 * message the platform sends to Safari, which renders it with no JavaScript at
 * all. The format was designed for exactly this: a declarative payload arriving
 * at an older browser "is handled imperatively by JavaScript as it always had
 * been" (webkit.org/blog/16535). So the server never branches on the recipient
 * — it could not, since a subscription says nothing about what will render it —
 * and this file is the imperative renderer of the identical JSON.
 *
 * NOTHING HERE REACHES A THIRD PARTY. The only request this file makes is the
 * re-enrolment POST to this origin's own /api/push/subscriptions.
 */

/** The one thing this worker renders. Keep in step with internal/push/send.go. */
const WEB_PUSH_DISAMBIGUATOR = 8030

/** The enrolment verb, on this origin. */
const ENROL_PATH = '/api/push/subscriptions'

/**
 * A new worker takes over immediately rather than waiting for every tab to
 * close. There is no fetch handler, so there is no half-updated page to
 * protect: the only thing skipping the wait changes is how soon a notification
 * is rendered by the current code.
 */
self.addEventListener('install', () => {
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

/**
 * Render one push.
 *
 * A notification is ALWAYS shown. Safari revokes push permission for a site
 * that receives a push and displays nothing, and Chrome/Edge require
 * `userVisibleOnly: true` at subscribe time — so an invisible push is not an
 * option this platform has, and the declarative form makes it structurally
 * impossible on the Apple leg anyway.
 */
self.addEventListener('push', (event) => {
  event.waitUntil(render(event.data))
})

async function render(data) {
  const message = readMessage(data)
  const options = {
    body: message.body,
    icon: message.icon,
    tag: message.tag,
    // The deep link travels in `data` because notificationclick is where it is
    // used; the declarative renderer reads the same value off `navigate`.
    data: { navigate: message.navigate },
    // A decision is worth an alert even when one for the same card is already
    // on screen: `tag` replaces the old notification and this makes the
    // replacement announce itself, which is what a re-nag is for.
    renotify: Boolean(message.tag),
  }
  await self.registration.showNotification(message.title, options)
  await setBadge(message.badge)
}

/**
 * Parse the declarative message.
 *
 * Anything that is not one is rendered as a bare notification rather than
 * dropped: a push that arrives and shows nothing costs the site its permission,
 * so an unreadable payload degrades to an honest "something needs you" instead
 * of to silence.
 */
function readMessage(data) {
  const fallback = { title: 'Sinet', navigate: '/inbox' }
  if (!data) return fallback
  let parsed
  try {
    parsed = data.json()
  } catch {
    return fallback
  }
  if (!parsed || parsed.web_push !== WEB_PUSH_DISAMBIGUATOR) return fallback
  const n = parsed.notification
  if (!n || typeof n.title !== 'string' || typeof n.navigate !== 'string') return fallback
  return {
    title: n.title,
    navigate: n.navigate,
    body: typeof n.body === 'string' ? n.body : undefined,
    icon: typeof n.icon === 'string' ? n.icon : undefined,
    tag: typeof n.tag === 'string' ? n.tag : undefined,
    badge: n.app_badge,
  }
}

/**
 * `app_badge` is a WebKit extension the declarative renderer applies on its own;
 * on the classic leg this is the equivalent, through the standard Badging API
 * where the browser has it. A value that is not a count clears the badge rather
 * than guessing one.
 */
async function setBadge(value) {
  if (typeof navigator.setAppBadge !== 'function') return
  const n = Number(value)
  try {
    if (Number.isFinite(n) && n > 0) await navigator.setAppBadge(n)
    else if (typeof navigator.clearAppBadge === 'function') await navigator.clearAppBadge()
  } catch {
    // Badging is user-configurable in the OS. A refusal is not an error worth
    // failing a notification over.
  }
}

/**
 * A tap opens the decision.
 *
 * An already-open Sinet window is FOCUSED and navigated rather than duplicated:
 * a person who taps a notification wants the card, not a second copy of the app.
 */
self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const target = (event.notification.data && event.notification.data.navigate) || '/inbox'
  event.waitUntil(openDecision(target))
})

async function openDecision(target) {
  const url = new URL(target, self.location.origin)
  // A notification composed by this platform always points at this origin. A
  // payload naming another one is not followed — that would make a push service
  // able to navigate a person anywhere.
  if (url.origin !== self.location.origin) return
  const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
  for (const client of windows) {
    if (new URL(client.url).origin !== self.location.origin) continue
    await client.focus()
    if ('navigate' in client) await client.navigate(url.href)
    return
  }
  await self.clients.openWindow(url.href)
}

/**
 * The subscription was rotated by the browser.
 *
 * `pushsubscriptionchange` fires when a user agent replaces a subscription on
 * its own. Without this the device silently stops receiving decisions: the
 * platform still holds the old endpoint, every send answers 410, and the row is
 * retired — correct behaviour with nothing to replace it. So the new
 * subscription is handed straight to the same enrolment verb the page uses, and
 * the enrolment is keyed on the endpoint, which makes this retry-safe.
 */
self.addEventListener('pushsubscriptionchange', (event) => {
  event.waitUntil(reEnrol(event))
})

async function reEnrol(event) {
  let subscription = event.newSubscription
  if (!subscription) {
    // Some browsers report only the OLD subscription and expect the worker to
    // re-subscribe with the same application server key.
    const old = event.oldSubscription
    const key = old && old.options && old.options.applicationServerKey
    if (!key) return
    subscription = await self.registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: key,
    })
  }
  const body = enrolBody(subscription)
  if (!body) return
  await fetch(ENROL_PATH, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

/**
 * The enrolment body, in the shape /api/push/subscriptions takes. It is the
 * SAME shape the page composes; the origin is this worker's own scope, which is
 * the origin a deep link for this device has to land on.
 */
function enrolBody(subscription) {
  if (!subscription) return null
  const raw = typeof subscription.toJSON === 'function' ? subscription.toJSON() : subscription
  const keys = raw.keys || {}
  if (!raw.endpoint || !keys.p256dh || !keys.auth) return null
  return {
    endpoint: raw.endpoint,
    keys: { p256dh: keys.p256dh, auth: keys.auth },
    origin: self.location.origin,
    label: '',
  }
}
