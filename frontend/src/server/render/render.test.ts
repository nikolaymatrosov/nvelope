// Golden tests: one canonical doc per block type, asserted byte-for-byte
// against fixture files under __fixtures__/. Updating a fixture is a
// deliberate PR-level change — the expected churn vector on minor react-email
// upgrades (per specs/014-visual-email-editor/research.md § R4 and the
// fixture-update note in plan.md).
//
// To regenerate every fixture in one pass after a deliberate change, run
// `pnpm vitest run --update src/server/render/render.test.ts` from
// frontend/.

import { fileURLToPath } from "node:url"
import { dirname, join } from "node:path"
import { describe, expect, it } from "vitest"

import { PlatformDefaultTheme, renderVisualDoc } from "./index"
import type { VisualDoc } from "./types"

const fixturesDir = join(dirname(fileURLToPath(import.meta.url)), "__fixtures__")
const fixture = (name: string) => join(fixturesDir, name)

function doc(...content: VisualDoc["content"]): VisualDoc {
  return { version: 1, type: "doc", content }
}

describe("renderVisualDoc — one fixture per block type", () => {
  it("paragraph with merge tag", async () => {
    const d = doc({
      type: "paragraph",
      content: [
        { type: "text", text: "Hi " },
        { type: "mergeTag", attrs: { namespace: "subscriber", key: "first_name" } },
        { type: "text", text: "!" },
      ],
    })
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("paragraph.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("paragraph.txt"))
  })

  it("heading levels", async () => {
    const d = doc(
      { type: "heading", attrs: { level: 1 }, content: [{ type: "text", text: "H1" }] },
      { type: "heading", attrs: { level: 2 }, content: [{ type: "text", text: "H2" }] },
      { type: "heading", attrs: { level: 3 }, content: [{ type: "text", text: "H3" }] },
    )
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("heading.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("heading.txt"))
  })

  it("bullet list", async () => {
    const d = doc({
      type: "bulletList",
      content: [
        {
          type: "listItem",
          content: [
            { type: "paragraph", content: [{ type: "text", text: "first" }] },
          ],
        },
        {
          type: "listItem",
          content: [
            { type: "paragraph", content: [{ type: "text", text: "second" }] },
          ],
        },
      ],
    })
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("bullet-list.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("bullet-list.txt"))
  })

  it("ordered list", async () => {
    const d = doc({
      type: "orderedList",
      content: [
        {
          type: "listItem",
          content: [
            { type: "paragraph", content: [{ type: "text", text: "alpha" }] },
          ],
        },
        {
          type: "listItem",
          content: [
            { type: "paragraph", content: [{ type: "text", text: "bravo" }] },
          ],
        },
      ],
    })
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("ordered-list.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("ordered-list.txt"))
  })

  it("blockquote", async () => {
    const d = doc({
      type: "blockquote",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "be excellent" }] },
      ],
    })
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("blockquote.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("blockquote.txt"))
  })

  it("code block", async () => {
    const d = doc({
      type: "codeBlock",
      content: [{ type: "text", text: "const answer = 42" }],
    })
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("code-block.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("code-block.txt"))
  })

  it("image with alt and href", async () => {
    const d = doc({
      type: "image",
      attrs: {
        mediaRef: "https://media.test/tenants/abc/banner.png",
        alt: "Spring sale banner",
        href: "https://example.test/sale",
      },
    })
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("image.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("image.txt"))
  })

  it("button", async () => {
    const d = doc({
      type: "button",
      attrs: { label: "Get started", href: "https://example.test/x" },
    })
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("button.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("button.txt"))
  })

  it("divider", async () => {
    const d = doc({ type: "divider" })
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("divider.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("divider.txt"))
  })

  it("two-column row", async () => {
    const d = doc({
      type: "columns",
      attrs: { count: 2 },
      content: [
        {
          type: "column",
          content: [
            { type: "paragraph", content: [{ type: "text", text: "left" }] },
          ],
        },
        {
          type: "column",
          content: [
            { type: "paragraph", content: [{ type: "text", text: "right" }] },
          ],
        },
      ],
    })
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("columns.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("columns.txt"))
  })

  it("raw HTML passthrough", async () => {
    const d = doc({
      type: "rawHtml",
      attrs: { html: "<p>verbatim <em>bytes</em></p>" },
    })
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("raw-html.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("raw-html.txt"))
  })

  it("merge tag from the campaign namespace", async () => {
    const d = doc({
      type: "paragraph",
      content: [
        { type: "text", text: "Unsubscribe at " },
        { type: "mergeTag", attrs: { namespace: "campaign", key: "unsubscribe_url" } },
      ],
    })
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("merge-tag-campaign.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("merge-tag-campaign.txt"))
  })
})
