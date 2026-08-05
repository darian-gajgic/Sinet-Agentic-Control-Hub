import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { chatRoutes, chatSessionID, FakeSource, scriptedFetch } from './doubles'
import { EventStream } from './events'
import { resetHints } from './hints'
import { hrefFor } from './routes'
import { flush, mount } from './testing'
import hintsSource from './hints.tsx?raw'

/**
 * One-shot first-use hints and the served-gated teaching empties (P3-UI-7
 * R14/R12; design proposal §3 as narrowed by its own §7 amendment A1.2).
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

beforeEach(() => {
  resetHints()
  FakeSource.reset()
  vi.stubGlobal('EventSource', FakeSource)
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// The composer only exists inside an OPEN conversation, so the hint that names
// its verb picker is only reachable there — which is exactly where a first-use
// hint about choosing a verb belongs.
async function openChat(extra: Record<string, { body?: unknown; pending?: boolean }> = {}) {
  const log = scriptedFetch({ ...chatRoutes(), ...extra })
  window.history.replaceState(null, '', `${hrefFor('chat')}?session=${chatSessionID}`)
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}

const click = (el: Element | null) => {
  expect(el).not.toBeNull()
  act(() => {
    el!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
}

test('a hint shows once per tab session, is dismissible, and stays dismissed', async () => {
  const first = await openChat()
  const hint = first.view.container.querySelector('[data-hint="composer-verb"]')
  expect(hint, 'the first-use hint never appeared').not.toBeNull()

  click(first.view.container.querySelector('[data-hint-dismiss="composer-verb"]'))
  await flush()
  expect(
    first.view.container.querySelector('[data-hint="composer-verb"]'),
    'dismissing the hint did not remove it',
  ).toBeNull()
  first.view.unmount()

  // ONCE PER TAB SESSION: a second mount in the same module lifetime does not
  // show it again. This is the whole persistence story — see the narrowing.
  document.querySelectorAll('body > div').forEach((n) => n.remove())
  const second = await openChat()
  expect(
    second.view.container.querySelector('[data-hint="composer-verb"]'),
    'the hint came back within the same tab session',
  ).toBeNull()
  second.view.unmount()
})

test('the hint never covers the control it names, and stores nothing anywhere', async () => {
  const { view } = await openChat()
  const hint = view.container.querySelector('[data-hint="composer-verb"]') as HTMLElement
  // In normal flow: no overlay, no portal, no absolute/fixed positioning. A tip
  // you must dismiss before you can use the thing it describes is worse than none.
  expect(hint.className).not.toMatch(/\b(absolute|fixed)\b/)
  expect(hint.closest('[data-tour-overlay]'), 'the hint rendered inside the tour overlay').toBeNull()
  // The control it names is on the page and reachable while the hint is up.
  expect(view.container.querySelector('.chat-verbs'), 'the hint names a control that is not there').not.toBeNull()

  // THE STORAGE BAN, at the module that would be tempted to break it.
  for (const banned of ['localStorage', 'sessionStorage', 'indexedDB', 'document.cookie', 'setInterval', 'setTimeout']) {
    expect(hintsSource, `the hints module reached for ${banned}`).not.toContain(banned)
  }
  // The narrowing is written down where the mechanism is.
  expect(hintsSource).toContain('ONCE PER TAB SESSION')
  view.unmount()
})

test('the count stays small, and every hint sits on a surface this packet may open', () => {
  const all = [...hintsSource.matchAll(/data-hint="([^"]+)"/g)].map((m) => m[1])
  // The teaching layer is the tour plus the empty states; hints are seasoning.
  expect(all.length).toBeLessThanOrEqual(2)
})

// ── R12: the teaching empties are SERVED-GATED ────────────────────────────

test('the conversation rail teaches only once its read has ANSWERED', async () => {
  // THE §51-D1 CLASS. The rail read `sessions.data?.sessions ?? []`, so a read
  // still in flight rendered as a served empty and taught "no conversations
  // yet" about a question the platform had not answered.
  const pending = await openChat({ 'GET /api/chat/sessions': { pending: true } })
  const railText = pending.view.container.textContent ?? ''
  expect(railText, 'the rail taught over a read that has not answered').not.toContain('No conversations yet.')
  pending.view.unmount()
  document.querySelectorAll('body > div').forEach((n) => n.remove())

  // ...and once it answers EMPTY, the teaching state is exactly what appears.
  const answered = await openChat({ 'GET /api/chat/sessions': { body: { sessions: [] } } })
  const text = answered.view.container.textContent ?? ''
  expect(text, 'an answered-empty read did not teach').toContain('No conversations yet.')
  // The `why` teaches what makes a row appear, which is what makes it a
  // teaching state rather than a label.
  expect(text).toContain('conversations live on the platform')
  answered.view.unmount()
})
