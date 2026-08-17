# PH-1 — round-1 interview phrasing silently fell back to canonical wording

Diagnosis note, 2026-08-17. Read-only investigation against code plus the stopped
cold-walk-1 world (`~/.sinet-coldwalk-w1`). No production code changed, no world
started, no live/paid/GPU calls.

Refs: Spec S06.5 (phrase-and-summarize seat), S06.10 (utility duty row), S12.4
(duty shaping), P3-RW-12 R6/R7/F1/OQ2, CONVENTIONS §14/§26 (degradation rule).

---

## 0. Two corrections to the brief, both load-bearing

**(a) The token count was misread.** The brief records the second local-duty
checkpoint row as `598 in / 400 out`. It is **`598 in / 4000 out`**. Verified
first-hand:

```
$ go run ./tools/dbq ~/.sinet-coldwalk-w1/platform.db \
    "select checkpoint_id, created_ts, model_id, usage_json
       from checkpoints where usage_json like '%utility%' order by created_ts"

[4  2026-08-17T05:18:49.008180293Z  Qwen3.5-9B
 {"input_tokens":598,"output_tokens":4000,"local":{"lane":"local","duty":"utility",
  "model":"Qwen3.5-9B","model_sha256":"dc2a39ae…","engine_build":"b10085"}}]
[13 2026-08-17T05:26:47.279587996Z  Qwen3.5-9B
 {"input_tokens":427,"output_tokens":700,"local":{"lane":"local","duty":"utility",…}}]
```

`4000` is *exactly* `phraseMaxTokens` (`internal/stage/local.go:57`). `700` is
*exactly* `helpMaxTokens` (`internal/stage/local.go:33`). Both drafting calls ran
to their cap and stopped. That single digit is the whole diagnosis.

**(b) Round 2 was never phrased either — it is unphrased BY DESIGN.** The
round-2 card is a *clarification* card carrying the planner's own
`NEEDS-CLARIFICATION:` markers, and `internal/intake/pipeline.go:1039-1041` says
so explicitly:

> It is NOT phrased — these questions are the planner's own marker text, not
> taxonomy wording, so there is nothing here for the phrasing seat to reword.

The fluent round-2 prose the operator saw was written by `claude-opus-4-8` in the
plan stage, not by the phrase seat. So the symptom is **worse than reported**:
this is not "round 1 fell back, round 2 worked." Across the entire world,

```
$ go run ./tools/dbq …/platform.db "select count(*) from asks where snapshot like '%phrased%'"
[any_phrased]
[0]
```

**The phrase seat has a 0% live success rate. Not one wording ever landed.** The
same defect silently took the S06.9 approval-card Help block (cp 13), which fell
back byte-identically to the deterministic `defaultHelp()`.

---

## 1. The mechanism

**A drafting duty keeps the reasoning model's think phase on, the think phase is
emitted before the schema-constrained region, it consumes the entire token
budget, the JSON region never opens, the decode fails, and the error is
discarded without a single log line.**

The chain, in order:

**1. The think phase is gated on the wrong flag.** `internal/local/duty.go:219`:

```go
NoThink: in.Classification,
```

`NoThink` is derived from `Classification`. The phrase duty sets
`Classification: false` (`internal/stage/local.go:285`) because it *is* drafting,
not classification — which is correct on its own terms, but it also silently
leaves `enable_thinking` ON for the reasoning workhorse
(`internal/local/client.go:173-177`). `NoThink` and `Classification` are two
different questions welded to one field.

**2. The think phase precedes the constrained region, so the cap budgets the
wrong thing.** This is already documented at `internal/stage/local.go:41-47`:

> It budgets the THINK PHASE, not just the JSON […] the think phase is emitted
> BEFORE the constrained region, so a cap sized for the JSON alone is spent
> entirely on reasoning and the schema region never starts.

**3. The engine stops at the cap and the caller cannot tell.**
`internal/local/client.go:214` parses `FinishReason` — which would have read
`"length"` — but `DutyResult` (`internal/local/duty.go:56-65`) drops the field and
`callOnSeat` never inspects it. The call is treated as a success, and the
mandatory D7 row is written *after* it (`duty.go:239-249`) — which is why the
evidence exists at all.

**4. The truncated content fails to decode.** `internal/stage/local.go:294-296`:

```go
var out map[string]string
if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
    return intake.PhraseResult{}, fmt.Errorf("stage: decode utility phrase output: %w", err)
}
```

**5. The error is swallowed in silence.** `internal/intake/pipeline.go:873-876`:

```go
res, err := p.Phraser.PhraseAndSummarize(ctx, in)
if err != nil {
    return card
}
```

No log, no event, no metric. The identical swallow takes Help at
`internal/intake/pipeline.go:1387-1389` (`if h, err := …; err == nil`). The card
ships with taxonomy wording, the frontend honestly admits it
(`web/src/Intake.tsx:1105`), and nothing anywhere records *why*.

### Decisive evidence: the discriminator is exactly `Classification`

Every local duty in the world, by output tokens:

```
$ go run ./tools/dbq …/platform.db "select json_extract(usage_json,'$.local.duty') as duty,
    json_extract(usage_json,'$.output_tokens') as out from checkpoints
    where usage_json like '%\"lane\":\"local\"%' order by created_ts"

duty                      out
watchdog-disambiguator     62      Classification:true   → NoThink ON  → OK
watchdog-disambiguator     70      Classification:true   → NoThink ON  → OK
intake-triage              69      Classification:true   → NoThink ON  → OK
utility                  4000      Classification:false  → NoThink OFF → CAP, FAILED
intake-triage             121      Classification:true   → NoThink ON  → OK
intake-triage              88      Classification:true   → NoThink ON  → OK
intake-triage              99      Classification:true   → NoThink ON  → OK
utility                   700      Classification:false  → NoThink OFF → CAP, FAILED
watchlist-triage           98/85/82/78                   → NoThink ON  → OK
```

Ten classification calls, all 62–121 tokens, all far under cap, all succeeded.
Two drafting calls, both pinned to their cap to the token, both produced nothing.
There is no overlap and no ambiguity.

### Corollary: the "~50s/card phrase latency" figure is an artifact, not a cost

The phrase call ran 69.19 s (event 140 `05:17:39.817Z` → cp write
`05:18:49.008Z`) because it generated 4000 junk tokens at ≈58 tok/s. A working
call emitting ~350 tokens of JSON is a ~6 s call. Any cost or latency model built
on the observed 50–70 s per card is built on the bug and should be rebuilt after
the fix.

---

## 2. Determinism

**Deterministic on fresh worlds — not probabilistic, not a race.**

- Both drafting calls failed; all ten classification calls succeeded. A 2/2 and
  0/10 split is a structural property, not a coin flip.
- The client sends `Temperature: 0` (`internal/local/client.go:148`), so the
  think phase is greedy and reproducible for a given prompt.
- The failure is a function of `(prompt, cap)` only. Nothing about the walk's
  timing, model warmth, contention, or VRAM enters it.

The one input that varies is *how much the model thinks*, which scales with
request complexity. That is why the standing tripwire passes and the real walk
fails — see §4. So the honest statement is: **deterministic for any request whose
think phase exceeds the cap, which on this evidence includes ordinary real
requests, and the failure is total (zero phrasings) rather than partial.**

### What was ruled out, and why

- **A per-call deadline / timeout.** No `context.WithTimeout` exists anywhere on
  the phrase path; the local client's only timeout is 5 minutes
  (`internal/local/client.go:89`). Structurally decisive:
  `internal/intake/pipeline.go:681` and `:701` call `p.issueCard(ctx, …)` with the
  *same* ctx immediately after `buildInterviewCard(ctx, …)` — an expired or
  cancelled ctx would have failed the ask write too, and the round-1 ask was
  written. This was the prior launch's leading candidate; it is wrong.
- **GPU admission refusal / model contention.** `internal/shell/local_seams.go:109`
  builds the admitter with no Ledger, so footprints are uncalibrated and
  `need = 0 + guard = 512 MiB` (`internal/local/vram.go:483-486`) — it admits.
  Battery mode is out too: `aliasUrgent` is true only for the watchdog
  (`vram.go:352`), so on battery the classify call would have been refused as
  well, and it succeeded. The cold `fast`→`workhorse` pool12 swap did happen and
  was harmless: the call completed at a steady ≈58 tok/s.
- **Usage-row write ordering.** Correct as built: the D7 row is written only
  after a successful `client.Chat` (`duty.go:229-249`). The row's existence is
  what proves the model answered.
- **A per-round or batch call violating RW-12 OQ2.** Not violated. Round 1 was
  **one card with four questions**, and `buildInterviewCard` phrases all
  questions of a card in exactly one call. `maxQuestionsPerCard = 4`
  (`internal/intake/cards.go:108`). The "4-card round 1" in the brief is four
  *questions* on one card. The frontend admission the operator saw is the
  round-level one, which only fires when the whole card is stock
  (`web/src/Intake.tsx:1078`, `allStock && questions.length > 1`) — itself
  evidence that the entire call, not some ids, contributed nothing. Six local
  rows for the walk reconcile exactly: 4 × intake-triage + 2 × utility.

---

## 3. Smallest honest fix

**Do not raise the cap again.** The cap has already been chased once
(1000 → 4000, P3-RW-12 drain r1 F1) and this walk proves 4000 is also
insufficient for an ordinary request. An unbounded think phase cannot be
out-budgeted; the next number would be another guess with another silent failure
behind it.

Three changes, smallest first:

**F1 — decouple `NoThink` from `Classification` and turn thinking off for the
drafting duties.** Add an explicit `NoThink bool` to `local.DutyRequest`, replace
`internal/local/duty.go:219` with `NoThink: in.NoThink || in.Classification`, and
set `NoThink: true` at the two drafting call sites
(`internal/stage/local.go:284` phrase, `:245` help). The RW-12 note at
`internal/stage/local.go:52-55` declined this because it "would trade away the
drafting quality the duty row asks for and was NOT measured." That reasoning is
now overtaken by evidence: the current setting delivers *zero* output, and no
quality argument survives a 0% success rate. It must still be **measured, not
reasoned** — the RW-12 lesson stands.

**F2 — never let a truncated reply masquerade as a success.** Carry
`FinishReason` through `DutyResult` and treat `finish_reason == "length"` (or
`OutputTokens >= MaxTokens`) as a typed error at the duty layer, so the caller
degrades on a named cause instead of on an opaque decode failure. This is the
belt that makes any future cap regression loud on the first occurrence.

**F3 — strengthen the standing tripwire.** `TestLivePhraseAndSummarize`
(`internal/stage/phrase_test.go:149`) is correctly un-gated and asserts non-empty
`Phrased` for every id, but it tests one short request and therefore tests a
point, not the margin. It must (a) use a request of realistic size — the walk's
was 598 input tokens against the test's much shorter one — and (b) assert
`OutputTokens < MaxTokens` with headroom, so "it fit, barely" fails before a
walk finds it. The same leg should cover the Help duty, which has no live leg at
all and failed identically.

### Sizing

**A light packet with a mandatory live measurement leg — not a drive-by, not a
full packet.** The code is small and local: one new seam field, one changed
expression, two call sites, one error type, two test legs (~40–60 lines across
`internal/local/duty.go`, `internal/local/client.go`, `internal/stage/local.go`,
`internal/stage/phrase_test.go`). What makes it a packet rather than a light-path
edit is the measurement: F1 changes model behaviour and must be proven live on
the ratified stack — phrasings land, quality is acceptable with thinking off, and
output sits comfortably under cap — and F3's margin assertion needs the same
stack. That needs the world + stack lock, currently held by the frontend builder,
and it needs the serial-test discipline (`-p 1`, one package, GPU-only, reap
orphans).

If F1's live measurement shows drafting quality genuinely degrades with thinking
off, the fallback is to keep thinking on but give the constrained region its own
guaranteed budget rather than sharing one cap — a larger change that should not
be attempted before the measurement says it is needed.

---

## 4. The logging the phrase path must gain

The control log holds **zero** phrase-related lines. Verified: between
`07:17:38.902+02:00` ("stage: request submitted") and `07:18:49.018+02:00`
("stage: intake gate open") — the entire 69-second window in which the call ran
and failed — the log contains only watchlist and retention background noise.
Greps for `phrase`, `degrade`, `timeout`, `deadline`, `decode`, `truncat`,
`llama`, `swap`, `vram`, `gpu` return nothing from the local path. The one `duty`
hit is the boot line. Meanwhile the *paid* stage logged its truncations properly
three times ("stage: session reply was truncated mid-delimiter…", P3-RW-16 R2) —
the local duty path simply has no equivalent.

That absence is itself a finding: a degradation the requester is shown on screen
left no trace an operator could search. Required, in priority order:

1. **WARN at the truncation boundary** (`internal/stage/local.go`, or better the
   duty layer so every duty inherits it) whenever `finish_reason == "length"` or
   `OutputTokens >= MaxTokens`: duty alias, model, cap, input/output tokens.
   Platform-authored fields only — nothing the model wrote may enter the log
   (S01.11), which the RW-16 line at `internal/stage/engines.go:350-353` already
   models.
2. **WARN at every degrade point that swallows a seam error** —
   `internal/intake/pipeline.go:874` (phrase) and `:1388` (help) — with run id,
   card kind, question count, and the error. A degradation the user can see must
   never be invisible to the operator. This is the single highest-value line in
   the list: it alone would have made PH-1 self-reporting.
3. **A run event for seat degradation**, so "the phrase seat fell back" is
   queryable in the run record rather than only in log scrollback — the card
   admits the fallback to the requester, so the record should carry it too.
4. **`finish_reason` in the D7 local marker.** The `checkpoints` table has no
   error or status column (schema confirmed), so `usage_json` is the only durable
   per-call surface. `output_tokens == MaxTokens` was the *sole* signal that
   solved this case, and it was implicit. Make it explicit.

A cheap standing check falls out of (4): **any `$0` local row whose
`output_tokens` equals its duty's cap is a defect until proven otherwise.** That
query would have caught PH-1 the moment the first cold walk finished.
