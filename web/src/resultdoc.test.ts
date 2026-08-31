import { expect, test } from 'vitest'

import { composeResult, escapeHtml, extractFences, humanBytes, inlineCss, markdownToHtml } from './resultDoc'

/**
 * The RA-B1 composition rules, driven at the shapes the walks actually
 * produced. The live fixture's shape — a WEBSITE delivered as one markdown
 * file wrapping an ```html document and a ```css stylesheet (the CONTENT-family
 * misfile RA-11 records) — is the first citizen here, because it is the shape
 * the owner's blocked moment wore.
 */

const aliceShape = [
  '```html',
  '<!DOCTYPE html>',
  '<html lang="en">',
  '<head>',
  '  <title>Amber &amp; Oak</title>',
  '  <link rel="stylesheet" href="styles.css">',
  '</head>',
  '<body><h1>Amber &amp; Oak Photography</h1></body>',
  '</html>',
  '```',
  '',
  '```css',
  'body { background: #faf3e9; }',
  '```',
].join('\n')

test('the delivered-website markdown composes into ONE page with its stylesheet inlined', () => {
  const out = composeResult([{ name: 'deliverable.md', text: aliceShape }], 'markdown')
  expect(out).not.toBeNull()
  if (out === null || out.kind !== 'document') throw new Error('expected a document')
  // The page is the fence's document, not the markdown wrapper.
  expect(out.html).toContain('<h1>Amber &amp; Oak Photography</h1>')
  // The stylesheet link is REPLACED by the fenced css — the frame has no
  // server to ask for styles.css, so an unreplaced link is an unstyled page.
  expect(out.html).toContain('background: #faf3e9')
  expect(out.html).not.toContain('<link rel="stylesheet"')
  // And nothing of the fence markers leaks into the document.
  expect(out.html).not.toContain('```')
})

test('a shipped .html file wins over everything and inlines its named stylesheet', () => {
  const out = composeResult(
    [
      { name: 'site/index.html', text: '<html><head><link rel="stylesheet" href="a.css"></head><body>hi</body></html>' },
      { name: 'site/a.css', text: 'body{color:red}' },
    ],
    'code',
  )
  if (out === null || out.kind !== 'document') throw new Error('expected a document')
  expect(out.html).toContain('body{color:red}')
  expect(out.html).not.toContain('<link')
})

test('a stylesheet no link references still styles the page: it is injected into head', () => {
  const out = composeResult(
    [
      { name: 'p.html', text: '<html><head><title>t</title></head><body>x</body></html>' },
      { name: 'extra.css', text: 'b{font-weight:bold}' },
    ],
    'code',
  )
  if (out === null || out.kind !== 'document') throw new Error('expected a document')
  expect(out.html).toContain('b{font-weight:bold}')
  expect(out.html.indexOf('b{font-weight:bold}')).toBeLessThan(out.html.indexOf('</head>'))
})

test('plain markdown renders as a document, escape-first: hostile markup stays text', () => {
  const hostile = '# Title\n\nHello <script>alert(1)</script> world\n\n- a **bold** item\n'
  const out = composeResult([{ name: 'notes.md', text: hostile }], 'markdown')
  if (out === null || out.kind !== 'document') throw new Error('expected a document')
  expect(out.html).toContain('<h1>Title</h1>')
  expect(out.html).toContain('<strong>bold</strong>')
  // The planted script arrives ESCAPED — never as an element.
  expect(out.html).toContain('&lt;script&gt;')
  expect(out.html).not.toContain('<script>')
})

test('a code deliverable with a README beside its source is SOURCE, not a rendered README', () => {
  const out = composeResult(
    [
      { name: 'site/README.md', text: '# readme' },
      { name: 'site/release.tsx', text: 'export const x = 1' },
    ],
    'code',
  )
  if (out === null) throw new Error('expected a result')
  expect(out.kind).toBe('text')
  expect(out.kind === 'text' && out.text).toContain('export const x = 1')
})

test('a lone .md file renders as a document even under a non-markdown type', () => {
  const out = composeResult([{ name: 'REPORT.md', text: '## Findings\n\nfine' }], 'text')
  if (out === null || out.kind !== 'document') throw new Error('expected a document')
  expect(out.html).toContain('<h2>Findings</h2>')
})

test('markdown link targets are kept only in legitimate shapes', () => {
  const html = markdownToHtml('[ok](https://example.com) and [bad](javascript:alert(1))')
  expect(html).toContain('href="https://example.com"')
  // The rejected target stays visible as the inert TEXT it is — honest about
  // what the author wrote — but never becomes a navigable attribute.
  expect(html).not.toContain('href="javascript:')
  expect(html).toContain('[bad](javascript:alert(1))')
})

test('inline code protects its contents from formatting', () => {
  const html = markdownToHtml('use `**not bold**` here')
  expect(html).toContain('<code>**not bold**</code>')
  expect(html).not.toContain('<strong>')
})

test('fence extraction pairs fences line-wise and keeps bodies verbatim', () => {
  const fences = extractFences('pre\n```css\na { b: c }\n```\npost\n```\nplain\n```\n')
  expect(fences).toEqual([
    { lang: 'css', body: 'a { b: c }' },
    { lang: '', body: 'plain' },
  ])
})

test('inlineCss with a NAMED sheet replaces only the link that references it', () => {
  const html = '<head><link rel="stylesheet" href="x/other.css"><link rel="stylesheet" href="mine.css"></head>'
  const out = inlineCss(html, [{ name: 'css/mine.css', text: 'q{r:s}' }])
  expect(out).toContain('href="x/other.css"')
  expect(out).not.toContain('href="mine.css"')
  expect(out).toContain('q{r:s}')
})

// ── the pipe-table pass (review M9: the flagship deliverable WAS a table) ──

const priceTable = [
  '# Candles',
  '',
  '| Candle | Description | Price |',
  '| --- | :---: | ---: |',
  '| Winter Pine | fresh forest | 8 € |',
  '| Amber Glow | warm evening | 11 € |',
].join('\n')

test('a pipe table renders as a real table, alignment from the delimiter row alone', () => {
  const html = markdownToHtml(priceTable)
  expect(html).toContain('<table>')
  expect(html).toContain('<th>Candle</th>')
  expect(html).toContain('<th style="text-align:center">Description</th>')
  expect(html).toContain('<th style="text-align:right">Price</th>')
  expect(html).toContain('<td>Winter Pine</td>')
  expect(html).toContain('<td style="text-align:right">11 €</td>')
  // No raw pipe syntax survives into the rendered document…
  expect(html).not.toContain('| Candle |')
  // …and the wide-table wrapper scrolls inside itself rather than widening a phone.
  expect(html).toContain('<div class="tablewrap">')
})

test('table cells stay escape-first and still carry inline formatting', () => {
  const html = markdownToHtml('| a | b |\n| --- | --- |\n| <script>x</script> | **bold** [ok](https://e.com) |')
  expect(html).toContain('&lt;script&gt;')
  expect(html).not.toContain('<script>')
  expect(html).toContain('<strong>bold</strong>')
  expect(html).toContain('href="https://e.com"')
})

test('what is not a table stays the escaped text it is', () => {
  // A delimiter row whose cell count disagrees with the header is NOT a
  // table — the strict recognition fails to plain prose, never to guesswork.
  const html = markdownToHtml('| a | b |\n| --- |\n| c | d |')
  expect(html).not.toContain('<table>')
  expect(html).toContain('| a | b |')
  // A lone pipe in prose is prose.
  expect(markdownToHtml('either | or')).not.toContain('<table>')
})

test('ragged body rows keep every served cell: short rows pad, long rows keep their extras', () => {
  const html = markdownToHtml('| a | b |\n| --- | --- |\n| only |\n| x | y | extra |')
  expect(html).toContain('<td>only</td><td></td>')
  expect(html).toContain('<td>extra</td>')
})

test('escapeHtml makes the four live characters inert', () => {
  expect(escapeHtml('<a href="x">&</a>')).toBe('&lt;a href=&quot;x&quot;&gt;&amp;&lt;/a&gt;')
})

test('humanBytes reads like a person says it', () => {
  expect(humanBytes(45)).toBe('45 bytes')
  expect(humanBytes(9301)).toBe('9.1 KB')
  expect(humanBytes(3 * 1024 * 1024)).toBe('3.0 MB')
})
