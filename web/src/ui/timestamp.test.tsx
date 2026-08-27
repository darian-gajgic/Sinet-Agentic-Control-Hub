import { expect, test } from 'vitest'

import { mount } from '../testing'
import { Timestamp } from './index'
import { relativeLabel } from './Timestamp'

/**
 * The D5 timestamp primitive (B6 gate §9 D5 answer).
 *
 * The load-bearing property is the same in both variants: the served UTC string
 * survives into the DOM. A relative label is computed against a device clock
 * nobody controls, so it may only ever be ADDED beside the record — never
 * substituted for it.
 */

const served = '2026-08-04T09:15:00Z'

test('the audit variant renders the served UTC verbatim and adds nothing', () => {
  const view = mount(<Timestamp ts={served} variant="audit" />)
  const time = view.container.querySelector('time')!
  expect(time.getAttribute('dateTime')).toBe(served)
  expect(time.textContent).toBe(served)
  // Nothing else: a record is the string, not a paraphrase of it.
  expect(view.container.textContent).toBe(served)
  view.unmount()
})

test('audit is the default, so a call site that says nothing gets the record', () => {
  const view = mount(<Timestamp ts={served} />)
  expect(view.container.textContent).toBe(served)
  view.unmount()
})

test('the live variant keeps the verbatim UTC on the element while the words read relative', () => {
  // REWRITTEN 2026-08-27 (P3-GF9; walk W5): the first rendering printed the
  // raw UTC string INLINE beside every friendly time and the walk read it as
  // noise on every requester surface. The load-bearing property is unchanged
  // — the served string survives into the DOM, machine-readable and one
  // hover away — but it is no longer the visible text.
  const view = mount(<Timestamp ts={served} variant="live" />)
  const time = view.container.querySelector('time')!
  expect(time.getAttribute('dateTime')).toBe(served)
  expect(time.getAttribute('title'), 'the exact instant left the hover').toBe(served)
  // The visible words are the relative label, not the raw stamp.
  const text = view.container.textContent ?? ''
  expect(text, 'the raw stamp is back on the visible line').not.toContain(served)
  expect(text, 'the live variant rendered no relative label').not.toBe('')
  view.unmount()
})

test('an absent stamp renders the honest absence rather than an epoch or a blank', () => {
  for (const empty of [null, undefined, '']) {
    const view = mount(<Timestamp ts={empty} variant="live" />)
    expect(view.container.textContent).toBe('not recorded')
    expect(view.container.querySelector('time'), 'an absent stamp rendered a <time>').toBeNull()
    view.unmount()
  }
})

test('an unreadable stamp still renders verbatim and gains no invented label', () => {
  // The served string is the record. A stamp we could not parse is exactly the
  // case where inventing "3m ago" would be a fabricated number.
  const view = mount(<Timestamp ts="not-a-timestamp" variant="live" />)
  expect(view.container.textContent).toBe('not-a-timestamp')
  view.unmount()
})

test('relative labels coarsen with distance and read correctly in both directions', () => {
  const now = Date.parse('2026-08-04T12:00:00Z')
  const ago = (ms: number) => relativeLabel(now - ms, now)
  expect(ago(5_000)).toBe('just now')
  expect(ago(90_000)).toBe('1m ago')
  expect(ago(3 * 60_000)).toBe('3m ago')
  expect(ago(2 * 3_600_000)).toBe('2h ago')
  expect(ago(5 * 86_400_000)).toBe('5d ago')
  // A served instant in the future is a real case — a park horizon is one —
  // and "in 3m" is the honest reading of it.
  expect(relativeLabel(now + 3 * 60_000, now)).toBe('in 3m')
  expect(relativeLabel(now + 5_000, now)).toBe('in a moment')
})
