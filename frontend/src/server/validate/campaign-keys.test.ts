// Cross-stack drift-catcher.
//
// The Go source `internal/campaign/domain/visualdoc.go` declares the
// canonical campaign-namespace allow-list as the `AllowedCampaignMergeTags`
// map literal. The TypeScript constant `AllowedCampaignMergeTags` in
// campaign-keys.ts MUST stay in sync — otherwise an operator authoring a
// new key in the visual editor would pass the BFF validator but get rejected
// by Go's defense-in-depth revalidation at save time (or vice versa, the
// editor's merge-tag picker would advertise a key Go doesn't recognize).
//
// This test parses the Go source at test time and fails the suite if the
// two sets diverge. Fixing the failure is a one-line edit in
// campaign-keys.ts (or visualdoc.go if the TS side was authoritative).

import { readFile } from "node:fs/promises"
import { fileURLToPath } from "node:url"
import { dirname, join } from "node:path"
import { describe, expect, it } from "vitest"

import { AllowedCampaignMergeTags } from "./campaign-keys"

const goSourcePath = join(
  dirname(fileURLToPath(import.meta.url)),
  "../../../../internal/campaign/domain/visualdoc.go",
)

// Lifts the keys out of the literal:
//
//   var AllowedCampaignMergeTags = map[string]bool{
//       "unsubscribe_url":     true,
//       ...
//   }
//
// Anchored to `var AllowedCampaignMergeTags = map[string]bool{` so we don't
// accidentally pick up another map literal that happens to use string keys.
function parseGoCampaignKeys(source: string): Set<string> {
  const start = source.indexOf("var AllowedCampaignMergeTags")
  if (start < 0) {
    throw new Error(
      "AllowedCampaignMergeTags declaration not found in Go source — drift-catcher cannot run",
    )
  }
  const open = source.indexOf("{", start)
  const close = source.indexOf("}", open)
  if (open < 0 || close < 0) {
    throw new Error("malformed AllowedCampaignMergeTags literal")
  }
  const body = source.slice(open + 1, close)
  const keys = new Set<string>()
  for (const match of body.matchAll(/"([^"]+)"\s*:/g)) {
    keys.add(match[1])
  }
  return keys
}

describe("AllowedCampaignMergeTags drift-catcher", () => {
  it("matches the Go source map literal exactly", async () => {
    const goSource = await readFile(goSourcePath, "utf8")
    const goKeys = parseGoCampaignKeys(goSource)
    const tsKeys = new Set(AllowedCampaignMergeTags)

    const goOnly = [...goKeys].filter((k) => !tsKeys.has(k)).sort()
    const tsOnly = [...tsKeys].filter((k) => !goKeys.has(k)).sort()

    expect(
      { goOnly, tsOnly },
      "Go-side AllowedCampaignMergeTags and TS-side campaign-keys.ts diverged. " +
        "Edit one of them (and the merge-tag picker label map in " +
        "internal/api/merge_tag_handlers.go) so all three stay aligned.",
    ).toEqual({ goOnly: [], tsOnly: [] })
  })
})
