(function(){
  var root = document.documentElement;
  function store(k, v){ try{ if(v===null) localStorage.removeItem(k); else localStorage.setItem(k, v); }catch(e){} }
  function calm(){ return root.classList.contains('calm'); }

  // deep link ?go=id (QA + presenting); ?flat=1 renders the finished state of everything (QA / screenshots / print)
  var qp = new URLSearchParams(location.search);
  var go = qp.get('go');
  var flat = qp.has('flat');
  var details = [].slice.call(document.querySelectorAll('details'));
  if (flat){ document.querySelectorAll('.reveal,.stag,.bench').forEach(function(el){ el.classList.add('in'); });
             details.forEach(function(d){ d.open = true; }); }

  // open a collapsed evidence table when a link (or the address bar) points into it
  function reveal(id){
    var t = id && document.getElementById(id); if(!t) return null;
    var d = t.closest('details'); if (d && !d.open) d.open = true;
    if (t.tagName === 'DETAILS') t.open = true;
    return t;
  }
  function jump(id, smooth){
    var t = reveal(id); if(!t) return;
    t.scrollIntoView({behavior: (smooth && !calm()) ? 'smooth' : 'auto'});
  }
  if (go){ root.style.scrollBehavior='auto'; jump(go,false); setTimeout(function(){ root.style.scrollBehavior=''; },50); }
  else if (location.hash){ var h = location.hash.slice(1); if (reveal(h)) setTimeout(function(){ jump(h,false); }, 0); }
  addEventListener('hashchange', function(){ reveal(location.hash.slice(1)); });
  addEventListener('beforeprint', function(){ details.forEach(function(d){ d.open = true; }); });

  // graphics start when they scroll into view (text is never hidden)
  var io = new IntersectionObserver(function(es){
    es.forEach(function(e){ if(e.isIntersecting){ e.target.classList.add('in'); io.unobserve(e.target);} });
  },{threshold:.18});
  document.querySelectorAll('.reveal,.stag,.bench').forEach(function(el){ io.observe(el); });
  if (go) document.querySelectorAll('.reveal,.stag,.bench').forEach(function(el){ el.classList.add('in'); });

  // count-up numbers: <span class="cnt" data-to="19" data-dec="0" data-pre="" data-suf="×">
  function setCnt(el){ el.textContent = (el.dataset.pre||'') + parseFloat(el.dataset.to).toFixed((el.dataset.dec||0)|0) + (el.dataset.suf||''); }
  function animateCnt(el){
    if (calm() || flat){ setCnt(el); return; }
    var to = parseFloat(el.dataset.to), dec = (el.dataset.dec||0)|0, suf = el.dataset.suf||'', pre = el.dataset.pre||'';
    var dur = 1400, t0 = null;
    function step(ts){ if(!t0) t0=ts; var p=Math.min(1,(ts-t0)/dur); var e=1-Math.pow(1-p,3);
      el.textContent = pre + (to*e).toFixed(dec) + suf; if(p<1) requestAnimationFrame(step); else setCnt(el); }
    requestAnimationFrame(step);
  }
  var io3 = new IntersectionObserver(function(es){
    es.forEach(function(e){ if(e.isIntersecting){ animateCnt(e.target); io3.unobserve(e.target);} });
  },{threshold:.6});
  document.querySelectorAll('.cnt').forEach(function(el){
    if (calm() || flat || go){ setCnt(el); } else { io3.observe(el); }
  });

  // ---------- contents: scroll-spy, progress, current-chapter label ----------
  var secs = [].slice.call(document.querySelectorAll('section.sec'));
  var tocItems = {};
  document.querySelectorAll('#toc li[data-sec]').forEach(function(li){ tocItems[li.dataset.sec] = li; });
  var subs = {};   // section id -> [{el, link}]
  secs.forEach(function(s){
    var li = tocItems[s.id]; if(!li) return;
    subs[s.id] = [].slice.call(li.querySelectorAll('.sub a')).map(function(a){
      return {el: document.getElementById(a.getAttribute('href').slice(1)), link: a};
    }).filter(function(x){ return x.el; });
  });
  var prog = document.getElementById('prog'), cur = document.getElementById('cur');
  var totop = document.getElementById('totop');
  var active = -1, activeSub = null, toc = document.getElementById('toc'), side = document.getElementById('side');
  function sync(){
    var line = innerHeight * 0.3, i = 0;
    for (var k=0;k<secs.length;k++){ if (secs[k].getBoundingClientRect().top <= line) i = k; }
    var max = document.documentElement.scrollHeight - innerHeight;
    if (prog) prog.style.width = (max > 0 ? Math.min(100, scrollY / max * 100) : 0) + '%';
    if (totop) totop.classList.toggle('show', scrollY > innerHeight * 2);
    if (i !== active){
      if (active >= 0 && tocItems[secs[active].id]){ var o = tocItems[secs[active].id]; o.classList.remove('on'); o.querySelector('a').removeAttribute('aria-current'); }
      active = i;
      var li = tocItems[secs[i].id];
      if (li){
        li.classList.add('on'); li.querySelector('a').setAttribute('aria-current','true');
        for (var j=0;j<i;j++){ var p = tocItems[secs[j].id]; if (p) p.classList.add('seen'); }
        // keep the active entry visible inside the scrolling contents list
        // (never while the reader is using the list: drawer open, or pointer over it)
        if (!side.classList.contains('open') && !toc.matches(':hover')){
          var r = li.getBoundingClientRect(), tr = toc.getBoundingClientRect();
          if (r.top < tr.top + 40 || r.bottom > tr.bottom - 40) toc.scrollTop += r.top - tr.top - tr.height * 0.3;
        }
      }
      if (cur) cur.textContent = secs[i].dataset.title || '';
    }
    var list = subs[secs[i].id] || [], s = null;
    for (var m=0;m<list.length;m++){
      var el = list[m].el, top = (el.closest('details') && !el.closest('details').open && el.tagName!=='DETAILS') ? Infinity : el.getBoundingClientRect().top;
      if (top <= line) s = list[m].link;
    }
    if (s !== activeSub){ if (activeSub) activeSub.classList.remove('on'); activeSub = s; if (s) s.classList.add('on'); }
  }
  var raf = 0;
  addEventListener('scroll', function(){ if (raf) return; raf = requestAnimationFrame(function(){ raf = 0; sync(); }); }, {passive:true});
  addEventListener('resize', sync);
  sync();

  // ---------- drawer (narrow screens) ----------
  var scrim = document.getElementById('scrim'), menu = document.getElementById('menu');
  function drawer(open){
    side.classList.toggle('open', open); scrim.classList.toggle('open', open);
    menu.setAttribute('aria-expanded', open ? 'true' : 'false');
    if (open){ var a = side.querySelector('li.on>a') || side.querySelector('a'); a && a.focus({preventScroll:true}); }
  }
  menu.addEventListener('click', function(){ drawer(!side.classList.contains('open')); });
  scrim.addEventListener('click', function(){ drawer(false); });
  toc.addEventListener('click', function(ev){
    var a = ev.target.closest('a'); if (!a) return;
    var id = a.getAttribute('href').slice(1);
    reveal(id);
    if (side.classList.contains('open')) drawer(false);
  });
  // links elsewhere on the page that point into a collapsed table
  document.addEventListener('click', function(ev){
    var a = ev.target.closest && ev.target.closest('a[href^="#"]'); if (a) reveal(a.getAttribute('href').slice(1));
  });

  // ---------- reading preferences ----------
  var themebtn = document.getElementById('themebtn'), themelbl = document.getElementById('themelbl');
  function isLight(){ var t = root.getAttribute('data-theme'); return t ? t === 'light' : matchMedia('(prefers-color-scheme: light)').matches; }
  function paintTheme(){ var l = isLight(); themelbl.textContent = l ? 'Dark' : 'Light'; themebtn.setAttribute('aria-label', l ? 'Switch to dark theme' : 'Switch to light theme'); }
  themebtn.addEventListener('click', function(){ var t = isLight() ? 'dark' : 'light'; root.setAttribute('data-theme', t); store('aipo-theme', t); paintTheme(); });
  matchMedia('(prefers-color-scheme: light)').addEventListener('change', paintTheme);
  paintTheme();
  var motionbtn = document.getElementById('motionbtn'), motionlbl = document.getElementById('motionlbl');
  function paintMotion(){ var c = calm(); motionbtn.setAttribute('aria-pressed', c ? 'true' : 'false'); motionlbl.textContent = c ? 'Play motion' : 'Pause motion'; }
  motionbtn.addEventListener('click', function(){ root.classList.toggle('calm'); store('aipo-motion', calm() ? 'off' : 'on'); paintMotion(); });
  paintMotion();
  function toTop(){ scrollTo({top:0, behavior: calm() ? 'auto' : 'smooth'}); }
  if (totop) totop.querySelector('button').addEventListener('click', toTop);
  document.getElementById('topbtn').addEventListener('click', toTop);

  // ---------- keys: ←/→ jump between sections (presenting); everything else scrolls natively ----------
  addEventListener('keydown', function(ev){
    if (ev.altKey || ev.ctrlKey || ev.metaKey) return;
    var t = ev.target; if (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable) return;
    if (t.classList && t.classList.contains('ref')) return;
    if (t.closest && t.closest('.tablewrap,.evb,.tline')) return;   // let scroll regions scroll sideways
    if (ev.key === 'Escape'){ if (side.classList.contains('open')) drawer(false); return; }
    if (ev.key==='ArrowRight'){ ev.preventDefault(); secs[Math.min(active+1,secs.length-1)].scrollIntoView({behavior:calm()?'auto':'smooth'}); }
    if (ev.key==='ArrowLeft'){ ev.preventDefault(); secs[Math.max(active-1,0)].scrollIntoView({behavior:calm()?'auto':'smooth'}); }
    if (ev.key==='f' || ev.key==='F'){ if(!document.fullscreenElement){document.documentElement.requestFullscreen&&document.documentElement.requestFullscreen();} else {document.exitFullscreen();} }
  });

  // ---------- inline expandable citations ----------
  var refs = [].slice.call(document.querySelectorAll('.ref'));
  refs.forEach(function(r){
    if (!r.hasAttribute('tabindex')) r.setAttribute('tabindex','0');
    r.setAttribute('role','button');
    r.setAttribute('aria-expanded','false');
  });
  function closeRefs(except){
    refs.forEach(function(r){ if(r!==except){ r.classList.remove('open'); r.setAttribute('aria-expanded','false'); } });
  }
  function place(r){
    r.classList.remove('rt','dn');
    var box = r.querySelector('.rbox'); if(!box) return;
    var vw = innerWidth;
    var prev = box.style.cssText;
    box.style.cssText = prev + ';opacity:0;visibility:visible;transition:none';
    var b = box.getBoundingClientRect();
    if (b.right > vw - 10) r.classList.add('rt');
    if (b.top < 70) r.classList.add('dn');
    box.style.cssText = prev;
    void box.offsetHeight;
  }
  document.addEventListener('click', function(ev){
    var r = ev.target.closest ? ev.target.closest('.ref') : null;
    if (!r){ closeRefs(null); return; }
    if (ev.target.closest('.rbox')) return;   // clicks inside the popup don't toggle
    ev.preventDefault();
    var isOpen = r.classList.contains('open');
    closeRefs(r);
    if (isOpen){ r.classList.remove('open'); r.setAttribute('aria-expanded','false'); }
    else { place(r); r.classList.add('open'); r.setAttribute('aria-expanded','true'); }
  });
  addEventListener('keydown', function(ev){
    if (ev.key === 'Escape'){ closeRefs(null); if (document.activeElement && document.activeElement.classList.contains('ref')) document.activeElement.blur(); }
    if ((ev.key === 'Enter' || ev.key === ' ') && document.activeElement && document.activeElement.classList.contains('ref')){
      ev.preventDefault(); document.activeElement.click();
    }
  });
  refs.forEach(function(r){ r.addEventListener('mouseenter', function(){ place(r); }); r.addEventListener('focus', function(){ place(r); }); });
})();
