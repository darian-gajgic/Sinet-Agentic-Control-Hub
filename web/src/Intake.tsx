import { useEffect, useMemo, useRef, useState } from 'react'
import { Check, CircleAlert, CornerDownRight, FolderOpen, Pencil, Pin, Sparkles, X } from 'lucide-react'

import {
  ApiError,
  Unreachable,
  api,
  type IntakeAnswerBody,
  type IntakeCard,
  type IntakeHelp,
  type IntakeQuestion,
  type IntakeTaskView,
  type IntakeUnderstood,
  type PinnableLane,
  type Session,
  type User,
} from './api'
import { whyHolds, whyOverLine } from './controls'
import type { EventStream } from './events'
import { describeError, inboxEventTypes, useLive } from './live'
import { Owner } from './parts'
import { Link, navigate } from './router'
import { hrefFor } from './routes'
import { SignInFirstDoor } from './signinfirst'
import { Button, Chip } from './ui'

/**
 * The give-work door (map §3 v3; Spec S06.5–S06.9 through the landed intake
 * routes) — the journey that kills finding 5.
 *
 * One plain ask box → the interview as a GUIDED SEQUENCE (P3-GF3-FE: one
 * decision in view at a time, each with the served recommendation as a
 * one-click take, a per-question skip riding {id,skip:true}, answered/skipped
 * state on every row, the self-explaining must-knows meter with its served
 * floor) → the plan card (what I understood · what you'll get · numbered
 * steps with done-when · what I will NOT do · assumptions front-and-center ·
 * cost/time) → Approve / Change the plan… (multi-contest + own words) /
 * Change my answers… (the review card) / Cancel.
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
export function DescribeGoal({
  search = '',
  stream,
  session,
  onSignedIn,
}: {
  search?: string
  stream?: EventStream
  /** The current identity, when the shell passes it. `dev: true` walls the
   *  door behind sign-in (W1-B1) — work born here must have a real owner. */
  session?: Session
  onSignedIn?: () => void
}) {
  const [view, setView] = useState<IntakeTaskView | null>(null)
  const params = new URLSearchParams(search)
  const pinned = params.get('project') ?? ''
  const resumeTask = params.get('task') ?? ''
  // `r` is the answered-rounds stamp the journey writes into its own address
  // (the §41-B one-pocket rule): a reload mid-interview knows THIS browser
  // already answered, so the wait face never greets the task as newborn again
  // (review #7 — the served task read carries no "answers were given" fact,
  // reported as a wire note).
  const resumedRounds = Number(params.get('r') ?? '0') || 0

  // A born task stamps its id onto the door's own address, so a reload lands
  // back IN the journey (the open card is durable server-side) instead of on
  // a fresh ask box with the interview seemingly lost — the tab holds no
  // storage (§41-B), the URL is the one pocket it has.
  const born = (v: IntakeTaskView) => {
    window.history.replaceState(null, '', `${hrefFor('new')}?task=${encodeURIComponent(v.task_id)}`)
    setView(v)
  }

  // THE SIGN-IN-FIRST WALL (W1-B1, release-gating): under the dev fallback
  // this door presents the sign-in step IN PLACE before any work exists —
  // create AND resume arms, because answering a card advances approval-gated
  // work just as creating one starts it. Signing in reloads the session and
  // the door unlocks under this same address, project pin and all: return to
  // origin by construction, never a dump to Home.
  if (session?.dev === true && onSignedIn !== undefined) {
    return (
      <section className="surface door" data-door="describe-goal">
        <SignInFirstDoor session={session} onSignedIn={onSignedIn} doorWords="This door starts a task." />
      </section>
    )
  }

  return (
    <section className="surface door" data-door="describe-goal">
      {view !== null ? (
        <Journey view={view} onView={setView} answeredRounds={resumedRounds} stream={stream} />
      ) : resumeTask !== '' ? (
        <ResumeJourney taskID={resumeTask} me={session?.user} onView={setView} stream={stream} />
      ) : (
        <AskForm pinned={pinned} onBorn={born} stream={stream} />
      )}
    </section>
  )
}

/**
 * Resume after a reload: a LOADER, not a renderer. It reads the task once and
 * hands the resolved view UP to the door, which then renders the SAME Journey
 * component position the live flow uses — deliberately, because rendering a
 * second Journey here meant the first answer after a resume REMOUNTED the
 * journey and reset its in-session memory (the working-face beat counter),
 * greeting a mid-interview task as newborn (found on the live walk,
 * 2026-08-16). No pipeline state is re-derived: what cannot be known from a
 * served read renders as the follow state's honest waiting face.
 */
function ResumeJourney({
  taskID,
  me,
  onView,
  stream,
}: {
  taskID: string
  /** WHO is resuming, when the shell knows — the refusal below names the
   *  identity instead of promising an Inbox that may not hold the card. */
  me?: User
  onView: (v: IntakeTaskView) => void
  stream?: EventStream
}) {
  const detail = useLive({
    key: `/api/tasks/${taskID}#door`,
    read: () => api.task(taskID),
    types: inboxEventTypes,
    stream,
  })
  const d = detail.data
  useEffect(() => {
    if (d === null) return
    onView({
      task_id: d.task_id,
      title: d.title,
      kanban_status: d.kanban_status,
      owner: d.owner,
      ...(d.kanban_status === 'cancelled' ? { phase: 'cancelled' } : {}),
    })
    // One hand-off: the Journey's own live follow owns the truth from here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [d === null])
  if (detail.error !== '' && detail.data === null) {
    // IDENTITY-AWARE refusal (W1-B1c). The old copy pointed every failure at
    // "your Inbox" — false for the walk's exact shape, where the task belongs
    // to a different identity and the reader's inbox will never hold its
    // card. The server's own classification (403/404) picks the words.
    const notYours = detail.errorStatus === 403 || detail.errorStatus === 404
    return (
      <div className="door-refusal" role="alert">
        <CircleAlert size={16} strokeWidth={2} aria-hidden="true" />
        {notYours ? (
          <p className="refusal-detail">
            This journey could not be resumed{me ? <> as <Owner id={me.user_id} /></> : null}: the platform answers
            that task {taskID} is not among {me ? `${me.display_name}'s` : 'your'} tasks. A task belongs to the
            account that created it, and only that account can see it or answer its questions — if it was started
            under a different sign-in, sign in as that account to continue it.
          </p>
        ) : (
          <p className="refusal-detail">
            This journey could not be resumed — the task {taskID} did not answer. Its card, if one is open, is in
            your <Link to={hrefFor('inbox')}>Inbox</Link>.
          </p>
        )}
      </div>
    )
  }
  return <p className="muted">Resuming the journey…</p>
}

/* ── the ask box ─────────────────────────────────────────────────────────── */

/**
 * titleFromGoal keeps the door's own promise — "it names itself otherwise".
 *
 * The platform mints no title of its own (RW-14 OQ6: tasks.title stays empty on
 * the wire), so a task submitted without one wore its raw `t-…` id on the board
 * and the overlay (design review 2026-08-17, blocker #3). The door is where the
 * human words exist, so the door derives the name: the goal's first sentence,
 * cut at a word boundary. It is DISPLAYED in the title field as the live
 * placeholder before it is sent, so what the board will say is on screen —
 * nothing is named behind the person's back.
 */
export function titleFromGoal(text: string): string {
  // First sentence-ish fragment: cut at the first terminator that ends a word.
  const flat = text.trim().replace(/\s+/g, ' ')
  const m = /^(.{8,79}?[.!?])(?:\s|$)/.exec(flat)
  let head = (m ? m[1].replace(/[.!?]$/, '') : flat).trim()
  if (head.length > 60) {
    const cut = head.slice(0, 60)
    let atWord = cut.slice(0, cut.lastIndexOf(' ') > 24 ? cut.lastIndexOf(' ') : 60).replace(/[,;:\s]+$/, '')
    // A name should not trail off on a connective ("…the order and…").
    atWord = atWord.replace(/\s+(and|or|the|a|an|to|for|of|with|in|on|at|by|it)$/i, '')
    head = `${atWord}…`
  }
  return head === '' ? '' : head.charAt(0).toUpperCase() + head.slice(1)
}

/**
 * The plain ask. The project pin is the LANDED P3-RW-1 field: the registry id
 * verbatim, validated server-side — an invalid pin refuses the submission
 * loudly (404/409 render below the button, nothing is quietly dropped).
 *
 * The pin choices are the REGISTRY's ACTIVE entries (P3-RW-2, consumed
 * 2026-08-11): a pin into a pending or unregistered project is refused
 * server-side, so no such choice is offered (finding-5's rule — a control
 * either works or is not rendered). "(no project)" is not a pin — it is the
 * unpinned submission. A door-carried pin (?project=) always renders so the
 * refusal, if any, is the server's loud one rather than a silent drop.
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
  // The P3-LN-9 per-task lane pin: '' is the ordinary case — no pin, the
  // platform chooses — and the member stays OFF the wire entirely (the
  // unpinned submission is byte-identical to the pre-pin one).
  const [lane, setLane] = useState('')
  const [laneOpen, setLaneOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [refusal, setRefusal] = useState<{ head: string; detail: string } | null>(null)

  // The pinnable set (P3-LN-10a): read ONCE when the form mounts. The set is
  // composed at control-plane startup — a placed key commissions its lane at
  // the NEXT start (P3-LN-4) — so no event type moves it under a running
  // plane, and a live follow here would poll for a fact that cannot change
  // (S15.12's spirit). A failed read is an honest state of its own, rendered
  // in the picker; the unpinned default keeps working regardless.
  const [laneWorld, setLaneWorld] = useState<LaneWorld>(null)
  useEffect(() => {
    let gone = false
    api.pinnableLanes().then(
      (v) => {
        if (!gone) setLaneWorld(v)
      },
      (err: unknown) => {
        if (!gone) setLaneWorld({ failed: describeError(err) })
      },
    )
    return () => {
      gone = true
    }
  }, [])

  // The registry moves through the onboarding task's own frames (no project.*
  // event type exists), which the inbox set carries.
  const registry = useLive({
    key: '/api/projects#door',
    read: () => api.projects(),
    types: inboxEventTypes,
    stream,
  })
  const projects = useMemo(() => {
    const names = new Set(
      (registry.data?.projects ?? []).filter((p) => p.state === 'active').map((p) => p.project_id),
    )
    if (project !== '') names.add(project)
    return [...names].sort((a, b) => a.localeCompare(b))
  }, [registry.data, project])

  const empty = text.trim() === ''
  // The name the task will actually wear: the typed one, or the derived one the
  // placeholder is showing. Sending the derivation is what keeps raw `t-…` ids
  // off the board — the promise under the field, kept (blocker #3).
  const derived = titleFromGoal(text)
  const effectiveTitle = title.trim() !== '' ? title.trim() : derived

  const submit = () => {
    if (empty || busy) return
    setBusy(true)
    setRefusal(null)
    api
      .submitRequest({
        ...(effectiveTitle !== '' ? { title: effectiveTitle } : {}),
        text: text.trim(),
        ...(project !== '' ? { project } : {}),
        ...(lane !== '' ? { pinned_lane: lane } : {}),
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
          } else if (err instanceof ApiError && err.code === 'lane_pin_refused') {
            // The S00.9 A13 refusal: a pin the platform cannot honor refuses
            // the SUBMISSION — never a silent fallback onto routing's own
            // choice. The detail is the server's whole sentence, which names
            // the lanes that ARE pinnable; the picker stays open and armed so
            // the person can re-pick and send again.
            setRefusal({
              head: `The pin to lane "${lane}" was refused — nothing was submitted.`,
              detail: err.message,
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
          A short name for the board{' '}
          <span className="door-optional">
            {derived === '' || title.trim() !== ''
              ? 'optional — it names itself from the goal otherwise'
              : 'optional — left empty, it will be called what the box below shows'}
          </span>
        </span>
        <input
          className="door-input"
          type="text"
          value={title}
          placeholder={derived}
          onChange={(e) => {
            setTitle(e.target.value)
          }}
          data-ask="title"
          data-derived-title={derived !== '' && title.trim() === '' ? derived : undefined}
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

      <LanePicker
        world={laneWorld}
        lane={lane}
        open={laneOpen}
        onOpen={() => {
          setLaneOpen(true)
        }}
        onPick={setLane}
      />

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

/* ── the lane pin (P3-LN-10; S00.9 A13) ──────────────────────────────────── */

/** What the ask box knows about the pinnable set: not yet answered, the
 *  served rows, or the read's own failure — three states, each with its own
 *  honest face below. Never a hardcoded list: the choices are the P3-LN-10a
 *  read VERBATIM, which serves the set the submit boundary validates
 *  `pinned_lane` against. */
export type LaneWorld = { lanes: PinnableLane[] } | { failed: string } | null

/**
 * The per-task lane-pin picker (P3-LN-10). The operator holds more than one
 * flat-rate lane and wants a head-to-head: the same goal pinned to lane A,
 * then to lane B, the receipts telling which lane ran. This control is that
 * pin's front door, and it is OPT-IN BY DESIGN: the ordinary task never asks
 * a person to think about lanes — the collapsed face states the default (the
 * platform chooses) and offers the pin in one small act.
 *
 * The honesty rules, in this form's own tradition:
 *  - choices are ENUMERATED from the running world; an unpinnable lane is
 *    never offered as a dead control (finding-5's rule — a control either
 *    works or is not rendered) but IS stated as a fact, with the platform's
 *    own refusal sentence verbatim, so "why can't I pin local?" is answered
 *    where the question arises;
 *  - a failed read degrades out loud and takes nothing with it: the unpinned
 *    default keeps working and the degraded face says exactly that;
 *  - an empty set is an honest absence with its meaning stated, never a
 *    control that silently vanished.
 */
export function LanePicker({
  world,
  lane,
  open,
  onOpen,
  onPick,
}: {
  world: LaneWorld
  lane: string
  open: boolean
  onOpen: () => void
  onPick: (lane: string) => void
}) {
  const lanes = world !== null && 'lanes' in world ? world.lanes : []
  const pinnable = lanes.filter((l) => l.pinnable)
  const unpinnable = lanes.filter((l) => !l.pinnable)
  const failed = world !== null && 'failed' in world ? world.failed : ''

  // Opening REPLACES the trigger with the chips, and a control that unmounts
  // under the keyboard drops focus to BODY (review F1 — a real Enter died
  // there). The hand-off: when the picker opens, focus lands on the standing
  // choice — the active chip — so the keyboard arrives ON its current answer.
  // There is no collapse act (the pin is one-way opt-in per form), so open is
  // the only transition to hand off; the why-disclosure below is a native
  // <details>, whose summary keeps focus across its own toggle.
  const chipsRef = useRef<HTMLDivElement | null>(null)
  const wasOpen = useRef(open)
  useEffect(() => {
    if (open && !wasOpen.current) {
      const c = chipsRef.current
      const target =
        c?.querySelector<HTMLButtonElement>('[data-lane-choice][data-active="true"]') ??
        c?.querySelector<HTMLButtonElement>('[data-lane-choice]')
      target?.focus()
    }
    wasOpen.current = open
  }, [open])

  return (
    <div className="door-field lane-pin" data-ask="lane" data-lane-open={open || undefined} data-lane={lane || undefined}>
      <span className="door-label">
        {/* The id wraps the QUESTION alone: it is the chip group's accessible
            name (review F5), and a name is a name — the optional-ness aside
            stays visible text without riding into the accName. */}
        <span id="lane-pin-label">Which lane runs this?</span>{' '}
        <span className="door-optional">optional — left alone, the platform chooses</span>
      </span>
      {!open ? (
        /* ONE bordered box, like every other value control on this form
           (review F3): the goal, the name and the project all sit in the
           .door-input rhythm, and the lane's answer-at-rest deserves the same
           body — the stated default inside it, the pin one small act away.
           The whole box is the trigger; the words stay quiet. */
        <button type="button" className="lane-closed" data-lane-open-act onClick={onOpen}>
          <span className="lane-closed-words">
            The platform picks the lane, favoring the one with the most room left.
          </span>
          <span className="lane-closed-act">Pin a lane for this task…</span>
        </button>
      ) : (
        <>
          <div className="lane-chips" role="group" aria-labelledby="lane-pin-label" ref={chipsRef}>
            <button
              type="button"
              className="q-option"
              data-lane-choice=""
              data-active={lane === ''}
              aria-pressed={lane === ''}
              onClick={() => {
                onPick('')
              }}
            >
              The platform chooses
            </button>
            {pinnable.map((l) => (
              <button
                key={l.lane}
                type="button"
                className="q-option mono"
                data-lane-choice={l.lane}
                data-active={lane === l.lane}
                aria-pressed={lane === l.lane}
                onClick={() => {
                  onPick(l.lane)
                }}
              >
                {l.lane}
              </button>
            ))}
          </div>
          {world === null && <p className="lane-aside">reading which lanes can be pinned…</p>}
          {failed !== '' && (
            <p className="lane-aside lane-degraded" role="status" data-lane-degraded>
              The pinnable lanes could not be read. {failed} Submitting unpinned still works — the platform
              chooses the lane, as it always does.
            </p>
          )}
          {world !== null && failed === '' && pinnable.length === 0 && (
            <p className="lane-aside" data-lane-empty>
              No lane on this platform can be pinned right now, so every task runs unpinned — the platform
              chooses.
            </p>
          )}
          {lane !== '' && (
            <p className="lane-pinned-note" data-lane-pinned={lane}>
              <Pin size={14} strokeWidth={1.8} aria-hidden="true" />
              <span>
                This task is pinned to lane <b className="mono">{lane}</b>: selection honors the pin instead of
                comparing what the lanes have consumed, and the run&apos;s receipt will name the lane that ran. A
                pin the platform cannot honor refuses the submission with its reason — it is never quietly
                rerouted.
              </span>
            </p>
          )}
          {/* The unpinnable FACT stays on the face — a lane that cannot be
              pinned is never a dead control and never a silence. The platform's
              whole sentence (the served one-spelling, VERBATIM — engineer-
              register, and ending in the same list the chips above already are)
              waits one quiet click behind it (review F4). */}
          {unpinnable.map((l) => (
            <details key={l.lane} className="lane-aside lane-unpinnable" data-lane-unpinnable={l.lane}>
              <summary>
                <b className="mono">{l.lane}</b> is not pinnable<span className="lane-why"> — why?</span>
              </summary>
              <p className="lane-unpinnable-words">{l.not_pinnable}</p>
            </details>
          ))}
        </>
      )}
    </div>
  )
}

/* ── the journey: card after card until approved ─────────────────────────── */

/** What each card kind asks of the person, in one line under the header.
 *  The interview line IS the up-front contract (design §2.F): what this is,
 *  that every question carries a one-click recommendation, and what skipping
 *  does — said before the first question, not discovered one screen late. */
const kindLine: Record<string, string> = {
  interview:
    'A few questions, each with a recommendation you can take in one click. Skip any of them: what you skip becomes a listed assumption, shown on the plan before anything runs or spends money.',
  clarification: 'Its draft still has open points only you can settle. The plan opens once these are answered.',
  escalation: 'One question it cannot proceed without.',
  'decision.coverage': 'The plan cannot cover everything you asked. Decide what happens to the part it cannot cover.',
  'decision.research': 'It is missing a fact it would otherwise have to research. Supply it, or send it back to planning.',
  'decision.spec_doubt': 'It is not sure it understood you correctly. Read its doubt and decide; this card is never skipped.',
  'decision.family':
    'One question before the interview: what KIND of work is this? The questions it asks next, and who does the work, follow from your answer.',
  approval: 'The plan. Nothing runs and nothing spends until you approve it.',
  'approval.delta': 'The plan changed after your approval. Exactly what changed is below; nothing else moved.',
}

function Journey({
  view,
  onView,
  answeredRounds = 0,
  stream,
}: {
  view: IntakeTaskView
  onView: (v: IntakeTaskView) => void
  /** Rounds this BROWSER already answered, read back from the ?r= stamp on a
   *  resume — so a reload mid-interview does not reset the face to the birth
   *  copy (review #7). */
  answeredRounds?: number
  stream?: EventStream
}) {
  const phase = view.phase ?? ''
  // How many answers this tab has folded in. The no-card face changes with
  // it: before the first answer the machine is reading the goal; after one,
  // it is composing the next round or the plan — different words, and the
  // "task is born" line after an answer read as a stall (live walk,
  // 2026-08-16). Seeded from the URL stamp on a resume; each fold restamps.
  const [beats, setBeats] = useState(answeredRounds)
  const beatsRef = useRef(answeredRounds)
  // The beat lands AT SEND, not at response: the answer request carries the
  // next round's compose and takes minutes, while the live approvals read
  // closes the card within seconds — so the card panel unmounts long before
  // the response resolves, and a response-time bump never ran (found live,
  // drain r1: the wait between rounds greeted the task as newborn — review
  // #7's second path). Sending IS this browser answering; the stamp rides the
  // address in the same breath (§41-B: the URL is the one pocket).
  const sent = () => {
    beatsRef.current += 1
    window.history.replaceState(
      null,
      '',
      `${hrefFor('new')}?task=${encodeURIComponent(view.task_id)}&r=${String(beatsRef.current)}`,
    )
    setBeats(beatsRef.current)
  }
  const fold = (v: IntakeTaskView) => {
    onView(v)
  }
  // The header's stakes chip and clearance meter read the OPEN CARD's own
  // figures. A cold resume's task read carries neither, and the live card
  // lived only inside FollowTask — so a resumed journey's header dropped
  // stakes and clearance at every width (review #20's mechanism). The card
  // the follow read finds is lifted here for the header to wear.
  const [liveCard, setLiveCard] = useState<IntakeCard | null>(null)

  // EVERY non-terminal state rides the live follow path (fixed 2026-08-11,
  // found on the seeded world): the pipeline keeps moving after a write
  // returns — planning can replace an interview card with a clarification
  // card seconds later — and a response snapshot rendered as a STANDING card
  // showed questions the platform had already withdrawn. The snapshot now
  // only SEEDS the panel (see FollowTask); the caller's own live read is the
  // one truth for which card is open.
  return (
    <div className="door-journey" data-task={view.task_id} data-beats={String(beats)}>
      <JourneyHead view={liveCard !== null && view.open_card === undefined ? { ...view, open_card: liveCard } : view} />
      {phase === 'approved' ? (
        <Landed view={view} />
      ) : phase === 'cancelled' ? (
        <Cancelled view={view} />
      ) : (
        <FollowTask view={view} onView={fold} onSent={sent} onCard={setLiveCard} answered={beats > 0} stream={stream} />
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
  onSent,
  onCard,
  answered = false,
  stream,
}: {
  view: IntakeTaskView
  onView: (v: IntakeTaskView) => void
  /** Fired the moment an answer LEAVES this tab (see Journey.sent). */
  onSent?: () => void
  /** Hands the live card up so the journey header can wear its stakes and
   *  clearance (review #20). */
  onCard?: (c: IntakeCard | null) => void
  answered?: boolean
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
  const cardKey = item !== undefined && card !== undefined ? `${item.id}:${String(card.version ?? 0)}` : ''
  useEffect(() => {
    onCard?.(card ?? null)
    // Keyed on the ask identity, not the object: the read re-serves equal
    // snapshots and an object-keyed effect would loop the parent's state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cardKey])

  // W1-11: remember which card was LAST on screen, so a replacement can say
  // so instead of silently dropping what was typed for the old one. A
  // replacement that follows this page's OWN send is the journey moving
  // forward — expected, not a re-issue — so the own-send flag suppresses the
  // notice for exactly one transition.
  const shownKey = useRef('')
  const ownSend = useRef(false)
  const reissued = shownKey.current !== '' && cardKey !== '' && shownKey.current !== cardKey && !ownSend.current
  useEffect(() => {
    if (cardKey !== '') {
      shownKey.current = cardKey
      ownSend.current = false
    }
  }, [cardKey])
  const sentHere = () => {
    ownSend.current = true
    onSent?.()
  }

  if (item !== undefined && card !== undefined) {
    const askID = item.id.startsWith('ask:') ? item.id.slice(4) : item.id
    return (
      <CardPanel
        key={`${askID}:${String(card.version ?? 0)}`}
        view={{ ...view, open_card: card }}
        card={card}
        askID={askID}
        reissued={reissued}
        onView={onView}
        onSent={sentHere}
      />
    )
  }
  // The response snapshot SEEDS the panel until the first live read answers:
  // the answer verb hands the next card back synchronously, and a round-trip
  // of nothing before showing what is already in hand would be a lie of its
  // own. The live read above stays the standing truth — when it lands with
  // the same card, the key matches and nothing remounts; when the pipeline
  // has already moved on, the fresh card replaces the withdrawn one.
  if (asks.data === null && view.open_card !== undefined && view.open_ask_id !== undefined && view.open_ask_id !== '') {
    return (
      <CardPanel
        key={`${view.open_ask_id}:${String(view.open_card.version ?? 0)}`}
        view={view}
        card={view.open_card}
        askID={view.open_ask_id}
        onView={onView}
        onSent={sentHere}
      />
    )
  }
  return <NoCardYet view={view} waiting={asks.data !== null} answered={answered} />
}

/** The journey's standing header: whose task, where it stands, the clearance
 *  meter. The clearance is the S06.5 measure the platform served — the meter
 *  renders it, it never recomputes it. */
function JourneyHead({ view }: { view: IntakeTaskView }) {
  const clearance = view.open_card?.clearance ?? view.clearance
  // A cold resume's task read carries no tier; the open card does. Either
  // source is the platform's own figure — never derived here.
  const tier = view.tier !== undefined && view.tier !== '' ? view.tier : view.open_card?.tier
  // RA-6: the walk saw stakes flip HIGH→LOW→HIGH with no reason given. The
  // wire serves no per-flip cause (the platform records floor reasons but
  // never serves them — a REPORTED gap), so the honest view-side story is the
  // mechanism itself: this page REMEMBERS the level it last showed for this
  // task, and when the level moves it says so and says why levels move at all.
  const seenTier = useRef<{ task: string; tier: string; moved: string }>({ task: '', tier: '', moved: '' })
  let moved = seenTier.current.task === view.task_id ? seenTier.current.moved : ''
  if (tier !== undefined && tier !== '') {
    if (seenTier.current.task === view.task_id && seenTier.current.tier !== tier && seenTier.current.tier !== '') {
      moved = seenTier.current.tier
    }
    seenTier.current = { task: view.task_id, tier, moved }
  }
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
        {tier !== undefined && tier !== '' && (
          // The badge carries its why AT FIRST SIGHT (W1-4): what stakes are,
          // what this level changes — not a bare token explained by a footnote
          // twenty-five minutes later. The specific cause is the platform's to
          // state (it arrives with the plan card's own notes); what the chip
          // can honestly say now is what the level MEANS.
          <span className="journey-tier" title={stakesWords(tier)}>
            <Chip tone={tier === 'high' ? 'red' : tier === 'medium' ? 'orange' : 'blue'}>stakes: {tier}</Chip>
            {/* GF1-W3: the chip keeps the served tier truth; the consequence
                stands BESIDE it in calm words instead of shouting inside the
                red — "EXTRA CARE, PIN TO APPROVE" terrified a birthday-dinner
                project. High stakes mean more care, and that is good news. */}
            {tier === 'high' && (
              <span className="muted text-xs" data-stakes-why>
                {' '}
                handled with extra care — approving the plan asks for your PIN
              </span>
            )}
            {moved !== '' && moved !== tier && (
              <span className="muted text-xs" data-stakes-moved={moved}>
                {' '}
                was {moved} — the platform refined its reading of the goal as it learned more
              </span>
            )}
          </span>
        )}
        {/* The meter shows only WHILE THE PLATFORM IS ASKING (W1-9): clearance
            measures how settled the must-knows are, so before the first
            question and after the questions end it measures nothing a reader
            can act on — at the plan stage the card's own open markers speak,
            and a full meter beside them read as a contradiction. The floor is
            the card's own served figure (P3-GF3-BE1 R11), never derived. */}
        {clearance !== undefined && (view.open_card?.questions ?? []).length > 0 && (
          <ClearanceMeter value={clearance} floor={view.open_card?.clearance_floor} />
        )}
      </div>
    </header>
  )
}

/** The stakes chip's plain-words why (W1-4): what the level MEANS, said where
 *  the chip is. The platform sets the level from the goal; an unknown served
 *  value gets the general words only. */
function stakesWords(tier: string): string {
  const what = 'Stakes — how much care this goal gets before anything runs. The platform set this from your description.'
  if (tier === 'high')
    return `${what} High stakes bring stricter questions, and approving the plan asks for your PIN. The plan card states what made it high.`
  if (tier === 'medium') return `${what} Medium stakes bring the standard questions and a plain approval.`
  if (tier === 'low') return `${what} Low stakes keep the ceremony light — trivial read-only work can skip it entirely.`
  return what
}

/** The served S06.5 clearance: how settled the must-knows are, on the
 *  platform's own 0–100 scale, with the tier's served stopping point beside
 *  it (P3-GF3-BE1 R11) — the meter that explains itself (design §2.F): what
 *  it measures, where the questions stop, and what happens to the rest. Both
 *  figures print as served; only the fill width is scaled onto the track. */
export function ClearanceMeter({ value, floor }: { value: number; floor?: number }) {
  const ratio = Math.max(0, Math.min(1, value > 1 ? value / 100 : value))
  // Display precision, not a different fact: the platform serves the measure
  // at float precision ("12.121212…"), and a meter is read at whole points.
  // The exact served values stay on data-clearance / data-clearance-floor.
  const shown = Math.round(value > 1 ? value : value * 100)
  const stopAt = floor !== undefined && floor > 0 ? Math.round(floor > 1 ? floor : floor * 100) : undefined
  const stopRatio = stopAt !== undefined ? Math.max(0, Math.min(1, stopAt / 100)) : undefined
  return (
    <div
      className="clearance"
      title={
        stopAt !== undefined
          ? `How settled the must-knows are, from 0 to 100. Answering raises it; a skip settles its point as a listed assumption. The questions stop once it reaches ${String(stopAt)} for work like this; whatever is still open then becomes listed assumptions on the plan.`
          : 'How settled the must-knows are, from 0 to 100. Answering raises it. The questions stop once enough is settled for work like this; whatever is still open then becomes listed assumptions on the plan.'
      }
      data-clearance={String(value)}
      data-clearance-floor={floor !== undefined ? String(floor) : undefined}
    >
      <span className="clearance-label">Must-knows</span>
      <span className="clearance-track" aria-hidden="true">
        <span className="clearance-fill" style={{ width: `${String(ratio * 100)}%` }} />
        {stopRatio !== undefined && <i className="clearance-stop" style={{ insetInlineStart: `${String(stopRatio * 100)}%` }} />}
      </span>
      <b className="mono">
        {String(shown)}
        <span className="clearance-unit">
          {stopAt !== undefined ? ` of the ${String(stopAt)} needed` : '/100 settled'}
        </span>
      </b>
    </div>
  )
}

/* ── the understanding block (P3-RW-12 R8/R9) ────────────────────────────── */

/** The origin labels, in the reader's words. The four values are the card
 *  vocabulary (intake/cards.go); anything new renders as itself (§42). */
function howWords(how: string): string {
  switch (how) {
    case 'registry':
      return 'from the project record'
    case 'answered':
      return 'you answered'
    case 'assumption':
      return 'assumed — out loud'
    case 'escalation':
      return 'you answered its question'
    default:
      return how
  }
}

/**
 * The platform's per-skipped-slot assumption prose is a TEMPLATE
 * (intake/pipeline.go): "<Name> — you asked me to go ahead without answering,
 * so I assumed a sensible default." / "…small enough to run without an
 * interview…". Rendered raw it doubled the slot's name and said nothing about
 * WHAT was assumed — nine near-identical rows walled the plan card (design
 * review 2026-08-17, blocker #15). These helpers recognize the template so the
 * views can collapse it and substitute the planner's ACTUAL assumed value,
 * which rides the same card in `layer1.assumptions` under `assumption:<slots>`
 * origins. Unrecognized prose renders as itself — nothing is guessed.
 */
export function assumptionRemainder(name: string, text: string): string {
  return text.startsWith(`${name} — `) ? text.slice(name.length + 3) : text
}

/** Exported for TaskDetail's spec render path (re-walk B): the same template
 *  walls the stored spec's assumption list, and the same recognition collapses
 *  it there. */
export function isAssumedDefaultBoilerplate(name: string, text: string): boolean {
  return /so I assumed a sensible default\.?$/.test(assumptionRemainder(name, text))
}

/** The slots an origin tag names: `assumption:a,b` → [a,b]; `slot:a` → [a];
 *  anything else → []. Reading, not inventing — the tags are the wire's. */
function originSlots(origin: string): string[] {
  const m = /^(assumption|slot|answered):(.+)$/.exec(origin)
  if (m === null) return []
  return m[2]
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '')
}

/** A slot id in reader's words: `comparison_rules` → "comparison rules".
 *  Raw slot ids never surface (blocker #15). */
function humanSlot(id: string): string {
  return id.replace(/[_-]+/g, ' ').trim()
}

/** An answer VALUE in reader's words. An option answer is recorded under the
 *  card's machine value (`keep_both_flag`), and the option list that carried
 *  its label is not on the approval card — so a value that is shaped like a
 *  machine token renders with its separators spaced (the humanSlot precedent,
 *  blocker #15; found on pixels, GF3 walk). Real prose passes through
 *  untouched, and the transformation never changes the words themselves. */
export function humanValue(v: string): string {
  return /^[a-z0-9]+(?:[_-][a-z0-9]+)+$/.test(v) ? v.replace(/[_-]+/g, ' ') : v
}

/**
 * The plan card's substantive assumed values, keyed by the slot each covers:
 * parsed from the planner's own assumption rows whose origin is
 * `assumption:<slot,…>`. This is the resolution the wire carries — the actual
 * "sensible default", stated by the planner.
 */
function slotResolutions(assumptions: { text: string; origin?: string }[]): Map<string, string> {
  const out = new Map<string, string>()
  for (const a of assumptions) {
    if (a.origin === undefined || !a.origin.startsWith('assumption:')) continue
    for (const slot of originSlots(a.origin)) {
      if (!out.has(slot)) out.set(slot, a.text)
    }
  }
  return out
}

/**
 * "Here is what I understood so far" — the per-round recap on interview and
 * clarification cards, and the platform's slot-by-slot record beside the
 * planner's restatement on the plan card. Items are the platform's own
 * deterministic record; `text` is the optional utility-phrased prose and
 * renders as the lead when present. An absent block renders nothing — the
 * first round has nothing to recap, and that is not a defect.
 *
 * Assumed slots render their ACTUAL assumed value wherever the card carries
 * one (`resolutions`, from the plan's own assumption rows); template rows with
 * no stated value collapse into ONE line naming the skipped points — never a
 * wall of "assumed a sensible default" (blocker #15).
 */
export function UnderstoodPanel({
  understood,
  heading,
  resolutions,
}: {
  understood?: IntakeUnderstood
  heading: string
  resolutions?: Map<string, string>
}) {
  if (understood === undefined) return null
  const items = understood.items ?? []
  const text = understood.text ?? ''
  if (items.length === 0 && text === '') return null

  // Rows in card order; boilerplate-assumed slots with a resolution GROUP by
  // that resolution (one sentence can cover five slots — say it once), and
  // ones without any stated value collect into the single skipped line.
  type Row = { key: string; names: string[]; slots: string[]; value: string; how: string }
  const rows: Row[] = []
  const byValue = new Map<string, Row>()
  const skipped: { name: string; slot: string }[] = []
  for (const it of items) {
    const stated =
      it.how === 'assumption' && it.assumption !== undefined && it.assumption !== ''
        ? it.assumption
        : it.value !== undefined && it.value !== ''
          ? it.value
          : ''
    if (it.how === 'assumption' && isAssumedDefaultBoilerplate(it.name, stated)) {
      const resolved = resolutions?.get(it.slot_id)
      if (resolved !== undefined && resolved !== '') {
        const have = byValue.get(resolved)
        if (have !== undefined) {
          have.names.push(it.name)
          have.slots.push(it.slot_id)
        } else {
          const row = { key: `${it.slot_id}:${it.how}`, names: [it.name], slots: [it.slot_id], value: resolved, how: it.how }
          byValue.set(resolved, row)
          rows.push(row)
        }
      } else {
        skipped.push({ name: it.name, slot: it.slot_id })
      }
      continue
    }
    rows.push({
      key: `${it.slot_id}:${it.how}`,
      names: [it.name],
      slots: [it.slot_id],
      value: stated === '' ? '—' : humanValue(assumptionRemainder(it.name, stated)),
      how: it.how,
    })
  }

  return (
    <div className="understood" data-understood>
      <p className="understood-head">{heading}</p>
      {text !== '' && <p className="understood-text">{text}</p>}
      {(rows.length > 0 || skipped.length > 0) && (
        <ul className="understood-items">
          {rows.map((r) => (
            <li key={r.key} data-slot={r.slots.join(',')} data-how={r.how}>
              <span className="understood-name">{r.names.join(' · ')}</span>
              <span className="understood-value">{r.value}</span>
              <span className="understood-how">{howWords(r.how)}</span>
            </li>
          ))}
          {skipped.length > 0 && (
            // HONEST COUNTING (W1-6): these are the platform's checklist
            // POINTS now covered by defaults — skipping one question can fan
            // out into several of them, so "N points you skipped" blamed the
            // reader for a count they never saw. The count names what it
            // counts, and the fan-out is said out loud.
            <li data-how="assumption" data-skipped={skipped.map((s) => s.slot).join(',')}>
              <span className="understood-name">
                {skipped.length === 1
                  ? 'One point runs on a default'
                  : `${String(skipped.length)} points run on defaults`}
              </span>
              <span className="understood-value">
                {skipped.map((s) => s.name).join(' · ')} — these are the checklist points behind what was left
                unanswered (one skipped question can cover several);{' '}
                {resolutions !== undefined
                  ? 'each one is listed under Assumptions below, where you can still contest it'
                  : 'each one becomes a listed assumption on the plan card, where you can still contest it'}
                .
              </span>
              <span className="understood-how">{howWords('assumption')}</span>
            </li>
          )}
        </ul>
      )}
    </div>
  )
}

/* ── the composing face (never-stall-silently; findings 2026-08-16 item 4) ── */

/** Elapsed-time words for the face's own clock — a display derivation of the
 *  moment THIS page sent the request, nothing more. */
function elapsedWords(s: number): string {
  if (s < 60) return `${String(s)} s`
  const m = Math.floor(s / 60)
  return `${String(m)} min ${String(s % 60)} s`
}

/**
 * The face's clock: real elapsed time from a start instant, re-rendered on
 * animation frames. DELIBERATELY NOT setInterval — the app tree carries a
 * standing no-timer scan (S15.12 live-by-default: nothing may poll on a
 * clock), and this hook honors the invariant's letter and spirit: it touches
 * no network, computes from one timestamp, pauses with the hidden tab, and
 * shows the true elapsed the moment frames resume.
 */
function useElapsedSeconds(active: boolean): number {
  const [seconds, setSeconds] = useState(0)
  useEffect(() => {
    if (!active) return
    const started = Date.now()
    let frame = 0
    const tick = () => {
      const s = Math.floor((Date.now() - started) / 1000)
      setSeconds((prev) => (prev === s ? prev : s))
      frame = window.requestAnimationFrame(tick)
    }
    frame = window.requestAnimationFrame(tick)
    return () => {
      window.cancelAnimationFrame(frame)
    }
  }, [active])
  return seconds
}

/**
 * The honest in-place progress face for the two long waits the pipeline
 * actually has (both measured on this machine, 2026-08-16): an interview
 * round's phrased card takes about a minute to build on the local model, and
 * the plan draft composes synchronously in one piece — minutes. The answer
 * request carries the work, so the wait is real; this face says what is
 * happening, counts the time it has itself been waiting, and tells the truth
 * about leaving: the card lands in the Inbox and the page resumes on its own.
 * No spinner stands alone, nothing pretends to know a percentage.
 */
function ComposingFace({ mode }: { mode: 'answer' | 'approve' }) {
  const seconds = useElapsedSeconds(true)
  const long = seconds >= 90
  return (
    <div className="composing" role="status" data-composing={mode}>
      <span className="composing-dots" aria-hidden="true">
        <i /><i /><i />
      </span>
      <div className="composing-body">
        <p className="composing-head">
          {mode === 'approve' ? 'Approved — it is starting the work' : 'Answers recorded — it is working on what comes next'}
        </p>
        <p className="composing-sub">
          {mode === 'approve'
            ? 'The approval is being recorded and the work handed to its worker. This usually takes a few seconds.'
            : // TODAY'S TRUTH (PH-1, 2026-08-17): the phrase seat has never
              // answered live — questions arrive in standard wording and the
              // page admits it per round — so this face does not claim a
              // "phrasing" step it cannot show. The choosing is real.
              'It is choosing the next questions, or drafting the plan. A question round takes about a minute on the local models; the full plan is drafted in one piece and can take a few minutes.'}
        </p>
        <p className="composing-clock mono">working · {elapsedWords(seconds)}</p>
        {mode === 'answer' && long && (
          <p className="composing-sub">
            Still at it — a full plan draft is the long step, and it arrives whole. You can leave: the card lands in
            your <Link to={hrefFor('inbox')}>Inbox</Link> and this page resumes by itself when you come back.
          </p>
        )}
      </div>
    </div>
  )
}

/* ── the card panel: one component per card kind ─────────────────────────── */

function CardPanel({
  view,
  card,
  askID,
  reissued,
  onView,
  onSent,
}: {
  view: IntakeTaskView
  card: IntakeCard
  askID: string
  /** True when this card REPLACED a different card/version on this screen —
   *  the remount that (correctly) drops unsent answers must say so out loud
   *  (W1-11: a typed answer vanished in silence). */
  reissued?: boolean
  onView: (v: IntakeTaskView) => void
  onSent?: () => void
}) {
  const [busy, setBusy] = useState(false)
  const [refusal, setRefusal] = useState('')
  const [needPin, setNeedPin] = useState(false)
  const [pin, setPin] = useState('')
  // The answer the step-up is holding: stashed when the platform said
  // `pin_required`, sent again by the armed panel's own confirm. Holding it
  // here is what lets the panel sit NEXT TO the person's attention instead of
  // needing the original button found again two screenfuls up (blocker #4).
  const [pending, setPending] = useState<IntakeAnswerBody | null>(null)
  // What the held answer IS, in the person's words — an armed panel that
  // doesn't say whether it holds an Approve or a Cancel invites a mis-fire.
  const pendingWords =
    pending !== null && 'action' in pending && typeof pending.action === 'string'
      ? (planActionLabels[pending.action] ?? pending.action)
      : 'your answer'
  const stepupRef = useRef<HTMLDivElement | null>(null)

  // The armed step-up must be SEEN to be armed: the card above it is long
  // (a full plan), and the field used to render off-screen below it — the
  // viewport stayed at the card top and nothing visibly happened (blocker #4).
  useEffect(() => {
    if (!needPin) return
    stepupRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    stepupRef.current?.querySelector('input')?.focus()
  }, [needPin])

  // ONE answer path for every card kind: send, fold the returned view in, and
  // render any refusal in the server's own words. `pin_required` arms the
  // step-up panel instead of failing — the S01.9 step-up rides the same request.
  const answer = (body: IntakeAnswerBody) => {
    if (busy) return
    setBusy(true)
    setRefusal('')
    onSent?.()
    api.answerAsk(askID, { answer: body, ...(pin !== '' ? { pin } : {}) }).then(
      (v) => {
        setBusy(false)
        setNeedPin(false)
        setPin('')
        setPending(null)
        onView(v)
      },
      (err: unknown) => {
        setBusy(false)
        if (err instanceof ApiError && err.code === 'pin_required') {
          setNeedPin(true)
          setPending(body)
          setRefusal('')
          return
        }
        if (err instanceof ApiError && err.code === 'pin_rejected') {
          setNeedPin(true)
          setPending(body)
          setRefusal('The PIN was not accepted. Try again.')
          return
        }
        if (err instanceof Unreachable) {
          // The long compose rides this very request, so a dropped connection
          // mid-draft is a real path. The truth (S02.3 R4): an answer that
          // landed resumes the run server-side, and a drive that died past
          // the resume is re-driven by the recovery ladder within a sweep —
          // machine-only. The live follow below picks the result up either
          // way; only an answer that never reached the platform needs
          // re-sending, and the still-open card is exactly that signal.
          setRefusal(
            'The connection was lost while it was working. If your answer reached the platform, the work continues on its own — a draft broken mid-write is healed automatically within a few minutes, and the next card appears right here and in your Inbox without anything from you. If this same card is still open in a few minutes, the answer never landed: send it again.',
          )
          return
        }
        setRefusal(describeError(err))
      },
    )
  }

  const kind = card.kind
  // A kind this file has never heard of still renders a WORKING form by its
  // BODY SHAPE (operator finding F3, 2026-08-16: the "answer it from its
  // inbox card" placeholder read as a bug): a card carries exactly one of the
  // four answerable bodies, and each body has a form here. Only a card with
  // no recognizable body falls back to its inbox pointer — that one this page
  // genuinely cannot compose an answer for.
  const known = kind in kindLine
  const fallbackForm = !known
    ? card.approval !== undefined
      ? 'approval'
      : card.delta !== undefined
        ? 'delta'
        : card.decision !== undefined
          ? 'decision'
          : (card.questions ?? []).length > 0
            ? 'questions'
            : 'none'
    : 'none'
  // While the answer request is in flight the FORM is done saying anything —
  // the wait is the story now (a phrased round takes ~a minute, a plan draft
  // minutes, and both ride this very request), so the composing face replaces
  // the dead controls instead of dimming them.
  const composing = busy && (kind === 'approval' || kind === 'approval.delta' ? 'approve' : 'answer')
  return (
    <div className="door-card" data-card-kind={kind}>
      {reissued === true && (
        // W1-11: the remount that replaces a re-issued card CORRECTLY drops
        // unsent answers for the old card — but doing it silently read as the
        // page eating what was typed. The drop says so, where it happened.
        <p className="card-reissued" role="alert" data-card-reissued>
          This card was re-issued while you were here. Anything typed and not yet sent belonged to the previous
          card and did not carry over — read the card as it now stands before answering.
        </p>
      )}
      <p className="card-kind-line">{kindLine[kind] ?? `A ${kind} card — what it asks of you is below.`}</p>
      {composing !== false ? (
        <ComposingFace mode={composing} />
      ) : (
        <>
          {kind === 'interview' &&
            // The S06.9 review card ("Change my answers") is the same interview
            // kind carrying per-question resolutions — its presence is the
            // signal to render review-and-adjust instead of asking blind
            // (P3-GF3-BE1 R8; design §2.D).
            ((card.questions ?? []).some((q) => q.resolution !== undefined) ? (
              <ReviewForm card={card} busy={busy} onAnswer={answer} />
            ) : (
              <InterviewForm card={card} busy={busy} onAnswer={answer} />
            ))}
          {kind === 'clarification' && <QuestionForm card={card} busy={busy} onAnswer={answer} />}
          {kind === 'escalation' && <EscalationForm card={card} busy={busy} onAnswer={answer} />}
          {(kind === 'decision.coverage' || kind === 'decision.research' || kind === 'decision.spec_doubt') && (
            <DecisionForm card={card} busy={busy} onAnswer={answer} />
          )}
          {kind === 'decision.family' && <FamilyForm card={card} busy={busy} onAnswer={answer} />}
          {kind === 'approval' && <PlanCard view={view} card={card} busy={busy} onAnswer={answer} />}
          {kind === 'approval.delta' && <DeltaForm card={card} busy={busy} onAnswer={answer} />}
          {fallbackForm === 'approval' && <PlanCard view={view} card={card} busy={busy} onAnswer={answer} />}
          {fallbackForm === 'delta' && <DeltaForm card={card} busy={busy} onAnswer={answer} />}
          {fallbackForm === 'decision' && <DecisionForm card={card} busy={busy} onAnswer={answer} />}
          {fallbackForm === 'questions' && <QuestionForm card={card} busy={busy} onAnswer={answer} />}
          {fallbackForm === 'none' && !known && (
            <p className="muted">
              This card carries a shape this page cannot answer.{' '}
              <Link to={hrefFor('inbox-item', { id: `ask:${askID}` })}>Its inbox card</Link> shows everything it holds —
              answering there resumes the journey right here.
            </p>
          )}
        </>
      )}

      {needPin && (
        <div className="door-stepup" ref={stepupRef} role="alert" data-stepup="armed">
          <p className="stepup-head">
            <CircleAlert size={15} strokeWidth={2} aria-hidden="true" /> Armed: &quot;{pendingWords}&quot; — your PIN
            confirms it
          </p>
          <p className="stepup-sub">
            This is a High-stakes act, so the platform asks you to prove it is you in the same breath. Nothing has
            happened yet: entering your PIN and confirming is what makes it real, and the PIN rides that one request —
            it is never stored.
          </p>
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
          <div className="door-acts">
            <Button
              variant="primary"
              disabled={pin === '' || busy || pending === null}
              aria-busy={busy}
              data-stepup-confirm
              onClick={() => {
                if (pending !== null) answer(pending)
              }}
            >
              {busy ? 'Confirming…' : 'Confirm with PIN'}
            </Button>
            <Button
              variant="ghost"
              disabled={busy}
              data-stepup-disarm
              onClick={() => {
                setNeedPin(false)
                setPin('')
                setPending(null)
                setRefusal('')
              }}
            >
              Never mind — stand down
            </Button>
            {pin === '' && !busy && <span className="door-why">enter your PIN to confirm, or stand down and nothing happens</span>}
          </div>
        </div>
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
 *  says what it does. The inbox note keeps the tab-lifetime state honest.
 *  P3-RW-19: pressing it opens the two-step — the optional one-line why rides
 *  the cancel verb as its `reason`, under the same 280 bound the verb applies
 *  (over it the act is HELD with the bound said; nothing is ever truncated). */
function JourneyFoot({ view, onView, busy }: { view: IntakeTaskView; onView: (v: IntakeTaskView) => void; busy: boolean }) {
  const [cancelBusy, setCancelBusy] = useState(false)
  const [note, setNote] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [why, setWhy] = useState('')
  const overLine = whyOverLine(why)
  return (
    <footer className="journey-foot">
      <span className="foot-note">
        This card also sits in your <Link to={hrefFor('inbox')}>Inbox</Link> — leaving this page loses nothing.
      </span>
      {!confirming ? (
        <Button
          variant="ghost"
          size="sm"
          disabled={busy || cancelBusy}
          data-door-cancel
          onClick={() => {
            setNote('')
            setConfirming(true)
          }}
        >
          Cancel this task
        </Button>
      ) : (
        <div className="foot-cancel" data-door-cancel-confirm>
          <p className="m-0 text-sm">
            Cancel this task? It stops here — nothing more is asked, started or spent, and the task keeps its record,
            marked cancelled.
          </p>
          <label className="door-field">
            <span className="door-label">
              Why cancel it <span className="door-optional">optional — one line, kept with the record of what was stopped</span>
            </span>
            <input
              className="door-input"
              type="text"
              data-field="cancel-why"
              value={why}
              onChange={(e) => {
                setWhy(e.target.value)
              }}
            />
          </label>
          {overLine !== null && (
            <span className="warn-flag" data-why-over="true">
              {overLine}
            </span>
          )}
          <div className="door-acts">
            <Button
              variant="danger"
              size="sm"
              disabled={busy || cancelBusy || whyHolds(why)}
              aria-busy={cancelBusy}
              data-door-cancel-fire
              onClick={() => {
                setCancelBusy(true)
                setNote('')
                api.cancelTask(view.task_id, why.trim() === '' ? undefined : why).then(
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
              {cancelBusy ? 'Cancelling…' : 'Cancel the task'}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={cancelBusy}
              data-door-cancel-back
              onClick={() => {
                setConfirming(false)
              }}
            >
              Keep going
            </Button>
          </div>
        </div>
      )}
      {note !== '' && <span className="foot-note">{note}</span>}
    </footer>
  )
}

/* ── the guided interview (P3-GF3-FE; design §2.F; S06.5) ────────────────── */

/**
 * What one question has been given on this card, in the card's own
 * vocabulary: an answer value, or the per-question skip — S06.5's
 * convert-to-assumption arm, riding the wire's `{id, skip: true}`
 * (P3-GF3-BE1 R6).
 */
type Given = { value: string } | { skip: true }

/** A value's words: the served option label when the value names one of the
 *  question's own options, the value itself otherwise (free text is already
 *  the person's own words). */
function optionLabel(q: IntakeQuestion, value: string): string {
  return (q.options ?? []).find((o) => o.value === value)?.label ?? value
}

/** The phrased wording when the utility seat answered, the canonical taxonomy
 *  text otherwise (P3-RW-12 R6) — the producer's own display rule. The
 *  clarification marker's internal token never surfaces (#17). */
function questionWording(q: IntakeQuestion): string {
  return (q.phrased !== undefined && q.phrased !== '' ? q.phrased : q.text).replace(/^NEEDS-CLARIFICATION:\s*/, '')
}

/**
 * The interview as a GUIDED SEQUENCE (design §2.F; GOV.UK one-decision-in-view,
 * chosen over the grouped form because the grouped form is what a non-IT
 * operator bounced off three times: F1/F3/F4).
 *
 * One question is open at a time; the others stay on screen as compact rows
 * wearing their state — answered ✓ with the answer, skipped with what the skip
 * means, open with "not yet" (the Nexus ✓-dimming idea, kept navigable: any
 * row reopens on tap). Every question offers the served recommendation as a
 * one-click take, its option chips with the recommended one starred, free text
 * always beside them, and a per-question skip that takes the recommendation as
 * a LISTED assumption. One send per card, armed when every question is either
 * answered or skipped; the stop-the-questions arm below is the whole-interview
 * escape hatch (force-proceed), unchanged in meaning.
 */
export function InterviewForm({
  card,
  busy,
  onAnswer,
}: {
  card: IntakeCard
  busy: boolean
  onAnswer: (b: IntakeAnswerBody) => void
}) {
  const questions = card.questions ?? []
  // When the WHOLE round is stock (the phrasing seat did not answer at all),
  // one line says it once instead of stamping every question (the #15 lesson);
  // a single unphrased question inside a phrased round keeps its own marker.
  const allStock = questions.every((q) => q.phrased === undefined || q.phrased === '')
  const [given, setGiven] = useState<Record<string, Given>>({})
  const [cursor, setCursor] = useState(0)

  const openCount = questions.filter((q) => given[q.id] === undefined).length
  const settled = questions.length - openCount
  const answeredCount = questions.filter((q) => {
    const g = given[q.id]
    return g !== undefined && 'value' in g
  }).length
  const skippedCount = settled - answeredCount
  const allSettled = openCount === 0

  // Settle question i and move to the next open one (wrapping), or to none:
  // when everything is settled the send row is the story.
  const settle = (i: number, g: Given) => {
    const next = { ...given, [questions[i].id]: g }
    setGiven(next)
    for (let step = 1; step <= questions.length; step++) {
      const j = (i + step) % questions.length
      if (next[questions[j].id] === undefined) {
        setCursor(j)
        return
      }
    }
    setCursor(-1)
  }

  const send = (force: boolean) => {
    const entries = questions
      .filter((q) => given[q.id] !== undefined)
      .map((q) => {
        const g = given[q.id]
        return 'skip' in g ? { id: q.id, skip: true } : { id: q.id, value: g.value }
      })
    onAnswer({ answers: entries, ...(force ? { force_proceed: true } : {}) })
  }

  return (
    <div className="q-form" data-interview-guided data-open-count={String(openCount)}>
      <UnderstoodPanel understood={card.understood} heading="What it understood so far" />
      {allStock && questions.length > 1 && (
        <p className="q-stock" data-phrasing="canonical-round">
          standard wording throughout: this round was not rephrased for your goal
        </p>
      )}
      <ol className="q-seq">
        {questions.map((q, i) => (
          <li key={q.id}>
            {i === cursor ? (
              <QuestionEditor
                q={q}
                head={`${String(i + 1)} of ${String(questions.length)}`}
                marker={!allStock}
                initial={given[q.id]}
                busy={busy}
                saveLabel="Save and continue"
                skip
                onSave={(v) => {
                  settle(i, { value: v })
                }}
                onSkip={() => {
                  settle(i, { skip: true })
                }}
              />
            ) : (
              <SettledRow
                q={q}
                n={i + 1}
                state={given[q.id]}
                onOpen={() => {
                  setCursor(i)
                }}
              />
            )}
          </li>
        ))}
      </ol>

      <div className="door-acts">
        <Button
          variant="primary"
          disabled={!allSettled || busy}
          aria-busy={busy}
          data-interview="send"
          onClick={() => {
            send(false)
          }}
        >
          {busy
            ? 'Sending…'
            : skippedCount > 0
              ? `Send: ${String(answeredCount)} answered, ${String(skippedCount)} skipped`
              : 'Send my answers'}
        </Button>
        {!allSettled && !busy && (
          <span className="door-why">
            {settled === 0
              ? 'go through the questions above: pick, type, or skip each one'
              : `${String(openCount)} to go: answer or skip each question above`}
          </span>
        )}
        {allSettled && !busy && (
          <span className="door-why">
            skipped questions become listed assumptions on the plan, still yours to change there
          </span>
        )}
      </div>

      <div className="door-acts q-escape">
        {/* The whole-interview escape hatch (S06.5 force-proceed, untouched in
            meaning). The button says what rides with it (W1-10) and what
            happens to everything else — one screen BEFORE the assumptions
            appear, not one after. */}
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          data-interview="force"
          onClick={() => {
            send(true)
          }}
        >
          {settled > 0
            ? 'Stop the questions: send what I settled, assume the rest'
            : 'Stop the questions: go straight to the plan'}
        </Button>
        <span className="door-why">
          every question left open, this round and later ones, becomes a listed assumption you will see on the plan
          before anything runs or spends
        </span>
      </div>
    </div>
  )
}

/** One settled-or-waiting question as a compact row: its state mark, its
 *  wording, what it was given, and the reopen affordance. The whole row is
 *  the button (the Nexus dimmed-card idea, kept navigable). */
function SettledRow({ q, n, state, onOpen }: { q: IntakeQuestion; n: number; state?: Given; onOpen: () => void }) {
  const answered = state !== undefined && 'value' in state
  const skipped = state !== undefined && 'skip' in state
  return (
    <button
      type="button"
      className="q-row"
      data-question-row={q.id}
      data-state={answered ? 'answered' : skipped ? 'skipped' : 'open'}
      onClick={onOpen}
    >
      <span className="q-row-mark" aria-hidden="true">
        {answered ? <Check size={13} strokeWidth={2.5} /> : skipped ? <CornerDownRight size={13} strokeWidth={2} /> : String(n)}
      </span>
      <span className="q-row-q">{questionWording(q)}</span>
      <span className="q-row-a">
        {answered
          ? optionLabel(q, state.value)
          : skipped
            ? q.suggested !== undefined && q.suggested !== ''
              ? 'skipped: goes with the recommendation'
              : 'skipped: it chooses and lists its choice'
            : 'not yet answered'}
      </span>
      <span className="q-row-edit">{state !== undefined ? 'change' : 'answer'}</span>
    </button>
  )
}

/**
 * One question, open for deciding — the decision card both interview modes
 * share. It carries, in order: the plain question (with the honest stock
 * marker when the phrase seat did not answer THIS one), the served why line,
 * the recommendation (one-click take when it names an option, a pre-fill
 * offer when it is words alone), the option chips with the recommended one
 * starred, and the always-present own-words field. The acts row prints its
 * reason whenever the primary is disabled (the finding-5 contract).
 */
function QuestionEditor({
  q,
  head,
  marker,
  initial,
  current,
  busy,
  saveLabel,
  skip,
  onSave,
  onSkip,
  onKeep,
}: {
  q: IntakeQuestion
  /** The position words in the number chip: "2 of 4". */
  head: string
  /** Whether an unphrased question marks itself (suppressed when the whole
   *  round already said "standard wording throughout" once). */
  marker: boolean
  initial?: Given
  /** The review editor's current-resolution line: how this stands today. */
  current?: string
  busy: boolean
  saveLabel: string
  /** Whether the per-question skip arm renders (interview rounds and OPEN
   *  review rows; a settled review row is changed or kept, never skipped). */
  skip?: boolean
  onSave: (value: string) => void
  onSkip?: () => void
  /** The review editor's way back: keep this point as it was. */
  onKeep?: () => void
}) {
  const options = q.options ?? []
  const initialValue = initial !== undefined && 'value' in initial ? initial.value : ''
  const initialIsOption = options.some((o) => o.value === initialValue)
  const [pick, setPick] = useState(initialIsOption ? initialValue : '')
  const [typed, setTyped] = useState(initialIsOption ? '' : initialValue)
  const recOption =
    q.suggested_option !== undefined && q.suggested_option !== ''
      ? options.find((o) => o.value === q.suggested_option)
      : undefined
  const suggestedWords = q.suggested !== undefined && q.suggested !== '' ? q.suggested : (recOption?.label ?? '')
  const hasRec = suggestedWords !== ''
  const draft = typed.trim() !== '' ? typed.trim() : pick
  const stock = q.phrased === undefined || q.phrased === ''

  return (
    <fieldset className="q-card q-live" data-question={q.id}>
      <legend className="q-legend">
        <span className="q-n mono">{head}</span> {questionWording(q)}
        {marker && stock && (
          <span className="q-stock" data-phrasing="canonical">
            standard wording: not rephrased for your goal
          </span>
        )}
      </legend>
      {q.why !== undefined && q.why !== '' && <p className="q-why">{q.why}</p>}
      {current !== undefined && <p className="q-current">{current}</p>}

      {hasRec && (
        <div className="q-rec" data-rec={recOption !== undefined ? 'option' : 'prefill'}>
          <span className="q-rec-star" aria-hidden="true">
            ★
          </span>
          <span className="q-rec-body">
            <span className="q-rec-head">Recommended for this goal</span>
            <span className="q-rec-text">{suggestedWords}</span>
          </span>
          {recOption !== undefined ? (
            // The one-click take (design §2.F): the recommendation IS one of
            // the card's own options, so taking it answers the question in
            // the card's own vocabulary and moves on.
            <Button
              variant="secondary"
              size="sm"
              disabled={busy}
              data-rec-take
              onClick={() => {
                onSave(recOption.value)
              }}
            >
              Take this
            </Button>
          ) : (
            // Words without an option: a recommended PRE-FILL, never a silent
            // answer — the words land in the box where they can be edited.
            <Button
              variant="secondary"
              size="sm"
              disabled={busy}
              data-rec-prefill
              onClick={() => {
                setTyped(suggestedWords)
                setPick('')
              }}
            >
              Start from this
            </Button>
          )}
        </div>
      )}

      {options.length > 0 && (
        <div className="q-options" role="group" aria-label={`options for: ${q.text}`}>
          {options.map((o) => {
            const active = pick === o.value && typed.trim() === ''
            const rec = o.value === q.suggested_option
            return (
              <button
                key={o.value}
                type="button"
                className="q-option"
                data-active={active ? 'true' : undefined}
                data-recommended={rec ? 'true' : undefined}
                aria-pressed={active}
                onClick={() => {
                  setPick((p) => (p === o.value ? '' : o.value))
                  setTyped('')
                }}
              >
                {active && <Check size={13} strokeWidth={2.5} aria-hidden="true" />}
                {rec && !active && (
                  <span className="q-option-star" aria-hidden="true">
                    ★
                  </span>
                )}
                {o.label}
              </button>
            )
          })}
        </div>
      )}
      <input
        className="door-input q-own"
        type="text"
        placeholder="or say it in your own words…"
        value={typed}
        onChange={(e) => {
          setTyped(e.target.value)
        }}
      />

      <div className="door-acts q-acts">
        <Button
          variant="primary"
          size="sm"
          disabled={draft === '' || busy}
          data-q-save
          onClick={() => {
            onSave(draft)
          }}
        >
          {saveLabel}
        </Button>
        {skip === true && (
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            data-q-skip
            onClick={() => {
              onSkip?.()
            }}
          >
            {hasRec ? 'Skip: go with the recommendation' : 'Skip: let it choose for me'}
          </Button>
        )}
        {onKeep !== undefined && (
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            data-q-keep
            onClick={onKeep}
          >
            Keep it as it is
          </Button>
        )}
        {draft === '' && !busy && (
          <span className="door-why">
            {hasRec
              ? 'pick an option, type your own words, or take the recommendation'
              : options.length > 0
                ? 'pick an option or type your own words'
                : 'type your answer in the box'}
          </span>
        )}
      </div>
    </fieldset>
  )
}

/* ── the review card: "Change my answers" (P3-GF3-BE1 R8; design §2.D) ───── */

/** The resolution origins in the reader's words, for the review rows. The
 *  vocabulary is the card's (intake/cards.go); anything new renders as itself
 *  (§42). */
function reviewHowWords(how: string): string {
  switch (how) {
    case 'answered':
      return 'your answer'
    case 'assumption':
      return 'assumed'
    case 'registry':
      return 'from the project record'
    case 'escalation':
      return 'you answered its question'
    default:
      return how
  }
}

/** A review row's current value, in words: the answer (as its option label
 *  where it names one), or the assumption prose the platform recorded. */
function resolutionWords(q: IntakeQuestion): string {
  const r = q.resolution
  if (r === undefined) return ''
  if (r.value !== undefined && r.value !== '') return humanValue(optionLabel(q, r.value))
  return r.assumption ?? ''
}

/**
 * Review-and-adjust: EVERY decision on this work, each wearing how it was
 * settled and what it currently says, each editable — nothing re-asked blind
 * (operator finding F5). Only what the person changes is sent; the platform
 * redrafts once with the changes merged and the plan card returns.
 */
export function ReviewForm({
  card,
  busy,
  onAnswer,
}: {
  card: IntakeCard
  busy: boolean
  onAnswer: (b: IntakeAnswerBody) => void
}) {
  const questions = card.questions ?? []
  const [changes, setChanges] = useState<Record<string, Given>>({})
  const [editing, setEditing] = useState<string | null>(null)
  const n = Object.keys(changes).length

  const save = () => {
    const entries = questions
      .filter((q) => changes[q.id] !== undefined)
      .map((q) => {
        const g = changes[q.id]
        return 'skip' in g ? { id: q.id, skip: true } : { id: q.id, value: g.value }
      })
    onAnswer({ answers: entries })
  }

  return (
    <div className="q-form" data-interview-review>
      <div className="review-head">
        <p className="decision-summary">Your decisions on this work, all in one place.</p>
        <p className="review-sub">
          Each point shows how it was settled and what it says now. Open any of them to change it; what you leave
          alone stays exactly as it is. Saving redrafts the plan with your changes and brings the plan card back.
        </p>
      </div>
      <ol className="q-seq" data-review-count={String(questions.length)}>
        {questions.map((q, i) => (
          <li key={q.id}>
            {editing === q.id ? (
              <QuestionEditor
                q={q}
                head={`${String(i + 1)} of ${String(questions.length)}`}
                marker={false}
                initial={changes[q.id] ?? (q.resolution?.how === 'answered' ? { value: q.resolution.value ?? '' } : undefined)}
                current={
                  q.resolution !== undefined
                    ? `Now: ${resolutionWords(q)} (${reviewHowWords(q.resolution.how)})`
                    : 'Still open: nobody has settled this one yet.'
                }
                busy={busy}
                saveLabel="Save this change"
                skip={q.resolution === undefined}
                onSave={(v) => {
                  setChanges((c) => ({ ...c, [q.id]: { value: v } }))
                  setEditing(null)
                }}
                onSkip={() => {
                  setChanges((c) => ({ ...c, [q.id]: { skip: true } }))
                  setEditing(null)
                }}
                onKeep={() => {
                  setChanges((c) => {
                    const rest = { ...c }
                    // eslint-disable-next-line @typescript-eslint/no-dynamic-delete
                    delete rest[q.id]
                    return rest
                  })
                  setEditing(null)
                }}
              />
            ) : (
              <ReviewRow
                q={q}
                change={changes[q.id]}
                onOpen={() => {
                  setEditing(q.id)
                }}
              />
            )}
          </li>
        ))}
      </ol>
      <div className="door-acts">
        <Button
          variant="primary"
          disabled={n === 0 || busy}
          aria-busy={busy}
          data-review="save"
          onClick={save}
        >
          {busy
            ? 'Redrafting…'
            : n === 0
              ? 'Save changes: redraft the plan'
              : `Save ${String(n)} change${n === 1 ? '' : 's'}: redraft the plan`}
        </Button>
        {n === 0 && !busy && (
          <span className="door-why">nothing changed yet: open a point above to change it, or Cancel below stops the task</span>
        )}
        {n > 0 && !busy && (
          <span className="door-why">only what you changed moves; the fresh plan card shows the result</span>
        )}
      </div>
    </div>
  )
}

/** One reviewed decision as a compact row: its state, its wording, what it
 *  says now (or what it will change to), and the change affordance. */
function ReviewRow({ q, change, onOpen }: { q: IntakeQuestion; change?: Given; onOpen: () => void }) {
  const r = q.resolution
  const changed = change !== undefined
  return (
    <button
      type="button"
      className="q-row"
      data-review-row={q.id}
      data-state={changed ? 'changed' : (r?.how ?? 'open')}
      onClick={onOpen}
    >
      <span className="q-row-mark" aria-hidden="true">
        {changed ? <Pencil size={12} strokeWidth={2} /> : r !== undefined ? <Check size={13} strokeWidth={2.5} /> : '·'}
      </span>
      <span className="q-row-q">{questionWording(q)}</span>
      <span className="q-row-a">
        {changed ? (
          'skip' in change ? (
            'changes to: take the recommendation'
          ) : (
            `changes to: ${optionLabel(q, change.value)}`
          )
        ) : r !== undefined ? (
          <>
            {resolutionWords(q)}
            <span className="q-row-how"> · {reviewHowWords(r.how)}</span>
          </>
        ) : (
          'still open: the planner decides if you leave it'
        )}
      </span>
      <span className="q-row-edit">change</span>
    </button>
  )
}

/* ── clarification: the open points that hold the plan ───────────────────── */

/**
 * The clarification (and unknown-kind fallback) form: every open point on one
 * card, options where served, free text always. Clarification cards are never
 * phrased BY DESIGN, so no stock marker rides here — marking an absence that
 * is not a fallback would be noise (#6).
 */
function QuestionForm({
  card,
  busy,
  onAnswer,
}: {
  card: IntakeCard
  busy: boolean
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

  return (
    <div className="q-form">
      <UnderstoodPanel understood={card.understood} heading="What it understood so far" />
      {questions.map((q, i) => (
        <fieldset className="q-card" key={q.id} data-question={q.id}>
          <legend className="q-legend">
            <span className="q-n mono">{String(i + 1)}</span> {questionWording(q)}
          </legend>
          {q.why !== undefined && q.why !== '' && <p className="q-why">{q.why}</p>}
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
            onAnswer({ answers: answered.map((q) => ({ id: q.id, value: valueOf(q) })) })
          }}
        >
          {busy
            ? 'Sending…'
            : openCount > 0
              ? `Send ${String(answered.length)} answer${answered.length === 1 ? '' : 's'}`
              : 'Send my answers'}
        </Button>
        {answered.length === 0 && !busy && (
          <span className="door-why">these open points hold the plan: answer at least one to move</span>
        )}
      </div>
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
          <legend className="q-legend">{q.phrased !== undefined && q.phrased !== '' ? q.phrased : q.text}</legend>
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
  // The cancel's why (P3-RW-19): `rethink` IS a cancel — it ends the intake —
  // so picking it opens the optional one-line why, riding the answer as its
  // `note`. No other choice carries it (the platform honors it on no other).
  const [why, setWhy] = useState('')
  if (d === undefined) return <p className="muted">This decision card carried no body — its controls live on its inbox card.</p>

  const needsCriteria = choice === 'drop_criterion'
  const needsFact = choice === 'supply_fact'
  const isRethink = choice === 'rethink'
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
      {isRethink && (
        <label className="door-field">
          <span className="door-label">
            Why stop it <span className="door-optional">optional — your words, kept with the record of what was stopped</span>
          </span>
          <input
            className="door-input"
            type="text"
            data-field="cancel-why"
            value={why}
            onChange={(e) => {
              setWhy(e.target.value)
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
              ...(isRethink && why.trim() !== '' ? { note: why } : {}),
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

/* ── the family card (P3-RW-11; operator finding F2 2026-08-16) ──────────── */

/**
 * The hint line under each KNOWN family value — presentation copy for a
 * non-programmer choosing a kind of work, keyed on the choice VALUE. The
 * card's own served label is always the label; an unknown value renders
 * label-only (forward tolerance, §42). These sentences describe the choice —
 * they state no platform fact and fabricate no number.
 */
const familyHints: Record<string, string> = {
  software: 'websites, apps, scripts, automations — anything that runs',
  research: 'a question to answer, options to compare, facts to gather',
  content: 'words, posts, documents, images — things people read or see',
  data: 'spreadsheets, records, cleaning, analysis, reports from numbers',
  chore: 'recurring upkeep with a known shape — tidy, rotate, archive',
  generic: 'none of the above fits — it asks broad questions instead',
}

/**
 * The family question, in the journey (F2: the "answer it from its inbox
 * card" placeholder read as a bug and is dead). Six large tappable kind-cards
 * from the card's OWN served choices — labels served, values quoted back via
 * the same {choice} envelope every decision card uses — with the 13.5 help
 * underneath. One pick, then the act; disabled states print their reason.
 */
function FamilyForm({ card, busy, onAnswer }: { card: IntakeCard; busy: boolean; onAnswer: (b: IntakeAnswerBody) => void }) {
  const d = card.decision
  const [choice, setChoice] = useState('')
  if (d === undefined) return <p className="muted">This family card carried no choices — that is a platform defect worth reporting, not something this page can answer.</p>

  const picked = d.choices.find((c) => c.value === choice)

  return (
    <div className="q-form" data-family-form>
      {d.summary !== '' && <p className="decision-summary">{d.summary}</p>}
      {(d.detail ?? []).length > 0 && (
        <ul className="decision-detail">
          {d.detail?.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      )}

      <div className="family-grid" role="radiogroup" aria-label="what kind of work this is">
        {d.choices.map((c) => {
          const active = choice === c.value
          return (
            <button
              key={c.value}
              type="button"
              className="family-choice"
              role="radio"
              aria-checked={active}
              data-family={c.value}
              data-active={active ? 'true' : undefined}
              onClick={() => {
                setChoice(c.value)
              }}
            >
              <span className="family-label">
                {active && <Check size={14} strokeWidth={2.5} aria-hidden="true" />}
                {c.label}
              </span>
              {familyHints[c.value] !== undefined && <span className="family-hint">{familyHints[c.value]}</span>}
            </button>
          )
        })}
      </div>

      <HelpNote help={d.help} />
      <div className="door-acts">
        <Button
          variant="primary"
          disabled={choice === '' || busy}
          aria-busy={busy}
          data-family-send
          onClick={() => {
            onAnswer({ choice })
          }}
        >
          {busy
            ? 'Sending…'
            : picked !== undefined
              ? `This is: ${picked.label}`
              : 'Tell it the kind of work'}
        </Button>
        {choice === '' && !busy && <span className="door-why">pick the kind of work first — the right questions follow from it</span>}
      </div>
    </div>
  )
}

/* ── the plan card (Stage-4 approval, map §3 anatomy) ────────────────────── */

/** The S06.9 verbs in plain names (design §2.F): what each one is called, and
 *  under it what it DOES — the consequence said where the button is, not one
 *  screen later. The ids stay the card's frozen vocabulary. */
const planActionLabels: Record<string, string> = {
  approve: 'Approve: start the work',
  replan: 'Change the plan…',
  reinterview: 'Change my answers…',
  cancel: 'Cancel the task',
  compose: 'Compose a specialist for this…',
}

const planActionWhy: Record<string, string> = {
  approve: 'locks this plan in and begins. Nothing spends before this.',
  replan: 'tap what is wrong, as many things as you like, and say it in your words. It redrafts once and shows exactly what changed.',
  reinterview: 'reopens every question with your current answers filled in. Change what you want; the rest stays.',
  cancel: 'stops here. Nothing runs, nothing is spent, the task keeps its record.',
}

export function PlanCard({
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
  // Multi-contest (P3-GF3-BE1 R10; design §2.E): ANY number of the plan's own
  // keys, plus the always-present own-words note — one send, one redraft.
  const [targets, setTargets] = useState<string[]>([])
  const [note, setNote] = useState('')
  // The cancel's two-step (P3-RW-19): pressing Cancel flips into the same
  // inline pane Re-plan uses, where the why is asked for — optional, the
  // person's own words, riding the `{action:"cancel"}` answer as its `note`.
  const [cancelling, setCancelling] = useState(false)
  const [why, setWhy] = useState('')
  if (a === undefined) return <p className="muted">This approval card carried no body — approve it from its inbox card.</p>

  const l1 = a.layer1
  const steps = a.layer2?.steps ?? []
  const acs = a.layer2?.acs ?? []
  const estimate = a.layer2?.estimate
  const actions = a.actions ?? ['approve', 'replan', 'reinterview', 'cancel']

  // The structured entry (S06.9): tap the steps, checks and assumptions being
  // contested — any number of them (the S06.9 structure is named targets, not
  // a cardinality of one). The keys are the card's own vocabulary; the group
  // headings say what each key family IS in plain words.
  const contestGroups: { name: string; items: { key: string; label: string }[] }[] = [
    { name: 'The steps', items: steps.map((s) => ({ key: s.id, label: `${s.id} · ${s.title}` })) },
    {
      name: 'The finish-line checks (what "done" means)',
      items: acs.map((ac) => ({ key: `AC-${String(ac.n)}`, label: `AC-${String(ac.n)} · ${ac.plain}` })),
    },
    {
      name: 'The assumptions',
      items: (l1.assumptions ?? []).map((as) => ({ key: `assumption:${as.text}`, label: as.text })),
    },
  ].filter((g) => g.items.length > 0)

  return (
    <div className="plan-card">
      {a.stale_flag === true && (
        <p className="plan-stale" role="alert">
          <CircleAlert size={14} strokeWidth={2} aria-hidden="true" /> Assumptions may be stale:{' '}
          {(a.stale_reasons ?? []).join(' · ') || 'the world moved since this card was drafted'}. &quot;Change the
          plan&quot; refreshes it; approving anyway is still yours to choose.
        </p>
      )}

      <section className="plan-sec" data-plan="understood">
        <h3 className="plan-h">What I understood</h3>
        <p className="plan-restatement">{l1.restatement}</p>
        {/* Beside the planner's prose: what the PLATFORM recorded, slot by
            slot, each answer origin-labeled (P3-RW-12 R9). The two together
            are the 1.3 restate-and-confirm — approve IS the confirmation.
            Assumed slots show the planner's ACTUAL assumed value (the
            `assumption:<slot>` rows this same card carries), never the
            contentless template (blocker #15). */}
        <UnderstoodPanel
          understood={l1.understood}
          heading="Point by point — what was settled, and how"
          resolutions={slotResolutions(l1.assumptions ?? [])}
        />
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
        <AssumptionList assumptions={l1.assumptions ?? []} />
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
        {/* ONE cost voice (W1-5): the platform's estimate is the cost figure,
            and when no dollar figure exists the line says WHY in the same
            words the receipts will use — never a third dialect. The time half
            is LABELED, so its absence is a loud stated absence instead of a
            heading quietly missing its second noun. The planner's own prose,
            when it exists, renders under its own name — it is the planner
            speaking, not this page's figure. */}
        <p className="m-0" data-cost-line>
          {estimate !== undefined && estimate.known ? (
            <b className="mono">cost: ≈ USD {String(estimate.usd ?? 0)}</b>
          ) : (
            // RA-3: the door promised "a plan with a price", and the walk met
            // "UNPRICED — no per-call dollar price exists…" instead. The humane
            // sentence is structurally honest at v0: metered per-call selection
            // is switched off (D5), so a routed paid lane is BY CONSTRUCTION
            // one this household holds as a subscription, and the local lane is
            // the house's own machine. The billing vocabulary moves one fold
            // down, where the receipt's own word (UNPRICED) is explained.
            <span data-cost-unpriced="true">
              {a.routing?.lane === 'local' ? (
                <>
                  cost: <b>no extra charge</b>
                  <span className="muted"> — this runs on the house&apos;s own computer</span>
                </>
              ) : a.routing?.lane !== undefined && a.routing.lane !== '' ? (
                <>
                  cost: <b>no extra charge</b>
                  <span className="muted">
                    {' '}
                    — covered by your {a.routing.lane} subscription, which this work runs on
                  </span>
                </>
              ) : (
                <>
                  cost: <span className="muted">no dollar figure exists for this work</span>
                </>
              )}
            </span>
          )}
          {estimate?.size_class !== undefined && estimate.size_class !== '' && <> · size: {estimate.size_class}</>}
          {l1.cost_time !== undefined && l1.cost_time !== '' ? (
            <>
              {' · '}the planner adds: <span data-planner-words>&ldquo;{l1.cost_time}&rdquo;</span>
            </>
          ) : (
            <>
              {' · '}time: <span className="muted">no estimate was drafted</span>
            </>
          )}
          {l1.size_note !== undefined && l1.size_note !== '' && <span className="warn-flag"> · {l1.size_note}</span>}
        </p>
        {estimate !== undefined && !estimate.known && (
          <details className="cost-tech" data-cost-detail>
            <summary className="cursor-pointer text-xs text-muted-foreground">how this is billed, exactly</summary>
            <p className="m-0 text-xs text-muted-foreground">
              No per-call dollar price exists
              {a.routing?.lane !== undefined && a.routing.lane !== '' && <> on the {a.routing.lane} lane</>}: paying
              per call is switched off at this version, so work runs on lanes this household already holds. The receipt
              itemizes every call with the same label the meters use — UNPRICED, never a silent $0.
            </p>
          </details>
        )}
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

      {!contesting && !cancelling ? (
        <div className="plan-verbs" data-plan-verbs>
          {actions.includes('approve') && (
            <div className="plan-verb">
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
              <span className="plan-verb-why">{planActionWhy.approve}</span>
            </div>
          )}
          {actions.includes('replan') && (
            <div className="plan-verb">
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
              <span className="plan-verb-why">{planActionWhy.replan}</span>
            </div>
          )}
          {actions.includes('reinterview') && (
            <div className="plan-verb">
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
              <span className="plan-verb-why">{planActionWhy.reinterview}</span>
            </div>
          )}
          {actions.includes('cancel') && (
            <div className="plan-verb">
              <Button
                variant="danger"
                disabled={busy}
                data-plan-act="cancel"
                onClick={() => {
                  setCancelling(true)
                }}
              >
                {planActionLabels.cancel}
              </Button>
              <span className="plan-verb-why">{planActionWhy.cancel}</span>
            </div>
          )}
        </div>
      ) : cancelling ? (
        <div className="contest" data-plan="cancel">
          <h3 className="plan-h plan-h-not">Cancel this task?</h3>
          <p className="m-0 text-sm">
            It stops here: the interview and this plan close, and nothing is started or spent. The task keeps its
            record, marked cancelled — with your why on it, if you give one.
          </p>
          <label className="door-field">
            <span className="door-label">
              Why cancel it <span className="door-optional">optional — your words, kept with the record of what was stopped</span>
            </span>
            <input
              className="door-input"
              type="text"
              data-field="cancel-why"
              value={why}
              onChange={(e) => {
                setWhy(e.target.value)
              }}
            />
          </label>
          <div className="door-acts">
            <Button
              variant="danger"
              disabled={busy}
              aria-busy={busy}
              data-plan-act="send-cancel"
              onClick={() => {
                onAnswer({ action: 'cancel', ...(why.trim() !== '' ? { note: why } : {}) })
              }}
            >
              {busy ? 'Cancelling…' : 'Cancel the task'}
            </Button>
            <Button
              variant="ghost"
              disabled={busy}
              data-plan-act="cancel-back"
              onClick={() => {
                setCancelling(false)
              }}
            >
              Back to the plan
            </Button>
          </div>
        </div>
      ) : (
        <div className="contest" data-plan="contest">
          <h3 className="plan-h">Change the plan</h3>
          <p className="contest-sub">
            Tap everything that is not right, as many things as you like, and say what you want below. It redrafts
            once with all of it and then shows you exactly what changed.
          </p>
          {contestGroups.map((g) => (
            <div className="contest-group" key={g.name}>
              <p className="contest-group-name">{g.name}</p>
              <div className="q-options">
                {g.items.map((t) => {
                  const active = targets.includes(t.key)
                  return (
                    <button
                      key={t.key}
                      type="button"
                      className="q-option contest-target"
                      data-active={active ? 'true' : undefined}
                      aria-pressed={active}
                      onClick={() => {
                        setTargets((ts) => (ts.includes(t.key) ? ts.filter((k) => k !== t.key) : [...ts, t.key]))
                      }}
                    >
                      {active && <Check size={13} strokeWidth={2.5} aria-hidden="true" />}
                      {t.label}
                    </button>
                  )
                })}
              </div>
            </div>
          ))}
          <label className="door-field">
            <span className="door-label">
              What should be different? <span className="door-optional">your own words are enough, tapped items or not</span>
            </span>
            <textarea
              className="door-text"
              rows={2}
              value={note}
              data-field="contest-note"
              onChange={(e) => {
                setNote(e.target.value)
              }}
            />
          </label>
          <div className="door-acts">
            <Button
              variant="primary"
              disabled={(targets.length === 0 && note.trim() === '') || busy}
              aria-busy={busy}
              data-plan-act="send-replan"
              onClick={() => {
                onAnswer({
                  action: 'replan',
                  ...(targets.length > 0 ? { contests: targets.map((t) => ({ target: t })) } : {}),
                  ...(note.trim() !== '' ? { note: note.trim() } : {}),
                })
              }}
            >
              {busy
                ? 'Redrafting…'
                : targets.length > 0
                  ? `Redraft the plan (${String(targets.length)} tapped${note.trim() !== '' ? ' + your words' : ''})`
                  : 'Redraft the plan'}
            </Button>
            <Button
              variant="ghost"
              disabled={busy}
              onClick={() => {
                setContesting(false)
                setTargets([])
              }}
            >
              Back to the plan
            </Button>
            {targets.length === 0 && note.trim() === '' && !busy && (
              <span className="door-why">tap what is wrong above, or say it in words: either is enough</span>
            )}
          </div>
        </div>
      )}
      {view.tier === 'high' && !contesting && !cancelling && (
        <p className="muted mt-1 text-xs">High stakes: approving asks for your PIN in the same breath.</p>
      )}
    </div>
  )
}

/**
 * The plan's assumption list, deduplicated (blocker #15).
 *
 * The wire carries every skipped slot TWICE: the planner's substantive row
 * (origin `assumption:<slots>`, the actual assumed value) and the platform's
 * per-slot template row (origin `slot:<id>`, "…so I assumed a sensible
 * default"). Both are true, but printing both walls the card while the second
 * says nothing. Here: substantive rows render whole; template rows whose slot
 * a substantive row already covers are DROPPED as the duplicates they are; the
 * uncovered remainder collapses into one plain line naming the skipped points.
 * Rows this logic does not recognize render verbatim — nothing is hidden on a
 * guess.
 */
function AssumptionList({ assumptions }: { assumptions: { text: string; origin?: string }[] }) {
  if (assumptions.length === 0) return <p className="muted m-0">None. Everything it needed, you answered.</p>
  const covered = new Set(slotResolutions(assumptions).keys())
  const shown: typeof assumptions = []
  const skipped: string[] = []
  for (const as of assumptions) {
    const slotOrigin = as.origin !== undefined && as.origin.startsWith('slot:') ? originSlots(as.origin) : []
    const nameFromText = /^(.+?) — /.exec(as.text)?.[1] ?? ''
    if (slotOrigin.length > 0 && isAssumedDefaultBoilerplate(nameFromText, as.text)) {
      if (slotOrigin.every((s) => covered.has(s))) continue // the substantive row above already states the default
      skipped.push(nameFromText !== '' ? nameFromText : humanSlot(slotOrigin[0]))
      continue
    }
    shown.push(as)
  }
  return (
    <ul className="plan-list">
      {shown.map((as) => (
        <li key={as.text}>
          {as.text}
          {as.origin !== undefined && as.origin !== '' && <span className="assume-origin"> · {originWords(as.origin)}</span>}
        </li>
      ))}
      {skipped.length > 0 && (
        <li data-assume-skipped={String(skipped.length)}>
          {skipped.join(' · ')} — skipped in the interview, so it goes ahead on sensible defaults it has not spelled
          out. If that is not what you meant, tap it under &quot;Change the plan&quot; below.
          <span className="assume-origin"> · assumed because you chose to proceed</span>
        </li>
      )}
    </ul>
  )
}

/** The S06.6 assumption origins, in the reader's words — raw slot ids and
 *  origin tags never surface (blocker #15; jargon sweep #17). */
export function originWords(origin: string): string {
  if (origin.startsWith('slot:')) return `assumed because you skipped "${humanSlot(origin.slice(5))}"`
  if (origin === 'force_proceed') return 'assumed because you chose to proceed'
  if (origin === 'planner') return 'the planner assumed it'
  if (origin === 'band') return 'assumed — the ask was small enough to run without an interview'
  if (origin.startsWith('answered:'))
    return `follows from what you answered (${originSlots(origin).map(humanSlot).join(', ')})`
  if (origin.startsWith('assumption:'))
    return `its stated call on ${originSlots(origin).map(humanSlot).join(', ')}`
  if (origin.startsWith('research:')) return 'from its own research notes'
  const marker = /^marker-(\d+) answered$/.exec(origin)
  if (marker !== null) return `settled by your answer to open question ${marker[1]}`
  return origin
}

/** The delta card's origin tokens in the reader's words (jargon sweep, design
 *  §2.F). The tokens are the wire's own vocabulary (intake/cards.go); an
 *  unknown one renders as itself — forward tolerance over silence (§42). */
export function deltaOriginWords(origin?: string): string {
  switch (origin) {
    case 'contested_card':
      return 'you asked for changes'
    case 'freshness_revalidation':
      return 'the world moved since your approval, so it re-checked its plan'
    case 'sibling_collision':
      return 'other accepted work in this project touched the same things'
    case 'confinement_widening':
      return 'it needs to touch more than it first said'
    case undefined:
    case '':
      return 'the cause was not recorded'
    default:
      return origin
  }
}

function DeltaForm({ card, busy, onAnswer }: { card: IntakeCard; busy: boolean; onAnswer: (b: IntakeAnswerBody) => void }) {
  const d = card.delta
  if (d === undefined) return <p className="muted">This delta card carried no body — answer it from its inbox card.</p>
  const actions = d.actions ?? ['approve', 'reject']
  return (
    <div className="q-form">
      <div className="q-card">
        <p className="decision-summary">
          Why it changed: {deltaOriginWords(d.origin)}. Only the items below changed; everything else you approved
          stands.
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
 *  toward its next card. This page resumes by itself when the card exists —
 *  and it SAYS what the machine is doing meanwhile, with its own clock
 *  (never-stall-silently; the first phrased card takes about a minute on the
 *  local models). */
function NoCardYet({ view, waiting, answered }: { view: IntakeTaskView; waiting: boolean; answered: boolean }) {
  const seconds = useElapsedSeconds(view.tier !== 'trivial')
  // The one served fact that changes this face's story: the task's own run
  // states. A CRASHED intake run is not a dead journey — the S02.5 recovery
  // ladder forks and re-drives it within a sweep, machine-only (it happened
  // on the very walk that wrote this component: the local planner emitted a
  // truncated draft, and the healed plan arrived minutes later with no button
  // pressed). The face tells that truth instead of letting a longer-than-
  // usual wait read as a stall.
  const detail = useLive({
    key: `/api/tasks/${view.task_id}#nocard`,
    read: () => api.task(view.task_id),
    types: ['run.state_changed', 'intake.state'],
  })
  const runs = detail.data?.runs ?? []
  const crashed = runs.length > 0 && runs[runs.length - 1].state === 'crashed'
  // The story has moved past birth when THIS TAB folded answers in — or when
  // the SERVED record says so: a second run (healed fork, later stage), a
  // drafted spec or plan (the first emission exists only after answers), any
  // recorded stage boundary or human decision. A resumed tab reads the record
  // instead of greeting a mid-interview task as newborn (review #7 — the
  // birth copy stood on two paths where this face knew better).
  // NOT stage_progress: the intake run records a boundary at BIRTH, which made
  // this face greet a newborn with "answers recorded" (caught live, drain r1).
  const working =
    answered ||
    runs.length > 1 ||
    detail.data?.spec != null ||
    detail.data?.plan != null ||
    (detail.data?.decisions ?? []).length > 0
  return (
    <div className="door-landed" data-door-result="no-card" data-after-answer={answered ? 'true' : undefined}>
      <h3 className="landed-head">
        {view.tier === 'trivial'
          ? 'It took the work without questions'
          : crashed
            ? 'The working session broke — it heals itself'
            : working
              ? 'Answers recorded — it is working'
              : 'The task is born — it is reading your goal'}
      </h3>
      <p className="landed-sub">
        {view.tier === 'trivial' ? (
          <>Trivial, read-only asks skip the ceremony on purpose. If a question comes up it lands in your{' '}
          <Link to={hrefFor('inbox')}>Inbox</Link>.</>
        ) : crashed ? (
          <>
            The session doing the work died mid-write — that happens, and nothing is asked of you: the platform
            notices, salvages what was done, and re-drives the work on its own within a few minutes. Your answers are
            all kept. The next card appears RIGHT HERE when it lands, and in your <Link to={hrefFor('inbox')}>Inbox</Link>{' '}
            too.
          </>
        ) : working ? (
          // TODAY'S TRUTH (PH-1): no live "phrasing" step exists to claim —
          // questions arrive in standard wording and admit it on the card.
          <>
            It took what you said and moved: it is choosing the next questions, or — once it knows
            enough — drafting the full plan. A question round takes about a minute on the local models; the plan is
            drafted in one piece and can take a few minutes. It appears RIGHT HERE the moment it exists, and lands in
            your <Link to={hrefFor('inbox')}>Inbox</Link> too. You can leave; nothing is lost.
          </>
        ) : (
          <>
            It is working out what it must ask you — sizing the goal and picking the questions that matter. The
            first card takes about a minute on the local models, appears RIGHT HERE the moment it exists,
            and lands in your <Link to={hrefFor('inbox')}>Inbox</Link> too. You can leave; nothing is lost.
          </>
        )}
      </p>
      {view.tier !== 'trivial' && (
        <p className="composing-clock mono" data-birth-clock data-run-crashed={crashed ? 'true' : undefined}>
          {crashed ? 'healing machine-side' : waiting ? 'listening' : 'catching up'} · {elapsedWords(seconds)}
        </p>
      )}
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
