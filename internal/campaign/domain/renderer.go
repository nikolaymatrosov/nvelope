package domain

// RenderWarning is a non-fatal note from the sanitizer — e.g.
// "the sanitizer removed a <script> tag from a RawHTML block". Warnings are
// returned to the operator alongside the saved row so they can correct or
// acknowledge the change; they do NOT block the save.
//
// Rendering itself lives outside Go in the TanStack Start + Nitro BFF — see
// specs/014-visual-email-editor/research.md § R4. The Go API's role is
// validate → sanitize → persist; warnings originate from the bluemonday
// sanitization pass that runs over the BFF-rendered HTML before persistence.
type RenderWarning struct {
	Kind   string
	Detail string
}
