# Kimi Code CLI print-mode spike — R0 of P3-LN-7

**Date:** 2026-08-26 · **Cost: $0** · **Zero provider calls, zero logins, zero quota consumed.**

Every model call in this spike terminated on a **loopback fake OpenAI-compatible provider** on
`127.0.0.1`, reached through the `KIMI_MODEL_*` channel. As a belt, every probe ran with
`HTTP_PROXY=HTTPS_PROXY=ALL_PROXY=http://127.0.0.1:1` (loopback is always proxy-bypassed), so any
attempt to reach a real endpoint would have failed at connect rather than authenticated. The
operator's own data root `~/.kimi-code` — which holds live credentials — was **never read and never
written**: every probe ran under its own `KIMI_CODE_HOME` and its own bounded `HOME`, both inside a
scratch directory.

**Engine under test:** `@moonshot-ai/kimi-code@0.38.0`, installed user-level from npm
(`npm install -g @moonshot-ai/kimi-code@0.38.0`, `KIMI_CODE_NO_AUTO_UPDATE=1`) at
`/home/sinep/.npm-global/lib/node_modules/@moonshot-ai/kimi-code`, bin shim
`/home/sinep/.npm-global/bin/kimi`. `kimi --version` → `0.38.0`. Node v22.22.1.

**Method note.** Every answer below is **answered by execution**. Where a vendor document and an
observation disagree, the observation is recorded as binding (S03.3 rule 4: suites assert behavior,
never docs) and the contradiction is named rather than smoothed over. Two doc statements were
falsified; both are called out under Q7 and Q9.

---

## The ten pre-registered questions

### Q1 — the complete `--output-format stream-json` envelope

**Six distinct frame shapes were observed** across a plain turn, a tool-calling turn, a
tool-denied turn, a retried-error turn, a terminal-error turn, a cancelled turn and two resumed
turns. That is the whole observed surface; there is no `type` enumeration published anywhere.

| # | Frame | Keys |
|---|---|---|
| 1 | `{"role":"meta","type":"system.version",…}` | `role`, `type`, `version` |
| 2 | `{"role":"assistant","content":…}` | `role`, `content` |
| 3 | `{"role":"assistant","tool_calls":[…]}` | `role`, `tool_calls` |
| 4 | `{"role":"tool","tool_call_id":…,"content":…}` | `role`, `tool_call_id`, `content` |
| 5 | `{"role":"meta","type":"turn.step.retrying",…}` | `role`, `type`, `failed_attempt`, `next_attempt`, `max_attempts`, `delay_ms`, `error_name`, `error_message`, `status_code` |
| 6 | `{"role":"meta","type":"session.resume_hint",…}` | `role`, `type`, `session_id`, `command`, `content` |

Verbatim samples:

```
{"role":"meta","type":"system.version","version":"0.38.0"}
{"role":"assistant","tool_calls":[{"type":"function","id":"call_spike1","function":{"name":"Bash","arguments":"{\"command\":\"echo SPIKE_TOOL_RAN\"}"}}]}
{"role":"tool","tool_call_id":"call_spike1","content":"SPIKE_TOOL_RAN\n"}
{"role":"assistant","content":"SPIKE_FINAL_ANSWER_TOOL"}
{"role":"meta","type":"session.resume_hint","session_id":"session_65cc4dae-3178-41d0-b2e1-e0bc489ba166","command":"kimi -r session_65cc4dae-3178-41d0-b2e1-e0bc489ba166","content":"To resume this session: kimi -r session_65cc4dae-3178-41d0-b2e1-e0bc489ba166"}
```

Established, each by observation:

- **`system.version` is always the FIRST frame**, on success and on failure alike. It carries the
  running engine version in-band — a $0 pin assertion the adapter gets for free (R13).
- **`session.resume_hint` is the LAST frame on a run that reached the model**, and it carries the
  **engine-reported session id**. It is **ABSENT on a run that failed before any model reply**
  (observed on the 403 probe) — so the stream is not a guaranteed source of the session id, and the
  adapter must fall back to `session_index.jsonl` rather than fabricate one.
- **There is NO terminal result envelope.** Nothing corresponds to the Claude lane's `result`
  object. `Outcome.ResultText` therefore has no source (1) on this substrate: the §60 order collapses
  to limb (2), the stream's final assistant message VERBATIM, and the code says so.
- **Session ids are `session_<uuid-v4>`**, not the ULID-style `01HZ…` the vendor docs show.
- **There is no error frame.** A terminal error is not in the JSONL at all — see Q3.

### Q2 — ★ does any frame carry token counts? **NO in the stream; YES in the run's own transcript**

**This is the packet's most consequential finding and it changes a stated requirement.**

**No frame on stdout carries usage, tokens or cost.** The six shapes above are the complete observed
set and none has a usage member. F2's suspicion is confirmed for the stream.

**But the engine records usage per model call in the run's own session transcript**,
`$KIMI_CODE_HOME/sessions/<workDirKey>/<sessionId>/agents/main/wire.jsonl`, as a `usage.record`
frame, verbatim:

```json
{"type":"usage.record","agentId":"main","model":"__kimi_env_model__",
 "usage":{"inputOther":73,"output":29,"inputCacheRead":64,"inputCacheCreation":0},
 "usageScope":"turn","time":1787763099487}
```

The loopback provider had returned `{"prompt_tokens":137,"completion_tokens":29,
"total_tokens":166,"prompt_tokens_details":{"cached_tokens":64}}`. So the engine's decomposition is
**already normalized the way S02.4(a) wants it**, and the arithmetic is checkable:

| `adapters.Usage` field | `usage.record` source | value | check |
|---|---|---|---|
| `InputTokens` | `usage.inputOther` | 73 | = 137 prompt − 64 cached |
| `CacheReadTokens` | `usage.inputCacheRead` | 64 | = `cached_tokens` |
| `CacheCreationTokens` | `usage.inputCacheCreation` | 0 | — |
| `OutputTokens` | `usage.output` | 29 | = `completion_tokens` |

**`inputOther` EXCLUDES cache reads.** This settles R6's open question by measurement rather than
assumption: the Anthropic accounting normalization (`total prompt = cache_read + cache_creation +
input_tokens`) has the **same field semantics here**, so 73+64+0 = 137 = the provider's own
`prompt_tokens`, and 137+29 = 166 = `total_tokens`. No re-normalization is owed and none is applied.

**One paid call is exactly one `usage.record`**, verified on a two-call turn (the tool probe made two
model calls and produced exactly two records). R6's grouping requirement is satisfied structurally by
the source rather than by de-duplication.

**The model id must NOT be taken from `usage.record`.** Its `model` member is the synthesized alias
`__kimi_env_model__`, not the model actually called. The real id is on the sibling `llm.request`
frame (`{"type":"llm.request","model":"k3","modelAlias":"__kimi_env_model__",…}`). An adapter reading
`usage.record.model` would meter every Kimi run under a fictional model name.

**The transcript is appended LIVE, per call, and the store exists early.** Measured on a run whose
first tool call slept 12 s: `session_index.jsonl` and `agents/main/wire.jsonl` both existed within
~2 s, and the FIRST `usage.record` was already flushed to disk at that point, while the run was still
in flight. The second appeared after the second model call. So an adapter tailing the file emits one
`Usage` per paid call **as it happens** — D7 checkpoints fire per call during the run, not in a burst
at exit, and a crash mid-run does not lose the calls already made. (The per-RUN home also makes the
path unambiguous: a fresh home holds exactly one session directory, so `sessions/*/*/` resolves it
without needing the session id the stream only reports at the end.)

`usageScope` was `"turn"` on every observed record. Other scopes (subagent, compaction) were not
reached — native subagents are disabled by lowering (Q8) and no compaction occurred.

**Consequence for OQ-A, stated plainly.** The coordinator's disposition made transport (1)
conditional on finding usage **in the stream**, with `kimi web` as the fallback *because* an adapter
that cannot emit `adapters.Usage` cannot satisfy D7. The stream has none — but D7 is satisfiable on
transport (1) anyway, from a source the brief itself already names as reachable (§4 Driver row: *"This
engine DOES keep a per-session JSONL (`agents/main/wire.jsonl`)"*). The transcript sits **inside the
per-run `KIMI_CODE_HOME` the platform creates, owns and destroys**, is appended live during the run,
and carries strictly more than the stream would have. **No transport flip is taken and none is
needed**; what changes is the usage SOURCE within transport (1), and it is recorded here loudly
rather than silently.

### Q3 — exit codes

| Situation | Exit | Where the reason appears |
|---|---|---|
| Clean completion | **0** | — |
| Provider terminal error (403) | **1** | **stderr only**: `error: failed to run prompt: provider.auth_error: 403 You've reached your weekly (7-day) usage limit` |
| Provider retryable error, attempts exhausted (500) | **1** | **stderr only**: `error: failed to run prompt: provider.overloaded: 500 The engine is currently overloaded` |
| Provider retryable error, retries in flight | (run continues) | **stdout** `turn.step.retrying` frames carrying `status_code` + `error_message` |
| Tool call fails / is denied | **0** | a `role:"tool"` frame carrying the failure text; the turn continues |
| Process-group SIGTERM | **143** | — (no survivors in the group) |

Three things follow and all three are load-bearing:

1. **The terminal error text reaches the platform ONLY on stderr.** The brief's §2(8) characterizes
   stderr as thinking, tool progress and "resuming session" notices; it is also the **sole carrier of
   the terminal failure message**. The adapter must capture stderr separately and bounded — not
   merely "for ops" but as the input its wire-signal seam classifies.
2. **The engine prefixes its own error taxonomy** (`provider.auth_error`, `provider.overloaded`,
   `loop.max_steps_exceeded`, `context.overflow`, `provider.api_error` — the vocabulary is embedded in
   the shipped bundle). A 403 depletion arrives labelled `provider.auth_error`, which is §64's whole
   lesson arriving on a new wire: the status and the engine's own label both say "auth" for what is
   an ordinary weekly-quota exhaustion.
3. **Exit code 1 is not diagnostic.** Auth failure, quota exhaustion and overload all exit 1. Nothing
   branches on exit codes beyond `0` / non-zero / `143`, exactly as the brief instructed.

### Q4 — does `-p` run on `KIMI_MODEL_*` alone, with no `config.toml`? **YES**

First probe, run with **no `config.toml` at all** in a fresh `KIMI_CODE_HOME`, exited 0 and produced
a complete turn. The inference in §2(6) is confirmed by execution. Credential design stands.

### Q5 — does the process actually exit, and does the lowered ceiling bind? **Measured, both**

**The runaway is real and reproducible.** With the fake provider driving a `Bash` call carrying
`{"run_in_background":true,"disable_timeout":true,"command":"sleep 45"}` — a shape the model can
reach on its own, since both members are in the tool's published schema:

| Config | Result |
|---|---|
| defaults (no `[background]` table) | **did NOT exit** — killed by the harness at **25.2 s**, exit 124 |
| `[background] print_background_mode = "exit"` | **exited cleanly in 1.11 s**, exit 0 |

That is F4's concern reproduced at $0 and closed by one config key. R12 is a measured requirement,
not an argued one.

**The retry ceiling binds and is observable in the stream.** With
`[loop_control] max_attempts_per_step = 2` against a 500-returning provider, the run made **exactly 2
provider calls** and exited 1 after **1.5 s**, and the `turn.step.retrying` frame echoed
`"max_attempts":2`. At defaults the same probe made 7 calls in 43 s and was still retrying when the
harness stopped it (`"max_attempts":10`).

**★ The tables are NOT where the brief's requirement list implies, and unknown keys are SILENTLY
IGNORED.** The real TOML locations, established by effect:

- `[background]` → `print_background_mode`, `print_max_turns`, `print_wait_ceiling_s`,
  `bash_task_timeout_s`, `kill_grace_period_ms`
- `[loop_control]` → `max_steps_per_turn`, `max_attempts_per_step`
- `[tools]` → `enabled`, `disabled`
- `[permission]` → `rules` (array of `{decision, scope, pattern, reason}`)
- `[[hooks]]` → `{event, matcher, command, timeout}`, `.strict()` — extra fields DO fail this section

A probe planting `zzz_bogus_top = 1` at top level and `zzz_bogus_bg = 2` inside `[background]` ran
**exit 0 with no warning of any kind**. A key written at the wrong level is therefore
indistinguishable from a key that took effect, **by inspection**. This is why the boundedness
assertion must be behavioral: reading the lowered `config.toml` back proves nothing. An **invalid**
section (wrong type) is reported as `Warning: Ignored invalid config section '<name>'` on stderr and
then ignored — a warning, never a refusal.

### Q6 — is `AGENTS.md` ingestion suppressible? **YES, completely**

Three sentinels were planted: `$KIMI_CODE_HOME/AGENTS.md`, `$HOME/.agents/AGENTS.md` (the leg that
does **not** move with `KIMI_CODE_HOME`) and `<cwd>/AGENTS.md`.

| Run | HOME leg | `~/.agents/` leg | cwd leg |
|---|---|---|---|
| no `SYSTEM.md` | **present** in the request | **present** | **present** |
| `$KIMI_CODE_HOME/SYSTEM.md` omitting `${agents_md}` | **absent** | **absent** | **absent** |

A `SYSTEM.md` that omits the `${agents_md}` placeholder **closes all three legs at once**, including
the `~/.agents/` leg that `KIMI_CODE_HOME` cannot move. G6 is closed and the §2(4) inference is
confirmed by execution. R8's instruction-file row therefore has a real knob and **no channel needs
to be reported as knob-less**. (`HOME` is still bounded, on the independent ground that a
knob-in-the-config is a weaker guarantee than a path the process cannot reach.)

### Q7 — is `--yolo -p` accepted? **REJECTED** — the reference wins

```
$ kimi --yolo -p "go" --output-format stream-json
error: Cannot combine --prompt with --yolo.        (exit 1)
```

The doc self-contradiction recorded in §2(2) is settled by execution: `docs/en/reference/kimi-command.md`
is right and `docs/en/configuration/overrides.md`'s `kimi --yolo -p "Batch rename…"` example is
wrong at this pin. This is a **doc falsification**, recorded rather than resolved silently (§64).

### Q8 — is native-subagent disable STRUCTURAL? **YES — stripped pre-inference**

The fake provider records the `tools` array of every request, which is the toolset **as the model
saw it**, before any inference.

| Config | Tools offered | `Agent` | `AgentSwarm` |
|---|---|---|---|
| none | **26** | present | present |
| `[tools] disabled = ["Agent","AgentSwarm"]` | **24** | **absent** | **absent** |

The sole-controller rider's guarantee is **structural on this engine**, meeting the ratified bar
(§61's opencode probe-5 precedent, where blanket deny strips a tool pre-inference). **No gate finding
is owed on R9/A10's structural-vs-behavioral question**; the answer is the good one.

The default 26-tool set, for the record: `Agent, AgentSwarm, AskUserQuestion, Bash, CreateGoal,
CronCreate, CronDelete, CronList, Edit, EnterPlanMode, ExitPlanMode, FetchURL, GetGoal, Glob, Grep,
Read, ReadMediaFile, SetGoalBudget, Skill, TaskList, TaskOutput, TaskStop, TodoList, UpdateGoal,
WaitFor, Write`.

### Q9 — ★ do `[permission] deny` rules hold in `-p`, and do `PreToolUse` hooks deny? **NO and YES — the inverse of what the docs say**

**`[permission]` deny rules are INERT in print mode.** Three probes, each with a cleanly-loading
config (no warning on stderr, no complaint in the log), each driving the model to call `Bash`:

| Rule | Tool ran? |
|---|---|
| `[[permission.rules]] decision="deny" pattern="Bash" scope="user"` | **YES — it ran** |
| `[[permission.rules]] decision="deny" pattern="Bash(*)" scope="user"` | **YES — it ran** |
| `[[permission.rules]] decision="deny" pattern="Bash" scope="project"` | **YES — it ran** |

This **falsifies** the vendor line quoted in the brief's §2(2): *"In `-p` mode, no human approval is
requested — regular tool calls are handled under the `auto` permission policy, while **static deny
rules remain in effect**."* At pin 0.38.0 they do not.

**A `PreToolUse` hook returning `permissionDecision: "deny"` DOES stop the call.** The tool did not
execute, and the reason was handed back to the model as the Tool message body:

```
{"role":"tool","tool_call_id":"call_spike1","content":"SPIKE_HOOK_DENIED"}
```

The hook's stdin contract was confirmed verbatim, and it carries one member the docs do not list —
`tool_call_id`, a real per-call identity:

```json
{"hook_event_name":"PreToolUse","session_id":"session_57fcff28-…","cwd":"…",
 "client_type":"kimi_code_cli","tool_name":"Bash","tool_input":{"command":"echo SPIKE_TOOL_RAN"},
 "tool_call_id":"call_spike1"}
```

**Consequence for R10, and it INVERTS the brief's assignment while confirming its conclusion.** The
brief assigned enforcement to `[permission] deny` + the `[tools]` allowlist and called hooks
"audit and denial only". Measured, it is the other way round: deny rules enforce **nothing** in print
mode, and hooks enforce **effectively but fail-open** (the vendor documents fail-open and says hooks
"should not be used as the sole security barrier" — and this spike gives that warning teeth by
removing the layer it was supposed to sit behind).

So on this substrate there is exactly **ONE structural brake: the `[tools]` allowlist**, proven
pre-inference at Q8. R10's *conclusion* is unchanged and in fact strengthened: a `CompiledWorker`
carrying `GatedTools` must be **refused by name**, because the substrate has no ask, no defer, and —
now measured — no static deny either. Auto-approving a gated call here would not merely skip a
review, it would run the call with nothing whatsoever standing in front of it.

### Q10 — is the session store populated, and does `--session <id>` resume? **YES to both**

A single `-p` run created `session_index.jsonl` (one record: `sessionId`, `sessionDir`, `workDir`)
and `sessions/<workDirKey>/<sessionId>/` containing `state.json`, `logs/kimi-code.log` and
`agents/main/wire.jsonl`. `workDirKey` had the documented `wd_<slug>_<12-hex>` shape.

Three consecutive turns in one home — `-p`, then `--session <id> -p`, then `-c -p` — all exited 0,
all reported the **same** session id, and the conversation grew as it should:

| turn | messages sent to the provider |
|---|---|
| 1 (`-p`) | 3 — `system, user, user` |
| 2 (`--session <id>`) | 6 — `…, assistant, user, user` |
| 3 (`-c`) | 9 — `…, assistant, user, user` |

The index stayed at **one** record and one session directory: resuming does not fork a session.

---

## Findings beyond the ten questions

**F-S1 · The credential channel is fail-closed, measured both ways.** With `KIMI_MODEL_NAME` set and
`KIMI_MODEL_API_KEY` **unset**, and again with it set to the **empty string**, the run exited **1**
with `error: failed to run prompt: provider __kimi_env__ has no credential configured` and made
**zero provider calls**. There is no silent unauthenticated call in either direction. (The failure is
at prompt-run rather than literally at process start; immaterial, since no request is made.)

**F-S2 · Nothing is written back to the config file.** After a run driven entirely by `KIMI_MODEL_*`,
the lowered `config.toml` was byte-identical to what the lowering wrote. The secret never lands on
disk. R15's stated property holds.

**F-S3 · The bounded `HOME` stayed empty.** After a full run, `find $HOME -type f` returned **zero
files**. Nothing outside `KIMI_CODE_HOME` and the run cwd was written.

**F-S4 · The engine spawns cleanly as a process-group leader and leaves no survivors.** `setsid` +
group TERM produced exit 143 with zero remaining processes in the group. The cancel ladder's group
rung is sound here, unlike the opencode substrate where it ends a per-user server.

**F-S5 · `max_attempts_per_step` must be lowered explicitly.** At its default of 10 with a 32 s
backoff cap, a single failing step can burn minutes before the platform's own classifier ever sees
the failure — engine-native retry standing in for the policy layer, which S10.5 forbids by name.

**F-S6 · The request carries `stream_options: {include_usage: true}`.** The engine asks its provider
for usage and consumes it; the omission is purely in what `--output-format stream-json` chooses to
print.

---

## Corrections this spike makes to the brief

| # | Brief said | Spike measured | Effect |
|---|---|---|---|
| C1 | R0-Q2 decides OQ-A; no usage in the stream ⇒ fall back to `kimi web` | No usage in the stream; **full per-call usage in the run's own `wire.jsonl`** | Transport (1) **kept**; usage source is the per-run transcript. No flip. |
| C2 | R10: enforcement = `[permission] deny` + `[tools]`; hooks are audit-only | Deny rules **inert** in `-p`; hooks **do** deny (fail-open) | Only structural brake is `[tools]`. R10's refusal conclusion strengthened. |
| C3 | R12 names `print_max_turns` etc. as if top-level | They live under **`[background]`** and **`[loop_control]`**; unknown keys are silently ignored | Boundedness must be asserted **behaviorally**, never by reading the file back. |
| C4 | §2(2): "static deny rules remain in effect" in `-p` | Falsified at 0.38.0 | Recorded as a vendor-doc falsification. |
| C5 | §2(2): `--yolo -p` contradiction unresolved | **Rejected**, exit 1 | Reference wins; the guide's example is wrong. |
| C6 | §2(4): `SYSTEM.md` omitting `${agents_md}` is an inference | Confirmed; closes **all three** AGENTS.md legs | No knob-less channel to report. |
| C7 | R9/A10: structural-vs-behavioral is an open gate question | **Structural** — stripped pre-inference | The good answer; no gate finding owed. |
| C8 | R6: Anthropic normalization owed "only if" semantics match | They **match** (`inputOther` excludes cache reads) | Normalization confirmed by arithmetic, not assumed. |
| C9 | R4/§60: `ResultText` limb (1) is the terminal envelope | **No terminal envelope exists** | The §60 order collapses to limb (2), with the reason in code. |
| C10 | §2(8): stderr carries thinking/progress/notices | It also carries the **only** copy of a terminal error | stderr is a classification input, not just ops capture. |

---

## Reproduction

Harness (scratch, not committed): a ~150-line Go loopback provider that logs every request and
scripts the reply, plus a bash probe runner that builds a fresh `KIMI_CODE_HOME`, a bounded `HOME`
and an isolated cwd per probe and invokes the real `kimi` under `env -i`. Every probe in this
document is one invocation of that runner. The tier-F fixtures of `internal/adapters/kimicli` are
this capture; the tier-R suite re-runs the same shape against the installed binary.
