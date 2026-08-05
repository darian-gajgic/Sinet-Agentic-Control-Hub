import { useCallback, useEffect, useRef, useState } from 'react'
import {
  BookMarked,
  Columns3,
  Inbox as InboxIcon,
  LayoutDashboard,
  MessagesSquare,
  Server,
  SlidersHorizontal,
  Users,
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
import { Memory, MemoryEntryView } from './Memory'
import { Settings } from './Settings'
import { MissionControl } from './MissionControl'
import { TaskDetail } from './TaskDetail'
import { NotFound, Stub } from './Stub'
import { Link, navigate, useRoute } from './router'
import { ToastProvider } from './ui'
import { TurnToasts, useEtiquette } from './etiquette'
import { TourButton, TourOverlay, tourSteps, useTour } from './tour'
import { hrefFor, routes, type RouteDef, type RouteID } from './routes'
import { Workforce } from './Workforce'

/**
 * Sidebar grouping is PRESENTATION and lives here, not in the route table:
 * `routes.ts` owns ids, patterns, titles and nav flags, and this packet leaves
 * every one of them byte-unchanged.
 *
 * Sections are the CONSECUTIVE RUNS of this label over the table's own nav
 * order, which is what makes grouping incapable of reordering or dropping a
 * link: every navigable route is walked exactly once, in table order, and one
 * with no label here still renders — in its own unlabelled run.
 */
const navGroupOf: Partial<Record<RouteID, string>> = {
  'mission-control': 'Command',
  board: 'Command',
  fleet: 'Command',
  inbox: 'Decisions',
  settings: 'System',
  chat: 'Intelligence',
  workforce: 'Intelligence',
  memory: 'Intelligence',
}

const navIcons: Partial<Record<RouteID, LucideIcon>> = {
  'mission-control': LayoutDashboard,
  board: Columns3,
  fleet: Server,
  inbox: InboxIcon,
  settings: SlidersHorizontal,
  chat: MessagesSquare,
  workforce: Users,
  memory: BookMarked,
}

/**
 * The app shell: one responsive workspace (Spec S1.10 via S15.12), the stable
 * URLs every deep link and push `navigate` field will target (S15.11), the
 * S01.9 session, and the always-visible connection state (S15.12).
 *
 * EVERY ROUTE IN THE TABLE IS BUILT. The workforce map was the last stub and it
 * landed with B6-8 part B, so no URL this SPA publishes answers with a
 * not-built-yet page. `Stub` stays as the terminal arm below because a later
 * packet publishes its route BEFORE it builds the surface — the URL contract is
 * what deep links and push payloads are written against, so it moves first — and
 * a route in that state has to say so rather than render blank. A stub reads
 * nothing from the API: honest absence, never a mocked screen that looks real.
 */
export default function App({ stream }: { stream?: EventStream } = {}) {
  const { route, params } = useRoute()
  const { session, reload, failure } = useSession()
  const authed = session?.authenticated === true
  const status = useConnection(authed, stream)
  // CENSUS HUNK (b) — background-run etiquette (P3-UI-7 R15). It joins the
  // stream `useConnection` already opened; it opens nothing, adds no topic and
  // reads nothing. The dot is cleared by arriving on the route.
  const { dots, requests } = useEtiquette(route.id, authed, stream)
  // CENSUS HUNK (c) — the guided tour (P3-UI-7 R13). It NEVER auto-starts:
  // nothing in this client persists, so a self-starting tour would ambush every
  // load. `TourButton` below is the only thing that can start it.
  const tour = useTour()

  // Fail closed, both directions: no session outside /login, and no /login
  // once there is one. `next` survives the round trip so a deep link that
  // arrived signed-out still lands where it was going.
  //
  // THE DEV-FALLBACK CARVE-OUT (B6 gate §9 finding C-1, fixed at P3-UI-4).
  //
  // `SessionAuthenticator` resolves EVERY session-less request in dev posture to
  // the fixed dev identity (internal/api/identity.go:83–97), so `authed` is
  // permanently true there. The bounce below therefore made /login unreachable
  // in dev and `api.logout()` a no-op — the re-read simply resolved to the dev
  // identity again — defeating the identity layer's own stated intent one layer
  // up ("sessions still win when present, so the full login flow is exercisable
  // in dev", identity.go:74–76).
  //
  // `session.dev === true` is the wire's own marker for that fallback and for
  // nothing else: the served shape is `{authenticated:true, dev:true}` with NO
  // `user` object, where a real session serves `{authenticated:true, user:{…}}`
  // with `dev` omitted (internal/api/auth_handlers.go:91–116). Real cookies
  // always win, so a dev-posture person who signs in stops matching this arm
  // immediately. PRODUCTION IS BYTE-EQUIVALENT: without the fallback `dev` is
  // never true, so both arms behave exactly as before.
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
    // CENSUS HUNK (a) — `ToastProvider`'s ONE production mount (P3-UI-7 R16;
    // named as UI-7's by both §48's and §51's seam lists). It wraps the whole
    // shell tree, so every test that mounts <App> inherits it and no surface
    // has to provide its own. `ui/Toast.tsx` is byte-frozen: the cap of four
    // and the `data-limited` overflow are the kit's, asserted at this mount.
    <ToastProvider>
      <div className="shell">
        <div className="aurora" aria-hidden="true" />
        {/* The toast sink lives inside the provider because `useToast` reads
            its manager. It subscribes to nothing — see `useEtiquette`. */}
        <TurnToasts requests={requests} />

      <aside className="shell-side">
        <Link to={hrefFor('mission-control')} className="brand">
          <span className="brand-mark" aria-hidden="true" />
          Sinet
        </Link>

        {authed && (
          <nav className="shell-nav" aria-label="Surfaces">
            {navSections().map((section) => (
              <div className="nav-group" key={section.label || section.items[0].id}>
                {section.label !== '' && <span className="nav-group-label">{section.label}</span>}
                {section.items.map((r) => {
                  const Icon = navIcons[r.id]
                  return (
                    <Link
                      key={r.id}
                      to={r.pattern}
                      aria-current={r.id === route.id ? 'page' : undefined}
                      // The dot says "something arrived here while you were
                      // elsewhere" and NOTHING about how much: no count can be
                      // derived from frames without risking disagreement with
                      // the queue the person then opens (§42).
                      data-dot={dots[r.id] === true ? 'true' : undefined}
                    >
                      {Icon && <Icon className="nav-icon" size={16} strokeWidth={1.7} aria-hidden="true" />}
                      {r.title}
                      {dots[r.id] === true && (
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
        )}
      </aside>

      <div className="shell-body">
        <header className="shell-head">
          <ConnectionState status={status} />
          {authed && <TourButton onStart={tour.start} />}
          {authed && (
            <span className="who">
              {session?.user?.display_name ?? (session?.dev === true ? 'dev' : '')}
              {/* The affordance is whichever one can actually WORK, and the two
                  server states are the only two rendered (C-1). Under the dev
                  fallback there is no session to revoke, so a Sign out would
                  clear nothing and leave the same identity resolved on the
                  re-read — a control that cannot honour its own label. What is
                  true there is that nobody is signed in yet, so the offer is the
                  one act that changes it, pointing at the picker the carve-out
                  above now lets render. This is not a synthesized "signed out"
                  state: the dev fallback is the ABSENCE of a person, with
                  browsing rights the server grants and this client never
                  second-guesses. */}
              {devFallback ? (
                <Link to={hrefFor('login')} data-auth="sign-in">
                  Sign in
                </Link>
              ) : (
                <button
                  type="button"
                  data-auth="sign-out"
                  onClick={() => {
                    void api.logout().then(reload, reload)
                  }}
                >
                  Sign out
                </button>
              )}
            </span>
          )}
        </header>

        <main className="shell-main">
          {session === null ? (
            <p className="muted">{failure === '' ? 'Loading…' : failure}</p>
          ) : route.id === 'login' ? (
            <Login session={session} onSignedIn={reload} />
          ) : route.id === 'not-found' ? (
            <NotFound pathname={window.location.pathname} />
          ) : route.id === 'mission-control' ? (
            // The personal filters are `/?view=…` on this surface: stable,
            // bookmarkable URLs that fill the route table rather than renaming it.
            <MissionControl stream={stream} me={session.user?.user_id ?? ''} search={window.location.search} />
          ) : route.id === 'board' ? (
            // The caller's own identity decides what is drag-reorderable, and the
            // server refuses the rest: the operator is not excepted from "your
            // own queued work" (S15.5).
            <Board me={session.user?.user_id ?? ''} stream={stream} />
          ) : route.id === 'fleet' ? (
            // The caller's own identity decides which pause switches and budget
            // editors render. The operator limb is DEV-INCLUSIVE because it
            // mirrors the server's own `isOperatorRead` — the predicate the read
            // on the other side of those verbs used — which is deliberately not
            // the users-row-role-only flag the memory surface takes below.
            <Fleet
              me={session.user?.user_id ?? ''}
              operator={session.dev === true || session.user?.role === 'operator'}
              stream={stream}
            />
          ) : route.id === 'task' ? (
            <TaskDetail id={params.id} stream={stream} />
          ) : route.id === 'inbox' ? (
            <Inbox stream={stream} />
          ) : route.id === 'inbox-item' ? (
            // The id arrives DECODED from the route table, which matters: real
            // card ids carry ':', '#' and a unit separator, so the round trip
            // through hrefFor/matchRoute is the only thing that keeps a deep
            // link pointing at the card it names (S15.11).
            <InboxItem id={params.id} stream={stream} />
          ) : route.id === 'settings' ? (
            <Settings stream={stream} />
          ) : route.id === 'chat' ? (
            // The open conversation is `/chat?session=…` — a stable, bookmarkable,
            // push-`navigate`-able URL that fills the route table rather than
            // adding to it, exactly as the personal filters do on mission control.
            <Chat stream={stream} search={window.location.search} />
          ) : route.id === 'deliverable' ? (
            // The caller's own identity decides whether the ACCEPT FORM renders:
            // an accept is the owner's own outward act under their own credentials,
            // so a non-owner (the operator included) reads the card and cannot act
            // on it. That is presentation over a served body — the server refuses
            // the verb regardless (S15.2).
            <Deliverable id={params.id} me={session.user?.user_id ?? ''} stream={stream} />
          ) : route.id === 'workforce' ? (
            // View-only by construction (S15.10 parks editing to 15.5): the map
            // takes no identity prop because it offers no act. What each caller
            // may SEE is decided server-side and the read says out loud which
            // reading it returned.
            <Workforce stream={stream} />
          ) : route.id === 'memory' ? (
            // The caller's own identity decides which of the four gate verbs
            // render, and the doors DIFFER: a correction and a retirement are
            // owner-or-operator, a true deletion is strictly the owner's, and a
            // house entry is the operator's to write. The role bit here is the
            // users-row one, because that is what the S09 gate resolves the
            // actor against — it opens nothing of another person's user-scope
            // store, which is a content line and not the §30 telemetry one.
            <Memory
              me={session.user?.user_id ?? ''}
              operator={session.user?.role === 'operator'}
              stream={stream}
            />
          ) : route.id === 'memory-entry' ? (
            <MemoryEntryView
              id={params.id}
              me={session.user?.user_id ?? ''}
              operator={session.user?.role === 'operator'}
              stream={stream}
            />
          ) : (
            <Stub route={route} params={params} />
          )}
        </main>
      </div>

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
    </ToastProvider>
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
        // The session route is pre-session, so a 401 here is not a thing; a
        // failure means the control plane is unreachable or broken, and the
        // shell says which rather than showing a login form that cannot work.
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
 * live — so the indicator is honest on every route, including the stubs that
 * render nothing.
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
        // A view with state answers this by re-reading its REST snapshot and
        // calling done() when that read lands — clearing its own debt only.
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
