#!/usr/bin/env bash
# P3/gates/lane-test-door.sh — P3-LN-5: the lane-test door.
#
# ONE COMMAND, cold to a CLICKABLE lane test:
#
#     ./P3/gates/lane-test-door.sh
#
# It builds a THROWAWAY world of its own, starts the per-person broker daemon
# that spawn-time credential injection dials, hands you the key ceremony pointed
# at that world, starts the world's control plane AFTER the keys exist, shows
# you the commissioning line the control plane printed about itself, declares
# the automation budget that makes the commissioned lane's consumption pressure
# comparable, and then tells you the exact URL, the exact sign-in, what to click
# and where the lane shows.
#
# Every step is independently re-runnable:
#
#     ./P3/gates/lane-test-door.sh <step>
#
# and the whole thing can be rehearsed with no prompts, no writes, no network
# and no processes at all:
#
#     ./P3/gates/lane-test-door.sh --dry-run
#     ./P3/gates/lane-test-door.sh --dry-run <step>
#
# Steps: preflight  world  broker  keys  plane  budget  walk
# Verbs: stop   (reap the broker and the control plane this door started)
#        clean  (delete this door's throwaway world — refuses any other path)
#
# WHY THIS SCRIPT EXISTS. A placed key alone does not make a lane clickable.
# Four wiring facts have to line up, and each one of them is silent when it
# does not (coordinator click-path analysis, P3/STATE.md 2026-08-25):
#
#   1. The key ceremony's default store is this HOST's (~/.local/state/sinet).
#      A world started with `control --state-dir W` reads broker.StoreRoot(W).
#      A key in the wrong store commissions nothing and says nothing.
#   2. The world's seeded people are op / alice / bob. Commissioning is keyed by
#      the broker store's PERSON, and the adapter asks ProvidersFor(userID) for
#      the person whose session is making the run — so a key placed under your
#      OS user commissions for a userID nobody ever signs in as.
#   3. No broker daemon runs in a world. Spawn-time injection dials
#      <world>/broker/<person>.sock, and an unanswered socket is a spawn that
#      authenticates as nobody. `sinet broker --user X --state-dir Y` exists
#      exactly for this, and this door starts it.
#   4. A commissioned lane the router may pick is not a lane the router DOES
#      pick. Selection compares covered flat-rate lanes by consumption
#      pressure, and pressure needs a declared denominator — the S10.4
#      automation budget, per (person, lane, window), in the window's own unit
#      and never in dollars (D5). Until P3-LN-6 no surface could declare one
#      for a plan-metered lane, so zai and kimi had no comparable pressure and
#      never won anything. `POST /api/meters/plan-budget` is that surface, and
#      the `budget` step below drives it for the person you sign in as.
#
# WHAT IT NEVER TOUCHES. Production (:8481, :8482, /var/lib/sinet, /etc/sinet,
# the installed binary), this host's real broker store (~/.local/state/sinet),
# the operator's live gate world (~/.sinet-b6-clickthrough) and every other
# ~/.sinet-* evidence world. It refuses BY CHECK rather than by intention: a
# state directory that already exists without this door's own marker file is
# somebody else's world and the door stops.
#
# P3/gates/B6-clickthrough.sh is NEVER edited or invoked by this script. It
# carries the operator's own uncommitted changes. This door mirrors the
# mechanisms it established — the same seed test, the same control-plane flags,
# the same production refusals — because it needs an ordering that script does
# not have: commissioning is STARTUP-BOUND, so the control plane must start
# after the keys exist, and it must stay up in the background afterwards while
# you click.
#
# SECRETS. This script never reads, prompts for, echoes, logs or stores a key.
# Key handling belongs entirely to P3/gates/lane-key-ceremony.sh, which takes it
# on stdin through a shell builtin. `set -x` is disabled here so no trace mode
# can print an environment.
#
# The one credential this script does handle is the DEMO WORLD's own sign-in
# PIN, which the `budget` step needs because the plan-budget verb sits behind a
# session. It is read out of the seed's own log at run time — never written
# down here — assembled into the login body by shell builtins alone, and handed
# to curl on STDIN, so it is never a process argument and never an environment
# variable. The session cookie it mints lands in the world at mode 0600 and is
# deleted when the step ends.

set -euo pipefail
set +x

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# Absolute, so every command this script PRINTS can be pasted from any working
# directory. A hand-step that only works from one cwd is a hand-step that fails.
SELF="$REPO/P3/gates/lane-test-door.sh"

# ─── the world this door owns ───────────────────────────────────────────────
DEFAULT_WORLD="$HOME/.sinet-lane-test"
DEFAULT_PORT="8485"
DEFAULT_PERSON="alice"
WORLD="${SINET_LANE_TEST_STATE:-$DEFAULT_WORLD}"
PORT="${SINET_LANE_TEST_PORT:-$DEFAULT_PORT}"
PERSON="${SINET_LANE_TEST_USER:-$DEFAULT_PERSON}"

# Every command this script prints has to work when pasted. If this run is on a
# non-default world, port or person, the paste has to carry that too — a printed
# `stop` that quietly targets the default world would reap nothing and say so
# convincingly.
ENVP=""
[ "$WORLD" = "$DEFAULT_WORLD" ]   || ENVP="${ENVP}SINET_LANE_TEST_STATE=$WORLD "
[ "$PORT" = "$DEFAULT_PORT" ]     || ENVP="${ENVP}SINET_LANE_TEST_PORT=$PORT "
[ "$PERSON" = "$DEFAULT_PERSON" ] || ENVP="${ENVP}SINET_LANE_TEST_USER=$PERSON "

BIN="$WORLD/sinet"
LANEKEY="$WORLD/lanekey"
# The shell spelling of broker.StoreRoot(stateDir). It is the same third
# spelling CONVENTIONS §65 names in the ceremony: held by this script's own
# output — the broker's startup line names the store it opened, and the
# verification below goes through the LIVE socket, so a divergence fails here.
STORE_ROOT="$WORLD/broker-store"
SOCK="$WORLD/broker/$PERSON.sock"
BROKER_LOG="$WORLD/broker.log"
BROKER_PID_FILE="$WORLD/broker.pid"
CONTROL_LOG="$WORLD/control.log"
CONTROL_PID_FILE="$WORLD/control.pid"
SEED_LOG="$WORLD/seed.log"
MARKER="$WORLD/.lane-test-door"
CEREMONY="$REPO/P3/gates/lane-key-ceremony.sh"
DIST="$REPO/internal/webui/dist"
BASE="http://127.0.0.1:$PORT"

# The plan documents the budget step enumerates windows out of. They are the
# SAME files the binary this door built embeds (`//go:embed plandata/*.json` in
# internal/metering/planunits.go), so a window named here and a window the
# running verb knows are one file and cannot disagree. Only the ENUMERATION is
# read here: whether a given window can carry a budget at all is never
# re-derived — the running control plane answers that, per window, in its own
# words, because a rule spelled twice is a rule that drifts.
PLANDATA="$REPO/internal/metering/plandata"
# A session bearer token and a scratch response body, both step-local. The
# cookie is chmod-ed by umask and removed when the step ends.
COOKIE="$WORLD/session.cookie"
API_OUT="$WORLD/api.out"
# api.SessionCookieName — the bearer the session-required surface reads.
SESSION_COOKIE="sinet_session"

LANES="zai kimi"
FORBIDDEN_PORTS="8481 8482"
DRY=0

# The systemd env vars are what the binary reads as "I am production", and the
# demo seed refuses to run at all while they are set. Cleared HERE rather than
# inside preflight, because every step is separately re-runnable and a step run
# on its own must be in the same posture as the same step run in sequence.
unset STATE_DIRECTORY CONFIGURATION_DIRECTORY NOTIFY_SOCKET

pass=0; fail=0
step()  { printf '\n\033[1m== %s\033[0m\n' "$*"; }
ok()    { printf '  \033[32mOK\033[0m    %s\n' "$*"; pass=$((pass+1)); }
bad()   { printf '  \033[31mFAIL\033[0m  %s\n' "$*"; fail=$((fail+1)); }
note()  { printf '  \033[33mNOTE\033[0m  %s\n' "$*"; }
info()  { printf '        %s\n' "$*"; }
dry()   { printf '  \033[36mDRY\033[0m   %s\n' "$*"; }
die()   { printf '\n  \033[31mSTOPPED\033[0m  %s\n\n' "$*" >&2; exit 1; }

lane_doc() { printf '%s/internal/adapters/opencode/lanedata/%s.json' "$REPO" "$1"; }

# ─── refusals ───────────────────────────────────────────────────────────────
# Everything this door must never write to, refused by check. The messages name
# the path, because a refusal an operator cannot act on is a wall with no door
# in it.
refuse_protected_world() {
  case "$WORLD" in
    /var/lib/sinet*|/etc/sinet*)
      die "$WORLD is PRODUCTION's state. Refusing." ;;
    "$HOME"/.local/state/sinet*)
      die "$WORLD is this host's REAL broker store — the one your own keys live in. Refusing." ;;
    "$HOME"/.sinet-b6-clickthrough*)
      die "$WORLD is the operator's live B6 gate world. Refusing — set SINET_LANE_TEST_STATE." ;;
    "$HOME"/.sinet-local-stack*|"$HOME"/.sinet-b45*)
      die "$WORLD belongs to the local GPU stack. Refusing." ;;
    ""|/|"$HOME")
      die "refusing a state directory of '$WORLD'" ;;
  esac
  # Any OTHER directory that already exists and was not made by this door is an
  # evidence world, a walk world, or something else somebody is holding. The
  # marker is the only thing that makes a directory ours.
  if [ -d "$WORLD" ] && [ ! -f "$MARKER" ]; then
    die "$WORLD already exists and carries no lane-test-door marker — it is not this door's world. Refusing to write into it."
  fi
}

# port_free reports whether nothing is listening on $1.
port_free() { ! ss -ltn 2>/dev/null | grep -q ":$1 "; }

# alive_ours reports whether the pid in file $1 is running AND is the process
# this door recorded — a pid file outlives its process, and a recycled pid is
# how a stop verb kills a stranger.
alive_ours() {
  local file=$1 want=$2 pid cmd
  [ -f "$file" ] || return 1
  pid="$(cat "$file" 2>/dev/null || true)"
  case "$pid" in ''|*[!0-9]*) return 1 ;; esac
  [ -d "/proc/$pid" ] || return 1
  cmd="$(tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null || true)"
  case "$cmd" in *"$want"*) return 0 ;; *) return 1 ;; esac
}

pid_of() { cat "$1" 2>/dev/null || true; }

reap() { # $1 = pid file, $2 = cmdline fragment, $3 = what to call it
  local file=$1 want=$2 what=$3 pid
  if ! alive_ours "$file" "$want"; then
    [ -f "$file" ] && rm -f "$file"
    note "$what: nothing of this door's was running"
    return 0
  fi
  pid="$(pid_of "$file")"
  kill "$pid" 2>/dev/null || true
  for _ in $(seq 1 50); do [ -d "/proc/$pid" ] || break; sleep 0.1; done
  [ -d "/proc/$pid" ] && kill -9 "$pid" 2>/dev/null || true
  rm -f "$file"
  ok "$what stopped (pid $pid)"
}

# ─── 1. preflight ───────────────────────────────────────────────────────────
do_preflight() {
  step "1/7  Preflight — what this door will and will not touch"

  [ -d "$REPO/.git" ] && [ -f "$REPO/go.mod" ] || die "this script is not inside the Sinet repository ($REPO)"
  ok "repository: $REPO"

  refuse_protected_world
  ok "world state dir: $WORLD"
  info "PRODUCTION and every other world are refused by check, not by intention:"
  info "  /var/lib/sinet, /etc/sinet, ports 8481/8482        (production)"
  info "  $HOME/.local/state/sinet                    (this host's real store)"
  info "  $HOME/.sinet-b6-clickthrough                (the live B6 gate world)"
  info "  any other existing directory without this door's marker file"

  # A unix socket path lives in a 108-byte field. Over it, the broker dies with
  # "bind: invalid argument" — an error that says nothing about the actual cause
  # and costs an afternoon. Measured here, where the fix is one env var.
  if [ "${#SOCK}" -gt 100 ]; then
    die "the broker socket path would be ${#SOCK} bytes ($SOCK) and a unix socket path must fit in 108. Use a shorter SINET_LANE_TEST_STATE."
  fi
  ok "the broker socket path fits a unix socket (${#SOCK} bytes)"

  local p
  for p in $FORBIDDEN_PORTS; do
    [ "$PORT" = "$p" ] && die "port $PORT is production's. Pick another with SINET_LANE_TEST_PORT."
  done
  if port_free "$PORT"; then
    ok "port $PORT is free"
  elif alive_ours "$CONTROL_PID_FILE" "--http-addr 127.0.0.1:$PORT"; then
    ok "port $PORT is held by this door's own control plane (it will be restarted)"
  else
    die "port $PORT is already in use by something that is not this door. Pick another with SINET_LANE_TEST_PORT=<n>."
  fi

  ok "STATE_DIRECTORY / CONFIGURATION_DIRECTORY / NOTIFY_SOCKET cleared — dev posture"

  command -v go >/dev/null || die "go is not on PATH"
  ok "go present: $(go version | awk '{print $3}')"
  command -v jq >/dev/null || die "jq is not on PATH — the key ceremony needs it"
  ok "jq present: $(command -v jq)"
  command -v curl >/dev/null || die "curl is not on PATH"

  if [ -f "$DIST/index.html" ]; then
    ok "the SPA build is present: $DIST (built $(date -r "$DIST/index.html" '+%Y-%m-%d %H:%M'))"
    info "It is embedded as-is. For a freshly built UI run 'npm run build' in web/ first."
  else
    note "the SPA build output is ABSENT — the world step will run 'npm run build'"
    command -v npm >/dev/null || die "npm is not on PATH and $DIST/index.html does not exist"
  fi

  [ -x "$CEREMONY" ] || die "the key ceremony is missing or not executable: $CEREMONY"
  ok "key ceremony present: $CEREMONY"

  local lane
  for lane in $LANES; do
    [ -f "$(lane_doc "$lane")" ] || die "lane document missing: $(lane_doc "$lane")"
  done
  ok "lane documents present: $LANES"

  info "sign-in person for this run: $PERSON   (override with SINET_LANE_TEST_USER)"
  info "the world's seeded people are checked against the seed in the next step"
}

# ─── 2. world ───────────────────────────────────────────────────────────────
do_world() {
  step "2/7  A fresh world — binary, SPA, seeded demo data"

  if [ "$DRY" = 1 ]; then
    dry "would create $WORLD (marker $MARKER)"
    dry "would build: go build -o $BIN ./cmd/sinet    and    ./tools/lanekey -> $LANEKEY"
    [ -f "$DIST/index.html" ] || dry "would build the SPA: (cd web && npm run build)"
    dry "would seed once: SINET_SEED_DEMO_WORLD=$WORLD go test ./internal/api -run TestSeedDemoWorld"
    dry "would read the seeded people and the PIN out of $SEED_LOG and check '$PERSON' is one of them"
    return 0
  fi

  refuse_protected_world
  mkdir -p "$WORLD"
  : > "$MARKER"
  ok "world directory ready: $WORLD"

  if [ ! -f "$DIST/index.html" ]; then
    ( cd "$REPO/web" && npm run build ) >"$WORLD/web-build.log" 2>&1 \
      || { bad "the SPA build failed — see $WORLD/web-build.log"; die "build failed"; }
    ok "SPA built into $DIST"
  fi

  ( cd "$REPO" && go build -o "$BIN" ./cmd/sinet ) >"$WORLD/go-build.log" 2>&1 \
    || { bad "the binary build failed — see $WORLD/go-build.log"; die "build failed"; }
  ok "binary built: $BIN"
  info "the installed production binary /usr/local/bin/sinet is untouched"

  ( cd "$REPO" && go build -o "$LANEKEY" ./tools/lanekey ) >>"$WORLD/go-build.log" 2>&1 \
    || { bad "tools/lanekey did not build — see $WORLD/go-build.log"; die "build failed"; }
  ok "credential tool built: $LANEKEY"

  # The same seed the B6 gate drives, into THIS world. It refuses any path under
  # /var/lib/sinet or /etc/sinet and refuses to run at all with STATE_DIRECTORY
  # or NOTIFY_SOCKET set — both cleared in preflight.
  if [ -f "$WORLD/.seeded" ]; then
    ok "demo world already seeded — delete it with '$ENVP$SELF clean' to build it fresh"
  else
    if ( cd "$REPO" && SINET_SEED_DEMO_WORLD="$WORLD" \
         go test ./internal/api -run TestSeedDemoWorld -count=1 -v ) >"$SEED_LOG" 2>&1; then
      touch "$WORLD/.seeded"
      ok "demo world seeded"
    else
      bad "the demo seed failed — see $SEED_LOG"
      die "seeding failed"
    fi
  fi

  # What the seed printed about itself, every run and not only the run that
  # seeded it — the people and the PIN are read back out of it, never assumed.
  sed -n 's/^ *seedworld_test\.go:[0-9]*: /        /p' "$SEED_LOG" 2>/dev/null || true

  grep -qF "$PERSON (" "$SEED_LOG" \
    || die "'$PERSON' is not one of this world's seeded people (see the lines above). Set SINET_LANE_TEST_USER to one of them."
  ok "'$PERSON' is a seeded person of this world"
}

# ─── 3. broker ──────────────────────────────────────────────────────────────
do_broker() {
  step "3/7  The broker daemon for $PERSON, inside this world"

  info "Spawn-time credential injection dials $SOCK."
  info "Nothing else in a world starts a broker, so without this the engine"
  info "would spawn authenticating as nobody."

  if [ "$DRY" = 1 ]; then
    dry "would start: $BIN broker --user $PERSON --state-dir $WORLD   (background, log $BROKER_LOG)"
    dry "would record its pid to $BROKER_PID_FILE"
    dry "would wait for the socket $SOCK to appear"
    dry "would probe it with: $LANEKEY verify --socket $SOCK --store-dir $STORE_ROOT --user $PERSON --lane $(lane_doc zai)"
    dry "  a DIAL failure means the socket is dead; a resolve refusal means it ANSWERED"
    return 0
  fi

  [ -x "$BIN" ] || die "the world is not built yet — run: $ENVP$SELF world"

  if alive_ours "$BROKER_PID_FILE" "broker --user $PERSON --state-dir $WORLD"; then
    ok "this door's broker is already running (pid $(pid_of "$BROKER_PID_FILE"))"
  else
    rm -f "$BROKER_PID_FILE"
    "$BIN" broker --user "$PERSON" --state-dir "$WORLD" >"$BROKER_LOG" 2>&1 &
    printf '%s' "$!" > "$BROKER_PID_FILE"
    ok "broker started (pid $(pid_of "$BROKER_PID_FILE")), log $BROKER_LOG"
  fi

  local i
  for i in $(seq 1 50); do
    [ -S "$SOCK" ] && break
    alive_ours "$BROKER_PID_FILE" "broker --user $PERSON" \
      || { bad "the broker exited during startup"; sed -n '1,20p' "$BROKER_LOG"; die "broker failed to start"; }
    sleep 0.1
  done
  [ -S "$SOCK" ] || { sed -n '1,20p' "$BROKER_LOG"; die "the broker did not bind $SOCK within 5s"; }

  # What the daemon says about itself: the person it serves and the store it
  # opened. This is the check that the ceremony's store and the control plane's
  # store are the same directory.
  # head FIRST: under `set -o pipefail`, a `sed | head -3` over a longer log
  # kills sed with EPIPE and takes the whole script down with it.
  head -3 "$BROKER_LOG" | sed 's/^/        /'

  probe_socket
}

# probe_socket proves the socket ANSWERS, whether or not a key is placed yet.
# `lanekey verify --socket` dials the live daemon: a dial failure and a resolve
# refusal are different outcomes, and only the first means a dead socket.
probe_socket() {
  local out rc=0
  out="$("$LANEKEY" verify --socket "$SOCK" --store-dir "$STORE_ROOT" --user "$PERSON" \
        --lane "$(lane_doc zai)" 2>&1)" || rc=$?
  if [ "$rc" = 0 ]; then
    ok "the broker socket ANSWERS and already resolves a placed credential"
    return 0
  fi
  case "$out" in
    *"dial live broker"*)
      printf '        %s\n' "$out"
      die "the broker socket at $SOCK did not answer — injection would dial an unanswered socket" ;;
    *)
      ok "the broker socket ANSWERS (it refused a profile that is not placed yet — a reply, not a dead socket)"
      info "${out#lanekey verify: }" ;;
  esac
}

# ─── 4. keys ────────────────────────────────────────────────────────────────
do_keys() {
  step "4/7  Key placement — the ceremony, pointed at this world and this person"

  info "The ceremony is handed two things and changes nothing else:"
  info "  STATE_DIRECTORY=$WORLD      -> store $STORE_ROOT/$PERSON"
  info "  SINET_CEREMONY_USER=$PERSON -> the person you will SIGN IN as"
  info "It prompts for each key with 'read -rs' and hands it to the placement"
  info "tool on stdin. This door never sees, logs or stores a key."

  if [ "$DRY" = 1 ]; then
    dry "would run, one step at a time (each tolerated to fail — a lane you have no key for must not stop the door):"
    dry "  STATE_DIRECTORY=$WORLD SINET_CEREMONY_USER=$PERSON $CEREMONY preflight"
    dry "  STATE_DIRECTORY=$WORLD SINET_CEREMONY_USER=$PERSON $CEREMONY zai-key"
    dry "  STATE_DIRECTORY=$WORLD SINET_CEREMONY_USER=$PERSON $CEREMONY kimi-key"
    dry "would then verify each lane through the LIVE socket: $LANEKEY verify --socket $SOCK ..."
    dry "would require at least ONE lane to resolve before starting the control plane"
    note "rehearse the ceremony itself with: $CEREMONY --dry-run"
    return 0
  fi

  [ -S "$SOCK" ] || die "the broker is not running — run: $ENVP$SELF broker"

  note "The door runs the ceremony's KEY steps only."
  info "Its other steps are gate acts of their own and are not part of a click"
  info "test: step 4 is a real paid call, step 5 posts to the vendor, steps 6-7"
  info "are recording tasks. Run '$CEREMONY' by itself for those."
  info "Its step-4 tier-L smoke would find nothing here anyway: the smoke resolves"
  info "its store person from the OS user (live_smoke_test.go, and no Go changed"
  info "this packet), so under person '$PERSON' it looks in $STORE_ROOT/$(id -un)"
  info "and prints a SANCTIONED SKIP."

  local s
  for s in preflight zai-key kimi-key; do
    printf '\n'
    if STATE_DIRECTORY="$WORLD" SINET_CEREMONY_USER="$PERSON" "$CEREMONY" "$s"; then
      ok "ceremony step '$s' completed"
    else
      note "ceremony step '$s' did not complete — carrying on (a lane you have no key for is not a door failure)"
    fi
  done

  printf '\n'
  local lane placed=0
  for lane in $LANES; do
    if "$LANEKEY" verify --socket "$SOCK" --store-dir "$STORE_ROOT" --user "$PERSON" \
         --lane "$(lane_doc "$lane")" >/dev/null 2>&1; then
      ok "$lane: resolves through the LIVE broker socket — the same socket a spawn dials"
      placed=$((placed+1))
    else
      note "$lane: no credential resolves for $PERSON (not placed, or declined)"
    fi
  done
  [ "$placed" -gt 0 ] || die "no lane is placed for '$PERSON', so there is nothing to click through. Re-run: $ENVP$SELF keys"
  ok "$placed lane(s) placed and live-resolvable for $PERSON"
}

# ─── 5. plane ───────────────────────────────────────────────────────────────
do_plane() {
  step "5/7  The control plane — started AFTER the keys, because commissioning is startup-bound"

  info "Coverage, the lane-to-substrate map, the alternate seats and the credential"
  info "injector are all composed once, at startup, from what is placed in each"
  info "person's broker store. A key placed later is picked up at the NEXT start,"
  info "which is why this step always restarts rather than reusing."

  if [ "$DRY" = 1 ]; then
    dry "would stop any control plane this door had running"
    dry "would start: $BIN control --state-dir $WORLD --config-dir $WORLD --http-addr 127.0.0.1:$PORT"
    dry "would wait for $BASE/api/health"
    dry "would re-check, while it runs, that production still holds 8481/8482 and its units are active"
    dry "would grep 'lanes: commissioned' out of $CONTROL_LOG and SHOW it as the proof"
    dry "would fail if that line does not name '$PERSON' with at least one lane"
    return 0
  fi

  [ -x "$BIN" ] || die "the world is not built yet — run: $ENVP$SELF world"
  [ -S "$SOCK" ] || die "the broker is not running — run: $ENVP$SELF broker"

  if alive_ours "$CONTROL_PID_FILE" "control --state-dir $WORLD"; then
    reap "$CONTROL_PID_FILE" "control --state-dir $WORLD" "the previous control plane"
  fi

  "$BIN" control --state-dir "$WORLD" --config-dir "$WORLD" --http-addr "127.0.0.1:$PORT" \
    >"$CONTROL_LOG" 2>&1 &
  printf '%s' "$!" > "$CONTROL_PID_FILE"
  ok "control plane started (pid $(pid_of "$CONTROL_PID_FILE")), log $CONTROL_LOG"

  local i ready=0
  for i in $(seq 1 100); do
    if curl -fsS "$BASE/api/health" >/dev/null 2>&1; then ready=1; break; fi
    alive_ours "$CONTROL_PID_FILE" "control --state-dir $WORLD" \
      || { bad "the control plane exited during startup"; tail -20 "$CONTROL_LOG"; die "startup failed"; }
    sleep 0.2
  done
  [ "$ready" = 1 ] || { tail -20 "$CONTROL_LOG"; die "the control plane did not answer /api/health in 20s"; }
  ok "control plane answering on $BASE"

  # Production, re-checked WHILE this world runs — the B6 gate's habit. A
  # refusal up front says what the script intends; this says what happened.
  local u
  for u in sinet-control sinet-broker; do
    if systemctl is-active --quiet "$u" 2>/dev/null; then ok "production $u still active"
    else info "$u not active (it may not be installed on this host)"; fi
  done
  ss -ltn 2>/dev/null | grep -q ':8482 ' && ok "the production unit still owns 8482" || true
  ss -ltn 2>/dev/null | grep -q ':8481 ' && ok "the live front chain still owns 8481" || true

  show_commissioning
}

# show_commissioning prints the control plane's OWN startup line about what it
# commissioned, verbatim. It is the only proof that a placed key became a
# routable lane for this person: everything before it is about a store.
show_commissioning() {
  local line
  line="$(grep 'lanes: commissioned' "$CONTROL_LOG" 2>/dev/null | tail -1 || true)"
  if [ -z "$line" ]; then
    bad "the control plane printed no commissioning line at all"
    info "That is a control plane that did not run the fill — see $CONTROL_LOG"
    die "no commissioning line"
  fi
  printf '\n  \033[1mThe control plane, about itself, at startup:\033[0m\n\n'
  printf '%s\n\n' "$line" | sed 's/^/        /'
  # Matched as "<person>=", which is how per_person spells it — a bare name
  # would also match a word inside the message itself.
  case "$line" in
    *"$PERSON="*) ok "the line names '$PERSON' — commissioning is keyed by the person you sign in as" ;;
    *) bad "the line does NOT name '$PERSON'"
       info "The key is in a store this control plane did not read, or under another person."
       die "commissioned for nobody you can sign in as" ;;
  esac
  case "$line" in
    *lanes=\"\"*|*lanes=\ *|*"lanes= "*)
      bad "the line names NO lane — a store was read and nothing in it commissioned"
      die "no lane commissioned" ;;
    *) ok "at least one lane is commissioned in this running control plane" ;;
  esac
  case "$line" in
    *placed_matching_no_lane_document=\"\"*|*placed_matching_no_lane_document=\ *) : ;;
    *placed_matching_no_lane_document=*)
      note "the line reports a placed profile that NO lane document names — that is a typo'd profile, not a lane" ;;
  esac
}

# ─── 6. budget ──────────────────────────────────────────────────────────────

# person_lanes prints the lanes commissioned FOR THIS PERSON, from the control
# plane's own per_person field — not the line's `lanes=` union, which is every
# person's lanes added together. slog quotes a field value that holds a space,
# so both spellings are handled; the field is `who=a+b who2=c`.
person_lanes() {
  local line pp tok
  line="$(grep 'lanes: commissioned' "$CONTROL_LOG" 2>/dev/null | tail -1 || true)"
  [ -n "$line" ] || return 0
  case "$line" in
    *'per_person="'*) pp="${line#*per_person=\"}"; pp="${pp%%\"*}" ;;
    *'per_person='*)  pp="${line#*per_person=}";   pp="${pp%% *}" ;;
    *) return 0 ;;
  esac
  for tok in $pp; do
    case "$tok" in
      "$PERSON="*) printf '%s' "${tok#*=}" | tr '+' ' ' ;;
    esac
  done
}

plan_doc()     { printf '%s/%s.json' "$PLANDATA" "$1"; }
plan_windows() { jq -r '.quotas[]?.name' "$(plan_doc "$1")" 2>/dev/null || true; }

# json_escape quotes a value for a JSON string using SHELL BUILTINS ONLY. It
# exists for the PIN: `jq --arg pin "$pin"` would put the PIN in argv, where
# any user on the host can read it out of ps.
json_escape() {
  local s=${1//\\/\\\\}
  s=${s//\"/\\\"}
  printf '%s' "$s"
}

# api_call runs one request and prints the HTTP status. The BODY, when there is
# one, arrives on this function's stdin and is forwarded to curl on ITS stdin —
# never as an argument. The response lands in $API_OUT.
#   $1 = method, $2 = path
api_call() {
  local method=$1 path=$2
  curl -sS -X "$method" -o "$API_OUT" -w '%{http_code}' \
       -b "$COOKIE" -H 'Content-Type: application/json' \
       --data-binary @- "$BASE$path" 2>/dev/null || true
}

api_get() {
  curl -sS -o "$API_OUT" -w '%{http_code}' -b "$COOKIE" "$BASE$1" 2>/dev/null || true
}

api_detail() { jq -r '.detail // .error // "(no detail in the response)"' "$API_OUT" 2>/dev/null || true; }
wrapped()    { printf '%s\n' "$1" | fold -s -w 72 | sed 's/^/          /'; }

# door_login mints a real session for $PERSON. It has to: the plan-budget verb
# sits behind the session-required surface, and in dev posture an unauthenticated
# request does NOT bounce — it resolves to the dev FALLBACK identity, whose user
# id is the literal "dev". A declaration sent without a session would therefore
# be a budget declared for "dev", answered 200, and invisible to '$PERSON'
# forever. Sending `person` explicitly turns that into a loud 403 instead of a
# silent mis-declaration.
door_login() {
  local pin code body
  pin="$(grep -oE 'PIN [^ )]+' "$SEED_LOG" 2>/dev/null | head -1 | cut -d' ' -f2 || true)"
  [ -n "$pin" ] || die "no sign-in PIN in $SEED_LOG — run '$ENVP$SELF clean' and start over"
  body="{\"user_id\":\"$(json_escape "$PERSON")\",\"pin\":\"$(json_escape "$pin")\"}"
  rm -f "$COOKIE"
  # umask INSIDE the substitution: the cookie is a bearer token and curl creates
  # the jar itself, so the mode has to be right before it exists, and the umask
  # must not leak into the rest of the run.
  code="$(umask 077; printf '%s' "$body" | curl -sS -o "$API_OUT" -w '%{http_code}' \
          -c "$COOKIE" -H 'Content-Type: application/json' \
          --data-binary @- "$BASE/api/auth/login" 2>/dev/null || true)"
  case "$code" in
    200) : ;;
    401) die "the world refused '$PERSON' with the PIN its own seed log printed. Re-seed: $ENVP$SELF clean" ;;
    *)   bad "POST /api/auth/login answered $code"; wrapped "$(api_detail)"; die "could not mint a session" ;;
  esac
  grep -q "$SESSION_COOKIE" "$COOKIE" 2>/dev/null \
    || die "the login answered 200 but set no $SESSION_COOKIE cookie — nothing below would be authenticated"
  ok "signed in as '$PERSON' through the real login route (session cookie in $COOKIE, mode 0600, deleted when this step ends)"
}

# declare_window declares ONE (person, lane, window) plan budget.
#   0 = declared    1 = the platform refused it BY ITS OWN RULE (a sanctioned
#                       skip, not a door failure)    anything else dies.
declare_window() {
  local lane=$1 window=$2 body code detail ends
  # from_proposal, not a figure: the platform derives the number from the plan's
  # PUBLISHED allowance at ⚙ budget.background_window_fraction. A denominator
  # this script invented would be the inferred provider window D4 bars, and the
  # door has no business choosing how much of somebody's plan automation may
  # spend. `person` is explicit so a session that is not '$PERSON' is a 403.
  body="{\"person\":\"$(json_escape "$PERSON")\",\"lane\":\"$(json_escape "$lane")\",\"window\":\"$(json_escape "$window")\",\"from_proposal\":true,\"reason\":\"lane-test-door budget step: the commissioned lane needs a declared denominator before its consumption pressure is comparable\"}"

  code="$(printf '%s' "$body" | api_call POST /api/meters/plan-budget)"
  case "$code" in
    200) : ;;
    400)
      detail="$(api_detail)"
      case "$detail" in
        *"cannot carry a budget on its"*|*"publishes no allowance"*)
          note "$lane/$window: SANCTIONED SKIP — the platform refused this window by its own rule:"
          wrapped "$detail"
          return 1 ;;
        *)
          bad "$lane/$window: the declaration was refused"; wrapped "$detail"
          die "plan-budget declaration refused" ;;
      esac ;;
    401)
      die "the control plane answered 401 — the session did not carry, so nothing was declared for '$PERSON'" ;;
    403)
      bad "$lane/$window: forbidden"; wrapped "$(api_detail)"
      die "the session is not '$PERSON' and is not the operator" ;;
    503)
      bad "the plan-budget verb is not wired in this control plane"; wrapped "$(api_detail)"
      die "POST /api/meters/plan-budget answered 503" ;;
    *)
      bad "$lane/$window: POST /api/meters/plan-budget answered $code"
      head -c 400 "$API_OUT" 2>/dev/null | sed 's/^/          /'; printf '\n'
      die "plan-budget declaration failed" ;;
  esac

  ok "$lane/$window: $(jq -r '"\(.budget.period_units) \(.budget.unit) over \(.budget.period_hours)h · source \(.budget.source) · seeded from the \"\(.budget.seeded_from)\" allowance at share \(.budget.seed_share)"' "$API_OUT")"

  # The response's own sentence about when this stops applying, printed because
  # for the shortest window that is HOURS away and it is by design.
  detail="$(jq -r '.detail // empty' "$API_OUT" 2>/dev/null || true)"
  ends="$(printf '%s' "$detail" | grep -oE 'ends at [^ ]+' | head -1 | cut -d' ' -f3 || true)"
  [ -n "$ends" ] && note "$lane/$window PERIOD ENDS $ends — after that this budget applies to NOTHING, by design."
  wrapped "$detail"

  # Read-back, through the verb's own store read: declaring again returns the row
  # it REPLACED, and that prior row is read inside the write transaction. No
  # prior means the first declaration never landed. It is also the answer to what
  # re-declaring does — the verb upserts, so it is accepted and starts the next
  # period rather than being refused.
  code="$(printf '%s' "$body" | api_call POST /api/meters/plan-budget)"
  [ "$code" = 200 ] || { bad "$lane/$window: re-declaring answered $code"; wrapped "$(api_detail)"; die "re-declare failed"; }
  local prior
  prior="$(jq -r '.prior // empty | "\(.period_units) \(.unit), window \"\(.window)\", source \(.source), declared by \(.declared_by)"' "$API_OUT" 2>/dev/null || true)"
  [ -n "$prior" ] || { bad "$lane/$window: re-declaring returned NO prior row, so the first declaration was never stored"; die "the plan budget did not land"; }
  ok "$lane/$window: re-declared — the verb read back the row it replaced ($prior)"
  return 0
}

# meters_show reads the lane back through GET /api/meters and shows its plan
# block. A lane with NO row there is not a failure and not a surprise: the
# meters read projects its (person, lane) rows out of the runs table, so a lane
# nobody has run work on has no row for a plan block to hang on. That is the
# same fact the walk's note (c) has always stated.
meters_show() {
  local lane=$1 code n bound
  code="$(api_get "/api/meters?lane=$lane")"
  [ "$code" = 200 ] || { bad "GET /api/meters?lane=$lane answered $code"; wrapped "$(api_detail)"; die "the meters read failed"; }
  n="$(jq --arg l "$lane" '[.lanes[] | select(.lane == $l)] | length' "$API_OUT" 2>/dev/null || echo 0)"
  if [ "$n" = 0 ]; then
    note "$lane: GET /api/meters serves no row for it yet, and that is the documented shape —"
    info "the meters read projects its (person, lane) rows from the RUNS table, so a"
    info "lane nobody has run work on has nothing for a plan block to hang on. The"
    info "proof the row landed is the verb's own read-back above; give the lane work"
    info "from the walk and the block appears here."
    return 0
  fi
  printf '\n  \033[1mGET /api/meters?lane=%s — the plan block, as the platform serves it:\033[0m\n\n' "$lane"
  jq --arg l "$lane" '.lanes[] | select(.lane == $l) | {owner, lane, budget_declared, plan}' "$API_OUT" | sed 's/^/        /'
  bound="$(jq -r --arg l "$lane" '[.lanes[] | select(.lane == $l) | .plan.budget.window // empty][0] // empty' "$API_OUT" 2>/dev/null || true)"
  if [ -n "$bound" ]; then
    ok "$lane: the plan block carries a declared budget, bound to the '$bound' window"
    info "One window binds a lane: the MOST CONSTRAINED one, whichever sits at the"
    info "highest share of its own budget. Every declared window is still listed."
  else
    bad "$lane: the meters row exists and its plan block carries NO budget"
    die "the declaration did not reach the reading"
  fi
}

do_budget() {
  step "6/7  The plan budget — what makes the commissioned lane's pressure comparable"

  info "A commissioned lane is a lane the router MAY pick, not one it does. It picks"
  info "between covered flat-rate lanes on consumption PRESSURE, and a pressure needs"
  info "a declared denominator: the S10.4 automation budget, at the (person, lane,"
  info "window) grain, in the WINDOW's own unit and never in dollars (D5)."
  info "This step declares one for $PERSON on every commissioned lane, taking the"
  info "platform's own PROPOSAL rather than inventing a figure."

  if [ "$DRY" = 1 ]; then
    dry "would read the lanes commissioned FOR $PERSON out of the per_person field of the control plane's own line in $CONTROL_LOG"
    dry "would read each lane's window names from $PLANDATA/<lane>.json — the same files the binary embeds"
    dry "would read $PERSON's PIN out of $SEED_LOG and mint a session:"
    dry "  POST $BASE/api/auth/login   {\"user_id\":\"$PERSON\",\"pin\":\"<the seed's own PIN>\"}"
    dry "  the PIN is assembled by shell builtins and handed to curl on STDIN — never argv, never the environment"
    dry "  the cookie lands in $COOKIE at mode 0600 and is deleted when the step ends"
    dry "would declare, per lane and per window, through that session:"
    dry "  POST $BASE/api/meters/plan-budget"
    dry "  {\"person\":\"$PERSON\",\"lane\":\"<lane>\",\"window\":\"<window>\",\"from_proposal\":true,\"reason\":\"...\"}"
    dry "would print each response's own detail, which states the instant that period ENDS"
    dry "would declare each window a SECOND time and require the response's 'prior' to carry the row just written — the verb's own read-back (it upserts, so a re-declare is accepted and starts the next period)"
    dry "would then read GET $BASE/api/meters?lane=<lane> and show the lane's plan block"
    dry "would treat a window the platform refuses BY ITS OWN RULE as a sanctioned skip:"
    dry "  a window denominated in a unit the lane's consumption is not counted in, or one whose allowance nobody published"
    dry "would fail if no window took a budget on any commissioned lane"
    return 0
  fi

  curl -fsS "$BASE/api/health" >/dev/null 2>&1 \
    || die "nothing is answering on $BASE — run: $ENVP$SELF plane"

  local lanes
  lanes="$(person_lanes)"
  [ -n "$lanes" ] \
    || die "the control plane commissioned no lane for '$PERSON', so there is no plan budget to declare. Run: $ENVP$SELF plane"
  ok "commissioned for $PERSON: $lanes   (the control plane's own per_person field)"

  # The cookie is a bearer token and the scratch body is response noise. Neither
  # outlives the step, and `die` exits, so the trap is what actually guarantees it.
  trap 'rm -f "$COOKIE" "$API_OUT"' EXIT
  door_login

  local lane window windows declared=0 skipped=0
  for lane in $lanes; do
    printf '\n'
    if [ ! -f "$(plan_doc "$lane")" ]; then
      note "$lane: it meters in no plan units, so it carries no PLAN budget — its"
      info "automation budget is the weighted-consumption one (POST /api/meters/budget)"
      continue
    fi
    windows="$(plan_windows "$lane")"
    if [ -z "$windows" ]; then
      note "$lane: its plan document declares no allowance window at all"
      continue
    fi
    for window in $windows; do
      if declare_window "$lane" "$window"; then
        declared=$((declared+1))
      else
        skipped=$((skipped+1))
      fi
    done
    meters_show "$lane"
  done

  printf '\n'
  [ "$declared" -gt 0 ] \
    || die "no window took a budget on any commissioned lane, so nothing gained comparable pressure. The refusals above say why."
  ok "$declared plan budget(s) declared for $PERSON; $skipped window(s) refused by the platform's own rule"
  note "Re-run '$ENVP$SELF budget' whenever a period has ended. Nothing rolls one"
  info "over — re-declaring IS the act that starts the next period."
}

# ─── 7. walk ────────────────────────────────────────────────────────────────
do_walk() {
  step "7/7  What to open, who to be, what to click"

  if [ "$DRY" = 1 ]; then
    dry "would read the people out of the RUNNING world: GET $BASE/api/auth/users"
    dry "would read the PIN out of $SEED_LOG (the seed's own printed line)"
    dry "would print the URL, the sign-in, the click path and the honest notes"
    return 0
  fi

  curl -fsS "$BASE/api/health" >/dev/null 2>&1 \
    || die "nothing is answering on $BASE — run: $ENVP$SELF plane"

  # Everything below is read back out of the RUNNING world and the seed's own
  # output. Nothing about this world is written down here, so this walk cannot
  # drift from what the platform actually serves.
  local pin people
  pin="$(grep -oE 'PIN [^ )]+' "$SEED_LOG" 2>/dev/null | head -1 | cut -d' ' -f2 || true)"
  people="$(curl -fsS "$BASE/api/auth/users" 2>/dev/null | tr '{' '\n' | grep '"user_id"' \
    | while IFS= read -r r; do
        u=$(printf '%s' "$r"  | grep -oE '"user_id":"[^"]+"' | cut -d'"' -f4)
        ro=$(printf '%s' "$r" | grep -oE '"role":"[^"]+"'    | cut -d'"' -f4)
        printf '          %-7s %-9s PIN %s\n' "$u" "$ro" "${pin:-(not in the seed log — run '$ENVP$SELF clean' and start over)}"
      done || true)"
  # A here-string, not a pipe: `printf | grep -q` can hand printf an EPIPE and
  # pipefail then reports a match as a failure.
  grep -q "^ *$PERSON " <<< "$people" \
    || die "the running world does not serve a person called '$PERSON' — the keys are placed for nobody"
  ok "'$PERSON' is served by the running world"

  local commissioned
  commissioned="$(grep 'lanes: commissioned' "$CONTROL_LOG" 2>/dev/null | tail -1 \
    | grep -oE 'lanes=[^ ]*' | cut -d= -f2 | tr -d '"' || true)"

  cat <<EOF

  $(printf '\033[1mOPEN THIS:  %s\033[0m' "$BASE")

  $(printf '\033[1mSIGN IN AS %s. This is the whole point of the door.\033[0m' "$PERSON")

$people
          (read out of the running world just now; the PIN is the seed's own)

  Sign in from the header, or go straight to $BASE/login.
  Browsing WITHOUT signing in gives you the dev fallback identity, whose user id
  is the literal string "dev" — it holds no broker store, so it is commissioned
  on nothing, and the household surfaces wall themselves off from it anyway.
  A key placed for '$PERSON' does nothing at all for "dev".

  $(printf '\033[1mGIVE IT WORK\033[0m')

   1. Open  $BASE/
      Press "Describe a goal". It is a BUTTON — on Mission control, on the
      board and on projects — never a nav tab.
   2. You land on  $BASE/new
      "The goal": type a small, real, coding-shaped task.
      "A short name for the board": anything.
      Press "Send it — plan this goal".
   3. Answer the interview questions it asks you, in place on the same page.
   4. The approval card appears. Before you press anything, read the
      "Who does it" section: worker, model, effort, and the plain reason.
   5. Press "Approve: start the work" to run it.

  $(printf '\033[1mWHERE THE LANE SHOWS\033[0m')

   a. The approval card's plain reason, step 4 — this is the one that reads
      back what the door did. With lanes commissioned it carries a sentence
      about CHOOSING, and which sentence you get is the whole click-path proof.
      There are two, and both are correct answers to different states:

        "N flat-rate lanes cover this duty but lane <name> has no declared
         automation budget, so there is no comparable consumption pressure and
         the deterministic duty-map order stands (S10.4; never dollars — D5)."

        "Chosen among N covered flat-rate lanes on consumption pressure —
         <lane> sits at X% of its declared automation budget, the least
         consumed (D5: never dollars)."

      N counts anthropic PLUS every lane commissioned for you (${commissioned:-none}).
      With nothing commissioned there is no such sentence at all — the router
      has one covered lane and says nothing about choosing. (One exception,
      also correct: a matched specialist that PINS a model skips lane choice
      entirely, so no such sentence appears.)
      READ THE LANE THE FIRST SENTENCE NAMES. Before the budget step it named
      zai or kimi. After it, those lanes have a declared denominator, so if you
      still get the first sentence it now names ANTHROPIC — and that is the
      budget step's own visible effect. Why anthropic: see the third honest
      note below.
   b. $BASE/tasks/<your task>
      The receipt table under the run: Purpose · Priced · Calls · Model · Lane ·
      Unpriced calls. The Lane column is the lane the run actually used.
   c. $BASE/fleet
      Per-person, per-lane meters: Whose · Lane · Open runs · Parked · Until,
      and consumption / pressure / budget remaining. A lane gets a row here once
      work has RUN on it — not when a key is placed.
   d. $BASE/workforce
      Per routed run: model · lane · effort, with the routing reason. The closest
      thing to a "which lane ran it" audit surface.

  $(printf '\033[1mHONEST NOTES — read these before reporting a defect\033[0m')

   · WHAT A GREEN DOOR NOW PROVES, exactly: the key is placed under the person
     you sign in as; it resolves through the LIVE broker socket that a spawn
     dials ($SOCK); the control plane commissioned that lane for that person at
     startup — the line printed above, from the control plane about itself; and
     that lane now carries a DECLARED automation budget, in its plan's own
     unit, at the (person, lane, window) grain, read back out of the platform
     after it was written.
   · THE LANE CAN NOW WIN. This is new, and it is the point of the budget step.
     Selection compares covered flat-rate lanes on consumption pressure —
     consumption over the DECLARED budget, each in its own unit — and the
     LEAST-CONSUMED lane takes the work. It is a real comparison, not a
     tie-break: on a fresh world the commissioned GLM/Kimi lane genuinely can
     and will take execution work, and it loses the moment it is the more
     consumed one (pinned in both directions by
     TestLN6CommissionedLaneWinsAndLosesOnConsumption, through the production
     router). Before P3-LN-6 no plan budget could be declared at all, so those
     lanes never had comparable pressure and anthropic won every time; that is
     the sentence this note used to carry, and it is retired.
   · COMPARABILITY IS ALL-OR-NOTHING, and this door only budgets the
     COMMISSIONED lanes. One covered lane that cannot be compared ends the
     comparison for all of them, and the deterministic duty-map order stands.
     Anthropic is covered too, it is metered in weighted TOKENS rather than
     plan units, and its budget is the token one — a different verb, with no
     proposal to derive a figure from, so this door does not declare it and
     will not invent a number that would decide your routing. Until you declare
     it, expect the "no comparable consumption pressure" sentence naming
     anthropic, and Lane = anthropic in the receipt. To close it, in the
     browser as $PERSON, or:

        curl -sS -X POST $BASE/api/meters/budget \\
          -b <your session cookie> -H 'Content-Type: application/json' \\
          -d '{"person":"$PERSON","lane":"anthropic","period_tokens":<yours>,"period_days":7}'

     The figure is YOURS to choose — that is exactly why nothing here picks it.
   · A DECLARED PERIOD ENDS, and nothing rolls it over. The budget step's
     shortest window is five hours long: five hours after you ran it, that
     budget applies to nothing, the lane stops being comparable, and routing
     falls back — silently, as far as the click path is concerned. Re-run
     '$ENVP$SELF budget'. Re-declaring IS the act that starts the next period.
   · A commissioned lane is an EXECUTION-duty alternate only. Planning and judge
     seats stay anthropic-only by design, so the interview and the plan you are
     about to read were never candidates for it.
   · It does not prove the key WORKS against the vendor either. The only thing
     that does is the ceremony's tier-L smoke, which is one real paid call and
     is deliberately not part of this door.
   · The Kimi C5 no-household-personal-data rider is RECORDED, NOT ENFORCED
     (lanedata/kimi.json, enforced:false). There is no per-lane data-policy
     enforcement point anywhere in the tree. Do not type personal or household
     data into a task on this world and expect the platform to hold that line.
   · This door does NOT wire the local GPU tier (the B6 gate script is the one
     that does). Local-tier seats are unconfigured here, so interview wording
     and any local seat degrade — expected, not a defect.
   · Re-running the door is safe. The world is reused, the seed runs once, the
     broker is reused if it still answers, and the control plane is ALWAYS
     restarted, because commissioning is startup-bound and a key placed after a
     start would otherwise be invisible to it. The budget step re-declares
     rather than skipping: the verb upserts, so the figure is the same one and
     the PERIOD starts again from now, which is what you want after a restart
     and the only way a lapsed window comes back.

  $(printf '\033[1mTHE BROKER AND THE CONTROL PLANE ARE STILL RUNNING\033[0m')

  On purpose: injection dials that socket every time work spawns, so it has to
  outlive this script. They are not systemd units and nothing restarts them.

     broker         pid $(pid_of "$BROKER_PID_FILE")   log $BROKER_LOG
     control plane  pid $(pid_of "$CONTROL_PID_FILE")   log $CONTROL_LOG

  Stop both when you are done:

     $ENVP$SELF stop

  Delete this throwaway world entirely (it refuses any other path):

     $ENVP$SELF clean

EOF
}

# ─── verbs ──────────────────────────────────────────────────────────────────
do_stop() {
  step "stop — reaping what this door started"
  if [ "$DRY" = 1 ]; then
    dry "would kill the pids recorded in $CONTROL_PID_FILE and $BROKER_PID_FILE"
    dry "  each is checked against /proc/<pid>/cmdline first, so a recycled pid is never killed"
    return 0
  fi
  reap "$CONTROL_PID_FILE" "control --state-dir $WORLD" "control plane"
  reap "$BROKER_PID_FILE" "broker --user $PERSON --state-dir $WORLD" "broker"
  info "the world is kept at $WORLD — re-run '$ENVP$SELF' to bring it back up"
}

do_clean() {
  step "clean — deleting this door's world"
  refuse_protected_world
  if [ "$DRY" = 1 ]; then
    dry "would stop the broker and the control plane, then rm -rf $WORLD"
    return 0
  fi
  [ -d "$WORLD" ] || { note "nothing to remove ($WORLD does not exist)"; return 0; }
  [ -f "$MARKER" ] || die "$WORLD carries no lane-test-door marker — refusing to delete it"
  do_stop
  rm -rf "$WORLD"
  ok "removed $WORLD"
}

# ─── driver ─────────────────────────────────────────────────────────────────
run_step() {
  case "$1" in
    preflight) do_preflight ;;
    world)     do_world ;;
    broker)    do_broker ;;
    keys)      do_keys ;;
    plane)     do_plane ;;
    budget)    do_budget ;;
    walk)      do_walk ;;
    stop)      do_stop ;;
    clean)     do_clean ;;
    *) die "unknown step '$1' (steps: preflight world broker keys plane budget walk · verbs: stop clean)" ;;
  esac
}

main() {
  local only=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --dry-run) DRY=1 ;;
      -h|--help) sed -n '2,80p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
      -*) die "unknown option '$1'" ;;
      *) only="$1" ;;
    esac
    shift
  done

  printf '\n\033[1mLANE-TEST DOOR — cold to a clickable lane test\033[0m\n'
  if [ "$DRY" = 1 ]; then
    printf '\033[36mDRY RUN\033[0m: nothing is built, written, started or dialled. Every path,\n'
    printf 'person, port and command below is the one a real run would use.\n'
  fi
  printf 'world:  %s\n' "$WORLD"
  printf 'person: %s   (broker store %s/%s, socket %s)\n' "$PERSON" "$STORE_ROOT" "$PERSON" "$SOCK"
  printf 'url:    %s\n' "$BASE"

  if [ -n "$only" ]; then
    run_step "$only"
  else
    for s in preflight world broker keys plane budget walk; do
      run_step "$s"
    done
  fi
  printf '\n'
  [ "$fail" = 0 ]
}

main "$@"
