import { expect, test } from 'vitest'

import { hrefFor, matchRoute, routes } from './routes'

// The URL contract (§41-B). These paths are the deep-link targets S15.11's
// push `navigate` field will carry, so they are pinned here: a later packet
// fills a surface in, it never renames the route.
test('the URL contract is exactly the recorded set', () => {
  expect(routes.map((r) => `${r.id} ${r.pattern}`)).toEqual([
    'mission-control /',
    'board /board',
    'task /tasks/:id',
    'fleet /fleet',
    'inbox /inbox',
    'inbox-item /inbox/:id',
    'settings /settings',
    'chat /chat',
    'deliverable /deliverables/:id',
    'workforce /workforce',
    'login /login',
  ])
})

test('every surface names the packet that will build it', () => {
  for (const r of routes) {
    if (r.id === 'login') continue // built here
    expect(r.owner, `route ${r.id} names no owning packet`).toMatch(/^B6-[5-9]$/)
  }
})

test('paths resolve to their route and capture their identity', () => {
  expect(matchRoute('/').route.id).toBe('mission-control')
  expect(matchRoute('/board').route.id).toBe('board')
  expect(matchRoute('/inbox').route.id).toBe('inbox')

  const item = matchRoute('/inbox/ask-7')
  expect(item.route.id).toBe('inbox-item')
  expect(item.params).toEqual({ id: 'ask-7' })

  const task = matchRoute('/tasks/t-1')
  expect(task.route.id).toBe('task')
  expect(task.params).toEqual({ id: 't-1' })
})

// /inbox and /inbox/:id are different surfaces; a one-segment path must never
// fall into the two-segment route or a deep link would render the wrong page.
test('a deeper path never collapses into a shallower route', () => {
  expect(matchRoute('/inbox/ask-7/extra').route.id).toBe('not-found')
  expect(matchRoute('/deliverables').route.id).toBe('not-found')
})

test('an unknown path resolves to not-found rather than throwing', () => {
  expect(matchRoute('/nope').route.id).toBe('not-found')
  expect(matchRoute('/v1/api/runs').route.id).toBe('not-found')
})

test('identities survive a round trip through the URL', () => {
  const weird = 'ask/with space#hash'
  const href = hrefFor('inbox-item', { id: weird })
  expect(href).not.toContain(' ')
  expect(matchRoute(href).params.id).toBe(weird)
})
