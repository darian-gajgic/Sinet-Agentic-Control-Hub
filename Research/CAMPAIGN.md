# Research Campaign — master plan

**What this is:** the standing plan for the pre-implementation research campaign, executed by Claude Fable 5 sessions. `STATE.md` is the live pointer (what's done, what's next). `.claude/skills/research-campaign/SKILL.md` is the executable runbook — in any new session, say **"continue the research campaign"** or invoke `/research-campaign` and the session picks up from `STATE.md` with no other context.

**Created:** 2026-07-16. **Owner:** operator. **Executor:** Fable 5 coordinator sessions.

---

## 1. Goal and phase map

Deliver everything needed to write the **core architecture spec** for v0 (feature list §15.1), then the spec itself — before any implementation, per project rules.

- **P0 — Setup** (done 2026-07-16): this plan, per-topic briefs, state tracking, coordinator skill.
- **P1 — Research campaign** (this plan's body): 16 topics in 4 waves, decision gates between waves. Output: `Research/NN-*.md` reports + gate decisions.
- **P2 — Core architecture spec**: synthesize research + decisions into `Spec/core-architecture-v1.md` (new top-level dir, created at P2 start). Reviewed at a gate; approval is the operator's explicit end of the research phase (CLAUDE.md gate — the phase flag change is the operator's act).
- **P3 — v0 skeleton implementation**: coordinator + worktree-isolated implementation subagents over a build-order DAG derived from the spec. Verification culture from slice one (`verify` skill, tests, the post-mortem's P46 escalation test early). Detailed planning happens in P2 with research in hand — deliberately not over-planned now.
- **P4 — Expansion**: v0.1 web research domain, benchmark gate (§15.3), v1 household onboarding — each addition runs its own mini research → spec-delta → implement → eval cycle using this same machinery.

## 2. Operating model (how Fable 5 runs this)

**Coordinator pattern.** One main session is the coordinator. Each research topic runs as a separate context: a subagent that reads the topic brief, invokes the `deep-research` skill (fan-out searches, source fetching, adversarial verification, cited synthesis), and writes the report file itself. The coordinator only sees digests — its context stays lean. Across session limits, the coordinator's identity lives in files: `STATE.md` + this plan + the skill. Any fresh session *is* the same coordinator.

**Autonomy contract** (the operator's standing instruction: autonomous as much as possible, pause only for decisions):

Run WITHOUT asking:
- launching topics of the currently approved wave, validating reports, bounded revision requests
- saving reports, updating `STATE.md`, committing, pushing
- spot-checking citations, resolving scoping questions from the docs

ALWAYS pause (gate):
- wave boundaries (present findings digest + decisions, wait for the operator)
- any decision that binds architecture, spends money, creates accounts, or changes scope
- anything implementation-shaped (research phase rule)
- contradictions between sources that change project direction

**Execution modes:**
- **Mode A (default):** topic runs inside an Agent-tool subagent that invokes `deep-research` and writes the report file directly. Verified available (probe 2026-07-16). Up to **2 topics concurrently**, launched in background; coordinator validates + commits each as it completes.
- **Mode B (fallback):** coordinator invokes `deep-research` directly in the main session, one topic at a time. Use if Mode A misbehaves.
- The **pilot (T01) runs Mode A with Mode B fallback** — it validates the pipeline end-to-end before the fleet flies.

**Checkpoint discipline:** `STATE.md` updated before and after every launch; one commit per completed report (`Research NN: <title> (T##)`); push after every commit (private remote — maximum loss-protection). A session can die at any moment and the next one resumes losslessly.

## 3. Topic map — 16 topics, 4 waves

Depth: **FULL** = deep-research run (multi-source, adversarially verified). **LIGHT** = targeted single-session research (well-trodden territory; currency check rather than frontier survey).

| T | Topic (slug) | Wave | Depth | Feature-list scope (core) | Covers (operator's terms) |
|---|---|---|---|---|---|
| T01 | execution-engines-and-adapters | A1 pilot | FULL | D3, Operating reality (engine/credits), 3.2, 2.7 | engine choice, agent harness |
| T02 | agent-loop-and-harness-engineering | A2 | FULL | D7, 4.3, 4.5, 4.6, 3.7, 3.8 | harness/loop engineering |
| T03 | orchestration-and-multiagent | A2 | FULL | D6, 2.4, 7.7, 14.1, 14.4 | orchestration, agent architecture, deep agents, dynamic subagents |
| T04 | context-engineering | A2 | FULL | 8.4, S4.6, 4.3, S1.6 | context engineering |
| T05 | intake-planning-spec-pipeline | A2 | FULL | 1.1–1.10, 2.3, 2.5 | agent engineering (intake) |
| T06 | verification-and-quality-loops | A2 | FULL | 5.1–5.8, P45, P46, 7.6 | evals (judging half) |
| T07 | durable-state-checkpointing-recovery | B1 | FULL | 3.8, 4.8, 4.5, S1.11 | loop engineering (state half) |
| T08 | metering-quota-scheduling | B1 | FULL | 3.1–3.11, D4, D5, 2.8 | — |
| T09 | sandboxing-confinement | B1 | FULL | S5, 4.1, 4.4, 4.7, D2 | — |
| T10 | memory-and-knowledge-architecture | B2 | FULL | §8, S4, 15.6 | memory engineering |
| T11 | evals-observability-benchmark | B2 | FULL | 11.1, 11.2, S2, 5.7 | evals |
| T12 | deliverables-review-git | B2 | FULL | §6, S1.8–S1.11, D9, 11.3 | — |
| T16 | oss-harvest-validation | B2 | FULL | harvest map, all | (repo harvest — operator ask) |
| T13 | platform-stack-architecture | C | LIGHT | S3, §13, 15.6 | — |
| T14 | worker-ontology-and-domain-agents | C | FULL | §7, D8, D10, 5.7 | domain-specific agents |
| T15 | local-models-layer | C | FULL | Operating reality (local tier), S2.7, 3.11, 1.10 | — |

Numbering note: report files get the next free `NN` in `Research/` **in completion order** (README convention); `STATE.md` maps T## → NN. Briefs live in `Research/briefs/T##-<slug>.md`; the shared payload preamble is `Research/briefs/00-shared-context.md`.

## 4. Waves and gates

```
G0  Launch decision (operator) ──────────────── recorded in STATE.md
A1  T01 pilot ──► validate pipeline + report quality
A2  T02 T03 T04 T05 T06  (2 concurrent)
G1  ARCHITECTURE DIRECTION gate ─ engine direction, session/stage model,
    orchestration defaults within D6, verification axes.
    B-wave briefs get a one-paragraph addendum from G1 decisions.
B1  T07 T08 T09   (resilience / economics / safety substrate)
B2  T10 T11 T12 T16
G2  SUBSTRATE + ADOPTION gate ─ state layer, metering design, sandbox tech,
    memory architecture, observability stack, harvest verdict list.
C   T13 T14 T15
G3  SPEC-READINESS gate ─ remaining decisions; approve starting P2 synthesis.
P2  Spec drafting ──► G4 SPEC REVIEW gate ─ operator approves spec and
    explicitly ends the research phase (CLAUDE.md flag is operator's act).
```

Why this order: T01 is the keystone (D3 + the subscription-coverage constraint make engine choice the biggest architectural bet, and the "Agent SDK moving to credits" claim needs live verification before anything leans on it). Wave B topics consume G1's direction (checkpointing design depends on the engine's session model; metering depends on which substrates exist; sandboxing depends on the engine's tool-execution shape). T16 runs late deliberately — harvest verdicts should reference fresh findings, not anchor them ("research fresh, don't anchor"). Wave C rounds out spec inputs that don't gate earlier work.

## 5. Gate protocol

At each gate the coordinator:
1. Writes `Research/decisions/GATE-N-<slug>.md` from `GATE-TEMPLATE.md`: per-topic findings digest (5–10 lines each, links to reports), then **numbered decisions** — each with options, a recommendation with reasoning, and what each option forecloses.
2. Presents the decisions in-session (AskUserQuestion, max 4 per call; minor items may be listed as "defaults unless objected", clearly marked).
3. Records answers in the gate file (choice, date), updates `STATE.md`, unblocks the next wave.
4. If the session is near its limit: write the memo, mark `STATE.md` `blocked-on-gate`, stop cleanly. The next session re-presents.

Decisions are permanent campaign inputs: later briefs and the P2 spec cite gate files, not conversation history.

## 6. Report contract

Every report follows `Research/README.md` plus the campaign additions — full text in `briefs/00-shared-context.md`, summarized: Scope · Current state of the art (dated, live-researched) · Candidate approaches with trade-offs against the operator context · Recommendation for Sinet (+ what would change it) · **What NOT to use and why** · Harvest-map verdicts (CONFIRM / REVISE / REJECT / SUPERSEDED-BY) · Open questions (answerable, with proposed owner) · Sources (URL + access date). Load-bearing claims need ≥2 independent sources; ToS/pricing/licensing/maintenance claims need primary or very recent sources — never model memory.

**Coordinator validation checklist** (per report, before commit): all sections present; recommendations traceable to cited evidence; 2–3 citations spot-checked by fetching them; harvest items verdicted; no D1–D10 contradictions; open questions actually answerable. One bounded revision request if it fails; still weak → flagged at the gate, never silently accepted.

## 7. Cost and pacing (honest estimate, not a promise)

A FULL deep-research run is heavyweight: many web fetches, adversarial verification, order-of-magnitude 10⁵–10⁶ tokens, tens of minutes wall clock. 15 FULL + 1 LIGHT ≈ **6–10 working sessions spread over 1–2 weeks** under plan limits, pacing with 2-concurrent waves. The operator can trim scope at any gate (each brief states what trimming would cost the spec). Usage-limit interruptions are normal events: state is committed, the next session resumes.

## 8. Risks and mitigations

- **Report quality variance** → per-report checklist, citation spot-checks, bounded revision, gate review by operator.
- **Stale "best practice"** (field moves monthly) → briefs demand dated sources, prefer ≤12 months, older material must be flagged as possibly superseded; verification against multiple leading sources (operator names langchain.com/blog as an example of the class).
- **Anchoring on Nexus/harvest map** → briefs frame map items as candidates-to-evaluate, never defaults; post-mortem failure analysis outranks map mechanisms; T16 runs after Wave A.
- **ToS/pricing claims wrong** → T01 verifies the load-bearing subscription/credits claims against primary sources first, before anything builds on them.
- **Session/usage limits mid-run** → per-report commits + STATE.md; nothing lives only in a conversation.
- **Skill availability drift in subagents** → probe re-run at session start when Mode A errors; Mode B fallback documented.

## 9. Standing rules (inherited, restated)

- Research **within** D1–D10, never against them. Docs/ is read-only.
- No implementation during research phase; implementation-shaped requests are flagged and confirmed.
- Adopt, don't fork — no engine modifications, ever. No load-bearing metered-API paths.
- Every real-world fact (pricing, ToS, model capability, library status) is live-researched with dated citations.
