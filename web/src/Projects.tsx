import { useEffect, useMemo, useState } from 'react'
import { BookOpen, FolderOpen, FolderPlus, Sparkles } from 'lucide-react'

import {
  ApiError,
  api,
  type Deliverable,
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
          <ProjectCard key={c.name} card={c} registryAnswered={registryAnswered} />
        ))}
      </div>

      {registry.data !== null && (
        <p className="proj-visibility">
          {registry.data.visibility}
          {registry.data.truncated && <> The list is truncated at the served page — not every entry is shown.</>}
        </p>
      )}

      <CreateProject open={creating} onOpenChange={setCreating} onLanded={registry.reload} />
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

/** The bucket a deliverable files under: its project, with BOTH the empty
 *  string and an absent field degrading to '(no project)'. The list wire
 *  promises the field, but an older binary omits it when empty — and an
 *  undefined bucket name would throw in the name sort and take the whole
 *  surface down with it (operator, 2026-08-12). */
function bucketOf(d: Deliverable): string {
  return d.project_id ? d.project_id : '(no project)'
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
    const key = bucketOf(d)
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
      .filter((d) => bucketOf(d) === name)
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

function ProjectCard({ card, registryAnswered }: { card: CardData; registryAnswered: boolean }) {
  const { setProject } = useProjectScope()
  const { name, entry, bucket } = card
  const noProject = name === '(no project)'
  const [recordOpen, setRecordOpen] = useState(false)

  return (
    <article className="proj-card" data-project={name}>
      <header className="proj-card-head">
        <FolderOpen size={17} strokeWidth={1.8} aria-hidden="true" className="proj-ico" />
        <h3 className="proj-name">{entry !== null && entry.name !== '' ? entry.name : name}</h3>
        {bucket !== null && bucket.waiting > 0 && (
          <Chip tone="orange" className="waiting-human">
            {String(bucket.waiting)} waiting on you
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
