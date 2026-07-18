# P2-S4 — Serialize-by-deny: converting the parallel-tool-call fallback into a cheap extra turn

**Spike battery:** P2 (spec-readiness) · **Carries:** S2 obligation 2 / blocked item "serialize-by-deny fallback candidate"
**Date:** 2026-07-18 · **CLI:** claude v2.1.214 (operator Max subscription) · **Model:** `claude-haiku-4-5` (cheap; alias `haiku-4-5` 404s — full name required) · **Host:** bench Linux 7.0.0-27-generic
**Scratch:** `…/c8abf96d-…/scratchpad/spike-s4/` (hooks, settings, fixtures copied from S2, raw probe outputs in `out/`)

## Scope

S2 measured that ~20% of gated turns on the default model emit ≥2 parallel tool calls, which cannot exit-park via `defer`, and *designed but could not afford to test* **serialize-by-deny**: on a parallel gated turn, deny every call with the reason **"re-issue the gated call alone"** so the model's single-call retry hits `defer` and parks — converting the fallback from a held process into one extra cheap turn. This spike settles it with live probes on a cheap model. One question: does the model respond to deny-all by re-issuing a *single* call, does that single call then park cleanly, and at what turn/cost delta. Empirical probes only; no application code, no system changes.

## Method

Fixtures reused from S2 (`fixtures/{a,b,c}.txt`, single-line text files). All probes `claude -p --model claude-haiku-4-5 --output-format json`, cwd = fixtures dir, `--max-turns ≤6 --max-budget-usd 0.10`. Trigger that reliably produces a parallel batch on haiku (confirmed by an ungated smoke: 3 `tool_use` Reads in one assistant `message.id`):

> **Prompt (verbatim):** `Read the files a.txt, b.txt, and c.txt from the current directory and report the first line of each.`

Three PreToolUse hook variants, all `"matcher": "*"`. Settings shape (verbatim, serialize variant):

```json
{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"<abs>/serialize2-hook.sh"}]}]}}
```

- **deny-all** (`denyall-hook.sh`): denies every call, reason `"re-issue the gated call alone"` (isolates the model's raw retry shape).
- **serialize v1** (`serialize-hook.sh`): reads the transcript, counts `tool_use` blocks in the assistant message that emitted the pending call (by `tool_use_id`), deny if >1 else defer. *(Detector defeated by transcript lag — see Finding 2; functionally ran as always-defer.)*
- **serialize v2** (`serialize2-hook.sh`): the working detector. Decision core, verbatim:

```sh
now="$(date +%s.%N)"
printf '{"pid":"%s","tuid":"%s","ts":%s}\n' "$pid" "$tuid" "$now" >> "$B"   # register this fire
sleep 0.9                                                                    # look-ahead window
cnt="$(jq -s --arg pid "$pid" --argjson t "$now" \
  '[.[]|select(.pid==$pid)|select(((.ts-$t)|if .<0 then -. else . end)<=1.0)]|length' "$B")"
# cnt>1  -> deny "re-issue the gated call alone"   (parallel batch member)
# cnt==1 -> defer "…parked awaiting operator"      (serialized single call)
```

Probe order: **S0** ungated smoke (confirm parallelism) → **P1** serialize-v1 (became always-defer) → **P1-resume** allow-all (lossiness/self-heal) → **P2** deny-all (retry shape) → **concurrency probe** (1 s-sleep hook: do sibling hooks overlap?) → **P3** serialize-v2 (full mechanism) → **P3-resume** serialize-v2 (drain) → **P4** serialize-v2 replicate (determinism). Costs are API-equivalent `total_cost_usd` from result JSON (Max subscription — no marginal cash).

## Findings

### 1. Serialize-by-deny works end-to-end — VIABLE (P3, replicated P4)

Under serialize-v2, on the parallel gated turn all three Reads were detected as batch members (`batch_count=3`) and **denied** with `"re-issue the gated call alone"`. The model's very next turn re-issued a **single** Read; the hook saw `batch_count=1` and **deferred** → clean park:

```
fires:  Read(a) batch_count=3 deny · Read(b) 3 deny · Read(c) 3 deny · Read(a) batch_count=1 defer
exit:   stop_reason=tool_deferred, terminal_reason=tool_deferred, result=""
        deferred_tool_use = {name:"Read", input:{file_path:".../a.txt", limit:1}}
```

**Observed retry shape (verbatim model text, P3 turn 2):** *"I need to call the Read tool for each file individually. Let me do that:"* — the model reads the deny reason and serializes on its own. P4 reproduced the trajectory identically (3×deny → 1×defer → park on `a.txt`). The parked `deferred_tool_use` is a **faithful single-call ask record** (contrast Finding 4). **Verdict: VIABLE.**

### 2. Transcript-based parallel-detection is broken by flush lag (kills serialize-v1)

PreToolUse fires **before** the current assistant message's `tool_use` blocks are written to the transcript. Serialize-v1's fires all logged `sibs_byid:0, sibs_last:0` — the hook could see *neither* its own call *nor* its siblings. Worse, on a serialized retry the last *flushed* assistant message is the previous (parallel) turn, so a "count last assistant message" heuristic reads a stale `3` and would wrongly deny the single retry, looping. **A hook must not classify the current turn by reading the transcript at fire time.** (Robust alternative in Implications.)

### 3. Parallel sibling hooks run CONCURRENTLY — enabling the look-ahead detector

A 1 s-sleep probe hook on the 3-Read batch produced overlapping execution intervals (start offsets 0.00 / 0.25 / 0.61 s; all three still running at 1.0 s; distinct PIDs). The engine dispatches sibling PreToolUse hooks concurrently, staggered ~0.25–0.35 s. So a call that registers its fire and sleeps ~0.9 s will observe its siblings' registrations — including the *first* call of the batch, whose siblings launch during its sleep. This is what makes serialize-v2 work. **Cost:** +0.9 s latency on *every* gated call, and the detector rests on an engine hook-concurrency timing property that is not contractual (could shift with version/load) — a robustness liability, not a correctness one.

### 4. Reframe — on 2.1.214/haiku, always-defer ALREADY parks a parallel turn (S2 Finding 3 did not reproduce)

Serialize-v1, deferring all three parallel calls (its detector saw 0), did **not** bypass-and-execute. The engine **parked**: `stop_reason=tool_deferred`, one `deferred_tool_use` (the *last* call, `c.txt`), **no read executed** (no `tool_result`/PostToolUse in the transcript). This contradicts S2 Finding 3 ("the defers were bypassed and the parallel Reads executed") measured on **2.1.212 / fable-5**. The park is **lossy but self-healing**: on `--resume` with allow (P1-resume), only the parked `c.txt` re-fired and executed; the model, seeing it lacked `a.txt`/`b.txt`, re-issued them as a fresh pair, which executed → correct final answer (all three first lines), 3 turns, $0.0159.

Consequence: the S2 premise that a parallel turn *cannot* exit-park (motivating a **held process**) does **not** hold on this build — `defer` parks it directly. **Caveat:** measured only on `claude-haiku-4-5`; not reconfirmed on the default model (`fable-5` budget-kills mid-turn at the $0.10 cap, per S2 Finding 4), which is the model that actually parallelizes ~20% of the time. Whether fable-5 still bypasses on 2.1.214 is untested here.

### 5. Deny-all *without* the defer step loops then fails (P2) — the single-retry defer is load-bearing

Pure deny-all (deny even the single retry) is **not** a viable fallback. The model serialized once — *"The Read tool requires individual permission for file access. Let me issue the calls separately:"* (single Read) — but that was denied too, so it switched tools (*"Let me use Bash to read these files instead:"* → single Bash), was denied again, and after 7 turns **gave up**: *"…blocking both Read and Bash access with a 're-issue the gated call alone' error… You'll need to grant permission…"*. The mechanism only works because the parallel batch is **denied** and the single retry is **deferred** (parks before the model wanders). serialize-v2 supplies exactly that transition.

### 6. Turn/cost deltas (the operative comparison)

| Fallback strategy | Extra API turns vs. plain single-call park | Cost to reach park | Ask record fidelity | Live process held? |
|---|---|---|---|---|
| **Held-process** (S2's alternative) | 0 | $0 extra | n/a (never parks) | **Yes** — OS/session held for hold duration |
| **Always-defer** (P1) | 0 (parks on the parallel turn itself, `num_turns=1`) | **$0.0131** | 1-of-N (siblings dropped, re-derived on resume) | No |
| **Serialize-by-deny** (P3 / P4) | **+1** (the deny turn) | **$0.0269 / $0.0247** | **Faithful single call** | No |

Serialize-by-deny costs **~+$0.012–0.014 and one model turn** over always-defer, and over held-process it adds that same turn but **saves holding a live process**. Re-defer while parked is **free** ($0, no API call — P3-resume, matching S2 F2b): polling a parked serialize session costs nothing.

## Verdict

**Serialize-by-deny is VIABLE for v0** — demonstrated deterministically (P3, P4) on the cheap model: deny the parallel batch → the model re-issues a single call on its own → that call defers and parks with a faithful one-call `deferred_tool_use`, for one extra cheap turn (~$0.013). Two caveats bound the recommendation:

1. **Detection method matters.** The in-hook timing look-ahead (Finding 3) works but is fragile (per-call +0.9 s latency; leans on non-contractual hook-concurrency timing). Prefer **post-park transcript reconstruction** (Finding 2/Implications): it is robust and needs no fire-time guessing.
2. **Model-sensitivity.** Retry-serialization was only shown on `claude-haiku-4-5`; the default worker model (which drives the 20% rate) must be re-checked.

And the larger point (Finding 4): on 2.1.214 **always-defer already parks parallel turns**, so serialize-by-deny is likely **not required for the park itself** — its distinct value is a *faithful* single-call ask record instead of always-defer's 1-of-N.

## Implication for the T07 adapter fallback path (vs. held-process parking)

- **Drop held-process from the design.** On 2.1.214 `defer` parks a parallel turn (Finding 4); there is no need to hold a live OS process to keep the turn recoverable. (Re-confirm on the default model before finalizing — see caveat.)
- **Default fallback = always-defer.** Simplest and free: park on the parallel turn, capture the exit-JSON `deferred_tool_use`, resume-with-allow when approved. The model self-heals the dropped siblings on resume (P1-resume). Accept that the operator's ask shows 1-of-N and the siblings re-execute (re-gated) on resume.
- **Serialize-by-deny = opt-in refinement** where a faithful single-call ask record matters (e.g., a batch mixing a benign read with a destructive write — always-defer might park the read and drop the write into a resume-time re-issue). Cost: +1 turn per batch; fully draining an N-call batch = N approve/resume cycles, one faithful call each.
- **Detection contract (correction to S2's "fire-count vs exit-reason"):** do **not** read the transcript at fire time to classify the current turn (lag, Finding 2). Detect a parallel park **after** exit: the parked transcript's last assistant message retains all N `tool_use` blocks even though `deferred_tool_use` reports one — reconstruct the full batch from there for the ask record, or trigger serialize-by-deny on resume. This is robust and version-independent.
- **Correct S2 Finding 3 for this build:** parallel `defer` was **not** silently bypassed-and-executed on 2.1.214/haiku; it parked. S2 obligation 2 downgrades from "first-class fallback path sized at ~20%" to "faithful-ask-record refinement" *if* the default model behaves like haiku here — a required re-measurement, not an assumption.

## Per-probe cost table

All probes `--max-budget-usd 0.10 --max-turns ≤6`, `claude-haiku-4-5`, cwd = fixtures. Costs = `total_cost_usd`.

| Probe | What | Turns | Result | Cost |
|---|---|---|---|---|
| S0 smoke (v1) | `haiku-4-5` alias | — | 404 not-found (alias invalid) | $0.0000 |
| S0 smoke (v2) | `claude-haiku-4-5`, ungated read-3-files | 4 | 3 parallel Reads confirmed | $0.0319 |
| P1 | serialize-v1 (→ always-defer), read-3-files | 1 | **parked** on `c.txt` (siblings dropped) | $0.0131 |
| P1-resume | allow-all resume of P1 park | 3 | completed; model re-derived a/b, self-healed | $0.0159 |
| P2 | deny-all, read-3-files | 7 | serialized once, wandered to Bash, gave up | $0.0293 |
| concurrency | 1 s-sleep hook (overlap test) | 3 | sibling hooks run concurrently | $0.0248 |
| P3 | **serialize-v2**, read-3-files | 4 | **deny×3 → defer×1 → clean park on `a.txt`** | $0.0269 |
| P3-resume | serialize-v2 resume (re-defer) | — | re-parked, free (no API call) | $0.0000 |
| P4 | serialize-v2 replicate | 4 | reproduced P3 exactly | $0.0247 |
| **Total** | 7 paid runs | | | **$0.1665** |

## Blocked / not attempted

- **Default-model (fable-5) re-measurement of Findings 4 & 1** — out of scope (cheap-model mandate) and uncleanly testable at the $0.10 cap (fable budget-kills mid-turn, S2 F4). This is the single most important follow-up: it decides whether the parallel fallback is still a real problem on 2.1.214 at all.
- **Full N-cycle serialize drain under gating** — first cycle shown (P3 + P1-resume mechanics); remaining cycles follow by composition, not separately probed.
