import { expect, test } from 'vitest'

/**
 * Escape-by-default, as a structural assertion (Spec S15.12; FC-v1 §2).
 *
 * React's JSX escaping is the mechanism, but a mechanism you can opt out of is
 * not a guarantee — so the opt-outs are banned outright and the ban is checked.
 * Model output and web-derived content are untrusted input; the ONE sanctioned
 * raw-HTML channel is the S15.8 preview iframe, which now exists.
 *
 * THE ALLOWLIST IS EMPTY, AND IT NEVER WIDENED (2026-07-30, P3-B6-8).
 *
 * FIVE landed texts — this header, the assertion below, and CONVENTIONS §41-B,
 * §43 and §44-B — promised that the allowlist would widen at B6-8 "with the
 * preview surface it exists for". All five are corrected. The COUNT here was wrong
 * twice before it was right: it read THREE and enumerated four (§44-B was missing
 * from both), then FOUR over the same five names (drain r2 R8 — the enumeration was
 * the part that was always complete). It did not widen, and the promise was wrong rather than
 * unkept: the sanctioned channel landed as an IFRAME BY `src`, which embeds a
 * browsing context BY REFERENCE and trips none of the constructs below. Every
 * banned token is a raw-HTML ASSIGNMENT or a code-execution call; an iframe with
 * a URL is neither, so it was never inside the banned set to be exempted from.
 * An allowlist entry nothing needs would have been a standing hole, so the
 * allowlist stays empty and the promise is corrected here instead.
 *
 * The one thing that DID move is a hardening: `srcdoc` joined the banned tokens,
 * because it is the exact class this list exists for — raw HTML by attribute —
 * and B6-8 is the packet that introduced iframes. The constraint "an iframe src
 * composes only from a served preview URL" now has a structural backstop, so the
 * one sanctioned channel cannot quietly become two.
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
  // The B6-8 hardening. An iframe's `src` names a document to fetch; its
  // `srcdoc` IS the document, inline, as markup — the banned class exactly. The
  // preview surface is the one place iframes exist in this tree, and it composes
  // src from a served session URL and the shell's own path, never from content.
  { token: 'src' + 'doc', why: 'raw HTML by iframe attribute' },
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
  // with the tree (10 at B6-4, 30 at B6-5, 35 at B6-6, 40 at B6-7, 42 at B6-8
  // part A, 44 at part B) so "the scan grew over the new views" is a checked fact
  // rather than an assumption about a glob.
  expect(paths.length).toBeGreaterThan(44)
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
    // The B6-8 review surface: it renders a deliverable's own diff text, a
    // person's comment bodies and the platform's commit trailers, and it is the
    // ONE file in the tree that mounts an iframe. If any file needed an escape
    // hatch it would be this one, and it is inside the scan by name.
    './Deliverable.tsx',
    // The B6-8 part-B workforce map: it renders worker DEFINITIONS — a
    // description, a prompt-body-adjacent selector set, tool and connector names
    // and a routing reason, all of which can be composer-written, which is model
    // output. It is inside the scan by name for that reason.
    './Workforce.tsx',
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
  expect(
    allowlist,
    'the allowlist is EMPTY and stays empty: the sanctioned channel landed as an iframe by src, which is outside the banned-construct set, so nothing here ever needed an exemption (2026-07-30, B6-8)',
  ).toEqual([])
})

// The planted probe: the scanner must be able to fail. Without this, an
// assertion that finds nothing proves nothing.
test('the scan catches a planted violation', () => {
  // Split, like every other token in this file: the fixtures must plant a
  // violation in the SCANNER's input without planting one in this SOURCE.
  // EVERY banned token gets a probe (drain r1, D13). Four of the eight were
  // unprobed before, which meant four lines of the list were assertions nobody had
  // shown could fire — and this is the packet that grew the list, so it is the
  // packet that owes the rest of the probes.
  const planted: Record<string, string> = {
    './planted1.tsx': 'export const boom = <div dangerously' + 'SetInnerHTML={{ __html: untrusted }} />',
    './planted2.ts': 'el.inner' + 'HTML = untrusted',
    './planted3.ts': 'el.outer' + 'HTML = untrusted',
    './planted4.ts': 'document.' + 'write(untrusted)',
    './planted5.ts': 'const v = eval' + '(untrusted)',
    './planted6.ts': 'const f = new ' + 'Function("return " + untrusted)',
    './planted7.ts': 'el.insert' + 'AdjacentHTML("beforeend", untrusted)',
    './planted8.tsx': 'export const frame = <iframe src' + 'doc={untrusted} />',
  }
  const hits = scan(planted)

  // One hit per planted file, and one per banned token: the scan is shown able to
  // fail on every construct it forbids, not just on the famous one.
  expect(hits).toHaveLength(banned.length)
  expect(Object.keys(planted)).toHaveLength(banned.length)
  for (const { token } of banned) {
    expect(
      hits.some((h) => h.includes(token)),
      `no planted probe exercises the ${token} ban`,
    ).toBe(true)
  }
})
