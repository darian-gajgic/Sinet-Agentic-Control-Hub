import { useState, type ReactNode } from 'react'

import { api, type AutomationState, type MeterLane, type Meters } from './api'
import { ActConfirm, OutcomeLine, outcomeOf, useAct } from './controls'
import type { EventStream } from './events'
import { missionEventTypes, useLive } from './live'
import { MetersPanel, ViewBlock } from './MissionControl'
import { Absent, Freshness, Owner, ParkedUntil, Section, StallBanner, SurfaceHead } from './parts'
import { Link } from './router'
import { hrefFor } from './routes'
import { Button, EmptyState } from './ui'

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
export function Fleet({
  me,
  operator,
  stream,
}: {
  /** The signed-in person and whether they act as the operator, both handed
   *  down by the shell from its ONE session read. */
  me: string
  operator: boolean
  stream?: EventStream
}) {
  const [person, setPerson] = useState('')
  const [lane, setLane] = useState('')
  const may = (owner: string) => operator || owner === me
  const { data, error, stale, reload } = useLive({
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
      <SurfaceHead
        title="Fleet"
        what="What is running on whose account, at what burn rate. Every figure stays with the person whose account it came from and is never added up across people, because each account burns separately."
      />
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
        {/* Never stall silently: a lane holding parked runs says so above the
            table, with the horizon in the row and the door to where a parked
            run is actually released. This points at the inbox; the release verb
            is the card's own and is not re-implemented here (§48 OQ2). */}
        {shown.some((l) => l.parked_runs > 0) && (
          <StallBanner
            kind="parked"
            tone="yellow"
            what="Some of these lanes are holding parked runs. They wait — nothing queued or parked is thrown away — and each one carries its own horizon in the Until column."
          >
            <Link to={hrefFor('inbox')}>A parked run is released from its card in the inbox</Link>
          </StallBanner>
        )}
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
        {/* Gated on the served read: "no lane matches" is a statement about an
            answer, and before one arrives `Freshness` above says so instead. */}
        {shown.length === 0 && data !== null && (
          <EmptyState
            what="No lane matches this filter."
            why="A lane appears once work has run on it. These selectors narrow what is shown; they never narrow what you were allowed to see."
          />
        )}
      </Section>

      <AutomationPanel meters={data} may={may} reload={reload} />

      <MetersPanel meters={data} stale={stale} error={error} />

      <BudgetPanel lanes={shown} meters={data} may={may} reload={reload} />

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
 * mayAct answers the ONE question both controls on this surface ask: may the
 * signed-in person act on this owner's subject?
 *
 * It states the SAME authority rule the two verbs enforce — own + operator-any
 * (D10, §39-B OQ4) — rather than inferring one, which is the B6-6 D9 lesson: a
 * surface that renders a control the door will refuse has invented an authority,
 * and one that hides a control the door would allow has invented a refusal. The
 * operator limb mirrors the server's own `isOperatorRead`, dev posture included,
 * because that is the predicate the read on the other side of this call used.
 *
 * IDENTITY IS HANDED DOWN BY THE SHELL (P3-UI-5; this CLOSES the §48 deviation
 * that had this surface read `GET /api/auth/session` itself). `App.tsx` holds
 * the one session read and gives Fleet the same identity it gives Board,
 * MissionControl and Deliverable — one read, one source (§41-B). The predicate
 * is byte-preserved from the read it replaces, dev posture included, because
 * that is the predicate the server's own `isOperatorRead` used on the other
 * side of these calls. It is deliberately NOT the users-row-role-only flag the
 * memory surface takes: the S09 gate resolves an actor against that row, and
 * these two verbs do not.
 *
 * Until the session lands nothing is actionable, and that property did not move
 * — it is now the shell's own `session === null` arm, which renders no surface
 * at all rather than a surface with its controls withheld.
 */
type MayAct = (owner: string) => boolean

/**
 * The 3.3 pause switch, one per person this read is about (S10.4; §39-B).
 *
 * WHOSE SWITCHES APPEAR IS THE SERVER'S ANSWER, NOT A CLIENT DECISION. The read
 * is owner-scoped, so a member is served their own position and nobody else's,
 * and the operator is served the household's — which is exactly the D10
 * administer path, without this surface ever guessing at a person. Pressing a
 * row's control names that row's owner in the request, so the subject of the act
 * is the subject on screen.
 *
 * When no position could be read the switch is NOT offered. The absence says
 * why; a control over a position the platform cannot see would be asking
 * somebody to flip a switch nobody can find.
 */
function AutomationPanel({ meters, may, reload }: { meters: Meters | null; may: MayAct; reload: () => void }) {
  const served = meters?.automation.states ?? []
  const mine = served.filter((s) => may(s.owner))
  const absent = meters?.automation.absent ?? ''
  return (
    <Section title="Automation">
      {/* The pre-act explainer (D8): what the switch DOES, in the platform's
          own terms, before anybody presses it. Every clause is a landed S10.4
          guarantee — the preservation clause most of all, which is the one
          people are right to worry about. */}
      <p className="automation-explainer">
        Pausing stops the platform from starting any new background work for that person. Work already running parks
        itself at its next safe boundary. <strong>Nothing queued and nothing parked is thrown away</strong> — it waits,
        and resuming lets it carry on. Talking to the platform yourself is never held back by this switch.
      </p>
      {absent !== '' && <Absent reason={absent} />}
      {mine.length > 0 && (
        <ul className="automation-switches">
          {mine.map((s) => (
            <PauseSwitch key={s.owner} state={s} reload={reload} />
          ))}
        </ul>
      )}
      {absent === '' && served.length > 0 && mine.length === 0 && (
        <EmptyState
          what="No automation switch here is yours to flip."
          why="A switch belongs to the person whose background work it stops. You are shown everybody you may see, and offered only your own."
        />
      )}
    </Section>
  )
}

/**
 * ONE person's switch, as TWO position verbs — never a toggle.
 *
 * The verb itself refuses a request that did not say which way it wanted the
 * switch, and that refusal is a design this control keeps rather than works
 * around: both buttons always state the position they ask for, so a stale render
 * can never send the opposite of what the person meant. The current position is
 * READ (the meters family serves it) rather than remembered from the last act.
 */
function PauseSwitch({ state, reload }: { state: AutomationState; reload: () => void }) {
  const act = useAct()
  const flip = (paused: boolean) => {
    act.run(
      () =>
        api
          .setAutomationPause({ person: state.owner, paused })
          .then((res) => outcomeOf(res.changed, res.changed ? 'the switch moved' : 'it was already there', res.detail)),
      reload,
    )
  }
  return (
    <li className="automation-switch" data-owner={state.owner} data-paused={String(state.paused)}>
      <p>
        <Owner id={state.owner} />{' '}
        <span className="automation-position">
          {state.paused ? 'background work is paused' : 'background work is running'}
        </span>
      </p>
      <div className="flex flex-wrap gap-2">
        <Button variant="secondary" size="sm" data-pause="true" disabled={act.busy} onClick={() => flip(true)}>
          Pause this automation
        </Button>
        <Button variant="secondary" size="sm" data-pause="false" disabled={act.busy} onClick={() => flip(false)}>
          Resume this automation
        </Button>
      </div>
      <OutcomeLine outcome={act.outcome} />
      {/* Where a run that parked while paused is released: its OWN resume, on
          the card that is holding it. This points; it does not re-implement. */}
      <p className="muted">
        A single run that parked and should go again is released from{' '}
        <Link to={hrefFor('inbox')}>the inbox</Link>, where it is waiting.
      </p>
    </li>
  )
}

/**
 * The S10.4 budget editor, per (owner, lane) — the grain a budget is declared
 * at, and the grain the gauge reads at (D4).
 *
 * THIS PANEL RENDERS NO FIGURE. The declared budget, the pressure and the
 * remaining figure are all SERVED, and they render where they are served: in the
 * meters panel above, absences and all. An editor that also printed its own copy
 * of them would be a second answer to a question the platform already answers —
 * and the moment they disagreed, the platform would be lying on one of the two
 * lines. So this offers the act, and the reading stays where the reading lives.
 */
function BudgetPanel({
  lanes,
  meters,
  may,
  reload,
}: {
  lanes: MeterLane[]
  meters: Meters | null
  may: MayAct
  reload: () => void
}) {
  if (meters === null) return null
  const mine = lanes.filter((l) => may(l.owner))
  return (
    <Section title="Automation budgets">
      <p className="budget-explainer">
        A budget is a ceiling on how much automation the platform will start for one person in one lane. The gauge above
        measures that person&apos;s consumption against it, and background work stops being admitted once the pressure
        reaches the platform&apos;s threshold. It never stops you working with the platform yourself. It is counted in
        the gauge&apos;s own weighted-consumption units — not money, and not anything this screen converts.
      </p>
      {mine.length === 0 ? (
        <EmptyState
          what="No lane here is yours to put a budget on."
          why="A budget is declared per person per lane, by that person or by the operator. Lanes you can read but not act on stay readable above."
        />
      ) : (
        <ul className="budget-rows">
          {mine.map((l) => (
            <BudgetRow key={`${l.owner}/${l.lane}`} lane={l} unit={servedUnit(meters)} reload={reload} />
          ))}
        </ul>
      )}
    </Section>
  )
}

/**
 * servedUnit reads the unit label OFF THE WIRE — the `budget_unit` column of the
 * Layer-0 budget view — and returns the empty string when nothing has been
 * declared yet, because before a declaration the platform has served no unit
 * string at all. The editor then names the unit in words in its own explainer,
 * which is prose about what the figure means, rather than printing a label it
 * invented and passing it off as the platform's.
 */
function servedUnit(meters: Meters): string {
  const answer = meters.budgets.answer
  if (!answer) return ''
  const at = answer.columns.indexOf('budget_unit')
  if (at < 0) return ''
  for (const row of answer.rows) {
    const value = (row as unknown[])[at]
    if (typeof value === 'string' && value !== '') return value
  }
  return ''
}

/**
 * wholePositive is the budget field's own parse, and `Number.parseInt` is
 * deliberately NOT it.
 *
 * `parseInt('3.5')` is 3 and `parseInt('3e5')` is 3: both read a typed figure as
 * a DIFFERENT one and fire it without a word. A budget is the wrong place in
 * this platform to be approximately right, so anything that is not a plain run
 * of digits is refused with its reason and nothing is sent.
 */
function wholePositive(raw: string): number | null {
  const trimmed = raw.trim()
  if (!/^[0-9]+$/.test(trimmed)) return null
  const value = Number(trimmed)
  return Number.isSafeInteger(value) && value > 0 ? value : null
}

/** One (owner, lane) row's editor door. The label says which act it is, from the
 *  SERVED declaration state — never from anything this client remembers. */
function BudgetRow({ lane, unit, reload }: { lane: MeterLane; unit: string; reload: () => void }) {
  const [open, setOpen] = useState(false)
  const act = useAct()
  const [tokens, setTokens] = useState('')
  const [days, setDays] = useState('')
  const [start, setStart] = useState('')
  const [why, setWhy] = useState('')
  const figure = wholePositive(tokens)
  const period = wholePositive(days)
  // Mirrors the verb's own bounds so an obviously-refused request does not need
  // a round trip. It never REPLACES the server's answer: anything that fires
  // renders whatever the server says about it.
  const budget = figure !== null && period !== null ? { period_tokens: figure, period_days: period } : null
  // A typed figure the field cannot read is REFUSED with the reason, never
  // quietly rounded into a different one.
  const held =
    tokens.trim() !== '' && figure === null
      ? 'A budget is a whole number of units — write it out in full, with no decimal point and no exponent.'
      : days.trim() !== '' && period === null
        ? 'A period is a whole number of days.'
        : ''

  return (
    <li className="budget-row" data-owner={lane.owner} data-lane={lane.lane}>
      <p>
        <Owner id={lane.owner} /> · <span className="lane-name">{lane.lane}</span>
      </p>
      <Button
        variant="primary"
        size="sm"
        data-declare-budget={`${lane.owner}/${lane.lane}`}
        onClick={() => {
          act.clear()
          setOpen(true)
        }}
      >
        {lane.budget_declared ? 'Change this budget' : 'Declare a budget'}
      </Button>
      <ActConfirm
        open={open}
        onOpenChange={setOpen}
        title={`Budget for ${lane.owner} in ${lane.lane}`}
        what={`Once this is declared the gauge measures ${lane.owner}'s ${lane.lane} consumption against it straight away, in the gauge's own weighted-consumption units — a measure of automation, not money, and nothing here converts it into any. To stop this person's automation outright, use the pause switch instead.`}
        act="Declare this budget"
        variant="primary"
        busy={act.busy || budget === null}
        onConfirm={() => {
          if (budget === null) return // the act row is disabled without it; the narrowing is said out loud
          setOpen(false)
          act.run(
            () =>
              api
                .declareBudget({
                  person: lane.owner,
                  lane: lane.lane,
                  ...budget,
                  ...(start === '' ? {} : { period_start: start }),
                  ...(why === '' ? {} : { reason: why }),
                })
                .then((res) => outcomeOf(true, declaredNote(res.budget.unit ?? unit, res.prior), res.detail)),
            reload,
          )
        }}
      >
        <div className="budget-form">
          <p className="muted">
            Lane: <span className="lane-name">{lane.lane}</span> — a budget belongs to one lane, and this is the row you
            opened.
          </p>
          <label>
            <span>How much{unit === '' ? '' : ` (${unit})`}</span>
            <input
              type="number"
              min="1"
              step="1"
              inputMode="numeric"
              className="font-mono tabular-nums"
              data-field="period_tokens"
              value={tokens}
              onChange={(e) => setTokens(e.currentTarget.value)}
            />
          </label>
          <label>
            <span>Over how many days</span>
            <input
              type="number"
              min="1"
              step="1"
              inputMode="numeric"
              className="font-mono tabular-nums"
              data-field="period_days"
              value={days}
              onChange={(e) => setDays(e.currentTarget.value)}
            />
          </label>
          {held !== '' && (
            <p className="warn-flag" data-held="true">
              {held}
            </p>
          )}
          <label>
            <span>Starting when (optional — left empty, it starts now)</span>
            <input
              type="text"
              data-field="period_start"
              placeholder="2026-08-05T00:00:00Z"
              value={start}
              onChange={(e) => setStart(e.currentTarget.value)}
            />
          </label>
          <label>
            <span>Why (optional — it is kept with the decision)</span>
            <input type="text" data-field="reason" value={why} onChange={(e) => setWhy(e.currentTarget.value)} />
          </label>
        </div>
      </ActConfirm>
      <OutcomeLine outcome={act.outcome} />
    </li>
  )
}

/**
 * The declaration's own account of what it replaced. A FIRST declaration has no
 * `prior` member, and the absence is what makes "there was no budget before"
 * true rather than assumed — nothing here fills it in with a zero.
 */
function declaredNote(unit: string, prior: { period_tokens: number } | undefined): ReactNode {
  const suffix = unit === '' ? '' : ` ${unit}`
  if (!prior) return `declared, and there was no budget on this lane before${suffix === '' ? '' : `; it is in${suffix}`}`
  // The figure is a served number, so it takes the mono/tabular treatment the
  // token contract gives every number on this platform (§47).
  return (
    <>
      declared, replacing <span className="font-mono tabular-nums">{String(prior.period_tokens)}</span>
      {suffix}
    </>
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
