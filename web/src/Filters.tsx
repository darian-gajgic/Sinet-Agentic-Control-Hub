import { api, type ApprovalItem, type Deliverable, type RunListItem } from './api'
import type { EventStream } from './events'
import { missionEventTypes, useLive } from './live'
import { Absent, Freshness, Owner, ParkedUntil, Section } from './parts'
import { Link, navigate } from './router'
import { hrefFor } from './routes'
import { Button, Chip, EmptyState, Timestamp } from './ui'

/**
 * The four personal filters (Spec S15.5 ¶5; S1.4; S1.10).
 *
 * IDENTITY-SCOPED BY THE SERVER, PRESENTED HERE. Each filter is a query over
 * the landed reads; the authorization is the server's, and the client narrows
 * only what is displayed (S15.2). "mine" therefore sends `?person=<self>`
 * rather than filtering someone else's rows out after receiving them — for a
 * member the two are the same answer, and for the operator the parameter is
 * what makes "mine" mean mine.
 *
 * DEEP-LINKABLE WITHOUT A NEW ROUTE. Each filter is `/?view=<id>` — a stable
 * URL that survives a bookmark and a push `navigate` target, riding the route
 * table as it stands. The URL contract is filled, never renamed (§41-B).
 *
 * The doors are honest: what-needs-me LINKS into the inbox and the review
 * surface. Answering happens there, in the packets that build them — this
 * filter tells you what is waiting, and does not pretend to resolve it.
 */

export type FilterID = 'what-needs-me' | 'mine' | 'running' | 'finished-today'

export const filters: { id: FilterID; label: string }[] = [
  { id: 'what-needs-me', label: 'What needs me' },
  { id: 'mine', label: 'Mine' },
  { id: 'running', label: 'Running' },
  { id: 'finished-today', label: 'Finished today' },
]

/** filterFromSearch reads the selected filter off the URL. An unknown value is
 *  no filter rather than an error: a stale bookmark should show the home
 *  surface, not a broken page. */
export function filterFromSearch(search: string): FilterID | '' {
  const want = new URLSearchParams(search).get('view') ?? ''
  return filters.some((f) => f.id === want) ? (want as FilterID) : ''
}

export function hrefForFilter(id: FilterID): string {
  return `${hrefFor('mission-control')}?view=${id}`
}

/**
 * localMidnightISO is "today" as the person keeping the household actually
 * means it: midnight in THEIR zone, expressed as the UTC instant the API
 * compares against.
 *
 * The server normalizes `?since=` to UTC before comparing it lexicographically
 * against stored UTC text, so sending a local-offset stamp would compare as the
 * characters typed rather than the instant meant. Sending the instant already
 * in UTC makes both sides of that compare one representation — and keeps
 * "today" meaning the reader's today, not London's.
 */
export function localMidnightISO(now: Date): string {
  const midnight = new Date(now.getTime())
  midnight.setHours(0, 0, 0, 0)
  return midnight.toISOString()
}

export function FilterBar({ active }: { active: FilterID | '' }) {
  return (
    <nav className="filter-bar" aria-label="Personal filters">
      {filters.map((f) => (
        <Link
          key={f.id}
          to={hrefForFilter(f.id)}
          aria-current={f.id === active ? 'page' : undefined}
          className="rounded-(--radius-sm) px-2 py-1 text-sm text-muted-foreground no-underline aria-[current=page]:bg-(image:--grad-soft) aria-[current=page]:text-foreground"
        >
          {f.label}
        </Link>
      ))}
      {active !== '' && (
        <Button variant="ghost" size="sm" onClick={() => navigate(hrefFor('mission-control'))}>
          Clear
        </Button>
      )}
    </nav>
  )
}

export function FilterView({ id, me, stream }: { id: FilterID; me: string; stream?: EventStream }) {
  if (id === 'what-needs-me') return <WhatNeedsMe stream={stream} />
  return <RunFilter id={id} me={me} stream={stream} />
}

/** What needs me: the caller's approvals and questions, plus the deliverables
 *  waiting to be reviewed. Three feeds, one question. */
function WhatNeedsMe({ stream }: { stream?: EventStream }) {
  const asks = useLive({
    key: '/api/approvals',
    read: () => api.approvals(),
    types: missionEventTypes,
    stream,
  })
  const review = useLive({
    key: '/api/deliverables?state=in-review',
    read: () => api.deliverables({ state: 'in-review' }),
    types: missionEventTypes,
    stream,
  })

  return (
    <>
      <Section title="Waiting on a decision" stale={asks.stale}>
        <Freshness stale={asks.stale} error={asks.error} hasData={asks.data !== null} />
        {asks.data && asks.data.items.length === 0 ? (
          <EmptyState
            what="Nothing is waiting on you."
            why="Approvals, gate questions and interview cards land here the moment a run needs a person. They are answered in the inbox."
          />
        ) : (
          <ul className="rows">
            {(asks.data?.items ?? []).map((item) => (
              <li className="row flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-border py-2" key={item.id}>
                <ApprovalLine item={item} />
              </li>
            ))}
          </ul>
        )}
      </Section>

      <Section title="Ready to review" stale={review.stale}>
        <Freshness stale={review.stale} error={review.error} hasData={review.data !== null} />
        {review.data && review.data.deliverables.length === 0 ? (
          <EmptyState
            what="Nothing is waiting for a review."
            why="A deliverable appears here when a worker finishes one and its delivery policy needs a person to look before it goes out."
          />
        ) : (
          <ul className="rows">
            {(review.data?.deliverables ?? []).map((d) => (
              <li className="row flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-border py-2" key={d.deliverable_id}>
                <DeliverableLine deliverable={d} />
              </li>
            ))}
          </ul>
        )}
      </Section>
    </>
  )
}

function ApprovalLine({ item }: { item: ApprovalItem }) {
  return (
    <>
      {/* The honest door: the card is ANSWERED on the inbox surface, which is
          B6-6's. This filter says it is waiting and takes you there. */}
      <Link to={hrefFor('inbox-item', { id: item.id })} className="font-mono text-sm tabular-nums">
        {item.id}
      </Link>
      <Chip className="run-state">{item.kind}</Chip>
      <Chip className={item.tier === 'high' ? 'warn-flag' : 'muted'} tone={item.tier === 'high' ? 'red' : 'accent'}>
        {item.tier}
      </Chip>
      <Owner id={item.owner} />
      {item.answerable ? (
        <span className="notice text-xs">yours to answer</span>
      ) : (
        <Absent reason={item.not_answerable_reason ?? 'not yours to answer'} />
      )}
      {item.step_up_required && (
        <Chip className="warn-flag" tone="orange">
          PIN required
        </Chip>
      )}
      {item.stale && (
        <span className="warn-flag text-xs">{(item.stale_reasons ?? ['stale']).join('; ')}</span>
      )}
      <span className="muted ms-auto text-xs">
        seen <Timestamp ts={item.observed_ts} variant="live" />
      </span>
    </>
  )
}

function DeliverableLine({ deliverable }: { deliverable: Deliverable }) {
  return (
    <>
      <Link
        to={hrefFor('deliverable', { id: deliverable.deliverable_id })}
        className="font-mono text-sm tabular-nums"
      >
        {deliverable.deliverable_id}
      </Link>
      <span className="muted text-xs">
        {deliverable.type} · revision <span className="font-mono tabular-nums">{String(deliverable.current_revision)}</span>
      </span>
      <Owner id={deliverable.owner} />
      <Link to={hrefFor('task', { id: deliverable.task_id })} className="font-mono text-sm tabular-nums">
        {deliverable.task_id}
      </Link>
      <span className="muted ms-auto text-xs">
        updated <Timestamp ts={deliverable.updated_ts} variant="live" />
      </span>
    </>
  )
}

/** What each run filter asks for, in the reader's words — so an empty answer
 *  says what would have filled it rather than only that nothing did. */
const filterMeaning: Record<FilterID, string> = {
  'what-needs-me': 'Work that is waiting on a person.',
  mine: 'Runs the platform recorded under your name. The server answers this one — it is asked as your own rows, not sieved here.',
  running: 'Runs the platform is working on right now. A queued or parked run is not one of them.',
  'finished-today': 'Runs that ENDED since midnight where you are. Something that only moved today is not finished.',
}

/** mine / running / finished-today: three questions over the run list, each a
 *  server-side filter rather than a client-side sieve. */
function RunFilter({ id, me, stream }: { id: FilterID; me: string; stream?: EventStream }) {
  const params =
    id === 'mine'
      ? { person: me }
      : id === 'running'
        ? { status: 'running' }
        : { since: localMidnightISO(new Date()) }
  const key = `/api/runs:${id}:${JSON.stringify(params)}`
  const { data, error, stale } = useLive({
    key,
    read: () => api.runs(params),
    types: missionEventTypes,
    stream,
  })

  // finished-today asks the server for everything since local midnight; which
  // of those rows have actually FINISHED is a property of the stored state, so
  // it is read off the row rather than asked for as a status the API has no
  // single value for.
  const rows = (data?.runs ?? []).filter((r) => id !== 'finished-today' || terminal.includes(r.state))

  return (
    <Section title={filters.find((f) => f.id === id)?.label ?? id} stale={stale}>
      <Freshness stale={stale} error={error} hasData={data !== null} />
      {data && rows.length === 0 ? (
        <EmptyState what="Nothing matches." why={filterMeaning[id]} />
      ) : (
        <ul className="rows" data-filter={id}>
          {rows.map((r) => (
            <li className="row flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-border py-2" key={r.run_id}>
              <FilterRunLine run={r} />
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}

const terminal = ['completed', 'crashed', 'finalized', 'tombstoned', 'died-at-gate']

function FilterRunLine({ run }: { run: RunListItem }) {
  return (
    <>
      {run.task_id !== '' ? (
        <Link to={hrefFor('task', { id: run.task_id })} className="font-mono text-sm tabular-nums">
          {run.task_id}
        </Link>
      ) : (
        <Absent reason="no task — this run stands alone" />
      )}
      <Chip className="run-state" tone={run.state === 'running' ? 'green' : 'accent'}>
        {run.state}
      </Chip>
      <Owner id={run.owner} />
      <span className="run-lane font-mono text-xs text-muted-foreground">{run.lane}</span>
      {run.waiting_on_human && <span className="waiting-human text-xs text-[var(--orange)]">waiting on a person</span>}
      {run.state === 'parked' && <ParkedUntil until={run.parked_until} />}
      <span className="muted ms-auto text-xs">
        <Timestamp ts={run.last_activity_ts} variant="live" />
      </span>
    </>
  )
}
