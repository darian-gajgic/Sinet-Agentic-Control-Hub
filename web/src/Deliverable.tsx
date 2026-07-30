import { useCallback, useState } from 'react'
import { Diff, Hunk, getChangeKey, markEdits, parseDiff, tokenize, type ChangeData, type FileData } from 'react-diff-view'
import 'react-diff-view/style/index.css'

import {
  ApiError,
  api,
  objectHref,
  staleAcceptCard,
  type AcceptCard,
  type AcceptOutcome,
  type Comment,
  type CommentCreate,
  type Comparison,
  type DeliverableDetail,
  type Door,
  type ObjectRef,
  type Placement,
  type PreviewComparison,
  type PreviewSession,
  type PreviewSessions,
  type Revision,
} from './api'
import type { EventStream } from './events'
import { deliverableEventTypes, describeError, useLive } from './live'
import { Absent, Empty, Freshness, Owner, Section, Stamp } from './parts'
import { Link } from './router'
import { hrefFor } from './routes'

/**
 * The review surface (Spec S15.8; S13.1–S13.4, S13.6, S13.8; FC-v1 §2).
 *
 * One deliverable, everything a person needs to review it: the immutable revision
 * lineage, the per-type comparison with its degrade lanes, the anchored comment
 * loop, the doors, try-it, and the one High-tier accept.
 *
 * FOUR RULES SHAPE EVERY BLOCK BELOW, and each of them is somebody else's
 * authority showing through:
 *
 *  - THE SERVER OWNS THE ANCHORS. Placements are recomputed server-side on every
 *    render (S13.3). This file composes a CLAIMED anchor from the rendered hunk
 *    data when somebody comments on a line, and then renders whatever status the
 *    ladder gave it — a claim on the wrong line comes back drifted or file-level
 *    and is shown that way, never as the position it asserted.
 *  - THE DOORS ARE THE ACTION SURFACE. A control renders only where a served door
 *    is `available`; a closed door renders its reason verbatim. No dead control,
 *    and no door this file invented (B6-3 OQ3).
 *  - EVERY PREVIEW DISPOSITION IS AN ANSWER. no-preview, self-preview,
 *    requires-container-tier, unavailable and at-capacity are states with reasons,
 *    and NONE of them gets an iframe: a broken frame is the failure S13.8 names.
 *  - THE DIFF TEXT IS THE SERVER'S. `Comparison.unified` is host-side git diff
 *    text and the only thing parsed here. There is no second diff parser, no
 *    server-rendered markup, and no server-side highlighter in the platform — so
 *    highlighting is the widget's own, client-side (FC-v1 §2).
 *
 * The ONE sanctioned raw-HTML channel on this page is the preview iframe, and it
 * rides `src` composed from a SERVED session URL plus the shell's own path state.
 * Nothing else here renders markup: diff text, comment bodies, labels, reasons and
 * trailers are text.
 */

// ── structural display constants, each with its reason (§42) ─────────────────
//
// None of these is a ⚙ key: S18 ratifies no setting for any of them, and the
// browser cannot read the registry before login anyway (the §41-B backoff
// precedent). Interim under the standing settings-tab directive.

/** Side-by-side is the review default because reviewing IS comparing two states;
 *  inline is one click away for a narrow screen or a long line. */
const defaultViewType: 'split' | 'unified' = 'split'

/** 2-up first for the same reason: two images side by side is the reading that
 *  needs no interaction to be useful, and swipe/onion are for the cases where a
 *  change is too small to see that way (S13.2's trio). */
const defaultImageMode: ImageMode = '2-up'

/** The swipe divider and the onion blend both start in the middle, which is the
 *  only position that shows both sides at once. The swipe value is carried as a
 *  whole-number CSS width so no arithmetic sits between the control and the style. */
const defaultSwipeAt = 50
const defaultBlend = 0.5

type ImageMode = '2-up' | 'swipe' | 'onion'

export function Deliverable({ id, me, stream }: { id: string; me: string; stream?: EventStream }) {
  const { data, error, stale, reload } = useLive<DeliverableDetail>({
    key: `/api/deliverables/${id}`,
    read: () => api.deliverable(id),
    types: deliverableEventTypes,
    stream,
  })

  return (
    <section className="surface deliverable">
      <h1>{data ? data.deliverable.id : 'Deliverable'}</h1>
      <Freshness stale={stale} error={error} hasData={data !== null} />
      {data && (
        <>
          <p className="muted">
            <Owner id={data.deliverable.owner} /> · {data.deliverable.type} ·{' '}
            <span className="dlv-state" data-state={data.deliverable.state}>
              {data.deliverable.state}
            </span>{' '}
            · <Link to={hrefFor('task', { id: data.deliverable.task_id })}>{data.deliverable.task_id}</Link>
            {data.deliverable.project_id ? <> · {data.deliverable.project_id}</> : null}
            {data.deliverable.subject_ref ? <> · {data.deliverable.subject_ref}</> : null}
          </p>
          <RevisionsBlock detail={data} stale={stale} />
          <ComparisonBlock detail={detailRefs(data)} me={me} stream={stream} />
          <DoorsBlock detail={data} />
          <TryItBlock detail={data} stream={stream} />
          <AcceptBlock detail={data} me={me} onApplied={reload} />
        </>
      )}
    </section>
  )
}

/** detailRefs narrows the detail to what the comparison block needs, so the diff
 *  panel cannot reach past the revision lineage it navigates. */
function detailRefs(detail: DeliverableDetail) {
  return { id: detail.deliverable.id, current: detail.deliverable.current_revision, revisions: detail.revisions }
}

type DetailRefs = ReturnType<typeof detailRefs>

/**
 * R2's first half: the lineage 1..N, never compressed.
 *
 * Every row shows what the platform RECORDED about that revision — its pin, the
 * minting run and attempt, and the verification verdict ref — because the numbers
 * are records, and a client that counted 1..N from `current_revision` would be
 * asserting a lineage it was never told.
 */
function RevisionsBlock({ detail, stale }: { detail: DeliverableDetail; stale: boolean }) {
  return (
    <Section title="Revisions" stale={stale}>
      {detail.revisions.length === 0 ? (
        <Empty what="No revision is minted yet, so there is nothing to review." />
      ) : (
        <ol className="revisions">
          {detail.revisions.map((r) => (
            <li key={r.n} data-revision={String(r.n)}>
              <span className="rev-n">revision {String(r.n)}</span>{' '}
              <span className="muted">
                {r.pin_kind} {revisionPin(r) === '' ? <Absent reason="no pin recorded" /> : revisionPin(r)}
              </span>
              <span className="muted">
                {' '}
                · minted by {r.run_id === undefined || r.run_id === '' ? <Absent reason="no minting run" /> : r.run_id}
              </span>
              <span className="muted">
                {' '}
                ·{' '}
                {r.verdict_ref === undefined || r.verdict_ref === 0 ? (
                  <Absent reason="no verification verdict recorded" />
                ) : (
                  <span data-verdict-ref={String(r.verdict_ref)}>verdict #{String(r.verdict_ref)}</span>
                )}
              </span>
              <span className="muted">
                {' '}
                · <Stamp ts={r.created_ts} />
              </span>
            </li>
          ))}
        </ol>
      )}
      <LineageEdges detail={detail} />
    </Section>
  )
}

function revisionPin(r: Revision): string {
  return r.content_sha256 ?? ''
}

/** R7's other half: follow-up lineage in BOTH directions, rendered as lineage. */
function LineageEdges({ detail }: { detail: DeliverableDetail }) {
  const lin = detail.lineage
  if (lin.succeeds.length === 0 && lin.succeeded_by.length === 0) {
    return <p className="muted">No follow-up lineage: this deliverable neither followed from one nor spawned a task.</p>
  }
  return (
    <div className="lineage">
      {lin.succeeds.map((s) => (
        <p key={`from-${s.deliverable_id}-${s.revision_n}`} data-lineage="succeeds">
          Follows from{' '}
          <Link to={hrefFor('deliverable', { id: s.deliverable_id })}>
            {s.deliverable_id} r{String(s.revision_n)}
          </Link>
        </p>
      ))}
      {lin.succeeded_by.map((s) => (
        <p key={`to-${s.task_id}`} data-lineage="succeeded-by">
          Followed up by <Link to={hrefFor('task', { id: s.task_id })}>{s.task_id}</Link>
        </p>
      ))}
    </div>
  )
}

// ── R1/R2/R3/R4/R5/R6: the comparison and the comment loop ───────────────────

/**
 * The comparison panel.
 *
 * The DEFAULT read sends no bounds at all, because round-over-round IS the
 * server's own default (new = current, old = new−1). A pair is only sent once
 * somebody navigates, and then it is sent explicitly — including `old=0`, the
 * pre-task base, which is a real target and not a revision.
 */
function ComparisonBlock({ detail, me, stream }: { detail: DetailRefs; me: string; stream?: EventStream }) {
  const [pair, setPair] = useState<{ old?: number; new?: number }>({})
  const [viewType, setViewType] = useState<'split' | 'unified'>(defaultViewType)
  const bounds = pair.new === undefined && pair.old === undefined ? '' : `?old=${pair.old ?? ''}&new=${pair.new ?? ''}`

  const cmp = useLive<Comparison>({
    key: `/api/deliverables/${detail.id}/compare${bounds}`,
    read: () => api.compare(detail.id, pair),
    types: deliverableEventTypes,
    stream,
  })

  return (
    <Section title="Comparison" stale={cmp.stale}>
      <Freshness stale={cmp.stale} error={cmp.error} hasData={cmp.data !== null} />
      <RevisionPicker detail={detail} pair={pair} onPick={setPair} served={cmp.data} />
      {cmp.data && (
        <>
          <SurfaceLabel cmp={cmp.data} />
          <SurfaceBody cmp={cmp.data} viewType={viewType} onViewType={setViewType} detail={detail} me={me} stream={stream} />
        </>
      )}
    </Section>
  )
}

/**
 * The pair navigation. The rendered pair is ALWAYS labelled from the served
 * comparison rather than from what was asked for, so a default read says which two
 * revisions it actually compared.
 */
function RevisionPicker({
  detail,
  pair,
  onPick,
  served,
}: {
  detail: DetailRefs
  pair: { old?: number; new?: number }
  onPick: (p: { old?: number; new?: number }) => void
  served: Comparison | null
}) {
  const numbers = detail.revisions.map((r) => r.n)
  return (
    <div className="rev-picker">
      <p data-compared={served ? `${served.old_n}:${served.new_n}` : ''}>
        {served ? (
          <>
            Comparing{' '}
            <strong>{served.old_n === 0 ? 'the pre-task base' : `revision ${String(served.old_n)}`}</strong> with{' '}
            <strong>revision {String(served.new_n)}</strong>
            {pair.new === undefined && pair.old === undefined && (
              <span className="muted"> — the platform&apos;s own round-over-round default</span>
            )}
          </>
        ) : (
          <Absent reason="no comparison read yet" />
        )}
      </p>
      <label>
        Older
        <select
          data-pick="old"
          value={String(pair.old ?? (served ? served.old_n : ''))}
          onChange={(e) => onPick({ ...pair, old: Number(e.target.value), new: pair.new ?? served?.new_n })}
        >
          {/* 0 is the pre-task base (S13.1) — the one option that is not a revision. */}
          <option value="0">the pre-task base</option>
          {numbers.map((n) => (
            <option key={n} value={String(n)}>
              revision {String(n)}
            </option>
          ))}
        </select>
      </label>
      <label>
        Newer
        <select
          data-pick="new"
          value={String(pair.new ?? (served ? served.new_n : ''))}
          onChange={(e) => onPick({ ...pair, new: Number(e.target.value), old: pair.old ?? served?.old_n })}
        >
          {numbers.map((n) => (
            <option key={n} value={String(n)}>
              revision {String(n)}
            </option>
          ))}
        </select>
      </label>
      {(pair.old !== undefined || pair.new !== undefined) && (
        <button type="button" data-action="round-over-round" onClick={() => onPick({})}>
          Back to round-over-round
        </button>
      )}
    </div>
  )
}

/** The served label, shown wherever the platform gave one. A fallback or a degrade
 *  NEVER poses as a rich surface: the label is the honesty, so it renders beside
 *  the surface rather than being folded into it. */
function SurfaceLabel({ cmp }: { cmp: Comparison }) {
  if (cmp.label === undefined || cmp.label === '') return null
  return (
    <p className={cmp.fallback === true ? 'warn-flag' : 'notice'} data-surface-label={cmp.surface}>
      {cmp.fallback === true ? 'Fallback surface: ' : ''}
      {cmp.label}
    </p>
  )
}

/**
 * One comparison, rendered per surface.
 *
 * The default arm is the point: a `surface` value this build does not know is a
 * FUTURE server's answer, and it renders the honest "no rich comparison surface"
 * state with the fields that were served rather than a blank or a crash (the §42
 * forward-tolerance, applied to a second vocabulary).
 */
function SurfaceBody({
  cmp,
  viewType,
  onViewType,
  detail,
  me,
  stream,
}: {
  cmp: Comparison
  viewType: 'split' | 'unified'
  onViewType: (v: 'split' | 'unified') => void
  detail: DetailRefs
  me: string
  stream?: EventStream
}) {
  switch (cmp.surface) {
    case 'line-diff':
    case 'extracted-text-diff':
      return (
        <DiffSurface cmp={cmp} viewType={viewType} onViewType={onViewType} detail={detail} me={me} stream={stream} />
      )
    case 'image-pair':
      return (
        <>
          <ImagePair cmp={cmp} />
          <OwnedCommentsBlock detail={detail} revision={cmp.new_n} me={me} stream={stream} anchored={null} />
        </>
      )
    case 'binary-cards':
      return (
        <>
          <ObjectCards cmp={cmp} />
          <OwnedCommentsBlock detail={detail} revision={cmp.new_n} me={me} stream={stream} anchored={null} />
        </>
      )
    default:
      return (
        <>
          <div className="surface-unknown" data-surface={cmp.surface}>
            <Absent
              reason={`no rich comparison surface for the answer "${cmp.surface}" — this build does not know how to draw it, so here is what the platform served`}
            />
            <dl className="served-fields">
              <dt>surface</dt>
              <dd>{cmp.surface}</dd>
              <dt>type</dt>
              <dd>{cmp.type}</dd>
              <dt>changed</dt>
              <dd>{cmp.changed === true ? 'the two sides differ by hash' : 'the two sides hash the same'}</dd>
            </dl>
          </div>
          <ObjectCards cmp={cmp} />
          <OwnedCommentsBlock detail={detail} revision={cmp.new_n} me={me} stream={stream} anchored={null} />
        </>
      )
  }
}

/**
 * R1: the diff widget.
 *
 * `Comparison.unified` goes through the pick's own `parseDiff` and nothing else —
 * there is no second parser here and no assumption that the server sent markup.
 * Highlighting is the widget's own tokenizer with `markEdits`, which marks the
 * changed WORDS inside a changed line and needs no language grammar. Syntax
 * highlighting would need a grammar package, which is a new adoption and not this
 * packet's; the recorded choice is edit marks now, and the tokenizer is where a
 * grammar would slot in.
 */
function DiffSurface({
  cmp,
  viewType,
  onViewType,
  detail,
  me,
  stream,
}: {
  cmp: Comparison
  viewType: 'split' | 'unified'
  onViewType: (v: 'split' | 'unified') => void
  detail: DetailRefs
  me: string
  stream?: EventStream
}) {
  const parsed = parseUnified(cmp.unified ?? '')
  const files: RenderedFile[] = parsed.files.map((file) => ({ file, path: renderedPath(file) }))
  const [claim, setClaim] = useState<ClaimedAnchor | null>(null)

  const placed = useLive<{ comments: Comment[]; placements: Placement[] }>({
    key: `/api/deliverables/${detail.id}/comments?revision=${cmp.new_n}`,
    read: () => api.comments(detail.id, cmp.new_n),
    types: deliverableEventTypes,
    stream,
  })

  return (
    <div className="diff-surface">
      <div className="diff-controls">
        {(['split', 'unified'] as const).map((v) => (
          <label key={v}>
            <input
              type="radio"
              name="diff-view"
              data-view-type={v}
              checked={viewType === v}
              onChange={() => onViewType(v)}
            />
            {v === 'split' ? 'Side by side' : 'Inline'}
          </label>
        ))}
      </div>
      {parsed.failure !== '' ? (
        <UnreadableDiff failure={parsed.failure} unified={cmp.unified ?? ''} />
      ) : files.length === 0 ? (
        <Empty what="The platform served no diff text for this pair — the two revisions have no differing file." />
      ) : (
        files.map(({ file, path }) => (
          <DiffFile
            key={path}
            file={file}
            path={path}
            viewType={viewType}
            comments={placed.data?.comments ?? []}
            placements={placed.data?.placements ?? []}
            onSelect={setClaim}
          />
        ))
      )}
      <CommentsBlock
        detail={detail}
        revision={cmp.new_n}
        me={me}
        anchored={claim}
        onClearAnchor={() => setClaim(null)}
        feed={placed}
        files={files}
      />
    </div>
  )
}

/**
 * parseUnified is the guarded parse, and the reason it exists is this packet's own
 * history: the deviation this surface required was a SERVED body that made this
 * exact parser throw. Fixing the producer was necessary and is not sufficient —
 * a parse that runs bare in render turns one malformed body into a dead review
 * surface for the whole application, because nothing above it catches.
 *
 * So the failure is an ANSWER, the same way an unrecognised `surface` value is: the
 * reason renders, and the text the platform served renders beside it as text, so a
 * person can still read the diff and say what is wrong with it.
 */
function parseUnified(unified: string): { files: FileData[]; failure: string } {
  if (unified === '') return { files: [], failure: '' }
  try {
    return { files: parseDiff(unified), failure: '' }
  } catch (err) {
    return { files: [], failure: err instanceof Error ? err.message : String(err) }
  }
}

function UnreadableDiff({ failure, unified }: { failure: string; unified: string }) {
  return (
    <div className="diff-unreadable" data-diff-unreadable="true">
      <Absent
        reason={`this diff could not be read as a unified diff (${failure}), so it cannot be drawn as one — the text the platform served is below, unchanged`}
      />
      <pre className="diff-raw">{unified}</pre>
    </div>
  )
}

/** What a line selection claims. It is composed from the RENDERED hunk data —
 *  the file the change belongs to, which side of the diff it is on, its line
 *  number and the line's own text — and it is a CLAIM: the server validates it. */
type ClaimedAnchor = { file_path: string; side: string; line_no: number; line_text: string }

function DiffFile({
  file,
  path,
  viewType,
  comments,
  placements,
  onSelect,
}: {
  file: FileData
  path: string
  viewType: 'split' | 'unified'
  comments: Comment[]
  placements: Placement[]
  onSelect: (a: ClaimedAnchor) => void
}) {
  const tokens = tokenize(file.hunks, { enhancers: [markEdits(file.hunks, { type: 'block' })] })
  const widgets = commentWidgets(file, path, comments, placements)

  return (
    <div className="diff-file" data-file={path}>
      <h4 className="diff-file-head">
        {path} <span className="muted">{file.type}</span>
      </h4>
      <Diff
        viewType={viewType}
        diffType={file.type}
        hunks={file.hunks}
        tokens={tokens}
        widgets={widgets}
        gutterEvents={{
          // The gutter of a split view has cells with no change on them (the
          // padding opposite an insert or a delete), so the change really can be
          // null and a click there anchors nothing rather than anchoring line 0.
          onClick: ({ change }) => {
            if (change !== null && change !== undefined) onSelect(claimFrom(path, change))
          },
        }}
      >
        {(hunks) => hunks.map((hunk) => <Hunk key={hunk.content} hunk={hunk} />)}
      </Diff>
    </div>
  )
}

/**
 * claimFrom composes the claimed anchor from one rendered change.
 *
 * A deletion is on the OLD side and an insertion on the new; an unchanged line
 * exists on both, and the new side is what a reviewer is looking at. `content` is
 * the line without its diff marker, which is exactly the quote selector the
 * server's ladder matches on — the marker would make every claim fail to confirm.
 */
function claimFrom(path: string, change: ChangeData): ClaimedAnchor {
  const side = change.type === 'delete' ? 'old' : 'new'
  const lineNo =
    change.type === 'normal'
      ? side === 'old'
        ? change.oldLineNumber
        : change.newLineNumber
      : change.lineNumber
  return { file_path: path, side, line_no: lineNo ?? 0, line_text: change.content }
}

/**
 * commentWidgets places every comment that HAS a live position in this file at
 * that position, keyed by the widget's own change key.
 *
 * The keys come from the parsed hunks, so a placement the widget cannot reach — a
 * line outside every rendered hunk, a file this pair does not show, a file-level or
 * orphan placement — is deliberately NOT forced in here. `widgetReach` is what
 * decides, and the strip renders its exact complement, which is what makes "no
 * comment without a render location" true rather than hopeful.
 */
function commentWidgets(
  file: FileData,
  path: string,
  comments: Comment[],
  placements: Placement[],
): Record<string, React.ReactNode> {
  const byID = new Map(comments.map((c) => [c.id, c]))
  const keyed = new Map<string, Comment[]>()
  for (const p of placements) {
    const c = byID.get(p.comment_id)
    if (c === undefined) continue
    const reach = widgetReach([{ file, path }], p)
    if (reach === null || reach.path !== path) continue
    keyed.set(reach.key, [...(keyed.get(reach.key) ?? []), c])
  }
  const out: Record<string, React.ReactNode> = {}
  for (const [key, list] of keyed) {
    out[key] = (
      <div className="diff-widget" data-widget-key={key}>
        {list.map((c) => (
          <CommentCard key={c.id} comment={c} placement={placements.find((p) => p.comment_id === c.id)} />
        ))}
      </div>
    )
  }
  return out
}

/** One rendered file of the current comparison, with the path the widget map and
 *  the strip both address it by. */
export type RenderedFile = { file: FileData; path: string }

/**
 * widgetReach answers ONE question, and it is the only place that answers it:
 * WILL a widget render this placement inside the rendered diff, and if so where?
 *
 * It exists because the answer was previously computed twice — once by the widget
 * map and once, differently, by the synthetic strip. The strip asked only whether
 * the placement had a line number, while the widget map additionally required the
 * file to be one of the rendered ones AND the line to fall inside a rendered hunk.
 * A placement that satisfied the first and failed the second rendered NOWHERE but
 * the flat list, which is the invisible-comment class this surface exists to
 * prevent (P-T12-2). The strip is now the exact complement of this function, so
 * the two cannot disagree again.
 */
export function widgetReach(
  files: RenderedFile[],
  placement: Placement | undefined,
): { path: string; key: string } | null {
  const anchor = placement?.anchor
  if (anchor === undefined || anchor.line_no === 0 || anchor.file_path === '') return null
  for (const { file, path } of files) {
    if (path !== anchor.file_path) continue
    const key = changeKeyAt(file, anchor.side, anchor.line_no)
    if (key !== null) return { path, key }
  }
  return null
}

/** renderedPath is the identity a parsed file is addressed by — the new side where
 *  there is one, the old side for a deletion. */
export function renderedPath(file: FileData): string {
  return file.newPath !== '' && file.newPath !== '/dev/null' ? file.newPath : file.oldPath
}

/** changeKeyAt finds the rendered change at (side, line) and returns the key the
 *  widget map is addressed by, or null when the line is not in a rendered hunk. */
function changeKeyAt(file: FileData, side: string, lineNo: number): string | null {
  for (const hunk of file.hunks) {
    for (const change of hunk.changes) {
      const at =
        change.type === 'normal'
          ? side === 'old'
            ? change.oldLineNumber
            : change.newLineNumber
          : (side === 'old') === (change.type === 'delete')
            ? change.lineNumber
            : undefined
      if (at === lineNo) return getChangeKey(change)
    }
  }
  return null
}

/**
 * R4/R5/R6: the comment list, the synthetic strip and the composer.
 *
 * THE INVARIANT: every served comment renders somewhere, and "somewhere" means a
 * widget inside the diff or the synthetic strip. A comment a widget WILL render
 * renders there and in this list; every other comment — file-level, orphaned, in a
 * file this pair does not show, or on a line outside every rendered hunk — renders
 * on the STRIP with its original quote and the revision it was said about. The two
 * sets are complements of ONE predicate (`widgetReach`) rather than two
 * hand-maintained conditions, because when they were two, a placement that fell
 * between them rendered nowhere but the flat list (P-T12-2).
 */
type CommentFeed = {
  data: { comments: Comment[]; placements: Placement[] } | null
  error: string
  stale: boolean
  reload: () => void
}

/**
 * OwnedCommentsBlock is the comment block on a surface that has no diff of its own
 * — the object and unknown-surface arms — so it owns the read.
 *
 * The split is not stylistic. `useLive`'s `key` is only an effect DEPENDENCY: the
 * read and the subscription run unconditionally, so a hook mounted with an empty
 * key still reads, still subscribes, and — the part that matters — still SETTLES
 * its §42 resnapshot debt for data nothing renders. Passing `key: ''` did not
 * disable anything. The honest answer is not to mount the hook at all, which is a
 * call-site fix: `useLive` is landed machinery and stays untouched.
 */
function OwnedCommentsBlock(props: Omit<Parameters<typeof CommentsBlock>[0], 'feed' | 'files'>) {
  const feed = useLive<{ comments: Comment[]; placements: Placement[] }>({
    key: `/api/deliverables/${props.detail.id}/comments?revision=${props.revision}`,
    read: () => api.comments(props.detail.id, props.revision),
    types: deliverableEventTypes,
    stream: props.stream,
  })
  return <CommentsBlock {...props} feed={feed} files={[]} />
}

function CommentsBlock({
  detail,
  revision,
  me,
  anchored,
  onClearAnchor,
  feed,
  files,
}: {
  detail: DetailRefs
  revision: number
  me: string
  stream?: EventStream
  anchored: ClaimedAnchor | null
  onClearAnchor?: () => void
  feed: CommentFeed
  /** The files of the CURRENT comparison, which is what decides whether a comment
   *  has a live position in THIS view. An empty list means no diff is on screen,
   *  so nothing has one and everything belongs on the strip. */
  files: RenderedFile[]
}) {
  const comments = feed.data?.comments ?? []
  const placements = feed.data?.placements ?? []
  const byID = new Map(placements.map((p) => [p.comment_id, p]))

  // The strip is the EXACT COMPLEMENT of widget reachability: every comment a
  // widget will not render inside the diff renders here, whatever the reason —
  // file-level, orphaned, in a file this pair does not show, or on a line outside
  // every rendered hunk. One function decides, so the two cannot drift apart.
  const unanchored = comments.filter((c) => widgetReach(files, byID.get(c.id)) === null)

  return (
    <div className="comments" data-revision={String(revision)}>
      <Freshness stale={feed.stale} error={feed.error} hasData={feed.data !== null} />
      <h4>Comments on revision {String(revision)}</h4>
      {comments.length === 0 ? (
        <Empty what="Nobody has commented on this revision yet." />
      ) : (
        <ul className="comment-list">
          {comments.map((c) => (
            <li key={c.id}>
              <CommentCard comment={c} placement={byID.get(c.id)} />
            </li>
          ))}
        </ul>
      )}

      <div className="synthetic-strip" data-strip="unanchored">
        <h5>Without a place in this view</h5>
        {unanchored.length === 0 ? (
          <Empty what="Every comment on this revision has a live position in the diff above." />
        ) : (
          <ul>
            {unanchored.map((c) => (
              <li key={c.id} data-strip-comment={String(c.id)}>
                <CommentCard comment={c} placement={byID.get(c.id)} />
              </li>
            ))}
          </ul>
        )}
      </div>

      <CommentComposer
        deliverable={detail.id}
        revision={revision}
        me={me}
        anchored={anchored}
        onClearAnchor={onClearAnchor}
        onCreated={feed.reload}
      />
    </div>
  )
}

/**
 * One comment, in the schema shared by human comments and verification findings.
 *
 * There is no edit control and no delete control, because no such verb exists at
 * any shape: immutability renders as the property it is (S13.3; P-T12-2), and
 * somebody who wants to change what they said adds another comment. A CONSUMED
 * comment shows its [F#], the batch stamp and the consuming attempt — which is how
 * "what did that rework receive" is answerable on the surface rather than only in
 * the log.
 */
function CommentCard({ comment, placement }: { comment: Comment; placement?: Placement }) {
  const c = comment
  return (
    <div
      className="comment"
      data-comment={String(c.id)}
      data-status={c.status}
      data-kind={c.kind}
      data-placement={placement?.status ?? 'unplaced'}
    >
      <p className="comment-head">
        <Owner id={c.owner} /> <span className="severity" data-severity={c.severity}>
          {severityMeaning(c.severity)}
        </span>{' '}
        <span className="muted">
          {c.kind} · said about revision {String(c.revision_n)} · <Stamp ts={c.created_ts} />
        </span>
      </p>
      <p className="comment-body">{c.body}</p>
      {c.suggested_change !== undefined && c.suggested_change !== '' && (
        <pre className="suggested">{c.suggested_change}</pre>
      )}
      {(c.category !== undefined && c.category !== '') || (c.criterion !== undefined && c.criterion !== '') ? (
        <p className="muted finding-meta">
          {c.category !== undefined && c.category !== '' ? <>category {c.category} </> : null}
          {c.criterion !== undefined && c.criterion !== '' ? <>· criterion {c.criterion}</> : null}
        </p>
      ) : null}
      <PlacementLine comment={c} placement={placement} />
      <LifecycleLine comment={c} />
    </div>
  )
}

/** blocker means another round; a note travels along with the batch (S13.4). An
 *  empty severity is served as-is and says so rather than being filled in. */
function severityMeaning(severity: string): string {
  switch (severity) {
    case 'blocker':
      return 'blocker — this triggers another round'
    case 'note':
      return 'note — polish, travels along'
    default:
      return severity === '' ? 'no severity recorded' : severity
  }
}

/**
 * Where the platform says this comment currently sits — and where it was ORIGINALLY
 * claimed to sit, whenever the two differ. A drifted placement says drifted: the
 * whole point of the recorded status is that a reader can tell a confirmed position
 * from a re-found one.
 */
function PlacementLine({ comment, placement }: { comment: Comment; placement?: Placement }) {
  const claimed = comment.anchor
  const live = placement?.anchor
  return (
    <p className="placement">
      <span className="placement-status" data-status={placement?.status ?? 'unplaced'}>
        {placementMeaning(placement?.status)}
      </span>
      {live !== undefined && live.line_no !== 0 ? (
        <span className="muted">
          {' '}
          · now at {live.file_path}:{String(live.line_no)} ({live.side} side)
        </span>
      ) : null}
      {live !== undefined && live.line_no === 0 && live.file_path !== '' ? (
        <span className="muted"> · on {live.file_path}</span>
      ) : null}
      {claimed !== undefined && claimed.line_text !== '' ? (
        <span className="quote">
          {' '}
          · quoted <code>{claimed.line_text}</code> at {claimed.file_path}:{String(claimed.line_no)}
        </span>
      ) : null}
      {comment.origin_anchor !== undefined && comment.origin_anchor !== '' ? (
        <span className="muted"> · as supplied: {comment.origin_anchor}</span>
      ) : null}
    </p>
  )
}

function placementMeaning(status: string | undefined): string {
  switch (status) {
    case 'exact':
      return 'exact — the quote is at the line it names'
    case 'mapped':
      return 'mapped — the line moved and the quote confirms where'
    case 'drifted':
      return 'drifted — the quote was found near, not at, the mapped position'
    case 'file':
      return 'file-level — no line position, the quote is kept'
    case 'orphan':
      return 'orphaned — no live location in this revision at all'
    case undefined:
      return 'no placement served for this revision'
    default:
      // A status this build does not know is a future server's answer, and the
      // raw value is more honest than a friendly name nobody decided on.
      return status
  }
}

function LifecycleLine({ comment }: { comment: Comment }) {
  if (comment.status !== 'consumed') {
    return <p className="lifecycle" data-lifecycle="open">Open — the next round will drain this.</p>
  }
  return (
    <p className="lifecycle" data-lifecycle="consumed">
      <span className="finding-number">
        [F{comment.finding_number === undefined ? '?' : String(comment.finding_number)}]
      </span>{' '}
      consumed <Stamp ts={comment.consumed_at} /> by{' '}
      {comment.consumed_by === undefined || comment.consumed_by === '' ? (
        <Absent reason="no consuming attempt recorded" />
      ) : (
        <span className="attempt-ref">{comment.consumed_by}</span>
      )}
    </p>
  )
}

/**
 * The composer. Two entries, both first-class: a line selected in the diff carries
 * a claimed anchor, and a file-level or deliverable-level comment carries none.
 *
 * The response's own `anchor_status` is what renders afterwards — so a claim on a
 * line the quote is not on comes back drifted or file-level, and the person SEES
 * the position the server gave it rather than the one they asserted.
 */
function CommentComposer({
  deliverable,
  revision,
  me,
  anchored,
  onClearAnchor,
  onCreated,
}: {
  deliverable: string
  revision: number
  me: string
  anchored: ClaimedAnchor | null
  onClearAnchor?: () => void
  onCreated: () => void
}) {
  const [body, setBody] = useState('')
  const [severity, setSeverity] = useState('')
  const [suggested, setSuggested] = useState('')
  const [fileLevel, setFileLevel] = useState('')
  const [born, setBorn] = useState<Comment | null>(null)
  const [failure, setFailure] = useState('')

  const submit = () => {
    setFailure('')
    const payload: CommentCreate = { revision, body, severity }
    if (anchored !== null) payload.anchor = anchored
    else if (fileLevel !== '') payload.file_level = fileLevel
    if (suggested !== '') payload.suggested_change = suggested
    api.addComment(deliverable, payload).then(
      (created) => {
        setBorn(created)
        setBody('')
        setSuggested('')
        onClearAnchor?.()
        onCreated()
      },
      (err: unknown) => setFailure(describeError(err)),
    )
  }

  return (
    <form
      className="comment-composer"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <h5>Add a comment as {me === '' ? 'yourself' : me}</h5>
      <p className="muted" data-claim={anchored === null ? '' : `${anchored.file_path}:${anchored.line_no}`}>
        {anchored === null ? (
          <>Select a line in the diff to anchor this, or leave it file-level.</>
        ) : (
          <>
            Anchored at {anchored.file_path}:{String(anchored.line_no)} ({anchored.side} side), quoting{' '}
            <code>{anchored.line_text}</code>{' '}
            <button type="button" data-action="clear-anchor" onClick={() => onClearAnchor?.()}>
              Drop the anchor
            </button>
          </>
        )}
      </p>
      {anchored === null && (
        <label>
          File (optional)
          <input data-field="file-level" value={fileLevel} onChange={(e) => setFileLevel(e.target.value)} />
        </label>
      )}
      <label>
        What you want to say
        <textarea data-field="body" value={body} onChange={(e) => setBody(e.target.value)} />
      </label>
      <label>
        Severity
        {/* The closed two-value vocabulary, plus the empty choice the verb accepts. */}
        <select data-field="severity" value={severity} onChange={(e) => setSeverity(e.target.value)}>
          <option value="">none given</option>
          <option value="blocker">blocker — another round</option>
          <option value="note">note — polish travels along</option>
        </select>
      </label>
      <label>
        Suggested change (optional)
        <textarea data-field="suggested" value={suggested} onChange={(e) => setSuggested(e.target.value)} />
      </label>
      <button type="submit" data-action="add-comment" disabled={body.trim() === ''}>
        Add comment
      </button>
      {failure !== '' && <p className="error">{failure}</p>}
      {born !== null && (
        <p className="notice" data-born-status={born.anchor_status}>
          Recorded. The platform placed it as <strong>{placementMeaning(born.anchor_status)}</strong> — that is where it
          lives, whatever position was claimed.
        </p>
      )}
    </form>
  )
}

// ── R3: the object surfaces ─────────────────────────────────────────────────

/**
 * The S13.2 image trio: 2-up, swipe and onion-skin, over the per-side objects.
 *
 * The bytes come from the owner-scoped object read, addressed by the sha the
 * SERVED ObjectRef carries. Whether a given type is served INLINE is the server's
 * decision and this file keeps no copy of that rule — it renders the image and, if
 * the browser could not paint it, says so with the recorded type, which is learned
 * from the actual response rather than from a duplicated allowlist.
 */
function ImagePair({ cmp }: { cmp: Comparison }) {
  const [mode, setMode] = useState<ImageMode>(defaultImageMode)
  const [swipeAt, setSwipeAt] = useState(defaultSwipeAt)
  const [blend, setBlend] = useState(defaultBlend)
  const oldRef = (cmp.old_objects ?? [])[0]
  const newRef = (cmp.new_objects ?? [])[0]

  return (
    <div className="image-pair" data-mode={mode}>
      <ChangedVerdict cmp={cmp} />
      <div className="image-modes">
        {(['2-up', 'swipe', 'onion'] as const).map((m) => (
          <label key={m}>
            <input type="radio" name="image-mode" data-image-mode={m} checked={mode === m} onChange={() => setMode(m)} />
            {m}
          </label>
        ))}
      </div>
      {mode === '2-up' && (
        <div className="two-up">
          <ImageSide label={sideLabel('old', cmp.old_n)} deliverable={cmp.deliverable_id} ref_={oldRef} />
          <ImageSide label={sideLabel('new', cmp.new_n)} deliverable={cmp.deliverable_id} ref_={newRef} />
        </div>
      )}
      {mode === 'swipe' && (
        <div className="swipe">
          <div className="swipe-stack">
            <ImageSide label={sideLabel('new', cmp.new_n)} deliverable={cmp.deliverable_id} ref_={newRef} />
            <div className="swipe-top" style={{ width: `${swipeAt}%` }}>
              <ImageSide label={sideLabel('old', cmp.old_n)} deliverable={cmp.deliverable_id} ref_={oldRef} />
            </div>
          </div>
          <input
            type="range"
            data-control="swipe"
            min={0}
            max={100}
            value={swipeAt}
            onChange={(e) => setSwipeAt(Number(e.target.value))}
            aria-label="Swipe between the two revisions"
          />
        </div>
      )}
      {mode === 'onion' && (
        <div className="onion">
          <div className="onion-stack">
            <ImageSide label={sideLabel('old', cmp.old_n)} deliverable={cmp.deliverable_id} ref_={oldRef} />
            <div className="onion-top" style={{ opacity: blend }}>
              <ImageSide label={sideLabel('new', cmp.new_n)} deliverable={cmp.deliverable_id} ref_={newRef} />
            </div>
          </div>
          <input
            type="range"
            data-control="onion"
            min={0}
            max={1}
            step={0.01}
            value={blend}
            onChange={(e) => setBlend(Number(e.target.value))}
            aria-label="Blend the two revisions"
          />
        </div>
      )}
      <PixelDiffAid cmp={cmp} />
      <ObjectCards cmp={cmp} />
    </div>
  )
}

function sideLabel(side: string, n: number): string {
  if (n === 0) return 'the pre-task base'
  return `${side === 'old' ? 'older' : 'newer'} — revision ${String(n)}`
}

function ImageSide({ label, deliverable, ref_ }: { label: string; deliverable: string; ref_?: ObjectRef }) {
  const [failed, setFailed] = useState(false)
  if (ref_ === undefined) {
    return (
      <figure className="image-side" data-image-side="absent">
        <Absent reason={`${label}: this side pins no object`} />
      </figure>
    )
  }
  return (
    <figure className="image-side" data-image-side={ref_.sha256}>
      {failed ? (
        <Absent
          reason={`${label}: the platform does not serve ${ref_.type === undefined || ref_.type === '' ? 'this type' : ref_.type} inline, so it cannot be drawn here — download the object to inspect it`}
        />
      ) : (
        <img src={objectHref(deliverable, ref_.sha256)} alt={`${label}: ${ref_.name}`} onError={() => setFailed(true)} />
      )}
      <figcaption className="muted">{label}</figcaption>
    </figure>
  )
}

/** The optional server-side aid. Absent at v0 — the seam exists, and saying
 *  nothing is the truthful render when no producer filled it. */
function PixelDiffAid({ cmp }: { cmp: Comparison }) {
  if (cmp.pixel_diff === undefined) {
    return <p className="muted">No pixel-diff aid was computed for this pair.</p>
  }
  return (
    <p className="pixel-diff" data-pixel-diff="served">
      Pixel-diff aid:{' '}
      {cmp.pixel_diff.changed_ratio === undefined ? (
        <Absent reason="no changed ratio recorded" />
      ) : (
        <>changed ratio {String(cmp.pixel_diff.changed_ratio)} as measured</>
      )}
      {cmp.pixel_diff.diff_object_sha !== undefined && cmp.pixel_diff.diff_object_sha !== '' && (
        <>
          {' '}
          ·{' '}
          <a href={objectHref(cmp.deliverable_id, cmp.pixel_diff.diff_object_sha)} data-action="download-pixel-diff">
            the aid image
          </a>
        </>
      )}
    </p>
  )
}

/** The by-hash verdict, which is the one thing an object surface can say with
 *  certainty about two opaque blobs. */
function ChangedVerdict({ cmp }: { cmp: Comparison }) {
  return (
    <p className="changed-verdict" data-changed={cmp.changed === true ? 'true' : 'false'}>
      {cmp.changed === true
        ? 'The two sides differ: their content hashes are not the same.'
        : 'The two sides are identical by content hash.'}
    </p>
  )
}

/**
 * R3: the metadata card per side, plus download-to-inspect.
 *
 * A binary comparison is honest about what it can and cannot say: the name, the
 * size, the recorded type and the hash are facts, "what changed inside" is not one
 * this platform computes, so the answer is the hash verdict and the bytes.
 */
function ObjectCards({ cmp }: { cmp: Comparison }) {
  const sides: { label: string; refs: ObjectRef[] }[] = [
    { label: sideLabel('old', cmp.old_n), refs: cmp.old_objects ?? [] },
    { label: sideLabel('new', cmp.new_n), refs: cmp.new_objects ?? [] },
  ]
  if (sides.every((s) => s.refs.length === 0)) return null
  return (
    <div className="object-cards">
      <ChangedVerdict cmp={cmp} />
      {sides.map((side) => (
        <div className="object-side" key={side.label} data-object-side={side.label}>
          <h5>{side.label}</h5>
          {side.refs.length === 0 ? (
            <Absent reason="this side pins no object" />
          ) : (
            <ul>
              {side.refs.map((ref) => (
                <li key={ref.sha256} data-object={ref.sha256}>
                  <span className="object-name">{ref.name}</span>{' '}
                  <span className="muted">
                    {String(ref.size)} bytes ·{' '}
                    {ref.type === undefined || ref.type === '' ? <Absent reason="no type recorded" /> : ref.type}
                  </span>
                  <br />
                  <code className="object-sha">{ref.sha256}</code>{' '}
                  <a href={objectHref(cmp.deliverable_id, ref.sha256)} data-action="download-object" download={ref.name}>
                    Download to inspect
                  </a>
                </li>
              ))}
            </ul>
          )}
        </div>
      ))}
    </div>
  )
}

// ── R7: the doors ───────────────────────────────────────────────────────────

/**
 * Every served door renders. An available one is a control; a closed one renders
 * its reason verbatim and nothing pressable. This file adds no door and hides
 * none — which is checkable, because the list comes from the served array.
 *
 * The two acting doors are the request-revision limbs. Both are LANDED verbs the
 * door itself names: the rework limb answers an open card with the card's own ask,
 * pin and `revise_with_guidance`, and the finished limb spawns the follow-up under
 * the `revision` preset. Neither is a verb this surface invented.
 */
function DoorsBlock({ detail }: { detail: DeliverableDetail }) {
  return (
    <Section title="What you can do">
      {detail.doors.length === 0 ? (
        <Empty what="The platform named no doors on this deliverable." />
      ) : (
        <ul className="doors">
          {detail.doors.map((door) => (
            <li key={door.verb} data-door={door.verb} data-available={door.available ? 'true' : 'false'}>
              <DoorRow door={door} />
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}

function DoorRow({ door }: { door: Door }) {
  return (
    <>
      <p className="door-head">
        <span className="door-verb">{door.verb}</span>{' '}
        <span className={door.available ? 'notice' : 'muted'}>{door.available ? 'open' : 'closed'}</span>
      </p>
      <p className="door-reason">{door.reason}</p>
      {door.route !== '' && (
        <p className="muted door-route">
          {door.method} {door.route}
          {door.preset !== undefined && door.preset !== '' ? <> · preset {door.preset}</> : null}
          {door.pin_from !== undefined && door.pin_from !== '' ? <> · pin from {door.pin_from}</> : null}
        </p>
      )}
      {door.available && door.verb === 'request-revision' && <RequestRevision door={door} />}
    </>
  )
}

/**
 * The rework limb. It posts the door's OWN ask id, the door's OWN pin and the
 * `revise_with_guidance` answer the door named — nothing composed here. The
 * guidance is durable requester comments that reach the next attempt through THE
 * drain, and the door's reason is what says so on screen.
 *
 * A stale pin is never retried: the door's hash came from a read, and a 409 means
 * the card moved, so the answer is a re-read of the deliverable rather than a
 * second post.
 */
function RequestRevision({ door }: { door: Door }) {
  const [guidance, setGuidance] = useState('')
  const [pin, setPin] = useState('')
  const [outcome, setOutcome] = useState('')
  const [failure, setFailure] = useState('')
  const [stale, setStale] = useState(false)

  const submit = () => {
    setFailure('')
    setStale(false)
    if (door.route === '' || door.payload_hash === undefined || door.answer === undefined) {
      setFailure('The door named no route, pin or answer, so there is nothing to send.')
      return
    }
    // The guidance rides the ANSWER, in ONE request. Two facts make that the only
    // correct shape, and the first cut here got both wrong by posting a separate
    // comment first:
    //
    //  - the card's own answer schema REQUIRES the points. `revise_with_guidance`
    //    with an empty `guidance` list is refused by the validator, so an answer
    //    that left them out could never have worked;
    //  - the drain runs INLINE inside this request. The answer records the
    //    guidance as durable requester comments and then drains the open set into
    //    the resumed attempt, in that order, in one transaction's worth of work.
    //    A comment posted before the answer duplicates on a retry and is
    //    undeletable; a comment posted after it arrives too late to be drained
    //    into the round it was written for.
    //
    // So there is exactly one write, and a refusal means nothing was written.
    api
      .answerAtDoor(door.route, {
        payload_hash: door.payload_hash,
        answer: { choice: door.answer, guidance: [{ text: guidance }] },
        pin: pin === '' ? undefined : pin,
      })
      .then(
        (res) => {
          setPin('')
          setGuidance('')
          setOutcome(res.detail ?? res.state)
        },
        (err: unknown) => {
          setPin('')
          if (err instanceof ApiError && err.code === 'stale_payload') {
            setStale(true)
            return
          }
          setFailure(describeError(err))
        },
      )
  }

  return (
    <form
      className="request-revision"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <label>
        Guidance for the next round
        <textarea data-field="guidance" value={guidance} onChange={(e) => setGuidance(e.target.value)} />
      </label>
      {/* Severity is not offered here, and the absence is the point: the platform
          records a guidance point as a BLOCKER itself (guidance always names a
          problem to fix), so a severity control on this form would be this surface
          claiming a choice the answer schema does not have. */}
      <label>
        PIN, if this card asks for one
        <input type="password" data-field="revision-pin" value={pin} onChange={(e) => setPin(e.target.value)} />
      </label>
      <button type="submit" data-action="request-revision" disabled={guidance.trim() === ''}>
        Ask for a revision
      </button>
      {guidance.trim() === '' && (
        <p className="muted">
          A revision request needs at least one point to act on — the card&apos;s own answer schema refuses an empty one,
          so there is nothing to send until you say what to change.
        </p>
      )}
      {stale && (
        <p className="warn-flag" data-stale="request-revision">
          The card moved since this page was read, so NOTHING was written — not the guidance, not the answer. Reload the
          deliverable to pick up the card as it now is, then say it again.
        </p>
      )}
      {outcome !== '' && <p className="notice">{outcome}</p>}
      {failure !== '' && <p className="error">{failure}</p>}
    </form>
  )
}

// ── R8/R9: try it out ───────────────────────────────────────────────────────

/**
 * Try-it, and the honest shape of "synced navigation" (S13.8; OQ2 as ratified).
 *
 * The preview origins are a tailnet subdomain or a loopback port, so they are
 * CROSS-ORIGIN to this page. That is not a limitation to work around; it is the
 * fact the design has to be honest about: the shell can neither read a frame's
 * current location nor observe a click inside it. So `sync.mode: "path"` is
 * SHELL-DRIVEN — one shared path control composes `<side.url><path>` and sets both
 * frames together — and the surface SAYS that a click inside a frame moves that
 * frame alone, with a re-sync control that re-asserts the shell's path on both.
 * There is no postMessage protocol (it would mean injecting script into somebody's
 * app) and no proxy rewriting.
 */
function TryItBlock({ detail, stream }: { detail: DeliverableDetail; stream?: EventStream }) {
  const previewDoor = detail.doors.find((d) => d.verb === 'preview')
  const compareDoor = detail.doors.find((d) => d.verb === 'preview-compare')
  const [session, setSession] = useState<PreviewSession | null>(null)
  const [pair, setPair] = useState<PreviewComparison | null>(null)
  const [failure, setFailure] = useState('')

  const sessions = useLive<PreviewSessions>({
    key: '/api/previews',
    read: () => api.previews(),
    types: deliverableEventTypes,
    stream,
  })

  const launch = (run: () => Promise<void>) => {
    setFailure('')
    run().catch((err: unknown) => setFailure(describeError(err)))
  }

  return (
    <Section title="Try it out" stale={sessions.stale}>
      {previewDoor === undefined ? (
        <Absent reason="the platform named no preview door on this deliverable" />
      ) : previewDoor.available ? (
        <button
          type="button"
          data-action="launch-preview"
          onClick={() =>
            launch(() =>
              api.launchPreview(detail.deliverable.id).then((s) => {
                setSession(s)
              }),
            )
          }
        >
          Launch a preview
        </button>
      ) : (
        <p className="muted" data-closed="preview">
          {previewDoor.reason}
        </p>
      )}
      {compareDoor !== undefined && compareDoor.available && (
        <button
          type="button"
          data-action="launch-compare"
          onClick={() =>
            launch(() =>
              api.previewComparison(detail.deliverable.id).then((c) => {
                setPair(c)
              }),
            )
          }
        >
          Compare before and after
        </button>
      )}
      {failure !== '' && <p className="error">{failure}</p>}
      {session !== null && <SessionPanel session={session} />}
      {pair !== null && <DualFrames pair={pair} />}
      <LiveSessions feed={sessions} />
    </Section>
  )
}

/**
 * One launched session. A backed one LINKS its served URL — it does not embed it;
 * embedding is the before/after comparison's job, where the sandboxed frames live.
 * Every NON-backed disposition renders its reason as the answer it is, and renders
 * NO frame at all, because an empty or broken iframe is the failure S13.8 names.
 */
function SessionPanel({ session }: { session: PreviewSession }) {
  const backed = session.state === 'live' && session.url !== undefined && session.url !== ''
  return (
    <div className="preview-session" data-preview-state={session.state} data-backed={backed ? 'true' : 'false'}>
      <p>
        <span className="preview-lane">{session.lane}</span> · revision {String(session.revision)} ·{' '}
        <span className="preview-state">{session.state}</span>
        {session.routed ? <span className="muted"> · routed through the front chain</span> : null}
      </p>
      {backed ? (
        <>
          <p>
            <a href={session.url} data-action="open-preview" target="_blank" rel="noreferrer noopener">
              {session.url}
            </a>
          </p>
          <PortPicker session={session} />
        </>
      ) : (
        <Absent
          reason={
            session.reason === undefined || session.reason === ''
              ? `this revision has no running preview: the platform answered "${session.state}"`
              : session.reason
          }
        />
      )}
    </div>
  )
}

/** The multi-port picker's data is served; a single-port session needs no picker
 *  and gets none. */
function PortPicker({ session }: { session: PreviewSession }) {
  const ports = session.ports ?? []
  const [chosen, setChosen] = useState(ports[0]?.number)
  if (ports.length < 2) return null
  return (
    <div className="port-picker" data-ports={String(ports.length)}>
      <p className="muted">This preview listens on more than one port — pick the one you want.</p>
      {ports.map((p) => (
        <label key={p.number}>
          <input
            type="radio"
            name="preview-port"
            data-port={String(p.number)}
            checked={chosen === p.number}
            onChange={() => setChosen(p.number)}
          />
          {String(p.number)}
          {p.label !== undefined && p.label !== '' ? ` — ${p.label}` : ''}
        </label>
      ))}
    </div>
  )
}

/**
 * R9: the dual frames.
 *
 * Both `src` values are composed from EXACTLY the served `SessionView.url` plus the
 * shell's own path — never from deliverable content, comment text or anything a
 * model produced. The inline-document attribute — the one the escape scan now
 * bans by name — is used nowhere: raw HTML by attribute is exactly the banned
 * class, and the sanctioned channel is a URL reference.
 */
function DualFrames({ pair }: { pair: PreviewComparison }) {
  const [path, setPath] = useState('/')
  const [applied, setApplied] = useState('/')
  const resync = () => setApplied(path)

  return (
    <div className="dual-frames" data-single-instance={pair.single_instance ? 'true' : 'false'}>
      <form
        className="frame-path"
        onSubmit={(e) => {
          e.preventDefault()
          resync()
        }}
      >
        <label>
          Path on both sides
          <input data-field="frame-path" value={path} onChange={(e) => setPath(e.target.value)} />
        </label>
        <button type="submit" data-action="apply-path">
          Go
        </button>
        <button type="button" data-action="resync" onClick={resync}>
          Re-sync both sides
        </button>
      </form>
      <p className="muted" data-sync-mode={pair.sync.mode} data-sync-enabled={pair.sync.enabled ? 'true' : 'false'}>
        {pair.sync.enabled
          ? 'Synced navigation is driven from here: the path above is applied to both sides together. A click INSIDE a frame moves that frame alone — these previews are separate origins, so this page cannot see where a frame has navigated to. Re-sync puts both back on the path above.'
          : 'Nothing to sync: there is only one instance below.'}
      </p>
      <div className="frames">
        {pair.before === undefined ? (
          <div className="frame-absent" data-frame="before">
            <Absent
              reason={
                pair.before === undefined && pair.single_instance
                  ? `no "before" exists: ${pair.after.reason === undefined || pair.after.reason === '' ? 'this deliverable has no accepted revision yet, and a second pane would be a fake' : pair.after.reason}`
                  : 'no before side was served'
              }
            />
          </div>
        ) : (
          <FrameSide side={pair.before} path={applied} />
        )}
        <FrameSide side={pair.after} path={applied} />
      </div>
    </div>
  )
}

function FrameSide({ side, path }: { side: import('./api').SessionView; path: string }) {
  const backed = side.state === 'live' && side.url !== ''
  if (!backed) {
    return (
      <div className="frame-absent" data-frame={side.role} data-preview-state={side.state}>
        <Absent
          reason={
            side.reason === undefined || side.reason === ''
              ? `the ${side.role} side answered "${side.state}", so there is nothing to embed`
              : side.reason
          }
        />
      </div>
    )
  }
  return (
    <div className="frame" data-frame={side.role}>
      <p className="muted">
        {side.role} · revision {String(side.revision)}
      </p>
      <iframe
        title={`${side.role} preview of ${side.deliverable}`}
        src={frameSrc(side.url, path)}
        sandbox={previewSandbox}
        // The SPA's own URL carries the deliverable id, and it would otherwise
        // travel to the preview as `Referer`. Same reason the external preview
        // link carries rel="noreferrer noopener".
        referrerPolicy="no-referrer"
      />
    </div>
  )
}

/**
 * The sandbox the preview frame rides in, token by token.
 *
 * S13.3 names this channel "the sandboxed rendered-document view" and
 * `review.EscapeFirst()` records it under exactly that name, so sandboxing is the
 * CONTRACT rather than a hardening preference. What is framed is a built
 * application whose content came out of a model, and being cross-origin stops a
 * frame reading this document's DOM — it does NOT stop the frame navigating the
 * top-level window. On a surface that collects a PIN, top-navigation to a
 * look-alike sign-in is the concrete risk, so it is the one capability most
 * deliberately withheld.
 *
 * GRANTED, each because a preview cannot be a fair try-out without it:
 *   allow-scripts      — a dev-server preview IS its scripts; without this the
 *                        frame renders an empty shell and the review is a lie.
 *   allow-forms        — reviewing an interface means submitting its forms.
 *   allow-same-origin  — the previewed app needs its OWN origin back for storage
 *                        and its own fetches. This is safe here for a structural
 *                        reason worth stating rather than leaving to luck: a
 *                        preview is served from a different origin than the SPA
 *                        (the tailnet `preview-<id>.<host>` subdomain, or a
 *                        loopback port out of this package's 47900-47919 pool), so
 *                        the token restores the FRAME's origin and never this
 *                        page's. If a preview were ever served same-origin with
 *                        the SPA, this token would defeat the whole sandbox and
 *                        would have to go.
 *   allow-popups       — an app that opens a window should be seen doing it. The
 *                        escape token is deliberately NOT granted, so a popup
 *                        inherits this same sandbox.
 *
 * WITHHELD, and why the absence is deliberate:
 *   allow-top-navigation (and the by-user-activation form) — the phishing edge
 *                        above; nothing in a preview needs to move this window.
 *   allow-modals       — a framed app could otherwise block the reviewing page
 *                        with a dialog.
 *   allow-downloads    — a try-out has no business writing to the reviewer's disk.
 *   allow-pointer-lock / allow-presentation / allow-orientation-lock — nothing a
 *                        preview does needs them.
 */
export const previewSandbox = 'allow-scripts allow-forms allow-same-origin allow-popups'

/** frameSrc is the whole composition rule: the SERVED base URL, then the shell's
 *  path. Nothing else can reach an iframe src on this page. */
export function frameSrc(base: string, path: string): string {
  const suffix = path.startsWith('/') ? path : `/${path}`
  return base.replace(/\/+$/, '') + suffix
}

/** The owner's live sessions, with stop. A partial teardown keeps the session, so
 *  the served answer is what renders — the surface never claims a stop completed
 *  because a button was pressed. */
function LiveSessions({ feed }: { feed: ReturnType<typeof useLive<PreviewSessions>> }) {
  const [note, setNote] = useState('')
  const [failure, setFailure] = useState('')
  const sessions = feed.data?.sessions ?? []
  return (
    <div className="live-previews">
      <h4>Preview sessions running now</h4>
      <Freshness stale={feed.stale} error={feed.error} hasData={feed.data !== null} />
      {sessions.length === 0 ? (
        <Empty what="No preview session is running." />
      ) : (
        <ul>
          {sessions.map((s) => (
            <li key={s.id} data-session={s.id}>
              <span className="muted">
                {s.deliverable} r{String(s.revision)} · {s.state} · {s.lane} · <Owner id={s.user} />
              </span>{' '}
              <button
                type="button"
                data-action="stop-preview"
                onClick={() => {
                  setFailure('')
                  api.stopPreview(s.id).then(
                    (res) => {
                      setNote(res.detail)
                      feed.reload()
                    },
                    (err: unknown) => {
                      setFailure(describeError(err))
                      feed.reload()
                    },
                  )
                }}
              >
                Stop
              </button>
            </li>
          ))}
        </ul>
      )}
      {note !== '' && <p className="notice" data-stop-detail="served">{note}</p>}
      {failure !== '' && (
        <p className="error" data-stop-failed="true">
          {failure} The session is kept, so stopping it again is safe.
        </p>
      )}
    </div>
  )
}

// ── R10: the accept ─────────────────────────────────────────────────────────

/**
 * The one High-tier action on the reviewed revision (S13.6; S15.6).
 *
 * PRESENTATION FOLLOWS AUTHORSHIP. The accept is the owner's own outward act,
 * pushed with their own credentials, so the FORM renders for the owner only. A
 * non-owner — the operator included — reads the same card with the authorship line
 * instead of a form. That is presentation over a served body (S15.2): the server
 * is the authority and its refusal is what actually stops a non-owner.
 *
 * The PIN rides the same request as the answer and lives nowhere else: it is
 * cleared the moment the request is fired, and it is never written to state that
 * outlives the act, to storage, or to a log.
 */
function AcceptBlock({ detail, me, onApplied }: { detail: DeliverableDetail; me: string; onApplied: () => void }) {
  const door = detail.doors.find((d) => d.verb === 'accept')
  const [card, setCard] = useState<AcceptCard | null>(null)
  const [outcome, setOutcome] = useState<AcceptOutcome | null>(null)
  const [changed, setChanged] = useState(false)
  const [failure, setFailure] = useState('')
  const [pinRefused, setPinRefused] = useState('')
  const owner = detail.deliverable.owner
  const mine = me !== '' && me === owner

  const read = useCallback(() => {
    setFailure('')
    api.acceptCard(detail.deliverable.id).then(
      (c) => setCard(c),
      (err: unknown) => setFailure(describeError(err)),
    )
  }, [detail.deliverable.id])

  return (
    <Section title="Accept">
      {door === undefined ? (
        <Absent reason="the platform named no accept door on this deliverable" />
      ) : (
        <>
          <p className="door-reason">{door.reason}</p>
          <button type="button" data-action="read-accept-card" onClick={read}>
            Read the accept card
          </button>
        </>
      )}
      {failure !== '' && <p className="error">{failure}</p>}
      {card !== null && (
        <>
          {changed && (
            <p className="warn-flag" data-stale-accept="true">
              The card moved since it was read, so nothing was accepted. What is below is the card as it is NOW — check
              it before accepting again.
            </p>
          )}
          <AcceptCardView card={card} />
          {!mine ? (
            <p className="muted" data-authorship="not-mine">
              This is {owner}&apos;s work to accept. An accept pushes a commit under the accepting person&apos;s own
              credentials, so only they can do it — reading the card is not the same as being able to act on it.
            </p>
          ) : card.acceptable ? (
            <AcceptForm
              card={card}
              onOutcome={(o) => {
                setOutcome(o)
                setChanged(false)
                setPinRefused('')
                onApplied()
              }}
              onStale={(fresh) => {
                setCard(fresh)
                setChanged(true)
                setOutcome(null)
              }}
              onPinRefused={setPinRefused}
              onFailure={setFailure}
            />
          ) : (
            <p className="muted" data-not-acceptable="true">
              {card.reason}
            </p>
          )}
          {pinRefused !== '' && (
            <p className="error" data-pin-refused="true">
              {pinRefused} Enter your PIN again — nothing was accepted.
            </p>
          )}
        </>
      )}
      {outcome !== null && <AcceptOutcomeView outcome={outcome} />}
    </Section>
  )
}

/** Every served field, including the trailers byte-for-byte — the S13.6 step-3
 *  rule is that a person SEES the attribution before it becomes a permanent
 *  commit, not afterwards. */
function AcceptCardView({ card }: { card: AcceptCard }) {
  return (
    <div className="accept-card" data-acceptable={card.acceptable ? 'true' : 'false'}>
      <dl>
        <dt>Revision</dt>
        <dd>{String(card.revision_n)}</dd>
        <dt>Content pin</dt>
        <dd>
          {card.pin_kind}{' '}
          {card.content_pin === '' ? <Absent reason="no pin recorded" /> : <code>{card.content_pin}</code>}
        </dd>
        <dt>Pushes to</dt>
        <dd>
          {card.project_id === '' ? <Absent reason="this deliverable belongs to no project" /> : card.project_id}
          {card.protected_ref !== undefined && card.protected_ref !== '' ? (
            <> · {card.protected_ref}</>
          ) : (
            <> · <Absent reason="no protected ref is registered for this project" /></>
          )}
        </dd>
        <dt>Tier</dt>
        <dd data-tier={card.tier}>
          {card.tier} — {card.tier_statement}
        </dd>
        <dt>Payload pin</dt>
        <dd>
          <code data-payload-hash={card.payload_hash}>{card.payload_hash}</code>
        </dd>
      </dl>
      <h5>Commit trailers, exactly as they will be written</h5>
      {card.trailers === '' ? (
        <Absent
          reason={
            card.provenance.absent === undefined || card.provenance.absent === ''
              ? 'no trailers could be rendered'
              : card.provenance.absent
          }
        />
      ) : (
        <pre className="trailers" data-trailers="verbatim">
          {card.trailers}
        </pre>
      )}
      <p className="muted provenance">
        {card.provenance.minting_run_id === undefined || card.provenance.minting_run_id === '' ? (
          <Absent reason="no minting run recorded" />
        ) : (
          <>
            from run {card.provenance.minting_run_id} · engine {card.provenance.engine ?? ''} · model{' '}
            {card.provenance.model ?? ''} · lane {card.provenance.lane ?? ''} · {card.provenance.vendor_noreply ?? ''}
          </>
        )}
      </p>
      <p className="signing" data-signing-structural={card.signing.structural ? 'true' : 'false'}>
        {card.signing.statement}
      </p>
    </div>
  )
}

function AcceptForm({
  card,
  onOutcome,
  onStale,
  onPinRefused,
  onFailure,
}: {
  card: AcceptCard
  onOutcome: (o: AcceptOutcome) => void
  onStale: (fresh: AcceptCard) => void
  onPinRefused: (msg: string) => void
  onFailure: (msg: string) => void
}) {
  const [pin, setPin] = useState('')
  const [subject, setSubject] = useState('')
  const [provenance, setProvenance] = useState('')
  const [sending, setSending] = useState(false)

  const submit = () => {
    setSending(true)
    onPinRefused('')
    const body = { payload_hash: card.payload_hash, pin, subject, provenance }
    // The PIN leaves this component the instant the request carries it. There is
    // no elevation to keep and nothing to re-use it for.
    setPin('')
    api.accept(card.deliverable_id, body).then(
      (out) => {
        setSending(false)
        onOutcome(out)
      },
      (err: unknown) => {
        setSending(false)
        const fresh = staleAcceptCard(err)
        if (fresh !== null) {
          // A stale pin is never retried: the card moved, so the next act is a
          // re-render of what is actually there.
          onStale(fresh)
          return
        }
        if (err instanceof ApiError && (err.code === 'pin_required' || err.status === 401)) {
          onPinRefused(err.message)
          return
        }
        onFailure(describeError(err))
      },
    )
  }

  return (
    <form
      className="accept-form"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <label>
        Commit subject (optional)
        <input data-field="subject" value={subject} onChange={(e) => setSubject(e.target.value)} />
      </label>
      <label>
        Provenance note (optional)
        <input data-field="provenance" value={provenance} onChange={(e) => setProvenance(e.target.value)} />
      </label>
      <label>
        Your PIN
        <input type="password" data-field="accept-pin" value={pin} onChange={(e) => setPin(e.target.value)} />
      </label>
      <button type="submit" data-action="accept" disabled={sending}>
        Accept this revision
      </button>
    </form>
  )
}

/**
 * Every outcome, rendered honestly.
 *
 * `applied:false` is not an error in either of its two forms: an already-accepted
 * deliverable reads its recorded outcome back, which is what makes a phone retry
 * safe, and a collision answers with a merge card and "nothing was pushed".
 */
function AcceptOutcomeView({ outcome }: { outcome: AcceptOutcome }) {
  return (
    <div className="accept-outcome" data-applied={outcome.applied ? 'true' : 'false'} data-state={outcome.state}>
      <p className={outcome.applied ? 'notice' : 'muted'}>{outcome.detail}</p>
      <dl>
        <dt>State</dt>
        <dd>{outcome.state}</dd>
        <dt>Commit</dt>
        <dd>
          {outcome.commit === undefined || outcome.commit === '' ? (
            <Absent reason="no commit recorded for this outcome" />
          ) : (
            <code data-commit={outcome.commit}>{outcome.commit}</code>
          )}
        </dd>
        <dt>Effect</dt>
        <dd>
          {outcome.effect_id === undefined || outcome.effect_id === '' ? (
            <Absent reason="no effect was proposed" />
          ) : (
            <code data-effect-id={outcome.effect_id}>{outcome.effect_id}</code>
          )}
        </dd>
        <dt>Superseded</dt>
        <dd>
          {outcome.superseded.length === 0 ? (
            <Absent reason="nothing was superseded" />
          ) : (
            outcome.superseded.join(', ')
          )}
        </dd>
        <dt>Sent for re-validation</dt>
        <dd>
          {outcome.routed_runs.length === 0 ? (
            <Absent reason="no active run needed re-validation" />
          ) : (
            outcome.routed_runs.join(', ')
          )}
        </dd>
      </dl>
      {outcome.merge_card !== undefined && <MergeCardPanel view={outcome.merge_card} />}
    </div>
  )
}

/**
 * The collision surface. All three options are `answerable:false` and each says
 * why — a button that 403s or 501s is the dead control the answerable-with-a-reason
 * discipline exists to prevent. The third names a LANDED door that reaches the same
 * outcome, which is where a person actually goes.
 */
function MergeCardPanel({ view }: { view: import('./api').MergeCardView }) {
  return (
    <div className="merge-card" data-merge-card="true">
      <h5>This does not apply cleanly — nothing was pushed</h5>
      {view.card === null ? (
        <Absent reason="the collision card itself was not served" />
      ) : (
        <dl>
          <dt>Onto</dt>
          <dd>
            <code>{view.card.onto}</code>
          </dd>
          <dt>Candidate</dt>
          <dd>
            <code>{view.card.candidate}</code>
          </dd>
          <dt>Why</dt>
          <dd>{view.card.reason}</dd>
          {view.card.conflicts !== undefined && view.card.conflicts !== '' && (
            <>
              <dt>Conflicts</dt>
              <dd>
                <pre>{view.card.conflicts}</pre>
              </dd>
            </>
          )}
        </dl>
      )}
      <ul className="merge-options">
        {view.options.map((o) => (
          <li key={o.option} data-merge-option={o.option} data-answerable={o.answerable ? 'true' : 'false'}>
            <span className="option-name">{o.option}</span> — {o.reason}
            {o.route !== undefined && o.route !== '' && (
              <span className="muted">
                {' '}
                · {o.route}
                {o.preset !== undefined && o.preset !== '' ? ` · preset ${o.preset}` : ''}
              </span>
            )}
          </li>
        ))}
      </ul>
      <p className="muted" data-durability="served">
        {view.durability}
      </p>
    </div>
  )
}
