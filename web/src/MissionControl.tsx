import { Activity, Clock3, Coins, Sparkles, UserCheck } from 'lucide-react'

import {
  api,
  type Answer,
  type ApprovalItem,
  type MeterView,
  type Meters,
  type RunListItem,
  type TaskListItem,
} from './api'
import type { EventStream } from './events'
import { FilterBar, FilterView, filterFromSearch } from './Filters'
import { AnswerView } from './History'
import { inboxEventTypes, missionEventTypes, useLive } from './live'
import { Absent, Freshness, Money, Owner, ParkedUntil, Section, StallBanner, ThresholdRing } from './parts'
import { useProjectScope } from './project'
import { Link } from './router'
import { hrefFor } from './routes'
import { StageName } from './TaskDetail'
import { Chip, EmptyState, Panel, StatTile, StatusDot, Timestamp, type Tone } from './ui'

/**
 * Home (Spec S15.5 ¶1; S3.1; product map v2.1 §3): the household's work at a
 * glance — hero, four stat tiles, the live roster, per-person burn, a slim
 * recent-activity feed, and the personal filters. The deep instruments live
 * where the map puts them: consumption detail on Fleet, the query layers on
 * History. Home stays a glance, and everything on it drills through.
 *
 * WHO OWNS WHAT IS ON EVERY ITEM. The server has already scoped the read — a
 * member's answer contains only their own rows — so this view filters nothing
 * for privacy and hides no owner (S15.2). The PROJECT scope narrows the
 * DISPLAY of that already-scoped answer: tasks by their served project value,
 * runs by their task's, and what the narrowing hides is counted on screen.
 */

/**
 * The recently-finished window.
 *
 * A structural display constant with its reason (§41 precedent; no ⚙ key
 * exists and the browser cannot read the registry before login): 24 hours,
 * because Home answers "what happened since I last looked" and a day is the
 * span a household's work is actually planned across. It changes only what is
 * SHOWN — every finished run remains readable through the run and task
 * surfaces and through the history layers.
 */
const recentlyFinishedWindowMs = 24 * 60 * 60 * 1000

/** The S02.3 stored terminal states. Derived nothing: these are the values the
 *  FSM writes, and a run in one of them has stopped. */
const terminalStates = ['completed', 'crashed', 'finalized', 'tombstoned', 'died-at-gate']

export type Bucket = { id: string; title: string; runs: RunListItem[] }

/**
 * What puts a run in each bucket, in the reader's words — the teaching half of
 * the D8 pass (B6 gate §9 A-1).
 *
 * These are the SAME rules `bucketRuns` sorts by, stated once so a tile's foot,
 * a section's subtitle and an empty state's "why" cannot drift from each other
 * or from the sort. Nothing here is a second classification: the sort below is
 * the only one.
 */
const bucketMeaning: Record<string, { tone: Tone; why: string }> = {
  running: { tone: 'green', why: 'A run the platform is working on right now.' },
  queued: {
    tone: 'blue',
    why: 'Created or queued and not started yet — waiting for the scheduler to pick it up when its lane has room.',
  },
  blocked: {
    tone: 'orange',
    why: 'Parked with an open question, so it moves again only once a person answers it in the inbox.',
  },
  parked: { tone: 'yellow', why: 'Stopped on a clock rather than on a person — it resumes at its own horizon.' },
  finished: { tone: 'accent', why: 'Ended in the last day. Older work stays readable from its task and from History.' },
  other: {
    tone: 'pink',
    why: 'A state these five do not name — mid-dispatch, draining, or one a later version added. It is shown rather than hidden.',
  },
}

const meaningOf = (id: string) => bucketMeaning[id] ?? { tone: 'accent' as Tone, why: '' }

/** The bucket's teaching sentence, exported so a test asserts the copy the
 *  surface actually renders rather than a second copy of it. */
export const bucketMeaningFor = (id: string): string => meaningOf(id).why

/**
 * bucketRuns sorts served rows into the five S15.5 buckets, plus an honest
 * catch-all.
 *
 * The buckets are DISJOINT, and the ordering of the tests is the reason:
 * blocked-on-a-human is a strict subset of parked (the server derives it as
 * "parked with an open ask"), so a run waiting on a person is shown under the
 * person bucket only and the parked bucket is left meaning "waiting on a
 * clock". The final bucket exists so that a state none of the five name is
 * still on the screen. A run must never vanish from Home because this list did
 * not anticipate its state.
 */
export function bucketRuns(runs: RunListItem[], now: number): Bucket[] {
  const buckets: Bucket[] = [
    { id: 'running', title: 'Running', runs: [] },
    { id: 'queued', title: 'Queued', runs: [] },
    { id: 'blocked', title: 'Blocked on a human', runs: [] },
    { id: 'parked', title: 'Parked', runs: [] },
    { id: 'finished', title: 'Recently finished', runs: [] },
    { id: 'other', title: 'Everything else', runs: [] },
  ]
  const put = (id: string, r: RunListItem) => buckets.find((b) => b.id === id)?.runs.push(r)

  for (const r of runs) {
    if (r.waiting_on_human) put('blocked', r)
    else if (r.state === 'parked') put('parked', r)
    else if (r.state === 'running') put('running', r)
    else if (r.state === 'new' || r.state === 'queued') put('queued', r)
    else if (terminalStates.includes(r.state)) {
      const at = r.last_activity_ts ?? r.updated_ts
      if (now - Date.parse(at) <= recentlyFinishedWindowMs) put('finished', r)
    } else put('other', r)
  }
  return buckets
}

export function MissionControl({
  stream,
  me = '',
  search = '',
}: { stream?: EventStream; me?: string; search?: string } = {}) {
  const filter = filterFromSearch(search)
  const { project } = useProjectScope()
  const work = useLive({
    key: '/api/runs',
    read: () => api.runs(),
    types: missionEventTypes,
    stream,
  })
  const tasks = useLive({
    key: '/api/tasks',
    read: () => api.tasks(),
    types: missionEventTypes,
    stream,
  })
  const meters = useLive({
    key: '/api/meters',
    read: () => api.meters(),
    types: missionEventTypes,
    stream,
  })
  const asks = useLive({
    key: '/api/approvals',
    read: () => api.approvals(),
    types: inboxEventTypes,
    stream,
  })

  // The project scope narrows the DISPLAY of the already-owner-scoped answers.
  // A run's project is its task's served project value; a run with no task has
  // no project dimension, so a scope hides it and the hiding is counted.
  const allTasks = tasks.data?.tasks ?? []
  const taskOf = new Map(allTasks.map((t) => [t.task_id, t]))
  const scopedTasks = project === '' ? allTasks : allTasks.filter((t) => t.project === project)
  const allRuns = work.data?.runs ?? []
  const runs =
    project === ''
      ? allRuns
      : allRuns.filter((r) => r.task_id !== '' && taskOf.get(r.task_id)?.project === project)
  const hiddenStandalone = project === '' ? 0 : allRuns.filter((r) => r.task_id === '').length

  // Read once per render rather than on a ticker: this view re-renders when the
  // feed says something changed, and a clock of its own would be the one thing
  // §32 rules out.
  const buckets = bucketRuns(runs, Date.now())
  const bucket = (id: string) => buckets.find((b) => b.id === id)!
  const answered = work.data !== null

  return (
    <section className="surface">
      <HomeHero
        buckets={buckets}
        tasks={scopedTasks}
        taskOf={taskOf}
        project={project}
        answered={answered}
      />

      <FilterBar active={filter} />
      {filter !== '' && <FilterView id={filter} me={me} stream={stream} />}
      <Freshness stale={work.stale || tasks.stale} error={work.error} hasData={work.data !== null} />

      {/* Counts of the rows ON SCREEN — presentation over the served lists,
          which the labels say. No tile carries a number no list below carries. */}
      <div className="my-4 grid grid-cols-2 gap-(--density-gap) xl:grid-cols-4" data-tiles="home">
        {/* Before the read lands the tiles show an absence, not a zero: a 0
            the platform never served would be a fabricated figure, stale
            marker or no stale marker. */}
        <StatTile
          label="Running"
          tone="green"
          icon={<Activity size={15} strokeWidth={1.8} aria-hidden="true" />}
          value={answered ? String(bucket('running').runs.length) : '—'}
          foot="runs executing now"
        />
        <StatTile
          label="Waiting on you"
          tone="orange"
          icon={<UserCheck size={15} strokeWidth={1.8} aria-hidden="true" />}
          value={answered ? String(bucket('blocked').runs.length) : '—'}
          foot={
            <>
              answered from the <Link to={hrefFor('inbox')}>Inbox</Link>
              {asks.data !== null && <> · {String(asks.data.items.length)} open cards</>}
            </>
          }
        />
        <StatTile
          label="Parked"
          tone="yellow"
          icon={<Clock3 size={15} strokeWidth={1.8} aria-hidden="true" />}
          value={answered ? String(bucket('parked').runs.length) : '—'}
          foot={answered ? <ParkedFoot parked={bucket('parked').runs} /> : 'waiting on a clock'}
        />
        <SpendTile view={meters.data?.per_period} scoped={project !== ''} absentAll={meters.data === null} />
      </div>

      <RosterPanel
        buckets={buckets}
        taskOf={taskOf}
        answered={answered}
        stale={work.stale}
        hiddenStandalone={hiddenStandalone}
        project={project}
      />

      <div className="grid gap-(--density-gap) lg:grid-cols-2">
        <BurnPanel meters={meters.data} stale={meters.stale} error={meters.error} />
        <ActivityPanel runs={runs} asks={asks.data?.items ?? []} taskOf={taskOf} answered={answered} />
      </div>
    </section>
  )
}

/* ── hero (the Nexus dashboard hero, without the dropped 3D galaxy) ──────── */

function HomeHero({
  buckets,
  tasks,
  taskOf,
  project,
  answered,
}: {
  buckets: Bucket[]
  tasks: TaskListItem[]
  taskOf: Map<string, TaskListItem>
  project: string
  answered: boolean
}) {
  const running = buckets.find((b) => b.id === 'running')!.runs
  const blocked = buckets.find((b) => b.id === 'blocked')!.runs
  const projects = new Set(tasks.map((t) => t.project))

  return (
    <div className="home-hero">
      <div className="hero-orb" aria-hidden="true" />
      <div className="hero-copy">
        <p className="hero-kicker">{project === '' ? 'Sinet · control room' : `Project · ${project}`}</p>
        <p className="hero-headline">
          {answered ? (
            <>
              <b className="mono">{String(running.length)}</b> running ·{' '}
              <b className="mono">{String(blocked.length)}</b> waiting on you
            </>
          ) : (
            'Reading the platform…'
          )}
        </p>
        <p className="hero-what" data-surface-what>
          Everything running, queued, parked, blocked on a human or recently finished — with each person&apos;s burn
          from the meters beside it.
          {answered && (
            <>
              {' '}
              {String(tasks.length)} task{tasks.length === 1 ? '' : 's'}
              {project === '' && projects.size > 0 && (
                <> across {String(projects.size)} project bucket{projects.size === 1 ? '' : 's'}</>
              )}
              .
            </>
          )}
        </p>
        {/* ▲ v3: the give-work DOOR opens from a button (never a nav tab).
            With a project scoped, the door opens into that project's world. */}
        <p className="hero-cta">
          <Link
            to={project === '' || project === '(no project)' ? hrefFor('new') : `${hrefFor('new')}?project=${encodeURIComponent(project)}`}
            className="btn-describe"
            data-door="describe-goal"
          >
            <Sparkles size={15} strokeWidth={2} aria-hidden="true" />
            Describe a goal
          </Link>
        </p>
      </div>
      <div className="hero-now">
        <p className="now-title">
          <StatusDot tone="green" live={running.length > 0} /> Executing right now
        </p>
        {running.length === 0 ? (
          <p className="now-empty">{answered ? 'No run is executing right now' : 'catching up…'}</p>
        ) : (
          running.slice(0, 4).map((r) => <NowRow key={r.run_id} run={r} task={taskOf.get(r.task_id)} />)
        )}
        {running.length > 4 && <p className="now-empty">and {String(running.length - 4)} more below</p>}
      </div>
    </div>
  )
}

function NowRow({ run, task }: { run: RunListItem; task?: TaskListItem }) {
  const body = (
    <>
      <span className="now-head">
        <StatusDot tone="green" live />
        <span className="truncate">{task?.title ?? run.run_id}</span>
      </span>
      <span className="now-task">
        <Owner id={run.owner} /> · {run.stage !== '' ? run.stage : 'no stage marker yet'} · {run.lane}
      </span>
      <span className="now-meta">
        <Timestamp ts={run.last_activity_ts} variant="live" />
      </span>
    </>
  )
  return run.task_id !== '' ? (
    <Link to={hrefFor('task', { id: run.task_id })} className="now-row">
      {body}
    </Link>
  ) : (
    <span className="now-row">{body}</span>
  )
}

/* ── tiles ───────────────────────────────────────────────────────────────── */

function ParkedFoot({ parked }: { parked: RunListItem[] }) {
  if (parked.length === 0) return <>waiting on a clock</>
  // The EARLIEST served horizon — a pick from served instants, not arithmetic.
  const horizons = parked.map((r) => r.parked_until).filter((h): h is string => h !== null && h !== '')
  if (horizons.length === 0) return <Absent reason="parked, no horizon given" />
  const next = horizons.reduce((a, b) => (Date.parse(b) < Date.parse(a) ? b : a))
  return (
    <>
      next resumes <Timestamp ts={next} variant="live" />
    </>
  )
}

/**
 * Spend today — per person, exactly as the served `cost_per_period` view
 * answers it (day × person × priced_usd). Money is picked off rows, NEVER
 * summed across people (§37): a household total is arithmetic nobody
 * performed, so the tile lists each person's own served figure instead.
 * "Today" is the served UTC day, and the foot says so.
 */
function SpendTile({ view, scoped, absentAll }: { view?: MeterView; scoped: boolean; absentAll: boolean }) {
  const icon = <Coins size={15} strokeWidth={1.8} aria-hidden="true" />
  const today = new Date().toISOString().slice(0, 10)
  const note = scoped ? ' · household-wide, not narrowed by the project scope' : ''

  let value: React.ReactNode = '—'
  let foot: React.ReactNode = `no receipts today (UTC day)${note}`

  if (absentAll) {
    foot = 'meters not read yet'
  } else if (view === undefined || (view.answer === undefined && view.absent !== undefined)) {
    foot = <Absent reason={view?.absent ?? 'not served'} />
  } else if (view.answer !== undefined) {
    const rows = todayRows(view.answer, today)
    if (rows.length > 0) {
      value = (
        <span className="flex flex-col gap-0.5">
          {rows.map((r) => (
            <span key={r.person} className="text-[15px] leading-tight">
              <Owner id={r.person} />{' '}
              {r.usd === null ? <span className="muted">unpriced</span> : <Money usd={r.usd} />}
            </span>
          ))}
        </span>
      )
      foot = `per person, ${today} (UTC)${note}`
    }
  }

  // W1-14: the tile looked pressable and was not — so it became a real door.
  // The full meters live on Fleet; the tile says so and goes there.
  return (
    <Link to={hrefFor('fleet')} className="tile-door" data-door="fleet" aria-label="Spend today — open Fleet for the full meters">
      <StatTile
        label="Spend today"
        tone="blue"
        icon={icon}
        value={value}
        foot={
          <>
            {foot} · <span className="tile-door-hint">open Fleet for the full meters</span>
          </>
        }
      />
    </Link>
  )
}

/** todayRows reads the served per-period answer BY ITS OWN COLUMNS — the view's
 *  grain is (day, person), and a row whose day is today is today's reading. */
function todayRows(answer: Answer, today: string): { person: string; usd: number | null }[] {
  const day = answer.columns.indexOf('day')
  const person = answer.columns.indexOf('user_id')
  const usd = answer.columns.indexOf('priced_usd')
  if (day === -1 || person === -1 || usd === -1) return []
  return answer.rows
    .filter((r) => String(r[day]) === today)
    .map((r) => ({
      person: String(r[person]),
      usd: typeof r[usd] === 'number' ? (r[usd] as number) : null,
    }))
}

/* ── the live roster ─────────────────────────────────────────────────────── */

/** The blocked bucket's door. Answering happens on the inbox — this says so and
 *  fires nothing, because the ask's own verbs live where the card is. */
function BlockedDoor() {
  return (
    <StallBanner
      kind="blocked"
      tone="orange"
      what="These runs are stopped until somebody answers their open question. Nothing here restarts on its own."
    >
      <Link to={hrefFor('inbox')}>Answer them in the inbox</Link>
    </StallBanner>
  )
}

/**
 * The roster (the Nexus dashboard roster, on Sinet's real rows): every
 * non-terminal run, grouped by the same disjoint buckets the sort defines,
 * each row joined to its task's card face for the title and cost. Recently
 * finished runs sit under it, collapsed — the feed tells the story, the
 * roster is who is on shift.
 */
function RosterPanel({
  buckets,
  taskOf,
  answered,
  stale,
  hiddenStandalone,
  project,
}: {
  buckets: Bucket[]
  taskOf: Map<string, TaskListItem>
  answered: boolean
  stale: boolean
  hiddenStandalone: number
  project: string
}) {
  const live = buckets.filter((b) => b.id !== 'finished')
  const finished = buckets.find((b) => b.id === 'finished')!
  const liveCount = live.reduce((n, b) => n + b.runs.length, 0)
  // W2-11: a run without a task is the platform's own housekeeping (checks,
  // sweeps — "no task, this run stands alone" is its own row label below).
  // Counting them into one big figure inflated the work story: "106 runs
  // finished" read as 106 tasks' worth of work. The split names both halves.
  const finishedTaskWork = finished.runs.filter((r) => r.task_id !== '').length
  const finishedHousekeeping = finished.runs.length - finishedTaskWork

  return (
    <Section title="The roster" stale={stale}>
      <Panel
        className="roster"
        head={
          <>
            <span className="panel-title">Live work</span>
            <span className="panel-meta">
              {answered ? `${String(liveCount)} runs on shift` : 'catching up…'}
            </span>
          </>
        }
      >
        {answered && liveCount === 0 && (
          <EmptyState
            what="Nothing is on shift."
            why="A run appears here the moment work starts — running first, then queued, blocked-on-a-person and parked, each saying why it sits where it does."
          />
        )}
        {live.map(
          (b) =>
            b.runs.length > 0 && (
              <div key={b.id} className="roster-sec" data-bucket={b.id}>
                <p className="roster-sec-head" title={meaningOf(b.id).why}>
                  <StatusDot tone={meaningOf(b.id).tone} live={b.id === 'running'} />
                  <span className="roster-sec-title">{b.title}</span>
                  <span className="roster-count mono">{String(b.runs.length)}</span>
                </p>
                {b.id === 'blocked' && <BlockedDoor />}
                <ul className="roster-rows">
                  {b.runs.map((r) => (
                    <RosterRow key={r.run_id} run={r} task={taskOf.get(r.task_id)} />
                  ))}
                </ul>
              </div>
            ),
        )}
        {hiddenStandalone > 0 && (
          <p className="muted mt-2 text-xs">
            {String(hiddenStandalone)} run{hiddenStandalone === 1 ? '' : 's'} without a task {hiddenStandalone === 1 ? 'is' : 'are'} outside
            every project, so the {project} scope hides {hiddenStandalone === 1 ? 'it' : 'them'} — clear the scope to
            see {hiddenStandalone === 1 ? 'it' : 'them'}.
          </p>
        )}
        {finished.runs.length > 0 && (
          <details className="roster-fin">
            <summary>
              Recently finished — {String(finished.runs.length)} run{finished.runs.length === 1 ? '' : 's'} in the last
              day
              {finishedHousekeeping > 0 && (
                <>
                  {' '}
                  ({String(finishedTaskWork)} task work · {String(finishedHousekeeping)} platform housekeeping)
                </>
              )}
            </summary>
            <ul className="roster-rows">
              {finished.runs.map((r) => (
                <RosterRow key={r.run_id} run={r} task={taskOf.get(r.task_id)} />
              ))}
            </ul>
          </details>
        )}
      </Panel>
    </Section>
  )
}

const stateTone = (r: RunListItem): Tone =>
  r.wedged
    ? 'red'
    : r.state === 'running'
      ? 'green'
      : r.state === 'parked'
        ? r.waiting_on_human
          ? 'orange'
          : 'yellow'
        : terminalStates.includes(r.state)
          ? 'accent'
          : 'blue'

/** One roster row: the run joined to its task's card face. The cost is the
 *  task face's cost and renders ONLY beside the run it was served for — the
 *  task's latest — never guessed onto an older generation. */
function RosterRow({ run, task }: { run: RunListItem; task?: TaskListItem }) {
  const isLatest = task?.latest_run?.run_id === run.run_id
  const cost = isLatest ? (task?.latest_run?.cost_so_far_usd ?? null) : null
  const effort = isLatest ? (task?.latest_run?.effort_mode ?? '') : ''

  return (
    <li className="roster-row">
      <span className="r-dot">
        <StatusDot tone={stateTone(run)} live={run.state === 'running'} />
      </span>
      <span className="r-task">
        {run.task_id !== '' ? (
          <Link to={hrefFor('task', { id: run.task_id })} className="r-title">
            {task?.title ?? run.task_id}
          </Link>
        ) : (
          <span className="r-title">
            <Absent reason="no task — this run stands alone" />
          </span>
        )}
        {/* The card-face facts as one quiet line: run · project · stage · lane
            · effort · cost. A latest-run cost that has no reading says so; an
            unmarked stage is simply not printed — the task page carries the
            full story. */}
        {/* Plain words, labeled (W2-9): the old dot-joined confetti
            ("r-claim · no cost reading · anthropic · quick") made a reader
            guess which token was which. Each fact now says what it is; the
            run id stays as the record it is. */}
        <span className="r-under mono">
          {run.run_id}
          {isLatest && (
            <>
              {' · '}
              {cost !== null ? (
                <>
                  spent <Money usd={cost} />
                </>
              ) : (
                <span className="absent">no cost recorded yet</span>
              )}
            </>
          )}
          {run.stage !== '' && (
            <>
              {' · '}
              <StageName stage={run.stage} />
            </>
          )}
          {` · on the ${run.lane} account`}
          {effort !== '' && ` · ${effort} effort`}
          {task !== undefined && task.project !== '' && ` · ${task.project}`}
        </span>
      </span>
      <span className="r-owner">
        <Owner id={run.owner} />
      </span>
      <span className="r-state">
        {/* The chip distinguishes the two kinds of standing still (W2-9): a
            run parked ON A PERSON wears "waiting on a person", and "parked"
            keeps meaning the clock kind — one badge, one meaning. */}
        <Chip tone={stateTone(run)}>
          {run.wedged ? 'wedged' : run.state === 'parked' && run.waiting_on_human ? 'waiting on a person' : run.state}
        </Chip>
      </span>
      <span className="r-when wide-cell">
        <Timestamp ts={run.last_activity_ts} variant="live" className="feed-stamp" />
      </span>
      {run.waiting_on_human && run.state !== 'parked' && (
        <span className="r-flag waiting-human">waiting on a person</span>
      )}
      {run.state === 'parked' && (run.parked_until !== null || !run.waiting_on_human) && (
        // A recorded horizon is a served fact no row may drop — waiting rows
        // included. The no-horizon arm renders only where the park is the
        // story (a clock park); on a waiting row "waiting on a person" IS the
        // reason, and a second flag saying "no horizon" would be noise.
        <span className="r-flag">
          <ParkedUntil until={run.parked_until} />
        </span>
      )}
      {run.wedged && (
        <span className="r-banner">
          <StallBanner
            kind="wedged"
            tone="red"
            what="Flagged and paused: the platform stopped this run rather than let it loop, and it waits for a person."
          >
            <Link to={hrefFor('inbox')}>Its card is in the inbox</Link>
          </StallBanner>
        </span>
      )}
    </li>
  )
}

/* ── per-person burn ─────────────────────────────────────────────────────── */

/**
 * Per-person burn: the served burn-rate view at its OWN grain (per person, per
 * observed day), each figure read off its row by column name — never divided,
 * never summed (§37) — beside the pause switch's served position. The deep
 * tables live on Fleet.
 */
function BurnPanel({ meters, stale, error }: { meters: Meters | null; stale: boolean; error: string }) {
  const paused = new Map((meters?.automation.states ?? []).map((s) => [s.owner, s.paused]))

  return (
    <Panel
      className="burn"
      head={
        <>
          <span className="panel-title">Per-person burn</span>
          <span className="panel-meta">
            <Link to={hrefFor('fleet')}>full meters on Fleet</Link>
          </span>
        </>
      }
    >
      <Freshness stale={stale} error={error} hasData={meters !== null} />
      {meters !== null &&
        (meters.burn_rates.answer !== undefined ? (
          <BurnRows answer={meters.burn_rates.answer} paused={paused} />
        ) : (
          <Absent reason={meters.burn_rates.absent ?? 'not available'} />
        ))}
      {meters !== null && meters.automation.absent !== undefined && (
        <p className="muted mt-2 text-xs">pause switches: {meters.automation.absent}</p>
      )}
    </Panel>
  )
}

function BurnRows({ answer, paused }: { answer: Answer; paused: Map<string, boolean> }) {
  const col = (name: string) => answer.columns.indexOf(name)
  const person = col('user_id')
  const rate = col('usd_per_day')
  const total = col('priced_usd')
  const days = col('observed_days')
  const status = col('pricing_status')

  // A shape this render does not recognise still reaches the screen, at the
  // served grain, through the generic answer table — shown rather than hidden.
  if (person === -1 || rate === -1) return <AnswerView answer={answer} />

  if (answer.rows.length === 0)
    return (
      <EmptyState
        what="No burn rate yet."
        why="A person appears here once their first receipt lands; the rate is USD per observed day across their receipts."
      />
    )

  return (
    <ul className="burn-rows">
      {answer.rows.map((r, i) => (
        <li className="burn-row" key={String(r[person]) + String(i)}>
          <span className="burn-who">
            <Owner id={String(r[person])} />
            {paused.get(String(r[person])) === true && <Chip tone="yellow">automation paused</Chip>}
          </span>
          <span className="burn-rate mono">
            {typeof r[rate] === 'number' ? `USD ${String(r[rate])} / day` : <Absent reason="unpriced" />}
          </span>
          <span className="burn-under">
            {total !== -1 && typeof r[total] === 'number' && <>USD {String(r[total])} priced</>}
            {days !== -1 && <> over {String(r[days])} observed day{String(r[days]) === '1' ? '' : 's'}</>}
            {status !== -1 && <> · {String(r[status])}</>}
          </span>
        </li>
      ))}
    </ul>
  )
}

/* ── the slim recent-activity feed ───────────────────────────────────────── */

type FeedItem = {
  key: string
  ts: string
  tone: Tone
  text: string
  /** Whose row this is — rendered structurally (D2/S3.10), never only prose. */
  owner: string
  href?: string
  live?: boolean
}

/**
 * The feed is a MERGE OF SERVED ROWS sorted by their own served instants —
 * runs by last activity, open cards by when they were first seen. Nothing here
 * is an event log of its own: the platform's real record is History's.
 */
function ActivityPanel({
  runs,
  asks,
  taskOf,
  answered,
}: {
  runs: RunListItem[]
  asks: ApprovalItem[]
  taskOf: Map<string, TaskListItem>
  answered: boolean
}) {
  const items: FeedItem[] = []
  for (const r of runs) {
    const at = r.last_activity_ts ?? r.updated_ts
    if (!at) continue
    const title = taskOf.get(r.task_id)?.title ?? r.task_id
    items.push({
      key: `run:${r.run_id}`,
      ts: at,
      tone: stateTone(r),
      text: `${title !== '' ? title : r.run_id} — ${r.wedged ? 'wedged' : r.state}${r.stage !== '' ? ` at ${r.stage}` : ''}`,
      owner: r.owner,
      href: r.task_id !== '' ? hrefFor('task', { id: r.task_id }) : undefined,
      live: r.state === 'running',
    })
  }
  for (const a of asks) {
    items.push({
      key: `ask:${a.id}`,
      ts: a.observed_ts,
      tone: a.tier === 'high' ? 'red' : 'orange',
      text: `${a.kind} waits on a person (${a.tier} tier)`,
      owner: a.owner,
      href: hrefFor('inbox-item', { id: a.id }),
    })
  }
  items.sort((a, b) => Date.parse(b.ts) - Date.parse(a.ts))
  const shown = items.slice(0, 9)

  return (
    <Panel
      className="feed"
      head={
        <>
          <span className="panel-title">Recent activity</span>
          <span className="panel-meta">
            <Link to={hrefFor('history')}>the full record is History</Link>
          </span>
        </>
      }
    >
      {answered && shown.length === 0 && (
        <EmptyState
          what="Nothing has moved yet."
          why="Run activity and cards that need a person land here, newest first, each linking to the thing it names."
        />
      )}
      <ul className="feed-rows">
        {shown.map((it) => (
          <li className="feed-row" key={it.key}>
            <StatusDot tone={it.tone} live={it.live === true} className="feed-dot" />
            {it.href !== undefined ? (
              <Link to={it.href} className="feed-text">
                {it.text}
              </Link>
            ) : (
              <span className="feed-text">{it.text}</span>
            )}
            <Owner id={it.owner} />
            <span className="feed-when">
              <Timestamp ts={it.ts} variant="live" className="feed-stamp" />
            </span>
          </li>
        ))}
      </ul>
    </Panel>
  )
}

/* ── the deep meters (mounted by FLEET — Home stays a glance) ────────────── */

/**
 * The meters (S15.5; S2.5; S10 through /api/meters), rendered faithfully.
 * Exported for Fleet, which is where the map puts the deep tables; the three
 * honesties are unchanged: the `assumed` label rides every reading it applies
 * to, pressure renders only when the gauge says it applies, and burn rates
 * render at the view's own grain — money is read, never computed (§37).
 */
export function MetersPanel({ meters, stale, error }: { meters: Meters | null; stale: boolean; error: string }) {
  return (
    <Section title="Consumption and budgets" stale={stale}>
      <Freshness stale={stale} error={error} hasData={meters !== null} />
      {meters && (
        <>
          <div className="table-scroll">
            <table className="meters">
              <thead>
                <tr>
                  <th>Whose</th>
                  <th>Lane</th>
                  <th>Weighted consumption</th>
                  <th>Pressure</th>
                  <th>Budget remaining</th>
                  <th>Runs</th>
                  <th>Parked</th>
                </tr>
              </thead>
              <tbody>
                {meters.lanes.map((l) => (
                  <tr key={`${l.owner}/${l.lane}`}>
                    <td>
                      <Owner id={l.owner} />
                    </td>
                    <td>{l.lane}</td>
                    <td>
                      {String(l.weighted_consumption)}
                      {l.assumed && (
                        <span className="assumed" title={`cache reads weighted at ${String(l.cache_read_weight)}`}>
                          {' '}
                          (assumed cache-read weight {String(l.cache_read_weight)})
                        </span>
                      )}
                    </td>
                    <td>
                      {l.pressure_applicable && l.pressure !== null ? (
                        <span className="inline-flex items-center gap-2">
                          <ThresholdRing
                            ratio={l.pressure}
                            label={`pressure ${String(l.pressure)} of declared budget — ${l.owner} in ${l.lane}`}
                          />
                          <span className="font-mono tabular-nums">{String(l.pressure)}</span>
                        </span>
                      ) : (
                        <Absent reason="no declared budget, so there is no denominator" />
                      )}
                    </td>
                    <td>
                      {l.budget_declared && l.budget_remaining !== null ? (
                        String(l.budget_remaining)
                      ) : (
                        <Absent reason="no budget declared" />
                      )}
                    </td>
                    <td>
                      {String(l.active_runs)} of {String(l.total_runs)}
                    </td>
                    <td>
                      {l.parked_runs > 0 ? <ParkedUntil until={l.parked_until} /> : <span className="muted">none</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {meters.lanes.length === 0 && (
            <EmptyState
              what="No lane has run anything yet."
              why="A lane appears the first time work runs on it. The meters are readings of what actually ran, so nothing here means nothing has."
            />
          )}

          <p className="mt-2 text-xs text-muted-foreground" data-ring-legend>
            The ring beside a pressure figure shows that figure as a share of the declared budget for that lane — green
            below three quarters, orange from there, red once the declared budget is reached.
          </p>

          <ViewBlock title="Burn rate (per person, per observed day)" view={meters.burn_rates} />
          <ViewBlock title="Budget remainders" view={meters.budgets} />
          {meters.lanes.some((l) => l.parked_runs > 0) && (
            <StallBanner
              kind="parked-lanes"
              tone="orange"
              what="A lane in this table is holding parked runs right now. New background work is not being started on it and running work parks at its next safe boundary — nothing queued or parked is discarded, and each lane's horizon is in its Parked column."
            >
              <Link to={hrefFor('inbox')}>Parked runs are released from their cards in the inbox</Link>
            </StallBanner>
          )}
          <ViewBlock title="Limit events — recorded history, not a current state" view={meters.limit_events} />
        </>
      )}
    </Section>
  )
}

/** ViewBlock renders one Layer-0 view at its own grain, or the reason it could
 *  not be read — an absence with its cause, never an empty table posing as an
 *  answer. */
export function ViewBlock({ title, view }: { title: string; view: MeterView }) {
  return (
    <div className="view-block mt-3">
      <h3 className="mt-0 mb-1 font-mono text-[9.5px] font-bold tracking-[2px] text-muted-foreground uppercase">
        {title}
      </h3>
      {view.answer ? <AnswerView answer={view.answer} /> : <Absent reason={view.absent ?? 'not available'} />}
    </div>
  )
}
