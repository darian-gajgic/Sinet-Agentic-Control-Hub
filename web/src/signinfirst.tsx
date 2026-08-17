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

/**
 * The same posture for a DATA surface under the dev fallback (re-walk B).
 *
 * The walls started at the doors that CREATE work (W1-B1); the walk showed the
 * exposure's other half — the dev landing could read the household's own
 * numbers (Home's counts, Fleet's per-person money, the inbox queue) before
 * anyone said who they were. So every surface whose content is the household's
 * own records teases its structure — its name and what it holds — and shows
 * the data after sign-in, in place. Production is byte-equivalent: without the
 * fallback `session.dev` is never true and this never renders.
 */
export function SurfaceSignInFirst({
  session,
  onSignedIn,
  title,
  sub,
}: {
  session: Session
  onSignedIn: () => void
  title: string
  sub: string
}) {
  return (
    <section className="surface" data-surface-signin-first={title}>
      <h2 className="mt-0 mb-1">{title} — behind sign-in</h2>
      <p className="mb-3 max-w-prose text-sm text-muted-foreground">
        {sub === '' ? 'This page reads the household’s own records' : sub} — the household&apos;s own records, shown
        to the household. They appear after you say who you are, and this page unlocks in place.
      </p>
      <SignInFirstDoor
        session={session}
        onSignedIn={onSignedIn}
        doorWords={`${title} shows the household's own records.`}
      />
    </section>
  )
}
