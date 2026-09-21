#!/usr/bin/env bash
# Unit tests for the loop's classifier + decision on canned results (H-1 acceptance). Run: P3/run/test-classify.sh
set -uo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
F="$RUN_DIR/fixtures"; fail=0; n=0
expect() { # expect <name> <expected-regex> <actual>
  n=$((n+1)); if printf '%s' "$3" | /usr/bin/grep -qE "$2"; then echo "ok   $1 → $3"; else echo "FAIL $1 → '$3' (want /$2/)"; fail=1; fi; }
M="$P3_MODEL_PRIMARY"
expect continue        '^CONTINUE$'                              "$(classify $F/continue.jsonl $F/continue.status.json a b "$M")"
expect gate            '^GATE:P3/gates/sit2-checkpoint-1.md$'    "$(classify $F/gate.jsonl $F/gate.status.json a b "$M")"
expect blocked         '^BLOCKED:needs the Kimi key placed'      "$(classify $F/blocked.jsonl $F/blocked.status.json a b "$M")"
expect done            '^DONE$'                                  "$(classify $F/done.jsonl $F/done.status.json a b "$M")"
expect limit-status    '^LIMIT:fable:0$'                         "$(classify $F/limit-status.jsonl $F/limit-status.status.json a b "$M")"
expect limit-text      '^LIMIT:fable:[1-9][0-9]+$'               "$(classify $F/limit-fable-text.jsonl /nonexistent a a "$M")"
expect limit-429       '^LIMIT:fable:0$'                         "$(classify $F/limit-429.jsonl /nonexistent a a "$M")"
expect limit-opus-in   '^LIMIT:opus:[1-9][0-9]+$'                "$(classify $F/limit-opus-in.jsonl /nonexistent a a "$M")"
expect limit-beats-status '^LIMIT:fable:'                         "$(classify $F/limit-fable-text.jsonl $F/continue.status.json a b "$M")"
expect crash-noresult  '^CRASH:no result line'                   "$(classify $F/crash-noresult.jsonl /nonexistent a a "$M")"
expect capped-maxturns '^CAPPED:max_turns'                      "$(classify $F/crash-maxturns.jsonl /nonexistent a a "$M")"
expect api-error       '^CRASH:api_error Not logged in'          "$(classify $F/api-error.jsonl /nonexistent a a "$M")"
expect limit-hook      '^LIMIT:fable:[1-9][0-9]+$'               "$(classify $F/continue.jsonl $F/limit-hook.status.json a a "$M")"
expect crash-nostatus  '^CRASH:no status.json'                   "$(classify $F/continue.jsonl /nonexistent a b "$M")"
expect reset-3pm       '^[1-9][0-9]+$'                           "$(parse_reset_epoch 'hit your limit · resets 3pm (Europe/Berlin)')"
expect reset-in        '^[1-9][0-9]+$'                           "$(parse_reset_epoch 'resets in 2h 15m')"
expect reset-none      '^0$'                                     "$(parse_reset_epoch 'no time given')"
# the 3pm case: must be in the future and within 24 h
e=$(parse_reset_epoch 'resets 3pm (Europe/Berlin)'); now=$(date +%s); n=$((n+1))
if [ "$e" -gt "$now" ] && [ "$e" -le $((now+86400)) ]; then echo "ok   reset-3pm-window → $(date -u -d @$e +%FT%TZ)"; else echo "FAIL reset-3pm-window → $e"; fail=1; fi
# gate marker
t=$(mktemp); printf 'Status: OPEN\nanswered: no\n' > "$t"; n=$((n+1)); gate_answered "$t" && { echo "FAIL gate-no"; fail=1; } || echo "ok   gate-no"
printf 'Status: ANSWERED\nanswered: yes\n' > "$t"; n=$((n+1)); gate_answered "$t" && echo "ok   gate-yes" || { echo "FAIL gate-yes"; fail=1; }
printf 'answered: partial\n' > "$t"; n=$((n+1)); gate_answered "$t" && echo "ok   gate-partial" || { echo "FAIL gate-partial"; fail=1; }; rm -f "$t"
# loop --dry-run decisions
expect dry-continue 'ACTION NEXT after 120s'                     "$("$RUN_DIR/loop.sh" --dry-run $F/continue.jsonl $F/continue.status.json a b | tail -1)"
expect dry-gate     'ACTION WAIT for answered'                   "$("$RUN_DIR/loop.sh" --dry-run $F/gate.jsonl $F/gate.status.json a b | tail -1)"
expect dry-limit    "ACTION SWITCH to $P3_MODEL_FALLBACK"        "$("$RUN_DIR/loop.sh" --dry-run $F/limit-fable-text.jsonl /nonexistent a a | tail -1)"
expect dry-opus     'ACTION SLEEP until'                         "$("$RUN_DIR/loop.sh" --dry-run $F/limit-opus-in.jsonl /nonexistent a a | tail -1)"
expect dry-crash    'ACTION CRASH #1'                            "$("$RUN_DIR/loop.sh" --dry-run $F/crash-noresult.jsonl /nonexistent a a | tail -1)"
expect dry-done     'ACTION EXIT 0'                              "$("$RUN_DIR/loop.sh" --dry-run $F/done.jsonl $F/done.status.json a b | tail -1)"
expect dry-capped   'ACTION NEXT after 120s \(budget rail'      "$("$RUN_DIR/loop.sh" --dry-run $F/crash-maxturns.jsonl /nonexistent a a | tail -1)"
echo "---- $n checks, $([ $fail = 0 ] && echo ALL PASS || echo FAILURES)"; exit $fail
