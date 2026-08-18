import { useState, type ReactNode } from 'react'

import { ApiError, Unreachable } from './api'
import { Button, Modal } from './ui'

/**
 * The act layer the D3 control groups share (P3/CONVENTIONS.md §48).
 *
 * A control on this platform is three things and no more: a verb the server
 * already offers, a plain-words statement of what pressing it will do, and the
 * server's OWN account of what happened rendered back verbatim. Everything here
 * exists to make those three the path of least resistance, so no surface has to
 * re-decide them.
 *
 * WHY THIS IS NOT IN `ui/`. The kit styles surfaces; it deliberately does not
 * re-open §42's honesty rules. What lives here IS an honesty rule — that a
 * server saying "nothing was cancelled, it had already ended" is neither a
 * success nor a failure, and that a 409 is a re-read rather than a fault — so it
 * belongs beside the surfaces rather than inside the styling layer. It builds
 * FROM the kit (`Modal`, `Button`) and declares no colour of its own.
 */

/**
 * The four honest shapes an act can come back in. Three of them are the SERVER
 * answering successfully, which is why they are not one "ok" arm:
 *
 *  - `applied` — something changed, and the platform says what.
 *  - `noop`    — the platform correctly did nothing (`applied:false`: the run
 *                had already ended; the switch was already in that position).
 *                Rendering this as an error would be a lie about the platform.
 *  - `retry`   — a 409. The subject moved under the reader (a claim window, a
 *                raced transition); the answer is to re-read and decide again,
 *                which is not a failure of anything.
 *  - `failed`  — the server refused (403/404/503/400) and said why.
 *
 * The landed inbox idiom carried two of these (`applied`/`failed`); the two
 * middle arms are added here because this packet's verbs serve both, and
 * collapsing them would put the platform's honest no-op behind failure styling.
 */
export type ActKind = 'applied' | 'noop' | 'retry' | 'failed'

export type ActOutcome = {
  kind: ActKind
  /** This client's one-line account of WHICH arm it is. Never a restatement of
   *  the platform's reasoning — that is `detail`'s job. */
  note: ReactNode
  /** The server's own sentence, rendered verbatim and never parsed. */
  detail: string
}

/** applied/noop from a verb that reports whether it did anything. */
export function outcomeOf(applied: boolean, note: ReactNode, detail: string): ActOutcome {
  return { kind: applied ? 'applied' : 'noop', note, detail }
}

/**
 * The default 409 note, and it is a CLAIM ABOUT THE VERB rather than a
 * pleasantry: a conflict on a single-subject verb fires nothing, because the
 * one transition it would have made is the one that was refused.
 *
 * A MULTI-SUBJECT verb cannot say this and must not. `stage.CancelTask` walks
 * its runs in order and each cancel commits on its own, so a run refusing
 * mid-dispatch stops the walk with everything already cancelled STILL cancelled
 * (internal/stage/cancel.go:221–233). Such a call site passes its own note.
 */
export const nothingFired = 'nothing fired — re-read it and decide again'

/**
 * refusalOf classifies from the server's MACHINE CODE, never from its prose.
 *
 * `claim_in_flight` and `cancel_raced` are the two 409s of this packet's verbs
 * and both mean the same thing to a reader: what you were looking at moved, so
 * look again. Everything else the server refuses is rendered as the refusal it
 * is, carrying the server's own text.
 */
export function refusalOf(err: unknown, conflictNote: string = nothingFired): ActOutcome {
  if (err instanceof ApiError) {
    const retry = err.status === 409
    return {
      kind: retry ? 'retry' : 'failed',
      note: retry ? conflictNote : `refused (${err.code || String(err.status)})`,
      detail: err.message,
    }
  }
  if (err instanceof Unreachable) {
    return {
      kind: 'failed',
      note: 'nothing fired',
      detail: 'The control plane is unreachable — the host may be asleep or the tailnet down.',
    }
  }
  return { kind: 'failed', note: 'nothing fired', detail: 'The control plane could not be read.' }
}

const outcomeClass: Record<ActKind, string> = {
  applied: 'notice',
  // A no-op and a re-read are STATEMENTS, not faults, so neither takes the
  // error class. The distinction survives in `data-outcome` for anything that
  // needs to tell them apart.
  noop: 'muted',
  retry: 'muted',
  failed: 'error',
}

/** The act-outcome line, in place, where the act was pressed. */
export function OutcomeLine({ outcome }: { outcome: ActOutcome | null }) {
  if (!outcome) return null
  return (
    <p className={outcomeClass[outcome.kind]} data-outcome={outcome.kind}>
      {outcome.note}
      {outcome.detail !== '' && <span className="muted"> — {outcome.detail}</span>}
    </p>
  )
}

export type Act = {
  busy: boolean
  outcome: ActOutcome | null
  /**
   * run fires one act, records its outcome and then RE-READS.
   *
   * The re-read is not optional and is not conditional on the outcome: the UI
   * never flips a rendered state itself (§42 — REST is the truth), so even a
   * refusal re-reads, because a refusal is often the platform telling you the
   * thing you are looking at has moved.
   */
  run(fire: () => Promise<ActOutcome>, reread: () => void, conflictNote?: string): void
  clear(): void
}

export function useAct(): Act {
  const [busy, setBusy] = useState(false)
  const [outcome, setOutcome] = useState<ActOutcome | null>(null)
  return {
    busy,
    outcome,
    // `conflictNote` is how a MULTI-SUBJECT verb says what its own 409 means.
    // The default claims nothing fired, which only a single-subject verb may.
    run(fire, reread, conflictNote) {
      setBusy(true)
      fire()
        .then(setOutcome, (err: unknown) => setOutcome(refusalOf(err, conflictNote)))
        .finally(() => {
          setBusy(false)
          reread()
        })
    },
    clear() {
      setOutcome(null)
    },
  }
}

/* ── the cancel's why (P3-RW-19) ─────────────────────────────────────────── */

/**
 * The one bound the cancel verbs apply to the reason, mirrored from
 * internal/api/actions.go `cancelReasonMaxRunes`. It counts RUNES there, so it
 * counts code points here — `.length` counts UTF-16 units and would hold back
 * a sentence the server accepts (or wave through one it refuses) the moment
 * anyone types outside the basic plane.
 */
export const cancelReasonMaxRunes = 280

export function runeCount(s: string): number {
  return [...s].length
}

/** The reason is over the server's bound, so the act is HELD — the same
 *  pre-validation stance the History question form takes: stop a request the
 *  server already refuses, never answer for the server. */
export function whyHolds(why: string): boolean {
  return runeCount(why) > cancelReasonMaxRunes
}

/** The over-bound sentence, ONE spelling for every surface that says it
 *  (the modal ceremony and the door's own field idiom): the bound is said,
 *  nothing is truncated, and cancelling without a reason stays open. Null
 *  while the why fits. */
export function whyOverLine(why: string): string | null {
  const runes = runeCount(why)
  if (runes <= cancelReasonMaxRunes) return null
  return `That is ${String(runes)} characters and the record keeps one line (${String(cancelReasonMaxRunes)}). Nothing is sent while it is over — shorten it, or clear it to cancel without a reason. Nothing is ever cut short for you.`
}

/**
 * The person-language arm for the one refusal this ceremony can draw
 * (`reason_too_long`, 400, nothing cancelled). The held field makes it
 * unreachable from this client, but a refusal that does arrive renders in the
 * person's words with the server's own sentence beside them — never as a bare
 * machine code.
 */
export function catchReasonRefusal(err: unknown): ActOutcome {
  if (err instanceof ApiError && err.code === 'reason_too_long') {
    return {
      kind: 'failed',
      note: 'nothing was cancelled — the reason needs to fit one line',
      detail: err.message,
    }
  }
  throw err
}

/**
 * The optional one-line why, part of the cancel confirm ceremony wherever the
 * two cancel verbs fire (P3-RW-19: the walk found rule citations where a
 * motive should be — this is where the motive enters).
 *
 * THE BOUND IS SAID, NEVER APPLIED SILENTLY: nothing is truncated, ever. Near
 * the bound a quiet count appears; over it the field says so in plain words
 * and the act is held (`whyHolds` feeds ActConfirm's `disabled`, which is the
 * N7 held-not-busy arm — the field beside the act states why it will not
 * fire). An empty why is always fine and always says so.
 */
export function CancelWhy({
  value,
  onChange,
  keptWhere = 'kept with the record of what was stopped',
}: {
  value: string
  onChange: (v: string) => void
  /** Where the words end up, in the sentence's own words — the run's record,
   *  every run this stops, the task's record. */
  keptWhere?: string
}) {
  const runes = runeCount(value)
  const over = runes > cancelReasonMaxRunes
  return (
    <label className="cancel-why flex flex-col gap-1 text-xs" data-cancel-why-field>
      <span className="text-muted-foreground">Why? — optional, one line, {keptWhere}</span>
      <input
        type="text"
        data-field="cancel-why"
        value={value}
        onChange={(e) => {
          onChange(e.currentTarget.value)
        }}
        className="rounded-(--radius-sm) border border-border bg-transparent px-2 py-1 text-sm"
      />
      {over ? (
        <span className="warn-flag" data-why-over="true">
          {whyOverLine(value)}
        </span>
      ) : runes > cancelReasonMaxRunes - 40 ? (
        <span className="muted" data-why-count>
          {String(runes)} of {String(cancelReasonMaxRunes)} characters
        </span>
      ) : null}
    </label>
  )
}

export type ActConfirmProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  /** What pressing the act will do, in plain words, faithful to the verb. */
  what: ReactNode
  /** The act button's label. */
  act: string
  variant?: 'danger' | 'primary'
  /** A request is in flight. It means EXACTLY that and nothing else. */
  busy: boolean
  /**
   * The act is held back for a reason the form beside it already STATES.
   *
   * It is separate from `busy` on purpose (carried note N7, P3-UI-2 evaluation):
   * the budget editor passed `busy || invalid` into one prop, so a form that had
   * simply not been filled in yet was indistinguishable from a request in
   * flight — and "why can't I press this?" got the wrong answer. A held act
   * fires nothing and says why at the field; a busy act is one already sent.
   */
  disabled?: boolean
  onConfirm: () => void
  children?: ReactNode
}

/**
 * Confirm-before-fire: the one idiom every destructive or declaring control on
 * this platform uses.
 *
 * The dialog says what will happen BEFORE it happens, in the platform's terms,
 * and the dismiss is a plain ghost so the act is the only emphasized thing on
 * screen. It teaches nothing beyond the act itself — tours, hints and spotlights
 * are a later row's, deliberately.
 */
export function ActConfirm({
  open,
  onOpenChange,
  title,
  what,
  act,
  variant = 'danger',
  busy,
  disabled = false,
  onConfirm,
  children,
}: ActConfirmProps) {
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description={what}
      footer={
        <>
          <Button variant="ghost" size="sm" data-act="dismiss" onClick={() => onOpenChange(false)}>
            Leave it alone
          </Button>
          <Button
            variant={variant}
            size="sm"
            data-act="confirm"
            data-busy={String(busy)}
            disabled={busy || disabled}
            onClick={onConfirm}
          >
            {act}
          </Button>
        </>
      }
    >
      {children}
    </Modal>
  )
}
