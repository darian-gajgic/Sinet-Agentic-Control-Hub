/**
 * The URL contract (P3/CONVENTIONS.md §41-B).
 *
 * These are REAL paths from day one, not placeholders: Spec S15.11 requires
 * every approval card and task to have a stable URL, and the `navigate` field
 * of a Declarative Web Push payload will point at one of them. A later packet
 * fills a surface in; it never renames the route.
 *
 * The table is owned rather than adopted (OQ6): the S15 need is pathname
 * routes, deep links and bookmarks — no loaders, no nested-data machinery — and
 * a router at that size is not an organ.
 */

export type RouteID =
  | 'mission-control'
  | 'board'
  | 'task'
  | 'fleet'
  | 'inbox'
  | 'inbox-item'
  | 'settings'
  | 'chat'
  | 'deliverable'
  | 'workforce'
  | 'memory'
  | 'memory-entry'
  | 'login'
  | 'not-found'

export type RouteDef = {
  id: RouteID
  /** Path pattern; `:name` segments capture into params. */
  pattern: string
  /** Nav/heading label. */
  title: string
  /** The packet that fills this surface in. '' = built. */
  owner: string
  /** Whether the route appears in the shell's primary nav. */
  nav: boolean
}

export const routes: RouteDef[] = [
  { id: 'mission-control', pattern: '/', title: 'Mission control', owner: '', nav: true },
  { id: 'board', pattern: '/board', title: 'Board', owner: '', nav: true },
  { id: 'task', pattern: '/tasks/:id', title: 'Task', owner: '', nav: false },
  { id: 'fleet', pattern: '/fleet', title: 'Fleet', owner: '', nav: true },
  { id: 'inbox', pattern: '/inbox', title: 'Inbox', owner: '', nav: true },
  { id: 'inbox-item', pattern: '/inbox/:id', title: 'Approval', owner: '', nav: false },
  { id: 'settings', pattern: '/settings', title: 'Settings', owner: '', nav: true },
  { id: 'chat', pattern: '/chat', title: 'Assistant', owner: '', nav: true },
  { id: 'deliverable', pattern: '/deliverables/:id', title: 'Deliverable', owner: '', nav: false },
  { id: 'workforce', pattern: '/workforce', title: 'Workforce', owner: '', nav: true },
  // ADDED 2026-08-05 (P3-UI-3), the first addition since the table was written.
  // The ban this file records is on RENAMES — a published path is what a deep
  // link and a push `navigate` field were written against. An ADDITION publishes
  // a path nothing pointed at before, and S09.4's v0 activation table requires
  // one: "Manual L2 — human-direct entries via workspace UI [XREF:S15]". The
  // entry route exists so a conflict edge naming another entry is navigable,
  // which is the S15.11 deep-links-always-land ethos on this family.
  { id: 'memory', pattern: '/memory', title: 'Memory', owner: '', nav: true },
  { id: 'memory-entry', pattern: '/memory/:id', title: 'Memory entry', owner: '', nav: false },
  { id: 'login', pattern: '/login', title: 'Sign in', owner: '', nav: false },
]

export type Match = {
  route: RouteDef
  params: Record<string, string>
}

const notFound: RouteDef = {
  id: 'not-found',
  pattern: '',
  title: 'Not found',
  owner: '',
  nav: false,
}

function segments(path: string): string[] {
  return path.split('/').filter((s) => s !== '')
}

/**
 * matchRoute resolves a pathname against the table. An unknown path resolves to
 * `not-found` rather than throwing: the server serves index.html for every
 * unrouted GET, so the client is what tells a person their URL is wrong.
 */
export function matchRoute(pathname: string): Match {
  const parts = segments(pathname)
  for (const route of routes) {
    const want = segments(route.pattern)
    if (want.length !== parts.length) continue
    const params: Record<string, string> = {}
    let ok = true
    for (let i = 0; i < want.length; i++) {
      const w = want[i]
      if (w.startsWith(':')) {
        params[w.slice(1)] = decodeURIComponent(parts[i])
        continue
      }
      if (w !== parts[i]) {
        ok = false
        break
      }
    }
    if (ok) return { route, params }
  }
  return { route: notFound, params: {} }
}

/** hrefFor builds a concrete path from a route id and its params. */
export function hrefFor(id: RouteID, params: Record<string, string> = {}): string {
  const route = routes.find((r) => r.id === id)
  if (!route) throw new Error(`unknown route id: ${id}`)
  return route.pattern
    .split('/')
    .map((seg) => (seg.startsWith(':') ? encodeURIComponent(params[seg.slice(1)] ?? '') : seg))
    .join('/')
}
