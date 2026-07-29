import { useCallback, useEffect, useRef, useState } from 'react'

import { ApiError, Unreachable, api, type Session } from './api'
import { ConnectionState } from './ConnectionState'
import { EventStream, sharedStream, type Status } from './events'
import { Login } from './Login'
import { NotFound, Stub } from './Stub'
import { Link, navigate, useRoute } from './router'
import { hrefFor, routes } from './routes'

/**
 * The app shell: one responsive workspace (Spec S1.10 via S15.12), the stable
 * URLs every deep link and push `navigate` field will target (S15.11), the
 * S01.9 session, and the always-visible connection state (S15.12).
 *
 * Every surface beyond this shell is a named stub. Nothing here renders data
 * from the S15.2 families — those views belong to B6-5..8.
 */
export default function App({ stream }: { stream?: EventStream } = {}) {
  const { route, params } = useRoute()
  const { session, reload, failure } = useSession()
  const authed = session?.authenticated === true
  const status = useConnection(authed, stream)

  // Fail closed, both directions: no session outside /login, and no /login
  // once there is one. `next` survives the round trip so a deep link that
  // arrived signed-out still lands where it was going.
  useEffect(() => {
    if (session === null) return
    if (!authed && route.id !== 'login') {
      const next = window.location.pathname + window.location.search
      navigate(`/login?next=${encodeURIComponent(next)}`, { replace: true })
      return
    }
    if (authed && route.id === 'login') {
      const next = new URLSearchParams(window.location.search).get('next')
      navigate(safeNext(next), { replace: true })
    }
  }, [authed, route.id, session])

  return (
    <div className="shell">
      <header className="shell-head">
        <Link to={hrefFor('mission-control')} className="brand">
          Sinet
        </Link>
        <ConnectionState status={status} />
        {authed && (
          <span className="who">
            {session?.user?.display_name ?? (session?.dev === true ? 'dev' : '')}
            <button
              type="button"
              onClick={() => {
                void api.logout().then(reload, reload)
              }}
            >
              Sign out
            </button>
          </span>
        )}
      </header>

      {authed && (
        <nav className="shell-nav" aria-label="Surfaces">
          {routes
            .filter((r) => r.nav)
            .map((r) => (
              <Link key={r.id} to={r.pattern} aria-current={r.id === route.id ? 'page' : undefined}>
                {r.title}
              </Link>
            ))}
        </nav>
      )}

      <main className="shell-main">
        {session === null ? (
          <p className="muted">{failure === '' ? 'Loading…' : failure}</p>
        ) : route.id === 'login' ? (
          <Login session={session} onSignedIn={reload} />
        ) : route.id === 'not-found' ? (
          <NotFound pathname={window.location.pathname} />
        ) : (
          <Stub route={route} params={params} />
        )}
      </main>
    </div>
  )
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
