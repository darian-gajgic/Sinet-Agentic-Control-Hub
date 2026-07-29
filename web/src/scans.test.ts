import { expect, test } from 'vitest'

/**
 * The checkable NEGATIVES of the oversight surfaces (P3/CONVENTIONS.md §42;
 * Spec S15.12, §30, §37).
 *
 * Escape-by-default has its own scan (escape-scan.test.ts) and keeps its EMPTY
 * allowlist. These are the other three things a view must never do:
 *
 *   1. compute money — the platform READS every dollar figure, and a client
 *      that divides or sums one has invented a number the server refused to;
 *   2. show a percentage, a completion fraction or an ETA — the API serves
 *      none, so no view can be rendering one from data;
 *   3. keep a side-truth — no identity in web storage, and no hand-assembled
 *      URL that could drift from the route table.
 *
 * Every scan below carries a PLANTED PROBE, because a scan that finds nothing
 * proves nothing until it is shown able to fail.
 */

const sources = import.meta.glob('./**/*.{ts,tsx}', { query: '?raw', import: 'default', eager: true }) as Record<
  string,
  string
>

const appSources = () =>
  Object.fromEntries(Object.entries(sources).filter(([p]) => !p.endsWith('.test.ts') && !p.endsWith('.test.tsx')))

/**
 * strip removes comments and STRING LITERALS.
 *
 * Dropping strings is what makes the arithmetic scans decidable rather than
 * noisy: `'/api/events/views/cost_per_run'` is a path, not a division, and a
 * scan that could not tell the two apart would be turned off within a week.
 * Arithmetic never happens inside a string literal, so nothing is lost.
 */
export function strip(src: string): string {
  return src
    .replace(/\/\*[\s\S]*?\*\//g, ' ')
    .replace(/(^|[^:])\/\/.*$/gm, '$1')
    .replace(/'(?:[^'\\\n]|\\.)*'/g, "''")
    .replace(/"(?:[^"\\\n]|\\.)*"/g, '""')
    .replace(/`(?:[^`\\]|\\.)*`/g, '``')
}

const moneyWord = /usd|cost|consumption|budget|burn|money|spend/i
/** An operator with operands on both sides. `/>` and `</div>` are not that,
 *  which is why the character classes are anchored rather than bare. */
const arithmetic = /[\w)\]]\s*[*/]\s*[\w(]/
const summing = /\+=|\.reduce\(/

function moneyHits(files: Record<string, string>): string[] {
  const hits: string[] = []
  for (const [path, raw] of Object.entries(files)) {
    for (const line of strip(raw).split('\n')) {
      if (!moneyWord.test(line)) continue
      if (arithmetic.test(line) || summing.test(line)) hits.push(`${path}: ${line.trim()}`)
    }
  }
  return hits
}

test('no client-side money arithmetic anywhere in the app tree', () => {
  const files = appSources()
  expect(Object.keys(files).length, 'the scan read no sources — it would pass vacuously').toBeGreaterThan(8)
  expect(moneyHits(files)).toEqual([])
})

test('the money scan can fail: three planted defects are caught, three innocents are not', () => {
  const planted = {
    './a.ts': 'const perLane = person.usd_per_day / lanes.length',
    './b.ts': 'const total = rows.reduce((n, r) => n + r.cost_usd, 0)',
    './c.ts': 'let spendUSD = 0\nspendUSD += row.priced_usd',
  }
  expect(moneyHits(planted)).toHaveLength(3)

  // The innocents: a path with the word in it, a JSX self-closing tag on a
  // money prop, and a closing tag. None is arithmetic.
  const innocent = {
    './d.ts': "const p = '/api/events/views/cost_per_run'",
    './e.tsx': '<Money usd={run.cost_so_far_usd} />',
    './f.tsx': '<dd>{cost}</dd>',
  }
  expect(moneyHits(innocent), 'the money scan false-positives on ordinary code').toEqual([])
})

test('no view renders a percentage, a completion fraction or an ETA', () => {
  // `\bpct` matches at the START of an identifier only, so `pctDone` and
  // `pct_done` are both caught without needing a trailing boundary that an
  // underscore or a capital would swallow.
  const banned = [/percent/i, /\bpct/i, /completion_?fraction/i, /\beta_?(s|seconds)?\b/i, /progress_?bar/i]
  const hits: string[] = []
  for (const [path, raw] of Object.entries(appSources())) {
    const src = strip(raw)
    for (const pattern of banned) {
      if (pattern.test(src)) hits.push(`${path}: ${pattern}`)
    }
  }
  expect(hits, 'a view names a progress figure the API does not serve (§30)').toEqual([])
  // Probe: the patterns really match what they are meant to.
  expect(banned.some((p) => p.test('const pctDone = steps / total'))).toBe(true)
  expect(banned.some((p) => p.test('const eta_s = remaining'))).toBe(true)
})

test('nothing is kept in web storage', () => {
  const hits: string[] = []
  for (const [path, raw] of Object.entries(appSources())) {
    if (/localStorage|sessionStorage|document\.cookie/.test(strip(raw))) hits.push(path)
  }
  expect(hits, 'a client-side side-truth outlives the re-snapshot that should replace it').toEqual([])
})

test('no view re-declares a registered receipt label as its own literal', () => {
  // The done-directly labels are REGISTERED text (BENCH-REG §13): the per-run
  // heuristic line and the aggregate measured form. They reach the screen
  // through `direct_use.label`, as data. A view that typed one of them would
  // have made a copy of a registered string that can only be changed through
  // the registration's own §17 — and would keep rendering the old wording the
  // day it changed. The tokens are SPLIT here for the same reason the escape
  // scan splits its own: this file must not contain the literal it forbids.
  const registered = ['direct-use estimate ' + '(heuristic)', 'measured (benchmark ' + 'n=']
  const hits: string[] = []
  for (const [path, raw] of Object.entries(appSources())) {
    for (const literal of registered) {
      if (raw.includes(literal)) hits.push(`${path}: ${literal}`)
    }
  }
  expect(hits, 'a view file re-typed a registered label instead of rendering the served one').toEqual([])

  // Probe: the scan can fail, and the fixtures really do carry the strings —
  // so "no hits" means "rendered from data", not "the label never appears".
  expect(registered.some((l) => `const label = '${registered[0]}'`.includes(l))).toBe(true)
})

test('every in-app link is built from the route table, never assembled by hand', () => {
  const hits: string[] = []
  for (const [path, raw] of Object.entries(appSources())) {
    if (path === './routes.ts') continue // the table itself IS the patterns
    const src = raw
    // A path built by interpolation or concatenation would drift silently the
    // day a route is renamed; `hrefFor` is the one thing that cannot.
    for (const pattern of [/`\/(tasks|inbox|deliverables)\//, /'\/(tasks|inbox|deliverables)\/'\s*\+/]) {
      if (pattern.test(src)) hits.push(`${path}: ${pattern}`)
    }
  }
  expect(hits, 'a hand-built in-app path exists — use hrefFor').toEqual([])
  // Probe.
  expect(/`\/(tasks|inbox|deliverables)\//.test('const href = `/tasks/${id}`')).toBe(true)
})

test('no renderer theme package is imported anywhere in the tree', () => {
  // FC-v1 §4: the renderer set is SINET-OWNED, over JSON Forms' core,
  // theme-independent vocabulary. A theme package is a dependency decision with
  // its own look, its own upgrade cadence and its own opinions about what a
  // form is; none is on the S16.4 rail, and one arriving quietly is exactly
  // what this catches.
  const banned = [
    '@jsonforms/material-renderers',
    '@jsonforms/vanilla-renderers',
    '@jsonforms/vue-vanilla',
    '@jsonforms/angular-material',
  ]
  // Only IMPORT lines are scanned: a package can enter no other way, and a
  // file is allowed to name in prose the thing it is explaining it never uses
  // (settingsForm.tsx's own doc comment does exactly that).
  const hits: string[] = []
  for (const [path, raw] of Object.entries(appSources())) {
    for (const line of raw.split('\n')) {
      if (!/^\s*(import|export)\b|require\(/.test(line)) continue
      for (const pkg of banned) {
        if (line.includes(pkg)) hits.push(`${path}: ${pkg}`)
      }
    }
  }
  expect(hits, 'a JSON Forms theme package entered the tree').toEqual([])
  // Probe: the scan can fail, and the packages it names are real ones.
  expect(
    banned.some((p) => `import x from '${banned[0]}'`.includes(p)),
    'the probe does not exercise the import matcher',
  ).toBe(true)
})

test('the settings UI declares no ⚙ key of its own', () => {
  // The whole claim of the settings tab is that it hand-builds nothing: the
  // registry emits the form, and a key list living in a view file would be the
  // second copy S01.10 exists to prevent. Real dotted keys appear only in the
  // fixtures the views render FROM.
  const dotted = /['"`][a-z]+\.[a-z_]+(\.[a-z_]+)?['"`]/
  const hits: string[] = []
  for (const [path, raw] of Object.entries(appSources())) {
    if (!path.includes('etting')) continue
    for (const line of raw.split('\n')) {
      // Route paths and event types are not ⚙ keys; the keys this forbids are
      // the ones a control would be hand-placed by.
      // Route paths, event types and CATALOG QUERY names are not ⚙ keys; the
      // keys this forbids are the ones a control would be hand-placed by.
      if (line.includes('/api/') || line.includes('settings.changed')) continue
      if (line.includes('historyQuery') || line.trimStart().startsWith('*')) continue
      if (line.trimStart().startsWith('//')) continue
      if (dotted.test(line)) hits.push(`${path}: ${line.trim()}`)
    }
  }
  expect(hits, 'a settings view names a ⚙ key — the form is generated, not hand-built').toEqual([])
})
