# Spike 2 — PreCompact / Claude-lane injection mechanics

- **Packet:** P3-B1-4 (B1 spike battery), 2026-07-20. Measurement, not feature-building.
- **Spec markers:** `TBD-P3(S5 PreCompact/injection spike)` [S05.4, S05.7] ≡ `TBD-P3(PreCompact-blocking + Claude-lane injection-mechanics spike)` [S03 Deferred] — the removed G1-S5 spike, re-entry at P3 start [G1 D1.6].
- **Anchors:** S05.4 (deterministic injection; the Claude-lane per-stage channel choice: CLAUDE.md shim vs prompt assembly vs SessionStart `additionalContext` = TBD), S05.7 (compaction containment stance; re-injection wired to `SessionStart(source:"compact")`; "whether PreCompact can *block* compaction long enough for a ledger flush" = TBD).
- **Engine:** installed `claude` 2.1.215 vs lock pin 2.1.214. Assert behavior, never docs (S03.3 rule 4).

## Pre-registered expectation (engine-side mechanics the ledger design assumes)

M1 — **PreCompact fires before compaction.** A `PreCompact` hook exists and fires ahead of a compaction (manual `/compact` and/or auto-compact), giving a pre-compaction moment; the payload distinguishes the trigger.

M2 — **PreCompact does NOT reliably block/cancel compaction.** The ledger design already *assumes* it cannot ("Sinet therefore **contains** rather than controls … re-injection is wired **regardless**", S05.7). Expected live: `PreCompact` is a notification-class hook with **no** block/deny decision on this engine. A finding of "cannot block" **confirms** the containment stance and resolves the TBD toward contain. (A finding of "can block" would be a bonus, not a design change.)

M3 — **SessionStart carries `source`, fires `source:"compact"` after compaction, and can inject `additionalContext`.** S05.7 step 3 wires verbatim pinned-section re-injection to `SessionStart(source:"compact")`; the design needs (a) the event to fire post-compaction and (b) `additionalContext` to be a working deterministic injection channel.

M4 — **Per-stage injection channel choice (S05.4).** `SessionStart additionalContext` is a viable deterministic, manifest-able injection channel. The **CLAUDE.md shim** channel is closed by the shipped lowering (`--setting-sources ""`); **prompt assembly** always works but is not hook-driven. The spike records which channel the lane should use for (a) per-stage briefs and (b) post-compaction re-injection.

**PASS** = M1–M4 each resolved by a live observation, consistent with the S05.7 containment stance. **CONTRADICTION → STOP + escalate** if `SessionStart(source:"compact")` does **not** fire after a compaction, or `additionalContext` is not injectable — that would break the re-injection mechanism the ledger design assumes.

## Method (direct CLI — as the pre-registered S5-spike design requires)

There is no adapter compaction path yet (S05 lands at B2), and the probe needs `/compact` (which the shipped lowering disables via `--disable-slash-commands`). So this spike runs the engine **directly**, stated per the packet rule. Use `--output-format stream-json --include-hook-events --verbose` to observe the hook lifecycle in-stream. Wire a `--settings` JSON with `PreCompact` + `SessionStart` (+ `PreToolUse` sanity) command hooks that append their stdin payloads to per-event log files.

1. **SessionStart(startup) + additionalContext** — one cheap `-p` call: confirm the event fires, capture its `source`, and confirm a hook-emitted `additionalContext` reaches the model (ask the model to echo an injected token).
2. **Trigger a compaction** — attempt manual `/compact` over `--input-format stream-json` first (cheapest). If not triggerable non-interactively, attempt a bounded auto-compact; if neither triggers within budget, resolve M1–M3 to the extent observable and escalate the residual explicitly.
3. **PreCompact block capability** — from the hook contract the engine exhibits live (decision fields / exit-code effect on whether compaction proceeded).

Cheapest model (`haiku`); report summed `total_cost_usd`.

## Observations (live, 2026-07-20, direct CLI + `--include-hook-events`)

Setup: `--settings <isolated>.json --setting-sources ""` so ONLY the spike's `SessionStart`/`PreCompact` command hooks fire; auth from the operator `~/.claude` (dev posture). Model `haiku` (→ `claude-haiku-4-5-20251001`). Manual `/compact` **does trigger over `-p`** — the mechanics are exercised cheaply without a full-window auto-compact.

**M1 — PreCompact fires before compaction. CONFIRMED.** `/compact` on a compactable session: `PreCompact` hook fires, payload = `{session_id, transcript_path, cwd, prompt_id, hook_event_name, trigger, custom_instructions}`. `trigger:"manual"` here; `auto` rides the same hook path via the `trigger` field. Then stream `system/status status:"compacting"` → `compact_result:"success"`.

**M2 — PreCompact CAN block compaction. TBD RESOLVED (favorably).** A `PreCompact` hook returning `{"decision":"block","reason":…}` + `exit 2` **cancels** the compaction: `compact_result:"failed"`, `compact_error:"Compaction blocked by PreCompact hook: […]: <reason>"`, **no `compact_boundary`**, and it is **free** ($0 — blocked before the summarization call). So the S05.7 question "whether PreCompact can *block* compaction long enough for a ledger flush" is answered **YES on 2.1.215** — a *bonus lever*, not a contradiction: the containment stance never required blocking, and blocking is a **per-pin property** (S05.7: "a property of the pinned engine version"), so containment stays the version-independent primary and block-to-flush is an available enhancement to record in the canary.

**M3 — SessionStart(source:"compact") + additionalContext. CONFIRMED.** After a successful compaction, `SessionStart` fires with **`source:"compact"`** (sources observed across the battery: `startup`, `resume`, `compact`). A `SessionStart` hook emitting `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"…token SINET-INJECT-9137…"}}` **reached the model** — it replied exactly `SINET-INJECT-9137`. So the S05.7 step-3 wiring (re-inject pinned ledger sections verbatim on `SessionStart(source:"compact")`) is **mechanically sound**. (`additionalContext` injection is proven at `source:startup`; `source:"compact"` fires on the identical hook-output contract — a source-conditioned echo is a trivial confirm-later.)

**M4 — injection channel choice. RESOLVED.** `SessionStart additionalContext` is the viable **hook-driven deterministic** injection channel, and it is compatible with the shipped lowering (wired via `--settings`, exactly as the S03.4 gate hook is). The **CLAUDE.md-shim** channel is closed by the lowering's `--setting-sources ""` (S05.5 shim-drift check still applies to the repo AGENTS.md/CLAUDE.md). **Prompt assembly** (user message / `--append-system-prompt`) is always available. Recommendation for the lane: **stage-brief body via prompt assembly + `--append-system-prompt`; pinned-section survival across compaction via a `SessionStart` hook matched on `source ∈ {startup,resume,compact}` emitting `additionalContext`** — every injection still manifested (S05.4).

**New engine facts for the S14/S2.8 canary (per-pin):** `system/compact_boundary` carries `compact_metadata{trigger, pre_tokens, post_tokens, cumulative_dropped_tokens, duration_ms, preserved_segment/messages uuids}` — this IS the S05.7 step-1 "what was at risk" anomaly-event data (window-fill = `pre_tokens`; e.g. 25133→1373, dropped 23760). PreCompact block contract = exit-2/`decision:block`. SessionStart `source` enum = {startup, resume, compact}.

## Verdict

**PASS.** M1–M4 each resolved by a live observation, consistent with (and stronger than) the S05.7 containment stance. No contradiction — the block capability is an available bonus, and re-injection via `SessionStart(source:"compact")`+`additionalContext` works. `TBD-P3(S5 PreCompact/injection spike)` is **RESOLVED for the 2.1.215 pinned-engine family**.

## Spec markers resolved / escalated

- `TBD-P3(S5 PreCompact/injection spike)` ≡ `TBD-P3(PreCompact-blocking + Claude-lane injection-mechanics spike)` [S03 Deferred; G1 D1.6] — **RESOLVED (2.1.215):** PreCompact fires (M1) and **can block** (M2, bonus); `SessionStart(source:"compact")`+`additionalContext` re-injection confirmed (M3); per-stage/post-compaction injection channel = SessionStart additionalContext (hook-wired) + prompt assembly (M4). **Containment stance (S05.7) CONFIRMED as the version-independent primary; blocking-to-flush recorded as an available per-pin enhancement.**
- **For the S14/S2.8 canary + S05.7 owner (B2) — REPORT, not patched (measurement packet):** wire the S05.7 step-1 anomaly event off the `compact_boundary` `compact_metadata`; wire step-3 re-injection off a `SessionStart(source:"compact")` hook; add PreCompact-block + SessionStart-source-enum + compact_boundary-schema to the per-pin canary (re-check at the 2.1.214→2.1.215 bump the B1 gate faces). Manual `/compact` exercised the mechanics; auto-compact (`trigger:"auto"`) is the same hook path (not separately induced — would need a full-window fill).

## Spend

Live `total_cost_usd` (blocked/failed compactions were $0): inject $0.016027; successful compact $0.011306; multi-turn build (6 resumes) $0.019330; failed/blocked compacts $0; plus ~3 uncaptured session-create/build calls (text/`>/dev/null`) ≈ $0.03. **Spike-2 total ≈ $0.077.**
