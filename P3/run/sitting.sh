#!/usr/bin/env bash
# One budgeted headless coordinator sitting (H-1). Usage: P3/run/sitting.sh [--model <id>] [--smoke]
#   --smoke   one-turn auth/flag/model check ("reply OK"), no repo work; proves the launch line end to end.
# Writes P3/run/log/sitting-<ts>.jsonl (stream-json) + .err + .meta; the sitting itself writes P3/run/status.json last.
set -uo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
cd "$P3_ROOT"
export DISABLE_AUTOUPDATER=1   # H-3b: the CLI must not update itself under a running sitting
# H-5: loop.sh starts this script as its own process group with default SIGINT and signals that group on Ctrl-C. timeout(1) puts
# itself and claude into a process group of their own (pgid = timeout's pid), out of that signal's reach, so it is forwarded here.
# Measured on this host's timeout (uutils coreutils 0.8.0), 2026-10-08:
#   INT  → the timeout process only: it passes the signal once to claude and claude's group (a signal to the whole group would
#          reach claude twice, directly and again from timeout); claude ends its turn, the documented clean stop.
#   TERM → timeout's whole group: once timeout has passed on one signal it passes on no other until --kill-after's SIGKILL, so the
#          escalation must reach claude and its children directly.
# A signal before the launch ends this script; no sitting starts. Logged to loop.log only: stdout's last line is the transcript path.
TPID=""; FWD=""
fwd() { # fwd <INT|TERM>
  FWD=1
  [ -n "$TPID" ] || TPID="${!:-}"   # a signal between the fork and TPID=$! (this script starts no other background job)
  [ -n "$TPID" ] || { log "sitting.sh: SIG$1 before launch — no sitting started" >/dev/null; exit 130; }
  if [ "$1" = INT ]; then log "sitting.sh: forwarding SIGINT to timeout $TPID" >/dev/null; kill -INT "$TPID" 2>/dev/null
  else log "sitting.sh: forwarding SIGTERM to timeout's process group $TPID" >/dev/null; kill -TERM -- "-$TPID" 2>/dev/null || kill -TERM "$TPID" 2>/dev/null; fi
}
trap 'fwd INT' INT; trap 'fwd TERM' TERM
MODEL="$P3_MODEL_PRIMARY"; SMOKE=0
while [ $# -gt 0 ]; do case "$1" in --model) MODEL="$2"; shift 2;; --smoke) SMOKE=1; shift;; *) echo "unknown arg $1" >&2; exit 2;; esac; done
# harness self-integrity check (research §2.11: control files have been deleted by agents in other harnesses)
for f in "$RUN_DIR/lib.sh" "$RUN_DIR/loop.sh" "$RUN_DIR/sitting-prompt.md" "$P3_ROOT/P3/STATE.md" "$P3_ROOT/P3/HANDOFF.md" "$P3_ROOT/.claude/skills/p3-implementation/SKILL.md"; do
  [ -s "$f" ] || { echo "PREFLIGHT FAIL: missing $f" >&2; exit 3; }; done
for c in claude jq git timeout; do command -v "$c" >/dev/null || { echo "PREFLIGHT FAIL: no $c on PATH" >&2; exit 3; }; done
git -C "$P3_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1 || { echo "PREFLIGHT FAIL: not a git work tree" >&2; exit 3; }
mkdir -p "$LOG_DIR"; rm -f "$STATUS_FILE"
# update guard (H-3b): a CLI version other than the pinned one must first pass the one-turn probe on the sitting model
CLI_NOW="$(claude --version 2>/dev/null | awk 'NR==1{print $1}')"; CLI_NOW="${CLI_NOW:-unknown}"
CLI_PIN="$(awk 'NR==1{print $1}' "$P3_CLI_PIN" 2>/dev/null)"; CLI_PIN="${CLI_PIN:-none}"
if [ "$CLI_NOW" != "$CLI_PIN" ]; then
  if [ "$CLI_NOW" != unknown ] && probe_model "$MODEL"; then
    log "CLI drift $CLI_PIN→$CLI_NOW: one-turn probe on $MODEL passed — pin updated, continuing"
    notify "P3 CLI updated" "Claude Code $CLI_PIN→$CLI_NOW passed the one-turn probe on $MODEL; pin updated"
    printf '%s\n' "$CLI_NOW" > "$P3_CLI_PIN"
  else
    jq -n --arg n "CLI drift $CLI_PIN→$CLI_NOW: smoke failed" '{outcome:"BLOCKED",note:$n}' > "$STATUS_FILE"
    log "PREFLIGHT BLOCKED: CLI drift $CLI_PIN→$CLI_NOW: smoke failed on $MODEL"; exit 3
  fi
fi
CAP="$(read_cap)"
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
Packet cap (measured, P3/run/cap; overrides the contract's default): land at most $CAP packets this sitting.
Repository truth at launch (distrust any handoff sentence that disagrees with it):
git log --oneline -10:
$GITCTX
git status --short (head -20):
${GITST:-(clean)}"
  WALL="$P3_SITTING_WALL"; TURNS="$P3_SITTING_MAX_TURNS"; SP=(--append-system-prompt-file "$RUN_DIR/sitting-prompt.md")
fi
printf 'model=%s\nstart=%s\nhard_stop=%s\nlog=%s\nsmoke=%s\ncap=%s\ncli=%s\n' "$MODEL" "$START" "$HARD" "$LOGF" "$SMOKE" "$CAP" "$CLI_NOW" > "$META"
log "SITTING $TS start model=$MODEL wall=$WALL turns=$TURNS smoke=$SMOKE cap=$CAP cli=$CLI_NOW"
BUDGET=(); [ -n "$P3_SITTING_MAX_BUDGET_USD" ] && BUDGET=(--max-budget-usd "$P3_SITTING_MAX_BUDGET_USD")
env -u CLAUDECODE TERM=dumb P3_SITTING=1 CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=0 \
timeout --signal=INT --kill-after=15m "$WALL" \
claude -p "$PROMPT" \
  --model "$MODEL" --effort max --max-turns "$TURNS" \
  --permission-mode auto --permission-prompts none \
  --name "p3-sitting-$TS" "${BUDGET[@]}" \
  "${SP[@]}" \
  --output-format stream-json --verbose \
  > "$LOGF" 2> "$ERRF" & TPID=$!
# Started as a background job so a trap runs at once (bash defers traps until a foreground command ends). A forwarded signal
# interrupts wait (status >128); wait again for timeout's own status. The background start hands timeout an ignored SIGINT and
# stdin /dev/null, as before; timeout installs its own SIGINT handler, and its exec resets claude's SIGINT to the default
# (measured 2026-10-08: claude's SigIgn is 0), so timeout's wall-clock INT and a forwarded INT both reach claude.
while FWD=""; wait "$TPID"; EXIT=$?; [ -n "$FWD" ] && [ "$EXIT" -gt 128 ]; do :; done
trap '' INT TERM   # timeout has exited: nothing left to forward to; finish the bookkeeping below (milliseconds)
printf 'exit=%s\nend=%s\n' "$EXIT" "$(date -u +%FT%TZ)" >> "$META"
RJ="$(last_result_json "$LOGF")"
log "SITTING $TS end exit=$EXIT (informational only) subtype=$(printf '%s' "$RJ" | jq -r '.subtype // "-"' 2>/dev/null) turns=$(printf '%s' "$RJ" | jq -r '.num_turns // "-"' 2>/dev/null) cost=$(printf '%s' "$RJ" | jq -r '.total_cost_usd // "-"' 2>/dev/null) log=$LOGF"
echo "$LOGF"
