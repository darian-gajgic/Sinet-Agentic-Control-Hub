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
  cn,
  tones,
  type Tone,
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

test('Skeleton is a shape, never a figure', () => {
  const view = mount(<Skeleton className="h-8" />)
  const el = view.container.querySelector('div')!
  // Nothing in it can be read as a completion figure: no text at all, and the
  // shimmer is motion-gated.
  expect(el.textContent).toBe('')
  expect(el.getAttribute('aria-hidden')).toBe('true')
  expect(el.className).toContain('motion-safe:animate-pulse')
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
