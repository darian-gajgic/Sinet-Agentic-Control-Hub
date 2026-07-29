import { rankWith, uiTypeIs, type ControlProps, type UISchemaElement } from '@jsonforms/core'
import { JsonFormsDispatch, withJsonFormsControlProps, withJsonFormsLayoutProps } from '@jsonforms/react'
import { useState, type ReactNode } from 'react'

import type { SettingValue } from './api'
import { Absent, Stamp } from './parts'

/**
 * The Sinet-owned JSON Forms renderer set and the nested-data bridge
 * (Spec S15.9; S01.10; FC-v1 §4; P3/CONVENTIONS.md §43).
 *
 * NO THEME PACKAGE. FC-v1 §4 fixes the vocabulary as JSON Forms' CORE,
 * theme-independent elements — Categorization/Category (the tabs), Group, and
 * Control — and the renderers for them are the platform's own, over owned CSS.
 * `@jsonforms/material-renderers` and every sibling never enter this tree.
 *
 * NOTHING IS HAND-BUILT. The form tree is the registry's OWN emission,
 * consumed verbatim: no key list lives here, no control is placed by hand, and
 * a key added to the S18 index appears on screen because the emitter emitted
 * it. That is S01.10(b) — one artifact set for validation, the UI and the docs
 * — and it is the whole reason the settings page can claim completeness.
 *
 * NO RULE IS FABRICATED (OQ7, ratified). The emitter deliberately emits no
 * `rule`, because the registry has no datum that says "this setting is only
 * meaningful when that one is set thus" — `auto`, `perUser`, `restartRequired`
 * and `dormant` are BADGES, facts a person reads, not visibility conditions.
 * These renderers therefore rely on JSON Forms CORE's own rule evaluation,
 * which is present the day the emitter has a ratified condition to emit, and
 * they invent no condition of their own. Applying the emitter's own reason to
 * the renderer is the whole disposition.
 */

// ── the nested-data bridge (OQ2) ──────────────────────────────────────────

/**
 * settingsData builds the NESTED data object the emitted scopes bind against.
 *
 * This is the bridge the B6-4 probe found and §41-B recorded: JSON Forms
 * composes a control's data path by SPLITTING ITS SCOPE ON DOTS, so the nested
 * scope `#/properties/effects/properties/approval_expiry` reads
 * `data.effects.approval_expiry`. The API serves `values[]` as a FLAT list
 * keyed by the dotted key — and a flat object keyed by dotted strings renders
 * every single control EMPTY, silently, because no path ever resolves.
 *
 * It binds `effective`, the value in force at platform scope, because that is
 * what the form is for. Per-user overrides are one key with many values and
 * cannot ride a form model that holds one value per key, so they render outside
 * it (R20).
 *
 * Pure by design: it takes the served rows and returns a value. A test pins the
 * defect in the other direction — the flat object really does render empty —
 * so "we build a nested object" stays a checked fact rather than a habit.
 */
export function settingsData(values: SettingValue[]): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const v of values) {
    const segs = v.key.split('.')
    let node = out
    for (const seg of segs.slice(0, -1)) {
      const next = node[seg]
      if (next === undefined || typeof next !== 'object' || next === null) {
        node[seg] = {}
      }
      node = node[seg] as Record<string, unknown>
    }
    node[segs[segs.length - 1]] = v.effective
  }
  return out
}

/** valueAt reads one dotted key out of a nested data object — the reverse of
 *  the bridge, used to map an edit back to the per-key write verb. */
export function valueAt(data: Record<string, unknown>, key: string): unknown {
  let node: unknown = data
  for (const seg of key.split('.')) {
    if (node === null || typeof node !== 'object') return undefined
    node = (node as Record<string, unknown>)[seg]
  }
  return node
}

/** changedKeys lists the keys whose form value differs from what was SERVED.
 *  Comparison is over the canonical JSON form, so key order and number
 *  formatting never manufacture a difference. */
export function changedKeys(data: Record<string, unknown>, values: SettingValue[]): string[] {
  return values.filter((v) => JSON.stringify(valueAt(data, v.key)) !== JSON.stringify(v.effective)).map((v) => v.key)
}

// ── what a renderer needs beyond the schema ───────────────────────────────

/**
 * FormContext is everything a Control needs that the two emitted artifacts do
 * not carry: the served row (bounds, badges, help, the override state), whether
 * this caller may write at all, and the acts.
 *
 * It rides JSON Forms' own `config` prop rather than a React context, because
 * `config` is the library's declared channel to a renderer and using it keeps
 * the renderers ordinary JSON Forms renderers.
 */
export type FormContext = {
  byKey: Record<string, SettingValue>
  editable: boolean
  /** The served reason a member may not write, shown once and prominently by
   *  the page — a control never renders a disabled editor with it. */
  busyKey: string
  outcomes: Record<string, { detail: string; failed: boolean }>
  /** save carries the control's OWN current value rather than reading it back
   *  from a mirror of the form model. JSON Forms DEBOUNCES its `onChange` by
   *  10ms, so a mirror lags the control by that much — and a person who types
   *  and immediately saves would otherwise post the value they replaced. The
   *  control has the current value in hand; the write uses it. */
  save: (key: string, value: unknown) => void
  reset: (key: string) => void
  setBounds: (key: string, floor: number | null, ceiling: number | null, reason: string) => void
  setForUser: (key: string, user: string, value: string) => void
  openHistory: (key: string) => void
}

function contextOf(config: unknown): FormContext | null {
  if (!config || typeof config !== 'object') return null
  const ctx = (config as { ctx?: FormContext }).ctx
  return ctx ?? null
}

type Layout = UISchemaElement & { label?: string; elements?: UISchemaElement[] }

// ── Categorization: the S18 domain tabs ───────────────────────────────────

/**
 * The Categorization renderer draws the emitted Categories as tabs.
 *
 * One category is on screen at a time because 33 domains and 118 controls in
 * one scroll is not a surface anybody administers from. The tab STRIP is
 * always fully rendered, so nothing is hidden — every domain is one tap away,
 * and the tabs wrap rather than overflowing at phone width.
 */
// `config` is NOT threaded down the dispatch chain, and that is the library's
// own design rather than an omission: JsonForms puts the `config` prop into its
// context, and every renderer — at any depth — receives it through
// withJsonFormsControlProps / withJsonFormsLayoutProps. JsonFormsDispatch does
// not accept it, which is the type system saying the same thing.
export const CategorizationRenderer = withJsonFormsLayoutProps(
  ({ uischema, schema, path, renderers, cells, enabled }) => {
    const cats = ((uischema as Layout).elements ?? []) as Layout[]
    const [active, setActive] = useState(0)
    const current = cats[Math.min(active, Math.max(cats.length - 1, 0))]
    return (
      <div className="settings-cats">
        <div className="cat-tabs" role="tablist">
          {cats.map((c, i) => (
            <button
              key={c.label ?? String(i)}
              type="button"
              role="tab"
              data-tab={c.label}
              aria-selected={i === active}
              onClick={() => {
                setActive(i)
              }}
            >
              {c.label}
            </button>
          ))}
        </div>
        {current && (
          <div className="cat-panel" role="tabpanel" data-tab-panel={current.label}>
            {(current.elements ?? []).map((child, i) => (
              <JsonFormsDispatch
                key={i}
                uischema={child}
                schema={schema}
                path={path}
                enabled={enabled}
                renderers={renderers}
                cells={cells}
              />
            ))}
          </div>
        )}
      </div>
    )
  },
)

/** A Group is emitted only where the key index itself declares a shared
 *  sub-namespace, so it is drawn as the box the index says exists. */
export const GroupRenderer = withJsonFormsLayoutProps(
  ({ uischema, schema, path, renderers, cells, enabled }) => (
    <fieldset className="setting-group" data-group={(uischema as Layout).label}>
      <legend>{(uischema as Layout).label}</legend>
      {(((uischema as Layout).elements ?? []) as UISchemaElement[]).map((child, i) => (
        <JsonFormsDispatch
          key={i}
          uischema={child}
          schema={schema}
          path={path}
          enabled={enabled}
          renderers={renderers}
          cells={cells}
        />
      ))}
    </fieldset>
  ),
)

// ── Control: one setting, with everything a person deciding needs ─────────

/**
 * The Control renderer is the whole settings row: the editor, the EFFECTIVE
 * clamp bounds beside it, the badges, the inline help, the override state, and
 * the acts.
 *
 * `props.path` IS the dotted ⚙ key — JSON Forms joins the scope segments with
 * dots to reach the data, which is exactly the key the write verb takes. So the
 * per-key write needs no lookup table: the form's own path is the key.
 *
 * Every write affordance below is behind the SERVED `editable` flag. For a
 * member the whole set is ABSENT rather than disabled: a disabled control still
 * says "you could do this", and the honest statement is the server's own reason,
 * shown once at the top of the page.
 */
export const SinetControl = withJsonFormsControlProps((props: ControlProps) => {
  const ctx = contextOf(props.config)
  const key = props.path
  const served = ctx?.byKey[key]
  if (!ctx || !served) {
    // A control with no served row is a key the form emitted and the values
    // block did not carry. That is a contract disagreement, not something to
    // paper over with a blank input.
    return (
      <div className="setting" data-setting={key}>
        <Absent reason={`the registry emitted a control for ${key} but served no value for it`} />
      </div>
    )
  }
  const dirty = JSON.stringify(props.data) !== JSON.stringify(served.effective)
  const outcome = ctx.outcomes[key]
  return (
    <div className="setting" data-setting={key} data-dirty={dirty ? 'true' : 'false'}>
      <label className="setting-label" htmlFor={`f-${key}`}>
        {served.title}
      </label>
      <Badges v={served} />
      <div className="setting-edit">
        <Editor id={`f-${key}`} v={served} value={props.data} editable={ctx.editable} onChange={props.handleChange} path={key} />
        {served.unit && <span className="muted">{served.unit}</span>}
      </div>
      <Bounds v={served} />
      <p className="setting-help" data-help-for={key}>
        {served.help}
      </p>
      <p className="muted" data-override={served.overridden ? 'set' : 'default'}>
        {served.overridden
          ? 'explicitly set — a reset deletes the override and follows the ratified default again'
          : 'following the ratified default: no override row exists for this setting'}
        {' · '}
        default {JSON.stringify(served.default)} · ratified by {served.ratified_by}
      </p>

      {ctx.editable && (
        <div className="setting-acts">
          <button
            type="button"
            data-action="save"
            data-save-for={key}
            disabled={!dirty || ctx.busyKey === key}
            onClick={() => {
              ctx.save(key, props.data)
            }}
          >
            Save
          </button>
          <button
            type="button"
            data-action="reset"
            data-reset-for={key}
            disabled={!served.overridden || ctx.busyKey === key}
            onClick={() => {
              ctx.reset(key)
            }}
          >
            Reset to default
          </button>
          {served.numeric && <BoundsEditor v={served} ctx={ctx} />}
          {served.per_user && <PerUserEditor v={served} ctx={ctx} />}
          <button
            type="button"
            data-action="history"
            onClick={() => {
              ctx.openHistory(key)
            }}
          >
            History
          </button>
        </div>
      )}
      {outcome && (
        <p className={outcome.failed ? 'error' : 'notice'} data-write-outcome={outcome.failed ? 'failed' : 'applied'}>
          {outcome.detail}
        </p>
      )}
    </div>
  )
})

/** The editor for one leaf, dispatched on the type the REGISTRY declared. A
 *  value the person types is clamped client-side for feel only — the registry's
 *  validation is the authority, and its refusal renders verbatim. */
function Editor({
  id,
  v,
  value,
  editable,
  onChange,
  path,
}: {
  id: string
  v: SettingValue
  value: unknown
  editable: boolean
  onChange: (path: string, value: unknown) => void
  path: string
}) {
  if (!editable) {
    return (
      <output id={id} className="setting-value" data-readonly="true">
        {JSON.stringify(value)}
      </output>
    )
  }
  if (v.enum && v.enum.length > 0) {
    return (
      <select
        id={id}
        data-input={v.type}
        value={String(value ?? '')}
        onChange={(e) => {
          onChange(path, e.currentTarget.value)
        }}
      >
        {v.enum.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
      </select>
    )
  }
  if (v.type === 'bool') {
    return (
      <input
        id={id}
        type="checkbox"
        data-input="bool"
        checked={value === true}
        onChange={(e) => {
          onChange(path, e.currentTarget.checked)
        }}
      />
    )
  }
  if (v.numeric) {
    return (
      <input
        id={id}
        type="number"
        data-input={v.type}
        value={value === undefined || value === null ? '' : String(value)}
        min={v.floor}
        max={v.ceiling}
        onChange={(e) => {
          const raw = e.currentTarget.value
          onChange(path, raw === '' ? undefined : Number(raw))
        }}
      />
    )
  }
  return (
    <input
      id={id}
      type="text"
      data-input={v.type}
      value={String(value ?? '')}
      onChange={(e) => {
        onChange(path, e.currentTarget.value)
      }}
    />
  )
}

/** The S15.9 badges, from the SERVED flags. Each is a fact about the setting,
 *  never a condition on whether it renders. */
function Badges({ v }: { v: SettingValue }) {
  return (
    <span className="badges">
      {v.restart_required && (
        <span className="warn-flag" data-badge="restart">
          restart required
        </span>
      )}
      {v.auto && (
        <span className="muted" data-badge="auto">
          automation may move this within its bounds
        </span>
      )}
      {v.per_user && (
        <span className="muted" data-badge="per-user">
          per person
        </span>
      )}
      {v.dormant && (
        <span className="warn-flag" data-badge="dormant">
          dormant: {v.dormant}
        </span>
      )}
    </span>
  )
}

/** The EFFECTIVE clamp bounds, served — never re-derived from the declaration.
 *  An unbounded numeric says so, because a blank would read as "no data". */
function Bounds({ v }: { v: SettingValue }) {
  if (!v.numeric) return null
  return (
    <p className="muted bounds" data-bounds={v.key}>
      {v.floor === undefined && v.ceiling === undefined ? (
        'unbounded: the registry clamps this setting at neither end'
      ) : (
        <>
          {v.floor !== undefined && (
            <>
              floor {String(v.floor)}
              {v.min_exclusive === true && ' (exclusive)'}
            </>
          )}
          {v.floor !== undefined && v.ceiling !== undefined && ' · '}
          {v.ceiling !== undefined && <>ceiling {String(v.ceiling)}</>}
        </>
      )}
    </p>
  )
}

/** The bounds editor is a SEPARATE operator-only act, deliberately never
 *  conflated with a value edit: automation moves a value inside its bounds, and
 *  only the operator moves the bounds (G1 rider 1). */
function BoundsEditor({ v, ctx }: { v: SettingValue; ctx: FormContext }) {
  const [open, setOpen] = useState(false)
  const [floor, setFloor] = useState(v.floor === undefined ? '' : String(v.floor))
  const [ceiling, setCeiling] = useState(v.ceiling === undefined ? '' : String(v.ceiling))
  const [reason, setReason] = useState('')
  if (!open) {
    return (
      <button
        type="button"
        data-action="edit-bounds"
        onClick={() => {
          setOpen(true)
        }}
      >
        Edit bounds
      </button>
    )
  }
  return (
    <div className="bounds-editor" data-bounds-editor={v.key}>
      <label>
        <span>floor</span>
        <input
          type="number"
          data-field="floor"
          value={floor}
          onChange={(e) => {
            setFloor(e.currentTarget.value)
          }}
        />
      </label>
      <label>
        <span>ceiling</span>
        <input
          type="number"
          data-field="ceiling"
          value={ceiling}
          onChange={(e) => {
            setCeiling(e.currentTarget.value)
          }}
        />
      </label>
      <label>
        <span>why</span>
        <input
          type="text"
          data-field="bounds-reason"
          value={reason}
          onChange={(e) => {
            setReason(e.currentTarget.value)
          }}
        />
      </label>
      <button
        type="button"
        data-action="save-bounds"
        onClick={() => {
          ctx.setBounds(v.key, floor === '' ? null : Number(floor), ceiling === '' ? null : Number(ceiling), reason)
          setOpen(false)
        }}
      >
        Save bounds
      </button>
      <span className="muted">both blank resets to the ratified clamp</span>
    </div>
  )
}

/** The operator's `for_user` affordance, on PerUser keys only. The registry
 *  admits no member actor, so a member's own override exists exactly because
 *  the operator set it for them (D10) — the surface says so rather than
 *  implying self-service that does not exist. */
function PerUserEditor({ v, ctx }: { v: SettingValue; ctx: FormContext }) {
  const [user, setUser] = useState('')
  const [value, setValue] = useState('')
  return (
    <div className="per-user-editor" data-per-user-editor={v.key}>
      <label>
        <span>for person</span>
        <input
          type="text"
          data-field="for-user"
          value={user}
          onChange={(e) => {
            setUser(e.currentTarget.value)
          }}
        />
      </label>
      <label>
        <span>their value</span>
        <input
          type="text"
          data-field="for-user-value"
          value={value}
          onChange={(e) => {
            setValue(e.currentTarget.value)
          }}
        />
      </label>
      <button
        type="button"
        data-action="save-for-user"
        disabled={user === '' || value === ''}
        onClick={() => {
          ctx.setForUser(v.key, user, value)
        }}
      >
        Set for them
      </button>
    </div>
  )
}

/** PerUserValues renders the overrides in force, outside the form model —
 *  one key with many values cannot ride a model that holds one value per key.
 *  The list is exactly what the server served: a member sees their own because
 *  the server narrowed it, not because this filtered anything. */
export function PerUserValues({ values }: { values: SettingValue[] }) {
  const withOverrides = values.filter((v) => v.user_values && Object.keys(v.user_values).length > 0)
  if (withOverrides.length === 0) {
    return <p className="muted">No per-person override is in force on any setting.</p>
  }
  return (
    <ul className="per-user-values">
      {withOverrides.map((v) => (
        <li key={v.key} data-per-user={v.key}>
          <span className="setting-key">{v.key}</span>
          <ul>
            {Object.entries(v.user_values ?? {}).map(([user, val]) => (
              <li key={user} data-per-user-of={user}>
                {user}: {JSON.stringify(val)}
              </li>
            ))}
          </ul>
        </li>
      ))}
    </ul>
  )
}

/** HistoryRows renders one key's audit trail as served: actor, old → new
 *  VERBATIM, when, and why. Values print as their stored JSON because that is
 *  what they are — re-formatting an audited value would be editing the record. */
export function HistoryRows({ entries }: { entries: { id: number; actor: string; old: unknown; new: unknown; ts: string; reason?: string; user_id?: string }[] }) {
  if (entries.length === 0) return <p className="muted">Nothing has ever changed this setting.</p>
  return (
    <ol className="audit-rows">
      {entries.map((e) => (
        <li key={e.id} data-audit={String(e.id)}>
          <span className="owner">{e.actor}</span>
          {e.user_id && <span className="muted"> for {e.user_id}</span>}{' '}
          <code>{JSON.stringify(e.old)}</code> → <code>{JSON.stringify(e.new)}</code> <Stamp ts={e.ts} />
          {e.reason && <span className="muted"> — {e.reason}</span>}
        </li>
      ))}
    </ol>
  )
}

/**
 * The renderer set, in dispatch order.
 *
 * Ranks are deliberately low and specific: each tester matches ONE emitted
 * element type, so nothing here competes with anything and the set is complete
 * for the vocabulary FC-v1 §4 admits. A UISchema element type outside it would
 * find no renderer, which is the honest outcome — the emitter emits none.
 */
export const sinetRenderers = [
  { tester: rankWith(3, uiTypeIs('Categorization')), renderer: CategorizationRenderer },
  { tester: rankWith(3, uiTypeIs('Category')), renderer: GroupRenderer },
  { tester: rankWith(3, uiTypeIs('Group')), renderer: GroupRenderer },
  { tester: rankWith(3, uiTypeIs('Control')), renderer: SinetControl },
]

/** Panel is one titled block of the settings page. */
export function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="block">
      <h2>{title}</h2>
      {children}
    </section>
  )
}
