import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { ConnectionState } from './ConnectionState'
import { devFallbackSession } from './doubles'
import { EventStream, type EventSourceLike, type Status } from './events'
import { hrefFor, routes, type RouteDef } from './routes'
import { Stub } from './Stub'
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

test('a route published ahead of its surface falls to a stub that names its packet and reads nothing', () => {
  // This used to loop over the UNBUILT rows of the route table. B6-8 part B
  // filled the last one, so that loop now quantifies over an empty set — its
  // own floor said so, which is why it is replaced rather than deleted. What it
  // was really checking is a property of `Stub`, and that property still
  // matters: the URL contract moves BEFORE the surface does (a route is
  // published so deep links and push `navigate` payloads can be written against
  // it), and between those two moments this is what answers.
  const calls = routeFetch({ 'GET /api/auth/session': signedIn })
  const pending: RouteDef = {
    // The id is not what Stub renders — it renders the title, the owning packet
    // and the captured params — so any RouteID stands in for a row that does
    // not exist yet.
    id: 'not-found',
    pattern: '/example/:id',
    title: 'Example surface',
    owner: 'B7-1',
    nav: false,
  }
  const view = mount(<Stub route={pending} params={{ id: 'x-1' }} />)
  const text = view.container.textContent ?? ''
  expect(text, 'the stub does not render its surface title').toContain(pending.title)
  expect(text, 'the stub does not name its owning packet').toContain(pending.owner)
  expect(text, 'the stub does not show the identity the URL already carries').toContain('id=x-1')
  // "renders nothing from the API" is the other half of the promise, and it is
  // checked rather than read: no request left this component.
  expect(calls, 'a stub reached the control plane').toEqual([])
  view.unmount()
})

test('every route in the contract is built, so no URL answers with a stub', () => {
  // The state B6-8 part B leaves the table in. Two-sided: the assertion in
  // routes.test.ts still admits an unbuilt row that names its packet, and this
  // records that none exists today.
  expect(routes.filter((r) => r.owner !== '').map((r) => `${r.id} → ${r.owner}`)).toEqual([])
})

test('a built surface renders its own heading, not a stub', async () => {
  // The scripted plane answers only the session; every data read fails, which
  // is the point: even unreadable, a built surface says what it IS rather than
  // claiming to be unbuilt.
  routeFetch({ 'GET /api/auth/session': signedIn })
  for (const id of ['mission-control', 'board', 'chat'] as const) {
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
  //
  // The nav is GROUPED as of UI-1, and this assertion is what proves grouping
  // is presentation only: the rendered links are still exactly the table's
  // navigable rows, in exactly the table's order. Grouping computes the
  // consecutive runs of a label over that order, so it cannot reorder or drop.
  const hrefs = [...view.container.querySelectorAll('.shell-nav a')].map((a) => a.getAttribute('href'))
  expect(hrefs).toEqual(routes.filter((r) => r.nav).map((r) => r.pattern))
  view.unmount()
})

test('the nav is grouped, and every navigable route sits inside a group', async () => {
  routeFetch({ 'GET /api/auth/session': signedIn })

  const view = mount(<App stream={inertStream()} />)
  await flush()

  const groups = [...view.container.querySelectorAll('.shell-nav .nav-group')]
  expect(groups.length, 'the nav renders no groups').toBeGreaterThan(1)
  // Every link belongs to a group: a link rendered outside one would be a
  // surface that fell out of the grouping and lost its heading.
  const inGroups = groups.flatMap((g) => [...g.querySelectorAll('a')])
  expect(inGroups).toHaveLength(routes.filter((r) => r.nav).length)
  // The labels are real text, not empty headings.
  const labels = groups.map((g) => g.querySelector('.nav-group-label')?.textContent ?? '')
  expect(labels.every((l) => l.length > 0), 'a group renders an empty label').toBe(true)
  view.unmount()
})

test('the aurora is one decorative field the reader cannot reach', async () => {
  routeFetch({ 'GET /api/auth/session': signedIn })

  const view = mount(<App stream={inertStream()} />)
  await flush()

  const auroras = view.container.querySelectorAll('.aurora')
  // ONE field, behind everything — not one per surface.
  expect(auroras).toHaveLength(1)
  // It carries no meaning, so it is hidden from assistive technology outright
  // rather than left for a screen reader to describe.
  expect(auroras[0].getAttribute('aria-hidden')).toBe('true')
  expect(auroras[0].textContent, 'the backdrop carries content').toBe('')
  view.unmount()
})

test('the brand mark leads back to mission control and says nothing twice', async () => {
  routeFetch({ 'GET /api/auth/session': signedIn })

  const view = mount(<App stream={inertStream()} />)
  await flush()

  const brand = view.container.querySelector('.brand')!
  expect(brand.getAttribute('href')).toBe(hrefFor('mission-control'))
  // The gradient mark is decoration beside the wordmark, so it is hidden rather
  // than announced as a second copy of the name.
  expect(brand.querySelector('.brand-mark')?.getAttribute('aria-hidden')).toBe('true')
  expect(brand.textContent).toContain('Sinet')
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

// ── C-1: the dev-posture login defeat (B6 gate §9), fixed at P3-UI-4 ──────
//
// THE SHAPES THESE ARMS DISCRIMINATE ON ARE THE SERVED ONES, not this file's
// idea of them. `SessionAuthenticator` resolves every session-less request in
// dev posture to the fixed dev identity (internal/api/identity.go:83–97), and
// `handleAuthSession` serves that as `{authenticated:true, dev:true}` with NO
// `user` object — the branch that fills one runs only `if !id.Dev`
// (internal/api/auth_handlers.go:91–104). A real session is the other shape:
// `user` present, `dev` omitted. Real cookies always win (identity.go:84–92).
//
// So `authed` was permanently true in dev: /login was unreachable and the header
// offered a Sign out whose logout cleared nothing the next resolve would not
// hand straight back.

test('the dev fallback reaches the login picker instead of being bounced off it', async () => {
  routeFetch({
    'GET /api/auth/session': devFallbackSession,
    'GET /api/auth/users': {
      body: { users: [{ user_id: 'alice', display_name: 'Alice', role: 'operator', pin_set: true }] },
    },
  })
  window.history.replaceState(null, '', '/login')

  const view = mount(<App stream={inertStream()} />)
  await flush()

  expect(window.location.pathname, 'the dev fallback was bounced off /login again').toBe('/login')
  // Scoped to MAIN: under the dev fallback the topbar renders the project
  // selector (also a <select>), and this assertion is about the login picker.
  const select = view.container.querySelector('main select')! as HTMLSelectElement
  expect(select, 'the picker did not render under the dev fallback').not.toBeNull()
  expect([...select.options].map((o) => o.value)).toEqual(['alice'])
  // Login.tsx is byte-unchanged: it always could render here, and this is the
  // one layer above it that would not let it.
  expect(view.container.querySelector('input[type=password]')).not.toBeNull()
  view.unmount()
})

test('a real session still bounces off /login, so the carve-out cannot be reached by omission', async () => {
  // The other direction of the same arm, and the one that must NOT move: in
  // production there is no fallback, `dev` is never true, and this is byte-for-
  // byte the behaviour that shipped.
  routeFetch({ 'GET /api/auth/session': signedIn })
  window.history.replaceState(null, '', '/login?next=%2Fboard')

  const view = mount(<App stream={inertStream()} />)
  await flush()

  expect(window.location.pathname, 'a real session was left sitting on the login picker').toBe('/board')
  view.unmount()
})

test('under the dev fallback the header offers Sign in, never a Sign out that cannot work', async () => {
  routeFetch({ 'GET /api/auth/session': devFallbackSession })

  const view = mount(<App stream={inertStream()} />)
  await flush()

  const who = view.container.querySelector('.who')!
  // The who-line keeps its honest label: this is not a person.
  expect(who.textContent).toContain('dev')
  expect(who.querySelector('[data-auth="sign-out"]'), 'a Sign out that revokes nothing is offered').toBeNull()
  const signIn = who.querySelector('[data-auth="sign-in"]')!
  expect(signIn, 'the dev posture offers no affordance that reaches the picker').not.toBeNull()
  // ▲ W1-B1b (cold walk 2026-08-17): the link carries WHERE YOU ARE, so
  // signing in returns to this exact page and query — never a dump to Home.
  const here = window.location.pathname + window.location.search
  expect(signIn.getAttribute('href')).toBe(`${hrefFor('login')}?next=${encodeURIComponent(here)}`)
  expect(signIn.textContent).toBe('Sign in')
  // NO INVENTED STATE: the client renders the two states the server serves and
  // never synthesizes a third. The dev identity still browses — deny-by-default
  // is the server's job and it already holds — so the nav is intact.
  expect(view.container.querySelectorAll('.shell-nav a').length).toBeGreaterThan(0)

  // 375px (§41-B): the affordance is the packet's one new piece of header
  // markup, and a Sign in nobody can reach on a phone is the same defeat C-1
  // was. jsdom has no layout engine, so what is checkable is the structure —
  // it pins no pixel width, and it inherits the header the landed responsive
  // suite already asserts wraps rather than overflowing.
  const pinned = /w-\[\d+px\]|min-w-\[\d+px\]/
  expect(pinned.test(signIn.className.toString()), 'the Sign in affordance pins a pixel width').toBe(false)
  expect(/\d+px/.test(signIn.getAttribute('style') ?? ''), 'the Sign in affordance pins an inline width').toBe(false)
  expect(pinned.test(who.className.toString()), 'the who-line pins a pixel width around it').toBe(false)
  // Probe: the detector really matches what it forbids.
  expect(pinned.test('w-[520px] flex')).toBe(true)
  view.unmount()
})

test('the whole dev-posture cycle is honest: fallback → picker → real session → sign out → fallback', async () => {
  const impl: Record<string, Route> = {
    'GET /api/auth/session': devFallbackSession,
    'GET /api/auth/users': {
      body: { users: [{ user_id: 'alice', display_name: 'Alice', role: 'operator', pin_set: true }] },
    },
    'POST /api/auth/login': { body: { user_id: 'alice', expires: '2026-08-28T00:00:00Z' } },
    'POST /api/auth/logout': { status: 204 },
  }
  const calls = routeFetch(impl)
  window.history.replaceState(null, '', '/login')

  const view = mount(<App stream={inertStream()} />)
  await flush()
  expect(view.container.querySelector('main select'), 'the cycle cannot start: no picker').not.toBeNull()
  expect(view.container.querySelector('[data-auth="sign-in"]')).not.toBeNull()

  // (2) A REAL LOGIN. The cookie is the server's and JS cannot read it, so what
  //     is proved here is that the act FIRED and that the next SESSION READ is
  //     what changed the screen — never a state this client flipped itself.
  setValue(view.container.querySelector('input[type=password]') as HTMLInputElement, '4321')
  impl['GET /api/auth/session'] = signedIn
  submit(view.container.querySelector('form')!)
  await flush()
  expect(calls.some((c) => c.method === 'POST' && c.path === '/api/auth/login')).toBe(true)

  // (3) The real session now bounces off /login exactly as it always did, and
  //     the header offers the act it CAN honour.
  expect(window.location.pathname).toBe(hrefFor('mission-control'))
  expect(view.container.querySelector('[data-auth="sign-in"]'), 'Sign in survived a real session').toBeNull()
  const out = view.container.querySelector('[data-auth="sign-out"]') as HTMLButtonElement
  expect(out, 'a real session was offered no way out').not.toBeNull()

  // (4) Signing out revokes and re-reads; the server falls back to dev again and
  //     the offer goes back to the one that works.
  impl['GET /api/auth/session'] = devFallbackSession
  click(out)
  await flush()
  expect(calls.some((c) => c.method === 'POST' && c.path === '/api/auth/logout')).toBe(true)
  expect(view.container.querySelector('[data-auth="sign-out"]'), 'a Sign out survived the logout it fired').toBeNull()
  expect(view.container.querySelector('[data-auth="sign-in"]')).not.toBeNull()
  view.unmount()
})

// ── the sign-in-first wall (W1-B1, cold walk 2026-08-17 — release-gating) ──
//
// The trap: work created under the dev fallback was owned by `dev`, which
// cannot step up at the PIN ceremony (S01.9) — so the task could never be
// approved by anyone, and the walk's errand died in a circle. The wall stands
// where work would be born: the give-work doors present the sign-in step IN
// PLACE, before any work exists, and signing in unlocks the same address.

test('the give-work door walls itself behind sign-in under the dev fallback, and unlocks IN PLACE (W1-B1)', async () => {
  const impl: Record<string, Route> = {
    'GET /api/auth/session': devFallbackSession,
    'GET /api/auth/users': {
      body: {
        users: [
          { user_id: 'alice', display_name: 'Alice', role: 'member', pin_set: true },
          { user_id: 'op', display_name: 'Op', role: 'operator', pin_set: true },
        ],
      },
    },
    'POST /api/auth/login': { body: { user_id: 'alice', expires: '2026-08-28T00:00:00Z' } },
  }
  const calls = routeFetch(impl)
  // The walk's exact door, project pin and all.
  window.history.replaceState(null, '', '/new?project=demo')

  const view = mount(<App stream={inertStream()} />)
  await flush()

  // The wall stands IN PLACE of the ask box — no work can be born as dev.
  const door = view.container.querySelector('[data-door="describe-goal"]')!
  expect(door.querySelector('[data-signin-first]'), 'the dev fallback reached a work-creating door').not.toBeNull()
  expect(door.querySelector('textarea'), 'the ask box renders before sign-in').toBeNull()

  // The picker explains the seeded people (W1-2): who each account is, and
  // that created work belongs to the picked account.
  expect(door.querySelector('[data-seeded-note]')).not.toBeNull()
  const labels = [...door.querySelectorAll('option')].map((o) => o.textContent ?? '')
  expect(labels.some((l) => l.includes('Alice') && l.includes('alice') && l.includes('household member'))).toBe(true)
  expect(labels.some((l) => l.includes('Op') && l.includes('operator'))).toBe(true)

  // Signing in unlocks the door IN PLACE: same address, query intact — never
  // a dump to Home (W1-B1b).
  setValue(door.querySelector('input[type=password]') as HTMLInputElement, '4321')
  impl['GET /api/auth/session'] = signedIn
  submit(door.querySelector('form')!)
  await flush()
  expect(calls.some((c) => c.method === 'POST' && c.path === '/api/auth/login')).toBe(true)
  expect(window.location.pathname + window.location.search).toBe('/new?project=demo')
  expect(view.container.querySelector('[data-signin-first]'), 'the wall outlived the sign-in').toBeNull()
  expect(view.container.querySelector('[data-door="describe-goal"] textarea'), 'the ask box did not unlock').not.toBeNull()
  view.unmount()
})

test('the chat give-work door stands behind the same wall under the dev fallback (W1-B1)', async () => {
  routeFetch({
    'GET /api/auth/session': devFallbackSession,
    'GET /api/auth/users': {
      body: { users: [{ user_id: 'alice', display_name: 'Alice', role: 'member', pin_set: true }] },
    },
  })
  window.history.replaceState(null, '', '/chat')

  const view = mount(<App stream={inertStream()} />)
  await flush()

  expect(view.container.querySelector('[data-signin-first]'), 'chat hands work to intake and was reachable as dev').not.toBeNull()
  expect(view.container.querySelector('.chat-body'), 'the conversation surface renders before sign-in').toBeNull()
  view.unmount()
})

test('a resume of somebody else\'s task refuses with WHO you are, not a false Inbox promise (W1-B1c)', async () => {
  routeFetch({
    'GET /api/auth/session': signedIn,
    // The server's own classification: this task is not among the caller's.
    'GET /api/tasks/t-devborn': { status: 403, body: { error: 'forbidden' } },
  })
  window.history.replaceState(null, '', '/new?task=t-devborn')

  const view = mount(<App stream={inertStream()} />)
  await flush()

  const refusal = view.container.querySelector('.door-refusal')!
  expect(refusal, 'no refusal rendered for a 403 resume').not.toBeNull()
  const text = refusal.textContent ?? ''
  // Identity-aware: it names the signed-in account and explains ownership.
  expect(text).toContain('alice')
  expect(text).toContain('belongs to the account that created it')
  // The old copy's false promise is gone: this card will never be in alice's
  // Inbox, so the refusal must not point there.
  expect(text, 'the refusal still promises an Inbox that cannot hold the card').not.toContain('Inbox')
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
