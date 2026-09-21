#!/usr/bin/env bash
# Breaker tests for loop.sh with a stub `claude` (no API calls). Research §2.10 lesson 10: assert the counters actually move.
set -uo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"; cd "$P3_ROOT"
F="$RUN_DIR/fixtures"; T="$(mktemp -d)"; fail=0
cat > "$T/claude" <<'STUB'
#!/usr/bin/env bash
# pops the next fixture name from $STUB_SEQ (space-separated); "stop" touches the STOP file and emits a CONTINUE
STUB_SEQ="$(cat "$STUB_SEQ_FILE")"   # read from the file: timeout/env exec the real PATH entry, so no shell function or env value survives
next="${STUB_SEQ%% *}"; rest="${STUB_SEQ#* }"; [ "$rest" = "$STUB_SEQ" ] && rest=""
printf '%s' "$rest" > "$STUB_SEQ_FILE"
case "$next" in
  crash)    cat "$STUB_FIX/crash-noresult.jsonl";;
  continue) cat "$STUB_FIX/continue.jsonl"; cp "$STUB_FIX/continue.status.json" "$STUB_STATUS";;
  limit)    cat "$STUB_FIX/limit-fable-text.jsonl";;
  stop)     touch "$STUB_STOP"; cat "$STUB_FIX/continue.jsonl"; cp "$STUB_FIX/continue.status.json" "$STUB_STATUS";;
esac
STUB
chmod +x "$T/claude"
run_seq() { # run_seq <name> <sequence> <expected exit>
  printf '%s' "$2" > "$T/seq"; rm -f "$STOP_FILE" "$STATUS_FILE"
  ( export PATH="$T:$PATH" STUB_FIX="$F" STUB_STATUS="$STATUS_FILE" STUB_STOP="$STOP_FILE" STUB_SEQ_FILE="$T/seq"
    export P3_CRASH_PAUSE=1 P3_PAUSE_MIN=1 P3_PAUSE_MAX=2 P3_SWITCH_PAUSE=1
    timeout 120 "$RUN_DIR/loop.sh" >/dev/null 2>&1 ); local rc=$?
  rm -f "$STOP_FILE" "$STATUS_FILE"
  if [ "$rc" = "$3" ]; then echo "ok   $1 → exit $rc"; else echo "FAIL $1 → exit $rc (want $3)"; fail=1; fi
}
run_seq crash-breaker            "crash crash crash stop"                 1
run_seq stall-breaker            "continue continue continue stop"        1
run_seq limit-resets-crash-count "crash crash limit crash crash stop"     0
run_seq stop-file                "stop"                                   0
rm -rf "$T"; echo "---- $([ $fail = 0 ] && echo ALL PASS || echo FAILURES)"; exit $fail
