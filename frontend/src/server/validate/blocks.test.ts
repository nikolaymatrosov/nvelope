import { describe, expect, it } from "vitest"
import { validateBlock, validateStyle } from "./blocks"
import { ValidatorError } from "./errors"
import type { BlockStyle } from "../render/types"

const ctx = () => ({
  knownSlugs: new Set(["first_name", "email", "country"]),
  mediaUrlPrefix: "https://media.test/tenants/abc/",
  unknownPlaceholders: undefined as Array<string> | undefined,
})

describe("validateBlock — heading", () => {
  it("accepts levels 1, 2, 3", () => {
    for (const level of [1, 2, 3] as const) {
      expect(() =>
        validateBlock(
          { type: "heading", attrs: { level }, content: [{ type: "text", text: "x" }] },
          ctx(),
        ),
      ).not.toThrow()
    }
  })

  it("rejects out-of-range levels", () => {
    expect(() =>
      validateBlock(
        // @ts-expect-error — exercising the runtime check
        { type: "heading", attrs: { level: 4 }, content: [{ type: "text", text: "x" }] },
        ctx(),
      ),
    ).toThrow(ValidatorError)
  })
})

describe("validateBlock — columns", () => {
  it("requires content length to match attrs.count", () => {
    expect(() =>
      validateBlock(
        {
          type: "columns",
          attrs: { count: 3 },
          content: [
            { type: "column", content: [] },
            { type: "column", content: [] },
          ],
        },
        ctx(),
      ),
    ).toThrow(ValidatorError)
  })

  it("rejects 1 or 5+ columns", () => {
    expect(() =>
      validateBlock(
        {
          type: "columns",
          // @ts-expect-error — exercising the runtime check
          attrs: { count: 1 },
          content: [{ type: "column", content: [] }],
        },
        ctx(),
      ),
    ).toThrow(ValidatorError)
  })

  it("accepts 2 / 3 / 4-column rows when count matches", () => {
    for (const count of [2, 3, 4] as const) {
      expect(() =>
        validateBlock(
          {
            type: "columns",
            attrs: { count },
            content: Array.from({ length: count }, () => ({ type: "column" as const, content: [] })),
          },
          ctx(),
        ),
      ).not.toThrow()
    }
  })
})

describe("validateBlock — image", () => {
  it("rejects mediaRef outside the tenant media prefix", () => {
    expect(() =>
      validateBlock(
        {
          type: "image",
          attrs: {
            mediaRef: "https://evil.test/x.png",
            alt: "x",
            href: "",
          },
        },
        ctx(),
      ),
    ).toThrow(ValidatorError)
  })

  it("accepts mediaRef under the tenant prefix", () => {
    expect(() =>
      validateBlock(
        {
          type: "image",
          attrs: {
            mediaRef: "https://media.test/tenants/abc/banner.png",
            alt: "x",
            href: "",
          },
        },
        ctx(),
      ),
    ).not.toThrow()
  })
})

describe("validateBlock — merge tags", () => {
  it("accepts a built-in or registry subscriber slug", () => {
    expect(() =>
      validateBlock(
        {
          type: "paragraph",
          content: [
            { type: "mergeTag", attrs: { namespace: "subscriber", key: "first_name" } },
          ],
        },
        ctx(),
      ),
    ).not.toThrow()
  })

  it("flags an unknown subscriber slug (single-block path)", () => {
    expect(() =>
      validateBlock(
        {
          type: "paragraph",
          content: [
            { type: "mergeTag", attrs: { namespace: "subscriber", key: "favourite_color" } },
          ],
        },
        ctx(),
      ),
    ).toThrow(ValidatorError)
  })

  it("accepts every campaign-namespace key in the allow-list", () => {
    for (const key of [
      "unsubscribe_url",
      "preference_url",
      "archive_url",
      "view_in_browser_url",
      "tenant_name",
      "current_date",
    ]) {
      expect(() =>
        validateBlock(
          {
            type: "paragraph",
            content: [{ type: "mergeTag", attrs: { namespace: "campaign", key } }],
          },
          ctx(),
        ),
      ).not.toThrow()
    }
  })

  it("rejects an unknown campaign-namespace key", () => {
    expect(() =>
      validateBlock(
        {
          type: "paragraph",
          content: [
            { type: "mergeTag", attrs: { namespace: "campaign", key: "unknown_url" } },
          ],
        },
        ctx(),
      ),
    ).toThrow(ValidatorError)
  })
})

describe("validateStyle — per-block style bounds (feature 017)", () => {
  it("accepts a fully-specified valid style", () => {
    const style: BlockStyle = {
      backgroundColor: "#1a73e8",
      color: "#fff",
      fontFamily: "Arial, Helvetica, sans-serif",
      fontSize: 16,
      fontWeight: 700,
      lineHeight: 1.5,
      textAlign: "center",
      paddingTop: 12,
      paddingRight: 20,
      paddingBottom: 12,
      paddingLeft: 20,
      borderRadius: 8,
      borderWidth: 1,
      borderStyle: "solid",
      borderColor: "#0b57d0",
    }
    expect(() => validateStyle(style)).not.toThrow()
  })

  it("treats undefined / empty style as inherit (no throw)", () => {
    expect(() => validateStyle(undefined)).not.toThrow()
    expect(() => validateStyle({})).not.toThrow()
  })

  it("rejects out-of-range, malformed, and unknown-enum values", () => {
    const bad: Array<BlockStyle> = [
      { backgroundColor: "red" },
      { color: "#12" },
      { borderColor: "rgb(0,0,0)" },
      { fontFamily: "Comic Sans MS" },
      { fontSize: 4 },
      { fontSize: 999 },
      // @ts-expect-error — runtime check on a value the type forbids
      { fontWeight: 500 },
      { lineHeight: 5 },
      // @ts-expect-error — runtime check on a value the type forbids
      { textAlign: "justify" },
      { paddingTop: 100 },
      { borderRadius: 999 },
      { borderWidth: 20 },
      // @ts-expect-error — runtime check on a value the type forbids
      { borderStyle: "groove" },
    ]
    for (const style of bad) {
      expect(() => validateStyle(style), JSON.stringify(style)).toThrow(ValidatorError)
    }
  })

  it("surfaces the invalid_style kind", () => {
    try {
      validateStyle({ fontSize: 1000 })
      throw new Error("expected validateStyle to throw")
    } catch (err) {
      expect(err).toBeInstanceOf(ValidatorError)
      expect((err as ValidatorError).kind).toBe("invalid_style")
    }
  })

  it("rejects an invalid style reached through a block (button)", () => {
    expect(() =>
      validateBlock(
        {
          type: "button",
          attrs: { label: "Go", href: "https://example.test/x", style: { borderRadius: 999 } },
        },
        ctx(),
      ),
    ).toThrow(ValidatorError)
  })
})

describe("validateBlock — listItem/column at root", () => {
  it("rejects orphan listItem at the document root", () => {
    expect(() =>
      validateBlock({ type: "listItem", content: [] }, ctx()),
    ).toThrow(ValidatorError)
  })

  it("rejects orphan column at the document root", () => {
    expect(() =>
      validateBlock({ type: "column", content: [] }, ctx()),
    ).toThrow(ValidatorError)
  })
})
