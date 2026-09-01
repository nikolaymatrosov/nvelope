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

// subscriberViewToAPI projects the audience context's read model onto the
// generated wire type.
//
// Attributes is normalised to a non-nil map and Memberships to a non-nil
// slice: both are required by the schema, and a nil Go value of either would
// marshal to `null`, which the contract does not allow.
func subscriberViewToAPI(v audiencequery.SubscriberView) openapitenant.Subscriber {
	attrs := v.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	memberships := make([]openapitenant.Membership, 0, len(v.Memberships))
	for _, m := range v.Memberships {
		memberships = append(memberships, openapitenant.Membership{
			ListId: m.ListID,
			Status: openapitenant.SubscriptionStatus(m.Status),
		})
	}
	return openapitenant.Subscriber{
		Id:          v.ID,
		Email:       v.Email,
		Name:        v.Name,
		State:       openapitenant.SubscriberState(v.State),
		Attributes:  attrs,
		Memberships: memberships,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
}

// subscriberPageToAPI projects a page of subscribers onto the {items,total}
// envelope the contract declares, the same shape searchSubscribers,
// querySubscribers both return.
func subscriberPageToAPI(page audiencequery.SubscriberPage) openapitenant.SubscriberPage {
	items := make([]openapitenant.Subscriber, 0, len(page.Subscribers))
	for _, v := range page.Subscribers {
		items = append(items, subscriberViewToAPI(v))
	}
	return openapitenant.SubscriberPage{Items: items, Total: page.Total}
}

// segmentNodeFromAPI converts the generated recursive wire type into the
// domain's condition tree. audiencedomain.NewSegment validates the resulting
// tree — this function only re-shapes it. Keeping the two types separate is
// what lets the wire format use lowerCamelCase field names without adding
// json tags to a domain type.
//
// Recursion mirrors the wire shape one-for-one: a node is either a group
// (Conj + Children) or exactly one leaf (Field, Attr, or Member), and each
// child in Children is converted by the same function. The domain, not this
// function, is what pins "exactly one leaf" and "a known conjunction" — see
// validateNode in internal/audience/domain/segment.go.
// jobStatusToAPI projects an import/export job's read model onto the wire
// type. Failures is normalised to a non-nil slice for the same reason Tags and
// Attributes are elsewhere in this file: the schema declares it required, and
// a nil Go slice marshals to `null`.
func jobStatusToAPI(v audiencequery.JobStatusView) openapitenant.JobStatus {
	failures := make([]openapitenant.RowFailure, 0, len(v.Failures))
	for _, f := range v.Failures {
		failures = append(failures, openapitenant.RowFailure{Row: f.Row, Reason: f.Reason})
	}
	return openapitenant.JobStatus{
		Id:           v.ID,
		Kind:         openapitenant.JobStatusKind(v.Kind),
		Status:       openapitenant.JobStatusStatus(v.Status),
		FileName:     v.FileName,
		CreatedCount: v.CreatedCount,
		UpdatedCount: v.UpdatedCount,
		FailedCount:  v.FailedCount,
		RowCount:     v.RowCount,
		Failures:     failures,
	}
}

func segmentNodeFromAPI(n openapitenant.SegmentNode) audiencedomain.Node {
	out := audiencedomain.Node{}
	if n.Conj != nil {
		out.Conj = audiencedomain.Conjunction(*n.Conj)
	}
	if n.Children != nil {
		out.Children = make([]audiencedomain.Node, 0, len(*n.Children))
		for _, c := range *n.Children {
			out.Children = append(out.Children, segmentNodeFromAPI(c))
		}
	}
	if n.Field != nil {
		out.Field = &audiencedomain.FieldCondition{
			Field: n.Field.Field,
			Op:    audiencedomain.SegmentOp(n.Field.Op),
			Value: deref(n.Field.Value, ""),
		}
	}
	if n.Attr != nil {
		out.Attr = &audiencedomain.AttrCondition{
			Key:   n.Attr.Key,
			Op:    audiencedomain.SegmentOp(n.Attr.Op),
			Value: n.Attr.Value,
		}
	}
	if n.Member != nil {
		out.Member = &audiencedomain.MemberCondition{
			ListID: n.Member.ListId,
			Status: string(deref(n.Member.Status, "")),
		}
	}
	return out
}
