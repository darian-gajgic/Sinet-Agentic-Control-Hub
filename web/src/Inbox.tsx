import { useCallback, useEffect, useState, type ReactNode } from 'react'

import {
  api,
  type VerdictReveal,
  staleCard,
  type ApprovalBatchOutcome,
  type ApprovalItem,
  type ApprovalList,
  type BenchmarkVerdictForms,
  type MemoryConflict,
  type PendingPair,
} from './api'
import { ActConfirm, OutcomeLine, outcomeOf, useAct } from './controls'
import type { EventStream } from './events'
import { describeError, inboxEventTypes, useLive } from './live'
import { Absent, Empty, Freshness, Owner, Stamp } from './parts'
import { Link } from './router'
import { hrefFor } from './routes'
import { reconcileBadge } from './push'
import { Button, Panel } from './ui'

/**
 * The one approval inbox (Spec S15.6; S3.2; 13.5; S06.9; S14.4–S14.7).
 *
 * Every card the platform needs a person for arrives here, in ONE queue, ranked
 * by risk SERVER-SIDE. Four rules shape every line below.
 *
 * THE CARD IS THE AUTHORITY. A control renders only where the card served the
 * action in its own `actions` vocabulary, and only where `answerable` is true.
 * A card the caller cannot answer renders its served reason instead — never a
 * dead button, never a door this view invented. That is the D9 principle on
 * screen: the inbox never offers a control whose use would be refused.
 *
 * THE SERVER RANKS, THE CLIENT DOES NOT. The list renders in served order. The
 * display groups below are presentation over that order and never a filter:
 * every served card appears exactly once, and a kind this file has never heard
 * of still gets a row.
 *
 * NOTHING IS COMPUTED THAT WAS NOT SERVED. No percentage, no fraction, no
 * estimate of anything. The expiry line is a display derivation of a served
 * INSTANT and nothing else; when no expiry is served, none is shown.
 *
 * THE PIN LIVES ONLY IN THE REQUEST. A High-tier answer collects the actor's
 * PIN, sends it in the SAME request the answer rides (S01.9 verify-at-act), and
 * clears it. It is never stored, never kept in state past the submit, and there
 * is no elevation to inherit.
 */
export function Inbox({ stream }: { stream?: EventStream }) {
  const live = useLive<ApprovalList>({
    key: '/api/approvals',
    read: () => api.approvals(),
    types: inboxEventTypes,
    stream,
  })
  const items = live.data?.items ?? []
  // The badge is reconciled from the SERVED count, not recomputed: it is the
  // length of the very list below, which is the same derivation the notifier
  // counts when it composes a push (B6-9 OQ5). Without this the badge holds its
  // last PUSHED value until the next cadence, so answering a card would leave
  // the home screen claiming work that is done (drain r1, D2).
  useEffect(() => {
    if (live.data !== null) reconcileBadge(items.length)
  }, [live.data, items.length])

  return (
    <section className="surface inbox">
      <h1>Inbox</h1>
      <Freshness stale={live.stale} error={live.error} hasData={live.data !== null} />
      <p className="muted">
        Everything waiting on a person, ranked by risk by the control plane. This page never re-orders it.
      </p>
      <NoFrameNote items={items} />
      <BenchmarkOptIn stream={stream} />
      {live.data && items.length === 0 ? (
        <Empty what="Nothing is waiting on you." />
      ) : (
        <>
          <BatchBar items={items} onAnswered={live.reload} />
          <WithForms items={items} stream={stream}>
            {(forms) => (
              <ol className="cards">
                {items.map((item) => (
                  <li
                    key={item.id}
                    className="card-row"
                    data-card-id={item.id}
                    data-kind={item.kind}
                    data-tier={item.tier}
                  >
                    <CardHead item={item} />
                    <CardBody item={item} forms={forms} onAnswered={live.reload} />
                  </li>
                ))}
              </ol>
            )}
          </WithForms>
        </>
      )}
      {live.data?.truncated && (
        <p className="muted">
          This is one page of a longer queue — the control plane bounds what one read returns. Answer some and re-read.
        </p>
      )}
    </section>
  )
}

/** /inbox/:id — the stable deep link one card has, and the target a push
 *  notification's `navigate` field points at (S15.11). The id round-trips
 *  through the route table's own encoding, which matters because real card ids
 *  carry `:`, `#` and a unit separator. */
export function InboxItem({ id, stream }: { id: string; stream?: EventStream }) {
  const live = useLive<ApprovalList>({
    key: '/api/approvals',
    read: () => api.approvals(),
    types: inboxEventTypes,
    stream,
  })
  const all = live.data?.items ?? []
  const item = all.find((i) => i.id === id) ?? null
  // The deep-link surface reconciles too, and it is the one that matters most:
  // it is where a tapped notification lands, and answering here is exactly the
  // moment the drill checks the badge against the queue.
  useEffect(() => {
    if (live.data !== null) reconcileBadge(all.length)
  }, [live.data, all.length])

  return (
    <section className="surface inbox">
      <h1>Approval</h1>
      <p className="muted">
        <Link to={hrefFor('inbox')}>← the whole queue</Link>
      </p>
      <Freshness stale={live.stale} error={live.error} hasData={live.data !== null} />
      {live.data && item === null ? (
        <Absent
          reason={`No card with the id ${id} is waiting on you. It may have been answered already, or it may belong to somebody else — the inbox only ever carries the cards you can see.`}
        />
      ) : (
        item && (
          <div className="card-row" data-card-id={item.id} data-kind={item.kind} data-tier={item.tier}>
            <CardHead item={item} />
            <WithForms items={[item]} stream={stream}>
              {(forms) => <CardBody item={item} forms={forms} onAnswered={live.reload} expanded />}
            </WithForms>
          </div>
        )
      )}
    </section>
  )
}

/**
 * WithForms is the ONE read of the blind-pair form data, and it exists in this
 * shape for two reasons.
 *
 * It is read ONCE for the whole queue rather than per card, because it is one
 * question with one answer — and it is mounted only when a benchmark card is
 * actually on screen, so an inbox with none never asks a subsystem that may not
 * be wired in this process. A hook cannot be called conditionally, which is why
 * the condition lives in what MOUNTS rather than in what the hook does.
 */
function WithForms({
  items,
  stream,
  children,
}: {
  items: ApprovalItem[]
  stream?: EventStream
  children: (forms: BenchmarkVerdictForms | null) => ReactNode
}) {
  const needed = items.some((i) => i.kind === 'benchmark_verdict' || i.kind === 'benchmark_alarm')
  if (!needed) return <>{children(null)}</>
  return <FormsReader stream={stream}>{children}</FormsReader>
}

function FormsReader({
  stream,
  children,
}: {
  stream?: EventStream
  children: (forms: BenchmarkVerdictForms | null) => ReactNode
}) {
  const live = useLive<BenchmarkVerdictForms>({
    key: '/api/benchmark/verdicts',
    read: () => api.benchmarkVerdicts(),
    types: inboxEventTypes,
    stream,
  })
  return (
    <>
      {live.error !== '' && <p className="error">{live.error}</p>}
      {children(live.data)}
    </>
  )
}

/**
 * The BENCH-REG §4.2.1 standing consent, and it lives HERE because this is where
 * its consequences arrive: a sampled pair becomes a blind card in this queue.
 *
 * IT RENDERS WHETHER OR NOT A PAIR IS PENDING. That is the whole point — this is
 * the control that MAKES somebody a participant, so gating it on already being
 * one would be a door only the people already through it can see. It is
 * therefore its own read of the verdict surface rather than a rider on the one
 * the cards make, which mount only when a benchmark card is on screen.
 *
 * NOBODY ELSE CAN SET IT. There is no person picker here and there is no
 * `person` in the request: consent is the requester's own, the operator
 * included, and the verb refuses a delegation at the transport AND inside the
 * package. So the subject of this control is always the person reading it.
 */
function BenchmarkOptIn({ stream }: { stream?: EventStream }) {
  const live = useLive<BenchmarkVerdictForms>({
    key: '/api/benchmark/verdicts',
    read: () => api.benchmarkVerdicts(),
    // The flip mints a family-5 `decision.recorded` inside its own transaction,
    // and this queue already subscribes to that type — so the position re-reads
    // on the landed subscription with no change to the stream layer at all.
    types: inboxEventTypes,
    stream,
  })
  const optIn = live.data?.opt_in
  const position = optIn?.enabled

  return (
    <Panel head={<strong>Comparing the platform against your own subscription</strong>} data-control="benchmark-opt-in">
      {/* The pre-act explanation (BENCH-REG §2/§3/§4.2), in operator words and
          carrying NO REGISTERED FIGURE. How often a task is sampled, how many
          pairs a result needs and where the alarm sits are frozen registered
          text with exactly two homes — the registration and the package that
          encodes it — and a third copy here would be free to drift from the
          machinery that decides on them (§17). What a person needs before
          consenting is the MECHANISM, and that is what this says. */}
      <div className="max-w-prose text-sm" data-explainer="benchmark-opt-in">
        <p>
          With this turned on, a task of yours in a domain the practice covers may be picked out for comparison. The
          picked task is run <strong>once more</strong> — a single shot, with no follow-up turns — against your own
          frontier subscription, from the same frozen task statement and the same attachments.
        </p>
        <p className="mt-2">
          <strong>That extra run is paid for out of your own automation budget</strong>, not the platform&apos;s.
        </p>
        <p className="mt-2">
          When both answers exist, a card arrives in this queue showing them side by side without saying which is which.
          You pick the better one and, in the same act, you say which one you think the platform produced — the guess is
          not optional, because it is how the comparison checks that the two were genuinely indistinguishable.
        </p>
        <p className="mt-2">
          You can turn down any particular pair. A decline is recorded and reported beside the results, so choosing not
          to take part in one comparison is never invisible. The record is kept for good.
        </p>
        <p className="mt-2">
          This switch starts off. Turning it on or off is itself logged as your decision, and{' '}
          <strong>nobody else can set it for you</strong> — the operator included.
        </p>
      </div>

      <p className="mt-3" data-position={position === undefined ? 'unread' : String(position)}>
        {live.data === null && live.error === '' ? (
          <span className="muted">Reading your standing decision…</span>
        ) : optIn === undefined || (optIn.enabled === undefined && (optIn.absent ?? '') === '') ? (
          <Absent reason="this process served no standing decision" />
        ) : optIn.enabled === undefined ? (
          <Absent reason={optIn.absent ?? ''} />
        ) : optIn.enabled ? (
          <>You are opted in: your tasks may be picked for comparison.</>
        ) : (
          <>You are not opted in: none of your tasks is picked for comparison.</>
        )}
      </p>
      {live.error !== '' && <p className="error">{live.error}</p>}

      <OptInSwitch reload={live.reload} />
    </Panel>
  )
}

/**
 * Two POSITION acts, never a toggle over a state this client guessed.
 *
 * Both are always offered, in both positions: the flip records a human decision
 * either way and the verb has no already-there arm to report, so this control
 * invents none. Asking for the position you are already in is a decision you are
 * allowed to record again.
 */
function OptInSwitch({ reload }: { reload: () => void }) {
  const [asking, setAsking] = useState<boolean | null>(null)
  const act = useAct()
  return (
    <div className="mt-3" data-control="benchmark-opt-in-switch">
      <div className="flex flex-wrap gap-2">
        <Button
          variant="secondary"
          size="sm"
          data-opt-in="true"
          disabled={act.busy}
          onClick={() => {
            act.clear()
            setAsking(true)
          }}
        >
          Opt in
        </Button>
        <Button
          variant="secondary"
          size="sm"
          data-opt-in="false"
          disabled={act.busy}
          onClick={() => {
            act.clear()
            setAsking(false)
          }}
        >
          Opt out
        </Button>
      </div>
      <ActConfirm
        open={asking !== null}
        onOpenChange={(open) => setAsking(open ? asking : null)}
        title={asking === true ? 'Take part in the comparison' : 'Stop taking part'}
        what={
          asking === true
            ? "From now on a task of yours may be picked out and run a second time against your own subscription, paid for out of your own automation budget, and you will be asked to judge the two answers blind. You can turn down any particular pair, and you can turn this off again at any time."
            : 'From now on none of your tasks is picked out for comparison. Any pair already waiting for your judgement stays in this queue — you can still judge it or turn it down.'
        }
        act={asking === true ? 'Opt me in' : 'Opt me out'}
        variant="primary"
        busy={act.busy}
        onConfirm={() => {
          const enabled = asking === true
          setAsking(null)
          act.run(
            () =>
              // No `person` is sent, ever: the subject defaults to the caller,
              // and that is the only subject this consent has.
              api.setBenchmarkOptIn(enabled).then((res) => outcomeOf(true, `recorded: ${res.enabled ? 'opted in' : 'opted out'}`, res.detail)),
            reload,
          )
        }}
      />
      <OutcomeLine outcome={act.outcome} />
    </div>
  )
}

/**
 * The honest note about the ninth kind's currency (B6-6 OQ1).
 *
 * Every other kind here is pushed: a frame arrives and the view re-reads. An
 * open memory conflict reaches its addressee through no frame at all, so this
 * says so where the cards are rather than compensating with a poll — a ticker
 * would make the surface look live for a kind that is not.
 */
function NoFrameNote({ items }: { items: ApprovalItem[] }) {
  if (!items.some((i) => i.kind === 'memory_conflict')) return null
  return (
    <p className="muted" data-note="no-live-frame">
      Memory-conflict questions are read fresh each time this page loads or anything else here changes — nothing pushes
      them. Answering one twice is safe: the second answer reads the closed question back.
    </p>
  )
}

// ── the card head: identity, tier, owner, answerability, expiry ────────────

function CardHead({ item }: { item: ApprovalItem }) {
  return (
    <div className="card-head">
      <Link to={hrefFor('inbox-item', { id: item.id })} className="card-id">
        {item.id}
      </Link>
      <span className={item.tier === 'high' ? 'warn-flag' : 'muted'} data-tier-label={item.tier}>
        {item.tier}
      </span>
      <span className="muted">{displayClass(item)}</span>
      <Owner id={item.owner} />
      {item.run_id && <Link to={hrefFor('task', { id: item.run_id })}>{item.run_id}</Link>}
      {item.answerable ? (
        <span className="notice">yours to answer</span>
      ) : (
        <Absent reason={item.not_answerable_reason ?? 'not yours to answer'} />
      )}
      {item.step_up_required && <span className="warn-flag">PIN required</span>}
      <span className="muted">
        seen <Stamp ts={item.observed_ts} />
      </span>
      <Expiry item={item} />
      <Staleness item={item} />
    </div>
  )
}

/**
 * The S15.6 display classes, over the SERVED kind.
 *
 * Presentation only: it groups nothing, hides nothing and re-orders nothing —
 * it labels a card so a reader can tell a sign-off from a question at a glance.
 * A kind this list has never seen renders under its own served name rather than
 * disappearing, which is the same forward-tolerance the board applies to kanban
 * columns (§42).
 */
function displayClass(item: ApprovalItem): string {
  switch (item.kind) {
    case 'ask':
      return 'question'
    case 'effect':
      return 'proposal'
    case 'watchdog_flag':
    case 'drift_card':
    case 'conformance_card':
    case 'benchmark_alarm':
      return 'sign-off'
    case 'benchmark_verdict':
      return 'judgement'
    case 'memory_conflict':
      return 'question'
    default:
      return item.kind
  }
}

/**
 * The expiry line. `expiry_at` is a served INSTANT; the remaining span is a
 * display derivation of it, taken at render and never on a timer — the view
 * re-reads on frames, so the figure moves when the data does. Absent expiry
 * renders NOTHING: inventing a deadline the platform did not set would be worse
 * than saying nothing at all.
 */
function Expiry({ item }: { item: ApprovalItem }) {
  if (!item.expiry_at) return null
  const remaining = Date.parse(item.expiry_at) - Date.now()
  return (
    <span className="expiry" data-expiry={item.expiry_at}>
      expires <Stamp ts={item.expiry_at} />
      <span className={remaining <= 0 ? 'warn-flag' : 'muted'}>
        {' '}
        {remaining <= 0 ? '(past)' : `(in ${describeSpan(remaining)})`}
      </span>
      {item.engine_expiry_ts && (
        <span className="muted">
          {' '}
          · the engine's own deadline: <Stamp ts={item.engine_expiry_ts} />
        </span>
      )}
    </span>
  )
}

/** describeSpan renders a duration in the largest whole unit it fills. It
 *  divides only a span the server handed over as two instants — there is no
 *  money, no progress and no denominator anywhere near it. */
function describeSpan(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${String(seconds)}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${String(minutes)}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${String(hours)}h`
  return `${String(Math.floor(hours / 24))}d`
}

/** The S06.9 staleness flag with its SERVED reasons. It never blocks
 *  approving — the one-click re-plan sits beside the approve, not in front of
 *  it (R5). */
function Staleness({ item }: { item: ApprovalItem }) {
  if (!item.stale) return null
  const reasons = item.stale_reasons ?? []
  return (
    <span className="warn-flag" data-stale-flag="true">
      assumptions may be stale
      {reasons.length > 0 && <span className="muted"> — {reasons.join('; ')}</span>}
    </span>
  )
}

// ── the card body: content, then the acts the card itself names ────────────

function CardBody({
  item,
  forms,
  onAnswered,
  expanded,
}: {
  item: ApprovalItem
  forms: BenchmarkVerdictForms | null
  onAnswered: () => void
  expanded?: boolean
}) {
  const [current, setCurrent] = useState<ApprovalItem | null>(null)
  // `current` is the FRESH card a 409 stale_payload carried back. It replaces
  // what is on screen and says so — never a silent swap, and never a retry of
  // an answer given for a card that has moved.
  const shown = current ?? item
  return (
    <div className="card-body">
      {current && (
        <p className="warn-flag" data-changed="stale-payload">
          This card changed while you were reading it. What you see now is the current card, and your answer was not
          applied — read it again before deciding.
        </p>
      )}
      <CardContent item={shown} forms={forms} expanded={expanded === true} />
      <Acts
        item={shown}
        forms={forms}
        onAnswered={onAnswered}
        onStale={(fresh) => {
          setCurrent(fresh)
        }}
      />
    </div>
  )
}

/** CardContent dispatches on the SERVED kind. Everything it renders is text:
 *  card bodies are model-derived and web-derived content, so nothing here is
 *  ever markup (§41-B escape-by-default). */
function CardContent({
  item,
  forms,
  expanded,
}: {
  item: ApprovalItem
  forms: BenchmarkVerdictForms | null
  expanded: boolean
}) {
  switch (item.kind) {
    case 'ask':
      return <AskContent card={item.card} expanded={expanded} />
    case 'effect':
      return <EffectContent item={item} />
    case 'memory_conflict':
      return <ConflictContent card={item.card as MemoryConflict} />
    case 'benchmark_verdict':
      return <VerdictContent item={item} forms={forms} />
    default:
      return <RowCard card={item.card} />
  }
}

/**
 * RowCard renders a projection row's own fields as a definition list.
 *
 * The watchdog, drift, conformance and alarm cards are landed projection rows,
 * and their field sets are the producers' — so this prints the keys it is given
 * rather than a hand-written field list that would silently drop a key the day
 * a producer adds one. Values render as text; nested values render as their
 * JSON, which is honest about being structured data rather than pretending to
 * be prose.
 */
function RowCard({ card }: { card: unknown }) {
  if (!card || typeof card !== 'object') {
    return <Absent reason="this card carries no stored body" />
  }
  const entries = Object.entries(card as Record<string, unknown>)
  if (entries.length === 0) return <Absent reason="this card carries no stored body" />
  return (
    <dl className="card-face">
      {entries.map(([key, value]) => (
        <div key={key} data-field={key}>
          <dt>{key}</dt>
          <dd>{typeof value === 'string' ? value : JSON.stringify(value)}</dd>
        </div>
      ))}
    </dl>
  )
}

// ── the ask card: the S06.9 phone screen, the 13.5 help, Layer 2 ──────────

type HelpBlock = { what?: string; wrong?: string; recommend?: string }

type AskSnapshot = {
  kind?: string
  task_id?: string
  clearance?: number
  tier?: string
  questions?: { id: string; text: string; options?: { label: string; value: string }[] }[]
  decision?: { summary?: string; detail?: string[]; choices?: { label: string; value: string }[]; help?: HelpBlock }
  approval?: {
    layer1?: {
      restatement?: string
      deliverable?: string[]
      steps?: string[]
      will_not_do?: string[]
      assumptions?: { text: string; origin?: string }[]
      risks?: string[]
      cost_time?: string
      clearance?: number
      size_class?: string
      size_note?: string
      help?: HelpBlock
      uncovered?: string[]
      open_findings?: string[]
    }
    layer2?: {
      acs?: { n: number; plain: string; structured?: string; structured_kind?: string }[]
      steps?: { id: string; title: string; done_when?: string }[]
      coverage?: Record<string, string[]>
      estimate?: { size_class?: string; usd?: number; known?: boolean; basis?: string }
    }
  }
  delta?: { origin?: string; items?: { kind: string; target: string; old?: string; new?: string }[]; help?: HelpBlock }
}

function asSnapshot(card: unknown): AskSnapshot {
  return card && typeof card === 'object' ? (card as AskSnapshot) : {}
}

function AskContent({ card, expanded }: { card: unknown; expanded: boolean }) {
  const snap = asSnapshot(card)
  if (snap.approval) return <ApprovalCard snap={snap} expanded={expanded} />
  if (snap.delta) return <DeltaCard snap={snap} />
  if (snap.decision) return <DecisionCard snap={snap} />
  if (snap.questions && snap.questions.length > 0) return <InterviewCard snap={snap} />
  return <RowCard card={card} />
}

/** The S06.9 Stage-4 card: one phone screen, with the assumptions as its
 *  centerpiece, and Layer 2 expandable underneath. Every string is the stored
 *  snapshot's own — nothing here is re-drafted client-side, help text least of
 *  all (13.5: the help is registry- and pipeline-sourced and arrives IN the
 *  card). */
function ApprovalCard({ snap, expanded }: { snap: AskSnapshot; expanded: boolean }) {
  const l1 = snap.approval?.layer1 ?? {}
  const l2 = snap.approval?.layer2 ?? {}
  return (
    <div className="ask-card" data-card-kind={snap.kind ?? 'approval'}>
      <p className="restatement">{l1.restatement ?? <Absent reason="the card records no restatement" />}</p>
      <Bullets title="What you get" items={l1.deliverable} />
      <Bullets title="What I will do" items={l1.steps} ordered />
      <Bullets title="What I will NOT do" items={l1.will_not_do} />

      <h4>Assumptions</h4>
      {(l1.assumptions ?? []).length === 0 ? (
        <Absent reason="the card lists no assumptions" />
      ) : (
        <ul className="assumptions">
          {(l1.assumptions ?? []).map((a) => (
            <li key={a.text} data-assumption={a.text}>
              {a.text}
              {a.origin && <span className="muted"> ({a.origin})</span>}
            </li>
          ))}
        </ul>
      )}
      <Bullets title="Risks" items={l1.risks} />
      <Bullets title="Coverage gaps you accepted" items={l1.uncovered} />
      <Bullets title="Open findings" items={l1.open_findings} />

      <p className="muted">
        {l1.cost_time ?? 'no cost or time line on this card'}
        {l1.clearance !== undefined && <> · clearance {String(l1.clearance)}</>}
        {l1.size_class && <> · {l1.size_class}</>}
      </p>
      {l1.size_note && <p className="muted">{l1.size_note}</p>}
      <Help help={l1.help} />

      <Expandable title="The detail behind this" open={expanded}>
        <h5>Acceptance criteria</h5>
        <ol className="acs">
          {(l2.acs ?? []).map((ac) => (
            <li key={ac.n} data-ac={`AC-${String(ac.n)}`}>
              <span className="ac-key">AC-{String(ac.n)}</span> {ac.plain}
              {ac.structured && (
                <span className="muted">
                  {' '}
                  — {ac.structured} ({ac.structured_kind})
                </span>
              )}
            </li>
          ))}
        </ol>
        <h5>Steps</h5>
        <ol className="steps">
          {(l2.steps ?? []).map((s) => (
            <li key={s.id}>
              <span className="step-id">{s.id}</span> {s.title}
              {s.done_when && <span className="muted"> — done when {s.done_when}</span>}
              <span className="muted">
                {' '}
                covers{' '}
                {Object.entries(l2.coverage ?? {})
                  .filter(([, steps]) => steps.includes(s.id))
                  .map(([ac]) => ac)
                  .join(', ') || 'nothing listed'}
              </span>
            </li>
          ))}
        </ol>
        {l2.estimate && (
          <p className="muted" data-estimate={l2.estimate.known === true ? 'known' : 'unknown'}>
            {l2.estimate.known === true ? (
              <>
                estimate {l2.estimate.size_class} · USD {String(l2.estimate.usd)} · {l2.estimate.basis}
              </>
            ) : (
              <Absent reason={`no estimate: ${l2.estimate.basis ?? 'nothing comparable to estimate from'}`} />
            )}
          </p>
        )}
      </Expandable>
    </div>
  )
}

/** The post-approval delta card: exactly what changed against the frozen
 *  artifacts, in the producer's own ADDED / MODIFIED / REMOVED vocabulary. */
function DeltaCard({ snap }: { snap: AskSnapshot }) {
  const d = snap.delta ?? {}
  return (
    <div className="ask-card" data-card-kind={snap.kind ?? 'approval.delta'}>
      <p className="muted">what changed · {d.origin ?? 'origin not recorded'}</p>
      <ul className="delta-items">
        {(d.items ?? []).map((it) => (
          <li key={`${it.kind}:${it.target}`} data-delta={it.kind}>
            <span className="delta-kind">{it.kind}</span> <span className="delta-target">{it.target}</span>
            {it.old && <span className="muted"> — was: {it.old}</span>}
            {it.new && <span> — now: {it.new}</span>}
          </li>
        ))}
      </ul>
      <Help help={d.help} />
    </div>
  )
}

/** The S06.7 decision cards. Their choices are the card's own labelled options;
 *  the action ids the platform accepts are the option VALUES, which is what the
 *  served `actions` list carries. */
function DecisionCard({ snap }: { snap: AskSnapshot }) {
  const d = snap.decision ?? {}
  return (
    <div className="ask-card" data-card-kind={snap.kind ?? 'decision'}>
      <p className="restatement">{d.summary ?? <Absent reason="the card records no summary" />}</p>
      <Bullets title="What this is about" items={d.detail} />
      <Help help={d.help} />
    </div>
  )
}

/** The S06.5 batched option card. */
function InterviewCard({ snap }: { snap: AskSnapshot }) {
  return (
    <div className="ask-card" data-card-kind={snap.kind ?? 'interview'}>
      <ol className="questions">
        {(snap.questions ?? []).map((q) => (
          <li key={q.id} data-question={q.id}>
            {q.text}
          </li>
        ))}
      </ol>
    </div>
  )
}

/** The 13.5 help block, rendered where the person decides. The three sentences
 *  are the card's own — this file writes none of them. */
function Help({ help }: { help?: HelpBlock }) {
  if (!help || (!help.what && !help.wrong && !help.recommend)) {
    return <Absent reason="this card carries no help block" />
  }
  return (
    <dl className="help-block" data-help="13.5">
      <div>
        <dt>What this means</dt>
        <dd>{help.what}</dd>
      </div>
      <div>
        <dt>What could go wrong</dt>
        <dd>{help.wrong}</dd>
      </div>
      <div>
        <dt>What I recommend</dt>
        <dd>{help.recommend}</dd>
      </div>
    </dl>
  )
}

function Bullets({ title, items, ordered }: { title: string; items?: string[]; ordered?: boolean }) {
  if (!items || items.length === 0) return null
  const rows = items.map((item) => <li key={item}>{item}</li>)
  return (
    <>
      <h4>{title}</h4>
      {ordered === true ? <ol className="numbered">{rows}</ol> : <ul>{rows}</ul>}
    </>
  )
}

/** Expandable is Layer 2's disclosure. It is a native <details>, so it works
 *  with no JavaScript and reads correctly to a screen reader. */
function Expandable({ title, open, children }: { title: string; open: boolean; children: ReactNode }) {
  return (
    <details className="layer2" open={open}>
      <summary>{title}</summary>
      {children}
    </details>
  )
}

// ── the effect card and its D10 co-approval state ─────────────────────────

/** An effect proposal, with BOTH approver states rendered from the served
 *  `approvals` block. Who has signed is a server-side derivation over the
 *  decision log (cycle-scoped); this view renders it and infers nothing from
 *  frames (OQ8). */
function EffectContent({ item }: { item: ApprovalItem }) {
  const appr = item.approvals
  return (
    <div className="effect-card">
      <RowCard card={item.card} />
      {appr && (
        <dl className="co-approval" data-platform-level={appr.platform_level ? 'true' : 'false'}>
          <div>
            <dt>owner</dt>
            <dd data-signed={appr.owner_approved ? 'true' : 'false'}>
              {appr.owner_approved ? `signed by ${appr.owner_approved_by ?? 'the owner'}` : 'not signed'}
            </dd>
          </div>
          {appr.platform_level && (
            <div>
              <dt>operator</dt>
              <dd data-signed={appr.operator_approved ? 'true' : 'false'}>
                {appr.operator_approved ? `signed by ${appr.operator_approved_by ?? 'the operator'}` : 'not signed'}
              </dd>
            </div>
          )}
        </dl>
      )}
      {appr?.platform_level === true && (
        <p className="muted">
          This effect belongs to no single run, so it needs both the owner&apos;s and the operator&apos;s approval before
          it is approved.
        </p>
      )}
    </div>
  )
}

// ── the ninth kind ────────────────────────────────────────────────────────

function ConflictContent({ card }: { card: MemoryConflict | undefined }) {
  if (!card) return <Absent reason="this card carries no stored body" />
  return (
    <div className="conflict-card">
      <p className="restatement">{card.question}</p>
      <p className="muted">
        {card.topic_key && <>topic {card.topic_key} · </>}
        <Link to={hrefFor('inbox-item', { id: `memory_conflict:${String(card.conflict_id)}` })}>
          {card.entry_id}
        </Link>{' '}
        and {card.other_entry_id} · noticed <Stamp ts={card.detected_ts} />
      </p>
    </div>
  )
}

// ── the blind-pair verdict form ───────────────────────────────────────────

/**
 * The verdict card (BENCH-REG §3.2/§3.3/§3.4).
 *
 * The two responses render through the SERVED side-keyed fields and nothing
 * else: there is no arm, no position and no model on the wire before the record
 * exists, and this form fabricates none. The pair's own body comes from
 * `GET /api/benchmark/verdicts`, whose read is scoped to the caller — the card
 * in the inbox carries the projection row, and the two texts come from the form
 * read that exists for exactly this.
 */
function VerdictContent({ item, forms }: { item: ApprovalItem; forms: BenchmarkVerdictForms | null }) {
  const pairID = item.id.slice(item.id.indexOf(':') + 1)
  const pair = (forms?.pairs ?? []).find((p) => p.pair_id === pairID) ?? null

  return (
    <div className="verdict-card">
      <RowCard card={item.card} />
      {pair ? (
        <PairRenders pair={pair} />
      ) : (
        <Absent reason="the two responses are not on this card — a pair with nothing to compare cannot be voted on, only declined" />
      )}
      {forms && <p className="muted">{forms.detail}</p>}
    </div>
  )
}

/** The two responses, keyed by SIDE. Every field printed here is a field the
 *  served PendingPair carries — a test enumerates them against the fixture, so
 *  a field that could identify an arm cannot creep in. */
function PairRenders({ pair }: { pair: PendingPair }) {
  return (
    <div className="pair">
      {(['a', 'b'] as const).map((side) => (
        <div key={side} className="pair-side" data-side={side}>
          <h4>Response {side.toUpperCase()}</h4>
          <p className="muted">
            {String(side === 'a' ? pair.length_a : pair.length_b)} characters — reported, never corrected
          </p>
          <pre className="render">{side === 'a' ? pair.render_a : pair.render_b}</pre>
        </div>
      ))}
    </div>
  )
}

// ── the acts: rendered from the card's OWN vocabulary ──────────────────────

type ActState = {
  busy: boolean
  note: string
  detail: string
  failed: boolean
  /** The committed §14 record a verdict answer read back — the arm identities
   *  the post-record promise is about. Null on every other act. */
  reveal: VerdictReveal | null
}

const idle: ActState = { busy: false, note: '', detail: '', failed: false, reveal: null }

/**
 * Acts renders one control per SERVED action and nothing else.
 *
 * The dispatch below maps an action id to the landed verb that accepts it. It
 * invents no action: a card that names none gets no controls, and says so.
 */
function Acts({
  item,
  forms,
  onAnswered,
  onStale,
}: {
  item: ApprovalItem
  forms: BenchmarkVerdictForms | null
  onAnswered: () => void
  onStale: (fresh: ApprovalItem) => void
}) {
  const [state, setState] = useState<ActState>(idle)
  const [pin, setPin] = useState('')
  const [contest, setContest] = useState<string>('')
  const [reason, setReason] = useState('')
  const [verdict, setVerdict] = useState('')
  const [guess, setGuess] = useState('')
  const [criteria, setCriteria] = useState<string[]>([])
  const actions = item.actions ?? []

  const run = useCallback(
    async (label: string, fire: () => Promise<ActResult>) => {
      setState({ busy: true, note: `${label}…`, detail: '', failed: false, reveal: null })
      try {
        const out = await fire()
        setState({
          busy: false,
          note: out.note ?? label,
          detail: out.detail ?? '',
          failed: false,
          reveal: out.reveal ?? null,
        })
      } catch (err: unknown) {
        const fresh = staleCard(err)
        if (fresh) {
          setState(idle)
          onStale(fresh)
          return
        }
        setState({ busy: false, note: label, detail: describeError(err), failed: true, reveal: null })
      } finally {
        // The PIN exists only for the request it rode in. Clearing it here means
        // it is gone whether the answer applied, was refused, or failed.
        setPin('')
        onAnswered()
      }
    },
    [onAnswered, onStale],
  )

  if (!item.answerable) {
    return (
      <p className="acts" data-acts="read-only">
        <Absent reason={item.not_answerable_reason ?? 'this card is not yours to answer'} />
      </p>
    )
  }
  if (actions.length === 0) {
    return (
      <p className="acts" data-acts="none">
        <Absent reason="this card declares no answer verbs, so there is nothing here to press. It is waiting on somebody, and the platform has not said what closes it." />
      </p>
    )
  }

  const envelope = answerEnvelope(item)
  const needsPIN = item.step_up_required

  return (
    <div className="acts" data-acts="live">
      {envelope === 'contest' && (
        <label className="contest">
          <span>What is wrong (tap the criterion, assumption or step)</span>
          <input
            type="text"
            value={contest}
            data-field="contest"
            onChange={(e) => {
              setContest(e.currentTarget.value)
            }}
          />
        </label>
      )}
      {envelope === 'choice' && <CriteriaPicker item={item} picked={criteria} onPick={setCriteria} />}
      {acceptsReason.includes(item.kind) && (
        <label className="reason">
          <span>Why (recorded with your decision)</span>
          <input
            type="text"
            value={reason}
            data-field="reason"
            onChange={(e) => {
              setReason(e.currentTarget.value)
            }}
          />
        </label>
      )}
      {item.kind === 'benchmark_verdict' && actions.includes('verdict') && (
        <VerdictPicker forms={forms} verdict={verdict} guess={guess} onVerdict={setVerdict} onGuess={setGuess} />
      )}
      {item.kind === 'benchmark_alarm' && (
        <DispositionPicker forms={forms} verdict={verdict} onVerdict={setVerdict} reason={reason} onReason={setReason} />
      )}
      {needsPIN && (
        <label className="pin">
          <span>Your PIN (sent with this answer, kept nowhere)</span>
          <input
            type="password"
            value={pin}
            data-field="pin"
            autoComplete="off"
            onChange={(e) => {
              setPin(e.currentTarget.value)
            }}
          />
        </label>
      )}

      <div className="buttons">
        {actions.map((action) => (
          <button
            key={action}
            type="button"
            data-action={action}
            disabled={state.busy || !canFire(item, action, { verdict, guess })}
            onClick={() => {
              void run(action, () =>
                fireAction(item, action, {
                  pin,
                  reason,
                  contest,
                  verdict,
                  guess,
                  criteria,
                }),
              )
            }}
          >
            {action}
          </button>
        ))}
      </div>

      {state.note !== '' && (
        <p className={state.failed ? 'error' : 'notice'} data-outcome={state.failed ? 'failed' : 'applied'}>
          {state.note}
          {state.detail !== '' && <span className="muted"> — {state.detail}</span>}
        </p>
      )}
      {state.reveal && <Reveal reveal={state.reveal} />}
    </div>
  )
}

/** The kinds whose landed verb accepts an optional reason. It is a list of the
 *  VERBS' own bodies, not a guess: an effect answer, a flag suppression, a
 *  drift dismissal and a conformance acknowledgement each take one, and the
 *  alarm disposition carries its own beside the disposition it explains. */
const acceptsReason = ['effect', 'watchdog_flag', 'drift_card', 'conformance_card']

/**
 * The reveal, rendered.
 *
 * This is the one moment BENCH-REG §3.4 has been holding back: the record is
 * committed, so which blind side was the platform's — and both arms' observed
 * model identities — are readable at last. Announcing "revealed" and printing
 * nothing would withhold exactly the thing the announcement is about.
 *
 * `guess_correct` is the §5 blindness measurement, scored by the platform. It
 * renders as the served boolean; nothing here re-scores it.
 */
function Reveal({ reveal }: { reveal: VerdictReveal }) {
  return (
    <dl className="reveal" data-reveal={reveal.pair_id}>
      <div>
        <dt>the platform&apos;s side</dt>
        <dd data-reveal-side={reveal.platform_side}>{reveal.platform_side}</dd>
      </div>
      <div>
        <dt>your guess</dt>
        <dd data-guess-correct={reveal.guess_correct ? 'true' : 'false'}>
          {reveal.platform_guess} — {reveal.guess_correct ? 'right' : 'wrong'}
        </dd>
      </div>
      <div>
        <dt>the two arms</dt>
        <dd>
          platform {reveal.platform_model === '' ? <Absent reason="model not recorded" /> : reveal.platform_model} ·
          direct {reveal.direct_model === '' ? <Absent reason="model not recorded" /> : reveal.direct_model}
        </dd>
      </div>
      <div>
        <dt>recorded</dt>
        <dd>
          <Stamp ts={reveal.recorded_ts} /> <span className="muted">epoch {reveal.epoch_id}</span>
        </dd>
      </div>
    </dl>
  )
}

/** ActResult is what one act reports back: the outcome line, the server's own
 *  detail, and — for the one act that earns one — the committed record. */
type ActResult = { note?: string; detail?: string; reveal?: VerdictReveal }

/** conflictID reads the ninth kind's row number off its card, or null when the
 *  card carries none. Null is the honest answer: a surface that fell back to a
 *  zero would ask the server to resolve a row that is nobody's. */
function conflictID(item: ApprovalItem): number | null {
  const card = item.card as MemoryConflict | undefined
  const id = card?.conflict_id
  return typeof id === 'number' && Number.isFinite(id) && id > 0 ? id : null
}

/** canFire is the ONE structural gate this view applies, and it exists because
 *  BENCH-REG §3.3 is frozen: no verdict without a guess. The backend refuses a
 *  guess-less vote regardless — its constructor cannot express one — so this is
 *  the form agreeing with the rule, never the form enforcing it. */
function canFire(item: ApprovalItem, action: string, form: { verdict: string; guess: string }): boolean {
  if (item.kind === 'benchmark_verdict' && action === 'verdict') {
    return form.verdict !== '' && form.guess !== ''
  }
  if (item.kind === 'benchmark_alarm' && action === 'dispose') return form.verdict !== ''
  // A conflict card with no readable row number has nothing to resolve, so the
  // control does not arm rather than posting a guess at an id.
  if (item.kind === 'memory_conflict' && action === 'resolve') return conflictID(item) !== null
  return true
}

/**
 * answerEnvelope names which S06 answer shape a card's snapshot takes.
 *
 * It reads the STORED CARD to decide, because the card is what declares its own
 * family — `approval` and `delta` bodies take {action, contest}, the S06.7
 * decision bodies take {choice, criteria}, and a question card takes the slot
 * answers. A snapshot whose family this does not recognize gets `unknown`, and
 * the act path says so rather than guessing an envelope the pipeline would
 * refuse for reasons nobody could read.
 */
function answerEnvelope(item: ApprovalItem): 'action' | 'contest' | 'choice' | 'answers' | 'unknown' {
  if (item.kind !== 'ask') return 'action'
  const snap = asSnapshot(item.card)
  if (snap.approval) return 'contest'
  if (snap.delta) return 'contest'
  if (snap.decision) return 'choice'
  if (snap.questions && snap.questions.length > 0) return 'answers'
  return 'unknown'
}

type ActInput = {
  pin: string
  reason: string
  contest: string
  verdict: string
  guess: string
  criteria: string[]
}

/**
 * fireAction dispatches one served action to the landed verb that accepts it.
 *
 * Every branch is a verb that exists. There is no default that "tries
 * something": an action id this map does not know is reported as exactly that,
 * because pressing a button that silently posts to the wrong route would be
 * worse than a card that says the platform and this surface disagree.
 */
async function fireAction(item: ApprovalItem, action: string, input: ActInput): Promise<ActResult> {
  switch (item.kind) {
    case 'ask':
    case 'effect': {
      const answer = composeAnswer(item, action, input)
      if (answer === null) {
        return {
          note: 'not sent',
          detail:
            'this card family has no answer shape on this surface yet, so nothing was posted rather than posting a guess',
        }
      }
      const res = await api.answerApproval(item.id, {
        payload_hash: item.payload_hash ?? '',
        answer,
        ...(input.pin === '' ? {} : { pin: input.pin }),
      })
      return {
        note: res.applied ? `${action}: ${res.state}` : `already ${res.state} — nothing fired again`,
        detail: res.detail ?? '',
      }
    }
    case 'watchdog_flag': {
      if (action === 'resume') {
        // The SERVED outcome, like every other branch here. The first cut
        // printed a sentence of its own ("the flag was a false alarm") — a
        // judgement about why the run was released that the platform never
        // made and this surface has no standing to make.
        const res = await api.resumeRun(item.run_id ?? '')
        return { note: `resumed: ${res.from} → ${res.to}`, detail: res.detail }
      }
      const card = item.card as { run_id?: string; anomaly_class?: string } | undefined
      const res = await api.suppressFlag({
        ...(card?.run_id ? { run_id: card.run_id } : {}),
        anomaly_class: card?.anomaly_class ?? '',
        ...(input.reason === '' ? {} : { reason: input.reason }),
      })
      return { note: 'suppressed', detail: res.detail }
    }
    case 'drift_card': {
      const res = await api.dismissDrift(item.id, input.reason === '' ? undefined : input.reason)
      return { note: 'dismissed', detail: res.detail }
    }
    case 'conformance_card': {
      const res = await api.acknowledgeConformance(item.id, input.reason === '' ? undefined : input.reason)
      return {
        note: res.still_red ? 'acknowledged — STILL RED' : 'acknowledged',
        detail: res.detail,
      }
    }
    case 'benchmark_verdict': {
      if (action === 'decline') {
        const res = await api.declineVerdict(item.id)
        return { note: 'declined', detail: res.detail }
      }
      const res = await api.recordVerdict(item.id, input.verdict, input.guess)
      // The reveal TRAVELS. Saying "revealed" and rendering nothing would make
      // the surface announce the one thing §3.4 withholds until now and then
      // withhold it anyway.
      return {
        note: res.reveal ? 'recorded, and revealed' : 'recorded',
        detail: res.detail,
        reveal: res.reveal,
      }
    }
    case 'benchmark_alarm': {
      const res = await api.disposeAlarm(item.id, input.verdict, input.reason === '' ? undefined : input.reason)
      return { note: `dispositioned: ${res.disposition}`, detail: res.detail }
    }
    case 'memory_conflict': {
      const conflict = conflictID(item)
      if (conflict === null) {
        // Never post a zero-value guess at an id. A card with no readable
        // conflict id is a card this surface cannot answer, and saying so is
        // the honest outcome — posting `0` would ask the server about a row
        // that is nobody's.
        return {
          note: 'not sent',
          detail: 'this card carries no conflict id, so there is nothing to resolve — re-read the inbox',
        }
      }
      const res = await api.resolveMemoryConflict(conflict)
      return { note: 'resolved', detail: res.detail }
    }
  }
  return {
    note: 'not sent',
    detail: `this surface knows no verb for a ${item.kind} card, so nothing was posted`,
  }
}

/** composeAnswer builds the answer in the CARD's own vocabulary. null means the
 *  card's family has no shape here, which the caller renders as the honest
 *  nothing-was-sent rather than posting a guess. */
function composeAnswer(item: ApprovalItem, action: string, input: ActInput): unknown {
  if (item.kind === 'effect') {
    return input.reason === '' ? { action } : { action, reason: input.reason }
  }
  switch (answerEnvelope(item)) {
    case 'contest':
      // The S06.9 structured Re-plan entry: the contest names the criterion,
      // assumption or step being contested, and rides the same answer.
      return input.contest === '' ? { action } : { action, contest: { target: input.contest } }
    case 'choice':
      return input.criteria.length === 0 ? { choice: action } : { choice: action, criteria: input.criteria }
    case 'answers':
      // A question card's answer is its slot answers, and this surface offers
      // no slot editor — so it composes only the one shape it can honestly
      // compose, and ONLY for the action that means it. Sending force-proceed
      // for any served action would answer a question the person did not
      // answer, under a verb they did press.
      return action === 'force_proceed' ? { force_proceed: true } : null
    default:
      return null
  }
}

/** CriteriaPicker offers the decision card's OWN listed items. Some choices
 *  (dropping a criterion) name which item they are about, and the card is where
 *  the list of candidates comes from. */
function CriteriaPicker({
  item,
  picked,
  onPick,
}: {
  item: ApprovalItem
  picked: string[]
  onPick: (next: string[]) => void
}) {
  const detail = asSnapshot(item.card).decision?.detail ?? []
  if (detail.length === 0) return null
  return (
    <fieldset className="criteria">
      <legend>Which of these (where the choice needs one)</legend>
      {detail.map((d) => (
        <label key={d}>
          <input
            type="checkbox"
            data-criterion={d}
            checked={picked.includes(d)}
            onChange={(e) => {
              onPick(e.currentTarget.checked ? [...picked, d] : picked.filter((p) => p !== d))
            }}
          />
          {d}
        </label>
      ))}
    </fieldset>
  )
}

/** The verdict form's two mandatory pickers. Both vocabularies are SERVED —
 *  this file declares neither, because both are frozen registered text whose
 *  only home is the package that owns the registration (OQ4). */
function VerdictPicker({
  forms,
  verdict,
  guess,
  onVerdict,
  onGuess,
}: {
  forms: BenchmarkVerdictForms | null
  verdict: string
  guess: string
  onVerdict: (v: string) => void
  onGuess: (g: string) => void
}) {
  const choices = forms?.choices ?? []
  const sides = forms?.guess_sides ?? []
  if (choices.length === 0 || sides.length === 0) {
    return <Absent reason="the verdict vocabulary has not been served, so this form offers no buttons of its own" />
  }
  return (
    <div className="verdict-form">
      <fieldset data-form="verdict">
        <legend>Which response is better</legend>
        {choices.map((c) => (
          <label key={c}>
            <input
              type="radio"
              name="verdict"
              data-verdict={c}
              checked={verdict === c}
              onChange={() => {
                onVerdict(c)
              }}
            />
            {c}
          </label>
        ))}
      </fieldset>
      <fieldset data-form="guess">
        <legend>Which one do you think was the platform&apos;s? (required — every vote carries a guess)</legend>
        {sides.map((s) => (
          <label key={s}>
            <input
              type="radio"
              name="guess"
              data-guess={s}
              checked={guess === s}
              onChange={() => {
                onGuess(s)
              }}
            />
            {s}
          </label>
        ))}
      </fieldset>
    </div>
  )
}

/** The §12 alarm dispositions, likewise served rather than declared. */
function DispositionPicker({
  forms,
  verdict,
  onVerdict,
  reason,
  onReason,
}: {
  forms: BenchmarkVerdictForms | null
  verdict: string
  onVerdict: (v: string) => void
  reason: string
  onReason: (r: string) => void
}) {
  const dispositions = forms?.dispositions ?? []
  if (dispositions.length === 0) {
    return <Absent reason="the disposition vocabulary has not been served, so this form offers no buttons of its own" />
  }
  return (
    <div className="disposition-form">
      <fieldset data-form="disposition">
        <legend>How you are disposing of this alarm</legend>
        {dispositions.map((d) => (
          <label key={d}>
            <input
              type="radio"
              name="disposition"
              data-disposition={d}
              checked={verdict === d}
              onChange={() => {
                onVerdict(d)
              }}
            />
            {d}
          </label>
        ))}
      </fieldset>
      <label className="reason">
        <span>Why (recorded with the disposition)</span>
        <input
          type="text"
          value={reason}
          data-field="reason"
          onChange={(e) => {
            onReason(e.currentTarget.value)
          }}
        />
      </label>
    </div>
  )
}

// ── the Low-tier batch ─────────────────────────────────────────────────────

/**
 * "One action answers a selected set" (S15.6), constrained the way OQ10
 * dispositioned it: the person picks the ACTION first, and only cards whose own
 * vocabulary carries that action become selectable. A mixed selection is
 * therefore impossible rather than silently split — every item in the request
 * carries its OWN payload hash and its own answer, and one refusal leaves its
 * siblings applied.
 */
function BatchBar({ items, onAnswered }: { items: ApprovalItem[]; onAnswered: () => void }) {
  const [action, setAction] = useState('')
  const [picked, setPicked] = useState<string[]>([])
  const [outcomes, setOutcomes] = useState<ApprovalBatchOutcome[] | null>(null)
  const [failure, setFailure] = useState('')

  const batchable = items.filter((i) => i.batchable)
  const offered = [...new Set(batchable.flatMap((i) => i.actions ?? []))].sort()
  const eligible = action === '' ? [] : batchable.filter((i) => (i.actions ?? []).includes(action))
  const selected = eligible.filter((i) => picked.includes(i.id))

  if (batchable.length === 0) return null

  const send = async () => {
    setFailure('')
    try {
      const res = await api.answerApprovalBatch(
        selected.map((i) => ({
          id: i.id,
          payload_hash: i.payload_hash ?? '',
          answer: composeAnswer(i, action, { pin: '', reason: '', contest: '', verdict: '', guess: '', criteria: [] }),
        })),
      )
      setOutcomes(res.outcomes)
      setPicked([])
    } catch (err: unknown) {
      setFailure(describeError(err))
    } finally {
      onAnswered()
    }
  }

  return (
    <div className="batch-bar" data-batchable={String(batchable.length)}>
      <p className="muted">
        {String(batchable.length)} low-risk card(s) can be answered together. Pick the one action first — a batch
        answers cards that all accept it, and nothing else.
      </p>
      <label>
        <span>Action</span>
        <select
          value={action}
          data-field="batch-action"
          onChange={(e) => {
            setAction(e.currentTarget.value)
            setPicked([])
            setOutcomes(null)
          }}
        >
          <option value="">choose an action</option>
          {offered.map((a) => (
            <option key={a} value={a}>
              {a}
            </option>
          ))}
        </select>
      </label>
      {action !== '' && (
        <ul className="batch-candidates">
          {eligible.map((i) => (
            <li key={i.id}>
              <label>
                <input
                  type="checkbox"
                  data-batch-pick={i.id}
                  checked={picked.includes(i.id)}
                  onChange={(e) => {
                    setPicked(e.currentTarget.checked ? [...picked, i.id] : picked.filter((p) => p !== i.id))
                  }}
                />
                {i.id}
              </label>
            </li>
          ))}
        </ul>
      )}
      {action !== '' && (
        <button
          type="button"
          data-action="answer-batch"
          disabled={selected.length === 0}
          onClick={() => {
            void send()
          }}
        >
          {action} {String(selected.length)} card(s)
        </button>
      )}
      {failure !== '' && <p className="error">{failure}</p>}
      {outcomes && (
        <ul className="batch-outcomes">
          {outcomes.map((o) => (
            <li key={o.id} data-outcome-id={o.id} data-outcome-status={String(o.status)}>
              {o.id} — {String(o.status)}
              {o.code && <span className="warn-flag"> {o.code}</span>}
              {o.detail && <span className="muted"> {o.detail}</span>}
              {o.result && <span className="muted"> {o.result.detail}</span>}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
