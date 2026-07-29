import type { TaskListItem } from './api'

/**
 * The board's column vocabulary (B6-5 OQ7, ratified: read the landed strings
 * as data).
 *
 * These six values are what the producers actually store today — the intake
 * pipeline writes `intake`, the stage skeleton writes the rest — and the board
 * renders them with display labels rather than renaming them at the source.
 * Renaming a producer string would touch internal/stage and internal/intake,
 * which is its own decision, not a side effect of building the view.
 *
 * FORWARD-TOLERANT BY CONSTRUCTION (S14.2 rule 3, applied client-side): a
 * stored value this list does not know gets its OWN column, labelled with the
 * raw string. A new producer string must never vanish a card, and inventing a
 * friendly name for a value the platform has not decided on would be worse
 * than showing the value.
 */
export type Column = {
  status: string
  label: string
  /** False for a column this list did not declare — the honest bucket. */
  known: boolean
}

const declared: { status: string; label: string }[] = [
  { status: 'intake', label: 'Intake' },
  { status: 'executing', label: 'Executing' },
  { status: 'verifying', label: 'Verifying' },
  { status: 'attention', label: 'Needs attention' },
  { status: 'done', label: 'Done' },
  { status: 'cancelled', label: 'Cancelled' },
]

export const declaredStatuses: readonly string[] = declared.map((c) => c.status)

/**
 * columnsFor returns the columns to render: the six declared ones, always, in
 * their flow order — an empty column is information, it says nothing is at
 * that stage — followed by one column per unrecognized stored value.
 */
export function columnsFor(tasks: TaskListItem[]): Column[] {
  const cols: Column[] = declared.map((c) => ({ ...c, known: true }))
  const unknown = new Set<string>()
  for (const t of tasks) {
    if (!declaredStatuses.includes(t.kanban_status)) unknown.add(t.kanban_status)
  }
  for (const status of [...unknown].sort()) {
    cols.push({ status, label: status === '' ? '(no status recorded)' : status, known: false })
  }
  return cols
}

/** The project bucket a card belongs to. '(no project)' arrives from the
 *  server as a real value — the honest bucket, never a dropped row (§37). */
export function groupByProject(tasks: TaskListItem[]): { project: string; tasks: TaskListItem[] }[] {
  const groups = new Map<string, TaskListItem[]>()
  for (const t of tasks) {
    const bucket = groups.get(t.project)
    if (bucket) bucket.push(t)
    else groups.set(t.project, [t])
  }
  return [...groups.entries()].sort((a, b) => a[0].localeCompare(b[0])).map(([project, list]) => ({ project, tasks: list }))
}
