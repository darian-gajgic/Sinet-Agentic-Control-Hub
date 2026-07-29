/**
 * The typed client for the S15.2 read surface: readiness, the S01.9 session
 * stack, and the oversight families the B6-5 views render. The inbox, settings,
 * assistant and review families belong to the packets that build those views.
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
  counters: { tokens: number; api_equiv_cost_usd: number; elapsed_s: number; steps: number }
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

export type DeliverableDetail = {
  deliverable: {
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
  revisions: Revision[]
  cursor: number
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
}
