import { useState } from 'react'

import { api } from './api'
import type { EventStream } from './events'
import { missionEventTypes, useLive } from './live'
import { MetersPanel, ViewBlock } from './MissionControl'
import { Absent, Freshness, Owner, ParkedUntil, Section } from './parts'

/**
 * Fleet overview (Spec S15.5 ¶4; 9.3; S2.5; S3.10; D2).
 *
 * What is running on whose account at what burn rate. Two things this surface
 * is built around:
 *
 * ACCOUNTS ARE ALWAYS DISTINGUISHED. Every figure carries its owner, and rows
 * are NEVER summed across owners. A household total would be money arithmetic
 * the client must not do — and it would answer a question nobody on this
 * platform asks, because the whole point of D2 is that each person's account
 * burns separately.
 *
 * FILTERS NARROW THE DISPLAY, NOT THE AUTHORIZATION. The server has already
 * scoped the read; the person/lane filters here are presentation over rows the
 * caller was allowed to see, exactly as S15.2 requires.
 */
export function Fleet({ stream }: { stream?: EventStream }) {
  const [person, setPerson] = useState('')
  const [lane, setLane] = useState('')
  const { data, error, stale } = useLive({
    key: '/api/meters',
    read: () => api.meters(),
    types: missionEventTypes,
    stream,
  })

  const lanes = data?.lanes ?? []
  const shown = lanes.filter((l) => (person === '' || l.owner === person) && (lane === '' || l.lane === lane))
  const people = [...new Set(lanes.map((l) => l.owner))].sort()
  const laneNames = [...new Set(lanes.map((l) => l.lane))].sort()

  return (
    <section className="surface">
      <h1>Fleet</h1>
      <Freshness stale={stale} error={error} hasData={data !== null} />

      <div className="fleet-filters">
        <label>
          Whose
          <select value={person} onChange={(e) => setPerson(e.target.value)}>
            <option value="">Everyone you can see</option>
            {people.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </label>
        <label>
          Lane
          <select value={lane} onChange={(e) => setLane(e.target.value)}>
            <option value="">Every lane</option>
            {laneNames.map((l) => (
              <option key={l} value={l}>
                {l}
              </option>
            ))}
          </select>
        </label>
      </div>

      <Section title="Limit status" stale={stale}>
        <div className="table-scroll">
          <table className="fleet-lanes">
            <thead>
              <tr>
                <th>Whose</th>
                <th>Lane</th>
                <th>Active</th>
                <th>Parked</th>
                <th>Until</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((l) => (
                <tr key={`${l.owner}/${l.lane}`} data-owner={l.owner} data-lane={l.lane}>
                  <td>
                    <Owner id={l.owner} />
                  </td>
                  <td>{l.lane}</td>
                  <td>{String(l.active_runs)}</td>
                  <td>{String(l.parked_runs)}</td>
                  <td>{l.parked_runs > 0 ? <ParkedUntil until={l.parked_until} /> : <span className="muted">—</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {shown.length === 0 && <p className="muted">No lane matches this filter.</p>}
      </Section>

      <MetersPanel meters={data} stale={stale} error={error} />

      {data && (
        <Section title="Cost, at each view's own grain" stale={stale}>
          <ViewBlock title="Per person" view={data.per_person} />
          <ViewBlock title="Per period" view={data.per_period} />
        </Section>
      )}

      <LocalTierSeams />
    </section>
  )
}

/**
 * The declared v0 absences (B6-5 OQ8, ratified: render the named honest
 * absence).
 *
 * `FleetSnapshot.local_seats` is an empty slice and `gpu` is nil — not because
 * nothing is running locally, but because the `internal/local` seat read and
 * the B4-6 VRAM read are not wired to this surface yet. Saying "0 seats" or
 * "0 GB VRAM" would be a reading; there is no reading. Saying nothing at all
 * would let a person infer the local tier is idle. So the seam is named, with
 * what it is waiting for.
 */
export function LocalTierSeams() {
  return (
    <Section title="Local tier">
      <p>
        Local duty seats:{' '}
        <Absent reason="not wired at v0 — the seat read lands with the B6 fleet bring-up, so this is an unwired instrument, not an idle tier" />
      </p>
      <p>
        GPU / VRAM:{' '}
        <Absent reason="not wired at v0 — the VRAM read lands with the local tier's own surface, and a zero here would be a reading nobody took" />
      </p>
    </Section>
  )
}
