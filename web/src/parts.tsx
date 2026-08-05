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
 *  disappearing, so a reader can tell "none" from "not loaded". */
export function Empty({ what }: { what: string }) {
  return <p className="muted">{what}</p>
}
