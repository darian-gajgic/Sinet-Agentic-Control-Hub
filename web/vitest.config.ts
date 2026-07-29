import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config.ts'

// The harness config is separate so `vite.config.ts` — the production build
// config — never imports the test runner. It reuses the build config verbatim
// (the React plugin above all: the probes compile the same JSX the app does).
export default defineConfig((env) =>
  mergeConfig(viteConfig(env), {
    test: {
      // jsdom is the default DOM environment: the probes render real React
      // trees, and import-compiles is not the bar (S16.4 #5).
      environment: 'jsdom',
      // Vitest stubs CSS to empty by default. The responsive skeleton is
      // asserted from the stylesheet itself, so it has to be real here.
      css: true,
      // The suite runs in a NON-UTC zone on purpose (B6-5).
      //
      // Every timestamp this platform stores and serves is UTC, and the one
      // place the client converts between the two is "finished today" — local
      // midnight, sent as the UTC instant the API compares against. In UTC that
      // conversion is the identity function, so a broken one would pass. Pinned
      // rather than left to the host so the result does not depend on where the
      // suite runs; +12/+13 is deliberately a zone whose local date differs from
      // the UTC date for half the day.
      env: { TZ: 'Pacific/Auckland' },
      globals: false,
      setupFiles: ['./src/test-setup.ts'],
      include: ['src/**/*.test.{ts,tsx}'],
    },
  }),
)
