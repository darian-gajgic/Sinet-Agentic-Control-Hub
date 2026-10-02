# H-3 execute — harness hardening (deterministic limits, update guard, measured cap)

Light-path process packet, no spec sections. Branch: the H-3 worktree branch; base `1267379`. Scope (a)–(c) only.

## (a) Deterministic limits

- `lib.sh`: `limit_retry_json <log> [live]` scans the stream-json transcript for `{"type":"system","subtype":"api_retry"}` events whose `error` kind contains `limit` (`rate_limit`, `usage_limit`, any future `*limit*`; research §4.3 fields `error`, `retry_delay_ms`, `error_status`). It uses `jq -R 'fromjson?'` so a torn last line cannot break the scan. `retry_reset_epoch` = event time + ceil(`retry_delay_ms`/1000), where event time is the event's `timestamp` if present, else the transcript mtime. `limit_epoch` = the prose reset (`parse_reset_epoch`), else the retry-derived epoch, else 0.
- `classify`: the result-line LIMIT (step 1) and the status.json/StopFailure LIMIT (step 2) now fall back to the retry-derived epoch when no prose time parses. New step 2b: there is no signature AND the last assistant/api_retry event is a limit-kind retry → `LIMIT:<family>:<epoch>`. This covers a wall-clock kill mid-retry with no result line, which was classified CRASH before. A retry the CLI recovered from (an assistant message follows it) is not a limit: that sitting stays CONTINUE or CRASH as before.
- `hooks.proposed.json`: the StopFailure matcher is now `rate_limit|usage_limit|.*limit.*`. The regex characters make Claude Code treat it as a regex, not an exact list. `install-hooks.sh` comment updated. `.claude/settings.json` is not touched, so the **operator must re-run `P3/run/install-hooks.sh`** for the wider matcher to take effect.
- `loop.sh`: every `LIMIT:*` classification calls `archive_limit` → `P3/run/fixtures/observed/<sitting ts>/{result.json,status.json,api_retry.jsonl,class.txt}`. The dir is committed with a `README.md` saying these are real observations to promote into tests.
- Tests (test-classify.sh, +23): killed mid-retry → `LIMIT:fable:mtime+5400` (`opus` when the sitting model is opus); a 429 result line with no prose time → epoch from the event timestamp + 7200 s; a hook status with no time → retry epoch; prose beats the retry epoch; a recovered retry → CONTINUE / `CRASH:no status.json`; `--dry-run` SWITCH. **Hook unit test:** the StopFailure `command` is extracted from hooks.proposed.json with jq and run with `sh -c`, with a sample hook JSON on stdin, `CLAUDE_PROJECT_DIR`=temp dir and `P3_SITTING=1`. Asserts: valid JSON, `outcome` LIMIT, `hook` StopFailure, `note` = error + details, `ended` ISO, classify of it → `LIMIT:fable:<epoch>`; without `P3_SITTING` nothing is written. Matcher (anchored ERE) matches rate_limit/usage_limit/weekly_limit/session_limit_reached and does not match overloaded/server_error/authentication_failed.
- New fixtures: `limit-retry.jsonl`, `limit-retry-recovered.jsonl`, `limit-429-retry.jsonl`, `limit-hook-notime.status.json`.

## (b) Update guard

- `sitting.sh` exports `DISABLE_AUTOUPDATER=1` for the whole sitting, preflight included.
- Preflight (after the integrity checks and the `status.json` removal): first field of `claude --version` vs `P3/run/cli-version.pinned` (created: `2.1.287`, the version on this host today). On drift, `probe_model "$MODEL"`:
  - probe fails → `status.json` = `{"outcome":"BLOCKED","note":"CLI drift <old>→<new>: smoke failed"}`, exit 3. The loop classifies `BLOCKED:CLI drift …`, notifies and waits for RESUME (`--once` exits 0).
  - probe passes → log line, one `notify` ("P3 CLI updated"), pin rewritten, sitting continues. Because the pin is updated, the next sitting is not drift, so the notification fires once.
  - An empty `--version` output counts as `unknown` and is BLOCKED without a probe (the pin is never set to empty).
- `.meta` and the start log line now carry `cap=` and `cli=`.
- Tests (test-loop.sh): a shim `claude` on PATH answers `--version` with `$STUB_VERSION` and the probe prompt with completed/api_error per `$STUB_PROBE`. Drift + probe fail → exit 3, BLOCKED status with the exact note, pin unchanged, no sitting launched; same through `loop.sh --once` → exit 0, `CLASS BLOCKED:CLI drift …`, no ledger row. Drift + probe pass → exit 0, pin = 9.9.9, sitting ran; a second sitting adds no second notification. `DISABLE_AUTOUPDATER=1` is seen by both the `--version` call and the `-p continue implementation` call.

## (c) Measured cap

- `P3/run/cap` (committed, `3`). `read_cap` → 1..5, default 3 when the file is missing or malformed.
- `record_sitting` (loop, after every classified sitting that has a transcript) appends to `log/sittings.tsv` (header on creation) the columns `ts model duration_s turns landed compactions transcript_bytes outcome` **+ `cap`**. `duration_s` comes from meta start/end, `turns` from the result line, `landed` from `.landed|length` in status.json. `compactions` counts `compactions.log` entries with `at` in [start, end] and `sitting != "interactive"`. `outcome` = the classification head.
- **Deviation, flagged:** the ninth column `cap` (the cap the sitting ran under, read from `.meta`) is beyond the eight listed columns. Without it the "five consecutive sittings that landed the cap" rule cannot survive loop restarts or `--once` runs. With it the ledger is the only memory, and the streak resets automatically after any cap change.
- `update_cap`: last row compactions > 0 → cap−1 (min 1); else the last five rows all have cap = current, landed ≥ cap and compactions = 0 → cap+1 (max 5). Every change is logged (`CAP a → b (reason)`).
- `sitting.sh` injects into the launch prompt: "Packet cap (measured, P3/run/cap; overrides the contract's default): land at most <cap> packets this sitting." `sitting-prompt.md` R1: default 3, and the launch prompt's cap overrides it.
- Tests (test-loop.sh): compaction sitting → cap 3→2, ledger header + exact row shape, row 2 runs at cap 2, prompts carry "land at most 3" then "land at most 2"; 4 seeded qualifying rows + a land-3 sitting → 4; a compaction in the streak → stays 3; only 4 rows → stays 3; min clamp 1, max clamp 5, malformed file → 3.

## Test isolation

All new state paths are env-overridable (`P3_CAP_FILE`, `P3_SITTINGS_TSV`, `P3_COMPACTIONS_LOG`, `P3_CLI_PIN`, `P3_OBSERVED_DIR`). test-loop.sh points them at a temp dir and suppresses desktop notifications (bogus DBUS address). Verified after the runs: the real `cap`/`cli-version.pinned`/`fixtures/observed`/`log/sittings.tsv` were untouched.

## Acceptance

- `bash -n` on lib.sh, sitting.sh, loop.sh, test-classify.sh, test-loop.sh, install-hooks.sh: OK; hooks.proposed.json valid JSON.
- `P3/run/test-classify.sh`: **53 checks, ALL PASS** (30 existing + 23 new).
- `P3/run/test-loop.sh`: **43 checks, ALL PASS** (4 existing + 39 new).
- README updated (file table, knobs, update guard, measured cap, classification order).

## Operator notes / residuals

1. Re-run `P3/run/install-hooks.sh` to install the wider StopFailure matcher (settings.json not edited by this packet).
2. The loop rewrites `P3/run/cap` (committed) and the update guard rewrites `cli-version.pinned`. Both then show as working-tree modifications on `main`; the coordinator should commit them with bookkeeping, or leave them.
3. Whether real `api_retry` events carry a `timestamp` field is unverified; the mtime fallback covers its absence. The first archived observation will settle it.
4. Existing behaviour, unchanged: test runs leave stub transcripts in the gitignored `P3/run/log/`.

## Draft STATE landing line (≤600 chars)

H-3 LANDED (<sha>): harness hardening. (a) classify reads stream-json system/api_retry limit events (killed mid-retry → LIMIT; reset from retry_delay_ms when no prose time), StopFailure matcher +`.*limit.*`, LIMITs archived to P3/run/fixtures/observed/; (b) DISABLE_AUTOUPDATER=1 + CLI pin 2.1.287, drift → probe, fail → BLOCKED exit 3, pass → notify once + repin; (c) log/sittings.tsv ledger (+cap col) and P3/run/cap 1..5 rule, cap injected into launch prompt. Tests 53/0 + 43/0. Operator: re-run install-hooks.sh.
