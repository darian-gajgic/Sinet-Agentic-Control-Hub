#!/usr/bin/env bash
# Text-fit audit for the deck's hand-authored SVG graphics.
#
# SVG text does not wrap and is clipped at the svg frame, so a label that grew
# past its box just silently breaks. This renders index.html headless, measures
# every <text> against (a) the shape it sits in, (b) its svg frame, and (c) its
# neighbours, and prints anything that spills, is cut off, or collides.
#
#   ./qa/run-textfit.sh            # default 1440px viewport
#   ./qa/run-textfit.sh 860        # any width
#
# Expected output: 1 finding — the '✕' drawn across the 6-unit firewall bar in
# p08's before/after graphic. That mark is deliberate. Anything else is a bug.
#
# Notes: measurement runs with animations frozen, so faded cross-fade layers and
# rotated text (the p12 see-saw) are skipped — their axis-aligned boxes lie.
# Chrome must read the file over file:// (localhost is blocked in the sandbox).
set -euo pipefail

W="${1:-1440}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

python3 - "$DIR" "$TMP" <<'PY'
import sys, pathlib
d, tmp = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
deck = d / 'index.html'
if not deck.exists():                      # deck file was renamed (Sinet-Research-Presentation.html)
    cands = [p for p in d.glob('*.html')]
    if len(cands) != 1:
        raise SystemExit('AUDIT FAILED — cannot identify the deck file in %s: %s' % (d, cands))
    deck = cands[0]
src = deck.read_text()
js  = (d / 'qa' / 'textfit.js').read_text()
(tmp / 'audit.html').write_text(src.replace('</body>', '<script>\n' + js + '\n</script>\n</body>'))
PY

google-chrome --headless=new --disable-gpu --no-sandbox --hide-scrollbars \
  --force-prefers-reduced-motion --window-size="$W",900 --dump-dom \
  "file://$TMP/audit.html?flat=1" 2>/dev/null \
| python3 -c "
import sys, re, html, json
m = re.search(r'<pre id=\"qa-report\">(.*?)</pre>', sys.stdin.read(), re.S)
if not m:
    print('AUDIT FAILED — no report in the rendered DOM'); raise SystemExit(1)
j = json.loads(html.unescape(m.group(1)))
print('viewport %d — %d finding(s)' % (j['vw'], j['n']))
for f in j['f']:
    o = f['over']
    where = 'box %s' % f.get('box', {}).get('width', '?') if f['kind'] == 'BOX' else 'viewBox w=%s' % f.get('vbw', '?')
    print('  %-8s %-6s %-22s over=%s  %r' % (f['kind'], f['sec'], where, o, f['text'][:60]))
    if f['kind'] == 'COLLIDE':
        print('           %r' % f['text2'][:60])
"
