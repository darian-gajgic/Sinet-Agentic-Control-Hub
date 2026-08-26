#!/usr/bin/env bash
# P3/gates/lane-test-door.sh — P3-LN-5, extended at P3-LN-8: the lane-test door.
#
# ONE COMMAND, cold to a CLICKABLE lane test:
#
#     ./P3/gates/lane-test-door.sh
#
# It builds a THROWAWAY world of its own, installs the PINNED Kimi CLI into that
# world so the kimi-cli lane runs the version this tree claims, starts the
# per-person broker daemon that spawn-time credential injection dials, hands you
# the key ceremony pointed at that world, starts the world's control plane AFTER
# the keys exist, shows you the commissioning line the control plane printed
# about itself, declares the automation budget that makes the commissioned
# lanes' consumption pressure comparable, and then tells you the exact URL, the
# exact sign-in, what to click and where the lane shows.
#
# SINCE P3-LN-8 IT ANSWERS A SECOND QUESTION. The operator holds ONE Kimi Code
# membership that TWO paths reach: lane `kimi` (the API endpoint, on the
# opencode substrate) and lane `kimi-cli` (Moonshot's own pinned CLI, on the
# kimi-cli substrate). Which one performs better is not answerable from a
# document — it is answerable by running comparable work down both and reading
# the receipts. The walk below is that recipe, and it is honest about the one
# thing that cannot be compared: they draw a SINGLE quota pool, so consumption
# figures are within-pool and only duration, token counts and outcome quality
# separate the two paths.
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
# Steps: preflight  world  engine  broker  keys  plane  budget  walk
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
#      the `budget` step below drives it for the person you sign in as. It
#      drives the sibling token verb for `anthropic` too, because comparability
#      is ALL-OR-NOTHING: one covered lane without a denominator ends the
#      comparison for every lane, and anthropic is covered whether or not this
#      door commissioned it.
#   5. The kimi-cli engine binary is resolved BY PATH, and a version match is
#      not a distribution match. The adapter sets no explicit path
#      (internal/shell/engineadapters.go composes kimicli.Adapter without a
#      Binary), so it spawns whatever `kimi` the CONTROL PLANE's PATH finds, and
#      that process's environment is the ambient one minus the KIMI_* scrub —
#      PATH is not scrubbed. This host carries the vendor's `curl | bash`
#      installer build inside the operator's own ~/.kimi-code as well as the npm
#      package this tree pins; both answer --version with the same string and
#      only one of them is the pinned artefact (§67). So the `engine` step
#      installs the pinned npm package into a DOOR-OWNED prefix inside the
#      throwaway world and the `plane` step prepends that prefix's bin
#      directory. The operator's own install is never used and never touched.
#   6. THE TWO KIMI LANES ARE ONE SUBSCRIPTION. `kimi` and `kimi-cli` name the
#      same broker auth-profile (`kimi-code`), so ONE paste commissions both,
#      and they name the same quota pool (`kimi-code-membership`), so their
#      consumption is SUMMED and the plan budget is declared ONCE against the
#      canonical lane `kimi`. Declaring it a second time on `kimi-cli` is
#      REFUSED by the platform, at the store, at the HTTP boundary and at the
#      reading — the budget step shows you that refusal in the platform's own
#      words, because a rule you can see enforced is worth more than a rule
#      this script asserts.
#
# WHAT IT NEVER TOUCHES. Production (:8481, :8482, /var/lib/sinet, /etc/sinet,
# the installed binary), this host's real broker store (~/.local/state/sinet),
# the operator's own Kimi CLI data root ~/.kimi-code — which holds live
# credentials — and the binary inside it, the operator's live gate world
# (~/.sinet-b6-clickthrough) and every other
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

# ─── the kimi-cli engine, pinned, inside this world ─────────────────────────
# The npm package is installed with `npm install -g --prefix ENGINE_PREFIX`, so
# it lands entirely inside the throwaway world and `clean` takes it with the
# rest. The CLI flag beats any prefix an .npmrc sets, which is what keeps this
# out of the operator's own global prefix.
ENGINE_PKG="@moonshot-ai/kimi-code"
ENGINE_PREFIX="$WORLD/engine-npm"
ENGINE_BINDIR="$ENGINE_PREFIX/bin"
# adapters/kimicli.DefaultBinary — the name the adapter resolves through PATH.
ENGINE_BIN="$ENGINE_BINDIR/kimi"
# A bounded HOME and data root for the door's OWN --version probe, so even the
# version print cannot read or write the operator's ~/.kimi-code. It mirrors
# what the adapter does at spawn (lower.go loweredEnv: HOME and KIMI_CODE_HOME
# are both moved, and KIMI_CODE_NO_AUTO_UPDATE=1 is set on every invocation).
ENGINE_PROBE_HOME="$WORLD/engine-probe"
# Where the running world puts each person's engine subtree, and each run's own
# KIMI_CODE_HOME beneath that: shell.kimicliRoot(stateDir).
ENGINE_ROOT="$WORLD/engines/kimi-cli"
# The pin, read out of the Go source that the components.lock entry is coupled
# to by TestPinMatchesLock. Never written down here: a second spelling of a pin
# is a pin that drifts.
ENGINE_PIN_SRC="$REPO/internal/adapters/kimicli/lower.go"

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

# The lane the duty map seats by default, and the one this door never
# commissions. It is COVERED all the same: internal/stage composes coverage as
# [the configured lane] + [the commissioned lanes], and the configured lane
# defaults to adapters.LaneAnthropic (internal/stage/skeleton.go) with nothing
# in the shell overriding it. It is metered in weighted TOKENS rather than plan
# units, so its budget is the sibling verb's — and until it has one, the whole
# comparison is off (see declare_token_budget).
BASE_LANE="anthropic"
# The TEST-WORLD token budget. This door mints no figure of its own: this is the
# one the Fleet page's own budget editor is exercised with (web/src/fleet.test.tsx
# — person alice, lane anthropic, period_tokens 250000, period_days 30). It is
# assumed, it is editable in the UI, and it means nothing outside a throwaway
# world; the step says all three out loud every time it runs.
BASE_TOKENS=250000
BASE_DAYS=30

# The lanes this door places keys for and verifies. THREE lanes, TWO pastes:
# `kimi` and `kimi-cli` name the same broker auth-profile, so the single Kimi
# paste lights both. The ceremony's own key steps are unchanged in count.
LANES="zai kimi kimi-cli"
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
# The broker auth-profile a lane's credential is placed under, and the pool its
# allowance belongs to — read out of the document rather than written down, so
# the "one profile, two lanes" and "one pool, two lanes" facts this door prints
# are the platform's own and cannot drift from it.
lane_profile() { jq -r '.credential.profile // ""' "$(lane_doc "$1")" 2>/dev/null || true; }
lane_pool()    { jq -r '.pool // ""'                "$(lane_doc "$1")" 2>/dev/null || true; }

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
  step "1/8  Preflight — what this door will and will not touch"

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

  # The kimi-cli lane's engine is an npm-published CLI that runs on Node, so
  # both are hard requirements now rather than SPA-build conveniences. The
  # engine step installs the pin; WHICH Node version it needs is the package's
  # own declaration and is enforced by npm at install time (--engine-strict),
  # never by a figure written down here.
  command -v npm >/dev/null || die "npm is not on PATH — the engine step installs the pinned $ENGINE_PKG with it"
  ok "npm present: $(npm --version)"
  command -v node >/dev/null || die "node is not on PATH — the pinned Kimi CLI is a Node program and cannot run without it"
  ok "node present: $(node --version)"

  local pin
  pin="$(engine_pin)" || die "could not read the kimi-cli engine pin out of $ENGINE_PIN_SRC"
  ok "kimi-cli engine pin: $ENGINE_PKG@$pin   (read from $ENGINE_PIN_SRC, the constant components.lock is coupled to)"
  info "The engine step installs THAT version into $ENGINE_PREFIX and the plane"
  info "step puts $ENGINE_BINDIR at the front of the control plane's PATH."
  if command -v kimi >/dev/null 2>&1; then
    info "your own 'kimi' on PATH is $(command -v kimi) — NOT used by this world, and not touched"
  fi

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
  step "2/8  A fresh world — binary, SPA, seeded demo data"

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

# ─── 3. engine ──────────────────────────────────────────────────────────────

# engine_pin prints the kimi-cli engine pin, read out of the Go constant the
# components.lock entry is mechanically coupled to (TestPinMatchesLock). The
# ceremony reads opencode.Pin the same way and for the same reason: a pin
# spelled twice is a pin that drifts.
engine_pin() {
  local v
  v="$(grep -E '^const Pin = ' "$ENGINE_PIN_SRC" 2>/dev/null | head -1 | cut -d'"' -f2)"
  [ -n "$v" ] || return 1
  printf '%s' "$v"
}

# engine_version runs the door's OWN pinned shim under a bounded HOME and a
# bounded data root, with auto-update off. Even a version print gets the belt:
# the operator's ~/.kimi-code holds live credentials, and a probe that reads it
# is a probe that had no business existing. This mirrors the adapter's own
# spawn environment (kimicli/lower.go loweredEnv).
engine_version() {
  env HOME="$ENGINE_PROBE_HOME" \
      KIMI_CODE_HOME="$ENGINE_PROBE_HOME/kimi-code" \
      KIMI_CODE_NO_AUTO_UPDATE=1 KIMI_DISABLE_TELEMETRY=1 NO_COLOR=1 CI=1 \
      "$ENGINE_BIN" --version 2>/dev/null | tail -1 | tr -d '[:space:]'
}

do_engine() {
  step "3/8  The pinned Kimi CLI — the kimi-cli lane's engine, inside this world"

  local pin
  pin="$(engine_pin)" || die "could not read the engine pin out of $ENGINE_PIN_SRC"

  info "The kimi-cli adapter names NO path for its engine: it spawns whatever"
  info "'kimi' the CONTROL PLANE's PATH resolves (kimicli.DefaultBinary), and the"
  info "spawn environment is the ambient one minus the KIMI_* scrub — PATH is not"
  info "scrubbed. So whichever 'kimi' this door's control plane can see is the one"
  info "your lane test measures, and a version match is NOT a distribution match:"
  info "the vendor's curl-installer build and the npm package this tree pins both"
  info "answer --version identically and only one of them is the pinned artefact."
  info "This step installs the pin into a door-owned prefix; the plane step puts"
  info "that prefix first on PATH. Your own install is never used and never touched."

  if [ "$DRY" = 1 ]; then
    dry "would install, user-level, into this door's own world:"
    dry "  npm install -g --engine-strict --prefix $ENGINE_PREFIX $ENGINE_PKG@$pin"
    dry "  --prefix beats any prefix an .npmrc sets, so nothing lands in your global npm root"
    dry "  --engine-strict makes npm enforce the PACKAGE's own Node requirement (no version written down here)"
    dry "would skip the install when $ENGINE_BIN already answers $pin"
    dry "would then verify the version, under a bounded HOME and data root:"
    dry "  HOME=$ENGINE_PROBE_HOME KIMI_CODE_HOME=$ENGINE_PROBE_HOME/kimi-code KIMI_CODE_NO_AUTO_UPDATE=1 $ENGINE_BIN --version"
    dry "would require it to print exactly $pin, and would print the file it resolves to"
    dry "would NEVER read, write or run anything under $HOME/.kimi-code"
    return 0
  fi

  [ -d "$WORLD" ] || die "the world does not exist yet — run: $ENVP$SELF world"
  refuse_protected_world
  mkdir -p "$ENGINE_PROBE_HOME/kimi-code"

  local have=""
  [ -x "$ENGINE_BIN" ] && have="$(engine_version || true)"
  if [ "$have" = "$pin" ]; then
    ok "the pinned engine is already installed in this world: $ENGINE_PKG@$pin"
  else
    [ -n "$have" ] && note "the copy in this world answers '$have', not the pin '$pin' — reinstalling"
    info "installing $ENGINE_PKG@$pin into $ENGINE_PREFIX (this is a normal npm fetch, no provider call)"
    if ! npm install -g --engine-strict --prefix "$ENGINE_PREFIX" "$ENGINE_PKG@$pin" \
         >"$WORLD/engine-install.log" 2>&1; then
      bad "the engine install failed — see $WORLD/engine-install.log"
      tail -15 "$WORLD/engine-install.log" | sed 's/^/        /'
      die "could not install $ENGINE_PKG@$pin"
    fi
    ok "installed $ENGINE_PKG@$pin into $ENGINE_PREFIX"
    # npm's own last line, whatever it chose to say ("added 6 packages", "changed
    # 6 packages" on a re-point). Printed rather than parsed: the door has no
    # business re-wording a tool's summary, and the version check below is what
    # actually decides.
    info "npm: $(grep -E 'packages' "$WORLD/engine-install.log" | tail -1 | sed 's/^ *//')"
  fi

  [ -x "$ENGINE_BIN" ] || die "the install left no executable at $ENGINE_BIN"

  local got
  got="$(engine_version)"
  [ "$got" = "$pin" ] \
    || die "the engine in this world answers '$got' but this tree pins '$pin' ($ENGINE_PIN_SRC). Refusing — a lane test on an unpinned engine measures something nobody can reproduce."
  ok "$ENGINE_BIN --version prints $got, which IS the pin ($ENGINE_PIN_SRC)"
  info "it resolves to $(readlink -f "$ENGINE_BIN" 2>/dev/null || printf '%s' "$ENGINE_BIN")"
  info "that is the npm package tree — the distribution components.lock records"
  ok "the version print ran under HOME=$ENGINE_PROBE_HOME, so $HOME/.kimi-code was never read"

  if command -v kimi >/dev/null 2>&1; then
    local ambient; ambient="$(command -v kimi)"
    if [ "$ambient" != "$ENGINE_BIN" ]; then
      note "your shell's own 'kimi' is $ambient and this world will NOT use it."
      info "The plane step puts $ENGINE_BINDIR ahead of it on the control plane's"
      info "PATH, so the lane runs the pin above. Nothing about your install changes."
    fi
  fi
}

# ─── 4. broker ──────────────────────────────────────────────────────────────
do_broker() {
  step "4/8  The broker daemon for $PERSON, inside this world"

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

# ─── 5. keys ────────────────────────────────────────────────────────────────
do_keys() {
  step "5/8  Key placement — the ceremony, pointed at this world and this person"

  info "The ceremony is handed two things and changes nothing else:"
  info "  STATE_DIRECTORY=$WORLD      -> store $STORE_ROOT/$PERSON"
  info "  SINET_CEREMONY_USER=$PERSON -> the person you will SIGN IN as"
  info "It prompts for each key with 'read -rs' and hands it to the placement"
  info "tool on stdin. This door never sees, logs or stores a key."
  printf '\n'
  note "TWO PASTES, THREE LANES — and the count of pastes did not change."
  info "The ceremony asks for a Z.AI key and a Kimi key, exactly as before. But"
  info "'kimi' and 'kimi-cli' name the SAME broker auth-profile ('kimi-code'),"
  info "because one Kimi Code membership is one Console key. Placing it once"
  info "commissions both documents, and the platform then delivers that one"
  info "secret under each lane's OWN variable at spawn — KIMI_API_KEY for the"
  info "API lane, KIMI_MODEL_API_KEY for the CLI, which reads no other name."
  info "The verification below therefore checks all three lanes through the live"
  info "socket, and the plane step shows the control plane naming them itself."

  if [ "$DRY" = 1 ]; then
    dry "would run, one step at a time (each tolerated to fail — a lane you have no key for must not stop the door):"
    dry "  STATE_DIRECTORY=$WORLD SINET_CEREMONY_USER=$PERSON $CEREMONY preflight"
    dry "  STATE_DIRECTORY=$WORLD SINET_CEREMONY_USER=$PERSON $CEREMONY zai-key"
    dry "  STATE_DIRECTORY=$WORLD SINET_CEREMONY_USER=$PERSON $CEREMONY kimi-key"
    dry "would then verify each of $LANES through the LIVE socket: $LANEKEY verify --socket $SOCK ..."
    dry "  the single Kimi paste is expected to resolve BOTH kimi lanes — they share one auth profile"
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
      ok "$lane: resolves through the LIVE broker socket under profile '$(lane_profile "$lane")' — the same socket a spawn dials"
      placed=$((placed+1))
    else
      note "$lane: no credential resolves for $PERSON (not placed, or declined)"
    fi
  done
  [ "$placed" -gt 0 ] || die "no lane is placed for '$PERSON', so there is nothing to click through. Re-run: $ENVP$SELF keys"
  ok "$placed lane(s) placed and live-resolvable for $PERSON"
}

# ─── 6. plane ───────────────────────────────────────────────────────────────
do_plane() {
  step "6/8  The control plane — started AFTER the keys, because commissioning is startup-bound"

  info "Coverage, the lane-to-substrate map, the alternate seats and the credential"
  info "injector are all composed once, at startup, from what is placed in each"
  info "person's broker store. A key placed later is picked up at the NEXT start,"
  info "which is why this step always restarts rather than reusing."
  info "It is started with $ENGINE_BINDIR at the FRONT of PATH, because the"
  info "kimi-cli adapter resolves its engine by name through the environment this"
  info "process passes down — so the PATH of the control plane decides which"
  info "'kimi' every run on that lane executes."

  if [ "$DRY" = 1 ]; then
    dry "would stop any control plane this door had running"
    dry "would start, with the pinned engine first on PATH:"
    dry "  PATH=$ENGINE_BINDIR:\$PATH $BIN control --state-dir $WORLD --config-dir $WORLD --http-addr 127.0.0.1:$PORT"
    dry "would refuse to start at all if $ENGINE_BIN is missing — see the engine step"
    dry "would wait for $BASE/api/health"
    dry "would re-check, while it runs, that production still holds 8481/8482 and its units are active"
    dry "would grep 'lanes: commissioned' out of $CONTROL_LOG and SHOW it as the proof"
    dry "would fail if that line does not name '$PERSON' with at least one lane"
    return 0
  fi

  [ -x "$BIN" ] || die "the world is not built yet — run: $ENVP$SELF world"
  [ -S "$SOCK" ] || die "the broker is not running — run: $ENVP$SELF broker"
  # Refused rather than degraded: without this the plane would silently resolve
  # whatever 'kimi' the ambient PATH offers — on this host, the installer build
  # inside the operator's own credential-holding data root — and every kimi-cli
  # measurement would be about a binary nobody pinned.
  [ -x "$ENGINE_BIN" ] \
    || die "the pinned kimi-cli engine is not installed in this world ($ENGINE_BIN) — run: $ENVP$SELF engine"

  if alive_ours "$CONTROL_PID_FILE" "control --state-dir $WORLD"; then
    reap "$CONTROL_PID_FILE" "control --state-dir $WORLD" "the previous control plane"
  fi

  PATH="$ENGINE_BINDIR:$PATH" \
  "$BIN" control --state-dir "$WORLD" --config-dir "$WORLD" --http-addr "127.0.0.1:$PORT" \
    >"$CONTROL_LOG" 2>&1 &
  printf '%s' "$!" > "$CONTROL_PID_FILE"
  ok "the control plane's PATH starts with $ENGINE_BINDIR — kimi-cli runs execute the pin"
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

# ─── 7. budget ──────────────────────────────────────────────────────────────

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

# plan_doc_for prints the plan document that COUNTS lane $1 — its own, or the
# POOLED one whose member list names it. It mirrors metering.PlanDoc.CountsLane,
# which is what every production reader resolves through since P3-LN-7: a lane
# sharing an allowance has no document of its own, and refusing to look for one
# is how `kimi-cli` would be reported as metering in no plan units when it in
# fact spends `kimi`'s. Fails (rc 1) when no shipped document counts the lane.
plan_doc_for() {
  local lane=$1 f
  for f in "$PLANDATA"/*.json; do
    [ -f "$f" ] || continue
    if [ "$(jq -r --arg l "$lane" '((.lane == $l) or ((.pool_lanes // []) | index($l) != null)) | tostring' "$f" 2>/dev/null)" = "true" ]; then
      printf '%s' "$f"
      return 0
    fi
  done
  return 1
}
# The window names a plan document declares. It takes a DOCUMENT PATH, not a
# lane, because a pooled lane's windows are its pool's.
plan_windows() { jq -r '.quotas[]?.name' "$1" 2>/dev/null || true; }

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

# declare_token_budget declares the WEIGHTED-TOKEN budget for the configured
# lane, and it is the act that completes the comparison rather than a tidy extra.
# chooseFlatLane walks EVERY covered flat-rate lane and returns the configured
# order the instant ONE of them cannot be compared — so plan budgets on zai and
# kimi alone change nothing whatever, because anthropic is covered too and had
# no denominator. With this declared, all three are comparable and the
# least-consumed lane actually wins.
declare_token_budget() {
  local body code prior
  body="{\"person\":\"$(json_escape "$PERSON")\",\"lane\":\"$(json_escape "$BASE_LANE")\",\"period_tokens\":$BASE_TOKENS,\"period_days\":$BASE_DAYS,\"reason\":\"lane-test-door budget step: an assumed test-world figure, declared so the configured lane is comparable against the commissioned ones\"}"

  code="$(printf '%s' "$body" | api_call POST /api/meters/budget)"
  case "$code" in
    200) : ;;
    400) bad "$BASE_LANE: the token budget was refused"; wrapped "$(api_detail)"
         die "token-budget declaration refused" ;;
    401) die "the control plane answered 401 — the session did not carry, so nothing was declared for '$PERSON'" ;;
    403) bad "$BASE_LANE: forbidden"; wrapped "$(api_detail)"
         die "the session is not '$PERSON' and is not the operator" ;;
    503) bad "the token-budget verb is not wired in this control plane"; wrapped "$(api_detail)"
         die "POST /api/meters/budget answered 503" ;;
    *)   bad "$BASE_LANE: POST /api/meters/budget answered $code"
         head -c 400 "$API_OUT" 2>/dev/null | sed 's/^/          /'; printf '\n'
         die "token-budget declaration failed" ;;
  esac

  ok "$BASE_LANE: $(jq -r '"\(.budget.period_tokens) \(.budget.unit) over \(.budget.period_days) days"' "$API_OUT")"
  wrapped "$(jq -r '.detail // empty' "$API_OUT" 2>/dev/null || true)"

  note "THAT FIGURE IS ASSUMED, AND IT IS A TEST-WORLD FIGURE."
  info "It is not a platform constant, not a recommendation, and it means nothing"
  info "at all outside this throwaway world. It exists for exactly one reason: the"
  info "comparison needs EVERY covered lane to have a denominator, and $BASE_LANE is"
  info "covered. The number itself is the one the Fleet page's own budget editor is"
  info "exercised with, reused so this door mints no figure of its own."
  info "IT IS YOURS TO CHANGE, in the browser, at any time: $BASE/fleet — the"
  info "budget editor on the $PERSON / $BASE_LANE row. Re-running this step will put"
  info "$BASE_TOKENS back; the Fleet page is the authority on what it ought to be."

  # Same read-back as the plan windows: re-declaring returns the row it REPLACED,
  # read inside the write transaction, so no prior means nothing was stored.
  code="$(printf '%s' "$body" | api_call POST /api/meters/budget)"
  [ "$code" = 200 ] || { bad "$BASE_LANE: re-declaring answered $code"; wrapped "$(api_detail)"; die "re-declare failed"; }
  prior="$(jq -r '.prior // empty | "\(.period_tokens) \(.unit), over \(.period_days) days, declared by \(.declared_by)"' "$API_OUT" 2>/dev/null || true)"
  [ -n "$prior" ] || { bad "$BASE_LANE: re-declaring returned NO prior row, so the first declaration was never stored"; die "the token budget did not land"; }
  ok "$BASE_LANE: re-declared — the verb read back the row it replaced ($prior)"
}

# meters_show_token reads the configured lane back through GET /api/meters. Its
# budget is the TOP-LEVEL one on the row, not a plan block: this lane meters in
# weighted tokens and ships no plan document.
meters_show_token() {
  local lane=$1 code n declared
  code="$(api_get "/api/meters?lane=$lane")"
  [ "$code" = 200 ] || { bad "GET /api/meters?lane=$lane answered $code"; wrapped "$(api_detail)"; die "the meters read failed"; }
  n="$(jq --arg l "$lane" '[.lanes[] | select(.lane == $l)] | length' "$API_OUT" 2>/dev/null || echo 0)"
  if [ "$n" = 0 ]; then
    note "$lane: GET /api/meters serves no row for it yet — the read projects its"
    info "(person, lane) rows from the RUNS table, so a lane $PERSON has not run"
    info "work on has none. The proof the row landed is the read-back above."
    return 0
  fi
  printf '\n  \033[1mGET /api/meters?lane=%s — the token budget, as the platform serves it:\033[0m\n\n' "$lane"
  jq --arg l "$lane" '.lanes[] | select(.lane == $l)
     | {owner, lane, weighted_consumption, budget_declared, pressure_applicable, pressure, budget_remaining}' \
     "$API_OUT" | sed 's/^/        /'
  declared="$(jq -r --arg l "$lane" '[.lanes[] | select(.lane == $l) | .budget_declared][0] // false' "$API_OUT" 2>/dev/null || true)"
  if [ "$declared" = "true" ]; then
    ok "$lane: the meters row carries a declared token budget — this lane is comparable now"
  else
    bad "$lane: the meters row exists and carries NO declared budget"
    die "the declaration did not reach the reading"
  fi
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

# pool_sibling handles a commissioned lane whose allowance is DECLARED ON
# ANOTHER LANE — the P3-LN-7 pool. It declares nothing. It states what the pool
# means for this lane, and then it ASKS THE PLATFORM to declare a second row and
# shows you the refusal in the platform's own words, because a rule you can see
# enforced is worth more than a rule this script asserts. Nothing is stored: the
# refusal is the whole point, and it is a feature — two rows against one
# allowance is a number nobody can reconcile, and either could bind the lane.
pool_sibling() {
  local lane=$1 doc canonical pool members window body code detail
  doc="$(plan_doc_for "$lane")" || return 0
  canonical="$(jq -r '.lane' "$doc")"
  pool="$(jq -r '.pool // ""' "$doc")"
  members="$(jq -r '(.pool_lanes // []) | join(" + ")' "$doc")"
  window="$(plan_windows "$doc" | head -1)"

  printf '\n'
  note "$lane: ONE ALLOWANCE, TWO LANES — and it is already declared."
  info "'$members' draw the shared '$pool' allowance between them. Consumption is"
  info "SUMMED across both and divided by the single denominator declared above on"
  info "'$canonical', so '$lane' is denominated and comparable already. Declaring"
  info "again here would put two independent rows against one allowance."
  info "The lane documents state the same fact on their own side, so it is one"
  info "reading rather than this script's opinion: $(lane_pool "$lane")"

  [ -n "$window" ] || { note "$lane: the pooled document declares no window to demonstrate the refusal with"; return 0; }

  body="{\"person\":\"$(json_escape "$PERSON")\",\"lane\":\"$(json_escape "$lane")\",\"window\":\"$(json_escape "$window")\",\"from_proposal\":true,\"reason\":\"lane-test-door: demonstrating that a pooled allowance is declared once\"}"
  code="$(printf '%s' "$body" | api_call POST /api/meters/plan-budget)"
  detail="$(api_detail)"
  case "$code" in
    400)
      case "$detail" in
        *"declared ONCE against lane"*)
          ok "$lane: the platform REFUSED a second declaration, in its own words:"
          wrapped "$detail"
          info "That is metering.PlanPoolRefusal, enforced at the store, at this HTTP"
          info "boundary and again at the reading. Nothing was written." ;;
        *)
          bad "$lane: the declaration was refused, but not by the pool rule:"
          wrapped "$detail"
          die "unexpected refusal on a pooled lane" ;;
      esac ;;
    200)
      bad "$lane: the platform ACCEPTED a second budget row against the '$pool' allowance."
      info "That is the state the pool rule exists to prevent: two independent rows"
      info "against one allowance, either of which could bind the lane under"
      info "max-binds, with no way to explain why a number nobody touched moved."
      die "the pooled-declaration refusal did not hold — this is a platform defect, not a door failure" ;;
    401)
      die "the control plane answered 401 — the session did not carry" ;;
    *)
      bad "$lane: POST /api/meters/plan-budget answered $code on the pooled sibling"
      wrapped "$detail"
      die "pooled-sibling probe failed" ;;
  esac

  meters_show "$lane"
}

do_budget() {
  step "7/8  The plan budget — what makes the commissioned lane's pressure comparable"

  info "A commissioned lane is a lane the router MAY pick, not one it does. It picks"
  info "between covered flat-rate lanes on consumption PRESSURE, and a pressure needs"
  info "a declared denominator: the S10.4 automation budget, at the (person, lane,"
  info "window) grain, in the WINDOW's own unit and never in dollars (D5)."
  info "This step declares one for $PERSON on every commissioned lane, taking the"
  info "platform's own PROPOSAL rather than inventing a figure — and then one on"
  info "$BASE_LANE, which this door does not commission but which is covered all the"
  info "same. A comparison needs EVERY covered lane to have a denominator: one that"
  info "cannot be compared ends the comparison for all of them."
  info "ONE EXCEPTION, AND IT IS THE POINT OF THE POOL: lanes that share a single"
  info "allowance get ONE row, declared against the pool's canonical lane. The"
  info "two kimi lanes are one Kimi Code membership, so 'kimi' carries the row and"
  info "'kimi-cli' reads through it — and the step shows you the platform refusing"
  info "a second declaration rather than taking this script's word for it."

  if [ "$DRY" = 1 ]; then
    dry "would read the lanes commissioned FOR $PERSON out of the per_person field of the control plane's own line in $CONTROL_LOG"
    dry "would resolve each lane to the plan document that COUNTS it in $PLANDATA — its own, or the POOLED one whose member list names it (the same files the binary embeds), and read that document's window names"
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
    dry "would then handle every POOLED sibling — a commissioned lane whose allowance another lane's row already declares:"
    dry "  it declares NOTHING there, and instead POSTs one deliberate second declaration to SHOW the platform refuse it,"
    dry "  expecting 400 with metering.PlanPoolRefusal's own sentence naming the canonical lane (nothing is stored),"
    dry "  and would treat a 200 as a PLATFORM DEFECT — two rows against one allowance — rather than a door failure"
    dry "would then declare the TOKEN budget for the covered-but-uncommissioned lane, through the sibling verb:"
    dry "  POST $BASE/api/meters/budget"
    dry "  {\"person\":\"$PERSON\",\"lane\":\"$BASE_LANE\",\"period_tokens\":$BASE_TOKENS,\"period_days\":$BASE_DAYS,\"reason\":\"...\"}"
    dry "  an ASSUMED test-world figure — the one the Fleet page's budget editor is exercised with — that exists only so $BASE_LANE is comparable; editable at $BASE/fleet and meaningless outside this world"
    dry "would read it back the same way (re-declare for the prior row, then GET $BASE/api/meters?lane=$BASE_LANE)"
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

  # TWO PASSES, and the order is load-bearing. A POOLED allowance is declared
  # ONCE against its canonical lane, and a sibling reads THROUGH that one row —
  # so the siblings are handled second, after the row they read exists. Doing it
  # in one pass would report a sibling as undeclared for no reason but ordering.
  local lane doc canonical window windows declared=0 skipped=0 pooled=""
  for lane in $lanes; do
    printf '\n'
    if ! doc="$(plan_doc_for "$lane")"; then
      note "$lane: no shipped plan document counts it, so it carries no PLAN budget —"
      info "its automation budget is the weighted-consumption one (POST /api/meters/budget)"
      continue
    fi
    canonical="$(jq -r '.lane' "$doc")"
    if [ "$canonical" != "$lane" ]; then
      # A pooled sibling. Handled in pass 2 so the canonical row is in first.
      pooled="$pooled $lane"
      note "$lane: it shares '$canonical's allowance — held for the pooled pass below"
      continue
    fi
    windows="$(plan_windows "$doc")"
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

  # Pass 2: every commissioned lane that draws an allowance somebody else's row
  # already declared.
  for lane in $pooled; do
    pool_sibling "$lane"
  done

  # And now the lane this door does NOT commission, without which none of the
  # above changes a single routing decision.
  printf '\n'
  declare_token_budget
  meters_show_token "$BASE_LANE"

  printf '\n'
  ok "every covered flat-rate lane now has a declared denominator: $lanes (plan units${pooled:+, ${pooled# } through the pool}) and $BASE_LANE (weighted tokens)"
  info "That is what selection needs: it now runs a REAL comparison and says so in"
  info "the approval card, instead of falling back for want of a denominator."
  info "It takes the strictly LESS consumed lane. On a world this fresh every lane"
  info "sits at 0%, so the first task ties and the tie goes to the configured order"
  info "($BASE_LANE first) — the commissioned lane takes over once $BASE_LANE has"
  info "spent the larger share of its own budget. Watch it over several tasks."
  if [ -n "$pooled" ]; then
    local sib canon
    sib="${pooled# }"; sib="${sib%% *}"
    canon="$(jq -r '.lane' "$(plan_doc_for "$sib")" 2>/dev/null || true)"
    printf '\n'
    note "AND ONE CONSEQUENCE OF THE POOL THAT THE WALK EXPLAINS IN FULL:"
    info "one pool means one number, so '$canon' and '$sib' report the IDENTICAL"
    info "pressure ratio at every moment. Selection takes the strictly less"
    info "consumed lane and keeps the earlier candidate on a tie, so between those"
    info "two the earlier one wins always. Read the walk's head-to-head section"
    info "before you read a run of '$canon' receipts as a verdict about either."
  fi
  note "Re-run '$ENVP$SELF budget' whenever a period has ended. Nothing rolls one"
  info "over — re-declaring IS the act that starts the next period."
}

# ─── 8. walk ────────────────────────────────────────────────────────────────
do_walk() {
  step "8/8  What to open, who to be, what to click"

  if [ "$DRY" = 1 ]; then
    dry "would read the people out of the RUNNING world: GET $BASE/api/auth/users"
    dry "would read the PIN out of $SEED_LOG (the seed's own printed line)"
    dry "would read the lanes commissioned for $PERSON, and the pool facts, out of the plan document that counts them"
    dry "would print the URL, the sign-in, the click path, the kimi-vs-kimi-cli head-to-head and the honest notes"
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

  # The head-to-head's facts, derived rather than written down. The candidate
  # ORDER is the thing the comparison turns on, and it is not a preference: the
  # duty-map seat's lane comes first and the commissioned lanes follow it sorted
  # by lane name (opencode.loadLaneDocs returns them "SORTED BY LANE NAME";
  # CommissionedSeats walks them in that order; worker.chooseFlatLane builds
  # `covered` as the seat then the alternates).
  local mine kimi_doc kimi_canon kimi_pool kimi_members kimi_model candidates
  mine="$(person_lanes)"
  kimi_doc="$(plan_doc_for "$(printf '%s' "$mine" | tr ' ' '\n' | grep -x 'kimi-cli' || true)" 2>/dev/null || true)"
  if [ -n "$kimi_doc" ]; then
    kimi_canon="$(jq -r '.lane' "$kimi_doc")"
    kimi_pool="$(jq -r '.pool // ""' "$kimi_doc")"
    kimi_members="$(jq -r '(.pool_lanes // []) | join(" and ")' "$kimi_doc")"
    # Both kimi documents front one model; the walk says so, and reads it off
    # the CLI document rather than repeating an id this tree already dates.
    kimi_model="$(jq -r '.default_model // ""' "$(lane_doc kimi-cli)" 2>/dev/null || true)"
  fi
  candidates="$BASE_LANE$(printf '%s' "$mine" | tr ' ' '\n' | sort | sed 's/^/, /' | tr -d '\n')"

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
      AFTER A GREEN BUDGET STEP YOU SHOULD GET THE SECOND ONE. Every covered
      lane has a declared denominator now, so the router has a real comparison
      to make and says which lane it picked and how consumed it was. The first
      sentence is what you saw BEFORE the budget step, and if it comes back it
      names the lane that lost its denominator — almost always a period that
      has ended, which is one '$ENVP$SELF budget' away.
   b. $BASE/tasks/<your task>
      The receipt table under the run: Purpose · Priced · Calls · Model · Lane ·
      Unpriced calls. The Lane column is the lane the run actually used, and it
      is the ONLY place that says so per call rather than per decision.
      Above the table, the run's own card carries the two figures a comparison
      is actually made of: a TOKEN count, and how long it ran — "ran for …,
      from its first record to its last" once the run has finished (a live run
      shows elapsed-since-created instead, which is a different quantity).
   c. $BASE/fleet
      Per-person, per-lane meters: Whose · Lane · Open runs · Parked · Until,
      and consumption / pressure / budget remaining. A lane gets a row here once
      work has RUN on it — not when a key is placed.
   d. $BASE/workforce
      Per routed run: model · lane · effort, with the routing reason. The closest
      thing to a "which lane ran it" audit surface.

  $(printf '\033[1mTHE HEAD-TO-HEAD: kimi (API) vs kimi-cli (CLI) — READ BEFORE YOU JUDGE\033[0m')

  You hold ONE Kimi Code membership that two paths reach, and the question this
  door was extended to answer is which path performs better. Here is the whole
  truth about what you can and cannot measure today.

  WHAT THIS DOOR HAS ALREADY PROVEN about the CLI path:
    · the pinned engine is installed in this world and answers the pin exactly
      (the engine step above), and the control plane runs with it first on PATH,
      so a kimi-cli run executes THAT binary and not your own install;
    · your one Kimi paste commissioned BOTH lanes — the same auth profile — and
      the control plane's own startup line names them: ${commissioned:-none};
    · both are denominated: ${kimi_members:-the two kimi lanes} draw the single
      "${kimi_pool:-shared}" allowance, declared once against ${kimi_canon:-kimi}.

  WHAT YOU CANNOT DO, AND IT IS BETTER TO KNOW IT NOW:
  There is no way to aim a task at a named lane. No form field, no button, no
  URL, no environment variable and no CLI flag anywhere in this platform takes a
  lane from you; every "lane" you can see in the UI is routing READING BACK a
  decision it already made. The benchmark's direct-arm path is not a way in
  either — its substrate and lane are compiled constants (claude-cli/anthropic).

  AND ROUTING WILL NOT DO IT FOR YOU. The arithmetic, in full, because the
  conclusion is uncomfortable and you should be able to check it:
    · candidates for this duty, in order: ${candidates:-$BASE_LANE} — the duty
      seat's lane first, then the lanes commissioned for you sorted by NAME;
    · selection takes the STRICTLY less consumed lane, and a tie keeps the
      earlier candidate;
    · ${kimi_members:-the two kimi lanes} share one pool, so their consumption is
      SUMMED and divided by the SAME single budget. Their pressure ratios are
      not merely close — they are the same number, always, by construction;
    · "kimi" sorts before "kimi-cli".
  Put together: kimi-cli ties kimi at every moment and loses every tie. It can
  never be picked while kimi is commissioned, and the two commission from one
  credential, so you cannot hold one without the other. Run ten tasks and you
  will get ten receipts reading anthropic, kimi or zai, and not one reading
  kimi-cli. That is the platform behaving exactly as designed — and reporting it
  as a broken lane would be reporting the wrong defect.

  SO WHAT IS WORTH DOING IN THIS SITTING:
   1. Run several tasks and watch the Lane column alternate across anthropic,
      kimi and zai as consumption builds. That is the LN-6 pressure mechanism
      working, and it is real: it is what a lane winning looks like.
   2. Read the kimi receipts for the two figures a path comparison is made of —
      the token count and the "ran for" duration on the task card — and keep
      them. They are the API path's half of the head-to-head, measured on real
      work rather than on a smoke test.
   3. Do NOT read the absence of kimi-cli receipts as a result about the CLI.
      It is a result about routing.

  WHAT WOULD MAKE THE COMPARISON RUNNABLE — named, so nobody re-derives it:
  a per-task lane pin (an operator surface that puts a lane on the request, the
  natural home being the settings/approval path), OR routing treating pooled
  siblings as separable candidates instead of two names for one ratio. Neither
  exists today. This door will not fake one: a recipe that quietly produced
  kimi receipts and called them a comparison would be worse than no recipe.

  AND WHEN IT DOES BECOME RUNNABLE, READ IT HERE:
    · the receipt Lane column (b) tells you which path actually ran the work;
    · the task card's token count and "ran for" duration are the comparable
      figures — same membership, and both lane documents front the same model
      seat (${kimi_model:-the same default model});
    · CONSUMPTION IS NOT ONE OF THEM. Both lanes draw one pool, so /fleet's
      consumption and pressure figures are WITHIN-POOL: they tell you what the
      membership spent, never which path spent it. Comparing them lane-to-lane
      compares a number against itself.
    · neither lane ships seeded prices, so both render UNPRICED and the Priced
      column is \$0.00 on both. That is not a discount and not a bug.

  $(printf '\033[1mHONEST NOTES — read these before reporting a defect\033[0m')

   · WHAT A GREEN DOOR NOW PROVES, exactly: the key is placed under the person
     you sign in as; it resolves through the LIVE broker socket that a spawn
     dials ($SOCK); the control plane commissioned that lane for that person at
     startup — the line printed above, from the control plane about itself; the
     kimi-cli lane's engine in this world IS the pinned version, verified by its
     own --version under a bounded home and put first on the control plane's
     PATH; and EVERY covered flat-rate lane now carries a DECLARED automation
     budget — the commissioned ones in their plan's own unit at the (person,
     lane, window) grain, pooled lanes through their canonical lane's single
     row, $BASE_LANE in weighted tokens — each read back out of the platform
     after it was written.
   · THE LANE CAN NOW WIN, ON REAL CONSUMPTION. This is new, and it is the whole
     point of the budget step. Selection compares covered flat-rate lanes on
     consumption pressure — consumption over the DECLARED budget, each in its
     own unit — and the LEAST-CONSUMED lane takes the work. Comparability is
     all-or-nothing: one covered lane without a denominator ends the comparison
     for all of them, which is why the budget step declares $BASE_LANE's token
     budget too, not just the commissioned lanes' plan budgets. With all of
     them denominated the router runs a REAL comparison and says so — you get
     the "Chosen among N covered flat-rate lanes on consumption pressure"
     sentence instead of the "no comparable pressure" one. Before P3-LN-6 no
     plan budget could be declared at all, so those lanes never had comparable
     pressure and no comparison happened; that is the sentence this note used
     to carry, and it is retired.
   · YOUR FIRST TASK STILL LANDS ON $BASE_LANE, and that is correct. On a world
     this fresh EVERY lane has consumed nothing, so every ratio is 0% and the
     comparison is a TIE — and the tie goes to the configured duty-map order,
     which seats $BASE_LANE first. The comparison is strict: a lane takes the
     work by being LESS consumed, not equally consumed. So watch the receipt's
     Lane column at $BASE/tasks/<your task> across SEVERAL tasks, not one.
   · AS CONSUMPTION BUILDS, THE LANES ALTERNATE: whichever one sits at the lower
     share of its own budget takes the next task, so once $BASE_LANE has spent
     more of its 250000 than the commissioned lane has of its plan window, the
     commissioned lane takes over — and hands it back when that reverses. That
     is the designed behaviour, not drift, and it is pinned in both directions
     by TestLN6CommissionedLaneWinsAndLosesOnConsumption against the production
     router. Nothing on a running control plane serves the router's would-be
     choice before a task exists, so that test is where the proof lives.
     ALTERNATION DOES NOT REACH INSIDE A POOL. Two lanes sharing one allowance
     report one ratio, so they never alternate with each other, only with the
     lanes outside their pool. The head-to-head section above works that through
     for the two kimi lanes and says what it costs you.
   · THE $BASE_LANE FIGURE IS ASSUMED AND IT IS YOURS. $BASE_TOKENS
     weighted-consumption units over $BASE_DAYS days is a TEST-WORLD number —
     the one the Fleet page's own budget editor is exercised with — declared so
     that lane is comparable at all. It is not a recommendation, and it means
     nothing outside this throwaway world. Change it in the browser at
     $BASE/fleet, on the $PERSON / $BASE_LANE row; re-running the budget step
     puts $BASE_TOKENS back.
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
     is deliberately not part of this door. On the kimi-cli lane there is no
     smoke to fall back on at all: that suite is written to skip even when every
     gate opens, because the FIRST live call on this membership through the CLI
     path was meant to be your own door run, made deliberately. Since routing
     will not send one (see the head-to-head), the CLI path's first live call is
     still ahead of you.
   · YOUR OWN KIMI INSTALL AND ITS DATA ROOT ARE UNTOUCHED. $HOME/.kimi-code
     holds live credentials; nothing in this door reads it, writes it or runs
     the binary inside it. The engine step installed the pin into this world,
     the version probe ran under a bounded HOME, and every kimi-cli RUN gets its
     own KIMI_CODE_HOME under $ENGINE_ROOT with HOME bounded too and
     auto-update off. Those run homes are never deleted by the platform — a
     parked run's home has to outlive its process — so '$ENVP$SELF clean' is
     what removes them, along with the rest of this world.
   · FIRST USE OF THE CLI MAY FETCH TWO HELPER BINARIES. Depending on which
     tools a run is given, the engine downloads and caches ripgrep and fd inside
     its own home. It is conditional rather than certain — the hermetic
     batteries triggered neither — and it lands inside this world, not in your
     home. If a first kimi-cli run is slower than the second, that is the most
     likely reason and it is not the lane being slow.
   · The Kimi C5 no-household-personal-data rider is RECORDED, NOT ENFORCED, on
     BOTH kimi lanes (lanedata/kimi.json and lanedata/kimi-cli.json,
     enforced:false). There is no per-lane data-policy enforcement point
     anywhere in the tree. Do not type personal or household data into a task on
     this world and expect the platform to hold that line.
   · Both kimi lanes carry the same RECORDED GRAY-ZONE Gate-A posture — the
     Community Guidelines' interactive-only clause, and the operator's
     2026-08-26 PROCEED ruling. It is one subscription, so the ruling binds both
     paths equally, and neither lane ever alters the client identity it presents.
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
# engine_strays lists any kimi-cli engine process still running against THIS
# world's engine root. It is a report, not a reap: the door owns two daemons and
# these are neither.
#
# WHY THERE IS NO ENGINE VERB. The kimi-cli substrate adds NO daemon class. It
# owns no standing process and no per-user server — unlike the opencode
# substrate, whose per-user serve manager the control plane starts and stops —
# so a run is one `kimi -p` that lives and dies with its session, which is why
# the platform adds nothing to its own shutdown reap either
# (internal/shell/engineadapters.go). Two daemons in, two daemons out: the stop
# verb is complete as it stands.
#
# What CAN outlive the plane is an in-flight run's own process group, orphaned
# by killing its parent mid-run. That is not a class needing a verb; it is one
# process needing to be seen, so it is named with the exact command to end it
# rather than swept silently. Matched on the run's own KIMI_CODE_HOME, which is
# the only thing that ties a `kimi` process to THIS world.
engine_strays() {
  local pid found=""
  for pid in /proc/[0-9]*; do
    pid="${pid#/proc/}"
    grep -qz "KIMI_CODE_HOME=$ENGINE_ROOT" "/proc/$pid/environ" 2>/dev/null && found="$found $pid"
  done
  printf '%s' "${found# }"
}

do_stop() {
  step "stop — reaping what this door started"
  if [ "$DRY" = 1 ]; then
    dry "would kill the pids recorded in $CONTROL_PID_FILE and $BROKER_PID_FILE"
    dry "  each is checked against /proc/<pid>/cmdline first, so a recycled pid is never killed"
    dry "would then REPORT (never kill) any engine process still running against $ENGINE_ROOT"
    dry "  the kimi-cli substrate starts no daemon of its own, so there is no third verb to add"
    return 0
  fi
  reap "$CONTROL_PID_FILE" "control --state-dir $WORLD" "control plane"
  reap "$BROKER_PID_FILE" "broker --user $PERSON --state-dir $WORLD" "broker"

  local strays
  strays="$(engine_strays)"
  if [ -n "$strays" ]; then
    note "engine processes still running against this world's engine root: $strays"
    info "A kimi-cli run is a child of the control plane, not a daemon, so stopping"
    info "the plane mid-run can leave its process group behind. End them with:"
    info "  kill $strays        (then 'kill -9' the same pids if any survive)"
  else
    ok "no kimi-cli engine process is running against $ENGINE_ROOT"
    info "There is no engine daemon to stop: that substrate owns no standing"
    info "process and no per-user server — a run lives and dies with its session."
  fi
  info "the world is kept at $WORLD — re-run '$ENVP$SELF' to bring it back up"
}

do_clean() {
  step "clean — deleting this door's world"
  refuse_protected_world
  if [ "$DRY" = 1 ]; then
    dry "would stop the broker and the control plane, then rm -rf $WORLD"
    dry "  that takes this door's pinned engine install ($ENGINE_PREFIX) and every run's"
    dry "  engine home ($ENGINE_ROOT) with it; the next run re-installs the pin"
    dry "would REFUSE while an engine process is still running against $ENGINE_ROOT"
    return 0
  fi
  [ -d "$WORLD" ] || { note "nothing to remove ($WORLD does not exist)"; return 0; }
  [ -f "$MARKER" ] || die "$WORLD carries no lane-test-door marker — refusing to delete it"
  do_stop
  # Deleting a world out from under a live engine leaves a process spending the
  # membership with its files gone — the one orphan shape this door can create.
  local strays
  strays="$(engine_strays)"
  [ -z "$strays" ] \
    || die "engine process(es) $strays are still running against $ENGINE_ROOT. Refusing to delete the world under them — end them first: kill $strays"
  rm -rf "$WORLD"
  ok "removed $WORLD (including the pinned engine install and every run's engine home)"
}

# ─── driver ─────────────────────────────────────────────────────────────────
run_step() {
  case "$1" in
    preflight) do_preflight ;;
    world)     do_world ;;
    engine)    do_engine ;;
    broker)    do_broker ;;
    keys)      do_keys ;;
    plane)     do_plane ;;
    budget)    do_budget ;;
    walk)      do_walk ;;
    stop)      do_stop ;;
    clean)     do_clean ;;
    *) die "unknown step '$1' (steps: preflight world engine broker keys plane budget walk · verbs: stop clean)" ;;
  esac
}

main() {
  local only=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --dry-run) DRY=1 ;;
      -h|--help) sed -n '2,118p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
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
    for s in preflight world engine broker keys plane budget walk; do
      run_step "$s"
    done
  fi
  printf '\n'
  [ "$fail" = 0 ]
}

main "$@"
