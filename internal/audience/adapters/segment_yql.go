package adapters

import (
	"fmt"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// ydbSegmentArg is one Utf8 parameter of a translated segment.
type ydbSegmentArg struct {
	name, value string
}

// ydbSegmentTranslator turns a validated domain.Segment into a parameterized
// YQL predicate over the subscribers table aliased s. It mirrors
// segmentTranslator (segment_sql.go). Every operand, attribute keys included,
// is bound as a Utf8 parameter: the query is user-authored, so no value is
// ever interpolated into the YQL text.
type ydbSegmentTranslator struct {
	args []ydbSegmentArg
}

// translateSegmentYQL returns the predicate and its parameters, named $p1,
// $p2, ... The predicate also reads $tenant_id, which the caller binds.
func translateSegmentYQL(seg domain.Segment) (string, []ydbSegmentArg) {
	t := &ydbSegmentTranslator{}
	clause := t.node(seg.Root())
	if clause == "" {
		clause = "true"
	}
	return clause, t.args
}

// param records an argument and returns its parameter name.
func (t *ydbSegmentTranslator) param(v string) string {
	name := fmt.Sprintf("$p%d", len(t.args)+1)
	t.args = append(t.args, ydbSegmentArg{name: name, value: v})
	return name
}

// node translates one condition-tree node.
func (t *ydbSegmentTranslator) node(n domain.Node) string {
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
func (t *ydbSegmentTranslator) field(c domain.FieldCondition) string {
	col := "s." + c.Field
	value := c.Value
	if c.Field == "email" {
		// Postgres compares the citext email case-insensitively. YDB stores
		// it lower-cased (domain.NewSubscriber), so lower the operand.
		value = strings.ToLower(value)
	}
	switch c.Op {
	case domain.OpEq:
		return fmt.Sprintf("%s = %s", col, t.param(value))
	case domain.OpNeq:
		return fmt.Sprintf("%s != %s", col, t.param(value))
	case domain.OpContains:
		return fmt.Sprintf(`%s ILIKE "%%"u || %s || "%%"u`, col, t.param(value))
	case domain.OpExists:
		return fmt.Sprintf(`%s != ""u`, col)
	default:
		return "true"
	}
}

// attr translates a custom-attribute condition over the JsonDocument column.
// The JsonPath picks the member whose name equals the bound key through
// keyvalue(), so the key is never part of the path text. JSON_VALUE returns a
// scalar as text, as Postgres ->> does, and NULL for an object or array.
func (t *ydbSegmentTranslator) attr(c domain.AttrCondition) string {
	path := fmt.Sprintf(`"$.keyvalue() ? (@.name == $k).value" PASSING %s AS k`, t.param(c.Key))
	value := "JSON_VALUE(s.attributes, " + path + ")"
	operand := func() string { return t.param(fmt.Sprintf("%v", c.Value)) }
	switch c.Op {
	case domain.OpExists:
		return "JSON_EXISTS(s.attributes, " + path + ")"
	case domain.OpEq:
		return fmt.Sprintf("%s = %s", value, operand())
	case domain.OpNeq:
		return fmt.Sprintf("%s != %s", value, operand())
	case domain.OpContains:
		return fmt.Sprintf(`%s ILIKE "%%"u || %s || "%%"u`, value, operand())
	case domain.OpGt, domain.OpLt, domain.OpGte, domain.OpLte:
		// A value that is not a number casts to NULL and matches nothing;
		// Postgres fails the whole query instead.
		return fmt.Sprintf("CAST(%s AS Double) %s CAST(%s AS Double)",
			value, sqlComparison(c.Op), operand())
	default:
		return "true"
	}
}

// member translates a list-membership condition.
func (t *ydbSegmentTranslator) member(c domain.MemberCondition) string {
	cond := "tenant_id = $tenant_id AND list_id = " + t.param(c.ListID)
	if c.Status != "" {
		cond += " AND subscription_status = " + t.param(c.Status)
	}
	return "s.id IN (SELECT subscriber_id FROM subscriber_lists VIEW idx_subscriber_lists_list WHERE " +
		cond + ")"
}

// ydbSegmentDeclares returns the DECLARE statements for a segment's
// parameters.
func ydbSegmentDeclares(args []ydbSegmentArg) string {
	var b strings.Builder
	for _, a := range args {
		b.WriteString("DECLARE " + a.name + " AS Utf8;\n")
	}
	return b.String()
}
