import { DragDropContext, Draggable, Droppable, type DropResult } from '@hello-pangea/dnd'
import { useMemo, useState } from 'react'
import { ChevronDown, ChevronUp, Sparkles } from 'lucide-react'

import { api, type PriorityHint, type TaskDetail, type TaskListItem } from './api'
import type { EventStream } from './events'
import { cancelledStatus, columnsFor, groupByProject, tasksInColumn } from './kanban'
import { boardEventTypes, describeError, useLive } from './live'
import { Absent, Freshness, Money, Owner, ParkedUntil } from './parts'
import { useProjectScope } from './project'
import { navigate } from './router'
import { hrefFor } from './routes'
import { Chip, StatusDot } from './ui'

/**
 * The board — a RECOGNIZABLE KANBAN (map §3 v3, operator-worded at
 * checkpoint 2): real bounded columns with glowing-dot headers and count
 * pills, horizontally scrolling and NEVER wrapping at zoom or narrow width;
 * Backlog · Executing · Verifying · Needs attention · Done, with cancelled
 * tasks IN Backlog under a clear cancelled sign, and Done always visible.
 *
 * Cards move from the feed: the board loads its REST snapshot, tails the one
 * connection, and re-reads on the frames that change a card. Movement follows
 * the STORED kanban_status — this view invents no status, computes no
 * percentage and shows no ETA, because the API serves none of those.
 *
 * D-A (operator decision): a card EXPANDS to show the task's plan steps (each
 * with its "done when") and the recorded stage story as sub-items — read
 * lazily from the task's own detail on first expand. Plan steps are not task
 * objects and carry no per-step completion flag, so none is invented: the
 * steps are the plan, the stage records are the progress, and both render as
 * the served facts they are.
 *
 * THE DRAG'S STRUCTURAL NEGATIVE (unchanged from the landed contract). Stage
 * is FSM state owned by the control plane (S02), so a stage column must not
 * be writable by drag. That is enforced by CONSTRUCTION: the stage columns
 * are plain lists, and the ONE `<Droppable>` on this screen is the caller's
 * own queued lane. There is no cross-column drop target to disable, and
 * `hintPostsFor` refuses any destination that is not that lane, so even a
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
 * workload.go): 0 is the NEUTRAL MIDDLE, not "unranked, put it last".
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
  // ones that visibly moved: 0 is the scheduler's neutral middle, so a card
  // left at 0 after a drag would sit among the ranked ones rather than where
  // it was dropped. Only the entries whose effective rank actually changed
  // are written, so the write set is idempotent.
  const posts: HintPost[] = []
  next.forEach((t, i) => {
    const rank = spacedRank(i)
    if (currentRank(t) !== rank) posts.push({ task: t.task_id, rank })
  })
  return posts
}

/**
 * applyDrag is the drag's whole write path, kept out of the component so the
 * contract can be driven directly: what it POSTs, what it refuses to POST,
 * and that a drop which is not an own-queue reorder never reaches the network.
 */
export async function applyDrag(queue: TaskListItem[], result: DropResult): Promise<PriorityHint[]> {
  const posts = hintPostsFor(queue, result)
  if (posts.length === 0) return []
  return Promise.all(posts.map((p) => api.priorityHint(p.task, p.rank)))
}

/* ── filters (only over what is real at v0: person, project, effort,
      waiting-on-you — map §3) ──────────────────────────────────────────── */

type BoardFilter = {
  q: string
  person: string
  project: string
  effort: string
  waiting: boolean
}

const noFilter: BoardFilter = { q: '', person: '', project: '', effort: '', waiting: false }

export function matchesFilter(t: TaskListItem, f: BoardFilter): boolean {
  if (f.q !== '' && !t.title.toLowerCase().includes(f.q.toLowerCase())) return false
  if (f.person !== '' && t.owner !== f.person) return false
  if (f.project !== '' && t.project !== f.project) return false
  if (f.effort !== '' && (t.latest_run?.effort_mode ?? '') !== f.effort) return false
  if (f.waiting && t.latest_run?.waiting_on_human !== true) return false
  return true
}

export function Board({ me, stream }: { me: string; stream?: EventStream }) {
  const { data, error, stale, reload } = useLive({
    key: '/api/tasks',
    read: () => api.tasks(),
    types: boardEventTypes,
    stream,
  })
  const [notes, setNotes] = useState<PriorityHint[]>([])
  const [filter, setFilter] = useState<BoardFilter>(noFilter)
  const { project: scope } = useProjectScope()

  const all = data?.tasks ?? []
  // The global project scope narrows FIRST (it is the app-wide lens); the
  // board's own filters narrow the rest. Both are presentation over the
  // already-owner-scoped read.
  const scoped = scope === '' ? all : all.filter((t) => t.project === scope)
  const tasks = scoped.filter((t) => matchesFilter(t, filter))
  const queue = ownQueued(tasks, me)
  const answered = data !== null

  const people = useMemo(() => [...new Set(scoped.map((t) => t.owner))].sort(), [scoped])
  const projects = useMemo(() => [...new Set(scoped.map((t) => t.project))].sort(), [scoped])
  const efforts = useMemo(
    () => [...new Set(scoped.map((t) => t.latest_run?.effort_mode ?? '').filter((e) => e !== ''))].sort(),
    [scoped],
  )
  const filtered = filter !== noFilter && tasks.length !== scoped.length

  const onDragEnd = (result: DropResult) => {
    void applyDrag(queue, result).then(
      (answers) => {
        if (answers.length === 0) return
        // The stale-board answer is surfaced, not swallowed: `applied:false`
        // means the work moved on between the render and the drag, and the
        // verb's own `detail` says what the hint can and cannot do. Then the
        // board RE-READS — the server's order is the order.
        setNotes(answers)
        reload()
      },
      (err: unknown) => {
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
    <section className="surface board-surface">
      <Freshness stale={stale} error={error} hasData={data !== null} />

      {/* The toolbar: search + the four honest filters + the door. */}
      <div className="kanban-toolbar">
        <input
          type="search"
          className="k-search"
          placeholder="Search tasks…"
          value={filter.q}
          onChange={(e) => {
            setFilter({ ...filter, q: e.target.value })
          }}
        />
        <select
          className="k-filter"
          value={filter.person}
          aria-label="Filter by person"
          onChange={(e) => {
            setFilter({ ...filter, person: e.target.value })
          }}
        >
          <option value="">Everyone</option>
          {people.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </select>
        {scope === '' && (
          <select
            className="k-filter"
            value={filter.project}
            aria-label="Filter by project"
            onChange={(e) => {
              setFilter({ ...filter, project: e.target.value })
            }}
          >
            <option value="">All projects</option>
            {projects.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        )}
        {efforts.length > 0 && (
          <select
            className="k-filter"
            value={filter.effort}
            aria-label="Filter by effort class"
            onChange={(e) => {
              setFilter({ ...filter, effort: e.target.value })
            }}
          >
            <option value="">Any effort</option>
            {efforts.map((ef) => (
              <option key={ef} value={ef}>
                {ef}
              </option>
            ))}
          </select>
        )}
        <button
          type="button"
          className="k-filter k-toggle"
          data-active={filter.waiting ? 'true' : undefined}
          aria-pressed={filter.waiting}
          onClick={() => {
            setFilter({ ...filter, waiting: !filter.waiting })
          }}
        >
          Waiting on you
        </button>
        {(filtered || filter.q !== '' || filter.waiting) && (
          <button
            type="button"
            className="k-filter"
            onClick={() => {
              setFilter(noFilter)
            }}
          >
            Clear
          </button>
        )}
        <span className="k-count mono">
          {answered ? `${String(tasks.length)}/${String(scoped.length)} tasks` : 'catching up…'}
        </span>
        <button
          type="button"
          className="btn-describe k-describe"
          data-door="describe-goal"
          onClick={() => {
            navigate(scope === '' || scope === '(no project)' ? hrefFor('new') : `${hrefFor('new')}?project=${encodeURIComponent(scope)}`)
          }}
        >
          <Sparkles size={14} strokeWidth={2} aria-hidden="true" />
          Describe a goal
        </button>
      </div>

      {/* The stale-marked block (S15.12): rows stay on screen through a
          re-snapshot — an empty board would be a worse lie — but they are
          MARKED, through the same structural selector every surface uses. */}
      <div className="block m-0" data-stale={stale ? 'true' : 'false'}>
      {/* Your queue: the ONE drag surface. Columns are the machine's state;
          this strip is the only place a drag means anything, and it only
          re-ranks — it never moves work between stages. */}
      <DragDropContext onDragEnd={onDragEnd}>
        {queue.length > 0 && (
          <div className="queue-strip" data-queue-strip>
            <p className="queue-head">
              <span className="queue-title">Your queue</span>
              <span className="queue-sub">
                drag to reorder what runs first — a hint breaks ties among your own queued work; it never moves a card
                to another stage and never reaches another person&apos;s queue
              </span>
            </p>
            {notes.map((n) => (
              <p key={n.task_id} className={`${n.applied ? 'notice' : 'muted'} m-0 text-xs`} data-hint-applied={String(n.applied)}>
                {n.detail}
              </p>
            ))}
            <Droppable droppableId={ownQueueDroppableId}>
              {(dropProvided) => (
                <ul className="queue-lane" ref={dropProvided.innerRef} {...dropProvided.droppableProps}>
                  {queue.map((t, index) => (
                    <Draggable key={t.task_id} draggableId={t.task_id} index={index}>
                      {(dragProvided) => (
                        <li
                          ref={dragProvided.innerRef}
                          {...dragProvided.draggableProps}
                          {...dragProvided.dragHandleProps}
                          className="queue-card"
                        >
                          <span className="queue-rank mono">{String(index + 1)}</span>
                          {/* Same fallback as the cards: a task the platform
                              has not titled names itself by id, never a blank
                              pill (found live 2026-08-11). */}
                          <span className="queue-name">{t.title !== '' ? t.title : t.task_id}</span>
                        </li>
                      )}
                    </Draggable>
                  ))}
                  {dropProvided.placeholder}
                </ul>
              )}
            </Droppable>
          </div>
        )}
      </DragDropContext>

      {/* The board: bounded columns, glowing headers, count pills. The row
          scrolls horizontally as one piece and never wraps. */}
      <div className="kanban-board" data-board>
        {columnsFor(tasks).map((col) => {
          const inCol = tasksInColumn(tasks, col.status)
          return (
            <section className="kanban-col" key={col.status} data-status={col.status} data-known={String(col.known)}>
              <header className="col-header">
                <StatusDot tone={col.tone as never} live={col.status === 'executing' && inCol.length > 0} className="col-dot" />
                <span className="col-title">{col.label}</span>
                <span className="col-count mono">{String(inCol.length)}</span>
              </header>
              {!col.known && (
                <p className="col-note">
                  A stored status the board has no name for — shown under its own value rather than hidden.
                </p>
              )}
              <div className="col-body">
                {inCol.length === 0 && answered && (
                  <p className="col-empty">
                    {col.status === 'intake'
                      ? 'Nothing waits here. "Describe a goal" starts the next task.'
                      : `Nothing is at this stage. Cards arrive here when the work reaches it — never by drag.`}
                  </p>
                )}
                {(scope === '' && groupByProject(inCol).length > 1
                  ? groupByProject(inCol)
                  : [{ project: '', tasks: inCol }]
                ).map((group) => (
                  <div className="col-group" key={group.project}>
                    {group.project !== '' && <p className="col-proj">{group.project}</p>}
                    {group.tasks.map((t) => (
                      <BoardCard key={t.task_id} task={t} showProject={scope === ''} />
                    ))}
                  </div>
                ))}
              </div>
            </section>
          )
        })}
      </div>
      </div>
    </section>
  )
}

/* ── one card ────────────────────────────────────────────────────────────── */

/** The state tone the card's rail carries — from the state the face already
 *  renders, so the rail can say nothing the face does not. */
function railTone(t: TaskListItem): string {
  if (t.kanban_status === cancelledStatus) return 'red'
  const run = t.latest_run
  if (run?.wedged) return 'red'
  if (run?.waiting_on_human) return 'orange'
  if (run?.state === 'parked') return 'yellow'
  if (run?.state === 'running') return 'green'
  return 'accent'
}

/**
 * One board card: the S1.3 face (what · whose · stage · effort · cost ·
 * waiting) in the Nexus card anatomy, the cancelled sign where the store says
 * so, and the D-A expansion underneath. Clicking the face opens the task's
 * own card via the route table; the expand control is its own button, so the
 * two gestures cannot collide.
 */
export function BoardCard({ task, showProject }: { task: TaskListItem; showProject: boolean }) {
  const [open, setOpen] = useState(false)
  const run = task.latest_run
  const cancelled = task.kanban_status === cancelledStatus

  return (
    <article className="task-card" data-task={task.task_id} data-rail={railTone(task)} data-cancelled={cancelled ? 'true' : undefined}>
      <button
        type="button"
        className="task-face"
        onClick={() => {
          navigate(hrefFor('task', { id: task.task_id }))
        }}
      >
        {/* A task the platform has not titled renders its id — never a blank
            face (§42: absence shown, not papered over). */}
        <span className="task-title">{task.title !== '' ? task.title : task.task_id}</span>
        <span className="task-under mono">
          <Owner id={task.owner} />
          {run?.stage !== undefined && run.stage !== '' && <> · {run.stage}</>}
          {run?.effort_mode !== undefined && run.effort_mode !== '' && <> · {run.effort_mode}</>}
          {showProject && task.project !== '' && <> · {task.project}</>}
        </span>
        <span className="task-foot">
          {cancelled && (
            <Chip tone="red" className="task-flag">
              cancelled — open for why
            </Chip>
          )}
          {run?.wedged === true && <Chip tone="red">wedged</Chip>}
          {run?.waiting_on_human === true && !cancelled && (
            <Chip tone="orange" className="task-flag waiting-human">
              waiting on a person
            </Chip>
          )}
          {run?.state === 'parked' && run.waiting_on_human !== true && !cancelled && (
            <span className="task-park">
              <ParkedUntil until={run.parked_until} />
            </span>
          )}
          {run !== null && run !== undefined && (
            <span className="task-cost mono">
              {run.cost_so_far_usd !== null ? <Money usd={run.cost_so_far_usd} /> : <Absent reason="no cost reading" />}
            </span>
          )}
          {run?.downgrade_note !== undefined && run.downgrade_note !== '' && (
            // The disclosed downgrade, as served — the note is the fact, so
            // the words render rather than hiding behind a hover.
            <span className="downgrade-note task-park">{run.downgrade_note}</span>
          )}
        </span>
      </button>
      <button
        type="button"
        className="task-expand"
        aria-expanded={open}
        title={open ? 'Hide the plan' : 'Show the plan steps and stage progress'}
        onClick={() => {
          setOpen(!open)
        }}
      >
        {open ? (
          <ChevronUp size={14} strokeWidth={2} aria-hidden="true" />
        ) : (
          <ChevronDown size={14} strokeWidth={2} aria-hidden="true" />
        )}
        <span>{open ? 'hide' : 'plan'}</span>
      </button>
      {open && <SubItems taskID={task.task_id} cancelled={cancelled} />}
    </article>
  )
}

/**
 * The D-A sub-items, read lazily from the task's own detail on first expand:
 * the plan's numbered steps (each with its "done when") and the recorded
 * stage story with live outcomes. No per-step completion exists on the wire,
 * so none is drawn — the steps are the plan, the stage records are the
 * progress. For a cancelled card the recorded human decisions say WHY.
 */
function SubItems({ taskID, cancelled }: { taskID: string; cancelled: boolean }) {
  const { data, error } = useLive<TaskDetail>({
    key: `/api/tasks/${taskID}#card`,
    read: () => api.task(taskID),
    types: boardEventTypes,
  })

  if (error !== '' && data === null) {
    return <p className="sub-note">The task&apos;s detail did not answer — open the card for the full story.</p>
  }
  if (data === null) return <p className="sub-note">reading the plan…</p>

  const steps = data.plan?.steps ?? []
  const progress = data.stage_progress
  const decisions = data.decisions

  return (
    <div className="task-sub" data-sub={taskID}>
      {cancelled && (
        <div className="sub-block">
          <p className="sub-head">Why it is cancelled</p>
          {decisions.length === 0 ? (
            <p className="sub-note">No decision row was recorded — open the card for the trail.</p>
          ) : (
            decisions.slice(-2).map((d) => (
              <p className="sub-note" key={d.seq}>
                <Owner id={d.actor} /> — {d.decision}
                {d.reason !== undefined && d.reason !== '' && <> · {d.reason}</>}
              </p>
            ))
          )}
        </div>
      )}
      <div className="sub-block">
        <p className="sub-head">The plan&apos;s steps</p>
        {data.plan === null ? (
          // Plain words for the absence — never the internal reason string
          // (the raw-error class the rework bans), and no phase claim either.
          <p className="sub-note">No drafted plan is stored for this task — the full story is on its card.</p>
        ) : steps.length === 0 ? (
          <p className="sub-note">The plan has no numbered steps.</p>
        ) : (
          <ol className="sub-steps">
            {steps.map((s) => (
              <li key={s.id}>
                <span className="sub-key mono">{s.id}</span> {s.title}
              </li>
            ))}
          </ol>
        )}
      </div>
      <div className="sub-block">
        <p className="sub-head">Stage progress — recorded, live</p>
        {progress.length === 0 ? (
          <p className="sub-note">No stage boundary is recorded yet.</p>
        ) : (
          <ul className="sub-stages">
            {progress.slice(-4).map((s) => (
              <li key={`${s.run_id}:${String(s.seq)}`}>
                <StatusDot
                  tone={s.outcome === 'error' ? 'red' : s.outcome === 'split' ? 'blue' : 'green'}
                  className="sub-dot"
                />
                <span className="mono">{s.stage === '' ? '(unnamed)' : s.stage}</span>
                {s.outcome !== undefined && s.outcome !== '' && <span className="sub-note"> · {s.outcome}</span>}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
