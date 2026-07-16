# T01 — Execution engines & the adapter layer

**Wave:** A1 (pilot — runs first, alone) · **Depth:** FULL · **Report slug:** `execution-engines-and-adapters`

## Scope
D3 (dual substrate, adapter contract), Operating reality (subscription-coverage rule, Agent-SDK-credits claim), 3.2 (limit events at the adapter), 2.7 (gap advice needs current subscription landscape), 12.7/12.8 (far-future multi-host — awareness only).

## Why this gates the spec
The engine is the platform's beating heart and the one component that may never be modified (adopt-don't-fork). Everything downstream — checkpointing (T07), metering (T08), sandboxing (T09) — inherits the engine's session model, API surface, and auth behavior. The single most load-bearing real-world claim in the spec ("Agent SDK moving to credits ⇒ not an engine option") is recorded from the operator and **must be verified against primary sources before anything leans on it.**

## Core question
Which execution engine(s) should Sinet adopt **unmodified** as its agent-run substrate in mid-2026 — and what does the adapter contract (start, stream, checkpoint, pause, resume, cancel, report usage) look like over each — given that every frontier call must ride a per-person flat-rate consumer subscription, at least one major provider only permits subscription-covered programmatic use through its own first-party tool, and local models must ride the same adapter?

## Sub-questions
1. **Verify live (primary sources, dated):** Anthropic's current rules for programmatic use under consumer plans (Pro/Max) — Claude Code CLI headless/`-p`, Agent SDK, OAuth tokens: what is subscription-covered today, what is credits/metered, what changed recently and what is announced? Is the operator's "Agent SDK moving to credits" note accurate as stated?
2. Same audit for the other majors mid-2026: OpenAI (Codex/CLI equivalents under consumer plans), Google (Gemini CLI), plus the flat-rate coding-plan ecosystem (Z.AI GLM plans, others popular now): which permit programmatic/automation use, through which tool, with what stated limits or ToS language?
3. **Engine field survey** (evaluate against the field — the harvest map proposes opencode, do not presume it): opencode, OpenHands, Goose, Crush (license!), aider, and anything credible that emerged since. Judge each on: adopt-unmodified viability for a control plane (server/API mode, events/streaming), per-session model/provider selection, sub-agent support, permission/approval hooks, session persistence/resume, compaction, extension points (skills/tools/MCP), license, maintenance velocity, bus factor.
4. First-party-CLI wrapping as a substrate: state of `claude` CLI headless automation (stream-json, session resume, usage/cost envelope), drift history and canaries (OpenClaw's claude-cli backend), equivalent wrapping for other providers' first-party tools.
5. Map D3's minimum contract onto each candidate: where are the impedance mismatches (mid-run pause? checkpoint granularity? usage reporting fidelity?) and how do real projects bridge them?
6. Single engine + CLI-wrap hybrid vs multiple engines: what do comparable control planes run in production, and what does each choice cost a solo maintainer?
7. Local-model path through the same adapter (OpenAI-compatible serving) — confirm candidates handle local providers cleanly.
8. License audit table for every candidate (dependency-adoption compatibility; flag fair-source/BUSL-class traps).

## Constraints that bind this topic
D2 (runs authenticate as their owner — engine must support per-run credential context without pooling), D3, D6 (engine's sub-agent model must be constrainable to the topology), D7 (checkpoint after every paid call — engine must expose enough state), adopt-don't-fork (absolute), no load-bearing metered paths.

## Harvest-map items to verdict
O1–O4 (opencode engine/providers/config formats/ACP), C1–C5 (OpenClaw patterns), reference rows OpenHands/Goose (fallback engines), Crush K-rows (license caveat), anti-harvest "8 core-mods" row (confirm the keystone — per-session models — is truly native in the recommended engine).

## Sources to prioritize
Provider ToS/pricing/docs pages (primary, dated); engine repos + release notes + maintainer statements; issue trackers for wrapping/drift war stories; practitioner writeups of subscription-based automation (mid-2026); comparisons from framework-neutral sources.

## Decisions this feeds
G1: engine direction (the gate's headline decision), adapter-contract shape, which substrate wraps vs calls. Spec: adapter layer section. T07/T08/T09 assumptions.
