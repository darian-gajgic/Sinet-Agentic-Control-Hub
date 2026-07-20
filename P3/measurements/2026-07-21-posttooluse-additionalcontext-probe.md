# Probe — PostToolUse `additionalContext` on the installed engine (C1-recitation channel)

- **Packet:** P3-B3-4 (codor C1-recitation + C3 rows), 2026-07-21. Measurement before feature-building — the pre-registered probe of Research/18 §7-C1 condition 5 and the P3/STATE standing directive (2026-07-20 codor harvest).
- **Question:** does a settings-compiled **PostToolUse** command hook deliver `hookSpecificOutput.additionalContext` to the model **mid-turn** in `-p` stream-json on the installed engine? PostToolUse appears nowhere in spec or P3 before this packet, and codor's proof runs on the Agent-SDK path, not `-p` — so Sinet probes, never assumes (S03.3 rule 4).
- **Engine:** installed `claude` **2.1.216** vs lock pin **2.1.215** — the same pin↔installed delta P3-B3-3 reported (CONVENTIONS §10: assert behavior at the installed engine, report LOUDLY, never silently retarget; reconciling is the operator's S03.3 deliberate-bump decision, already a B3 gate item).
- **Method:** B1-4 method (`P3/measurements/2026-07-20-precompact-injection-mechanics.md`), direct CLI — there is no adapter PostToolUse path yet (this packet builds it on PASS). Isolated `--settings <probe>.json --setting-sources "" --strict-mcp-config`, `--tools Bash`, model `haiku`, `--max-turns 6 --max-budget-usd 0.05` belts, `--include-hook-events` to observe the hook lifecycle in-stream; auth from the operator `~/.claude` (dev posture, as B1-4). Hook = shell command appending its stdin payload to a log, then emitting the leg's stdout. Prompt (both legs): run `echo PROBE-STEP` via Bash once, then reply with a delivery token of the form `SINET-RECITE-<digits>` if one arrived, else exactly `NONE`. The token appears ONLY in the inject-leg hook stdout — never in any prompt.

## Pre-registered expectations

- **P1 — PostToolUse fires** after a matched tool call in `-p` stream-json (payload observable; capture its field set).
- **P2 — `additionalContext` reaches the model mid-turn:** a hook emitting `{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"…token…"}}` gets the token into the model's context after the tool boundary, inside the same session turn flow (echo test — the B1-4 M3 method).
- **P3 — the quiet path is silent:** a hook printing NOTHING (empty stdout, exit 0) injects zero context and does not disturb the turn (codor's "a quiet inbox emits zero stdout and therefore injects no context" contract, asserted on Sinet's engine surface).

**PASS** = P1+P2+P3 → implement the per-turn recitation path + a per-pin canary row. **FAIL** (P2 false) → stage-boundary-only recitation fallback — both outcomes spec-satisfying (S05.3 at ⚙ 0-off). Stop rule: projected cost > ~$0.10 → STOP and report.

## Observations (live, 2026-07-21)

**P1 — CONFIRMED.** PostToolUse fired after the Bash call on both legs (`system/hook_started` + `system/hook_response` frames under `--include-hook-events`, `hook_name:"PostToolUse:Bash"` — matcher = exact tool name, the same alternation semantics the S03.4 PreToolUse gate uses). Captured stdin payload field set (both legs, dated engine fact for the per-pin canary):
`{session_id, transcript_path, cwd, prompt_id, permission_mode, hook_event_name:"PostToolUse", tool_name, tool_input, tool_response, tool_use_id, duration_ms}` — the PreToolUse set (SPIKE G1-S1/G1-S2) **plus `tool_response` and `duration_ms`**.

**P2 — CONFIRMED.** Inject leg: final result envelope `"result":"SINET-RECITE-7741"` — exactly the token the hook's `additionalContext` carried; `num_turns:2`, `is_error:false`, `stop_reason:"end_turn"`. The token existed nowhere but the hook stdout, so the injection reached the model after the tool boundary, mid-session. Cost $0.0135704.

**P3 — CONFIRMED.** Quiet leg: hook fired (782-byte stdin payload logged; `hook_response` `output:""`), printed nothing → model replied exactly `"NONE"`, `subtype:"success"`, `num_turns:2` — zero injection, zero disturbance. Cost $0.0135144.

## Verdict

**PASS** — all three pre-registered expectations confirmed live on 2.1.216. The per-turn recitation path is buildable exactly as the Research/18 §7-C1 coherence check shaped it: platform-authored pending-file on the S03.4 ctl-dir airlock → `sinet engine-hook` PostToolUse trigger → `additionalContext`; the quiet path costs zero context.

**Per-pin canary rows (S14/S2.8, beside the B1-4 SessionStart rows — machinery lands B5; recorded here + executable as the `SINET_B3_4=1`-gated live test in `internal/adapters/claudecli`):**
1. PostToolUse command hooks fire per matched tool call in `-p` stream-json; matcher = tool-name alternation (same as PreToolUse).
2. PostToolUse stdout contract: `hookSpecificOutput.additionalContext` reaches the model mid-turn; **empty stdout injects nothing** (the delivery valve's quiet path).
3. PostToolUse stdin payload = PreToolUse fields + `tool_response` + `duration_ms`.
4. Hook lifecycle observable as `system/hook_started`/`hook_response` frames under `--include-hook-events` (probe instrumentation only — the shipped lowering does not enable it).

Re-check all four at every S03.3 pin bump (the pending 2.1.215→2.1.216 bump decision re-runs this file's gated test).

## Spend

Inject $0.0135704 + quiet $0.0135144 = **$0.0270848 total** (~$0.027, vs the ~$0.02 estimate and the $0.10 stop line). Raw streams + hook stdin logs retained in session scratchpad only; this file is the durable record (B1-4 precedent).
