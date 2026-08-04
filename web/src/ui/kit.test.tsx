import { useState } from 'react'
import { expect, test } from 'vitest'

import { click, mount } from '../testing'
import {
  Button,
  Chip,
  Drawer,
  EmptyState,
  Modal,
  Panel,
  Skeleton,
  StatTile,
  StatusDot,
  ToastProvider,
  cn,
  tones,
  useToast,
  type Tone,
  type ToastSeverity,
} from './index'

/**
 * The primitive kit, RENDERED (S16.4 #5: a pick is verified behaviourally, not
 * by importing it). Every component and every variant reaches the DOM here,
 * because a variant nobody renders is a class string nobody has checked.
 */

test('cn joins conditionally and lets a caller override the variant', () => {
  expect(cn('a', false && 'b', 'c')).toBe('a c')
  // The reason tailwind-merge is in the tree: a caller's utility must beat the
  // variant's rather than depending on stylesheet order.
  expect(cn('px-4', 'px-2')).toBe('px-2')
})

test('Button renders every variant and every size, and defaults to a non-submitting type', () => {
  for (const variant of ['primary', 'secondary', 'ghost', 'danger'] as const) {
    for (const size of ['sm', 'md'] as const) {
      const view = mount(
        <Button variant={variant} size={size}>
          Act
        </Button>,
      )
      const el = view.container.querySelector('button')!
      expect(el.textContent).toBe('Act')
      // A kit button inside somebody's form must not submit it by accident.
      expect(el.getAttribute('type')).toBe('button')
      expect(el.className.length, `${variant}/${size} emitted no classes`).toBeGreaterThan(0)
      view.unmount()
    }
  }
})

test('Button carries the identity gradient on primary alone, and a disabled one cannot be pressed', () => {
  const primary = mount(<Button variant="primary">Go</Button>)
  expect(primary.container.querySelector('button')!.className).toContain('--grad')
  primary.unmount()

  const secondary = mount(<Button variant="secondary">Go</Button>)
  expect(secondary.container.querySelector('button')!.className).not.toContain('--grad')
  secondary.unmount()

  let pressed = 0
  const view = mount(
    <Button disabled onClick={() => pressed++}>
      Go
    </Button>,
  )
  click(view.container.querySelector('button'))
  expect(pressed, 'a disabled button ran its handler').toBe(0)
  view.unmount()
})

test('Panel is one material, with an optional head above the body', () => {
  const view = mount(
    <Panel head={<span>Head</span>}>
      <p>Body</p>
    </Panel>,
  )
  const text = view.container.textContent ?? ''
  expect(text).toContain('Head')
  expect(text).toContain('Body')
  // The kit does NOT reuse the landed `.panel` class, which already means
  // something else — it styles with utilities so no name collision decides
  // what a box looks like.
  expect(view.container.querySelector('.panel')).toBeNull()
  view.unmount()
})

test('Chip implements ONE formula over all seven tones', () => {
  expect(tones).toHaveLength(7)
  const classNames = new Set<string>()
  for (const tone of tones) {
    const view = mount(<Chip tone={tone}>running</Chip>)
    const el = view.container.querySelector('span')!
    expect(el.textContent).toBe('running')
    // The tone is a TOKEN NAME bound to --tone, never a colour baked into the
    // component: that is what makes it one formula rather than seven rules.
    expect(el.getAttribute('style')).toContain(`--tone: var(--${tone})`)
    classNames.add(el.className)
    view.unmount()
  }
  // Every tone emits the SAME classes — the difference is the token alone.
  expect(classNames.size, 'a tone grew its own rule').toBe(1)
  expect([...classNames][0]).toContain('color-mix')
})

test('StatusDot glows in its tone and pulses only when told it is genuinely live', () => {
  const still = mount(<StatusDot tone="green" />)
  const stillEl = still.container.querySelector('span')!
  expect(stillEl.className).toContain('shadow-')
  expect(stillEl.className, 'a non-live dot pulses').not.toContain('animate-pulse')
  // Decoration beside a label, so it is not announced twice.
  expect(stillEl.getAttribute('aria-hidden')).toBe('true')
  still.unmount()

  const live = mount(<StatusDot tone="green" live />)
  const liveEl = live.container.querySelector('span')!
  // Gated: a reduce reader gets the same dot, standing still.
  expect(liveEl.className).toContain('motion-safe:animate-pulse')
  live.unmount()
})

test('StatTile renders the figure it is given, from the first frame, with no count-up', () => {
  const view = mount(<StatTile label="Runs today" value="17" tone="blue" foot="since 00:00 UTC" icon={<span>i</span>} />)
  const text = view.container.textContent ?? ''
  expect(text).toContain('Runs today')
  expect(text).toContain('17')
  expect(text).toContain('since 00:00 UTC')
  // The figure is mono and tabular — the instrument-panel discipline — and it
  // is NOT animated from zero: for the seconds a count-up runs it shows a
  // number the platform never served.
  const figure = view.container.querySelector('.tabular-nums')!
  expect(figure.textContent).toBe('17')
  expect(figure.className).toContain('font-mono')
  // Nothing in the rendered tree animates the figure itself.
  const animated = [...view.container.querySelectorAll('*')].filter((el) => el.className.toString().includes('animate'))
  expect(animated, 'the tile animates its own figure').toHaveLength(0)
  view.unmount()
})

test('Skeleton shimmers as a travelling sweep, and is a shape rather than a figure', () => {
  const view = mount(<Skeleton className="h-8" />)
  const el = view.container.querySelector('div')!
  // Nothing in it can be read as a completion figure: no text at all.
  expect(el.textContent).toBe('')
  expect(el.getAttribute('aria-hidden')).toBe('true')
  // A SWEEP, not a fill: the band travels and never stops anywhere, so it
  // cannot be misread as a position the way a filling bar could.
  expect(el.className).toContain('motion-safe:animate-[skeleton-shimmer')
  expect(el.className).toContain('--skeleton-sweep')
  view.unmount()
})

test('EmptyState teaches: what will appear here, and why', () => {
  const view = mount(
    <EmptyState
      what="No pending approvals"
      why="Agents queue risky actions here for your sign-off."
      glyph="◇"
      action={<Button>Refresh</Button>}
    />,
  )
  const text = view.container.textContent ?? ''
  expect(text).toContain('No pending approvals')
  // The teaching half is the reason this primitive exists at all.
  expect(text).toContain('Agents queue risky actions here for your sign-off.')
  expect(text).toContain('Refresh')
  view.unmount()

  // `why` is optional, and its absence renders nothing rather than a blank line.
  const bare = mount(<EmptyState what="Nothing yet" />)
  expect(bare.container.querySelectorAll('p')).toHaveLength(1)
  bare.unmount()
})

// ── the Base UI behaviour layer, driven ────────────────────────────────────

function Harness({ kind }: { kind: 'modal' | 'drawer' }) {
  const [open, setOpen] = useState(false)
  return (
    <div>
      <Button onClick={() => setOpen(true)}>Open</Button>
      {kind === 'modal' ? (
        <Modal open={open} onOpenChange={setOpen} title="Confirm" description="This cannot be undone.">
          <p>Body copy</p>
        </Modal>
      ) : (
        <Drawer open={open} onOpenChange={setOpen} title="Details">
          <p>Body copy</p>
        </Drawer>
      )}
    </div>
  )
}

test('Modal opens through the real Base UI dialog and carries its title and description', () => {
  const view = mount(<Harness kind="modal" />)
  expect(document.body.textContent).not.toContain('Confirm')

  click(view.container.querySelector('button'))

  // Portalled, so it is asserted on the document rather than the container.
  const dialog = document.querySelector('[role=dialog]')
  expect(dialog, 'the modal did not open').not.toBeNull()
  expect(document.body.textContent).toContain('Confirm')
  expect(document.body.textContent).toContain('This cannot be undone.')
  expect(document.body.textContent).toContain('Body copy')
  view.unmount()
})

test('Drawer is the same dialog behaviour arriving from the side, and closes from its own control', () => {
  const view = mount(<Harness kind="drawer" />)
  click(view.container.querySelector('button'))

  expect(document.querySelector('[role=dialog]'), 'the drawer did not open').not.toBeNull()
  expect(document.body.textContent).toContain('Details')

  const close = [...document.querySelectorAll('button')].find((b) => b.textContent === 'Close')
  click(close)
  expect(document.querySelector('[role=dialog]'), 'the drawer did not close').toBeNull()
  view.unmount()
})

test('every tone in the kit is a token name, so no component can invent an eighth hue', () => {
  const known: Tone[] = ['accent', 'green', 'yellow', 'red', 'blue', 'orange', 'pink']
  expect(tones).toEqual(known)
})

// ── the toast stack, driven through the real Base UI subsystem ─────────────

/** Fires real toasts through the kit's own API — no stub anywhere: the
 *  provider, the portal, the viewport and the manager are Base UI's. */
function Fire({ toasts }: { toasts: { title: string; type: ToastSeverity }[] }) {
  const manager = useToast()
  return (
    <Button
      onClick={() => {
        for (const t of toasts) manager.add({ title: t.title, type: t.type })
      }}
    >
      Fire
    </Button>
  )
}

function ToastHarness({ toasts }: { toasts: { title: string; type: ToastSeverity }[] }) {
  return (
    <ToastProvider>
      <Fire toasts={toasts} />
    </ToastProvider>
  )
}

test('the toast stack caps at four visible and marks the overflow rather than dropping it', () => {
  const five: { title: string; type: ToastSeverity }[] = [
    { title: 'one', type: 'info' },
    { title: 'two', type: 'success' },
    { title: 'three', type: 'warning' },
    { title: 'four', type: 'error' },
    { title: 'five', type: 'info' },
  ]
  const view = mount(<ToastHarness toasts={five} />)
  click(view.container.querySelector('button'))

  // Portalled by Base UI, so the stack is read off the document. A toast ROOT
  // is Base UI's own shape — it carries the severity as `data-type` AND a
  // dialog role; the title and close button inside it carry `data-type` alone.
  const roots = [...document.querySelectorAll('[data-type][role]')]
  expect(roots.length, 'the real toast subsystem rendered nothing').toBe(five.length)

  // FOUR visible, and the fifth is marked `data-limited` rather than removed —
  // the overflow is hidden by CSS, so nothing a caller raised is silently lost.
  const limited = roots.filter((r) => r.hasAttribute('data-limited'))
  expect(roots.length - limited.length, 'more than four toasts are visible').toBe(4)
  expect(limited, 'the overflow was dropped instead of marked').toHaveLength(1)
  expect(limited[0].className, 'the limited toast is not hidden').toContain('data-limited:hidden')

  // Every one of them still carries its title: capping is a display rule.
  for (const t of five) expect(document.body.textContent).toContain(t.title)
  view.unmount()
})

test('a toast carries its severity as a tone token, not a colour', () => {
  const view = mount(
    <ToastHarness
      toasts={[
        { title: 'broke', type: 'error' },
        { title: 'worked', type: 'success' },
      ]}
    />,
  )
  click(view.container.querySelector('button'))

  const styleOf = (title: string) =>
    [...document.querySelectorAll('[data-type][role]')]
      .find((r) => (r.textContent ?? '').includes(title))
      ?.getAttribute('style') ?? ''
  expect(styleOf('broke'), 'error does not map to the red token').toContain('--tone: var(--red)')
  expect(styleOf('worked'), 'success does not map to the green token').toContain('--tone: var(--green)')
  view.unmount()
})

// ── the kit's own disciplines, scanned ────────────────────────────────────

const kitSources = import.meta.glob('./*.tsx', { query: '?raw', import: 'default', eager: true }) as Record<
  string,
  string
>

/** Comments out, code in: a doc comment is allowed to DISCUSS motion, and only
 *  a class string can start any. */
function code(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/(^|[^:])\/\/.*$/gm, '$1')
}

test('every motion utility in the kit is authored behind motion-safe', () => {
  // The stylesheet's own animations are gated in the stylesheet and checked by
  // design.test.ts. The kit starts its motion from UTILITIES instead, which that
  // detector cannot see — so the same rule is enforced here, at the one place
  // the kit can express it. The global reduce kill switch remains the backstop;
  // this is the authored gate the proposal asks for.
  const hits: string[] = []
  for (const [path, src] of Object.entries(kitSources)) {
    if (path.includes('.test.')) continue
    for (const m of code(src).matchAll(/(?:^|[\s'"`])((?:motion-safe:)?)((?:transition|animate)-[^\s'"`]+)/g)) {
      if (m[1] === '') hits.push(`${path}: ${m[2]}`)
    }
  }
  expect(hits, 'a kit component starts motion a reduce reader cannot turn off at the source').toEqual([])

  // Probe, both directions: the detector really fires, and really accepts a gate.
  const bad = { './x.tsx': "className={'transition-opacity duration-200'}" }
  const good = { './x.tsx': "className={'motion-safe:transition-opacity duration-200'}" }
  const scan = (files: Record<string, string>) =>
    Object.entries(files).flatMap(([p, s]) =>
      [...code(s).matchAll(/(?:^|[\s'"`])((?:motion-safe:)?)((?:transition|animate)-[^\s'"`]+)/g)]
        .filter((m) => m[1] === '')
        .map((m) => `${p}: ${m[2]}`),
    )
  expect(scan(bad)).toHaveLength(1)
  expect(scan(good)).toEqual([])
})

test('no kit component carries a raw colour value — colour lives in the token block', () => {
  // The §47 discipline, made checkable: a component names a TOKEN, so the
  // palette is re-pointable in one place and a hue cannot be forked by a box
  // that wanted a slightly different glass.
  const hits: string[] = []
  for (const [path, src] of Object.entries(kitSources)) {
    if (path.includes('.test.')) continue
    for (const m of code(src).matchAll(/(#[0-9a-fA-F]{3,8}\b|rgba?\(\s*\d)/g)) hits.push(`${path}: ${m[1]}`)
  }
  expect(hits, 'a kit component hardcodes a colour instead of naming a token').toEqual([])
  // Probe: the detector matches both spellings it forbids.
  expect(/(#[0-9a-fA-F]{3,8}\b|rgba?\(\s*\d)/.test('bg-[#07070d]')).toBe(true)
  expect(/(#[0-9a-fA-F]{3,8}\b|rgba?\(\s*\d)/.test('bg-[rgba(148,148,190,0.075)]')).toBe(true)
})
