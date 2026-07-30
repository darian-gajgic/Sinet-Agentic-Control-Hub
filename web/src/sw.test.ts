import { beforeEach, afterEach, describe, expect, test, vi } from 'vitest'

/**
 * The push-only service worker, driven as the SHIPPED FILE (B6-9 OQ6/rubric 12).
 *
 * `web/public/sw.js` is copied verbatim into the build — no bundler entry, no
 * plugin, no dependency — so it is not part of the typed `src` tree and nothing
 * else imports it. That makes it exactly the kind of file that rots unwatched,
 * which is why these tests load THAT file rather than a copy of its logic: the
 * worker registers its handlers on `self` at import time, and jsdom's `self` is
 * the window, so the handlers can be captured and driven.
 *
 * Nothing here opens a connection: the one fetch the worker makes is scripted.
 */

// import.meta.glob rather than a bare import: the specifier is not a typed
// module, and this is Vite's own way to reach a file the type tree does not
// cover without a bundler escape hatch.
const swModules = import.meta.glob('../public/sw.js')

type Handler = (event: unknown) => void

type ServiceWorkerWorld = {
  handlers: Map<string, Handler>
  shown: { title: string; options: Record<string, unknown> }[]
  badges: (number | 'cleared')[]
  focused: string[]
  opened: string[]
  navigated: string[]
  subscribeCalls: { userVisibleOnly: boolean; applicationServerKey: unknown }[]
}

let world: ServiceWorkerWorld

async function loadWorker(over: { clients?: unknown; registration?: unknown } = {}): Promise<void> {
  world = {
    handlers: new Map(),
    shown: [],
    badges: [],
    focused: [],
    opened: [],
    navigated: [],
    subscribeCalls: [],
  }
  const registration = over.registration ?? {
    showNotification: (title: string, options: Record<string, unknown>) => {
      world.shown.push({ title, options })
      return Promise.resolve()
    },
    pushManager: {
      subscribe: (options: { userVisibleOnly: boolean; applicationServerKey: unknown }) => {
        world.subscribeCalls.push(options)
        return Promise.resolve({
          endpoint: 'https://push.example/rotated',
          toJSON: () => ({ endpoint: 'https://push.example/rotated', keys: { p256dh: 'NEWPUB', auth: 'NEWAUTH' } }),
        })
      },
    },
  }
  const clients = over.clients ?? {
    claim: () => Promise.resolve(),
    matchAll: () => Promise.resolve([]),
    openWindow: (url: string) => {
      world.opened.push(url)
      return Promise.resolve(null)
    },
  }
  vi.stubGlobal('registration', registration)
  vi.stubGlobal('clients', clients)
  vi.stubGlobal('skipWaiting', () => {})
  vi.spyOn(window, 'addEventListener').mockImplementation(((type: string, handler: Handler) => {
    world.handlers.set(type, handler)
  }) as typeof window.addEventListener)
  vi.resetModules()
  await swModules['../public/sw.js']()
}

/** fire drives one worker event and settles whatever it passed to waitUntil. */
async function fire(type: string, event: Record<string, unknown>): Promise<void> {
  const handler = world.handlers.get(type)
  if (!handler) throw new Error(`the worker registered no ${type} handler`)
  const pending: Promise<unknown>[] = []
  handler({ ...event, waitUntil: (p: Promise<unknown>) => pending.push(p) })
  await Promise.all(pending)
}

function declarative(over: Record<string, unknown> = {}): { json: () => unknown } {
  return {
    json: () => ({
      web_push: 8030,
      notification: {
        title: 'Sinet — a decision is waiting',
        navigate: 'https://sinet.example.ts.net/inbox/ask%3Aask-1',
        icon: 'https://sinet.example.ts.net/icon-192.png',
        tag: 'ask:ask-1',
        app_badge: '3',
        ...over,
      },
    }),
  }
}

describe('the push-only service worker', () => {
  beforeEach(async () => {
    await loadWorker()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  test('registers exactly the push handlers and NO fetch handler', () => {
    expect([...world.handlers.keys()].sort()).toEqual(
      ['activate', 'install', 'notificationclick', 'push', 'pushsubscriptionchange'].sort(),
    )
    // The load-bearing negative: fetch interception is where every hazard the
    // no-service-worker posture excluded lives — a cached shell posing as live
    // data, a deploy that does not take effect. This worker intercepts nothing.
    expect(world.handlers.has('fetch')).toBe(false)
  })

  test('renders the SAME declarative payload the platform sends to Safari', async () => {
    await fire('push', { data: declarative() })
    expect(world.shown).toHaveLength(1)
    const [n] = world.shown
    expect(n.title).toBe('Sinet — a decision is waiting')
    expect(n.options.icon).toBe('https://sinet.example.ts.net/icon-192.png')
    expect(n.options.tag).toBe('ask:ask-1')
    // The deep link travels in `data` because notificationclick is where it is
    // used; the declarative renderer reads the same value off `navigate`.
    expect(n.options.data).toEqual({ navigate: 'https://sinet.example.ts.net/inbox/ask%3Aask-1' })
  })

  test('applies the badge through the standard Badging API where it exists', async () => {
    const set = vi.fn(() => Promise.resolve())
    const clear = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'setAppBadge', { value: set, configurable: true })
    Object.defineProperty(navigator, 'clearAppBadge', { value: clear, configurable: true })

    await fire('push', { data: declarative() })
    expect(set).toHaveBeenCalledWith(3)

    // A zero CLEARS rather than setting a badge of nothing; a value that is not
    // a count clears too, rather than guessing one.
    await fire('push', { data: declarative({ app_badge: '0' }) })
    await fire('push', { data: declarative({ app_badge: 'lots' }) })
    expect(clear).toHaveBeenCalledTimes(2)
  })

  test('ALWAYS shows a notification, even for a payload it cannot read', async () => {
    // Safari revokes push permission for a site that receives a push and shows
    // nothing, so an unreadable payload must degrade to an honest notification
    // rather than to silence.
    await fire('push', { data: { json: () => ({ something: 'else' }) } })
    await fire('push', {
      data: {
        json: () => {
          throw new Error('not JSON')
        },
      },
    })
    await fire('push', {})
    expect(world.shown).toHaveLength(3)
    for (const n of world.shown) {
      expect(n.title).toBe('Sinet')
      expect(n.options.data).toEqual({ navigate: '/inbox' })
    }
  })

  test('a tap FOCUSES an open window and navigates it rather than opening a second', async () => {
    const client = {
      url: window.location.origin + '/board',
      focus: () => {
        world.focused.push('focused')
        return Promise.resolve()
      },
      navigate: (url: string) => {
        world.navigated.push(url)
        return Promise.resolve(null)
      },
    }
    await loadWorker({
      clients: {
        claim: () => Promise.resolve(),
        matchAll: () => Promise.resolve([client]),
        openWindow: (url: string) => {
          world.opened.push(url)
          return Promise.resolve(null)
        },
      },
    })
    const closed: string[] = []
    await fire('notificationclick', {
      notification: { close: () => closed.push('closed'), data: { navigate: window.location.origin + '/inbox/ask%3Aa' } },
    })
    expect(closed).toEqual(['closed'])
    expect(world.focused).toEqual(['focused'])
    expect(world.navigated).toEqual([window.location.origin + '/inbox/ask%3Aa'])
    expect(world.opened).toEqual([])
  })

  test('with no window open it opens one, and a relative navigate resolves on this origin', async () => {
    await fire('notificationclick', {
      notification: { close: () => {}, data: { navigate: window.location.origin + '/inbox/ask%3Aa' } },
    })
    // The fallback path the unreadable-payload case produces is relative, and it
    // has to resolve here rather than be refused as cross-origin.
    await fire('notificationclick', { notification: { close: () => {}, data: { navigate: '/inbox' } } })
    expect(world.opened).toEqual([window.location.origin + '/inbox/ask%3Aa', window.location.origin + '/inbox'])
  })

  test('a notification naming ANOTHER origin navigates nowhere', async () => {
    // A push service that could steer a person to any URL would be a real
    // capability handed to a third party.
    await fire('notificationclick', {
      notification: { close: () => {}, data: { navigate: 'https://evil.example/steal' } },
    })
    expect(world.opened).toEqual([])
    expect(world.navigated).toEqual([])
  })

  test('a rotated subscription is handed straight back to the enrolment verb', async () => {
    const calls: { path: string; body: unknown }[] = []
    vi.stubGlobal('fetch', async (input: string, init: RequestInit) => {
      calls.push({ path: String(input), body: JSON.parse(String(init.body)) })
      return { ok: true, status: 200, json: async () => ({}) } as Response
    })
    await fire('pushsubscriptionchange', {
      newSubscription: {
        endpoint: 'https://push.example/rotated',
        toJSON: () => ({ endpoint: 'https://push.example/rotated', keys: { p256dh: 'NEWPUB', auth: 'NEWAUTH' } }),
      },
    })
    expect(calls).toHaveLength(1)
    expect(calls[0].path).toBe('/api/push/subscriptions')
    expect(calls[0].body).toMatchObject({
      endpoint: 'https://push.example/rotated',
      keys: { p256dh: 'NEWPUB', auth: 'NEWAUTH' },
      origin: window.location.origin,
    })
  })

  test('and where only the OLD subscription is reported, it re-subscribes with the same key first', async () => {
    const calls: unknown[] = []
    vi.stubGlobal('fetch', async (_input: string, init: RequestInit) => {
      calls.push(JSON.parse(String(init.body)))
      return { ok: true, status: 200, json: async () => ({}) } as Response
    })
    await fire('pushsubscriptionchange', {
      oldSubscription: { options: { applicationServerKey: 'THEKEY' } },
    })
    expect(world.subscribeCalls).toEqual([{ userVisibleOnly: true, applicationServerKey: 'THEKEY' }])
    expect(calls).toHaveLength(1)
  })

  test('with neither subscription it posts nothing rather than guessing', async () => {
    const calls: unknown[] = []
    vi.stubGlobal('fetch', async () => {
      calls.push(1)
      return { ok: true, status: 200, json: async () => ({}) } as Response
    })
    await fire('pushsubscriptionchange', {})
    expect(calls).toHaveLength(0)
  })
})
