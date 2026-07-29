import { DragDropContext, Draggable, Droppable } from '@hello-pangea/dnd'
import { AssistantRuntimeProvider, ThreadPrimitive, useLocalRuntime, useThread, type ChatModelAdapter } from '@assistant-ui/react'
import { JsonForms, JsonFormsDispatch, withJsonFormsControlProps, withJsonFormsLayoutProps } from '@jsonforms/react'
import { rankWith, uiTypeIs, type ControlProps, type JsonSchema, type UISchemaElement } from '@jsonforms/core'
import { Diff, Hunk, parseDiff } from 'react-diff-view'
import { afterEach, expect, test, vi } from 'vitest'

import { flush, mount } from './testing'

/**
 * The behavioral half of "live-verified at adoption" (S16.4 #5). One probe per
 * binding FC-v1 pick, at the pinned version, under React 19.2.8 and the pinned
 * DOM environment: import-compiles is not the bar — each of these renders.
 *
 * Every probe is fixture-fed and hermetic. Nothing here reaches the API, the
 * network, or any running unit.
 */

afterEach(() => {
  document.querySelectorAll('body > div').forEach((n) => n.remove())
})

// ── @hello-pangea/dnd 18.0.1 — the S15.5 board's drag (B6-5 wires it) ─────

test('@hello-pangea/dnd renders a draggable list under React 19', () => {
  const view = mount(
    <DragDropContext onDragEnd={() => {}}>
      <Droppable droppableId="lane-a">
        {(dropProvided) => (
          <ul ref={dropProvided.innerRef} {...dropProvided.droppableProps}>
            {['t-1', 't-2'].map((id, index) => (
              <Draggable key={id} draggableId={id} index={index}>
                {(dragProvided) => (
                  <li
                    ref={dragProvided.innerRef}
                    {...dragProvided.draggableProps}
                    {...dragProvided.dragHandleProps}
                  >
                    {id}
                  </li>
                )}
              </Draggable>
            ))}
            {dropProvided.placeholder}
          </ul>
        )}
      </Droppable>
    </DragDropContext>,
  )

  const items = [...view.container.querySelectorAll('li')]
  expect(items.map((li) => li.textContent)).toEqual(['t-1', 't-2'])
  // The library really took over the nodes: a draggable carries its own
  // data attribute and a drag handle, which is what B6-5 will hang the
  // drag→hint verb on.
  expect(items[0].getAttribute('data-rfd-draggable-id')).toBe('t-1')
  expect(items[0].getAttribute('data-rfd-drag-handle-draggable-id')).toBe('t-1')
  view.unmount()
})

// ── react-diff-view 3.3.3 (+ gitdiff-parser) — the S15.8 review surface ───

const unifiedDiff = `diff --git a/pipeline.go b/pipeline.go
index 1111111..2222222 100644
--- a/pipeline.go
+++ b/pipeline.go
@@ -3,7 +3,8 @@ package pipeline
 func Run(ctx context.Context) error {
 \tif err := prepare(ctx); err != nil {
-\t\treturn err
+\t\treturn fmt.Errorf("prepare: %w", err)
 \t}
+\tdefer cleanup()
 \treturn nil
 }
`

test('react-diff-view parses a real unified diff and renders its hunks', () => {
  const files = parseDiff(unifiedDiff)
  expect(files).toHaveLength(1)
  expect(files[0].newPath).toBe('pipeline.go')
  expect(files[0].hunks).toHaveLength(1)

  const view = mount(
    <Diff viewType="unified" diffType={files[0].type} hunks={files[0].hunks}>
      {(hunks) => hunks.map((hunk) => <Hunk key={hunk.content} hunk={hunk} />)}
    </Diff>,
  )

  const text = view.container.textContent ?? ''
  expect(text).toContain('return fmt.Errorf("prepare: %w", err)')
  expect(text).toContain('defer cleanup()')
  // Insert and delete lines are distinguishable in the DOM — the anchor model
  // B6-8 carries (file_path + side + line_no) needs exactly that.
  expect(view.container.querySelectorAll('.diff-code-insert').length).toBeGreaterThan(0)
  expect(view.container.querySelectorAll('.diff-code-delete').length).toBeGreaterThan(0)
  view.unmount()
})

// ── JSON Forms 3.8.0 — the S15.9 settings renderer (B6-6 owns the set) ────

// The vocabulary shape the B6-3A UISchema emitter really produces: a
// Categorization of Categories holding Controls, over a NESTED schema — a
// dotted ⚙ key `a.b` emits as `#/properties/a/properties/b`
// (internal/settings/uischema.go), because a dot inside a single property name
// would be read as a path separator on the way to the data. Fixture-fed,
// never API-fed.
const settingsSchema: JsonSchema = {
  type: 'object',
  properties: {
    orchestration: {
      type: 'object',
      properties: { helper_turns: { type: 'integer', minimum: 1, maximum: 50 } },
    },
    obs: {
      type: 'object',
      properties: { sse_keepalive: { type: 'integer' } },
    },
  },
}

const settingsUISchema: UISchemaElement = {
  type: 'Categorization',
  elements: [
    {
      type: 'Category',
      label: 'Orchestration',
      elements: [{ type: 'Control', scope: '#/properties/orchestration/properties/helper_turns' }],
    },
    {
      type: 'Category',
      label: 'Observability',
      elements: [{ type: 'Control', scope: '#/properties/obs/properties/sse_keepalive' }],
    },
  ],
} as UISchemaElement

type Layout = UISchemaElement & { label?: string; elements?: UISchemaElement[] }

/** The Sinet-owned renderer SET is B6-6's work. This probe carries only the
 *  renderers the vocabulary needs — enough to prove the library dispatches
 *  Categorization, Category and Control at the pinned version. */
const CategorizationRenderer = withJsonFormsLayoutProps(({ uischema, schema, path, renderers, cells }) => (
  <div>
    {((uischema as Layout).elements ?? []).map((child, i) => (
      <JsonFormsDispatch
        key={i}
        uischema={child}
        schema={schema}
        path={path}
        renderers={renderers}
        cells={cells}
      />
    ))}
  </div>
))

const CategoryRenderer = withJsonFormsLayoutProps(({ uischema, schema, path, renderers, cells }) => (
  <fieldset>
    <legend>{(uischema as Layout).label}</legend>
    {((uischema as Layout).elements ?? []).map((child, i) => (
      <JsonFormsDispatch
        key={i}
        uischema={child}
        schema={schema}
        path={path}
        renderers={renderers}
        cells={cells}
      />
    ))}
  </fieldset>
))

const ControlRenderer = withJsonFormsControlProps(({ data, label, handleChange, path }: ControlProps) => (
  <label>
    {label}
    <input value={String(data ?? '')} onChange={(e) => handleChange(path, Number(e.target.value))} />
  </label>
))

test('JSON Forms renders a Categorization + Control fixture', () => {
  const renderers = [
    { tester: rankWith(3, uiTypeIs('Categorization')), renderer: CategorizationRenderer },
    { tester: rankWith(3, uiTypeIs('Category')), renderer: CategoryRenderer },
    { tester: rankWith(3, uiTypeIs('Control')), renderer: ControlRenderer },
  ]

  const view = mount(
    <JsonForms
      schema={settingsSchema}
      uischema={settingsUISchema}
      data={{ orchestration: { helper_turns: 25 }, obs: { sse_keepalive: 20 } }}
      renderers={renderers}
      onChange={() => {}}
    />,
  )

  const text = view.container.textContent ?? ''
  expect(text).toContain('Orchestration')
  expect(text).toContain('Observability')
  const inputs = [...view.container.querySelectorAll('input')]
  expect(inputs.map((i) => i.value)).toEqual(['25', '20'])
  view.unmount()
})

// ── @assistant-ui/react 0.14.27 — the S15.7 chat surface (B6-7 wires it) ──

test('@assistant-ui/react round-trips a turn through LocalRuntime with no cloud and no network', async () => {
  const fetchSpy = vi.fn(() => Promise.reject(new Error('a probe must not reach the network')))
  vi.stubGlobal('fetch', fetchSpy)

  // A ChatModelAdapter that answers from memory. LocalRuntime is the documented
  // "connect a custom backend" seam; at B6-7 the backend behind it becomes the
  // platform's own API and SSE cursor, and message state stays platform-owned.
  const adapter: ChatModelAdapter = {
    run: async function* () {
      yield { content: [{ type: 'text' as const, text: 'from the platform, not a cloud' }] }
    },
  }

  let send: ((text: string) => void) | null = null

  function Transcript() {
    const messages = useThread((t) => t.messages)
    return (
      <ol>
        {messages.map((m, i) => (
          <li key={i}>
            {m.role}: {m.content.map((c) => (c.type === 'text' ? c.text : '')).join('')}
          </li>
        ))}
      </ol>
    )
  }

  function Chat() {
    const runtime = useLocalRuntime(adapter)
    send = (text) => {
      void runtime.thread.append({ role: 'user', content: [{ type: 'text', text }] })
    }
    return (
      <AssistantRuntimeProvider runtime={runtime}>
        <ThreadPrimitive.Root>
          <ThreadPrimitive.Viewport>
            <Transcript />
          </ThreadPrimitive.Viewport>
        </ThreadPrimitive.Root>
      </AssistantRuntimeProvider>
    )
  }

  const view = mount(<Chat />)
  send!('hello')
  await flush(6)

  const turns = [...view.container.querySelectorAll('li')].map((li) => li.textContent)
  expect(turns).toEqual(['user: hello', 'assistant: from the platform, not a cloud'])

  // The checkable negative FC-v1/S15.4 asks for. assistant-cloud is IN the tree
  // — it is a hard dependency of this package — and it is never wired and never
  // called: a whole turn completed without a single request leaving the client.
  expect(fetchSpy).not.toHaveBeenCalled()
  view.unmount()
  vi.unstubAllGlobals()
})
