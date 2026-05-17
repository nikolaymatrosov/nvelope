package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func TestNewSegmentAcceptsValidConditions(t *testing.T) {
	t.Parallel()

	t.Run("field condition", func(t *testing.T) {
		t.Parallel()
		_, err := domain.NewSegment(domain.Node{
			Field: &domain.FieldCondition{Field: "email", Op: domain.OpContains, Value: "@acme"},
		})
		require.NoError(t, err)
	})

	t.Run("attribute condition", func(t *testing.T) {
		t.Parallel()
		_, err := domain.NewSegment(domain.Node{
			Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "pro"},
		})
		require.NoError(t, err)
	})

	t.Run("membership condition", func(t *testing.T) {
		t.Parallel()
		_, err := domain.NewSegment(domain.Node{
			Member: &domain.MemberCondition{ListID: "l1", Status: "confirmed"},
		})
		require.NoError(t, err)
	})

	t.Run("nested group", func(t *testing.T) {
		t.Parallel()
		seg, err := domain.NewSegment(domain.Node{
			Conj: domain.ConjAnd,
			Children: []domain.Node{
				{Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "pro"}},
				{Member: &domain.MemberCondition{ListID: "l1"}},
			},
		})
		require.NoError(t, err)
		require.Equal(t, domain.ConjAnd, seg.Root().Conj)
	})
}

func TestNewSegmentRejectsMalformedQueries(t *testing.T) {
	t.Parallel()

	cases := map[string]domain.Node{
		"unknown field": {
			Field: &domain.FieldCondition{Field: "phone", Op: domain.OpEq, Value: "x"},
		},
		"unknown operator on field": {
			Field: &domain.FieldCondition{Field: "email", Op: domain.SegmentOp("like"), Value: "x"},
		},
		"comparison operator on text field": {
			Field: &domain.FieldCondition{Field: "name", Op: domain.OpGt, Value: "x"},
		},
		"attribute without a key": {
			Attr: &domain.AttrCondition{Key: "", Op: domain.OpEq, Value: "x"},
		},
		"unknown operator on attribute": {
			Attr: &domain.AttrCondition{Key: "plan", Op: domain.SegmentOp("matches"), Value: "x"},
		},
		"membership without a list": {
			Member: &domain.MemberCondition{ListID: ""},
		},
		"membership with an unknown status": {
			Member: &domain.MemberCondition{ListID: "l1", Status: "bogus"},
		},
		"empty leaf": {},
		"two leaves at once": {
			Field: &domain.FieldCondition{Field: "email", Op: domain.OpEq, Value: "x"},
			Attr:  &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "x"},
		},
		"group without a conjunction": {
			Children: []domain.Node{
				{Field: &domain.FieldCondition{Field: "email", Op: domain.OpEq, Value: "x"}},
			},
		},
		"empty group": {Conj: domain.ConjAnd},
	}
	for name, node := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := domain.NewSegment(node)
			require.Error(t, err)
		})
	}
}

func TestNewSegmentRejectsMalformedChildren(t *testing.T) {
	t.Parallel()
	_, err := domain.NewSegment(domain.Node{
		Conj: domain.ConjOr,
		Children: []domain.Node{
			{Attr: &domain.AttrCondition{Key: "plan", Op: domain.OpEq, Value: "pro"}},
			{Field: &domain.FieldCondition{Field: "unknown", Op: domain.OpEq, Value: "x"}},
		},
	})
	require.Error(t, err, "a malformed child invalidates the whole segment")
}
