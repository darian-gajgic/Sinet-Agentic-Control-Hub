/**
 * B6-4 part A ships the tree, the adoption rail and the embed/serve path. The
 * app shell — router with stable URLs, the named surface stubs, login over
 * S01.9, the one-EventSource client, the connection indicator and the
 * responsive skeleton — is part B of this packet. Until it lands, the built
 * SPA says so rather than showing a blank page or faking a surface.
 */
export default function App() {
  return (
    <main className="scaffold">
      <h1>Sinet</h1>
      <p>Control-plane SPA scaffold.</p>
      <p className="muted">
        The app shell (routes, login, live connection) lands with B6-4 part B.
      </p>
    </main>
  )
}
