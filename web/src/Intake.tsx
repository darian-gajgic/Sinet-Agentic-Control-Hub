import { useMemo, useState } from 'react'
import { Check, CircleAlert, FolderOpen, Sparkles, X } from 'lucide-react'

import {
  ApiError,
  api,
  type IntakeAnswerBody,
  type IntakeCard,
  type IntakeHelp,
  type IntakeQuestion,
  type IntakeTaskView,
} from './api'
import type { EventStream } from './events'
import { describeError, inboxEventTypes, useLive } from './live'
import { Owner } from './parts'
import { Link, navigate } from './router'
import { hrefFor } from './routes'
import { Button, Chip } from './ui'

/**
 * The give-work door (map §3 v3; Spec S06.5–S06.9 through the landed intake
 * routes) — the journey that kills finding 5.
 *
 * One plain ask box → the interview as a FORM (cards of up to 4 questions,
 * labeled options + free text, a live clearance meter) → the plan card (what I
 * understood · what you'll get · numbered steps with done-when · what I will
 * NOT do · assumptions front-and-center · cost/time) → Approve / Re-plan /
 * Re-interview / Cancel.
 *
 * THE SUBMIT CONTRACT THAT KILLS FINDING 5: every primary act on this surface
 * is either visibly enabled, or visibly disabled WITH ITS REASON PRINTED
 * BESIDE IT, and every completed act shows a visible result. A control that
 * looks dead while being the only way forward is the defect class this file
 * exists to end.
 *
 * STATE SHAPE. The pipeline's two writes each return the whole task view, so
 * the journey is a fold: submit → view → answer → view → … → approved (or
 * cancelled). Nothing here re-derives pipeline state; the view is the truth
 * and the card's own vocabulary (options, choices, actions) is the only verb
 * source. Mid-journey state lives in this tab; the same open card is always
 * also in the Inbox, which the header says.
 */
export function DescribeGoal({ search = '', stream }: { search?: string; stream?: EventStream }) {
  const [view, setView] = useState<IntakeTaskView | null>(null)
  const params = new URLSearchParams(search)
  const pinned = params.get('project') ?? ''
  const resumeTask = params.get('task') ?? ''

  // A born task stamps its id onto the door's own address, so a reload lands
  // back IN the journey (the open card is durable server-side) instead of on
  // a fresh ask box with the interview seemingly lost — the tab holds no
  // storage (§41-B), the URL is the one pocket it has.
  const born = (v: IntakeTaskView) => {
    window.history.replaceState(null, '', `${hrefFor('new')}?task=${encodeURIComponent(v.task_id)}`)
    setView(v)
  }

  return (
    <section className="surface door" data-door="describe-goal">
      {view !== null ? (
        <Journey view={view} onView={setView} stream={stream} />
      ) : resumeTask !== '' ? (
        <ResumeJourney taskID={resumeTask} onView={setView} stream={stream} />
      ) : (
        <AskForm pinned={pinned} onBorn={born} stream={stream} />
      )}
    </section>
  )
}

/**
 * Resume after a reload: the task's face comes from the owner-scoped task
 * read, and the open card — if one is open — from the same follow path the
 * live journey uses. No pipeline state is re-derived: what cannot be known
 * from a served read renders as the follow state's honest waiting face.
 */
function ResumeJourney({ taskID, onView, stream }: { taskID: string; onView: (v: IntakeTaskView) => void; stream?: EventStream }) {
  const detail = useLive({
    key: `/api/tasks/${taskID}#door`,
    read: () => api.task(taskID),
    types: inboxEventTypes,
    stream,
  })
  if (detail.error !== '' && detail.data === null) {
    return (
      <div className="door-refusal" role="alert">
        <CircleAlert size={16} strokeWidth={2} aria-hidden="true" />
        <p className="refusal-detail">
          This journey could not be resumed — the task {taskID} did not answer. Its card, if one is open, is in your{' '}
          <Link to={hrefFor('inbox')}>Inbox</Link>.
        </p>
      </div>
    )
  }
  if (detail.data === null) return <p className="muted">Resuming the journey…</p>
  const d = detail.data
  const view: IntakeTaskView = {
    task_id: d.task_id,
    title: d.title,
    kanban_status: d.kanban_status,
    owner: d.owner,
    ...(d.kanban_status === 'cancelled' ? { phase: 'cancelled' } : {}),
  }
  return <Journey view={view} onView={onView} stream={stream} />
}

/* ── the ask box ─────────────────────────────────────────────────────────── */

/**
 * The plain ask. The project pin is the LANDED P3-RW-1 field: the registry id
 * verbatim, validated server-side — an invalid pin refuses the submission
 * loudly (404/409 render below the button, nothing is quietly dropped).
 *
 * The pin choices derive from the caller's own served task rows until the
 * projects read (P3-RW-2, in flight) lands; "(no project)" is not a pin — it
 * is the unpinned submission.
 */
function AskForm({
  pinned,
  onBorn,
  stream,
}: {
  pinned: string
  onBorn: (v: IntakeTaskView) => void
  stream?: EventStream
}) {
  const [text, setText] = useState('')
  const [title, setTitle] = useState('')
  const [project, setProject] = useState(pinned)
  const [busy, setBusy] = useState(false)
  const [refusal, setRefusal] = useState<{ head: string; detail: string } | null>(null)

  const tasks = useLive({
    key: '/api/tasks#door',
    read: () => api.tasks(),
    types: ['intake.state'],
    stream,
  })
  const projects = useMemo(() => {
    const names = new Set((tasks.data?.tasks ?? []).map((t) => t.project))
    names.delete('(no project)')
    if (project !== '') names.add(project)
    return [...names].sort((a, b) => a.localeCompare(b))
  }, [tasks.data, project])

  const empty = text.trim() === ''

  const submit = () => {
    if (empty || busy) return
    setBusy(true)
    setRefusal(null)
    api
      .submitRequest({
        ...(title.trim() !== '' ? { title: title.trim() } : {}),
        text: text.trim(),
        ...(project !== '' ? { project } : {}),
      })
      .then(
        (v) => {
          setBusy(false)
          onBorn(v)
        },
        (err: unknown) => {
          setBusy(false)
          // LOUD, per the map: the pin path's refusals say exactly what was
          // refused and why, in the server's own words — never a quiet drop.
          if (err instanceof ApiError && err.code === 'not_found' && project !== '') {
            setRefusal({
              head: `The project pin was refused — nothing was submitted.`,
              detail: `No project "${project}" is registered to you. ${err.message}`,
            })
          } else if (err instanceof ApiError && err.code === 'project_not_active') {
            setRefusal({
              head: `"${project}" is not active yet — nothing was submitted.`,
              detail: `${err.message} Finish the project's onboarding approval first, or submit without the pin.`,
            })
          } else {
            setRefusal({ head: 'The submission did not land.', detail: describeError(err) })
          }
        },
      )
  }

  return (
    <div className="door-ask">
      <p className="door-kicker">
        <Sparkles size={13} strokeWidth={2} aria-hidden="true" /> Give it work
      </p>
      <h2 className="door-head">Describe a goal in plain words</h2>
      <p className="door-sub">
        Say what you want the way you&apos;d tell a person. It asks its questions next — as a short form — and shows
        you its plan with a price before anything runs. Nothing spends until you approve.
      </p>

      <label className="door-field">
        <span className="door-label">The goal</span>
        <textarea
          className="door-text"
          rows={4}
          value={text}
          placeholder="e.g. an online shop for GPU parts — dark theme, card checkout, order emails"
          onChange={(e) => {
            setText(e.target.value)
          }}
          data-ask="text"
        />
      </label>

      <label className="door-field">
        <span className="door-label">
          A short name for the board <span className="door-optional">optional — it names itself otherwise</span>
        </span>
        <input
          className="door-input"
          type="text"
          value={title}
          onChange={(e) => {
            setTitle(e.target.value)
          }}
          data-ask="title"
        />
      </label>

      <label className="door-field">
        <span className="door-label">
          Into a project <span className="door-optional">optional</span>
        </span>
        <select
          className="door-input door-select"
          value={project}
          onChange={(e) => {
            setProject(e.target.value)
          }}
          data-ask="project"
        >
          <option value="">no project — plan it standalone</option>
          {projects.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </select>
      </label>

      {project !== '' && (
        <p className="door-world" data-door-world={project}>
          <FolderOpen size={14} strokeWidth={1.8} aria-hidden="true" />
          <span>
            This task is pinned to <b>{project}</b> and builds on that project&apos;s world: the planner reads the
            project&apos;s accumulated work, its conventions and commands are injected up front, and the interview
            skips what the project record already answers.
          </span>
        </p>
      )}

      <div className="door-acts">
        <Button variant="primary" onClick={submit} disabled={empty || busy} aria-busy={busy} data-ask="submit">
          {busy ? 'Sending…' : 'Send it — plan this goal'}
        </Button>
        {empty && !busy && <span className="door-why">say what you want first — the box above is the only required field</span>}
      </div>

      {refusal !== null && (
        <div className="door-refusal" role="alert">
          <CircleAlert size={16} strokeWidth={2} aria-hidden="true" />
          <div>
            <p className="refusal-head">{refusal.head}</p>
            <p className="refusal-detail">{refusal.detail}</p>
          </div>
        </div>
      )}
    </div>
  )
}

/* ── the journey: card after card until approved ─────────────────────────── */

/** What each card kind asks of the person, in one line under the header. */
const kindLine: Record<string, string> = {
  interview: 'Its questions — pick an option or answer in your own words. What you skip, it will assume out loud.',
  clarification: 'The drafted spec has open markers. Approval opens once these are answered.',
  escalation: 'One question it cannot proceed without.',
  'decision.coverage': 'The plan cannot cover everything you asked. Decide what happens to the gap.',
  'decision.research': 'It is missing a fact it would otherwise have to research. Supply it, or send it to re-plan.',
  'decision.spec_doubt': 'It doubts the spec captures what you meant. This card is never skipped.',
  approval: 'The plan. Nothing runs and nothing spends until you approve it.',
  'approval.delta': 'The plan changed after your approval. Exactly what changed is below — nothing else moved.',
}

function Journey({
  view,
  onView,
  stream,
}: {
  view: IntakeTaskView
  onView: (v: IntakeTaskView) => void
  stream?: EventStream
}) {
  const card = view.open_card
  const phase = view.phase ?? ''

  return (
    <div className="door-journey" data-task={view.task_id}>
      <JourneyHead view={view} />
      {phase === 'approved' ? (
        <Landed view={view} />
      ) : phase === 'cancelled' ? (
        <Cancelled view={view} />
      ) : card === undefined || view.open_ask_id === undefined || view.open_ask_id === '' ? (
        <FollowTask view={view} onView={onView} stream={stream} />
      ) : (
        <CardPanel key={`${view.open_ask_id}:${String(card.version ?? 0)}`} view={view} card={card} askID={view.open_ask_id} onView={onView} />
      )}
    </div>
  )
}

/**
 * The follow state: the pipeline issues its cards a beat AFTER a write
 * returns (the intake run goes through the scheduler), so a view with no open
 * gate FOLLOWS the task through the caller's own approvals read — the ask
 * item carries the full card — and the journey resumes in place the moment
 * the next card exists. The same card sits in the Inbox throughout.
 */
function FollowTask({
  view,
  onView,
  stream,
}: {
  view: IntakeTaskView
  onView: (v: IntakeTaskView) => void
  stream?: EventStream
}) {
  const asks = useLive({
    key: `/api/approvals#door:${view.task_id}`,
    read: () => api.approvals(),
    types: inboxEventTypes,
    stream,
  })
  const item = (asks.data?.items ?? []).find((i) => i.kind === 'ask' && i.task_id === view.task_id)
  // `card` is the stored ask snapshot the approvals read serves (api.ts): for
  // an intake ask that IS the IntakeCard shape, by the same producer.
  const card = item?.card as IntakeCard | undefined

  if (item !== undefined && card !== undefined) {
    const askID = item.id.startsWith('ask:') ? item.id.slice(4) : item.id
    return (
      <CardPanel
        key={`${askID}:${String(card.version ?? 0)}`}
        view={{ ...view, open_card: card }}
        card={card}
        askID={askID}
        onView={onView}
      />
    )
  }
  return <NoCardYet view={view} waiting={asks.data !== null} />
}

/** The journey's standing header: whose task, where it stands, the clearance
 *  meter. The clearance is the S06.5 measure the platform served — the meter
 *  renders it, it never recomputes it. */
function JourneyHead({ view }: { view: IntakeTaskView }) {
  const clearance = view.open_card?.clearance ?? view.clearance
  return (
    <header className="journey-head">
      <div className="journey-title">
        <h2 className="m-0">{view.title !== '' ? view.title : 'Untitled goal'}</h2>
        <p className="journey-under mono">
          {view.task_id} · <Owner id={view.owner} />
          {view.family !== undefined && view.family !== '' && <> · {view.family}</>}
        </p>
      </div>
      <div className="journey-side">
        {view.tier !== undefined && view.tier !== '' && (
          <span className="journey-tier">
            <Chip tone={view.tier === 'high' ? 'red' : view.tier === 'medium' ? 'orange' : 'blue'}>
              stakes: {view.tier}
            </Chip>
          </span>
        )}
        {clearance !== undefined && <ClearanceMeter value={clearance} />}
      </div>
    </header>
  )
}

/** The served S06.5 clearance: how settled the must-knows are, on the
 *  platform's own 0–100 scale. The figure prints as served; only the fill
 *  width is scaled onto the track. */
function ClearanceMeter({ value }: { value: number }) {
  const ratio = Math.max(0, Math.min(1, value > 1 ? value / 100 : value))
  return (
    <div
      className="clearance"
      title="Clearance — how much of what it must know before planning is settled, out of 100. The platform computes it; answering raises it."
      data-clearance={String(value)}
    >
      <span className="clearance-label">Clearance</span>
      <span className="clearance-track" aria-hidden="true">
        <span className="clearance-fill" style={{ width: `${String(ratio * 100)}%` }} />
      </span>
      <b className="mono">{String(value)}</b>
    </div>
  )
}

/* ── the card panel: one component per card kind ─────────────────────────── */

function CardPanel({
  view,
  card,
  askID,
  onView,
}: {
  view: IntakeTaskView
  card: IntakeCard
  askID: string
  onView: (v: IntakeTaskView) => void
}) {
  const [busy, setBusy] = useState(false)
  const [refusal, setRefusal] = useState('')
  const [needPin, setNeedPin] = useState(false)
  const [pin, setPin] = useState('')

  // ONE answer path for every card kind: send, fold the returned view in, and
  // render any refusal in the server's own words. `pin_required` reveals the
  // PIN field instead of failing — the S01.9 step-up rides the same request.
  const answer = (body: IntakeAnswerBody) => {
    if (busy) return
    setBusy(true)
    setRefusal('')
    api.answerAsk(askID, { answer: body, ...(pin !== '' ? { pin } : {}) }).then(
      (v) => {
        setBusy(false)
        setNeedPin(false)
        setPin('')
        onView(v)
      },
      (err: unknown) => {
        setBusy(false)
        if (err instanceof ApiError && err.code === 'pin_required') {
          setNeedPin(true)
          setRefusal('This approval is High stakes: enter your PIN below and confirm again — it rides the same request and is never stored.')
          return
        }
        if (err instanceof ApiError && err.code === 'pin_rejected') {
          setNeedPin(true)
          setRefusal('The PIN was not accepted. Try again.')
          return
        }
        setRefusal(describeError(err))
      },
    )
  }

  const kind = card.kind
  return (
    <div className="door-card" data-card-kind={kind}>
      <p className="card-kind-line">{kindLine[kind] ?? `A ${kind} card — its controls are below.`}</p>
      {kind === 'interview' && <QuestionForm card={card} busy={busy} allowAssume onAnswer={answer} />}
      {kind === 'clarification' && <QuestionForm card={card} busy={busy} allowAssume={false} onAnswer={answer} />}
      {kind === 'escalation' && <EscalationForm card={card} busy={busy} onAnswer={answer} />}
      {(kind === 'decision.coverage' || kind === 'decision.research' || kind === 'decision.spec_doubt') && (
        <DecisionForm card={card} busy={busy} onAnswer={answer} />
      )}
      {kind === 'approval' && <PlanCard view={view} card={card} busy={busy} onAnswer={answer} />}
      {kind === 'approval.delta' && <DeltaForm card={card} busy={busy} onAnswer={answer} />}
      {!(kind in kindLine) && (
        <p className="muted">
          This card kind has no form here yet — answer it from{' '}
          <Link to={hrefFor('inbox-item', { id: `ask:${askID}` })}>its inbox card</Link>, and the journey resumes in
          place.
        </p>
      )}

      {needPin && (
        <label className="door-field door-pin">
          <span className="door-label">Your PIN</span>
          <input
            className="door-input"
            type="password"
            inputMode="numeric"
            autoComplete="off"
            value={pin}
            onChange={(e) => {
              setPin(e.target.value)
            }}
            data-ask="pin"
          />
        </label>
      )}
      {refusal !== '' && (
        <div className="door-refusal" role="alert">
          <CircleAlert size={16} strokeWidth={2} aria-hidden="true" />
          <p className="refusal-detail">{refusal}</p>
        </div>
      )}
      <JourneyFoot view={view} onView={onView} busy={busy} />
    </div>
  )
}

/** Cancel is always available pre-approval (4.5) — quiet, but present, and it
 *  says what it does. The inbox note keeps the tab-lifetime state honest. */
function JourneyFoot({ view, onView, busy }: { view: IntakeTaskView; onView: (v: IntakeTaskView) => void; busy: boolean }) {
  const [cancelBusy, setCancelBusy] = useState(false)
  const [note, setNote] = useState('')
  return (
    <footer className="journey-foot">
      <span className="foot-note">
        This card also sits in your <Link to={hrefFor('inbox')}>Inbox</Link> — leaving this page loses nothing.
      </span>
      <Button
        variant="ghost"
        size="sm"
        disabled={busy || cancelBusy}
        data-door-cancel
        onClick={() => {
          setCancelBusy(true)
          setNote('')
          api.cancelTask(view.task_id).then(
            (res) => {
              setCancelBusy(false)
              if (res.applied) onView({ ...view, phase: 'cancelled', open_ask_id: '', open_card: undefined })
              else setNote(res.runs[0]?.detail ?? 'nothing was cancelled')
            },
            (err: unknown) => {
              setCancelBusy(false)
              setNote(describeError(err))
            },
          )
        }}
      >
        {cancelBusy ? 'Cancelling…' : 'Cancel this task'}
      </Button>
      {note !== '' && <span className="foot-note">{note}</span>}
    </footer>
  )
}

/* ── interview / clarification: the real form ────────────────────────────── */

/**
 * The interview as a FORM. Each question renders its 2–4 labeled options as
 * real buttons plus an always-present own-words input; the submit row is
 * unmissable — the primary act is enabled the moment one question is
 * answered, the disabled state prints its reason beside it, and the
 * force-proceed arm says exactly what it does with what you skipped.
 */
function QuestionForm({
  card,
  busy,
  allowAssume,
  onAnswer,
}: {
  card: IntakeCard
  busy: boolean
  allowAssume: boolean
  onAnswer: (b: IntakeAnswerBody) => void
}) {
  const questions = card.questions ?? []
  const [picked, setPicked] = useState<Record<string, string>>({})
  const [own, setOwn] = useState<Record<string, string>>({})

  // The effective answer per question: own words win over a picked option,
  // because the person typed them later and free text is always available.
  const valueOf = (q: IntakeQuestion): string => {
    const typed = (own[q.id] ?? '').trim()
    if (typed !== '') return typed
    return picked[q.id] ?? ''
  }
  const answered = questions.filter((q) => valueOf(q) !== '')
  const openCount = questions.length - answered.length

  const send = (force: boolean) => {
    onAnswer({
      answers: answered.map((q) => ({ id: q.id, value: valueOf(q) })),
      ...(force ? { force_proceed: true } : {}),
    })
  }

  return (
    <div className="q-form">
      {questions.map((q, i) => (
        <fieldset className="q-card" key={q.id} data-question={q.id}>
          <legend className="q-legend">
            <span className="q-n mono">{String(i + 1)}</span> {q.text}
          </legend>
          <div className="q-options" role="group" aria-label={`options for: ${q.text}`}>
            {(q.options ?? []).map((o) => {
              const active = picked[q.id] === o.value && (own[q.id] ?? '').trim() === ''
              return (
                <button
                  key={o.value}
                  type="button"
                  className="q-option"
                  data-active={active ? 'true' : undefined}
                  aria-pressed={active}
                  onClick={() => {
                    setPicked((p) => (p[q.id] === o.value ? { ...p, [q.id]: '' } : { ...p, [q.id]: o.value }))
                    setOwn((o2) => ({ ...o2, [q.id]: '' }))
                  }}
                >
                  {active && <Check size={13} strokeWidth={2.5} aria-hidden="true" />}
                  {o.label}
                </button>
              )
            })}
          </div>
          <input
            className="door-input q-own"
            type="text"
            placeholder="or answer in your own words…"
            value={own[q.id] ?? ''}
            onChange={(e) => {
              setOwn((o2) => ({ ...o2, [q.id]: e.target.value }))
            }}
          />
        </fieldset>
      ))}

      <div className="door-acts">
        <Button
          variant="primary"
          disabled={answered.length === 0 || busy}
          aria-busy={busy}
          data-interview="send"
          onClick={() => {
            send(false)
          }}
        >
          {busy
            ? 'Sending…'
            : openCount > 0
              ? `Send ${String(answered.length)} answer${answered.length === 1 ? '' : 's'}`
              : 'Send my answers'}
        </Button>
        {answered.length === 0 && !busy && (
          <span className="door-why">
            {allowAssume
              ? 'answer at least one question — or proceed below and it will assume the rest, out loud'
              : 'these markers block approval — answer at least one to move'}
          </span>
        )}
        {answered.length > 0 && openCount > 0 && !busy && allowAssume && (
          <span className="door-why">it will ask again about the {String(openCount)} you skipped</span>
        )}
      </div>

      {allowAssume && (
        <div className="door-acts">
          <Button
            variant="secondary"
            disabled={busy}
            data-interview="force"
            onClick={() => {
              send(true)
            }}
          >
            Proceed — turn open questions into assumptions
          </Button>
          <span className="door-why">
            what you don&apos;t answer becomes a LISTED assumption on the plan card, where you can still contest it
          </span>
        </div>
      )}
    </div>
  )
}

function EscalationForm({ card, busy, onAnswer }: { card: IntakeCard; busy: boolean; onAnswer: (b: IntakeAnswerBody) => void }) {
  const [text, setText] = useState('')
  const q = (card.questions ?? [])[0]
  return (
    <div className="q-form">
      {q !== undefined && (
        <fieldset className="q-card">
          <legend className="q-legend">{q.text}</legend>
          <textarea
            className="door-text"
            rows={3}
            value={text}
            onChange={(e) => {
              setText(e.target.value)
            }}
          />
        </fieldset>
      )}
      <div className="door-acts">
        <Button
          variant="primary"
          disabled={text.trim() === '' || busy}
          aria-busy={busy}
          onClick={() => {
            onAnswer({ text: text.trim() })
          }}
        >
          {busy ? 'Sending…' : 'Send the answer'}
        </Button>
        {text.trim() === '' && !busy && <span className="door-why">this one needs words — there is no option row</span>}
      </div>
    </div>
  )
}

/* ── the decision cards ──────────────────────────────────────────────────── */

/**
 * Coverage / research / spec-doubt: the choices are the card's own, rendered
 * as real buttons. The two choices that need structured entry (drop_criterion,
 * supply_fact) open their entry inline and their act stays disabled-with-why
 * until the entry is filled.
 */
function DecisionForm({ card, busy, onAnswer }: { card: IntakeCard; busy: boolean; onAnswer: (b: IntakeAnswerBody) => void }) {
  const d = card.decision
  const [choice, setChoice] = useState('')
  const [criteria, setCriteria] = useState('')
  const [ruleID, setRuleID] = useState('')
  const [fact, setFact] = useState('')
  if (d === undefined) return <p className="muted">This decision card carried no body — its controls live on its inbox card.</p>

  const needsCriteria = choice === 'drop_criterion'
  const needsFact = choice === 'supply_fact'
  const criteriaList = criteria
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '')
  const blocked = (needsCriteria && criteriaList.length === 0) || (needsFact && (ruleID.trim() === '' || fact.trim() === ''))

  return (
    <div className="q-form">
      <div className="q-card">
        <p className="decision-summary">{d.summary}</p>
        {(d.detail ?? []).length > 0 && (
          <ul className="decision-detail">
            {d.detail?.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        )}
      </div>

      <div className="q-options" role="group" aria-label="your decision">
        {d.choices.map((c) => (
          <button
            key={c.value}
            type="button"
            className="q-option"
            data-active={choice === c.value ? 'true' : undefined}
            aria-pressed={choice === c.value}
            onClick={() => {
              setChoice(c.value)
            }}
          >
            {choice === c.value && <Check size={13} strokeWidth={2.5} aria-hidden="true" />}
            {c.label}
          </button>
        ))}
      </div>

      {needsCriteria && (
        <label className="door-field">
          <span className="door-label">Which criteria to drop — their keys, comma-separated (they are listed above)</span>
          <input
            className="door-input"
            type="text"
            placeholder="e.g. AC-2, AC-4"
            value={criteria}
            onChange={(e) => {
              setCriteria(e.target.value)
            }}
          />
        </label>
      )}
      {needsFact && (
        <>
          <label className="door-field">
            <span className="door-label">Which research trigger this answers — its rule id (listed above)</span>
            <input
              className="door-input"
              type="text"
              value={ruleID}
              onChange={(e) => {
                setRuleID(e.target.value)
              }}
            />
          </label>
          <label className="door-field">
            <span className="door-label">The fact, in your words — it is recorded on the spec as yours</span>
            <textarea
              className="door-text"
              rows={2}
              value={fact}
              onChange={(e) => {
                setFact(e.target.value)
              }}
            />
          </label>
        </>
      )}

      <HelpNote help={d.help} />
      <div className="door-acts">
        <Button
          variant="primary"
          disabled={choice === '' || blocked || busy}
          aria-busy={busy}
          onClick={() => {
            onAnswer({
              choice,
              ...(needsCriteria ? { criteria: criteriaList } : {}),
              ...(needsFact ? { facts: [{ rule_id: ruleID.trim(), fact: fact.trim() }] } : {}),
            })
          }}
        >
          {busy ? 'Sending…' : 'Record this decision'}
        </Button>
        {choice === '' && !busy && <span className="door-why">pick one of the choices above first</span>}
        {choice !== '' && blocked && !busy && (
          <span className="door-why">{needsCriteria ? 'name at least one criterion key' : 'both fields are needed — the fact is recorded as yours'}</span>
        )}
      </div>
    </div>
  )
}

/* ── the plan card (Stage-4 approval, map §3 anatomy) ────────────────────── */

const planActionLabels: Record<string, string> = {
  approve: 'Approve — start the work',
  replan: 'Re-plan…',
  reinterview: 'Re-interview',
  cancel: 'Cancel the task',
  compose: 'Compose a specialist for this…',
}

function PlanCard({
  view,
  card,
  busy,
  onAnswer,
}: {
  view: IntakeTaskView
  card: IntakeCard
  busy: boolean
  onAnswer: (b: IntakeAnswerBody) => void
}) {
  const a = card.approval
  const [contesting, setContesting] = useState(false)
  const [target, setTarget] = useState('')
  const [note, setNote] = useState('')
  if (a === undefined) return <p className="muted">This approval card carried no body — approve it from its inbox card.</p>

  const l1 = a.layer1
  const steps = a.layer2?.steps ?? []
  const acs = a.layer2?.acs ?? []
  const estimate = a.layer2?.estimate
  const actions = a.actions ?? ['approve', 'replan', 'reinterview', 'cancel']

  // Re-plan's structured entry (S06.9): tap the step, criterion or assumption
  // being contested. The targets are the card's own keys.
  const contestTargets: { key: string; label: string }[] = [
    ...steps.map((s) => ({ key: s.id, label: `${s.id} — ${s.title}` })),
    ...acs.map((ac) => ({ key: `AC-${String(ac.n)}`, label: `AC-${String(ac.n)} — ${ac.plain}` })),
    ...(l1.assumptions ?? []).map((as) => ({ key: `assumption:${as.text}`, label: `assumption — ${as.text}` })),
  ]

  return (
    <div className="plan-card">
      {a.stale_flag === true && (
        <p className="plan-stale" role="alert">
          <CircleAlert size={14} strokeWidth={2} aria-hidden="true" /> Assumptions may be stale:{' '}
          {(a.stale_reasons ?? []).join(' · ') || 'the world moved since this card was drafted'} — re-plan refreshes it, and
          approving anyway is still yours to choose.
        </p>
      )}

      <section className="plan-sec" data-plan="understood">
        <h3 className="plan-h">What I understood</h3>
        <p className="plan-restatement">{l1.restatement}</p>
      </section>

      {(l1.deliverable ?? []).length > 0 && (
        <section className="plan-sec" data-plan="deliverable">
          <h3 className="plan-h">What you&apos;ll get</h3>
          <ul className="plan-list">
            {l1.deliverable?.map((d) => (
              <li key={d}>{d}</li>
            ))}
          </ul>
        </section>
      )}

      <section className="plan-sec" data-plan="steps">
        <h3 className="plan-h">The steps, in order</h3>
        {steps.length > 0 ? (
          <ol className="plan-steps">
            {steps.map((s) => (
              <li key={s.id}>
                <span className="step-key mono">{s.id}</span>
                <span className="step-body">
                  {s.title}
                  {s.done_when !== undefined && s.done_when !== '' && (
                    <span className="step-done">done when: {s.done_when}</span>
                  )}
                </span>
              </li>
            ))}
          </ol>
        ) : (
          <ol className="plan-steps">
            {(l1.steps ?? []).map((s, i) => (
              <li key={s}>
                <span className="step-key mono">{String(i + 1)}</span>
                <span className="step-body">{s}</span>
              </li>
            ))}
          </ol>
        )}
      </section>

      {(l1.will_not_do ?? []).length > 0 && (
        <section className="plan-sec" data-plan="will-not-do">
          <h3 className="plan-h plan-h-not">What I will NOT do</h3>
          <ul className="plan-list plan-not">
            {l1.will_not_do?.map((w) => (
              <li key={w}>
                <X size={13} strokeWidth={2.5} aria-hidden="true" /> {w}
              </li>
            ))}
          </ul>
        </section>
      )}

      <section className="plan-sec plan-assumptions" data-plan="assumptions">
        <h3 className="plan-h">Assumptions — read these first</h3>
        {(l1.assumptions ?? []).length === 0 ? (
          <p className="muted m-0">None. Everything it needed, you answered.</p>
        ) : (
          <ul className="plan-list">
            {l1.assumptions?.map((as) => (
              <li key={as.text}>
                {as.text}
                {as.origin !== undefined && as.origin !== '' && <span className="assume-origin"> · {originWords(as.origin)}</span>}
              </li>
            ))}
          </ul>
        )}
      </section>

      {(l1.uncovered ?? []).length > 0 && (
        <section className="plan-sec" data-plan="uncovered">
          <h3 className="plan-h plan-h-not">Accepted gaps</h3>
          <ul className="plan-list plan-not">
            {l1.uncovered?.map((u) => (
              <li key={u}>{u} — you accepted this stays uncovered</li>
            ))}
          </ul>
        </section>
      )}

      <section className="plan-sec plan-costs" data-plan="cost">
        <h3 className="plan-h">Cost and time</h3>
        <p className="m-0">
          {l1.cost_time !== undefined && l1.cost_time !== '' ? l1.cost_time : 'no time estimate was drafted'}
          {estimate !== undefined && (
            <>
              {' · '}
              {estimate.known ? (
                <b className="mono">≈ USD {String(estimate.usd ?? 0)}</b>
              ) : (
                <span className="muted">cost unpriced — no honest dollar figure exists yet</span>
              )}
              {estimate.size_class !== undefined && estimate.size_class !== '' && <> · size: {estimate.size_class}</>}
            </>
          )}
          {l1.size_note !== undefined && l1.size_note !== '' && <span className="warn-flag"> · {l1.size_note}</span>}
        </p>
      </section>

      {a.routing !== undefined && (
        <section className="plan-sec" data-plan="routing">
          <h3 className="plan-h">Who does it</h3>
          <p className="m-0">
            {a.routing.generalist === true ? 'the generalist' : (a.routing.worker_name ?? 'the selected specialist')}
            {a.routing.model !== undefined && a.routing.model !== '' && (
              <span className="muted"> · {a.routing.model}{a.routing.effort !== undefined && a.routing.effort !== '' ? ` (${a.routing.effort})` : ''}</span>
            )}
            {a.routing.plain_reason !== undefined && a.routing.plain_reason !== '' && (
              <span className="muted"> — {a.routing.plain_reason}</span>
            )}
          </p>
        </section>
      )}

      <HelpNote help={l1.help} />

      {!contesting ? (
        <div className="door-acts plan-acts">
          {actions.includes('approve') && (
            <Button
              variant="primary"
              disabled={busy}
              aria-busy={busy}
              data-plan-act="approve"
              onClick={() => {
                onAnswer({ action: 'approve' })
              }}
            >
              {busy ? 'Approving…' : planActionLabels.approve}
            </Button>
          )}
          {actions.includes('replan') && (
            <Button
              variant="secondary"
              disabled={busy}
              data-plan-act="replan"
              onClick={() => {
                setContesting(true)
              }}
            >
              {planActionLabels.replan}
            </Button>
          )}
          {actions.includes('reinterview') && (
            <Button
              variant="secondary"
              disabled={busy}
              data-plan-act="reinterview"
              onClick={() => {
                onAnswer({ action: 'reinterview' })
              }}
            >
              {planActionLabels.reinterview}
            </Button>
          )}
          {actions.includes('cancel') && (
            <Button
              variant="danger"
              disabled={busy}
              data-plan-act="cancel"
              onClick={() => {
                onAnswer({ action: 'cancel' })
              }}
            >
              {planActionLabels.cancel}
            </Button>
          )}
        </div>
      ) : (
        <div className="contest" data-plan="contest">
          <h3 className="plan-h">Re-plan — tap what you&apos;re contesting</h3>
          <div className="q-options">
            {contestTargets.map((t) => (
              <button
                key={t.key}
                type="button"
                className="q-option contest-target"
                data-active={target === t.key ? 'true' : undefined}
                aria-pressed={target === t.key}
                onClick={() => {
                  setTarget(t.key)
                }}
              >
                {target === t.key && <Check size={13} strokeWidth={2.5} aria-hidden="true" />}
                {t.label}
              </button>
            ))}
          </div>
          <label className="door-field">
            <span className="door-label">
              What&apos;s wrong with it <span className="door-optional">optional, but it plans better with words</span>
            </span>
            <input
              className="door-input"
              type="text"
              value={note}
              onChange={(e) => {
                setNote(e.target.value)
              }}
            />
          </label>
          <div className="door-acts">
            <Button
              variant="primary"
              disabled={target === '' || busy}
              aria-busy={busy}
              data-plan-act="send-replan"
              onClick={() => {
                onAnswer({ action: 'replan', contest: { target, ...(note.trim() !== '' ? { note: note.trim() } : {}) } })
              }}
            >
              {busy ? 'Sending…' : 'Send it back to plan'}
            </Button>
            <Button
              variant="ghost"
              disabled={busy}
              onClick={() => {
                setContesting(false)
                setTarget('')
              }}
            >
              Back to the plan
            </Button>
            {target === '' && !busy && <span className="door-why">pick the step, criterion or assumption you&apos;re contesting</span>}
          </div>
        </div>
      )}
      {view.tier === 'high' && !contesting && (
        <p className="muted mt-1 text-xs">High stakes: approving asks for your PIN in the same breath.</p>
      )}
    </div>
  )
}

/** The S06.6 assumption origins, in the reader's words. */
function originWords(origin: string): string {
  if (origin.startsWith('slot:')) return `assumed because you skipped "${origin.slice(5)}"`
  if (origin === 'force_proceed') return 'assumed because you chose to proceed'
  if (origin === 'planner') return 'the planner assumed it'
  if (origin === 'band') return 'assumed by the low-stakes band'
  return origin
}

function DeltaForm({ card, busy, onAnswer }: { card: IntakeCard; busy: boolean; onAnswer: (b: IntakeAnswerBody) => void }) {
  const d = card.delta
  if (d === undefined) return <p className="muted">This delta card carried no body — answer it from its inbox card.</p>
  const actions = d.actions ?? ['approve', 'reject']
  return (
    <div className="q-form">
      <div className="q-card">
        <p className="decision-summary">
          Origin: {d.origin ?? 'unrecorded'} — only the items below changed. Everything else you approved stands.
        </p>
        <ul className="delta-items">
          {(d.items ?? []).map((it) => (
            <li key={`${it.kind}:${it.target}`}>
              <b className="mono">{it.target}</b> <span className="muted">({it.kind})</span>
              {it.old !== undefined && it.old !== '' && <span className="delta-old"> was: {it.old}</span>}
              {it.new !== undefined && it.new !== '' && <span className="delta-new"> now: {it.new}</span>}
            </li>
          ))}
        </ul>
      </div>
      <HelpNote help={d.help} />
      <div className="door-acts">
        {actions.includes('approve') && (
          <Button
            variant="primary"
            disabled={busy}
            aria-busy={busy}
            onClick={() => {
              onAnswer({ action: 'approve' })
            }}
          >
            {busy ? 'Sending…' : 'Approve the change'}
          </Button>
        )}
        {actions.includes('reject') && (
          <Button
            variant="danger"
            disabled={busy}
            onClick={() => {
              onAnswer({ action: 'reject' })
            }}
          >
            Reject it
          </Button>
        )}
      </div>
    </div>
  )
}

/** The 13.5 help block, rendered quietly under the card it advises. */
function HelpNote({ help }: { help?: IntakeHelp }) {
  if (help === undefined) return null
  const lines = [help.what, help.wrong, help.recommend].filter((s): s is string => s !== undefined && s !== '')
  if (lines.length === 0) return null
  return (
    <div className="help-note">
      {lines.map((l) => (
        <p key={l}>{l}</p>
      ))}
    </div>
  )
}

/* ── the journey's terminal states ───────────────────────────────────────── */

/** The visible result finding 5 never had: task created, approved, and the
 *  doors to watch it. */
function Landed({ view }: { view: IntakeTaskView }) {
  return (
    <div className="door-landed" data-door-result="approved">
      <p className="landed-mark" aria-hidden="true">
        <Check size={26} strokeWidth={2.5} />
      </p>
      <h3 className="landed-head">Approved — the work is starting</h3>
      <p className="landed-sub">
        <b>{view.title !== '' ? view.title : view.task_id}</b> is on the board
        {view.kanban_status !== '' && <> under &quot;{view.kanban_status === 'intake' ? 'Backlog' : view.kanban_status}&quot;</>}. It moves
        column by column as the platform works; anything that needs you lands in the Inbox.
      </p>
      <div className="door-acts">
        <Button
          variant="primary"
          data-landed="board"
          onClick={() => {
            navigate(hrefFor('board'))
          }}
        >
          Watch it on the Board
        </Button>
        <Button
          variant="secondary"
          data-landed="task"
          onClick={() => {
            navigate(hrefFor('task', { id: view.task_id }))
          }}
        >
          Open the task card
        </Button>
      </div>
    </div>
  )
}

function Cancelled({ view }: { view: IntakeTaskView }) {
  return (
    <div className="door-landed" data-door-result="cancelled">
      <p className="landed-mark landed-mark-off" aria-hidden="true">
        <X size={26} strokeWidth={2.5} />
      </p>
      <h3 className="landed-head">Cancelled — nothing runs</h3>
      <p className="landed-sub">
        {view.title !== '' ? view.title : view.task_id} keeps its record on the board with a cancelled sign; no work
        starts and nothing is spent.
      </p>
      <div className="door-acts">
        <Button
          variant="secondary"
          onClick={() => {
            navigate(hrefFor('board'))
          }}
        >
          Back to the Board
        </Button>
      </div>
    </div>
  )
}

/** Born, no open gate YET: the follow state's face while the pipeline works
 *  toward its next card. This page resumes by itself when the card exists. */
function NoCardYet({ view, waiting }: { view: IntakeTaskView; waiting: boolean }) {
  return (
    <div className="door-landed" data-door-result="no-card">
      <h3 className="landed-head">
        {view.tier === 'trivial' ? 'It took the work without questions' : 'The task is born — it is working'}
      </h3>
      <p className="landed-sub">
        {view.tier === 'trivial' ? (
          <>Trivial, read-only asks skip the ceremony on purpose. If a question comes up it lands in your{' '}
          <Link to={hrefFor('inbox')}>Inbox</Link>.</>
        ) : (
          <>
            {waiting ? 'This page is listening: ' : ''}its questions — or its plan — appear RIGHT HERE the moment they
            exist, and land in your <Link to={hrefFor('inbox')}>Inbox</Link> too. You can also leave; nothing is lost.
          </>
        )}
      </p>
      <div className="door-acts">
        <Button
          variant="primary"
          onClick={() => {
            navigate(hrefFor('board'))
          }}
        >
          Watch it on the Board
        </Button>
        <Button
          variant="secondary"
          onClick={() => {
            navigate(hrefFor('task', { id: view.task_id }))
          }}
        >
          Open the task card
        </Button>
      </div>
    </div>
  )
}
