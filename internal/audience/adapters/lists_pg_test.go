package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func newList(t *testing.T, tenantID, name string) *domain.List {
	t.Helper()
	l, err := domain.NewList(tenantID, name, "desc", domain.VisibilityPrivate, domain.OptInSingle, []string{"x"})
	require.NoError(t, err)
	return l
}

// addList persists a list and returns its id.
func addList(t *testing.T, repo *adapters.Lists, tenantID, name string) string {
	t.Helper()
	id, err := repo.Add(context.Background(), tenantID, newList(t, tenantID, name))
	require.NoError(t, err)
	return id
}

func TestListsAddGetAll(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewLists(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	addList(t, repo, tenantID, "Newsletter")
	addList(t, repo, tenantID, "Announcements")

	lists, total, err := repo.All(ctx, tenantID, domain.DefaultPage)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, lists, 2)
	require.Equal(t, "Announcements", lists[0].Name(), "lists are ordered by name")

	got, err := repo.Get(ctx, tenantID, lists[0].ID())
	require.NoError(t, err)
	require.Equal(t, "Announcements", got.Name())
	require.Equal(t, []string{"x"}, got.Tags())
}

func TestListsAddDuplicateNameConflicts(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewLists(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	addList(t, repo, tenantID, "Dup")
	_, err := repo.Add(ctx, tenantID, newList(t, tenantID, "Dup"))
	require.ErrorIs(t, err, domain.ErrListNameTaken)
}

func TestListsUpdateAndDelete(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewLists(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id := addList(t, repo, tenantID, "Old")

	require.NoError(t, repo.Update(ctx, tenantID, id, func(l *domain.List) (*domain.List, error) {
		return l, l.Rename("Renamed")
	}))
	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, "Renamed", got.Name())

	require.NoError(t, repo.Delete(ctx, tenantID, id))
	_, err = repo.Get(ctx, tenantID, id)
	require.ErrorIs(t, err, domain.ErrListNotFound)
}

func TestListsGetMissing(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewLists(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	_, err := repo.Get(ctx, tenantID, "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrListNotFound)
}
