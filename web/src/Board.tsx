import { DragDropContext, Draggable, Droppable, type DropResult } from '@hello-pangea/dnd'
import { useState } from 'react'

import { api, type PriorityHint, type TaskListItem } from './api'
import type { EventStream } from './events'
import { columnsFor, groupByProject } from './kanban'
import { boardEventTypes, describeError, useLive } from './live'
import { Absent, Freshness, Money, Owner, ParkedUntil, Section, SurfaceHead } from './parts'
import { Link } from './router'
import { hrefFor } from './routes'
import { Chip, EmptyState } from './ui'

/**
 * The live board (Spec S15.5 ¶2; 9.1; S1.3; FC-v1 §1).
 *
 * Cards move from the feed: the board loads its REST snapshot, tails the one
 * connection with the types that change a card, and re-reads on the resnapshot
 * signal. Movement follows the STORED kanban_status — this view invents no
 * status, computes no percentage and shows no ETA, because the API serves none
 * of those and a board is exactly where a made-up "60% done" would look most
 * plausible.
 *
 * THE DRAG'S STRUCTURAL NEGATIVE. Stage is FSM state owned by the control
 * plane (S02), so a stage column must not be writable by drag. That is enforced
 * by CONSTRUCTION rather than by a rule somebody has to keep: the stage columns
 * below are plain lists, and the ONE `<Droppable>` on this screen is the
 * caller's own queued lane. There is no cross-column drop target to disable,
 * and `hintPostsFor` refuses any destination that is not that lane, so even a
 * synthesized drop result calls no verb.
 */

/** The one droppable id on the board. Reordering own queued work is the whole
 *  sanctioned v0 drag interaction (S15.5). */
export const ownQueueDroppableId = 'own-queued'

/**
 * Rank spacing and bound (B6-5 OQ6(ii)).
 *
 * Structural constants with named reasons; no ⚙ key exists and the browser
 * cannot read the registry before login (§41 precedent).
 *
 *  - `rankSpacing` 10, positions 1-based: ranks are 10, 20, 30…, so a later
 *    single-card move usually has an integer gap to land in, and 0 is never
 *    assigned — 0 means "no hint" to the scheduler, so a strategy that used it
 *    for "first" would silently un-rank the card it just promoted.
 *  - `hintRankBound` 1000 mirrors the verb's own structural bound. NAMED EDGE:
 *    spaced ranks reach it at position 100, so beyond the hundredth queued task
 *    the ranks clamp and the tail falls back to the scheduler's default order.
 *    That is stated rather than worked around — a household queue does not
 *    reach 100 own-queued tasks, and pretending to order past the bound would
 *    mean sending ranks the verb rejects.
 */
const rankSpacing = 10
const hintRankBound = 1000

export function spacedRank(position: number): number {
  return Math.min((position + 1) * rankSpacing, hintRankBound)
}

/** The rank the server currently holds for a task; 0 is "no hint". */
function currentRank(t: TaskListItem): number {
  return t.latest_run?.queue_hint_rank ?? 0
}

/**
 * ownQueued is the drag-eligible set: the CALLER's own tasks whose latest run
 * is still queued. The operator is deliberately not excepted — the verb refuses
 * another person's task, and a card that cannot be reordered must not look
 * draggable.
 *
 * The order is PLAIN ASCENDING by hint rank, then the scheduler's own default.
 * That is the landed comparator's rule, verbatim (internal/scheduler/
 * workload.go: `if x.hintRank != y.hintRank { return x.hintRank < y.hintRank }`,
 * then the natural key) — 0 is the NEUTRAL MIDDLE, not "unranked, put it last".
 * Treating 0 as last was the first cut's bug: a task enqueued after a drag sits
 * at 0, so it would have claimed FIRST from the scheduler while rendering LAST
 * here, which is the exact "rendered order = recorded order" promise inverted.
 */
export function ownQueued(tasks: TaskListItem[], me: string): TaskListItem[] {
  return tasks
    .filter((t) => t.owner === me && me !== '' && t.latest_run?.state === 'queued')
    .sort((a, b) => {
      const ra = currentRank(a)
      const rb = currentRank(b)
      if (ra !== rb) return ra - rb
      return a.created_ts.localeCompare(b.created_ts)
    })
}

export type HintPost = { task: string; rank: number }

/**
 * hintPostsFor turns a completed drag into the writes it implies.
 *
 * It is a pure function so the negative is checkable without a real drag: a
 * drop outside the own-queued lane — the shape a stage-column drop would have
 * if one were ever wired — returns NO posts, and therefore calls no verb.
 * Only the entries whose EFFECTIVE rank changed are written, so a drag that
 * put a card back where it came from writes nothing.
 */
export function hintPostsFor(queue: TaskListItem[], result: DropResult): HintPost[] {
  const to = result.destination
  if (!to) return [] // dropped outside any list
  if (to.droppableId !== ownQueueDroppableId) return []
  if (result.source.droppableId !== ownQueueDroppableId) return []
  if (to.index === result.source.index) return []

  const next = [...queue]
  const [moved] = next.splice(result.source.index, 1)
  if (!moved) return []
  next.splice(to.index, 0, moved)

  // Every card in the lane is assigned a POSITIVE spaced rank, not just the
  // ones that visibly moved. That is what leaves no 0 behind: 0 is the
  // scheduler's neutral middle, so a card left at 0 after a drag would sit
  // among the ranked ones rather than where it was dropped. Only the entries
  // whose effective rank actually changed are written — assigning every
  // position and writing the difference is idempotent, so a drag that puts a
  // card back writes nothing.
  const posts: HintPost[] = []
  next.forEach((t, i) => {
    const rank = spacedRank(i)
    if (currentRank(t) !== rank) posts.push({ task: t.task_id, rank })
  })
  return posts
}

/**
 * applyDrag is the drag's whole write path, kept out of the component so the
 * contract can be driven directly: what it POSTs, what it refuses to POST, and
 * that a drop which is not an own-queue reorder reaches the network at all.
 */
export async function applyDrag(queue: TaskListItem[], result: DropResult): Promise<PriorityHint[]> {
  const posts = hintPostsFor(queue, result)
  if (posts.length === 0) return []
  return Promise.all(posts.map((p) => api.priorityHint(p.task, p.rank)))
}

export function Board({ me, stream }: { me: string; stream?: EventStream }) {
  const { data, error, stale, reload } = useLive({
    key: '/api/tasks',
    read: () => api.tasks(),
    types: boardEventTypes,
    stream,
  })
  const [notes, setNotes] = useState<PriorityHint[]>([])

  const tasks = data?.tasks ?? []
  const queue = ownQueued(tasks, me)
  // Same rule as every other surface: a teaching empty states what the platform
  // answered, so it waits for an answer. Until then `Freshness` says the board
  // is catching up (§42).
  const answered = data !== null

  const onDragEnd = (result: DropResult) => {
    void applyDrag(queue, result).then(
      (answers) => {
        if (answers.length === 0) return
        // The stale-board answer is surfaced, not swallowed: `applied:false`
        // means the work moved on between the render and the drag, and the
        // verb's own `detail` says what the hint can and cannot do. Then the
        // board RE-READS — the server's order is the order, and nothing this
        // drag believed outlives that read.
        setNotes(answers)
        reload()
      },
      (err: unknown) => {
        // An honest failure note: the drag did not land, and the board says so
        // rather than silently snapping back as though nothing was attempted.
        setNotes([
          {
            task_id: 'drag',
            rank: 0,
            runs: [],
            applied: false,
            detail: `The re-rank did not land. ${describeError(err)}`,
          },
        ])
        reload()
      },
    )
  }

  return (
    <section className="surface">
      <SurfaceHead
        title="Board"
        what="Every task as a card, moving through its stages live from the feed. Dragging re-ranks your own queued work only — it never moves a card to another stage."
      />
      <Freshness stale={stale} error={error} hasData={data !== null} />

      <DragDropContext onDragEnd={onDragEnd}>
        <Section title="Your queue" stale={stale}>
          <p className="muted mt-0 max-w-prose text-xs">
            Drag to re-rank your own queued work. A hint breaks ties among your own same-class queued work — it never
            outranks the workload class ladder and never reaches another person&apos;s queue.
          </p>
          {notes.map((n) => (
            <p
              key={n.task_id}
              className={`${n.applied ? 'notice' : 'muted'} text-xs`}
              data-hint-applied={String(n.applied)}
            >
              {n.detail}
            </p>
          ))}
          <Droppable droppableId={ownQueueDroppableId}>
            {(dropProvided) => (
              <ul className="queue-lane min-h-8 list-none p-0" ref={dropProvided.innerRef} {...dropProvided.droppableProps}>
                {queue.map((t, index) => (
                  <Draggable key={t.task_id} draggableId={t.task_id} index={index}>
                    {(dragProvided) => (
                      // The lift affordance lives HERE and nowhere else: this is
                      // the one lane a card can be reordered in, and a card that
                      // cannot be reordered must not look draggable (§42).
                      <li
                        ref={dragProvided.innerRef}
                        {...dragProvided.draggableProps}
                        {...dragProvided.dragHandleProps}
                        className="card my-1 cursor-grab rounded-(--radius-sm) border border-border bg-(image:--panel-grad) p-2 motion-safe:transition-[border-color,box-shadow] hover:border-[var(--border-l)] hover:shadow-(--shadow-soft) active:cursor-grabbing"
                      >
                        <CardFace task={t} />
                      </li>
                    )}
                  </Draggable>
                ))}
                {dropProvided.placeholder}
                {queue.length === 0 && answered && (
                  <EmptyState
                    what="Nothing of yours is queued."
                    why="Your own tasks appear here while they wait for the scheduler to start them. This lane is the only place a drag does anything."
                  />
                )}
              </ul>
            )}
          </Droppable>
        </Section>
      </DragDropContext>

      <div className="columns">
        {columnsFor(tasks).map((col) => {
          const inColumn = tasks.filter((t) => t.kanban_status === col.status)
          return (
            <section className="column" key={col.status} data-status={col.status} data-known={String(col.known)}>
              <h2 className="mt-0 mb-2 font-mono text-[9.5px] font-bold tracking-[2px] text-muted-foreground uppercase">
                {col.label}
              </h2>
              {!col.known && (
                <p className="muted text-xs">
                  This column is a stored status the board does not have a name for. The card is shown under its own
                  value rather than hidden.
                </p>
              )}
              {inColumn.length === 0 && answered ? (
                <EmptyState
                  what="No card is here."
                  why={`A card sits under the stage the platform stored for it. It arrives in "${col.label}" when its work reaches that stage — never by being dragged.`}
                />
              ) : (
                groupByProject(inColumn).map((group) => (
                  <div className="project-group" key={group.project} data-project={group.project}>
                    <h3 className="mt-3 mb-1 text-[0.85rem] text-muted-foreground">{group.project}</h3>
                    <ul className="cards list-none p-0">
                      {group.tasks.map((t) => (
                        // No drag affordance: this card is not in the one
                        // reorderable lane, and looking draggable would promise
                        // a gesture the verb refuses.
                        <li
                          className="card my-1 rounded-(--radius-sm) border border-border bg-(image:--panel-grad) p-2"
                          key={t.task_id}
                        >
                          <CardFace task={t} />
                        </li>
                      ))}
                    </ul>
                  </div>
                ))
              )}
            </section>
          )
        })}
      </div>
    </section>
  )
}

/**
 * The card face is EXACTLY the S1.3 set (S15.5; 3.5; G2 D2.1): what it is,
 * whose it is, current stage, effort mode with any disclosed downgrade note,
 * cost so far, and waiting-on-human. Nothing more — the exhaustive trace is the
 * task detail's job — and nothing less.
 *
 * The park horizon rides the waiting line because "waiting on a human" and
 * "waiting on a clock" are the two ways a card is stopped, and a card that is
 * stopped without saying which is the one this face must not produce.
 */
export function CardFace({ task }: { task: TaskListItem }) {
  const run = task.latest_run
  // The left priority rail is TREATMENT, not a field: it takes its tone from
  // the state the face already renders, so it can say nothing the face does not.
  const rail = run?.waiting_on_human ? 'orange' : run?.state === 'parked' ? 'yellow' : run?.state === 'running' ? 'green' : 'accent'
  return (
    <article className="card-face border-s-2 ps-2" style={{ borderInlineStartColor: `var(--${rail})` }}>
      <h4 className="mt-0 mb-2 text-[0.95rem]">
        <Link to={hrefFor('task', { id: task.task_id })}>{task.title}</Link>
      </h4>
      <dl className="m-0 grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 text-[0.85rem]">
        <dt>Whose</dt>
        <dd>
          <Owner id={task.owner} />
        </dd>

        <dt>Stage</dt>
        <dd>
          {run?.stage ? <Chip>{run.stage}</Chip> : <Absent reason="no stage marker yet" />}
        </dd>

        <dt>Effort</dt>
        <dd>
          {run?.effort_mode ? run.effort_mode : <Absent reason="no effort mode recorded" />}
          {run?.downgrade_note ? (
            <span className="downgrade-note text-muted-foreground"> — {run.downgrade_note}</span>
          ) : null}
        </dd>

        <dt>Cost so far</dt>
        <dd>
          <Money usd={run?.cost_so_far_usd} />
        </dd>

        <dt>Waiting</dt>
        <dd>
          {run?.waiting_on_human ? (
            <span className="waiting-human text-[var(--orange)]">waiting on a person</span>
          ) : run?.state === 'parked' ? (
            <ParkedUntil until={run.parked_until} />
          ) : (
            'no'
          )}
        </dd>
      </dl>
    </article>
  )
}
