import sys, asyncio
from playwright.async_api import async_playwright
JS='''()=>{
 const out=[]; const boxed=e=>{const cs=getComputedStyle(e); return parseFloat(cs.borderTopWidth)>0||parseFloat(cs.borderLeftWidth)>0||(cs.backgroundColor!=='rgba(0, 0, 0, 0)'&&cs.backgroundColor!=='transparent')};
 const els=[...document.querySelectorAll('.g30 *, .tm-vig *')].filter(e=>!e.closest('svg') && [...e.childNodes].some(n=>n.nodeType===3&&n.textContent.trim()));
 for(const e of els){
   const cs=getComputedStyle(e); if(cs.display==='none'||cs.visibility==='hidden'||cs.position==='absolute'||cs.position==='fixed') continue;
   if(e.closest('.rbox')) continue;
   // own clipping
   if((cs.overflowX==='hidden'||cs.overflow==='hidden') && (e.scrollWidth>e.clientWidth+2||e.scrollHeight>e.clientHeight+2)) out.push(['CLIP',e]);
   // text escaping its nearest boxed ancestor
   let a=e.parentElement; while(a && !boxed(a)) a=a.parentElement; if(!a) continue;
   if(getComputedStyle(a).overflowX==='auto'||getComputedStyle(a).overflowX==='scroll') continue;
   const ar=a.getBoundingClientRect(); let rr={left:1e9,right:-1e9,bottom:-1e9,width:0};
   for(const n of e.childNodes){ if(n.nodeType!==3||!n.textContent.trim()) continue; const r=document.createRange(); r.selectNodeContents(n);
     for(const q of r.getClientRects()){ if(q.width<1) continue; rr.left=Math.min(rr.left,q.left); rr.right=Math.max(rr.right,q.right); rr.bottom=Math.max(rr.bottom,q.bottom); rr.width=1; } }
   if(rr.width===0) continue;
   if(rr.right>ar.right+2||rr.left<ar.left-2||rr.bottom>ar.bottom+2) out.push(['ESC',e,Math.round(Math.max(rr.right-ar.right,ar.left-rr.left,rr.bottom-ar.bottom)), a]);
 }
 return out.map(x=>{const e=x[1]; const s=e.closest('section'); return [x[0], s?s.id:'?', (e.className||e.tagName)+'', x[2]||0, (x[3]? (x[3].className||x[3].tagName)+'':''), e.textContent.trim().slice(0,40)]});
}'''
async def main():
    async with async_playwright() as p:
        b=await p.chromium.launch(executable_path='/usr/bin/google-chrome',args=['--no-sandbox'])
        for path in [__import__('os').path.abspath(a) for a in sys.argv[1:]]:
            for w in (1440,1280):
                pg=await b.new_page(viewport={'width':w,'height':900},reduced_motion='reduce',color_scheme='dark')
                await pg.goto('file://'+path+'?flat=1'); await pg.wait_for_timeout(700)
                r=await pg.evaluate(JS)
                print(f'== {path.split("/")[-2]} @{w}: {len(r)}')
                for x in r: print('  ',x)
                await pg.close()
        await b.close()
asyncio.run(main())
