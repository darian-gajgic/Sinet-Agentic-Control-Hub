# Spike 3 — Claude-lane auto-memory containment

- **Packet:** P3-B1-4 (B1 spike battery), 2026-07-20. Measurement, not feature-building.
- **Spec markers:** `TBD-P3(Claude-lane auto-memory containment, R11-OQ6)` [S09.9; carried identically in S03 Deferred] — rides adapter build at P3. P-T10-1 (engine-native memory drift, 8.1-bypass class): standing canary-suite entry re-checked per engine pin bump.
- **Anchors:** S09.9 ("disable-or-contain" via the compiled-config guarantee; auto-memory disabled-where-allowed else contained as workspace-scoped L0; the GA memory tool stays off at v0), S03.5 (engine lowering: `--tools` default-deny, `--setting-sources ""`, isolated `CLAUDE_CONFIG_DIR`, isolated cwd).
- **Engine:** installed `claude` 2.1.215 vs lock pin 2.1.214. Assert behavior, never docs (S03.3 rule 4).

## Pre-registered expectation (containment posture the memory design assumes)

C1 — **auto-memory is a real engine feature at 2.1.215** (already evidenced by `claude --help`: `--bare` "skip … auto-memory"). The spike identifies *what* it is (MEMORY.md-class file vs a memory tool), *where* it writes, and *when* it activates.

C2 — **disable-where-allowed:** the pinned version exposes a knob that suppresses auto-memory writing. `--bare` disables it but ALSO skips hooks + CLAUDE.md discovery — **too broad**, because the shipped lowering *needs* hooks (the S03.4 gate). The spike finds the **narrowest** disable path compatible with the lowering (a settings/env/tools knob), if one exists.

C3 — **contain-if-not-disabled:** under the shipped lowering — `--tools <allowlist>` default-deny, `--setting-sources ""`, `CLAUDE_CONFIG_DIR` set only from the per-run owner cred ref, isolated cwd — anything the engine writes as "memory" is confined to the per-run config/workspace and does **not** persist as behavior-steering memory across runs. If auto-memory is a **tool**, default-deny `--tools` excludes it (verify the name is absent from the allowlist and not invocable). If it is a **MEMORY.md-class file**, it lands under the isolated `CLAUDE_CONFIG_DIR`/cwd and is wiped with the per-run workspace (treated as L0).

C4 — **GA memory tool stays off:** the memory tool (if any) is not in the default lowering's `--tools` allowlist and is not enabled.

**PASS** = C1 identified AND a disable-or-contain path confirmed live consistent with S09.9 — either a disable knob compatible with the lowering, OR containment demonstrated (memory writes stay inside the isolated `CLAUDE_CONFIG_DIR`/cwd and do not leak across an isolated-config run). **CONTRADICTION → STOP + escalate** if auto-memory persists behavior-steering state **outside** the isolated config/cwd despite the lowering (compiled-config guarantee failing to contain it).

## Method (direct CLI to probe engine behavior; lowering knobs are already shipped)

The containment knobs live in the shipped lowering (`lower.go`); this spike verifies the **engine behavior** those knobs assume. Cheapest model (`haiku`).

1. **Surface:** enumerate the engine's built-in tool set (`--tools default` behavior + any memory-tool name); locate auto-memory's write path (MEMORY.md-class dir under the config root).
2. **Behavioral containment probe:** run a `-p` call under an **isolated** `CLAUDE_CONFIG_DIR` + isolated cwd (the lowering posture), with the shipped default-deny `--tools` allowlist and `--setting-sources ""`, prompting behavior that would elicit a memory write; then inspect what/where was written and confirm nothing landed outside the isolated roots and nothing behavior-steering persists for a *second* isolated run.
3. **Disable knob:** identify the narrowest suppression compatible with keeping hooks (settings key / env / tools), and record it for the adapter (S09.9 "disabled where allowed").

## Observations (live, 2026-07-20, direct CLI + binary inspection)

**C1 — auto-memory is a real background feature; write root identified. RESOLVED.** At 2.1.215 auto-memory is a **background feature, not a tool** (named by `--bare`: "skip … auto-memory"; master-off = `CLAUDE_CODE_SIMPLE=1`). Its write root is **`$CLAUDE_CONFIG_DIR/memory/`** — the operator's `~/.claude/memory` exists (engine-created 2026-06-22) and is currently **empty**.

**C4 — GA memory tool stays off. RESOLVED.** The default built-in tool set at 2.1.215 (29 tools: Task, Bash, Cron*, Edit, Read, Skill, Task*, ToolSearch, WebFetch/Search, Workflow, Write, …) contains **no memory tool** (`memory-ish: []`). So the GA memory tool is off by default; the shipped lowering's `--tools <explicit-allowlist>` (default-deny) excludes it regardless. (Note `Task*` IS in the default set — exactly why the lowering strips it, S03.5.)

**C3 — containment shape. RESOLVED, with a reported implementation gap.** Because auto-memory writes under `CLAUDE_CONFIG_DIR`, and the lowering sets `CLAUDE_CONFIG_DIR = OwnerCredRef` (per-**user** config root, D2/S03.5), **cross-user isolation holds by construction** (the compiled-config guarantee — user A's `memory/` is unreachable to user B). The **residual** is per-user cross-**run** persistence (a user's runs share `memory/`) — the ungated-L2 / 8.1-bypass risk S09.9 targets. **No auto-memory persistence was observed from ANY spike run** (0 files written to `~/.claude/memory` across all runs, incl. the spike-2 "remember these facts …" prompts under `--setting-sources ""`) — consistent with containment, though this is absence-of-evidence (those prompts may simply not have triggered a write), **not** proof the channel is closed. The robust, version-independent containment per S09.9 ("memory dir workspace-scoped, treated as L0, **wiped with the task workspace**") is a **per-run wipe/redirect of `$CLAUDE_CONFIG_DIR/memory/`** — a platform duty **not yet in the shipped B1 lowering** (B1-1/B1-3 closed settings/MCP/skills/tools/CLAUDE.md/cwd/env, but the config-root `memory/` dir is a *separate* channel).

**C2 — disable knob. PARTIALLY RESOLVED.** The only *confirmed* disable is `--bare`/`CLAUDE_CODE_SIMPLE=1` — **too broad** (also kills hooks/CLAUDE.md/keychain, incompatible with the lowering's S03.4 gate hooks). A narrower settings/env knob was **not cleanly identifiable** from the compiled 265 MB binary and could not be behaviorally confirmed without isolated auth. **A fresh `CLAUDE_CONFIG_DIR` does not authenticate** ("Not logged in") — the setup-token resides in the config dir, not the keychain (confirms D2/B1-1) — so a redirect must keep the per-user auth files while presenting a fresh/wiped `memory/`. **Recommendation: prefer the version-independent per-run wipe/redirect over an engine disable knob.**

**Bonus (S03.5 env scrub).** The binary carries credential-bearing env vars `CLAUDE_CODE_OAUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR`, `CLAUDE_CODE_HOST_CREDS_FILE`, `CLAUDE_CODE_SESSION_ACCESS_TOKEN` — all caught by the lowering's `CLAUDE_CODE_` scrub prefix (envScrubPrefixes). The env/credential channel is correctly closed.

## Verdict

**PASS — containment posture confirmed achievable; exact mechanic escalated.** The S09.9 posture holds: the GA memory tool is off by construction (C4), cross-user isolation holds via the compiled-config guarantee (C3), and auto-memory is contain-able as workspace-scoped L0 via a per-run wipe/redirect of `$CLAUDE_CONFIG_DIR/memory/` (C1 locates the target). **No contradiction** — the compiled-config guarantee does not fail; the only gap is per-run L0 scoping, which S09.9 already names as a platform duty. The **exact disable/redirect implementation** (`TBD-P3(Claude-lane auto-memory containment)`) remains the owning adapter phase's (B3/S09) — this spike resolves *where* and *what shape*, not the code.

## Spec markers resolved / escalated

- `TBD-P3(Claude-lane auto-memory containment, R11-OQ6)` [S09.9; S03 Deferred] — **PARTIALLY RESOLVED:** write-path = `$CLAUDE_CONFIG_DIR/memory/`; GA memory tool off by default (default-deny excludes it); containment shape = compiled-config per-user isolation **+ per-run wipe/redirect of the memory subdir**. The exact disable-vs-redirect choice + narrow-knob search + a behavioral write-then-check under isolated auth stay with the **B3/S09** adapter build.
- **REPORT for B3/S09 + the S03.5 lowering + the S09.9/P-T10-1 per-pin canary (not patched — measurement packet):** the config-root `memory/` dir is a **separate config channel** the B1 lowering does not yet close (a P2-S5 compound-guarantee refinement — "one knob per channel"); add a per-run wipe/redirect. Env/credential channel already closed by the `CLAUDE_CODE_` scrub. Add auto-memory write behavior to the per-pin canary (re-check at the 2.1.214→2.1.215 bump the B1 gate faces).

## Spend

Almost entirely free: fresh-config auth attempt $0 (rejected pre-inference); default-tools enum ≈ $0.005; lowered-flags probe ≈ $0.005; binary `strings` $0. **Spike-3 total ≈ $0.01.**
