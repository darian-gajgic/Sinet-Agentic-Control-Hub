import { CircleAlert } from 'lucide-react'

import type { Session } from './api'
import { Login } from './Login'

/**
 * The sign-in-first wall for doors that CREATE work (W1-B1, cold walk
 * 2026-08-17 — release-gating).
 *
 * The trap this kills: under the dev fallback, a task created while browsing
 * was owned by the `dev` identity — and that identity cannot step up at the
 * PIN ceremony (S01.9, correct server behavior), so the task could never be
 * approved by anyone: dev 403s, a real user cannot see it (owner-scoped), and
 * the operator's card is D10 owner-only. Every backend behaved per spec; the
 * flow was the defect. So the wall stands where the work would be BORN: any
 * door that creates approval-gated work presents the sign-in step IN PLACE,
 * before work exists. Signing in reloads the session and the same door
 * unlocks under the same address — the person never leaves the page, which is
 * also the return-to-origin rule enforced by construction.
 *
 * Production is byte-equivalent: `session.dev` is never true without the
 * fallback, so this component never renders there.
 */
export function SignInFirstDoor({
  session,
  onSignedIn,
  doorWords,
}: {
  session: Session
  onSignedIn: () => void
  doorWords: string
}) {
  return (
    <div className="door-signin" data-signin-first>
      <div className="door-refusal" role="note">
        <CircleAlert size={16} strokeWidth={2} aria-hidden="true" />
        <p className="refusal-detail">
          <strong>Sign in before you start.</strong> {doorWords} Work needs an owner — a person who answers its
          questions and approves its plan. You are browsing under the developer fallback, which can look around but
          cannot own or approve anything, so work started here would belong to nobody who can say yes to it. Sign in
          below and this page unlocks in place — you stay exactly where you are.
        </p>
      </div>
      <Login session={session} onSignedIn={onSignedIn} />
    </div>
  )
}
