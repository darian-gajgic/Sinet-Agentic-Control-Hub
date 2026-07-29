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

/** ApiError carries the HTTP status so callers can branch on 401 alone. */
export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
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
  if (!res.ok) {
    throw new ApiError(res.status, await errorMessage(res))
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

/** errorMessage prefers the machine surface's own {error, detail} shape and
 * degrades to the status text — it never invents a reason. */
async function errorMessage(res: Response): Promise<string> {
  try {
    const body: unknown = await res.json()
    if (body && typeof body === 'object') {
      const { error, detail } = body as { error?: string; detail?: string }
      if (detail) return detail
      if (error) return error
    }
  } catch {
    // Not JSON. The status is what we have, and saying so is honest.
  }
  return `${res.status} ${res.statusText}`.trim()
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

// ── the what-needs-me feeds (S1.4) ────────────────────────────────────────

export type ApprovalItem = {
  id: string
  kind: string
  owner: string
  run_id: string
  tier: string
  answerable: boolean
  not_answerable_reason?: string
  batchable: boolean
  step_up_required: boolean
  observed_ts: string
  expiry_at?: string
  stale?: boolean
  stale_reasons?: string[]
}

export type ApprovalList = { items: ApprovalItem[]; cursor: number; truncated?: boolean }

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
  approvals: () => request<ApprovalList>('/api/approvals'),
  deliverables: (f: { state?: string; project?: string; type?: string } = {}) =>
    request<DeliverableList>(`/api/deliverables${query(f)}`),

  priorityHint: (task: string, rank: number, reason?: string) =>
    post<PriorityHint>(`/api/tasks/${encodeURIComponent(task)}/priority-hint`, reason ? { rank, reason } : { rank }),
}
