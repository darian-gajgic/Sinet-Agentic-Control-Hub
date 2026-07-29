import { api, type Receipt, type Spec, type TaskDecision, type TaskDetail as Detail, type TaskRunView } from './api'
import type { EventStream } from './events'
import { boardEventTypes, useLive } from './live'
import { Absent, Empty, Freshness, Money, Owner, Section, Stamp } from './parts'
import { Link } from './router'
import { hrefFor } from './routes'

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
  const { data, error, stale } = useLive<Detail>({
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
          <SpecBlock detail={data} stale={stale} />
          <StageBlock detail={data} stale={stale} />
          <DecisionsBlock decisions={data.decisions} stale={stale} />
          <DeliverablesBlock detail={data} stale={stale} />
          <ReceiptsBlock runs={data.runs} stale={stale} />
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

/** R11: the stage story, plus the live-activity seam. Counters are monotonic
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
      <p className="muted">
        This story is derived from the log and updates from the feed — the runs below carry the live activity.
      </p>
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

/** R13: the deliverables link into the review surface. Rendering diffs,
 *  comments and the accept is B6-8's — the link is the honest door. */
function DeliverablesBlock({ detail, stale }: { detail: Detail; stale: boolean }) {
  const revisions = detail.lineage.succeeded_by
  return (
    <Section title="Deliverables" stale={stale}>
      {revisions.length === 0 ? (
        <p className="muted">
          Deliverable revisions are reviewed on their own surface. Any this task produced are reachable from the review
          list.
        </p>
      ) : (
        <ul>
          {revisions.map((s) => (
            <li key={s.deliverable_id}>
              <Link to={hrefFor('deliverable', { id: s.deliverable_id })}>
                {s.deliverable_id} — revision {String(s.revision_n)}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}

/** R14: the receipt per run. */
function ReceiptsBlock({ runs, stale }: { runs: TaskRunView[]; stale: boolean }) {
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
