// SVG text-fit auditor — emits JSON with SOURCE coordinates so findings map onto the markup.
(function(){
  var kill = document.createElement('style');
  kill.textContent = '*,*::before,*::after{transition:none!important;animation:none!important}.ref>.rbox{display:none!important}';
  document.head.appendChild(kill);
  document.querySelectorAll('.reveal,.stag,.bench,.cnt').forEach(function(e){ e.classList.add('in'); });
  document.querySelectorAll('section.sec').forEach(function(e){ e.style.minHeight='auto'; });

  function attrs(el){
    var o = {};
    ['x','y','width','height','class','transform','text-anchor','font-size'].forEach(function(a){
      if (el.hasAttribute(a)) o[a] = el.getAttribute(a);
    });
    return o;
  }
  var findings = [];
  var svgs = document.querySelectorAll('section.sec svg');
  for (var s=0;s<svgs.length;s++){
    var svg = svgs[s];
    if (!svg.getClientRects().length) continue;
    var sec = svg.closest('section.sec');
    var secId = sec ? sec.id : '?';
    var g30 = svg.closest('.g30,.scene,.diagram');
    var gt = g30 ? (g30.querySelector('.gt,.dt') ? g30.querySelector('.gt,.dt').textContent.trim().slice(0,44) : '') : '';
    var sr = svg.getBoundingClientRect();
    if (sr.width < 5) continue;
    // scale: user units -> px
    var vb = svg.getAttribute('viewBox');
    var scale = 1, vbw = 0;
    if (vb){ vbw = parseFloat(vb.trim().split(/[\s,]+/)[2]); scale = sr.width / vbw; }

    // any FILLED shape is a container a label can spill out of — not just <rect>.
    // (unfilled paths are edges/arrows: they carry no box, so they are not candidates)
    var rects = [].slice.call(svg.querySelectorAll('rect,ellipse,circle,polygon,path')).filter(function(el){
      var f = getComputedStyle(el).fill;
      return f && f !== 'none' && f !== 'rgba(0, 0, 0, 0)';
    }).map(function(r){
      return { el:r, r:r.getBoundingClientRect() };
    }).filter(function(o){ return o.r.width > 8 && o.r.height > 8; });

    function faded(el){
      var n = el;
      while (n && n !== document.body){
        var cs = getComputedStyle(n);
        if (parseFloat(cs.opacity) < 0.05 || cs.visibility === 'hidden' || cs.display === 'none') return true;
        n = n.parentNode;
      }
      return false;
    }
    // rotated text: getBoundingClientRect returns an inflated axis-aligned box, so box/collision
    // maths would report phantom overlaps. Measure them, but don't judge them.
    function rotated(el){
      try { var m = el.getScreenCTM(); return m && Math.abs(m.b) > 0.004; } catch(e){ return false; }
    }
    var texts = [].slice.call(svg.querySelectorAll('text')).filter(function(t){ return t.textContent.trim() && !faded(t); });
    var placed = [];
    texts.forEach(function(te){
      var tr = te.getBoundingClientRect();
      if (tr.width < 1) return;
      if (rotated(te)) return;
      placed.push({el:te, r:tr});
      var base = { sec:secId, svg:s, gt:gt, text:te.textContent.trim(), a:attrs(te),
                   wUser:+(tr.width/scale).toFixed(1) };
      // frame clipping
      var fR = tr.right - sr.right, fL = sr.left - tr.left, fT = sr.top - tr.top, fB = tr.bottom - sr.bottom;
      if (fR > 1 || fL > 1 || fT > 1 || fB > 1){
        findings.push(Object.assign({kind:'FRAME'}, base,
          {over:{R:+(fR/scale).toFixed(1), L:+(fL/scale).toFixed(1), T:+(fT/scale).toFixed(1), B:+(fB/scale).toFixed(1)}, vbw:vbw}));
      }
      // smallest containing rect
      var cx=(tr.left+tr.right)/2, cy=(tr.top+tr.bottom)/2, best=null;
      rects.forEach(function(o){
        var R=o.r;
        if (cx>=R.left && cx<=R.right && cy>=R.top && cy<=R.bottom){
          if (!best || R.width*R.height < best.r.width*best.r.height) best=o;
        }
      });
      if (best){
        var R=best.r;
        var oR=tr.right-R.right, oL=R.left-tr.left, oT=R.top-tr.top, oB=tr.bottom-R.bottom;
        if (oR>1 || oL>1 || oT>1 || oB>1){
          findings.push(Object.assign({kind:'BOX'}, base,
            {over:{R:+(oR/scale).toFixed(1), L:+(oL/scale).toFixed(1), T:+(oT/scale).toFixed(1), B:+(oB/scale).toFixed(1)},
             box:attrs(best.el), boxW:+(R.width/scale).toFixed(1)}));
        }
      }
    });
    for (var a=0;a<placed.length;a++) for (var b=a+1;b<placed.length;b++){
      var A=placed[a].r, B=placed[b].r;
      var ix=Math.min(A.right,B.right)-Math.max(A.left,B.left);
      var iy=Math.min(A.bottom,B.bottom)-Math.max(A.top,B.top);
      if (ix>2 && iy>2){
        findings.push({kind:'COLLIDE', sec:secId, svg:s, gt:gt,
          text:placed[a].el.textContent.trim(), text2:placed[b].el.textContent.trim(),
          a:attrs(placed[a].el), b:attrs(placed[b].el),
          over:{x:+(ix/scale).toFixed(1), y:+(iy/scale).toFixed(1)}});
      }
    }
  }
  var pre = document.createElement('pre');
  pre.id='qa-report';
  pre.textContent = JSON.stringify({vw:innerWidth, n:findings.length, f:findings});
  document.body.textContent='';
  document.body.appendChild(pre);
})();
