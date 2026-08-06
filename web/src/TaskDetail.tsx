import { useEffect, useState, type ReactNode } from 'react'
import { ArrowUpRight, X } from 'lucide-react'

import {
  api,
  type CancelOutcome,
  type DeliverableDetail,
  type Receipt,
  type RunDetail,
  type Spec,
  type TaskCancelOutcome,
  type TaskDecision,
  type TaskDetail as Detail,
  type TaskRunView,
} from './api'
import { ActConfirm, OutcomeLine, outcomeOf, useAct } from './controls'
import type { EventStream } from './events'
import { activityEventTypes, boardEventTypes, inboxEventTypes, useLive } from './live'
import { Absent, Freshness, Money, Owner, ParkedUntil, Section, StallBanner, Stamp } from './parts'
import { Link, navigate } from './router'
import { hrefFor } from './routes'
import { Button, Chip, EmptyState, StatusDot, Timestamp, type Tone } from './ui'

/**
 * The task card (map §3 v3; Spec S15.5 ¶3; 9.2; S2.2; S2.4; G2 D2.8) — a
 * STRUCTURED OVERLAY WINDOW over the surface it was opened from, never a
 * full-page swap (operator, checkpoint 2). The task route stays its
 * deep-link address and renders the same window standalone.
 *
 * The whole story of one task: the specification with its numbered acceptance
 * criteria, the plan with per-step done-when, live activity and the stage
 * rail, every human decision, the deliverables with doors into review, the
 * receipts, and the lineage — under a state-computed action bar that offers
 * only what this task's state allows.
 *
 * TWO RULES SHAPE EVERY BLOCK, unchanged from the landed surface:
 *
 * The §38 ruling-(a) display contract: a pre-approval task serves its DRAFT
 * pair with its status on it, so this view labels a draft a draft and never
 * presents it as the confirmed specification.
 *
 * Render-verbatim-as-served for the receipt labels: the done-directly label
 * is REGISTERED text the platform puts on every receipt; this file declares
 * no label string of its own.
 *
 * TWO DEFECT CLASSES ARE BANNED HERE BY NAME (checkpoint-2 C2-13):
 *  - a raw internal error string must NEVER render as body text — absences
 *    and failures get plain words;
 *  - durations render human-readable through `fmtDuration`, never as raw
 *    seconds.
 */
export function TaskDetail({
  id,
  stream,
  onClose,
}: {
  id: string
  stream?: EventStream
  /** Closing the window returns to the surface it covered. */
  onClose?: () => void
}) {
  // Deliberately UNNARROWED: every frame of the declared types re-reads this
  // resource. Filtering to "frames whose run_id is one of this task's runs"
  // would drop the `run.created` frame for a run this task does not have YET.
  const { data, error, stale, reload } = useLive<Detail>({
    key: `/api/tasks/${id}`,
    read: () => api.task(id),
    types: boardEventTypes,
    stream,
  })

  const close = onClose ?? (() => { navigate(hrefFor('board')) })

  // Esc closes — the window behaves like the Nexus card it recreates.
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close()
    }
    window.addEventListener('keydown', h)
    return () => {
      window.removeEventListener('keydown', h)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div
      className="win-overlay"
      data-task-overlay={id}
      onClick={close}
      role="presentation"
    >
      <section
        className="win surface"
        role="dialog"
        aria-modal="true"
        aria-label={data ? data.title : 'Task'}
        onClick={(e) => {
          e.stopPropagation()
        }}
      >
        <header className="win-head">
          <div className="win-title">
            <h2 className="m-0">{data ? (data.title !== '' ? data.title : data.task_id) : 'Task'}</h2>
            {data && (
              <p className="win-under mono">
                {data.task_id} · <Owner id={data.owner} /> · opened <Timestamp ts={data.created_ts} variant="live" />
              </p>
            )}
          </div>
          {data && <Chip tone={statusTone(data.kanban_status)}>{statusWord(data.kanban_status)}</Chip>}
          <button type="button" className="win-close" aria-label="Close the task card" onClick={close}>
            <X size={16} strokeWidth={2} aria-hidden="true" />
          </button>
        </header>

        <div className="win-body">
          {/* The D8 teaching line, carried onto the window: what this card
              holds — claiming nothing about approval status (§38: that is
              StatusLine's job, per artifact, from the served status). */}
          <p className="win-what" data-surface-what>
            The whole story of one task — what was asked for and how it was planned, every stage the work passed
            through, what is happening right now, each decision a person made, the deliverables and the receipts.
          </p>
          <Freshness stale={stale} error={error} hasData={data !== null} />
          {data === null && error === '' && <p className="muted">Reading the task…</p>}
          {data && (
            <>
              <ActionBar detail={data} reload={reload} stream={stream} />
              <SpecBlock detail={data} stale={stale} />
              <StageBlock detail={data} stale={stale} />
              <LiveActivity run={activeRun(data)} stream={stream} />
              <DecisionsBlock decisions={data.decisions} stale={stale} />
              <DeliverablesBlock taskID={id} stream={stream} />
              <ReceiptsBlock runs={data.runs} stale={stale} reload={reload} />
            </>
          )}
        </div>
      </section>
    </div>
  )
}

/** The board's own display words for a stored status (kanban.ts vocabulary):
 *  the card and the column must not disagree. */
function statusWord(status: string): string {
  return status === 'intake' ? 'Backlog' : status === 'attention' ? 'Needs attention' : status
}

function statusTone(status: string): Tone {
  switch (status) {
    case 'executing':
      return 'green'
    case 'verifying':
      return 'blue'
    case 'attention':
      return 'orange'
    case 'cancelled':
      return 'red'
    case 'done':
      return 'accent'
    default:
      return 'blue'
  }
}

/** fmtDuration: seconds → human words. Raw seconds never render (C2-13). */
export function fmtDuration(seconds: number): string {
  const s = Math.max(0, Math.round(seconds))
  if (s < 60) return `${String(s)} s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${String(m)} min${s % 60 !== 0 ? ` ${String(s % 60)} s` : ''}`
  const h = Math.floor(m / 60)
  if (h < 48) return `${String(h)} h${m % 60 !== 0 ? ` ${String(m % 60)} min` : ''}`
  const d = Math.floor(h / 24)
  return `${String(d)} d${h % 24 !== 0 ? ` ${String(h % 24)} h` : ''}`
}

/* ── the state-computed action bar ───────────────────────────────────────── */

/**
 * Only what this state allows (map §3): cancel while something can be
 * cancelled; the open card's door while one is open; the review door while a
 * deliverable exists. Every verb here is a door or a landed verb — nothing is
 * invented, and a state that allows nothing renders no dead controls.
 */
function ActionBar({ detail, reload, stream }: { detail: Detail; reload: () => void; stream?: EventStream }) {
  const asks = useLive({
    key: `/api/approvals#task:${detail.task_id}`,
    read: () => api.approvals(),
    types: inboxEventTypes,
    stream,
  })
  const openAsk = (asks.data?.items ?? []).find((i) => i.task_id === detail.task_id)

  return (
    <div className="win-acts">
      {openAsk !== undefined && (
        <Button
          variant="primary"
          size="sm"
          data-act="answer"
          onClick={() => {
            navigate(hrefFor('inbox-item', { id: openAsk.id }))
          }}
        >
          <ArrowUpRight size={13} strokeWidth={2} aria-hidden="true" />
          Answer its open card
        </Button>
      )}
      <CancelTask taskID={detail.task_id} runs={detail.runs} reload={reload} />
    </div>
  )
}

/* ── the spec + plan ─────────────────────────────────────────────────────── */

/** R10: the spec with its numbered ACs and the plan, each labelled with the
 *  approval status it actually has. */
function SpecBlock({ detail, stale }: { detail: Detail; stale: boolean }) {
  const { spec, plan } = detail
  return (
    <Section title="Specification and plan" stale={stale}>
      {!spec || !plan ? (
        // Plain words for the absence (C2-13: the served internal reason
        // string is a fact about the pipeline, not a sentence for a person).
        <Absent reason="no confirmed spec and plan are stored yet — the interview and planning produce them, and they appear here the moment they exist" />
      ) : (
        <>
          <StatusLine what="Specification" status={spec.status} version={spec.version} />
          <p>{spec.restatement}</p>
          <h4>Acceptance criteria</h4>
          <ol className="acs">
            {spec.acs.map((ac) => (
              <li key={ac.n} data-ac={`AC-${ac.n}`}>
                <span className="ac-key">AC-{ac.n}</span> {ac.plain}
                {ac.structured && (
                  <span className="muted">
                    {' '}
                    — {ac.structured} ({ac.structured_kind})
                  </span>
                )}
              </li>
            ))}
          </ol>
          <SpecLists spec={spec} />

          <StatusLine what="Plan" status={plan.status} version={plan.version} />
          <ol className="steps">
            {plan.steps.map((s) => (
              <li key={s.id}>
                <span className="step-id">{s.id}</span> {s.title}
                <span className="muted">
                  {' '}
                  covers {Object.entries(plan.coverage)
                    .filter(([, steps]) => steps.includes(s.id))
                    .map(([ac]) => ac)
                    .join(', ') || 'nothing listed'}
                </span>
              </li>
            ))}
          </ol>
          {(plan.risks ?? []).length > 0 && (
            <ul className="risks">
              {plan.risks?.map((r) => (
                <li key={r}>{r}</li>
              ))}
            </ul>
          )}
        </>
      )}
    </Section>
  )
}

/**
 * StatusLine is the §38 ruling-(a) honesty, in one place so it cannot be
 * forgotten on one of the two artifacts: an approved pair reads as confirmed,
 * a draft reads as a draft and says what that means.
 */
function StatusLine({ what, status, version }: { what: string; status: string; version: number }) {
  const approved = status === 'approved'
  return (
    <h3 data-artifact-status={status}>
      {what} v{version} —{' '}
      <span className={approved ? 'notice' : 'warn-flag'}>
        {approved ? 'confirmed' : `${status} (not approved — nobody has signed this off yet)`}
      </span>
    </h3>
  )
}

function SpecLists({ spec }: { spec: Spec }) {
  const lists: { title: string; items: string[] }[] = [
    { title: 'Outcome', items: spec.outcome ?? [] },
    { title: 'Constraints', items: spec.constraints ?? [] },
    { title: 'Will not do', items: spec.out_of_scope ?? [] },
    { title: 'Open clarifications', items: spec.clarifications ?? [] },
    { title: 'Assumptions', items: (spec.assumptions ?? []).map((a) => (a.basis ? `${a.text} (${a.basis})` : a.text)) },
  ]
  return (
    <>
      {lists
        .filter((l) => l.items.length > 0)
        .map((l) => (
          <div key={l.title}>
            <h4>{l.title}</h4>
            <ul>
              {l.items.map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ul>
          </div>
        ))}
    </>
  )
}

/* ── cancel (R3, both grains — landed choreography unchanged) ────────────── */

/**
 * activeRun is the run the live feed is about: the task's latest run that has
 * not reached a terminal state, or — when everything has finished — the last
 * one, so the panel still shows what it was doing when it stopped.
 */
const terminalStates = ['completed', 'crashed', 'finalized', 'tombstoned', 'died-at-gate']

export function activeRun(detail: Detail): TaskRunView | null {
  const live = [...detail.runs].reverse().find((r) => !terminalStates.includes(r.state))
  return live ?? detail.runs[detail.runs.length - 1] ?? null
}

/**
 * cancellable is ONE list, read the other way round: a run that has not
 * reached a terminal state is one the 4.5 verb has an edge for. A state this
 * list has never seen is offered rather than hidden (§42) — the client owns
 * no state vocabulary, and the verb refuses what it cannot cancel.
 */
function cancellable(state: string): boolean {
  return !terminalStates.includes(state)
}

/**
 * The plain words for what a cancel DOES to a run in the state it is in
 * (S02.3 through internal/stage/cancel.go's ratified mapping; §39).
 */
function cancelConsequence(state: string): string {
  switch (state) {
    case 'running':
    case 'draining':
      return 'It stops now: the live session is closed down through its own shutdown ladder, and the run is recorded as ended, carrying the cancel on its record.'
    case 'parked':
      return 'It closes now, and any open question card on it closes with it. It will not resume afterwards.'
    case 'queued':
      return 'It is withdrawn before it starts, and its place in the queue is settled in the same breath.'
    case 'claimed':
    case 'new':
      return 'It is being dispatched right this moment, so the platform may refuse and ask you to try again in a moment. Nothing half-happens either way.'
    default:
      return 'The platform decides which ending applies to a run in this state, and tells you which one it took.'
  }
}

/** One run's disposition, rendered from the wire. */
function runOutcomeNote(res: CancelOutcome): string {
  if (!res.applied) return 'nothing was cancelled'
  return res.ladder_invoked
    ? `${res.from} → ${res.to} · the live session was closed through its shutdown ladder`
    : `${res.from} → ${res.to}`
}

/**
 * R3's task half: one control over every run of the task that has not ended.
 * It renders only while there IS such a run.
 */
function CancelTask({ taskID, runs, reload }: { taskID: string; runs: TaskRunView[]; reload: () => void }) {
  const [open, setOpen] = useState(false)
  const [result, setResult] = useState<TaskCancelOutcome | null>(null)
  const act = useAct()
  const live = runs.filter((r) => cancellable(r.state))
  if (live.length === 0) return null

  return (
    <div className="task-actions">
      <Button
        variant="danger"
        size="sm"
        data-cancel="task"
        onClick={() => {
          act.clear()
          setResult(null)
          setOpen(true)
        }}
      >
        Cancel this task
      </Button>
      <ActConfirm
        open={open}
        onOpenChange={setOpen}
        title="Cancel this task?"
        // The truth about this verb, and it is not atomic: `stage.CancelTask`
        // walks the task's runs in order and each cancel COMMITS ON ITS OWN.
        what={`Every run of this task that has not ended is cancelled — ${String(live.length)} right now — each under the rule for the state it is in. If one of them turns out to be mid-dispatch the request stops there: the runs already cancelled stay cancelled, and this page re-reads so you can see how far it got before trying the rest.`}
        act="Cancel every unfinished run"
        busy={act.busy}
        onConfirm={() => {
          setOpen(false)
          act.run(
            () =>
              api.cancelTask(taskID).then((res) => {
                setResult(res)
                return outcomeOf(
                  res.applied,
                  res.applied ? `cancelled, and the board reads ${res.kanban_status}` : 'nothing was cancelled',
                  '',
                )
              }),
            reload,
            'the request stopped where it was refused — anything already cancelled stays cancelled, and this page has re-read',
          )
        }}
      >
        <ul className="cancel-preview">
          {live.map((r) => (
            <li key={r.run_id} data-run={r.run_id}>
              <span className="run-id font-mono">{r.run_id}</span> <span className="run-state">{r.state}</span> —{' '}
              {cancelConsequence(r.state)}
            </li>
          ))}
        </ul>
      </ActConfirm>
      <OutcomeLine outcome={act.outcome} />
      {result && (
        <ul className="cancel-runs">
          {result.runs.map((r) => (
            <li key={r.run_id} data-run={r.run_id} data-outcome={r.applied ? 'applied' : 'noop'}>
              <span className="run-id font-mono">{r.run_id}</span> {runOutcomeNote(r)}
              <span className="muted"> — {r.detail}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** R3's run half: the same verb at the grain a person is looking at. */
function CancelRun({ run, reload }: { run: TaskRunView; reload: () => void }) {
  const [open, setOpen] = useState(false)
  const act = useAct()
  if (!cancellable(run.state)) return null

  return (
    <div className="run-actions">
      <Button
        variant="danger"
        size="sm"
        data-cancel-run={run.run_id}
        onClick={() => {
          act.clear()
          setOpen(true)
        }}
      >
        Cancel this run
      </Button>
      <ActConfirm
        open={open}
        onOpenChange={setOpen}
        title={`Cancel run ${run.run_id}?`}
        what={cancelConsequence(run.state)}
        act="Cancel this run"
        busy={act.busy}
        onConfirm={() => {
          setOpen(false)
          act.run(() => api.cancelRun(run.run_id).then((res) => outcomeOf(res.applied, runOutcomeNote(res), res.detail)), reload)
        }}
      />
      <OutcomeLine outcome={act.outcome} />
    </div>
  )
}

/* ── live activity ───────────────────────────────────────────────────────── */

/**
 * R11's other half: the LIVE ACTIVITY FEED (Spec S15.5; S2.2), reading the
 * run card: the last-activity line, the current stage and tool, and the
 * monotonic counters — no denominator exists and none is invented.
 */
export function LiveActivity({ run, stream }: { run: TaskRunView | null; stream?: EventStream }) {
  const { data, error, stale } = useLive<RunDetail>({
    key: run ? `/api/runs/${run.run_id}` : '',
    read: () => (run ? api.run(run.run_id) : Promise.reject(new Error('no run'))),
    types: activityEventTypes,
    applies: (e) => e.run_id === undefined || e.run_id === run?.run_id,
    stream,
  })

  if (!run) {
    return (
      <Section title="Live activity">
        <Absent reason="this task has no run yet, so there is nothing running to watch" />
      </Section>
    )
  }
  const card = data?.card
  return (
    <Section title="Live activity" stale={stale}>
      <Freshness stale={stale} error={error} hasData={data !== null} />
      {card && (
        <div className="activity" data-run={card.run_id}>
          <p className="flex flex-wrap items-center gap-2">
            <StatusDot tone={card.wedged ? 'red' : 'green'} live={!card.wedged} />
            <Chip className="run-state" tone={card.wedged ? 'red' : 'green'}>
              {card.state}
            </Chip>{' '}
            {card.stage !== '' ? (
              <span className="stage-name font-mono text-xs tracking-wide uppercase">{card.stage}</span>
            ) : (
              <Absent reason="no stage marker" />
            )}
            {card.wedged && <span className="warn-flag"> wedged</span>}
            {card.waiting_on_human && <span className="waiting-human"> waiting on a person</span>}
          </p>
          {card.wedged && (
            <StallBanner
              kind="wedged"
              tone="red"
              what="Flagged and paused. The platform stopped this run rather than let it keep going, and it now waits for a person to look."
            >
              <Link to={hrefFor('inbox')}>Its card is in the inbox</Link>
            </StallBanner>
          )}
          {card.waiting_on_human && (
            <StallBanner
              kind="blocked"
              tone="orange"
              what="Stopped on an open question. It moves again once somebody answers — nothing restarts on its own, and nothing is thrown away while it waits."
            >
              <Link to={hrefFor('inbox')}>Answer it in the inbox</Link>
            </StallBanner>
          )}
          {card.state === 'parked' && (
            <StallBanner
              kind="parked"
              tone="yellow"
              what={<ParkedUntil until={card.parked_until} />}
            >
              <Link to={hrefFor('inbox')}>It is released from its own card in the inbox</Link>
            </StallBanner>
          )}
          <p className="activity-line break-words">
            {card.last_activity ? (
              <>
                <span className="muted">{card.last_activity.type}</span> {card.last_activity.line}{' '}
                <Timestamp ts={card.last_activity.ts} variant="live" />
              </>
            ) : (
              <Absent reason="nothing has happened on this run yet" />
            )}
          </p>
          {card.tool && (
            <p className="muted">
              tool {card.tool.name} · args {card.tool.args_digest === '' ? 'not digested' : card.tool.args_digest}
            </p>
          )}
          <ul className="counters m-0 flex list-none flex-wrap gap-x-4 gap-y-1 p-0 text-muted-foreground">
            <li>{String(card.counters.steps)} steps</li>
            <li>{String(card.counters.tokens)} tokens</li>
            {/* Human-readable, never raw seconds (C2-13: "1500991 s elapsed"
                is the banned class). The counter stays monotonic fact. */}
            <li>{fmtDuration(card.counters.elapsed_s)} elapsed</li>
            <li className="run-cost">
              <Money usd={card.counters.api_equiv_cost_usd} />
              {card.counters.unpriced === true && (
                <> · subscription lane, so this is the API-equivalent figure</>
              )}
            </li>
          </ul>
        </div>
      )}
    </Section>
  )
}

/* ── the stage rail ──────────────────────────────────────────────────────── */

type RailNode = {
  key: string
  /** The served instant, verbatim — what the node is placed by. */
  at: string
  /** What that instant IS, where it is not simply "when this happened". */
  atLabel?: string
  kind: 'step' | 'error' | 'split' | 'decision' | 'park' | 'terminal'
  tone: Tone
  stage?: string
  head: ReactNode
  note?: ReactNode
  /** Stopped states carry the door to where they are answered. */
  door?: string
}

/** The tone one served outcome takes; anything unrecognized is neutral —
 *  this client owns no vocabulary here (§42). */
function outcomeTone(outcome: string): string {
  return outcome === 'error' ? 'red' : outcome === 'split' ? 'blue' : outcome === 'completed' ? 'ok' : 'muted'
}

function stepNode(s: Detail['stage_progress'][number]): RailNode {
  const kind = s.outcome === 'error' ? 'error' : s.outcome === 'split' ? 'split' : 'step'
  return {
    key: `stage/${s.run_id}/${String(s.seq)}`,
    at: s.ts,
    kind,
    tone: kind === 'error' ? 'red' : kind === 'split' ? 'blue' : 'green',
    stage: s.stage,
    head: (
      <>
        <span className="stage-name font-mono text-xs tracking-wide uppercase">
          {s.stage === '' ? <Absent reason="unnamed stage" /> : s.stage}
        </span>
        <span className="muted text-xs"> {s.type}</span>
        {s.kind !== '' && <span className="muted text-xs"> · {s.kind}</span>}
        {s.outcome && (
          <span
            className="stage-outcome text-xs"
            data-outcome-tone={outcomeTone(s.outcome)}
            style={{ color: `var(--${outcomeTone(s.outcome)})` }}
          >
            {' '}
            · {s.outcome}
          </span>
        )}
      </>
    ),
    note:
      kind === 'error' ? (
        'This stage session ended short of completing — parked on an ask, a ceiling reached, or the engine stopping.'
      ) : kind === 'split' ? (
        'The stage was split at a checkpoint boundary and carried on in a successor session. Not a completion, and not a failure.'
      ) : undefined,
  }
}

function decisionNode(d: TaskDecision): RailNode {
  return {
    key: `decision/${String(d.seq)}`,
    at: d.decided_at ?? d.ts,
    kind: 'decision',
    tone: 'accent',
    head: (
      <>
        <Owner id={d.actor} />
        {d.actor_is_operator && <span className="muted"> (as operator)</span>}{' '}
        <span className="decision">{d.decision}</span>{' '}
        <span className="muted text-xs">{d.card_type}</span>
      </>
    ),
  }
}

function parkNode(runID: string, p: NonNullable<Receipt['park_history']>[number], at: number): RailNode {
  return {
    key: `park/${runID}/${String(at)}`,
    at: p.parked_at,
    kind: 'park',
    tone: 'yellow',
    head: (
      <>
        <span className="font-mono text-xs tracking-wide uppercase">parked</span>
        {p.ongoing ? (
          <span className="warn-flag"> still parked</span>
        ) : (
          <span className="muted text-xs">
            {' '}
            · resumed{p.duration_seconds === undefined ? '' : ` after ${fmtDuration(p.duration_seconds)}`}
          </span>
        )}
        {p.park_reason && <span className="muted text-xs"> — {p.park_reason}</span>}
        {p.resume_cause && <span className="muted text-xs"> · resumed on {p.resume_cause}</span>}
      </>
    ),
    note: p.ongoing ? 'It waits. Nothing parked is thrown away, and it goes again from the card holding it.' : undefined,
    door: p.ongoing ? 'A parked run is released from its own card' : undefined,
  }
}

/**
 * Where the run stands, from the served state string and nothing else. THE
 * INSTANT IS LABELLED AS WHAT IT IS: this read serves no instant for a run's
 * standing, so `created_ts` renders under its own word, "opened".
 */
function terminalNode(r: TaskRunView): RailNode {
  return {
    key: `run/${r.run_id}`,
    at: r.created_ts,
    atLabel: 'opened',
    kind: 'terminal',
    tone: terminalStates.includes(r.state) ? 'accent' : 'green',
    head: (
      <>
        <span className="run-id font-mono text-xs">{r.run_id}</span>{' '}
        <span className="muted text-xs">stands at</span>{' '}
        <Chip tone={terminalStates.includes(r.state) ? 'accent' : 'green'}>{r.state}</Chip>
      </>
    ),
  }
}

export function railNodes(detail: Detail): RailNode[] {
  const nodes: RailNode[] = detail.stage_progress.map(stepNode)
  for (const d of detail.decisions) nodes.push(decisionNode(d))
  for (const r of detail.runs) {
    for (const [at, p] of (r.receipt?.park_history ?? []).entries()) nodes.push(parkNode(r.run_id, p, at))
  }

  const placed = nodes.map((node, servedAt) => ({ node, servedAt }))
  placed.sort((a, b) => {
    const ta = Date.parse(a.node.at)
    const tb = Date.parse(b.node.at)
    if (Number.isNaN(ta) || Number.isNaN(tb) || ta === tb) return a.servedAt - b.servedAt
    return ta - tb
  })
  // The run-standing nodes close the rail in served order: a run's state
  // carries no instant of its own on this read.
  return [...placed.map((p) => p.node), ...detail.runs.map(terminalNode)]
}

/** R11: the stage story, as the rail. Counters are monotonic facts; nothing
 *  here is turned into "how far along". */
function StageBlock({ detail, stale }: { detail: Detail; stale: boolean }) {
  const nodes = railNodes(detail)
  return (
    <Section title="Progress by stage" stale={stale}>
      {nodes.length === 0 ? (
        <EmptyState
          what="No stage boundary has been recorded yet."
          why="The rail fills in as the platform opens and closes stage sessions, records a person's decision, and parks or ends a run. An empty rail means none of that has happened yet."
        />
      ) : (
        <ol className="stages rail relative m-0 list-none border-s border-border ps-0">
          {nodes.map((n) => (
            <RailStep key={n.key} node={n} />
          ))}
        </ol>
      )}
      <ProjectLineage detail={detail} />
    </Section>
  )
}

/** One node on the rail: a toned dot, what happened, and — where the thing is
 *  STOPPED — the plain words and the door to where it is answered. */
function RailStep({ node }: { node: RailNode }) {
  return (
    <li
      className="relative py-1.5 ps-4"
      data-node={node.kind}
      {...(node.stage === undefined ? {} : { 'data-stage': node.stage })}
    >
      <StatusDot tone={node.tone} className="absolute start-0 top-3 -translate-x-1/2" />
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        {node.head}
        <span className="muted ms-auto text-xs">
          {node.atLabel !== undefined && `${node.atLabel} `}
          <Timestamp ts={node.at} variant="live" />
        </span>
      </div>
      {node.note !== undefined && (
        <StallBanner kind={node.kind} tone={node.tone} what={node.note}>
          {node.door !== undefined && <Link to={hrefFor('inbox')}>{node.door} in the inbox</Link>}
        </StallBanner>
      )}
    </li>
  )
}

/** R7's other half: where this task sits among the others. `project_choices > 1`
 *  is an ambiguity the platform HAS, so it is shown rather than resolved. */
function ProjectLineage({ detail }: { detail: Detail }) {
  const lin = detail.lineage
  return (
    <div className="lineage">
      <h4>Project and lineage</h4>
      <p>
        Project: {lin.project}
        {lin.project_choices > 1 && (
          <span className="warn-flag" data-ambiguous="project">
            {' '}
            — this task claims {String(lin.project_choices)} projects, so which one it belongs to is genuinely
            unresolved
          </span>
        )}
      </p>
      {lin.succeeds.length === 0 && lin.succeeded_by.length === 0 ? (
        <p className="muted">No follow-up lineage: this task neither followed from a deliverable nor spawned one.</p>
      ) : (
        <>
          {lin.succeeds.map((s) => (
            <p key={`from-${s.deliverable_id}-${s.revision_n}`} data-lineage="succeeds">
              Follows from{' '}
              <Link to={hrefFor('deliverable', { id: s.deliverable_id })}>
                {s.deliverable_id} r{s.revision_n}
              </Link>
            </p>
          ))}
          {lin.succeeded_by.map((s) => (
            <p key={`to-${s.task_id}`} data-lineage="succeeded-by">
              Followed up by <Link to={hrefFor('task', { id: s.task_id })}>{s.task_id}</Link>
            </p>
          ))}
        </>
      )}
    </div>
  )
}

/* ── decisions ───────────────────────────────────────────────────────────── */

/** R12: every human decision along the way, from served rows only. */
function DecisionsBlock({ decisions, stale }: { decisions: TaskDecision[]; stale: boolean }) {
  return (
    <Section title="Human decisions" stale={stale}>
      {decisions.length === 0 ? (
        <EmptyState
          what="Nobody has had to decide anything on this task yet."
          why="A row appears here every time a person approves, rejects, accepts or otherwise answers a card on this task. The rail above shows where each one fell in the story."
        />
      ) : (
        <ul className="decisions">
          {decisions.map((d) => (
            <li key={d.seq} data-card-type={d.card_type}>
              <Owner id={d.actor} />
              {d.actor_is_operator && <span className="muted"> (as operator)</span>}{' '}
              <span className="decision text-[var(--ok)]">{d.decision}</span>{' '}
              <span className="muted">{d.card_id}</span>{' '}
              <Stamp ts={d.decided_at ?? d.ts} />
              {d.reason && <span className="muted"> — {d.reason}</span>}
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}

/* ── deliverables ────────────────────────────────────────────────────────── */

/**
 * R13: the task's REAL deliverables, with their immutable numbered revisions
 * and the door into their review. The revisions come from each deliverable's
 * own detail read — the numbers are records, never counted 1..N here.
 */
function DeliverablesBlock({ taskID, stream }: { taskID: string; stream?: EventStream }) {
  const { data, error, stale } = useLive<DeliverableDetail[]>({
    key: `/api/deliverables?task=${taskID}`,
    read: () =>
      api
        .deliverables({ task: taskID })
        .then((list) => Promise.all(list.deliverables.map((d) => api.deliverable(d.deliverable_id)))),
    types: boardEventTypes,
    stream,
  })

  return (
    <Section title="Deliverables" stale={stale}>
      <Freshness stale={stale} error={error} hasData={data !== null} />
      {data && data.length === 0 ? (
        <EmptyState
          what="This task has produced no deliverables yet."
          why="A deliverable appears when a worker produces one, with every revision kept as its own immutable record — the numbers are records rather than a count."
        />
      ) : (
        (data ?? []).map((d) => (
          <div className="deliverable" key={d.deliverable.id} data-deliverable={d.deliverable.id}>
            <h4>
              <Link to={hrefFor('deliverable', { id: d.deliverable.id })}>{d.deliverable.id}</Link>{' '}
              <span className="muted">
                {d.deliverable.type} · {d.deliverable.state}
              </span>
            </h4>
            <ol className="revisions">
              {d.revisions.map((r) => (
                <li key={r.n} data-revision={String(r.n)}>
                  <Link to={hrefFor('deliverable', { id: d.deliverable.id })}>revision {String(r.n)}</Link>{' '}
                  <span className="muted">
                    {r.pin_kind}
                    {r.content_sha256 ? ` ${r.content_sha256.slice(0, 12)}` : ''} · <Stamp ts={r.created_ts} />
                  </span>
                </li>
              ))}
            </ol>
          </div>
        ))
      )}
    </Section>
  )
}

/* ── receipts ────────────────────────────────────────────────────────────── */

/** R14: the receipt per run — and, where the run has not ended, the 4.5
 *  cancel at the grain the person is reading. */
function ReceiptsBlock({ runs, stale, reload }: { runs: TaskRunView[]; stale: boolean; reload: () => void }) {
  return (
    <Section title="Receipts" stale={stale}>
      {runs.length === 0 ? (
        <EmptyState
          what="This task has no runs yet."
          why="A run is one attempt at the work, and each carries its own receipt of what it consumed. Nothing has been started for this task."
        />
      ) : (
        runs.map((r) => (
          <div className="run-receipt" key={r.run_id} data-run={r.run_id}>
            <h4>
              {r.run_id} <span className="muted">{r.state}</span>
            </h4>
            <CancelRun run={r} reload={reload} />
            {r.receipt ? <ReceiptView receipt={r.receipt} /> : <Absent reason={r.receipt_absent ?? 'no receipt'} />}
          </div>
        ))
      )}
    </Section>
  )
}

/**
 * ReceiptView renders one stored receipt. Ceremony is itemized separately
 * from execution; unpriced calls are shown as unpriced, never folded into
 * the priced total.
 */
export function ReceiptView({ receipt }: { receipt: Receipt }) {
  const direct = receipt.direct_use
  return (
    <div className="receipt my-2">
      <div className="table-scroll">
        <table className="items">
          <thead>
            <tr>
              <th>Purpose</th>
              <th>Model</th>
              <th>Lane</th>
              <th>Calls</th>
              <th>Priced</th>
              <th>Unpriced calls</th>
            </tr>
          </thead>
          <tbody>
            {receipt.items.map((it, i) => (
              <tr key={i} data-purpose={it.Purpose}>
                <td>{it.Purpose}</td>
                <td>{it.Model}</td>
                <td>{it.Lane}</td>
                <td>{String(it.Calls)}</td>
                <td>
                  <Money usd={it.PricedUSD} />
                </td>
                <td>{it.UnpricedCalls > 0 ? <span className="warn-flag">{String(it.UnpricedCalls)}</span> : '0'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p>
        Total priced <Money usd={receipt.total_priced_usd} /> over {String(receipt.total_calls)} calls
        {receipt.total_unpriced_calls > 0 && (
          <span className="warn-flag"> · {String(receipt.total_unpriced_calls)} call(s) UNPRICED</span>
        )}{' '}
        <span className="muted">
          · currency {receipt.currency} · worst approximation tier {String(receipt.worst_tier)}
        </span>
      </p>

      <h5>Parks</h5>
      {(receipt.park_history ?? []).length === 0 ? (
        <p className="muted">This run was never parked.</p>
      ) : (
        <ul className="parks">
          {receipt.park_history?.map((p, i) => (
            <li key={i}>
              <Stamp ts={p.parked_at} /> →{' '}
              {p.ongoing ? <span className="warn-flag">still parked</span> : <Stamp ts={p.resumed_at} />}
              {p.park_reason && <span className="muted"> — {p.park_reason}</span>}
              {p.resume_cause && <span className="muted"> · resumed on {p.resume_cause}</span>}
            </li>
          ))}
        </ul>
      )}

      {/* The S10.6 seam note, printed exactly as the platform wrote it. */}
      <p className="mode-note" data-mode-note="verbatim">
        {receipt.mode.note === '' ? <Absent reason="no mode line recorded" /> : receipt.mode.note}
      </p>

      {/* The done-directly figure UNDER THE LABEL THE API SERVED. No label
          string is written in this file; the registered text lives in the
          registration and reaches the screen through the data. */}
      <p className="direct-use" data-direct-use-label={direct.label}>
        <span className="direct-use-label text-foreground">{direct.label}</span>:{' '}
        {direct.unpriced ? (
          <Absent reason={direct.reason ?? 'unpriced — no dollar figure can be honest here'} />
        ) : (
          <Money usd={direct.heuristic_usd} />
        )}
        <span className="muted"> · {direct.formula_ref}</span>
        {direct.measured_stage_seam && <span className="muted"> · {direct.measured_stage_seam}</span>}
      </p>
    </div>
  )
}
