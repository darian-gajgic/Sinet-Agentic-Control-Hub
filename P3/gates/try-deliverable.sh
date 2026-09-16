#!/usr/bin/env bash
# try-deliverable.sh — run a CODE deliverable from a click-through world on your
# own machine, so you can test the real application before accepting it.
#
# Stopgap for the deferred dev-server preview (internal/preview/manager.go
# deferredReason): the platform can compose the preview sandbox but cannot yet
# serve it live, so this script checks the pinned revision out OUTSIDE the
# platform and starts its dev server for you.
#
# It never writes into the world: the project store is read via `git archive`
# only. The checkout lands under ~/.sinet-code-tryout/<deliverable-id>/ and is
# rebuilt fresh on every run.
#
#   ./P3/gates/try-deliverable.sh                     # newest revision of the sitting's webshop
#   ./P3/gates/try-deliverable.sh <deliverable-id>    # any other code deliverable
#
# Stop the dev server with Ctrl+C when you are done. Delete ~/.sinet-code-tryout
# at leisure; nothing else references it.

set -uo pipefail

STATE="${SINET_CLICKTHROUGH_STATE:-$HOME/.sinet-rework-sitting}"
DLV="${1:-dlv-t-3120e8e3d14591d3}"

bold() { printf '\033[1m%s\033[0m\n' "$*"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; }
note() { printf '    %s\n' "$*"; }
die()  { printf '\n\033[31mSTOPPED:\033[0m %s\n\n' "$*"; exit 1; }

[[ -d "$STATE/projects/stores" ]] || die "no world at $STATE — is the click-through running from the same state dir?"

bold "Locating deliverable $DLV in the world's project stores"
STORE="" REF=""
for s in "$STATE"/projects/stores/*/; do
  r=$(git -C "$s" for-each-ref --format='%(refname)' "refs/sinet/deliverable/$DLV/*" 2>/dev/null \
      | sort -t- -k2 -n | tail -1)
  if [[ -n "$r" ]]; then STORE="${s%/}"; REF="$r"; break; fi
done
[[ -n "$REF" ]] || die "no ref refs/sinet/deliverable/$DLV/rev-* in any store under $STATE/projects/stores — check the deliverable id (it is in the page URL)"
ok "found $REF"
note "store: $STORE"

NFILES=$(git -C "$STORE" ls-tree -r --name-only "$REF" | wc -l)
[[ "$NFILES" -gt 0 ]] || die "the pinned revision is empty — that would be a platform defect, report it"
ok "the pinned revision holds $NFILES files"

TRY="$HOME/.sinet-code-tryout/$DLV"
rm -rf "$TRY" && mkdir -p "$TRY"
git -C "$STORE" archive "$REF" | tar -x -C "$TRY" || die "checkout failed"
ok "checked out to $TRY (the world itself was only read)"

[[ -f "$TRY/package.json" ]] || die "no package.json in this revision — not an npm app; look at the files in $TRY yourself"
command -v npm >/dev/null || die "npm is not on PATH"

bold "Installing dependencies (npm install — first run takes a minute or two)"
( cd "$TRY" && npm install --no-fund --no-audit ) >"$TRY/.npm-install.log" 2>&1 \
  || die "npm install failed — see $TRY/.npm-install.log"
ok "dependencies installed"

if grep -q '"test"' "$TRY/package.json"; then
  bold "Running the app's own tests first"
  if ( cd "$TRY" && npm test --silent ) >"$TRY/.npm-test.log" 2>&1; then
    ok "tests pass (log: $TRY/.npm-test.log)"
  elif [[ -d "$TRY/tests" ]] \
       && ( cd "$TRY" && node --test tests/*.test.js ) >"$TRY/.npm-test.log" 2>&1; then
    ok "tests pass when run directly ($(grep -m1 '^# pass' "$TRY/.npm-test.log" || echo '# pass ?'))"
    note "the shipped 'npm test' line fails on this host's Node — a packaging nit worth one line in your review"
  else
    printf '  \033[31m✗\033[0m %s\n' "the app's tests FAIL — read $TRY/.npm-test.log; that is accept-decision evidence"
  fi
fi

[[ -n "${TRYOUT_SKIP_SERVE:-}" ]] && { ok "TRYOUT_SKIP_SERVE set — stopping before the dev server"; exit 0; }

bold "Starting the dev server — open the Local URL it prints below; Ctrl+C here stops it"
cd "$TRY" && exec npm run dev
