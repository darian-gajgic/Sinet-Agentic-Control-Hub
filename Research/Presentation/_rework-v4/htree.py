"""Offset-preserving HTML element tree (no rewriting of untouched markup)."""
import re
from html.parser import HTMLParser
VOID={'br','img','input','meta','hr','wbr','source','col','link','area','base','embed','param','track'}
class Node:
    __slots__=('tag','attrs','start','end','kids','parent','stag_end')
    def __init__(s,tag,attrs,start,parent):
        s.tag=tag; s.attrs=attrs; s.start=start; s.end=None; s.kids=[]; s.parent=parent; s.stag_end=None
    @property
    def cls(s): return (s.attrs.get('class') or '').split()
    def has(s,c): return c in s.cls
    def find(s,pred):
        out=[]
        def rec(n):
            for k in n.kids:
                if pred(k): out.append(k)
                rec(k)
        rec(s); return out
    def __repr__(s): return f'<{s.tag} {s.attrs.get("class","")} {s.start}-{s.end}>'
class _P(HTMLParser):
    def __init__(s,src):
        super().__init__(convert_charrefs=False); s.src=src
        s.lines=[0]+[m.end() for m in re.finditer('\n',src)]
        s.root=Node('#root',{},0,None); s.stack=[s.root]
    def off(s): l,c=s.getpos(); return s.lines[l-1]+c
    def handle_starttag(s,t,a):
        o=s.off(); n=Node(t,dict(a),o,s.stack[-1]); n.stag_end=o+len(s.get_starttag_text())
        s.stack[-1].kids.append(n)
        if t in VOID: n.end=n.stag_end
        else: s.stack.append(n)
    def handle_startendtag(s,t,a):
        o=s.off(); n=Node(t,dict(a),o,s.stack[-1]); n.stag_end=n.end=o+len(s.get_starttag_text())
        s.stack[-1].kids.append(n)
    def handle_endtag(s,t):
        if t in VOID: return
        o=s.off()
        while len(s.stack)>1:
            n=s.stack.pop()
            if n.tag==t: n.end=o+len(s.src[o:].split('>',1)[0])+1; return
            raise ValueError(f'unclosed {n} before </{t}> at {o}')
def parse(src):
    p=_P(src); p.feed(src); p.close(); p.root.end=len(src); return p.root
def inner(src,n):
    """source between start tag end and end tag start"""
    return src[n.stag_end:src.rindex('</',n.stag_end,n.end)]
