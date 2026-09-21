#!/usr/bin/env bash
# One budgeted headless coordinator sitting (H-1). Usage: P3/run/sitting.sh [--model <id>] [--smoke]
#   --smoke   one-turn auth/flag/model check ("reply OK"), no repo work; proves the launch line end to end.
# Writes P3/run/log/sitting-<ts>.jsonl (stream-json) + .err + .meta; the sitting itself writes P3/run/status.json last.
set -uo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cd "$P3_ROOT"
MODEL="$P3_MODEL_PRIMARY"; SMOKE=0
while [ $# -gt 0 ]; do case "$1" in --model) MODEL="$2"; shift 2;; --smoke) SMOKE=1; shift;; *) echo "unknown arg $1" >&2; exit 2;; esac; done
# harness self-integrity check (research §2.11: control files have been deleted by agents in other harnesses)
for f in "$RUN_DIR/lib.sh" "$RUN_DIR/loop.sh" "$RUN_DIR/sitting-prompt.md" "$P3_ROOT/P3/STATE.md" "$P3_ROOT/P3/HANDOFF.md" "$P3_ROOT/.claude/skills/p3-implementation/SKILL.md"; do
  [ -s "$f" ] || { echo "PREFLIGHT FAIL: missing $f" >&2; exit 3; }; done
for c in claude jq git timeout; do command -v "$c" >/dev/null || { echo "PREFLIGHT FAIL: no $c on PATH" >&2; exit 3; }; done
git -C "$P3_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1 || { echo "PREFLIGHT FAIL: not a git work tree" >&2; exit 3; }
mkdir -p "$LOG_DIR"; rm -f "$STATUS_FILE"
TS="$(date -u +%Y%m%d-%H%M%S)"; LOGF="$LOG_DIR/sitting-$TS.jsonl"; ERRF="$LOG_DIR/sitting-$TS.err"; META="$LOG_DIR/sitting-$TS.meta"
START="$(date -u +%FT%TZ)"
WALL_S="$(wall_seconds "$P3_SITTING_WALL")"
HARD="$(date -u -d "@$(( $(date +%s) + WALL_S ))" +%FT%TZ)"
if [ "$SMOKE" = 1 ]; then
  PROMPT="Reply with exactly the single word OK and nothing else. Do not read files or run tools."
  WALL="5m"; TURNS=1; SP=(); HARD="$(date -u -d "@$(( $(date +%s) + 300 ))" +%FT%TZ)"
else
  GITCTX="$(git -C "$P3_ROOT" log --oneline -10 2>/dev/null | cut -c1-110)"; GITST="$(git -C "$P3_ROOT" status --short 2>/dev/null | head -20)"
  PROMPT="continue implementation — headless sitting. sitting_start=$START hard_stop=$HARD model=$MODEL. Obey the sitting contract in your system prompt (budget, gate files, status.json last).
Repository truth at launch (distrust any handoff sentence that disagrees with it):
git log --oneline -10:
$GITCTX
git status --short (head -20):
${GITST:-(clean)}"
  WALL="$P3_SITTING_WALL"; TURNS="$P3_SITTING_MAX_TURNS"; SP=(--append-system-prompt-file "$RUN_DIR/sitting-prompt.md")
fi
printf 'model=%s\nstart=%s\nhard_stop=%s\nlog=%s\nsmoke=%s\n' "$MODEL" "$START" "$HARD" "$LOGF" "$SMOKE" > "$META"
log "SITTING $TS start model=$MODEL wall=$WALL turns=$TURNS smoke=$SMOKE"
BUDGET=(); [ -n "$P3_SITTING_MAX_BUDGET_USD" ] && BUDGET=(--max-budget-usd "$P3_SITTING_MAX_BUDGET_USD")
env -u CLAUDECODE TERM=dumb P3_SITTING=1 CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=0 \
timeout --signal=INT --kill-after=15m "$WALL" \
claude -p "$PROMPT" \
  --model "$MODEL" --effort max --max-turns "$TURNS" \
  --permission-mode auto --permission-prompts none \
  --name "p3-sitting-$TS" "${BUDGET[@]}" \
  "${SP[@]}" \
  --output-format stream-json --verbose \
  > "$LOGF" 2> "$ERRF"
EXIT=$?
printf 'exit=%s\nend=%s\n' "$EXIT" "$(date -u +%FT%TZ)" >> "$META"
RJ="$(last_result_json "$LOGF")"
log "SITTING $TS end exit=$EXIT (informational only) subtype=$(printf '%s' "$RJ" | jq -r '.subtype // "-"' 2>/dev/null) turns=$(printf '%s' "$RJ" | jq -r '.num_turns // "-"' 2>/dev/null) cost=$(printf '%s' "$RJ" | jq -r '.total_cost_usd // "-"' 2>/dev/null) log=$LOGF"
echo "$LOGF"
