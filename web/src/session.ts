import { createContext, useContext } from 'react'

import type { Session } from './api'

/**
 * The current identity, as the shell read it from GET /api/auth/session —
 * provided app-wide so a deep surface can answer "who is looking?" without
 * threading a prop through every layer. Read-only: reloading the session
 * stays the shell's job (its own useSession hook).
 *
 * First consumer: the Inbox's read-only acts line (coldwalk W2-4) — a card
 * that cannot be answered because the viewer is the dev fallback must SAY
 * "sign in to act" rather than leave a verbless card posing as a dead end.
 */
export const SessionContext = createContext<Session | null>(null)

export function useSessionInfo(): Session | null {
  return useContext(SessionContext)
}
