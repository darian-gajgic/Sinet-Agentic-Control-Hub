"""Theme tokens. Dark = the deck's original palette (text contrast raised); light = a reading palette
built by inverting each colour's ROLE: dark surfaces -> light surfaces, light inks -> dark inks,
bright accents -> deep accents, deep accent tints -> pastel tints, ink-on-accent -> white."""
import re
DARK_TO_LIGHT = {
 # surfaces
 '070a10':'f5f6f8','0a0e17':'eceff3','0f141f':'ffffff','131a29':'f3f5f8','0e1420':'eaedf2','0c1018':'ffffff',
 '080c14':'ffffff','05070c':'f8fafc','080b12':'f8fafc','111725':'f0f3f7','101827':'edf1f6','20283c':'dce2ea',
 '152033':'e3e8ef','1d2536':'e1e5ec','2a3550':'c2cad7','26304a':'d3d9e2',
 # tag / chip fills that carry ink text
 '3d4a63':'e1e7ef','4a3f7a':'c4b5fd','5a3a42':'f6d6dc',
 # inks
 'e9edf5':'111827','fff':'111827','ffffff':'111827','c8d2e4':'1f2937','9aa4b8':'3b4556','66718a':'596476',
 '9fd8c4':'0b6b4d','7fd9e6':'0b6a84',
 # accents
 '53d7e8':'0b6a84','63e2a3':'14743a','f2b45e':'a34a07','f07684':'be123c','a48dfa':'6d28d9','d6a24f':'92400e',
 # deep accent tints (borders, dim strokes)
 '1d5f6f':'86c6d3','2c5d45':'8bcca6','6b3340':'eba0ad','6b5127':'e3bb80','4a3a20':'e8cfa6','3a2530':'eec9d1',
 # ink on accent
 '04252b':'ffffff','2a1c05':'ffffff','0a1f18':'ffffff',
}
# dark mode: same palette, surfaces lifted off near-black and headings off pure white (less glare)
DARK_LIFT = {'070a10':'0d1117','0a0e17':'11161f','0f141f':'161b26','131a29':'1b2230','0e1420':'151a25','0c1018':'141923',
 '080c14':'151a24','05070c':'0b0e14','080b12':'0d1118','111725':'182030','101827':'172033','20283c':'283149',
 '152033':'1a2537','1d2536':'252d3e','2a3550':'34405a','26304a':'2e3850','66718a':'8a95a9','fff':'f1f3f7','ffffff':'f1f3f7','e9edf5':'e8ebf0'}
RGB_DARK = {'070a10':'13,17,23','131a29':'27,34,48'}
RGB_LIGHT = {'53d7e8':'11,106,132','f07684':'190,18,60','f2b45e':'163,74,7','63e2a3':'20,116,58',
             'a48dfa':'109,40,217','070a10':'245,246,248','131a29':'243,245,248','000000':'15,23,42',
             '66718a':'89,100,118'}
SEM_DARK = dict(bg='#0d1117', bg2='#11161f', panel='#161b26', panel2='#1b2230', line='#252d3e', line2='#34405a',
    ink='#e8ebf0', mut='#b6bfcd', dim='#939db0', cy='#53d7e8', cyd='#1d5f6f', gn='#63e2a3', am='#f2b45e',
    rs='#f07684', vi='#a48dfa', prose='#d2d8e1', shadow='0 14px 44px rgba(0,0,0,.7)', glow='.10',
    **{'tint-am':'rgba(242,180,94,.06)','tint-cy':'rgba(83,215,232,.05)'})
SEM_LIGHT = dict(bg='#f5f6f8', bg2='#eceff3', panel='#ffffff', panel2='#f3f5f8', line='#e1e5ec', line2='#c2cad7',
    ink='#111827', mut='#3b4556', dim='#596476', cy='#0b6a84', cyd='#86c6d3', gn='#14743a', am='#a34a07',
    rs='#be123c', vi='#6d28d9', prose='#1f2937', shadow='0 10px 30px rgba(15,23,42,.16)', glow='.05',
    **{'tint-am':'#fbf2e6','tint-cy':'#eaf5f8'})

def _norm(h):
    h=h.lower().lstrip('#'); return h
def theme_css(doc):
    hexes = sorted({_norm(h) for h in re.findall(r'#(?:[0-9a-fA-F]{6}|[0-9a-fA-F]{3})(?![0-9a-fA-F])', re.sub(r'data:image[^"]+','',doc))})
    rgbs = sorted({'%02x%02x%02x' % tuple(map(int,m)) for m in re.findall(r'rgba\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,', doc)})
    missing = [h for h in hexes if h not in DARK_TO_LIGHT]
    missing_rgb = [h for h in rgbs if h not in RGB_LIGHT]
    assert not missing, missing
    assert not missing_rgb, missing_rgb
    def full(h): return h if len(h)==6 else ''.join(c*2 for c in h)
    dark = ['--%s:%s' % (k,v) for k,v in SEM_DARK.items()]
    dark += ['--c-%s:#%s' % (h,DARK_LIFT.get(h,h)) for h in hexes]
    dark += ['--rgb-%s:%s' % (h, RGB_DARK.get(h) or ','.join(str(int(full(h)[i:i+2],16)) for i in (0,2,4))) for h in rgbs]
    light = ['--%s:%s' % (k,v) for k,v in SEM_LIGHT.items()]
    light += ['--c-%s:#%s' % (h,DARK_TO_LIGHT[h]) for h in hexes]
    light += ['--rgb-%s:%s' % (h,RGB_LIGHT[h]) for h in rgbs]
    attrs = []
    for a in ('fill','stroke','stop-color'):
        for h in sorted({m for m in re.findall(r'\b%s="(#[0-9a-fA-F]{3,6})"' % a, doc)}):
            attrs.append('[%s="%s"]{%s:var(--c-%s)}' % (a, h, a, _norm(h)))
    D = ';'.join(dark); L = ';'.join(light)
    return ('/* ---------- theme tokens (generated) ---------- */\n'
            ':root{color-scheme:dark;%s}\n' % D +
            ':root[data-theme="light"]{color-scheme:light;%s}\n' % L +
            '@media (prefers-color-scheme:light){:root:not([data-theme="dark"]){color-scheme:light;%s}}\n' % L +
            '@media print{:root,:root[data-theme]{color-scheme:light;%s}}\n' % L +
            '/* svg presentation attributes follow the theme */\n' + '\n'.join(attrs) + '\n')
