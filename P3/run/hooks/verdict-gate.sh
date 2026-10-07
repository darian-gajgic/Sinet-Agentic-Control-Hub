#!/usr/bin/env bash
# PreToolUse hook, matcher Write|Edit|Bash (H-4b, gate C1a): a PASS verdict written to P3/reports/*-evaluate.md needs green
# battery evidence for the HEAD of the worktree the report lives in — <main checkout>/P3/run/log/evidence/<branch>-<sha>.json
# with ok:true, written by P3/run/battery.sh. Contract (Claude Code 2.1.292 hooks reference): stdin is the PreToolUse JSON
# (cwd, tool_name, tool_input; Write/Edit file_path is absolute); exit 0 with empty stdout = no objection; exit 2 blocks the
# call and Claude reads stderr as the reason. Bash is matched because auto mode steers agents to write files through Bash.
set -u
IN=$(cat)
case $IN in *-evaluate.md*) ;; *) exit 0 ;; esac # fast path: almost no tool call names an evaluate report
PASS_RE='VERDICT[*_` ]*:[*_` ]*PASS'              # case-insensitive, so **Verdict:** PASS counts too
REPORT_RE='(^|/)P3/reports/.*-evaluate\.md$'
PATH_RE="[^[:space:]'\"=<>|;&()]*P3/reports/[^[:space:]'\"<>|;&()]*-evaluate\\.md"
CD_RE="^[[:space:]]*cd[[:space:]]+(\"[^\"]+\"|'[^']+'|[^[:space:];&|]+)[[:space:]]*(&&|;)"
BATTERY="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/battery.sh"

block() { echo "VERDICT GATE: $*" >&2; exit 2; }

has_pass() { local r; shopt -s nocasematch; [[ $1 =~ $PASS_RE ]]; r=$?; shopt -u nocasematch; return $r; }

require_evidence() { # require_evidence <absolute report path> — returns when the report's worktree HEAD has green evidence
  local d top sha br main ev why
  d=$(dirname "$1"); while [ ! -d "$d" ]; do d=$(dirname "$d"); done
  top=$(git -C "$d" rev-parse --show-toplevel 2>/dev/null) || block "$1 is not inside a git worktree, so no battery evidence can back a PASS verdict there."
  sha=$(git -C "$top" rev-parse HEAD 2>/dev/null) || block "cannot read the HEAD of $top."
  br=$(git -C "$top" symbolic-ref --short -q HEAD || echo detached)
  main=$(dirname "$(git -C "$top" rev-parse --path-format=absolute --git-common-dir)")
  ev="$main/P3/run/log/evidence/${br//\//_}-$sha.json" # battery.sh writes the same name
  jq -e --arg h "$sha" '.ok == true and .head == $h' "$ev" >/dev/null 2>&1 && return 0
  if [ -f "$ev" ]; then why="the battery evidence for $br@${sha:0:7} is red"; else why="no battery evidence for $br@${sha:0:7}"; fi
  block "$why ($ev). A PASS verdict in an evaluate report needs a green battery on that worktree's HEAD: run  $BATTERY $top  in the foreground, then write the verdict again; every new commit needs a new run. A FAIL verdict needs no evidence; a command that only reads the report can search for VERDICT alone."
}

case $(jq -r '.tool_name // empty' <<<"$IN" 2>/dev/null) in
  Write|Edit)
    P=$(jq -r '.tool_input.file_path // empty' <<<"$IN")
    [[ $P =~ $REPORT_RE ]] || exit 0
    has_pass "$(jq -r '.tool_input.content // .tool_input.new_string // empty' <<<"$IN")" || exit 0
    require_evidence "$P" ;;
  Bash)
    CMD=$(jq -r '.tool_input.command // empty' <<<"$IN")
    has_pass "$CMD" || exit 0
    mapfile -t PATHS < <(grep -oE -- "$PATH_RE" <<<"$CMD" | sort -u)
    [ ${#PATHS[@]} -gt 0 ] || exit 0
    BASE=$(jq -r '.cwd // empty' <<<"$IN")
    if [[ $CMD =~ $CD_RE ]]; then # a leading `cd <dir> &&` moves the base of relative paths
      D=${BASH_REMATCH[1]}; D=${D#\"}; D=${D%\"}; D=${D#\'}; D=${D%\'}; [[ $D == "~"* ]] && D=$HOME${D:1}
      case $D in /*) BASE=$D ;; *) BASE=$BASE/$D ;; esac
    fi
    for P in "${PATHS[@]}"; do case $P in /*) ;; *) P=$BASE/$P ;; esac; require_evidence "$P"; done ;;
esac
exit 0
