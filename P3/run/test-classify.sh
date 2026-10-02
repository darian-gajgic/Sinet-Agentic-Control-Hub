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
# progress predicate: bookkeeping-only commits are not progress
B=$(git -C "$P3_ROOT" rev-parse HEAD); n=$((n+1)); progress_since "$B" "$B" && { echo "FAIL progress-same-head"; fail=1; } || echo "ok   progress-same-head → none"
P=$(git -C "$P3_ROOT" log --format=%H -1 --diff-filter=A -- P3/run/loop.sh); n=$((n+1)); progress_since "$P~1" "$P" && echo "ok   progress-real-commit → yes" || { echo "FAIL progress-real-commit"; fail=1; }
# loop --dry-run decisions
expect dry-continue 'ACTION NEXT after the pause'                     "$("$RUN_DIR/loop.sh" --dry-run $F/continue.jsonl $F/continue.status.json a b | tail -1)"
expect dry-gate     'ACTION WAIT for answered'                   "$("$RUN_DIR/loop.sh" --dry-run $F/gate.jsonl $F/gate.status.json a b | tail -1)"
expect dry-limit    "ACTION SWITCH to $P3_MODEL_FALLBACK"        "$("$RUN_DIR/loop.sh" --dry-run $F/limit-fable-text.jsonl /nonexistent a a | tail -1)"
expect dry-opus     'ACTION SLEEP until'                         "$("$RUN_DIR/loop.sh" --dry-run $F/limit-opus-in.jsonl /nonexistent a a | tail -1)"
expect dry-crash    'ACTION CRASH #1'                            "$("$RUN_DIR/loop.sh" --dry-run $F/crash-noresult.jsonl /nonexistent a a | tail -1)"
expect dry-done     'ACTION EXIT 0'                              "$("$RUN_DIR/loop.sh" --dry-run $F/done.jsonl $F/done.status.json a b | tail -1)"
expect dry-capped   'ACTION NEXT after the pause \(budget rail'      "$("$RUN_DIR/loop.sh" --dry-run $F/crash-maxturns.jsonl /nonexistent a a | tail -1)"
# ---- H-3a deterministic limits: system/api_retry events (research §4.3)
MT=$(stat -c %Y "$F/limit-retry.jsonl")
expect retry-live-killed   "^LIMIT:fable:$((MT+5400))$"               "$(classify $F/limit-retry.jsonl /nonexistent a a "$M")"
expect retry-opus-model    "^LIMIT:opus:$((MT+5400))$"                "$(classify $F/limit-retry.jsonl /nonexistent a a claude-opus-5)"
expect retry-429-noprose   "^LIMIT:fable:$(( $(date -d 2026-09-22T03:00:00Z +%s) + 7200 ))$" "$(classify $F/limit-429-retry.jsonl /nonexistent a a "$M")"
expect retry-hook-notime   "^LIMIT:fable:$((MT+5400))$"               "$(classify $F/limit-retry.jsonl $F/limit-hook-notime.status.json a a "$M")"
expect retry-prose-wins    '^LIMIT:fable:[1-9][0-9]+$'                "$(classify $F/limit-retry.jsonl $F/limit-hook.status.json a a "$M")"
e=$(classify $F/limit-retry.jsonl $F/limit-hook.status.json a a "$M"); n=$((n+1))
[ "${e##*:}" != "$((MT+5400))" ] && echo "ok   retry-prose-wins-epoch → prose reset kept" || { echo "FAIL retry-prose-wins-epoch → $e"; fail=1; }
expect retry-recovered-ok  '^CONTINUE$'                               "$(classify $F/limit-retry-recovered.jsonl $F/continue.status.json a b "$M")"
expect retry-recovered-nosig '^CRASH:no status.json'                 "$(classify $F/limit-retry-recovered.jsonl /nonexistent a a "$M")"
expect dry-retry           "ACTION SWITCH to $P3_MODEL_FALLBACK"      "$("$RUN_DIR/loop.sh" --dry-run $F/limit-retry.jsonl /nonexistent a a | tail -1)"
# ---- H-3a the StopFailure hook COMMAND from hooks.proposed.json, run in isolation
H="$(mktemp -d)"; mkdir -p "$H/P3/run"
HC="$(jq -r '.StopFailure[0].hooks[0].command' "$RUN_DIR/hooks.proposed.json")"
HIN='{"hook_event_name":"StopFailure","session_id":"s1","error":"usage_limit","error_details":"You have hit your Claude Fable limit · resets 9am (Europe/Berlin)"}'
printf '%s' "$HIN" | CLAUDE_PROJECT_DIR="$H" P3_SITTING=1 sh -c "$HC"
expect hook-writes-json    '^ok$'                                     "$(jq -e . "$H/P3/run/status.json" >/dev/null 2>&1 && echo ok || echo 'no valid status.json')"
expect hook-outcome        '^LIMIT$'                                  "$(jq -r .outcome "$H/P3/run/status.json" 2>/dev/null)"
expect hook-source         '^StopFailure$'                            "$(jq -r .hook "$H/P3/run/status.json" 2>/dev/null)"
expect hook-note           '^usage_limit You have hit your Claude Fable limit · resets 9am' "$(jq -r .note "$H/P3/run/status.json" 2>/dev/null)"
expect hook-ended          '^20[0-9]{2}-[0-9]{2}-[0-9]{2}T'           "$(jq -r .ended "$H/P3/run/status.json" 2>/dev/null)"
expect hook-then-classify  '^LIMIT:fable:[1-9][0-9]+$'                "$(classify $F/continue.jsonl "$H/P3/run/status.json" a a "$M")"
rm -f "$H/P3/run/status.json"; printf '%s' "$HIN" | env -u P3_SITTING CLAUDE_PROJECT_DIR="$H" sh -c "$HC"; n=$((n+1))
[ -f "$H/P3/run/status.json" ] && { echo "FAIL hook-interactive-noop → wrote status.json outside a sitting"; fail=1; } || echo "ok   hook-interactive-noop → nothing written without P3_SITTING"
rm -rf "$H"
HM="$(jq -r '.StopFailure[0].matcher' "$RUN_DIR/hooks.proposed.json")"
for t in rate_limit usage_limit weekly_limit session_limit_reached; do n=$((n+1))
  printf '%s' "$t" | /usr/bin/grep -qE "^($HM)$" && echo "ok   hook-matcher $t → match" || { echo "FAIL hook-matcher $t → no match"; fail=1; }; done
for t in overloaded server_error authentication_failed; do n=$((n+1))
  printf '%s' "$t" | /usr/bin/grep -qE "^($HM)$" && { echo "FAIL hook-matcher $t → matched"; fail=1; } || echo "ok   hook-matcher $t → no match"; done
echo "---- $n checks, $([ $fail = 0 ] && echo ALL PASS || echo FAILURES)"; exit $fail
