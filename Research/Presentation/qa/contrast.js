(() => {
  function parse(c){ const m=c.match(/rgba?\(([^)]+)\)/); if(!m) return null; const p=m[1].split(/[ ,\/]+/).filter(Boolean).map(Number); return {r:p[0],g:p[1],b:p[2],a:p.length>3?p[3]:1}; }
  function lum(c){ const f=v=>{v/=255; return v<=0.03928? v/12.92 : Math.pow((v+0.055)/1.055,2.4)}; return 0.2126*f(c.r)+0.7152*f(c.g)+0.0722*f(c.b); }
  function blend(top,bot){ const a=top.a; return {r:top.r*a+bot.r*(1-a), g:top.g*a+bot.g*(1-a), b:top.b*a+bot.b*(1-a), a:1}; }
  function ratio(a,b){ const x=lum(a),y=lum(b); return (Math.max(x,y)+.05)/(Math.min(x,y)+.05); }
  function bgOf(el){ // composite backgrounds up the tree (ignores gradients/images -> treats as transparent)
    const stack=[]; let e=el;
    while(e && e.nodeType===1){ const cs=getComputedStyle(e); const c=parse(cs.backgroundColor); if(c && c.a>0) stack.push(c); if(c && c.a>=1) break; e=e.parentElement; }
    let base={r:255,g:255,b:255,a:1}; const bodyc=parse(getComputedStyle(document.body).backgroundColor); if(bodyc) base=bodyc;
    for(let i=stack.length-1;i>=0;i--) base=blend(stack[i],base);
    return base;
  }
  // svg: find the topmost filled shape under the text centre that precedes it in document order
  function svgBg(t){
    const svg=t.ownerSVGElement; const r=t.getBoundingClientRect(); const cx=r.left+r.width/2, cy=r.top+r.height/2;
    const shapes=[...svg.querySelectorAll('rect,circle,ellipse,path,polygon')];
    let best=null;
    for(const s of shapes){ if(s.compareDocumentPosition(t) & Node.DOCUMENT_POSITION_PRECEDING) continue; // s after t
      const cs=getComputedStyle(s); if(cs.fill==='none'||cs.display==='none'||+cs.opacity===0||+cs.fillOpacity===0) continue;
      const b=s.getBoundingClientRect(); if(cx<b.left||cx>b.right||cy<b.top||cy>b.bottom) continue;
      if(b.width*b.height > r.width*r.height*400 && s.tagName==='path') continue;
      const c=parse(cs.fill); if(!c) continue; c.a*=(+cs.fillOpacity)*(+cs.opacity);
      best={c, s};
    }
    let base=bgOf(svg);
    if(best) base=blend(best.c,base);
    return base;
  }
  const out=[];
  const w=document.createTreeWalker(document.querySelector('main'),NodeFilter.SHOW_TEXT);
  const seen=new Set();
  while(w.nextNode()){
    const n=w.currentNode; if(!n.textContent.trim()) continue; const el=n.parentElement; if(seen.has(el)) continue; seen.add(el);
    if(el.closest('style,script,.rbox')) continue;
    const cs=getComputedStyle(el); if(cs.visibility==='hidden'||cs.display==='none') continue;
    const r=el.getBoundingClientRect(); if(r.width===0||r.height===0) continue;
    let op=1; for(let e=el;e;e=e.parentElement){ op*=+getComputedStyle(e).opacity; } if(op<0.3) continue;
    let fg, bg, svgt=el.closest('svg');
    if(svgt){ fg=parse(cs.fill); if(!fg) continue; fg.a*=(+cs.fillOpacity); bg=svgBg(el.closest('text')||el); }
    else { fg=parse(cs.color); if(!fg) continue; bg=bgOf(el); }
    if(cs.backgroundClip==='text' || cs.webkitBackgroundClip==='text') continue;
    const f=blend(fg,bg); const cr=ratio(f,bg);
    const size=parseFloat(cs.fontSize), bold=+cs.fontWeight>=700;
    const need=(size>=24||(size>=18.66&&bold))?3:4.5;
    if(cr<need){ const sec=el.closest('section'); out.push({sec:sec?sec.id:'?', cr:+cr.toFixed(2), need, size, svg:!!svgt, cls:(el.getAttribute('class')||el.tagName), text:n.textContent.trim().slice(0,50)}); }
  }
  return out;
})()
