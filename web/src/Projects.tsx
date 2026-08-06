import { useMemo, useState } from 'react'
import { FolderOpen, FolderPlus, Sparkles } from 'lucide-react'

import { ApiError, api, type Deliverable, type TaskListItem } from './api'
import type { EventStream } from './events'
import { columnsFor } from './kanban'
import { boardEventTypes, describeError, useLive } from './live'
import { Freshness, Money, Owner } from './parts'
import { useProjectScope } from './project'
import { Link, navigate } from './router'
import { hrefFor } from './routes'
import { Button, Chip, EmptyState, Modal, Timestamp } from './ui'

/**
 * Projects (map §3 v3): work lives in projects — every project as a card,
 * "(no project)" as its own real bucket, the give-work door straight into a
 * project, and the create/onboard door.
 *
 * WHAT THE CARDS ARE MADE OF. Task-derived aggregates over the caller's own
 * served rows (map §5: counts and spend stay client-side): active tasks by
 * stage in the board's own column words, the waiting-on-you count, the spend
 * across the project's latest runs — labelled as exactly that arithmetic —
 * and the recent deliverables. The REGISTRY detail (capture summary,
 * conventions, danger zones, ACTIVE/PENDING state) arrives with the
 * `/api/projects` read doors (packet P3-RW-2, mid-flight); until they land
 * this surface says so rather than inventing a status.
 *
 * THE CREATE DOOR (S13.7 through P3-RW-2's POST): register → clone → scan →
 * draft → the OWNER's approval card in the Inbox → active. The form says the
 * whole journey up front, and a control plane that does not serve the door
 * yet refuses loudly in its own words.
 */
export function Projects({ me, stream }: { me: string; stream?: EventStream }) {
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
  const [creating, setCreating] = useState(false)

  const buckets = useMemo(
    () => projectBuckets(tasks.data?.tasks ?? [], deliverables.data?.deliverables ?? []),
    [tasks.data, deliverables.data],
  )
  const answered = tasks.data !== null

  return (
    <section className="surface">
      <Freshness stale={tasks.stale} error={tasks.error} hasData={answered} />
      <div className="proj-head">
        <p className="proj-intro">
          Work lives in projects: a task pinned to one is planned against the project&apos;s accumulated work, its
          conventions and its memory. Opening a project scopes the whole app to it. The registry detail — capture
          summary, conventions, danger zones — joins these cards when the projects service door lands.
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

      {answered && buckets.length === 0 && (
        <EmptyState
          what="No project has work yet."
          why='A project appears here the moment a task lives in it. "New project" registers one; "Describe a goal" starts the first task.'
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
        {buckets.map((b) => (
          <ProjectCard key={b.name} bucket={b} />
        ))}
      </div>

      <CreateProject open={creating} onOpenChange={setCreating} />
    </section>
  )
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

export function projectBuckets(tasks: TaskListItem[], deliverables: Deliverable[]): Bucket[] {
  const names = new Map<string, TaskListItem[]>()
  for (const t of tasks) {
    const list = names.get(t.project)
    if (list) list.push(t)
    else names.set(t.project, [t])
  }
  // Deliverables can name a project no task currently shows — keep the bucket.
  for (const d of deliverables) {
    const key = d.project_id === '' ? '(no project)' : d.project_id
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
      .filter((d) => (d.project_id === '' ? '(no project)' : d.project_id) === name)
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

/* ── one card ────────────────────────────────────────────────────────────── */

function ProjectCard({ bucket }: { bucket: Bucket }) {
  const { setProject } = useProjectScope()
  const noProject = bucket.name === '(no project)'

  return (
    <article className="proj-card" data-project={bucket.name}>
      <header className="proj-card-head">
        <FolderOpen size={17} strokeWidth={1.8} aria-hidden="true" className="proj-ico" />
        <h3 className="proj-name">{bucket.name}</h3>
        {bucket.waiting > 0 && (
          <Chip tone="orange" className="waiting-human">
            {String(bucket.waiting)} waiting on you
          </Chip>
        )}
      </header>

      {noProject && (
        <p className="proj-sub">
          The honest bucket: tasks submitted without a project pin. They plan standalone — nothing from any
          project&apos;s world is injected.
        </p>
      )}

      <div className="proj-stages">
        {bucket.byStage.length === 0 ? (
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

      {bucket.recent.length > 0 && (
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
          data-proj-open={bucket.name}
          onClick={() => {
            setProject(bucket.name)
            navigate(hrefFor('board'))
          }}
        >
          Open — scope the app to it
        </Button>
        {!noProject && (
          <Button
            variant="primary"
            size="sm"
            data-proj-describe={bucket.name}
            onClick={() => {
              navigate(`${hrefFor('new')}?project=${encodeURIComponent(bucket.name)}`)
            }}
          >
            <Sparkles size={13} strokeWidth={2} aria-hidden="true" />
            Describe a goal
          </Button>
        )}
      </div>
    </article>
  )
}

/* ── the create/onboard door (S13.7 via P3-RW-2's POST) ──────────────────── */

function CreateProject({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const [projectID, setProjectID] = useState('')
  const [name, setName] = useState('')
  const [remote, setRemote] = useState('')
  const [busy, setBusy] = useState(false)
  const [refusal, setRefusal] = useState('')
  const [landed, setLanded] = useState(false)

  const idOk = /^[a-z0-9][a-z0-9-]*$/.test(projectID)
  const ready = projectID !== '' && idOk && name.trim() !== ''

  const close = () => {
    onOpenChange(false)
    setRefusal('')
    setLanded(false)
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
      })
      .then(
        () => {
          setBusy(false)
          setLanded(true)
        },
        (err: unknown) => {
          setBusy(false)
          if (err instanceof ApiError && err.status === 404) {
            setRefusal(
              'This control plane does not serve the projects door yet — it is landing as its own backend packet. Nothing was registered.',
            )
          } else if (err instanceof ApiError && err.code === 'already_registered') {
            setRefusal(`"${projectID}" is already an active project. ${err.message}`)
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
        landed ? (
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
            {!ready && !busy && (
              <span className="door-why">
                {projectID === '' || name.trim() === ''
                  ? 'both the id and the name are needed'
                  : 'ids are lowercase letters, digits and dashes'}
              </span>
            )}
            <Button variant="ghost" disabled={busy} onClick={close}>
              Cancel
            </Button>
          </>
        )
      }
    >
      {landed ? (
        <p className="m-0 text-sm" data-created={projectID}>
          <b>{projectID}</b> is registered and onboarding — its approval card is on its way to your Inbox. Until you
          approve, the project is pending and tasks cannot pin to it.
        </p>
      ) : (
        <div className="flex flex-col gap-3">
          <label className="door-field m-0">
            <span className="door-label">Project id — how tasks pin to it</span>
            <input
              className="door-input"
              type="text"
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
              placeholder="e.g. The shop's backend"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
          </label>
          <label className="door-field m-0">
            <span className="door-label">
              Remote to clone <span className="door-optional">optional — empty starts a fresh store</span>
            </span>
            <input
              className="door-input"
              type="text"
              placeholder="e.g. https://github.com/you/repo.git"
              value={remote}
              onChange={(e) => {
                setRemote(e.target.value)
              }}
            />
          </label>
          {refusal !== '' && (
            <div className="door-refusal" role="alert">
              <p className="refusal-detail">{refusal}</p>
            </div>
          )}
        </div>
      )}
    </Modal>
  )
}
