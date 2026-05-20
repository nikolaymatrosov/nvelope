// Validator error kinds. The transport layer (visual-save and render-preview
// Nitro routes) maps these to the HTTP error envelope kinds defined in
// specs/014-visual-email-editor/contracts/tenant-api.md.

export type ValidatorErrorKind =
  | "invalid_doc"
  | "unknown_placeholder"
  | "invalid_media_ref"

export class ValidatorError extends Error {
  readonly kind: ValidatorErrorKind
  // For `unknown_placeholder`, the placeholders the operator referenced that
  // don't resolve in the registry — surfaced in the response so the SPA can
  // highlight each one in the editor.
  readonly placeholders: Array<string>

  constructor(kind: ValidatorErrorKind, message: string, placeholders: Array<string> = []) {
    super(message)
    this.kind = kind
    this.placeholders = placeholders
  }
}
