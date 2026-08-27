---
kind: plan
task: t-0
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-0 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C1
   - Writes: src/**
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - New spend
   - New tools: ripgrep
2. **S-2**: Implement the change
   - Done when: the note exists
   - Confinement: C2
   - Writes: src/**
   - New spend
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-1
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-1 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C2
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: docs/*.md
   - New spend
   - Credential touch
   - New tools: ripgrep
   - Research node

## Coverage map

- AC-1 → S-1

## Research nodes

- [P47-7] → S-1: verify the current version

## Cited knowledge

- project:conventions

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-2
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-2 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Implement the change
   - Done when: the checks are green
   - Confinement: C0
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - New spend
   - Credential touch
2. **S-2**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C1
   - Reads: docs/*.md
   - Outward effects: publishes the note
   - Credential touch
   - Research node
3. **S-3**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: docs/*.md
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: docs/*.md
   - Shared-asset write
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-3
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-3 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C2
   - Writes: src/**
   - Reads: docs/*.md
   - New tools: ripgrep
2. **S-2**: Verify against the criteria
   - Done when: tests pass
   - Confinement: C1
   - Reads: app/**
3. **S-3**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: docs/*.md
   - Writes: UNBOUNDED (whole-project claim, S02.8)

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-4
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-4 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: UNBOUNDED (whole-project claim, S02.8)
2. **S-2**: Collect the merged changes
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Reads: src/**
   - Outward effects: publishes the note
   - New spend
   - Shared-asset write
   - New tools: ripgrep
   - Research node
3. **S-3**: Verify against the criteria
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Writes: docs/*.md
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Outward effects: publishes the note
   - New tools: ripgrep
4. **S-4**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C2
   - Writes: app/**
   - Outward effects: publishes the note
   - New spend
   - Shared-asset write
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-5
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-5 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: app/**
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - New spend
   - Credential touch
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1

## Cited knowledge

- project:conventions

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-6
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-6 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Implement the change
   - Done when: tests pass
   - Confinement: C2
   - New spend
   - Research node
2. **S-2**: Collect the merged changes
   - Done when: the note exists
   - Confinement: C1
   - Writes: docs/*.md
3. **S-3**: Write the note
   - Done when: tests pass
   - Confinement: C2
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Research nodes

- [P47-7] → S-1: verify the current version

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-7
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-7 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Write the note
   - Done when: the note exists
   - Confinement: C1
   - Writes: src/**
2. **S-2**: Verify against the criteria
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Writes: app/**
   - Shared-asset write
3. **S-3**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C0
   - Reads: src/**

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-8
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-8 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Write the note
   - Done when: tests pass
   - Confinement: C1
   - Writes: app/**
   - New tools: ripgrep
2. **S-2**: Implement the change
   - Done when: the note exists
   - Confinement: C0
   - Outward effects: publishes the note

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-9
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-9 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: tests pass
   - Confinement: C0
   - Writes: src/**

## Coverage map

- AC-1 → S-1

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-10
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-10 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C2
   - Writes: docs/*.md
   - Reads: app/**
   - New tools: ripgrep
   - Research node
2. **S-2**: Write the note
   - Done when: the note exists
   - Confinement: C1
   - Writes: app/**
   - Reads: app/**
3. **S-3**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C2
   - Outward effects: publishes the note
   - New spend
4. **S-4**: Collect the merged changes
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Reads: app/**

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Research nodes

- [P47-7] → S-1: verify the current version

## Cited knowledge

- project:conventions

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-11
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-11 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Writes: src/**
   - New spend
2. **S-2**: Implement the change
   - Done when: the checks are green
   - Confinement: C1
   - New tools: ripgrep
   - Research node
3. **S-3**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C0
   - Reads: docs/*.md
   - Outward effects: publishes the note
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-12
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-12 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Implement the change
   - Done when: the checks are green
   - Confinement: C0
   - Writes: src/**
   - Reads: src/**
   - Outward effects: publishes the note
   - New spend
   - New tools: ripgrep
2. **S-2**: Collect the merged changes
   - Done when: the checks are green
   - Confinement: C0
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: app/**
3. **S-3**: Implement the change
   - Done when: the checks are green
   - Confinement: C1

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Cited knowledge

- project:conventions

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-13
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-13 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Reads: app/**
   - Shared-asset write
   - Research node
2. **S-2**: Verify against the criteria
   - Done when: tests pass
   - Confinement: C0
   - Writes: docs/*.md
   - Reads: app/**
   - New spend
3. **S-3**: Implement the change
   - Done when: the note exists
   - Confinement: C2
   - Writes: src/**
   - Credential touch
   - Research node
4. **S-4**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C1
   - Writes: app/**
   - Credential touch

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Research nodes

- [P47-7] → S-1: verify the current version

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-14
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-14 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: tests pass
   - Confinement: C2
   - Research node
2. **S-2**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C0
   - Writes: docs/*.md
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - New spend
   - New tools: ripgrep
   - Research node
3. **S-3**: Verify against the criteria
   - Done when: the note exists
   - Confinement: C2
   - Writes: app/**
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: app/**
   - Credential touch
4. **S-4**: Implement the change
   - Done when: tests pass
   - Confinement: C1
   - Reads: src/**
   - Outward effects: publishes the note

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Research nodes

- [P47-7] → S-1: verify the current version

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-15
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-15 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Implement the change
   - Done when: tests pass
   - Confinement: C2

## Coverage map

- AC-1 → S-1

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-16
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-16 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Implement the change
   - Done when: tests pass
   - Confinement: C1
   - Writes: app/**
   - Reads: src/**
   - Credential touch

## Coverage map

- AC-1 → S-1

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-17
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-17 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Implement the change
   - Done when: the checks are green
   - Confinement: C0
   - Reads: app/**
   - Outward effects: publishes the note
   - New spend
   - New tools: ripgrep
2. **S-2**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C1
   - Writes: src/**
   - Reads: src/**
   - New tools: ripgrep
3. **S-3**: Collect the merged changes
   - Done when: all ACs demonstrably hold
   - Confinement: C2
   - Shared-asset write
4. **S-4**: Verify against the criteria
   - Done when: all ACs demonstrably hold
   - Confinement: C2
   - Reads: src/**
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Cited knowledge

- project:conventions

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-18
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-18 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: tests pass
   - Confinement: C1
   - Writes: app/**
   - Reads: docs/*.md
2. **S-2**: Collect the merged changes
   - Done when: the note exists
   - Confinement: C0
   - Reads: docs/*.md
   - New tools: ripgrep
   - Research node
3. **S-3**: Write the note
   - Done when: the note exists
   - Confinement: C0
   - Writes: app/**
   - Reads: src/**
   - Outward effects: publishes the note
   - New tools: ripgrep
   - Research node
4. **S-4**: Verify against the criteria
   - Done when: the note exists
   - Confinement: C0
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Outward effects: publishes the note
   - New spend
   - Shared-asset write

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-19
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-19 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Implement the change
   - Done when: the checks are green
   - Confinement: C0
   - Reads: src/**
   - Research node
2. **S-2**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Reads: app/**
   - New spend
   - Credential touch
   - Shared-asset write
3. **S-3**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Writes: src/**
   - New tools: ripgrep
4. **S-4**: Verify against the criteria
   - Done when: the note exists
   - Confinement: C0
   - Writes: docs/*.md
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Outward effects: publishes the note
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Research nodes

- [P47-7] → S-1: verify the current version

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-20
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-20 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C2
   - Writes: UNBOUNDED (whole-project claim, S02.8)
2. **S-2**: Collect the merged changes
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: docs/*.md
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: docs/*.md

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-21
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-21 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Outward effects: publishes the note
2. **S-2**: Write the note
   - Done when: the note exists
   - Confinement: C2
   - Outward effects: publishes the note

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-22
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-22 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Write the note
   - Done when: the note exists
   - Confinement: C2
   - Credential touch
2. **S-2**: Write the note
   - Done when: the note exists
   - Confinement: C2
   - Reads: docs/*.md
   - New tools: ripgrep
   - Research node
3. **S-3**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C0
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - New spend
4. **S-4**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C0
   - Writes: src/**
   - Shared-asset write
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-23
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-23 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the note exists
   - Confinement: C0
   - Reads: src/**
   - Research node
2. **S-2**: Implement the change
   - Done when: the checks are green
   - Confinement: C0
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: src/**
   - Outward effects: publishes the note
   - New spend
3. **S-3**: Implement the change
   - Done when: the note exists
   - Confinement: C1
   - Writes: docs/*.md
   - Outward effects: publishes the note
   - Shared-asset write

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Research nodes

- [P47-7] → S-1: verify the current version

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-24
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-24 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C2

## Coverage map

- AC-1 → S-1

## Cited knowledge

- project:conventions

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-25
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-25 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Implement the change
   - Done when: tests pass
   - Confinement: C0
   - Writes: docs/*.md
   - Shared-asset write
   - Research node

## Coverage map

- AC-1 → S-1

## Research nodes

- [P47-7] → S-1: verify the current version

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-26
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-26 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Collect the merged changes
   - Done when: all ACs demonstrably hold
   - Confinement: C2
   - Reads: docs/*.md
   - Outward effects: publishes the note
   - New spend
   - Credential touch
   - Research node
2. **S-2**: Verify against the criteria
   - Done when: tests pass
   - Confinement: C0
   - Outward effects: publishes the note
   - Shared-asset write
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Research nodes

- [P47-7] → S-1: verify the current version

## Cited knowledge

- project:conventions

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-27
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-27 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Collect the merged changes
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Shared-asset write
2. **S-2**: Implement the change
   - Done when: the note exists
   - Confinement: C2
   - Writes: docs/*.md
   - Reads: src/**
   - Outward effects: publishes the note
3. **S-3**: Collect the merged changes
   - Done when: the note exists
   - Confinement: C0
   - Writes: app/**
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Shared-asset write
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-28
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-28 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: tests pass
   - Confinement: C0
   - Writes: src/**
   - Reads: docs/*.md

## Coverage map

- AC-1 → S-1

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-29
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-29 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Collect the merged changes
   - Done when: the note exists
   - Confinement: C1
   - Credential touch
   - Shared-asset write
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1

## Cited knowledge

- project:conventions

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-30
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-30 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Write the note
   - Done when: tests pass
   - Confinement: C0
   - New spend
2. **S-2**: Write the note
   - Done when: the note exists
   - Confinement: C0
   - Writes: app/**
   - New spend
   - Shared-asset write
3. **S-3**: Write the note
   - Done when: tests pass
   - Confinement: C1
   - Reads: src/**
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-31
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-31 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Write the note
   - Done when: the checks are green
   - Confinement: C2
   - Outward effects: publishes the note
   - Shared-asset write
   - Research node
2. **S-2**: Implement the change
   - Done when: the checks are green
   - Confinement: C0
   - Reads: app/**
   - New tools: ripgrep
3. **S-3**: Collect the merged changes
   - Done when: the note exists
   - Confinement: C0
   - Writes: docs/*.md
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: src/**

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Research nodes

- [P47-7] → S-1: verify the current version

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-32
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-32 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the note exists
   - Confinement: C0
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - New spend
   - New tools: ripgrep
   - Research node
2. **S-2**: Implement the change
   - Done when: tests pass
   - Confinement: C1
   - Writes: src/**
   - Credential touch
   - Shared-asset write
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Research nodes

- [P47-7] → S-1: verify the current version

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-33
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-33 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Write the note
   - Done when: tests pass
   - Confinement: C0
   - Writes: app/**
2. **S-2**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: docs/*.md
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: src/**
   - Shared-asset write

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-34
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-34 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the note exists
   - Confinement: C1
   - Writes: docs/*.md
   - Writes: UNBOUNDED (whole-project claim, S02.8)
2. **S-2**: Verify against the criteria
   - Done when: all ACs demonstrably hold
   - Confinement: C2
   - Writes: docs/*.md
   - Writes: UNBOUNDED (whole-project claim, S02.8)

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-35
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-35 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Collect the merged changes
   - Done when: the note exists
   - Confinement: C2
   - Writes: app/**
   - Outward effects: publishes the note
   - New spend
   - Research node
2. **S-2**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - New spend
   - Shared-asset write
3. **S-3**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: src/**
   - New spend
   - Shared-asset write
4. **S-4**: Verify against the criteria
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: app/**
   - Credential touch

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Research nodes

- [P47-7] → S-1: verify the current version

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-36
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-36 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Credential touch
   - New tools: ripgrep
2. **S-2**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Research node
3. **S-3**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C0
   - Writes: app/**
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: app/**
   - Outward effects: publishes the note
   - Shared-asset write

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-37
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-37 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C0
   - Outward effects: publishes the note
   - Credential touch
2. **S-2**: Implement the change
   - Done when: the note exists
   - Confinement: C2
   - New tools: ripgrep
3. **S-3**: Collect the merged changes
   - Done when: the note exists
   - Confinement: C1
   - Writes: app/**
   - Credential touch
4. **S-4**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C0
   - Reads: docs/*.md
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Cited knowledge

- project:conventions

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-38
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-38 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C2
   - Writes: docs/*.md
   - Reads: src/**
   - Outward effects: publishes the note
2. **S-2**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Research node
3. **S-3**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C0
   - Outward effects: publishes the note
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-39
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-39 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the note exists
   - Confinement: C0
2. **S-2**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C2
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Research node
3. **S-3**: Implement the change
   - Done when: the note exists
   - Confinement: C0
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: app/**
   - Outward effects: publishes the note
   - Shared-asset write
   - Research node
4. **S-4**: Write the note
   - Done when: the note exists
   - Confinement: C2
   - Reads: src/**
   - Outward effects: publishes the note
   - New spend
   - Credential touch

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-40
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-40 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: docs/*.md
   - Outward effects: publishes the note

## Coverage map

- AC-1 → S-1

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-41
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-41 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Reads: src/**
   - Outward effects: publishes the note
   - New spend
2. **S-2**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C2
   - Writes: docs/*.md
   - Outward effects: publishes the note
   - New spend
   - Research node
3. **S-3**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Research node
4. **S-4**: Implement the change
   - Done when: tests pass
   - Confinement: C1
   - Outward effects: publishes the note
   - New tools: ripgrep
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-42
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-42 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C0
   - Writes: src/**
   - Outward effects: publishes the note
   - Credential touch
2. **S-2**: Write the note
   - Done when: the note exists
   - Confinement: C2
   - Writes: app/**
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Outward effects: publishes the note
   - Credential touch
   - Research node
3. **S-3**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C0
   - New spend
   - Credential touch

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-43
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-43 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Write the note
   - Done when: the checks are green
   - Confinement: C2
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: docs/*.md
   - New spend
   - New tools: ripgrep
   - Research node
2. **S-2**: Write the note
   - Done when: the note exists
   - Confinement: C2
   - New spend
   - New tools: ripgrep
3. **S-3**: Implement the change
   - Done when: the checks are green
   - Confinement: C2
   - Writes: app/**
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Credential touch
   - Shared-asset write
   - Research node
4. **S-4**: Write the note
   - Done when: the checks are green
   - Confinement: C2
   - Writes: docs/*.md
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Research nodes

- [P47-7] → S-1: verify the current version

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-44
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-44 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Collect the merged changes
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Reads: app/**
   - Shared-asset write
   - Research node
2. **S-2**: Implement the change
   - Done when: the note exists
   - Confinement: C2
   - Research node
3. **S-3**: Collect the merged changes
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Reads: app/**
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Research nodes

- [P47-7] → S-1: verify the current version

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-45
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-45 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Implement the change
   - Done when: the note exists
   - Confinement: C2
   - Writes: app/**
   - Reads: docs/*.md
   - Credential touch
   - New tools: ripgrep
   - Research node

## Coverage map

- AC-1 → S-1

## Research nodes

- [P47-7] → S-1: verify the current version

## Cited knowledge

- project:conventions

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-46
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-46 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Collect the merged changes
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: app/**
2. **S-2**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C0
   - New spend
   - Credential touch
   - Shared-asset write
   - Research node
3. **S-3**: Verify against the criteria
   - Done when: tests pass
   - Confinement: C0
   - Writes: src/**
   - Reads: docs/*.md
   - Outward effects: publishes the note
4. **S-4**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C1
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: app/**

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-47
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-47 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: docs/*.md
   - Research node
2. **S-2**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Writes: app/**
   - Shared-asset write
3. **S-3**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - New tools: ripgrep
   - Research node
4. **S-4**: Collect the merged changes
   - Done when: the note exists
   - Confinement: C1
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: src/**
   - New spend
   - Credential touch
   - Shared-asset write
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Research nodes

- [P47-7] → S-1: verify the current version

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-48
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-48 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Implement the change
   - Done when: tests pass
   - Confinement: C1
   - Writes: src/**
   - Reads: src/**

## Coverage map

- AC-1 → S-1

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-49
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-49 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Outward effects: publishes the note
   - New spend
   - New tools: ripgrep
2. **S-2**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C1
   - Reads: docs/*.md
   - Outward effects: publishes the note
   - New tools: ripgrep
3. **S-3**: Implement the change
   - Done when: the note exists
   - Confinement: C1
   - Writes: src/**
   - Reads: docs/*.md
   - Credential touch
   - Shared-asset write
4. **S-4**: Verify against the criteria
   - Done when: the note exists
   - Confinement: C1
   - Writes: src/**
   - Shared-asset write
   - New tools: ripgrep
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-50
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-50 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Writes: app/**
   - New spend
   - Research node
2. **S-2**: Write the note
   - Done when: tests pass
   - Confinement: C1
   - New spend
3. **S-3**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C2
   - Writes: src/**
   - New spend
   - Shared-asset write

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Research nodes

- [P47-7] → S-1: verify the current version

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-51
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-51 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Implement the change
   - Done when: the note exists
   - Confinement: C2
2. **S-2**: Verify against the criteria
   - Done when: the note exists
   - Confinement: C1
   - Writes: app/**
   - New spend
   - New tools: ripgrep
   - Research node
3. **S-3**: Write the note
   - Done when: the note exists
   - Confinement: C2
   - Writes: docs/*.md
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-52
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-52 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: src/**
   - Outward effects: publishes the note

## Coverage map

- AC-1 → S-1

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-53
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-53 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C1
   - New spend
   - New tools: ripgrep
   - Research node

## Coverage map

- AC-1 → S-1

## Research nodes

- [P47-7] → S-1: verify the current version

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-54
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-54 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Write the note
   - Done when: tests pass
   - Confinement: C2
   - Writes: docs/*.md
   - Reads: docs/*.md
   - Outward effects: publishes the note
2. **S-2**: Write the note
   - Done when: tests pass
   - Confinement: C2
   - Outward effects: publishes the note
   - Shared-asset write
   - Research node
3. **S-3**: Implement the change
   - Done when: the checks are green
   - Confinement: C0
   - Writes: app/**
   - Reads: src/**
   - New spend
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-55
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-55 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: all ACs demonstrably hold
   - Confinement: C2
   - Reads: app/**
   - New spend

## Coverage map

- AC-1 → S-1

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-56
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-56 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Outward effects: publishes the note
   - New tools: ripgrep

## Coverage map

- AC-1 → S-1

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-57
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-57 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the note exists
   - Confinement: C1
   - Writes: app/**
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Reads: docs/*.md
   - Research node
2. **S-2**: Verify against the criteria
   - Done when: the note exists
   - Confinement: C0
   - Writes: docs/*.md
   - Reads: app/**
   - Outward effects: publishes the note
   - Credential touch
   - New tools: ripgrep
   - Research node
3. **S-3**: Write the note
   - Done when: the checks are green
   - Confinement: C1
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Research nodes

- [P47-7] → S-1: verify the current version

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-58
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-58 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Implement the change
   - Done when: all ACs demonstrably hold
   - Confinement: C2
   - Reads: docs/*.md
   - Research node
2. **S-2**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C2

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Research nodes

- [P47-7] → S-1: verify the current version

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

---
kind: plan
task: t-59
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-59 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Collect the merged changes
   - Done when: the checks are green
   - Confinement: C1
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - New spend
2. **S-2**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C0
   - Outward effects: publishes the note

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-60
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-60 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Write the note
   - Done when: tests pass
   - Confinement: C1
   - Reads: docs/*.md
   - Shared-asset write
2. **S-2**: Write the note
   - Done when: the note exists
   - Confinement: C1
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-61
owner: u1
version: 2
spec_version: 2
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-61 plan-v2 (implements spec-v2)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C1
   - Writes: src/**
   - Research node
2. **S-2**: Write the note
   - Done when: the checks are green
   - Confinement: C0
   - Writes: src/**
3. **S-3**: Write the note
   - Done when: all ACs demonstrably hold
   - Confinement: C1
   - Writes: UNBOUNDED (whole-project claim, S02.8)
4. **S-4**: Collect the merged changes
   - Done when: the note exists
   - Confinement: C2
   - New spend

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3
- AC-4 → S-4

## Research nodes

- [P47-7] → S-1: verify the current version

## Cited knowledge

- project:conventions

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-62
owner: u1
version: 1
spec_version: 1
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-62 plan-v1 (implements spec-v1)

## Steps

1. **S-1**: Write the note
   - Done when: the note exists
   - Confinement: C2
   - Writes: docs/*.md
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Outward effects: publishes the note
2. **S-2**: Collect the merged changes
   - Done when: tests pass
   - Confinement: C2
   - Writes: UNBOUNDED (whole-project claim, S02.8)
   - Outward effects: publishes the note
   - Research node

## Coverage map

- AC-1 → S-1
- AC-2 → S-2

## Cited knowledge

- project:conventions

## Risks

- the estimate may be off

## Estimate

- Size M, UNPRICED (no price table — honest posture, S10)

---
kind: plan
task: t-63
owner: u1
version: 3
spec_version: 3
status: draft
tier: standard
provenance: pre-A15 planner
---

# Plan — t-63 plan-v3 (implements spec-v3)

## Steps

1. **S-1**: Verify against the criteria
   - Done when: the checks are green
   - Confinement: C1
2. **S-2**: Collect the merged changes
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Reads: docs/*.md
   - New spend
   - New tools: ripgrep
   - Research node
3. **S-3**: Verify against the criteria
   - Done when: all ACs demonstrably hold
   - Confinement: C0
   - Writes: app/**

## Coverage map

- AC-1 → S-1
- AC-2 → S-2
- AC-3 → S-3

## Cited knowledge

- project:conventions

## Estimate

- Size S, ~1.25 USD (API-equivalent, D5)
- Basis: median of the last five

