package history

import "sort"

// catalog.go — S14.10 ¶2, Layer 1: "~30–50 named queries (status, failures,
// verdicts, routing quality, drift history) with typed slots … This is the
// reliability floor."
//
// The catalog is DATA (OQ3, ratified): a Go table of {name, description,
// category, SQL, typed slot descriptors}. Data rather than code for the same
// reason the conformance registry and the benchmark's registered numbers are
// data — the set is the thing being reviewed, and a reviewer must be able to
// read it as a list.
//
// EVERY entry obeys four structural rules, each asserted over the whole table
// by catalog_test.go rather than trusted per row:
//
//  1. `?` PLACEHOLDERS ONLY. No entry interpolates anything. A slot value is
//     bound, never formatted, so a slot cannot carry SQL.
//  2. The OWNER-SCOPE PREDICATE `(? = '' OR <col> = ?)` is present in every
//     entry (S01.9, CONVENTIONS §30 OQ1). It is parameterized rather than two
//     query variants precisely so there is no unscoped form of any query to
//     accidentally reach for: the operator binds '' and a member binds their
//     identity, and user_id is CHECK'd non-empty everywhere so '' can only
//     ever mean "operator".
//  3. Every entry ends `LIMIT ?`, so no answer is unbounded.
//  4. No entry contains a statement separator. One query, one statement.
//
// Argument order is fixed by the shape: the declared slots in order, then the
// scope value twice, then the limit. Executor and rules live in layer1.go.

// Category is one of the S14.10 ¶2 named categories, plus cost (which selects
// Layer-0 views — the money question is answered by a view, never recomputed
// here, S14.10 ¶1).
type Category string

const (
	CatStatus         Category = "status"
	CatFailures       Category = "failures"
	CatVerdicts       Category = "verdicts"
	CatRoutingQuality Category = "routing quality"
	CatDriftHistory   Category = "drift history"
	CatCost           Category = "cost"
)

// Categories is the closed set, in the order S14.10 ¶2 names them.
var Categories = []Category{CatStatus, CatFailures, CatVerdicts, CatRoutingQuality, CatDriftHistory, CatCost}

// SlotType is a slot's TYPE (S14.10 ¶2 "typed slots"). The first eight are the
// S14.10 preamble's filter/search axes verbatim — "project, person, status,
// date range, worker, domain, lane, run/task id" — and a test asserts each of
// them is reachable through at least one catalog query. The rest are the
// vocabulary the named categories need beyond the axes.
type SlotType string

const (
	// The eight spec axes.
	SlotProject  SlotType = "project"
	SlotPerson   SlotType = "person"
	SlotStatus   SlotType = "status"
	SlotDateFrom SlotType = "date_from"
	SlotDateTo   SlotType = "date_to"
	SlotWorker   SlotType = "worker"
	SlotDomain   SlotType = "domain"
	SlotLane     SlotType = "lane"
	SlotRunID    SlotType = "run_id"
	SlotTaskID   SlotType = "task_id"

	// Category vocabulary beyond the axes.
	SlotCause       SlotType = "routing_cause"
	SlotChangeClass SlotType = "change_class"
	SlotSource      SlotType = "watch_source"
	SlotSeverity    SlotType = "severity"
	SlotRubric      SlotType = "rubric_id"
	SlotRule        SlotType = "watchdog_rule"
)

// Axes are the eight S14.10-preamble filter/search axes. Date range is one axis
// expressed as two slots, and run/task id is one axis expressed as two ids;
// both halves must be reachable, so both are listed.
var Axes = []SlotType{
	SlotProject, SlotPerson, SlotStatus,
	SlotDateFrom, SlotDateTo,
	SlotWorker, SlotDomain, SlotLane,
	SlotRunID, SlotTaskID,
}

// Slot is one typed parameter of a catalog query.
type Slot struct {
	Name string   `json:"name"`
	Type SlotType `json:"type"`
	Desc string   `json:"description"`
}

// Query is one catalog entry.
type Query struct {
	Name        string   `json:"name"`
	Category    Category `json:"category"`
	Description string   `json:"description"`
	// Slots are the typed parameters, in BIND ORDER.
	Slots []Slot `json:"slots"`
	// SQL is the statement. Slots bind first, then the scope value twice, then
	// the limit.
	SQL string `json:"-"`
}

// OwnerScopeOpen / OwnerScopeClose bracket the parameterized S01.9 predicate
// every catalog entry carries. They are exported constants so the test that
// asserts the predicate's presence checks the SAME text production builds with,
// rather than a paraphrase of it (the §36 D7 lesson).
const (
	OwnerScopeOpen  = " AND (? = '' OR "
	OwnerScopeClose = " = ?)"
)

// scoped renders the owner-scope predicate for a column. It is called at
// package init over constant column names only — never over user input.
func scoped(col string) string { return OwnerScopeOpen + col + OwnerScopeClose }

// catalog is the Layer-1 reliability floor. Sized into the 30–50 band across
// the six categories (OQ3).
var catalog = []Query{
	// ── status ────────────────────────────────────────────────────────────────
	{
		Name: "status.runs_by_state", Category: CatStatus,
		Description: "runs currently in a given lifecycle state",
		Slots:       []Slot{{"state", SlotStatus, "a run lifecycle state (running, parked, completed, crashed, …)"}},
		SQL: `SELECT run_id, user_id, task_id, state, lane, created_ts, updated_ts FROM runs WHERE state = ?` +
			scoped("user_id") + ` ORDER BY updated_ts DESC LIMIT ?`,
	},
	{
		Name: "status.runs_active", Category: CatStatus,
		Description: "runs that are live right now — anything not yet in a terminal state",
		SQL: `SELECT run_id, user_id, task_id, state, lane, updated_ts FROM runs` +
			` WHERE state IN ('new', 'queued', 'claimed', 'running', 'draining', 'parked')` +
			scoped("user_id") + ` ORDER BY updated_ts DESC LIMIT ?`,
	},
	{
		Name: "status.runs_for_task", Category: CatStatus,
		Description: "every run of one task — its intake, compose, execute and verify legs",
		Slots:       []Slot{{"task_id", SlotTaskID, "the task id"}},
		SQL: `SELECT run_id, user_id, state, lane, created_ts, updated_ts FROM runs WHERE task_id = ?` +
			scoped("user_id") + ` ORDER BY created_ts LIMIT ?`,
	},
	{
		Name: "status.runs_in_date_range", Category: CatStatus,
		Description: "runs created within a date range",
		Slots: []Slot{
			{"from", SlotDateFrom, "inclusive start, RFC3339 or YYYY-MM-DD"},
			{"to", SlotDateTo, "exclusive end, RFC3339 or YYYY-MM-DD"},
		},
		SQL: `SELECT run_id, user_id, task_id, state, lane, created_ts FROM runs WHERE created_ts >= ? AND created_ts < ?` +
			scoped("user_id") + ` ORDER BY created_ts DESC LIMIT ?`,
	},
	{
		Name: "status.runs_by_lane", Category: CatStatus,
		Description: "runs on one provider lane",
		Slots:       []Slot{{"lane", SlotLane, "the lane name"}},
		SQL: `SELECT run_id, user_id, task_id, state, created_ts FROM runs WHERE lane = ?` +
			scoped("user_id") + ` ORDER BY created_ts DESC LIMIT ?`,
	},
	{
		Name: "status.run_state_history", Category: CatStatus,
		Description: "the lifecycle transitions of one run, in event order",
		Slots:       []Slot{{"run_id", SlotRunID, "the run id"}},
		SQL: `SELECT event_seq, ts, json_extract(payload, '$.from') AS from_state, json_extract(payload, '$.to') AS to_state` +
			` FROM run_events WHERE run_id = ? AND type = 'run.state_changed'` +
			scoped("user_id") + ` ORDER BY event_seq LIMIT ?`,
	},
	{
		Name: "status.tasks_by_kanban", Category: CatStatus,
		Description: "tasks sitting in one kanban column",
		Slots:       []Slot{{"kanban_status", SlotStatus, "the kanban column"}},
		SQL: `SELECT task_id, user_id, title, kanban_status, created_ts FROM tasks WHERE kanban_status = ?` +
			scoped("user_id") + ` ORDER BY created_ts DESC LIMIT ?`,
	},
	{
		// P3-RW-18 D4-R1. A household reader asked "what got cancelled recently
		// and why?" and the fifty queries held no cancel vocabulary at all — so
		// the grammar's only options were fifty wrong answers or an abstain,
		// and the classifier guessed. The right answer has to EXIST before any
		// gate can protect it; this is that answer.
		//
		// No slots: "recently" is the ORDER BY and the LIMIT, not a filter — and
		// a slotless entry also means no second model call, so the reliability
		// floor answers this question with one.
		//
		// Two predicate legs, because the record has two shapes. The first is
		// the D1-R1 discriminator, written by the ONE cancel-detail constructor
		// (internal/run.CancelCauseHuman, internal/run/canceldetail.go — the
		// literal is repeated here because the catalog is SQL data, and a test
		// pins the two spellings together). The second matches the `(4.5)` the
		// cancel reasons have always carried, so runs cancelled BEFORE mint
		// parity still list — a history that silently began at a fix is not a
		// history. `cancelled_by` prefers the structured actor and falls back to
		// the transition's actor unless that is the platform, which on a
		// pre-parity row means "who is not recorded here" rather than "the
		// platform decided".
		Name: "status.tasks_cancelled", Category: CatStatus,
		Description: "tasks that were cancelled or stopped — who cancelled each one, why, and when, most recent first",
		// cancelled_by reads the SAME two legs internal/api's task-page derive
		// reads (reads.go cancelDecisions), in the same order, because the two
		// surfaces answering one question differently is what the walk found:
		// the banner said "cancelled by op" while this row said "—". The
		// structured leg WINS; the ledger leg only FILLS, for rows minted before
		// the structured detail existed; the NULLIF-platform fallback stays last;
		// and a row where all three are empty stays NULL, which is an honest
		// residual absence and not an error (P3-RW-19 R1).
		//
		// The ledger leg is a correlated SCALAR subquery on the same run, never
		// a join: a join would widen this row's `GROUP BY t.task_id` answer, and
		// one act must stay one row. It adds no bind placeholder, so the §37
		// structural rules hold unchanged.
		//
		// Its qualification is reads.go's, restated in SQL: the LAST decisions
		// entry (each update carries the whole ledger, so the entry this update
		// added is the last one), authored `human`, opening with one of the two
		// cancel mint prefixes — `ledger.decide` also carries retries, accepts
		// and revisions, and the entry has no typed kind to read instead. Every
		// literal below is pinned to its producer by cancelrow_rw19_test.go —
		// three to internal/ledger's exported constants, the two prefixes to the
		// mint-side tests that pin the same spellings in stage and intake.
		//
		// BOUNDED DIVERGENCE, stated rather than papered over (drain r1 F1).
		// `lower(trim(...))` here is not literally the same function as leg B's
		// `strings.ToLower(strings.TrimSpace(...))`: SQLite's trim strips spaces
		// only and its lower folds ASCII only, while Go's do the whole Unicode
		// space. A ledger text led by a TAB would therefore attribute on the task
		// page and not here. No producer can write one: every mint concatenates
		// one of these prefixes as a literal head, so the matched region is
		// always exactly those bytes — and while both spellings are pure
		// printable ASCII with no leading whitespace, the two implementations
		// agree on them exactly. That is the invariant, and it is CHECKED
		// (TestCancelMintPrefixesStayInTheAsciiSubsetBothReadersAgreeOn), so a
		// mint reworded into a Unicode-space-led text fails at the pin instead of
		// silently splitting the two surfaces. Widening this SQL toward Unicode
		// would be a second spelling of a qualification that already has one.
		//
		// `reason` serves the PERSON'S OWN WORDS, or the frozen honest absence:
		// nothing renders between this row and its reader, so the honest line
		// has to be IN the row (the §37 'UNPRICED' / '(no project)' precedent).
		// The mechanical sentence is no longer served here — it is still served
		// verbatim by status.run_state_history and by the task page — because a
		// row whose question is "who cancelled each one, why, and when" must not
		// answer the why three different ways.
		//
		// `cause` is the mechanical ENDING in plain words, written for someone
		// who is not a programmer (CONVENTIONS §57): the running-stop and the
		// waiting/queued-withdrawal are different facts and read differently,
		// and no rule citation appears in either.
		SQL: `SELECT t.task_id, t.user_id, t.title, MAX(e.event_seq) AS event_seq, e.ts AS cancelled_ts,` +
			` COALESCE(json_extract(e.payload, '$.detail.actor'),` +
			`          (SELECT json_extract(le.payload, '$.change.actor') FROM run_events le` +
			`            WHERE le.run_id = r.run_id AND le.type = 'ledger_update'` +
			`              AND json_extract(le.payload, '$.change.verb') = 'ledger.decide'` +
			`              AND json_extract(le.payload, '$.ledger.decisions[#-1].author') = 'human'` +
			`              AND (instr(lower(trim(COALESCE(json_extract(le.payload, '$.ledger.decisions[#-1].text'), ''))),` +
			`                         'requester cancelled') = 1` +
			`                OR instr(lower(trim(COALESCE(json_extract(le.payload, '$.ledger.decisions[#-1].text'), ''))),` +
			`                         'requester: rethink') = 1)` +
			`            ORDER BY le.event_seq DESC LIMIT 1),` +
			`          NULLIF(json_extract(e.payload, '$.actor'), 'platform')) AS cancelled_by,` +
			` COALESCE(json_extract(e.payload, '$.detail.reason'), 'no reason given') AS reason,` +
			` CASE json_extract(e.payload, '$.to')` +
			`      WHEN 'completed' THEN 'stopped while it was already running'` +
			`      WHEN 'finalized' THEN 'called off before it went any further'` +
			`      ELSE 'called off' END AS cause` +
			` FROM tasks t` +
			` JOIN runs r ON r.task_id = t.task_id` +
			` JOIN run_events e ON e.run_id = r.run_id AND e.type = 'run.state_changed'` +
			`  AND (json_extract(e.payload, '$.detail.cause') = 'human cancel (4.5)'` +
			`       OR instr(COALESCE(json_extract(e.payload, '$.reason'), ''), '(4.5)') > 0)` +
			` WHERE t.kanban_status = 'cancelled'` +
			scoped("t.user_id") + ` GROUP BY t.task_id ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "status.waiting_on_human", Category: CatStatus,
		Description: "runs blocked on an unanswered question — the waiting-on-human queue",
		SQL: `SELECT a.ask_id, a.run_id, a.user_id, a.status, a.observed_ts, r.state AS run_state` +
			` FROM asks a JOIN runs r ON r.run_id = a.run_id WHERE a.answered_ts IS NULL` +
			scoped("a.user_id") + ` ORDER BY a.observed_ts DESC LIMIT ?`,
	},

	// ── failures ──────────────────────────────────────────────────────────────
	{
		Name: "failures.runs_crashed", Category: CatFailures,
		Description: "runs that crashed",
		SQL: `SELECT run_id, user_id, task_id, lane, updated_ts FROM runs WHERE state = 'crashed'` +
			scoped("user_id") + ` ORDER BY updated_ts DESC LIMIT ?`,
	},
	{
		Name: "failures.runs_died_at_gate", Category: CatFailures,
		Description: "runs that died at a gate — the ask outlived the engine session",
		SQL: `SELECT run_id, user_id, task_id, lane, updated_ts FROM runs WHERE state = 'died-at-gate'` +
			scoped("user_id") + ` ORDER BY updated_ts DESC LIMIT ?`,
	},
	{
		Name: "failures.watchdog_flags", Category: CatFailures,
		Description: "recent watchdog flags with their rule, anomaly class and severity",
		SQL: `SELECT event_seq, ts, COALESCE(run_id, '') AS run_id, json_extract(payload, '$.rule') AS rule,` +
			` json_extract(payload, '$.anomaly_class') AS anomaly_class, json_extract(payload, '$.severity') AS severity,` +
			` json_extract(payload, '$.parked') AS parked` +
			` FROM run_events WHERE type = 'watchdog.flagged'` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "failures.watchdog_flags_by_rule", Category: CatFailures,
		Description: "watchdog flags raised by one rule",
		Slots:       []Slot{{"rule", SlotRule, "the watchdog rule id"}},
		SQL: `SELECT event_seq, ts, COALESCE(run_id, '') AS run_id, json_extract(payload, '$.severity') AS severity,` +
			` json_extract(payload, '$.detail') AS detail` +
			` FROM run_events WHERE type = 'watchdog.flagged' AND json_extract(payload, '$.rule') = ?` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "failures.runs_wedged", Category: CatFailures,
		Description: "runs the recovery layer marked wedged",
		SQL: `SELECT event_seq, ts, COALESCE(run_id, '') AS run_id, user_id FROM run_events WHERE type = 'run.wedged'` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "failures.unmetered_defects", Category: CatFailures,
		Description: "local duty calls that could not write their mandatory D7 metering row — never silent (S12.1 R18)",
		SQL: `SELECT event_seq, ts, json_extract(payload, '$.run') AS run, json_extract(payload, '$.duty') AS duty,` +
			` json_extract(payload, '$.cause') AS cause` +
			` FROM run_events WHERE type = 'local.unmetered_defect'` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "failures.by_lane", Category: CatFailures,
		Description: "how many runs failed on each lane, worst first",
		SQL: `SELECT lane, count(*) AS failed_runs FROM runs WHERE state IN ('crashed', 'died-at-gate')` +
			scoped("user_id") + ` GROUP BY lane ORDER BY failed_runs DESC LIMIT ?`,
	},
	{
		Name: "failures.for_run", Category: CatFailures,
		Description: "everything that went wrong on one run — flags, wedges and terminal transitions",
		Slots:       []Slot{{"run_id", SlotRunID, "the run id"}},
		SQL: `SELECT event_seq, ts, type, json_extract(payload, '$.rule') AS rule, json_extract(payload, '$.to') AS to_state` +
			` FROM run_events WHERE run_id = ? AND type IN ('watchdog.flagged', 'run.wedged', 'run.state_changed')` +
			scoped("user_id") + ` ORDER BY event_seq LIMIT ?`,
	},

	// ── verdicts ──────────────────────────────────────────────────────────────
	{
		Name: "verdicts.recent", Category: CatVerdicts,
		Description: "the most recent verification verdicts",
		SQL: `SELECT event_seq, ts, COALESCE(run_id, '') AS run_id, json_extract(payload, '$.round') AS round,` +
			` json_extract(payload, '$.verdict') AS verdict, json_extract(payload, '$.domain') AS domain` +
			` FROM run_events WHERE type IN ('verdict.recorded', 'verify.round')` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "verdicts.for_run", Category: CatVerdicts,
		Description: "every verdict round on one run, in order",
		Slots:       []Slot{{"run_id", SlotRunID, "the run id"}},
		SQL: `SELECT event_seq, ts, json_extract(payload, '$.round') AS round, json_extract(payload, '$.verdict') AS verdict,` +
			` json_extract(payload, '$.rubric_id') AS rubric_id, json_extract(payload, '$.judge_model') AS judge_model` +
			` FROM run_events WHERE run_id = ? AND type IN ('verdict.recorded', 'verify.round')` +
			scoped("user_id") + ` ORDER BY event_seq LIMIT ?`,
	},
	{
		Name: "verdicts.adverse", Category: CatVerdicts,
		Description: "verdicts that did not ship — REVISE, ESCALATE or REOPEN-SPEC",
		SQL: `SELECT event_seq, ts, COALESCE(run_id, '') AS run_id, json_extract(payload, '$.round') AS round,` +
			` json_extract(payload, '$.verdict') AS verdict, json_extract(payload, '$.domain') AS domain` +
			` FROM run_events WHERE type IN ('verdict.recorded', 'verify.round')` +
			` AND json_extract(payload, '$.verdict') IN ('REVISE', 'ESCALATE', 'REOPEN-SPEC')` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "verdicts.by_rubric", Category: CatVerdicts,
		Description: "verdicts judged against one rubric",
		Slots:       []Slot{{"rubric_id", SlotRubric, "the rubric id"}},
		SQL: `SELECT event_seq, ts, COALESCE(run_id, '') AS run_id, json_extract(payload, '$.verdict') AS verdict,` +
			` json_extract(payload, '$.rubric_version') AS rubric_version` +
			` FROM run_events WHERE type IN ('verdict.recorded', 'verify.round')` +
			` AND json_extract(payload, '$.rubric_id') = ?` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "verdicts.by_domain", Category: CatVerdicts,
		Description: "verdicts in one domain — the domain axis, read from the verdict record itself",
		Slots:       []Slot{{"domain", SlotDomain, "the domain (software, web-research, …)"}},
		SQL: `SELECT event_seq, ts, COALESCE(run_id, '') AS run_id, json_extract(payload, '$.verdict') AS verdict,` +
			` json_extract(payload, '$.round') AS round` +
			` FROM run_events WHERE type IN ('verdict.recorded', 'verify.round')` +
			` AND json_extract(payload, '$.domain') = ?` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "verdicts.rework_rounds", Category: CatVerdicts,
		Description: "how many verification rounds each run needed — rounds beyond the first are the rework",
		SQL: `SELECT COALESCE(run_id, '') AS run_id, max(json_extract(payload, '$.round')) AS rounds` +
			` FROM run_events WHERE type IN ('verdict.recorded', 'verify.round')` +
			scoped("user_id") + ` GROUP BY run_id ORDER BY rounds DESC LIMIT ?`,
	},
	{
		Name: "verdicts.self_family_judge", Category: CatVerdicts,
		Description: "verdicts where the judge was in the same model family as the author — always flagged (G1 Def.1)",
		SQL: `SELECT event_seq, ts, COALESCE(run_id, '') AS run_id, json_extract(payload, '$.verdict') AS verdict,` +
			` json_extract(payload, '$.judge_model') AS judge_model` +
			` FROM run_events WHERE type IN ('verdict.recorded', 'verify.round')` +
			` AND json_extract(payload, '$.self_family_judge') = 1` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		// The S14.8 / S14.5 recorded suite results — the D4(b) serving read.
		//
		// Until B6-6 the `eval.score_recorded` rows minted by
		// internal/conformance.RecordResult (and driven through it by the S14.8
		// evals) were reachable only through Layer-2 open SQL or a redacted
		// search: a deterministic question with no deterministic answer. This is
		// the sanctioned-narrow addition that fixes it, and it stays narrow —
		// one entry, no new category, no guardrail change.
		//
		// NAMING, and it is a deviation worth stating: the disposition called
		// this `eval.scores`, but `Category` is a CLOSED, spec-named set (S14.10
		// ¶2) and every catalog name's prefix is its category. Inventing an
		// `eval` category to match a name would have been a larger change than
		// naming the query for the category it belongs to. A recorded result IS
		// a verdict on an asset version against a registered floor.
		//
		// COLUMNS are what the producer actually mints. There is no top-level
		// `score`, `floor` or `pass` key: `result` is the green/red outcome, and
		// the floor lives inside `metrics` and only on the runbook path — so an
		// asset evaluated by the sweep reports a null floor, which is the honest
		// "this path registers none" rather than a zero.
		//
		// OWNER SCOPE: the producer attributes these rows to `platform`, so the
		// operator sees every row and a member sees none. That is the same
		// answer the drift and canary entries give for the same reason, and it
		// is the scope predicate doing its job rather than an omission.
		Name: "verdicts.eval_scores", Category: CatVerdicts,
		Description: "recorded regression-eval and conformance suite results — suite, asset, outcome and the registered floor where one applies",
		SQL: `SELECT event_seq, ts, json_extract(payload, '$.suite_id') AS suite_id,` +
			` json_extract(payload, '$.suite_version') AS suite_version,` +
			` json_extract(payload, '$.asset_id') AS asset_id,` +
			` json_extract(payload, '$.asset_version') AS asset_version,` +
			` json_extract(payload, '$.result') AS result,` +
			` json_extract(payload, '$.metrics.floor') AS floor,` +
			` json_extract(payload, '$.metrics.floor_green') AS floor_green,` +
			` json_extract(payload, '$.runner') AS runner` +
			` FROM run_events WHERE type = 'eval.score_recorded'` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},

	// ── routing quality ───────────────────────────────────────────────────────
	{
		Name: "routing.decisions_recent", Category: CatRoutingQuality,
		Description: "the most recent routing decisions with their cause and plain-language reason",
		SQL: `SELECT event_seq, decided_ts, run_id, cause, model, lane, worker_name, plain_reason` +
			` FROM routing_quality WHERE 1 = 1` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "routing.by_cause", Category: CatRoutingQuality,
		Description: "routing decisions made for one cause (selector-match, no-fit-generalist, override, pinned, helper-spawn)",
		Slots:       []Slot{{"cause", SlotCause, "the routing cause"}},
		SQL: `SELECT event_seq, decided_ts, run_id, model, lane, worker_name, reworked, verdicts_adverse` +
			` FROM routing_quality WHERE cause = ?` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "routing.by_worker", Category: CatRoutingQuality,
		Description: "every decision that routed to one worker, with what happened next — the worker axis",
		Slots:       []Slot{{"worker", SlotWorker, "the worker template id"}},
		SQL: `SELECT event_seq, decided_ts, run_id, worker_version, domain, run_final_state, verify_rounds, reworked` +
			` FROM routing_quality WHERE worker_template_id = ?` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "routing.by_lane", Category: CatRoutingQuality,
		Description: "routing decisions onto one lane",
		Slots:       []Slot{{"lane", SlotLane, "the lane name"}},
		SQL: `SELECT event_seq, decided_ts, run_id, cause, model, run_final_state, reworked` +
			` FROM routing_quality WHERE lane = ?` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "routing.for_run", Category: CatRoutingQuality,
		Description: "how one run was routed and how it turned out",
		Slots:       []Slot{{"run_id", SlotRunID, "the run id"}},
		SQL: `SELECT event_seq, decided_ts, cause, score, model, lane, worker_name, worker_version, effort,` +
			` degraded, plain_reason, run_final_state, verdicts, verdicts_adverse, verify_rounds` +
			` FROM routing_quality WHERE run_id = ?` +
			scoped("user_id") + ` ORDER BY event_seq LIMIT ?`,
	},
	{
		Name: "routing.rework_by_cause", Category: CatRoutingQuality,
		Description: "the rework rate per routing cause — the S2.6 auditable-over-time figure",
		SQL: `SELECT cause, count(*) AS decisions, sum(reworked) AS reworked_runs,` +
			` sum(verdicts_adverse) AS adverse_verdicts` +
			` FROM routing_quality WHERE 1 = 1` +
			scoped("user_id") + ` GROUP BY cause ORDER BY decisions DESC LIMIT ?`,
	},
	{
		Name: "routing.rework_by_worker", Category: CatRoutingQuality,
		Description: "the rework rate per worker version — the S08.4 version→outcome join",
		SQL: `SELECT worker_template_id, worker_version, domain, count(*) AS decisions, sum(reworked) AS reworked_runs` +
			` FROM routing_quality WHERE worker_template_id <> ''` +
			scoped("user_id") + ` GROUP BY worker_template_id, worker_version, domain ORDER BY decisions DESC LIMIT ?`,
	},
	{
		Name: "routing.degraded", Category: CatRoutingQuality,
		Description: "decisions marked degraded — the domain lacked full verification maturity",
		SQL: `SELECT event_seq, decided_ts, run_id, cause, domain, model, plain_reason` +
			` FROM routing_quality WHERE degraded = 1` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},

	// ── drift history ─────────────────────────────────────────────────────────
	{
		Name: "drift.findings_recent", Category: CatDriftHistory,
		Description: "the most recent outside-world drift findings",
		SQL: `SELECT event_seq, ts, json_extract(payload, '$.source') AS source, COALESCE(change_class, '') AS change_class,` +
			` json_extract(payload, '$.severity') AS severity, json_extract(payload, '$.summary') AS summary` +
			` FROM run_events WHERE type = 'drift.finding'` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "drift.by_change_class", Category: CatDriftHistory,
		Description: "drift findings of one change class",
		Slots:       []Slot{{"change_class", SlotChangeClass, "the change class"}},
		SQL: `SELECT event_seq, ts, json_extract(payload, '$.source') AS source, json_extract(payload, '$.severity') AS severity,` +
			` json_extract(payload, '$.summary') AS summary` +
			` FROM run_events WHERE type = 'drift.finding' AND COALESCE(change_class, '') = ?` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "drift.by_source", Category: CatDriftHistory,
		Description: "drift findings from one watched source",
		Slots:       []Slot{{"source", SlotSource, "the watch source id"}},
		SQL: `SELECT event_seq, ts, COALESCE(change_class, '') AS change_class, json_extract(payload, '$.severity') AS severity,` +
			` json_extract(payload, '$.summary') AS summary` +
			` FROM run_events WHERE type = 'drift.finding' AND json_extract(payload, '$.source') = ?` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "drift.by_severity", Category: CatDriftHistory,
		Description: "drift findings at one severity",
		Slots:       []Slot{{"severity", SlotSeverity, "the severity label"}},
		SQL: `SELECT event_seq, ts, json_extract(payload, '$.source') AS source, COALESCE(change_class, '') AS change_class,` +
			` json_extract(payload, '$.summary') AS summary` +
			` FROM run_events WHERE type = 'drift.finding' AND json_extract(payload, '$.severity') = ?` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "drift.in_date_range", Category: CatDriftHistory,
		Description: "drift findings within a date range",
		Slots: []Slot{
			{"from", SlotDateFrom, "inclusive start, RFC3339 or YYYY-MM-DD"},
			{"to", SlotDateTo, "exclusive end, RFC3339 or YYYY-MM-DD"},
		},
		SQL: `SELECT event_seq, ts, json_extract(payload, '$.source') AS source, COALESCE(change_class, '') AS change_class,` +
			` json_extract(payload, '$.summary') AS summary` +
			` FROM run_events WHERE type = 'drift.finding' AND ts >= ? AND ts < ?` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "drift.for_lane", Category: CatDriftHistory,
		Description: "drift findings that name one lane among the lanes they affect",
		Slots:       []Slot{{"lane", SlotLane, "the lane name"}},
		SQL: `SELECT event_seq, ts, json_extract(payload, '$.source') AS source, COALESCE(change_class, '') AS change_class,` +
			` json_extract(payload, '$.summary') AS summary` +
			` FROM run_events WHERE type = 'drift.finding'` +
			` AND EXISTS (SELECT 1 FROM json_each(payload, '$.lanes') WHERE json_each.value = ?)` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "drift.canary_results", Category: CatDriftHistory,
		Description: "canary results — the behavioral and auth probes against the outside world",
		SQL: `SELECT event_seq, ts, json_extract(payload, '$.canary_kind') AS canary_kind, json_extract(payload, '$.lane') AS lane,` +
			` json_extract(payload, '$.result') AS result, json_extract(payload, '$.summary') AS summary` +
			` FROM run_events WHERE type = 'canary.result'` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "drift.canary_failures", Category: CatDriftHistory,
		Description: "canary probes that did not pass",
		SQL: `SELECT event_seq, ts, json_extract(payload, '$.canary_kind') AS canary_kind, json_extract(payload, '$.lane') AS lane,` +
			` json_extract(payload, '$.result') AS result, json_extract(payload, '$.detail') AS detail` +
			` FROM run_events WHERE type = 'canary.result' AND json_extract(payload, '$.result') <> 'pass'` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},

	// ── cost — every entry SELECTS a Layer-0 view (S14.10 ¶1) ─────────────────
	{
		Name: "cost.for_run", Category: CatCost,
		Description: "what one run cost, from the priced receipt",
		Slots:       []Slot{{"run_id", SlotRunID, "the run id"}},
		SQL: `SELECT run_id, user_id, task_id, lane, state, pricing_status, priced_usd, total_calls, unpriced_calls, currency` +
			` FROM cost_per_run WHERE run_id = ?` +
			scoped("user_id") + ` ORDER BY run_id LIMIT ?`,
	},
	{
		Name: "cost.for_task", Category: CatCost,
		Description: "what one task cost across all of its runs",
		Slots:       []Slot{{"task_id", SlotTaskID, "the task id"}},
		SQL: `SELECT task_id, user_id, title, runs, pricing_status, priced_usd, unpriced_runs, total_calls` +
			` FROM cost_per_task WHERE task_id = ?` +
			scoped("user_id") + ` ORDER BY task_id LIMIT ?`,
	},
	{
		Name: "cost.for_project", Category: CatCost,
		Description: "what one project cost — the project axis, via the intake-resolved linkage",
		Slots:       []Slot{{"project", SlotProject, "the project id, or '(no project)' for unassigned work"}},
		SQL: `SELECT project_id, project_name, user_id, runs, pricing_status, priced_usd, unpriced_runs, total_calls, linkage` +
			` FROM cost_per_project WHERE project_id = ?` +
			scoped("user_id") + ` ORDER BY project_id LIMIT ?`,
	},
	{
		Name: "cost.for_person", Category: CatCost,
		Description: "what one person's work cost — the person axis",
		Slots:       []Slot{{"person", SlotPerson, "the person's user id"}},
		SQL: `SELECT user_id, runs, pricing_status, priced_usd, unpriced_runs, total_calls, first_run_ts, last_run_ts` +
			` FROM cost_per_person WHERE user_id = ?` +
			scoped("user_id") + ` ORDER BY user_id LIMIT ?`,
	},
	{
		Name: "cost.by_period", Category: CatCost,
		Description: "consumption per day within a date range",
		Slots: []Slot{
			{"from", SlotDateFrom, "inclusive start, YYYY-MM-DD"},
			{"to", SlotDateTo, "exclusive end, YYYY-MM-DD"},
		},
		SQL: `SELECT day, user_id, receipts, pricing_status, priced_usd, total_calls, unpriced_calls` +
			` FROM cost_per_period WHERE day >= ? AND day < ?` +
			scoped("user_id") + ` ORDER BY day DESC LIMIT ?`,
	},
	{
		Name: "cost.burn_rate", Category: CatCost,
		Description: "the burn rate — usd per observed day, with the span it was observed over",
		SQL: `SELECT user_id, usd_per_day, priced_usd, active_days, observed_days, first_day, last_day, pricing_status, rate_basis` +
			` FROM cost_burn_rate WHERE 1 = 1` +
			scoped("user_id") + ` ORDER BY usd_per_day DESC LIMIT ?`,
	},
	{
		Name: "cost.budget_remainder", Category: CatCost,
		Description: "the budget remainder — a recorded absence at v0, with its reason",
		SQL: `SELECT user_id, budget_declared, budget_usd, remainder_usd, remainder_status, absence_reason, consumed_priced_usd` +
			` FROM cost_budget_remainder WHERE 1 = 1` +
			scoped("user_id") + ` ORDER BY user_id LIMIT ?`,
	},
	{
		Name: "cost.done_directly", Category: CatCost,
		Description: "the done-directly line per run, cited from the registered formula",
		SQL: `SELECT run_id, user_id, receipt_ts, label, formula_ref, pricing_status, heuristic_usd, absence_reason` +
			` FROM cost_done_directly WHERE 1 = 1` +
			scoped("user_id") + ` ORDER BY receipt_ts DESC LIMIT ?`,
	},
	{
		Name: "cost.limit_events", Category: CatCost,
		Description: "the provider limit signals hit, with class, signal and reset time",
		SQL: `SELECT event_seq, ts, user_id, run_id, type, limit_class, provider_signal, resets_at, lane` +
			` FROM limit_event_history WHERE 1 = 1` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
	{
		Name: "cost.limit_events_in_range", Category: CatCost,
		Description: "limit signals hit within a date range",
		Slots: []Slot{
			{"from", SlotDateFrom, "inclusive start, RFC3339 or YYYY-MM-DD"},
			{"to", SlotDateTo, "exclusive end, RFC3339 or YYYY-MM-DD"},
		},
		SQL: `SELECT event_seq, ts, user_id, run_id, limit_class, provider_signal, resets_at, lane` +
			` FROM limit_event_history WHERE ts >= ? AND ts < ?` +
			scoped("user_id") + ` ORDER BY event_seq DESC LIMIT ?`,
	},
}

// Catalog returns the Layer-1 catalog, sorted by name.
func Catalog() []Query {
	out := append([]Query(nil), catalog...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// QueryByName looks up a catalog entry.
func QueryByName(name string) (Query, bool) {
	for _, q := range catalog {
		if q.Name == name {
			return q, true
		}
	}
	return Query{}, false
}

// CatalogNames returns every query name, sorted — the enum the intent schema is
// built over.
func CatalogNames() []string {
	out := make([]string, 0, len(catalog))
	for _, q := range catalog {
		out = append(out, q.Name)
	}
	sort.Strings(out)
	return out
}

// choicesForCard renders the catalog as disambiguation choices.
func choicesForCard(names []string) []Choice {
	var out []Choice
	for _, n := range names {
		if q, ok := QueryByName(n); ok {
			out = append(out, Choice{Query: q.Name, Category: string(q.Category), Description: q.Description})
		}
	}
	return out
}
