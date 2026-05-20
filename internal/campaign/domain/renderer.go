package domain

// RenderWarning is a non-fatal note from the renderer or sanitizer — e.g.
// "the sanitizer removed a <script> tag from a RawHTML block". Warnings are
// returned to the operator alongside the saved row so they can correct or
// acknowledge the change; they do NOT block the save.
type RenderWarning struct {
	Kind   string
	Detail string
}

// Renderer is the consumer-owned interface the save commands use to turn a
// VisualDoc + Theme into email-ready HTML + plain text. The adapter that
// implements it (internal/campaign/adapters/visualrender) is responsible for
// sanitization, table-based layout, inline styling, and emission of the
// literal `{{ namespace.key }}` strings for MergeTag nodes.
//
// The interface is declared by the consumer (per Constitution VI: "Contracts
// are owned by the consumer"). The adapter conforms.
type Renderer interface {
	Render(doc *VisualDoc, theme Theme) (html string, text string, warnings []RenderWarning, err error)
}
