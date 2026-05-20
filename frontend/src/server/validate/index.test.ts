import { describe, expect, it } from "vitest"
import { ValidatorError, validateVisualDoc } from "./index"
import type { VisualDoc } from "../render/types"

const ctx = () => ({
  knownSlugs: new Set(["first_name", "email", "country"]),
  mediaUrlPrefix: "https://media.test/tenants/abc/",
})

function doc(...content: VisualDoc["content"]): VisualDoc {
  return { version: 1, type: "doc", content }
}

describe("validateVisualDoc", () => {
  it("accepts a well-formed doc", () => {
    const d = doc(
      {
        type: "paragraph",
        content: [
          { type: "text", text: "Hi " },
          { type: "mergeTag", attrs: { namespace: "subscriber", key: "first_name" } },
        ],
      },
      {
        type: "image",
        attrs: {
          mediaRef: "https://media.test/tenants/abc/x.png",
          alt: "x",
          href: "",
        },
      },
    )
    expect(() => validateVisualDoc(d, ctx())).not.toThrow()
  })

  it("batches every unknown subscriber slug into a single error", () => {
    const d = doc({
      type: "paragraph",
      content: [
        { type: "mergeTag", attrs: { namespace: "subscriber", key: "favourite_color" } },
        { type: "text", text: " " },
        { type: "mergeTag", attrs: { namespace: "subscriber", key: "shoe_size" } },
      ],
    })
    try {
      validateVisualDoc(d, ctx())
      throw new Error("expected ValidatorError")
    } catch (err) {
      expect(err).toBeInstanceOf(ValidatorError)
      const ve = err as ValidatorError
      expect(ve.kind).toBe("unknown_placeholder")
      expect(ve.placeholders.sort()).toEqual([
        "subscriber.favourite_color",
        "subscriber.shoe_size",
      ])
    }
  })

  it("surfaces invalid_media_ref before unknown_placeholder", () => {
    const d = doc({
      type: "image",
      attrs: { mediaRef: "https://evil.test/x.png", alt: "x", href: "" },
    })
    try {
      validateVisualDoc(d, ctx())
      throw new Error("expected ValidatorError")
    } catch (err) {
      expect(err).toBeInstanceOf(ValidatorError)
      expect((err as ValidatorError).kind).toBe("invalid_media_ref")
    }
  })
})
