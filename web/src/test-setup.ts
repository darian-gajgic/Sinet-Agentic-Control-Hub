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

/**
 * Element.scrollTo as a no-op, for the SAME reason (B6-7).
 *
 * jsdom implements scrolling on `window` and not on elements, because there is
 * nothing to scroll: no layout, no viewport, no overflow. @assistant-ui/react's
 * thread viewport calls `div.scrollTo` from a requestAnimationFrame to keep the
 * newest turn in view, so without this a mounted transcript throws from a frame
 * callback AFTER the assertions have passed — an unhandled error rather than a
 * failure, which is worse than either. Scroll POSITION is a real-browser concern
 * and belongs to the B6-9 device drill; nothing here asserts on it.
 */
if (typeof Element.prototype.scrollTo !== 'function') {
  Element.prototype.scrollTo = function scrollTo(): void {}
}

export {}
