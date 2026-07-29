import { useCallback, useEffect, useState, type MouseEvent, type ReactNode } from 'react'

import { matchRoute, type Match } from './routes'

/**
 * The owned router: the History API, a subscription to `popstate`, and one
 * anchor component. No router library is adopted (OQ6) — S15's need is
 * pathname routes, deep links and bookmarks, and every link below is a real
 * `<a href>`, so a middle-click, a bookmark and a hard reload all work exactly
 * as they would with no JavaScript at all.
 */

const navEvent = 'sinet:navigate'

/** navigate pushes a path and tells the app about it. */
export function navigate(to: string, opts: { replace?: boolean } = {}): void {
  if (opts.replace) window.history.replaceState(null, '', to)
  else window.history.pushState(null, '', to)
  window.dispatchEvent(new Event(navEvent))
}

/** useRoute resolves the current location against the route table. */
export function useRoute(): Match {
  const read = useCallback(() => matchRoute(window.location.pathname), [])
  const [match, setMatch] = useState<Match>(read)

  useEffect(() => {
    const sync = () => setMatch(read())
    window.addEventListener('popstate', sync)
    window.addEventListener(navEvent, sync)
    // A route can change between first render and this effect (a redirect on
    // mount), so resync once rather than trusting the initial read.
    sync()
    return () => {
      window.removeEventListener('popstate', sync)
      window.removeEventListener(navEvent, sync)
    }
  }, [read])

  return match
}

/**
 * Link is a real anchor that intercepts only the plain left-click. Modified
 * clicks fall through to the browser, so "open in new tab" keeps working.
 */
export function Link({
  to,
  children,
  ...rest
}: { to: string; children: ReactNode } & Record<string, unknown>): ReactNode {
  const onClick = (e: MouseEvent<HTMLAnchorElement>) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    e.preventDefault()
    navigate(to)
  }
  return (
    <a href={to} onClick={onClick} {...rest}>
      {children}
    </a>
  )
}
