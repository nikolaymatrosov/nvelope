// Cross-stack drift-catcher for the per-block style font allow-list (feature
// 017). The Go source internal/campaign/domain/visualdoc_validate.go declares
// the canonical set as the `AllowedFontFamilies` map literal; the TS constant
// ALLOWED_FONT_FAMILIES in fonts.ts MUST stay in sync, or a font the operator
// picks in the editor would pass one validator and be rejected by the other.
// This parses the Go source at test time and fails if the two diverge.

import { readFile } from "node:fs/promises"
import { fileURLToPath } from "node:url"
import { dirname, join } from "node:path"
import { describe, expect, it } from "vitest"

import { ALLOWED_FONT_FAMILIES } from "./fonts"

const goSourcePath = join(
  dirname(fileURLToPath(import.meta.url)),
  "../../../../internal/campaign/domain/visualdoc_validate.go",
)

// Lifts the keys out of:
//
//   var AllowedFontFamilies = map[string]bool{
//       "Arial, Helvetica, sans-serif":          true,
//       ...
//   }
function parseGoFontFamilies(source: string): Set<string> {
  const start = source.indexOf("var AllowedFontFamilies")
  if (start < 0) {
    throw new Error(
      "AllowedFontFamilies declaration not found in Go source — drift-catcher cannot run",
    )
  }
  const open = source.indexOf("{", start)
  const close = source.indexOf("}", open)
  if (open < 0 || close < 0) {
    throw new Error("malformed AllowedFontFamilies literal")
  }
  const body = source.slice(open + 1, close)
  const families = new Set<string>()
  for (const match of body.matchAll(/"([^"]+)"\s*:/g)) {
    families.add(match[1])
  }
  return families
}

describe("AllowedFontFamilies drift-catcher", () => {
  it("matches the Go source map literal exactly", async () => {
    const goSource = await readFile(goSourcePath, "utf8")
    const goFamilies = parseGoFontFamilies(goSource)
    const tsFamilies = new Set(ALLOWED_FONT_FAMILIES)

    const goOnly = [...goFamilies].filter((f) => !tsFamilies.has(f)).sort()
    const tsOnly = [...tsFamilies].filter((f) => !goFamilies.has(f)).sort()

    expect(
      { goOnly, tsOnly },
      "Go-side AllowedFontFamilies and TS-side fonts.ts diverged. Edit one so " +
        "the per-block style font allow-list stays aligned across the stack.",
    ).toEqual({ goOnly: [], tsOnly: [] })
  })
})
