"""Content-preservation check: every text node of the original must survive in the new file.
usage: textcheck.py old.html new.html [allowlist.txt]
Reports text nodes lost (count dropped) and text nodes added (new UI copy / intentional duplicates)."""
import sys, re, collections
from html.parser import HTMLParser
class T(HTMLParser):
    def __init__(s):
        super().__init__(convert_charrefs=True); s.skip=0; s.c=collections.Counter(); s.attrs=collections.Counter()
    def handle_starttag(s,t,a):
        if t in ('style','script'): s.skip+=1
        for k,v in a:
            if k in ('title','aria-label','alt') and v: s.attrs[(k,' '.join(v.split()))]+=1
    def handle_endtag(s,t):
        if t in ('style','script'): s.skip-=1
    def handle_data(s,d):
        if s.skip: return
        d=' '.join(d.split())
        if d: s.c[d]+=1
def load(p):
    t=T(); t.feed(open(p).read()); return t
a,b=load(sys.argv[1]),load(sys.argv[2])
allow=set()
if len(sys.argv)>3: allow={l.strip() for l in open(sys.argv[3]) if l.strip()}
lost=a.c-b.c; added=b.c-a.c
print('text nodes old %d new %d'%(sum(a.c.values()),sum(b.c.values())))
print('chars old %d new %d'%(sum(len(k)*v for k,v in a.c.items()),sum(len(k)*v for k,v in b.c.items())))
print('LOST (%d):'%sum(lost.values()))
for k,v in lost.most_common(): print('  -%d  %r'%(v,k[:140]))
add=[(k,v) for k,v in added.items() if k not in allow]
dup=[(k,v) for k,v in add if k in a.c]
new=[(k,v) for k,v in add if k not in a.c]
print('ADDED copies of existing text (%d nodes) — e.g. the at-a-glance list reusing the close recap'%sum(v for k,v in dup))
print('ADDED new text (%d nodes):'%sum(v for k,v in new))
for k,v in new: print('  +%d  %r'%(v,k[:160]))
la=a.attrs-b.attrs
if la: print('ATTR text lost:', list(la.items())[:20])
sys.exit(1 if lost else 0)
