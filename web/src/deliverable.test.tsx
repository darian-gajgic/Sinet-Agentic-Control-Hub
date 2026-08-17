import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import App from './App'
import { frameSrc } from './Deliverable'
import type { AcceptCard, Comparison, PlacedComments } from './api'
import {
  FakeSource,
  binaryDeliverableID,
  fixtures,
  imageDeliverableID,
  notebookDeliverableID,
  reviewDeliverableID,
  reviewRoutes,
  reworkDeliverableID,
  scriptedFetch,
  signedInAsOperator,
  type Scripted,
} from './doubles'
import { EventStream } from './events'
import { hrefFor } from './routes'
import { click, flush, mount, typeInto } from './testing'
// The restyle's own pins read the real shipped sources, because the cascade
// hazard they guard against is a property of the STYLESHEET beside the MARKUP
// and neither one alone can show it (§53).
import cssSource from './index.css?raw'
import deliverableSource from './Deliverable.tsx?raw'
import taskDetailSource from './TaskDetail.tsx?raw'
import workforceSource from './Workforce.tsx?raw'

/**
 * The review surface (Spec S15.8; S13.1–S13.4, S13.6, S13.8; FC-v1 §2), driven
 * against the golden bodies the fixture world produces through S13's own verbs.
 *
 * WHAT IS COMMITTED GROUND AND WHAT IS DERIVED. Every read this surface makes has
 * a producer-driven committed body: the detail with its doors (both limbs of the
 * request-revision door, from two deliverables), the six comparison surfaces, the
 * placed comments carrying all five anchor statuses beside a consumed batch, the
 * accept card with real trailers, and the live-session list.
 *
 * Four families are DERIVED from those bodies, each named at its assertion and
 * each for a reason that is about the world and not about convenience (OQ8):
 *
 *  - the ACCEPT OUTCOMES (applied, merge card, read-back, superseded, stale, PIN
 *    refused). The reason first recorded here was FALSE — it claimed these need a
 *    git remote, a broker push and a collision engineered against a moving HEAD,
 *    all three of which `internal/accept/accept_test.go` already does hermetically:
 *    `TestAcceptEndToEnd` applies a real accept against a t.TempDir bare remote
 *    through the real broker server + client, and `TestAcceptCollisionMergeCard`
 *    engineers the collision. A `fakePusher` is composed in THIS fixture world
 *    (internal/api) too, so the push is not the obstacle either. The TRUE reason is the one
 *    the snapshot pins give: a real accept produces a commit whose SHA is
 *    unpinnable, because its parent's is — the base commit is seeded without
 *    GIT_AUTHOR_DATE/GIT_COMMITTER_DATE, so it varies per run and the accept sha
 *    varies with it, and a varying sha cannot live in a fixed-clock golden body.
 *    Not the timestamp: `When` is recorded UTC for determinism and this world
 *    pins the journal clock, so that half would in fact be stable — the reason
 *    rests on the sha alone.
 *    The accept CARD — which is what the pin, the trailers and the acceptability
 *    come from — is real.
 *  - the PREVIEW dispositions and the comparison. A launched preview spawns a
 *    process and binds a port, which no hermetic test may do, and the whole point
 *    of the non-backed states is that they render an ANSWER — so they are driven
 *    as the server's own state vocabulary over the real Session shape.
 *  - an UNKNOWN `surface` value, which by definition no current producer emits, and
 *    a SERVED pixel-diff aid, which is a nil seam at v0.
 *  - the THREE object-surface DETAILS — image, binary and notebook — whose
 *    COMPARISONS are the real committed bodies; a fourth committed detail would
 *    prove nothing the first does not.
 */

const inertStream = () =>
  new EventStream({
    createEventSource: (url) => new FakeSource(url),
    probeSession: () => Promise.resolve({ authenticated: true }),
    schedule: () => 0,
    cancel: () => {},
  })

async function review(id = reviewDeliverableID, extra: Record<string, Scripted> = {}) {
  const routes = { ...reviewRoutes(), ...extra }
  const log = scriptedFetch(routes)
  window.history.replaceState(null, '', `/deliverables/${id}`)
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}

/** The signed-in identity in these suites is alice, who OWNS the fixture world's
 *  deliverables — which is what makes the accept form render at all. `asMember`
 *  is the same read taken by somebody else, for the authorship half. */
const asBob: Scripted = {
  body: { authenticated: true, user: { user_id: 'bob', display_name: 'Bob', role: 'member', pin_set: true } },
}

const text = (view: { container: HTMLElement }) => view.container.textContent ?? ''
const at = (view: { container: HTMLElement }, sel: string) => view.container.querySelector(sel)
const all = (view: { container: HTMLElement }, sel: string) => [...view.container.querySelectorAll(sel)]

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── R7 / rubric 1: the doors ────────────────────────────────────────────────

test('every served door renders, an open one is a control and a closed one is its reason', async () => {
  const { view } = await review()
  const served = fixtures.deliverableReview() as { doors: { verb: string; available: boolean; reason: string }[] }
  const rendered = all(view, '[data-door]').map((n) => n.getAttribute('data-door'))

  // Both directions at once: no door invented, none hidden.
  expect(rendered, 'the doors rendered are not the doors served').toEqual(served.doors.map((d) => d.verb))
  for (const door of served.doors) {
    const row = at(view, `[data-door="${door.verb}"]`)!
    expect(row.getAttribute('data-available')).toBe(door.available ? 'true' : 'false')
    expect(row.textContent, `the ${door.verb} door does not carry its served reason`).toContain(door.reason)
  }
  // A closed door has no pressable control of its own.
  const closed = served.doors.find((d) => !d.available)!
  expect(
    at(view, `[data-door="${closed.verb}"]`)!.querySelectorAll('button'),
    'a closed door rendered a control',
  ).toHaveLength(0)
})

test('the request-revision door renders BOTH limbs, from two deliverables', async () => {
  // In-review with no rework card: the door names NO route and carries the
  // narrative instead. A route naming a verb that cannot open from this state
  // would be the dead-door shape.
  const closed = await review()
  const closedRow = at(closed.view, '[data-door="request-revision"]')!
  expect(closedRow.getAttribute('data-available')).toBe('false')
  expect(closedRow.querySelector('.door-route'), 'a closed door named a route').toBeNull()
  closed.view.unmount()

  // Parked on a rework card: the door is live and carries the ask, the answer verb
  // and the card's own pin.
  const open = await review(reworkDeliverableID)
  const openRow = at(open.view, '[data-door="request-revision"]')!
  expect(openRow.getAttribute('data-available')).toBe('true')
  const served = (fixtures.deliverableRework() as { doors: { verb: string; route: string; ask_id?: string }[] }).doors.find(
    (d) => d.verb === 'request-revision',
  )!
  expect(openRow.textContent).toContain(served.route)
  expect(openRow.querySelector('[data-action="request-revision"]')).not.toBeNull()
})

test('the FINISHED limb offers NO control it cannot honour (drain r1, D4)', async () => {
  // The verb `request-revision` covers two server states. Finished means the
  // S13.9 FOLLOW-UP: route /follow-up, preset revision, and deliberately no pin
  // and no answer, because a follow-up is a successor task rather than an answer
  // to a card. Gating the form on the VERB rendered it here anyway, with an
  // ENABLED submit button whose handler could only report "The door named no
  // route, pin or answer" — the dead control this file's own rule forbids.
  const { view } = await review('d-notes', {
    'GET /api/deliverables/d-notes': { body: fixtures.deliverableDetail() },
    'GET /api/deliverables/d-notes/compare': { body: fixtures.compareLineDiff() },
    'GET /api/deliverables/d-notes/comments?revision=2': { body: fixtures.placedComments() },
  })
  const row = at(view, '[data-door="request-revision"]')!
  expect(row.getAttribute('data-available')).toBe('true')
  expect(
    row.querySelector('[data-action="request-revision"]'),
    'the finished limb renders a submit button, and pressing it can only fail',
  ).toBeNull()
  expect(row.querySelector('[data-field="revision-guidance"]')).toBeNull()
  // The door itself is NOT hidden — a closed control is not a hidden door. Its
  // route, its preset and its reason still render, which is what lets a reader
  // see the door exists and where it lives.
  expect(row.textContent).toContain('/api/deliverables/d-notes/follow-up')
  expect(row.textContent).toContain('preset revision')

  // The control, without which "no button" could mean the form never renders at
  // all: the CARD-BACKED limb, which supplies a pin and an answer, still does.
  const open = await review(reworkDeliverableID)
  const openRow = at(open.view, '[data-door="request-revision"]')!
  expect(openRow.querySelector('[data-action="request-revision"]')).not.toBeNull()
  open.view.unmount()
})

test('the FINISHED limb links the follow-up spawn under the revision preset', async () => {
  // d-notes is accepted, so its request-revision door is the S13.9 follow-up.
  const { view } = await review('d-notes', {
    'GET /api/deliverables/d-notes': { body: fixtures.deliverableDetail() },
    'GET /api/deliverables/d-notes/compare': { body: fixtures.compareLineDiff() },
    'GET /api/deliverables/d-notes/comments?revision=2': { body: fixtures.placedComments() },
  })
  const row = at(view, '[data-door="request-revision"]')!
  expect(row.getAttribute('data-available')).toBe('true')
  expect(row.textContent).toContain('/api/deliverables/d-notes/follow-up')
  expect(row.textContent, 'the follow-up limb does not name its preset').toContain('preset revision')
})

// ── R1 / rubric 2: the diff widget ──────────────────────────────────────────

test('split AND unified render the SERVED unified diff through the pick, with edit marks', async () => {
  const { view } = await review()
  const served = fixtures.compareLineDiff() as unknown as Comparison

  // The file headers name the deliverable's own paths, which is what the widget
  // reads a file's identity from.
  expect(all(view, '[data-file]').map((n) => n.getAttribute('data-file'))).toContain('site/release.tsx')
  // Real inserted and deleted lines, distinguishable in the DOM — the anchor model
  // is built on exactly that.
  expect(all(view, '.diff-code-insert').length).toBeGreaterThan(0)
  expect(all(view, '.diff-code-delete').length).toBeGreaterThan(0)
  // The line the served diff actually added.
  expect(text(view)).toContain("import { theme } from './theme'")
  // Highlighting is the widget's own tokenizer: markEdits emits per-word edit
  // spans, so their presence is the checkable half of "client-side, no server
  // highlighter" (the other half is that the server sent no markup — below).
  expect(all(view, '.diff-code-edit').length, 'no edit marks rendered — the tokenizer is not wired').toBeGreaterThan(0)
  // And the server sent no markup at all: the whole render came from plain text.
  expect(served.unified ?? '').not.toContain('<span')

  // The inline view is the same served text through the same parse.
  click(at(view, '[data-view-type="unified"]'))
  await flush()
  expect(all(view, '.diff-code-insert').length).toBeGreaterThan(0)
  expect(text(view)).toContain("import { theme } from './theme'")
})

test('the view consumes Comparison.unified and no other diff source', async () => {
  const src = (await import('./Deliverable.tsx?raw')).default as string
  // One parser, and it is the pick's. A second parse of diff text anywhere in this
  // file would be a second answer to "what does this diff say".
  expect(src).toContain('parseDiff(unified)')
  expect(src).toContain('parseUnified(cmp.unified')
  for (const banned of ['diff-match-patch', 'jsdiff', 'createTwoFilesPatch', 'unifiedDiff(']) {
    expect(src, `a second diff parser (${banned}) reached the review view`).not.toContain(banned)
  }
})

// ── R2 / rubric 3: round-over-round and the pair navigation ─────────────────

test('the default read sends NO bounds, and the rendered pair is labelled from the answer', async () => {
  const { view, log } = await review()
  const compares = log.calls.filter((c) => c.path.includes('/compare'))
  expect(compares, 'no comparison was read').not.toHaveLength(0)
  expect(compares[0].path, 'the default read composed a pair instead of asking for the default').toBe(
    `/api/deliverables/${reviewDeliverableID}/compare`,
  )
  const served = fixtures.compareLineDiff() as unknown as Comparison
  expect(at(view, '[data-compared]')!.getAttribute('data-compared')).toBe(`${served.old_n}:${served.new_n}`)
  expect(text(view)).toContain("the platform's own round-over-round default")
})

test('navigation drives an explicit pair including old=0, the pre-task base', async () => {
  const { view, log } = await review()
  const olds = at(view, '[data-pick="old"]') as HTMLSelectElement
  // 0 is offered as what it is: a target that is not a revision.
  expect([...olds.options].map((o) => o.value)).toContain('0')
  expect(olds.textContent).toContain('the pre-task base')

  const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')?.set
  await flush()
  setter?.call(olds, '0')
  olds.dispatchEvent(new Event('change', { bubbles: true }))
  await flush()

  expect(
    log.calls.some((c) => c.path === `/api/deliverables/${reviewDeliverableID}/compare?old=0&new=2`),
    'picking the base did not drive an explicit pair',
  ).toBe(true)
  expect(at(view, '[data-compared]')!.textContent).toContain('pre-task base')
})

// ── R3 / rubric 4: every surface type, degrade lanes included ───────────────

test('the image-pair surface renders the trio, the hash verdict and the per-side cards', async () => {
  const { view } = await review(imageDeliverableID)
  const served = fixtures.compareImagePair() as unknown as Comparison

  expect(at(view, '.image-pair')).not.toBeNull()
  // The trio, all three modes offered and each one switchable.
  expect(all(view, '[data-image-mode]').map((n) => n.getAttribute('data-image-mode'))).toEqual([
    '2-up',
    'swipe',
    'onion',
  ])
  expect(at(view, '[data-changed]')!.getAttribute('data-changed')).toBe(served.changed === true ? 'true' : 'false')

  // The bytes are addressed by the SERVED sha, through the owner-scoped read.
  const oldSHA = (served.old_objects ?? [])[0].sha256
  const newSHA = (served.new_objects ?? [])[0].sha256
  const srcs = all(view, 'img').map((n) => n.getAttribute('src'))
  expect(srcs).toContain(`/api/deliverables/${imageDeliverableID}/objects/${oldSHA}`)
  expect(srcs).toContain(`/api/deliverables/${imageDeliverableID}/objects/${newSHA}`)

  click(at(view, '[data-image-mode="swipe"]'))
  await flush()
  expect(at(view, '[data-control="swipe"]'), 'the swipe control did not render').not.toBeNull()
  click(at(view, '[data-image-mode="onion"]'))
  await flush()
  expect(at(view, '[data-control="onion"]'), 'the onion control did not render').not.toBeNull()

  // The per-side metadata card, and the aid that was not computed says so.
  expect(all(view, '[data-object]').map((n) => n.getAttribute('data-object')).sort()).toEqual([oldSHA, newSHA].sort())
  expect(text(view)).toContain('No pixel-diff aid was computed')
})

test('a side the platform will not serve inline says so with the recorded type', async () => {
  const { view } = await review(imageDeliverableID)
  const img = at(view, 'img') as HTMLImageElement
  // The client keeps NO copy of the server's inline allowlist. It learns from the
  // actual response: a load failure is what says "not served inline here", and the
  // recorded type is what makes the absence readable.
  img.dispatchEvent(new Event('error'))
  await flush()
  expect(text(view)).toContain('does not serve image/png inline')
  expect(text(view)).toContain('download the object to inspect it')
})

test('the binary-cards surface renders metadata, the hash verdict and download-to-inspect', async () => {
  const { view } = await review(binaryDeliverableID)
  const served = fixtures.compareBinaryCards() as unknown as Comparison
  const ref = (served.new_objects ?? [])[0]

  expect(text(view)).toContain(ref.name)
  expect(text(view)).toContain(String(ref.size))
  expect(text(view)).toContain(ref.type ?? '')
  expect(text(view)).toContain(ref.sha256)
  const link = at(view, `[data-object="${ref.sha256}"] [data-action="download-object"]`) as HTMLAnchorElement
  expect(link.getAttribute('href')).toBe(`/api/deliverables/${binaryDeliverableID}/objects/${ref.sha256}`)
  // An opaque blob has no rich comparison, so no diff widget is drawn.
  expect(at(view, '.diff-file'), 'a binary surface drew a diff widget').toBeNull()
})

test('the PDF extraction failure degrades to labeled cards, and the label is on screen', async () => {
  const { view } = await review(reworkDeliverableID)
  const served = fixtures.comparePdfDegrade() as unknown as Comparison
  expect(served.fallback).toBe(true)
  // The whole honesty posture: a degrade NEVER poses as a rich surface, so its
  // served label renders verbatim beside it.
  const label = at(view, '[data-surface-label]')!
  expect(label.getAttribute('data-surface-label')).toBe(served.surface)
  expect(label.textContent).toContain('Fallback surface')
  expect(label.textContent).toContain(served.label ?? '')
  expect(at(view, '.object-cards'), 'the degrade did not fall back to the metadata cards').not.toBeNull()
})

test('the extracted-text FALLBACK renders the same diff widget under its served label', async () => {
  const { view } = await review(notebookDeliverableID)
  const served = fixtures.compareExtractedText() as unknown as Comparison
  expect(served.surface).toBe('extracted-text-diff')
  expect(served.fallback).toBe(true)

  // The SAME widget — a fallback is not a lesser renderer, it is the same one
  // under a label that says the platform has no rich comparison for this type.
  expect(all(view, '.diff-file').length, 'the fallback did not render the diff widget').toBeGreaterThan(0)
  expect(all(view, '.diff-code-insert').length).toBeGreaterThan(0)
  const label = at(view, '[data-surface-label]')!
  expect(label.getAttribute('data-surface-label')).toBe('extracted-text-diff')
  expect(label.textContent, 'a fallback surface posed as a rich one').toContain('Fallback surface')
  expect(label.textContent).toContain(served.label ?? '')
})

test('a SERVED pixel-diff aid renders as the measurement it is', async () => {
  // DERIVED BODY, named: the aid is a nil seam at v0 (S13.2), so no producer fills
  // it and the absent branch is the only real one. The served branch has to exist
  // for the day one does, and it renders the figure AS MEASURED with a link to the
  // aid image through the same owner-scoped object read.
  const withAid = {
    ...(fixtures.compareImagePair() as unknown as Comparison),
    pixel_diff: { changed_ratio: 0.042, diff_object_sha: 'a'.repeat(64) },
  }
  const { view } = await review(imageDeliverableID, {
    [`GET /api/deliverables/${imageDeliverableID}/compare`]: { body: withAid },
  })
  const aid = at(view, '[data-pixel-diff="served"]')!
  expect(aid.textContent).toContain('0.042')
  expect(aid.textContent, 'the figure is not stated as a measurement').toContain('as measured')
  expect((aid.querySelector('[data-action="download-pixel-diff"]') as HTMLAnchorElement).getAttribute('href')).toBe(
    `/api/deliverables/${imageDeliverableID}/objects/${'a'.repeat(64)}`,
  )
})

test('an UNKNOWN surface value renders the honest state with the served fields', async () => {
  // DERIVED BODY, and it has to be: an unrecognised `surface` is by definition a
  // FUTURE server's answer, so no producer in this tree can mint one. It derives
  // from the real committed comparison and changes only that one field.
  const future = { ...(fixtures.compareBinaryCards() as unknown as Comparison), surface: 'pixel-overlay' }
  const { view } = await review(binaryDeliverableID, {
    [`GET /api/deliverables/${binaryDeliverableID}/compare`]: { body: future },
  })
  const block = at(view, '[data-surface="pixel-overlay"]')!
  expect(block.textContent).toContain('no rich comparison surface')
  expect(block.textContent, 'the unknown surface hid the value it could not draw').toContain('pixel-overlay')
  // Not a blank and not a crash: what the platform served still renders.
  expect(text(view)).toContain(future.type)
  expect(at(view, '.object-cards')).not.toBeNull()
})

// ── R4/R5/R6 / rubric 5, 6, 7: the comment loop ─────────────────────────────

test('all five placement statuses render, visibly distinct, from one served body', async () => {
  const { view } = await review()
  const served = fixtures.placedComments() as unknown as PlacedComments
  const statuses = new Set(served.placements.map((p) => p.status))
  for (const want of ['exact', 'mapped', 'drifted', 'file', 'orphan']) {
    expect(statuses, `the committed body carries no ${want} placement`).toContain(want)
  }
  // Each comment's rendered card carries the status the SERVER computed, and the
  // three degraded ones say so in words rather than only in an attribute.
  for (const p of served.placements) {
    // review #2: list and strip are complements — the card renders exactly once
    // somewhere under `.comments`, wherever its placement puts it.
    const card = at(view, `.comments [data-comment="${p.comment_id}"]`)!
    expect(card.getAttribute('data-placement'), `comment ${p.comment_id} renders the wrong placement`).toBe(p.status)
  }
  expect(text(view)).toContain('drifted — the quote was found near, not at, the mapped position')
  expect(text(view)).toContain('orphaned — no live location in this revision at all')
  expect(text(view)).toContain('file-level — no line position, the quote is kept')
})

// THE NO-INVISIBLE-COMMENT INVARIANT lives in ONE test, further down: the
// DOM-driven `D2/D3: EVERY served comment is reachable as a widget or present on
// the strip`. The version that sat here was written first and is deleted (drain r2
// R4) rather than kept beside it: its first half asserted a `[data-comment]` node
// per served comment, which is a property of `comments.map` running, and its second
// half recomputed the production strip predicate and compared the implementation to
// a copy of itself — so it passed on the code D2 found broken. Two tests where one
// is decorative invites trusting the wrong one.

test('an anchored comment renders AT its placement inside the diff', async () => {
  const { view } = await review()
  const served = fixtures.placedComments() as unknown as PlacedComments
  const anchored = served.placements.find((p) => p.anchor !== undefined && p.anchor.line_no !== 0)!
  // The widget region is inside the diff table, not in the list below it.
  const widgets = all(view, '.diff-widget [data-comment]').map((n) => n.getAttribute('data-comment'))
  expect(widgets, 'no comment rendered at a position inside the diff').not.toHaveLength(0)
  expect(widgets).toContain(String(anchored.comment_id))
})

test('open and consumed render distinctly, and a consumed one shows [F#] and its attempt', async () => {
  const { view } = await review()
  const served = fixtures.placedComments() as unknown as PlacedComments
  const consumed = served.comments.find((c) => c.status === 'consumed')!
  const open = served.comments.find((c) => c.status === 'open')!

  const consumedCard = at(view, `.comments [data-comment="${consumed.id}"]`)!
  expect(consumedCard.getAttribute('data-status')).toBe('consumed')
  expect(consumedCard.querySelector('[data-lifecycle="consumed"]')).not.toBeNull()
  expect(consumedCard.textContent).toContain(`[F${String(consumed.finding_number)}]`)
  expect(consumedCard.textContent, 'the batch stamp is not on screen').toContain(consumed.consumed_at ?? '')
  expect(consumedCard.textContent, 'the consuming attempt is not on screen').toContain(consumed.consumed_by ?? '')

  const openCard = at(view, `.comments [data-comment="${open.id}"]`)!
  expect(openCard.querySelector('[data-lifecycle="open"]')).not.toBeNull()
})

test('a verification finding renders under the same schema as a human comment', async () => {
  const { view } = await review()
  const served = fixtures.placedComments() as unknown as PlacedComments
  const finding = served.comments.find((c) => c.kind === 'finding')!
  const card = at(view, `.comments [data-comment="${finding.id}"]`)!
  expect(card.getAttribute('data-kind')).toBe('finding')
  expect(card.textContent).toContain(finding.category ?? '')
  expect(card.textContent).toContain(finding.criterion ?? '')
  // The as-supplied anchor is kept verbatim: it is the finding's own claim about
  // where it applies, and it is not a position.
  expect(card.textContent, 'origin_anchor did not render').toContain(finding.origin_anchor ?? '')
})

test('the severity vocabulary renders with its S13.4 meaning, and an empty one says so', async () => {
  const { view } = await review()
  expect(text(view)).toContain('blocker — this triggers another round')
  expect(text(view)).toContain('note — polish, travels along')
})

test('NO update and NO delete affordance exists — immutability renders as the property it is', async () => {
  const { view } = await review()
  // The DOM walk: nothing on a comment card offers to change or remove it.
  for (const card of all(view, '[data-comment]')) {
    const labels = [...card.querySelectorAll('button, a')].map((n) => (n.textContent ?? '').toLowerCase())
    for (const label of labels) {
      for (const banned of ['edit', 'delete', 'remove', 'update']) {
        expect(label, `a comment card offers to ${banned} it`).not.toContain(banned)
      }
    }
  }
  // The code scan: no such verb is even reachable, and no drain verb exists here
  // either — consumption is THE drain's, and this surface renders it, never runs it.
  const src = (await import('./Deliverable.tsx?raw')).default as string
  for (const banned of ['DELETE', 'PATCH', 'PUT', '/drain', 'api.drain']) {
    expect(src, `the review view reaches a ${banned} verb`).not.toContain(banned)
  }
})

test('selecting a diff line composes the CLAIMED anchor from the rendered hunk data', async () => {
  const { view, log } = await review()
  // The gutter of the rendered diff is what a person clicks.
  const gutters = all(view, '.diff-gutter')
  expect(gutters.length, 'the diff rendered no clickable gutter').toBeGreaterThan(0)
  click(gutters.find((g) => (g.textContent ?? '').trim() !== ''))
  await flush()

  const claim = at(view, '[data-claim]')!.getAttribute('data-claim') ?? ''
  expect(claim, 'no anchor was claimed from the rendered hunk').not.toBe('')
  expect(claim, 'the claimed anchor names no file from the served diff').toContain('site/')

  typeInto(at(view, '[data-field="body"]') as HTMLTextAreaElement, 'This line needs a comment.')
  const severity = at(view, '[data-field="severity"]') as HTMLSelectElement
  const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')?.set
  setter?.call(severity, 'blocker')
  severity.dispatchEvent(new Event('change', { bubbles: true }))
  await flush()

  const created = { ...(fixtures.placedComments() as unknown as PlacedComments).comments[0], id: 99 }
  log.set(`POST /api/deliverables/${reviewDeliverableID}/comments`, { body: created })
  click(at(view, '[data-action="add-comment"]'))
  await flush()

  const posted = log.calls.find((c) => c.method === 'POST' && c.path.endsWith('/comments'))!
  const body = posted.body as { anchor?: { file_path: string; line_no: number; line_text: string }; severity: string }
  expect(body.severity, 'the create body carries a severity outside the closed vocabulary').toBe('blocker')
  expect(body.anchor, 'the POST carried no claimed anchor').toBeTruthy()
  expect(body.anchor!.line_text, 'the claimed quote is empty, so the ladder could never confirm it').not.toBe('')
  // The quote carries NO diff marker: a leading + or - would make every claim fail
  // to confirm against the revision's own line.
  expect(['+', '-'], 'the claimed quote kept its diff marker').not.toContain(body.anchor!.line_text[0])
})

test("the response's OWN born status renders — a wrong claim shows the server's answer", async () => {
  const { view, log } = await review()
  typeInto(at(view, '[data-field="body"]') as HTMLTextAreaElement, 'A file-level point.')
  typeInto(at(view, '[data-field="file-level"]') as HTMLInputElement, 'site/README.md')
  await flush()

  // The server-CORRECTED case: whatever was claimed, the platform placed it
  // file-level, and that is what the surface says.
  const corrected = {
    ...(fixtures.placedComments() as unknown as PlacedComments).comments[0],
    id: 101,
    anchor_status: 'file',
  }
  log.set(`POST /api/deliverables/${reviewDeliverableID}/comments`, { body: corrected })
  click(at(view, '[data-action="add-comment"]'))
  await flush()

  const born = at(view, '[data-born-status]')!
  expect(born.getAttribute('data-born-status')).toBe('file')
  expect(born.textContent).toContain('file-level')
  expect(born.textContent, 'the surface did not say the platform decided the position').toContain(
    'that is where it lives, whatever position was claimed',
  )
  // File-level entry is first-class: no anchor was sent at all.
  const posted = log.calls.find((c) => c.method === 'POST' && c.path.endsWith('/comments'))!
  expect((posted.body as { anchor?: unknown }).anchor).toBeUndefined()
  expect((posted.body as { file_level?: string }).file_level).toBe('site/README.md')
})

// ── R7 / rubric 8: the request-revision answer flow ─────────────────────────

test('the rework door posts the door OWN ask, pin and revise_with_guidance', async () => {
  const { view, log } = await review(reworkDeliverableID)
  const door = (
    fixtures.deliverableRework() as {
      doors: { verb: string; route: string; ask_id?: string; payload_hash?: string; answer?: string }[]
    }
  ).doors.find((d) => d.verb === 'request-revision')!

  typeInto(at(view, '[data-field="guidance"]') as HTMLTextAreaElement, 'Tighten the summary to two sentences.')
  typeInto(at(view, '[data-field="revision-pin"]') as HTMLInputElement, 'hunter2hunter')
  await flush()

  // The route is the DOOR'S own — the card id is a composite whose halves the door
  // serves separately, so the surface follows the path it was given rather than
  // assembling one from `ask_id`.
  log.set(`POST ${door.route}`, {
    body: { id: `ask:${door.ask_id}`, applied: true, state: 'answered', detail: 'the next round will drain the guidance' },
  })
  click(at(view, '[data-action="request-revision"]'))
  await flush(8)

  const answered = log.calls.find((c) => c.path === door.route)!
  expect(answered.path, 'the answer did not go to the route the door named').toContain(`:${door.ask_id}/answer`)
  const body = answered.body as {
    payload_hash: string
    answer: { choice: string; guidance: { text: string }[] }
    pin?: string
  }
  expect(body.payload_hash, "the answer did not quote the door's own pin").toBe(door.payload_hash)
  expect(body.answer.choice).toBe(door.answer)
  // The guidance rides the ANSWER — the platform records it as durable requester
  // comments and drains it into the resumed attempt, in that order, in this one
  // request. The validator refuses the verb without it.
  expect(body.answer.guidance.map((g) => g.text)).toEqual(['Tighten the summary to two sentences.'])
  expect(body.pin).toBe('hunter2hunter')
  expect(text(view)).toContain('the next round will drain the guidance')
  // The PIN is gone from the form the moment it was sent.
  expect((at(view, '[data-field="revision-pin"]') as HTMLInputElement).value).toBe('')
})

test('a stale rework pin is a re-read, never a retry', async () => {
  const { view, log } = await review(reworkDeliverableID)
  const door = (fixtures.deliverableRework() as { doors: { verb: string; route: string }[] }).doors.find(
    (d) => d.verb === 'request-revision',
  )!
  log.set(`POST ${door.route}`, {
    status: 409,
    body: { error: 'stale_payload', detail: 'the card moved', current: {} },
  })
  typeInto(at(view, '[data-field="guidance"]') as HTMLTextAreaElement, 'Say it again.')
  await flush()
  click(at(view, '[data-action="request-revision"]'))
  await flush(8)

  expect(at(view, '[data-stale="request-revision"]')).not.toBeNull()
  expect(text(view)).toContain('NOTHING was written')
  expect(log.calls.filter((c) => c.path === door.route), 'a stale pin was retried').toHaveLength(1)
})

// ── R8 / rubric 9: try it out ───────────────────────────────────────────────

/** A launched session, over the real Session shape. DERIVED: a real launch spawns
 *  a process and binds a port, which no hermetic test may do. */
const liveSession = (over: Record<string, unknown> = {}) => ({
  id: 'pv-1',
  deliverable: reviewDeliverableID,
  revision: 2,
  role: 'single',
  lane: 'dev-server',
  state: 'live',
  url: 'https://preview-pv-1.tail.invalid',
  backend_addr: '127.0.0.1:47901',
  pool_port: 47901,
  routed: true,
  user: 'alice',
  created_ts: '2026-07-20T09:04:00Z',
  ...over,
})

test('a backed launch links its served URL and offers the multi-port picker', async () => {
  const { view, log } = await review()
  log.set(`POST /api/deliverables/${reviewDeliverableID}/preview`, {
    body: liveSession({
      ports: [
        { number: 47901, label: 'dev server' },
        { number: 47902, label: 'docs' },
      ],
    }),
  })
  click(at(view, '[data-action="launch-preview"]'))
  await flush()

  const panel = at(view, '.preview-session')!
  expect(panel.getAttribute('data-backed')).toBe('true')
  expect((panel.querySelector('[data-action="open-preview"]') as HTMLAnchorElement).getAttribute('href')).toBe(
    'https://preview-pv-1.tail.invalid',
  )
  expect(all(view, '[data-port]').map((n) => n.getAttribute('data-port'))).toEqual(['47901', '47902'])
})

test('EVERY non-backed disposition renders its reason and NO iframe', async () => {
  // The five states the manager can answer with, each a 200 ANSWER with a reason.
  for (const state of ['no-preview', 'self-preview', 'requires-container-tier', 'unavailable', 'at-capacity']) {
    const { view, log } = await review()
    log.set(`POST /api/deliverables/${reviewDeliverableID}/preview`, {
      body: liveSession({ state, url: '', pool_port: 0, routed: false, reason: `the platform says: ${state}` }),
    })
    click(at(view, '[data-action="launch-preview"]'))
    await flush()

    const panel = at(view, '.preview-session')!
    expect(panel.getAttribute('data-preview-state')).toBe(state)
    expect(panel.getAttribute('data-backed')).toBe('false')
    expect(panel.textContent, `${state} did not render its served reason`).toContain(`the platform says: ${state}`)
    // The S13.8 failure by name: never a broken PREVIEW iframe. The RA-B1
    // rendered-document frame is a different, always-backed channel, so it is
    // excluded by its own attribute rather than pretended away.
    expect(
      view.container.querySelectorAll('iframe:not([data-result-doc])'),
      `${state} rendered a frame`,
    ).toHaveLength(0)
    view.unmount()
  }
})

test('the live-session list renders with stop, and a partial teardown stays retryable', async () => {
  const running = { sessions: [liveSession()], cursor: 62 }
  const { view, log } = await review(reviewDeliverableID, { 'GET /api/previews': { body: running } })
  expect(at(view, '[data-session="pv-1"]')).not.toBeNull()

  // The served detail is what renders — never "stopped" because a button was hit.
  log.set('POST /api/previews/pv-1/stop', {
    body: { session: 'pv-1', stopped: true, detail: 'stopped: the port is back in the pool' },
  })
  click(at(view, '[data-action="stop-preview"]'))
  await flush()
  expect(at(view, '[data-stop-detail="served"]')!.textContent).toContain('the port is back in the pool')

  // A partial teardown: the session is KEPT, so stopping again is safe and the
  // surface says so instead of claiming the stop completed.
  log.set('POST /api/previews/pv-1/stop', { status: 500, body: { error: 'internal', detail: 'route removal failed' } })
  click(at(view, '[data-action="stop-preview"]'))
  await flush()
  expect(at(view, '[data-stop-failed="true"]')!.textContent).toContain('stopping it again is safe')
})

// ── R9 / rubric 10: the dual frames ─────────────────────────────────────────

const sideView = (role: string, over: Record<string, unknown> = {}) => ({
  role,
  deliverable: reviewDeliverableID,
  revision: role === 'before' ? 1 : 2,
  url: `https://preview-${role}.tail.invalid`,
  state: 'live',
  ...over,
})

test('both frame srcs compose from the SERVED url plus the shared path, and re-sync re-asserts it', async () => {
  const { view, log } = await review()
  log.set(`POST /api/deliverables/${reviewDeliverableID}/preview/compare`, {
    body: {
      deliverable: reviewDeliverableID,
      before: sideView('before'),
      after: sideView('after'),
      single_instance: false,
      sync: { mode: 'path', enabled: true },
    },
  })
  click(at(view, '[data-action="launch-compare"]'))
  await flush()

  const srcs = () => all(view, 'iframe').map((n) => n.getAttribute('src'))
  expect(srcs()).toEqual(['https://preview-before.tail.invalid/', 'https://preview-after.tail.invalid/'])

  typeInto(at(view, '[data-field="frame-path"]') as HTMLInputElement, '/settings?tab=general')
  await flush()
  // Typing alone moves nothing: the shell APPLIES the path to both sides together.
  expect(srcs()).toEqual(['https://preview-before.tail.invalid/', 'https://preview-after.tail.invalid/'])
  click(at(view, '[data-action="apply-path"]'))
  await flush()
  expect(srcs(), 'the shared path did not reach both frames').toEqual([
    'https://preview-before.tail.invalid/settings?tab=general',
    'https://preview-after.tail.invalid/settings?tab=general',
  ])

  // The honest limit, stated on screen, plus the affordance that answers it.
  expect(text(view)).toContain('A click INSIDE a frame moves that frame alone')
  expect(text(view)).toContain('this page cannot see where a frame has navigated to')
  expect(at(view, '[data-action="resync"]'), 'no re-sync affordance exists').not.toBeNull()
  click(at(view, '[data-action="resync"]'))
  await flush()
  expect(srcs()[0]).toBe('https://preview-before.tail.invalid/settings?tab=general')
})

test('single-instance renders ONE frame, the served no-before reason, and sync disabled', async () => {
  const { view, log } = await review()
  log.set(`POST /api/deliverables/${reviewDeliverableID}/preview/compare`, {
    body: {
      deliverable: reviewDeliverableID,
      after: sideView('after'),
      single_instance: true,
      sync: { mode: 'path', enabled: false },
    },
  })
  click(at(view, '[data-action="launch-compare"]'))
  await flush()

  expect(view.container.querySelectorAll('iframe'), 'a second pane was faked').toHaveLength(1)
  expect(at(view, '.dual-frames')!.getAttribute('data-single-instance')).toBe('true')
  expect(at(view, '[data-sync-enabled]')!.getAttribute('data-sync-enabled')).toBe('false')
  expect(text(view)).toContain('no "before" exists')
  expect(text(view)).toContain('a second pane would be a fake')
})

test('frameSrc composes a base and a path and nothing else', () => {
  expect(frameSrc('https://p.invalid', '/')).toBe('https://p.invalid/')
  expect(frameSrc('https://p.invalid/', '/a/b?c=1')).toBe('https://p.invalid/a/b?c=1')
  // A path with no leading separator is rooted rather than concatenated blindly.
  expect(frameSrc('https://p.invalid', 'a')).toBe('https://p.invalid/a')
})

// ── R10 / rubric 11, 12, 13: the accept ─────────────────────────────────────

async function acceptCard(extra: Record<string, Scripted> = {}) {
  const { view, log } = await review(reviewDeliverableID, extra)
  click(at(view, '[data-action="read-accept-card"]'))
  await flush()
  return { view, log }
}

test('the accept card renders every served field, trailers byte-for-byte', async () => {
  const { view } = await acceptCard()
  const card = fixtures.acceptCard() as unknown as AcceptCard

  const rendered = at(view, '.accept-card')!
  expect(rendered.getAttribute('data-acceptable')).toBe('true')
  expect(rendered.textContent).toContain(card.content_pin)
  expect(rendered.textContent).toContain(card.protected_ref ?? '')
  expect(rendered.textContent).toContain(card.project_id)
  expect(rendered.textContent).toContain(card.tier_statement)
  expect(at(view, '[data-payload-hash]')!.getAttribute('data-payload-hash')).toBe(card.payload_hash)
  // Byte-for-byte: the exact string the commit will carry, not a rewording.
  expect((at(view, '[data-trailers="verbatim"]') as HTMLElement).textContent).toBe(card.trailers)
  // The provenance SOURCES, so a reader can see where each trailer input came from.
  for (const fact of [card.provenance.minting_run_id, card.provenance.engine, card.provenance.model, card.provenance.vendor_noreply]) {
    expect(rendered.textContent, `the card does not say where ${fact} came from`).toContain(fact ?? '')
  }
  expect(rendered.textContent).toContain(card.signing.statement)
})

test('acceptable:false renders the reason and NO act affordance', async () => {
  const closed = { ...(fixtures.acceptCard() as unknown as AcceptCard), acceptable: false, reason: 'this deliverable is accepted' }
  const { view } = await acceptCard({ [`GET /api/deliverables/${reviewDeliverableID}/accept-card`]: { body: closed } })
  expect(at(view, '[data-not-acceptable="true"]')!.textContent).toContain('this deliverable is accepted')
  expect(at(view, '.accept-form'), 'a closed accept still offered a form').toBeNull()
  // The facts are still there — a closed act is a reason, not a hidden card.
  expect(at(view, '[data-payload-hash]')).not.toBeNull()
})

test('the PIN rides the same request and is gone from the DOM afterwards', async () => {
  const { view, log } = await acceptCard()
  const card = fixtures.acceptCard() as unknown as AcceptCard
  typeInto(at(view, '[data-field="accept-pin"]') as HTMLInputElement, 'hunter2hunter')
  typeInto(at(view, '[data-field="subject"]') as HTMLInputElement, 'Ship the release page')
  await flush()

  log.set(`POST /api/deliverables/${reviewDeliverableID}/accept`, {
    body: {
      deliverable_id: reviewDeliverableID,
      applied: true,
      state: 'accepted',
      revision_n: 2,
      commit: 'abc1234',
      effect_id: 'e-accept',
      superseded: ['d-changelog'],
      routed_runs: ['r-sibling'],
      detail: 'accepted: one attributed commit is on the protected ref',
    },
  })
  click(at(view, '[data-action="accept"]'))
  await flush()

  const posted = log.calls.find((c) => c.path === `/api/deliverables/${reviewDeliverableID}/accept`)!
  const body = posted.body as { payload_hash: string; pin: string; subject: string }
  expect(body.pin, 'the PIN did not ride the accept request').toBe('hunter2hunter')
  expect(body.payload_hash, 'the accept did not quote the card it was shown for').toBe(card.payload_hash)
  expect(body.subject).toBe('Ship the release page')

  // Nowhere in the DOM afterwards: no stored elevation exists to keep it for.
  for (const input of all(view, 'input')) {
    expect((input as HTMLInputElement).value, 'the PIN survived the act').not.toBe('hunter2hunter')
  }
  const outcome = at(view, '.accept-outcome')!
  expect(outcome.getAttribute('data-applied')).toBe('true')
  expect(outcome.textContent).toContain('abc1234')
  expect(outcome.textContent).toContain('e-accept')
  expect(outcome.textContent).toContain('d-changelog')
  expect(outcome.textContent, 'the re-validation fan-out did not render').toContain('r-sibling')
})

test('a collision renders the merge card: three unanswerable options and nothing pushed', async () => {
  const { view, log } = await acceptCard()
  typeInto(at(view, '[data-field="accept-pin"]') as HTMLInputElement, 'hunter2hunter')
  await flush()
  log.set(`POST /api/deliverables/${reviewDeliverableID}/accept`, {
    body: {
      deliverable_id: reviewDeliverableID,
      applied: false,
      state: 'in-review',
      revision_n: 2,
      superseded: [],
      routed_runs: [],
      detail: 'not accepted: the candidate does not apply cleanly. The deliverable is still in review and nothing was pushed.',
      merge_card: {
        card: {
          deliverable_id: reviewDeliverableID,
          project_id: 'release-notes',
          onto: 'refs/heads/main',
          candidate: '9f8e7d6c',
          reason: 'applies-clean-collision',
          conflicts: 'site/release.tsx',
          options: ['agent-auto-resolve', 'resolve-manually', 'abort-to-new-attempt'],
        },
        options: [
          { option: 'agent-auto-resolve', answerable: false, reason: 'no executor at v0' },
          { option: 'resolve-manually', answerable: false, reason: 'no executor by design' },
          {
            option: 'abort-to-new-attempt',
            answerable: false,
            reason: 'no executor at v0, but the landed door reaches the same outcome',
            route: `POST /api/deliverables/${reviewDeliverableID}/follow-up`,
            preset: 'revision',
          },
        ],
        durability: 'v0 keeps no merge-card store',
      },
    },
  })
  click(at(view, '[data-action="accept"]'))
  await flush()

  expect(at(view, '.accept-outcome')!.getAttribute('data-applied')).toBe('false')
  expect(text(view)).toContain('nothing was pushed')
  const options = all(view, '[data-merge-option]')
  expect(options).toHaveLength(3)
  for (const o of options) {
    // Statements, never buttons: a control that 403s is the dead-control shape.
    expect(o.getAttribute('data-answerable')).toBe('false')
    expect(o.querySelectorAll('button'), 'an unanswerable option rendered a control').toHaveLength(0)
    expect((o.textContent ?? '').length).toBeGreaterThan(20)
  }
  expect(text(view), 'the third option does not name the landed door').toContain('/follow-up')
  expect(text(view)).toContain('preset revision')
  expect(at(view, '[data-durability="served"]')!.textContent).toContain('v0 keeps no merge-card store')
})

test('an already-accepted deliverable reads its outcome back as applied:false, not an error', async () => {
  const { view, log } = await acceptCard()
  typeInto(at(view, '[data-field="accept-pin"]') as HTMLInputElement, 'hunter2hunter')
  await flush()
  log.set(`POST /api/deliverables/${reviewDeliverableID}/accept`, {
    body: {
      deliverable_id: reviewDeliverableID,
      applied: false,
      state: 'accepted',
      revision_n: 2,
      commit: 'abc1234',
      effect_id: 'e-accept',
      superseded: [],
      routed_runs: [],
      detail: 'already accepted: the recorded outcome is returned and nothing fired again',
    },
  })
  click(at(view, '[data-action="accept"]'))
  await flush()
  const outcome = at(view, '.accept-outcome')!
  expect(outcome.getAttribute('data-applied')).toBe('false')
  expect(outcome.getAttribute('data-state')).toBe('accepted')
  expect(outcome.textContent).toContain('nothing fired again')
  expect(at(view, '.error'), 'a retry-safe read-back rendered as an error').toBeNull()
})

test('a superseded deliverable refuses with 409 and the refusal renders', async () => {
  const { view, log } = await acceptCard()
  typeInto(at(view, '[data-field="accept-pin"]') as HTMLInputElement, 'hunter2hunter')
  await flush()
  log.set(`POST /api/deliverables/${reviewDeliverableID}/accept`, {
    status: 409,
    body: { error: 'conflict', detail: 'this deliverable is superseded: a newer accepted version replaced it' },
  })
  click(at(view, '[data-action="accept"]'))
  await flush()
  expect(text(view)).toContain('this deliverable is superseded')
})

test('a stale pin re-renders from the CARRIED card with a change notice, and posts exactly once', async () => {
  const { view, log } = await acceptCard()
  typeInto(at(view, '[data-field="accept-pin"]') as HTMLInputElement, 'hunter2hunter')
  await flush()

  // The 409's `current` is an ACCEPT CARD, not an ApprovalItem — which is exactly
  // why this family needs its own stale reader.
  const fresh = { ...(fixtures.acceptCard() as unknown as AcceptCard), payload_hash: 'freshhash', revision_n: 3 }
  log.set(`POST /api/deliverables/${reviewDeliverableID}/accept`, {
    status: 409,
    body: { error: 'stale_payload', detail: 'the accept card moved since you read it', current: fresh },
  })
  click(at(view, '[data-action="accept"]'))
  await flush()

  expect(at(view, '[data-stale-accept="true"]')!.textContent).toContain('nothing was accepted')
  expect(at(view, '[data-payload-hash]')!.getAttribute('data-payload-hash')).toBe('freshhash')
  expect(text(view), 'the fresh card did not re-render').toContain('3')
  expect(
    log.calls.filter((c) => c.path === `/api/deliverables/${reviewDeliverableID}/accept`),
    'a stale pin was blindly retried',
  ).toHaveLength(1)
})

test('a missing or wrong PIN re-prompts, and nothing was accepted', async () => {
  const { view, log } = await acceptCard()
  log.set(`POST /api/deliverables/${reviewDeliverableID}/accept`, {
    status: 401,
    body: { error: 'pin_required', detail: 'this action needs your PIN in the same request' },
  })
  click(at(view, '[data-action="accept"]'))
  await flush()
  const refused = at(view, '[data-pin-refused="true"]')!
  expect(refused.textContent).toContain('this action needs your PIN')
  expect(refused.textContent).toContain('nothing was accepted')
  // The form is still there to re-enter into.
  expect(at(view, '[data-field="accept-pin"]')).not.toBeNull()
})

test('presentation follows AUTHORSHIP: the owner gets the form, a non-owner gets the card', async () => {
  // The owner: the form renders.
  const owner = await acceptCard()
  expect(at(owner.view, '.accept-form'), 'the owner has no accept form').not.toBeNull()
  expect(at(owner.view, '[data-authorship="not-mine"]')).toBeNull()
  owner.view.unmount()

  // A non-owner reads the SAME card and cannot act on it. `acceptable` carries no
  // caller limb — the operator genuinely sees a card that says the act is open —
  // so this is presentation, and the line says whose act it is.
  const other = await acceptCard({ 'GET /api/auth/session': asBob })
  expect(at(other.view, '.accept-card'), 'a non-owner cannot even read the card').not.toBeNull()
  expect(at(other.view, '.accept-form'), 'a non-owner was offered the accept form').toBeNull()
  const line = at(other.view, '[data-authorship="not-mine"]')!
  expect(line.textContent).toContain('alice')
  expect(line.textContent).toContain("only they can do it")
})

test('the SERVER is the authority: a non-owner accept is refused and the refusal renders', async () => {
  // Driven through the verb rather than only asserted about the UI: the 403 is what
  // actually stops a non-owner, and the surface renders it as the answer it is.
  const { view, log } = await acceptCard()
  log.set(`POST /api/deliverables/${reviewDeliverableID}/accept`, {
    status: 403,
    body: { error: 'forbidden', detail: 'the accept is the owner’s own outward act' },
  })
  typeInto(at(view, '[data-field="accept-pin"]') as HTMLInputElement, 'hunter2hunter')
  await flush()
  click(at(view, '[data-action="accept"]'))
  await flush()
  expect(text(view)).toContain('own outward act')
  expect(at(view, '.accept-outcome'), 'a refused accept rendered an outcome').toBeNull()
})

// ── R17 / rubric 20: live wiring ────────────────────────────────────────────

test('a declared frame triggers a re-read and an undeclared one reaches nothing', async () => {
  const { log } = await review()
  const source = FakeSource.last()
  const before = log.calls.filter((c) => c.path === `/api/deliverables/${reviewDeliverableID}`).length

  await flush()
  source.send('artifact.produced', { seq: 70, type: 'artifact.produced', topics: ['board'], payload: {} })
  await flush()
  const after = log.calls.filter((c) => c.path === `/api/deliverables/${reviewDeliverableID}`).length
  expect(after, 'a declared frame did not trigger a re-read').toBeGreaterThan(before)

  source.send('meter.utilization', { seq: 71, type: 'meter.utilization', topics: ['meters'], payload: {} })
  await flush()
  expect(
    log.calls.filter((c) => c.path === `/api/deliverables/${reviewDeliverableID}`).length,
    'an undeclared type reached this view',
  ).toBe(after)
})

test('after an own verb the view re-reads', async () => {
  const { view, log } = await acceptCard()
  const before = log.calls.filter((c) => c.path === `/api/deliverables/${reviewDeliverableID}`).length
  typeInto(at(view, '[data-field="accept-pin"]') as HTMLInputElement, 'hunter2hunter')
  await flush()
  log.set(`POST /api/deliverables/${reviewDeliverableID}/accept`, {
    body: {
      deliverable_id: reviewDeliverableID,
      applied: true,
      state: 'accepted',
      revision_n: 2,
      superseded: [],
      routed_runs: [],
      detail: 'accepted',
    },
  })
  click(at(view, '[data-action="accept"]'))
  await flush()
  expect(
    log.calls.filter((c) => c.path === `/api/deliverables/${reviewDeliverableID}`).length,
    'the surface did not re-read after its own act',
  ).toBeGreaterThan(before)
})

test('the surface declares the types it consumes, each against the registry', async () => {
  const { deliverableEventTypes } = await import('./live')
  // The registry's own names, and nothing invented: every one of these appears in
  // a landed set or is minted by a landed producer.
  expect(deliverableEventTypes).toContain('artifact.produced')
  expect(deliverableEventTypes).toContain('review.comment')
  expect(deliverableEventTypes).toContain('review.drained')
  expect(deliverableEventTypes).toContain('deliverable.accepted')
  expect(deliverableEventTypes).toContain('preview.started')
  expect(deliverableEventTypes).toContain('preview.stopped')
  expect(new Set(deliverableEventTypes).size, 'a type is declared twice').toBe(deliverableEventTypes.length)
})

// ── R16 / rubric 19: the iframe src composition ─────────────────────────────

test('an iframe src can only come from a served preview URL or a blob of revision bytes', async () => {
  const src = (await import('./Deliverable.tsx?raw')).default as string
  // Exactly THREE frames exist since the polish round, all in this one file:
  // the preview (src composed from a served session URL + the shell path),
  // the rendered-document view (src a blob: URL minted from same-origin
  // revision bytes, sandbox granted NOTHING) and the hidden MEASURING frame
  // (the SAME blob URL, sandbox allow-same-origin only, scripts withheld —
  // the escape scan pins both grant lists by name).
  expect(src.match(/<iframe/g), 'a frame beyond the preview, the rendered document and its measurer exists').toHaveLength(3)
  expect(src, 'the measuring frame must ride the same minted blob URL').toContain('sandbox={measureSandbox}')
  expect(src).toContain('src={frameSrc(side.url, path)}')
  expect(src, 'the document frame must ride the blob URL the composition minted').toContain(
    'URL.createObjectURL(new Blob([composed.html]',
  )
  expect(src, 'the document frame must carry the empty grant list').toContain('sandbox={documentSandbox}')
  // Nothing that could carry content reaches the PREVIEW frame: no comment
  // body, no diff text, no label, no trailer.
  for (const banned of ['frameSrc(cmp', 'frameSrc(comment', 'frameSrc(card', 'frameSrc(detail']) {
    expect(src, `${banned} would put content into a frame src`).not.toContain(banned)
  }
})

test('a hostile comment body and diff line render as TEXT, not as elements', async () => {
  const hostile = fixtures.placedComments() as unknown as PlacedComments
  hostile.comments[1] = {
    ...hostile.comments[1],
    body: '<img src=x onerror="alert(1)"> and <script>alert(2)</script>',
  }
  const { view } = await review(reviewDeliverableID, {
    [`GET /api/deliverables/${reviewDeliverableID}/comments?revision=2`]: { body: hostile },
  })
  expect(text(view)).toContain('<img src=x onerror="alert(1)">')
  expect(view.container.querySelectorAll('script'), 'a planted script became an element').toHaveLength(0)
  expect(
    [...view.container.querySelectorAll('img')].filter((n) => n.getAttribute('src') === 'x'),
    'a planted img became an element',
  ).toHaveLength(0)
})

// ── drain r1 ────────────────────────────────────────────────────────────────

test('D1: the preview frame is sandboxed, without top-navigation, and leaks no referrer', async () => {
  const { view, log } = await review()
  log.set(`POST /api/deliverables/${reviewDeliverableID}/preview/compare`, {
    body: {
      deliverable: reviewDeliverableID,
      before: sideView('before'),
      after: sideView('after'),
      single_instance: false,
      sync: { mode: 'path', enabled: true },
    },
  })
  click(at(view, '[data-action="launch-compare"]'))
  await flush()

  const frames = all(view, 'iframe')
  expect(frames).toHaveLength(2)
  for (const frame of frames) {
    // S13.3 names this channel "the sandboxed rendered-document view" and
    // review.EscapeFirst records it under that name, so the attribute is the
    // contract rather than a preference.
    const tokens = (frame.getAttribute('sandbox') ?? '').split(/\s+/).filter((t) => t !== '')
    expect(tokens.length, 'the preview frame ships with no sandbox at all').toBeGreaterThan(0)
    expect(tokens.sort()).toEqual(['allow-forms', 'allow-same-origin', 'allow-scripts'].concat(['allow-popups']).sort())
    // The withheld capability that matters: a framed, model-produced app must not
    // be able to move the window this PIN is typed into.
    for (const banned of [
      'allow-top-navigation',
      'allow-top-navigation-by-user-activation',
      'allow-top-navigation-to-custom-protocols',
      'allow-popups-to-escape-sandbox',
      'allow-modals',
      'allow-downloads',
    ]) {
      expect(tokens, `the frame grants ${banned}`).not.toContain(banned)
    }
    expect(frame.getAttribute('referrerpolicy')).toBe('no-referrer')
  }
})

test('D2/D3: EVERY served comment is reachable as a widget or present on the strip', async () => {
  const { view } = await review()
  const served = fixtures.placedComments() as unknown as PlacedComments
  expect(served.comments.length, 'the body carries no comments, so the invariant is untested').toBeGreaterThan(4)

  // Computed from what RENDERED, never from a re-derivation of the production
  // predicate — which is how the first version of this test passed over a real
  // gap: it recomputed the strip filter and compared the implementation to a copy
  // of itself.
  const inWidgets = new Set(all(view, '.diff-widget [data-comment]').map((n) => n.getAttribute('data-comment')))
  const onStrip = new Set(all(view, '[data-strip-comment]').map((n) => n.getAttribute('data-strip-comment')))
  expect(inWidgets.size, 'no comment rendered inside the diff, so one half is untested').toBeGreaterThan(0)
  expect(onStrip.size, 'nothing rendered on the strip, so the other half is untested').toBeGreaterThan(0)

  for (const c of served.comments) {
    const id = String(c.id)
    expect(
      inWidgets.has(id) || onStrip.has(id),
      `comment ${id} renders in NEITHER a diff widget nor the strip — it exists only in the flat list`,
    ).toBe(true)
  }
  // And the two are complements, not overlapping sets: a comment in both would mean
  // the strip is showing something that already has a place.
  for (const id of inWidgets) {
    expect(onStrip.has(id ?? ''), `comment ${id} is both anchored and on the strip`).toBe(false)
  }
})

test('review #2: no comment renders twice in the block below the diff', async () => {
  // The ×4 sighting (design review 2026-08-17): the flat list held EVERY
  // comment while the strip re-held the unplaced ones, so each unplaced
  // comment rendered twice on one screen — and ×2 data made it ×4. The list
  // and the strip are complements now: one card per served comment down there.
  const { view } = await review()
  const served = fixtures.placedComments() as unknown as PlacedComments
  for (const c of served.comments) {
    const cards = all(view, `.comments [data-comment="${String(c.id)}"]`)
    expect(cards.length, `comment ${String(c.id)} renders ${String(cards.length)} times below the diff`).toBe(1)
  }
})

test('D2: a placement whose line is outside every rendered hunk lands on the strip', async () => {
  // The exact shape that used to vanish: a real file, a non-zero line, and no
  // rendered hunk containing it.
  const body = fixtures.placedComments() as unknown as PlacedComments
  const target = body.placements.find((p) => p.anchor !== undefined && p.anchor.line_no !== 0)!
  target.anchor = { ...target.anchor!, line_no: 9999 }
  const { view } = await review(reviewDeliverableID, {
    [`GET /api/deliverables/${reviewDeliverableID}/comments?revision=2`]: { body },
  })
  expect(
    at(view, `[data-strip-comment="${target.comment_id}"]`),
    'an unreachable anchor rendered nowhere but the flat list',
  ).not.toBeNull()
  expect(at(view, `.diff-widget [data-comment="${target.comment_id}"]`)).toBeNull()
})

test('D4: the guidance rides the ANSWER in one request, and a refusal writes nothing', async () => {
  const { view, log } = await review(reworkDeliverableID)
  const door = (fixtures.deliverableRework() as { doors: { verb: string; route: string; payload_hash?: string }[] }).doors.find(
    (d) => d.verb === 'request-revision',
  )!

  // Empty guidance arms nothing: the card's own answer schema refuses an empty
  // list, so a control that could send one would only earn a 400.
  expect((at(view, '[data-action="request-revision"]') as HTMLButtonElement).disabled).toBe(true)

  typeInto(at(view, '[data-field="guidance"]') as HTMLTextAreaElement, 'Tighten the summary to two sentences.')
  await flush()
  log.set(`POST ${door.route}`, {
    status: 409,
    body: { error: 'stale_payload', detail: 'the card moved', current: {} },
  })
  click(at(view, '[data-action="request-revision"]'))
  await flush(8)

  // ONE request, and it is the answer — no separate comment write exists to
  // survive the refusal or to duplicate on a retry.
  expect(
    log.calls.filter((c) => c.method === 'POST' && c.path.endsWith('/comments')),
    'a durable comment was written outside the answer',
  ).toHaveLength(0)
  const posted = log.calls.filter((c) => c.path === door.route)
  expect(posted).toHaveLength(1)
  const answer = (posted[0].body as { answer: { choice: string; guidance: { text: string }[] } }).answer
  expect(answer.guidance, 'the answer carried no guidance, which the validator refuses').toHaveLength(1)
  expect(answer.guidance[0].text).toBe('Tighten the summary to two sentences.')

  // The failure says what was and was not written, and a second press still sends
  // exactly one request — nothing accumulated.
  expect(at(view, '[data-stale="request-revision"]')!.textContent).toContain('NOTHING was written')
  click(at(view, '[data-action="request-revision"]'))
  await flush(8)
  expect(log.calls.filter((c) => c.method === 'POST' && c.path.endsWith('/comments'))).toHaveLength(0)
})

/** ONE mount of a `useLive` hook on this page costs TWO calls, and the 2 is not
 *  this view's doing. CONVENTIONS §44-B (drain r1, D7) pins the rule as landed
 *  machinery WITH its own demonstration: the FIRST subscriber opens the source and
 *  reads once, and every LATER one JOINS — `EventStream.subscribe` hands a joiner
 *  an immediate synchronous `onResnapshot('connected')` (`events.ts`), and
 *  `useLive`'s debt rule (only a read that STARTED AFTER the signal clears it)
 *  then forces a second. Child-ness is NOT the variable, which is what the earlier
 *  recording of this measurement got wrong: on `/chat`, two SIBLING hooks in one
 *  component read once and twice respectively. This page's detail hook is the
 *  opener, so every other hook here — previews, comments — is a joiner at two.
 *
 *  A literal is the right pin BECAUSE §44-B already proves it. The earlier form
 *  compared the comments count to a live `/api/previews` count, which had a real
 *  blind spot: a second previews subscriber would make it 4 === 4 and pass
 *  silently, and any change to the try-it block failed this test while naming the
 *  comment feed as the culprit. The defect these two tests exist for is a SECOND
 *  comments mount, which costs four against this two. */
const oneMountedRead = 2

test('D5: the comments read is mounted ONCE, at the §44-B joiner cost', async () => {
  const { log } = await review()
  const comments = log.calls.filter((c) => c.path.includes('/comments')).length
  expect(
    comments,
    `the comments read is mounted more than once: ${comments} calls where one subscribed mount costs ${oneMountedRead}`,
  ).toBe(oneMountedRead)
})

test('D5: an object surface still owns its own comments read', async () => {
  const { log } = await review(imageDeliverableID)
  const comments = log.calls.filter((c) => c.path.includes('/comments')).length
  expect(comments, 'a surface with no diff of its own did not read comments exactly once').toBe(oneMountedRead)
})

test('D11: the object surfaces render the real comment state, not an error', async () => {
  for (const id of [imageDeliverableID, binaryDeliverableID]) {
    const { view } = await review(id)
    expect(
      text(view),
      `${id} renders the comment block in its error state — the read was unscripted and the throw was swallowed`,
    ).not.toContain('The control plane is unreachable')
    expect(at(view, '.comment-list'), `${id} rendered no comments`).not.toBeNull()
  }
})

test('D6: a diff the parser rejects renders the honest failure, not a dead surface', async () => {
  // The shape this packet's own deviation was ABOUT: a served body this parser
  // throws on. Fixing the producer was necessary and not sufficient.
  // The EXACT pre-fix shape: a separator line before a real `diff --git` block,
  // which is what the producer used to emit and what makes gitdiff-parser throw.
  const served = fixtures.compareLineDiff() as unknown as Comparison
  const broken = { ...served, unified: `=== site/release.tsx ===\n${served.unified ?? ''}` }
  const { view } = await review(reviewDeliverableID, {
    [`GET /api/deliverables/${reviewDeliverableID}/compare`]: { body: broken },
  })
  const block = at(view, '[data-diff-unreadable="true"]')
  expect(block, 'a malformed diff took the surface down instead of rendering a reason').not.toBeNull()
  expect(block!.textContent).toContain('could not be read as a unified diff')
  // The text is still readable, as text, so a person can say what is wrong with it.
  expect(block!.textContent).toContain('=== site/release.tsx ===')
  // And the rest of the page still works — the doors and the accept are untouched.
  expect(at(view, '[data-door="accept"]')).not.toBeNull()
})

test('D14: answerAtDoor refuses a route that escapes the machine surface', async () => {
  const { apiPath } = await import('./api')
  expect(apiPath('/api/approvals/ask:x/answer')).toBe(true)
  // The forms the prefix test admitted and the comment did not claim.
  expect(apiPath('/api/../evil'), 'a traversal that leaves /api/ was admitted').toBe(false)
  expect(apiPath('//evil.invalid/api/x'), 'a protocol-relative host was admitted').toBe(false)
  expect(apiPath('https://evil.invalid/api/x')).toBe(false)
  expect(apiPath('/apiary/x')).toBe(false)
  // The ENCODED forms (drain r2 R7). `new URL` does not decode `%2f`, so these
  // resolved to a path still under /api/ while Go's mux — which unescapes and then
  // cleans — routes them to /evil. The claim is about the result, so the result is
  // what is checked.
  expect(apiPath('/api/..%2fevil'), 'an encoded traversal was admitted').toBe(false)
  expect(apiPath('/api/..%2Fevil'), 'an encoded traversal was admitted').toBe(false)
  expect(apiPath('/api/%2e%2e/evil'), 'an encoded dot-dot was admitted').toBe(false)
  expect(apiPath('/api/%2e%2e%2fevil'), 'a fully encoded traversal was admitted').toBe(false)
  // An encoded DOUBLE separator is admitted, and that is the right answer rather
  // than a leftover: decoded it is `/api///evil.invalid/x`, which Go's mux cleans to
  // `/api/evil.invalid/x` — still on the machine surface, so it is a route that does
  // not exist rather than a route that escapes. Only the `..` families leave.
  expect(apiPath('/api/%2f%2fevil.invalid/x')).toBe(true)
  // A malformed escape is refused rather than thrown out of the caller.
  expect(apiPath('/api/%zz'), 'a malformed escape must be refused, not raised').toBe(false)
  // And an ordinary encoded segment still passes — a composite ask id carries a
  // colon, and an encoded one must not become a refusal.
  expect(apiPath('/api/approvals/ask%3Ax/answer'), 'an ordinary encoded segment was refused').toBe(true)
})

// ── the S13.9 follow-up spawn: the fourteenth D3 control (P3-UI-4) ────────

/** The accepted deliverable, which is where BOTH follow-up limbs are open: the
 *  plain `follow-up` door with no preset, and the `request-revision` door's
 *  finished limb carrying the platform's `revision` framing. */
const spawnRoutes = (over: Record<string, Scripted> = {}) => ({
  ...reviewRoutes(),
  'GET /api/deliverables/d-notes': { body: fixtures.deliverableDetail() },
  ...over,
})

const spawned = {
  task_id: 't-followup-1',
  owner: 'alice',
  title: 'Follow up on d-notes',
  deliverable_id: 'd-notes',
  revision_n: 1,
  preset: 'revision',
}

async function accepted(extra: Record<string, Scripted> = {}) {
  const routes = spawnRoutes(extra)
  const log = scriptedFetch(routes)
  window.history.replaceState(null, '', '/deliverables/d-notes')
  const view = mount(<App stream={inertStream()} />)
  await flush()
  return { view, log }
}

const spawnControls = (view: { container: HTMLElement }) => all(view, '[data-control="follow-up"]')

test('the control renders on every door that SUPPLIES the spawn, and on no other', async () => {
  const { view } = await accepted()
  const served = fixtures.deliverableDetail() as unknown as { doors: { verb: string; route: string }[] }
  const driveable = served.doors.filter((d) => d.route.endsWith('/follow-up'))
  expect(driveable.length, 'the served body no longer opens the spawn').toBeGreaterThan(1)

  // Gated on what the door SUPPLIES, not on a verb name — drain r1 D4's rule,
  // applied to the other verb. Both served limbs reach the same landed route.
  expect(spawnControls(view)).toHaveLength(driveable.length)
  for (const d of driveable) {
    expect(at(view, `[data-door="${d.verb}"] [data-control="follow-up"]`), `${d.verb} supplies the spawn and offers no control`).not.toBeNull()
  }
  // And a door that does NOT supply it offers nothing, however available it is.
  expect(at(view, '[data-door="accept"] [data-control="follow-up"]')).toBeNull()
  expect(at(view, '[data-door="preview"] [data-control="follow-up"]')).toBeNull()
  view.unmount()
})

test('the spawn sends the DOOR’s own preset and the person’s own ask, and nothing composed here', async () => {
  const { view, log } = await accepted({
    'POST /api/deliverables/d-notes/follow-up': { body: spawned },
  })
  const control = at(view, '[data-door="request-revision"] [data-control="follow-up"]')!
  typeInto(control.querySelector('[data-field="follow-up-objective"]') as HTMLTextAreaElement, 'Tighten the intro')
  click(control.querySelector('[data-open="follow-up"]'))
  await flush()

  // CONFIRM BEFORE FIRE, and it says what a follow-up IS — a new task of your
  // own — and what it explicitly does not do to the work in front of you.
  const dialog = document.querySelector('[role="dialog"]')!
  expect(dialog, 'the spawn fired with no confirm').not.toBeNull()
  expect(dialog.textContent).toContain('NEW task')
  expect(dialog.textContent).toContain('revision framing')
  expect(dialog.textContent, 'the confirm claims the deliverable changes').toContain('changes nothing about this deliverable')
  // It overclaims nothing: no deletion, no rollback, no replaced revision.
  for (const word of ['deleted', 'rolled back', 'replaces this', 'undone']) {
    expect(dialog.textContent, `the confirm claims a "${word}" the verb does not do`).not.toContain(word)
  }
  const before = log.calls.length
  click(dialog.querySelector('[data-act="confirm"]'))
  await flush()

  const sent = log.calls.find((c) => c.path === '/api/deliverables/d-notes/follow-up')!
  expect(sent, 'the confirm fired nothing').toBeDefined()
  expect(sent.method).toBe('POST')
  // The preset is the door's own (`revision`, deliverables.go:350) and NO
  // revision is sent: the verb defaults to the current one, which is the honest
  // reading of "follow up on this deliverable".
  expect(sent.body).toEqual({ preset: 'revision', objective: 'Tighten the intro' })
  expect(log.calls.length).toBeGreaterThan(before)

  // The answer renders as served: the task it opened, its owner and the revision
  // it linked to — and the RE-READ fires unconditionally after the act.
  const outcome = at(view, '[data-door="request-revision"] [data-outcome]')!
  expect(outcome.getAttribute('data-outcome')).toBe('applied')
  expect(outcome.textContent).toContain(spawned.task_id)
  expect(outcome.textContent).toContain(String(spawned.revision_n))
  expect(outcome.textContent).toContain(spawned.title)
  expect(log.calls.filter((c) => c.path === '/api/deliverables/d-notes').length, 'the act did not re-read').toBeGreaterThan(1)
  view.unmount()
})

test('the plain door sends its own EMPTY preset rather than borrowing the other limb’s', async () => {
  const { view, log } = await accepted({
    'POST /api/deliverables/d-notes/follow-up': { body: { ...spawned, preset: '' } },
  })
  const control = at(view, '[data-door="follow-up"] [data-control="follow-up"]')!
  typeInto(control.querySelector('[data-field="follow-up-objective"]') as HTMLTextAreaElement, 'A companion piece')
  click(control.querySelector('[data-open="follow-up"]'))
  await flush()
  // The plain door names no framing, so the confirm claims none either.
  expect(document.querySelector('[role="dialog"]')!.textContent).not.toContain('framing')
  click(document.querySelector('[data-act="confirm"]'))
  await flush()
  // `""` is a landed preset the verb admits (internal/api/actions.go:102–108),
  // so this is the platform's own plain follow-up rather than an omission.
  expect(log.calls.find((c) => c.path === '/api/deliverables/d-notes/follow-up')!.body).toEqual({
    preset: '',
    objective: 'A companion piece',
  })
  view.unmount()
})

test('a follow-up with no ask is HELD at the form and fires nothing; busy is a different state', async () => {
  const { view, log } = await accepted({
    'POST /api/deliverables/d-notes/follow-up': { body: spawned },
  })
  const control = at(view, '[data-door="follow-up"] [data-control="follow-up"]')!
  const button = control.querySelector('[data-open="follow-up"]')!
  expect(button.hasAttribute('disabled'), 'an empty follow-up offers a fire').toBe(true)
  // HELD IS NOT BUSY (§49 N7): nothing is in flight and the form says why.
  expect(button.getAttribute('data-busy')).toBe('false')
  expect(control.querySelector('[data-held="follow-up"]')?.textContent).toContain('needs its own ask')
  const before = log.calls.length
  click(button)
  await flush()
  expect(document.querySelector('[role="dialog"]'), 'a held control opened its confirm').toBeNull()
  expect(log.calls.length, 'a held control reached the control plane').toBe(before)
  view.unmount()
})

test('the operator spawns a follow-up on somebody else’s deliverable, and the task is the OPERATOR’s', async () => {
  // Authority is the verb's own: `authorizeOwner` admits the owner and the
  // operator for any row (internal/api/reads.go:444–452). The successor belongs
  // to whoever ASKED (actions.go:206–210), so an operator's follow-up on alice's
  // deliverable is the operator's own task — asserted from the served answer,
  // because no client may decide whose work it is.
  const { view, log } = await accepted({
    'GET /api/auth/session': signedInAsOperator,
    'POST /api/deliverables/d-notes/follow-up': { body: { ...spawned, owner: 'op', task_id: 't-op-follow' } },
  })
  const control = at(view, '[data-door="follow-up"] [data-control="follow-up"]')!
  typeInto(control.querySelector('[data-field="follow-up-objective"]') as HTMLTextAreaElement, 'Check the numbers')
  click(control.querySelector('[data-open="follow-up"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  expect(log.calls.some((c) => c.method === 'POST' && c.path === '/api/deliverables/d-notes/follow-up')).toBe(true)
  const outcome = at(view, '[data-door="follow-up"] [data-outcome]')!
  expect(outcome.getAttribute('data-outcome')).toBe('applied')
  expect(outcome.textContent, 'the spawn claimed the wrong owner').toContain('op')
  expect(outcome.textContent).toContain('t-op-follow')
  view.unmount()
})

test('a refusal renders the server’s own words and is not dressed as anything else', async () => {
  // The other authority direction. `authorizeOwner`'s 403 is the bare word
  // `forbidden` (reads.go:444–452) and its 404 is `deliverable not found`,
  // 404-before-403 so an unknown id cannot become an existence oracle. Both are
  // transcribed rather than paraphrased.
  const { view } = await accepted({
    'POST /api/deliverables/d-notes/follow-up': { status: 403, body: { error: 'forbidden', detail: 'forbidden' } },
  })
  const control = at(view, '[data-door="follow-up"] [data-control="follow-up"]')!
  typeInto(control.querySelector('[data-field="follow-up-objective"]') as HTMLTextAreaElement, 'Not mine to ask')
  click(control.querySelector('[data-open="follow-up"]'))
  await flush()
  click(document.querySelector('[data-act="confirm"]'))
  await flush()

  const outcome = at(view, '[data-door="follow-up"] [data-outcome]')!
  expect(outcome.getAttribute('data-outcome')).toBe('failed')
  expect(outcome.textContent).toContain('forbidden')
  // A refusal is not a no-op and not a re-read prompt, and it invents no task.
  expect(outcome.textContent).not.toContain('opened')
  expect(outcome.textContent).not.toContain('re-read it and decide again')
  view.unmount()
})

test('the control is reachable and operable at 375px', async () => {
  const { view } = await accepted({ 'POST /api/deliverables/d-notes/follow-up': { body: spawned } })
  const control = at(view, '[data-control="follow-up"]')!
  typeInto(control.querySelector('[data-field="follow-up-objective"]') as HTMLTextAreaElement, 'x')
  click(control.querySelector('[data-open="follow-up"]'))
  await flush()
  const dialog = document.querySelector('[role="dialog"]')!
  const pinned = /w-\[\d+px\]|min-w-\[\d+px\]/
  const scope = [...control.querySelectorAll('*'), control, ...dialog.querySelectorAll('*'), dialog]
  expect(scope.length, 'the phone-width scan reached almost nothing').toBeGreaterThan(8)
  const fixed = scope.filter((n) => pinned.test(n.className.toString()) || /\d+px/.test(n.getAttribute('style') ?? ''))
  expect(fixed.map((n) => n.className.toString()), 'a control pins a pixel width a phone cannot fit').toEqual([])
  expect(pinned.test('w-[520px] flex')).toBe(true)
  // And it is genuinely operable from there.
  click(dialog.querySelector('[data-act="confirm"]'))
  await flush()
  expect(at(view, '[data-outcome]')?.getAttribute('data-outcome')).toBe('applied')
  view.unmount()
})

// ── P3-UI-7 seam C-2: the restyle's own pins ────────────────────────────────
//
// Everything above this line passed the restyle UNMODIFIED — no landed
// predicate was re-pointed, re-worded or weakened — which is the honest form of
// "presentation moved and behaviour did not". What follows is the new work: the
// R11 head, the R12 teaching states with their served gates, and a standing
// guard for the carried cascade hazard.

test('R11: the head says what the surface is, and its overclaims are absent', async () => {
  const { view } = await review()
  const what = at(view, '[data-surface-what]')!
  const line = what.textContent ?? ''

  // The five verbs this surface really has, each of which is driven by a landed
  // test above: the revision lineage, the round-over-round comparison, the
  // anchored comment loop, try-it, and the one accept.
  for (const fact of ['revision', 'compare', 'comment', 'try', 'accept']) {
    expect(line.toLowerCase(), `the head does not name ${fact}`).toContain(fact)
  }
  // And the fact that makes the accept worth warning about at all.
  expect(line).toContain('your own credentials')

  // THE OVERCLAIMS. Approval state is served PER ARTIFACT (§38 ruling (a)), so
  // this page may not call anything approved or signed off; and no verb at any
  // shape un-accepts a revision, so it may not offer one.
  for (const banned of ['approved', 'signed off', 'sign-off', 'reversible', 'undo', 'revert', 'roll back']) {
    expect(line.toLowerCase(), `the head claims "${banned}"`).not.toContain(banned)
  }
  // Non-vacuity: the negatives run over a line that really exists and really
  // says something, so a missing head could not pass them.
  expect(line.length, 'the head line is empty, so the negatives above prove nothing').toBeGreaterThan(120)
  view.unmount()
})

test('R12: the comment feed teaches only AFTER it has answered, never during', async () => {
  // THE D1 SHAPE (§51 drain D1). `comments` is `feed.data?.comments ?? []`, so
  // without the gate a read still in flight renders as a served empty and
  // teaches "nobody has commented" about a question nobody has answered.
  const pending = await review(reviewDeliverableID, {
    'GET /api/deliverables/d-site/comments?revision=2': { pending: true },
  })
  expect(text(pending.view)).not.toContain('Nobody has commented on this revision yet.')
  expect(
    text(pending.view),
    'the strip spoke about comments the platform had not sent',
  ).not.toContain('Every comment on this revision has a live position')
  // The surface really IS mounted and really HAS asked — otherwise the negative
  // would be looking at an empty page (the instrument §51 D1 had to fix).
  expect(at(pending.view, '.comments'), 'the comment block never mounted').not.toBeNull()
  expect(
    pending.log.calls.some((c) => c.path.includes('/comments?revision=2')),
    'the comment read never fired, so nothing was pending',
  ).toBe(true)
  pending.view.unmount()

  // Once it answers EMPTY, the teaching state is exactly what appears — and it
  // teaches what makes a row show up rather than only admitting there are none.
  const empty = await review(reviewDeliverableID, {
    'GET /api/deliverables/d-site/comments?revision=2': { body: { comments: [], placements: [] } },
  })
  expect(text(empty.view)).toContain('Nobody has commented on this revision yet.')
  expect(text(empty.view), 'the empty state teaches nothing').toContain('anchors a comment to it')
  empty.view.unmount()
})

test('R12: the preview-session list teaches only AFTER /api/previews has answered', async () => {
  const pending = await review(reviewDeliverableID, { 'GET /api/previews': { pending: true } })
  expect(text(pending.view)).not.toContain('No preview session is running.')
  expect(at(pending.view, '.live-previews'), 'the session block never mounted').not.toBeNull()
  pending.view.unmount()

  const empty = await review(reviewDeliverableID, { 'GET /api/previews': { body: { sessions: [] } } })
  expect(text(empty.view)).toContain('No preview session is running.')
  expect(text(empty.view), 'the empty state teaches nothing').toContain('Launching a preview above puts it here')
  empty.view.unmount()
})

test('R12: the served-empty revision and door lists teach what makes a row appear', async () => {
  const detail = fixtures.deliverableReview() as { revisions: unknown[]; doors: unknown[] }
  const { view } = await review(reviewDeliverableID, {
    'GET /api/deliverables/d-site': { body: { ...detail, revisions: [], doors: [] } },
  })
  expect(text(view)).toContain('No revision is minted yet, so there is nothing to review.')
  expect(text(view), 'the revision empty teaches nothing').toContain('when a run mints one')
  expect(text(view)).toContain('The platform named no doors on this deliverable.')
  expect(text(view), 'the door empty teaches nothing').toContain('actually let you through it')
  view.unmount()
})

test('⚠ the carried cascade hazard: no kept owned rule competes with a utility on the same element', async () => {
  // THE MECHANISM, stated so the next reader does not have to re-derive it.
  // Since the cascade fix the owned sheet sits in `@layer base`, so an owned rule
  // — media-gated or not — LOSES to `@layer utilities`. `.chat-verbs` is the
  // instance that shipped (§53 A2): the moment a utility arrived, the owned
  // wide-screen arm went DEAD and no jsdom test in the tree could see it.
  //
  // The review surface answers the same question the other way round: these
  // seven elements KEEP their owned rules, so they must carry NO layout utility.
  // Both halves are asserted, because either alone passes for the wrong reason.
  const wide = cssSource.slice(cssSource.indexOf('@media (min-width:'))

  // The check is PROPERTY-AWARE rather than "no layout utility anywhere", and it
  // has to be: a kept rule only wins or loses on the properties it declares, so
  // banning every spacing utility would ban ones that cannot collide and would
  // teach the next reader the wrong rule. The banned families are read off the
  // sheet's own declarations, so the guard cannot drift from the rules it
  // guards.
  const utilityFor: Record<string, RegExp> = {
    display: /(^|\s)(flex|grid|inline-flex|inline-grid|block|hidden|contents)(\s|$)/,
    'flex-direction': /(^|\s)(md:)?(flex-col|flex-row)(\s|$)/,
    'flex-wrap': /(^|\s)(md:)?(flex-wrap|flex-nowrap)(\s|$)/,
    'align-items': /(^|\s)(md:)?items-/,
    gap: /(^|\s)gap(-[xy])?-/,
    'margin-bottom': /(^|\s)(mb-|my-|m-)/,
    'min-width': /(^|\s)min-w-/,
    flex: /(^|\s)flex-(1|auto|initial|none)(\s|$)/,
  }
  /** Every property the sheet declares for one selector, base arm and wide arm. */
  const declaredFor = (selector: string): string[] => {
    const props = new Set<string>()
    for (const source of [cssSource, wide]) {
      let from = 0
      for (;;) {
        const at_ = source.indexOf(selector, from)
        if (at_ === -1) break
        from = at_ + selector.length
        const open = source.indexOf('{', at_)
        const close = source.indexOf('}', at_)
        // Only a rule this selector really heads or joins — the run between the
        // selector and the next `{` must be selector text, never a declaration.
        if (open === -1 || (close !== -1 && close < open)) continue
        if (/[:;]/.test(source.slice(at_ + selector.length, open))) continue
        for (const decl of source.slice(open + 1, source.indexOf('}', open)).split(';')) {
          const name = decl.split(':')[0].trim()
          if (name !== '') props.add(name)
        }
      }
    }
    return [...props]
  }

  const kept = [
    // `.rev-picker` is scoped under `.deliverable`; the rest are bare.
    { cls: 'rev-picker', selector: '.deliverable .rev-picker', arm: true },
    { cls: 'diff-controls', selector: '.diff-controls', arm: true },
    { cls: 'image-modes', selector: '.image-modes', arm: true },
    { cls: 'port-picker', selector: '.port-picker', arm: true },
    { cls: 'frame-path', selector: '.frame-path', arm: true },
    { cls: 'two-up', selector: '.two-up', arm: true },
    { cls: 'frames', selector: '.frames', arm: true },
    // `merge-options` is the CONTROL: it has no wide arm and it does carry
    // utilities (`mt-2 list-none ps-0`) for properties its rule never declares,
    // so it proves the guard discriminates rather than banning everything.
    { cls: 'merge-options', selector: '.merge-options', arm: false },
  ]
  let propertiesChecked = 0
  for (const { cls, selector, arm } of kept) {
    const declared = declaredFor(selector)
    expect(declared.length, `.${cls} lost its owned rule — the sheet declares nothing for it`).toBeGreaterThan(0)
    expect(declared, `.${cls} stopped being a flex container, so this entry is stale`).toContain('flex-direction')
    // Its wide-screen arm, where it has one, is still in the media query.
    if (arm) expect(wide, `.${cls} lost its wide-screen arm`).toContain(`.${cls}`)

    const marks = [...deliverableSource.matchAll(new RegExp(`className="${cls}([^"]*)"`, 'g'))]
    expect(marks.length, `no <element className="${cls} …"> is rendered — the check reached nothing`).toBeGreaterThan(0)
    for (const property of declared) {
      const banned = utilityFor[property]
      if (banned === undefined) continue
      propertiesChecked++
      for (const m of marks) {
        expect(
          banned.test(m[1]),
          `.${cls} gained a "${property}" utility beside its kept owned rule: "${m[1]}"`,
        ).toBe(false)
      }
    }
  }
  expect(propertiesChecked, 'no property was actually checked — the guard is vacuous').toBeGreaterThan(20)

  // The detector really bites, in both directions, on the two families that
  // actually killed `.chat-verbs`.
  expect(utilityFor['flex-direction'].test(' flex flex-col gap-2')).toBe(true)
  expect(utilityFor.display.test(' flex flex-col gap-2')).toBe(true)
  expect(utilityFor['flex-direction'].test(' mt-2 list-none ps-0')).toBe(false)
  expect(utilityFor.display.test(' text-sm font-semibold')).toBe(false)

  // And the ten `responsive.test.ts` reads this surface must not break: each
  // named selector still heads a rule carrying the declaration that suite asks
  // for. That file is byte-frozen, so this is the shed's own tripwire.
  for (const [selector, decl] of [
    ['.diff-file', 'overflow-x: auto'],
    ['.deliverable .object-sha', 'overflow-wrap: anywhere'],
    ['.merge-card pre', 'overflow-x: auto'],
    ['.signing', 'overflow-wrap: anywhere'],
    ['.merge-options', 'flex-direction: column'],
    ['.frame-path', 'flex-direction: column'],
    ['.frames', 'flex-direction: column'],
    ['.image-side img', 'max-width: 100%'],
    ['.frame iframe', 'width: 100%'],
  ] as const) {
    const at_ = cssSource.indexOf(`\n${selector} {`)
    expect(at_, `${selector} is not styled at all — responsive.test.ts would fail`).toBeGreaterThan(-1)
    expect(cssSource.slice(at_, cssSource.indexOf('}', at_)), `${selector} lost ${decl}`).toContain(decl)
  }
})

test('the shed rules are gone and their replacements really compile', async () => {
  // Each shed rule had `Deliverable.tsx` as its only consumer (grep-verified at
  // the shed), so what has to be true afterwards is that the presentation moved
  // rather than vanished — the §53 A2 D1 lesson, where a shed rule's utilities
  // reached one of two `<li>` families and not the other.
  for (const gone of [
    '.diff-file-head {',
    '.diff-widget {',
    '.comment {',
    '.synthetic-strip {',
    '.image-side {',
    '.swipe-stack,',
    '.frame-absent {',
    '.accept-card dd,',
  ]) {
    expect(cssSource, `${gone} survived the shed`).not.toContain(gone)
  }
  // `.lineage` and `.revisions` did NOT shed, and that is the recorded finding:
  // TaskDetail.tsx renders both classes and is byte-frozen here, so the brief's
  // "if the markup move frees them" is false. The rules must still be present.
  expect(cssSource, '.lineage shed while TaskDetail still consumes it').toContain('.lineage {')
  expect(cssSource, '.revisions shed while TaskDetail still consumes it').toContain('.revisions {')
  expect(taskDetailSource, 'TaskDetail stopped consuming .lineage, so the stay-note is stale').toContain(
    'className="lineage"',
  )
  expect(taskDetailSource, 'TaskDetail stopped consuming .revisions, so the stay-note is stale').toContain(
    'className="revisions"',
  )

  // The replacements are carried at the sites the shed rules used to reach —
  // every family, not just the first one.
  const { view } = await review()
  // ⚠ SELECTED BY `[data-widget-key]`, NOT by `.diff-widget`, and the reason is a
  // collision this shed surfaced: `diff-widget` is ALSO react-diff-view's own
  // class for the row it mounts a widget into, so the owned rule (a .35rem/.5rem
  // padding and a 3px left rail in `currentColor`) MATCHED the adopted widget's
  // `<tr>` as well as our own div inside it.
  //
  // DATED CORRECTION 2026-08-05 (C-2 drain r1, D4): this comment used to say the
  // rule applied "one left rail and one padding, applied twice", which overstates
  // it by one property. Only the RAIL ever doubled. react-diff-view's `.diff`
  // table collapses its borders (its own stylesheet sets that border model to
  // `collapse`), and under the collapsed model a border set on a table ROW box IS
  // painted — so the 3px rail really was drawn on the adopted `<tr>` and again on
  // our div inside it. Padding, by contrast, does not apply to a table row box at
  // all, so the row's padding never painted and only our div's ever did. Matching
  // a rule and painting it are not the same thing, and this comment claimed
  // painting. The defect is a doubled rail, not a doubled pair.
  //
  // (Deliberately written WITHOUT the hyphenated CSS property and display-value
  // literals this paragraph is about. Tailwind scans this file, so such a token
  // left bare in prose here compiles a real — and permanently dead — rule into the
  // production sheet. Measured, not guessed: the first draft of this correction
  // shipped exactly two, and the rebuild is what caught them.)
  //
  // The rule is gone, the presentation now lands once on our own element, and the
  // adopted stylesheet is left to style its own DOM (adopt-don't-fork).
  // `.diff-widget` stays as the class it was: the landed pins select
  // `.diff-widget [data-comment]`, which reaches ours either way.
  expect(at(view, '[data-widget-key]')?.className, 'the widget rail did not move to utilities').toContain(
    'border-s-[3px]',
  )
  expect(at(view, '.synthetic-strip')?.className, 'the strip rail did not move to utilities').toContain('border-s-[3px]')
  expect(at(view, '.comment')?.className, 'the comment padding did not move to utilities').toContain('py-1.5')
  expect(at(view, '.diff-file-head')?.className, 'the file head did not move to utilities').toContain('wrap-anywhere')
  expect(at(view, '.comment-list')?.className, 'the comment list decoration did not move').toContain('list-none')
  expect(at(view, '.doors')?.className, 'the door list decoration did not move').toContain('list-none')
  // The accept card's `dd` wrap moved to ONE descendant utility on the container,
  // so both dl consumers are covered by the same string rather than by two.
  click(at(view, '[data-action="read-accept-card"]'))
  await flush()
  expect(at(view, '.accept-card dl')?.className, 'the accept card dd wrap did not move').toContain(
    '[&_dd]:wrap-anywhere',
  )
  view.unmount()
})

test('⚠ the accept card stopped borrowing the workforce map`s .provenance grid', async () => {
  // THE FINDING. `.provenance` is the S15.10 workforce rule — `display: grid`
  // with a grid column template, written for a `<dl>` — and the accept card
  // rendered a `<p>` of prose under the same class, so it silently took a grid
  // display and a margin meant for somebody else's markup. `Workforce.tsx` is
  // byte-frozen, so the rule stays for its real owner and this line carries its
  // own utilities.
  const { view } = await review()
  click(at(view, '[data-action="read-accept-card"]'))
  await flush()
  const prov = at(view, '[data-provenance="accept-card"]')!
  expect(prov, 'the accept card lost its provenance line').not.toBeNull()
  expect(prov.className, 'the accept card still carries the workforce grid class').not.toContain('provenance')
  expect(prov.className, 'the provenance line is not in the mono/tabular treatment').toContain('font-mono')
  // The rule itself is untouched, because its real consumer still renders it.
  expect(cssSource, '.provenance was shed out from under Workforce.tsx').toContain('.provenance {')
  expect(workforceSource, 'Workforce stopped consuming .provenance').toContain('className="provenance"')
  view.unmount()
})

// ── seam C-2 drain r1 ───────────────────────────────────────────────────────

test('D3: the POPULATED lineage arms render both directions, and the empty form is unchanged', async () => {
  // WHY THIS PIN EXISTS AT ALL, written down because its absence was the finding.
  // `LineageEdges` renders the two edge arrays of `TaskLineage` — `succeeds` and
  // `succeeded_by`, each a `TaskSuccessor` (api.ts:343–352), reached through
  // `DeliverableDetail.lineage` (api.ts:1078–1084) — and until this test NOTHING
  // in the tree pinned their POPULATED form. The golden detail serves
  // `succeeds: []` / `succeeded_by: []` and the seeded world serves the same, so
  // the C-2 browser walk could not reach the block either (§53 records that
  // region NOT WALKED, honestly). Typecheck was the whole of the cover, and
  // typecheck cannot see this bug: both id fields are `string`, so an arm
  // pointing at the wrong ROUTE FAMILY compiles perfectly. It rendered correctly
  // by luck rather than by verification.
  //
  // The body is SCRIPTED per test — the §49 error-contract precedent — because no
  // producer in this world mints a deliverable that both follows from one and has
  // spawned a follow-up task. Declared derived here rather than at the fixtures.
  const detail = fixtures.deliverableReview() as { lineage: Record<string, unknown> }

  // The EMPTY arms first, so the populated far side is a real change of state
  // rather than a shape that was on screen all along.
  const bare = await review()
  expect(text(bare.view), 'the landed absent sentence moved').toContain(
    'No follow-up lineage: this deliverable neither followed from one nor spawned a task.',
  )
  expect(at(bare.view, '.lineage'), 'the empty arms mounted the lineage block anyway').toBeNull()
  expect(all(bare.view, '[data-lineage]'), 'an empty arm rendered an edge').toHaveLength(0)
  bare.view.unmount()

  const { view } = await review(reviewDeliverableID, {
    [`GET /api/deliverables/${reviewDeliverableID}`]: {
      body: {
        ...detail,
        lineage: {
          ...detail.lineage,
          succeeds: [
            { task_id: 't-prior77', deliverable_id: 'd-charter', revision_n: 3, created_ts: '2026-07-01T09:00:00Z' },
          ],
          succeeded_by: [
            {
              task_id: 't-follow42',
              deliverable_id: reviewDeliverableID,
              revision_n: 2,
              created_ts: '2026-07-02T09:00:00Z',
            },
          ],
        },
      },
    },
  })

  expect(at(view, '.lineage'), 'the populated arms never mounted the lineage block').not.toBeNull()
  expect(text(view), 'the absent sentence survived a populated lineage').not.toContain('No follow-up lineage')

  // BOTH DIRECTIONS, each with its own route family. The families differ on
  // purpose and the difference is the point: a backward edge names the
  // DELIVERABLE this one followed from, a forward edge names the TASK it spawned
  // — which is the same rule TaskDetail renders lineage under — so a copy-pasted
  // arm would send one of them to a route that cannot serve it.
  const from = at(view, '[data-lineage="succeeds"]')!
  expect(from.textContent, 'the backward edge lost its words').toContain('Follows from')
  expect(from.textContent, 'the backward edge lost its deliverable or its revision').toContain('d-charter r3')
  const fromHref = from.querySelector('a')?.getAttribute('href')
  expect(fromHref, 'a backward edge must link the DELIVERABLE it followed from').toBe(
    hrefFor('deliverable', { id: 'd-charter' }),
  )
  expect(fromHref, 'the deliverable route moved out from under the edge').toBe('/deliverables/d-charter')

  const to = at(view, '[data-lineage="succeeded-by"]')!
  expect(to.textContent, 'the forward edge lost its words').toContain('Followed up by')
  expect(to.textContent, 'the forward edge lost its task').toContain('t-follow42')
  const toHref = to.querySelector('a')?.getAttribute('href')
  expect(toHref, 'a forward edge must link the TASK it spawned, never a deliverable').toBe(
    hrefFor('task', { id: 't-follow42' }),
  )
  expect(toHref, 'the task route moved out from under the edge').toBe('/tasks/t-follow42')
  view.unmount()
})

// ── RA-B1: the owner's moment — result-first, download, honest state, plain
//    accept, machine findings never signed as a person ───────────────────────

test('RA-B1: the result leads the page, with the source one click away and downloads served', async () => {
  const { view } = await review()

  // THE ORDER IS THE FINDING: the walk ended at a diff because the diff came
  // first. "The work" must precede "Revisions" in the document itself.
  const sections = all(view, '.deliverable h3, .deliverable h2').map((h) => h.textContent ?? '')
  const workAt = sections.findIndex((t) => t.includes('The work'))
  const revisionsAt = sections.findIndex((t) => t.includes('Revisions'))
  expect(workAt, 'no "The work" section exists').toBeGreaterThanOrEqual(0)
  expect(revisionsAt).toBeGreaterThan(workAt)

  // d-site is a CODE deliverable: its README does not upstage its source. The
  // composed result is the current revision's text, as text.
  const composed = at(view, '[data-result-kind="text"]')
  expect(composed, 'the code deliverable did not render its text result').not.toBeNull()
  expect(composed?.textContent).toContain('export function Release()')

  // The source mode shows the files by name.
  click(at(view, '[data-result-mode="source"]'))
  await flush()
  expect(at(view, '[data-result-source]')?.textContent).toContain('site/README.md')

  // The download door: one real link per pinned object of the current
  // revision, riding the objects route with the download attribute.
  const downloads = all(view, '[data-action="download-result"]')
  expect(downloads.length, 'the current revision must offer its bytes').toBe(2)
  for (const a of downloads) {
    expect(a.getAttribute('href')).toContain('/api/deliverables/d-site/objects/')
    expect(a.getAttribute('download'), 'a download link must actually download').not.toBeNull()
  }
  view.unmount()
})

test('RA-B1: the in-review story names the reviewer honestly, for the owner and for a visitor', async () => {
  // The owner (alice) is told the work waits on HER — with real doors.
  const mine = await review()
  const story = at(mine.view, '[data-state-story="in-review"]')!
  expect(story.textContent).toContain('waiting for you')
  expect(story.querySelector('[data-jump="accept"]'), 'the story must door to the accept').not.toBeNull()
  mine.view.unmount()

  // Somebody else is told WHOSE review it waits on — never a vague "in review".
  const theirs = await review(reviewDeliverableID, { 'GET /api/auth/session': asBob })
  const other = at(theirs.view, '[data-state-story="in-review"]')!
  expect(other.textContent).toContain("alice's review")
  theirs.view.unmount()
})

test('RA-5: a verification finding is signed as the platform’s checker, never as a person', async () => {
  const { view } = await review()
  // Fixture comment 7 is kind "finding" and its wire row carries alice as its
  // author — the REPORTED attribution gap. The view is where the honest label
  // lives.
  const finding = at(view, '[data-comment="7"]')!
  expect(finding.querySelector('[data-author="platform-checker"]')?.textContent).toContain('checker')
  expect(finding.querySelector('.comment-head')?.textContent).not.toContain('alice')
  // A HUMAN comment keeps its human.
  const human = at(view, '[data-comment="1"]')!
  expect(human.querySelector('[data-author="platform-checker"]')).toBeNull()
  expect(human.querySelector('.comment-head')?.textContent).toContain('alice')
  view.unmount()
})

test('RA-B1: the accept card speaks the owner’s language, mechanics one fold away', async () => {
  const { view } = await review()
  click(at(view, '[data-action="read-accept-card"]'))
  await flush()

  // The plain half: what accepting MEANS, unfolded.
  const plain = at(view, '[data-accept-plain]')!
  expect(plain.textContent).toContain('revision 2')
  expect(plain.textContent).toContain('permanent record')
  expect(plain.textContent).toContain('your own decision')

  // The mechanics — pins, trailers, provenance, the signing statement — are
  // ALL still on the surface, inside the one technical fold.
  const tech = at(view, '.accept-tech')!
  expect(tech.querySelector('[data-payload-hash]'), 'the payload pin left the surface').not.toBeNull()
  expect(tech.querySelector('[data-trailers]'), 'the trailers left the surface').not.toBeNull()
  expect(tech.querySelector('.signing'), 'the signing statement left the surface').not.toBeNull()

  // And the form still drives the same accept, PIN and all.
  expect(at(view, '[data-action="accept"]')).not.toBeNull()
  expect(at(view, '[data-field="accept-pin"]')).not.toBeNull()
  view.unmount()
})
