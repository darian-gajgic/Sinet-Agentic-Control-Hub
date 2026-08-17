import { useCallback, useEffect, useRef, useState, type DragEvent } from 'react'
import {
  AssistantRuntimeProvider,
  ComposerPrimitive,
  MessagePrimitive,
  ThreadPrimitive,
  useExternalStoreRuntime,
  type AppendMessage,
} from '@assistant-ui/react'

import {
  api,
  type Answer,
  type ApprovalItem,
  type ChatBornTask,
  type ChatFile,
  type ChatSession,
  type ChatSessionView,
  type ChatTurn,
  type ChatTurnRequest,
  type HistoryRegistry,
  type Session,
} from './api'
import {
  argumentReady,
  chatFeed,
  composerVerbs,
  convertChatItem,
  emptyPick,
  textOf,
  turnRequestFor,
  type ChatFeedItem,
  type ComposerPick,
  type ComposerVerb,
} from './chatRuntime'
import { describeChatFailure, turnFact, type ChatFailure, type TurnError } from './chatFacts'
import { AnswerView } from './History'
import { type EventStream } from './events'
import {
  chatFileEventTypes,
  chatIntakeEventTypes,
  chatSessionEventTypes,
  chatTurnEventTypes,
  describeError,
  useLive,
} from './live'
import { Absent, Empty, Freshness, SurfaceHead } from './parts'
import { Link, navigate } from './router'
import { hrefFor } from './routes'
import { SignInFirstDoor } from './signinfirst'
import { Button, EmptyState, Timestamp } from './ui'
import { Hint } from './hints'
import { cn } from './lib/utils'

/**
 * The three regions' shared panel treatment (P3-UI-7 R8).
 *
 * The kit's `Panel` is deliberately NOT mounted here: it wraps its children in
 * an inner padded `<div>`, and this surface's landed assertions traverse the
 * region elements directly (`.chat-sessions li`, `.chat-files li`, the viewport's
 * own children). Carrying the SAME look through utilities restyles the regions
 * without moving one node of the tree those pins walk — the §51 "class kept as
 * the DOM hook, presentation moved to utilities" resolution, applied to a
 * primitive rather than to a rule.
 */
const chatPanel =
  'max-w-full min-w-0 rounded-(--radius) border border-border bg-(image:--panel-grad) p-(--density-pad)'

/**
 * The conversational assistant (Spec S15.7; S15.4; FC-v1 §3; S14.10).
 *
 * FOUR RULES THIS SURFACE IS BUILT ON, all of them ratified rather than invented
 * here:
 *
 *  1. THE CLIENT ROUTES, THE SERVER ACTS (OQ3(a)). Every turn carries the verb
 *     the person's own gesture chose, from the closed set ask|view|query|
 *     open_sql|task. No model classifies a turn: a misfiring classifier could
 *     start a task nobody asked for or escalate to open SQL nobody escalated, and
 *     neither is a failure a person sees coming. Layer 2 is reached ONLY by
 *     pressing the escalation control on an answer, and a task starts ONLY by
 *     choosing to start one. Typed prose with no gesture is an `ask` — the
 *     Layer-1 floor, whose own disambiguation card IS the honest answer when a
 *     turn is none of the three S15.7 verbs.
 *  2. THE STATE IS THE PLATFORM'S (OQ5(i)). @assistant-ui/react runs on
 *     ExternalStoreRuntime over the served session, so this tab holds no
 *     transcript. A reload restores the conversation, a second tab sees it, and
 *     nothing here is written to web storage. assistant-cloud is never wired and
 *     never called.
 *  3. AN ANSWER IS RENDERED BY THE PLATFORM'S OWN RENDERER. `AnswerView` — the
 *     same component the history panel uses — renders the layer, the confidence
 *     with its lower-confidence flag, the notes, the audit block and the pickable
 *     disambiguation choices. A second renderer here would be a second place for
 *     the G3 D3.5 flag to be forgotten.
 *  4. STOPPING IS AN ACT (OQ6). The stop control calls the abandon verb.
 *     NAVIGATION CALLS NOTHING: no unload handler, no cleanup verb, no cancel on
 *     view switch. Walking away leaves the turn running on the platform, which is
 *     what makes it there when you come back.
 */
export function Chat({
  stream,
  search,
  session,
  onSignedIn,
}: {
  stream?: EventStream
  search: string
  /** The current identity, when the shell passes it (W1-B1: chat is the
   *  second give-work door — it hands work to intake, and its conversations
   *  are owned like tasks are). */
  session?: Session
  onSignedIn?: () => void
}) {
  const active = new URLSearchParams(search).get('session') ?? ''

  // THE SIGN-IN-FIRST WALL (W1-B1): under the dev fallback the give-work door
  // signs you in IN PLACE before any conversation or task exists here. Same
  // wall as Describe-a-goal; same return-by-construction (the address, session
  // param included, is untouched). All hooks below run unconditionally — this
  // gate renders INSTEAD of the surface, after every hook has been called.
  const devWalled = session?.dev === true && onSignedIn !== undefined

  const sessions = useLive<{ sessions: ChatSession[] }>({
    key: 'chat-sessions',
    read: () => api.chatSessions(),
    types: chatSessionEventTypes,
    stream,
  })
  const files = useLive<{ files: ChatFile[] }>({
    key: 'chat-files',
    read: () => api.chatFiles(),
    types: chatFileEventTypes,
    stream,
  })

  const [failure, setFailure] = useState<ChatFailure | null>(null)
  const reloadSessions = sessions.reload
  const reloadFiles = files.reload

  const perform = useCallback(
    async (run: () => Promise<unknown>, after: () => void) => {
      setFailure(null)
      try {
        await run()
      } catch (err) {
        setFailure(describeChatFailure(err))
        return
      }
      after()
    },
    [],
  )

  if (devWalled && session !== undefined && onSignedIn !== undefined) {
    return (
      <section className="chat flex max-w-full flex-col gap-4" data-live="assistant">
        <SignInFirstDoor
          session={session}
          onSignedIn={onSignedIn}
          doorWords="This is a give-work door: a conversation here can hand something over as a task, and conversations themselves belong to an account."
        />
      </section>
    )
  }

  return (
    <section className="chat flex max-w-full flex-col gap-4" data-live="assistant">
      {/* THE THREE-VERB CONTRACT, in operator words (B6 gate record §9 finding
          A-2; P3-UI-7 R7c). The landed line named two of the three and left out
          file exchange, which is a verb this surface really has
          (`chatRuntime.ts`'s composer set + the upload gestures). What it must
          NOT say is what the surface cannot do: there is no free-form
          generation here by S15.7 / S14.10 design, and the assistant edits
          nothing — it asks, it hands work to intake, and it exchanges files. */}
      <header className="chat-head">
        <SurfaceHead
          title="Assistant"
          what="Three things happen here: ask about the platform and the work it is doing, hand something over as a task for intake to shape, or exchange files with it. Conversations live on the platform, so they are here when you come back — on this device or another."
        />
        {failure !== null && <FailureLine failure={failure} />}
      </header>

      <div className="chat-body">
        <SessionRail
          // SERVED-GATED (§51 drain D1). `?? []` would render a read that has
          // not answered yet as a served empty, so the rail would teach "no
          // conversations yet" while /api/chat/sessions was still in flight.
          // `null` is the not-answered state and `Freshness` owns that window.
          sessions={sessions.data?.sessions ?? null}
          active={active}
          stale={sessions.stale}
          error={sessions.error}
          onCreate={() =>
            void perform(
              async () => {
                const made = await api.createChatSession()
                navigateToSession(made.session.session_id)
              },
              reloadSessions,
            )
          }
          onRename={(id, title) => void perform(() => api.renameChatSession(id, title), reloadSessions)}
          onDelete={(id) =>
            void perform(() => api.deleteChatSession(id), () => {
              if (id === active) navigateToSession('')
              reloadSessions()
            })
          }
        />

        {active === '' ? (
          <div className={cn(chatPanel, 'chat-thread')}>
            <Empty what="No conversation open. Pick one from the list, or start a new one." />
          </div>
        ) : (
          <Conversation
            key={active}
            id={active}
            stream={stream}
            files={files.data?.files ?? []}
            onFilesChanged={reloadFiles}
            onSessionChanged={reloadSessions}
          />
        )}

        <Exchange
          files={files.data?.files ?? []}
          stale={files.stale}
          error={files.error}
          onUpload={(file) => void perform(() => api.uploadChatFile(file.name, file), reloadFiles)}
          onDelete={(id) => void perform(() => api.deleteChatFile(id), reloadFiles)}
        />
      </div>
    </section>
  )
}

/** The active conversation is a URL fact, so a deep link, a bookmark and a hard
 *  reload all land on the same one (S15.11). The path comes from the route table
 *  — the surface never assembles it — and the session rides the query string, the
 *  landed `/?view=…` shape, rather than adding a route the table does not have. */
function sessionHref(id: string): string {
  return id === '' ? hrefFor('chat') : `${hrefFor('chat')}?session=${encodeURIComponent(id)}`
}

function navigateToSession(id: string): void {
  navigate(sessionHref(id))
}

function FailureLine({ failure }: { failure: ChatFailure }) {
  return (
    <p className="error" data-failure={failure.class}>
      {failure.headline}
      {failure.detail !== '' && <span className="muted"> {failure.detail}</span>}
    </p>
  )
}

// ── the session rail (R16) ────────────────────────────────────────────────

function SessionRail({
  sessions,
  active,
  stale,
  error,
  onCreate,
  onRename,
  onDelete,
}: {
  sessions: ChatSession[] | null
  active: string
  stale: boolean
  error: string
  onCreate: () => void
  onRename: (id: string, title: string) => void
  onDelete: (id: string) => void
}) {
  const [renaming, setRenaming] = useState('')
  const [draft, setDraft] = useState('')
  // A hard delete takes the whole thread and there is no undo verb, so the act
  // is two-pressed: the second press is the one that means it.
  const [armed, setArmed] = useState('')

  return (
    <nav className={cn(chatPanel, 'chat-sessions')} aria-label="Conversations">
      <div className="chat-sessions-head flex flex-wrap items-baseline justify-between gap-2">
        <h3>Conversations</h3>
        <Button variant="primary" size="sm" onClick={onCreate}>
          New
        </Button>
      </div>
      <Freshness stale={stale} error={error} hasData={(sessions ?? []).length > 0} />
      {sessions !== null && sessions.length === 0 && (
        <EmptyState
          what="No conversations yet."
          why="Starting one puts it here and keeps it — conversations live on the platform, not in this browser, so they are here on your next visit and on your other devices."
        />
      )}
      <ul className="m-0 flex list-none flex-col gap-2 p-0">
        {(sessions ?? []).map((s) => (
          <li
            key={s.session_id}
            data-session-id={s.session_id}
            data-active={s.session_id === active ? 'true' : 'false'}
            className={cn(
              'flex flex-col gap-1 wrap-anywhere',
              // The active conversation carries the identity rail, exactly as the
              // shed `li[data-active='true']` rule drew it.
              s.session_id === active && 'border-s-[3px] border-[var(--accent)] ps-2',
            )}
          >
            <Link to={sessionHref(s.session_id)} aria-current={s.session_id === active ? 'page' : undefined}>
              <SessionTitle session={s} />
            </Link>
            <span className="chat-session-acts flex flex-wrap items-center gap-2">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => { setRenaming(s.session_id); setDraft(s.title) }}
              >
                Rename
              </Button>
              {armed === s.session_id ? (
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => { setArmed(''); onDelete(s.session_id) }}
                >
                  Confirm delete
                </Button>
              ) : (
                <Button variant="ghost" size="sm" onClick={() => setArmed(s.session_id)}>
                  Delete
                </Button>
              )}
            </span>
            {renaming === s.session_id && (
              <span className="chat-rename flex flex-wrap items-center gap-2">
                <input
                  type="text"
                  aria-label="Conversation title"
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                />
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => {
                    setRenaming('')
                    onRename(s.session_id, draft)
                  }}
                >
                  Save
                </Button>
              </span>
            )}
          </li>
        ))}
      </ul>
    </nav>
  )
}

/** An untitled conversation says so. The emptiness is the honest state the store
 *  serves until a first message names it — never a placeholder, and never a
 *  fabricated summary of a conversation that has not happened. */
function SessionTitle({ session }: { session: ChatSession }) {
  if (session.title === '') return <Absent reason="Untitled — nothing said yet" />
  return <span className="chat-session-title wrap-anywhere">{session.title}</span>
}

// ── one conversation ──────────────────────────────────────────────────────

function Conversation({
  id,
  stream,
  files,
  onFilesChanged,
  onSessionChanged,
}: {
  id: string
  stream?: EventStream
  files: ChatFile[]
  onFilesChanged: () => void
  onSessionChanged: () => void
}) {
  const detail = useLive<ChatSessionView>({
    key: `chat-session:${id}`,
    read: () => api.chatSession(id),
    types: chatTurnEventTypes,
    stream,
  })
  // The person's own decision queue, read here because a handoff turn renders the
  // born task's open intake card and THAT card is never in the turn's outcome (see
  // `openIntakeCard`). It is the same bounded read the inbox does, and it is what
  // makes the card appear when the pipeline issues it and disappear when it is
  // answered — from here or from the inbox.
  const queue = useLive<{ items: ApprovalItem[] }>({
    key: 'chat-intake-queue',
    read: () => api.approvals(),
    types: chatIntakeEventTypes,
    stream,
  })
  const registries = useRegistries()

  const [pick, setPick] = useState<ComposerPick>(emptyPick)
  const [failure, setFailure] = useState<ChatFailure | null>(null)
  const [sending, setSending] = useState(false)

  const view = detail.data
  const running = view?.running ?? null
  const feed = view ? chatFeed(view) : []

  // The pick is read through a ref inside the adapter callbacks so a turn always
  // carries the gesture that was on screen when it was sent, whatever React has
  // re-rendered since.
  const pickRef = useRef(pick)
  pickRef.current = pick

  const reload = detail.reload
  const reloadQueue = queue.reload
  // Answering the born task's card moves BOTH the conversation (the pipeline
  // advances) and the queue (the card is resolved). The `decision.recorded` frame
  // says so too, but the person who just pressed the button should not have to
  // wait for a round trip through the relay to see their own act.
  const answered = useCallback(() => {
    reload()
    reloadQueue()
  }, [reload, reloadQueue])
  const submit = useCallback(
    async (req: ChatTurnRequest) => {
      setFailure(null)
      setSending(true)
      const pending = api.chatTurn(id, req)
      // A turn verb is SYNCHRONOUS, so this POST does not answer until the whole
      // act has settled — but the server wrote the message and opened the turn
      // before the act began, so both are already readable. Re-read now rather
      // than making a person watch nothing happen for the length of their own
      // question. It is best effort by design: the reliable trigger is the
      // `chat.turn_started` frame on the relay (REST is the truth, frames are the
      // trigger), and a read that raced the commit simply comes back unchanged.
      reload()
      try {
        await pending
      } catch (err) {
        setFailure(describeChatFailure(err))
      } finally {
        setSending(false)
        // THE GESTURE DIES WITH ITS TURN. A pick that outlived the turn it routed
        // would send the NEXT typed line under the last one's verb — and for
        // `task` that means starting a task nobody asked for, which is the exact
        // misfire OQ3 keeps a model away from. Re-picking is one click; a
        // surprise task is not undoable.
        setPick(emptyPick)
        // REST is the truth: the POST confirms, and what the surface renders is
        // the re-read. A turn may also have named the session (the titling duty)
        // and may have produced a file, so both siblings re-read too.
        reload()
        onSessionChanged()
        onFilesChanged()
      }
    },
    [id, reload, onSessionChanged, onFilesChanged],
  )

  const stop = useCallback(
    async (turn: string) => {
      setFailure(null)
      try {
        await api.stopChatTurn(turn)
      } catch (err) {
        setFailure(describeChatFailure(err))
      } finally {
        reload()
      }
    },
    [reload],
  )

  const runtime = useExternalStoreRuntime<ChatFeedItem>({
    messages: feed,
    convertMessage: convertChatItem,
    // In-flight is the POST being outstanding OR a `running` row the server is
    // still holding — which is what a view re-entered mid-turn re-attaches to.
    isRunning: sending || running !== null,
    // The verb's argument gates SENDING without disabling the input: the person
    // can keep typing while the act is unavailable, and the reason is on screen.
    isSendDisabled: !argumentReady(pick),
    onNew: async (message: AppendMessage) => {
      const text = textOf(message.content)
      await submit(turnRequestFor(pickRef.current, text))
    },
    // The widget's own cancel affordance, wired to the SAME abandon verb the
    // in-feed stop control calls. Two affordances, one act.
    onCancel: async () => {
      if (running !== null) await stop(running.turn_id)
    },
  })

  return (
    <div className={cn(chatPanel, 'chat-thread')}>
      <ConversationHead view={view} />
      <Freshness stale={detail.stale} error={detail.error} hasData={view !== null} />
      {failure !== null && <FailureLine failure={failure} />}

      <AssistantRuntimeProvider runtime={runtime}>
        <ThreadPrimitive.Root className="chat-thread-root">
          <ThreadPrimitive.Viewport className="chat-viewport flex max-w-full flex-col gap-3">
            <ThreadPrimitive.Empty>
              <Empty what="Nothing said in this conversation yet." />
            </ThreadPrimitive.Empty>
            <ThreadPrimitive.Messages>
              {({ message }) => {
                const item = feed.find((f) => f.key === message.id)
                if (item === undefined || item.role === 'user') return <SaidBubble />
                return (
                  <TurnBubble
                    turn={item.turn}
                    question={item.question}
                    files={files}
                    queue={queue.data?.items ?? []}
                    onStop={() => void stop(item.turn.turn_id)}
                    onEscalate={(question) => void submit({ kind: 'open_sql', text: question })}
                    onChoose={(query, question) => void submit({ kind: 'query', text: question, query })}
                    onAnswered={answered}
                  />
                )
              }}
            </ThreadPrimitive.Messages>
            {/* This tab's own truth about its own outstanding request, and only
                while the platform has not yet served the turn it opened. It is
                not a copy of the message — the message is the server's — and it
                is the one thing on this surface that a navigation sheds, which
                the survives-navigation inventory says out loud. */}
            {sending && running === null && (
              <p className="muted" data-sending="true">
                Sending — the platform is recording your message. There is nothing to stop yet: a turn can only be
                stopped once the platform has opened it and told us its id.
              </p>
            )}
          </ThreadPrimitive.Viewport>

          <Composer pick={pick} setPick={setPick} registries={registries} files={files} stoppable={running} />
        </ThreadPrimitive.Root>
      </AssistantRuntimeProvider>
    </div>
  )
}

/** The conversation's own header: its title, and — when the label came from the
 *  mechanical fallback rather than the titling seat — WHY, so a person is not
 *  left wondering why their conversation is named after its first line. */
function ConversationHead({ view }: { view: ChatSessionView | null }) {
  if (view === null) return null
  const s = view.session
  return (
    <div className="chat-thread-head">
      <h3>{s.title === '' ? 'Untitled conversation' : s.title}</h3>
      {s.title_source === 'mechanical' && !view.title_seat_wired && (
        <p className="muted">
          Named from your first message: the local titling seat is not wired in this process, so the platform used your
          own words rather than inventing a label.
        </p>
      )}
      {s.title_source === 'human' && <p className="muted">You named this conversation.</p>}
    </div>
  )
}

/** The person's own words, rendered through the widget's own text part — escaped,
 *  like every other piece of content on every surface (S15.12). */
function SaidBubble() {
  return (
    <MessagePrimitive.Root
      className="chat-said max-w-full rounded-(--radius-sm) border border-border bg-(--panel) p-2 whitespace-pre-wrap wrap-anywhere"
      data-role="user"
    >
      <MessagePrimitive.Parts />
    </MessagePrimitive.Root>
  )
}

// ── one turn's outcome ────────────────────────────────────────────────────

function TurnBubble({
  turn,
  question,
  files,
  queue,
  onStop,
  onEscalate,
  onChoose,
  onAnswered,
}: {
  turn: ChatTurn
  question: string
  files: ChatFile[]
  queue: ApprovalItem[]
  onStop: () => void
  onEscalate: (question: string) => void
  onChoose: (query: string, question: string) => void
  onAnswered: () => void
}) {
  const fact = turnFact(turn)
  return (
    <MessagePrimitive.Root
      className="chat-turn"
      data-role="assistant"
      data-turn={turn.turn_id}
      data-turn-kind={turn.kind}
      data-turn-state={turn.state}
      data-turn-fact={fact.kind}
    >
      {fact.kind === 'running' && (
        <div className="chat-running flex flex-wrap items-center gap-2">
          <p className="m-0">{fact.headline}</p>
          <Button variant="danger" size="sm" onClick={onStop}>
            Stop this turn
          </Button>
        </div>
      )}

      {fact.kind === 'abandoned' && <p className="muted">{fact.headline}</p>}
      {fact.kind === 'empty' && <p className="absent">{fact.headline}</p>}
      {fact.kind === 'error' && <TurnErrorLine headline={fact.headline} outcome={turn.outcome as TurnError} />}

      {/* An abandoned turn whose result landed anyway still shows it, with its
          honest state above: the record says the person stopped, and the late
          arrival is not hidden. */}
      {(fact.kind === 'answer' || (fact.kind === 'abandoned' && turn.outcome_kind === 'answer')) && (
        <AnswerOutcome
          answer={turn.outcome as Answer}
          question={question}
          onEscalate={onEscalate}
          onChoose={onChoose}
        />
      )}
      {(fact.kind === 'task' || (fact.kind === 'abandoned' && turn.outcome_kind === 'task')) && (
        <BornTaskView task={turn.outcome as ChatBornTask} queue={queue} onAnswered={onAnswered} />
      )}

      <ProducedChips produced={turn.produced ?? []} files={files} />
    </MessagePrimitive.Root>
  )
}

/** A failed turn prints the platform's own {error, detail} envelope. The client
 *  adds no diagnosis of its own: the code is the classification the SERVER made. */
function TurnErrorLine({ headline, outcome }: { headline: string; outcome: TurnError }) {
  return (
    <div className="chat-turn-error" data-error-code={outcome?.error ?? ''}>
      <p className="error">{headline}</p>
      {outcome?.error !== undefined && <p className="muted">{outcome.error}</p>}
      {outcome?.detail !== undefined && outcome.detail !== '' && <p className="muted">{outcome.detail}</p>}
    </div>
  )
}

/**
 * An answered turn.
 *
 * The answer itself goes through `AnswerView` untouched. The one thing added
 * beside it is the ESCALATION ACT: Layer 2 exists, and reaching it is a thing a
 * person does deliberately, on a question they already asked and were not
 * satisfied by. It is never a fallback, it is never automatic, and it does not
 * appear on an answer that is already Layer 2.
 */
function AnswerOutcome({
  answer,
  question,
  onEscalate,
  onChoose,
}: {
  answer: Answer
  question: string
  onEscalate: (question: string) => void
  onChoose: (query: string, question: string) => void
}) {
  const escalatable = answer.layer < 2 && question !== ''
  return (
    <div className="chat-answer max-w-full overflow-x-auto">
      <AnswerView answer={answer} onChoose={(query) => onChoose(query, question)} />
      {escalatable && (
        <Button variant="secondary" size="sm" className="chat-escalate mt-2" onClick={() => onEscalate(question)}>
          Escalate this question to open SQL (Layer 2, lower confidence, audited)
        </Button>
      )}
    </div>
  )
}

/** The produced-files chips (OQ7 render half).
 *
 *  The list is the SERVER's attribution — a manifest window closed at settle plus
 *  the late arrivals whose origin names this turn — and this renders it. At v0
 *  only uploads write the exchange folder, so most turns produce nothing and say
 *  so; a chip is never invented to make a turn look productive. */
function ProducedChips({ produced, files }: { produced: string[]; files: ChatFile[] }) {
  if (produced.length === 0) {
    return (
      <p className="chat-chips-empty" data-chips="none">
        <Absent reason="No files came out of this turn" />
      </p>
    )
  }
  return (
    <ul className="chat-chips m-0 flex list-none flex-col gap-2 p-0" data-chips="some">
      {produced.map((id) => {
        const file = files.find((f) => f.file_id === id)
        return (
          <li
            key={id}
            className="chip"
            data-chip={id}
          >
            {file ? (
              <>
                <span className="chip-name wrap-anywhere">{file.name}</span> <span className="muted">{String(file.size_bytes)} bytes</span>
              </>
            ) : (
              <>
                <span className="chip-name wrap-anywhere">{id}</span> <Absent reason="no longer in the exchange folder" />
              </>
            )}
          </li>
        )
      })}
    </ul>
  )
}

// ── the S06 handoff, continued in place (OQ8(i)) ──────────────────────────

/**
 * The open intake card for a born task, taken from the person's OWN DECISION
 * QUEUE rather than from the turn that started the task.
 *
 * This is the correction of a real defect, and the reason is worth stating: a
 * settled turn's outcome is written exactly once and is never re-enriched on
 * read, while the pipeline issues the interview card only AFTER the handoff verb
 * has answered — `Start` births the task with no ask at all, and the ask and the
 * intake run's park land together in a later transaction of their own. So a turn
 * outcome that carries a card is a fixture's shape, not production's: reading the
 * card off the outcome meant OQ8-(i)'s in-place card could never appear on a real
 * platform, and no re-read could ever add it.
 *
 * The queue is also the only surface serving the PIN an answer must quote, so
 * card and pin now come from one place, and the card leaves the feed the moment
 * it is resolved — including when it is answered in the inbox instead. Matching
 * is on the card's own `task_id`; nothing here composes an id.
 */
function openIntakeCard(queue: ApprovalItem[], taskID: string): { item: ApprovalItem; card: IntakeCard } | null {
  for (const item of queue) {
    const card = item.card as IntakeCard | undefined
    if (card !== undefined && card !== null && card.task_id === taskID) return { item, card }
  }
  return null
}

/** The intake card shapes this surface can be handed. `questions` is the
 *  interview shape it renders; a card without them is a real kind it does not
 *  render (a delta decision), which is said plainly rather than shown empty. */
type IntakeCard = NonNullable<ChatBornTask['open_card']> & { task_id?: string }

/**
 * The born task, with its OPEN intake card rendered inline.
 *
 * Answering it here goes through the LANDED approvals answer verb with the card's
 * OWN payload hash, re-read at the moment of answering: there is no second answer
 * path, and this surface never composes a card id or invents a pin. When nothing
 * of the person's is open on the task, that is said plainly and the deep links are
 * there.
 */
function BornTaskView({
  task,
  queue,
  onAnswered,
}: {
  task: ChatBornTask
  queue: ApprovalItem[]
  onAnswered: () => void
}) {
  const open = openIntakeCard(queue, task.task_id)
  return (
    <div className="chat-born max-w-full overflow-x-auto" data-task={task.task_id}>
      <p>
        Task started: <Link to={hrefFor('task', { id: task.task_id })}>{task.title}</Link>
      </p>
      <dl className="chat-born-facts my-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 [&_dd]:m-0 [&_dd]:wrap-anywhere [&_dt]:text-muted-foreground">
        <dt>task</dt>
        <dd>{task.task_id}</dd>
        <dt>stage</dt>
        <dd>{task.kanban_status}</dd>
        <dt>phase</dt>
        <dd>{task.phase}</dd>
        <dt>tier</dt>
        <dd>{task.tier}</dd>
        {task.clearance !== undefined && (
          <>
            <dt>clearance</dt>
            <dd>{String(task.clearance)}</dd>
          </>
        )}
      </dl>
      {open !== null && (open.card.questions ?? []).length > 0 ? (
        <IntakeCardInPlace askID={nativeID(open.item.id)} card={open.card} onAnswered={onAnswered} />
      ) : open !== null ? (
        <p className="muted">
          This task is waiting on you, but not with a question this surface answers — it is in your{' '}
          <Link to={hrefFor('inbox-item', { id: open.item.id })}>decision queue</Link>.
        </p>
      ) : (
        <p className="muted">
          Nothing is waiting on you for this task right now — its progress is on the task page.
        </p>
      )}
    </div>
  )
}

function IntakeCardInPlace({
  askID,
  card,
  onAnswered,
}: {
  askID: string
  card: NonNullable<ChatBornTask['open_card']>
  onAnswered: () => void
}) {
  const questions = card.questions ?? []
  const [answers, setAnswers] = useState<Record<string, string>>({})
  const [failure, setFailure] = useState<ChatFailure | null>(null)
  const [notice, setNotice] = useState('')
  const [found, setFound] = useState<ApprovalItem | null>(null)
  const [busy, setBusy] = useState(false)

  const answered = questions.filter((q) => (answers[q.id] ?? '') !== '')

  const send = async () => {
    setBusy(true)
    setFailure(null)
    setNotice('')
    try {
      // The PIN comes from the card the PLATFORM is showing in its own queue.
      // Matching on the ask's native id rather than composing "<kind>:<id>" keeps
      // the id used for the verb and the deep link the server's own string.
      const queue = await api.approvals()
      const item = queue.items.find((i) => nativeID(i.id) === askID)
      if (item === undefined) {
        setNotice('This card is no longer in your decision queue — it may already have been answered.')
        return
      }
      setFound(item)
      // A CARD THAT DEMANDS A PIN IS ANSWERED WHERE THE PIN LIVES (§43's
      // card-is-the-authority rule). This surface has no PIN field and must never
      // grow one: a High-tier step-up exists so approval identity is never
      // inherited from an idle session (G3 Def.1), and the inbox already has the
      // prompt. At v0 an intake interview card declares standard/medium and never
      // asks for one, so this branch is the assumption made explicit rather than a
      // hole being closed — the day a card arrives with it, the person is sent to
      // the surface that can do it instead of being handed a refusal.
      if (item.step_up_required) {
        setNotice(
          'This card needs your PIN re-entered, which happens in the decision queue — open it there and the platform will ask.',
        )
        return
      }
      if (item.payload_hash === undefined || item.payload_hash === '' || !item.answerable) {
        setNotice(
          item.not_answerable_reason ??
            'The platform is not offering this card for an answer from here — open it in the inbox.',
        )
        return
      }
      await api.answerApproval(item.id, {
        payload_hash: item.payload_hash,
        answer: { answers: answered.map((q) => ({ id: q.id, value: answers[q.id] })) },
      })
      onAnswered()
    } catch (err) {
      setFailure(describeChatFailure(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="chat-intake" data-intake-ask={askID} data-intake-kind={card.kind}>
      <p>
        The platform needs an answer to get started — {card.kind}
        {card.tier !== undefined && <span className="muted"> · {card.tier}</span>}
      </p>
      {questions.length === 0 && <Empty what="This card carries no questions to answer here." />}
      <ul className="chat-intake-questions m-0 flex list-none flex-col gap-2 p-0">
        {questions.map((q) => (
          <li key={q.id} data-question={q.id}>
            <label>
              {/* The utility-phrased wording when the seat answered, the
                  canonical text otherwise (RW-12 R6) — same rule as the
                  journey and the inbox. */}
              {q.phrased !== undefined && q.phrased !== '' ? q.phrased : q.text}
              {q.options && q.options.length > 0 ? (
                <select
                  value={answers[q.id] ?? ''}
                  onChange={(e) => setAnswers({ ...answers, [q.id]: e.target.value })}
                >
                  <option value="">Choose…</option>
                  {q.options.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  type="text"
                  value={answers[q.id] ?? ''}
                  onChange={(e) => setAnswers({ ...answers, [q.id]: e.target.value })}
                />
              )}
            </label>
          </li>
        ))}
      </ul>
      <div className="chat-intake-acts">
        <Button variant="primary" size="sm" disabled={busy || answered.length === 0} onClick={() => void send()}>
          Answer here
        </Button>
        <Link to={hrefFor('inbox')}>Open the decision queue</Link>
        {found !== null && <Link to={hrefFor('inbox-item', { id: found.id })}>Open this card</Link>}
      </div>
      {notice !== '' && <p className="absent">{notice}</p>}
      {failure !== null && <FailureLine failure={failure} />}
    </div>
  )
}

/** The queue keys a card as `<kind>:<native id>`, which api.ts records as the id
 *  the answer verbs and /inbox/:id take. Reading the native half back out is how
 *  this surface finds a card WITHOUT composing one. */
function nativeID(id: string): string {
  const at = id.indexOf(':')
  return at === -1 ? id : id.slice(at + 1)
}

// ── the composer: the verb, its argument, and the text ────────────────────

/**
 * The composer is one act with a stated verb.
 *
 * The text is always recorded as the message, whatever the verb — a turn a person
 * could not read back afterwards would make the transcript a lie by omission. The
 * choice surfaces are the SERVED registries: a hand-written list of view or query
 * names here would be a second copy of registries that already exist.
 */
function Composer({
  pick,
  setPick,
  registries,
  files,
  stoppable,
}: {
  pick: ComposerPick
  setPick: (p: ComposerPick) => void
  registries: Registries
  files: ChatFile[]
  /** The turn the widget's own cancel affordance would end, or null when there is
   *  nothing stoppable — which is every moment before the platform has opened the
   *  turn and named it. */
  stoppable: ChatTurn | null
}) {
  const selected = (registries.catalog?.queries ?? []).find((q) => q.name === pick.query)
  return (
    <ComposerPrimitive.Root className="chat-composer flex max-w-full flex-col gap-2">
      <Hint id="composer-verb">
        You choose what a message does — ask, run a named view, run a catalog question, or start a task. Nothing here
        guesses that for you.
      </Hint>
      <div
        // The wide arm must be a UTILITY now, not an owned rule: after the
        // cascade fix an owned `@media` rule sits in `@layer base` and can no
        // longer override a utility on the same element, so the landed
        // `.chat-verbs { flex-direction: row }` breakpoint would be dead here.
        // `md:` IS the landed 48rem breakpoint (§47).
        className="chat-verbs flex flex-col gap-1 md:flex-row md:flex-wrap md:items-center [&_label]:flex [&_label]:items-center [&_label]:gap-2"
        role="group" aria-label="What this turn should do">
        {composerVerbs.map((v) => (
          <label key={v.kind} data-verb={v.kind}>
            <input
              type="radio"
              name="chat-verb"
              value={v.kind}
              checked={pick.verb === v.kind}
              onChange={() => setPick({ ...emptyPick, verb: v.kind as ComposerVerb })}
            />
            {v.label}
          </label>
        ))}
      </div>

      {registries.error !== '' && <p className="error">{registries.error}</p>}

      {pick.verb === 'view' && (
        <label className="chat-arg flex max-w-full flex-col gap-2">
          A named view
          <select value={pick.view} onChange={(e) => setPick({ ...pick, view: e.target.value })}>
            <option value="">Choose a view…</option>
            {(registries.views?.views ?? []).map((v) => (
              <option key={v.Name} value={v.Name}>
                {v.Name} — {v.Question}
              </option>
            ))}
          </select>
        </label>
      )}

      {pick.verb === 'query' && (
        <div className="chat-arg flex max-w-full flex-col gap-2">
          <label>
            A catalog question
            <select value={pick.query} onChange={(e) => setPick({ ...pick, query: e.target.value, slots: {} })}>
              <option value="">Choose a question…</option>
              {(registries.catalog?.queries ?? []).map((q) => (
                <option key={q.name} value={q.name}>
                  {q.name} — {q.description}
                </option>
              ))}
            </select>
          </label>
          {(selected?.slots ?? []).map((s) => (
            <label key={s.name}>
              {s.name}
              <input
                type="text"
                value={pick.slots[s.name] ?? ''}
                onChange={(e) => setPick({ ...pick, slots: { ...pick.slots, [s.name]: e.target.value } })}
              />
            </label>
          ))}
        </div>
      )}

      {pick.verb === 'task' && (
        <div className="chat-arg flex max-w-full flex-col gap-2">
          <label>
            Title (optional — the platform uses your first line if you leave it)
            <input type="text" value={pick.title} onChange={(e) => setPick({ ...pick, title: e.target.value })} />
          </label>
          <fieldset className="chat-inputs [&_label]:flex [&_label]:items-center [&_label]:gap-2">
            <legend>Hand files to this task</legend>
            {files.length === 0 ? (
              <Empty what="Nothing in your exchange folder to hand over." />
            ) : (
              files.map((f) => (
                <label key={f.file_id} data-input={f.file_id}>
                  <input
                    type="checkbox"
                    checked={pick.inputs.includes(f.file_id)}
                    onChange={(e) =>
                      setPick({
                        ...pick,
                        inputs: e.target.checked
                          ? [...pick.inputs, f.file_id]
                          : pick.inputs.filter((id) => id !== f.file_id),
                      })
                    }
                  />
                  {f.name}
                </label>
              ))
            )}
          </fieldset>
        </div>
      )}

      {!argumentReady(pick) && (
        <p className="absent">Pick one from the list above before sending — the platform could only refuse it.</p>
      )}

      <div className="chat-composer-row">
        <ComposerPrimitive.Input
          className="chat-input"
          placeholder="Ask what the platform is doing…"
          aria-label="Message"
        />
        <ComposerPrimitive.Send className="chat-send">Send</ComposerPrimitive.Send>
        {/* The widget's cancel affordance renders only when there is a turn to
            cancel. `ThreadPrimitive.If running` was the wrong gate: the thread is
            also "running" while the POST is merely outstanding, and in THAT window
            no turn id exists yet — so the button was a door onto nothing. OQ6
            rejected client-only forget, so there is no honest local abort to put
            behind it; a control that cannot act must not be drawn (§43). The
            sending notice says so in words instead. */}
        {stoppable !== null && <ComposerPrimitive.Cancel className="chat-cancel">Stop</ComposerPrimitive.Cancel>}
      </div>
    </ComposerPrimitive.Root>
  )
}

type Registries = { views: HistoryRegistry | null; catalog: HistoryRegistry | null; error: string }

/** The registries are compiled-in data, not a projection: one read each, and no
 *  subscription — a registry does not change while a tab is open. */
function useRegistries(): Registries {
  const [state, setState] = useState<Registries>({ views: null, catalog: null, error: '' })
  useEffect(() => {
    let live = true
    Promise.all([api.historyViews(), api.historyCatalog()]).then(
      ([views, catalog]) => {
        if (live) setState({ views, catalog, error: '' })
      },
      (err: unknown) => {
        if (live) setState({ views: null, catalog: null, error: describeError(err) })
      },
    )
    return () => {
      live = false
    }
  }, [])
  return state
}

// ── the file exchange (R13) ──────────────────────────────────────────────

/**
 * The exchange sidebar: drag-drop and click-browse into ONE upload verb, the
 * uploaded list, and delete.
 *
 * The exchange is control-plane territory. A file here is not mounted into any
 * run — it reaches a task only as an input descriptor through the intake handoff,
 * which is why the "hand files to this task" affordance lives on the composer
 * beside the task verb and not on this list.
 */
function Exchange({
  files,
  stale,
  error,
  onUpload,
  onDelete,
}: {
  files: ChatFile[]
  stale: boolean
  error: string
  onUpload: (file: File) => void
  onDelete: (id: string) => void
}) {
  const [over, setOver] = useState(false)

  const take = (list: FileList | null | undefined) => {
    for (const file of Array.from(list ?? [])) onUpload(file)
  }

  return (
    <aside
      className={cn(chatPanel, 'chat-exchange data-[drop-target=over]:border-[var(--accent)]')}
      data-drop-target={over ? 'over' : 'idle'}
      onDragOver={(e: DragEvent<HTMLElement>) => {
        e.preventDefault()
        setOver(true)
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e: DragEvent<HTMLElement>) => {
        e.preventDefault()
        setOver(false)
        take(e.dataTransfer?.files)
      }}
    >
      <h3>Exchange folder</h3>
      <p className="muted">Drop files here, or browse. They stay yours until you delete them.</p>
      <label className="chat-browse flex flex-col gap-1">
        Browse…
        <input
          type="file"
          multiple
          aria-label="Upload to the exchange folder"
          onChange={(e) => {
            take(e.target.files)
            e.target.value = ''
          }}
        />
      </label>
      <Freshness stale={stale} error={error} hasData={files.length > 0} />
      {files.length === 0 && !stale ? (
        <Empty what="Nothing in your exchange folder." />
      ) : (
        <ul className="chat-files m-0 flex list-none flex-col gap-2 p-0">
          {files.map((f) => (
            <li
              key={f.file_id}
              data-file={f.file_id}
              // The SAME set the conversation rows carry. Both <li> families were
              // ONE shed rule (`.chat-sessions li, .chat-files li`), so both must
              // carry its presentation or the row collapses to inline flow —
              // name, size, sha, stamp and Delete with no separating space.
              // jsdom has no layout engine and cannot see that, so the set is
              // pinned in chat.test.tsx instead.
              className="flex flex-col gap-1 wrap-anywhere"
            >
              <span className="chat-file-name wrap-anywhere">{f.name}</span>
              <span className="muted">{String(f.size_bytes)} bytes</span>
              <span className="muted chat-file-sha">{f.sha256}</span>
              <Timestamp ts={f.uploaded_ts} variant="live" />
              <Button variant="ghost" size="sm" onClick={() => onDelete(f.file_id)}>
                Delete
              </Button>
            </li>
          ))}
        </ul>
      )}
    </aside>
  )
}
