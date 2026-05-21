// T101 — BFF render integration test: every <img src=…> emitted by the
// canonical render pipeline matches the tenant media-library URL pattern
// (FR-021). The visual editor inserts images exclusively through the
// MediaPicker / drag-paste upload paths, both of which yield a tenant
// media URL; the renderer must preserve that URL verbatim and must never
// inline data: URIs or rewrite to third-party hosts. The pattern covers
// images at top level, inside columns, and inside other block containers
// that legally accept them.
//
// The companion guardrail at validate-time lives in
// frontend/src/server/validate/blocks.test.ts (rejects non-prefix
// mediaRefs); this test pins the render-side property so a regression in
// the components mapping can't silently smuggle in a data URI.

import { describe, expect, it } from "vitest"
import { PlatformDefaultTheme, renderVisualDoc } from "./index"
import type { VisualDoc } from "./types"

// Mirror of `frontend/src/server/validate/blocks.test.ts` — the tenant
// media prefix used by the render fixtures. Real deployments resolve this
// from `process.env.OBJECT_STORAGE_PUBLIC_BASE_URL`; the test pins a
// canonical value so the regex assertion is precise.
const TENANT_MEDIA_PREFIX = "https://media.test/tenants/abc/"
const TENANT_MEDIA_REGEX = /^https:\/\/media\.test\/tenants\/abc\/[^"'\s]+$/

function extractImgSrcs(html: string): Array<string> {
  const out: Array<string> = []
  const re = /<img[^>]*?\ssrc\s*=\s*"([^"]+)"/gi
  let match: RegExpExecArray | null
  while ((match = re.exec(html)) !== null) {
    out.push(match[1])
  }
  return out
}

describe("renderVisualDoc — produced <img> URLs (T101)", () => {
  it("every <img src> in the rendered HTML matches the tenant media URL pattern", async () => {
    const doc: VisualDoc = {
      version: 1,
      type: "doc",
      content: [
        // Top-level image.
        {
          type: "image",
          attrs: {
            mediaRef: `${TENANT_MEDIA_PREFIX}hero.png`,
            alt: "Hero",
            href: "",
          },
        },
        // Image inside a two-column row — exercises the column-rendering
        // path that wraps content in a `<td>` and would otherwise be easy
        // to accidentally short-circuit.
        {
          type: "columns",
          attrs: { count: 2 },
          content: [
            {
              type: "column",
              content: [
                {
                  type: "image",
                  attrs: {
                    mediaRef: `${TENANT_MEDIA_PREFIX}left.png`,
                    alt: "Left",
                    href: "",
                  },
                },
              ],
            },
            {
              type: "column",
              content: [
                {
                  type: "image",
                  attrs: {
                    mediaRef: `${TENANT_MEDIA_PREFIX}right.png`,
                    alt: "Right",
                    href: "",
                  },
                },
              ],
            },
          ],
        },
        // A second top-level image with a wrapping href — the renderer
        // emits a hyperlinked image; both src and href must still resolve
        // to tenant-controlled targets.
        {
          type: "image",
          attrs: {
            mediaRef: `${TENANT_MEDIA_PREFIX}footer.png`,
            alt: "Footer",
            href: "https://example.test/sale",
          },
        },
      ],
    }

    const { bodyHtml } = await renderVisualDoc(doc, PlatformDefaultTheme)

    const srcs = extractImgSrcs(bodyHtml)
    // We supplied three image blocks; react-email may emit additional
    // tracking pixels in the future, in which case the regex still
    // applies. The lower-bound here pins the supplied images survived.
    expect(srcs.length).toBeGreaterThanOrEqual(3)
    for (const src of srcs) {
      expect(src).toMatch(TENANT_MEDIA_REGEX)
    }

    // FR-021 negative: no data URI anywhere in the produced HTML. The
    // visual editor never produces them, but a regression in the
    // renderer that base64-inlines an image would silently bypass the
    // tenant-media constraint, so we pin it explicitly.
    expect(bodyHtml).not.toMatch(/src\s*=\s*"data:/i)
  })

  it("preserves the supplied mediaRef byte-for-byte (no rewriting, no proxying)", async () => {
    const original = `${TENANT_MEDIA_PREFIX}exact-bytes-preserved.png`
    const doc: VisualDoc = {
      version: 1,
      type: "doc",
      content: [
        {
          type: "image",
          attrs: { mediaRef: original, alt: "x", href: "" },
        },
      ],
    }
    const { bodyHtml } = await renderVisualDoc(doc, PlatformDefaultTheme)
    expect(bodyHtml).toContain(`src="${original}"`)
  })
})
