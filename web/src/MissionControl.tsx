import { api, type MeterView, type Meters, type RunListItem } from './api'
import type { EventStream } from './events'
import { FilterBar, FilterView, filterFromSearch } from './Filters'
import { AnswerView, HistoryPanel } from './History'
import { missionEventTypes, useLive } from './live'
import { Absent, Empty, Freshness, Owner, ParkedUntil, Section, Stamp } from './parts'
import { Link } from './router'
import { hrefFor } from './routes'

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
      <h1>Mission control</h1>
      <FilterBar active={filter} />
      {filter !== '' && <FilterView id={filter} me={me} stream={stream} />}
      <Freshness stale={work.stale} error={work.error} hasData={work.data !== null} />

      {buckets.map((b) => (
        <Section title={b.title} key={b.id} stale={work.stale}>
          {b.runs.length === 0 ? (
            <Empty what="Nothing here." />
          ) : (
            <ul className="rows" data-bucket={b.id}>
              {b.runs.map((r) => (
                <li key={r.run_id} className="row">
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

/** One work item. Every item drills through to its task detail — built with
 *  `hrefFor` over the URL contract, never a hand-assembled path. */
function RunLine({ run }: { run: RunListItem }) {
  return (
    <>
      {run.task_id !== '' ? (
        <Link to={hrefFor('task', { id: run.task_id })}>{run.task_id}</Link>
      ) : (
        <Absent reason="no task — this run stands alone" />
      )}
      <span className="run-state">{run.state}</span>
      {run.wedged && <span className="warn-flag">wedged</span>}
      <Owner id={run.owner} />
      <span className="run-stage">{run.stage !== '' ? run.stage : <Absent reason="no stage marker yet" />}</span>
      <span className="run-lane">{run.lane}</span>
      {run.waiting_on_human && <span className="waiting-human">waiting on a person</span>}
      {run.state === 'parked' && <ParkedUntil until={run.parked_until} />}
      <span className="muted">
        last activity <Stamp ts={run.last_activity_ts} />
      </span>
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
                      {l.pressure_applicable && l.pressure !== null ? (
                        String(l.pressure)
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
          {meters.lanes.length === 0 && <Empty what="No lane has run anything yet." />}

          <ViewBlock title="Burn rate (per person, per observed day)" view={meters.burn_rates} />
          <ViewBlock title="Budget remainders" view={meters.budgets} />
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
    <div className="view-block">
      <h3>{title}</h3>
      {view.answer ? <AnswerView answer={view.answer} /> : <Absent reason={view.absent ?? 'not available'} />}
    </div>
  )
}
