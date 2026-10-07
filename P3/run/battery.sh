#!/usr/bin/env bash
# P3/run/battery.sh <worktree> — the CI legs on one worktree's HEAD, serial, as machine-readable evidence (H-4b, gate C1a).
# Writes <main checkout>/P3/run/log/evidence/<branch>-<sha>.json (main checkout = parent of the git common dir) and exits
# 0 iff every leg is green. The verdict gate (hooks/verdict-gate.sh) admits a PASS verdict only against a green file for
# the HEAD of the report's worktree. One battery at a time on the host: any other running `go test` is waited out first.
set -uo pipefail
: "${P3_BATTERY_SKIP:=TestLivePhraseAndSummarize|TestLiveIntakeTriageClassifiesWebshop}" # the two live GPU tests
: "${P3_BATTERY_POLL:=30}"                                                                # re-check while another go test runs

[ $# -eq 1 ] || { echo "usage: P3/run/battery.sh <worktree>" >&2; exit 2; }
WT=$(git -C "$1" rev-parse --show-toplevel 2>/dev/null) || { echo "battery: not inside a git worktree: $1" >&2; exit 2; }
HEAD_SHA=$(git -C "$WT" rev-parse HEAD) || exit 2
BRANCH=$(git -C "$WT" symbolic-ref --short -q HEAD || echo detached)
MAIN=$(dirname "$(git -C "$WT" rev-parse --path-format=absolute --git-common-dir)")
EVFILE="$MAIN/P3/run/log/evidence/${BRANCH//\//_}-$HEAD_SHA.json" # the verdict gate derives the same name
LOGS=$(mktemp -d /tmp/p3-battery.XXXXXX); LEGS="$LOGS/legs.jsonl"; : > "$LEGS"

foreign_go_tests() { # PIDs running `go test`, minus this script's ancestors (a caller's own shell may name go test too)
  local mine=" " p=$$
  while [ "${p:-0}" -gt 1 ]; do mine+="$p "; p=$(ps -o ppid= -p "$p" 2>/dev/null | tr -d ' '); done
  for p in $(pgrep -f '[g]o test '); do [[ $mine == *" $p "* ]] || echo "$p"; done
}

leg() { # leg <name> <command...> — run in the worktree root, full output to $LOGS/<name>.log, one JSON record per leg
  local name=$1 t rc secs; shift; t=$(date +%s)
  ( cd "$WT" && "$@" ) > "$LOGS/$name.log" 2>&1; rc=$?; secs=$(( $(date +%s) - t ))
  jq -nc --arg n "$name" --argjson rc "$rc" --argjson s "$secs" --arg tail "$(tail -n 20 "$LOGS/$name.log")" \
    '{name:$n, ok:($rc == 0), rc:$rc, secs:$s} + (if $rc == 0 then {} else {tail:$tail} end)' >> "$LEGS"
  printf '%-4s %s %ss\n' "$([ "$rc" = 0 ] && echo ok || echo FAIL)" "$name" "$secs"
  [ "$rc" = 0 ] || tail -n 20 "$LOGS/$name.log" | sed 's/^/     /'
}

merge_last() { # merge_last <json object> — add fields to the newest leg record
  { head -n -1 "$LEGS"; tail -n 1 "$LEGS" | jq -c --argjson x "$1" '. + $x'; } > "$LEGS.tmp" && mv "$LEGS.tmp" "$LEGS"
}

clean_tree() { # the tree is HEAD: no tracked change, no untracked file — P3/reports/ excepted (reports are drafted there)
  local s; s=$(git status --porcelain=v1 --untracked-files=all -- . ':(exclude)P3/reports')
  [ -z "$s" ] || { echo "the tree differs from HEAD (P3/reports/ excepted):"; echo "$s"; return 1; }
}

gofmt_tracked() { # CI runs `gofmt -l .` on a fresh checkout, i.e. over the tracked Go files
  local s; s=$(git ls-files -z -- '*.go' | xargs -0 -r gofmt -l) || return 1
  [ -z "$s" ] || { echo "gofmt required for:"; echo "$s"; return 1; }
}

in_web() { cd web && "$@"; }

test_summary() { # test_summary <go test log> — package counts and failing names
  jq -nc --argjson ok "$(grep -cE '^ok[[:space:]]' "$1")" --argjson notest "$(grep -cE '^\?[[:space:]]' "$1")" \
    --arg fail "$(grep -E '^FAIL[[:space:]]+[^[:space:]]' "$1" | awk '{print $2}' | sort -u)" \
    --arg tests "$(grep -oE -- '--- FAIL: [^[:space:]]+' "$1" | awk '{print $3}' | sort -u)" \
    '{pkgs_ok:$ok, pkgs_notest:$notest, pkgs_fail:($fail | split("\n") | map(select(. != ""))),
      tests_failed:($tests | split("\n") | map(select(. != "")))}'
}

head_unchanged() { local now; now=$(git rev-parse HEAD); [ "$now" = "$HEAD_SHA" ] || { echo "HEAD moved during the run: $HEAD_SHA → $now"; return 1; }; }

T0=$(date +%s); STARTED=$(date -u +%FT%TZ)
while f=$(foreign_go_tests); [ -n "$f" ]; do
  echo "battery: waiting for a foreign go test (pid $(echo $f)) — re-check in ${P3_BATTERY_POLL}s"
  ps -o pid=,etime=,args= -p "$(echo $f | tr ' ' ',')" 2>/dev/null | cut -c1-160 | sed 's/^/     /'
  sleep "$P3_BATTERY_POLL"
done
WAITED=$(( $(date +%s) - T0 ))

if git -C "$WT" rev-parse -q --verify refs/heads/main >/dev/null; then
  if git -C "$WT" diff --quiet main...HEAD -- web/src; then WEB="skipped: web/src unchanged vs main"; else WEB="ran: web/src changed vs main"; fi
else WEB="ran: no main branch to compare with"; fi
echo "battery: $BRANCH@${HEAD_SHA:0:7} in $WT — web legs $WEB; logs $LOGS"

leg clean clean_tree
if [[ $WEB == ran* ]]; then # CI order: the SPA first, so the Go legs build over the real embedded assets
  leg web-install   in_web npm ci --ignore-scripts
  leg web-typecheck in_web npm run typecheck
  leg web-test      in_web npm run test
  leg web-build     in_web npm run build
fi
leg gofmt    gofmt_tracked
leg vet      go vet ./...
leg build    go build ./...
leg test     go test -p 1 -count=1 -skip "$P3_BATTERY_SKIP" ./...
merge_last "$(test_summary "$LOGS/test.log")"
leg lockgate go run ./tools/lockgate
leg stable   head_unchanged

mkdir -p "${EVFILE%/*}"
jq -n --arg branch "$BRANCH" --arg head "$HEAD_SHA" --arg wt "$WT" --arg web "$WEB" --arg skip "$P3_BATTERY_SKIP" \
  --arg logs "$LOGS" --arg started "$STARTED" --arg finished "$(date -u +%FT%TZ)" \
  --argjson waited "$WAITED" --argjson total "$(( $(date +%s) - T0 ))" --slurpfile legs "$LEGS" \
  '{branch:$branch, head:$head, ok:(($legs | length) > 0 and ($legs | all(.ok))), worktree:$wt, web:$web, skip:$skip,
    legs:$legs, logs:$logs, times:{started:$started, finished:$finished, waited_s:$waited, total_s:$total}}' \
  > "$EVFILE.tmp.$$" && mv -f "$EVFILE.tmp.$$" "$EVFILE" || { echo "battery: cannot write $EVFILE" >&2; exit 1; }
OK=$(jq -r .ok "$EVFILE")
echo "battery: $([ "$OK" = true ] && echo GREEN || echo RED) $BRANCH@${HEAD_SHA:0:7} → $EVFILE"
[ "$OK" = true ]
