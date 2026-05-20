package domain

// RenderWarning is a non-fatal note from the renderer or sanitizer — e.g.
// "the sanitizer removed a <script> tag from a RawHTML block". Warnings are
// returned to the operator alongside the saved row so they can correct or
// acknowledge the change; they do NOT block the save.
type RenderWarning struct {
	Kind   string
	Detail string
}

// Renderer is the legacy consumer-owned interface from the pre-pivot design.
// Rendering moved out of Go and into the TanStack Start + Nitro BFF — see
// specs/014-visual-email-editor/research.md § R4. This interface is retained
// only so the current Template/Campaign constructors compile; T033/T034 will
// rewrite those constructors to accept already-rendered HTML + plain text
// directly, after which this declaration should be removed.
type Renderer interface {
	Render(doc *VisualDoc, theme Theme) (html string, text string, warnings []RenderWarning, err error)
}
