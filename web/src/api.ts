/**
 * The typed client for the surfaces the SHELL consumes: readiness, the S01.9
 * session stack, and nothing else. The S15.2 data families belong to the
 * packets that build their views (B6-5..8).
 *
 * Two rules the whole client rests on:
 *  - The session cookie is HttpOnly, so JS never reads it and never stores a
 *    token. Identity comes from GET /api/auth/session and from nowhere else.
 *  - Every request is same-origin with credentials, because that cookie is the
 *    only thing that authenticates a call (Spec S01.9 layer 3).
 */

export type User = {
  user_id: string
  display_name: string
  role: string
  pin_set: boolean
}

/** The S01.9 layer-2 device hint: the login-picker prefill contract. */
export type DeviceHint = {
  device_login: string
  user_id?: string
  auto_login?: boolean
}

export type Session = {
  authenticated: boolean
  dev?: boolean
  user?: User
  hint?: DeviceHint
}

export type Health = {
  ready: boolean
  mode: string
  version: string
  /** The snapshot-then-tail cursor bootstrap (Spec S15.3). */
  event_head: number
}

/** ApiError carries the HTTP status so callers can branch on 401 alone. */
export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

/** Unreachable is a transport failure — the host is asleep or the tailnet is
 * down. It is a different fact from a server that answered with an error, and
 * the connection indicator shows it as one (Spec S15.12). */
export class Unreachable extends Error {
  constructor(cause: unknown) {
    super('control plane unreachable')
    this.name = 'Unreachable'
    this.cause = cause
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(path, {
      ...init,
      credentials: 'same-origin',
      headers: { Accept: 'application/json', ...(init?.headers ?? {}) },
    })
  } catch (cause) {
    throw new Unreachable(cause)
  }
  if (!res.ok) {
    throw new ApiError(res.status, await errorMessage(res))
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

/** errorMessage prefers the machine surface's own {error, detail} shape and
 * degrades to the status text — it never invents a reason. */
async function errorMessage(res: Response): Promise<string> {
  try {
    const body: unknown = await res.json()
    if (body && typeof body === 'object') {
      const { error, detail } = body as { error?: string; detail?: string }
      if (detail) return detail
      if (error) return error
    }
  } catch {
    // Not JSON. The status is what we have, and saying so is honest.
  }
  return `${res.status} ${res.statusText}`.trim()
}

function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export const api = {
  health: () => request<Health>('/api/health'),
  session: () => request<Session>('/api/auth/session'),
  users: () => request<{ users: User[] }>('/api/auth/users'),
  /** An empty pin is the S01.9 layer-2 grant auto-login attempt. */
  login: (user_id: string, pin: string) => post<{ user_id: string; expires: string }>('/api/auth/login', { user_id, pin }),
  logout: () => post<void>('/api/auth/logout', {}),
  /**
   * The bootstrap window: while `users` is empty one anonymous create is
   * allowed and MUST be an operator (D10 needs a holder from the first user).
   *
   * The answer is 201 with the new id and NO session cookie — creating an
   * account is not signing in, and the caller has to say so.
   */
  createFirstOperator: (user_id: string, display_name: string, pin: string) =>
    post<{ user_id: string }>('/api/auth/users', { user_id, display_name, role: 'operator', pin }),
}
