import { useEffect, useState } from 'react'

import { ApiError, Unreachable, api, type Session, type User } from './api'

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
        <h1>Sign in</h1>
        <p className="muted">{error === '' ? 'Loading…' : error}</p>
      </section>
    )
  }

  // The bootstrap window: while `users` is empty ONE anonymous create is
  // allowed and MUST be an operator — D10 needs a holder from the first user.
  if (users.length === 0) {
    return (
      <section className="panel">
        <h1>First run</h1>
        <p className="muted">
          No accounts exist yet. The first account is the operator, and this is the only time one can
          be created without signing in.
        </p>
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
          <button type="submit" disabled={busy}>
            Create operator
          </button>
        </form>
        {error !== '' && <p className="error">{error}</p>}
      </section>
    )
  }

  const autoLogin = session.hint?.auto_login === true && session.hint.user_id === userID

  return (
    <section className="panel">
      <h1>Sign in</h1>
      {notice !== '' && <p className="notice">{notice}</p>}
      {session.hint?.device_login && (
        <p className="muted">This device is known as {session.hint.device_login}.</p>
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
          <select value={userID} onChange={(e) => setUserID(e.target.value)}>
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
        <button type="submit" disabled={busy}>
          Sign in
        </button>
        {autoLogin && (
          <button
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
          </button>
        )}
      </form>
      {error !== '' && <p className="error">{error}</p>}
    </section>
  )
}

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
