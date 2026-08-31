import { act } from 'react'
import { beforeEach, describe, expect, test } from 'vitest'

/**
 * The give-work door's guided planning journey (P3-GF3-FE; design §2.F).
 *
 * These are BEHAVIOR-CONTRACT probes on the send envelopes and the honesty
 * markers — what one click of "Take this" sends, what a skip sends, what the
 * multi-contest pane sends — written after the design settled on the live
 * walk (FRONTEND.md rule 5: presentation-coupled tests follow the pixels,
 * never precede them). The pixels themselves are judged in real Chrome.
 */
import {
  ClearanceMeter,
  InterviewForm,
  PlanCard,
  ReviewForm,
  UnderstoodPanel,
  deltaOriginWords,
  humanValue,
} from './Intake'
import type { IntakeAnswerBody, IntakeCard, IntakeTaskView } from './api'
import { mount } from './testing'

const click = (el: Element | null) => {
  expect(el, 'the control this probe clicks must exist').not.toBeNull()
  act(() => {
    ;(el as HTMLElement).click()
  })
}

const typeInto = (input: HTMLInputElement | HTMLTextAreaElement, value: string) => {
  const proto = input instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set
  act(() => {
    setter?.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

/** One interview card in the wire's own shape: four questions, the first with
 *  a served suggestion naming its option, the second with a words-only
 *  suggestion, the third with no suggestion at all. */
function interviewCard(): IntakeCard {
  return {
    kind: 'interview',
    task_id: 't-1',
    clearance: 12,
    clearance_floor: 75,
    questions: [
      {
        id: 'collection_semantics',
        text: 'What things does this keep track of?',
        phrased: 'What specific items does this store need to track?',
        why: 'Repeats are the most common mess.',
        suggested: 'Keep both and flag them',
        suggested_option: 'keep_both_flag',
        options: [
          { label: 'Merge them into one', value: 'merge_newest' },
          { label: 'Keep both and flag them', value: 'keep_both_flag' },
        ],
      },
      {
        id: 'comparison_rules',
        text: 'What order should things appear in?',
        suggested: 'Newest parts first, name as the tiebreak',
        options: [{ label: 'Newest first', value: 'newest_first' }],
      },
      { id: 'ordering_atomicity', text: 'Does anything have to happen in strict order?' },
      { id: 'technology_stack', text: 'What should this be built with?' },
    ],
  }
}

let sent: IntakeAnswerBody[] = []
const onAnswer = (b: IntakeAnswerBody) => {
  sent.push(b)
}

beforeEach(() => {
  sent = []
})

describe('the guided interview (design §2.F)', () => {
  test('one click on the recommendation answers in the card\'s own option value', () => {
    const m = mount(<InterviewForm card={interviewCard()} busy={false} onAnswer={onAnswer} />)
    // Q1 is open and carries the one-click take, because its suggestion names
    // one of its own options.
    click(m.container.querySelector('[data-rec-take]'))
    // Q1 collapsed to a settled row wearing the answered state.
    const row = m.container.querySelector('[data-question-row="collection_semantics"]')
    expect(row?.getAttribute('data-state')).toBe('answered')
    m.unmount()
  })

  test('a words-only suggestion is a PRE-FILL offer, never a silent answer', () => {
    const m = mount(<InterviewForm card={interviewCard()} busy={false} onAnswer={onAnswer} />)
    click(m.container.querySelector('[data-rec-take]')) // settle Q1; Q2 opens
    const active = m.container.querySelector('[data-question="comparison_rules"]')
    expect(active?.querySelector('[data-rec-take]')).toBeNull()
    click(active?.querySelector('[data-rec-prefill]') ?? null)
    const own = active?.querySelector('.q-own') as HTMLInputElement
    expect(own.value).toBe('Newest parts first, name as the tiebreak')
    expect(sent).toHaveLength(0) // nothing was sent by either click
    m.unmount()
  })

  test('skip rides the wire as {id, skip:true} with NO value beside it', () => {
    const m = mount(<InterviewForm card={interviewCard()} busy={false} onAnswer={onAnswer} />)
    click(m.container.querySelector('[data-rec-take]')) // Q1 answered
    click(m.container.querySelector('[data-q-skip]')) // Q2 skipped
    // Answer Q3 and Q4 in own words so the send arms.
    for (const q of ['ordering_atomicity', 'technology_stack']) {
      const active = m.container.querySelector(`[data-question="${q}"]`)
      typeInto(active?.querySelector('.q-own') as HTMLInputElement, `my words for ${q}`)
      click(active?.querySelector('[data-q-save]') ?? null)
    }
    click(m.container.querySelector('[data-interview="send"]'))
    expect(sent).toHaveLength(1)
    expect(sent[0].force_proceed).toBeUndefined()
    expect(sent[0].answers).toEqual([
      { id: 'collection_semantics', value: 'keep_both_flag' },
      { id: 'comparison_rules', skip: true },
      { id: 'ordering_atomicity', value: 'my words for ordering_atomicity' },
      { id: 'technology_stack', value: 'my words for technology_stack' },
    ])
    m.unmount()
  })

  test('the send stays disabled with its printed reason until every question is answered or skipped', () => {
    const m = mount(<InterviewForm card={interviewCard()} busy={false} onAnswer={onAnswer} />)
    const send = m.container.querySelector('[data-interview="send"]') as HTMLButtonElement
    expect(send.disabled).toBe(true)
    expect(m.container.textContent).toContain('go through the questions')
    m.unmount()
  })

  test('the escape hatch sends what was settled plus force_proceed', () => {
    const m = mount(<InterviewForm card={interviewCard()} busy={false} onAnswer={onAnswer} />)
    click(m.container.querySelector('[data-rec-take]')) // one answered
    click(m.container.querySelector('[data-interview="force"]'))
    expect(sent).toHaveLength(1)
    expect(sent[0].force_proceed).toBe(true)
    expect(sent[0].answers).toEqual([{ id: 'collection_semantics', value: 'keep_both_flag' }])
    m.unmount()
  })

  test('an unphrased question wears the honest stock marker inside a phrased round', () => {
    const m = mount(<InterviewForm card={interviewCard()} busy={false} onAnswer={onAnswer} />)
    // The round is NOT all-stock (Q1 is phrased), so no round-level line...
    expect(m.container.querySelector('[data-phrasing="canonical-round"]')).toBeNull()
    // ...and the OPEN unphrased question marks itself.
    click(m.container.querySelector('[data-rec-take]'))
    const active = m.container.querySelector('[data-question="comparison_rules"]')
    expect(active?.querySelector('[data-phrasing="canonical"]')).not.toBeNull()
    m.unmount()
  })
})

describe('the review card: "Change my answers" (design §2.D)', () => {
  function reviewCard(): IntakeCard {
    const c = interviewCard()
    c.questions = c.questions?.map((q) => ({ ...q }))
    if (c.questions) {
      c.questions[0].resolution = {
        slot_id: 'collection_semantics',
        name: 'The things it keeps track of',
        how: 'answered',
        value: 'keep_both_flag',
      }
      c.questions[1].resolution = {
        slot_id: 'comparison_rules',
        name: 'What order things come in',
        how: 'assumption',
        assumption: 'newest first, so I went with that',
      }
      c.questions[2].resolution = {
        slot_id: 'ordering_atomicity',
        name: 'What has to happen in order',
        how: 'registry',
        value: 'payment after save',
      }
      // Q4 stays open: still-open rows keep the skip arm in the editor.
    }
    return c
  }

  test('every decision renders with how it was settled; save is disabled at zero changes with its reason', () => {
    const m = mount(<ReviewForm card={reviewCard()} busy={false} onAnswer={onAnswer} />)
    expect(m.container.querySelectorAll('[data-review-row]')).toHaveLength(4)
    expect(m.container.querySelector('[data-review-row="collection_semantics"]')?.textContent).toContain('your answer')
    expect(m.container.querySelector('[data-review-row="comparison_rules"]')?.textContent).toContain('assumed')
    expect(m.container.querySelector('[data-review-row="ordering_atomicity"]')?.textContent).toContain(
      'from the project record',
    )
    expect(m.container.querySelector('[data-review-row="technology_stack"]')?.textContent).toContain('still open')
    const save = m.container.querySelector('[data-review="save"]') as HTMLButtonElement
    expect(save.disabled).toBe(true)
    expect(m.container.textContent).toContain('nothing changed yet')
    m.unmount()
  })

  test('only the changed decisions are sent; keeping a point removes its change', () => {
    const m = mount(<ReviewForm card={reviewCard()} busy={false} onAnswer={onAnswer} />)
    // Open the technology row, answer it in own words, save the change.
    click(m.container.querySelector('[data-review-row="technology_stack"]'))
    const editor = m.container.querySelector('[data-question="technology_stack"]')
    typeInto(editor?.querySelector('.q-own') as HTMLInputElement, 'Python and Django')
    click(editor?.querySelector('[data-q-save]') ?? null)
    // Open the answered row and then KEEP it: no change entry survives.
    click(m.container.querySelector('[data-review-row="collection_semantics"]'))
    click(m.container.querySelector('[data-q-keep]'))
    click(m.container.querySelector('[data-review="save"]'))
    expect(sent).toHaveLength(1)
    expect(sent[0].answers).toEqual([{ id: 'technology_stack', value: 'Python and Django' }])
    expect(sent[0].force_proceed).toBeUndefined()
    m.unmount()
  })

  test('an answered row prefills its editor with the current value', () => {
    const m = mount(<ReviewForm card={reviewCard()} busy={false} onAnswer={onAnswer} />)
    click(m.container.querySelector('[data-review-row="collection_semantics"]'))
    const editor = m.container.querySelector('[data-question="collection_semantics"]')
    // The recorded value names an option, so the option chip is the prefill.
    const picked = editor?.querySelector('.q-option[data-active="true"]')
    expect(picked?.textContent).toContain('Keep both and flag them')
    m.unmount()
  })
})

describe('the plan card verbs (design §2.E/§2.F)', () => {
  function planView(): { view: IntakeTaskView; card: IntakeCard } {
    const card: IntakeCard = {
      kind: 'approval',
      task_id: 't-1',
      approval: {
        layer1: {
          restatement: 'Build a webshop for car parts.',
          assumptions: [{ text: 'A conventional stack is used.', origin: 'assumption:technology_stack' }],
        },
        layer2: {
          acs: [
            { n: 1, plain: 'A visitor can browse parts.' },
            { n: 2, plain: 'Search comes back ordered.' },
          ],
          steps: [
            { id: 'S-1', title: 'Scaffold' },
            { id: 'S-2', title: 'Catalog' },
          ],
        },
        actions: ['approve', 'replan', 'reinterview', 'cancel'],
      },
    }
    const view: IntakeTaskView = { task_id: 't-1', title: 'A webshop', kanban_status: 'intake', owner: 'op' }
    return { view, card }
  }

  test('the understood panel: ordinary rows are correctable, carried-over and escalation rows are read-only with their why', () => {
    // GF9's exception, pinned: an off-card correction is offered ONLY where
    // the wire honors it. A carried-over cross-family item (its slot left the
    // question set at a family switch — the producer appends it with the raw
    // slot id as its display name, intake/pipeline.go understoodBlock) is
    // REFUSED server-side ("slot was not asked"), so no affordance renders
    // and the row says why; an answered escalation is not a slot at all.
    const corrected: Record<string, string> = {}
    const m = mount(
      <UnderstoodPanel
        heading="What it understood so far"
        understood={{
          items: [
            { slot_id: 'look_feel', name: 'Look and feel', how: 'answered', value: 'plain and calm' },
            { slot_id: 'audience_profile', name: 'audience_profile', how: 'assumption', assumption: 'carried over' },
            { slot_id: 'escalation-1', name: 'Which city is the shop in?', how: 'escalation', value: 'Bonn' },
          ],
        }}
        correct={{
          corrected,
          onCorrect: (slot, v) => {
            corrected[slot] = v
          },
          onRevert: () => undefined,
        }}
      />,
    )
    // The ordinary row wears its fix affordance…
    expect(m.container.querySelector('[data-understood-fix="look_feel"]')).not.toBeNull()
    // …the carried row does not, and says why in plain words with no raw id
    // as its display name…
    const carried = m.container.querySelector('li[data-carried]')!
    expect(carried, 'the carried-over row lost its marker').not.toBeNull()
    expect(carried.querySelector('.understood-fix')).toBeNull()
    expect(carried.textContent).toContain('kept from before the kind of work changed')
    expect(carried.querySelector('.understood-name')?.textContent).toBe('audience profile')
    // …and the escalation row is read-only too.
    expect(m.container.querySelector('[data-understood-fix="escalation-1"]')).toBeNull()
    m.unmount()
  })

  test('a redraft opens with WHAT CHANGED between the two served cards (RA-1), framed as this page’s own comparison', () => {
    // The RA-1 promise: "Change the plan" says "shows exactly what changed",
    // and pre-approval the pipeline serves no delta card — so the page
    // compares the two cards it rendered, and says the comparison is its
    // own. Live driving hit a backend wall (the reviser's redraft blows the
    // A15 approach cap and tombstones — reported), so the block is pinned
    // here over two served-shaped snapshots.
    const { view, card } = planView()
    const next: IntakeCard = JSON.parse(JSON.stringify(card)) as IntakeCard
    next.approval!.layer1.restatement = 'Build a webshop for car parts, with the phone number shown.'
    next.approval!.layer2!.acs = [
      { n: 1, plain: 'A visitor can browse parts.' },
      { n: 2, plain: 'Search comes back ordered by relevance.' },
      { n: 3, plain: 'The phone number is visible on every page.' },
    ]
    next.approval!.layer2!.steps = [{ id: 'S-1', title: 'Scaffold' }] // S-2 removed
    const m = mount(<PlanCard view={view} card={next} busy={false} previous={card} onAnswer={onAnswer} />)
    const block = m.container.querySelector('[data-plan="changed"]')!
    expect(block, 'no what-changed block on a redraft').not.toBeNull()
    const text = block.textContent ?? ''
    // The restatement change is legible old → now.
    expect(text).toContain('Build a webshop for car parts.')
    expect(text).toContain('with the phone number shown')
    // A changed check, an added check, and the REMOVED step all render — a
    // silently disappearing item is the exact thing this block exists to end.
    expect(text).toContain('ordered by relevance')
    expect(text).toContain('The phone number is visible on every page.')
    expect(text).toContain('Catalog')
    expect(block.querySelector('[data-change-kind="removed"]')).not.toBeNull()
    // The framing: the comparison is the page's, the cards the platform's.
    // (Reworded in the GF9 drain — review L3 read the old sentence as
    // garbled; the framing CLAIM is what this pin protects.)
    expect(text).toContain('the comparison between them is this page')
    m.unmount()
  })

  test('a first draft and a cold resume carry NO what-changed block — only what this browser saw is compared', () => {
    const { view, card } = planView()
    const m = mount(<PlanCard view={view} card={card} busy={false} onAnswer={onAnswer} />)
    expect(m.container.querySelector('[data-plan="changed"]')).toBeNull()
    m.unmount()
  })

  test('an unchanged redraft says so honestly instead of rendering an empty diff', () => {
    const { view, card } = planView()
    const same: IntakeCard = JSON.parse(JSON.stringify(card)) as IntakeCard
    const m = mount(<PlanCard view={view} card={same} busy={false} previous={card} onAnswer={onAnswer} />)
    const block = m.container.querySelector('[data-plan="changed"]')!
    expect(block).not.toBeNull()
    expect(block.querySelector('[data-plan-unchanged]')?.textContent).toContain('Nothing this comparison covers moved')
    m.unmount()
  })

  test('every verb states its consequence beside its plain name', () => {
    const { view, card } = planView()
    const m = mount(<PlanCard view={view} card={card} busy={false} onAnswer={onAnswer} />)
    const verbs = m.container.querySelector('[data-plan-verbs]')
    expect(verbs?.textContent).toContain('Approve: start the work')
    expect(verbs?.textContent).toContain('Change the plan')
    expect(verbs?.textContent).toContain('Change my answers')
    expect(verbs?.textContent).toContain('locks this plan in and begins')
    expect(verbs?.textContent).toContain('reopens every question with your current answers filled in')
    m.unmount()
  })

  test('contest-in-place sends ANY number of tapped fields, each with ITS OWN note, in ONE send', () => {
    // REWRITTEN 2026-08-27 (P3-GF9): the contest moved IN PLACE — every field
    // of the plan wears its tap where the field is (r5 §C rule 7), and an
    // armed field opens its own note box (the wire's per-target note). The
    // send contract is the same one verb, one redraft.
    const { view, card } = planView()
    const m = mount(<PlanCard view={view} card={card} busy={false} onAnswer={onAnswer} />)
    click(m.container.querySelector('[data-plan-act="replan"]'))
    const tap = (target: string) =>
      click(m.container.querySelector(`[data-contest-item="${target}"] .contest-tap`))
    tap('S-2')
    tap('AC-2')
    // The per-target note lands on the field it is about.
    typeInto(
      m.container.querySelector('[data-contest-note-for="S-2"]') as HTMLInputElement,
      'skip the animations here',
    )
    const note = m.container.querySelector('[data-field="contest-note"]') as HTMLTextAreaElement
    typeInto(note, 'softer colors, in my own words')
    click(m.container.querySelector('[data-plan-act="send-replan"]'))
    expect(sent).toHaveLength(1)
    expect(sent[0]).toEqual({
      action: 'replan',
      contests: [{ target: 'S-2', note: 'skip the animations here' }, { target: 'AC-2' }],
      note: 'softer colors, in my own words',
    })
    m.unmount()
  })

  test('leaving contest mode KEEPS the armed taps (RA-12), and the verb says what it holds', () => {
    const { view, card } = planView()
    const m = mount(<PlanCard view={view} card={card} busy={false} onAnswer={onAnswer} />)
    click(m.container.querySelector('[data-plan-act="replan"]'))
    click(m.container.querySelector('[data-contest-item="S-2"] .contest-tap'))
    click(m.container.querySelector('[data-plan-act="contest-back"]'))
    // Back on the plan: nothing fired, and the Re-plan verb carries the count.
    expect(sent).toHaveLength(0)
    expect(m.container.querySelector('[data-plan-act="replan"]')?.textContent).toContain('1 tapped')
    click(m.container.querySelector('[data-plan-act="replan"]'))
    click(m.container.querySelector('[data-plan-act="send-replan"]'))
    expect(sent[0]).toEqual({ action: 'replan', contests: [{ target: 'S-2' }] })
    m.unmount()
  })

  test('a note alone is enough; an empty pane stays disabled with its reason', () => {
    const { view, card } = planView()
    const m = mount(<PlanCard view={view} card={card} busy={false} onAnswer={onAnswer} />)
    click(m.container.querySelector('[data-plan-act="replan"]'))
    const send = m.container.querySelector('[data-plan-act="send-replan"]') as HTMLButtonElement
    expect(send.disabled).toBe(true)
    expect(m.container.textContent).toContain('tap what is wrong above, or say it in words')
    typeInto(m.container.querySelector('[data-field="contest-note"]') as HTMLTextAreaElement, 'do it my way')
    click(m.container.querySelector('[data-plan-act="send-replan"]'))
    expect(sent).toHaveLength(1)
    expect(sent[0]).toEqual({ action: 'replan', note: 'do it my way' })
    m.unmount()
  })

  test('"Change my answers" fires the reinterview verb directly', () => {
    const { view, card } = planView()
    const m = mount(<PlanCard view={view} card={card} busy={false} onAnswer={onAnswer} />)
    click(m.container.querySelector('[data-plan-act="reinterview"]'))
    expect(sent).toEqual([{ action: 'reinterview' }])
    m.unmount()
  })
})

describe('the GF9 drain pins (review M4/M5/L11)', () => {
  const skipSentence =
    'Examples to follow: you skipped this one, so I am going with what was suggested on the card: match the shop tone'

  test('an option-backed answer SHOWS the label the person clicked; its editor seeds the machine value (M4)', () => {
    const corrected: Record<string, string> = {}
    const m = mount(
      <UnderstoodPanel
        heading="What it understood so far"
        understood={{
          items: [{ slot_id: 'tone_voice', name: 'Tone and voice', how: 'answered', value: 'warm', label: 'Warm and personal' }],
        }}
        correct={{
          corrected,
          onCorrect: (slot, v) => {
            corrected[slot] = v
          },
          onRevert: () => undefined,
        }}
      />,
    )
    // The row reads as the option the person clicked, never the token.
    expect(m.container.querySelector('.understood-value')?.textContent).toBe('Warm and personal')
    // The editor seeds the MACHINE value — the label is display-only and
    // must never ride back as the answer.
    click(m.container.querySelector('[data-understood-fix="tone_voice"]'))
    const input = m.container.querySelector('[data-understood-editor="tone_voice"] input') as HTMLInputElement
    expect(input.value).toBe('warm')
    m.unmount()
  })

  test('a skipped slot’s row shows the assumed VALUE; the narration is provenance and never seeds the editor (M5)', () => {
    const corrected: Record<string, string> = {}
    const m = mount(
      <UnderstoodPanel
        heading="What it understood so far"
        understood={{
          items: [{ slot_id: 'references', name: 'Examples to follow', how: 'assumption', assumption: skipSentence }],
        }}
        correct={{
          corrected,
          onCorrect: (slot, v) => {
            corrected[slot] = v
          },
          onRevert: () => undefined,
        }}
      />,
    )
    expect(m.container.querySelector('.understood-value')?.textContent).toBe('match the shop tone')
    expect(m.container.querySelector('.understood-assume-why')?.textContent).toContain('you skipped this one')
    click(m.container.querySelector('[data-understood-fix="references"]'))
    const input = m.container.querySelector('[data-understood-editor="references"] input') as HTMLInputElement
    // One edit-slip from submitting boilerplate is exactly the M5 hazard:
    // the seed is the value alone, never the sentence.
    expect(input.value).toBe('match the shop tone')
    m.unmount()
  })

  function planWith(assumptions: { text: string; origin?: string }[]): { view: IntakeTaskView; card: IntakeCard } {
    const card: IntakeCard = {
      kind: 'approval',
      task_id: 't-1',
      approval: {
        layer1: {
          restatement: 'A short note.',
          understood: {
            items: [{ slot_id: 'references', name: 'Examples to follow', how: 'assumption', assumption: skipSentence }],
          },
          assumptions,
        },
        actions: ['approve'],
      },
    }
    return { view: { task_id: 't-1', title: 'A note', kanban_status: 'intake', owner: 'op' }, card }
  }

  test('the skipped question’s assumption ships ONCE: the platform’s template row drops when the planner’s prose twin names the slot (M5/W7)', () => {
    const { view, card } = planWith([
      {
        text: 'The copy follows the shop tone of voice.',
        origin: 'You skipped the question about examples to follow, so I am going with the suggested default.',
      },
      { text: skipSentence, origin: 'slot:references' },
    ])
    const m = mount(<PlanCard view={view} card={card} busy={false} onAnswer={onAnswer} />)
    const sec = m.container.querySelector('[data-plan="assumptions"]')!
    expect(sec.textContent).toContain('The copy follows the shop tone of voice.')
    // The template twin is the duplicate and drops — its sentence appears
    // nowhere in the assumption list.
    expect(sec.textContent).not.toContain('so I am going with what was suggested')
    m.unmount()
  })

  test('a template row with NO planner twin still renders — transformed: plain name, the value, the narration as provenance, no raw token (M5)', () => {
    const { view, card } = planWith([{ text: skipSentence, origin: 'slot:references' }])
    const m = mount(<PlanCard view={view} card={card} busy={false} onAnswer={onAnswer} />)
    const sec = m.container.querySelector('[data-plan="assumptions"]')!
    const text = sec.textContent ?? ''
    expect(text).toContain('Examples to follow — match the shop tone')
    expect(text).toContain('you skipped this one, so it went with what was suggested on the card')
    // The raw slot token never surfaces; the question’s plain name stands.
    expect(text).not.toContain('"references"')
    expect(text).not.toContain('so I am going with what was suggested')
    m.unmount()
  })

  test('while the PIN step-up is armed the verb list folds to one pointer (L11)', () => {
    const { view, card } = planWith([])
    const m = mount(<PlanCard view={view} card={card} busy={false} pinArmed onAnswer={onAnswer} />)
    expect(m.container.querySelector('[data-plan-verbs]'), 'a second live verb set beside the armed panel').toBeNull()
    const note = m.container.querySelector('[data-plan="armed-note"]')
    expect(note?.textContent).toContain('armed just below')
    expect(note?.textContent).toContain('nothing has happened yet')
    m.unmount()
  })
})

describe('the meter and the plain words', () => {
  test('the clearance meter explains itself with the SERVED floor', () => {
    const m = mount(<ClearanceMeter value={12.1212} floor={75} />)
    const el = m.container.querySelector('.clearance')
    expect(el?.getAttribute('data-clearance')).toBe('12.1212')
    expect(el?.getAttribute('data-clearance-floor')).toBe('75')
    expect(el?.textContent).toContain('12')
    expect(el?.textContent).toContain('of the 75 needed')
    expect(el?.getAttribute('title')).toContain('questions stop once it reaches 75')
    expect(m.container.querySelector('.clearance-stop')).not.toBeNull()
    m.unmount()
  })

  test('without a served floor the meter keeps the landed /100 words and grows no tick', () => {
    const m = mount(<ClearanceMeter value={40} />)
    expect(m.container.textContent).toContain('/100 settled')
    expect(m.container.querySelector('.clearance-stop')).toBeNull()
    m.unmount()
  })

  test('machine tokens read as words; prose passes through untouched', () => {
    expect(humanValue('keep_both_flag')).toBe('keep both flag')
    expect(humanValue('Keep both and flag them for me to look at')).toBe('Keep both and flag them for me to look at')
    expect(humanValue('newest')).toBe('newest')
  })

  test('delta origins map to reader words and unknown ones render as themselves (§42)', () => {
    expect(deltaOriginWords('contested_card')).toBe('you asked for changes')
    expect(deltaOriginWords('freshness_revalidation')).toContain('re-checked')
    expect(deltaOriginWords(undefined)).toBe('the cause was not recorded')
    expect(deltaOriginWords('somebody_new')).toBe('somebody_new')
  })
})
