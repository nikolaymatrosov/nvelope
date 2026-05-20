// Document envelope check. Mirrors the Go validator's outer guard in
// internal/campaign/domain/visualdoc_validate.go::Validate.
//
// Accepts `unknown` so it can act as the boundary between untrusted JSON
// (from the Nitro route's request body) and the typed VisualDoc the rest
// of the BFF passes around. After this check returns, callers can treat
// the value as a VisualDoc.

import { ValidatorError } from "./errors"
import type { VisualDoc } from "../render/types"

export function validateEnvelope(doc: unknown): asserts doc is VisualDoc {
  if (doc === null || doc === undefined || typeof doc !== "object") {
    throw new ValidatorError("invalid_doc", "document is required")
  }
  const d = doc as { type?: unknown; version?: unknown; content?: unknown }
  if (d.type !== "doc") {
    throw new ValidatorError("invalid_doc", `unexpected document type: ${String(d.type)}`)
  }
  if (d.version !== 1) {
    throw new ValidatorError(
      "invalid_doc",
      `unsupported document version: ${String(d.version)}`,
    )
  }
  if (!Array.isArray(d.content)) {
    throw new ValidatorError("invalid_doc", "document content must be an array")
  }
}
