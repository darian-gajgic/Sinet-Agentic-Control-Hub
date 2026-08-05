import { api, type MeterView, type Meters, type RunListItem } from './api'
import type { EventStream } from './events'
import { FilterBar, FilterView, filterFromSearch } from './Filters'
import { AnswerView, HistoryPanel } from './History'
import { missionEventTypes, useLive } from './live'
import {
  Absent,
  Freshness,
  Owner,
  ParkedUntil,
  Section,
  StallBanner,
  SurfaceHead,
  ThresholdRing,
} from './parts'
import { Link } from './router'
import { hrefFor } from './routes'
import { Chip, EmptyState, StatTile, StatusDot, Timestamp, type Tone } from './ui'

/**
 * Mission control (Spec S15.5 ¶1; S3.1): one live screen of the household's
 * work — everything running, queued, parked, blocked on a human, and recently
 * finished, with the consumption meters beside it and the filterable history
 * under it.
 *
 * WHO OWNS WHAT IS ON EVERY ITEM. The server has already scoped the read — a
 * member's answer contains only their own rows — so this view filters nothing
 * for privacy and hides no owner. Showing less than was served would be a
 * second, weaker access-control implementation living in a browser (S15.2).
 */

/**
 * The recently-finished window.
 *
 * A structural display constant with its reason (§41 precedent; no ⚙ key
 * exists and the browser cannot read the registry before login): 24 hours,
 * because mission control answers "what happened since I last looked" and a day
 * is the span a household's work is actually planned across. It changes only
 * what is SHOWN — every finished run remains readable through the run and task
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
  queued: { tone: 'blue', why: 'Accepted and waiting its turn — the scheduler starts it when the lane has room.' },
  blocked: {
    tone: 'orange',
    why: 'Parked with an open question, so it moves again only once a person answers it in the inbox.',
  },
  parked: { tone: 'yellow', why: 'Stopped on a clock rather than on a person — it resumes at its own horizon.' },
  finished: { tone: 'accent', why: 'Ended in the last day. Older work stays readable from its task and the history below.' },
  other: {
    tone: 'pink',
    why: 'A state these five do not name — mid-dispatch, draining, or one a later version added. It is shown rather than hidden.',
  },
}

const meaningOf = (id: string) => bucketMeaning[id] ?? { tone: 'accent' as Tone, why: '' }

/**
 * bucketRuns sorts served rows into the five S15.5 buckets, plus an honest
 * catch-all.
 *
 * The buckets are DISJOINT, and the ordering of the tests is the reason:
 * blocked-on-a-human is a strict subset of parked (the server derives it as
 * "parked with an open ask"), so a run waiting on a person is shown under the
 * person bucket only and the parked bucket is left meaning "waiting on a
 * clock". The final bucket exists so that a state none of the five name — a
 * claimed or draining run today, a state a later packet adds tomorrow — is
 * still on the screen. A run must never vanish from mission control because
 * this list did not anticipate its state.
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
  const work = useLive({
    key: '/api/runs',
    read: () => api.runs(),
    types: missionEventTypes,
    stream,
  })
  const meters = useLive({
    key: '/api/meters',
    read: () => api.meters(),
    types: missionEventTypes,
    stream,
  })

  // Read once per render rather than on a ticker: this view re-renders when the
  // feed says something changed, and a clock of its own would be the one thing
  // §32 rules out.
  const buckets = bucketRuns(work.data?.runs ?? [], Date.now())

  return (
    <section className="surface">
      <SurfaceHead
        title="Mission control"
        what="One live screen of the household's work — what is running, queued, parked, blocked on a human and recently finished — with the consumption meters beside it."
      />
      <FilterBar active={filter} />
      {filter !== '' && <FilterView id={filter} me={me} stream={stream} />}
      <Freshness stale={work.stale} error={work.error} hasData={work.data !== null} />

      {/* A count of the rows ON SCREEN, which is presentation over the served
          list rather than a figure of its own. The labels say so, and no tile
          carries a number no list below it carries. */}
      <div className="my-4 grid grid-cols-2 gap-(--density-gap) sm:grid-cols-3 lg:grid-cols-6" data-tiles="buckets">
        {buckets.map((b) => (
          <StatTile
            key={b.id}
            label={b.title}
            tone={meaningOf(b.id).tone}
            value={String(b.runs.length)}
            foot="runs on screen"
          />
        ))}
      </div>

      {buckets.map((b) => (
        <Section title={b.title} key={b.id} stale={work.stale}>
          <p className="mt-0 mb-2 max-w-prose text-xs text-muted-foreground" data-bucket-why={b.id}>
            {meaningOf(b.id).why}
          </p>
          {b.id === 'blocked' && b.runs.length > 0 && <BlockedDoor />}
          {b.runs.length === 0 ? (
            <EmptyState what={`Nothing is ${b.title.toLowerCase()}.`} why={meaningOf(b.id).why} />
          ) : (
            <ul className="rows" data-bucket={b.id}>
              {b.runs.map((r) => (
                <li key={r.run_id} className="row flex flex-col gap-1 border-b border-border py-2">
                  <RunLine run={r} />
                </li>
              ))}
            </ul>
          )}
        </Section>
      ))}

      <MetersPanel meters={meters.data} stale={meters.stale} error={meters.error} />
      <HistoryPanel />
    </section>
  )
}

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

/** One work item. Every item drills through to its task detail — built with
 *  `hrefFor` over the URL contract, never a hand-assembled path.
 *
 *  A row carries no card id — the run list serves none — so a stopped row's
 *  door points at the SURFACE where its ask is answered rather than at a card
 *  this client would have had to guess (§42-B). */
function RunLine({ run }: { run: RunListItem }) {
  return (
    <>
      <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <StatusDot tone={run.wedged ? 'red' : run.state === 'running' ? 'green' : 'accent'} live={run.state === 'running'} />
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
        {run.wedged && (
          <Chip className="warn-flag" tone="red">
            wedged
          </Chip>
        )}
        <Owner id={run.owner} />
        <span className="run-stage text-xs text-muted-foreground">
          {run.stage !== '' ? run.stage : <Absent reason="no stage marker yet" />}
        </span>
        <span className="run-lane font-mono text-xs text-muted-foreground">{run.lane}</span>
        {run.waiting_on_human && <span className="waiting-human text-xs text-[var(--orange)]">waiting on a person</span>}
        {run.state === 'parked' && <ParkedUntil until={run.parked_until} />}
        <span className="muted ms-auto text-xs">
          last activity <Timestamp ts={run.last_activity_ts} variant="live" />
        </span>
      </span>
      {run.wedged && (
        <StallBanner
          kind="wedged"
          tone="red"
          what="Flagged and paused: the platform stopped this run rather than let it loop, and it waits for a person."
        >
          <Link to={hrefFor('inbox')}>Its card is in the inbox</Link>
        </StallBanner>
      )}
    </>
  )
}

/**
 * The meters (S15.5; S2.5; S10 through /api/meters), rendered faithfully.
 *
 * Three honesties this panel exists to keep:
 *  - the S10.4 gauge's `assumed` label rides every reading it applies to — the
 *    assumption is stated, never dropped (G1 Def.10);
 *  - pressure renders ONLY when the gauge says it applies, because a pressure
 *    figure without a declared budget would be a fabricated denominator;
 *  - burn rates render at the view's OWN grain, which is per PERSON. This
 *    client never divides a person's rate across their lanes to fill a
 *    per-lane column — money is read, never computed (§37).
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
                      {/* The ring is the ONE capacity idiom on these surfaces,
                          and it renders only what the gauge served: the two
                          landed gates are the whole condition, and the figure
                          stays beside it because a ring is a glance and a
                          number is the answer. */}
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

          {/* What the ring means, in one line. It is deliberately about the
              DECLARATION and never about a cutoff: where background work stops
              being admitted is a server setting no browser can read. */}
          <p className="mt-2 text-xs text-muted-foreground" data-ring-legend>
            The ring beside a pressure figure shows that figure as a share of the declared budget for that lane — green
            below three quarters, orange from there, red once the declared budget is reached.
          </p>

          <ViewBlock title="Burn rate (per person, per observed day)" view={meters.burn_rates} />
          <ViewBlock title="Budget remainders" view={meters.budgets} />
          {/* Never stall silently: a limit event is the platform having stopped
              admitting work, and it says so in words with the door to where a
              stopped run is released. Rendered only when the view really
              carries rows — a banner over an empty answer would be an alarm
              about nothing. */}
          {(meters.limit_events.answer?.rows.length ?? 0) > 0 && (
            <StallBanner
              kind="limit"
              tone="orange"
              what="A limit was reached on at least one lane below. New background work stops being started there and running work parks at its next safe boundary — nothing is discarded."
            >
              <Link to={hrefFor('inbox')}>Parked runs are released from their cards in the inbox</Link>
            </StallBanner>
          )}
          <ViewBlock title="Limit events" view={meters.limit_events} />
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
