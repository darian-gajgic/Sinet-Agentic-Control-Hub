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
 *
 * KNOWN BLIND SPOT, defused rather than hidden: `import.meta.glob` does not
 * include the importing module, so this file is the one file under src/ the
 * scan cannot see. Every banned token below — in the token list AND in the
 * planted fixtures — is therefore written SPLIT, so this file contains no
 * unsplit banned literal at all. The blind spot then costs nothing, and it
 * stays costing nothing if glob behaviour ever changes and the file starts
 * scanning itself.
 */
const allowlist: string[] = []
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
  // A scanner that silently matched nothing would pass forever. The floor moves
  // with the tree (10 at B6-4, 30 at B6-5, 35 at B6-6, 40 at B6-7) so "the scan
  // grew over the new views" is a checked fact rather than an assumption about a
  // glob.
  expect(paths.length).toBeGreaterThan(40)
  expect(paths).toContain('./App.tsx')
  expect(paths).toContain('./events.ts')
  // The B6-5 oversight surfaces are inside the scan, by name.
  for (const view of [
    './MissionControl.tsx',
    './Board.tsx',
    './TaskDetail.tsx',
    './Fleet.tsx',
    './Filters.tsx',
    // The B6-6 decision surfaces: card bodies are model-derived content and a
    // settings help string is registry text, so the two places raw HTML would
    // be tempting are inside the scan by name.
    './Inbox.tsx',
    './Settings.tsx',
    './settingsForm.tsx',
    // The B6-7 assistant: a transcript is the one surface whose content is
    // literally what somebody typed, and a chat widget is where a markdown
    // renderer would be reached for first. The transcript is plain escaped text
    // at v0 and the allowlist did not move for it.
    './Chat.tsx',
    './chatRuntime.ts',
    './chatFacts.ts',
  ]) {
    expect(paths, `${view} is not covered by the escape scan`).toContain(view)
  }
  // The blind spot, asserted so it stays a known fact rather than a belief.
  expect(paths, 'glob now includes this file — see the split-token note above').not.toContain(
    './escape-scan.test.ts',
  )
})

test('no raw-HTML or code-execution escape hatch exists in web/src', () => {
  expect(scan(sources)).toEqual([])
  expect(allowlist, 'the allowlist widens at B6-8 with the preview surface, not before').toEqual([])
})

// The planted probe: the scanner must be able to fail. Without this, an
// assertion that finds nothing proves nothing.
test('the scan catches a planted violation', () => {
  // Split, like every other token in this file: the fixtures must plant a
  // violation in the SCANNER's input without planting one in this SOURCE.
  const planted = {
    './planted.tsx': 'export const boom = <div dangerously' + 'SetInnerHTML={{ __html: untrusted }} />',
    './planted2.ts': 'el.inner' + 'HTML = untrusted',
    './planted3.ts': 'const f = new ' + 'Function("return " + untrusted)',
  }
  const hits = scan(planted)

  expect(hits).toHaveLength(3)
  expect(hits[0]).toContain('dangerously' + 'SetInnerHTML')
  expect(hits[1]).toContain('inner' + 'HTML')
  expect(hits[2]).toContain('new ' + 'Function')
})
