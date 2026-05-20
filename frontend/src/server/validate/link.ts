// Link-scheme allow-list. Mirrors the Go validator's check in
// internal/campaign/domain/visualdoc_validate.go::validateLink — http, https,
// mailto, tel only. Other schemes (javascript:, data:, vbscript:, file:) are
// refused at save time so they cannot reach the sanitizer.

import { ValidatorError } from "./errors"

const ALLOWED_SCHEMES = new Set(["http", "https", "mailto", "tel"])

export function validateLink(href: string): void {
  if (!href.trim()) {
    throw new ValidatorError("invalid_doc", "link href is required")
  }
  let url: URL
  try {
    // URL needs an absolute spec; mailto: and tel: are URI schemes, so we
    // can construct them directly. For relative paths the constructor
    // throws — those are rejected since email-HTML hrefs must be absolute.
    url = new URL(href)
  } catch {
    throw new ValidatorError("invalid_doc", "link href is malformed")
  }
  const scheme = url.protocol.replace(/:$/, "").toLowerCase()
  if (!ALLOWED_SCHEMES.has(scheme)) {
    throw new ValidatorError(
      "invalid_doc",
      "link scheme must be http, https, mailto, or tel",
    )
  }
}
