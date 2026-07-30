import type { RouteDef } from './routes'

/**
 * A surface that does not exist yet says so, names the packet that will build
 * it, and renders nothing from the API. Honest absence over a blank page or a
 * mocked-up screen that looks real.
 *
 * No route is in that state today — the workforce map was the last one and it
 * landed with B6-8 part B. This stays because the URL contract moves BEFORE the
 * surface does: a route is published so deep links and push payloads can be
 * written against it, and between those two moments this is what answers it.
 */
export function Stub({ route, params }: { route: RouteDef; params: Record<string, string> }) {
  const keys = Object.keys(params)
  return (
    <section className="stub">
      <h1>{route.title}</h1>
      <p className="muted">
        Not built yet — this surface lands with packet <strong>{route.owner}</strong>.
      </p>
      {keys.length > 0 && (
        <p className="muted">
          This URL is already the real one and already carries its identity:{' '}
          {keys.map((k) => `${k}=${params[k]}`).join(', ')}.
        </p>
      )}
    </section>
  )
}

export function NotFound({ pathname }: { pathname: string }) {
  return (
    <section className="stub">
      <h1>No such page</h1>
      <p className="muted">Nothing in this app answers {pathname}.</p>
    </section>
  )
}
