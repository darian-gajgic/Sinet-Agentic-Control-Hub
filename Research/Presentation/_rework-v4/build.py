"""Rebuild the research deck: same content, new structure + style.
usage: build.py SRC OUT"""
import re, sys, os, html as H
sys.path.insert(0, os.path.dirname(__file__))
import htree
SRC, OUT = sys.argv[1], sys.argv[2]
TOOLS = os.path.dirname(os.path.abspath(__file__))
s = open(SRC).read()

def rd(p): return open(os.path.join(TOOLS, p)).read()
def text_of(fragment):
    return ' '.join(re.sub(r'<[^>]+>', ' ', H.unescape(fragment)).split())

# ---------------------------------------------------------------- theme tokens
from theme import DARK_TO_LIGHT, RGB_LIGHT
def tok(h):
    h = h.lower().lstrip('#')
    return f'var(--c-{h})'
HEX_RE = re.compile(r'#(?:[0-9a-fA-F]{6}|[0-9a-fA-F]{3})(?![0-9a-fA-F])')
def tokenize_css(css):
    css = re.sub(r'rgba\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,', lambda m: 'rgba(var(--rgb-%02x%02x%02x),' % tuple(map(int, m.groups())), css)
    return HEX_RE.sub(lambda m: tok(m.group(0)), css)

# ---------------------------------------------------------------- locate pieces
head_end = s.index('<style>')
gstyle_end = s.index('</style>') + len('</style>')
body_open = s.index('<body>') + len('<body>')
hdr_a = s.index('<header class="topnav">'); hdr_b = s.index('</header>', hdr_a) + len('</header>')
logo = re.search(r'<img class="logo"[^>]*>', s[hdr_a:hdr_b]).group(0)
dots = '<div class="dots" id="dots" aria-hidden="true"></div>'
assert s.count(dots) == 1
script_a = s.rindex('<script>'); script_b = s.index('</script>', script_a) + len('</script>')
sprite_a = s.index('<svg width="0" height="0"'); sprite_b = s.index('</svg>', sprite_a) + len('</svg>')
foot_a = s.index('<footer>'); foot_b = s.index('</footer>') + len('</footer>')

body = s[sprite_a:script_a]   # sprite .. footer (before script)
off0 = sprite_a

# work on the region of sections only
sec_a = s.index('<section class="sec" id="hero"')
sec_b = foot_a
region = s[sec_a:sec_b]
root = htree.parse(region)
sections = [n for n in root.kids if n.tag == 'section']
assert len(sections) == 18, len(sections)

EL_LABEL = {'impact': 'What it costs', 'cause': 'Why it happens', 'solution': 'The fix',
            'beforeafter': 'Before / after', 'impl': 'What it takes'}
ST_LABEL = [('evnow', 'current'), ('evpast', 'historical'), ('scope', 'scope-limited'),
            ('thin', 'thin evidence'), ('vend', 'vendor-measured'), ('unm', 'unmeasured')]

def words(fragment):
    f = re.sub(r'<(style|script|svg)\b.*?</\1>', ' ', fragment, flags=re.S)
    f = re.sub(r'<span class="rbox">.*?</span></span>', '</span>', f, flags=re.S)
    return len(text_of(f).split())

def rebuild_g30(src, g, pid):
    """figure: id, real heading, takeaway promoted under the heading"""
    kids = g.kids
    gt = [k for k in kids if k.has('gt')]
    takes = [k for k in kids if k.has('take')]
    assert len(gt) == 1 and kids[0] is gt[0], (pid, g)
    gap_text = ''.join(src[a.end:b.start] for a, b in zip(kids, kids[1:]))
    assert gap_text.strip() == '', (pid, gap_text[:80])
    stag = src[g.start:g.stag_end]
    el = g.attrs.get('data-el')
    if el:
        stag = stag.replace('<div class="g30', f'<div id="{pid}-{el}" class="g30', 1)
    stag = stag.replace('class="g30 reveal"', 'class="g30 fig reveal"')
    gth = src[gt[0].start:gt[0].end]
    gth = re.sub(r'^<div class="gt">', '<h3 class="gt">', gth); gth = re.sub(r'</div>$', '</h3>', gth)
    order = [gth] + [src[t.start:t.end] for t in takes] + \
            [src[k.start:k.end] for k in kids if k is not gt[0] and k not in takes]
    body = '\n  '.join(order)
    return stag + '\n  ' + body + '\n</div>'

def region_attrs(t):
    """every horizontally scrolling table wrapper becomes a labelled, keyboard-scrollable region"""
    def one(m):
        tag = m.group(0)
        after = t[m.end():m.end()+600]
        cap = re.search(r'<(?:div|h3) class="(?:tt|et)(?: n)?">(.*?)</(?:div|h3)>', after, re.S)
        lab = text_of(cap.group(1)).split(' · ')[0] if cap else 'Table'
        return tag[:-1] + f' role="region" tabindex="0" aria-label="{H.escape(lab)}">'
    return re.sub(r'<div (?:id="[^"]+" )?class="(?:tablewrap|evb)[^"]*"[^>]*>', one, t)

def chapter(src, sec):
    pid = sec.attrs['id']; num = int(pid[1:])
    wrap = sec.kids[0]; assert wrap.has('wrap')
    k = wrap.kids
    assert k[0].tag == 'style' and k[1].has('reveal') and k[2].has('who') and k[3].has('beats'), pid
    assert k[-2].has('evb') and k[-1].has('srcline'), pid
    style, headdiv, who, beats = k[0], k[1], k[2], k[3]
    mid = k[4:-2]; evb, srcl = k[-2], k[-1]
    part = lambda n: src[n.start:n.end]
    # --- header block
    w = words(src[sec.start:sec.end]) - words(part(evb))
    mins = max(1, round(w / 230))
    rows = evb.find(lambda n: n.tag == 'tr' and any(c.tag == 'td' for c in n.kids))
    st = evb.find(lambda n: n.has('st'))
    counts = [(lbl, sum(1 for t in st if t.has(c))) for c, lbl in ST_LABEL]
    head = (f'<header class="ch-head">\n{part(headdiv)}\n'
            f'<div class="ch-meta">{part(who)}<span class="mins">{mins} min read · {len(rows)} sourced claims</span></div>\n'
            f'<div class="sum"><h3 class="sumh">The chapter in four steps</h3>\n{part(beats)}</div>\n</header>')
    # --- middle blocks
    out = []
    for n in mid:
        t = part(n)
        if n.has('g30'):
            t = rebuild_g30(src, n, pid)
        elif n.has('tablewrap'):
            t = t.replace('<div class="tablewrap', f'<div id="{pid}-tools" class="tablewrap', 1)
            t = region_attrs(t)
            t = re.sub(r'<div class="tt( n)?">(.*?)</div>', lambda m: f'<h3 class="tt{m.group(1) or ""}">{m.group(2)}</h3>', t, count=1, flags=re.S)
        out.append(t)
    # --- evidence: collapsed by default
    ev = part(evb)
    et = re.search(r'<div class="et">(.*?)</div>', ev, re.S)
    ev = ev.replace(et.group(0), '', 1)
    ev = ev.replace('<div class="evb reveal">', f'<div class="evb reveal" role="region" tabindex="0" aria-label="{H.escape(text_of(et.group(1)).split(" · ")[0])}">', 1)
    cls = {lbl: c for c, lbl in ST_LABEL}
    cnt = ' '.join(f'<span class="st {cls[lbl]}">{lbl.upper()}</span>&nbsp;{c}' for lbl, c in counts if c)
    det = (f'<details class="evd" id="{pid}-evidence">\n'
           f'<summary><span class="et">{et.group(1)}</span>'
           f'<span class="evc"><b>{len(rows)} claims.</b> Status labels used: {cnt}</span></summary>\n{ev}\n</details>\n{part(srcl)}')
    inner = '\n'.join([part(style), head] + out + [det])
    return src[sec.start:wrap.stag_end] + '\n' + inner + '\n  </div>\n</section>'

pieces = []; cur = 0
chap_meta = []
for sec in sections:
    pieces.append(region[cur:sec.start])
    sid = sec.attrs['id']
    if re.fullmatch(r'p\d\d', sid):
        pieces.append(chapter(region, sec))
    else:
        t = region[sec.start:sec.end]
        if sid in ('squeeze','blueprint'):
            sub = htree.parse(t)
            figs = sub.find(lambda n: n.has('g30'))
            for g in sorted(figs, key=lambda n: -n.start):
                t = t[:g.start] + rebuild_g30(t, g, sid) + t[g.end:]
        if sid in ('hero','squeeze','team','blueprint','close'):
            # intro numbering clashed with chapter numbering ("01 · WHY NOW" vs "PROBLEM 01")
            t = t.replace('<div class="overline">01 · WHY NOW — THE SQUEEZE</div>', '<div class="overline">WHY NOW — THE SQUEEZE</div>')
            t = t.replace('<div class="overline">02 · MEET YOUR TEAM</div>', '<div class="overline">MEET YOUR TEAM</div>')
            t = t.replace('<div class="legend reveal">', '<div id="legend" class="legend reveal">')
            t = re.sub(r'<div class="gt">(.*?)</div>', r'<h3 class="gt">\1</h3>', t)
            t = re.sub(r'<div class="tt( n)?">(.*?)</div>', lambda m: f'<h3 class="tt{m.group(1) or ""}">{m.group(2)}</h3>', t)
            t = region_attrs(t)
        pieces.append(t)
    cur = sec.end
pieces.append(region[cur:])
new_region = ''.join(pieces)

# ---------------------------------------------------------------- at-a-glance (from the close recap)
close_sec = region[sections[-1].start:sections[-1].end]
cards = re.findall(r'<a class="cl-card" href="#(p\d\d)">\s*<span class="n">(.*?)</span>\s*<h4>(.*?)</h4>\s*<p>(.*?)</p></a>', close_sec, re.S)
assert len(cards) == 13, len(cards)
items = []
for pid, n, h4, p in cards:
    items.append(f'<li><a href="#{pid}"><span class="gn">{n}</span><b class="gh">{h4}</b><span class="gp">{p}</span></a></li>')
glance = rd('glance.html').replace('@@ITEMS@@', '\n    '.join(items))
hero_end = new_region.index('</section>') + len('</section>')
new_region = new_region[:hero_end] + '\n\n' + glance + new_region[hero_end:]

# ---------------------------------------------------------------- table of contents
all_secs = re.findall(r'<section class="sec[^"]*" id="([^"]+)" data-title="([^"]+)"', new_region)
toc = []
for sid, title in all_secs:
    if re.fullmatch(r'p\d\d', sid):
        sub = ''.join(f'<li><a href="#{sid}-{el}">{lbl}</a></li>' for el, lbl in EL_LABEL.items())
        sub += f'<li><a href="#{sid}-tools">Named tools</a></li><li><a href="#{sid}-evidence">Evidence table</a></li>'
        toc.append(f'<li data-sec="{sid}"><a href="#{sid}"><span class="no">{sid[1:]}</span><span class="tl">{H.escape(H.unescape(title))}</span></a><ol class="sub">{sub}</ol></li>')
    else:
        if sid == 'p01' or (sid == 'squeeze'): pass
        toc.append(f'<li data-sec="{sid}"><a href="#{sid}"><span class="no"></span><span class="tl">{H.escape(H.unescape(title))}</span></a></li>')
toc_html = '\n'.join(toc)
# group label before chapter 1 and after 13
toc_html = toc_html.replace('<li data-sec="p01">', '<li class="grp" aria-hidden="true">The thirteen problems</li>\n<li data-sec="p01">', 1)
toc_html = toc_html.replace('<li data-sec="blueprint">', '<li class="grp" aria-hidden="true">Building it</li>\n<li data-sec="blueprint">', 1)

chrome = rd('chrome.html').replace('@@TOC@@', toc_html).replace('@@LOGO@@', logo)

# ---------------------------------------------------------------- assemble
gcss = rd('global.css')
headpart = s[:head_end]
headpart = headpart.replace('<meta name="color-scheme" content="dark">', '<meta name="color-scheme" content="dark light">')
headpart += rd('head.html')
theme_css = rd_theme = None
from theme import theme_css as mk_theme
out = (headpart + '<style>\n' + mk_theme(s) + '\n' + gcss + '\n</style>\n</head>\n<body>\n'
       + s[sprite_a:sprite_b] + '\n\n' + chrome + '\n<main id="main">\n' + new_region + '</main>\n\n'
       + s[foot_a:foot_b] + '\n\n<script>\n' + rd('app.js') + '\n</script>\n</body>\n</html>\n')

# tokenize chapter-local <style> blocks and inline style attributes (theme-aware colours)
def calm_mirror(css):
    """append html.calm copies of each @media (prefers-reduced-motion:reduce){...} block"""
    out = []
    for m in re.finditer(r'@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{', css):
        i = m.end(); depth = 1; j = i
        while depth:
            if css[j] == '{': depth += 1
            elif css[j] == '}': depth -= 1
            j += 1
        block = css[i:j-1]
        for rule in re.finditer(r'([^{}]+)\{([^{}]*)\}', block):
            sels = [x.strip() for x in rule.group(1).split(',') if x.strip()]
            pref = ','.join('html.calm' + (x[4:] if x.startswith('html') else ' ' + x) for x in sels)
            out.append(pref + '{' + rule.group(2) + '}')
    return css + ('\n/* motion toggle (mirrors the reduced-motion block above) */\n' + '\n'.join(out) + '\n' if out else '')

SVG_TEXT_CLASSES = set()
NO_SCALE = ('p07-dc', 'p01-model', 'p01-mask', 'p01-count')   # hand-positioned mock-ups keep their measured size
def _bump(px, sel=''):
    if any(k in sel for k in NO_SCALE): return px
    term = any(k in sel for k in ('term', 'sabo', 'code', 'datarow', '-out', 'tm-out'))
    if px < 10: return px
    if term: return px          # terminal / log mock-ups: wider text would break their one-line rows
    if px < 11: return px + 1
    if px < 12: return px + 1.5
    if px <= 13.5: return px + 2.5
    return px
def scale_css(css):
    def rule(m):
        sel, body = m.group(1), m.group(2)
        if set(re.findall(r'\.([\w-]+)', sel)) & SVG_TEXT_CLASSES or re.search(r'\btext\b', sel):
            return m.group(0)
        body = re.sub(r'font-size:\s*([\d.]+)px', lambda f: 'font-size:%gpx' % _bump(float(f.group(1)), sel), body)
        return sel + '{' + body + '}'
    return re.sub(r'([^{}]+)\{([^{}]*)\}', rule, css)
def scale_inline(style):
    return re.sub(r'font-size:\s*([\d.]+)px', lambda f: 'font-size:%gpx' % _bump(float(f.group(1))), style)

def tok_styles(doc):
    first = doc.index('<style>'); first_end = doc.index('</style>', first)
    head, rest = doc[:first_end], doc[first_end:]
    SVG_TEXT_CLASSES.update(c for m in re.finditer(r'<text[^>]*class="([^"]+)"', doc) for c in m.group(1).split())
    rest = re.sub(r'(<style>)(.*?)(</style>)', lambda m: m.group(1) + calm_mirror(scale_css(tokenize_css(m.group(2)))) + m.group(3), rest, flags=re.S)
    def inline(m):
        st = tokenize_css(m.group(2))
        return m.group(1) + 'style="' + (st if m.group(1).startswith('<text') or m.group(1).startswith('<tspan') else scale_inline(st)) + '"'
    rest = re.sub(r'(<[a-zA-Z]+[^<>]*?)style="([^"]*)"', inline, rest)
    return head + rest
out = tok_styles(out)
assert out.count('<div class="p05-mat">') == 1
out = out.replace('<div class="p05-mat">', '<div class="p05-mat" role="region" tabindex="0" aria-label="Duty table">')
open(OUT, 'w').write(out)
print('wrote', OUT, len(out))
