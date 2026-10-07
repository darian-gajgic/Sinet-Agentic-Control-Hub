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
# ---- H-4b evidence-gated evaluation (gate C1a): the verdict gate = the PreToolUse command from hooks.proposed.json, run in
# isolation on the stdin documented for Claude Code 2.1.292 (cwd, tool_name, tool_input), against temp git repos
VG="$(jq -r '.PreToolUse[0].hooks[0].command' "$RUN_DIR/hooks.proposed.json" 2>/dev/null)"
VM="$(jq -r '.PreToolUse[0].matcher' "$RUN_DIR/hooks.proposed.json" 2>/dev/null)"
TT="$(mktemp -d)"; R="$TT/repo"
same() { n=$((n+1)); if [ "$3" = "$2" ]; then echo "ok   $1 → $3"; else echo "FAIL $1 → '$3' (want '$2')"; fail=1; fi; }
newrepo() { git init -q -b main "$1" && git -C "$1" config user.email t@example.invalid && git -C "$1" config user.name t \
  && git -C "$1" config commit.gpgsign false && mkdir -p "$1/P3/reports" && : > "$1/P3/reports/.keep" && printf 'P3/run/log/\n' > "$1/.gitignore"; }
mkev() { mkdir -p "$1/P3/run/log/evidence"; jq -nc --arg b "$2" --arg h "$3" --argjson ok "$4" '{branch:$b,head:$h,ok:$ok,legs:[{name:"test",ok:$ok}]}' \
  > "$1/P3/run/log/evidence/${2//\//_}-$3.json"; } # mkev <main checkout> <branch> <sha> <true|false> — a synthetic evidence file
pw() { jq -nc --arg p "$1" --arg c "$2" --arg d "${3:-$R}" '{session_id:"s",transcript_path:"/t",cwd:$d,permission_mode:"auto",hook_event_name:"PreToolUse",tool_name:"Write",tool_input:{file_path:$p,content:$c},tool_use_id:"t1"}'; }
pe() { jq -nc --arg p "$1" --arg s "$2" --arg d "${3:-$R}" '{session_id:"s",transcript_path:"/t",cwd:$d,permission_mode:"auto",hook_event_name:"PreToolUse",tool_name:"Edit",tool_input:{file_path:$p,old_string:"VERDICT: FAIL",new_string:$s,replace_all:false},tool_use_id:"t2"}'; }
pb() { jq -nc --arg c "$1" --arg d "${2:-$R}" '{session_id:"s",transcript_path:"/t",cwd:$d,permission_mode:"auto",hook_event_name:"PreToolUse",tool_name:"Bash",tool_input:{command:$c,description:"d"},tool_use_id:"t3"}'; }
vgx() { # vgx <name> <want exit> <payload> [stderr regex]  — exit code, empty stdout, and the reason Claude would see
  n=$((n+1)); local out rc err; out=$(printf '%s' "$3" | CLAUDE_PROJECT_DIR="$P3_ROOT" sh -c "$VG" 2>"$TT/vg.err"); rc=$?; err=$(cat "$TT/vg.err")
  if [ "$rc" = "$2" ] && [ -z "$out" ] && { [ -z "${4:-}" ] || printf '%s' "$err" | /usr/bin/grep -qE -- "$4"; }; then echo "ok   $1 → exit $rc"
  else echo "FAIL $1 → exit $rc (want $2), stdout '${out:0:80}', stderr '${err:0:200}'"; fail=1; fi; }
newrepo "$R"; echo a > "$R/a.txt"; git -C "$R" add -A && git -C "$R" commit -qm c1; H1=$(git -C "$R" rev-parse HEAD)
RP="$R/P3/reports/P3-X-1-evaluate.md"
vgx vg-no-evidence-blocked   2 "$(pw "$RP" $'# P3-X-1 evaluation\n\nVERDICT: PASS\n')" "no battery evidence for main@${H1:0:7}"
vgx vg-remedy-names-battery  2 "$(pw "$RP" 'VERDICT: PASS')" "battery\.sh $R "
vgx vg-fail-verdict-allowed  0 "$(pw "$RP" $'VERDICT: FAIL\n\nF1 [HIGH/high] x.go:1 — y')"
vgx vg-other-report-allowed  0 "$(pw "$R/P3/reports/P3-X-1-execute.md" 'VERDICT: PASS')"
vgx vg-other-dir-allowed     0 "$(pw "$R/notes/P3-X-1-evaluate.md" 'VERDICT: PASS')"
vgx vg-suffix-allowed        0 "$(pw "$RP.bak" 'VERDICT: PASS')"
vgx vg-variant-blocked       2 "$(pw "$RP" $'**Verdict:** PASS — nothing above nit')"
mkev "$R" main "$H1" false
vgx vg-red-evidence-blocked  2 "$(pw "$RP" 'VERDICT: PASS')" "evidence for main@${H1:0:7} is red"
mkev "$R" main "$H1" true
vgx vg-green-head-allowed    0 "$(pw "$RP" $'# P3-X-1 evaluation\n\nVERDICT: PASS\n')"
vgx vg-edit-green-allowed    0 "$(pe "$RP" 'VERDICT: PASS')"
echo b > "$R/a.txt"; git -C "$R" commit -qam c2; H2=$(git -C "$R" rev-parse HEAD)
vgx vg-stale-sha-blocked     2 "$(pw "$RP" 'VERDICT: PASS')" "no battery evidence for main@${H2:0:7}"
vgx vg-edit-blocked          2 "$(pe "$RP" $'## Re-check r1\n\nVERDICT: PASS')" "main@${H2:0:7}"
vgx vg-edit-nopass-allowed   0 "$(pe "$RP" 'VERDICT: FAIL (F2 open)')"
vgx vg-bash-heredoc-blocked  2 "$(pb $'cat > P3/reports/P3-X-1-evaluate.md <<\'EOF\'\n# P3-X-1\n\nVERDICT: PASS\nEOF')"
vgx vg-bash-noreport-allowed 0 "$(pb "echo 'VERDICT: PASS'")"
vgx vg-bash-read-allowed     0 "$(pb 'grep -n VERDICT P3/reports/P3-X-1-evaluate.md')"
mkev "$R" main "$H2" true
vgx vg-edit-allowed          0 "$(pe "$RP" $'## Re-check r1\n\nVERDICT: PASS')"
vgx vg-bash-heredoc-allowed  0 "$(pb $'cat > P3/reports/P3-X-1-evaluate.md <<\'EOF\'\n# P3-X-1\n\nVERDICT: PASS\nEOF')"
vgx vg-bash-cd-allowed       0 "$(pb "cd $R && cat >> P3/reports/P3-X-1-evaluate.md <<'EOF'
VERDICT: PASS
EOF" /tmp)"
vgx vg-bash-abs-allowed      0 "$(pb "python3 - <<'PY'
open('$RP','a').write('VERDICT: PASS\n')
PY" /tmp)"
vgx vg-bash-wrongcwd-blocked 2 "$(pb $'cat > P3/reports/P3-X-1-evaluate.md <<\'EOF\'\nVERDICT: PASS\nEOF' "$TT")" 'not inside a git worktree'
W="$TT/wt"; git -C "$R" worktree add -q "$W" -b p3/wt1 2>/dev/null; HW=$(git -C "$W" rev-parse HEAD); WP="$W/P3/reports/P3-X-2-evaluate.md"
mkev "$W" p3/wt1 "$HW" true
vgx vg-wt-own-log-blocked    2 "$(pw "$WP" 'VERDICT: PASS' "$W")" "no battery evidence for p3/wt1@${HW:0:7} \($R/P3/run/log/evidence/p3_wt1-$HW\.json\)"
mkev "$R" p3/wt1 "$HW" true
vgx vg-wt-main-log-allowed   0 "$(pw "$WP" 'VERDICT: PASS' "$W")"
mkdir -p "$TT/nogit/P3/reports"
vgx vg-nogit-blocked         2 "$(pw "$TT/nogit/P3/reports/a-evaluate.md" 'VERDICT: PASS' "$TT")" 'not inside a git worktree'
for t in Write Edit Bash; do n=$((n+1))
  printf '%s' "$t" | /usr/bin/grep -qE "^($VM)$" && echo "ok   vg-matcher $t → match" || { echo "FAIL vg-matcher $t → no match"; fail=1; }; done
for t in Read NotebookEdit WebFetch Agent; do n=$((n+1))
  printf '%s' "$t" | /usr/bin/grep -qE "^($VM)$" && { echo "FAIL vg-matcher $t → matched"; fail=1; } || echo "ok   vg-matcher $t → no match"; done
same vg-timeout '30' "$(jq -r '.PreToolUse[0].hooks[0].timeout' "$RUN_DIR/hooks.proposed.json" 2>/dev/null)"
# ---- H-4b battery.sh on a temp Go module (stub lockgate): legs, evidence location + fields, red legs, web decision, worktrees, the wait
B="$TT/bat"; newrepo "$B"; printf 'module example.com/bat\n\n%s\n' "$(/usr/bin/grep -m1 '^go ' "$P3_ROOT/go.mod")" > "$B/go.mod"; mkdir -p "$B/x" "$B/tools/lockgate"
printf 'package x\n\n// One returns 1.\nfunc One() int { return 1 }\n' > "$B/x/x.go"
printf 'package x\n\nimport "testing"\n\nfunc TestOne(t *testing.T) {\n\tif One() != 1 {\n\t\tt.Fatal("One")\n\t}\n}\n' > "$B/x/x_test.go"
printf 'package main\n\nimport "os"\n\nfunc main() {\n\tif _, err := os.Stat("LOCKGATE_RED"); err == nil {\n\t\tos.Exit(1)\n\t}\n}\n' > "$B/tools/lockgate/main.go"
git -C "$B" add -A && git -C "$B" commit -qm b1; HB=$(git -C "$B" rev-parse HEAD); EB="$B/P3/run/log/evidence/main-$HB.json"
bat() { BAT_OUT=$(P3_BATTERY_POLL=1 "$RUN_DIR/battery.sh" "$1" 2>&1); BAT_RC=$?; }
bat "$B"
same   bat-green-exit      0 "$BAT_RC"
same   bat-green-summary   "battery: GREEN main@${HB:0:7} → $EB" "$(printf '%s\n' "$BAT_OUT" | tail -1)"
same   bat-fields          "main true $HB $B" "$(jq -r '"\(.branch) \(.ok) \(.head) \(.worktree)"' "$EB" 2>/dev/null)"
same   bat-legs            'clean,gofmt,vet,build,test,lockgate,stable' "$(jq -r '.legs|map(.name)|join(",")' "$EB" 2>/dev/null)"
same   bat-test-pkgs       '1 0 0' "$(jq -r '.legs[]|select(.name=="test")|"\(.pkgs_ok) \(.pkgs_fail|length) \(.tests_failed|length)"' "$EB" 2>/dev/null)"
same   bat-skip-recorded   'TestLivePhraseAndSummarize|TestLiveIntakeTriageClassifiesWebshop' "$(jq -r .skip "$EB" 2>/dev/null)"
expect bat-web-skipped     '^skipped: web/src unchanged vs main$' "$(jq -r .web "$EB" 2>/dev/null)"
expect bat-times           '^20[0-9-]+T[0-9:]+Z 20[0-9-]+T[0-9:]+Z [0-9]+ [0-9]+$' "$(jq -r '.times|"\(.started) \(.finished) \(.waited_s) \(.total_s)"' "$EB" 2>/dev/null)"
vgx    bat-admits-pass     0 "$(pw "$B/P3/reports/T-1-evaluate.md" 'VERDICT: PASS' "$B")"
echo draft > "$B/P3/reports/T-1-evaluate.md"; bat "$B"
same   bat-report-excepted 0 "$BAT_RC"
printf 'package x\n\n// One returns one (an uncommitted, gofmt-clean edit).\nfunc One() int { return 1 }\n' > "$B/x/x.go"; bat "$B"
same   bat-dirty-exit      1 "$BAT_RC"
same   bat-dirty-red-legs  'clean' "$(jq -r '[.legs[]|select(.ok|not).name]|join(",")' "$EB" 2>/dev/null)"
expect bat-dirty-names     ' M x/x.go' "$(jq -r '.legs[]|select(.name=="clean").tail' "$EB" 2>/dev/null)"
vgx    bat-dirty-blocks    2 "$(pw "$B/P3/reports/T-1-evaluate.md" 'VERDICT: PASS' "$B")" 'is red'
git -C "$B" checkout -q -- x/x.go; rm -f "$B/P3/reports/T-1-evaluate.md"
: > "$B/LOCKGATE_RED"; printf 'package x\nfunc  Two( ) int { return 2 }\n' > "$B/x/two.go"
printf 'package x\n\nimport "testing"\n\nfunc TestBad(t *testing.T) { t.Fatal("bad") }\n' > "$B/x/bad_test.go"
git -C "$B" add -A && git -C "$B" commit -qm b2; HB2=$(git -C "$B" rev-parse HEAD); EB2="$B/P3/run/log/evidence/main-$HB2.json"; bat "$B"
same   bat-red-exit        1 "$BAT_RC"
same   bat-red-legs        'gofmt,test,lockgate' "$(jq -r '[.legs[]|select(.ok|not).name]|join(",")' "$EB2" 2>/dev/null)"
same   bat-red-pkgs        'example.com/bat/x TestBad' "$(jq -r '.legs[]|select(.name=="test")|"\(.pkgs_fail|join(",")) \(.tests_failed|join(","))"' "$EB2" 2>/dev/null)"
expect bat-red-gofmt-names 'x/two\.go' "$(jq -r '.legs[]|select(.name=="gofmt").tail' "$EB2" 2>/dev/null)"
same   bat-red-summary     "battery: RED main@${HB2:0:7} → $EB2" "$(printf '%s\n' "$BAT_OUT" | tail -1)"
git -C "$B" checkout -q -b webby "$HB"; mkdir -p "$B/web/src"; echo 'export {}' > "$B/web/src/a.ts"
git -C "$B" add -A && git -C "$B" commit -qm w1; HBW=$(git -C "$B" rev-parse HEAD); bat "$B"; EW="$B/P3/run/log/evidence/webby-$HBW.json"
expect bat-web-ran         '^ran: web/src changed vs main$' "$(jq -r .web "$EW" 2>/dev/null)"
same   bat-web-legs        'clean,web-install,web-typecheck,web-test,web-build,gofmt,vet,build,test,lockgate,stable' "$(jq -r '.legs|map(.name)|join(",")' "$EW" 2>/dev/null)"
same   bat-web-red         1 "$BAT_RC"
git -C "$B" checkout -q main
BW="$TT/batwt"; git -C "$B" worktree add -q "$BW" -b p3/slash "$HB" 2>/dev/null; ES="$B/P3/run/log/evidence/p3_slash-$HB.json"; bat "$BW"
same   bat-wt-exit         0 "$BAT_RC"
same   bat-wt-main-log     "p3/slash $HB $BW" "$(jq -r '"\(.branch) \(.head) \(.worktree)"' "$ES" 2>/dev/null)"
same   bat-wt-no-own-log   none "$([ -e "$BW/P3/run/log" ] && echo present || echo none)"
vgx    bat-wt-admits-pass  0 "$(pw "$BW/P3/reports/T-2-evaluate.md" 'VERDICT: PASS' "$BW")"
bash -c 'exec -a "go test fake-foreign-battery" sleep 3' & FP=$!; sleep 0.5; bat "$BW"; wait "$FP" 2>/dev/null
expect bat-waits-foreign   "waiting for a foreign go test \(pid ([0-9]+ )*$FP\b" "$BAT_OUT"
expect bat-waited-secs     '^([2-9]|[1-9][0-9]+)$' "$(jq -r .times.waited_s "$ES" 2>/dev/null)"
AO=$(bash -c 'echo $$ > "$3"; : go test marker-ancestor; P3_BATTERY_POLL=1 "$1" "$2"' _ "$RUN_DIR/battery.sh" "$BW" "$TT/anc.pid" 2>&1); AP=$(cat "$TT/anc.pid" 2>/dev/null)
n=$((n+1)); if [ -n "$AP" ] && printf '%s\n' "$AO" | tail -1 | /usr/bin/grep -q '^battery: GREEN' && ! printf '%s' "$AO" | /usr/bin/grep -qE "pid ([0-9]+ )*$AP\b"
then echo "ok   bat-ignores-own-ancestor → pid $AP not waited for, GREEN"
else echo "FAIL bat-ignores-own-ancestor → ancestor $AP: $(printf '%s' "$AO" | head -2 | tr '\n' ' ') … $(printf '%s\n' "$AO" | tail -1)"; fail=1; fi
same   bat-usage           2 "$("$RUN_DIR/battery.sh" >/dev/null 2>&1; echo $?)"
same   bat-not-a-worktree  2 "$("$RUN_DIR/battery.sh" "$TT/nogit" >/dev/null 2>&1; echo $?)"
git -C "$B" worktree remove --force "$BW" 2>/dev/null; git -C "$R" worktree remove --force "$W" 2>/dev/null; rm -rf "$TT"
echo "---- $n checks, $([ $fail = 0 ] && echo ALL PASS || echo FAILURES)"; exit $fail
