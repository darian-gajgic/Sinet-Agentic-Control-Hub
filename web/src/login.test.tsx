import { afterEach, expect, test, vi } from 'vitest'

import { Login } from './Login'
import type { Session } from './api'
import { scriptedFetch } from './doubles'
import { flush, mount } from './testing'

/**
 * The login surface's own CLIENT-AUTHORED copy (P3-UI-6; S01.9; B6 gate §9 A-1).
 *
 * WHY THIS FILE EXISTS AND IS NOT PART OF `shell.test.tsx`. Every landed login
 * assertion lives there — the picker's shape, the PIN's type, the bootstrap
 * window, the C-1 dev-posture cycle — and that file is byte-frozen for this
 * packet precisely because those pins are the restyle's regression net: they
 * have to pass UNMODIFIED or "the flow did not move" is a claim nobody checked.
 * The one thing they cannot cover is copy that did not exist when they were
 * written, so the what-line's facts and its overclaims get their own home here.
 * Nothing in this file re-tests the flow.
 *
 * It mounts `Login` directly, the way `push.test.tsx` mounts the push panel:
 * the shell's routing, redirects and session machinery are `shell.test.tsx`'s
 * subject and are untouched here.
 */

const users = [
  { user_id: 'alice', display_name: 'Alice', role: 'operator', pin_set: true },
  { user_id: 'bob', display_name: 'Bob', role: 'member', pin_set: true },
]

async function openLogin(session: Session, list: unknown[] = users) {
  scriptedFetch({ 'GET /api/auth/users': { body: { users: list } } })
  const view = mount(<Login session={session} onSignedIn={() => {}} />)
  await flush()
  return view
}

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

test('the picker says who is signing in, and promises nothing this page cannot do', async () => {
  const view = await openLogin({ authenticated: false })
  const what = view.container.querySelector('[data-surface-what]')?.textContent ?? ''

  // The facts, and each is one the page really shows or the flow really keeps.
  expect(what, 'the line does not say what the page is for').toContain('Who is signing in on this device')
  expect(what, 'the line drops the shared-house fact the picker exists for').toContain("accounts are the household's")
  expect(what).toContain('pick yours and enter your PIN')
  // The client holds no credential and no token: the cookie is HttpOnly and the
  // PIN lives in component state only until the request that spends it returns.
  expect(what).toContain('checked by the platform')
  expect(what, 'the line does not say the browser keeps nothing').toContain('no copy of it and no token of its own')

  // The overclaims. There is no self-service account creation outside the
  // bootstrap window, no password reset and no remembered sign-in — a line
  // offering any of them would be a door that is not there.
  for (const claim of ['create an account', 'sign up', 'register', 'forgot', 'reset your pin', 'remember me', 'stay signed in']) {
    expect(what.toLowerCase(), `the line offers something the flow does not have: ${claim}`).not.toContain(claim)
  }
  view.unmount()
})

test('the bootstrap window states the window itself, and says it closes', async () => {
  const view = await openLogin({ authenticated: false }, [])
  const what = view.container.querySelector('[data-surface-what]')?.textContent ?? ''
  expect(view.container.textContent, 'the bootstrap window lost its title').toContain('First run')
  expect(what).toContain('No accounts exist yet')
  // D10 needs a holder from the first user, and this is the only anonymous
  // create there will ever be — both facts are on the line.
  expect(what).toContain('The first account is the operator')
  expect(what, 'the line does not say the window is once-only').toContain('only time one can be created without signing in')
  view.unmount()
})

test('the restyle kept the shapes the flow is pinned on: one form, a native picker, a password PIN', async () => {
  // These are the same shapes `shell.test.tsx` selects on, asserted here from
  // the other side so a restyle that broke them fails on this file too rather
  // than only where the frozen pins happen to look.
  const view = await openLogin({
    authenticated: false,
    hint: { device_login: 'alice@example.ts.net', user_id: 'bob', auto_login: false },
  })
  expect(view.container.querySelectorAll('form'), 'the page grew a second form').toHaveLength(1)
  const select = view.container.querySelector('select')!
  expect(select, 'the picker stopped being a native select').not.toBeNull()
  expect([...select.options].map((o) => o.value)).toEqual(['alice', 'bob'])
  expect(select.value, 'the device hint stopped prefilling the picker').toBe('bob')
  const pin = view.container.querySelector('input[type=password]') as HTMLInputElement
  expect(pin, 'the PIN stopped being a password field').not.toBeNull()
  expect(pin.autocomplete).toBe('current-password')
  view.unmount()
})

test('the login card reads at phone width and pins no pixel width', async () => {
  const pinned = /w-\[\d+px\]|min-w-\[\d+px\]|max-w-\[\d+px\]/
  const view = await openLogin({ authenticated: false })
  const nodes = [view.container.querySelector('.panel'), view.container.querySelector('[data-surface-what]')]
  for (const n of nodes) {
    expect(n, 'a render this leg is about is missing').not.toBeNull()
    expect(pinned.test((n as HTMLElement).className.toString()), 'a login render pins a pixel width').toBe(false)
    expect(/\d+px/.test(n?.getAttribute('style') ?? ''), 'a login render pins an inline width').toBe(false)
  }
  // Both-way probe on the detector.
  expect(pinned.test('panel w-[420px]')).toBe(true)
  expect(pinned.test('panel')).toBe(false)
  view.unmount()
})
