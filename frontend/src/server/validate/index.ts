// Public entry point for the TypeScript doc validator. The Nitro routes
// (visual-save, render-preview) call validateVisualDoc as a fast-feedback
// gate before render; Go's authoritative revalidation in
// internal/campaign/app/command/save_visual_{campaign,template}.go runs
// inside the persistence transaction (defense in depth).
//
// Unknown placeholders are collected into one batch error so the SPA can
// highlight every mistake in a single round-trip instead of forcing the
// operator to fix them one at a time.

import { validateBlock } from "./blocks"
import { validateEnvelope } from "./envelope"
import { ValidatorError } from "./errors"
import type { VisualDoc } from "../render/types"

export type ValidateContext = {
  knownSlugs: Set<string>
  // mediaUrlPrefix is the tenant's media-library base URL — read from
  // process.env.OBJECT_STORAGE_PUBLIC_BASE_URL in the Nitro routes.
  mediaUrlPrefix: string
}

export function validateVisualDoc(doc: VisualDoc, ctx: ValidateContext): void {
  validateEnvelope(doc)
  const unknownPlaceholders: Array<string> = []
  const blockCtx = {
    knownSlugs: ctx.knownSlugs,
    mediaUrlPrefix: ctx.mediaUrlPrefix,
    unknownPlaceholders,
  }
  for (const block of doc.content) {
    validateBlock(block, blockCtx)
  }
  if (unknownPlaceholders.length > 0) {
    throw new ValidatorError(
      "unknown_placeholder",
      `unknown subscriber field(s): ${unknownPlaceholders.join(", ")}`,
      unknownPlaceholders,
    )
  }
}

export { ValidatorError } from "./errors"
export type { ValidatorErrorKind } from "./errors"
