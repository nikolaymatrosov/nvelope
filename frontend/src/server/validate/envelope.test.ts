import { describe, expect, it } from "vitest"
import { validateEnvelope } from "./envelope"
import { ValidatorError } from "./errors"

describe("validateEnvelope", () => {
  it("accepts a well-formed envelope", () => {
    expect(() => validateEnvelope({ version: 1, type: "doc", content: [] })).not.toThrow()
  })

  it("rejects an unsupported version", () => {
    expect(() => validateEnvelope({ version: 2, type: "doc", content: [] })).toThrow(
      ValidatorError,
    )
  })

  it("rejects an unexpected type", () => {
    expect(() => validateEnvelope({ version: 1, type: "frag", content: [] })).toThrow(
      ValidatorError,
    )
  })

  it("rejects a non-array content", () => {
    expect(() => validateEnvelope({ version: 1, type: "doc", content: null })).toThrow(
      ValidatorError,
    )
  })

  it("rejects null/undefined input", () => {
    expect(() => validateEnvelope(null)).toThrow(ValidatorError)
    expect(() => validateEnvelope(undefined)).toThrow(ValidatorError)
  })
})
