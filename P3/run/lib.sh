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

classify() { # classify <stream log> <status file> <head_before> <head_after> <sitting model>
  # Prints exactly one line: OUTCOME[:detail...]  where OUTCOME ∈ CONTINUE GATE BLOCKED LIMIT DONE CRASH
  #   GATE:<gate file>   LIMIT:<family>:<reset epoch or 0>   CRASH:<reason>
  local logf="$1" statusf="$2" before="$3" after="$4" model="$5"
  local rj text
  rj="$(last_result_json "$logf")"; text="$(result_text "$rj")"
  # 1. a limit hit anywhere in the result beats everything (a sitting cannot write a status after it)
  local status429; status429="$(printf '%s' "$rj" | jq -r '.api_error_status // empty' 2>/dev/null)"
  if [ -n "$rj" ] && { [ "$status429" = "429" ] || printf '%s' "$text" | /usr/bin/grep -qiE "$LIMIT_RE"; }; then
    echo "LIMIT:$(limit_family "$text" "$model"):$(parse_reset_epoch "$text")"; return; fi
  # 2. the sitting's own signature
  if [ -f "$statusf" ] && jq -e . "$statusf" >/dev/null 2>&1; then
    local oc; oc="$(jq -r '.outcome // empty' "$statusf")"
    case "$oc" in
      CONTINUE|DONE) echo "$oc"; return;;
      BLOCKED) echo "BLOCKED:$(jq -r '.note // ""' "$statusf" | tr '\n' ' ')"; return;;
      GATE) echo "GATE:$(jq -r '.gate // ""' "$statusf")"; return;;
      LIMIT) echo "LIMIT:$(jq -r '.family // "unknown"' "$statusf"):0"; return;;
      *) echo "CRASH:bad status outcome '$oc'"; return;;
    esac
  fi
  # 3. no signature: crash class, with the best reason we have
  local sub; sub="$(printf '%s' "$rj" | jq -r '.subtype // empty' 2>/dev/null)"
  [ -z "$rj" ] && { echo "CRASH:no result line (killed, or claude never started)"; return; }
  echo "CRASH:no status.json (subtype=${sub:-?} is_error=$(printf '%s' "$rj" | jq -r '.is_error // "?"') turns=$(printf '%s' "$rj" | jq -r '.num_turns // "?"'))"
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
