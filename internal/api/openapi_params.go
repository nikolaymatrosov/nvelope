package api

import (
	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// Conversions from the generated request shapes into the values the
// application layer takes. Nothing here is specific to one bounded context:
// oapi-codegen models every optional query parameter and every optional body
// property as a pointer, and each generated operation needs the same handful of
// translations back out of that representation.

// deref returns the pointed-to value, or fallback when the pointer is nil.
//
// A nil pointer means the property or parameter was absent from the request.
// Where the schema declares a `default:`, the fallback passed here must be that
// default: kin-openapi validates against the schema but does not write defaults
// back into the decoded body, and oapi-codegen models a defaulted optional
// property as a pointer like any other. So the contract's default is applied at
// this seam — deliberately, and not left to the domain, which would let the
// two drift apart silently.
func deref[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

// pageFrom converts the generated optional limit/offset query parameters into
// the audience context's Page value. A nil pointer means the parameter was
// absent, which the domain reads as "server default".
//
// The three contexts that page (audience, campaign, iam) each declare their own
// structurally identical Page type, and Go will not let one generic function
// build a value of an unknown struct type, so a context whose operations are
// migrated later needs its own one-line sibling of this function rather than a
// type parameter here.
func pageFrom(limit, offset *int) audiencedomain.Page {
	p := audiencedomain.Page{}
	if limit != nil {
		p.Limit = *limit
	}
	if offset != nil {
		p.Offset = *offset
	}
	return p
}
