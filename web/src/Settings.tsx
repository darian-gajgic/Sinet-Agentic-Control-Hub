import { JsonForms } from '@jsonforms/react'
import { useCallback, useMemo, useState } from 'react'

import {
  api,
  type Answer,
  type SettingValue,
  type SettingsHistory,
  type SettingsView,
  type StoredPricesView,
} from './api'
import type { EventStream } from './events'
import { describeError, useLive } from './live'
import { PushDevices } from './PushDevices'
import { Absent, Freshness, Stamp, SurfaceHead } from './parts'
import { Button, EmptyState } from './ui'
import {
  HistoryRows,
  Panel,
  PerUserValues,
  changedKeys,
  settingsData,
  sinetRenderers,

  type FormContext,
} from './settingsForm'

/**
 * The settings tab (Spec S15.9; S01.10; S18; FC-v1 §4; 13.4/13.5).
 *
 * Every operator-relevant number the platform has, on one page, with its clamp
 * bounds, its badges, its help and its audit trail — plus the two surfaces that
 * are settings-shaped without being settings rows: the S18.3 price table and
 * the recorded eval results.
 *
 * FOUR RULES SHAPE IT.
 *
 * The form is GENERATED, never hand-built: the registry emits the JSON Schema
 * and the UISchema, this page consumes them verbatim through the Sinet-owned
 * renderer set, and no key list exists in this tree. A key added to the S18
 * index appears here because the emitter emitted it.
 *
 * A WRITE IS AN AUDITED ACT, so it never fires on a keystroke. Typing changes
 * the form model; Save is a separate, explicit act that posts ONE key. That is
 * the landed write grain — there is no bulk write — and it is also the honest
 * one: every save lands its clamp check, its audit row and its settings.changed
 * event in one transaction, and a person should know they did that.
 *
 * AUTHORITY IS SERVED. `editable` decides whether any write affordance renders
 * at all. For a member the whole write surface is ABSENT — not disabled — and
 * the server's own reason is shown once, prominently. The UI never renders a
 * control that would 403.
 *
 * THE REGISTRY IS THE VALIDATOR. Client-side clamping exists for feel; the
 * registry decides, and its refusals render verbatim beside the setting that
 * earned them.
 */
export function Settings({ stream }: { stream?: EventStream }) {
  const live = useLive<SettingsView>({
    key: '/api/settings',
    // A settings change is announced by `settings.changed`, and the surface
    // re-reads on it like every other view re-reads on its own facts.
    read: () => api.settings(),
    types: settingsEventTypes,
    stream,
  })

  return (
    <section className="surface settings">
      {/* The D8 "what this is" line. Its two facts are the page's own claim —
          the form is the registry's emission, so a setting is here because the
          registry declares it — and what a row carries. It deliberately does NOT
          promise editing: whether any write affordance renders at all is the
          SERVER's answer, stated once below in the server's own words. */}
      <SurfaceHead
        title="Settings"
        what="Every setting the platform has, each with its clamp bounds, its badges, its help and the history of who changed it. The page is generated from the registry rather than hand-built, so a setting appears here because the registry declares it."
      />
      <Freshness stale={live.stale} error={live.error} hasData={live.data !== null} />
      {live.data && (
        <>
          <Authority view={live.data} />
          <SettingsForm view={live.data} onWritten={live.reload} />
          <Panel title="Per-person overrides">
            <p className="max-w-prose text-sm text-muted-foreground">
              A per-person override is one setting with more than one value, so it lives beside the form rather than in
              it. The registry records an operator or an automation actor and admits no member kind, so a person&apos;s
              own override is set FOR them by the operator.
            </p>
            <PerUserValues values={live.data.values} />
          </Panel>
          <RegisteredBlock view={live.data} />
          {/* Per-DEVICE state, beside the generated form rather than inside it:
              push enrolment is not a ⚙ registry key, so this panel renders none
              and §43-B's no-hand-built-registry-control rule stays intact (OQ10). */}
          <PushDevices stream={stream} />
          <PriceTable stream={stream} />
          <EvalResults stream={stream} />
        </>
      )}
    </section>
  )
}

/** The types that change what this page shows. `settings.changed` is the
 *  registry's own row, minted inside the same transaction as the write. */
const settingsEventTypes = ['settings.changed'] as const

/** The served authority statement, shown ONCE and prominently — not repeated
 *  beside every control it explains.
 *
 *  The reason is the SERVER's sentence and it is this element's whole content:
 *  the platform serves exactly three of them (internal/api/settings.go:455, :458,
 *  :460) and the client can tell the three postures apart only through it, so
 *  paraphrasing one or adding to it would erase the only posture signal there
 *  is. Anything this client has to say goes in the sibling block below. */
function Authority({ view }: { view: SettingsView }) {
  return (
    <>
      <p
        className={
          view.editable
            ? 'max-w-prose text-sm text-[var(--green)]'
            : 'max-w-prose text-sm text-[var(--yellow)]'
        }
        data-editable={view.editable ? 'true' : 'false'}
      >
        {view.editable_reason}
      </p>
      {!view.editable && <ReadOnlyExplanation />}
    </>
  )
}

/**
 * The A-2 fix (B6 gate §9): the page says out loud why its editors are missing.
 *
 * THE FINDING IT ANSWERS. The write surface is absent-not-disabled — the no-403
 * -controls rule, and the right rule — but the only explanation was one banner
 * line a person has no reason to connect to the missing Save buttons. So a
 * reader saw a page with no editors and a sentence about actor kinds, and had to
 * join them up themselves.
 *
 * WHAT IT MAY NOT DO, and each is asserted in the negative:
 *
 *  - It never restates or paraphrases the served reason. That line is the
 *    server's and sits above, alone, in the one `[data-editable]` element.
 *  - It never says the values, the bounds, the badges, the help or the History
 *    are unavailable, because none of them is: History sits outside the write
 *    gate deliberately (settingsForm.tsx's own note), and every value still
 *    reads through the read-only render.
 *  - It carries NO link to /login. Half its readers are signed-in members, for
 *    whom that door bounces straight back (the shell's own arm) — a control
 *    that cannot do what it says, which is the C-1 defect this batch fixed
 *    rather than a pattern to repeat. The dev-posture reader already has the
 *    header's own Sign-in affordance, which is the door that works.
 *  - It never names a posture. Dev and member are two different reasons and the
 *    SERVER says which one applies; a client-authored "you are in dev posture"
 *    would be this page guessing at something it was told.
 */
function ReadOnlyExplanation() {
  return (
    <div className="max-w-prose text-sm text-muted-foreground" data-readonly-explained="true">
      <p>This page is read-only for you right now — the line above is the platform&apos;s own reason for that.</p>
      <p>
        That is why there is no Save, no Reset, no bounds editor and no way to append a price anywhere below: the
        platform does not show a control it would refuse.
      </p>
      <p>
        Everything else is still here to read — every setting&apos;s value, its bounds, its badges, its help, the
        History of who changed it and from what, the price table and the recorded results.
      </p>
      <p>Editing is real behind a real operator session.</p>
    </div>
  )
}

// ── the generated form ────────────────────────────────────────────────────

function SettingsForm({ view, onWritten }: { view: SettingsView; onWritten: () => void }) {
  const served = useMemo(() => settingsData(view.values), [view.values])
  const byKey = useMemo(() => Object.fromEntries(view.values.map((v) => [v.key, v])), [view.values])
  const [data, setData] = useState<Record<string, unknown>>(served)
  const [busyKey, setBusyKey] = useState('')
  const [outcomes, setOutcomes] = useState<Record<string, { detail: string; failed: boolean }>>({})
  const [history, setHistory] = useState<{ key: string; entries: SettingsHistory['entries'] } | null>(null)

  // The served data is the truth; a re-read replaces the model rather than
  // merging into it, so nothing a person typed outlives the snapshot that
  // supersedes it (§29 derive-from-log, client side).
  const [seen, setSeen] = useState(served)
  if (seen !== served) {
    setSeen(served)
    setData(served)
  }

  const write = useCallback(
    async (key: string, label: string, fire: () => Promise<{ detail: string }>) => {
      setBusyKey(key)
      try {
        const out = await fire()
        setOutcomes((o) => ({ ...o, [key]: { detail: out.detail, failed: false } }))
      } catch (err: unknown) {
        // The registry's refusal is the authority and it renders VERBATIM —
        // this surface never re-classifies or paraphrases one.
        setOutcomes((o) => ({ ...o, [key]: { detail: `${label} refused: ${describeError(err)}`, failed: true } }))
      } finally {
        setBusyKey('')
        onWritten()
      }
    },
    [onWritten],
  )

  const ctx: FormContext = {
    byKey,
    editable: view.editable,
    busyKey,
    outcomes,
    save: (key, value) => {
      void write(key, 'set', () => api.setSetting(key, { value }))
    },
    // Reset is the null write, which DELETES the override row. It is the same
    // verb, and that is why "reset" cannot drift from what the registry means.
    reset: (key) => {
      void write(key, 'reset', () => api.setSetting(key, { value: null }))
    },
    setBounds: (key, floor, ceiling, reason) => {
      void write(key, 'bounds', () => api.setSettingBounds(key, { floor, ceiling, reason }))
    },
    setForUser: (key, user, value) => {
      void write(key, 'per-person set', () =>
        api.setSetting(key, { value: coerceLike(byKey[key], value), for_user: user }),
      )
    },
    openHistory: (key) => {
      void api.settingsHistory(key).then(
        (h) => {
          setHistory({ key, entries: h.entries })
        },
        (err: unknown) => {
          setOutcomes((o) => ({ ...o, [key]: { detail: describeError(err), failed: true } }))
        },
      )
    },
  }

  const dirty = changedKeys(data, view.values)

  return (
    <Panel title={`Every setting (${String(view.values.length)} across ${String(view.domains.length)} domains)`}>
      {dirty.length > 0 && (
        <p className="warn-flag" data-dirty-keys={String(dirty.length)}>
          {String(dirty.length)} unsaved change(s): {dirty.join(', ')}. Nothing is written until you save each one — a
          settings write is an audited act, so it never fires as you type.
        </p>
      )}
      <JsonForms
        schema={view.schema as never}
        uischema={view.uischema as never}
        data={data}
        renderers={sinetRenderers}
        cells={[]}
        config={{ ctx }}
        // THE REGISTRY IS THE ONE VALIDATOR, so the form runs none of its own.
        //
        // This is the S01.10 one-artifact-set rule applied to the client half:
        // a second validator here could refuse a value the registry accepts, or
        // accept one it refuses, and a person would have no way to tell which
        // answer was the platform's. The registry checks every write inside the
        // transaction that records it, and its refusal renders verbatim beside
        // the setting.
        //
        // REPORTED, and this is what surfaced it: the registry emits a DRAFT
        // 2020-12 schema, and JSON Forms 3.8.0's bundled Ajv is configured for
        // draft-07 — so the default validating mode throws
        // "no schema with key or ref .../2020-12/schema" before a single
        // control renders. Reaching for `ajv/dist/2020` would make a transitive
        // dependency a direct one with no lock entry, and stripping `$schema`
        // from a served artifact would be editing what the registry emitted.
        // Neither is acceptable, and neither is needed: the honest posture was
        // already the right one.
        validationMode="NoValidation"
        // JSON Forms DEBOUNCES this by 10ms (jsonforms-react's `debouncedEmit`),
        // so this mirror is deliberately only what the unsaved-changes line
        // reads — never what a write reads. The Save act carries the control's
        // own current value, which cannot lag.
        onChange={({ data: next }) => {
          setData(next as Record<string, unknown>)
        }}
      />
      {history && (
        <div className="history-panel" data-history-for={history.key}>
          <h3>{history.key} — who changed it, from what</h3>
          <HistoryRows entries={history.entries} />
        </div>
      )}
    </Panel>
  )
}

/** coerceLike parses a typed-in per-user value the way the key's own declared
 *  type says to. The registry validates regardless; this only stops a numeric
 *  key being sent the string "5" when the person meant the number. */
function coerceLike(v: SettingValue | undefined, raw: string): unknown {
  if (!v) return raw
  if (v.type === 'bool') return raw === 'true'
  if (v.numeric) {
    const n = Number(raw)
    return Number.isNaN(n) ? raw : n
  }
  return raw
}

/** The S18.4 R9 frozen values, read-only with the marker each one carries from
 *  the package that owns it. Nothing on this page can change one. */
function RegisteredBlock({ view }: { view: SettingsView }) {
  return (
    <Panel title="Registered values (read-only)">
      {view.registered && view.registered.length > 0 ? (
        <dl className="registered">
          {view.registered.map((r) => (
            <div key={r.name} data-registered={r.name}>
              <dt className="font-mono text-xs tabular-nums text-muted-foreground">{r.name}</dt>
              <dd>
                <span className="font-mono tabular-nums">{r.value}</span>{' '}
                <span className="text-muted-foreground">{r.ref}</span>
                <span className="text-[var(--yellow)]"> {r.marker}</span>
              </dd>
            </div>
          ))}
        </dl>
      ) : (
        <Absent reason={view.registered_absent ?? 'no registered-value block was served'} />
      )}
    </Panel>
  )
}

// ── the S18.3 price table ─────────────────────────────────────────────────

/**
 * The price table over the append-only store (S10.3; S18.3).
 *
 * IMMUTABILITY IS RENDERED AS A PROPERTY, NOT HIDDEN AS A LIMITATION. There is
 * no edit and no delete affordance here because no such verb exists: past rows
 * are immutable by database trigger, an effective-dated append never rewrites
 * what a past receipt was charged at, and a UI that offered a pencil icon would
 * be promising something the platform refuses.
 */
function PriceTable({ stream }: { stream?: EventStream }) {
  const live = useLive<StoredPricesView>({
    key: '/api/settings/prices',
    read: () => api.prices(),
    types: settingsEventTypes,
    stream,
  })
  const [outcome, setOutcome] = useState<{ detail: string; failed: boolean } | null>(null)
  const rows = live.data?.rows ?? []

  return (
    <Panel title="Model prices">
      <Freshness stale={live.stale} error={live.error} hasData={live.data !== null} />
      {live.data && (
        <>
          {live.data.posture && (
            <p className="max-w-prose text-sm text-[var(--yellow)]" data-posture="empty-table">
              {live.data.posture}
            </p>
          )}
          {rows.length > 0 && (
            <div className="table-scroll">
              <table className="items">
                <thead>
                  <tr>
                    <th>Model</th>
                    <th>Lane</th>
                    <th>Unit prices</th>
                    <th>Effective from</th>
                    <th>Verified on</th>
                    <th>Source</th>
                    <th>Added by</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr key={r.id} data-price-row={String(r.id)}>
                      <td>{r.model}</td>
                      <td>{r.lane}</td>
                      <td>
                        {Object.entries(r.unit_prices).map(([unit, usd]) => (
                          <span key={unit} className="font-mono tabular-nums whitespace-nowrap" data-unit={unit}>
                            {unit} USD {String(usd)}{' '}
                          </span>
                        ))}
                      </td>
                      <td>
                        <Stamp ts={r.effective_from} />
                      </td>
                      <td>
                        <Stamp ts={r.verified_on} />
                      </td>
                      <td>{r.source}</td>
                      <td>
                        {r.created_by} <Stamp ts={r.created_ts} />
                        {r.reason && <span className="text-muted-foreground"> — {r.reason}</span>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="max-w-prose text-sm text-muted-foreground" data-price-version={live.data.version}>
            table version {live.data.version}. Rows are effective-dated and APPEND-ONLY: adding one never rewrites what
            a past receipt was charged at, and there is no edit or delete — the platform refuses both.
          </p>
          <h4>Stated decisions, not gaps</h4>
          <ul className="deferred">
            {live.data.deferred.map((d) => (
              <li key={d}>{d}</li>
            ))}
          </ul>
          {live.data.editable && (
            <AddPriceRow
              onAdded={(detail, failed) => {
                setOutcome({ detail, failed })
                live.reload()
              }}
            />
          )}
          {outcome && (
            <p
              className={outcome.failed ? 'text-sm text-[var(--red)]' : 'text-sm text-[var(--green)]'}
              data-price-outcome={outcome.failed ? 'failed' : 'added'}
            >
              {outcome.detail}
            </p>
          )}
        </>
      )}
    </Panel>
  )
}

/**
 * The FIVE unit classes a composed cost multiplies (internal/metering's own
 * `unit_prices` keys, S10.3).
 *
 * All five, not the three that are easy: `PricedCost` folds every unit
 * UNCONDITIONALLY, and cache-creation tokens and server-tool calls are real fed
 * classes — so a form that offered three would leave two priced at a traceless
 * $0 inside a cost the receipt calls priced. A golden-fixture test pins this
 * list against the served row's own keys, which is what stops it drifting from
 * the shape the store accepts.
 */
type UnitField =
  | 'input_usd'
  | 'output_usd'
  | 'cache_read_usd'
  | 'cache_creation_usd'
  | 'server_tool_usd'

const unitFields: [UnitField, string][] = [
  ['input_usd', 'input, USD per token'],
  ['output_usd', 'output, USD per token'],
  ['cache_read_usd', 'cache read, USD per token'],
  ['cache_creation_usd', 'cache creation, USD per token'],
  ['server_tool_usd', 'server tool, USD per call'],
]

const blankUnits = (): Record<UnitField, string> =>
  Object.fromEntries(unitFields.map(([f]) => [f, ''])) as Record<UnitField, string>

const noneFree = (): Record<UnitField, boolean> =>
  Object.fromEntries(unitFields.map(([f]) => [f, false])) as Record<UnitField, boolean>

/** positiveNumber is the boundary shape check: `Number('')` is 0 and
 *  `Number('abc')` is NaN, so neither may pass for a price. */
function positiveNumber(raw: string): boolean {
  const n = Number(raw)
  return raw.trim() !== '' && Number.isFinite(n) && n > 0
}

/**
 * The add-row form composes the S10.3 row from its known fields (OQ6). It is a
 * structured form rather than raw JSON because raw JSON is operator-hostile,
 * and it is pinned to the OWNER's shape by a golden fixture marshaled from the
 * real stored row — so the form cannot drift from what the store accepts. The
 * store's validation remains the authority and its refusals render verbatim.
 */
function AddPriceRow({ onAdded }: { onAdded: (detail: string, failed: boolean) => void }) {
  const [row, setRow] = useState({ model: '', lane: '', effective_from: '', verified_on: '', source: '' })
  const [prices, setPrices] = useState<Record<UnitField, string>>(blankUnits())
  const [free, setFree] = useState<Record<UnitField, boolean>>(noneFree())
  const [reason, setReason] = useState('')
  const set = (field: keyof typeof row) => (e: { currentTarget: { value: string } }) => {
    const value = e.currentTarget.value
    setRow((r) => ({ ...r, [field]: value }))
  }

  // EVERY unit must be answered, one way or the other. A unit left blank and
  // not declared free is the one state that composes a lie: the row exists, so
  // the lane is "priced", and the composed table multiplies EVERY unit
  // unconditionally — so a real cache-creation or server-tool reading would be
  // charged a traceless $0 inside a cost the receipt calls priced. Saying "this
  // lane does not charge for that" is a deliberate act the operator takes; the
  // form never takes it for them.
  const answered = (f: UnitField) => free[f] || positiveNumber(prices[f])
  const allAnswered = unitFields.every(([f]) => answered(f))
  const pricedUnits = unitFields.filter(([f]) => !free[f] && positiveNumber(prices[f])).map(([f]) => f)
  const freeUnits = unitFields.filter(([f]) => free[f]).map(([f]) => f)

  return (
    <div className="add-price-row">
      <h4>Add a row</h4>
      <p className="max-w-prose text-sm text-muted-foreground">
        An append, effective-dated. The store validates it — if it refuses, the refusal appears here in its own words.
      </p>
      {(
        [
          ['model', 'model'],
          ['lane', 'lane'],
          ['effective_from', 'effective from'],
          ['verified_on', 'verified on'],
          ['source', 'where you read it'],
        ] as [keyof typeof row, string][]
      ).map(([field, label]) => (
        <label key={field}>
          <span>{label}</span>
          <input type="text" data-price-field={field} value={row[field]} onChange={set(field)} />
        </label>
      ))}

      <fieldset className="unit-prices">
        <legend>Every unit this lane can charge for</legend>
        <p className="max-w-prose text-sm text-muted-foreground">
          A cost is composed from all five. Each one needs either a price or your word that this lane does not charge
          for it — a unit left blank would be charged at nothing, inside a cost the receipt calls priced.
        </p>
        {unitFields.map(([field, label]) => (
          <div key={field} className="unit-row" data-unit-row={field}>
            <label>
              <span>{label}</span>
              <input
                type="text"
                data-price-field={field}
                disabled={free[field]}
                value={prices[field]}
                onChange={(e) => {
                  const value = e.currentTarget.value
                  setPrices((p) => ({ ...p, [field]: value }))
                }}
              />
            </label>
            <label>
              <input
                type="checkbox"
                data-price-free={field}
                checked={free[field]}
                onChange={(e) => {
                  const checked = e.currentTarget.checked
                  setFree((f) => ({ ...f, [field]: checked }))
                  if (checked) setPrices((p) => ({ ...p, [field]: '' }))
                }}
              />
              not charged on this lane
            </label>
            {!answered(field) && (
              <span className="text-xs text-[var(--yellow)]" data-unit-unanswered={field}>
                needs a positive price, or your word that it is not charged
              </span>
            )}
          </div>
        ))}
      </fieldset>

      <label>
        <span>why (recorded with the row)</span>
        <input
          type="text"
          data-price-field="reason"
          value={reason}
          onChange={(e) => {
            setReason(e.currentTarget.value)
          }}
        />
      </label>

      {/* Nothing composes silently: the append says which units it prices and
          which the operator declared this lane does not charge for. */}
      <p className="max-w-prose text-sm text-muted-foreground" data-append-summary={allAnswered ? 'ready' : 'incomplete'}>
        {allAnswered ? (
          <>
            This row will price {pricedUnits.length === 0 ? 'nothing' : pricedUnits.join(', ')}
            {freeUnits.length > 0 && <> and declare {freeUnits.join(', ')} not charged on this lane</>}.
          </>
        ) : (
          <>Every unit needs an answer before this row can be appended.</>
        )}
      </p>

      <Button
        variant="primary"
        data-action="add-price-row"
        disabled={row.model === '' || row.lane === '' || !allAnswered}
        onClick={() => {
          // A declared-free unit is OMITTED from the row rather than sent as a
          // zero. All five declared free composes an empty unit-price set,
          // which the STORE refuses on its own all-zero wall — the authority
          // stays where OQ6 put it, and its sentence is what a person reads.
          const unit_prices: Record<string, number> = {}
          for (const [field] of unitFields) {
            if (!free[field]) unit_prices[field] = Number(prices[field])
          }
          void api
            .addPriceRow(
              {
                model: row.model,
                lane: row.lane,
                unit_prices,
                effective_from: row.effective_from,
                verified_on: row.verified_on,
                source: row.source,
              },
              reason === '' ? undefined : reason,
            )
            .then(
              (res) => {
                onAdded(`${res.detail} (table version ${res.version})`, false)
              },
              (err: unknown) => {
                onAdded(describeError(err), true)
              },
            )
        }}
      >
        Append row
      </Button>
    </div>
  )
}

// ── the eval-results surface (OQ5; the D4(b) unlock) ──────────────────────

/**
 * The recorded suite results, read through the LANDED audited query route.
 *
 * Scores are DATA. Nothing here judges one: the outcome column is the
 * producer's own green/red, the floor is the registered number where the path
 * registers one, and a result with no floor renders as having none rather than
 * as having zero.
 */
function EvalResults({ stream }: { stream?: EventStream }) {
  const live = useLive<Answer>({
    key: '/api/events/query/verdicts.eval_scores',
    read: () => api.historyQuery('verdicts.eval_scores'),
    types: evalEventTypes,
    stream,
  })
  const answer = live.data

  return (
    <Panel title="Recorded eval and conformance results">
      <Freshness stale={live.stale} error={live.error} hasData={answer !== null} />
      {answer &&
        (answer.rows.length === 0 ? (
          <EmptyState
            what="No suite result has been recorded yet."
            why="A row appears here when a suite finishes and records its score — an eval run or a conformance check. Until one has run there is nothing to show, which is not the same as a suite having failed."
          />
        ) : (
          <div className="table-scroll">
            <table className="items">
              <thead>
                <tr>
                  {answer.columns.map((c) => (
                    <th key={c}>{c}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {answer.rows.map((r, i) => (
                  <tr key={i} data-eval-row={String(i)}>
                    {r.map((cell, j) => (
                      <td key={answer.columns[j]} data-col={answer.columns[j]}>
                        {cell === null || cell === undefined ? (
                          <Absent reason="not recorded on this path" />
                        ) : (
                          String(cell)
                        )}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
      {answer?.notes && answer.notes.length > 0 && (
        <ul className="text-sm text-muted-foreground">
          {answer.notes.map((n) => (
            <li key={n}>{n}</li>
          ))}
        </ul>
      )}
    </Panel>
  )
}

/**
 * The types that change THIS panel, and only those (§42 per-view declaration).
 *
 * It read the whole inbox set at first, which subscribed a table of suite
 * results to gate asks, drift findings and watchdog flags — frames that cannot
 * move a single row here. One type does move it, and it is the same one S14.5
 * says is the only thing that can clear a red: a recorded suite result.
 */
const evalEventTypes = ['eval.score_recorded'] as const
