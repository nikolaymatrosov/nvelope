// Golden tests for inline mark combinations on a Text node.

import { fileURLToPath } from "node:url"
import { dirname, join } from "node:path"
import { describe, expect, it } from "vitest"

import { PlatformDefaultTheme, renderVisualDoc } from "./index"
import type { Mark, VisualDoc } from "./types"

const fixturesDir = join(dirname(fileURLToPath(import.meta.url)), "__fixtures__")
const fixture = (name: string) => join(fixturesDir, name)

function paragraph(text: string, marks: Array<Mark>): VisualDoc {
  return {
    version: 1,
    type: "doc",
    content: [
      {
        type: "paragraph",
        content: [{ type: "text", text, marks }],
      },
    ],
  }
}

describe("renderVisualDoc — mark combinations on a single Text run", () => {
  it("bold", async () => {
    const d = paragraph("emphasis", [{ type: "bold" }])
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("mark-bold.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("mark-bold.txt"))
  })

  it("italic", async () => {
    const d = paragraph("emphasis", [{ type: "italic" }])
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("mark-italic.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("mark-italic.txt"))
  })

  it("underline", async () => {
    const d = paragraph("underline", [{ type: "underline" }])
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("mark-underline.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("mark-underline.txt"))
  })

  it("strike", async () => {
    const d = paragraph("strike", [{ type: "strike" }])
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("mark-strike.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("mark-strike.txt"))
  })

  it("color", async () => {
    const d = paragraph("colored", [{ type: "color", attrs: { color: "#cc0000" } }])
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("mark-color.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("mark-color.txt"))
  })

  it("link", async () => {
    const d = paragraph("the docs", [
      { type: "link", attrs: { href: "https://example.test/docs" } },
    ])
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("mark-link.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("mark-link.txt"))
  })

  it("bold + italic + link composed", async () => {
    const d = paragraph("hot link", [
      { type: "bold" },
      { type: "italic" },
      { type: "link", attrs: { href: "https://example.test/x" } },
    ])
    const { bodyHtml, bodyText } = await renderVisualDoc(d, PlatformDefaultTheme)
    await expect(bodyHtml).toMatchFileSnapshot(fixture("mark-combo.html"))
    await expect(bodyText).toMatchFileSnapshot(fixture("mark-combo.txt"))
  })
})
