import path from 'node:path'

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

/**
 * The dev-proxy target environment variable. There is deliberately NO default.
 *
 * `127.0.0.1:8482` is `shell.DefaultHTTPAddr` — and on the maintainer's host it
 * is the port the LIVE production `sinet-control` unit holds. A defaulted proxy
 * would silently point a dev SPA at the production control plane, so the dev
 * server refuses to start until a target is named (P3/CONVENTIONS.md §41).
 */
const devAddrEnv = 'VITE_SINET_ADDR'

function devProxyTarget(): string {
  const addr = process.env[devAddrEnv]
  if (addr) return addr
  throw new Error(
    `${devAddrEnv} is required to run the dev server.\n\n` +
      'The Vite dev server proxies /api and /events to a control plane, and this\n' +
      'config has no default on purpose: 127.0.0.1:8482 is shell.DefaultHTTPAddr\n' +
      'and, on this host, the LIVE production sinet-control unit. Point the dev\n' +
      'SPA at a DEV control plane on a free port instead:\n\n' +
      '  go run ./cmd/sinet --http-addr 127.0.0.1:<free-port>\n' +
      `  ${devAddrEnv}=http://127.0.0.1:<free-port> npm run dev\n`,
  )
}

/**
 * True only when this config is about to back a browser-facing dev server.
 *
 * `command === 'serve'` is not enough on its own: vitest boots a Vite server in
 * `serve` mode to transform the test modules, and a test run must never resolve
 * a proxy target — tests are hermetic and dial nothing (`process.env.VITEST` is
 * vitest's own documented marker for its config/test process).
 */
function isDevServer(command: string): boolean {
  return command === 'serve' && !process.env.VITEST
}

// https://vite.dev/config/
export default defineConfig(({ command }) => ({
  // Tailwind v4 is a COMPILER, not a runtime: the plugin reads the `@import
  // "tailwindcss"` in index.css and emits only the utilities the sources use.
  // No Tailwind code is reachable from the binary, which serves built assets
  // alone (P3/CONVENTIONS.md §47).
  plugins: [react(), tailwindcss()],
  resolve: {
    // `@/…` is the shadcn CLI's own import convention, so generated components
    // resolve their siblings without a hand edit. Mirrored in tsconfig.app.json.
    //
    // Resolved from the working directory rather than `import.meta.url`: the
    // harness imports this very module through Vite's own transform to assert
    // the dev-proxy rules, and there `import.meta.url` is not a file URL. Every
    // invocation that reads this config — `vite`, `vite build`, `vitest` — runs
    // with `web/` as the working directory, which is also this config's root.
    alias: { '@': path.resolve(process.cwd(), 'src') },
  },
  build: {
    // The build output IS the embed source: Go embeds internal/webui/dist into
    // the one release binary (Spec S01.5). `web/` stays a pure npm tree with no
    // Go file in it; `internal/webui` stays the only Go home for the assets.
    outDir: '../internal/webui/dist',
    // Required by Vite to clean an outDir outside the project root, and the
    // reason the embedded FS never carries assets from an older build.
    emptyOutDir: true,
  },
  // `build` and the vitest runs never resolve a proxy target, so a tree with no
  // VITE_SINET_ADDR still typechecks, tests and builds — the requirement bites
  // exactly where the hazard is.
  ...(isDevServer(command)
    ? {
        server: {
          // Loopback only: the dev server is never a LAN/tailnet surface.
          host: '127.0.0.1',
          proxy: {
            // The two server surfaces the SPA speaks to (Spec S15.2/S15.3).
            // http-proxy streams responses through unbuffered and Vite adds no
            // compression on a proxied leg, so /events stays a live SSE tail.
            //
            // A `^` key is a RegExp, and these are anchored on a path BOUNDARY
            // so dev matches what production enforces: `isAPIPath` in
            // internal/api/spa.go treats /api and /api/… as the machine
            // surface and /apifoo as an app route. A bare '/api' key here is a
            // PREFIX match, which would proxy /apifoo and make the dev server
            // disagree with the binary it stands in for.
            '^/api(/|$)': { target: devProxyTarget() },
            '^/events(/|$)': { target: devProxyTarget() },
          },
        },
      }
    : {}),
}))
