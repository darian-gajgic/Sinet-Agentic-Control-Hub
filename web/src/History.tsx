import { useEffect, useState } from 'react'

import { api, type Answer, type Disambiguation, type HistoryRegistry } from './api'
import { describeError } from './live'
import { Button, EmptyState } from './ui'

/** The question both free-text controls hold back, mirroring the ONE bound the
 *  transport applies at its own boundary (`historyQuestion`,
 *  internal/api/historyapi.go:197–204: a trimmed-empty `?q=` is refused 400).
 *  Pre-validation exists to stop a request that is already refused, never to
 *  answer for the server — everything that fires renders the server's own words. */
const emptyQuestionReason =
  'A question is required — the query surface refuses an empty one, so there is nothing to send yet.'

/**
 * Filterable history (Spec S15.5; S2.10 through the S14 query layers).
 *
 * The choice surface is the SERVED registry — the Layer-0 views and the
 * Layer-1 catalog as the packages themselves define them. A hand-written list
 * of query names here would be a second copy of a registry that already exists,
 * drifting the moment either side gained a row.
 *
 * A REFUSAL AND A DISAMBIGUATION CARD ARE ANSWERS. The transport serves both at
 * 200 with their audit attached, on purpose, and this panel renders them as the
 * honest non-answers they are rather than swallowing them into an error banner.
 * Layer 2's lower-confidence flag is rendered wherever its answers render
 * (G3 D3.5) — it is the whole reason that layer is allowed to exist.
 *
 * NOT LIVE, AND SAYS SO. Everything else on mission control is a projection
 * that the feed keeps current. This is a QUERY INSTRUMENT: an answer is a
 * point-in-time reply to a question somebody asked, and re-running it on every
 * frame would be noise on Layer 0 and a repeated model call on Layer 2. Asking
 * again is an act, so the panel offers one.
 */
export function HistoryPanel() {
  const [registry, setRegistry] = useState<HistoryRegistry | null>(null)
  const [catalog, setCatalog] = useState<HistoryRegistry | null>(null)
  const [failure, setFailure] = useState('')

  const [view, setView] = useState('')
  const [queryName, setQueryName] = useState('')
  const [slots, setSlots] = useState<Record<string, string>>({})
  const [answer, setAnswer] = useState<Answer | null>(null)
  /** Which control has a request in flight, or '' for none. It is the SOURCE
   *  rather than a boolean because four gestures share one answer slot and each
   *  one's own button has to be able to say "this is mine, and it is in flight"
   *  — which is not the same statement as "this form is not ready to send"
   *  (§49 N7: busy and held are two facts, and one prop for both answered the
   *  wrong question). */
  const [pending, setPending] = useState<'' | 'view' | 'query' | 'ask' | 'search'>('')
  const asking = pending !== ''

  const [question, setQuestion] = useState('')
  const [term, setTerm] = useState('')

  useEffect(() => {
    // The registries are compiled-in data, not a projection: one read each.
    Promise.all([api.historyViews(), api.historyCatalog()]).then(
      ([v, c]) => {
        setRegistry(v)
        setCatalog(c)
        setFailure('')
      },
      (err: unknown) => setFailure(describeError(err)),
    )
  }, [])

  const ask = (source: 'view' | 'query' | 'ask' | 'search', run: () => Promise<Answer>) => {
    setPending(source)
    run().then(
      (a) => {
        setAnswer(a)
        setPending('')
        setFailure('')
      },
      (err: unknown) => {
        setPending('')
        setFailure(describeError(err))
      },
    )
  }

  const selected = (catalog?.queries ?? []).find((q) => q.name === queryName)

  return (
    <section className="block history" data-live="query-instrument">
      <h2>History</h2>
      <p className="muted max-w-prose text-xs">
        Answers here are point-in-time replies to the question you ask, not a live projection — ask again for a fresh
        one.
      </p>
      {failure !== '' && <p className="error">{failure}</p>}

      <div className="history-choices flex flex-col gap-2 md:flex-row md:flex-wrap md:items-end">
        <label>
          Layer 0 — a named view
          <select
            value={view}
            onChange={(e) => {
              const name = e.target.value
              setView(name)
              setQueryName('')
              if (name !== '') ask('view', () => api.historyView(name))
            }}
          >
            <option value="">Choose a view…</option>
            {(registry?.views ?? []).map((v) => (
              <option key={v.Name} value={v.Name}>
                {v.Name} — {v.Question}
              </option>
            ))}
          </select>
        </label>

        <label>
          Layer 1 — a catalog question
          <select
            value={queryName}
            onChange={(e) => {
              setQueryName(e.target.value)
              setView('')
              setSlots({})
              setAnswer(null)
            }}
          >
            <option value="">Choose a question…</option>
            {(catalog?.queries ?? []).map((q) => (
              <option key={q.name} value={q.name}>
                {q.name} — {q.description}
              </option>
            ))}
          </select>
        </label>
      </div>

      {selected && (
        <div className="history-slots flex flex-col gap-2 md:flex-row md:flex-wrap md:items-end">
          {(selected.slots ?? []).map((s) => (
            <label key={s.name}>
              {s.name}
              <input
                type="text"
                value={slots[s.name] ?? ''}
                onChange={(e) => setSlots({ ...slots, [s.name]: e.target.value })}
              />
            </label>
          ))}
          <Button size="sm" onClick={() => ask('query', () => api.historyQuery(selected.name, slots))}>
            Ask
          </Button>
        </div>
      )}

      <div className="flex flex-col gap-4 md:flex-row md:items-start">
        <QuestionForm
          name="ask"
          label="Ask in your own words"
          note="The platform matches your question to one of the questions above and fills it in. When it cannot, it answers with the ones it could have run — that is an answer, not a failure, and it says why."
          act="Ask"
          value={question}
          onChange={setQuestion}
          pending={pending}
          onFire={(q) => ask('ask', () => api.historyAsk(q))}
        />
        <QuestionForm
          name="search"
          label="Search the recorded history"
          note="Your words are sent exactly as typed. What comes back are bounded excerpts with the reference each was found under — the record itself is read from that reference."
          act="Search"
          value={term}
          onChange={setTerm}
          pending={pending}
          onFire={(q) => ask('search', () => api.historySearch(q))}
        />
      </div>

      {asking && <p className="muted">Asking…</p>}
      {answer && (
        <AnswerView
          answer={answer}
          onChoose={(name) => {
            // A CARD CHOICE SELECTS THE QUESTION; IT DOES NOT FIRE IT.
            //
            // The choices are catalog NAMES, and a catalog question has typed
            // slots of its own. The landed binding fired the chosen name with
            // whatever slots the picker happened to be holding — for a card that
            // came from the free-text ask, those are another question's slots or
            // none at all, so the reader would have got a different question's
            // answer under the one they clicked. So the choice lands where a
            // Layer-1 question is asked from, with its slots empty and rendered,
            // and the person fires it deliberately.
            //
            // The card is deliberately NOT cleared: it is the context the choice
            // was made in, and it stays on screen until an answer replaces it.
            setView('')
            setQueryName(name)
            setSlots({})
          }}
        />
      )}
    </section>
  )
}

/**
 * The two free-text verbs' shared form (P3-UI-4).
 *
 * They are ONE component because they differ only in their words and their
 * verb: both take a question, both refuse an empty one at the same bound the
 * transport applies, both render their answer through the panel's single
 * `AnswerView`. Two copies would be two places for the held/busy distinction to
 * drift apart.
 *
 * BUSY AND HELD ARE TWO FACTS (§49 N7). `data-busy` is true only while THIS
 * form's own request is in flight; a form that has simply not been filled in
 * says so at the field and fires nothing. The button is disabled by either, but
 * the two reasons never collapse into one another on screen.
 *
 * The draft survives everything: an answer, a refusal and the other form firing
 * all leave what was typed exactly where it was, because losing it would make
 * fixing one word mean retyping the sentence.
 */
function QuestionForm({
  name,
  label,
  note,
  act,
  value,
  onChange,
  pending,
  onFire,
}: {
  name: 'ask' | 'search'
  label: string
  note: string
  act: string
  value: string
  onChange: (v: string) => void
  pending: string
  onFire: (q: string) => void
}) {
  const held = value.trim() === '' ? emptyQuestionReason : ''
  const busy = pending === name

  return (
    <form
      className="flex min-w-0 flex-1 flex-col gap-2"
      data-form={name}
      onSubmit={(e) => {
        e.preventDefault()
        if (held !== '' || busy) return
        onFire(value)
      }}
    >
      <label className="flex flex-col gap-1">
        {label}
        <input
          type="text"
          data-field={name}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="w-full min-w-0 rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
        />
      </label>
      <p className="muted">{note}</p>
      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" size="sm" data-act={name} data-busy={String(busy)} disabled={busy || held !== ''}>
          {act}
        </Button>
        {held !== '' && (
          <span className="muted" data-held={name}>
            {held}
          </span>
        )}
      </div>
    </form>
  )
}

/**
 * The choices of a disambiguation card, cut into the consecutive runs that
 * share a served `category`.
 *
 * Order in, order out — deliberately the same algorithm class as the shell's
 * `navSections` (App.tsx), and for the same reason: GROUPING IS PRESENTATION.
 * Every served choice is rendered exactly once, in served order; a choice whose
 * category is empty gets its own unlabelled run rather than being folded under
 * the heading of the run beside it. The category is the SERVED string
 * (api.ts:202–206) and is never rewritten here.
 */
type Choice = Disambiguation['choices'][number]

function choiceRuns(choices: Disambiguation['choices']): { category: string; items: Choice[] }[] {
  const runs: { category: string; items: Choice[] }[] = []
  for (const c of choices ?? []) {
    const last = runs.at(-1)
    if (last && last.category === c.category) last.items.push(c)
    else runs.push({ category: c.category, items: [c] })
  }
  return runs
}

/**
 * The S14.10 / S15.7 disambiguation card, in operator words (B6 gate record §9
 * finding A-2, assistant half; design proposal §3 as narrowed by its own §7
 * amendment A1.1, 2026-08-05).
 *
 * WHAT THIS CLIENT MAY NOT SAY, and why. The wire carries NO machine cause code
 * — a `Disambiguation` is `{question, reason, choices[]}` (api.ts:202–206) —
 * and the store serves this card for at least seven distinct reason classes
 * (internal/history/layer1.go: :155 no local tier wired, :178/:190 parse,
 * :202/:210 below threshold, :232 unknown intent, :256/:264 slot-fill).
 * Classifying served prose is banned (§38; chatFacts.ts:143–144 records the
 * rule). So no sentence written here may name WHICH cause produced this card.
 * The proposal's illustrative "no local model is wired…" cannot ship as
 * unconditional copy for exactly that reason; the platform's own served
 * `reason` is the SOLE statement of cause and renders verbatim below.
 *
 * What the client words do say is true of EVERY card in every one of those
 * classes: this is a list rather than an answer, nothing was run, and picking
 * one selects that question for you to ask. No free-generation is offered,
 * because by S15.7 / S14.10 design none exists.
 *
 * A CHOICE SELECTS AND NEVER FIRES (§50's landed contract): the handler hands
 * the query up so the picker can be filled with its own empty slots and the
 * person asks deliberately. The card is not cleared — it is the context the
 * choice was made in.
 */
function DisambiguationCard({
  card,
  onChoose,
}: {
  card: Disambiguation
  onChoose?: (query: string) => void
}) {
  return (
    <div
      className="my-3 rounded-(--radius-sm) border border-border bg-card/40 p-3"
      data-card="disambiguation"
    >
      <p className="mt-0 mb-1 font-medium">{card.question}</p>

      {/* The client's own words. Every clause is true of every reason class,
          and none of them names a cause. */}
      <p className="m-0 text-sm text-muted-foreground" data-card-words>
        These are questions the platform can answer. It could not turn what you asked into one of them, so nothing was
        run. Pick one and it becomes your question — you still ask it yourself.
      </p>

      {/* The platform's own statement of cause, verbatim and alone. */}
      <p className="mt-2 mb-0 text-sm text-muted-foreground" data-card-reason>
        {card.reason}
      </p>

      <div className="mt-3 flex flex-col gap-3">
        {choiceRuns(card.choices).map((run, i) => (
          <div key={`${run.category}-${String(i)}`} data-choice-group={run.category}>
            {run.category !== '' && (
              <p className="m-0 mb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                {run.category}
              </p>
            )}
            <ul className="m-0 flex list-none flex-col gap-1 p-0">
              {run.items.map((c) => (
                <li key={c.query} className="flex flex-wrap items-baseline gap-2">
                  <Button
                    variant="secondary"
                    size="sm"
                    data-choice={c.query}
                    onClick={() => onChoose?.(c.query)}
                  >
                    {c.query}
                  </Button>
                  <span className="text-sm text-muted-foreground">{c.description}</span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </div>
  )
}

/** AnswerView renders one S14.10 answer with its layer, confidence, notes and
 *  audit VERBATIM — the fields are the contract, so they are rendered as
 *  fields rather than folded into prose. */
export function AnswerView({ answer, onChoose }: { answer: Answer; onChoose?: (query: string) => void }) {
  const lowerConfidence = answer.layer === 2
  return (
    <div
      className="answer my-2 rounded-(--radius-sm) border border-border p-2"
      data-layer={String(answer.layer)}
      data-confidence={answer.confidence}
    >
      <p className="answer-head flex flex-wrap items-center gap-2">
        <span className="answer-query font-mono text-sm">{answer.query}</span>{' '}
        <span className="answer-layer font-mono text-xs tabular-nums text-muted-foreground">
          layer {answer.layer}
        </span>{' '}
        <span className={lowerConfidence ? 'warn-flag' : 'muted'}>confidence: {answer.confidence}</span>
      </p>
      {answer.question && <p className="muted">{answer.question}</p>}
      {(answer.notes ?? []).map((n) => (
        <p className="muted" key={n}>
          {n}
        </p>
      ))}

      {answer.card && <DisambiguationCard card={answer.card} onChoose={onChoose} />}

      {answer.audit && (
        <div className="audit my-2 border-s-2 border-[var(--accent)] ps-2" data-audit="open-sql">
          <p>
            outcome: {answer.audit.outcome}
            {answer.audit.refusal ? ` — refused: ${answer.audit.refusal}` : ''}
          </p>
          {/* A generated statement is genuinely wider than a phone, so it
              scrolls INSIDE its own box and wraps rather than pushing the page
              sideways (S1.10). */}
          {answer.audit.sql_generated && (
            <pre className="overflow-x-auto break-words whitespace-pre-wrap">{answer.audit.sql_generated}</pre>
          )}
          <p className="muted">
            alias {answer.audit.alias}
            {answer.audit.model ? ` · model ${answer.audit.model}` : ''} · rows {answer.audit.row_count}
          </p>
        </div>
      )}

      {(answer.columns ?? []).length === 0 ? (
        <EmptyState
          what="This answer carries no rows."
          why="The layer answered — it simply matched nothing. A refusal and a disambiguation card are answers too, and each says so above."
        />
      ) : (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                {(answer.columns ?? []).map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {(answer.rows ?? []).map((row, i) => (
                <tr key={i}>
                  {(row ?? []).map((cell, j) => (
                    <td key={j}>{cell === null || cell === undefined ? '—' : String(cell)}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {answer.truncated && (
        <p className="muted text-xs">This answer was truncated at the query surface&apos;s own bound.</p>
      )}
    </div>
  )
}
