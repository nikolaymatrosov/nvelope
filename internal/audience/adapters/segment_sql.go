package adapters

import (
	"fmt"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// segmentTranslator turns a validated domain.Segment into a parameterized SQL
// WHERE clause. Every operand is bound as a parameter — the query is
// user-authored, so no value is ever interpolated into the SQL text.
type segmentTranslator struct {
	args []any
}

// translate returns the WHERE-clause SQL and the bound arguments for a segment.
func translateSegment(seg domain.Segment) (string, []any) {
	t := &segmentTranslator{}
	clause := t.node(seg.Root())
	if clause == "" {
		clause = "true"
	}
	return clause, t.args
}

// placeholder records an argument and returns its $N placeholder.
func (t *segmentTranslator) placeholder(v any) string {
	t.args = append(t.args, v)
	return fmt.Sprintf("$%d", len(t.args))
}

// node translates one condition-tree node.
func (t *segmentTranslator) node(n domain.Node) string {
	if n.Conj != "" || len(n.Children) > 0 {
		parts := make([]string, 0, len(n.Children))
		for _, c := range n.Children {
			parts = append(parts, t.node(c))
		}
		sep := " AND "
		if n.Conj == domain.ConjOr {
			sep = " OR "
		}
		return "(" + strings.Join(parts, sep) + ")"
	}
	switch {
	case n.Field != nil:
		return t.field(*n.Field)
	case n.Attr != nil:
		return t.attr(*n.Attr)
	case n.Member != nil:
		return t.member(*n.Member)
	default:
		return "true"
	}
}

// field translates a subscriber-field condition. The field name is validated
// by the domain, so it is safe to embed.
func (t *segmentTranslator) field(c domain.FieldCondition) string {
	col := "s." + c.Field
	switch c.Op {
	case domain.OpEq:
		return fmt.Sprintf("%s = %s", col, t.placeholder(c.Value))
	case domain.OpNeq:
		return fmt.Sprintf("%s <> %s", col, t.placeholder(c.Value))
	case domain.OpContains:
		return fmt.Sprintf("%s::text ILIKE '%%' || %s || '%%'", col, t.placeholder(c.Value))
	case domain.OpExists:
		return fmt.Sprintf("%s::text <> ''", col)
	default:
		return "true"
	}
}

// attr translates a custom-attribute condition over the jsonb column.
func (t *segmentTranslator) attr(c domain.AttrCondition) string {
	key := t.placeholder(c.Key)
	switch c.Op {
	case domain.OpExists:
		return fmt.Sprintf("s.attributes ? %s", key)
	case domain.OpEq:
		return fmt.Sprintf("s.attributes->>%s = %s", key, t.placeholder(fmt.Sprintf("%v", c.Value)))
	case domain.OpNeq:
		return fmt.Sprintf("s.attributes->>%s <> %s", key, t.placeholder(fmt.Sprintf("%v", c.Value)))
	case domain.OpContains:
		return fmt.Sprintf("s.attributes->>%s ILIKE '%%' || %s || '%%'",
			key, t.placeholder(fmt.Sprintf("%v", c.Value)))
	case domain.OpGt, domain.OpLt, domain.OpGte, domain.OpLte:
		return fmt.Sprintf("(s.attributes->>%s)::numeric %s (%s)::numeric",
			key, sqlComparison(c.Op), t.placeholder(fmt.Sprintf("%v", c.Value)))
	default:
		return "true"
	}
}

// member translates a list-membership condition.
func (t *segmentTranslator) member(c domain.MemberCondition) string {
	cond := fmt.Sprintf("sl.subscriber_id = s.id AND sl.list_id = %s", t.placeholder(c.ListID))
	if c.Status != "" {
		cond += fmt.Sprintf(" AND sl.subscription_status = %s", t.placeholder(c.Status))
	}
	return "EXISTS (SELECT 1 FROM subscriber_lists sl WHERE " + cond + ")"
}

// sqlComparison maps a comparison operator to its SQL symbol.
func sqlComparison(op domain.SegmentOp) string {
	switch op {
	case domain.OpGt:
		return ">"
	case domain.OpLt:
		return "<"
	case domain.OpGte:
		return ">="
	case domain.OpLte:
		return "<="
	default:
		return "="
	}
}
