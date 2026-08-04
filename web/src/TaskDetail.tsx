import { useState } from 'react'

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
import { activityEventTypes, boardEventTypes, useLive } from './live'
import { Absent, Empty, Freshness, Money, Owner, Section, Stamp } from './parts'
import { Link } from './router'
import { hrefFor } from './routes'
import { Button } from './ui'

/**
 * Task detail (Spec S15.5 ¶3; 9.2; S2.2; S2.4; G2 D2.8).
 *
 * The whole story of one task: the specification with its numbered acceptance
 * criteria, the plan, the per-stage progress and live activity, every human
 * decision along the way, the deliverable revisions, and the receipt.
 *
 * TWO RULES SHAPE EVERY BLOCK BELOW.
 *
 * The §38 ruling-(a) display contract: a pre-approval task serves its DRAFT
 * pair with its status on it, so this view labels a draft a draft. S15.5 calls
 * this surface "the confirmed specification", which is the APPROVED view — and
 * presenting an unapproved draft under that heading would be the one way this
 * page could mislead about something a person is about to sign off.
 *
 * Render-verbatim-as-served for the receipt labels: the done-directly label is
 * REGISTERED text (BENCH-REG §13) that the platform already puts on every
 * receipt. This file declares no label string of its own — it prints
 * `direct_use.label`, whatever it says. A scan proves the literals appear in no
 * view file, so the UI can never drift from the registration by re-typing it.
 */
export function TaskDetail({ id, stream }: { id: string; stream?: EventStream }) {
  // Deliberately UNNARROWED: every frame of the declared types re-reads this
  // resource. Filtering to "frames whose run_id is one of this task's runs"
  // looks like the obvious saving and is wrong — a `run.created` for a run this
  // task does not have YET is exactly the frame that would be dropped, and the
  // new run would never appear until something else happened to trigger a read.
  // One page is open at a time; correctness is worth more than the saving.
  const { data, error, stale, reload } = useLive<Detail>({
    key: `/api/tasks/${id}`,
    read: () => api.task(id),
    types: boardEventTypes,
    stream,
  })

  return (
    <section className="surface">
      <h1>{data ? data.title : 'Task'}</h1>
      <Freshness stale={stale} error={error} hasData={data !== null} />
      {data && (
        <>
          <p className="muted">
            <Owner id={data.owner} /> · {data.kanban_status} · opened <Stamp ts={data.created_ts} />
          </p>
          <CancelTask taskID={id} runs={data.runs} reload={reload} />
          <SpecBlock detail={data} stale={stale} />
          <StageBlock detail={data} stale={stale} />
          <LiveActivity run={activeRun(data)} stream={stream} />
          <DecisionsBlock decisions={data.decisions} stale={stale} />
          <DeliverablesBlock taskID={id} stream={stream} />
          <ReceiptsBlock runs={data.runs} stale={stale} reload={reload} />
        </>
      )}
    </section>
  )
}

/** R10: the spec with its numbered ACs and the plan, each labelled with the
 *  approval status it actually has. */
function SpecBlock({ detail, stale }: { detail: Detail; stale: boolean }) {
  const { spec, plan } = detail
  return (
    <Section title="Specification and plan" stale={stale}>
      {!spec || !plan ? (
        <Absent reason={detail.artifacts_absent ?? 'no spec/plan pair is stored for this task'} />
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
 * forgotten on one of the two artifacts: an approved pair reads as confirmed, a
 * draft reads as a draft and says what that means.
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
 * cancellable is ONE list, read the other way round: a run that has not reached
 * a terminal state is one the 4.5 verb has an edge for.
 *
 * A state this list has never seen is offered rather than hidden — the same
 * forward-tolerance the kanban vocabulary takes (§42): the client owns no state
 * vocabulary of its own, and the verb refuses what it cannot cancel. Hiding a
 * control on an unrecognized state would make a new server state silently
 * uncancellable from every surface.
 */
function cancellable(state: string): boolean {
  return !terminalStates.includes(state)
}

/**
 * The plain words for what a cancel DOES to a run in the state it is in
 * (S02.3 through internal/stage/cancel.go's ratified mapping; §39).
 *
 * Every sentence is a promise about a landed edge and asserts nothing beyond
 * it: no deletion, no rollback, no "stopping" a run that is only queued. The
 * unknown-state arm says what is actually true — the platform decides — rather
 * than describing an edge nobody has ratified.
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
 *
 * It renders only while there IS such a run — a task whose work is over offers
 * no cancel, because there is nothing to cancel and a control that can only
 * report "nothing happened" is a control that should not be there.
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
        // walks the task's runs in order and each cancel COMMITS ON ITS OWN, so
        // a run that turns out to be mid-dispatch stops the walk with everything
        // already cancelled still cancelled (internal/stage/cancel.go:221–233).
        // Saying "nothing is cancelled" here would be a promise the platform
        // does not keep, on the one screen where that matters most.
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

/**
 * R11's other half: the LIVE ACTIVITY FEED (Spec S15.5; S2.2).
 *
 * Stage boundaries alone say a stage started and ended — which is exactly the
 * "started and ended" S2.2 says is not enough to watch work happen. This reads
 * the run card: the last-activity line, the current stage and tool, and the
 * monotonic counters, seeded from the REST snapshot and re-read on this run's
 * own frames.
 *
 * The counters are rendered as the monotonic values they are. There is no
 * denominator on the wire and none is invented here — no percentage, no
 * fraction, no estimate of what is left.
 */
export function LiveActivity({ run, stream }: { run: TaskRunView | null; stream?: EventStream }) {
  const { data, error, stale } = useLive<RunDetail>({
    key: run ? `/api/runs/${run.run_id}` : '',
    read: () => (run ? api.run(run.run_id) : Promise.reject(new Error('no run'))),
    types: activityEventTypes,
    // Only this run's frames matter here — a sibling run moving is not this
    // panel's business, and unlike the task resource above, a frame for a run
    // that does not exist yet cannot change what THIS run is doing.
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
          <p>
            <span className="run-state">{card.state}</span>{' '}
            {card.stage !== '' ? <span className="stage-name">{card.stage}</span> : <Absent reason="no stage marker" />}
            {card.wedged && <span className="warn-flag"> wedged</span>}
            {card.waiting_on_human && <span className="waiting-human"> waiting on a person</span>}
          </p>
          <p className="activity-line">
            {card.last_activity ? (
              <>
                <span className="muted">{card.last_activity.type}</span> {card.last_activity.line}{' '}
                <Stamp ts={card.last_activity.ts} />
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
          <ul className="counters">
            <li>{String(card.counters.steps)} steps</li>
            <li>{String(card.counters.tokens)} tokens</li>
            <li>{String(card.counters.elapsed_s)} s elapsed</li>
            <li className="run-cost">
              <Money usd={card.counters.api_equiv_cost_usd} />
              {/* The subscription lane prices UNPRICED, so the figure beside it
                  is 0 — and a bare "USD 0" says the run was free when what is
                  true is that nobody priced it. The served marking is what makes
                  the number readable, exactly as on the workforce map. */}
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

/** R11: the stage story. Counters are monotonic
 *  facts; nothing here is turned into "how far along". */
function StageBlock({ detail, stale }: { detail: Detail; stale: boolean }) {
  return (
    <Section title="Progress by stage" stale={stale}>
      {detail.stage_progress.length === 0 ? (
        <Empty what="No stage boundary has been recorded yet." />
      ) : (
        <ol className="stages">
          {detail.stage_progress.map((s) => (
            <li key={`${s.run_id}/${s.seq}`} data-stage={s.stage}>
              <span className="stage-name">{s.stage === '' ? <Absent reason="unnamed stage" /> : s.stage}</span>
              <span className="muted"> {s.type}</span>
              {s.kind !== '' && <span className="muted"> · {s.kind}</span>}
              {s.outcome && <span className="stage-outcome"> · {s.outcome}</span>}
              <span className="muted">
                {' '}
                <Stamp ts={s.ts} />
              </span>
            </li>
          ))}
        </ol>
      )}
      <ProjectLineage detail={detail} />
    </Section>
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

/** R12: every human decision along the way, from served rows only. */
function DecisionsBlock({ decisions, stale }: { decisions: TaskDecision[]; stale: boolean }) {
  return (
    <Section title="Human decisions" stale={stale}>
      {decisions.length === 0 ? (
        <Empty what="Nobody has had to decide anything on this task yet." />
      ) : (
        <ul className="decisions">
          {decisions.map((d) => (
            <li key={d.seq} data-card-type={d.card_type}>
              <Owner id={d.actor} />
              {d.actor_is_operator && <span className="muted"> (as operator)</span>}{' '}
              <span className="decision">{d.decision}</span> <span className="muted">{d.card_id}</span>{' '}
              <Stamp ts={d.decided_at ?? d.ts} />
              {d.reason && <span className="muted"> — {d.reason}</span>}
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}

/**
 * R13: the task's REAL deliverables, with their immutable numbered revisions.
 *
 * The first cut rendered `lineage.succeeded_by` here — the follow-up TASKS
 * spawned from a deliverable — which is a different fact wearing the same
 * label, and left the task's actual deliverables unlisted. Lineage renders as
 * lineage, in its own block, and nowhere else.
 *
 * The revisions come from each deliverable's own detail read rather than being
 * counted 1..N from `current_revision`: the numbers are records, and inferring
 * them would be the client asserting a lineage it was never told.
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
        <Empty what="This task has produced no deliverables yet." />
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

/** R14: the receipt per run — and, where the run has not ended, the 4.5 cancel
 *  at the grain the person is reading. */
function ReceiptsBlock({ runs, stale, reload }: { runs: TaskRunView[]; stale: boolean; reload: () => void }) {
  return (
    <Section title="Receipts" stale={stale}>
      {runs.length === 0 ? (
        <Empty what="This task has no runs yet." />
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
 * ReceiptView renders one stored receipt.
 *
 * Ceremony is itemized separately from execution because that split is the
 * point of the S10.10 account: it is how a person sees what the platform spent
 * on ITSELF. Unpriced calls are shown as unpriced rather than folded into the
 * priced total — a silent zero would be the one dishonest number on the page.
 */
export function ReceiptView({ receipt }: { receipt: Receipt }) {
  const direct = receipt.direct_use
  return (
    <div className="receipt">
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
        <span className="direct-use-label">{direct.label}</span>:{' '}
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
