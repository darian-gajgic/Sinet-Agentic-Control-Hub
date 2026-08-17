import { useCallback, useEffect, useState } from 'react'
import { Diff, Hunk, getChangeKey, markEdits, parseDiff, tokenize, type ChangeData, type FileData } from 'react-diff-view'
import 'react-diff-view/style/index.css'

import { composeResult, humanBytes, resultSizeCap, type ComposedResult, type ResultFile } from './resultDoc'

import {
  ApiError,
  api,
  objectHref,
  objectText,
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
import { ActConfirm, OutcomeLine, useAct } from './controls'
import { Absent, Empty, Freshness, Owner, Section, Stamp, SurfaceHead } from './parts'
import { Link } from './router'
import { hrefFor } from './routes'
import { Button, Chip, EmptyState, cn, toneStyle, type Tone } from './ui'

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

// ── the restyle vocabulary (P3-UI-7 seam C-2; design proposal §1/§3) ─────────
//
// THE METHOD IS §51's, unchanged: a class keeps its name as the DOM hook it
// already was, and the PRESENTATION moves to utilities. Nothing here is renamed,
// so every landed assertion that selects on a class or a `data-` attribute still
// selects on it.
//
// ⚠ THE ONE RULE THAT BOUNDS EVERYTHING BELOW (§53's carried hazard). Since the
// cascade fix the owned sheet sits in `@layer base`, so an owned rule can no
// longer override a utility on the same element — the `.chat-verbs` regression.
// The review section keeps ten rules that `responsive.test.ts` is written
// against, and NO element carrying one of them is given a utility for the same
// property here. The kept set, and the property each keeps, is listed at the
// stylesheet's own section head.

/** The resting look of a review region — the A2 `chatPanel` idiom (§53), same
 *  tokens, same reason: `Panel`'s hover lift/shadow is right for a card you can
 *  press and wrong for a static region, and several of these regions are
 *  landmark elements a generic `<div>` would erase. Border, radius, panel
 *  gradient and density padding; no motion, no hover. */
const reviewPanel = 'rounded-(--radius) border border-border bg-(image:--panel-grad) p-(--density-pad)'

/**
 * The composer textareas.
 *
 * `textarea` is the ONE form element the sheet's global chrome never covered
 * (`input, select, button`), so before this restyle every free-text box on the
 * surface rendered the user agent's own default beside inputs that carried the
 * owned chrome. The utilities give it the same border, radius, surface and
 * padding the native controls already have, plus the 4rem floor and the
 * `min-width: 0` the shed rule carried — which is what makes a long line wrap
 * inside a flex column instead of widening it.
 */
const reviewTextarea =
  'min-h-16 w-full min-w-0 rounded-(--radius-sm) border border-border bg-background p-2 text-foreground'

/**
 * ⚠ THE RADIO ROWS, and this is a REPORTED LEGIBILITY DEFECT this restyle fixes
 * rather than a cosmetic preference.
 *
 * The sheet's global chrome makes every `label` a flex COLUMN and every
 * `input` `width: 100%`. Inside the three review pickers — which are flex ROWS
 * at the breakpoint — that stacked a stretched radio ABOVE its own text and let
 * the text overflow its column, so at 1280px the shipped page read
 * "(o) (o) / Side by side  Inline": two radios adjacent and two labels adjacent,
 * with nothing saying which belonged to which. Verified in a real browser over
 * the served dist, and verified LANDED rather than introduced here — the label
 * markup is byte-identical to the pre-packet file.
 *
 * The fix is a utility, and after the cascade fix that is the only shape it can
 * take: an owned rule can no longer override a utility, so the winner has to be
 * the utility. It competes with the GLOBAL `label`/`input` chrome and with
 * nothing this section keeps — the three container rules sit on the parents.
 */
const radioRow = 'flex-row items-center gap-2'
const radioInput = 'w-auto'

/** The one treatment for a served id, hash, count or line number — §47's
 *  "every number, id, timestamp and status token". `code`, `pre` and `time`
 *  already carry it from the sheet's own global rule, so this is for the spans
 *  that are none of those. */
const figure = 'font-mono tabular-nums'

/** A tone-carrying inline token, on the ONE chip formula (§47) over `--tone`.
 *  It is deliberately NOT the kit `Chip`: these slots render the platform's own
 *  MEANING SENTENCES verbatim ("blocker — this triggers another round"), and a
 *  sentence given the uppercase mono status-token treatment stops reading as a
 *  sentence — the §53 A2 content-vs-status-token judgement, applied the other
 *  way round. Single-WORD statuses do take the kit `Chip`. */
const toneToken =
  'inline-flex items-center gap-1.5 rounded-(--radius-sm) border px-1.5 py-0.5 text-xs ' +
  'text-(--tone) border-[color-mix(in_srgb,var(--tone)_25%,transparent)] ' +
  'bg-[color-mix(in_srgb,var(--tone)_9%,transparent)]'

/**
 * Every tone map below is over a CLOSED served vocabulary, and every one of them
 * hands back `null` for a value this build has never seen.
 *
 * That is the §51 D5b rule applied: a status colour that disagrees with the word
 * beside it is the one thing a status colour must not do, and a future server's
 * value coloured by a guess would do exactly that. `null` renders the value in
 * the neutral treatment with no tone claimed — the same forward tolerance the
 * text arms already take (`placementMeaning`'s default returns the raw value).
 */
function severityTone(severity: string): Tone | null {
  switch (severity) {
    case 'blocker':
      return 'red'
    case 'note':
      return 'blue'
    default:
      return null
  }
}

/** The five S13.3 placement statuses. `exact` and `mapped` are both confirmed
 *  positions, so both read as settled; `drifted` is the near-miss the recorded
 *  status exists to make visible; `orphan` has no live location at all, which is
 *  the state a reviewer most needs to notice. `file` claims no line and takes no
 *  hue, because file-level is not a degraded line-level — it is its own answer. */
function placementTone(status: string | undefined): Tone | null {
  switch (status) {
    case 'exact':
      return 'green'
    case 'mapped':
      return 'blue'
    case 'drifted':
      return 'yellow'
    case 'orphan':
      return 'orange'
    default:
      return null
  }
}

/** A tone-carrying span that renders its text VERBATIM, and renders it in the
 *  neutral treatment when no tone is claimed. */
function ToneSpan({
  tone,
  className,
  children,
  ...rest
}: { tone: Tone | null; className?: string; children: React.ReactNode } & Record<string, unknown>) {
  if (tone === null) {
    return (
      <span
        className={cn(
          'inline-flex items-center gap-1.5 rounded-(--radius-sm) border border-border px-1.5 py-0.5 text-xs text-muted-foreground',
          className,
        )}
        {...rest}
      >
        {children}
      </span>
    )
  }
  return (
    <span style={toneStyle(tone)} className={cn(toneToken, className)} {...rest}>
      {children}
    </span>
  )
}

export function Deliverable({ id, me, stream }: { id: string; me: string; stream?: EventStream }) {
  const { data, error, stale, reload } = useLive<DeliverableDetail>({
    key: `/api/deliverables/${id}`,
    read: () => api.deliverable(id),
    types: deliverableEventTypes,
    stream,
  })

  return (
    <section className="surface deliverable">
      {/* R11's line. What it may NOT say is as pinned as what it says: approval
          state is served PER ARTIFACT (§38 ruling (a)), so this page never
          calls anything approved or signed off, and it never offers acceptance
          as something it can take back — no un-accept verb exists at any shape.
          The one accept the SPA has is the owner's own outward push. */}
      <SurfaceHead
        title={data ? data.deliverable.id : 'Deliverable'}
        what="See the finished work first, download it, and accept it when it is right. Below it: every revision the platform minted, compare any round against any other, comment on a line or on the whole file, and try the built thing live. Accepting makes a revision the official version — a commit pushed under your own credentials — and nothing on this page takes an accept back."
      />
      <Freshness stale={stale} error={error} hasData={data !== null} />
      {data && (
        <>
          <p className={cn('mt-0 mb-3 text-xs text-muted-foreground', figure)} data-provenance="deliverable">
            <Owner id={data.deliverable.owner} /> · {data.deliverable.type} ·{' '}
            <ToneSpan
              // The state vocabulary is the SERVER's and this build owns no map
              // of it, so the token claims no hue: a state coloured by a guess
              // would be a status colour disagreeing with its own word (§51 D5b).
              tone={null}
              className="dlv-state text-[11px] tracking-wide uppercase"
              data-state={data.deliverable.state}
            >
              {data.deliverable.state}
            </ToneSpan>{' '}
            · <Link to={hrefFor('task', { id: data.deliverable.task_id })}>{data.deliverable.task_id}</Link>
            {data.deliverable.project_id ? <> · {data.deliverable.project_id}</> : null}
            {data.deliverable.subject_ref ? <> · {data.deliverable.subject_ref}</> : null}
          </p>
          <StateStory detail={data} me={me} />
          <ResultBlock detail={data} />
          <RevisionsBlock detail={data} stale={stale} />
          <ComparisonBlock detail={detailRefs(data)} me={me} stream={stream} />
          <DoorsBlock detail={data} reload={reload} />
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

// ── RA-B1: the owner's moment — the result, the download, the honest state ───

/**
 * StateStory says where this work STANDS, in the owner's language (RA-8/RA-B1
 * item 4). Every sentence is derivable from served facts alone:
 *
 *  - "in-review" work waits on exactly one person — its OWNER. That is D10
 *    (decisions belong to the decision owner) rendered as a sentence, not a
 *    guess: the wire serves no separate reviewer because there is none.
 *  - the next step is on THIS page (accept below, or ask for changes), so the
 *    story's door is a jump, not an IOU.
 *
 * A state this build does not know renders verbatim with no story claimed —
 * the same forward tolerance every vocabulary map on this surface takes.
 */
function StateStory({ detail, me }: { detail: DeliverableDetail; me: string }) {
  const d = detail.deliverable
  const mine = me !== '' && me === d.owner
  const jump = (selector: string) => {
    document.querySelector(selector)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
  if (d.state === 'in-review') {
    return (
      <p className={cn('state-story mb-3 text-sm', reviewPanel)} data-state-story="in-review">
        {mine ? (
          <>
            <strong>This finished work is waiting for you.</strong> Nobody else reviews it — it is yours to judge. When
            it is right,{' '}
            <button type="button" className="jump-link" data-jump="accept" onClick={() => jump('.accept-card, .deliverable [data-door="accept"]')}>
              accept it below
            </button>{' '}
            and it becomes the official version. If something is off,{' '}
            <button type="button" className="jump-link" data-jump="comments" onClick={() => jump('.comments')}>
              say what to change
            </button>{' '}
            and the platform runs another round.
          </>
        ) : (
          <>
            <strong>This finished work is waiting for {d.owner}&apos;s review</strong> — it is theirs to judge, nobody
            else&apos;s. They accept it to make it the official version, or ask for changes and the platform runs
            another round.
          </>
        )}
      </p>
    )
  }
  if (d.state === 'accepted') {
    return (
      <p className={cn('state-story mb-3 text-sm', reviewPanel)} data-state-story="accepted">
        <strong>This work was accepted</strong> — revision {String(d.current_revision)} is the official version.
      </p>
    )
  }
  if (d.state === 'superseded') {
    return (
      <p className={cn('state-story mb-3 text-sm', reviewPanel)} data-state-story="superseded">
        <strong>A newer accepted version replaced this work</strong> — everything here stays readable as the record it
        is.
      </p>
    )
  }
  return null
}

/**
 * THE SANDBOX OF THE RENDERED-DOCUMENT FRAME: nothing. No scripts, no forms,
 * no same-origin, no popups, no navigation — the empty attribute applies every
 * restriction the platform has.
 *
 * This frame is the "sandboxed rendered-document view" S13.3 sanctions by name
 * (review.EscapeFirst()): the one place a deliverable's own markup renders as
 * the document it is. It is strictly TIGHTER than the preview frame above it —
 * a preview is a running app and needs its scripts; a rendered document is a
 * still image of the work and needs nothing at all. The escape scan pins this
 * grant list empty by name.
 */
export const documentSandbox = ''

/**
 * ResultBlock — the result, FIRST (RA-B1 item 1).
 *
 * The re-walk's blocking finding: the owner's journey ended at a raw diff
 * behind dev banners, with no way to see, download or accept the website she
 * ordered. This block is the landing view now: the current revision's content,
 * rendered as the thing it is — a page as a page, a document as a document,
 * an image as an image — with the source one click away and the bytes
 * downloadable. The diff stays below as the comparison it always was.
 *
 * WHAT MAY RENDER WHERE is the whole design:
 *  - composed DOCUMENTS (a shipped page, rendered markdown) go through the
 *    sandboxed rendered-document frame ONLY — by blob: URL reference, never by
 *    the inline-document attribute the escape scan bans, sandbox empty (see
 *    documentSandbox above);
 *  - plain text renders as escaped text, no frame;
 *  - binary objects render their metadata cards and download doors — the
 *    bytes route decides what may show inline (its closed image allowlist),
 *    and this block learns the answer from the response, never from a
 *    duplicated rule.
 */
function ResultBlock({ detail }: { detail: DeliverableDetail }) {
  const d = detail.deliverable
  const rev = detail.revisions.find((r) => r.n === d.current_revision)
  const refs = rev?.objects ?? []
  const textRefs = refs.filter((r) => r.type === 'text')
  const totalTextBytes = textRefs.reduce((sum, r) => sum + r.size, 0)
  const overCap = totalTextBytes > resultSizeCap

  const [files, setFiles] = useState<ResultFile[] | null>(null)
  const [failure, setFailure] = useState('')
  const [mode, setMode] = useState<'result' | 'source'>('result')

  // The bytes are content-addressed and immutable, so this read keys on the
  // revision: a new revision re-reads, anything else never does.
  useEffect(() => {
    if (rev === undefined || textRefs.length === 0 || overCap) return
    let live = true
    setFailure('')
    Promise.all(
      textRefs.map((ref) => objectText(d.id, ref.sha256).then((text): ResultFile => ({ name: ref.name, text }))),
    ).then(
      (all) => {
        if (live) setFiles(all)
      },
      (err: unknown) => {
        if (live) setFailure(err instanceof Error ? err.message : String(err))
      },
    )
    return () => {
      live = false
    }
    // Keyed by identity + revision on purpose: `textRefs` derives from exactly
    // those two, and the bytes behind a sha can never change (content-addressed).
  }, [d.id, rev?.n])

  if (rev === undefined) {
    return (
      <Section title="The work">
        <EmptyState
          what="Nothing is made yet, so there is nothing to show."
          why="The result lands here the moment a revision is minted — rendered as the thing it is, with its download beside it."
        />
      </Section>
    )
  }

  const composed = files !== null ? composeResult(files, d.type) : null

  return (
    <Section title="The work">
      <div className="result-head mb-2 flex flex-wrap items-center gap-2" data-result-revision={String(rev.n)}>
        <span className="text-sm text-muted-foreground">
          Revision {String(rev.n)}
          {d.state === 'in-review' ? ' — the newest, not yet accepted' : ''}
        </span>
        {textRefs.length > 0 && files !== null && (
          <span className="result-modes ms-auto flex items-center gap-2">
            {(['result', 'source'] as const).map((m) => (
              <label key={m} className={radioRow}>
                <input
                  className={radioInput}
                  type="radio"
                  name="result-mode"
                  data-result-mode={m}
                  checked={mode === m}
                  onChange={() => setMode(m)}
                />
                {m === 'result' ? 'The result' : 'The source'}
              </label>
            ))}
          </span>
        )}
      </div>

      {failure !== '' && (
        <p className="error m-0 text-sm" data-result-failed="true">
          The result could not be read: {failure}. The download below still serves the exact bytes.
        </p>
      )}
      {overCap && (
        <p className="m-0 text-sm text-muted-foreground" data-result-overcap="true">
          This revision&apos;s text runs {humanBytes(totalTextBytes)} — too large to render here. The download below
          serves the exact bytes.
        </p>
      )}

      {composed !== null && mode === 'result' && <ComposedView composed={composed} />}
      {files !== null && mode === 'source' && (
        <div className="result-source flex flex-col gap-2" data-result-source="true">
          {files.map((f) => (
            <div key={f.name}>
              <p className={cn('mt-0 mb-1 text-xs text-muted-foreground', figure)}>{f.name}</p>
              <pre className="suggested rounded-(--radius-sm) border border-border bg-background p-2 text-xs">
                {f.text}
              </pre>
            </div>
          ))}
        </div>
      )}
      {textRefs.length > 0 && files === null && failure === '' && !overCap && (
        <p className="m-0 text-sm text-muted-foreground" data-result-loading="true">
          Reading the work…
        </p>
      )}

      {/* An image deliverable's newest revision IS the result: the bytes route
          serves raster images inline (its own closed allowlist decides), and
          the comparison trio below stays the place two revisions meet. */}
      {refs
        .filter((r) => r.type !== 'text' && (r.type ?? '').startsWith('image/'))
        .map((r) => (
          <figure className="image-side m-0" key={r.sha256} data-result-image={r.sha256}>
            <img src={objectHref(d.id, r.sha256)} alt={`revision ${String(rev.n)}: ${r.name}`} />
            <figcaption className="mt-1 text-xs text-muted-foreground">{r.name}</figcaption>
          </figure>
        ))}

      <DownloadDoor deliverable={d.id} refs={refs} />
    </Section>
  )
}

/** The composed result: a document in the sandboxed frame, or plain text. */
function ComposedView({ composed }: { composed: ComposedResult }) {
  const [src, setSrc] = useState('')

  // The blob URL is minted from the composed document and revoked when the
  // composition changes or the block unmounts. `src` by URL reference is the
  // sanctioned mechanism; the inline-document attribute is the banned one
  // (escape scan, by token).
  useEffect(() => {
    if (composed.kind !== 'document') return
    const url = URL.createObjectURL(new Blob([composed.html], { type: 'text/html' }))
    setSrc(url)
    return () => {
      URL.revokeObjectURL(url)
      setSrc('')
    }
  }, [composed])

  if (composed.kind === 'text') {
    return (
      <pre
        className="result-text suggested rounded-(--radius-sm) border border-border bg-background p-2 text-sm"
        data-result-kind="text"
      >
        {composed.text}
      </pre>
    )
  }
  return (
    <div className="result-frame" data-result-kind="document">
      {src !== '' && (
        <iframe
          title="The finished work, rendered"
          src={src}
          sandbox={documentSandbox}
          referrerPolicy="no-referrer"
          className="h-[34rem] w-full rounded-(--radius) border border-border bg-white"
          data-result-doc="true"
        />
      )}
      <p className="mt-1 mb-0 text-xs text-muted-foreground">
        Shown: {composed.how}. This is a still view — buttons and links inside it are switched off here; the source is
        one click up.
      </p>
    </div>
  )
}

/**
 * DownloadDoor — the bytes, fetchable (RA-B1 item 2).
 *
 * The wire already serves every pinned object of every revision
 * (GET /api/deliverables/{id}/objects/{sha}, content-addressed, owner-scoped,
 * attachment-lane for everything but raster images) — the walk's finding was
 * that no control on the CURRENT revision reached it. These are plain download
 * links to that route, named and sized.
 */
function DownloadDoor({ deliverable, refs }: { deliverable: string; refs: ObjectRef[] }) {
  if (refs.length === 0) return null
  return (
    <p className="result-downloads mt-2 mb-0 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm" data-result-downloads={String(refs.length)}>
      <span className="text-muted-foreground">Take it with you:</span>
      {refs.map((r) => (
        <a
          key={r.sha256}
          href={objectHref(deliverable, r.sha256)}
          download={r.name.split('/').pop() ?? r.name}
          data-action="download-result"
          data-sha={r.sha256}
        >
          Download {r.name.split('/').pop() ?? r.name}{' '}
          <span className={cn('text-xs text-muted-foreground', figure)}>({humanBytes(r.size)})</span>
        </a>
      ))}
    </p>
  )
}

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
        <EmptyState
          what="No revision is minted yet, so there is nothing to review."
          why="A revision appears here when a run mints one — the platform records its content pin, the run and attempt that produced it, and the verification verdict, and each one stays for good."
        />
      ) : (
        // `.revisions` KEEPS its rule, and this element gains NO layout utility.
        // Two reasons, both structural: `TaskDetail.tsx:841` renders the same
        // class and that file is byte-frozen here (so the rule cannot follow the
        // markup), and the ordered-list marker this rule indents is the numbering
        // itself — a `flex` utility would silently drop it on one consumer and
        // keep it on the other. §53's brief expected the markup move to free the
        // rule; the grep says a second consumer holds it.
        <ol className="revisions">
          {detail.revisions.map((r) => (
            <li key={r.n} data-revision={String(r.n)} className="wrap-anywhere text-sm">
              <span className={cn('rev-n font-semibold', figure)}>revision {String(r.n)}</span>{' '}
              <span className={cn('text-xs text-muted-foreground', figure)}>
                {r.pin_kind} {revisionPin(r) === '' ? <Absent reason="no pin recorded" /> : revisionPin(r)}
              </span>
              <span className={cn('text-xs text-muted-foreground', figure)}>
                {' '}
                · minted by {r.run_id === undefined || r.run_id === '' ? <Absent reason="no minting run" /> : r.run_id}
              </span>
              <span className={cn('text-xs text-muted-foreground', figure)}>
                {' '}
                ·{' '}
                {r.verdict_ref === undefined || r.verdict_ref === 0 ? (
                  <Absent reason="no verification verdict recorded" />
                ) : (
                  // W2-8: a bare "verdict #N" was a citation with no door. The
                  // verdict's numbered findings land in this page's own
                  // comments block, so the citation walks you there.
                  <button
                    type="button"
                    className="verdict-link"
                    data-verdict-ref={String(r.verdict_ref)}
                    title="The verdict's findings land in the comments block on this page"
                    onClick={() => {
                      document.querySelector('.comments')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
                    }}
                  >
                    verdict #{String(r.verdict_ref)} — findings below
                  </button>
                )}
              </span>
              <span className={cn('text-xs text-muted-foreground', figure)}>
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
    return (
      <p className="mb-0 text-xs text-muted-foreground">
        No follow-up lineage: this deliverable neither followed from one nor spawned a task.
      </p>
    )
  }
  // `.lineage` KEEPS its rule for the same reason `.revisions` does —
  // `TaskDetail.tsx:739` is the second consumer and is byte-frozen — so no
  // margin utility competes with its `margin-block` here.
  return (
    <div className="lineage">
      {lin.succeeds.map((s) => (
        <p key={`from-${s.deliverable_id}-${s.revision_n}`} data-lineage="succeeds" className="my-1 text-sm">
          Follows from{' '}
          <Link to={hrefFor('deliverable', { id: s.deliverable_id })} className={figure}>
            {s.deliverable_id} r{String(s.revision_n)}
          </Link>
        </p>
      ))}
      {lin.succeeded_by.map((s) => (
        <p key={`to-${s.task_id}`} data-lineage="succeeded-by" className="my-1 text-sm">
          Followed up by{' '}
          <Link to={hrefFor('task', { id: s.task_id })} className={figure}>
            {s.task_id}
          </Link>
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
  // `.rev-picker` KEEPS its rule (flex column at phone width, row inside the one
  // breakpoint), so this element takes no display/flex/gap/margin utility: an
  // owned `@media` arm in `@layer base` LOSES to a competing utility, and that is
  // the `.chat-verbs` regression class no jsdom test in the tree can see (§53).
  return (
    <div className="rev-picker">
      <p className="m-0 text-sm" data-compared={served ? `${served.old_n}:${served.new_n}` : ''}>
        {served ? (
          <>
            Comparing{' '}
            <strong>{served.old_n === 0 ? 'the project’s pre-task base' : `revision ${String(served.old_n)}`}</strong> with{' '}
            <strong>revision {String(served.new_n)}</strong>
            {pair.new === undefined && pair.old === undefined && (
              <span className="text-muted-foreground"> — the platform&apos;s own round-over-round default</span>
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
        <Button variant="ghost" size="sm" data-action="round-over-round" onClick={() => onPick({})}>
          Back to round-over-round
        </Button>
      )}
    </div>
  )
}

/** The served label, shown wherever the platform gave one. A fallback or a degrade
 *  NEVER poses as a rich surface: the label is the honesty, so it renders beside
 *  the surface rather than being folded into it. */
function SurfaceLabel({ cmp }: { cmp: Comparison }) {
  if (cmp.label === undefined || cmp.label === '') return null
  // The two shared blocks `.warn-flag` and `.notice` KEEP their rules — their
  // consumers span the tree, most on byte-frozen surfaces (the §53 stay-notes
  // carry the measured lists) — so the hue stays theirs and this adds only rhythm.
  return (
    <p className={cn(cmp.fallback === true ? 'warn-flag' : 'notice', 'my-2 text-sm')} data-surface-label={cmp.surface}>
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
          <div className={cn('surface-unknown', reviewPanel)} data-surface={cmp.surface}>
            <Absent
              reason={`no rich comparison surface for the answer "${cmp.surface}" — this build does not know how to draw it, so here is what the platform served`}
            />
            <dl
              className={cn(
                'served-fields mt-2 mb-0 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-1 text-xs',
                '[&_dd]:m-0 [&_dd]:wrap-anywhere [&_dt]:text-muted-foreground',
                figure,
              )}
            >
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
      {/* `.diff-controls` KEEPS its rule — flex column, row at the breakpoint —
          so the toggle row takes no layout utility. The radios themselves are
          native and utility-less on purpose: they keep the owned form chrome,
          which is the negative leg the cascade record measured (§53 A1). */}
      <div className="diff-controls">
        {(['split', 'unified'] as const).map((v) => (
          <label key={v} className={radioRow}>
            <input
              className={radioInput}
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
        // Deliberately `Empty`-grade rather than a teaching state: this is a
        // MID-FORM absence with the reason already in the sentence, and R12 says
        // migrate only where a teaching `why` is true and useful.
        <Empty what="The platform served no diff text for this pair — the two revisions have no differing file." />
      ) : (
        (() => {
          // THE PRE-TASK-BASE COMPARE IS A CROSS-GRAIN COMPARE (review #1):
          // the base side is the whole project as it stood, while a content
          // revision records only the files the work shipped. The served diff
          // unions the two, so every base file the revision does not carry
          // arrived rendered as a DELETION — a false story about the work
          // (the walk's Makefile "deleted" by a task that only added a note).
          // The revision's own fileset is exactly the files with a NEW side,
          // so the base-only files are separable here: they render collapsed
          // under the truth ("not carried, no change recorded"), never as
          // deletions the work made. Same-grain compares (revision vs
          // revision) keep deletions as the real deletions they are.
          const crossGrain = cmp.old_n === 0
          // A base-only file arrives as `delete` — or, because the server
          // diffs file-vs-empty with --no-index, as a `modify` whose every
          // line is a deletion and whose new side is empty. Both shapes mean
          // the same fact here: the revision records nothing about this file.
          const notCarried = ({ file }: RenderedFile) =>
            file.type === 'delete' ||
            (file.hunks.length > 0 && file.hunks.every((h) => h.changes.every((c) => c.type === 'delete')))
          const own = crossGrain ? files.filter((f) => !notCarried(f)) : files
          const baseOnly = crossGrain ? files.filter(notCarried) : []
          const renderFile = ({ file, path }: RenderedFile) => (
            <DiffFile
              key={path}
              file={file}
              path={path}
              viewType={viewType}
              comments={placed.data?.comments ?? []}
              placements={placed.data?.placements ?? []}
              onSelect={setClaim}
            />
          )
          return (
            <>
              {crossGrain && (
                <p className="diff-scope my-2 text-xs text-muted-foreground" data-diff-scope="pre-task-base">
                  Two different scopes meet here: the pre-task base is the WHOLE project as it stood, while revision{' '}
                  {String(cmp.new_n)} records only the {own.length === 1 ? 'file' : `${String(own.length)} files`} the
                  work shipped. Only those files carry changes below.
                </p>
              )}
              {own.map(renderFile)}
              {baseOnly.length > 0 && (
                <details className="diff-basefiles my-2" data-base-only={String(baseOnly.length)}>
                  <summary className="text-xs text-muted-foreground">
                    {baseOnly.length === 1 ? '1 project file' : `${String(baseOnly.length)} project files`} from the
                    pre-task base that revision {String(cmp.new_n)} does not carry — the revision records no change to
                    {baseOnly.length === 1 ? ' it' : ' them'}, and nothing here was deleted by the work. Open to see
                    {baseOnly.length === 1 ? ' it' : ' them'} as the base held {baseOnly.length === 1 ? 'it' : 'them'}.
                  </summary>
                  {baseOnly.map(renderFile)}
                </details>
              )}
            </>
          )
        })()
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
    <div className={cn('diff-unreadable', reviewPanel)} data-diff-unreadable="true">
      <Absent
        reason={`this diff could not be read as a unified diff (${failure}), so it cannot be drawn as one — the text the platform served is below, unchanged`}
      />
      <pre className="diff-raw mt-2 mb-0 overflow-x-auto text-xs">{unified}</pre>
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
    // `.diff-file` KEEPS its rule whole (`margin` + `overflow-x: auto`, the one
    // property `responsive.test.ts:233` is written against), so no margin or
    // overflow utility competes with it here.
    <div className={cn('diff-file', 'rounded-(--radius) border border-border')} data-file={path}>
      <h4 className={cn('diff-file-head my-1 wrap-anywhere px-2 pt-2 text-sm', figure)}>
        {path} <span className="text-xs text-muted-foreground">{file.type}</span>
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
      <div
        className="diff-widget border-s-[3px] border-current px-2 py-1.5"
        data-widget-key={key}
      >
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
  //
  // AND THE LIST IS THE COMPLEMENT'S COMPLEMENT (review #2): it holds only the
  // comments the strip does not. When the list held EVERYTHING, each unplaced
  // comment rendered twice on one screen — list and strip — which doubled into
  // ×4 the day the platform served a duplicated finding. Every served comment
  // still renders exactly once down here, plus its widget inside the diff when
  // it has a live position.
  // With no diff on screen there is no "position" to lack, so the strip would
  // be a distinction without a difference: the object, image and unknown
  // surfaces list every comment plainly and render no strip at all.
  const hasDiff = files.length > 0
  const unanchored = hasDiff ? comments.filter((c) => widgetReach(files, byID.get(c.id)) === null) : []
  const placed = hasDiff ? comments.filter((c) => widgetReach(files, byID.get(c.id)) !== null) : comments

  return (
    <div className={cn('comments', reviewPanel, 'mt-3')} data-revision={String(revision)}>
      <Freshness stale={feed.stale} error={feed.error} hasData={feed.data !== null} />
      <h4 className="mt-0 mb-2">Comments on revision {String(revision)}</h4>
      {/* SERVED-GATED (§51 drain D1). `comments` is `feed.data?.comments ?? []`,
          so before this gate a read still in flight rendered as a served empty
          and taught "nobody has commented" about a question the platform had not
          answered. `feed.data !== null` is the answered state; `Freshness` owns
          the window before it. The same gate covers the strip below, whose
          sentence ("every comment has a live position") would otherwise be a
          statement about an empty list nobody had been sent yet. */}
      {feed.data !== null &&
        (comments.length === 0 ? (
          <EmptyState
            what="Nobody has commented on this revision yet."
            why="Selecting a line in the diff anchors a comment to it; leaving the anchor off files it against the deliverable. A verification run's findings land here too, and a blocker is what makes the platform run another round."
          />
        ) : placed.length > 0 ? (
          <ul className="comment-list m-0 flex list-none flex-col gap-2 ps-0">
            {placed.map((c) => (
              <li key={c.id}>
                <CommentCard comment={c} placement={byID.get(c.id)} />
              </li>
            ))}
          </ul>
        ) : (
          <p className="m-0 text-xs text-muted-foreground">
            {comments.length === 1
              ? 'One comment exists on this revision; it has no live position in the view above, so it is in the strip below.'
              : `${String(comments.length)} comments exist on this revision; none has a live position in the view above, so they are in the strip below.`}
          </p>
        ))}

      {hasDiff && (
      <div className="synthetic-strip my-2 border-s-[3px] border-current px-2 py-1.5" data-strip="unanchored">
        <h5 className="mt-0 mb-1">Without a place in this view</h5>
        {feed.data !== null &&
          (unanchored.length === 0 ? (
            // Deliberately `Empty`-grade: this is a mid-form absence and, unlike
            // an empty list, it is a POSITIVE statement about the comments that
            // do exist — there is no "what would make a row appear here" that is
            // useful to teach.
            <Empty what="Every comment on this revision has a live position in the diff above." />
          ) : (
            <ul className="m-0 flex list-none flex-col gap-2 ps-0">
              {unanchored.map((c) => (
                <li key={c.id} data-strip-comment={String(c.id)}>
                  <CommentCard comment={c} placement={byID.get(c.id)} />
                </li>
              ))}
            </ul>
          ))}
      </div>
      )}

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
      className="comment py-1.5"
      data-comment={String(c.id)}
      data-status={c.status}
      data-kind={c.kind}
      data-placement={placement?.status ?? 'unplaced'}
    >
      <p className="comment-head m-0 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
        {/* RA-5: a machine finding is NEVER signed as a person. The drain's
            findings rows carry the requester's identity as their author — a
            REPORTED wire gap — so the view is where the honest attribution
            lives: kind "finding" is the platform's own checker speaking, and
            it says so instead of wearing the owner's name. */}
        {c.kind === 'finding' ? (
          <span
            className="checker font-semibold"
            data-author="platform-checker"
            title="An automated verification finding — written by the platform's checker, not by a person."
          >
            the platform&apos;s checker
          </span>
        ) : (
          <Owner id={c.owner} />
        )}{' '}
        <ToneSpan tone={severityTone(c.severity)} className="severity" data-severity={c.severity}>
          {severityMeaning(c.severity)}
        </ToneSpan>{' '}
        <span className={cn('text-muted-foreground', figure)}>
          {c.kind} · said about revision {String(c.revision_n)} · <Stamp ts={c.created_ts} />
        </span>
      </p>
      {/* `.comment-body` KEEPS its rule (`overflow-wrap: anywhere`, the LAST-but-one
          selector of the group `responsive.test.ts:245` reads through `.signing`),
          so no wrap utility competes with it. */}
      <p className="comment-body my-1 text-sm">{c.body}</p>
      {c.suggested_change !== undefined && c.suggested_change !== '' && (
        // `.suggested` KEEPS its rule (`overflow-x` + `white-space`, the group
        // `responsive.test.ts:242` reads through `.merge-card pre`).
        <pre className="suggested rounded-(--radius-sm) border border-border bg-background p-2 text-xs">
          {c.suggested_change}
        </pre>
      )}
      {(c.category !== undefined && c.category !== '') || (c.criterion !== undefined && c.criterion !== '') ? (
        <p className={cn('finding-meta m-0 text-xs text-muted-foreground', figure)}>
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
    // `.placement` KEEPS its rule (`overflow-wrap`, same pinned group), so no
    // wrap utility competes; the same holds for `.quote code` below.
    <p className="placement my-1 text-xs">
      <ToneSpan
        tone={placementTone(placement?.status)}
        className="placement-status"
        data-status={placement?.status ?? 'unplaced'}
      >
        {placementMeaning(placement?.status)}
      </ToneSpan>
      {live !== undefined && live.line_no !== 0 ? (
        <span className={cn('text-muted-foreground', figure)}>
          {' '}
          · now at {live.file_path}:{String(live.line_no)} ({live.side} side)
        </span>
      ) : null}
      {live !== undefined && live.line_no === 0 && live.file_path !== '' ? (
        <span className={cn('text-muted-foreground', figure)}> · on {live.file_path}</span>
      ) : null}
      {claimed !== undefined && claimed.line_text !== '' ? (
        <span className={cn('quote text-muted-foreground', figure)}>
          {' '}
          · quoted <code>{claimed.line_text}</code> at {claimed.file_path}:{String(claimed.line_no)}
        </span>
      ) : null}
      {comment.origin_anchor !== undefined && comment.origin_anchor !== '' ? (
        <span className={cn('text-muted-foreground', figure)}> · as supplied: {comment.origin_anchor}</span>
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
  // Open and consumed are the two ends of the S13.4 drain, and they take the two
  // tones that say so: an open comment is still owed a round, a consumed one has
  // had its round and carries the [F#] the attempt received it as.
  if (comment.status !== 'consumed') {
    return (
      <p className="lifecycle my-1 text-xs" data-lifecycle="open">
        <ToneSpan tone="blue">Open — the next round will drain this.</ToneSpan>
      </p>
    )
  }
  return (
    <p className="lifecycle my-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs" data-lifecycle="consumed">
      <ToneSpan tone="green" className={cn('finding-number font-semibold', figure)}>
        [F{comment.finding_number === undefined ? '?' : String(comment.finding_number)}]
      </ToneSpan>{' '}
      <span className={cn('text-muted-foreground', figure)}>
        consumed <Stamp ts={comment.consumed_at} /> by{' '}
        {comment.consumed_by === undefined || comment.consumed_by === '' ? (
          <Absent reason="no consuming attempt recorded" />
        ) : (
          <span className="attempt-ref font-semibold">{comment.consumed_by}</span>
        )}
      </span>
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
      <h5 className="m-0">Add a comment as {me === '' ? 'yourself' : me}</h5>
      <p
        className="m-0 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"
        data-claim={anchored === null ? '' : `${anchored.file_path}:${anchored.line_no}`}
      >
        {anchored === null ? (
          <>Select a line in the diff to anchor this, or leave it file-level.</>
        ) : (
          <>
            <span className={figure}>
              Anchored at {anchored.file_path}:{String(anchored.line_no)} ({anchored.side} side), quoting{' '}
              <code>{anchored.line_text}</code>
            </span>{' '}
            <Button variant="ghost" size="sm" data-action="clear-anchor" onClick={() => onClearAnchor?.()}>
              Drop the anchor
            </Button>
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
        <textarea
          className={reviewTextarea}
          data-field="body"
          value={body}
          onChange={(e) => setBody(e.target.value)}
        />
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
        <textarea
          className={reviewTextarea}
          data-field="suggested"
          value={suggested}
          onChange={(e) => setSuggested(e.target.value)}
        />
      </label>
      <Button variant="primary" size="sm" type="submit" data-action="add-comment" disabled={body.trim() === ''}>
        Add comment
      </Button>
      {failure !== '' && <p className="error m-0 text-sm">{failure}</p>}
      {born !== null && (
        <p className="notice m-0 text-sm" data-born-status={born.anchor_status}>
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
    <div className={cn('image-pair', reviewPanel)} data-mode={mode}>
      <ChangedVerdict cmp={cmp} />
      {/* `.image-modes` and `.two-up` both KEEP their rules — each is read by
          `responsive.test.ts:251/:255` and each has a live wide-screen arm — so
          neither takes a layout utility. */}
      <div className="image-modes">
        {(['2-up', 'swipe', 'onion'] as const).map((m) => (
          <label key={m} className={radioRow}>
            <input
              className={radioInput}
              type="radio"
              name="image-mode"
              data-image-mode={m}
              checked={mode === m}
              onChange={() => setMode(m)}
            />
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
          <div className="swipe-stack relative">
            <ImageSide label={sideLabel('new', cmp.new_n)} deliverable={cmp.deliverable_id} ref_={newRef} />
            <div className="swipe-top absolute inset-y-0 start-0 end-auto overflow-hidden" style={{ width: `${swipeAt}%` }}>
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
          <div className="onion-stack relative">
            <ImageSide label={sideLabel('old', cmp.old_n)} deliverable={cmp.deliverable_id} ref_={oldRef} />
            <div className="onion-top absolute inset-0 overflow-hidden" style={{ opacity: blend }}>
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
  // `.image-side` sheds its `margin: 0` to a utility; `.image-side img` KEEPS its
  // own rule, so the <img> below carries no sizing utility.
  if (ref_ === undefined) {
    return (
      <figure className="image-side m-0" data-image-side="absent">
        <Absent reason={`${label}: this side pins no object`} />
      </figure>
    )
  }
  return (
    <figure className="image-side m-0" data-image-side={ref_.sha256}>
      {failed ? (
        <Absent
          reason={`${label}: the platform does not serve ${ref_.type === undefined || ref_.type === '' ? 'this type' : ref_.type} inline, so it cannot be drawn here — download the object to inspect it`}
        />
      ) : (
        <img src={objectHref(deliverable, ref_.sha256)} alt={`${label}: ${ref_.name}`} onError={() => setFailed(true)} />
      )}
      <figcaption className="mt-1 text-xs text-muted-foreground">{label}</figcaption>
    </figure>
  )
}

/** The optional server-side aid. Absent at v0 — the seam exists, and saying
 *  nothing is the truthful render when no producer filled it. */
function PixelDiffAid({ cmp }: { cmp: Comparison }) {
  if (cmp.pixel_diff === undefined) {
    return <p className="my-2 text-xs text-muted-foreground">No pixel-diff aid was computed for this pair.</p>
  }
  return (
    <p className={cn('pixel-diff my-2 text-xs', figure)} data-pixel-diff="served">
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
    // The hash verdict is a FACT rather than a status: "they differ" is neither
    // good nor bad on a review surface, so it takes no hue and reads as prose.
    <p className="changed-verdict my-1 text-sm" data-changed={cmp.changed === true ? 'true' : 'false'}>
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
    <div className={cn('object-cards mt-2 flex flex-col gap-2', reviewPanel)}>
      <ChangedVerdict cmp={cmp} />
      {sides.map((side) => (
        <div className="object-side" key={side.label} data-object-side={side.label}>
          <h5 className="mt-0 mb-1">{side.label}</h5>
          {side.refs.length === 0 ? (
            <Absent reason="this side pins no object" />
          ) : (
            <ul className="m-0 flex list-none flex-col gap-2 ps-0">
              {side.refs.map((ref) => (
                <li key={ref.sha256} data-object={ref.sha256} className="text-sm">
                  <span className={cn('object-name font-semibold', figure)}>{ref.name}</span>{' '}
                  <span className={cn('text-xs text-muted-foreground', figure)}>
                    {String(ref.size)} bytes ·{' '}
                    {ref.type === undefined || ref.type === '' ? <Absent reason="no type recorded" /> : ref.type}
                  </span>
                  <br />
                  {/* `.deliverable .object-sha` KEEPS its rule — it is the LAST
                      selector of the group `responsive.test.ts:239` reads — so
                      the hash takes no wrap utility of its own. */}
                  <code className="object-sha text-xs">{ref.sha256}</code>{' '}
                  <a
                    href={objectHref(cmp.deliverable_id, ref.sha256)}
                    data-action="download-object"
                    download={ref.name}
                    className="text-xs"
                  >
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
function DoorsBlock({ detail, reload }: { detail: DeliverableDetail; reload: () => void }) {
  return (
    <Section title="What you can do">
      {detail.doors.length === 0 ? (
        <EmptyState
          what="The platform named no doors on this deliverable."
          why="A door appears here when the platform will actually let you through it — asking for a revision, spawning a follow-up, launching a preview, accepting. What is open depends on the deliverable's state and on whose work it is, so this list changes as the work moves."
        />
      ) : (
        <ul className="doors m-0 flex list-none flex-col gap-3 ps-0">
          {detail.doors.map((door) => (
            <li
              key={door.verb}
              data-door={door.verb}
              data-available={door.available ? 'true' : 'false'}
              className={cn(reviewPanel, 'flex flex-col gap-1')}
            >
              <DoorRow door={door} detail={detail} reload={reload} />
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}

function DoorRow({ door, detail, reload }: { door: Door; detail: DeliverableDetail; reload: () => void }) {
  return (
    <>
      <p className="door-head m-0 flex flex-wrap items-center gap-2">
        <span className={cn('door-verb font-semibold', figure)}>{door.verb}</span>{' '}
        {/* open/closed is a single-WORD status, so it takes the kit `Chip`'s
            status-token treatment — unlike the meaning SENTENCES elsewhere on
            this surface, which stay prose (§53 A2's content-vs-token judgement). */}
        <Chip tone={door.available ? 'green' : 'yellow'}>{door.available ? 'open' : 'closed'}</Chip>
      </p>
      {/* `.door-reason` KEEPS its rule (the `responsive.test.ts:245` group), so
          no wrap utility competes with it. */}
      <p className="door-reason m-0 text-sm">{door.reason}</p>
      {/* W2-6/RA-4: the wire address is REAL and stays on the surface — but as
          a technical fold, not as a sentence. A household reviewer read "POST
          /api/deliverables/…" as radio chatter; an integrator opens one fold. */}
      {door.route !== '' && (
        <details className="door-tech">
          <summary className="cursor-pointer text-xs text-muted-foreground">technical detail</summary>
          <p className={cn('door-route m-0 text-xs text-muted-foreground', figure)}>
            {door.method} {door.route}
            {door.preset !== undefined && door.preset !== '' ? <> · preset {door.preset}</> : null}
            {door.pin_from !== undefined && door.pin_from !== '' ? <> · pin from {door.pin_from}</> : null}
          </p>
        </details>
      )}
      {door.available && driveableRevisionDoor(door) && <RequestRevision door={door} />}
      {driveableFollowUpDoor(door) && <SpawnFollowUp door={door} detail={detail} reload={reload} />}
    </>
  )
}

/**
 * driveableFollowUpDoor answers the same question `driveableRevisionDoor` does,
 * for the other landed verb: can THIS surface perform this door?
 *
 * It is gated on what the door SUPPLIES — an open door, a POST, and a route that
 * really is the S13.9 spawn — rather than on a verb name, which is the rule
 * drain r1 D4 wrote after the opposite mistake shipped a form nothing could
 * send. TWO served doors reach this route and both are honoured: the plain
 * `follow-up` door, which carries no preset, and the `request-revision` door's
 * FINISHED limb, which carries `revision` (internal/api/deliverables.go:264–267
 * and :350). The preset sent is whichever the door named — never one composed
 * here — so the framing on the wire is the platform's own.
 */
function driveableFollowUpDoor(door: Door): boolean {
  return door.available && door.method === 'POST' && door.route.endsWith('/follow-up')
}

/**
 * The S13.9 follow-up spawn — the fourteenth D3 control (P3-UI-4, OQ1(a)).
 *
 * A follow-up is a successor TASK, not an answer to a card: no pin, no ask id,
 * no payload hash, which is exactly why gating it on the `request-revision` verb
 * shipped a dead form before. What it needs is the door's route and the door's
 * preset, and the person's own account of what the successor should do.
 *
 * NO REVISION PICKER, and that is the design. The verb defaults to the CURRENT
 * revision when the body names none (internal/api/actions.go:170–174), which is
 * the honest reading of "follow up on this deliverable", and the answer says
 * which revision it linked to. A picker here would be this surface inventing a
 * choice in order to re-answer a question the platform already answers, and the
 * lineage the successor links to is a fact rather than a preference.
 */
function SpawnFollowUp({
  door,
  detail,
  reload,
}: {
  door: Door
  detail: DeliverableDetail
  reload: () => void
}) {
  const [objective, setObjective] = useState('')
  const [open, setOpen] = useState(false)
  const act = useAct()
  const preset = door.preset ?? ''
  const held = objective.trim() === '' ? 'A follow-up needs its own ask — say what the successor should do.' : ''

  return (
    <div className="follow-up mt-1 flex flex-col items-start gap-2" data-control="follow-up">
      <label className="w-full">
        What should the follow-up do?
        <textarea
          className={reviewTextarea}
          data-field="follow-up-objective"
          value={objective}
          onChange={(e) => setObjective(e.currentTarget.value)}
        />
      </label>
      <Button
        variant="primary"
        size="sm"
        data-open="follow-up"
        data-busy={String(act.busy)}
        disabled={act.busy || held !== ''}
        onClick={() => setOpen(true)}
      >
        Spawn a follow-up
      </Button>
      {held !== '' && (
        <p className="m-0 text-xs text-muted-foreground" data-held="follow-up">
          {held}
        </p>
      )}
      <ActConfirm
        open={open}
        onOpenChange={setOpen}
        title="Spawn a follow-up task"
        variant="primary"
        act="Spawn it"
        busy={act.busy}
        disabled={held !== ''}
        what={
          <>
            This opens a NEW task of your own, linked to this deliverable&apos;s current revision as its predecessor
            {preset !== '' ? <> under the platform&apos;s {preset} framing</> : null}. It changes nothing about this
            deliverable — no revision is replaced, nothing is re-opened, and the accepted work stays accepted. The new
            task enters intake like any other, so it is planned and approved on its own terms before anything runs.
          </>
        }
        onConfirm={() => {
          setOpen(false)
          act.run(
            () =>
              api.spawnFollowUp(detail.deliverable.id, { preset, objective }).then((res) => ({
                kind: 'applied' as const,
                note: (
                  <>
                    opened{' '}
                    <span className="font-mono tabular-nums" data-spawned={res.task_id}>
                      {res.task_id}
                    </span>{' '}
                    for {res.owner}, linked to revision{' '}
                    <span className="font-mono tabular-nums">{String(res.revision_n)}</span>
                  </>
                ),
                detail: res.title,
              })),
            reload,
          )
        }}
      />
      <OutcomeLine outcome={act.outcome} />
      {act.outcome?.kind === 'applied' && (
        <p className="m-0 text-xs text-muted-foreground">
          It is a task now, so it lives on the board and its own detail — this deliverable is unchanged.
        </p>
      )}
    </div>
  )
}

/**
 * driveableRevisionDoor answers whether THIS surface can actually perform the
 * request-revision door — which is not the same question as whether the door is
 * open (B6-9 drain r1, D4).
 *
 * The verb name covers TWO server states. When a rework card is open the door
 * carries the answer route, the card's own pin and its `revise_with_guidance`
 * verb, and the form below drives it. When the deliverable is FINISHED the same
 * verb names the S13.9 follow-up spawn instead: route `/follow-up`, preset
 * `revision`, and deliberately **no pin and no answer** — because a follow-up is
 * a successor TASK, not an answer to a card.
 *
 * Gating the form on the verb rendered it in both states, so the finished limb
 * shipped a full form with an ENABLED submit button whose handler could only
 * ever report "The door named no route, pin or answer" — precisely the dead
 * control this file's own rule at the top forbids. Gating on what the door
 * SUPPLIES makes that rule true: the finished limb still renders its door, its
 * route, its preset and its reason, and simply offers no control this surface
 * cannot honour. The follow-up spawn itself has no control anywhere in the SPA,
 * which is a REPORTED GAP on the S15.12 sweep's own list rather than something
 * to invent here.
 */
function driveableRevisionDoor(door: Door): boolean {
  return (
    door.verb === 'request-revision' &&
    door.route !== '' &&
    door.payload_hash !== undefined &&
    door.payload_hash !== '' &&
    door.answer !== undefined &&
    door.answer !== ''
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
        <textarea
          className={reviewTextarea}
          data-field="guidance"
          value={guidance}
          onChange={(e) => setGuidance(e.target.value)}
        />
      </label>
      {/* Severity is not offered here, and the absence is the point: the platform
          records a guidance point as a BLOCKER itself (guidance always names a
          problem to fix), so a severity control on this form would be this surface
          claiming a choice the answer schema does not have. */}
      <label>
        PIN, if this card asks for one
        <input type="password" data-field="revision-pin" value={pin} onChange={(e) => setPin(e.target.value)} />
      </label>
      <Button variant="primary" size="sm" type="submit" data-action="request-revision" disabled={guidance.trim() === ''}>
        Ask for a revision
      </Button>
      {guidance.trim() === '' && (
        <p className="m-0 text-xs text-muted-foreground">
          A revision request needs at least one point to act on — the card&apos;s own answer schema refuses an empty one,
          so there is nothing to send until you say what to change.
        </p>
      )}
      {stale && (
        <p className="warn-flag m-0 text-sm" data-stale="request-revision">
          The card moved since this page was read, so NOTHING was written — not the guidance, not the answer. Reload the
          deliverable to pick up the card as it now is, then say it again.
        </p>
      )}
      {outcome !== '' && <p className="notice m-0 text-sm">{outcome}</p>}
      {failure !== '' && <p className="error m-0 text-sm">{failure}</p>}
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
        <Button
          variant="secondary"
          size="sm"
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
        </Button>
      ) : (
        <p className="m-0 text-sm text-muted-foreground" data-closed="preview">
          {previewDoor.reason}
        </p>
      )}
      {compareDoor !== undefined && compareDoor.available && (
        <Button
          variant="secondary"
          size="sm"
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
        </Button>
      )}
      {failure !== '' && <p className="error m-0 text-sm">{failure}</p>}
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
    <div
      className={cn('preview-session mt-2', reviewPanel)}
      data-preview-state={session.state}
      data-backed={backed ? 'true' : 'false'}
    >
      <p className={cn('mt-0 mb-1 text-sm', figure)}>
        <span className="preview-lane font-semibold">{session.lane}</span> · revision {String(session.revision)} ·{' '}
        <span className="preview-state font-semibold">{session.state}</span>
        {session.routed ? <span className="text-muted-foreground"> · routed through the front chain</span> : null}
      </p>
      {backed ? (
        <>
          <p className={cn('m-0 wrap-anywhere text-sm', figure)}>
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
    // `.port-picker` KEEPS its rule (column, row at the breakpoint), so no
    // layout utility competes with it.
    <div className="port-picker" data-ports={String(ports.length)}>
      <p className="m-0 text-xs text-muted-foreground">
        This preview listens on more than one port — pick the one you want.
      </p>
      {ports.map((p) => (
        <label key={p.number} className={radioRow}>
          <input
            className={radioInput}
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
    <div className={cn('dual-frames mt-2', reviewPanel)} data-single-instance={pair.single_instance ? 'true' : 'false'}>
      {/* `.frame-path` KEEPS its rule and is read TWICE by `responsive.test.ts`
          (:251 for the phone column, :255 for the wide arm), so this form takes
          no layout utility at all. */}
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
        <Button variant="secondary" size="sm" type="submit" data-action="apply-path">
          Go
        </Button>
        <Button variant="ghost" size="sm" data-action="resync" onClick={resync}>
          Re-sync both sides
        </Button>
      </form>
      <p
        className="my-2 text-xs text-muted-foreground"
        data-sync-mode={pair.sync.mode}
        data-sync-enabled={pair.sync.enabled ? 'true' : 'false'}
      >
        {pair.sync.enabled
          ? 'Synced navigation is driven from here: the path above is applied to both sides together. A click INSIDE a frame moves that frame alone — these previews are separate origins, so this page cannot see where a frame has navigated to. Re-sync puts both back on the path above.'
          : 'Nothing to sync: there is only one instance below.'}
      </p>
      {/* `.frames` KEEPS its rule for the same reason `.frame-path` does. */}
      <div className="frames">
        {pair.before === undefined ? (
          <div className="frame-absent rounded-(--radius-sm) border border-dashed border-current p-2" data-frame="before">
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
      <div
        className="frame-absent rounded-(--radius-sm) border border-dashed border-current p-2"
        data-frame={side.role}
        data-preview-state={side.state}
      >
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
      <p className={cn('mt-0 mb-1 text-xs text-muted-foreground', figure)}>
        {side.role} · revision {String(side.revision)}
      </p>
      {/* `.frame iframe` KEEPS its rule (`responsive.test.ts:263`), so the frame
          itself carries no sizing utility. */}
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
    <div className={cn('live-previews mt-3', reviewPanel)}>
      <h4 className="mt-0 mb-1">Preview sessions running now</h4>
      <Freshness stale={feed.stale} error={feed.error} hasData={feed.data !== null} />
      {/* SERVED-GATED (§51 drain D1), the same class as the comment feed above:
          `sessions` is `feed.data?.sessions ?? []`, so an unanswered `/api/previews`
          rendered as a served empty and taught "no preview session is running"
          before the platform had said anything at all. */}
      {feed.data !== null &&
        (sessions.length === 0 ? (
          <EmptyState
            what="No preview session is running."
            why="Launching a preview above puts it here, with the lane it runs in and whose it is, until you stop it or the platform reclaims the port."
          />
        ) : (
        <ul className="m-0 flex list-none flex-col gap-2 ps-0">
          {sessions.map((s) => (
            <li key={s.id} data-session={s.id} className="flex flex-wrap items-center gap-2 text-sm">
              <span className={cn('text-xs text-muted-foreground', figure)}>
                {s.deliverable} r{String(s.revision)} · {s.state} · {s.lane} · <Owner id={s.user} />
              </span>{' '}
              <Button
                variant="danger"
                size="sm"
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
              </Button>
            </li>
          ))}
        </ul>
        ))}
      {note !== '' && (
        <p className="notice m-0 text-sm" data-stop-detail="served">
          {note}
        </p>
      )}
      {failure !== '' && (
        <p className="error m-0 text-sm" data-stop-failed="true">
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
          {/* The owner's sentence first (RA-B1 item 3): what accepting MEANS,
              in the owner's language. The served door reason stays, as the
              platform's own precondition line beneath it. */}
          <p className="accept-plain mt-0 mb-1 text-sm">
            When this work is right, accepting it makes it <strong>the official version</strong> — filed permanently
            under {mine ? 'your' : `${owner}'s`} name. Nothing on this page takes an accept back; if something needs
            fixing later, that is a new round or a follow-up, never an undo.
          </p>
          <p className="door-reason mt-0 mb-2 text-xs text-muted-foreground">{door.reason}</p>
          <Button variant="secondary" size="sm" data-action="read-accept-card" onClick={read}>
            Accept this work…
          </Button>
        </>
      )}
      {failure !== '' && <p className="error m-0 text-sm">{failure}</p>}
      {card !== null && (
        <>
          {changed && (
            <p className="warn-flag my-2 text-sm" data-stale-accept="true">
              The card moved since it was read, so nothing was accepted. What is below is the card as it is NOW — check
              it before accepting again.
            </p>
          )}
          <AcceptCardView card={card} />
          {!mine ? (
            <p className="my-2 text-sm text-muted-foreground" data-authorship="not-mine">
              This is {owner}&apos;s work to accept. Accepting files the work under the accepting person&apos;s own
              name, so only they can do it — reading this is not the same as being able to act on it.
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
            <p className="my-2 text-sm text-muted-foreground" data-not-acceptable="true">
              {card.reason}
            </p>
          )}
          {pinRefused !== '' && (
            <p className="error my-2 text-sm" data-pin-refused="true">
              {pinRefused} Enter your PIN again — nothing was accepted.
            </p>
          )}
        </>
      )}
      {outcome !== null && <AcceptOutcomeView outcome={outcome} />}
    </Section>
  )
}

/**
 * The accept card, in the owner's language first (RA-B1 item 3).
 *
 * The PLAIN HALF says what accepting does in words the person who ordered the
 * work uses; the TECHNICAL HALF — every served field, the trailers
 * byte-for-byte — sits behind one fold, still on this surface BEFORE the
 * accept. That keeps the S13.6 step-3 rule true (the attribution is seen
 * before it becomes permanent: the signing statement stays unfolded, and the
 * exact record is one click, not one page, away) without asking a household
 * reviewer to read payload pins as a sentence.
 */
function AcceptCardView({ card }: { card: AcceptCard }) {
  return (
    <div className={cn('accept-card my-2', reviewPanel)} data-acceptable={card.acceptable ? 'true' : 'false'}>
      <p className="mt-0 mb-2 text-sm" data-accept-plain="true">
        You are accepting <strong>revision {String(card.revision_n)}</strong> — the version this page shows.{' '}
        {card.project_id === '' ? (
          <>The accepted copy stays filed here as the official version of this work, recorded as your own decision.</>
        ) : (
          <>
            The official copy is written into <strong>{card.project_id}</strong>&apos;s permanent record, in your name,
            as your own decision.
          </>
        )}{' '}
        The exact record — pins, attribution, the platform&apos;s signing posture — is under &quot;what happens
        technically&quot;, worth a look before your first accept.
      </p>
      <details className="accept-tech mb-1">
        <summary className="cursor-pointer text-sm text-muted-foreground">What happens technically</summary>
        {/* `.signing` KEEPS its rule — it is the LAST selector of the group
            `responsive.test.ts:245` reads — so no wrap utility competes. The
            HUMAN half of the attribution ("in your name, as your own
            decision") is stated in the plain sentence above, unfolded, which
            is what keeps S13.6 step 3 true; this is the platform's own
            mechanics statement, verbatim. */}
        <p className="signing mt-2 mb-2 text-sm" data-signing-structural={card.signing.structural ? 'true' : 'false'}>
          {card.signing.statement}
        </p>
        {/* The `dd` wrap that `.accept-card dd` carried moves to a descendant
            utility on the container, so one string covers every field. */}
        <dl
          className={cn(
            'mt-2 mb-2 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-1 text-sm',
            '[&_dd]:m-0 [&_dd]:wrap-anywhere [&_dt]:text-muted-foreground',
            figure,
          )}
        >
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
        <h5 className="mt-0 mb-1">Commit trailers, exactly as they will be written</h5>
        {card.trailers === '' ? (
          <Absent
            reason={
              card.provenance.absent === undefined || card.provenance.absent === ''
                ? 'no trailers could be rendered'
                : card.provenance.absent
            }
          />
        ) : (
          // `.trailers` KEEPS its rule (the `responsive.test.ts:242` group).
          <pre className="trailers rounded-(--radius-sm) border border-border bg-background p-2 text-xs" data-trailers="verbatim">
            {card.trailers}
          </pre>
        )}
        {/* ⚠ THE `provenance` CLASS IS DROPPED HERE, and that is a finding rather
            than a tidy. `.provenance` is the WORKFORCE map's rule — a `<dl>` laid
            out as a grid (index.css, the S15.10 section) — and this is a `<p>` of
            prose, so the accept card was silently taking a grid display, a grid
            column template and a margin written for somebody else's markup.
            `Workforce.tsx` is byte-frozen, so the rule stays for its real owner and
            this line carries its own utilities instead. */}
        <p className={cn('my-2 text-xs text-muted-foreground', figure)} data-provenance="accept-card">
          {card.provenance.minting_run_id === undefined || card.provenance.minting_run_id === '' ? (
            <Absent reason="no minting run recorded" />
          ) : (
            <>
              from run {card.provenance.minting_run_id} · engine {card.provenance.engine ?? ''} · model{' '}
              {card.provenance.model ?? ''} · lane {card.provenance.lane ?? ''} · {card.provenance.vendor_noreply ?? ''}
            </>
          )}
        </p>
      </details>
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
        A title for the record (optional)
        <input data-field="subject" value={subject} onChange={(e) => setSubject(e.target.value)} />
      </label>
      <label>
        A note about where this came from (optional)
        <input data-field="provenance" value={provenance} onChange={(e) => setProvenance(e.target.value)} />
      </label>
      <label>
        Your PIN
        <input type="password" data-field="accept-pin" value={pin} onChange={(e) => setPin(e.target.value)} />
      </label>
      <Button variant="primary" size="sm" type="submit" data-action="accept" disabled={sending}>
        Accept — make this the official version
      </Button>
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
    <div
      className={cn('accept-outcome my-2', reviewPanel)}
      data-applied={outcome.applied ? 'true' : 'false'}
      data-state={outcome.state}
    >
      <p className={cn(outcome.applied ? 'notice' : 'text-muted-foreground', 'mt-0 mb-2 text-sm')}>{outcome.detail}</p>
      <dl
        className={cn(
          'm-0 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-1 text-sm',
          '[&_dd]:m-0 [&_dd]:wrap-anywhere [&_dt]:text-muted-foreground',
          figure,
        )}
      >
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
    <div className={cn('merge-card mt-2', reviewPanel)} data-merge-card="true">
      <h5 className="mt-0 mb-1">This does not apply cleanly — nothing was pushed</h5>
      {view.card === null ? (
        <Absent reason="the collision card itself was not served" />
      ) : (
        // `.merge-card pre` KEEPS its rule (`responsive.test.ts:242`), so the
        // conflict block below carries no overflow or white-space utility.
        <dl
          className={cn(
            'm-0 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-1 text-sm',
            '[&_dd]:m-0 [&_dd]:wrap-anywhere [&_dt]:text-muted-foreground',
            figure,
          )}
        >
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
      {/* `.merge-options` KEEPS its rule (`responsive.test.ts:251`), so it takes
          no layout utility; only the list decoration moves. */}
      <ul className="merge-options mt-2 list-none ps-0">
        {view.options.map((o) => (
          <li
            key={o.option}
            data-merge-option={o.option}
            data-answerable={o.answerable ? 'true' : 'false'}
            className="text-sm"
          >
            <span className={cn('option-name font-semibold', figure)}>{o.option}</span> — {o.reason}
            {/* Same W2-6 rule as the doors: the wire address folds. */}
            {o.route !== undefined && o.route !== '' && (
              <details className="door-tech">
                <summary className="cursor-pointer text-xs text-muted-foreground">technical detail</summary>
                <span className={cn('text-xs text-muted-foreground', figure)}>
                  {o.route}
                  {o.preset !== undefined && o.preset !== '' ? ` · preset ${o.preset}` : ''}
                </span>
              </details>
            )}
          </li>
        ))}
      </ul>
      <p className="m-0 text-xs text-muted-foreground" data-durability="served">
        {view.durability}
      </p>
    </div>
  )
}
