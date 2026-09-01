package api

import (
	"github.com/nikolaymatrosov/nvelope/internal/api/openapitenant"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// This file is the seam between the audience context and the generated wire
// types. Domain entities and query views are never serialized directly — every
// projection lives here, so a change to either shape has exactly one place to
// meet the other, and the handlers in audience_api.go stay free of field-by-
// field translation.

// listViewToAPI projects the audience context's read model onto the generated
// wire type.
//
// Tags is normalised to a non-nil slice: the schema declares tags as required,
// and a nil Go slice marshals to `null`, which the contract does not allow.
func listViewToAPI(v audiencequery.ListView) openapitenant.List {
	tags := v.Tags
	if tags == nil {
		tags = []string{}
	}
	return openapitenant.List{
		Id:          v.ID,
		Name:        v.Name,
		Description: v.Description,
		Visibility:  openapitenant.ListVisibility(v.Visibility),
		OptIn:       openapitenant.ListOptIn(v.OptIn),
		Tags:        tags,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
}

// listPageToAPI projects a page of lists onto the {items,total} envelope the
// contract declares. Items is always a non-nil slice, so an empty page is `[]`
// rather than `null`.
func listPageToAPI(page audiencequery.ListPage) openapitenant.ListPage {
	items := make([]openapitenant.List, 0, len(page.Lists))
	for _, v := range page.Lists {
		items = append(items, listViewToAPI(v))
	}
	return openapitenant.ListPage{Items: items, Total: page.Total}
}

// pageFrom converts the generated optional limit/offset query parameters into
// the audience context's Page value. A nil pointer means the parameter was
// absent, which the domain reads as "server default" — the same meaning the
// hand-written pageFromRequest gave a missing or unparseable parameter.
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

// derefString returns the pointed-to string, or "" when the optional property
// was absent from the request body.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// derefStrings returns the pointed-to slice, or nil when absent.
func derefStrings(p *[]string) []string {
	if p == nil {
		return nil
	}
	return *p
}

// derefVisibility returns the pointed-to visibility, defaulting to private.
// The schema declares `default: private`, but kin-openapi does not write
// defaults back into the decoded body and oapi-codegen models a defaulted
// optional property as a pointer, so the default is re-applied here rather
// than left to the domain.
func derefVisibility(p *openapitenant.ListVisibility) openapitenant.ListVisibility {
	if p == nil {
		return openapitenant.Private
	}
	return *p
}

// derefOptIn mirrors derefVisibility for the opt-in mode.
func derefOptIn(p *openapitenant.ListOptIn) openapitenant.ListOptIn {
	if p == nil {
		return openapitenant.Single
	}
	return *p
}
