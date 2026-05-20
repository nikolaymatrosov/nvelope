import { describe, expect, it } from "vitest"
import { validateLink } from "./link"
import { ValidatorError } from "./errors"

describe("validateLink", () => {
  it.each([
    "https://example.test/x",
    "http://example.test/x",
    "mailto:contact@example.test",
    "tel:+15551234567",
  ])("accepts %s", (href) => {
    expect(() => validateLink(href)).not.toThrow()
  })

  it.each([
    "javascript:alert(1)",
    "data:text/html,<script>",
    "vbscript:msgbox",
    "file:///etc/passwd",
    "/relative/path",
    "no-scheme",
    "",
    "   ",
  ])("rejects %s", (href) => {
    expect(() => validateLink(href)).toThrow(ValidatorError)
  })
})
