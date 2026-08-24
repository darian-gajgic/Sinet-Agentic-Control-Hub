#!/usr/bin/env bash
# P3/gates/lane-key-ceremony.sh — LN-CEREMONY: commission the zai and kimi
# lanes on this host, in one guided, self-verifying pass.
#
# ONE COMMAND:
#
#     ./P3/gates/lane-key-ceremony.sh
#
# It walks eight steps in order and stops at the first real failure. Every step
# is independently re-runnable:
#
#     ./P3/gates/lane-key-ceremony.sh <step>
#
# and the whole thing can be rehearsed with no prompts, no writes and no
# network at all:
#
#     ./P3/gates/lane-key-ceremony.sh --dry-run
#     ./P3/gates/lane-key-ceremony.sh --dry-run <step>
#
# Steps: preflight  zai-key  kimi-key  smoke  kimi-wire  zai-calibration
#        model-list  summary
#
# WHERE IT WRITES. This host's own broker store by default: ~/.local/state/sinet
# under the OS user. STATE_DIRECTORY moves the state root and SINET_CEREMONY_USER
# moves the person — which is how P3/gates/lane-test-door.sh points the ceremony
# at a throwaway world and at the person you sign into that world as.
#
# SECRET POSTURE. A key is read with `read -rs` (never shown), handed to the
# placement tool on STDIN through a shell builtin, and unset immediately. It
# never reaches a command line, an environment variable, a log, this script's
# output, or a file outside the encrypted broker store. Nothing here echoes a
# key, and `set -x` is explicitly disabled so no trace mode can.
#
# NO VALUE IS HARDCODED. Every endpoint, profile, variable name, model id and
# quota figure is derived at print time from the lane documents
# (internal/adapters/opencode/lanedata/) and the plan documents
# (internal/metering/plandata/) through the platform's own loader, so this
# script cannot drift away from what the platform actually reads.
#
# WHAT IT NEVER DOES: it never edits a classifier fixture (reconciling the
# captured wire body against the lane document's signal rows is a FOLLOW-UP
# packet, so the change is reviewed rather than made by the script that took the
# measurement); it never arms a canary; it never makes a paid call without its
# own typed confirmation naming what that call spends.

set -euo pipefail
set +x

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DRY=0
LANES="zai kimi"

pass=0; fail=0
step()  { printf '\n\033[1m== %s\033[0m\n' "$*"; }
ok()    { printf '  \033[32mOK\033[0m    %s\n' "$*"; pass=$((pass+1)); }
bad()   { printf '  \033[31mFAIL\033[0m  %s\n' "$*"; fail=$((fail+1)); }
note()  { printf '  \033[33mNOTE\033[0m  %s\n' "$*"; }
info()  { printf '        %s\n' "$*"; }
dry()   { printf '  \033[36mDRY\033[0m   %s\n' "$*"; }
die()   { printf '\n  \033[31mSTOPPED\033[0m  %s\n\n' "$*" >&2; exit 1; }

# ─── where this host keeps its broker state ─────────────────────────────────
# Mirrors the dev-posture convention `sinet broker` resolves its own defaults
# from, so the ceremony writes where the platform reads.
state_dir() {
  if [ -n "${STATE_DIRECTORY:-}" ]; then printf '%s' "$STATE_DIRECTORY"
  elif [ -n "${XDG_STATE_HOME:-}" ]; then printf '%s' "$XDG_STATE_HOME/sinet"
  else printf '%s' "$HOME/.local/state/sinet"; fi
}
STORE_ROOT="$(state_dir)/broker-store"
# The PERSON this ceremony writes under. The OS user is the right default: at a
# single-operator host it is the one name the broker daemon, the store directory
# and the platform's own person id all coincide on (the unsettled namespace
# question is gate-batch item 3 below). A throwaway WORLD does not coincide —
# it carries its own seeded people, commissioning is keyed by the broker store's
# person, and the adapter asks ProvidersFor(userID) for whoever's session made
# the run. So a key placed under the OS user there commissions for a userID
# nobody can sign in as. SINET_CEREMONY_USER points this ceremony at that
# person instead; nothing else about placement changes (P3-LN-5).
STORE_USER="${SINET_CEREMONY_USER:-$(id -un)}"

BIN=""            # the placement tool, built into a temp dir by need_tool
TOOLDIR=""
# cleanup always succeeds: an EXIT trap whose last command fails REPLACES the
# script's exit status with that failure, which would report a clean ceremony
# as a broken one.
cleanup() {
  [ -n "$TOOLDIR" ] && rm -rf "$TOOLDIR"
  return 0
}
trap cleanup EXIT

lane_doc()  { printf '%s/internal/adapters/opencode/lanedata/%s.json' "$REPO" "$1"; }
plan_doc()  { printf '%s/internal/metering/plandata/%s.json' "$REPO" "$1"; }

# need_tool builds the credential tool. It is a TOOL under tools/, not a
# platform change: no `sinet` verb places an engine-cred (the binary's modes are
# control, broker, portpool, run-launch, snapshot, restore-drill, units, local,
# engine-hook and version; `sinet broker` is the daemon mode and has no
# sub-verb, and the wire's admin `store` op is dev-gated and reachable from no
# command line).
#
# It is called ONCE from main, in the parent shell. Calling it from inside a
# command substitution would build into a temp dir the parent never learns
# about — leaked on every call, and rebuilt on the next one.
need_tool() {
  [ -n "$BIN" ] && return 0
  TOOLDIR="$(mktemp -d)"
  BIN="$TOOLDIR/lanekey"
  ( cd "$REPO" && go build -o "$BIN" ./tools/lanekey ) || die "the credential tool did not build"
}

# lane_fact reads ONE derived fact through the platform's own lane loader, which
# also runs the R11 endpoint self-check before any value is handed back — so a
# lane pointed at a sibling endpoint fails here rather than on the wire.
lane_fact() {
  local lane=$1 key=$2 line
  line="$("$BIN" show --lane "$(lane_doc "$lane")" | grep "^${key}=" || true)"
  [ -n "$line" ] || die "lane document for '$lane' declares no '$key'"
  printf '%s' "${line#*=}"
}

confirm() { # $1 = the exact word required; rest = the lines to show first
  local want=$1 answer=''; shift
  printf '%s\n' "$@"
  if [ "$DRY" = 1 ]; then dry "would require you to type: $want"; return 1; fi
  printf '  Type \033[1m%s\033[0m and press Enter (anything else declines): ' "$want"
  read -r answer || true
  [ "$answer" = "$want" ]
}

# ─── 1. preflight ───────────────────────────────────────────────────────────
do_preflight() {
  step "1/8  Preflight"

  [ -d "$REPO/.git" ] || die "this script is not inside the Sinet repository ($REPO)"
  ok "repository: $REPO"

  if [ -d "$STORE_ROOT/$STORE_USER" ]; then
    ok "broker store root exists: $STORE_ROOT/$STORE_USER"
  else
    note "broker store root is ABSENT: $STORE_ROOT/$STORE_USER"
    info "That is expected on a host that has never run the broker. Placing the"
    info "first key creates it (0700, master key 0600) — nothing else does."
  fi

  # The pin is read from the adapter's own exported constant — the thing the
  # conformance suite is test-coupled to — never written down a second time here.
  local want installed
  want="$(grep -E '^const Pin =' "$REPO/internal/adapters/opencode/opencode.go" | head -1 | cut -d'"' -f2)"
  [ -n "$want" ] || die "could not read opencode.Pin from the adapter"
  if command -v opencode > /dev/null 2>&1; then
    installed="$(opencode --version 2>/dev/null | tail -1 | awk '{print $NF}')"
    if [ "$installed" = "$want" ]; then
      ok "opencode pin match: installed $installed = pin $want"
    else
      bad "opencode PIN DELTA: installed '$installed' != pin '$want'"
      info "The adapter targets the PIN's contract. Resolve through the S03.3"
      info "deliberate-bump procedure — never by silently retargeting."
    fi
  else
    bad "opencode is not installed — the tier-L smoke (step 4) cannot run"
  fi

  if command -v jq > /dev/null 2>&1; then ok "jq present: $(command -v jq)"; else bad "jq is not installed"; fi
  if command -v go > /dev/null 2>&1; then ok "go present: $(go version | awk '{print $3}')"; else die "go is not installed"; fi

  need_tool
  ok "credential tool built: tools/lanekey"

  local lane
  for lane in $LANES; do
    [ -f "$(lane_doc "$lane")" ] || die "lane document missing: $(lane_doc "$lane")"
    # Reading a fact runs the loader AND the endpoint self-check.
    info "$(printf 'lane %-5s profile=%-16s var=%-14s endpoint=%s' \
        "$lane" "$(lane_fact "$lane" profile)" "$(lane_fact "$lane" env_var)" "$(lane_fact "$lane" base_url)")"
  done
  ok "both lane documents load and pass the endpoint self-check"

  [ "$fail" = 0 ] || die "preflight found $fail problem(s) — fix them before placing a key"
}

# ─── 2/3. key placement ─────────────────────────────────────────────────────
place_key() {
  local lane=$1 profile envvar doc secret=''
  doc="$(lane_doc "$lane")"
  profile="$(lane_fact "$lane" profile)"
  envvar="$(lane_fact "$lane" env_var)"

  info "This places the key under broker auth-profile '$profile'."
  info "The platform delivers it to the engine as \$$envvar at spawn, resolved"
  info "fresh each time. It is stored encrypted and never logged or committed."
  info "Store: $STORE_ROOT/$STORE_USER"

  if [ "$DRY" = 1 ]; then
    dry "would prompt for the $lane key with 'read -rs' (never shown)"
    dry "would run: lanekey put --store-dir $STORE_ROOT --user $STORE_USER --lane $doc  (key on stdin)"
    dry "would then round-trip it back through the broker and compare digests"
    return 0
  fi

  printf '\n  Paste the %s API key and press Enter. It will NOT be displayed.\n' "$lane"
  printf '  key> '
  IFS= read -rs secret || true
  printf '\n'
  [ -n "$secret" ] || die "nothing was entered — no key was placed"

  # The key travels on STDIN through a shell BUILTIN, so it never appears in any
  # process's argv and never becomes an environment variable.
  if printf '%s' "$secret" | "$BIN" put --store-dir "$STORE_ROOT" --user "$STORE_USER" --lane "$doc"; then
    unset secret
    ok "$lane: placed and verified by round-trip through the broker"
  else
    unset secret
    die "$lane: placement or round-trip verification FAILED"
  fi
}

do_zai_key() {
  step "2/8  Z.AI key placement"
  place_key zai
}

do_kimi_key() {
  step "3/8  Kimi key placement"

  local overflow
  overflow="$(jq -r '[.models[].overflow_mode] | unique | join(", ")' "$(lane_doc kimi)")"
  printf '\n'
  note "READ THIS FIRST. The Kimi lane is the first lane that can SPILL."
  info "Its models are '$overflow': the Extra Usage Credit Pack can carry"
  info "spending past the flat membership. The recorded recommended posture is"
  info "that it stays OFF, under which the lane behaves as a true hard stop."
  printf '\n'
  printf '  \033[1mTurn Extra Usage OFF in the Kimi console before continuing.\033[0m\n'
  info "The vendor's own wording: \"You can turn it off at any time: your balance"
  info "stays in your account and the system pauses spending from it; turn it"
  info "back on to resume.\""
  printf '\n'

  if ! confirm "off" \
      "  Confirm you have turned Extra Usage OFF in the Kimi console."; then
    if [ "$DRY" = 1 ]; then
      dry "would then place the kimi key (placement follows the confirmation, never before it)"
      place_key kimi
      return 0
    fi
    die "Extra Usage was not confirmed OFF — no key was placed"
  fi
  ok "Extra Usage confirmed OFF by the operator"

  place_key kimi
}

# ─── 4. tier-L smoke ────────────────────────────────────────────────────────
smoke_lane() {
  local lane=$1 model plan unit q
  model="$(lane_fact "$lane" default_model)"
  plan="$(plan_doc "$lane")"
  unit="$(jq -r '.unit // "units"' "$plan")"
  q="$(jq -r '[.quotas[] | if (.allowance_unverified // false) then "\(.name): allowance UNPUBLISHED (\(.unit // "units"))" else "\(.name): \(.units) \(.unit // "'"$unit"'")" end] | join("; ")' "$plan")"

  printf '\n'
  note "COST OF THIS STEP, on the $lane lane:"
  info "ONE minimal paid call to model '$model'."
  info "Real dollars: \$0.00 — this is a flat-rate subscription lane."
  info "It DOES consume plan quota. Declared windows: $q"
  info "The plan reading is derived, not metered per request: the vendor"
  info "publishes no per-request counter."

  if ! confirm "spend" "  This makes a real, billable-against-quota call on the $lane lane."; then
    if [ "$DRY" = 1 ]; then
      dry "would run: SINET_LIVE_SMOKE=1 go test -count=1 -p 1 -v -run 'TestLiveSmoke/$lane' ./internal/adapters/opencode/"
    else
      note "$lane smoke DECLINED — no call was made"
    fi
    return 0
  fi

  printf '\n'
  if ( cd "$REPO" && SINET_LIVE_SMOKE=1 go test -count=1 -p 1 -v -run "TestLiveSmoke/$lane" ./internal/adapters/opencode/ ); then
    ok "$lane: tier-L smoke completed"
  else
    bad "$lane: tier-L smoke FAILED — the lane is not proven end to end"
  fi
}

do_smoke() {
  step "4/8  Tier-L smoke — one minimal paid call per lane"
  # The smoke inherits STATE_DIRECTORY from this shell, so it reads the same
  # store ROOT this ceremony wrote to. It resolves the store PERSON from the OS
  # user in Go and has no env seam for it, so under SINET_CEREMONY_USER it looks
  # somewhere else and says so rather than calling. Named here because a paid
  # step that silently answers about another store is worse than one that skips.
  if [ "$STORE_USER" != "$(id -un)" ]; then
    note "STORE PERSON MISMATCH for this step, and it is not a lane failure."
    info "This run places under person '$STORE_USER', but the tier-L smoke resolves"
    info "its store person from the OS user (live_smoke_test.go), so it will look in"
    info "$STORE_ROOT/$(id -un) and print a SANCTIONED SKIP instead of calling."
    info "Decline below: a paid call proves the KEY, and the key is already proven"
    info "by the round-trip through the broker in the placement step."
  fi
  info "Tier L is the ratified paid tier: it runs only under SINET_LIVE_SMOKE=1"
  info "and only against a lane whose credential is actually placed. Each lane"
  info "is asked for separately, below."
  local lane
  for lane in $LANES; do smoke_lane "$lane"; done
}

# ─── 5. Kimi wire capture ───────────────────────────────────────────────────
do_kimi_wire() {
  step "5/8  Kimi wire capture — ONE real error body"

  local out marker
  out="$REPO/P3/measurements/$(date -u +%Y-%m-%d)-kimi-wire-capture.md"
  marker="$(jq -r '.reset_marker' "$(lane_doc kimi)")"

  info "Every signal row on the kimi lane is DOCUMENTED-NOT-OBSERVED: each is a"
  info "message string quoted from the vendor's error reference, not a body this"
  info "platform has ever seen. This captures one real body."
  info "The credential sent is a literal invalid constant — the capture has no"
  info "code path to the broker store, so no real key can reach the wire."

  if [ "$DRY" = 1 ]; then
    dry "would POST once to $(lane_fact kimi base_url) with a deliberately-invalid key"
    dry "would write: $out"
    dry "would then report whether any reset / ratelimit / retry-after header came back"
    info "Lane document's reset_marker is currently: '${marker}' (empty = no documented reset signal)"
    note "Fixtures are NEVER edited here — reconciliation is a follow-up packet"
    return 0
  fi

  if "$BIN" wire401 --lane "$(lane_doc kimi)" --out "$out"; then
    ok "captured: $out"
  else
    bad "the capture did not complete — nothing was reconciled and no fixture changed"
    return 0
  fi

  printf '\n'
  if [ -z "$marker" ]; then
    note "reset-header check: the lane document's reset_marker is DELIBERATELY EMPTY."
    info "No reset-time signal is documented anywhere for this lane, so depletion"
    info "routes to a probe-park rather than to a fabricated wait. If the capture"
    info "above reported a reset/ratelimit header, that is new information and the"
    info "follow-up packet should populate the marker from it."
  else
    note "reset-header check: the lane document declares reset_marker '$marker' — compare it against the captured headers above."
  fi
  printf '\n'
  note "RECONCILIATION IS A FOLLOW-UP PACKET, and this script did not do it."
  info "No classifier fixture and no lane document was edited. Reconciling the"
  info "observed body's shape, code field, message grammar and reset marker"
  info "against the lane document's signal rows is a reviewed change."
}

# ─── 6. Z.AI dashboard calibration ──────────────────────────────────────────
do_zai_calibration() {
  step "6/8  Z.AI dashboard calibration recipe"

  local spike section
  spike="$REPO/Research/spikes/P2-S1-opencode-live-auth-zai-calibration.md"
  [ -f "$spike" ] || die "the calibration spike is missing: $spike"

  section="$(sed -n '/^## Blocked items/,/^\*\*Optional/p' "$spike" | sed '$d')"
  [ -n "$section" ] || die "the spike's blocked-items section did not extract — the document moved"

  info "Printed VERBATIM from the spike record, at print time:"
  info "$spike"
  printf '\n%s\n' "$section"

  local reqid
  reqid="$(grep -n 'request_id' "$spike" || true)"
  [ -n "$reqid" ] || die "the spike's request_id guidance did not extract — the document moved"
  printf '  \033[1mrequest_id guidance, verbatim from the same record:\033[0m\n\n'
  printf '%s\n' "$reqid" | sed 's/^/  /'
  printf '\n'
  note "request_id is the reconciliation key between Sinet's ledger rows and the"
  info "provider dashboard. It rides the receipt, so it is readable where a"
  info "person reads their run, not only queryable from the ledger."
}

# ─── 7. observed tier / model list ──────────────────────────────────────────
do_model_list() {
  step "7/8  Observed tier and model-list capture"

  note "THE ACCOUNT'S ACTUAL MODEL LIST IS THE AUTHORITY — never the docs, never"
  info "the lane document, never the engine's embedded catalogue. Every model id"
  info "shipped in this tree is dated seed data, and the whole point of the"
  info "model-list canary is to find the day the account and the seed disagree."

  local lane models
  for lane in $LANES; do
    models="$(lane_fact "$lane" models)"
    printf '\n'
    info "lane '$lane' — CONFIGURED (seed) model ids: $models"
    info "default seat: $(lane_fact "$lane" default_model)"
    case "$lane" in
      zai)
        info "Capture the OBSERVED list from the Z.AI console for this account."
        info "The coding endpoint publishes no model list — /models is undocumented"
        info "there and answers 404 — so the console is the only source."
        ;;
      kimi)
        info "Capture the OBSERVED list AND the membership tier from the Kimi"
        info "console. This lane's gate is by MEMBERSHIP TIER, not by region, and"
        info "it is enforced on the wire, so the tier is part of the observation."
        info "Two ids in the seed are flagged seed-only-pending-observation:"
        info "$(jq -r '[.models[] | select(.observation_grade == "seed-only-pending-observation") | .id] | join(", ")' "$(lane_doc kimi)")"
        info "Whether they resolve for THIS account is settled here and nowhere else."
        ;;
    esac
  done

  printf '\n'
  info "Write what you observe into a dated measurement file, one per lane:"
  info "  $REPO/P3/measurements/$(date -u +%Y-%m-%d)-<lane>-observed-models.md"
  info "Record, per lane: the account tier, every model id offered, and the date."
  printf '\n'
  note "Do NOT edit the lane documents from what you observe in this step."
  info "A seed id that the account does not serve is a finding for the follow-up"
  info "packet, the same way the wire capture is: measured here, reconciled there."
}

# ─── 8. summary ─────────────────────────────────────────────────────────────
do_summary() {
  step "8/8  Summary — what is commissioned, and what is not"

  local lane profile state
  need_tool
  for lane in $LANES; do
    profile="$(lane_fact "$lane" profile)"
    if [ "$DRY" = 1 ]; then
      dry "would run: lanekey verify --store-dir $STORE_ROOT --user $STORE_USER --lane $(lane_doc "$lane")"
      continue
    fi
    if "$BIN" verify --store-dir "$STORE_ROOT" --user "$STORE_USER" --lane "$(lane_doc "$lane")" > /dev/null 2>&1; then
      state="COMMISSIONED — credential placed and resolvable through the broker"
    else
      state="not commissioned — no credential resolves under profile '$profile'"
    fi
    printf '  %-6s %s\n' "$lane" "$state"
  done

  printf '\n'
  note "WHAT PLACING A KEY DOES, AND WHEN — read this before expecting a lane to run."
  info "A lane is COMMISSIONED by a credential and made SELECTABLE by a provider"
  info "entry. This ceremony does the first. The second is composed by the"
  info "control plane at STARTUP: it reads each person's broker store and gives"
  info "that person a provider entry for every lane whose credential it finds"
  info "placed there (P3-LN-4). So a key placed here does make its lane"
  info "routable — at the control plane's NEXT START."
  printf '\n'
  note "A control plane that was already running when you placed this key will"
  note "not send work to the lane until you restart it. Commissioning is"
  info "startup-bound: coverage, the lane-to-substrate map and the alternate"
  info "seats are composed once, at startup, and are never refilled live."
  printf '\n'
  info "The tier-L smoke in step 4 goes around that map on purpose — it drives"
  info "the adapter directly — which is exactly why a green smoke proves the"
  info "KEY and not the routing."

  printf '\n'
  note "SINET_CANARY_ARM stays an operator gate decision, and this script did not touch it."
  info "The auth, behavioral and model-list canary legs stay DISARMED. Arming"
  info "them is a separate, deliberate decision with its own cost projection:"
  info "the real-dollar stop line is any non-zero probe-tagged spend, which on"
  info "flat-rate lanes must be exactly \$0.00."

  printf '\n'
  note "LN gate-batch items ahead (none of them done by this script):"
  info "  1. Reconcile the captured Kimi wire body against the lane document's"
  info "     signal rows and the classifier fixtures — a reviewed packet."
  info "  2. Reconcile the observed model lists and the Kimi membership tier"
  info "     against the seeded model ids, including the two flagged"
  info "     seed-only-pending-observation."
  info "  3. Settle the person-namespace question. The broker keys its store"
  info "     directory and its socket by an OS-level user name; the platform"
  info "     carries its own person ids (runs.user_id / auth). Commissioning is"
  info "     keyed by the broker name — the only choice that makes injection"
  info "     dial the right socket — and the startup log line names the people"
  info "     it commissioned so a mismatch is visible. At one operator the two"
  info "     coincide; the household case needs a ratified reading."
  info "  4. Run the Z.AI dashboard calibration (step 6) and record the"
  info "     prompts-per-request mapping it yields."
  info "  5. Decide SINET_CANARY_ARM at the gate."

  printf '\n'
  if [ "$fail" = 0 ]; then
    ok "ceremony finished with no failures recorded in this run"
  else
    bad "$fail failure(s) recorded in this run — see above"
  fi
}

# ─── driver ─────────────────────────────────────────────────────────────────
run_step() {
  case "$1" in
    preflight)       do_preflight ;;
    zai-key)         do_zai_key ;;
    kimi-key)        do_kimi_key ;;
    smoke)           do_smoke ;;
    kimi-wire)       do_kimi_wire ;;
    zai-calibration) do_zai_calibration ;;
    model-list)      do_model_list ;;
    summary)         do_summary ;;
    *) die "unknown step '$1' (steps: preflight zai-key kimi-key smoke kimi-wire zai-calibration model-list summary)" ;;
  esac
}

main() {
  local only=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --dry-run) DRY=1 ;;
      -h|--help) sed -n '2,40p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
      -*) die "unknown option '$1'" ;;
      *) only="$1" ;;
    esac
    shift
  done

  printf '\n\033[1mLN-CEREMONY — lane key placement\033[0m\n'
  if [ "$DRY" = 1 ]; then
    printf '\033[36mDRY RUN\033[0m: no prompt is answered, nothing is written to the broker store,\n'
    printf 'and no network request is made. The credential tool IS built into a temp\n'
    printf 'directory so every value below is derived for real from the lane documents.\n'
  fi
  printf 'store: %s/%s\n' "$STORE_ROOT" "$STORE_USER"

  # Built once, in THIS shell, so every step and every command substitution
  # below shares one binary and one temp dir.
  need_tool

  if [ -n "$only" ]; then
    run_step "$only"
  else
    for s in preflight zai-key kimi-key smoke kimi-wire zai-calibration model-list summary; do
      run_step "$s"
    done
  fi
  printf '\n'
  [ "$fail" = 0 ]
}

main "$@"
