import { useEffect, useState } from 'react'

import { api, type Answer, type HistoryRegistry } from './api'
import { describeError } from './live'
import { Empty } from './parts'
import { Button } from './ui'

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
      <p className="muted">
        Answers here are point-in-time replies to the question you ask, not a live projection — ask again for a fresh
        one.
      </p>
      {failure !== '' && <p className="error">{failure}</p>}

      <div className="history-choices">
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
        <div className="history-slots">
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
          <button type="button" onClick={() => ask('query', () => api.historyQuery(selected.name, slots))}>
            Ask
          </button>
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

/** AnswerView renders one S14.10 answer with its layer, confidence, notes and
 *  audit VERBATIM — the fields are the contract, so they are rendered as
 *  fields rather than folded into prose. */
export function AnswerView({ answer, onChoose }: { answer: Answer; onChoose?: (query: string) => void }) {
  const lowerConfidence = answer.layer === 2
  return (
    <div className="answer" data-layer={String(answer.layer)} data-confidence={answer.confidence}>
      <p className="answer-head">
        <span className="answer-query">{answer.query}</span>{' '}
        <span className="answer-layer">layer {answer.layer}</span>{' '}
        <span className={lowerConfidence ? 'warn-flag' : 'muted'}>confidence: {answer.confidence}</span>
      </p>
      {answer.question && <p className="muted">{answer.question}</p>}
      {(answer.notes ?? []).map((n) => (
        <p className="muted" key={n}>
          {n}
        </p>
      ))}

      {answer.card && (
        <div className="disambiguation" data-card="disambiguation">
          <p>{answer.card.question}</p>
          <p className="muted">{answer.card.reason}</p>
          <ul>
            {answer.card.choices.map((c) => (
              <li key={c.query}>
                <button type="button" onClick={() => onChoose?.(c.query)}>
                  {c.query}
                </button>{' '}
                <span className="muted">
                  {c.category} — {c.description}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {answer.audit && (
        <div className="audit" data-audit="open-sql">
          <p>
            outcome: {answer.audit.outcome}
            {answer.audit.refusal ? ` — refused: ${answer.audit.refusal}` : ''}
          </p>
          {answer.audit.sql_generated && <pre>{answer.audit.sql_generated}</pre>}
          <p className="muted">
            alias {answer.audit.alias}
            {answer.audit.model ? ` · model ${answer.audit.model}` : ''} · rows {answer.audit.row_count}
          </p>
        </div>
      )}

      {answer.columns.length === 0 ? (
        <Empty what="This answer carries no rows." />
      ) : (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                {answer.columns.map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {answer.rows.map((row, i) => (
                <tr key={i}>
                  {row.map((cell, j) => (
                    <td key={j}>{cell === null || cell === undefined ? '—' : String(cell)}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {answer.truncated && <p className="muted">This answer was truncated at the query surface&apos;s own bound.</p>}
    </div>
  )
}
