import type { ReactNode } from 'react'

// Imported from the MODULE rather than the kit barrel on purpose, and the reason
// is the MODULE-EVALUATION GRAPH rather than the bundle.
//
// `ui/Timestamp` imports `Absent` from this file, so either form makes a cycle.
// Through the module the cycle is exactly two files, both of which export
// hoisted function declarations and neither of which calls the other at
// evaluation time — safe by construction. Through the barrel it would become
// parts ↔ ui/index → every kit module, so every unit test that mounts one
// primitive, and every surface that touches a `parts.tsx` render, would evaluate
// the whole kit to get a park horizon.
//
// CORRECTED 2026-08-05 (drain r1, D4): this comment first claimed a bundle
// saving. It is not one — MEASURED both ways, the chunk is 1113.19 kB via the
// module and 1113.21 via the barrel, because `@base-ui/react` already rides the
// landed `controls.tsx` → `ui/Modal` path and is in the chunk either way.
import { Timestamp } from './ui/Timestamp'
// Same reasoning, same shape: `ui/tone` imports nothing but a React type, so
// naming the module rather than the barrel keeps the evaluation graph two files
// wide instead of pulling every kit component in behind it.
import { toneStyle, type Tone } from './ui/tone'

/**
 * The shared render primitives every oversight surface is built from
 * (P3/CONVENTIONS.md §42).
 *
 * Three rules they exist to make structural rather than remembered:
 *
 *  - AN ABSENCE IS RENDERED, NOT FILLED. A null cost is "no meter reading",
 *    never $0; a park with no horizon is "parked, no horizon given", never a
 *    made-up time. `<Absent>` is the only way any of these views says nothing.
 *  - MONEY IS SHOWN AS SERVED. `<Money>` prints the number the API sent and
 *    does no arithmetic — no sums across owners, no division across lanes, no
 *    rounding into a figure the server never computed (§37).
 *  - STALE NEVER POSES AS LIVE. `<Freshness>` marks a view that owes a
 *    re-snapshot, and the marker sits above the data rather than beside it.
 *
 * There is deliberately NO progress bar, percentage, completion fraction or
 * ETA primitive here: the API serves none, so the views render none (§30).
 *
 * AMENDED 2026-08-05 (P3-UI-5, §51). `ThresholdRing` below is a figure-shaped
 * render and belongs under the sentence's own rationale rather than against it:
 * the rule is "the API serves none", and `pressure` IS served — computed by the
 * S10.4 gauge the scheduler admits work by, gated on the platform's own
 * `pressure_applicable`. The ring consumes that ratio and derives nothing, so
 * the carve-out is exactly the case the sentence never covered. Progress,
 * percentage-of-done, completion fraction and ETA remain unserved and unrendered.
 */

/** Absent states what is missing and WHY, in the server's own words wherever
 *  the server gave a reason. */
export function Absent({ reason }: { reason: string }) {
  return <span className="absent">{reason}</span>
}

/** Money renders a served dollar figure. The unit comes from the field name on
 *  the wire (…_usd), so naming it here is reading, not inventing; the value is
 *  printed as received, never re-scaled. */
export function Money({ usd }: { usd: number | null | undefined }) {
  if (usd === null || usd === undefined) return <Absent reason="no meter reading" />
  return <span className="money">USD {String(usd)}</span>
}

/** Stamp renders a served timestamp verbatim — the AUDIT render.
 *
 *  Deliberately not localized: every stamp the platform serves is UTC RFC3339,
 *  reformatting one is a transformation of a fact, and an ambiguous "2:00" on a
 *  park horizon is worse than an unambiguous instant.
 *
 *  DATED 2026-08-05 (P3-UI-4, §50). The human-friendly rendering this comment
 *  used to defer to "the S15.12 sweep" has been DECIDED and applied: the B6 gate
 *  D5 answer is relative time BESIDE the verbatim UTC on live surfaces and UTC
 *  ALONE in audit and history detail. `ui/Timestamp.tsx` is that primitive and
 *  the live sites were migrated to it here. `Stamp` is not deprecated and did
 *  not move: it already renders exactly the audit half of that contract, and the
 *  eighteen record sites that keep calling it are keeping the answer rather than
 *  waiting for one. */
export function Stamp({ ts }: { ts: string | null | undefined }) {
  if (!ts) return <Absent reason="not recorded" />
  return <time dateTime={ts}>{ts}</time>
}

/** ParkedUntil is the S15.5 "parked until…" line — including the honest case
 *  where the platform is parked but nothing told it for how long.
 *
 *  The horizon is a LIVE reading (D5, P3-UI-4): "in 20m" is what a person is
 *  actually asking when they read a park line, so it renders beside the served
 *  instant rather than instead of it. Between frames the label freezes with the
 *  data it describes, and a frozen FUTURE horizon freezes toward overstating the
 *  time left — which is exactly why the verbatim UTC never leaves. The absence
 *  arm is byte-kept: a park with no horizon still says so and invents nothing. */
export function ParkedUntil({ until }: { until: string | null | undefined }) {
  if (!until) return <Absent reason="parked, no horizon given" />
  return (
    <span className="parked-until">
      parked until <Timestamp ts={until} variant="live" />
    </span>
  )
}

/** Owner is the D2/S3.10 attribution that rides every figure: the fleet always
 *  shows whose account burns, and the board always shows whose work it is. */
export function Owner({ id }: { id: string }) {
  return <span className="owner">{id}</span>
}

/**
 * Freshness is the per-view half of "live by default" (S15.12).
 *
 * The shell's indicator says whether the FEED is live. This says whether THIS
 * view's data is current — the two come apart exactly when a view still owes a
 * re-snapshot, which is the moment stale data would otherwise look fresh.
 */
export function Freshness({ stale, error, hasData }: { stale: boolean; error: string; hasData: boolean }) {
  if (error !== '') return <p className="error">{error}</p>
  if (!stale) return null
  return (
    <p className="muted" data-freshness={hasData ? 'stale' : 'catching-up'}>
      {hasData
        ? 'Catching up — showing what was on screen before the gap, which may already be out of date.'
        : 'Catching up…'}
    </p>
  )
}

/** Section is one titled block of a surface, carrying its own freshness state
 *  so a reader can tell WHICH part of a screen is behind. */
export function Section({
  title,
  stale,
  children,
}: {
  title: string
  stale?: boolean
  children: ReactNode
}) {
  return (
    <section className="block" data-stale={stale ? 'true' : 'false'}>
      <h2>{title}</h2>
      {children}
    </section>
  )
}

/** Empty is the honest nothing-here: a bucket with no rows says so instead of
 *  disappearing, so a reader can tell "none" from "not loaded".
 *
 *  It is NOT deprecated: the surfaces this packet restyles moved their served-
 *  empty arms to the kit's teaching `EmptyState`, and the call sites on the
 *  surfaces UI-6/UI-7 own still render this. Each sheds at its own packet. */
export function Empty({ what }: { what: string }) {
  return <p className="muted">{what}</p>
}

/**
 * The D8 "what this is" line (B6 gate §9 A-1: a surface teaches its contract
 * rather than only admitting it is empty).
 *
 * One component rather than five copies of a class string, so the placement and
 * the treatment are structural across the oversight surfaces instead of
 * remembered — which is the drift the §47 migration ledger warns about.
 */
export function SurfaceHead({ title, what, aside }: { title: string; what: ReactNode; aside?: ReactNode }) {
  return (
    <header className="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <h1 className="mt-0 mb-1">{title}</h1>
        <p className="m-0 max-w-prose text-sm text-muted-foreground" data-surface-what>
          {what}
        </p>
      </div>
      {aside}
    </header>
  )
}

/**
 * The never-stall-silently banner (design proposal §3; the S02.5/S10.8
 * no-silent-caps invariant, drawn).
 *
 * A stopped thing says in plain words what stopped it and carries the door to
 * where it is answered. The DOOR IS PASSED IN and never assembled here: a
 * parked run is released from the card that is holding it and a wedged one from
 * the watchdog's card — both on the inbox — so this points and re-implements no
 * verb. No banner fires anything.
 */
export function StallBanner({
  kind,
  tone = 'yellow',
  what,
  children,
}: {
  kind: string
  tone?: Tone
  what: ReactNode
  children?: ReactNode
}) {
  return (
    <p
      data-stall={kind}
      style={toneStyle(tone)}
      className="my-1 flex flex-wrap items-center gap-x-2 gap-y-1 rounded-(--radius-sm) border border-[color-mix(in_srgb,var(--tone)_25%,transparent)] bg-[color-mix(in_srgb,var(--tone)_9%,transparent)] px-2 py-1 text-xs"
    >
      <span className="text-(--tone)">{what}</span>
      {children}
    </p>
  )
}

/**
 * The band edges, and they are DISPLAY constants with named reasons (§42
 * precedent; flagged to the standing settings-tab directive's list).
 *
 *  - `ringFull` 1: pressure is weighted consumption over the DECLARED ceiling,
 *    so 1 is the declaration reached. That is a served semantic read off the
 *    gauge's own contract, not a policy figure this client picked.
 *  - `ringWatch` 0.75: the glance-ahead band, so a lane that is about to arrive
 *    reads differently at a glance from one that is not.
 *
 * Neither is the ADMISSION threshold. Where background work actually stops
 * being started is a ⚙ setting on the server that no browser can read, so the
 * ring speaks about the declaration and never about a cutoff.
 */
const ringFull = 1
const ringWatch = 0.75

const ringRadius = 9
const ringSweep = 2 * Math.PI * ringRadius

function ringTone(ratio: number): Tone {
  if (ratio >= ringFull) return 'red'
  if (ratio >= ringWatch) return 'orange'
  return 'green'
}

/**
 * The capacity ring — ONE idiom for a served ratio (design proposal §3's
 * threshold donut, reimplemented in this codebase's own idiom).
 *
 * It renders the ratio it is handed and derives nothing. Recomputing it here
 * from a consumption figure and a declared ceiling would be a second
 * implementation of the S10.4 gauge, free to disagree with the one the
 * scheduler actually admits work by (D4, §11).
 *
 * The served figure stays rendered BESIDE it at every call site: the ring is
 * glanceable redundancy, never a replacement for the number the platform sent.
 */
export function ThresholdRing({ ratio, label }: { ratio: number; label: string }) {
  const tone = ringTone(ratio)
  const filled = Math.min(Math.max(ratio, 0), 1) * ringSweep
  return (
    <span
      data-capacity={tone}
      data-capacity-ratio={String(ratio)}
      style={toneStyle(tone)}
      className="inline-flex flex-none items-center"
    >
      <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true" className="-rotate-90">
        <circle cx="12" cy="12" r={ringRadius} fill="none" stroke="var(--border-l)" strokeWidth="3" />
        <circle
          cx="12"
          cy="12"
          r={ringRadius}
          fill="none"
          stroke="var(--tone)"
          strokeWidth="3"
          strokeLinecap="round"
          strokeDasharray={`${String(filled)} ${String(ringSweep)}`}
          className="motion-safe:transition-[stroke-dasharray] motion-safe:duration-300"
        />
      </svg>
      <span className="sr-only">{label}</span>
    </span>
  )
}
