import { Component, type ErrorInfo, type ReactNode } from 'react'
import { CircleAlert, RotateCcw } from 'lucide-react'

import { navigate } from './router'
import { hrefFor } from './routes'
import { Button } from './ui'

/**
 * The app-level error boundary (operator finding F1, 2026-08-16): an unhandled
 * render exception used to unmount the whole React root — a BLACK SCREEN that
 * survived SPA navigation and answered only to F5. The never-stall-silently
 * rule makes that the worst possible face, so no crash is allowed to reach the
 * root any more.
 *
 * Three fences, all this one component:
 *
 *  - main.tsx wraps the whole app (`grain="app"`) — the last net. Its recover
 *    act is a full reload, because app-level state is gone with the crash.
 *  - App.tsx wraps the routed view (`grain="view"`) — a crashing surface takes
 *    down ITSELF, never the shell: the sidebar, topbar and every other page
 *    keep working, and the fence RESETS ITSELF when the route changes, so
 *    navigating away always lands on a live page (the F1 report's "browser
 *    back does not recover" can no longer happen).
 *  - App.tsx wraps the task overlay separately — a crashing card must not kill
 *    the surface underneath it.
 *
 * The fallback is a surface in the app's own language, not a browser error
 * page: it says what broke in plain words, keeps the exception's own message
 * as the one technical line (honest, never a wall of stack), and offers the
 * two acts that actually recover — try again in place, or leave for Home.
 */

type BoundaryProps = {
  /** Which fence this is — it sizes the fallback and picks the recover verbs. */
  grain: 'app' | 'view' | 'overlay'
  /**
   * Changes reset the fence: the view fence keys this off the route, so a
   * crash on one page never follows the reader to the next.
   */
  resetKey?: string
  /** The overlay fence closes to the surface underneath. */
  onClose?: () => void
  children: ReactNode
}

type BoundaryState = { error: Error | null }

export class ErrorBoundary extends Component<BoundaryProps, BoundaryState> {
  state: BoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): BoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // The console is this client's only telemetry sink; the boundary must not
    // swallow the fault it caught.
    console.error('sinet: render fault caught by the error boundary', error, info.componentStack)
  }

  componentDidUpdate(prev: BoundaryProps) {
    if (this.state.error !== null && prev.resetKey !== this.props.resetKey) {
      this.setState({ error: null })
    }
  }

  render() {
    const { error } = this.state
    if (error === null) return this.props.children
    const { grain, onClose } = this.props
    const retry = () => {
      this.setState({ error: null })
    }
    return (
      <div className={`fault fault-${grain}`} role="alert" data-boundary={grain}>
        <div className="fault-card">
          <p className="fault-mark" aria-hidden="true">
            <CircleAlert size={26} strokeWidth={2} />
          </p>
          <h2 className="fault-head">
            {grain === 'app' ? 'The app hit a fault' : grain === 'overlay' ? 'This card hit a fault' : 'This page hit a fault'}
          </h2>
          <p className="fault-sub">
            {grain === 'app'
              ? 'Something broke while the app was drawing itself. Nothing about your work is lost — the platform keeps its own records server-side.'
              : 'Something broke while this part was drawing itself. The rest of the app is still running, and nothing about your work is lost — the platform keeps its own records server-side.'}
          </p>
          <p className="fault-detail mono">{error.message !== '' ? error.message : String(error)}</p>
          <div className="fault-acts">
            {grain === 'app' ? (
              <Button
                variant="primary"
                data-fault-act="reload"
                onClick={() => {
                  window.location.reload()
                }}
              >
                <RotateCcw size={14} strokeWidth={2} aria-hidden="true" />
                Reload the app
              </Button>
            ) : (
              <>
                <Button variant="primary" data-fault-act="retry" onClick={retry}>
                  <RotateCcw size={14} strokeWidth={2} aria-hidden="true" />
                  Try again
                </Button>
                {grain === 'overlay' && onClose !== undefined ? (
                  <Button variant="secondary" data-fault-act="close" onClick={onClose}>
                    Close the card
                  </Button>
                ) : (
                  <Button
                    variant="secondary"
                    data-fault-act="home"
                    onClick={() => {
                      retry()
                      navigate(hrefFor('mission-control'))
                    }}
                  >
                    Go to Home
                  </Button>
                )}
              </>
            )}
          </div>
          {grain !== 'app' && (
            <p className="fault-note">
              If it breaks again the same way, the fault is repeatable — the sidebar still works, and every other
              page is unaffected.
            </p>
          )}
        </div>
      </div>
    )
  }
}
