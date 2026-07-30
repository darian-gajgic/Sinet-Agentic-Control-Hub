#!/usr/bin/env bash
# B6 gate — the operator's own UI click-through (directive 2026-07-20).
#
# ONE command. It builds the SPA and the binary, starts a THROWAWAY control
# plane on its own port with its own fresh database, tells you what to click,
# and shuts down cleanly when you press a key.
#
# It does not touch production. It refuses to start if it would:
#   - production units sinet-control / sinet-broker stay running, untouched
#   - the live front chain (tailscale serve -> Caddy 8481 -> unit 8482) is
#     never bound, never reconfigured, never read
#   - /var/lib/sinet and the installed /usr/local/bin/sinet are never written
#   - migration 0021 is applied to the THROWAWAY database only
#
# Nothing here needs sudo. Nothing here costs money. No push leaves the machine.
#
#   ./P3/gates/B6-clickthrough.sh          # build, run, click, stop
#   ./P3/gates/B6-clickthrough.sh --clean  # delete the throwaway state and exit

set -uo pipefail

PORT="${SINET_CLICKTHROUGH_PORT:-8483}"
STATE="${SINET_CLICKTHROUGH_STATE:-$HOME/.sinet-b6-clickthrough}"
BIN="$STATE/sinet"
LOG="$STATE/control.log"

# Ports this script must never bind: 8481 is the live Caddy, 8482 the live unit.
FORBIDDEN_PORTS="8481 8482"

bold() { printf '\033[1m%s\033[0m\n' "$*"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; }
bad()  { printf '  \033[31m✗\033[0m %s\n' "$*"; }
note() { printf '    %s\n' "$*"; }
die()  { printf '\n\033[31mSTOPPED:\033[0m %s\n\n' "$*"; exit 1; }

if [[ "${1:-}" == "--clean" ]]; then
  if [[ -d "$STATE" ]]; then
    rm -rf "$STATE"
    echo "removed $STATE"
  else
    echo "nothing to remove ($STATE does not exist)"
  fi
  exit 0
fi

cd "$(dirname "$0")/../.." || die "cannot reach the repository root"
REPO="$PWD"
[[ -f go.mod && -d web ]] || die "this does not look like the Sinet repo: $REPO"

bold "B6 click-through — preflight"

# ── 1. Refuse anything that could touch production ──────────────────────────
for p in $FORBIDDEN_PORTS; do
  [[ "$PORT" == "$p" ]] && die "port $PORT is production's. Pick another with SINET_CLICKTHROUGH_PORT."
done
case "$STATE" in
  /var/lib/sinet*|/etc/sinet*) die "state dir $STATE is production's. Refusing." ;;
esac
ok "port $PORT and state dir $STATE are not production's"

if ss -ltn 2>/dev/null | grep -q ":$PORT "; then
  die "port $PORT is already in use. Pick another with SINET_CLICKTHROUGH_PORT=<n>."
fi
ok "port $PORT is free"

# The systemd env vars are what the binary reads as "I am production".
# Clearing them here is what keeps this run in dev posture.
unset STATE_DIRECTORY CONFIGURATION_DIRECTORY NOTIFY_SOCKET

command -v go   >/dev/null || die "go is not on PATH"
command -v npm  >/dev/null || die "npm is not on PATH"
ok "go $(go version | awk '{print $3}') and npm $(npm --version) present"

mkdir -p "$STATE" || die "cannot create $STATE"

# ── 2. Build the SPA, then the binary that embeds it ────────────────────────
bold "Building (about a minute the first time)"

if ! (cd web && npm run build >"$STATE/web-build.log" 2>&1); then
  bad "the SPA build failed"; note "see $STATE/web-build.log"; die "build failed"
fi
BUNDLE=$(grep -oE 'index-[A-Za-z0-9_-]+\.js[[:space:]]+[0-9.]+ kB' "$STATE/web-build.log" | tail -1)
ok "SPA built  ${BUNDLE:-(size not parsed)}"

if ! go build -o "$BIN" ./cmd/sinet 2>"$STATE/go-build.log"; then
  bad "the binary build failed"; note "see $STATE/go-build.log"; die "build failed"
fi
ok "binary built at $BIN"
note "the installed production binary /usr/local/bin/sinet is untouched"

# ── 3. Start the throwaway control plane ────────────────────────────────────
bold "Starting a throwaway control plane"

"$BIN" control --state-dir "$STATE" --config-dir "$STATE" --http-addr "127.0.0.1:$PORT" \
  >"$LOG" 2>&1 &
PID=$!

cleanup() {
  if kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null
    for _ in $(seq 1 50); do kill -0 "$PID" 2>/dev/null || break; sleep 0.1; done
    kill -9 "$PID" 2>/dev/null
  fi
}
trap cleanup EXIT INT TERM

for _ in $(seq 1 100); do
  if curl -fsS "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1; then READY=1; break; fi
  kill -0 "$PID" 2>/dev/null || { bad "the control plane exited during startup"; tail -20 "$LOG"; die "startup failed"; }
  sleep 0.2
done
[[ "${READY:-}" == "1" ]] || { bad "the control plane did not answer /api/health in 20s"; tail -20 "$LOG"; die "startup failed"; }
ok "control plane answering on http://127.0.0.1:$PORT"

SCHEMA=$(grep -oE 'user_version[^0-9]*[0-9]+' "$LOG" | tail -1)
ok "throwaway database created and migrated  ${SCHEMA:-(0001-0021)}"

# Health answering is not the same as the app being there. Check the thing you
# are actually about to open: the shell, the bundle it names, a deep route
# falling back to it, and a session you can browse with.
ASSET=$(curl -fsS "http://127.0.0.1:$PORT/" 2>/dev/null | grep -oE 'assets/index-[A-Za-z0-9_-]+\.js' | head -1)
[[ -n "$ASSET" ]] || { bad "the root served no SPA bundle reference"; tail -20 "$LOG"; die "the app did not ship into the binary"; }
ok "the shell serves and names its bundle  $ASSET"

curl -fsS -o /dev/null "http://127.0.0.1:$PORT/$ASSET" 2>/dev/null \
  && ok "the bundle itself downloads" \
  || die "the shell names a bundle the binary does not serve"

DEEP=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/workforce")
[[ "$DEEP" == "200" ]] && ok "deep routes fall back to the shell (/workforce -> $DEEP)" \
                       || die "/workforce answered $DEEP — a hard refresh on any page would 404"

SESS=$(curl -fsS "http://127.0.0.1:$PORT/api/auth/session" 2>/dev/null)
case "$SESS" in
  *'"authenticated":true'*) ok "dev posture grants a browsable session — no PIN ceremony needed here" ;;
  *) bad "session probe says: $SESS"
     note "you will land on /login. That is production posture, not dev — check that"
     note "STATE_DIRECTORY / CONFIGURATION_DIRECTORY / NOTIFY_SOCKET are unset." ;;
esac

# ── 4. Prove production was not disturbed ───────────────────────────────────
bold "Production, re-checked while this is running"
for u in sinet-control sinet-broker; do
  if systemctl is-active --quiet "$u" 2>/dev/null; then ok "$u still active"
  else note "$u not active (it may not be installed on this host)"; fi
done
ss -ltn 2>/dev/null | grep -q ':8482 ' && ok "the production unit still owns 8482"
ss -ltn 2>/dev/null | grep -q ':8481 ' && ok "the live Caddy still owns 8481"

# ── 5. What to click ────────────────────────────────────────────────────────
URL="http://127.0.0.1:$PORT/"
cat <<EOF

$(bold "OPEN THIS: $URL")

This database is brand new, so most surfaces will be honestly EMPTY. That is
the thing to look at: an empty surface should tell you it is empty and why —
never a blank panel, never a fabricated zero, never a spinner that never ends.

Walk these eleven, in this order. Every one is a real route.

   1.  /              Mission control — running / queued / parked, meters
   2.  /board         The live board — stage columns, card faces
   3.  /tasks/:id     (from the board, if any card exists)
   4.  /fleet         Per-person and per-model meters, budget remainders
   5.  /inbox         The approval inbox — ranked, with tier mechanics
   6.  /settings      Every setting, generated from the registry, with
                      clamp bounds, restart badges and audit history
   7.  /settings      -> the "This device" panel — push enrolment
   8.  /chat          The assistant
   9.  /deliverables/:id  (from a task, if any exists) — diff, comments, try-it
  10.  /workforce     The workforce map — empty on a fresh host is CORRECT
                      ("no standing army": machinery is composed when earned)
  11.  Resize to a phone width and re-walk 1, 5, 6 — phone-complete is a
       requirement; diff review and settings-bulk are desktop-comfortable.

What the gate actually needs from you, in your own words afterwards:

   A. Did anything look WRONG, ugly, or confusing? Taste counts here — the
      report asks you to rule on relative-vs-UTC timestamps, and on whether
      the bundle feels heavy on a phone.
   B. Did any surface fabricate a number instead of saying it did not know?
   C. Did any control look pressable but do nothing?
   D. Anything missing you expected to be there?

Logs are at $LOG if a surface misbehaves.

EOF

read -r -p "Press Enter when you are finished clicking (this shuts it down)... " _ || true

bold "Shutting down"
cleanup
ok "throwaway control plane stopped"
ok "production untouched throughout"
note "state kept at $STATE — re-run this script to pick up where you left off"
note "delete it with: ./P3/gates/B6-clickthrough.sh --clean"
echo
