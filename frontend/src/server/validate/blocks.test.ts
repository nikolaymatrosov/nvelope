import { describe, expect, it } from "vitest"
import { validateBlock } from "./blocks"
import { ValidatorError } from "./errors"

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
