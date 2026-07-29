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

test('add-row composes the S10.3 row and renders the version and re-validation story', async () => {
  const { view, log } = await openSettings({
    'POST /api/settings/prices': {
      body: {
        row: (fixtures.prices() as unknown as StoredPricesView).rows?.[0],
        version: 'prices/2026-07-29#2',
        detail: 'row appended and in force: the table re-composed in place',
      },
    },
  })
  const form = view.container.querySelector('.add-price-row')!
  const fill = (field: string, value: string) => {
    typeInto(
      view.container.querySelector(`.add-price-row [data-price-field="${field}"]`) as HTMLInputElement,
      value,
    )
  }
  expect(form, 'the operator is offered no way to add a row').not.toBeNull()
  fill('model', 'claude-haiku-5')
  fill('lane', 'anthropic')
  fill('input_usd', '0.000001')
  fill('output_usd', '0.000005')
  fill('cache_read_usd', '0.0000001')
  fill('effective_from', '2026-08-01')
  fill('verified_on', '2026-07-29')
  fill('source', 'the published pricing page')
  fill('reason', 'the cheap lane went live')
  click(view.container.querySelector('[data-action="add-price-row"]'))
  await flush()

  // The POST specifically: this path is also the GET the panel reads from.
  const posted = log.calls.find((c) => c.method === 'POST' && c.path === '/api/settings/prices')
  expect(posted?.body).toEqual({
    row: {
      model: 'claude-haiku-5',
      lane: 'anthropic',
      unit_prices: { input_usd: 0.000001, output_usd: 0.000005, cache_read_usd: 0.0000001 },
      effective_from: '2026-08-01',
      verified_on: '2026-07-29',
      source: 'the published pricing page',
    },
    reason: 'the cheap lane went live',
  })
  expect(view.container.querySelector('[data-price-outcome="added"]')?.textContent).toContain('prices/2026-07-29#2')
})

test('a blank or mistyped unit price cannot be appended — a $0 row is the silent zero UNPRICED bars', async () => {
  const { view, log } = await openSettings()
  const fill = (field: string, value: string) => {
    typeInto(view.container.querySelector(`.add-price-row [data-price-field="${field}"]`) as HTMLInputElement, value)
  }
  const button = () => view.container.querySelector('[data-action="add-price-row"]') as HTMLButtonElement

  fill('model', 'claude-haiku-5')
  fill('lane', 'anthropic')
  // Model and lane alone are not enough: `Number('')` is 0, so an append here
  // would compose a row priced at nothing — and a row that EXISTS prices its
  // lane, so every call on it would be charged $0 with no trace.
  expect(button().disabled, 'a row with no unit prices could be appended').toBe(true)
  expect(view.container.querySelector('[data-price-guard="unit-prices"]')).not.toBeNull()

  fill('input_usd', '0.000001')
  fill('output_usd', 'not a number')
  fill('cache_read_usd', '0.0000001')
  expect(button().disabled, 'a mistyped unit price could be appended as NaN').toBe(true)

  fill('output_usd', '0')
  expect(button().disabled, 'a zero unit price could be appended').toBe(true)

  fill('output_usd', '-0.5')
  expect(button().disabled, 'a negative unit price could be appended').toBe(true)

  fill('output_usd', '0.000005')
  expect(button().disabled, 'a fully priced row is still refused').toBe(false)
  expect(log.calls.filter((c) => c.method === 'POST'), 'typing appended a row').toHaveLength(0)
})

test("the STORE's own refusal of a $0-priced row renders verbatim", async () => {
  // The form guards the shape; the STORE is the authority (OQ6), and it refuses
  // the same row on its own terms. Its sentence is what a person reads.
  const { view } = await openSettings({
    'POST /api/settings/prices': {
      status: 400,
      body: {
        error: 'bad_request',
        detail:
          'metering: price row is not admissible: the output unit price is 0 — a declared price must be a real positive number, and a row that priced a lane at nothing would silently charge $0 where the empty table would honestly say UNPRICED (S10.1)',
      },
    },
  })
  for (const [field, value] of [
    ['model', 'claude-haiku-5'],
    ['lane', 'anthropic'],
    ['input_usd', '0.000001'],
    ['output_usd', '0.000005'],
    ['cache_read_usd', '0.0000001'],
  ] as [string, string][]) {
    typeInto(view.container.querySelector(`.add-price-row [data-price-field="${field}"]`) as HTMLInputElement, value)
  }
  click(view.container.querySelector('[data-action="add-price-row"]'))
  await flush()
  expect(view.container.querySelector('[data-price-outcome="failed"]')?.textContent).toContain(
    'would silently charge $0',
  )
})

test("the store's refusal on a field the FORM does not guard renders in its own words", async () => {
  // The form guards the unit prices, because a blank there composes a silent
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
  for (const [field, value] of [
    ['model', 'claude-haiku-5'],
    ['lane', 'anthropic'],
    ['input_usd', '0.000001'],
    ['output_usd', '0.000005'],
    ['cache_read_usd', '0.0000001'],
  ] as [string, string][]) {
    typeInto(view.container.querySelector(`.add-price-row [data-price-field="${field}"]`) as HTMLInputElement, value)
  }
  click(view.container.querySelector('[data-action="add-price-row"]'))
  await flush()
  expect(log.calls.some((c) => c.method === 'POST' && c.path === '/api/settings/prices')).toBe(true)
  expect(view.container.querySelector('[data-price-outcome="failed"]')?.textContent).toContain(
    'never an aggregator',
  )
})

// ── R22: the eval-results surface (the D4(b) unlock) ──────────────────────

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
