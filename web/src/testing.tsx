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
