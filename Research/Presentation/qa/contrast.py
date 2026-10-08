import sys, asyncio, json, collections
from playwright.async_api import async_playwright
path=__import__('os').path.abspath(sys.argv[1]); js=open(__import__('os').path.join(__import__('os').path.dirname(__import__('os').path.abspath(__file__)),'contrast.js')).read()
async def main():
    async with async_playwright() as p:
        b=await p.chromium.launch(executable_path='/usr/bin/google-chrome',args=['--no-sandbox'])
        for scheme in sys.argv[2:]:
            pg=await b.new_page(viewport={'width':1440,'height':1000},reduced_motion='reduce',color_scheme=scheme)
            await pg.goto('file://'+path+'?flat=1'); await pg.wait_for_timeout(800)
            r=await pg.evaluate(js)
            print(f'== {scheme}: {len(r)} text elements below WCAG AA')
            byk=collections.Counter((x['sec'],x['cls'],x['svg']) for x in r)
            for x in sorted(r,key=lambda x:x['cr'])[:400]:
                pass
            agg=collections.defaultdict(list)
            for x in r: agg[(x['cls'],x['svg'])].append(x)
            for (cls,svg),xs in sorted(agg.items(), key=lambda kv:-len(kv[1])):
                worst=min(xs,key=lambda x:x['cr'])
                secs=sorted({x['sec'] for x in xs})
                print(f"  {len(xs):3}x {'SVG ' if svg else ''}{cls[:38]:38} worst {worst['cr']} (need {worst['need']}, {worst['size']}px) {','.join(secs)[:50]}  e.g. {worst['text']!r}")
            await pg.close()
        await b.close()
asyncio.run(main())
