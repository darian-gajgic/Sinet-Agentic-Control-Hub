#!/usr/bin/env bash
# B5 gate — decision D2: install the two adopted observability organs.
#
# Run it with one command, from anywhere:
#
#     bash ~/Sinet-Agentic-Control-Hub/P3/gates/B5-organ-install.sh
#
# What it does, in order, stopping at the first real failure:
#   1. checks preconditions and prints them
#   2. installs promptfoo at the exact pin 0.121.19        (npm, user-level prefix)
#   3. installs changedetection.io at the exact pin 0.55.8 (uv tool, user-level)
#   4. verifies each installed version against the pin recorded in components.lock
#   5. re-runs the Sinet-side conformance legs that activate once the organ exists
#
# What it deliberately does NOT do:
#   * no sudo, no apt, no system-wide change — both organs install under $HOME
#   * no systemd unit is installed (sinet-watchlist.service stays GENERATED, not
#     installed; that is its own gate decision)
#   * no canary is armed and no secret is read, written or echoed
#   * changedetection.io is installed, NOT started
#
# Re-running is safe: every step is idempotent and re-verifies rather than assuming.

set -u -o pipefail

PROMPTFOO_PIN="0.121.19"
CDIO_PIN="0.55.8"
CDIO_PYTHON="3.11"
REPO="${SINET_REPO:-$HOME/Sinet-Agentic-Control-Hub}"

pass=0; fail=0; skip=0
step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
ok()   { printf '  \033[32mOK\033[0m    %s\n' "$*"; pass=$((pass+1)); }
bad()  { printf '  \033[31mFAIL\033[0m  %s\n' "$*"; fail=$((fail+1)); }
note() { printf '  \033[33mNOTE\033[0m  %s\n' "$*"; skip=$((skip+1)); }
info() { printf '        %s\n' "$*"; }

# ---------------------------------------------------------------- preconditions
step "1/5  Preconditions"

if [ "$(id -u)" = "0" ]; then
  printf '\033[31mRefusing to run as root.\033[0m Both organs install under $HOME; run as your own user.\n'
  exit 2
fi

for tool in node npm uv; do
  if command -v "$tool" >/dev/null 2>&1; then
    ok "$tool present — $("$tool" --version 2>&1 | head -1)"
  else
    bad "$tool is NOT on PATH; this script needs it and will not install it for you"
  fi
done

NPM_PREFIX="$(npm config get prefix 2>/dev/null || true)"
if [ -n "$NPM_PREFIX" ] && [ -d "$NPM_PREFIX/lib/node_modules" ] && [ -w "$NPM_PREFIX/lib/node_modules" ]; then
  ok "npm global prefix is user-writable — $NPM_PREFIX (no sudo needed)"
else
  bad "npm global prefix '$NPM_PREFIX' is not user-writable; fix the prefix rather than reaching for sudo"
fi

if [ -d "$REPO/.git" ]; then
  ok "repo found at $REPO"
else
  bad "repo not found at $REPO (override with SINET_REPO=/path)"
fi

if [ "$fail" -ne 0 ]; then
  printf '\n\033[31mPreconditions failed — nothing was installed.\033[0m\n'
  exit 1
fi

# -------------------------------------------------------------------- promptfoo
step "2/5  promptfoo $PROMPTFOO_PIN  (the pinned regression-eval runner, S14.8)"

if command -v promptfoo >/dev/null 2>&1 && [ "$(promptfoo --version 2>/dev/null | tr -d '[:space:]')" = "$PROMPTFOO_PIN" ]; then
  ok "already installed at the pin — nothing to do"
else
  info "installing: npm install -g promptfoo@$PROMPTFOO_PIN"
  if npm install -g "promptfoo@$PROMPTFOO_PIN" >/tmp/sinet-promptfoo-install.log 2>&1; then
    ok "npm install completed"
  else
    bad "npm install failed — full log at /tmp/sinet-promptfoo-install.log"
    tail -15 /tmp/sinet-promptfoo-install.log | sed 's/^/        /'
  fi
fi

if command -v promptfoo >/dev/null 2>&1; then
  GOT="$(promptfoo --version 2>/dev/null | tr -d '[:space:]')"
  if [ "$GOT" = "$PROMPTFOO_PIN" ]; then
    ok "version verified: $GOT == pin $PROMPTFOO_PIN"
  else
    bad "version MISMATCH: installed '$GOT', pin '$PROMPTFOO_PIN' — the lock and the host disagree"
  fi
else
  bad "promptfoo is still not on PATH after install (is $NPM_PREFIX/bin on your PATH?)"
fi

# ------------------------------------------------------------ changedetection.io
step "3/5  changedetection.io $CDIO_PIN  (the page-diff tier, S14.6)"

CDIO_BIN="$HOME/.local/bin/changedetection.io"
if [ -x "$CDIO_BIN" ]; then
  ok "already installed — $CDIO_BIN"
else
  info "installing: uv tool install changedetection.io==$CDIO_PIN --python $CDIO_PYTHON --prerelease=allow"
  info "(uv fetches a managed CPython $CDIO_PYTHON if needed; the host's Python 3.14 is newer than"
  info " this dependency tree's wheel coverage, so the install is pinned to $CDIO_PYTHON deliberately."
  info " --prerelease=allow is required because 0.55.8 itself PINS pyppeteer-ng==2.0.0rc13, a"
  info " pre-release uv would otherwise refuse to resolve — this satisfies upstream's own pin,"
  info " it does not float anything)"
  if uv tool install "changedetection.io==$CDIO_PIN" --python "$CDIO_PYTHON" --prerelease=allow \
       >/tmp/sinet-cdio-install.log 2>&1; then
    ok "uv tool install completed"
  else
    bad "uv tool install failed — full log at /tmp/sinet-cdio-install.log"
    tail -20 /tmp/sinet-cdio-install.log | sed 's/^/        /'
  fi
fi

# uv normalizes the PyPI name to "changedetection-io" (PEP 503), so match dot OR hyphen
if uv tool list 2>/dev/null | grep -qiE "changedetection[.-]io[[:space:]]+v?$CDIO_PIN"; then
  ok "version verified against the pin: $(uv tool list 2>/dev/null | grep -i 'changedetection' | head -1)"
elif uv tool list 2>/dev/null | grep -qi 'changedetection'; then
  bad "version MISMATCH: $(uv tool list 2>/dev/null | grep -i 'changedetection' | head -1) — pin is $CDIO_PIN"
else
  bad "changedetection.io is not registered as a uv tool after install"
fi

# --------------------------------------------------------- Sinet-side conformance
step "4/5  Sinet conformance legs that activate now the organs exist"

cd "$REPO" || { bad "cannot cd to $REPO"; exit 1; }

info "promptfoo real-binary leg (asserts the installed binary matches internal/evals.PromptfooPin)"
if go test -count=1 -run 'TestPromptfooRealBinaryMatchesPin' ./internal/evals >/tmp/sinet-pf-test.log 2>&1; then
  if grep -q 'SANCTIONED SKIP' /tmp/sinet-pf-test.log; then
    bad "the leg still reports a SANCTIONED SKIP — promptfoo was not discovered (PATH or SINET_PROMPTFOO_PATH)"
  else
    ok "promptfoo real-binary leg green"
  fi
else
  bad "promptfoo real-binary leg FAILED — log at /tmp/sinet-pf-test.log"
  tail -20 /tmp/sinet-pf-test.log | sed 's/^/        /'
fi

info "changedetection.io real-organ leg"
note "stays a sanctioned skip until the organ is RUNNING and SINET_CDIO_URL + SINET_CDIO_API_KEY are set."
info "Installing it does not start it. Starting it unattended is the separate"
info "sinet-watchlist.service decision; the API key is a broker-custody secret and"
info "must never be pasted into a chat or a shell history."

info "full battery (nothing above should have changed it)"
if go test -count=1 ./... >/tmp/sinet-battery.log 2>&1; then
  ok "full battery green — $(grep -c '^ok  ' /tmp/sinet-battery.log) packages"
else
  bad "full battery FAILED after the installs — log at /tmp/sinet-battery.log"
  grep -E '^(FAIL|--- FAIL)' /tmp/sinet-battery.log | head -15 | sed 's/^/        /'
fi

# ------------------------------------------------------------------------ summary
step "5/5  Summary"
printf '  %d ok, %d failed, %d needing a later act\n\n' "$pass" "$fail" "$skip"

if [ "$fail" -eq 0 ]; then
  printf '  \033[32mBoth organs are installed at their pinned versions and Sinet agrees.\033[0m\n'
else
  printf '  \033[31m%d check(s) failed — read the FAIL lines above before doing anything else.\033[0m\n' "$fail"
fi

cat <<'EOF'

  Deliberately NOT done by this script:
    * no systemd unit installed — sinet-watchlist.service is still GENERATED only
    * no canary armed — SINET_CANARY_ARM is untouched
    * no secret read or written — SINET_CDIO_API_KEY is yours to place via the broker
    * changedetection.io is installed but not started

  To undo either install:
    npm uninstall -g promptfoo
    uv tool uninstall changedetection.io
EOF

exit "$([ "$fail" -eq 0 ] && echo 0 || echo 1)"
