import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'

/**
 * The minimum render helper the probes need. No testing library is adopted at
 * B6-4 (a library is a dependency and none is ratified for this); the probes
 * drive react-dom/client and `act` directly, which is what the pinned React
 * itself supports.
 */
export type Mounted = {
  container: HTMLElement
  root: Root
  unmount(): void
}

export function mount(node: ReactNode): Mounted {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  act(() => {
    root.render(node)
  })
  return {
    container,
    root,
    unmount() {
      act(() => root.unmount())
      container.remove()
    },
  }
}

/** flush lets queued promise callbacks (a fetch double, a probe) settle inside
 *  an act scope, so the resulting render is committed before assertions. */
export async function flush(times = 3): Promise<void> {
  for (let i = 0; i < times; i++) {
    await act(async () => {
      await Promise.resolve()
    })
  }
}

/**
 * settle waits REAL time inside an act scope, for the one thing microtasks
 * cannot reach: a debounced callback.
 *
 * JSON Forms debounces its `onChange` by 10ms (jsonforms-react's
 * `debouncedEmit`), so a parent that mirrors the form model does not see an
 * edit until that timer fires. `flush` drains microtasks and never will.
 */
export async function settle(ms = 25): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms))
  })
}

/**
 * typeInto sets a controlled input's value the way a person does.
 *
 * The native setter is called explicitly because React tracks the last value it
 * wrote on the DOM node itself: assigning `el.value` directly updates the node
 * but leaves React's tracker in step with it, so the `input` event is treated
 * as a no-op change and `onChange` never fires. Going through the prototype
 * setter is what makes the tracker see a difference.
 */
export function typeInto(el: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set
  act(() => {
    setter?.call(el, value)
    el.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

/** choose picks a <select> option the same way. */
export function choose(el: HTMLSelectElement, value: string): void {
  const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')?.set
  act(() => {
    setter?.call(el, value)
    el.dispatchEvent(new Event('change', { bubbles: true }))
  })
}

/** click drives a real click, so a button's disabled state and a checkbox's
 *  own toggle behave exactly as they would for a person. */
export function click(el: Element | null | undefined): void {
  if (!el) throw new Error('click: no such element')
  act(() => {
    ;(el as HTMLElement).click()
  })
}
