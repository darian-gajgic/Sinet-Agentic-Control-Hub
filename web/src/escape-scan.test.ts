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
const src = import.meta.glob('./**/*.{ts,tsx}', { query: '?raw', import: 'default', eager: true }) as Record<
  string,
  string
>

/**
 * The SHIPPED SCRIPTS UNDER web/public, added at B6-9.
 *
 * `sw.js` is owned source that ships verbatim and renders a notification from a
 * payload a vendor relay delivered — model- and web-derived content by any
 * reading — and it is outside the typed `src` tree, which is exactly the kind of
 * file a scan bounded to `src/` would never see. It is inside the scan now, by
 * name.
 */
const publicScripts = import.meta.glob('../public/*.js', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

const sources: Record<string, string> = { ...src, ...publicScripts }

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
  // part A, 44 at part B, 50 at B6-9) so "the scan grew over the new views" is a
  // checked fact rather than an assumption about a glob.
  expect(paths.length).toBeGreaterThan(50)
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
    // The B6-9 push surfaces. The enrolment panel renders a SERVED sentence
    // about what leaves this machine and a person's own device label; push.ts
    // handles subscription material; and the service worker renders a
    // notification composed from a payload that arrived through a vendor relay,
    // which is untrusted input by construction.
    './PushDevices.tsx',
    './push.ts',
    '../public/sw.js',
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

/**
 * THE ONE SANCTIONED RAW-HTML CHANNEL, asserted rather than asserted-in-prose
 * (S15.12 escape-by-default; B6-9 R15).
 *
 * S13.3 sanctions "the sandboxed rendered-document view", and as of RA-B1 that
 * channel has TWO instances, both inside the ONE file allowed to embed a
 * browsing context:
 *
 *  - the S15.8 preview frame — a launched app, `src` composed from a SERVED
 *    session URL plus the shell's own path, sandboxed with top-navigation and
 *    downloads withheld (the grant list below);
 *  - the RA-B1 rendered-document frame — the deliverable's own document shown
 *    as the thing it is, `src` a blob: URL minted from same-origin revision
 *    bytes, sandbox EMPTY: no scripts, no forms, no same-origin, nothing.
 *
 * Both are checked over the final tree: exactly one FILE mounts iframes, the
 * preview's withheld tokens stay withheld, and the document frame's grant
 * list stays empty — a still image of the work, never a running one.
 */
test('exactly one surface embeds a browsing context, and it is the review surface', () => {
  const withIframes = Object.entries(sources)
    .filter(([p]) => !p.includes('.test.'))
    .filter(([, text]) => text.includes('<iframe'))
    .map(([p]) => p)
    .sort()
  expect(
    withIframes,
    'a second surface embeds a browsing context; S15.12 sanctions exactly one file and it is the review surface',
  ).toEqual(['./Deliverable.tsx'])

  // And it is sandboxed with top navigation withheld — the property S13.3 and
  // review.EscapeFirst() both name (§45 D1).
  //
  // The assertion reads the GRANT LIST itself rather than the file text: that
  // file's own doc comment enumerates the withheld tokens by name, and a text
  // scan would read the documentation of an absence as the absence's opposite.
  const preview = sources['./Deliverable.tsx']
  const grant = /previewSandbox\s*=\s*'([^']*)'/.exec(preview)
  expect(grant, 'the preview sandbox grant list is gone or was renamed').not.toBeNull()
  const tokens = (grant?.[1] ?? '').split(/\s+/).filter((t) => t !== '')
  expect(tokens.length, 'the sandbox attribute is empty').toBeGreaterThan(0)
  for (const withheld of [
    'allow-top-navigation',
    'allow-top-navigation-by-user-activation',
    'allow-top-navigation-to-custom-protocols',
    'allow-popups-to-escape-sandbox',
    'allow-modals',
    'allow-downloads',
  ]) {
    expect(tokens, `the preview frame is granted ${withheld}`).not.toContain(withheld)
  }
  // The control: the grant list is real and non-empty, so "does not contain"
  // is a statement about what was granted rather than about an empty string.
  expect(tokens).toContain('allow-scripts')

  // The RA-B1 rendered-document frame: its grant list exists and is EMPTY —
  // every sandbox restriction applies. A document view that gained a token
  // would be a second preview surface wearing a quieter name, and this pin is
  // what makes that a build failure instead of a drift.
  const doc = /documentSandbox\s*=\s*'([^']*)'/.exec(preview)
  expect(doc, 'the rendered-document sandbox grant list is gone or was renamed').not.toBeNull()
  expect(doc?.[1], 'the rendered-document frame must be granted NOTHING').toBe('')

  // The MEASURING frame (polish round 2026-08-17): `allow-same-origin` and
  // nothing else. The origin grant exists so the PARENT can read the hidden
  // document's height and give the visible frame its exact size (the letterbox
  // fix); scripts staying withheld is what keeps the framed content inert — a
  // scriptless document cannot act on the origin it was handed. The exact-match
  // pin means gaining ANY further token (allow-scripts above all, which
  // together with allow-same-origin would hand model output this page's
  // origin) is a build failure, not a drift.
  const measure = /measureSandbox\s*=\s*'([^']*)'/.exec(preview)
  expect(measure, 'the measuring-frame sandbox grant list is gone or was renamed').not.toBeNull()
  expect(measure?.[1], 'the measuring frame is granted allow-same-origin and NOTHING else').toBe('allow-same-origin')
})
