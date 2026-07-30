import { ApiError, Unreachable, type ChatTurn } from './api'

/**
 * The assistant's two pinned tables (Spec S15.7 carried Jarvis behavior specs;
 * FC-v1 §3; P3/CONVENTIONS.md §38).
 *
 *  1. THE SURVIVES-NAVIGATION INVENTORY — an explicit list of what a view switch
 *     may shed versus what MUST survive it, in code so a test can pin it rather
 *     than in prose nobody can check.
 *  2. THE ERROR-HUMANIZATION TABLE — one mapping, keyed on the CLASS of the
 *     failure and on status/code, NEVER on message text (§38). The served
 *     `{error, detail}` is preserved and rendered; this module classifies, it
 *     never re-words what the platform said.
 *
 * They live together because they answer the same question from two sides: what
 * a person can still see after something goes wrong or they walk away.
 */

// ── R14: the survives-navigation inventory ────────────────────────────────

export type SurvivalRow = {
  /** What the person could lose. */
  thing: string
  /** Whether leaving `/chat` and coming back keeps it. */
  survives: boolean
  /** WHY it survives or why shedding it is right. Not decoration: a row whose
   *  reason is "we cached it" would be the client-side side-truth this whole
   *  contract exists to rule out. */
  because: string
}

/**
 * The inventory. Everything that survives does so because it is PLATFORM state
 * re-read from the API or a fact carried in the URL — nothing here survives by
 * being kept in the client, and nothing is kept in web storage at all.
 *
 * Read the `false` rows as deliberate: a thing that is not platform state must
 * not be made to look like it is.
 *
 * HARD-STOP IS AN ACT, NOT A SIDE EFFECT (OQ6), and that is why nothing in this
 * table is conditional on leaving. Navigation calls no verb: there is no unload
 * handler and no cleanup that posts anything. A person who walks away has not
 * stopped their turn — it is still running on the server and is there when they
 * come back — and a person who stopped one pressed something that says so. The
 * guarantee is DRIVEN (a round trip that posts nothing, plus a tree scan for
 * unload-family handlers); it is deliberately not restated here as a boolean
 * constant, because a constant nothing reads pins nothing.
 */
export const survivesNavigation: readonly SurvivalRow[] = [
  {
    thing: 'the in-flight turn',
    survives: true,
    because:
      'a running turn is a server-side row, served as `running` on the session detail — the view re-attaches by re-reading it rather than by remembering it',
  },
  {
    thing: "the in-flight turn's progress",
    survives: true,
    because:
      'progress IS the turn\'s recorded state (running → settled | abandoned); there is no client-side progress to lose, and nothing streams that could be missed',
  },
  {
    thing: 'the produced-files accumulation',
    survives: true,
    because:
      'the chips are a manifest diff computed server-side at settle and unioned at read time from each file\'s origin_turn — the accumulation lives in the manifest, not in this tab',
  },
  {
    thing: 'the transcript',
    survives: true,
    because: 'messages and turn outcomes are table-backed platform state; a reload restores them and a second tab sees the same thing',
  },
  {
    thing: 'which conversation is open',
    survives: true,
    because: 'the session id is in the URL (?session=…), so a deep link, a bookmark and a hard reload all land on the same conversation',
  },
  {
    thing: "the tab's one EventSource",
    survives: true,
    because:
      'the stream is held at module scope and this view subscribes to it by event TYPE; a view switch unsubscribes and re-subscribes without re-opening the connection',
  },
  {
    thing: 'the composer draft',
    survives: false,
    because:
      'unsent text is not platform state, and carrying it across a view switch would be a client-side side-truth that no re-snapshot could correct',
  },
  {
    thing: "the composer's verb pick",
    survives: false,
    because:
      'a routing choice belongs to the turn it routes; a pick that outlived its turn would send the next question under the last one\'s gesture',
  },
  {
    thing: 'the uploaded-file list',
    survives: false,
    because: 'it is re-read from the manifest, which is the record — the list is a render of it, never a copy of it',
  },
  {
    thing: 'a request-level notice (sending, or a failure)',
    survives: false,
    because:
      "it describes THIS tab's own outstanding or finished request, not the conversation; the turn's recorded state is what survives, and that is served",
  },
]

// ── R15: the error-humanization table ─────────────────────────────────────

/**
 * The classes a failure can land in. They are the classes that EXIST on this
 * surface — every one is reachable from a landed status or a landed error code,
 * and none was invented to look thorough.
 */
export type ChatFailureClass =
  | 'transport'
  | 'signed-out'
  | 'not-found'
  | 'turn-running'
  | 'too-large'
  | 'over-quota'
  | 'overloaded'
  | 'refused'
  | 'not-wired'
  | 'server'

export type ChatFailure = {
  class: ChatFailureClass
  /** Plain language, ours. */
  headline: string
  /** The platform's own words, kept verbatim — never re-written, never dropped. */
  detail: string
}

/**
 * describeChatFailure maps a REQUEST failure onto one rendered fact.
 *
 * A TRANSPORT failure and a SERVER answer are different things and a person acts
 * differently on each, so they never collapse into one banner. Everything below
 * keys on the exception CLASS, the HTTP STATUS and the server's own `code`;
 * `err.message` is only ever carried through as the served detail, because
 * re-classifying a failure by reading its prose is the server's job done badly
 * in the wrong place (§38).
 */
export function describeChatFailure(err: unknown): ChatFailure {
  if (err instanceof Unreachable) {
    return {
      class: 'transport',
      headline: 'The control plane is unreachable — the host may be asleep or the tailnet down.',
      detail: 'Nothing was sent. Your conversation is server-side, so nothing is lost.',
    }
  }
  if (!(err instanceof ApiError)) {
    return { class: 'server', headline: 'The control plane could not be read.', detail: '' }
  }
  const detail = err.message
  if (err.code === 'not_wired') {
    return { class: 'not-wired', headline: 'That part of the platform is not wired in this process.', detail }
  }
  switch (err.status) {
    case 401:
      return { class: 'signed-out', headline: 'Your session ended — sign in again to continue.', detail }
    case 403:
      return { class: 'refused', headline: 'The platform refused that.', detail }
    case 404:
      return { class: 'not-found', headline: 'That is not there, or it is not yours.', detail }
    case 409:
      if (err.code === 'turn_running') {
        return {
          class: 'turn-running',
          headline: 'This conversation already has a turn in flight — it is still running above.',
          detail,
        }
      }
      return { class: 'over-quota', headline: 'The platform refused that on one of its own bounds.', detail }
    case 413:
      return { class: 'too-large', headline: 'That is larger than the exchange folder accepts.', detail }
    case 429:
    case 503:
      // The overload class. 429 is the rate answer and 503 is the unavailable
      // one; both mean "not now, and not because of anything you did", and both
      // keep the served detail because it is the only thing that says how long.
      return { class: 'overloaded', headline: 'The platform is busy or unavailable right now — try again shortly.', detail }
    case 400:
      return { class: 'refused', headline: 'The platform refused that request.', detail }
    default:
      return { class: 'server', headline: `The control plane answered ${err.status}.`, detail }
  }
}

// ── what a served turn IS ─────────────────────────────────────────────────

/**
 * The facts a recorded turn can be. `empty` is the one worth naming: a turn that
 * SETTLED without recording an outcome is a real served state and a different
 * fact from a request that never landed — the "empty-stream vs transport-error"
 * distinction, in a surface where answers arrive whole rather than as a stream.
 */
export type TurnFactKind = 'running' | 'abandoned' | 'answer' | 'task' | 'error' | 'empty'

export type TurnFact = {
  kind: TurnFactKind
  /** Plain language for the states that are not themselves a rendered answer. */
  headline: string
}

/** turnFact reads the turn's own recorded state and outcome kind — its fields,
 *  never its prose. */
export function turnFact(turn: ChatTurn): TurnFact {
  if (turn.state === 'running') {
    return { kind: 'running', headline: 'Working on it — this turn is running on the platform, not in this tab.' }
  }
  if (turn.state === 'abandoned') {
    return {
      kind: 'abandoned',
      headline: 'You stopped this turn. The record of it stands, and any result that arrived late is shown with it.',
    }
  }
  if (turn.outcome === undefined || turn.outcome === null || turn.outcome_kind === undefined || turn.outcome_kind === '') {
    return { kind: 'empty', headline: 'This turn settled without recording an outcome — nothing came back to show.' }
  }
  switch (turn.outcome_kind) {
    case 'answer':
      return { kind: 'answer', headline: '' }
    case 'task':
      return { kind: 'task', headline: '' }
    default:
      return { kind: 'error', headline: 'This turn could not be completed.' }
  }
}

/** The served `{error, detail}` envelope a failed turn recorded. Read as data:
 *  the surface prints what the platform wrote, and adds nothing. */
export type TurnError = { error?: string; detail?: string }
