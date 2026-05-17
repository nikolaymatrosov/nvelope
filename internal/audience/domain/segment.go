package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Conjunction joins the children of a segment group.
type Conjunction string

const (
	// ConjAnd requires every child condition to match.
	ConjAnd Conjunction = "and"
	// ConjOr requires at least one child condition to match.
	ConjOr Conjunction = "or"
)

// SegmentOp is a comparison operator in a segment condition.
type SegmentOp string

const (
	OpEq       SegmentOp = "eq"
	OpNeq      SegmentOp = "neq"
	OpExists   SegmentOp = "exists"
	OpContains SegmentOp = "contains"
	OpGt       SegmentOp = "gt"
	OpLt       SegmentOp = "lt"
	OpGte      SegmentOp = "gte"
	OpLte      SegmentOp = "lte"
)

// segmentFields is the set of subscriber fields a field condition may target.
var segmentFields = map[string]bool{"email": true, "name": true, "state": true}

// comparisonOps is the set of operators valid only for ordered operands.
var comparisonOps = map[SegmentOp]bool{OpGt: true, OpLt: true, OpGte: true, OpLte: true}

// knownOps is every valid operator.
var knownOps = map[SegmentOp]bool{
	OpEq: true, OpNeq: true, OpExists: true, OpContains: true,
	OpGt: true, OpLt: true, OpGte: true, OpLte: true,
}

// FieldCondition matches against a known subscriber field.
type FieldCondition struct {
	Field string
	Op    SegmentOp
	Value string
}

// AttrCondition matches against a custom-attribute key.
type AttrCondition struct {
	Key   string
	Op    SegmentOp
	Value any
}

// MemberCondition matches by membership of a list, optionally with a specific
// subscription status.
type MemberCondition struct {
	ListID string
	Status string
}

// Node is one node of a segment condition tree: either a group of children
// joined by a conjunction, or exactly one leaf condition.
type Node struct {
	Conj     Conjunction
	Children []Node
	Field    *FieldCondition
	Attr     *AttrCondition
	Member   *MemberCondition
}

// isGroup reports whether the node is a conjunction group.
func (n Node) isGroup() bool { return n.Conj != "" || len(n.Children) > 0 }

// Segment is a validated query selecting subscribers. Construction rejects a
// malformed query before it ever reaches the adapter (FR-015).
type Segment struct {
	root Node
}

// NewSegment validates a condition tree and returns a Segment.
func NewSegment(root Node) (*Segment, error) {
	if err := validateNode(root); err != nil {
		return nil, err
	}
	return &Segment{root: root}, nil
}

// Root returns the segment's root condition node.
func (s *Segment) Root() Node { return s.root }

// validateNode recursively checks that a node is well-formed.
func validateNode(n Node) error {
	if n.isGroup() {
		if n.Conj != ConjAnd && n.Conj != ConjOr {
			return apperr.NewIncorrectInput("invalid_segment", "a group needs a valid conjunction")
		}
		if len(n.Children) == 0 {
			return apperr.NewIncorrectInput("invalid_segment", "a group needs at least one child")
		}
		for _, c := range n.Children {
			if err := validateNode(c); err != nil {
				return err
			}
		}
		return nil
	}

	leaves := 0
	if n.Field != nil {
		leaves++
		if !segmentFields[n.Field.Field] {
			return apperr.NewIncorrectInput("invalid_segment", "unknown field: "+n.Field.Field)
		}
		if !knownOps[n.Field.Op] {
			return apperr.NewIncorrectInput("invalid_segment", "unknown operator")
		}
		if comparisonOps[n.Field.Op] {
			return apperr.NewIncorrectInput("invalid_segment",
				"comparison operators are not valid for text fields")
		}
	}
	if n.Attr != nil {
		leaves++
		if n.Attr.Key == "" {
			return apperr.NewIncorrectInput("invalid_segment", "an attribute key is required")
		}
		if !knownOps[n.Attr.Op] {
			return apperr.NewIncorrectInput("invalid_segment", "unknown operator")
		}
	}
	if n.Member != nil {
		leaves++
		if n.Member.ListID == "" {
			return apperr.NewIncorrectInput("invalid_segment", "a membership condition needs a list")
		}
		if n.Member.Status != "" && !ValidSubscriptionStatus(SubscriptionStatus(n.Member.Status)) {
			return apperr.NewIncorrectInput("invalid_segment", "unknown subscription status")
		}
	}
	if leaves != 1 {
		return apperr.NewIncorrectInput("invalid_segment",
			"a condition must be exactly one of field, attribute, or membership")
	}
	return nil
}
