import { useEffect, useMemo, useState } from 'react'
import { BookOpen, FolderOpen, FolderPlus, Inbox as InboxIcon, Sparkles, Terminal } from 'lucide-react'

import {
  ApiError,
  api,
  type Deliverable,
  type ProjectCommands,
  type ProjectDetail,
  type ProjectListItem,
  type ProjectStarted,
  type TaskListItem,
} from './api'
import type { EventStream } from './events'
import { columnsFor } from './kanban'
import { boardEventTypes, describeError, useLive } from './live'
import { Freshness, Money, Owner } from './parts'
import { useProjectScope } from './project'
import { hrefForInbox } from './Inbox'
import { Link, navigate } from './router'
import { hrefFor } from './routes'
import { Button, Chip, EmptyState, Modal, Timestamp, type Tone } from './ui'

/**
 * Projects (map §3 v3): work lives in projects — every project as a card,
 * "(no project)" as its own real bucket, the give-work door straight into a
 * project, and the create/onboard door.
 *
 * WHAT THE CARDS ARE MADE OF — two reads, merged by project id:
 *
 *  - THE REGISTRY (`/api/projects`, P3-RW-2): the S13.7 entry — ACTIVE or
 *    PENDING, branch, remote PRESENCE (never a URL), and the capture summary
 *    of what the platform injects into every run in the project. The served
 *    `visibility` sentence renders verbatim; the full record — conventions,
 *    commands, danger zones, protected refs — is each card's own door onto
 *    `/api/projects/{id}`.
 *  - TASK-DERIVED AGGREGATES over the caller's own served rows (map §5:
 *    counts and spend stay client-side): active tasks by stage in the board's
 *    own column words, the waiting-on-you count, the costliest served cost
 *    reading — labelled as exactly that arithmetic — and recent deliverables.
 *
 * A registry entry with no tasks is still a card (a fresh registration is
 * exactly that), and a task bucket with no visible entry says so rather than
 * inventing a status. Describe-a-goal pins only into an entry the registry
 * shows as ACTIVE — a pin into anything else is refused server-side (P3-RW-1),
 * so no button is rendered that would only exist to fail.
 *
 * THE CREATE DOOR (S13.7 through P3-RW-2's POST): register → clone → scan →
 * draft → the OWNER's approval card in the Inbox → active. The form says the
 * whole journey up front, and the landed answer renders the platform's own
 * sentence, the onboarding task and the approval ref — not this client's
 * paraphrase.
 *
 * THE COMMANDS DOOR (P3-GF6, the r4-F1b fix): each card carries a "Commands"
 * door onto the project's captured build/test/lint/run/preview set — the
 * OWNER edits it (a full-replacement write through the P3-GF5 route), a
 * member reads it with an honest sentence naming who can edit, and a pending
 * entry points at its real door, the onboarding card. `?commands=<id>` is the
 * door's deep-linkable address, which is what gives the bootstrap-checked
 * deliverable's card a destination (`hrefForProjectCommands`).
 */
export function Projects({ me, stream, search = '' }: { me: string; stream?: EventStream; search?: string }) {
  const tasks = useLive({
    key: `/api/tasks#projects:${me}`,
    read: () => api.tasks(),
    types: boardEventTypes,
    stream,
  })
  const deliverables = useLive({
    key: `/api/deliverables#projects:${me}`,
    read: () => api.deliverables(),
    types: boardEventTypes,
    stream,
  })
  // The registry moves through the onboarding TASK's own lifecycle — there is
  // no project.* frame on the wire — so the board's types are the honest
  // refresh triggers: intake.state and decision.recorded are what carry a
  // registration and its approval. The create door reloads explicitly too.
  const registry = useLive({
    key: `/api/projects#registry:${me}`,
    read: () => api.projects(),
    types: boardEventTypes,
    stream,
  })
  const [creating, setCreating] = useState(false)
  // The Commands door has two openers: a card's own button, and the
  // deep-linkable `?commands=<id>` address the bootstrap-checked deliverable
  // links to. Closing a URL-opened editor also clears the address (replace,
  // not push — Back should leave the surface, not re-open the modal).
  const urlCommandsFor = new URLSearchParams(search).get('commands') ?? ''
  const [localCommandsFor, setLocalCommandsFor] = useState('')
  const commandsFor = localCommandsFor !== '' ? localCommandsFor : urlCommandsFor
  const closeCommands = () => {
    setLocalCommandsFor('')
    if (urlCommandsFor !== '') navigate(hrefFor('projects'), { replace: true })
  }

  const buckets = useMemo(
    () => projectBuckets(tasks.data?.tasks ?? [], deliverables.data?.deliverables ?? []),
    [tasks.data, deliverables.data],
  )
  const cards = useMemo(() => mergeCards(buckets, registry.data?.projects ?? []), [buckets, registry.data])
  const registryAnswered = registry.data !== null
  const answered = tasks.data !== null && registryAnswered

  return (
    <section className="surface">
      <Freshness
        stale={tasks.stale || registry.stale}
        error={[tasks.error, registry.error].filter((e) => e !== '').join(' ')}
        hasData={tasks.data !== null || registryAnswered}
      />
      <div className="proj-head">
        <p className="proj-intro">
          Work lives in projects: a task pinned to one is planned against the project&apos;s accumulated work, its
          conventions and its memory. Opening a project scopes the whole app to it. Each card carries the project&apos;s
          registry record — its state and what the platform captured about it — beside the work in flight.
        </p>
        <Button
          variant="primary"
          data-door="new-project"
          onClick={() => {
            setCreating(true)
          }}
        >
          <FolderPlus size={15} strokeWidth={2} aria-hidden="true" />
          New project
        </Button>
      </div>

      {answered && cards.length === 0 && (
        <EmptyState
          what="No projects yet."
          why='"New project" registers one with the platform; "Describe a goal" starts work without one.'
          action={
            <Button
              variant="secondary"
              onClick={() => {
                navigate(hrefFor('new'))
              }}
            >
              Describe a goal
            </Button>
          }
        />
      )}

      <div className="proj-grid">
        {cards.map((c) => (
          <ProjectCard
            key={c.name}
            card={c}
            registryAnswered={registryAnswered}
            onCommands={(id) => {
              setLocalCommandsFor(id)
            }}
          />
        ))}
      </div>

      {registry.data !== null && (
        <p className="proj-visibility">
          {registry.data.visibility}
          {registry.data.truncated && <> The list is truncated at the served page — not every entry is shown.</>}
        </p>
      )}

      <CreateProject open={creating} onOpenChange={setCreating} onLanded={registry.reload} />
      {commandsFor !== '' && (
        <CommandsEditor projectID={commandsFor} me={me} onClose={closeCommands} onLanded={registry.reload} />
      )}
    </section>
  )
}

/** The Commands editor's deep-linkable address — what the bootstrap-checked
 *  deliverable's card links to (P3-GF6): /projects with the editor open on
 *  this project. */
export function hrefForProjectCommands(projectID: string): string {
  return `${hrefFor('projects')}?commands=${encodeURIComponent(projectID)}`
}

/* ── the aggregates (client-side over served rows, map §5) ────────────────── */

type Bucket = {
  name: string
  tasks: TaskListItem[]
  /** Column label → count, in board order, zeros dropped. */
  byStage: { label: string; count: number; tone: string }[]
  waiting: number
  /**
   * Spend, WITHOUT arithmetic (§37: money is read, never computed — the scan
   * enforces it and the ParkedFoot earliest-horizon pick is the precedent):
   * the COSTLIEST served reading among the project's latest runs, picked, and
   * the counts beside it. A summed project total is a figure nobody served —
   * it appears when a project-spend view exists on the wire, not before.
   */
  costliest: { usd: number; title: string } | null
  pricedRuns: number
  unreadRuns: number
  recent: Deliverable[]
}

/** The bucket a deliverable files under: its own project when the wire stamps
 *  one, else ITS TASK's project — the deliverable names its task, the task list
 *  names the project, and that join is exactly the client-side aggregation this
 *  surface already owns (map §5). Only a deliverable whose task is also
 *  unpinned degrades to '(no project)'; an absent field never throws the name
 *  sort (operator, 2026-08-12). */
function bucketOf(d: Deliverable, taskProject: Map<string, string>): string {
  if (d.project_id) return d.project_id
  const viaTask = taskProject.get(d.task_id)
  return viaTask !== undefined && viaTask !== '' ? viaTask : '(no project)'
}

export function projectBuckets(tasks: TaskListItem[], deliverables: Deliverable[]): Bucket[] {
  const names = new Map<string, TaskListItem[]>()
  const taskProject = new Map<string, string>()
  for (const t of tasks) {
    taskProject.set(t.task_id, t.project)
    const list = names.get(t.project)
    if (list) list.push(t)
    else names.set(t.project, [t])
  }
  // Deliverables can name a project no task currently shows — keep the bucket.
  for (const d of deliverables) {
    const key = bucketOf(d, taskProject)
    if (!names.has(key)) names.set(key, [])
  }

  const buckets: Bucket[] = []
  for (const [name, list] of names) {
    const cols = columnsFor(list)
    const byStage = cols
      .map((c) => ({
        label: c.label,
        tone: c.tone,
        count: list.filter((t) => t.kanban_status === c.status || (c.status === 'intake' && t.kanban_status === 'cancelled'))
          .length,
      }))
      .filter((s) => s.count > 0)
    let costliest: { usd: number; title: string } | null = null
    let priced = 0
    let unread = 0
    for (const t of list) {
      const c = t.latest_run?.cost_so_far_usd
      if (c === null || c === undefined) {
        if (t.latest_run) unread += 1
      } else {
        priced += 1
        if (costliest === null || c > costliest.usd) {
          costliest = { usd: c, title: t.title !== '' ? t.title : t.task_id }
        }
      }
    }
    const recent = deliverables
      .filter((d) => bucketOf(d, taskProject) === name)
      .sort((a, b) => b.updated_ts.localeCompare(a.updated_ts))
      .slice(0, 3)
    buckets.push({
      name,
      tasks: list,
      byStage,
      waiting: list.filter((t) => t.latest_run?.waiting_on_human === true).length,
      costliest,
      pricedRuns: priced,
      unreadRuns: unread,
      recent,
    })
  }
  // "(no project)" sinks to the end; the rest alphabetical.
  return buckets.sort((a, b) =>
    a.name === '(no project)' ? 1 : b.name === '(no project)' ? -1 : a.name.localeCompare(b.name),
  )
}

/* ── the merge: registry entry ⋈ task bucket, by project id ──────────────── */

type CardData = {
  /** The project id — the bucket name and the registry key are the same id space (P3-RW-1's pin). */
  name: string
  entry: ProjectListItem | null
  bucket: Bucket | null
}

export function mergeCards(buckets: Bucket[], entries: ProjectListItem[]): CardData[] {
  const byName = new Map<string, CardData>()
  for (const b of buckets) byName.set(b.name, { name: b.name, entry: null, bucket: b })
  for (const e of entries) {
    const found = byName.get(e.project_id)
    if (found) found.entry = e
    else byName.set(e.project_id, { name: e.project_id, entry: e, bucket: null })
  }
  return [...byName.values()].sort((a, b) =>
    a.name === '(no project)' ? 1 : b.name === '(no project)' ? -1 : a.name.localeCompare(b.name),
  )
}

/** The registry state's chip, forward-tolerant like the board's columns: a
 *  state this list does not know renders under its own name, never coerced. */
function stateChip(state: string): { tone: Tone; label: string } {
  if (state === 'active') return { tone: 'green', label: 'active' }
  if (state === 'pending') return { tone: 'orange', label: 'pending approval' }
  return { tone: 'pink', label: state === '' ? '(no state recorded)' : state }
}

/* ── one card ────────────────────────────────────────────────────────────── */

function ProjectCard({
  card,
  registryAnswered,
  onCommands,
}: {
  card: CardData
  registryAnswered: boolean
  onCommands: (id: string) => void
}) {
  const { setProject } = useProjectScope()
  const { name, entry, bucket } = card
  const noProject = name === '(no project)'
  const [recordOpen, setRecordOpen] = useState(false)

  return (
    <article className="proj-card" data-project={name}>
      <header className="proj-card-head">
        <FolderOpen size={17} strokeWidth={1.8} aria-hidden="true" className="proj-ico" />
        <h3 className="proj-name">{entry !== null && entry.name !== '' ? entry.name : name}</h3>
        {/* F7 (drain r1): the chip counts TASKS whose latest run waits on a
            person (this read's own derivation) — the inbox slice counts CARDS,
            and one task can hold several. The unit is said so the two numbers
            stop reading as a bug; nothing is recomputed. */}
        {bucket !== null && bucket.waiting > 0 && (
          <Chip tone="orange" className="waiting-human">
            {String(bucket.waiting)} task{bucket.waiting === 1 ? '' : 's'} waiting on you
          </Chip>
        )}
        {entry !== null && <Chip tone={stateChip(entry.state).tone}>{stateChip(entry.state).label}</Chip>}
      </header>

      {noProject && (
        <p className="proj-sub">
          The honest bucket: tasks submitted without a project pin. They plan standalone — nothing from any
          project&apos;s world is injected.
        </p>
      )}

      {entry !== null && (
        <p className="proj-reg" data-reg={entry.project_id}>
          <span>
            id <b className="mono">{entry.project_id}</b>
          </span>
          <span>
            branch <b className="mono">{entry.default_branch === '' ? '(none recorded)' : entry.default_branch}</b>
          </span>
          <span>{entry.has_remote ? 'remote attached' : 'no remote — local store only'}</span>
          <span>
            <Owner id={entry.owner} />
            {entry.members.length > 0 && (
              <span className="proj-quiet">
                {' '}
                +{String(entry.members.length)} member{entry.members.length === 1 ? '' : 's'}
              </span>
            )}
          </span>
          <span>
            capture v{String(entry.capture.version)} · {String(entry.capture.conventions)} convention
            {entry.capture.conventions === 1 ? '' : 's'} · {String(entry.capture.danger_zones)} danger zone
            {entry.capture.danger_zones === 1 ? '' : 's'}
            {entry.capture.test_command !== undefined && entry.capture.test_command !== '' && (
              <>
                {' '}
                · tests: <b className="mono">{entry.capture.test_command}</b>
              </>
            )}
          </span>
        </p>
      )}

      {!noProject && entry === null && registryAnswered && (
        <p className="proj-sub">
          Not in your project registry — its record is not visible to you, and new tasks cannot pin to it.
        </p>
      )}

      <div className="proj-stages">
        {bucket === null || bucket.byStage.length === 0 ? (
          <span className="proj-quiet">no tasks right now</span>
        ) : (
          bucket.byStage.map((s) => (
            <span className="proj-stage" key={s.label}>
              <i className="proj-stage-dot" style={{ background: `var(--${s.tone})` }} aria-hidden="true" />
              {s.label} <b className="mono">{String(s.count)}</b>
            </span>
          ))
        )}
      </div>

      {bucket !== null && (
        <p className="proj-spend">
          {bucket.costliest !== null ? (
            <>
              <Money usd={bucket.costliest.usd} />{' '}
              <span className="proj-quiet">
                the costliest run so far ({bucket.costliest.title}) · {String(bucket.pricedRuns)} run
                {bucket.pricedRuns === 1 ? '' : 's'} carr{bucket.pricedRuns === 1 ? 'ies' : 'y'} a cost reading
                {bucket.unreadRuns > 0 && <> · {String(bucket.unreadRuns)} without one</>} — per-run figures are on each
                card
              </span>
            </>
          ) : (
            <span className="proj-quiet">no run has a cost reading yet</span>
          )}
        </p>
      )}

      {bucket !== null && bucket.recent.length > 0 && (
        <div className="proj-recent">
          <p className="proj-recent-head">Recent deliverables</p>
          {bucket.recent.map((d) => (
            <p className="proj-deliv" key={d.deliverable_id}>
              <Link to={hrefFor('deliverable', { id: d.deliverable_id })}>{d.deliverable_id}</Link>
              <span className="proj-quiet">
                {' '}
                {d.type} · r{String(d.current_revision)} · {d.state} · <Owner id={d.owner} /> ·{' '}
                <Timestamp ts={d.updated_ts} variant="live" />
              </span>
            </p>
          ))}
        </div>
      )}

      <div className="proj-acts">
        <Button
          variant="secondary"
          size="sm"
          data-proj-open={name}
          onClick={() => {
            setProject(name)
            navigate(hrefFor('board'))
          }}
        >
          Open — scope the app to it
        </Button>
        {/* The gate-ordered jump (findings 2026-08-22, requirement 1): a
            project reaches ITS OWN inbox slice by URL. The address is the
            inbox's deep-linkable filter, so it survives a bookmark; an empty
            slice answers honestly over there rather than hiding the door. */}
        <Button
          variant="secondary"
          size="sm"
          data-proj-inbox={name}
          onClick={() => {
            navigate(hrefForInbox({ project: name, task: '', sort: '' }))
          }}
        >
          <InboxIcon size={13} strokeWidth={2} aria-hidden="true" />
          Its inbox cards
        </Button>
        {entry !== null && (
          <Button
            variant="secondary"
            size="sm"
            data-proj-record={entry.project_id}
            onClick={() => {
              setRecordOpen(true)
            }}
          >
            <BookOpen size={13} strokeWidth={2} aria-hidden="true" />
            Project record
          </Button>
        )}
        {/* The Commands door (P3-GF6). Rendered for every registry entry, in
            every state — the modal itself answers honestly for a member (read
            with the owner named) and for a pending draft (its real door is the
            onboarding card), so the deep link from a bootstrap-checked
            deliverable always has a destination that explains itself. */}
        {entry !== null && (
          <Button
            variant="secondary"
            size="sm"
            data-proj-commands={entry.project_id}
            onClick={() => {
              onCommands(entry.project_id)
            }}
          >
            <Terminal size={13} strokeWidth={2} aria-hidden="true" />
            Commands
          </Button>
        )}
        {entry !== null && entry.state === 'active' && (
          <Button
            variant="primary"
            size="sm"
            data-proj-describe={name}
            onClick={() => {
              navigate(`${hrefFor('new')}?project=${encodeURIComponent(name)}`)
            }}
          >
            <Sparkles size={13} strokeWidth={2} aria-hidden="true" />
            Describe a goal
          </Button>
        )}
        {entry !== null && entry.state === 'pending' && (
          <span className="proj-quiet">
            pending — the owner&apos;s approval card in the Inbox activates it; tasks can pin to it once active
          </span>
        )}
      </div>

      {entry !== null && <ProjectRecord entry={entry} open={recordOpen} onOpenChange={setRecordOpen} />}
    </article>
  )
}

/* ── the project record (the S13.7 capture, whole, via GET /api/projects/{id}) ── */

function ProjectRecord({
  entry,
  open,
  onOpenChange,
}: {
  entry: ProjectListItem
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  const [detail, setDetail] = useState<ProjectDetail | null>(null)
  const [refusal, setRefusal] = useState('')

  useEffect(() => {
    if (!open) return
    let mounted = true
    setDetail(null)
    setRefusal('')
    api.project(entry.project_id).then(
      (r) => {
        if (mounted) setDetail(r.project)
      },
      (err: unknown) => {
        if (mounted) setRefusal(describeError(err))
      },
    )
    return () => {
      mounted = false
    }
  }, [open, entry.project_id])

  const cap = detail?.capture

  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={entry.name !== '' ? entry.name : entry.project_id}
      description="The project record: what the platform captured about this project, injected into every run in it."
      footer={
        <Button variant="ghost" onClick={() => { onOpenChange(false) }}>
          Close
        </Button>
      }
    >
      {refusal !== '' && (
        <div className="door-refusal" role="alert">
          <p className="refusal-detail">{refusal}</p>
        </div>
      )}
      {refusal === '' && detail === null && <p className="proj-quiet m-0">Reading the project record…</p>}
      {detail !== null && cap !== undefined && (
        <div data-record={detail.project_id}>
          <div className="rec-sec">
            <p className="rec-facts">
              <Chip tone={stateChip(detail.state).tone}>{stateChip(detail.state).label}</Chip>
              <span>
                id <b className="mono">{detail.project_id}</b>
              </span>
              <span>
                branch <b className="mono">{detail.default_branch === '' ? '(none recorded)' : detail.default_branch}</b>
              </span>
              <span>{detail.has_remote ? 'remote attached' : 'no remote — local store only'}</span>
              <span>
                owner <Owner id={detail.owner} />
              </span>
              <span>
                members:{' '}
                {(detail.members ?? []).length === 0 ? (
                  'none besides the owner'
                ) : (
                  (detail.members ?? []).map((m, i) => (
                    <span key={m}>
                      {i > 0 && ', '}
                      <Owner id={m} />
                    </span>
                  ))
                )}
              </span>
              <span>
                registered <Timestamp ts={detail.created_ts} />
              </span>
            </p>
          </div>

          <div className="rec-sec">
            <p className="rec-head">Protected refs — accepts never push here</p>
            {(detail.protected_refs ?? []).length === 0 ? (
              <p className="proj-quiet m-0">none recorded</p>
            ) : (
              <ul className="rec-list">
                {(detail.protected_refs ?? []).map((r) => (
                  <li key={r} className="mono">
                    {r}
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="rec-sec">
            <p className="rec-head">
              Capture v{String(cap.version)}
              {cap.captured_by !== undefined && cap.captured_by !== '' && <> · by {cap.captured_by}</>}
            </p>
            {cap.captured_ts !== undefined && cap.captured_ts !== '' && (
              <p className="proj-quiet m-0 mb-2">
                captured <Timestamp ts={cap.captured_ts} />
                {cap.scan_hash !== undefined && cap.scan_hash !== '' && (
                  <>
                    {' '}
                    · scan <span className="mono">{cap.scan_hash}</span>
                  </>
                )}
              </p>
            )}

            <p className="rec-sub">Conventions — the house rules every run reads</p>
            {(cap.conventions ?? []).length === 0 ? (
              <p className="proj-quiet m-0">the scan recorded no conventions</p>
            ) : (
              <ul className="rec-list">
                {(cap.conventions ?? []).map((c) => (
                  <li key={c}>{c}</li>
                ))}
              </ul>
            )}

            <p className="rec-sub">Commands</p>
            {Object.values(cap.commands).every((v) => v === undefined || v === '') ? (
              <p className="proj-quiet m-0">no commands captured</p>
            ) : (
              <dl className="rec-cmd">
                {(['build', 'test', 'lint', 'run', 'preview'] as const).map((k) =>
                  cap.commands[k] !== undefined && cap.commands[k] !== '' ? (
                    <div key={k} className="rec-cmd-row">
                      <dt>{k}</dt>
                      <dd className="mono">{cap.commands[k]}</dd>
                    </div>
                  ) : null,
                )}
              </dl>
            )}

            <p className="rec-sub">Danger zones — paths the platform treats as hazardous</p>
            {(cap.danger_zones ?? []).length === 0 ? (
              <p className="proj-quiet m-0">no danger zones recorded</p>
            ) : (
              (cap.danger_zones ?? []).map((z) => (
                <p key={z.path} className="rec-zone">
                  <b className="mono">{z.path}</b>
                  {z.action !== undefined && z.action !== '' && <> · {z.action}</>}
                  <span className="proj-quiet"> — {z.rule}</span>
                </p>
              ))
            )}
          </div>
        </div>
      )}
    </Modal>
  )
}

/* ── the Commands editor (P3-GF6: the r4-F1b door, via P3-GF5's POST) ────── */

/** The wire vocabulary of the captured set, in the record modal's own order. */
const commandSlots = ['build', 'test', 'lint', 'run', 'preview'] as const
type CommandSlot = (typeof commandSlots)[number]

/** What each slot is, in the reader's words — one line, no jargon. */
const slotHelp: Record<CommandSlot, string> = {
  build: 'How the project compiles or bundles — e.g. go build ./... or npm run build',
  test: 'How its tests run — e.g. go test ./... or npm test',
  lint: 'Its style and static checks — e.g. gofmt -l . or npm run lint',
  run: 'How the finished thing starts',
  preview: 'How to start it for a look — a dev server, for example',
}

function hasChecks(c: ProjectCommands): boolean {
  return [c.build, c.test, c.lint].some((v) => v !== undefined && v.trim() !== '')
}

function hasAny(c: ProjectCommands): boolean {
  return commandSlots.some((k) => c[k] !== undefined && c[k].trim() !== '')
}

/**
 * The Commands editor: the owner of an ACTIVE project edits its five captured
 * command slots and saves the set whole (full replacement on the wire — the
 * boxes ARE the set, and all-empty is the explicit clear back to the
 * bootstrap posture, said out loud before the press).
 *
 * Anyone else gets the honest read: a member sees the captured set read-only
 * with a sentence naming the owner; a pending draft points at its real door
 * (the onboarding card in the Inbox); the not-signed-in dev posture is told
 * signing in is what makes a capture attributable. Nothing here renders a
 * control that would only exist to fail — and every refusal the server can
 * still answer (state moved under us) renders in its own words.
 *
 * Nothing executes on save: a captured command runs only inside the
 * verification sandbox, on the next judged round — the body says so, because
 * "the platform will run what I type" is exactly the kind of thing a person
 * should not have to guess about.
 */
function CommandsEditor({
  projectID,
  me,
  onClose,
  onLanded,
}: {
  projectID: string
  me: string
  onClose: () => void
  onLanded?: () => void
}) {
  const [detail, setDetail] = useState<ProjectDetail | null>(null)
  const [readRefusal, setReadRefusal] = useState('')
  const [slots, setSlots] = useState<Record<CommandSlot, string>>({ build: '', test: '', lint: '', run: '', preview: '' })
  const [busy, setBusy] = useState(false)
  const [refusal, setRefusal] = useState<{ kind: 'sign-in' | 'plain'; text: string } | null>(null)
  const [landed, setLanded] = useState('')

  useEffect(() => {
    let mounted = true
    setDetail(null)
    setReadRefusal('')
    setRefusal(null)
    setLanded('')
    api.project(projectID).then(
      (r) => {
        if (!mounted) return
        setDetail(r.project)
        const c = r.project.capture.commands
        setSlots({
          build: c.build ?? '',
          test: c.test ?? '',
          lint: c.lint ?? '',
          run: c.run ?? '',
          preview: c.preview ?? '',
        })
      },
      (err: unknown) => {
        if (!mounted) return
        if (err instanceof ApiError && err.status === 404) {
          setReadRefusal(
            `No project "${projectID}" is visible to you — either it does not exist, or its record is someone else's. Nothing here can show or edit its commands.`,
          )
        } else {
          setReadRefusal(describeError(err))
        }
      },
    )
    return () => {
      mounted = false
    }
  }, [projectID])

  const cap = detail?.capture
  const stored: ProjectCommands = cap?.commands ?? {}
  const signedOut = me === ''
  const owner = detail?.owner ?? ''
  const mine = !signedOut && owner === me
  const active = detail?.state === 'active'
  const editable = detail !== null && mine && active

  /** The outgoing set: the boxes as they stand, blanks left out — the wire's
   *  full-replacement unit, where {} is the sanctioned explicit clear. */
  const outgoing = (): ProjectCommands => {
    const out: ProjectCommands = {}
    for (const k of commandSlots) {
      const v = slots[k].trim()
      if (v !== '') out[k] = v
    }
    return out
  }
  const clearing = detail !== null && !hasAny(outgoing()) && hasAny(stored)

  const save = () => {
    if (busy || !editable) return
    setBusy(true)
    setRefusal(null)
    setLanded('')
    api.setProjectCommands(projectID, outgoing()).then(
      (r) => {
        setBusy(false)
        setDetail(r.project)
        const c = r.project.capture.commands
        setSlots({
          build: c.build ?? '',
          test: c.test ?? '',
          lint: c.lint ?? '',
          run: c.run ?? '',
          preview: c.preview ?? '',
        })
        // The sentence is the platform's own — captured as version N, cleared
        // back to bootstrap, or nothing changed — never re-told here.
        setLanded(r.detail)
        onLanded?.()
      },
      (err: unknown) => {
        setBusy(false)
        if (err instanceof ApiError && err.code === 'dev_identity') {
          setRefusal({ kind: 'sign-in', text: '' })
        } else if (err instanceof ApiError) {
          // The server's refusals here are person-shaped sentences (who owns
          // it, where the pending draft's real door is, what a bad body
          // needed) — render them as said.
          setRefusal({ kind: 'plain', text: err.message })
        } else {
          setRefusal({ kind: 'plain', text: describeError(err) })
        }
      },
    )
  }

  return (
    <Modal
      open
      onOpenChange={(o) => {
        if (!o) onClose()
      }}
      // The landed sentence and a long captured set make this taller than a
      // short viewport; without its own scroll the fixed-centred popup would
      // clip the act row off-screen (seen live, GF6 walk).
      className="max-h-[86vh] overflow-y-auto"
      title={`Commands — ${detail !== null && detail.name !== '' ? detail.name : projectID}`}
      description="What the platform runs to check work in this project. Captured here as data — nothing runs when you save; a command executes only inside the verification sandbox, on the next round."
      footer={
        editable ? (
          <>
            <Button variant="primary" disabled={busy} aria-busy={busy} data-commands-save onClick={save}>
              {busy ? 'Capturing…' : clearing ? 'Clear every command' : 'Capture — replace the set'}
            </Button>
            <Button variant="ghost" disabled={busy} onClick={onClose}>
              Close
            </Button>
          </>
        ) : (
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
        )
      }
    >
      {readRefusal !== '' && (
        <div className="door-refusal" role="alert">
          <p className="refusal-detail">{readRefusal}</p>
        </div>
      )}
      {readRefusal === '' && detail === null && <p className="proj-quiet m-0">Reading the project record…</p>}

      {detail !== null && cap !== undefined && (
        <div data-commands-editor={detail.project_id} data-commands-mode={editable ? 'edit' : 'read'}>
          <p className="rec-facts">
            <Chip tone={stateChip(detail.state).tone}>{stateChip(detail.state).label}</Chip>
            <span>
              capture v{String(cap.version)}
              {cap.captured_by !== undefined && cap.captured_by !== '' && (
                <>
                  {' '}
                  · by <Owner id={cap.captured_by} />
                </>
              )}
            </span>
            {cap.captured_ts !== undefined && cap.captured_ts !== '' && (
              <span>
                <Timestamp ts={cap.captured_ts} />
              </span>
            )}
            <span>
              owner <Owner id={detail.owner} />
            </span>
          </p>

          {/* The honest posture, from the STORED capture — what is true now.
              Only for an ACTIVE entry: a pending draft has no runnable work,
              so "work in this project is checked in bootstrap mode" would be
              a claim about rounds that cannot happen yet (seen live, GF6
              walk — the pending face said it beside the onboarding pointer). */}
          {detail.state !== 'active' ? null : hasChecks(stored) ? (
            <p className="cmd-posture" data-cmd-posture="checked">
              Verification runs the captured build, test and lint below as its checks on every round of work in this
              project.
            </p>
          ) : (
            <p className="cmd-posture cmd-posture-bootstrap" data-cmd-posture="bootstrap">
              No check commands are captured, so work in this project is checked in <b>bootstrap mode</b>: the platform
              runs no build, test or lint of its own, its verdict is advisory, and the requester&apos;s review is what
              decides. Capturing a build, test or lint command switches the next round to real checks.
            </p>
          )}

          {/* ── who may edit, said before any control (or its absence) ── */}
          {signedOut ? (
            <p className="cmd-who" data-cmd-why="signed-out">
              Setting commands is a person&apos;s act, and you are browsing without being signed in — the captured set is
              shown read-only. <Link to={hrefFor('login')}>Sign in</Link> as the owner (<Owner id={detail.owner} />) to
              edit it.
            </p>
          ) : detail.state === 'pending' && mine ? (
            <div className="cmd-who" data-cmd-why="pending">
              <p className="m-0">
                This project is still waiting for your approval, so its drafted commands are edited on the onboarding
                card in your Inbox — answering that card captures the draft you approve and turns the project active.
                This editor serves an active project.
              </p>
              <Button
                variant="secondary"
                size="sm"
                className="mt-2"
                onClick={() => {
                  navigate(hrefForInbox({ project: projectID, task: '', sort: '' }))
                }}
              >
                <InboxIcon size={13} strokeWidth={2} aria-hidden="true" />
                To its onboarding card
              </Button>
            </div>
          ) : !mine ? (
            <p className="cmd-who" data-cmd-why="not-owner">
              Only the project&apos;s owner edits these, and this project is owned by <Owner id={detail.owner} /> — what
              they capture here is what the platform runs to check everybody&apos;s work in it. The set is shown
              read-only; ask them for a change.
            </p>
          ) : !active ? (
            <p className="cmd-who" data-cmd-why="not-active">
              This project&apos;s record is in state &quot;{detail.state}&quot;, and commands are edited on an active
              project — the set is shown read-only.
            </p>
          ) : null}

          {/* ── the set: five slots, editable or honestly read-only ── */}
          {editable ? (
            <div className="flex flex-col gap-3 mt-3">
              {commandSlots.map((k) => (
                <label className="door-field m-0" key={k}>
                  <span className="door-label">
                    {k} <span className="door-optional">{slotHelp[k]}</span>
                  </span>
                  <input
                    className="door-input mono"
                    type="text"
                    value={slots[k]}
                    data-cmd-slot={k}
                    onChange={(e) => {
                      setSlots({ ...slots, [k]: e.target.value })
                    }}
                  />
                </label>
              ))}
              <p className="cmd-note m-0">
                Build, test and lint are what checking runs; run and preview are how the platform starts the project
                when something needs it running. Each is one line, run inside the project&apos;s own sandboxed copy.
                Saving replaces the whole set — an emptied box is an emptied slot.
              </p>
            </div>
          ) : hasAny(stored) ? (
            <dl className="rec-cmd mt-3">
              {commandSlots.map((k) =>
                stored[k] !== undefined && stored[k] !== '' ? (
                  <div key={k} className="rec-cmd-row">
                    <dt>{k}</dt>
                    <dd className="mono">{stored[k]}</dd>
                  </div>
                ) : null,
              )}
            </dl>
          ) : (
            <p className="proj-quiet mt-3 mb-0">no commands captured</p>
          )}

          {/* The clear is loud BEFORE the press, not after: all boxes empty
              while something is captured means the primary act erases the set. */}
          {editable && clearing && (
            <p className="cmd-clear-warning" role="status" data-cmd-clearing>
              Every box is empty, so saving now removes the whole captured set. From the next round on, work in this
              project is checked in bootstrap mode — no build, test or lint of the platform&apos;s own, advisory
              verdicts, the requester&apos;s review deciding — until commands are captured again.
            </p>
          )}

          {landed !== '' && (
            <div className="door-saved" role="status" data-commands-landed>
              {/* The platform's own sentence about what the write did. */}
              <p className="saved-detail">{landed}</p>
            </div>
          )}
          {refusal !== null &&
            (refusal.kind === 'sign-in' ? (
              <div className="door-refusal" role="alert">
                <p className="refusal-detail">
                  Setting commands is a person&apos;s act, and you are browsing without being signed in — nothing was
                  captured. <Link to={hrefFor('login')}>Sign in</Link> as the owner, then capture them again.
                </p>
              </div>
            ) : (
              <div className="door-refusal" role="alert">
                <p className="refusal-detail">{refusal.text}</p>
              </div>
            ))}
        </div>
      )}
    </Modal>
  )
}

/* ── the create/onboard door (S13.7 via P3-RW-2's POST) ──────────────────── */

function CreateProject({
  open,
  onOpenChange,
  onLanded,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  onLanded?: () => void
}) {
  const [projectID, setProjectID] = useState('')
  const [name, setName] = useState('')
  const [remote, setRemote] = useState('')
  const [family, setFamily] = useState('')
  const [busy, setBusy] = useState(false)
  const [refusal, setRefusal] = useState('')
  const [started, setStarted] = useState<ProjectStarted | null>(null)

  const idOk = /^[a-z0-9][a-z0-9-]*$/.test(projectID)
  const ready = projectID !== '' && idOk && name.trim() !== ''

  const close = () => {
    onOpenChange(false)
    setRefusal('')
    setStarted(null)
    setBusy(false)
  }

  const submit = () => {
    if (!ready || busy) return
    setBusy(true)
    setRefusal('')
    api
      .createProject({
        project_id: projectID,
        name: name.trim(),
        ...(remote.trim() !== '' ? { remote_url: remote.trim() } : {}),
        ...(family !== '' ? { family } : {}),
      })
      .then(
        (r) => {
          setBusy(false)
          setStarted(r)
          onLanded?.()
        },
        (err: unknown) => {
          setBusy(false)
          if (err instanceof ApiError && err.code === 'dev_identity') {
            // The server's own refusal cites its spec section; a person mid-task
            // needs the act, not the citation (2026-08-12 walk, cosmetic 3).
            setRefusal('sign-in')
          } else if (err instanceof ApiError && err.status === 404) {
            setRefusal(
              'This control plane does not serve the projects door — it predates the projects service. Nothing was registered.',
            )
          } else if (err instanceof ApiError && err.code === 'already_registered') {
            setRefusal(`"${projectID}" is already registered. ${err.message}`)
          } else {
            setRefusal(describeError(err))
          }
        },
      )
  }

  return (
    <Modal
      open={open}
      onOpenChange={(o) => {
        if (!o) close()
        else onOpenChange(o)
      }}
      title="New project"
      description="Registering starts onboarding: the platform prepares the project's store, scans what it finds, and drafts the project record. Activation is YOURS — an approval card lands in your Inbox, and the project turns active when you approve it."
      footer={
        started !== null ? (
          <>
            <Button
              variant="primary"
              onClick={() => {
                navigate(hrefFor('inbox'))
              }}
            >
              To the Inbox — the approval finishes it
            </Button>
            <Button variant="ghost" onClick={close}>
              Close
            </Button>
          </>
        ) : (
          <>
            <Button variant="primary" disabled={!ready || busy} aria-busy={busy} data-create-project onClick={submit}>
              {busy ? 'Registering…' : 'Register — start onboarding'}
            </Button>
            <Button variant="ghost" disabled={busy} onClick={close}>
              Cancel
            </Button>
            {/* GF1-W2: the why-line sits AFTER both buttons and KEEPS ITS SPACE
                when the form becomes ready (hidden, not unmounted) — the old
                placement (between the two) shifted Cancel the instant Register
                enabled and ate the operator's click, and unmounting the line
                re-centered the whole dialog for the same effect vertically. */}
            <span
              className="door-why"
              style={ready || busy ? { visibility: 'hidden' } : undefined}
              aria-hidden={ready || busy ? true : undefined}
            >
              {projectID === '' || name.trim() === ''
                ? 'both the id and the name are needed'
                : 'ids are lowercase letters, digits and dashes'}
            </span>
          </>
        )
      }
    >
      {started !== null ? (
        <div data-created={started.project.project_id}>
          {/* The platform's own sentence — in-flight, freshly started, or healed,
              it says which; this client does not re-tell it. */}
          <p className="m-0 text-sm">
            <b>{started.project.name !== '' ? started.project.name : started.project.project_id}</b> — {started.detail}
          </p>
          <p className="proj-quiet m-0 mt-2">
            onboarding task <b className="mono">{started.task_id}</b> · approval <b className="mono">{started.ask_ref}</b>{' '}
            · <Chip tone={stateChip(started.project.state).tone}>{stateChip(started.project.state).label}</Chip>
          </p>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <label className="door-field m-0">
            <span className="door-label">Project id — how tasks pin to it</span>
            <input
              className="door-input"
              type="text"
              aria-label="Project id"
              placeholder="e.g. shop-backend"
              value={projectID}
              onChange={(e) => {
                setProjectID(e.target.value)
              }}
            />
          </label>
          <label className="door-field m-0">
            <span className="door-label">Name — plain words</span>
            <input
              className="door-input"
              type="text"
              aria-label="Name"
              placeholder="e.g. The shop's backend"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
          </label>
          <label className="door-field m-0">
            {/* GF1-W1: "Remote to clone" is git's vocabulary, not the
                reader's. Same served field, plain words. */}
            <span className="door-label">
              Start from an existing repository{' '}
              <span className="door-optional">optional — paste its address; empty starts a fresh store</span>
            </span>
            <input
              className="door-input"
              type="text"
              aria-label="Start from an existing repository"
              placeholder="e.g. https://github.com/you/repo.git"
              value={remote}
              onChange={(e) => {
                setRemote(e.target.value)
              }}
            />
          </label>
          <label className="door-field m-0">
            <span className="door-label">
              What kind of tasks this project holds <span className="door-optional">optional</span>
            </span>
            <select
              className="door-input door-select"
              value={family}
              data-create-family
              onChange={(e) => {
                setFamily(e.target.value)
              }}
            >
              <option value="">decide later — each task is classified or asked</option>
              <option value="software">Build or change software</option>
              <option value="research">Find something out</option>
              <option value="content">Write or create content</option>
              <option value="data">Work with data</option>
              <option value="chore">A routine chore</option>
              <option value="generic">Something else</option>
            </select>
            <span className="door-optional">
              Declared here, every task in this project opens the right questions at once — no kind-of-work question,
              no guessing.
            </span>
          </label>
          {refusal === 'sign-in' ? (
            <div className="door-refusal" role="alert">
              <p className="refusal-detail">
                Creating a project is a person&apos;s act, and you are browsing without being signed in — nothing was
                registered. <Link to={hrefFor('login')}>Sign in</Link>, then register it again; the project is recorded
                as yours.
              </p>
            </div>
          ) : refusal !== '' ? (
            <div className="door-refusal" role="alert">
              <p className="refusal-detail">{refusal}</p>
            </div>
          ) : null}
        </div>
      )}
    </Modal>
  )
}
