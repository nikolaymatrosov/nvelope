package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Typed errors raised by the visual-editor save and render paths. They live
// in their own file to keep visualdoc.go pure (data types only). Each
// carries a stable response slug and the transport-agnostic category that
// internal/api/errmap.go maps to an HTTP status in one place.
var (
	// ErrVisualDocInvalid is the generic "the document does not parse / does
	// not satisfy a shape rule" error. Callers attach a specific message via
	// apperr.WithMessage when they raise it.
	ErrVisualDocInvalid = apperr.NewIncorrectInput("invalid_doc",
		"the visual document is invalid")

	// ErrInvalidPlaceholder is returned when a merge tag's namespace is not
	// one of the supported namespaces or a campaign-namespace key is not in
	// the platform allow-list.
	ErrInvalidPlaceholder = apperr.NewIncorrectInput("invalid_placeholder",
		"the placeholder is not valid")

	// ErrUnknownSlug is returned when a subscriber-namespace placeholder
	// references a slug that is not present in the tenant's subscriber
	// custom-field registry (built-in pseudo-rows count). Save-time gate
	// from FR-016c.
	ErrUnknownSlug = apperr.NewIncorrectInput("unknown_placeholder",
		"the subscriber field is not defined for this tenant")

	// ErrInvalidMediaRef is returned when an Image block's MediaRef is not a
	// tenant-scoped media-library URL. Save-time gate from FR-021.
	ErrInvalidMediaRef = apperr.NewIncorrectInput("invalid_media_ref",
		"the image must reference a tenant media-library asset")

	// ErrInvalidStyle is returned when a block's per-block style (feature 017)
	// carries an out-of-range, malformed, or unsupported value — a color that
	// is not #RGB/#RRGGBB, a numeric outside its bound, an unknown enum value,
	// or a font family outside the platform allow-list. Callers attach the
	// offending field via apperr.WithMessage. Mirrors the BFF validator's
	// "invalid_style" kind.
	ErrInvalidStyle = apperr.NewIncorrectInput("invalid_style",
		"the block style is invalid")

	// ErrSanitizationStripped is informational — used by the renderer to
	// flag that the sanitizer removed content the operator may want to be
	// aware of. The renderer returns it as a warning, not a save-blocking
	// error.
	ErrSanitizationStripped = apperr.NewIncorrectInput("sanitizer_stripped",
		"the sanitizer removed disallowed content")

	// ErrUnsupportedNode is returned during raw-HTML → VisualDoc conversion
	// when a node cannot be represented as a block and the conversion did
	// not fall back to a RawHTML block. Reserved for future use.
	ErrUnsupportedNode = apperr.NewIncorrectInput("unsupported_node",
		"the node cannot be converted to a visual block")

	// ErrAlreadyVisual is returned by the convert-to-visual endpoints when
	// the row already carries a structured body_doc, so there is nothing
	// to convert and the operator should open the visual editor directly
	// (per contracts/tenant-api.md "already_visual").
	ErrAlreadyVisual = apperr.NewConflict("already_visual",
		"the row already has a visual document and does not need conversion")
)
