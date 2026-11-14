// slashItems() builds the slash menu from the shared catalog with labels from
// the active i18n language (setup.ts initializes English).

import { describe, expect, it } from "vitest"
import { BLOCK_CATALOG } from "../blockCatalog"
import { slashItems } from "./SlashCommandMenu"

describe("slashItems", () => {
  it("lists every catalog block plus the merge tag, with translated labels", () => {
    const items = slashItems("")
    expect(items.map((i) => i.key)).toEqual([...BLOCK_CATALOG.map((i) => i.key), "merge-tag"])
    expect(items.find((i) => i.key === "heading-1")?.label).toBe("Heading 1")
    expect(items.find((i) => i.key === "merge-tag")?.description).toBe(
      "Insert a subscriber or campaign placeholder",
    )
  })

  it("filters by the typed query, case-insensitively", () => {
    expect(slashItems("HEAD").map((i) => i.key)).toEqual(["heading-1", "heading-2", "heading-3"])
  })

  it("matches the catalog key regardless of the translated label", () => {
    expect(slashItems("heading-2").map((i) => i.key)).toEqual(["heading-2"])
  })
})
