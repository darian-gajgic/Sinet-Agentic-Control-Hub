import { useEffect, useState } from 'react'

import { ApiError, Unreachable, api, type Session, type User } from './api'
import { SurfaceHead } from './parts'
import { Button } from './ui'

/**
 * The S01.9 login surface: the user picker, the per-user PIN, the layer-2
 * device-hint prefill, and the bootstrap window.
 *
 * The client holds no credential and no token. The session cookie is HttpOnly,
 * so JS cannot read it; identity comes from GET /api/auth/session and the PIN
 * lives in component state only until the request that spends it returns.
 */
export function Login({ session, onSignedIn }: { session: Session; onSignedIn: () => void }) {
  const [users, setUsers] = useState<User[] | null>(null)
  const [userID, setUserID] = useState('')
  const [pin, setPin] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)

  // The bootstrap-create form's own fields.
  const [newID, setNewID] = useState('')
  const [newName, setNewName] = useState('')

  useEffect(() => {
    let live = true
    api.users().then(
      (r) => {
        if (!live) return
        setUsers(r.users)
        // The device hint PREFILLS the picker (S01.9 layer 2). It never logs
        // anyone in on its own — that is the server's call on the grant.
        const hinted = session.hint?.user_id
        setUserID(hinted && r.users.some((u) => u.user_id === hinted) ? hinted : (r.users[0]?.user_id ?? ''))
      },
      (err: unknown) => {
        if (live) setError(describe(err))
      },
    )
    return () => {
      live = false
    }
  }, [session.hint?.user_id])

  const run = async (fn: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try {
      await fn()
    } catch (err) {
      setError(describe(err))
    } finally {
      // The secret never outlives the request that spent it, on either path.
      setPin('')
      setBusy(false)
    }
  }

  /**
   * Creating the first operator does NOT sign anyone in: the server answers
   * 201 with the new user_id and no session cookie, and the bootstrap window
   * closes the instant the row exists. So the create lands on the picker with
   * the new account selected — one honest step, "now sign in" — instead of
   * re-rendering a form whose window has already shut and whose second submit
   * would fail with a misleading 401.
   */
  const createOperator = () =>
    run(async () => {
      const created = await api.createFirstOperator(newID.trim(), newName.trim(), pin)
      const listed = await api.users()
      setUsers(listed.users)
      setUserID(created.user_id)
      setNotice(`Operator ${created.user_id} created. Sign in with the PIN you just set.`)
    })

  if (users === null) {
    return (
      <section className="panel">
        <SurfaceHead title="Sign in" what={signInWhat} />
        <p className="text-sm text-muted-foreground">{error === '' ? 'Loading…' : error}</p>
      </section>
    )
  }

  // The bootstrap window: while `users` is empty ONE anonymous create is
  // allowed and MUST be an operator — D10 needs a holder from the first user.
  if (users.length === 0) {
    return (
      <section className="panel">
        {/* The bootstrap window's own what-line: it states the window rather
            than the picker, because that is the page in front of the reader,
            and it promises no self-service account creation — once this window
            closes there is none (S01.9). */}
        <SurfaceHead
          title="First run"
          what="No accounts exist yet. The first account is the operator, and this is the only time one can be created without signing in."
        />
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void createOperator()
          }}
        >
          <label>
            User id
            <input value={newID} onChange={(e) => setNewID(e.target.value)} autoComplete="username" required />
          </label>
          <label>
            Display name
            <input value={newName} onChange={(e) => setNewName(e.target.value)} required />
          </label>
          <label>
            PIN
            <input
              type="password"
              value={pin}
              onChange={(e) => setPin(e.target.value)}
              autoComplete="new-password"
              required
            />
          </label>
          <Button type="submit" variant="primary" disabled={busy}>
            Create operator
          </Button>
        </form>
        {error !== '' && <p className="text-sm text-[var(--red)]">{error}</p>}
      </section>
    )
  }

  const autoLogin = session.hint?.auto_login === true && session.hint.user_id === userID

  return (
    <section className="panel">
      <SurfaceHead title="Sign in" what={signInWhat} />
      {notice !== '' && <p className="text-sm text-[var(--green)]">{notice}</p>}
      {session.hint?.device_login && (
        <p className="text-sm text-muted-foreground">
          This device is known as <span className="font-mono tabular-nums">{session.hint.device_login}</span>.
        </p>
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void run(async () => {
            await api.login(userID, pin)
            onSignedIn()
          })
        }}
      >
        <label>
          Account
          <select
            value={userID}
            onChange={(e) => {
              // Switching WHO clears what was typed (review #8): a PIN typed
              // for one account must not sit in the field under another's
              // name on a shared device — and a stale failure line about the
              // previous account clears with it.
              setUserID(e.target.value)
              setPin('')
              setError('')
            }}
          >
            {users.map((u) => (
              <option key={u.user_id} value={u.user_id}>
                {u.display_name}
              </option>
            ))}
          </select>
        </label>
        <label>
          PIN
          <input
            type="password"
            value={pin}
            onChange={(e) => setPin(e.target.value)}
            autoComplete="current-password"
            required={!autoLogin}
          />
        </label>
        {/* The failure, WHERE THE PERSON IS LOOKING (review #8): beside the
            act, as an alert — not a quiet line after the buttons. The PIN
            field is already empty again (`run` clears it on every path), and
            saying so saves a puzzled retry of a blank field. */}
        {error !== '' && (
          <p className="login-error text-sm text-[var(--red)]" role="alert" data-login-error>
            {error} {error.startsWith('That did not') ? 'The PIN field was cleared — type it again.' : ''}
          </p>
        )}
        <Button type="submit" variant="primary" disabled={busy}>
          Sign in
        </Button>
        {autoLogin && (
          <Button
            type="button"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                await api.login(userID, '')
                onSignedIn()
              })
            }
          >
            Sign in with this device
          </Button>
        )}
      </form>
    </section>
  )
}

/**
 * The D8 "what this is" line for the picker (B6 gate §9 A-1).
 *
 * It says only what this page does: the accounts are the household's, you pick
 * yours, and the PIN is checked by the platform. It promises no account
 * creation — outside the bootstrap window above there is none, and a line
 * offering one would be a door that is not there. It promises no remembered
 * sign-in either: a device hint PREFILLS the picker and never signs anybody in,
 * which stays the server's call on the grant.
 *
 * ⚠ CORRECTED 2026-08-05 (P3-UI-6 drain r1, D2). This line first said "this
 * BROWSER keeps no copy of it and no token of its own", and its first clause was
 * FALSE: the PIN input below carries the landed `autoComplete="current-password"`
 * — a pinned shape this packet must not change — which is an explicit invitation
 * to the browser's own password manager to store exactly that. What this page can
 * truthfully speak for is ITSELF, which is also what the file's doc comment at the
 * top claims: the client holds no credential, the PIN is cleared in `finally` on
 * every path, and the session it leaves behind is the server's HttpOnly cookie
 * that JS cannot read. The subject moved from the browser to the page, so the copy
 * claims nothing about software this page does not control.
 */
const signInWhat =
  "Who is signing in on this device. The accounts are the household's — pick yours and enter your PIN. This page stores no credential of its own: your PIN is sent to the platform to be checked and cleared the moment the answer comes back, and what signing in leaves behind is the platform's own session cookie, which this page cannot read."

/** describe keeps the server's collapsed failure collapsed: a login-shaped
 *  failure says only that it failed, because the event log carries the precise
 *  reason and the response deliberately does not (Spec S01.9). */
function describe(err: unknown): string {
  if (err instanceof Unreachable) return 'The control plane is unreachable — the host may be asleep.'
  if (err instanceof ApiError) {
    if (err.status === 401) return 'That did not sign you in.'
    return err.message
  }
  return 'Something went wrong.'
}
