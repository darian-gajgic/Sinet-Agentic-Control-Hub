/**
 * The typed client for the S15.2 read surface: readiness, the S01.9 session
 * stack, and the oversight, decision, settings and assistant families the built
 * views render. The review family belongs to the packet that builds that view.
 *
 * Every type below is the shape the Go handler SERVES — pinned by the golden
 * fixtures under src/fixtures/api/, which an internal/api test asserts the real
 * handlers still produce. A field that drifts fails the Go suite rather than
 * quietly rendering as `undefined` here.
 *
 * Two rules the whole client rests on:
 *  - The session cookie is HttpOnly, so JS never reads it and never stores a
 *    token. Identity comes from GET /api/auth/session and from nowhere else.
 *  - Every request is same-origin with credentials, because that cookie is the
 *    only thing that authenticates a call (Spec S01.9 layer 3).
 */

export type User = {
  user_id: string
  display_name: string
  role: string
  pin_set: boolean
}

/** The S01.9 layer-2 device hint: the login-picker prefill contract. */
export type DeviceHint = {
  device_login: string
  user_id?: string
  auto_login?: boolean
}

export type Session = {
  authenticated: boolean
  dev?: boolean
  user?: User
  hint?: DeviceHint
}

export type Health = {
  ready: boolean
  mode: string
  version: string
  /** The snapshot-then-tail cursor bootstrap (Spec S15.3). */
  event_head: number
}

/**
 * ApiError carries the HTTP status so callers can branch on 401 alone.
 *
 * It also carries the machine surface's OWN error envelope: `code` is the
 * server's `error` field and `body` the whole JSON. Both exist so a caller can
 * branch on the code the server assigned — `stale_payload`, `pin_required`,
 * `already_answered` — rather than re-classifying a failure by reading its
 * message text, which is the server's classification to make and not the
 * client's (§30/§38). `stale_payload` carries the FRESH card in `body.current`,
 * and that is the only reason the body is kept at all.
 */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly body: unknown

  constructor(status: number, message: string, code = '', body?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.body = body
  }
}

/** Unreachable is a transport failure — the host is asleep or the tailnet is
 * down. It is a different fact from a server that answered with an error, and
 * the connection indicator shows it as one (Spec S15.12). */
export class Unreachable extends Error {
  constructor(cause: unknown) {
    super('control plane unreachable')
    this.name = 'Unreachable'
    this.cause = cause
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(path, {
      ...init,
      credentials: 'same-origin',
      headers: { Accept: 'application/json', ...(init?.headers ?? {}) },
    })
  } catch (cause) {
    throw new Unreachable(cause)
  }
  if (!res.ok) throw await apiError(res)
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

/** apiError reads the machine surface's own {error, detail} envelope and
 * degrades to the status text — it never invents a reason, and it never
 * re-derives a classification the server already made. */
async function apiError(res: Response): Promise<ApiError> {
  try {
    const body: unknown = await res.json()
    if (body && typeof body === 'object') {
      const { error, detail } = body as { error?: string; detail?: string }
      const message = detail ?? error ?? `${res.status} ${res.statusText}`.trim()
      return new ApiError(res.status, message, error ?? '', body)
    }
  } catch {
    // Not JSON. The status is what we have, and saying so is honest.
  }
  return new ApiError(res.status, `${res.status} ${res.statusText}`.trim())
}

function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

// ── the S15.2 oversight families ──────────────────────────────────────────

/**
 * TaskListRun is one task's latest run at the S1.3 card-face grain.
 *
 * The six face elements are: the task's title and owner (on the task, below),
 * `stage`, `effort_mode` + any `downgrade_note`, `cost_so_far_usd`, and
 * `waiting_on_human`. Every nullable field is an HONEST ABSENCE the view
 * renders as one: a null cost is "no meter reading", never $0, and a missing
 * downgrade note means the platform disclosed none.
 */
export type TaskListRun = {
  run_id: string
  state: string
  wedged: boolean
  last_activity_ts: string | null
  stage: string
  waiting_on_human: boolean
  parked_until: string | null
  cost_so_far_usd: number | null
  effort_mode: string
  downgrade_note?: string
  /** The recorded drag order of the task's queued runs; 0 is "no hint" and
   *  null means the task has no queued run at all. */
  queue_hint_rank: number | null
}

export type TaskListItem = {
  task_id: string
  owner: string
  title: string
  kanban_status: string
  /** '(no project)' is a real, selectable bucket — never a dropped row. */
  project: string
  created_ts: string
  latest_run: TaskListRun | null
}

export type TaskList = { tasks: TaskListItem[]; cursor: number; truncated: boolean }

export type RunListItem = {
  run_id: string
  owner: string
  task_id: string
  state: string
  waiting_on_human: boolean
  parked_until: string | null
  wedged: boolean
  stage: string
  lane: string
  generation: number
  created_ts: string
  updated_ts: string
  last_activity_ts: string | null
}

export type RunList = { runs: RunListItem[]; cursor: number; truncated: boolean }

/** One (person, lane) meter reading. `pressure` exists only when
 *  `pressure_applicable` — a fabricated denominator is worse than no number. */
export type MeterLane = {
  owner: string
  lane: string
  weighted_consumption: number
  cache_read_weight: number
  assumed: boolean
  pressure_applicable: boolean
  pressure: number | null
  budget_declared: boolean
  budget_remaining: number | null
  total_runs: number
  active_runs: number
  parked_runs: number
  parked_until: string | null
}

/** A disambiguation card is an ANSWER — the honest "which did you mean" — and
 *  renders as one, never as an error. */
export type Disambiguation = {
  question: string
  reason: string
  choices: { query: string; category: string; description: string }[]
}

/** The Layer-2 audit block. Present on open-SQL answers INCLUDING refusals,
 *  which is exactly when it matters most. */
export type OpenSQLAudit = {
  question: string
  outcome: string
  sql_generated?: string
  sql_executed?: string
  views?: string[]
  refusal?: string
  row_count: number
  truncated: boolean
  model?: string
  alias: string
  as_operator: boolean
}

/** The one shape every S14.10 layer returns. `confidence` is set with `layer`
 *  by the store's own constructor, so an answer can never mislabel its floor. */
export type Answer = {
  layer: number
  query: string
  confidence: string
  question?: string
  columns: string[]
  rows: unknown[][]
  row_count: number
  truncated: boolean
  notes?: string[]
  card?: Disambiguation
  audit?: OpenSQLAudit
}

/** A Layer-0 view served at its OWN grain. `absent` carries the reason the
 *  surface could not answer — rendered as the absence it is. */
export type MeterView = { answer?: Answer; absent?: string }

export type Meters = {
  lanes: MeterLane[]
  burn_rates: MeterView
  budgets: MeterView
  limit_events: MeterView
  per_person: MeterView
  per_period: MeterView
  cursor: number
}

/**
 * The Layer-0 view registry entry.
 *
 * The keys really are capitalized: `history.View` carries no json tags, so the
 * Go field names go on the wire as-is. Consumed as served — renaming a landed
 * read's keys is a contract change, not a tidy-up (reported, not papered over).
 */
export type HistoryView = {
  Name: string
  Question: string
  OwnerColumn: string
  Description: string
  Order: string
}

export type HistoryQuery = {
  name: string
  category: string
  description: string
  slots: { name: string; type: string; required?: boolean }[] | null
}

/** The choice surface, served from the packages' own registries. A client that
 *  hand-listed these would be maintaining a second copy of them. */
export type HistoryRegistry = {
  views: HistoryView[] | null
  cost_questions: string[] | null
  categories: string[] | null
  axes: string[] | null
  queries: HistoryQuery[] | null
  query_names: string[] | null
}

// ── the task detail (S15.5 ¶3) ────────────────────────────────────────────

/** One numbered acceptance criterion. `structured` is the optional EARS/GWT
 *  restatement; `plain` is always present. */
export type AC = { n: number; plain: string; structured?: string; structured_kind?: string }

/** The SPEC as the pipeline stored it. `status` is the §38 ruling-(a) fact the
 *  view must render honestly: a draft pair is labelled a draft, and is never
 *  presented as the confirmed specification. */
export type Spec = {
  task_id: string
  owner: string
  version: number
  status: string
  tier?: string
  provenance?: string
  restatement: string
  outcome?: string[]
  acs: AC[]
  constraints?: string[]
  assumptions?: { text: string; basis?: string }[]
  out_of_scope?: string[]
  clarifications?: string[]
}

export type Plan = {
  task_id: string
  owner: string
  version: number
  spec_version: number
  status: string
  steps: { id: string; title: string }[]
  coverage: Record<string, string[]>
  risks?: string[]
}

/** One entry of the per-stage story, derived from the log. No percentage: the
 *  stages that happened are facts, "how far along" is not one. */
export type StageStep = {
  run_id: string
  seq: number
  type: string
  stage: string
  kind: string
  outcome?: string
  ts: string
}

export type TaskSuccessor = { task_id: string; deliverable_id: string; revision_n: number; created_ts: string }

export type TaskLineage = {
  project: string
  /** > 1 means the task claimed more than one project: the ambiguity renders,
   *  it is never collapsed to a winner. */
  project_choices: number
  succeeds: TaskSuccessor[]
  succeeded_by: TaskSuccessor[]
}

/** One human decision along the way (S2.4): who decided what, and when. */
export type TaskDecision = {
  seq: number
  type: string
  ts: string
  run_id?: string
  actor: string
  actor_is_operator?: boolean
  card_id: string
  card_type: string
  decision: string
  subject?: string
  reason?: string
  decided_at?: string
}

/** The receipt as internal/metering stored it, served verbatim. `items` carries
 *  Go field names because metering.LineItem has no json tags — consumed as
 *  served (renaming a landed read's keys is a contract change). */
export type Receipt = {
  run_id: string
  user_id: string
  items: {
    Model: string
    Lane: string
    Purpose: string
    Calls: number
    PromptTokens: number
    BilledOutputTokens: number
    PricedUSD: number
    PricedCalls: number
    UnpricedCalls: number
    Currency: string
  }[]
  currency: string
  total_priced_usd: number
  total_calls: number
  total_unpriced_calls: number
  worst_tier: number
  park_history?: {
    parked_at: string
    resumed_at?: string
    duration_seconds?: number
    park_reason?: string
    resume_cause?: string
    ongoing?: boolean
  }[]
  mode: { note: string }
  /** `label` is the registered done-directly text, served verbatim. The UI
   *  renders the string it is given and declares none of its own. */
  direct_use: {
    label: string
    formula_ref: string
    unpriced: boolean
    reason?: string
    heuristic_usd?: number
    currency: string
    measured_stage_seam?: string
  }
  materialized_ts: string
}

export type TaskRunView = { run_id: string; state: string; created_ts: string; receipt?: Receipt; receipt_absent?: string }

export type TaskDetail = {
  task_id: string
  owner: string
  title: string
  kanban_status: string
  created_ts: string
  cursor: number
  spec: Spec | null
  plan: Plan | null
  /** Why there is no pair — a fact about the task, not an error hiding it. */
  artifacts_absent?: string
  stage_progress: StageStep[]
  lineage: TaskLineage
  runs: TaskRunView[]
  decisions: TaskDecision[]
}

/** The run card: the S14.3 progress figures a live view reads. Counters are
 *  MONOTONIC — there is no denominator anywhere, so nothing here can become a
 *  percentage. */
export type RunCard = {
  run_id: string
  owner: string
  state: string
  waiting_on_human: boolean
  parked_until: string | null
  stage: string
  tool: { name: string; args_digest: string } | null
  /** `api_equiv_cost_usd` is null when the run has no cost reading. Tokens stay
   *  a plain counter beside it: a monotonic count at zero is a true reading of a
   *  run that has consumed nothing, while a money zero is a price nobody set. */
  counters: { tokens: number; api_equiv_cost_usd: number | null; elapsed_s: number; steps: number }
  last_activity: { type: string; ts: string; line: string } | null
  wedged: boolean
  lane: string
  generation: number
}

/** What a client needs to go live on one run: the cursor the snapshot was taken
 *  at, the topic and run to subscribe with, and the last line already known. */
export type LiveActivityRefs = {
  cursor: number
  topic: string
  run_id: string
  last: { type: string; ts: string; line: string } | null
}

export type RunDetail = {
  card: RunCard
  live_activity: LiveActivityRefs
  spawn_records: { seq: number; type: string; ts: string; payload: unknown }[]
  routing_records: { seq: number; type: string; ts: string; payload: unknown }[]
  cursor: number
}

// ── the what-needs-me feeds (S1.4) ────────────────────────────────────────

/**
 * The D10 co-approval state of a proposed effect, DERIVED server-side from the
 * decision.recorded rows and cycle-scoped there. The inbox renders this block
 * verbatim and infers nothing about who signed from event frames: the served
 * derivation is the one truth (B6-6 OQ8).
 */
export type EffectApprovals = {
  platform_level: boolean
  owner_approved: boolean
  operator_approved: boolean
  owner_approved_by?: string
  operator_approved_by?: string
}

/**
 * One inbox card, ranked by risk server-side.
 *
 * Three fields carry all the AUTHORITY this surface has: `answerable` (with the
 * reason when it is false), `actions` — the card's OWN verb vocabulary — and the
 * two tier flags. The UI renders controls from them and invents none: a card
 * that does not name an action has no button for it, and a card that says it is
 * not answerable renders its served reason instead of a dead control (D9).
 *
 * There is no percent, fraction or ETA field on the wire and none is derived
 * from one here (§30).
 */
export type ApprovalItem = {
  /** "<kind>:<native id>" — the id the answer verbs and /inbox/:id take. */
  id: string
  kind: string
  owner: string
  run_id?: string
  tier: string
  answerable: boolean
  not_answerable_reason?: string
  batchable: boolean
  step_up_required: boolean
  /** The pin an answer must quote back (S15.2). Absent on the kinds whose verbs
   *  take no hash — the oversight cards. */
  payload_hash?: string
  observed_ts: string
  expiry_at?: string
  engine_expiry_ts?: string
  stale?: boolean
  stale_reasons?: string[]
  actions?: string[]
  approvals?: EffectApprovals
  /** The card content: the stored ask snapshot (Layer-1 + Layer-2 + the 13.5
   *  help block), the effect payload, or the projection row of the other kinds.
   *  Rendered as DATA — every string in it escapes, because a card body is
   *  model-derived input (§41-B). */
  card?: unknown
}

export type ApprovalList = { items: ApprovalItem[]; cursor: number; truncated?: boolean }

/** One answered card. `applied:false` is the honest repeat — the item was
 *  already resolved and this request changed nothing, which is exactly what
 *  makes a phone retry safe (S15.2). */
export type ApprovalAnswerResult = {
  id: string
  applied: boolean
  state: string
  result?: unknown
  approvals?: EffectApprovals
  detail?: string
}

/** One member of a Low-tier batch. A batch is a transport convenience: each
 *  item carries ITS OWN pin and its answer in ITS OWN card's vocabulary. */
export type ApprovalBatchItem = { id: string; payload_hash: string; answer: unknown }

/** One member's own outcome. A refusal of one item leaves the rest applied, so
 *  each is rendered beside its siblings rather than collapsed into a banner. */
export type ApprovalBatchOutcome = {
  id: string
  status: number
  code?: string
  detail?: string
  result?: ApprovalAnswerResult
}

export type ApprovalBatchResult = { outcomes: ApprovalBatchOutcome[]; cursor: number }

/** The 409 the answer verb serves when the card moved under the answerer: the
 *  FRESH card travels with the refusal, so the next act is a re-render rather
 *  than a guess or a blind retry (S15.2). */
export type StalePayload = { error: string; detail: string; current: ApprovalItem }

/** The S02.3 parked→running edge, as internal/stage records it. `generation` is
 *  the bump that makes a second resume inert, and `detail` is the platform's own
 *  account of what it did — this client authors none of its own. */
export type RunResumed = {
  run_id: string
  from: string
  to: string
  applied: boolean
  generation: number
  detail: string
}

export type FlagSuppressed = { run_id?: string; anomaly_class: string; suppressed: boolean; detail: string }

export type DriftDismissed = {
  card_id: string
  fingerprint: string
  window_start_seq: number
  dismissed: boolean
  detail: string
}

/** `still_red` is always true and is in the shape on purpose: an
 *  acknowledgement is not a pass, and this surface renders it as exactly that. */
export type ConformanceAcknowledged = {
  card_id: string
  row_id: string
  last_run_ts: string
  acknowledged: boolean
  still_red: boolean
  detail: string
}

/**
 * The blind-pair form's data (BENCH-REG §3.2/§3.3).
 *
 * `pairs` is the benchmark package's own pre-record shape, passed through: it
 * carries no arm, no position and no model, structurally. The three vocabulary
 * lists are REGISTERED text marshaled by the package that owns the
 * registration — the form renders the buttons it is served and declares none of
 * its own (B6-6 OQ4).
 */
export type PendingPair = {
  pair_id: string
  user_id: string
  domain: string
  task_id: string
  sampled_ts: string
  render_a: string
  render_b: string
  length_a: number
  length_b: number
}

export type BenchmarkVerdictForms = {
  pairs: PendingPair[] | null
  guess_required: boolean
  choices: string[] | null
  guess_sides: string[] | null
  dispositions: string[] | null
  detail: string
}

/**
 * The committed §14 record, read back after the verdict landed — the ONLY place
 * arm identity is readable (BENCH-REG §3.4).
 *
 * These are the fields the post-record promise is ABOUT: which blind side was
 * the platform's, the two arms' observed model identities, and whether the
 * mandatory guess was right (the §5 blindness measurement). Typing them is what
 * lets the form render the answer the voter just earned instead of only saying
 * that an answer exists.
 */
export type VerdictReveal = {
  pair_id: string
  platform_side: string
  platform_model: string
  direct_model: string
  verdict: string
  platform_guess: string
  guess_correct: boolean
  epoch_id: string
  recorded_ts: string
}

/** The reveal is a READ of the committed record. Its absence with
 *  `recorded:true` is the honest late-reveal branch, not a failed vote. */
export type VerdictRecorded = { pair_id: string; recorded: boolean; reveal?: VerdictReveal; detail: string }

export type VerdictDeclined = { pair_id: string; declined: boolean; detail: string }

export type AlarmDispositioned = {
  card_id: string
  domain: string
  disposition: string
  cleared: boolean
  detail: string
}

/** The ninth inbox kind's card: one open memory conflict, folded server-side
 *  and visible ONLY to its addressee (B6-6 OQ1). */
export type MemoryConflict = {
  conflict_id: number
  affected_owner: string
  entry_id: string
  other_entry_id: string
  topic_key?: string
  question: string
  status: string
  detected_ts: string
  resolved_by?: string
  resolved_ts?: string
}

export type MemoryConflictResolved = { conflict: MemoryConflict; detail: string }

// ── the S15.9 settings family ─────────────────────────────────────────────

/**
 * One declared setting, as the REGISTRY computes it.
 *
 * Everything here is the registry's own answer: what the effective value is,
 * what the EFFECTIVE clamp bounds are (the operator's override where set, else
 * the ratified clamp — G1 rider 1), whether an override row exists at all, and
 * the help text. The UI re-derives none of it. `overridden` is what keeps
 * "equals the default" and "is explicitly set to the default" distinguishable,
 * which is the whole reason reset-to-default can be shown as a real act.
 */
export type SettingValue = {
  key: string
  section: string
  domain: string
  title: string
  help: string
  type: string
  unit?: string
  ratified_by: string
  dormant?: string
  default: unknown
  effective: unknown
  overridden: boolean
  /** The per-user overrides in force. A member's read carries only their own —
   *  the server narrows it, and no client filtering creates that privacy. */
  user_values?: Record<string, unknown>
  numeric: boolean
  floor?: number
  ceiling?: number
  min_exclusive?: boolean
  enum?: string[]
  auto: boolean
  per_user: boolean
  restart_required: boolean
}

/** The whole settings surface in one read. `schema` and `uischema` are the
 *  registry's OWN emissions, served verbatim: one artifact set drives
 *  validation, the UI and the docs, so they cannot drift (S01.10(b)). */
export type SettingsView = {
  schema: unknown
  uischema: unknown
  values: SettingValue[]
  domains: string[]
  registered?: { name: string; value: string; ref: string; marker: string }[]
  registered_absent?: string
  /** Whether THIS caller may write here, with the server's own reason when not.
   *  The write surface renders from this flag and from nothing else. */
  editable: boolean
  editable_reason: string
}

export type SettingsAuditEntry = {
  id: number
  actor: string
  key: string
  user_id?: string
  old: unknown
  new: unknown
  ts: string
  reason?: string
}

export type SettingsHistory = { key: string; entries: SettingsAuditEntry[]; limit: number }

export type SettingsWritten = {
  key: string
  reset: boolean
  value: SettingValue
  for_user?: string
  detail: string
}

/** One stored price row, in the metering package's OWN marshaled shape. The
 *  transport treats it as opaque and so does this client: S10 owns what a row
 *  is, and a second declaration of it would be a second definition of the
 *  platform's money. */
export type StoredPriceRow = {
  id: number
  model: string
  lane: string
  unit_prices: Record<string, number>
  effective_from: string
  verified_on: string
  source: string
  created_by: string
  created_ts: string
  reason?: string
}

export type StoredPricesView = {
  rows: StoredPriceRow[] | null
  version: string
  /** What an EMPTY table means, in the server's own sentence. UNPRICED is the
   *  shipped stance, not a fault, and the honest answer is prose. */
  posture?: string
  deferred: string[]
  editable: boolean
}

export type PriceRowAdded = { row: StoredPriceRow; version: string; detail: string }

/** staleCard returns the FRESH card a 409 `stale_payload` carried with it. The
 *  card comes off the refusal itself, so the answerer's next act is a re-render
 *  of what is actually there — never a guess, never a blind retry (S15.2). */
export function staleCard(err: unknown): ApprovalItem | null {
  if (!(err instanceof ApiError) || err.code !== 'stale_payload') return null
  const body = err.body
  if (!body || typeof body !== 'object') return null
  return (body as { current?: ApprovalItem }).current ?? null
}

export type Deliverable = {
  deliverable_id: string
  owner: string
  task_id: string
  project_id: string
  type: string
  current_revision: number
  state: string
  created_ts: string
  updated_ts: string
}

export type DeliverableList = { deliverables: Deliverable[]; cursor: number; truncated: boolean }

/** One immutable numbered revision (S13.1). The lineage is 1..N and is never
 *  compressed — each entry is a fact, not a diff of the one before it. */
export type Revision = {
  deliverable_id: string
  n: number
  owner: string
  run_id?: string
  pin_kind: string
  content_sha256?: string
  platform_ref: string
  verdict_ref?: number
  created_ts: string
}

/** One content-addressed object pin: the metadata card's fields plus the hash the
 *  bytes are read back by. `type` is the label the MINT recorded, not a wire
 *  header — the bytes route decides from a closed allowlist what may render
 *  inline, and everything else is an attachment. */
export type ObjectRef = { name: string; size: number; sha256: string; type?: string }

export type DeliverableRow = {
  id: string
  owner: string
  task_id: string
  project_id?: string
  subject_ref?: string
  type: string
  current_revision: number
  state: string
  created_ts: string
  updated_ts: string
}

/**
 * One live verb on a deliverable, with the preconditions that decide whether it
 * is open right now — doors-as-data (B6-3 OQ3).
 *
 * The surface renders a CONTROL only where `available`, and renders `reason`
 * verbatim where it is not. It never invents a door and never hides one: a closed
 * door still ships, carrying why, so the state can be explained instead of
 * silently omitted (the D3/D9 honesty rule).
 */
export type Door = {
  verb: string
  method: string
  route: string
  available: boolean
  reason: string
  /** The read a caller makes FIRST to obtain the `payload_hash` its body quotes. */
  pin_from?: string
  /** The answer verb the body carries when this door answers a card. */
  answer?: string
  ask_id?: string
  payload_hash?: string
  /** The S13.9 framing a follow-up spawns under. */
  preset?: string
}

export type DeliverableDetail = {
  deliverable: DeliverableRow
  revisions: Revision[]
  lineage: TaskLineage
  doors: Door[]
  cursor: number
}

/** The server-side pixel-diff aid. Nil at v0 — the seam exists, the surface
 *  renders it when a producer fills it and says nothing when none does. */
export type PixelDiff = { diff_object_sha?: string; changed_ratio?: number }

/**
 * The host-side comparison of two revisions (S13.2), served verbatim.
 *
 * `surface` is the type's DEFINED answer — no type is refused. `label` carries
 * the visible labeling text a fallback or degrade must show, which is the whole
 * 2.1 honesty posture: a fallback never poses as a rich surface. `unified` is the
 * host's own git diff text and nothing else — no HTML, no tokens, no
 * server-rendered highlight, because react-diff-view parses unified diffs and
 * highlights client-side and no server-side highlighter exists (FC-v1 §2).
 */
export type Comparison = {
  deliverable_id: string
  type: string
  /** 0 is the PRE-TASK BASE, not a revision (S13.1). */
  old_n: number
  new_n: number
  surface: string
  fallback?: boolean
  label?: string
  unified?: string
  old_objects?: ObjectRef[]
  new_objects?: ObjectRef[]
  /** The by-hash verdict for the object surfaces. */
  changed?: boolean
  pixel_diff?: PixelDiff
}

/** The five recorded anchor states (S13.3). Consumed as served: an unknown value
 *  renders under its own name rather than being coerced into one of these. */
export const anchorStatuses = ['exact', 'mapped', 'drifted', 'file', 'orphan'] as const

/** The FC-v1 §2 anchor record: `line_text` is the quote selector, (side, line_no)
 *  the position selector. */
export type AnchorRecord = { file_path: string; side: string; line_no: number; line_text: string }

/** Where a comment anchors in ONE revision, recomputed server-side on every
 *  render. A client's positions are never placement authority. */
export type Placement = {
  comment_id: number
  revision_n: number
  status: string
  /** The live position for exact/mapped/drifted; the file for `file` (line_no 0);
   *  zero-valued for `orphan`, whose quote lives on the comment row. */
  anchor?: AnchorRecord
}

/**
 * One review comment — the ONE schema shared by human comments and verification
 * findings (S13.1/S13.3).
 *
 * There is no update verb and no delete verb at any shape, so this surface offers
 * neither: immutability renders as the property it is, and somebody who wants to
 * change what they said adds another comment. `finding_number` is stamped by THE
 * drain at consumption and is 0 while open.
 */
export type Comment = {
  id: number
  owner: string
  deliverable_id: string
  /** The ORIGINAL-revision link: which revision this was said about. */
  revision_n: number
  run_id?: string
  kind: string
  severity: string
  category?: string
  criterion?: string
  body: string
  suggested_change?: string
  /** The original AS-CLAIMED anchor, kept beside the live placement. */
  anchor?: AnchorRecord
  /** A non-positional as-supplied anchor, kept verbatim (findings). */
  origin_anchor?: string
  /** The server-validated status at birth. */
  anchor_status: string
  status: string
  finding_number?: number
  consumed_at?: string
  consumed_by?: string
  created_ts: string
}

export type PlacedComments = {
  deliverable_id: string
  revision_n: number
  comments: Comment[]
  placements: Placement[]
  cursor: number
}

/** The create body. `revision` 0 is the current one; the anchor is CLAIMED and is
 *  validated server-side, so what comes back may not be the position asserted. */
export type CommentCreate = {
  revision: number
  body: string
  severity: string
  anchor?: AnchorRecord
  file_level?: string
  suggested_change?: string
}

/** The S13.6 step-5 signing posture, stated as data and secret-free. Signing is
 *  not a choice on the form: it is structural per user, and the broker holds the
 *  key. */
export type AcceptSigning = {
  structural: boolean
  per_call_flag: boolean
  key_leaves_broker: boolean
  statement: string
}

/** Where each trailer input came from. Every one is a platform fact read off the
 *  minting run — no string a model produced can name a co-author. `absent` states
 *  why the trailers could not be rendered, when they could not. */
export type AcceptProvenance = {
  minting_run_id?: string
  engine?: string
  model?: string
  lane?: string
  vendor_noreply?: string
  absent?: string
}

/**
 * The accept decision data, shown BEFORE the act (S13.6 step 3).
 *
 * The trailers are byte-for-byte what the commit will carry: the mis-attribution
 * lesson is that they are SHOWN, not discovered in the commit afterwards. The card
 * answers for any state — an accepted or non-repo-backed deliverable gets
 * `acceptable:false` with the reason rather than a refusal that hides the facts.
 */
export type AcceptCard = {
  deliverable_id: string
  revision_n: number
  pin_kind: string
  content_pin: string
  project_id: string
  protected_ref?: string
  trailers: string
  provenance: AcceptProvenance
  signing: AcceptSigning
  tier: string
  tier_statement: string
  payload_hash: string
  acceptable: boolean
  reason: string
  route: string
}

/** The collision card, verbatim from internal/accept. A collision is an ANSWER,
 *  never a refusal and never a silent overwrite (S13.6 step 1). */
export type MergeCard = {
  deliverable_id: string
  project_id: string
  onto: string
  candidate: string
  reason: string
  conflicts?: string
  options: string[]
}

/** One collision option with its honest answerability. All three are
 *  `answerable:false` at v0; the third names a LANDED door reaching the same
 *  outcome, which is where the person goes rather than an executor. */
export type MergeCardOption = {
  option: string
  answerable: boolean
  reason: string
  route?: string
  preset?: string
}

export type MergeCardView = { card: MergeCard | null; options: MergeCardOption[]; durability: string }

/**
 * The accept's answer.
 *
 * `applied:false` covers two honest cases and neither is an error: a read-back of
 * an already-accepted deliverable (which is what makes a phone retry safe), and a
 * collision, where nothing was pushed and the merge card says so.
 */
export type AcceptOutcome = {
  deliverable_id: string
  applied: boolean
  state: string
  revision_n: number
  commit?: string
  effect_id?: string
  /** Deliverables this accept moved to superseded, and the project's active runs
   *  the S02.8 sibling-accept freshness trigger sent for re-validation. */
  superseded: string[]
  routed_runs: string[]
  merge_card?: MergeCardView
  detail: string
}

/** One probed listening port — the multi-port picker's data, populated when a dev
 *  server exposes more than one. */
export type PreviewPort = { number: number; label?: string }

/**
 * One preview instance (S13.8).
 *
 * `state` is a DEFINED answer for every type: `live` is backed, and
 * `self-preview`, `requires-container-tier`, `no-preview`, `unavailable` and
 * `at-capacity` are states carrying their reason. None of them is an error, and
 * none of them gets an iframe — a broken frame is the failure S13.8 names.
 */
export type PreviewSession = {
  id: string
  deliverable: string
  revision: number
  role: string
  lane: string
  state: string
  reason?: string
  /** The tailnet front-chain subdomain when routed, the loopback backend in dev. */
  url?: string
  backend_addr?: string
  pool_port?: number
  route_id?: string
  routed: boolean
  ports?: PreviewPort[]
  runner?: { tool: string; argv?: string[]; source: string }
  user: string
  host_injections?: { mechanism: string; key: string; value: string }[]
  created_ts: string
}

/** The lean per-side data the dual-iframe shell consumes. */
export type SessionView = {
  role: string
  deliverable: string
  revision: number
  url: string
  state: string
  reason?: string
}

/** How navigation mirrors across the two frames. `path` = mirror the URL
 *  path+query across both sides; `enabled` is false when there is no before. */
export type SyncSemantics = { mode: string; enabled: boolean }

/** The before-vs-after contract. `before` is absent when the deliverable has no
 *  accepted revision: revision 1 has no before, and inventing one would be a
 *  preview fake of a story the S13.1 diff already tells truthfully. */
export type PreviewComparison = {
  deliverable: string
  before?: SessionView
  after: SessionView
  single_instance: boolean
  sync: SyncSemantics
}

export type PreviewSessions = { sessions: PreviewSession[]; cursor: number }
export type PreviewStopped = { session: string; stopped: boolean; detail: string }

/**
 * staleAcceptCard returns the FRESH AcceptCard a 409 `stale_payload` carried.
 *
 * `staleCard` above cannot be reused: the accept verb's 409 carries an
 * ACCEPT CARD in `current`, not an ApprovalItem, and reading one as the other
 * would silently produce a card with no pin. Same refusal shape, two different
 * families — so two readers, each typed to what its verb actually sends.
 */
/**
 * objectHref is the URL of one pinned object's BYTES.
 *
 * It is a URL rather than a fetch because the two consumers are an `<img src>` and
 * a download `<a href>` — the browser reads these, not this client. The path is
 * composed from the deliverable id and the sha the SERVED ObjectRef carries, so
 * nothing here names an object the platform did not pin; the server resolves the
 * sha against that deliverable's own revisions and 404s otherwise, which is what
 * keeps the route from being a hash oracle.
 */
export function objectHref(deliverable: string, sha: string): string {
  return `/api/deliverables/${encodeURIComponent(deliverable)}/objects/${encodeURIComponent(sha)}`
}

export function staleAcceptCard(err: unknown): AcceptCard | null {
  if (!(err instanceof ApiError) || err.code !== 'stale_payload') return null
  const body = err.body
  if (!body || typeof body !== 'object') return null
  return (body as { current?: AcceptCard }).current ?? null
}

/** The board drag's outcome. `applied:false` is the honest stale-board answer,
 *  not an error: the work moved on between the render and the drag. */
export type PriorityHint = {
  task_id: string
  rank: number
  runs: string[]
  applied: boolean
  detail: string
}

// ── the S15.7 assistant family (B6-7) ─────────────────────────────────────

/** One conversation's registry row. An EMPTY `title` is the honest untitled
 *  state, never a placeholder; `title_source` says who or what named it
 *  (`mechanical` = a truncation of the person's own first message, `duty` = the
 *  local `utility` seat, `human` = a rename, which the duty never overwrites). */
export type ChatSession = {
  session_id: string
  owner: string
  title: string
  title_source: string
  created_ts: string
  updated_ts: string
}

/** One immutable transcript entry. There is no edit verb and no message delete:
 *  a record of speech that can be rewritten is not a record. */
export type ChatMessage = {
  message_id: string
  session_id: string
  owner: string
  role: string
  body: string
  turn_id: string
  created_ts: string
}

/**
 * One turn's lifecycle record.
 *
 * `outcome` is served VERBATIM as the store recorded it, discriminated by
 * `outcome_kind`: an `Answer` for the four query verbs, a born task for the S06
 * handoff, and the platform's own `{error, detail}` envelope for a turn whose
 * act failed. Nothing on the way to the screen re-derives a confidence or
 * restyles a lower-confidence flag (G3 D3.5).
 */
export type ChatTurn = {
  turn_id: string
  session_id: string
  owner: string
  /** The verb the person's own gesture chose: ask | view | query | open_sql | task. */
  kind: string
  /** running | settled | abandoned. */
  state: string
  outcome_kind?: string
  outcome?: unknown
  /** The exchange-manifest ids attributed to this turn (the OQ7 window diff plus
   *  the late-arrival origin-ref union). Honestly sparse at v0. */
  produced?: string[]
  started_ts: string
  settled_ts?: string
}

/** The born task the S06 handoff returns, with its OPEN intake card — which is
 *  what the feed renders in place (OQ8(i)). The shape is the pipeline's own
 *  `taskView`, tied to it by a Go test rather than imagined here. */
export type ChatBornTask = {
  task_id: string
  title: string
  kanban_status: string
  owner: string
  phase: string
  tier: string
  family?: string
  clearance?: number
  open_ask_id?: string
  open_card?: ChatIntakeCard
  runs?: { run_id: string; role: string; state: string; has_receipt: boolean }[]
}

/** The intake card as the pipeline issued it. Questions carry their own option
 *  values, and an answer quotes those values back — the vocabulary is the
 *  card's, never the surface's. */
export type ChatIntakeCard = {
  kind: string
  task_id?: string
  run_id?: string
  version?: number
  issued_ts?: string
  clearance?: number
  tier?: string
  questions?: { id: string; text: string; options?: { label: string; value: string }[]; weight?: number }[]
}

/**
 * One session with everything the surface renders without a second round trip.
 *
 * `running` is the in-flight turn as SERVER state, which is what makes leaving
 * the view — or reloading the page entirely — unable to lose it (R14).
 * `title_seat_wired` lets a mechanically-named session say WHY rather than
 * leaving a person to wonder.
 */
export type ChatSessionView = {
  session: ChatSession
  messages: ChatMessage[]
  turns: ChatTurn[]
  running?: ChatTurn
  title_seat_wired: boolean
}

/** One exchange object's manifest row. The manifest is the record; the
 *  filesystem is only where the bytes are. */
export type ChatFile = {
  file_id: string
  owner: string
  name: string
  size_bytes: number
  sha256: string
  origin_session?: string
  origin_turn?: string
  uploaded_ts: string
}

/** The turn submit body: the client's routing decision plus that verb's
 *  arguments (OQ3(a) — the client routes, the server acts). */
export type ChatTurnRequest = {
  kind: string
  text: string
  view?: string
  query?: string
  slots?: Record<string, string>
  title?: string
  /** Exchange-manifest ids handed to a born task, resolved against the CALLER's
   *  own folder server-side. */
  inputs?: string[]
}

export type ChatTurnResult = { turn: ChatTurn; message?: ChatMessage; session: ChatSession }
/** `applied:false` is the already-resolved state, not a refusal: the turn had
 *  already ended when the stop arrived (S15.2 retry-safety). */
export type ChatTurnStopped = { turn: ChatTurn; applied: boolean }
export type ChatUploaded = { file: ChatFile; applied: boolean }
export type ChatFileDeleted = { file: ChatFile; applied: boolean }
export type ChatSessionDeleted = { session_id: string; messages: number; turns: number; applied: boolean }

// ── the S15.10 workforce map (B6-8 part B) ────────────────────────────────
//
// ONE read serves the whole map, and every absence on it arrives with a reason:
// a worker with no active version, a version nobody approved, a version nothing
// was ever routed to. At v0 the roster is EMPTY on a fresh host
// (compose-when-earned, S08.6) and most versions carry no outcomes at all —
// that empty IS the truthful answer, so the shapes below make it renderable
// rather than guessable.

/** The S08.9 multi-stage chain: the dialect, the ONE named service, and the
 *  ordered steps with their explicit approval nodes marked. Step args are not
 *  served — the map renders how stages connect, not the values between them. */
export type WorkflowStep = {
  id: string
  verb: string
  approval: boolean
  /** This step's `$from` edges: argument name → the reference it reads
   *  (`payload.day`, `steps.fetch.summary`). A reference is a definition-time
   *  connection, which is what makes the chain a chain; a LITERAL arg is a value
   *  and is deliberately not served. Absent when the step reads neither. */
  needs?: Record<string, string>
}
export type WorkerWorkflow = { dialect: string; service: string; steps: WorkflowStep[] }

/** What the definition FILE requests (S08.1). What was granted is the version's
 *  own `granted` block — the S08.2 split, kept as two fields on the wire. */
export type RequestedEquipment = {
  tools?: string[]
  skills?: string[]
  knowledge?: string[]
  connectors?: string[]
}

export type WorkerDefinition = {
  description?: string
  selectors: { family?: string; task_classes?: string[]; triggers?: string[] }
  profile: { duty?: string; effort_floor?: string; model_pin?: string; model_pin_reason?: string }
  requested_equipment: RequestedEquipment
  eval: { golden_set_ref?: string; planted_defect_ref?: string }
  persona?: string[]
  workflow?: WorkerWorkflow
  workflow_absent?: string
}

/** The worker_guardrails row — the S08.2 enforcement state, exclusively. */
export type WorkerGrants = {
  granted_tools: string[]
  gated_tools?: string[]
  permission_mode?: string
  confinement_class: string
  egress: string
  egress_hosts?: string[]
  /** Nil when no ceiling was declared. `worker_guardrails` stores 0 for "none
   *  requested" and approval grants only what was requested, so a 0 here would
   *  be a ceiling nobody set — the same fabricated figure the meters surface
   *  refuses to print (S10.1). */
  budget_usd: number | null
  budget_steps: number | null
  gate_policy?: string
  first_n_remaining: number
  schedule_attachable: boolean
  updated_ts: string
}

export type WorkerValidation = {
  model?: string
  engine_pin: string
  green: boolean
  approver?: string
  approved_ts?: string
  created_ts: string
}

/** A dated revalidation stamp (S14.8 ¶3). A RED stamp is on the record too, and
 *  the version stays flagged — which is why `green` is rendered either way. */
export type WorkerRevalidation = {
  trigger_kind: string
  trigger_subject?: string
  suites: string
  baseline: string
  green: boolean
  actor: string
  stamped_ts: string
}

export type WorkerVerdict = { round: number; verdict: string; revision?: number; domain?: string; ts: string }

/** One run routed to a version: the routing decision as recorded, that run's
 *  verdicts, and its meter reading. `tokens`/`api_equiv_cost_usd` are READ from
 *  the metering seam and are null when there is no reading — never a zero. */
export type RoutedRun = {
  run_id: string
  owner: string
  cause: string
  model?: string
  lane?: string
  effort?: string
  plain_reason?: string
  degraded?: boolean
  generalist?: boolean
  routed_ts: string
  verdicts: WorkerVerdict[]
  verdicts_absent?: string
  verdicts_truncated?: boolean
  tokens: number | null
  api_equiv_cost_usd: number | null
  unpriced?: boolean
  meter_absent?: string
}

/** The S08.4 version→outcome join. There is deliberately no total: a cross-run
 *  figure would be arithmetic nobody performed. `truncated` reports the
 *  per-version bound cutting the list, so a reading of the newest runs is never
 *  read as the whole history. */
export type WorkerOutcomes = {
  runs: RoutedRun[]
  truncated: boolean
  /** How many runs this reading found routed to the version, BEFORE the
   *  per-version bound — so it stays true when the list is cut. A count of rows
   *  is a reading; money is never summed. */
  runs_routed: number
  /** How many recorded rounds carried each verdict, across those runs. */
  verdict_tally: { verdict: string; rounds: number }[]
  absent?: string
}

export type WorkerVersion = {
  version_id: string
  version: number
  supersedes?: string
  active: boolean
  file_path: string
  file_sha256: string
  file_commit?: string
  author_kind: string
  composer?: string
  playbook_version?: string
  evidence_ref?: string
  origin: string
  origin_ref?: string
  created_by: string
  created_ts: string
  approved_by?: string
  approved_ts?: string
  graduated_ts?: string
  requested_grants: {
    tools?: string[]
    class: string
    egress: string
    egress_hosts?: string[]
    gated_tools?: string[]
    permission_mode?: string
    budget_usd?: number
    budget_steps?: number
    schedule_attachable?: boolean
  }
  granted: WorkerGrants | null
  granted_absent?: string
  validation: WorkerValidation | null
  validation_absent?: string
  revalidation: WorkerRevalidation | null
  revalidation_absent?: string
  outcomes: WorkerOutcomes
}

/** The S08.7 policy, served from the registry's own read: what this worker may
 *  deliver without a person looking, and why not. Degraded mode is structural,
 *  not advisory, so its consequence arrives as data.
 *
 *  It is always present. There is no "unread" state to render because the server
 *  fails the whole read rather than serving one — the zero policy would say
 *  `requires_review: false`, and a reader must never be told a worker needs no
 *  human review by an accident of decoding. */
export type DeliveryPolicy = { deliverable: boolean; requires_review: boolean; reasons?: string[] }

export type WorkerRow = {
  template_id: string
  name: string
  owner: string
  scope: string
  kind: string
  status: string
  active_version: string
  created_ts: string
  updated_ts: string
  /** `maturity` is the RAW stored value, so one this build has not heard of
   *  still reaches the screen rather than vanishing (§42 forward tolerance). */
  domain: { name: string; maturity: string; rubric_ref?: string }
  delivery: DeliveryPolicy
  definition: WorkerDefinition | null
  definition_absent?: string
  versions: WorkerVersion[]
  versions_truncated: boolean
}

/** `roster_scope` and `outcome_scope` are SERVED sentences, not derived: a
 *  member's answer and the operator's are different readings of the registry and
 *  neither may pose as the other. */
export type Workforce = {
  workers: WorkerRow[]
  roster_scope: string
  outcome_scope: string
  truncated: boolean
  outcomes_truncated: boolean
  cursor: number
}

/**
 * apiPath reports whether a SERVED route is a machine-surface path on this origin.
 *
 * It resolves the string the way the browser will before deciding, because that is
 * the only way the answer matches what the request would actually do: a prefix test
 * accepts `/api/../evil`, which resolves off the machine surface entirely. A route
 * naming another origin is refused for the same reason.
 *
 * AND IT RESOLVES THE DECODED FORM, because the browser is not the only resolver.
 * The `URL` constructor leaves `%2f` alone, so `/api/..%2fevil` resolved to a path
 * still under `/api/` while the SERVER — Go's mux, which unescapes and then cleans —
 * routes it to `/evil` (drain r2 R7). The claim this check makes is about the
 * RESULT, so it decodes once (once, because the server decodes once: `%252f` stays
 * a literal `%2f` there too) and re-resolves before deciding. A malformed escape is
 * refused rather than guessed at.
 */
export function apiPath(route: string): boolean {
  if (!route.startsWith('/')) return false
  const resolved = new URL(route, 'https://sinet.invalid')
  if (resolved.origin !== 'https://sinet.invalid') return false
  let decoded: string
  try {
    decoded = decodeURIComponent(resolved.pathname)
  } catch {
    return false
  }
  const settled = new URL(decoded, 'https://sinet.invalid')
  return settled.origin === 'https://sinet.invalid' && settled.pathname.startsWith('/api/')
}

/** query builds a search string, dropping unset filters rather than sending
 *  empty ones — an empty `?person=` is not the same request as no `?person=`. */
function query(params: Record<string, string | number | undefined>): string {
  const out = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === '') continue
    out.set(k, String(v))
  }
  const qs = out.toString()
  return qs ? `?${qs}` : ''
}

export type ListFilters = {
  person?: string
  status?: string
  project?: string
  task?: string
  since?: string
  until?: string
  limit?: number
}

export const api = {
  health: () => request<Health>('/api/health'),
  session: () => request<Session>('/api/auth/session'),
  users: () => request<{ users: User[] }>('/api/auth/users'),
  /** An empty pin is the S01.9 layer-2 grant auto-login attempt. */
  login: (user_id: string, pin: string) => post<{ user_id: string; expires: string }>('/api/auth/login', { user_id, pin }),
  logout: () => post<void>('/api/auth/logout', {}),
  /**
   * The bootstrap window: while `users` is empty one anonymous create is
   * allowed and MUST be an operator (D10 needs a holder from the first user).
   *
   * The answer is 201 with the new id and NO session cookie — creating an
   * account is not signing in, and the caller has to say so.
   */
  createFirstOperator: (user_id: string, display_name: string, pin: string) =>
    post<{ user_id: string }>('/api/auth/users', { user_id, display_name, role: 'operator', pin }),

  tasks: (f: ListFilters = {}) => request<TaskList>(`/api/tasks${query(f)}`),
  runs: (f: ListFilters = {}) => request<RunList>(`/api/runs${query(f)}`),
  meters: (f: { person?: string; lane?: string; limit?: number } = {}) => request<Meters>(`/api/meters${query(f)}`),

  historyViews: () => request<HistoryRegistry>('/api/events/views'),
  historyCatalog: () => request<HistoryRegistry>('/api/events/catalog'),
  historyView: (view: string) => request<Answer>(`/api/events/views/${encodeURIComponent(view)}`),
  /** Layer-1 slots ride the `slot_` prefix so a slot can never collide with a
   *  transport parameter — the server's own namespacing, mirrored. */
  historyQuery: (name: string, slots: Record<string, string> = {}) => {
    const params: Record<string, string> = {}
    for (const [k, v] of Object.entries(slots)) if (v !== '') params[`slot_${k}`] = v
    return request<Answer>(`/api/events/query/${encodeURIComponent(name)}${query(params)}`)
  },

  /**
   * The ONE mutation the oversight views call. Rank is an ordering position
   * within ±1000 where 0 means "no hint" — so a re-rank strategy never assigns
   * 0 to mean "first" — and it lands only on the caller's OWN queued runs.
   */
  task: (id: string) => request<TaskDetail>(`/api/tasks/${encodeURIComponent(id)}`),
  run: (id: string) => request<RunDetail>(`/api/runs/${encodeURIComponent(id)}`),
  deliverable: (id: string) => request<DeliverableDetail>(`/api/deliverables/${encodeURIComponent(id)}`),
  approvals: () => request<ApprovalList>('/api/approvals'),
  deliverables: (f: { state?: string; project?: string; type?: string; task?: string } = {}) =>
    request<DeliverableList>(`/api/deliverables${query(f)}`),

  priorityHint: (task: string, rank: number, reason?: string) =>
    post<PriorityHint>(`/api/tasks/${encodeURIComponent(task)}/priority-hint`, reason ? { rank, reason } : { rank }),

  // ── the S15.6 decision verbs (B6-6) ─────────────────────────────────────
  //
  // Every one of them is the LANDED verb of the card that names it in its own
  // `actions`. Nothing here invents a door: the inbox renders a control only
  // where a card served the action, and the server is the authority on whether
  // the act is allowed.

  /**
   * The one answer verb. `pin` rides the SAME request as the answer (S01.9
   * verify-at-act): there is no stored elevation, so a High-tier card's PIN is
   * collected, sent, and gone. `payload_hash` is the pin the card was shown
   * for — an answer quoting a stale one fires nothing and comes back with the
   * fresh card (S15.2).
   */
  answerApproval: (id: string, body: { payload_hash: string; answer: unknown; pin?: string }) =>
    post<ApprovalAnswerResult>(`/api/approvals/${encodeURIComponent(id)}/answer`, body),
  /** The Low-tier batch. Transport convenience only: each item is validated,
   *  applied and logged individually, and one refusal leaves the rest alone. */
  answerApprovalBatch: (items: ApprovalBatchItem[]) => post<ApprovalBatchResult>('/api/approvals/answer-batch', { items }),
  /**
   * The same answer verb, reached at the route a DOOR named.
   *
   * A door serves the whole path, card id included — and the card id is a
   * `<kind>:<native-id>` composite whose halves the door serves SEPARATELY
   * (`ask_id` is the native half alone). Assembling the composite here would be
   * this client re-deriving a transport identity it was already handed, which is
   * the mistake §43 recorded on the other decision family. So the door's route is
   * used as given.
   *
   * The one check is a boundary check, because a served string is becoming a
   * request target: the route must be an `/api/` path on this origin. A door that
   * named anything else would be a bug in the platform, and following it would be
   * this client's. `startsWith('/api/')` alone was weaker than that sentence —
   * `/api/../evil` passes it and resolves elsewhere — so the check is the one the
   * comment claims: resolve the route and require the RESULT to stay under `/api/`
   * on this origin.
   */
  answerAtDoor: (route: string, body: { payload_hash: string; answer: unknown; pin?: string }) => {
    if (!apiPath(route)) {
      return Promise.reject(new ApiError(0, `a door named a route this client will not follow: ${route}`, 'bad_door'))
    }
    return post<ApprovalAnswerResult>(route, body)
  },

  suppressFlag: (body: { run_id?: string; anomaly_class: string; reason?: string }) =>
    post<FlagSuppressed>('/api/watchdog/flags/suppress', body),
  /** "Resume — I was wrong" (S14.4). Path-only and owner-scoped; a run parked on
   *  an OPEN ask refuses with a pointer, which renders verbatim. */
  resumeRun: (run: string) => post<RunResumed>(`/api/runs/${encodeURIComponent(run)}/resume`, {}),
  dismissDrift: (id: string, reason?: string) =>
    post<DriftDismissed>(`/api/approvals/${encodeURIComponent(id)}/dismiss`, reason ? { reason } : {}),
  acknowledgeConformance: (id: string, reason?: string) =>
    post<ConformanceAcknowledged>(`/api/approvals/${encodeURIComponent(id)}/acknowledge`, reason ? { reason } : {}),

  benchmarkVerdicts: () => request<BenchmarkVerdictForms>('/api/benchmark/verdicts'),
  /** The §3.3 ONE act: the blind pick and the arm-guess together. The backend's
   *  constructor makes a guess-less verdict inexpressible regardless. */
  recordVerdict: (id: string, verdict: string, guess: string) =>
    post<VerdictRecorded>(`/api/approvals/${encodeURIComponent(id)}/verdict`, { verdict, guess }),
  declineVerdict: (id: string) => post<VerdictDeclined>(`/api/approvals/${encodeURIComponent(id)}/decline`, {}),
  disposeAlarm: (id: string, disposition: string, reason?: string) =>
    post<AlarmDispositioned>(`/api/approvals/${encodeURIComponent(id)}/dispose`,
      reason ? { disposition, reason } : { disposition }),

  /** The ninth kind's verb. The path id is the conflict ROW number, which the
   *  card carries; a repeat answers 200 with the already-closed detail. */
  resolveMemoryConflict: (conflict: number) =>
    post<MemoryConflictResolved>(`/api/memory/conflicts/${encodeURIComponent(String(conflict))}/resolve`, {}),

  // ── the S15.9 settings family (B6-6 part B) ──────────────────────────────

  settings: () => request<SettingsView>('/api/settings'),
  settingsHistory: (key: string) => request<SettingsHistory>(`/api/settings/${encodeURIComponent(key)}/history`),
  /**
   * The per-key write. A null (or absent) `value` is RESET-TO-DEFAULT — it
   * deletes the override row — so reset needs no verb of its own and cannot
   * drift from what the registry means by it. `for_user` targets one person's
   * override on a per-user key; the registry admits no member actor, so this is
   * how a member's own override comes to exist.
   */
  setSetting: (key: string, body: { value: unknown; for_user?: string; reason?: string }) =>
    post<SettingsWritten>(`/api/settings/${encodeURIComponent(key)}`, body),
  /** Bounds are a SEPARATE act from a value, and operator-only: automation
   *  moves a value inside its bounds, only the operator moves the bounds
   *  themselves (G1 rider 1). Both null resets to the ratified clamp. */
  setSettingBounds: (key: string, body: { floor: number | null; ceiling: number | null; reason?: string }) =>
    post<SettingsWritten>(`/api/settings/${encodeURIComponent(key)}/bounds`, body),

  prices: () => request<StoredPricesView>('/api/settings/prices'),
  /** The ONLY price mutation there is: an effective-dated APPEND. Past rows are
   *  immutable by trigger, and no edit or delete verb exists to offer. */
  addPriceRow: (row: unknown, reason?: string) =>
    post<PriceRowAdded>('/api/settings/prices', reason ? { row, reason } : { row }),

  // ── the S15.7 assistant family (B6-7) ────────────────────────────────────

  chatSessions: () => request<{ sessions: ChatSession[] }>('/api/chat/sessions'),
  chatSession: (id: string) => request<ChatSessionView>(`/api/chat/sessions/${encodeURIComponent(id)}`),
  createChatSession: () => post<{ session: ChatSession }>('/api/chat/sessions', {}),
  /** The human override of the titling duty. The duty's own write requires the
   *  source to be unset, so a rename can never be overwritten by it. */
  renameChatSession: (id: string, title: string) =>
    post<{ session: ChatSession }>(`/api/chat/sessions/${encodeURIComponent(id)}/rename`, { title }),
  deleteChatSession: (id: string) =>
    post<ChatSessionDeleted>(`/api/chat/sessions/${encodeURIComponent(id)}/delete`, {}),

  /**
   * The ONE turn verb, and it is SYNCHRONOUS: the answer arrives whole, because
   * nothing in the platform streams a chat turn (no engine session runs for one).
   * "In-flight" is this POST being outstanding over a server-side `running` row,
   * which is what a re-entered view re-attaches to rather than re-asking.
   */
  chatTurn: (session: string, body: ChatTurnRequest) =>
    post<ChatTurnResult>(`/api/chat/sessions/${encodeURIComponent(session)}/turns`, body),
  /** The S15.7 hard-stop: an EXPLICIT act with a record, never a navigation side
   *  effect. It abandons the turn; it does not cancel a task the turn handed off. */
  stopChatTurn: (turn: string) => post<ChatTurnStopped>(`/api/chat/turns/${encodeURIComponent(turn)}/stop`, {}),

  // ── the S15.8 review family (B6-8) ───────────────────────────────────────
  //
  // Every read below is owner-scoped server-side and every verb is one the
  // deliverable's own DOORS name. The surface renders a control where a door is
  // available and the door's reason where it is not; nothing here reaches an act
  // the served detail did not offer.

  /**
   * The comparison of two revisions. Sending NO bounds is the point: the server's
   * own default is round-over-round (new = current, old = new−1), so the default
   * view asks for exactly that rather than composing a pair the client inferred.
   * `old: 0` is the pre-task base — a real navigation target that is not a revision.
   */
  compare: (id: string, bounds: { old?: number; new?: number } = {}) =>
    request<Comparison>(`/api/deliverables/${encodeURIComponent(id)}/compare${query(bounds)}`),

  /** Every comment of the deliverable, with where each one anchors in `revision`
   *  (absent = the current one). Placements come from the server on every render;
   *  this client never computes one. */
  comments: (id: string, revision?: number) =>
    request<PlacedComments>(`/api/deliverables/${encodeURIComponent(id)}/comments${query({ revision })}`),

  /**
   * Create ONE comment. There is deliberately no update and no delete verb here,
   * because none exists at any shape: a comment is immutable and is consumed only
   * by THE drain (S13.3/S13.4). The anchor sent is CLAIMED — the response carries
   * the status the server's own ladder gave it, which is what the surface renders.
   */
  addComment: (id: string, body: CommentCreate) =>
    post<Comment>(`/api/deliverables/${encodeURIComponent(id)}/comments`, body),

  /** The pin-bearing read the accept door names in `pin_from`. */
  acceptCard: (id: string) => request<AcceptCard>(`/api/deliverables/${encodeURIComponent(id)}/accept-card`),

  /**
   * The ONE outward act on this API, as one request: the pin the card was shown
   * with, and the PIN verified at the act (S01.9 verify-at-act — no elevation is
   * inherited, so the PIN is collected, sent, and gone). A stale `payload_hash`
   * fires nothing and comes back 409 with the FRESH card in `current`.
   */
  accept: (id: string, body: { payload_hash: string; pin: string; subject?: string; provenance?: string }) =>
    post<AcceptOutcome>(`/api/deliverables/${encodeURIComponent(id)}/accept`, body),

  /** Try it out. Low tier throughout (S13.8): no journal, no proposal, no PIN on
   *  any preview route — launching one is reversible and touches nothing outside
   *  the host. `revision` 0 is the current one. */
  launchPreview: (id: string, revision = 0) =>
    post<PreviewSession>(`/api/deliverables/${encodeURIComponent(id)}/preview`, { revision }),
  previewComparison: (id: string, revision = 0) =>
    post<PreviewComparison>(`/api/deliverables/${encodeURIComponent(id)}/preview/compare`, { revision }),
  previews: () => request<PreviewSessions>('/api/previews'),
  /** A partial teardown keeps the session, so the stop stays retryable and the
   *  caller is told it did not complete rather than told it did. */
  stopPreview: (session: string) =>
    post<PreviewStopped>(`/api/previews/${encodeURIComponent(session)}/stop`, {}),

  // ── the S15.10 workforce map (B6-8 part B) ───────────────────────────────
  //
  // ONE read, and no verb: the v0 map has no mutation affordance at all
  // (S15.10 parks editing to 15.5), so there is nothing else here to call.
  workforce: () => request<Workforce>('/api/workforce'),

  chatFiles: () => request<{ files: ChatFile[] }>('/api/chat/files'),
  /**
   * The ONE upload verb, driven by both gestures (drag-drop and click-browse) so
   * there is no second write path to keep in step. The body is the raw object and
   * the name rides the query string: the bytes are never decoded, re-encoded or
   * wrapped by this client, and the server's own bound refuses an oversized one
   * with its own reason.
   */
  uploadChatFile: (name: string, body: Blob) =>
    request<ChatUploaded>(`/api/chat/files?name=${encodeURIComponent(name)}`, { method: 'POST', body }),
  deleteChatFile: (id: string) =>
    post<ChatFileDeleted>(`/api/chat/files/${encodeURIComponent(id)}/delete`, {}),
}
