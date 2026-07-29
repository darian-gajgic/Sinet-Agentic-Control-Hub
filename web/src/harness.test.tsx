import { act, useState } from 'react'
import { expect, test } from 'vitest'

import { mount } from './testing'

// The behavioral half of adopting the HARNESS itself (S16.4 #5): vitest 4.1.10
// + jsdom 30.0.0 must actually render and commit a React 19.2.8 tree through
// the pinned JSX transform, not merely import it. Every probe in this suite —
// the four pick probes above all — stands on this.

test('the harness renders a React tree into a real DOM', () => {
  const view = mount(<h1>Sinet</h1>)

  expect(view.container.querySelector('h1')?.textContent).toBe('Sinet')
  // A real commit into a real document, not a string comparison: the node is
  // attached to the jsdom document and reachable from it.
  expect(document.body.contains(view.container.querySelector('h1'))).toBe(true)
  view.unmount()
})

test('state updates commit through act', () => {
  function Counter() {
    const [n, setN] = useState(0)
    return (
      <button type="button" onClick={() => setN(n + 1)}>
        {n}
      </button>
    )
  }

  const view = mount(<Counter />)
  const button = view.container.querySelector('button')!
  expect(button.textContent).toBe('0')

  act(() => {
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
  expect(button.textContent).toBe('1')
  view.unmount()
})
