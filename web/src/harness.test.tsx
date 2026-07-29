import { act, useState, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, expect, test } from 'vitest'

import App from './App'

// The behavioral half of adopting the harness itself (S16.4 #5): vitest 4.1.10 +
// jsdom 30.0.0 must actually render a React 19.2.8 tree through the pinned JSX
// transform, not merely import it. The pick probes (B6-4 part B) stand on this.

let container: HTMLDivElement | null = null

afterEach(() => {
  container?.remove()
  container = null
})

function mount(node: ReactNode): HTMLElement {
  container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  act(() => {
    root.render(node)
  })
  return container
}

test('the harness renders a React tree into a real DOM', () => {
  const host = mount(<App />)

  expect(host.querySelector('h1')?.textContent).toBe('Sinet')
  // A real commit into a real document, not a string comparison: the node is
  // attached to the jsdom document and reachable from it.
  expect(document.body.contains(host.querySelector('h1'))).toBe(true)
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

  const host = mount(<Counter />)
  const button = host.querySelector('button')!
  expect(button.textContent).toBe('0')

  act(() => {
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
  expect(button.textContent).toBe('1')
})
