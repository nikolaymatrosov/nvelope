// TypeScript mirror of Go's `AllowedFontFamilies` map in
// internal/campaign/domain/visualdoc_validate.go. The per-block style picker
// (feature 017) offers exactly these email-safe font-family stacks; the
// validator rejects any `style.fontFamily` outside the set. Whenever a stack is
// added on the Go side, add it here too — the drift-catcher test (fonts.test.ts)
// reads the Go source at test time and fails the suite if the two diverge.

export const ALLOWED_FONT_FAMILIES: ReadonlySet<string> = new Set([
  "Arial, Helvetica, sans-serif",
  "Verdana, Geneva, sans-serif",
  "Tahoma, Geneva, sans-serif",
  "'Trebuchet MS', Helvetica, sans-serif",
  "Georgia, 'Times New Roman', serif",
  "'Times New Roman', Times, serif",
  "'Courier New', Courier, monospace",
  "Inter, Arial, sans-serif",
])

// fontLabel derives a short, human-friendly label (the first family name,
// unquoted) for a stack — used as the font dropdown's option text.
export function fontLabel(stack: string): string {
  const first = stack.split(",")[0]?.trim() ?? stack
  return first.replace(/^['"]|['"]$/g, "")
}
