import { afterEach, expect, test } from 'vitest'

import viteConfig from '../vite.config'

/**
 * The dev server must not disagree with the binary it stands in for.
 *
 * In production `isAPIPath` (internal/api/spa.go) splits the machine surface
 * from the app on a path BOUNDARY: `/api` and `/api/…` are the API, `/apifoo`
 * is an app route. A bare `'/api'` proxy key in Vite is a PREFIX match, so it
 * would proxy `/apifoo` and a dev session would behave differently from the
 * shipped one — the kind of divergence that is only ever found the hard way.
 */

/** The dev-server block only materialises for a real `vite` dev run: it is
 *  suppressed under vitest so a test run never resolves a proxy target. This
 *  reads it the way the dev server would, without starting one. */
function devServerConfig() {
  const savedVitest = process.env.VITE_SINET_ADDR
  const savedMarker = process.env.VITEST
  delete process.env.VITEST
  process.env.VITE_SINET_ADDR = 'http://127.0.0.1:65530'
  try {
    const cfg = viteConfig({ command: 'serve', mode: 'development' })
    return cfg as { server?: { host?: string | boolean; proxy?: Record<string, unknown> } }
  } finally {
    if (savedMarker === undefined) delete process.env.VITEST
    else process.env.VITEST = savedMarker
    if (savedVitest === undefined) delete process.env.VITE_SINET_ADDR
    else process.env.VITE_SINET_ADDR = savedVitest
  }
}

afterEach(() => {
  // The marker must survive: without it the config would demand a proxy target
  // during ordinary test runs.
  expect(process.env.VITEST).toBeDefined()
})

test('the dev server proxies exactly the two machine surfaces, and binds loopback', () => {
  const server = devServerConfig().server
  expect(server?.host).toBe('127.0.0.1')
  expect(Object.keys(server?.proxy ?? {})).toEqual(['^/api(/|$)', '^/events(/|$)'])
})

test('the proxy matches on a path boundary, exactly like the production wall', () => {
  const keys = Object.keys(devServerConfig().server?.proxy ?? {})
  // A `^` key is a RegExp to Vite; anything else is a prefix match.
  for (const key of keys) expect(key.startsWith('^')).toBe(true)
  const matches = (path: string) => keys.some((k) => new RegExp(k).test(path))

  for (const machine of ['/api', '/api/health', '/api/auth/session', '/events', '/events/anything']) {
    expect(matches(machine), `${machine} must reach the control plane`).toBe(true)
  }
  for (const app of ['/apifoo', '/eventsfoo', '/api-docs', '/', '/inbox/ask-7', '/board']) {
    expect(matches(app), `${app} is an app route and must not be proxied`).toBe(false)
  }
})

test('a test run never resolves a proxy target', () => {
  // VITEST is set here, so the dev block is absent and no VITE_SINET_ADDR is
  // needed — which is what keeps `npm run test` and `npm run build` hermetic.
  const cfg = viteConfig({ command: 'serve', mode: 'test' }) as { server?: unknown }
  expect(cfg.server).toBeUndefined()
})
