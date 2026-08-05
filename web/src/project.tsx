import { createContext, useContext } from 'react'

import type { RouteID } from './routes'

/**
 * The global project scope (product map v2.1 §2) — the Nexus focus-context
 * chips, carried onto the current backend contract.
 *
 * WHAT IT IS. One selected project name, held in React state for the life of
 * the tab. `''` means "all projects". `'(no project)'` is a REAL served bucket
 * (the tasks read serves it as a project value), so it is a selectable scope
 * like any other, never a dropped row.
 *
 * WHAT IT IS NOT. It is not persisted: §41-B bans web storage outright and no
 * server-side per-person UI-state store exists, so the scope lives exactly as
 * long as the tab does — a reload shows everything again. Recorded as the
 * honest shape rather than papered over (the same reasoning as the tour's
 * never-auto-start).
 *
 * WHO CONSUMES IT. Surfaces whose data carries a project dimension scope to
 * it; surfaces without that dimension say so instead of pretending (map §2).
 * `scopedRoutes` below is the registry the topbar reads to say which one the
 * current page is — a surface joins it in the build-order step that reworks it.
 */
export type ProjectScope = {
  /** Selected project name; '' = all projects. */
  project: string
  setProject: (project: string) => void
}

export const ProjectScopeContext = createContext<ProjectScope>({
  project: '',
  setProject: () => {},
})

export function useProjectScope(): ProjectScope {
  return useContext(ProjectScopeContext)
}

/**
 * The routes that scope to the selected project TODAY. Board, Reviews and
 * History join in their own rework steps; listing one before its surface
 * actually narrows would make the chip lie about what the reader is seeing.
 */
export const scopedRoutes: ReadonlySet<RouteID> = new Set<RouteID>(['mission-control'])
