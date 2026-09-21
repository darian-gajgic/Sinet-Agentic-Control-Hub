#!/usr/bin/env bash
# P3 continuous-run supervisor (H-1). Runs budgeted fresh sittings until DONE, a breaker, or P3/run/STOP.
# Usage: P3/run/loop.sh [--once] [--dry-run <stream.jsonl> [<status.json>] [<head_before> <head_after>]]
#   --once     run exactly one sitting, classify, act on notifications, then exit (H-2(a) supervised run)
#   --dry-run  classify canned inputs and print the decision; nothing is launched
# Stop:  touch P3/run/STOP  (takes effect before the next sitting; Ctrl-C forwards SIGINT to the running sitting)
# Resume after a gate/blocked wait without editing the gate file:  touch P3/run/RESUME
set -uo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cd "$P3_ROOT"

decide() { # decide <classification> → prints the action line the loop takes (pure; used by --dry-run and tests)
  local c="$1"
  case "$c" in
    CONTINUE)  echo "NEXT after the pause (${P3_PAUSE_MIN}s, growing to ${P3_PAUSE_MAX}s while sittings make no progress)";;
    CAPPED:*)  echo "NEXT after the pause (budget rail: ${c#CAPPED:}; the next sitting recovers from git)";;
    DONE)      echo "EXIT 0 (queue empty)";;
    GATE:*)    echo "WAIT for answered:yes|partial in ${c#GATE:} (poll 600s), notify";;
    BLOCKED:*) echo "WAIT for RESUME file (poll 600s), notify: ${c#BLOCKED:}";;
    LIMIT:*)   local fam="${c#LIMIT:}"; local reset="${fam#*:}"; fam="${fam%%:*}"
               if [ "$fam" = fable ] && [ "$MODEL" = "$P3_MODEL_PRIMARY" ]; then echo "SWITCH to $P3_MODEL_FALLBACK until $( [ "$reset" -gt 0 ] && date -u -d "@$reset" +%FT%TZ || echo 'backoff' ), NEXT";
               else echo "SLEEP until $( [ "$reset" -gt 0 ] && date -u -d "@$reset" +%FT%TZ || echo "backoff ${BACKOFF}s" ), NEXT"; fi;;
    CRASH:*)   echo "CRASH #$((CRASHES+1)) (${c#CRASH:}); $([ $((CRASHES+1)) -ge 3 ] && echo 'EXIT 1 breaker' || echo 'NEXT after 300s')";;
    *)         echo "EXIT 1 (unclassifiable: $c)";;
  esac
}

MODEL="$P3_MODEL_PRIMARY"; CRASHES=0; STALLS=0; BACKOFF=900; PAUSE="$P3_PAUSE_MIN"; PROGRESS=0; FABLE_LIMITED_UNTIL=0; ONCE=0

if [ "${1:-}" = "--dry-run" ]; then
  shift; LOGF="${1:-/dev/null}"; STF="${2:-/nonexistent}"; HB="${3:-a}"; HA="${4:-b}"
  c="$(classify "$LOGF" "$STF" "$HB" "$HA" "$MODEL")"; echo "CLASS $c"; echo "ACTION $(decide "$c")"; exit 0
fi
[ "${1:-}" = "--once" ] && ONCE=1

exec 9>"$RUN_DIR/loop.lock"
flock -n 9 || { echo "another loop holds $RUN_DIR/loop.lock" >&2; exit 1; }
echo $$ > "$RUN_DIR/loop.pid"; LOOP_PID=$$
CHILD=""
trap 'log "signal: forwarding SIGINT to the sitting"; [ -n "$CHILD" ] && kill -INT "$CHILD" 2>/dev/null; wait "$CHILD" 2>/dev/null; log "loop exiting on signal"; exit 130' INT TERM

wait_for() { # wait_for <predicate-fn> <label> — poll every 600 s; STOP file ends the loop
  while ! "$1"; do
    [ -f "$STOP_FILE" ] && { log "STOP file seen while waiting ($2)"; exit 0; }
    [ -f "$RESUME_FILE" ] && { rm -f "$RESUME_FILE"; log "RESUME touched ($2)"; return 0; }
    sleep 600
  done
}

log "LOOP start pid=$$ primary=$P3_MODEL_PRIMARY fallback=$P3_MODEL_FALLBACK once=$ONCE"
while :; do
  rotate_log
  [ -f "$STOP_FILE" ] && { log "STOP file present — exiting"; exit 0; }
  rm -f "$RESUME_FILE"
  reap_orphans
  # model choice: back to primary once the Fable window has passed
  if [ "$MODEL" != "$P3_MODEL_PRIMARY" ] && [ "$(date +%s)" -ge "$FABLE_LIMITED_UNTIL" ]; then
    if probe_model "$P3_MODEL_PRIMARY"; then MODEL="$P3_MODEL_PRIMARY"; log "Fable window passed (probe OK) — back to $MODEL"
    else FABLE_LIMITED_UNTIL=$(( $(date +%s) + P3_PROBE_INTERVAL )); log "Fable probe still limited — staying on $MODEL for ${P3_PROBE_INTERVAL}s"; fi
  fi
  HEAD_BEFORE="$(git rev-parse HEAD)"
  "$RUN_DIR/sitting.sh" --model "$MODEL" > "$LOG_DIR/.last-sitting-path" 2>&1 & CHILD=$!
  wait "$CHILD"; CHILD=""
  LOGF="$(tail -n 1 "$LOG_DIR/.last-sitting-path")"
  HEAD_AFTER="$(git rev-parse HEAD)"
  CLASS="$(classify "$LOGF" "$STATUS_FILE" "$HEAD_BEFORE" "$HEAD_AFTER" "$MODEL")"
  ACTION="$(decide "$CLASS")"
  if progress_since "$HEAD_BEFORE" "$HEAD_AFTER"; then PROGRESS=1; else PROGRESS=0; fi
  log "CLASS $CLASS | progress $( [ "$PROGRESS" = 1 ] && echo yes || echo "none (bookkeeping-only or no commit)" ) | ACTION $ACTION"
  # stall breaker: three consecutive COMPLETED sittings without progress beyond STATE/HANDOFF bookkeeping
  # (crashes have their own breaker; limits count toward neither); idle backoff: the pause between
  # unproductive sittings grows P3_PAUSE_MIN ×5 per step up to P3_PAUSE_MAX (diminishing returns)
  case "$CLASS" in CONTINUE|CAPPED:*|GATE:*|BLOCKED:*)
    if [ "$PROGRESS" = 1 ]; then STALLS=0; PAUSE="$P3_PAUSE_MIN"; else STALLS=$((STALLS+1)); PAUSE=$(( PAUSE*5 > P3_PAUSE_MAX ? P3_PAUSE_MAX : PAUSE*5 )); fi
    [ "$STALLS" -ge 3 ] && { notify "P3 loop stopped: stall" "3 sittings without progress beyond bookkeeping — see $LOG_DIR"; exit 1; };;
  esac
  case "$CLASS" in
    CONTINUE|CAPPED:*) CRASHES=0; BACKOFF=900; [ "$ONCE" = 1 ] && { log "--once: done"; exit 0; }; log "next sitting in ${PAUSE}s"; sleep "$PAUSE";;
    DONE)     notify "P3 loop finished" "Queue empty — DONE"; exit 0;;
    GATE:*)   CRASHES=0; GATEF="${CLASS#GATE:}"; notify "P3 needs a decision" "Gate file: $GATEF — answer in the file (answered: yes) or in a session"
              [ "$ONCE" = 1 ] && exit 0
              wait_for "gate_answered $GATEF" "gate $GATEF"; log "gate answered/resumed: $GATEF";;
    BLOCKED:*) CRASHES=0; notify "P3 loop blocked" "${CLASS#BLOCKED:}"; [ "$ONCE" = 1 ] && exit 0
              wait_for false "blocked";;
    LIMIT:*)  CRASHES=0; FAM="${CLASS#LIMIT:}"; RESET="${FAM#*:}"; FAM="${FAM%%:*}"
              if [ "$FAM" = fable ] && [ "$MODEL" = "$P3_MODEL_PRIMARY" ]; then
                FABLE_LIMITED_UNTIL=$(( RESET > 0 ? RESET : $(date +%s) + 3600 )); MODEL="$P3_MODEL_FALLBACK"
                log "Fable limit — switching sittings to $MODEL until $(date -u -d "@$FABLE_LIMITED_UNTIL" +%FT%TZ)"; [ "$ONCE" = 1 ] && exit 0; sleep "$P3_SWITCH_PAUSE"
              else
                SLEEP=$(( RESET > 0 ? RESET - $(date +%s) + 60 : BACKOFF )); [ "$SLEEP" -lt 60 ] && SLEEP=60
                log "limit on $FAM — sleeping ${SLEEP}s, then canary probes every ${P3_PROBE_INTERVAL}s"; BACKOFF=$(( BACKOFF < 3600 ? BACKOFF*2 : 3600 )); [ "$BACKOFF" -gt 3600 ] && BACKOFF=3600
                [ "$ONCE" = 1 ] && exit 0; sleep "$SLEEP"
                until probe_model "$MODEL"; do [ -f "$STOP_FILE" ] && { log "STOP while limited"; exit 0; }; log "probe on $MODEL still limited"; sleep "$P3_PROBE_INTERVAL"; done
                log "probe on $MODEL completed — resuming"
              fi;;
    CRASH:*)  CRASHES=$((CRASHES+1)); log "crash #$CRASHES: ${CLASS#CRASH:}"
              [ "$CRASHES" -ge 3 ] && { notify "P3 loop stopped: crashes" "3 consecutive sittings without a status file — see $LOG_DIR"; exit 1; }
              [ "$ONCE" = 1 ] && exit 1; sleep "$P3_CRASH_PAUSE";;
    *)        notify "P3 loop stopped" "unclassifiable: $CLASS"; exit 1;;
  esac
done
