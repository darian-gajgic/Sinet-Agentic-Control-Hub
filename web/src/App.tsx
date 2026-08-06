import { useCallback, useEffect, useRef, useState } from 'react'
import {
  BookOpen,
  Columns3,
  Database,
  FolderOpen,
  HeartPulse,
  History as HistoryIcon,
  Inbox as InboxIcon,
  LayoutDashboard,
  Menu,
  MessagesSquare,
  PackageCheck,
  Server,
  SlidersHorizontal,
  Trophy,
  Users,
  X,
  type LucideIcon,
} from 'lucide-react'

import { ApiError, Unreachable, api, type Session } from './api'
import { Board } from './Board'
import { Chat } from './Chat'
import { ConnectionState } from './ConnectionState'
import { Deliverable } from './Deliverable'
import { EventStream, sharedStream, type Status } from './events'
import { Login } from './Login'
import { Fleet } from './Fleet'
import { Inbox, InboxItem } from './Inbox'
import { DescribeGoal } from './Intake'
import { inboxEventTypes, useLive } from './live'
import { Memory, MemoryEntryView } from './Memory'
import { Settings } from './Settings'
import { MissionControl } from './MissionControl'
import { TaskDetail } from './TaskDetail'
import { NotFound } from './Stub'
import { ComingSurface, HistorySurface } from './Placeholders'
import { Projects } from './Projects'
import { OldFence } from './fence'
import { ProjectScopeContext, scopedRoutes, useProjectScope } from './project'
import { Link, navigate, useRoute } from './router'
import { ToastProvider } from './ui'
import { TurnToasts, useEtiquette } from './etiquette'
import { TourButton, TourOverlay, tourSteps, useTour } from './tour'
import { hrefFor, routes, type RouteDef, type RouteID } from './routes'
import { Workforce } from './Workforce'

/**
 * The app shell, reworked to the ratified art direction (2026-08-05): the Nexus
 * violet-glass control room — grouped sidebar over a blurred near-black band,
 * topbar with the page's own title and the global project selector, connection
 * state always visible — recreated from the real Nexus source
 * (~/Nexus-Agentic-Coding-Setup/app/static/) on this stack.
 *
 * Sidebar grouping is PRESENTATION and lives here, not in the route table:
 * `routes.ts` owns ids, patterns, titles and nav flags. Sections are the
 * CONSECUTIVE RUNS of this label over the table's own nav order, which is what
 * makes grouping incapable of reordering or dropping a link: every navigable
 * route is walked exactly once, in table order, and one with no label here
 * still renders — in its own unlabelled run.
 *
 * The groups are the product map §2's: Work · Results · Intelligence · System.
 */
const navGroupOf: Partial<Record<RouteID, string>> = {
  'mission-control': 'Work',
  projects: 'Work',
  board: 'Work',
  inbox: 'Work',
  reviews: 'Results',
  lessons: 'Results',
  chat: 'Intelligence',
  history: 'Intelligence',
  memory: 'Intelligence',
  workforce: 'System',
  fleet: 'System',
  health: 'System',
  settings: 'System',
  manual: 'System',
}

const navIcons: Partial<Record<RouteID, LucideIcon>> = {
  'mission-control': LayoutDashboard,
  projects: FolderOpen,
  board: Columns3,
  inbox: InboxIcon,
  reviews: PackageCheck,
  lessons: Trophy,
  chat: MessagesSquare,
  history: HistoryIcon,
  memory: Database,
  workforce: Users,
  fleet: Server,
  health: HeartPulse,
  settings: SlidersHorizontal,
  manual: BookOpen,
}

/** The topbar's one-line answer to "what is this page for" (the Nexus page-sub,
 *  carried). Routes without one render the title alone. */
const pageSub: Partial<Record<RouteID, string>> = {
  'mission-control': 'The household’s work at a glance',
  projects: 'Work lives in projects',
  new: 'Describe a goal in plain words',
  board: 'Every task by stage',
  task: 'One task, end to end',
  inbox: 'Everything that needs a person',
  'inbox-item': 'One decision',
  reviews: 'Judge the work that came back',
  deliverable: 'One deliverable under review',
  lessons: 'What worked, and what taught a lesson',
  chat: 'Ask about anything, hand over work',
  history: 'The platform’s own record, queryable',
  memory: 'What the platform knows, scoped and sourced',
  'memory-entry': 'One knowledge entry',
  workforce: 'The workforce map — who does what, measured',
  fleet: 'Who burns what, on whose account',
  health: 'Known issues first, honestly',
  settings: 'Every setting, with bounds and history',
  manual: 'How to use each surface, in plain words',
  login: 'Tailnet first, PIN second',
}

/**
 * EVERY ROUTE IN THE TABLE IS BUILT — the seven rework placeholders included,
 * each an honest surface that says what will appear there. `Stub` retired from
 * the switch because nothing routes to it; NotFound remains the terminal arm.
 */
export default function App({ stream }: { stream?: EventStream } = {}) {
  const { route, params } = useRoute()
  const { session, reload, failure } = useSession()
  const authed = session?.authenticated === true

  // ▲ v3 (operator, checkpoint 2): a task opens as a structured OVERLAY over
  // the surface it was opened from, never a full-page swap. The shell
  // remembers the last non-task view; while /tasks/:id is the address, that
  // view stays mounted underneath and the card floats over it. A COLD deep
  // link has no underlay — the card renders standalone and closing it lands
  // on the board.
  const underRef = useRef<{ route: RouteDef; params: Record<string, string> } | null>(null)
  const overlayOpen = route.id === 'task'
  if (!overlayOpen) underRef.current = { route, params }
  const shown = overlayOpen ? underRef.current : { route, params }
  const v = shown?.route.id
  const vp = shown?.params ?? {}
  const status = useConnection(authed, stream)
  const { dots, requests, drain } = useEtiquette(route.id, authed, stream)
  // The guided tour (P3-UI-7 R13). It NEVER auto-starts: nothing in this client
  // persists, so a self-starting tour would ambush every load. The header's
  // TourButton and the Manual page's launcher are the only things that start it.
  const tour = useTour(authed)

  // The global project scope (map §2): tab-lifetime state, no storage (§41-B).
  const [project, setProject] = useState('')

  // The phone nav drawer (the Nexus mobile pass, min-width-first). Route
  // changes close it, so a nav tap never leaves the drawer over the new page.
  const [navOpen, setNavOpen] = useState(false)
  useEffect(() => {
    setNavOpen(false)
  }, [route.id, params.id])

  // Fail closed, both directions: no session outside /login, and no /login
  // once there is one. `next` survives the round trip so a deep link that
  // arrived signed-out still lands where it was going.
  //
  // THE DEV-FALLBACK CARVE-OUT (B6 gate §9 finding C-1, fixed at P3-UI-4):
  // `session.dev === true` is the wire's own marker for the dev identity —
  // served as `{authenticated:true, dev:true}` with NO `user` object — and a
  // real cookie always wins, so a dev-posture person who signs in stops
  // matching this arm immediately. Production is byte-equivalent: without the
  // fallback `dev` is never true.
  const devFallback = authed && session?.dev === true
  useEffect(() => {
    if (session === null) return
    if (!authed && route.id !== 'login') {
      const next = window.location.pathname + window.location.search
      navigate(`/login?next=${encodeURIComponent(next)}`, { replace: true })
      return
    }
    if (authed && session.dev !== true && route.id === 'login') {
      const next = new URLSearchParams(window.location.search).get('next')
      navigate(safeNext(next), { replace: true })
    }
  }, [authed, route.id, session])

  return (
    <ToastProvider>
      <ProjectScopeContext.Provider value={{ project, setProject }}>
        <div className="shell">
          <div className="aurora" aria-hidden="true" />
          <TurnToasts requests={requests} drain={drain} />

          {/* The scrim renders only while the drawer is open, under the sidebar
              and over the page: one tap anywhere outside the nav closes it. */}
          {navOpen && (
            <div
              className="shell-scrim"
              data-scrim
              aria-hidden="true"
              onClick={() => {
                setNavOpen(false)
              }}
            />
          )}


          <div className="shell-body">
            <header className="shell-head">
              <button
                type="button"
                className="menu-btn"
                aria-label="Open navigation"
                onClick={() => {
                  setNavOpen(true)
                }}
              >
                <Menu size={17} strokeWidth={2} aria-hidden="true" />
              </button>
              <div className="page-title">
                <h1>{route.title}</h1>
                {pageSub[route.id] !== undefined && <span className="page-sub">{pageSub[route.id]}</span>}
              </div>
              <div className="head-right">
                {authed && <ProjectSelector route={route.id} me={session?.user?.user_id ?? 'dev'} stream={stream} />}
                {authed && <TourButton onStart={tour.start} />}
                <ConnectionState status={status} />
                {authed && (
                  <span className="who">
                    {devFallback ? (
                      <Link to={hrefFor('login')} data-auth="sign-in">
                        Sign in
                      </Link>
                    ) : (
                      <>
                        <span className="who-avatar" aria-hidden="true">
                          {(session?.user?.display_name ?? '?').slice(0, 1).toUpperCase()}
                        </span>
                        <span className="who-name">{session?.user?.display_name ?? ''}</span>
                        <button
                          type="button"
                          data-auth="sign-out"
                          onClick={() => {
                            void api.logout().then(reload, reload)
                          }}
                        >
                          Sign out
                        </button>
                      </>
                    )}
                    {devFallback && <span className="who-name muted">dev</span>}
                  </span>
                )}
              </div>
            </header>

            <main className="shell-main">
              <div className="view-in" key={`${v ?? 'none'}:${vp.id ?? ''}`}>
                {/* The fence rule (P1): every not-yet-reworked OLD surface
                    declares itself before its own content renders. The fence
                    follows the SHOWN surface — the one under the overlay. */}
                {authed && v !== undefined && <OldFence route={v} />}
                {session === null ? (
                  <p className="muted">{failure === '' ? 'Loading…' : failure}</p>
                ) : v === undefined ? (
                  // A cold /tasks/:id deep link: nothing underneath — the
                  // card renders standalone over the room itself.
                  null
                ) : v === 'login' ? (
                  <Login session={session} onSignedIn={reload} />
                ) : v === 'not-found' ? (
                  <NotFound pathname={window.location.pathname} />
                ) : v === 'mission-control' ? (
                  <MissionControl stream={stream} me={session.user?.user_id ?? ''} search={window.location.search} />
                ) : v === 'new' ? (
                  <DescribeGoal search={window.location.search} stream={stream} />
                ) : v === 'projects' ? (
                  <Projects me={session.user?.user_id ?? ''} stream={stream} />
                ) : v === 'board' ? (
                  <Board me={session.user?.user_id ?? ''} stream={stream} />
                ) : v === 'fleet' ? (
                  <Fleet
                    me={session.user?.user_id ?? ''}
                    operator={session.dev === true || session.user?.role === 'operator'}
                    stream={stream}
                  />
                ) : v === 'inbox' ? (
                  <Inbox stream={stream} />
                ) : v === 'inbox-item' ? (
                  <InboxItem id={vp.id} stream={stream} />
                ) : v === 'settings' ? (
                  <Settings stream={stream} />
                ) : v === 'chat' ? (
                  <Chat stream={stream} search={window.location.search} />
                ) : v === 'deliverable' ? (
                  <Deliverable id={vp.id} me={session.user?.user_id ?? ''} stream={stream} />
                ) : v === 'workforce' ? (
                  <Workforce stream={stream} />
                ) : v === 'memory' ? (
                  <Memory
                    me={session.user?.user_id ?? ''}
                    operator={session.user?.role === 'operator'}
                    stream={stream}
                  />
                ) : v === 'memory-entry' ? (
                  <MemoryEntryView
                    id={vp.id}
                    me={session.user?.user_id ?? ''}
                    operator={session.user?.role === 'operator'}
                    stream={stream}
                  />
                ) : v === 'history' ? (
                  <HistorySurface />
                ) : v === 'manual' ? (
                  <ComingSurface id="manual" onStartTour={tour.start} />
                ) : (
                  // reviews · lessons · health — the map's published-ahead
                  // placeholders, each an honest surface.
                  <ComingSurface id={v} />
                )}
              </div>

              {/* The task card, floating over whatever surface it was opened
                  from. Closing returns there (history.back over the push that
                  opened it); a cold load closes to the board. */}
              {overlayOpen && authed && session !== null && (
                <TaskDetail
                  id={params.id}
                  stream={stream}
                  onClose={() => {
                    if (underRef.current !== null) window.history.back()
                    else navigate(hrefFor('board'))
                  }}
                />
              )}
            </main>
          </div>

          <aside className="shell-side" data-open={navOpen ? 'true' : undefined}>
            <div className="brand-row">
              <Link to={hrefFor('mission-control')} className="brand">
                <span className="brand-mark" aria-hidden="true">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6">
                    <polygon points="12 2.5 21 7.5 21 16.5 12 21.5 3 16.5 3 7.5" />
                    <polygon points="12 7 16.5 9.5 16.5 14.5 12 17 7.5 14.5 7.5 9.5" />
                    <circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none" />
                  </svg>
                </span>
                <span className="brand-txt">
                  <span className="brand-name">Sinet</span>
                  <span className="brand-sub">Agentic Control Hub</span>
                </span>
              </Link>
              <button
                type="button"
                className="menu-btn nav-close"
                aria-label="Close navigation"
                onClick={() => {
                  setNavOpen(false)
                }}
              >
                <X size={16} strokeWidth={2} aria-hidden="true" />
              </button>
            </div>

            {authed && <SideNav route={route} me={session?.user?.user_id ?? 'dev'} dots={dots} stream={stream} status={status} />}
            <SideFooter authed={authed} />
          </aside>

          {tour.running && tour.step !== null && (
            <TourOverlay
              step={tour.step}
              index={tourSteps.indexOf(tour.step)}
              total={tourSteps.length}
              onNext={tour.next}
              onStop={tour.stop}
            />
          )}
        </div>
      </ProjectScopeContext.Provider>
    </ToastProvider>
  )
}

/**
 * The grouped nav with its two honest badges.
 *
 * BADGES ARE SERVED COUNTS, NEVER FRAME ARITHMETIC: both ride `useLive`, so a
 * frame triggers a RE-READ of the queue and the count is always the length of
 * the list the person will find when they open the surface. A read that has
 * not landed renders NO badge — never a fabricated zero.
 *
 *  - Inbox: every open card in the caller's own served queue.
 *  - Health & evals: the oversight subset of the same queue (watchdog flags,
 *    drift, conformance, alarms) plus runs the platform has wedged — the
 *    "known issues" the map's Health surface will list.
 */
const overseeKinds = ['watchdog_flag', 'drift_card', 'conformance_card', 'benchmark_alarm']

function SideNav({
  route,
  me,
  dots,
  stream,
  status,
}: {
  route: RouteDef
  /** WHOSE reads these are. In the key so a sign-in or sign-out re-reads:
   *  the served queue is identity-scoped and a count from the previous
   *  identity must not survive onto the next one's screen. */
  me: string
  dots: Partial<Record<string, boolean>>
  stream?: EventStream
  status: Status
}) {
  const asks = useLive({
    key: `/api/approvals#nav:${me}`,
    read: () => api.approvals(),
    types: inboxEventTypes,
    stream,
  })

  const badges: Partial<Record<RouteID, number>> = {}
  if (asks.data !== null) {
    badges.inbox = asks.data.items.length
    // The oversight subset of the same served queue. Wedged runs join this
    // count when the Health surface itself lands (build step 6) — counting
    // them here would cost the shell a second global read for a number no
    // placeholder page can drill into yet.
    badges.health = asks.data.items.filter((i) => overseeKinds.includes(i.kind)).length
  }

  return (
    <nav className="shell-nav" aria-label="Surfaces">
      {navSections().map((section) => (
        <div className="nav-group" key={section.label || section.items[0].id}>
          {section.label !== '' && <span className="nav-group-label">{section.label}</span>}
          {section.items.map((r) => {
            const Icon = navIcons[r.id]
            const badge = badges[r.id]
            return (
              <Link
                key={r.id}
                to={r.pattern}
                aria-current={r.id === route.id ? 'page' : undefined}
                data-pinned={r.id === 'chat' ? 'true' : undefined}
                // The dot says "something arrived here while you were
                // elsewhere" and NOTHING about how much (§42).
                data-dot={dots[r.id] === true ? 'true' : undefined}
              >
                {Icon && <Icon className="nav-icon" size={18} strokeWidth={1.7} aria-hidden="true" />}
                <span className="nav-label">{r.title}</span>
                {r.id === 'chat' && (
                  // The assistant's live mark is the STREAM's state, not a
                  // decoration: it pulses only while the feed is genuinely live.
                  <span
                    className={
                      status === 'live'
                        ? 'ms-auto text-[9px] text-(--accent-2) motion-safe:animate-[dot-pulse_2s_ease-in-out_infinite]'
                        : 'ms-auto text-[9px] text-(--text-faint)'
                    }
                    aria-hidden="true"
                  >
                    ◉
                  </span>
                )}
                {badge !== undefined && badge > 0 && (
                  <span className="nav-badge" aria-label={`${String(badge)} waiting`}>
                    {badge}
                  </span>
                )}
                {dots[r.id] === true && (badge === undefined || badge === 0) && r.id !== 'chat' && (
                  <span
                    className="ms-auto size-1.5 shrink-0 rounded-full bg-(--accent) motion-safe:animate-[dot-pulse_2s_ease-in-out_infinite]"
                    aria-label="new since you were last here"
                  />
                )}
              </Link>
            )
          })}
        </div>
      ))}
    </nav>
  )
}

/**
 * The sidebar footer: the Nexus CPU · MEM · GPU · GPU-MEM micro-meters as
 * HONEST placeholders — the host-monitoring wiring is operator-deferred (map
 * §6), so the tracks render empty with no value, never a fabricated zero — and
 * the version line, read once from the platform's own health answer.
 */
function SideFooter({ authed }: { authed: boolean }) {
  const [version, setVersion] = useState('')
  useEffect(() => {
    // One read at mount; the version changes only on deploy. Pre-session route:
    // it answers signed-out too, and a failure leaves the line at its name.
    api.health().then(
      (h) => {
        setVersion(typeof h.version === 'string' ? h.version : '')
      },
      () => {
        setVersion('')
      },
    )
  }, [])

  return (
    <div className="side-foot">
      {authed && (
        <div className="sys-mini" title="Host meters are not wired yet — the monitoring seam is operator-deferred.">
          {['CPU', 'MEM', 'GPU', 'GPU MEM'].map((m) => (
            <div className="mini-row" key={m}>
              <span>{m}</span>
              <span className="mini-bar" aria-hidden="true" />
              <b>—</b>
            </div>
          ))}
          <p className="mini-note">host meters — not wired yet</p>
        </div>
      )}
      <p className="side-version">SINET{version !== '' ? ` ${version}` : ''}</p>
    </div>
  )
}

/**
 * The global project selector (map §2) — the Nexus focus-context chips carried
 * onto the real contract. The choice list is DERIVED FROM SERVED ROWS: every
 * distinct project on the caller's own tasks read, "(no project)" included,
 * because it is a real served bucket. Scoping is presentation over
 * already-owner-scoped reads; surfaces without a project dimension say so here
 * instead of pretending.
 */
function ProjectSelector({ route, me, stream }: { route: RouteID; me: string; stream?: EventStream }) {
  const { project, setProject } = useProjectScope()
  // JOIN, NEVER OPEN (§45): the topbar sits above <main> in the tree, so its
  // effects would commit before the route view's and this read would become
  // the stream's opener — putting every view into joiner debt (two reads per
  // mount). One deferred commit keeps the view the opener; the selector pays
  // its own join re-snapshot instead, which is the cheap side of that trade.
  const [joined, setJoined] = useState(false)
  useEffect(() => {
    setJoined(true)
  }, [])

  const applied = scopedRoutes.has(route)

  return (
    <span className="scope" data-scoped={project !== '' ? 'true' : undefined}>
      <FolderOpen className="scope-ico" size={14} strokeWidth={1.8} aria-hidden="true" />
      <label className="scope-label">
        <span className="sr-only">Project scope</span>
        <select
          className="scope-select"
          value={project}
          onChange={(e) => {
            setProject(e.target.value)
          }}
        >
          <option value="">All projects</option>
          {joined ? (
            <ScopeOptions project={project} me={me} stream={stream} />
          ) : (
            project !== '' && <option value={project}>{project}</option>
          )}
        </select>
      </label>
      {project !== '' && (
        <button
          type="button"
          className="scope-clear"
          title="Clear the project scope"
          aria-label="Clear the project scope"
          onClick={() => {
            setProject('')
          }}
        >
          ✕
        </button>
      )}
      {project !== '' && !applied && (
        <span className="scope-note" title="This page has no project dimension, so the scope does not narrow it.">
          not applied here
        </span>
      )}
    </span>
  )
}

/**
 * The choice list, DERIVED FROM SERVED ROWS: every distinct project on the
 * caller's own tasks read, "(no project)" included because it is a real served
 * bucket. Its own component so the subscription can join one commit after the
 * route view opened the stream (see ProjectSelector). The list moves when a
 * task is born — the intake family's frame — not on every run heartbeat.
 */
function ScopeOptions({ project, me, stream }: { project: string; me: string; stream?: EventStream }) {
  const tasks = useLive({
    // Keyed on the identity like the badges: the task list is owner-scoped.
    key: `/api/tasks#projects:${me}`,
    read: () => api.tasks(),
    types: ['intake.state'],
    stream,
  })

  const names = [...new Set((tasks.data?.tasks ?? []).map((t) => t.project))].sort((a, b) =>
    // "(no project)" sinks to the end; the rest alphabetical.
    a === '(no project)' ? 1 : b === '(no project)' ? -1 : a.localeCompare(b),
  )
  // A selected project stays selectable even if its tasks left the read —
  // clearing somebody's scope for them would be the selector deciding.
  const options = project !== '' && !names.includes(project) ? [...names, project] : names

  return (
    <>
      {options.map((p) => (
        <option key={p} value={p}>
          {p}
        </option>
      ))}
    </>
  )
}

/** navSections cuts the table's nav order into the consecutive runs that share
 *  a group label. Order in, order out. */
function navSections(): { label: string; items: RouteDef[] }[] {
  const sections: { label: string; items: RouteDef[] }[] = []
  for (const r of routes.filter((x) => x.nav)) {
    const label = navGroupOf[r.id] ?? ''
    const last = sections.at(-1)
    if (last && last.label === label) last.items.push(r)
    else sections.push({ label, items: [r] })
  }
  return sections
}

/** safeNext keeps a return-to on this origin: an absolute or protocol-relative
 *  URL from the query string is an open redirect, so only a rooted path is
 *  honoured. */
function safeNext(next: string | null): string {
  if (!next || !next.startsWith('/') || next.startsWith('//')) return hrefFor('mission-control')
  return next
}

/** useSession reads identity from GET /api/auth/session and NOWHERE else — no
 *  cookie is readable from JS, and no identity is cached in localStorage. */
function useSession() {
  const [session, setSession] = useState<Session | null>(null)
  const [failure, setFailure] = useState('')

  const reload = useCallback(() => {
    api.session().then(
      (s) => {
        setFailure('')
        setSession(s)
      },
      (err: unknown) => {
        setFailure(
          err instanceof Unreachable
            ? 'The control plane is unreachable — the host may be asleep or the tailnet down.'
            : err instanceof ApiError
              ? `The control plane answered ${err.status}.`
              : 'The control plane could not be read.',
        )
      },
    )
  }, [])

  useEffect(reload, [reload])
  return { session, reload, failure }
}

/**
 * useConnection owns the tab's ONE stream. The shell subscribes with no topics
 * and no event types: it consumes no data, it only reports whether the feed is
 * live — so the indicator is honest on every route.
 */
function useConnection(authed: boolean, injected?: EventStream): Status {
  const [status, setStatus] = useState<Status>('logged-out')
  const streamRef = useRef<EventStream | null>(null)

  useEffect(() => {
    if (!authed) {
      setStatus('logged-out')
      return
    }
    const stream = injected ?? sharedStream()
    streamRef.current = stream
    const unwatch = stream.onStatus(setStatus)
    const unsubscribe = stream.subscribe({
      onResnapshot: (_reason, done) => {
        // The shell holds no state to re-load, so it is immediately caught up.
        done()
      },
    })
    return () => {
      unsubscribe()
      unwatch()
    }
  }, [authed, injected])

  return status
}
