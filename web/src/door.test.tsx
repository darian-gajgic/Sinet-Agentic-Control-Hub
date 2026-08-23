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

  test('the contest pane sends ANY number of tapped targets plus the note in ONE send', () => {
    const { view, card } = planView()
    const m = mount(<PlanCard view={view} card={card} busy={false} onAnswer={onAnswer} />)
    click(m.container.querySelector('[data-plan-act="replan"]'))
    const chips = [...m.container.querySelectorAll('.contest-target')]
    click(chips.find((c) => c.textContent?.includes('S-2')) ?? null)
    click(chips.find((c) => c.textContent?.includes('AC-2')) ?? null)
    const note = m.container.querySelector('[data-field="contest-note"]') as HTMLTextAreaElement
    typeInto(note, 'softer colors, in my own words')
    click(m.container.querySelector('[data-plan-act="send-replan"]'))
    expect(sent).toHaveLength(1)
    expect(sent[0]).toEqual({
      action: 'replan',
      contests: [{ target: 'S-2' }, { target: 'AC-2' }],
      note: 'softer colors, in my own words',
    })
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
