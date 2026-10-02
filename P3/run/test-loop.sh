#!/usr/bin/env bash
# Breaker tests for loop.sh with a stub `claude` (no API calls). Research §2.10 lesson 10: assert the counters actually move.
# H-3: also the LIMIT archive, the update guard (sitting.sh preflight) and the measured cap, all against temp state files.
set -uo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"; cd "$P3_ROOT"
F="$RUN_DIR/fixtures"; T="$(mktemp -d)"; fail=0; n=0
cat > "$T/claude" <<'STUB'
#!/usr/bin/env bash
# --version → $STUB_VERSION (default: the pinned version). The probe prompt → completed/api_error per $STUB_PROBE.
# Otherwise pops the next fixture name from the sequence file; "stop" touches the STOP file and emits a CONTINUE.
[ "${DISABLE_AUTOUPDATER:-}" = 1 ] && echo "DISABLE_AUTOUPDATER=1 $*" | head -n 1 | cut -c1-80 >> "$STUB_ENV_OUT"
if [ "${1:-}" = --version ]; then echo "${STUB_VERSION:-$(cat "$P3_CLI_PIN")} (Claude Code)"; exit 0; fi
if [ "${2:-}" = "Reply with exactly the single word OK." ]; then
  if [ "${STUB_PROBE:-ok}" = ok ]; then echo '{"type":"result","subtype":"success","is_error":false,"terminal_reason":"completed","result":"OK"}'
  else echo '{"type":"result","subtype":"success","is_error":true,"terminal_reason":"api_error","result":"API Error: 500"}'; fi; exit 0; fi
printf '%s\n' "$2" >> "$STUB_PROMPTS"
STUB_SEQ="$(cat "$STUB_SEQ_FILE")"   # read from the file: timeout/env exec the real PATH entry, so no shell function survives
next="${STUB_SEQ%% *}"; rest="${STUB_SEQ#* }"; [ "$rest" = "$STUB_SEQ" ] && rest=""
printf '%s' "$rest" > "$STUB_SEQ_FILE"
land3() { cat "$STUB_FIX/continue.jsonl"; echo '{"outcome":"CONTINUE","landed":["A","B","C"],"next":"x","gate":null,"family":null,"note":""}' > "$STUB_STATUS"; }
case "$next" in
  crash)      cat "$STUB_FIX/crash-noresult.jsonl";;
  continue)   cat "$STUB_FIX/continue.jsonl"; cp "$STUB_FIX/continue.status.json" "$STUB_STATUS";;
  limit)      cat "$STUB_FIX/limit-fable-text.jsonl";;
  limitretry) cat "$STUB_FIX/limit-retry.jsonl";;
  compact)    echo "{\"at\":\"$(date -u +%FT%TZ)\",\"session\":\"s\",\"reason\":\"auto\",\"sitting\":\"1\"}" >> "$P3_COMPACTIONS_LOG"
              cat "$STUB_FIX/continue.jsonl"; cp "$STUB_FIX/continue.status.json" "$STUB_STATUS";;
  land3)      land3;;
  land3stop)  touch "$STUB_STOP"; land3;;
  stop)       touch "$STUB_STOP"; cat "$STUB_FIX/continue.jsonl"; cp "$STUB_FIX/continue.status.json" "$STUB_STATUS";;
esac
STUB
chmod +x "$T/claude"
PIN0="$(cat "$P3_CLI_PIN")"
reset_state() { # fresh temp cap/ledger/pin/observed for each test (the real P3/run files are never touched)
  echo 3 > "$T/cap"; rm -rf "$T/sittings.tsv" "$T/compactions.log" "$T/observed" "$T/prompts" "$T/env"
  printf '%s\n' "$PIN0" > "$T/pin"; rm -f "$STOP_FILE" "$STATUS_FILE"
}
stub_env() { # exported into the subshell that runs loop.sh / sitting.sh
  export PATH="$T:$PATH" STUB_FIX="$F" STUB_STATUS="$STATUS_FILE" STUB_STOP="$STOP_FILE" STUB_SEQ_FILE="$T/seq"
  export STUB_PROMPTS="$T/prompts" STUB_ENV_OUT="$T/env"
  export P3_CRASH_PAUSE=1 P3_PAUSE_MIN=1 P3_PAUSE_MAX=2 P3_SWITCH_PAUSE=1
  export P3_CAP_FILE="$T/cap" P3_SITTINGS_TSV="$T/sittings.tsv" P3_COMPACTIONS_LOG="$T/compactions.log"
  export P3_CLI_PIN="$T/pin" P3_OBSERVED_DIR="$T/observed" DBUS_SESSION_BUS_ADDRESS="unix:path=/nonexistent" P3_NOTIFY_URL=""
}
ok() { n=$((n+1)); echo "ok   $1"; }
bad() { n=$((n+1)); echo "FAIL $1"; fail=1; }
check() { # check <name> <expected-regex> <actual>
  if printf '%s' "$3" | /usr/bin/grep -qE "$2"; then ok "$1 → $3"; else bad "$1 → '$3' (want /$2/)"; fi; }
run_seq() { # run_seq <name> <sequence> <expected exit> [--once]
  printf '%s' "$2" > "$T/seq"; rm -f "$STOP_FILE" "$STATUS_FILE"
  ( stub_env; timeout 120 "$RUN_DIR/loop.sh" ${4:-} >/dev/null 2>&1 ); local rc=$?
  rm -f "$STOP_FILE" "$STATUS_FILE"
  if [ "$rc" = "$3" ]; then ok "$1 → exit $rc"; else bad "$1 → exit $rc (want $3)"; fi
}
rows() { [ -f "$T/sittings.tsv" ] && tail -n +2 "$T/sittings.tsv" | wc -l || echo 0; }

# ---- H-1 breakers
reset_state; run_seq crash-breaker            "crash crash crash stop"                 1
reset_state; run_seq stall-breaker            "continue continue continue stop"        1
reset_state; run_seq limit-resets-crash-count "crash crash limit crash crash stop"     0
reset_state; run_seq stop-file                "stop"                                   0

# ---- H-3a: every LIMIT-classified sitting is archived as an observation
reset_state; run_seq limit-archive "limitretry stop" 0
D="$(ls -d "$T"/observed/*/ 2>/dev/null | head -n 1)"
check archive-dir        '/observed/[0-9]{8}-[0-9]{6}/$'  "${D:-none}"
check archive-class      '^LIMIT:fable:[1-9][0-9]+$'       "$(cat "${D}class.txt" 2>/dev/null)"
check archive-retries    '^2$'                              "$(wc -l < "${D}api_retry.jsonl" 2>/dev/null)"
check archive-result     '^present$'                        "$([ -f "${D}result.json" ] && echo present || echo missing)"
check archive-only-limit '^1$'                              "$(ls -d "$T"/observed/*/ | wc -l)"
reset_state; run_seq limit-archive-status "crash crash limit crash crash stop" 0
check archive-text-limit '^LIMIT:fable:'                    "$(cat "$T"/observed/*/class.txt 2>/dev/null | head -n 1)"

# ---- H-3b: update guard
reset_state; printf '' > "$T/seq"
( stub_env; export STUB_VERSION=9.9.9 STUB_PROBE=fail; "$RUN_DIR/sitting.sh" >/dev/null 2>&1 ); rc=$?
check drift-fail-exit    '^3$'                              "$rc"
check drift-fail-status  '^BLOCKED$'                        "$(jq -r .outcome "$STATUS_FILE" 2>/dev/null)"
check drift-fail-note    "^CLI drift $PIN0→9.9.9: smoke failed$" "$(jq -r .note "$STATUS_FILE" 2>/dev/null)"
check drift-fail-pin     "^$PIN0$"                          "$(cat "$T/pin")"
check drift-fail-nosit   '^0$'                              "$( [ -f "$T/prompts" ] && wc -l < "$T/prompts" || echo 0)"
reset_state
( stub_env; export STUB_VERSION=9.9.9 STUB_PROBE=fail; printf '' > "$T/seq"; timeout 60 "$RUN_DIR/loop.sh" --once >/dev/null 2>&1 ); rc=$?
check drift-fail-loop    '^0$'                              "$rc"
check drift-fail-loopcls 'CLASS BLOCKED:CLI drift .*smoke failed' "$(/usr/bin/grep 'CLASS ' "$LOOP_LOG" | tail -n 1)"
check drift-fail-norow   '^0$'                              "$(rows)"
rm -f "$STATUS_FILE"
reset_state; NB="$(/usr/bin/grep -c 'NOTIFY: P3 CLI updated' "$LOOP_LOG" 2>/dev/null)"; NB=${NB:-0}
printf 'continue' > "$T/seq"
( stub_env; export STUB_VERSION=9.9.9 STUB_PROBE=ok; "$RUN_DIR/sitting.sh" >/dev/null 2>&1 ); rc=$?
check drift-pass-exit    '^0$'                              "$rc"
check drift-pass-pin     '^9\.9\.9$'                        "$(cat "$T/pin")"
check drift-pass-ran     '^1$'                              "$(/usr/bin/grep -c '^continue implementation' "$T/prompts" 2>/dev/null)"
printf 'continue' > "$T/seq"
( stub_env; export STUB_VERSION=9.9.9 STUB_PROBE=ok; "$RUN_DIR/sitting.sh" >/dev/null 2>&1 )
check drift-notify-once  "^$((NB+1))$"                      "$(/usr/bin/grep -c 'NOTIFY: P3 CLI updated' "$LOOP_LOG")"
check autoupdater-off    '^DISABLE_AUTOUPDATER=1 --version'  "$(head -n 1 "$T/env" 2>/dev/null)"
check autoupdater-sitting '^[1-9]'                          "$(/usr/bin/grep -c '^DISABLE_AUTOUPDATER=1 -p continue implementation' "$T/env" 2>/dev/null)"
rm -f "$STATUS_FILE"

# ---- H-3c: measured cap
reset_state; run_seq cap-down "compact stop" 0
check cap-down-file      '^2$'                              "$(cat "$T/cap")"
check cap-down-rows      '^2$'                              "$(rows)"
check cap-ledger-header  '^ts\|model\|duration_s\|turns\|landed\|compactions\|transcript_bytes\|outcome\|cap$' "$(sed -n 1p "$T/sittings.tsv" | tr '\t' '|')"
check cap-down-ledger    '^[0-9]{8}-[0-9]{6}\|claude-fable-5-1\[1m\]\|[0-9]+\|212\|1\|1\|[0-9]+\|CONTINUE\|3$' "$(sed -n 2p "$T/sittings.tsv" | tr '\t' '|')"
check cap-down-row2      '\|1\|0\|[0-9]+\|CONTINUE\|2$'     "$(sed -n 3p "$T/sittings.tsv" | tr '\t' '|')"
check cap-prompt-1       'land at most 3 packets this sitting' "$(/usr/bin/grep -o 'land at most [0-9] packets this sitting' "$T/prompts" | sed -n 1p)"
check cap-prompt-2       'land at most 2 packets this sitting' "$(/usr/bin/grep -o 'land at most [0-9] packets this sitting' "$T/prompts" | sed -n 2p)"
seed() { # seed <cap> <n rows> [compactions on the last seeded row]
  printf 'ts\tmodel\tduration_s\tturns\tlanded\tcompactions\ttranscript_bytes\toutcome\tcap\n' > "$T/sittings.tsv"
  local i; for i in $(seq 1 "$2"); do printf '2026010%d-000000\tm\t100\t50\t%s\t%s\t9\tCONTINUE\t%s\n' "$i" "$1" "$( [ "$i" = "$2" ] && echo "${3:-0}" || echo 0)" "$1" >> "$T/sittings.tsv"; done
  echo "$1" > "$T/cap"; }
reset_state; seed 3 4; run_seq cap-up "land3stop" 0
check cap-up-file        '^4$'                              "$(cat "$T/cap")"
reset_state; seed 3 4 1; run_seq cap-up-broken-streak "land3stop" 0
check cap-up-broken      '^3$'                              "$(cat "$T/cap")"
reset_state; seed 3 3; run_seq cap-up-short-streak "land3stop" 0
check cap-up-short       '^3$'                              "$(cat "$T/cap")"
reset_state; seed 1 1 1
check cap-min            '^1$'                              "$( ( stub_env; update_cap >/dev/null; cat "$T/cap" ) )"
reset_state; seed 5 5
check cap-max            '^5$'                              "$( ( stub_env; update_cap >/dev/null; cat "$T/cap" ) )"
reset_state; echo garbage > "$T/cap"
check cap-default        '^3$'                              "$( ( stub_env; read_cap ) )"

rm -f "$STOP_FILE" "$STATUS_FILE"; rm -rf "$T"; echo "---- $n checks, $([ $fail = 0 ] && echo ALL PASS || echo FAILURES)"; exit $fail
