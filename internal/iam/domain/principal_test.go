package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestPrincipalCan(t *testing.T) {
	t.Parallel()
	p := domain.NewPrincipal(domain.PrincipalSession, "t1", "u1",
		[]domain.Permission{domain.PermListsGet}, nil)

	require.True(t, p.Can(domain.PermListsGet))
	require.False(t, p.Can(domain.PermListsManage))
}

func TestPrincipalCanOnList(t *testing.T) {
	t.Parallel()
	p := domain.NewPrincipal(domain.PrincipalSession, "t1", "u1",
		[]domain.Permission{domain.PermListsGet},
		map[string][]domain.Permission{
			"list-1": {domain.PermSubscribersManage},
		})

	// A tenant-level permission applies to every list.
	require.True(t, p.CanOnList(domain.PermListsGet, "list-1"))
	require.True(t, p.CanOnList(domain.PermListsGet, "list-2"))

	// A per-list role widens access for that list only.
	require.True(t, p.CanOnList(domain.PermSubscribersManage, "list-1"))
	require.False(t, p.CanOnList(domain.PermSubscribersManage, "list-2"),
		"a per-list role does not widen access for another list")
}

func TestPrincipalKindAndActor(t *testing.T) {
	t.Parallel()
	p := domain.NewPrincipal(domain.PrincipalAPIKey, "t1", "key-1", nil, nil)
	require.Equal(t, domain.PrincipalAPIKey, p.Kind())
	require.Equal(t, "key-1", p.ActorID())
}
