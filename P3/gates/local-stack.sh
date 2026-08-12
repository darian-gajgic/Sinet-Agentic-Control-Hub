#!/usr/bin/env bash
# P3/gates/local-stack.sh — bring the S12 local tier up for a DEMO WORLD, at
# user level, with zero host changes (P3-RW-11 R8 / OQ4; Spec S12.1 class (b),
# S12.2, S12.8; CONVENTIONS §26).
#
# WHAT THIS IS FOR. The intake classifier seam is wired only when
# SINET_LOCAL_ENDPOINT is set (internal/shell/local_seams.go). No world has ever
# set it, so `taxonomyFor(FamilyGeneric)` stood for every task and the ratified
# software question set was unreachable. This script starts the pinned
# llama-swap as a USER-LEVEL PROCESS on a loopback port and prints the four
# exports a control plane needs to reach it.
#
# WHAT IT IS NOT. It is not a bring-up ceremony and it installs nothing. The
# generated `sinet-llamaswap.service` is NEVER installed here — generated-never-
# installed stands (§26), and installing it is a B4-gate/hardening act.
# Production activation is D6/S19.6's. No driver, kernel, or system package is
# touched; nothing is written outside this script's own run directory.
#
# ONE COMMAND:
#
#     ./P3/gates/local-stack.sh
#
# then paste the printed exports into the shell that launches the demo world.
# Stop it with the printed stop command.
#
# Options (all optional):
#   --state-dir DIR   SINET_LOCAL_STATE to print (default: <rundir>/state).
#                     The directory is created ONLY if you name one.
#   --port N          loopback port (default: a free one, chosen here).
#   --stop            stop a stack this script started, and exit.
#
# Environment overrides:
#   SINET_LOCAL_STACK_ROOT   installed stack root      (default: ~/.sinet-b45)
#   SINET_LOCAL_STACK_RUN    run dir for config/pid/log (default: ~/.sinet-local-stack)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STACK_ROOT="${SINET_LOCAL_STACK_ROOT:-$HOME/.sinet-b45}"
RUN_DIR="${SINET_LOCAL_STACK_RUN:-$HOME/.sinet-local-stack}"
LLAMA_SWAP="$STACK_ROOT/bin/llama-swap"
LLAMA_SERVER="$STACK_ROOT/build/llama.cpp/build-cuda/bin/llama-server"
MODEL_CACHE="$STACK_ROOT/models"
PID_FILE="$RUN_DIR/llama-swap.pid"
PORT_FILE="$RUN_DIR/llama-swap.port"
LOG_FILE="$RUN_DIR/llama-swap.log"
CFG_FILE="$RUN_DIR/llamaswap.yaml"
STATE_DIR=""
PORT=""

die()  { printf '\nFAILED: %s\n' "$*" >&2; exit 1; }
step() { printf '\n== %s\n' "$*"; }
ok()   { printf '   ok: %s\n' "$*"; }
note() { printf '   NOTE: %s\n' "$*"; }

stop_stack() {
    if [[ -f "$PID_FILE" ]]; then
        local pid; pid="$(cat "$PID_FILE")"
        if kill -0 "$pid" 2>/dev/null; then
            # Unload BEFORE the manager dies: llama-swap runs llama-server as a
            # child, and killing the manager orphans that child still holding
            # its VRAM. The unload route is the manager's own teardown verb.
            local port; port="$(cat "$PORT_FILE" 2>/dev/null || true)"
            [[ -n "$port" ]] && curl -fsS -X POST "http://127.0.0.1:$port/api/models/unload" >/dev/null 2>&1 || true
            kill "$pid" 2>/dev/null || true
            sleep 1
            kill -9 "$pid" 2>/dev/null || true
            printf 'stopped llama-swap (pid %s)\n' "$pid"
        fi
        rm -f "$PID_FILE" "$PORT_FILE"
    fi
    # Any llama-server this run dir's config started and left behind.
    pkill -f "llama-server .*--model $MODEL_CACHE/" 2>/dev/null || true
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --state-dir) STATE_DIR="${2:-}"; shift 2 ;;
        --port)      PORT="${2:-}"; shift 2 ;;
        --stop)      stop_stack; exit 0 ;;
        *)           die "unknown argument: $1 (see the header of this script)" ;;
    esac
done

step "1/7  the installed stack is where this expects it"
[[ -x "$LLAMA_SWAP" ]]   || die "no llama-swap at $LLAMA_SWAP — set SINET_LOCAL_STACK_ROOT, or install the stack (that is the B4 gate/hardening step, not this script's)"
[[ -x "$LLAMA_SERVER" ]] || die "no llama-server at $LLAMA_SERVER — the CUDA build is the B4-5 drain's user-level build"
[[ -d "$MODEL_CACHE" ]]  || die "no model cache at $MODEL_CACHE"
ok "llama-swap $LLAMA_SWAP"
ok "llama-server $LLAMA_SERVER"
ok "model cache $MODEL_CACHE"

step "2/7  the pinned versions (LOUD on a delta, never retargeted — §10/§26)"
PIN_SWAP="$(grep -oP 'LlamaSwapPin\s*=\s*"\K[^"]+' "$REPO_ROOT/internal/local/local.go" | head -1)"
HAVE_SWAP="$("$LLAMA_SWAP" --version 2>&1 | grep -oE 'v[0-9]+' | head -1 || true)"
if [[ -n "$HAVE_SWAP" && "$HAVE_SWAP" != "$PIN_SWAP" ]]; then
    note "PIN DELTA: installed llama-swap $HAVE_SWAP vs pin $PIN_SWAP — reconcile through the S12.10 deliberate bump, never silently. Continuing."
else
    ok "llama-swap $HAVE_SWAP matches the pin"
fi

step "3/7  the fast seat's weights (intake-triage resolves here via ⚙ local.alias)"
FAST_GGUF="$(ls -1 "$MODEL_CACHE"/Qwen3.5-4B-*.gguf 2>/dev/null | head -1 || true)"
[[ -n "$FAST_GGUF" ]] || die "the intake-triage fast seat's GGUF is not in $MODEL_CACHE — pull it before wiring the classifier (nothing here fakes a seat)"
ok "$(basename "$FAST_GGUF")"

step "4/7  the placement GPU UUID (placement is by UUID, never index — S12.2)"
# The shape is matched, not just the exit status: a failing nvidia-smi still
# prints a line ("Failed to initialize NVML: Driver/library version mismatch"),
# and taking that as a UUID would put a fabricated placement string in the
# config that nothing rejects until a model tries to load.
GPU_UUID="$(nvidia-smi --query-gpu=uuid --format=csv,noheader 2>/dev/null | grep -oE '^GPU-[0-9a-fA-F-]+' | head -1 || true)"
if [[ -z "$GPU_UUID" ]]; then
    # NVML refuses with "Driver/library version mismatch" on a host whose driver
    # package was upgraded under a running kernel module — a reboot-pending
    # condition that leaves CUDA itself working. The driver's own procfs record
    # carries the same real identity, so it is read rather than invented.
    GPU_UUID="$(grep -h '^GPU UUID:' /proc/driver/nvidia/gpus/*/information 2>/dev/null | grep -oE 'GPU-[0-9a-fA-F-]+' | head -1 || true)"
    [[ -n "$GPU_UUID" ]] && note "nvidia-smi/NVML did not answer; UUID read from the driver's procfs record instead"
fi
[[ "$GPU_UUID" == GPU-* ]] || die "no GPU UUID readable from nvidia-smi or /proc/driver/nvidia — placement is by UUID (S12.2) and none is invented"
ok "$GPU_UUID"

step "5/7  generate the config from TODAY'S manifest (never a reused smoke config)"
mkdir -p "$RUN_DIR"
SINET_LOCAL_MODEL_CACHE="$MODEL_CACHE" \
SINET_LOCAL_LLAMA_SERVER="$LLAMA_SERVER" \
SINET_LOCAL_GPU_UUID="$GPU_UUID" \
    go run "$REPO_ROOT/P3/gates/localstackconfig" > "$CFG_FILE" || die "config generation failed (see the error above)"
ok "$CFG_FILE ($(grep -c '^  "' "$CFG_FILE" || true) seat(s) configured from the manifest's PULLED, GPU-seated, servable rows)"

step "6/7  start llama-swap on a loopback port (user-level; no unit is installed)"
stop_stack >/dev/null 2>&1 || true
if [[ -z "$PORT" ]]; then
    PORT="$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')"
fi
nohup "$LLAMA_SWAP" --config "$CFG_FILE" --listen "127.0.0.1:$PORT" >"$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"
echo "$PORT" > "$PORT_FILE"
for _ in $(seq 1 60); do
    if curl -fsS "http://127.0.0.1:$PORT/v1/models" >/dev/null 2>&1; then READY=1; break; fi
    sleep 0.5
done
[[ "${READY:-}" == "1" ]] || { tail -20 "$LOG_FILE" >&2; die "llama-swap did not become ready on 127.0.0.1:$PORT (log above; full log $LOG_FILE)"; }
ok "listening on 127.0.0.1:$PORT (pid $(cat "$PID_FILE"))"
ok "/v1/models serves: $(curl -fsS "http://127.0.0.1:$PORT/v1/models" | python3 -c 'import json,sys;print(", ".join(d["id"] for d in json.load(sys.stdin)["data"]))')"

step "7/7  the exports the demo world's control plane needs"
if [[ -n "$STATE_DIR" ]]; then
    mkdir -p "$STATE_DIR"
    note "created $STATE_DIR (you named it with --state-dir)"
else
    STATE_DIR="$RUN_DIR/state"
    mkdir -p "$STATE_DIR"
fi
cat <<EOF

Paste these into the shell that launches the world, then start it:

export SINET_LOCAL_ENDPOINT="http://127.0.0.1:$PORT"
export SINET_LOCAL_LLAMA_SERVER="$LLAMA_SERVER"
export SINET_LOCAL_MODEL_CACHE="$MODEL_CACHE"
export SINET_LOCAL_STATE="$STATE_DIR"

With those set, internal/shell wires the S12.4 duty seams and intake triage
runs for real: a request classifies its family on the local 4B seat and writes
its one \$0 D7 row. Without them the seams stay unwired and the interview asks
the requester what kind of task it is — both are honest, neither is generic by
default.

  log:   $LOG_FILE
  stop:  $0 --stop

EOF
