import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import type { Answer, SettingValue, SettingsHistory, SettingsView, StoredPricesView } from './api'
import { FakeSource, fixtures, oversightRoutes, scriptedFetch, type Scripted } from './doubles'
import { EventStream } from './events'
import { settingsData, valueAt } from './settingsForm'
import { click, flush, mount, settle, typeInto } from './testing'

/**
 * The settings tab (Spec S15.9; S01.10; S18; FC-v1 §4), driven against the
 * GOLDEN FIXTURES — the registry's REAL emitted schema and UISchema, its real
 * 118 values across 33 domains, and a real audit trail written by the real
 * write verbs.
 *
 * That matters more here than anywhere else on the frontend: the whole claim of
 * this surface is that it hand-builds nothing. A test against a three-key toy
 * schema would prove the renderers dispatch; only the real emission proves the
 * page renders the platform's actual configuration.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

async function openSettings(extra: Record<string, Scripted> = {}) {
  const log = scriptedFetch({ ...oversightRoutes(), ...extra })
  window.history.replaceState(null, '', '/settings')
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}

const served = () => fixtures.settings() as unknown as SettingsView
const key = (v: SettingsView, k: string): SettingValue => {
  const found = v.values.find((x) => x.key === k)
  if (!found) throw new Error(`the fixture serves no setting ${k}`)
  return found
}

/** tabs walks every emitted domain tab and collects what each one renders, so
 *  completeness is measured over the whole form rather than over whichever tab
 *  happened to be open. */
function everySetting(view: { container: HTMLElement }): string[] {
  const seen: string[] = []
  const strip = [...view.container.querySelectorAll('[data-tab]')]
  for (const tab of strip) {
    click(tab)
    for (const n of view.container.querySelectorAll('[data-setting]')) {
      const k = n.getAttribute('data-setting')
      if (k) seen.push(k)
    }
  }
  return seen
}

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── R14: the nested-data bridge ───────────────────────────────────────────

test('the bridge splits dotted keys into the nested object the emitted scopes bind against', () => {
  const view = served()
  const data = settingsData(view.values)
  // The shape JSON Forms needs: a control whose scope is
  // #/properties/freshness/properties/max_age reads data.freshness.max_age.
  expect((data.freshness as Record<string, unknown>).max_age).toBe(key(view, 'freshness.max_age').effective)
  // Every served key resolves through the same path walk the form uses.
  for (const v of view.values) {
    expect(valueAt(data, v.key), `${v.key} does not resolve in the nested data`).toEqual(v.effective)
  }
  // No dotted key survives as a literal property — that flat shape is exactly
  // the defect this bridge exists to avoid.
  expect(Object.keys(data).some((k) => k.includes('.'))).toBe(false)
})

test('THE REGRESSION PROBE: the flat object really does render every control empty', async () => {
  // The defect §41-B recorded, pinned in the direction that proves it: a flat
  // object keyed by dotted strings resolves NO path, so a form built on it
  // shows nothing. If this ever stops being true the bridge could be deleted —
  // and until then, the bridge is load-bearing rather than decorative.
  const view = served()
  const flat: Record<string, unknown> = {}
  for (const v of view.values) flat[v.key] = v.effective
  for (const v of view.values.slice(0, 20)) {
    expect(valueAt(flat, v.key), `${v.key} resolved in the FLAT object — the probe is vacuous`).toBeUndefined()
  }
})

test('every control shows its served value, read from the REAL emitted schema', async () => {
  const { view } = await openSettings()
  const v = served()
  const target = key(v, 'freshness.max_age')

  // Find the tab that holds it, then read the input.
  const tab = [...view.container.querySelectorAll('[data-tab]')].find((t) =>
    (t.getAttribute('data-tab') ?? '').startsWith(target.domain),
  )
  expect(tab, `no tab for the ${target.domain} domain`).toBeTruthy()
  click(tab)
  const input = view.container.querySelector('[data-setting="freshness.max_age"] input') as HTMLInputElement
  expect(input, 'the control rendered no editor').not.toBeNull()
  expect(input.value, 'the control rendered empty — the flat-object defect').toBe(String(target.effective))
})

// ── R13/R15: generated, complete, no theme package ────────────────────────

test('the form is the registry emission: one tab per domain, one control per key, nothing hand-built', async () => {
  const { view } = await openSettings()
  const v = served()

  const tabs = [...view.container.querySelectorAll('[data-tab]')].map((n) => n.getAttribute('data-tab'))
  expect(tabs, 'the tab strip is not one tab per emitted Category').toHaveLength(v.domains.length)
  expect(tabs.length).toBe(33)

  // Completeness, derived from the SERVED data in both directions.
  const rendered = everySetting(view)
  const servedKeys = v.values.map((x) => x.key).sort()
  expect([...new Set(rendered)].sort(), 'a served setting never renders').toEqual(servedKeys)
  expect(servedKeys).toHaveLength(118)
  for (const k of rendered) {
    expect(servedKeys, `${k} renders but was never served`).toContain(k)
  }
})

test('groups render exactly where the key index declares a shared sub-namespace', async () => {
  const { view } = await openSettings()
  // The emitter draws a Group only where two or more keys share a parent path.
  // Walking every tab finds them; a Group of one would be a box around nothing.
  let groups = 0
  for (const tab of [...view.container.querySelectorAll('[data-tab]')]) {
    click(tab)
    groups += view.container.querySelectorAll('[data-group]').length
  }
  expect(groups, 'the emitted Groups never render').toBeGreaterThan(0)
})

// ── R16/R17/R19: bounds, badges, help, override state ─────────────────────

test('each numeric setting shows its EFFECTIVE bounds, its badges and its inline help', async () => {
  const { view } = await openSettings()
  const v = served()
  const target = key(v, 'freshness.max_age')
  // The fixture drove a real operator bounds edit, so the effective bounds
  // differ from the ratified clamp — which is the only way this assertion is
  // about served data rather than about a default.
  expect(target.floor, 'the fixture never narrowed the bounds').toBe(7200)

  click([...view.container.querySelectorAll('[data-tab]')].find((t) => (t.getAttribute('data-tab') ?? '').startsWith('freshness')))
  const node = view.container.querySelector('[data-setting="freshness.max_age"]')!
  expect(node.querySelector('[data-bounds]')?.textContent).toContain(String(target.floor))
  expect(node.querySelector('[data-bounds]')?.textContent).toContain(String(target.ceiling))
  // The help is the REGISTRY's sentence, rendered where the person decides.
  expect(node.querySelector('[data-help-for="freshness.max_age"]')?.textContent).toBe(target.help)
  // And the override state is distinguishable from following the default.
  expect(node.querySelector('[data-override]')?.getAttribute('data-override')).toBe('set')
})

test('restart-required and dormant badge from the served flags, and only where they are set', async () => {
  const { view } = await openSettings()
  const v = served()
  const restart = v.values.find((x) => x.restart_required)
  expect(restart, 'no served setting is restart-required, so this asserts nothing').toBeTruthy()

  click(
    [...view.container.querySelectorAll('[data-tab]')].find((t) =>
      (t.getAttribute('data-tab') ?? '').startsWith(restart?.domain ?? ''),
    ),
  )
  const node = view.container.querySelector(`[data-setting="${CSS.escape(restart?.key ?? '')}"]`)!
  expect(node.querySelector('[data-badge="restart"]'), 'a restart-required setting is not badged').not.toBeNull()

  // The other direction: a setting without the flag carries no badge.
  const plain = v.values.find((x) => !x.restart_required && x.domain === restart?.domain)
  if (plain) {
    const other = view.container.querySelector(`[data-setting="${CSS.escape(plain.key)}"]`)
    expect(other?.querySelector('[data-badge="restart"]')).toBeNull()
  }
})

// ── R14/R17: an edit maps back to the per-key write ───────────────────────

async function openFreshness(extra: Record<string, Scripted> = {}) {
  const out = await openSettings(extra)
  click(
    [...out.view.container.querySelectorAll('[data-tab]')].find((t) =>
      (t.getAttribute('data-tab') ?? '').startsWith('freshness'),
    ),
  )
  return out
}

test('an edit posts to THAT key, on an explicit save, and never as you type', async () => {
  const { view, log } = await openFreshness({
    'POST /api/settings/freshness.max_age': {
      body: {
        key: 'freshness.max_age',
        reset: false,
        value: key(served(), 'freshness.max_age'),
        detail: 'set: the new value is in force for the next read',
      },
    },
  })
  const input = view.container.querySelector('[data-setting="freshness.max_age"] input') as HTMLInputElement
  typeInto(input, '9000')
  // The unsaved-changes line reads a mirror of the form model, and JSON Forms
  // debounces the change that feeds it by 10ms.
  await settle()

  // Typing wrote NOTHING: a settings write is an audited act.
  expect(log.calls.filter((c) => c.method === 'POST'), 'a keystroke fired a write').toHaveLength(0)
  // …and the page says there is an unsaved change rather than hiding it.
  expect(view.container.querySelector('[data-dirty-keys]')?.textContent).toContain('freshness.max_age')

  click(view.container.querySelector('[data-save-for="freshness.max_age"]'))
  await flush()
  const posted = log.calls.find((c) => c.path === '/api/settings/freshness.max_age')
  expect(posted, 'the save posted nothing').toBeTruthy()
  expect(posted?.body).toEqual({ value: 9000 })
  expect(view.container.querySelector('[data-write-outcome="applied"]')?.textContent).toContain('in force')
})

test('reset-to-default is the null write, and is offered only where an override exists', async () => {
  const { view, log } = await openFreshness({
    'POST /api/settings/freshness.max_age': {
      body: {
        key: 'freshness.max_age',
        reset: true,
        value: key(served(), 'freshness.max_age'),
        detail: 'reset to the ratified default: the override row is deleted',
      },
    },
  })
  const reset = view.container.querySelector('[data-reset-for="freshness.max_age"]') as HTMLButtonElement
  expect(reset.disabled, 'a setting with an override cannot be reset').toBe(false)
  click(reset)
  await flush()
  expect(log.calls.find((c) => c.path === '/api/settings/freshness.max_age')?.body).toEqual({ value: null })
  expect(view.container.textContent).toContain('the override row is deleted')

  // A setting FOLLOWING the default has nothing to reset, and says so through
  // the disabled control rather than by pretending the act would do something.
  const following = served().values.find((v) => !v.overridden && v.domain === 'freshness')
  if (following) {
    const node = view.container.querySelector(`[data-reset-for="${CSS.escape(following.key)}"]`) as HTMLButtonElement
    expect(node.disabled).toBe(true)
  }
})

test("a registry refusal renders VERBATIM beside the setting that earned it", async () => {
  const { view, log } = await openFreshness({
    'POST /api/settings/freshness.max_age': {
      status: 400,
      body: {
        error: 'bad_request',
        detail: 'settings: freshness.max_age = 60 is below its floor of 7200 (S18.2 clamp)',
      },
    },
  })
  typeInto(view.container.querySelector('[data-setting="freshness.max_age"] input') as HTMLInputElement, '60')
  click(view.container.querySelector('[data-save-for="freshness.max_age"]'))
  await flush()
  const failed = view.container.querySelector('[data-write-outcome="failed"]')?.textContent ?? ''
  expect(failed, 'the refusal was paraphrased').toContain('is below its floor of 7200')
  expect(log.calls.some((c) => c.path === '/api/settings/freshness.max_age')).toBe(true)
})

// ── R16: the bounds edit is a SEPARATE, operator-only act ─────────────────

test('bounds are edited by their own verb, never by the value editor', async () => {
  const { view, log } = await openFreshness({
    'POST /api/settings/freshness.max_age/bounds': {
      body: {
        key: 'freshness.max_age',
        reset: false,
        value: key(served(), 'freshness.max_age'),
        detail: 'bounds recorded: automation may move this value only inside them',
      },
    },
  })
  click(view.container.querySelector('[data-setting="freshness.max_age"] [data-action="edit-bounds"]'))
  const editor = view.container.querySelector('[data-bounds-editor="freshness.max_age"]')!
  typeInto(editor.querySelector('[data-field="floor"]') as HTMLInputElement, '3600')
  typeInto(
    view.container.querySelector('[data-bounds-editor="freshness.max_age"] [data-field="bounds-reason"]') as HTMLInputElement,
    'widened again',
  )
  click(view.container.querySelector('[data-bounds-editor="freshness.max_age"] [data-action="save-bounds"]'))
  await flush()

  const posted = log.calls.find((c) => c.path === '/api/settings/freshness.max_age/bounds')
  expect(posted, 'the bounds edit went to the value verb').toBeTruthy()
  expect(posted?.body).toMatchObject({ floor: 3600, reason: 'widened again' })
  // The VALUE verb was never called by a bounds edit.
  expect(log.calls.filter((c) => c.path === '/api/settings/freshness.max_age')).toHaveLength(0)
  expect(view.container.textContent).toContain('only inside them')
})

// ── R15/OQ3: the member's read-only view ──────────────────────────────────

test('a member gets the whole VIEW and no write affordance at all, with the served reason once', async () => {
  const member = fixtures.settingsMember() as unknown as SettingsView
  expect(member.editable, 'the member fixture is editable — the test would assert nothing').toBe(false)
  // The price table states its OWN authority, computed for the same caller —
  // so a member's page is two served read-only bodies, not one flag reused.
  const { view } = await openSettings({
    'GET /api/settings': { body: member },
    'GET /api/settings/prices': { body: fixtures.pricesMember() },
  })

  // The reason is stated ONCE, prominently, not repeated beside every control.
  const notice = view.container.querySelector('[data-editable="false"]')
  expect(notice?.textContent).toBe(member.editable_reason)
  expect(view.container.querySelectorAll('[data-editable]')).toHaveLength(1)

  // Every write affordance is ABSENT, not disabled: a disabled control still
  // says "you could do this".
  const withAny = everySetting(view)
  expect(withAny.length).toBeGreaterThan(0)
  for (const sel of ['[data-action="save"]', '[data-action="reset"]', '[data-action="edit-bounds"]', '[data-action="add-price-row"]']) {
    expect(view.container.querySelectorAll(sel), `a member was offered ${sel}`).toHaveLength(0)
  }
  // …but HISTORY is a read, and the ratified OQ3 gives a member the FULL view:
  // values, bounds, badges, help AND history. The server gates that route at
  // nothing beyond the session, so hiding it would be this surface inventing an
  // authority the platform does not have.
  expect(
    view.container.querySelectorAll('[data-action="history"]').length,
    'a member cannot see how a setting came to have its value',
  ).toBeGreaterThan(0)
  // The values themselves still read.
  click([...view.container.querySelectorAll('[data-tab]')].find((t) => (t.getAttribute('data-tab') ?? '').startsWith('freshness')))
  expect(view.container.querySelector('[data-setting="freshness.max_age"] [data-readonly="true"]')?.textContent).toBe(
    String(key(member, 'freshness.max_age').effective),
  )
})

test('a member can open a setting\'s audit history — the read is not behind the write flag', async () => {
  const member = fixtures.settingsMember() as unknown as SettingsView
  const { view, log } = await openSettings({
    'GET /api/settings': { body: member },
    'GET /api/settings/prices': { body: fixtures.pricesMember() },
  })
  click(
    [...view.container.querySelectorAll('[data-tab]')].find((t) =>
      (t.getAttribute('data-tab') ?? '').startsWith('freshness'),
    ),
  )
  click(view.container.querySelector('[data-setting="freshness.max_age"] [data-action="history"]'))
  await flush()
  expect(log.calls.some((c) => c.path === '/api/settings/freshness.max_age/history')).toBe(true)
  expect(view.container.querySelector('[data-history-for="freshness.max_age"]')).not.toBeNull()
})

// ── R18: the per-key audit trail ──────────────────────────────────────────

test('a key opens its audit rows: actor, old → new verbatim, when and why', async () => {
  const { view, log } = await openFreshness()
  click(view.container.querySelector('[data-setting="freshness.max_age"] [data-action="history"]'))
  await flush()
  expect(log.calls.some((c) => c.path === '/api/settings/freshness.max_age/history')).toBe(true)

  const panel = view.container.querySelector('[data-history-for="freshness.max_age"]')!
  const entries = (fixtures.settingsHistory() as unknown as SettingsHistory).entries
  expect(entries.length, 'the fixture has no audit rows').toBeGreaterThan(1)
  expect(panel.querySelectorAll('[data-audit]')).toHaveLength(entries.length)

  const first = panel.querySelector('[data-audit]')!
  expect(first.textContent).toContain(entries[0].actor)
  // Values print as their STORED JSON: reformatting an audited value would be
  // editing the record.
  expect(first.textContent).toContain(JSON.stringify(entries[0].old))
  expect(first.textContent).toContain(JSON.stringify(entries[0].new))
  expect(first.textContent).toContain(entries[0].reason ?? '')
})

// ── R20: per-user overrides ───────────────────────────────────────────────

test('per-person overrides render outside the form, as served', async () => {
  const { view } = await openSettings()
  const v = served()
  const withOverride = v.values.find((x) => x.user_values && Object.keys(x.user_values).length > 0)
  expect(withOverride, 'the fixture drove no per-user override').toBeTruthy()

  const node = view.container.querySelector(`[data-per-user="${CSS.escape(withOverride?.key ?? '')}"]`)
  expect(node, 'a per-person override in force is invisible').not.toBeNull()
  const [user, value] = Object.entries(withOverride?.user_values ?? {})[0]
  expect(node?.querySelector(`[data-per-user-of="${CSS.escape(user)}"]`)?.textContent).toContain(JSON.stringify(value))
})

test("the operator's for_user write targets one person on a per-user key", async () => {
  const v = served()
  const perUser = v.values.find((x) => x.per_user)!
  const { view, log } = await openSettings({
    [`POST /api/settings/${perUser.key}`]: {
      body: { key: perUser.key, reset: false, value: perUser, for_user: 'bob', detail: 'set for bob' },
    },
  })
  click([...view.container.querySelectorAll('[data-tab]')].find((t) => (t.getAttribute('data-tab') ?? '').startsWith(perUser.domain)))
  const editor = view.container.querySelector(`[data-per-user-editor="${CSS.escape(perUser.key)}"]`)!
  typeInto(editor.querySelector('[data-field="for-user"]') as HTMLInputElement, 'bob')
  typeInto(
    view.container.querySelector(`[data-per-user-editor="${CSS.escape(perUser.key)}"] [data-field="for-user-value"]`) as HTMLInputElement,
    '0.75',
  )
  click(view.container.querySelector(`[data-per-user-editor="${CSS.escape(perUser.key)}"] [data-action="save-for-user"]`))
  await flush()
  expect(log.calls.find((c) => c.path === `/api/settings/${perUser.key}`)?.body).toEqual({
    value: 0.75,
    for_user: 'bob',
  })
})

// ── R21: the price table over the append-only store ───────────────────────

test('price rows render verbatim, and the table offers NO edit and NO delete', async () => {
  const { view } = await openSettings()
  const prices = fixtures.prices() as unknown as StoredPricesView
  const row = (prices.rows ?? [])[0]
  const node = view.container.querySelector(`[data-price-row="${String(row.id)}"]`)!
  expect(node.textContent).toContain(row.model)
  expect(node.textContent).toContain(row.lane)
  expect(node.textContent).toContain(row.source)
  expect(node.querySelector('[data-unit="input_usd"]')?.textContent).toContain(String(row.unit_prices.input_usd))

  // Immutability rendered as the property it is.
  expect(view.container.textContent).toContain('there is no edit or delete')
  for (const sel of ['[data-action="edit-price-row"]', '[data-action="delete-price-row"]']) {
    expect(view.container.querySelectorAll(sel), `the table offers ${sel}, which no verb backs`).toHaveLength(0)
  }
  // The stated decisions render as the decisions they are.
  expect(view.container.querySelectorAll('.deferred li')).toHaveLength(prices.deferred.length)
})

test('an EMPTY price table renders its posture prominently — UNPRICED is a stance, not a fault', async () => {
  const prices = fixtures.prices() as unknown as StoredPricesView
  const empty = {
    ...prices,
    rows: [],
    posture:
      'this table is EMPTY, which is the shipped posture, not a fault: every usage row prices UNPRICED and says so on the receipt',
  }
  const { view } = await openSettings({ 'GET /api/settings/prices': { body: empty } })
  const node = view.container.querySelector('[data-posture="empty-table"]')
  expect(node, 'an empty table said nothing').not.toBeNull()
  expect(node?.textContent).toBe(empty.posture)
  expect(view.container.querySelectorAll('[data-price-row]')).toHaveLength(0)
})

test('the form offers EVERY unit the composed cost multiplies — pinned to the stored shape', async () => {
  // A cost folds all five units unconditionally, so a form offering three would
  // leave cache-creation and server-tool usage priced at a traceless $0 inside
  // a cost the receipt calls priced. The list is pinned to the SERVED row's own
  // keys, so it cannot drift from the shape the store accepts (OQ6).
  const { view } = await openSettings()
  const stored = (fixtures.prices() as unknown as StoredPricesView).rows?.[0]
  const offered = [...view.container.querySelectorAll('.unit-prices [data-price-field]')].map((n) =>
    n.getAttribute('data-price-field'),
  )
  expect(offered.slice().sort(), 'the form does not offer exactly the stored unit set').toEqual(
    Object.keys(stored?.unit_prices ?? {}).sort(),
  )
  expect(offered).toHaveLength(5)
})

test('a unit left blank and not declared free cannot be appended', async () => {
  const { view, log } = await openSettings()
  const fill = (field: string, value: string) => {
    typeInto(view.container.querySelector(`.add-price-row [data-price-field="${field}"]`) as HTMLInputElement, value)
  }
  const button = () => view.container.querySelector('[data-action="add-price-row"]') as HTMLButtonElement

  fill('model', 'claude-haiku-5')
  fill('lane', 'anthropic')
  expect(button().disabled, 'a row with no unit answers could be appended').toBe(true)

  // Answering four still leaves the fifth composing a silent zero.
  for (const f of ['input_usd', 'output_usd', 'cache_read_usd', 'cache_creation_usd']) fill(f, '0.000001')
  expect(button().disabled, 'a row with one unanswered unit could be appended').toBe(true)
  expect(view.container.querySelector('[data-unit-unanswered="server_tool_usd"]')).not.toBeNull()
  expect(view.container.querySelector('[data-append-summary]')?.getAttribute('data-append-summary')).toBe('incomplete')

  // Zero, negative and mistyped are all still refused as PRICES.
  for (const bad of ['0', '-1', 'not a number']) {
    fill('server_tool_usd', bad)
    expect(button().disabled, `a ${bad} unit price could be appended`).toBe(true)
  }
  expect(log.calls.filter((c) => c.method === 'POST'), 'typing appended a row').toHaveLength(0)
})

test('"not charged on this lane" is an explicit ACT that composes the omission deliberately', async () => {
  const { view, log } = await openSettings({
    'POST /api/settings/prices': {
      body: {
        row: (fixtures.prices() as unknown as StoredPricesView).rows?.[0],
        version: 'prices/2026-07-29#2',
        detail: 'row appended and in force: the table re-composed in place',
      },
    },
  })
  const fill = (field: string, value: string) => {
    typeInto(view.container.querySelector(`.add-price-row [data-price-field="${field}"]`) as HTMLInputElement, value)
  }
  fill('model', 'claude-haiku-5')
  fill('lane', 'anthropic')
  fill('input_usd', '0.000001')
  fill('output_usd', '0.000005')
  fill('cache_read_usd', '0.0000001')
  fill('cache_creation_usd', '0.00000125')
  // The fifth is not priced — it is DECLARED not charged, by the operator.
  click(view.container.querySelector('[data-price-free="server_tool_usd"]'))

  // The summary says exactly what will be composed; nothing is silent.
  const summary = view.container.querySelector('[data-append-summary]')
  expect(summary?.getAttribute('data-append-summary')).toBe('ready')
  expect(summary?.textContent).toContain('cache_creation_usd')
  expect(summary?.textContent).toContain('declare server_tool_usd not charged on this lane')

  click(view.container.querySelector('[data-action="add-price-row"]'))
  await flush()
  const posted = log.calls.find((c) => c.method === 'POST' && c.path === '/api/settings/prices')
  // The declared-free unit is OMITTED, not sent as a zero — and the two units
  // the first cut never offered carry real values through.
  expect(posted?.body).toMatchObject({
    row: {
      unit_prices: {
        input_usd: 0.000001,
        output_usd: 0.000005,
        cache_read_usd: 0.0000001,
        cache_creation_usd: 0.00000125,
      },
    },
  })
  const sent = (posted?.body as { row: { unit_prices: Record<string, number> } }).row.unit_prices
  expect(Object.keys(sent), 'a declared-free unit was sent as a zero').not.toContain('server_tool_usd')
  expect(view.container.querySelector('[data-price-outcome="added"]')?.textContent).toContain('prices/2026-07-29#2')
})

test('declaring EVERY unit free is refused by the store, in the store&apos;s own words', async () => {
  // The form permits the act — it is a thing an operator can coherently mean —
  // and the STORE refuses it, because a row with no unit price at all prices
  // every call at $0. The authority stays where OQ6 put it.
  const { view, log } = await openSettings({
    'POST /api/settings/prices': {
      status: 400,
      body: {
        error: 'bad_request',
        detail:
          'metering: price row is not admissible: a row declares at least one unit price — a row with none prices every call at $0, which is the silent zero the UNPRICED posture exists to bar (S10.1; P-T08-1)',
      },
    },
  })
  typeInto(view.container.querySelector('.add-price-row [data-price-field="model"]') as HTMLInputElement, 'flat-lane')
  typeInto(view.container.querySelector('.add-price-row [data-price-field="lane"]') as HTMLInputElement, 'coding-plan')
  for (const [field] of [
    ['input_usd'],
    ['output_usd'],
    ['cache_read_usd'],
    ['cache_creation_usd'],
    ['server_tool_usd'],
  ] as [string][]) {
    click(view.container.querySelector(`[data-price-free="${field}"]`))
  }
  const button = view.container.querySelector('[data-action="add-price-row"]') as HTMLButtonElement
  expect(button.disabled, 'an all-declared-free row is blocked by the FORM instead of the store').toBe(false)
  click(button)
  await flush()
  const posted = log.calls.find((c) => c.method === 'POST' && c.path === '/api/settings/prices')
  expect((posted?.body as { row: { unit_prices: Record<string, number> } }).row.unit_prices).toEqual({})
  expect(view.container.querySelector('[data-price-outcome="failed"]')?.textContent).toContain(
    'a row with none prices every call at $0',
  )
})

test("the store's refusal on a field the FORM does not guard renders in its own words", async () => {
  // The form guards the unit answers, because a blank there composes a silent
  // $0. It guards nothing else, deliberately: the store owns what a row is, and
  // a second copy of its rules here would be a second definition of the
  // platform's money. A row that parses but is missing its source comes back
  // refused, in the store's own sentence.
  const { view, log } = await openSettings({
    'POST /api/settings/prices': {
      status: 400,
      body: {
        error: 'bad_request',
        detail:
          'metering: price row is not admissible: a row names where its price came from; D5 quotes the provider actually serving the lane, never an aggregator (S10.3)',
      },
    },
  })
  const fill = (field: string, value: string) => {
    typeInto(view.container.querySelector(`.add-price-row [data-price-field="${field}"]`) as HTMLInputElement, value)
  }
  fill('model', 'claude-haiku-5')
  fill('lane', 'anthropic')
  for (const [f] of [['input_usd'], ['output_usd'], ['cache_read_usd'], ['cache_creation_usd'], ['server_tool_usd']] as [string][]) {
    fill(f, '0.000001')
  }
  click(view.container.querySelector('[data-action="add-price-row"]'))
  await flush()
  expect(log.calls.some((c) => c.method === 'POST' && c.path === '/api/settings/prices')).toBe(true)
  expect(view.container.querySelector('[data-price-outcome="failed"]')?.textContent).toContain('never an aggregator')
})

test('recorded suite results render as served, and a path that registers no floor says so', async () => {
  const { view, log } = await openSettings()
  expect(log.calls.some((c) => c.path === '/api/events/query/verdicts.eval_scores')).toBe(true)

  const answer = fixtures.evalScores() as unknown as Answer
  expect(answer.rows.length, 'the fixture recorded no eval results').toBe(2)
  const rows = [...view.container.querySelectorAll('[data-eval-row]')]
  expect(rows).toHaveLength(answer.rows.length)

  // The columns are the query's own, and the values render as data — nothing
  // here judges a score.
  const first = rows[0]
  expect(first.querySelector('[data-col="suite_id"]')?.textContent).toBe('prompt-sweep')
  expect(first.querySelector('[data-col="result"]')?.textContent).toBe('red')
  // The sweep path registers no floor: an absence, not a zero.
  expect(first.querySelector('[data-col="floor"]')?.textContent).toContain('not recorded on this path')
  // …and the runbook path's row carries the registered number.
  expect(rows[1].querySelector('[data-col="floor"]')?.textContent).toBe('0.82')
})

// ── R15: the registered block is read-only ────────────────────────────────

test('the registered block renders with each value carrying its own read-only marker', async () => {
  const v = served()
  const { view } = await openSettings()
  if (v.registered && v.registered.length > 0) {
    const node = view.container.querySelector(`[data-registered="${CSS.escape(v.registered[0].name)}"]`)
    expect(node?.textContent).toContain(v.registered[0].marker)
    expect(view.container.querySelectorAll('.registered [data-registered]')).toHaveLength(v.registered.length)
  } else {
    // The honest absence is what a process with no practice composed serves.
    expect(view.container.textContent).toContain(v.registered_absent ?? '')
  }
})

// ── D8 pass II: the surface header and the A-2 read-only explanation ───────

test('the settings tab says what it is, and promises no editing it cannot deliver', async () => {
  const { view } = await openSettings()
  const what = view.container.querySelector('[data-surface-what]')?.textContent ?? ''

  expect(what, 'the line does not say what is on the page').toContain('Every setting the platform has')
  for (const fact of ['bounds', 'badges', 'help', 'history']) {
    expect(what.toLowerCase(), `the line drops what a row carries (${fact})`).toContain(fact)
  }
  // The page's own central claim, which is what makes completeness checkable.
  expect(what).toContain('generated from the registry')
  expect(what, 'the line does not say why a setting is here').toContain('because the registry declares it')

  // AUTHORITY IS SERVED, so the header may not promise editing to anybody: who
  // may write is the server's answer and it is stated once, in its own words.
  for (const claim of ['you can change', 'you can edit', 'edit any', 'editable', 'save']) {
    expect(what.toLowerCase(), `the header promises editing (${claim}) the server may refuse`).not.toContain(claim)
  }
})

/** The three authority sentences the server serves, transcribed byte-exact from
 *  internal/api/settings.go — :455 (dev), :458 (member), :460 (operator). The
 *  client can tell the three postures apart ONLY through this string, which is
 *  why it is the whole content of the one `[data-editable]` element. */
const devReason =
  'the dev-posture identity reads but never administers (§9): settings writes need a real operator session'

/** devPosture is a hand-scripted body: no golden fixture is produced under the
 *  dev fallback, because the fixture world runs with real sessions. Only the
 *  authority pair moves — everything else is the committed member body, so the
 *  render under test is the real emission. */
const devPosture = () => {
  const member = fixtures.settingsMember() as unknown as SettingsView
  return { ...member, editable: false, editable_reason: devReason }
}

test('a read-only page EXPLAINS itself, and the served reason stays alone in its own element', async () => {
  const member = fixtures.settingsMember() as unknown as SettingsView
  const { view } = await openSettings({
    'GET /api/settings': { body: member },
    'GET /api/settings/prices': { body: fixtures.pricesMember() },
  })

  // The landed pin holds unchanged: ONE element, and the server's sentence is
  // the whole of its text. The explanation has to be a SIBLING or this breaks.
  const notice = view.container.querySelector('[data-editable="false"]')
  expect(notice?.textContent).toBe(member.editable_reason)
  expect(view.container.querySelectorAll('[data-editable]')).toHaveLength(1)

  const block = view.container.querySelector('[data-readonly-explained="true"]')
  expect(block, 'a read-only page still explains nothing — the A-2 finding').not.toBeNull()
  expect(block, 'the explanation was folded into the served-reason element').not.toBe(notice)
  const said = block?.textContent ?? ''

  // (a) it is read-only for you, and the platform's own reason is the line above
  expect(said).toContain('read-only for you')
  expect(said, 'the block does not point at the served reason').toContain("platform's own reason")
  // (b) THE CONNECTION the gate record says nobody could make: absent-not-disabled
  expect(said).toContain('no Save')
  expect(said).toContain('no Reset')
  expect(said).toContain('bounds editor')
  expect(said).toContain('append a price')
  expect(said, 'the block does not teach absent-not-disabled').toContain('does not show a control it would refuse')
  // (c) everything is still readable
  for (const readable of ['value', 'bounds', 'badges', 'help', 'History', 'price table', 'recorded results']) {
    expect(said, `the block does not say ${readable} still reads`).toContain(readable)
  }
  // (d) the record's own posture-neutral sentence
  expect(said).toContain('Editing is real behind a real operator session')

  // …and the block's claim is TRUE on the page it is on: every write affordance
  // really is absent, and History really is there.
  expect(everySetting(view).length).toBeGreaterThan(0)
  for (const sel of ['[data-action="save"]', '[data-action="reset"]', '[data-action="edit-bounds"]', '[data-action="add-price-row"]', '[data-action="save-for-user"]']) {
    expect(view.container.querySelectorAll(sel), `the explanation says ${sel} is absent, and it is not`).toHaveLength(0)
  }
  expect(view.container.querySelectorAll('[data-action="history"]').length).toBeGreaterThan(0)
})

test('the explanation overclaims nothing: no bounced door, no unavailable read, no posture it was told', async () => {
  const member = fixtures.settingsMember() as unknown as SettingsView
  const { view } = await openSettings({
    'GET /api/settings': { body: member },
    'GET /api/settings/prices': { body: fixtures.pricesMember() },
  })
  const block = view.container.querySelector('[data-readonly-explained="true"]')!
  const said = (block.textContent ?? '').toLowerCase()

  // NO /login DOOR. For a signed-in member that link bounces straight back to
  // where they came from, which would be a control that cannot do what it says
  // — the C-1 defect, repeated. The dev reader's door is the header's own.
  expect(block.querySelectorAll('a'), 'the explanation offers a link').toHaveLength(0)
  expect(said, 'the explanation points at the sign-in door').not.toContain('sign in')
  expect(said).not.toContain('/login')
  expect(said, 'the explanation promises a member that logging in unlocks editing').not.toContain('log in')

  // IT NEVER SAYS A READ IS UNAVAILABLE, because none is: History sits outside
  // the write gate on purpose and every value renders read-only.
  for (const claim of ['not available', 'unavailable', 'hidden from you', 'cannot see', 'you cannot read']) {
    expect(said, `the explanation claims something is withheld: ${claim}`).not.toContain(claim)
  }
  // IT NEVER NAMES A POSTURE. Which reason applies is the SERVER's statement,
  // and it is already on screen above in the server's own words.
  for (const word of ['dev posture', 'dev-posture', 'member', 'operator session is missing']) {
    expect(said, `the client-authored copy names a posture (${word})`).not.toContain(word)
  }
  // The one place "operator" may appear is the record's own sentence.
  expect(said.split('operator').length - 1, 'the copy names the operator more than the one recorded sentence').toBe(1)
})

test('the dev-posture reader gets the SAME explanation under the SERVER\'s own dev sentence', async () => {
  // internal/api/settings.go:455, byte-exact: the dev fallback identity reads
  // and administers nothing. The client distinguishes the postures ONLY through
  // this string, so the arm is driven over a body that really carries it.
  const { view } = await openSettings({
    'GET /api/settings': { body: devPosture() },
    'GET /api/settings/prices': { body: fixtures.pricesMember() },
  })
  const notice = view.container.querySelector('[data-editable="false"]')
  expect(notice?.textContent, 'the dev reason was not rendered verbatim').toBe(devReason)
  expect(view.container.querySelectorAll('[data-editable]')).toHaveLength(1)

  // Same explanation, keyed on `editable === false` and nothing else — the
  // client reads no posture of its own to decide what to say.
  const said = view.container.querySelector('[data-readonly-explained="true"]')?.textContent ?? ''
  expect(said, 'a dev-posture reader gets no explanation').toContain('read-only for you')
  expect(said).toContain('Editing is real behind a real operator session')
  expect(said, 'the client re-typed the posture the server named').not.toContain('dev')
  // And the write surface is absent here too, for the same served reason.
  for (const sel of ['[data-action="save"]', '[data-action="edit-bounds"]']) {
    expect(view.container.querySelectorAll(sel), `a dev-posture reader was offered ${sel}`).toHaveLength(0)
  }
})

test('an operator sees NO explanation block, and every editor', async () => {
  // The negative direction, over the committed operator body: the block is keyed
  // on `editable === false` alone, so a page that can be edited does not carry
  // an explanation of why it cannot.
  const { view } = await openSettings()
  expect((served() as SettingsView).editable, 'the operator fixture is not editable').toBe(true)
  expect(view.container.querySelector('[data-editable="true"]')?.textContent).toBe(served().editable_reason)
  expect(
    view.container.querySelector('[data-readonly-explained="true"]'),
    'an editable page explains why it is read-only',
  ).toBeNull()
  click([...view.container.querySelectorAll('[data-tab]')].find((t) => (t.getAttribute('data-tab') ?? '').startsWith('freshness')))
  for (const sel of ['[data-save-for="freshness.max_age"]', '[data-action="edit-bounds"]', '[data-action="add-price-row"]']) {
    expect(view.container.querySelectorAll(sel).length, `the operator lost ${sel}`).toBeGreaterThan(0)
  }
})

test('the recorded-results panel TEACHES when it is empty — and teaches nothing while its read is in flight', async () => {
  const answer = fixtures.evalScores() as unknown as Answer
  const { view } = await openSettings({
    'GET /api/events/query/verdicts.eval_scores': { body: { ...answer, rows: [] } },
  })
  const text = view.container.textContent ?? ''
  expect(text).toContain('No suite result has been recorded yet.')
  expect(text, 'the empty state teaches nothing about what puts a row here').toContain('when a suite finishes')
  expect(text, 'the empty state lets "none" read as a failure').toContain('not the same as a suite having failed')

  // The D1 shape: this panel's OWN read never answers, so nothing may teach.
  const { view: pending } = await openSettings({
    'GET /api/events/query/verdicts.eval_scores': { pending: true },
  })
  expect(pending.container.textContent, 'a teaching empty rendered over a read that had not answered').not.toContain(
    'No suite result has been recorded yet.',
  )
})

test('the A-2 explanation reads at phone width and pins no pixel width', async () => {
  const pinned = /w-\[\d+px\]|min-w-\[\d+px\]|max-w-\[\d+px\]/
  const { view } = await openSettings({
    'GET /api/settings': { body: fixtures.settingsMember() },
    'GET /api/settings/prices': { body: fixtures.pricesMember() },
  })
  const nodes = [
    view.container.querySelector('[data-surface-what]'),
    view.container.querySelector('[data-readonly-explained="true"]'),
    view.container.querySelector('[data-editable="false"]'),
  ]
  for (const n of nodes) {
    expect(n, 'a render this leg is about is missing').not.toBeNull()
    expect(pinned.test((n as HTMLElement).className.toString()), 'a new render pins a pixel width').toBe(false)
    expect(/\d+px/.test(n?.getAttribute('style') ?? ''), 'a new render pins an inline width').toBe(false)
  }
  // Both-way probe on the detector itself.
  expect(pinned.test('max-w-[520px] text-sm')).toBe(true)
  expect(pinned.test('max-w-prose text-sm text-muted-foreground')).toBe(false)
})
