#!/usr/bin/env bash
# Merge P3/run/hooks.proposed.json into the PROJECT settings (.claude/settings.json). Operator-run, idempotent.
# Why a script: the coordinator's own edit of settings.json is denied by the auto-mode classifier (self-modification),
# so the hooks are installed by the operator with ONE command:  P3/run/install-hooks.sh
# Hooks (verified against Claude Code 2.1.278, see P3/design/harness-sota-research-2026-09-22.md §1.8/§4.3):
#   StopFailure(rate_limit|usage_limit) → writes P3/run/status.json {"outcome":"LIMIT"} so the loop never guesses from prose
#   PreCompact                          → appends P3/run/log/compactions.log (a sitting that compacts is over budget)
#   PermissionDenied                    → appends P3/run/log/denials.log (the only record of what auto mode blocked)
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
S=.claude/settings.json; P=P3/run/hooks.proposed.json
[ -f "$S" ] || echo '{}' > "$S"
jq -e . "$S" >/dev/null || { echo "FAIL: $S is not valid JSON — fix it first"; exit 1; }
jq -e . "$P" >/dev/null || { echo "FAIL: $P is not valid JSON"; exit 1; }
cp "$S" "$S.bak.$(date +%s)"
jq --slurpfile h "$P" '.hooks = ((.hooks // {}) + $h[0])' "$S" > "$S.tmp" && mv "$S.tmp" "$S"
echo "OK: hooks installed into $S — events now: $(jq -r '.hooks | keys | join(", ")' "$S")"
echo "Verify: jq .hooks $S   (a backup of the previous file sits next to it as $S.bak.<epoch>)"
