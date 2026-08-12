import { expect, test } from 'vitest'

import { mergeCards, projectBuckets } from './Projects'
import type { Deliverable, TaskListItem } from './api'

/**
 * The bucket math behind /projects (map §3 v3), as pure functions.
 *
 * FOUND LIVE 2026-08-12 (operator, B6 click-through): the list wire used to
 * serve a no-project deliverable with the `project_id` field ABSENT
 * (`omitempty`), not `''` as the type promised — the undefined bucket name
 * threw in the alphabetical sort and the whole surface unmounted to a black
 * page. The Go tag is fixed alongside this, but the client must also never
 * again let one absent field take the page down.
 */

function dlv(over: Partial<Deliverable>): Deliverable {
  return {
    deliverable_id: 'dlv-x',
    owner: 'alice',
    task_id: 't-x',
    project_id: 'orchard-shop',
    type: 'code',
    current_revision: 2,
    state: 'in_review',
    created_ts: '2026-08-10T10:00:00Z',
    updated_ts: '2026-08-11T10:00:00Z',
    ...over,
  }
}

function task(over: Partial<TaskListItem>): TaskListItem {
  return {
    task_id: 't-x',
    owner: 'alice',
    title: 'a task',
    kanban_status: 'intake',
    project: '(no project)',
    created_ts: '2026-08-10T09:00:00Z',
    latest_run: null,
    ...over,
  }
}

test('a deliverable with project_id ABSENT files under (no project) instead of crashing', () => {
  const absent = dlv({ deliverable_id: 'dlv-absent' })
  delete (absent as { project_id?: string }).project_id

  const buckets = projectBuckets(
    [task({ task_id: 't-a', project: 'orchard-shop' })],
    [absent, dlv({ deliverable_id: 'dlv-empty', project_id: '', updated_ts: '2026-08-11T09:00:00Z' })],
  )

  // Sorting the bucket names is where the undefined key used to throw.
  expect(buckets.map((b) => b.name)).toEqual(['orchard-shop', '(no project)'])
  const none = buckets.find((b) => b.name === '(no project)')
  expect(none?.recent.map((d) => d.deliverable_id)).toEqual(['dlv-absent', 'dlv-empty'])
})

test('(no project) sinks to the end through the merge as well', () => {
  const buckets = projectBuckets(
    [task({ task_id: 't-a', project: 'zzz-last-alphabetically' })],
    [dlv({ deliverable_id: 'dlv-none', project_id: '' })],
  )
  const cards = mergeCards(buckets, [])
  expect(cards.map((c) => c.name)).toEqual(['zzz-last-alphabetically', '(no project)'])
})
