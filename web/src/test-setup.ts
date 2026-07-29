// Vitest setup: React 19 refuses to run `act` outside an act environment, and
// the flag is a global rather than an import. Declared here so every probe can
// drive real render/commit cycles instead of asserting on markup strings.
declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true

/**
 * ResizeObserver as a no-op.
 *
 * It is a LAYOUT API, and a headless DOM has no layout engine — jsdom does not
 * implement it and no DOM double could implement it meaningfully, so this is
 * not a reason to swap the environment. @assistant-ui/react observes content
 * size for its viewport auto-scroll and constructs one on mount; a no-op lets
 * the component tree mount and commit, which is what the probes assert.
 * Anything that genuinely depends on measured size is a real-browser concern
 * (the B6-9 device drill), never a jsdom assertion.
 */
if (!('ResizeObserver' in globalThis)) {
  class NoopResizeObserver implements ResizeObserver {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  globalThis.ResizeObserver = NoopResizeObserver
}

export {}
