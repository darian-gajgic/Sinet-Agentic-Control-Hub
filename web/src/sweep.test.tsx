import { describe, expect, test } from 'vitest'

import { routes, type RouteID } from './routes'
import routeInventoryRaw from './fixtures/api/route-inventory.json?raw'

/**
 * The S15.12 cross-cutting sweep (B6-9 part B).
 *
 * S15.12 states four duties over the WHOLE workspace rather than over any one
 * surface — live by default, multi-user aware, one responsive workspace, escape
 * by default — and S19.5 adds a fifth: "the React SPA consumes every API built
 * above". Each is checked here across the finished tree, mechanically where a
 * mechanism can carry it and as a dated checklist-of-record in CONVENTIONS §46
 * where a judgement cannot (B6-9 OQ11).
 *
 * WHAT THE MECHANISMS ARE AND WHY THEY CANNOT BE TAUTOLOGICAL. The route
 * coverage below compares two INDEPENDENTLY PRODUCED lists: the server's is
 * written by its own registration calls (internal/api's Handler records each
 * pattern as it registers it) and the client's is extracted from what the client
 * SOURCES actually CALL — naming a path is not calling it, in any file (drain
 * r2, R1). An inventory compared against itself proves nothing —
 * B6-8 shipped one and it hid a live defect — so neither side here is allowed to
 * be a hand-kept list.
 */

const sources = import.meta.glob('./**/*.{ts,tsx}', { query: '?raw', import: 'default', eager: true }) as Record<
  string,
  string
>

/** The shell stylesheet, read raw. `test.css: true` is what makes this the real
 *  file rather than the empty string vitest stubs CSS to by default (§41-B). */
const shellStylesheet = (
  import.meta.glob('./index.css', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
)['./index.css']

/** The shipped service worker, which is owned source outside the typed tree and
 *  is a real client of the API (it re-enrols on a subscription rotation). */
const publicScripts = import.meta.glob('../public/*.js', { query: '?raw', import: 'default', eager: true }) as Record<
  string,
  string
>

/** appSources is the shipped APP: no test files, no test harness. What a test
 *  names is not what the product calls. */
const appSources = (): Record<string, string> =>
  Object.fromEntries([
    ...Object.entries(sources).filter(
      ([p]) =>
        !p.includes('.test.') &&
        !p.endsWith('/doubles.ts') &&
        !p.endsWith('/testing.tsx') &&
        // The vitest setup file is harness too: it ships in no build, and a
        // route named there would be a test naming it (drain r1, D14).
        !p.endsWith('/test-setup.ts'),
    ),
    ...Object.entries(publicScripts),
  ])

/** stripComments removes prose before matching. A file is allowed to EXPLAIN a
 *  route it does not call — api.ts's own doc comments name several — and a scan
 *  that counted those would report coverage that does not exist (the §43 ⚙-key
 *  scan's own rule). */
function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1')
}

/**
 * normalizePath reduces a path to its SHAPE, so the two languages' spellings
 * meet: Go writes `/api/tasks/{task}`, the client writes
 * `/api/tasks/${encodeURIComponent(id)}`, and both mean one task.
 *
 * PARAM is a TEXT sentinel, not a control character. The first cut used literal
 * NUL bytes, which made this file `data` to git and to every diff tool — so the
 * packet's headline check was unreviewable, now and for every future change to
 * it (drain r1, D11). An invisible separator is outside every path alphabet and
 * survives a diff.
 */
const PARAM = '⁣PARAM⁣'

export function normalizePath(path: string): string {
  const base = path.split('?')[0]
  // An interpolation that fills a whole SEGMENT is a parameter; one appended to
  // a segment (`/api/tasks${query(f)}`) is a query builder and is dropped.
  const marked = stripInterpolations(base)
  const withParams = marked.split('/' + PARAM).join('/{}').split(PARAM).join('')
  return withParams.replace(/\{[A-Za-z_][A-Za-z0-9_]*\}/g, '{}').replace(/\/+$/, '') || '/'
}

/**
 * stripInterpolations replaces every `${…}` with PARAM, matching BRACES.
 *
 * A `\$\{[^}]*\}` regex ends the interpolation at the FIRST `}`, so a nested
 * object literal breaks it: `api.comments`'s
 * `/api/deliverables/${…}/comments${query({ revision })}` normalized to
 * `/api/deliverables/{}/comments)}` — a shape matching no route the server
 * serves. Found at drain r2 R1, and only findable then: the route looked
 * consumed because the `useLive` KEY beside it spelled the shape correctly by
 * hand, and R1 stopped counting keys. One check passing for another check's
 * reason is the shape of every finding in this section.
 */
function stripInterpolations(text: string): string {
  let out = ''
  for (let i = 0; i < text.length; i++) {
    if (text[i] === '$' && text[i + 1] === '{') {
      let depth = 0
      let j = i + 1
      for (; j < text.length; j++) {
        if (text[j] === '{') depth++
        else if (text[j] === '}' && --depth === 0) break
      }
      out += PARAM
      i = j
      continue
    }
    out += text[i]
  }
  return out
}

/** A consumed route is a METHOD and a shape. Comparing shapes alone let a new
 *  MUTATING verb land at a path some GET already consumed, with no caller at
 *  all, and stay invisible (drain r1, D3). */
function key(method: string, path: string): string {
  return `${method.toUpperCase()} ${normalizePath(path)}`
}

const pathLiteral = /[`'"](\/(?:api|events)[A-Za-z0-9_\-/${}().]*)/

/**
 * consumedRoutes is every (method, shape) the shipped app actually CALLS.
 *
 * THREE CORRECTIONS THIS FUNCTION EXISTS FOR, each found by an independent pass:
 *
 *  - a path literal is not a method, so a POST landing where a GET already
 *    lived was invisible (drain r1, D3);
 *  - a literal inside `api.ts`'s client object is a DECLARATION, not
 *    consumption. `api.health` is declared and called by nothing, and the old
 *    scan counted it consumed because the string was present (drain r1, D5);
 *    and
 *  - THAT FIX REACHED `api.ts` ONLY (drain r2, R1). Every other file still had
 *    presence meaning consumption: a never-called `const EVAL_DEAD =
 *    '/api/workforce'` in any app file re-hid a real gap, which was reproduced.
 *    A completeness check that UNDER-reports is worse than none, because it
 *    will be believed at a gate.
 *
 * SO CONSUMPTION IS A CALL IN EVERY FILE, BY TWO DIFFERENT UNITS.
 *
 * `api.ts` is a map of member → routes, and a member's routes count only where
 * some other app file names `api.<member>(` — the callable unit there is the
 * MEMBER, because that is what a caller names. Everywhere else the callable
 * unit is the REQUEST, so each literal has to be shown reaching one
 * (`classifyFile`), and a literal that reaches nothing is REPORTED rather than
 * assumed to be either answer.
 */
function consumedRoutes(): Set<string> {
  const app = appSources()
  const out = new Set<string>()

  const apiSrc = stripComments(app['./api.ts'] ?? '')
  const others = Object.entries(app).filter(([p]) => p !== './api.ts')
  for (const [name, routes] of Object.entries(apiMembers(apiSrc))) {
    const called = others.some(([, t]) => new RegExp(`\\bapi\\s*\\.\\s*${name}\\s*\\(`).test(stripComments(t)))
    if (!called) continue
    for (const r of routes) out.add(r)
  }
  // An exported HELPER is a callable unit too. `objectHref` composes the
  // object-bytes URL an <img src> and a download link read — a real
  // consumption that no `request<>` call names.
  for (const [name, routes] of Object.entries(apiHelpers(apiSrc))) {
    const called = others.some(([, t]) => new RegExp(`\\b${name}\\s*\\(`).test(stripComments(t)))
    if (!called) continue
    for (const r of routes) out.add(r)
  }
  for (const [, text] of others) {
    for (const r of classifyFile(text).consumed) out.add(r)
  }
  return out
}

/** unresolvedLiterals is the loud half of the R1 fix: every path literal in a
 *  non-`api.ts` app file that this scan cannot show reaching a request and
 *  cannot explain as a subscription key. It is neither counted nor ignored. */
function unresolvedLiterals(): string[] {
  return Object.entries(appSources())
    .filter(([p]) => p !== './api.ts')
    .flatMap(([p, t]) => classifyFile(t).unresolved.map((lit) => `${p}: ${lit}`))
    .sort()
}

/** byFileClients names every file whose request call takes an expression this
 *  scan cannot resolve to a literal, so its own path literals are counted
 *  wholesale. It is asserted to be an exact set, because that is the one place
 *  presence still implies consumption. */
function byFileClients(): string[] {
  return Object.entries(appSources())
    .filter(([p, t]) => p !== './api.ts' && classifyFile(t).byFile)
    .map(([p]) => p)
    .sort()
}

/** declaredButUncalled names every `api` member nothing calls — the hole D5
 *  found, reported rather than silently absorbed. */
function declaredButUncalled(): string[] {
  const app = appSources()
  const members = apiMembers(stripComments(app['./api.ts'] ?? ''))
  const others = Object.entries(app).filter(([p]) => p !== './api.ts')
  return Object.keys(members)
    .filter((name) => !others.some(([, t]) => new RegExp(`\\bapi\\s*\\.\\s*${name}\\s*\\(`).test(stripComments(t))))
    .sort()
}

/** apiHelpers maps each exported helper in api.ts to the routes it composes. */
function apiHelpers(src: string): Record<string, string[]> {
  const out: Record<string, string[]> = {}
  const decls = [...src.matchAll(/\nexport function ([a-zA-Z][a-zA-Z0-9]*)\(/g)]
  for (let i = 0; i < decls.length; i++) {
    const from = decls[i].index ?? 0
    const to = i + 1 < decls.length ? (decls[i + 1].index ?? src.length) : src.length
    const routes = routesNamedIn(src.slice(from, to))
    if (routes.length > 0) out[decls[i][1]] = routes
  }
  return out
}

/** apiMembers maps each `api` object member to the routes its body performs. */
function apiMembers(src: string): Record<string, string[]> {
  const start = src.indexOf('export const api = {')
  const out: Record<string, string[]> = {}
  if (start < 0) return out
  const body = src.slice(start)
  const starts = [...body.matchAll(/\n {2}([a-zA-Z][a-zA-Z0-9]*):\s/g)]
  for (let i = 0; i < starts.length; i++) {
    const from = starts[i].index ?? 0
    const to = i + 1 < starts.length ? (starts[i + 1].index ?? body.length) : body.length
    const routes = routesNamedIn(body.slice(from, to))
    if (routes.length > 0) out[starts[i][1]] = routes
  }
  return out
}

/**
 * classifyFile answers, for ONE non-`api.ts` app file, which of its path
 * literals reach a request — and says so about every literal rather than
 * assuming (drain r2, R1).
 *
 * Four resolutions, each with its reason, and everything else is reported:
 *
 *  1. THE TYPED CLIENT CALLS — `post<T>('/x')` / `request<T>('/x')`. A call
 *     with the literal in its own argument list.
 *  2. A DIRECT REQUEST — `fetch(…)` or `new EventSource(…)`, with the argument
 *     either the literal itself or a same-file `const NAME = '/x'`. This is how
 *     the service worker names its re-enrolment POST.
 *  3. A `useLive` KEY — NOT a request. `useLive` uses `key` only as an effect
 *     DEPENDENCY (live.ts's deps array; measured at B6-8 §45 D5, where a
 *     blanked key disabled nothing), so the request beside it is the `read`
 *     closure, which names `api.<member>(` and is counted through the member
 *     map. A key resolves either inside the `useLive(…)` call or as a const
 *     whose name is passed into one.
 *  4. BY FILE — a file whose request call takes an expression that does not
 *     statically resolve. `events.ts` opens the stream through an injectable
 *     seam (`this.make(this.url(…))`, defaulting to `new EventSource(url)`), so
 *     the literal and the constructor are one hop apart. Such a file's literals
 *     are counted only where they are NAMED in `BY_FILE_LITERALS`, and the set
 *     of such files is asserted to be exactly one. Counting them wholesale is
 *     what the post-cap fix removed: it was the last place where presence still
 *     implied consumption, so a dead string dropped into `events.ts` re-hid a
 *     real gap exactly as it had everywhere else before drain r2.
 *
 * Anything else is UNRESOLVED: a literal nothing calls. That is what a dead
 * declaration looks like, and it is exactly what used to be counted as a GET.
 */
/**
 * The literals a BY-FILE client is allowed to have counted for it. Naming them
 * is what keeps the exemption from being the thing it exempts: the one-file pin
 * says WHICH file may resolve by expression, and this says WHICH strings it may
 * resolve — so a dead literal dropped into that file is unresolved and loud,
 * like a dead literal anywhere else. Both are `events.ts`'s own SSE route.
 */
const BY_FILE_LITERALS = new Set(['/events', '/events?${qs}'])

function classifyFile(raw: string): {
  consumed: string[]
  keys: string[]
  unresolved: string[]
  byFile: boolean
} {
  const src = stripComments(raw)
  const consumed: string[] = []
  const keys: string[] = []
  const unresolved: string[] = []

  // Resolution 1, which claims the literals inside its own calls.
  const claimed = new Set<number>()
  consumed.push(...routesInCalls(src, claimed))

  // A const's name, and where its literal sits.
  const consts = new Map<string, { path: string; at: number }>()
  for (const m of src.matchAll(/\bconst\s+([A-Za-z_$][\w$]*)\s*=\s*(['"`])(\/(?:api|events)[^'"`]*)/g)) {
    consts.set(m[1], { path: m[3], at: (m.index ?? 0) + m[0].indexOf(m[2]) })
  }

  const liveSpans = callSpans(src, /\buseLive\s*(?:<[^<>]*>)?\s*\(/g)
  const requestSpans = callSpans(src, /\b(?:fetch|EventSource)\s*\(/g)
  const inside = (spans: { from: number; to: number }[], at: number): { from: number; to: number } | undefined =>
    spans.find((s) => at > s.from && at < s.to)

  // Resolutions 2 and 3, const legs: a const is resolved by where its NAME is
  // USED, never by where it is written — which is the whole point. A
  // declaration nobody uses resolves to nothing and falls through to the
  // unresolved report below.
  for (const [ident, { path, at }] of consts) {
    if (claimed.has(at)) continue
    const uses = [...src.matchAll(new RegExp(`\\b${ident}\\b`, 'g'))].map((u) => u.index ?? 0)
    if (uses.some((u) => inside(liveSpans, u))) {
      claimed.add(at)
      keys.push(path)
      continue
    }
    const req = uses.map((u) => inside(requestSpans, u)).find(Boolean)
    if (req) {
      claimed.add(at)
      const method = /method:\s*['"`]([A-Z]+)/.exec(src.slice(req.from, req.to))
      consumed.push(key(method ? method[1] : 'GET', path))
    }
  }

  // Whether this file opens a request through an expression: a request call
  // whose argument is neither a literal nor a resolvable const.
  const byFile = requestSpans.some((s) => {
    const arg = src.slice(s.from + 1, s.to).split(',')[0].trim()
    return !/^['"`]/.test(arg) && !consts.has(arg)
  })

  for (const m of src.matchAll(/['"`](\/(?:api|events)[A-Za-z0-9_\-/${}().]*)/g)) {
    const at = m.index ?? -1
    if (claimed.has(at)) continue
    if (inside(liveSpans, at)) {
      keys.push(m[1])
      continue
    }
    if (byFile && BY_FILE_LITERALS.has(m[1])) {
      consumed.push(key('GET', m[1]))
      continue
    }
    unresolved.push(m[1])
  }
  return { consumed, keys, unresolved, byFile }
}

/**
 * callSpans finds each call to a named function and returns the source range of
 * its argument list, parentheses BALANCED — which a regex cannot do, and which
 * every "is this literal inside that call?" question above needs. Quoted
 * sections are skipped so a parenthesis inside a string cannot unbalance it.
 */
function callSpans(src: string, opener: RegExp): { from: number; to: number }[] {
  const out: { from: number; to: number }[] = []
  for (const m of src.matchAll(opener)) {
    const from = (m.index ?? 0) + m[0].length - 1
    let depth = 0
    let quote = ''
    for (let i = from; i < src.length; i++) {
      const c = src[i]
      if (quote) {
        if (c === '\\') i++
        else if (c === quote) quote = ''
        continue
      }
      if (c === "'" || c === '"' || c === '`') quote = c
      else if (c === '(') depth++
      else if (c === ')' && --depth === 0) {
        out.push({ from, to: i })
        break
      }
    }
  }
  return out
}

/**
 * routesInCalls extracts the (method, shape) of every literal sitting in one of
 * the TYPED CLIENT CALLS, and records the index of each literal it claimed.
 *
 * `post<T>('/x')` is a POST; `request<T>('/x')` is a GET unless the same call
 * carries a `method:`.
 */
function routesInCalls(text: string, claimed: Set<number>): string[] {
  const out: string[] = []
  for (const m of text.matchAll(/\bpost<[^>]*>\(\s*(['"`][^'"`]+)/g)) {
    const p = pathLiteral.exec(m[1])
    if (!p) continue
    out.push(key('POST', p[1]))
    claimed.add((m.index ?? 0) + m[0].indexOf(m[1]))
  }
  for (const m of text.matchAll(/\brequest<[^>]*>\(\s*(['"`][^'"`]+)([\s\S]{0,240})/g)) {
    const p = pathLiteral.exec(m[1])
    if (!p) continue
    const method = /method:\s*['"`]([A-Z]+)/.exec(m[2])
    out.push(key(method ? method[1] : 'GET', p[1]))
    claimed.add((m.index ?? 0) + m[0].indexOf(m[1]))
  }
  return out
}

/**
 * routesNamedIn is every route ONE `api.ts` member or exported helper names:
 * the typed calls above, plus its BARE literals.
 *
 * A bare literal counts HERE and nowhere else, and the reason is that the
 * callable unit differs. In `api.ts` the unit is the member — `objectHref`
 * composes a URL an `<img src>` reads and no `request<>` call names it, and the
 * member is only counted at all where an app file writes `api.<member>(`
 * (drain r1, D5). In every other file the unit is the request itself, so
 * `classifyFile` requires each literal to reach one (drain r2, R1).
 */
function routesNamedIn(text: string): string[] {
  const claimed = new Set<number>()
  const out = routesInCalls(text, claimed)
  for (const m of text.matchAll(/['"`](\/(?:api|events)[A-Za-z0-9_\-/${}().]*)/g)) {
    if (claimed.has(m.index ?? -1)) continue
    out.push(key('GET', m[1]))
  }
  return out
}

type InventoryRoute = { method: string; path: string; session_required: boolean }
const inventory = (JSON.parse(routeInventoryRaw) as { routes: InventoryRoute[] }).routes

/**
 * THE SANCTIONED EXCEPTION LIST (S19.5; B6-9 R17).
 *
 * Every route the SPA does not call, with the reason. Nothing is here to make a
 * number go green: each entry is either a server surface the spec never asked
 * this client to render, or a REPORTED GAP carried to the B6 gate — and the two
 * are labelled, because they are different facts.
 *
 * `gap: true` means the spec names a capability whose route exists and whose
 * surface does not. Those are findings, not exemptions.
 */
const exceptions: { method: string; path: string; why: string; gap?: boolean }[] = [
  // ── Server surfaces the SPA was never asked to render ──────────────────
  {
    method: 'GET',
    path: '/api/health',
    why: 'the readiness surface. It answers 200/503 for the front chain, for systemd and for an operator with curl — not for a workspace. The SPA\u2019s own connection truth is the session probe plus the SSE state (\u00a741-B), which is why nothing calls it. NOTE: `api.health` is a DECLARED-BUT-UNCALLED client member, and it is exactly what made the old shape-only scan report this route as consumed (drain r1, D5).',
  },
  {
    method: 'POST',
    path: '/api/auth/verify-pin',
    why: 'the S01.9 step-up is verify-AT-ACT: the PIN rides the same request as the answer or the accept, so no surface calls a standalone verify (§9).',
  },
  {
    method: 'POST',
    path: '/api/auth/pin',
    why: 'setting a PIN is an operator ceremony at bootstrap, not a workspace surface; the login page uses it only through POST /api/auth/users.',
  },
  {
    method: 'GET',
    path: '/api/auth/grants',
    why: 'device grants are the operator granting trusted auto-login to a personal device (S01.9 layer 2) — an administration act with no ratified UI at v0. The login page consumes the HINT, which is the half a person meets.',
  },
  {
    method: 'POST',
    path: '/api/auth/grants',
    why: 'creating a device grant — the operator trusting one personal device with auto-login (S01.9 layer 2). Administration with no ratified UI at v0; the login page consumes the HINT, which is the half a person actually meets.',
  },
  {
    method: 'POST',
    path: '/api/auth/grants/revoke',
    why: 'the other half of the same act: revoking a device grant is administration, and it has no ratified UI at v0 either.',
  },
  {
    method: 'POST',
    path: '/api/intake/requests',
    why: 'the B2-4 walking-skeleton submit. The SPA starts work through the S15.7 assistant’s `task` verb, which reaches intake server-side (§44) — one ingress, not two.',
  },
  {
    method: 'POST',
    path: '/api/tasks/{}/advance',
    why: 'the B2-4 walking-skeleton stage advance. Stage is FSM state owned by the control plane (S15.5: “stage columns are never writable”), so no surface may drive it.',
  },
  {
    method: 'POST',
    path: '/api/asks/{}/answer',
    why: 'the B2-4 walking-skeleton answer, superseded for every surface by the hash-pinned POST /api/approvals/{id}/answer the inbox and the assistant both use.',
  },
  {
    method: 'GET',
    path: '/api/runs/{}/receipt',
    why: 'the receipt is SERVED INSIDE GET /api/tasks/{task} (runs[].receipt), which is where the task detail renders it. The standalone route is the same data at a second grain and needs no second consumer.',
  },
  {
    method: 'GET',
    path: '/api/events/open-sql',
    why: 'Layer 2 is reached by the assistant’s escalation control, which posts a chat TURN — the server calls this layer, and the guardrail’s audit row is what makes that the right place for it (§44). A client naming it directly would bypass the turn record.',
  },
  // ── REPORTED GAPS: a route exists, a surface does not ───────────────────
  {
    method: 'POST',
    path: '/api/memory',
    gap: true,
    why: 'REPORTED GAP \u2014 CREATING a knowledge entry, which is the memory family\u2019s own write verb and its most consequential one: it is how a person tells the platform something it should keep. It is named separately here because counting by SHAPE hid it behind the GET at the same path (drain r1, D6).',
  },
  {
    method: 'GET',
    path: '/api/memory',
    gap: true,
    why: 'REPORTED GAP — the S15.2 `memory` family has NO SPA surface. S15’s own surface list (S15.5–S15.11) names no memory browser, so this is not an omission inside a built view; it is a family row in the S15.2 table whose only consumed edge is the ninth inbox kind’s resolve verb. Carried to the B6 gate.',
  },
  { method: 'GET',
    path: '/api/memory/{}', gap: true, why: 'REPORTED GAP — the same surface-less family: one knowledge entry with its provenance and gate status.' },
  { method: 'POST',
    path: '/api/memory/{}/new-version', gap: true, why: 'REPORTED GAP — the same surface-less family: the S09.4 station-3 gated edit, which is where a person would revise what the platform knows.' },
  { method: 'POST',
    path: '/api/memory/{}/remove', gap: true, why: 'REPORTED GAP — the same surface-less family: retiring an entry, which is a real act with no control anywhere in the workspace.' },
  { method: 'POST',
    path: '/api/memory/{}/delete', gap: true, why: 'REPORTED GAP — the same surface-less family: the owner’s hard delete of their own entry.' },
  // CLOSED 2026-08-05 (P3-UI-2), four entries removed from this list rather
  // than annotated: feature 4.5's two cancel verbs are the task detail's
  // state-computed action bar and per-run control, the S10.4 budget editor and
  // the 3.3 pause switch are the fleet's. The stale-entry test below is what
  // forced their removal the moment a surface called them, which is the whole
  // reason it exists — an exception list that keeps excusing a consumed route
  // is a list nobody can trust in the other direction either.
  {
    method: 'POST',
    path: '/api/deliverables/{}/follow-up',
    gap: true,
    why: 'REPORTED GAP — the S13.9 follow-up spawn. The review surface SERVES and RENDERS this door with its route and its `revision` preset, and NOTHING in the SPA calls it. CORRECTED (drain r1, D4): the first characterization said "shown, not pressable", which understated it — the surface shipped a full form with an ENABLED submit button whose handler could only report "The door named no route, pin or answer", because the form was gated on the door\u2019s VERB rather than on what it supplies. The dead control is fixed; the missing capability is what remains, and it is this gap.',
  },
  {
    method: 'POST',
    path: '/api/benchmark/opt-in',
    gap: true,
    why: 'REPORTED GAP — the BENCH-REG §4.2.1 consent flip. The verdict form renders for a participant; nothing offers the opt-in that makes somebody one.',
  },
  {
    method: 'GET',
    path: '/api/events/ask',
    gap: true,
    why: 'REPORTED GAP — the S14.10 intent-filling ask layer. The assistant routes by GESTURE (a verb radio) rather than by intent, deliberately (§44 OQ3: no model classifies a turn), so this layer has no caller today.',
  },
  {
    method: 'GET',
    path: '/api/events/search',
    gap: true,
    why: 'REPORTED GAP — the S14.10 redact-before-match search over the history. It is built, bounded and audited server-side, and no surface offers a search box to reach it.',
  },
]

describe('the SPA consumes every API built above (S19.5)', () => {
  const consumed = consumedRoutes()
  const excused = new Map(exceptions.map((e) => [`${e.method} ${e.path}`, e]))

  test('the two lists are both real', () => {
    // Floors on BOTH sides. A generated inventory that came back empty, or a
    // client scan that matched nothing, would make every assertion below pass
    // while proving nothing.
    expect(inventory.length).toBeGreaterThan(60)
    expect(consumed.size).toBeGreaterThan(40)
    // And the scan really is reading the app rather than the tests: a path only
    // a TEST names must not appear.
    expect(consumed.has('GET /api/definitely-not-a-route')).toBe(false)
    expect(consumed.has('GET /api/approvals')).toBe(true)
    // The METHOD really is carried (D3): the approvals family is read AND
    // written at shapes that differ only by verb.
    expect(consumed.has('POST /api/approvals/answer-batch')).toBe(true)
    expect(consumed.has('POST /api/approvals')).toBe(false)
  })

  test('every registered route is consumed or named on the exception list', () => {
    const unexplained = inventory
      .map((r) => ({ ...r, k: key(r.method, r.path) }))
      .filter((r) => !consumed.has(r.k) && !excused.has(r.k))
      .map((r) => `${r.method} ${r.path}`)
    expect(
      unexplained,
      'these routes are served and nothing in the SPA names them. Consume them or name them on the exception list WITH A REASON — an unexplained route is either a dead surface or a missing one.',
    ).toEqual([])
  })

  test('and the exception list carries no entry that is actually consumed', () => {
    // The other direction, which is what stops the list rotting into a place
    // where things go to be forgotten.
    const stale = exceptions.filter((e) => consumed.has(`${e.method} ${e.path}`)).map((e) => `${e.method} ${e.path}`)
    expect(stale, 'these are on the exception list AND consumed; remove them from the list').toEqual([])
    // Every exception must name a route that exists, too.
    const served = new Set(inventory.map((r) => key(r.method, r.path)))
    const phantom = exceptions
      .filter((e) => !served.has(`${e.method} ${e.path}`))
      .map((e) => `${e.method} ${e.path}`)
    expect(phantom, 'these exceptions excuse routes the server does not serve').toEqual([])
    // A reason is required and cannot be a placeholder.
    for (const e of exceptions) expect(e.why.length, `${e.path} has no real reason`).toBeGreaterThan(40)
  })

  test('the gaps are LABELLED as gaps rather than hidden among the exemptions', () => {
    const gaps = exceptions.filter((e) => e.gap).map((e) => e.path)
    // Recorded as a count so a later packet closing one has to move this line
    // deliberately rather than letting the list quietly shrink or grow.
    // TEN REGISTERED ROUTES over NINE distinct shapes: `/api/memory` is a gap
    // at both its verbs, and the POST — creating a knowledge entry — is the
    // family's most consequential one. Counting by shape alone hid it (drain
    // r1, D6), so all three numbers are pinned rather than one.
    //
    // MOVED 2026-08-05 (P3-UI-2), 14 → 10 routes over 13 → 9 shapes: the two
    // 4.5 cancel verbs, the S10.4 budget editor and the 3.3 pause switch are
    // consumed by the task detail and the fleet. Four routes, four shapes —
    // none of them shared a shape with anything else on the list. CONVENTIONS
    // §46's gap record carries the dated pointer (§48).
    expect(gaps.length, 'the reported-gap ROUTE count moved; update CONVENTIONS §46 with it').toBe(10)
    expect(new Set(gaps).size, 'the reported-gap SHAPE count moved').toBe(9)
    const gapRoutes = inventory.filter((r) => gaps.includes(normalizePath(r.path)))
    expect(gapRoutes.length, 'a reported gap names a route the server does not serve').toBe(10)
    for (const e of exceptions.filter((x) => x.gap)) {
      expect(e.why, `${e.path} is flagged a gap but does not say so in its reason`).toContain('REPORTED GAP')
    }
  })

  test('a path literal nothing calls is not consumption, in ANY file (drain r2, R1)', () => {
    // THE DEFECT THIS REPLACES. D5 made consumption mean a CALL — but only for
    // `api.ts`. Everywhere else presence still meant consumption, so a single
    // never-called `const EVAL_DEAD = '/api/workforce'` in any app file put a
    // real gap back out of sight. Reproduced before this test existed: with the
    // genuine consumption removed from both real sites the sweep fired, and one
    // dead line returned it to green.
    const dead = `const EVAL_DEAD = '/api/workforce'\nexport function Fleet() { return null }\n`
    const got = classifyFile(dead)
    expect(got.consumed, 'a dead declaration was counted as a call').toEqual([])
    expect(got.unresolved, 'a literal reaching no request must be REPORTED, not assumed either way').toEqual([
      '/api/workforce',
    ])
    // And the same literal in a real call still counts, so the rule is "a call"
    // rather than "no bare literals ever".
    const called = `const P = '/api/workforce'\nawait fetch(P, { method: 'POST' })\n`
    expect(classifyFile(called).consumed).toEqual(['POST /api/workforce'])
    expect(classifyFile(called).unresolved).toEqual([])
    // A `useLive` key is EXPLAINED rather than counted: live.ts uses `key` only
    // as an effect dependency (§45 D5 measured it), so the request beside it is
    // the `read` closure and that is what the member map counts.
    const live = `useLive({ key: '/api/workforce', read: () => api.workforce() })`
    expect(classifyFile(live).consumed).toEqual([])
    expect(classifyFile(live).keys).toEqual(['/api/workforce'])
    expect(classifyFile(live).unresolved).toEqual([])
  })

  test('a nested-brace interpolation normalizes to the shape the server serves', () => {
    // The second finding R1 turned up. `${query({ revision })}` ends at its
    // INNER brace under a `[^}]*` rule, gluing `)}` onto the shape — so this
    // member's GET normalized to a route the server does not serve, and the
    // check stayed green only because the `useLive` key beside it spelled the
    // shape correctly by hand.
    expect(normalizePath('/api/deliverables/${encodeURIComponent(id)}/comments${query({ revision })}')).toBe(
      '/api/deliverables/{}/comments',
    )
    expect(consumed.has('GET /api/deliverables/{}/comments'), 'the comments read is consumed by api.comments').toBe(
      true,
    )
  })

  test('every path literal in the shipped app is EXPLAINED, and the one by-file client is named', () => {
    // The loud half. A literal this scan cannot show reaching a request and
    // cannot explain as a subscription key fails HERE, naming its file — rather
    // than silently counting as a GET, which is what let a dead string mask a
    // gap.
    expect(
      unresolvedLiterals(),
      'these path literals reach no request the scan can see. Either they are dead (delete them) or the scan cannot follow the call (teach it, do not let it assume).',
    ).toEqual([])
    // The ONE place presence still implies consumption, named rather than
    // diffuse: events.ts opens the stream through an injectable seam
    // (`this.make(this.url(…))`, defaulting to `new EventSource(url)`), so the
    // literal and the constructor are one hop apart.
    expect(byFileClients(), 'a second file now opens a request through an expression; teach the scan or name it').toEqual([
      './events.ts',
    ])
    expect(consumed.has('GET /events'), 'the SSE stream is a consumed route and must stay counted').toBe(true)
  })

  test('a DECLARED client verb nothing calls is reported, not counted as consumption', () => {
    // The hole the old scan had (drain r1, D5): a literal inside api.ts made a
    // route look consumed even when no caller existed. Consumption is a CALL —
    // in THIS file at D5 and, since drain r2 R1, in every other file too — and
    // what the change exposed is recorded here rather than absorbed.
    //
    // THE PREDICATE IS SYNTAX-FRAGILE AND THAT IS THE SAFE DIRECTION (drain r2,
    // R5, accepted rather than fixed). `api.<member>(` does not see a call
    // rewritten as `(api).workforce.call(api, …)`, or a destructured
    // `const { workforce } = api`, or `api?.workforce?.()`. All of those make
    // this test FAIL, naming the member — a loud over-report, not a silent
    // under-count — which is the direction a completeness check must fail in.
    expect(
      declaredButUncalled(),
      'the set of declared-but-uncalled api members moved. A new one is a dead client verb; a departed one means something started calling it — either way §46 records this list.',
    ).toEqual(['health'])
    // The floor, so a scan that suddenly matched nothing could not pass here.
    const app = appSources()
    expect(Object.keys(app).length).toBeGreaterThan(20)
  })

  test('the scan’s ONE blind spot is empty, and that is checked rather than assumed', () => {
    // A route reached only through a SERVED door route would be invisible here:
    // `answerAtDoor` follows a path the server chose, so no literal appears in
    // the client. Today exactly one call site exists and the route it reaches is
    // also named literally by `answerApproval`, so the blind spot costs nothing.
    const app = appSources()
    const callSites = Object.entries(app).filter(([, t]) => stripComments(t).includes('.answerAtDoor('))
    expect(callSites.map(([p]) => p)).toEqual(['./Deliverable.tsx'])
    expect(consumed.has('POST /api/approvals/{}/answer')).toBe(true)
  })
})

describe('the eight S15.2 resource families each reach a live surface', () => {
  // The family table's own roots (S15.2), plus the two roots added
  // additive-first since: /api/chat (B6-7) and /api/push (B6-9). Each maps to
  // the SPA route that renders it — a family whose root nothing consumes is the
  // gap this walk exists to find, and `memory` is exactly that.
  const families: { family: string; root: string; surface: RouteID | null }[] = [
    { family: 'tasks', root: '/api/tasks', surface: 'task' },
    { family: 'runs', root: '/api/runs', surface: 'board' },
    { family: 'approvals', root: '/api/approvals', surface: 'inbox' },
    { family: 'deliverables', root: '/api/deliverables', surface: 'deliverable' },
    { family: 'settings', root: '/api/settings', surface: 'settings' },
    { family: 'meters', root: '/api/meters', surface: 'fleet' },
    { family: 'events', root: '/api/events', surface: 'mission-control' },
    { family: 'chat', root: '/api/chat', surface: 'chat' },
    { family: 'push', root: '/api/push', surface: 'settings' },
    // The REPORTED one. Its root is served and no surface reads it.
    { family: 'memory', root: '/api/memory', surface: null },
  ]

  test('every family with a surface has a built route AND a consumed root', () => {
    const consumed = consumedRoutes()
    const built = new Set(routes.filter((r) => r.owner === '').map((r) => r.id))
    let checked = 0
    for (const f of families) {
      if (f.surface === null) continue
      expect(built.has(f.surface), `the ${f.family} family's surface (${f.surface}) is not built`).toBe(true)
      const reached = [...consumed].some((k) => k.split(' ')[1]?.startsWith(f.root))
      expect(reached, `nothing in the SPA reads the ${f.family} family root ${f.root}`).toBe(true)
      checked++
    }
    expect(checked, 'the family walk covered nothing').toBeGreaterThan(8)
  })

  test('and the one family with NO surface is stated, not silently dropped', () => {
    const orphans = families.filter((f) => f.surface === null).map((f) => f.family)
    expect(orphans, 'the set of surface-less S15.2 families moved; §46 records it').toEqual(['memory'])
  })
})

// ── R14: phone-complete, and live by default, over the FINISHED tree ───────

describe('phone-complete (S1.10 via S15.12)', () => {
  const css = shellStylesheet

  /**
   * The S15.12 phone-complete list, verbatim: "the inbox and every decision,
   * personal filters, board and fleet glancing, task status, chat, push
   * handling". Each row names the route that carries it and the container class
   * a phone would scroll sideways on.
   *
   * jsdom has no layout engine, so what is checked is the STRUCTURE that makes a
   * phone-width surface work (the §41-B method). Real-device behaviour is the
   * B6-9 drill's, on real phones, which is why the runbook exists.
   */
  const phoneComplete: { duty: string; route: RouteID; containers: string[] }[] = [
    { duty: 'the inbox and every decision', route: 'inbox', containers: ['.cards', '.card-row', '.batch-bar'] },
    { duty: 'one decision, deep-linked', route: 'inbox-item', containers: ['.cards', '.card-row'] },
    { duty: 'personal filters', route: 'mission-control', containers: ['.filter-bar'] },
    { duty: 'board glancing', route: 'board', containers: ['.columns', '.column', '.card'] },
    { duty: 'fleet glancing', route: 'fleet', containers: ['.fleet-filters', '.table-scroll'] },
    { duty: 'task status', route: 'task', containers: ['.block', '.table-scroll'] },
    { duty: 'chat', route: 'chat', containers: ['.chat-body'] },
    { duty: 'push handling', route: 'settings', containers: ['.push-device', '.push-devices'] },
  ]

  test('the stylesheet really is the shell stylesheet', () => {
    expect(css.length).toBeGreaterThan(2000)
  })

  test('every phone-complete duty has a BUILT route and no fixed-width container', () => {
    const built = new Set(routes.filter((r) => r.owner === '').map((r) => r.id))
    let containersChecked = 0
    for (const row of phoneComplete) {
      expect(built.has(row.route), `${row.duty} has no built route (${row.route})`).toBe(true)
      for (const selector of row.containers) {
        const at = css.indexOf(`\n${selector} {`)
        expect(at, `${row.duty}: ${selector} is not styled at all`).toBeGreaterThan(-1)
        const rules = css.slice(at, css.indexOf('}', at))
        expect(rules, `${row.duty}: ${selector} pins a pixel width — a phone would scroll sideways`).not.toMatch(
          /(^|[^-])width:\s*\d+px/,
        )
        containersChecked++
      }
    }
    expect(containersChecked).toBeGreaterThan(14)
  })

  test('the responsive skeleton widens UP from the phone, never down from the desktop', () => {
    // A max-width query would mean the desktop is the base and the phone the
    // exception — the two-products shape S1.10 rules out.
    expect(css).toContain('@media (min-width:')
    expect(css, 'a max-width breakpoint makes the desktop the base').not.toMatch(/@media\s*\(\s*max-width:/)
  })

  test('the desktop-comfortable surfaces are NAMED, not assumed', () => {
    // S15.12's own second list: diff review, the workforce map, settings bulk
    // edits. They are built and reachable at phone width; they are simply where
    // the spec allows a wider screen to be more comfortable.
    const built = new Set(routes.filter((r) => r.owner === '').map((r) => r.id))
    for (const id of ['deliverable', 'workforce', 'settings'] as RouteID[]) {
      expect(built.has(id), `${id} is a desktop-comfortable surface and is not built`).toBe(true)
    }
  })
})

/**
 * The ONE surface that deliberately does not ride the feed, with its reason
 * (§42, recorded when the history panel landed).
 *
 * An ANSWER is a point-in-time reply to a question somebody asked. Re-running it
 * on every frame would be noise at Layer 0 and a repeated model call at Layer 2,
 * so asking again is an ACT and the panel offers one — and it says on screen
 * that it is a query instrument rather than a live projection, which is what
 * keeps it from posing as live.
 */
const namedNonLiveReaders = [
  './History.tsx',
  // The login page is PRE-SESSION. The one EventSource opens only for an
  // authed lifetime (§41-B), so there is no feed here to ride: what this
  // surface reads is the user picker, and what changes it is signing in.
  './Login.tsx',
]

describe('live by default, on every surface (S15.12)', () => {
  const app = appSources()

  test('the connection state is rendered ONCE, by the shell, above the route switch', () => {
    // Rendering it per view would be a rule every future view has to remember.
    // Rendering it in the shell head makes "every surface says whether it is
    // live" structural — so it is checked as a property of App.tsx's shape.
    const shell = stripComments(app['./App.tsx'] ?? '')
    expect(shell).not.toBe('')
    const mounts = shell.match(/<ConnectionState\b/g) ?? []
    expect(mounts, 'the connection indicator is mounted more than once, or not at all').toHaveLength(1)
    expect(
      shell.indexOf('<ConnectionState'),
      'the indicator is inside the route switch, so a surface could render without it',
    ).toBeLessThan(shell.indexOf('<main'))
    // And no OTHER file mounts one, which is what makes the count above mean
    // what it says.
    const others = Object.entries(app).filter(([p, t]) => p !== './App.tsx' && stripComments(t).includes('<ConnectionState'))
    expect(others.map(([p]) => p)).toEqual([])
  })

  test('every view that reads the API rides useLive — no view polls', () => {
    const readers = Object.entries(app).filter(
      ([p, t]) => p.endsWith('.tsx') && /\bapi\.[a-zA-Z]/.test(stripComments(t)),
    )
    expect(readers.length, 'the scan found no API-reading views').toBeGreaterThan(6)
    for (const [path, text] of readers) {
      if (namedNonLiveReaders.includes(path)) continue
      const body = stripComments(text)
      // A view either subscribes through useLive, or it only ACTS (a verb call
      // inside a handler) and re-reads through a parent that does.
      const rides = body.includes('useLive') || body.includes('onChanged') || body.includes('reload')
      expect(rides, `${path} reads the API without riding the feed`).toBe(true)
    }
    // Every exemption has to be a file that exists AND actually reads the API,
    // so the list cannot rot into a place things go to be forgotten.
    for (const path of namedNonLiveReaders) {
      expect(readers.map(([p]) => p), `${path} is exempted but reads no API`).toContain(path)
    }
  })

  test('nothing in the app tree schedules a poll', () => {
    // The §32 no-ticker rule, on the client: currency comes from frames, and a
    // timer would be a second, competing source of truth that keeps running
    // when the feed is down — stale posing as live.
    const hits: string[] = []
    for (const [path, text] of Object.entries(app)) {
      const body = stripComments(text)
      for (const banned of ['setInterval(', 'requestIdleCallback(']) {
        if (body.includes(banned)) hits.push(`${path}: ${banned}`)
      }
    }
    expect(hits, 'a client-side poll exists').toEqual([])
    // The probe: the predicate can match.
    expect('const t = setInterval(fn, 1000)').toContain('setInterval(')
  })
})

// ── R16: client-side filtering is PRESENTATION ONLY (S15.2) ────────────────

describe('client-side filtering narrows the DISPLAY, never the scope (S15.2)', () => {
  const app = appSources()

  /**
   * Every place the client compares an OWNER while filtering, with what it is
   * for. The list is an EXACT PARTITION, not a floor: a new site fails here and
   * has to be reasoned about, which is the whole mechanism (the §44-B D1
   * lesson — a check that only counts is a check that can be satisfied).
   *
   * NEITHER of these narrows a SCOPE. Filtering another person's rows out in the
   * browser would mean the browser had been sent them, which is what S15.2 bars;
   * choosing which of an already-scoped set to act on or to show is
   * presentation, which is what S15.2 permits in the same breath.
   */
  const ownerFilterSites: Record<string, string> = {
    './Board.tsx':
      'the OWN-QUEUED lane, which is the board’s one <Droppable>. It decides what is DRAGGABLE, not what is visible: every other task still renders in its stage column, and the priority-hint verb refuses a non-own run server-side regardless (§42). The operator is deliberately not excepted, matching the verb.',
    './Fleet.tsx':
      'the person and lane SELECTORS, whose options are derived from the served rows themselves. The default is ALL — an empty selection hides nothing — and the server already scoped the read (§42-B). Narrowing the DATA is a server parameter, which is what the `mine` filter uses.',
  }

  test('every owner-comparing filter is a NAMED presentation-only site', () => {
    const sieve = /\.filter\([\s\S]{0,140}?\b(owner|user_id)\b\s*[=!]==?/
    const hits = Object.entries(app)
      .filter(([, t]) => sieve.test(stripComments(t)))
      .map(([p]) => p)
      .sort()
    expect(
      hits,
      'an owner-comparing filter appeared in a file that is not on the presentation-only list. Either it narrows the DISPLAY of an already-scoped read — say so, with its reason — or it is narrowing a SCOPE the server should have narrowed.',
    ).toEqual(Object.keys(ownerFilterSites).sort())
    for (const [path, why] of Object.entries(ownerFilterSites)) {
      expect(why.length, `${path}'s reason is a placeholder`).toBeGreaterThan(80)
    }
    // Both directions of the probe, so the pattern is shown to discriminate.
    expect(sieve.test('rows.filter((r) => r.owner === me)')).toBe(true)
    expect(sieve.test('rows.filter((r) => r.state === "running")')).toBe(false)
  })

  test('the fleet’s selector hides NOTHING by default — the property that makes it presentation', () => {
    // Driven rather than argued: with no person chosen, every served lane row
    // reaches the screen. A selector that dropped rows by default would be
    // narrowing a scope while calling itself a filter.
    const fleet = stripComments(app['./Fleet.tsx'] ?? '')
    expect(fleet).not.toBe('')
    expect(fleet, 'the empty selection is not the all-rows case').toMatch(/person === ''\s*\|\|/)
    expect(fleet, 'the empty lane selection is not the all-rows case').toMatch(/lane === ''\s*\|\|/)
  })

  test('the personal filters narrow by SERVER PARAMETER where they narrow data', () => {
    // `mine` sends ?person=<self> rather than sieving: for a member the two
    // answers are identical, and for the operator the parameter is what makes
    // "mine" mean mine (§42-B).
    const filters = stripComments(app['./Filters.tsx'] ?? '')
    expect(filters).not.toBe('')
    expect(filters, 'the `mine` filter does not send a server parameter').toMatch(/person:/)
  })
})

// ── R18: the timestamp policy, applied through ONE primitive ───────────────

describe('timestamps render through one primitive, verbatim (B6-9 OQ12)', () => {
  const app = appSources()

  test('only the timestamp primitives render a timestamp element', () => {
    // WIDENED, not weakened (P3-UI-1): the B6 gate's D5 answer ratified a SECOND
    // timestamp primitive — `ui/Timestamp.tsx`, which adds the relative-beside-
    // verbatim `live` variant the audit-only `Stamp` deliberately does not have.
    // Naming it here is what keeps this a closed list: a THIRD file rendering
    // <time>, or a view rendering one inline, still fails, and each primitive is
    // asserted to still exist so the list cannot pass by naming a dead file.
    const primitives = ['./parts.tsx', './ui/Timestamp.tsx']
    const hits = Object.entries(app)
      .filter(([p]) => !primitives.includes(p))
      .filter(([, t]) => stripComments(t).includes('<time'))
      .map(([p]) => p)
    expect(hits, 'a view renders its own <time> element instead of using a timestamp primitive').toEqual([])
    for (const p of primitives) {
      expect(stripComments(app[p] ?? ''), `the primitive ${p} is gone`).toContain('<time')
    }
    // Both variants of the D5 primitive keep the served UTC in `dateTime`: the
    // relative label is an ADDITION, never a substitution (B6 §9 D5).
    expect(stripComments(app['./ui/Timestamp.tsx'] ?? '')).toContain('dateTime={ts}')
  })

  test('no view formats a date itself', () => {
    // The policy recorded at v0 is VERBATIM UTC: every stamp the platform serves
    // is UTC RFC3339, and reformatting one is a transformation of a fact.
    //
    // The open half of this note — "a relative presentation beside it is a real
    // want, deferred to the B6 click-through" — was ANSWERED by the gate's D5
    // decision and built at P3-UI-1: `ui/Timestamp.tsx` renders relative time
    // BESIDE the verbatim UTC in its `live` variant. That answer changes nothing
    // here. Relative-beside-verbatim is an addition; LOCALIZING an instant is
    // still a transformation of a fact, and is still banned tree-wide.
    const formatters = ['toLocaleString(', 'toLocaleDateString(', 'toLocaleTimeString(', 'Intl.DateTimeFormat']
    const hits: string[] = []
    for (const [path, text] of Object.entries(app)) {
      const body = stripComments(text)
      for (const f of formatters) if (body.includes(f)) hits.push(`${path}: ${f}`)
    }
    expect(hits, 'a view localizes a served instant').toEqual([])
    for (const f of formatters) expect(`d.${f}`).toContain(f)
  })

  test('the ONE local-time conversion in the tree is a REQUEST parameter, not a render', () => {
    // `localMidnightISO` exists because "finished today" means midnight where
    // the READER is — it composes the UTC instant the API compares against, and
    // it renders nothing (§42-B). It is named here so the uniformity claim above
    // is not quietly false.
    const users = Object.entries(app)
      .filter(([, t]) => stripComments(t).includes('localMidnightISO'))
      .map(([p]) => p)
      .sort()
    expect(users).toEqual(['./Filters.tsx'])
  })
})


// ── R10/S01.8: the shipped shell reaches no third party ────────────────────

describe('the installed app fetches nothing from outside this origin', () => {
  // index.html and the manifest are the two files a browser reads BEFORE any of
  // this SPA runs, so a CDN font or a remote icon there would be an external
  // observable the S01.8 register does not admit — and one no source scan over
  // `web/src` could ever see. It held by inspection only until now (drain r1,
  // D14).
  const shipped = {
    ...(import.meta.glob('../index.html', { query: '?raw', import: 'default', eager: true }) as Record<string, string>),
    ...(import.meta.glob('../public/*.{webmanifest,svg,js}', { query: '?raw', import: 'default', eager: true }) as Record<
      string,
      string
    >),
  }

  test('the scan reads the real shipped files', () => {
    expect(Object.keys(shipped).sort()).toEqual(
      ['../index.html', '../public/icon.svg', '../public/manifest.webmanifest', '../public/sw.js'].sort(),
    )
    expect(shipped['../index.html']).toContain('<div id="root">')
  })

  test('no absolute URL to another origin appears in any of them', () => {
    const hits: string[] = []
    for (const [path, text] of Object.entries(shipped)) {
      // sw.js's own prose cites the WebKit post it was written against; code may
      // not dial what prose may cite, so comments come off first — the same
      // rule every other scan in this tree applies.
      const body = path.endsWith('.js') ? stripComments(text) : text.replace(/<!--[\s\S]*?-->/g, '')
      for (const m of body.matchAll(/\bhttps?:\/\/[^\s"'`<>)]+/g)) {
        // An XML namespace is a NAME, not a fetch: the SVG's xmlns is never
        // dereferenced by any renderer.
        if (m[0].startsWith('http://www.w3.org/')) continue
        hits.push(`${path}: ${m[0]}`)
      }
    }
    expect(hits, 'the shipped shell reaches a third party — the S01.8 register admits no such observable').toEqual([])
  })

  test('every asset the shell names is a same-origin path', () => {
    const html = shipped['../index.html']
    const refs = [...html.matchAll(/(?:href|src)="([^"]+)"/g)].map((m) => m[1])
    expect(refs.length, 'the shell names no assets at all').toBeGreaterThan(3)
    for (const ref of refs) {
      expect(ref.startsWith('/'), `${ref} is not a rooted same-origin path`).toBe(true)
    }
    // And the manifest's icons likewise.
    const manifest = JSON.parse(shipped['../public/manifest.webmanifest']) as { icons: { src: string }[] }
    expect(manifest.icons.length).toBeGreaterThan(2)
    for (const icon of manifest.icons) expect(icon.src.startsWith('/')).toBe(true)
    // The probe: the predicate can fail.
    expect('https://cdn.example/x.png'.startsWith('/')).toBe(false)
  })
})
