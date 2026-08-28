import { describe, expect, test } from 'vitest'

/**
 * P3-GF12 R9 — the `decision.emission` card is CONSUMED, not merely served.
 *
 * The backend gained one additive card kind: the honest landing when a planner
 * emission is refused and the seam's bounded re-emission is spent. Its body is
 * an ordinary DecisionBody with the ordinary choices, so the ordinary form
 * answers it — and in fact the journey's body-shape fallback would already
 * render it. That fallback is exactly the problem this probe exists to prevent:
 * an unnamed kind renders with the page's apology line ("a card this page has no
 * words of its own for") over a card that has a great deal to say, which is how
 * a requester meets a refusal.
 *
 * So both sites are pinned: the kind line, and the enumeration that routes it to
 * DecisionForm. This is a SOURCE tripwire, deliberately — the card router is not
 * an exported component and this packet does not widen the surface's API to test
 * it (FRONTEND.md's single-author rule governs surfaces). What it proves is that
 * the two lines are there and stay there; what it does not prove is how the card
 * looks, which is the GF9 author's live gauntlet.
 */

const sources = import.meta.glob('./Intake.tsx', { query: '?raw', import: 'default', eager: true }) as Record<
  string,
  string
>
const intakeSource = Object.values(sources)[0] ?? ''

describe('the refused-emission card is consumed by the journey', () => {
  test('Intake.tsx is readable by this probe at all', () => {
    // Without this the two assertions below pass vacuously on an empty string.
    expect(intakeSource.length).toBeGreaterThan(1000)
    expect(intakeSource).toContain('const kindLine: Record<string, string>')
  })

  test('the kind carries its own plain-words line', () => {
    const line = intakeSource.match(/'decision\.emission':\s*\n?\s*'([^']+)'/)
    expect(line, 'decision.emission has no kindLine entry — it would render under the unknown-kind apology').not.toBeNull()
    // Plain words for the person, not the platform's own vocabulary.
    expect(line?.[1]).not.toMatch(/emission|artifact|seam|ErrBadArtifact/i)
  })

  test('the kind is routed to the decision form by name', () => {
    expect(
      intakeSource,
      'decision.emission is not named in the DecisionForm enumeration — it would fall through to the body-shape fallback',
    ).toContain("kind === 'decision.emission'")
  })
})
