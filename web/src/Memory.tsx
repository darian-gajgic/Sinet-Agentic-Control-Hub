import { useState, type ReactNode } from 'react'

import {
  api,
  type MemoryConflict,
  type MemoryEntry,
  type MemoryEntryDetail,
  type MemoryInfluenceRef,
  type MemorySelectors,
  type MemoryWriteBody,
} from './api'
import { ActConfirm, OutcomeLine, outcomeOf, useAct } from './controls'
import type { EventStream } from './events'
import { useLive } from './live'
import { Absent, Freshness } from './parts'
import { Link } from './router'
import { hrefFor } from './routes'
import { Button, Chip, EmptyState, Panel, Timestamp, type Tone } from './ui'

/**
 * The S09 memory family's workspace surface (Spec S09; S15.2 memory row; S09.4's
 * v0 activation row "Manual L2 — human-direct entries via workspace UI").
 *
 * WHAT THIS SURFACE IS ALLOWED TO KNOW. Nothing here decides who may see what.
 * The read is scoped server-side by a predicate with no role parameter at all
 * (`Store.ListVisible`), and the answer carries the rule that produced it as a
 * SENTENCE, which this surface prints verbatim rather than paraphrasing. The
 * operator role bit opens nothing of another person's user-scope store: memory
 * is personal CONTENT and D10 is authority over the HOUSE scope — the content
 * line, not the §30 observability line.
 *
 * EVERY WRITE IS THE GATE'S. Create, edit, retire and delete are four landed
 * gate verbs, and every refusal renders the GATE'S OWN sentence. This file
 * re-implements no wall and paraphrases no reason: where a control's authority
 * is decided here, it MIRRORS the door it will knock on, per verb — which is
 * not one blanket own-or-operator rule, because the four doors genuinely differ
 * (edit and retire are owner-or-operator, a true delete is strictly the owner's,
 * and a house entry is the operator's to write).
 *
 * FILTERS ARE SERVER PARAMETERS. `?scope=`/`?kind=`/`?status=` narrow the READ,
 * over closed S09.2 vocabularies the handler validates. Nothing in this file
 * sieves a served row out of the display by who owns it.
 */

/**
 * The four knowledge events the S09 store mints INSIDE the gate's own
 * transactions (internal/eventlog/contract.go:468–471). The feed is owner-scoped
 * per event `user_id` (internal/api/sse.go:51–58) and the event carries the
 * WRITER's id, so these frames push your own acts back to your own tab, and the
 * operator's unfiltered relay sees the household's.
 *
 * What that leaves uncovered is STATED on the surface (`LiveNote` below) rather
 * than covered up with a poll: a timer would be a second source of currency that
 * keeps running when the feed is down (§32).
 */
const memoryEventTypes = ['knowledge.write', 'knowledge.remove', 'knowledge.delete', 'knowledge.conflict'] as const

/**
 * The closed S09.2 filter vocabularies, exactly the handler's own maps
 * (internal/api/memory.go:68–84). An unknown value is a 400 naming the
 * vocabulary — never a silently empty page — so these lists are the whole set a
 * filter may ask for.
 */
const scopeVocabulary = ['user', 'project', 'house', 'worker_overlay'] as const
const kindVocabulary = [
  'lesson',
  'preference',
  'playbook',
  'rubric',
  'style',
  'exemplar',
  'convention',
  'observation',
  'taxonomy',
  'trigger_rules',
] as const
const statusVocabulary = ['proposed', 'active', 'retired', 'removed'] as const

/**
 * The NINE kinds a v0 write may name.
 *
 * `observation` is the L1 layer and L1 has no writer at v0 — the gate refuses it
 * with `no L1 writer is active at v0`, and that refusal IS the dormancy (S09.1;
 * S09.4 activation table). So it stays offerable as a FILTER, because rows of
 * that kind may exist to read, and is never offered as something to write.
 */
const writableKinds = kindVocabulary.filter((k) => k !== 'observation')

/** The status chip's tone. A status this list has never seen renders under its
 *  own served name in the neutral tone rather than disappearing — the same
 *  forward-tolerance the board gives kanban columns (§42). */
function statusTone(status: string): Tone {
  switch (status) {
    case 'active':
      return 'green'
    case 'proposed':
      return 'yellow'
    case 'retired':
      return 'orange'
    case 'removed':
      return 'red'
    default:
      return 'accent'
  }
}

/** Mono, tabular: every id, version, key and timestamp on this platform (§47). */
function Mono({ children }: { children: ReactNode }) {
  return <span className="font-mono text-[12px] tabular-nums">{children}</span>
}

// ── /memory — the browse ───────────────────────────────────────────────────

export function Memory({ me, operator, stream }: { me: string; operator: boolean; stream?: EventStream }) {
  const [scope, setScope] = useState('')
  const [kind, setKind] = useState('')
  const [status, setStatus] = useState('')

  const live = useLive({
    // The filters are part of the read's IDENTITY because they are part of the
    // REQUEST: a different filter is a different question, and the hook drops
    // the previous answer rather than showing one question's rows under
    // another's heading.
    key: `/api/memory|${scope}|${kind}|${status}`,
    read: () => api.memoryList({ scope, kind, status }),
    types: memoryEventTypes,
    stream,
  })

  const data = live.data
  const entries = data?.entries ?? []
  const projects = data?.projects ?? []

  return (
    <section className="surface">
      <h1>Memory</h1>
      <Freshness stale={live.stale} error={live.error} hasData={data !== null} />

      <p className="muted">
        What the platform has been told to remember, and what it may use again. Every entry here was written by a person
        through the S09 gate; nothing on this page was inferred.
      </p>

      <Panel head={<strong>What you can see here</strong>}>
        {/* The SERVED rule, verbatim. "Why is that entry not here?" is a question
            a person asks of a memory surface, and the honest answer is the
            server's own sentence rather than this client's summary of it. */}
        {data === null ? (
          <p className="muted">Reading…</p>
        ) : (
          <p data-visibility-rule="served" className="max-w-prose">
            {data.visibility}
          </p>
        )}
        {data !== null && (
          <p className="mt-2 text-xs text-muted-foreground" data-projects="served">
            {projects.length === 0 ? (
              <>
                Project scopes this answer rests on: <Absent reason="you own and are invited to no project" />
              </>
            ) : (
              <>
                Project scopes this answer rests on:{' '}
                {projects.map((p, i) => (
                  <span key={p}>
                    {i > 0 && ', '}
                    <Mono>{p}</Mono>
                  </span>
                ))}
              </>
            )}
          </p>
        )}
      </Panel>

      <LiveNote />

      <div className="my-4 flex flex-wrap items-end gap-3">
        <FilterSelect label="Scope" value={scope} onChange={setScope} options={scopeVocabulary} name="scope" />
        <FilterSelect label="Kind" value={kind} onChange={setKind} options={kindVocabulary} name="kind" />
        <FilterSelect label="Status" value={status} onChange={setStatus} options={statusVocabulary} name="status" />
        <span className="text-xs text-muted-foreground">
          These narrow the request itself, not this page — the control plane answers with the rows that match.
        </span>
      </div>

      <EntryComposer projects={projects} operator={operator} reload={live.reload} />

      {data !== null && entries.length === 0 ? (
        <EmptyState
          what="No entry matches this reading."
          why={
            <>
              Entries appear here when somebody writes one — a lesson worth keeping, a preference the platform should
              follow, a house convention. Write one with <strong>Remember something</strong> above, or widen the filters.
            </>
          }
        />
      ) : (
        <ul className="mt-4 flex list-none flex-col gap-2 p-0">
          {entries.map((e) => (
            <EntryRow key={e.entry_id} entry={e} me={me} />
          ))}
        </ul>
      )}

      {data?.truncated === true && (
        <p className="muted" data-truncated="true">
          This is one page of a longer set — the control plane bounds what one read returns. Narrow the filters to reach
          the rest; this page does not say how many more there are, because the read does not.
        </p>
      )}
    </section>
  )
}

/** One filter, over one closed vocabulary. The empty option is "no `?scope=` at
 *  all", which is a different request from any value — not a client-side "all". */
function FilterSelect({
  label,
  value,
  onChange,
  options,
  name,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  options: readonly string[]
  name: string
}) {
  return (
    <label className="flex flex-col gap-1 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <select
        data-filter={name}
        value={value}
        onChange={(e) => onChange(e.currentTarget.value)}
        className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
      >
        <option value="">Every {label.toLowerCase()}</option>
        {options.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
      </select>
    </label>
  )
}

/**
 * The honest-currency note (the inbox's `NoFrameNote` generalized, B6-6 OQ1).
 *
 * Say exactly what does NOT push, scoped to the truth of the feed's own
 * owner-scoping — never a poll, never a ticker, and never a claim of liveness
 * this surface cannot keep.
 */
function LiveNote() {
  return (
    <p className="muted" data-note="live-currency">
      Your own writes, retirements and deletions push this page as they land. Two things do not: a change somebody else
      makes to an entry you can see, and a question raised against one of your entries by somebody else's write — the
      feed carries an event to the person who caused it. This page re-reads when you open it and after every act here.
    </p>
  )
}

/** One row of the browse. The link is the entry's own stable URL, which is also
 *  what a conflict edge on another entry points at. */
function EntryRow({ entry, me }: { entry: MemoryEntry; me: string }) {
  return (
    <li data-entry-id={entry.entry_id} data-owner={entry.owner} data-status={entry.status}>
      <Panel className="motion-safe:transition-colors">
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-2">
          <Link to={hrefFor('memory-entry', { id: entry.entry_id })} className="font-medium">
            {entry.title}
          </Link>
          <Chip tone="accent" data-chip="kind">
            {entry.kind}
          </Chip>
          <Chip tone="blue" data-chip="scope">
            {entry.scope}
            {entry.scope_ref !== undefined && entry.scope_ref !== '' ? ` · ${entry.scope_ref}` : ''}
          </Chip>
          <Chip tone={statusTone(entry.status)} data-chip="status">
            {entry.status}
          </Chip>
        </div>
        <div className="mt-2 flex flex-wrap items-baseline gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <span>
            {entry.owner === me ? 'yours' : <>owner {entry.owner}</>}
          </span>
          <span>
            version <Mono>{String(entry.provenance.version)}</Mono>
          </span>
          <span>
            updated <Timestamp ts={entry.updated_ts} variant="live" />
          </span>
        </div>
      </Panel>
    </li>
  )
}

// ── /memory/:id — one entry ────────────────────────────────────────────────

export function MemoryEntryView({
  id,
  me,
  operator,
  stream,
}: {
  id: string
  me: string
  operator: boolean
  stream?: EventStream
}) {
  const live = useLive<MemoryEntryDetail>({
    key: `/api/memory/${id}`,
    read: () => api.memoryEntry(id),
    types: memoryEventTypes,
    stream,
  })
  const detail = live.data
  const entry = detail?.entry ?? null

  return (
    <section className="surface">
      <h1>Memory entry</h1>
      <p className="muted">
        <Link to={hrefFor('memory')}>← everything you can see</Link>
      </p>
      <Freshness stale={live.stale} error={live.error} hasData={detail !== null} />

      {entry !== null && (
        <>
          <EntryBody entry={entry} />
          <ConflictCards conflicts={detail?.conflicts ?? []} />
          <EntryVerbs entry={entry} me={me} operator={operator} reload={live.reload} />
        </>
      )}
    </section>
  )
}

/**
 * The whole served record, and only the served record.
 *
 * A TOMBSTONE is rendered as what it is: after a true deletion the wire carries
 * no content and no title, so this prints the stub's own note and does not fill
 * the gaps back in.
 */
function EntryBody({ entry }: { entry: MemoryEntry }) {
  return (
    <Panel
      head={
        <>
          <strong>{entry.tombstone ? 'A deleted entry' : entry.title}</strong>
          <Chip tone="accent" data-chip="kind">
            {entry.kind}
          </Chip>
          <Chip tone="blue" data-chip="scope">
            {entry.scope}
            {entry.scope_ref !== undefined && entry.scope_ref !== '' ? ` · ${entry.scope_ref}` : ''}
          </Chip>
          <Chip tone={statusTone(entry.status)} data-chip="status">
            {entry.status}
          </Chip>
        </>
      }
      data-entry-id={entry.entry_id}
      data-owner={entry.owner}
      data-tombstone={String(entry.tombstone)}
    >
      <dl className="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-[max-content_1fr]">
        <Field label="Entry">
          <Mono>{entry.entry_id}</Mono>
        </Field>
        <Field label="Owner">{entry.owner}</Field>
        <Field label="Layer">{entry.layer}</Field>
        {entry.topic_key !== undefined && entry.topic_key !== '' && (
          <Field label="Topic key">
            <Mono>{entry.topic_key}</Mono>
          </Field>
        )}
      </dl>

      {entry.tombstone ? (
        <p className="mt-4" data-tombstone-note="served">
          {entry.tombstone_note === undefined || entry.tombstone_note === '' ? (
            <Absent reason="the stub carries no note" />
          ) : (
            entry.tombstone_note
          )}
        </p>
      ) : entry.file_ref !== undefined && entry.file_ref !== '' ? (
        // A file-backed entry serves its ref and its commit rather than the
        // bytes: the row is the selector index, the file lives in the scope's
        // git-versioned knowledge dir, and git is its history (S09.2). Nothing
        // here fetches those bytes and nothing links to a filesystem.
        <p className="mt-4" data-file-backed="true">
          The content of this entry is a file in the scope's knowledge directory: <Mono>{entry.file_ref}</Mono> at commit{' '}
          <Mono>{entry.file_commit === undefined || entry.file_commit === '' ? '(no commit recorded)' : entry.file_commit}</Mono>. The
          row is the index; git holds the history.
        </p>
      ) : (
        <p className="mt-4 max-w-prose whitespace-pre-wrap" data-content="served">
          {entry.content === undefined || entry.content === '' ? (
            <Absent reason="this entry carries no body text" />
          ) : (
            entry.content
          )}
        </p>
      )}

      <SelectorsBlock selectors={entry.selectors} />
      <ProvenanceBlock entry={entry} />
      <VerificationBlock entry={entry} />
    </Panel>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="m-0 text-sm">{children}</dd>
    </>
  )
}

/** The S09.2 rule inputs. An empty selector set is an entry that carries no
 *  narrowing at all, which is a real answer and is said as one. */
function SelectorsBlock({ selectors }: { selectors: MemorySelectors }) {
  const rows: [string, string][] = []
  if (selectors.domain !== undefined && selectors.domain !== '') rows.push(['Domain', selectors.domain])
  if (selectors.project !== undefined && selectors.project !== '') rows.push(['Project', selectors.project])
  if (selectors.task_type !== undefined && selectors.task_type !== '') rows.push(['Task type', selectors.task_type])
  const triggers = selectors.triggers ?? []
  return (
    <div className="mt-4" data-block="selectors">
      <h3 className="text-xs tracking-wide text-muted-foreground uppercase">Selectors</h3>
      {rows.length === 0 && triggers.length === 0 ? (
        <p className="text-sm">
          <Absent reason="no selectors — nothing narrows where this entry applies" />
        </p>
      ) : (
        <dl className="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-[max-content_1fr]">
          {rows.map(([label, value]) => (
            <Field key={label} label={label}>
              <Mono>{value}</Mono>
            </Field>
          ))}
          {triggers.length > 0 && (
            <Field label="Triggers">
              {triggers.map((t, i) => (
                <span key={t}>
                  {i > 0 && ', '}
                  <Mono>{t}</Mono>
                </span>
              ))}
            </Field>
          )}
        </dl>
      )}
    </div>
  )
}

/** The S09.5 provenance block: where the entry came from, who approved it, and
 *  where it sits in its version chain. */
function ProvenanceBlock({ entry }: { entry: MemoryEntry }) {
  const p = entry.provenance
  return (
    <div className="mt-4" data-block="provenance">
      <h3 className="text-xs tracking-wide text-muted-foreground uppercase">Provenance</h3>
      <dl className="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-[max-content_1fr]">
        <Field label="Origin">
          <Mono>{p.origin}</Mono>
          {p.origin_ref !== undefined && p.origin_ref !== '' && (
            <>
              {' '}
              from <Mono>{p.origin_ref}</Mono>
            </>
          )}
        </Field>
        {p.proposer_model !== undefined && p.proposer_model !== '' && (
          <Field label="Proposed by">
            <Mono>{p.proposer_model}</Mono>
          </Field>
        )}
        <Field label="Approved by">
          {p.approved_by === undefined || p.approved_by === '' ? (
            <Absent reason="no approver recorded" />
          ) : (
            <>
              {p.approved_by} · <Timestamp ts={p.approved_ts} variant="audit" />
            </>
          )}
        </Field>
        <Field label="Version">
          <Mono>{String(p.version)}</Mono>
          {p.supersedes !== undefined && p.supersedes !== '' && (
            <>
              {' '}
              — supersedes{' '}
              <Link to={hrefFor('memory-entry', { id: p.supersedes })}>
                <Mono>{p.supersedes}</Mono>
              </Link>
            </>
          )}
        </Field>
        <Field label="Created">
          <Timestamp ts={entry.created_ts} variant="audit" />
        </Field>
        <Field label="Updated">
          <Timestamp ts={entry.updated_ts} variant="audit" />
        </Field>
      </dl>
    </div>
  )
}

/**
 * The S09.8 lifecycle block.
 *
 * A ZERO interval is a real answer and is said in words: an entry carries no
 * re-verify interval unless a human flags one. And an entry the platform has
 * never injected has no `last_injected_ts` at all — which is "never injected",
 * not the number zero and not a made-up instant.
 */
function VerificationBlock({ entry }: { entry: MemoryEntry }) {
  const v = entry.verification
  const interval = v.reverify_interval_days ?? 0
  return (
    <div className="mt-4" data-block="verification">
      <h3 className="text-xs tracking-wide text-muted-foreground uppercase">Verification</h3>
      <dl className="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-[max-content_1fr]">
        <Field label="Verified by">
          {v.verified_by === undefined || v.verified_by === '' ? (
            <Absent reason="no verifier recorded" />
          ) : (
            <>
              {v.verified_by} · <Timestamp ts={v.verified_ts} variant="audit" />
            </>
          )}
        </Field>
        <Field label="Re-verify">
          {interval === 0 ? (
            <span data-interval="none">no re-verify interval unless a human flags one</span>
          ) : (
            <span data-interval="set">
              every <Mono>{String(interval)}</Mono> days
            </span>
          )}
        </Field>
        {v.expires_ts !== undefined && v.expires_ts !== '' && (
          <Field label="Expires">
            <Timestamp ts={v.expires_ts} variant="audit" />
          </Field>
        )}
        <Field label="Last injected">
          {entry.last_injected_ts === undefined || entry.last_injected_ts === '' ? (
            <Absent reason="never injected into a run" />
          ) : (
            <Timestamp ts={entry.last_injected_ts} variant="audit" />
          )}
        </Field>
      </dl>
    </div>
  )
}

/**
 * The S09.7 question edges, exactly as served (drain D2).
 *
 * The list arrives ADDRESSEE-FILTERED server-side: a writer whose write raised a
 * question addressed to somebody else does not see that edge, because the card
 * is that person's to answer. That absence is honest and this surface does not
 * compensate for it.
 *
 * RESOLUTION IS NOT HERE. The ninth inbox kind owns the answering verb (§40-C
 * OQ6) — one door, one place — so this links to the card and re-implements
 * nothing.
 */
function ConflictCards({ conflicts }: { conflicts: MemoryConflict[] }) {
  if (conflicts.length === 0) return null
  return (
    <div className="mt-4" data-block="conflicts">
      <h2>Open questions about this entry</h2>
      <ul className="flex list-none flex-col gap-2 p-0">
        {conflicts.map((c) => (
          <li key={c.conflict_id} data-conflict-id={String(c.conflict_id)}>
            <Panel>
              <p className="max-w-prose" data-question="served">
                {c.question}
              </p>
              <p className="mt-2 text-xs text-muted-foreground">
                against{' '}
                <Link to={hrefFor('memory-entry', { id: c.other_entry_id })}>
                  <Mono>{c.other_entry_id}</Mono>
                </Link>{' '}
                · detected <Timestamp ts={c.detected_ts} variant="audit" />
              </p>
              <p className="mt-2 text-xs">
                <Link to={hrefFor('inbox-item', { id: `memory_conflict:${c.conflict_id}` })}>
                  Answer this in the inbox
                </Link>{' '}
                — a question card is answered where every other decision is.
              </p>
            </Panel>
          </li>
        ))}
      </ul>
    </div>
  )
}

// ── the verbs ──────────────────────────────────────────────────────────────

/**
 * Authority is MIRRORED FROM THE DOORS, per verb, and the doors differ.
 *
 *  - a new version is owner-or-operator (the gate refuses a co-member who can
 *    READ a project entry with `not the entry owner`, §40-C drain D6);
 *  - a removal is owner-or-operator (§17 reading 4);
 *  - a TRUE DELETION is strictly the owner's — the operator is refused, because
 *    the house authority bit is not a right to purge another person's memory
 *    (S4.1; G2 Def.9). Authorship, not visibility.
 *
 * `operator` here is the USERS-ROW role bit and deliberately not the read-side
 * dev-posture equivalence the fleet mirrors: this family's gate resolves the
 * actor against the users table, so a login-less dev identity is not an operator
 * to it — it is not a person to it at all, and its writes come back
 * `gate actor is not a known person`.
 */
function EntryVerbs({
  entry,
  me,
  operator,
  reload,
}: {
  entry: MemoryEntry
  me: string
  operator: boolean
  reload: () => void
}) {
  const owner = entry.owner === me
  const mayEdit = owner || operator
  const mayRemove = owner || operator
  const mayDelete = owner
  // A tombstone has nothing left to correct, retire or purge; the verbs are
  // about a live row.
  if (entry.tombstone) {
    return (
      <p className="muted" data-verbs="none">
        This entry has been deleted by its owner. What is left is the stub above.
      </p>
    )
  }
  return (
    <div className="mt-6 flex flex-col gap-4" data-verbs="entry">
      {mayEdit && <EditControl entry={entry} reload={reload} />}
      {mayRemove && <RetireControl entry={entry} reload={reload} />}
      {mayDelete ? (
        <DeleteControl entry={entry} reload={reload} />
      ) : (
        <p className="muted" data-verb-absent="delete">
          Purging this entry outright is its owner's own right and nobody else's — an administrator may retire it, and
          cannot delete it.
        </p>
      )}
    </div>
  )
}

/**
 * The draft a create and an edit both fill in. It maps ONE-TO-ONE onto the
 * landed write body and adds nothing to it (internal/api/memory.go:328–342).
 *
 * `origin` is never sent — omitted means `human_direct`, the S09.4 workspace
 * entry, which is the only ceremony this surface performs. `file_backed` is not
 * offered: the workspace entry is text-first, and file-backed authoring and the
 * operator's N20 import both stay off this surface at v0.
 */
type Draft = {
  scope: string
  scope_ref: string
  kind: string
  title: string
  content: string
  topic_key: string
  domain: string
  project: string
  task_type: string
  triggers: string
}

function emptyDraft(): Draft {
  return {
    scope: 'user',
    scope_ref: '',
    kind: 'lesson',
    title: '',
    content: '',
    topic_key: '',
    domain: '',
    project: '',
    task_type: '',
    triggers: '',
  }
}

function draftOf(entry: MemoryEntry): Draft {
  const s = entry.selectors
  return {
    scope: entry.scope,
    scope_ref: entry.scope_ref ?? '',
    kind: entry.kind,
    title: entry.title,
    content: entry.content ?? '',
    topic_key: entry.topic_key ?? '',
    domain: s.domain ?? '',
    project: s.project ?? '',
    task_type: s.task_type ?? '',
    triggers: (s.triggers ?? []).join(', '),
  }
}

function bodyOf(draft: Draft): MemoryWriteBody {
  const selectors: MemorySelectors = {}
  if (draft.domain.trim() !== '') selectors.domain = draft.domain.trim()
  if (draft.project.trim() !== '') selectors.project = draft.project.trim()
  if (draft.task_type.trim() !== '') selectors.task_type = draft.task_type.trim()
  const triggers = draft.triggers
    .split(',')
    .map((t) => t.trim())
    .filter((t) => t !== '')
  if (triggers.length > 0) selectors.triggers = triggers
  return {
    scope: draft.scope,
    ...(draft.scope === 'project' ? { scope_ref: draft.scope_ref } : {}),
    kind: draft.kind,
    title: draft.title.trim(),
    content: draft.content,
    ...(draft.topic_key.trim() === '' ? {} : { topic_key: draft.topic_key.trim() }),
    ...(Object.keys(selectors).length === 0 ? {} : { selectors }),
  }
}

/**
 * Pre-validation that mirrors what the gate obviously refuses, and NEVER answers
 * for it.
 *
 * It holds back a request the platform would certainly refuse and says why, in
 * this client's own words. It is a SEPARATE signal from busy — busy means one
 * thing, a request in flight — and it never replaces a server answer: anything
 * that fires renders whatever the gate says about it, verbatim.
 */
function heldBack(draft: Draft): string {
  if (draft.title.trim() === '') return 'An entry needs a title — it is how you and the platform find it again.'
  if (draft.content.trim() === '') return 'An entry needs something to remember. Write what the platform should know.'
  if (draft.scope === 'project' && draft.scope_ref === '')
    return 'A project entry has to name its project: project knowledge enters every run of that project.'
  return ''
}

/** The create door, on the browse. */
function EntryComposer({
  projects,
  operator,
  reload,
}: {
  projects: string[]
  operator: boolean
  reload: () => void
}) {
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [written, setWritten] = useState<MemoryEntryDetail | null>(null)
  const act = useAct()
  const held = heldBack(draft)

  return (
    <div data-control="memory-create">
      <Button
        variant="primary"
        size="sm"
        data-open="create"
        data-busy={String(act.busy)}
        disabled={act.busy}
        onClick={() => {
          act.clear()
          setWritten(null)
          setDraft(emptyDraft())
          setOpen(true)
        }}
      >
        Remember something
      </Button>
      <ActConfirm
        open={open}
        onOpenChange={setOpen}
        title="Write a new entry"
        what="Writing this is the approval: you are the person deciding, so the entry lands active and the platform may use it from the next run that matches it. You can correct it later — a correction is a new version — and you can retire or delete it at any time."
        act="Write it"
        variant="primary"
        busy={act.busy}
        disabled={held !== ''}
        onConfirm={() => {
          setOpen(false)
          act.run(
            () =>
              api.memoryCreate(bodyOf(draft)).then((res) => {
                setWritten(res)
                return outcomeOf(true, 'written', '')
              }),
            reload,
          )
        }}
      >
        <DraftForm draft={draft} setDraft={setDraft} projects={projects} operator={operator} held={held} scopeLocked={false} />
      </ActConfirm>
      <OutcomeLine outcome={act.outcome} />
      <WriteResult result={written} what="The entry as it now stands" />
    </div>
  )
}

/** The edit door, on the entry. An edit is a NEW VERSION and says so. */
function EditControl({ entry, reload }: { entry: MemoryEntry; reload: () => void }) {
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<Draft>(() => draftOf(entry))
  const [written, setWritten] = useState<MemoryEntryDetail | null>(null)
  const act = useAct()
  const held = heldBack(draft)

  return (
    <div data-control="memory-edit">
      <Button
        variant="secondary"
        size="sm"
        data-open="edit"
        data-busy={String(act.busy)}
        disabled={act.busy}
        onClick={() => {
          act.clear()
          setWritten(null)
          setDraft(draftOf(entry))
          setOpen(true)
        }}
      >
        Correct this entry
      </Button>
      <ActConfirm
        open={open}
        onOpenChange={setOpen}
        title="Write a new version"
        what="A correction is a new version, not an edit in place: this writes a fresh entry carrying everything below, and the version you are looking at is retired in the same transaction. Nothing is overwritten and nothing is lost — the old version stays readable, and the new one supersedes it from now on."
        act="Write the new version"
        variant="primary"
        busy={act.busy}
        disabled={held !== ''}
        onConfirm={() => {
          setOpen(false)
          act.run(
            () =>
              api.memoryNewVersion(entry.entry_id, bodyOf(draft)).then((res) => {
                setWritten(res)
                return outcomeOf(true, 'a new version is written', '')
              }),
            reload,
          )
        }}
      >
        {/* The scope of a correction is the scope of what it corrects: a new
            version supersedes one particular entry, so moving it to another
            scope is a different act (write a new entry there). */}
        <DraftForm draft={draft} setDraft={setDraft} projects={[]} operator={false} held={held} scopeLocked />
      </ActConfirm>
      <OutcomeLine outcome={act.outcome} />
      <WriteResult result={written} what="The new version" />
    </div>
  )
}

/**
 * A write's own answer, rendered where the write happened.
 *
 * The response is the entry's read-back WITH the conflict edges the write raised
 * that are addressed to the writer — so a write that raised a question shows the
 * question in the same breath rather than a page later.
 */
function WriteResult({ result, what }: { result: MemoryEntryDetail | null; what: string }) {
  if (result === null) return null
  const e = result.entry
  return (
    <div className="mt-2" data-write-result="served">
      <p className="text-sm">
        {what}:{' '}
        <Link to={hrefFor('memory-entry', { id: e.entry_id })}>
          <Mono>{e.entry_id}</Mono>
        </Link>{' '}
        · <Chip tone={statusTone(e.status)} data-chip="status">{e.status}</Chip> · version{' '}
        <Mono>{String(e.provenance.version)}</Mono>
        {e.provenance.supersedes !== undefined && e.provenance.supersedes !== '' && (
          <>
            {' '}
            superseding{' '}
            <Link to={hrefFor('memory-entry', { id: e.provenance.supersedes })}>
              <Mono>{e.provenance.supersedes}</Mono>
            </Link>
          </>
        )}
      </p>
      <ConflictCards conflicts={result.conflicts} />
    </div>
  )
}

/** The one form both write doors use. */
function DraftForm({
  draft,
  setDraft,
  projects,
  operator,
  held,
  scopeLocked,
}: {
  draft: Draft
  setDraft: (d: Draft) => void
  projects: string[]
  operator: boolean
  held: string
  scopeLocked: boolean
}) {
  const set = (patch: Partial<Draft>) => setDraft({ ...draft, ...patch })
  return (
    <div className="flex flex-col gap-3">
      {!scopeLocked && (
        <label className="flex flex-col gap-1 text-xs">
          <span className="text-muted-foreground">Who this is for</span>
          <select
            data-field="scope"
            value={draft.scope}
            onChange={(e) => set({ scope: e.currentTarget.value, scope_ref: '' })}
            className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
          >
            <option value="user">Just me</option>
            {/* Project scope is offered only where the served membership list
                says there is one to name, and the value comes FROM that list —
                never free text. Project knowledge enters every run of that
                project, so an unscoped write would be a way to steer other
                people's runs (S09.6). */}
            {projects.length > 0 && <option value="project">A project I am on</option>}
            {/* The house scope is the operator's to curate (D10). */}
            {operator && <option value="house">The whole household</option>}
          </select>
        </label>
      )}
      {!scopeLocked && draft.scope === 'project' && (
        <label className="flex flex-col gap-1 text-xs">
          <span className="text-muted-foreground">Which project</span>
          <select
            data-field="scope_ref"
            value={draft.scope_ref}
            onChange={(e) => set({ scope_ref: e.currentTarget.value })}
            className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
          >
            <option value="">Choose a project</option>
            {projects.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </label>
      )}
      {scopeLocked && (
        <p className="text-xs text-muted-foreground">
          This version stays in the scope of the entry it corrects: <Mono>{draft.scope}</Mono>
          {draft.scope_ref === '' ? '' : ' · '}
          {draft.scope_ref === '' ? '' : <Mono>{draft.scope_ref}</Mono>}.
        </p>
      )}
      <label className="flex flex-col gap-1 text-xs">
        <span className="text-muted-foreground">What kind of thing this is</span>
        <select
          data-field="kind"
          value={draft.kind}
          onChange={(e) => set({ kind: e.currentTarget.value })}
          className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
        >
          {writableKinds.map((k) => (
            <option key={k} value={k}>
              {k}
            </option>
          ))}
        </select>
      </label>
      <label className="flex flex-col gap-1 text-xs">
        <span className="text-muted-foreground">Title</span>
        <input
          type="text"
          data-field="title"
          value={draft.title}
          onChange={(e) => set({ title: e.currentTarget.value })}
          className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
        />
      </label>
      <label className="flex flex-col gap-1 text-xs">
        <span className="text-muted-foreground">What the platform should remember</span>
        <textarea
          data-field="content"
          rows={5}
          value={draft.content}
          onChange={(e) => set({ content: e.currentTarget.value })}
          className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
        />
      </label>
      <label className="flex flex-col gap-1 text-xs">
        <span className="text-muted-foreground">
          Topic key (optional) — entries sharing one are compared, and a disagreement becomes a question card
        </span>
        <input
          type="text"
          data-field="topic_key"
          value={draft.topic_key}
          onChange={(e) => set({ topic_key: e.currentTarget.value })}
          className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 font-mono text-sm"
        />
      </label>
      <fieldset className="m-0 flex flex-col gap-2 border border-border p-3">
        <legend className="px-1 text-xs text-muted-foreground">
          Where it applies (optional) — plain facts and phrases the platform looks up, never a similarity guess
        </legend>
        <input
          type="text"
          data-field="domain"
          placeholder="domain"
          value={draft.domain}
          onChange={(e) => set({ domain: e.currentTarget.value })}
          className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
        />
        <input
          type="text"
          data-field="selector_project"
          placeholder="project"
          value={draft.project}
          onChange={(e) => set({ project: e.currentTarget.value })}
          className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
        />
        <input
          type="text"
          data-field="task_type"
          placeholder="task type"
          value={draft.task_type}
          onChange={(e) => set({ task_type: e.currentTarget.value })}
          className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
        />
        <input
          type="text"
          data-field="triggers"
          placeholder="trigger phrases, separated by commas"
          value={draft.triggers}
          onChange={(e) => set({ triggers: e.currentTarget.value })}
          className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
        />
      </fieldset>
      {held !== '' && (
        <p className="warn-flag" data-held="true">
          {held}
        </p>
      )}
    </div>
  )
}

/**
 * Retire — and the confirm says exactly what retiring is, which is NOT deleting.
 *
 * The entry leaves every future assembly immediately; its content stays in git
 * history and its row stays for audit; a true deletion is a separate right with
 * a separate control. Nothing here says "gone".
 */
function RetireControl({ entry, reload }: { entry: MemoryEntry; reload: () => void }) {
  const [open, setOpen] = useState(false)
  const [why, setWhy] = useState('')
  const [removed, setRemoved] = useState<{ status: string; influence: MemoryInfluenceRef[] } | null>(null)
  const act = useAct()

  return (
    <div data-control="memory-retire">
      <Button
        variant="danger"
        size="sm"
        data-open="retire"
        data-busy={String(act.busy)}
        disabled={act.busy}
        onClick={() => {
          act.clear()
          setRemoved(null)
          setOpen(true)
        }}
      >
        Retire this entry
      </Button>
      <ActConfirm
        open={open}
        onOpenChange={setOpen}
        title="Retire this entry"
        what="Retiring takes this entry out of every future assembly straight away — nothing new will be built with it. Its content stays in git history and its row stays for audit, so this is not a deletion: purging the content outright is a separate right with its own control below."
        act="Retire it"
        variant="danger"
        busy={act.busy}
        onConfirm={() => {
          setOpen(false)
          act.run(
            () =>
              api.memoryRemove(entry.entry_id, why.trim() === '' ? undefined : why.trim()).then((res) => {
                setRemoved({ status: res.status, influence: res.influence })
                return outcomeOf(true, `status is now ${res.status}`, res.detail)
              }),
            reload,
          )
        }}
      >
        <label className="flex flex-col gap-1 text-xs">
          <span className="text-muted-foreground">Why (optional — it is kept with the removal)</span>
          <input
            type="text"
            data-field="reason"
            value={why}
            onChange={(e) => setWhy(e.currentTarget.value)}
            className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
          />
        </label>
      </ActConfirm>
      <OutcomeLine outcome={act.outcome} />
      {removed !== null && <InfluenceList influence={removed.influence} />}
    </div>
  )
}

/**
 * The S2.9 forward trace, served verbatim.
 *
 * It carries a run id and an event sequence and NOTHING ELSE — no task ref, no
 * deliverable ref — so there is no link target for a row here and none is
 * invented. The served detail names the re-verification offer in its own words
 * and renders above; no button is added for a verb the platform does not have.
 */
function InfluenceList({ influence }: { influence: MemoryInfluenceRef[] }) {
  return (
    <div className="mt-2" data-block="influence">
      <h3 className="text-xs tracking-wide text-muted-foreground uppercase">Where this entry had been used</h3>
      {influence.length === 0 ? (
        <p className="text-sm">
          <Absent reason="no run's trace manifest names this entry" />
        </p>
      ) : (
        <ul className="flex list-none flex-col gap-1 p-0 text-sm">
          {influence.map((i) => (
            <li key={`${i.run_id}/${i.event_seq}`} data-run-id={i.run_id}>
              <Mono>{i.run_id}</Mono> at event <Mono>{String(i.event_seq)}</Mono>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/**
 * True deletion — strictly the owner's, and honest about what it cannot reach.
 *
 * The four limits arrive as DATA on the answer (`limits[]`, the S09.5 honest
 * limits) and render verbatim afterwards. The confirm states the shape of the
 * act; the wire states its limits, and neither is this client's summary of the
 * other.
 */
function DeleteControl({ entry, reload }: { entry: MemoryEntry; reload: () => void }) {
  const [open, setOpen] = useState(false)
  const [deleted, setDeleted] = useState<{ note: string; limits: string[] } | null>(null)
  const act = useAct()

  return (
    <div data-control="memory-delete">
      <Button
        variant="danger"
        size="sm"
        data-open="delete"
        data-busy={String(act.busy)}
        disabled={act.busy}
        onClick={() => {
          act.clear()
          setDeleted(null)
          setOpen(true)
        }}
      >
        Delete this entry outright
      </Button>
      <ActConfirm
        open={open}
        onOpenChange={setOpen}
        title="Delete this entry outright"
        what="This purges the content: the body, the title, the topic key and the selectors are cleared and the knowledge file is removed from the working tree. What is left is a stub — its id, its dates and the note that it was removed at its owner's request. This is a different act from retiring, and the platform will tell you afterwards exactly what a purge does not reach."
        act="Delete it"
        variant="danger"
        busy={act.busy}
        onConfirm={() => {
          setOpen(false)
          act.run(
            () =>
              api.memoryDelete(entry.entry_id).then((res) => {
                setDeleted({ note: res.tombstone_note, limits: res.limits })
                return outcomeOf(res.tombstone, res.tombstone ? 'the content is purged' : 'no tombstone came back', res.detail)
              }),
            reload,
          )
        }}
      />
      <OutcomeLine outcome={act.outcome} />
      {deleted !== null && (
        <div className="mt-2" data-block="deletion-limits">
          <p className="text-sm" data-tombstone-note="served">
            {deleted.note}
          </p>
          <h3 className="mt-2 text-xs tracking-wide text-muted-foreground uppercase">What a purge does not reach</h3>
          <ul className="flex list-none flex-col gap-1 p-0 text-sm">
            {deleted.limits.map((l) => (
              <li key={l} data-limit="served">
                {l}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
