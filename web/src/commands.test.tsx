import { act } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { Deliverable } from './Deliverable'
import { Projects, hrefForProjectCommands } from './Projects'
import type { ProjectDetail } from './api'
import { FakeSource, fixtures, type Scripted, scriptedFetch } from './doubles'
import { click, flush, mount, typeInto } from './testing'

/**
 * The Commands door (P3-GF6; CONVENTIONS §70) — behavior-contract probes.
 *
 * Two connected halves, both driven from the P3-GF5 GOLDEN fixtures so what
 * these tests prove is the landed wire contract, never a shape this file
 * imagined:
 *
 *  - the bootstrap DISCLOSURE on a deliverable renders from the served
 *    `verification` member (posture + review_mandatory) and links into the
 *    project's Commands editor — never by matching prose or any token;
 *  - the EDITOR on the Projects surface: the owner's full-replacement write
 *    (the golden request/response pair verbatim), the explicit all-empty
 *    clear said out loud before the press, the member's honest read-only, the
 *    pending draft's pointer at its real door, and refusals in the server's
 *    own words.
 *
 * The pixels are judged in real Chrome (FRONTEND.md rule 5); this file pins
 * the contract underneath them.
 */

const inertStream = undefined

const doc = (sel: string) => document.body.querySelector(sel)
const docText = () => document.body.textContent ?? ''

/** The golden bootstrap detail's own facts, read from the fixture rather than
 *  repeated: the deliverable id, its project, and the one pinned object. */
const bootDetail = () =>
  fixtures.deliverableBootstrap() as {
    deliverable: { id: string; project_id: string }
    revisions: { n: number; objects: { sha256: string }[] }[]
    verification?: { posture: string; review_mandatory: boolean }
  }

/** The golden WRITE answer, whose `project` member is also the only served
 *  p-fresh detail shape. */
const writtenFixture = () => fixtures.projectCommands() as { project: ProjectDetail; detail: string; cursor: number }

/** p-fresh BEFORE the write, derived from the golden write answer's own
 *  project member (the one served p-fresh shape) by stepping the capture back
 *  to the state the GF5 world seeded it in: version 1, nothing captured. A
 *  hand-imagined detail body is the §42-B fixture defect; this stays inside
 *  the served shape and changes only the two facts the journey is about. */
function freshDetailBefore(): { project: ProjectDetail; cursor: number } {
  const p = writtenFixture().project
  return {
    project: {
      ...p,
      capture_version: 1,
      capture: { ...p.capture, version: 1, commands: {} },
    },
    cursor: 11,
  }
}

function bootRoutes(): Record<string, Scripted> {
  const d = bootDetail()
  const id = d.deliverable.id
  const routes: Record<string, Scripted> = {
    [`GET /api/deliverables/${id}`]: { body: d },
    // The comparison surface is not under probe here; a pending read keeps it
    // honestly in flight without inventing a compare body no producer served.
    [`GET /api/deliverables/${id}/compare`]: { pending: true },
    [`GET /api/deliverables/${id}/comments?revision=1`]: { body: { comments: [], placements: [] } },
    [`GET /api/deliverables/${id}/accept-card`]: { body: fixtures.acceptCard() },
    'GET /api/previews': { body: { sessions: [] } },
  }
  for (const rev of d.revisions) {
    for (const o of rev.objects) {
      routes[`GET /api/deliverables/${id}/objects/${o.sha256}`] = { body: 'package main\n\nfunc main() {}\n' }
    }
  }
  return routes
}

function projectsRoutes(): Record<string, Scripted> {
  return {
    'GET /api/tasks': { body: fixtures.tasks() },
    'GET /api/deliverables': { body: fixtures.deliverablesInReview() },
    'GET /api/projects': { body: fixtures.projects() },
    'GET /api/projects/p-shop': { body: fixtures.projectDetail() },
    'GET /api/projects/p-fresh': { body: freshDetailBefore() },
  }
}

const slotValue = (k: string) => (doc(`[data-cmd-slot="${k}"]`) as HTMLInputElement | null)?.value

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  FakeSource.reset()
  vi.stubGlobal('EventSource', FakeSource)
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── the disclosure on the deliverable ───────────────────────────────────────

test('the bootstrap posture renders from the served member, with the door to the Commands editor', async () => {
  const d = bootDetail()
  scriptedFetch(bootRoutes())
  const view = mount(<Deliverable id={d.deliverable.id} me="alice" stream={inertStream} />)
  await flush()

  const banner = view.container.querySelector('[data-posture="bootstrap"]')
  expect(banner, 'the served bootstrap member must render its disclosure').not.toBeNull()
  const text = banner?.textContent ?? ''
  expect(text).toContain('bootstrap mode')
  expect(text).toContain('your review is what decides')
  // review_mandatory is TRUE in the golden body, and the sentence renders
  // from the member, not from any note text.
  expect(d.verification?.review_mandatory).toBe(true)
  expect(text).toContain('nothing counts as verified until the requester judges it')
  // The DOOR: a real address into the project's own Commands editor.
  const door = banner?.querySelector('[data-door="project-commands"]')
  expect(door, 'the bootstrap disclosure must offer the Commands door').not.toBeNull()
  expect(door?.getAttribute('href')).toBe(hrefForProjectCommands(d.deliverable.project_id))
  expect(door?.getAttribute('href')).toBe('/projects?commands=p-fresh')
  // Plain words only: no machine token, no spec citation on the card.
  expect(text).not.toContain('check-pack')
  expect(text).not.toContain('S07')
})

test('an ordinary deliverable carries NO posture banner — the absent member renders nothing', async () => {
  const detail = fixtures.deliverableDetail() as Record<string, unknown>
  expect('verification' in detail, 'the ordinary golden body must not carry the member').toBe(false)
  scriptedFetch({
    'GET /api/deliverables/d-notes': { body: detail },
    'GET /api/deliverables/d-notes/compare': { pending: true },
    'GET /api/deliverables/d-notes/comments?revision=2': { body: { comments: [], placements: [] } },
    'GET /api/deliverables/d-notes/accept-card': { body: fixtures.acceptCard() },
    'GET /api/previews': { body: { sessions: [] } },
  })
  const view = mount(<Deliverable id="d-notes" me="alice" stream={inertStream} />)
  await flush()
  expect(view.container.querySelector('[data-posture]')).toBeNull()
})

test('a posture this build does not know renders as itself, with no bootstrap story and no door', async () => {
  const d = bootDetail()
  const routes = bootRoutes()
  routes[`GET /api/deliverables/${d.deliverable.id}`] = {
    body: { ...d, verification: { posture: 'strict-v2', review_mandatory: false } },
  }
  scriptedFetch(routes)
  const view = mount(<Deliverable id={d.deliverable.id} me="alice" stream={inertStream} />)
  await flush()

  const banner = view.container.querySelector('[data-posture="strict-v2"]')
  expect(banner).not.toBeNull()
  expect(banner?.textContent).toContain('strict-v2')
  expect(banner?.textContent).not.toContain('bootstrap mode')
  // review_mandatory false: no deciding-gate claim invented.
  expect(banner?.textContent).not.toContain('deciding gate')
  expect(banner?.querySelector('[data-door="project-commands"]')).toBeNull()
})

// ── the editor on the Projects surface ──────────────────────────────────────

test('the deep link opens the editor prefilled from the served capture, posture said honestly', async () => {
  scriptedFetch(projectsRoutes())
  window.history.replaceState(null, '', '/projects?commands=p-shop')
  mount(<Projects me="alice" search="?commands=p-shop" />)
  await flush()

  const editor = doc('[data-commands-editor="p-shop"]')
  expect(editor, 'the ?commands= address must open the editor').not.toBeNull()
  expect(editor?.getAttribute('data-commands-mode')).toBe('edit')
  // Prefilled from the golden p-shop detail, slot for slot.
  expect(slotValue('build')).toBe('go build ./...')
  expect(slotValue('test')).toBe('go test ./...')
  expect(slotValue('lint')).toBe('gofmt -l .')
  expect(slotValue('run')).toBe('go run ./cmd/shop')
  expect(slotValue('preview')).toBe('go run ./cmd/shop --port 8080')
  expect(doc('[data-cmd-posture="checked"]')).not.toBeNull()
})

test('the owner captures commands: the golden request goes out whole, the platform sentence comes back', async () => {
  const log = scriptedFetch({
    ...projectsRoutes(),
    'POST /api/projects/p-fresh/commands': { body: writtenFixture() },
  })
  mount(<Projects me="alice" search="?commands=p-fresh" />)
  await flush()

  // The fresh project's honest state first: nothing captured, bootstrap said.
  expect(doc('[data-commands-editor="p-fresh"]')?.getAttribute('data-commands-mode')).toBe('edit')
  expect(doc('[data-cmd-posture="bootstrap"]')).not.toBeNull()
  expect(docText()).toContain('bootstrap mode')

  // Type the golden request's own three commands.
  typeInto(doc('[data-cmd-slot="build"]') as HTMLInputElement, 'npm run build')
  typeInto(doc('[data-cmd-slot="test"]') as HTMLInputElement, 'npm test')
  typeInto(doc('[data-cmd-slot="lint"]') as HTMLInputElement, 'npm run lint')
  click(doc('[data-commands-save]'))
  await flush()

  // FULL REPLACEMENT, blanks left out: byte-shape of the golden request.
  const posted = log.calls.find((c) => c.method === 'POST')
  expect(posted?.path).toBe('/api/projects/p-fresh/commands')
  expect(posted?.body).toEqual({ commands: { build: 'npm run build', test: 'npm test', lint: 'npm run lint' } })

  // The platform's own sentence, verbatim — never re-told by this client.
  expect(doc('[data-commands-landed]')?.textContent).toBe(writtenFixture().detail)
  // The read-back moves the editor to the captured state: v2, real checks.
  expect(docText()).toContain('capture v2')
  expect(doc('[data-cmd-posture="checked"]')).not.toBeNull()
  expect(slotValue('build')).toBe('npm run build')
})

test('all boxes emptied is the EXPLICIT clear: disclosed before the press, {} on the wire', async () => {
  const log = scriptedFetch({
    ...projectsRoutes(),
    'POST /api/projects/p-shop/commands': {
      body: {
        project: {
          ...(fixtures.projectDetail() as { project: ProjectDetail }).project,
          capture_version: 2,
          capture: {
            ...(fixtures.projectDetail() as { project: ProjectDetail }).project.capture,
            version: 2,
            commands: {},
          },
        },
        detail:
          'cleared: every captured command is removed as version 2, and work in this project is checked in the bootstrap posture — requester review decides — until commands are captured again.',
        cursor: 12,
      },
    },
  })
  mount(<Projects me="alice" search="?commands=p-shop" />)
  await flush()

  for (const k of ['build', 'test', 'lint', 'run', 'preview']) {
    typeInto(doc(`[data-cmd-slot="${k}"]`) as HTMLInputElement, '')
  }
  // Said out loud BEFORE the press, and the act renames itself.
  expect(doc('[data-cmd-clearing]')).not.toBeNull()
  expect(doc('[data-cmd-clearing]')?.textContent).toContain('bootstrap mode')
  expect(doc('[data-commands-save]')?.textContent).toContain('Clear every command')

  click(doc('[data-commands-save]'))
  await flush()
  const posted = log.calls.find((c) => c.method === 'POST')
  expect(posted?.body, 'the sanctioned explicit-clear recipe').toEqual({ commands: {} })
  expect(doc('[data-commands-landed]')?.textContent).toContain('cleared')
})

test('a member sees the capture read-only, with the honest sentence naming the owner', async () => {
  scriptedFetch(projectsRoutes())
  mount(<Projects me="bob" search="?commands=p-shop" />)
  await flush()

  const editor = doc('[data-commands-editor="p-shop"]')
  expect(editor?.getAttribute('data-commands-mode')).toBe('read')
  expect(doc('[data-cmd-why="not-owner"]')?.textContent).toContain('alice')
  expect(doc('[data-cmd-slot="build"]'), 'no editable control for a member').toBeNull()
  expect(doc('[data-commands-save]')).toBeNull()
  // The captured set still reads — a member works in this project too.
  expect(docText()).toContain('go test ./...')
})

test('a pending draft points at its real door — the onboarding card — and renders no editor', async () => {
  const shop = (fixtures.projectDetail() as { project: ProjectDetail; cursor: number }).project
  scriptedFetch({
    ...projectsRoutes(),
    'GET /api/projects/p-shop': { body: { project: { ...shop, state: 'pending' }, cursor: 11 } },
  })
  mount(<Projects me="alice" search="?commands=p-shop" />)
  await flush()

  expect(doc('[data-commands-editor="p-shop"]')?.getAttribute('data-commands-mode')).toBe('read')
  expect(doc('[data-cmd-why="pending"]')?.textContent).toContain('onboarding card')
  expect(doc('[data-cmd-slot="build"]')).toBeNull()
  expect(doc('[data-commands-save]')).toBeNull()
})

test('a refusal renders in the SERVER’s words', async () => {
  const detailSentence =
    'this project is still waiting for your approval, so its drafted commands are edited on the onboarding card in your Inbox'
  scriptedFetch({
    ...projectsRoutes(),
    'POST /api/projects/p-fresh/commands': { status: 409, body: { error: 'not_active', detail: detailSentence } },
  })
  mount(<Projects me="alice" search="?commands=p-fresh" />)
  await flush()

  typeInto(doc('[data-cmd-slot="test"]') as HTMLInputElement, 'npm test')
  click(doc('[data-commands-save]'))
  await flush()
  expect(doc('.door-refusal')?.textContent).toContain(detailSentence)
  expect(doc('[data-commands-landed]')).toBeNull()
})

test('an id that is not visible answers the one honest sentence, not an editor', async () => {
  scriptedFetch({
    ...projectsRoutes(),
    'GET /api/projects/p-ghost': { status: 404, body: { error: 'not_found', detail: 'project not found' } },
  })
  mount(<Projects me="alice" search="?commands=p-ghost" />)
  await flush()

  expect(doc('[data-commands-editor]')).toBeNull()
  expect(docText()).toContain('No project "p-ghost" is visible to you')
})

test('each card carries the Commands door, and closing a URL-opened editor clears the address', async () => {
  scriptedFetch(projectsRoutes())
  const view = mount(<Projects me="alice" search="" />)
  await flush()

  click(view.container.querySelector('[data-proj-commands="p-shop"]'))
  await flush()
  expect(doc('[data-commands-editor="p-shop"]')).not.toBeNull()

  // Close, then the URL door: ?commands= opens, closing returns to /projects
  // by REPLACE so Back leaves the surface rather than re-opening the modal.
  const close = [...document.body.querySelectorAll('button')].find((b) => b.textContent === 'Close')
  click(close)
  await flush()
  expect(doc('[data-commands-editor="p-shop"]')).toBeNull()

  window.history.replaceState(null, '', '/projects?commands=p-fresh')
  const linked = mount(<Projects me="alice" search="?commands=p-fresh" />)
  await flush()
  expect(doc('[data-commands-editor="p-fresh"]')).not.toBeNull()
  const close2 = [...document.body.querySelectorAll('button')].find((b) => b.textContent === 'Close')
  click(close2)
  await flush()
  // Closing a URL-opened editor REPLACES the address back to /projects, so
  // Back leaves the surface rather than re-opening the modal…
  expect(window.location.pathname).toBe('/projects')
  expect(window.location.search).toBe('')
  // …and the shell re-derives `search` from the location on that navigation
  // (App.tsx passes window.location.search on every route render). The direct
  // mount here pins the prop, so the shell's half is simulated by re-rendering
  // with the address the close just wrote.
  act(() => {
    linked.root.render(<Projects me="alice" search={window.location.search} />)
  })
  await flush()
  expect(doc('[data-commands-editor="p-fresh"]')).toBeNull()
  linked.unmount()
})
