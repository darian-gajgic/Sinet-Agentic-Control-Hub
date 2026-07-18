## S16 — Adoption manifest & components.lock

**Scope:** The binding inventory of everything Sinet adopts: the `components.lock` format and its CI enforcement, the complete v0 manifest, the component-onboarding checklist every future adoption passes, the patterns-only (Layer C) list, the binding no-adopt register, the quarterly dependency pass, and the standing watch rows this section registers into S14's executor.
**Binding inputs:** G2 D2.2 (+ Def.15, Def.16; §Follow-ups watchlist adds) · G3 D3.3 riders · [R14 §2, §4.1, §5, §6.1 (the citable harvest reference), §6.2, §6.4, §6.5, §7] · [R17 §4.8] · `Spec/frontend-components-v1.md` (binding picks) · [R12 §2, §4] · [R16 §2.3, §4.1] · siblings S01.3/S01.5/S01.11, S03.2/S03.3/S03.6 · G1 P1, P2, P6, P7, rider 3 · feature list D3, 3.10, 4.7, 7.3, S2.8, 14.1/14.4/14.5.

Boundary lines: S03 owns the engine bump procedure and lane onboarding; S01 owns CI mechanics and the unit map; S14 owns watch/canary machinery; S15 owns how components are used. S16 owns the **inventory**, the **gate**, and the **pass**.

### S16.1 Adoption law

- **Report 14 §6.1 is the citable adoption reference** for the whole harvest, superseding `Docs/component-harvest-map-proposal-v1.md` [G2 D2.2; R14 §1]. Pattern/STUDY rows live with their consuming sections; this section binds the ADOPT consolidation and the exclusions.
- **Adopt, don't fork.** Every adopted component runs unmodified and is integrated by configuration, wrapping, or data consumption only. A component that would need patching is not adopted — it is built, or its pattern is copied (S16.5) [G2 D2.2; CLAUDE.md hard rule].
- **Organ-grade only.** Adoptions are data files, OS mechanisms, and single-purpose components — replaceable, pinnable, carrying no orchestration opinions. Nothing spine-shaped (session-owning, board-owning, orchestration-owning) is ever adopted: the 2025 control-plane cohort died at ~14-month half-life, and build-the-control-plane / adopt-the-organs is what that mortality teaches [R14 §4.1, §2.2; G1 D1.1].
- **Every adoption sits behind the S01.3 adoption seam:** pin + replacement path + abandonment criteria exist *before* first use — the exit plan pre-dates the need for it [S01.3; R14 §7 P-T16-1]. Replacement path + abandonment criteria together are the entry's **funeral plan** [R14 §2.2].
- Local **models** are not manifest entries: the serving stack is adopted here; the model set, its 6-monthly re-evaluation, and the swap⇒recalibrate gate are S12's registry, decoupled from workers by duty aliases [G3 §T15 digest; XREF:S12].

### S16.2 `components.lock` — format & enforcement

One manifest file, `components.lock`, in the platform repo [R17 §4.8; P-T16-1]. The **fields are normative; the serialization is P3's choice** (TBD-P3(lock file serialization), any text format diffable in review). Every entry carries, mandatorily:

| Field | Content |
|---|---|
| `name` | canonical component name |
| `kind` | `engine` \| `organ-unit` \| `library` \| `data` \| `frontend` \| `toolchain` \| `os-mechanism` \| `host-managed` \| `mechanism` \| `standby` |
| `pin` | exact version/tag/commit — never `latest`, never a floating range [S03.3]; content hash where vendored |
| `license` | SPDX id + **path scope** of the directories actually consumed + license-check date [R14 §6.4; P-T16-2] |
| `role` | one line + owning spec section |
| `replacement` | the named replacement-or-rebuild path (funeral plan, part 1) [P-T16-1] |
| `abandonment` | exit-trigger criteria (funeral plan, part 2); defaults to ⚙ `adoption.abandonment_months` (S16.7), overridable per entry [R17 §4.8] |
| `watch` | reference to the S14 watch/canary row(s) covering this entry [XREF:S14] |
| `last_review` | date of last quarterly-pass review [R17 §4.8] |

Rules with normative force:

- **CI fails when any running unit or bundled dependency lacks an entry** — the adoption seam enforced mechanically; enforcement mechanics are S01's [S01.11; G2 D2.2]. The S16.3 table is the complete v0 set: anything not listed there and not added through the S16.4 gate is a CI failure, not a judgment call.
- **`standby` entries** (term used as defined here: a pre-registered fallback that is pinned/named but not running or bundled) are exempt from the CI running-unit rule but MUST exist for every replacement path that names a concrete third-party component, so the funeral plan stays executable.
- **OS-mechanism entries are tracked differently from packaged dependencies.** systemd (transient units, timers, slices, systemd-creds), bubblewrap, seccomp, Landlock, and netns+nftables ride the host OS (Ubuntu 26.04 LTS); CI cannot pin the kernel. Their entries record: mechanism name, host-OS release, **last probe result + date** (the D2.3 probe set — cleared [SPIKE P2-S2]), and the ratified per-mechanism fallback (per-binary AppArmor profile; nftables-only egress — which is the boundary anyway; plain-copy workspaces) [G2 D2.3; XREF:S11]. An OS release upgrade is a deliberate event that re-runs the probes before the platform resumes normal admission.
- **`host-managed` entries** (tailscaled) are inventory-complete but not sinet-pinned; Sinet health-checks them (P-T13-1) and never manages their versions [S01.2].
- **Path-scoped licensing (P-T16-2).** Repo-level SPDX labels lie (`MIT except enterprise/` is now a standard shape); license verification is per-directory at the exact pinned ref, at adoption time and on every pin bump [R14 §2.5, §6.4]. Upstream LICENSE/NOTICE files and file headers are kept in-tree for everything vendored or copied — zero cost now, automatic compliance if anything is ever distributed [R14 §6.4]. NOTICE hygiene includes the SAW attribution line (honored though not legally required) [R14 §6.4].
- **Removal discipline.** An entry leaves the lock only by executing its funeral plan, recorded as a dated lock edit with the reason — never by silent deletion.
- **Engine pins are recorded here; the bump procedure is S03.3's.** A bump lands only through the operator-gated release event with conformance + quality probes, and every bump is a worker-revalidation trigger (P-T14-1) [S03.3; XREF:S08].

### S16.3 The v0 manifest

The consolidated ADOPT set this spec has bound [G2 D2.2 (R14 §4.1 Layers A+B); G3 D3.3; sibling sections as cited]. TBD-P3 pins are versions whose exact value is recorded when the lock file is first materialized at P3; the *entries* are binding now.

| Entry (kind) | v0 pin | License | Role → owner | Replacement path (funeral plan) | Watch |
|---|---|---|---|---|---|
| `claude` CLI (engine) | **2.1.214** [S03.3] | proprietary — Consumer ToS; gray-zone posture as-is, compliance quotes per R14 §7-OQ8 [G1 P2] | Anthropic lane → S03.2 | Agent SDK as verified in-adapter alternative, flip conditions pre-registered [S03.2]; billing un-pause → interactive-only demotion [G1 P1] | S2.8 drift canary + legal-text watch [XREF:S14] |
| `opencode-ai` (engine) | **1.18.3 exact** npm [S03.2] | MIT [R14 §2.1] | Z.AI + local lanes (`serve`) → S03.2 | OpenHands `software-agent-sdk` = pre-registered fallback spike target [G2 P4; R14 §6.2]; Goose re-eval after its 2.0/ACP consolidation [R14 §6.2] | v2 declared API break: pin holds until conformance passes [R14 §4.2] |
| llama-swap (organ-unit) | TBD-P3(release pin; v240 observed current) [R16 §2.3, §4.1] | MIT [R16 §2.3] | local-lane front, TTL unload, manual-unload verb → [XREF:S12] | llama-server router mode + Sinet kill-idle timer, or Ollama [R16 §4.1] | bus factor 1 → conformance suite asserts its contract on every bump [R16 §4.1] |
| llama.cpp `llama-server` (engine) | TBD-P3(`b`-tag pin) [R16 §4.1] | MIT [R16 §2.3] | local model server per GPU-UUID → [XREF:S12] | Ollama (standby row) | `/v1` logprobs + `json_schema` asserted **behaviorally** every bump — docs are never the check [R16 §7-OQ9; S03.3] |
| Ollama (standby) | v0.32.1 observed; pin on activation [R16 §2.3] | MIT core [R16 §2.3] | capability-complete local-lane fallback (incl. `/v1` logprobs) [R16 §4.1] | — (is itself the fallback) | re-run stack comparison if it gains per-model GPU pinning [R16 §4.10] |
| `sandbox-runtime` (library) | TBD-P3(pin at sandbox compose) | Apache-2.0 [R14 §6.4] | bwrap+seccomp+proxy core inside the composed sandbox stack → [XREF:S11] | rebuild: direct composition of the same kernel primitives the stack already ratifies [G2 §T09 digest, D2.3] | S2.8 watch [XREF:S14] |
| OS mechanisms: systemd (units/timers/slices/creds) · bubblewrap · seccomp · Landlock · netns+nftables (os-mechanism) | host OS = Ubuntu 26.04 LTS; probes cleared [SPIKE P2-S2; G2 D2.3] | OS packages, various [R14 §6.4] | shell + sandbox + broker-injection substrate → [XREF:S01; S11] | per-mechanism ratified fallbacks [G2 D2.3; XREF:S11] | re-probe on OS upgrade + quarterly pass |
| tailscaled (host-managed) | host-managed | — | tailnet front door (D1) → [S01.2] | — (D1-constitutive) | post-resume reconcile, P-T13-1 [XREF:S01] |
| Caddy (organ-unit) | TBD-P3(pin at onboarding) | TBD-P3(path-scoped verify at onboarding — no research input records it) | front router; admin-API preview routes → [S01.2; XREF:S13] | process-seam swap; routing config only, admin-API usage contained in the preview module [S01.3; XREF:S13] | quarterly pass |
| changedetection.io (organ-unit) | TBD-P3(pin; v0.55.8 observed) [R12 §2] | Apache-2.0 [R12 §2] | watchlist page-diff tier → [G2 Def.12; XREF:S14] | process-seam swap [S01.3]; native feed poller + canary layer carry the watch duty through any gap [G2 Def.12] | quarterly pass |
| promptfoo (library, runner) | TBD-P3(pin; v0.121.19 observed) [R12 §4] | MIT [R12 §4] | regression-eval runner; runner-never-store — all scores land in `platform.db` [G2 D2.1(d); P-T11-4] → [XREF:S14] | **DeepEval swap pre-registered** (standby; Apache-2.0) [R12 §4; G2 §T11 digest] | OpenAI-acquisition drift watch [R12 §2] |
| genai-prices `data.json` (data, vendored) | vendored + hash-pinned; TBD-P3(exact; v0.0.71 line observed) [R14 §3.2] | MIT [R14 §3.2] | shipped price-table defaults; user edits overlay, never overwritten → [XREF:S10] | LiteLLM JSON as primary + manual table (the original posture) [R14 §4.2] | scheduled refresh lands as a **proposal**, never a silent overwrite [G2 Def.16]; staleness ⚙ (S16.7) |
| LiteLLM `model_prices…json` (data, cross-check) | pinned per refresh | MIT **root only — path-scoped** (`enterprise/` carve-out) [R14 §6.4; P-T16-2] | price-breadth cross-check → [XREF:S10] | drop — cross-check is optional | rides the price-refresh task |
| sops (library/tool) | TBD-P3(pin) | MPL-2.0 [R14 §6.4] | per-person encrypted store format, broker mechanics → [XREF:S11] | gopass (same role, GPG+git flavor) [R14 §3.1] | quarterly pass |
| age (library/tool) | TBD-P3(pin) | BSD-3 [R14 §6.4] | store + snapshot encryption → [XREF:S11; S13] | gopass-class store alternative [R14 §3.1]; snapshot-lane crypto swap at S13's pipeline seam [XREF:S13] | X-Wing age-CLI plugin row (S16.8) |
| @hello-pangea/dnd (frontend) | **18.0.1** | Apache-2.0 | board drag-drop (S1.3) → [XREF:S15] | pinned @dnd-kit/core 6.3.1 (standby, frozen-but-working) | archive/unmaintained watch; @dnd-kit/react 1.0 row (S16.8) [frontend-components-v1] |
| react-diff-view + gitdiff-parser (frontend) | **3.3.3** | MIT | diff review surface, rendering the ported N15 review behavior (S3.5) → [XREF:S15] | monaco diff via @monaco-editor/react 4.7.0 (standby upgrade path) | single-maintainer watch [frontend-components-v1] |
| @assistant-ui/react (frontend) | **0.14.27** pinned | MIT | chat surface via LocalRuntime/ExternalStoreRuntime, no assistant-cloud (S3.6) → [XREF:S15] | owned chat primitives on the same SSE cursor — ExternalStoreRuntime keeps message state platform-owned, so the swap loses no data | 0.x churn vs quarterly-pass absorption; 1.0 row (S16.8) [frontend-components-v1] |
| JSON Forms (@jsonforms/core+react) (frontend) | **3.8.0** | MIT | generated settings UI renderer (13.4/13.5) → [S01.10; XREF:S15] | RJSF v6 (documented runner-up); re-entry condition: stall >18 months or a rule/layout vocabulary gap | stall watch [frontend-components-v1] |
| React 19 + Vite + TypeScript tree (toolchain) | lockfile-pinned; exact tree TBD-P3(SPA scaffold) | recorded at P3 onboarding | SPA build; dev-time only, no runtime service exposure [R17 §4.3, §5] | HTMX 2.x re-entry only via its named conditions [G3 D3.3; R17 §4.10; XREF:S15] | Vite majors adopted on a lag, never day-one [G3 D3.3 rider] |
| Go toolchain (toolchain) | TBD-P3(pin at scaffold) | recorded at P3 onboarding | backend build → [S01.5] | none live — Python/Litestar posture is documentation only [S01.5; G3 D3.2] | go1compat; quarterly pass |
| SQLite via pure-Go driver (library) | TBD-P3(driver pin) [S01.5] | SQLite public domain [R14 §6.4]; driver license recorded at P3 onboarding | `platform.db` engine + driver → [XREF:S02] | driver swap inside the storage module (storage seam) [S01.3] | currency policy: pin the release line, take patch releases deliberately [R17 §4.7] |
| AGENTS.md + CLAUDE.md shim (mechanism) | format as consumed at the pinned engine versions | — (format, no dependency) | repo-onboarding instruction mechanism [G2 D2.2 Layer A; R14 §6.1 N18] → [XREF:S05] | own instruction assembly (mechanism adoption — nothing to abandon) | format drift rides the engine conformance suites [S03.3] |

### S16.4 The component-onboarding checklist (P-T16-1 — the report-02 §5 twin)

Every future adoption passes this gate; it is the component twin of the provider/lane onboarding checklist that S03.6 owns [R14 §7 P-T16-1; G1 rider 3]. A lane addition never forces a new component (S03.6); a component addition never creates a lane. Checks, in order — all MUST pass:

1. **Niche & seam.** The role is named in one line, with the owning spec section and which S01.3 seam the component sits behind [S01.3].
2. **Organ-grade screen.** Single-purpose; no orchestration, session-ownership, or store-of-record opinions. Spine-shaped candidates fail here regardless of quality — build instead [R14 §4.1, §2.2].
3. **Health check from primary sources.** Maintenance cadence, bus-factor/commit concentration, security-response record, governance — checked at the candidate ref, the way R14 checked every current entry [R14 §2, method].
4. **License, path-scoped.** Verified per-directory at the exact candidate ref (carve-out scan: `enterprise/`, `ee/`, dual-license trailers); date recorded; LICENSE/NOTICE retained in-tree [P-T16-2; R14 §6.4].
5. **Capability claims verified behaviorally.** Probe the pinned version; conformance suites assert behavior, never docs [S03.3; R16 §7-OQ9].
6. **Adopt-unmodified check.** Integration is config/wrapping/data only; any needed patch disqualifies adoption [G2 D2.2].
7. **Runner-never-store.** Whatever the component computes, the system of record stays `platform.db` [G2 §Follow-ups, P-T11-4 criteria].
8. **Exact pin + funeral plan.** Pin (never `latest` [S03.3]); named replacement-or-rebuild path; abandonment criteria — all written before first use [P-T16-1; R17 §4.8].
9. **Watch registration.** A drift/canary/feed row registered into S14's executor; protocol touchpoints additionally pin the protocol version with a dated migration trigger [P-T16-3; XREF:S14].
10. **Lock entry + approval.** Entry written; CI green [S01.11]; the adoption lands as an operator-approved proposal — a platform-level change under D10, matching the precedent that every current entry was gate-ratified [G2 D2.2; G3 D3.3].

### S16.5 Layer C — patterns-only adoptions

Copied shapes, zero dependencies: no lock entries exist because nothing runs or is bundled. Each pattern's normative home is its consuming section; they are listed here so no future session mistakes them for adoptable runtimes [G2 D2.2 Layer C].

| Pattern | What is copied | What is NOT taken | Consumer |
|---|---|---|---|
| LangGraph interrupt/approval schema | The approval-item contract, verbatim: action request + `allow_ignore/allow_respond/allow_edit/allow_accept` + response types `accept/edit/response/ignore` [R14 §2.3, §3.4] | agent-inbox as a dependency (LangGraph+LangSmith-locked). The approval-inbox niche is **empty** in mid-2026 — S3.2 is **built, not adopted** [R14 §2.3] | approval inbox [XREF:S15] |
| gh-aw safe-outputs | Read-only-default effects gating, structured action requests, per-handler caps, sanitization, **default-on threat-detection pass** over proposed outputs; effects gate ≠ quality gate [R14 §6.1 G1 row] | the GitHub Actions runtime | effect journal + verification [XREF:S02; S07] |
| AG-UI event vocabulary | Event shapes mirrored in the SSE contract [R14 §3.3; frontend-components-v1] | CopilotKit runtime | live surfaces [XREF:S15] |
| Archon A2 dialect vocabulary | Own **versioned dialect** mirroring the proven vocabulary (prompt/bash nodes, `depends_on`, loop/until), with corrections baked in: fresh-context ON at stage boundaries; explicit approval node [R14 §6.2 A2] | their parser or runtime, ever — Sinet's control plane executes workflows [G1 rider 2] | workflow/automation dialect [XREF:S08] |
| Agent Vault egress-substitution | STUDY input to the broker workshop only (single-host voids its premise) [G2 P3; R14 §3.1] | the Infisical stack | broker design [XREF:S11] |
| Crush K1/K2 | Durable ask-record + pull-based enumeration; default-deny ToolPolicy **atop** the OS sandbox, never instead [R14 §6.1 K1/K2] | any Crush code — see below | already embedded [S03.4; XREF:S11] |

**Crush patterns-only is POLICY, not legal necessity.** FSL-1.1-MIT legally permits private, non-competing use including code — the harvest map's "legally required" rationale was corrected by R14. The operator ratified patterns-only anyway (adopt-don't-fork hygiene; Go/TUI stack mismatch; K1/K2 patterns suffice) [G2 D2.2; R14 §6.5]. Revisitable **per-version** as FSL→MIT conversions land from ~mid-2027 — a dated watch row (S16.8), operator decision only.

### S16.6 Anti-harvest register — the binding no-adopt list

These rows are settled; a future session re-litigates one only through S00.9 amendment mechanics with new primary evidence. Terse by design [R14 §6.5, §5; G2 D2.2].

| No-adopt | Reason (one line) | Binding rule |
|---|---|---|
| Engine forks / Nexus core-mods | adopt-don't-fork; the keystone need (per-session models) is native upstream [R14 §6.5] | no modification to any adopted engine, ever |
| Standing specialist roster | single-agent-first is the ratified, field-convergent posture [G1 D1.1; R14 §6.5] | machinery only when earned (14.4) [XREF:S04; S08] |
| Monolith topology | bus-factor-1 death shape; cohort mortality [R14 §6.5; R17 §6] | the five seams bind [S01.3] |
| Peer-mesh / group-chat topologies | 17.2× vs 4.4× error amplification; 72% fact erasure [R14 §6.5] | D6 / 14.1: no lateral messaging at any depth |
| Judge-only-frontier / cheap-executor-by-default | the bench-02 failure; inversion #2 [R14 §6.2 S2 row, §6.5] | executor floors ride effort modes [XREF:S10] |
| JARVIS breadth (voice/avatar/galaxy) | 14.5 freeze; validate before breadth [R14 §6.5; frontend-components-v1 §3] | re-entry only via the 15.3 benchmark gate, as satellites (12.4) |
| Vector-first memory sidecars (mem0/qdrant-class) | benchmark basis collapsed under audit; files+FTS5 win [R14 §6.5] | vector post-gate only, rank-only [G2 Def.8; XREF:S09] |
| Live-tree symlinks / machine-coupled deploys | CI-verified artifacts + transient units ratified [R14 §6.5] | deploy per S01.11 only |
| Fire-and-forget triggers | every ingress spawns under the scheduler with owner attribution (3.4) [R14 §6.5] | no unattributed execution path exists [XREF:S10] |
| Load-bearing metered paths | two of three majors tightened in H1-2026; revocable forbearance is the fragility [R14 §6.5] | 3.10; metered-exception list EMPTY at v0 [G1 P7] |
| Crush code | policy, per S16.5 | patterns-only until a dated operator revisit |

**Dependency classes rejected with cause** (each with its named replacement-in-kind already in this spec) [R14 §5]: the dead control-plane cohort (HumanLayer, Vibe Kanban, Omnara, Terragon, Crystal — STUDY/archival only); agent-inbox (deployment-locked); MCP elicitation as approval spine (mid-restructure; at most a future inbox *adapter* behind a seam [P-T16-3]); container-use (superseded by the ratified kernel-primitive stack); OpenMeter/Lago/LiteLLM-proxy as metering (proxies are structurally blind to CLI-wrapped subscription traffic); Vault/OpenBao/Infisical core (one-host overkill; OpenBao noted as the license-safe successor if a vault is ever needed); tokencost (stale); trace platforms as system of record (P-T11-4; projection targets at most); n8n (condition lapsed — own dialect + C0 connectors); durable-execution spines LangGraph/Temporal/Restate/Inngest/DBOS (shape mismatch; **DBOS = pre-registered fallback only** [R14 §3.5]).

**No-public-registry-imports (P-T16-4).** The self-extending workforce NEVER auto-imports public skills/templates/workers (ClawHavoc poisoned ~12% of a public registry). Anything a human imports is untrusted content under 4.7 and enters only through D10 gates with provenance recorded. Enforcement lives in the composer/import path [R14 §2.6, §7 P-T16-4; XREF:S08].

### S16.7 The quarterly dependency pass ⚙

A scheduled, operator-visible pass over every lock entry [G3 D3.3 rider; R17 §4.3, §4.8], co-scheduled with the snapshot restore drill [R17 §4.8; XREF:S13]. Per entry it checks:

- **Version currency** — upstream releases and security advisories vs the pin; candidate bumps identified (engines route through S03.3's gate; components through their conformance/behavior checks).
- **License drift** — path-scoped re-check at the candidate ref on any proposed bump [P-T16-2].
- **Archive/abandonment** — observed activity vs the entry's abandonment criteria; a breach proposes activating the funeral plan.
- **Peer-range / toolchain health** — frontend peers vs pinned React/Vite; Vite-major lag respected [G3 D3.3; frontend-components-v1].
- **Watch-row hygiene** — S16.8 rows reviewed for staleness; `last_review` updated.

**Outputs are proposals, never silent bumps** [S03.3; G2 Def.16 as the shipped precedent]: bump proposals, exit-plan activations, and watch-row retirements, each an operator-approved, dated lock edit. The first pass (at P3) additionally records the TBD-P3 onboarding facts left open in S16.3 (Caddy + toolchain licenses, exact pins) and runs the R13-OQ7 verification (S16.8).

### S16.8 Watchlist adds — standing rows registered into S14's executor

Registration only; polling machinery, cadence, and alert routing are S14's [G2 Def.12; XREF:S14].

| Row | Watches | Fires → |
|---|---|---|
| Fine-grained-PAT collaborator gap | GitHub PAT capability change | broker/push posture options widen [G2 §Follow-ups; XREF:S13] |
| Free-plan ruleset availability | enforced rulesets on Free private repos | server-side second guardrail beyond the broker (P-T12-1) [G2 §Follow-ups; XREF:S13] |
| diffity maturity | project maturity | S13 diff-tooling candidate re-eval [G2 §Follow-ups] |
| jj compat matrix | jj/git compatibility | S13 git-machinery re-eval (no jj at v0) [G2 §Follow-ups; G2 D2.1(e)] |
| X-Wing age-CLI plugin | plugin status | broker/backup crypto options [G2 §Follow-ups; XREF:S11; S13] |
| DBOS TS/SQLite parity | fallback health | durable-execution fallback stays credible [G2 §Follow-ups; R14 §3.5] |
| MCP 2026-07-28 final (Tasks/Extensions) | protocol restructure landing | P-T16-3 dated migration trigger; inbox-adapter seam re-eval [R14 §2.3, §7; XREF:S03] |
| ACP v1 line | verb-set/transport convergence | adapter-contract alignment check (verbs already a superset) [R14 §6.1 O4; S03.1] |
| Litestream #1083 | silent-replication-failure triage | re-decide the Litestream unit at implementation [G2 D2.5; XREF:S01; S13] |
| Python-Redlines + pandiff | license/maintenance — verified at the first quarterly pass (the P3 dependency audit, R13-OQ7) | S13 doc-diff candidates admitted or dropped [G2 §Follow-ups] |
| @dnd-kit/react 1.0 | 1.0 with React-19-clean peers | re-evaluate the board pick with a dated note [frontend-components-v1] |
| assistant-ui 1.0 | stability promise or hostile 1.0; 0.x churn rate | re-pin, or owned-primitives fallback [frontend-components-v1] |
| awesome-harness-engineering referents | `walkinglabs/…` + `ai-boost/…` activity | drop the row if both stall [G2 Def.15] |
| Crush FSL→MIT conversions | per-version MIT from ~mid-2027 | operator may revisit patterns-only policy (S16.5) [G2 D2.2; R14 §4.2] |
| genai-prices staleness | >⚙ `adoption.price_data_stale_days` without updates while providers reprice | flip to LiteLLM-primary + manual table [R14 §4.2] |
| opencode v2 | GA / declared server-API break | bump held at the S03.3 conformance gate [R14 §4.2] |

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `adoption.dependency_pass_months` | 3 | (1, 6) [coordinator-draft] | G3 D3.3 rider; R17 §4.8 |
| `adoption.abandonment_months` | 6 [coordinator-draft] | (2, 24) [coordinator-draft]; per-entry overridable | R17 §4.8 (⚙-flagged, unnumbered); P-T16-1 |
| `adoption.price_data_stale_days` | 60 | (14, 180) [coordinator-draft] | R14 §4.2 |

**Known problems owned here:**

- **P-T16-1** — ecosystem mortality (~14-month half-life): discharged — onboarding checklist (S16.4) + mandatory funeral-plan fields (S16.2) + CI presence rule [S01.11].
- **P-T16-2** — per-directory license splits: discharged — path-scoped license field + checklist #4 + quarterly re-check on bumps (S16.2/S16.7).
- **P-T16-3** — protocol-surface churn as a scheduled event: protocol touchpoints pin their protocol version in the lock with dated migration triggers (MCP/ACP rows, S16.8); canary machinery [XREF:S14]; adapter mechanics [XREF:S03].
- **P-T16-4** — registry supply chain: the no-public-registry-imports rule bound at S16.6; enforcement in the composer/import path [XREF:S08].

Referenced, owned elsewhere: P-T14-1 (bump = mass revalidation event — S03.3 [XREF:S08]); P-T11-4 (runner-never-store — checklist #7, promptfoo row); P-T12-1 (broker-is-the-guardrail — watch rows 1–2 [XREF:S13]).

**Deferred / parked:**

- Litestream unit → re-entry: #1083 triaged at implementation [G2 D2.5].
- Crush code harvest → re-entry: per-version FSL→MIT conversions from ~mid-2027, operator policy revisit (S16.5/S16.8).
- @dnd-kit/react family → re-entry: its 1.0 with clean React-19 peers, dated note [frontend-components-v1].
- vLLM as eGPU batch backend → re-entry: a measured throughput need; llama-swap manages it unchanged [R16 §4.1].
- xAI-via-opencode lane entry → re-entry: a member holds the plan; S03.6 onboarding checklist, config-only [G1 P6; R14 §4.1].
- HTMX 2.x architecture re-entry → only via its named conditions [G3 D3.3; R17 §4.10; XREF:S15].

**Coverage:**

| Feature-list item | Where |
|---|---|
| D3-adjacent adoption law (adopt-don't-fork, one manifest) | S16.1, S16.2 |
| S2.8 external-change watch — component/protocol drift rows (machinery → S14) | S16.3 watch column, S16.8 |
| 7.3 model/engine deprecation vs stored configurations — the pin registry as the revalidation clock | S16.2, S16.3 (procedure [XREF:S03; S08]) |
| 13.4 settings surface (pass cadence, abandonment, staleness as ⚙) | S16.7 |
| 14.1 / 14.4 / 14.5 deliberate non-goals as binding no-adopt rows | S16.6 |
| 3.10 + Operating reality (no load-bearing metered paths) | S16.6 |
| 4.7 untrusted imported definitions (P-T16-4) | S16.6 |

**Open items for G4:** none. Drafting-time sub-choices flagged inline as [coordinator-draft]: the `adoption.abandonment_months` = 6 default and the three clamp ranges (R17 §4.8 ⚙-flags the number without fixing it). The TBD-P3 cells in S16.3 (Caddy + toolchain licenses, exact pins) are recording tasks at the first quarterly pass, not decisions.
