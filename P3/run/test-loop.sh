#!/usr/bin/env bash
# Breaker tests for loop.sh with a stub `claude` (no API calls). Research §2.10 lesson 10: assert the counters actually move.
# H-3: also the LIMIT archive, the update guard (sitting.sh preflight) and the measured cap, all against temp state files.
# H-4a: the main guard (a stub `gh`; default: the latest CI run on main succeeded) and the landing tags. The loop tags and
# pushes, so every loop here runs in a temp repo with a temp bare origin (P3_ROOT points there; the harness files are copied
# in): the real repo, its origin and its runtime files (status.json, STOP, RESUME, log/, loop.lock) are never touched.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; T="$(mktemp -d)"; R="$T/repo"; O="$T/origin.git"
mkdir -p "$R/P3/run" "$R/.claude/skills/p3-implementation"
cp "$HERE"/{lib,loop,sitting}.sh "$HERE"/sitting-prompt.md "$HERE"/cli-version.pinned "$HERE"/cap "$R/P3/run/"; cp -r "$HERE/fixtures" "$R/P3/run/"
for f in P3/STATE.md P3/HANDOFF.md .claude/skills/p3-implementation/SKILL.md; do echo "stub $f" > "$R/$f"; done
git init -q -b main "$R"; git -C "$R" config user.name p3-test; git -C "$R" config user.email p3-test@localhost
git -C "$R" add -A; git -C "$R" commit -qm "temp repo for test-loop.sh"
git init -q --bare -b main "$O"; git -C "$R" remote add origin "$O"; git -C "$R" push -q origin main
export P3_ROOT="$R"
source "$P3_ROOT/P3/run/lib.sh"; cd "$P3_ROOT"
F="$RUN_DIR/fixtures"; fail=0; n=0
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
work() { echo "$RANDOM" >> "$P3_ROOT/$1"; git -C "$P3_ROOT" add "$1" && git -C "$P3_ROOT" commit -qm "$2"; }   # one commit in the temp repo
signed() { cat "$STUB_FIX/continue.jsonl"; echo "{\"outcome\":\"CONTINUE\",\"landed\":$1,\"next\":\"x\",\"gate\":null,\"family\":null,\"note\":\"\"}" > "$STUB_STATUS"; }
case "$next" in
  landcommit)  work work.txt "P3-X-1: work"; signed '["P3-X-1","P3-X-2"]';;
  bookkeeping) work P3/STATE.md "P3: STATE"; signed '[]';;
  progress)    work work.txt "P3-X-3: work"; signed '[]';;
  crashwork)   work work.txt "P3-X-4: half done"; cat "$STUB_FIX/crash-noresult.jsonl";;
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
mkdir -p "$T/ghstub"; cat > "$T/ghstub/gh" <<'STUB'
#!/usr/bin/env bash
# `gh run list …` stub: appends its argv to $STUB_GH_CALLS, answers with the next state from $STUB_GH_SEQ_FILE (the last
# state repeats; no file = green). green|red|red2|running|queued → one run; none → no runs; error → an auth failure;
# resume|stop → touch RESUME|STOP, then answer red (the same run as "red").
printf '%s\n' "$*" >> "$STUB_GH_CALLS"
seq="$(cat "$STUB_GH_SEQ_FILE" 2>/dev/null)"; next="${seq%% *}"; rest="${seq#* }"
[ "$rest" = "$seq" ] || printf '%s' "$rest" > "$STUB_GH_SEQ_FILE"
run() { printf '[{"conclusion":"%s","headSha":"5ca1ab1e00000000000000000000000000000000","status":"%s","url":"https://github.com/o/r/actions/runs/%s"}]\n' "$1" "$2" "$3"; }
case "${next:-green}" in
  green)   run success completed 1001;;
  red)     run failure completed 1002;;
  red2)    run timed_out completed 1003;;
  running) run "" in_progress 1004;;
  queued)  run "" queued 1005;;
  none)    echo '[]';;
  error)   echo 'HTTP 401: Bad credentials (https://api.github.com/graphql)' >&2; exit 1;;
  resume)  touch "$STUB_RESUME"; run failure completed 1002;;
  stop)    touch "$STUB_STOP"; run failure completed 1002;;
esac
STUB
chmod +x "$T/ghstub/gh"
nogh_path() { # nogh_path — $PATH with each directory that holds a gh swapped for a symlink copy without it ("gh absent")
  local d out="" i=0 dirs; IFS=: read -ra dirs <<< "$PATH"
  for d in "${dirs[@]}"; do
    if [ -e "$d/gh" ]; then i=$((i+1)); mkdir -p "$T/nogh$i"; ln -s "$d"/* "$T/nogh$i/" 2>/dev/null; rm -f "$T/nogh$i/gh"; d="$T/nogh$i"; fi
    out="${out:+$out:}$d"
  done; printf '%s' "$out"
}
NOGH="$(nogh_path)"
PIN0="$(cat "$P3_CLI_PIN")"
reset_state() { # fresh temp cap/ledger/pin/observed for each test (the real P3/run files are never touched)
  echo 3 > "$T/cap"; rm -rf "$T/sittings.tsv" "$T/compactions.log" "$T/observed" "$T/prompts" "$T/env"
  printf '%s\n' "$PIN0" > "$T/pin"; rm -f "$STOP_FILE" "$STATUS_FILE"
}
stub_env() { # exported into the subshell that runs loop.sh / sitting.sh
  export PATH="$T:$T/ghstub:$PATH" STUB_FIX="$F" STUB_STATUS="$STATUS_FILE" STUB_STOP="$STOP_FILE" STUB_SEQ_FILE="$T/seq"
  export STUB_PROMPTS="$T/prompts" STUB_ENV_OUT="$T/env"
  export STUB_RESUME="$RESUME_FILE" STUB_GH_SEQ_FILE="$T/ghseq" STUB_GH_CALLS="$T/ghcalls" P3_CI_POLL=1 P3_WAIT_POLL=1
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

# ---- H-4a: the main guard (stub gh). Every loop above already ran under the default stub (green).
[ "$(git -C "$P3_ROOT" remote get-url origin)" = "$O" ] || { echo "FAIL setup: origin of $P3_ROOT is not the temp bare repo — refusing to run the tag tests"; exit 1; }
ci_run() { # ci_run <name> <gh states> <claude sequence> <expected exit> [--once] [VAR=value ...]
  # one loop run under the stubs with a fresh loop log, gh call log and prompt log; VAR=value pairs are exported after stub_env
  local name="$1" want="$4" a args=() envs=(); printf '%s' "$2" > "$T/ghseq"; printf '%s' "$3" > "$T/seq"; shift 4
  for a in "$@"; do case "$a" in *=*) envs+=("$a");; *) args+=("$a");; esac; done
  mkdir -p "$LOG_DIR"; : > "$LOOP_LOG"; : > "$T/ghcalls"; rm -f "$T/prompts" "$STOP_FILE" "$STATUS_FILE" "$RESUME_FILE"
  ( stub_env; [ ${#envs[@]} -eq 0 ] || export "${envs[@]}"; timeout 120 "$RUN_DIR/loop.sh" "${args[@]}" >/dev/null 2>&1 ); local rc=$?
  rm -f "$STOP_FILE" "$STATUS_FILE" "$RESUME_FILE"
  if [ "$rc" = "$want" ]; then ok "$name → exit $rc"; else bad "$name → exit $rc (want $want)"; fi
}
calls() { /usr/bin/grep -c . "$T/ghcalls"; }                                            # gh invocations in the last ci_run
sat() { cat "$T/prompts" 2>/dev/null | /usr/bin/grep -c '^continue implementation'; }    # sittings launched in the last ci_run
logn() { /usr/bin/grep -cE -- "$1" "$LOOP_LOG"; }                                       # loop-log lines matching <ERE>
trail() { # the guard's readings and decisions and the sitting starts of the last ci_run, in order (e.g. failure>success>SITTING)
  /usr/bin/grep -oE 'CI on main: [a-z_]+|RESUME touched \(CI on main\)|STOP file seen while waiting \(CI on main\)|SITTING [0-9]{8}-[0-9]{6} start' "$LOOP_LOG" \
    | sed -E 's/^CI on main: //; s/^RESUME.*/RESUME/; s/^STOP.*/STOP/; s/^SITTING.*/SITTING/' | paste -sd'>'
}
ci_run ci-green          "green"                "stop"          0 --once
check ci-gh-command      '^run list --branch main --limit 1 --json status,conclusion,headSha,url$' "$(head -n 1 "$T/ghcalls")"
check ci-green-trail     '^success>SITTING$'                                "$(trail)"
check ci-green-sha       '^CI on main: success 5ca1ab1e0{32}$'               "$(/usr/bin/grep -oE 'CI on main: success [0-9a-f]+' "$LOOP_LOG")"
check ci-defaults        '^1 120 600$'                                       "$( ( unset P3_CI_GUARD P3_CI_POLL P3_WAIT_POLL; source "$RUN_DIR/lib.sh"; echo "$P3_CI_GUARD $P3_CI_POLL $P3_WAIT_POLL" ) 2>&1 )"
ci_run ci-red-green      "red red green"        "stop"          0 --once P3_CI_POLL=7
check ci-red-trail       '^failure>failure>success>SITTING$'                "$(trail)"
check ci-red-calls       '^3$'                                               "$(calls)"
check ci-red-notify-once '^1$'                                               "$(logn 'NOTIFY: P3 loop waiting: CI on main failure')"
check ci-red-notify-text 'NOTIFY: P3 loop waiting: CI on main failure — https://github.com/o/r/actions/runs/1002$' "$(/usr/bin/grep 'NOTIFY: P3 loop waiting' "$LOOP_LOG")"
check ci-red-poll        '^2$'                                               "$(logn 'CI on main: failure .* re-check in 1s')"
check ci-red-ran         '^1$'                                               "$(sat)"
ci_run ci-red-resume     "red resume"           "stop"          0 --once
check ci-resume-trail    '^failure>failure>RESUME>SITTING$'                 "$(trail)"
check ci-resume-notify   '^1$'                                               "$(logn 'NOTIFY: P3 loop waiting')"
check ci-resume-ran      '^1$'                                               "$(sat)"
ci_run ci-red-stop       "red stop"             "stop"          0 --once
check ci-stop-trail      '^failure>failure>STOP$'                           "$(trail)"
check ci-stop-nosit      '^0$'                                               "$(sat)"
ci_run ci-running        "queued running green" "stop"          0 --once P3_WAIT_POLL=7
check ci-running-trail   '^queued>in_progress>success>SITTING$'             "$(trail)"
check ci-running-poll    '^2$'                                               "$(logn 'CI on main: (queued|in_progress) .* re-check in 1s')"
check ci-running-quiet   '^0$'                                               "$(logn 'NOTIFY:')"
check ci-running-ran     '^1$'                                               "$(sat)"
ci_run ci-gh-absent      "red"                  "stop"          0 --once "PATH=$T:$NOGH"
check ci-absent-log      'CI guard: no gh on PATH — proceeding$'             "$(/usr/bin/grep 'CI guard' "$LOOP_LOG")"
check ci-absent-calls    '^0$'                                               "$(calls)"
check ci-absent-ran      '^1$'                                               "$(sat)"
ci_run ci-gh-error       "error"                "stop"          0 --once
check ci-error-log       'CI guard: gh run list failed \(exit 1\): HTTP 401: Bad credentials .* — proceeding$' "$(/usr/bin/grep 'CI guard' "$LOOP_LOG")"
check ci-error-ran       '^1$'                                               "$(sat)"
ci_run ci-no-runs        "none"                 "stop"          0 --once
check ci-none-log        'CI guard: no CI runs on main — proceeding$'        "$(/usr/bin/grep 'CI guard' "$LOOP_LOG")"
check ci-none-ran        '^1$'                                               "$(sat)"
ci_run ci-guard-off      "red"                  "stop"          0 --once P3_CI_GUARD=0
check ci-off-calls       '^0$'                                               "$(calls)"
check ci-off-start       'LOOP start .* ci_guard=0$'                         "$(/usr/bin/grep 'LOOP start' "$LOOP_LOG")"
check ci-off-ran         '^1$'                                               "$(sat)"
ci_run ci-every-sitting  "green"                "continue stop" 0
check ci-every-trail     '^success>SITTING>success>SITTING$'                "$(trail)"
ci_run ci-notify-per-run "resume red green"     "continue stop" 0
check ci-perrun-trail    '^failure>RESUME>SITTING>failure>success>SITTING$' "$(trail)"
check ci-perrun-once     '^1$'                                               "$(logn 'NOTIFY: P3 loop waiting')"
ci_run ci-notify-new-run "resume red2 green"    "continue stop" 0
check ci-newrun-twice    '^2$'                                               "$(logn 'NOTIFY: P3 loop waiting')"
check ci-newrun-text     'NOTIFY: P3 loop waiting: CI on main timed_out — https://github.com/o/r/actions/runs/1003$' "$(/usr/bin/grep 'NOTIFY: P3 loop waiting' "$LOOP_LOG" | tail -n 1)"

# ---- H-4a: landing tags, in the temp repo with its temp bare origin. The tag name has second resolution (as real sittings,
# hours apart, do) and a --once stub sitting takes milliseconds, so each case below starts its sitting ≥1 s after the last one.
tags() { git -C "$P3_ROOT" tag -l 'sitting/*' | LC_ALL=C sort; }
fresh() { LC_ALL=C comm -13 <(printf '%s\n' "$1") <(tags) | /usr/bin/grep .; }          # fresh <tags before>  — tags created since
want_tag() { /usr/bin/grep -oE 'SITTING [0-9]{8}-[0-9]{6} start' "$LOOP_LOG" | tail -n 1 | sed -E 's#SITTING (.*) start#sitting/\1#'; }
msg() { git -C "$P3_ROOT" tag -l --format='%(contents)' "$1" | sed '/^$/d'; }
peel() { git -C "$1" rev-parse -q --verify "$2" 2>/dev/null || echo missing; }
pushed() { local a b; a="$(peel "$P3_ROOT" "refs/tags/$1")"; b="$(peel "$O" "refs/tags/$1")"
  [ "$a" = "$b" ] && [ "$a" != missing ] && echo pushed || echo "not pushed (local $a, origin $b)"; }
sleep 1; TB="$(tags)"; HB="$(git -C "$P3_ROOT" rev-parse HEAD)"
ci_run tag-landing       "green"                "landcommit"    0 --once
TN="$(fresh "$TB")"
check tag-landing-one    '^1$'                                               "$(printf '%s' "$TN" | /usr/bin/grep -c .)"
check tag-landing-ts     '^sitting/[0-9]{8}-[0-9]{6}$'                       "$TN"
check tag-landing-name   "^$(want_tag)$"                                     "$TN"
check tag-landing-annot  '^tag$'                                             "$(git -C "$P3_ROOT" cat-file -t "$TN" 2>&1)"
check tag-landing-head   "^$(git -C "$P3_ROOT" rev-parse HEAD)$"             "$(peel "$P3_ROOT" "$TN^{commit}")"
check tag-landing-moved  '^moved$'                                           "$([ "$(git -C "$P3_ROOT" rev-parse HEAD)" != "$HB" ] && echo moved || echo same)"
check tag-landing-msg    '^P3-X-1, P3-X-2$'                                  "$(msg "$TN")"
check tag-landing-pushed '^pushed$'                                          "$(pushed "$TN")"
check tag-landing-log    "TAG $TN → [0-9a-f]{7} pushed"                      "$(/usr/bin/grep 'TAG ' "$LOOP_LOG")"
sleep 1; TB="$(tags)"; OB="$(git -C "$O" tag -l 'sitting/*')"
ci_run tag-bookkeeping   "green"                "bookkeeping"   0 --once
check tag-bookkeeping-commit '^P3: STATE$'                                   "$(git -C "$P3_ROOT" log -1 --format=%s)"
check tag-bookkeeping-none   '^0$'                                           "$(fresh "$TB" | /usr/bin/grep -c .)"
check tag-bookkeeping-origin '^same$'                                        "$([ "$(git -C "$O" tag -l 'sitting/*')" = "$OB" ] && echo same || echo changed)"
sleep 1; TB="$(tags)"
ci_run tag-progress      "green"                "progress"      0 --once
TN="$(fresh "$TB")"
check tag-progress-one   '^1$'                                               "$(printf '%s' "$TN" | /usr/bin/grep -c .)"
check tag-progress-msg   '^no landed list; commits beyond bookkeeping [0-9a-f]{7}\.\.[0-9a-f]{7}$' "$(msg "$TN")"
check tag-progress-pushed '^pushed$'                                         "$(pushed "$TN")"
sleep 1; TB="$(tags)"
ci_run tag-landed-only   "green"                "continue"      0 --once
TN="$(fresh "$TB")"
check tag-landed-only-msg  '^P3-TQ-8$'                                       "$(msg "$TN")"
check tag-landed-only-head "^$(git -C "$P3_ROOT" rev-parse HEAD)$"           "$(peel "$P3_ROOT" "$TN^{commit}")"
sleep 1; TB="$(tags)"
ci_run tag-crash         "green"                "crashwork"     1 --once
TN="$(fresh "$TB")"
check tag-crash-class    '^CLASS CRASH:'                                     "$(/usr/bin/grep -oE 'CLASS CRASH:.*' "$LOOP_LOG" | head -n 1)"
check tag-crash-pushed   '^pushed$'                                          "$(pushed "$TN")"
sleep 1; TB="$(tags)"; git -C "$P3_ROOT" remote set-url origin "$T/missing.git"
ci_run tag-push-fails    "green"                "landcommit"    0 --once
git -C "$P3_ROOT" remote set-url origin "$O"; TN="$(fresh "$TB")"
check tag-pushfail-local  '^1$'                                              "$(printf '%s' "$TN" | /usr/bin/grep -c .)"
check tag-pushfail-log    "TAG $TN created, push failed"                     "$(/usr/bin/grep 'TAG ' "$LOOP_LOG")"
check tag-pushfail-origin '^absent$'                                         "$([ "$(peel "$O" "refs/tags/$TN")" = missing ] && echo absent || echo present)"

rm -f "$STOP_FILE" "$STATUS_FILE"; rm -rf "$T"; echo "---- $n checks, $([ $fail = 0 ] && echo ALL PASS || echo FAILURES)"; exit $fail
