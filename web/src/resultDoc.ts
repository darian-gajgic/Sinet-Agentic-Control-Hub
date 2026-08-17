/**
 * resultDoc.ts — composing THE RESULT for the review surface (RA-B1).
 *
 * The re-walk proved the journey's final moment: the owner arrives at a
 * finished deliverable and must SEE the thing that was made — a website as a
 * page, a document as a document — not a raw diff. This module is the pure
 * composition half of that moment: given the current revision's text files, it
 * answers "what should the rendered-document frame show?".
 *
 * TWO RULES BOUND EVERYTHING HERE:
 *
 *  - ESCAPE FIRST (S13.3 / S15.12). Markdown prose is untrusted model output.
 *    Every character of it is HTML-escaped BEFORE any markdown formatting is
 *    applied, so the formatting pass operates on inert text and can never
 *    resurrect markup the author wrote. The one place source markup passes
 *    through unescaped is a document the work itself shipped (an .html file, or
 *    a full HTML document inside a fenced block) — and that renders ONLY inside
 *    the sandboxed rendered-document frame, the channel S13.3 itself sanctions,
 *    with every sandbox permission withheld (see documentSandbox in
 *    Deliverable.tsx and the escape-scan pin).
 *
 *  - NO DOM IN THIS MODULE. The frame, the blob URL and the sandbox attribute
 *    live in Deliverable.tsx — the ONE file the escape scan permits to embed a
 *    browsing context. This module is strings in, strings out, so the whole
 *    composition is testable without a browser.
 */

export type ResultFile = { name: string; text: string }

export type ComposedResult =
  /** A real document: render it in the sandboxed rendered-document frame. */
  | { kind: 'document'; html: string; how: string }
  /** Plain text: render it as text, no frame needed. */
  | { kind: 'text'; text: string; how: string }

/** One fenced code block of a markdown source. */
type Fence = { lang: string; body: string }

/**
 * composeResult decides what the result view shows for one revision's text
 * files. Priority order, most-faithful first:
 *
 *  1. A shipped .html file IS a page — render it, with shipped .css inlined.
 *  2. A markdown file whose fenced blocks carry a full HTML document (the
 *     "website delivered as markdown" shape the classifier's CONTENT-family
 *     misfile produces, RA-11) — render THAT page, fenced CSS inlined.
 *  3. Any other markdown — render the markdown itself, escape-first.
 *  4. Anything else — plain text.
 */
export function composeResult(files: ResultFile[], dtype: string): ComposedResult | null {
  if (files.length === 0) return null

  const htmlFile = files.find((f) => /\.html?$/i.test(f.name))
  if (htmlFile !== undefined) {
    const css = files.filter((f) => /\.css$/i.test(f.name))
    return {
      kind: 'document',
      html: inlineCss(htmlFile.text, css),
      how: `the page the work shipped (${htmlFile.name})`,
    }
  }

  // Markdown renders as a document only where markdown IS the deliverable: the
  // type says so, or the revision is one lone .md file. A code deliverable that
  // happens to carry a README beside its source is source — rendering its
  // README as "the work" would be a prettier lie than the diff ever told.
  const mdCandidate = files.find((f) => /\.(md|markdown)$/i.test(f.name))
  const mdFile =
    dtype === 'markdown'
      ? (mdCandidate ?? files[0])
      : files.length === 1 && mdCandidate !== undefined
        ? mdCandidate
        : undefined
  if (mdFile !== undefined) {
    const fences = extractFences(mdFile.text)
    const page = fences.find((f) => (f.lang === 'html' || f.lang === 'htm') && looksLikeDocument(f.body))
    if (page !== undefined) {
      const css = fences.filter((f) => f.lang === 'css').map((f) => ({ name: '', text: f.body }))
      const shippedCss = files.filter((f) => /\.css$/i.test(f.name))
      return {
        kind: 'document',
        html: inlineCss(page.body, [...css, ...shippedCss]),
        how: 'the web page inside the delivered file, shown with its stylesheet applied',
      }
    }
    return {
      kind: 'document',
      html: markdownDocument(mdFile.text),
      how: `the delivered document (${mdFile.name}), rendered`,
    }
  }

  return {
    kind: 'text',
    text: files.map((f) => (files.length === 1 ? f.text : `── ${f.name} ──\n\n${f.text}`)).join('\n\n'),
    how: 'the delivered text, as written',
  }
}

/** A fence body that IS a full HTML document rather than a snippet. */
function looksLikeDocument(body: string): boolean {
  return /^\s*<(!doctype\s+html|html)\b/i.test(body)
}

/**
 * extractFences finds every ``` fenced block. Line-based on purpose: a regex
 * across the whole text mispairs fences whenever a body contains backticks,
 * and a parser that walks lines cannot.
 */
export function extractFences(md: string): Fence[] {
  const fences: Fence[] = []
  let open: { lang: string; lines: string[] } | null = null
  for (const line of md.split('\n')) {
    const mark = /^\s*```(\S*)\s*$/.exec(line)
    if (mark !== null) {
      if (open === null) {
        open = { lang: mark[1].toLowerCase(), lines: [] }
      } else {
        fences.push({ lang: open.lang, body: open.lines.join('\n') })
        open = null
      }
      continue
    }
    if (open !== null) open.lines.push(line)
  }
  return fences
}

/**
 * inlineCss folds stylesheets into a page so the sandboxed frame — which has no
 * server to ask for a sibling styles.css — shows the page AS DESIGNED.
 *
 * A stylesheet with a name replaces the <link> that references that name; a
 * nameless one (a fenced ```css block) replaces the first stylesheet link,
 * because in the delivered-as-markdown shape the fence IS the file the page's
 * one link points at. Whatever finds no link to replace is added to <head> —
 * styling the page is what the stylesheet was shipped FOR.
 */
export function inlineCss(html: string, css: { name: string; text: string }[]): string {
  let out = html
  const leftovers: string[] = []
  for (const sheet of css) {
    const style = `<style>\n${sheet.text}\n</style>`
    const base = sheet.name.split('/').pop() ?? ''
    const link = findStylesheetLink(out, base)
    if (link !== null) {
      out = out.slice(0, link.start) + style + out.slice(link.end)
    } else {
      leftovers.push(style)
    }
  }
  if (leftovers.length > 0) {
    const joined = leftovers.join('\n')
    const head = /<\/head>/i.exec(out)
    if (head !== null) {
      out = out.slice(0, head.index) + joined + '\n' + out.slice(head.index)
    } else {
      out = joined + '\n' + out
    }
  }
  return out
}

/** findStylesheetLink locates a <link rel="stylesheet"> — the one referencing
 *  `base` when a name is given, the first one when not. */
function findStylesheetLink(html: string, base: string): { start: number; end: number } | null {
  const links = /<link\b[^>]*>/gi
  for (let m = links.exec(html); m !== null; m = links.exec(html)) {
    const tag = m[0]
    if (!/rel\s*=\s*["']?stylesheet["']?/i.test(tag)) continue
    if (base !== '') {
      const href = /href\s*=\s*["']?([^"'\s>]+)/i.exec(tag)
      const linkedBase = (href?.[1] ?? '').split(/[?#]/)[0].split('/').pop() ?? ''
      if (linkedBase.toLowerCase() !== base.toLowerCase()) continue
    }
    return { start: m.index, end: m.index + tag.length }
  }
  return null
}

// ── the escape-first markdown rendering ──────────────────────────────────────

/** escapeHtml makes text inert. It runs BEFORE any formatting below, which is
 *  the whole safety argument: formatting operates on escaped text only. */
export function escapeHtml(s: string): string {
  return s.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;')
}

/** A link/image target is kept only in the shapes a document legitimately
 *  links: web, mail, an in-page anchor or a relative path. Anything else —
 *  javascript:, data:text/html, vbscript: — renders as text, not as a href. */
function safeUrl(url: string): string | null {
  if (/^(https?:|mailto:|#|\/|\.\/|\.\.\/)/i.test(url)) return url
  if (/^[\w./-]+$/.test(url)) return url // bare relative path
  return null
}

/**
 * markdownDocument renders markdown as a readable document, escape-first.
 *
 * A deliberate SUBSET: headings, paragraphs, lists, blockquotes, rules, code
 * (fenced and inline), emphasis, links and images. What the subset does not
 * know renders as the escaped text it is — the failure mode is plain-looking
 * prose, never markup execution. Tables and footnotes wait for a real need.
 */
export function markdownToHtml(md: string): string {
  const out: string[] = []
  const lines = md.split('\n')
  let i = 0
  let paragraph: string[] = []
  let list: { ordered: boolean; items: string[] } | null = null
  let quote: string[] = []

  const flushParagraph = () => {
    if (paragraph.length > 0) {
      out.push(`<p>${inline(paragraph.join(' '))}</p>`)
      paragraph = []
    }
  }
  const flushList = () => {
    if (list !== null) {
      const tag = list.ordered ? 'ol' : 'ul'
      out.push(`<${tag}>${list.items.map((it) => `<li>${inline(it)}</li>`).join('')}</${tag}>`)
      list = null
    }
  }
  const flushQuote = () => {
    if (quote.length > 0) {
      out.push(`<blockquote><p>${inline(quote.join(' '))}</p></blockquote>`)
      quote = []
    }
  }
  const flushAll = () => {
    flushParagraph()
    flushList()
    flushQuote()
  }

  while (i < lines.length) {
    const line = lines[i]

    const fence = /^\s*```(\S*)\s*$/.exec(line)
    if (fence !== null) {
      flushAll()
      const body: string[] = []
      i++
      while (i < lines.length && !/^\s*```\s*$/.test(lines[i])) {
        body.push(lines[i])
        i++
      }
      i++ // past the closing fence (or the end)
      out.push(`<pre><code>${escapeHtml(body.join('\n'))}</code></pre>`)
      continue
    }

    if (/^\s*$/.test(line)) {
      flushAll()
      i++
      continue
    }

    const heading = /^(#{1,6})\s+(.*)$/.exec(line)
    if (heading !== null) {
      flushAll()
      const level = heading[1].length
      out.push(`<h${level}>${inline(heading[2])}</h${level}>`)
      i++
      continue
    }

    if (/^\s*(-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
      flushAll()
      out.push('<hr>')
      i++
      continue
    }

    const quoted = /^\s*>\s?(.*)$/.exec(line)
    if (quoted !== null) {
      flushParagraph()
      flushList()
      quote.push(quoted[1])
      i++
      continue
    }

    const bullet = /^\s*[-*+]\s+(.*)$/.exec(line)
    const numbered = /^\s*\d+[.)]\s+(.*)$/.exec(line)
    if (bullet !== null || numbered !== null) {
      flushParagraph()
      flushQuote()
      const ordered = numbered !== null
      const item = (numbered ?? bullet)?.[1] ?? ''
      if (list === null || list.ordered !== ordered) {
        flushList()
        list = { ordered, items: [] }
      }
      list.items.push(item)
      i++
      continue
    }

    flushList()
    flushQuote()
    paragraph.push(line.trim())
    i++
  }
  flushAll()
  return out.join('\n')
}

/**
 * inline formats ONE escaped line: code spans first (their contents are
 * protected from further formatting), then images, links, bold, italic.
 * The input is escaped inside this function — callers pass raw text exactly
 * once, here, and everything after operates on inert characters.
 */
function inline(raw: string): string {
  const text = escapeHtml(raw)
  // Code spans are split out FIRST so no formatting reaches inside them.
  const parts = text.split(/(`+)([^`]*?)\1/)
  const rendered: string[] = []
  for (let p = 0; p < parts.length; p++) {
    if ((p - 2) % 3 === 0 && p >= 2) {
      rendered.push(`<code>${parts[p]}</code>`)
    } else if ((p - 1) % 3 === 0 && p >= 1) {
      // the backtick run itself — consumed by the code span
    } else {
      rendered.push(formatSpans(parts[p]))
    }
  }
  return rendered.join('')
}

/** Emphasis, links and images over an already-escaped span. */
function formatSpans(escaped: string): string {
  let s = escaped
  s = s.replace(/!\[([^\]]*)\]\(([^)\s]+)\)/g, (whole, alt: string, url: string) => {
    const safe = safeUrl(url)
    return safe === null ? whole : `<img src="${safe}" alt="${alt}">`
  })
  s = s.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (whole, label: string, url: string) => {
    const safe = safeUrl(url)
    return safe === null ? whole : `<a href="${safe}">${label}</a>`
  })
  s = s.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  s = s.replace(/(^|\W)\*([^*\s][^*]*)\*(?=\W|$)/g, '$1<em>$2</em>')
  return s
}

/**
 * markdownDocument wraps rendered markdown in a small self-contained reading
 * document. The frame paints its own world, so this is a deliberate fixed
 * light reading theme — a document, not a themed app surface.
 */
export function markdownDocument(md: string): string {
  return [
    '<!doctype html>',
    '<html><head><meta charset="utf-8">',
    '<meta name="viewport" content="width=device-width, initial-scale=1">',
    '<style>',
    'body{margin:0 auto;max-width:46rem;padding:2.5rem 1.5rem;background:#fffdf8;color:#1c1917;',
    'font:16px/1.65 system-ui,-apple-system,"Segoe UI",sans-serif}',
    'h1,h2,h3,h4{line-height:1.25}',
    'pre{overflow-x:auto;background:#f4f0e8;border:1px solid #e2dccc;border-radius:6px;padding:.85rem 1rem}',
    'code{font:.9em/1.5 ui-monospace,monospace}',
    'blockquote{margin:0;padding-left:1rem;border-left:3px solid #d9c9ae;color:#57534e}',
    'img{max-width:100%;height:auto}',
    'a{color:#8250df}',
    'hr{border:none;border-top:1px solid #e2dccc;margin:2rem 0}',
    '</style></head><body>',
    markdownToHtml(md),
    '</body></html>',
  ].join('\n')
}

/** humanBytes says a size the way a person reads one. */
export function humanBytes(n: number): string {
  if (n < 1024) return `${String(n)} bytes`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

/**
 * resultSizeCap bounds what the result view will FETCH to compose its render.
 * A deliverable is reviewed by a person; a text payload past a few megabytes
 * is not going to be read in a frame, and fetching it would only stall the
 * page — past the cap the view offers the download and says so.
 */
export const resultSizeCap = 4 * 1024 * 1024
