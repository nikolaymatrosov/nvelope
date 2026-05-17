package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func TestNewListValid(t *testing.T) {
	t.Parallel()
	l, err := domain.NewList("t1", "  Newsletter  ", " weekly ",
		domain.VisibilityPublic, domain.OptInDouble, []string{" vip ", "", "news"})
	require.NoError(t, err)
	require.Equal(t, "Newsletter", l.Name(), "the name is trimmed")
	require.Equal(t, "weekly", l.Description())
	require.Equal(t, domain.VisibilityPublic, l.Visibility())
	require.Equal(t, domain.OptInDouble, l.OptIn())
	require.Equal(t, []string{"vip", "news"}, l.Tags(), "tags are trimmed and emptied dropped")
}

func TestNewListRejectsInvalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		tenantID   string
		listName   string
		visibility domain.Visibility
		optIn      domain.OptIn
	}{
		{"empty tenant", "", "L", domain.VisibilityPrivate, domain.OptInSingle},
		{"blank name", "t1", "   ", domain.VisibilityPrivate, domain.OptInSingle},
		{"bad visibility", "t1", "L", domain.Visibility("loud"), domain.OptInSingle},
		{"bad opt-in", "t1", "L", domain.VisibilityPrivate, domain.OptIn("triple")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := domain.NewList(tc.tenantID, tc.listName, "", tc.visibility, tc.optIn, nil)
			require.Error(t, err)
		})
	}
}

func TestListRename(t *testing.T) {
	t.Parallel()
	l, err := domain.NewList("t1", "Old", "", domain.VisibilityPrivate, domain.OptInSingle, nil)
	require.NoError(t, err)
	require.NoError(t, l.Rename("  New  "))
	require.Equal(t, "New", l.Name())
	require.Error(t, l.Rename("   "), "an empty rename is rejected")
}

func TestListDescribeAndRetag(t *testing.T) {
	t.Parallel()
	l, err := domain.NewList("t1", "L", "", domain.VisibilityPrivate, domain.OptInSingle, nil)
	require.NoError(t, err)
	l.Describe("  a list  ")
	require.Equal(t, "a list", l.Description())
	l.Retag([]string{"a", " ", "b"})
	require.Equal(t, []string{"a", "b"}, l.Tags())
}
