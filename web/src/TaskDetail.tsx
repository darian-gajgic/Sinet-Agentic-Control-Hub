import { useEffect, useState, type ReactNode } from 'react'
import { ArrowUpRight, X } from 'lucide-react'

import {
  api,
  type AC,
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
import { ActConfirm, CancelWhy, OutcomeLine, catchReasonRefusal, outcomeOf, useAct, whyHolds } from './controls'
import type { EventStream } from './events'
import { intakeResumeHref } from './Inbox'
import { isAssumedDefaultBoilerplate } from './Intake'
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
              {data.kanban_status === 'cancelled' && <CancelledBanner decisions={data.decisions} runs={data.runs} />}
              <StateStory detail={data} />
              <ActionBar detail={data} reload={reload} stream={stream} />
              <SpecBlock detail={data} stale={stale} />
              <StageBlock detail={data} stale={stale} />
              <LiveActivity run={activeRun(data)} stream={stream} />
              <DecisionsBlock decisions={data.decisions} stale={stale} cancelled={data.kanban_status === 'cancelled'} />
              <DeliverablesBlock taskID={id} ended={data.kanban_status === 'done' || data.kanban_status === 'cancelled'} stream={stream} />
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

/**
 * ONE state story (W2-3). The walk saw a task wear EXECUTING, "stands at
 * parked" and a blocked-on-human mark at once and read three contradicting
 * states. All three are served facts about DIFFERENT questions — the column
 * is where the work stands, the run state is how it sits there, the open
 * card is what it waits for — so the fix is one sentence tying them
 * together, rendered exactly when the combination would otherwise read as a
 * contradiction: a paused run under a moving column.
 */
function StateStory({ detail }: { detail: Detail }) {
  if (detail.kanban_status === 'done' || detail.kanban_status === 'cancelled') return null
  const run = activeRun(detail)
  if (run === null || run.state !== 'parked') return null
  const park = (run.receipt?.park_history ?? []).find((p) => p.ongoing)
  return (
    <p className="state-story" data-state-story>
      One story, three words: <b>{statusWord(detail.kanban_status)}</b> is where the work stands · <b>parked</b> is
      how it sits there{park?.park_reason !== undefined && park.park_reason !== '' && <> ({park.park_reason})</>} ·
      and it moves again when what parked it clears — a card answered, or a wait passing. Nothing is stuck twice —
      these are one state, read three ways.
    </p>
  )
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

/**
 * A stage marker in the reader's words (review #17): live runs serve internal
 * step ids ("intake.state", "t-….execute"), and the rail wore them raw. The
 * FAMILY gets plain words; the full id stays beside them as the record it is.
 * An unmapped family renders verbatim — this client owns no stage vocabulary
 * (§42).
 */
const stageFamilyWords: Record<string, string> = {
  intake: 'understanding the ask',
  interview: 'asking its questions',
  plan: 'planning',
  planning: 'planning',
  drafting: 'drafting',
  execute: 'doing the work',
  executing: 'doing the work',
  verify: 'checking the work',
  verifying: 'checking the work',
  spine: 'cross-checking the spec',
  critique: 'fresh-eyes critique of the plan',
}

export function StageName({ stage }: { stage: string }) {
  if (stage === '') return <Absent reason="no stage marker" />
  const family = stage.split('.')[0]
  const words = stageFamilyWords[family]
  if (words === undefined) {
    return <span className="stage-name font-mono text-xs tracking-wide uppercase">{stage}</span>
  }
  return (
    <span className="stage-name" data-stage-id={stage}>
      {words}
      {stage !== family && <span className="stage-id font-mono text-[10.5px] text-(--text-faint)"> · {stage}</span>}
    </span>
  )
}

/** A recorded event type in the reader's words (P3-GF9; walk W4 — the
 *  timeline wore "intake.state" and friends as raw tokens on a requester
 *  surface). Known types get a plain label; anything else renders its token
 *  with the separators spaced — the §42 forward tolerance: jargon reduced,
 *  never hidden. */
const eventTypePlain: Record<string, string> = {
  'stage.boundary': 'stage boundary',
  'intake.state': 'planning moved',
  'intake.delta_decision': 'a plan change was decided',
  'decision.recorded': 'decision recorded',
  'run.state_changed': 'the run moved',
  'engine.call': 'engine call',
  'engine.gate_ask': 'a question opened',
  'artifact.produced': 'work produced',
  'verdict.recorded': 'check verdict',
  'review.comment': 'review note',
}

export function eventTypeWords(t: string): string {
  return eventTypePlain[t] ?? t.replace(/[._]/g, ' ')
}

/** The receipt purpose tokens, in the reader's words (W2-6). The two values
 *  are the S10 receipt vocabulary; anything new renders verbatim (§42). */
const purposeWords: Record<string, string> = {
  ceremony: 'planning & questions (ceremony)',
  execution: 'doing the work',
}

/** The S10.10 receipt `currency` member in the reader's words (exit walk F6:
 *  "currency api-equivalent" is engineer dialect on a money surface).
 *  `api-equivalent` is D5's flat-rate posture — figures are what the calls
 *  would have cost at list prices, not a bill. An unknown served value
 *  renders under its own name (§42). */
export function currencyWords(currency: string): string {
  if (currency === 'api-equivalent') return 'dollar figures are list-price equivalents, not a bill'
  if (currency === 'real') return 'dollar figures are real billed money'
  return `currency ${currency}`
}

/** The S10.1 honest-approximation rank (1 best … 5 unknown) in the reader's
 *  words: HOW the usage behind this receipt was counted. The served number
 *  stays on the paragraph's data attribute; an unknown rank prints itself
 *  (§42). */
export function approximationWords(tier: number): string {
  switch (tier) {
    case 1:
      return "usage counted from the provider's own per-call numbers"
    case 2:
      return "usage counted from the engine's own aggregate counters"
    case 3:
      return "usage derived from the plan's own units"
    case 4:
      return 'usage estimated by tokenizer before the run'
    case 5:
      return 'usage could not be counted — listed UNPRICED, never a silent zero'
    default:
      return `approximation tier ${String(tier)}`
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
  // The r2 finding-1 fix: an intake QUESTION card (interview / clarification /
  // escalation / the family question) is answered on the give-work journey —
  // option chips, free text, Send — so this door goes straight there instead
  // of bouncing through an inbox card that has no controls of its own. Every
  // other card kind keeps its inbox address, where its verbs live.
  const resume = openAsk === undefined ? null : intakeResumeHref(openAsk)

  return (
    <div className="win-acts">
      {openAsk !== undefined && (
        <Button
          variant="primary"
          size="sm"
          data-act="answer"
          onClick={() => {
            navigate(resume ?? hrefFor('inbox-item', { id: openAsk.id }))
          }}
        >
          <ArrowUpRight size={13} strokeWidth={2} aria-hidden="true" />
          {resume !== null ? 'Continue answering its questions' : 'Answer its open card'}
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
          {/* The plain line is what the requester reads (review M6): the old
              render doubled every criterion (plain, then formal restatement)
              and repeated the notation parenthetical on each — jargon ×N.
              The formal wording is still the record, one fold below, with
              the notation named ONCE for the set. */}
          <ol className="acs">
            {spec.acs.map((ac) => (
              <li key={ac.n} data-ac={`AC-${ac.n}`}>
                <span className="ac-key">AC-{ac.n}</span> {ac.plain}
              </li>
            ))}
          </ol>
          <FormalACs acs={spec.acs} />
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

/**
 * The formal restatements, one fold below the plain criteria (review M6).
 * The plain line is what the requester reads; the formal wording is still the
 * record — behind a summary that says what it is, with the notation named
 * ONCE for the whole set instead of a parenthetical per criterion. Criteria
 * whose kinds differ keep a per-row note (nothing served is dropped).
 */
function FormalACs({ acs }: { acs: AC[] }) {
  const formal = acs.filter((ac) => ac.structured !== undefined && ac.structured !== '')
  if (formal.length === 0) return null
  const kinds = [...new Set(formal.map((ac) => ac.structured_kind ?? ''))].filter((k) => k !== '')
  const oneKind = kinds.length === 1 ? kinds[0] : undefined
  return (
    <details className="acs-formal" data-acs-formal>
      <summary className="cursor-pointer">The formal wording — the same checks, as the checker reads them</summary>
      {oneKind !== undefined && (
        <p className="muted acs-formal-note">
          Each of these is {structuredKindWords(oneKind)} — a shape the platform can check mechanically. The plain
          list above says the same things.
        </p>
      )}
      {/* Its own class on purpose: `.acs li` is a pinned selector for the
          PLAIN list, and this fold must not leak into it. */}
      <ol className="formal-acs">
        {formal.map((ac) => (
          <li key={ac.n} value={ac.n} data-ac-formal={`AC-${ac.n}`}>
            <span className="ac-key">AC-{ac.n}</span> <span className="muted">{ac.structured}</span>
            {oneKind === undefined && ac.structured_kind !== undefined && ac.structured_kind !== '' && (
              <span className="muted"> ({structuredKindWords(ac.structured_kind)})</span>
            )}
          </li>
        ))}
      </ol>
    </details>
  )
}

/** The two structured-AC notations, in plain words instead of their internal
 *  tokens "(ears)" / "(gwt)" (review #17). Unknown values render verbatim. */
function structuredKindWords(kind: string): string {
  switch (kind) {
    case 'ears':
      // RA-4: "EARS" is standards-committee dialect on a lay surface. The
      // plain words say what the shape IS; the acronym stays findable in
      // parentheses for anyone who wants to look it up.
      return 'written as strict when-X-then-Y requirements (the EARS form)'
    case 'gwt':
      return 'written as Given / When / Then'
    default:
      return kind
  }
}

function SpecLists({ spec }: { spec: Spec }) {
  const lists: { title: string; items: string[] }[] = [
    { title: 'Outcome', items: spec.outcome ?? [] },
    { title: 'Constraints', items: spec.constraints ?? [] },
    { title: 'Will not do', items: spec.out_of_scope ?? [] },
    { title: 'Open clarifications', items: spec.clarifications ?? [] },
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
      <SpecAssumptions assumptions={spec.assumptions ?? []} />
    </>
  )
}

/**
 * The stored spec's assumption list, collapsed the way the plan card's is
 * (re-walk B; blocker #15's collapse applied to THIS render path too).
 *
 * The old-era wall: every skipped interview slot leaves a template row —
 * "<Name> — …so I assumed a sensible default." — that names nothing anyone
 * can contest and repeats per slot. Substantive assumptions render whole,
 * with their basis; the template rows collapse into ONE line naming the
 * points. Rows the recognition does not match render verbatim — nothing is
 * hidden on a guess.
 */
function SpecAssumptions({ assumptions }: { assumptions: { text: string; basis?: string }[] }) {
  if (assumptions.length === 0) return null
  const shown: typeof assumptions = []
  const skipped: string[] = []
  for (const a of assumptions) {
    const name = /^(.+?) — /.exec(a.text)?.[1] ?? ''
    if (name !== '' && isAssumedDefaultBoilerplate(name, a.text)) {
      skipped.push(name)
      continue
    }
    shown.push(a)
  }
  return (
    <div>
      <h4>Assumptions</h4>
      <ul>
        {shown.map((a) => (
          <li key={a.text}>{a.basis ? `${a.text} (${a.basis})` : a.text}</li>
        ))}
        {skipped.length > 0 && (
          <li data-assume-skipped={String(skipped.length)}>
            {skipped.join(' · ')} — skipped in the interview, so the work went ahead on sensible defaults it did not
            spell out here.
          </li>
        )}
      </ul>
    </div>
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
  // The person's one-line why (P3-RW-19). The draft SURVIVES a refusal —
  // reopening the dialog keeps what was typed, because losing it would make
  // fixing one word mean retyping the sentence. It clears only when the
  // cancel it was written for actually fires.
  const [why, setWhy] = useState('')
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
        disabled={whyHolds(why)}
        onConfirm={() => {
          setOpen(false)
          act.run(
            () =>
              api
                .cancelTask(taskID, why.trim() === '' ? undefined : why)
                .then((res) => {
                  setResult(res)
                  if (res.applied) setWhy('')
                  return outcomeOf(
                    res.applied,
                    res.applied ? `cancelled, and the board reads ${res.kanban_status}` : 'nothing was cancelled',
                    '',
                  )
                }, catchReasonRefusal),
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
        <CancelWhy value={why} onChange={setWhy} keptWhere="kept on the record of every run this stops, so anyone who finds this task later knows" />
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
  // The one-line why, same ceremony as the task grain: draft survives a
  // refusal, clears only when the cancel fires.
  const [why, setWhy] = useState('')
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
        disabled={whyHolds(why)}
        onConfirm={() => {
          setOpen(false)
          act.run(
            () =>
              api
                .cancelRun(run.run_id, why.trim() === '' ? undefined : why)
                .then((res) => {
                  if (res.applied) setWhy('')
                  return outcomeOf(res.applied, runOutcomeNote(res), res.detail)
                }, catchReasonRefusal),
            reload,
          )
        }}
      >
        <CancelWhy value={why} onChange={setWhy} keptWhere="kept on this run's record, so anyone who finds it later knows" />
      </ActConfirm>
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
            <StageName stage={card.stage} />
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
                <span className="muted">{eventTypeWords(card.last_activity.type)}</span> {card.last_activity.line}{' '}
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
            {/* Counted words agree with their count — "1 steps" (exit walk
                nit) is the kind of seam a requester notices. */}
            <li>
              {String(card.counters.steps)} step{card.counters.steps === 1 ? '' : 's'}
            </li>
            <li>
              {String(card.counters.tokens)} token{card.counters.tokens === 1 ? '' : 's'}
            </li>
            {/* Human-readable, never raw seconds (C2-13: "1500991 s elapsed"
                is the banned class). The counter stays monotonic fact — AND IT
                STOPS AT THE TERMINAL (return-visit item 7): the served
                `elapsed_s` is now−created at read time, so on a COMPLETED run
                it kept counting hours after the work ended ("3 h 52 min
                elapsed" on a 3-minute check). A finished run's honest figure
                is the span between two SERVED instants — its first record and
                its last — so that is what renders once the state is terminal;
                the ticking derivation stays for a run still going. */}
            {(() => {
              const ended = terminalStates.includes(card.state)
              const ranFor =
                ended && card.last_activity !== null && run !== null
                  ? (Date.parse(card.last_activity.ts) - Date.parse(run.created_ts)) / 1000
                  : Number.NaN
              return ended && Number.isFinite(ranFor) && ranFor >= 0 ? (
                <li data-elapsed="final">ran for {fmtDuration(ranFor)} — from its first record to its last</li>
              ) : (
                <li data-elapsed="live">{fmtDuration(card.counters.elapsed_s)} elapsed</li>
              )
            })()}
            <li className="run-cost">
              {/* The unpriced truth (review #9): a subscription-lane run's
                  served figure is 0 because NO dollar price exists — calling
                  that 0 "the API-equivalent figure" contradicted the UNPRICED
                  receipts below it. A zero says unpriced; only a real nonzero
                  estimate may call itself API-equivalent. */}
              {card.counters.unpriced === true ? (
                card.counters.api_equiv_cost_usd !== null && card.counters.api_equiv_cost_usd > 0 ? (
                  <>
                    <Money usd={card.counters.api_equiv_cost_usd} /> · API-equivalent estimate — this work runs on a
                    subscription lane with no per-call price
                  </>
                ) : (
                  <>subscription lane — no dollar price exists for these calls; the receipt lists them as UNPRICED</>
                )
              ) : (
                <Money usd={card.counters.api_equiv_cost_usd} />
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
  /** The step's fact signature (stage|type|kind|outcome). Consecutive rows
   *  with the SAME signature fold into one counted row (review L5: the rail
   *  opened with six visually identical lines and no way to tell them
   *  apart) — only plain steps carry one; marked nodes never fold. */
  sig?: string
  /** How many identical consecutive rows this node stands for (≥2 when
   *  folded); `firstAt` keeps the earliest served instant of the run. */
  count?: number
  firstAt?: string
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
    // Only an unmarked step may fold (error/split rows each tell their own
    // story and stay individual).
    sig: kind === 'step' ? `${s.stage}|${s.type}|${s.kind}|${s.outcome ?? ''}` : undefined,
    head: (
      <>
        {s.stage === '' ? <Absent reason="unnamed stage" /> : <StageName stage={s.stage} />}
        <span className="muted text-xs"> {eventTypeWords(s.type)}</span>
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
  // Consecutive rows with the SAME fact signature fold into one counted row
  // (review L5): nothing served is dropped — the count, the first and the
  // latest instant of the run all render — but six indistinguishable lines
  // stop posing as six separate stories.
  const folded: RailNode[] = []
  for (const { node } of placed) {
    const prev = folded[folded.length - 1]
    if (prev !== undefined && prev.sig !== undefined && prev.sig === node.sig) {
      prev.count = (prev.count ?? 1) + 1
      prev.firstAt = prev.firstAt ?? prev.at
      prev.at = node.at
      continue
    }
    folded.push({ ...node })
  }

  // The run-standing nodes close the rail in served order: a run's state
  // carries no instant of its own on this read.
  return [...folded, ...detail.runs.map(terminalNode)]
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
        {node.count !== undefined && node.count > 1 && (
          // A folded run of identical boundaries says its count out loud
          // (review L5) — the instants stay: first on the left, latest right.
          <span className="muted text-xs" data-folded={String(node.count)}>
            · {String(node.count)} of these in a row
          </span>
        )}
        <span className="muted ms-auto text-xs">
          {node.count !== undefined && node.count > 1 && node.firstAt !== undefined && (
            <>
              first <Timestamp ts={node.firstAt} variant="live" /> · latest{' '}
            </>
          )}
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

/** A reconstructed cancel row — the ONE machine code the serve side puts on
 *  them (`decision:"cancel"`, reads.go cancelDecisions; coordinator ruling
 *  OQ1: the plain words are this client's to render from that code). */
function isCancelRow(d: TaskDecision): boolean {
  return d.decision === 'cancel'
}

/**
 * The cancelled task's plain-words why (review #10; P3-RW-19). A cancel is
 * ALWAYS a person's act on this platform (internal/stage/cancel.go: every
 * entry point takes an authenticated actor). The serve side now reconstructs
 * the cancel into `decisions` (P3-RW-18) and — where the person said why —
 * carries their own words in `human_reason` (P3-RW-19). ABSENCE IS
 * STRUCTURAL on that key: no key means no reason was given, and the banner
 * says exactly that in the same words History's cancelled view uses, instead
 * of the rule citation the walk found standing where a motive should be.
 */
function CancelledBanner({ decisions, runs }: { decisions: TaskDecision[]; runs: TaskRunView[] }) {
  const cancel = [...decisions].reverse().find(isCancelRow)
  // Second served source: the receipt's park history records the resume cause,
  // and a cancel ends the last park as "cancelled by <actor> …" — reading the
  // name out of the record the card already receives.
  const fromPark = runs
    .flatMap((r) => r.receipt?.park_history ?? [])
    .map((park) => /^cancelled by ([^\s(]+)/.exec(park.resume_cause ?? '')?.[1])
    .find((who) => who !== undefined)
  const who = cancel !== undefined && cancel.actor !== '' ? cancel.actor : fromPark
  return (
    <StallBanner kind="cancelled" tone="red" what={
      who !== undefined ? (
        <>
          This task was cancelled by <Owner id={who} />
          {cancel?.human_reason !== undefined && cancel.human_reason !== '' ? (
            <span data-cancel-why="given"> — &ldquo;{cancel.human_reason}&rdquo;</span>
          ) : cancel !== undefined ? (
            <span data-cancel-why="absent"> — no reason was given</span>
          ) : null}
          . Nothing runs on it and nothing more is spent; the record below stays.
        </>
      ) : (
        <>
          This task was cancelled — a person stopped it; a cancel is never the machine&apos;s own act. Nothing runs on
          it and nothing more is spent; the record below stays. (Its records don&apos;t say who: they were written
          before the platform kept the cancel&apos;s who and why.)
        </>
      )
    } />
  )
}

/** R12: every human decision along the way, from served rows only. */
function DecisionsBlock({ decisions, stale, cancelled }: { decisions: TaskDecision[]; stale: boolean; cancelled?: boolean }) {
  return (
    <Section title="Human decisions" stale={stale}>
      {decisions.length === 0 ? (
        cancelled === true ? (
          // "Nobody has decided anything" on a task a person CANCELLED is a
          // false sentence (review #10). The cancel is normally reconstructed
          // into this list from the run's own records (P3-RW-18); a cancelled
          // task with NO row means those records predate that reconstruction
          // or carry nothing it can read — said plainly, claimed no further.
          <EmptyState
            what="One decision was made here: a person cancelled this task."
            why="A cancel is never the machine's own act. Its row is normally rebuilt from the run's records and listed here — this task's records carry none the platform can read that way, so who pressed it is not listed. No other decision was recorded."
          />
        ) : (
        <EmptyState
          what="Nobody has had to decide anything on this task yet."
          why="A row appears here when a person's decision is recorded in its own right — a delta re-approval, an operator override, accepting a deliverable. Answering a plan or interview card resumes the pipeline instead of minting a row: those answers show in the rail above, as parked stretches that end with 'resumed on plan approved'."
        />
        )
      ) : (
        <ul className="decisions">
          {decisions.map((d) =>
            isCancelRow(d) ? (
              // The cancel row, in plain words from its machine code (ruling
              // OQ1: the serve side ships no prose for this page). The why is
              // the person's own words — or its honest absence, in History's
              // exact phrase. The record's mechanical sentence stays served in
              // `reason`; it does not render here, because a rule citation
              // standing where a motive should be is the walk finding this
              // packet exists to end.
              <li key={d.seq} data-card-type={d.card_type} data-decision="cancel">
                <Owner id={d.actor} />
                {d.actor_is_operator && <span className="muted"> (as operator)</span>}{' '}
                <span className="decision text-[var(--red)]">cancelled this work</span>{' '}
                {d.run_id !== undefined && d.run_id !== '' && <span className="muted font-mono">{d.run_id}</span>}{' '}
                <Stamp ts={d.decided_at ?? d.ts} />
                {d.human_reason !== undefined && d.human_reason !== '' ? (
                  <span data-cancel-why="given"> — &ldquo;{d.human_reason}&rdquo;</span>
                ) : (
                  <span className="muted" data-cancel-why="absent"> — no reason was given</span>
                )}
              </li>
            ) : (
            <li key={d.seq} data-card-type={d.card_type}>
              <Owner id={d.actor} />
              {d.actor_is_operator && <span className="muted"> (as operator)</span>}{' '}
              <span className="decision text-[var(--ok)]">{d.decision}</span>{' '}
              <span className="muted">{d.card_id}</span>{' '}
              <Stamp ts={d.decided_at ?? d.ts} />
              {d.reason && <span className="muted"> — {d.reason}</span>}
            </li>
            ),
          )}
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
function DeliverablesBlock({ taskID, ended, stream }: { taskID: string; ended?: boolean; stream?: EventStream }) {
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
        ended === true ? (
          // W2-2: on an ENDED task, "no deliverables YET" promised something
          // still coming. The ended state picks the honest tense. The
          // demo-seed clause that stood here guessed a CAUSE this page cannot
          // know (the M10 class: the same guess misattributed on a real
          // crashed run) — the honest words claim only what is served, and
          // point at the records that do say how it ended.
          <EmptyState
            what="This task is finished, and no deliverable is recorded for it."
            why="A run that produces work attaches it, and it would be listed here with a door into review. Nothing is attached on this one — the stage rail and receipts above say how its runs ended."
          />
        ) : (
        <EmptyState
          what="This task has produced no deliverables yet."
          why="A deliverable appears when a worker produces one, with every revision kept as its own immutable record — the numbers are records rather than a count."
        />
        )
      ) : (
        (data ?? []).map((d) => (
          <div className="deliverable" key={d.deliverable.id} data-deliverable={d.deliverable.id}>
            <h4>
              <Link to={hrefFor('deliverable', { id: d.deliverable.id })}>{d.deliverable.id}</Link>{' '}
              <span className="muted">
                {d.deliverable.type} · {d.deliverable.state}
              </span>
            </h4>
            {/* Re-walk B: DONE up top and "in-review" down here read as two
                stories until somebody says they are one. They are one: the
                TASK'S work ends when the result is minted; the RESULT then
                waits on its own owner. Said where the two words meet. */}
            {d.deliverable.state === 'in-review' && (
              <p className="muted my-1 text-sm" data-one-story="in-review">
                The task&apos;s work on this is finished — what remains is not the platform&apos;s to do:{' '}
                <strong>{d.deliverable.owner}</strong> reviews it, and accepts it or asks for changes on{' '}
                <Link to={hrefFor('deliverable', { id: d.deliverable.id })}>its review page</Link>.
              </p>
            )}
            <ol className="revisions">
              {d.revisions.map((r) => (
                <li key={r.n} data-revision={String(r.n)}>
                  <Link to={hrefFor('deliverable', { id: d.deliverable.id })}>revision {String(r.n)}</Link>{' '}
                  <span className="muted">
                    {r.pin_kind}
                    {r.content_sha256 ? ` ${r.content_sha256.slice(0, 12)}` : ''} · <Timestamp ts={r.created_ts} variant="live" />
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
        (() => {
          // The S10.6 mode note is often the SAME sentence on every run of a
          // task; printing it whole per receipt read as a stuck footnote
          // (review #17). The first receipt carries it in full; repeats point
          // up instead. The note stays the platform's verbatim text where it
          // renders, and a run whose note DIFFERS still prints its own.
          const seen = new Set<string>()
          const seenDirect = new Set<string>()
          return runs.map((r) => {
            const note = r.receipt?.mode.note ?? ''
            const repeated = note !== '' && seen.has(note)
            if (note !== '') seen.add(note)
            const d = r.receipt?.direct_use
            const directSig =
              d === undefined ? '' : [d.label, d.reason ?? '', d.formula_ref, d.measured_stage_seam ?? '', String(d.unpriced)].join('|')
            const directRepeated = directSig !== '' && seenDirect.has(directSig)
            if (directSig !== '') seenDirect.add(directSig)
            return (
              <div className="run-receipt" key={r.run_id} data-run={r.run_id}>
                <h4>
                  {r.run_id} <span className="muted">{r.state}</span>
                </h4>
                <CancelRun run={r} reload={reload} />
                {r.receipt ? (
                  <ReceiptView receipt={r.receipt} modeNoteRepeated={repeated} directUseRepeated={directRepeated} />
                ) : (
                  // The SERVED absence reason (P3-GF14 R6): the wire now
                  // speaks to the run's own ending, state by state — a
                  // terminal run is never promised a receipt that will not
                  // come, and a crashed leg names the successor carrying the
                  // work. The client-side terminal guess that stood here
                  // ("a demo-seeded task can be minted finished without
                  // one") misattributed on a REAL crashed run (review M10)
                  // and is deleted; the fallback covers only a snapshot
                  // written before the member existed.
                  <Absent reason={r.receipt_absent ?? 'no receipt is recorded for this run'} />
                )}
              </div>
            )
          })
        })()
      )}
    </Section>
  )
}

/**
 * ReceiptView renders one stored receipt. Ceremony is itemized separately
 * from execution; unpriced calls are shown as unpriced, never folded into
 * the priced total.
 */
export function ReceiptView({
  receipt,
  modeNoteRepeated,
  directUseRepeated,
}: {
  receipt: Receipt
  modeNoteRepeated?: boolean
  directUseRepeated?: boolean
}) {
  const direct = receipt.direct_use
  // `items` is null on the wire when the receipt itemizes nothing — a run
  // cancelled before any model call materializes exactly that (F1's crasher).
  // Zero line items is a fact worth a sentence, not an empty table.
  const items = receipt.items ?? []
  return (
    <div className="receipt my-2">
      {items.length === 0 ? (
        <p className="muted">
          Nothing itemized — this receipt records no model calls. A run cancelled or ended before any call was made
          leaves exactly this.
        </p>
      ) : (
      <div className="table-scroll">
        {/* COLUMN ORDER IS THE NARROW-SCREEN DESIGN (re-walk B): on a phone
            the sideways scroll hides everything past the first ~two columns,
            and the reason a person opens a receipt is the MONEY — so the
            priced figure rides second, right beside the purpose, and never
            arrives cut. Model and lane are the detail a reader scrolls FOR,
            not the answer they came for. */}
        <table className="items">
          <thead>
            <tr>
              <th>Purpose</th>
              <th>Priced</th>
              <th>Calls</th>
              <th>Model</th>
              <th>Lane</th>
              <th>Unpriced calls</th>
            </tr>
          </thead>
          <tbody>
            {items.map((it, i) => (
              <tr key={i} data-purpose={it.Purpose}>
                {/* The purpose token in the reader's words (W2-6: "Purpose:
                    ceremony" was radio chatter); an unmapped served value
                    renders verbatim (§42). */}
                <td>{purposeWords[it.Purpose] ?? it.Purpose}</td>
                <td>
                  <Money usd={it.PricedUSD} />
                </td>
                <td>{String(it.Calls)}</td>
                <td>{it.Model}</td>
                <td>{it.Lane}</td>
                <td>{it.UnpricedCalls > 0 ? <span className="warn-flag">{String(it.UnpricedCalls)}</span> : '0'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      )}
      <p data-receipt-currency={receipt.currency} data-worst-tier={String(receipt.worst_tier)}>
        Total priced <Money usd={receipt.total_priced_usd} /> over {String(receipt.total_calls)} call
        {receipt.total_calls === 1 ? '' : 's'}
        {receipt.total_unpriced_calls > 0 && (
          <span className="warn-flag">
            {' '}
            · {String(receipt.total_unpriced_calls)} call{receipt.total_unpriced_calls === 1 ? '' : 's'} UNPRICED
          </span>
        )}{' '}
        <span className="muted">
          {/* The machine members in the reader's words (exit walk F6:
              "currency api-equivalent · worst approximation tier 1" is
              engineer dialect on a money surface). The exact served values
              stay on this paragraph's data attributes — humanized, never
              hidden (§42: an unknown value renders as itself). */}
          · {currencyWords(receipt.currency)} · {approximationWords(receipt.worst_tier)}
        </span>
      </p>

      <h5>Parks</h5>
      {(receipt.park_history ?? []).length === 0 ? (
        <p className="muted">This run was never parked.</p>
      ) : (
        <ul className="parks">
          {receipt.park_history?.map((p, i) => (
            <li key={i}>
              {/* Live-face instants (review M7): the raw nanosecond pair read
                  as noise on a requester surface — the verbatim UTC stays on
                  each element per the D5 primitive, one hover away. */}
              parked <Timestamp ts={p.parked_at} variant="live" />
              {p.ongoing ? (
                <span className="warn-flag"> · still parked</span>
              ) : p.duration_seconds !== undefined ? (
                <span> · resumed after {fmtDuration(p.duration_seconds)}</span>
              ) : (
                <span>
                  {' '}
                  · resumed <Timestamp ts={p.resumed_at} variant="live" />
                </span>
              )}
              {p.park_reason && <span className="muted"> — {p.park_reason}</span>}
              {p.resume_cause && <span className="muted"> · resumed on {p.resume_cause}</span>}
            </li>
          ))}
        </ul>
      )}

      {/* The S10.6 seam note, printed exactly as the platform wrote it — once:
          a note identical to an earlier receipt's points up instead (review
          #17's stuck-footnote read). */}
      <p className="mode-note" data-mode-note={modeNoteRepeated === true ? 'repeated' : 'verbatim'}>
        {receipt.mode.note === '' ? (
          <Absent reason="no mode line recorded" />
        ) : modeNoteRepeated === true ? (
          'same accounting note as the receipt above'
        ) : (
          receipt.mode.note
        )}
      </p>

      {/* The done-directly figure UNDER THE LABEL THE API SERVED. No label
          string is written in this file; the registered text lives in the
          registration and reaches the screen through the data. */}
      {directUseRepeated === true ? (
        // The same registered pricing note, word for word, on every run of a
        // task read as a stuck footnote (review #17) — the first receipt
        // carries it whole; repeats point up.
        <p
          className="direct-use"
          data-direct-use-label={direct.label}
          data-direct-use="repeated"
          data-formula-ref={direct.formula_ref}
        >
          <span className="direct-use-label text-foreground" title={`the registered formula: ${direct.formula_ref}`}>
            {direct.label}
          </span>
          :{' '}
          <span className="muted">same pricing note as the receipt above</span>
          {!direct.unpriced && (
            <>
              {' '}
              <Money usd={direct.heuristic_usd} />
            </>
          )}
        </p>
      ) : (
      <p className="direct-use" data-direct-use-label={direct.label} data-formula-ref={direct.formula_ref}>
        {/* The registered LABEL stays verbatim (its GF13 keep row). The
            formula REF is machine provenance — a repository path with a
            section sign, meaningless and slightly alarming to a household
            reader on a money surface (exit walk F6) — so it moves off the
            prose line onto this paragraph's data attribute and the label's
            hover, where the record stays findable without standing in a
            sentence. */}
        <span className="direct-use-label text-foreground" title={`the registered formula: ${direct.formula_ref}`}>
          {direct.label}
        </span>
        :{' '}
        {direct.unpriced ? (
          <Absent reason={direct.reason ?? 'unpriced — no dollar figure can be honest here'} />
        ) : (
          <Money usd={direct.heuristic_usd} />
        )}
        {direct.measured_stage_seam && <span className="muted"> · {direct.measured_stage_seam}</span>}
      </p>
      )}
    </div>
  )
}
