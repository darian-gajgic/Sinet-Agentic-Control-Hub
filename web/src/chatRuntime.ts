import type { ThreadMessageLike } from '@assistant-ui/react'

import type { ChatMessage, ChatSessionView, ChatTurn, ChatTurnRequest } from './api'

/**
 * The Sinet adapter's data half (Spec S15.4/S15.7; FC-v1 §3; OQ5(i)).
 *
 * @assistant-ui/react runs here on **ExternalStoreRuntime**, and that choice is
 * the load-bearing one: the store the widget reads is the PLATFORM's served
 * state, so the widget holds no transcript of its own. A reload restores the
 * conversation because the conversation was never in the browser, and swapping
 * the widget for owned primitives later loses no data — which is precisely the
 * swap FC-v1 named in advance as this pick's fallback.
 *
 * ASSISTANT-CLOUD IS NEVER WIRED AND NEVER CALLED. Nothing in this module or its
 * consumers imports it or builds a cloud config, and a driven turn makes no
 * request the platform did not answer. It sits in `node_modules` because the
 * pinned package depends on it; the negative is about wiring, and it is checked.
 *
 * The functions below are PURE, deliberately: the mapping from served rows to
 * thread messages is the part most worth testing directly, and a pure function
 * cannot smuggle a fetch into a render.
 */

/**
 * One row of the feed.
 *
 * A turn and the message that opened it are separate items because they are
 * separate records — the message is what the person said, the turn is what the
 * platform did about it, and the transcript reads as a conversation only if both
 * appear. `key` is the underlying row's own id, so the widget's message identity
 * IS the platform's identity and nothing has to be generated.
 */
export type ChatFeedItem =
  | { key: string; role: 'user'; said: ChatMessage }
  | { key: string; role: 'assistant'; turn: ChatTurn; question: string }

/**
 * chatFeed orders one session's served state into the feed.
 *
 * Messages carry their `turn_id`, so the pairing is the platform's rather than
 * positional: each message is followed by ITS turn. A turn with no message cannot
 * happen (a turn always writes the message that opened it), and a message whose
 * turn is missing still renders — the person's own words are never dropped
 * because the record beside them is incomplete.
 */
export function chatFeed(view: ChatSessionView): ChatFeedItem[] {
  const turns = new Map(view.turns.map((t) => [t.turn_id, t]))
  const out: ChatFeedItem[] = []
  const placed = new Set<string>()
  for (const said of view.messages) {
    out.push({ key: said.message_id, role: 'user', said })
    const turn = turns.get(said.turn_id)
    if (turn) {
      out.push({ key: turn.turn_id, role: 'assistant', turn, question: said.body })
      placed.add(turn.turn_id)
    }
  }
  // A turn whose message did not come back in the same bounded read still
  // belongs on screen: dropping it would hide an outcome the platform recorded.
  for (const turn of view.turns) {
    if (!placed.has(turn.turn_id)) out.push({ key: turn.turn_id, role: 'assistant', turn, question: '' })
  }
  return out
}

/**
 * convertChatItem is the ExternalStoreRuntime message converter.
 *
 * An assistant item carries NO text part. That is not an omission: a turn's
 * outcome is a table, a card, a born task or an audited refusal — structured
 * records that the surface renders through the platform's own answer renderer,
 * with the layer, the confidence flag and the audit block intact. Flattening one
 * into a sentence here would be the surface authoring what the platform said.
 *
 * The user item's text IS the person's stored words, and it renders through the
 * widget's own text part — escaped, like everything else (S15.12).
 */
export function convertChatItem(item: ChatFeedItem): ThreadMessageLike {
  if (item.role === 'user') {
    return { role: 'user', id: item.key, content: [{ type: 'text', text: item.said.body }] }
  }
  return {
    role: 'assistant',
    id: item.key,
    content: [],
    status: item.turn.state === 'running' ? { type: 'running' } : { type: 'complete', reason: 'stop' },
  }
}

/** The verbs the composer can route to. Open SQL is NOT here: Layer 2 is reached
 *  only by an explicit escalation act on an answer, so it can never be the verb a
 *  typed sentence falls into (OQ3(a) — act, not fallback). */
export const composerVerbs = [
  { kind: 'ask', label: 'Ask about the platform' },
  { kind: 'view', label: 'Run a named view (Layer 0)' },
  { kind: 'query', label: 'Run a catalog question (Layer 1)' },
  { kind: 'task', label: 'Start a task' },
] as const

export type ComposerVerb = (typeof composerVerbs)[number]['kind']

/** What the composer has been set to, beside the text. */
export type ComposerPick = {
  verb: ComposerVerb
  view: string
  query: string
  slots: Record<string, string>
  title: string
  /** Exchange ids ticked to hand to a born task. */
  inputs: string[]
}

export const emptyPick: ComposerPick = { verb: 'ask', view: '', query: '', slots: {}, title: '', inputs: [] }

/**
 * turnRequestFor builds the turn body from what the person typed and the gesture
 * they chose. It is the whole of the client's routing (OQ3(a)): the verb travels
 * as data, NO MODEL CLASSIFIES IT, and a verb's arguments only ride along when
 * that verb is the one selected — so a stale view pick cannot leak onto an ask.
 */
export function turnRequestFor(pick: ComposerPick, text: string): ChatTurnRequest {
  switch (pick.verb) {
    case 'view':
      return { kind: 'view', text, view: pick.view }
    case 'query': {
      const slots: Record<string, string> = {}
      for (const [k, v] of Object.entries(pick.slots)) if (v !== '') slots[k] = v
      return { kind: 'query', text, query: pick.query, slots }
    }
    case 'task': {
      const body: ChatTurnRequest = { kind: 'task', text }
      if (pick.title !== '') body.title = pick.title
      if (pick.inputs.length > 0) body.inputs = pick.inputs
      return body
    }
    default:
      return { kind: 'ask', text }
  }
}

/**
 * argumentReady says whether the chosen verb has the argument it needs.
 *
 * A Layer-0 or Layer-1 turn with nothing picked is a request the server can only
 * refuse, so sending does not arm at all — the widget's own `isSendDisabled` gate
 * carries it, which keeps the input usable while the act stays unavailable. The
 * honest alternative would be to send it and render the refusal, which teaches a
 * person nothing they could not have been told first.
 */
export function argumentReady(pick: ComposerPick): boolean {
  if (pick.verb === 'view') return pick.view !== ''
  if (pick.verb === 'query') return pick.query !== ''
  return true
}

/** textOf lifts the typed words out of an assistant-ui append. Only text parts
 *  exist on this composer — no attachment adapter is wired, because a file
 *  belongs to the exchange manifest and reaches a task as an input descriptor,
 *  never as a blob riding a message. */
export function textOf(parts: readonly { readonly type: string }[]): string {
  return parts
    .filter((p): p is { type: 'text'; text: string } => p.type === 'text')
    .map((p) => p.text)
    .join('')
}
