import type { TaskListItem } from './api'

/**
 * The board's column vocabulary — map §3 v3 (operator-worded, checkpoint-2
 * decision D-B).
 *
 * The stored statuses are what the producers write today (`intake` from the
 * intake pipeline, the rest from the stage skeleton) and they are NEVER
 * renamed at the source: the board renders display labels over them. The
 * operator's words fix the labels: the first column is **Backlog** (it renders
 * the stored `intake`), **Done stays visible**, and there is **no Cancelled
 * column** — a cancelled task renders IN Backlog wearing a clear cancelled
 * sign, because a cancelled goal is still a record of intent, not a stage of
 * work.
 *
 * FORWARD-TOLERANT BY CONSTRUCTION (S14.2 rule 3, applied client-side,
 * unchanged from the landed vocabulary): a stored value this list does not
 * know gets its OWN column, labelled with the raw string. A new producer
 * string must never vanish a card, and inventing a friendly name for a value
 * the platform has not decided on would be worse than showing the value.
 */
export type Column = {
  status: string
  label: string
  /** The glow tone of the column header's dot (a CSS token name). */
  tone: string
  /** False for a column this list did not declare — the honest bucket. */
  known: boolean
}

const declared: { status: string; label: string; tone: string }[] = [
  { status: 'intake', label: 'Backlog', tone: 'blue' },
  { status: 'executing', label: 'Executing', tone: 'green' },
  { status: 'verifying', label: 'Verifying', tone: 'cyan' },
  { status: 'attention', label: 'Needs attention', tone: 'orange' },
  { status: 'done', label: 'Done', tone: 'accent' },
]

export const declaredStatuses: readonly string[] = declared.map((c) => c.status)

/** The stored status Backlog absorbs instead of a column of its own (D-B). */
export const cancelledStatus = 'cancelled'

/**
 * columnsFor returns the columns to render: the five declared ones, always,
 * in their flow order — an empty column is information, it says nothing is at
 * that stage — followed by one column per unrecognized stored value.
 * `cancelled` is recognized and absorbed by Backlog, so it never becomes a
 * column of its own.
 */
export function columnsFor(tasks: TaskListItem[]): Column[] {
  const cols: Column[] = declared.map((c) => ({ ...c, known: true }))
  const unknown = new Set<string>()
  for (const t of tasks) {
    if (!declaredStatuses.includes(t.kanban_status) && t.kanban_status !== cancelledStatus) {
      unknown.add(t.kanban_status)
    }
  }
  for (const status of [...unknown].sort()) {
    cols.push({ status, label: status === '' ? '(no status recorded)' : status, tone: 'pink', known: false })
  }
  return cols
}

/** The cards of one column: its own stored status, plus — for Backlog — every
 *  cancelled task, each of which renders its cancelled sign on the card. */
export function tasksInColumn(tasks: TaskListItem[], status: string): TaskListItem[] {
  return tasks.filter(
    (t) => t.kanban_status === status || (status === 'intake' && t.kanban_status === cancelledStatus),
  )
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
