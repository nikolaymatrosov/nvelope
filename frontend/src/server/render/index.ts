// renderVisualDoc is the BFF's pure rendering function. It walks the typed
// VisualDoc tree, emits an email-ready HTML document via @react-email, and
// produces a matching plain-text alternative.
//
// The Go-side bluemonday pass is the authoritative sanitizer for what reaches
// the database (FR-014). The preview-output sanitizer in this module is the
// FR-014a guard that keeps `<script>` and other dangerous constructs out of
// the preview iframe even transiently — preview HTML is never persisted, so
// this is the only place that protection lives for the preview path.

import { createElement } from "react"
import { render } from "react-email"
import DOMPurify from "isomorphic-dompurify"

import { DocumentView } from "./components"
import type { Inline, Mark, RenderWarning, Theme, VisualBlock, VisualDoc } from "./types"

// PlatformDefaultTheme is the fallback theme used when the row's `theme` is
// null AND the BFF cannot resolve tenant branding (the route is responsible
// for fetching `GET /branding` first; this constant only exists as a
// last-ditch render-safe default for tests and dev).
export const PlatformDefaultTheme: Theme = {
  textColor: "#222222",
  linkColor: "#0066cc",
  buttonColor: "#0066cc",
  buttonTextColor: "#ffffff",
  fontFamily: "'Helvetica Neue', Helvetica, Arial, sans-serif",
  containerWidth: 600,
}

export type RenderResult = {
  bodyHtml: string
  bodyText: string
  warnings: Array<RenderWarning>
}

// renderVisualDoc returns the canonical email-ready HTML, a plain-text
// alternative, and any warnings emitted along the way. The returned HTML is
// the rendered react-email output; callers that need the preview-time
// sanitization pass (render-preview Nitro route) should run
// `sanitizePreviewHtml` over the result before responding to the SPA.
export async function renderVisualDoc(doc: VisualDoc, theme: Theme): Promise<RenderResult> {
  const element = createElement(DocumentView, { blocks: doc.content, theme })
  const bodyHtml = await render(element, { pretty: false })
  const bodyText = renderPlainText(doc.content, theme)
  return { bodyHtml, bodyText, warnings: [] }
}

// sanitizePreviewHtml strips dangerous constructs from HTML that is about to
// be loaded into the preview iframe. It does NOT replace Go's bluemonday
// pass; that runs over the same HTML server-side before persistence (FR-014).
// Returned warnings list every removed element / attribute so the SPA can
// surface a non-blocking notice (FR-014a).
export function sanitizePreviewHtml(html: string): { html: string; warnings: Array<RenderWarning> } {
  const warnings: Array<RenderWarning> = []
  const removed: Array<string> = []
  const hookId = "preview-track"
  // DOMPurify's removed[] array is a global per-instance — collect with a
  // hook so we can attribute each strip to a stable warning kind.
  DOMPurify.addHook("uponSanitizeElement", (_node, data) => {
    if (!data.allowedTags[data.tagName]) {
      removed.push(`<${data.tagName}>`)
    }
  })
  DOMPurify.addHook("uponSanitizeAttribute", (_node, data) => {
    if (!data.allowedAttributes[data.attrName] && data.attrName.startsWith("on")) {
      removed.push(`event-handler:${data.attrName}`)
    }
  })
  try {
    const clean = DOMPurify.sanitize(html, {
      FORBID_TAGS: ["script", "style", "iframe", "object", "embed", "form", "input", "link"],
      ALLOWED_URI_REGEXP: /^(?:https?|mailto|tel|cid):/i,
    })
    for (const r of removed) {
      warnings.push({ kind: "sanitizer_stripped", detail: `Removed ${r} from preview` })
    }
    return { html: typeof clean === "string" ? clean : String(clean), warnings }
  } finally {
    DOMPurify.removeHook("uponSanitizeElement")
    DOMPurify.removeHook("uponSanitizeAttribute")
  }
  void hookId
}

// ── Plain-text walker ──────────────────────────────────────────────────────

function renderPlainText(blocks: Array<VisualBlock>, theme: Theme): string {
  return blocks
    .map((b) => plainBlock(b, theme))
    .filter((s) => s.length > 0)
    .join("\n\n")
}

function plainBlock(block: VisualBlock, theme: Theme): string {
  switch (block.type) {
    case "paragraph":
      return plainInlines(block.content)
    case "heading": {
      const prefix = "#".repeat(block.attrs.level) + " "
      return prefix + plainInlines(block.content)
    }
    case "bulletList":
      return block.content
        .map((item) => "- " + item.content.map((c) => plainBlock(c, theme)).join("\n"))
        .join("\n")
    case "orderedList":
      return block.content
        .map((item, i) => `${i + 1}. ` + item.content.map((c) => plainBlock(c, theme)).join("\n"))
        .join("\n")
    case "blockquote":
      return block.content
        .map((c) => "> " + plainBlock(c, theme))
        .join("\n")
    case "codeBlock":
      return block.content.map((t) => t.text).join("")
    case "image":
      return `[image: ${block.attrs.alt || block.attrs.mediaRef}]`
    case "button":
      return `[ ${block.attrs.label} ] (${block.attrs.href})`
    case "divider":
      return "----"
    case "columns":
      return block.content
        .map((col) => col.content.map((c) => plainBlock(c, theme)).join("\n\n"))
        .join("\n\n")
    case "rawHtml":
      // Crude HTML→text fallback: strip tags. The Go sanitizer has already
      // run by the time this lands in `body_text` for a persisted row.
      return block.attrs.html.replace(/<[^>]+>/g, "").trim()
    case "listItem":
    case "column":
      // Reached only via a parent list/columns block; never at the top
      // level. The validator rejects an envelope that puts these here.
      return ""
  }
}

function plainInlines(inlines: Array<Inline>): string {
  return inlines
    .map((i) => {
      if (i.type === "mergeTag") {
        return `{{ ${i.attrs.namespace}.${i.attrs.key} }}`
      }
      // Link marks are surfaced as `text (url)` so the operator can verify
      // the URL is correct in a plain-text viewer.
      const link = i.marks?.find(
        (m): m is Extract<Mark, { type: "link" }> => m.type === "link",
      )
      if (link) {
        return `${i.text} (${link.attrs.href})`
      }
      return i.text
    })
    .join("")
}
