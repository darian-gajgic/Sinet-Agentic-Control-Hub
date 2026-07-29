// Vitest setup: React 19 refuses to run `act` outside an act environment, and
// the flag is a global rather than an import. Declared here so every probe can
// drive real render/commit cycles instead of asserting on markup strings.
declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true

export {}
