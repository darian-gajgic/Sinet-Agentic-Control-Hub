import { expect, test, vi } from 'vitest'
import { act } from 'react'

import { ErrorBoundary } from './boundary'
import { mount } from './testing'

/**
 * The F1 contract (operator findings 2026-08-16): an unhandled render
 * exception must never unmount the root into a black screen. The boundary
 * catches, says what broke in plain words plus the exception's own message,
 * and offers an in-place recovery.
 */

function Bomb({ armed }: { armed: boolean }) {
  if (armed) throw new Error('the probe exploded on purpose')
  return <p data-alive="true">alive</p>
}

test('a render crash lands on the fault card, not a dead root', () => {
  const quiet = vi.spyOn(console, 'error').mockImplementation(() => undefined)
  const view = mount(
    <ErrorBoundary grain="view" resetKey="a">
      <Bomb armed />
    </ErrorBoundary>,
  )
  const card = view.container.querySelector('[data-boundary="view"]')
  expect(card, 'the crash was not caught — this is the black-screen class').not.toBeNull()
  const text = card?.textContent ?? ''
  expect(text).toContain('This page hit a fault')
  expect(text, 'the exception message is the one honest technical line').toContain('the probe exploded on purpose')
  expect(card?.querySelector('[data-fault-act="retry"]'), 'no in-place recover affordance').not.toBeNull()
  view.unmount()
  quiet.mockRestore()
})

test('try-again recovers in place once the fault is gone', () => {
  const quiet = vi.spyOn(console, 'error').mockImplementation(() => undefined)
  let armed = true
  function Sometimes() {
    return <Bomb armed={armed} />
  }
  const view = mount(
    <ErrorBoundary grain="view" resetKey="a">
      <Sometimes />
    </ErrorBoundary>,
  )
  expect(view.container.querySelector('[data-boundary="view"]')).not.toBeNull()
  armed = false
  const retry = view.container.querySelector<HTMLButtonElement>('[data-fault-act="retry"]')
  act(() => {
    retry?.click()
  })
  expect(view.container.querySelector('[data-alive="true"]'), 'retry did not re-render the child').not.toBeNull()
  expect(view.container.querySelector('[data-boundary="view"]')).toBeNull()
  view.unmount()
  quiet.mockRestore()
})

test('navigation resets the fence: a crash on one page never follows the reader to the next', () => {
  const quiet = vi.spyOn(console, 'error').mockImplementation(() => undefined)
  let armed = true
  function Sometimes() {
    return <Bomb armed={armed} />
  }
  const view = mount(
    <ErrorBoundary grain="view" resetKey="page-one">
      <Sometimes />
    </ErrorBoundary>,
  )
  expect(view.container.querySelector('[data-boundary="view"]')).not.toBeNull()
  armed = false
  act(() => {
    view.root.render(
      <ErrorBoundary grain="view" resetKey="page-two">
        <Sometimes />
      </ErrorBoundary>,
    )
  })
  expect(view.container.querySelector('[data-alive="true"]'), 'the dead fence survived a route change').not.toBeNull()
  view.unmount()
  quiet.mockRestore()
})

test('the app grain offers the full reload, because app state is gone with the crash', () => {
  const quiet = vi.spyOn(console, 'error').mockImplementation(() => undefined)
  const view = mount(
    <ErrorBoundary grain="app">
      <Bomb armed />
    </ErrorBoundary>,
  )
  expect(view.container.querySelector('[data-fault-act="reload"]')).not.toBeNull()
  view.unmount()
  quiet.mockRestore()
})
