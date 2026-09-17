package worker

// playbook.go — the composer playbook seed (Spec S08.6: "the versioned
// best-practice knowledge object that steers the composer"; Spec S09.10:
// an L2 house-scope knowledge object under S09 governance — versioned,
// attributable, removable, updated only through the D10 knowledge gate).
// The CONTENT is owned here (S08 is the owning section; S09 owns how it
// lives, changes, and dies); memory.EnsureComposerPlaybook seeds it as the
// governed house object, and the composer reads the current approved
// version through a seam at the composition root. Seed-content ratification
// is a B3 GATE item; operator edits after ratification are ordinary gated
// new versions — this in-code seed is the initial import only, never a
// live source.

// ComposerPlaybookTopicKey is the S09.2 topic key of the governed object
// (the conflict-lookup key and the composition root's read key).
const ComposerPlaybookTopicKey = "worker/composer-playbook"

// ComposerPlaybookSeedVersion labels the seed content revision (the
// governed entry carries its own S09 version chain after import).
//
// seed-2 [A16, 2026-09-17] adds the web-deliverable feedback section. Its
// reach is INDIRECT and the section says so rather than pretending otherwise:
// the playbook steers the S08.6 composer, the composer drafts the worker
// template, and the template's behavioral content is what reaches an executor.
// There is no direct route today — a house knowledge entry declaring a domain
// or task type injects into nothing, and a selector-free one would inject a
// frontend rule into every task in the house.
const ComposerPlaybookSeedVersion = "seed-2"

// ComposerPlaybookTitle is the governed entry title.
const ComposerPlaybookTitle = "Composer playbook — worker-authoring practice (S08.6 seed, B3-5)"

// ComposerPlaybookSeed returns the seed playbook content (markdown — the
// consumer is the composer's prompt frame, not a machine parser).
func ComposerPlaybookSeed() string {
	return `# Composer playbook — worker-authoring practice

Seed ` + ComposerPlaybookSeedVersion + ` (P3-B3-5). Sources: Spec S08.5 (specialization
content policy), S08.6 (composer contract), S08.1/S08.2 (template format and
guardrail split) — each carrying its ratified research provenance (R15 §2.2,
§4.2–§4.3). Ratification of this content is a B3 gate item; changes go
through the D10 knowledge gate as new versions.

## The one-shot contract

You draft ONE worker in one pass. There is no iteration, no candidate pool,
no tuning against validation results. Draft conservatively: a draft that
fails validation costs a human round; a draft that requests too much power
gets flagged line by line at approval.

## Where specialization pays (in this order — S08.5)

1. **Curated tools scoped to the task class** — the first-order lever.
   Request the SMALLEST tool set the recurring work actually needs; the
   task-class ceiling table in the reference is the maximum, not the goal.
2. **2–3 concise curated skills** — never comprehensive dumps, never
   unreviewed self-generated content (both measured negative). Name only
   skills that exist in the reference.
3. **Domain knowledge deterministically injected** — name knowledge topic
   keys in equipment.knowledge; the platform injects them by manifest.
   Never instruct the worker to go discover its own context.
4. **Output contract + domain rubric ref** — say exactly what the worker
   emits and what "done" means; cite the domain's rubric in eval hooks
   where one exists.
5. **Persona: a tone lever only.** At most the ⚙-capped line count, and
   only where the duty warrants tone (e.g. a brand-voice writer). Long
   personas measurably harm accuracy and add no capability.

## Honesty rules

- **Thin templates in strong-pretraining domains.** For software and
  similar domains the model already knows the craft: write conventions,
  danger zones, and verify commands — not tutorials. More template is NOT
  a better worker.
- **The body steers; it never enforces.** No security prose pretending to
  be enforcement, and NEVER a guardrail-class field (granted tools,
  permissions, confinement, egress, budgets, gates, schedules) anywhere in
  the file — enforcement lives in control-plane rows, and a guardrail
  field in a file is a structural reject.
- **Selectors are deterministic rule inputs.** Family from the ceiling
  table's vocabulary, trigger phrases that literally appear in the
  recurring requests, a delegation-grade description. No cleverness.

## Web deliverables: feedback a person can actually see

A template for work that produces a web page or app says these four things,
because an app whose state is right and whose screen is dead reads to a
person as broken — and the platform's own acceptance walk reads the DOM, so
a template that gets this right is also a template whose work can be checked.

- **Correctness feedback shows on the first frame.** Whatever tells the user
  their action worked must be in the document as soon as the action lands,
  without waiting for an animation to finish. Animation may decorate that;
  it may never be what delivers it.
- **Never gate one view on another view's exit.** A transition that plays
  the outgoing view out before the incoming one mounts leaves the user
  looking at nothing if the outgoing animation stalls. Views mount on their
  own.
- **Respect reduced motion.** Honour the user's reduced-motion preference:
  when it is set, transitions collapse to an immediate state change and
  nothing is lost with them.
- **Every action leaves observable state.** A cart badge is a number in the
  document, not only a pop animation; a saved row is a changed value, not
  only a toast that fades. If the only evidence an action happened is
  motion, the action left no evidence.

## The draft package

Emit all four parts: the template file draft; the requested grants
(smallest set that serves the gap's evidence — grants are requests, the
permission audit and a human approval decide); a dry-run sample task that
is REPRESENTATIVE of the gap's recurring work and completable in one small
session (the requester curates it on the approval card); and a golden-case
note saying what a correct outcome of that sample looks like (it seeds the
worker's golden set, labeled unverified until a real accepted outcome
confirms it).
`
}
