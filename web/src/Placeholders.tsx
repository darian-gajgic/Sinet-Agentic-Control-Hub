import {
  BookOpen,
  FolderOpen,
  HeartPulse,
  History as HistoryIcon,
  PackageCheck,
  Trophy,
  type LucideIcon,
} from 'lucide-react'

import { HistoryPanel } from './History'
import { Link } from './router'
import { hrefFor, type RouteID } from './routes'
import { Button, EmptyState, Panel } from './ui'

/**
 * The seven routes the product map (v2.1 §2) publishes AHEAD of their surfaces
 * — build-order steps 2–7 fill them in. Each answers with an honest placeholder:
 * what WILL appear here (the map's own words), and the real door to where that
 * job is done today. No control here fires anything it cannot honour, and no
 * placeholder reads the API — a mocked screen that looks real is the failure
 * mode these exist to avoid.
 */

type Coming = {
  icon: LucideIcon
  /** What this surface will show, from the approved product map §3. */
  will: string
  /** The teaching line under it. */
  why: string
  /** Where the job is done TODAY — real links, or nothing. */
  today?: { label: string; to: string }[]
  /** Which build-order step delivers the real surface. */
  step: string
}

const coming: Partial<Record<RouteID, Coming>> = {
  projects: {
    icon: FolderOpen,
    will: 'Every project as a card: active tasks by stage, what is waiting on you, spend, recent deliverables and follow-up lineage — with "(no project)" as its own real bucket.',
    why: 'Opening a project will scope the whole app to it, and "Describe a goal" started from here pins the new task to the project so its planning builds on the project\'s prior work.',
    today: [
      { label: 'Scope the app with the project selector in the top bar', to: hrefFor('mission-control') },
      { label: 'See every task on the Board', to: hrefFor('board') },
    ],
    step: 'build step 2',
  },
  reviews: {
    icon: PackageCheck,
    will: 'Deliverables as numbered immutable revisions: side-by-side diffs with line-anchored comments, the judge\'s verdict and findings per round, image compare, and "Run it" — one click starts a program deliverable live in the before/after preview.',
    why: 'Round-over-round "what changed since I sent it back" becomes the default rework view; every previous revision stays navigable and downloadable.',
    today: [
      { label: 'Each task\'s deliverables open from its task page — start on the Board', to: hrefFor('board') },
      { label: '"What needs me" on Home lists work waiting for review', to: `${hrefFor('mission-control')}?view=what-needs-me` },
    ],
    step: 'build step 4',
  },
  lessons: {
    icon: Trophy,
    will: 'The feedback ledger as browsable shelves: WINS (measured successes) and LESSONS (a flop and its correction), each scoped to a person, a project or the house, with provenance.',
    why: 'These feed future work today — matching entries are injected into runs — so logging a win or a lesson here makes the platform measurably smarter.',
    today: [{ label: 'The entries live in Memory — browse and create them there', to: hrefFor('memory') }],
    step: 'build step 5',
  },
  health: {
    icon: HeartPulse,
    will: 'Known issues first: open watchdog flags, drift records, conformance failures, alarms and parked runs — each with what happened, why, and what to do — then benchmark state and canary status, displayed honestly.',
    // W2-8: the tab's badge said "17" while this room showed none of them —
    // the count and the room must tell one story, so the room says where
    // those counted items LIVE until it lands.
    why: 'The badge on this tab counts real open issues — but this room is not built yet, so it lists none of them here. Every one of those counted cards is in your Inbox today, and that is where they are answered. Every disposition verb (suppress, dismiss, acknowledge, resume) is already served and lands here.',
    today: [{ label: 'The counted issue cards are in the Inbox — answer them there', to: hrefFor('inbox') }],
    step: 'build step 5',
  },
  manual: {
    icon: BookOpen,
    will: 'A plain-words page per surface — what it is for, how to use it, what the words mean — plus the guided tour launcher for every tab.',
    why: 'Written for the household, not for engineers: no jargon survives review.',
    step: 'build step 6',
  },
}

export function ComingSurface({
  id,
  onStartTour,
}: {
  id: RouteID
  onStartTour?: () => void
}) {
  const c = coming[id]
  if (!c) return null
  const Icon = c.icon
  return (
    <section className="surface" data-coming={id}>
      <Panel>
        <EmptyState
          glyph={<Icon size={30} strokeWidth={1.6} aria-hidden="true" />}
          what={c.will}
          why={`${c.why} This surface arrives with ${c.step} of the rework — the URL is already the real one.`}
          action={
            <span className="flex flex-col items-center gap-2">
              {(c.today ?? []).map((door) => (
                <Link key={door.to + door.label} to={door.to} className="text-sm text-(--accent-2) underline">
                  {door.label}
                </Link>
              ))}
              {id === 'manual' && onStartTour !== undefined && (
                <Button variant="ghost" size="sm" onClick={onStartTour}>
                  Show me around — start the guided tour
                </Button>
              )}
            </span>
          }
        />
      </Panel>
    </section>
  )
}

/**
 * /history — the placeholder carries the ALREADY-BUILT interim instrument.
 *
 * The full observability surface (filterable event stream, drill-through) is
 * build step 6; the served query layers — views, canned questions, open SQL,
 * search — landed at B6-5 and used to live at the bottom of Home. Home is a
 * glance now (map §3), so the working instrument moves HERE, where the map
 * says it belongs, rather than being parked behind a promise.
 */
export function HistorySurface() {
  return (
    <section className="surface" data-coming="history">
      <p className="m-0 mb-4 max-w-prose text-sm text-muted-foreground">
        The platform&apos;s own record, queryable: served views, canned questions, open questions with their audit, and
        redact-before-match search. The filterable event stream and drill-through arrive with build step 6 of the
        rework. <HistoryIcon className="inline align-[-2px]" size={14} aria-hidden="true" /> Answers are
        point-in-time — this is a query instrument, not a live feed, and it says so below.
      </p>
      <HistoryPanel />
    </section>
  )
}
