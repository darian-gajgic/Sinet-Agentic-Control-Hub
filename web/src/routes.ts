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
 *
 * EXTENDED 2026-08-05 (frontend rework step 1, product map v2.1 §2): seven new
 * routes — /projects, /new, /reviews, /lessons, /history, /health, /manual —
 * added per the approved map. ADDITIONS publish paths nothing pointed at
 * before; every previously published pattern below is byte-identical. Two rows
 * changed TITLE only (the map's plain names): mission-control is "Home" and
 * workforce is "Specialists" — the URLs never rename.
 */

export type RouteID =
  | 'mission-control'
  | 'projects'
  | 'new'
  | 'board'
  | 'task'
  | 'inbox'
  | 'inbox-item'
  | 'reviews'
  | 'deliverable'
  | 'lessons'
  | 'chat'
  | 'history'
  | 'memory'
  | 'memory-entry'
  | 'workforce'
  | 'fleet'
  | 'health'
  | 'settings'
  | 'manual'
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

/**
 * Nav order IS the product map §2 order — the shell's grouping cuts this
 * table's nav rows into consecutive runs, so the sidebar cannot reorder it.
 * The seven new routes are BUILT (owner: ''): each answers with an honest
 * placeholder surface that says what will appear there, never a blank page.
 */
export const routes: RouteDef[] = [
  { id: 'mission-control', pattern: '/', title: 'Home', owner: '', nav: true },
  { id: 'projects', pattern: '/projects', title: 'Projects', owner: '', nav: true },
  { id: 'new', pattern: '/new', title: 'Describe a goal', owner: '', nav: true },
  { id: 'board', pattern: '/board', title: 'Board', owner: '', nav: true },
  { id: 'task', pattern: '/tasks/:id', title: 'Task', owner: '', nav: false },
  { id: 'inbox', pattern: '/inbox', title: 'Inbox', owner: '', nav: true },
  { id: 'inbox-item', pattern: '/inbox/:id', title: 'Approval', owner: '', nav: false },
  { id: 'reviews', pattern: '/reviews', title: 'Reviews', owner: '', nav: true },
  { id: 'deliverable', pattern: '/deliverables/:id', title: 'Deliverable', owner: '', nav: false },
  { id: 'lessons', pattern: '/lessons', title: 'Wins & Lessons', owner: '', nav: true },
  { id: 'chat', pattern: '/chat', title: 'Assistant', owner: '', nav: true },
  { id: 'history', pattern: '/history', title: 'History', owner: '', nav: true },
  { id: 'memory', pattern: '/memory', title: 'Memory', owner: '', nav: true },
  { id: 'memory-entry', pattern: '/memory/:id', title: 'Memory entry', owner: '', nav: false },
  { id: 'workforce', pattern: '/workforce', title: 'Specialists', owner: '', nav: true },
  { id: 'fleet', pattern: '/fleet', title: 'Fleet', owner: '', nav: true },
  { id: 'health', pattern: '/health', title: 'Health & evals', owner: '', nav: true },
  { id: 'settings', pattern: '/settings', title: 'Settings', owner: '', nav: true },
  { id: 'manual', pattern: '/manual', title: 'Manual', owner: '', nav: true },
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
