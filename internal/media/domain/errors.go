package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Typed media-domain errors. Each carries the stable response slug and the
// transport-agnostic category; internal/api/errmap.go maps the category to an
// HTTP status in one place.
var (
	// ErrMediaNotFound is returned when no media asset matches a lookup; RLS
	// makes another tenant's asset invisible regardless of the id supplied.
	ErrMediaNotFound = apperr.NewNotFound("media_not_found", "no such media asset")

	// ErrUnsupportedMediaType is returned when the uploaded content type is
	// not on the allowlist.
	ErrUnsupportedMediaType = apperr.NewIncorrectInput("unsupported_media_type",
		"this file type is not supported")

	// ErrMediaTooLarge is returned when the uploaded file exceeds the
	// configured size cap.
	ErrMediaTooLarge = apperr.NewIncorrectInput("media_too_large",
		"this file exceeds the maximum upload size")

	// ErrEmptyUpload is returned when the request carries no bytes.
	ErrEmptyUpload = apperr.NewIncorrectInput("empty_upload", "the uploaded file is empty")

	// ErrEmptyFilename is returned when the upload has no usable filename.
	ErrEmptyFilename = apperr.NewIncorrectInput("empty_upload", "a filename is required")
)
