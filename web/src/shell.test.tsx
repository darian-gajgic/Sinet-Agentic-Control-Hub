import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { ConnectionState } from './ConnectionState'
import { EventStream, type EventSourceLike, type Status } from './events'
import { routes } from './routes'
import { flush, mount } from './testing'

/** A stream that never opens anything — the shell's connection machinery is
 *  exercised elsewhere; here it must simply not dial. */
const inertSource: EventSourceLike = { addEventListener: () => {}, close: () => {}, readyState: 0 }
const inertStream = () =>
  new EventStream({
    createEventSource: () => inertSource,
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

type Route = { body?: unknown; status?: number }

/** routeFetch is the scripted control plane: a map of "METHOD path" to a
 *  response. Anything not scripted fails the test loudly rather than silently
 *  returning undefined. */
function routeFetch(table: Record<string, Route>) {
  const calls: { method: string; path: string; body: unknown }[] = []
  const impl = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input)
    const method = init?.method ?? 'GET'
    const key = `${method} ${path}`
    calls.push({ method, path, body: init?.body ? JSON.parse(String(init.body)) : undefined })
    const route = table[key]
    if (!route) throw new Error(`unscripted request: ${key}`)
    const status = route.status ?? 200
    return {
      ok: status >= 200 && status < 300,
      status,
      statusText: '',
      json: async () => route.body ?? {},
    } as Response
  })
  vi.stubGlobal('fetch', impl)
  return calls
}

const anonymous: Route = { body: { authenticated: false } }
const signedIn: Route = {
  body: { authenticated: true, user: { user_id: 'alice', display_name: 'Alice', role: 'operator', pin_set: true } },
}

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  localStorage.clear()
})

afterEach(() => {
  vi.unstubAllGlobals()
  // Each mount removes its own container on unmount; anything a failed test
  // left behind goes here. (Assigning markup would trip the escape scan, and
  // rightly so — nothing in web/src writes raw HTML.)
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── routes and stubs (S15.11) ─────────────────────────────────────────────

test('every unbuilt surface has a reachable stub naming its owning packet', async () => {
  const table: Record<string, Route> = { 'GET /api/auth/session': signedIn }
  routeFetch(table)

  let stubs = 0
  for (const r of routes) {
    // A BUILT surface renders its data, not a stub. Asserting `toContain('')`
    // over those would be vacuous — the built ones have their own suites
    // (mission.test.tsx, board.test.tsx), and are exercised there.
    if (r.id === 'login' || r.owner === '') continue
    const path = r.pattern.replace(/:(\w+)/g, 'x-1')
    window.history.replaceState(null, '', path)
    const view = mount(<App stream={inertStream()} />)
    await flush()

    const text = view.container.textContent ?? ''
    expect(text, `${path} did not render its surface title`).toContain(r.title)
    expect(text, `${path} does not name its owning packet`).toContain(r.owner)
    stubs++
    view.unmount()
  }
  expect(stubs, 'no stub was checked — the loop would pass vacuously').toBeGreaterThan(0)
})

test('a built surface renders its own heading, not a stub', async () => {
  // The scripted plane answers only the session; every data read fails, which
  // is the point: even unreadable, a built surface says what it IS rather than
  // claiming to be unbuilt.
  routeFetch({ 'GET /api/auth/session': signedIn })
  for (const id of ['mission-control', 'board'] as const) {
    const r = routes.find((x) => x.id === id)!
    window.history.replaceState(null, '', r.pattern)
    const view = mount(<App stream={inertStream()} />)
    await flush()

    const text = view.container.textContent ?? ''
    expect(text).toContain(r.title)
    expect(text, `${r.pattern} still renders the not-built-yet stub`).not.toContain('Not built yet')
    view.unmount()
  }
})

test('a deep link carries its identity into the surface that owns it', async () => {
  // /inbox/:id is built now, so the identity reaches the real card surface
  // rather than a stub that echoed it. A card id that is not in the caller's
  // own queue renders the honest not-visible state and names the id it looked
  // for — the same URL contract, answered by the surface instead of a stub.
  routeFetch({
    'GET /api/auth/session': signedIn,
    'GET /api/approvals': { body: { items: [], cursor: 1, truncated: false } },
  })
  window.history.replaceState(null, '', '/inbox/ask-7')

  const view = mount(<App stream={inertStream()} />)
  await flush()

  expect(view.container.textContent).toContain('Approval')
  expect(view.container.textContent).toContain('ask-7')
  view.unmount()
})

test('the nav reaches every navigable surface, and only real paths', async () => {
  routeFetch({ 'GET /api/auth/session': signedIn })

  const view = mount(<App stream={inertStream()} />)
  await flush()

  // Scoped to the SHELL nav: mission control now carries a second <nav> for
  // the personal filters, and this assertion is about the primary one.
  const hrefs = [...view.container.querySelectorAll('.shell-nav a')].map((a) => a.getAttribute('href'))
  expect(hrefs).toEqual(routes.filter((r) => r.nav).map((r) => r.pattern))
  view.unmount()
})

// ── session (Spec S01.9) ──────────────────────────────────────────────────

test('no session anywhere redirects to login and keeps a return-to', async () => {
  routeFetch({ 'GET /api/auth/session': anonymous, 'GET /api/auth/users': { body: { users: [] } } })
  window.history.replaceState(null, '', '/inbox/ask-7')

  const view = mount(<App stream={inertStream()} />)
  await flush()

  expect(window.location.pathname).toBe('/login')
  expect(new URLSearchParams(window.location.search).get('next')).toBe('/inbox/ask-7')
  view.unmount()
})

test('the login picker comes from the users route and the device hint prefills it', async () => {
  routeFetch({
    'GET /api/auth/session': {
      body: {
        authenticated: false,
        hint: { device_login: 'alice@example.ts.net', user_id: 'bob', auto_login: false },
      },
    },
    'GET /api/auth/users': {
      body: {
        users: [
          { user_id: 'alice', display_name: 'Alice', role: 'operator', pin_set: true },
          { user_id: 'bob', display_name: 'Bob', role: 'member', pin_set: true },
        ],
      },
    },
  })
  window.history.replaceState(null, '', '/login')

  const view = mount(<App stream={inertStream()} />)
  await flush()

  const select = view.container.querySelector('select')!
  expect([...select.options].map((o) => o.value)).toEqual(['alice', 'bob'])
  expect(select.value, 'the device hint did not prefill the picker').toBe('bob')
  expect(view.container.textContent).toContain('alice@example.ts.net')
  view.unmount()
})

test('a PIN posts to the login route, is typed as a password, and is not kept', async () => {
  const calls = routeFetch({
    'GET /api/auth/session': anonymous,
    'GET /api/auth/users': { body: { users: [{ user_id: 'alice', display_name: 'Alice', role: 'operator', pin_set: true }] } },
    'POST /api/auth/login': { body: { user_id: 'alice', expires: '2026-08-28T00:00:00Z' } },
  })
  window.history.replaceState(null, '', '/login')

  const view = mount(<App stream={inertStream()} />)
  await flush()

  const pin = view.container.querySelector('input[type=password]') as HTMLInputElement
  expect(pin, 'the PIN field is not a password field').not.toBeNull()
  setValue(pin, '4321')
  submit(view.container.querySelector('form')!)
  await flush()

  const login = calls.find((c) => c.path === '/api/auth/login')
  expect(login?.body).toEqual({ user_id: 'alice', pin: '4321' })
  // The secret does not outlive the request that spent it.
  expect((view.container.querySelector('input[type=password]') as HTMLInputElement | null)?.value ?? '').toBe('')
  view.unmount()
})

// The first real-host click-through. Creating the operator answers 201 with no
// session cookie and CLOSES the bootstrap window, so the flow has to land
// somewhere that still works — re-rendering the create form would dead-end the
// operator on a second submit that fails with a misleading 401.
test('an empty user list opens the bootstrap window and lands on the picker after creating', async () => {
  const impl: Record<string, Route> = {
    'GET /api/auth/session': anonymous,
    'GET /api/auth/users': { body: { users: [] } },
    'POST /api/auth/users': { status: 201, body: { user_id: 'alice' } },
  }
  const calls = routeFetch(impl)
  window.history.replaceState(null, '', '/login')

  const view = mount(<App stream={inertStream()} />)
  await flush()

  expect(view.container.textContent).toContain('First run')
  const inputs = view.container.querySelectorAll('input')
  setValue(inputs[0] as HTMLInputElement, 'alice')
  setValue(inputs[1] as HTMLInputElement, 'Alice')
  setValue(inputs[2] as HTMLInputElement, '1234')

  // The account exists from the moment the POST lands: the re-read shows it.
  impl['GET /api/auth/users'] = {
    body: { users: [{ user_id: 'alice', display_name: 'Alice', role: 'operator', pin_set: true }] },
  }
  submit(view.container.querySelector('form')!)
  await flush()

  const create = calls.find((c) => c.path === '/api/auth/users' && c.method === 'POST')
  expect(create?.body).toEqual({ user_id: 'alice', display_name: 'Alice', role: 'operator', pin: '1234' })

  // The post-create state, which is the point: the picker, carrying the new
  // account, with the closed bootstrap form gone and the next step named.
  const text = view.container.textContent ?? ''
  expect(text, 'the closed bootstrap window rendered again').not.toContain('First run')
  expect(text).toContain('Operator alice created')
  const select = view.container.querySelector('select')!
  expect([...select.options].map((o) => o.value)).toEqual(['alice'])
  expect(select.value).toBe('alice')
  // And the PIN field is empty: creating an account is not signing in.
  expect((view.container.querySelector('input[type=password]') as HTMLInputElement).value).toBe('')
  view.unmount()
})

// The other half of the same fix: with the account in place, the sign-in that
// follows actually works from the state the create left behind.
test('the account created in the bootstrap window can sign in from where it lands', async () => {
  const impl: Record<string, Route> = {
    'GET /api/auth/session': anonymous,
    'GET /api/auth/users': { body: { users: [] } },
    'POST /api/auth/users': { status: 201, body: { user_id: 'alice' } },
    'POST /api/auth/login': { body: { user_id: 'alice', expires: '2026-08-28T00:00:00Z' } },
  }
  const calls = routeFetch(impl)
  window.history.replaceState(null, '', '/login')

  const view = mount(<App stream={inertStream()} />)
  await flush()
  const inputs = view.container.querySelectorAll('input')
  setValue(inputs[0] as HTMLInputElement, 'alice')
  setValue(inputs[1] as HTMLInputElement, 'Alice')
  setValue(inputs[2] as HTMLInputElement, '1234')
  impl['GET /api/auth/users'] = {
    body: { users: [{ user_id: 'alice', display_name: 'Alice', role: 'operator', pin_set: true }] },
  }
  submit(view.container.querySelector('form')!)
  await flush()

  setValue(view.container.querySelector('input[type=password]') as HTMLInputElement, '1234')
  submit(view.container.querySelector('form')!)
  await flush()

  expect(calls.find((c) => c.path === '/api/auth/login')?.body).toEqual({ user_id: 'alice', pin: '1234' })
  view.unmount()
})

test('signing out posts to the logout route and re-reads identity', async () => {
  const calls = routeFetch({
    'GET /api/auth/session': signedIn,
    'POST /api/auth/logout': { status: 204 },
  })

  const view = mount(<App stream={inertStream()} />)
  await flush()

  const button = [...view.container.querySelectorAll('button')].find((b) => b.textContent === 'Sign out')!
  click(button)
  await flush()

  expect(calls.some((c) => c.path === '/api/auth/logout' && c.method === 'POST')).toBe(true)
  view.unmount()
})

// The cookie is HttpOnly and identity comes from the session route alone; the
// client keeps no credential of its own anywhere.
test('the client stores no identity and reads no cookie', async () => {
  routeFetch({ 'GET /api/auth/session': signedIn })

  const view = mount(<App stream={inertStream()} />)
  await flush()

  expect(localStorage.length).toBe(0)
  expect(sessionStorage.length).toBe(0)
  view.unmount()
})

// ── connection state (S15.12) ─────────────────────────────────────────────

test('the indicator renders every state and never poses as live', () => {
  const states: Status[] = ['connecting', 'catching-up', 'live', 'disconnected', 'logged-out', 'unreachable']
  const seen = new Set<string>()

  for (const status of states) {
    const view = mount(<ConnectionState status={status} />)
    const node = view.container.querySelector('.conn')!
    expect(node.getAttribute('data-status')).toBe(status)
    const label = node.querySelector('.conn-label')!.textContent ?? ''
    expect(label.length).toBeGreaterThan(0)
    if (status !== 'live') {
      expect(label.toLowerCase(), `${status} reads as live`).not.toContain('live')
    }
    seen.add(label)
    view.unmount()
  }
  expect(seen.size, 'two states render the same label').toBe(states.length)
})

test('the indicator is present on every route, signed in or not', async () => {
  routeFetch({ 'GET /api/auth/session': anonymous, 'GET /api/auth/users': { body: { users: [] } } })
  window.history.replaceState(null, '', '/login')

  const login = mount(<App stream={inertStream()} />)
  await flush()
  expect(login.container.querySelector('.conn')).not.toBeNull()
  login.unmount()

  routeFetch({ 'GET /api/auth/session': signedIn })
  window.history.replaceState(null, '', '/board')
  const board = mount(<App stream={inertStream()} />)
  await flush()
  expect(board.container.querySelector('.conn')).not.toBeNull()
  board.unmount()
})

function setValue(input: HTMLInputElement, value: string): void {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
  act(() => {
    setter.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

function submit(form: HTMLFormElement): void {
  act(() => {
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  })
}

function click(el: HTMLElement): void {
  act(() => el.click())
}
