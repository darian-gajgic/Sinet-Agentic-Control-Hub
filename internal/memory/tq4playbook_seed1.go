package memory

// tq4playbook_seed1.go — the composer playbook content the B3 gate ratified,
// frozen [P3-TQ-4a, A16 2026-09-17].
//
// CONVENTIONS §57: governance content is a SNAPSHOT of what was ratified,
// never a live pointer. EnsureComposerPlaybook used to write whatever
// worker.ComposerPlaybookSeed() returns, which made the governed entry a
// pointer at whatever the code says today — so the moment this packet added
// the web-deliverable section, a fresh world would have written seed-2 content
// under a B3 record that never read it. The seed-1 bytes live here instead,
// and seed-2 enters as its own supersession (tq4governance.go).

// tq4PlaybookSeed1 returns the B3-ratified composer playbook (seed-1),
// byte-for-byte as EnsureComposerPlaybook wrote it before this packet.
func tq4PlaybookSeed1() string {
	return `# Composer playbook — worker-authoring practice

Seed seed-1 (P3-B3-5). Sources: Spec S08.5 (specialization
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
