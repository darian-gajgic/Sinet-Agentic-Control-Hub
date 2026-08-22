import { ChevronDown } from 'lucide-react'
import { useCallback, useEffect, useRef, useState, type ReactNode, type Ref } from 'react'

import {
  api,
  type VerdictReveal,
  staleCard,
  type ApprovalBatchOutcome,
  type ApprovalItem,
  type ApprovalList,
  type BenchmarkVerdictForms,
  type IntakeUnderstood,
  type MemoryConflict,
  type PendingPair,
  type TaskList,
} from './api'
import { ActConfirm, OutcomeLine, outcomeOf, useAct } from './controls'
import { UnderstoodPanel } from './Intake'
import type { EventStream } from './events'
import { describeError, inboxEventTypes, useLive, type Live } from './live'
import { useProjectScope } from './project'
import { Absent, Freshness, Owner, Stamp, SurfaceHead } from './parts'
import { Link, navigate } from './router'
import { hrefFor } from './routes'
import { reconcileBadge } from './push'
import { useSessionInfo } from './session'
import { Button, Chip, EmptyState, Panel, Timestamp, type Tone } from './ui'

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
 * THE SERVER RANKS; THE CLIENT MAY NARROW AND RE-ORDER, SAYING SO. The landed
 * posture here was "never a filter" — OVERRULED by the operator at the B6 gate
 * (P3/design/b6-gate-clickthrough-findings-2026-08-22.md: finding 1). Filtering
 * by project/task and a reading order are PRESENTATION over the served list
 * (S15.12): the served order stays the default, an active filter states plainly
 * what it is hiding, every served card stays reachable, and nothing is ever
 * silently dropped — a kind this file has never heard of still gets a row.
 *
 * NOTHING IS COMPUTED THAT WAS NOT SERVED. No percentage, no fraction, no
 * estimate of anything. The expiry line is a display derivation of a served
 * INSTANT and nothing else; when no expiry is served, none is shown. The
 * project on a row is a client-side JOIN of the card's served `task_id`
 * against the served tasks read — never a guess, and a card that resolves to
 * no task files under the honest "(no project)" bucket (api.ts:156 precedent).
 *
 * THE PIN LIVES ONLY IN THE REQUEST. A High-tier answer collects the actor's
 * PIN, sends it in the SAME request the answer rides (S01.9 verify-at-act), and
 * clears it. It is never stored, never kept in state past the submit, and there
 * is no elevation to inherit.
 *
 * THE FACE IS FOR FINDING; THE DETAIL IS FOR DECIDING (gate findings 2/3/5).
 * A row's face carries what project · what task · the issue in plain words,
 * with the class and tier as chips. Pressing it opens the full detail — the
 * issue, what has to be decided, what that decision needs, what the platform
 * recommends, then the verbs. Raw projection internals (row id, change class,
 * fingerprint, seq) live only inside the collapsed "Technical details" fold.
 *
 * THE QUEUE IS WHAT NEEDS *YOU*; EVERYTHING ELSE STANDS ASIDE (gate round 2,
 * P3/design/b6-gate-operator-findings-r2-2026-08-22.md). The main list and the
 * sidebar badge count only the cards the signed-in person can act on. Notice-
 * class cards — the platform read something for you and recorded it — live in
 * their own collapsed drawer, grouped by source, each clearable with the
 * served dismiss verb. Cards someone ELSE must answer sit in a second drawer.
 * Nothing is dropped: every served card stays on this surface and reachable —
 * the split is presentation over the one served list.
 *
 * EVERY ANSWERABLE CARD CARRIES A WORKING DOOR (round-2 bug). An intake
 * question card (interview / clarification / escalation / the family
 * question) declares no approvals-verb actions — its real answering surface
 * is the give-work journey at /new?task=<id>, which resumes the interview.
 * Those cards render that door instead of the honest "nothing here to press"
 * sentence, which remains only for a kind that genuinely has no route.
 */
// ── the round-2 split: what needs you · notices · waiting on others ────────

/**
 * The notice class (gate r2 order §2): cards that INFORM and gate nothing.
 * `drift_card` is the one landed kind of the class — the platform noticed an
 * outside-world change while watching a source; reading it (and clearing it
 * with the served dismiss) is the whole act, and no work is paused on it.
 * Everything else in the queue asks for a decision, an answer or a sign-off.
 */
const noticeKinds = ['drift_card']

export function isNotice(item: Pick<ApprovalItem, 'kind'>): boolean {
  return noticeKinds.includes(item.kind)
}

/** needsYou — the card asks THIS person to decide, answer or sign off. The
 *  main queue and the sidebar badge are exactly this set (r2 order §1): a
 *  notice is not a demand, and a card someone else must answer is not yours. */
export function needsYou(item: Pick<ApprovalItem, 'kind' | 'answerable'>): boolean {
  return item.answerable && !isNotice(item)
}

/**
 * The intake question-class card kinds (internal/intake/cards.go vocabulary):
 * the cards whose real answering surface is the give-work journey at
 * /new?task=<id> — option chips, free text, Send (verified live, r2 finding 1).
 * `interview` and `decision.family` are the gate-named two; `clarification`
 * and `escalation` are the same class (question cards answered through the
 * intake answer route, not the approvals verb) and dead-end identically
 * without the door.
 */
const intakeQuestionKinds = ['interview', 'clarification', 'escalation', 'decision.family']

/** intakeResumeHref — the working door for an open intake ask, or null when
 *  the card is not one. Null never renders a door: absence invents nothing. */
export function intakeResumeHref(item: Pick<ApprovalItem, 'kind' | 'task_id' | 'card'>): string | null {
  if (item.kind !== 'ask' || item.task_id === undefined || item.task_id === '') return null
  const snapKind = asSnapshot(item.card).kind ?? ''
  if (!intakeQuestionKinds.includes(snapKind)) return null
  return `${hrefFor('new')}?task=${encodeURIComponent(item.task_id)}`
}

export function Inbox({ stream, search = '' }: { stream?: EventStream; search?: string }) {
  const live = useLive<ApprovalList>({
    key: '/api/approvals',
    read: () => api.approvals(),
    types: inboxEventTypes,
    stream,
  })
  const items = live.data?.items ?? []
  const join = useTaskJoin(stream)
  const scope = useProjectScope()
  const q = inboxQueryFromSearch(search)
  // The badge is reconciled from the SERVED list, not recomputed per view: it
  // is the needs-you count over the whole served list — never the filtered
  // one, because a filter is presentation and the badge is the platform's own
  // claim about what waits (B6-9 OQ5). Counting only what needs the person is
  // the r2 gate order §1: a badge of 72 over a queue of notices is exactly the
  // glance disagreement OQ5 exists to prevent. Without the reconcile the badge
  // holds its last PUSHED value until the next cadence, so answering a card
  // would leave the home screen claiming work that is done (drain r1, D2).
  const needsMe = items.filter(needsYou).length
  useEffect(() => {
    if (live.data !== null) reconcileBadge(needsMe)
  }, [live.data, needsMe])

  const project = effectiveProject(q, scope.project)
  // The join answers the project filter. While it is still in flight the cards
  // cannot honestly be bucketed, so a project-filtered view waits for it; a
  // join that FAILED leaves the filter unapplied and says so, which is honester
  // than hiding cards on a guess.
  const joinPending = project !== '' && join.data === null && join.error === ''
  const joinFailed = project !== '' && join.data === null && join.error !== ''
  const wheres = new Map(items.map((i) => [i.id, whereOf(i, join)]))
  const shown = filterItems(items, wheres, joinFailed ? { ...q, project: '' } : q, joinFailed ? '' : scope.project)
  const ordered = orderItems(shown, q.sort)
  // The r2 split, applied AFTER the filter and the reading order so all three
  // sections agree with the controls above them. It is a partition of the one
  // served list: every card lands in exactly one section, nothing is dropped.
  const queue = ordered.filter(needsYou)
  const notices = ordered.filter(isNotice)
  const waiting = ordered.filter((i) => !needsYou(i) && !isNotice(i))

  return (
    <section className="surface inbox">
      {/* The D8 "what this is" line, updated with the gate-ordered powers this
          page now genuinely has: the served order is the default, narrowing and
          re-ordering are presentation, and a narrowed view declares itself. */}
      <SurfaceHead
        title="Inbox"
        what="What needs you, in one queue — served in the control plane's own order. Notices the platform read for you sit in their own drawer below the queue, and cards waiting on somebody else sit under those; nothing is dropped. You can narrow the page to one project or one task and change the reading order; that changes what it shows, never what is served, and a narrowed view says what it is hiding. Answering a card releases the work that was paused on it."
      />
      <Freshness stale={live.stale} error={live.error} hasData={live.data !== null} />
      <NoFrameNote items={items} />
      {live.data && items.length === 0 ? (
        <EmptyState
          what="Nothing is waiting on you."
          why="Cards arrive here on their own: a plan waiting for approval, a question a run cannot answer for itself, an outward effect needing a signature, a watchdog or drift finding to sign off, a blind comparison to judge. Answering one clears it from this queue."
        />
      ) : (
        <>
          {live.data !== null && (
            <InboxControls items={items} wheres={wheres} q={q} scopeProject={scope.project} join={join} shown={shown.length} />
          )}
          {joinFailed && (
            <p className="max-w-prose text-sm text-[var(--yellow)]" data-inbox-join="failed">
              The project filter is NOT applied — the task list that says which project each card belongs to could not
              be read ({join.error}). Every card is shown instead.
            </p>
          )}
          {joinPending ? (
            <p className="max-w-prose text-sm text-muted-foreground" data-inbox-join="pending">
              Matching cards to their projects…
            </p>
          ) : (
            <>
              {live.data !== null && shown.length === 0 && (
                <EmptyState
                  what="No card matches the filter."
                  why={`${String(items.length)} card${items.length === 1 ? ' is' : 's are'} hidden by it — nothing is dropped. Clear the filter above to see ${items.length === 1 ? 'it' : 'them all'}.`}
                />
              )}
              {/* The calm queue (r2 order §1): zero actionable cards is said
                  plainly even while notices or other people's cards exist —
                  those sit in their drawers below and demand nothing. */}
              {live.data !== null && shown.length > 0 && queue.length === 0 && (
                <EmptyState
                  what="Nothing needs you right now."
                  why={[
                    notices.length > 0
                      ? `${String(notices.length)} notice${notices.length === 1 ? '' : 's'} — things the platform read and recorded for you — sit${notices.length === 1 ? 's' : ''} in the drawer below`
                      : '',
                    waiting.length > 0
                      ? `${String(waiting.length)} card${waiting.length === 1 ? ' waits' : 's wait'} on somebody else`
                      : '',
                  ]
                    .filter((s) => s !== '')
                    .join(', and ')
                    .concat('. None of it needs a decision from you.')}
                />
              )}
              <BatchBar items={queue} onAnswered={live.reload} />
              <WithForms items={ordered} stream={stream}>
                {(forms) => (
                  <>
                    {queue.length > 0 && (
                      <ol className="cards">
                        {queue.map((item) => (
                          <InboxRow
                            key={item.id}
                            item={item}
                            where={wheres.get(item.id) ?? { project: null, task: item.task_id ?? null, note: '' }}
                            forms={forms}
                            onAnswered={live.reload}
                          />
                        ))}
                      </ol>
                    )}
                    <NoticesDrawer items={notices} wheres={wheres} forms={forms} onAnswered={live.reload} />
                    <WaitingDrawer items={waiting} wheres={wheres} forms={forms} onAnswered={live.reload} />
                  </>
                )}
              </WithForms>
            </>
          )}
        </>
      )}
      {live.data?.truncated && (
        <p className="max-w-prose text-sm text-muted-foreground">
          This is one page of a longer queue — the control plane bounds what one read returns. Answer some and re-read.
        </p>
      )}
      {/* Return-visit item 5: alice's own acceptance was nowhere in
          waiting-on-you while thirteen alien cards were. Finished work waiting
          on the CALLER'S review is served by the deliverables read; it renders
          here as its own section — after the card queue (whose order is the
          control plane's and stays untouched), before the opt-in pitch. */}
      <ReviewWaiting stream={stream} />
      {/* RA-10 (third report: W2-10, RA-10): the OPT-IN PITCH renders after
          the mail, never above it. Earlier fixes softened the page's promise
          but left the panel physically first, so the queue still greeted a
          person with an essay before their urgent cards. The door stays on the
          page for everyone (gating it on already-having-a-pair would hide the
          way in); it just stops standing in front of the queue. */}
      <BenchmarkOptIn stream={stream} />
    </section>
  )
}

// ── the notices drawer: informational cards, grouped by source ─────────────

/** One source's standing notices, in served order. The group KEY is the served
 *  `source` string verbatim; the honest bucket catches a card that names none. */
type NoticeGroupData = { source: string; items: ApprovalItem[] }

const noSource = '(no source named)'

function groupBySource(items: ApprovalItem[]): NoticeGroupData[] {
  const bySource = new Map<string, ApprovalItem[]>()
  for (const item of items) {
    const source = readString(item.card, 'source') ?? noSource
    const list = bySource.get(source) ?? []
    list.push(item)
    bySource.set(source, list)
  }
  return [...bySource.entries()].map(([source, grouped]) => ({ source, items: grouped }))
}

/**
 * sourceLabel — the served source string in plain words (r2 order §2: "grouped
 * by source in plain words"). A display DERIVATION of the one served string,
 * the same class of move as `describeSpan` over a served instant: an http(s)
 * URL renders as its host plus the path segments that name what is watched
 * (a feed filename is dropped — it names a format, not a subject), plus the
 * `q` search term where one exists, because that is what distinguishes two
 * feeds on one host. The platform's own self-check watches name themselves
 * "conformance:<suite>" (internal/watchlist/canary_conformance.go) — a second
 * known shape, rendered in plain words (GF2 F6). Anything else renders
 * verbatim — never a guess at what it means. The raw string stays on the
 * group.
 */
export function sourceLabel(source: string): string {
  const selfCheck = /^conformance:(.+)$/.exec(source)
  if (selfCheck !== null) return `platform self-check · ${selfCheck[1]}`
  let u: URL
  try {
    u = new URL(source)
  } catch {
    return source
  }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return source
  const host = u.hostname.replace(/^www\./, '')
  const segs = u.pathname.split('/').filter((s) => s !== '')
  if (segs.length > 0 && /\.(atom|rss|xml|json)$/i.test(segs[segs.length - 1])) segs.pop()
  const path = segs.slice(0, 2).join('/')
  const q = u.searchParams.get('q') ?? ''
  const base = path === '' ? host : `${host} · ${path}`
  return q === '' ? base : `${base} · “${q}”`
}

/**
 * The collapsed notices drawer (r2 order §2/§3). Everything the platform read
 * for you and recorded, OUT of the main queue but never out of reach: one
 * group per watched source, a count each, the cards whole inside. Clearing is
 * the SERVED per-card dismiss — surfaced in plain words per card, and as a
 * per-group clear-all that fires the same served verb once per card,
 * sequentially, with visible progress. No new wire verb exists here.
 */
function NoticesDrawer({
  items,
  wheres,
  forms,
  onAnswered,
}: {
  items: ApprovalItem[]
  wheres: Map<string, Where>
  forms: BenchmarkVerdictForms | null
  onAnswered: () => void
}) {
  if (items.length === 0) return null
  const groups = groupBySource(items)
  return (
    <details className="side-drawer" data-notices={String(items.length)}>
      <summary>
        <span className="drawer-title">Notices</span>
        <span className="drawer-count">{String(items.length)}</span>
        <span className="drawer-sub">
          things the platform noticed while watching outside sources — nothing here needs a decision
        </span>
      </summary>
      <p className="max-w-prose text-sm text-muted-foreground">
        Each notice records that something the platform depends on changed outside it. Reading one and pressing
        &ldquo;Got it — clear this&rdquo; records that you saw it; nothing outside this page changes, and the watch
        itself keeps running.
      </p>
      {groups.map((g) => (
        <NoticeGroup key={g.source} group={g} wheres={wheres} forms={forms} onAnswered={onAnswered} />
      ))}
    </details>
  )
}

/** One source's group: the plain-words label, the count, the served cards
 *  expandable inside, and the sequential clear-all over the served verb. */
function NoticeGroup({
  group,
  wheres,
  forms,
  onAnswered,
}: {
  group: NoticeGroupData
  wheres: Map<string, Where>
  forms: BenchmarkVerdictForms | null
  onAnswered: () => void
}) {
  return (
    <details className="notice-group" data-notice-source={group.source}>
      <summary>
        <span className="drawer-title">{sourceLabel(group.source)}</span>
        <span className="drawer-count">{String(group.items.length)}</span>
      </summary>
      {group.source !== noSource && (
        <p className="m-0 mb-2 font-mono text-[11px] text-muted-foreground wrap-anywhere" data-notice-raw-source>
          watching {group.source}
        </p>
      )}
      <ClearAll items={group.items} onAnswered={onAnswered} />
      <ol className="cards">
        {group.items.map((item) => (
          <InboxRow
            key={item.id}
            item={item}
            where={wheres.get(item.id) ?? { project: null, task: item.task_id ?? null, note: '' }}
            forms={forms}
            onAnswered={onAnswered}
            quietWhere
          />
        ))}
      </ol>
    </details>
  )
}

/** The clear-all progress: how far the sequence got, and the honest stop. */
type ClearRun = { done: number; total: number; error: string; finished: boolean }

/**
 * ClearAll issues the SERVED per-card dismiss once per clearable card in the
 * group, sequentially, narrating progress (r2 order §3: presentation over
 * served actions, no new wire verbs). Only cards that are answerable AND
 * declare the dismiss verb are cleared — the card stays the authority, so a
 * group of somebody else's notices offers no button. A refusal mid-sequence
 * stops the run and says which card refused; the cards already cleared stay
 * cleared, which the re-read shows honestly.
 */
function ClearAll({ items, onAnswered }: { items: ApprovalItem[]; onAnswered: () => void }) {
  const [run, setRun] = useState<ClearRun | null>(null)
  const clearable = items.filter((i) => i.answerable && (i.actions ?? []).includes('dismiss'))
  if (clearable.length === 0) return null
  const busy = run !== null && !run.finished

  const clearAll = async () => {
    setRun({ done: 0, total: clearable.length, error: '', finished: false })
    let done = 0
    for (const item of clearable) {
      try {
        await api.dismissDrift(item.id)
        done++
        setRun({ done, total: clearable.length, error: '', finished: false })
      } catch (err: unknown) {
        setRun({ done, total: clearable.length, error: describeError(err), finished: true })
        onAnswered()
        return
      }
    }
    setRun({ done, total: clearable.length, error: '', finished: true })
    onAnswered()
  }

  return (
    <div className="clear-all" data-clear-all={String(clearable.length)}>
      <Button
        variant="secondary"
        size="sm"
        data-action="clear-group"
        disabled={busy}
        onClick={() => {
          void clearAll()
        }}
      >
        {busy ? `Clearing ${String(run.done + 1)} of ${String(run.total)}…` : `Got it — clear all ${String(clearable.length)}`}
      </Button>
      {run !== null && run.finished && run.error === '' && (
        <span className="text-sm text-[var(--green)]" data-clear-outcome="done">
          Cleared {String(run.done)} of {String(run.total)} — each recorded as read by you.
        </span>
      )}
      {run !== null && run.error !== '' && (
        <span className="text-sm text-[var(--red)]" data-clear-outcome="failed">
          Cleared {String(run.done)} of {String(run.total)}, then one refused: {run.error}. The rest stay listed.
        </span>
      )}
    </div>
  )
}

/**
 * The second drawer (r2 order §1's complement): cards this person can SEE but
 * not answer — somebody else's decisions, kept reachable because a shared
 * household reads over each other's shoulders, but never counted as yours.
 * Each card's detail carries its served not-answerable reason.
 */
function WaitingDrawer({
  items,
  wheres,
  forms,
  onAnswered,
}: {
  items: ApprovalItem[]
  wheres: Map<string, Where>
  forms: BenchmarkVerdictForms | null
  onAnswered: () => void
}) {
  if (items.length === 0) return null
  return (
    <details className="side-drawer" data-waiting-drawer={String(items.length)}>
      <summary>
        <span className="drawer-title">Waiting on somebody else</span>
        <span className="drawer-count">{String(items.length)}</span>
        <span className="drawer-sub">cards you can see but not answer — each says whose they are</span>
      </summary>
      <ol className="cards">
        {items.map((item) => (
          <InboxRow
            key={item.id}
            item={item}
            where={wheres.get(item.id) ?? { project: null, task: item.task_id ?? null, note: '' }}
            forms={forms}
            onAnswered={onAnswered}
          />
        ))}
      </ol>
    </details>
  )
}

/**
 * Finished work waiting on the CALLER's review (return-visit item 5).
 *
 * "Waiting on you" was the approvals queue alone, so an owner's own finished
 * deliverable — the thing the whole journey exists to hand over — never
 * appeared while a dozen platform cards did. The deliverables read serves the
 * in-review set; the rows OWNED BY the caller are exactly the ones waiting on
 * nobody else (D10: the review is the owner's), so those render here with a
 * door each. Rows owned by others are not "waiting on you" and are not shown.
 * Nothing here re-orders or filters the card queue above — this is a second
 * served feed, rendered as its own section.
 */
function ReviewWaiting({ stream }: { stream?: EventStream }) {
  const session = useSessionInfo()
  const me = session?.user?.user_id ?? ''
  const live = useLive({
    key: `/api/deliverables?state=in-review#inbox:${me}`,
    read: () => api.deliverables({ state: 'in-review' }),
    types: inboxEventTypes,
    stream,
  })
  const mine = (live.data?.deliverables ?? []).filter((d) => me !== '' && d.owner === me)
  // Nothing to say until the read lands (§51 served-gating), and nothing to
  // say when nothing waits — an empty section would out-shout the queue.
  if (live.data === null || mine.length === 0) return null
  return (
    <div className="review-waiting mt-4" data-review-waiting={String(mine.length)}>
      <h3 className="mt-0 mb-1">Finished work waiting for your review</h3>
      <p className="m-0 mb-2 text-sm text-muted-foreground">
        {mine.length === 1 ? 'One piece of finished work waits' : `${String(mine.length)} pieces of finished work wait`}{' '}
        on you — nobody else reviews your work. Open one to see it, accept it, or ask for changes.
      </p>
      <ul className="m-0 flex list-none flex-col gap-1 ps-0">
        {mine.map((d) => (
          <li key={d.deliverable_id} className="text-sm" data-waiting-review={d.deliverable_id}>
            <Link to={hrefFor('deliverable', { id: d.deliverable_id })}>{d.deliverable_id}</Link>{' '}
            <span className="text-xs text-muted-foreground">
              {d.type} · from <Link to={hrefFor('task', { id: d.task_id })}>{d.task_id}</Link> · revision{' '}
              {String(d.current_revision)}
            </span>
          </li>
        ))}
      </ul>
    </div>
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
  // moment the drill checks the badge against the queue. Same count as the
  // queue's own: what needs THIS person (r2 order §1).
  const needsMe = all.filter(needsYou).length
  useEffect(() => {
    if (live.data !== null) reconcileBadge(needsMe)
  }, [live.data, needsMe])

  return (
    <section className="surface inbox">
      {/* The deep-link surface teaches what its URL IS, because that is the one
          thing this page has and the queue does not: a stable address a push
          notification's `navigate` field points at (S15.11). It renders the same
          card the queue renders — one component serves both. */}
      <SurfaceHead
        title="Approval"
        what="One decision, at its own stable address — the link a notification opens. It shows exactly what the queue shows for this card, expanded."
        aside={
          <Link to={hrefFor('inbox')} className="text-sm">
            ← the whole queue
          </Link>
        }
      />
      <Freshness stale={live.stale} error={live.error} hasData={live.data !== null} />
      {live.data && item === null ? (
        <Absent
          reason={`No card with the id ${id} is waiting on you. It may have been answered already, or it may belong to somebody else — the inbox only ever carries the cards you can see.`}
        />
      ) : (
        item && <DeepLinkedCard item={item} stream={stream} onAnswered={live.reload} />
      )}
    </section>
  )
}

/** The one card at its stable address: the same face the queue shows (static —
 *  there is no list to fold back into), with the detail already open. */
function DeepLinkedCard({
  item,
  stream,
  onAnswered,
}: {
  item: ApprovalItem
  stream?: EventStream
  onAnswered: () => void
}) {
  const join = useTaskJoin(stream)
  const where = whereOf(item, join)
  return (
    <div className="card-row" data-card-id={item.id} data-kind={item.kind} data-tier={item.tier} data-open="true">
      <CardFace item={item} where={where} />
      <WithForms items={[item]} stream={stream}>
        {(forms) => <CardDetail item={item} where={where} forms={forms} onAnswered={onAnswered} deep />}
      </WithForms>
    </div>
  )
}

// ── the gate-ordered filters (2026-08-22): project · task · reading order ──
//
// All three live in the URL, so a filtered slice is a stable address a project
// page can link to and a bookmark keeps (?project=…&task=…&sort=…). They are
// PRESENTATION over the served list — sanctioned at the gate (findings §1,
// S15.12) — and the served order remains the default.

export type InboxSort = '' | 'newest' | 'oldest' | 'deadline' | 'tier'

/** The reading orders, each a derivation over one SERVED field ('' = the
 *  control plane's own order, always the default). */
const sortChoices: { value: InboxSort; label: string }[] = [
  { value: '', label: "The platform's order" },
  { value: 'newest', label: 'Newest first' },
  { value: 'oldest', label: 'Waiting longest first' },
  { value: 'deadline', label: 'Deadline first' },
  { value: 'tier', label: 'Highest risk first' },
]

export type InboxQuery = {
  /** null = no param → follow the app-wide project scope; '' = the explicit
   *  "all projects" override; anything else = that served project bucket. */
  project: string | null
  /** '' = no task filter; else the served task id. */
  task: string
  sort: InboxSort
}

/** inboxQueryFromSearch reads the filter off the URL. An unknown sort is the
 *  default order rather than an error — a stale bookmark still shows the
 *  queue, exactly the posture the mission-control filters take. */
export function inboxQueryFromSearch(search: string): InboxQuery {
  const p = new URLSearchParams(search)
  const sort = p.get('sort') ?? ''
  return {
    project: p.get('project'),
    task: p.get('task') ?? '',
    sort: sortChoices.some((s) => s.value === sort) ? (sort as InboxSort) : '',
  }
}

/** hrefForInbox builds the deep-linkable address of one slice. The project
 *  param stays PRESENT-BUT-EMPTY for the explicit "all projects" override —
 *  that is what distinguishes "show everything" from "follow the app scope". */
export function hrefForInbox(q: InboxQuery): string {
  const p = new URLSearchParams()
  if (q.project !== null) p.set('project', q.project)
  if (q.task !== '') p.set('task', q.task)
  if (q.sort !== '') p.set('sort', q.sort)
  const qs = p.toString()
  return qs === '' ? hrefFor('inbox') : `${hrefFor('inbox')}?${qs}`
}

/** The project this view narrows to: the URL when it says anything (its empty
 *  form is the explicit override), else the app-wide scope from the topbar. */
function effectiveProject(q: InboxQuery, scopeProject: string): string {
  return q.project !== null ? q.project : scopeProject
}

/** paren wraps a name for prose unless it already wears its own parentheses —
 *  the "(no project)" bucket's served name IS parenthesised, and wrapping it
 *  again printed "((no project))" (F4, drain r1). */
function paren(name: string): string {
  return name.startsWith('(') && name.endsWith(')') ? name : `(${name})`
}

/**
 * The tasks read this surface joins cards against (gate finding 4). The wire
 * serves `task_id` on a card but no project; the project is a served fact OF
 * THE TASK, so it comes from the served tasks read — a presentation-only join,
 * never a guess. The read is the same one the topbar's scope options make;
 * its own key keeps the subscriptions independent (§49 records the shape).
 */
function useTaskJoin(stream?: EventStream): Live<TaskList> {
  const session = useSessionInfo()
  const me = session?.user?.user_id ?? ''
  return useLive<TaskList>({
    key: `/api/tasks#inbox-join:${me}`,
    read: () => api.tasks(),
    // A task is born (and gets its project) on the intake family's frame.
    types: ['intake.state'],
    stream,
  })
}

/** Where one card belongs, resolved from served facts only. */
type Where = {
  /** The served project bucket; '(no project)' for a card with no task (the
   *  honest bucket, api.ts:156); null while the tasks read has not answered —
   *  or answered TRUNCATED without this card's task, when claiming any bucket
   *  would be a guess. */
  project: string | null
  /** The task's own title where the join found it, else the served task id;
   *  null when the card names no task. */
  task: string | null
  /** The honest note when a served task ref could not be resolved. */
  note: string
}

function whereOf(item: ApprovalItem, join: Live<TaskList>): Where {
  if (item.task_id === undefined || item.task_id === '') return { project: '(no project)', task: null, note: '' }
  if (join.data === null) return { project: null, task: item.task_id, note: '' }
  const t = join.data.tasks.find((row) => row.task_id === item.task_id)
  if (t !== undefined) {
    return { project: t.project, task: t.title !== '' ? t.title : t.task_id, note: '' }
  }
  if (join.data.truncated) {
    // The join broke on the read's own page bound: the task exists somewhere
    // past what one read returns. No bucket is claimed — the card stays
    // visible under every filter rather than hidden on a guess.
    return { project: null, task: item.task_id, note: 'its project is not readable from one page of the task list' }
  }
  return {
    project: '(no project)',
    task: item.task_id,
    note: 'its task is not in your task list, so it files under (no project)',
  }
}

/** filterItems narrows by task then by project. A card whose project could not
 *  be resolved (truncated tasks read) is KEPT under a project filter — shown
 *  with its honest note, never silently dropped on a guess. */
function filterItems(
  items: ApprovalItem[],
  wheres: Map<string, Where>,
  q: InboxQuery,
  scopeProject: string,
): ApprovalItem[] {
  const project = effectiveProject(q, scopeProject)
  return items.filter((item) => {
    if (q.task !== '' && item.task_id !== q.task) return false
    if (project !== '') {
      const where = wheres.get(item.id)
      if (where === undefined) return true
      if (where.project !== null && where.project !== project) return false
    }
    return true
  })
}

const tierOrder: Record<string, number> = { high: 0, medium: 1, low: 2 }

/** orderItems applies the chosen reading order. '' returns the served array
 *  untouched; every other order is a STABLE sort over one served field, so
 *  ties keep the control plane's own order. */
function orderItems(items: ApprovalItem[], sort: InboxSort): ApprovalItem[] {
  if (sort === '') return items
  const out = [...items]
  if (sort === 'newest') out.sort((a, b) => cmp(b.observed_ts, a.observed_ts))
  if (sort === 'oldest') out.sort((a, b) => cmp(a.observed_ts, b.observed_ts))
  if (sort === 'deadline') {
    // A card with no served deadline sinks below every card with one — it is
    // not "due last", it has no deadline at all, and inventing one would be
    // worse than sorting it after.
    out.sort((a, b) => {
      const ax = a.expiry_at ?? ''
      const bx = b.expiry_at ?? ''
      if (ax === '' && bx === '') return 0
      if (ax === '') return 1
      if (bx === '') return -1
      return cmp(ax, bx)
    })
  }
  if (sort === 'tier') out.sort((a, b) => (tierOrder[a.tier] ?? 3) - (tierOrder[b.tier] ?? 3))
  return out
}

/** Served instants are UTC RFC3339, so the lexicographic compare IS the time
 *  compare — no Date parsing, no zone. */
function cmp(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

/**
 * The filter bar and its honesty line. Every option list is derived from the
 * cards actually served (plus the current value, so a deep link to a slice
 * with no cards still shows what it asked for); the line under the controls
 * says how many cards are hidden and by what, because a filter that hides
 * silently is the failure mode the landed posture existed to prevent.
 */
function InboxControls({
  items,
  wheres,
  q,
  scopeProject,
  join,
  shown,
}: {
  items: ApprovalItem[]
  wheres: Map<string, Where>
  q: InboxQuery
  scopeProject: string
  join: Live<TaskList>
  shown: number
}) {
  const project = effectiveProject(q, scopeProject)
  const joinReady = join.data !== null

  // The project options: every bucket the served cards resolve into.
  const projects = [...new Set([...wheres.values()].map((w) => w.project).filter((p): p is string => p !== null))].sort(
    (a, b) => (a === '(no project)' ? 1 : b === '(no project)' ? -1 : a.localeCompare(b)),
  )
  if (project !== '' && !projects.includes(project)) projects.push(project)

  // The task options: every task the served cards name, labelled by its own
  // title where the join knows it. Narrowed to the project filter so the two
  // selects agree about what is on offer.
  const tasks = new Map<string, string>()
  for (const item of items) {
    if (item.task_id === undefined || item.task_id === '') continue
    const where = wheres.get(item.id)
    if (project !== '' && where !== undefined && where.project !== null && where.project !== project) continue
    tasks.set(item.task_id, where?.task ?? item.task_id)
  }
  if (q.task !== '' && !tasks.has(q.task)) tasks.set(q.task, q.task)

  const go = (next: InboxQuery) => {
    navigate(hrefForInbox(next))
  }

  const hidden = items.length - shown
  const filters: string[] = []
  if (q.task !== '') filters.push(`the task filter ${paren(tasks.get(q.task) ?? q.task)}`)
  if (project !== '') {
    filters.push(
      q.project !== null
        ? `the project filter ${paren(project)}`
        : `your project scope ${paren(project)} — picked at the top of the app`,
    )
  }
  // F3 (drain r1): the URL beating the topbar scope is the designed rule, and
  // it must be SAID — a person whose topbar shows one project while the page
  // filters by another was left to guess which control had won.
  const overridden = q.project !== null && scopeProject !== '' && q.project !== scopeProject

  return (
    <div className="inbox-controls" data-inbox-controls>
      <div className="inbox-selects">
        <label>
          <span>Project</span>
          <select
            value={project}
            data-filter="project"
            disabled={!joinReady && join.error !== ''}
            onChange={(e) => {
              const picked = e.currentTarget.value
              // Picking "all" writes the explicit override only where the app
              // scope would otherwise re-narrow this page; changing project
              // drops a task filter that may not belong to it.
              go({ project: picked === '' && scopeProject === '' ? null : picked, task: '', sort: q.sort })
            }}
          >
            <option value="">All projects</option>
            {projects.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Task</span>
          <select
            value={q.task}
            data-filter="task"
            onChange={(e) => {
              go({ project: q.project, task: e.currentTarget.value, sort: q.sort })
            }}
          >
            <option value="">All tasks</option>
            {[...tasks.entries()]
              .sort((a, b) => a[1].localeCompare(b[1]))
              .map(([id, label]) => (
                <option key={id} value={id}>
                  {label}
                </option>
              ))}
          </select>
        </label>
        <label>
          <span>Order</span>
          <select
            value={q.sort}
            data-filter="sort"
            onChange={(e) => {
              go({ project: q.project, task: q.task, sort: e.currentTarget.value as InboxSort })
            }}
          >
            {sortChoices.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
        </label>
      </div>
      {filters.length > 0 && (
        <p className="max-w-prose text-sm text-muted-foreground" data-inbox-hiding={String(hidden)}>
          {hidden > 0 ? (
            <>
              Showing <span className="font-mono tabular-nums">{String(shown)}</span> of{' '}
              <span className="font-mono tabular-nums">{String(items.length)}</span> cards —{' '}
              <span className="font-mono tabular-nums">{String(hidden)}</span> hidden by {filters.join(' and ')}.
              Nothing is dropped: every card is still served and waiting.
            </>
          ) : (
            <>Narrowed by {filters.join(' and ')} — every served card happens to match, so nothing is hidden.</>
          )}{' '}
          <Button
            variant="ghost"
            size="sm"
            data-action="clear-filters"
            onClick={() => {
              // "Show everything" must beat the app scope too, or clearing
              // would quietly re-narrow to it — hence the explicit override.
              go({ project: scopeProject !== '' ? '' : null, task: '', sort: q.sort })
            }}
          >
            Show everything
          </Button>
        </p>
      )}
      {overridden && (
        <p className="max-w-prose text-sm text-muted-foreground" data-inbox-override>
          This page&apos;s address carries its own project filter
          {q.project === '' && <> — every project</>}, and it overrides the project picked at the top of the app{' '}
          {paren(scopeProject)}.
        </p>
      )}
      {q.sort !== '' && (
        <p className="max-w-prose text-sm text-muted-foreground" data-inbox-sorted={q.sort}>
          Re-ordered for reading: {sortChoices.find((s) => s.value === q.sort)?.label.toLowerCase()}. The platform&apos;s
          own order is the default, and this changes nothing about what is served.{' '}
          <Button
            variant="ghost"
            size="sm"
            data-action="clear-sort"
            onClick={() => {
              go({ project: q.project, task: q.task, sort: '' })
            }}
          >
            Platform order
          </Button>
        </p>
      )}
    </div>
  )
}

/**
 * WithForms is the CARDS' read of the blind-pair form data, and it exists in
 * this shape for two reasons.
 *
 * It is read ONCE for the whole queue rather than per card, because it is one
 * question with one answer — and it is mounted only when a benchmark card is
 * actually on screen. A hook cannot be called conditionally, which is why the
 * condition lives in what MOUNTS rather than in what the hook does.
 *
 * ⚠ CORRECTED 2026-08-05 (P3-UI-3): this was "the ONE read", and it no longer
 * is. The standing opt-in panel above reads the same route unconditionally,
 * because it has to render whether or not a pair is pending — so an inbox that
 * ALSO carries a benchmark card now makes the request twice, and the sentence
 * about never asking an unwired subsystem is no longer true of this surface
 * either. Folding the two into one read means restructuring this component,
 * which was outside that packet's sanction; the duplication is recorded in
 * CONVENTIONS §49 rather than left for someone to discover here.
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
      {live.error !== '' && <p className="text-sm text-[var(--red)]">{live.error}</p>}
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
          <>This is turned on for you: a task of yours may be picked for comparison.</>
        ) : (
          <>This is turned off for you: none of your tasks is picked for comparison.</>
        )}
      </p>
      {live.error !== '' && <p className="text-sm text-[var(--red)]">{live.error}</p>}

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
 *
 * THE VERBS ARE PLAIN WORDS (r2 finding 2: "opt-in and opt-out… nobody knows
 * what this shit means"). What is recorded on the wire is unchanged; only the
 * words on the buttons and in the outcome line moved to the reader's language.
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
          Take part
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
          Don&apos;t take part
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
        act={asking === true ? 'Yes — take part' : 'No — stop taking part'}
        variant="primary"
        busy={act.busy}
        onConfirm={() => {
          const enabled = asking === true
          setAsking(null)
          act.run(
            () =>
              // No `person` is sent, ever: the subject defaults to the caller,
              // and that is the only subject this consent has.
              //
              // The served `detail` is NOT rendered here (GF2 F1): it is the
              // registration's own sentence — "your standing benchmark
              // opt-in … (BENCH-REG §4.2.1)" — and parroting it made the
              // panel's recorded line jargon in both positions. The
              // plain-words position line above, re-read on the reload, is
              // this panel's only status line.
              api.setBenchmarkOptIn(enabled).then((res) => outcomeOf(true, `recorded: ${res.enabled ? 'you are taking part' : 'you are not taking part'}`, '')),
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
    <p className="max-w-prose text-sm text-muted-foreground" data-note="no-live-frame">
      Memory-conflict questions are read fresh each time this page loads or anything else here changes — nothing pushes
      them. Answering one twice is safe: the second answer reads the closed question back.
    </p>
  )
}

// ── the row anatomy: a compact face for finding, a detail for deciding ─────
//
// Reworked whole at the gate order (findings 2/3/5, 2026-08-22): the face
// carries what project · what task · the issue in plain words with the class
// and tier as chips; pressing it opens the full detail; raw internals live
// only in the collapsed "Technical details" fold at the detail's end.

/** One queue row: the face, and the detail while it is open. `quietWhere`
 *  (the notices drawer) drops the project/task slot from the face when the
 *  card names no task — the drawer's own header already says what these are,
 *  and "(no project)" forty-five times over is the round-1 noise class again.
 *  A notice that DOES name a task keeps its slot. */
function InboxRow({
  item,
  where,
  forms,
  onAnswered,
  quietWhere,
}: {
  item: ApprovalItem
  where: Where
  forms: BenchmarkVerdictForms | null
  onAnswered: () => void
  quietWhere?: boolean
}) {
  const [open, setOpen] = useState(false)
  const faceRef = useRef<HTMLButtonElement | null>(null)
  return (
    <li
      className="card-row"
      data-card-id={item.id}
      data-kind={item.kind}
      data-tier={item.tier}
      data-open={open ? 'true' : 'false'}
      // F6 (drain r1): Escape folds the detail from anywhere inside the row —
      // keyboard parity with the click-to-close — and hands focus back to the
      // face, so the keyboard reader is not dropped into a removed subtree.
      onKeyDown={(e) => {
        if (e.key === 'Escape' && open) {
          setOpen(false)
          faceRef.current?.focus()
        }
      }}
    >
      <CardFace
        item={item}
        where={where}
        open={open}
        faceRef={faceRef}
        quietWhere={quietWhere}
        onToggle={() => {
          setOpen((was) => !was)
        }}
      />
      {open && <CardDetail item={item} where={where} forms={forms} onAnswered={onAnswered} />}
    </li>
  )
}

/**
 * The compact face (gate finding 2): what project · what task · a one-line
 * plain-words account of the issue, with the class label and tier as chips.
 * Every string on it is served or a class-grain line (`issueLine`); no raw id,
 * no kind token, no projection field. Without `onToggle` it renders static —
 * the deep-link surface has no list to fold back into.
 */
function CardFace({
  item,
  where,
  open,
  onToggle,
  faceRef,
  quietWhere,
}: {
  item: ApprovalItem
  where: Where
  open?: boolean
  onToggle?: () => void
  faceRef?: Ref<HTMLButtonElement>
  quietWhere?: boolean
}) {
  const hideWhere = quietWhere === true && (item.task_id === undefined || item.task_id === '')
  const inner = (
    <>
      <span className="face-top">
        {!hideWhere && (
        <span className="face-where">
          {where.project !== null ? (
            <b className="face-project" data-face-project={where.project}>
              {where.project}
            </b>
          ) : (
            // The join has not answered (or answered truncated): claiming a
            // bucket would be a guess, so the slot says so instead.
            <b className="face-project" data-face-project="">
              (project not known yet)
            </b>
          )}
          {where.task !== null && (
            <span className="face-task" data-face-task={item.task_id ?? ''}>
              {where.task}
            </span>
          )}
        </span>
        )}
        <span className="face-chips">
          <Chip data-display-class={displayClass(item)}>{displayClass(item)}</Chip>
          <Chip tone={tierTone(item.tier)} data-tier-label={item.tier}>
            {item.tier}
          </Chip>
          <ExpiryChip item={item} />
          {!item.answerable && (
            <Chip tone="pink" className="normal-case tracking-normal" data-face-not-yours>
              not yours to answer
            </Chip>
          )}
          {item.stale && (
            <Chip tone="yellow" className="normal-case tracking-normal" data-face-stale>
              may be stale
            </Chip>
          )}
          {onToggle !== undefined && <ChevronDown size={14} strokeWidth={2} aria-hidden="true" className="face-chev" />}
        </span>
      </span>
      <span className="face-issue" data-face-issue>
        {issueLine(item)}
      </span>
      {where.note !== '' && (
        <span className="face-note text-xs text-muted-foreground" data-face-note>
          {where.note}
        </span>
      )}
    </>
  )
  if (onToggle === undefined) {
    return (
      <div className="card-face-btn" data-face="static">
        {inner}
      </div>
    )
  }
  return (
    <button
      type="button"
      className="card-face-btn"
      data-face
      aria-expanded={open === true}
      onClick={onToggle}
      ref={faceRef}
    >
      {inner}
    </button>
  )
}

/** The compact deadline signal. A served expiry earns a face chip — a card
 *  past its date is exactly the one a person is scanning for — and a card
 *  with none shows nothing, never an invented deadline. */
function ExpiryChip({ item }: { item: ApprovalItem }) {
  if (!item.expiry_at) return null
  const remaining = Date.parse(item.expiry_at) - Date.now()
  return remaining <= 0 ? (
    <Chip tone="red" className="normal-case tracking-normal" data-face-expired>
      expired — still waiting
    </Chip>
  ) : (
    <Chip tone="orange" className="normal-case tracking-normal" data-face-expires>
      expires in {describeSpan(remaining)}
    </Chip>
  )
}

/** The one served not-answerable sentence, byte-exact (internal/api/approvals.go:253). */
const d10Reason = "the card's owner answers it (D10); you can read it but not decide it"

/** plainReason — the served not-answerable reason minus its bare spec cite
 *  (GF2 F2): "(D10)" names a spec section, not anything a reader can use.
 *  Exactly the known sentence renders without the cite — a display derivation
 *  over one known served string (the humanizeVerifySummary precedent); every
 *  other reason renders verbatim, never parsed. */
export function plainReason(reason: string): string {
  return reason === d10Reason ? "the card's owner answers it; you can read it but not decide it" : reason
}

/** readString lifts one string field off a served card body, or null. It reads
 *  a key by name and judges nothing — the §38 no-classifying rule stays. */
function readString(card: unknown, key: string): string | null {
  if (!card || typeof card !== 'object') return null
  const v = (card as Record<string, unknown>)[key]
  return typeof v === 'string' && v !== '' ? v : null
}

/**
 * The face's one-line account of the issue (gate finding 3: "the headline is
 * sign-off or question — what do I want with that").
 *
 * SERVED TEXT FIRST. Wherever the card body carries the platform's own plain
 * words — a plan's restatement, a decision's summary, a watchdog's detail, a
 * drift or alarm summary, the conflict's question — that is the line, verbatim
 * (clamped by CSS, whole in the detail). Where a kind serves no prose, the
 * line is class-grain teaching copy under the same D8 mandate as `checkFirst`:
 * true of every card of the kind, naming no fact the wire did not carry. An
 * unknown kind gets its honest generic line and still renders (§42).
 */
function issueLine(item: ApprovalItem): string {
  switch (item.kind) {
    case 'ask':
      return askIssueLine(item)
    case 'effect': {
      const kind = readString(item.card, 'kind')
      return kind !== null
        ? `Wants to run "${kind}" outside the platform — nothing happens until it is signed.`
        : 'Wants to do something outside the platform — nothing happens until it is signed.'
    }
    case 'memory_conflict':
      return readString(item.card, 'question') ?? 'Two saved lessons may disagree — say which one is right.'
    case 'benchmark_verdict':
      return (
        readString(item.card, 'reason') ??
        'Two answers to the same request, shown blind — pick the better one and guess which was the platform.'
      )
    case 'benchmark_alarm':
      return readString(item.card, 'summary') ?? 'The blind-comparison practice raised an alarm — decide what to do with it.'
    case 'watchdog_flag':
      return readString(item.card, 'detail') ?? 'A watchdog flagged something unusual — look at it and sign off.'
    case 'drift_card': {
      // The face leads with the summary's plain words (GF2 F4) — the raw
      // watch id it arrives prefixed with stays in the technical fold — and
      // the fallback claims no decision (GF2 F5): a notice is read, not
      // signed off.
      const summary = readString(item.card, 'summary')
      return summary !== null
        ? plainDriftSummary(summary)
        : 'Something the platform depends on changed outside it — open it to read what was noticed.'
    }
    case 'conformance_card':
      return 'A platform self-check came back red — acknowledging records that a person has seen it.'
    default:
      return 'The platform sent a card this page has not met before — open it for the record it carries.'
  }
}

/** The ask families' face lines, keyed the same way the answer envelope is:
 *  off the stored snapshot's own declarations. */
function askIssueLine(item: ApprovalItem): string {
  const snap = asSnapshot(item.card)
  if (snap.approval) {
    return snap.approval.layer1?.restatement ?? 'A plan is ready — approve it or send it back.'
  }
  if (snap.delta) return 'The plan changed after you approved it — look at exactly what changed.'
  if (snap.decision) return snap.decision.summary ?? 'The run needs a decision before it can go on.'
  if (isVerifyCard(snap)) {
    const humanized = snap.category === 'CHECK-INTEGRITY' ? humanizeVerifySummary(snap.summary ?? '') : null
    if (humanized !== null) return humanized.plain
    return snap.summary !== undefined && snap.summary !== ''
      ? snap.summary
      : 'Checking the work hit a wall — decide how it goes on.'
  }
  const questions = snap.questions ?? []
  if (questions.length > 0) {
    const first = questions[0].phrased !== undefined && questions[0].phrased !== '' ? questions[0].phrased : questions[0].text
    return questions.length === 1 ? first : `${first} (+${String(questions.length - 1)} more question${questions.length === 2 ? '' : 's'})`
  }
  return 'A question the platform needs answered — open it for what was sent.'
}

/**
 * The full detail (gate finding 3): what the issue is, what has to be decided
 * and the information that decision needs (the card's own content), what the
 * platform recommends (the served 13.5 help, rendered inside the content),
 * then the verbs — with the raw internals folded at the end. Every fact is the
 * landed served field it always was; only the arrangement is new.
 */
function CardDetail({
  item,
  where,
  forms,
  onAnswered,
  deep,
}: {
  item: ApprovalItem
  where?: Where
  forms: BenchmarkVerdictForms | null
  onAnswered: () => void
  deep?: boolean
}) {
  return (
    <div className="card-detail" data-detail>
      <div className="card-head">
        {item.answerable ? (
          <Chip tone="green" data-answerable="true">
            {/* A notice's chip says what a notice is (GF2 F5): something to
                read and clear, never a decision claim — the drawer's own
                intro promises nothing here needs one. */}
            {isNotice(item) ? 'yours to clear' : 'yours to answer'}
          </Chip>
        ) : (
          <Absent reason={plainReason(item.not_answerable_reason ?? 'not yours to answer')} />
        )}
        {item.step_up_required && (
          <Chip tone="orange" data-step-up="true">
            PIN required
          </Chip>
        )}
        <Staleness item={item} />
      </div>
      <p className="flex flex-wrap items-baseline gap-x-2 gap-y-1 text-sm text-muted-foreground" data-card-facts>
        <Owner id={item.owner} />
        <span>
          seen <Timestamp ts={item.observed_ts} variant="live" />
        </span>
        <Expiry item={item} />
      </p>
      <CheckFirst item={item} />
      <JumpToWork item={item} where={where} />
      <CardBody item={item} forms={forms} onAnswered={onAnswered} expanded={deep} />
      <TechDetails item={item} />
    </div>
  )
}

/** The kinds whose stored card is a projection row: their raw record renders
 *  ONLY inside the technical fold (gate finding 2/4). The list names the kinds
 *  whose plain-words content above already says what matters — an unknown
 *  kind's record stays VISIBLE in the detail instead (§42: forward tolerance
 *  must not hide the one thing a new kind has). */
const foldedRowKinds = ['watchdog_flag', 'drift_card', 'conformance_card', 'benchmark_alarm', 'benchmark_verdict']

/**
 * The collapsed fold every card ends with (gate finding 4): the card's own
 * ids — card id (as its stable address), kind, run ref — and, for the
 * projection-row kinds, the row's raw record whole. Nothing in here is new
 * derivation; it is the landed provenance line and the landed RowCard, moved
 * out of everyone's way and kept for the reader who needs the record.
 */
function TechDetails({ item }: { item: ApprovalItem }) {
  return (
    <details className="tech-fold" data-tech-details>
      <summary className="cursor-pointer text-sm text-muted-foreground">Technical details</summary>
      <p
        className="flex flex-wrap items-baseline gap-x-2 gap-y-1 font-mono text-[11px] tabular-nums text-muted-foreground"
        data-provenance="card"
      >
        <Link to={hrefFor('inbox-item', { id: item.id })} className="card-id" title="This card's own stable address">
          {item.id}
        </Link>
        <span data-provenance-field="kind">{item.kind}</span>
        {/* The run ref stays a run ref: the task route keys on task ids, and
            the resolved task jump lives in the detail above. The task id sits
            here too (F5) — the jump line wears the task's human title, so the
            raw ref's home is this fold. */}
        {item.run_id && <span data-provenance-field="run">run {item.run_id}</span>}
        {item.task_id && <span data-provenance-field="task">task {item.task_id}</span>}
        <span>
          seen <Stamp ts={item.observed_ts} />
        </span>
      </p>
      <PlanMechanics item={item} />
      {foldedRowKinds.includes(item.kind) && <RowCard card={item.card} />}
    </details>
  )
}

/** plainCostLine — the plan's served cost line in the reader's words (GF2 F7).
 *  The estimator's own dialect is one known shape — "~0.00 USD
 *  (API-equivalent, D5)" (internal/intake/pipeline.go:1439) — whose cite meant
 *  nothing on the card face. The known shape leads with plain words and the
 *  served line stands whole in the fold (PlanMechanics); any other cost line
 *  renders verbatim — the other served shapes are already plain sentences. */
export function plainCostLine(costTime: string): string {
  const m = /^~(\d+(?:\.\d+)?) USD \(API-equivalent, D5\)$/.exec(costTime)
  if (m === null) return costTime
  return `about ${m[1]} USD of model use, by the platform's own estimate`
}

/**
 * The plan card's routing mechanics, in the fold (GF2 F7): the served cost
 * line verbatim, the clearance measure, the size class and the size note.
 * None of it is a decision input for the card face; all of it stays whole and
 * findable for the reader who wants the record. Renders nothing for a card
 * that is not an S06.9 plan.
 */
function PlanMechanics({ item }: { item: ApprovalItem }) {
  if (item.kind !== 'ask') return null
  const l1 = asSnapshot(item.card).approval?.layer1
  if (l1 === undefined) return null
  const rows = [
    l1.cost_time,
    l1.clearance !== undefined ? `clearance ${String(l1.clearance)}` : undefined,
    l1.size_class !== undefined && l1.size_class !== '' ? `size class ${l1.size_class}` : undefined,
    l1.size_note,
  ].filter((s): s is string => typeof s === 'string' && s !== '')
  if (rows.length === 0) return null
  return (
    <p className="font-mono text-[11px] tabular-nums text-muted-foreground wrap-anywhere" data-plan-mechanics>
      {rows.join(' · ')}
    </p>
  )
}

/** The tier's tone, over the S15.6 vocabulary (:90–94): High reaches outward
 *  and is irreversible, Medium writes the platform's own stores, Low is
 *  read-only or reversible inside the workspace. A tier this list has never
 *  seen takes the neutral identity tone rather than a guessed severity. */
function tierTone(tier: string): Tone {
  if (tier === 'high') return 'red'
  if (tier === 'medium') return 'orange'
  if (tier === 'low') return 'blue'
  return 'accent'
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
    case 'conformance_card':
    case 'benchmark_alarm':
      return 'sign-off'
    case 'drift_card':
      // GF2 F5: inside the notices drawer a "sign-off" chip read as a
      // decision claim under an intro promising none. A notice is a notice.
      return 'notice'
    case 'benchmark_verdict':
      return 'judgement'
    case 'memory_conflict':
      return 'question'
    default:
      return item.kind
  }
}

/**
 * LEG 3 — "what to check first", and it is CLASS-GRAIN, never card-grain.
 *
 * No served field carries per-card guidance. The one thing that does is the 13.5
 * help block, which arrives IN the card, is registry- and pipeline-sourced, and
 * renders in full and byte-true wherever it is served (`Help` below). It stays
 * the card-specific authority; this line is orientation, and it is written here
 * under the D8 mandate exactly as the `SurfaceHead` copy is.
 *
 * SO IT NAMES NO CARD FACT. Each line says what to read on a card OF THIS CLASS
 * and what answering one does — statements that are true of every card in the
 * class, derived from nothing on the wire. Generating per-card guidance would
 * fabricate advice the platform never wrote, which the no-fabrication invariant
 * bars outright.
 *
 * A class this file has never seen gets NO line. An unknown kind still renders
 * its row, its provenance and its served verbs (the forward tolerance §42
 * requires); what it does not get is guidance nobody could have written for it.
 */
function CheckFirst({ item }: { item: ApprovalItem }) {
  const advice = checkFirst(item)
  if (advice === '') return null
  return (
    <p className="max-w-prose text-sm text-foreground" data-check-first={checkClass(item)}>
      <span className="text-muted-foreground">What to check first: </span>
      {advice}
    </p>
  )
}

/**
 * checkClass reads the class off the card's OWN declarations and nothing else:
 * the served `kind`, and — for an `ask`, whose four families are one kind on the
 * wire — the stored snapshot's own family through the landed `answerEnvelope`.
 *
 * `displayClass` above is the reader's LABEL and is deliberately coarser: it
 * calls every `ask` a question, and an S06.9 plan and a decision card need
 * different things read first. Keying the advice on the label would have made
 * one of those two lines false.
 */
function checkClass(item: ApprovalItem): string {
  switch (item.kind) {
    case 'effect':
      return 'effect'
    case 'memory_conflict':
      return 'question'
    case 'benchmark_verdict':
      return 'judgement'
    case 'watchdog_flag':
    case 'conformance_card':
    case 'benchmark_alarm':
      return 'sign-off'
    case 'drift_card':
      // The notice class (GF2 F5): its advice must not read as a decision —
      // the sign-off line's "signing it off" was one inside the drawer.
      return 'notice'
    case 'ask': {
      const envelope = answerEnvelope(item)
      if (envelope === 'contest') return 'proposal'
      if (envelope === 'choice' || envelope === 'answers' || envelope === 'verify') return 'question'
      return ''
    }
  }
  return ''
}

/**
 * The six lines. Each is checked against the thing it teaches:
 *
 *  - the proposal line points at `ApprovalCard`'s own centerpiece — the
 *    assumptions and the will-not-do list (S06.9);
 *  - the question line is true of a decision card, an interview card and the
 *    ninth kind alike: the options ON the card are the answer vocabulary the
 *    verbs accept (`composeAnswer`);
 *  - the sign-off line agrees with the conformance verb, whose acknowledgement
 *    is served STILL RED — acknowledging records that somebody read it;
 *  - the notice line agrees with the served dismiss verb (GF2 F5): clearing
 *    records a reader and stops nothing, and it claims no decision;
 *  - the judgement line states BENCH-REG §3.3's frozen rule, which `canFire`
 *    enforces in the form: no verdict without a guess;
 *  - the effect line is careful in the one direction that matters — where a
 *    platform-level effect still needs a second signature, approving is NOT the
 *    outward act, and the co-approval block below says who has signed.
 *
 * None re-states a tier's meaning, none tells anybody a card can be answered in
 * a batch, and none uses the 13.5 block's own three headings.
 */
function checkFirst(item: ApprovalItem): string {
  switch (checkClass(item)) {
    case 'proposal':
      return 'the assumptions this plan rests on, and the list of what it will NOT do. Those are where a plan goes wrong, and the verbs below answer the plan as it is written here.'
    case 'question':
      return 'the question itself and the options listed on this card. Those options are the whole of the answer — nothing outside this card is part of it.'
    case 'sign-off':
      return "the platform's own account of what happened, below — the full raw record is inside Technical details. Signing it off records that a person has read it; it does not undo it and does not make it go away."
    case 'notice':
      return "the platform's own account of what it noticed, below — the full raw record is inside Technical details. Clearing it just records that you've seen it; the watch itself keeps running either way."
    case 'judgement':
      return 'both responses, without knowing which is which, and then say which one you think the platform produced. The guess travels with the vote — it is part of the answer rather than an extra.'
    case 'effect':
      return 'what this would do outside the platform, and who has already signed. Where both the owner and the operator are required, it is not approved until both of them are in.'
  }
  return ''
}

/**
 * LEG 4 — the jump to the work, and it renders ONLY what the wire carries.
 *
 * The card serves TWO work references and they are not interchangeable. `run_id`
 * is the run that raised the card and is the run ref the provenance line above
 * carries. `task_id` is the TASK that run belongs to, resolved server-side from
 * the run row (internal/api/approvals.go, `fillTaskRefs`) — and it is the one the
 * task route can be given, because a run id is "<task_id>.<stage>[.gN]" while the
 * task read keys on `tasks.task_id`, so the two id spaces never overlap.
 *
 * ⚠ HISTORY, kept so nobody re-introduces it. Before P3-UI-6 drain r1 this leg
 * handed the served RUN id to the task route, which meant every card's link
 * resolved to the task read's own not-found — structurally, for every card, not
 * as an edge case. The field above was sanctioned to fix it at the source rather
 * than by teaching this client to derive a task id from a run id, which would
 * have been a second implementation of a mapping only the run row actually
 * knows. The link is now labelled for what it opens because it now opens it.
 *
 * ABSENCE INVENTS NOTHING. A card with no run — a blind pair, a memory conflict
 * — has no leg. A card whose run resolves to no task serves no `task_id` and
 * likewise has no leg, rather than a door to a page that does not exist.
 */
function JumpToWork({ item, where }: { item: ApprovalItem; where?: Where }) {
  if (!item.task_id) return null
  // F5 (drain r1): the joined task TITLE is the human name for what the link
  // opens; the raw id renders only where no title is known (the join has not
  // answered, or the task has no title) and stays findable in the technical
  // fold either way.
  const title = where?.task ?? item.task_id
  return (
    <p className="text-sm" data-jump="task">
      <Link to={hrefFor('task', { id: item.task_id })}>
        Open the task this came from — its deliverable revisions and receipts are there
      </Link>{' '}
      {title === item.task_id ? (
        <span className="font-mono tabular-nums text-muted-foreground">{item.task_id}</span>
      ) : (
        <span className="text-muted-foreground" data-jump-title>
          {title}
        </span>
      )}
    </p>
  )
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
      {/* A PAST expiry says what it means (W2-4): the fact that the card is
          still in this served queue IS the consequence — the date passing
          withdrew nothing and answered nothing, the ask still stands. A bare
          "(past)" left the reader guessing whether this was live work. */}
      {remaining <= 0 ? (
        <span className="text-[var(--red)]">
          expired <Timestamp ts={item.expiry_at} variant="live" /> ({describeSpan(-remaining)} ago) — still waiting
          on you: passing the date withdrew nothing and answered nothing
        </span>
      ) : (
        <>
          expires <Timestamp ts={item.expiry_at} variant="live" />
          <span className="text-muted-foreground"> (in {describeSpan(remaining)})</span>
        </>
      )}
      {item.engine_expiry_ts && (
        <span className="text-muted-foreground">
          {' '}
          · the engine's own deadline: <Timestamp ts={item.engine_expiry_ts} variant="live" />
        </span>
      )}
    </span>
  )
}

/** The sign-in-to-act line (W2-4): rendered beside a read-only acts row when
 *  the viewer is the dev fallback — the one posture where "not yours to
 *  answer" has a remedy this page can name. A real signed-in user seeing a
 *  card that is not theirs gets no false invitation. */
function SignInToAct() {
  const s = useSessionInfo()
  if (s?.dev !== true) return null
  return (
    <span className="text-sm text-muted-foreground" data-sign-in-to-act>
      {' '}
      You are browsing as nobody in particular — <b>Sign in</b> (top right) as the person this belongs to, and its
      answer controls appear here.
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
    <Chip tone="yellow" data-stale-flag="true" className="normal-case tracking-normal">
      assumptions may be stale
      {reasons.length > 0 && <span className="opacity-80"> — {reasons.join('; ')}</span>}
    </Chip>
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
    <div className="mt-2">
      {current && (
        <p
          className="my-2 rounded-(--radius-sm) border border-[color-mix(in_srgb,var(--yellow)_25%,transparent)] bg-[color-mix(in_srgb,var(--yellow)_9%,transparent)] px-2 py-1 text-sm text-[var(--yellow)]"
          data-changed="stale-payload"
        >
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
 *  ever markup (§41-B escape-by-default). The projection-row kinds lead with
 *  their own served prose; their raw record lives in the technical fold (gate
 *  finding 2). An UNKNOWN kind keeps its record visible right here — hiding
 *  the one thing a new kind has would be forward tolerance in name only. */
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
    case 'watchdog_flag':
      return <WatchdogContent card={item.card} />
    case 'drift_card':
      return <DriftContent card={item.card} />
    case 'conformance_card':
      return <ConformanceContent card={item.card} />
    case 'benchmark_alarm':
      return <AlarmContent card={item.card} />
    default:
      return <RowCard card={item.card} />
  }
}

// ── the projection-row kinds, in plain words (gate findings 2/3) ───────────
//
// Each of these renders the SERVED fields a person deciding actually needs —
// the producer's own prose first, the honest absence where it served none —
// and nothing else: the full raw row stands in the technical fold, so no key
// a producer adds is ever lost, it is just never the face of the card.

/** The watchdog flag: the watchdog's own account of what looked wrong. */
function WatchdogContent({ card }: { card: unknown }) {
  const detail = readString(card, 'detail')
  const ts = readString(card, 'flagged_ts')
  return (
    <div className="ask-card" data-card-kind="watchdog_flag">
      <p className="restatement wrap-anywhere">
        {detail ?? <Absent reason="the flag carries no account of what looked wrong" />}
      </p>
      <p className="muted">
        The run this flag is about is paused on it.
        {ts !== null && (
          <>
            {' '}
            Flagged <Timestamp ts={ts} variant="live" />.
          </>
        )}
      </p>
    </div>
  )
}

/** plainDriftSummary — the watch runner's summaries arrive prefixed with the
 *  watch row's own id ("w-localllama: …"), which put a raw id at the head of
 *  every notice face (GF2 F4). The known prefix shape is stripped for display
 *  — a derivation over the one known producer shape, the sourceLabel move —
 *  and the raw summary stays whole in the technical fold's record. A summary
 *  not wearing the prefix renders verbatim. */
export function plainDriftSummary(summary: string): string {
  return summary.replace(/^w-[a-z0-9][\w-]*:\s+/i, '')
}

/** The outside-world drift card: the producer's summary, and how often the
 *  same incident has hit. `hits` is a served count, never a computation. */
function DriftContent({ card }: { card: unknown }) {
  const summary = readString(card, 'summary')
  const source = readString(card, 'source')
  const note = readString(card, 'revalidation_note')
  const hits = card !== null && typeof card === 'object' ? (card as Record<string, unknown>).hits : undefined
  const classified = card !== null && typeof card === 'object' ? (card as Record<string, unknown>).classified : undefined
  return (
    <div className="ask-card" data-card-kind="drift_card">
      <p className="restatement wrap-anywhere">
        {summary !== null ? plainDriftSummary(summary) : <Absent reason="the finding carries no summary" />}
      </p>
      <p className="muted">
        {source !== null && <>Noticed watching {source}. </>}
        {typeof hits === 'number' && hits > 1 && <>The same incident has hit {String(hits)} times. </>}
        {classified === false && <>The local pass could not judge it — this card is here for a person&apos;s eyes. </>}
        {/* Plain words for the clear (GF2 F3): what the served dismiss verb
            records, said the way the drawer's own button says it. */}
        Clearing it just records that you&apos;ve seen it; it changes nothing outside this queue.
      </p>
      {note !== null && <p className="muted">{note}</p>}
    </div>
  )
}

/** The red conformance row: which recurring self-check, when it last ran, and
 *  what acknowledging does (records a reader — the red clears only when the
 *  check itself passes, S14.5). */
function ConformanceContent({ card }: { card: unknown }) {
  const result = readString(card, 'last_result')
  const schedule = readString(card, 'schedule')
  const ts = readString(card, 'last_run_ts')
  return (
    <div className="ask-card" data-card-kind="conformance_card">
      <p className="restatement wrap-anywhere">
        One of the platform&apos;s own recurring self-checks came back{' '}
        {result ?? <Absent reason="no result recorded" />} on its last run
        {ts !== null && (
          <>
            {' '}
            (<Timestamp ts={ts} variant="live" />)
          </>
        )}
        {schedule !== null && <> — it runs on the {schedule}</>}.
      </p>
      <p className="muted">
        Acknowledging records that a person has read this. It does not fix anything and does not make the red go away —
        only the check passing does that.
      </p>
    </div>
  )
}

/** The standing benchmark alarm: the practice's own summary and the served
 *  freeze state. The measured figure and its registered line are served
 *  numbers, printed as served (§30). */
function AlarmContent({ card }: { card: unknown }) {
  const summary = readString(card, 'summary')
  const domain = readString(card, 'domain')
  const body = card !== null && typeof card === 'object' ? (card as Record<string, unknown>) : {}
  const freeze = body.expansion_freeze === true
  return (
    <div className="ask-card" data-card-kind="benchmark_alarm">
      <p className="restatement wrap-anywhere">
        {summary ?? <Absent reason="the alarm carries no summary" />}
      </p>
      <p className="muted">
        {domain !== null && <>Domain: {domain}. </>}
        {typeof body.loss_g === 'number' && typeof body.threshold === 'number' && (
          <>The measured reading is {String(body.loss_g)} against the registered line of {String(body.threshold)}. </>
        )}
        {freeze && <>While this alarm stands, the practice does not expand to new domains. </>}
        Recording a disposition is what closes it.
      </p>
    </div>
  )
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
          <dt className="font-mono text-[11px] tabular-nums">{key}</dt>
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
  understood?: IntakeUnderstood
  questions?: { id: string; text: string; phrased?: string; options?: { label: string; value: string }[] }[]
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
      understood?: IntakeUnderstood
    }
    layer2?: {
      acs?: { n: number; plain: string; structured?: string; structured_kind?: string }[]
      steps?: { id: string; title: string; done_when?: string }[]
      coverage?: Record<string, string[]>
      estimate?: { size_class?: string; usd?: number; known?: boolean; basis?: string }
    }
  }
  delta?: { origin?: string; items?: { kind: string; target: string; old?: string; new?: string }[]; help?: HelpBlock }
  // The S07.7 verify escalation card (verify.Card): its choices are plain verb
  // strings, its findings the drain's own numbered findings. `infrastructure`
  // marks the P3-RW-14 R4 card — verification never ran, the run is parked on
  // this card, and its verbs are exactly {retry, cancel}.
  category?: string
  summary?: string
  detail?: string[]
  choices?: string[]
  findings?: { n?: number; severity?: string; category?: string; criterion?: string; anchor?: string; text?: string; round?: number }[]
  best_effort?: string
  quarantined?: string
  infrastructure?: boolean
  run_id?: string
}

/** isVerifyCard recognizes the S07.7 escalation snapshot by its own declared
 *  shape: the `decision_card` sink kind carrying a plain `summary` string —
 *  the one family whose choices are verb STRINGS rather than labelled options. */
function isVerifyCard(snap: AskSnapshot): boolean {
  return snap.kind === 'decision_card' && typeof snap.summary === 'string' && snap.decision === undefined
}

/** The verify verbs the landed server accepts, per card (verify.ValidateAnswer):
 *  an infrastructure card answers {retry, cancel}; the rework family
 *  (CAP-HIT / AC-BLOCKER / SANITY-BLOCKER) answers its three verbs; every other
 *  category's verbs land with their owning packet and the server refuses them
 *  loudly today. Mirroring that here keeps the buttons honest: a verb the
 *  platform would refuse renders disabled WITH the reason, never as a live
 *  control that fails on press. */
function verifyVerbLive(snap: AskSnapshot, action: string): boolean {
  if (snap.infrastructure === true) return action === 'retry' || action === 'cancel'
  const answerable = snap.category === 'CAP-HIT' || snap.category === 'AC-BLOCKER' || snap.category === 'SANITY-BLOCKER'
  if (!answerable) return false
  return action === 'accept_best_effort' || action === 'revise_with_guidance' || action === 'cancel'
}

function asSnapshot(card: unknown): AskSnapshot {
  return card && typeof card === 'object' ? (card as AskSnapshot) : {}
}

function AskContent({ card, expanded }: { card: unknown; expanded: boolean }) {
  const snap = asSnapshot(card)
  if (snap.approval) return <ApprovalCard snap={snap} expanded={expanded} />
  if (snap.delta) return <DeltaCard snap={snap} />
  if (snap.decision) return <DecisionCard snap={snap} />
  if (isVerifyCard(snap)) return <VerifyEscalationCard snap={snap} />
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
      <p className="restatement wrap-anywhere">{l1.restatement ?? <Absent reason="the card records no restatement" />}</p>
      <UnderstoodPanel understood={l1.understood} heading="Point by point — what was settled, and how" />
      <Bullets title="What you get" items={l1.deliverable} />
      <Bullets title="What I will do" items={l1.steps} ordered />
      <Bullets title="What I will NOT do" items={l1.will_not_do} />

      <h4>Assumptions</h4>
      {(l1.assumptions ?? []).length === 0 ? (
        <Absent reason="the card lists no assumptions" />
      ) : (
        <ul className="assumptions">
          {(l1.assumptions ?? []).map((a) => (
            <li key={a.text} className="wrap-anywhere" data-assumption={a.text}>
              {a.text}
              {a.origin && <span className="muted"> ({a.origin})</span>}
            </li>
          ))}
        </ul>
      )}
      <Bullets title="Risks" items={l1.risks} />
      <Bullets title="Coverage gaps you accepted" items={l1.uncovered} />
      <Bullets title="Open findings" items={l1.open_findings} />

      {/* The cost line in plain words (GF2 F7). The clearance measure, the
          bare size class and the cite-carrying size note are routing
          mechanics, not decision inputs — they moved whole into the
          Technical details fold (PlanMechanics). */}
      <p className="muted" data-plan-cost>
        {l1.cost_time !== undefined && l1.cost_time !== '' ? plainCostLine(l1.cost_time) : 'no cost or time line on this card'}
      </p>
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
          <li key={`${it.kind}:${it.target}`} className="wrap-anywhere" data-delta={it.kind}>
            <span className="font-mono tabular-nums text-(--accent)">{it.kind}</span> <span className="delta-target">{it.target}</span>
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
      <p className="restatement wrap-anywhere">{d.summary ?? <Absent reason="the card records no summary" />}</p>
      <Bullets title="What this is about" items={d.detail} />
      <Help help={d.help} />
    </div>
  )
}

/** The S07.7 verify escalation card, in the ratified row anatomy: what is
 *  being decided (the drain's own summary, written for the person who can fix
 *  it), what it means (the card's detail lines), the numbered findings, and
 *  the quarantine record. Every string is the stored snapshot's own. */
/**
 * The CHECK-INTEGRITY summary arrives as the platform's own error chain —
 * `check-integrity: check "lint" runner failure (not a verdict): verify:
 * compose check sandbox: … no such file or directory` — which is a record, not
 * a sentence for a person (review #11; the C2-13 raw-error ban, applied to a
 * SERVED body). Where the chain matches the known shape, plain words lead and
 * the verbatim chain stands under them as the record (the greyed-verbs
 * pattern beside it: say it plainly, keep the machine's own line visible).
 * A summary this parser does not recognize renders whole, unchanged.
 */
function humanizeVerifySummary(summary: string): { plain: string; record: string } | null {
  const m = /^check-integrity: check "([^"]+)" runner failure \(not a verdict\): (.+)$/.exec(summary)
  if (m === null) return null
  return {
    plain:
      `The "${m[1]}" check could not RUN — the machinery that runs checks failed before any checking happened, ` +
      `so this is not a verdict on the work. Nothing was judged, and the platform did not fake a pass.`,
    record: m[2],
  }
}

function VerifyEscalationCard({ snap }: { snap: AskSnapshot }) {
  const humanized = snap.category === 'CHECK-INTEGRITY' ? humanizeVerifySummary(snap.summary ?? '') : null
  return (
    <div className="ask-card" data-card-kind="verify.decision_card" data-category={snap.category ?? ''}>
      {humanized !== null ? (
        <>
          <p className="restatement wrap-anywhere" data-summary="humanized">{humanized.plain}</p>
          <p className="raw-record wrap-anywhere" data-summary-record>
            <span className="raw-record-label">the platform&apos;s own record:</span> <code>{humanized.record}</code>
          </p>
        </>
      ) : (
        <p className="restatement wrap-anywhere">{snap.summary !== '' ? snap.summary : <Absent reason="the card records no summary" />}</p>
      )}
      {snap.infrastructure === true && (
        <p className="text-sm text-[var(--yellow)]" data-verify="infrastructure">
          Verification never ran — nothing was judged, nothing was delivered, and the work is parked on this card.
          Answering it is what moves the task.
        </p>
      )}
      <Bullets title="What this means" items={snap.detail} />
      {(snap.findings ?? []).length > 0 && (
        <>
          <h4>The findings</h4>
          <ol className="findings">
            {(snap.findings ?? []).map((f, i) => (
              <li key={f.n ?? i} className="wrap-anywhere" data-finding={String(f.n ?? i + 1)}>
                {f.severity && <span className="font-mono text-[11px] uppercase text-[var(--yellow)]">{f.severity} </span>}
                {f.text ?? <Absent reason="this finding carries no text" />}
                {f.anchor && <span className="muted"> — at {f.anchor}</span>}
              </li>
            ))}
          </ol>
        </>
      )}
      {snap.best_effort && (
        <p className="muted" data-verify="best-effort">
          Best-effort state on this card: {snap.best_effort}
        </p>
      )}
      {snap.quarantined && (
        <p className="muted" data-verify="quarantined">
          Quarantined pending fix: {snap.quarantined}
        </p>
      )}
    </div>
  )
}

/** The S06.5 batched option card: the phrased wording when the utility seat
 *  answered (RW-12 R6), the canonical text otherwise, each question's own
 *  options beside it, and the per-round understanding recap above. */
function InterviewCard({ snap }: { snap: AskSnapshot }) {
  return (
    <div className="ask-card" data-card-kind={snap.kind ?? 'interview'}>
      <UnderstoodPanel understood={snap.understood} heading="What it understood so far" />
      <ol className="questions">
        {(snap.questions ?? []).map((q) => (
          <li key={q.id} className="wrap-anywhere" data-question={q.id}>
            {q.phrased !== undefined && q.phrased !== '' ? q.phrased : q.text}
            {(q.options ?? []).length > 0 && (
              <span className="muted"> — {(q.options ?? []).map((o) => o.label).join(' · ')}</span>
            )}
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
    <dl className="border-s-[3px] border-(--accent) ps-3" data-help="13.5">
      {(
        [
          ['What this means', help.what],
          ['What could go wrong', help.wrong],
          ['What I recommend', help.recommend],
        ] as [string, string | undefined][]
      ).map(([label, text]) => (
        <div key={label}>
          <dt className="text-xs text-muted-foreground">{label}</dt>
          <dd className="wrap-anywhere">{text}</dd>
        </div>
      ))}
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
      <summary className="cursor-pointer text-sm text-muted-foreground">{title}</summary>
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
          <Signer role="owner" signed={appr.owner_approved} by={appr.owner_approved_by} fallback="the owner" />
          {appr.platform_level && (
            <Signer role="operator" signed={appr.operator_approved} by={appr.operator_approved_by} fallback="the operator" />
          )}
        </dl>
      )}
      {appr?.platform_level === true && (
        <p className="max-w-prose text-sm text-muted-foreground">
          This effect belongs to no single run, so it needs both the owner&apos;s and the operator&apos;s approval before
          it is approved.
        </p>
      )}
    </div>
  )
}

/** One approver's state, from the served block. An unsigned limb takes the
 *  warning tone: the missing signature is the reason the effect has not fired,
 *  and it has to read differently from the one that is in. */
function Signer({
  role,
  signed,
  by,
  fallback,
}: {
  role: string
  signed: boolean
  by?: string
  fallback: string
}) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{role}</dt>
      <dd
        className={signed ? 'text-sm' : 'text-sm text-[var(--yellow)]'}
        data-signed={signed ? 'true' : 'false'}
      >
        {signed ? `signed by ${by ?? fallback}` : 'not signed'}
      </dd>
    </div>
  )
}

// ── the ninth kind ────────────────────────────────────────────────────────

function ConflictContent({ card }: { card: MemoryConflict | undefined }) {
  if (!card) return <Absent reason="this card carries no stored body" />
  return (
    <div className="conflict-card">
      <p className="restatement wrap-anywhere">{card.question}</p>
      <p className="muted">
        {card.topic_key && <>topic {card.topic_key} · </>}
        <Link to={hrefFor('inbox-item', { id: `memory_conflict:${String(card.conflict_id)}` })}>
          {card.entry_id}
        </Link>{' '}
        and {card.other_entry_id} · noticed <Timestamp ts={card.detected_ts} variant="live" />
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
  // The FAILED pair's served reason (arm ended without an answer): the one
  // plain-words account the row carries, and exactly what the person deciding
  // between "judge" and "decline" needs. The row itself stands in the fold.
  const reason = readString(item.card, 'reason')

  return (
    <div className="verdict-card">
      {reason !== null && <p className="restatement wrap-anywhere">{reason}</p>}
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
          <h4 className="text-sm">Response {side.toUpperCase()}</h4>
          <p className="font-mono text-xs tabular-nums text-muted-foreground">
            {String(side === 'a' ? pair.length_a : pair.length_b)} characters — reported, never corrected
          </p>
          <pre className="render text-xs">{side === 'a' ? pair.render_a : pair.render_b}</pre>
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
  // The cancel's why (P3-RW-19): one line in the person's own words, riding
  // ONLY the cancel-shaped answer it is labeled for. Typing it and pressing
  // any other verb sends nothing of it — the label promises exactly that.
  const [note, setNote] = useState('')
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
        <Absent reason={plainReason(item.not_answerable_reason ?? 'this card is not yours to answer')} />
        {/* The missing sentence (W2-4): under the dev fallback every card is
            read-only, and a card with no verbs and no way forward posed as a
            dead end. Who can act, and how to become them, is said here. */}
        <SignInToAct />
      </p>
    )
  }
  // The working door (r2 finding 1): an intake question card's answers travel
  // the intake route, not the approvals verb, so its real controls live on the
  // give-work journey. The door renders WHEREVER the card resolves to one —
  // alone where the card declares no verbs (the interview shape), above the
  // card's own buttons where it declares some (the family question).
  const resume = intakeResumeHref(item)
  if (actions.length === 0) {
    if (resume !== null) {
      return (
        <div className="acts" data-acts="intake-door">
          <IntakeDoor href={resume} />
        </div>
      )
    }
    // The honest fallback, now reachable ONLY from a kind with genuinely no
    // route (r2 order §4): a card marked yours-to-answer with a route renders
    // its door above instead of this sentence.
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
      {resume !== null && <IntakeDoor href={resume} />}
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
      {envelope === 'choice' && takesCriteria(item) && (
        <CriteriaPicker item={item} picked={criteria} onPick={setCriteria} />
      )}
      {envelope === 'verify' &&
        actions.includes('revise_with_guidance') &&
        verifyVerbLive(asSnapshot(item.card), 'revise_with_guidance') && (
          <label className="contest">
            <span>What should change (recorded as your guidance — the send-back needs at least this one line)</span>
            <input
              type="text"
              value={contest}
              data-field="guidance"
              onChange={(e) => {
                setContest(e.currentTarget.value)
              }}
            />
          </label>
        )}
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
      {actions.some((a) => isCancelShaped(item, a)) && (
        <label className="reason">
          <span>If you cancel — say why, in your own words (optional, kept with the record of what was stopped)</span>
          <input
            type="text"
            value={note}
            data-field="cancel-note"
            onChange={(e) => {
              setNote(e.currentTarget.value)
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

      {envelope === 'verify' && actions.some((a) => !verifyVerbLive(asSnapshot(item.card), a)) && (
        <p className="text-sm text-muted-foreground" data-verify-verbs="deferred">
          The greyed verbs are declared by this card, but their machinery lands with a later build — the platform
          refuses them today, and the card stands as the record until then. Nothing is lost by leaving it open.
        </p>
      )}

      <div className="buttons">
        {actions.map((action) => (
          <Button
            key={action}
            size="sm"
            variant={action === 'approve' ? 'primary' : 'secondary'}
            data-action={action}
            disabled={state.busy || !canFire(item, action, { verdict, guess, contest })}
            onClick={() => {
              void run(actionLabel(item, action), () =>
                fireAction(item, action, {
                  pin,
                  reason,
                  note,
                  contest,
                  verdict,
                  guess,
                  criteria,
                }),
              )
            }}
          >
            {actionLabel(item, action)}
          </Button>
        ))}
      </div>

      {state.note !== '' && (
        <p
          className={state.failed ? 'text-sm text-[var(--red)]' : 'text-sm text-[var(--green)]'}
          data-outcome={state.failed ? 'failed' : 'applied'}
        >
          {state.note}
          {state.detail !== '' && <span className="text-muted-foreground"> — {state.detail}</span>}
        </p>
      )}
      {state.reveal && <Reveal reveal={state.reveal} />}
    </div>
  )
}

/** The door an intake question card carries (r2 finding 1): the give-work
 *  journey resumes this task's interview — option chips, free text, Send —
 *  and the answers given there are what close this card. A real navigation,
 *  in plain words, where "nothing here to press" used to be. */
function IntakeDoor({ href }: { href: string }) {
  return (
    <p className="intake-door" data-answer-door="intake">
      <Button
        variant="primary"
        size="sm"
        data-action="continue-answering"
        onClick={() => {
          navigate(href)
        }}
      >
        Continue answering its questions
      </Button>
      <span className="text-sm text-muted-foreground">
        Opens this task&apos;s interview — pick from its options or answer in your own words. What you answer there is
        what closes this card.
      </span>
    </p>
  )
}

/** The kinds whose landed verb accepts an optional reason. It is a list of the
 *  VERBS' own bodies, not a guess: an effect answer, a flag suppression, a
 *  drift dismissal and a conformance acknowledgement each take one, and the
 *  alarm disposition carries its own beside the disposition it explains. */
const acceptsReason = ['effect', 'watchdog_flag', 'drift_card', 'conformance_card']

/**
 * Plain words on the verb buttons (operator finding F3, 2026-08-16: raw
 * action ids — "approve", "force_proceed", bare family values — read as
 * "buggy buttons all over the place").
 *
 * PRECEDENCE IS THE CARD'S OWN VOCABULARY: a choice-envelope card carries its
 * choices WITH labels, and the label whose value matches the action id is the
 * card's own name for the act — the family card's "Build or change software"
 * comes from there, written by the platform, not by this file. Only the fixed
 * pipeline verbs, whose ids are frozen constants with no served label, get a
 * plain-words name here (the D8 mandate, same as `checkFirst`). An id neither
 * source knows renders as itself — forward tolerance over silence (§42).
 */
const plainVerbs: Record<string, string> = {
  approve: 'Approve',
  reject: 'Reject',
  deny: 'Deny',
  replan: 'Send it back to plan',
  reinterview: 'Re-open the interview',
  cancel: 'Cancel the task',
  compose: 'Compose a specialist',
  force_proceed: 'Proceed — open questions become assumptions',
  verdict: 'Record my pick',
  decline: 'Decline to judge',
  resume: 'Resume the run',
  suppress: 'Set this flag aside',
  dismiss: 'Got it — clear this',
  acknowledge: 'I have read this',
  dispose: 'Record my decision',
  resolve: 'Resolve',
  // The S07.7 verify card verbs (P3-RW-14): frozen ids, plain names.
  retry: 'Retry — run verification again',
  accept_best_effort: 'Accept it as it stands (best effort)',
  revise_with_guidance: 'Send it back with guidance',
  fix_suite: 'The checks are fixed — clear the quarantine',
  waive_check: 'Waive this check',
}

function actionLabel(item: ApprovalItem, action: string): string {
  if (answerEnvelope(item) === 'choice') {
    const choice = (asSnapshot(item.card).decision?.choices ?? []).find((c) => c.value === action)
    if (choice !== undefined && choice.label !== '') return choice.label
  }
  return plainVerbs[action] ?? action
}

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
function canFire(item: ApprovalItem, action: string, form: { verdict: string; guess: string; contest?: string }): boolean {
  if (item.kind === 'benchmark_verdict' && action === 'verdict') {
    return form.verdict !== '' && form.guess !== ''
  }
  if (item.kind === 'benchmark_alarm' && action === 'dispose') return form.verdict !== ''
  if (answerEnvelope(item) === 'verify') {
    // A verb the landed server would refuse does not arm (the note beside the
    // buttons says why), and a revise with no guidance text does not arm —
    // verify.ValidateAnswer requires at least one guidance point.
    if (!verifyVerbLive(asSnapshot(item.card), action)) return false
    if (action === 'revise_with_guidance') return (form.contest ?? '') !== ''
  }
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
function answerEnvelope(item: ApprovalItem): 'action' | 'contest' | 'choice' | 'answers' | 'verify' | 'unknown' {
  if (item.kind !== 'ask') return 'action'
  const snap = asSnapshot(item.card)
  if (snap.approval) return 'contest'
  if (snap.delta) return 'contest'
  if (snap.decision) return 'choice'
  if (isVerifyCard(snap)) return 'verify'
  if (snap.questions && snap.questions.length > 0) return 'answers'
  return 'unknown'
}

/**
 * isCancelShaped names the three answers whose landed decoder honors the
 * person's `note` as the cancel's why (P3-RW-19, ratified OQ2): the verify /
 * ladder cards' `{choice:"cancel"}`, the intake approval card's
 * `{action:"cancel"}`, and the SPEC-DOUBT card's `{choice:"rethink"}` —
 * which IS a cancel: it ends the intake ("requester: rethink — intake
 * cancelled after SPEC-DOUBT"). Every other verb ignores the field
 * server-side, so this client sends it on no other.
 */
function isCancelShaped(item: ApprovalItem, action: string): boolean {
  switch (answerEnvelope(item)) {
    case 'verify':
    case 'contest':
      return action === 'cancel'
    case 'choice':
      return action === 'rethink'
    default:
      return false
  }
}

type ActInput = {
  pin: string
  reason: string
  /** The cancel's why — attached ONLY to a cancel-shaped answer. */
  note: string
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
      return { note: `recorded: ${res.disposition}`, detail: res.detail }
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
  // The cancel's why rides its cancel-shaped answer and no other (P3-RW-19):
  // the field's label promises the words go with the CANCEL, so a why typed
  // and then Approved sends nothing of it. Whitespace-only is no reason;
  // real words go verbatim.
  const why = isCancelShaped(item, action) && input.note.trim() !== '' ? { note: input.note } : {}
  if (item.kind === 'effect') {
    return input.reason === '' ? { action } : { action, reason: input.reason }
  }
  switch (answerEnvelope(item)) {
    case 'contest':
      // The S06.9 structured Re-plan entry: the contest names the criterion,
      // assumption or step being contested, and rides the same answer.
      return input.contest === '' ? { action, ...why } : { action, contest: { target: input.contest }, ...why }
    case 'choice':
      return input.criteria.length === 0
        ? { choice: action, ...why }
        : { choice: action, criteria: input.criteria, ...why }
    case 'verify':
      // The verify answer body (verify.DecodeAnswer): {choice}, with the
      // person's guidance riding revise_with_guidance — the server refuses a
      // guidance-less revise, and `canFire` keeps that button unarmed until
      // the text exists.
      if (action === 'revise_with_guidance') {
        return input.contest === '' ? null : { choice: action, guidance: [{ text: input.contest }] }
      }
      return { choice: action, ...why }
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

/**
 * criteriaChoices — the decision-card choice values whose answer CONSUMES a
 * criteria list (today: dropping a criterion names which ones). The picker
 * renders ONLY when the card offers such a choice: on every other decision
 * card — the onboarding digest, the family question — the detail lines are
 * context to read, and rendering them as checkboxes made a digest look like a
 * broken form (operator finding, 2026-08-12: "Which of these" over lines
 * nothing can pick).
 */
const criteriaChoices = ['drop_criterion']

function takesCriteria(item: ApprovalItem): boolean {
  const choices = asSnapshot(item.card).decision?.choices ?? []
  return choices.some((c) => criteriaChoices.includes(c.value))
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
    <fieldset className="criteria border-0 p-0">
      <legend className="text-sm text-muted-foreground">Which of these (where the choice needs one)</legend>
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
    <div className="verdict-form flex flex-col gap-2">
      <fieldset className="border-0 p-0" data-form="verdict">
        <legend className="text-sm text-muted-foreground">Which response is better</legend>
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
      <fieldset className="border-0 p-0" data-form="guess">
        <legend className="text-sm text-muted-foreground">
          Which one do you think was the platform&apos;s? (required — every vote carries a guess)
        </legend>
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
    <div className="disposition-form flex flex-col gap-2">
      <fieldset className="border-0 p-0" data-form="disposition">
        <legend className="text-sm text-muted-foreground">What should happen with this alarm</legend>
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
          // A batch answers many cards with ONE action and no per-card entry,
          // so it carries no why — a batched cancel is honestly reason-less.
          answer: composeAnswer(i, action, { pin: '', reason: '', note: '', contest: '', verdict: '', guess: '', criteria: [] }),
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
      <p className="max-w-prose text-sm text-muted-foreground">
        <span className="font-mono tabular-nums">{String(batchable.length)}</span> low-risk card
        {batchable.length === 1 ? '' : 's'} can be answered together. Pick the one action first — a batch answers cards
        that all accept it, and nothing else.
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
              {plainVerbs[a] ?? a}
            </option>
          ))}
        </select>
      </label>
      {action !== '' && (
        <ul className="batch-candidates">
          {eligible.map((i) => (
            <li key={i.id}>
              <label className="text-sm">
                <input
                  type="checkbox"
                  data-batch-pick={i.id}
                  checked={picked.includes(i.id)}
                  onChange={(e) => {
                    setPicked(e.currentTarget.checked ? [...picked, i.id] : picked.filter((p) => p !== i.id))
                  }}
                />
                {/* The plain issue line names the card the way the queue does;
                    the raw id stays findable in the row's technical fold. */}
                <span>{issueLine(i)}</span>
              </label>
            </li>
          ))}
        </ul>
      )}
      {action !== '' && (
        <Button
          variant="primary"
          size="sm"
          data-action="answer-batch"
          disabled={selected.length === 0}
          onClick={() => {
            void send()
          }}
        >
          {plainVerbs[action] ?? action} — {String(selected.length)} card{selected.length === 1 ? '' : 's'}
        </Button>
      )}
      {failure !== '' && <p className="text-sm text-[var(--red)]">{failure}</p>}
      {outcomes && (
        <ul className="batch-outcomes">
          {outcomes.map((o) => (
            <li
              key={o.id}
              className="wrap-anywhere text-sm"
              data-outcome-id={o.id}
              data-outcome-status={String(o.status)}
            >
              <span className="font-mono tabular-nums">
                {o.id} — {String(o.status)}
              </span>
              {o.code && <span className="text-[var(--yellow)]"> {o.code}</span>}
              {o.detail && <span className="text-muted-foreground"> {o.detail}</span>}
              {o.result && <span className="text-muted-foreground"> {o.result.detail}</span>}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
