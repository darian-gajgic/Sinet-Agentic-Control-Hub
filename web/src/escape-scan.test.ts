import { expect, test } from 'vitest'

/**
 * Escape-by-default, as a structural assertion (Spec S15.12; FC-v1 §2).
 *
 * React's JSX escaping is the mechanism, but a mechanism you can opt out of is
 * not a guarantee — so the opt-outs are banned outright and the ban is checked.
 * Model output and web-derived content are untrusted input; the ONE sanctioned
 * raw-HTML channel is the S15.8 preview iframe, which does not exist yet.
 *
 * THE ALLOWLIST IS EMPTY. It widens at B6-8, in B6-8's own commit, with the
 * preview surface it exists for — never quietly, and never here.
 */
const allowlist: string[] = []

// Built by concatenation so the scanner's own source is not a hit: the scan
// covers EVERY file under src/, this one included.
const banned: { token: string; why: string }[] = [
  { token: 'dangerously' + 'SetInnerHTML', why: "React's escape hatch" },
  { token: 'inner' + 'HTML', why: 'raw HTML assignment' },
  { token: 'outer' + 'HTML', why: 'raw HTML assignment' },
  { token: 'document.' + 'write', why: 'raw document injection' },
  { token: 'eval' + '(', why: 'code execution from data' },
  { token: 'new ' + 'Function', why: 'code execution from data' },
  { token: 'insert' + 'AdjacentHTML', why: 'raw HTML injection' },
]

/** Every source file under src/, as text. import.meta.glob is Vite's own
 *  mechanism, so the scan needs no Node types and no extra dependency. */
const sources = import.meta.glob('./**/*.{ts,tsx}', { query: '?raw', import: 'default', eager: true }) as Record<
  string,
  string
>

function scan(files: Record<string, string>): string[] {
  const hits: string[] = []
  for (const [path, text] of Object.entries(files)) {
    if (allowlist.includes(path)) continue
    for (const { token, why } of banned) {
      if (text.includes(token)) hits.push(`${path}: ${token} (${why})`)
    }
  }
  return hits
}

test('the scan actually covers the source tree', () => {
  const paths = Object.keys(sources)
  // A scanner that silently matched nothing would pass forever.
  expect(paths.length).toBeGreaterThan(10)
  expect(paths).toContain('./App.tsx')
  expect(paths).toContain('./events.ts')
})

test('no raw-HTML or code-execution escape hatch exists in web/src', () => {
  expect(scan(sources)).toEqual([])
  expect(allowlist, 'the allowlist widens at B6-8 with the preview surface, not before').toEqual([])
})

// The planted probe: the scanner must be able to fail. Without this, an
// assertion that finds nothing proves nothing.
test('the scan catches a planted violation', () => {
  const planted = {
    './planted.tsx': 'export const boom = <div dangerouslySetInnerHTML={{ __html: untrusted }} />',
    './planted2.ts': 'el.innerHTML = untrusted',
    './planted3.ts': 'const f = new Function("return " + untrusted)',
  }
  const hits = scan(planted)

  expect(hits).toHaveLength(3)
  expect(hits[0]).toContain('dangerouslySetInnerHTML')
  expect(hits[1]).toContain('innerHTML')
  expect(hits[2]).toContain('new Function')
})
