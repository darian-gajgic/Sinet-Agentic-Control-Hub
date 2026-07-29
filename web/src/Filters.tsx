import { api, type ApprovalItem, type Deliverable, type RunListItem } from './api'
import type { EventStream } from './events'
import { missionEventTypes, useLive } from './live'
import { Absent, Empty, Freshness, Owner, ParkedUntil, Section, Stamp } from './parts'
import { Link, navigate } from './router'
import { hrefFor } from './routes'

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
        <Link key={f.id} to={hrefForFilter(f.id)} aria-current={f.id === active ? 'page' : undefined}>
          {f.label}
        </Link>
      ))}
      {active !== '' && (
        <button type="button" onClick={() => navigate(hrefFor('mission-control'))}>
          Clear
        </button>
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
          <Empty what="Nothing is waiting on you." />
        ) : (
          <ul className="rows">
            {(asks.data?.items ?? []).map((item) => (
              <li className="row" key={item.id}>
                <ApprovalLine item={item} />
              </li>
            ))}
          </ul>
        )}
      </Section>

      <Section title="Ready to review" stale={review.stale}>
        <Freshness stale={review.stale} error={review.error} hasData={review.data !== null} />
        {review.data && review.data.deliverables.length === 0 ? (
          <Empty what="Nothing is waiting for a review." />
        ) : (
          <ul className="rows">
            {(review.data?.deliverables ?? []).map((d) => (
              <li className="row" key={d.deliverable_id}>
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
      <Link to={hrefFor('inbox-item', { id: item.id })}>{item.id}</Link>
      <span className="run-state">{item.kind}</span>
      <span className={item.tier === 'high' ? 'warn-flag' : 'muted'}>{item.tier}</span>
      <Owner id={item.owner} />
      {item.answerable ? (
        <span className="notice">yours to answer</span>
      ) : (
        <Absent reason={item.not_answerable_reason ?? 'not yours to answer'} />
      )}
      {item.step_up_required && <span className="warn-flag">PIN required</span>}
      {item.stale && <span className="warn-flag">{(item.stale_reasons ?? ['stale']).join('; ')}</span>}
      <span className="muted">
        seen <Stamp ts={item.observed_ts} />
      </span>
    </>
  )
}

function DeliverableLine({ deliverable }: { deliverable: Deliverable }) {
  return (
    <>
      <Link to={hrefFor('deliverable', { id: deliverable.deliverable_id })}>{deliverable.deliverable_id}</Link>
      <span className="muted">
        {deliverable.type} · revision {String(deliverable.current_revision)}
      </span>
      <Owner id={deliverable.owner} />
      <Link to={hrefFor('task', { id: deliverable.task_id })}>{deliverable.task_id}</Link>
      <span className="muted">
        updated <Stamp ts={deliverable.updated_ts} />
      </span>
    </>
  )
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
        <Empty what="Nothing matches." />
      ) : (
        <ul className="rows" data-filter={id}>
          {rows.map((r) => (
            <li className="row" key={r.run_id}>
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
        <Link to={hrefFor('task', { id: run.task_id })}>{run.task_id}</Link>
      ) : (
        <Absent reason="no task — this run stands alone" />
      )}
      <span className="run-state">{run.state}</span>
      <Owner id={run.owner} />
      <span className="run-lane">{run.lane}</span>
      {run.waiting_on_human && <span className="waiting-human">waiting on a person</span>}
      {run.state === 'parked' && <ParkedUntil until={run.parked_until} />}
      <span className="muted">
        <Stamp ts={run.last_activity_ts} />
      </span>
    </>
  )
}
