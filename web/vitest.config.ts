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
      globals: false,
      setupFiles: ['./src/test-setup.ts'],
      include: ['src/**/*.test.{ts,tsx}'],
    },
  }),
)
