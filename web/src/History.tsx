import { useEffect, useState } from 'react'

import { api, type Answer, type HistoryRegistry } from './api'
import { describeError } from './live'
import { Empty } from './parts'

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
  const [asking, setAsking] = useState(false)

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

  const ask = (run: () => Promise<Answer>) => {
    setAsking(true)
    run().then(
      (a) => {
        setAnswer(a)
        setAsking(false)
        setFailure('')
      },
      (err: unknown) => {
        setAsking(false)
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
              if (name !== '') ask(() => api.historyView(name))
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
          <button type="button" onClick={() => ask(() => api.historyQuery(selected.name, slots))}>
            Ask
          </button>
        </div>
      )}

      {asking && <p className="muted">Asking…</p>}
      {answer && <AnswerView answer={answer} onChoose={(name) => ask(() => api.historyQuery(name, slots))} />}
    </section>
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
