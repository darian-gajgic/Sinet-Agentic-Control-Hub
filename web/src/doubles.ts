import { vi } from 'vitest'

import type { EventSourceLike } from './events'

import approvalsMineRaw from './fixtures/api/approvals-mine.json?raw'
import approvalsRaw from './fixtures/api/approvals.json?raw'
import benchmarkVerdictsRaw from './fixtures/api/benchmark-verdicts.json?raw'
import chatFilesRaw from './fixtures/api/chat-files.json?raw'
import chatSessionRaw from './fixtures/api/chat-session.json?raw'
import chatSessionEmptyRaw from './fixtures/api/chat-session-empty.json?raw'
import chatSessionsRaw from './fixtures/api/chat-sessions.json?raw'
import evalScoresRaw from './fixtures/api/eval-scores.json?raw'
import pricesRaw from './fixtures/api/prices.json?raw'
import pricesMemberRaw from './fixtures/api/prices-member.json?raw'
import settingsRaw from './fixtures/api/settings.json?raw'
import settingsHistoryRaw from './fixtures/api/settings-history.json?raw'
import settingsMemberRaw from './fixtures/api/settings-member.json?raw'
import acceptCardRaw from './fixtures/api/accept-card.json?raw'
import compareBaseRaw from './fixtures/api/compare-base.json?raw'
import compareBinaryRaw from './fixtures/api/compare-binary-cards.json?raw'
import compareImageRaw from './fixtures/api/compare-image-pair.json?raw'
import compareLineDiffRaw from './fixtures/api/compare-line-diff.json?raw'
import compareExtractedRaw from './fixtures/api/compare-extracted-text.json?raw'
import comparePdfRaw from './fixtures/api/compare-pdf-degrade.json?raw'
import deliverableReviewRaw from './fixtures/api/deliverable-review.json?raw'
import deliverableReworkRaw from './fixtures/api/deliverable-rework.json?raw'
import placedCommentsRaw from './fixtures/api/placed-comments.json?raw'
import previewsRaw from './fixtures/api/previews.json?raw'
import deliverableDetailRaw from './fixtures/api/deliverable-detail.json?raw'
import deliverablesRaw from './fixtures/api/deliverables-in-review.json?raw'
import deliverablesOfTaskRaw from './fixtures/api/deliverables-of-task.json?raw'
import catalogRaw from './fixtures/api/history-catalog.json?raw'
import queryAnswerRaw from './fixtures/api/history-query-answer.json?raw'
import viewAnswerRaw from './fixtures/api/history-view-answer.json?raw'
import viewsRaw from './fixtures/api/history-views.json?raw'
import metersRaw from './fixtures/api/meters.json?raw'
import runsRaw from './fixtures/api/runs.json?raw'
import receiptRaw from './fixtures/api/receipt.json?raw'
import runDetailRaw from './fixtures/api/run-detail.json?raw'
import taskDetailBareRaw from './fixtures/api/task-detail-bare.json?raw'
import taskDetailDraftRaw from './fixtures/api/task-detail-draft.json?raw'
import taskDetailOpsRaw from './fixtures/api/task-detail-ops.json?raw'
import taskDetailRaw from './fixtures/api/task-detail.json?raw'
import tasksRaw from './fixtures/api/tasks.json?raw'

/**
 * The offline harness every view test drives (P3/CONVENTIONS.md §42).
 *
 * Nothing here opens a connection to anything: no test dials 8481/8482, the
 * live front chain, any unit, or the network, and VITE_SINET_ADDR stays unset.
 *
 * THE BODIES ARE THE GOLDEN FIXTURES. `fixtures.tasks()` is the byte-for-byte
 * JSON an internal/api test asserts `GET /api/tasks` still serves. So a Go
 * handler that renames a field does not quietly become `undefined` in a view —
 * it fails the GO suite, in the same commit, naming the read. One shape, two
 * consumers.
 */

/** A scripted EventSource, in the landed shape (events.test.ts). */
export class FakeSource implements EventSourceLike {
  static made: FakeSource[] = []
  readonly url: string
  readyState = 0
  private listeners = new Map<string, ((ev: Event) => void)[]>()

  constructor(url: string) {
    this.url = url
    FakeSource.made.push(this)
  }

  static reset(): void {
    FakeSource.made = []
  }

  static last(): FakeSource {
    return FakeSource.made[FakeSource.made.length - 1]
  }

  addEventListener(type: string, listener: (ev: Event) => void): void {
    const list = this.listeners.get(type) ?? []
    list.push(listener)
    this.listeners.set(type, list)
  }

  close(): void {
    this.readyState = 2
  }

  open(): void {
    this.readyState = 1
    this.fire(new Event('open'))
  }

  /** send delivers one delta frame under its stored event type — the transport
   *  names every frame with its type, which is why subscriptions declare the
   *  types they consume. */
  send(type: string, data: unknown): void {
    this.fire(new MessageEvent(type, { data: JSON.stringify(data) }))
  }

  private fire(ev: Event): void {
    for (const l of this.listeners.get(ev.type) ?? []) l(ev)
  }
}

/** One frame, with the fields the client routes on. */
export function frame(seq: number, type: string, topics: string[] = ['board']) {
  return { seq, user_id: 'alice', type, schema_version: 1, topics, payload: {}, ts: '2026-07-20T09:05:00Z' }
}

export type Scripted = { body?: unknown; status?: number }

export type FetchLog = {
  calls: { method: string; path: string; body: unknown }[]
  /** Replace one route's answer mid-test, to prove a re-read really re-reads. */
  set(key: string, route: Scripted): void
}

/**
 * scriptedFetch installs a control plane made of a routing table. An unscripted
 * request FAILS the test loudly rather than resolving to undefined — a view
 * that quietly calls something nobody scripted is exactly the drift these
 * tests exist to catch.
 */
/**
 * recordBody keeps what was actually sent.
 *
 * Almost every verb sends JSON and a parsed object is what a test wants to assert
 * on. The exchange upload does NOT: its body is the raw object itself, because
 * the client never decodes or re-encodes a person's file. So a non-JSON body is
 * recorded AS THE VALUE SENT rather than making the double throw — the assertion
 * on it is then about the real thing (a Blob and its size), not about a string
 * somebody serialized to keep a test helper happy.
 */
function recordBody(body: BodyInit | null | undefined): unknown {
  if (body === null || body === undefined) return undefined
  if (typeof body !== 'string') return body
  try {
    return JSON.parse(body)
  } catch {
    return body
  }
}

export function scriptedFetch(table: Record<string, Scripted>): FetchLog {
  const routes = { ...table }
  const calls: { method: string; path: string; body: unknown }[] = []
  const impl = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input)
    const method = init?.method ?? 'GET'
    calls.push({ method, path, body: recordBody(init?.body) })
    const route = routes[`${method} ${path}`]
    if (!route) throw new Error(`unscripted request: ${method} ${path}`)
    const status = route.status ?? 200
    return {
      ok: status >= 200 && status < 300,
      status,
      statusText: '',
      json: async () => route.body ?? {},
    } as Response
  })
  vi.stubGlobal('fetch', impl)
  return {
    calls,
    set(key, route) {
      routes[key] = route
    },
  }
}

/** signedIn is the session every view test runs under: an operator, so the
 *  household altitude (every owner's rows) is what the views receive. */
export const signedIn: Scripted = {
  body: { authenticated: true, user: { user_id: 'alice', display_name: 'Alice', role: 'operator', pin_set: true } },
}

function parse<T>(raw: string): T {
  return JSON.parse(raw) as T
}

/** Fresh parses each call, so one test mutating a body cannot reach another. */
export const fixtures = {
  tasks: () => parse<Record<string, unknown>>(tasksRaw),
  runs: () => parse<Record<string, unknown>>(runsRaw),
  meters: () => parse<Record<string, unknown>>(metersRaw),
  historyViews: () => parse<Record<string, unknown>>(viewsRaw),
  historyCatalog: () => parse<Record<string, unknown>>(catalogRaw),
  historyViewAnswer: () => parse<Record<string, unknown>>(viewAnswerRaw),
  historyQueryAnswer: () => parse<Record<string, unknown>>(queryAnswerRaw),
  taskDetail: () => parse<Record<string, unknown>>(taskDetailRaw),
  taskDetailDraft: () => parse<Record<string, unknown>>(taskDetailDraftRaw),
  taskDetailBare: () => parse<Record<string, unknown>>(taskDetailBareRaw),
  receipt: () => parse<Record<string, unknown>>(receiptRaw),
  approvals: () => parse<Record<string, unknown>>(approvalsRaw),
  /** The same read as its OWNER: `answerable` is per-caller (D10), so the
   *  "yours to answer" branch only exists in a body an owner read. */
  approvalsMine: () => parse<Record<string, unknown>>(approvalsMineRaw),
  /** The blind-pair form's data, read as the requester: the two renders and the
   *  registered answer vocabularies the form's buttons come from. */
  benchmarkVerdicts: () => parse<Record<string, unknown>>(benchmarkVerdictsRaw),
  /** The S15.9 settings surface, read BOTH ways: `editable` is computed per
   *  caller, so the operator's write surface and a member's read-only view are
   *  two served bodies rather than two renders. */
  settings: () => parse<Record<string, unknown>>(settingsRaw),
  settingsMember: () => parse<Record<string, unknown>>(settingsMemberRaw),
  settingsHistory: () => parse<Record<string, unknown>>(settingsHistoryRaw),
  prices: () => parse<Record<string, unknown>>(pricesRaw),
  pricesMember: () => parse<Record<string, unknown>>(pricesMemberRaw),
  evalScores: () => parse<Record<string, unknown>>(evalScoresRaw),
  /**
   * The S15.7 assistant world, read as alice — there IS no operator body to
   * commit, because the operator does not read another member's transcripts, and
   * that absence is the disposition rather than an omission.
   *
   * `chatSession` deliberately carries every render the widget owes: a Layer-0
   * deterministic answer, a disambiguation card whose choices are real catalog
   * queries, the S06 handoff with its OPEN intake card AND a non-empty produced
   * chip, a Layer-2 escalation carrying `lower-confidence` with its audit, and a
   * turn left RUNNING for the in-flight re-attach. `chatSessions` holds a second,
   * empty session in the honest untitled state.
   */
  chatSessions: () => parse<Record<string, unknown>>(chatSessionsRaw),
  chatSession: () => parse<Record<string, unknown>>(chatSessionRaw),
  /** The empty session. It is where a driven SEND belongs: one turn at a time is
   *  a per-session rule, so the rich body's own running turn is a conversation the
   *  composer is right to refuse a second turn into. */
  chatSessionEmpty: () => parse<Record<string, unknown>>(chatSessionEmptyRaw),
  chatFiles: () => parse<Record<string, unknown>>(chatFilesRaw),
  taskDetailOps: () => parse<Record<string, unknown>>(taskDetailOpsRaw),
  deliverablesInReview: () => parse<Record<string, unknown>>(deliverablesRaw),
  deliverablesOfTask: () => parse<Record<string, unknown>>(deliverablesOfTaskRaw),
  deliverableDetail: () => parse<Record<string, unknown>>(deliverableDetailRaw),
  runDetail: () => parse<Record<string, unknown>>(runDetailRaw),
  /**
   * The S15.8 review world (B6-8), every body produced by S13's own verbs.
   *
   * `deliverableReview` is the in-review code deliverable: two real revisions, the
   * accept and both preview doors OPEN, and the request-revision door CLOSED with
   * its narrative reason. `deliverableRework` is the other limb — a deliverable
   * whose task is parked on a rework card, so that door is live and carries the
   * ask, the answer verb and the card's own pin. No single deliverable can serve
   * both, because the door's state IS the deliverable's state.
   */
  deliverableReview: () => parse<Record<string, unknown>>(deliverableReviewRaw),
  deliverableRework: () => parse<Record<string, unknown>>(deliverableReworkRaw),
  /** The round-over-round default read, with a REAL host-side unified diff whose
   *  headers name the deliverable's own paths. */
  compareLineDiff: () => parse<Record<string, unknown>>(compareLineDiffRaw),
  /** `old=0` — the pre-task base, which is a navigation target and not a revision. */
  compareBase: () => parse<Record<string, unknown>>(compareBaseRaw),
  compareImagePair: () => parse<Record<string, unknown>>(compareImageRaw),
  compareBinaryCards: () => parse<Record<string, unknown>>(compareBinaryRaw),
  /** The PDF degrade: extraction could not run, so the surface falls back to the
   *  metadata cards with the reason ON THE LABEL — an answer, never a refusal. */
  comparePdfDegrade: () => parse<Record<string, unknown>>(comparePdfRaw),
  /** The honest FALLBACK diff: a real unified diff under a label saying this type
   *  has no rich comparison at v0. A different render from the PDF degrade, which
   *  falls past it to the metadata cards. */
  compareExtractedText: () => parse<Record<string, unknown>>(compareExtractedRaw),
  /** All five placement statuses over one revision hop, an open set beside a
   *  consumed one, and a verification finding under the same schema. */
  placedComments: () => parse<Record<string, unknown>>(placedCommentsRaw),
  acceptCard: () => parse<Record<string, unknown>>(acceptCardRaw),
  previews: () => parse<Record<string, unknown>>(previewsRaw),
}

/** The routes the B6-5 oversight surfaces read, answered from the fixtures. */
export function oversightRoutes(): Record<string, Scripted> {
  return {
    'GET /api/auth/session': signedIn,
    'GET /api/tasks': { body: fixtures.tasks() },
    'GET /api/runs': { body: fixtures.runs() },
    'GET /api/meters': { body: fixtures.meters() },
    'GET /api/events/views': { body: fixtures.historyViews() },
    'GET /api/events/catalog': { body: fixtures.historyCatalog() },
    // The signed-in identity in these suites is alice, so the inbox read is
    // the one SHE would get — including the cards she can answer.
    'GET /api/approvals': { body: fixtures.approvalsMine() },
    'GET /api/benchmark/verdicts': { body: fixtures.benchmarkVerdicts() },
    'GET /api/settings': { body: fixtures.settings() },
    'GET /api/settings/prices': { body: fixtures.prices() },
    'GET /api/events/query/verdicts.eval_scores': { body: fixtures.evalScores() },
    'GET /api/settings/freshness.max_age/history': { body: fixtures.settingsHistory() },
    'GET /api/deliverables?state=in-review': { body: fixtures.deliverablesInReview() },
    'GET /api/deliverables?task=t-ship': { body: fixtures.deliverablesOfTask() },
    'GET /api/deliverables/d-notes': { body: fixtures.deliverableDetail() },
    // The task's second deliverable, shaped from the served list row so the
    // detail chain has both to walk.
    'GET /api/deliverables/d-changelog': {
      body: {
        // The DETAIL shape keys the deliverable as `id` (review.Deliverable),
        // where the list row keys it `deliverable_id` — served as served.
        deliverable: {
          id: 'd-changelog',
          owner: 'alice',
          task_id: 't-ship',
          project_id: 'release-notes',
          type: 'text',
          current_revision: 1,
          state: 'in-review',
          created_ts: '2026-07-20T09:02:00Z',
          updated_ts: '2026-07-20T09:04:00Z',
        },
        revisions: [
          {
            deliverable_id: 'd-changelog',
            n: 1,
            owner: 'alice',
            run_id: 'r-ship',
            pin_kind: 'content',
            content_sha256: 'sha-d-changelog-1',
            platform_ref: 'notes/CHANGELOG.md',
            created_ts: '2026-07-20T09:02:00Z',
          },
        ],
        cursor: 13,
      },
    },
    // The three review deliverables the B6-8 fixture world minted on the same
    // task. The task detail reads EVERY deliverable's own detail (the revisions
    // are records, not a count), so a body missing here is an unscripted request
    // and the block renders nothing.
    [`GET /api/deliverables/${reviewDeliverableID}`]: { body: fixtures.deliverableReview() },
    [`GET /api/deliverables/${imageDeliverableID}`]: { body: imageDetail() },
    [`GET /api/deliverables/${binaryDeliverableID}`]: { body: binaryDetail() },
    [`GET /api/deliverables/${notebookDeliverableID}`]: { body: notebookDetail() },
    'GET /api/runs/r-ship': { body: fixtures.runDetail() },
    'GET /api/tasks/t-ops': { body: fixtures.taskDetailOps() },
    'GET /api/deliverables?task=t-ops': { body: { deliverables: [], cursor: 15, truncated: false } },
  }
}

/** The review surface's ids, the fixture world's own. */
export const reviewDeliverableID = 'd-site'
export const reworkDeliverableID = 'd-brief'
export const imageDeliverableID = 'd-hero'
export const binaryDeliverableID = 'd-bundle'
export const notebookDeliverableID = 'd-notebook'

/** The reads the S15.8 review surface makes, answered from its own golden bodies.
 *  The compare keys carry NO query string for the default read, because the
 *  server's own round-over-round default is what "no parameters" means. */
export function reviewRoutes(): Record<string, Scripted> {
  return {
    ...oversightRoutes(),
    [`GET /api/deliverables/${reviewDeliverableID}`]: { body: fixtures.deliverableReview() },
    [`GET /api/deliverables/${reviewDeliverableID}/compare`]: { body: fixtures.compareLineDiff() },
    [`GET /api/deliverables/${reviewDeliverableID}/compare?old=0&new=2`]: { body: fixtures.compareBase() },
    [`GET /api/deliverables/${reviewDeliverableID}/comments?revision=2`]: { body: fixtures.placedComments() },
    [`GET /api/deliverables/${reviewDeliverableID}/accept-card`]: { body: fixtures.acceptCard() },
    [`GET /api/deliverables/${reworkDeliverableID}`]: { body: fixtures.deliverableRework() },
    [`GET /api/deliverables/${reworkDeliverableID}/compare`]: { body: fixtures.comparePdfDegrade() },
    [`GET /api/deliverables/${reworkDeliverableID}/comments?revision=2`]: { body: fixtures.placedComments() },
    [`GET /api/deliverables/${reworkDeliverableID}/accept-card`]: { body: fixtures.acceptCard() },
    [`GET /api/deliverables/${imageDeliverableID}`]: { body: imageDetail() },
    [`GET /api/deliverables/${imageDeliverableID}/compare`]: { body: fixtures.compareImagePair() },
    [`GET /api/deliverables/${binaryDeliverableID}`]: { body: binaryDetail() },
    [`GET /api/deliverables/${binaryDeliverableID}/compare`]: { body: fixtures.compareBinaryCards() },
    [`GET /api/deliverables/${notebookDeliverableID}`]: { body: notebookDetail() },
    [`GET /api/deliverables/${notebookDeliverableID}/compare`]: { body: fixtures.compareExtractedText() },
    [`GET /api/deliverables/${notebookDeliverableID}/comments?revision=2`]: { body: fixtures.placedComments() },
    'GET /api/previews': { body: fixtures.previews() },
  }
}

/**
 * The two object-surface deliverables' DETAILS, derived from the review detail by
 * swapping the identity fields the served comparison already pins.
 *
 * DECLARED DOWNGRADE (OQ8): these two bodies are derived rather than golden. The
 * COMPARISONS they are read alongside are real committed bodies produced by real
 * mints — which is where the surface's whole render comes from — and a third and
 * fourth committed detail would add two more fixture-world seedings to prove
 * nothing the first detail does not already prove about the detail shape.
 */
function imageDetail(): Record<string, unknown> {
  return objectDetail(imageDeliverableID, 'image', 'site/hero.png')
}

function binaryDetail(): Record<string, unknown> {
  return objectDetail(binaryDeliverableID, 'binary', 'dist/site.tar')
}

function notebookDetail(): Record<string, unknown> {
  return objectDetail(notebookDeliverableID, 'notebook', 'analysis/report.ipynb')
}

function objectDetail(id: string, type: string, subject: string): Record<string, unknown> {
  const detail = fixtures.deliverableReview() as {
    deliverable: Record<string, unknown>
    revisions: Record<string, unknown>[]
    doors: Record<string, unknown>[]
  }
  detail.deliverable = { ...detail.deliverable, id, type, subject_ref: subject }
  detail.revisions = detail.revisions.map((r) => ({ ...r, deliverable_id: id }))
  detail.doors = detail.doors.map((d) => ({
    ...d,
    route: String(d.route ?? '').replace(reviewDeliverableID, id),
  }))
  return detail as unknown as Record<string, unknown>
}

/** The reads the S15.7 assistant surface makes, answered from its own golden
 *  fixtures. Both ids are the fixture world's real ones, because those are the
 *  ids the committed bodies carry. */
export const chatSessionID = 'cs-0000000000000001'
export const chatEmptySessionID = 'cs-0000000000000014'

export function chatRoutes(): Record<string, Scripted> {
  return {
    ...oversightRoutes(),
    'GET /api/chat/sessions': { body: fixtures.chatSessions() },
    [`GET /api/chat/sessions/${chatSessionID}`]: { body: fixtures.chatSession() },
    [`GET /api/chat/sessions/${chatEmptySessionID}`]: { body: fixtures.chatSessionEmpty() },
    'GET /api/chat/files': { body: fixtures.chatFiles() },
  }
}
