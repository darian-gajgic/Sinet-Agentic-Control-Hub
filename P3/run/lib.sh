#!/usr/bin/env bash
# P3 continuous-run harness — shared library (sourced by loop.sh, sitting.sh, test-classify.sh).
# Design record: P3/design/continuous-run-harness-proposal-2026-09-22.md (ratified 2026-09-22).
# Rule: a sitting is classified from its status file + the JSON result + git state — NEVER from the exit code.

P3_ROOT="${P3_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
RUN_DIR="$P3_ROOT/P3/run"
LOG_DIR="$RUN_DIR/log"
STATUS_FILE="$RUN_DIR/status.json"
STOP_FILE="$RUN_DIR/STOP"
RESUME_FILE="$RUN_DIR/RESUME"
LOOP_LOG="$LOG_DIR/loop.log"

: "${P3_MODEL_PRIMARY:=claude-fable-5-1[1m]}"   # matches ~/.claude/settings.json "model"
: "${P3_MODEL_FALLBACK:=claude-opus-5}"          # lossless fallback while the Fable limit is active
: "${P3_SITTING_WALL:=4h}"                        # SIGINT after this (rule R1); +15 min grace then SIGKILL
: "${P3_SITTING_MAX_TURNS:=600}"                  # secondary rail (runaway guard); the wall clock is the hard rail
: "${P3_NOTIFY_URL:=}"                            # optional ntfy.sh topic URL for phone push (curl -d)
: "${P3_SITTING_MAX_BUDGET_USD:=}"                # optional nominal-cost rail (--max-budget-usd); empty = none (subscription lane)
: "${P3_PROBE_INTERVAL:=900}"                     # seconds between canary probes while a limit is active
: "${P3_PAUSE_MIN:=120}"                          # pause after a productive sitting
: "${P3_PAUSE_MAX:=1800}"                         # idle backoff cap while sittings make no progress (120 → 600 → cap)
: "${P3_CRASH_PAUSE:=300}"                        # pause after a crash-class sitting
: "${P3_SWITCH_PAUSE:=60}"                        # pause after switching models on a Fable limit
: "${P3_CAP_FILE:=$RUN_DIR/cap}"                  # measured packet cap (H-3c), committed; default 3, range 1..5
: "${P3_SITTINGS_TSV:=$LOG_DIR/sittings.tsv}"     # one row per sitting: the measured-cap ledger
: "${P3_COMPACTIONS_LOG:=$LOG_DIR/compactions.log}" # written by the PreCompact hook
: "${P3_CLI_PIN:=$RUN_DIR/cli-version.pinned}"    # Claude Code version the harness last passed a probe on (H-3b)
: "${P3_OBSERVED_DIR:=$RUN_DIR/fixtures/observed}" # raw evidence of every LIMIT-classified sitting (H-3a)
: "${P3_CI_GUARD:=1}"                             # main guard (H-4a): 0 = start sittings without reading CI on main
: "${P3_CI_POLL:=120}"                            # re-check interval while the latest CI run on main is queued/in progress
: "${P3_WAIT_POLL:=600}"                          # re-check interval of every other wait: gate, blocked, CI on main not green
: "${P3_INT_GRACE:=90}"                           # Ctrl-C/SIGTERM on the loop (H-5): seconds the sitting gets to end its turn after SIGINT, then SIGTERM

LIMIT_RE='hit your (usage |session |weekly )?limit|reached your [a-z ]*limit|usage limit|rate[ _-]?limit|limit reached|limit will reset|out of (usage|credits)|quota (exceeded|reached)'

wall_seconds() { # wall_seconds <Nh|Nm|Ns|N>  — a timeout(1)-style duration in seconds
  case "$1" in *h) echo $(( ${1%h} * 3600 ));; *m) echo $(( ${1%m} * 60 ));; *s) echo "${1%s}";; *) echo "$1";; esac
}

log() { # log <msg>  — timestamped line to stdout and the loop log
  local line; line="$(date -u +%FT%TZ) $*"
  echo "$line"; mkdir -p "$LOG_DIR"; echo "$line" >> "$LOOP_LOG"
}

rotate_log() { # keep loop.log under ~5 MB
  [ -f "$LOOP_LOG" ] && [ "$(stat -c %s "$LOOP_LOG")" -gt 5000000 ] && mv -f "$LOOP_LOG" "$LOOP_LOG.1" || true
}

notify() { # notify <title> <body> — desktop notification (+ optional phone push); never fails the loop
  local title="$1" body="$2"
  export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=/run/user/$(id -u)/bus}"
  command -v notify-send >/dev/null 2>&1 && notify-send -u critical -a "P3 loop" "$title" "$body" 2>/dev/null || true
  [ -n "$P3_NOTIFY_URL" ] && curl -fsS -m 10 -H "Title: $title" -d "$body" "$P3_NOTIFY_URL" >/dev/null 2>&1 || true
  log "NOTIFY: $title — $body"
}

last_result_json() { # last_result_json <stream-json log>  — the final {"type":"result"} line, or empty
  [ -f "$1" ] || { echo ""; return; }
  jq -c 'select(.type=="result")' "$1" 2>/dev/null | tail -n 1
}

result_text() { # result_text <result json>  — result + error text joined, for regex matching
  [ -n "$1" ] && printf '%s' "$1" | jq -r '[.result // "", .error // "", (.errors // [] | join(" "))] | join(" ")' 2>/dev/null || echo ""
}

limit_family() { # limit_family <text> <sitting model>  — fable | opus | unknown
  local t; t="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  case "$t" in *fable*|*mythos*) echo fable; return;; *opus*) echo opus; return;; esac
  case "$2" in *fable*) echo fable;; *opus*) echo opus;; *) echo unknown;; esac
}

parse_reset_epoch() { # parse_reset_epoch <text>  — epoch seconds of the stated reset, or 0 when unparseable
  local t="$1" tz="" when="" now; now=$(date +%s)
  # "(Europe/Berlin)" style zone hint
  if [[ "$t" =~ \(([A-Za-z]+/[A-Za-z_]+)\) ]]; then tz="${BASH_REMATCH[1]}"; fi
  # "resets in 2h 15m" / "in 45 minutes"
  if [[ "$t" =~ (resets?|reset)\ in\ ([0-9]+)h(\ ?([0-9]+)m)? ]]; then
    echo $(( now + BASH_REMATCH[2]*3600 + ${BASH_REMATCH[4]:-0}*60 )); return; fi
  if [[ "$t" =~ (resets?|reset)\ in\ ([0-9]+)\ ?min ]]; then echo $(( now + BASH_REMATCH[2]*60 )); return; fi
  # "resets at 3pm" / "resets 3:30pm" / "resets at 14:00" / "reset at 2026-09-22T15:00:00Z"
  if [[ "$t" =~ (resets?|reset)(\ at)?\ ([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:]+Z?) ]]; then when="${BASH_REMATCH[3]}";
  elif [[ "$t" =~ (resets?|reset)(\ at)?\ ([0-9]{1,2}(:[0-9]{2})?\ ?[ap]m) ]]; then when="${BASH_REMATCH[3]}";
  elif [[ "$t" =~ (resets?|reset)(\ at)?\ ([0-9]{1,2}:[0-9]{2}) ]]; then when="${BASH_REMATCH[3]}"; fi
  [ -z "$when" ] && { echo 0; return; }
  local e; e=$(TZ="${tz:-$(cat /etc/timezone 2>/dev/null || echo UTC)}" date -d "$when" +%s 2>/dev/null || echo 0)
  [ "$e" -gt 0 ] && [ "$e" -lt "$now" ] && e=$(( e + 86400 ))   # a clock time already past today = tomorrow
  echo "${e:-0}"
}

limit_retry_json() { # limit_retry_json <stream log> [live]  — the last system/api_retry event whose error kind names a limit, or empty
  # Kinds: rate_limit, usage_limit, any *limit* (fields per research §4.3: error, retry_delay_ms, error_status).
  # With "live": only when that retry is the last assistant/retry event, i.e. the sitting was still retrying when it
  # ended (a retry the CLI recovered from is not a limit).
  [ -f "$1" ] || return 0
  local last
  if [ "${2:-}" = live ]; then
    last="$(jq -cR 'fromjson? | select(.type=="assistant" or (.type=="system" and .subtype=="api_retry"))' "$1" 2>/dev/null | tail -n 1)"
  else
    last="$(jq -cR 'fromjson? | select(.type=="system" and .subtype=="api_retry" and ((.error // "")|tostring|test("limit";"i")))' "$1" 2>/dev/null | tail -n 1)"
  fi
  [ -n "$last" ] && printf '%s' "$last" | jq -e '.type=="system" and ((.error // "")|tostring|test("limit";"i"))' >/dev/null 2>&1 && printf '%s' "$last"
  return 0
}

retry_reset_epoch() { # retry_reset_epoch <stream log> <api_retry json>  — event time + retry_delay_ms, or 0
  # Event time = the event's own timestamp when it carries one, else the transcript's mtime (the sitting ended retrying).
  local ms ts base=""
  ms="$(printf '%s' "$2" | jq -r '.retry_delay_ms // empty | floor' 2>/dev/null)"
  [ -n "$ms" ] && [ "$ms" -gt 0 ] 2>/dev/null || { echo 0; return; }
  ts="$(printf '%s' "$2" | jq -r '.timestamp // empty' 2>/dev/null)"
  [ -n "$ts" ] && base="$(date -d "$ts" +%s 2>/dev/null)"
  [ -z "$base" ] && base="$(stat -c %Y "$1" 2>/dev/null || date +%s)"
  echo $(( base + (ms + 999) / 1000 ))
}

limit_epoch() { # limit_epoch <prose> <stream log>  — the prose reset time; else from the last limit retry_delay_ms; else 0
  local e rl; e="$(parse_reset_epoch "$1")"
  [ "$e" -gt 0 ] && { echo "$e"; return; }
  rl="$(limit_retry_json "$2")"
  [ -n "$rl" ] && { retry_reset_epoch "$2" "$rl"; return; }
  echo 0
}

classify() { # classify <stream log> <status file> <head_before> <head_after> <sitting model>
  # Prints exactly one line: OUTCOME[:detail...]  where OUTCOME ∈ CONTINUE GATE BLOCKED LIMIT DONE CRASH
  #   GATE:<gate file>   LIMIT:<family>:<reset epoch or 0>   CRASH:<reason>
  local logf="$1" statusf="$2" before="$3" after="$4" model="$5"
  local rj text
  rj="$(last_result_json "$logf")"; text="$(result_text "$rj")"
  # Field semantics verified on 2.1.278 (P3/design/harness-sota-research-2026-09-22.md §1.3): decide on
  # terminal_reason + is_error + text, never on subtype (an API failure reports subtype "success" with is_error true).
  local status429 treason iserr; status429="$(printf '%s' "$rj" | jq -r '.api_error_status // empty' 2>/dev/null)"
  treason="$(printf '%s' "$rj" | jq -r '.terminal_reason // empty' 2>/dev/null)"; iserr="$(printf '%s' "$rj" | jq -r '.is_error // false' 2>/dev/null)"
  # 1. a limit hit anywhere in the result beats everything (a sitting cannot wind down after it)
  if [ -n "$rj" ] && { [ "$status429" = "429" ] || printf '%s' "$text" | /usr/bin/grep -qiE "$LIMIT_RE"; }; then
    echo "LIMIT:$(limit_family "$text" "$model"):$(limit_epoch "$text" "$logf")"; return; fi
  # 2. the sitting's own signature (its last act) — or the StopFailure hook's LIMIT record
  if [ -f "$statusf" ] && jq -e . "$statusf" >/dev/null 2>&1; then
    local oc; oc="$(jq -r '.outcome // empty' "$statusf")"
    case "$oc" in
      CONTINUE|DONE) echo "$oc"; return;;
      BLOCKED) echo "BLOCKED:$(jq -r '.note // ""' "$statusf" | tr '\n' ' ')"; return;;
      GATE) echo "GATE:$(jq -r '.gate // ""' "$statusf")"; return;;
      LIMIT) local fam note; fam="$(jq -r '.family // "unknown"' "$statusf")"; note="$(jq -r '.note // ""' "$statusf")"
             [ "$fam" = unknown ] || [ -z "$fam" ] && fam="$(limit_family "$note" "$model")"
             echo "LIMIT:$fam:$(limit_epoch "$note" "$logf")"; return;;
      *) echo "CRASH:bad status outcome '$oc'"; return;;
    esac
  fi
  # 2b. no signature, and the sitting ended while the CLI was still retrying a limit (system/api_retry, research §4.3):
  #     a deterministic LIMIT even when no result line was written (the wall clock cut it mid-retry)
  local rl; rl="$(limit_retry_json "$logf" live)"
  if [ -n "$rl" ]; then
    echo "LIMIT:$(limit_family "$text $(printf '%s' "$rl" | jq -r '.error // "" | tostring')" "$model"):$(limit_epoch "$text" "$logf")"; return; fi
  # 3. no signature: a budget rail cut the sitting before its wind-down → CAPPED (the next sitting recovers from git)
  case "$treason" in max_turns|budget_exhausted) echo "CAPPED:$treason (turns=$(printf '%s' "$rj" | jq -r '.num_turns // "?"'))"; return;; esac
  # 4. an API failure that is not a limit (auth wall, overloaded, server error): crash class, backoff applies
  if [ "$treason" = api_error ] || { [ "$iserr" = true ] && [ "$(printf '%s' "$rj" | jq -r '.duration_api_ms // 1')" = 0 ]; }; then
    echo "CRASH:api_error $(printf '%s' "$text" | tr '\n' ' ' | cut -c1-160)"; return; fi
  # 5. anything else without a signature
  local sub; sub="$(printf '%s' "$rj" | jq -r '.subtype // empty' 2>/dev/null)"
  [ -z "$rj" ] && { echo "CRASH:no result line (killed, or claude never started)"; return; }
  echo "CRASH:no status.json (terminal_reason=${treason:-?} subtype=${sub:-?} is_error=$iserr turns=$(printf '%s' "$rj" | jq -r '.num_turns // "?"'))"
}

probe_model() { # probe_model <model>  — 0 when a one-turn canary completes on that model (used before resuming after a limit)
  local out; out="$(cd "$P3_ROOT" && timeout 120 claude -p "Reply with exactly the single word OK." --model "$1" --max-turns 1 \
      --permission-mode auto --permission-prompts none --output-format json --no-session-persistence 2>/dev/null)"
  [ "$(printf '%s' "$out" | jq -r '.terminal_reason // ""' 2>/dev/null)" = completed ] && [ "$(printf '%s' "$out" | jq -r '.is_error' 2>/dev/null)" = false ]
}

progress_since() { # progress_since <head_before> <head_after>  — 0 when the sitting committed anything beyond bookkeeping
  # STATE/HANDOFF/history commits happen every sitting, so they are not progress (research §5 gap 5).
  [ "$1" = "$2" ] && return 1
  git -C "$P3_ROOT" diff --name-only "$1" "$2" 2>/dev/null | /usr/bin/grep -vqE '^P3/(STATE|STATE-HISTORY|HANDOFF)\.md$'
}

proc_alive() { # proc_alive <pid>  — 0 while that process exists and is not a zombie (kill -0 also succeeds on an unreaped zombie)
  local s; s="$(ps -o stat= -p "$1" 2>/dev/null)"; s="${s// /}"
  [ -n "$s" ] && [ "${s:0:1}" != Z ]
}

gate_answered() { # gate_answered <gate file>  — 0 when the operator marked it answered (yes|partial)
  [ -f "$1" ] && /usr/bin/grep -qiE '^answered:[[:space:]]*(yes|partial)' "$1"
}

reap_orphans() { # kill test runners no sitting is running (called between sittings only).
  # Bracketed first letters so the pattern never matches a shell whose command line CONTAINS the pattern
  # (the self-match hazard: a pkill that killed its own caller, memory serial-tests-gpu-only); own pid/ppid excluded.
  local pids p keep=" $$ $PPID ${LOOP_PID:-} "
  pids="$(pgrep -f '[g]o test |[.]test -test[.]|[v]itest' 2>/dev/null || true)"; [ -z "$pids" ] && return 0
  for p in $pids; do case "$keep" in *" $p "*) ;; *) log "reaping orphan $p: $(tr '\0' ' ' < /proc/$p/cmdline 2>/dev/null | cut -c1-120)"; kill "$p" 2>/dev/null || true;; esac; done
  return 0
}

sitting_ts() { # sitting_ts <stream log>  — the sitting's <ts> from its transcript name (sitting-<ts>.jsonl); now (UTC) without one
  local ts; ts="$(basename "$1" .jsonl)"; ts="${ts#sitting-}"
  [ -f "$1" ] || ts="$(date -u +%Y%m%d-%H%M%S)"
  echo "$ts"
}

archive_limit() { # archive_limit <stream log> <status file> <classification>  — keep a LIMIT sitting's raw evidence (H-3a)
  # → $P3_OBSERVED_DIR/<sitting ts>/{result.json,status.json,api_retry.jsonl,class.txt}: real observations to promote into tests.
  local d; d="$P3_OBSERVED_DIR/$(sitting_ts "$1")"; mkdir -p "$d" 2>/dev/null || return 0
  last_result_json "$1" > "$d/result.json"
  [ -f "$2" ] && cp -f "$2" "$d/status.json"
  [ -f "$1" ] && jq -cR 'fromjson? | select(.type=="system" and .subtype=="api_retry")' "$1" > "$d/api_retry.jsonl" 2>/dev/null
  [ -s "$d/api_retry.jsonl" ] || rm -f "$d/api_retry.jsonl"
  printf '%s\n' "$3" > "$d/class.txt"
  log "archived LIMIT evidence → $d"
}

read_cap() { # read_cap  — the measured packet cap (1..5); 3 when the file is missing or malformed
  local c; c="$(tr -dc '0-9' < "$P3_CAP_FILE" 2>/dev/null)"
  case "$c" in [1-5]) echo "$c";; *) echo 3;; esac
}

record_sitting() { # record_sitting <stream log> <status file> <classification> <model>  — append one ledger row (H-3c)
  # columns: ts model duration_s turns landed compactions transcript_bytes outcome cap   (cap = the cap that sitting ran under)
  [ -f "$1" ] || return 0   # no transcript = no sitting ran (e.g. a preflight BLOCKED)
  local meta="${1%.jsonl}.meta" ts start end dur turns landed comp bytes cap
  ts="$(basename "$1" .jsonl)"; ts="${ts#sitting-}"
  start="$(sed -n 's/^start=//p' "$meta" 2>/dev/null | head -n 1)"; end="$(sed -n 's/^end=//p' "$meta" 2>/dev/null | tail -n 1)"
  [ -z "$end" ] && end="$(date -u +%FT%TZ)"
  dur=0; [ -n "$start" ] && dur=$(( $(date -d "$end" +%s) - $(date -d "$start" +%s) ))
  turns="$(last_result_json "$1" | jq -r '.num_turns // 0' 2>/dev/null)"; [ -n "$turns" ] || turns=0
  landed=0; [ -f "$2" ] && landed="$(jq -r '(.landed // []) | length' "$2" 2>/dev/null)"; [ -n "$landed" ] || landed=0
  comp=0   # compactions logged by the PreCompact hook inside the sitting's [start, end] window (interactive ones excluded)
  [ -n "$start" ] && [ -f "$P3_COMPACTIONS_LOG" ] && comp="$(jq -rR --arg s "$start" --arg e "$end" \
    'fromjson? | select((.at // "") >= $s and (.at // "") <= $e and (.sitting // "") != "interactive") | 1' "$P3_COMPACTIONS_LOG" 2>/dev/null | wc -l)"
  bytes="$(stat -c %s "$1")"
  cap="$(sed -n 's/^cap=//p' "$meta" 2>/dev/null | head -n 1)"; [ -n "$cap" ] || cap="$(read_cap)"
  mkdir -p "$(dirname "$P3_SITTINGS_TSV")"
  [ -s "$P3_SITTINGS_TSV" ] || printf 'ts\tmodel\tduration_s\tturns\tlanded\tcompactions\ttranscript_bytes\toutcome\tcap\n' > "$P3_SITTINGS_TSV"
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$ts" "$4" "$dur" "$turns" "$landed" "$comp" "$bytes" "${3%%:*}" "$cap" >> "$P3_SITTINGS_TSV"
}

update_cap() { # update_cap  — apply the measured-cap rule to the ledger row of the sitting that just ended (H-3c)
  # any compaction in it → cap−1 (min 1); the last five rows all ran at the current cap, landed it, and compacted
  # zero times → cap+1 (max 5). The ledger is the memory, so the rule survives loop restarts and --once runs.
  [ -s "$P3_SITTINGS_TSV" ] || return 0
  local cap new last comp streak; cap="$(read_cap)"; new="$cap"
  last="$(tail -n 1 "$P3_SITTINGS_TSV")"; [ "$(printf '%s' "$last" | cut -f1)" = ts ] && return 0
  comp="$(printf '%s' "$last" | cut -f6)"
  if [ "${comp:-0}" -gt 0 ] 2>/dev/null; then new=$(( cap > 1 ? cap - 1 : 1 ))
  else
    streak="$(tail -n 5 "$P3_SITTINGS_TSV" | awk -F'\t' -v c="$cap" '$1!="ts" && $9==c && $5>=c && $6==0' | wc -l)"
    [ "$streak" -ge 5 ] && new=$(( cap < 5 ? cap + 1 : 5 ))
  fi
  if [ "$new" != "$cap" ]; then
    echo "$new" > "$P3_CAP_FILE"
    log "CAP $cap → $new ($( [ "$new" -lt "$cap" ] && echo 'compaction in the last sitting' || echo "5 consecutive sittings landed $cap with zero compactions"))"
  fi
  return 0
}

ci_state() { # ci_state  — the latest CI run on main, one line (H-4a):
  #   GREEN <sha> <url> | RUNNING <status> <sha> <url> | RED <conclusion> <sha> <url> | UNKNOWN <reason>
  # Every status other than completed is a run still in progress. UNKNOWN (no gh, an auth or API error, no runs) never holds a sitting.
  command -v gh >/dev/null 2>&1 || { echo "UNKNOWN no gh on PATH"; return; }
  local out rc ef; ef="$(mktemp)"
  out="$(cd "$P3_ROOT" && GH_NO_UPDATE_NOTIFIER=1 GH_PROMPT_DISABLED=1 timeout 60 gh run list --branch main --limit 1 --json status,conclusion,headSha,url 2>"$ef")"; rc=$?
  [ "$rc" = 0 ] || { echo "UNKNOWN gh run list failed (exit $rc): $(tr '\n' ' ' < "$ef" | cut -c1-160 | sed 's/ *$//')"; rm -f "$ef"; return; }
  rm -f "$ef"
  printf '%s' "$out" | jq -r 'if length == 0 then "UNKNOWN no CI runs on main" else .[0] |
      if .status != "completed" then "RUNNING \(.status) \(.headSha) \(.url)"
      elif .conclusion == "success" then "GREEN \(.headSha) \(.url)"
      else "RED \(.conclusion) \(.headSha) \(.url)" end end' 2>/dev/null \
    || echo "UNKNOWN unreadable gh output: $(printf '%s' "$out" | tr '\n' ' ' | cut -c1-120)"
}

tag_landing() { # tag_landing <stream log> <status file> <head_before> <head_after>  — tag a landing sitting (H-4a)
  # A landing: status.json .landed is non-empty, or the sitting committed beyond bookkeeping (progress_since). Its post-sitting
  # HEAD gets the annotated tag sitting/<ts> (ts as in archive_limit; message = the landed list), pushed to origin.
  # Failures are logged, never fatal.
  local landed tag out
  landed="$(jq -r '(.landed // []) | map(tostring) | join(", ")' "$2" 2>/dev/null)"
  [ -n "$landed" ] || progress_since "$3" "$4" || return 0
  tag="sitting/$(sitting_ts "$1")"
  out="$(git -C "$P3_ROOT" tag -a "$tag" -m "${landed:-no landed list; commits beyond bookkeeping ${3:0:7}..${4:0:7}}" "$4" 2>&1)" \
    || { log "TAG $tag failed: $(printf '%s' "$out" | tr '\n' ' ' | cut -c1-160)"; return 0; }
  out="$(GIT_TERMINAL_PROMPT=0 timeout 120 git -C "$P3_ROOT" push -q origin "refs/tags/$tag" 2>&1)" \
    || { log "TAG $tag created, push failed: $(printf '%s' "$out" | tr '\n' ' ' | cut -c1-160)"; return 0; }
  log "TAG $tag → ${4:0:7} pushed (${landed:-progress beyond bookkeeping})"
}
