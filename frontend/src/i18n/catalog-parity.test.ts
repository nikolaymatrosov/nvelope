import { describe, expect, it } from "vitest"

// Every translation catalog, loaded straight from disk.
const enModules = import.meta.glob("../locales/en/*.json", {
  eager: true,
  import: "default",
})
const ruModules = import.meta.glob("../locales/ru/*.json", {
  eager: true,
  import: "default",
})

function namespaceOf(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1).replace(/\.json$/, "")
}

function byNamespace(
  modules: Record<string, unknown>
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [path, mod] of Object.entries(modules)) {
    out[namespaceOf(path)] = mod
  }
  return out
}

// Flattens a nested catalog into a sorted list of dotted leaf keys.
function leafKeys(obj: unknown, prefix = ""): Array<string> {
  if (obj === null || typeof obj !== "object") return [prefix]
  const keys: Array<string> = []
  for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
    keys.push(...leafKeys(v, prefix ? `${prefix}.${k}` : k))
  }
  return keys.sort()
}

const en = byNamespace(enModules)
const ru = byNamespace(ruModules)

describe("translation catalog parity", () => {
  it("defines the same namespaces in English and Russian", () => {
    expect(Object.keys(ru).sort()).toEqual(Object.keys(en).sort())
  })

  for (const ns of Object.keys(en)) {
    it(`ru/${ns}.json has exactly the keys of en/${ns}.json`, () => {
      expect(leafKeys(ru[ns] ?? {})).toEqual(leafKeys(en[ns]))
    })
  }
})
